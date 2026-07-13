package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

var bitrateRe = regexp.MustCompile(`(?i)^(\d+)\s*k$`)

// BuildPlaybackVariant builds a readable, cache-friendly identifier for a transcode ladder.
// Example: h_,360_800,720_2500,1080_5000,k__6fa82d1a
func BuildPlaybackVariant(qualities []QualityProfile) string {
	if len(qualities) == 0 {
		return ""
	}

	// Normalize order (stable across config ordering changes).
	q := make([]QualityProfile, 0, len(qualities))
	q = append(q, qualities...)
	sort.Slice(q, func(i, j int) bool { return q[i].Height < q[j].Height })

	parts := make([]string, 0, len(q))
	for _, it := range q {
		br := normalizeK(it.VideoBitrate)
		if br == "" {
			// Fall back to "height" only if bitrate is missing/unparseable.
			parts = append(parts, strings.TrimSpace(it.Name))
			continue
		}
		parts = append(parts, strings.TrimSuffix(strings.TrimSpace(it.Name), "p")+"_"+br)
	}

	// Hash the full normalized quality list to avoid collisions and to ensure variant changes
	// when any effective transcode setting changes (bitrate/size/name).
	b, _ := json.Marshal(q)
	sum := sha256.Sum256(b)
	hash8 := hex.EncodeToString(sum[:])[:8]

	return "h_," + strings.Join(parts, ",") + ",k__" + hash8
}

func normalizeK(s string) string {
	s = strings.TrimSpace(s)
	m := bitrateRe.FindStringSubmatch(s)
	if len(m) != 2 {
		return ""
	}
	return m[1]
}
