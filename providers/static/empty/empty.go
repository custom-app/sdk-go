// Package empty - тестовая реализация провайдера работы со статическими файлами.
//
// Файлы сохраняется в map по ключу (получается аналог s3 в оперативной памяти).
package empty

import (
	"bytes"
	"context"
	"fmt"
	"github.com/custom-app/sdk-go/providers/static/util"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
)

var (
	files = map[string][]byte{}
)

// Provider - структура, имплементирующая интерфейс провайдера
type Provider struct {
	defaultImages map[string][]byte
}

// NewProvider - создание провайдера. defaultImages - изображения по умолчанию
func NewProvider(defaultImages map[string][]byte) *Provider {
	return &Provider{
		defaultImages: defaultImages,
	}
}

// GetFile - получение файла по ключу. Метод наиболее полезен в тестах для проверки, что по нужному пути сохранился нужный файл
func GetFile(key string) []byte {
	return files[key]
}

// SetFiles - замена файлов. Метод наиболее полезен в тестах для замены всех файлов перед тестом
func SetFiles(data map[string][]byte) {
	files = data
}

// SaveImage - реализация метода SaveImage интерфейса Provider
func (p *Provider) SaveImage(ctx context.Context, id []int64, imgBytes []byte, sizeGroup []util.SizeGroup, kind string) error {
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
	for _, group := range sizeGroup {
		if imgBytes == nil {
			files[filepath.Join(string(group), kind, util.ComplexIdToPath(id))] = p.defaultImages[kind]
		} else {
			toSaveImg := util.ResizeImage(img, util.GroupSize(group))
			buf := &bytes.Buffer{}
			if format == "png" {
				if err := png.Encode(buf, toSaveImg); err != nil {
					return err
				}
			} else if format == "jpeg" {
				if err := jpeg.Encode(buf, toSaveImg, &jpeg.Options{Quality: 97}); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("unexpected format: %s", format)
			}
			files[filepath.Join(string(group), kind, util.ComplexIdToPath(id))] = buf.Bytes()
		}
	}
	return nil
}

// MoveSet - реализация метода MoveSet интерфейса Provider
func (p *Provider) MoveSet(ctx context.Context, oldId, newId []int64, qty int64, sizeGroups []util.SizeGroup, kind string) error {
	for _, g := range sizeGroups {
		for i := int64(0); i < qty; i++ {
			before := filepath.Join(string(g), kind, util.ComplexIdToPath(oldId), fmt.Sprintf("%d", i))
			after := filepath.Join(string(g), kind, util.ComplexIdToPath(newId), fmt.Sprintf("%d", i))
			files[after] = files[before]
			delete(files, before)
		}
	}
	return nil
}

// RemoveMultiple - реализация метода RemoveMultiple интерфейса Provider
func (p *Provider) RemoveMultiple(ctx context.Context, ids [][]int64, sizeGroup []util.SizeGroup, kind string) error {
	for _, id := range ids {
		for _, g := range sizeGroup {
			delete(files, filepath.Join(string(g), kind, util.ComplexIdToPath(id)))
		}
	}
	return nil
}

// LoadObject - реализация метода LoadObject интерфейса Provider
func (p *Provider) LoadObject(ctx context.Context, path string) ([]byte, error) {
	return files[path], nil
}

// PutObject - реализация метода PutObject интерфейса Provider
func (p *Provider) PutObject(ctx context.Context, path string, data []byte, contentType string) error {
	files[path] = data
	return nil
}

// RemoveObject - реализация метода RemoveObject интерфейса Provider
func (p *Provider) RemoveObject(ctx context.Context, path string) error {
	delete(files, path)
	return nil
}

// MoveObject - реализация метода MoveObject интерфейса Provider
func (p *Provider) MoveObject(ctx context.Context, oldPath, newPath string) error {
	files[newPath] = files[oldPath]
	delete(files, oldPath)
	return nil
}

// SourceName - реализация метода SourceName интерфейса Provider
func (p *Provider) SourceName() string {
	return "empty"
}
