package stream

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rubenpari/lumen/internal/db"
)

// Options select how an item should be transcoded.
type Options struct {
	Start       float64 // seconds
	AudioStream int     // absolute stream index, -1 = default
	MaxHeight   int     // 0 = original
	MaxBitrate  int     // kbps, 0 = unlimited
	ForceVideo  bool    // force video re-encode
	// SegmentLength in seconds (default 4) and StartNumber of the first segment.
	SegmentLength int
	StartNumber   int
	// VOD keeps output timestamps aligned with the source (used for
	// pre-computed VOD playlists served to Jellyfin clients).
	VOD bool
	// Live produces a sliding-window playlist from an endless input.
	Live bool
	// InputOptions are placed before -i (user agent, reconnect, -re...).
	InputOptions []string
	// BurnSubtitle is the absolute index of an image subtitle stream (PGS,
	// VobSub) to burn into the video; -1 disables it.
	BurnSubtitle int
}

// Session is a running ffmpeg HLS job.
type Session struct {
	ID          string
	ItemID      string
	Dir         string
	Start       float64
	StartNumber int
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	lastSeen    time.Time
	done        chan struct{}
	err         error
}

// Manager keeps track of transcoding sessions.
type Manager struct {
	FFmpeg  string
	TmpDir  string
	HWAccel string // "", "vaapi", "nvenc", "qsv", "videotoolbox"
	Log     *slog.Logger

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager creates a manager and starts the idle reaper.
func NewManager(ffmpeg, tmp string, log *slog.Logger) *Manager {
	m := &Manager{FFmpeg: ffmpeg, TmpDir: tmp, Log: log, sessions: map[string]*Session{}}
	_ = os.RemoveAll(tmp)
	_ = os.MkdirAll(tmp, 0o755)
	go m.reap()
	return m
}

func (m *Manager) reap() {
	t := time.NewTicker(15 * time.Second)
	for range t.C {
		m.mu.Lock()
		for id, s := range m.sessions {
			if time.Since(s.lastSeen) > 90*time.Second {
				m.stopLocked(id)
			}
		}
		m.mu.Unlock()
	}
}

func (m *Manager) stopLocked(id string) {
	s, ok := m.sessions[id]
	if !ok {
		return
	}
	s.cancel()
	<-s.done
	_ = os.RemoveAll(s.Dir)
	delete(m.sessions, id)
}

// Stop terminates a session.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(id)
}

// Count returns the number of active sessions.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// Find returns the running session of an owner for an item, if any.
func (m *Manager) Find(owner, itemID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		if s.ItemID == itemID && strings.HasPrefix(id, owner) {
			s.lastSeen = time.Now()
			return s
		}
	}
	return nil
}

// Get returns a session and refreshes its idle timer.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if ok {
		s.lastSeen = time.Now()
	}
	return s, ok
}

func browserVideoOK(s db.Stream) bool {
	if s.Codec != "h264" {
		return false
	}
	// 10-bit H.264 is not decodable by browsers.
	return !strings.Contains(s.PixFmt, "10") && !strings.Contains(strings.ToLower(s.Profile), "high 10")
}

func browserAudioOK(s db.Stream) bool {
	return (s.Codec == "aac" || s.Codec == "mp3") && s.Channels <= 2
}

// Start launches a new HLS transcoding session for an item.
func (m *Manager) Start(it *db.Item, opt Options, owner string) (*Session, error) {
	if m.FFmpeg == "" {
		return nil, fmt.Errorf("ffmpeg not available")
	}
	id := db.NewID()
	dir := filepath.Join(m.TmpDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var video, audio *db.Stream
	for i := range it.Media.Streams {
		s := &it.Media.Streams[i]
		switch s.Type {
		case "video":
			if video == nil {
				video = s
			}
		case "audio":
			if opt.AudioStream >= 0 && s.Index == opt.AudioStream {
				audio = s
			} else if audio == nil && opt.AudioStream < 0 && s.Default {
				audio = s
			}
		}
	}
	if audio == nil {
		for i := range it.Media.Streams {
			if it.Media.Streams[i].Type == "audio" {
				audio = &it.Media.Streams[i]
				break
			}
		}
	}

	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if opt.Start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(opt.Start, 'f', 3, 64))
	}
	segLen := opt.SegmentLength
	if segLen <= 0 {
		segLen = 4
	}
	if opt.VOD {
		opt.ForceVideo = true // segment boundaries must match the playlist
	}
	burn := opt.BurnSubtitle >= 0 && video != nil
	hw := m.HWAccel
	if burn {
		hw = "" // overlay runs on the CPU
	}
	needScale := video != nil && opt.MaxHeight > 0 && video.Height > opt.MaxHeight
	copyVideo := video != nil && browserVideoOK(*video) && !needScale && opt.MaxBitrate == 0 && !opt.ForceVideo && !video.HDR && !burn
	if !copyVideo {
		switch hw {
		case "vaapi":
			args = append(args, "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi", "-vaapi_device", "/dev/dri/renderD128")
		case "qsv":
			args = append(args, "-hwaccel", "qsv")
		case "nvenc":
			args = append(args, "-hwaccel", "cuda")
		}
	}
	args = append(args, opt.InputOptions...)
	args = append(args, "-i", InputFor(it))
	if burn {
		// Mapped later, after building the filter graph.
	} else if video != nil {
		args = append(args, "-map", fmt.Sprintf("0:%d", video.Index))
	} else {
		args = append(args, "-map", "0:v:0?")
	}
	if audio != nil {
		args = append(args, "-map", fmt.Sprintf("0:%d", audio.Index))
	} else {
		args = append(args, "-map", "0:a:0?")
	}
	if copyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		var filters []string
		enc := []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-profile:v", "high", "-pix_fmt", "yuv420p"}
		switch hw {
		case "vaapi":
			enc = []string{"-c:v", "h264_vaapi"}
		case "nvenc":
			enc = []string{"-c:v", "h264_nvenc", "-preset", "p4"}
		case "qsv":
			enc = []string{"-c:v", "h264_qsv"}
		case "videotoolbox":
			enc = []string{"-c:v", "h264_videotoolbox"}
		}
		if video != nil && video.HDR && hw == "" {
			// Simple tone mapping to SDR (requires zscale in the ffmpeg build).
			filters = append(filters, "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p")
		}
		if needScale {
			if hw == "vaapi" {
				filters = append(filters, fmt.Sprintf("scale_vaapi=w=-2:h=%d", opt.MaxHeight))
			} else {
				filters = append(filters, fmt.Sprintf("scale=-2:%d", opt.MaxHeight))
			}
		}
		if burn {
			graph := fmt.Sprintf("[0:%d][0:%d]overlay=eof_action=pass:repeatlast=0", video.Index, opt.BurnSubtitle)
			if len(filters) > 0 {
				graph += "," + strings.Join(filters, ",")
			}
			args = append(args, "-filter_complex", graph+"[vout]", "-map", "[vout]")
		} else if len(filters) > 0 {
			args = append(args, "-vf", strings.Join(filters, ","))
		}
		args = append(args, enc...)
		if opt.MaxBitrate > 0 {
			args = append(args, "-maxrate", fmt.Sprintf("%dk", opt.MaxBitrate), "-bufsize", fmt.Sprintf("%dk", opt.MaxBitrate*2))
		}
		args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", segLen))
	}
	if audio != nil && browserAudioOK(*audio) && opt.MaxBitrate == 0 {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
	}
	if opt.VOD && opt.Start > 0 {
		args = append(args, "-output_ts_offset", strconv.FormatFloat(opt.Start, 'f', 3, 64))
	}
	playlist := []string{"-hls_list_size", "0", "-hls_playlist_type", "event", "-hls_flags", "independent_segments+temp_file"}
	if opt.Live {
		playlist = []string{"-hls_list_size", "10", "-hls_flags", "independent_segments+temp_file+delete_segments", "-hls_delete_threshold", "4"}
	}
	args = append(args, "-sn", "-dn", "-max_muxing_queue_size", "2048", "-f", "hls", "-hls_time", strconv.Itoa(segLen))
	args = append(args, playlist...)
	args = append(args, "-start_number", strconv.Itoa(opt.StartNumber),
		"-hls_segment_filename", filepath.Join(dir, "seg%05d.ts"), filepath.Join(dir, "index.m3u8"))

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, m.FFmpeg, args...)
	logf, _ := os.Create(filepath.Join(dir, "ffmpeg.log"))
	cmd.Stderr = logf
	s := &Session{ID: id, ItemID: it.ID, Dir: dir, Start: opt.Start, StartNumber: opt.StartNumber, cmd: cmd, cancel: cancel, lastSeen: time.Now(), done: make(chan struct{})}
	m.Log.Info("transcode start", "item", it.Name, "start", opt.Start, "copyVideo", copyVideo, "args", strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	go func() {
		s.err = cmd.Wait()
		if logf != nil {
			logf.Close()
		}
		close(s.done)
	}()
	m.mu.Lock()
	// Only keep one session per owner+item: stop previous ones.
	for oid, prev := range m.sessions {
		if prev.ItemID == it.ID && strings.HasPrefix(oid, owner) {
			m.stopLocked(oid)
		}
	}
	s.ID = owner + id
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return s, nil
}

// WaitPlaylist waits until the playlist has at least one segment.
func (s *Session) WaitPlaylist(ctx context.Context) error {
	p := filepath.Join(s.Dir, "index.m3u8")
	for {
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), "#EXTINF") {
			return nil
		}
		select {
		case <-s.done:
			if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), "#EXTINF") {
				return nil
			}
			log, _ := os.ReadFile(filepath.Join(s.Dir, "ffmpeg.log"))
			return fmt.Errorf("ffmpeg exited: %v: %s", s.err, strings.TrimSpace(string(log)))
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Done reports whether ffmpeg has exited.
func (s *Session) Done() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// HasSegment reports whether a segment file has been written.
func (s *Session) HasSegment(name string) bool {
	_, err := os.Stat(filepath.Join(s.Dir, filepath.Base(name)))
	return err == nil
}

// LatestSegment returns the highest segment number written so far (-1 if none).
func (s *Session) LatestSegment() int {
	entries, _ := os.ReadDir(s.Dir)
	best := -1
	for _, e := range entries {
		var n int
		if _, err := fmt.Sscanf(e.Name(), "seg%05d.ts", &n); err == nil && strings.HasSuffix(e.Name(), ".ts") && n > best {
			best = n
		}
	}
	return best
}

// WaitSegment waits for a segment file to be fully written.
func (s *Session) WaitSegment(ctx context.Context, name string) (string, error) {
	p := filepath.Join(s.Dir, filepath.Base(name))
	for {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		select {
		case <-s.done:
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
			return "", os.ErrNotExist
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
