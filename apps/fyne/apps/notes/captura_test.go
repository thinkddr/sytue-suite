// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestPegarUnTextoEsLaNotaDeLaWeb: los casos de captura.test.ts. Una URL sola es un
// enlace con el host de título; con algo más, o con otro esquema, es texto, y el texto se
// escapa y se parte en párrafos.
func TestPegarUnTextoEsLaNotaDeLaWeb(t *testing.T) {
	casos := []struct {
		texto, tipo, titulo, contenido, data string
	}{
		{"  https://Example.com:8080/a?b=1 ", "link", "example.com", "", `{"url":"https://Example.com:8080/a?b=1"}`},
		{"http://x.eu", "link", "x.eu", "", `{"url":"http://x.eu"}`},
		{"https://a.eu y más", "text", "", "<p>https://a.eu y más</p>", ""},
		{"javascript:alert(1)", "text", "", "<p>javascript:alert(1)</p>", ""},
		{"ftp://x.eu", "text", "", "<p>ftp://x.eu</p>", ""},
		{"hola\r\n\r\n\r\nmundo <b>\nx", "text", "", "<p>hola</p><p>mundo &lt;b&gt;<br>x</p>", ""},
	}
	for _, c := range casos {
		in, ok := interpretarCaptura(c.texto)
		if !ok || in.Kind != c.tipo || in.Title != c.titulo || in.Content != c.contenido || string(in.Data) != c.data {
			t.Errorf("%q da %+v (%s)", c.texto, in, in.Data)
		}
	}
	if _, ok := interpretarCaptura(" \n\t "); ok {
		t.Error("pegar nada crea una nota")
	}
}

// TestPegarSobreElCuadernoCreaLaNota: Cmd+V con nada enfocado crea la nota; con el filtro
// enfocado, pega ahí y no crea nada. (Se prueba `captura`: el lienzo de pruebas no dispara
// los atajos de `AddShortcut`, y `pegar` solo la lanza.)
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin mirar `Focused()`, pegar en el filtro
// creaba además una nota.
func TestPegarSobreElCuadernoCreaLaNota(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"POST /notebooks/c1/notes": `{"id":"n9","notebook_id":"c1","kind":"link"}`,
	}, api.Note{ID: "a", NotebookID: "c1"})
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()
	pp := test.NewClipboard()
	pp.SetContent("https://sytue.eu/precios")
	pegado := &fyne.ShortcutPaste{Clipboard: pp}

	// Un cuaderno en lista es una página: su filtro está tras la lupa, que lo abre y le
	// da el foco.
	p.barra.AlAbrirFiltro()
	if w.Canvas().Focused() == nil {
		t.Fatal("el filtro de la página no toma el foco al abrir la lupa")
	}
	if in, ok := p.captura(pegado); ok {
		t.Fatalf("pegar en el filtro crearía una nota: %+v", in)
	}
	w.Canvas().Unfocus()
	in, ok := p.captura(pegado)
	if !ok {
		t.Fatal("pegar sobre el cuaderno no crea nada")
	}
	if err := p.crear(in); err != nil {
		t.Fatal(err)
	}
	c := falso.ultima(t, "POST /notebooks/c1/notes")
	if c["kind"] != "link" || c["title"] != "sytue.eu" || p.lienzo.Seleccionada != "n9" {
		t.Errorf("la nota pegada viaja como %v y queda elegida %q", c, p.lienzo.Seleccionada)
	}
}

// TestSoltarSobreElCuadernoCreaLaNota: el `drop` de NoteCanvas.tsx. Un enlace soltado en
// el lienzo libre es nota de enlace con el host de título y nace DONDE se soltó; la lista
// de URI manda sobre el texto; una lista sin enlace web queda como texto; y soltar fuera
// del cuaderno no crea nada.
func TestSoltarSobreElCuadernoCreaLaNota(t *testing.T) {
	casos := []struct {
		texto           string
		urls            []string
		tipo, contenido string
	}{
		{"Precios de Sytue", []string{"https://sytue.eu/precios"}, "link", ""},
		{"", []string{"mailto:ada@sytue.eu"}, "text", "<p>mailto:ada@sytue.eu</p>"},
		{"Hola\n\ncuaderno", nil, "text", "<p>Hola</p><p>cuaderno</p>"},
	}
	for _, c := range casos {
		in, ok := interpretarSoltado(c.texto, c.urls)
		if !ok || in.Kind != c.tipo || in.Content != c.contenido {
			t.Errorf("%q %q da %+v", c.texto, c.urls, in)
		}
	}

	p, falso := pantallaConServicio(t, map[string]string{
		"POST /notebooks/c1/notes": `{"id":"n9","notebook_id":"c1","kind":"link"}`,
	}, api.Note{ID: "a", NotebookID: "c1"})
	p.activo.Vista = Libre
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()
	zona := fyne.CurrentApp().Driver().AbsolutePositionForObject(p.desplazador)

	if in, ok := p.soltado("", []string{"https://sytue.eu/precios"}, zona.SubtractXY(10, 10)); ok {
		t.Fatalf("soltar fuera del cuaderno crearía una nota: %+v", in)
	}
	in, ok := p.soltado("Precios", []string{"https://sytue.eu/precios"}, zona.AddXY(100, 60))
	if !ok {
		t.Fatal("soltar un enlace sobre el cuaderno no crea nada")
	}
	if err := p.crear(in); err != nil {
		t.Fatal(err)
	}
	c := falso.ultima(t, "POST /notebooks/c1/notes")
	if c["kind"] != "link" || c["title"] != "sytue.eu" || c["x"] != 100.0 || c["y"] != 60.0 {
		t.Errorf("la nota soltada viaja como %v", c)
	}
}
