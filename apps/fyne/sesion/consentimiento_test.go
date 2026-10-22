// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type appDeConsentimiento struct {
	fyne.App
	abiertas chan *url.URL
}

func (a appDeConsentimiento) Metadata() fyne.AppMetadata {
	m := a.App.Metadata()
	m.Custom = map[string]string{"esquema": "sytue-drive"}
	return m
}

func (a appDeConsentimiento) OpenURL(u *url.URL) error {
	a.abiertas <- u
	return nil
}

func TestAltaOAuthNuevaVuelveALaMismaSuiteYRechazaRetornosAjenos(t *testing.T) {
	for _, cancelar := range []bool{false, true} {
		t.Run(map[bool]string{false: "consentimiento", true: "cancelar"}[cancelar], func(t *testing.T) {
			aparte(t)
			a := appDeConsentimiento{App: test.NewApp(), abiertas: make(chan *url.URL, 1)}
			defer a.Quit()
			s := Abrir(a, "sytue.eu")
			var finales atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /api/v1/templates":
					_, _ = w.Write([]byte(`{"templates":[{"key":"google-drive","enabled":true,"oauth_available":true,"manifest":{"name":"Drive","oauth":{"authorize_url":"https://accounts.google.com/authorize"}}}]}`))
				case "POST /api/v1/instances":
					_, _ = w.Write([]byte(`{"id":"cuenta-nueva"}`))
				case "POST /api/v1/instances/cuenta-nueva/oauth/start":
					_, _ = w.Write([]byte(`{"authorize_url":"https://accounts.google.com/authorize?state=unico"}`))
				case "POST /api/v1/oauth/finish":
					finales.Add(1)
					_, _ = w.Write([]byte(`{"section":"native-storage","instance":"cuenta-nueva"}`))
				default:
					t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			gateway := api.NewIntegraciones(server.URL, nil, server.Client())
			ctx, cancelarFlujo := context.WithCancel(t.Context())
			defer cancelarFlujo()
			type resultado struct {
				id  string
				err error
			}
			fin := make(chan resultado, 1)
			go func() {
				id, err := s.AutorizarAppNueva(ctx, gateway, "google-drive", "native-storage")
				fin <- resultado{id, err}
			}()
			espera, pulso := time.NewTimer(5*time.Second), time.NewTicker(time.Millisecond)
			defer espera.Stop()
			defer pulso.Stop()
			var abierta *url.URL
			for abierta == nil {
				// El driver de pruebas ejecuta las tareas de goroutines sólo en un
				// punto de sincronización del hilo UI, igual que el bucle real.
				fyne.DoAndWait(func() {})
				select {
				case abierta = <-a.abiertas:
				case r := <-fin:
					t.Fatalf("el consentimiento terminó antes de abrir: %v", r.err)
				case <-pulso.C:
				case <-espera.C:
					t.Fatal("el alta desde cero no abrió el navegador")
				}
			}
			if abierta.Host != "accounts.google.com" || abierta.Query().Get("state") != "unico" {
				t.Fatal("se abrió otra autorización")
			}
			ajena, _ := url.Parse("sytue-drive://connect/callback?state=ajeno&code=otro")
			s.recibirConsentimiento(ajena)
			s.mu.Lock()
			pendientes := len(s.consentimiento.llegada)
			s.mu.Unlock()
			if pendientes != 0 || finales.Load() != 0 {
				t.Fatal("un retorno ajeno canjea una cuenta")
			}
			vuelta, _ := url.Parse("sytue-drive://connect/callback?state=unico&code=consentido")
			if cancelar {
				cancelarFlujo()
			} else {
				s.recibirConsentimiento(vuelta)
			}
			select {
			case r := <-fin:
				if cancelar {
					if !errors.Is(r.err, context.Canceled) || r.id != "cuenta-nueva" {
						t.Fatalf("cancelar pierde el borrador de la tentativa: %+v", r)
					}
				} else if r.err != nil || r.id != "cuenta-nueva" || finales.Load() != 1 {
					t.Fatalf("el retorno no finaliza una única cuenta nueva: %+v", r)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("el wizard no recibió el retorno ni la cancelación")
			}
			s.mu.Lock()
			limpio := s.consentimiento == nil
			s.mu.Unlock()
			if !limpio {
				t.Fatal("el alta cerrada bloquea el siguiente wizard")
			}
			if cancelar {
				s.recibirConsentimiento(vuelta)
				if finales.Load() != 0 {
					t.Fatal("un retorno tardío de un wizard cerrado canjea una cuenta")
				}
			}
		})
	}
}

type consentApp struct {
	fyne.App
	metadata fyne.AppMetadata
}

func (a consentApp) Metadata() fyne.AppMetadata { return a.metadata }

type failedConsentBrowser struct {
	consentApp
	err error
}

func (a failedConsentBrowser) OpenURL(*url.URL) error { return a.err }

func TestLinkedConsentPreservesValidatedDraftAfterBrowserFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"instance_id":"pending-account","authorize_url":"https://accounts.google.com/authorize?state=sealed"}`))
	}))
	defer server.Close()
	a := test.NewApp()
	defer a.Quit()
	failure := errors.New("browser unavailable")
	s := Suite{app: failedConsentBrowser{consentApp{App: a, metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": api.Esquema("calendar")}}}, failure}}
	gateway := api.NewIntegraciones(server.URL, nil, server.Client())
	id, err := s.AutorizarCorreoDesde(context.Background(), gateway, "gmail", "", "native-calendar", "mail", "calendar")
	if id != "pending-account" || !errors.Is(err, failure) {
		t.Fatalf("retry lost pending identity or falsely succeeded: %q %v", id, err)
	}
	if s.consentimiento != nil {
		t.Fatal("failed browser kept a pending callback owner")
	}
}

func TestNativeConsentDispatcherIncludesMailAndBindsLocalNonce(t *testing.T) {
	for _, product := range []string{"drive", "rooms", "mail", "one", "calendar", "contacts", "notes"} {
		t.Run(product, func(t *testing.T) {
			scheme := api.Esquema(product)
			flow := &autorizacionApp{start: api.OAuthAuthorization{State: "current-state"}, llegada: make(chan api.OAuthCallback, 1)}
			s := Suite{app: consentApp{metadata: fyne.AppMetadata{Custom: map[string]string{"esquema": scheme}}}, consentimiento: flow}
			for _, raw := range []string{scheme + "://connect/callback?state=old-state&code=stale", scheme + "://connect/callback?state=current-state&state=duplicate&code=x", scheme + "://auth/callback?state=current-state&code=login", "sytue-unregistered://connect/callback?state=current-state&code=foreign"} {
				u, _ := url.Parse(raw)
				s.recibirConsentimiento(u)
				if len(flow.llegada) != 0 {
					t.Fatalf("unrelated callback reached authenticated finish: %s", raw)
				}
			}
			u, _ := url.Parse(scheme + "://connect/callback?state=current-state&code=consented")
			if !s.recibirConsentimiento(u) {
				t.Fatal("registered product callback was not dispatched")
			}
			select {
			case callback := <-flow.llegada:
				if callback.Code != "consented" {
					t.Fatal("wrong callback delivered")
				}
			default:
				t.Fatal("matching native callback did not reach the pending consent")
			}
			s.consentimiento = nil
			if !s.recibirConsentimiento(u) {
				t.Fatal("unsolicited integration callback should stay out of the ID login handler")
			}
			if len(flow.llegada) != 0 {
				t.Fatal("unsolicited callback reused previous flow")
			}
		})
	}
}
