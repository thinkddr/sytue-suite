// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// CampoFechaHora es `<input type="datetime-local">`: una fecha y una hora que se eligen
// juntas —«enviar más tarde», «reprogramar»—. Son CampoFecha y CampoHora uno al lado
// del otro, con `gap-1`, y se leen juntos con Leer.
//
// En la web el control lo pinta el navegador y su calendario es el del sistema; aquí se
// TECLEA, como en CampoFecha (ver allí el porqué y el techo).
//
// AlConfirmar se llama al pulsar Intro en cualquiera de los dos con una fecha y hora que
// valen: es el `onChange` del control nativo, que no dispara a cada tecla sino cuando
// el valor está entero. Dispararlo a cada tecla reprogramaría un mensaje a las 0:00
// mientras se teclea «09:30».
type CampoFechaHora struct {
	Fecha, Hora *Entrada
	// AlConfirmar recibe el instante tecleado, ya leído en la zona local.
	AlConfirmar func(time.Time)

	fila *fyne.Container
}

// HuecoFechaHora es el `gap-1` entre los dos campos.
var HuecoFechaHora = tema.Espacio(1)

// NuevoCampoFechaHora crea los dos campos de esa talla con `valor` escrito; con el cero,
// vacíos.
func NuevoCampoFechaHora(t TallaCampo, valor time.Time) *CampoFechaHora {
	c := &CampoFechaHora{Fecha: CampoFecha(t), Hora: CampoHora(t)}
	m := medidasCampo[t]
	// Lo justo para el ejemplo que llevan escrito, como el ancho intrínseco del control
	// nativo: `w-full` no, que en una fila de botones lo estira todo.
	ancho := func(ejemplo string) float32 {
		return 2*tema.Borde + m.margenIcono + m.margen + Medir(ejemplo, m.texto, Normal).Width
	}
	c.Fecha.Ancho, c.Hora.Ancho = ancho("31/12/2026"), ancho("09:30")
	c.Poner(valor)
	for _, e := range []*Entrada{c.Fecha, c.Hora} {
		e.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
				return false
			}
			if cuando, ok := c.Leer(time.Local); ok && c.AlConfirmar != nil {
				c.AlConfirmar(cuando)
			}
			return true
		}
	}
	c.fila = container.New(Fila{Hueco: HuecoFechaHora}, c.Fecha, c.Hora)
	return c
}

// Objeto es lo que se pone en la pantalla.
func (c *CampoFechaHora) Objeto() fyne.CanvasObject { return c.fila }

// Poner escribe un instante en los dos campos; el cero los vacía.
func (c *CampoFechaHora) Poner(t time.Time) {
	if t.IsZero() {
		c.Fecha.PonerTexto("")
		c.Hora.PonerTexto("")
		return
	}
	c.Fecha.PonerTexto(EscribirFecha(t))
	c.Hora.PonerTexto(EscribirHora(t))
}

// Leer es lo tecleado, si es una fecha y una hora que valen.
func (c *CampoFechaHora) Leer(zona *time.Location) (time.Time, bool) {
	return LeerFechaHora(c.Fecha.Texto(), c.Hora.Texto(), zona)
}

// LeerFechaHora junta una fecha y una hora tecleadas en un instante de `zona`. Las dos
// son obligatorias, como en `datetime-local`: una fecha sin hora no es un momento.
//
// La hora se pone con `time.Date` y no sumando horas a la medianoche: el día que cambia
// la hora, las 9:00 están a 8 o a 10 horas de las 0:00, no a 9.
func LeerFechaHora(fecha, hora string, zona *time.Location) (time.Time, bool) {
	dia, vacia, ok := LeerFecha(fecha, zona)
	if vacia || !ok {
		return time.Time{}, false
	}
	h, m, vacia, ok := LeerHora(hora)
	if vacia || !ok {
		return time.Time{}, false
	}
	return time.Date(dia.Year(), dia.Month(), dia.Day(), h, m, 0, 0, zona), true
}
