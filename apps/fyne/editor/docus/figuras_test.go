// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image"
	"image/png"
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// capturaFolio pinta el folio y devuelve la imagen y cómo pasar de px del contenido del
// folio (desde la esquina de dentro de los márgenes) a los de la imagen. Con la variable de entorno puesta, deja el PNG.
func capturaFolio(t *testing.T, f *Folio, variable string) (image.Image, func(x, y float32) (int, int)) {
	t.Helper()
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewStack(f))
	c.Resize(fyne.NewSize(900, 1250))
	img := c.Capture()
	if ruta := os.Getenv(variable); ruta != "" {
		fh, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer fh.Close()
		_ = png.Encode(fh, img)
	}
	ox, oy := f.pag.origen()
	esc := float32(img.Bounds().Dx()) / 900
	return img, func(x, y float32) (int, int) {
		return int((ox + x) * esc), int((oy + y) * esc)
	}
}

// TestLaFormaSeInsertaCambiaYSePinta: el panel la inserta en el cursor con el párrafo de
// escape, queda activa y un cambio la rehornea en su sitio, sin otra figura; pintada, el
// centro de la elipse es su relleno. Se vio en ROJO sin la rama de Figura en pintarCaja: el
// centro salía blanco.
func TestLaFormaSeInsertaCambiaYSePinta(t *testing.T) {
	f := folioCon(t, `<p>Hola</p>`)
	f.anc, f.cur = punto{0, 4}, punto{0, 4}
	fo := rico.SanearForma("ellipse", "", "", "", "", "")
	f.Mandar(cmdFigura, fo.HTML("Forma elipse"))
	if il := f.figuraActiva("shape"); il == nil || f.figuraActiva("chart") != nil {
		t.Fatalf("tras insertarla, la forma es la activa y no hay gráfico: %s", f.Serializar())
	}
	fo.Relleno, fo.Texto = "#2563EB", "Hola"
	f.Mandar(cmdFigura, fo.HTML("Forma elipse"))
	h := f.Serializar()
	if strings.Count(h, "<figure") != 1 || !strings.Contains(h, `data-fill="#2563EB"`) || !strings.HasSuffix(h, `</figure><p><br></p>`) {
		t.Fatalf("el cambio no la rehornea en su sitio: %s", h)
	}
	img, en := capturaFolio(t, f, "SYTUE_CAPTURA_FORMA")
	var fig *caja
	for i := range f.cajas {
		if f.cajas[i].b.Tipo == rico.Figura {
			fig = &f.cajas[i]
		}
	}
	if fig == nil || fig.alto != rico.AltoForma {
		t.Fatalf("la caja de la figura: %+v", fig)
	}
	// El centro lleva el texto: se mira a un cuarto del ancho, dentro de la elipse.
	x, y := en(fig.izq+(fig.ancho-rico.AnchoForma)/2+rico.AnchoForma/4, fig.y+rico.AltoForma/2)
	if r, g, b, _ := img.At(x, y).RGBA(); r>>8 > 0x40 || g>>8 < 0x50 || g>>8 > 0x70 || b>>8 < 0xd0 {
		t.Errorf("dentro de la elipse, en (%d, %d), hay %v y es el azul del relleno (caja %+v)", x, y, img.At(x, y), *fig)
	}
}

// TestElGraficoSeInsertaYSePinta: el de arranque, con sus barras azules y los rótulos
// del eje y de las categorías como texto encima.
func TestElGraficoSeInsertaYSePinta(t *testing.T) {
	f := folioCon(t, `<p>Hola</p>`)
	f.anc, f.cur = punto{0, 4}, punto{0, 4}
	f.Mandar(cmdFigura, rico.Grafico{Tipo: "bar", Rejilla: rico.RejillaInicial()}.HTML())
	if f.figuraActiva("chart") == nil {
		t.Fatalf("el gráfico no queda activo: %s", f.Serializar())
	}
	var textos []string
	for _, o := range f.pintarContenido(nil, fyne.Position{}, 0, 0, 1) {
		if tx, ok := o.(*canvas.Text); ok {
			textos = append(textos, tx.Text)
		}
	}
	for _, quiero := range []string{"15", "Ene", "Abr"} {
		if !strings.Contains(strings.Join(textos, "|"), quiero) {
			t.Errorf("falta el rótulo %q: %v", quiero, textos)
		}
	}
	img, en := capturaFolio(t, f, "SYTUE_CAPTURA_GRAFICO")
	var c *caja
	for i := range f.cajas {
		if f.cajas[i].b.Tipo == rico.Figura {
			c = &f.cajas[i]
		}
	}
	// La segunda barra (Feb, 15) llega arriba del todo: su centro, a media altura del
	// área de trazado, es azul. 480 de ancho: la figura entra entera en el folio.
	w := float32(rico.AnchoGrafico)
	hueco := (w - 44 - 10) / 4
	x, y := en(c.izq+(c.ancho-w)/2+44+hueco*1.5, c.y+12+(300-12-26)/2)
	if r, g, b, _ := img.At(x, y).RGBA(); r>>8 > 0x40 || b>>8 < 0xd0 || g>>8 > 0x70 {
		t.Errorf("en la barra de febrero hay %v y es azul", img.At(x, y))
	}
}

// TestElSmartArtSeInsertaYSePinta: el proceso de arranque, con el texto de sus tres
// nodos encima del SVG. Se vio en ROJO sin la rama del diagrama en pintarFigura: no salía
// ningún texto.
func TestElSmartArtSeInsertaYSePinta(t *testing.T) {
	f := folioCon(t, `<p>Hola</p>`)
	f.anc, f.cur = punto{0, 4}, punto{0, 4}
	sa := rico.SmartArt{Disposicion: "process", Elementos: rico.ElementosIniciales("process")}
	f.Mandar(cmdFigura, sa.HTML("Diagrama proceso", "Vacío"))
	if f.figuraActiva("smartart") == nil || f.figuraActiva("shape") != nil {
		t.Fatalf("el diagrama no queda activo: %s", f.Serializar())
	}
	sa.Elementos[0].Texto = "Arranque"
	f.Mandar(cmdFigura, sa.HTML("Diagrama proceso", "Vacío"))
	if h := f.Serializar(); strings.Count(h, "<figure") != 1 || !strings.Contains(h, "Arranque") {
		t.Fatalf("el cambio no lo rehornea en su sitio: %s", h)
	}
	var textos []string
	for _, o := range f.pintarContenido(nil, fyne.Position{}, 0, 0, 1) {
		if tx, ok := o.(*canvas.Text); ok {
			textos = append(textos, tx.Text)
		}
	}
	for _, quiero := range []string{"Arranque", "Desarrollo", "Resultado"} {
		if !slices.Contains(textos, quiero) {
			t.Errorf("falta el nodo %q: %v", quiero, textos)
		}
	}
	for _, c := range f.cajas {
		if c.b.Tipo == rico.Figura && c.alto != 170*c.ancho/rico.AnchoSmart {
			t.Errorf("el proceso mide %v de alto en %v de ancho y es 170/640 del ancho", c.alto, c.ancho)
		}
	}
	_, _ = capturaFolio(t, f, "SYTUE_CAPTURA_SMART")
}

// TestElBloqueReutilizableSeInsertaEnElCursor: el HTML del bloque entra donde está el
// cursor con el párrafo de escape detrás (`insertHTML` de BloquesGroup.tsx), y deshacer lo
// quita entero.
func TestElBloqueReutilizableSeInsertaEnElCursor(t *testing.T) {
	f := folioCon(t, `<p>Hola</p>`)
	f.anc, f.cur = punto{0, 4}, punto{0, 4}
	f.Mandar(cmdBloque, `<h2>Condiciones</h2><p>Pago a treinta días.</p>`)
	h := f.Serializar()
	if !strings.Contains(h, `<h2>Condiciones</h2><p>Pago a treinta días.</p>`) || !strings.HasPrefix(h, `<p>Hola</p>`) {
		t.Fatalf("el bloque no entró tras el párrafo del cursor: %s", h)
	}
	f.Mandar(editor.Deshacer, "")
	if got := f.Serializar(); got != `<p>Hola</p>` {
		t.Errorf("deshacer no quita el bloque: %s", got)
	}
}
