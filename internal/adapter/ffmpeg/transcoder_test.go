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

func TestTooltipInterval(t *testing.T) {
	tests := []struct {
		duration float64
		interval float64
		max      int
		want     float64
	}{
		{120, 5, 60, 5},
		{600, 5, 60, 10},
		{0, 5, 60, 5},
		{300, 30, 60, 30},
	}
	for _, tc := range tests {
		got := tooltipInterval(tc.duration, tc.interval, tc.max)
		if got != tc.want {
			t.Errorf("tooltipInterval(%v, %v, %d) = %v, want %v", tc.duration, tc.interval, tc.max, got, tc.want)
		}
	}
}

func TestTooltipGrid(t *testing.T) {
	tests := []struct {
		frames int
		cols   int
		wantC  int
		wantR  int
	}{
		{24, 10, 10, 3},
		{5, 10, 5, 1},
		{60, 10, 10, 6},
	}
	for _, tc := range tests {
		cols, rows := tooltipGrid(tc.frames, tc.cols)
		if cols != tc.wantC || rows != tc.wantR {
			t.Errorf("tooltipGrid(%d, %d) = (%d, %d), want (%d, %d)", tc.frames, tc.cols, cols, rows, tc.wantC, tc.wantR)
		}
	}
}
