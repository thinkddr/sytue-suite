// SPDX-License-Identifier: Apache-2.0
//go:build integration

// Package casttest provides only an object-provider boundary for retained-source tests.
package casttest

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type Objects struct {
	mu      sync.Mutex
	Bodies  map[string][]byte
	Gets    int
	BadSize bool
	Types   map[string]string
}

func (o *Objects) Set(key string, raw []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Bodies[key] = bytes.Clone(raw)
}

func (o *Objects) List(_ context.Context, _ tenancy.Tenant, prefix, after string, _ int) (objectstore.ObjectPage, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	keys := []string{}
	for key := range o.Bodies {
		if strings.HasPrefix(key, prefix) && key > after {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	page := objectstore.ObjectPage{Objects: []objectstore.ListedObject{}}
	for _, key := range keys {
		size := int64(len(o.Bodies[key]))
		if o.BadSize {
			size++
		}
		page.Objects = append(page.Objects, objectstore.ListedObject{Key: key, Size: size})
	}
	return page, nil
}

func (o *Objects) Get(_ context.Context, _ tenancy.Tenant, key string) (io.ReadCloser, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Gets++
	raw, ok := o.Bodies[key]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(bytes.Clone(raw))), nil
}

func (o *Objects) GetRange(ctx context.Context, t tenancy.Tenant, key, rango string) (*objectstore.Objeto, error) {
	if rango != "" {
		return nil, objectstore.ErrRangoInvalido
	}
	body, err := o.Get(ctx, t, key)
	if err != nil {
		return nil, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	size := int64(len(o.Bodies[key]))
	return &objectstore.Objeto{Body: body, Size: size, Total: size, ContentType: o.Types[key]}, nil
}
