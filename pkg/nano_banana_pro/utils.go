package nano_banana_pro

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SaveBase64Image 保存 Base64 编码的图片到文件
func SaveBase64Image(base64Data, filename string) error {
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("解码 Base64 失败: %w", err)
	}

	if err := os.WriteFile(filename, imageData, 0644); err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}

	return nil
}

// SaveInlineData 保存 InlineData 到文件
func SaveInlineData(data *InlineData, filename string) error {
	return SaveBase64Image(data.Data, filename)
}

// DecodeBase64Image 解码 Base64 图片数据
func DecodeBase64Image(base64Data string) ([]byte, error) {
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("解码 Base64 失败: %w", err)
	}
	return imageData, nil
}

// SaveAllImages 保存响应中的所有图片到指定目录
// 返回保存的文件路径列表
func SaveAllImages(resp *Response, dir, prefix string) ([]string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}

	images := resp.GetImages()
	if len(images) == 0 {
		return nil, nil
	}

	var savedFiles []string
	for i, img := range images {
		ext := getExtensionFromMimeType(img.MimeType)
		filename := filepath.Join(dir, fmt.Sprintf("%s_%d%s", prefix, i+1, ext))

		if err := SaveInlineData(img, filename); err != nil {
			return savedFiles, fmt.Errorf("保存图片 %d 失败: %w", i+1, err)
		}
		savedFiles = append(savedFiles, filename)
	}

	return savedFiles, nil
}

// getExtensionFromMimeType 根据 MIME 类型获取文件扩展名
func getExtensionFromMimeType(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	default:
		return ".jpg"
	}
}

// GetFileSize 获取文件大小（字节）
func GetFileSize(filename string) (int64, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// GetFileSizeKB 获取文件大小（KB）
func GetFileSizeKB(filename string) (float64, error) {
	size, err := GetFileSize(filename)
	if err != nil {
		return 0, err
	}
	return float64(size) / 1024, nil
}
