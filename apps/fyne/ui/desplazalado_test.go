// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

// arrastrableMovilEn replica `findObjectAtPositionMatching` de
// `internal/driver/mobile/canvas.go`: recorre en preorden lo visible dentro del
// recorte de cada contenedor desplazable, conserva el descendiente más profundo que
// implemente `fyne.Draggable` o `mobile.Touchable` y devuelve el `fyne.Draggable`
// resultante (nil si el más profundo solo era `Touchable` o ninguno coincidió).
func arrastrableMovilEn(raiz fyne.CanvasObject, pos fyne.Position) fyne.Draggable {
	var hallado fyne.CanvasObject
	var caminar func(o fyne.CanvasObject, origen, clipPos fyne.Position, clipTam fyne.Size)
	caminar = func(o fyne.CanvasObject, origen, clipPos fyne.Position, clipTam fyne.Size) {
		if o == nil || !o.Visible() {
			return
		}
		p := origen.Add(o.Position())
		tam := o.Size()
		if pos.X < clipPos.X || pos.Y < clipPos.Y || pos.X >= clipPos.X+clipTam.Width || pos.Y >= clipPos.Y+clipTam.Height {
			return
		}
		dentro := pos.X >= p.X && pos.Y >= p.Y && pos.X < p.X+tam.Width && pos.Y < p.Y+tam.Height
		recorta := false
		if _, ok := o.(fyne.Scrollable); ok {
			recorta = true
		}
		var hijos []fyne.CanvasObject
		switch x := o.(type) {
		case fyne.Widget:
			r := test.WidgetRenderer(x)
			if _, ok := r.(interface{ IsClip() }); ok {
				recorta = true
			}
			hijos = r.Objects()
		case *fyne.Container:
			hijos = x.Objects
		}
		if recorta && !dentro {
			return
		}
		if dentro {
			switch o.(type) {
			case fyne.Draggable, mobile.Touchable:
				hallado = o
			}
		}
		sigClipPos, sigClipTam := clipPos, clipTam
		if recorta {
			x0 := max(clipPos.X, p.X)
			y0 := max(clipPos.Y, p.Y)
			x1 := min(clipPos.X+clipTam.Width, p.X+tam.Width)
			y1 := min(clipPos.Y+clipTam.Height, p.Y+tam.Height)
			sigClipPos = fyne.NewPos(x0, y0)
			sigClipTam = fyne.NewSize(max(x1-x0, 0), max(y1-y0, 0))
		}
		for _, h := range hijos {
			caminar(h, p, sigClipPos, sigClipTam)
		}
	}
	caminar(raiz, fyne.NewPos(0, 0), fyne.NewPos(0, 0), raiz.Size())
	d, _ := hallado.(fyne.Draggable)
	return d
}

// TestDesplazaLadoArrastraConElDedoYCedeElVertical: en el móvil el driver táctil solo
// busca `fyne.Draggable` (nunca llama a `Scrolled` con un dedo). Arrastrar sobre un
// botón interior alcanza `DesplazaLado`, corre el contenido en horizontal con aviso a
// `AlDesplazar` y sin pulsar el botón, y cede el gesto vertical al desplazador exterior.
func TestDesplazaLadoArrastraConElDedoYCedeElVertical(t *testing.T) {
	defer TactilDePrueba(true)()
	pulsado := false
	botones := make([]fyne.CanvasObject, 0, 8)
	for range 8 {
		b := NuevoBoton("Acción larga", func() { pulsado = true })
		botones = append(botones, b)
	}
	alto := canvas.NewRectangle(color.Black)
	alto.SetMinSize(fyne.NewSize(200, 900))
	var pag *container.Scroll
	avisos := 0
	lado := NuevoDesplazaLado(container.New(Fila{Hueco: 4}, botones...), 0, func() fyne.Scrollable { return pag })
	lado.AlDesplazar = func() { avisos++ }
	pag = container.NewVScroll(container.New(Columna{Hueco: 8}, lado, alto))
	w := test.NewTempWindow(t, pag)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(240, 300))

	if lado.AnchoContenido() <= 240 {
		t.Fatalf("la fila cabe entera (%g <= 240): la prueba no mide desplazamiento", lado.AnchoContenido())
	}
	d := arrastrableMovilEn(w.Canvas().Content(), fyne.NewPos(60, 22))
	if d != lado {
		t.Fatalf("sobre un botón de la tira el driver móvil alcanza %T y no *DesplazaLado", d)
	}
	d.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-90, 6)})
	d.DragEnd()
	if lado.X() != 90 || avisos == 0 || pulsado {
		t.Fatalf("tras arrastrar -90 px: X=%g, avisos=%d, pulsado=%v", lado.X(), avisos, pulsado)
	}
	// En vertical manda la página exterior (`Afuera`).
	d.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-4, -120)})
	d.DragEnd()
	if pag.Offset.Y != 120 || lado.X() != 90 {
		t.Fatalf("el gesto vertical dejó pag.Offset.Y=%g y lado.X=%g", pag.Offset.Y, lado.X())
	}
}
