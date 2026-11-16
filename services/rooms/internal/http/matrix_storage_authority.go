// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type storageAuthorityService interface {
	MatrixStorageAuthority(context.Context, string, string) (matrixproof.Authority, error)
	MatrixRootChallenge(context.Context, string, string, string, storage.PublicationTarget) (matrixproof.RootChallenge, error)
	MatrixRootChangeChallenge(context.Context, string, string, string, string, storage.PublicationTarget) (matrixproof.RootChallenge, error)
	ConfirmMatrixRoot(context.Context, string, string, matrixproof.SignedAction) (matrixproof.Authority, error)
	VerifyMatrixStorageAction(context.Context, string, string, matrixproof.Action, matrixproof.SignedAction) (matrixproof.Authorization, error)
}

type storageAuthorityHandlers struct{ svc storageAuthorityService }

func authorityHTTPError(err error) error {
	switch {
	case errors.Is(err, resource.ErrInvalid):
		return httpx.ErrBadRequest
	case errors.Is(err, resource.ErrRevoked):
		return httpx.ErrForbidden
	case errors.Is(err, matrixproof.ErrRootEnrollment):
		return httpx.ErrConflict.WithCode("storage_root_enrollment_required")
	case errors.Is(err, matrixproof.ErrUnverified):
		return httpx.ErrForbidden
	default:
		return toAPIError(err)
	}
}

func (h storageAuthorityHandlers) scope(w http.ResponseWriter, r *http.Request) (storage.Scope, bool) {
	scope, ok := httpx.AuthenticatedStorageScope(w, r, chi.URLParam(r, "product"), func(ctx context.Context) (httpx.PrivateSourceIdentity, bool) {
		claims, ok := authn.FromContext(ctx)
		return httpx.PrivateSourceIdentity{Subject: claims.Subject, Tenant: claims.Tenant, Delegado: claims.Delegado}, ok
	})
	if !ok {
		return storage.Scope{}, false
	}
	if h.svc == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return storage.Scope{}, false
	}
	return scope, true
}

func (h storageAuthorityHandlers) get(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	result, err := h.svc.MatrixStorageAuthority(r.Context(), scope.Subject, scope.Product)
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h storageAuthorityHandlers) challenge(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var request struct {
		Kind   string                    `json:"kind,omitempty"`
		Epoch  string                    `json:"epoch"`
		Target storage.PublicationTarget `json:"target"`
	}
	if httpx.DecodeJSONLimit(w, r, &request, 4096) != nil || !storage.MigrationEpoch(request.Epoch) || !request.Target.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	var result matrixproof.RootChallenge
	var err error
	switch request.Kind {
	case "", "root.enroll":
		result, err = h.svc.MatrixRootChallenge(r.Context(), scope.Subject, scope.Product, request.Epoch, request.Target)
	case "root.resume", "root.rotate":
		result, err = h.svc.MatrixRootChangeChallenge(r.Context(), scope.Subject, scope.Product, request.Kind, request.Epoch, request.Target)
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, result)
}

func (h storageAuthorityHandlers) confirm(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scope(w, r)
	if !ok {
		return
	}
	var proof matrixproof.SignedAction
	if httpx.DecodeJSONLimit(w, r, &proof, 16<<10) != nil || proof.Scope != scope {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	result, err := h.svc.ConfirmMatrixRoot(r.Context(), scope.Subject, scope.Product, proof)
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

// The route is mounted exclusively under the existing constant-time internal
// shared-secret gate. The body supplies routing, never a trust anchor. Domain
// nonce/expected action remain authoritative in the domain's own transaction.
func (h storageAuthorityHandlers) verifyInternal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request struct {
		Scope    storage.Scope            `json:"scope"`
		Expected matrixproof.Action       `json:"expected"`
		Proof    matrixproof.SignedAction `json:"proof"`
	}
	if httpx.DecodeJSONLimit(w, r, &request, 32<<10) != nil || !request.Scope.Valid() || request.Expected.Scope != request.Scope || request.Proof.Action != request.Expected {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	tenant, err := tenancy.New(request.Scope.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if h.svc == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	result, err := h.svc.VerifyMatrixStorageAction(tenancy.Into(r.Context(), tenant), request.Scope.Subject, request.Scope.Product, request.Expected, request.Proof)
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	if err := result.Validate(request.Expected); err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h storageAuthorityHandlers) stateInternal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request struct {
		Scope storage.Scope `json:"scope"`
	}
	if httpx.DecodeJSONLimit(w, r, &request, 4096) != nil || !request.Scope.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	tenant, err := tenancy.New(request.Scope.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if h.svc == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	result, err := h.svc.MatrixStorageAuthority(tenancy.Into(r.Context(), tenant), request.Scope.Subject, request.Scope.Product)
	if err != nil {
		httpx.Fail(w, r, authorityHTTPError(err))
		return
	}
	if result.Validate(request.Scope) != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	httpx.JSON(w, r, http.StatusOK, result)
}
