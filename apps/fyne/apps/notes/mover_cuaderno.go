// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// ---- «Mover cuaderno a…» -----------------------------------------------------------
//
// Es el `destinosDeCuaderno`/`moverCuadernoA` de client/cuadernos.ts: cambiar un
// cuaderno de padre, o sacarlo a la raíz, sin arrastrar. La web lo ofrece en el «…» de
// cada fila del árbol y en el de la página; aquí, en el «…» de la página, que es donde
// Fyne tiene los ajustes del cuaderno abierto.

// ramaDe es el cuaderno y todo lo que cuelga de él, a cualquier nivel.
func ramaDe(cs []Cuaderno, id string) map[string]bool {
	rama := map[string]bool{id: true}
	for cambio := true; cambio; {
		cambio = false
		for _, c := range cs {
			if c.Padre != "" && rama[c.Padre] && !rama[c.ID] {
				rama[c.ID], cambio = true, true
			}
		}
	}
	return rama
}

// destinosDeCuaderno son los padres posibles: ni él ni su rama (sería un ciclo), ni uno
// de solo lectura, ni el padre que ya tiene (no movería nada).
func destinosDeCuaderno(cs []Cuaderno, id string) []Cuaderno {
	rama := ramaDe(cs, id)
	padre := ""
	for _, c := range cs {
		if c.ID == id {
			padre = c.Padre
		}
	}
	var out []Cuaderno
	for _, c := range cs {
		if !rama[c.ID] && !c.Crudo.ReadOnly && c.ID != padre {
			out = append(out, c)
		}
	}
	return out
}

// entradaMoverCuaderno es el cuerpo del PUT: el cuaderno ENTERO (`Crudo.Input()`, que
// conserva su cifrado) con el padre nuevo y detrás del último de sus nuevos hermanos,
// con el mismo hueco de 1024 que deja la web. `padre` vacío es la raíz. `false` si no
// hay nada que mover o no se puede.
func entradaMoverCuaderno(cs []Cuaderno, c Cuaderno, padre string) (api.NotebookInput, bool) {
	if c.Crudo.ReadOnly || c.Padre == padre || ramaDe(cs, c.ID)[padre] {
		return api.NotebookInput{}, false
	}
	pos, hay := 0, false
	for _, x := range cs {
		if x.Padre == padre && x.ID != c.ID && (!hay || x.Posicion > pos) {
			pos, hay = x.Posicion, true
		}
	}
	if hay {
		pos += 1024
	} else {
		pos = 1024
	}
	in := c.Crudo.Input()
	in.ParentID, in.Position = nil, &pos
	if padre != "" {
		in.ParentID = ptr(padre)
	}
	return in, true
}

// puedeMoverCuaderno: con red, de escritura, no un día del diario y quien lo administra
// —el servicio niega cambiar de padre a los demás—.
func (p *Pantalla) puedeMoverCuaderno(c Cuaderno) bool {
	return c.ID != "" && p.svc != nil && p.hayRed() && !c.Crudo.ReadOnly &&
		c.Crudo.DailyDate == "" && p.puedeAdministrarCuaderno(&c)
}

// dialogoMoverCuaderno es el MoverNotaModal de la web en su modo de cuaderno: el nivel
// superior primero (si no está ya en él) y los destinos con su sangría. Un cifrado es
// un padre más: el que se cuelga de él no se recifra.
func (p *Pantalla) dialogoMoverCuaderno(c Cuaderno) {
	v, t := p.variante(), p.tr.T
	candidatos := destinosDeCuaderno(cuadernosDelArbol(enArbol(p.cuadernos), p.esMio), c.ID)
	var d *ui.Dialogo
	mover := func(padre string) func() {
		return func() {
			d.Cerrar()
			in, ok := entradaMoverCuaderno(p.cuadernos, c, padre)
			if ok {
				lanzar(func() error { return p.guardarCuadernoCon(c.ID, in, "move.failed") })
			}
		}
	}
	var filas []fyne.CanvasObject
	if c.Padre != "" {
		raiz := Cuaderno{ID: "", Titulo: t("notebook.toRoot", nil)}
		filas = append(filas, p.filaMover(raiz, 0, mover(""), v))
	}
	for _, x := range candidatos {
		x.Cifrado = false // no se apaga: colgarse de un cifrado no toca la clave
		filas = append(filas, p.filaMover(x, profundidad(x, candidatos), mover(x.ID), v))
	}
	var cuerpo fyne.CanvasObject
	if len(filas) == 0 {
		cuerpo = lineaTok(t("notebook.noTarget", nil), tema.TextoSM, ui.Normal, "text-muted", v)
	} else {
		cuerpo = listaConFiletes(v, filas)
	}
	d = ui.NuevoDialogo(t("notebook.moveTo", nil), cuerpo).Con(ui.DialogoSM, nil)
	p.mostrar(d)
}
