// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Copiar, cortar y pegar: `doClipboard` y `applyPaste` de SheetCanvas.tsx.
//
// # Lo que sale al portapapeles del sistema es TSV de lo que se VE
//
// Como en la web (`buildClipboardPayload`): el valor calculado de cada fórmula, no su
// texto, porque es lo que espera cualquier otra aplicación donde se pegue.
//
// # Y lo que se pega DENTRO conserva las fórmulas
//
// La web guarda además una tabla HTML con el texto crudo de cada celda y, al pegar, la
// reconoce como suya y desplaza las referencias relativas (`shiftCellHtml`). El
// portapapeles de Fyne solo lleva texto, así que la copia de alta fidelidad va aparte y
// se reconoce porque el texto del portapapeles sigue siendo el que se escribió. Si otra
// aplicación lo pisa entre copiar y pegar, se pega su texto.
//
// Aparte es en memoria y, además, en la carpeta que comparte la suite
// (`ajustes.Carpeta`): así una ventana de Calcus pega las fórmulas que copió OTRA —dos
// procesos—, como la web entre dos pestañas. Lo que viene de otro proceso se pega como
// copia aunque fuera un corte: vaciar el origen es cosa de su ventana, como en la web.
//
// ponytail: si otra aplicación copia EXACTAMENTE el texto de lo último que copió Calcus,
// se pegan las celdas de Calcus (las fórmulas de lo que se veía igual). Es el mismo
// techo que ya tenía la copia en memoria; se cierra con un portapapeles de varios tipos
// cuando Fyne lo tenga.
type copia struct {
	tsv    string
	c0, f0 int
	celdas [][]celdaCopia // el HTML y el formato de cada celda: lo que se pega dentro
	texto  [][]string     // lo que se ve: lo que se pega «solo valores»
	corte  bool
	rango  [4]int
	hoja   string
}

// Copiar lleva el rango seleccionado al portapapeles; con `corte`, al pegar se vacía
// el origen.
func (r *Rejilla) Copiar(p fyne.Clipboard, corte bool) {
	c1, f1, c2, f2 := r.Rango()
	cp := &copia{c0: c1, f0: f1, corte: corte, rango: [4]int{c1, f1, c2, f2}, hoja: r.L.Hoja()}
	for f := f1; f <= f2; f++ {
		var celdas []celdaCopia
		var texto []string
		for c := c1; c <= c2; c++ {
			cc := r.L.celdaCopia(c, f)
			if r.L.FormulaOculta(c, f) {
				cc.html = escaparHTML(r.L.Texto(c, f)) // su VALOR, nunca el «=…»
			}
			celdas = append(celdas, cc)
			texto = append(texto, r.L.Texto(c, f))
		}
		cp.celdas, cp.texto = append(cp.celdas, celdas), append(cp.texto, texto)
	}
	cp.tsv = serializarTSV(cp.texto)
	r.copiado = cp
	if p != nil {
		p.SetContent(cp.tsv)
		guardarCopia(cp)
	}
}

// copiaCompartida es la copia de alta fidelidad en disco, para otra ventana de Calcus.
type copiaCompartida struct {
	TSV    string            `json:"tsv"`
	C0     int               `json:"c0"`
	F0     int               `json:"f0"`
	Celdas [][]celdaGuardada `json:"celdas"`
	Texto  [][]string        `json:"texto"`
}

type celdaGuardada struct {
	HTML string      `json:"html,omitempty"`
	Num  *FormatoNum `json:"num,omitempty"`
	Est  *Estilo     `json:"est,omitempty"`
}

// carpetaCopia es dónde va; las pruebas la cambian por una temporal.
var carpetaCopia = ajustes.Carpeta

const ficheroCopia = "calcus-copia.json"

// guardarCopia la deja para las otras ventanas. Es lo mejor que se puede hacer: si no se
// puede escribir, se pega igual dentro de esta ventana y las otras pegan valores.
func guardarCopia(cp *copia) {
	dir, err := carpetaCopia()
	if err != nil {
		return
	}
	g := copiaCompartida{
		TSV: cp.tsv, C0: cp.c0, F0: cp.f0, Texto: cp.texto,
		Celdas: mapa(cp.celdas, func(c celdaCopia) celdaGuardada { return celdaGuardada{c.html, c.num, c.est} }),
	}
	datos, err := json.Marshal(g)
	if err != nil {
		return
	}
	// Se escribe al lado y se renombra: la otra ventana nunca lee media copia. El
	// temporal nace con 0600: son celdas de un documento.
	tmp, err := os.CreateTemp(dir, ficheroCopia+".*")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = tmp.Write(datos)
	if cerr := tmp.Close(); err != nil || cerr != nil {
		return
	}
	_ = os.Rename(tmp.Name(), filepath.Join(dir, ficheroCopia))
}

// copiaDeOtra es la copia que dejó otra ventana, si es la de este texto.
func copiaDeOtra(texto string) *copia {
	dir, err := carpetaCopia()
	if err != nil {
		return nil
	}
	datos, err := os.ReadFile(filepath.Join(dir, ficheroCopia))
	if err != nil {
		return nil
	}
	var g copiaCompartida
	if json.Unmarshal(datos, &g) != nil || g.TSV != texto {
		return nil
	}
	return &copia{
		tsv: g.TSV, c0: g.C0, f0: g.F0, texto: g.Texto,
		celdas: mapa(g.Celdas, func(c celdaGuardada) celdaCopia { return celdaCopia{c.HTML, c.Num, c.Est} }),
	}
}

// Pegar vuelca `texto` —lo que hay en el portapapeles— en la esquina superior
// izquierda de la selección, y deja seleccionado lo pegado. `soloValores` es el pegado
// especial (Ctrl+Mayús+V): literales, sin fórmulas.
func (r *Rejilla) Pegar(texto string, soloValores bool) {
	if r.SoloLectura {
		return
	}
	c0, f0, _, _ := r.Rango()
	cp := r.copiado
	if cp == nil || cp.tsv != texto {
		cp = copiaDeOtra(texto) // la de otra ventana de Calcus, si es esta
	}
	nuestra := cp != nil && cp.tsv == texto
	var bloque [][]celdaCopia
	switch {
	case nuestra && !soloValores:
		dc, df := c0-cp.c0, f0-cp.f0
		bloque = mapa(cp.celdas, func(cc celdaCopia) celdaCopia {
			cc.html = desplazarCelda(cc.html, dc, df)
			return cc
		})
	case nuestra:
		bloque = mapa(cp.texto, literal)
	default:
		bloque = mapa(leerTSV(texto), literal)
	}
	var corte *[4]int
	if nuestra && cp.corte && cp.hoja == r.L.Hoja() {
		corte = &cp.rango
		r.copiado = nil
	}
	r.L.Pegar(c0, f0, r.Cols, r.Fils, bloque, corte)

	cMax, fMax := c0, f0
	for i, fila := range bloque {
		fMax = max(fMax, min(f0+i, r.Fils-1))
		cMax = max(cMax, min(c0+len(fila)-1, r.Cols-1))
	}
	r.sel, r.ext = Celda{C: c0, F: f0}, nil
	if cMax != c0 || fMax != f0 {
		r.ext = &Celda{C: cMax, F: fMax}
	}
	r.aLaVista(c0, f0)
	if r.AlEditar != nil {
		r.AlEditar()
	}
	r.Refresh()
}

// desplazarCelda es `shiftCellHtml`: una fórmula se pega con sus referencias
// relativas movidas y su negrita, cursiva y alineación intactas; un literal, tal cual.
func desplazarCelda(html string, dc, df int) string {
	if html == "" || !esFormula(html) {
		return html
	}
	p := leerCelda(html)
	p.dentro = escaparHTML(formula.ShiftRefs(textoPlano(html), dc, df))
	return p.html()
}

// literal es una celda pegada como texto: escapado y SIN formato, que al pegar borra
// el que tuviera la de destino (`put(…, null, null)` en la web).
func literal(s string) celdaCopia {
	if s == "" {
		return celdaCopia{}
	}
	return celdaCopia{html: escaparHTML(s)}
}

func mapa[A, B any](filas [][]A, f func(A) B) [][]B {
	out := make([][]B, len(filas))
	for i, fila := range filas {
		out[i] = make([]B, len(fila))
		for j, v := range fila {
			out[i][j] = f(v)
		}
	}
	return out
}

var reSaltoTSV = regexp.MustCompile(`[\t\r\n]+`)

// serializarTSV es `serializeTsv`: filas por «\n», columnas por «\t», y lo que dentro
// de una celda rompería la tabla, cambiado por un espacio.
func serializarTSV(filas [][]string) string {
	lineas := make([]string, len(filas))
	for i, fila := range filas {
		celdas := make([]string, len(fila))
		for j, v := range fila {
			celdas[j] = reSaltoTSV.ReplaceAllString(v, " ")
		}
		lineas[i] = strings.Join(celdas, "\t")
	}
	return strings.Join(lineas, "\n")
}

// leerTSV es `parseTsv`: ignora UN salto final, que los portapapeles añaden.
func leerTSV(texto string) [][]string {
	t := strings.ReplaceAll(strings.ReplaceAll(texto, "\r\n", "\n"), "\r", "\n")
	t = strings.TrimSuffix(t, "\n")
	if t == "" {
		return [][]string{{""}}
	}
	var out [][]string
	for _, linea := range strings.Split(t, "\n") {
		out = append(out, strings.Split(linea, "\t"))
	}
	return out
}
