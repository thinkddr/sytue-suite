// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema/voz"
)

// TestLeerEnVozAltaUsaLaVozDelSistema: el texto de la página que se mira, en orden de
// lectura, va a la voz del sistema en el idioma de la interfaz; el botón queda pulsado;
// pulsarlo otra vez calla; y sin voz en el sistema se DICE en vez de callarse.
func TestLeerEnVozAltaUsaLaVozDelSistema(t *testing.T) {
	var leido, idioma string
	callada := false
	antes := hablar
	defer func() { hablar = antes }()
	hablar = func(texto, id string, _ func()) (func(), error) {
		leido, idioma = texto, id
		return func() { callada = true }, nil
	}

	p, _, listo, cerrar := abrirParaAnotar(t)
	defer cerrar()
	p.Idioma = "es-ES"
	p.leer()
	esperarRed(t, listo) // el texto del documento
	if leido != "Estimado cliente" || idioma != "es-ES" {
		t.Fatalf("se ha leído %q en %q", leido, idioma)
	}
	if !p.barra.Leyendo {
		t.Error("mientras lee, «Leer en voz alta» no está pulsado")
	}
	p.leer()
	if !callada || p.barra.Leyendo {
		t.Error("pulsar otra vez no calla")
	}

	hablar = func(string, string, func()) (func(), error) { return nil, voz.ErrNoDisponible }
	p.leer()
	if p.barra.Leyendo || p.Aviso != "Este navegador no sabe leer en voz alta." {
		t.Errorf("sin voz queda Leyendo=%v y el aviso %q", p.barra.Leyendo, p.Aviso)
	}
}

// TestElFirmadoVuelveSolo: el vigilante no entrega el original ni un fichero que no sea
// PDF, espera a que el firmado deje de crecer y lo entrega una sola vez.
func TestElFirmadoVuelveSolo(t *testing.T) {
	antes := pasoFirmado
	pasoFirmado = 10 * time.Millisecond
	defer func() { pasoFirmado = antes }()
	dir := t.TempDir()
	original := filepath.Join(dir, "contrato.pdf")
	_ = os.WriteFile(original, []byte("%PDF-original"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "registro.txt"), []byte("no"), 0o600)
	llego := make(chan string, 2)
	go vigilarFirmado(dir, original, func(r string) { llego <- r })
	firmado := filepath.Join(dir, "contrato_signed.pdf")
	if err := os.WriteFile(firmado, []byte("%PDF-firmado"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-llego:
		if r != firmado {
			t.Errorf("ha vuelto %q y el firmado es %q", r, firmado)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el firmado no ha vuelto")
	}
}

// TestElSaltoDePaginaDuraLoQueEnChromium: la raíz de lo que se mueve entre 60 —las tres
// medidas del arnés— con tope de tres segundos, y la curva empieza y acaba donde debe.
func TestElSaltoDePaginaDuraLoQueEnChromium(t *testing.T) {
	for _, c := range []struct {
		delta float32
		ms    int64
	}{{1436, 631}, {-1427, 629}, {2854, 890}, {13553, 1940}, {1e7, 3000}} {
		if got := ui.DuracionDesplazamiento(c.delta).Milliseconds(); got < c.ms-2 || got > c.ms+2 {
			t.Errorf("mover %g dura %d ms; en Chromium, %d", c.delta, got, c.ms)
		}
	}
	if ui.CurvaDesplazamiento(0) != 0 || ui.CurvaDesplazamiento(1) != 1 || ui.CurvaDesplazamiento(1.0/6) < 0.4 {
		t.Error("la curva no arranca en 0, no acaba en 1 o no va por delante como la medida")
	}
}

// TestLaTiraQueSeDeslizaSeDifuminaMientrasQuede: `MASCARA_SCROLL_X` —los últimos 24 px
// hacia el fondo— solo mientras quede contenido a la derecha; al llegar al final se
// apaga.
func TestLaTiraQueSeDeslizaSeDifuminaMientrasQuede(t *testing.T) {
	relleno := canvas.NewRectangle(color.Black)
	relleno.SetMinSize(fyne.NewSize(500, 20))
	d, s := deslizableCon(relleno)
	caja := conMascara(d, s, "surface")
	w := ventanaSinRelleno(caja)
	defer w.Close()
	w.Resize(fyne.NewSize(200, 20))
	degradado := caja.(*fyne.Container).Objects[1]
	if !degradado.Visible() || degradado.Position().X != 176 || degradado.Size().Width != 24 {
		t.Fatalf("con 300 px por ver, el degradado está visible=%v en x=%g de %g", degradado.Visible(), degradado.Position().X, degradado.Size().Width)
	}
	// Como la rueda o el arrastre: es lo que avisa a `OnScrolled`.
	s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-300, 0)})
	if s.Offset.X != 300 {
		t.Fatalf("la tira no ha llegado al final: x=%g", s.Offset.X)
	}
	if degradado.Visible() {
		t.Error("al final de la tira el degradado sigue anunciando más")
	}
}

// TestLaSombraDelPapelEsLaDeShadowSm: fuera del papel, pegado al borde de abajo, la
// sombra de `shadow-sm` —rgb(14,26,20) en dos capas, al 6 % y al 8 %, un píxel hacia
// abajo— y nada a seis píxeles; a un lado apenas asoma y arriba, con el desplazamiento,
// menos que abajo.
func TestLaSombraDelPapelEsLaDeShadowSm(t *testing.T) {
	capas := tema.Sombra(tema.SombraSM, tema.Claro)
	abajo := AlfaSombra(capas, 100, 100, 50, 100.5)
	arriba := AlfaSombra(capas, 100, 100, 50, -0.5)
	lejos := AlfaSombra(capas, 100, 100, 50, 106)
	if abajo.A < 10 || abajo.A > 30 || arriba.A >= abajo.A || lejos.A > 1 {
		t.Errorf("alfa abajo %d, arriba %d, a seis píxeles %d", abajo.A, arriba.A, lejos.A)
	}
	if abajo.R != 14 || abajo.G != 26 || abajo.B != 20 {
		t.Errorf("la sombra es %v y el tinte de la hoja es rgb(14,26,20)", abajo)
	}
}

// TestElRecorteEsLaZonaYVaAlPortapapeles: de la página dibujada a escala 2 sale justo la
// zona marcada (en puntos por dos), sobre blanco; y si el portapapeles la acepta, el
// aviso es el de «copiado» y no se descarga nada.
func TestElRecorteEsLaZonaYVaAlPortapapeles(t *testing.T) {
	pagina := image.NewRGBA(image.Rect(0, 0, 200, 100))
	draw.Draw(pagina, image.Rect(40, 20, 60, 30), image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
	datos, err := recorteEnPNG(pagina, CajaSVG{X: 20, Y: 10, Ancho: 10, Alto: 5}, 2)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 20 || b.Dy() != 10 {
		t.Fatalf("el recorte mide %v y la zona son 20×10 píxeles", b)
	}
	if r, g, _, _ := img.At(10, 5).RGBA(); r>>8 != 255 || g != 0 {
		t.Error("el recorte no es la zona roja de la página")
	}
	var copiado []byte
	antes := copiarImagen
	defer func() { copiarImagen = antes }()
	copiarImagen = func(b []byte) error { copiado = b; return nil }
	if aviso := llevarRecorte(datos, "recorte-1.png"); aviso != "pdf.snapshot.copied" || !bytes.Equal(copiado, datos) {
		t.Errorf("aviso %q, %d bytes en el portapapeles", aviso, len(copiado))
	}
}
