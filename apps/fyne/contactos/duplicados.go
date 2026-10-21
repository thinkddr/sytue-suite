// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La detección de duplicados y la fusión de dos fichas: `client/duplicates.ts` y
// `client/merge.ts` de la web, sin interfaz.

// normaDuplicado es `norm`: minúsculas y sin espacios ni `.-_()`, para comparar
// nombres y correos con tolerancia al formato.
func normaDuplicado(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', '.', '-', '_', '(', ')':
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(s)))
}

// clavesDe son las claves de identidad de un contacto: cada correo, cada teléfono de
// seis dígitos o más (los cortos son extensiones) y el nombre.
func clavesDe(c api.Contact) []string {
	var claves []string
	for _, e := range c.Emails {
		if k := normaDuplicado(e.Email); k != "" {
			claves = append(claves, "e:"+k)
		}
	}
	for _, t := range c.Phones {
		if k := soloDigitos(t.Phone); len(k) >= 6 {
			claves = append(claves, "t:"+k)
		}
	}
	if k := normaDuplicado(c.DisplayName()); k != "" {
		claves = append(claves, "n:"+k)
	}
	return claves
}

// GruposDuplicados es `findDuplicateGroups`: las fichas que muy probablemente son la
// misma persona porque comparten un correo, un teléfono o el nombre, y de forma
// TRANSITIVA (A y B por correo, B y C por teléfono: un solo grupo). Solo los grupos de
// dos o más, cada contacto en uno, en el orden de la libreta.
func GruposDuplicados(cs []api.Contact) [][]api.Contact {
	padre := make([]int, len(cs))
	for i := range padre {
		padre[i] = i
	}
	raiz := func(i int) int {
		for padre[i] != i {
			padre[i] = padre[padre[i]] // compresión de caminos
			i = padre[i]
		}
		return i
	}
	primero := map[string]int{}
	for i, c := range cs {
		for _, k := range clavesDe(c) {
			if j, ya := primero[k]; ya {
				padre[raiz(j)] = raiz(i)
			} else {
				primero[k] = i
			}
		}
	}
	porRaiz := map[int][]api.Contact{}
	var orden []int
	for i, c := range cs {
		r := raiz(i)
		if _, ya := porRaiz[r]; !ya {
			orden = append(orden, r)
		}
		porRaiz[r] = append(porRaiz[r], c)
	}
	var grupos [][]api.Contact
	for _, r := range orden {
		if g := porRaiz[r]; len(g) > 1 {
			grupos = append(grupos, g)
		}
	}
	return grupos
}

// unirCorreos es `unionValues` para correos: recortados, sin repetir (sin mirar
// mayúsculas) y en su orden, con la primera grafía. Conserva la etiqueta de cada uno y
// el principal del superviviente, que la web pierde al fusionar por valores planos.
func unirCorreos(listas ...[]api.ContactEmail) []api.ContactEmail {
	visto := map[string]bool{}
	var fuera []api.ContactEmail
	principal := false
	for _, l := range listas {
		for _, e := range l {
			e.Email = strings.TrimSpace(e.Email)
			k := strings.ToLower(e.Email)
			if e.Email == "" || visto[k] {
				continue
			}
			visto[k] = true
			e.IsPrimary = e.IsPrimary && !principal
			principal = principal || e.IsPrimary
			fuera = append(fuera, e)
		}
	}
	return fuera
}

// unirTelefonos es lo mismo para los teléfonos.
func unirTelefonos(listas ...[]api.ContactPhone) []api.ContactPhone {
	visto := map[string]bool{}
	var fuera []api.ContactPhone
	principal := false
	for _, l := range listas {
		for _, t := range l {
			t.Phone = strings.TrimSpace(t.Phone)
			k := strings.ToLower(t.Phone)
			if t.Phone == "" || visto[k] {
				continue
			}
			visto[k] = true
			t.IsPrimary = t.IsPrimary && !principal
			principal = principal || t.IsPrimary
			fuera = append(fuera, t)
		}
	}
	return fuera
}

// Fusion es `buildMergeInput`: el superviviente ENTERO (el PUT lo sustituye, y lo que
// no vaya se pierde) con los correos y teléfonos de los dos, y el nombre y la
// organización elegidos. La organización hereda la del perdedor si el superviviente
// no tiene: fusionar nunca pierde un dato que se proyecta.
func Fusion(superviviente, perdedor api.Contact, nombreDelPerdedor, orgDelPerdedor bool) api.ContactInput {
	in := superviviente.Input()
	if nombreDelPerdedor {
		in.FormattedName = perdedor.FormattedName
	}
	if (orgDelPerdedor || in.Organization == "") && perdedor.Organization != "" {
		in.Organization = perdedor.Organization
	}
	in.Emails = unirCorreos(superviviente.Emails, perdedor.Emails)
	in.Phones = unirTelefonos(superviviente.Phones, perdedor.Phones)
	return in
}
