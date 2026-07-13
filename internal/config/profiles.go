package config

import (
	"fmt"
	"strings"

	"castflow/internal/domain"
)

// PlaybackConfig defines named playback profiles and webhook settings.
type PlaybackConfig struct {
	PrimaryProfile string
	Profiles       []domain.PlaybackProfile
	ProfileMap     map[string]domain.PlaybackProfile
	WebhookURL     string
	WebhookSecret  string
}

func loadPlaybackConfig(defaultQualities []domain.QualityProfile) PlaybackConfig {
	primary := domainSanitizeProfile(env("CASTFLOW_PLAYBACK_PROFILE", "default"))

	profileNames := parseProfileNames(env("CASTFLOW_PLAYBACK_PROFILES", ""), primary)
	presets := qualityPresets()

	profiles := make([]domain.PlaybackProfile, 0, len(profileNames))
	profileMap := make(map[string]domain.PlaybackProfile, len(profileNames))

	for _, name := range profileNames {
		name = domainSanitizeProfile(name)
		qualities := qualitiesForProfile(name, presets, defaultQualities)
		playerQualities := playerQualitiesForProfile(name, qualities)
		p := domain.PlaybackProfile{
			Name:            name,
			Qualities:       qualities,
			PlayerQualities: playerQualities,
		}
		profiles = append(profiles, p)
		profileMap[name] = p
	}

	return PlaybackConfig{
		PrimaryProfile: primary,
		Profiles:       profiles,
		ProfileMap:     profileMap,
		WebhookURL:     env("CASTFLOW_WEBHOOK_URL", ""),
		WebhookSecret:  env("CASTFLOW_WEBHOOK_SECRET", ""),
	}
}

func parseProfileNames(raw, primary string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{primary}
	}
	seen := map[string]bool{}
	var out []string
	for _, name := range strings.Split(raw, ",") {
		name = domainSanitizeProfile(strings.TrimSpace(name))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return []string{primary}
	}
	return out
}

func qualitiesForProfile(name string, presets map[string]domain.QualityProfile, fallback []domain.QualityProfile) []domain.QualityProfile {
	key := fmt.Sprintf("CASTFLOW_PROFILE_%s_QUALITIES", strings.ToUpper(strings.ReplaceAll(name, "-", "_")))
	raw := env(key, "")
	if raw == "" {
		switch name {
		case "mobile":
			raw = "144p,240p,360p,480p"
		case "default":
			if len(fallback) > 0 {
				return append([]domain.QualityProfile(nil), fallback...)
			}
			raw = "360p,720p,1080p"
		default:
			if len(fallback) > 0 {
				return append([]domain.QualityProfile(nil), fallback...)
			}
			raw = "360p,720p,1080p"
		}
	}
	return parseQualityList(raw, presets)
}

func playerQualitiesForProfile(name string, transcoded []domain.QualityProfile) []string {
	key := fmt.Sprintf("CASTFLOW_PROFILE_%s_PLAYER_QUALITIES", strings.ToUpper(strings.ReplaceAll(name, "-", "_")))
	raw := env(key, "")
	if raw == "" {
		out := make([]string, len(transcoded))
		for i, q := range transcoded {
			out[i] = q.Name
		}
		return out
	}
	return parsePlayerQualityList(raw, transcoded)
}

func parseQualityList(raw string, presets map[string]domain.QualityProfile) []domain.QualityProfile {
	var out []domain.QualityProfile
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if q, ok := presets[name]; ok {
			out = append(out, q)
		}
	}
	return out
}

func parsePlayerQualityList(raw string, transcoded []domain.QualityProfile) []string {
	names := qualityPresetNames()
	var out []string
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if names[name] {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		out = make([]string, len(transcoded))
		for i, q := range transcoded {
			out[i] = q.Name
		}
	}
	return out
}

func qualityPresets() map[string]domain.QualityProfile {
	return map[string]domain.QualityProfile{
		"144p":  {Name: "144p", Width: 256, Height: 144, VideoBitrate: "200k", AudioBitrate: "64k"},
		"240p":  {Name: "240p", Width: 426, Height: 240, VideoBitrate: "400k", AudioBitrate: "64k"},
		"360p":  {Name: "360p", Width: 640, Height: 360, VideoBitrate: "800k", AudioBitrate: "96k"},
		"480p":  {Name: "480p", Width: 854, Height: 480, VideoBitrate: "1200k", AudioBitrate: "128k"},
		"720p":  {Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2500k", AudioBitrate: "128k"},
		"1080p": {Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: "5000k", AudioBitrate: "128k"},
	}
}

func domainSanitizeProfile(s string) string {
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

func (c *PlaybackConfig) ProfileNames() []string {
	names := make([]string, len(c.Profiles))
	for i, p := range c.Profiles {
		names[i] = p.Name
	}
	return names
}

func (c *PlaybackConfig) ResolveProfiles(requested []string) []domain.PlaybackProfile {
	if len(requested) == 0 {
		return c.Profiles
	}
	var out []domain.PlaybackProfile
	for _, name := range requested {
		name = domainSanitizeProfile(name)
		if p, ok := c.ProfileMap[name]; ok {
			out = append(out, p)
		}
	}
	return out
}
