package notebooklm

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := c.doRefreshTokens(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if it's a network/timeout error worth retrying
		if isRetryableError(err) && attempt < maxRetries {
			time.Sleep(retryDelay * time.Duration(attempt))
			continue
		}

		break
	}

	return lastErr
}

// doRefreshTokens performs a single refresh attempt
func (c *Client) doRefreshTokens(ctx context.Context) error {
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

// isRetryableError checks if error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "TLS handshake") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "network is unreachable")
}

// rpcCall makes an RPC call to batchexecute with retry
func (c *Client) rpcCall(ctx context.Context, method vo.RPCMethod, params []any, sourcePath string) (any, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := c.doRPCCall(ctx, method, params, sourcePath)
		if err == nil {
			return result, nil
		}

		lastErr = err

		if isRetryableError(err) && attempt < maxRetries {
			time.Sleep(retryDelay * time.Duration(attempt))
			continue
		}

		break
	}

	return nil, lastErr
}

// doRPCCall performs a single RPC call attempt
func (c *Client) doRPCCall(ctx context.Context, method vo.RPCMethod, params []any, sourcePath string) (any, error) {
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
	reqURL := rpc.BuildURL(method, c.auth.SessionID, sourcePath)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(body))
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

// ListSources returns all sources in a notebook
func (c *Client) ListSources(ctx context.Context, notebookID string) ([]vo.Source, error) {
	params := []any{notebookID, nil, []any{2}, nil, 0}
	result, err := c.rpcCall(ctx, vo.RPCGetNotebook, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseSourceList(result, notebookID)
}

// AddSourceURL adds a URL source to a notebook
// Automatically detects YouTube URLs and uses the appropriate method
func (c *Client) AddSourceURL(ctx context.Context, notebookID, sourceURL string) (*vo.Source, error) {
	var params []any

	if isYouTubeURL(sourceURL) {
		// YouTube format: URL at position 7, with extra params
		// [[[None, None, None, None, None, None, None, [url], None, None, 1]], notebook_id, [2], [1, None, None, None, None, None, None, None, None, None, [1]]]
		params = []any{
			[]any{[]any{nil, nil, nil, nil, nil, nil, nil, []any{sourceURL}, nil, nil, 1}},
			notebookID,
			[]any{2},
			[]any{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []any{1}},
		}
	} else {
		// Regular URL format: URL at position 2
		// [[[None, None, [url], None, None, None, None, None]], notebook_id, [2], None, None]
		params = []any{
			[]any{[]any{nil, nil, []any{sourceURL}, nil, nil, nil, nil, nil}},
			notebookID,
			[]any{2},
			nil,
			nil,
		}
	}

	result, err := c.rpcCall(ctx, vo.RPCAddSource, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	source, err := parseSourceFromAdd(result, notebookID)
	if err != nil {
		return nil, err
	}

	// Set URL and type from input since response may not include it
	if source.URL == "" {
		source.URL = sourceURL
	}
	if isYouTubeURL(sourceURL) {
		source.SourceType = "youtube"
	}

	return source, nil
}

// isYouTubeURL checks if URL is a YouTube video link
func isYouTubeURL(url string) bool {
	return strings.Contains(url, "youtube.com/watch") ||
		strings.Contains(url, "youtu.be/") ||
		strings.Contains(url, "youtube.com/shorts/")
}

// AddSourceFile adds a local file as a source to a notebook
// Uses Google's resumable upload protocol
func (c *Client) AddSourceFile(ctx context.Context, notebookID, filePath string) (*vo.Source, error) {
	// Check file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if fileInfo.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file")
	}

	filename := filepath.Base(filePath)
	fileSize := fileInfo.Size()

	// Ensure we have tokens
	if c.auth.CSRFToken == "" {
		if err := c.RefreshTokens(ctx); err != nil {
			return nil, fmt.Errorf("failed to refresh tokens: %w", err)
		}
	}

	// Step 1: Register source intent → get SOURCE_ID
	sourceID, err := c.registerFileSource(ctx, notebookID, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to register file source: %w", err)
	}

	// Step 2: Start resumable upload → get upload URL
	uploadURL, err := c.startResumableUpload(ctx, notebookID, filename, fileSize, sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to start upload: %w", err)
	}

	// Step 3: Upload file content
	if err := c.uploadFile(ctx, uploadURL, filePath); err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	return &vo.Source{
		ID:         sourceID,
		NotebookID: notebookID,
		Title:      filename,
		SourceType: "file",
		Status:     "processing",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

// registerFileSource registers a file source intent and gets SOURCE_ID
func (c *Client) registerFileSource(ctx context.Context, notebookID, filename string) (string, error) {
	params := []any{
		[]any{[]any{filename}},
		notebookID,
		[]any{2},
		[]any{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []any{1}},
	}

	result, err := c.rpcCall(ctx, vo.RPCAddSourceFile, params, "/notebook/"+notebookID)
	if err != nil {
		return "", err
	}

	// Extract SOURCE_ID from nested response
	sourceID := extractNestedString(result)
	if sourceID == "" {
		return "", fmt.Errorf("failed to get source ID from response")
	}

	return sourceID, nil
}

// startResumableUpload starts a resumable upload and returns the upload URL
func (c *Client) startResumableUpload(ctx context.Context, notebookID, filename string, fileSize int64, sourceID string) (string, error) {
	uploadURL := rpc.UploadURL + "?authuser=0"

	body := fmt.Sprintf(`{"PROJECT_ID":"%s","SOURCE_NAME":"%s","SOURCE_ID":"%s"}`,
		notebookID, filename, sourceID)

	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Cookie", c.auth.CookieHeader())
	req.Header.Set("Origin", "https://notebooklm.google.com")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("x-goog-authuser", "0")
	req.Header.Set("x-goog-upload-command", "start")
	req.Header.Set("x-goog-upload-header-content-length", fmt.Sprintf("%d", fileSize))
	req.Header.Set("x-goog-upload-protocol", "resumable")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload start failed with status %d", resp.StatusCode)
	}

	resultURL := resp.Header.Get("x-goog-upload-url")
	if resultURL == "" {
		return "", fmt.Errorf("no upload URL in response headers")
	}

	return resultURL, nil
}

// uploadFile uploads file content to the resumable upload URL
func (c *Client) uploadFile(ctx context.Context, uploadURL, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, file)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	req.Header.Set("Cookie", c.auth.CookieHeader())
	req.Header.Set("Origin", "https://notebooklm.google.com")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("x-goog-authuser", "0")
	req.Header.Set("x-goog-upload-command", "upload, finalize")
	req.Header.Set("x-goog-upload-offset", "0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}

	return nil
}

// AddSourceText adds a text source to a notebook
func (c *Client) AddSourceText(ctx context.Context, notebookID, title, content string) (*vo.Source, error) {
	// Python format: [[[None, [title, content], None, None, None, None, None, None]], notebook_id, [2], None, None]
	params := []any{
		[]any{[]any{nil, []any{title, content}, nil, nil, nil, nil, nil, nil}},
		notebookID,
		[]any{2},
		nil,
		nil,
	}
	result, err := c.rpcCall(ctx, vo.RPCAddSource, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseSourceFromAdd(result, notebookID)
}

// DeleteSource deletes a source from a notebook
func (c *Client) DeleteSource(ctx context.Context, notebookID, sourceID string) error {
	// Python format: [[[source_id]]]
	params := []any{[]any{[]any{sourceID}}}
	_, err := c.rpcCall(ctx, vo.RPCDeleteSource, params, "/notebook/"+notebookID)
	return err
}

// ========== Artifact Operations ==========

// GenerateAudio generates an audio podcast from notebook sources
func (c *Client) GenerateAudio(ctx context.Context, notebookID string, format vo.AudioFormat, length vo.AudioLength) (*vo.GenerationStatus, error) {
	// Get source IDs first
	sourceIDs, err := c.getSourceIDs(ctx, notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source IDs: %w", err)
	}

	if len(sourceIDs) == 0 {
		return nil, fmt.Errorf("notebook has no sources to generate audio from")
	}

	// Build source arrays
	// source_ids_triple: [[[sid]] for sid in source_ids]
	sourceIDsTriple := make([]any, len(sourceIDs))
	for i, sid := range sourceIDs {
		sourceIDsTriple[i] = []any{[]any{sid}}
	}

	// source_ids_double: [[sid] for sid in source_ids]
	sourceIDsDouble := make([]any, len(sourceIDs))
	for i, sid := range sourceIDs {
		sourceIDsDouble[i] = []any{sid}
	}

	// Python format:
	// [[2], notebook_id, [None, None, 1, source_ids_triple, None, None, [None, [instructions, length, None, source_ids_double, "en", None, format]]]]
	var formatCode, lengthCode any
	if format != 0 {
		formatCode = int(format)
	}
	if length != 0 {
		lengthCode = int(length)
	}

	params := []any{
		[]any{2},
		notebookID,
		[]any{
			nil,
			nil,
			1, // StudioContentType.AUDIO
			sourceIDsTriple,
			nil,
			nil,
			[]any{
				nil,
				[]any{
					nil,       // instructions
					lengthCode,
					nil,
					sourceIDsDouble,
					"en",     // language
					nil,
					formatCode,
				},
			},
		},
	}

	// Use RPCCreateVideo for all artifact generation (same as Python implementation)
	result, err := c.rpcCall(ctx, vo.RPCCreateVideo, params, "/notebook/"+notebookID)
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
	// Note: parameter order is [taskID, notebookID, [2]] - same as Python
	params := []any{taskID, notebookID, []any{2}}
	result, err := c.rpcCall(ctx, vo.RPCPollStudio, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parsePollStatus(result, taskID)
}

// ListArtifacts lists all artifacts in a notebook
func (c *Client) ListArtifacts(ctx context.Context, notebookID string) ([]vo.Artifact, error) {
	// Python format: [[2], notebook_id, 'NOT artifact.status = "ARTIFACT_STATUS_SUGGESTED"']
	params := []any{[]any{2}, notebookID, `NOT artifact.status = "ARTIFACT_STATUS_SUGGESTED"`}
	result, err := c.rpcCall(ctx, vo.RPCListArtifacts, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return parseArtifactList(result)
}

// DownloadAudio downloads a completed audio artifact to a file
func (c *Client) DownloadAudio(ctx context.Context, notebookID, outputPath string, artifactID string) (string, error) {
	artifacts, err := c.ListArtifacts(ctx, notebookID)
	if err != nil {
		return "", fmt.Errorf("failed to list artifacts: %w", err)
	}

	// Find audio artifact (type 1 = audio)
	var audioArtifact *vo.Artifact
	for i := range artifacts {
		a := &artifacts[i]
		if a.ArtifactType == 1 && a.Status == "completed" {
			if artifactID == "" || a.ID == artifactID {
				audioArtifact = a
				break
			}
		}
	}

	if audioArtifact == nil {
		return "", fmt.Errorf("no completed audio artifact found")
	}

	if audioArtifact.DownloadURL == "" {
		return "", fmt.Errorf("audio artifact has no download URL")
	}

	// Download the file
	if err := c.downloadFile(ctx, audioArtifact.DownloadURL, outputPath); err != nil {
		return "", fmt.Errorf("failed to download audio: %w", err)
	}

	return outputPath, nil
}

// DownloadVideo downloads a completed video artifact to a file
func (c *Client) DownloadVideo(ctx context.Context, notebookID, outputPath string, artifactID string) (string, error) {
	artifacts, err := c.ListArtifacts(ctx, notebookID)
	if err != nil {
		return "", fmt.Errorf("failed to list artifacts: %w", err)
	}

	// Find video artifact (type 3 = video)
	var videoArtifact *vo.Artifact
	for i := range artifacts {
		a := &artifacts[i]
		if a.ArtifactType == 3 && a.Status == "completed" {
			if artifactID == "" || a.ID == artifactID {
				videoArtifact = a
				break
			}
		}
	}

	if videoArtifact == nil {
		return "", fmt.Errorf("no completed video artifact found")
	}

	if videoArtifact.DownloadURL == "" {
		return "", fmt.Errorf("video artifact has no download URL")
	}

	// Download the file
	if err := c.downloadFile(ctx, videoArtifact.DownloadURL, outputPath); err != nil {
		return "", fmt.Errorf("failed to download video: %w", err)
	}

	return outputPath, nil
}

// downloadFile downloads a file from URL to local path
func (c *Client) downloadFile(ctx context.Context, downloadURL, outputPath string) error {
	// Build cookie header for all requests
	cookieHeader := c.auth.CookieHeader()

	// Create a custom client that adds cookies to every request including redirects
	downloadClient := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Add cookies to redirected request
			if cookieHeader != "" {
				req.Header.Set("Cookie", cookieHeader)
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}

	// Set headers for initial request
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		return fmt.Errorf("received HTML instead of media file (authentication may have failed)")
	}

	// Create output file
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
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

	// If no source IDs provided, get all sources from notebook
	if len(sourceIDs) == 0 {
		ids, err := c.getSourceIDs(ctx, notebookID)
		if err != nil {
			return nil, fmt.Errorf("failed to get source IDs: %w", err)
		}
		sourceIDs = ids
	}

	if len(sourceIDs) == 0 {
		return nil, fmt.Errorf("notebook has no sources to query")
	}

	// Generate new conversation ID
	conversationID := generateUUID()

	// Build chat request with CSRF token
	body, err := rpc.EncodeChatRequest(question, sourceIDs, conversationID, nil, c.auth.CSRFToken)
	if err != nil {
		return nil, fmt.Errorf("failed to encode chat request: %w", err)
	}

	// Build URL
	c.reqCounter += 100000
	reqURL := rpc.BuildChatURL(c.auth.SessionID, c.reqCounter)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Cookie", c.auth.CookieHeader())

	// Execute with retry
	var resp *http.Response
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err = c.httpClient.Do(req)
		if err == nil {
			break
		}
		lastErr = err
		if isRetryableError(err) && attempt < maxRetries {
			time.Sleep(retryDelay * time.Duration(attempt))
			// Recreate request for retry
			req, _ = http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
			req.Header.Set("Cookie", c.auth.CookieHeader())
			continue
		}
		break
	}
	if resp == nil {
		return nil, fmt.Errorf("chat request failed: %w", lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chat request failed with status %d: %s", resp.StatusCode, string(respBody[:min(200, len(respBody))]))
	}

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse chat response
	answer, _, err := rpc.ParseChatResponse(string(respBody))
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (response preview: %s)", err, string(respBody[:min(500, len(respBody))]))
	}

	return &vo.AskResult{
		Answer:         answer,
		ConversationID: conversationID,
		TurnNumber:     1,
	}, nil
}

// getSourceIDs extracts source IDs from a notebook
func (c *Client) getSourceIDs(ctx context.Context, notebookID string) ([]string, error) {
	params := []any{notebookID, nil, []any{2}, nil, 0}
	result, err := c.rpcCall(ctx, vo.RPCGetNotebook, params, "/notebook/"+notebookID)
	if err != nil {
		return nil, err
	}

	return extractSourceIDs(result), nil
}

// extractSourceIDs extracts source IDs from notebook response
func extractSourceIDs(data any) []string {
	var ids []string

	arr, ok := data.([]any)
	if !ok || len(arr) == 0 {
		return ids
	}

	// Notebook data is in first element
	nbData, ok := arr[0].([]any)
	if !ok || len(nbData) < 2 {
		return ids
	}

	// Sources are in second element
	sources, ok := nbData[1].([]any)
	if !ok {
		return ids
	}

	for _, source := range sources {
		sourceArr, ok := source.([]any)
		if !ok || len(sourceArr) == 0 {
			continue
		}

		// Source ID is nested: source[0][0][0] or source[0][0]
		id := extractNestedID(sourceArr[0])
		if id != "" {
			ids = append(ids, id)
		}
	}

	return ids
}

// extractNestedID extracts ID from nested structure
func extractNestedID(data any) string {
	switch v := data.(type) {
	case string:
		if isUUIDFormat(v) {
			return v
		}
	case []any:
		for _, item := range v {
			if id := extractNestedID(item); id != "" {
				return id
			}
		}
	}
	return ""
}

// isUUIDFormat checks if string is UUID format
func isUUIDFormat(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// generateUUID generates a new UUID v4
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	// Set version 4
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
