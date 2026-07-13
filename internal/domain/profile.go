package domain

// PlaybackProfile defines one named transcode ladder (default, mobile, cinema, …).
type PlaybackProfile struct {
	Name            string
	Qualities       []QualityProfile
	PlayerQualities []string
}

// RevisionInput captures settings that affect a rendition revision hash.
type RevisionInput struct {
	Profile         string
	Qualities       []QualityProfile
	HLSSegmentSec   int
	ThumbnailAtSec  float64
	TooltipInterval float64
}
