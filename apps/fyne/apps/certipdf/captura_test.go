// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// TestCapturaDeLaCinta deja la cinta en un PNG con SYTUE_CAPTURA_CINTA: es donde se ve
// el grupo «Trazo» —las seis muestras y el grosor—, que es lo que ningún recuento de
// píxeles dice si está bien repartido.
func TestCapturaDeLaCinta(t *testing.T) {
	ruta := os.Getenv("SYTUE_CAPTURA_CINTA")
	if ruta == "" {
		t.Skip("sin SYTUE_CAPTURA_CINTA: esto es para sacar la lámina, no un guardián")
	}
	b := NuevaBarra(identidad)
	b.Elegir(PestAnotar)
	b.Color = 3
	w := ventanaSinRelleno(b.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(680, AltoBarra()))

	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

// TestCapturaDelPanel deja los tres paneles portados en PNG, uno por panel, en el
// directorio SYTUE_CAPTURA_PANEL.
func TestCapturaDelPanel(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURA_PANEL")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURA_PANEL: esto es para sacar la lámina, no un guardián")
	}
	c := &Contenido{
		Total: 40,
		Indice: []Marcador{
			{Titulo: "1. Objeto del contrato", Pagina: 0},
			{Titulo: "1.1 Alcance de los servicios contratados y exclusiones", Pagina: 1, Nivel: 1},
			{Titulo: "1.2 Plazos", Pagina: 3, Nivel: 1},
			{Titulo: "Anexo sin destino", Pagina: -1},
			{Titulo: "2. Precio", Pagina: 11},
		},
		Adjuntos: []Adjunto{
			{Nombre: "factura-2026-0142.xml", Descripcion: "Factura electrónica", TamanoBytes: 18432, Tipo: "text/xml", Pagina: -1},
		},
		Firmas: []FirmaLeida{
			{Nombre: "Ana Pérez", Motivo: "Conforme", Fecha: "2026-09-10", CubreTodo: true},
			{Nombre: "", Fecha: "2026-09-12", CubreTodo: false},
		},
	}
	p := NuevoPanelLateral(traductorES(t), c)
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(AnchoPanel, 360))
	for nombre, a := range map[string]PanelActivo{"indice": PanelIndice, "adjuntos": PanelAdjuntos, "firmas": PanelFirmas} {
		p.Abrir(a)
		w.Resize(fyne.NewSize(AnchoPanel, 361))
		w.Resize(fyne.NewSize(AnchoPanel, 360))
		f, err := os.Create(dir + "/panel-" + nombre + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
}

// TestCapturaDeLaHoja deja la hoja en un PNG con SYTUE_CAPTURA. Es para MIRARLA: los
// números dicen que hay tinta, no que el dibujo sea el de la web.
//
// Mismo patrón que `iconos/captura_test.go`. Existe porque en esta ola cinco fallos
// graves pasaron todos los tests y solo se vieron abriendo la imagen.
func TestCapturaDeLaHoja(t *testing.T) {
	ruta := os.Getenv("SYTUE_CAPTURA")
	if ruta == "" {
		t.Skip("sin SYTUE_CAPTURA: esto es para sacar la lámina, no un guardián")
	}
	p := a4()
	const zoom = 0.6
	marcas := []Marca{
		{Rect: Rect{60, 640, 300, 700}, Clase: MarcaAnotacion, Titulo: "anotación"},
		{Rect: Rect{60, 500, 260, 540}, Clase: MarcaCensura, Titulo: "censura"},
		{Rect: Rect{60, 380, 340, 420}, Clase: MarcaCampo, Titulo: "campo"},
	}
	h := NuevaHoja(p, zoom, HojaSinDibujar,
		"Esta página usa algo que todavía no sabemos dibujar.", marcas)

	w := ventanaSinRelleno(h)
	defer w.Close()
	w.Resize(MedidaHoja(p, zoom))

	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

// TestCapturaDeLaPantalla deja la pantalla entera en PNG, a lo ancho y a lo estrecho, en
// el directorio SYTUE_CAPTURA_PANTALLA: con el documento de la medida de la web y cada
// página «dibujada» con un rectángulo gris y su número, como el SVG del caso del arnés.
func TestCapturaDeLaPantalla(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURA_PANTALLA")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURA_PANTALLA: esto es para sacar la lámina, no un guardián")
	}
	// SYTUE_CAPTURA_TEMA=oscuro saca las mismas láminas en oscuro, con «-oscuro» detrás.
	sufijo := ""
	if os.Getenv("SYTUE_CAPTURA_TEMA") == "oscuro" {
		tema.Elegir("oscuro")
		defer tema.Elegir("")
		sufijo = "-oscuro"
	}
	for _, c := range []struct {
		nombre string
		tam    fyne.Size
		cajon  bool
		// panel es el que se abre; el documento de «campos» es el del `modo=campos`.
		panel PanelActivo
	}{
		{"ancha", fyne.NewSize(1280, 800), false, PanelPaginas},
		{"estrecha", fyne.NewSize(360, 740), false, PanelPaginas},
		{"cajon", fyne.NewSize(360, 740), true, PanelPaginas},
		{"campos", fyne.NewSize(1280, 800), false, PanelCampos},
		{"comentarios", fyne.NewSize(1280, 800), false, PanelComentarios},
		{"editar", fyne.NewSize(1280, 800), false, PanelEditar},
		{"asistente", fyne.NewSize(1280, 800), false, PanelAsistente},
		{"organizar", fyne.NewSize(1280, 800), false, PanelPaginas},
		{"capas", fyne.NewSize(1280, 800), false, PanelPaginas},
		{"proteger", fyne.NewSize(1280, 800), false, PanelPaginas},
		{"firmar", fyne.NewSize(1280, 800), false, PanelPaginas},
		{"firmar-oficial", fyne.NewSize(1280, 800), false, PanelPaginas},
		{"organizar-360", fyne.NewSize(360, 740), false, PanelPaginas},
		{"capas-360", fyne.NewSize(360, 740), false, PanelPaginas},
		{"proteger-360", fyne.NewSize(360, 740), false, PanelPaginas},
		{"comparar-360", fyne.NewSize(360, 740), false, PanelPaginas},
		{"herramientas-360", fyne.NewSize(360, 740), false, PanelPaginas},
		{"firmar-360", fyne.NewSize(360, 740), false, PanelPaginas},
	} {
		base := strings.TrimSuffix(c.nombre, "-360")
		p := NuevaPantalla(traductorES(t))
		w := ventanaSinRelleno(p.Objeto())
		w.Resize(c.tam)
		doc := documentoDePrueba()
		if c.panel == PanelCampos {
			doc = documentoConCampos()
		}
		p.PonerContenido(doc)
		p.panel.Abrir(c.panel)
		if a := p.panel.asistenteP; a != nil {
			a.respuesta = ia.BloquesDe(respuestaDePrueba)
			a.rehacer()
		}
		if e := p.panel.editarP; e != nil {
			e.cargando, e.analisis, e.textos = false, analisisDePrueba(), map[int]string{1: "Contrato de prestación de servicios", 2: "Madrid, 10 de septiembre de 2026"}
			e.rehacer()
		}
		for n := 1; n <= 3; n++ {
			p.ponerImagen(n, paginaDePrueba(p.escalaDeDibujo()))
		}
		if c.cajon {
			p.PanelAbierto = true
			p.Recomponer()
		}
		w.Resize(fyne.NewSize(c.tam.Width, c.tam.Height+1))
		w.Resize(c.tam)
		if d, ok := map[string]Dialogo{
			"organizar": DialogoOrganizar, "capas": DialogoCapas, "proteger": DialogoProteger,
			"firmar": DialogoFirmar, "firmar-oficial": DialogoFirmar,
			"comparar": DialogoComparar, "herramientas": DialogoHerramientas,
		}[base]; ok {
			p.doc = &Documento{}
			p.abrirDialogo(d)
		}
		if base == "firmar" || base == "firmar-oficial" {
			f := nuevoDialogoFirmar(p.dialogo)
			if c.nombre == "firmar" {
				f.lienzo.trazos = [][][2]float64{{{40, 100}, {80, 40}, {120, 110}, {170, 50}, {230, 90}, {300, 60}}}
			}
			if c.nombre == "firmar-oficial" {
				f.modo, f.sistemaAbierto = "oficial", true
			}
			p.dialogo.pintar = f.vista
			p.dialogo.rehacer()
		}
		f, err := os.Create(dir + "/pantalla-" + c.nombre + sufijo + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		w.Close()
	}
}

// paginaDePrueba es un A4 a esa escala con la banda gris de 60,60 475×40 del caso del
// arnés de la web.
func paginaDePrueba(escala float64) image.Image {
	w, h := int(math.Ceil(595*escala)), int(math.Ceil(842*escala))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.White)
		}
	}
	gris := color.RGBA{221, 221, 221, 255}
	for y := int(60 * escala); y < int(100*escala); y++ {
		for x := int(60 * escala); x < int(535*escala); x++ {
			img.Set(x, y, gris)
		}
	}
	return img
}

// TestCapturaDeLaPortada deja la portada en PNG, a 1280 y a 360, en el directorio
// SYTUE_CAPTURA_PANTALLA (en oscuro con SYTUE_CAPTURA_TEMA=oscuro).
func TestCapturaDeLaPortada(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURA_PANTALLA")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURA_PANTALLA: esto es para sacar la lámina, no un guardián")
	}
	sufijo := ""
	if os.Getenv("SYTUE_CAPTURA_TEMA") == "oscuro" {
		tema.Elegir("oscuro")
		defer tema.Elegir("")
		sufijo = "-oscuro"
	}
	for nombre, tam := range map[string]fyne.Size{"portada": fyne.NewSize(1280, 800), "portada-360": fyne.NewSize(360, 740)} {
		_, w := portadaMontada(t, tam)
		f, err := os.Create(dir + "/pantalla-" + nombre + sufijo + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		w.Close()
	}
}
