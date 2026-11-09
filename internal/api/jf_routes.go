package api

import "net/http"

func (s *Server) jellyfinRouter() *jfRouter {
	jr := &jfRouter{s: s}
	pub := func(method, p string, h handler) { jr.add(method, p, false, h) }

	// System & authentication
	pub("GET", "/System/Info/Public", s.jfPublicInfo)
	pub("GET", "/System/Ping", s.jfPing)
	pub("GET", "/socket", s.jfSocket)
	pub("POST", "/System/Ping", s.jfPing)
	pub("GET", "/Users/Public", s.jfPublicUsers)
	pub("POST", "/Users/AuthenticateByName", s.jfAuthenticate)
	pub("GET", "/Branding/Configuration", s.jfEmpty(map[string]any{"LoginDisclaimer": "", "CustomCss": "", "SplashscreenEnabled": false}))
	pub("GET", "/Branding/Css", s.jfNoContent)
	pub("GET", "/Branding/Css.css", s.jfNoContent)
	pub("GET", "/QuickConnect/Enabled", s.jfEmpty(false))
	pub("GET", "/Startup/Configuration", s.jfEmpty(map[string]any{"UICulture": "en-US", "MetadataCountryCode": "US", "PreferredMetadataLanguage": "en"}))
	jr.get("/System/Info", s.jfSystemInfo)
	jr.get("/System/Endpoint", s.jfEndpoint)
	jr.get("/Users/Me", s.jfMe)
	jr.get("/Users/{userId}", s.jfUserByID)
	jr.post("/Sessions/Logout", s.jfLogout)
	jr.get("/Sessions", s.jfSessions)
	jr.post("/Sessions/Capabilities", s.jfNoContent)
	jr.post("/Sessions/Capabilities/Full", s.jfNoContent)
	jr.get("/Localization/Cultures", s.jfEmpty([]any{}))
	jr.get("/Localization/Countries", s.jfEmpty([]any{}))
	jr.get("/Localization/ParentalRatings", s.jfEmpty([]any{}))
	jr.get("/Localization/Options", s.jfEmpty([]any{}))
	jr.get("/Plugins", s.jfEmpty([]any{}))
	jr.get("/Packages", s.jfEmpty([]any{}))
	jr.get("/ScheduledTasks", s.jfEmpty([]any{}))
	jr.get("/Devices", s.jfEmpty(map[string]any{"Items": []any{}, "TotalRecordCount": 0}))
	jr.post("/ClientLog/Document", s.jfEmpty(map[string]any{"FileName": "log.txt"}))
	jr.get("/DisplayPreferences/{id}", s.jfDisplayPrefs)
	jr.post("/DisplayPreferences/{id}", s.jfDisplayPrefs)

	// Browsing (both 10.9+ and legacy /Users/{userId} routes)
	for _, pre := range []string{"", "/Users/{userId}"} {
		jr.get(pre+"/Items", s.jfQueryItems)
		jr.get(pre+"/Items/{itemId}", s.jfGetItem)
		jr.get(pre+"/Items/Latest", s.jfLatest)
		jr.get(pre+"/Items/Resume", s.jfResume)
	}
	jr.get("/UserItems/Resume", s.jfResume)
	jr.get("/UserViews", s.jfViews)
	jr.get("/Users/{userId}/Views", s.jfViews)
	jr.get("/UserViews/GroupingOptions", s.jfEmpty([]any{}))
	jr.get("/Shows/NextUp", s.jfNextUp)
	jr.get("/Shows/{seriesId}/Seasons", s.jfSeasons)
	jr.get("/Shows/{seriesId}/Episodes", s.jfEpisodes)
	jr.get("/Items/{itemId}/Similar", s.jfSimilar)
	jr.get("/Movies/{itemId}/Similar", s.jfSimilar)
	jr.get("/Shows/{itemId}/Similar", s.jfSimilar)
	jr.get("/Items/{itemId}/ThemeMedia", s.jfEmpty(map[string]any{
		"ThemeVideosResult":     map[string]any{"Items": []any{}, "TotalRecordCount": 0},
		"ThemeSongsResult":      map[string]any{"Items": []any{}, "TotalRecordCount": 0},
		"SoundtrackSongsResult": map[string]any{"Items": []any{}, "TotalRecordCount": 0},
	}))
	jr.get("/Items/{itemId}/SpecialFeatures", s.jfEmpty([]any{}))
	jr.get("/Users/{userId}/Items/{itemId}/SpecialFeatures", s.jfEmpty([]any{}))
	jr.get("/Items/{itemId}/LocalTrailers", s.jfEmpty([]any{}))
	jr.get("/Users/{userId}/Items/{itemId}/LocalTrailers", s.jfEmpty([]any{}))
	jr.get("/Items/{itemId}/Intros", s.jfEmpty(map[string]any{"Items": []any{}, "TotalRecordCount": 0}))
	jr.get("/Users/{userId}/Items/{itemId}/Intros", s.jfEmpty(map[string]any{"Items": []any{}, "TotalRecordCount": 0}))
	jr.get("/MediaSegments/{itemId}", s.jfEmpty(map[string]any{"Items": []any{}, "TotalRecordCount": 0}))
	jr.get("/Search/Hints", s.jfSearchHints)
	jr.get("/Genres", s.jfGenres)
	jr.get("/MusicGenres", s.jfGenres)
	jr.get("/Items/Filters", s.jfEmpty(map[string]any{"Genres": []any{}, "Tags": []any{}, "OfficialRatings": []any{}, "Years": []any{}}))
	jr.get("/Items/Filters2", s.jfEmpty(map[string]any{"Genres": []any{}, "Tags": []any{}}))
	pub("GET", "/Items/{itemId}/Images/{type}", s.jfImage)
	pub("GET", "/Items/{itemId}/Images/{type}/{index}", s.jfImage)
	pub("HEAD", "/Items/{itemId}/Images/{type}", s.jfImage)

	// User data
	for _, pre := range []string{"", "/Users/{userId}"} {
		p := "/UserPlayedItems/{itemId}"
		f := "/UserFavoriteItems/{itemId}"
		if pre != "" {
			p, f = pre+"/PlayedItems/{itemId}", pre+"/FavoriteItems/{itemId}"
		}
		jr.post(p, s.jfMarkPlayed(true))
		jr.del(p, s.jfMarkPlayed(false))
		jr.post(f, s.jfMarkFavorite(true))
		jr.del(f, s.jfMarkFavorite(false))
	}
	jr.get("/UserItems/{itemId}/UserData", s.jfItemUserData)

	// Playback
	jr.get("/Items/{itemId}/PlaybackInfo", s.jfPlaybackInfo)
	jr.post("/Items/{itemId}/PlaybackInfo", s.jfPlaybackInfo)
	for _, p := range []string{"/Videos/{itemId}/stream", "/Videos/{itemId}/stream.{ext}", "/Audio/{itemId}/stream", "/Audio/{itemId}/stream.{ext}", "/Items/{itemId}/Download"} {
		jr.get(p, s.jfStream)
	}
	jr.get("/Audio/{itemId}/universal", s.jfUniversalAudio)
	jr.get("/Videos/{itemId}/master.m3u8", s.jfMasterPlaylist)
	jr.get("/Videos/{itemId}/main.m3u8", s.jfMainPlaylist)
	jr.get("/Videos/{itemId}/hls1/{playlistId}/{segmentId}", s.jfSegment)
	jr.get("/Videos/{itemId}/{mediaSourceId}/Subtitles/{index}/{file}", s.jfSubtitle)
	jr.get("/Videos/{itemId}/{mediaSourceId}/Subtitles/{index}/{ticks}/{file}", s.jfSubtitle)
	jr.post("/Sessions/Playing", s.jfPlaying("start"))
	jr.post("/Sessions/Playing/Progress", s.jfPlaying("progress"))
	jr.post("/Sessions/Playing/Ping", s.jfNoContent)
	jr.post("/Sessions/Playing/Stopped", s.jfPlaying("stop"))
	jr.post("/Users/{userId}/PlayingItems/{itemId}", s.jfPlaying("start"))
	jr.post("/Users/{userId}/PlayingItems/{itemId}/Progress", s.jfPlaying("progress"))
	jr.del("/Users/{userId}/PlayingItems/{itemId}", s.jfPlaying("stop"))
	jr.post("/PlayingItems/{itemId}", s.jfPlaying("start"))
	jr.post("/PlayingItems/{itemId}/Progress", s.jfPlaying("progress"))
	jr.del("/PlayingItems/{itemId}", s.jfPlaying("stop"))
	// Live TV
	jr.get("/LiveTv/Info", s.jfLiveInfo)
	jr.get("/LiveTv/Channels", s.jfLiveChannels)
	jr.get("/LiveTv/Channels/{channelId}", s.jfLiveChannel)
	jr.get("/LiveTv/Programs", s.jfLivePrograms)
	jr.post("/LiveTv/Programs", s.jfLivePrograms)
	jr.get("/LiveTv/Programs/Recommended", s.jfLivePrograms)
	jr.get("/LiveTv/Recordings", s.jfEmpty(map[string]any{"Items": []any{}, "TotalRecordCount": 0}))
	jr.get("/LiveTv/Timers", s.jfEmpty(map[string]any{"Items": []any{}, "TotalRecordCount": 0}))
	jr.get("/LiveTv/Channels/{channelId}/live.m3u8", s.jfLivePlaylist)
	jr.get("/LiveTv/Channels/{channelId}/live/{file}", s.jfLiveSegment)
	return jr
}

// withJellyfin serves Jellyfin-compatible routes before the web client.
func (s *Server) withJellyfin(web http.HandlerFunc) http.HandlerFunc {
	jr := s.jellyfinRouter()
	return func(w http.ResponseWriter, r *http.Request) {
		if jr.serve(w, r) {
			return
		}
		web(w, r)
	}
}
