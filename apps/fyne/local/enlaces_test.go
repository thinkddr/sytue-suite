// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los argumentos que se atienden: un fichero, el esquema propio y los de más de la app
// (`mailto:` siempre). Una dirección web o un esquema cualquiera, no.
func TestUriDeArgumento(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "carta.eml")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	extra := map[string]bool{"mailto": true, "webcal": true}
	for arg, quiero := range map[string]bool{
		f:                                     true,
		"file:///tmp/x.docx":                  true,
		"sytueoffice-mail:/redactar?para=a@b": true,
		"mailto:ana@ejemplo.eu?subject=hola":  true,
		"MAILTO:ana@ejemplo.eu":               true,
		"webcal://ejemplo.eu/cal.ics":         true,
		"https://ejemplo.eu":                  false,
		"javascript:alert(1)":                 false,
		filepath.Join(dir, "no-existe.txt"):   false,
		dir:                                   false,
	} {
		if _, ok := uriDeArgumento(arg, extra); ok != quiero {
			t.Errorf("%q: %v; quiero %v", arg, ok, quiero)
		}
	}
	if _, ok := uriDeArgumento("webcal://x", nil); ok {
		t.Error("sin registrarlo, webcal no se atiende")
	}
}

// Cada enlace va a su gancho: `?local=` a AlAbrirFichero, `?clase&id` a
// AlAbrirElemento y el resto (también `mailto:`) a AlAbrirEnlace, que recibe lo que llegó
// antes de registrarse. Las direcciones web no llegan a ninguno.
func TestRecibirRepartePorGancho(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	l := &App{A: a}
	parse := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	l.recibir(parse("mailto:ana@ejemplo.eu"))
	l.recibir(parse("https://ejemplo.eu/"))
	var enlaces []string
	l.AlAbrirEnlace(func(u *url.URL) { enlaces = append(enlaces, u.String()) })
	// Hasta que la app arranca (los argumentos llegan antes que la ventana), espera.
	if len(enlaces) != 0 {
		t.Fatalf("entregado antes de arrancar: %q", enlaces)
	}
	l.alArrancar()
	l.recibir(parse("sytueoffice-calendar:/events/new?action=create&title=Hola"))
	var elementos [][2]string
	l.AlAbrirElemento(func(c, id string) { elementos = append(elementos, [2]string{c, id}) })
	l.recibir(parse("sytueoffice-notes:/abrir?clase=nota&id=n1"))
	abiertos := make(chan string, 1)
	l.AlAbrirFichero(func(_ context.Context, ap api.Apertura) error { abiertos <- ap.Local; return nil }, nil)
	l.recibir(parse("sytueoffice-docs:/abrir?local=file%3A%2F%2F%2Ftmp%2Fa.docx"))
	if got := <-abiertos; got != "file:///tmp/a.docx" {
		t.Errorf("fichero: %q", got)
	}
	if len(enlaces) != 2 || enlaces[0] != "mailto:ana@ejemplo.eu" || enlaces[1] != "sytueoffice-calendar:/events/new?action=create&title=Hola" {
		t.Errorf("enlaces: %q", enlaces)
	}
	if len(elementos) != 1 || elementos[0] != [2]string{"nota", "n1"} {
		t.Errorf("elementos: %q", elementos)
	}
}

// El enlace de cuentas va y vuelve, y lo que no es una acción conocida no se atiende.
func TestEnlaceCuentas(t *testing.T) {
	u, err := url.Parse(EnlaceCuentas("mail", AccionQuitar, "c 1"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "sytueoffice-mail" {
		t.Fatalf("esquema %q", u.Scheme)
	}
	if a, id, ok := LeerEnlaceCuentas(u); !ok || a != AccionQuitar || id != "c 1" {
		t.Fatalf("%q %q %v", a, id, ok)
	}
	for _, malo := range []string{
		"sytueoffice-mail:/cuentas?accion=borrar-todo", "sytueoffice-mail:/cuentas?accion=quitar",
		"sytueoffice-mail:/redactar?accion=anadir", "https://ejemplo.eu/cuentas?accion=anadir",
	} {
		v, _ := url.Parse(malo)
		if _, _, ok := LeerEnlaceCuentas(v); ok {
			t.Errorf("%s se ha dado por bueno", malo)
		}
	}
	v, _ := url.Parse(EnlaceCuentas("rooms", AccionAnadir, ""))
	if a, _, ok := LeerEnlaceCuentas(v); !ok || a != AccionAnadir {
		t.Fatalf("añadir: %q %v", a, ok)
	}
	if got := NombreFichero("a/b:c", "x", ".md", map[string]int{"a_b_c.md": 1}); got != "a_b_c (2).md" {
		t.Errorf("NombreFichero: %q", got)
	}
}
