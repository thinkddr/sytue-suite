// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Valores de celda: de lo que viaja (JSON) a lo que se escribe en la rejilla y de vuelta.
// Es el espejo de `lib/valores.ts` y, como allí, COMODIDAD de la interfaz: el servicio
// vuelve a validar todo y es el que manda.

// puedeEscribir dice si el rol deja escribir registros (editor o propietario).
func puedeEscribir(rol string) bool { return rol == api.BasesOwner || rol == api.BasesEditor }

// textoDeValor es el valor de una celda como texto para mostrar o para editar.
func textoDeValor(valor any) string {
	switch v := valor.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "✓"
		}
	case []any:
		partes := make([]string, len(v))
		for i, x := range v {
			partes[i] = textoDeValor(x)
		}
		return strings.Join(partes, ", ")
	case []string:
		return strings.Join(v, ", ")
	}
	return ""
}

// valorDeTexto es lo escrito en un campo → el valor a enviar. nil es vaciar. `ok` falso:
// no vale para ese tipo (la interfaz avisa y no envía).
func valorDeTexto(campo api.BasesField, texto string) (valor any, ok bool) {
	t := strings.TrimSpace(texto)
	if t == "" {
		return nil, true
	}
	switch campo.Kind {
	case api.BasesKindText:
		return texto, true
	case api.BasesKindNumber:
		// Admite la coma decimal, que es como se escribe en gran parte de la UE.
		n, err := strconv.ParseFloat(strings.Replace(t, ",", ".", 1), 64)
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) || math.Abs(n) > 1e15 {
			return nil, false
		}
		return n, true
	case api.BasesKindDate:
		if _, err := time.Parse("2006-01-02", t); err != nil {
			return nil, false
		}
		return t, true
	case api.BasesKindSelect:
		return t, slices.Contains(campo.Choices, t)
	}
	return nil, false
}

// etiquetaDeRegistro es cómo se llama un registro en una lista de enlaces: su primer
// campo con valor.
func etiquetaDeRegistro(campos []api.BasesField, r api.BasesRecord) string {
	for _, c := range campos {
		if c.Kind == api.BasesKindLink || c.Kind == api.BasesKindCheckbox || c.Kind == api.BasesKindAttachment {
			continue
		}
		if t := textoDeValor(r.Data[c.ID]); t != "" {
			return t
		}
	}
	return r.ID[:min(len(r.ID), 8)]
}

// cadenasDe son los valores de una celda de selección múltiple, o nada.
func cadenasDe(valor any) []string {
	lista, _ := valor.([]any)
	out := make([]string, 0, len(lista))
	for _, x := range lista {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// efectivo es el campo tal como se filtra, se ordena y se pinta: una fórmula se comporta
// como el tipo de su resultado (`lib/formulas.ts`).
func efectivo(c api.BasesField) api.BasesField {
	if c.Kind != api.BasesKindFormula {
		return c
	}
	c.Kind = c.Result
	if c.Kind == "" {
		c.Kind = api.BasesKindNumber
	}
	return c
}
