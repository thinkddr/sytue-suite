// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
)

// Un SMTP de mentira en 127.0.0.1, con PLAIN: lo que llega se guarda tal cual.
// marcarDirecto marca sin portero: el SMTP de estas pruebas escucha en este equipo. Sin
// Red.Marcar el correo no sale (R-RED).
func marcarDirecto(ctx context.Context, red, direccion string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, red, direccion)
}

type smtpFalso struct {
	mu        sync.Mutex
	recibidos []smtpRecibido
	ln        net.Listener
}

type smtpRecibido struct {
	de    string
	para  []string
	datos []byte
}

type sesionFalsa struct {
	s  *smtpFalso
	r  smtpRecibido
	ok bool
}

func (x *sesionFalsa) AuthMechanisms() []string { return []string{sasl.Plain} }
func (x *sesionFalsa) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, u, p string) error {
		if u != "ana@ejemplo.eu" || p != "contraseña-de-app" {
			return errors.New("credenciales")
		}
		x.ok = true
		return nil
	}), nil
}

func (x *sesionFalsa) Mail(de string, _ *smtp.MailOptions) error {
	if !x.ok {
		return smtp.ErrAuthRequired
	}
	x.r.de = de
	return nil
}

func (x *sesionFalsa) Rcpt(para string, _ *smtp.RcptOptions) error {
	x.r.para = append(x.r.para, para)
	return nil
}

func (x *sesionFalsa) Data(r io.Reader) error {
	d, err := io.ReadAll(r)
	x.r.datos = d
	x.s.mu.Lock()
	x.s.recibidos = append(x.s.recibidos, x.r)
	x.s.mu.Unlock()
	return err
}
func (x *sesionFalsa) Reset()        { x.r = smtpRecibido{} }
func (x *sesionFalsa) Logout() error { return nil }

func nuevoSMTPFalso(t *testing.T) *smtpFalso {
	t.Helper()
	s := &smtpFalso{}
	srv := smtp.NewServer(smtp.BackendFunc(func(*smtp.Conn) (smtp.Session, error) { return &sesionFalsa{s: s}, nil }))
	srv.Domain, srv.AllowInsecureAuth = "localhost", true
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.ln = ln
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return s
}

func (s *smtpFalso) lista() []smtpRecibido {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]smtpRecibido(nil), s.recibidos...)
}

type llaveroFalso struct{ m map[string]string }

func (l *llaveroFalso) Leer(n string) (string, error) { return l.m[n], nil }
func (l *llaveroFalso) Guardar(n, v string) error     { l.m[n] = v; return nil }
func (l *llaveroFalso) Borrar(n string) error         { delete(l.m, n); return nil }

// cuentaDeMail da de alta en `raiz` (la carpeta de Mail) una cuenta que envía por el
// SMTP de mentira, como lo haría Sytue Mail, y la cierra: el ERP no abre su buzón.
func cuentaDeMail(t *testing.T, raiz string, llavero *llaveroFalso, s *smtpFalso) correo.Cuenta {
	t.Helper()
	b, err := correo.Abrir(raiz, llavero, correo.Red{Marcar: marcarDirecto})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cerrar()
	c, err := b.Anadir(correo.Cuenta{
		Proveedor: correo.ProveedorIMAP, Direccion: "ana@ejemplo.eu", Nombre: "Ana", Rotulo: "Trabajo",
		IMAP: correo.Servidor{Host: "127.0.0.1", Puerto: 1, Seguridad: correo.SeguridadNinguna},
		SMTP: correo.Servidor{Host: "127.0.0.1", Puerto: s.ln.Addr().(*net.TCPAddr).Port, Seguridad: correo.SeguridadNinguna},
	}, "contraseña-de-app")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// facturaParaEnviar monta el ERP con su organización, un cliente con dirección de
// facturación y una factura en borrador.
func facturaParaEnviar(t *testing.T) (*api.ERP, *Servidor, api.ERPRegistro) {
	t.Helper()
	ctx := context.Background()
	cat, obl, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	alm, err := Abrir(t.TempDir(), cat)
	if err != nil {
		t.Fatal(err)
	}
	s := NuevoServidor(cat, obl, alm)
	e := api.NewERP(Base, nil, s.Cliente(), "es")
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{Nombre: "Acme Ibérica SL", NIF: "B12345678", Pais: "ES", Forma: "sl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CambiarModulo(ctx, "facturacion", true); err != nil {
		t.Fatal(err)
	}
	cli, err := e.Crear(ctx, "contacto", map[string]any{
		"nombre": "Ferretería Óscar", "tipo": "organizacion",
		"correo": "hola@ferreteria.es", "email_facturas": "pagos@ferreteria.es",
	})
	if err != nil {
		t.Fatal(err)
	}
	fac, err := e.Crear(ctx, "factura_venta", map[string]any{
		"cliente": cli.ID, "fecha": "2026-05-04", "vencimiento": "2026-06-03",
		"base": 100000, "iva": 21000, "total": 121000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e, s, fac
}

// De extremo a extremo: el diálogo arranca con la dirección de facturación del cliente,
// las cuentas de Mail y la plantilla; el correo sale por el SMTP de la cuenta con el PDF
// adjunto y queda en el historial con el texto del servicio.
func TestEnviarFacturaPorLaCuentaDeMail(t *testing.T) {
	ctx := context.Background()
	e, s, fac := facturaParaEnviar(t)
	srv := nuevoSMTPFalso(t)
	raizMail, llavero := t.TempDir(), &llaveroFalso{m: map[string]string{}}

	// Sin cuentas de correo: el diálogo lo sabe y enviar contesta 503, como un
	// despliegue sin SMTP.
	d, err := e.EnvioFactura(ctx, "factura_venta", fac.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Disponible || !d.PorCuenta || len(d.Cuentas) != 0 || d.Enviable || d.Para != "pagos@ferreteria.es" {
		t.Fatalf("sin cuentas: %+v", d)
	}
	correoFactura := api.ERPCorreoFactura{Para: "pagos@ferreteria.es", Asunto: "Factura", Mensaje: "Hola"}
	if err := e.EnviarCorreoFactura(ctx, "factura_venta", fac.ID, correoFactura); codigo(err) != "correo_no_configurado" || estado(err) != http.StatusServiceUnavailable {
		t.Fatalf("sin cuentas, enviar: %v", err)
	}

	cuenta := cuentaDeMail(t, raizMail, llavero, srv)
	s.PonerCorreo(correo.NuevoRemitente(raizMail, llavero, correo.Red{Marcar: marcarDirecto}))
	d, err = e.EnvioFactura(ctx, "factura_venta", fac.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Disponible || len(d.Cuentas) != 1 || d.Cuentas[0].ID != cuenta.ID || d.Cuentas[0].Direccion != "ana@ejemplo.eu" || d.Cuentas[0].Nombre != "Trabajo" {
		t.Fatalf("cuentas: %+v", d)
	}
	if d.Asunto != "Factura FV-2026-00001 de Acme Ibérica SL" {
		t.Errorf("asunto propuesto: %q", d.Asunto)
	}
	for _, quiero := range []string{"FV-2026-00001", "1.210,00 €", "03/06/2026", "Acme Ibérica SL"} {
		if !strings.Contains(d.Mensaje, quiero) {
			t.Errorf("el texto propuesto no lleva %q:\n%s", quiero, d.Mensaje)
		}
	}

	// Un borrador no se envía.
	if err := e.EnviarCorreoFactura(ctx, "factura_venta", fac.ID, correoFactura); codigo(err) != "factura_no_enviable" {
		t.Fatalf("borrador: %v", err)
	}
	if len(srv.lista()) != 0 {
		t.Fatal("un borrador llegó al servidor de correo")
	}
	if _, err := e.CambiarEstado(ctx, "factura_venta", fac.ID, "confirmado"); err != nil {
		t.Fatal(err)
	}
	// Lo que escribe la persona se comprueba antes de tocar la red.
	for _, mal := range []api.ERPCorreoFactura{
		{Para: "no es una dirección", Asunto: "x"},
		{Para: "pagos@ferreteria.es", Asunto: "  "},
		{Para: "pagos@ferreteria.es", Asunto: "x", Cuenta: "otra"},
	} {
		if err := e.EnviarCorreoFactura(ctx, "factura_venta", fac.ID, mal); err == nil {
			t.Errorf("%+v: debía rechazarse", mal)
		}
	}

	correoFactura = api.ERPCorreoFactura{Para: "pagos@ferreteria.es", Asunto: d.Asunto, Mensaje: d.Mensaje, Cuenta: cuenta.ID}
	if err := e.EnviarCorreoFactura(ctx, "factura_venta", fac.ID, correoFactura); err != nil {
		t.Fatal(err)
	}
	got := srv.lista()
	if len(got) != 1 || got[0].de != "ana@ejemplo.eu" || len(got[0].para) != 1 || got[0].para[0] != "pagos@ferreteria.es" {
		t.Fatalf("sobre: %+v", got)
	}
	m, err := eml.Leer(bytes.NewReader(got[0].datos))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Texto, "1.210,00 €") || len(m.Adjuntos) != 1 || m.Adjuntos[0].Nombre != "FV-2026-00001.pdf" ||
		!bytes.HasPrefix(m.Adjuntos[0].Datos, []byte("%PDF-")) {
		t.Fatalf("mensaje: %q, adjuntos %+v", m.Texto, len(m.Adjuntos))
	}
	cab, err := mail.ReadMessage(bytes.NewReader(got[0].datos))
	if err != nil {
		t.Fatal(err)
	}
	if de, err := mail.ParseAddress(cab.Header.Get("From")); err != nil || de.Address != "ana@ejemplo.eu" || de.Name != "Acme Ibérica SL" {
		t.Errorf("remitente: %v %v", de, err)
	}

	// Queda en el historial, con el texto del servicio.
	h, err := e.Historial(ctx, "factura_venta", fac.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) == 0 || h[0].Tipo != "nota" || h[0].Texto != "Enviada por correo a pagos@ferreteria.es, con el PDF adjunto." {
		t.Fatalf("historial: %+v", h)
	}
}

// correoQueFalla devuelve un error de Mail al enviar.
type correoQueFalla struct{ err error }

func (c correoQueFalla) Cuentas() ([]correo.Cuenta, error) {
	return []correo.Cuenta{{ID: "c1", Direccion: "ana@ejemplo.eu"}}, nil
}
func (c correoQueFalla) Enviar(context.Context, string, []string, []byte) error { return c.err }

// Cuando no sale, el código dice por qué (la pantalla lo cuenta con su texto) y no se
// apunta nada en el historial.
func TestEnviarFacturaDicePorQueNoSalio(t *testing.T) {
	ctx := context.Background()
	e, s, fac := facturaParaEnviar(t)
	if _, err := e.CambiarEstado(ctx, "factura_venta", fac.ID, "confirmado"); err != nil {
		t.Fatal(err)
	}
	antes, _ := e.Historial(ctx, "factura_venta", fac.ID)
	c := api.ERPCorreoFactura{Para: "pagos@ferreteria.es", Asunto: "Factura"}
	for err, quiero := range map[error]string{
		correo.ErrAcceso:        CodigoCorreoAcceso,
		correo.ErrSinRed:        CodigoCorreoSinRed,
		correo.ErrEnvioIncierto: CodigoCorreoIncierto,
		correo.ErrRechazado:     "correo_no_enviado",
		correo.ErrSinCuenta:     CodigoCuentaCorreo,
	} {
		s.PonerCorreo(correoQueFalla{err: err})
		if got := e.EnviarCorreoFactura(ctx, "factura_venta", fac.ID, c); codigo(got) != quiero || estado(got) != http.StatusBadGateway {
			t.Errorf("%v: %v", err, got)
		}
	}
	if despues, _ := e.Historial(ctx, "factura_venta", fac.ID); len(despues) != len(antes) {
		t.Errorf("lo que no salió quedó apuntado: %+v", despues)
	}
}

// La plantilla: con vencimiento y sin él, en el idioma de quien la manda (los 26 de la
// suite; uno que no es de la suite, en inglés).
func TestPlantillaEnvio(t *testing.T) {
	a, m := PlantillaEnvio("en-GB", DatosEnvio{Numero: "FV-1", Importe: "12,10 €", Vence: "01/07/2026", Emisor: "Acme"})
	if a != "Invoice FV-1 from Acme" || !strings.Contains(m, "due on 01/07/2026") || !strings.HasSuffix(m, "Acme") {
		t.Errorf("en: %q\n%s", a, m)
	}
	a, m = PlantillaEnvio("es", DatosEnvio{Numero: "FV-1", Importe: "12,10 €"})
	if a != "Factura FV-1" || strings.Contains(m, "vencimiento") || !strings.Contains(m, "12,10 €") {
		t.Errorf("es sin vencimiento: %q\n%s", a, m)
	}
	a, m = PlantillaEnvio("fr-BE", DatosEnvio{Numero: "FV-1", Importe: "12,10 €", Vence: "01/07/2026"})
	if a != "Facture FV-1" || !strings.HasPrefix(m, "Bonjour,") || !strings.Contains(m, "12,10 €") || !strings.Contains(m, "01/07/2026") {
		t.Errorf("fr: %q\n%s", a, m)
	}
	if a, _ = PlantillaEnvio("xx", DatosEnvio{Numero: "FV-1"}); a != "Invoice FV-1" {
		t.Errorf("un idioma que no es de la suite no cae al inglés: %q", a)
	}
}
