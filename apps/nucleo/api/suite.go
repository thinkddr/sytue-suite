// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"maps"
	"slices"
)

// Apex por defecto de la suite. En desarrollo es `sytue.localhost`.
const ApexProd = "sytue.eu"

// hostDe es el host del SERVICIO de cada producto, el que da su API (`ServiceURL`). Casi
// siempre es el de `appDomains` de packages/config/src/tokens.ts; las excepciones, abajo y
// en `webDe`. Los que comparten host lo comparten aquí también: Hojas, Diapos y PDF son el
// servicio de Sytue Docs; chat y meet son Rooms; la consola es `admin`.
var hostDe = map[string]string{
	"one": "one", "mail": "mail", "drive": "drive", "calendar": "calendar",
	"contacts": "contacts", "docs": "docs", "sheets": "docs", "slides": "docs", "pdf": "docs",
	"notes": "notes", "chat": "rooms", "meet": "rooms", "rooms": "rooms",
	// El servicio de fotos es `syphotos.` (deploy/helm/charts/syphotos, `subdomain:
	// syphotos`): `photos.` es la web y su BFF, que al token de la app nativa le contesta
	// 401. Con `photos.` la app de Photos decía «No tienes acceso» a quien la web sí deja
	// entrar (medido contra producción el 2026-09-27: 401 en photos., 200 en syphotos.).
	"photos": "syphotos",
	"cloud":  "cloud", "console": "admin", "hub": "hub", "erp": "erp", "cast": "cast", "id": "id",
	// Sytue Web: la clave del producto es `pages`; su host, `web.` (2026-10-04).
	"pages": "web", "bases": "bases",
	// El gateway de integraciones no es un producto —no tiene tesela—, pero sí host.
	"integrations": "integrations",
	// Sytue AI tampoco: la web la llama desde su BFF por dentro del clúster, y el
	// ingress `ai.` existe solo para las apps nativas (deploy/helm/charts/ai).
	"ai": "ai",
}

// EsProducto dice si `producto` es una clave de producto de la suite. Es la pregunta que
// hay que hacer ANTES de [ServiceURL] con una clave que viene de fuera —un enlace, un
// aviso—: ServiceURL entra en pánico con una que no existe, porque para él eso es un
// error de quien escribe la vista, y con un dato de fuera el pánico lo elige quien manda
// el enlace.
func EsProducto(producto string) bool {
	_, ok := hostDe[producto]
	return ok
}

// Productos son las claves de producto de la suite, en orden alfabético.
func Productos() []string { return slices.Sorted(maps.Keys(hostDe)) }

// ServiceURL es la raíz del servicio de un producto en ese apex: "https://notes.sytue.eu".
// Las apps nativas hablan con el servicio EN DIRECTO y con el token de la persona, no a
// través del BFF de la web. Un producto que no existe entra en pánico: es un error de
// quien escribe la vista.
func ServiceURL(apex, producto string) string {
	host, ok := hostDe[producto]
	if !ok {
		panic("api: no existe el producto " + producto)
	}
	return "https://" + host + "." + apex
}
