package util

import (
	"fmt"
	"github.com/nfnt/resize"
	"image"
	"math"
	"path/filepath"
)

type SizeGroup string

const (
	SizeGroupIcons                  = SizeGroup("icons")
	SizeGroupStatic                 = SizeGroup("static")
	SizeGroupOriginals              = SizeGroup("originals")
	SizeGroupNonCompressedOriginals = SizeGroup("origs")
)

var (
	groupSizes = map[SizeGroup]uint{
		SizeGroupStatic: 800,
		SizeGroupIcons:  600,
	}
)

func ComplexIdToPath(id []int64) string {
	path := make([]string, len(id))
	for ind, i := range id {
		path[ind] = fmt.Sprintf("%d", i)
	}
	return filepath.Join(path...)
}

func GroupSize(g SizeGroup) uint {
	return groupSizes[g]
}

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
