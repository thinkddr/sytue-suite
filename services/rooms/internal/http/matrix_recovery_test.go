// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

// El selector de cuenta de /matrix/recovery-wrappers: compatible hacia atrás (sin
// parámetros es Rooms) y sin ambigüedad (una versión sin producto no elige nada).
func TestElSelectorDeCuentaDeLosEnvoltorios(t *testing.T) {
	for query, want := range map[string]*service.MatrixRecoveryAccount{
		"":                                  {},
		"?product=drive":                    {Product: "drive"},
		"?product=drive&identity_version=2": {Product: "drive", IdentityVersion: storage.IsolatedIdentity},
		"?identity_version=2":               nil,
		"?product=":                         nil,
		"?product=drive&product=notes":      nil,
		"?product=drive&identity_version=x": nil,
		"?product=drive&identity_version=3": nil,
		"?product=drive&identity_version=2&identity_version=2": nil,
	} {
		got, ok := recoveryAccount(httptest.NewRequest(http.MethodGet, "/matrix/recovery-wrappers"+query, http.NoBody))
		if ok != (want != nil) || want != nil && got != *want {
			t.Errorf("%q = %+v, %v; quiero %+v", query, got, ok, want)
		}
	}
}

// Un cliente viejo que intenta custodiar en claro recibe 410, no un 2xx que le
// haga creer que su clave quedó guardada.
func TestLaCustodiaEnClaroRespondeGone(t *testing.T) {
	var apiErr *httpx.Error
	if err := toAPIError(domain.ErrRecoveryEscrowRetired); !errors.As(err, &apiErr) || apiErr.Status != http.StatusGone {
		t.Fatalf("toAPIError = %v; quiero 410", err)
	}
}
