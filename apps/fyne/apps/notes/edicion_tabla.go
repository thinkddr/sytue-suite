// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// LAS ÓRDENES DE TABLA del editor: el botón «Tabla» (`insertarTabla` de RichText.tsx) y
// la barra que sale con el cursor en una celda, que son las funciones de
// `packages/ui/src/browser/tabla.ts` —las mismas que usa el redactor de Mail—.
//
// Cada orden rehace la tabla entera en una `Tabla` NUEVA (las fotos de deshacer
// comparten la vieja) y deja el cursor en la misma celda, que es lo que hace el
// navegador al tocar el DOM sin mover la selección.

// Los estilos que la barra de la web escribe en línea: `ESTILO_CELDA` de tabla.ts y el
// de la tabla de `insertarTabla`. El servicio los quita al guardar (ver tabla.go).
const (
	EstiloCelda      = "border: 1px solid #d4d0c8; padding: 6px 8px"
	EstiloTablaNueva = "border-collapse: collapse; width: 100%"
)

// Las órdenes de la barra de tabla, con su clave de catálogo, en el orden de la web.
var OrdenesTabla = []struct{ Orden, Clave string }{
	{"filaEncima", "editor.rowAbove"},
	{"filaDebajo", "editor.rowBelow"},
	{"colAntes", "editor.colBefore"},
	{"colDespues", "editor.colAfter"},
	{"borrarFila", "editor.rowDelete"},
	{"borrarCol", "editor.colDelete"},
	{"borrarTabla", "editor.tableDelete"},
}

// celdaVacia son los bloques de una celda nueva: `&nbsp;` para que no se colapse.
func celdaVacia() []Bloque {
	return []Bloque{{Tipo: Parrafo, Tramos: []Tramo{{Texto: "\u00a0"}}}}
}

func celdaNueva() CeldaTabla {
	return CeldaTabla{Atributos: []html.Attribute{{Key: "style", Val: EstiloCelda}}}
}

// InsertarTabla mete una tabla de filas × columnas en el cursor y detrás un párrafo
// vacío (`<table>…</table><p><br></p>`), con el cursor en él, que es donde lo deja el
// `insertHTML` del navegador. Dentro de una celda no hace nada: una tabla anidada no se
// edita (va opaca, ver documento.go), y meter algo que no se puede rellenar es peor que
// no meterlo.
func (d *Documento) InsertarTabla(p Pos, filas, columnas int) Pos {
	if len(d.Bloques) > 0 && d.Bloques[p.B].Celda.EnTabla() {
		return p
	}
	t := &Tabla{Atributos: []html.Attribute{{Key: "style", Val: EstiloTablaNueva}}}
	celdas := make([][][]Bloque, filas)
	for f := range filas {
		fila := FilaTabla{Seccion: "tbody", Nueva: f == 0}
		for range columnas {
			fila.Celdas = append(fila.Celdas, celdaNueva())
			celdas[f] = append(celdas[f], celdaVacia())
		}
		t.Filas = append(t.Filas, fila)
	}
	tabla := aplanar(t, celdas)
	parrafo := Bloque{Tipo: Parrafo, Etiqueta: "p"}
	if len(d.Bloques) == 0 {
		tabla = append(tabla, parrafo)
		d.Bloques = tabla
		return Pos{len(d.Bloques) - 1, 0}
	}
	actual := d.Bloques[p.B]
	if vacio(actual) {
		d.Bloques = slices.Concat(d.Bloques[:p.B], tabla, []Bloque{parrafo}, d.Bloques[p.B+1:])
		return Pos{p.B + len(tabla), 0}
	}
	q := d.Partir(p)
	if vacio(d.Bloques[q.B]) {
		d.Bloques[q.B] = parrafo // al final de la línea, el que nace es el de detrás
		d.Bloques = slices.Insert(d.Bloques, q.B, tabla...)
	} else {
		d.Bloques = slices.Insert(d.Bloques, q.B, append(tabla, parrafo)...)
	}
	return Pos{q.B + len(tabla), 0}
}

// vacio dice si en un bloque escribible no hay nada escrito.
func vacio(b Bloque) bool {
	return b.Tipo != Regla && b.Tipo != BloqueOpaco &&
		strings.TrimSpace(strings.ReplaceAll(b.Texto(), "\u00a0", " ")) == ""
}

// aplanar son los bloques de una tabla, fila por fila y celda por celda, cada uno con su
// `Celda`. Una celda sin bloques recibe uno vacío: sin él no habría dónde poner el cursor.
func aplanar(t *Tabla, celdas [][][]Bloque) []Bloque {
	var out []Bloque
	for f, fila := range t.Filas {
		for c := range fila.Celdas {
			bs := celdas[f][c]
			if len(bs) == 0 {
				bs = []Bloque{{Tipo: Parrafo}}
			}
			for _, b := range bs {
				b.Celda = Celda{Tabla: t, Fila: f, Col: c}
				out = append(out, b)
			}
		}
	}
	return out
}

// rangoTabla son los bloques [i, j) de la tabla del bloque `b`.
func (d *Documento) rangoTabla(b int) (int, int) {
	t := d.Bloques[b].Celda.Tabla
	i, j := b, b+1
	for i > 0 && d.Bloques[i-1].Celda.Tabla == t {
		i--
	}
	for j < len(d.Bloques) && d.Bloques[j].Celda.Tabla == t {
		j++
	}
	return i, j
}

// desplegar son los bloques de la tabla por fila y celda.
func (d *Documento) desplegar(i, j int) [][][]Bloque {
	t := d.Bloques[i].Celda.Tabla
	celdas := make([][][]Bloque, len(t.Filas))
	for f := range t.Filas {
		celdas[f] = make([][]Bloque, len(t.Filas[f].Celdas))
	}
	for _, b := range d.Bloques[i:j] {
		if f, c := b.Celda.Fila, b.Celda.Col; f < len(celdas) && c < len(celdas[f]) {
			celdas[f][c] = append(celdas[f][c], b)
		}
	}
	return celdas
}

// EditarTabla aplica una de OrdenesTabla a la tabla del cursor y devuelve dónde queda.
// Fuera de una tabla no hace nada.
//
//nolint:gocognit,gocyclo // Las órdenes de tabla comparten cursor y estructura, separarlas duplica invariantes.
func (d *Documento) EditarTabla(orden string, p Pos) Pos {
	if len(d.Bloques) == 0 || !d.Bloques[p.B].Celda.EnTabla() {
		return p
	}
	en := d.Bloques[p.B].Celda
	i, j := d.rangoTabla(p.B)
	k := p.B // qué bloque de su celda es el del cursor, para volver a él
	for k > i && d.Bloques[k-1].Celda == en {
		k--
	}
	dentro := p.B - k
	viejas := en.Tabla.Filas
	filas := make([]FilaTabla, len(viejas))
	for f, fila := range viejas {
		fila.Celdas = slices.Clone(fila.Celdas)
		filas[f] = fila
	}
	celdas := d.desplegar(i, j)
	f, c := en.Fila, en.Col
	anchura := 0
	for _, fila := range filas {
		anchura = max(anchura, len(fila.Celdas))
	}
	switch orden {
	case "filaEncima", "filaDebajo":
		fila := FilaTabla{Seccion: filas[f].Seccion, AtributosSeccion: filas[f].AtributosSeccion}
		var nuevas [][]Bloque
		for range filas[f].Celdas {
			fila.Celdas = append(fila.Celdas, celdaNueva())
			nuevas = append(nuevas, celdaVacia())
		}
		donde := f + 1
		if orden == "filaEncima" {
			// La nueva pasa a abrir la sección si la de encima la abría.
			donde, fila.Nueva, filas[f].Nueva = f, filas[f].Nueva, false
			f++
		}
		filas = slices.Insert(filas, donde, fila)
		celdas = slices.Insert(celdas, donde, nuevas)
	case "colAntes", "colDespues":
		for g := range filas {
			// Nivelar las filas cortas ANTES de insertar, como tabla.ts: si no, la
			// columna nueva caería en otra posición en cada fila.
			for len(filas[g].Celdas) < anchura {
				filas[g].Celdas = append(filas[g].Celdas, celdaNueva())
				celdas[g] = append(celdas[g], celdaVacia())
			}
			donde := c + 1
			if orden == "colAntes" {
				donde = c
			}
			filas[g].Celdas = slices.Insert(filas[g].Celdas, donde, celdaNueva())
			celdas[g] = slices.Insert(celdas[g], donde, celdaVacia())
		}
		if orden == "colAntes" {
			c++
		}
	case "borrarFila":
		// La última fila se lleva la tabla: una sin filas no se ve ni se alcanza.
		if len(filas) <= 1 {
			return d.quitarTabla(i, j)
		}
		if filas[f].Nueva && f+1 < len(filas) && !filas[f+1].Nueva {
			filas[f+1].Nueva = true
		}
		filas, celdas = slices.Delete(filas, f, f+1), slices.Delete(celdas, f, f+1)
		f = min(f, len(filas)-1)
		dentro = 0
	case "borrarCol":
		if anchura <= 1 {
			return d.quitarTabla(i, j)
		}
		for g := range filas {
			if c < len(filas[g].Celdas) {
				filas[g].Celdas = slices.Delete(filas[g].Celdas, c, c+1)
				celdas[g] = slices.Delete(celdas[g], c, c+1)
			}
		}
		dentro = 0
	case "borrarTabla":
		return d.quitarTabla(i, j)
	default:
		return p
	}
	t := &Tabla{Atributos: en.Tabla.Atributos, Filas: filas}
	nuevos := aplanar(t, celdas)
	d.Bloques = slices.Concat(d.Bloques[:i], nuevos, d.Bloques[j:])
	// Vuelta a la celda del cursor (o a la que quede en su sitio).
	f = min(f, len(filas)-1)
	c = min(c, len(filas[f].Celdas)-1)
	for k := i; k < i+len(nuevos); k++ {
		if b := d.Bloques[k]; b.Celda.Fila == f && b.Celda.Col == c {
			k += min(dentro, len(celdas[f][c])-1)
			if dentro == 0 || k != p.B {
				return Pos{k, min(p.O, largo(d.Bloques[k]))}
			}
			return Pos{k, p.O}
		}
	}
	return Pos{i, 0}
}

// quitarTabla borra la tabla entera y deja el cursor en lo que venía detrás, o en lo de
// delante si era lo último.
func (d *Documento) quitarTabla(i, j int) Pos {
	d.Bloques = slices.Delete(d.Bloques, i, j)
	switch {
	case i < len(d.Bloques):
		return Pos{i, 0}
	case i > 0:
		return Pos{i - 1, largo(d.Bloques[i-1])}
	}
	return Pos{}
}

// enCelda prepara lo que se pega DENTRO de una celda: todo pasa a ser de esa celda, y una
// tabla pegada va entera como bloque opaco, que es como se lee una tabla dentro de otra
// (documento.go). Aplanada, sus bloques partirían la de fuera en dos.
func enCelda(bloques []Bloque, celda Celda) []Bloque {
	var out []Bloque
	for i := 0; i < len(bloques); i++ {
		b := bloques[i]
		if b.Celda.EnTabla() {
			j := i + 1
			for j < len(bloques) && bloques[j].Celda.Tabla == b.Celda.Tabla {
				j++
			}
			var h strings.Builder
			escribirTabla(&h, bloques[i:j])
			var texto []string
			for _, c := range bloques[i:j] {
				texto = append(texto, c.Texto())
			}
			b = Bloque{
				Tipo: BloqueOpaco, Etiqueta: "table", Crudo: h.String(),
				Tramos: []Tramo{{Texto: strings.Join(texto, " ")}},
			}
			i = j - 1
		}
		b.Celda = celda
		out = append(out, b)
	}
	return out
}
