package fs

import (
	"bytes"
	"context"
	"fmt"
	"github.com/loyal-inform/sdk-go/providers/static/util"
	"image"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
)

type Provider struct {
	root string
}

func NewProvider(root string) *Provider {
	return &Provider{
		root: root,
	}
}

func (p *Provider) SaveImage(_ context.Context, id []int64, imgBytes []byte, sizeGroup []util.SizeGroup, kind string) error {
	var (
		img    image.Image
		format string
		err    error
	)
	if imgBytes != nil {
		img, format, err = image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return err
		}
		if format != "png" && format != "jpeg" {
			return fmt.Errorf("unexpected format: %s", format)
		}
	}
	subPath, realId := util.SplitComplexId(id)
	exists := err == nil
	for _, group := range sizeGroup {
		if exists {
			if err := os.Remove(filepath.Join(p.root, string(group), kind,
				subPath, fmt.Sprintf("%d", realId))); err != nil {
				return err
			}
		}
		toSaveImg := util.ResizeImage(img, util.GroupSize(group))
		if err := os.MkdirAll(filepath.Join(p.root, string(group), kind, subPath), os.ModePerm); err != nil {
			return err
		}
		file, err := os.OpenFile(filepath.Join(p.root, string(group), kind,
			subPath, fmt.Sprintf("%d", realId)),
			os.O_CREATE|os.O_WRONLY, os.ModePerm)
		if err != nil {
			return err
		}
		if format == "png" {
			if err := png.Encode(file, toSaveImg); err != nil {
				return err
			}
		} else if format == "jpeg" {
			if err := jpeg.Encode(file, toSaveImg, &jpeg.Options{Quality: 100}); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unexpected format: %s", format)
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) MoveSet(_ context.Context, oldId, newId []int64, _ int64, sizeGroups []util.SizeGroup, kind string) error {
	for _, g := range sizeGroups {
		if err := os.Rename(filepath.Join(p.root, string(g), kind, util.ComplexIdToPath(oldId)),
			filepath.Join(p.root, string(g), kind, util.ComplexIdToPath(newId))); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) RemoveMultiple(_ context.Context, ids [][]int64, sizeGroup []util.SizeGroup, kind string) error {
	for _, id := range ids {
		for _, g := range sizeGroup {
			if err := os.Remove(filepath.Join(p.root, string(g), kind, util.ComplexIdToPath(id))); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Provider) LoadObject(_ context.Context, path string) ([]byte, error) {
	return ioutil.ReadFile(path)
}

func (p *Provider) PutObject(_ context.Context, path string, data []byte, _ string) error {
	return ioutil.WriteFile(filepath.Join(p.root, path), data, os.ModePerm)
}

func (p *Provider) RemoveObject(_ context.Context, path string) error {
	return os.Remove(filepath.Join(p.root, path))
}

func (p *Provider) MoveObject(_ context.Context, oldPath, newPath string) error {
	return os.Rename(filepath.Join(p.root, oldPath), filepath.Join(p.root, newPath))
}
