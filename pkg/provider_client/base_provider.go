package provider_client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BaseProvider 公共 HTTP 客户端
type BaseProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type BaseOption func(*BaseProvider)

func WithModel(model string) BaseOption {
	return func(p *BaseProvider) { p.Model = model }
}

func WithTimeout(timeout time.Duration) BaseOption {
	return func(p *BaseProvider) { p.HTTPClient.Timeout = timeout }
}

func NewBaseProvider(baseURL, apiKey, defaultModel string, opts ...BaseOption) *BaseProvider {
	p := &BaseProvider{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      defaultModel,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *BaseProvider) DoRequest(ctx context.Context, method, url string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed (%d): %s", resp.StatusCode, string(data))
	}

	return data, nil
}

func (p *BaseProvider) GetModel(reqModel string) string {
	if reqModel != "" {
		return reqModel
	}
	return p.Model
}
