// Package util - вспомогательные типы и функции для работы со статическими файлами
package util

import (
	"fmt"
	"github.com/nfnt/resize"
	"image"
	"math"
	"path/filepath"
)

type SizeGroup string // SizeGroup - группа по размеру максимальной стороны изображения

const (
	// SizeGroupIcons - изображения, сжатые, чтобы максимальная сторона не превышала 600 пикселей
	SizeGroupIcons = SizeGroup("icons")
	// SizeGroupStatic - изображения, сжатые, чтобы максимальная сторона не превышала 800 пикселей
	SizeGroupStatic = SizeGroup("static")
	// SizeGroupOriginals - оригиналы (может быть применено сжатие jpeg)
	SizeGroupOriginals = SizeGroup("originals")
	// SizeGroupNonCompressedOriginals - оригиналы без сжатия
	SizeGroupNonCompressedOriginals = SizeGroup("origs")
)

var (
	groupSizes = map[SizeGroup]uint{
		SizeGroupStatic: 800,
		SizeGroupIcons:  600,
	}
)

// ComplexIdToPath - преобразование произвольного идентификатора (состоит из произвольного количества чисел) в путь
func ComplexIdToPath(id []int64) string {
	path := make([]string, len(id))
	for ind, i := range id {
		path[ind] = fmt.Sprintf("%d", i)
	}
	return filepath.Join(path...)
}

// GroupSize - размер группы
func GroupSize(g SizeGroup) uint {
	return groupSizes[g]
}

// SplitComplexId - преобразование произвольного идентификатора (состоит из произвольного количества чисел) в директорию и имя файла
func SplitComplexId(id []int64) (string, int64) {
	subPath := make([]string, len(id)-1)
	for ind, i := range id[:len(id)-1] {
		subPath[ind] = fmt.Sprintf("%d", i)
	}
	return filepath.Join(subPath...), id[len(id)-1]
}

// ResizeImage - генерация изображения с новым размером. size - максимальная длина изображения
func ResizeImage(img image.Image, size uint) image.Image {
	if size == 0 {
		return img
	}
	if img.Bounds().Size().X > img.Bounds().Size().Y {
		if uint(img.Bounds().Size().X) > size {
			return resize.Resize(
				size,
				uint(math.Floor(float64(img.Bounds().Size().Y)*float64(size)/float64(img.Bounds().Size().X))),
				img, resize.Lanczos3)
		} else {
			return img
		}
	} else {
		if uint(img.Bounds().Size().Y) > size {
			return resize.Resize(
				uint(math.Floor(float64(img.Bounds().Size().X)*float64(size)/float64(img.Bounds().Size().Y))),
				size,
				img, resize.Lanczos3)
		} else {
			return img
		}
	}
}
