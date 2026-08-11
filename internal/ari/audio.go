package ari

import (
	"context"
	"fmt"
)

// audioNames maps internal prompt keys to the WAV base names (same contract
// as ami.AudioMap; the files ship in audios/).
var audioNames = map[string]string{
	"rating_request":   "ambulance-rating-request",
	"rating_thankyou":  "ambulance-rating-thankyou",
	"rating_invalid":   "ambulance-rating-invalid",
	"transfer_message": "ambulance-transfer-message",
	"transfer_error":   "ambulance-transfer-error",
	"failed_rating":    "ambulance-failed-rating",
}

// audioMode is resolved once per call — the "ladder":
//  1. http:   res_http_media_cache loaded + AUDIO_MEDIA_BASE_URL set →
//     Asterisk fetches prompts from the app over HTTP (admin uploads are
//     live instantly, nothing stored on the PBX).
//  2. sound:  prompts exist in the PBX sounds dir (setup's SSH copy).
//  3. custom: prompts uploaded via FreePBX GUI (System Recordings) land in
//     custom/.
type audioMode string

const (
	modeHTTP   audioMode = "http"
	modeSound  audioMode = "sound"
	modeCustom audioMode = "custom"
)

type audioResolver struct {
	mode    audioMode
	baseURL string
}

// resolveAudio picks the first working rung and returns a precise error
// naming the fix when none works.
func resolveAudio(ctx context.Context, c *Client, mediaBaseURL string) (*audioResolver, error) {
	// Diqqat: ARI modul resursi aynan ".so" suffiksli nomni talab qiladi
	// (suffikssiz so'rov 409 qaytaradi) — lab'da aniqlangan.
	if mediaBaseURL != "" && c.ModuleLoaded(ctx, "res_http_media_cache.so") {
		return &audioResolver{mode: modeHTTP, baseURL: mediaBaseURL}, nil
	}
	probe := audioNames["rating_request"]
	if c.SoundExists(ctx, probe) {
		return &audioResolver{mode: modeSound}, nil
	}
	if c.SoundExists(ctx, "custom/"+probe) {
		return &audioResolver{mode: modeCustom}, nil
	}
	return nil, fmt.Errorf(
		"PBX'da audio topilmadi: %s na sounds katalogida, na custom/ ichida; "+
			"res_http_media_cache ham yo'q. Yechim: `emergency-callback setup` (SSH copy), "+
			"yoki FreePBX GUI → System Recordings orqali 6 faylni yuklang", probe)
}

// Media returns the ARI media URI for a prompt key.
func (r *audioResolver) Media(key string) string {
	name, ok := audioNames[key]
	if !ok {
		name = key
	}
	switch r.mode {
	case modeHTTP:
		return r.baseURL + "/call-media/" + name + ".wav"
	case modeCustom:
		return "sound:custom/" + name
	default:
		return "sound:" + name
	}
}
