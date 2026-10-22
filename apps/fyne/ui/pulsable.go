// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"
)

// pulsable es la CÁSCARA de los widgets de navegación: puntero, pulsación, foco de
// teclado y nombre accesible. No pinta nada; el renderer del widget que la embebe lee
// su estado y decide los colores.
//
// Existe por dos motivos medidos, y ninguno es la comodidad:
//
//   - **Sin cáscara no hay clics.** El repartidor de eventos de Fyne
//     (`internal/driver/util.go`) solo desciende por `*fyne.Container` y por
//     `fyne.Widget`. Un árbol de `canvas.Rectangle` y `canvas.Text` sueltos es decorado
//     mudo: se ve, y no se puede pulsar ni enfocar.
//   - **Un `*fyne.Container` se anuncia como «Container»** al lector de pantalla
//     (`container.go`: `AccessibilityLabel` devuelve la constante) y no se puede
//     cambiar. Una fila de menú tiene que anunciarse con su rótulo.
//
// La embeben las cuatro piezas interactivas de la navegación —FilaLateral,
// AccionLateral, Pestana y Miga—, que si no repetirían nueve métodos idénticos cuatro
// veces; y cuatro veces es donde una de ellas se deja el foco por el camino.
type pulsable struct {
	widget.BaseWidget

	// Nombre es el nombre accesible cuando el widget no tiene rótulo propio. Quien
	// tenga uno mejor sobreescribe AccessibilityLabel.
	Nombre string
	// Inerte es `disabled`: ni puntero, ni pulsación, ni foco.
	Inerte bool
	// AlPulsar es `onClick`. Lo dispara igual el ratón que Intro y Espacio.
	AlPulsar func()
	// AlEnfocar avisa de que el control ha ganado el foco: lo usa quien lo tiene dentro de
	// algo que se desplaza para llevarlo a la vista, que es lo que hace el navegador al
	// enfocar un botón recortado (la banda de la cinta). Mismo nombre que en Boton.
	AlEnfocar func()

	encima, pulsado, enfocado bool
	// pista es la ayuda que envuelve al control, para abrirla con el teclado.
	pista *Pista
	// alCambio avisa al padre de que el puntero o el foco han entrado o salido: es el
	// `group-hover`/`group-focus-within` de la web, que Fyne no tiene.
	alCambio func()
}

func (p *pulsable) avisar() {
	if p.alCambio != nil {
		p.alCambio()
	}
}

func (p *pulsable) ponerPista(x *Pista) { p.pista = x }

var (
	_ fyne.Tappable     = (*pulsable)(nil)
	_ fyne.Focusable    = (*pulsable)(nil)
	_ fyne.Accessible   = (*pulsable)(nil)
	_ desktop.Hoverable = (*pulsable)(nil)
	_ desktop.Mouseable = (*pulsable)(nil)
)

// Tapped es el clic del ratón y el toque del dedo.
func (p *pulsable) Tapped(*fyne.PointEvent) { p.activar() }

func (p *pulsable) activar() {
	if p.Inerte || p.AlPulsar == nil {
		return
	}
	p.AlPulsar()
}

// MouseIn enciende `:hover`.
func (p *pulsable) MouseIn(*desktop.MouseEvent) {
	if p.Inerte || p.encima {
		return
	}
	p.encima = true
	p.pista.alPuntero(true)
	p.Refresh()
	p.avisar()
}

// MouseMoved no hace nada: el estado ya lo fijan MouseIn y MouseOut.
func (p *pulsable) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve el widget a su color de reposo.
func (p *pulsable) MouseOut() {
	if !p.encima {
		return
	}
	p.encima = false
	p.pista.alPuntero(false)
	p.Refresh()
	p.avisar()
}

// FocusGained enciende el anillo. Es `:focus-visible` y no `:focus`: Fyne solo da el
// foco por teclado, así que la distinción del navegador aquí se cumple sola.
func (p *pulsable) FocusGained() {
	if p.Inerte {
		return
	}
	p.enfocado = true
	p.pista.alFoco(true)
	p.Refresh()
	p.avisar()
	if p.AlEnfocar != nil {
		p.AlEnfocar()
	}
}

// FocusLost apaga el anillo.
func (p *pulsable) FocusLost() {
	if !p.enfocado {
		return
	}
	p.enfocado = false
	p.pista.alFoco(false)
	p.Refresh()
	p.avisar()
}

// TypedRune activa con Espacio, que es la tecla que llega como runa y no como tecla.
func (p *pulsable) TypedRune(r rune) {
	if r == ' ' {
		p.activar()
	}
}

// TypedKey activa con Intro, las dos teclas que el navegador dispara sobre un `button`,
// y sube Escape a la capa de encima (`Descartar`).
//
// Lo que no es de un botón SUBE a la ventana ([SubirTecla]): en la web el `keydown`
// de un botón enfocado llega al del documento, y así Escape o Av Pág de un visor siguen
// funcionando con el foco en una miniatura o en una pestaña.
func (p *pulsable) TypedKey(e *fyne.KeyEvent) {
	switch e.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		p.activar()
		return
	case fyne.KeyEscape, mobile.KeyBack:
		p.pista.alEscape()
		if c := p.lienzo(); c != nil && Descartar(c) {
			return
		}
	default:
	}
	SubirTecla(p.lienzo(), e)
}

// SubirTecla entrega a la ranura de teclas sueltas de la ventana (`OnTypedKey`) la tecla
// que el control enfocado no usa, como sube un `keydown` al documento.
func SubirTecla(c fyne.Canvas, e *fyne.KeyEvent) {
	if c == nil {
		return
	}
	if f := c.OnTypedKey(); f != nil {
		f(e)
	}
}

// lienzo es el lienzo que tiene este control enfocado. Se busca así porque un widget que
// embebe `pulsable` es, para Fyne, el de FUERA, y desde aquí dentro no se le conoce:
// `CanvasForObject(p)` no lo encuentra.
func (p *pulsable) lienzo() fyne.Canvas {
	a := fyne.CurrentApp()
	if a == nil {
		return nil
	}
	for _, w := range a.Driver().AllWindows() {
		if f, es := w.Canvas().Focused().(interface{ esPulsable() *pulsable }); es && f.esPulsable() == p {
			return w.Canvas()
		}
	}
	return nil
}

func (p *pulsable) esPulsable() *pulsable { return p }

// MouseDown y MouseUp son `:active`: el color de pulsado mientras el botón del ratón
// está abajo. No lo ponía nadie, y ningún `active:` de la suite llegaba a verse.
func (p *pulsable) MouseDown(*desktop.MouseEvent) {
	if p.Inerte {
		return
	}
	p.pulsado = true
	p.Refresh()
}

func (p *pulsable) MouseUp(*desktop.MouseEvent) {
	if !p.pulsado {
		return
	}
	p.pulsado = false
	p.Refresh()
}

// AccessibilityLabel es el nombre que se anuncia.
func (p *pulsable) AccessibilityLabel() string { return p.Nombre }

// AccessibilityRole: todo lo de esta familia se activa como un botón. Fyne 2.8 solo
// tiene cuatro roles (botón, contenedor, enlace y texto), así que una pestaña y una
// miga de pan se anuncian como botón, que es lo que hacen.
func (p *pulsable) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// Pulsable es la MISMA cáscara, exportada para que las aplicaciones escriban sus
// propios widgets sin volver a derivar los nueve métodos.
//
// Hace falta porque hay piezas que son de UNA app y no del catálogo compartido —la
// tesela del lanzador de Sytue Hub, la fila de triaje de Mail—: en la web tampoco
// están en `packages/ui`, viven en `apps/<app>/src/components`. Sin esto, cada app se
// copiaría estos nueve métodos, que es exactamente lo que `docs/CONVENTIONS.md` §13.2
// prohíbe.
//
// Es un ALIAS y no un tipo nuevo a propósito: así los widgets de este paquete siguen
// embebiendo `pulsable` sin tocar una línea.
type Pulsable = pulsable

// Encima es `:hover`: el puntero está sobre el widget.
func (p *pulsable) Encima() bool { return p.encima }

// Enfocado es `:focus-visible`. Fyne solo da el foco por teclado, así que la
// distinción del navegador se cumple sola.
func (p *pulsable) Enfocado() bool { return p.enfocado }

// AlCambiarEstado avisa a `f` cuando el puntero o el foco entran o salen de este
// control. Es el `group-hover`/`group-focus-within` de la web para un padre que vive
// fuera de este paquete: la fila de Calendar enseña su kebab mientras el foco o el
// puntero están en él, y sin el aviso lo escondía nada más llegarle el foco.
func (p *pulsable) AlCambiarEstado(f func()) { p.alCambio = f }
