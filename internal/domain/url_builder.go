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

func (b *URLBuilder) hlsURL(videoID, variant string) string {
	base := b.videoBase(videoID)
	if strings.TrimSpace(variant) == "" {
		return base + "/hls/master.m3u8"
	}
	return base + "/hls/" + url.PathEscape(variant) + "/master.m3u8"
}

func (b *URLBuilder) dashURL(videoID, variant string) string {
	base := b.videoBase(videoID)
	if strings.TrimSpace(variant) == "" {
		return base + "/dash/manifest.mpd"
	}
	return base + "/dash/" + url.PathEscape(variant) + "/manifest.mpd"
}

// BuildLinks generates all playback URLs for a video.
func (b *URLBuilder) BuildLinks(videoID, title string) PlaybackLinks {
	base := b.videoBase(videoID)
	config := base + "/config.json"
	player := fmt.Sprintf("%s/index.html?config=%s", trimRightSlash(b.PlayerBase), url.QueryEscape(config))

	return PlaybackLinks{
		VideoID:    videoID,
		Title:      title,
		HLS:        b.hlsURL(videoID, ""),
		DASH:       b.dashURL(videoID, ""),
		Config:     config,
		Player:     player,
		Thumbnail:  base + "/thumbnail.jpg",
		TooltipVTT: base + "/tooltip.vtt",
		OriginMP4:  base + "/origin.mp4",
		IFrame:     buildIFrame(player, title),
	}
}

// BuildLinksForVideo generates playback URLs using the video's stored playback variant.
func (b *URLBuilder) BuildLinksForVideo(video *Video) PlaybackLinks {
	links := b.BuildLinks(video.ID.String(), video.Title)
	links.HLS = b.hlsURL(video.ID.String(), video.PlaybackVariant)
	links.DASH = b.dashURL(video.ID.String(), video.PlaybackVariant)
	return links
}

// BuildPlayerConfig creates the JSON config consumed by the player page.
func (b *URLBuilder) BuildPlayerConfig(video *Video) PlayerConfig {
	links := b.BuildLinksForVideo(video)
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
		Qualities: b.PlayerQualities,
	}
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
