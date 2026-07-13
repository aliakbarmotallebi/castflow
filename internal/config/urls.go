package config

import "strings"

// resolvePublicURLs builds API/CDN/Player URLs from CASTFLOW_BASE_URL.
// Optional overrides: CASTFLOW_API_BASE_URL, CASTFLOW_CDN_BASE_URL, CASTFLOW_PLAYER_BASE_URL.
func resolvePublicURLs() (api, cdn, player string) {
	base := trimRightSlash(env("CASTFLOW_BASE_URL", "http://localhost:8080"))

	api = trimRightSlash(env("CASTFLOW_API_BASE_URL", ""))
	if api == "" {
		api = base
	}

	cdn = trimRightSlash(env("CASTFLOW_CDN_BASE_URL", ""))
	if cdn == "" {
		cdn = base + "/media"
	}

	player = trimRightSlash(env("CASTFLOW_PLAYER_BASE_URL", ""))
	if player == "" {
		player = base + "/player"
	}
	return api, cdn, player
}

func trimRightSlash(s string) string {
	return strings.TrimRight(s, "/")
}
