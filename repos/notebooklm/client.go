package notebooklm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/crosszan/modu/repos/notebooklm/rpc"
	vo "github.com/crosszan/modu/vo/notebooklm_vo"
)

const (
	defaultTimeout = 120 * time.Second
	maxRetries     = 3
	retryDelay     = 2 * time.Second
)

// Client is the NotebookLM API client
type Client struct {
	auth       *vo.AuthTokens
	httpClient *http.Client
	reqCounter int
}

// NewClient creates a new NotebookLM client
func NewClient(auth *vo.AuthTokens) *Client {
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	// Check for proxy from environment
	if proxyURL := os.Getenv("HTTPS_PROXY"); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	} else if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &Client{
		auth: auth,
		httpClient: &http.Client{
			Timeout:   defaultTimeout,
			Transport: transport,
		},
		reqCounter: 100000,
	}
}

// NewClientFromStorage creates a client from stored auth
func NewClientFromStorage(storagePath string) (*Client, error) {
	auth, err := LoadAuthTokens(storagePath)
	if err != nil {
		return nil, err
	}
	return NewClient(auth), nil
}

// RefreshTokens fetches fresh CSRF token and session ID from homepage
func (c *Client) RefreshTokens(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", rpc.BaseURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Cookie", c.auth.CookieHeader())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch homepage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("homepage returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read homepage: %w", err)
	}

	html := string(body)

	csrf, err := ExtractCSRFToken(html)
	if err != nil {
		return err
	}

	sessionID, err := ExtractSessionID(html)
	if err != nil {
		return err
	}

	c.auth.CSRFToken = csrf
	c.auth.SessionID = sessionID

	return nil
}

// rpcCall makes an RPC call to batchexecute
func (c *Client) rpcCall(ctx context.Context, method vo.RPCMethod, params []any, sourcePath string) (any, error) {
	// Ensure we have tokens
	if c.auth.CSRFToken == "" {
		if err := c.RefreshTokens(ctx); err != nil {
			return nil, fmt.Errorf("failed to refresh tokens: %w", err)
		}
	}

	// Encode request
	rpcReq, err := rpc.EncodeRPCRequest(method, params)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	body, err := rpc.BuildRequestBody(rpcReq, c.auth.CSRFToken)
	if err != nil {
		return nil, fmt.Errorf("failed to build request body: %w", err)
	}

	// Build URL
	url := rpc.BuildURL(method, c.auth.SessionID, sourcePath)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Cookie", c.auth.CookieHeader())

	// Execute
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for auth errors
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: status %d", rpc.ErrAuthError, resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Decode response
	return rpc.DecodeResponse(string(respBody), method)
}

// ========== Notebook Operations ==========

// ListNotebooks returns all notebooks
func (c *Client) ListNotebooks(ctx context.Context) ([]vo.Notebook, error) {
	params := []any{nil, 1, nil, []any{2}}
	result, err := c.rpcCall(ctx, vo.RPCListNotebooks, params, "/")
	if err != nil {
		return nil, err
	}

	return parseNotebookList(result)
}

// CreateNotebook creates a new notebook
func (c *Client) CreateNotebook(ctx context.Context, title string) (*vo.Notebook, error) {
	params := []any{title, nil, nil, []any{2}, []any{1}}
	result, err := c.rpcCall(ctx, vo.RPCCreateNotebook, params, "/")
	if err != nil {
		return nil, err
	}

	return parseNotebook(result)
}

// GetNotebook retrieves a notebook by ID
func (c *Client) GetNotebook(ctx context.Context, notebookID string) (*vo.Notebook, error) {
	params := []any{notebookID, nil, []any{2}, nil, 0}
	result, err := c.rpcCall(ctx, vo.RPCGetNotebook, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	arr, ok := result.([]any)
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("invalid notebook response")
	}

	return parseNotebook(arr[0])
}

// RenameNotebook renames a notebook
func (c *Client) RenameNotebook(ctx context.Context, notebookID, newTitle string) error {
	params := []any{notebookID, newTitle}
	_, err := c.rpcCall(ctx, vo.RPCRenameNotebook, params, "/notebook/"+notebookID)
	return err
}

// DeleteNotebook deletes a notebook
func (c *Client) DeleteNotebook(ctx context.Context, notebookID string) error {
	params := []any{[]any{notebookID}}
	_, err := c.rpcCall(ctx, vo.RPCDeleteNotebook, params, "/")
	return err
}

// ========== Source Operations ==========

// AddSourceURL adds a URL source to a notebook
func (c *Client) AddSourceURL(ctx context.Context, notebookID, url string) (*vo.Source, error) {
	params := []any{notebookID, []any{[]any{url}}, []any{2}}
	result, err := c.rpcCall(ctx, vo.RPCAddSourceURL, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseSource(result, notebookID)
}

// AddSourceText adds a text source to a notebook
func (c *Client) AddSourceText(ctx context.Context, notebookID, title, content string) (*vo.Source, error) {
	params := []any{notebookID, []any{[]any{nil, nil, nil, []any{title, content}}}, []any{2}}
	result, err := c.rpcCall(ctx, vo.RPCAddSource, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseSource(result, notebookID)
}

// DeleteSource deletes a source from a notebook
func (c *Client) DeleteSource(ctx context.Context, notebookID, sourceID string) error {
	params := []any{notebookID, []any{[]any{[]any{sourceID}}}}
	_, err := c.rpcCall(ctx, vo.RPCDeleteSource, params, "/notebook/"+notebookID)
	return err
}

// ========== Artifact Operations ==========

// GenerateAudio generates an audio podcast
func (c *Client) GenerateAudio(ctx context.Context, notebookID string, format vo.AudioFormat, length vo.AudioLength) (*vo.GenerationStatus, error) {
	params := []any{notebookID, []any{int(format), int(length)}, []any{2}}
	result, err := c.rpcCall(ctx, vo.RPCCreateAudio, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseGenerationStatus(result)
}

// GenerateVideo generates a video
func (c *Client) GenerateVideo(ctx context.Context, notebookID string, format vo.VideoFormat, style vo.VideoStyle) (*vo.GenerationStatus, error) {
	params := []any{notebookID, []any{int(format), int(style)}, []any{2}}
	result, err := c.rpcCall(ctx, vo.RPCCreateVideo, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseGenerationStatus(result)
}

// PollGeneration checks the status of artifact generation
func (c *Client) PollGeneration(ctx context.Context, notebookID, taskID string) (*vo.GenerationStatus, error) {
	params := []any{notebookID, taskID, []any{2}}
	result, err := c.rpcCall(ctx, vo.RPCPollStudio, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseGenerationStatus(result)
}

// ListArtifacts lists all artifacts in a notebook
func (c *Client) ListArtifacts(ctx context.Context, notebookID string) ([]vo.Artifact, error) {
	params := []any{notebookID, []any{2}}
	result, err := c.rpcCall(ctx, vo.RPCListArtifacts, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseArtifactList(result)
}

// ========== Chat Operations ==========

// Ask sends a question to the notebook
func (c *Client) Ask(ctx context.Context, notebookID, question string, sourceIDs []string) (*vo.AskResult, error) {
	// Ensure we have tokens
	if c.auth.CSRFToken == "" {
		if err := c.RefreshTokens(ctx); err != nil {
			return nil, fmt.Errorf("failed to refresh tokens: %w", err)
		}
	}

	// If no source IDs provided, get all sources
	if len(sourceIDs) == 0 {
		nb, err := c.GetNotebook(ctx, notebookID)
		if err != nil {
			return nil, fmt.Errorf("failed to get notebook sources: %w", err)
		}
		_ = nb // TODO: extract source IDs from notebook
	}

	// Build chat request
	body, err := rpc.EncodeChatRequest(notebookID, question, sourceIDs, "", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encode chat request: %w", err)
	}

	// Build URL
	c.reqCounter += 100000
	url := rpc.BuildChatURL(c.auth.SessionID, c.reqCounter)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Cookie", c.auth.CookieHeader())

	// Execute
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chat request failed with status %d", resp.StatusCode)
	}

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse chat response
	answer, _, err := rpc.ParseChatResponse(string(respBody))
	if err != nil {
		return nil, err
	}

	return &vo.AskResult{
		Answer: answer,
	}, nil
}
