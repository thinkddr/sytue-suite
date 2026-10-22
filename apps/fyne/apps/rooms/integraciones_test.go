// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// conIntegraciones es el doble con «Apps e integraciones»; `prohibido` es el 403.
type conIntegraciones struct {
	*servicioFalso
	prohibido bool
}

func (s *conIntegraciones) IncomingWebhooks(_ context.Context, c string) ([]api.RoomsIncomingWebhook, error) {
	s.apuntar("IncomingWebhooks", c)
	if s.prohibido {
		return nil, &api.Error{Status: 403}
	}
	return []api.RoomsIncomingWebhook{{ID: "i1", ChannelID: c, Name: "CI"}}, nil
}

func (s *conIntegraciones) CreateIncomingWebhook(_ context.Context, c, n, _ string) (api.RoomsIncomingSecret, error) {
	s.apuntar("CreateIncomingWebhook", c, n)
	return api.RoomsIncomingSecret{Token: "tk", URL: "https://rooms.eu/hooks/tk", ChannelMailDomain: "rooms.sytue.eu"}, nil
}

// RotateIncomingWebhook contesta como un servicio con el correo a canal APAGADO.
func (s *conIntegraciones) RotateIncomingWebhook(context.Context, string) (api.RoomsIncomingSecret, error) {
	return api.RoomsIncomingSecret{Token: "tk2", URL: "https://rooms.eu/hooks/tk2"}, nil
}
func (s *conIntegraciones) DeleteIncomingWebhook(context.Context, string) error { return nil }
func (s *conIntegraciones) OutgoingWebhooks(context.Context) ([]api.RoomsOutgoingWebhook, error) {
	return nil, nil
}

func (s *conIntegraciones) CreateOutgoingWebhook(_ context.Context, w api.RoomsOutgoingWebhook) (api.RoomsOutgoingWebhook, error) {
	return w, nil
}
func (s *conIntegraciones) DeleteOutgoingWebhook(context.Context, string) error { return nil }
func (s *conIntegraciones) Bots(context.Context) ([]api.RoomsBot, error)        { return nil, nil }
func (s *conIntegraciones) CreateBot(_ context.Context, n string) (api.RoomsBot, string, error) {
	s.apuntar("CreateBot", n)
	return api.RoomsBot{ID: "b1", Name: n}, "bot_x", nil
}

func (s *conIntegraciones) DeleteBot(context.Context, string) error { return nil }

func (s *conIntegraciones) SetBotInChannel(context.Context, string, string, bool) error { return nil }

func (s *conIntegraciones) Bridges(context.Context, string) ([]api.RoomsBridge, error) {
	return nil, nil
}

func (s *conIntegraciones) DeleteBridge(context.Context, string) error { return nil }
func (s *conIntegraciones) WhatsApp(context.Context) ([]api.RoomsWhatsApp, error) {
	return nil, nil
}

func (s *conIntegraciones) DisconnectWhatsApp(context.Context, string) error { return nil }
func (s *conIntegraciones) CreateCommand(_ context.Context, k api.RoomsCommand) (api.RoomsCommand, error) {
	return k, nil
}
func (s *conIntegraciones) DeleteCommand(context.Context, string) error { return nil }

// «Apps» abre el diálogo con lo del canal abierto; crear un entrante enseña su URL
// una vez con las variantes de Slack, Teams y Google Chat; crear un bot, su token; un
// 403 es «no tienes permiso», no una avería.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con el 403 tratado como cualquier fallo,
// el diálogo dice «No se pudieron cargar».
func TestAppsEIntegraciones(t *testing.T) {
	if got := VariantesGancho("https://hooks.eu/api/hooks/tk"); len(got) != 4 || got[1].URL != "https://hooks.eu/api/hooks/slack/tk" {
		t.Errorf("variantes: %+v", got)
	}
	if got := PalabrasDisparadoras(" deploy, ,status\nfin"); len(got) != 3 || got[1] != "status" {
		t.Errorf("palabras: %v", got)
	}
	// Los casos de hookVariants.test.ts.
	const u = "https://rooms.sytue.eu/hooks/abc123"
	for _, c := range []struct{ dominio, canal, quiero string }{
		{"rooms.sytue.eu", "", "abc123@rooms.sytue.eu"},
		{"@rooms.sytue.eu.", "", "abc123@rooms.sytue.eu"},
		{"rooms.sytue.eu", "General", "general+abc123@rooms.sytue.eu"},
		{"rooms.sytue.eu", "Avisos de Producción", "avisos-de-produccion+abc123@rooms.sytue.eu"},
		{"rooms.sytue.eu", "###", "abc123@rooms.sytue.eu"},
		{"", "General", ""},
	} {
		if got := CorreoCanal(u, c.dominio, c.canal); got != c.quiero {
			t.Errorf("CorreoCanal(%q, %q) = %q; quiero %q", c.dominio, c.canal, got, c.quiero)
		}
	}
	if got := CorreoCanal("https://rooms.sytue.eu/otra/ruta", "rooms.sytue.eu", ""); got != "" {
		t.Errorf("sin /hooks/ sale %q", got)
	}
	base := servicioConDos()
	s := &conIntegraciones{servicioFalso: base}
	sa, _, w := salaEn(t, base, 1280)
	sa.S = s
	sa.Apex = "sytue.eu"
	var abierto []string
	sa.AbrirEnlace = func(u *url.URL) error { abierto = append(abierto, u.String()); return nil }
	sa.Arrancar(context.Background())
	// El «+» del compositor acaba en «Apps e integraciones», tras su separador.
	sa.MenuMas(sa.P.Canal.Compositor)
	if n := len(sa.menuMas.Elementos); n < 2 {
		t.Fatalf("el menú «+» lleva %d", n)
	} else if ult, ok := sa.menuMas.Elementos[n-1].(*ui.ElementoMenu); !ok || ult.Texto != sa.P.T.T("integrations.apps", nil) {
		t.Errorf("el «+» acaba en %#v", sa.menuMas.Elementos[n-1])
	} else if _, ok := sa.menuMas.Elementos[n-2].(*ui.SeparadorMenu); !ok {
		t.Error("«Apps e integraciones» va sin su separador")
	}
	sa.menuMas.Cerrar()
	sa.AbrirPanel(PanelIntegracion)
	c := sa.integraciones
	if c == nil || c.Cargando || c.Canal != "c1" || len(c.Datos.Entrantes) != 1 || len(c.Canales) != 1 {
		t.Fatalf("el diálogo abre con %+v", c)
	}
	// Sin ser admin de la organización, «Apps» no ofrece conectar: lo diría un 403.
	if c.AdminOrg || len(c.Pestanas().Rotulos) != 2 {
		t.Fatalf("pestañas %v, admin de la organización %v", c.Pestanas().Rotulos, c.AdminOrg)
	}
	c.Tipo = "incoming"
	c.Campo("name").PonerTexto("CI")
	c.crear()
	if c.Secreto == nil || c.Secreto.Valor != "https://rooms.eu/hooks/tk" || len(c.Secreto.Variantes) != 5 || c.Campo("name").Texto() != "" {
		t.Fatalf("el secreto del entrante: %+v", c.Secreto)
	}
	if correo := c.Secreto.Variantes[4]; correo.URL != "equipo+tk@rooms.sytue.eu" || correo.Rotulo != sa.P.T.T("integrations.channelEmail", nil) {
		t.Errorf("la dirección de correo: %+v", correo)
	}
	// Con el correo a canal apagado en el servicio no se ofrece la dirección.
	c.AlRotar("i1")
	if c.Secreto == nil || c.Secreto.Valor != "https://rooms.eu/hooks/tk2" || len(c.Secreto.Variantes) != 4 {
		t.Errorf("rotar sin correo a canal: %+v", c.Secreto)
	}
	c.AlCrearBot("Deploy")
	if c.Secreto == nil || c.Secreto.Valor != "bot_x" {
		t.Errorf("el token del bot: %+v", c.Secreto)
	}
	c.AdminOrg = true
	for i := range c.Pestanas().Rotulos {
		c.Pestanas().Elegir(i) // cada pestaña se pinta sin romperse
	}
	for _, tipo := range tiposDesarrollo {
		c.Tipo = tipo.clave
		c.Refresh() // y cada tipo de «Desarrolladores»
	}
	if dir := os.Getenv("SYTUE_CAPTURA_SALA"); dir != "" {
		c.Secreto = nil
		c.Pestanas().Elegir(PestanaApps)
		f, err := os.Create(filepath.Join(dir, "sala-integraciones-1280.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	c.AlConectarApp("slack-rooms")
	c.AlPolitica()
	// Without native account capability/permission the action must not escape to Admin web.
	if len(abierto) != 1 || !strings.HasSuffix(abierto[0], "/es/privacy") {
		t.Errorf("abre %v", abierto)
	}
	s.prohibido = true
	c.AlCanal("c1")
	if c.Error != "forbidden" {
		t.Errorf("un 403 deja %q", c.Error)
	}
}
