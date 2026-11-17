// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type publicationServiceFixture struct {
	recovery         publication.RecoveryState
	calls            int
	cancelled        bool
	subject, product string
}

func (f *publicationServiceFixture) ReserveMatrixStoragePublication(_ context.Context, subject, product string, _ publication.SignedRequest) (publication.Receipt, error) {
	f.calls++
	f.subject, f.product = subject, product
	return publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: "reserved", RequestSHA256: strings.Repeat("b", 64)}, nil
}

func (f *publicationServiceFixture) ReadMatrixStoragePublication(ctx context.Context, subject, product, _, _ string) (publication.Receipt, error) {
	return f.ReserveMatrixStoragePublication(ctx, subject, product, publication.SignedRequest{})
}

func TestPublicationHTTPUsesAuthenticatedScopeAndNeverCachesMarkers(t *testing.T) {
	for _, operation := range []string{"reserve", "cancel"} {
		t.Run(operation, func(t *testing.T) { publicationHTTPAuthorization(t, operation) })
	}
}

func publicationHTTPAuthorization(t *testing.T, operation string) {
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	for _, fault := range []string{"valid", "delegated", "missing", "foreign-header", "foreign-body", "unsigned", "resources"} {
		t.Run(fault, func(t *testing.T) {
			identity := claims()
			identity.Subject = scope.Subject
			identity.Tenant = scope.Tenant
			if fault == "delegated" {
				identity.Delegado = true
			}
			if fault == "missing" {
				identity = nil
			}
			f := &publicationServiceFixture{}
			h := storagePublicationHandlers{svc: f}
			router := chi.NewRouter()
			if operation == "cancel" {
				router.Post("/matrix/storage/{product}/publications/cancel", h.cancel)
			} else {
				router.Post("/matrix/storage/{product}/publications/reserve", h.reserve)
			}
			sdk := matrixprooftest.New(t, scope, "matrix.test", "DEVICE")
			signed := publication.SignedRequest{Request: publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "storage:mail", Scope: scope, Target: storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}, TransactionID: "opaque-op", CipherSHA256: strings.Repeat("a", 64), AuthorityGeneration: 1}}
			signed.Signatures = matrixprooftest.Sign(t, sdk.Device, signed, sdk.Origin.UserID, sdk.Origin.DeviceID.String())
			if fault == "foreign-body" {
				signed.Scope.Subject = "other"
			}
			if fault == "unsigned" {
				signed.Signatures = nil
			}
			if fault == "resources" {
				signed.Namespace = "resources:mail"
			}
			raw, err := json.Marshal(signed)
			if err != nil {
				t.Fatal(err)
			}
			request := req(http.MethodPost, "/matrix/storage/mail/publications/"+operation, string(raw), identity, nil)
			request = request.WithContext(tenancy.Into(request.Context(), tenancy.Tenant{Slug: scope.Tenant}))
			headerScope := scope
			if fault == "foreign-header" {
				headerScope.Subject = "other"
			}
			header, err := storage.EncodeScopeHeader(headerScope)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set(storage.ScopeHeader, header)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if fault == "valid" {
				if response.Code != http.StatusOK || f.calls != 1 || f.subject != scope.Subject || f.product != scope.Product || f.cancelled != (operation == "cancel") {
					t.Fatal("lost authenticated owner", response.Code, f.calls)
				}
			} else if response.Code < 400 || f.calls != 0 {
				t.Fatal("untrusted scope reached publication service", response.Code, f.calls)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("public marker cached")
			}
		})
	}
}

func (f *publicationServiceFixture) CancelMatrixStoragePublication(ctx context.Context, subject, product string, signed publication.SignedRequest) (publication.Receipt, error) {
	f.cancelled = true
	receipt, err := f.ReserveMatrixStoragePublication(ctx, subject, product, signed)
	receipt.Status = "cancelled"
	return receipt, err
}

func (f *publicationServiceFixture) ReadMatrixStoragePublicationRecovery(_ context.Context, subject, product, room, txn string) (publication.RecoveryState, error) {
	f.calls++
	f.subject, f.product = subject, product
	return f.recovery, nil
}

func TestPublicationHTTPRecoverySelectsOnlyAuthenticatedPublicHistory(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	sdk := matrixprooftest.New(t, scope, "matrix.test", "OLD_DEVICE")
	proof := publication.SignedRequest{Request: publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "storage:mail", Scope: scope, Target: storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "OLD_DEVICE"}, TransactionID: "retained.1", CipherSHA256: strings.Repeat("a", 64), AuthorityGeneration: 1}}
	proof.Signatures = matrixprooftest.Sign(t, sdk.Device, proof, sdk.Origin.UserID, sdk.Origin.DeviceID.String())
	digest, err := matrixproof.RequestDigest(proof.Request)
	if err != nil {
		t.Fatal(err)
	}
	state, err := publication.RecoveryFromStored(proof, publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: strings.Repeat("A", 43)}, Status: "accepted", RequestSHA256: digest, Reference: &storage.Reference{RoomID: proof.Target.RoomID, EventID: "$original"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"valid", "delegated", "missing", "foreign-header", "duplicate-query", "extra-query", "bad-room", "foreign-result"} {
		t.Run(fault, func(t *testing.T) {
			identity := claims()
			identity.Subject = scope.Subject
			identity.Tenant = scope.Tenant
			if fault == "delegated" {
				identity.Delegado = true
			}
			if fault == "missing" {
				identity = nil
			}
			f := &publicationServiceFixture{recovery: state}
			if fault == "foreign-result" {
				f.recovery.RequestSHA256 = strings.Repeat("b", 64)
			}
			router := chi.NewRouter()
			h := storagePublicationHandlers{svc: f}
			router.Get("/matrix/storage/{product}/publications/recovery", h.getRecovery)
			query := url.Values{"room_id": {proof.Target.RoomID}, "transaction_id": {proof.TransactionID}}
			if fault == "duplicate-query" {
				query.Add("transaction_id", "other")
			}
			if fault == "extra-query" {
				query.Set("url", "https://foreign.test")
			}
			if fault == "bad-room" {
				query.Set("room_id", "invalid")
			}
			request := req(http.MethodGet, "/matrix/storage/mail/publications/recovery?"+query.Encode(), "", identity, nil)
			request = request.WithContext(tenancy.Into(request.Context(), tenancy.Tenant{Slug: scope.Tenant}))
			headerScope := scope
			if fault == "foreign-header" {
				headerScope.Subject = "other"
			}
			header, _ := storage.EncodeScopeHeader(headerScope)
			request.Header.Set(storage.ScopeHeader, header)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if fault == "valid" {
				if response.Code != 200 || f.calls != 1 || f.subject != scope.Subject || f.product != scope.Product {
					t.Fatal("lost authenticated selection", response.Code, f.calls)
				}
				if strings.Contains(response.Body.String(), "marker") {
					t.Fatal("read-only recovery exposed send marker")
				}
				selection := publication.RecoverySelection{Scope: scope, RoomID: proof.Target.RoomID, TransactionID: proof.TransactionID}
				hint, err := publication.DecodeRecoveryHint(response.Body.Bytes(), selection)
				if err != nil || hint.Original() != proof.Request {
					t.Fatal("changed retained producer", err)
				}
			} else if response.Code < 400 || (fault != "foreign-result" && f.calls != 0) {
				t.Fatal("untrusted recovery reached service", response.Code, f.calls)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("public history cached")
			}
		})
	}
}
