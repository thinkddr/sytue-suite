// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PrivateSourceIdentity comes from the service's verified claims context. The
// resolver accepts no request/header/JSON. authn imports httpx already, so this
// neutral transport type avoids another claims parser or an import cycle.
type PrivateSourceIdentity struct {
	Tenant, Subject string
	Delegado        bool
}

// PrivateSourcePort owns authorization, frozen state/epoch consistency and the
// continuing ingress policy. Begin must require a real domain capability; the
// transport never infers readiness from a historical source or an option flag.
// Open returns an owned reader whose Close safely interrupts a concurrent Read.
type PrivateSourcePort interface {
	State(context.Context, storage.Scope) (storage.AppMigrationState, error)
	Begin(context.Context, storage.Scope, uuid.UUID) (storage.AppMigrationState, error)
	Page(context.Context, storage.Scope, uuid.UUID, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error)
	Open(context.Context, storage.Scope, uuid.UUID, string, string) (storage.StreamSourceRow, io.ReadCloser, error)
}

type PrivateSourceHTTPConfig struct {
	Product      string
	Port         PrivateSourcePort
	Identity     func(context.Context) (PrivateSourceIdentity, bool)
	Kind         func(string) bool
	MapError     func(error) *Error
	MaxBodyBytes int64
}

type PrivateSourceHandlers struct{ cfg PrivateSourceHTTPConfig }

// AuthenticatedScope shares the exact source guard with domain control-plane
// operations. It does not assert completion or grant any encryption key access.
func (h *PrivateSourceHandlers) AuthenticatedScope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	return h.scope(w, r)
}

func NewPrivateSourceHandlers(cfg PrivateSourceHTTPConfig) (*PrivateSourceHandlers, error) {
	if !storage.Product(cfg.Product) || cfg.Identity == nil || cfg.Kind == nil || cfg.MapError == nil || cfg.MaxBodyBytes < 1 || cfg.MaxBodyBytes > storage.MaxFileBytes {
		return nil, errors.New("httpx: invalid private source configuration")
	}
	return &PrivateSourceHandlers{cfg: cfg}, nil
}

func (h *PrivateSourceHandlers) scope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	scope, ok := privateStorageHTTPScope(w, r, h.cfg.Product, h.cfg.Identity)
	if !ok {
		return storage.Scope{}, false
	}
	if h.cfg.Port == nil {
		Fail(w, r, ErrUnavailable)
		return storage.Scope{}, false
	}
	return scope, true
}

// Domain error mappings supply status/code only. Causes, Params, Details and
// custom messages may contain original content or routing, so none leave here.
func (h *PrivateSourceHandlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	privateStorageHTTPFailure(w, r, err, h.cfg.MapError)
}

func privateSourceInvalidResponse(w http.ResponseWriter, r *http.Request) {
	Fail(w, r, ErrInternal.WithCode("private_source_invalid_response"))
}

func (h *PrivateSourceHandlers) State(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	state, err := h.cfg.Port.State(r.Context(), scope)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if state.Validate(scope) != nil {
		privateSourceInvalidResponse(w, r)
		return
	}
	JSON(w, r, http.StatusOK, state)
}

func (h *PrivateSourceHandlers) Begin(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var request struct {
		Epoch string `json:"epoch"`
	}
	if DecodeJSONLimit(w, r, &request, 128) != nil || !storage.MigrationEpoch(request.Epoch) {
		Fail(w, r, ErrBadRequest)
		return
	}
	epoch, err := uuid.Parse(request.Epoch)
	if err != nil {
		Fail(w, r, ErrBadRequest)
		return
	}
	state, err := h.cfg.Port.Begin(r.Context(), scope, epoch)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if state.Validate(scope) != nil || (state.Mode != "frozen" && state.Mode != "private") || state.Epoch != request.Epoch {
		privateSourceInvalidResponse(w, r)
		return
	}
	JSON(w, r, http.StatusOK, state)
}

func privateSourceHTTPEpoch(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "epoch")
	if !storage.MigrationEpoch(raw) {
		Fail(w, r, ErrBadRequest)
		return uuid.Nil, false
	}
	epoch, err := uuid.Parse(raw)
	if err != nil {
		Fail(w, r, ErrBadRequest)
		return uuid.Nil, false
	}
	return epoch, true
}

func (h *PrivateSourceHandlers) Page(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	epoch, ok := privateSourceHTTPEpoch(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	if len(query["after"]) > 1 || len(query["limit"]) > 1 {
		Fail(w, r, ErrBadRequest)
		return
	}
	after, err := storage.DecodeSourceCursor(query.Get("after"))
	if err != nil || after != (storage.LegacySourceCursor{}) && !h.cfg.Kind(after.Kind) {
		Fail(w, r, ErrBadRequest)
		return
	}
	limit := storage.MaxLegacySourceRows
	if values, exists := query["limit"]; exists {
		limit, err = strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > storage.MaxLegacySourceRows {
			Fail(w, r, ErrBadRequest)
			return
		}
	}
	page, err := h.cfg.Port.Page(r.Context(), scope, epoch, after, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if page.Validate(scope, epoch.String(), after) != nil || len(page.Rows) > limit {
		privateSourceInvalidResponse(w, r)
		return
	}
	for _, row := range page.Rows {
		if !h.cfg.Kind(row.Kind) || row.Size > h.cfg.MaxBodyBytes {
			privateSourceInvalidResponse(w, r)
			return
		}
	}
	JSON(w, r, http.StatusOK, page)
}

func (h *PrivateSourceHandlers) Body(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	epoch, ok := privateSourceHTTPEpoch(w, r)
	if !ok {
		return
	}
	kind, id := chi.URLParam(r, "kind"), chi.URLParam(r, "source")
	if !h.cfg.Kind(kind) || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		Fail(w, r, ErrBadRequest)
		return
	}
	row, body, err := h.cfg.Port.Open(r.Context(), scope, epoch, kind, id)
	servePrivateStorageBody(w, r, row, body, err, row.Kind == kind && row.ID == id, h.cfg.MaxBodyBytes, h.cfg.MapError)
}

// Keep the final bounded chunk until length AND digest are verified. A corrupt
// long stream therefore cannot deliver a full declared body with a successful
// HTTP framing, while large MIME never needs to be buffered in memory.
func writePrivateSource(ctx context.Context, w io.Writer, body io.Reader, row storage.StreamSourceRow) (int64, error) {
	left, right := make([]byte, 32<<10), make([]byte, 32<<10)
	defer clear(left)
	defer clear(right)
	reader := io.LimitReader(&privateSourceContextReader{ctx: ctx, reader: body}, row.Size+1)
	sum := sha256.New()
	pending, readErr := io.ReadFull(reader, left)
	total := int64(pending)
	_, _ = sum.Write(left[:pending])
	var written int64
	for {
		if total > row.Size {
			return written, storage.ErrRecord
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
				return written, readErr
			}
			if total != row.Size || hex.EncodeToString(sum.Sum(nil)) != row.SHA256 {
				return written, storage.ErrRecord
			}
			if err := ctx.Err(); err != nil {
				return written, err
			}
			if pending == 0 {
				return written, nil
			}
			n, err := writePrivateSourceChunk(w, left[:pending])
			written += n
			return written, err
		}
		next, nextErr := io.ReadFull(reader, right)
		total += int64(next)
		_, _ = sum.Write(right[:next])
		if total > row.Size {
			return written, storage.ErrRecord
		}
		if next == 0 {
			readErr = nextErr
			continue
		}
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, err := writePrivateSourceChunk(w, left[:pending])
		written += n
		if err != nil {
			return written, err
		}
		left, right = right, left
		pending, readErr = next, nextErr
	}
}

func writePrivateSourceChunk(w io.Writer, payload []byte) (int64, error) {
	n, err := w.Write(payload)
	if err == nil && n != len(payload) {
		err = io.ErrShortWrite
	}
	return int64(n), err
}

type privateSourceContextReader struct {
	ctx    context.Context
	reader io.Reader
	empty  int
}

func (r *privateSourceContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if n == 0 && err == nil {
		r.empty++
		if r.empty >= 100 {
			return 0, io.ErrNoProgress
		}
	} else {
		r.empty = 0
	}
	return n, err
}
