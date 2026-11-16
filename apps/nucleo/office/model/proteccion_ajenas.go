// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Al insertar o borrar filas en «Ventas», las fórmulas de las OTRAS hojas que la citan
// siguen a su dato (`moverReferenciasAjenas` de refshift.ts): «Resumen» con
// `=Ventas!B4` pasa a `=Ventas!B5`. Si «Resumen» está protegida, esas celdas suelen
// estar bloqueadas, y reescribirlas parecería editar contenido protegido. Pero no lo es:
// quien inserta la fila en «Ventas» no cambia lo que dice la fórmula, solo dónde está
// su dato. Negarlo dejaba la fórmula bloqueada sumando la fila de al lado sin avisar,
// que es justo lo que la protección debería impedir.
//
// Aquí el servidor acepta ese cambio solo si es EXACTAMENTE el desplazamiento:
//
//   - existe una inserción o un borrado de filas o columnas en otra hoja H del libro,
//     que la persona podía hacer (H sin proteger, o protegida con el permiso concedido);
//   - aplicado a TODAS las celdas de la hoja protegida (`ReescribirCeldaEn`, el mismo
//     código que usa el lienzo nativo), da lo que llega, salvo donde la persona podía
//     escribir de todos modos. Ni una celda bloqueada más, ni una menos: reescribir unas
//     y dejar otras no es lo que produce una inserción, y se rechaza.
//
// No se exige ver el desplazamiento en H, y no hace falta: quien puede insertar una fila
// en H puede insertarla y volver a escribir las celdas de H como estaban, así que el
// modelo resultante es uno que ya podía producir con acciones permitidas. Si H está
// protegida, la persona necesita su permiso, como para el desplazamiento real.
//
// ponytail: el desplazamiento se busca con candidatos sacados de las propias fórmulas
// (sus extremos antes y después, y lo que creció o menguó H); uno que no se encuentre se
// rechaza, y rechazar de más nunca relaja la protección (el propietario sí puede).

// hojasAjenas es lo que la hoja protegida necesita saber de las demás del libro.
type hojasAjenas struct {
	// nombre de la hoja protegida, donde viven las celdas.
	nombre string
	// puede: ¿podía la persona abrir o cerrar ese tramo (permiso de `desplazamiento`)
	// en la hoja `hoja` (sin distinguir mayúsculas)?
	puede func(hoja, permiso string) bool
	// crece: cuánto cambió la última fila (o columna) con contenido de esa hoja.
	crece func(hoja string, filas bool) int
}

// desplazablesDesde prepara las hojasAjenas de la hoja protegida `nombre` entre el
// libro previo y el nuevo.
func desplazablesDesde(prev, nuevo *libro, nombre string) *hojasAjenas {
	buscar := func(l *libro, hoja string) map[string]json.RawMessage {
		for i, n := range l.nombres {
			if strings.EqualFold(n, hoja) {
				return l.hojas[i]
			}
		}
		return nil
	}
	return &hojasAjenas{
		nombre: nombre,
		puede: func(hoja, permiso string) bool {
			if strings.EqualFold(hoja, nombre) || buscar(nuevo, hoja) == nil {
				return false
			}
			ph := buscar(prev, hoja)
			if ph == nil || !protegido(ph["protect"]) {
				return true // sin proteger (o recién creada): cualquier editor puede
			}
			pp := map[string]json.RawMessage{}
			_ = json.Unmarshal(ph["protect"], &pp)
			return dice(pp, permiso, true)
		},
		crece: func(hoja string, filas bool) int {
			ultima := func(h map[string]json.RawMessage) int {
				cel, _ := celdas(h)
				u := 0
				for ref := range cel {
					c, f, ok := leerCelda(ref)
					switch {
					case !ok:
					case filas:
						u = max(u, f)
					default:
						u = max(u, c)
					}
				}
				return u
			}
			return abs(ultima(buscar(nuevo, hoja)) - ultima(buscar(prev, hoja)))
		},
	}
}

// Topes de la búsqueda, para que un modelo hecho a mano no la convierta en trabajo sin
// fin: por encima se rechaza.
const (
	maxCeldasAjenas  = 64  // celdas cambiadas de las que se sacan candidatos
	maxExtremosAjena = 256 // extremos de referencia por eje
)

// explican: ¿hay un desplazamiento en otra hoja que lleve pc a nc con las reglas de
// contenido de siempre (celdasPermitidas)?
func (a *hojasAjenas) explican(pc, nc map[string]string, bloqueada func(string) bool, relajado bool) bool {
	if a == nil {
		return false
	}
	var cambian []string
	for ref := range union(pc, nc) {
		if pc[ref] != nc[ref] && bloqueada(ref) {
			cambian = append(cambian, ref)
		}
	}
	slices.Sort(cambian) // determinista: los candidatos no dependen del orden del mapa
	for _, d := range a.candidatos(pc, nc, cambian) {
		// Primero lo barato: las bloqueadas que cambian son su reescritura.
		bien := true
		for _, ref := range cambian {
			if d.ReescribirCeldaEn(pc[ref], a.nombre) != nc[ref] {
				bien = false
				break
			}
		}
		if !bien {
			continue
		}
		movidas := make(map[string]string, len(pc))
		for ref, v := range pc {
			movidas[ref] = d.ReescribirCeldaEn(v, a.nombre)
		}
		if celdasPermitidas(movidas, nc, bloqueada, relajado) {
			return true
		}
	}
	return false
}

// candidatos son los desplazamientos que vale la pena probar, ya filtrados por permiso.
//
//nolint:gocognit,gocyclo // hojas × ejes × cantidades × comienzos, en ese orden.
func (a *hojasAjenas) candidatos(pc, nc map[string]string, cambian []string) []formula.Desplazamiento {
	type extremos struct{ antes, despues map[int]bool }
	ejes := map[bool]*extremos{
		true:  {map[int]bool{}, map[int]bool{}},
		false: {map[int]bool{}, map[int]bool{}},
	}
	var hojas []string
	anota := func(r formula.Referencia, despues bool) {
		for _, filas := range []bool{true, false} {
			e := ejes[filas]
			m := e.antes
			if despues {
				m = e.despues
			}
			x1, x2 := r.Rect.C1, r.Rect.C2
			if filas {
				x1, x2 = r.Rect.R1, r.Rect.R2
			}
			m[x1], m[x2] = true, true
		}
	}
	for i, ref := range cambian {
		if i >= maxCeldasAjenas {
			break
		}
		for _, r := range formula.ExtractRefs(formula.TextoCelda(pc[ref])) {
			if r.Hoja == "" || strings.EqualFold(r.Hoja, a.nombre) {
				continue
			}
			if !slices.ContainsFunc(hojas, func(h string) bool { return strings.EqualFold(h, r.Hoja) }) {
				hojas = append(hojas, r.Hoja)
			}
			anota(r, false)
		}
		for _, r := range formula.ExtractRefs(formula.TextoCelda(nc[ref])) {
			if r.Hoja != "" && !strings.EqualFold(r.Hoja, a.nombre) {
				anota(r, true)
			}
		}
	}
	var out []formula.Desplazamiento
	for _, hoja := range hojas {
		for _, filas := range []bool{true, false} {
			e := ejes[filas]
			if len(e.antes) > maxExtremosAjena || len(e.despues) > maxExtremosAjena {
				return nil
			}
			antes, despues := ordenados(e.antes), ordenados(e.despues)
			for _, m := range cantidades(antes, despues, a.crece(hoja, filas)) {
				for _, signo := range []int{1, -1} {
					d := desplazamiento{filas: filas, n: signo * m}
					if !a.puede(hoja, d.permiso()) {
						continue
					}
					for _, desde := range comienzos(antes, despues, m) {
						out = append(out, formula.Desplazamiento{
							Filas: filas, Desde: desde, Cantidad: signo * m, Tope: d.tope(), Hoja: hoja,
						})
					}
				}
			}
		}
	}
	return out
}

// cantidades: lo que creció la hoja citada y lo que se movió cada extremo.
func cantidades(antes, despues []int, crece int) []int {
	set := map[int]bool{}
	for _, x := range antes {
		for _, y := range despues {
			if x != y {
				set[abs(x-y)] = true
			}
		}
	}
	delete(set, crece)
	out := make([]int, 0, len(set)+1)
	if crece > 0 {
		out = append(out, crece) // la primera: es la de casi siempre
	}
	resto := ordenados(set)
	out = append(out, resto...)
	return out[:min(len(out), maxCandidatas)]
}

// comienzos: dónde puede empezar un tramo de m que explique esos extremos (base cero).
// Al insertar, en el primer extremo que se mueve; al borrar, justo antes del que se
// mueve (x-m), en el que se pega al borde (j, j+1) o en uno de los dos bordes del que
// muere (x-m+1, x).
func comienzos(antes, despues []int, m int) []int {
	set := map[int]bool{}
	for _, x := range antes {
		for _, c := range []int{x, x - m, x - m + 1} {
			if c >= 0 {
				set[c] = true
			}
		}
	}
	for _, j := range despues {
		set[j], set[j+1] = true, true
	}
	return ordenados(set)
}

func ordenados(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
