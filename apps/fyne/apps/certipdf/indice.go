// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
El ÍNDICE en edición (`Indice` de `Paneles.tsx`, con `borrador !== null`): cada marcador
con su título, su página, sangrar, quitar sangría y quitarlo; «Añadir aquí» con la página
que se está mirando; y Guardar, que deja COPIA —perder el índice que traía un documento
ajeno no se recupera de otra forma—, dicho antes de pulsar.

Editar arranca con una COPIA del índice: cancelar lo deja como estaba sin haber tocado el
documento.
*/

// panelIndice es el borrador. Nil es el modo lectura.
type panelIndice struct {
	l        *PanelLateral
	borrador []Marcador
}

// editarIndice abre la edición. Un marcador sin destino (`pagina: -1`) no se puede
// escribir: se le pone la primera página en vez de dejarlo caer al guardar.
func (l *PanelLateral) editarIndice() {
	b := make([]Marcador, len(l.c.Indice))
	for i, m := range l.c.Indice {
		m.Pagina = max(m.Pagina, 0)
		b[i] = m
	}
	l.indiceP = &panelIndice{l: l, borrador: b}
	l.Recomponer()
}

// PuedeSangrar es la regla de `⇥`: el primero no puede sangrarse —un hijo sin padre
// encima no existe en el árbol— y nadie baja más de un nivel por debajo de su anterior.
func PuedeSangrar(b []Marcador, i int) bool {
	return i > 0 && i < len(b) && b[i].Nivel <= b[i-1].Nivel
}

// LimpiarBorrador quita los marcadores sin título: lo que se guarda es lo que se ve.
func LimpiarBorrador(b []Marcador) []Marcador {
	out := make([]Marcador, 0, len(b))
	for _, m := range b {
		if strings.TrimSpace(m.Titulo) != "" {
			out = append(out, m)
		}
	}
	return out
}

func (x *panelIndice) t(clave string) string { return x.l.t(clave, nil) }

// edicion es el `<div class="flex flex-col gap-1 p-1">` del borrador.
func (x *panelIndice) edicion() fyne.CanvasObject {
	l := x.l
	ancho := anchoUtil() - 2*tema.Espacio(1)
	filas := make([]fyne.CanvasObject, 0, len(x.borrador))
	for i := range x.borrador {
		filas = append(filas, x.fila(i, ancho))
	}
	actual := 1
	if l.pant != nil {
		actual = l.pant.PaginaActual
	}
	anadir := botonPanel(l.t("pdf.outline.addHere", i18n.Vars{"n": strconv.Itoa(actual)}), tema.TextoXS, tema.Espacio(1.5), func() {
		// El nuevo hereda el nivel del anterior: al listar los capítulos de una sección
		// seguida, sangrar cada uno a mano sería el trabajo.
		nivel := 0
		if n := len(x.borrador); n > 0 {
			nivel = x.borrador[n-1].Nivel
		}
		x.borrador = append(x.borrador, Marcador{Pagina: actual - 1, Nivel: nivel})
		l.Recomponer()
	})
	anadir.Peso, anadir.FondoEncima, anadir.Ancho = ui.Medio, "bg-subtle", ancho

	cancelar := botonPanel(x.t("pdf.outline.cancel"), tema.TextoXS, tema.Espacio(1.5), func() {
		l.indiceP = nil
		l.Recomponer()
	})
	cancelar.FondoEncima = "bg-subtle"
	guardar := botonPrimario(x.t("pdf.outline.save"), tema.TextoXS, tema.Espacio(2), tema.Espacio(1.5), x.guardar)
	guardar.Inerte = l.trabajando()
	// `flex gap-1` con Guardar en `flex-1`.
	guardar.Ancho = ancho - tema.Espacio(1) - cancelar.MinSize().Width
	botones := container.New(ui.Fila{Hueco: tema.Espacio(1)}, guardar, cancelar)

	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	nota := container.New(ui.Margen{Izq: tema.Espacio(1), Der: tema.Espacio(1)},
		ui.NuevoParrafo(x.t("pdf.outline.savesCopy"), tema.Texto2XS, ui.Normal, "text-subtle", v).A(ancho-2*tema.Espacio(1)))
	return container.New(ui.Margen{Izq: tema.Espacio(1), Arr: tema.Espacio(1), Der: tema.Espacio(1), Abj: tema.Espacio(1)},
		container.New(ui.Columna{Hueco: tema.Espacio(1)},
			container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...), anadir, botones, nota))
}

// fila es `<li class="flex items-center gap-1">` con la sangría del nivel (0,6 rem por
// nivel, topada en 6): título en `flex-1`, página en `w-12` y los tres botones.
func (x *panelIndice) fila(i int, ancho float32) fyne.CanvasObject {
	l := x.l
	m := x.borrador[i]
	sangria := float32(min(max(m.Nivel, 0), 6)) * 0.6 * tema.Rem
	propio := ui.CampoPropio{MargenX: tema.Espacio(1.5), MargenY: tema.Espacio(1), Texto: tema.TextoXS, Borde: "border"}

	pagina := ui.NuevaEntrada("").ConPropia(ui.CampoPropio{MargenX: tema.Espacio(1), MargenY: tema.Espacio(1), Texto: tema.TextoXS, Borde: "border"})
	pagina.Radio, pagina.Ancho = 0.25*tema.Rem, tema.Espacio(12)
	pagina.PonerTexto(strconv.Itoa(m.Pagina + 1))
	pagina.AlCambiar(func(s string) {
		total := max(l.c.Total, 1)
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			x.borrador[i].Pagina = min(max(n, 1), total) - 1
		}
	})

	fuera := x.botonFila(iconos.EdMenosSangria, "text-muted", x.t("pdf.outline.outdent"), func() {
		x.borrador[i].Nivel = max(x.borrador[i].Nivel-1, 0)
		l.Recomponer()
	})
	fuera.Inerte = m.Nivel == 0
	dentro := x.botonFila(iconos.EdMasSangria, "text-muted", x.t("pdf.outline.indent"), func() {
		x.borrador[i].Nivel = min(x.borrador[i].Nivel+1, x.borrador[i-1].Nivel+1)
		l.Recomponer()
	})
	dentro.Inerte = !PuedeSangrar(x.borrador, i)
	quitar := x.botonFila(iconos.EdCerrar, "danger-text", x.t("pdf.outline.remove"), func() {
		x.borrador = append(x.borrador[:i:i], x.borrador[i+1:]...)
		l.Recomponer()
	})

	botones := fuera.MinSize().Width + dentro.MinSize().Width + quitar.MinSize().Width
	titulo := ui.NuevaEntrada(x.t("pdf.outline.titlePlaceholder")).ConPropia(propio)
	titulo.Radio = 0.25 * tema.Rem
	titulo.Ancho = max(ancho-sangria-pagina.MinSize().Width-botones-4*tema.Espacio(1), 0)
	titulo.PonerTexto(m.Titulo)
	titulo.AlCambiar(func(s string) { x.borrador[i].Titulo = s })

	return container.New(ui.Margen{Izq: sangria}, container.New(filaCentrada{hueco: tema.Espacio(1)}, titulo, pagina, fuera, dentro, quitar))
}

// botonFila es `rounded px-1.5 py-1 text-xs hover:bg-bg-subtle disabled:opacity-30`.
//
// ponytail: la web pone los caracteres ⇤, ⇥ y ✕ con la fuente del sistema, que Figtree no
// tiene; aquí van los glifos del catálogo (sangría y aspa) en la caja de línea web.
func (x *panelIndice) botonFila(i iconos.Ed, tinta, rotulo string, alPulsar func()) *Accion {
	a := NuevaAccion("", alPulsar)
	a.Nombre = rotulo
	a.PX, a.PY, a.Radio, a.Tinta = tema.Espacio(1.5), tema.Espacio(1), 0.25*tema.Rem, tinta
	a.IconoEd, a.TamIcono, a.Opacidad = i, 12, 0.3
	// Estos botones no llevan `border`: ancho del carácter más `px-1.5` a ambos lados.
	// Paneles.tsx real, Chromium con Figtree cargada y raíz 16: la fuente de
	// sustitución da 12,15625 px a ⇤/⇥ y 10,375 px a ✕ en text-xs (12 px).
	// Figtree no tiene estos caracteres: ui.Medir no mide el glifo de la web.
	avance := float32(12.15625)
	if i == iconos.EdCerrar {
		avance = 10.375
	}
	a.Ancho = avance + 2*a.PX
	return a
}

// guardar escribe el índice (`op: indice`) en una copia.
func (x *panelIndice) guardar() {
	l := x.l
	if l.pant == nil {
		return
	}
	limpio := LimpiarBorrador(x.borrador)
	l.pant.Aplicar(api.PDFCambio{Op: "indice", Datos: limpio, Sufijo: x.t("pdf.suffix.outline")},
		x.t("pdf.saved.outline"), func(ok bool) {
			if ok {
				l.indiceP = nil
				l.Recomponer()
			}
		})
}
