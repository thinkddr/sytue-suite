// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"maps"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Las CELDAS COMBINADAS de la hoja (`merges`), el puerto de `sheets/fusiones.ts`: rangos
// A1 de más de una celda que no se solapan. La de arriba a la izquierda es el ANCLA, que
// se pinta ocupando todo el rango; las demás quedan cubiertas: no se pintan, no se
// seleccionan y no se escriben. Los vectores compartidos (`fusiones` en
// `vectores/calcus.json`) exigen que esto dé lo mismo que la web.

// Fusion es una combinada, en coordenadas de la hoja (columna y fila desde cero). Las
// etiquetas son las de la web (`r1`, `r2`): así se leen los vectores tal cual.
type Fusion struct {
	C1 int `json:"c1"`
	F1 int `json:"r1"` //nolint:tagliatelle // el nombre lo fija la web
	C2 int `json:"c2"`
	F2 int `json:"r2"` //nolint:tagliatelle // el nombre lo fija la web
}

// MaxFusiones es el tope de combinadas por hoja (`MAX_FUSIONES`): lo que importa un .xlsx
// o un .ods no crece sin medida.
const MaxFusiones = 5000

// TopeFusionesVentana es el `tope` por defecto de `fusionesEnVentana`.
const TopeFusionesVentana = 400

// contiene dice si la celda (c, f) cae en la combinada, ancla incluida.
func (u Fusion) contiene(c, f int) bool {
	return c >= u.C1 && c <= u.C2 && f >= u.F1 && f <= u.F2
}

// ancla es la celda que se pinta y la que recibe la selección.
func (u Fusion) ancla() Celda { return Celda{C: u.C1, F: u.F1} }

func tocan(a, b Fusion) bool {
	return a.C1 <= b.C2 && a.C2 >= b.C1 && a.F1 <= b.F2 && a.F2 >= b.F1
}

func fusionDe(r formula.Rect) Fusion { return Fusion{C1: r.C1, F1: r.R1, C2: r.C2, F2: r.R2} }

// parseFusiones son las combinadas válidas de la hoja: más de una celda y sin solaparse
// (la primera gana).
func parseFusiones(merges []string) []Fusion {
	var out []Fusion
	for _, m := range merges {
		if len(out) >= MaxFusiones {
			break
		}
		r, ok := formula.ParseRangeRef(m)
		if !ok || (r.C1 == r.C2 && r.R1 == r.R2) {
			continue
		}
		u := fusionDe(r)
		solapa := false
		for _, o := range out {
			if tocan(u, o) {
				solapa = true
				break
			}
		}
		if !solapa {
			out = append(out, u)
		}
	}
	return out
}

// fusionEn es la combinada que cubre la celda (ancla incluida).
func fusionEn(fs []Fusion, c, f int) (Fusion, bool) {
	for _, u := range fs {
		if u.contiene(c, f) {
			return u, true
		}
	}
	return Fusion{}, false
}

// fusionesEnVentana son las que tocan el rectángulo [c1..c2] × [f1..f2] (lo que se
// pinta), hasta `tope`.
func fusionesEnVentana(fs []Fusion, c1, f1, c2, f2, tope int) []Fusion {
	var out []Fusion
	v := Fusion{C1: c1, F1: f1, C2: c2, F2: f2}
	for _, u := range fs {
		if tocan(u, v) {
			out = append(out, u)
			if len(out) >= tope {
				break
			}
		}
	}
	return out
}

// moverPorFusiones es a dónde va una celda tras un movimiento de teclado de (dc, df): si
// sale de una combinada se la salta entera (Excel), y si cae en una, se queda en su ancla.
func moverPorFusiones(fs []Fusion, base Celda, dc, df, maxC, maxF int) Celda {
	c, f := base.C, base.F
	if desde, ok := fusionEn(fs, c, f); ok {
		switch {
		case dc > 0:
			c = desde.C2
		case dc < 0:
			c = desde.C1
		}
		switch {
		case df > 0:
			f = desde.F2
		case df < 0:
			f = desde.F1
		}
	}
	c, f = min(maxC, max(0, c+dc)), min(maxF, max(0, f+df))
	if hasta, ok := fusionEn(fs, c, f); ok {
		return hasta.ancla()
	}
	return Celda{C: c, F: f}
}

// fusionARango es el «B2:D3» de una combinada, lo que se guarda en `merges`.
func fusionARango(u Fusion) string {
	return formula.NumToCol(u.C1) + strconv.Itoa(u.F1+1) + ":" + formula.NumToCol(u.C2) + strconv.Itoa(u.F2+1)
}

// sinLasQueTocan son las entradas de `merges` que no tocan el rectángulo; las que no se
// leen se conservan, como en la web.
func sinLasQueTocan(merges []string, rect Fusion) []string {
	out := []string{}
	for _, m := range merges {
		if r, ok := formula.ParseRangeRef(m); !ok || !tocan(rect, fusionDe(r)) {
			out = append(out, m)
		}
	}
	return out
}

// combinar es COMBINAR el rectángulo: la lista nueva de `merges` y la combinada que queda,
// o false si no hay nada que combinar. Como Excel, si la selección pisa a medias otra
// combinada se agranda hasta englobarla, y las que quedan dentro se sustituyen por la
// nueva: dos combinadas solapadas no pueden convivir (parseFusiones descartaría la
// segunda y la hoja pintaría otra cosa de lo que se pidió).
func combinar(merges []string, rect Fusion) ([]string, Fusion, bool) {
	existentes := parseFusiones(merges)
	u := rect
	// Agrandar puede alcanzar otra combinada: se repite hasta que no cambie.
	for cambia := true; cambia; {
		cambia = false
		for _, o := range existentes {
			if !tocan(u, o) {
				continue
			}
			if o.C1 < u.C1 || o.F1 < u.F1 || o.C2 > u.C2 || o.F2 > u.F2 {
				u = Fusion{C1: min(u.C1, o.C1), F1: min(u.F1, o.F1), C2: max(u.C2, o.C2), F2: max(u.F2, o.F2)}
				cambia = true
			}
		}
	}
	if u.C1 == u.C2 && u.F1 == u.F2 {
		return nil, Fusion{}, false
	}
	quedan := sinLasQueTocan(merges, u)
	// Con la hoja en el tope, parseFusiones descartaría la nueva: no se pintaría y las
	// celdas cubiertas ya se habrían vaciado. Mejor no combinar.
	if len(parseFusiones(quedan)) >= MaxFusiones {
		return nil, Fusion{}, false
	}
	return append(quedan, fusionARango(u)), u, true
}

// separar quita las combinadas que toca el rectángulo (basta con tener seleccionada su
// ancla). False si no había ninguna, para no anotar un paso de deshacer que no hace nada.
func separar(merges []string, rect Fusion) ([]string, bool) {
	quedan := sinLasQueTocan(merges, rect)
	if len(quedan) == len(merges) {
		return nil, false
	}
	return quedan, true
}

// celdasCubiertas son las celdas guardadas que la combinada CUBRE (todas menos el ancla).
// Se recorre el mapa disperso y no el rango: combinar una columna entera no puede
// iterar un millón de filas vacías.
func celdasCubiertas(celdas map[Celda]string, u Fusion) []Celda {
	var out []Celda
	for k := range celdas {
		if u.contiene(k.C, k.F) && k != u.ancla() {
			out = append(out, k)
		}
	}
	return out
}

// combinarPierdeDatos dice si combinar el rectángulo vaciaría alguna celda con algo
// escrito (y entonces se avisa antes).
func combinarPierdeDatos(celdas map[Celda]string, u Fusion) bool {
	for _, k := range celdasCubiertas(celdas, u) {
		if strings.TrimSpace(textoPlano(celdas[k])) != "" {
			return true
		}
	}
	return false
}

// combinarHoja es lo que hace «Combinar» sobre una hoja: la combinada nueva en `merges` y
// las celdas cubiertas vaciadas —solo se conserva el ancla, como en Excel y Sheets; el
// formato de las cubiertas se queda, que es lo que reaparece al separar—. Con `centrar`,
// el texto del ancla se centra («Combinar y centrar»). Devuelve un mapa NUEVO.
func combinarHoja(celdas map[Celda]string, merges []string, rect Fusion, centrar bool) (map[Celda]string, []string, Fusion, bool) {
	nuevas, u, ok := combinar(merges, rect)
	if !ok {
		return nil, nil, Fusion{}, false
	}
	out := maps.Clone(celdas)
	if out == nil {
		out = map[Celda]string{}
	}
	for _, k := range celdasCubiertas(out, u) {
		delete(out, k)
	}
	if html, hay := out[u.ancla()]; hay && centrar {
		if p := leerCelda(html); p.dentro != "" {
			p.alinear = "center"
			out[u.ancla()] = p.html()
		}
	}
	return out, nuevas, u, true
}

// ---- en el libro ----

// Fusiones son las combinadas válidas de la hoja activa, leídas una vez por versión de
// su lista: la rejilla las mira en cada pintado y en cada tecla.
func (l *Libro) Fusiones() []Fusion {
	h := l.h()
	if !mismaLista(l.fusFuente, h.fusiones) {
		l.fusFuente, l.fusCache = h.fusiones, parseFusiones(h.fusiones)
	}
	return l.fusCache
}

// mismaLista compara por identidad y, si no, por contenido: la lista es inmutable
// (cambiarla es poner otra), así que casi siempre basta la identidad.
func mismaLista(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	if &a[0] == &b[0] {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// FusionEn es la combinada que cubre la celda de la hoja activa.
func (l *Libro) FusionEn(c, f int) (Fusion, bool) { return fusionEn(l.Fusiones(), c, f) }

// Combinar es «Combinar celdas» (y, con `centrar`, «Combinar y centrar»): un solo paso de
// deshacer con las celdas cubiertas vaciadas y la combinada nueva. Devuelve la combinada
// que queda —puede ser mayor que el rango, si pisaba otra— o false si no combinó nada.
func (l *Libro) Combinar(c1, f1, c2, f2 int, centrar bool) (Fusion, bool) {
	h := l.h()
	rect := Fusion{C1: c1, F1: f1, C2: c2, F2: f2}
	celdas, merges, u, ok := combinarHoja(h.celdas, h.fusiones, rect, centrar)
	if !ok {
		return Fusion{}, false
	}
	l.anotar()
	for _, k := range celdasCubiertas(h.celdas, u) {
		l.poner(l.f.activa, k.C, k.F, "")
	}
	if a := u.ancla(); celdas[a] != h.celdas[a] {
		l.poner(l.f.activa, a.C, a.F, celdas[a])
	}
	h.fusiones = merges
	l.version++
	return u, true
}

// CombinarPierdeDatos dice si combinar el rango vaciaría alguna celda escrita, con la
// combinada que de verdad saldría (la que engloba a las que pisa).
func (l *Libro) CombinarPierdeDatos(c1, f1, c2, f2 int) (Fusion, bool) {
	h := l.h()
	_, u, ok := combinar(h.fusiones, Fusion{C1: c1, F1: f1, C2: c2, F2: f2})
	return u, ok && combinarPierdeDatos(h.celdas, u)
}

// Separar es «Separar celdas»: quita las combinadas que toca el rango. False si no había
// ninguna, y entonces no se anota nada.
func (l *Libro) Separar(c1, f1, c2, f2 int) bool {
	h := l.h()
	quedan, ok := separar(h.fusiones, Fusion{C1: c1, F1: f1, C2: c2, F2: f2})
	if !ok {
		return false
	}
	l.anotar()
	if len(quedan) == 0 {
		quedan = nil // no ensuciar el JSON guardado
	}
	l.h().fusiones = quedan
	l.version++
	return true
}
