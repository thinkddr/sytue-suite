// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"image/color"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La ficha (`Ficha` de ErpApp.tsx)
//
// Un formulario generado de los campos del modelo, en un panel a la derecha de la lista
// —la lista sigue visible detrás, así que se ve en qué se está trabajando— y, en el
// móvil, encima y a pantalla completa.
//
// Medido en el arnés del frontal (1280×800, rem 14, 2026-09-23), factura FV-2026-00041:
//
//	panel        x 888, w 392 (`max-w-md`), `border-l bg-surface`       (medido)
//	formulario   `px-6 py-5`: contenido en x 910, w 349                 (medido)
//	cabecera     título `text-lg` truncado y la referencia en mono `xs` que PARTE
//	             por los guiones cuando los botones se comen el sitio: 22,2 de
//	             ancho y tres líneas —así sale en la web con los cinco botones—
//	barra estado `-mx-6 border-y bg-surface-sunken px-6 py-2.5`, 391×77  (medido)
//	campos       49,25 cada uno con `space-y-3`                          (medido)
//	pie          Guardar 73,08 · Llevar al libro 103,23 · Duplicar como borrador
//	             160,09 · Eliminar 72,61: el último SE SALE del panel en la web y
//	             aquí también, que es lo que hay que copiar                (medido)

// Medidas de la ficha.
const (
	// AnchoFicha es `max-w-md`: 28 rem.
	AnchoFicha float32 = 28 * tema.Rem
	// FichaPX y FichaPY son el `px-6 py-5` del formulario.
	FichaPX float32 = 6 * 0.25 * tema.Rem
	FichaPY float32 = 5 * 0.25 * tema.Rem
	// CabeceraFichaMB es el `mb-4` de la cabecera y de la barra de estado.
	CabeceraFichaMB float32 = 4 * 0.25 * tema.Rem
	// HuecoCabeceraFicha es el `gap-3` de la cabecera.
	HuecoCabeceraFicha float32 = 3 * 0.25 * tema.Rem
	// BarraEstadoPY es el `py-2.5` de la barra de estado; HuecoBarraEstado su `gap-3`,
	// HuecoAccionesEstado el `gap-1.5` de las acciones y HuecoEtapas el `gap-1`.
	BarraEstadoPY       float32 = 2.5 * 0.25 * tema.Rem
	HuecoBarraEstado    float32 = 3 * 0.25 * tema.Rem
	HuecoAccionesEstado float32 = 1.5 * 0.25 * tema.Rem
	HuecoEtapas         float32 = 1 * 0.25 * tema.Rem
	// EtapaPX y EtapaPY son el `px-2.5 py-1` de una etapa.
	EtapaPX float32 = 2.5 * 0.25 * tema.Rem
	EtapaPY float32 = 1 * 0.25 * tema.Rem
	// AvisoFichaPX, AvisoFichaPY y AvisoFichaMB son el `px-3 py-2 mb-3` de los avisos.
	AvisoFichaPX float32 = 3 * 0.25 * tema.Rem
	AvisoFichaPY float32 = 2 * 0.25 * tema.Rem
	AvisoFichaMB float32 = 3 * 0.25 * tema.Rem
	// SeccionMT es el `mt-6` de las secciones de debajo de los campos y del pie.
	SeccionMT float32 = 6 * 0.25 * tema.Rem
	// SeguimientoPT es el `pt-4` tras el filete, y SeguimientoGap su `space-y-5`.
	SeguimientoPT  float32 = 4 * 0.25 * tema.Rem
	SeguimientoGap float32 = 5 * 0.25 * tema.Rem
	// HuecoPieFicha es el `gap-2` del pie.
	HuecoPieFicha float32 = 2 * 0.25 * tema.Rem
	// ImprimirPX y ImprimirPY son el `px-2 py-1.5` del enlace de imprimir.
	ImprimirPX float32 = 2 * 0.25 * tema.Rem
	ImprimirPY float32 = 1.5 * 0.25 * tema.Rem
	// TrackingEtapa es el `tracking-wider` de «Etapa:».
	TrackingEtapa float32 = 0.05
)

// Ficha es el estado del panel de un registro. Como la lista, es ESTADO y no vista: la
// pantalla la rellena y pide el objeto.
type Ficha struct {
	tr  *i18n.Translator
	loc i18n.Locale

	Modelo Modelo
	// Registro es el que se edita; nil es una ficha nueva.
	Registro *Registro
	// Valores son los textos de los campos; Rotulos, el nombre legible de cada
	// relación; Errores, lo que el servicio dijo de cada campo.
	Valores, Rotulos, Errores map[string]string
	// Fallo es la banda roja y Aviso la verde.
	Fallo, Aviso string
	// Guardando es el `guardando` de la web: las acciones esperan.
	Guardando bool
	// EstadoActual es el de la barra de etapas.
	EstadoActual string
	// EnPapelera: se abrió desde la papelera y lo que toca es restaurar. Facturae: la
	// organización es española. SoloLectura: sin escritura en el módulo.
	EnPapelera, Facturae, SoloLectura bool
	// Movil es la ventana por debajo de `md`: el panel va a pantalla completa.
	Movil bool

	// Lo que cuelga del registro. Nil es «todavía no ha llegado» (la rueda de 14).
	Asientos   []api.ERPAsiento
	Tareas     []api.ERPActividad
	Adjuntos   []api.ERPAdjunto
	Historial  []api.ERPEntradaHistorial
	Verifactu  *api.ERPFacturaVerifactu
	Requisitos *api.ERPRequisitosFactura
	// Planificando es el editor de «Planificar tarea» abierto; los otros, las ruedas
	// de sus botones.
	Planificando, SubiendoDoc, Anotando, Reenviando bool
	// VerRequisitos es el `open` del `<details>` de lo que tiene que llevar una factura.
	VerRequisitos bool

	// BuscarRelacion es la consulta del selector de relaciones.
	BuscarRelacion     func(ctx context.Context, modelo, q string) ([]Registro, error)
	PuedeCrearRelacion func(modelo string) bool
	CrearRelacion      func(modelo string, creado func(Registro))
	alCreado           func(Registro)
	alRepintar         func()

	AlCerrar, AlGuardar, AlAsentar, AlBorrar, AlRestaurar, AlDuplicar, AlCopiarID func()
	AlEstado                                                                      func(estado string)
	AlAnular                                                                      func(api.ERPAsiento)
	// AlDescargar baja «ubl», «facturae» o «imprimir».
	AlDescargar func(que string)
	AlHecha     func(api.ERPActividad)
	// AlPlanificar crea la tarea; `para` es su responsable (Sub vacío: quien la crea).
	AlPlanificar func(tipo, resumen, vence string, para api.ERPPersona)
	// Equipo es la gente que puede llevar una tarea: el selector de «Responsable».
	// Vacío (sin directorio), el selector no se enseña, como en la web.
	Equipo          []api.ERPPersona
	AlAbrirAdjunto  func(api.ERPAdjunto)
	AlBorrarAdjunto func(api.ERPAdjunto)
	AlSubir         func()
	AlAnotar        func(texto string)
	AlReenviarVF    func()
	AlPlanear       func()
	AlIA            func()
	AlRequisitos    func()

	// Los controles y los campos que se teclean fuera de ellos viven lo que la ficha:
	// ver `Control`.
	controles []*Control
	nota      *ui.AreaTexto
	plan      *planTarea
	ancho     float32
}

// NuevaFicha abre la ficha de `r` (o una nueva con `r` nil, copiando los VALORES de
// `semilla` si la hay: el documento nuevo no hereda ni el id ni la referencia, que la
// pone la secuencia al guardar).
func NuevaFicha(tr *i18n.Translator, m Modelo, r, semilla *Registro, etiquetas map[string]string) *Ficha {
	f := &Ficha{
		tr: tr, loc: tr.Locale(), Modelo: m, Registro: r,
		Rotulos: map[string]string{}, Errores: map[string]string{},
	}
	for k, v := range etiquetas {
		f.Rotulos[k] = v
	}
	origen := r
	if origen == nil {
		origen = semilla
	}
	f.Valores = ValoresIniciales(m, origen)
	switch {
	case r != nil && r.Estado != "":
		f.EstadoActual = r.Estado
	case len(m.Estados) > 0:
		f.EstadoActual = m.Estados[0].Valor
	}
	for _, c := range m.Campos {
		f.controles = append(f.controles, nuevoControl(c, f.Valores[c.Clave], f))
	}
	f.nota = ui.NuevaAreaTexto(tr.T("history.notePlaceholder", nil))
	f.nota.Filas = 2
	f.nota.Propia = &CampoTarea
	return f
}

func (f *Ficha) t(clave string) string { return f.tr.T(clave, nil) }

// fijar apunta el valor de un campo y quita su error: quien lo corrige no tiene por
// qué seguir viéndolo en rojo.
func (f *Ficha) fijar(clave, valor string) {
	f.Valores[clave] = valor
	delete(f.Errores, clave)
}

// Rellenar escribe lo que Sytue AI ha leído: solo esos campos, el resto se queda.
func (f *Ficha) Rellenar(data map[string]any, etiquetas map[string]string) {
	leidos := ValoresIniciales(f.Modelo, &Registro{Data: data})
	for _, k := range f.controles {
		if _, hay := data[k.Campo.Clave]; hay {
			f.Valores[k.Campo.Clave] = leidos[k.Campo.Clave]
			k.ponerValor(leidos[k.Campo.Clave])
		}
	}
	for id, n := range etiquetas {
		f.Rotulos[id] = n
	}
}

// Comprobar es la comprobación previa de la web: las relaciones obligatorias las mira
// este código, porque el control es un botón y no un campo que el formulario valide.
// Devuelve si se puede guardar.
func (f *Ficha) Comprobar() bool {
	faltan := map[string]string{}
	for _, c := range f.Modelo.Campos {
		if c.Tipo == api.ERPRelacion && c.Requerido && strings.TrimSpace(f.Valores[c.Clave]) == "" {
			faltan[c.Clave] = f.t("form.required")
		}
	}
	if len(faltan) == 0 {
		return true
	}
	f.Errores, f.Fallo = faltan, ""
	return false
}

// Enfocable es el primer control que se teclea, para darle el foco al abrir.
func (f *Ficha) Enfocable() fyne.Focusable {
	for _, k := range f.controles {
		if e := k.enfocable(); e != nil {
			return e
		}
	}
	return nil
}

// anchoCampos es el ancho útil del formulario.
func (f *Ficha) anchoCampos() float32 { return max(f.ancho-tema.Borde-2*FichaPX, 0) }

// Titulo es lo que dice la cabecera: el nombre del registro, o «Nuevo · Modelo».
func (f *Ficha) Titulo() string {
	if f.Registro != nil {
		return NombreDe(*f.Registro)
	}
	return f.t("action.new") + " · " + f.Modelo.Etiqueta
}

// Objeto arma el panel para `ancho` (el del `<aside>`, con su filete) y lo desplaza.
func (f *Ficha) Objeto(v tema.Variante, ancho float32) fyne.CanvasObject {
	f.ancho = ancho
	p := &pilaFicha{}
	add := func(o fyne.CanvasObject, alto, arriba float32, completo bool) {
		p.bloques = append(p.bloques, bloqueFicha{o, alto, arriba, completo})
	}
	cab, altoCab := f.cabecera(v)
	add(cab, altoCab, 0, false)
	siguiente := CabeceraFichaMB
	if f.Registro != nil && len(f.Modelo.Estados) > 0 {
		b, alto := f.barraEstado(v)
		add(b, alto, siguiente, true)
		siguiente = CabeceraFichaMB
	}
	for _, a := range []struct {
		texto        string
		fondo, tinta string
	}{{f.Fallo, "danger-bg", "danger-text"}, {f.Aviso, "success-bg", "success-text"}} {
		if a.texto == "" {
			continue
		}
		o, alto := avisoFicha(v, a.texto, a.fondo, a.tinta, f.anchoCampos())
		add(o, alto, siguiente, false)
		siguiente = AvisoFichaMB
	}
	for i, k := range f.controles {
		o, alto := f.bloqueCampo(v, k)
		if i > 0 {
			siguiente = SeparaCampos
		}
		add(o, alto, siguiente, false)
	}
	if len(f.Asientos) > 0 {
		o, alto := f.seccionAsientos(v)
		add(o, alto, SeccionMT, false)
	}
	if f.Registro != nil {
		o, alto := f.seguimiento(v)
		add(o, alto, SeccionMT, false)
		o, alto = f.trazabilidad(v)
		add(o, alto, SeccionMT, false)
	}
	pie, altoPie := f.pie()
	add(pie, altoPie, SeccionMT, false)

	cuerpo := container.New(p, p.objetos()...)
	aside := &panelFicha{
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		scroll: desplazable(cuerpo),
		movil:  f.Movil,
	}
	return container.New(aside, aside.fondo, aside.scroll, aside.filete)
}

// panelFicha es el `<aside>`: fondo de superficie, el filete de la izquierda (salvo en
// el móvil, donde va a pantalla completa) y el formulario desplazable.
type panelFicha struct {
	fondo, filete *canvas.Rectangle
	scroll        fyne.CanvasObject
	movil         bool
}

func (p *panelFicha) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (p *panelFicha) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	p.fondo.Resize(t)
	x := tema.Borde
	if p.movil {
		x = 0
	}
	p.filete.Hidden = p.movil
	p.filete.Resize(fyne.NewSize(tema.Borde, t.Height))
	p.scroll.Move(fyne.NewPos(x, 0))
	p.scroll.Resize(fyne.NewSize(t.Width-x, t.Height))
}

// bloqueFicha es un bloque del formulario con el margen que lo separa del anterior.
// `completo` es el `-mx-6` de la barra de estado, que va de borde a borde.
type bloqueFicha struct {
	obj          fyne.CanvasObject
	alto, arriba float32
	completo     bool
}

// pilaFicha apila los bloques con el `px-6 py-5` del formulario.
type pilaFicha struct{ bloques []bloqueFicha }

func (p *pilaFicha) objetos() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, len(p.bloques))
	for i, b := range p.bloques {
		out[i] = b.obj
	}
	return out
}

func (p *pilaFicha) MinSize([]fyne.CanvasObject) fyne.Size {
	alto := 2 * FichaPY
	for _, b := range p.bloques {
		alto += b.arriba + b.alto
	}
	return fyne.NewSize(0, alto)
}

func (p *pilaFicha) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	y := FichaPY
	for _, b := range p.bloques {
		y += b.arriba
		x, ancho := FichaPX, t.Width-2*FichaPX
		if b.completo {
			x, ancho = 0, t.Width
		}
		b.obj.Move(fyne.NewPos(x, y))
		b.obj.Resize(fyne.NewSize(max(ancho, 0), b.alto))
		y += b.alto
	}
}

// --- La cabecera ------------------------------------------------------------

// cabecera es el título con la referencia debajo y, a la derecha, UBL, imprimir,
// Facturae, su aviso con FACe, Sytue AI y Cerrar, en ese orden y los que toquen.
func (f *Ficha) cabecera(v tema.Variante) (fyne.CanvasObject, float32) {
	c := &cabeceraFicha{}
	r := f.Registro
	botones := f.botonesCabecera()
	util := f.anchoCampos()
	c.anchoTitulo = util // `basis-full`: el título y la referencia tienen su fila.
	c.botones = botones
	c.hueco = tema.Espacio(2) // `gap-2` entre filas y acciones.
	anchos, altos := make([]float32, len(botones)), make([]float32, len(botones))
	for i, b := range botones {
		m := b.MinSize()
		anchos[i], altos[i] = m.Width, m.Height
	}
	var ocupa fyne.Size
	c.posBotones, ocupa = Envolver(anchos, altos, c.hueco, c.hueco, util)
	c.altoBotones = ocupa.Height
	if r != nil && f.Modelo.Cobro && f.Facturae {
		c.aviso, c.altoAviso = f.avisoFacturae(v, util)
	}
	// `sytue-h4`: desde que la ficha va DENTRO de la hoja, su título es de la forma.
	c.titulo = ui.Texto(f.Titulo(), ui.EscalonTitulo(4), ui.PesoTitulo(4), tema.Color("text", v))
	c.completo = f.Titulo()
	objs := []fyne.CanvasObject{c.titulo}
	alto := ui.EscalonTitulo(4).Linea
	if r != nil && r.Ref != "" {
		cel := celdaERP{texto: r.Ref, mono: true, escalon: tema.TextoXS, tinta: "text-subtle"}
		for _, linea := range lineasCelda(cel, c.anchoTitulo) {
			t := textoCelda(linea, cel, v)
			c.ref = append(c.ref, t)
			objs = append(objs, t)
		}
		alto += float32(len(c.ref)) * tema.TextoXS.Linea
	}
	c.altoTitulo = alto
	alto += c.hueco + c.altoBotones
	objs = append(objs, botones...)
	if c.aviso != nil {
		alto += c.hueco + c.altoAviso
		objs = append(objs, c.aviso)
	}
	c.alto = alto
	return container.New(c, objs...), alto
}

// botonesCabecera reúne las acciones del registro en el mismo orden de la web.
func (f *Ficha) botonesCabecera() []fyne.CanvasObject {
	r := f.Registro
	botones := make([]fyne.CanvasObject, 0, 5)
	boton := func(texto, nombre string, ic iconos.Icono, al func()) *ui.Boton {
		b := ui.NuevoBoton(texto, al).Con(ui.Fantasma, ui.TallaSM)
		if ic != iconos.Ninguno {
			b.ConIcono(ic, 14)
			if texto != "" {
				b.MargenIcono = 1 * 0.25 * tema.Rem
			}
		}
		b.Rotulo = nombre
		return b
	}
	descargar := func(que string) func() {
		return func() {
			if f.AlDescargar != nil {
				f.AlDescargar(que)
			}
		}
	}
	if r != nil && f.Modelo.Cobro {
		botones = append(botones, boton("UBL", f.t("form.downloadUbl"), iconos.Descargar, descargar("ubl")))
	}
	if r != nil && f.Modelo.Clave == api.ERPFacturaVenta {
		botones = append(botones, NuevoEnlaceIcono(iconos.Recibo, f.t("form.print"), descargar("imprimir")))
	}
	if r != nil && f.Modelo.Cobro && f.Facturae {
		botones = append(botones, boton("Facturae", f.t("form.downloadFacturae"), iconos.Descargar, descargar("facturae")))
	}
	if !f.EnPapelera {
		// «Rellenar con Sytue AI» abre `RellenarIA`.
		ia := boton("", f.t("ai.fill"), iconos.IA, f.AlIA)
		ia.Inerte = f.AlIA == nil
		botones = append(botones, ia)
	}
	botones = append(botones, boton(f.t("action.close"), "", iconos.Ninguno, f.AlCerrar))
	return botones
}

// avisoFacturae ocupa una fila propia con su enlace FACe al final.
func (f *Ficha) avisoFacturae(v tema.Variante, util float32) (fyne.CanvasObject, float32) {
	// `order-last basis-full`: el aviso nunca compite con el título ni
	// estrecha las acciones. El enlace sigue siendo una pieza en línea.
	palabras := strings.Fields(f.t("sales.facturaeHint"))
	ps := make([]pieza, 0, len(palabras)+1)
	espacio := ui.Medir(" ", tema.TextoXS, ui.Normal).Width
	for _, palabra := range palabras {
		ps = append(ps, textoPieza(palabra, tema.TextoXS, ui.Normal, "text-muted", v))
	}
	face := ui.NuevoEnlace("FACe", tema.TextoXS, ui.Normal, "text-muted", func() {
		abrirURL("https://proveedores.face.gob.es/inicio")
	})
	face.Subrayado = true
	ps = append(ps, piezaDe(face, face.MinSize()))
	anchos, altos := medidasDe(ps)
	pos, ocupa := Envolver(anchos, altos, espacio, 0, util)
	l := &lienzo{}
	l.piezas(ps, pos, 0, 0)
	return l.objeto(ocupa.Height)
}

// cabeceraFicha coloca título, acciones envueltas y ayuda: `flex-wrap items-center gap-2`.
type cabeceraFicha struct {
	titulo      *canvas.Text
	completo    string
	ref         []*canvas.Text
	anchoTitulo float32
	botones     []fyne.CanvasObject
	posBotones  []fyne.Position
	altoBotones float32
	aviso       fyne.CanvasObject
	altoAviso   float32
	altoTitulo  float32
	hueco       float32
	alto        float32
}

func (c *cabeceraFicha) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, c.alto) }

func (c *cabeceraFicha) Layout(_ []fyne.CanvasObject, _ fyne.Size) {
	h4 := ui.EscalonTitulo(4)
	ui.Truncar(c.titulo, c.completo, h4, ui.PesoTitulo(4), c.anchoTitulo)
	ui.ColocarTexto(c.titulo, h4, 0, 0)
	for i, t := range c.ref {
		ui.ColocarTexto(t, tema.TextoXS, 0, h4.Linea+float32(i)*tema.TextoXS.Linea)
	}
	y := c.altoTitulo + c.hueco
	for i, b := range c.botones {
		b.Resize(b.MinSize())
		b.Move(c.posBotones[i].AddXY(0, y))
	}
	if c.aviso != nil {
		c.aviso.Move(fyne.NewPos(0, y+c.altoBotones+c.hueco))
		c.aviso.Resize(fyne.NewSize(c.anchoTitulo, c.altoAviso))
	}
}

// --- La barra de estado -----------------------------------------------------

// barraEstado es la franja de la etapa: a la izquierda el paso siguiente (y «Cancelado»
// si el modelo lo tiene) y a la derecha las etapas, que se pulsan todas.
func (f *Ficha) barraEstado(v tema.Variante) (fyne.CanvasObject, float32) {
	b := &barraEstado{
		fondo:  ui.NuevaCaja(tema.Color("surface-sunken", v), sinColor, 0),
		arriba: canvas.NewRectangle(tema.Color("border", v)),
		abajo:  canvas.NewRectangle(tema.Color("border", v)),
	}
	objs := append(b.fondo.Objetos(), b.arriba, b.abajo)
	est := f.Modelo.Estados
	idx := slices.IndexFunc(est, func(o api.ERPOpcion) bool { return o.Valor == f.EstadoActual })
	cambiar := func(e string) func() {
		return func() {
			if f.AlEstado != nil {
				f.AlEstado(e)
			}
		}
	}
	if idx >= 0 && idx+1 < len(est) && est[idx+1].Valor != "cancelado" {
		s := ui.NuevoBoton(est[idx+1].Etiqueta, cambiar(est[idx+1].Valor)).Con(ui.Primario, ui.TallaXS)
		s.Cargando = f.Guardando
		b.acciones = append(b.acciones, piezaDe(s, s.MinSize()))
	}
	cancelado := f.EstadoActual == "cancelado" || f.EstadoActual == "rechazado"
	for _, o := range est {
		if o.Valor == "cancelado" && !cancelado {
			c := ui.NuevoBoton(o.Etiqueta, cambiar("cancelado")).Con(ui.Fantasma, ui.TallaXS)
			c.Cargando = f.Guardando
			b.acciones = append(b.acciones, piezaDe(c, c.MinSize()))
		}
	}
	rot := f.t("form.pipeline") + ":"
	etapa := ui.NuevoVersalCon(rot, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v), TrackingEtapa)
	b.etapa = etapa
	b.etapas = append(b.etapas, pieza{nil, etapa.Ancho() + MargenRotuloFil, tema.TextoXS.Linea, false})
	objs = append(objs, etapa.Objetos()...)
	for _, o := range est {
		e := NuevaEtapa(o.Etiqueta, o.Valor == f.EstadoActual, cambiar(o.Valor))
		e.Inerte = f.Guardando
		b.etapas = append(b.etapas, piezaDe(e, e.MinSize()))
	}
	for _, p := range append(slices.Clone(b.acciones), b.etapas...) {
		if p.obj != nil {
			objs = append(objs, p.obj)
		}
	}
	b.colocar(f.anchoCampos())
	return container.New(b, objs...), b.alto
}

// barraEstado coloca la franja: `flex flex-wrap items-center justify-between gap-3`.
type barraEstado struct {
	fondo            *ui.Caja
	arriba, abajo    *canvas.Rectangle
	etapa            *ui.Versal
	acciones, etapas []pieza
	posA, posE       []fyne.Position
	tamA, tamE       fyne.Size
	dxE, dyE, dyA    float32
	alto             float32
}

func (b *barraEstado) colocar(util float32) {
	aa, ha := medidasDe(b.acciones)
	ae, he := medidasDe(b.etapas)
	b.posA, b.tamA = Envolver(aa, ha, HuecoAccionesEstado, HuecoAccionesEstado, util)
	b.posE, b.tamE = Envolver(ae, he, HuecoEtapas, HuecoEtapas, util)
	// Un grupo vacío no ocupa: `justify-between` con un hijo sin nada dentro sigue
	// siendo un hijo, pero de ancho cero.
	if b.tamA.Width+HuecoBarraEstado+b.tamE.Width <= util {
		fila := max(b.tamA.Height, b.tamE.Height)
		b.dyA, b.dxE, b.dyE = (fila-b.tamA.Height)/2, util-b.tamE.Width, (fila-b.tamE.Height)/2
		b.alto = fila + 2*BarraEstadoPY + 2*tema.Borde
		return
	}
	b.dyA, b.dxE, b.dyE = 0, 0, b.tamA.Height+HuecoBarraEstado
	b.alto = b.dyE + b.tamE.Height + 2*BarraEstadoPY + 2*tema.Borde
}

func (b *barraEstado) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, b.alto) }

func (b *barraEstado) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	b.fondo.Redimensionar(t)
	b.arriba.Resize(fyne.NewSize(t.Width, tema.Borde))
	b.abajo.Move(fyne.NewPos(0, t.Height-tema.Borde))
	b.abajo.Resize(fyne.NewSize(t.Width, tema.Borde))
	x0, y0 := FichaPX, tema.Borde+BarraEstadoPY
	colocarPiezas(b.acciones, b.posA, x0, y0+b.dyA)
	for i, p := range b.etapas {
		pos := fyne.NewPos(x0+b.dxE+b.posE[i].X, y0+b.dyE+b.posE[i].Y)
		if p.obj == nil {
			b.etapa.Colocar(pos.X, pos.Y)
			continue
		}
		p.obj.Resize(fyne.NewSize(p.ancho, p.alto))
		p.obj.Move(pos)
	}
}

// Etapa es un paso del flujo: una píldora que lleva el registro a ese estado. La activa
// va rellena de `primary` con la tinta de `on-primary` (PR #840: el `primary-text` de
// antes medía 1,87:1 sobre la tinta); las demás, perfiladas sobre `surface`.
type Etapa struct {
	ui.Pulsable
	Rotulo string
	Activa bool
}

// NuevaEtapa crea una etapa.
func NuevaEtapa(rotulo string, activa bool, alPulsar func()) *Etapa {
	e := &Etapa{Rotulo: rotulo, Activa: activa}
	e.Nombre, e.AlPulsar = rotulo, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

func (e *Etapa) peso() ui.Peso {
	if e.Activa {
		return ui.SemiNegrita
	}
	return ui.Medio
}

// MinSize es el rótulo con `px-2.5 py-1`: la activa sin borde (21), las demás con él
// (23). Medido.
func (e *Etapa) MinSize() fyne.Size {
	e.ExtendBaseWidget(e)
	borde := 2 * tema.Borde
	if e.Activa {
		borde = 0
	}
	return fyne.NewSize(ui.Medir(e.Rotulo, tema.TextoXS, e.peso()).Width+2*EtapaPX+borde,
		tema.TextoXS.Linea+2*EtapaPY+borde)
}

// CreateRenderer construye la píldora.
func (e *Etapa) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &etapaRenderer{
		e:      e,
		aro:    ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
		sombra: ui.NuevaSombra(tema.SombraXS, tema.Forma("radius-control")),
		caja:   ui.NuevaCaja(sinColor, sinColor, tema.Forma("radius-control")),
		texto:  ui.Texto(e.Rotulo, tema.TextoXS, e.peso(), tema.Color("text-muted", v)),
	}
	r.Refresh()
	return r
}

type etapaRenderer struct {
	e      *Etapa
	aro    *ui.Aro
	sombra *ui.Sombra
	caja   *ui.Caja
	texto  *canvas.Text
}

func (r *etapaRenderer) MinSize() fyne.Size { return r.e.MinSize() }
func (r *etapaRenderer) Objects() []fyne.CanvasObject {
	objs := r.aro.Objetos()
	if r.e.Activa {
		objs = append(objs, r.sombra.Objeto())
	}
	objs = append(objs, r.caja.Objetos()...)
	return append(objs, r.texto)
}
func (r *etapaRenderer) Destroy() {}

func (r *etapaRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.aro.Colocar(t)
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, v)
	r.caja.Redimensionar(t)
	ui.ColocarTexto(r.texto, tema.TextoXS, (t.Width-ui.Medir(r.e.Rotulo, tema.TextoXS, r.e.peso()).Width)/2,
		(t.Height-tema.TextoXS.Linea)/2)
}

func (r *etapaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	switch {
	case r.e.Activa:
		r.caja.Pintar(tema.Color("primary", v), sinColor)
		r.texto.Color = tema.Color("on-primary", v)
	default:
		r.caja.Pintar(tema.Color("surface", v), tema.Color("border", v))
		r.texto.Color = tema.Color("text-muted", v)
		if r.e.Encima() {
			r.texto.Color = tema.Color("text", v)
		}
	}
	r.texto.FontSource = r.e.peso().Cara()
	r.texto.Refresh()
	r.aro.Ver(r.e.Enfocado())
	r.aro.Pintar(v)
}

// --- Los avisos y el pie ------------------------------------------------------

// avisoFicha es la banda roja o verde de la ficha: `rounded-md px-3 py-2 text-sm`, y el
// texto se reparte en líneas si no cabe.
func avisoFicha(v tema.Variante, texto, fondo, tinta string, ancho float32) (fyne.CanvasObject, float32) {
	p := ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, tinta, v)
	alto := ui.AltoParrafo(p, ancho-2*AvisoFichaPX) + 2*AvisoFichaPY
	caja := ui.NuevaCaja(tema.Color(fondo, v), sinColor, tema.RadioMD)
	l := &cajaConTexto{caja: caja, p: p, px: AvisoFichaPX, py: AvisoFichaPY}
	return container.New(l, append(caja.Objetos(), p)...), alto
}

type cajaConTexto struct {
	caja   *ui.Caja
	p      *ui.Parrafo
	px, py float32
}

func (c *cajaConTexto) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (c *cajaConTexto) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.caja.Redimensionar(t)
	c.p.Move(fyne.NewPos(c.px, c.py))
	c.p.Resize(fyne.NewSize(t.Width-2*c.px, t.Height-2*c.py))
}

// pie es Guardar y, con registro, Llevar al libro, Duplicar como borrador y Eliminar (o
// Restaurar desde la papelera). La fila NO salta: en la web, con los cuatro, el último
// sale del panel.
func (f *Ficha) pie() (fyne.CanvasObject, float32) {
	l := &pieFicha{}
	guardar := ui.NuevoBoton(f.t("action.save"), f.AlGuardar).Con(ui.Primario, ui.TallaMD)
	guardar.Cargando = f.Guardando
	l.izq = append(l.izq, guardar)
	r := f.Registro
	if f.Modelo.Contable && r != nil {
		a := ui.NuevoBoton(f.t("action.post"), f.AlAsentar).Con(ui.Secundario, ui.TallaMD)
		a.Inerte = f.Guardando
		l.izq = append(l.izq, a)
	}
	if r != nil {
		d := ui.NuevoBoton(f.t("action.duplicateDraft"), f.AlDuplicar).Con(ui.Fantasma, ui.TallaMD)
		d.Inerte = f.Guardando
		texto, al := f.t("action.delete"), f.AlBorrar
		if f.EnPapelera {
			texto, al = f.t("action.restore"), f.AlRestaurar
		}
		b := ui.NuevoBoton(texto, al).Con(ui.Fantasma, ui.TallaMD)
		b.Inerte = f.Guardando
		l.der = []*ui.Boton{d, b}
	}
	objs := make([]fyne.CanvasObject, 0, len(l.izq)+len(l.der))
	for _, b := range append(slices.Clone(l.izq), l.der...) {
		objs = append(objs, b)
	}
	return container.New(l, objs...), ui.AltoCampo(ui.CampoMD)
}

// pieFicha coloca el pie: los de la izquierda seguidos y los de la derecha empujados por
// el `ml-auto` del primero, sin bajar nunca de fila.
type pieFicha struct{ izq, der []*ui.Boton }

func (p *pieFicha) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (p *pieFicha) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	x := float32(0)
	for _, b := range p.izq {
		m := b.MinSize()
		b.Resize(m)
		b.Move(fyne.NewPos(x, 0))
		x += m.Width + HuecoPieFicha
	}
	var anchoDer float32
	for i, b := range p.der {
		anchoDer += b.MinSize().Width
		if i > 0 {
			anchoDer += HuecoPieFicha
		}
	}
	// `ml-auto`: se arriman a la derecha si sobra sitio; si no, siguen detrás.
	x = max(x, t.Width-anchoDer)
	for _, b := range p.der {
		m := b.MinSize()
		b.Resize(m)
		b.Move(fyne.NewPos(x, 0))
		x += m.Width + HuecoPieFicha
	}
}

// --- El enlace de imprimir ----------------------------------------------------

// EnlaceIcono es el `<a>` de imprimir: solo el icono de 14 con `px-2 py-1.5`, `rounded-md`
// y `hover:bg-bg-subtle`. Medido: 28×24,5.
type EnlaceIcono struct {
	ui.Pulsable
	Icono iconos.Icono
}

// NuevoEnlaceIcono crea el enlace.
func NuevoEnlaceIcono(i iconos.Icono, nombre string, alPulsar func()) *EnlaceIcono {
	e := &EnlaceIcono{Icono: i}
	e.Nombre, e.AlPulsar = nombre, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

// MinSize es el icono con su relleno.
func (e *EnlaceIcono) MinSize() fyne.Size {
	e.ExtendBaseWidget(e)
	return fyne.NewSize(14+2*ImprimirPX, 14+2*ImprimirPY)
}

// CreateRenderer construye el fondo y el icono.
func (e *EnlaceIcono) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// `rounded-control`: el radio del botón es de la forma.
	r := &enlaceIconoRenderer{
		e:     e,
		aro:   ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
		caja:  ui.NuevaCaja(sinColor, sinColor, tema.Forma("radius-control")),
		icono: iconos.Trazo(e.Icono, 14, "text", v),
	}
	r.Refresh()
	return r
}

type enlaceIconoRenderer struct {
	e     *EnlaceIcono
	aro   *ui.Aro
	caja  *ui.Caja
	icono *iconos.Dibujo
}

func (r *enlaceIconoRenderer) MinSize() fyne.Size { return r.e.MinSize() }
func (r *enlaceIconoRenderer) Objects() []fyne.CanvasObject {
	return append(append(r.aro.Objetos(), r.caja.Objetos()...), r.icono.Objeto())
}
func (r *enlaceIconoRenderer) Destroy() {}

func (r *enlaceIconoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.icono.Colocar(ImprimirPX, ImprimirPY, 14)
}

func (r *enlaceIconoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.e.Encima() {
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, sinColor)
	r.aro.Ver(r.e.Enfocado())
	r.aro.Pintar(v)
}
