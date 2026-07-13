package domain

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

// URLBuilder generates public playback URLs on custom domains.
type URLBuilder struct {
	CDNBase         string
	PlayerBase      string
	PlayerQualities []string
}

// NewURLBuilder creates a URL builder from CDN and player base URLs.
func NewURLBuilder(cdnBase, playerBase string, playerQualities []string) *URLBuilder {
	return &URLBuilder{CDNBase: cdnBase, PlayerBase: playerBase, PlayerQualities: playerQualities}
}

func (b *URLBuilder) videoBase(videoID string) string {
	return fmt.Sprintf("%s/v/%s", trimRightSlash(b.CDNBase), videoID)
}

func (b *URLBuilder) hlsURL(videoID, profile, revision string) string {
	base := b.videoBase(videoID)
	path := playbackPath(profile, revision)
	if path == "" {
		return base + "/hls/master.m3u8"
	}
	return base + "/hls/" + escapePath(path) + "/master.m3u8"
}

func (b *URLBuilder) dashURL(videoID, profile, revision string) string {
	base := b.videoBase(videoID)
	path := playbackPath(profile, revision)
	if path == "" {
		return base + "/dash/manifest.mpd"
	}
	return base + "/dash/" + escapePath(path) + "/manifest.mpd"
}

func playbackPath(profile, revision string) string {
	if strings.TrimSpace(profile) == "" && strings.TrimSpace(revision) == "" {
		return ""
	}
	return BuildVariantPath(profile, revision)
}

func escapePath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// BuildRenditionLinks builds URLs for one profile/revision pair.
func (b *URLBuilder) BuildRenditionLinks(videoID, title, profile, revision string, qualities []string) RenditionLinks {
	return RenditionLinks{
		Profile:   profile,
		Revision:  revision,
		Status:    string(RenditionReady),
		HLS:       b.hlsURL(videoID, profile, revision),
		DASH:      b.dashURL(videoID, profile, revision),
		Qualities: qualities,
	}
}

// BuildLinks generates legacy flat playback URLs (primary / empty variant).
func (b *URLBuilder) BuildLinks(videoID, title string) PlaybackLinks {
	base := b.videoBase(videoID)
	config := base + "/config.json"
	player := fmt.Sprintf("%s/index.html?config=%s", trimRightSlash(b.PlayerBase), url.QueryEscape(config))

	return PlaybackLinks{
		VideoID:    videoID,
		Title:      title,
		HLS:        b.hlsURL(videoID, "", ""),
		DASH:       b.dashURL(videoID, "", ""),
		Config:     config,
		Player:     player,
		Thumbnail:  base + "/thumbnail.jpg",
		TooltipVTT: base + "/tooltip.vtt",
		OriginMP4:  base + "/origin.mp4",
		IFrame:     buildIFrame(player, title),
	}
}

// BuildLinksForRendition generates playback links for a specific rendition.
func (b *URLBuilder) BuildLinksForRendition(video *Video, r *Rendition) PlaybackLinks {
	links := b.BuildLinks(video.ID.String(), video.Title)
	links.HLS = b.hlsURL(video.ID.String(), r.Profile, r.Revision)
	links.DASH = b.dashURL(video.ID.String(), r.Profile, r.Revision)
	return links
}

// BuildLinksForVideo uses the stored playback variant (legacy profile/revision path).
func (b *URLBuilder) BuildLinksForVideo(video *Video) PlaybackLinks {
	links := b.BuildLinks(video.ID.String(), video.Title)
	if video.PlaybackVariant != "" {
		profile, revision := splitVariantPath(video.PlaybackVariant)
		links.HLS = b.hlsURL(video.ID.String(), profile, revision)
		links.DASH = b.dashURL(video.ID.String(), profile, revision)
	}
	return links
}

// BuildPlayerConfig creates config.json from video metadata and ready renditions.
func (b *URLBuilder) BuildPlayerConfig(video *Video, renditions []*Rendition, profiles map[string]PlaybackProfile, primaryProfile string) PlayerConfig {
	base := b.videoBase(video.ID.String())
	var desc *string
	if video.Description != "" {
		desc = &video.Description
	}

	primary := sanitizeProfileName(primaryProfile)
	if primary == "" {
		primary = "default"
	}

	cfg := PlayerConfig{
		Title:       video.Title,
		Description: desc,
		MediaID:     video.ID.String(),
		Primary:     primary,
		Behavior:    DefaultPlayerBehavior(),
		Appearance:  DefaultPlayerAppearance(),
		Poster:      base + "/thumbnail.jpg",
		Thumbnail:   base + "/tooltip.vtt",
	}

	ready := filterReadyRenditions(renditions)
	for _, r := range ready {
		qualities := r.Qualities
		if prof, ok := profiles[r.Profile]; ok && len(prof.PlayerQualities) > 0 {
			qualities = prof.PlayerQualities
		}
		cfg.Renditions = append(cfg.Renditions, RenditionSource{
			Profile:   r.Profile,
			Revision:  r.Revision,
			Qualities: qualities,
			Source: []PlayerSource{
				{Src: b.hlsURL(video.ID.String(), r.Profile, r.Revision), Type: "application/x-mpegURL"},
				{Src: b.dashURL(video.ID.String(), r.Profile, r.Revision), Type: "application/dash+xml"},
			},
			Poster:    base + "/thumbnail.jpg",
			Thumbnail: base + "/tooltip.vtt",
		})
	}

	primaryRendition := pickPrimaryRendition(ready, primary)
	if primaryRendition != nil {
		qualities := primaryRendition.Qualities
		if prof, ok := profiles[primaryRendition.Profile]; ok && len(prof.PlayerQualities) > 0 {
			qualities = prof.PlayerQualities
		} else if len(qualities) == 0 {
			qualities = b.PlayerQualities
		}
		cfg.Source = []PlayerSource{
			{Src: b.hlsURL(video.ID.String(), primaryRendition.Profile, primaryRendition.Revision), Type: "application/x-mpegURL"},
			{Src: b.dashURL(video.ID.String(), primaryRendition.Profile, primaryRendition.Revision), Type: "application/dash+xml"},
		}
		cfg.Qualities = qualities
	} else {
		links := b.BuildLinksForVideo(video)
		cfg.Source = []PlayerSource{
			{Src: links.HLS, Type: "application/x-mpegURL"},
			{Src: links.DASH, Type: "application/dash+xml"},
		}
		cfg.Qualities = b.PlayerQualities
	}

	return cfg
}

func filterReadyRenditions(renditions []*Rendition) []*Rendition {
	var out []*Rendition
	for _, r := range renditions {
		if r != nil && r.Status == RenditionReady {
			out = append(out, r)
		}
	}
	return out
}

func pickPrimaryRendition(renditions []*Rendition, primary string) *Rendition {
	for _, r := range renditions {
		if r.Profile == primary {
			return r
		}
	}
	if len(renditions) > 0 {
		return renditions[0]
	}
	return nil
}

func splitVariantPath(variant string) (profile, revision string) {
	variant = strings.TrimSpace(variant)
	if variant == "" {
		return "", ""
	}
	parts := strings.SplitN(variant, "/", 2)
	if len(parts) == 1 {
		return "default", parts[0]
	}
	return parts[0], parts[1]
}

// StorageKeys returns object storage keys for a video.
func StorageKeys(videoID string) struct {
	Prefix, Origin, Config, Thumbnail, TooltipVTT, TooltipPNG, HLSMaster, DASHManifest string
} {
	return StorageKeysWithVariant(videoID, "")
}

func StorageKeysWithVariant(videoID, variant string) struct {
	Prefix, Origin, Config, Thumbnail, TooltipVTT, TooltipPNG, HLSMaster, DASHManifest string
} {
	prefix := fmt.Sprintf("v/%s", videoID)
	v := strings.TrimSpace(variant)
	hlsMaster := prefix + "/hls/master.m3u8"
	dashManifest := prefix + "/dash/manifest.mpd"
	if v != "" {
		hlsMaster = prefix + "/hls/" + v + "/master.m3u8"
		dashManifest = prefix + "/dash/" + v + "/manifest.mpd"
	}
	return struct {
		Prefix, Origin, Config, Thumbnail, TooltipVTT, TooltipPNG, HLSMaster, DASHManifest string
	}{
		Prefix:       prefix,
		Origin:       prefix + "/origin.mp4",
		Config:       prefix + "/config.json",
		Thumbnail:    prefix + "/thumbnail.jpg",
		TooltipVTT:   prefix + "/tooltip.vtt",
		TooltipPNG:   prefix + "/tooltip.png",
		HLSMaster:    hlsMaster,
		DASHManifest: dashManifest,
	}
}

func buildIFrame(playerURL, title string) string {
	escapedTitle := html.EscapeString(title)
	return fmt.Sprintf(
		`<style>.cf_embed{position:relative;overflow:hidden;width:100%%;height:auto;padding-top:56.25%%}`+
			`.cf_embed iframe{position:absolute;top:0;left:0;width:100%%;height:100%%;border:0}</style>`+
			`<div class="cf_embed"><iframe src="%s" title="%s" frameborder="0" `+
			`allow="accelerometer;autoplay;encrypted-media;gyroscope;picture-in-picture" `+
			`allowfullscreen></iframe></div>`,
		playerURL, escapedTitle,
	)
}

func trimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
