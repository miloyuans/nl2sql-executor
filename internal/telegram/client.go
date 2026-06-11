package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"nl2sql-executor-go-prod/internal/config"
)

type Client struct {
	cfg    config.TelegramConfig
	httpc  *http.Client
	apiURL string
}

func NewClient(cfg config.TelegramConfig) *Client {
	return &Client{cfg: cfg, httpc: &http.Client{Timeout: 60 * time.Second}, apiURL: cfg.BaseURL + "/bot" + cfg.BotToken}
}

func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	if chatID == "" {
		return fmt.Errorf("chat_id is empty")
	}
	body := map[string]any{"chat_id": chatID, "text": text, "disable_notification": c.cfg.DisableNotification}
	return c.postJSON(ctx, "sendMessage", body)
}

func (c *Client) SendDocument(ctx context.Context, chatID, filePath, caption string) error {
	if chatID == "" {
		return fmt.Errorf("chat_id is empty")
	}
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", chatID)
	_ = mw.WriteField("caption", caption)
	if c.cfg.DisableNotification {
		_ = mw.WriteField("disable_notification", "true")
	}
	part, err := mw.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/sendDocument", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("telegram sendDocument status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, method string, payload any) error {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/"+method, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("telegram %s status=%d body=%s", method, resp.StatusCode, string(bb))
	}
	return nil
}
