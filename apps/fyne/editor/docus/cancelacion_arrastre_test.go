// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
)

func TestCancelarElTiradorRestauraLaImagenSinGuardar(t *testing.T) {
	src := pngLleno(t, 40, 20, color.NRGBA{B: 200, A: 255})
	f := folioCon(t, `<p><img src="`+src+`" data-w="80" data-h="40"></p>`)
	c := software.NewCanvas()
	c.SetContent(container.NewStack(f))
	c.Resize(fyne.NewSize(1000, 900))
	im := f.imagenes()[0]
	f.sel = &im.q
	ox, oy := f.pag.origen()
	esq := fyne.NewPos(ox+im.x+im.w, oy+im.y+im.h)
	antes, pasos := f.Serializar(), len(f.hist.atras)
	cambios := 0
	f.AlEditar = func() { cambios++ }
	if !f.pulsarImagen(esq) || f.arrastre == nil {
		t.Fatal("el tirador no empezó la edición de imagen")
	}
	ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: esq.AddXY(60, 0)}}
	f.pag.Dragged(ev)
	if f.Serializar() == antes {
		t.Fatal("el tirador no cambió la previa")
	}
	f.pag.CancelarArrastre()
	f.pag.Dragged(ev) // el sistema puede entregar un movimiento pendiente
	f.pag.DragEnd()
	if f.Serializar() != antes || cambios != 0 || len(f.hist.atras) != pasos {
		t.Fatalf("cancelar guardó o dejó la imagen provisional: cambios=%d, historial=%d", cambios, len(f.hist.atras))
	}
	if !f.pulsarImagen(esq) {
		t.Fatal("no se puede iniciar el gesto siguiente")
	}
	f.pag.Dragged(ev)
	f.pag.DragEnd()
	if f.Serializar() == antes || cambios != 1 {
		t.Fatalf("el gesto nuevo no se confirmó una vez: cambios=%d", cambios)
	}
}
