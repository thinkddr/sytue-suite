// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

const (
	ResourceBootstrapSourcePath        = "/internal/resources/bootstrap/source"
	ResourceBootstrapSourceProofDomain = "org.sytue.resource.bootstrap.source.response.v1"
)

type ResourceBootstrapSourceRequest struct {
	Version int                                 `json:"v"`
	Nonce   string                              `json:"nonce"`
	Query   resource.SignedBootstrapSourceQuery `json:"query"`
}
type ResourceBootstrapSourceResponse struct {
	Version  int                              `json:"v"`
	Nonce    string                           `json:"nonce"`
	Snapshot resource.BootstrapSourceSnapshot `json:"snapshot"`
	MAC      string                           `json:"mac"`
}

type ResourceBootstrapSourcePort interface {
	ReadResourceBootstrapSource(context.Context, resource.BootstrapSourceQuery) (resource.BootstrapSourceSnapshot, error)
}
type ResourceBootstrapSourceConfig struct {
	ServiceID string
	Secret    string
	Port      ResourceBootstrapSourcePort
	Provider  *resource.SessionProvider
}

// NewResourceBootstrapSourceHandler applies the existing internal-only shared
// authentication at construction, so mounting the source cannot forget it.
// Missing credentials deny access. The port owns the ORIGINAL frozen SQL/ACL.
func NewResourceBootstrapSourceHandler(cfg ResourceBootstrapSourceConfig) (http.Handler, error) {
	if resource.OriginServiceID(cfg.ServiceID) != cfg.ServiceID || cfg.ServiceID == "" || (cfg.Secret != "" && !ValidMetadataSecret(cfg.Secret)) {
		return nil, errors.New("httpx: invalid resource source service")
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveResourceBootstrapSource(w, r, cfg)
	})
	return SoloDesdeDentro(cfg.Secret)(handler), nil
}

func serveResourceBootstrapSource(w http.ResponseWriter, r *http.Request, cfg ResourceBootstrapSourceConfig) {
	request, ok := decodeResourceBootstrapSourceRequest(w, r, cfg.ServiceID)
	if !ok {
		return
	}
	if cfg.Port == nil || cfg.Provider == nil {
		Fail(w, r, ErrUnavailable)
		return
	}
	query := request.Query.BootstrapSourceQuery
	tenant, err := tenancy.New(query.Resource.Tenant)
	if err != nil {
		Fail(w, r, ErrBadRequest)
		return
	}
	ctx := tenancy.Into(r.Context(), tenant)
	current, err := cfg.Provider.Resolve(ctx, query.OwnerScope, query.Target.DeviceID)
	if err != nil {
		resourceSourceSessionFailure(w, r, err)
		return
	}
	verified, err := resource.VerifyBootstrapSourceQuery(ctx, request.Query, current)
	if err != nil {
		Fail(w, r, ErrForbidden)
		return
	}
	snapshot, err := cfg.Port.ReadResourceBootstrapSource(ctx, query)
	if err != nil {
		resourceSourcePortFailure(w, r, err)
		return
	}
	if !snapshot.Valid() || snapshot.Resource != query.Resource || snapshot.OwnerScope != query.OwnerScope {
		Fail(w, r, ErrUnavailable)
		return
	}
	if err := verified.Revalidate(ctx); err != nil {
		resourceSourceSessionFailure(w, r, err)
		return
	}
	response := ResourceBootstrapSourceResponse{Version: 1, Nonce: request.Nonce, Snapshot: snapshot}
	response.MAC, err = MetadataMAC(cfg.Secret, ResourceBootstrapSourceProofDomain, request, response)
	if err != nil {
		Fail(w, r, ErrUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	JSON(w, r, http.StatusOK, response)
}

func decodeResourceBootstrapSourceRequest(w http.ResponseWriter, r *http.Request, serviceID string) (ResourceBootstrapSourceRequest, bool) {
	var request ResourceBootstrapSourceRequest
	if r.Method != http.MethodPost {
		Fail(w, r, ErrNotFound)
		return request, false
	}
	if DecodeJSONLimit(w, r, &request, 32768) != nil || request.Version != 1 || !ValidMetadataNonce(request.Nonce) {
		Fail(w, r, ErrBadRequest)
		return request, false
	}
	signed := request.Query
	if !signed.Valid() || resource.OriginServiceID(signed.Resource.Product) != serviceID {
		Fail(w, r, ErrBadRequest)
		return request, false
	}
	return request, true
}

func resourceSourcePortFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, resource.ErrInvalid):
		Fail(w, r, ErrBadRequest)
	case errors.Is(err, resource.ErrRevoked):
		Fail(w, r, ErrForbidden)
	default:
		Fail(w, r, ErrUnavailable)
	}
}

func resourceSourceSessionFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, resource.ErrRevoked) || errors.Is(err, resource.ErrInvalid) || errors.Is(err, matrixproof.ErrUnverified) || errors.Is(err, matrixproof.ErrRootEnrollment) {
		Fail(w, r, ErrForbidden)
		return
	}
	Fail(w, r, ErrUnavailable)
}
