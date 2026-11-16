// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El asistente de facturas (`AsistenteFacturas.tsx`)
//
// Guía de principio a fin lo que antes eran cuatro gestos sueltos: «¿qué factura?»,
// el documento de origen (un PDF que lee Sytue AI, una factura electrónica de
// proveedor o nada), la revisión en el alta rápida, adjuntar el original y llevarla al
// libro. Este tipo es el ESTADO y lo que se pinta; lo que habla con el servicio está en
// `asistente_facturas_pantalla.go`, como `PanelIA` e `ia_pantalla.go`.
//
// Solo el alta rápida crea la factura. Hasta entonces nada se escribe en el servicio,
// así que cancelar en los tres primeros pasos no deja restos; desde que existe, el
// asistente ya no vuelve atrás (adjuntar y asentar reintentan sobre ESE registro).

// Los cinco pasos de la web.
const (
	PasoFacturaTipo = iota
	PasoFacturaOrigen
	PasoFacturaRevisar
	PasoFacturaAdjuntar
	PasoFacturaAsentar
)

// Los topes del original: el PDF que lee Sytue AI y el XML de `asistencia/leer`
// (`maxImportBytes` del servicio).
const (
	MaxPDFFactura int64 = 10 * 1024 * 1024
	MaxXMLFactura int64 = 8 * 1024 * 1024
)

// camposResumenFactura son los que la revisión enseña antes de abrir el formulario.
var camposResumenFactura = []string{"proveedor", "cliente", "fecha", "base", "iva", "total"}

// camposEmisionFactura son la identidad y las marcas de emisión que una factura
// NUEVA para un cliente no hereda del documento de apoyo.
var camposEmisionFactura = []string{"externo_id", "tpv_operacion", "tpv_huella", "sesion", "medio_pago", "rectifica"}

// AsistenteFacturas es el estado del asistente.
type AsistenteFacturas struct {
	tr  *i18n.Translator
	loc i18n.Locale

	// Modelos son los del catálogo; el asistente usa los dos de facturas.
	Modelos []Modelo
	// PuedeCrear y PuedeAsentar son los permisos efectivos de la pantalla.
	PuedeCrear   func(clave string) bool
	PuedeAsentar func(modulo string) bool

	// Clave es la factura elegida: `factura_compra` o `factura_venta`.
	Clave string
	Paso  int
	// Fichero es la ruta del original elegido y Tamano lo que mide; vacío, a mano.
	Fichero string
	Tamano  int64
	// Semilla y Etiquetas es lo leído, que se lleva al alta rápida.
	Semilla   map[string]any
	Etiquetas map[string]string
	// Contacto es el proveedor leído que aún no está en contactos: se propone al crear
	// la relación desde el formulario.
	Contacto     map[string]any
	NumeroOrigen string
	// Registro es la factura ya creada (o la que ya existía). Desde que la hay no se
	// vuelve atrás.
	Registro *Registro
	Asiento  string
	Ocupado  bool
	Fallo    string
	// Ventana es el ancho de la ventana, que decide los rótulos del progreso.
	Ventana float32

	// AlElegirFichero abre el selector del sistema; nil, no lo hay (el móvil) y solo
	// queda la entrada a mano.
	AlElegirFichero func()
	AlPreparar      func()
	AlRevisar       func()
	AlAdjuntar      func()
	AlAsentar       func()
	AlAbrir         func()
	AlCambio        func()
}

// NuevoAsistenteFacturas abre en la factura recibida si se puede crear; si no, en la
// emitida, como la web.
func NuevoAsistenteFacturas(tr *i18n.Translator, modelos []Modelo, puedeCrear, puedeAsentar func(string) bool) *AsistenteFacturas {
	x := &AsistenteFacturas{
		tr: tr, loc: tr.Locale(), Modelos: modelos, PuedeCrear: puedeCrear, PuedeAsentar: puedeAsentar,
		Clave: api.ERPFacturaVenta, Semilla: map[string]any{}, Etiquetas: map[string]string{},
	}
	if x.hay(api.ERPFacturaCompra) && puedeCrear(api.ERPFacturaCompra) {
		x.Clave = api.ERPFacturaCompra
	}
	return x
}

func (x *AsistenteFacturas) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return x.tr.T(clave, vars[0])
	}
	return x.tr.T(clave, nil)
}

func (x *AsistenteFacturas) hay(clave string) bool {
	return slices.ContainsFunc(x.Modelos, func(m Modelo) bool { return m.Clave == clave })
}

// Modelo es el de la factura elegida; false si el catálogo no la tiene.
func (x *AsistenteFacturas) Modelo() (Modelo, bool) {
	for _, m := range x.Modelos {
		if m.Clave == x.Clave {
			return m, true
		}
	}
	return Modelo{}, false
}

// Recibida es la factura de proveedor: la única que se lee de un XML.
func (x *AsistenteFacturas) Recibida() bool { return x.Clave == api.ERPFacturaCompra }

// puedeSeguir es el `canContinue` de los tres primeros pasos.
func (x *AsistenteFacturas) puedeSeguir() bool {
	_, ok := x.Modelo()
	return ok && x.PuedeCrear(x.Clave)
}

// creada dice si ya hay factura: entonces el asistente no vuelve atrás.
func (x *AsistenteFacturas) creada() bool { return x.Registro != nil }

func (x *AsistenteFacturas) cambio() {
	if x.AlCambio != nil {
		x.AlCambio()
	}
}

// olvidarLectura deja el asistente como antes de leer nada.
func (x *AsistenteFacturas) olvidarLectura() {
	x.Semilla, x.Etiquetas, x.Contacto, x.NumeroOrigen, x.Fallo = map[string]any{}, map[string]string{}, nil, "", ""
}

// Elegir cambia de factura y olvida el original y lo leído: un PDF elegido para una
// recibida no vale como apoyo de una emitida, ni al revés.
func (x *AsistenteFacturas) Elegir(clave string) {
	if x.Ocupado || x.creada() || !x.hay(clave) || !x.PuedeCrear(clave) {
		return
	}
	x.Clave, x.Fichero, x.Tamano = clave, "", 0
	x.olvidarLectura()
	x.cambio()
}

// PonerFichero apunta el original elegido.
func (x *AsistenteFacturas) PonerFichero(ruta string, tamano int64) {
	if x.Ocupado || x.creada() {
		return
	}
	x.Fichero, x.Tamano, x.Fallo = ruta, tamano, ""
	x.cambio()
}

// Manual salta a la revisión sin original: solo con la fecha de hoy.
func (x *AsistenteFacturas) Manual() {
	if x.Ocupado || x.creada() {
		return
	}
	x.Fichero, x.Tamano = "", 0
	x.olvidarLectura()
	x.Semilla["fecha"] = time.Now().Format(time.DateOnly)
	x.Paso = PasoFacturaRevisar
	x.cambio()
}

// IrA es volver por el progreso o con «Atrás». Desde que hay factura, o mientras se
// lee, no se mueve.
func (x *AsistenteFacturas) IrA(n int) {
	if x.Ocupado || x.creada() || n < 0 || n > PasoFacturaRevisar {
		return
	}
	x.Paso, x.Fallo = n, ""
	x.cambio()
}

// esPDF y esXML miran la extensión: es lo que el selector del sistema garantiza.
func esPDF(ruta string) bool { return strings.EqualFold(filepath.Ext(ruta), ".pdf") }
func esXML(ruta string) bool { return strings.EqualFold(filepath.Ext(ruta), ".xml") }

// Comprobar valida el original antes de mandarlo: un PDF, o un XML si la factura es
// recibida, y dentro de su tope. Devuelve el fallo, vacío si vale.
func (x *AsistenteFacturas) Comprobar() string {
	pdf := esPDF(x.Fichero)
	if !pdf && (!esXML(x.Fichero) || !x.Recibida()) {
		return x.t("invoiceAssist.invalidFile")
	}
	limite := MaxXMLFactura
	if pdf {
		limite = MaxPDFFactura
	}
	if x.Tamano > limite {
		return x.t("invoiceAssist.fileTooLarge", i18n.Vars{"limit": i18n.FormatBytes(x.loc, float64(limite))})
	}
	return ""
}

// Siguiente es el botón principal de los tres primeros pasos.
func (x *AsistenteFacturas) Siguiente() {
	if x.Ocupado || x.creada() || !x.puedeSeguir() {
		return
	}
	switch x.Paso {
	case PasoFacturaTipo:
		x.Paso, x.Fallo = PasoFacturaOrigen, ""
		x.cambio()
	case PasoFacturaOrigen:
		if x.Fichero == "" {
			x.Manual()
			return
		}
		if fallo := x.Comprobar(); fallo != "" {
			x.Fallo = fallo
			x.cambio()
			return
		}
		if x.AlPreparar != nil {
			x.AlPreparar()
		}
	case PasoFacturaRevisar:
		x.Fallo = ""
		if x.AlRevisar != nil {
			x.AlRevisar()
		}
	}
}

// Leido aplica la lectura del original. Si la factura ya había entrado, salta a
// adjuntar con la existente en vez de preparar otra igual.
func (x *AsistenteFacturas) Leido(r api.ERPRelleno) {
	x.Ocupado, x.Fallo = false, ""
	x.NumeroOrigen = r.NumeroOrigen
	if r.Existente != nil {
		e := *r.Existente
		x.Registro, x.Paso = &e, PasoFacturaAdjuntar
		x.cambio()
		return
	}
	data := maps.Clone(r.Data)
	if data == nil {
		data = map[string]any{}
	}
	if !x.Recibida() {
		for _, c := range camposEmisionFactura {
			delete(data, c)
		}
	}
	x.Semilla = data
	x.Etiquetas = maps.Clone(r.Etiquetas)
	if x.Etiquetas == nil {
		x.Etiquetas = map[string]string{}
	}
	// Del contacto solo los escalares: es la semilla de un formulario, no un documento.
	x.Contacto = nil
	if x.Recibida() {
		for k, v := range r.Contacto {
			switch v.(type) {
			case nil, string, float64, bool:
				if x.Contacto == nil {
					x.Contacto = map[string]any{}
				}
				x.Contacto[k] = v
			}
		}
	}
	x.Paso = PasoFacturaRevisar
	x.cambio()
}

// Creada es el registro que devolvió el alta rápida: con original se adjunta; sin él,
// se pasa directamente a contabilizar.
func (x *AsistenteFacturas) Creada(r Registro) {
	if x.creada() {
		return
	}
	x.Registro, x.Fallo = &r, ""
	x.Paso = PasoFacturaAsentar
	if x.Fichero != "" {
		x.Paso = PasoFacturaAdjuntar
	}
	x.cambio()
}

// Adjuntado es el original ya subido: queda contabilizarla.
func (x *AsistenteFacturas) Adjuntado() {
	x.Ocupado, x.Fallo, x.Paso = false, "", PasoFacturaAsentar
	x.cambio()
}

// Asentado apunta el asiento: la factura ya no es un borrador.
func (x *AsistenteFacturas) Asentado(ref string) {
	x.Ocupado, x.Fallo, x.Asiento = false, "", ref
	if x.Registro != nil {
		x.Registro.Estado = "confirmado"
	}
	x.cambio()
}

// PuedeAsentarla es la condición del botón «Llevar al libro» del último paso.
func (x *AsistenteFacturas) PuedeAsentarla() bool {
	m, ok := x.Modelo()
	return ok && x.Registro != nil && x.Asiento == "" && x.Registro.Estado == "borrador" &&
		m.Contable && x.PuedeAsentar != nil && x.PuedeAsentar(m.Modulo)
}

// Titulos son los de los cinco pasos, en el orden de la web.
func (x *AsistenteFacturas) Titulos() []string {
	return []string{
		x.t("invoiceAssist.kind"), x.t("invoiceAssist.source"), x.t("invoiceAssist.review"),
		x.t("docs.title"), x.t("action.post"),
	}
}

// --- Lo que se pinta --------------------------------------------------------------------

// Cuerpo es el `Wizard`: el progreso, el encabezado del paso, su pista y su contenido,
// con el fallo arriba como el `role="alert"` del Modal.
func (x *AsistenteFacturas) Cuerpo(v tema.Variante, ancho float32) fyne.CanvasObject {
	p := &pila{}
	hueco := float32(0)
	add := func(o fyne.CanvasObject, alto float32) {
		p.add(o, alto, hueco)
		hueco = tema.Espacio(6) // el `gap-6` del Wizard
	}
	if x.Fallo != "" {
		o, h := ayudaPeligro(x.Fallo, ancho)
		add(o, h)
	}
	pasos := ui.NuevosPasos(x.Titulos()...)
	pasos.Actual, pasos.Ventana = x.Paso, x.Ventana
	pasos.RotuloIr = func(n int) string { return x.tr.T("ui.wizard.goToStep", i18n.Vars{"n": n}) }
	// Como el Wizard: ni en un paso terminal ni mientras trabaja; y, como el asistente,
	// tampoco desde que la factura existe.
	if x.Paso < PasoFacturaAdjuntar && !x.Ocupado && !x.creada() {
		pasos.AlElegir = x.IrA
	}
	add(pasos, ui.AltoPasos)

	titulo := ui.NuevoParrafo(x.Titulos()[x.Paso], ui.EscalonTitulo(3), ui.PesoTitulo(3), "text", v).A(ancho)
	alto := ui.AltoParrafo(titulo, ancho)
	cab := &pila{}
	cab.add(titulo, alto, 0)
	if d := x.descripcion(); d != "" {
		pista := ui.NuevoParrafo(d, tema.TextoBase, ui.Normal, "text-muted", v).A(ancho)
		cab.add(pista, ui.AltoParrafo(pista, ancho), 1.5*0.25*tema.Rem)
	}
	o, h := cab.objeto()
	add(o, h)

	var c fyne.CanvasObject
	switch x.Paso {
	case PasoFacturaTipo:
		c, h = x.cuerpoTipo(ancho)
	case PasoFacturaOrigen:
		c, h = x.cuerpoOrigen(v, ancho)
	case PasoFacturaRevisar:
		c, h = x.cuerpoRevisar(v, ancho)
	case PasoFacturaAdjuntar:
		c, h = x.cuerpoAdjuntar(v, ancho)
	default:
		c, h = x.cuerpoAsentar(v, ancho)
	}
	add(c, h)
	r, _ := p.objeto()
	return r
}

func (x *AsistenteFacturas) descripcion() string {
	switch x.Paso {
	case PasoFacturaOrigen:
		if x.Recibida() {
			return x.t("invoiceAssist.sourceHint")
		}
		return x.t("invoiceAssist.issuedSourceHint")
	case PasoFacturaAdjuntar:
		return x.t("invoiceAssist.attachHint")
	}
	return ""
}

// cuerpoTipo son las dos `ChoiceCard`: recibida y emitida.
func (x *AsistenteFacturas) cuerpoTipo(ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	for i, op := range []struct{ clave, titulo string }{
		{api.ERPFacturaCompra, x.t("invoiceAssist.received")},
		{api.ERPFacturaVenta, x.t("invoiceAssist.issued")},
	} {
		desc := ""
		for _, m := range x.Modelos {
			if m.Clave == op.clave {
				desc = m.Etiqueta
			}
		}
		clave := op.clave
		c := ui.NuevaTarjetaEleccion(op.titulo, desc, func() { x.Elegir(clave) }).A(ancho)
		c.Elegida = x.Clave == clave
		c.Inerte = !x.hay(clave) || !x.PuedeCrear(clave) || x.Ocupado
		arriba := float32(0)
		if i > 0 {
			arriba = tema.Espacio(3)
		}
		p.add(c, ui.AltoTarjetaEleccion(c, ancho), arriba)
	}
	return p.objeto()
}

// cuerpoOrigen es el campo del original: el selector del sistema detrás de «Subir»,
// el nombre de lo elegido, los topes y, con fichero, la salida a mano.
func (x *AsistenteFacturas) cuerpoOrigen(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	if x.AlElegirFichero != nil {
		rot := ui.NuevoParrafo(x.t("docs.title"), tema.TextoSM, ui.SemiNegrita, "text", v).A(ancho)
		p.add(rot, ui.AltoParrafo(rot, ancho), 0)
		b := ui.NuevoBoton(x.t("docs.upload"), func() {
			if !x.Ocupado && !x.creada() {
				x.AlElegirFichero()
			}
		}).Con(ui.Secundario, ui.TallaMD)
		b.ConIcono(iconos.Subir, 16)
		b.MargenIcono = HuecoDos
		b.Inerte = x.Ocupado
		ps := []pieza{piezaDe(b, b.MinSize())}
		if x.Fichero != "" {
			nombre := filepath.Base(x.Fichero)
			t := ui.Texto(nombre, tema.TextoSM, ui.Normal, tema.Color("text", v))
			cabe := max(ancho-b.MinSize().Width-HuecoDos, 0)
			w := ui.Truncar(t, nombre, tema.TextoSM, ui.Normal, cabe)
			ps = append(ps, pieza{obj: t, ancho: w, alto: tema.TextoSM.Linea, texto: true})
		}
		o, h := botonera(ps, ancho)
		p.add(o, h, tema.Espacio(2))
		topes := "PDF · " + i18n.FormatBytes(x.loc, float64(MaxPDFFactura))
		if x.Recibida() {
			topes += " / XML · " + i18n.FormatBytes(x.loc, float64(MaxXMLFactura))
		}
		nota := ui.NuevoParrafo(topes, tema.TextoXS, ui.Normal, "text-muted", v).A(ancho)
		p.add(nota, ui.AltoParrafo(nota, ancho), tema.Espacio(3))
	}
	if x.Fichero != "" {
		m := ui.NuevoBoton(x.t("invoiceAssist.manual"), x.Manual).Con(ui.Fantasma, ui.TallaMD)
		m.Inerte = x.Ocupado
		p.add(container.New(ui.Fila{}, m), m.MinSize().Height, tema.Espacio(3))
	}
	if len(p.bloques) == 0 {
		// Sin selector solo queda escribirla: lo dice el botón principal.
		nota := ui.NuevoParrafo(x.t("invoiceAssist.manual"), tema.TextoSM, ui.Normal, "text-muted", v).A(ancho)
		p.add(nota, ui.AltoParrafo(nota, ancho), 0)
	}
	return p.objeto()
}

// cuerpoRevisar es el resumen de lo leído (el `<dl>`) y, si el proveedor no está en
// contactos, su tarjeta.
func (x *AsistenteFacturas) cuerpoRevisar(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	pista := ui.NuevoParrafo(x.t("ai.filled"), tema.TextoSM, ui.Normal, "text-muted", v).A(ancho)
	p.add(pista, ui.AltoParrafo(pista, ancho), 0)
	var filas [][2]string
	if x.NumeroOrigen != "" {
		filas = append(filas, [2]string{x.t("invoiceAssist.source"), x.NumeroOrigen})
	}
	if m, ok := x.Modelo(); ok {
		for _, c := range m.Campos {
			valor, hay := x.Semilla[c.Clave]
			if !slices.Contains(camposResumenFactura, c.Clave) || !hay || valor == nil {
				continue
			}
			texto := Celda(c, valor, x.loc, x.Etiquetas)
			if s, _ := valor.(string); c.Tipo == api.ERPRelacion && x.Etiquetas[s] == "" {
				// Leído pero sin contacto: se elige (o se crea) al revisar.
				texto = x.t("relation.choose")
			}
			filas = append(filas, [2]string{c.Etiqueta, texto})
		}
	}
	if len(filas) > 0 {
		o, h := listaDefiniciones(filas, v, ancho)
		p.add(o, h, tema.Espacio(4))
	}
	if x.Contacto != nil {
		o, h := x.tarjetaContacto(v, ancho)
		p.add(o, h, tema.Espacio(4))
	}
	return p.objeto()
}

// listaDefiniciones es `divide-y border-y` con `flex justify-between gap-4 py-2`.
func listaDefiniciones(filas [][2]string, v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	filete := func(y float32) {
		f := ui.NuevaCaja(tema.Color("border", v), tema.Color("border", v), 0)
		l.poner(container.New(&soloCaja{f}, f.Objetos()...), 0, y, ancho, tema.Borde)
	}
	py := tema.Espacio(2)
	y := float32(0)
	filete(y)
	y += tema.Borde
	for _, f := range filas {
		dt := ui.Texto(f[0], tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		dd := ui.Texto(f[1], tema.TextoSM, ui.Normal, tema.Color("text", v))
		mitad := max((ancho-tema.Espacio(4))/2, 0)
		wdd := ui.Truncar(dd, f[1], tema.TextoSM, ui.Normal, mitad)
		ui.Truncar(dt, f[0], tema.TextoSM, ui.Normal, max(ancho-wdd-tema.Espacio(4), 0))
		l.texto(dt, 0, y+py)
		l.texto(dd, ancho-wdd, y+py)
		y += 2*py + tema.TextoSM.Linea
		filete(y)
		y += tema.Borde
	}
	return l.objeto(y)
}

// tarjetaContacto es el `rounded-md border p-3` con el nombre, el NIF y la pista.
func (x *AsistenteFacturas) tarjetaContacto(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	pad := tema.Espacio(3)
	dentro := max(ancho-2*pad-2*tema.Borde, 0)
	p := &pila{}
	nombre, _ := x.Contacto["nombre"].(string)
	n := ui.NuevoParrafo(nombre, tema.TextoBase, ui.SemiNegrita, "text", v).A(dentro)
	p.add(n, ui.AltoParrafo(n, dentro), 0)
	if nif, _ := x.Contacto["nif"].(string); nif != "" {
		t := ui.NuevoParrafo(nif, tema.TextoSM, ui.Normal, "text-muted", v).A(dentro)
		p.add(t, ui.AltoParrafo(t, dentro), tema.Espacio(2))
	}
	h := ui.NuevoParrafo(x.t("invoiceAssist.contactHint"), tema.TextoSM, ui.Normal, "text-muted", v).A(dentro)
	p.add(h, ui.AltoParrafo(h, dentro), tema.Espacio(2))
	o, alto := p.objeto()
	l := &lienzo{}
	total := alto + 2*pad + 2*tema.Borde
	l.caja(ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD), ancho, total)
	l.poner(o, pad+tema.Borde, pad+tema.Borde, dentro, alto)
	return l.objeto(total)
}

// rotuloRegistro es «nombre · ref», el `role="status"` de los pasos terminales.
func (x *AsistenteFacturas) rotuloRegistro(v tema.Variante, ancho float32) (*ui.Parrafo, float32) {
	s := ""
	if x.Registro != nil {
		s = NombreDe(*x.Registro)
		if x.Registro.Ref != "" && x.Registro.Ref != s {
			s += " · " + x.Registro.Ref
		}
	}
	t := ui.NuevoParrafo(s, tema.TextoBase, ui.SemiNegrita, "text", v).A(ancho)
	return t, ui.AltoParrafo(t, ancho)
}

func (x *AsistenteFacturas) cuerpoAdjuntar(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	t, h := x.rotuloRegistro(v, ancho)
	p.add(t, h, 0)
	if x.Fichero != "" {
		n := ui.NuevoParrafo(filepath.Base(x.Fichero), tema.TextoBase, ui.Normal, "text-muted", v).A(ancho)
		p.add(n, ui.AltoParrafo(n, ancho), tema.Espacio(4))
	}
	return p.objeto()
}

func (x *AsistenteFacturas) cuerpoAsentar(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	t, h := x.rotuloRegistro(v, ancho)
	p.add(t, h, 0)
	m, _ := x.Modelo()
	var linea *ui.Parrafo
	switch {
	case x.Asiento != "":
		linea = ui.NuevoParrafo(x.t("form.posted", i18n.Vars{"ref": x.Asiento}), tema.TextoBase, ui.Normal, "success-text", v)
	case x.Registro != nil && x.Registro.Estado != "borrador":
		estado := x.Registro.Estado
		for _, e := range m.Estados {
			if e.Valor == estado {
				estado = e.Etiqueta
			}
		}
		linea = ui.NuevoParrafo(estado, tema.TextoBase, ui.Normal, "text", v)
	default:
		clave := "invoiceAssist.issueHint"
		if x.Recibida() {
			clave = "invoiceAssist.postHint"
		}
		linea = ui.NuevoParrafo(x.t(clave), tema.TextoSM, ui.Normal, "text-muted", v)
	}
	linea.A(ancho)
	p.add(linea, ui.AltoParrafo(linea, ancho), tema.Espacio(5))
	if !x.Recibida() && x.Asiento == "" {
		f := ui.NuevoParrafo(x.t("sales.facturaeHint"), tema.TextoSM, ui.Normal, "text-muted", v).A(ancho)
		p.add(f, ui.AltoParrafo(f, ancho), tema.Espacio(5))
	}
	return p.objeto()
}

// Pie es el pie del diálogo: un `ui.PieAcciones` con la primera acción a la izquierda,
// que en el teléfono pasa a la hoja con sus botones de 44 de alto y a todo el ancho.
func (x *AsistenteFacturas) Pie(alCerrar func()) *fyne.Container {
	return container.New(ui.PieAcciones{Hueco: ui.HuecoPieDialogo, Izquierda: 1}, x.Acciones(alCerrar)...)
}

// Acciones son las del paso, la de cerrar primero. En los tres primeros, las del
// Wizard: «Cancelar» a la izquierda y «Atrás / Continuar» a la derecha. En los dos
// últimos —que en la web van dentro del paso— las suyas: subir el original o llevarla
// al libro, abrirla y cerrar.
func (x *AsistenteFacturas) Acciones(alCerrar func()) []fyne.CanvasObject {
	boton := func(texto string, variante ui.Variante, al func()) *ui.Boton {
		return ui.NuevoBoton(texto, al).Con(variante, ui.TallaMD)
	}
	var izq, der []fyne.CanvasObject
	if x.Paso <= PasoFacturaRevisar {
		c := boton(x.tr.T("ui.wizard.cancel", nil), ui.Fantasma, alCerrar)
		c.Inerte = x.Ocupado
		izq = append(izq, c)
		if x.Paso > PasoFacturaTipo {
			a := boton(x.tr.T("ui.wizard.back", nil), ui.Secundario, func() { x.IrA(x.Paso - 1) })
			a.ConIcono(iconos.ChevronIzquierda, 16)
			a.MargenIcono = HuecoDos
			a.Inerte = x.Ocupado
			der = append(der, a)
		}
		rot := x.tr.T("ui.wizard.next", nil)
		switch x.Paso {
		case PasoFacturaOrigen:
			rot = x.t("invoiceAssist.manual")
			if x.Fichero != "" {
				rot = x.t("process.prepare")
			}
		case PasoFacturaRevisar:
			rot = x.t("invoiceAssist.review")
		}
		s := boton(rot, ui.Primario, x.Siguiente)
		s.Cargando, s.Inerte = x.Ocupado, x.Ocupado || !x.puedeSeguir()
		der = append(der, s)
	} else {
		if x.Paso == PasoFacturaAdjuntar {
			s := boton(x.t("docs.upload"), ui.Primario, func() {
				if x.AlAdjuntar != nil && !x.Ocupado {
					x.AlAdjuntar()
				}
			})
			s.ConIcono(iconos.Subir, 16)
			s.MargenIcono = HuecoDos
			s.Cargando = x.Ocupado
			der = append(der, s)
		} else if x.PuedeAsentarla() {
			s := boton(x.t("action.post"), ui.Primario, func() {
				if x.AlAsentar != nil && !x.Ocupado && x.PuedeAsentarla() {
					x.AlAsentar()
				}
			})
			s.Cargando = x.Ocupado
			der = append(der, s)
		}
		if x.AlAbrir != nil {
			a := boton(x.t("action.open"), ui.Secundario, x.AlAbrir)
			a.Inerte = x.Ocupado
			der = append(der, a)
		}
		c := boton(x.t("action.close"), ui.Fantasma, alCerrar)
		c.Inerte = x.Ocupado
		izq = append(izq, c)
	}
	return append(izq, der...)
}
