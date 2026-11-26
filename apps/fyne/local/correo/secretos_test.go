// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build edicionlocal

package correo

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/asistente"
	almacen "git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

// Los secretos de prueba: que no aparezcan en ningún fichero de la edición es lo que se
// mira, así que son textos que no salen por casualidad.
const (
	direccionPrueba  = "ana@ejemplo.eu"
	contrasenaCorreo = "contraseña-de-app-7f3a91"
	contrasenaNube   = "clave-webdav-c08d2e"
	claveIA          = "sk-ant-prueba-5b61f4e0"
)

// TestLosSecretosNoEstanEnLosAjustes (R-CLARO, §8.1): tras dar de alta, por el código de
// alta de verdad y sin salir del equipo, una cuenta de correo con contraseña (la prueba
// de IMAP y SMTP del formulario de Mail, contra servidores en proceso), una cuenta de
// nube WebDAV (la de Ajustes → Nube, y se lista con ella) y una clave de IA (Ajustes →
// IA), ningún fichero de la carpeta personal —ajustes, caché y datos— contiene ninguno
// de los tres, salvo el almacén de secretos (`<ajustes>/secretos`, ficheros 0600; en el
// escritorio con llavero van a él). Y los tres se pueden leer del almacén.
//
// VISTO EN ROJO (2026-11-25): guardando la clave de la IA también en `ia.json` (el
// modelo), y la contraseña de la nube en el rótulo de su cuenta, nombró los dos ficheros.
func TestLosSecretosNoEstanEnLosAjustes(t *testing.T) {
	casa := t.TempDir()
	for clave, valor := range map[string]string{
		"HOME": casa, "USERPROFILE": casa, "XDG_CONFIG_HOME": filepath.Join(casa, ".config"),
		"XDG_CACHE_HOME": filepath.Join(casa, ".cache"), "APPDATA": filepath.Join(casa, ".config"),
		"LOCALAPPDATA": filepath.Join(casa, ".cache"), "SYTUE_OFFICE_DATOS": "",
	} {
		t.Setenv(clave, valor)
	}
	transporte, cliente, transporteAPI := http.DefaultTransport, http.DefaultClient, api.Transporte
	t.Cleanup(func() { http.DefaultTransport, http.DefaultClient, api.Transporte = transporte, cliente, transporteAPI })
	a := test.NewApp()
	t.Cleanup(a.Quit)
	l, err := local.Abrir(a, "mail")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Cerrar)
	// El mismo almacén que abre la cáscara, pero siempre en ficheros: una prueba no
	// escribe en el llavero de quien la corre. La IA y la nube, abiertas sobre él como en
	// local.Abrir, y con el cliente del portero.
	sec := secretos.SoloFichero(l.Secretos.Reserva())
	l.Secretos = sec
	l.IA, l.Nube = asistente.Abrir(l.Espacio, sec), nube.Abrir(l.Espacio, sec)
	l.IA.HTTP, l.Nube.HTTP = l.Red.Cliente(red.IA), l.Red.Cliente(red.Nube)
	ctx := t.Context()

	// Correo: la prueba y el alta del formulario de Mail (probarYAnadir), contra un IMAP
	// y un SMTP de este equipo, por el portero.
	imapDir, smtpDir := servidoresDeCorreo(t)
	b, err := almacen.Abrir(filepath.Join(l.Espacio.Rutas.Datos, "Correo"), l.Secretos,
		almacen.Red{Marcar: l.Red.Marcador(red.Correo), HTTP: l.Red.Cliente(red.OAuth)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Cerrar)
	g := &Gestor{L: l, B: b, W: a.NewWindow("Mail")}
	cuenta := almacen.Cuenta{
		Proveedor: "imap", Direccion: direccionPrueba, Acceso: almacen.AccesoContrasena,
		IMAP: almacen.Servidor{Host: "127.0.0.1", Puerto: imapDir, Seguridad: almacen.SeguridadNinguna},
		SMTP: almacen.Servidor{Host: "127.0.0.1", Puerto: smtpDir, Seguridad: almacen.SeguridadNinguna},
	}
	if err := g.probarYAnadir(ctx, cuenta, contrasenaCorreo); err != nil {
		t.Fatalf("alta de correo: %v", err)
	}
	if cs := b.Cuentas(); len(cs) != 1 {
		t.Fatalf("cuentas de correo: %+v", cs)
	}

	// Nube: el alta de Ajustes → Nube (l.Nube.Añadir) y la comprobación con la que la
	// confirma (listar), contra un WebDAV de este equipo.
	dav := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "ana" || p != contrasenaNube {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
	}))
	t.Cleanup(dav.Close)
	cn, err := l.Nube.Añadir(nube.Cuenta{Tipo: nube.WebDAV, URL: dav.URL + "/dav", Usuario: "ana"}, contrasenaNube)
	if err != nil {
		t.Fatalf("alta de nube: %v", err)
	}
	p, err := l.Nube.Proveedor(cn.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Su certificado es de prueba: lo que se comprueba es que entra con la contraseña
	// guardada, no el TLS del portero (que tiene sus propias pruebas).
	p.(*nube.DAV).HTTP = dav.Client()
	if _, err := p.Listar(ctx, ""); err != nil {
		t.Fatalf("la nube no entra con la contraseña guardada: %v", err)
	}

	// IA: Ajustes → IA con la clave de la persona.
	if err := l.IA.Guardar(asistente.Config{Proveedor: asistente.Anthropic, Modelo: "claude-de-prueba"}, claveIA); err != nil {
		t.Fatal(err)
	}
	if l.IA.Clave() != claveIA {
		t.Fatal("la clave de la IA no se lee del almacén")
	}
	// Lo que se escribe solo: las cuentas de toda la edición y la lista del portero.
	if cs := l.Cuentas.Cuentas(); len(cs) != 2 {
		t.Errorf("cuentas de la edición: %+v; quiero la de correo y la de nube", cs)
	}
	_ = l.Red.Destinos()
	if v, _ := sec.Leer("cuenta-" + b.Cuentas()[0].ID + "-contrasena"); v != contrasenaCorreo {
		t.Error("la contraseña del correo no se lee del almacén")
	}

	reserva := sec.Reserva()
	err = filepath.WalkDir(casa, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ruta == reserva {
				return filepath.SkipDir
			}
			return nil
		}
		crudo, err := os.ReadFile(ruta) //nolint:gosec // la carpeta personal de la prueba
		if err != nil {
			return err
		}
		for _, s := range []string{contrasenaCorreo, contrasenaNube, claveIA} {
			if bytes.Contains(crudo, []byte(s)) || bytes.Contains(crudo, []byte(base64.StdEncoding.EncodeToString([]byte(s)))) {
				rel, _ := filepath.Rel(casa, ruta)
				t.Errorf("%s contiene el secreto %q", rel, s[:8]+"…")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(reserva); len(es) != 3 {
		t.Errorf("el almacén de secretos tiene %d ficheros; quiero los tres secretos", len(es))
	}
}

// servidoresDeCorreo arranca un IMAP en memoria (el de la propia biblioteca) y un SMTP de
// prueba en 127.0.0.1, que aceptan solo la contraseña de prueba, y devuelve sus puertos.
func servidoresDeCorreo(t *testing.T) (imapPuerto, smtpPuerto int) {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser(direccionPrueba, contrasenaCorreo)
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(u)
	si := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
	})
	li, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = si.Serve(li) }()
	t.Cleanup(func() { _ = si.Close() })

	ss := smtp.NewServer(smtp.BackendFunc(func(*smtp.Conn) (smtp.Session, error) { return &sesionSMTP{}, nil }))
	ss.Domain, ss.AllowInsecureAuth = "localhost", true
	ls, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = ss.Serve(ls) }()
	t.Cleanup(func() { _ = ss.Close() })
	return li.Addr().(*net.TCPAddr).Port, ls.Addr().(*net.TCPAddr).Port
}

// sesionSMTP solo deja entrar con la contraseña de prueba; no se envía nada.
type sesionSMTP struct{}

func (*sesionSMTP) AuthMechanisms() []string { return []string{sasl.Plain} }
func (*sesionSMTP) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, u, p string) error {
		if u != direccionPrueba || p != contrasenaCorreo {
			return smtp.ErrAuthFailed
		}
		return nil
	}), nil
}
func (*sesionSMTP) Mail(string, *smtp.MailOptions) error { return nil }
func (*sesionSMTP) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (*sesionSMTP) Data(io.Reader) error                 { return nil }
func (*sesionSMTP) Reset()                               {}
func (*sesionSMTP) Logout() error                        { return nil }
