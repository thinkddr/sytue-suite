// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func obligacionArchivada(ob api.ERPObligacion) bool {
	return ob.Proceso != nil && ob.Proceso.Fase == 4
}

// Archivadas separa los justificantes guardados de las próximas y vencidas.
func (o *Obligaciones) Archivadas() []mes {
	var lista []api.ERPObligacion
	for _, ob := range o.Lista() {
		if obligacionArchivada(ob) {
			lista = append(lista, ob)
		}
	}
	return o.porMes(lista)
}

// etapasProceso es el ol de Obligaciones.tsx: cuatro pasos, sin el Stepper del diálogo.
func (o *Obligaciones) etapasProceso(v tema.Variante, ob api.ERPObligacion, ancho float32) (fyne.CanvasObject, float32) {
	fase := 0
	if ob.Proceso != nil {
		fase = ob.Proceso.Fase
	}
	ps := make([]pieza, 0, 4)
	for i, clave := range []string{"process.prepare", "onboarding.step.review", "process.file", "process.receipt"} {
		rol, peso := "text-subtle", ui.Normal
		if fase > i {
			rol = "success-text"
		} else if fase == i {
			rol, peso = "text", ui.SemiNegrita
		}
		rotulo := textoPieza(o.t(clave), tema.TextoXS, peso, rol, v)
		var marca pieza
		if fase > i {
			marca = piezaDe(iconos.Trazo(iconos.Tic, 14, rol, v).Objeto(), fyne.NewSize(14, 14))
		} else {
			marca = textoPieza(strconv.Itoa(i+1)+".", tema.TextoXS, peso, rol, v)
		}
		l := &lienzo{}
		h := max(marca.alto, rotulo.alto)
		l.pieza(marca, 0, (h-marca.alto)/2)
		l.pieza(rotulo, marca.ancho+HuecoUno, (h-rotulo.alto)/2)
		obj, _ := l.objeto(h)
		ps = append(ps, pieza{obj, marca.ancho + HuecoUno + rotulo.ancho, h, false})
	}
	a, h := medidasDe(ps)
	pos, tam := Envolver(a, h, HuecoColumnasObl, HuecoUno, ancho)
	l := &lienzo{}
	l.piezas(ps, pos, 0, 0)
	return l.objeto(tam.Height)
}

// ProcesoObligacion conserva el progreso del servidor. Paso puede volver atrás para
// revisar; Progreso es la fase persistida, que sólo cambia con una respuesta de API.
type ProcesoObligacion struct {
	tr                                                                        *i18n.Translator
	loc                                                                       i18n.Locale
	Obligacion                                                                api.ERPObligacion
	Progreso                                                                  *api.ERPProcesoObligacion
	Paso                                                                      int
	Moneda                                                                    string
	SoloLectura, Ocupado                                                      bool
	Revisado, Presentado                                                      bool
	Fallo, FalloCalculo, FalloFacturas                                        string
	Calculo                                                                   *api.ERPCalculoObligacion
	Facturas                                                                  *api.ERPPagina
	Ocupada                                                                   string
	CargandoMas                                                               bool
	Adjuntos                                                                  map[string][]api.ERPAdjunto
	Banco                                                                     *Banco
	VerBanco                                                                  bool
	Ventana                                                                   float32
	ReferenciaEditada                                                         bool
	AlCambio, AlCerrar, AlComprobar, AlAvanzar, AlCalcular, AlFacturas, AlMas func()
	AlElegir                                                                  func(int)
	AlAsentar                                                                 func(Registro)
	AlAbrirRegistro                                                           func(Registro)
	AlNominas                                                                 func()
	AlBanco                                                                   func()
	PuedeLeer, PuedeEscribir                                                  func(string) bool
	PuedeAccion                                                               func(string, string) bool
	AlSubir                                                                   func(string)
	AlAbrirAdjunto                                                            func(api.ERPAdjunto)
	AlBorrarAdjunto                                                           func(string, api.ERPAdjunto)
	referencia                                                                *ui.Entrada
	revisado, presentado                                                      *ui.Eleccion
}

func NuevoProcesoObligacion(tr *i18n.Translator, ob api.ERPObligacion) *ProcesoObligacion {
	x := &ProcesoObligacion{tr: tr, loc: tr.Locale(), Obligacion: ob, Moneda: "EUR", Adjuntos: map[string][]api.ERPAdjunto{}}
	x.referencia = ui.NuevaEntrada("").ConPropia(ui.CampoPropio{MargenX: tema.Espacio(3), MargenY: tema.Espacio(3), Texto: tema.TextoBase, Borde: "border", Radio: tema.RadioMD, Relleno: "bg"})
	x.referencia.AlCambiar(func(s string) {
		if utf8.RuneCountInString(s) > 200 {
			x.referencia.PonerTexto(string([]rune(s)[:200]))
			return
		}
		x.ReferenciaEditada = true
		x.cambio()
	})
	x.revisado = ui.NuevaCasilla(x.t("process.checked"), func(v bool) { x.Revisado = v; x.cambio() })
	x.presentado = ui.NuevaCasilla(x.t("process.filed"), func(v bool) { x.Presentado = v; x.cambio() })
	return x
}

func (x *ProcesoObligacion) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return x.tr.T(clave, vars[0])
	}
	return x.tr.T(clave, nil)
}

func (x *ProcesoObligacion) cambio() {
	if x.AlCambio != nil {
		x.AlCambio()
	}
}

func (x *ProcesoObligacion) puedeLeer(modulo string) bool {
	return x.PuedeLeer == nil || x.PuedeLeer(modulo)
}

func (x *ProcesoObligacion) puedeAccion(modulo, verbo string) bool {
	return x.PuedeAccion != nil && x.PuedeAccion(modulo, verbo)
}

func (x *ProcesoObligacion) Pendientes() bool {
	for _, n := range x.Obligacion.Pendientes {
		if n > 0 {
			return true
		}
	}
	return false
}
func (x *ProcesoObligacion) Referencia() string { return x.referencia.Texto() }

// Cargado adopta incluso una fase superior que otra persona haya guardado.
func (x *ProcesoObligacion) Cargado(p api.ERPProcesoObligacion) {
	x.Progreso = &p
	x.Paso = max(0, min(p.Fase, 4))
	if !x.ReferenciaEditada || p.Fase == 4 {
		x.referencia.PonerTexto(p.Referencia)
		x.ReferenciaEditada = false
	}
	x.Revisado, x.Presentado = false, false
}

func (x *ProcesoObligacion) documentos() []api.ERPDocumentoObligacion {
	if len(x.Obligacion.Documentos) > 0 {
		return x.Obligacion.Documentos
	}
	return []api.ERPDocumentoObligacion{{Clave: "justificante", Nombre: x.t("process.receipt")}}
}

func (x *ProcesoObligacion) Recibos() bool {
	for _, d := range x.documentos() {
		if len(x.Adjuntos[RegistroDocumento(x.Obligacion, d.Clave)]) == 0 {
			return false
		}
	}
	return true
}

func (x *ProcesoObligacion) PuedeContinuar() bool {
	if x.Progreso == nil || x.SoloLectura || x.Ocupado {
		return false
	}
	switch x.Paso {
	case 0:
		return !x.Pendientes()
	case 1:
		return x.Revisado && (!x.Obligacion.Calculable || x.Calculo != nil)
	case 2:
		return x.Presentado
	case 3:
		return x.Recibos() && strings.TrimSpace(x.Referencia()) != ""
	}
	return false
}

func (x *ProcesoObligacion) Elegir(paso int) {
	if x.Progreso == nil || x.Ocupado || x.Paso == 4 || paso < 0 || paso > x.Progreso.Fase {
		return
	}
	x.Paso = paso
	x.Revisado, x.Presentado = false, false
	if x.AlElegir != nil {
		x.AlElegir(paso)
	}
	x.cambio()
}

func (x *ProcesoObligacion) Cuerpo(v tema.Variante, ancho float32) fyne.CanvasObject {
	p := &pila{}
	if x.Fallo != "" {
		a := ui.NuevoParrafo(x.Fallo, tema.TextoBase, ui.Normal, "danger-text", v)
		l := &lienzo{}
		h := ui.AltoParrafo(a, max(ancho-2*tema.Espacio(3), 0)) + 2*tema.Espacio(3)
		l.caja(ui.NuevaCaja(tema.Color("danger-bg", v), sinColor, tema.RadioMD), ancho, h)
		l.poner(a, tema.Espacio(3), tema.Espacio(3), max(ancho-2*tema.Espacio(3), 0), h-2*tema.Espacio(3))
		o, _ := l.objeto(h)
		p.add(o, h, 0)
	}
	if x.Progreso == nil {
		if x.Fallo == "" {
			p.add(centrado(ui.NuevaRueda(24, "text-muted", v), 24+2*tema.Espacio(8)), 24+2*tema.Espacio(8), 0)
		} else {
			b := ui.NuevoBoton(x.t("process.check"), x.AlComprobar).Con(ui.Secundario, ui.TallaMD)
			p.add(b, b.MinSize().Height, tema.Espacio(3))
		}
		b := ui.NuevoBoton(x.t("action.close"), x.AlCerrar).Con(ui.Fantasma, ui.TallaMD)
		p.add(b, b.MinSize().Height, tema.Espacio(3))
		o, _ := p.objeto()
		return o
	}
	titulos := []string{x.t("process.prepare"), x.t("onboarding.step.review"), x.t("process.file"), x.t("process.receipt"), x.t("onboarding.step.done")}
	pasos := ui.NuevosPasos(titulos...)
	pasos.Actual, pasos.Ventana = x.Paso, x.Ventana
	pasos.RotuloIr = func(n int) string { return x.t("ui.wizard.goToStep", i18n.Vars{"n": n}) }
	if !x.Ocupado && x.Paso < 4 {
		pasos.AlElegir = x.Elegir
	}
	gap := float32(0)
	if x.Fallo != "" {
		gap = tema.Espacio(4)
	}
	p.add(pasos, ui.AltoPasos, gap)
	enc := &pila{}
	addParrafo(enc, titulos[x.Paso], tema.TextoXL, ui.SemiNegrita, "text", v, ancho, 0)
	descripcion := ""
	if x.Paso == 0 {
		descripcion = x.Obligacion.ConceptoRotulo
		if x.Obligacion.Guia != nil {
			descripcion = x.Obligacion.Guia.Que
		}
	}
	if x.Paso == 2 {
		descripcion = x.t("process.signature")
	}
	if descripcion != "" {
		addParrafo(enc, descripcion, tema.TextoBase, ui.Normal, "text-muted", v, min(ancho, tema.Espacio(168)), tema.Espacio(1.5))
	}
	o, h := enc.objeto()
	p.add(o, h, ObligGap)
	o, h = x.contenido(v, ancho)
	p.add(o, h, ObligGap)
	if x.Paso < 4 {
		o, h = x.pie(v, ancho)
		p.add(o, h, ObligGap)
	}
	o, _ = p.objeto()
	return o
}

func addParrafo(p *pila, texto string, escala tema.Escalon, peso ui.Peso, rol string, v tema.Variante, ancho, margen float32) {
	a := ui.NuevoParrafo(texto, escala, peso, rol, v)
	p.add(a, ui.AltoParrafo(a, ancho), margen)
}

func (x *ProcesoObligacion) contenido(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	gap := tema.Espacio(4)
	switch x.Paso {
	case 0:
		x.contenidoPreparacion(p, v, ancho, gap)
	case 1:
		x.contenidoRevision(p, v, ancho, gap)
	case 2:
		x.contenidoPresentacion(p, v, ancho, gap)
	case 3:
		x.contenidoJustificante(p, v, ancho)
	case 4:
		x.contenidoArchivado(p, v, ancho)
	}
	o, h := p.objeto()
	// Checkbox es inline-flex: su etiqueta de una línea deja 4 px de
	// descendente. La etiqueta móvil de 44 px ya ocupa la caja de línea entera.
	// Medido en ProcesoObligacion.tsx a670/278 px (2026-10-04).
	unaLinea := x.Paso == 1 && x.revisado.MinSize().Height <= tema.TextoBase.Linea || x.Paso == 2 && x.presentado.MinSize().Height <= tema.TextoBase.Linea
	if unaLinea {
		l := &lienzo{}
		l.poner(o, 0, 0, ancho, h)
		return l.objeto(h + 4)
	}
	return o, h
}

func (x *ProcesoObligacion) contenidoPreparacion(p *pila, v tema.Variante, ancho, gap float32) {
	addParrafo(p, x.t("process.prepareHint"), tema.TextoBase, ui.Normal, "text-muted", v, ancho, 0)
	for _, f := range Faltas(x.Obligacion, x.tr) {
		clave := "process.invoices"
		if f.Clave == "banco" {
			clave = "nav.bank"
		}
		if f.Clave == "nominas" {
			clave = "process.payroll"
		}
		rot := ui.Texto(x.t(clave)+" "+strconv.Itoa(x.Obligacion.Pendientes[f.Clave]), tema.TextoBase, ui.Normal, tema.Color("text", v))
		var ps []pieza
		if f.Clave == "banco" && x.puedeLeer("contabilidad") {
			b := ui.NuevoBoton(x.t("action.open"), x.AlBanco).Con(ui.Secundario, ui.TallaMD)
			ps = append(ps, piezaDe(b, b.MinSize()))
		}
		l := &lienzo{}
		contentW := max(ancho-2*tema.Espacio(3)-2*tema.Borde, 0)
		fila, h := cabeceraConAcciones(rot, tema.TextoBase, ps, tema.Espacio(3), contentW)
		l.caja(ui.NuevaCaja(sinColor, tema.Color("border", v), tema.RadioMD), ancho, h+2*tema.Espacio(3)+2*tema.Borde)
		l.poner(fila, tema.Borde+tema.Espacio(3), tema.Borde+tema.Espacio(3), contentW, h)
		o, hh := l.objeto(h + 2*tema.Espacio(3) + 2*tema.Borde)
		p.add(o, hh, gap)
	}
	faltaAcceso := false
	for k, m := range map[string]string{"banco": "contabilidad", "facturas": "facturacion", "nominas": "nominas"} {
		if x.Obligacion.Pendientes[k] > 0 && !x.puedeLeer(m) {
			faltaAcceso = true
		}
	}
	if faltaAcceso {
		addParrafo(p, x.t("process.accessNeeded"), tema.TextoBase, ui.Normal, "warning-text", v, ancho, gap)
	}
	if x.Obligacion.Pendientes["facturas"] > 0 && x.puedeLeer("facturacion") {
		o, h := x.facturas(v, ancho)
		p.add(o, h, gap)
	}
	if x.Obligacion.Pendientes["nominas"] > 0 && x.puedeLeer("nominas") {
		b := ui.NuevoBoton(x.t("process.payroll"), x.AlNominas).Con(ui.Secundario, ui.TallaMD)
		p.add(b, b.MinSize().Height, gap)
	}
	if x.VerBanco && x.Banco != nil && x.puedeLeer("contabilidad") {
		x.Banco.Ventana = ancho
		o := x.Banco.Objeto(v)
		p.add(o, o.MinSize().Height, gap)
	}
	if !x.Pendientes() {
		addParrafo(p, x.t("obligations.ready"), tema.TextoBase, ui.Normal, "success-text", v, ancho, gap)
	}
	b := ui.NuevoBoton(x.t("process.check"), x.AlComprobar).Con(ui.Fantasma, ui.TallaMD)
	p.add(b, b.MinSize().Height, gap)
}

func (x *ProcesoObligacion) contenidoRevision(p *pila, v tema.Variante, ancho, gap float32) {
	if x.Obligacion.Calculable && x.Calculo == nil {
		if x.FalloCalculo != "" {
			addParrafo(p, x.FalloCalculo, tema.TextoBase, ui.Normal, "danger-text", v, ancho, 0)
			b := ui.NuevoBoton(x.t("process.check"), x.AlCalcular).Con(ui.Secundario, ui.TallaMD)
			p.add(b, b.MinSize().Height, HuecoDos)
		} else {
			p.add(ui.NuevaRueda(24, "text-muted", v), 24, 0)
		}
	}
	if c := x.Calculo; c != nil {
		addParrafo(p, x.t("obligations.prepared", i18n.Vars{"from": c.Desde, "to": c.Hasta}), tema.TextoBase, ui.Normal, "text-muted", v, ancho, 0)
		l := &lienzo{}
		y := float32(0)
		for i, r := range []struct {
			clave string
			valor int64
		}{{"process.outputTax", c.HaberCentimos}, {"process.inputTax", c.DebeCentimos}, {"obligations.result", c.ResultadoCentimos}} {
			if i > 0 {
				l.poner(fileteHoja(v), 0, y, ancho, tema.Borde)
				y += tema.Borde
			}
			dt := textoPieza(x.t(r.clave), tema.TextoBase, ui.Normal, "text", v)
			dd := textoPieza(Dinero(x.loc, r.valor, x.Moneda), tema.TextoLG, ui.SemiNegrita, "text", v)
			h := max(dt.alto, dd.alto)
			l.pieza(dt, 0, y+tema.Espacio(3))
			l.pieza(dd, ancho-dd.ancho, y+tema.Espacio(3))
			y += h + 2*tema.Espacio(3)
		}
		o, h := l.objeto(y)
		p.add(o, h, gap)
		if !c.Asentado {
			addParrafo(p, x.t("obligations.notPosted"), tema.TextoBase, ui.Normal, "warning-text", v, ancho, gap)
		}
	}
	if g := x.Obligacion.Guia; g != nil && len(g.Necesitas) > 0 {
		for i, n := range g.Necesitas {
			arr := HuecoDos
			if i == 0 {
				arr = gap
			}
			addParrafo(p, n, tema.TextoBase, ui.Normal, "text", v, ancho, arr)
		}
	}
	x.revisado.AnchoEtiqueta = max(ancho-ui.LadoEleccion-ui.HuecoEleccion, 1)
	x.revisado.Marcada = x.Revisado
	if x.SoloLectura {
		x.revisado.Disable()
	} else {
		x.revisado.Enable()
	}
	p.add(x.revisado, x.revisado.MinSize().Height, gap)
}

func (x *ProcesoObligacion) contenidoPresentacion(p *pila, v tema.Variante, ancho, gap float32) {
	// El nombre del portal debajo y no en el botón (como la web): en la hoja de 390 el
	// rótulo entero pedía 375 y salía cortado por los dos lados.
	b := ui.NuevoBoton(x.t("obligations.portal"), func() { abrirURL(x.Obligacion.Portal.URL) }).Con(ui.Primario, ui.TallaMD)
	p.add(b, b.MinSize().Height, 0)
	nombre := ui.NuevoParrafo(x.Obligacion.Portal.Nombre, tema.TextoXS, ui.Normal, "text-subtle", v)
	p.add(nombre, ui.AltoParrafo(nombre, ancho), tema.Espacio(1))
	if g := x.Obligacion.Guia; g != nil && len(g.Pasos) > 0 {
		o, h := listaMarcada(g.Pasos, true, tema.TextoBase, v, ancho)
		p.add(o, h, gap)
	}
	x.presentado.AnchoEtiqueta = max(ancho-ui.LadoEleccion-ui.HuecoEleccion, 1)
	x.presentado.Marcada = x.Presentado
	if x.SoloLectura {
		x.presentado.Disable()
	} else {
		x.presentado.Enable()
	}
	p.add(x.presentado, x.presentado.MinSize().Height, gap)
}

func (x *ProcesoObligacion) contenidoJustificante(p *pila, v tema.Variante, ancho float32) {
	o, h := x.justificantes(v, ancho)
	p.add(o, h, 0)
	l := &lienzo{}
	label := textoPieza(x.t("process.reference"), tema.TextoBase, ui.Normal, "text", v)
	l.pieza(label, 0, 0)
	x.referencia.SoloLectura(x.SoloLectura)
	h = label.alto + HuecoDos + x.referencia.MinSize().Height
	l.poner(x.referencia, 0, label.alto+HuecoDos, ancho, x.referencia.MinSize().Height)
	o, _ = l.objeto(h)
	p.add(o, h, tema.Espacio(5))
}

func (x *ProcesoObligacion) contenidoArchivado(p *pila, v tema.Variante, ancho float32) {
	addParrafo(p, x.t("process.archived"), tema.TextoLG, ui.SemiNegrita, "success-text", v, ancho, 0)
	addParrafo(p, x.Progreso.Referencia, tema.TextoBase, ui.Normal, "text", v, ancho, tema.Espacio(3))
	addParrafo(p, x.t("process.acceptance"), tema.TextoBase, ui.Normal, "text-muted", v, ancho, tema.Espacio(3))
	o, h := x.justificantes(v, ancho)
	p.add(o, h, tema.Espacio(3))
	b := ui.NuevoBoton(x.t("action.close"), x.AlCerrar).Con(ui.Primario, ui.TallaMD)
	p.add(b, b.MinSize().Height, tema.Espacio(3))
}

func (x *ProcesoObligacion) justificantes(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	for i, d := range x.documentos() {
		reg := RegistroDocumento(x.Obligacion, d.Clave)
		doc := &Documentos{tr: x.tr, loc: x.loc, Titulo: d.Nombre, Requisitos: d.Requisitos, Adjuntos: x.Adjuntos[reg], Escribe: !x.SoloLectura, Ancho: ancho, AlAbrir: x.AlAbrirAdjunto}
		if x.AlSubir != nil {
			doc.AlSubir = func() { x.AlSubir(reg) }
		}
		if x.AlBorrarAdjunto != nil {
			doc.AlBorrar = func(a api.ERPAdjunto) { x.AlBorrarAdjunto(reg, a) }
		}
		o, h := doc.Objeto(v)
		gap := float32(0)
		if i > 0 {
			gap = tema.Espacio(5)
		}
		p.add(o, h, gap)
	}
	return p.objeto()
}

func (x *ProcesoObligacion) facturas(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	if x.FalloFacturas != "" {
		addParrafo(p, x.FalloFacturas, tema.TextoBase, ui.Normal, "danger-text", v, ancho, 0)
	}
	if x.Facturas == nil {
		if x.FalloFacturas == "" {
			p.add(ui.NuevaRueda(24, "text-muted", v), 24, 0)
		} else {
			b := ui.NuevoBoton(x.t("process.check"), x.AlFacturas).Con(ui.Secundario, ui.TallaMD)
			p.add(b, b.MinSize().Height, tema.Espacio(3))
		}
	} else {
		for i, r := range x.Facturas.Registros {
			reg := r
			l := &lienzo{}
			w := max(ancho-2*tema.Espacio(3)-2*tema.Borde, 0)
			pp := &pila{}
			moneda := x.Moneda
			if s, ok := r.Data["moneda"].(string); ok && s != "" {
				moneda = s
			}
			total, _ := numero(r.Data["total"])
			addParrafo(pp, NombreDe(r)+" · "+Dinero(x.loc, int64(total), moneda), tema.TextoBase, ui.SemiNegrita, "text", v, w, 0)
			abrir := ui.NuevoBoton(x.t("onboarding.step.review"), func() {
				if x.AlAbrirRegistro != nil {
					x.AlAbrirRegistro(reg)
				}
			}).Con(ui.Secundario, ui.TallaMD)
			ps := []pieza{piezaDe(abrir, abrir.MinSize())}
			if x.puedeAccion("facturacion", "asentar") {
				asentar := ui.NuevoBoton(x.t("action.post"), func() {
					if x.AlAsentar != nil {
						x.AlAsentar(reg)
					}
				}).Con(ui.Primario, ui.TallaMD)
				asentar.Inerte = x.Ocupada != "" || x.CargandoMas
				asentar.Cargando = x.Ocupada == r.ID
				ps = append(ps, piezaDe(asentar, asentar.MinSize()))
			}
			a, h := medidasDe(ps)
			pos, tam := Envolver(a, h, HuecoDos, HuecoDos, w)
			row := &lienzo{}
			row.piezas(ps, pos, 0, 0)
			obj, _ := row.objeto(tam.Height)
			pp.add(obj, tam.Height, HuecoDos)
			obj, alto := pp.objeto()
			l.caja(ui.NuevaCaja(sinColor, tema.Color("border", v), tema.RadioMD), ancho, alto+2*tema.Espacio(3)+2*tema.Borde)
			l.poner(obj, tema.Borde+tema.Espacio(3), tema.Borde+tema.Espacio(3), w, alto)
			obj, alto = l.objeto(alto + 2*tema.Espacio(3) + 2*tema.Borde)
			gap := float32(0)
			if i > 0 {
				gap = tema.Espacio(3)
			}
			p.add(obj, alto, gap)
		}
		if x.Facturas.Siguiente != "" {
			b := ui.NuevoBoton(x.t("action.more"), x.AlMas).Con(ui.Secundario, ui.TallaMD)
			b.Cargando = x.CargandoMas
			b.Inerte = x.Ocupada != ""
			p.add(b, b.MinSize().Height, tema.Espacio(3))
		}
	}
	return p.objeto()
}

func (x *ProcesoObligacion) pie(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	cancelar := ui.NuevoBoton(x.t("ui.wizard.cancel"), x.AlCerrar).Con(ui.Fantasma, ui.TallaMD)
	cancelar.Inerte = x.Ocupado
	var der []pieza
	if x.Paso > 0 {
		b := ui.NuevoBoton(x.t("ui.wizard.back"), func() { x.Elegir(x.Paso - 1) }).Con(ui.Secundario, ui.TallaMD)
		b.ConIcono(iconos.ChevronIzquierda, 16)
		b.Inerte = x.Ocupado
		der = append(der, piezaDe(b, b.MinSize()))
	}
	claves := []string{"obligations.prepare", "process.review", "process.filed", "process.finish"}
	b := ui.NuevoBoton(x.t(claves[x.Paso]), func() {
		if x.PuedeContinuar() && x.AlAvanzar != nil {
			x.AlAvanzar()
		}
	}).Con(ui.Primario, ui.TallaMD)
	b.Cargando = x.Ocupado
	b.Inerte = !x.PuedeContinuar()
	der = append(der, piezaDe(b, b.MinSize()))
	return pieGuiado(v, cancelar, der, ancho)
}
