// SPDX-License-Identifier: Apache-2.0
package objectstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/minio/minio-go/v7"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

var ErrList = errors.New("objectstore: invalid or incomplete scoped object listing")

// ListedObject names an existing object, including historical recipe paths.
// Key is relative to the selected tenant; Size is the provider's stored size.
// Applications must read/hash the full body before retaining a source receipt.
type ListedObject struct {
	Key  string
	Size int64
}

type ObjectPage struct {
	Objects []ListedObject
	Next    string
}

type objectCursor struct{ Tenant, Prefix, After string }

func listPrefix(t tenancy.Tenant, prefix string) (string, error) {
	if !tenancy.ValidSlug(t.Slug) || prefix == "" || len(prefix) > 1024 || !utf8.ValidString(prefix) || strings.ContainsAny(prefix, "\x00\r\n\\") || strings.HasPrefix(prefix, "/") || strings.HasPrefix(prefix, PrefijoCompartido) {
		return "", ErrList
	}
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" || path.Clean(prefix) != prefix || prefix == "." || strings.HasPrefix(prefix, "../") {
		return "", ErrList
	}
	return prefix + "/", nil
}

func decodeObjectCursor(cursor string, t tenancy.Tenant, prefix string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > 4096 {
		return "", ErrList
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", ErrList
	}
	var value objectCursor
	if json.Unmarshal(raw, &value) != nil || value.Tenant != t.Slug || value.Prefix != prefix || !strings.HasPrefix(value.After, tenantKey(t, prefix)) || value.After == tenantKey(t, prefix) || !utf8.ValidString(value.After) || len(value.After) > 2048 {
		return "", ErrList
	}
	return value.After, nil
}

// List enumerates only a tenant's complete subtree through the maintained S3
// adapter. The cursor is opaque and bound to the tenant and normalized prefix.
// Writes must be fenced by the caller when a stable snapshot is required.
// Cancellation, provider errors, malformed/truncated pages or escaped keys
// never return an empty successful page. No object is modified or regenerated.
func (c *Client) List(ctx context.Context, t tenancy.Tenant, prefix, cursor string, limit int) (ObjectPage, error) {
	prefix, err := listPrefix(t, prefix)
	if err != nil || c == nil || limit < 1 || limit > 1000 {
		return ObjectPage{}, ErrList
	}
	if err := ctx.Err(); err != nil {
		return ObjectPage{}, err
	}
	after, err := decodeObjectCursor(cursor, t, prefix)
	if err != nil {
		return ObjectPage{}, err
	}
	full := tenantKey(t, prefix)
	mc, bucket, _ := c.backend(t, prefix)
	page := ObjectPage{Objects: []ListedObject{}}
	last := after
	for object := range mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: full, Recursive: true, StartAfter: after, MaxKeys: limit + 1}) {
		if object.Err != nil {
			return ObjectPage{}, fmt.Errorf("objectstore: listar %q: %w", full, object.Err)
		}
		if object.Size < 0 || !strings.HasPrefix(object.Key, full) || object.Key <= last || object.Key == full || !utf8.ValidString(object.Key) {
			return ObjectPage{}, ErrList
		}
		if len(page.Objects) == limit {
			encoded, err := json.Marshal(objectCursor{Tenant: t.Slug, Prefix: prefix, After: last})
			if err != nil {
				return ObjectPage{}, err
			}
			page.Next = base64.RawURLEncoding.EncodeToString(encoded)
			break
		}
		last = object.Key
		page.Objects = append(page.Objects, ListedObject{Key: strings.TrimPrefix(object.Key, t.StoragePrefix()), Size: object.Size})
	}
	if err := ctx.Err(); err != nil {
		return ObjectPage{}, err
	}
	return page, nil
}
