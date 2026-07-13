package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// BuildRevision returns a short hash of the effective transcode settings for a profile.
func BuildRevision(in RevisionInput) string {
	q := append([]QualityProfile(nil), in.Qualities...)
	sort.Slice(q, func(i, j int) bool { return q[i].Height < q[j].Height })

	payload := struct {
		Profile         string           `json:"profile"`
		Qualities       []QualityProfile `json:"qualities"`
		HLSSegmentSec   int              `json:"hlsSegmentSec"`
		ThumbnailAtSec  float64          `json:"thumbnailAtSec"`
		TooltipInterval float64          `json:"tooltipIntervalSec"`
	}{
		Profile:         strings.TrimSpace(in.Profile),
		Qualities:       q,
		HLSSegmentSec:   in.HLSSegmentSec,
		ThumbnailAtSec:  in.ThumbnailAtSec,
		TooltipInterval: in.TooltipInterval,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:8]
}

// BuildVariantPath joins profile and revision for storage/URL paths.
func BuildVariantPath(profile, revision string) string {
	profile = sanitizeProfileName(profile)
	revision = strings.TrimSpace(revision)
	if profile == "" {
		return revision
	}
	if revision == "" {
		return profile
	}
	return profile + "/" + revision
}

// ReadableLadder returns resolution labels for logging/debug (e.g. 360-720-1080).
func ReadableLadder(qualities []QualityProfile) string {
	q := append([]QualityProfile(nil), qualities...)
	sort.Slice(q, func(i, j int) bool { return q[i].Height < q[j].Height })
	parts := make([]string, 0, len(q))
	for _, it := range q {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			name = fmt.Sprintf("%dp", it.Height)
		}
		parts = append(parts, strings.TrimSuffix(name, "p"))
	}
	return strings.Join(parts, "-")
}

func sanitizeProfileName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "default"
	}
	return out
}
