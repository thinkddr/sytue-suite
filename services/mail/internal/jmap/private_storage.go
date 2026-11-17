// SPDX-License-Identifier: Apache-2.0
package jmap

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

type privateSourceStore interface {
	PrivateStorageOwnsMessage(context.Context, string, uuid.UUID) (bool, error)
	PrivateStorageShared(context.Context, string) (bool, error)
	PrivateStorageState(context.Context, string) (storage.LegacyMigrationState, error)
	BeginPrivateStorageMigration(context.Context, string, uuid.UUID, func(context.Context, string) (io.ReadCloser, error)) (storage.LegacyMigrationState, error)
	PrivateStorageSourcePage(context.Context, string, uuid.UUID, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error)
	PrivateStorageSourceBody(context.Context, string, uuid.UUID, string, string) (store.PrivateSourceBody, error)
}

// PrivateIngress is supplied only by the suite's verified-device bridge. A
// historical snapshot does not authorize the server to retain new clear MIME.
// No implementation or boolean environment switch grants this capability here.
type privateAccountProvisioner interface {
	ProvisionPrivateStorage(context.Context, string, string, string, int64, uuid.UUID, func(context.Context, string) (io.ReadCloser, error)) (storage.LegacyMigrationState, error)
}

type PrivateIngress interface {
	PreparePrivateIngress(context.Context, storage.Scope) error
	PrivateIngressReady(context.Context, storage.Scope) (bool, error)
}

func (a *API) WithPrivateIngress(ingress PrivateIngress) *API { a.privateIngress = ingress; return a }

func privateMailRequestScope(r *http.Request) (storage.Scope, error) {
	claims, ok := authn.FromContext(r.Context())
	tenant, err := tenancy.From(r.Context())
	if !ok || err != nil {
		return storage.Scope{}, httpx.ErrUnauthorized
	}
	scope := storage.Scope{Tenant: tenant.Slug, Subject: claims.Subject, Product: "mail"}
	if err = storage.ValidateRequestScope(r.Header, scope); err != nil {
		failure := httpx.ErrBadRequest
		if errors.Is(err, storage.ErrRequestScopeMismatch) {
			failure = httpx.ErrConflict.WithCode("storage_identity_changed")
		}
		return storage.Scope{}, failure
	}
	return scope, nil
}

func (a *API) privateSourceScope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	scope, err := privateMailRequestScope(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return storage.Scope{}, false
	}
	if a.privateSource == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return storage.Scope{}, false
	}
	return scope, true
}

func privateMailError(err error) error {
	err = store.PrivateStorageError(err)
	switch {
	case errors.Is(err, store.ErrPrivateStorageFrozen):
		return httpx.ErrConflict.WithCode("private_storage_migration_required")
	case errors.Is(err, store.ErrPrivateStorageEpoch):
		return httpx.ErrConflict.WithCode("private_storage_snapshot_changed")
	case errors.Is(err, store.ErrPrivateStorageCollaboration):
		return httpx.ErrConflict.WithCode("private_mail_collaboration_required")
	case errors.Is(err, store.ErrPrivateStorageBusy):
		return httpx.ErrConflict.WithCode("private_mail_delivery_in_flight")
	default:
		return mapErr(err)
	}
}

func (a *API) privateStorageState(w http.ResponseWriter, r *http.Request) {
	scope, ok := a.privateSourceScope(w, r)
	if !ok {
		return
	}
	state, err := a.privateSource.PrivateStorageState(r.Context(), scope.Subject)
	if err != nil {
		httpx.Fail(w, r, privateMailError(err))
		return
	}
	a.privateStateResponse(w, r, scope, state)
}

func (a *API) beginPrivateStorage(w http.ResponseWriter, r *http.Request) {
	scope, ok := a.privateSourceScope(w, r)
	if !ok {
		return
	}
	var request struct {
		Epoch     string `json:"epoch"`
		Provision bool   `json:"provision,omitempty"`
	}
	if httpx.DecodeJSONLimit(w, r, &request, 128) != nil || !storage.MigrationEpoch(request.Epoch) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	var provisioner privateAccountProvisioner
	var claims authn.Claims
	var authenticated bool
	if request.Provision {
		provisioner, ok = a.privateSource.(privateAccountProvisioner)
		claims, authenticated = authn.FromContext(r.Context())
		address, parseErr := domain.ParseAddress(claims.Mailbox)
		if !ok {
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
		if !authenticated || claims.Mailbox == "" || parseErr != nil || address.String() != claims.Mailbox {
			httpx.Fail(w, r, httpx.ErrForbidden.WithCode("private_mail_assigned_address_required"))
			return
		}
	}
	shared, err := a.privateSource.PrivateStorageShared(r.Context(), scope.Subject)
	if err != nil {
		httpx.Fail(w, r, privateMailError(err))
		return
	}
	if shared {
		httpx.Fail(w, r, privateMailError(store.ErrPrivateStorageCollaboration))
		return
	}
	if a.privateIngress == nil || a.blobs == nil && !request.Provision {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("private_mail_ingress_required"))
		return
	}
	if err := a.privateIngress.PreparePrivateIngress(r.Context(), scope); err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("private_mail_ingress_required"))
		return
	}
	epoch, _ := uuid.Parse(request.Epoch)
	tenant := tenancy.MustFrom(r.Context())
	open := func(ctx context.Context, key string) (io.ReadCloser, error) {
		if a.blobs == nil {
			return nil, httpx.ErrUnavailable
		}
		return a.blobs.Get(ctx, tenant, key)
	}
	var state storage.LegacyMigrationState
	if request.Provision {
		quota := a.cfg.DefaultMailboxQuotaBytes
		if claims.MailboxBytes > 0 {
			quota = claims.MailboxBytes
		}
		state, err = provisioner.ProvisionPrivateStorage(r.Context(), scope.Subject, claims.Mailbox, claims.Name, quota, epoch, open)
	} else {
		// Import remains strict: only this explicit provisioning flag may create a
		// new metadata-only account; missing historical sources never become empty.
		state, err = a.privateSource.BeginPrivateStorageMigration(r.Context(), scope.Subject, epoch, open)
	}
	if err != nil {
		httpx.Fail(w, r, privateMailError(err))
		return
	}
	a.privateStateResponse(w, r, scope, state)
}

func privateMailEpoch(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	value := chi.URLParam(r, "epoch")
	if !storage.MigrationEpoch(value) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return uuid.Nil, false
	}
	epoch, err := uuid.Parse(value)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return uuid.Nil, false
	}
	return epoch, true
}

func (a *API) privateStoragePage(w http.ResponseWriter, r *http.Request) {
	scope, ok := a.privateSourceScope(w, r)
	if !ok {
		return
	}
	epoch, ok := privateMailEpoch(w, r)
	if !ok {
		return
	}
	after, err := storage.DecodeSourceCursor(r.URL.Query().Get("after"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	page, err := a.privateSource.PrivateStorageSourcePage(r.Context(), scope.Subject, epoch, after, storage.MaxLegacySourceRows)
	if err != nil {
		httpx.Fail(w, r, privateMailError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, page)
}

func (a *API) privateStorageBody(w http.ResponseWriter, r *http.Request) {
	scope, ok := a.privateSourceScope(w, r)
	if !ok {
		return
	}
	epoch, ok := privateMailEpoch(w, r)
	if !ok {
		return
	}
	value, err := a.privateSource.PrivateStorageSourceBody(r.Context(), scope.Subject, epoch, chi.URLParam(r, "kind"), chi.URLParam(r, "source"))
	if err != nil {
		httpx.Fail(w, r, privateMailError(err))
		return
	}
	var body io.ReadCloser
	switch {
	case value.StorageKey == "":
		body, err = value.OpenJSON()
	case a.blobs != nil:
		body, err = a.blobs.Get(r.Context(), tenancy.MustFrom(r.Context()), value.StorageKey)
	default:
		err = httpx.ErrUnavailable
	}
	if err != nil {
		httpx.Fail(w, r, privateMailError(err))
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(value.Descriptor.Size, 10))
	w.Header().Set("X-Sytue-Source-SHA256", value.Descriptor.SHA256)
	_, _ = io.Copy(w, io.LimitReader(body, value.Descriptor.Size+1))
}

func (a *API) privateStateResponse(w http.ResponseWriter, r *http.Request, scope storage.Scope, state storage.LegacyMigrationState) {
	shared, err := a.privateSource.PrivateStorageShared(r.Context(), scope.Subject)
	if err != nil {
		httpx.Fail(w, r, privateMailError(err))
		return
	}
	ready := false
	if a.privateIngress != nil {
		ready, err = a.privateIngress.PrivateIngressReady(r.Context(), scope)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable)
			return
		}
	}
	capabilities := storage.AppMigrationState{Version: 1, LegacyMigrationState: state, Shared: shared, MaxBodyBytes: a.cfg.MaxMessageBytes, PrivateIngressReady: ready}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, capabilities)
}

// The state is looked up for the authenticated PERSONAL account only. Shared
// targets and administration have separate owner/ACL checks; freezing one
// person's history must not block another account's legacy operation.
func (a *API) guardLegacyPrivateMail(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.privateSource == nil || legacyMailControlPath(r.URL.Path) || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		claims, ok := authn.FromContext(r.Context())
		if !ok {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		state, err := a.privateSource.PrivateStorageState(r.Context(), claims.Subject)
		if err != nil {
			httpx.Fail(w, r, privateMailError(err))
			return
		}
		if state.Mode != "legacy" {
			other, err := a.readsAnotherMailbox(r, claims.Subject)
			if err != nil {
				httpx.Fail(w, r, privateMailError(err))
				return
			}
			if !other {
				httpx.Fail(w, r, privateMailError(store.ErrPrivateStorageFrozen))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func legacyMailControlPath(path string) bool {
	if strings.HasPrefix(path, "/api/v1/aliases/") {
		raw := strings.TrimPrefix(path, "/api/v1/aliases/")
		id, err := uuid.Parse(raw)
		return err == nil && id.String() == raw
	}
	for _, prefix := range []string{"/api/v1/private-storage/", "/api/v1/shared-mailboxes", "/api/v1/admin/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	switch path {
	case "/api/v1/quota", "/api/v1/aliases", "/api/v1/delegation":
		return true
	default:
		return false
	}
}

func (a *API) readsAnotherMailbox(r *http.Request, owner string) (bool, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false, nil
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v1/messages/") {
		return false, nil
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		return false, nil
	}
	id, err := uuid.Parse(parts[4])
	if err != nil {
		return false, httpx.ErrBadRequest
	}
	own, err := a.privateSource.PrivateStorageOwnsMessage(r.Context(), owner, id)
	return !own, err
}
