// SPDX-License-Identifier: Apache-2.0
package http

import (
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

const ResourceBootstrapPeerPath = "/api/v1/resources/bootstrap/peer"

// NewResourceBootstrapPeerHandler constructs the authenticated public metadata
// boundary. It is deliberately NOT mounted by Handler.Mount or main until the
// complete verified composition is demonstrated. Existing own-session routes
// remain owner-only. Authority is a fixed service endpoint, never request data.
func NewResourceBootstrapPeerHandler(st *store.Store, authority *resourcepg.SessionAuthorityClient) (http.Handler, error) {
	if st == nil || authority == nil {
		return nil, resource.ErrInvalid
	}
	if _, err := authority.Provider(); err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		claims, ok := authn.FromContext(r.Context())
		tenant, tenantErr := tenancy.From(r.Context())
		if !ok || claims.Subject == "" || tenantErr != nil || claims.Tenant != tenant.Slug {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		var query resource.BootstrapPeerQuery
		if httpx.DecodeJSONLimit(w, r, &query, 32768) != nil || !query.Valid() || resource.OriginServiceID(query.Plan.Source.Resource.Product) != "docs" {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		if _, ok := authn.FirstPartyProductClient(r.Context(), claims.Subject, query.Plan.Source.Resource.Product); !ok || query.Plan.CreatorScope.Subject != claims.Subject || query.Plan.CreatorScope.Tenant != tenant.Slug || query.Plan.Source.OwnerScope != query.Plan.CreatorScope {
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		ctx := r.Context()
		original, err := store.NewResourceBootstrapSource(st, query.Plan.CreatorScope)
		if err != nil {
			resourcePeerFailure(w, r, err)
			return
		}
		capture, err := store.NewResourceBootstrapCaptureStore(st, claims.Subject)
		if err != nil {
			resourcePeerFailure(w, r, err)
			return
		}
		evidence, err := capture.CurrentPeerEvidenceOperation(ctx, query, original, authority)
		if err != nil {
			resourcePeerFailure(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpx.JSON(w, r, http.StatusOK, evidence)
	}), nil
}

func resourcePeerFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, resource.ErrInvalid), errors.Is(err, resource.ErrRevoked):
		httpx.Fail(w, r, httpx.ErrForbidden)
	case errors.Is(err, resource.ErrPending):
		httpx.Fail(w, r, httpx.ErrConflict)
	default:
		httpx.Fail(w, r, httpx.ErrUnavailable)
	}
}
