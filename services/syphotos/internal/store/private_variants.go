// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

var ErrPrivateSource = errors.New("photos: retained source changed or is incomplete")

const maxRetainedCatalogBytes = 32 << 20

type retainedVariantStore interface {
	List(context.Context, tenancy.Tenant, string, string, int) (objectstore.ObjectPage, error)
	Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error)
}

type (
	retainedVariant struct {
		Key   string
		Media model.MediaRow
	}
	capturedPhotoSource struct {
		Rows        []model.Row
		Variants    []retainedVariant
		Descriptors []storage.StreamSourceRow
		SHA256      string
		Originals   int64
	}
)

func (c *capturedPhotoSource) clear() {
	for _, row := range c.Rows {
		clear(row.JSON)
	}
	c.Rows = nil
}

func sourceKind(kind string) bool { return kind == "variant" || model.SourceKind(kind) }

func retainedVariantKey(asset, key string) (model.Derivada, error) {
	parts := strings.Split(key, "/")
	if len(parts) != 4 || parts[0] != "a" || parts[1] != asset {
		return model.Derivada{}, ErrPrivateSource
	}
	ext := strings.LastIndexByte(parts[3], '.')
	if ext <= 0 || ext == len(parts[3])-1 {
		return model.Derivada{}, ErrPrivateSource
	}
	id, err := uuid.Parse(asset)
	if err != nil {
		return model.Derivada{}, ErrPrivateSource
	}
	value := model.Derivada{ActivoID: id, Huella: parts[2], Variante: model.Variante(parts[3][:ext]), Formato: parts[3][ext+1:]}
	exact, err := value.Clave()
	if err != nil || exact != key {
		return model.Derivada{}, ErrPrivateSource
	}
	return value, nil
}

func retainedVariantMedia(ctx context.Context, objects retainedVariantStore, scope storage.Scope, asset string, object objectstore.ListedObject) (retainedVariant, error) {
	key, err := retainedVariantKey(asset, object.Key)
	if err != nil || object.Size < 0 || object.Size > storage.MaxFileBytes {
		return retainedVariant{}, ErrPrivateSource
	}
	reader, err := objects.Get(ctx, tenancy.Tenant{Slug: scope.Tenant}, object.Key)
	if err != nil {
		return retainedVariant{}, err
	}
	if reader == nil {
		return retainedVariant{}, ErrPrivateSource
	}
	sum := sha256.New()
	buffer := make([]byte, 64<<10)
	defer clear(buffer)
	n, readErr := io.CopyBuffer(sum, io.LimitReader(reader, object.Size+1), buffer)
	closeErr := reader.Close()
	if err := errors.Join(readErr, closeErr, ctx.Err()); err != nil {
		return retainedVariant{}, err
	}
	if n != object.Size {
		return retainedVariant{}, ErrPrivateSource
	}
	id := sha256.Sum256([]byte(object.Key))
	contentType := mime.TypeByExtension("." + key.Formato)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	media := model.MediaRow{ID: hex.EncodeToString(id[:]), Tenant: scope.Tenant, Owner: scope.Subject, AssetID: asset, Role: "variant", Size: n, SHA256: hex.EncodeToString(sum.Sum(nil)), ContentType: contentType, Fingerprint: key.Huella, Variant: key.Variante, Format: key.Formato}
	if err := media.Validate(); err != nil {
		return retainedVariant{}, err
	}
	return retainedVariant{Key: object.Key, Media: media}, nil
}

func enumerateRetainedVariants(ctx context.Context, objects retainedVariantStore, scope storage.Scope, snapshot *model.Snapshot) ([]retainedVariant, error) {
	if objects == nil || snapshot == nil {
		return nil, ErrPrivateSource
	}
	values := []retainedVariant{}
	for _, original := range snapshot.Originals() {
		found, err := enumerateAssetVariants(ctx, objects, scope, original.AssetID, maxRetainedCatalogBytes/1024-len(values))
		if err != nil {
			return nil, err
		}
		values = append(values, found...)
	}
	return values, nil
}

func enumerateAssetVariants(ctx context.Context, objects retainedVariantStore, scope storage.Scope, asset string, limit int) ([]retainedVariant, error) {
	values := []retainedVariant{}
	cursor, last := "", ""
	seen := map[string]bool{}
	for {
		page, err := objects.List(ctx, tenancy.Tenant{Slug: scope.Tenant}, model.PrefijoDelActivo(uuid.MustParse(asset)), cursor, 1000)
		if err != nil {
			return nil, err
		}
		if len(page.Objects) > 1000 || len(page.Objects) == 0 && page.Next != "" {
			return nil, ErrPrivateSource
		}
		for _, object := range page.Objects {
			if object.Key <= last {
				return nil, ErrPrivateSource
			}
			last = object.Key
			variant, err := retainedVariantMedia(ctx, objects, scope, asset, object)
			if err != nil {
				return nil, err
			}
			values = append(values, variant)
			if len(values) > limit {
				return nil, ErrPrivateSource
			}
		}
		if page.Next == "" {
			break
		}
		if seen[page.Next] || page.Next == cursor {
			return nil, ErrPrivateSource
		}
		seen[page.Next] = true
		cursor = page.Next
	}
	return values, nil
}

// capturePhotoSource requires the caller's exclusive legacy owner lock. SQL
// writers and every S3 writer hold its shared side throughout their operation.
// Historical recipe paths are enumerated and hashed as-is; none are generated.
func capturePhotoSource(ctx context.Context, tx pgx.Tx, objects retainedVariantStore, scope storage.Scope, epoch string) (capturedPhotoSource, error) {
	if !storage.MigrationEpoch(epoch) {
		return capturedPhotoSource{}, ErrPrivateSource
	}
	snapshot, err := readPrivateStorageInventory(ctx, tx, scope, maxRetainedCatalogBytes)
	if err != nil {
		return capturedPhotoSource{}, err
	}
	if err := snapshot.PersonalCompatible(); err != nil {
		return capturedPhotoSource{}, err
	}
	variants, err := enumerateRetainedVariants(ctx, objects, scope, snapshot)
	if err != nil {
		return capturedPhotoSource{}, err
	}
	result := capturedPhotoSource{Rows: snapshot.Rows(), Variants: variants, Originals: int64(len(snapshot.Originals()))}
	var size int64
	for _, row := range result.Rows {
		size += int64(len(row.JSON))
	}
	for _, variant := range variants {
		raw, err := json.Marshal(variant.Media)
		if err != nil {
			result.clear()
			return capturedPhotoSource{}, err
		}
		size += int64(len(raw))
		if size > maxRetainedCatalogBytes {
			clear(raw)
			result.clear()
			return capturedPhotoSource{}, ErrPrivateSource
		}
		result.Rows = append(result.Rows, model.Row{Kind: "media", ID: variant.Media.ID, JSON: raw})
		result.Descriptors = append(result.Descriptors, variant.Media.Descriptor())
	}
	for _, row := range result.Rows {
		sum := sha256.Sum256(row.JSON)
		result.Descriptors = append(result.Descriptors, storage.StreamSourceRow{Kind: row.Kind, ID: row.ID, Size: int64(len(row.JSON)), SHA256: hex.EncodeToString(sum[:])})
	}
	slices.SortFunc(result.Descriptors, func(a, b storage.StreamSourceRow) int {
		if order := strings.Compare(a.Kind, b.Kind); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	sum, err := model.SourceInventoryHash(scope, epoch)
	if err != nil {
		result.clear()
		return capturedPhotoSource{}, err
	}
	for _, row := range result.Descriptors {
		if err := model.AppendSourceInventoryHash(sum, row); err != nil {
			result.clear()
			return capturedPhotoSource{}, err
		}
	}
	result.SHA256 = hex.EncodeToString(sum.Sum(nil))
	return result, nil
}
