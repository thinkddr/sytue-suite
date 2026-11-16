// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # El parpadeo del cursor
//
// En una hoja blanca, una raya de 1 px quieta cuesta de encontrar: el navegador y el
// `widget.Entry` de Fyne la hacen parpadear, y el cuerpo de una nota también. Encendida y
// apagada a medias de PeriodoParpadeo; cada tecla, clic o cambio la vuelve a encender y
// empieza la cuenta, para que mientras se escribe se vea siempre (es lo que hace el
// navegador). Sin foco no hay cursor, y con «menos movimiento» se queda encendida y quieta.
//
// Corre como una animación de Fyne y no con un temporizador, igual que el cursor del
// folio de Docs: los fotogramas van en el hilo de la interfaz, así que no queda ninguna
// rutina viva que repinte a la vez que otra cosa, ni al cerrar la ventana. La FASE no sale
// de la fracción de la animación sino del reloj (`zonaEdicion.ahora`), que es lo que deja
// reiniciarla en cada tecla sin rehacer la animación y lo que las pruebas cambian por uno
// suyo. Cada fotograma solo repinta la raya, y solo cuando cambia de fase: el resto del
// cuerpo (la prosa maquetada) no se vuelve a pintar.

// PeriodoParpadeo es lo que el cursor pasa encendido y lo que pasa apagado: los 530 ms
// del parpadeo de Windows y GTK, que es el que ve la persona en el navegador.
const PeriodoParpadeo = 530 * time.Millisecond

// reloj es la hora que mira el parpadeo; la cambian las pruebas.
func (z *zonaEdicion) reloj() time.Time {
	if z.ahora != nil {
		return z.ahora()
	}
	return time.Now()
}

// encenderCursor deja la raya encendida y empieza la cuenta del parpadeo, que arranca si
// hace falta: al recibir el foco y tras cada tecla, clic o cambio.
func (z *zonaEdicion) encenderCursor() {
	z.desde = z.reloj()
	if !z.enfocado || ui.MenosMovimiento() {
		z.pararParpadeo()
		return
	}
	z.verRaya(true)
	if z.parpadeo == nil {
		z.parpadeo = fyne.NewAnimation(2*PeriodoParpadeo, func(float32) { z.ticParpadeo() })
		z.parpadeo.Curve = fyne.AnimationLinear
		z.parpadeo.RepeatCount = fyne.AnimationRepeatForever
		z.parpadeo.Start()
	}
}

// pararParpadeo para la animación y deja la raya encendida para la próxima vez.
func (z *zonaEdicion) pararParpadeo() {
	if z.parpadeo != nil {
		z.parpadeo.Stop()
		z.parpadeo = nil
	}
	z.verRaya(true)
}

// ticParpadeo es cada fotograma: pone la raya en la fase que toca según el reloj. Sin
// foco, sin ventana (se ha cerrado: cerrarla no avisa con FocusLost) o con «menos
// movimiento» recién pedido, se para.
func (z *zonaEdicion) ticParpadeo() {
	if !z.enfocado || ui.MenosMovimiento() || !z.tieneElFoco() {
		z.pararParpadeo()
		return
	}
	fase := int64(z.reloj().Sub(z.desde) / PeriodoParpadeo)
	z.verRaya(fase%2 == 0)
}

// tieneElFoco dice si la zona sigue siendo lo enfocado de una ventana abierta.
func (z *zonaEdicion) tieneElFoco() bool {
	c := fyne.CurrentApp().Driver().CanvasForObject(z)
	return c != nil && c.Focused() == fyne.Focusable(z)
}

// verRaya enciende o apaga la raya pintada, repintando solo ella y solo si cambia.
func (z *zonaEdicion) verRaya(ver bool) {
	if z.apagado == !ver {
		return
	}
	z.apagado = !ver
	if z.raya != nil {
		z.raya.Hidden = z.apagado
		canvas.Refresh(z.raya)
	}
}

// cursorEncendido dice si la raya del cursor se ve ahora mismo (para las pruebas y las
// capturas): con el cuerpo enfocado, sin selección y en la fase encendida.
func (z *zonaEdicion) cursorEncendido() bool {
	return z.raya != nil && !z.raya.Hidden
}
