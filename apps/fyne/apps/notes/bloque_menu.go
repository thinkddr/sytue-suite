// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// EL ASA DE CADA BLOQUE (⋮⋮) Y SU MENÚ: con el ratón encima de una línea, a su izquierda
// sale el asa; pulsarla abre lo que se hace con el bloque entero —subir, bajar, duplicar,
// eliminar y «Convertir en…»—. Con el dedo no hay «encima»: las mismas acciones van en el
// menú de mantener pulsado (editor_menu.go).
//
// Cada acción es un paso de deshacer. Un bloque de una tabla no se mueve ni se duplica
// solo (rompería la rejilla): sus acciones son las de la barra de tabla.
//
// Y arrastrar el asa lleva el bloque (o la tabla, o las columnas, enteras) a otro sitio:
// una raya en el color de marca dice entre qué bloques caerá (`.notes-soltar` de la web),
// y soltarlo es un paso de deshacer (arrastre_bloque.go).

var _ desktop.Hoverable = (*zonaEdicion)(nil)

// Medidas del asa: el glifo `drag` de 12 en el `px-3` de la izquierda del cuerpo.
const ladoAsaBloque float32 = 12

// MouseIn y MouseMoved apuntan qué bloque está debajo del ratón, para su asa.
func (z *zonaEdicion) MouseIn(ev *desktop.MouseEvent) { z.MouseMoved(ev) }

func (z *zonaEdicion) MouseMoved(ev *desktop.MouseEvent) {
	if ui.EsTactil() {
		return
	}
	b := -1
	if p, ok := z.posEn(ev.Position); ok && !z.e.doc.Bloques[p.B].Celda.EnTabla() {
		b = p.B
	}
	if b != z.encima {
		z.encima = b
		z.Refresh()
	}
}

// MouseOut quita el asa.
func (z *zonaEdicion) MouseOut() {
	if z.encima >= 0 {
		z.encima = -1
		z.Refresh()
	}
}

// cajaAsa es dónde va el asa del bloque `b`, en el sistema de la zona: a la izquierda de
// su primera línea, centrada en su alto.
func (z *zonaEdicion) cajaAsa(b int) (fyne.Position, bool) {
	m := z.maquetado()
	for _, l := range m.Lineas {
		if l.Bloque == b {
			return fyne.NewPos(0, PYZonaEd+l.Y+(l.Alto-ladoAsaBloque)/2), true
		}
	}
	return fyne.Position{}, false
}

// asa es el glifo del asa del bloque bajo el ratón, o nil.
func (z *zonaEdicion) asa(v tema.Variante) fyne.CanvasObject {
	if z.encima < 0 || z.encima >= len(z.e.doc.Bloques) || ui.EsTactil() {
		return nil
	}
	p, ok := z.cajaAsa(z.encima)
	if !ok {
		return nil
	}
	d := iconos.TrazoDeRuta(AsaD, ladoAsaBloque, tema.Color("text-subtle", v))
	d.Colocar(p.X, p.Y, ladoAsaBloque)
	return d.Objeto()
}

// tocarAsa abre el menú del bloque si el toque cae en su asa. Dice si era ahí.
func (z *zonaEdicion) tocarAsa(pos fyne.Position) bool {
	if z.encima < 0 || ui.EsTactil() {
		return false
	}
	p, ok := z.cajaAsa(z.encima)
	if !ok || pos.X < p.X-2 || pos.X > p.X+PXZonaEd || pos.Y < p.Y-4 || pos.Y > p.Y+ladoAsaBloque+4 {
		return false
	}
	b := z.encima
	z.e.cur = Pos{b, inicioEditable(z.e.doc.Bloques[b])}
	z.e.ancla = z.e.cur
	z.e.menuDeBloque(z.anclaEn(fyne.NewPos(p.X, p.Y+ladoAsaBloque)))
	return true
}

// opcionesDeBloque son las acciones sobre el bloque del cursor.
func (e *EditorNota) opcionesDeBloque() []*opcionMenu {
	t := func(k string) string { return e.tr.T(k, nil) }
	b := e.cur.B
	if len(e.doc.Bloques) == 0 || e.doc.Bloques[b].Celda.EnTabla() {
		return nil
	}
	mover := func(paso int) func() {
		return func() { e.hacer(func() { e.cur = e.doc.MoverBloque(e.cur, paso); e.ancla = e.cur }) }
	}
	return []*opcionMenu{
		{icono: iconos.ChevronArriba, rotulo: t("editor.moveUp"), hacer: mover(-1)},
		{icono: iconos.ChevronAbajo, rotulo: t("editor.moveDown"), hacer: mover(1)},
		{icono: iconos.Copias, rotulo: t("editor.duplicate"), hacer: func() {
			e.hacer(func() { e.cur = e.doc.DuplicarBloque(e.cur); e.ancla = e.cur })
		}},
		{icono: iconos.Papelera, rotulo: t("editor.delete"), hacer: func() {
			e.hacer(func() { e.cur = e.doc.BorrarBloque(e.cur); e.ancla = e.cur })
		}},
		nil,
		{glifo: TituloD, rotulo: t("editor.convertTo") + "…", hacer: func() {
			e.menuConvertir(e.disparadorDe("bloque"))
		}},
	}
}

// menuDeBloque abre las acciones del bloque del cursor colgando de `disparador`.
func (e *EditorNota) menuDeBloque(disparador fyne.CanvasObject) {
	if o := e.opcionesDeBloque(); len(o) > 0 {
		e.mostrarMenu(disparador, e.tr.T("editor.blockMenu", nil), o)
	}
}

// menuConvertir es «Convertir en…»: los bloques en que se puede convertir el del cursor.
func (e *EditorNota) menuConvertir(disparador fyne.CanvasObject) {
	t := func(k string) string { return e.tr.T(k, nil) }
	bloque := func(glifo string, icono iconos.Icono, clave, valor string) *opcionMenu {
		return &opcionMenu{glifo: glifo, icono: icono, rotulo: t(clave), hacer: func() { e.Orden("bloque", valor) }}
	}
	e.mostrarMenu(disparador, t("editor.convertTo"), []*opcionMenu{
		bloque("", iconos.Texto, "editor.paragraph", "p"),
		bloque(TituloD, iconos.Ninguno, "editor.heading1", "h1"),
		bloque(TituloD, iconos.Ninguno, "editor.heading2", "h2"),
		bloque(TituloD, iconos.Ninguno, "editor.heading3", "h3"),
		{glifo: VinetasD, rotulo: t("editor.bulletList"), hacer: func() { e.Orden("vinetas", "") }},
		{glifo: NumerosD, rotulo: t("editor.numberedList"), hacer: func() { e.Orden("numeros", "") }},
		{glifo: TareaD, rotulo: t("editor.taskList"), hacer: func() { e.Orden("tarea", "") }},
		bloque(CitaD, iconos.Ninguno, "editor.quote", "blockquote"),
		bloque("", iconos.Codigo, "editor.codeBlock", "pre"),
	})
}

// unidad son los bloques [i, j) que se mueven juntos con el `b`: él solo, o la tabla (o
// las columnas) entera si es de una.
func (d *Documento) unidad(b int) (int, int) {
	if d.Bloques[b].Celda.EnTabla() {
		return d.rangoTabla(b)
	}
	return b, b + 1
}

// MoverBloque sube (paso −1) o baja (+1) el bloque del cursor por encima del bloque (o
// la tabla) de al lado, y devuelve dónde queda el cursor, que va con él.
func (d *Documento) MoverBloque(p Pos, paso int) Pos {
	if len(d.Bloques) == 0 || d.Bloques[p.B].Celda.EnTabla() {
		return p
	}
	b := d.Bloques[p.B]
	switch {
	case paso < 0 && p.B > 0:
		i, _ := d.unidad(p.B - 1)
		d.Bloques = slices.Delete(d.Bloques, p.B, p.B+1)
		d.Bloques = slices.Insert(d.Bloques, i, b)
		return Pos{i, p.O}
	case paso > 0 && p.B+1 < len(d.Bloques):
		_, j := d.unidad(p.B + 1)
		d.Bloques = slices.Insert(d.Bloques, j, b)
		d.Bloques = slices.Delete(d.Bloques, p.B, p.B+1)
		return Pos{j - 1, p.O}
	}
	return p
}

// DuplicarBloque pone una copia del bloque del cursor debajo de él y el cursor en ella.
func (d *Documento) DuplicarBloque(p Pos) Pos {
	if len(d.Bloques) == 0 || d.Bloques[p.B].Celda.EnTabla() {
		return p
	}
	b := d.Bloques[p.B]
	b.Tramos = slices.Clone(b.Tramos)
	d.Bloques = slices.Insert(d.Bloques, p.B+1, b)
	return Pos{p.B + 1, p.O}
}

// BorrarBloque quita el bloque del cursor entero. Si era el único, deja una línea vacía.
func (d *Documento) BorrarBloque(p Pos) Pos {
	if len(d.Bloques) == 0 || d.Bloques[p.B].Celda.EnTabla() {
		return p
	}
	d.Bloques = slices.Delete(d.Bloques, p.B, p.B+1)
	if len(d.Bloques) == 0 {
		d.Bloques = []Bloque{{Tipo: Parrafo, Etiqueta: "p"}}
	}
	b := min(p.B, len(d.Bloques)-1)
	return Pos{b, 0}
}
