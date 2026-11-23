// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/soltarfuente"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

type noOfficeObjects struct{}

func (noOfficeObjects) Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error) {
	return nil, model.ErrSource
}

func officeSourceError(err error) *httpx.Error {
	switch {
	case privateOfficeWriteBlocked(err):
		return httpx.ErrConflict.WithCode("private_storage_frozen")
	case errors.Is(err, pg.ErrPrivateStorageScope):
		return httpx.ErrConflict.WithCode("storage_identity_changed")
	case errors.Is(err, model.ErrSource), errors.Is(err, pg.ErrPrivateSource), errors.Is(err, pg.ErrPrivateSnapshotEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	default:
		return httpx.ErrInternal
	}
}

func privateOfficeIdentity(ctx context.Context) (httpx.PrivateSourceIdentity, bool) {
	claims, ok := authn.FromContext(ctx)
	return httpx.PrivateSourceIdentity{Tenant: claims.Tenant, Subject: claims.Subject, Delegado: claims.Delegado}, ok
}

func (h *Handler) privateSourcePort() httpx.PrivateSourcePort {
	if port, ok := h.store.(httpx.PrivateSourcePort); ok {
		return port
	}
	st, ok := h.store.(*store.Store)
	if !ok || st == nil {
		return nil
	}
	var objects store.RetainedOfficeObjects = noOfficeObjects{}
	if h.objects != nil {
		objects = h.objects
	}
	port, err := store.NewPrivateSource(st, objects)
	if err != nil {
		return nil
	}
	return port
}

func (h *Handler) mountPrivateSource(pr chi.Router) {
	port := h.privateSourcePort()
	byProduct := make(map[string]*httpx.PrivateSourceHandlers, 3)
	for _, product := range []string{"docs", "sheets", "slides"} {
		handlers, err := httpx.NewPrivateSourceHandlers(httpx.PrivateSourceHTTPConfig{
			Product:      product,
			Port:         port,
			MaxBodyBytes: storage.MaxFileBytes,
			Kind:         model.SourceKind,
			MapError:     officeSourceError,
			Identity:     privateOfficeIdentity,
		})
		if err != nil {
			panic(err)
		}
		byProduct[product] = handlers
	}
	selectHandler := func(r *http.Request) *httpx.PrivateSourceHandlers {
		if scope, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader)); err == nil {
			if handlers, ok := byProduct[scope.Product]; ok {
				return handlers
			}
		}
		return byProduct["docs"]
	}
	state := func(w http.ResponseWriter, r *http.Request) { selectHandler(r).State(w, r) }
	begin := func(w http.ResponseWriter, r *http.Request) { selectHandler(r).Begin(w, r) }
	page := func(w http.ResponseWriter, r *http.Request) { selectHandler(r).Page(w, r) }
	body := func(w http.ResponseWriter, r *http.Request) { selectHandler(r).Body(w, r) }

	pr.Get("/api/v1/private-storage/migration", state)
	pr.Post("/api/v1/private-storage/migration", begin)
	pr.Post("/api/v1/private-storage/migration/freeze", begin)
	pr.Get("/api/v1/private-storage/migration/{epoch}/rows", page)
	pr.Get("/api/v1/private-storage/migration/source", page)
	pr.Get("/api/v1/private-storage/migration/{epoch}/source/{kind}/{source}", body)
	pr.Get("/api/v1/private-storage/migration/source/{kind}/{source}", body)
	h.mountPrivateRelease(pr, port)
}

// mountPrivateRelease es «soltar la fuente» (platform/pkg/soltarfuente) para las
// tres ofimáticas: el producto lo dice la cabecera de ámbito, como en el resto.
func (h *Handler) mountPrivateRelease(pr chi.Router, port httpx.PrivateSourcePort) {
	source, ok := port.(*store.PrivateSource)
	if !ok || source == nil {
		return
	}
	releases := make(map[string]http.HandlerFunc, 3)
	for _, product := range []string{"docs", "sheets", "slides"} {
		handler, err := soltarfuente.Handler(soltarfuente.Config{Product: product, Soltar: source.Release, Avisar: h.hubNotifier.Send})
		if err != nil {
			panic(err)
		}
		releases[product] = handler
	}
	pr.Post("/api/v1"+soltarfuente.Ruta, func(w http.ResponseWriter, r *http.Request) {
		product := "docs"
		if scope, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader)); err == nil {
			if _, ok := releases[scope.Product]; ok {
				product = scope.Product
			}
		}
		releases[product](w, r)
	})
}
