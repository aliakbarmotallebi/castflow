package domain

// PlaybackLinks holds all public URLs for a ready video asset.
type PlaybackLinks struct {
	VideoID    string `json:"videoId"`
	Title      string `json:"title"`
	HLS        string `json:"hlsUrl"`
	DASH       string `json:"dashUrl"`
	Config     string `json:"configUrl"`
	Player     string `json:"playerUrl"`
	Thumbnail  string `json:"thumbnailUrl"`
	TooltipVTT string `json:"tooltipUrl"`
	OriginMP4  string `json:"videoUrl"`
	IFrame     string `json:"iframe"`
}

// PlayerConfig is served at config.json for the embedded player.
type PlayerConfig struct {
	Title       string            `json:"title"`
	Description *string           `json:"description"`
	MediaID     string            `json:"mediaid"`
	Primary     string            `json:"primary,omitempty"`
	Behavior    PlayerBehavior    `json:"behavior"`
	Appearance  PlayerAppearance  `json:"appearance"`
	Source      []PlayerSource    `json:"source"`
	Renditions  []RenditionSource `json:"renditions,omitempty"`
	Poster      string            `json:"poster"`
	Thumbnail   string            `json:"thumbnail"`
	Qualities   []string          `json:"qualities,omitempty"`
}

type PlayerBehavior struct {
	Type      string `json:"type"`
	Mode      string `json:"mode"`
	Preload   string `json:"preload"`
	Autostart bool   `json:"autostart"`
	Repeat    bool   `json:"repeat"`
	Mute      bool   `json:"mute"`
}

type PlayerAppearance struct {
	Lang                string  `json:"lang"`
	Controls            bool    `json:"controls"`
	AspectRatio         *string `json:"aspectratio"`
	TouchNativeControls bool    `json:"touchnativecontrols"`
	DisplayTitle        bool    `json:"displaytitle"`
	DisplayDescription  bool    `json:"displaydescription"`
}

type PlayerSource struct {
	Src  string `json:"src"`
	Type string `json:"type"`
}

// DefaultPlayerBehavior returns sensible player defaults.
func DefaultPlayerBehavior() PlayerBehavior {
	return PlayerBehavior{
		Type:      "video",
		Mode:      "static",
		Preload:   "auto",
		Autostart: false,
		Repeat:    false,
		Mute:      false,
	}
}

// DefaultPlayerAppearance returns sensible appearance defaults.
func DefaultPlayerAppearance() PlayerAppearance {
	return PlayerAppearance{
		Lang:                "fa",
		Controls:            true,
		TouchNativeControls: false,
		DisplayTitle:        true,
		DisplayDescription:  false,
	}
}
