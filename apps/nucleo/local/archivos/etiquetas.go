// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// # Etiquetas libres (DR-09)
//
// Sin organización no hay taxonomía de sensibilidad; en su hueco, la persona pone a sus
// ficheros y carpetas las etiquetas que quiera («facturas», «viaje 2026»), varias por
// elemento. Van en el índice de la raíz (`.sytue/indice.json`, legible), por ruta, y
// siguen al elemento cuando se renombra o se mueve desde aquí; tirarlo a la papelera se las
// lleva con él (vuelven al restaurarlo), y borrarlo del todo las quita. La búsqueda por nombre
// encuentra también lo que lleva una etiqueta que coincide.

// MaxEtiqueta es el largo máximo de una etiqueta, en caracteres.
const MaxEtiqueta = 40

// limpiarEtiquetas recorta, quita las vacías y las repetidas (sin distinguir mayúsculas)
// y las ordena.
func limpiarEtiquetas(ls []string) []string {
	var out []string
	for _, l := range ls {
		l = strings.Join(strings.Fields(l), " ")
		if l == "" {
			continue
		}
		if utf8.RuneCountInString(l) > MaxEtiqueta {
			l = string([]rune(l)[:MaxEtiqueta])
		}
		if !slices.ContainsFunc(out, func(x string) bool { return strings.EqualFold(x, l) }) {
			out = append(out, l)
		}
	}
	slices.SortFunc(out, func(x, y string) int { return cmp.Compare(strings.ToLower(x), strings.ToLower(y)) })
	return out
}

// Etiquetas son las de un elemento (fichero o carpeta), en orden.
func (a *Archivos) Etiquetas(_ context.Context, id string) ([]string, error) {
	r, err := a.ruta(id)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.ind.Etiquetas[r]), nil
}

// PonerEtiquetas sustituye las etiquetas de un elemento; vacías, se las quita.
func (a *Archivos) PonerEtiquetas(_ context.Context, id string, etiquetas []string) error {
	r, err := a.ruta(id)
	if err != nil {
		return err
	}
	ls := limpiarEtiquetas(etiquetas)
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(ls) == 0 {
		if _, ya := a.ind.Etiquetas[r]; !ya {
			return nil
		}
		delete(a.ind.Etiquetas, r)
	} else {
		if a.ind.Etiquetas == nil {
			a.ind.Etiquetas = map[string][]string{}
		}
		a.ind.Etiquetas[r] = ls
	}
	return a.guardarIndice()
}

// TodasLasEtiquetas son las que se usan en algún elemento, sin repetir y en orden: lo
// que el diálogo ofrece para marcar.
func (a *Archivos) TodasLasEtiquetas(context.Context) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var todas []string
	for _, ls := range a.ind.Etiquetas {
		todas = append(todas, ls...)
	}
	return limpiarEtiquetas(todas)
}

// conEtiqueta dice si la ruta lleva una etiqueta que contiene q (ya plegada con buscar.Plegar). Con
// a.mu tomado.
func (a *Archivos) conEtiqueta(r, q string) bool {
	return slices.ContainsFunc(a.ind.Etiquetas[r], func(l string) bool { return strings.Contains(buscar.Plegar(l), q) })
}

// moverEtiquetas lleva las etiquetas de lo que cambia de ruta. Con a.mu tomado.
func (a *Archivos) moverEtiquetas(cambiar func(string) (string, bool)) {
	if len(a.ind.Etiquetas) == 0 {
		return
	}
	nuevas := make(map[string][]string, len(a.ind.Etiquetas))
	for r, ls := range a.ind.Etiquetas {
		if n, ok := cambiar(r); ok {
			r = n
		}
		nuevas[r] = ls
	}
	a.ind.Etiquetas = nuevas
}

// sacarEtiquetas quita del índice las de r y, si es carpeta, las de lo que cuelga de
// ella, y las devuelve. Con a.mu tomado.
func (a *Archivos) sacarEtiquetas(r string, carpeta bool) map[string][]string {
	var out map[string][]string
	for k, ls := range a.ind.Etiquetas {
		if k == r || (carpeta && dentro(r, k)) {
			if out == nil {
				out = map[string][]string{}
			}
			out[k] = ls
			delete(a.ind.Etiquetas, k)
		}
	}
	return out
}
