package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"castflow/internal/domain"
)

// Notifier POSTs lifecycle events to a configured webhook URL.
type Notifier struct {
	url    string
	secret string
	client *http.Client
}

func NewNotifier(url, secret string) *Notifier {
	if url == "" {
		return &Notifier{}
	}
	return &Notifier{
		url:    url,
		secret: secret,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

type renditionReadyEvent struct {
	Event     string                `json:"event"`
	VideoID   string                `json:"videoId"`
	Title     string                `json:"title"`
	Profile   string                `json:"profile"`
	Revision  string                `json:"revision"`
	Duration  int                   `json:"durationSec"`
	Rendition domain.RenditionLinks `json:"rendition"`
}

func (n *Notifier) NotifyRenditionReady(ctx context.Context, video *domain.Video, rendition *domain.Rendition, links domain.RenditionLinks) error {
	if n == nil || n.url == "" {
		return nil
	}
	body, err := json.Marshal(renditionReadyEvent{
		Event:     "rendition.ready",
		VideoID:   video.ID.String(),
		Title:     video.Title,
		Profile:   rendition.Profile,
		Revision:  rendition.Revision,
		Duration:  rendition.DurationSec,
		Rendition: links,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "castflow-webhook/1.0")
	if n.secret != "" {
		mac := hmac.New(sha256.New, []byte(n.secret))
		mac.Write(body)
		req.Header.Set("X-Castflow-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := n.client.Do(req)
	if err != nil {
		slog.Warn("webhook delivery failed", "url", n.url, "err", err)
		return fmt.Errorf("webhook post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("webhook non-2xx", "url", n.url, "status", resp.StatusCode)
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

var _ domain.WebhookNotifier = (*Notifier)(nil)
