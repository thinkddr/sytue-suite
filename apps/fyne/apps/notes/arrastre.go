// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
	"slices"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type cambioOrden struct {
	id             string
	antes, despues api.NotePosition
}

// planOrdenNotas conserva las agrupaciones y la sección de fijadas. En una lista
// se ordenan sus hermanas, incluidas las ocultas por el filtro; en columnas solo las
// de la columna de destino. Las posiciones son enteras y no empatan al recargar.
func planOrdenNotas(notas []Nota, id, destino string, despues bool, columna int) []cambioOrden {
	mi := slices.IndexFunc(notas, func(n Nota) bool { return n.ID == id })
	if mi < 0 || id == destino {
		return nil
	}
	movida := notas[mi]
	hermanas := hermanasDeArrastre(notas, movida, columna)
	en := len(hermanas)
	if destino != "" {
		en = slices.IndexFunc(hermanas, func(n Nota) bool { return n.ID == destino })
		if en < 0 {
			return nil
		}
		if despues {
			en++
		}
	}
	hermanas = slices.Insert(hermanas, en, movida)
	var cambios []cambioOrden
	for i, n := range hermanas {
		antes := posicionDe(n.Crudo)
		nueva := antes
		nueva.Position = i
		if n.ID == id && columna >= 0 {
			nueva.Col = columna
		}
		if antes.Col != nueva.Col || antes.Position != nueva.Position {
			cambios = append(cambios, cambioOrden{n.ID, antes, nueva})
		}
	}
	return cambios
}

func hermanasDeArrastre(notas []Nota, movida Nota, columna int) []Nota {
	var hermanas []Nota
	for _, n := range notas {
		if n.ID != movida.ID && n.Grupo == movida.Grupo && n.Fijada == movida.Fijada && (columna < 0 || n.Col == columna) {
			hermanas = append(hermanas, n)
		}
	}
	slices.SortStableFunc(hermanas, func(a, b Nota) int { return cmp.Compare(a.Posicion, b.Posicion) })
	return hermanas
}

func (p *Pantalla) reordenarNota(n Nota, destino string, despues bool, columna int) {
	if !p.puedeEscribir() || p.orden != OrdenManual {
		return
	}
	cambios := planOrdenNotas(p.notas, n.ID, destino, despues, columna)
	if len(cambios) == 0 {
		return
	}
	destinos := make([]posicionNota, len(cambios))
	for i, c := range cambios {
		destinos[i] = posicionNota{c.id, c.despues}
	}
	lanzar(func() error { return p.colocarVarias(destinos, true) })
}

func contienePunto(o fyne.CanvasObject, en fyne.Position) bool {
	if o == nil || !o.Visible() {
		return false
	}
	a, tam := fyne.CurrentApp().Driver().AbsolutePositionForObject(o), o.Size()
	return en.X >= a.X && en.Y >= a.Y && en.X <= a.X+tam.Width && en.Y <= a.Y+tam.Height
}

func (p *Pantalla) diaEnArrastre(en fyne.Position) int {
	if p.activo.Cifrado || p.dia.semana == nil {
		return -1
	}
	for i, o := range p.dia.semana.Objects {
		if i != 3 && contienePunto(o, en) {
			return i
		}
	}
	return -1
}

func (p *Pantalla) cuadernoEnArrastre(en fyne.Position) (Cuaderno, bool) {
	if p.activo.Cifrado || (p.anchoVentana() < ui.Estrecho && !p.cajon) {
		return Cuaderno{}, false
	}
	for _, c := range p.cuadernos {
		f := p.lateral.filasCuaderno[c.ID]
		if f != nil && c.ID != p.activo.ID && !c.Cifrado && !c.Crudo.ReadOnly && contienePunto(f, en) {
			return c, true
		}
	}
	return Cuaderno{}, false
}

func (p *Pantalla) notaEnArrastre(n Nota, en fyne.Position) (*Tarjeta, bool) {
	if p.orden != OrdenManual || p.desplazador == nil || !contienePunto(p.desplazador, en) {
		return nil, false
	}
	for _, t := range p.lienzo.vivas {
		if t.nota.ID == n.ID || t.nota.Grupo != n.Grupo || t.nota.Fijada != n.Fijada || !contienePunto(t, en) {
			continue
		}
		a := fyne.CurrentApp().Driver().AbsolutePositionForObject(t)
		return t, en.Y >= a.Y+t.Size().Height/2
	}
	return nil, false
}

func (p *Pantalla) columnaEnArrastre(en fyne.Position) int {
	if p.orden == OrdenManual && p.desplazador != nil && contienePunto(p.desplazador, en) {
		for i, o := range p.lienzo.columnas {
			if contienePunto(o, en) {
				return i
			}
		}
	}
	return -1
}

func (p *Pantalla) limpiarArrastreNota() {
	for _, t := range p.lienzo.vivas {
		if t.insercion != 0 {
			t.insercion = 0
			t.Refresh()
		}
	}
	for id, f := range p.lateral.filasCuaderno {
		if activa := id == p.lateral.Activo; f.Activa != activa {
			f.Activa = activa
			f.Refresh()
		}
	}
	for _, o := range p.dia.marcasDestino {
		o.Hide()
	}
	for _, o := range p.lienzo.marcasColumnas {
		o.Hide()
	}
}

func (p *Pantalla) marcarArrastreNota(n Nota, en fyne.Position) {
	p.marcarDestinoNota(n, en)
	s := p.desplazador
	if !p.puedeEscribir() || s == nil || !contienePunto(s, en) {
		p.autoArrastre.Parar()
		return
	}
	p.autoArrastre.Actualizar(s, en, func(_, dy float32) {
		s.ScrollToOffset(fyne.NewPos(s.Offset.X, s.Offset.Y+dy))
		p.marcarDestinoNota(n, en)
	})
}

func (p *Pantalla) marcarDestinoNota(n Nota, en fyne.Position) {
	p.limpiarArrastreNota()
	if !p.puedeEscribir() {
		return
	}
	if i := p.diaEnArrastre(en); i >= 0 {
		if i < len(p.dia.marcasDestino) {
			p.dia.marcasDestino[i].Show()
		}
		return
	}
	if c, ok := p.cuadernoEnArrastre(en); ok {
		p.lateral.filasCuaderno[c.ID].Activa = true
		p.lateral.filasCuaderno[c.ID].Refresh()
		return
	}
	if t, despues := p.notaEnArrastre(n, en); t != nil {
		t.insercion = -1
		if despues {
			t.insercion = 1
		}
		t.Refresh()
	} else if col := p.columnaEnArrastre(en); col >= 0 {
		p.lienzo.marcasColumnas[col].Show()
	}
}

func (p *Pantalla) soltarNota(n Nota, en fyne.Position) {
	p.autoArrastre.Parar()
	defer p.limpiarArrastreNota()
	if !p.puedeEscribir() {
		return
	}
	if p.diaEnArrastre(en) >= 0 {
		p.soltarEnDia(n, en)
		return
	}
	if c, ok := p.cuadernoEnArrastre(en); ok {
		lanzar(func() error { return p.moverACuaderno(n.ID, c, true) })
		return
	}
	columna := -1
	if p.activo.Vista == Columnas {
		columna = p.columnaEnArrastre(en)
	}
	if t, despues := p.notaEnArrastre(n, en); t != nil {
		p.reordenarNota(n, t.nota.ID, despues, columna)
	} else if columna >= 0 {
		p.reordenarNota(n, "", false, columna)
	}
}

func (p *Pantalla) cancelarArrastreNota() {
	p.autoArrastre.Parar()
	for _, t := range p.lienzo.vivas {
		t.cancelarArrastre()
	}
	p.limpiarArrastreNota()
}
