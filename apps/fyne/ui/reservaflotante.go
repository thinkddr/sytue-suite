// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// # El hueco del botón flotante
//
// El «+» flotante (BotonFlotante) va encima de la pantalla, abajo a la derecha, y las listas
// no dejaban sitio debajo de su última fila: al bajar del todo, el botón la tapaba (Mail,
// Contacts, Drive y Cast en la auditoría del 2026-11-24). En la web lo que desplaza bajo el
// botón lleva `sytue-reserva-flotante` y acaba con un relleno de abajo; aquí lo dan estas
// dos piezas. En TODOS los anchos desde el 2026-11-25, que es desde cuando el botón se ve
// también en escritorio: antes, solo por debajo de `Estrecho`.

// ReservaFlotante es lo que una lista deja libre al final para el botón flotante: el botón
// y su margen por arriba y por abajo (`h-14` y dos `bottom-4`), 88 con el rem de 16.
var ReservaFlotante = LadoBotonFlotante + 2*MargenBotonFlotante

// ConReservaFlotante envuelve el contenido de un desplazamiento vertical y le añade la
// reserva al final. Lo de dentro se coloca arriba con su alto; lo que sobra queda vacío,
// del fondo de la lista.
func ConReservaFlotante(contenido fyne.CanvasObject) *fyne.Container {
	return container.New(reservaFlotante{}, contenido)
}

type reservaFlotante struct{}

var (
	_ fyne.Layout = reservaFlotante{}
	_ conAlto     = reservaFlotante{}
)

// reservaPara es la reserva que toca a lo que vive en el lienzo de `o`: la entera en
// cuanto está en una ventana, sea del ancho que sea.
func reservaPara(o fyne.CanvasObject) float32 {
	if c := lienzoDe(o); c != nil && c.Size().Width > 0 {
		return ReservaFlotante
	}
	return 0
}

func (reservaFlotante) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, m.Height+reservaPara(objs[0]))
}

func (reservaFlotante) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return AltoA(objs[0], ancho) + reservaPara(objs[0])
}

func (reservaFlotante) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, max(t.Height-reservaPara(objs[0]), objs[0].MinSize().Height)))
}

// ReservaEnLista es la reserva en un `widget.List`, que no tiene relleno de abajo: una fila
// VACÍA más al final, del alto de ReservaFlotante. Se monta una vez sobre la lista ya
// creada (envuelve su Length y su UpdateItem) y se ajusta con Ajustar.
type ReservaEnLista struct {
	l      *widget.List
	largo  func() int
	poner  func(widget.ListItemID, fyne.CanvasObject)
	activa bool
	// fila es el índice que lleva puesto el alto de la reserva, o -1.
	fila int
	// plantilla es el alto de una fila cualquiera, para devolvérselo a la que deja de
	// ser la reserva cuando la lista crece.
	plantilla float32
}

// ReservarEnLista monta la reserva sobre `l`. Las filas de verdad se rellenan como
// siempre; la de la reserva se esconde, y si se pulsa no queda elegida.
func ReservarEnLista(l *widget.List) *ReservaEnLista {
	r := &ReservaEnLista{l: l, largo: l.Length, poner: l.UpdateItem, fila: -1}
	l.Length = func() int {
		n := r.largo()
		if r.activa {
			n++
		}
		return n
	}
	l.UpdateItem = func(id widget.ListItemID, o fyne.CanvasObject) {
		if r.activa && id >= r.largo() {
			o.Hide()
			return
		}
		o.Show()
		r.poner(id, o)
	}
	elegir := l.OnSelected
	l.OnSelected = func(id widget.ListItemID) {
		if r.activa && id >= r.largo() {
			l.Unselect(id)
			return
		}
		if elegir != nil {
			elegir(id)
		}
	}
	return r
}

// Ajustar enciende la reserva en cuanto la lista está en una ventana con ancho (0: el del
// lienzo de la lista) y le pone su alto al índice que toca. Se llama donde la app ya fija los altos de
// sus filas —tras cambiar la lista o el ancho— y ANTES de fijarlos: devuelve al alto de
// la plantilla la fila que deja de ser reserva, y la app le pone después el suyo si es
// otro. Nunca desde Length ni UpdateItem, que corren dentro del pintado de la lista.
func (r *ReservaEnLista) Ajustar(ventana float32) {
	if ventana <= 0 {
		if c := lienzoDe(r.l); c != nil {
			ventana = c.Size().Width
		}
	}
	r.encender(ventana > 0)
}

// Quitar apaga la reserva aunque la lista esté en una ventana: es para quien la deja al
// final de OTRO desplazamiento (la franja compacta de Drive, que lleva la lista dentro).
func (r *ReservaEnLista) Quitar() { r.encender(false) }

// Largo es el número de filas de verdad, sin la de la reserva: el último índice al que
// pueden ir Fin y ↓ (la reserva no es de nadie y abrirla no hace nada).
func (r *ReservaEnLista) Largo() int { return r.largo() }

func (r *ReservaEnLista) encender(activa bool) {
	n := r.largo()
	if r.fila >= 0 && (!activa || r.fila != n) {
		r.l.SetItemHeight(r.fila, r.altoPlantilla())
		r.fila = -1
	}
	r.activa = activa
	if activa && r.fila != n {
		r.l.SetItemHeight(n, ReservaFlotante)
		r.fila = n
	}
}

// Activa dice si la lista lleva ahora la fila de la reserva.
func (r *ReservaEnLista) Activa() bool { return r.activa }

func (r *ReservaEnLista) altoPlantilla() float32 {
	if r.plantilla == 0 && r.l.CreateItem != nil {
		r.plantilla = r.l.CreateItem().MinSize().Height
	}
	return r.plantilla
}
