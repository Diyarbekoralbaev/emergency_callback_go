package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
)

// Eskiz wraps the Eskiz.uz SMS HTTP API with auto token refresh.
//
// API reference: https://documenter.getpostman.com/view/663428/RzfmES4z
//
//	POST /api/auth/login  (email, password) -> { data: { token } }
//	POST /api/message/sms/send (mobile_phone, message, from) -> {...}
type Eskiz struct {
	cfg    config.EskizConfig
	client *http.Client

	mu    sync.Mutex
	token string
}

func New(cfg config.EskizConfig) *Eskiz {
	return &Eskiz{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
}

// Send sends one SMS. If our token is missing/expired (401), authenticate and retry once.
func (e *Eskiz) Send(ctx context.Context, phone, message string) error {
	if e.cfg.DryRun {
		slog.Info("eskiz dry-run", "phone", phone, "len", len(message))
		return nil
	}
	if err := e.ensureToken(ctx); err != nil {
		return err
	}
	err := e.send(ctx, phone, message)
	if err == nil {
		return nil
	}
	// On 401, force reauth and retry once
	if strings.Contains(err.Error(), "401") {
		e.clearToken()
		if err := e.ensureToken(ctx); err != nil {
			return err
		}
		return e.send(ctx, phone, message)
	}
	return err
}

func (e *Eskiz) ensureToken(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.token != "" {
		return nil
	}
	form := &bytes.Buffer{}
	mw := multipart.NewWriter(form)
	_ = mw.WriteField("email", e.cfg.Email)
	_ = mw.WriteField("password", e.cfg.Password)
	_ = mw.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/auth/login", form)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("eskiz login: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("eskiz login: status %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("eskiz login parse: %w", err)
	}
	if parsed.Data.Token == "" {
		return fmt.Errorf("eskiz login: empty token in response %s", string(body))
	}
	e.token = parsed.Data.Token
	return nil
}

func (e *Eskiz) clearToken() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.token = ""
}

func (e *Eskiz) send(ctx context.Context, phone, message string) error {
	form := &bytes.Buffer{}
	mw := multipart.NewWriter(form)
	_ = mw.WriteField("mobile_phone", normalizePhone(phone))
	_ = mw.WriteField("message", message)
	_ = mw.WriteField("from", "4546")
	_ = mw.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/message/sms/send", form)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	e.mu.Lock()
	tok := e.token
	e.mu.Unlock()
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("eskiz send: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("eskiz send: 401 %s", string(body))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("eskiz send: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// normalizePhone strips leading '+' since Eskiz expects digits only.
func normalizePhone(p string) string {
	out := make([]byte, 0, len(p))
	for i := 0; i < len(p); i++ {
		if p[i] >= '0' && p[i] <= '9' {
			out = append(out, p[i])
		}
	}
	return string(out)
}
