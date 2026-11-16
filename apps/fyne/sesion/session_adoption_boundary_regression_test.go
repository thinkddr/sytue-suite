//go:build linux && !android

// SPDX-License-Identifier: Apache-2.0
package sesion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// This exercises the Suite and experimental local transition gate with a fake
// browser/IdP and an injected save failure. It does not test a real keyring,
// native mobile platform, interprocess fencing or power-loss durability.
func TestSuitePrivateTransitionDoesNotConfirmFailedSessionAdoption(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		name := "confirmed-persistence"
		if failSave {
			name = "ordinary-save-failure-after-token-exchange"
		}
		t.Run(name, func(t *testing.T) {
			aparte(t)
			app := test.NewApp()
			t.Cleanup(app.Quit)
			mobile := appDispositivoFalso{App: app, movil: true, esquema: "sytue-notes"}

			var issued, opened atomic.Int32
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/.well-known/openid-configuration":
					_ = json.NewEncoder(w).Encode(map[string]string{
						"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token",
					})
				case r.Method == http.MethodPost && r.URL.Path == "/token":
					if err := r.ParseForm(); err != nil || r.PostForm.Get("code") != "synthetic-code" || r.PostForm.Get("client_id") != auth.ClientIDMovil || r.PostForm.Get("redirect_uri") != "sytue-notes://auth/callback" || r.PostForm.Get("code_verifier") == "" {
						http.Error(w, "invalid synthetic request", http.StatusBadRequest)
						return
					}
					issued.Add(1)
					claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"bea","tenant":"acme"}`))
					_ = json.NewEncoder(w).Encode(map[string]any{
						"access_token": "header." + claims + ".signature", "id_token": "header." + claims + ".signature", "refresh_token": "synthetic-b", "expires_in": 300,
					})
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			s := Abrir(mobile, "sytue.eu")
			s.oidc.Issuer = server.URL
			s.almacen = s.oidc.Almacen()
			old := auth.Tokens{Issuer: server.URL, Cliente: auth.ClientIDMovil, Subject: "ana", Organizacion: "acme", Refresh: "synthetic-a"}
			if err := s.almacen.Guardar(old); err != nil {
				t.Fatal("seed the original persisted login", err)
			}
			var saves int
			var savedSubject string
			ordinaryFailure := errors.New("synthetic ordinary persistence failure")
			s.Sesion = auth.NewSession(old, nil, func(next auth.Tokens) error {
				saves++
				savedSubject = next.Subject
				if failSave {
					return ordinaryFailure
				}
				return s.almacen.Guardar(next)
			})
			s.oidc.Open = func(raw string) error {
				opened.Add(1)
				authorization, err := url.Parse(raw)
				if err != nil {
					return err
				}
				state := authorization.Query().Get("state")
				callback, err := url.Parse(authorization.Query().Get("redirect_uri"))
				if err != nil || state == "" || callback == nil {
					return errors.New("invalid synthetic authorization")
				}
				q := callback.Query()
				q.Set("code", "synthetic-code")
				q.Set("state", state)
				callback.RawQuery = q.Encode()
				if !s.oidc.Vuelta(callback) {
					return errors.New("the login did not accept its own callback")
				}
				return nil
			}

			scope := storage.Scope{Tenant: "acme", Subject: "ana", Product: "notes"}
			witness, err := s.TestigoTrabajo(t.Context(), scope)
			if err != nil {
				t.Fatal("capture original login witness", err)
			}
			journal, err := Almacen()
			if err != nil {
				t.Fatal(err)
			}
			work := &sessionPrivateWorkFixture{scope: scope, gate: offline.NewWorkGate(t.Context()), journal: journal}
			t.Cleanup(work.gate.Close)
			unregister, err := s.RegistrarTrabajoPrivado(work, witness)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(unregister)
			var cycle uint64
			var phases []FaseTransicionPrivada
			unregisterTransition, err := s.RegistrarPreparadorTransicionPrivada(work, witness, func(_ context.Context, _ func() bool, first EventoTransicionPrivada) (func(EventoTransicionPrivada), error) {
				cycle = first.Ciclo
				phases = append(phases, first.Fase)
				return func(event EventoTransicionPrivada) { phases = append(phases, event.Fase) }, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(unregisterTransition)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			enterErr := s.Entrar(ctx)
			if opened.Load() != 1 || issued.Load() != 1 || saves != 1 || savedSubject != "bea" || cycle == 0 {
				t.Fatalf("did not reach the intended boundary: opens=%d exchanges=%d saves=%d subject=%q cycle=%d err=%v", opened.Load(), issued.Load(), saves, savedSubject, cycle, enterErr)
			}
			persisted, err := s.almacen.Cargar()
			if err != nil {
				t.Fatal("read back persisted login", err)
			}
			state, found := s.EstadoTransicionPrivada(cycle)
			if !found {
				t.Fatal("lost the original transition cycle")
			}
			if !failSave {
				if enterErr != nil || persisted.Subject != "bea" || persisted.Refresh != "synthetic-b" || s.Sesion.Subject() != "bea" || state.Fase != TransicionCompletada {
					t.Fatalf("successful adoption was not confirmed: err=%v persisted=%q memory=%q phase=%q", enterErr, persisted.Subject, s.Sesion.Subject(), state.Fase)
				}
				// Read the marker directly: capturing a new witness could repair or
				// republish it and accidentally make this control pass.
				marker, err := ajustes.LeerAccesoSesionCon(ctx, s.almacen.Identificador())
				if err != nil {
					t.Fatal("read the persisted work marker after adoption", err)
				}
				identity := s.Sesion.SnapshotIdentity()
				if !marker.Activo || marker.Suspendido || marker.Identidad != identity.IdentidadSesion() || marker.Identidad == witness.Acceso.Identidad || marker.Revision == witness.Acceso.Revision {
					t.Fatal("adoption did not persist a distinct active B marker", marker)
				}
				if err := witness.Validar(); !errors.Is(err, auth.ErrTrabajoSesion) {
					t.Fatal("successful interactive adoption did not reject the old work witness", err)
				}
				return
			}
			if persisted.Subject != "ana" || persisted.Refresh != "synthetic-a" {
				t.Fatalf("failure injection changed the original stored login: %q", persisted.Subject)
			}
			// After the token endpoint has succeeded, failure to persist B must not
			// turn the old login into a successful or safely aborted transition. The
			// suite-global status is authoritative here; notifications can be dropped
			// after a witness changes and do not prove a UI received any event.
			if !errors.Is(enterErr, ordinaryFailure) {
				t.Errorf("Suite did not retain the injected persistence failure; err=%v memory=%q phase=%q", enterErr, s.Sesion.Subject(), state.Fase)
			}
			if state.Fase != TransicionEfectoRemotoIncierto && state.Fase != TransicionRemotaConfirmadaAccesoFallido {
				t.Errorf("token exchange succeeded, but transition was classified as %q (notifications %v)", state.Fase, phases)
			}
			for _, phase := range phases {
				if phase == TransicionPreflightAbortado || phase == TransicionCompletada {
					t.Errorf("unsafe transition notification after token exchange: %q", phase)
				}
			}
			// The uncertainty belongs to the session, not to the presenter. Even if
			// the original UI/work unregisters, a second public login must not open
			// a browser or exchange another token before explicit reconciliation.
			unregisterTransition()
			unregister()
			probeCtx, probeCancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer probeCancel()
			probeErr := s.Entrar(probeCtx)
			if !errors.Is(probeErr, ErrTransicionPrivada) || probeCtx.Err() != nil || opened.Load() != 1 || issued.Load() != 1 || saves != 1 {
				t.Errorf("unresolved exchange did not fence public login: err=%v context=%v opens=%d exchanges=%d saves=%d", probeErr, probeCtx.Err(), opened.Load(), issued.Load(), saves)
				return // The unsafe probe may have changed the transition state.
			}
			// Also verify the gate directly with a live context, so an unrelated
			// setup error or an expired deadline cannot count as the fence.
			againCtx, againCancel := context.WithCancel(t.Context())
			defer againCancel()
			again, againErr := s.prepararTransicionPrivada(againCtx, againCancel, nil)
			if again != nil {
				again.fallo(false)
			}
			if !errors.Is(againErr, ErrTransicionPrivada) || againCtx.Err() != nil || again != nil {
				t.Errorf("unresolved exchange did not fence another transition: %v", againErr)
			}
			if opened.Load() != 1 || issued.Load() != 1 || saves != 1 {
				t.Fatal("the safety probe repeated authorization or token exchange")
			}
		})
	}
}
