// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Both immutable sources and ongoing device ingress resolve ownership from the
// same verified context. Each adapter independently checks its required port;
// a valid identity never implies that an ingress capability is available.
// AuthenticatedStorageScope is the shared identity-only guard for storage
// control operations. It grants no source/ingress capability or readiness.
func AuthenticatedStorageScope(w http.ResponseWriter, r *http.Request, product string, resolve func(context.Context) (PrivateSourceIdentity, bool)) (storage.Scope, bool) {
	return privateStorageHTTPScope(w, r, product, resolve)
}

func privateStorageHTTPScope(w http.ResponseWriter, r *http.Request, product string, resolve func(context.Context) (PrivateSourceIdentity, bool)) (storage.Scope, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if resolve == nil {
		Fail(w, r, ErrUnauthorized)
		return storage.Scope{}, false
	}
	identity, ok := resolve(r.Context())
	tenant, err := tenancy.From(r.Context())
	scope := storage.Scope{Tenant: tenant.Slug, Subject: identity.Subject, Product: product}
	if !ok || err != nil || identity.Tenant != tenant.Slug || !scope.Valid() {
		Fail(w, r, ErrUnauthorized)
		return storage.Scope{}, false
	}
	if identity.Delegado {
		Fail(w, r, ErrForbidden)
		return storage.Scope{}, false
	}
	if err := storage.ValidateRequestScope(r.Header, scope); err != nil {
		failure := ErrBadRequest
		if errors.Is(err, storage.ErrRequestScopeMismatch) {
			failure = ErrConflict.WithCode("storage_identity_changed")
		}
		Fail(w, r, failure)
		return storage.Scope{}, false
	}
	if r.Context().Err() != nil {
		Fail(w, r, ErrUnavailable)
		return storage.Scope{}, false
	}
	return scope, true
}

func privateStorageHTTPFailure(w http.ResponseWriter, r *http.Request, err error, mapping func(error) *Error) {
	failure := ErrInternal
	if mapping != nil {
		failure = mapping(err)
	}
	if failure == nil || failure.Status < 400 || failure.Status > 599 || !storage.Identifier(failure.Code) {
		failure = ErrInternal
	}
	Fail(w, r, NewError(failure.Status, failure.Code, http.StatusText(failure.Status)))
}
