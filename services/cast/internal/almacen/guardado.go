// SPDX-License-Identifier: Apache-2.0
package almacen

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// Guardado imports the existing S3 client and holds the retained SQL owner lock
// around every mutation, including transcoder quality removals and GC prefixes.
type Guardado struct {
	*objectstore.Client
	state *store.Store
}

func ConFuenteRetenida(c *objectstore.Client, state *store.Store) *Guardado {
	if c == nil || state == nil {
		return nil
	}
	return &Guardado{Client: c, state: state}
}

func (g *Guardado) Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts objectstore.PutOptions) error {
	return g.state.WithLegacyVideoWrite(ctx, t.Slug, key, func() error { return g.Client.Put(ctx, t, key, r, size, opts) })
}

func (g *Guardado) Delete(ctx context.Context, t tenancy.Tenant, key string) error {
	return g.state.WithLegacyVideoWrite(ctx, t.Slug, key, func() error { return g.Client.Delete(ctx, t, key) })
}

func (g *Guardado) DeletePrefix(ctx context.Context, t tenancy.Tenant, key string) error {
	return g.state.WithLegacyVideoWrite(ctx, t.Slug, key, func() error { return g.Client.DeletePrefix(ctx, t, key) })
}
