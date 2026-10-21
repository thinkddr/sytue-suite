// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

// paridad-web: apps/frontend/packages/ui/src/components/ProductIcon.tsx

import (
	"embed"
	"fmt"
	"image/color"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// marcas/ es la copia que deja `scripts/sync-branding.sh` de `branding/marks`, que es
// la fuente de verdad y la misma que usa la web. La copia existe porque `go:embed` no
// sale de su módulo; `TestLasMarcasSonLasDeBranding` compara byte a byte.
//
// Sin las `-maskable`: esas son para el lanzador del sistema, no para la interfaz.
//
//go:embed marcas/*.svg
var marcasFS embed.FS

// Producto es un producto de la suite con marca propia. Conjunto CERRADO, igual que
// Icono: `iconos.Mail` existe o no compila.
//
// Son veinte y no veintidós: `chat` y `meet` no tienen marca, y no es un olvido.
// Son hosts legados que solo devuelven un 301 al canónico, así que la web tampoco se
// la da (`packages/ui/src/assets/marks.ts` lo deja escrito). Quien necesite pintarlos
// usa el icono de trazo del destino.
type Producto uint8

// En el orden de `productColor` de `packages/config/src/tokens.ts`, saltando chat y meet.
const (
	One Producto = iota
	ID
	Drive
	Docs
	Sheets
	Slides
	PDF
	Mail
	Calendar
	Contacts
	Notes
	Photos
	Rooms
	Cloud
	Console
	Hub
	ERP
	Cast
	Pages
	Bases
)

// Clave es el `ProductKey` de la web, que es también el nombre del fichero en
// `branding/marks/` y el sufijo del token `--sytue-product-<clave>`.
func (p Producto) Clave() string { return claves[p] }

var claves = [...]string{
	One: "one", ID: "id", Drive: "drive", Docs: "docs", Sheets: "sheets",
	Slides: "slides", PDF: "pdf", Mail: "mail", Calendar: "calendar",
	Contacts: "contacts", Notes: "notes", Photos: "photos", Rooms: "rooms",
	Cloud: "cloud", Console: "console", Hub: "hub", ERP: "erp", Cast: "cast",
	Pages: "pages", Bases: "bases",
}

// Nombre es la marca comercial, con su prefijo: «Sytue Mail».
//
// Está aquí, junto a la clave y al dibujo, porque es lo mismo: los tres identifican al
// producto y los tres son un conjunto cerrado de diecinueve. Vivía en la tabla del
// lanzador de Sytue Hub, y al necesitarlo también la barra de la suite —que vive en
// `ui`, que no puede importar `hub`— habría hecho falta una segunda copia. Ahora la
// tabla del lanzador lo pide aquí.
//
// No se traduce y no tiene clave de catálogo: «Sytue Mail» se llama igual en los
// veintiséis idiomas. Lo que sí sale del catálogo es la descripción
// (`app.<clave>.description`).
func (p Producto) Nombre() string { return nombres[p] }

// NombreCorto es la marca SIN el prefijo de la suite: «Mail». Es el `shortName` de la
// web, que es como se nombra un producto cuando el contexto ya es la suite —la
// cabecera de un grupo de resultados de búsqueda, la tesela del lanzador—.
func (p Producto) NombreCorto() string { return strings.TrimPrefix(nombres[p], "Sytue ") }

var nombres = [...]string{
	One: "Sytue Hub", ID: "Sytue ID", Drive: "Sytue Drive", Docs: "Sytue Docus",
	Sheets: "Sytue Calcus", Slides: "Sytue Diapos", PDF: "Sytue CertiPDF",
	Mail: "Sytue Mail", Calendar: "Sytue Calendar", Contacts: "Sytue Contacts",
	Notes: "Sytue Notepads", Photos: "Sytue Photos", Rooms: "Sytue Rooms",
	Cloud: "Sytue DevX", Console: "Sytue Admin", Hub: "Sytue Hub", ERP: "Sytue ERP",
	Cast: "Sytue Cast", Pages: "Sytue Web", Bases: "Sytue Bases",
}

// ProductoDe es la marca cuya clave es esa, que es lo que hace falta para nombrar el
// origen de un resultado de búsqueda o de un aviso: el Hub los etiqueta con la clave
// (`mail`, `drive`), no con la marca.
//
// `chat` y `meet` no tienen marca propia —son hosts legados con un 301 al canónico—,
// pero el Hub sigue etiquetando con `chat` los resultados de Rooms: son Rooms, como en
// la web (`SOURCE_ALIAS` de apps.ts). Sin esto el grupo salía como «CHAT», sin marca.
// `false` para una clave que nadie conoce, y entonces quien llama enseña lo que pueda:
// una clave sin dibujo es mejor que un dibujo equivocado.
func ProductoDe(clave string) (Producto, bool) {
	if clave == "chat" || clave == "meet" {
		return Rooms, true
	}
	for i, c := range claves {
		if c == clave {
			return Producto(i), true //nolint:gosec // i viene del propio array de claves
		}
	}
	return One, false
}

// Los tamaños de la MARCA, que no son los del icono de trazo: una marca es la tesela
// de color con el glifo dentro y la web la usa en cuatro sitios. Medidos sobre sus
// usos, igual que las tallas de icono.
const (
	MarcaXS float32 = 20 // dentro de una línea de texto: «17 correos sin leer»
	MarcaSM float32 = 24 // junto a un rótulo: fila de lista, conmutador de la barra
	MarcaMD float32 = 28 // el producto abierto, en la barra superior
	MarcaLG float32 = 40 // la tesela del lanzador de Sytue Hub (`Launcher.tsx`)
)

// Marca pinta la marca de un producto: la tesela de color con su glifo y, en tema
// oscuro, el anillo que le devuelve el contorno.
//
// El anillo es el `box-shadow: 0 0 0 1px var(--sytue-product-tile-ring)` de
// `ProductIcon.tsx`, con su misma geometría: la tesela no llena el lienzo —va de 4 a
// 36 sobre 40, o sea el 80 % centrado, con radio 9 (22,5 %)—, así que el anillo se
// ciñe a la tesela y no al borde del dibujo. En claro el token es transparente y no se
// pinta nada; en oscuro se enciende, porque sobre `#22262b` diez de los once productos
// quedan por debajo del 3:1.
//
// Va DENTRO del SVG y no como rectángulo aparte por lo mismo que `ui.Caja` no usa
// `StrokeWidth`: hace falta un anillo de un píxel que no reparta su cobertura entre
// dos filas. Aquí el trazo lo pinta `rasterx` sobre el mismo mapa de bits, con el
// grosor ya escalado a píxeles de dispositivo, así que sale entero.
func Marca(p Producto, lado float32, v tema.Variante) *Dibujo {
	datos, err := marcasFS.ReadFile("marcas/" + claves[p] + ".svg")
	if err != nil {
		panic("iconos: falta la marca de " + claves[p] + ": " + err.Error())
	}
	anillo := tema.Color("product-tile-ring", v)
	return nuevoDibujo(lado, anillo, func(p pintura) []byte {
		return conAnillo(datos, p.tinta)
	})
}

// conAnillo mete el contorno de la tesela en el SVG de la marca, justo antes de
// cerrarlo. Con alfa 0 no se añade nada: en tema claro la marca cuesta lo mismo que
// antes.
//
// Las cuentas son las de `ProductIcon.tsx` sobre el viewBox de 40 de la marca:
// `inset: size*0.1` son 4, `border-radius: size*0.225` son 9. Y medio píxel de
// corrección en cada dirección porque un `box-shadow` con extensión de 1 se pinta
// FUERA de la caja, mientras que un trazo de SVG va centrado en ella.
func conAnillo(marca []byte, c color.NRGBA) []byte {
	if c.A == 0 {
		return marca
	}
	const inset, radio = 4.0, 9.0
	fin := len(marca)
	for fin > 0 && marca[fin-1] != '<' {
		fin--
	}
	anillo := fmt.Sprintf(
		`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" ry="%.1f" fill="none"`+
			` stroke="#%02x%02x%02x" stroke-opacity="%.4f" stroke-width="1"/>`,
		inset-0.5, inset-0.5, 40-2*inset+1, 40-2*inset+1, radio+0.5, radio+0.5,
		c.R, c.G, c.B, float32(c.A)/255)
	out := make([]byte, 0, len(marca)+len(anillo))
	out = append(out, marca[:fin-1]...)
	out = append(out, anillo...)
	return append(out, marca[fin-1:]...)
}
