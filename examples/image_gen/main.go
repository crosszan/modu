package main

import (
	"fmt"

	"github.com/crosszan/modu/pkg/nano_banana_pro"
)

const (
	BaseURL = "http://127.0.0.1:8045"
	APIKey  = "sk-5fec10axxxxxxxxx"
)

func main() {
	// 创建客户端，使用默认配置
	client := nano_banana_pro.NewClient(BaseURL, APIKey)

	// 或者使用选项配置
	// client := nano_banana_pro.NewClient(BaseURL, APIKey,
	// 	nano_banana_pro.WithModel("gemini-3-pro-image"),
	// 	nano_banana_pro.WithTimeout(180*time.Second),
	// )

	// 生成图片
	prompt := "a beautiful sunset over mountains, highly detailed, photorealistic"

	fmt.Printf("正在生成图片...\n")
	fmt.Printf("提示词: %s\n", prompt)
	fmt.Printf("模型: %s\n\n", client.GetModel())

	result, err := client.GenerateImage(prompt)
	if err != nil {
		fmt.Printf("生成失败: %v\n", err)
		return
	}

	// 显示使用统计
	fmt.Printf("✓ 生成成功!\n")
	fmt.Printf("模型版本: %s\n", result.ModelVersion)
	fmt.Printf("响应ID: %s\n", result.ResponseID)
	fmt.Printf("Token使用情况:\n")
	fmt.Printf("  - Prompt tokens: %d\n", result.UsageMetadata.PromptTokenCount)
	fmt.Printf("  - Candidates tokens: %d\n", result.UsageMetadata.CandidatesTokenCount)
	fmt.Printf("  - Thoughts tokens: %d\n", result.UsageMetadata.ThoughtsTokenCount)
	fmt.Printf("  - Total tokens: %d\n\n", result.UsageMetadata.TotalTokenCount)

	// 检查是否有图片
	if !result.HasImages() {
		fmt.Println("没有生成任何图片")

		// 检查是否有文本响应
		texts := result.GetTexts()
		for _, text := range texts {
			fmt.Printf("文本内容: %s\n", text)
		}
		return
	}

	// 显示完成原因
	if len(result.Candidates) > 0 {
		fmt.Printf("完成原因: %s\n", result.Candidates[0].FinishReason)
	}

	// 保存所有图片到当前目录
	savedFiles, err := nano_banana_pro.SaveAllImages(result, ".", "generated_image")
	if err != nil {
		fmt.Printf("保存图片失败: %v\n", err)
		return
	}

	for _, file := range savedFiles {
		sizeKB, _ := nano_banana_pro.GetFileSizeKB(file)
		fmt.Printf("✓ 保存成功: %s (%.2f KB)\n", file, sizeKB)
	}

	// 也可以单独处理每张图片
	// images := result.GetImages()
	// for i, img := range images {
	// 	filename := fmt.Sprintf("image_%d.jpg", i+1)
	// 	if err := nano_banana_pro.SaveInlineData(img, filename); err != nil {
	// 		fmt.Printf("保存失败: %v\n", err)
	// 		continue
	// 	}
	// 	fmt.Printf("保存: %s\n", filename)
	// }
}
