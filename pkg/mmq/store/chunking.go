package store

import (
	"strings"
	"unicode"
)

// ChunkSize 默认分块大小（字符）
const (
	ChunkSizeChars    = 3200 // ~800 tokens
	ChunkOverlapChars = 480  // 15% overlap
)

// Chunk 文档块
type Chunk struct {
	Text   string // 块内容
	Pos    int    // 在原文档中的字符位置
	Tokens int    // token数量（如果可用）
}

// ChunkDocument 将文档分块
// 使用字符级分块策略，寻找自然分界点（段落>句子>行>单词）
func ChunkDocument(content string, chunkSize, chunkOverlap int) []Chunk {
	if chunkSize == 0 {
		chunkSize = ChunkSizeChars
	}
	if chunkOverlap == 0 {
		chunkOverlap = ChunkOverlapChars
	}

	var chunks []Chunk
	charPos := 0
	contentLen := len(content)

	for charPos < contentLen {
		// 计算当前块的结束位置
		endPos := charPos + chunkSize
		if endPos > contentLen {
			endPos = contentLen
		}

		// 寻找自然分界点
		breakPos := findBreakPoint(content, charPos, endPos)

		// 提取块内容
		chunkText := content[charPos:breakPos]

		// 跳过空块
		if len(strings.TrimSpace(chunkText)) > 0 {
			chunks = append(chunks, Chunk{
				Text: chunkText,
				Pos:  charPos,
			})
		}

		// 移动到下一个块的起始位置（带重叠）
		nextPos := breakPos - chunkOverlap

		// 防止后退（如果重叠太大）
		if nextPos <= charPos {
			nextPos = breakPos
		}

		charPos = nextPos

		// 如果已经到达末尾，退出
		if breakPos >= contentLen {
			break
		}
	}

	return chunks
}

// findBreakPoint 在指定范围内寻找最佳分界点
// 优先级：段落边界 > 句子边界 > 行边界 > 单词边界
func findBreakPoint(content string, start, end int) int {
	if end >= len(content) {
		return len(content)
	}

	// 在最后30%的范围内寻找分界点
	// 这样可以保证每个块至少有70%的内容
	searchStart := start + (end-start)*7/10
	if searchStart < start {
		searchStart = start
	}

	searchRange := content[searchStart:end]

	// 1. 寻找段落边界（两个换行符）
	if idx := strings.LastIndex(searchRange, "\n\n"); idx != -1 {
		return searchStart + idx + 2
	}

	// 2. 寻找句子边界（. ! ? 后跟换行或空格）
	sentenceEndings := []string{".\n", "!\n", "?\n", ". ", "! ", "? "}
	maxIdx := -1
	for _, ending := range sentenceEndings {
		if idx := strings.LastIndex(searchRange, ending); idx > maxIdx {
			maxIdx = idx
		}
	}
	if maxIdx != -1 {
		return searchStart + maxIdx + 1
	}

	// 3. 寻找行边界
	if idx := strings.LastIndex(searchRange, "\n"); idx != -1 {
		return searchStart + idx + 1
	}

	// 4. 寻找单词边界（空格）
	if idx := strings.LastIndex(searchRange, " "); idx != -1 {
		return searchStart + idx + 1
	}

	// 5. 没有找到合适的分界点，强制截断
	return end
}

// EstimateTokens 估算文本的token数量
// 使用简单的启发式规则：平均4个字符≈1个token
func EstimateTokens(text string) int {
	// 移除多余的空白
	text = strings.TrimSpace(text)

	// 统计单词数（对英文）和字符数（对中文）
	words := 0
	inWord := false

	for _, r := range text {
		if unicode.IsSpace(r) {
			if inWord {
				words++
				inWord = false
			}
		} else {
			inWord = true
		}
	}

	if inWord {
		words++
	}

	// 估算：英文单词数 + 中文字符数/2
	// 简化为：总字符数 / 4
	return len(text) / 4
}

// ChunkWithTokenLimit 按token限制分块
// 这是一个简化版本，使用字符估算
func ChunkWithTokenLimit(content string, maxTokens, overlapTokens int) []Chunk {
	// 将token转换为字符（4个字符≈1个token）
	chunkSize := maxTokens * 4
	overlap := overlapTokens * 4

	chunks := ChunkDocument(content, chunkSize, overlap)

	// 为每个块估算token数
	for i := range chunks {
		chunks[i].Tokens = EstimateTokens(chunks[i].Text)
	}

	return chunks
}
