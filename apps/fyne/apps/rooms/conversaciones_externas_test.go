// SPDX-License-Identifier: Apache-2.0
package rooms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestOtraCuentaPersonalEsNuevaYNoRepiteOAuthAntesDeConfirmarla(t *testing.T) {
	for _, proveedor := range []string{"slack", "microsoft-teams", "google-chat"} {
		t.Run(proveedor, func(t *testing.T) {
			sala, _ := salaDePrueba(t, servicioConDos())
			var mu sync.Mutex
			confirmada, canales := false, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.Method + " " + r.URL.Path {
				case "GET /api/v1/external-conversations/accounts":
					cuentas := api.ExternalChatAccounts{Items: []api.ExternalChatAccount{{InstanceID: "anterior", Provider: proveedor, Name: "Anterior", Connected: true}}, Providers: []api.ExternalChatProvider{{Provider: proveedor, Available: true}}}
					if confirmada {
						cuentas.Items = append(cuentas.Items, api.ExternalChatAccount{InstanceID: "nueva", Provider: proveedor, Name: "Nueva", Connected: true})
					}
					_ = json.NewEncoder(w).Encode(cuentas)
				case "GET /api/v1/external-conversations/nueva/channels":
					canales++
					_, _ = w.Write([]byte(`{"items":[{"id":"general","name":"General"}]}`))
				default:
					t.Errorf("se consulta otra identidad: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			sala.CuentasApps = api.NewIntegraciones(server.URL, nil, server.Client())
			altas := 0
			sala.AutorizarConversaciones = func(_ context.Context, p, instancia string) (string, error) {
				altas++
				if p != proveedor || instancia != "" {
					t.Error("añadir cuenta reautoriza la identidad anterior")
				}
				return "nueva", nil
			}
			w := test.NewWindow(sala.P.Objeto())
			t.Cleanup(w.Close)
			c := &conversacionExterna{
				sa: sala, ctx: t.Context(), lienzo: w.Canvas(), cuerpo: container.NewStack(), pie: container.NewStack(), entrada: ui.NuevaAreaTexto(""), proveedor: proveedor, instancia: "anterior", paso: 1,
				cuentas: api.ExternalChatAccounts{Items: []api.ExternalChatAccount{{InstanceID: "anterior", Provider: proveedor, Name: "Anterior", Connected: true}}, Providers: []api.ExternalChatProvider{{Provider: proveedor, Available: true}}},
			}
			c.d = ui.NuevoDialogo("Conectar", c.cuerpo).Con(ui.DialogoLG, c.pie)
			c.autorizarNueva()
			if altas != 1 || c.instancia != "anterior" || c.esperada != "nueva" || c.paso != 1 || c.errorClave == "" {
				t.Fatal("se sustituye la identidad o se repite el consentimiento antes de que la cuenta sea visible")
			}
			mu.Lock()
			if canales != 0 {
				t.Error("se consultan canales antes de verificar la cuenta nueva")
			}
			confirmada = true
			mu.Unlock()
			c.cargarCuentas("")
			if altas != 1 || c.instancia != "nueva" || c.esperada != "" || c.paso != 2 || c.errorClave != "" || len(c.cuentas.Items) != 2 {
				t.Fatal("la cuenta confirmada no se adopta o pierde las identidades anteriores")
			}
			mu.Lock()
			if canales != 1 {
				t.Error("no se consultó una única vez la cuenta nueva")
			}
			mu.Unlock()
		})
	}
}

func TestMeetNoUsaElPrimerBorradorNiComparteLaURLEntreCuentas(t *testing.T) {
	c := &conversacionExterna{cuentas: api.ExternalChatAccounts{Meetings: []api.ExternalMeeting{
		{Provider: "google-meet", InstanceID: "borrador", Available: true},
		{Provider: "google-meet", InstanceID: "activa", Connected: true, Available: true},
	}}}
	c.reconciliarMeet()
	if c.cuentaMeet != "activa" {
		t.Fatal("un borrador oculta la cuenta activa")
	}
	c.cuentas.Meetings = append(c.cuentas.Meetings, api.ExternalMeeting{Provider: "google-meet", InstanceID: "segunda", Connected: true, Available: true})
	c.cuentaMeet, c.enlaceMeet = "", "https://meet.google.com/privada"
	c.reconciliarMeet()
	if c.cuentaMeet != "" || c.enlaceMeet != "" {
		t.Fatal("varias cuentas se seleccionan solas o muestran un enlace sin identidad")
	}
	c.elegirMeet("activa")
	c.enlaceMeet = "https://meet.google.com/privada"
	c.elegirMeet("segunda")
	if c.cuentaMeet != "segunda" || c.enlaceMeet != "" {
		t.Fatal("cambiar de cuenta conserva una URL privada anterior")
	}
	c.esperadaMeet = "tercera"
	c.reconciliarMeet()
	if c.cuentaMeet != "segunda" || c.esperadaMeet != "tercera" || c.errorMeet == "" {
		t.Fatal("el retorno no verificado selecciona otra cuenta")
	}
	c.cuentas.Meetings = append(c.cuentas.Meetings, api.ExternalMeeting{Provider: "google-meet", InstanceID: "tercera", Connected: true, Available: true})
	c.reconciliarMeet()
	if c.cuentaMeet != "tercera" || c.esperadaMeet != "" || c.errorMeet != "" || len(c.cuentas.Meetings) != 4 {
		t.Fatal("el retorno verificado no conserva las otras cuentas")
	}
}

func TestMeetCreaSoloConLaCuentaElegida(t *testing.T) {
	sala, _ := salaDePrueba(t, servicioConDos())
	var mu sync.Mutex
	var peticiones []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/external-conversations/meeting" {
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["provider"] != "google-meet" {
			t.Error("falta el proveedor de la reunión")
		}
		mu.Lock()
		peticiones = append(peticiones, body["instance_id"])
		mu.Unlock()
		_, _ = w.Write([]byte(`{"url":"https://meet.google.com/abc-defg-hij"}`))
	}))
	defer server.Close()
	sala.CuentasApps = api.NewIntegraciones(server.URL, nil, server.Client())
	w := test.NewWindow(sala.P.Objeto())
	t.Cleanup(w.Close)
	c := &conversacionExterna{
		sa: sala, ctx: t.Context(), lienzo: w.Canvas(), cuerpo: container.NewStack(), pie: container.NewStack(), entrada: ui.NuevaAreaTexto(""), proveedor: "slack",
		cuentas: api.ExternalChatAccounts{Meetings: []api.ExternalMeeting{
			{Provider: "google-meet", InstanceID: "primera", Connected: true, Available: true},
			{Provider: "google-meet", InstanceID: "segunda", Connected: true, Available: true},
		}},
	}
	c.d = ui.NuevoDialogo("Meet", c.cuerpo).Con(ui.DialogoLG, c.pie)
	c.crearMeet()
	mu.Lock()
	ninguna := len(peticiones) == 0
	mu.Unlock()
	if !ninguna {
		t.Fatal("Meet crea una reunión sin elegir entre varias cuentas")
	}
	c.elegirMeet("segunda")
	c.crearMeet()
	mu.Lock()
	defer mu.Unlock()
	if len(peticiones) != 1 || peticiones[0] != "segunda" || c.enlaceMeet == "" || c.errorMeet != "" {
		t.Fatalf("la reunión usa otra identidad: %v", peticiones)
	}
}

func TestExternalHistoryRefreshRemovesDeletedMessagesWithoutLosingOlderPages(t *testing.T) {
	base := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)
	msg := func(id string, minute int) api.ExternalChatMessage {
		return api.ExternalChatMessage{ID: id, Text: id, CreatedAt: base.Add(time.Duration(minute) * time.Minute)}
	}
	previous := []api.ExternalChatMessage{msg("older", 1), msg("deleted", 3), msg("updated", 4)}
	current := []api.ExternalChatMessage{msg("boundary", 2), msg("updated", 4), msg("new", 5)}
	got := mezclarMensajesExternos(previous, current, false, false)
	if len(got) != 4 || got[0].ID != "older" || got[1].ID != "boundary" || got[2].ID != "updated" || got[3].ID != "new" {
		t.Fatalf("recent authoritative window: %+v", got)
	}
	got = mezclarMensajesExternos(got, []api.ExternalChatMessage{msg("oldest", 0)}, true, false)
	if len(got) != 5 || got[0].ID != "oldest" {
		t.Fatalf("older pagination lost history: %+v", got)
	}
	got = mezclarMensajesExternos(got, nil, false, false)
	if len(got) != 0 {
		t.Fatalf("empty authoritative history retained deleted text: %+v", got)
	}
}

func TestSlackThreadRefreshUsesItsAscendingWindow(t *testing.T) {
	base := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)
	msg := func(id string, minute int) api.ExternalChatMessage {
		return api.ExternalChatMessage{ID: id, CreatedAt: base.Add(time.Duration(minute) * time.Minute)}
	}
	got := mezclarMensajesExternos([]api.ExternalChatMessage{msg("parent", 0), msg("deleted-reply", 1), msg("later-page", 4)}, []api.ExternalChatMessage{msg("parent", 0), msg("new-reply", 2)}, false, true)
	if len(got) != 3 || got[0].ID != "parent" || got[1].ID != "new-reply" || got[2].ID != "later-page" {
		t.Fatalf("thread refreshed wrong end: %+v", got)
	}
}

func TestTeamsMeetingUncertainWriteNeverReplaysAndAccountSwitchClearsLink(t *testing.T) {
	sala, _ := salaDePrueba(t, servicioConDos())
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writes++
		var in map[string]any
		if json.NewDecoder(r.Body).Decode(&in) != nil || in["expected_account_id"] != "account-b" || in["instance_id"] != "second" || in["e2ee"] != true {
			t.Error("Teams wrote under the wrong account")
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"external_meeting_unknown"}}`))
	}))
	defer server.Close()
	sala.CuentasApps = api.NewIntegraciones(server.URL, nil, server.Client())
	win := test.NewWindow(sala.P.Objeto())
	t.Cleanup(win.Close)
	c := &conversacionExterna{sa: sala, ctx: t.Context(), lienzo: win.Canvas(), cuerpo: container.NewStack(), pie: container.NewStack(), entrada: ui.NuevaAreaTexto(""), proveedor: "slack", proveedorMeet: "microsoft-teams", cuentas: api.ExternalChatAccounts{Meetings: []api.ExternalMeeting{
		{Provider: "microsoft-teams", InstanceID: "first", AccountID: "account-a", Connected: true, Available: true},
		{Provider: "microsoft-teams", InstanceID: "second", AccountID: "account-b", Connected: true, Available: true},
	}}}
	c.d = ui.NuevoDialogo("Teams", c.cuerpo).Con(ui.DialogoLG, c.pie)
	c.elegirMeet("first")
	c.enlaceMeet, c.envioMeet = "https://teams.microsoft.com/meet/old", "old-txn"
	c.elegirMeet("second")
	if c.enlaceMeet != "" || c.envioMeet != "" {
		t.Fatal("another account retained an old meeting")
	}
	c.crearMeet()
	c.crearMeet()
	c.elegirMeet("first")
	c.elegirMeet("second")
	c.crearMeet()
	if writes != 1 || c.enlaceMeet != "" || !c.inciertasMeet["second"] {
		t.Fatalf("uncertain POST replayed: %d", writes)
	}
	c.cambiarProveedorMeet("google-meet")
	if c.enlaceMeet != "" || c.cuentaMeet != "" {
		t.Fatal("provider change retained a protected link")
	}
}

type emisorTeamsPrueba struct {
	Servicio
	transacciones []string
	enlaces       []string
}

func (e *emisorTeamsPrueba) PostWithTxn(_ context.Context, _, enlace, txn string) (api.RoomsMessage, error) {
	e.transacciones = append(e.transacciones, txn)
	e.enlaces = append(e.enlaces, enlace)
	if len(e.transacciones) == 1 {
		return api.RoomsMessage{}, context.DeadlineExceeded
	}
	return api.RoomsMessage{}, nil
}

func TestTeamsMeetingShareRetainsEncryptedTransactionAndNeverRecreates(t *testing.T) {
	sala, _ := salaDePrueba(t, servicioConDos())
	emisor := &emisorTeamsPrueba{Servicio: sala.S}
	sala.S = emisor
	win := test.NewWindow(sala.P.Objeto())
	t.Cleanup(win.Close)
	c := &conversacionExterna{sa: sala, ctx: t.Context(), lienzo: win.Canvas(), cuerpo: container.NewStack(), pie: container.NewStack(), entrada: ui.NuevaAreaTexto(""), proveedor: "slack", proveedorMeet: "microsoft-teams", enlaceMeet: "https://teams.microsoft.com/meet/opaque", canalMeet: "general"}
	c.d = ui.NuevoDialogo("Teams", c.cuerpo).Con(ui.DialogoLG, c.pie)
	c.compartirMeet()
	if c.compartidoMeet || c.enlaceMeet == "" || c.envioMeet == "" {
		t.Fatal("uncertain encrypted share lost its recovery state")
	}
	c.compartirMeet()
	c.compartirMeet()
	if len(emisor.transacciones) != 2 || emisor.transacciones[0] != emisor.transacciones[1] || emisor.enlaces[0] != emisor.enlaces[1] || !c.compartidoMeet {
		t.Fatal("encrypted retry changed identity or duplicated a confirmed share")
	}
}

func TestTeamsMeetingGenerationChangeBlocksLateCreation(t *testing.T) {
	sala, _ := salaDePrueba(t, servicioConDos())
	generation := "session-a"
	sala.GeneracionSesion = func() string { return generation }
	c := &conversacionExterna{sa: sala, ctx: t.Context(), generacionMeet: generation}
	guard := c.ambitoMeet()
	generation = "session-b"
	if guard() || c.vigenteMeet() {
		t.Fatal("old meeting scope accepted a new session")
	}
	c.cerrado = true
	generation = "session-a"
	if c.vigenteMeet() {
		t.Fatal("closed wizard accepted a late result")
	}
}
