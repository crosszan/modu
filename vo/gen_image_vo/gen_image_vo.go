package genimagevo

type GenImageRequest struct {
	UserPrompt   string
	SystemPrompt string
}

type GenImageResponse struct {
	Images       []*Image
	Model        string
	ProviderName string
	Usage        *UsageInfo
	RawResponse  any
}

type Image struct {
	Data     []byte
	MimeType string
	URL      string
}

type UsageInfo struct {
	PromptTokens int
	TotalTokens  int
}
