// SPDX-License-Identifier: Apache-2.0
package rooms

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type destinosDeCuenta struct {
	integradorOAuth
	instancias []string
}

func (d *destinosDeCuenta) IntegrationDestinations(_ context.Context, instancia string) ([]api.RoomsIntegrationDestination, error) {
	d.instancias = append(d.instancias, instancia)
	return []api.RoomsIntegrationDestination{{ID: "destino-" + instancia}}, nil
}

func TestRoomsOtraCuentaUsaSuIdentidadYConservaLasAnteriores(t *testing.T) {
	for _, proveedor := range []string{"slack-rooms", "microsoft-teams-rooms", "google-chat"} {
		t.Run(proveedor, func(t *testing.T) {
			sala, _ := salaDePrueba(t, servicioConDos())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/instances" {
					t.Errorf("petición no esperada %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"instances":[{"id":"anterior","template_key":"` + proveedor + `","name":"Anterior","status":"active","has_secrets":true,"oauth_available":true},{"id":"nueva","template_key":"` + proveedor + `","name":"Nueva","status":"active","has_secrets":true,"oauth_available":true}]}`))
			}))
			defer server.Close()
			sala.CuentasApps = api.NewIntegraciones(server.URL, nil, server.Client())
			altas := 0
			sala.AutorizarAppNueva = func(_ context.Context, plantilla string) (string, error) {
				altas++
				if plantilla != proveedor {
					t.Error("el alta usa otro proveedor")
				}
				return "nueva", nil
			}
			sala.AutorizarApp = func(context.Context, string, string) (string, error) {
				t.Error("se reautoriza la identidad anterior")
				return "anterior", nil
			}
			w := test.NewWindow(sala.P.Objeto())
			t.Cleanup(w.Close)
			destinos := &destinosDeCuenta{}
			c := &conexionRooms{
				sa: sala, api: destinos, ctx: t.Context(), proveedor: proveedor, instancia: "anterior", paso: 1,
				lienzo: w.Canvas(), cuerpo: container.NewStack(), pie: container.NewStack(), pin: ui.NuevaEntrada(""),
				cuentas:     []api.OAuthInstance{{ID: "anterior", TemplateKey: proveedor, Status: "active", HasSecrets: true, OAuthAvailable: true}},
				proveedores: []api.OAuthTemplate{{Key: proveedor, Available: true}},
			}
			c.d = ui.NuevoDialogo("Conectar", c.cuerpo).Con(ui.DialogoLG, c.pie)
			c.autorizar()
			if altas != 1 || c.instancia != "nueva" || c.cuenta() == nil || c.cuenta().ID != "nueva" || c.paso != 2 || len(c.cuentas) != 2 || c.cuentas[0].ID != "anterior" {
				t.Fatalf("el retorno usa otra identidad: %+v", c)
			}
			if !slices.Equal(destinos.instancias, []string{"nueva"}) {
				t.Fatalf("se consultaron destinos de otra cuenta: %v", destinos.instancias)
			}
		})
	}
}

func TestRoomsReintentaLaMismaCuentaSinDuplicarElConsentimientoConfirmado(t *testing.T) {
	for _, denegada := range []bool{false, true} {
		t.Run(map[bool]string{false: "inventario pendiente", true: "consentimiento denegado"}[denegada], func(t *testing.T) {
			sala, _ := salaDePrueba(t, servicioConDos())
			var visible atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/instances" {
					t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
				}
				if visible.Load() {
					_, _ = w.Write([]byte(`{"instances":[{"id":"nueva","template_key":"slack-rooms","status":"active","has_secrets":true,"oauth_available":true}]}`))
				} else {
					_, _ = w.Write([]byte(`{"instances":[]}`))
				}
			}))
			defer server.Close()
			sala.CuentasApps = api.NewIntegraciones(server.URL, nil, server.Client())
			altas, reintentos := 0, 0
			sala.AutorizarAppNueva = func(context.Context, string) (string, error) {
				altas++
				if denegada {
					return "nueva", api.ErrOAuthDenied
				}
				return "nueva", nil
			}
			sala.AutorizarApp = func(_ context.Context, proveedor, id string) (string, error) {
				reintentos++
				if proveedor != "slack-rooms" || id != "nueva" || !denegada {
					t.Error("se reautoriza una cuenta distinta o un consentimiento ya confirmado")
				}
				return "nueva", nil
			}
			w := test.NewWindow(sala.P.Objeto())
			t.Cleanup(w.Close)
			c := &conexionRooms{
				sa: sala, api: &destinosDeCuenta{}, ctx: t.Context(), proveedor: "slack-rooms", paso: 1,
				lienzo: w.Canvas(), cuerpo: container.NewStack(), pie: container.NewStack(), pin: ui.NuevaEntrada(""),
				proveedores: []api.OAuthTemplate{{Key: "slack-rooms", Available: true}},
			}
			c.d = ui.NuevoDialogo("Conectar", c.cuerpo).Con(ui.DialogoLG, c.pie)
			c.autorizar()
			if altas != 1 || c.paso != 1 || c.errorClave == "" || (denegada && c.pendiente != "nueva") || (!denegada && c.esperada != "nueva") {
				t.Fatal("se pierde la tentativa o se anuncia una cuenta todavía no verificada")
			}
			visible.Store(true)
			c.autorizar()
			esperados := 0
			if denegada {
				esperados = 1
			}
			if altas != 1 || reintentos != esperados || c.instancia != "nueva" || c.paso != 2 || c.pendiente != "" || c.esperada != "" {
				t.Fatal("el reintento duplica cuentas o vuelve a pedir un consentimiento confirmado")
			}
		})
	}
}

func TestRoomsNoSeleccionaNiRevocaLaPrimeraCuentaDelProveedor(t *testing.T) {
	c := &conexionRooms{proveedor: "slack-rooms", cuentas: []api.OAuthInstance{
		{ID: "una", TemplateKey: "slack-rooms", Status: "active", HasSecrets: true, OAuthAvailable: true},
		{ID: "dos", TemplateKey: "slack-rooms", Status: "active", HasSecrets: true, OAuthAvailable: true},
	}}
	if c.cuenta() != nil {
		t.Fatal("se selecciona una identidad sin intervención de la persona")
	}
	c.elegirCuenta("dos")
	if c.cuenta() == nil || c.cuenta().ID != "dos" {
		t.Fatal("la identidad elegida se sustituye por la primera")
	}
	c.elegirCuenta("ajena")
	if c.cuenta().ID != "dos" {
		t.Fatal("una identidad fuera del inventario reemplaza la selección")
	}
}

// Visto en rojo en el efímero ARM64 (2026-10-20): Slack → Atrás → Teams
// conservaba la instancia de Slack y el gateway rechazaba el nuevo consentimiento.
func TestCambiarProveedorNoReutilizaLaCuentaAnterior(t *testing.T) {
	p := NuevaPantalla(traductor(t), "es")
	w := test.NewWindow(p.Objeto())
	t.Cleanup(w.Close)
	c := &conexionRooms{
		sa: &Sala{P: p}, lienzo: w.Canvas(), cuerpo: container.NewStack(), pie: container.NewStack(),
		proveedor: "slack-rooms", elegida: "microsoft-teams-rooms", instancia: "slack-account",
		destino: "slack-channel", destinos: []api.RoomsIntegrationDestination{{ID: "slack-channel"}},
		cuentas: []api.OAuthInstance{{ID: "slack-account", TemplateKey: "slack-rooms", Status: "active", HasSecrets: true}},
	}
	c.d = ui.NuevoDialogo("Conectar", c.cuerpo).Con(ui.DialogoLG, c.pie)
	c.avanzar()
	if c.paso != 1 || c.proveedor != "microsoft-teams-rooms" || c.instancia != "" || c.destino != "" || len(c.destinos) != 0 {
		t.Fatalf("el nuevo consentimiento conserva datos de Slack: paso=%d proveedor=%s instancia=%s destino=%s", c.paso, c.proveedor, c.instancia, c.destino)
	}
}

func TestConsentimientoDeRoomsRequiereRegistroRealDelProveedor(t *testing.T) {
	c := &conexionRooms{proveedor: "google-chat", paso: 1}
	if c.valido() {
		t.Fatal("un catálogo sin disponibilidad habilita consentimiento")
	}
	c.proveedores = []api.OAuthTemplate{{Key: "google-chat", Available: true}}
	if !c.valido() {
		t.Fatal("el registro de plataforma no habilita consentimiento")
	}
	c.proveedores = nil
	c.cuentas = []api.OAuthInstance{{ID: "own", TemplateKey: "google-chat", OAuthAvailable: true}}
	if !c.valido() {
		t.Fatal("el registro propio no habilita consentimiento")
	}
	c.cuentas[0].TemplateKey = "slack-rooms"
	if c.valido() {
		t.Fatal("el registro de otra app habilita este proveedor")
	}
}

func TestConsentCannotReviewHiddenDirectChannelOrStaleDestination(t *testing.T) {
	c := &conexionRooms{sa: &Sala{canales: []api.RoomsChannel{{ID: "direct", Kind: "direct"}, {ID: "archived", Kind: "public", Archived: true}, {ID: "channel", Kind: "public"}}}, proveedor: "slack-rooms", proveedores: []api.OAuthTemplate{{Key: "slack-rooms", Available: true}}, paso: 2, canal: "direct", destino: "remote", destinos: []api.RoomsIntegrationDestination{{ID: "remote", Name: "General"}}}
	if c.valido() {
		t.Fatal("hidden direct channel can advance")
	}
	c.canal = "archived"
	if c.valido() {
		t.Fatal("archived channel can advance")
	}
	c.canal = "channel"
	c.destino = "stale"
	if c.valido() {
		t.Fatal("destination outside current provider inventory can advance")
	}
	c.destino = "remote"
	if !c.valido() {
		t.Fatal("actual channel and provider destination cannot advance")
	}
	c.ocupado = true
	if c.valido() {
		t.Fatal("pending request can submit again")
	}
}

func TestWhatsAppConsentRequiresRealNumberAndRegistrationPinOnlyWhenNeeded(t *testing.T) {
	c := &conexionRooms{sa: &Sala{}, proveedor: "whatsapp-business", paso: 2, telefono: "unknown", pin: ui.NuevaEntrada(""), numeros: []api.WhatsAppNumber{{ID: "registered", DisplayPhone: "+34123"}, {ID: "new", RegistrationRequired: true}}}
	if c.valido() {
		t.Fatal("unknown phone number can activate")
	}
	c.telefono = "registered"
	if !c.valido() {
		t.Fatal("registered number unexpectedly asks for PIN")
	}
	c.telefono = "new"
	if c.valido() {
		t.Fatal("new number can skip registration PIN")
	}
	c.pin.PonerTexto("12345x")
	if c.valido() {
		t.Fatal("non-numeric PIN can advance")
	}
	c.pin.PonerTexto("123456")
	if !c.valido() {
		t.Fatal("six-digit registration PIN rejected")
	}
	c.paso = 0
	c.equipo = "foreign"
	c.equipos = []api.RoomsTeam{{ID: "team"}}
	if c.valido() {
		t.Fatal("team outside current session can advance")
	}
}
