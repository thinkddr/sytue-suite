// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/plantillas"
)

func (h *Handlers) catalogoFuentes(w http.ResponseWriter, r *http.Request) {
	if h.Catalogo == nil {
		httpx.JSON(w, r, http.StatusOK, []any{})
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.Catalogo.Familias)
}

func (h *Handlers) catalogoBiblioteca(w http.ResponseWriter, r *http.Request) {
	if h.Catalogo == nil {
		httpx.JSON(w, r, http.StatusOK, []any{})
		return
	}
	etiqueta := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tag")))
	if etiqueta == "" {
		httpx.JSON(w, r, http.StatusOK, h.Catalogo.Fotos)
		return
	}
	var filtradas []catalogo.Foto
	for _, f := range h.Catalogo.Fotos {
		if slices.Contains(f.Etiquetas, etiqueta) {
			filtradas = append(filtradas, f)
		}
	}
	httpx.JSON(w, r, http.StatusOK, filtradas)
}

func (h *Handlers) catalogoTemas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(plantillas.TemasRaw())
}

func (h *Handlers) catalogoFichero(w http.ResponseWriter, r *http.Request) {
	nombre := chi.URLParam(r, "nombre")
	if nombre == "" || strings.Contains(nombre, "..") || strings.Contains(nombre, "/") {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_file_name"))
		return
	}

	// Servir desde S3 o desde el embed local con cabecera inmutable.
	ext := strings.ToLower(filepath.Ext(nombre))
	var ctype string
	switch ext {
	case ".woff2":
		ctype = "font/woff2"
	case ".jpg", ".jpeg":
		ctype = "image/jpeg"
	case ".png":
		ctype = "image/png"
	default:
		ctype = "application/octet-stream"
	}

	clave, conocido := h.Catalogo.ClaveRecurso(nombre)
	if conocido && h.Objetos != nil {
		t, _ := tenancy.From(r.Context())
		rc, err := h.Objetos.Get(r.Context(), t, clave)
		if err == nil {
			defer rc.Close()
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			_, _ = io.Copy(w, rc)
			return
		}
		if !errors.Is(err, objectstore.ErrNotFound) {
			w.Header().Set("Cache-Control", "no-store")
			httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("object_store_unavailable").Wrap(err))
			return
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	httpx.Fail(w, r, httpx.ErrNotFound.WithCode("catalog_file_not_found"))
}
