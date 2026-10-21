// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El catálogo de productos del lanzador: el mismo CATALOGO_ORDENADO y en el mismo
// orden que `packages/ui/src/lib/apps.ts`, que es el del conmutador de la barra.
//
// El nombre va aquí y NO en el catálogo de idiomas porque en la web tampoco se
// traduce: «Sytue Mail» se llama igual en los veintiséis. Lo que sí sale del catálogo
// común es la DESCRIPCIÓN (`app.<clave>.description`).
type Producto struct {
	// Marca es la del catálogo de iconos, que es un conjunto cerrado: un dedazo no
	// compila.
	Marca iconos.Producto
	// Nombre es el del catálogo de la web, con el prefijo «Sytue » incluido. El
	// lanzador lo recorta al pintarlo, como hace `shortName`.
	Nombre string
	// Despliegue es la clave del servicio del que depende: Calcus, Diapos y CertiPDF
	// viven dentro del despliegue de Docus, así que se presentan si ese está vivo.
	// Vacío es «se despliega solo». Es una cadena y no un `iconos.Producto` porque
	// el cero de ese tipo es `One`, y entonces «sin dependencia» y «depende del Hub»
	// serían el mismo valor.
	Despliegue string
}

// Catalogo es el catálogo en orden, `one` incluido —hace falta para nombrar un
// resultado de búsqueda—; quien pinta el lanzador lo excluye.
var Productos = []Producto{
	{Marca: iconos.One, Nombre: "Sytue Hub"},
	{Marca: iconos.Mail, Nombre: "Sytue Mail"},
	{Marca: iconos.Drive, Nombre: "Sytue Drive"},
	{Marca: iconos.Calendar, Nombre: "Sytue Calendar"},
	{Marca: iconos.Contacts, Nombre: "Sytue Contacts"},
	{Marca: iconos.Docs, Nombre: "Sytue Docus"},
	{Marca: iconos.Sheets, Nombre: "Sytue Calcus", Despliegue: "docs"},
	{Marca: iconos.Slides, Nombre: "Sytue Diapos", Despliegue: "docs"},
	{Marca: iconos.PDF, Nombre: "Sytue CertiPDF", Despliegue: "docs"},
	{Marca: iconos.Notes, Nombre: "Sytue Notepads"},
	{Marca: iconos.Photos, Nombre: "Sytue Photos"},
	{Marca: iconos.Rooms, Nombre: "Sytue Rooms"},
	{Marca: iconos.Cast, Nombre: "Sytue Cast"},
	{Marca: iconos.ERP, Nombre: "Sytue ERP"},
	{Marca: iconos.Pages, Nombre: "Sytue Web"},
	{Marca: iconos.Bases, Nombre: "Sytue Bases"},
	{Marca: iconos.Cloud, Nombre: "Sytue DevX"},
	{Marca: iconos.Console, Nombre: "Sytue Admin"},
	{Marca: iconos.ID, Nombre: "Sytue ID"},
}

// porClave indexa el catálogo COMPLETO —no el presentado— para poder nombrar el
// origen de un resultado o de un aviso aunque su producto no esté levantado: una cosa
// es no ofrecer DevX y otra no saber nombrarlo. Es el `appForKey` de la web.
var porClave = func() map[string]Producto {
	m := make(map[string]Producto, len(Productos)+2)
	for _, p := range Productos {
		m[p.Marca.Clave()] = p
	}
	// Chat y Meet son Rooms y el conector del Hub todavía emite esos orígenes. No
	// tienen tesela propia (`marcas.go` lo deja escrito) y aquí resuelven a Rooms. Y un
	// aviso etiquetado `hub` es del propio Hub: el `SOURCE_ALIAS` de `apps.ts`.
	m["chat"] = m["rooms"]
	m["meet"] = m["rooms"]
	m["hub"] = m["one"]
	return m
}()

// ProductoDe resuelve el `source` que manda el Hub. El segundo valor dice si se
// conoce: un origen desconocido se enseña con su propia cadena, no con una marca
// inventada.
func ProductoDe(clave string) (Producto, bool) {
	p, ok := porClave[clave]
	return p, ok
}

// NombreCorto es el `shortName` de la web: el nombre sin el prefijo «Sytue ». Es el
// que llevan las teselas del lanzador y la columna de origen de la actividad.
func (p Producto) NombreCorto() string {
	const prefijo = "Sytue "
	if len(p.Nombre) > len(prefijo) && p.Nombre[:len(prefijo)] == prefijo {
		return p.Nombre[len(prefijo):]
	}
	return p.Nombre
}

// ClaveDescripcion es la clave del catálogo COMÚN con la descripción del producto.
func (p Producto) ClaveDescripcion() string { return "app." + p.Marca.Clave() + ".description" }

// Presentables deja solo lo que ESTA instalación levanta, que es el
// `appsPresentables` de la web: la lista la da el Hub (`GET /api/v1/apps`) y la sabe
// el despliegue, no el cliente. Vacía o nil es «no se sabe», y entonces se presenta la
// suite entera — ofrecer una tesela de más es preferible a esconder media suite porque
// una petición no llegó.
//
// `one` no se filtra nunca: es la puerta de vuelta a casa.
func Presentables(desplegadas []string) []Producto {
	if len(desplegadas) == 0 {
		return Productos
	}
	vivas := make(map[string]bool, len(desplegadas))
	for _, d := range desplegadas {
		vivas[strings.ToLower(d)] = true
	}
	out := make([]Producto, 0, len(Productos))
	for _, p := range Productos {
		clave := p.Marca.Clave()
		if p.Despliegue != "" {
			clave = p.Despliegue
		}
		if p.Marca == iconos.One || vivas[clave] {
			out = append(out, p)
		}
	}
	return out
}

// ---- Abrir otra aplicación ----

// El mecanismo —esquema, respaldo web y la tabla de qué app abre qué fichero— vive en
// `apps/nucleo/api/abrir.go` desde el 2026-09-22: nació aquí para las teselas del
// lanzador y ahora lo usan también Drive, Mail y Rooms, así que subió a un sitio
// compartido (`docs/CONVENTIONS.md` §13.2). Lo que se queda aquí es el CATÁLOGO del
// lanzador, que sí es del Hub.

// EsquemaDe es el esquema de URL con el que se invoca a la aplicación nativa de un
// producto. Se declara en DOS sitios —aquí y el `FyneApp.toml` de su binario— y
// `TestElEsquemaEsElDelFyneApp` los ata.
func EsquemaDe(p Producto) string { return api.Esquema(p.Marca.Clave()) }

// Abridor es lo que hace falta para saltar a otra aplicación. Lo cumple `fyne.App`;
// en los tests es un doble que se limita a apuntar la URL.
type Abridor = api.Abridor

// Abrir invoca a la aplicación de otro producto por su esquema y, si el sistema no
// tiene quién lo atiende —la app no está instalada—, cae a su dirección web. La org es
// la de la sesión de quien abre, para el respaldo (ADR 0056).
func Abrir(a Abridor, apex, org string, p Producto, ruta string) error {
	return api.AbrirEn(a, apex, p.Marca.Clave(), org, ruta)
}

// AbridorDeLaApp es el `fyne.App` en marcha, que es quien sabe delegar en el sistema
// en las cinco plataformas.
func AbridorDeLaApp() Abridor { return fyne.CurrentApp() }
