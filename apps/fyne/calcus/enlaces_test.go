// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"errors"
	"net/url"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestSoloSeAbrenLosEsquemasPermitidos: http, https y mailto con su máquina o su
// destinatario; nada de `javascript:`, `data:` ni `file:`, ni textos que solo lo parecen.
func TestSoloSeAbrenLosEsquemasPermitidos(t *testing.T) {
	buenos := [][2]string{
		{"https://sytue.eu/precios", "https://sytue.eu/precios"},
		{"  HTTP://ejemplo.org ", "http://ejemplo.org"},
		{"mailto:ana@ejemplo.org", "mailto:ana@ejemplo.org"},
		{"https://ejemplo.org/a?x=1", "https://ejemplo.org/a?x=1"},
	}
	for _, b := range buenos {
		if got := hrefSeguro(b[0]); got != b[1] {
			t.Errorf("hrefSeguro(%q) = %q, quería %q", b[0], got, b[1])
		}
	}
	for _, malo := range []string{
		"javascript:alert(1)", "JavaScript:alert(1)", "data:text/html,x", "file:///etc/passwd",
		"ftp://ejemplo.org", "www.ejemplo.org", "https://ejemplo.org y más", "http://", "mailto:",
		"Total", "",
	} {
		if got := hrefSeguro(malo); got != "" {
			t.Errorf("hrefSeguro(%q) = %q y no es un enlace que se abra", malo, got)
		}
	}
}

// libroConEnlaces: A1 HYPERLINK literal, A2 una URL escrita, A3 un HYPERLINK con
// `javascript:`, A4 texto, A5 HYPERLINK calculado desde B1.
func libroConEnlaces(l *Libro) {
	l.Escribir(0, 0, `=HYPERLINK("https://ejemplo.org/precios";"Precios")`)
	l.Escribir(0, 1, "mailto:ana@ejemplo.org")
	l.Escribir(0, 2, `=HYPERLINK("javascript:alert(1)";"Trampa")`)
	l.Escribir(0, 3, "Texto normal")
	l.Escribir(1, 0, "https://ejemplo.org")
	l.Escribir(0, 4, `=HYPERLINK(B1&"/ficha";"Ficha")`)
}

// TestElEnlaceDeUnaCelda: el destino de HYPERLINK (literal o calculado) aunque la celda
// enseñe el nombre, la URL escrita tal cual, y nada para el esquema vetado o el texto.
func TestElEnlaceDeUnaCelda(t *testing.T) {
	l := NuevoLibro()
	libroConEnlaces(l)
	casos := []struct {
		f    int
		href string
	}{
		{0, "https://ejemplo.org/precios"},
		{1, "mailto:ana@ejemplo.org"},
		{2, ""},
		{3, ""},
		{4, "https://ejemplo.org/ficha"},
	}
	for _, c := range casos {
		if got := l.Enlace(0, c.f); got != c.href {
			t.Errorf("A%d: enlace %q, quería %q", c.f+1, got, c.href)
		}
	}
	if l.Texto(0, 0) != "Precios" {
		t.Errorf("A1 enseña %q y tiene que seguir enseñando el nombre", l.Texto(0, 0))
	}
}

// posCelda es el centro de una celda en la rejilla sin desplazar.
func posCelda(c, f int) fyne.Position {
	return fyne.NewPos(AnchoCabecera+float32(c)*AnchoCelda+10, AltoCelda+float32(f)*AltoCelda+10)
}

func pulsarCelda(r *Rejilla, c, f int, mod fyne.KeyModifier) {
	p := posCelda(c, f)
	r.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: p}, Modifier: mod})
	r.Tapped(&fyne.PointEvent{Position: p})
}

// TestCtrlClicYAltIntroAbrenElEnlace: el clic solo selecciona; Ctrl+clic (⌘ en el Mac) y
// Alt+Intro abren el destino; el esquema vetado y el texto no abren nada.
func TestCtrlClicYAltIntroAbrenElEnlace(t *testing.T) {
	c := lienzoDePrueba()
	libroConEnlaces(c.Libro)
	r := c.Rejilla
	var abiertos []string
	r.AlAbrirEnlace = func(href string) { abiertos = append(abiertos, href) }

	pulsarCelda(r, 0, 1, 0)
	if len(abiertos) != 0 || r.Seleccion() != (Celda{C: 0, F: 1}) {
		t.Fatalf("el clic solo ha abierto %v o no ha seleccionado A2 (%v)", abiertos, r.Seleccion())
	}
	pulsarCelda(r, 0, 0, fyne.KeyModifierControl)
	if r.Seleccion() != (Celda{}) {
		t.Errorf("Ctrl+clic no ha dejado A1 seleccionada: %v", r.Seleccion())
	}
	pulsarCelda(r, 0, 1, fyne.KeyModifierSuper)
	pulsarCelda(r, 0, 2, fyne.KeyModifierControl) // javascript: no se abre nunca
	pulsarCelda(r, 0, 3, fyne.KeyModifierControl)
	// Ctrl+Mayús+clic extiende la selección, como en la web: no abre el enlace.
	pulsarCelda(r, 0, 0, fyne.KeyModifierControl|fyne.KeyModifierShift)
	r.Seleccionar(0, 4, false)
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierAlt})
	quiero := []string{"https://ejemplo.org/precios", "mailto:ana@ejemplo.org", "https://ejemplo.org/ficha"}
	if !slices.Equal(abiertos, quiero) {
		t.Errorf("se han abierto %v y tocaba %v", abiertos, quiero)
	}
	if r.editando {
		t.Error("Alt+Intro ha abierto la celda para escribir en vez de abrir su enlace")
	}
}

// TestElChipDelEnlace: la celda activa con enlace enseña el chip con la máquina (o el
// correo) y pulsarlo abre el destino; sin enlace o escribiendo en la celda, no hay chip.
func TestElChipDelEnlace(t *testing.T) {
	c := lienzoDePrueba()
	libroConEnlaces(c.Libro)
	r := c.Rejilla
	var abiertos []string
	r.AlAbrirEnlace = func(href string) { abiertos = append(abiertos, href) }

	chip := c.chipEnlace(r.vista())
	if chip == nil || chip.Texto != "ejemplo.org" {
		t.Fatalf("A1 con enlace enseña el chip %v", chip)
	}
	if chip.Rotulo == "" || chip.Position().Y < AltoCelda*2 {
		t.Errorf("el chip no tiene nombre accesible (%q) o no va debajo de A1 (%v)", chip.Rotulo, chip.Position())
	}
	if !slices.Contains(r.Flotantes(r.vista()), fyne.CanvasObject(chip)) {
		t.Error("el chip no está entre los flotantes de la rejilla: no seguiría al desplazarse")
	}
	chip.AlPulsar()
	if !slices.Equal(abiertos, []string{"https://ejemplo.org/precios"}) {
		t.Errorf("pulsar el chip abre %v", abiertos)
	}
	r.Seleccionar(0, 1, false)
	if chip := c.chipEnlace(r.vista()); chip == nil || chip.Texto != "ana@ejemplo.org" {
		t.Errorf("el chip de un correo enseña %v", chip)
	}
	for _, f := range []int{2, 3} {
		r.Seleccionar(0, f, false)
		if c.chipEnlace(r.vista()) != nil {
			t.Errorf("A%d no tiene un enlace que abrir y enseña el chip", f+1)
		}
	}
	r.Seleccionar(0, 0, false)
	r.Editar("")
	if c.chipEnlace(r.vista()) != nil {
		t.Error("escribiendo en la celda el chip sigue encima")
	}
}

// TestElEnlaceSePintaSubrayadoYAzul: la celda con enlace va subrayada y con el azul de la
// selección; si el usuario le puso tinta, manda la suya; la del esquema vetado, nada.
func TestElEnlaceSePintaSubrayadoYAzul(t *testing.T) {
	r := rejillaDePrueba(500, 200)
	libroConEnlaces(r.L)
	r.L.CambiarEstilo(conTinta("#DC2626"), 0, 1, 0, 1) // A2 en rojo
	r.Seleccionar(3, 6, false)                         // lejos: el ancla no tapa nada
	pinta(t, 500, 200, r)
	azul := tema.Color("primary-500", tema.Claro)
	if got, sub := estiloTextoCelda(t, r, 0, 0); got != azul || !sub {
		t.Errorf("A1 pinta %v subrayado=%v y un enlace va en %v subrayado", got, sub, azul)
	}
	if got, sub := estiloTextoCelda(t, r, 0, 1); got == azul || !sub {
		t.Errorf("A2 con tinta propia pinta %v subrayado=%v: la tinta es la suya y va subrayado", got, sub)
	}
	if _, sub := estiloTextoCelda(t, r, 0, 2); sub {
		t.Error("el HYPERLINK a javascript: se pinta como un enlace")
	}
	if _, sub := estiloTextoCelda(t, r, 0, 3); sub {
		t.Error("el texto normal se pinta subrayado")
	}
}

// estiloTextoCelda es la tinta y el subrayado del texto pintado de una celda visible.
func estiloTextoCelda(t *testing.T, r *Rejilla, c, f int) (any, bool) {
	t.Helper()
	x, y := AnchoCabecera+float32(c)*AnchoCelda-r.desX, AltoCelda+float32(f)*AltoCelda-r.desY
	for _, cp := range r.r.celdas {
		if cp.texto.Visible() && cp.texto.Text != "" && cp.fondo.Position() == fyne.NewPos(x, y) {
			return cp.texto.Color, cp.texto.TextStyle.Underline
		}
	}
	t.Fatalf("no se ha pintado texto en %d,%d", c, f)
	return nil, false
}

type abridorDePrueba struct{ abiertas []string }

func (a *abridorDePrueba) OpenURL(u *url.URL) error {
	a.abiertas = append(a.abiertas, u.String())
	return nil
}

// TestAbrirEnlaceVaPorLasPuertasDeLaSuite: la web va a `api.AbrirFuera`; un `mailto:` se
// recompone con destinatario, asunto y cuerpo (sin el `cc` que traiga), y lo vetado no
// llega nunca al sistema.
func TestAbrirEnlaceVaPorLasPuertasDeLaSuite(t *testing.T) {
	a := &abridorDePrueba{}
	if err := abrirEnlace(a, "https://ejemplo.org/a"); err != nil {
		t.Fatal(err)
	}
	if err := abrirEnlace(a, "mailto:ana@ejemplo.org?subject=Hola%20Ana&cc=otro@x.org"); err != nil {
		t.Fatal(err)
	}
	if err := abrirEnlace(a, "javascript:alert(1)"); !errors.Is(err, api.ErrNoSeAbre) {
		t.Errorf("javascript: devuelve %v", err)
	}
	quiero := []string{"https://ejemplo.org/a", "mailto:ana@ejemplo.org?subject=Hola%20Ana"}
	if !slices.Equal(a.abiertas, quiero) {
		t.Errorf("al sistema han llegado %v y tocaba %v", a.abiertas, quiero)
	}
}
