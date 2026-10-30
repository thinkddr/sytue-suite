// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import "strings"

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
