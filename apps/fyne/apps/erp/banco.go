// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Banco (`Banco.tsx`)
//
// Cada movimiento del banco es una TAREA hasta que tiene su factura o su explicación.
// No pregunta por cuentas ni asientos: pregunta «¿es esta factura?» y «¿qué es esto?», y
// el asiento lo escribe el servicio. Medido en el arnés (1280×1000, 2026-09-23):
//
//	cabecera   72,75: título `text-lg`, frase `max-w-prose` (dos líneas) y «Subir
//	           movimientos» (`sm` primario) a la derecha
//	cómo       `details` con borde, 38,75 plegado
//	tareas     versalitas y la lista `rounded-lg border divide-y`; cada tarea `px-4
//	           py-3 space-y-2`: concepto y fecha con el importe a la derecha (29,75), la
//	           pista de Sytue AI (14), las candidatas (`bg-bg-subtle`, 55,5 con una) y
//	           los tres mandos `xs` (24): 166,25 la de la pista, 81,75 la sola
//	hechas     `details` con los últimos justificados y «Deshacer»

// Medidas del banco.
const (
	// TareaBancoGap es el `space-y-2` de una tarea y del recuadro de candidatas el `space-y-1`.
	TareaBancoGap float32 = 2 * 0.25 * tema.Rem
	// HechaPY es el `py-2` de un movimiento justificado y HechasPB el `pb-2` de su lista.
	HechaPY  float32 = 2 * 0.25 * tema.Rem
	HechasPB float32 = 2 * 0.25 * tema.Rem
)

// esHTTPS: solo enlaces https; la dirección la propuso un modelo o la tecleó alguien.
var esHTTPS = regexp.MustCompile(`(?i)^https://`).MatchString

// Banco es el estado de la pantalla.
type Banco struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Datos son las tareas; nil, cargando.
	Datos       *api.ERPBanco
	Fallo       string
	Subiendo    bool
	SoloLectura bool
	// Ocupada es el movimiento cuya acción espera al servicio.
	Ocupada string
	// Modo es lo desplegado de cada tarea: «otra» (elegir otra factura) o «motivo».
	Modo map[string]string
	// Motivo es el elegido en cada tarea; Otra, la factura elegida (id y nombre).
	Motivo map[string]string
	Otra   map[string][2]string
	// Abiertos son los `details`: «como» y «hechas».
	Abiertos map[string]bool
	Ventana  float32
	// Buscar es el de SelectorRelacion, para «Otra factura».
	Buscar func(ctx context.Context, modelo, q string) ([]Registro, error)

	// AlSubir abre el selector del sistema; nil, no hay y el botón no se pinta.
	AlSubir    func()
	AlCasar    func(id, factura string)
	AlExplicar func(id, motivo string)
	AlReabrir  func(id string)
	AlCrear    func(t api.ERPTareaBanco)
	AlAbrirURL func(u string)
	// AlPedirFactura abre el correo ya escrito en Sytue Mail.
	AlPedirFactura func(t api.ERPTareaBanco)
	AlCambio       func()

	selectores map[string]*SelectorRelacion
	motivos    map[string]*ui.Selector
	// pistas: ya se le pidieron a Sytue AI en esta visita.
	pistas bool
}

// NuevoBanco crea la pantalla.
func NuevoBanco(tr *i18n.Translator) *Banco {
	return &Banco{
		tr: tr, loc: tr.Locale(), Modo: map[string]string{}, Motivo: map[string]string{}, Otra: map[string][2]string{},
		Abiertos: map[string]bool{}, selectores: map[string]*SelectorRelacion{}, motivos: map[string]*ui.Selector{},
	}
}

func (b *Banco) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return b.tr.T(clave, vars[0])
	}
	return b.tr.T(clave, nil)
}

func (b *Banco) cambio() {
	if b.AlCambio != nil {
		b.AlCambio()
	}
}

func (b *Banco) sm() bool { return b.Ventana == 0 || b.Ventana >= ui.CorteSM }

func (b *Banco) fecha(iso string) string {
	if d, _, ok := ui.LeerFecha(iso, time.UTC); ok {
		return i18n.FormatDayMonth(b.loc, d)
	}
	return iso
}

func (b *Banco) rotuloMotivo(clave string) string {
	if b.Datos != nil {
		for _, m := range b.Datos.Motivos {
			if m.Clave == clave {
				return m.Rotulo
			}
		}
	}
	return clave
}

// Importe es «+3025,00 €» o «−12,10 €»: el signo lo pone la dirección del dinero.
func Importe(l i18n.Locale, centimos int64) string {
	if centimos > 0 {
		return "+" + i18n.FormatCurrency(l, centimos)
	}
	return "−" + i18n.FormatCurrency(l, -centimos)
}

func (b *Banco) importe(v tema.Variante, c int64) pieza {
	token := "text"
	if c > 0 {
		token = "success-text"
	}
	o, w := textoTabular(Importe(b.loc, c), tema.TextoSM, ui.SemiNegrita, tema.Color(token, v))
	return pieza{obj: o, ancho: w, alto: tema.TextoSM.Linea}
}

// Objeto arma la pantalla.
func (b *Banco) Objeto(v tema.Variante) fyne.CanvasObject {
	ancho := anchoUtilDe(b.Ventana, AnchoObligaciones)
	p := &pila{}
	add := func(o fyne.CanvasObject, alto float32) {
		arriba := ObligGap
		if len(p.bloques) == 0 {
			arriba = 0
		}
		p.add(o, alto, arriba)
	}
	add(b.cabecera(v, ancho))
	add(b.plegado(v, "como", b.t("bank.howTo"), ancho, func(w float32) (fyne.CanvasObject, float32) {
		par := ui.NuevoParrafo(b.t("bank.howToBody"), tema.TextoSM, ui.Normal, "text-muted", v)
		cw := min(w, ProsaObligaciones)
		h := ui.AltoParrafo(par, cw)
		l := &lienzo{}
		l.poner(par, 0, 0, cw, h)
		return l.objeto(h)
	}, FilaOblPX))
	if b.Fallo != "" {
		bnd := &banda{
			caja:     ui.NuevaCaja(tema.Color("danger-bg", v), sinColor, tema.RadioMD),
			texto:    ui.Texto(b.Fallo, tema.TextoSM, ui.Normal, tema.Color("danger-text", v)),
			completo: b.Fallo,
		}
		add(container.New(bnd, append(bnd.caja.Objetos(), bnd.texto)...), AltoBandaAviso)
	}
	d := b.Datos
	switch {
	case d == nil:
		alto := 24 + 2*CargandoOblPY
		add(centrado(ui.NuevaRueda(24, "text-muted", v), alto), alto)
	case len(d.Pendientes) == 0:
		x := ui.NuevoVacio(b.t("bank.allDone"), b.t("bank.allDoneHint"))
		x.Ilustracion = iconos.Pintar(iconos.VacioCalendario, v).A(VacioOblIlustracion).Objeto()
		alto := x.MedidaVacio(ancho)
		add(centrado(x, alto), alto)
	default:
		add(b.pendientes(v, ancho))
	}
	if d != nil && len(d.Hechas) > 0 {
		add(b.plegado(v, "hechas", b.t("bank.done"), ancho, b.hechas(v), HechasPB))
	}
	o, alto := p.objeto()
	return enPagina(o, alto, AnchoObligaciones, MargenLista)
}

// cabecera es el título y la frase, y «Subir movimientos» a la derecha.
func (b *Banco) cabecera(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	var boton *ui.Boton
	var mb fyne.Size
	if !b.SoloLectura && b.AlSubir != nil {
		boton = ui.NuevoBoton(b.t("bank.upload"), b.AlSubir).Con(ui.Primario, ui.TallaSM)
		boton.ConIcono(iconos.Subir, 14)
		boton.MargenIcono, boton.Cargando = HuecoUno, b.Subiendo
		mb = boton.MinSize()
	}
	anchoIzq := ancho
	if b.sm() && boton != nil {
		anchoIzq = max(ancho-mb.Width-CabeceraObligGap, 0)
	}
	l.texto(ui.Texto(b.t("bank.title"), tema.TextoLG, ui.SemiNegrita, tema.Color("text", v)), 0, 0)
	frase := ui.NuevoParrafo(b.t("bank.subtitle"), tema.TextoSM, ui.Normal, "text-muted", v)
	wf := min(anchoIzq, ProsaObligaciones)
	hf := ui.AltoParrafo(frase, wf)
	l.poner(frase, 0, tema.TextoLG.Linea+HuecoUno, wf, hf)
	alto := tema.TextoLG.Linea + HuecoUno + hf
	if boton != nil {
		if b.sm() {
			l.poner(boton, ancho-mb.Width, 0, mb.Width, mb.Height)
			alto = max(alto, mb.Height)
		} else {
			l.poner(boton, 0, alto+CabeceraObligGap, mb.Width, mb.Height)
			alto += CabeceraObligGap + mb.Height
		}
	}
	alto += CabeceraPB
	l.poner(canvas.NewRectangle(tema.Color("border", v)), 0, alto, ancho, tema.Borde)
	return l.objeto(alto + tema.Borde)
}

// plegado es un `details` con borde: el `summary` `px-4 py-3 text-sm` y, desplegado,
// su contenido con `px-4` y el `pb` que diga.
func (b *Banco) plegado(v tema.Variante, clave, titulo string, ancho float32, cuerpo func(w float32) (fyne.CanvasObject, float32), pb float32) (fyne.CanvasObject, float32) {
	abierto := b.Abiertos[clave]
	s := NuevoPulsador(titulo, tema.TextoSM, ui.Medio, func() {
		b.Abiertos[clave] = !b.Abiertos[clave]
		b.cambio()
	})
	s.PX, s.PY, s.Marca, s.Radio = FilaOblPX, FilaOblPY, marcaDe(abierto), radioTarjeta()
	ms := s.MinSize()
	interior := ancho - 2*tema.Borde
	l := &lienzo{}
	caja := ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), radioTarjeta())
	l.caja(caja, 0, 0)
	l.poner(s, tema.Borde, tema.Borde, interior, ms.Height)
	alto := tema.Borde + ms.Height
	if abierto {
		o, h := cuerpo(interior - 2*FilaOblPX)
		l.poner(o, tema.Borde+FilaOblPX, alto, interior-2*FilaOblPX, h)
		alto += h + pb
	}
	alto += tema.Borde
	l.puestos[0].w, l.puestos[0].h = ancho, alto
	return l.objeto(alto)
}

// pendientes son «N movimientos por justificar» y su lista.
func (b *Banco) pendientes(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	t := ui.NuevoVersal(b.t("bank.pendingTitle", i18n.Vars{"count": b.Datos.TotalPendientes}), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	p.add(t.Objeto(), tema.TextoXS.Linea, 0)
	filas := make([]bloque, 0, len(b.Datos.Pendientes))
	for _, tarea := range b.Datos.Pendientes {
		o, alto := b.tarea(v, tarea, ancho-2*tema.Borde)
		filas = append(filas, bloque{o, alto, 0})
	}
	o, alto := listaEnCaja(v, filas, radioTarjeta())
	p.add(o, alto, HuecoDos)
	return p.objeto()
}

// tarea es un movimiento pendiente: concepto e importe, la pista de Sytue AI, las
// candidatas y los mandos, y lo que se despliegue («otra factura» o «¿qué es?»).
func (b *Banco) tarea(v tema.Variante, t api.ERPTareaBanco, ancho float32) (fyne.CanvasObject, float32) {
	dentro := ancho - 2*FilaOblPX
	x0 := FilaOblPX
	l := &lienzo{}
	y := FilaOblPY
	// Concepto y fecha; el importe a la derecha (`items-start justify-between gap-3`).
	imp := b.importe(v, t.Importe)
	wIzq := max(dentro-imp.ancho-HuecoTarjetas, 0)
	desc := ui.NuevoParrafo(t.Descripcion, tema.TextoSM, ui.Medio, "text", v)
	hd := ui.AltoParrafo(desc, wIzq)
	l.poner(desc, x0, y, wIzq, hd)
	l.texto(ui.Texto(b.fecha(t.Fecha), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)), x0, y+hd)
	l.pieza(imp, x0+dentro-imp.ancho, y)
	y += max(hd+tema.TextoXS.Linea, imp.alto)
	entra := t.Importe > 0
	// La pista de Sytue AI: quién lo cobró y dónde está su factura.
	if !entra && (t.Empresa != "" || esHTTPS(t.URLFactura)) {
		o, h := b.pista(v, t, dentro)
		y += TareaBancoGap
		l.poner(o, x0, y, dentro, h)
		y += h
	}
	if len(t.Candidatas) > 0 {
		o, h := b.candidatas(v, t, dentro)
		y += TareaBancoGap
		l.poner(o, x0, y, dentro, h)
		y += h
	}
	if entra {
		aviso := ui.NuevoParrafo(b.t("bank.customerPaymentHint"), tema.TextoSM, ui.Normal, "text-muted", v)
		h := ui.AltoParrafo(aviso, dentro)
		y += TareaBancoGap
		l.poner(aviso, x0, y, dentro, h)
		y += h
	}
	if !b.SoloLectura {
		o, h := b.mandos(t, dentro)
		y += TareaBancoGap
		l.poner(o, x0, y, dentro, h)
		y += h
		switch b.Modo[t.ID] {
		case "otra":
			o, h := b.otra(t, dentro)
			y += TareaBancoGap
			l.poner(o, x0, y, dentro, h)
			y += h
		case "motivo":
			o, h := b.motivo(v, t, dentro)
			y += TareaBancoGap
			l.poner(o, x0, y, dentro, h)
			y += h
		}
	}
	return l.objeto(y + FilaOblPY)
}

// pista es la línea de Sytue AI (`text-xs flex-wrap gap-x-1.5`): la empresa y el enlace
// a su factura, o el correo que la pide, o «pídele la factura» con su web.
func (b *Banco) pista(v tema.Variante, t api.ERPTareaBanco, ancho float32) (fyne.CanvasObject, float32) {
	ps := []pieza{piezaDe(iconos.Trazo(iconos.IA, 12, "text-muted", v).Objeto(), fyne.NewSize(12, 12))}
	if t.Empresa != "" {
		ps = append(ps, textoPieza(t.Empresa, tema.TextoXS, ui.Medio, "text", v))
	}
	enlace := func(texto, token string, al func()) {
		e := NuevoPulsador(texto, tema.TextoXS, ui.Normal, al)
		e.Subrayado, e.Radio, e.Token, e.TokenEncima = true, tema.RadioSM, token, token
		ps = append(ps, piezaDe(e, e.MinSize()))
	}
	abrir := func(u string) func() {
		return func() {
			if b.AlAbrirURL != nil {
				b.AlAbrirURL(u)
			}
		}
	}
	switch {
	case esHTTPS(t.URLFactura):
		enlace(b.t("bank.hintDownload"), "text", abrir(t.URLFactura))
	case t.EmailFacturas != "":
		tarea := t
		enlace(b.t("bank.hintRequest", i18n.Vars{"email": t.EmailFacturas}), "text", func() {
			if b.AlPedirFactura != nil {
				b.AlPedirFactura(tarea)
			}
		})
	default:
		ps = append(ps, textoPieza(b.t("bank.hintAsk"), tema.TextoXS, ui.Normal, "text-muted", v))
		if esHTTPS(t.Web) {
			ps = append(ps, textoPieza("·", tema.TextoXS, ui.Normal, "text-muted", v))
			enlace(b.t("bank.hintWeb"), "text-muted", abrir(t.Web))
		}
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, 1.5*0.25*tema.Rem, 0, ancho)
	l := &lienzo{}
	l.piezas(ps, pos, 0, 0)
	return l.objeto(ocupa.Height)
}

// candidatas es «¿Es esta factura?» con cada una y su «Sí, es esta» (`primary` la segura).
func (b *Banco) candidatas(v tema.Variante, t api.ERPTareaBanco, ancho float32) (fyne.CanvasObject, float32) {
	dentro := ancho - 2*CalculoPX
	l := &lienzo{}
	l.caja(ui.NuevaCaja(tema.Color("bg-subtle", v), sinColor, tema.RadioMD), 0, 0)
	y := CalculoPY
	l.texto(ui.Texto(b.t("bank.isThisIt"), tema.TextoXS, ui.Medio, tema.Color("text-muted", v)), CalculoPX, y)
	y += tema.TextoXS.Linea
	for _, f := range t.Candidatas {
		id, factura := t.ID, f.ID
		variante := ui.Secundario
		if f.Segura {
			variante = ui.Primario
		}
		s := ui.NuevoBoton(b.t("bank.yesThisOne"), func() {
			if b.AlCasar != nil {
				b.AlCasar(id, factura)
			}
		}).Con(variante, ui.TallaXS)
		s.Cargando, s.Inerte = b.Ocupada == t.ID, b.SoloLectura
		ms := s.MinSize()
		tercero := f.Tercero
		if tercero == "" {
			tercero = SinValor
		}
		texto := f.Ref + " · " + tercero + " · " + i18n.FormatCurrency(b.loc, f.Total)
		tx := ui.Texto(texto, tema.TextoSM, ui.Normal, tema.Color("text", v))
		y += HuecoUno
		// `flex-wrap justify-between gap-2`: si el texto y el botón no caben, el botón baja.
		if ui.Medir(texto, tema.TextoSM, ui.Normal).Width+HuecoDos+ms.Width > dentro {
			ui.Truncar(tx, texto, tema.TextoSM, ui.Normal, dentro)
			l.texto(tx, CalculoPX, y)
			y += tema.TextoSM.Linea + HuecoDos
			l.poner(s, CalculoPX, y, ms.Width, ms.Height)
		} else {
			l.texto(tx, CalculoPX, y+(ms.Height-tema.TextoSM.Linea)/2)
			l.poner(s, CalculoPX+dentro-ms.Width, y, ms.Width, ms.Height)
		}
		y += ms.Height
	}
	alto := y + CalculoPY
	l.puestos[0].w, l.puestos[0].h = ancho, alto
	return l.objeto(alto)
}

// mandos son «Añadir factura», «Otra factura» y «No lleva factura» (`gap-1.5`).
func (b *Banco) mandos(t api.ERPTareaBanco, ancho float32) (fyne.CanvasObject, float32) {
	tarea := t
	crear := botonXS(b.t("bank.addInvoice"), ui.Secundario, iconos.Mas, func() {
		if b.AlCrear != nil {
			b.AlCrear(tarea)
		}
	})
	alternar := func(modo string) func() {
		return func() {
			if b.Modo[tarea.ID] == modo {
				b.Modo[tarea.ID] = ""
			} else {
				b.Modo[tarea.ID] = modo
			}
			b.cambio()
		}
	}
	otra := ui.NuevoBoton(b.t("bank.otherInvoice"), alternar("otra")).Con(ui.Fantasma, ui.TallaXS)
	motivo := ui.NuevoBoton(b.t("bank.noInvoice"), alternar("motivo")).Con(ui.Fantasma, ui.TallaXS)
	ps := []pieza{piezaDe(crear, crear.MinSize()), piezaDe(otra, otra.MinSize()), piezaDe(motivo, motivo.MinSize())}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoPildorasOblig, HuecoPildorasOblig, ancho)
	l := &lienzo{}
	l.piezas(ps, pos, 0, 0)
	return l.objeto(ocupa.Height)
}

// otra es elegir otra factura con el selector de relación y «Sí, es esta».
func (b *Banco) otra(t api.ERPTareaBanco, ancho float32) (fyne.CanvasObject, float32) {
	modelo := "factura_compra"
	if t.Importe > 0 {
		modelo = api.ERPFacturaVenta
	}
	id := t.ID
	s := b.selectores[id]
	if s == nil {
		s = NuevoSelectorRelacion(b.tr, modelo, "", "", func(valor, etiqueta string) {
			b.Otra[id] = [2]string{valor, etiqueta}
			b.cambio()
		})
		b.selectores[id] = s
	}
	s.Buscar = b.Buscar
	elegida := b.Otra[id][0]
	si := ui.NuevoBoton(b.t("bank.yesThisOne"), func() {
		if elegida != "" && b.AlCasar != nil {
			b.AlCasar(id, elegida)
		}
	}).Con(ui.Primario, ui.TallaSM)
	si.Cargando, si.Inerte = b.Ocupada == id, elegida == ""
	ms, mr := si.MinSize(), s.MinSize()
	l := &lienzo{}
	if b.sm() {
		alto := max(ms.Height, mr.Height)
		w := max(ancho-ms.Width-HuecoDos, 0)
		l.poner(s, 0, (alto-mr.Height)/2, w, mr.Height)
		l.poner(si, w+HuecoDos, (alto-ms.Height)/2, ms.Width, ms.Height)
		return l.objeto(alto)
	}
	l.poner(s, 0, 0, ancho, mr.Height)
	l.poner(si, 0, mr.Height+HuecoDos, ms.Width, ms.Height)
	return l.objeto(mr.Height + HuecoDos + ms.Height)
}

// motivo es «¿Qué es?» con el `<select>` de motivos (el sugerido, marcado) y «Guardar».
func (b *Banco) motivo(v tema.Variante, t api.ERPTareaBanco, ancho float32) (fyne.CanvasObject, float32) {
	id := t.ID
	elegido, ok := b.Motivo[id]
	if !ok {
		elegido = t.Sugerencia
	}
	ops := make([]string, 0, 1+len(b.Datos.Motivos))
	ops = append(ops, b.t("relation.choose"))
	idx := 0
	for i, m := range b.Datos.Motivos {
		r := m.Rotulo
		if m.Clave == t.Sugerencia {
			r += " · " + b.t("bank.suggested")
		}
		ops = append(ops, r)
		if m.Clave == elegido {
			idx = i + 1
		}
	}
	s := b.motivos[id]
	if s == nil {
		s = &ui.Selector{Propio: selectorPropio(CampoFicha, AltoSelectNativo)}
		s.ExtendBaseWidget(s)
		b.motivos[id] = s
	}
	motivos := b.Datos.Motivos
	s.Opciones, s.Elegida = ops, idx
	s.AlElegir = func(i int) {
		if i > 0 {
			b.Motivo[id] = motivos[i-1].Clave
			b.cambio()
		}
	}
	guardar := ui.NuevoBoton(b.t("action.save"), func() {
		if elegido != "" && b.AlExplicar != nil {
			b.AlExplicar(id, elegido)
		}
	}).Con(ui.Primario, ui.TallaSM)
	guardar.Cargando, guardar.Inerte = b.Ocupada == id, elegido == ""
	rot := textoPieza(b.t("bank.reason"), tema.TextoXS, ui.Medio, "text-muted", v)
	mg, alto := guardar.MinSize(), AltoSelectNativo
	l := &lienzo{}
	var y float32
	if b.sm() {
		fila := max(alto, mg.Height)
		ws := max(ancho-rot.ancho-mg.Width-2*HuecoDos, 0)
		l.pieza(rot, 0, (fila-rot.alto)/2)
		s.Ancho = ws
		l.poner(s, rot.ancho+HuecoDos, (fila-alto)/2, ws, alto)
		l.poner(guardar, ancho-mg.Width, (fila-mg.Height)/2, mg.Width, mg.Height)
		y = fila
	} else {
		l.pieza(rot, 0, 0)
		y = rot.alto + HuecoDos
		s.Ancho = ancho
		l.poner(s, 0, y, ancho, alto)
		y += alto + HuecoDos
		l.poner(guardar, 0, y, mg.Width, mg.Height)
		y += mg.Height
	}
	if t.Importe > 0 && elegido == "otro" {
		h := ui.NuevoParrafo(b.t("bank.otherIncomeHint"), tema.TextoXS, ui.Normal, "text-subtle", v)
		hh := ui.AltoParrafo(h, ancho)
		y += HuecoUno
		l.poner(h, 0, y, ancho, hh)
		y += hh
	}
	return l.objeto(y)
}

// hechas son los últimos justificados, con «Deshacer».
func (b *Banco) hechas(v tema.Variante) func(w float32) (fyne.CanvasObject, float32) {
	return func(w float32) (fyne.CanvasObject, float32) {
		filas := make([]bloque, 0, len(b.Datos.Hechas))
		for _, m := range b.Datos.Hechas {
			filas = append(filas, b.hecha(v, m, w))
		}
		l := &lienzo{}
		var y float32
		for i, f := range filas {
			if i > 0 {
				l.poner(canvas.NewRectangle(tema.Color("border", v)), 0, y, w, tema.Borde)
				y += tema.Borde
			}
			l.poner(f.obj, 0, y, w, f.alto)
			y += f.alto
		}
		return l.objeto(y)
	}
}

func (b *Banco) hecha(v tema.Variante, m api.ERPMovimientoBanco, ancho float32) bloque {
	imp := b.importe(v, m.Importe)
	der := []pieza{imp}
	if !b.SoloLectura {
		id := m.ID
		d := ui.NuevoBoton(b.t("bank.undo"), func() {
			if b.AlReabrir != nil {
				b.AlReabrir(id)
			}
		}).Con(ui.Fantasma, ui.TallaXS)
		d.Cargando = b.Ocupada == m.ID
		der = append(der, piezaDe(d, d.MinSize()))
	}
	var wd, hd float32
	for i, p := range der {
		if i > 0 {
			wd += HuecoDos
		}
		wd += p.ancho
		hd = max(hd, p.alto)
	}
	cabe := max(ancho-wd-HuecoTarjetas, 0)
	desc := ui.Texto(m.Descripcion, tema.TextoSM, ui.Normal, tema.Color("text", v))
	ui.Truncar(desc, m.Descripcion, tema.TextoSM, ui.Normal, cabe)
	sub := b.fecha(m.Fecha) + " · "
	if m.FacturaRef != "" {
		sub += b.t("bank.matched", i18n.Vars{"ref": m.FacturaRef})
	} else {
		sub += b.rotuloMotivo(m.Motivo)
	}
	st := ui.Texto(sub, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	ui.Truncar(st, sub, tema.TextoXS, ui.Normal, cabe)
	izq := tema.TextoSM.Linea + tema.TextoXS.Linea
	fila := max(izq, hd)
	alto := fila + 2*HechaPY
	l := &lienzo{}
	y0 := HechaPY + (fila-izq)/2
	l.texto(desc, 0, y0)
	l.texto(st, 0, y0+tema.TextoSM.Linea)
	x := ancho - wd
	for _, p := range der {
		l.pieza(p, x, HechaPY+(fila-p.alto)/2)
		x += p.ancho + HuecoDos
	}
	o, _ := l.objeto(alto)
	return bloque{o, alto, 0}
}

// PedirFacturaRuta es la ruta de Sytue Mail con el correo que pide la factura ya
// escrito: el cargo (fecha, importe, concepto) y los datos fiscales que la factura
// tiene que llevar.
func PedirFacturaRuta(tr *i18n.Translator, l i18n.Locale, t api.ERPTareaBanco, org *api.ERPOrganizacion) string {
	importe := i18n.FormatCurrency(l, max(t.Importe, -t.Importe))
	fecha := t.Fecha
	if d, _, ok := ui.LeerFecha(t.Fecha, time.UTC); ok {
		fecha = i18n.FormatDayMonth(l, d)
	}
	var nombre, nif string
	var dir []string
	if org != nil {
		nombre, nif = org.Nombre, org.NIF
		for _, s := range []string{org.Direccion, org.CP, org.Ciudad} {
			if s != "" {
				dir = append(dir, s)
			}
		}
	}
	q := url.Values{
		"action": {"compose"}, "to": {t.EmailFacturas},
		"subject": {tr.T("bank.requestSubject", i18n.Vars{"amount": importe, "date": fecha})},
		"body": {tr.T("bank.requestBody", i18n.Vars{
			"amount": importe, "date": fecha, "concept": t.Descripcion, "company": nombre, "nif": nif,
			"address": strings.Join(dir, ", "),
		})},
	}
	return "/?" + q.Encode()
}
