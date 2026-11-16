// SPDX-License-Identifier: Apache-2.0
package jmap

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

type privateProvisionFixture struct {
	*privateSourceFixture
	provisions int
	address    string
}

func (f *privateProvisionFixture) ProvisionPrivateStorage(_ context.Context, owner, address, name string, quota int64, epoch uuid.UUID, _ func(context.Context, string) (io.ReadCloser, error)) (storage.LegacyMigrationState, error) {
	f.provisions++
	f.address = address
	now := time.Now()
	return storage.LegacyMigrationState{Scope: storage.Scope{Tenant: "acme", Subject: owner, Product: "mail"}, Mode: "frozen", Epoch: epoch.String(), StartedAt: &now, Rows: 7}, nil
}

func TestMailPrivateAccountProvisioningIsExplicitScopedMetadataAndNeverUsesRegistrationEmail(t *testing.T) {
	for _, assigned := range []string{"", "owner@sytue.eu"} {
		t.Run(assigned, func(t *testing.T) {
			source := &privateProvisionFixture{privateSourceFixture: &privateSourceFixture{state: storage.LegacyMigrationState{Scope: mailHTTPSourceScope, Mode: "legacy"}}}
			ingress := &privateIngressFixture{ready: true}
			a := &API{privateSource: source, privateIngress: ingress, cfg: config.Config{MaxMessageBytes: 1 << 20, DefaultMailboxQuotaBytes: 1 << 30}}
			request := sourceRequest(t, http.MethodPost, "/api/v1/private-storage/migration", `{"epoch":"`+mailHTTPSourceEpoch+`","provision":true}`)
			request = request.WithContext(authn.Into(request.Context(), authn.Claims{Subject: mailHTTPSourceScope.Subject, Tenant: mailHTTPSourceScope.Tenant, Email: "external@example.test", Mailbox: assigned}))
			response := httptest.NewRecorder()
			a.beginPrivateStorage(response, request)
			if assigned == "" {
				if response.Code != http.StatusForbidden || source.provisions != 0 || source.begins != 0 || ingress.prepares != 0 {
					t.Fatal("registration email or missing mailbox caused provisioning", response.Code)
				}
			} else if response.Code != http.StatusOK || source.provisions != 1 || source.address != assigned || source.begins != 0 || ingress.prepares != 1 {
				t.Fatal("explicit metadata-only first use failed", response.Code)
			}
		})
	}
}

type missingPrivateAccountStore struct {
	*fakeStore
	ensures int
}

func (f *missingPrivateAccountStore) EnsureAccountConCuota(context.Context, string, string, string, int64, bool) (domain.Account, error) {
	f.ensures++
	return domain.Account{}, nil
}

func TestMailReadDoesNotProvisionANewAccount(t *testing.T) {
	backing := &missingPrivateAccountStore{fakeStore: &fakeStore{bySubErr: domain.ErrNotFound}}
	a := &API{store: backing}
	response := httptest.NewRecorder()
	a.listMailboxes(response, req(http.MethodGet, "/api/v1/mailboxes", "", true, nil))
	if response.Code != http.StatusConflict || backing.ensures != 0 {
		t.Fatal("GET provisioned an account before private first use", response.Code, backing.ensures)
	}
}
