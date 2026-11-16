// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

type identityMatrixService struct {
	fakeService
	product  string
	consumer string
	version  storage.IdentityVersion
	calls    int
}

func (s *identityMatrixService) BootstrapMatrixStorageForIdentity(_ context.Context, _, product string, version storage.IdentityVersion) (service.MatrixBootstrap, error) {
	s.product, s.version = product, version
	s.calls++
	return service.MatrixBootstrap{StorageProduct: product, StorageIdentityVersion: version}, nil
}

func (s *identityMatrixService) StartMatrixNativeForConsumer(_ context.Context, _, _, _, product, consumer string, version storage.IdentityVersion) (service.MatrixNativeStart, error) {
	s.consumer = consumer
	s.product, s.version = product, version
	s.calls++
	return service.MatrixNativeStart{NativeConsumer: consumer, Product: product, IdentityVersion: version}, nil
}

func (s *identityMatrixService) FinishMatrixNativeForConsumer(_ context.Context, _, _, _, _, product, consumer string, version storage.IdentityVersion) (string, error) {
	s.consumer = consumer
	s.product, s.version = product, version
	s.calls++
	return "artificial-token", nil
}

func TestMatrixIdentityHTTPBindsExplicitVersionAndPreservesOldDefault(t *testing.T) {
	for _, query := range []string{"", "&identity_version=1", "&identity_version=2"} {
		f := &identityMatrixService{}
		h := &Handlers{svc: f}
		w := httptest.NewRecorder()
		h.MatrixStorageBootstrap(w, req(http.MethodGet, "/?product=drive"+query, "", claims(), nil))
		expected := storage.LegacyIdentity
		if query == "&identity_version=2" {
			expected = storage.IsolatedIdentity
		}
		if w.Code != http.StatusOK || f.calls != 1 || f.product != "drive" || f.version != expected || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("bootstrap changed identity", w.Code, f.version)
		}
	}
	for _, query := range []string{"", "0", "3", "-1", "two", "2&identity_version=1"} {
		f := &identityMatrixService{}
		w := httptest.NewRecorder()
		(&Handlers{svc: f}).MatrixStorageBootstrap(w, req(http.MethodGet, "/?product=drive&identity_version="+query, "", claims(), nil))
		if w.Code != http.StatusBadRequest || f.calls != 0 {
			t.Fatal("invalid or ambiguous version reached service", query, w.Code)
		}
	}
	for _, phase := range []string{"start", "finish"} {
		f := &identityMatrixService{}
		h := &Handlers{svc: f}
		w := httptest.NewRecorder()
		request := req(http.MethodPost, "/", `{"product":"drive","identity_version":2}`, claims(), nil)
		if phase == "start" {
			h.MatrixNativeStart(w, request)
		} else {
			h.MatrixNativeFinish(w, request)
		}
		if f.calls != 1 || f.product != "drive" || f.version != storage.IsolatedIdentity || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("native endpoint lost grant identity", phase)
		}
	}
}

func TestMatrixNativeHTTPPreservesConsumerIndependentlyFromDataScope(t *testing.T) {
	for _, phase := range []string{"start", "finish"} {
		fake := &identityMatrixService{}
		response := httptest.NewRecorder()
		request := req(http.MethodPost, "/", `{"product":"drive","native_consumer":"pdf","identity_version":2}`, claims(), nil)
		handler := &Handlers{svc: fake}
		if phase == "start" {
			handler.MatrixNativeStart(response, request)
		} else {
			handler.MatrixNativeFinish(response, request)
		}
		if response.Code < 200 || response.Code >= 300 || fake.calls != 1 || fake.consumer != "pdf" || fake.product != "drive" || fake.version != storage.IsolatedIdentity {
			t.Fatal("HTTP consumer replaced data product or lost binding", phase, response.Code)
		}
	}
}
