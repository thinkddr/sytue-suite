// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type correoFalso struct {
	sent    api.ExternalMailSent
	sendErr error
	sends   int
	// mu guarda reads y pedidas: un buzón unificado pide sus carpetas en paralelo.
	// pedidas son las páginas pedidas, como «cuenta/carpeta/cursor».
	mu           sync.Mutex
	reads        int
	pedidas      []string
	out          api.ExternalMailOutgoing
	cuentas      api.ExternalMailAccounts
	listados     int
	configureErr error
	accountsErr  error
	foldersErr   error
	resumenes    int
	resumenErr   error
	// estados y carpetas cambian, por cuenta, el «ok» con su Recibidos del resumen; las
	// carpetas son también las que da MailFolders.
	estados  map[string]string
	carpetas map[string][]api.ExternalMailFolder
	// paginas son las de cada carpeta, por «cuenta/carpeta/cursor»; mensajesErr las
	// tumba todas, y caidas, las de esas cuentas.
	paginas     map[string]api.ExternalMailPage
	mensajesErr error
	caidas      map[string]bool
	// leido es lo que da MailMessage; acciones, cada MailAction como
	// «cuenta/carpeta/id accion destino <dirección>».
	leido    api.ExternalMailMessage
	leidos   []string
	acciones []string
}

// MailSummary es el resumen como lo da el gateway: las cuentas con buzón de
// `cuentas`, cada una «ok» y con su Recibidos.
func (s *correoFalso) MailSummary(context.Context) ([]api.ExternalMailAccountSummary, error) {
	s.resumenes++
	if s.resumenErr != nil {
		return nil, s.resumenErr
	}
	out := []api.ExternalMailAccountSummary{}
	for _, a := range s.cuentas.Items {
		if a.ServesMail() {
			c := api.ExternalMailAccountSummary{ExternalMailAccount: a, State: cmp.Or(s.estados[a.InstanceID], api.ExternalMailOK), Folders: []api.ExternalMailFolder{{ID: "INBOX", Name: "INBOX", Role: "inbox"}}}
			if f, ok := s.carpetas[a.InstanceID]; ok {
				c.Folders = f
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *correoFalso) MailAccounts(context.Context) (api.ExternalMailAccounts, error) {
	s.listados++
	return s.cuentas, s.accountsErr
}

func (s *correoFalso) MailConfigure(context.Context, api.ExternalMailConfiguration) (string, error) {
	if s.configureErr != nil {
		return "", s.configureErr
	}
	return "fixture", nil
}

func (s *correoFalso) MailFolders(_ context.Context, id, _ string) ([]api.ExternalMailFolder, error) {
	if s.foldersErr != nil {
		return nil, s.foldersErr
	}
	if f, ok := s.carpetas[id]; ok {
		return f, nil
	}
	return []api.ExternalMailFolder{{ID: "INBOX", Role: "inbox"}}, nil
}

func (s *correoFalso) MailMessages(_ context.Context, cuenta, carpeta, cursor, _ string) (api.ExternalMailPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	s.pedidas = append(s.pedidas, cuenta+"/"+carpeta+"/"+cursor)
	if s.mensajesErr != nil {
		return api.ExternalMailPage{}, s.mensajesErr
	}
	if s.caidas[cuenta] {
		return api.ExternalMailPage{}, &api.Error{Status: 502, Body: `{"error":{"code":"mail_unavailable"}}`}
	}
	return s.paginas[cuenta+"/"+carpeta+"/"+cursor], nil
}

// paginasPedidas son las páginas que se han pedido, en orden (las de un buzón
// unificado salen en paralelo: se ordenan), y las olvida.
func (s *correoFalso) paginasPedidas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := slices.Sorted(slices.Values(s.pedidas))
	s.pedidas = nil
	return out
}

// lecturas es cuántas veces se ha llamado a MailMessages.
func (s *correoFalso) lecturas() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func (s *correoFalso) MailMessage(_ context.Context, cuenta, carpeta, id, direccion string) (api.ExternalMailMessage, error) {
	s.leidos = append(s.leidos, cuenta+"/"+carpeta+"/"+id+" <"+direccion+">")
	return s.leido, nil
}

func (s *correoFalso) MailSend(_ context.Context, _ string, out api.ExternalMailOutgoing) (api.ExternalMailSent, error) {
	s.sends++
	s.out = out
	return s.sent, s.sendErr
}

func (s *correoFalso) MailAction(_ context.Context, cuenta, carpeta, id, accion, destino, direccion string) error {
	s.acciones = append(s.acciones, cuenta+"/"+carpeta+"/"+id+" "+accion+" "+destino+" <"+direccion+">")
	return nil
}
func (*correoFalso) MailDisconnect(context.Context, string) error { return nil }
func correoDePrueba(t *testing.T, s *correoFalso) *correoExterno {
	t.Helper()
	b, _ := buzonDePrueba(t, servicioDeEjemplo())
	b.ctx = context.Background()
	b.ConCorreoExterno(s, nil)
	ctx, cancel := context.WithCancel(b.ctx)
	t.Cleanup(cancel)
	c := &correoExterno{b: b, ctx: ctx, cancelar: cancel, proveedor: "imap", smtpSecurity: "tls", instancia: "fixture", campos: map[string]*ui.Entrada{}}
	for _, key := range []string{"name", "address", "host", "port", "username", "password", "smtpHost", "smtpPort", "smtpUser", "smtpPassword"} {
		c.campos[key] = ui.NuevaEntrada("")
	}
	c.cuerpo = container.NewVBox()
	c.pie = container.NewHBox()
	c.d = ui.NuevoDialogo("Fixture", c.cuerpo)
	return c
}

func TestExternalConfigurationKeepsExplicitTLSModeAndClearsSecrets(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	for key, value := range map[string]string{"address": "fixture@example.eu", "host": "imap.example.eu", "port": "993", "password": "fixture-secret", "smtpHost": "smtp.example.eu", "smtpPort": "2525", "smtpPassword": "smtp-fixture"} {
		c.campos[key].PonerTexto(value)
	}
	c.smtpSecurity = "starttls"
	cfg, err := c.configuracion()
	if err != nil || cfg.Outgoing.Security != "starttls" || cfg.Outgoing.Port != 2525 || cfg.Outgoing.Username != "fixture@example.eu" {
		t.Fatalf("config %#v %v", cfg, err)
	}
	c.limpiarClaves()
	if c.campos["password"].Texto() != "" || c.campos["smtpPassword"].Texto() != "" {
		t.Fatal("password retained")
	}
	if _, err = c.configuracion(); err == nil {
		t.Fatal("missing password accepted")
	}
}

func TestExternalReconnectWithoutConfigurationClearsPreviousServers(t *testing.T) {
	for _, provider := range []string{"imap", "pop3"} {
		t.Run(provider, func(t *testing.T) {
			c := correoDePrueba(t, &correoFalso{})
			for key, value := range map[string]string{"name": "Previous account", "address": "previous@example.eu", "host": "incoming.previous.example.eu", "port": "1993", "username": "previous-incoming", "password": "synthetic-previous-secret", "smtpHost": "smtp.previous.example.eu", "smtpPort": "2525", "smtpUser": "previous-smtp", "smtpPassword": "synthetic-previous-smtp-secret"} {
				c.campos[key].PonerTexto(value)
			}
			c.smtpSecurity, c.avanzado = "starttls", true
			c.reconectar(api.ExternalMailAccount{InstanceID: "new-account", Provider: provider, Name: "New account", Address: "new@example.eu", Services: []string{"calendar"}})
			for _, key := range []string{"host", "username", "password", "smtpHost", "smtpUser", "smtpPassword"} {
				if c.campos[key].Texto() != "" {
					t.Errorf("reconnect without configuration retained %s", key)
				}
			}
			wantPort := "993"
			if provider == "pop3" {
				wantPort = "995"
			}
			if c.campos["port"].Texto() != wantPort || c.campos["smtpPort"].Texto() != "465" || c.smtpSecurity != "tls" || c.avanzado {
				t.Fatal("reconnect without configuration retained previous advanced settings")
			}
			if c.campos["name"].Texto() != "New account" || c.campos["address"].Texto() != "new@example.eu" ||
				c.instancia != "new-account" || c.proveedor != provider || c.paso != 1 || !c.asistente ||
				!slices.Equal(c.servicios, []string{"mail", "calendar"}) {
				t.Fatal("reconnect lost account identity or its mail consent")
			}
			for key, value := range map[string]string{"host": "incoming.new.example.eu", "smtpHost": "smtp.new.example.eu", "password": "synthetic-new-secret"} {
				c.campos[key].PonerTexto(value)
			}
			cfg, err := c.configuracion()
			if err != nil || cfg.InstanceID != "new-account" || cfg.Incoming.Username != "new@example.eu" ||
				cfg.Outgoing.Username != "new@example.eu" || cfg.Outgoing.Port != 465 || cfg.Outgoing.Security != "tls" || cfg.SMTPPassword != "" {
				t.Fatal("reconnection payload inherited previous account settings", err)
			}
		})
	}
}

func TestExternalAttachmentLimitIsAppliedBeforeReadingAllFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "fixture.txt")
	if err := os.WriteFile(file, []byte("too large for configured limit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := leerAdjuntosConLimite([]string{file}, 5); err == nil {
		t.Fatal("oversized attachment accepted")
	}
	if got, err := leerAdjuntosConLimite([]string{file}, 100); err != nil || len(got) != 1 {
		t.Fatalf("valid attachment %v %v", got, err)
	}
}

func TestMailServicesRequireMailAndBlockUnsupportedNotes(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	c.proveedor = "gmail"
	c.cuentas.Providers = []api.ExternalMailProvider{{Provider: "gmail", Available: true, Services: []api.LinkedServiceCapability{{Service: "mail", Available: true}, {Service: "calendar", Available: true}, {Service: "notes", Reason: "workspace_admin_required"}}}}
	c.pintarServicios()
	choices := []*ui.Eleccion{}
	for _, item := range c.cuerpo.Objects {
		if choice, ok := item.(*ui.Eleccion); ok {
			choices = append(choices, choice)
		}
	}
	if len(choices) != 3 {
		t.Fatalf("missing service choices %d", len(choices))
	}
	choices[0].Tapped(nil)
	choices[1].Tapped(nil)
	choices[2].Tapped(nil)
	if !slices.Contains(c.servicios, "mail") || !slices.Contains(c.servicios, "calendar") || slices.Contains(c.servicios, "notes") {
		t.Fatalf("invalid consent selection: %v", c.servicios)
	}
	var granted []string
	attempts := 0
	c.b.AutorizarCorreo = func(_ context.Context, provider, instance string, services ...string) (string, error) {
		attempts++
		if attempts == 2 && instance != "pending-account" {
			t.Error("denied consent created another account on retry")
		}
		granted = append([]string(nil), services...)
		return "pending-account", context.Canceled
	}
	c.autorizar()
	if !reflect.DeepEqual(granted, []string{"mail", "calendar"}) {
		t.Fatalf("selected permissions did not reach consent: %v", granted)
	}
	c.autorizar()
	if attempts != 2 || c.instancia != "pending-account" || c.b.externa != nil {
		t.Fatal("denied draft was lost or falsely marked connected")
	}
}

func TestServiceOnlyAccountAddsMailBeforeOpeningItsMailbox(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	account := api.ExternalMailAccount{InstanceID: "linked-only", Provider: "microsoft", Services: []string{"calendar"}, Connected: true}
	c.abrirCuenta(account)
	if c.b.externa != nil || c.cerrado {
		t.Fatal("service-only account opened a mailbox without mail consent")
	}
	c.reconectar(account)
	if !slices.Contains(c.servicios, "mail") || !slices.Contains(c.servicios, "calendar") {
		t.Fatalf("Mail reconnection lost its required or existing service: %v", c.servicios)
	}
}

// B7: volver del proveedor cancelado, sin el permiso de correo o tarde no es el
// genérico «no se pudo acceder al buzón».
func TestAuthorizationErrorsTellWhatToFix(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{api.ErrOAuthDenied, "externalMail.cancelled"},
		{context.Canceled, "externalMail.cancelled"},
		{api.ErrOAuthScope, "externalMail.scopeMissing"},
		{api.ErrOAuthState, "externalMail.timeout"},
		{context.DeadlineExceeded, "externalMail.timeout"},
		{&api.Error{Status: 401, Body: `{"error":{"code":"mail_auth"}}`}, "externalMail.authError"},
		{errors.New("oauth: exchange"), "externalMail.error"},
	} {
		c := correoDePrueba(t, &correoFalso{})
		c.proveedor, c.asistente = "gmail", true
		c.cuentas.Providers = []api.ExternalMailProvider{{Provider: "gmail", Available: true}}
		c.b.AutorizarCorreo = func(context.Context, string, string, ...string) (string, error) { return "borrador", tc.err }
		c.autorizar()
		if c.errorClave != tc.want {
			t.Errorf("%v → %q; quiero %q", tc.err, c.errorClave, tc.want)
		}
	}
}

// B3: si la cuenta ya estaba conectada, se vuelve a la lista con aviso y sin
// reintentar con el borrador que el servidor descartó.
func TestAlreadyConnectedAccountReturnsToTheListWithANotice(t *testing.T) {
	cuentas := api.ExternalMailAccounts{
		Items:     []api.ExternalMailAccount{{InstanceID: "la-de-antes", Provider: "gmail", Address: "ana@example.eu", Connected: true, Services: []string{"mail"}}},
		Providers: []api.ExternalMailProvider{{Provider: "gmail", Available: true}},
	}
	c := correoDePrueba(t, &correoFalso{cuentas: cuentas})
	c.proveedor, c.asistente, c.instancia = "gmail", true, ""
	c.cuentas = cuentas
	c.b.AutorizarCorreo = func(context.Context, string, string, ...string) (string, error) {
		return "borrador", &api.OAuthExistsError{Instance: "la-de-antes"}
	}
	c.autorizar()
	if c.asistente || c.instancia != "" || c.aviso != "externalMail.exists" || c.errorClave != "" || c.b.externa != nil {
		t.Fatalf("cuenta repetida: asistente %v instancia %q aviso %q error %q", c.asistente, c.instancia, c.aviso, c.errorClave)
	}
	// Lo mismo a mano (IMAP): el 409 del servidor no deja el asistente en error.
	s := &correoFalso{cuentas: cuentas, configureErr: &api.Error{Status: 409, Body: `{"error":{"code":"mail_account_exists","params":{"instance_id":"la-de-antes"}}}`}}
	c = correoDePrueba(t, s)
	c.asistente, c.instancia = true, ""
	for key, value := range map[string]string{"address": "ana@example.eu", "host": "imap.example.eu", "smtpHost": "smtp.example.eu", "password": "secreta"} {
		c.campos[key].PonerTexto(value)
	}
	c.campos["port"].PonerTexto("993")
	c.campos["smtpPort"].PonerTexto("465")
	c.conectar()
	if c.asistente || c.aviso != "externalMail.exists" || c.errorClave != "" || c.campos["password"].Texto() != "" {
		t.Fatalf("IMAP repetido: asistente %v aviso %q error %q", c.asistente, c.aviso, c.errorClave)
	}
	// Abrir la cuenta responde al aviso y la lleva al panel principal.
	c.abrirCuenta(cuentas.Items[0])
	if c.aviso != "" || !c.cerrado || c.b.externa == nil || c.b.externa.Cuenta != "la-de-antes" {
		t.Fatalf("abrir la cuenta: aviso %q, diálogo cerrado %v, vista %+v", c.aviso, c.cerrado, c.b.externa)
	}
}

// B12: las cuentas conectadas solo para Calendario no salen en Mail.
func TestMailListsOnlyAccountsWithAMailbox(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{
		{InstanceID: "antigua", Connected: true},
		{InstanceID: "correo", Connected: true, Services: []string{"mail", "calendar"}},
		{InstanceID: "calendario", Connected: true, Services: []string{"calendar"}},
	}}})
	c.cargarCuentas("", "")
	dialogo := make([]string, 0, len(c.cuentas.Items))
	lateral := make([]string, 0, len(c.b.P.Menu.Cuentas))
	for _, a := range c.cuentas.Items {
		dialogo = append(dialogo, a.InstanceID)
	}
	for _, a := range c.b.P.Menu.Cuentas {
		lateral = append(lateral, a.ID)
	}
	if want := []string{"antigua", "correo"}; !reflect.DeepEqual(dialogo, want) || !reflect.DeepEqual(lateral, want) {
		t.Fatalf("diálogo %v, lateral %v; quiero %v en los dos", dialogo, lateral, want)
	}
}

// iCloud no tiene OAuth para terceros: es IMAP con sus servidores fijos y una
// contraseña de app. Lo mismo que «iCloud pide solo la dirección…» en la web.
func TestICloudSavesAnIMAPConnectionWithItsFixedServers(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	c.asistente, c.instancia = true, ""
	c.elegirProveedor("icloud")
	if c.proveedor != "imap" || !c.icloud {
		t.Fatalf("iCloud elige %q (icloud %v); quiero imap con el preajuste", c.proveedor, c.icloud)
	}
	c.campos["address"].PonerTexto("ana@icloud.com")
	c.campos["password"].PonerTexto("abcd-efgh-ijkl-mnop")
	cfg, err := c.configuracion()
	want := api.ExternalMailConfiguration{
		Provider: "imap", Address: "ana@icloud.com", Password: "abcd-efgh-ijkl-mnop",
		Incoming: api.ExternalMailServer{Host: "imap.mail.me.com", Port: 993, Username: "ana@icloud.com", Security: "tls"},
		Outgoing: api.ExternalMailServer{Host: "smtp.mail.me.com", Port: 587, Username: "ana@icloud.com", Security: "starttls"},
	}
	if err != nil || !reflect.DeepEqual(cfg, want) {
		t.Fatalf("configuración de iCloud %+v %v", cfg, err)
	}
	c.paso = 1
	c.pintar()

	// Pasar a IMAP no se lleva los servidores de iCloud.
	c.elegirProveedor("imap")
	if c.icloud || c.campos["host"].Texto() != "" || c.campos["smtpHost"].Texto() != "" || c.campos["smtpPort"].Texto() != "465" || c.smtpSecurity != "tls" {
		t.Fatalf("IMAP heredó iCloud: host %q smtp %q:%q %s", c.campos["host"].Texto(), c.campos["smtpHost"].Texto(), c.campos["smtpPort"].Texto(), c.smtpSecurity)
	}

	// Reconectar una cuenta de iCloud vuelve al formulario corto.
	a := api.ExternalMailAccount{InstanceID: "nube", Provider: "imap", Address: "ana@icloud.com"}
	a.Configuration = &struct {
		Address  string                 `json:"address"`
		Incoming api.ExternalMailServer `json:"incoming"`
		Outgoing api.ExternalMailServer `json:"outgoing"`
	}{Address: a.Address, Incoming: want.Incoming, Outgoing: want.Outgoing}
	c.reconectar(a)
	if !c.icloud || c.proveedor != "imap" || c.campos["smtpHost"].Texto() != "smtp.mail.me.com" || c.smtpSecurity != "starttls" {
		t.Fatalf("reconectar iCloud: icloud %v proveedor %q", c.icloud, c.proveedor)
	}
}

// Un dominio propio de iCloud+ o un alias entra con la dirección principal de iCloud:
// sin campo para ella, la contraseña de app correcta daba «revisa usuario y contraseña».
// Lo mismo que «iCloud admite otro usuario en «Avanzado»…» en la web.
func TestICloudAcceptsAnotherUserNameForBothServers(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	c.asistente, c.instancia = true, ""
	c.elegirProveedor("icloud")
	c.campos["address"].PonerTexto("ana@ana.example")
	c.campos["password"].PonerTexto("abcd-efgh-ijkl-mnop")
	c.campos["username"].PonerTexto("ana@icloud.com")
	c.campos["smtpUser"].PonerTexto("lo-que-quedara@example.eu")
	cfg, err := c.configuracion()
	if err != nil || cfg.Address != "ana@ana.example" || cfg.Incoming.Username != "ana@icloud.com" || cfg.Outgoing.Username != "ana@icloud.com" {
		t.Fatalf("iCloud con otro usuario: %+v %v", cfg, err)
	}
}

// Reconectar conservaba, oculto, el usuario guardado: editar la dirección seguía
// autenticando con la antigua. Lo mismo que «reconectar iCloud con otra dirección…».
func TestReconnectingICloudFollowsTheEditedAddress(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	a := api.ExternalMailAccount{InstanceID: "nube", Provider: "imap", Address: "ana@icloud.com"}
	a.Configuration = &struct {
		Address  string                 `json:"address"`
		Incoming api.ExternalMailServer `json:"incoming"`
		Outgoing api.ExternalMailServer `json:"outgoing"`
	}{
		Address:  a.Address,
		Incoming: api.ExternalMailServer{Host: "imap.mail.me.com", Port: 993, Username: "ANA@icloud.com", Security: "tls"},
		Outgoing: api.ExternalMailServer{Host: "smtp.mail.me.com", Port: 587, Username: "ana@icloud.com", Security: "starttls"},
	}
	c.reconectar(a)
	if c.campos["username"].Texto() != "" || c.avanzado {
		t.Fatalf("el usuario que es la dirección no se guarda aparte: %q (avanzado %v)", c.campos["username"].Texto(), c.avanzado)
	}
	c.campos["address"].PonerTexto("ana@me.com")
	c.campos["password"].PonerTexto("abcd-efgh-ijkl-mnop")
	cfg, err := c.configuracion()
	if err != nil || cfg.InstanceID != "nube" || cfg.Incoming.Username != "ana@me.com" || cfg.Outgoing.Username != "ana@me.com" {
		t.Fatalf("reconectar con otra dirección: %+v %v", cfg, err)
	}
	// Un usuario distinto de la dirección (el de un alias) sí se conserva, a la vista.
	a.Configuration.Incoming.Username = "principal@icloud.com"
	c.reconectar(a)
	if c.campos["username"].Texto() != "principal@icloud.com" || !c.avanzado {
		t.Fatalf("el usuario del alias se perdió: %q (avanzado %v)", c.campos["username"].Texto(), c.avanzado)
	}
}

// Tras «ya conectada» el botón «Conectar» que tenía el foco desaparece: el foco va a
// la cuenta que ya estaba, como focusAfterList en la web.
func TestAlreadyConnectedFocusesTheExistingAccount(t *testing.T) {
	cuentas := api.ExternalMailAccounts{
		Items: []api.ExternalMailAccount{
			{InstanceID: "otra", Provider: "gmail", Address: "otra@example.eu", Connected: true, Services: []string{"mail"}},
			{InstanceID: "la-de-antes", Provider: "imap", Address: "ana@example.eu", Connected: true, Services: []string{"mail"}},
		},
		Providers: []api.ExternalMailProvider{{Provider: "imap", Available: true}, {Provider: "gmail", Available: true}},
	}
	abrir := func(c *correoExterno) fyne.Window {
		w := test.NewWindow(c.b.P.Objeto())
		t.Cleanup(w.Close)
		c.d.Mostrar(w.Canvas())
		return w
	}
	enfocado := func(w fyne.Window) string {
		b, ok := w.Canvas().Focused().(*ui.Boton)
		if !ok {
			return fmt.Sprintf("%T", w.Canvas().Focused())
		}
		return b.Texto
	}

	// A mano (IMAP): el 409 trae la cuenta en params.instance_id.
	s := &correoFalso{cuentas: cuentas, configureErr: &api.Error{Status: 409, Body: `{"error":{"code":"mail_account_exists","params":{"instance_id":"la-de-antes"}}}`}}
	c := correoDePrueba(t, s)
	w := abrir(c)
	c.asistente, c.instancia = true, ""
	for key, value := range map[string]string{"address": "ana@example.eu", "host": "imap.example.eu", "port": "993", "smtpHost": "smtp.example.eu", "smtpPort": "465", "password": "secreta"} {
		c.campos[key].PonerTexto(value)
	}
	c.conectar()
	abrirLaDeAntes := botonDeCuenta(t, c, 1, "externalMail.open")
	if w.Canvas().Focused() != fyne.Focusable(abrirLaDeAntes) {
		t.Fatalf("IMAP repetido: el foco está en %s, no en «Abrir» de la cuenta que ya estaba", enfocado(w))
	}

	// Por OAuth: la cuenta viene en OAuthExistsError.Instance.
	c = correoDePrueba(t, &correoFalso{cuentas: cuentas})
	w = abrir(c)
	c.proveedor, c.asistente, c.instancia = "gmail", true, ""
	c.cuentas = cuentas
	c.b.AutorizarCorreo = func(context.Context, string, string, ...string) (string, error) {
		return "borrador", &api.OAuthExistsError{Instance: "otra"}
	}
	c.autorizar()
	if w.Canvas().Focused() != fyne.Focusable(botonDeCuenta(t, c, 0, "externalMail.open")) {
		t.Fatalf("OAuth repetido: el foco está en %s, no en «Abrir» de la cuenta que ya estaba", enfocado(w))
	}
	// Usar la lista responde al foco pendiente.
	for _, o := range c.pie.Objects {
		if b, ok := o.(*ui.Boton); ok && b.Texto == c.t("nav.refresh") {
			b.AlPulsar()
		}
	}
	if c.foco != "" {
		t.Fatalf("el foco pendiente sobrevive a usar la lista: %q", c.foco)
	}
}

// botonDeCuenta es el botón `key` de la n-ésima cuenta de la lista pintada.
func botonDeCuenta(t *testing.T, c *correoExterno, n int, key string) *ui.Boton {
	t.Helper()
	texto := c.t(key)
	vistos := -1
	for _, o := range c.cuerpo.Objects {
		b, ok := o.(*ui.Boton)
		if !ok {
			continue
		}
		if b.Texto == c.t("externalMail.disconnect") {
			vistos++
			continue
		}
		if b.Texto == texto && vistos == n-1 {
			return b
		}
	}
	t.Fatalf("no hay «%s» en la cuenta %d", texto, n)
	return nil
}

// cuentaGmailNueva es lo que lista el servidor tras un OAuth que ha terminado bien.
func cuentaGmailNueva() api.ExternalMailAccounts {
	return api.ExternalMailAccounts{
		Items: []api.ExternalMailAccount{{
			InstanceID: "nueva", Provider: "gmail", Address: "ana@example.eu", Connected: true, Services: []string{"mail"},
			Capabilities: api.ExternalMailCapabilities{Read: true, Send: true, Actions: true},
		}},
		Providers: []api.ExternalMailProvider{{Provider: "gmail", Available: true}},
	}
}

// B1: iniciar sesión en el proveedor no hereda los 45 s de una llamada al buzón:
// la persona está en el navegador con su contraseña o su segundo factor.
func TestAuthorizationOutlivesTheMailboxCallTimeout(t *testing.T) {
	antes := plazoCorreo
	plazoCorreo = 20 * time.Millisecond
	t.Cleanup(func() { plazoCorreo = antes })
	cuentas := cuentaGmailNueva()
	c := correoDePrueba(t, &correoFalso{cuentas: cuentas})
	c.proveedor, c.asistente, c.paso, c.instancia = "gmail", true, 1, ""
	c.cuentas = cuentas
	c.b.AutorizarCorreo = func(ctx context.Context, _, _ string, _ ...string) (string, error) {
		select {
		case <-time.After(10 * plazoCorreo):
			return "nueva", nil
		case <-ctx.Done():
			return "borrador", ctx.Err()
		}
	}
	c.autorizar()
	// Conectada: su Entrada en el panel principal, y el diálogo cerrado.
	if c.errorClave != "" || c.b.externa == nil || c.b.externa.Cuenta != "nueva" || !c.cerrado || c.autorizando != nil {
		t.Fatalf("el inicio de sesión se cortó al plazo de una llamada: error %q, vista %+v", c.errorClave, c.b.externa)
	}
}

// B1: con diez minutos de plazo, «Cancelar» y el aspa cortan la espera en vez de
// quedarse inertes.
func TestAuthorizationCanBeCancelledWhileWaitingForTheBrowser(t *testing.T) {
	for _, cerrar := range []bool{false, true} {
		cuentas := cuentaGmailNueva()
		c := correoDePrueba(t, &correoFalso{cuentas: cuentas})
		c.proveedor, c.asistente, c.paso, c.instancia = "gmail", true, 1, ""
		c.cuentas = cuentas
		var trabajo func()
		c.b.Lanzar = func(f func()) { trabajo = f }
		c.b.AutorizarCorreo = func(ctx context.Context, _, _ string, _ ...string) (string, error) {
			select {
			case <-ctx.Done():
				return "borrador", ctx.Err()
			case <-time.After(time.Second):
				return "", errors.New("la espera no se cortó")
			}
		}
		c.autorizar()
		if !c.ocupado || c.autorizando == nil {
			t.Fatal("autorizar no ha quedado esperando al navegador")
		}
		if cerrar {
			c.pedirCerrar()
			trabajo()
			if !c.cerrado {
				t.Fatal("el aspa no cierra mientras se espera al navegador")
			}
			continue
		}
		for _, o := range c.pie.Objects {
			if b, ok := o.(*ui.Boton); ok && b.Texto == c.t("common.cancel") && b.Inerte {
				t.Fatal("«Cancelar» está inerte mientras se espera al navegador")
			}
		}
		pulsarPieCorreo(t, c, "common.cancel")
		trabajo()
		if c.ocupado || !c.asistente || c.errorClave != "externalMail.cancelled" || c.instancia != "borrador" {
			t.Fatalf("Cancelar no cortó la espera: ocupado %v asistente %v error %q instancia %q", c.ocupado, c.asistente, c.errorClave, c.instancia)
		}
	}
}

// B9: «Requiere volver a iniciar sesión» del lateral pide la lista de cuentas una sola
// vez y abre el inicio de sesión de esa cuenta, no la lista ni un buzón. VISTO EN ROJO
// (2026-11-06) sin `reconectar` en cargarCuentas: se quedaba en la lista.
func TestOpeningAnAccountFromTheSidebarListsAccountsOnce(t *testing.T) {
	s := &correoFalso{cuentas: cuentaGmailNueva()}
	b := correoDePrueba(t, s).b
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	b.abrirCorreoExternoEn("nueva")
	t.Cleanup(func() {
		if b.cerrarCorreoExterno != nil {
			b.cerrarCorreoExterno()
		}
	})
	if s.listados != 1 {
		t.Errorf("se han pedido las cuentas %d veces para abrir una", s.listados)
	}
	if c := b.correoAbierto; c == nil || !c.asistente || c.paso != 1 || c.instancia != "nueva" || b.externa != nil {
		t.Fatal("no se ha abierto el inicio de sesión de la cuenta elegida en el lateral")
	}
}

// cuentaICloud es una conexión guardada con el preajuste de iCloud.
func cuentaICloud() api.ExternalMailAccount {
	a := api.ExternalMailAccount{InstanceID: "nube", Provider: "imap", Name: "Personal", Address: "ana@icloud.com", Connected: true, Services: []string{"mail"}}
	a.Configuration = &struct {
		Address  string                 `json:"address"`
		Incoming api.ExternalMailServer `json:"incoming"`
		Outgoing api.ExternalMailServer `json:"outgoing"`
	}{Address: a.Address, Incoming: api.ICloudMail.Incoming, Outgoing: api.ICloudMail.Outgoing}
	return a
}

// El formulario de iCloud enlaza a account.apple.com, donde se crea la contraseña
// de app; IMAP no lo lleva. Lo mismo que «el formulario de iCloud enlaza…» en la web.
func TestICloudFormLinksToAppleAccount(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	abridor := &abridorFalso{}
	c.b.Abridor = abridor
	c.asistente, c.instancia = true, ""
	enlace := func() *ui.Enlace {
		for _, o := range c.cuerpo.Objects {
			if e, ok := o.(*ui.Enlace); ok && e.Texto == c.t("externalMail.icloudLink") {
				return e
			}
		}
		return nil
	}
	c.elegirProveedor("icloud")
	c.paso = 1
	c.pintar()
	e := enlace()
	if e == nil {
		t.Fatal("el formulario de iCloud no enlaza a account.apple.com")
	}
	e.Tapped(nil)
	if !slices.Equal(abridor.abiertas, []string{"https://account.apple.com/"}) {
		t.Fatalf("abre %v; quiero account.apple.com", abridor.abiertas)
	}
	c.elegirProveedor("imap")
	c.pintar()
	if enlace() != nil {
		t.Fatal("IMAP también enlaza a la cuenta de Apple")
	}
}

// Con la contraseña del Apple ID, iCloud rechaza el LOGIN: el aviso pide la
// contraseña específica de app; en un IMAP cualquiera sigue el general.
func TestICloudLoginRejectedAsksForAnAppPassword(t *testing.T) {
	for _, tc := range []struct {
		eleccion, direccion, quiero string
	}{
		{"icloud", "ana@icloud.com", "externalMail.icloudAuthError"},
		{"imap", "ana@example.eu", "externalMail.authError"},
	} {
		s := &correoFalso{configureErr: &api.Error{Status: 422, Body: `{"error":{"code":"mail_auth"}}`}}
		c := correoDePrueba(t, s)
		c.asistente, c.instancia = true, ""
		c.elegirProveedor(tc.eleccion)
		for key, value := range map[string]string{"address": tc.direccion, "password": "la-del-apple-id", "host": "imap.example.eu", "smtpHost": "smtp.example.eu", "port": "993", "smtpPort": "465"} {
			if c.campos[key].Texto() == "" {
				c.campos[key].PonerTexto(value)
			}
		}
		c.conectar()
		if c.errorClave != tc.quiero {
			t.Errorf("%s: aviso %q; quiero %q", tc.eleccion, c.errorClave, tc.quiero)
		}
	}
}

// La lista nombra la cuenta de iCloud como iCloud Mail, no como IMAP, y el lateral
// le pone la nube (en la web, la lista de ExternalMail.tsx y ExternalAccountsSection).
func TestICloudIsNamedInTheListAndCloudyInTheSidebar(t *testing.T) {
	icloud := cuentaICloud()
	otra := api.ExternalMailAccount{InstanceID: "otra", Provider: "imap", Name: "Trabajo", Address: "ana@example.eu", Connected: true, Services: []string{"mail"}}
	s := &correoFalso{cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{icloud, otra}}}
	c := correoDePrueba(t, s)
	c.cuentas = s.cuentas
	c.pintar()
	var textos []string
	for _, o := range c.cuerpo.Objects {
		if p, ok := o.(*ui.Parrafo); ok {
			textos = append(textos, p.Texto)
		}
	}
	for _, want := range []string{"ana@icloud.com · iCloud Mail", "ana@example.eu · IMAP"} {
		if !slices.Contains(textos, want) {
			t.Errorf("la lista no dice %q: %q", want, textos)
		}
	}
	c.b.fijarExternas(s.cuentas)
	glifos := make([]iconos.Icono, 0, len(c.b.P.Menu.Cuentas))
	for _, e := range c.b.P.Menu.Cuentas {
		glifos = append(glifos, iconoExterna(e))
	}
	if want := []iconos.Icono{iconos.Nube, iconos.Usuario}; !slices.Equal(glifos, want) {
		t.Fatalf("iconos del lateral %v; quiero la nube para iCloud y la persona para la otra", glifos)
	}
}
