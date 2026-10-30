// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

type cipherBlobService interface {
	PutCipherBlob(context.Context, string, string, int64, io.Reader) (domain.CipherBlob, error)
	CipherBlob(context.Context, string, string) (domain.CipherBlob, error)
	OpenCipherBlob(context.Context, string, string) (domain.CipherBlob, io.ReadCloser, error)
	DeleteCipherBlob(context.Context, string, string) error
}

func (h *Handlers) PutCipherBlob(w http.ResponseWriter, r *http.Request) {
	if h.cipher == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	if r.Header.Get("Content-Type") != "application/octet-stream" || r.Header.Get("Content-Encoding") != "" || r.ContentLength < 0 || r.ContentLength > storage.MaxCipherBlockBytes {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	blob, err := h.cipher.PutCipherBlob(r.Context(), chi.URLParam(r, "product"), chi.URLParam(r, "id"), r.ContentLength, r.Body)
	if err != nil {
		httpx.Fail(w, r, finalizeError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, blob.BlobReceipt)
}

func (h *Handlers) GetCipherBlob(w http.ResponseWriter, r *http.Request) {
	if h.cipher == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	blob, err := h.cipher.CipherBlob(r.Context(), chi.URLParam(r, "product"), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, blob.BlobReceipt)
}

func (h *Handlers) DownloadCipherBlob(w http.ResponseWriter, r *http.Request) {
	if h.cipher == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	blob, body, err := h.cipher.OpenCipherBlob(r.Context(), chi.URLParam(r, "product"), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(blob.SizeBytes, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, io.LimitReader(body, blob.SizeBytes))
}

func (h *Handlers) DeleteCipherBlob(w http.ResponseWriter, r *http.Request) {
	if h.cipher == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	if err := h.cipher.DeleteCipherBlob(r.Context(), chi.URLParam(r, "product"), chi.URLParam(r, "id")); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
