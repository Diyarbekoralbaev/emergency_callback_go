// Package ari drives one rating call over the Asterisk REST Interface
// (Stasis). No dialplan contexts are needed on the PBX — the only PBX-side
// setup is an ARI user. Audio is played via an HTTP URL when
// res_http_media_cache is available, else from the PBX sounds dir.
package ari

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Client is a minimal ARI REST + WebSocket client scoped to one app name.
type Client struct {
	base string // http://host:8088/ari
	user string
	pass string
	app  string
	http *http.Client
}

func NewClient(baseURL, user, pass, app string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		user: user,
		pass: pass,
		app:  app,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values) ([]byte, int, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(c.user, c.pass)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode, nil
}

func (c *Client) doOK(ctx context.Context, method, path string, q url.Values) ([]byte, error) {
	body, code, err := c.do(ctx, method, path, q)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("ari %s %s: HTTP %d: %s", method, path, code, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// Originate creates a channel into this client's Stasis app (no dialplan).
// Returns the channel id.
func (c *Client) Originate(ctx context.Context, endpoint, callerID string, timeoutSec int) (string, error) {
	q := url.Values{
		"endpoint": {endpoint},
		"app":      {c.app},
		"callerId": {callerID},
		"timeout":  {fmt.Sprint(timeoutSec)},
	}
	body, err := c.doOK(ctx, http.MethodPost, "/channels", q)
	if err != nil {
		return "", err
	}
	var ch struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ch); err != nil {
		return "", fmt.Errorf("originate parse: %w", err)
	}
	return ch.ID, nil
}

// Play queues media on a channel; returns the playback id.
// media examples: "sound:ambulance-rating-request", "sound:custom/x",
// "http://app:8000/call-media/x.wav" (needs res_http_media_cache).
func (c *Client) Play(ctx context.Context, channelID, media string) (string, error) {
	q := url.Values{"media": {media}}
	body, err := c.doOK(ctx, http.MethodPost, "/channels/"+channelID+"/play", q)
	if err != nil {
		return "", err
	}
	var pb struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &pb); err != nil {
		return "", err
	}
	return pb.ID, nil
}

// StopPlayback cancels a queued/active playback (barge-in).
func (c *Client) StopPlayback(ctx context.Context, playbackID string) {
	_, _, _ = c.do(ctx, http.MethodDelete, "/playbacks/"+playbackID, nil)
}

// Hangup terminates a channel (404 is fine — already gone).
func (c *Client) Hangup(ctx context.Context, channelID string) {
	_, _, _ = c.do(ctx, http.MethodDelete, "/channels/"+channelID, nil)
}

// CreateBridge makes a mixing bridge.
func (c *Client) CreateBridge(ctx context.Context) (string, error) {
	body, err := c.doOK(ctx, http.MethodPost, "/bridges", url.Values{"type": {"mixing"}})
	if err != nil {
		return "", err
	}
	var br struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &br); err != nil {
		return "", err
	}
	return br.ID, nil
}

// AddToBridge places a channel into a bridge.
func (c *Client) AddToBridge(ctx context.Context, bridgeID, channelID string) error {
	_, err := c.doOK(ctx, http.MethodPost, "/bridges/"+bridgeID+"/addChannel", url.Values{"channel": {channelID}})
	return err
}

// DeleteBridge tears a bridge down.
func (c *Client) DeleteBridge(ctx context.Context, bridgeID string) {
	_, _, _ = c.do(ctx, http.MethodDelete, "/bridges/"+bridgeID, nil)
}

// SoundExists checks the PBX sound index for a bare sound name.
func (c *Client) SoundExists(ctx context.Context, name string) bool {
	_, code, err := c.do(ctx, http.MethodGet, "/sounds/"+url.PathEscape(name), nil)
	return err == nil && code == 200
}

// ModuleLoaded reports whether an Asterisk module is loaded.
func (c *Client) ModuleLoaded(ctx context.Context, name string) bool {
	_, code, err := c.do(ctx, http.MethodGet, "/asterisk/modules/"+url.PathEscape(name), nil)
	return err == nil && code == 200
}

// Event is one decoded ARI WebSocket event (only the fields we use).
type Event struct {
	Type     string `json:"type"`
	Digit    string `json:"digit,omitempty"`
	Playback struct {
		ID string `json:"id"`
	} `json:"playback,omitempty"`
	Channel struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"channel,omitempty"`
}

// EventConn is an open ARI events WebSocket.
type EventConn struct {
	ws *websocket.Conn
}

// ConnectEvents opens the events WebSocket for this app. Connecting
// implicitly registers the app name with Asterisk.
func (c *Client) ConnectEvents(ctx context.Context) (*EventConn, error) {
	wsURL := strings.Replace(c.base, "http", "ws", 1) +
		"/events?app=" + url.QueryEscape(c.app) +
		"&api_key=" + url.QueryEscape(c.user+":"+c.pass)
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ari websocket: %w", err)
	}
	ws.SetReadLimit(1 << 20)
	return &EventConn{ws: ws}, nil
}

// Read blocks until the next event (or ctx cancellation).
func (e *EventConn) Read(ctx context.Context) (Event, error) {
	var ev Event
	_, data, err := e.ws.Read(ctx)
	if err != nil {
		return ev, err
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return ev, fmt.Errorf("ari event parse: %w", err)
	}
	return ev, nil
}

func (e *EventConn) Close() { _ = e.ws.Close(websocket.StatusNormalClosure, "") }
