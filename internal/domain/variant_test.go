package domain

import "testing"

func TestBuildRevision_stable(t *testing.T) {
	in := RevisionInput{
		Profile: "default",
		Qualities: []QualityProfile{
			{Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2500k", AudioBitrate: "128k"},
			{Name: "360p", Width: 640, Height: 360, VideoBitrate: "800k", AudioBitrate: "96k"},
		},
		HLSSegmentSec: 6,
	}
	a := BuildRevision(in)
	b := BuildRevision(in)
	if a != b || len(a) != 8 {
		t.Fatalf("revision unstable: %q %q", a, b)
	}
}

func TestBuildRevision_bitrateChange(t *testing.T) {
	base := RevisionInput{
		Profile: "default",
		Qualities: []QualityProfile{
			{Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2500k", AudioBitrate: "128k"},
		},
		HLSSegmentSec: 6,
	}
	changed := RevisionInput{
		Profile: base.Profile,
		Qualities: []QualityProfile{
			{Name: "720p", Width: 1280, Height: 720, VideoBitrate: "3000k", AudioBitrate: "128k"},
		},
		HLSSegmentSec: base.HLSSegmentSec,
	}
	if BuildRevision(base) == BuildRevision(changed) {
		t.Fatal("bitrate change should change revision")
	}
}

func TestBuildVariantPath(t *testing.T) {
	if got := BuildVariantPath("default", "a3f2b1c4"); got != "default/a3f2b1c4" {
		t.Fatalf("path: %s", got)
	}
}

func TestReadableLadder(t *testing.T) {
	got := ReadableLadder([]QualityProfile{
		{Name: "1080p", Height: 1080},
		{Name: "360p", Height: 360},
	})
	if got != "360-1080" {
		t.Fatalf("ladder: %s", got)
	}
}
