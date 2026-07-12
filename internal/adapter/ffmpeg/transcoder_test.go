package ffmpeg

import "testing"

func TestBitrateToInt(t *testing.T) {
	tests := map[string]int{
		"800k":  800000,
		"2500k": 2500000,
		"96k":   96000,
	}
	for in, want := range tests {
		if got := bitrateToInt(in); got != want {
			t.Errorf("bitrateToInt(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestFormatVTTTime(t *testing.T) {
	got := formatVTTTime(65.5)
	want := "00:01:05.500"
	if got != want {
		t.Errorf("formatVTTTime(65.5) = %q, want %q", got, want)
	}
}
