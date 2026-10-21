// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"image/color"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Los glifos PROPIOS de Sytue Contacts.
//
// No están en `apps/fyne/iconos` porque en la web tampoco están en `packages/ui`: la
// estrella y la tarta van escritas a mano dentro de `ContactsApp.tsx` y de
// `ContactDetail.tsx`, con el comentario «@sytue/ui no expone glifo star». El catálogo
// compartido es el espejo EXACTO de `Icon.tsx` y su guardián compara en los dos
// sentidos: meter aquí dos constantes que allí no existen lo pondría en rojo, y con
// razón.
//
// Las rutas son verbatim de esos dos ficheros, con la misma rejilla de 24 y el mismo
// trazo de 2 que el catálogo. La etiqueta y la papelera de esta app también son
// inline en la web y NO coinciden con las del catálogo (`tag` y `trash` de Icon.tsx
// dibujan otra cosa), así que van igualmente aquí: es variación deliberada de la web,
// no duplicación nuestra.
const (
	// EstrellaD es el `StarGlyph`. Relleno cuando el contacto está destacado y solo
	// trazo cuando no; la ruta es la misma y lo que cambia es el `fill`.
	EstrellaD = "M12 2.5l2.9 5.9 6.5.95-4.7 4.58 1.11 6.47L12 17.9l-5.81 3.06 1.11-6.47-4.7-4.58 6.5-.95L12 2.5z"
	// TartaD es el `CakeGlyph` de «Próximos cumpleaños». Son dos subrutas, como en la
	// web: la tarta con su glaseado y la vela.
	TartaD = "M4 20h16M4 20v-7a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v7M4 15.5c1.5 1.5 3 1.5 4.5 0s3-1.5 4.5 0 3 1.5 4.5 0 2-1 2.5-.5 M12 11V8M12 5.5a1 1 0 1 0 0-.01"
	// EtiquetaD es el `TagGlyph` de esta app, que NO es el `tag` del catálogo.
	EtiquetaD = "M20.6 13.4 12 22l-9-9V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z M7.5 7.5h.01"
	// PapeleraD es el `TrashGlyph` de esta app: sus cuatro `<path>` en una sola ruta,
	// que con trazo y sin relleno pinta lo mismo. Hasta el 2026-09-22 aquí había OTRA
	// papelera —tapa en y = 7, asa cuadrada— que no era la de ContactsApp.tsx.
	PapeleraD = "M3 6h18 M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2 M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6 M10 11v6M14 11v6"
)

// Glifo pinta uno de los de arriba al lado y con el color pedidos.
//
// Usa `iconos.TrazoDeRuta`, que es la AMPLIACIÓN mínima que el catálogo necesitaba
// para esto: el `Dibujo` —con su rasterizado a píxeles de DISPOSITIVO y la
// normalización de `oksvg`— ya estaba, y lo único que faltaba era poder pedirlo con
// una ruta en vez de con una constante del catálogo. Así cada app guarda sus glifos en
// su paquete y el catálogo compartido sigue siendo el espejo exacto de `Icon.tsx`.
func Glifo(d string, lado float32, token string, v tema.Variante) *iconos.Dibujo {
	return iconos.TrazoDeRuta(d, lado, tema.Color(token, v)).DeToken(token)
}

// GlifoRelleno es la estrella de un contacto destacado: la misma ruta, RELLENA además
// de trazada (`fill={filled ? 'currentColor' : 'none'}`).
func GlifoRelleno(d string, lado float32, c color.NRGBA) *iconos.Dibujo {
	return iconos.TrazoDeRutaRelleno(d, lado, c)
}
