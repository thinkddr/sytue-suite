// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	almacen "git.sytue.eu/sytue/one/apps/nucleo/local/correo"
)

type abridorDoble struct{ urls []string }

func (a *abridorDoble) OpenURL(u *url.URL) error { a.urls = append(a.urls, u.String()); return nil }

// TestEnlacesQueAbrenElRedactor: `mailto:` y `sytueoffice-mail:/redactar?…`.
func TestEnlacesQueAbrenElRedactor(t *testing.T) {
	for enlace, quiero := range map[string]api.Redactar{
		"mailto:ana@ejemplo.eu?subject=Hola%20Ana&body=Qu%C3%A9%20tal&to=luis@ejemplo.eu": {Para: "ana@ejemplo.eu, luis@ejemplo.eu", Asunto: "Hola Ana", Cuerpo: "Qué tal"},
		"sytueoffice-mail:/redactar?para=ana@ejemplo.eu&asunto=Factura&cuerpo=Adjunta":    {Para: "ana@ejemplo.eu", Asunto: "Factura", Cuerpo: "Adjunta"},
		"sytueoffice-mail:/?action=compose&to=ana@ejemplo.eu&subject=X":                   {Para: "ana@ejemplo.eu", Asunto: "X"},
	} {
		u, _ := url.Parse(enlace)
		r, ok := LeerEnlace(u, almacen.ParaEnviar{})
		if !ok || r.Para != quiero.Para || r.Asunto != quiero.Asunto || r.Cuerpo != quiero.Cuerpo {
			t.Errorf("%s: %+v %v", enlace, r, ok)
		}
	}
	for _, no := range []string{"sytueoffice-mail:/abrir?local=file:///x.eml", "https://ejemplo.eu", "sytue-mail:/?action=compose"} {
		u, _ := url.Parse(no)
		if _, ok := LeerEnlace(u, almacen.ParaEnviar{}); ok {
			t.Errorf("%s no pide redactar", no)
		}
	}
	// Una dirección mal escrita no entra (ni una cabecera colada).
	u, _ := url.Parse("mailto:ana@ejemplo.eu%0D%0ABcc:x@y.eu")
	if r, _ := LeerEnlace(u, almacen.ParaEnviar{}); strings.Contains(r.Para, "Bcc") {
		t.Errorf("cabecera colada: %q", r.Para)
	}
}

// leerAdjuntos lee el enlace y dice lo adjuntado y lo que no.
func leerAdjuntos(t *testing.T, enlace string, salida almacen.ParaEnviar) api.Redactar {
	t.Helper()
	u, err := url.Parse(enlace)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := LeerEnlace(u, salida)
	if !ok {
		t.Fatalf("%s no abre el redactor", enlace)
	}
	return r
}

// TestElEnlaceSoloAdjuntaLoQueSeDejo: el enlace de redactar de Sytue Office puede venir
// de cualquier sitio (una web, un correo). Adjunta lo que otra app de este equipo dejó en
// «para-enviar» por su identificador, una sola vez, y nada más: ni una ruta de fuera, ni
// `..`, ni un enlace simbólico que salga de la carpeta, ni un identificador inventado o
// ya usado. Lo que no adjunta lo cuenta, para que el redactor lo diga.
func TestElEnlaceSoloAdjuntaLoQueSeDejo(t *testing.T) {
	salida := almacen.ParaEnviar{Dir: filepath.Join(t.TempDir(), "para-enviar")}
	pdf := []byte("%PDF-1.7 factura FV-2026-00001")
	id, err := salida.Dejar("FV-2026-00001.pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	enlace := almacen.EnlaceRedactar("pagos@cliente.es", "Factura FV-2026-00001", "Adjunta la factura.", id)
	r := leerAdjuntos(t, enlace, salida)
	if r.Para != "pagos@cliente.es" || r.Asunto != "Factura FV-2026-00001" || r.Cuerpo != "Adjunta la factura." ||
		len(r.Adjuntos) != 1 || r.SinAdjuntar != 0 {
		t.Fatalf("redactar: %+v", r)
	}
	if a := r.Adjuntos[0]; a.Nombre != "FV-2026-00001.pdf" || a.Tipo != "application/pdf" || !bytes.Equal(a.Datos, pdf) {
		t.Fatalf("adjunto: %q %q %q", a.Nombre, a.Tipo, a.Datos)
	}
	// Recogido, ya no está en la carpeta, y el mismo enlace otra vez no adjunta nada.
	if es, _ := os.ReadDir(salida.Dir); len(es) != 0 {
		t.Errorf("quedó en la carpeta: %v", es)
	}
	if r := leerAdjuntos(t, enlace, salida); len(r.Adjuntos) != 0 || r.SinAdjuntar != 1 {
		t.Errorf("identificador reutilizado: %+v", r)
	}

	// Un fichero cualquiera del disco, fuera de la carpeta.
	fuera := t.TempDir()
	secreto := filepath.Join(fuera, "secreto.txt")
	if err := os.WriteFile(secreto, []byte("clave privada"), 0o600); err != nil {
		t.Fatal(err)
	}
	inventado := "ABCDEFGHIJKLMNOPQRSTUVWXYZ" // con la forma de uno, pero nadie lo dejó
	for _, adjunto := range []string{
		secreto, "file://" + secreto, "../../" + filepath.Base(fuera) + "/secreto.txt",
		"../../" + filepath.Base(fuera), fuera, // una carpeta de fuera con un solo fichero: como una entrega
		"..", "../para-enviar", "." + string(filepath.Separator) + id, inventado, strings.ToLower(inventado), id + "/FV-2026-00001.pdf",
	} {
		enlace := almacen.EnlaceRedactar("a@b.eu", "x", "", adjunto)
		if r := leerAdjuntos(t, enlace, salida); len(r.Adjuntos) != 0 || r.SinAdjuntar != 1 {
			t.Errorf("%q se adjuntó: %+v", adjunto, r)
		}
	}
	// Escrito a mano, sin pasar por EnlaceRedactar.
	if r := leerAdjuntos(t, "sytueoffice-mail:/redactar?para=a@b.eu&adjunto=..%2F..%2Fetc%2Fpasswd&adjunto=%2Fetc%2Fpasswd", salida); len(r.Adjuntos) != 0 || r.SinAdjuntar != 2 {
		t.Errorf("rutas escritas a mano: %+v", r)
	}
	// `mailto:` no adjunta nada, ni siquiera lo que sí está en la carpeta.
	id2, err := salida.Dejar("otra.pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if r := leerAdjuntos(t, "mailto:a@b.eu?adjunto="+id2+"&attach="+id2, salida); len(r.Adjuntos) != 0 {
		t.Errorf("mailto adjuntó: %+v", r)
	}

	if runtime.GOOS == "windows" {
		return // los enlaces simbólicos piden permisos en Windows
	}
	// Un enlace simbólico con nombre de entrega que lleva a una carpeta de fuera.
	falso := "QRSTUVWXYZ234567ABCDEFGHIJ"
	if err := os.Symlink(fuera, filepath.Join(salida.Dir, falso)); err != nil {
		t.Fatal(err)
	}
	if r := leerAdjuntos(t, almacen.EnlaceRedactar("a@b.eu", "x", "", falso), salida); len(r.Adjuntos) != 0 || r.SinAdjuntar != 1 {
		t.Errorf("carpeta enlazada: %+v", r)
	}
	// Una entrega de verdad cuyo fichero es un enlace simbólico a uno de fuera.
	falso2 := "JIHGFEDCBAZYXWVUTSRQPONMLK"
	if err := os.Mkdir(filepath.Join(salida.Dir, falso2), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secreto, filepath.Join(salida.Dir, falso2, "factura.pdf")); err != nil {
		t.Fatal(err)
	}
	if r := leerAdjuntos(t, almacen.EnlaceRedactar("a@b.eu", "x", "", falso2), salida); len(r.Adjuntos) != 0 || r.SinAdjuntar != 1 {
		t.Errorf("fichero enlazado: %+v", r)
	}
	// Ni lo de fuera se ha tocado.
	if datos, err := os.ReadFile(secreto); err != nil || string(datos) != "clave privada" {
		t.Fatalf("se tocó lo de fuera: %q %v", datos, err)
	}
	// Y si la propia carpeta es un enlace simbólico a otra, no se recoge nada de ella.
	otra := filepath.Join(t.TempDir(), "para-enviar")
	if err := os.Symlink(salida.Dir, otra); err != nil {
		t.Fatal(err)
	}
	if r := leerAdjuntos(t, almacen.EnlaceRedactar("a@b.eu", "x", "", id2), almacen.ParaEnviar{Dir: otra}); len(r.Adjuntos) != 0 || r.SinAdjuntar != 1 {
		t.Errorf("carpeta de salida enlazada: %+v", r)
	}
}

// TestCrearEventoAbreCalendarLocal: el intent compone el compositor de Calendar y el
// abridor lo manda por `sytueoffice-calendar:`, sin web.
func TestCrearEventoAbreCalendarLocal(t *testing.T) {
	r, err := Intents{}.ResolveIntent(context.Background(), "create-event-from-mail",
		map[string]string{"message_id": "7", "subject": "Revisión", "body": "Hablamos", "attendees": "ana@ejemplo.eu"})
	if err != nil || r.Target != "calendar" {
		t.Fatal(r, err)
	}
	a := &abridorDoble{}
	if err := api.AbrirURL(Abridor{A: a}, r.Target, r.URL); err != nil {
		t.Fatal(err)
	}
	if len(a.urls) != 1 || !strings.HasPrefix(a.urls[0], "sytueoffice-calendar:/events/new?") ||
		!strings.Contains(a.urls[0], "action=create") || !strings.Contains(a.urls[0], "title=Revisi%C3%B3n") {
		t.Fatalf("abierto: %v", a.urls)
	}
	// Una web sigue yendo al navegador; un esquema raro, a ningún sitio.
	if err := (Abridor{A: a}).OpenURL(&url.URL{Scheme: "file", Path: "/etc/passwd"}); err == nil {
		t.Fatal("file: no se abre")
	}
	if err := AbrirEnCalendar(a, t.TempDir(), "e.ics", []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")); err != nil ||
		!strings.HasPrefix(a.urls[len(a.urls)-1], "sytueoffice-calendar:/abrir?local=file") {
		t.Fatalf("ics: %v %v", err, a.urls)
	}
}
