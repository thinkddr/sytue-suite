// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Ordenar por apellido» (`client/orden.ts`)
//
// La libreta en el orden de la guía de teléfonos, con las cabeceras A–Z por la inicial
// del apellido: quien guarda «Ana García» y «Luis García» los busca juntos, en la G. Un
// contacto sin apellido (una empresa, un «Fontanero») se ordena por su nombre, como en
// Contactos de Apple. Es de quien mira: va al almacén por persona, como en la web va a
// su `localStorage`.

// ClaveOrden es la clave del almacén por persona; «apellido» o nada.
const ClaveOrden = "sytue.contacts.orden"

// Almacen es donde se guarda; lo cumple `sesion.PorPersona`. Sin él, por nombre.
type Almacen interface {
	Leer(clave string) string
	Poner(clave, valor string)
}

// ClaveDeOrden es `claveDeOrden`: el apellido si se ordena por él y lo hay; si no, el
// nombre visible.
func ClaveDeOrden(c api.Contact, porApellido bool) string {
	if a := strings.TrimSpace(c.FamilyName); porApellido && a != "" {
		return a
	}
	return c.DisplayName()
}

// ordenar deja la libreta en el orden del idioma por su clave y, a igualdad, por el
// nombre visible.
func ordenar(lista []api.Contact, l i18n.Locale, porApellido bool) {
	c := cotejador(l)
	slices.SortStableFunc(lista, func(x, y api.Contact) int {
		if n := c.CompareString(ClaveDeOrden(x, porApellido), ClaveDeOrden(y, porApellido)); n != 0 {
			return n
		}
		return c.CompareString(x.DisplayName(), y.DisplayName())
	})
}

// clavesDeOrden son las claves de agrupar por inicial, en el orden de la lista.
func clavesDeOrden(lista []api.Contact, porApellido bool) []string {
	claves := make([]string, len(lista))
	for i, c := range lista {
		claves[i] = ClaveDeOrden(c, porApellido)
	}
	return claves
}

// restaurarOrden lee el de quien acaba de entrar.
func (a *App) restaurarOrden() {
	a.Pantalla.PorApellido = a.Almacen != nil && a.Almacen.Leer(ClaveOrden) == "apellido"
}

// cambiarOrden es «Ordenar por apellido» / «Ordenar por nombre»: lo guarda, reordena la
// libreta cargada y repinta.
func (a *App) cambiarOrden() {
	p := a.Pantalla
	p.PorApellido = !p.PorApellido
	if a.Almacen != nil {
		valor := ""
		if p.PorApellido {
			valor = "apellido"
		}
		a.Almacen.Poner(ClaveOrden, valor)
	}
	ordenar(a.todos, p.locale(), p.PorApellido)
	a.repintar()
}
