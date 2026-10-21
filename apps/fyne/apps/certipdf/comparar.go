// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// dialogoComparar es `Comparar`: la ruta del otro documento y, al pulsar, lo que cambió
// en el texto de cada página.
type dialogoComparar struct {
	a    *dialogoAbierto
	otro *ui.Entrada
	// modo es «texto» (el defecto, también con el valor cero) o «visual».
	modo string
	// v es el estado del modo visual (ver [estadoVisual]); nil hasta que se usa.
	v *estadoVisual
	// resultado es nil hasta la primera comparación que llega bien.
	resultado  *Comparacion
	trabajando bool
	fallo      bool
}

// modosDeComparar son las pestañas, en su orden.
var modosDeComparar = []struct{ id, clave string }{
	{"texto", "pdf.dialog.comparar.modo.texto"},
	{"visual", "pdf.dialog.comparar.modo.visual"},
}

// vista es el diálogo: la tira de pestañas y, debajo, el modo elegido.
func (d *dialogoComparar) vista(ancho float32) fyne.CanvasObject {
	cuerpo := d.vistaTexto
	if d.modo == "visual" {
		cuerpo = d.vistaVisual
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, d.pestanas(ancho), cuerpo(ancho))
}

// pestanas es la tira `flex gap-1 border-b pb-2` de los dos modos, la misma que la de
// Firmar.
func (d *dialogoComparar) pestanas(ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	botones := make([]*Accion, len(modosDeComparar))
	for i, m := range modosDeComparar {
		id := m.id
		b := NuevaAccion(d.a.t(m.clave), func() { d.modo = id; d.a.rehacer() })
		b.PX, b.PY = tema.Espacio(2.5), tema.Espacio(1)
		if m.id == d.modo || (m.id == "texto" && d.modo == "") {
			b.Fondo, b.Tinta, b.FondoEncima = "selected", "primary-text", ""
		} else {
			b.Tinta, b.TintaEncima = "text-muted", "text"
		}
		botones[i] = b
	}
	return container.New(conLineaAbajo{}, canvas.NewRectangle(tema.Color("border", v)),
		container.New(ui.Margen{Abj: tema.Espacio(2)}, repartirFila(botones, -1, ancho, tema.Espacio(1))))
}

// vistaTexto es la pestaña «Texto»: la ruta del otro documento y lo que cambió en el texto.
func (d *dialogoComparar) vistaTexto(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	comparar := botonPrimario(a.t("pdf.dialog.comparar.run"), tema.TextoXS, tema.Espacio(3), tema.Espacio(1.5), d.comparar)
	comparar.Inerte = d.trabajando
	hueco := tema.Espacio(2)
	d.entrada(d.comparar)
	// `flex gap-2`: el campo `flex-1` y el botón estirado al alto del campo.
	d.otro.Ancho = ancho - hueco - comparar.MinSize().Width
	comparar.Alto = d.otro.MinSize().Height
	partes := []fyne.CanvasObject{
		ui.NuevoParrafo(a.t("pdf.dialog.comparar.help"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho),
		container.New(ui.Fila{Hueco: hueco}, d.otro, comparar),
	}
	if d.fallo {
		partes = append(partes, ui.NuevoParrafo(a.t("pdf.dialog.comparar.error"), tema.TextoXS, ui.Normal, "danger-text", v).A(ancho))
	}
	if r := d.resultado; r != nil {
		if r.Iguales {
			partes = append(partes, ui.NuevoParrafo(a.t("pdf.dialog.comparar.same"), tema.TextoXS, ui.Normal, "success-text", v).A(ancho))
		}
		dentro := ancho - 2*tema.Borde - 2*tema.Espacio(2)
		for _, dif := range r.Diferencias {
			lineas := []fyne.CanvasObject{ui.NuevoRotulo(a.p.tv("pdf.page.n", i18n.Vars{"n": dif.Pagina}), tema.TextoXS, ui.Medio, "text")}
			for _, l := range dif.Quitado {
				lineas = append(lineas, ui.NuevoParrafo("− "+l, tema.Texto2XS, ui.Normal, "danger-text", v).A(dentro))
			}
			for _, l := range dif.Anadido {
				lineas = append(lineas, ui.NuevoParrafo("+ "+l, tema.Texto2XS, ui.Normal, "success-text", v).A(dentro))
			}
			partes = append(partes, nuevoRecuadroDe(container.New(ui.Columna{}, lineas...), ancho))
		}
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// comparar pide la comparación; lo que falle —la ruta no existe, no es un PDF— se dice
// con una sola frase, como en la web.
func (d *dialogoComparar) comparar() {
	p := d.a.p
	otra := d.otro.Texto()
	if d.trabajando || p.doc == nil || (d.otro != nil && !d.otro.Requerida()) || strings.TrimSpace(otra) == "" {
		return
	}
	d.trabajando, d.fallo = true, false
	d.a.rehacer()
	doc := p.doc
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		r, err := doc.Comparar(ctx, otra)
		fyne.Do(func() {
			defer p.avisarRed()
			d.trabajando = false
			if err != nil {
				d.fallo = true
			} else {
				d.resultado = &r
			}
			d.a.rehacer()
		})
	}()
}
