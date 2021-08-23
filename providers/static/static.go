package static

import (
	"context"
	"github.com/loyal-inform/sdk-go/providers/static/empty"
	"github.com/loyal-inform/sdk-go/providers/static/util"
)

type Provider interface {
	SaveImage(ctx context.Context, id []int64, imgBytes []byte, sizeGroup []util.SizeGroup, kind string) error
	MoveSet(ctx context.Context, oldId, newId []int64, qty int64, sizeGroups []util.SizeGroup, kind string) error
	RemoveMultiple(ctx context.Context, ids [][]int64, sizeGroup []util.SizeGroup, kind string) error
	LoadObject(ctx context.Context, path string) ([]byte, error)
	PutObject(ctx context.Context, path string, data []byte, contentType string) error
	RemoveObject(ctx context.Context, path string) error
	MoveObject(ctx context.Context, oldPath, newPath string) error
}

var defaultProvider Provider = empty.NewProvider(map[string][]byte{})

func SetDefaultProvider(p Provider) {
	defaultProvider = p
}

func SaveImage(ctx context.Context, id []int64, imgBytes []byte, sizeGroup []util.SizeGroup, kind string) error {
	return defaultProvider.SaveImage(ctx, id, imgBytes, sizeGroup, kind)
}

func MoveSet(ctx context.Context, oldId, newId []int64, qty int64, sizeGroups []util.SizeGroup, kind string) error {
	return defaultProvider.MoveSet(ctx, oldId, newId, qty, sizeGroups, kind)
}

func RemoveMultiple(ctx context.Context, ids [][]int64, sizeGroup []util.SizeGroup, kind string) error {
	return defaultProvider.RemoveMultiple(ctx, ids, sizeGroup, kind)
}

func LoadObject(ctx context.Context, path string) ([]byte, error) {
	return defaultProvider.LoadObject(ctx, path)
}

func PutObject(ctx context.Context, path string, data []byte, contentType string) error {
	return defaultProvider.PutObject(ctx, path, data, contentType)
}

func RemoveObject(ctx context.Context, path string) error {
	return defaultProvider.RemoveObject(ctx, path)
}

func MoveObject(ctx context.Context, oldPath, newPath string) error {
	return defaultProvider.MoveObject(ctx, oldPath, newPath)
}
