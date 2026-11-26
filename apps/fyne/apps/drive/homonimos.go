// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Subir con un nombre que ya está (Sytue Office, unidades de la nube)
//
// En el disco, subir un fichero que se llama como otro lo sustituye guardando antes la
// versión anterior, como Drive. En una unidad de la nube se PREGUNTA, como hace Google
// Drive: «Sustituir» o «Guardar los dos» (el nuevo con el primer «nombre (n).ext» libre).
// Google Drive admite varios que se llaman igual en una carpeta: entonces «Sustituir» pide
// elegir cuál —cada uno con el nombre con que se ve, su tamaño y su fecha— y no se puede
// pulsar hasta que se elige; nunca se adivina. Lo pregunta solo lo que va a la carpeta
// abierta: lo que va dentro de una carpeta que se sube entera no se adivina tampoco
// (`local/archivos.Remotas.Upload` no sube si hay varios).

// decision es lo que eligió la persona para un fichero.
type decision struct {
	que int
	id  string // el que sustituye
}

const (
	noSubirlo = iota
	sustituirlo
	guardarAmbos
	sinPreguntar // no hay ventana: se sube como siempre
)

// decidirHomonimos pregunta por cada fichero que va a la carpeta abierta de una unidad de
// la nube con el nombre de otro; lo que la persona no quiere subir sale de la lista (y su
// flujo se cierra). Corre fuera de la interfaz: espera a cada respuesta.
func (p *Pantalla) decidirHomonimos(carpeta string, entradas []Entrada) []Entrada {
	out := make([]Entrada, 0, len(entradas))
	for _, e := range entradas {
		if e.Dir != "" {
			out = append(out, e)
			continue
		}
		ctx, cancelar := p.ctx()
		iguales, libre, err := p.EnEquipo.Homonimos(ctx, carpeta, e.nombre())
		cancelar()
		if err != nil || len(iguales) == 0 {
			out = append(out, e) // si no se sabe, subir no adivina: con varios, no sube
			continue
		}
		switch d := p.preguntarHomonimo(e.nombre(), iguales, libre); d.que {
		case sustituirlo:
			e.Sustituye = d.id
		case guardarAmbos:
			e.Como = libre
		case sinPreguntar:
		default:
			cerrar([]Entrada{e})
			continue
		}
		out = append(out, e)
	}
	return out
}

// preguntarHomonimo enseña el diálogo y espera a la respuesta. Corre fuera de la interfaz.
func (p *Pantalla) preguntarHomonimo(nombre string, iguales []Elemento, libre string) decision {
	if p.win == nil {
		return decision{que: sinPreguntar}
	}
	listo := make(chan decision, 1)
	enHilo(func() { p.dialogoHomonimo(nombre, iguales, libre, func(d decision) { listo <- d }) })
	return <-listo
}

// dialogoHomonimo es la pregunta. decidir se llama una sola vez: con un botón, o con el
// aspa, Escape o el velo, que son «No subirlo». Se llama en la rutina de la interfaz y
// devuelve el diálogo (para el guardián).
func (p *Pantalla) dialogoHomonimo(nombre string, iguales []Elemento, libre string, decidir func(decision)) *ui.Dialogo {
	v := p.variante()
	varios := len(iguales) > 1
	titulo, texto := "homonimoTitulo", "homonimoTexto"
	if varios {
		titulo, texto = "homonimosTitulo", "homonimosTexto"
	}
	elegido := ""
	if !varios {
		elegido = iguales[0].ID
	}
	var d *ui.Dialogo
	hecho := false
	cerrarCon := func(x decision) {
		if hecho {
			return
		}
		hecho = true
		d.Cerrar()
		decidir(x)
	}
	sustituir := ui.NuevoBoton(p.textoEquipo("sustituir"), func() {
		if elegido != "" {
			cerrarCon(decision{que: sustituirlo, id: elegido})
		}
	}).Con(ui.Primario, ui.TallaMD)
	sustituir.Inerte = elegido == ""
	ambos := ui.NuevoBoton(p.textoEquipo("guardarAmbos"), func() { cerrarCon(decision{que: guardarAmbos}) }).Con(ui.Secundario, ui.TallaMD)
	no := ui.NuevoBoton(p.textoEquipo("noSubir"), func() { cerrarCon(decision{que: noSubirlo}) }).Con(ui.Fantasma, ui.TallaMD)

	ancho := AnchoCuerpoDialogo(ui.DialogoMD, p.explor.Ventana)
	filas := []fyne.CanvasObject{parrafoEn(v, strings.ReplaceAll(p.textoEquipo(texto), "{free}", libre), ancho)}
	if varios {
		radios := make([]*ui.Eleccion, 0, len(iguales))
		for _, el := range iguales {
			var r *ui.Eleccion
			r = ui.NuevoRadio(p.etiquetaHomonimo(el), func(si bool) {
				if !si {
					return
				}
				elegido = el.ID
				for _, o := range radios {
					if o != r && o.Marcada {
						o.Marcada = false
						o.Refresh()
					}
				}
				sustituir.Inerte = false
				sustituir.Refresh()
			})
			r.AnchoEtiqueta = ancho - ui.LadoEleccion - ui.HuecoEleccion
			radios = append(radios, r)
			filas = append(filas, r)
		}
	}
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(3)}, filas...)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, no, ambos, sustituir)
	d = ui.NuevoDialogo(strings.ReplaceAll(p.textoEquipo(titulo), "{name}", nombre), cuerpo).Con(ui.DialogoMD, pie)
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.AlCerrar = func() { cerrarCon(decision{que: noSubirlo}) }
	d.Mostrar(p.win.Canvas())
	return d
}

// etiquetaHomonimo es cómo se distingue uno de los que se llaman igual: el nombre con que
// se ve, lo que ocupa y cuándo cambió.
func (p *Pantalla) etiquetaHomonimo(el Elemento) string {
	partes := []string{el.Nombre}
	if el.Tamano >= 0 {
		partes = append(partes, Tamano(p.tr.Locale(), el.Tamano))
	}
	if !el.Modificado.IsZero() {
		partes = append(partes, Fecha(p.tr.Locale(), el.Modificado))
	}
	return strings.Join(partes, " · ")
}
