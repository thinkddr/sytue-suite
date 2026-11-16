// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El panel derecho «Ajustes» de Docus: `panels/SettingsPanel.tsx`. Con el cursor en un
// párrafo, el espaciado antes y después (pt), el interlineado (múltiplo) y las sangrías
// izquierda, derecha y de primera línea (cm), cada uno con −/+ y entrada libre; con el
// cursor en una tabla, sus bordes, la cabecera alternada y el ancho automático. Es el
// «equivalente» de los marcadores de la regla que pide WCAG 2.5.8 (docs/design.md §2.6.1).
//
// La web escribe pt y cm; aquí se escriben en px (`margin-bottom:16px` por 12 pt), que es
// lo único que relee `rico` y lo mismo que escriben la sangría y la regla. Los valores
// leídos coinciden con los de `getComputedStyle` de la web para el mismo HTML.

const (
	pxPorPt = 96.0 / 72   // `PX_POR_PT`
	pxPorCm = 96.0 / 2.54 // `PX_POR_CM`
)

// pasoAjuste es un `Stepper` de la web: sus topes, su paso y cómo escribe su valor.
type pasoAjuste struct {
	clave          string // la de su etiqueta
	min, max, paso float64
	decimales      int
	leer           func(rico.Bloque) float64
	prop           string
	css            func(float64) string
	menos, mas     *ui.BotonIcono
	entrada        *ui.Entrada
	valor          float64
}

// Los seis pasos de `SeccionParrafo`, con sus topes y pasos.
func pasosParrafo() []*pasoAjuste {
	px := func(f float64) func(float64) string {
		return func(n float64) string { return cssPx(n * f) }
	}
	return []*pasoAjuste{
		{
			clave: "settings.spacing.before", max: 96, paso: 6, decimales: 1, prop: "margin-top", css: px(pxPorPt),
			leer: func(b rico.Bloque) float64 { return float64(b.Arriba) / pxPorPt },
		},
		{
			clave: "settings.spacing.after", max: 96, paso: 6, decimales: 1, prop: "margin-bottom", css: px(pxPorPt),
			leer: func(b rico.Bloque) float64 { return float64(b.Abajo) / pxPorPt },
		},
		{
			clave: "settings.spacing.lineHeight", min: 1, max: 3, paso: 0.1, decimales: 2, prop: "line-height",
			css:  func(n float64) string { return numero(n, 2) },
			leer: func(b rico.Bloque) float64 { return float64(b.Base.Interlineado) },
		},
		{
			clave: "settings.indent.left", max: 10, paso: 0.25, decimales: 2, prop: "margin-left", css: px(pxPorCm),
			leer: func(b rico.Bloque) float64 { return float64(b.Base.MargenIzq) / pxPorCm },
		},
		{
			clave: "settings.indent.right", max: 10, paso: 0.25, decimales: 2, prop: "margin-right", css: px(pxPorCm),
			leer: func(b rico.Bloque) float64 { return float64(b.Base.MargenDer) / pxPorCm },
		},
		{
			clave: "settings.indent.firstLine", min: -5, max: 5, paso: 0.25, decimales: 2, prop: "text-indent", css: px(pxPorCm),
			leer: func(b rico.Bloque) float64 { return float64(b.Base.Sangria1) / pxPorCm },
		},
	}
}

// numero es `String(Number(v.toFixed(d)))`: sin ceros de sobra.
func numero(v float64, d int) string {
	p := math.Pow(10, float64(d))
	r := math.Round(v*p) / p
	if r == 0 {
		r = 0 // sin «-0»
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// cssPx es un valor en px con dos decimales como mucho.
func cssPx(v float64) string {
	if s := numero(v, 2); s != "0" {
		return s + "px"
	}
	return "0"
}

// panelAjustes es el cuerpo del panel. Se construye una vez por folio y se relee con
// cada cambio de formato o de documento.
type panelAjustes struct {
	f                          *Folio
	tr                         func(string) string
	pasos                      []*pasoAjuste
	bordes, cabecera, anchoAut *ui.Conmutador
	parrafo, tabla             fyne.CanvasObject
	pista                      *ui.Rotulo
	cuerpo                     *fyne.Container
	objeto                     fyne.CanvasObject
	enTabla                    bool
}

// PanelAjustes es el `PanelAjustes` del contexto de Docus.
func (f *Folio) PanelAjustes(t func(string, i18n.Vars) string) fyne.CanvasObject {
	if f.ajustes == nil {
		f.ajustes = nuevoPanelAjustes(f, t)
	}
	f.ajustes.refrescar()
	return f.ajustes.objeto
}

func nuevoPanelAjustes(f *Folio, t func(string, i18n.Vars) string) *panelAjustes {
	p := &panelAjustes{f: f, tr: func(k string) string {
		if t == nil {
			return k
		}
		return t(k, nil)
	}, pasos: pasosParrafo()}
	titulo := func(clave string) fyne.CanvasObject {
		// `sytue-overline text-text-subtle px-3 pt-3 pb-1`
		r := ui.NuevoRotulo(ui.Versalitas(p.tr(clave)), tema.TextoXS, ui.SemiNegrita, "text-subtle")
		return container.New(layout.NewCustomPaddedLayout(tema.Espacio(3), tema.Espacio(1), tema.Espacio(3), tema.Espacio(3)), r)
	}
	filas := []fyne.CanvasObject{titulo("settings.spacing")}
	for i, s := range p.pasos {
		if i == 3 {
			filas = append(filas, titulo("settings.indent"))
		}
		filas = append(filas, p.filaPaso(s))
	}
	// `sytue-meta px-3 pt-2`
	p.pista = ui.NuevoRotulo(p.tr("settings.paragraph.hint"), tema.TextoXS, ui.Normal, "text-muted")
	p.pista.Truncar = true
	filas = append(filas, container.New(layout.NewCustomPaddedLayout(tema.Espacio(2), 0, tema.Espacio(3), tema.Espacio(3)), p.pista))
	p.parrafo = container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, filas...)

	conmutador := func(clave string, poner func(*rico.Tabla, bool)) *ui.Conmutador {
		c := ui.NuevoConmutador(p.tr(clave), nil)
		c.Escalon = tema.TextoSM
		c.AlCambiar = func(v bool) { p.f.cambiarTabla(func(t *rico.Tabla) { poner(t, v) }) }
		return c
	}
	p.bordes = conmutador("settings.table.borders", (*rico.Tabla).PonerBordes)
	p.cabecera = conmutador("settings.table.bandedHeader", (*rico.Tabla).PonerCabeceraAlternada)
	p.anchoAut = conmutador("settings.table.autoWidth", (*rico.Tabla).PonerAnchoAuto)
	p.tabla = container.New(ui.Columna{Hueco: tema.Espacio(1)}, titulo("settings.table"),
		container.New(layout.NewCustomPaddedLayout(0, 0, tema.Espacio(2), tema.Espacio(2)),
			container.New(ui.Columna{Hueco: tema.Espacio(2)}, p.bordes, p.cabecera, p.anchoAut)))

	p.cuerpo = container.NewStack(p.parrafo)
	// `pb-3`, con su propio desplazamiento: el panel puede ser más alto que la pantalla.
	p.objeto = container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(3), 0, 0), p.cuerpo))
	return p
}

// filaPaso es `Stepper`: la etiqueta a la izquierda y −, la entrada y + a la derecha.
func (p *panelAjustes) filaPaso(s *pasoAjuste) fyne.CanvasObject {
	nombre := p.tr(s.clave)
	rotulo := func(k string) string {
		return strings.ReplaceAll(p.tr(k), "{name}", nombre)
	}
	if p.f.ctx != nil {
		rotulo = func(k string) string { return p.f.ctx.Traducir(k, i18n.Vars{"name": nombre}) }
	}
	saltar := func(signo float64) {
		n, _ := strconv.ParseFloat(numero(s.valor+signo*s.paso, s.decimales), 64)
		p.fijar(s, n)
	}
	s.menos = ui.NuevoBotonIcono(iconos.Menos, rotulo("settings.decrease"), func() { saltar(-1) })
	s.mas = ui.NuevoBotonIcono(iconos.Mas, rotulo("settings.increase"), func() { saltar(1) })
	s.entrada = ui.NuevaEntrada("")
	s.entrada.Rotulo = nombre
	s.entrada.Ancho = tema.Espacio(16) // `w-16`
	// `h-7 border-y px-1 text-center text-xs`, tan alta como sus botones: los tres son un
	// solo mando.
	alto := s.menos.MinSize().Height
	s.entrada.ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(1), MargenY: max((alto-tema.TextoXS.Linea)/2-tema.Borde, 0),
		Texto: tema.TextoXS, Borde: "border", Centrado: true,
	})
	confirmar := func() {
		v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s.entrada.Texto()), ",", "."), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			s.entrada.PonerTexto(numero(s.valor, s.decimales)) // como la web: vuelve lo que había
			return
		}
		if numero(v, s.decimales) != numero(s.valor, s.decimales) {
			p.fijar(s, v)
		}
	}
	s.entrada.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			confirmar()
			return true
		}
		return false
	}
	s.entrada.AlEnfocar = func(dentro bool) {
		if !dentro {
			confirmar()
		}
	}
	// `sytue-label min-w-0 truncate`: cede el ancho a los mandos y recorta con «…».
	etiqueta := ui.NuevoRotulo(nombre, tema.TextoSM, ui.Medio, "text")
	etiqueta.Truncar = true
	mandos := container.New(ui.Fila{Hueco: 0}, s.menos, s.entrada, s.mas)
	// `px-3 py-1`, `justify-between`, `gap-2`.
	return container.New(layout.NewCustomPaddedLayout(tema.Espacio(1), tema.Espacio(1), tema.Espacio(3), tema.Espacio(3)),
		container.NewBorder(nil, nil, nil, mandos, container.New(layout.NewCustomPaddedLayout(0, 0, 0, tema.Espacio(2)),
			container.NewVBox(layout.NewSpacer(), etiqueta, layout.NewSpacer()))))
}

// fijar acota el valor y lo escribe en los bloques de la selección (`aplicarParrafo`).
func (p *panelAjustes) fijar(s *pasoAjuste, v float64) {
	v = min(max(v, s.min), s.max)
	p.f.ponerParrafo(s.prop, s.css(v))
	p.refrescar()
}

// refrescar relee el documento: qué sección toca, sus valores y si se puede tocar.
func (p *panelAjustes) refrescar() {
	b, hay := p.f.bloqueAjustes()
	puede := hay && p.f.editable()
	t := p.f.tablaActual()
	if enTabla := t != nil; enTabla != p.enTabla || len(p.cuerpo.Objects) == 0 {
		p.enTabla = enTabla
		p.cuerpo.Objects = []fyne.CanvasObject{map[bool]fyne.CanvasObject{true: p.tabla, false: p.parrafo}[enTabla]}
		p.cuerpo.Refresh()
	}
	for _, s := range p.pasos {
		s.valor = 0
		if hay {
			s.valor = s.leer(b)
		}
		if !hay && s.prop == "line-height" {
			s.valor = 1.5 // `PARRAFO_VACIO`: el de `.sytue-page`
		}
		if !s.entrada.Enfocado() { // no pisa lo que se está tecleando
			s.entrada.PonerTexto(numero(s.valor, s.decimales))
		}
		s.entrada.Inerte(!puede)
		for _, bt := range []*ui.BotonIcono{s.menos, s.mas} {
			if puede {
				bt.Enable()
			} else {
				bt.Disable()
			}
		}
	}
	if hay {
		p.pista.Hide()
	} else {
		p.pista.Show()
	}
	estado := map[*ui.Conmutador]bool{p.bordes: t.Bordes(), p.cabecera: t.CabeceraAlternada(), p.anchoAut: t.AnchoAuto()}
	for c, v := range estado {
		c.Encendido = v
		if t != nil && p.f.editable() {
			c.Enable()
		} else {
			c.Disable()
		}
	}
}

// bloqueAjustes es el bloque de texto del cursor, del que lee el panel.
func (f *Folio) bloqueAjustes() (rico.Bloque, bool) {
	if f.doc == nil || !f.conCursor || f.cur.b >= len(f.bloques) || !f.deTexto(f.cur.b) {
		return rico.Bloque{}, false
	}
	return f.bloques[f.cur.b], true
}

// tablaActual es la tabla del cursor, o nil.
func (f *Folio) tablaActual() *rico.Tabla {
	if b, ok := f.bloqueAjustes(); ok && b.Celda != nil {
		return b.Celda.Tabla
	}
	return nil
}

// ponerParrafo pone una propiedad de párrafo en todos los bloques de la selección.
func (f *Folio) ponerParrafo(prop, valor string) {
	if !f.editable() {
		return
	}
	if _, ok := f.bloqueAjustes(); !ok {
		return
	}
	a, b := f.orden()
	f.porBloques(a, b, func(_ rico.Bloque, e *rico.Edicion) bool { return e.PonerBloque(prop, valor) })
}

// cambiarTabla aplica un ajuste a la tabla del cursor, con su paso de deshacer.
func (f *Folio) cambiarTabla(hacer func(*rico.Tabla)) {
	t := f.tablaActual()
	if t == nil || !f.editable() {
		return
	}
	f.antesDeCambiar(false)
	hacer(t)
	f.reestructurado()
	if f.ajustes != nil {
		f.ajustes.refrescar()
	}
}
