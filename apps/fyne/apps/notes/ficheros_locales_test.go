// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// ruido es una imagen que no se comprime: la peor foto para el tope de `data`.
func ruido(w, h int, alfa bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.IntN(256))
		if !alfa && i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	return img
}

func escribir(t *testing.T, ruta string, datos []byte) string {
	t.Helper()
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// TestLaImagenDelEquipoVaDentroDeLaNota (N-06): una imagen pequeña entra tal cual como
// adjunto de una nota de imagen sin `data.url`; una foto grande se reduce a
// LadoImagenNota (JPEG si es opaca, PNG si tiene transparencia) y cabe en `data`; lo que
// no es una imagen no entra.
func TestLaImagenDelEquipoVaDentroDeLaNota(t *testing.T) {
	dir := t.TempDir()
	ahora := time.UnixMilli(1700000000000)

	var b bytes.Buffer
	_ = png.Encode(&b, ruido(40, 20, false))
	pequena := escribir(t, filepath.Join(dir, "plano de la cocina.png"), b.Bytes())
	sinHEIC := &FicherosLocales{}
	in, err := sinHEIC.notaDeImagen(t.Context(), pequena, ahora)
	if err != nil {
		t.Fatal(err)
	}
	if in.Kind != "image" || in.Title != "plano de la cocina" || strings.Contains(string(in.Data), `"url"`) {
		t.Fatalf("la nota de imagen: %+v", in)
	}
	as := adjuntosDe(in.Data)
	if len(as) != 1 || as[0].Name != "plano de la cocina.png" || as[0].Type != "image/png" || as[0].Size != int64(b.Len()) {
		t.Fatalf("el adjunto: %+v", as)
	}
	if got, _ := as[0].contenido(); !bytes.Equal(got, b.Bytes()) {
		t.Fatal("la imagen pequeña no va tal cual")
	}

	b.Reset()
	_ = jpeg.Encode(&b, ruido(3000, 1500, false), &jpeg.Options{Quality: 95})
	if b.Len() <= MaxDatosNota {
		t.Fatalf("la prueba quiere una foto que no quepa tal cual: %d", b.Len())
	}
	foto := escribir(t, filepath.Join(dir, "foto.JPG"), b.Bytes())
	a, err := sinHEIC.adjuntoDeFichero(t.Context(), foto, ahora)
	if err != nil {
		t.Fatal(err)
	}
	cfg, formato, err := image.DecodeConfig(strings.NewReader(mustContenido(t, a)))
	if err != nil || formato != "jpeg" || max(cfg.Width, cfg.Height) > LadoImagenNota || a.Type != "image/jpeg" || a.Name != "foto.jpg" {
		t.Fatalf("la foto reducida: %+v %v %s %s", cfg, err, a.Type, a.Name)
	}
	if data, err := conAdjuntos(nil, []Adjunto{a}); err != nil || len(data) > MaxDatosNota {
		t.Fatalf("no cabe en data: %d %v", len(data), err)
	}

	b.Reset()
	_ = png.Encode(&b, ruido(2600, 300, true))
	con, err := sinHEIC.adjuntoDeFichero(t.Context(), escribir(t, filepath.Join(dir, "logo.png"), b.Bytes()), ahora)
	if err != nil || con.Type != "image/png" || con.Name != "logo.png" {
		t.Fatalf("con transparencia sigue en PNG: %s %s %v", con.Type, con.Name, err)
	}

	if _, err := sinHEIC.notaDeImagen(t.Context(), escribir(t, filepath.Join(dir, "falsa.png"), []byte("no soy un png")), ahora); err == nil {
		t.Error("un fichero que no es una imagen entró")
	}
	if _, err := sinHEIC.notaDeImagen(t.Context(), filepath.Join(dir, "no-existe.png"), ahora); err == nil {
		t.Error("un fichero que no existe entró")
	}
}

func mustContenido(t *testing.T, a Adjunto) string {
	t.Helper()
	b, err := a.contenido()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestLaImagenDentroDeLaNotaSeVeSinRed: la tarjeta pinta la imagen que la nota lleva
// dentro, a lo ancho y con su proporción, sin pedir nada a la red; en la edición
// conectada (sin FicherosLocales), la misma nota sigue como siempre: el marco vacío.
func TestLaImagenDentroDeLaNotaSeVeSinRed(t *testing.T) {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 400, 200)))
	a := nuevoAdjunto("plano.png", "image/png", b.Bytes(), time.Now())
	data, err := conAdjuntos(nil, []Adjunto{a})
	if err != nil {
		t.Fatal(err)
	}
	nota := api.Note{ID: "i1", NotebookID: "c1", Kind: "image", Title: "Plano", Data: data}

	p, falso := pantallaConServicio(t, nil, nota)
	p.FicherosLocales = &FicherosLocales{}
	p.Objeto()
	p.Repintar()
	p.cargarImagenAdjunta(a)
	n, _ := p.nota("i1")
	if got, quiero := altoCuerpo(p.lienzo.tarjeta(n).cuerpo(tema.Claro), 300.7), float32((300.7-2)/2+2); got < quiero-0.1 || got > quiero+0.1 {
		t.Errorf("la imagen de dentro mide %.2f, quiero %.2f", got, quiero)
	}
	if rs := falso.rutas(); len(rs) != 0 {
		t.Errorf("pintar la imagen de dentro pidió %v", rs)
	}

	conectada, _ := pantallaConServicio(t, nil, nota)
	conectada.Objeto()
	conectada.Repintar()
	n, _ = conectada.nota("i1")
	if c, ok := conectada.lienzo.tarjeta(n).cuerpo(tema.Claro).(*fyne.Container); !ok || len(c.Objects) != 0 {
		t.Error("sin FicherosLocales, la nota sin url debería quedarse como siempre")
	}
}

// TestUnEnlaceAUnFicheroVaYVuelve (N-07): la ruta de un fichero del equipo da un
// `file:///…` con lo que haga falta escapado, y el enlace vuelve a la misma ruta; una de
// Windows queda `file:///C:/…` y vuelve sin la barra de delante.
func TestUnEnlaceAUnFicheroVaYVuelve(t *testing.T) {
	for _, ruta := range []string{"/home/ana/Presupuesto 2026 (v2).pdf", "/tmp/a#b?c%d.txt", "/home/ana/ñandú.odt"} {
		e := enlaceAFichero(ruta)
		u, err := url.Parse(e)
		if err != nil || u.Scheme != "file" || !strings.HasPrefix(e, "file:///") || rutaDeEnlace(u) != filepath.FromSlash(ruta) {
			t.Errorf("%q → %q → %q (%v)", ruta, e, rutaDeEnlace(u), err)
		}
	}
	if e := enlaceAFichero(`C:/Users/ana/x.pdf`); e != "file:///C:/Users/ana/x.pdf" {
		t.Errorf("ruta de Windows: %q", e)
	}
	u, _ := url.Parse("file:///C:/Users/ana/x.pdf")
	if r := rutaDeEnlace(u); r != filepath.FromSlash("C:/Users/ana/x.pdf") {
		t.Errorf("de vuelta a Windows: %q", r)
	}
	if p := parrafoEnlace("/home/ana/a<b>.pdf"); p != `<p><a href="file:///home/ana/a%3Cb%3E.pdf">a&lt;b&gt;.pdf</a></p>` {
		t.Errorf("el párrafo: %s", p)
	}
}

// TestSoltarFicherosEnLaNotaOEnElCuaderno (N-06, N-07): un fichero soltado sobre una nota
// de texto le añade su enlace al final, sin tocar lo demás; si es la que se escribe, va
// donde está el cursor; sobre el cuaderno, una imagen es una nota de imagen con la imagen
// dentro y otro fichero una nota con su enlace, donde se soltaron; fuera del cuaderno, o
// sin FicherosLocales, no es de la pantalla.
func TestSoltarFicherosEnLaNotaOEnElCuaderno(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	_ = png.Encode(&b, ruido(30, 30, false))
	imagen := escribir(t, filepath.Join(dir, "foto.png"), b.Bytes())
	pdf := escribir(t, filepath.Join(dir, "acta final.pdf"), []byte("%PDF-1.7"))

	p, falso := pantallaConServicio(t, map[string]string{
		"POST /notebooks/c1/notes": `{"id":"n9","notebook_id":"c1","kind":"text"}`,
		"PUT /notes/t1":            `{"id":"t1","notebook_id":"c1","kind":"text","content":"<p>Hola</p>"}`,
	}, api.Note{ID: "t1", NotebookID: "c1", Kind: "text", Title: "Reunión", Content: "<p>Hola</p>", X: 20, Y: 20})
	p.activo.Vista = Libre
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()

	if p.SoltarFicheros([]string{pdf}, fyne.NewPos(600, 400)) {
		t.Fatal("sin FicherosLocales, soltar un fichero es de la pantalla")
	}
	p.FicherosLocales = &FicherosLocales{}
	p.Repintar()

	tj := p.lienzo.vivas["t1"]
	sobre := fyne.CurrentApp().Driver().AbsolutePositionForObject(tj).AddXY(10, 10)
	if !p.SoltarFicheros([]string{pdf}, sobre) {
		t.Fatal("soltar sobre la nota no la enlaza")
	}
	esperarVeces(t, falso, "PUT /notes/t1", 1)
	put := falso.ultima(t, "PUT /notes/t1")
	if put["content"] != `<p>Hola</p><p><a href="file://`+filepath.ToSlash(dir)+`/acta%20final.pdf">acta final.pdf</a></p>` || put["title"] != "Reunión" {
		t.Errorf("la nota enlazada viaja como %v", put)
	}

	zona := fyne.CurrentApp().Driver().AbsolutePositionForObject(p.desplazador)
	if p.SoltarFicheros([]string{pdf}, zona.SubtractXY(5, 5)) {
		t.Error("fuera del cuaderno, soltar un fichero es de la pantalla")
	}
	if !p.SoltarFicheros([]string{imagen, pdf}, zona.AddXY(500, 300)) {
		t.Fatal("soltar sobre el cuaderno no crea nada")
	}
	esperarVeces(t, falso, "POST /notebooks/c1/notes", 2)
	// Y que la pantalla haya acabado con la segunda (`creando` sigue puesto hasta que se
	// atiende su respuesta): si no, la que llega tarde abre el editor de la nota nueva
	// (vacía en el servicio falso) encima del que se abre abajo. Bajo carga pasaba.
	esperar(t, func() bool {
		creando := true
		fyne.DoAndWait(func() { creando = p.creando })
		return !creando
	})
	fyne.DoAndWait(func() {})
	var img, enlace map[string]any
	falso.mu.Lock()
	for _, v := range falso.vistas {
		if v.metodo+" "+v.ruta != "POST /notebooks/c1/notes" {
			continue
		}
		if v.cuerpo["kind"] == "image" {
			img = v.cuerpo
		} else {
			enlace = v.cuerpo
		}
	}
	falso.mu.Unlock()
	if img == nil || enlace == nil {
		t.Fatalf("las notas soltadas: %v / %v", img, enlace)
	}
	raw, _ := json.Marshal(img["data"])
	if img["title"] != "foto" || !strings.Contains(string(raw), `"dataUrl":"data:image/png;base64,`) || strings.Contains(string(raw), `"url"`) ||
		img["x"] != 500.0 || img["y"] != 300.0 {
		t.Errorf("la nota de imagen soltada: %v %s", img, raw)
	}
	if enlace["kind"] != "text" || enlace["title"] != "acta final" || !strings.Contains(enlace["content"].(string), `/acta%20final.pdf">acta final.pdf</a>`) || enlace["x"] != 528.0 {
		t.Errorf("la nota del fichero soltado: %v", enlace)
	}

	// La nota que se escribe: el enlace va al editor, donde está el cursor, y no al servicio.
	n, _ := p.nota("t1")
	p.abrirEditor(n)
	if p.edicion == nil || p.edicion.nota != "t1" {
		t.Fatal("no se abrió el editor")
	}
	puts := len(falso.rutas())
	tj = p.lienzo.vivas["t1"]
	sobre = fyne.CurrentApp().Driver().AbsolutePositionForObject(tj).AddXY(10, 10)
	if !p.SoltarFicheros([]string{pdf}, sobre) {
		t.Fatal("soltar sobre la nota que se escribe no la enlaza")
	}
	if html := p.edicion.editor.HTML(); !strings.Contains(html, `href="file://`+filepath.ToSlash(dir)+`/acta%20final.pdf"`) || !strings.Contains(html, ">acta final.pdf</a>") {
		t.Errorf("el editor tiene %s", html)
	}
	if len(falso.rutas()) != puts {
		t.Errorf("el enlace en el editor fue directo al servicio: %v", falso.rutas()[puts:])
	}
}
