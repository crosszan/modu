// Package nano_banana_pro 提供 Gemini 图片生成 API 的客户端封装
package nano_banana_pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultTimeout 默认请求超时时间
	DefaultTimeout = 120 * time.Second
	// DefaultModel 默认使用的模型
	DefaultModel = "gemini-3-pro-image"
)

// Client 是 Gemini 图片生成 API 的客户端
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	model      string
}

// ClientOption 客户端配置选项
type ClientOption func(*Client)

// WithTimeout 设置请求超时时间
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithModel 设置默认模型
func WithModel(model string) ClientOption {
	return func(c *Client) {
		c.model = model
	}
}

// WithHTTPClient 设置自定义 HTTP 客户端
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// NewClient 创建新的客户端实例
func NewClient(baseURL, apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		model: DefaultModel,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// GenerateImage 根据提示词生成图片
func (c *Client) GenerateImage(prompt string) (*Response, error) {
	return c.GenerateImageWithModel(c.model, prompt)
}

// GenerateImageWithModel 使用指定模型根据提示词生成图片
func (c *Client) GenerateImageWithModel(model, prompt string) (*Response, error) {
	req := Request{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, model, c.apiKey)

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败 (%d): %s", resp.StatusCode, string(body))
	}

	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &result, nil
}

// GetModel 获取当前使用的模型
func (c *Client) GetModel() string {
	return c.model
}

// SetModel 设置默认模型
func (c *Client) SetModel(model string) {
	c.model = model
}
