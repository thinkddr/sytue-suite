// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// Los ajustes de TABLA del panel «Ajustes» (`SeccionTabla` de `panels/SettingsPanel.tsx`):
// bordes, cabecera alternada y ancho automático. Escriben el mismo HTML que la web —los
// estilos en línea como los serializa el navegador— para que un documento tocado aquí se
// relea igual allí.

// Colores de CONTENIDO de la cabecera y de las bandas (`FONDO_CABECERA` y `FONDO_BANDA`
// de la web, #E2E8F0 y #F1F5F9): viajan en el HTML, como el navegador los escribe.
const (
	fondoCabecera = "rgb(226, 232, 240)"
	fondoBanda    = "rgb(241, 245, 249)"
)

// marcaCabecera es el atributo con el que la web relee «Cabecera alternada».
const marcaCabecera = "data-cabecera-alternada"

// declaracion devuelve el valor de una propiedad del `style` de n, o "".
func declaracion(n *html.Node, prop string) string {
	for _, d := range declaraciones(atributo(n, "style")) {
		if strings.EqualFold(d[0], prop) {
			return strings.ToLower(d[1])
		}
	}
	return ""
}

// Bordes dice si las celdas llevan raya: la apaga un `border: none` en la primera.
func (t *Tabla) Bordes() bool {
	if t == nil || len(t.Celdas) == 0 {
		return true
	}
	c := t.Celdas[0].nodo
	return declaracion(c, "border") != "none" && declaracion(c, "border-style") != "none" &&
		declaracion(c, "border-top-style") != "none"
}

// PonerBordes pone o quita el `border: none` de todas las celdas (`aplicarBordes`).
func (t *Tabla) PonerBordes(visibles bool) {
	v := "none"
	if visibles {
		v = ""
	}
	for _, c := range t.Celdas {
		ponerDeclaracion(c.nodo, "border", v)
	}
}

// CabeceraAlternada dice si la tabla lleva la marca de «Cabecera alternada».
func (t *Tabla) CabeceraAlternada() bool { return t != nil && tieneAtributo(t.nodo, marcaCabecera) }

// PonerCabeceraAlternada destaca la primera fila (negrita y fondo) y pone bandas en las
// pares del resto, o lo quita todo (`aplicarCabecera`).
func (t *Tabla) PonerCabeceraAlternada(activa bool) {
	for i, fila := range filasDe(t.nodo) {
		fondo := ""
		switch {
		case !activa:
		case i == 0:
			fondo = fondoCabecera
		case i%2 == 0:
			fondo = fondoBanda
		}
		ponerDeclaracion(fila, "background-color", fondo)
		if i != 0 {
			continue
		}
		peso := ""
		if activa {
			peso = "600"
		}
		for c := fila.FirstChild; c != nil; c = c.NextSibling {
			if esCelda(c) {
				ponerDeclaracion(c, "font-weight", peso)
			}
		}
	}
	i := slices.IndexFunc(t.nodo.Attr, func(a html.Attribute) bool { return a.Namespace == "" && a.Key == marcaCabecera })
	switch {
	case activa && i < 0:
		t.nodo.Attr = append(t.nodo.Attr, html.Attribute{Key: marcaCabecera})
	case !activa && i >= 0:
		t.nodo.Attr = slices.Delete(t.nodo.Attr, i, i+1)
	}
}

// AnchoAuto dice si las columnas se reparten por su contenido (`table-layout: auto`).
func (t *Tabla) AnchoAuto() bool { return t != nil && declaracion(t.nodo, "table-layout") == "auto" }

// PonerAnchoAuto anula o devuelve el `table-layout: fixed` de la hoja (`aplicarAnchoAuto`).
// ponytail: aquí las columnas siguen a partes iguales (ver maquetarTabla); el HTML es el
// de la web, que sí las reparte por su contenido.
func (t *Tabla) PonerAnchoAuto(auto bool) {
	v := ""
	if auto {
		v = "auto"
	}
	ponerDeclaracion(t.nodo, "table-layout", v)
	ponerDeclaracion(t.nodo, "width", v)
}
