// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cuentas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

func escribir(t *testing.T, ruta, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Las cuentas de Mail, Rooms y la nube se ven juntas, sin secretos, y el modelo común
// queda escrito; los ficheros de cada app no se tocan.
func TestJuntaLasCuentasDeCadaApp(t *testing.T) {
	ajustes, datos := t.TempDir(), t.TempDir()
	correo := `[{"id":"c1","proveedor":"microsoft","direccion":"ana@ejemplo.eu","imap":{"host":"outlook.office365.com","puerto":993,"seguridad":"tls"},"smtp":{"host":"smtp.office365.com","puerto":587,"seguridad":"starttls"},"acceso":"oauth-microsoft","estado":"reconectar"},
{"id":"c2","proveedor":"imap","direccion":"yo@casa.lan","imap":{"host":"casa.lan","puerto":143,"seguridad":"ninguna"},"smtp":{"host":"casa.lan","puerto":25,"seguridad":"ninguna"},"acceso":"contrasena"}]`
	escribir(t, filepath.Join(datos, "Correo", "cuentas.json"), correo)
	escribir(t, filepath.Join(ajustes, "chat-cuentas.json"), `[{"id":"m1","proveedor":"matrix","rotulo":"@ana:ejemplo.org","servidor":"https://matrix.ejemplo.org","usuario":"@ana:ejemplo.org"}]`)
	escribir(t, filepath.Join(ajustes, "nube.json"), `{"cuentas":[{"id":"n1","tipo":"webdav","rotulo":"Nextcloud","url":"https://nube.ejemplo.eu/remote.php/dav/files/ana","usuario":"ana"}]}`)

	r := Abrir(ajustes, datos)
	cs := r.Cuentas()
	if len(cs) != 4 {
		t.Fatalf("cuentas: %+v", cs)
	}
	if cs[0].Origen != OrigenCorreo || cs[0].Estado != EstadoReconectar || cs[0].OAuth != HostOAuthMicrosoft {
		t.Errorf("correo de Outlook: %+v", cs[0])
	}
	if cs[2].Origen != OrigenChat || cs[2].Servidores[0].Host != "matrix.ejemplo.org" {
		t.Errorf("chat: %+v", cs[2])
	}
	if cs[3].Origen != OrigenNube || cs[3].Servidores[0].Host != "nube.ejemplo.eu" {
		t.Errorf("nube: %+v", cs[3])
	}
	raw, err := os.ReadFile(filepath.Join(ajustes, "cuentas.json"))
	if err != nil || !strings.Contains(string(raw), `"origen": "chat"`) {
		t.Fatalf("cuentas.json: %s, %v", raw, err)
	}
	if orig, _ := os.ReadFile(filepath.Join(datos, "Correo", "cuentas.json")); string(orig) != correo {
		t.Error("se tocó el fichero de Mail")
	}

	// Lo que el portero deja: los servidores de cada cuenta, en claro solo el marcado.
	p := red.Abrir("")
	p.Fuente(r.Destinos)
	for _, caso := range []struct {
		pr     red.Proposito
		host   string
		puerto int
		tls    bool
		vale   bool
	}{
		{red.Correo, "outlook.office365.com", 993, true, true},
		{red.Correo, "smtp.office365.com", 587, true, true},
		{red.OAuth, HostOAuthMicrosoft, 443, true, true},
		{red.Correo, "casa.lan", 143, false, true},
		{red.Chat, "matrix.ejemplo.org", 443, true, true},
		{red.Chat, "matrix.ejemplo.org", 80, false, false},
		{red.Nube, "nube.ejemplo.eu", 443, true, true},
		{red.Correo, "imap.gmail.com", 993, true, false},
	} {
		if err := p.Permitido(caso.pr, caso.host, caso.puerto, caso.tls); (err == nil) != caso.vale {
			t.Errorf("%s %s:%d: %v", caso.pr, caso.host, caso.puerto, err)
		}
	}
}

// Una cuenta que otra app añade después se ve sin volver a abrir.
func TestSeReleeAlCambiarUnOrigen(t *testing.T) {
	ajustes, datos := t.TempDir(), t.TempDir()
	r := Abrir(ajustes, datos)
	if len(r.Cuentas()) != 0 {
		t.Fatal("cuentas sin configurar nada")
	}
	ruta := filepath.Join(ajustes, "chat-cuentas.json")
	escribir(t, ruta, `[{"id":"m1","proveedor":"matrix","servidor":"https://matrix.ejemplo.org","usuario":"@ana:ejemplo.org"}]`)
	futuro := time.Now().Add(time.Second)
	_ = os.Chtimes(ruta, futuro, futuro)
	if cs := r.Cuentas(); len(cs) != 1 || cs[0].Rotulo != "@ana:ejemplo.org" {
		t.Fatalf("después del alta en Rooms: %+v", cs)
	}
}

// Mail pasa su cuenta en prueba al modelo común para abrirle el portero.
func TestDeCorreo(t *testing.T) {
	c, err := DeCorreo([]byte(`{"id":"x","direccion":"a@b.eu","imap":{"host":"imap.b.eu","puerto":993,"seguridad":"tls"},"smtp":{"host":"smtp.b.eu","puerto":465,"seguridad":"tls"}}`))
	if err != nil {
		t.Fatal(err)
	}
	ds := DestinosDe(c)
	if len(ds) != 2 || ds[0].Host != "imap.b.eu" || ds[1].Puerto != 465 || ds[0].Claro {
		t.Fatalf("destinos: %+v", ds)
	}
}

// Las cuentas de Teams y de Google Chat de Rooms abren al portero sus API y su servidor
// de tokens (y nada más), dan también reuniones y dicen si hay que volver a entrar.
func TestLasCuentasDeTeamsYGoogleAbrenSusServidores(t *testing.T) {
	ajustes, datos := t.TempDir(), t.TempDir()
	escribir(t, filepath.Join(ajustes, "chat-cuentas.json"), `[
{"id":"t1","proveedor":"teams","rotulo":"Teams · ana@empresa.eu","usuario":"ana@empresa.eu","servidores":["https://graph.microsoft.com/v1.0"],"oauth":"login.microsoftonline.com","estado":"reconectar"},
{"id":"g1","proveedor":"google-chat","rotulo":"Google Chat · ana@empresa.eu","servidores":["https://chat.googleapis.com/v1","https://meet.googleapis.com/v2","http://127.0.0.1:8787/v1"],"oauth":"oauth2.googleapis.com"},
{"id":"x1","proveedor":"teams","rotulo":"mal","servidores":["http://graph.ejemplo.eu"],"oauth":".ejemplo.eu"}]`)
	cs := Abrir(ajustes, datos).Cuentas()
	if len(cs) != 3 || cs[0].Estado != EstadoReconectar || cs[1].Estado != EstadoOK || cs[0].OAuth != "login.microsoftonline.com" || cs[2].OAuth != "" {
		t.Fatalf("cuentas: %+v", cs)
	}
	if len(cs[0].Servicios) != 2 || cs[0].Servicios[1] != Reuniones {
		t.Errorf("servicios de Teams: %v", cs[0].Servicios)
	}
	p := red.Abrir("")
	p.Fuente(Abrir(ajustes, datos).Destinos)
	for _, caso := range []struct {
		pr     red.Proposito
		host   string
		puerto int
		tls    bool
		vale   bool
	}{
		{red.Chat, "graph.microsoft.com", 443, true, true},
		{red.OAuth, "login.microsoftonline.com", 443, true, true},
		{red.Chat, "chat.googleapis.com", 443, true, true},
		{red.Chat, "meet.googleapis.com", 443, true, true},
		{red.OAuth, "oauth2.googleapis.com", 443, true, true},
		{red.Chat, "127.0.0.1", 8787, false, true},
		{red.Chat, "graph.ejemplo.eu", 80, false, false},
		{red.Chat, "login.microsoftonline.com", 443, true, false},
	} {
		if err := p.Permitido(caso.pr, caso.host, caso.puerto, caso.tls); (err == nil) != caso.vale {
			t.Errorf("%s %s:%d: %v", caso.pr, caso.host, caso.puerto, err)
		}
	}
}
