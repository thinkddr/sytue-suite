// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"path/filepath"
	"strconv"
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

// # La factura de un movimiento (`FacturaDeMovimiento` de Banco.tsx)
//
// Crear la factura sin salir del banco: quién, qué IVA lleva y listo. La fecha y el
// total salen del propio movimiento. Lo que no cabe aquí (varios tipos de IVA,
// retenciones) se crea en su lista y se elige con «Otra factura».

// TiposIVA son los que se ofrecen, en el orden de la web.
var TiposIVA = []int{21, 10, 4, 0}

// Desglosar es la base y el IVA de un total con el IVA incluido, en céntimos. El IVA es
// lo que falta hasta el total, no un redondeo aparte: la suma cuadra siempre al céntimo.
func Desglosar(total int64, tipo int) (base, iva int64) {
	// `Math.round` de JavaScript: la mitad sube.
	base = (total*100*2 + int64(100+tipo)) / (2 * int64(100+tipo))
	return base, total - base
}

// FacturaMovimiento es el estado del diálogo.
type FacturaMovimiento struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Tarea      api.ERPTareaBanco
	Requisitos *api.ERPRequisitosFactura
	Tipo       int
	// PDF es la ruta del fichero elegido; vacía, ninguno.
	PDF       string
	Guardando bool
	Fallo     string
	// Requisitos abiertos (el `details`).
	VerRequisitos bool
	// Estrecha es la ventana por debajo de `sm`: la fecha y el IVA, apilados.
	Estrecha bool

	// AlElegirPDF abre el selector del sistema; nil, no hay y el campo no se ofrece.
	AlElegirPDF func()
	AlGuardar   func()
	AlCambio    func()

	nombre, fecha *ui.Entrada
}

// NuevaFacturaMovimiento abre el diálogo con el nombre que sugirió Sytue AI y la fecha
// del movimiento.
func NuevaFacturaMovimiento(tr *i18n.Translator, t api.ERPTareaBanco) *FacturaMovimiento {
	f := &FacturaMovimiento{tr: tr, loc: tr.Locale(), Tarea: t, Tipo: 21}
	f.nombre = ui.NuevaEntrada("").ConPropia(CampoFicha)
	f.nombre.PonerTexto(t.Empresa)
	f.nombre.AlCambiar(func(string) { f.cambio() })
	f.fecha = ui.CampoFecha(ui.CampoMD).ConPropia(CampoFicha)
	if d, _, ok := ui.LeerFecha(t.Fecha, time.UTC); ok {
		f.fecha.PonerTexto(ui.EscribirFecha(d))
	}
	return f
}

func (f *FacturaMovimiento) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return f.tr.T(clave, vars[0])
	}
	return f.tr.T(clave, nil)
}

func (f *FacturaMovimiento) cambio() {
	if f.AlCambio != nil {
		f.AlCambio()
	}
}

// Entra dice si el movimiento es un cobro (factura emitida) o un pago (recibida).
func (f *FacturaMovimiento) Entra() bool { return f.Tarea.Importe > 0 }

// Total es el importe del movimiento, sin signo.
func (f *FacturaMovimiento) Total() int64 { return max(f.Tarea.Importe, -f.Tarea.Importe) }

// Nombre es el tercero tecleado, recortado.
func (f *FacturaMovimiento) Nombre() string { return strings.TrimSpace(f.nombre.Texto()) }

// Fecha es la de la factura en AAAA-MM-DD, o la del movimiento si lo tecleado no vale.
func (f *FacturaMovimiento) Fecha() string {
	if d, _, ok := ui.LeerFecha(f.fecha.Texto(), time.UTC); ok {
		return d.Format(time.DateOnly)
	}
	return f.Tarea.Fecha
}

// Titulo es el del diálogo.
func (f *FacturaMovimiento) Titulo() string {
	if f.Entra() {
		return f.t("bank.invoiceTitleIn")
	}
	return f.t("bank.invoiceTitleOut")
}

// Cuerpo arma el contenido (`space-y-4`) al ancho del diálogo.
func (f *FacturaMovimiento) Cuerpo(v tema.Variante, ancho float32) fyne.CanvasObject {
	l := &lienzo{}
	var y float32
	add := func(o fyne.CanvasObject, w, h float32) {
		if y > 0 {
			y += HuecoCampos
		}
		l.poner(o, 0, y, w, h)
		y += h
	}
	dinero := func(c int64) string { return i18n.FormatCurrency(f.loc, c) }
	cab := ui.NuevoParrafo(f.Tarea.Descripcion+" · "+dinero(f.Total()), tema.TextoSM, ui.Normal, "text-muted", v)
	add(cab, ancho, ui.AltoParrafo(cab, ancho))
	// ¿Quién?, con su pista.
	quien := f.t("bank.whoOut")
	if f.Entra() {
		quien = f.t("bank.whoIn")
	}
	f.nombre.Ancho = ancho
	g, hg := campoConPista(v, quien, f.nombre, f.t("bank.whoHint"), ancho)
	add(g, ancho, hg)
	// La fecha y el IVA, en dos columnas desde `sm`; por debajo, una encima de otra.
	w := (ancho - HuecoCampos) / 2
	if f.Estrecha {
		w = ancho
	}
	f.fecha.Ancho = w
	gf := ui.GrupoCampo(f.t("bank.date"), false, f.fecha, "", "")
	ps := make([]pieza, 0, len(TiposIVA))
	for _, x := range TiposIVA {
		tipo := x
		r := strconv.Itoa(x) + " %"
		if x == 0 {
			r = f.t("bank.vatNone")
		}
		pi := NuevaPildoraIVA(r, f.Tipo == x, func() { f.Tipo = tipo; f.cambio() })
		ps = append(ps, piezaDe(pi, pi.MinSize()))
	}
	an, al := medidasDe(ps)
	pos, ocupa := Envolver(an, al, HuecoPildorasOblig, HuecoPildorasOblig, w)
	iva := &lienzo{}
	iva.texto(ui.Texto(f.t("bank.vat"), tema.TextoSM, ui.Medio, tema.Color("text", v)), 0, 0)
	iva.piezas(ps, pos, 0, tema.TextoSM.Linea+HuecoUno)
	altoIVA := tema.TextoSM.Linea + HuecoUno + ocupa.Height
	oIVA, _ := iva.objeto(altoIVA)
	hf := gf.MinSize().Height
	fila := &lienzo{}
	fila.poner(gf, 0, 0, w, hf)
	var oFila fyne.CanvasObject
	var altoFila float32
	if f.Estrecha {
		fila.poner(oIVA, 0, hf+HuecoCampos, w, altoIVA)
		oFila, altoFila = fila.objeto(hf + HuecoCampos + altoIVA)
	} else {
		fila.poner(oIVA, w+HuecoCampos, 0, w, altoIVA)
		oFila, altoFila = fila.objeto(max(hf, altoIVA))
	}
	add(oFila, ancho, altoFila)
	base, cuota := Desglosar(f.Total(), f.Tipo)
	des := ui.NuevoParrafo(f.t("bank.breakdown", i18n.Vars{"base": dinero(base), "vat": dinero(cuota), "total": dinero(f.Total())}),
		tema.TextoXS, ui.Normal, "text-muted", v)
	add(des, ancho, ui.AltoParrafo(des, ancho))
	// El PDF: con él la factura queda completa; sin él, queda la tarea de subirlo hoy.
	if f.AlElegirPDF != nil {
		pdf := &lienzo{}
		pdf.texto(ui.Texto(f.t("bank.invoiceFile"), tema.TextoSM, ui.Medio, tema.Color("text", v)), 0, 0)
		yp := tema.TextoSM.Linea + HuecoUno
		b := botonXS(f.t("docs.upload"), ui.Secundario, iconos.Subir, f.AlElegirPDF)
		mb := b.MinSize()
		pdf.poner(b, 0, yp, mb.Width, mb.Height)
		if f.PDF != "" {
			n := ui.Texto(filepath.Base(f.PDF), tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
			ui.Truncar(n, filepath.Base(f.PDF), tema.TextoSM, ui.Normal, max(ancho-mb.Width-HuecoDos, 0))
			pdf.texto(n, mb.Width+HuecoDos, yp+(mb.Height-tema.TextoSM.Linea)/2)
		}
		yp += mb.Height + HuecoUno
		pista := f.t("bank.invoiceFileHint")
		if f.PDF != "" {
			pista = f.t("bank.invoiceHint")
		}
		hp := ui.NuevoParrafo(pista, tema.TextoXS, ui.Normal, "text-subtle", v)
		hh := ui.AltoParrafo(hp, ancho)
		pdf.poner(hp, 0, yp, ancho, hh)
		o, h := pdf.objeto(yp + hh)
		add(o, ancho, h)
	}
	if r := f.Requisitos; r != nil && len(r.Completa) > 0 {
		o, h := f.requisitos(v, ancho)
		add(o, ancho, h)
	}
	if f.Fallo != "" {
		e := ui.NuevoParrafo(f.Fallo, tema.TextoSM, ui.Normal, "danger-text", v)
		add(e, ancho, ui.AltoParrafo(e, ancho))
	}
	o, _ := l.objeto(y)
	return o
}

// RequisitosDeFactura es el componente de Seguimiento.tsx: el `<details class="rounded-md
// border px-3 py-2">` con lo que tiene que llevar una factura para valer —la completa, la
// simplificada y la de fuera— y su fuente. Lo usan el diálogo de la factura de un
// movimiento del banco y la ficha de un gasto.
//
// La web no le da fondo: se ve el de debajo. Aquí `fondo` es ese color, con el que se
// rellena la caja, porque una `ui.Caja` con relleno transparente enseña su borde por
// dentro.
func RequisitosDeFactura(tr *i18n.Translator, r *api.ERPRequisitosFactura, abierto bool, alPulsar func(),
	fondo color.NRGBA, v tema.Variante, ancho float32,
) (fyne.CanvasObject, float32) {
	t := func(clave string) string { return tr.T(clave, nil) }
	dentro := ancho - 2*tema.Borde - 2*CalculoPX
	l := &lienzo{}
	l.caja(ui.NuevaCaja(fondo, tema.Color("border", v), tema.RadioMD), 0, 0)
	s := NuevoPulsador(t("docs.invoiceValid"), tema.TextoXS, ui.Medio, alPulsar)
	s.Marca, s.Radio = marcaDe(abierto), tema.RadioSM
	x0 := tema.Borde + CalculoPX
	y := tema.Borde + CalculoPY
	l.poner(s, x0, y, dentro, tema.TextoXS.Linea)
	y += tema.TextoXS.Linea
	if abierto {
		y += HuecoDos
		primero := true
		for _, bl := range []struct {
			titulo string
			lista  []string
		}{{"docs.invoiceFull", r.Completa}, {"docs.invoiceSimplified", r.Simplificada}, {"docs.invoiceForeign", r.Extranjera}} {
			if len(bl.lista) == 0 {
				continue
			}
			if !primero {
				y += HuecoDos
			}
			primero = false
			l.texto(ui.Texto(t(bl.titulo), tema.TextoXS, ui.SemiNegrita, tema.Color("text", v)), x0, y)
			y += tema.TextoXS.Linea + HuecoUno
			o, h := listaMarcada(bl.lista, false, tema.TextoXS, v, dentro)
			l.poner(o, x0, y, dentro, h)
			y += h
		}
		if r.Fuente != "" {
			y += HuecoDos
			p := ui.NuevoParrafo(r.Fuente, tema.TextoXS, ui.Normal, "text-subtle", v)
			h := ui.AltoParrafo(p, dentro)
			l.poner(p, x0, y, dentro, h)
			y += h
		}
	}
	alto := y + CalculoPY + tema.Borde
	l.puestos[0].w, l.puestos[0].h = ancho, alto
	return l.objeto(alto)
}

// requisitos es RequisitosDeFactura sobre el panel del diálogo (`surface-raised`).
func (f *FacturaMovimiento) requisitos(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	return RequisitosDeFactura(f.tr, f.Requisitos, f.VerRequisitos, func() { f.VerRequisitos = !f.VerRequisitos; f.cambio() },
		tema.Color("surface-raised", v), v, ancho)
}

// Pie son «Cerrar» y «Guardar y casar».
func (f *FacturaMovimiento) Pie(alCerrar func()) fyne.CanvasObject {
	c := ui.NuevoBoton(f.t("action.close"), alCerrar).Con(ui.Fantasma, ui.TallaSM)
	g := ui.NuevoBoton(f.t("bank.saveAndMatch"), f.AlGuardar).Con(ui.Primario, ui.TallaSM)
	g.Cargando, g.Inerte = f.Guardando, f.Nombre() == ""
	return container.New(ui.Fila{Hueco: HuecoDos}, c, g)
}

// campoConPista es el rótulo `text-sm font-medium`, el campo y la pista `text-xs`, con
// `space-y-1`. Devuelve también lo que mide.
func campoConPista(v tema.Variante, rotulo string, campo fyne.CanvasObject, pista string, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	l.texto(ui.Texto(rotulo, tema.TextoSM, ui.Medio, tema.Color("text", v)), 0, 0)
	y := tema.TextoSM.Linea + HuecoUno
	h := campo.MinSize().Height
	l.poner(campo, 0, y, ancho, h)
	y += h + HuecoUno
	p := ui.NuevoParrafo(pista, tema.TextoXS, ui.Normal, "text-subtle", v)
	hp := ui.AltoParrafo(p, ancho)
	l.poner(p, 0, y, ancho, hp)
	return l.objeto(y + hp)
}

// PildoraIVA es un tipo de IVA del diálogo: `rounded-control border px-3 py-1 text-sm`, la
// elegida con `border-strong bg-selected` y peso 600.
type PildoraIVA struct {
	ui.Pulsable
	Rotulo string
	Activa bool
}

// NuevaPildoraIVA crea la píldora.
func NuevaPildoraIVA(rotulo string, activa bool, al func()) *PildoraIVA {
	p := &PildoraIVA{Rotulo: rotulo, Activa: activa}
	p.Nombre, p.AlPulsar = rotulo, al
	p.ExtendBaseWidget(p)
	return p
}

func (p *PildoraIVA) peso() ui.Peso {
	if p.Activa {
		return ui.SemiNegrita
	}
	return ui.Normal
}

// MinSize es el rótulo con su `px-3 py-1` y el borde.
func (p *PildoraIVA) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(ui.Medir(p.Rotulo, tema.TextoSM, p.peso()).Width+2*PildoraPX+2*tema.Borde,
		tema.TextoSM.Linea+2*PildoraPY+2*tema.Borde)
}

// CreateRenderer arma el anillo, la caja y el rótulo.
func (p *PildoraIVA) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := tema.Color("bg", v), tema.Color("border", v), tema.Color("text-muted", v)
	if p.Activa {
		fondo, borde, tinta = sobre(tema.Color("selected", v), tema.Color("bg", v)), tema.Color("border-strong", v), tema.Color("text", v)
	}
	r := &pildoraIVARenderer{
		p: p, aro: ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v).Abierto(),
		caja:   ui.NuevaCaja(fondo, borde, tema.Forma("radius-control")),
		rotulo: ui.Texto(p.Rotulo, tema.TextoSM, p.peso(), tinta),
	}
	r.Refresh()
	return r
}

type pildoraIVARenderer struct {
	p      *PildoraIVA
	aro    *ui.Aro
	caja   *ui.Caja
	rotulo *canvas.Text
}

func (r *pildoraIVARenderer) MinSize() fyne.Size { return r.p.MinSize() }
func (r *pildoraIVARenderer) Objects() []fyne.CanvasObject {
	return append(append(r.aro.Objetos(), r.caja.Objetos()...), r.rotulo)
}
func (r *pildoraIVARenderer) Destroy() {}

func (r *pildoraIVARenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	ui.ColocarTexto(r.rotulo, tema.TextoSM, tema.Borde+PildoraPX, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *pildoraIVARenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if !r.p.Activa {
		tinta := tema.Color("text-muted", v)
		if r.p.Encima() {
			tinta = tema.Color("text", v)
		}
		r.rotulo.Color = tinta
		r.rotulo.Refresh()
	}
	r.aro.Ver(r.p.Enfocado())
	r.aro.Pintar(v)
}
