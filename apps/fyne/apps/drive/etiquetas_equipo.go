// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Etiquetas libres (Sytue Office, DR-09)
//
// En el equipo no hay organización ni taxonomía de sensibilidad: en el hueco de
// «Clasificar» va «Etiquetas», que son las de la persona, varias por elemento y también
// en carpetas. El diálogo es el de clasificar —el nombre y una fila por etiqueta con la
// marca de las que lleva— más un campo para escribir una nueva. Cada pulsación se guarda
// en el momento (en el índice de la carpeta, `local/archivos`): no hay «Guardar» que
// olvidar.

// EtiquetasEnEquipo son las etiquetas libres del equipo. Las tres funciones se llaman
// fuera de la interfaz.
type EtiquetasEnEquipo struct {
	// De son las que lleva un elemento.
	De func(ctx context.Context, id string) ([]string, error)
	// Todas son las que se usan en algún elemento: lo que se ofrece para marcar.
	Todas func(ctx context.Context) []string
	// Poner sustituye las de un elemento.
	Poner func(ctx context.Context, id string, etiquetas []string) error
}

// dialogoEtiquetas es el estado del diálogo.
type dialogoEtiquetas struct {
	p       *Pantalla
	el      Elemento
	d       *ui.Dialogo
	cuerpo  *fyne.Container
	campo   *ui.Entrada
	todas   []string
	puestas []string
	cargado bool
	ocupado bool
	ancho   float32
}

// pedirEtiquetas abre el diálogo de etiquetas de un elemento.
func (p *Pantalla) pedirEtiquetas(el Elemento) {
	if p.win == nil || p.EnEquipo == nil || p.EnEquipo.Etiquetas == nil || el.ID == "" {
		return
	}
	de := &dialogoEtiquetas{p: p, el: el, cuerpo: container.NewStack()}
	de.ancho = AnchoCuerpoDialogo(ui.DialogoSM, p.explor.Ventana)
	de.campo = ui.NuevaEntrada(p.textoEquipo("etiquetaNueva"))
	de.campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			de.anadir()
			return true
		}
		return false
	}
	anadir := ui.NuevoBoton(p.textoEquipo("etiquetaAnadir"), de.anadir).Con(ui.Primario, ui.TallaMD)
	cerrar := ui.NuevoBoton(p.tr.T("ui.close", nil), func() { de.d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cerrar, anadir)
	de.d = ui.NuevoDialogo(p.textoEquipo("etiquetas"), de.cuerpo).Con(ui.DialogoSM, pie)
	de.d.RotuloCerrar = p.tr.T("ui.close", nil)
	de.pintar()
	de.d.Mostrar(p.win.Canvas())
	et := p.EnEquipo.Etiquetas
	p.enSegundoPlano(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		puestas, err := et.De(ctx, el.ID)
		todas := et.Todas(ctx)
		enHilo(func() {
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.textoEquipo("etiquetasFallo"), el.Nombre)
			}
			de.puestas, de.todas, de.cargado = puestas, todas, true
			de.pintar()
		})
	})
}

// lleva dice si el elemento lleva la etiqueta (sin distinguir mayúsculas).
func (de *dialogoEtiquetas) lleva(l string) bool {
	return slices.ContainsFunc(de.puestas, func(x string) bool { return strings.EqualFold(x, l) })
}

// alternar pone o quita una etiqueta y lo guarda. Se llama en la rutina de la interfaz.
func (de *dialogoEtiquetas) alternar(l string) {
	nuevas := slices.Clone(de.puestas)
	if de.lleva(l) {
		nuevas = slices.DeleteFunc(nuevas, func(x string) bool { return strings.EqualFold(x, l) })
	} else {
		nuevas = append(nuevas, l)
	}
	de.guardar(nuevas)
}

// anadir pone la etiqueta escrita en el campo.
func (de *dialogoEtiquetas) anadir() {
	l := strings.Join(strings.Fields(de.campo.Texto()), " ")
	if l == "" || !de.cargado {
		return
	}
	de.campo.PonerTexto("")
	if de.lleva(l) {
		return
	}
	if !slices.ContainsFunc(de.todas, func(x string) bool { return strings.EqualFold(x, l) }) {
		de.todas = append(de.todas, l)
	}
	de.guardar(append(slices.Clone(de.puestas), l))
}

// guardar escribe las etiquetas y vuelve a leer las que quedaron (el índice las limpia).
func (de *dialogoEtiquetas) guardar(nuevas []string) {
	if de.ocupado {
		return
	}
	de.ocupado = true
	de.pintar()
	et, id := de.p.EnEquipo.Etiquetas, de.el.ID
	de.p.enSegundoPlano(func() {
		ctx, cancelar := de.p.ctx()
		defer cancelar()
		err := et.Poner(ctx, id, nuevas)
		puestas, _ := et.De(ctx, id)
		enHilo(func() {
			de.ocupado = false
			if err != nil {
				de.p.avisar(ui.AvisoPeligro, de.p.textoEquipo("etiquetasFallo"), de.el.Nombre)
			} else {
				de.puestas = puestas
			}
			de.pintar()
		})
	})
}

// opciones son todas las conocidas más las que lleva, sin repetir y en orden.
func (de *dialogoEtiquetas) opciones() []string {
	out := slices.Clone(de.todas)
	for _, l := range de.puestas {
		if !slices.ContainsFunc(out, func(x string) bool { return strings.EqualFold(x, l) }) {
			out = append(out, l)
		}
	}
	slices.SortFunc(out, func(x, y string) int { return strings.Compare(strings.ToLower(x), strings.ToLower(y)) })
	return out
}

// pintar rehace el cuerpo: el nombre, la lista (o la rueda, o la ayuda si no hay
// ninguna) y el campo.
func (de *dialogoEtiquetas) pintar() {
	v := de.p.variante()
	nombre := ui.Texto(de.el.Nombre, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	ui.Truncar(nombre, de.el.Nombre, tema.TextoSM, ui.Normal, de.ancho)
	var dentro fyne.CanvasObject
	ops := de.opciones()
	switch {
	case !de.cargado:
		dentro = container.New(&altoFijo{alto: AltoCargaEtiquetas}, container.NewCenter(ui.NuevaRueda(20, "text", v)))
	case len(ops) == 0:
		dentro = ui.AlAncho(de.ancho, ui.NuevoParrafo(de.p.textoEquipo("etiquetasVacio"), tema.TextoSM, ui.Normal, "text-muted", v))
	default:
		filas := make([]fyne.CanvasObject, 0, len(ops))
		separas := make([]float32, 0, len(ops))
		for i, l := range ops {
			f := nuevaFilaEtiqueta(l, "", func() { de.alternar(l) })
			f.marcada, f.Inerte = de.lleva(l), de.ocupado
			filas = append(filas, f)
			sep := tema.Espacio(1)
			if i == 0 {
				sep = 0
			}
			separas = append(separas, sep)
		}
		dentro = container.New(&pilaSeparada{separas: separas}, filas...)
	}
	de.cuerpo.Objects = []fyne.CanvasObject{container.New(&pilaSeparada{separas: []float32{0, SeparaNombreEtiq, SeparaNombreEtiq}},
		lineaTexto(nombre, tema.TextoSM), dentro, de.campo)}
	de.cuerpo.Refresh()
	de.d.Recolocar()
}
