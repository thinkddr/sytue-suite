// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestAnchoAltoYRecorteExactosDeLaImagen: lo que la barra flotante de la web añade a la
// cinta. El ancho exacto arrastra el alto con la proporción y se acota al ancho del texto;
// el alto, al revés, hasta 4000; el recorte de un lado va en % hasta 90 y el conjunto se
// acota como `clampCrop`. Cada cifra es un paso de deshacer agrupado con las siguientes.
// Se vio en ROJO sin ellas: el documento no cambiaba.
func TestAnchoAltoYRecorteExactosDeLaImagen(t *testing.T) {
	src := pngLleno(t, 40, 20, color.NRGBA{B: 200, A: 255})
	f := folioCon(t, `<p>a<img src="`+src+`" data-w="200" data-h="100" data-sytue-image="">b</p>`)
	f.sel = &punto{0, 1}
	f.medirSeleccionada(medidaAncho, "300")
	if got := f.Serializar(); !strings.Contains(got, `data-w="300" data-h="150"`) {
		t.Errorf("el ancho exacto guarda la proporción: %.300s", got[min(len(got), 60):])
	}
	f.medirSeleccionada(medidaAlto, "50")
	if got := f.Serializar(); !strings.Contains(got, `data-w="100" data-h="50"`) {
		t.Errorf("el alto exacto guarda la proporción: %.300s", got[min(len(got), 60):])
	}
	f.medirSeleccionada(medidaAncho, "99999")
	if im, _ := f.imagenSeleccionada(); im.img.Ancho != redondear2(f.anchoTexto()) {
		t.Errorf("el ancho se acota al del texto (%g) y es %g", f.anchoTexto(), im.img.Ancho)
	}
	f.medirSeleccionada(medidaAlto, "3")
	if im, _ := f.imagenSeleccionada(); im.img.Alto != 24 {
		t.Errorf("el alto no baja de 24 y es %g", im.img.Alto)
	}
	antes := f.Serializar()
	f.medirSeleccionada(medidaAncho, "abc")
	if f.Serializar() != antes {
		t.Error("lo que no es una cifra no cambia la imagen")
	}
	f.recortarSeleccionada("t=10")
	f.recortarSeleccionada("b=95")
	// 10 + 90 pasa de 90 por eje: los dos se escalan a la par, como `clampCrop`.
	if im, _ := f.imagenSeleccionada(); abs(im.img.Recorte[0]-0.09) > 1e-4 || abs(im.img.Recorte[2]-0.81) > 1e-4 {
		t.Errorf("arriba 10 %% y abajo 90 %% acotados a la suma de 0,9: %v", im.img.Recorte)
	}
	if !strings.Contains(f.Serializar(), `data-crop=`) {
		t.Errorf("el recorte se guarda en data-crop: %.300s", f.Serializar())
	}
	f.recortarSeleccionada("x=10")
	f.recortarSeleccionada("")
	if strings.Contains(f.Serializar(), "data-crop") {
		t.Error("«Reiniciar» quita el recorte")
	}
}

// TestElPanelDeHerramientasDeImagen: la cinta enseña «Herramientas de imagen» con la imagen
// seleccionada; en su panel, escribir el ancho pone el alto que sigue a la proporción sin
// volver a mandarlo (ni entrar en un bucle), y el recorte por lados llega al documento.
func TestElPanelDeHerramientasDeImagen(t *testing.T) {
	test.NewTempApp(t)
	src := pngLleno(t, 40, 20, color.NRGBA{B: 200, A: 255})
	f := folioCon(t, `<p>a<img src="`+src+`" data-w="200" data-h="100" data-sytue-image="">b</p>`)
	c := software.NewCanvas()
	c.SetContent(container.NewStack(f))
	c.Resize(fyne.NewSize(1000, 900))
	ox, oy := f.pag.origen()
	im := f.imagenes()[0]
	f.pag.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(ox+im.x+5, oy+im.y+5)}, Button: desktop.MouseButtonPrimary})
	hay := false
	for _, p := range piezasDe(nuevoInicio("es").cuerpoInsertar(f.ctx)) {
		if b, ok := p.(*editor.BotonCinta); ok && b.Rotulo == "image.toolbar" {
			hay = true
		}
	}
	if !hay {
		t.Fatal("con la imagen seleccionada, la cinta enseña «Herramientas de imagen»")
	}
	campos := map[string]*ui.Entrada{}
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Entrada:
			campos[x.Rotulo] = x
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		}
	}
	ir(f.panelMedidas(f.ctx))
	if campos["image.width"] == nil || campos["image.height"] == nil || campos["image.crop.left"] == nil {
		t.Fatalf("faltan campos en el panel: %v", campos)
	}
	if campos["image.width"].Texto() != "200" || campos["image.height"].Texto() != "100" {
		t.Errorf("el panel abre con la medida de ahora: %q × %q", campos["image.width"].Texto(), campos["image.height"].Texto())
	}
	campos["image.width"].PonerTexto("120")
	if got := campos["image.height"].Texto(); got != "60" {
		t.Errorf("el alto sigue al ancho: %q", got)
	}
	if !strings.Contains(f.Serializar(), `data-w="120" data-h="60"`) {
		t.Errorf("el ancho del panel llega al documento: %.300s", f.Serializar())
	}
	campos["image.crop.left"].PonerTexto("25")
	if im, _ := f.imagenSeleccionada(); im.img.Recorte[3] != 0.25 {
		t.Errorf("el recorte izquierdo del panel: %v", im.img.Recorte)
	}
	// Todo lo tecleado en el panel se deshace de una vez, como lo escrito en el folio.
	f.deshacer()
	if strings.Contains(f.Serializar(), `data-w="120"`) {
		t.Errorf("deshacer vuelve a como estaba: %.300s", f.Serializar())
	}
}
