// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import "git.sytue.eu/sytue/one/apps/nucleo/api"

// Carga por ventanas de la rejilla, espejo de `lib/ventanas.ts`. El servicio pagina por
// cursor (sin techo de registros), así que la interfaz no puede acumular la tabla entera:
// guarda como mucho maxVentanas páginas consecutivas y recuerda el cursor con el que
// pidió cada una, que es lo que permite volver atrás sin que el servicio sepa paginar
// hacia atrás.

const (
	maxVentanas = 3
	// pagina es el tamaño de página que se pide (`PAGINA` de lib/limites.ts).
	pagina = 100
	// maxOpcionesEnlace es cuántos registros de la tabla destino se ofrecen en un enlace.
	maxOpcionesEnlace = 500
)

// ventanas son las páginas en memoria.
type ventanas struct {
	// paginas son consecutivas; primera es el índice (desde 0) de la primera.
	paginas [][]api.BasesRecord
	primera int
	// cursores[i] es el cursor con el que se pide la página i ("" la primera).
	cursores []string
	// siguiente es el cursor de la página que sigue a la última; "" = no hay más.
	siguiente string
}

func sinVentanas() ventanas { return ventanas{cursores: []string{""}} }

// empezar es la primera página de una tabla o vista.
func empezar(p api.BasesRecordPage) ventanas {
	return ventanas{paginas: [][]api.BasesRecord{p.Records}, cursores: []string{""}, siguiente: p.NextCursor}
}

// avanzar añade la página pedida con `v.siguiente`; si ya hay maxVentanas, suelta la primera.
func avanzar(v ventanas, p api.BasesRecordPage) ventanas {
	indice := v.primera + len(v.paginas)
	cursores := append([]string{}, v.cursores...)
	for len(cursores) <= indice {
		cursores = append(cursores, "")
	}
	cursores[indice] = v.siguiente
	paginas := append(append([][]api.BasesRecord{}, v.paginas...), p.Records)
	primera := v.primera
	if len(paginas) > maxVentanas {
		paginas, primera = paginas[1:], primera+1
	}
	return ventanas{paginas: paginas, primera: primera, cursores: cursores, siguiente: p.NextCursor}
}

// cursorAnterior es el cursor con el que se pide la página anterior a la primera en
// memoria; `ok` falso si no hay anterior.
func cursorAnterior(v ventanas) (cursor string, ok bool) {
	if v.primera == 0 || v.primera-1 >= len(v.cursores) {
		return "", false
	}
	return v.cursores[v.primera-1], true
}

// retroceder pone delante la página pedida con cursorAnterior; si sobra una, suelta la última.
func retroceder(v ventanas, p api.BasesRecordPage) ventanas {
	if v.primera == 0 {
		return v
	}
	paginas := append([][]api.BasesRecord{p.Records}, v.paginas...)
	primera := v.primera - 1
	if len(paginas) <= maxVentanas {
		v.paginas, v.primera = paginas, primera
		return v
	}
	// La que se suelta se volverá a pedir con el cursor con el que se pidió.
	siguiente := ""
	if i := primera + maxVentanas; i < len(v.cursores) {
		siguiente = v.cursores[i]
	}
	return ventanas{paginas: paginas[:maxVentanas], primera: primera, cursores: v.cursores, siguiente: siguiente}
}

// registros son todos los que hay en memoria, en orden.
func (v ventanas) registros() []api.BasesRecord {
	var out []api.BasesRecord
	for _, p := range v.paginas {
		out = append(out, p...)
	}
	return out
}

// editar aplica un cambio local (editar, borrar) a los registros de cada página.
func (v *ventanas) editar(f func(api.BasesRecord) (api.BasesRecord, bool)) {
	for i, p := range v.paginas {
		nueva := make([]api.BasesRecord, 0, len(p))
		for _, r := range p {
			if r, queda := f(r); queda {
				nueva = append(nueva, r)
			}
		}
		v.paginas[i] = nueva
	}
}

// anadir enseña un registro recién creado al final de lo que hay en pantalla.
func (v *ventanas) anadir(r api.BasesRecord) {
	if len(v.paginas) == 0 {
		v.paginas = [][]api.BasesRecord{nil}
	}
	u := len(v.paginas) - 1
	v.paginas[u] = append(append([]api.BasesRecord{}, v.paginas[u]...), r)
}
