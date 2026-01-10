package genimagerepo

import (
	"context"

	"github.com/crosszan/modu/consts/provider"
	genimagevo "github.com/crosszan/modu/vo/gen_image_vo"
)

type geminiImageImpl struct {
}

func NewGeminiImageImpl() ImageGenRepo {
	return &geminiImageImpl{}
}

func (i *geminiImageImpl) Name() string {
	return string(provider.ImageProvider_Gemini)
}

func (i *geminiImageImpl) Generate(ctx context.Context, req *genimagevo.GenImageRequest) (*genimagevo.GenImageResponse, error) {
	return nil, nil
}
