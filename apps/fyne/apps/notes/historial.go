// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"sync"
	"sync/atomic"
)

// Paridad web: apps/frontend/apps/notes/src/client/historial.ts.

// Accion es un paso que se puede deshacer: crear, editar, mover o eliminar una nota. Es
// el «Deshacer» de la barra del BasKet clásico, a nivel de CUADERNO; el del texto es
// del editor de cada nota.
//
// Deshacer y Rehacer hablan con el servicio, así que se llaman FUERA del hilo de la
// interfaz y devuelven el error: una inversa que falla sigue siendo lo siguiente que
// deshacer.
type Accion struct {
	// Etiqueta es lo que se dice en el botón y en el aviso («Deshacer: mover nota»).
	Etiqueta string
	Deshacer func() error
	Rehacer  func() error

	fusion string
	// id es el que devuelve Registrar, para deshacer ESTA y no la última.
	id uint64
}

// Historial es la pila de deshacer y la de rehacer. El cerrojo existe porque se registra
// desde el hilo de la interfaz y se deshace desde la rutina que habla con el servicio.
type Historial struct {
	mu     sync.Mutex
	pasado []Accion
	futuro []Accion
	// Limite son los pasos que se recuerdan: los 50 de la web.
	Limite int
}

// ultimoID numera las acciones registradas, de TODOS los historiales del proceso. No
// vive en el struct: `p.hist = Historial{}` (al cerrar sesión) lo pondría a 0, y un
// aviso viejo aún en pantalla desharía la primera acción nueva, que llevaría su número.
// En la web la identidad es el objeto, y no se repite.
var ultimoID atomic.Uint64

// limiteHistorial es el `limite = 50` del constructor de la web.
const limiteHistorial = 50

// Registrar apunta una acción recién hecha.
//
// Con `fusion`, una acción que llega con la MISMA marca que la de arriba no apila otra:
// se queda con la inversa de la primera y el «rehacer» de la última. Es lo que hace que
// los guardados de una misma edición —uno por pausa al teclear— se deshagan de una vez.
//
// Devuelve con qué número quedó apuntada, para DeshacerEsta.
func (h *Historial) Registrar(a Accion, fusion string) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := len(h.pasado); fusion != "" && n > 0 && h.pasado[n-1].fusion == fusion {
		h.pasado[n-1].Rehacer = a.Rehacer
		h.futuro = nil
		return h.pasado[n-1].id
	}
	a.fusion, a.id = fusion, ultimoID.Add(1)
	h.pasado = append(h.pasado, a)
	tope := h.Limite
	if tope <= 0 {
		tope = limiteHistorial
	}
	if len(h.pasado) > tope {
		h.pasado = h.pasado[len(h.pasado)-tope:]
	}
	// Lo que se podía rehacer deja de tener sentido: la rama cambió.
	h.futuro = nil
	return a.id
}

// DeshacerEsta deshace UNA acción concreta, la del número que dio Registrar: es el
// «Deshacer» del aviso tras eliminar o mover una nota (`deshacerEsta` de la web). Si
// desde entonces se hizo otra cosa, Deshacer desharía la que no es.
//
// Devuelve "" sin error si ya no está (se deshizo con Ctrl+Z o salió por el límite). Si
// era la de arriba pasa a rehacer; si no, se descarta. Si falla, vuelve a su sitio.
func (h *Historial) DeshacerEsta(id uint64) (string, error) {
	h.mu.Lock()
	i := slices.IndexFunc(h.pasado, func(a Accion) bool { return a.id == id })
	if i < 0 {
		h.mu.Unlock()
		return "", nil
	}
	a, cima := h.pasado[i], i == len(h.pasado)-1
	h.pasado = slices.Delete(h.pasado, i, i+1)
	h.mu.Unlock()

	var err error
	if a.Deshacer != nil {
		err = a.Deshacer()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		h.pasado = slices.Insert(h.pasado, min(i, len(h.pasado)), a)
		return "", err
	}
	if cima {
		h.futuro = append(h.futuro, a)
	}
	return a.Etiqueta, nil
}

// SiguienteDeshacer y SiguienteRehacer son la etiqueta de lo que toca, o "" si nada.
func (h *Historial) SiguienteDeshacer() string { return h.cima(&h.pasado) }

// SiguienteRehacer es la etiqueta de lo siguiente que se puede rehacer, o "".
func (h *Historial) SiguienteRehacer() string { return h.cima(&h.futuro) }

func (h *Historial) cima(pila *[]Accion) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := len(*pila); n > 0 {
		return (*pila)[n-1].Etiqueta
	}
	return ""
}

// Deshacer ejecuta la inversa de lo último y devuelve su etiqueta ("" si no había
// nada). Si falla, la acción vuelve a su sitio: sigue siendo lo siguiente que deshacer.
func (h *Historial) Deshacer() (string, error) { return h.mover(&h.pasado, &h.futuro, true) }

// Rehacer es lo mismo en el otro sentido.
func (h *Historial) Rehacer() (string, error) { return h.mover(&h.futuro, &h.pasado, false) }

func (h *Historial) mover(desde, hacia *[]Accion, deshacer bool) (string, error) {
	h.mu.Lock()
	n := len(*desde)
	if n == 0 {
		h.mu.Unlock()
		return "", nil
	}
	a := (*desde)[n-1]
	*desde = (*desde)[:n-1]
	h.mu.Unlock()

	// La llamada al servicio va SIN el cerrojo: tarda, y mientras tanto la interfaz
	// sigue preguntando qué etiqueta poner en los botones.
	f := a.Rehacer
	if deshacer {
		f = a.Deshacer
	}
	var err error
	if f != nil {
		err = f()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		*desde = append(*desde, a)
		return "", err
	}
	*hacia = append(*hacia, a)
	return a.Etiqueta, nil
}
