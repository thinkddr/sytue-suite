// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Hold a slot until the response has finished, so slow clients cannot retain
// an unbounded number of verified blocks in memory. This is process-wide.
var cipherDownloadSlots = make(chan struct{}, 8)

const cipherDownloadTimeout = 2 * time.Minute

type cipherBlobService interface {
	PutCipherBlob(context.Context, string, string, int64, io.Reader) (domain.CipherBlob, error)
	CipherBlob(context.Context, string, string) (domain.CipherBlob, error)
	OpenCipherBlob(context.Context, string, string) (domain.CipherBlob, io.ReadCloser, error)
	DeleteCipherBlob(context.Context, string, string) error
}

// Check before invoking any service, including reads and deletion. A token may
// refresh into another account while a device still holds the original job.
func cipherRequestScope(w http.ResponseWriter, r *http.Request) bool {
	return storageRequestScope(w, r, chi.URLParam(r, "product"))
}

func storageRequestScope(w http.ResponseWriter, r *http.Request, product string) bool {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrConflict)
		return false
	}
	err = storage.ValidateRequestScope(r.Header, storage.Scope{Tenant: tenant.Slug, Subject: store.OwnerFrom(r.Context()), Product: product})
	if err != nil {
		response := httpx.ErrBadRequest
		if errors.Is(err, storage.ErrRequestScopeMismatch) {
			response = httpx.ErrConflict
		}
		httpx.Fail(w, r, response)
		return false
	}
	return true
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
	if !cipherRequestScope(w, r) {
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
	if !cipherRequestScope(w, r) {
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
	if !cipherRequestScope(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), cipherDownloadTimeout)
	defer cancel()
	select {
	case cipherDownloadSlots <- struct{}{}:
		defer func() { <-cipherDownloadSlots }()
	case <-ctx.Done():
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobProvider))
		return
	}
	if ctx.Err() != nil {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobProvider))
		return
	}
	product, id := chi.URLParam(r, "product"), chi.URLParam(r, "id")
	blob, data, ok := h.readVerifiedCipherBlob(ctx, w, r, product, id)
	if !ok {
		return
	}
	writeVerifiedCipherBlob(ctx, w, r, blob.SizeBytes, data)
}

func (h *Handlers) readVerifiedCipherBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, product, id string) (domain.CipherBlob, []byte, bool) {
	blob, body, err := h.cipher.OpenCipherBlob(ctx, product, id)
	if err != nil {
		if body != nil {
			_ = body.Close()
		}
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrPermission) || errors.Is(err, domain.ErrInvalidName) || errors.Is(err, domain.ErrConflict) || privateStorageAPIError(err) != nil {
			httpx.Fail(w, r, toAPIError(err))
		} else {
			httpx.Fail(w, r, toAPIError(service.ErrCipherBlobProvider))
		}
		return domain.CipherBlob{}, nil, false
	}
	if body == nil {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobProvider))
		return domain.CipherBlob{}, nil, false
	}
	var closeOnce sync.Once
	var closeErr error
	closeBody := func() { closeOnce.Do(func() { closeErr = body.Close() }) }
	stopClose := context.AfterFunc(ctx, closeBody)
	defer func() { stopClose(); closeBody() }()
	if !blob.Valid(product, id) {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobCorrupt))
		return domain.CipherBlob{}, nil, false
	}
	// Reserve one extra byte to detect an appended suffix. Read until EOF, not
	// just the SQL size: the provider's complete object must be verified before
	// any response headers or bytes are committed.
	data := make([]byte, int(blob.SizeBytes)+1)
	n, readErr := readCipherObject(ctx, body, data)
	closeBody()
	if n == 0 && errors.Is(readErr, objectstore.ErrNotFound) && closeErr == nil && ctx.Err() == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return domain.CipherBlob{}, nil, false
	}
	if readErr != nil || closeErr != nil || ctx.Err() != nil {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobProvider))
		return domain.CipherBlob{}, nil, false
	}
	if int64(n) != blob.SizeBytes {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobCorrupt))
		return domain.CipherBlob{}, nil, false
	}
	sum := sha256.Sum256(data[:n])
	if hex.EncodeToString(sum[:]) != blob.SHA256 {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobCorrupt))
		return domain.CipherBlob{}, nil, false
	}
	return blob, data[:n], true
}

func writeVerifiedCipherBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, sizeBytes int64, data []byte) {
	if ctx.Err() != nil {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobProvider))
		return
	}
	// The Drive server has no global write deadline. Do not retain a slot and
	// its buffer forever if a client stops reading. The router's shorter context
	// deadline still wins. A writer that cannot enforce a deadline fails closed.
	controller := http.NewResponseController(w)
	deadline, _ := ctx.Deadline()
	if err := controller.SetWriteDeadline(deadline); err != nil {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobProvider))
		return
	}
	deadlineDone := make(chan struct{})
	stopDeadline := context.AfterFunc(ctx, func() {
		_ = controller.SetWriteDeadline(time.Now())
		close(deadlineDone)
	})
	defer func() {
		if !stopDeadline() {
			<-deadlineDone
		}
		_ = controller.SetWriteDeadline(time.Time{})
	}()
	if ctx.Err() != nil {
		httpx.Fail(w, r, toAPIError(service.ErrCipherBlobProvider))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(sizeBytes, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// readCipherObject treats only EOF as a complete provider read. In particular,
// a reader returning both bytes and an error cannot turn that error into success.
func readCipherObject(ctx context.Context, body io.Reader, dst []byte) (int, error) {
	n, empty := 0, 0
	for n < len(dst) {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		m, err := body.Read(dst[n:])
		n += m
		if err != nil {
			if err == io.EOF {
				return n, nil
			}
			return n, err
		}
		if m == 0 {
			empty++
			if empty >= 100 {
				return n, io.ErrNoProgress
			}
		} else {
			empty = 0
		}
	}
	return n, nil
}

func (h *Handlers) DeleteCipherBlob(w http.ResponseWriter, r *http.Request) {
	if h.cipher == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	if !cipherRequestScope(w, r) {
		return
	}
	if err := h.cipher.DeleteCipherBlob(r.Context(), chi.URLParam(r, "product"), chi.URLParam(r, "id")); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
