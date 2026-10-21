// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type correoFalso struct {
	sent    api.ExternalMailSent
	sendErr error
	sends   int
	out     api.ExternalMailOutgoing
}

func (*correoFalso) MailAccounts(context.Context) (api.ExternalMailAccounts, error) {
	return api.ExternalMailAccounts{}, nil
}

func (*correoFalso) MailConfigure(context.Context, api.ExternalMailConfiguration) (string, error) {
	return "fixture", nil
}

func (*correoFalso) MailFolders(context.Context, string, string) ([]api.ExternalMailFolder, error) {
	return []api.ExternalMailFolder{{ID: "INBOX", Role: "inbox"}}, nil
}

func (*correoFalso) MailMessages(context.Context, string, string, string, string) (api.ExternalMailPage, error) {
	return api.ExternalMailPage{}, nil
}

func (*correoFalso) MailMessage(context.Context, string, string, string, string) (api.ExternalMailMessage, error) {
	return api.ExternalMailMessage{}, nil
}

func (s *correoFalso) MailSend(_ context.Context, _ string, out api.ExternalMailOutgoing) (api.ExternalMailSent, error) {
	s.sends++
	s.out = out
	return s.sent, s.sendErr
}

func (*correoFalso) MailAction(context.Context, string, string, string, string, string, string) error {
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
	c := &correoExterno{b: b, ctx: ctx, cancelar: cancel, proveedor: "imap", smtpSecurity: "tls", instancia: "fixture", carpeta: "INBOX", campos: map[string]*ui.Entrada{}, cuenta: &api.ExternalMailAccount{InstanceID: "fixture", Provider: "imap", Address: "fixture@example.eu"}}
	for _, key := range []string{"name", "address", "host", "port", "username", "password", "smtpHost", "smtpPort", "smtpUser", "smtpPassword", "to", "cc", "bcc", "subject"} {
		c.campos[key] = ui.NuevaEntrada("")
	}
	c.texto = ui.NuevaAreaTexto("")
	c.cuerpo = container.NewVBox()
	c.pie = container.NewHBox()
	c.d = ui.NuevoDialogo("Fixture", c.cuerpo)
	return c
}

func TestExternalSendUnknownKeepsDraftAndNeverReplays(t *testing.T) {
	s := &correoFalso{sendErr: &api.Error{Status: 502, Body: `{"error":{"code":"mail_send_unknown"}}`}}
	c := correoDePrueba(t, s)
	c.redactando = true
	c.campos["to"].PonerTexto("Recipient <to@example.eu>")
	c.campos["bcc"].PonerTexto("blind@example.eu")
	c.texto.PonerTexto("synthetic draft")
	c.enviar()
	if s.sends != 1 || c.errorClave != "externalMail.sendUnknown" || c.texto.Texto() != "synthetic draft" || !c.redactando || s.out.Bcc[0].Email != "blind@example.eu" {
		t.Fatalf("ambiguous send replay/lost draft: sends %d error %s draft %s", s.sends, c.errorClave, c.texto.Texto())
	}
}

func TestExternalAcceptedSendDoesNotReplayOnSentCopyFailure(t *testing.T) {
	saved := false
	s := &correoFalso{sent: api.ExternalMailSent{SentCopySaved: &saved}}
	c := correoDePrueba(t, s)
	c.redactando = true
	c.campos["to"].PonerTexto("to@example.eu")
	c.texto.PonerTexto("synthetic")
	c.enviar()
	if s.sends != 1 || c.hayBorrador() || c.redactando || c.aviso != "externalMail.sentNoCopy" || c.errorClave != "" {
		t.Fatalf("accepted SMTP treated as failed: sends %d warning %s", s.sends, c.aviso)
	}
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

func TestExternalRecipientValidationNeverReachesSend(t *testing.T) {
	s := &correoFalso{sendErr: errors.New("must not run")}
	c := correoDePrueba(t, s)
	c.campos["to"].PonerTexto("not an address")
	c.enviar()
	if s.sends != 0 || c.errorClave != "externalMail.invalidRecipients" {
		t.Fatal("invalid recipient reached SMTP")
	}
}

func TestMailServicesRequireMailAndBlockUnsupportedNotes(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	c.proveedor = "gmail"
	c.cuenta = nil
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
	if attempts != 2 || c.instancia != "pending-account" || c.cuenta != nil {
		t.Fatal("denied draft was lost or falsely marked connected")
	}
}

func TestServiceOnlyAccountAddsMailBeforeOpeningItsMailbox(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	account := api.ExternalMailAccount{InstanceID: "linked-only", Provider: "microsoft", Services: []string{"calendar"}, Connected: true}
	c.cuenta = nil
	c.abrirCuenta(account)
	if c.cuenta != nil {
		t.Fatal("service-only account opened a mailbox without mail consent")
	}
	c.reconectar(account)
	if !slices.Contains(c.servicios, "mail") || !slices.Contains(c.servicios, "calendar") {
		t.Fatalf("Mail reconnection lost its required or existing service: %v", c.servicios)
	}
}
