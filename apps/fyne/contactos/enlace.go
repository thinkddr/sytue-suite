// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"log/slog"
	"net/url"
	"regexp"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El enlace canónico de un contacto: `/contacts/{id}`, el que componen los resultados
// de Contacts en el buscador de la suite y los intents del Hub (pages/contacts/[id].ts
// en la web). Aquí llega como `sytue-contacts:/contacts/{id}`, o con el prefijo de la
// app delante si viene de la dirección de la organización.

var reEnlace = regexp.MustCompile(`(?i)/contacts/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/?$`)

// IDDeEnlace es el id del contacto de un enlace canónico, o "" si no lo es.
func IDDeEnlace(u *url.URL) string {
	if u == nil {
		return ""
	}
	ruta := u.Path
	if ruta == "" {
		ruta = u.Opaque
	}
	if m := reEnlace.FindStringSubmatch(ruta); m != nil {
		return m[1]
	}
	return ""
}

// AbrirContacto resuelve el contacto a su libreta, la abre y enseña su ficha. Si no es
// de quien lo pide o ya no existe, se queda en la vista sin decir cuál de las dos: es lo
// que hace la web, porque un «no encontrado» propio delataría qué ids existen. Bloquea:
// se llama en segundo plano.
func (a *App) AbrirContacto(id string) {
	if a.svc == nil {
		return
	}
	ctx, fin := a.ctx()
	c, err := a.svc.Contact(ctx, id)
	fin()
	if err != nil {
		slog.Info("contactos: el enlace no lleva a ningún contacto", "error", err)
		return
	}
	var abrir *api.AddressBook
	fyne.DoAndWait(func() {
		a.pedido = c.ID
		p := a.Pantalla
		switch {
		case p.Activa != nil && p.Activa.ID == c.AddressBookID:
			a.abrirPedido() // ya está abierta: si la lista ha llegado, la ficha sale ya
		case len(p.Libretas) == 0:
			a.libretaPedida = c.AddressBookID // `Cargar` abrirá esta y no la primera
		default:
			for i := range p.Libretas {
				if p.Libretas[i].ID == c.AddressBookID {
					abrir = &p.Libretas[i]
				}
			}
		}
	})
	// ponytail: si el enlace llega con la app arrancando y la respuesta cae entre que
	// llegan las libretas y `Cargar` abre la primera, se cargan las dos y gana la última.
	// Se releva haciendo que `Cargar` espere a la resolución del enlace pendiente.
	if abrir != nil {
		a.abrirLibreta(*abrir)
	}
}

// abrirPedido enseña la ficha pedida por enlace si ya está en la lista. En el hilo de
// la interfaz.
func (a *App) abrirPedido() {
	if a.pedido == "" {
		return
	}
	if c := buscarPorID(a.todos, a.pedido); c != nil {
		a.pedido = ""
		a.elegir(*c)
	}
}
