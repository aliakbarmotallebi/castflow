package domain

import (
	"fmt"
	"html"
	"net/url"
)

// URLBuilder generates public playback URLs on custom domains.
type URLBuilder struct {
	CDNBase    string
	PlayerBase string
}

// NewURLBuilder creates a URL builder from CDN and player base URLs.
func NewURLBuilder(cdnBase, playerBase string) *URLBuilder {
	return &URLBuilder{CDNBase: cdnBase, PlayerBase: playerBase}
}

func (b *URLBuilder) videoBase(videoID string) string {
	return fmt.Sprintf("%s/v/%s", trimRightSlash(b.CDNBase), videoID)
}

// BuildLinks generates all playback URLs for a video.
func (b *URLBuilder) BuildLinks(videoID, title string) PlaybackLinks {
	base := b.videoBase(videoID)
	config := base + "/config.json"
	player := fmt.Sprintf("%s/index.html?config=%s", trimRightSlash(b.PlayerBase), url.QueryEscape(config))

	return PlaybackLinks{
		VideoID:    videoID,
		Title:      title,
		HLS:        base + "/hls/master.m3u8",
		DASH:       base + "/dash/manifest.mpd",
		Config:     config,
		Player:     player,
		Thumbnail:  base + "/thumbnail.jpg",
		TooltipVTT: base + "/tooltip.vtt",
		OriginMP4:  base + "/origin.mp4",
		IFrame:     buildIFrame(player, title),
	}
}

// BuildPlayerConfig creates the JSON config consumed by the player page.
func (b *URLBuilder) BuildPlayerConfig(video *Video) PlayerConfig {
	links := b.BuildLinks(video.ID.String(), video.Title)
	var desc *string
	if video.Description != "" {
		desc = &video.Description
	}
	return PlayerConfig{
		Title:       video.Title,
		Description: desc,
		MediaID:     video.ID.String(),
		Behavior:    DefaultPlayerBehavior(),
		Appearance:  DefaultPlayerAppearance(),
		Source: []PlayerSource{
			{Src: links.HLS, Type: "application/x-mpegURL"},
			{Src: links.DASH, Type: "application/dash+xml"},
		},
		Poster:    links.Thumbnail,
		Thumbnail: links.TooltipVTT,
	}
}

// StorageKeys returns object storage keys for a video.
func StorageKeys(videoID string) struct {
	Prefix, Origin, Config, Thumbnail, TooltipVTT, TooltipPNG, HLSMaster, DASHManifest string
} {
	prefix := fmt.Sprintf("v/%s", videoID)
	return struct {
		Prefix, Origin, Config, Thumbnail, TooltipVTT, TooltipPNG, HLSMaster, DASHManifest string
	}{
		Prefix:       prefix,
		Origin:       prefix + "/origin.mp4",
		Config:       prefix + "/config.json",
		Thumbnail:    prefix + "/thumbnail.jpg",
		TooltipVTT:   prefix + "/tooltip.vtt",
		TooltipPNG:   prefix + "/tooltip.png",
		HLSMaster:    prefix + "/hls/master.m3u8",
		DASHManifest: prefix + "/dash/manifest.mpd",
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
