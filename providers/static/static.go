// Package static - провайдер работы со статическими файлами.
//
// Все реализации работают на основании пути к файлу, который формируется из имени группы по размеру(оригиналы, иконки, обычные изображения),
// типа изображения(название сущности) и идентификатора сущности, состоящего из произвольного количества чисел.
//
// Например, вот так выглядит путь до иконки курьера с id 1:
//  /icons/couriers/1
//
// А так - путь до первого(индекс - 0) вложенного файла третьей(индекс - 2) апелляции по заказу с id 10:
//  /static/appeals/10/2/0
package static

import (
	"context"
	"github.com/loyal-inform/sdk-go/providers/static/empty"
	"github.com/loyal-inform/sdk-go/providers/static/util"
)

// Provider - интерфейс для работы с хранилищем статических файлов
type Provider interface {
	SaveImage(ctx context.Context, id []int64, imgBytes []byte, sizeGroup []util.SizeGroup, kind string) error    // сохранение изображения
	MoveSet(ctx context.Context, oldId, newId []int64, qty int64, sizeGroups []util.SizeGroup, kind string) error // перенос директории
	RemoveMultiple(ctx context.Context, ids [][]int64, sizeGroup []util.SizeGroup, kind string) error             // удаление нескольких файлов
	LoadObject(ctx context.Context, path string) ([]byte, error)                                                  // получение файла
	PutObject(ctx context.Context, path string, data []byte, contentType string) error                            // сохранение файла
	RemoveObject(ctx context.Context, path string) error                                                          // удаление файла
	MoveObject(ctx context.Context, oldPath, newPath string) error                                                // перенос файла
	SourceName() string                                                                                           // название источника (фактически используется только для s3)
}

var defaultProvider Provider = empty.NewProvider(map[string][]byte{})

// SetDefaultProvider - установка провайдера по умолчанию
func SetDefaultProvider(p Provider) {
	defaultProvider = p
}

// SaveImage - вызов метода SaveImage провайдера по умолчанию
func SaveImage(ctx context.Context, id []int64, imgBytes []byte, sizeGroup []util.SizeGroup, kind string) error {
	return defaultProvider.SaveImage(ctx, id, imgBytes, sizeGroup, kind)
}

// MoveSet - вызов метода MoveSet провайдера по умолчанию
func MoveSet(ctx context.Context, oldId, newId []int64, qty int64, sizeGroups []util.SizeGroup, kind string) error {
	return defaultProvider.MoveSet(ctx, oldId, newId, qty, sizeGroups, kind)
}

// RemoveMultiple - вызов метода RemoveMultiple провайдера по умолчанию
func RemoveMultiple(ctx context.Context, ids [][]int64, sizeGroup []util.SizeGroup, kind string) error {
	return defaultProvider.RemoveMultiple(ctx, ids, sizeGroup, kind)
}

// LoadObject - вызов метода LoadObject провайдера по умолчанию
func LoadObject(ctx context.Context, path string) ([]byte, error) {
	return defaultProvider.LoadObject(ctx, path)
}

// PutObject - вызов метода PutObject провайдера по умолчанию
func PutObject(ctx context.Context, path string, data []byte, contentType string) error {
	return defaultProvider.PutObject(ctx, path, data, contentType)
}

// RemoveObject - вызов метода RemoveObject провайдера по умолчанию
func RemoveObject(ctx context.Context, path string) error {
	return defaultProvider.RemoveObject(ctx, path)
}

// MoveObject - вызов метода MoveObject провайдера по умолчанию
func MoveObject(ctx context.Context, oldPath, newPath string) error {
	return defaultProvider.MoveObject(ctx, oldPath, newPath)
}

// SourceName - вызов метода SourceName провайдера по умолчанию
func SourceName() string {
	return defaultProvider.SourceName()
}
