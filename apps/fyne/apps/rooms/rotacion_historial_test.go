// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

func comprobarHistorialRotacion(t *testing.T, w fyne.Window, h *Historial, paso string) {
	t.Helper()
	if h.Estado != Listo || len(h.Mensajes) != 4 {
		t.Fatalf("Rooms %s antes de capturar: estado=%v, mensajes=%d; quiero Listo y 4", paso, h.Estado, len(h.Mensajes))
	}
	s := scrollHistorial(t, h)
	t.Logf("Rooms %s ventana=%v estado=%v mensajes=%d historial=%v/%v viewport=%v offset=%v contenido=%v minimo=%v fondo=%t", paso,
		w.Canvas().Size(), h.Estado, len(h.Mensajes), h.Position(), h.Size(), s.Size(), s.Offset, s.Content.Size(), s.Content.MinSize(), alFondo(s))
	contenido, ok := s.Content.(*fyne.Container)
	if !ok {
		t.Fatal("el historial perdió la columna de mensajes")
	}
	d := fyne.CurrentApp().Driver()
	for i, o := range contenido.Objects {
		t.Logf("Rooms slot=%d tipo=%T caja=%v/%v", i, o, o.Position(), o.Size())
		if f, ok := o.(*FilaMensaje); ok {
			for _, o := range f.cuerpo.objetos {
				if texto, ok := o.(*canvas.Text); ok {
					t.Logf("Rooms mensaje=%s BBoxTexto=%v/%v texto=%q", f.Mensaje.ID, d.AbsolutePositionForObject(texto), texto.Size(), texto.Text)
				}
			}
		}
	}
	if !alFondo(s) {
		t.Fatal("girar un canal recién abierto dejó de mostrar el final")
	}
	if s.Content.Size().Width > s.Size().Width+1 {
		t.Fatal("el contenido conservó el ancho anterior al giro")
	}
	ultimo := filaDe(t, h, "m4")
	visibles := 0
	for _, o := range ultimo.cuerpo.objetos {
		if texto, ok := o.(*canvas.Text); ok && texto.Text == "vale" {
			textoEnHistorial(t, texto, s)
			visibles++
		}
	}
	if visibles == 0 {
		t.Fatal("el final cargado no contiene el texto visible «vale»")
	}
}

func TestLaRotacionDelHistorialConservaLaLecturaYReparteElTexto(t *testing.T) {
	h := NuevoHistorial(traductor(t), "es")
	h.Estado = Listo
	base := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h.Ahora = func() time.Time { return base.Add(time.Hour) }
	for i := range 30 {
		h.Mensajes = append(h.Mensajes, Mensaje{
			ID: fmt.Sprint(i), Autor: Autor{ID: fmt.Sprint(i), Nombre: "Ana"}, Creado: base.Add(time.Duration(i) * time.Minute),
			Cuerpo: strings.Repeat("Contenido del acta para revisar antes de aprobarla. ", 4),
		})
	}
	w := test.NewTempWindow(t, h)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(240, 120))
	s := scrollHistorial(t, h)
	if !alFondo(s) {
		t.Fatal("el historial no empieza por el final")
	}
	ancla := filaDe(t, h, "10")
	s.ScrollToOffset(fyne.NewPos(0, ancla.Position().Y+12))
	for _, ancho := range []float32{960, 480, 240, 960, 240} {
		w.Resize(fyne.NewSize(ancho, 120))
		s = scrollHistorial(t, h)
		if alFondo(s) {
			t.Fatal("girar llevó al final a quien estaba leyendo mensajes anteriores")
		}
		if y := ancla.Position().Y - s.Offset.Y; y < -13 || y > -11 {
			t.Fatalf("la lectura perdió su mensaje al cambiar a %g: ancla=%g, offset=%g", ancho, ancla.Position().Y, s.Offset.Y)
		}
		if s.Content.Size().Width != s.Size().Width {
			t.Fatalf("a %g el contenido mide %v dentro de %v", ancho, s.Content.Size(), s.Size())
		}
	}
	antes := ancla.Position().Y - s.Offset.Y
	h.Mensajes = append(h.Mensajes, Mensaje{ID: "nuevo", Autor: Autor{ID: "beto", Nombre: "Beto"}, Cuerpo: "Mensaje nuevo", Creado: base.Add(time.Hour)})
	h.Refresh()
	s = scrollHistorial(t, h)
	ancla = filaDe(t, h, "10")
	if y := ancla.Position().Y - s.Offset.Y; y < antes-1 || y > antes+1 || alFondo(s) {
		t.Fatal("un mensaje nuevo cambió la lectura que se conservó al girar")
	}
	s.ScrollToBottom()
	for _, tam := range []fyne.Size{{Width: 960, Height: 400}, {Width: 240, Height: 120}, {Width: 480, Height: 80}, {Width: 960, Height: 400}} {
		w.Resize(tam)
		s = scrollHistorial(t, h)
		if !alFondo(s) {
			t.Fatalf("el final se perdió a %v: offset=%v, contenido=%v", tam, s.Offset, s.Content.Size())
		}
		ultimo := filaDe(t, h, "nuevo")
		for _, o := range ultimo.cuerpo.objetos {
			if texto, ok := o.(*canvas.Text); ok {
				textoEnHistorial(t, texto, s)
			}
		}
	}
}

func scrollHistorial(t *testing.T, h *Historial) *container.Scroll {
	t.Helper()
	s, ok := h.cuerpo.(*container.Scroll)
	if !ok {
		t.Fatal("el historial con datos no es un Scroll")
	}
	return s
}

func textoEnHistorial(t *testing.T, texto *canvas.Text, s *container.Scroll) {
	t.Helper()
	d := fyne.CurrentApp().Driver()
	p, v := d.AbsolutePositionForObject(texto), d.AbsolutePositionForObject(s)
	if p.X < v.X-1 || p.Y < v.Y-1 || p.X+texto.Size().Width > v.X+s.Size().Width+1 || p.Y+texto.Size().Height > v.Y+s.Size().Height+1 {
		t.Fatalf("texto %q fuera del historial: BBoxTexto=%v/%v, viewport=%v/%v, offset=%v", texto.Text, p, texto.Size(), v, s.Size(), s.Offset)
	}
}
