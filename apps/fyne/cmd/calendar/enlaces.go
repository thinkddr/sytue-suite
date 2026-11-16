// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"net/url"
)

// enlacesIdentidad se usa solo en el hilo UI. Un enlace recibido sin sesión se
// conserva para la próxima; uno ya encolado para una sesión no cambia de dueño.
type enlacesIdentidad struct {
	vida       context.Context
	vigente    func() bool
	pendientes []*url.URL
	despachar  func(func())
	abrir      func(*url.URL)
	cerrada    bool
	epoca      uint64
}

func (e *enlacesIdentidad) recibir(u *url.URL) {
	if u == nil || e.cerrada {
		return
	}
	if e.vida == nil || e.vida.Err() != nil {
		e.pendientes = append(e.pendientes, u)
		return
	}
	epoca := e.epoca
	vigente := e.vigente
	despacharVigente(e.vida, e.despachar, func() {
		if !e.cerrada && e.epoca == epoca && vigente != nil && vigente() {
			e.abrir(u)
		}
	})
}

func (e *enlacesIdentidad) activar(vida context.Context, vigente func() bool) {
	if e.cerrada || vida == nil || vida.Err() != nil || vigente == nil {
		return
	}
	e.epoca++
	e.vida = vida
	e.vigente = vigente
	pendientes := e.pendientes
	e.pendientes = nil
	for _, u := range pendientes {
		e.recibir(u)
	}
}

func (e *enlacesIdentidad) retirar() { e.epoca++; e.vida = nil; e.vigente = nil }

func (e *enlacesIdentidad) cerrar() {
	e.cerrada = true
	e.epoca++
	e.vida, e.vigente, e.pendientes = nil, nil, nil
}
