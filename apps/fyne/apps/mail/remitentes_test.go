// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// cuentaFixture es una cuenta IMAP que envía.
var cuentaFixture = api.ExternalMailAccount{InstanceID: "fixture", Provider: "imap", Address: "fixture@example.eu", Connected: true, Capabilities: api.ExternalMailCapabilities{Read: true, Send: true}}

// redactorDeCuenta abre el redactor con la cuenta de prueba en el «De».
func redactorDeCuenta(t *testing.T, s *correoFalso) (*Buzon, *redaccion) {
	t.Helper()
	b := correoDePrueba(t, s).b
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 744))
	b.cuentasDialogo = []api.ExternalMailAccount{cuentaFixture}
	b.abrirRedactor(Borrador{De: cuentaFixture.Address, Cuenta: cuentaFixture.InstanceID})
	rd := b.redaccion
	if rd == nil || rd.r.B.Cuenta != "fixture" {
		t.Fatal("el fixture no abrió el redactor con la cuenta en el «De»")
	}
	t.Cleanup(func() {
		if b.redaccion != nil {
			b.cerrar(b.redaccion)
		}
	})
	return b, rd
}

// avisoFinal es el título y la descripción del último aviso.
func avisoFinal(b *Buzon) (string, string) {
	av := b.P.Avisos.Avisos()
	if len(av) == 0 {
		return "", ""
	}
	return av[len(av)-1].Titulo, av[len(av)-1].Descripcion
}

// TestLosRemitentesSonLosDeLaWeb recorre testdata/remitentes.json, que recorre también
// remitentes.test.ts.
//
// VISTO EN ROJO (2026-11-06): sin mirar `Capabilities.Send`, salía la cuenta de
// Calendario; sin comparar sin mayúsculas, el alias salía dos veces; con `(0, false)` para
// el valor que no está, un valor desconectado no elegía nada. Deshecho.
func TestLosRemitentesSonLosDeLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/remitentes.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre   string
			Sytue    []string
			Externas []struct {
				InstanceID string `json:"instance_id"`
				Address    string
				Send       bool
			}
			Esperado []OpcionDe
			Elegir   []struct{ Valor, Esperado string }
		}
		SinImagenesCid []struct{ Nombre, HTML, Esperado string }
	}
	if err := json.Unmarshal(crudo, &v); err != nil || len(v.Casos) < 5 || len(v.SinImagenesCid) < 3 {
		t.Fatalf("vectores: %v", err)
	}
	for _, c := range v.Casos {
		t.Run(c.Nombre, func(t *testing.T) {
			cuentas := make([]api.ExternalMailAccount, 0, len(c.Externas))
			for _, e := range c.Externas {
				cuentas = append(cuentas, api.ExternalMailAccount{InstanceID: e.InstanceID, Address: e.Address, Capabilities: api.ExternalMailCapabilities{Send: e.Send}})
			}
			lista := remitentes(c.Sytue, cuentas)
			if !slices.Equal(lista, c.Esperado) {
				t.Fatalf("opciones %+v, quería %+v", lista, c.Esperado)
			}
			for _, e := range c.Elegir {
				if i, ok := remitenteElegido(e.Valor, lista); !ok || lista[i].Valor != e.Esperado {
					t.Errorf("elegir %q da %d (%v), quería %q", e.Valor, i, ok, e.Esperado)
				}
			}
		})
	}
	for _, c := range v.SinImagenesCid {
		if got := sinImagenesCid(c.HTML); got != c.Esperado {
			t.Errorf("%s: %q, quería %q", c.Nombre, got, c.Esperado)
		}
	}
}

// El «De» del redactor de Sytue lista los alias y, detrás, las cuentas externas que
// envían; elegir una oculta «Programar» (es de la cola de Sytue) y el correo sale por su
// proveedor, no por Sytue.
//
// VISTO EN ROJO (2026-11-06): con `NuevoRedactorEn` (solo Sytue) en abrirRedactor no
// salía la cuenta; sin la rama de `enviar`, el correo de la externa iba a S.Send con su
// dirección como «De»; sin `AlCambiarDe`, el reloj seguía a la vista; sin mirar `Cuenta`
// en `componer`, el borrador se guardaba en Sytue con la dirección externa. Deshecho.
func TestElDeOfreceLasCuentasExternasYEnviaPorSuProveedor(t *testing.T) {
	s := servicioConTodas()
	s.alias = []string{"ventas@example.org"}
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	c := &correoFalso{}
	b.ConCorreoExterno(c, nil)
	b.cuentasDialogo = []api.ExternalMailAccount{cuentaFixture, {InstanceID: "cal", Address: "cal@example.eu"}}
	b.Redactar()
	rd := b.redaccion
	if rd == nil || rd.r.filaDe == nil {
		t.Fatal("con alias y una cuenta externa no hay fila «De»")
	}
	if want := []string{"ada@example.org", "ventas@example.org", "fixture@example.eu"}; !slices.Equal(rd.r.OpcionesDe, want) {
		t.Fatalf("«De» %q, quería %q", rd.r.OpcionesDe, want)
	}
	selector := rd.r.filaDe.campo.(campoDe).s
	if !rd.reloj.Visible() || rd.r.B.Cuenta != "" {
		t.Fatal("desde Sytue el reloj de programar no se ve")
	}
	rd.r.AlternarProgramar()
	selector.AlElegir(2)
	if rd.r.B.De != "fixture@example.eu" || rd.r.B.Cuenta != "fixture" || rd.reloj.Visible() || rd.r.verProgramar {
		t.Fatalf("elegir la externa: de %q cuenta %q, reloj %v, panel %v", rd.r.B.De, rd.r.B.Cuenta, rd.reloj.Visible(), rd.r.verProgramar)
	}
	rd.r.B.Para, rd.r.B.Asunto, rd.r.B.Cuerpo = "Grace <grace@example.org>", "Hola", "Qué tal."
	rd.r.B.HTML = `<p>Qué tal.</p><img src="cid:i1">`
	rd.r.B.Adjuntos = []eml.Adjunto{{Nombre: "foto.png", Tipo: "image/png", Datos: []byte("png"), CID: "i1"}}
	// El borrador se guarda en Sytue con la principal: la dirección externa no es suya.
	b.guardarBorrador(rd, true, false)
	if s.pidio("SaveDraft") != 1 || !strings.Contains(s.guardado, "From: <ada@example.org>") || strings.Contains(s.guardado, "fixture@example.eu") {
		t.Fatalf("el borrador de la externa:\n%s", s.guardado)
	}
	b.enviar(rd, false)
	if s.pidio("Send") != 0 || c.sends != 1 {
		t.Fatalf("salió por Sytue (%v) o no salió por la cuenta (%d)", s.llamadas, c.sends)
	}
	o := c.out
	if o.ExpectedAddress != "fixture@example.eu" || o.To[0] != (api.ExternalMailAddress{Name: "Grace", Email: "grace@example.org"}) ||
		o.Subject != "Hola" || o.Text != "Qué tal." || o.HTML != "<p>Qué tal.</p>" ||
		len(o.Attachments) != 1 || o.Attachments[0].Name != "foto.png" || o.Attachments[0].ContentBase64 != "cG5n" {
		t.Fatalf("lo enviado: %+v", o)
	}
	if b.redaccion != nil {
		t.Fatal("el redactor sigue abierto tras enviar")
	}
	if titulo, _ := avisoFinal(b); titulo != traductor(t).T("externalMail.sent", nil) {
		t.Errorf("aviso %q", titulo)
	}
}

// Lo que el proveedor pudo aceptar sin decirlo (5xx) no se repite: el redactor se queda
// con lo escrito y el aviso pide mirar en Enviados.
//
// VISTO EN ROJO (2026-11-06): con errorCorreo también para el 502, el aviso decía «no se
// pudo» e invitaba a reenviar. Deshecho.
func TestExternalSendUnknownKeepsDraftAndNeverReplays(t *testing.T) {
	s := &correoFalso{sendErr: &api.Error{Status: 502, Body: `{"error":{"code":"mail_send_unknown"}}`}}
	b, rd := redactorDeCuenta(t, s)
	rd.r.B.Para, rd.r.B.Cco, rd.r.B.Cuerpo = "Recipient <to@example.eu>", "blind@example.eu", "synthetic draft"
	b.enviar(rd, false)
	titulo, _ := avisoFinal(b)
	if s.sends != 1 || titulo != traductor(t).T("externalMail.sendUnknown", nil) || b.redaccion != rd || rd.r.B.Cuerpo != "synthetic draft" ||
		rd.enviando || s.out.Bcc[0].Email != "blind@example.eu" || s.out.ExpectedAddress != "fixture@example.eu" {
		t.Fatalf("ambiguous send replay/lost draft: sends %d aviso %q", s.sends, titulo)
	}
}

func TestExternalAcceptedSendDoesNotReplayOnSentCopyFailure(t *testing.T) {
	saved := false
	s := &correoFalso{sent: api.ExternalMailSent{SentCopySaved: &saved}}
	b, rd := redactorDeCuenta(t, s)
	rd.r.B.Para, rd.r.B.Cuerpo = "to@example.eu", "synthetic"
	b.enviar(rd, false)
	if titulo, _ := avisoFinal(b); s.sends != 1 || b.redaccion != nil || titulo != traductor(t).T("externalMail.sentNoCopy", nil) {
		t.Fatalf("accepted SMTP treated as failed: sends %d aviso %q", s.sends, titulo)
	}
}

func TestExternalRecipientValidationNeverReachesSend(t *testing.T) {
	s := &correoFalso{sendErr: errors.New("must not run")}
	b, rd := redactorDeCuenta(t, s)
	rd.r.B.Para = "not an address"
	b.enviar(rd, false)
	if _, desc := avisoFinal(b); s.sends != 0 || desc != traductor(t).T("compose.sendInvalid", nil) {
		t.Fatalf("invalid recipient reached SMTP: %d, %q", s.sends, desc)
	}
}

// Los adjuntos que no caben por la cuenta no llegan a salir.
func TestExternalAttachmentsOverTheLimitNeverReachSend(t *testing.T) {
	s := &correoFalso{}
	b, rd := redactorDeCuenta(t, s)
	rd.r.B.Para, rd.r.B.Cuerpo = "to@example.eu", "synthetic"
	rd.r.B.Adjuntos = []eml.Adjunto{{Nombre: "grande.bin", Tipo: "application/octet-stream", Datos: make([]byte, limiteAdjuntosExternos+1)}}
	b.enviar(rd, false)
	if _, desc := avisoFinal(b); s.sends != 0 || desc != traductor(t).T("externalMail.tooLarge", nil) || b.redaccion != rd {
		t.Fatalf("oversized send: %d, %q", s.sends, desc)
	}
}

// Solo un envío en camino puede haber salido: al volver a la identidad, el redactor
// recuperado lo dice y conserva su «De» aunque las cuentas aún no hayan llegado.
//
// VISTO EN ROJO (2026-11-06): sin la opción que conserva la cuenta en NuevoRedactorDe,
// el borrador recuperado volvía con «De» de Sytue. Deshecho.
func TestCambiarDeCuentaConservaElDeYDaPorInciertoElEnvio(t *testing.T) {
	b := correoDePrueba(t, &correoFalso{}).b
	test.NewTempWindow(t, b.P.Objeto())
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	b.cuentasDialogo = []api.ExternalMailAccount{cuentaFixture}
	b.abrirRedactor(Borrador{De: cuentaFixture.Address, Cuenta: cuentaFixture.InstanceID, Para: "bea@example.eu"})
	b.Lanzar = func(func()) {} // la petición se queda en camino
	b.enviar(b.redaccion, false)
	if !b.redaccion.enviando {
		t.Fatal("el fixture no dejó el envío en camino")
	}
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-b")
	b.Lanzar = func(f func()) { f() }
	b.CambiarSesion(context.Background(), "emisor-a\x00persona\x00org-a")
	rd := b.redaccion
	if rd == nil || rd.r.B.Cuenta != "fixture" || rd.r.B.De != "fixture@example.eu" {
		t.Fatalf("el redactor recuperado perdió su «De»: %+v", rd)
	}
	if titulo, _ := avisoFinal(b); titulo != traductor(t).T("externalMail.sendUnknown", nil) {
		t.Fatalf("al volver, el aviso dice %q", titulo)
	}
}
