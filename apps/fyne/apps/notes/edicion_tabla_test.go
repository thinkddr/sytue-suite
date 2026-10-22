// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strings"
	"testing"
)

// LOS GUARDIANES DE LAS TABLAS en el modelo: se leen aplanadas y vuelven enteras, se
// escriben dentro de cada celda sin salirse de ella, y las órdenes de la barra de tabla
// dejan el HTML que deja `packages/ui/src/browser/tabla.ts` en la web.

// tdNueva es una celda nueva de la barra, con su `&nbsp;`.
const tdNueva = `<td style="border: 1px solid #d4d0c8; padding: 6px 8px">` + "\u00a0" + `</td>`

func dosPorDos(t *testing.T) *Documento {
	t.Helper()
	return editar(t, `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`)
}

// TestUnaTablaVuelveEntera: cabeceras, marcas, una celda vacía, dos párrafos en una celda
// y un salto; y cada bloque sabe de qué celda es.
func TestUnaTablaVuelveEntera(t *testing.T) {
	crudo := `<p>Recuento:</p><table><tbody><tr><th>Cuarto</th><th>Cajas</th></tr>` +
		`<tr><td><b>Cocina</b></td><td></td></tr><tr><td><p>a</p><p>b</p></td><td>7<br/>8</td></tr></tbody></table><p>Fin.</p>`
	d := Leer(crudo)
	if !d.Fiel() {
		t.Fatalf("no vuelve entera:\n  %s\n  %s", crudo, d.HTML())
	}
	var celdas []string
	for _, b := range d.Bloques {
		if b.Celda.EnTabla() {
			celdas = append(celdas, b.Texto())
		}
	}
	if got := strings.Join(celdas, "|"); got != "Cuarto|Cajas|Cocina||a|b|7\n8" {
		t.Errorf("las celdas son %q", got)
	}
	if u := d.Bloques[len(d.Bloques)-1]; u.Celda.EnTabla() || u.Texto() != "Fin." {
		t.Errorf("lo de detrás de la tabla se ha metido en ella: %+v", u)
	}
}

// TestInsertarUnaTablaYEscribirEnElla: `insertarTabla(3, 3)` al final de una línea deja
// la tabla y un párrafo vacío detrás con el cursor; lo escrito en una celda nueva pisa
// su `&nbsp;`; Intro parte la celda por dentro y no sale de ella.
func TestInsertarUnaTablaYEscribirEnElla(t *testing.T) {
	d := editar(t, "<p>hola</p>")
	p := d.InsertarTabla(Pos{0, 4}, 3, 3)
	fila := "<tr>" + strings.Repeat(tdNueva, 3) + "</tr>"
	quiero(t, d, `<p>hola</p><table style="border-collapse: collapse; width: 100%"><tbody>`+
		strings.Repeat(fila, 3)+`</tbody></table><p><br/></p>`)
	if p != (Pos{10, 0}) {
		t.Errorf("el cursor queda en %v y tenía que ir al párrafo de detrás", p)
	}
	if d.InsertarTabla(Pos{1, 0}, 2, 2) != (Pos{1, 0}) || len(d.Bloques) != 11 {
		t.Error("dentro de una celda no se mete otra tabla")
	}

	q := d.Insertar(Pos{1, 1}, "x", estiloEn(d.Bloques[1], 1))
	q = d.Partir(q)
	d.Insertar(q, "y", estiloEn(d.Bloques[q.B], q.O))
	if got := d.ParaGuardar(); !strings.Contains(got, `<td style="border: 1px solid #d4d0c8; padding: 6px 8px">x<div>y</div></td>`) {
		t.Errorf("la celda no lleva lo escrito:\n  %s", got)
	}
	if !d.Bloques[q.B].Celda.EnTabla() || d.Bloques[q.B].Celda != d.Bloques[1].Celda {
		t.Error("Intro en una celda ha sacado la línea de ella")
	}
}

// TestLaBarraDeTabla son las siete órdenes de tabla.ts, con el cursor en «b».
func TestLaBarraDeTabla(t *testing.T) {
	casos := []struct {
		orden, html string
		cursor      Pos
	}{
		{"filaDebajo", `<table><tbody><tr><td>a</td><td>b</td></tr><tr>` + tdNueva + tdNueva +
			`</tr><tr><td>c</td><td>d</td></tr></tbody></table>`, Pos{1, 1}},
		{"filaEncima", `<table><tbody><tr>` + tdNueva + tdNueva + `</tr><tr><td>a</td><td>b</td></tr>` +
			`<tr><td>c</td><td>d</td></tr></tbody></table>`, Pos{3, 1}},
		{"colAntes", `<table><tbody><tr><td>a</td>` + tdNueva + `<td>b</td></tr><tr><td>c</td>` + tdNueva +
			`<td>d</td></tr></tbody></table>`, Pos{2, 1}},
		{"colDespues", `<table><tbody><tr><td>a</td><td>b</td>` + tdNueva + `</tr><tr><td>c</td><td>d</td>` +
			tdNueva + `</tr></tbody></table>`, Pos{1, 1}},
		{"borrarFila", `<table><tbody><tr><td>c</td><td>d</td></tr></tbody></table>`, Pos{1, 1}},
		{"borrarCol", `<table><tbody><tr><td>a</td></tr><tr><td>c</td></tr></tbody></table>`, Pos{0, 1}},
		{"borrarTabla", ``, Pos{}},
	}
	for _, c := range casos {
		t.Run(c.orden, func(t *testing.T) {
			d := dosPorDos(t)
			antes := d.Bloques[0].Celda.Tabla
			p := d.EditarTabla(c.orden, Pos{1, 1})
			quiero(t, d, c.html)
			if p != c.cursor {
				t.Errorf("el cursor queda en %v, y tenía que quedar en %v", p, c.cursor)
			}
			if len(antes.Filas) != 2 || len(antes.Filas[0].Celdas) != 2 {
				t.Error("la orden ha cambiado la tabla en su sitio: el deshacer la comparte")
			}
		})
	}
	// La última fila y la última columna se llevan la tabla (`eliminarFila`).
	d := editar(t, `<p>x</p><table><tbody><tr><td>a</td></tr></tbody></table>`)
	if p := d.EditarTabla("borrarFila", Pos{1, 0}); p != (Pos{0, 1}) {
		t.Errorf("sin tabla, el cursor tenía que volver al final de lo de antes: %v", p)
	}
	quiero(t, d, `<p>x</p>`)
}

// TestLasTeclasNoSeSalenDeLaCelda: borrar al principio de una celda o suprimir al final
// no junta dos celdas, y una selección entre celdas las vacía y deja la rejilla, como el
// navegador.
func TestLasTeclasNoSeSalenDeLaCelda(t *testing.T) {
	d := dosPorDos(t)
	if p := d.Retroceso(Pos{1, 0}); p != (Pos{1, 0}) {
		t.Errorf("borrar al principio de «b» mueve a %v", p)
	}
	if p := d.Suprimir(Pos{0, 1}); p != (Pos{0, 1}) {
		t.Errorf("suprimir al final de «a» mueve a %v", p)
	}
	quiero(t, d, `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`)

	d.Borrar(Pos{0, 0}, Pos{3, 1})
	quiero(t, d, `<table><tbody><tr><td><br/></td><td><br/></td></tr><tr><td><br/></td><td><br/></td></tr></tbody></table>`)

	// Un párrafo vacío detrás de una tabla sí se va, y el cursor queda al final de ella.
	e := editar(t, `<table><tbody><tr><td>a</td></tr></tbody></table><p><br/></p>`)
	if p := e.Retroceso(Pos{1, 0}); p != (Pos{0, 1}) {
		t.Errorf("borrar el párrafo de detrás deja el cursor en %v", p)
	}
	quiero(t, e, `<table><tbody><tr><td>a</td></tr></tbody></table>`)
}

// TestPegarEnUnaCelda: lo pegado se queda en la celda, y una tabla pegada dentro va
// entera (opaca), sin partir la de fuera, con su línea detrás para seguir escribiendo.
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin `enCelda`, lo pegado parte la tabla en
// dos y sale escrita dos veces.
func TestPegarEnUnaCelda(t *testing.T) {
	d := dosPorDos(t)
	d.InsertarDocumento(Pos{1, 1}, Leer("<p>uno</p><p>dos</p>"))
	quiero(t, d, `<table><tbody><tr><td>a</td><td>buno<p>dos</p></td></tr><tr><td>c</td><td>d</td></tr></tbody></table>`)

	d = dosPorDos(t)
	d.InsertarDocumento(Pos{1, 1}, Leer("<table><tbody><tr><td>x</td></tr></tbody></table>"))
	quiero(t, d, `<table><tbody><tr><td>a</td><td>b<table><tbody><tr><td>x</td></tr></tbody></table><div><br/></div></td></tr>`+
		`<tr><td>c</td><td>d</td></tr></tbody></table>`)

	// Y fuera de una tabla, lo de detrás del cursor no se mete en la última celda pegada.
	d = editar(t, "<p>antes|después</p>")
	d.InsertarDocumento(Pos{0, 6}, Leer("<table><tbody><tr><td>x</td></tr></tbody></table>"))
	quiero(t, d, `<p>antes|</p><table><tbody><tr><td>x</td></tr></tbody></table><p>después</p>`)
}
