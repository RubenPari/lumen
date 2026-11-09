package stream

import (
	"strings"
	"testing"
)

func TestSRTToVTT(t *testing.T) {
	in := "1\r\n00:00:01,5 --> 00:00:04,250\r\nHello <i>world</i>\r\nSecond line\r\n\r\n2\r\n01:02:03,000 --> 01:02:04,000\r\n{\\an8}Top\r\n"
	got := SRTToVTT(in)
	for _, want := range []string{"WEBVTT", "00:00:01.500 --> 00:00:04.250", "Hello <i>world</i>\nSecond line", "01:02:03.000 --> 01:02:04.000\nTop"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestASSToVTT(t *testing.T) {
	in := `[Script Info]
Title: x

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:02.50,Default,,0,0,0,,{\b1}Hi{\b0}, there\NFriend
`
	got := ASSToVTT(in)
	if !strings.Contains(got, "00:00:01.000 --> 00:00:02.500\nHi, there\nFriend") {
		t.Errorf("unexpected:\n%s", got)
	}
}

func TestDecodeLatin1(t *testing.T) {
	if got := decodeText([]byte{'c', 'a', 'f', 0xe9}); got != "café" {
		t.Errorf("got %q", got)
	}
}
