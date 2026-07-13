package config

import "testing"

func TestLoadPlaybackProfiles_defaultAndMobile(t *testing.T) {
	t.Setenv("CASTFLOW_PLAYBACK_PROFILE", "default")
	t.Setenv("CASTFLOW_PLAYBACK_PROFILES", "default,mobile")
	t.Setenv("CASTFLOW_PROFILE_MOBILE_QUALITIES", "144p,360p")

	cfg := loadPlaybackConfig(defaultQualities())
	if cfg.PrimaryProfile != "default" {
		t.Fatalf("primary: %s", cfg.PrimaryProfile)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("profiles: %d", len(cfg.Profiles))
	}
	mobile := cfg.ProfileMap["mobile"]
	if len(mobile.Qualities) != 2 || mobile.Qualities[0].Name != "144p" {
		t.Fatalf("mobile qualities: %+v", mobile.Qualities)
	}
}

func TestPlaybackConfig_ResolveProfiles(t *testing.T) {
	cfg := loadPlaybackConfig(defaultQualities())
	subset := cfg.ResolveProfiles([]string{"mobile"})
	if len(subset) != 1 || subset[0].Name != "mobile" {
		t.Fatalf("subset: %+v", subset)
	}
}
