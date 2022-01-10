// Package fs - реализация провайдера работы со статическими файлами с помощью файловой системы.
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

// Provider - структура, имплементирующая интерфейс провайдера
type Provider struct {
	root string
}

// NewProvider - создание провайдера. root - корневая директория
func NewProvider(root string) *Provider {
	return &Provider{
		root: root,
	}
}

// SaveImage - реализация метода SaveImage интерфейса Provider
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
	for _, group := range sizeGroup {
		toSaveImg := util.ResizeImage(img, util.GroupSize(group))
		if err := os.MkdirAll(filepath.Join(p.root, string(group), kind, subPath), os.ModePerm); err != nil {
			return err
		}
		file, err := os.OpenFile(filepath.Join(p.root, string(group), kind,
			subPath, fmt.Sprintf("%d", realId)),
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
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

// MoveSet - реализация метода MoveSet интерфейса Provider
func (p *Provider) MoveSet(_ context.Context, oldId, newId []int64, _ int64, sizeGroups []util.SizeGroup, kind string) error {
	for _, g := range sizeGroups {
		if err := os.Rename(filepath.Join(p.root, string(g), kind, util.ComplexIdToPath(oldId)),
			filepath.Join(p.root, string(g), kind, util.ComplexIdToPath(newId))); err != nil {
			return err
		}
	}
	return nil
}

// RemoveMultiple - реализация метода RemoveMultiple интерфейса Provider
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

// LoadObject - реализация метода LoadObject интерфейса Provider
func (p *Provider) LoadObject(_ context.Context, path string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(p.root, path))
}

// PutObject - реализация метода PutObject интерфейса Provider
func (p *Provider) PutObject(_ context.Context, path string, data []byte, _ string) error {
	path = filepath.Join(p.root, path)
	dir, _ := filepath.Split(path)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, os.ModePerm)
}

// RemoveObject - реализация метода RemoveObject интерфейса Provider
func (p *Provider) RemoveObject(_ context.Context, path string) error {
	return os.Remove(filepath.Join(p.root, path))
}

// MoveObject - реализация метода MoveObject интерфейса Provider
func (p *Provider) MoveObject(_ context.Context, oldPath, newPath string) error {
	newPath = filepath.Join(p.root, newPath)
	dir, _ := filepath.Split(newPath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}
	return os.Rename(filepath.Join(p.root, oldPath), newPath)
}

// SourceName - реализация метода SourceName интерфейса Provider
func (p *Provider) SourceName() string {
	return "file system"
}
