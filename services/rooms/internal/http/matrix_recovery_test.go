// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
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

// retireOlderService distingue qué guardado pidió el handler.
type retireOlderService struct {
	*fakeService
	normal, retiro int
}

func (f *retireOlderService) PutMatrixRecoveryWrapper(context.Context, string, service.MatrixRecoveryAccount, recoverywrap.Wrapper) (string, error) {
	f.normal++
	return "id", nil
}

func (f *retireOlderService) PutMatrixRecoveryWrapperRetiringOlder(context.Context, string, service.MatrixRecoveryAccount, recoverywrap.Wrapper) (string, error) {
	f.retiro++
	return "id", nil
}

// `retire_older=1` es el ÚNICO modo de pedir que, con el envoltorio `sytue-id` que el
// dispositivo acaba de comprobar, se retiren los de la ADR 0082 (ADR 0085); cualquier
// otro valor es un error, nunca un guardado normal que el cliente no pidió.
func TestRetirarLosEnvoltoriosViejosSoloSiSePideExplicitamente(t *testing.T) {
	cuerpo := `{"v":1,"kind":"sytue-id","key_id":"K1","credential_id":"","kdf":{"alg":"sytue-id-unlock-hkdf-sha256","hint":"aGludGhpbnRoaW50aGludA=="},"nonce":"bm9uY2Vub25jZW5v","ciphertext":"Y2lwaGVy"}`
	for consulta, quiero := range map[string]struct{ code, normal, retiro int }{
		"":                               {http.StatusOK, 1, 0},
		"?retire_older=1":                {http.StatusOK, 0, 1},
		"?retire_older=0":                {http.StatusBadRequest, 0, 0},
		"?retire_older=1&retire_older=1": {http.StatusBadRequest, 0, 0},
	} {
		f := &retireOlderService{fakeService: &fakeService{}}
		h := &Handlers{svc: f}
		rec := httptest.NewRecorder()
		h.PutMatrixRecoveryWrapper(rec, req(http.MethodPut, "/matrix/recovery-wrappers"+consulta, cuerpo, claims(), nil))
		if rec.Code != quiero.code || f.normal != quiero.normal || f.retiro != quiero.retiro {
			t.Errorf("%q: %d normal=%d retiro=%d; quiero %+v", consulta, rec.Code, f.normal, f.retiro, quiero)
		}
	}
}
