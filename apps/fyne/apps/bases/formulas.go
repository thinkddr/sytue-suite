// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Campos de fórmula, del lado de la interfaz (espejo de `lib/formulas.ts`). La fórmula la
// valida y la CALCULA el servicio; aquí solo se traduce lo que escribe una persona
// —`{Precio} * {Cantidad}`, `SUM({Líneas.Importe})`— a lo que se guarda, que referencia
// los campos por id para que renombrar un campo no rompa nada.

// sustituir cambia cada `{…}` que esté FUERA de un texto entre comillas; `ok` falso si
// alguna no se resuelve.
func sustituir(expr string, ref func(dentro string) (string, bool)) (string, bool) {
	var out strings.Builder
	enTexto := false
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if c == '"' {
			enTexto = !enTexto
		}
		if c != '{' || enTexto {
			out.WriteByte(c)
			continue
		}
		fin := strings.IndexByte(expr[i:], '}')
		if fin < 0 {
			return "", false
		}
		r, ok := ref(expr[i+1 : i+fin])
		if !ok {
			return "", false
		}
		out.WriteString("{" + r + "}")
		i += fin
	}
	return out.String(), true
}

func campoPorNombre(campos []api.BasesField, nombre string) (api.BasesField, bool) {
	for _, c := range campos {
		if strings.EqualFold(strings.TrimSpace(c.Name), strings.TrimSpace(nombre)) {
			return c, true
		}
	}
	return api.BasesField{}, false
}

func camposDeTabla(tablas []api.BasesTable, id string) []api.BasesField {
	for _, t := range tablas {
		if t.ID == id {
			return t.Fields
		}
	}
	return nil
}

// formulaAIds pasa de nombres a ids: `{Campo}` es un campo de esta tabla y
// `{Enlace.Campo}`, un campo de la tabla a la que apunta ese enlace. `ok` falso si algún
// nombre no existe.
func formulaAIds(expr string, propios []api.BasesField, tablas []api.BasesTable) (string, bool) {
	return sustituir(expr, func(dentro string) (string, bool) {
		// Un nombre de campo puede llevar punto: primero el nombre entero.
		if propio, ok := campoPorNombre(propios, dentro); ok {
			return propio.ID, true
		}
		for p := strings.IndexByte(dentro, '.'); p >= 0; {
			enlace, ok := campoPorNombre(propios, dentro[:p])
			if ok && enlace.Kind == api.BasesKindLink {
				if campo, ok := campoPorNombre(camposDeTabla(tablas, enlace.LinkTableID), dentro[p+1:]); ok {
					return enlace.ID + "." + campo.ID, true
				}
			}
			sig := strings.IndexByte(dentro[p+1:], '.')
			if sig < 0 {
				break
			}
			p += 1 + sig
		}
		return "", false
	})
}

// referenciasDeFormula son las referencias que se pueden escribir en una fórmula de esta
// tabla, ya entre llaves.
func referenciasDeFormula(propios []api.BasesField, tablas []api.BasesTable) []string {
	var out []string
	for _, c := range propios {
		out = append(out, "{"+c.Name+"}")
		if c.Kind != api.BasesKindLink {
			continue
		}
		for _, d := range camposDeTabla(tablas, c.LinkTableID) {
			switch efectivo(d).Kind {
			case api.BasesKindNumber, api.BasesKindDate, api.BasesKindCheckbox:
				out = append(out, "{"+c.Name+"."+d.Name+"}")
			}
		}
	}
	return out
}
