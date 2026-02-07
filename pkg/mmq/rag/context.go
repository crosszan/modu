package rag

import (
	"fmt"
	"strings"
)

// ContextBuilder 上下文构建器
type ContextBuilder struct {
	maxTokens      int
	includeSource  bool
	includeScore   bool
	separator      string
	contextFormat  ContextFormat
	tokenEstimator func(string) int
}

// ContextFormat 上下文格式
type ContextFormat string

const (
	FormatPlain    ContextFormat = "plain"    // 纯文本
	FormatMarkdown ContextFormat = "markdown" // Markdown格式
	FormatXML      ContextFormat = "xml"      // XML格式
	FormatJSON     ContextFormat = "json"     // JSON格式
)

// ContextBuilderOptions 构建器选项
type ContextBuilderOptions struct {
	MaxTokens     int           // 最大token数
	IncludeSource bool          // 包含来源信息
	IncludeScore  bool          // 包含相关性分数
	Separator     string        // 上下文分隔符
	Format        ContextFormat // 输出格式
}

// DefaultContextBuilderOptions 默认选项
func DefaultContextBuilderOptions() ContextBuilderOptions {
	return ContextBuilderOptions{
		MaxTokens:     2000,
		IncludeSource: true,
		IncludeScore:  true,
		Separator:     "\n\n---\n\n",
		Format:        FormatMarkdown,
	}
}

// NewContextBuilder 创建上下文构建器
func NewContextBuilder(opts ContextBuilderOptions) *ContextBuilder {
	return &ContextBuilder{
		maxTokens:      opts.MaxTokens,
		includeSource:  opts.IncludeSource,
		includeScore:   opts.IncludeScore,
		separator:      opts.Separator,
		contextFormat:  opts.Format,
		tokenEstimator: estimateTokens,
	}
}

// Build 构建上下文
func (cb *ContextBuilder) Build(contexts []Context) string {
	if len(contexts) == 0 {
		return ""
	}

	var parts []string
	totalTokens := 0

	for i, ctx := range contexts {
		// 格式化单个上下文
		formatted := cb.formatContext(ctx, i+1)

		// 估算token数
		tokens := cb.tokenEstimator(formatted)

		// 检查是否超出限制
		if totalTokens+tokens > cb.maxTokens {
			break
		}

		parts = append(parts, formatted)
		totalTokens += tokens
	}

	return strings.Join(parts, cb.separator)
}

// formatContext 格式化单个上下文
func (cb *ContextBuilder) formatContext(ctx Context, index int) string {
	switch cb.contextFormat {
	case FormatMarkdown:
		return cb.formatMarkdown(ctx, index)
	case FormatXML:
		return cb.formatXML(ctx, index)
	case FormatJSON:
		return cb.formatJSON(ctx, index)
	default:
		return cb.formatPlain(ctx, index)
	}
}

// formatPlain 纯文本格式
func (cb *ContextBuilder) formatPlain(ctx Context, index int) string {
	var parts []string

	if cb.includeSource {
		parts = append(parts, fmt.Sprintf("Source: %s", ctx.Source))
	}

	if cb.includeScore {
		parts = append(parts, fmt.Sprintf("Relevance: %.0f%%", ctx.Relevance*100))
	}

	parts = append(parts, ctx.Text)

	return strings.Join(parts, "\n")
}

// formatMarkdown Markdown格式
func (cb *ContextBuilder) formatMarkdown(ctx Context, index int) string {
	var builder strings.Builder

	// 标题
	if title, ok := ctx.Metadata["title"].(string); ok && title != "" {
		builder.WriteString(fmt.Sprintf("### %d. %s\n\n", index, title))
	} else {
		builder.WriteString(fmt.Sprintf("### Context %d\n\n", index))
	}

	// 元信息
	if cb.includeSource || cb.includeScore {
		builder.WriteString("**Metadata:**\n")
		if cb.includeSource {
			builder.WriteString(fmt.Sprintf("- Source: `%s`\n", ctx.Source))
		}
		if cb.includeScore {
			builder.WriteString(fmt.Sprintf("- Relevance: %.1f%%\n", ctx.Relevance*100))
		}
		builder.WriteString("\n")
	}

	// 内容
	builder.WriteString(ctx.Text)

	return builder.String()
}

// formatXML XML格式
func (cb *ContextBuilder) formatXML(ctx Context, index int) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("<context id=\"%d\">\n", index))

	if cb.includeSource {
		builder.WriteString(fmt.Sprintf("  <source>%s</source>\n", escapeXML(ctx.Source)))
	}

	if cb.includeScore {
		builder.WriteString(fmt.Sprintf("  <relevance>%.4f</relevance>\n", ctx.Relevance))
	}

	builder.WriteString(fmt.Sprintf("  <text>%s</text>\n", escapeXML(ctx.Text)))
	builder.WriteString("</context>")

	return builder.String()
}

// formatJSON JSON格式（简化版）
func (cb *ContextBuilder) formatJSON(ctx Context, index int) string {
	var builder strings.Builder

	builder.WriteString("{\n")
	builder.WriteString(fmt.Sprintf("  \"id\": %d,\n", index))

	if cb.includeSource {
		builder.WriteString(fmt.Sprintf("  \"source\": \"%s\",\n", escapeJSON(ctx.Source)))
	}

	if cb.includeScore {
		builder.WriteString(fmt.Sprintf("  \"relevance\": %.4f,\n", ctx.Relevance))
	}

	builder.WriteString(fmt.Sprintf("  \"text\": \"%s\"\n", escapeJSON(ctx.Text)))
	builder.WriteString("}")

	return builder.String()
}

// estimateTokens 估算token数（简化版本）
func estimateTokens(text string) int {
	// 简化估算：4个字符 ≈ 1个token
	return len(text) / 4
}

// escapeXML 转义XML特殊字符
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// escapeJSON 转义JSON特殊字符
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// BuildPrompt 构建完整的提示词
func (cb *ContextBuilder) BuildPrompt(query string, contexts []Context, systemPrompt string) string {
	var builder strings.Builder

	// 系统提示
	if systemPrompt != "" {
		builder.WriteString(systemPrompt)
		builder.WriteString("\n\n")
	}

	// 上下文
	if len(contexts) > 0 {
		builder.WriteString("## Retrieved Context\n\n")
		builder.WriteString(cb.Build(contexts))
		builder.WriteString("\n\n")
	}

	// 用户查询
	builder.WriteString("## User Query\n\n")
	builder.WriteString(query)

	return builder.String()
}

// TruncateContext 截断上下文以适应token限制
func (cb *ContextBuilder) TruncateContext(text string, maxTokens int) string {
	tokens := cb.tokenEstimator(text)

	if tokens <= maxTokens {
		return text
	}

	// 简单截断（按字符）
	maxChars := maxTokens * 4
	if len(text) <= maxChars {
		return text
	}

	// 在单词边界截断
	truncated := text[:maxChars]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxChars-100 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// MergeContexts 合并多个上下文
func (cb *ContextBuilder) MergeContexts(contexts []Context) Context {
	if len(contexts) == 0 {
		return Context{}
	}

	if len(contexts) == 1 {
		return contexts[0]
	}

	// 合并文本
	var texts []string
	var sources []string
	totalRelevance := 0.0

	for _, ctx := range contexts {
		texts = append(texts, ctx.Text)
		sources = append(sources, ctx.Source)
		totalRelevance += ctx.Relevance
	}

	return Context{
		Text:      strings.Join(texts, "\n\n"),
		Source:    strings.Join(sources, ", "),
		Relevance: totalRelevance / float64(len(contexts)),
		Metadata: map[string]interface{}{
			"merged_count": len(contexts),
		},
	}
}
