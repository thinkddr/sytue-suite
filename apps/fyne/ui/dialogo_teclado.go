// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
)

// El teclado del teléfono y la hoja de un diálogo.
//
// En la web la capa del Modal mide el viewport VISUAL (`--sytue-vvh`), así que al salir el
// teclado la hoja se apoya encima de él, el cuerpo encoge y el campo se desplaza a la
// vista. En Fyne nada de eso pasaba: en Android ni el lienzo ni su zona interactiva
// encogen, y el campo en el que se escribía —el asunto de un evento, el nombre de una
// carpeta— quedaba DEBAJO del teclado, con «Guardar» también tapado. Solo Cast lo
// arreglaba, y solo para su página.

// AltoTeclado dice lo que el teclado en pantalla tapa AHORA desde abajo de la pantalla,
// en unidades del lienzo `c` (0 si no está). Solo se pregunta en el teléfono; un
// guardián lo sustituye por su propio teclado.
//
// sesion.AltoTeclado contesta en PÍXELES en Android y en PUNTOS en iOS, y las unidades de
// Fyne en iOS ya son puntos: dividir también allí por la escala dejaba el teclado en un
// tercio en un iPhone de @3x.
var AltoTeclado = func(c fyne.Canvas) float32 {
	alto := sesion.AltoTeclado()
	if runtime.GOOS == "android" && c != nil && c.Scale() > 0 {
		alto /= c.Scale()
	}
	return alto
}

// tapadoPorTeclado es lo que el teclado se come de la capa del diálogo por abajo. La capa
// vive en `InteractiveArea`, que ya deja fuera la barra de inicio —y, en iOS, el propio
// teclado—: solo cuenta lo que pisa DENTRO de ella, así que no se descuenta dos veces.
func (d *Dialogo) tapadoPorTeclado(lienzo fyne.Size) float32 {
	if d.teclado <= 0 {
		return 0
	}
	tapa := d.teclado
	if c := lienzoDe(d.capa); c != nil {
		debajo := c.Size().Height - d.capa.Position().Y - lienzo.Height
		tapa -= max(debajo, 0)
	}
	// Siempre queda sitio para la cabecera: un teclado que lo tapara todo dejaría la
	// hoja sin aspa.
	return min(max(tapa, 0), max(lienzo.Height-HuecoHoja-AltoCabeceraDialogo(), 0))
}

// tapadoEnCapa calcula cuánto pisa el teclado en pantalla dentro de la capa `cp` de
// tamaño `lienzo`. En escritorio o sin teclado devuelve 0.
func tapadoEnCapa(c fyne.Canvas, cp *capa, lienzo fyne.Size) float32 {
	if !esMovil() || c == nil || AltoTeclado == nil {
		return 0
	}
	tapa := AltoTeclado(c)
	if tapa <= 0 {
		return 0
	}
	if cp != nil {
		debajo := c.Size().Height - cp.Position().Y - lienzo.Height
		tapa -= max(debajo, 0)
	}
	return max(tapa, 0)
}

// ponerTeclado apoya la hoja sobre un teclado que tapa `alto` y deja a la vista el campo
// enfocado. Con 0, la hoja vuelve abajo y acota el desplazamiento sobrante.
func (d *Dialogo) ponerTeclado(c fyne.Canvas, alto float32) {
	if d.capa == nil {
		return
	}
	cambio := d.teclado != alto
	if cambio {
		d.teclado = alto
		d.capa.Refresh()
	}
	if alto > 0 {
		MostrarFoco(d.Desplazador(), c, 0)
	} else if cambio {
		acotarDesplazamiento(d.Desplazador())
	}
}

func acotarDesplazamiento(s *container.Scroll) {
	if s == nil || s.Content == nil {
		return
	}
	maxY := max(s.Content.MinSize().Height-s.Size().Height, 0)
	if s.Offset.Y > maxY {
		s.ScrollToOffset(fyne.NewPos(s.Offset.X, maxY))
	}
}

// dialogoArriba es el diálogo de la capa de más arriba de `c`, si lo hay.
func dialogoArriba(c fyne.Canvas) *Dialogo {
	lista := c.Overlays().List()
	for i := len(lista) - 1; i >= 0; i-- {
		if cp, es := lista[i].(*capa); es {
			if d, es := cp.panel.(*Dialogo); es {
				return d
			}
			return nil
		}
	}
	return nil
}

type seguimientoTeclado struct {
	alto       float32
	parar      chan struct{}
	hecho      chan struct{}
	cerrarOnce sync.Once
}

func (s *seguimientoTeclado) cancelar() {
	s.cerrarOnce.Do(func() { close(s.parar) })
}

var (
	tecladoMu           sync.Mutex
	tecladoSeguimientos = map[fyne.Canvas]*seguimientoTeclado{}
)

// seguirTeclado mide el teclado mientras sale y lo acompaña hasta que se recoge. Si ya
// estaba abierto (cambiar de un campo de texto a otro dentro del mismo diálogo), lleva a
// la vista el nuevo campo de inmediato sin esperar otro ciclo de animación.
func seguirTeclado(c fyne.Canvas) {
	if c == nil {
		return
	}
	medir := AltoTeclado
	if medir == nil {
		return
	}
	actual := medir(c)
	alto, nuevo := registrarSeguimientoTeclado(c, actual)
	if d := dialogoArriba(c); d != nil {
		if alto > 0 {
			d.ponerTeclado(c, alto)
		} else {
			MostrarFoco(d.Desplazador(), c, 0)
		}
	}
	if nuevo == nil {
		return
	}
	go acompanarTeclado(c, nuevo, medir, alto)
}

func registrarSeguimientoTeclado(c fyne.Canvas, actual float32) (float32, *seguimientoTeclado) {
	tecladoMu.Lock()
	defer tecladoMu.Unlock()
	if seg, ok := tecladoSeguimientos[c]; ok {
		if actual > 0 {
			seg.alto = actual
		}
		return seg.alto, nil
	}
	seg := &seguimientoTeclado{
		alto:  max(actual, 0),
		parar: make(chan struct{}),
		hecho: make(chan struct{}),
	}
	tecladoSeguimientos[c] = seg
	return seg.alto, seg
}

func detenerSeguimientoTeclado(c fyne.Canvas) {
	if c == nil {
		return
	}
	tecladoMu.Lock()
	seg := tecladoSeguimientos[c]
	delete(tecladoSeguimientos, c)
	tecladoMu.Unlock()
	if seg != nil {
		seg.cancelar()
		<-seg.hecho
	}
}

func aplicarAltoTeclado(c fyne.Canvas, seg *seguimientoTeclado, alto float32) {
	tecladoMu.Lock()
	activo := tecladoSeguimientos[c] == seg
	if activo {
		seg.alto = alto
	}
	tecladoMu.Unlock()
	if !activo {
		return
	}
	fyne.Do(func() {
		if d := dialogoArriba(c); d != nil {
			d.ponerTeclado(c, alto)
		} else {
			seg.cancelar()
		}
	})
}

func acompanarTeclado(c fyne.Canvas, seg *seguimientoTeclado, medir func(fyne.Canvas) float32, inicial float32) {
	defer func() {
		tecladoMu.Lock()
		if tecladoSeguimientos[c] == seg {
			delete(tecladoSeguimientos, c)
		}
		tecladoMu.Unlock()
		close(seg.hecho)
	}()
	ultimo, ok := esperarSalidaTeclado(c, seg, medir, inicial)
	if !ok || ultimo <= 0 {
		return // se cerró el diálogo o no ha salido (solo lectura / teclado físico)
	}
	// Mientras siga fuera, atento a que cambie de alto (la fila de sugerencias, otro
	// idioma) y, al irse, la hoja vuelve abajo y acota su desplazamiento.
	vigilarTecladoAbierto(c, seg, medir, ultimo)
}

func esperarPasoTeclado(parar <-chan struct{}, d time.Duration) bool {
	reloj := time.NewTimer(d)
	defer reloj.Stop()
	select {
	case <-parar:
		return false
	case <-reloj.C:
		return true
	}
}

func esperarSalidaTeclado(c fyne.Canvas, seg *seguimientoTeclado, medir func(fyne.Canvas) float32, inicial float32) (float32, bool) {
	ultimo := inicial
	estables := 0
	for range 24 {
		if !esperarPasoTeclado(seg.parar, 25*time.Millisecond) {
			return 0, false
		}
		medido := medir(c)
		if medido <= 0 {
			continue
		}
		if medido != ultimo {
			ultimo = medido
			estables = 0
			aplicarAltoTeclado(c, seg, medido)
			continue
		}
		estables++
		if estables >= 3 {
			aplicarAltoTeclado(c, seg, medido)
			return ultimo, true
		}
	}
	return ultimo, true
}

func vigilarTecladoAbierto(c fyne.Canvas, seg *seguimientoTeclado, medir func(fyne.Canvas) float32, ultimo float32) {
	for esperarPasoTeclado(seg.parar, 80*time.Millisecond) {
		medido := medir(c)
		if medido <= 0 {
			aplicarAltoTeclado(c, seg, 0)
			return
		}
		if medido != ultimo {
			ultimo = medido
			aplicarAltoTeclado(c, seg, medido)
		}
	}
}
