//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	chathttp "git.sytue.eu/sytue/one/services/rooms/internal/http"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type sourceControlMetadataWitness struct {
	snapshot resource.BootstrapSourceSnapshot
	onRead   func()
}

func (s sourceControlMetadataWitness) ReadResourceBootstrapSource(_ context.Context, q resource.BootstrapSourceQuery) (resource.BootstrapSourceSnapshot, error) {
	if q.Resource != s.snapshot.Resource || q.OwnerScope != s.snapshot.OwnerScope {
		return resource.BootstrapSourceSnapshot{}, resource.ErrRevoked
	}
	if s.onRead != nil {
		s.onRead()
	}
	return s.snapshot, nil
}

// The source SQL witness is artificial in THIS test; its original three-product
// SQL/HTTP trajectory is separately tested in Docs. All Rooms Registry, consumed
// consent, directory/SDK, internal HTTP/MAC and Service checks here are actual.
func exerciseResourceSourceControlWithActualRegistry(t testing.TB, ctx context.Context, st *store.Store, cfg config.Config, current resource.VerifiedSession, chain *matrixprooftest.Fixture) {
	t.Helper()
	var identityActive atomic.Bool
	identityActive.Store(true)
	identitySecret := strings.Repeat("c", 64)
	identityServer := httptest.NewServer(httpx.SoloDesdeDentro(identitySecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			UserID string `json:"user_id"`
		}
		if r.URL.Path != "/internal/people/orgs" || httpx.DecodeJSONLimit(w, r, &request, 4096) != nil || request.UserID != current.Scope().Subject {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		orgs := []string{}
		if identityActive.Load() {
			orgs = []string{current.Scope().Tenant}
		}
		json.NewEncoder(w).Encode(map[string]any{"orgs": orgs, "personal": false})
	})))
	defer identityServer.Close()
	cfg.IDURL = identityServer.URL
	cfg.MatrixIdentitySecret = identitySecret
	authorityService := service.New(cfg, st, nil, nil)
	authorityServer := httptest.NewServer(chathttp.NewRouter(chathttp.Deps{Cfg: cfg, Service: authorityService}))
	defer authorityServer.Close()
	client, err := resourcepg.NewSessionAuthorityClient("docs", authorityServer.URL, cfg.ResourceSessionDocsSecret, authorityServer.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := client.Provider()
	if err != nil {
		t.Fatal(err)
	}
	key := resource.Resource{Tenant: current.Scope().Tenant, Product: "docs", Kind: "document", ID: uuid.NewString()}
	snapshot := resource.BootstrapSourceSnapshot{Resource: key, OwnerScope: current.Scope(), SourceEpoch: uuid.NewString(), SourceSeq: "9007199254740993", ACLVersion: "0", History: resource.HistoryFull}
	sourceSecret := strings.Repeat("b", 64)
	var withdrawDuringRead atomic.Bool
	sourceHandler, err := httpx.NewResourceBootstrapSourceHandler(httpx.ResourceBootstrapSourceConfig{ServiceID: "docs", Secret: sourceSecret, Provider: provider, Port: sourceControlMetadataWitness{snapshot: snapshot, onRead: func() {
		if withdrawDuringRead.Load() {
			identityActive.Store(false)
		}
	}}})
	if err != nil {
		t.Fatal(err)
	}
	sourceServer := httptest.NewServer(sourceHandler)
	defer sourceServer.Close()
	cfg.ResourceSourceDocsURL = sourceServer.URL
	cfg.ResourceSourceDocsSecret = sourceSecret
	control := service.New(cfg, st, nil, nil)
	unsigned, err := resource.PrepareBootstrapSourceQuery(ctx, current, key, "!source-control:matrix.test")
	if err != nil {
		t.Fatal(err)
	}
	query := resource.SignedBootstrapSourceQuery{BootstrapSourceQuery: unsigned, Signatures: matrixprooftest.Sign(t, chain.Device, unsigned, chain.Origin.UserID, chain.Origin.DeviceID.String())}
	authenticated := authn.Into(ctx, authn.Claims{Subject: current.Scope().Subject, Tenant: current.Scope().Tenant, Audience: []string{"sytue-docus"}})
	got, err := control.MatrixResourceBootstrapSource(authenticated, current.Scope().Subject, "docs", query)
	if err != nil || got != snapshot {
		t.Fatal("actual Registry/SDK source control rejected current purpose", err)
	}
	wrongAudience := authn.Into(ctx, authn.Claims{Subject: current.Scope().Subject, Tenant: current.Scope().Tenant, Audience: []string{"sytue-mail"}})
	if _, err := control.MatrixResourceBootstrapSource(wrongAudience, current.Scope().Subject, "docs", query); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("Mail OIDC audience borrowed Docs source consent", err)
	}
	foreign := query
	foreign.OwnerScope.Subject = "other"
	if _, err := control.MatrixResourceBootstrapSource(authenticated, current.Scope().Subject, "docs", foreign); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("source body impersonated another owner", err)
	}
	unsignedQuery := query
	unsignedQuery.Signatures = nil
	if _, err := control.MatrixResourceBootstrapSource(authenticated, current.Scope().Subject, "docs", unsignedQuery); err == nil {
		t.Fatal("OIDC/service credential replaced source SDK signature")
	}
	withdrawDuringRead.Store(true)
	if _, err := control.MatrixResourceBootstrapSource(authenticated, current.Scope().Subject, "docs", query); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("owner membership changed during Source read accepted", err)
	}
	withdrawDuringRead.Store(false)
	if _, err := control.MatrixResourceBootstrapSource(authenticated, current.Scope().Subject, "docs", query); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("stale OIDC membership revived source owner", err)
	}
}
