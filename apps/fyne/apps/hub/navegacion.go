// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"net/url"
	"strings"
	"time"
)

// Destino es una pantalla de ESTA aplicación: las ocho del portal y el asistente
// secundario de clientes externos. Saltar a otro PRODUCTO no es un destino, es una acción del
// sistema (ver `productos.go`).
type Destino uint8

// Pantallas del portal y asistente secundario, sin cambiar el menú principal.
const (
	AInicio           Destino = iota // /one
	AActividad                       // /one/actividad
	ABuscar                          // /one/buscar
	AAvisos                          // /one/notificaciones
	ADatos                           // /one/datos
	ADescargas                       // /one/descargas
	ADispositivos                    // /one/dispositivos
	ABienvenida                      // /one/bienvenida
	AClientesExternos                // /one/dispositivos/clientes
)

// Ruta es el camino que esa pantalla tiene en la web. Se conserva porque es lo que
// llega por el esquema de URL cuando otra aplicación llama al Hub
// (`sytue-one:/actividad`), y porque es la clave con la que el gate de paridad casa
// pantalla nativa y pantalla de web.
func (d Destino) Ruta() string {
	return [...]string{
		AInicio: "/", AActividad: "/actividad", ABuscar: "/buscar", AAvisos: "/notificaciones",
		ADatos: "/datos", ADescargas: "/descargas", ADispositivos: "/dispositivos",
		ABienvenida: "/bienvenida", AClientesExternos: "/dispositivos/clientes",
	}[d]
}

// ClaveTitulo es la clave de catálogo con el título de la pantalla (`page.*`), que es
// lo que la ventana pone en su barra.
func (d Destino) ClaveTitulo() string {
	return [...]string{
		AInicio: "page.home", AActividad: "page.activity", ABuscar: "page.search",
		AAvisos: "page.inbox", ADatos: "page.data", ADescargas: "page.downloads",
		ADispositivos: "page.devices", ABienvenida: "page.welcome", AClientesExternos: "page.devices",
	}[d]
}

// DestinoDeRuta es la pantalla que pide un enlace entrante (`sytue-one:/notificaciones`,
// que es lo que lanza «Ver todas» desde el panel de avisos de cualquier otra app).
//
// Acepta la ruta con y sin el prefijo `/one` de la web, porque `api.AbrirURL` arma el
// esquema con el camino ENTERO de la dirección de un aviso (`/one/actividad`).
func DestinoDeRuta(ruta string) (Destino, bool) {
	ruta = strings.TrimSuffix(strings.TrimPrefix(ruta, "/one"), "/")
	if ruta == "" {
		ruta = "/"
	}
	for d := AInicio; d <= AClientesExternos; d++ {
		if d.Ruta() == ruta {
			return d, true
		}
	}
	return AInicio, false
}

// RutaAgenda es la ruta de la fila de un evento de la agenda de hoy en el widget nativo
// de One (fuentes.RutaAgenda): `sytue-one:/agenda?date=AAAA-MM-DD`, con `&all_day=1` en
// uno de todo el día. No es una pantalla de One: la agenda es de Calendar.
const RutaAgenda = "/agenda"

// DiaDeAgenda es la ruta de Calendar que abre un enlace `sytue-one:/agenda?date=…`: la
// vista de día en esa fecha (`/?date=AAAA-MM-DD`, y `&all_day=1`), la misma que abre la
// fila de la tarjeta «Hoy» (RutaDelEvento) y que Calendar atiende
// (`calendario.FechaDelEnlace`). Se vuelve a escribir con la fecha leída, nunca se copia
// la consulta: un enlace con otra cosa —otra ruta, otros parámetros, una fecha que no es
// una fecha— no lleva a ningún sitio.
func DiaDeAgenda(u *url.URL) (string, bool) {
	if u == nil || u.Host != "" || u.User != nil || u.Opaque != "" || u.Fragment != "" ||
		strings.TrimSuffix(strings.TrimPrefix(u.Path, "/one"), "/") != RutaAgenda {
		return "", false
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["date"]) != 1 || len(q) > 2 ||
		(len(q) == 2 && (len(q["all_day"]) != 1 || q.Get("all_day") != "1")) {
		return "", false
	}
	dia, err := time.Parse(time.DateOnly, q.Get("date"))
	if err != nil {
		return "", false
	}
	destino := url.Values{"date": {dia.Format(time.DateOnly)}}
	if q.Has("all_day") {
		destino.Set("all_day", "1")
	}
	return "/?" + destino.Encode(), true
}
