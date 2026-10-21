// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// DesplazarSuave lleva `s` a la altura `y` como `scrollTo({behavior: 'smooth'})` de
// Chromium, o de golpe si el sistema pide menos movimiento: la web lo CONSULTA
// (`prefers-reduced-motion`), porque un `behavior` pedido desde JavaScript gana a la regla
// de la hoja. Para la animación `previa` si la hay (otro salto a medias) y devuelve la
// nueva, nil si ha ido de golpe. Lo usan el salto de página de CertiPDF y el del título
// de un enlace (`h=`) en Notepads.
func DesplazarSuave(s *container.Scroll, y float32, previa *fyne.Animation) *fyne.Animation {
	if previa != nil {
		previa.Stop()
	}
	x, desde := s.Offset.X, s.Offset.Y
	delta := y - desde
	if delta == 0 || sistema.MenosMovimiento() {
		s.ScrollToOffset(fyne.NewPos(x, y))
		return nil
	}
	a := fyne.NewAnimation(DuracionDesplazamiento(delta), func(f float32) {
		s.ScrollToOffset(fyne.NewPos(x, desde+delta*f))
	})
	a.Curve = CurvaDesplazamiento
	a.Start()
	return a
}

// DuracionDesplazamiento es la de Chromium para un desplazamiento pedido por programa:
// la raíz de lo que se mueve, entre 60, en segundos, con un tope de tres. Medido en el
// arnés (`medir-desplazar.js`): 1436 px llegan en ≈ 0,63 s, 2854 en ≈ 0,89 y 13 553 en
// ≈ 1,7 (la cola ya no se ve).
func DuracionDesplazamiento(delta float32) time.Duration {
	return time.Duration(math.Min(math.Sqrt(math.Abs(float64(delta))), 180) / 60 * float64(time.Second))
}

// CurvaDesplazamiento es la que mejor sigue lo medido: arranca rápido y frena largo.
// ponytail: ajustada a ojo sobre las muestras —el 46 % a un sexto del tiempo y el 94 % a
// algo más de la mitad—, que en un Chromium sin pantalla llegan con los fotogramas
// irregulares. Una cúbica de salida; la de Chromium no se ha leído de su código.
func CurvaDesplazamiento(t float32) float32 {
	u := 1 - t
	return 1 - u*u*u
}

// MostrarFoco desplaza `s` lo justo para que se vea el control enfocado del lienzo, si
// está dentro: es lo que hace el navegador de un teléfono cuando el teclado sale y tapa
// el campo en el que se escribe. En Fyne nada se mueve y el campo acaba debajo del
// teclado (visto en el emulador con el buscador de «Mis vídeos» de Cast). Devuelve si
// ha movido algo.
//
// `tapado` es lo que el teclado tapa desde abajo del lienzo, en sus unidades: Fyne no lo
// sabe —ni el lienzo ni su zona interactiva encogen al salir el teclado, medido en el
// emulador—, así que lo pregunta la app al sistema (sesion.AltoTeclado). Con 0 solo se
// mira el borde del desplazamiento.
func MostrarFoco(s *container.Scroll, c fyne.Canvas, tapado float32) bool {
	if s == nil || c == nil || s.Content == nil {
		return false
	}
	// Sitio para el teclado debajo del contenido: una página más corta que la pantalla no
	// se puede desplazar, y el campo se quedaba debajo del teclado igual.
	if cont, ok := s.Content.(*fyne.Container); ok {
		if h, ok := cont.Layout.(*HuecoTeclado); ok && h.Alto != tapado {
			h.Alto = tapado
			cont.Refresh()
			s.Refresh()
		}
	}
	if c.Focused() == nil {
		return false
	}
	f, ok := c.Focused().(fyne.CanvasObject)
	if !ok {
		return false
	}
	// ¿Está dentro? Por geometría: Fyne no guarda el padre de un objeto, y el contenido
	// de `s` ocupa, en el lienzo, su ancho y todo su alto a partir de donde empieza.
	d := fyne.CurrentApp().Driver()
	ps, pf := d.AbsolutePositionForObject(s), d.AbsolutePositionForObject(f)
	inicio := ps.Y - s.Offset.Y
	if pf.X < ps.X || pf.X > ps.X+s.Size().Width || pf.Y < inicio || pf.Y > inicio+s.Content.Size().Height {
		return false
	}
	arriba := pf.Y - ps.Y
	abajo := arriba + f.Size().Height
	margen := tema.Espacio(4)
	// Lo que se ve del desplazamiento: hasta su borde o, si el teclado lo tapa antes,
	// hasta el teclado. En el móvil el lienzo NO encoge al salir el teclado; lo que
	// encoge es su zona interactiva, y en ella están las posiciones absolutas del driver.
	visible := s.Size().Height
	if tapado > 0 {
		zp, _ := c.InteractiveArea()
		visible = min(visible, c.Size().Height-tapado-zp.Y-ps.Y)
	}
	switch {
	case abajo+margen > visible:
		s.ScrollToOffset(fyne.NewPos(s.Offset.X, s.Offset.Y+abajo+margen-visible))
	case arriba < 0:
		s.ScrollToOffset(fyne.NewPos(s.Offset.X, max(s.Offset.Y+arriba-margen, 0)))
	default:
		return false
	}
	return true
}

// AlEscribir, si no es nil, se llama cuando un campo de la suite recibe el foco en el
// teléfono, con su lienzo: es donde la app deja el campo a la vista por encima del
// teclado (MostrarFoco), que en el teléfono Fyne no hace.
var AlEscribir func(c fyne.Canvas)

// avisarEscritura es la llamada a AlEscribir desde un campo que acaba de recibir el foco.
func avisarEscritura(o fyne.CanvasObject) {
	if !esMovil() {
		return
	}
	c := lienzoDe(o)
	if c == nil {
		return
	}
	// La hoja del diálogo de encima se apoya sobre el teclado (dialogo_teclado.go).
	if dialogoArriba(c) != nil {
		seguirTeclado(c)
	}
	if f := AlEscribir; f != nil {
		f(c)
	}
}

// HuecoTeclado es el layout del contenido de un desplazamiento que deja, debajo, el sitio
// que tapa el teclado del teléfono (lo pone MostrarFoco). Sin teclado, Alto es 0 y no
// cambia nada.
type HuecoTeclado struct{ Alto float32 }

// MinSize es el del contenido más el hueco.
func (h *HuecoTeclado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, m.Height+h.Alto)
}

// Layout coloca el contenido sin el hueco.
func (h *HuecoTeclado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, max(t.Height-h.Alto, objs[0].MinSize().Height)))
}
