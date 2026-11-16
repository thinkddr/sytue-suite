// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// IDENTIDAD de las hojas, de sus filas y de sus columnas, y los comentarios anclados a una
// celda. Es `sheets/idhoja.ts`, `sheets/anclas.ts` y `sheets/anclaCelda.ts` de la web.
//
// Un comentario de una hoja va pegado a una celda, como en Excel y Sheets, y la rejilla la
// marca con un triángulo en su esquina. La referencia («B4») no sirve de identidad: insertar
// una fila encima convierte B4 en B5, y el comentario tiene que ir con su celda. Por eso el
// ancla es `{hoja, fila, col, text}` con el id de la hoja y la identidad de su fila y de su
// columna, que este libro lee, conserva y mueve al insertar y borrar igual que la web: las
// primeras posiciones de cada eje van en una lista (`rowIds`, `colIds`) y las de más allá
// son IMPLÍCITAS, `t<posición − longitud de la lista + base>`.

// reIDAncla es lo que admite el servicio en el id de una fila o una columna (`reIDAncla` de
// services/docs) y lo que la web deja pasar al cargar (`ID_ANCLA`).
var reIDAncla = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// reImplicita es el nombre de una posición implícita, con el tope de `indiceDe`.
var reImplicita = regexp.MustCompile(`^t(\d{1,7})$`)

// maxAnclasHoja es el tope de la lista materializada de un eje (`MAX_ANCLAS_HOJA`).
const maxAnclasHoja = 20_000

// ejeAnclas es un eje de la hoja (`Eje` de anclas.ts): la lista materializada y el
// desplazamiento del nombre de la cola. Es inmutable: cambiarlo es poner otro, así que la
// foto de deshacer lo comparte.
type ejeAnclas struct {
	ids  []string
	base int
}

// idDe es la identidad de la posición i (0-based).
func (e ejeAnclas) idDe(i int) string {
	if i < len(e.ids) {
		return e.ids[i]
	}
	return "t" + strconv.Itoa(i-len(e.ids)+e.base)
}

// indiceDe es la posición actual de una identidad, o false si ya no existe (borrada).
func (e ejeAnclas) indiceDe(id string) (int, bool) {
	if i := slices.Index(e.ids, id); i >= 0 {
		return i, true
	}
	m := reImplicita.FindStringSubmatch(id)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	if k := n - e.base; k >= 0 {
		return len(e.ids) + k, true
	}
	return 0, false
}

// materializar escribe en la lista hasta `hasta` posiciones con el nombre que ya tenían.
func (e ejeAnclas) materializar(hasta int) ejeAnclas {
	ids := slices.Clone(e.ids)
	falta := min(hasta, maxAnclasHoja) - len(ids)
	for k := range max(falta, 0) {
		ids = append(ids, "t"+strconv.Itoa(e.base+k))
	}
	base := e.base
	if falta > 0 {
		base += falta
	}
	return ejeAnclas{ids: ids, base: base}
}

// mover es `conEstructura`: insertar `cantidad` posiciones nuevas en `desde` (> 0) o quitar
// las que hay (< 0). Las nuevas llevan un id aleatorio, único entre clientes sin hablarse.
func (e ejeAnclas) mover(desde, cantidad int) ejeAnclas {
	if cantidad > 0 {
		m := e.materializar(desde)
		nuevas := make([]string, cantidad)
		for i := range nuevas {
			nuevas[i] = "n" + uuid.NewString()[:8]
		}
		m.ids = slices.Insert(m.ids, min(desde, len(m.ids)), nuevas...)
		return m
	}
	m := e.materializar(desde - cantidad)
	if desde < len(m.ids) {
		m.ids = slices.Delete(m.ids, desde, min(desde-cantidad, len(m.ids)))
	}
	return m
}

// ejeLeido es `sanitizeAnclas` de un eje: ids válidos y únicos, con el tope de la
// materialización, y la base entera y no negativa. Lo que no vale se descarta entero.
func ejeLeido(crudo json.RawMessage, base any) ejeAnclas {
	var ids []anclaID
	if json.Unmarshal(crudo, &ids) != nil || len(ids) == 0 || len(ids) > maxAnclasHoja {
		return ejeAnclas{}
	}
	out := make([]string, 0, len(ids))
	vistos := make(map[string]bool, len(ids))
	for _, x := range ids {
		if !reIDAncla.MatchString(x.ID) || vistos[x.ID] {
			return ejeAnclas{}
		}
		vistos[x.ID] = true
		out = append(out, x.ID)
	}
	e := ejeAnclas{ids: out}
	if n, ok := base.(float64); ok && n >= 0 && n == float64(int(n)) && n <= 1<<53 {
		e.base = int(n)
	}
	return e
}

// escribir es el eje como lo guarda la web: la lista y su base, o nada si no hay lista.
func (e ejeAnclas) escribir() ([]anclaID, int) {
	if len(e.ids) == 0 {
		return nil, 0
	}
	out := make([]anclaID, len(e.ids))
	for i, id := range e.ids {
		out[i] = anclaID{ID: id}
	}
	return out, e.base
}

// anclaID es una entrada de `rowIds`/`colIds`.
type anclaID struct {
	ID string `json:"id"`
}

// nuevoIDHoja es `nuevoIdHoja`: el de una hoja que se añade o se duplica.
func nuevoIDHoja() string { return uuid.NewString() }

// IDsHojas es `conIdsDeHoja`: el id de cada hoja, y para las que no lo tienen (un libro
// anterior a los ids, uno importado) el que deduce la web de su nombre. Es DETERMINISTA, así
// que la web y este libro anclan en la misma hoja sin haberlo guardado ninguno.
func (l *Libro) IDsHojas() []string {
	vistos := map[string]bool{}
	out := make([]string, len(l.f.hojas))
	for i, h := range l.f.hojas {
		id := h.id
		if id == "" {
			id = "h:" + h.nombre
		}
		for vistos[id] {
			id += "#" + strconv.Itoa(i)
		}
		vistos[id] = true
		out[i] = id
	}
	return out
}

// AnclaDeCelda es el ancla de la celda (c, f) de la hoja activa (`anclaDeCelda`); de una
// combinada, la de su ancla, que es la celda que se ve.
func (l *Libro) AnclaDeCelda(c, f int) api.AnclaCelda {
	if fu, ok := l.FusionEn(c, f); ok {
		c, f = fu.C1, fu.F1
	}
	h := l.h()
	return api.AnclaCelda{Hoja: l.IDsHojas()[l.f.activa], Fila: h.filasA.idDe(f), Col: h.colsA.idDe(c), Text: Ref(c, f)}
}

// CeldaDeAncla es dónde está HOY la celda de un ancla (`celdaDeAncla`): la hoja y la
// posición, o false si la hoja, la fila o la columna ya no existen. El comentario no se
// borra; solo deja de marcar una celda.
func (l *Libro) CeldaDeAncla(a api.AnclaCelda) (hoja int, k Celda, ok bool) {
	hoja = slices.Index(l.IDsHojas(), a.Hoja)
	if hoja < 0 {
		return 0, Celda{}, false
	}
	h := l.f.hojas[hoja]
	f, okF := h.filasA.indiceDe(a.Fila)
	c, okC := h.colsA.indiceDe(a.Col)
	if !okF || !okC || f >= MaxFilas || c >= MaxColumnas {
		return 0, Celda{}, false
	}
	return hoja, Celda{C: c, F: f}, true
}

// RefDeAncla es lo que dice el hilo de una celda en el panel: «B5» en la hoja activa,
// «Hoja2!B5» en otra, o "" si la celda ya no existe (`sheetCellRefReader`).
func (l *Libro) RefDeAncla(a api.AnclaCelda) string {
	hoja, k, ok := l.CeldaDeAncla(a)
	switch {
	case !ok:
		return ""
	case hoja == l.f.activa:
		return Ref(k.C, k.F)
	}
	return l.f.hojas[hoja].nombre + "!" + Ref(k.C, k.F)
}

// marcasDeComentarios son las celdas de la hoja activa con marca (`marcasDeComentarios`): las
// de los hilos ABIERTOS anclados a ellas, y de cada una cuántos. Las respuestas no marcan.
func (l *Libro) marcasDeComentarios(cs []api.DocComment) map[Celda]int {
	if len(cs) == 0 {
		return nil
	}
	var out map[Celda]int
	for _, x := range cs {
		if x.Resolved || x.ParentID != "" {
			continue
		}
		a, ok := x.AnclaCelda()
		if !ok {
			continue
		}
		hoja, k, ok := l.CeldaDeAncla(a)
		if !ok || hoja != l.f.activa {
			continue
		}
		if out == nil {
			out = map[Celda]int{}
		}
		out[k]++
	}
	return out
}
