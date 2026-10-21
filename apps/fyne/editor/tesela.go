// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las piezas de la portada de documentos (`DocList.tsx`): la tesela de un documento, sus
// botones de acción, el acceso «Crear nuevo» y el filtro de destacados.
//
// Todas las medidas salen de la HOJA con el raíz de 16 de la suite; donde la web escribe
// px literales (`h-24` no, `180px` sí) se dice.

// Tesela de documento (`DocTile`): `rounded-lg border`, una banda `h-24 border-b
// bg-bg-subtle` con el glifo de 32 en el color del producto y un cuerpo `p-3 gap-0.5`
// con el título en `font-medium` y la fecha en `text-sm`.
var (
	AltoBandaTesela            = tema.Espacio(24) // h-24 → 96, con su border-b DENTRO
	GlifoBandaTesela   float32 = 32
	RellenoTesela              = tema.Espacio(3)   // p-3 → 12
	HuecoTesela                = tema.Espacio(0.5) // gap-0.5 → 2
	LadoBotonTesela            = tema.Espacio(7)   // h-7 w-7 → 28
	GlifoBotonTesela   float32 = 15                // `size = 15` de `Glyph`
	MargenBotonTesela          = tema.Espacio(2)   // top-2 left-2 / right-2 → 8
	HuecoBotonesTesela         = tema.Espacio(1)   // gap-1 → 4
	// MinTesela es el `minmax(180px, 1fr)`: en px de la hoja, así que el rem no lo toca.
	MinTesela float32 = 180
	// HuecoRejillaTesela es el `gap-4` de la rejilla (16) y el `pb-4` de cada fila.
	HuecoRejillaTesela = tema.Espacio(4)
	// HuecoColumnasJS es el `GAP_PX = 16` con el que la web CUENTA las columnas. No es
	// el hueco pintado —ese es el `gap-4` de 16—, y confundirlos da una columna más a
	// ciertos anchos.
	HuecoColumnasJS float32 = 16
)

// MedidaTesela es lo que ocupa una tesela de documento: de ancho, el que le dé la
// rejilla; de alto, borde + banda + cuerpo + borde.
func MedidaTesela() float32 {
	cuerpo := 2*RellenoTesela + tema.TextoRaiz.Linea + HuecoTesela + tema.TextoSM.Linea
	return tema.Borde + AltoBandaTesela + cuerpo + tema.Borde
}

// ColumnasPortada es el `Math.max(1, Math.floor((clientWidth + GAP_PX) / (TILE_MIN_PX +
// GAP_PX)))` de la web, tal cual.
func ColumnasPortada(ancho float32) int {
	return max(1, int((ancho+HuecoColumnasJS)/(MinTesela+HuecoColumnasJS)))
}

// TonoProducto es el `KIND_COLOR`: el color del producto de cada tipo.
func (t Tipo) TonoProducto() string {
	switch t {
	case Hoja:
		return "product-sheets"
	case Presentacion:
		return "product-slides"
	case Documento:
	}
	return "product-docs"
}

// TipoDe es el tipo nativo de un `kind` del servicio. Uno que no se conoce cae a
// documento, que es lo que hace `safeKind` en la web.
func TipoDe(k api.DocKind) Tipo {
	switch k {
	case api.DocSheet:
		return Hoja
	case api.DocSlides:
		return Presentacion
	case api.DocText:
	}
	return Documento
}

// Kind es el `kind` del servicio de este tipo.
func (t Tipo) Kind() api.DocKind {
	switch t {
	case Hoja:
		return api.DocSheet
	case Presentacion:
		return api.DocSlides
	case Documento:
	}
	return api.DocText
}

// conClaves tapa un traductor nil devolviendo la clave, que es lo que quiere un
// guardián y lo que hace [Contexto.Traducir].
func conClaves(t func(string, i18n.Vars) string) func(string, i18n.Vars) string {
	if t != nil {
		return t
	}
	return func(c string, _ i18n.Vars) string { return c }
}

// ---- botón de acción de la tesela ----

// BotonTesela es el `tileActionBtn`: `h-7 w-7 rounded-md border border-border bg-surface
// text-text-muted shadow-xs`, con `hover:bg-bg-subtle hover:text-text`.
type BotonTesela struct {
	activable

	glifo string
	// Pulsado es el `aria-pressed` de la estrella: tinta `primary-text` y relleno.
	Pulsado bool
	// Peligro es el `hover:text-danger-text` de «Eliminar».
	Peligro bool
	// AlEncima avisa a la tesela de que el puntero está sobre el botón: Fyne entrega el
	// paso del puntero a UN objeto, y sin esto la tesela creería que el puntero se ha ido
	// y escondería el botón que se está a punto de pulsar.
	AlEncima func(bool)
}

// NuevoBotonTesela crea el botón con la ruta de su glifo y su rótulo accesible.
func NuevoBotonTesela(glifo, rotulo string, alPulsar func()) *BotonTesela {
	b := &BotonTesela{glifo: glifo}
	b.Rotulo, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MouseIn y MouseOut avisan además a la tesela.
func (b *BotonTesela) MouseIn(e *desktop.MouseEvent) {
	b.activable.MouseIn(e)
	if b.AlEncima != nil {
		b.AlEncima(true)
	}
}

// MouseOut es el reverso de MouseIn.
func (b *BotonTesela) MouseOut() {
	b.activable.MouseOut()
	if b.AlEncima != nil {
		b.AlEncima(false)
	}
}

// MinSize es el `h-7 w-7`.
func (b *BotonTesela) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(LadoBotonTesela, LadoBotonTesela)
}

func (b *BotonTesela) tinta(v tema.Variante) color.NRGBA {
	switch {
	case b.Pulsado:
		return tema.Color("primary-text", v)
	case b.Encima() && b.Peligro:
		return tema.Color("danger-text", v)
	case b.Encima():
		return tema.Color("text", v)
	}
	return tema.Color("text-muted", v)
}

// CreateRenderer compone sombra, aro, caja y el glifo en sus dos versiones (hueco y
// relleno), porque la estrella cambia de FORMA y no solo de color.
func (b *BotonTesela) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonTeselaRenderer{
		b:      b,
		sombra: ui.NuevaSombra(tema.SombraXS, tema.RadioMD),
		aro:    ui.NuevoAro(tema.RadioMD, ui.ConHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD),
		hueco:  iconos.TrazoDeRuta(b.glifo, GlifoBotonTesela, b.tinta(v)),
		lleno:  iconos.TrazoDeRutaRelleno(b.glifo, GlifoBotonTesela, b.tinta(v)),
	}
	r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.aro.Antes(r.caja.Objetos()...)...)
	r.objetos = append(r.objetos, r.hueco.Objeto(), r.lleno.Objeto())
	r.Refresh()
	return r
}

type botonTeselaRenderer struct {
	b       *BotonTesela
	sombra  *ui.Sombra
	aro     *ui.Aro
	caja    *ui.Caja
	hueco   *iconos.Dibujo
	lleno   *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *botonTeselaRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonTeselaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonTeselaRenderer) Destroy()                     {}

func (r *botonTeselaRenderer) Layout(t fyne.Size) {
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x, y := (t.Width-GlifoBotonTesela)/2, (t.Height-GlifoBotonTesela)/2
	r.hueco.Colocar(x, y, GlifoBotonTesela)
	r.lleno.Colocar(x, y, GlifoBotonTesela)
}

func (r *botonTeselaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := tema.Color("surface", v)
	if r.b.Encima() {
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, tema.Color("border", v))
	tinta := r.b.tinta(v)
	r.hueco.TenirCon(tinta)
	r.lleno.TenirCon(tinta)
	r.hueco.Ver(!r.b.Pulsado)
	r.lleno.Ver(r.b.Pulsado)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.Layout(r.b.Size())
}

// ---- tesela de documento ----

// TeselaDoc es la tarjeta de un documento de la portada: se pulsa para abrirlo, y
// lleva la estrella arriba a la izquierda y renombrar/eliminar arriba a la derecha.
//
// Las acciones se esconden hasta que el puntero pasa por encima o el foco entra
// (`opacity-0 group-hover:opacity-100 focus-within:opacity-100`); la estrella de un
// DESTACADO se ve siempre, que es la razón de que vaya aparte. En un teléfono
// (`pointer-coarse:opacity-100`) se ven todas siempre.
type TeselaDoc struct {
	activable

	Doc    api.Document
	fecha  string
	rotTip string
	tono   string
	glifo  string

	estrella *BotonTesela
	acciones []*BotonTesela
	// sobreHijo cuenta si el puntero está sobre uno de sus botones.
	sobreHijo bool
	// SinRed esconde las acciones: sin conexión no se muta nada (`!offline`).
	SinRed bool
}

// Acciones de una tesela. Van juntas porque la portada las pasa todas a la vez.
type AccionesTesela struct {
	Abrir, Destacar, Renombrar, Eliminar func(api.Document)
}

// NuevaTeselaDoc monta la tesela. `t` traduce y `loc` es el idioma de la fecha.
func NuevaTeselaDoc(d api.Document, t func(string, i18n.Vars) string, loc i18n.Locale, a AccionesTesela) *TeselaDoc {
	t = conClaves(t)
	tipo := TipoDe(d.Kind)
	s := &TeselaDoc{Doc: d, rotTip: t("kind."+string(tipo.Kind()), nil), tono: tipo.TonoProducto(), glifo: tipo.Glifo()}
	if !d.UpdatedAt.IsZero() {
		s.fecha = i18n.FormatDateShortMonth(loc, d.UpdatedAt.In(time.Local))
	}
	s.Rotulo = d.Title
	llamar := func(f func(api.Document)) func() {
		return func() {
			if f != nil {
				f(s.Doc)
			}
		}
	}
	s.AlPulsar = llamar(a.Abrir)
	clave := "tile.star"
	if d.Starred {
		clave = "tile.unstar"
	}
	s.estrella = NuevoBotonTesela(GlifoEstrella, t(clave+".label", i18n.Vars{"title": d.Title}), llamar(a.Destacar))
	s.estrella.Pulsado = d.Starred
	ren := NuevoBotonTesela(GlifoLapiz, t("tile.rename.label", i18n.Vars{"title": d.Title}), llamar(a.Renombrar))
	bor := NuevoBotonTesela(GlifoPapelera, t("tile.delete.label", i18n.Vars{"title": d.Title}), llamar(a.Eliminar))
	bor.Peligro = true
	// ponytail: faltan «Descargar» y «Compartir» entre los dos: el SDK no tiene aún las
	// exportaciones ni los permisos del servicio (`/exports`, `/permissions`). Van en el
	// mismo sitio, en el orden de la web: renombrar · descargar · compartir · eliminar.
	s.acciones = []*BotonTesela{ren, bor}
	for _, b := range append([]*BotonTesela{s.estrella}, s.acciones...) {
		b.AlEncima = func(si bool) { s.sobreHijo = si; s.Refresh() }
	}
	s.ExtendBaseWidget(s)
	return s
}

// AccessibilityRole es el `<a href>` de la web: un enlace al editor.
func (s *TeselaDoc) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleLink }

// MinSize es el mínimo de la rejilla con el alto de la tesela.
func (s *TeselaDoc) MinSize() fyne.Size {
	s.ExtendBaseWidget(s)
	return fyne.NewSize(MinTesela, MedidaTesela())
}

// accionesVisibles es `group-hover` o `focus-within`, o un teléfono.
func (s *TeselaDoc) accionesVisibles() bool {
	if s.SinRed {
		return false
	}
	if ui.EsTactil() {
		return true
	}
	return s.Encima() || s.sobreHijo || s.Enfocado()
}

// CreateRenderer compone aro, caja, banda, glifo, textos y botones.
func (s *TeselaDoc) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &teselaRenderer{
		s:      s,
		aro:    ui.NuevoAro(tema.RadioLG, ui.ConHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		bandaR: canvas.NewRectangle(color.NRGBA{}),
		bandaC: canvas.NewRectangle(color.NRGBA{}),
		linea:  canvas.NewRectangle(color.NRGBA{}),
		glifo:  iconos.TrazoDeRuta(s.glifo, GlifoBandaTesela, tema.Color(s.tono, v)),
		titulo: ui.Texto(s.Doc.Title, tema.TextoRaiz, ui.Medio, color.NRGBA{}),
		meta:   ui.Texto("", tema.TextoSM, ui.Normal, color.NRGBA{}),
	}
	// `overflow-hidden rounded-lg`: la banda lleva arriba el radio de la caja MENOS su
	// borde. Un rectángulo de Fyne redondea las cuatro esquinas, así que abajo se le
	// pone otro recto encima.
	r.bandaR.CornerRadius = tema.RadioLG - tema.Borde
	r.objetos = r.aro.Antes(r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.bandaR, r.bandaC, r.linea, r.glifo.Objeto(), r.titulo, r.meta, s.estrella)
	for _, b := range s.acciones {
		r.objetos = append(r.objetos, b)
	}
	r.Refresh()
	return r
}

type teselaRenderer struct {
	s                     *TeselaDoc
	aro                   *ui.Aro
	caja                  *ui.Caja
	bandaR, bandaC, linea *canvas.Rectangle
	glifo                 *iconos.Dibujo
	titulo, meta          *canvas.Text
	objetos               []fyne.CanvasObject
}

func (r *teselaRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *teselaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *teselaRenderer) Destroy()                     {}

func (r *teselaRenderer) Layout(t fyne.Size) {
	s := r.s
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	dentro := t.Width - 2*tema.Borde
	relleno := AltoBandaTesela - tema.Borde // la banda sin su border-b
	r.bandaR.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.bandaR.Resize(fyne.NewSize(dentro, relleno))
	r.bandaC.Move(fyne.NewPos(tema.Borde, tema.Borde+r.bandaR.CornerRadius))
	r.bandaC.Resize(fyne.NewSize(dentro, relleno-r.bandaR.CornerRadius))
	r.linea.Move(fyne.NewPos(tema.Borde, tema.Borde+relleno))
	r.linea.Resize(fyne.NewSize(dentro, tema.Borde))
	r.glifo.Colocar((t.Width-GlifoBandaTesela)/2, tema.Borde+(relleno-GlifoBandaTesela)/2, GlifoBandaTesela)

	x := tema.Borde + RellenoTesela
	cabe := max(dentro-2*RellenoTesela, 0)
	y := tema.Borde + AltoBandaTesela + RellenoTesela
	ui.Truncar(r.titulo, s.Doc.Title, tema.TextoRaiz, ui.Medio, cabe)
	ui.ColocarTexto(r.titulo, tema.TextoRaiz, x, y)
	meta := s.rotTip
	if s.fecha != "" {
		meta += " · " + s.fecha
	}
	ui.Truncar(r.meta, meta, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.meta, tema.TextoSM, x, y+tema.TextoRaiz.Linea+HuecoTesela)

	lado := fyne.NewSize(LadoBotonTesela, LadoBotonTesela)
	s.estrella.Move(fyne.NewPos(MargenBotonTesela, MargenBotonTesela))
	s.estrella.Resize(lado)
	der := t.Width - MargenBotonTesela
	for i := len(s.acciones) - 1; i >= 0; i-- {
		der -= LadoBotonTesela
		s.acciones[i].Move(fyne.NewPos(der, MargenBotonTesela))
		s.acciones[i].Resize(lado)
		der -= HuecoBotonesTesela
	}
}

func (r *teselaRenderer) Refresh() {
	s := r.s
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde := tema.Color("surface", v), tema.Color("border", v)
	if s.Encima() || s.sobreHijo {
		fondo, borde = tema.Color("bg-subtle", v), tema.Color("border-strong", v)
	}
	r.caja.Pintar(fondo, borde)
	sutil := tema.Color("bg-subtle", v)
	r.bandaR.FillColor, r.bandaC.FillColor = sutil, sutil
	r.linea.FillColor = tema.Color("border", v)
	r.glifo.Tenir(s.tono, v)
	r.titulo.Color = tema.Color("text", v)
	r.meta.Color = tema.Color("text-muted", v)
	for _, o := range []fyne.CanvasObject{r.bandaR, r.bandaC, r.linea, r.titulo, r.meta} {
		o.Refresh()
	}
	visibles := s.accionesVisibles()
	switch {
	case s.SinRed:
		s.estrella.Hide()
	case s.Doc.Starred || visibles:
		s.estrella.Show()
	default:
		s.estrella.Hide()
	}
	for _, b := range s.acciones {
		if visibles {
			b.Show()
		} else {
			b.Hide()
		}
	}
	r.aro.Pintar(v)
	r.aro.Ver(s.Enfocado())
	r.Layout(s.Size())
}

// ---- acceso «Crear nuevo» ----

// AccesoNuevo es el botón de «Crear nuevo» de un tipo: `flex items-center gap-3
// rounded-lg border p-4`, con la tesela de color `h-11 w-11 rounded-lg` y su glifo de
// 22 en blanco, y a su derecha el tipo en `font-semibold` y «En blanco» en `text-sm`.
type AccesoNuevo struct {
	activable

	tipo   Tipo
	titulo string
	blanco string
	creado string
	// Creando es `aria-busy`: la rueda de 18 releva al glifo y el subtítulo dice
	// «Creando…».
	Creando bool
}

// Medidas del acceso.
var (
	RellenoAcceso            = tema.Espacio(4)  // p-4 → 16
	HuecoAcceso              = tema.Espacio(3)  // gap-3 → 12
	LadoTeselaAcceso         = tema.Espacio(11) // h-11 w-11 → 38,5
	GlifoAcceso      float32 = 22
	RuedaAcceso      float32 = 18
)

// NuevoAccesoNuevo crea el acceso de ese tipo.
func NuevoAccesoNuevo(tipo Tipo, t func(string, i18n.Vars) string, alPulsar func()) *AccesoNuevo {
	t = conClaves(t)
	a := &AccesoNuevo{
		tipo: tipo, titulo: t("kind."+string(tipo.Kind()), nil),
		blanco: t("list.new.blank", nil), creado: t("list.new.creating", nil),
	}
	a.Rotulo, a.AlPulsar = a.titulo, alPulsar
	a.ExtendBaseWidget(a)
	return a
}

// MedidaAccesoNuevo es su alto: el `p-4` y lo mayor entre la tesela de color y las dos
// líneas de texto, con los dos bordes.
func MedidaAccesoNuevo() float32 {
	texto := tema.TextoRaiz.Linea + tema.TextoSM.Linea
	return 2*tema.Borde + 2*RellenoAcceso + max(LadoTeselaAcceso, texto)
}

// MinSize es el alto medido y un ancho mínimo que no recorta la tesela de color.
func (a *AccesoNuevo) MinSize() fyne.Size {
	a.ExtendBaseWidget(a)
	return fyne.NewSize(2*tema.Borde+2*RellenoAcceso+LadoTeselaAcceso, MedidaAccesoNuevo())
}

// opacidad es el `opacity-60` de un acceso deshabilitado que no está creando.
func (a *AccesoNuevo) opacidad() float32 {
	if a.Inerte && !a.Creando {
		return 0.6
	}
	return 1
}

// CreateRenderer compone aro, caja, tesela de color, glifo, rueda y textos.
func (a *AccesoNuevo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	blanco := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	r := &accesoRenderer{
		a:      a,
		aro:    ui.NuevoAro(tema.RadioLG, ui.ConHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		marca:  canvas.NewRectangle(color.NRGBA{}),
		glifo:  iconos.TrazoDeRuta(a.tipo.Glifo(), GlifoAcceso, blanco),
		rueda:  ui.NuevaRuedaCon(RuedaAcceso, blanco),
		titulo: ui.Texto(a.titulo, tema.TextoRaiz, ui.SemiNegrita, color.NRGBA{}),
		sub:    ui.Texto(a.blanco, tema.TextoSM, ui.Normal, color.NRGBA{}),
	}
	r.marca.CornerRadius = tema.RadioLG
	r.objetos = r.aro.Antes(r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.marca, r.glifo.Objeto(), r.rueda, r.titulo, r.sub)
	r.Refresh()
	return r
}

type accesoRenderer struct {
	a           *AccesoNuevo
	aro         *ui.Aro
	caja        *ui.Caja
	marca       *canvas.Rectangle
	glifo       *iconos.Dibujo
	rueda       *ui.Rueda
	titulo, sub *canvas.Text
	objetos     []fyne.CanvasObject
}

func (r *accesoRenderer) MinSize() fyne.Size           { return r.a.MinSize() }
func (r *accesoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *accesoRenderer) Destroy()                     {}

func (r *accesoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x := tema.Borde + RellenoAcceso
	y := (t.Height - LadoTeselaAcceso) / 2
	r.marca.Move(fyne.NewPos(x, y))
	r.marca.Resize(fyne.NewSize(LadoTeselaAcceso, LadoTeselaAcceso))
	r.glifo.Colocar(x+(LadoTeselaAcceso-GlifoAcceso)/2, y+(LadoTeselaAcceso-GlifoAcceso)/2, GlifoAcceso)
	r.rueda.Move(fyne.NewPos(x+(LadoTeselaAcceso-RuedaAcceso)/2, y+(LadoTeselaAcceso-RuedaAcceso)/2))
	r.rueda.Resize(ui.MedidaRueda(RuedaAcceso))

	x += LadoTeselaAcceso + HuecoAcceso
	cabe := max(t.Width-x-tema.Borde-RellenoAcceso, 0)
	arriba := (t.Height - tema.TextoRaiz.Linea - tema.TextoSM.Linea) / 2
	ui.Truncar(r.titulo, r.a.titulo, tema.TextoRaiz, ui.SemiNegrita, cabe)
	ui.ColocarTexto(r.titulo, tema.TextoRaiz, x, arriba)
	sub := r.a.blanco
	if r.a.Creando {
		sub = r.a.creado
	}
	ui.Truncar(r.sub, sub, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.sub, tema.TextoSM, x, arriba+tema.TextoRaiz.Linea)
}

// atenuarSobre mezcla un color con la superficie, que es lo que hace `opacity-60` sobre
// el grupo entero (Fyne no tiene opacidad de grupo: `docs/roadmap.md`, techos).
func atenuarSobre(c, fondo color.NRGBA, o float32) color.NRGBA {
	if o >= 1 {
		return c
	}
	mezcla := func(a, b uint8) uint8 { return uint8(float32(a)*o + float32(b)*(1-o) + 0.5) }
	a := float32(c.A) / 255
	return color.NRGBA{
		R: mezcla(uint8(float32(c.R)*a+float32(fondo.R)*(1-a)), fondo.R),
		G: mezcla(uint8(float32(c.G)*a+float32(fondo.G)*(1-a)), fondo.G),
		B: mezcla(uint8(float32(c.B)*a+float32(fondo.B)*(1-a)), fondo.B),
		A: 0xFF,
	}
}

func (r *accesoRenderer) Refresh() {
	a := r.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	superficie := tema.Color("surface", v)
	fondo, borde := superficie, tema.Color("border", v)
	if a.Encima() && !a.Inerte {
		fondo, borde = tema.Color("bg-subtle", v), tema.Color("border-strong", v)
	}
	o := a.opacidad()
	r.caja.Pintar(atenuarSobre(fondo, superficie, o), atenuarSobre(borde, superficie, o))
	r.marca.FillColor = atenuarSobre(tema.Color(a.tipo.TonoProducto(), v), superficie, o)
	r.titulo.Color = atenuarSobre(tema.Color("text", v), superficie, o)
	r.sub.Color = atenuarSobre(tema.Color("text-muted", v), superficie, o)
	r.glifo.Ver(!a.Creando)
	if a.Creando {
		r.rueda.Show()
	} else {
		r.rueda.Hide()
	}
	for _, o := range []fyne.CanvasObject{r.marca, r.titulo, r.sub} {
		o.Refresh()
	}
	r.aro.Pintar(v)
	r.aro.Ver(a.Enfocado())
	r.Layout(a.Size())
}

// ---- filtro «Destacados» ----

// FiltroDestacados es el conmutador de la cabecera de «Recientes»: `h-8 rounded-md
// border px-2.5 gap-1.5 text-sm font-medium`, con la estrella de 14 que se rellena al
// encenderse (`aria-pressed`).
type FiltroDestacados struct {
	activable
	Activo bool
	// Glifo es el trazado del icono; vacío, la estrella. La papelera de la portada es
	// el MISMO botón con la papelera dentro (en la web, las mismas clases) y nunca se
	// activa: lleva a otra pantalla.
	Glifo string
}

// Medidas del filtro.
var (
	AltoFiltro           = tema.Espacio(8)   // h-8 → 28
	MargenFiltro         = tema.Espacio(2.5) // px-2.5 → 10
	HuecoFiltro          = tema.Espacio(1.5) // gap-1.5 → 6
	GlifoFiltro  float32 = 14
)

// NuevoFiltroDestacados crea el filtro con su rótulo YA traducido.
func NuevoFiltroDestacados(rotulo string, alPulsar func()) *FiltroDestacados {
	f := &FiltroDestacados{}
	f.Rotulo, f.AlPulsar = rotulo, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MedidaFiltro es lo que ocupa: el glifo, el hueco y el rótulo con sus márgenes y bordes.
func MedidaFiltro(rotulo string) fyne.Size {
	return fyne.NewSize(
		2*tema.Borde+2*MargenFiltro+GlifoFiltro+HuecoFiltro+ui.Medir(rotulo, tema.TextoSM, ui.Medio).Width,
		AltoFiltro)
}

func (f *FiltroDestacados) glifo() string {
	if f.Glifo != "" {
		return f.Glifo
	}
	return GlifoEstrella
}

// NuevoAccesoPapelera es el botón «Papelera» de la portada: el del filtro, con la
// papelera, que abre la pantalla de la papelera.
func NuevoAccesoPapelera(rotulo string, alPulsar func()) *FiltroDestacados {
	f := NuevoFiltroDestacados(rotulo, alPulsar)
	f.Glifo = GlifoPapelera
	return f
}

// MinSize es MedidaFiltro.
func (f *FiltroDestacados) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return MedidaFiltro(f.Rotulo)
}

func (f *FiltroDestacados) colores(v tema.Variante) (fondo, borde, tinta color.NRGBA) {
	switch {
	case f.Activo:
		return tema.Color("selected", v), tema.Color("border-strong", v), tema.Color("text", v)
	case f.Encima():
		return tema.Color("bg-subtle", v), tema.Color("border", v), tema.Color("text", v)
	}
	return tema.Color("surface", v), tema.Color("border", v), tema.Color("text-muted", v)
}

// CreateRenderer compone aro, caja, estrella (hueca y rellena) y rótulo.
func (f *FiltroDestacados) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	_, _, tinta := f.colores(v)
	r := &filtroRenderer{
		f:      f,
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD),
		hueco:  iconos.TrazoDeRuta(f.glifo(), GlifoFiltro, tinta),
		lleno:  iconos.TrazoDeRutaRelleno(f.glifo(), GlifoFiltro, tinta),
		rotulo: ui.Texto(f.Rotulo, tema.TextoSM, ui.Medio, tinta),
	}
	r.objetos = r.aro.Antes(r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.hueco.Objeto(), r.lleno.Objeto(), r.rotulo)
	r.Refresh()
	return r
}

type filtroRenderer struct {
	f            *FiltroDestacados
	aro          *ui.Aro
	caja         *ui.Caja
	hueco, lleno *iconos.Dibujo
	rotulo       *canvas.Text
	objetos      []fyne.CanvasObject
}

func (r *filtroRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filtroRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filtroRenderer) Destroy()                     {}

func (r *filtroRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x := tema.Borde + MargenFiltro
	y := (t.Height - GlifoFiltro) / 2
	r.hueco.Colocar(x, y, GlifoFiltro)
	r.lleno.Colocar(x, y, GlifoFiltro)
	ui.ColocarTexto(r.rotulo, tema.TextoSM, x+GlifoFiltro+HuecoFiltro, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *filtroRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := r.f.colores(v)
	r.caja.Pintar(fondo, borde)
	r.hueco.TenirCon(tinta)
	r.lleno.TenirCon(tinta)
	r.hueco.Ver(!r.f.Activo)
	r.lleno.Ver(r.f.Activo)
	r.rotulo.Text, r.rotulo.Color = r.f.Rotulo, tinta
	r.rotulo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.Layout(r.f.Size())
}
