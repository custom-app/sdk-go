// Package s3 - реализация провайдера работы со статическими файлами с помощью S3-совместимых хранилищ.
//
// Подробнее о s3 - https://cloud.yandex.ru/docs/storage/s3/.
package s3

import (
	"bytes"
	"context"
	"fmt"
	"github.com/custom-app/sdk-go/providers/static/util"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"image"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"path/filepath"
)

var (
	contentTypes = map[string]string{
		"png":  "image/png",
		"jpeg": "image/jpeg",
	}
)

// Provider - структура, имплементирующая интерфейс провайдера
type Provider struct {
	bucket string
	client *minio.Client
}

// NewProvider - создание провайдера. endpoint - адрес бакета, bucket - имя бакета, keyId - id ключа для доступа, secretKey - секретный ключ для доступа
func NewProvider(endpoint, bucket, keyId, secretKey string) (*Provider, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(keyId, secretKey, ""),
		Secure: true,
	})
	if err != nil {
		return nil, err
	}
	return &Provider{
		bucket: bucket,
		client: minioClient,
	}, nil
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
			if _, err := p.client.CopyObject(ctx, minio.CopyDestOptions{
				Bucket: p.bucket,
				Object: filepath.Join(string(group), kind, util.ComplexIdToPath(id)),
			}, minio.CopySrcOptions{
				Bucket: p.bucket,
				Object: filepath.Join(string(group), kind, "default.png"),
			}); err != nil {
				return err
			}
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
			if err := p.PutObject(ctx, filepath.Join(string(group), kind, util.ComplexIdToPath(id)),
				buf.Bytes(), contentTypes[format]); err != nil {
				return err
			}
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
			if _, err := p.client.CopyObject(ctx, minio.CopyDestOptions{
				Bucket: p.bucket,
				Object: after,
			}, minio.CopySrcOptions{
				Bucket: p.bucket,
				Object: before,
			}); err != nil {
				return err
			}
			if err := p.client.RemoveObject(ctx, p.bucket, before, minio.RemoveObjectOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

// RemoveMultiple - реализация метода RemoveMultiple интерфейса Provider
func (p *Provider) RemoveMultiple(ctx context.Context, ids [][]int64, sizeGroup []util.SizeGroup, kind string) error {
	for _, id := range ids {
		for _, g := range sizeGroup {
			if err := p.client.RemoveObject(ctx, p.bucket, filepath.Join(string(g), kind, util.ComplexIdToPath(id)),
				minio.RemoveObjectOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

// LoadObject - реализация метода LoadObject интерфейса Provider
func (p *Provider) LoadObject(ctx context.Context, path string) ([]byte, error) {
	o, err := p.client.GetObject(ctx, p.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	res, err := ioutil.ReadAll(o)
	if err != nil {
		return nil, err
	}
	if err := o.Close(); err != nil {
		return nil, err
	}
	return res, nil
}

// PutObject - реализация метода PutObject интерфейса Provider
func (p *Provider) PutObject(ctx context.Context, path string, data []byte, contentType string) error {
	if _, err := p.client.PutObject(ctx, p.bucket, path,
		bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
			ContentType: contentType,
		}); err != nil {
		return err
	}
	return nil
}

// RemoveObject - реализация метода RemoveObject интерфейса Provider
func (p *Provider) RemoveObject(ctx context.Context, path string) error {
	if err := p.client.RemoveObject(ctx, p.bucket, path, minio.RemoveObjectOptions{}); err != nil {
		return err
	}
	return nil
}

// MoveObject - реализация метода MoveObject интерфейса Provider
func (p *Provider) MoveObject(ctx context.Context, oldPath, newPath string) error {
	if _, err := p.client.CopyObject(ctx, minio.CopyDestOptions{
		Bucket: p.bucket,
		Object: oldPath,
	}, minio.CopySrcOptions{
		Bucket: p.bucket,
		Object: newPath,
	}); err != nil {
		return err
	}
	if err := p.client.RemoveObject(ctx, p.bucket, oldPath, minio.RemoveObjectOptions{}); err != nil {
		return err
	}
	return nil
}

// SourceName - реализация метода SourceName интерфейса Provider
func (p *Provider) SourceName() string {
	return p.bucket
}
