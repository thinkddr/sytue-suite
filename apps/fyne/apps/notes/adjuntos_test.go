// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestUnAdjuntoViajaComoEnLaWeb: leer un fichero da la URL de datos de `readAsDataURL`
// y los bytes vuelven enteros al descargarlo; uno que no cabe en `data` no se lee; y el
// tamaño se escribe como `formatearTamano`.
func TestUnAdjuntoViajaComoEnLaWeb(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "acta.txt")
	if err := os.WriteFile(ruta, []byte("hola\x00mundo"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := leerAdjunto(ruta, time.UnixMilli(1700000000000))
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "acta.txt" || a.Size != 10 || a.Type != "text/plain" ||
		a.DataURL != "data:text/plain;base64,aG9sYQBtdW5kbw==" || !strings.HasPrefix(a.ID, "1700000000000-") {
		t.Errorf("el adjunto es %+v", a)
	}
	if b, err := a.contenido(); err != nil || string(b) != "hola\x00mundo" {
		t.Errorf("descargado da %q, %v", b, err)
	}
	raro := filepath.Join(dir, "x.sytuezz")
	_ = os.WriteFile(raro, []byte{1}, 0o600)
	if a, _ := leerAdjunto(raro, time.Now()); a.Type != "" || !strings.HasPrefix(a.DataURL, "data:application/octet-stream;base64,") {
		t.Errorf("sin tipo conocido sale %q / %q", a.Type, a.DataURL)
	}
	grande := filepath.Join(dir, "grande.bin")
	if err := os.WriteFile(grande, make([]byte, MaxDatosNota+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := leerAdjunto(grande, time.Now()); err == nil {
		t.Error("un fichero que no cabe en `data` se lee igual")
	}
}

// TestQuitarUnAdjuntoConservaElRestoDeData: el PUT sustituye, así que quitar un adjunto
// manda `data` entero con la lista sin él —y `[]`, no `null`, al quitar el último—, y la
// tarjeta deja de enseñarlo.
//
// VISTO EN ROJO (2026-09-23, en el frontal): armando `data` solo con `attachments`, el
// icono de la nota se perdía al quitar un adjunto.
func TestQuitarUnAdjuntoConservaElRestoDeData(t *testing.T) {
	original := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Data: json.RawMessage(
		`{"icon":"pin","attachments":[{"id":"a1","name":"uno.txt","size":3,"type":"text/plain","dataUrl":"data:text/plain;base64,dW5v"}]}`)}
	p, falso := pantallaConServicio(t, map[string]string{
		"PUT /notes/n1": `{"id":"n1","notebook_id":"c1","kind":"text","data":{"icon":"pin","attachments":[]}}`,
	}, original)
	if n, _ := p.nota("n1"); len(n.Adjuntos) != 1 || n.Adjuntos[0].Name != "uno.txt" {
		t.Fatalf("la nota lee los adjuntos %+v", n.Adjuntos)
	}
	if err := p.cambiarAdjuntos("n1", func([]Adjunto) []Adjunto { return nil }); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(falso.ultima(t, "PUT /notes/n1")["data"])
	if string(got) != `{"attachments":[],"icon":"pin"}` {
		t.Errorf("quitar el adjunto manda data %s", got)
	}
	if n, _ := p.nota("n1"); len(n.Adjuntos) != 0 {
		t.Errorf("la nota sigue con %+v", n.Adjuntos)
	}
}

// TestLaNotaDeImagenSeVe: la imagen de `data.url` se baja una vez y se pinta a lo ancho
// con su proporción (`w-full`, borde incluido), hasta el `max-h-64`; una que no llega
// deja su `alt`, el título.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el tope de `AltoMaxImagen`, la imagen
// alta ocupa 1 195,4 donde la web se queda en 224.
func TestLaNotaDeImagenSeVe(t *testing.T) {
	servida := func(w, h int) []byte {
		var b bytes.Buffer
		_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h)))
		return b.Bytes()
	}
	ancha, alta := servida(400, 200), servida(200, 800)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ancha.png":
			_, _ = w.Write(ancha)
		case "/alta.png":
			_, _ = w.Write(alta)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	nota := func(id, ruta string) api.Note {
		return api.Note{
			ID: id, NotebookID: "c1", Kind: "image", Title: "Plano " + id,
			Data: json.RawMessage(`{"url":"` + srv.URL + ruta + `"}`),
		}
	}
	p, _ := pantallaConServicio(t, nil, nota("a", "/ancha.png"), nota("b", "/alta.png"), nota("c", "/no.png"))
	// `cargarImagen` a mano y no `imagenDe`, que la lanza en su rutina y se cruzaría con
	// el test.
	for _, r := range []string{"/ancha.png", "/alta.png", "/no.png"} {
		p.cargarImagen(srv.URL + r)
	}
	cuerpo := func(id string) float32 {
		n, _ := p.nota(id)
		tj := p.lienzo.tarjeta(n)
		return altoCuerpo(tj.cuerpo(tema.Claro), 300.7)
	}
	casi := func(que string, got, quiero float32) {
		t.Helper()
		if got < quiero-0.1 || got > quiero+0.1 {
			t.Errorf("%s: %.2f, quiero %.2f", que, got, quiero)
		}
	}
	casi("la ancha", cuerpo("a"), (300.7-2)/2+2)
	casi("la alta", cuerpo("b"), AltoMaxImagen)
	n, _ := p.nota("c")
	if c, ok := p.lienzo.tarjeta(n).cuerpo(tema.Claro).(*fyne.Container); !ok || len(c.Objects) != 2 {
		t.Error("la que no llega no deja su icono y su título")
	} else if alt, ok := c.Objects[1].(*ui.Parrafo); !ok || alt.Texto != "Plano c" {
		t.Error("la que no llega no deja su título")
	}

	// En su columna, la tarjeta recién hecha —sin ancho todavía— se coloca ya con el
	// alto de su imagen a ese ancho, y no con el de su mínimo a ciegas.
	//
	// VISTO EN ROJO (2026-09-23, en el frontal): con `ui.Columna`, la tarjeta medía 55,5
	// y la imagen se salía por debajo hasta el fotograma siguiente.
	a, _ := p.nota("a")
	tj := p.lienzo.tarjeta(a)
	columnaTarjetas{hueco: HuecoLista}.Layout([]fyne.CanvasObject{tj}, fyne.NewSize(323.7, 800))
	casi("la tarjeta en su columna", tj.Size().Height, MedidaTarjetaNota(tj, 323.7).Height)
	if tj.Size().Height < 150 {
		t.Errorf("la tarjeta de imagen mide %.2f en su columna", tj.Size().Height)
	}
}

// TestLaNotaDocEnseñaElDocumento: la vista previa de una nota `doc` pide a Docs el HTML
// del documento enlazado y lo pinta con el maquetador de Docs, dentro de la caja de alto
// fijo del iframe (`h-64`), encima de «Abrir documento».
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin la lectura en `vistaDoc`, la caja sale
// vacía aunque el documento haya llegado.
func TestLaNotaDocEnseñaElDocumento(t *testing.T) {
	docs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/documents/d1/state" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"state":{"lamport":3,"registers":{"doc:html":{"value":"<h1>Orden del día</h1><p>Hola</p>","lamport":3}}}}`))
	}))
	defer docs.Close()
	p, _ := pantallaConServicio(t, nil, api.Note{
		ID: "a", NotebookID: "c1", Kind: "doc",
		Links: []api.NoteLink{{TargetKind: "doc", TargetID: "d1", Label: "Acta", URL: "https://docs.sytue.eu/d/d1"}},
	})
	p.Docs = api.NewDocs(docs.URL, nil, nil)
	p.cargarDoc("d1") // a mano: `docDe` lo lanza en su rutina y se cruzaría con el test
	n, _ := p.nota("a")
	tj := p.lienzo.tarjeta(n)
	w := test.NewWindow(tj)
	defer w.Close()
	w.Resize(fyne.NewSize(323.7, 400))
	var textos []string
	cajas := 0
	recorrerInterno(tj, func(o fyne.CanvasObject) {
		if x, ok := o.(*canvas.Text); ok {
			textos = append(textos, x.Text)
		}
		if c, ok := o.(*fyne.Container); ok {
			if _, es := c.Layout.(*vistaDocLayout); es {
				cajas++
				if c.Size().Height != AltoVistaDoc {
					t.Errorf("la caja del documento mide %.2f y el iframe %.2f", c.Size().Height, AltoVistaDoc)
				}
			}
		}
	})
	todo := strings.Join(textos, "|")
	if cajas != 1 || !strings.Contains(todo, "Orden") || !strings.Contains(todo, p.tr.T("docs.openViewer", nil)) {
		t.Errorf("la nota doc enseña %d cajas y %q", cajas, todo)
	}
	// Medido en la web con la nota como TARJETA: a 323,7 de ancho, 340 (el iframe de 256 y
	// «Abrir documento» con su `gap-1.5`). En lista la nota es una FILA: en vez del `p-3`
	// con borde lleva `py-1.5` sin él, así que mide 340 − 2·(12+1) + 2·6 = 326.
	if h := MedidaTarjetaNota(tj, 323.7).Height; h < 325.5 || h > 326.5 {
		t.Errorf("la fila de la nota doc mide %.2f y en la web 326", h)
	}
}
