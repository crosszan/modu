package genimagerepo

import (
	"context"

	genimagevo "github.com/crosszan/modu/vo/gen_image_vo"
)

type ImageGenRepo interface {
	Generate(ctx context.Context, req *genimagevo.GenImageRequest) (resp *genimagevo.GenImageResponse, err error)
	Name() string
}
