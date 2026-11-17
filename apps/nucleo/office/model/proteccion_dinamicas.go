// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Las tablas dinámicas de una hoja protegida (`pivots`) no se crean ni se cambian: Excel
// lo deshabilita y aquí no hay permiso que lo conceda. Pero al insertar o borrar filas o
// columnas su origen, su esquina y lo que ocupan SIGUEN A SUS CELDAS
// (`moverDinamicas` de refshift.ts), igual que una fórmula. Vetar cualquier cambio de
// `pivots` dejaba la tabla dinámica leyendo un origen corrido una fila —con la fila de
// encabezados dentro de los datos o la última fila fuera— sin avisar, que es justo lo
// que la protección debería impedir.
//
// Como con las fórmulas de otras hojas (proteccion_ajenas.go) y los rangos editables
// (proteccion_desplazar.go), el servidor acepta ese cambio solo si es EXACTAMENTE el
// desplazamiento: `dinamicasMovidas` reproduce `moverDinamicas` campo a campo —solo
// `source`, `anchor` y `range` cambian; la que pierde su origen entero, o su esquina y
// todo lo que ocupaba, desaparece— y el resultado tiene que ser lo que llega, ni una
// tabla de más ni un campo distinto. Hay dos maneras de que ocurra:
//
//   - en la PROPIA hoja protegida, con el permiso de insertar o borrar concedido: el
//     mismo desplazamiento tiene que explicar a la vez las dinámicas, el contenido y el
//     bloqueo de cada línea y los rangos editables (`rangosHojaMovidos`);
//   - en la hoja de la que LEEN (`sourceSheet`): la hoja protegida pasa a necesitar el
//     desplazamiento de otra hoja, y lo decide `ajenasLibro` con el MISMO para todo el
//     guardado (sus topes, el permiso en todas las hojas gemelas, nunca otro libro). Con
//     él, las dinámicas de cada hoja protegida —las que cambian y las que no— tienen que
//     quedar exactamente como llegan.
//
// Rechazar de más nunca relaja la protección: el propietario sí puede.
//
// Las que el servidor NO SABE LEER (`pivots` que no es una lista de objetos, que pasa de
// maxDinamicas —la web no tiene tope— o con un valor que no se decodifica) no se pueden
// mover para comparar, así que no se sabe si un desplazamiento las deja bien:
//
//   - si el guardado no alega ninguno, basta con que lleguen iguales (mismoJSON);
//   - si alega uno que podría moverlas —en su hoja: rangos o dinámicas movidos, o celdas o
//     marcas bloqueadas que cambian de sitio con un permiso estructural; o en otra hoja,
//     que decide ajenasLibro—, se rechaza;
//   - y quien no está autorizado no ESCRIBE unas ilegibles, nuevas o cambiadas: lo que
//     llega distinto tiene que leerse (dinamicasLegibles).
//
// Sin tablas (ausente, null, lista u objeto vacío: la web y Fyne lo leen como ninguna)
// no hay nada que mover. Un documento con unas ilegibles se sigue abriendo y guardando;
// solo el desplazamiento que las pudiera mover queda para el propietario.

// maxDinamicas es el tope de tablas dinámicas de una hoja que se comprueban: un libro
// real tiene unas pocas.
const maxDinamicas = 64

// listaDinamicas lee `pivots` como lista de objetos (nil si no hay); false si no lo es o
// si algún valor no se decodifica (un número que no cabe en un float64 se decodificaba
// como null y pasaba por igual a uno que sí).
func listaDinamicas(raw json.RawMessage) ([]map[string]json.RawMessage, bool) {
	if isNullJSON(raw) {
		return nil, true
	}
	var l []map[string]json.RawMessage
	if json.Unmarshal(raw, &l) != nil || len(l) > maxDinamicas {
		return nil, false
	}
	for _, p := range l {
		if p == nil {
			return nil, false
		}
	}
	var todo any
	if json.Unmarshal(raw, &todo) != nil {
		return nil, false
	}
	return l, true
}

// dinamicasVacias: ¿no hay ninguna tabla dinámica en `pivots`? Un valor que no se
// decodifica no cuenta como vacío.
func dinamicasVacias(raw json.RawMessage) bool {
	if isNullJSON(raw) {
		return true
	}
	var x any
	return json.Unmarshal(raw, &x) == nil && vacioANil(x) == nil
}

// dinamicasLegibles: ¿es `pivots` lo que se puede escribir? Nada, o una lista de hasta
// maxDinamicas objetos que se decodifica sin error (listaDinamicas). Ni siquiera `{}`:
// la web lo descarta y Fyne no abre la hoja.
func dinamicasLegibles(raw json.RawMessage) bool {
	_, ok := listaDinamicas(raw)
	return ok
}

// dinamicasIlegibles: ¿hay en `pivots` tablas que el servidor no sabe leer, y por tanto
// no sabe mover?
func dinamicasIlegibles(raw json.RawMessage) bool {
	return !dinamicasVacias(raw) && !dinamicasLegibles(raw)
}

// leerDinamicas es listaDinamicas con lo vacío (`{}` también) como ninguna.
func leerDinamicas(raw json.RawMessage) ([]map[string]json.RawMessage, bool) {
	if dinamicasVacias(raw) {
		return nil, true
	}
	return listaDinamicas(raw)
}

// mismoJSON: ¿llega `pivots` igual que estaba? Los mismos bytes, o el mismo valor
// decodificado sin error (quien lo vuelve a serializar puede cambiar espacios o
// escapes). Sin la tolerancia de `distinto` con lo vacío, que dejaba escribir `{}` donde
// no había nada.
func mismoJSON(a, b json.RawMessage) bool {
	if bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b)) {
		return true
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// campoTexto lee un campo de texto: (valor, presente, legible).
func campoTexto(m map[string]json.RawMessage, k string) (string, bool, bool) {
	raw, ok := m[k]
	if !ok {
		return "", false, true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", true, false
	}
	return s, true, true
}

// hojaOrigen es la hoja de la que lee la tabla dinámica (`sourceSheet ?? s.name`): la
// que la aloja si no dice otra; "" con un valor que no es texto.
func hojaOrigen(p map[string]json.RawMessage, aloja string) (string, bool) {
	raw, ok := p["sourceSheet"]
	if !ok || isNullJSON(raw) {
		return aloja, true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// moverDinamicas es `moverDinamicas` de refshift.ts sobre el JSON: la lista `previas` de
// la hoja `aloja` tras `d` (propia: el desplazamiento es en esa hoja y mueve también la
// esquina y lo que ocupan). false si una tabla no se deja leer como la lee la web.
func moverDinamicas(previas []map[string]json.RawMessage, aloja string, d formula.Desplazamiento, propia bool) ([]map[string]json.RawMessage, bool) {
	vivo := func(ref string) (string, bool) {
		r := d.MoverReferencia(ref)
		return r, r != formula.RefRota
	}
	out := make([]map[string]json.RawMessage, 0, len(previas))
	for _, p := range previas {
		q := make(map[string]json.RawMessage, len(p))
		for k, v := range p {
			q[k] = v
		}
		origen, ok := hojaOrigen(p, aloja)
		if !ok {
			return nil, false
		}
		// Tal cual, distinguiendo mayúsculas: es lo que compara la web al leer el origen.
		if d.Hoja == "" || origen == d.Hoja {
			src, presente, legible := campoTexto(p, "source")
			if !presente || !legible {
				return nil, false
			}
			nuevo, vive := vivo(src)
			if !vive {
				continue // su origen ya no existe: la tabla se va con él
			}
			q["source"] = jsonTexto(nuevo)
		}
		if propia {
			vive, legible := moverEsquina(p, q, vivo)
			if !legible {
				return nil, false
			}
			if !vive {
				continue // sin esquina ni nada de lo que ocupaba
			}
		}
		out = append(out, q)
	}
	return out, true
}

// moverEsquina pone en `q` la esquina y lo que ocupa la tabla `p` tras el
// desplazamiento: sin esquina, la primera celda que queda de lo que ocupaba. Devuelve si
// sigue viva y si se dejó leer.
func moverEsquina(p, q map[string]json.RawMessage, vivo func(string) (string, bool)) (vive, legible bool) {
	rango := ""
	r, presente, ok := campoTexto(p, "range")
	if presente && !ok {
		return false, false
	}
	if presente {
		if m, sigue := vivo(r); sigue {
			rango = m
		}
	}
	esquinaTxt, presente, ok := campoTexto(p, "anchor")
	if !presente || !ok {
		return false, false
	}
	esquina, sigue := vivo(esquinaTxt)
	if !sigue && rango != "" {
		esquina, _, _ = strings.Cut(rango, ":")
	} else if !sigue {
		esquina = ""
	}
	if esquina == "" {
		return false, true
	}
	q["anchor"] = jsonTexto(esquina)
	if rango != "" {
		q["range"] = jsonTexto(rango)
	} else {
		delete(q, "range")
	}
	return true, true
}

// jsonTexto es un texto ya codificado como valor JSON.
func jsonTexto(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// dinamicasMovidas: ¿es `nuevas` exactamente `previas` tras `d`?
func dinamicasMovidas(previas []map[string]json.RawMessage, nuevas json.RawMessage, aloja string, d formula.Desplazamiento, propia bool) bool {
	movidas, ok := moverDinamicas(previas, aloja, d, propia)
	if !ok {
		return false
	}
	raw, err := json.Marshal(movidas)
	if err != nil {
		return false
	}
	return !distinto(map[string]json.RawMessage{"pivots": raw}, map[string]json.RawMessage{"pivots": nuevas}, "pivots")
}

// bordesRef son los extremos (base uno) de una referencia en un eje; nada si no se lee
// o si es una fila o columna entera en ese eje.
func bordesRef(ref string, filas bool) []int {
	r, ok := leerRangoEditable(strings.ReplaceAll(ref, "$", ""))
	if !ok {
		return nil
	}
	a, b := r.c1, r.c2
	if filas {
		a, b = r.f1, r.f2
	}
	if a == 0 {
		return nil
	}
	return []int{a, b}
}

// --- En la propia hoja protegida ---

// dinamicasPropias son las de la hoja protegida, para que el desplazamiento de esa hoja
// (rangosHojaMovidos) las explique a la vez que todo lo demás.
type dinamicasPropias struct {
	aloja    string
	previas  []map[string]json.RawMessage
	nuevas   json.RawMessage
	listaNue []map[string]json.RawMessage
}

var _ otrosDesplazados = (*dinamicasPropias)(nil)

// nuevasDinamicasPropias lee las dinámicas de la hoja `aloja` antes (p) y después (n).
func nuevasDinamicasPropias(aloja string, p, n map[string]json.RawMessage) (*dinamicasPropias, bool) {
	previas, ok1 := leerDinamicas(p["pivots"])
	nuevas, ok2 := leerDinamicas(n["pivots"])
	if !ok1 || !ok2 {
		return nil, false
	}
	return &dinamicasPropias{aloja: aloja, previas: previas, nuevas: n["pivots"], listaNue: nuevas}, true
}

// bordes: los del origen (si es de esta hoja), la esquina y lo que ocupan.
func (dp *dinamicasPropias) bordes(filas bool) (antes, despues []int) {
	de := func(l []map[string]json.RawMessage) []int {
		var out []int
		for _, p := range l {
			if o, ok := hojaOrigen(p, dp.aloja); ok && o == dp.aloja {
				s, _, _ := campoTexto(p, "source")
				out = append(out, bordesRef(s, filas)...)
			}
			for _, k := range []string{"anchor", "range"} {
				s, _, _ := campoTexto(p, k)
				out = append(out, bordesRef(s, filas)...)
			}
		}
		return out
	}
	return de(dp.previas), de(dp.listaNue)
}

// explica: ¿las deja ese desplazamiento de la hoja (base uno) como llegan?
func (dp *dinamicasPropias) explica(d desplazamiento) bool {
	fd := formula.Desplazamiento{Filas: d.filas, Desde: d.desde - 1, Cantidad: d.n, Tope: d.tope(), Hoja: dp.aloja}
	return dinamicasMovidas(dp.previas, dp.nuevas, dp.aloja, fd, true)
}

// --- En la hoja de la que leen ---

// dinamicasHoja son las tablas dinámicas de una hoja protegida tal como las ve
// `ajenasLibro`: con un desplazamiento en otra hoja, solo cambia el origen de las que
// leen de ella, y tienen que quedar como llegan.
type dinamicasHoja struct {
	previas []map[string]json.RawMessage
	nuevas  json.RawMessage
	// legibles: `previas` se dejó leer (dinamicasLegibles). Con unas ilegibles no se sabe
	// si el desplazamiento las mueve, y cualquiera se rechaza; si cambian, contenidoHoja
	// ya lo ha rechazado.
	legibles bool
	// necesita: cambian y no las explica un desplazamiento de la propia hoja.
	necesita bool
}

// leerDinamicasHoja prepara las dinámicas de la hoja p → n para `ajenasLibro`.
func leerDinamicasHoja(p, n map[string]json.RawMessage, necesita bool) dinamicasHoja {
	previas, ok := leerDinamicas(p["pivots"])
	return dinamicasHoja{previas: previas, nuevas: n["pivots"], legibles: ok, necesita: necesita}
}

// valen: ¿las deja `d` como llegan? `aloja` es la hoja protegida y `hoja` el nombre de
// verdad de la que se mueve: la web compara el origen con él tal cual.
func (dh dinamicasHoja) valen(aloja, hoja string, d formula.Desplazamiento) bool {
	if !dh.legibles {
		return false // no se sabe si las mueve: rechazar de más nunca relaja la protección
	}
	if len(dh.previas) == 0 && !dh.necesita {
		return true
	}
	d.Hoja = hoja
	return dinamicasMovidas(dh.previas, dh.nuevas, aloja, d, false)
}

// hojaCitada es la única hoja (sin distinguir mayúsculas) de la que leen las dinámicas
// que cambian: las de antes que no llegan igual y las que llegan sin estar antes. Un
// desplazamiento en otra hoja solo mueve las que leen de ella, así que una que lee de
// la propia hoja, o de dos hojas distintas, no se explica con ninguno.
func (dh dinamicasHoja) hojaCitada(aloja string) (string, bool) {
	nuevas, ok := listaDinamicas(dh.nuevas)
	if !ok || !dh.legibles {
		return "", false
	}
	igual := func(a map[string]json.RawMessage, l []map[string]json.RawMessage) bool {
		for _, b := range l {
			if reflect.DeepEqual(decodificar(a), decodificar(b)) {
				return true
			}
		}
		return false
	}
	hoja := ""
	for _, par := range []struct{ de, en []map[string]json.RawMessage }{{dh.previas, nuevas}, {nuevas, dh.previas}} {
		for _, q := range par.de {
			if igual(q, par.en) {
				continue
			}
			o, ok := hojaOrigen(q, aloja)
			if !ok || strings.EqualFold(o, aloja) || (hoja != "" && !strings.EqualFold(o, hoja)) {
				return "", false
			}
			hoja = o
		}
	}
	return hoja, hoja != ""
}

// citaDinamicas anota en `m` los extremos (base cero, por eje) de los orígenes de las
// dinámicas de `l` que leen de `hoja`.
func citaDinamicas(l []map[string]json.RawMessage, aloja, hoja string, m [2]map[int]bool) {
	for _, q := range l {
		if o, ok := hojaOrigen(q, aloja); !ok || !strings.EqualFold(o, hoja) {
			continue
		}
		s, _, _ := campoTexto(q, "source")
		for _, filas := range []bool{true, false} {
			for _, x := range formula.Extremos(s, filas) {
				m[eje(filas)][x] = true
			}
		}
	}
}
