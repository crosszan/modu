package nano_banana_pro

// Request 请求结构
type Request struct {
	Contents []Content `json:"contents"`
}

// Content 内容结构
type Content struct {
	Parts []Part `json:"parts"`
}

// Part 部分内容
type Part struct {
	Text string `json:"text"`
}

// Response 响应结构
type Response struct {
	Candidates    []Candidate   `json:"candidates"`
	ModelVersion  string        `json:"modelVersion"`
	ResponseID    string        `json:"responseId"`
	UsageMetadata UsageMetadata `json:"usageMetadata"`
}

// Candidate 候选结果
type Candidate struct {
	Content      CandidateContent `json:"content"`
	FinishReason string           `json:"finishReason"`
}

// CandidateContent 候选内容
type CandidateContent struct {
	Parts []ResponsePart `json:"parts"`
	Role  string         `json:"role"`
}

// ResponsePart 响应部分
type ResponsePart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

// InlineData 内联数据（图片）
type InlineData struct {
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data"` // Base64 编码的图片数据
}

// UsageMetadata 使用统计
type UsageMetadata struct {
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	PromptTokenCount     int `json:"promptTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// HasImages 检查响应是否包含图片
func (r *Response) HasImages() bool {
	if len(r.Candidates) == 0 {
		return false
	}
	for _, candidate := range r.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.Data != "" {
				return true
			}
		}
	}
	return false
}

// GetImages 获取所有图片数据
func (r *Response) GetImages() []*InlineData {
	var images []*InlineData
	for _, candidate := range r.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.Data != "" {
				images = append(images, part.InlineData)
			}
		}
	}
	return images
}

// GetTexts 获取所有文本内容
func (r *Response) GetTexts() []string {
	var texts []string
	for _, candidate := range r.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
	}
	return texts
}
