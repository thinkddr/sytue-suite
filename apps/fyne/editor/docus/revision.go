// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El CONTROL DE CAMBIOS en el folio: teclear y borrar con el modo encendido (lo que en la
// web intercepta `beforeinput` en `PageCanvas.tsx`) y el panel «Revisión»
// (`panels/RevisionPanel.tsx`). Las marcas y su HTML son de `rico` (cambios.go).
//
// Intro, el formato y pegar imágenes no se rastrean, como en la web: se aplican tal cual.

// sello es el `Date.now()` de la marca.
func sello() int64 { return time.Now().UnixMilli() }

// rastreando dice si el documento está en control de cambios.
func (f *Folio) rastreando() bool { return f.doc != nil && f.doc.Rastreo() }

// autor es el de las marcas: el de la sesión, o la reserva de la web si no se sabe.
func (f *Folio) autor() rico.Autor {
	if f.Autor.ID == "" {
		return rico.AutorReserva
	}
	return f.Autor
}

// borrarRastreado marca como borrado [desde, hasta) del bloque del cursor, o lo salta si
// ya lo estaba: Retroceso y Suprimir con el modo encendido.
func (f *Folio) borrarRastreado(desde, hasta int, atras bool) {
	foto := paso{f.doc.HTML(), f.cur, f.anc}
	p, cambio := f.edicion(f.cur.b).BorrarRastreado(desde, hasta, f.autor(), sello(), atras)
	if !cambio {
		f.mover(punto{f.cur.b, p}, false)
		return
	}
	f.apuntar(foto, true)
	f.cur.p = p
	f.anc = f.cur
	f.cambiado()
}

// borrarSeleccionRastreada marca la selección como borrada, bloque a bloque y sin
// fundirlos, y deja el cursor delante (`atras`) o detrás de lo marcado.
func (f *Folio) borrarSeleccionRastreada(atras bool) {
	a, b := f.orden()
	autor, ts := f.autor(), sello()
	var donde rico.Lugar
	en := 0
	// Del último al primero: marcar un bloque no mueve los nodos de los de delante.
	for i := b.b; i >= a.b; i-- {
		if !f.deTexto(i) {
			continue
		}
		desde, hasta := 0, f.largo(i)
		if i == a.b {
			desde = a.p
		}
		if i == b.b {
			hasta = b.p
		}
		e := f.bloques[i].Editar()
		p, _ := e.BorrarRastreado(desde, hasta, autor, ts, atras)
		if (atras && i == a.b) || (!atras && i == b.b) || donde == (rico.Lugar{}) {
			donde, en = e.Lugar, p
		}
	}
	f.ed = nil
	f.irA(donde, en)
}

// EnRastreo dice si el control de cambios está encendido.
func (f *Folio) EnRastreo() bool { return f.rastreando() }

// AlternarRastreo enciende o apaga el control de cambios. Es un cambio del documento —la
// sentinela se guarda, y así reabrirlo lo restaura— aunque no entra en el deshacer, como
// en la web (`conmutarControlCambios`).
func (f *Folio) AlternarRastreo() {
	if !f.editable() {
		return
	}
	f.doc.PonerRastreo(!f.doc.Rastreo())
	f.cambios++
	if f.AlEditar != nil {
		f.AlEditar()
	}
}

// CambiosRastreados son los cambios del control de cambios, en orden de documento.
func (f *Folio) CambiosRastreados() []rico.Cambio { return f.doc.Cambios() }

// ResolverCambios acepta o rechaza todos, en UN paso de deshacer. Dice si había alguno.
func (f *Folio) ResolverCambios(aceptar bool) bool {
	return f.resolver(func() bool { return f.doc.Resolver(aceptar) })
}

// ResolverCambio acepta o rechaza el de ese ordinal.
func (f *Folio) ResolverCambio(i int, aceptar bool) bool {
	return f.resolver(func() bool { return f.doc.ResolverCambio(i, aceptar) })
}

func (f *Folio) resolver(hacer func() bool) bool {
	if !f.editable() {
		return false
	}
	foto := paso{f.doc.HTML(), f.cur, f.anc}
	if !hacer() {
		return false
	}
	f.apuntar(foto, false)
	f.revisado = nil
	f.reestructurado()
	return true
}

// ResaltarCambio pinta el cambio i con el violeta del panel (`#e9d5ff`, el
// `::highlight(sytue-track-review)` de la web) y lo lleva al centro de la vista; -1 lo
// quita.
//
// ponytail: la web pinta además su texto en `#0a0a0a`; aquí conserva el color del autor.
func (f *Folio) ResaltarCambio(i int) {
	f.revisado = nil
	if cs := f.doc.Cambios(); i >= 0 && i < len(cs) {
		for b, bl := range f.bloques {
			if desde, hasta, ok := bl.Donde(cs[i]); ok {
				f.revisado = append(f.revisado, coincidencia{b, desde, hasta})
			}
		}
		if len(f.revisado) > 0 {
			f.llevarAVista(punto{f.revisado[0].b, f.revisado[0].desde})
		}
	}
	f.pag.Refresh()
}

// ColorRevisado es el del cambio en foco (`#e9d5ff` de HIGHLIGHT_PALETTE).
var ColorRevisado = color.NRGBA{R: 0xe9, G: 0xd5, B: 0xff, A: 0xff}

// PausaRevision es el rebote con el que la lista sigue al teclear (150 ms en la web).
const PausaRevision = 150 * time.Millisecond

// ---- el panel ----

// revisor es el panel «Revisión» de un folio: `flex min-h-full flex-col gap-2 p-3`, con
// la navegación arriba, la lista de cambios y «Aceptar/Rechazar todos» al pie.
type revisor struct {
	f      *Folio
	t      func(string, i18n.Vars) string
	loc    i18n.Locale
	activa int // el cambio en foco; -1 ninguno
	tanda  int

	anterior, siguiente *ui.BotonIcono
	contador            *ui.Rotulo
	lista               *fyne.Container
	aceptar, rechazar   *ui.Boton
	objeto              fyne.CanvasObject
}

// PanelRevision es el cuerpo de «Revisión». Se construye UNA vez por folio, y la lista se
// rehace al cambiar el documento.
func (f *Folio) PanelRevision(t func(string, i18n.Vars) string, loc i18n.Locale) fyne.CanvasObject {
	if f.revisor == nil {
		f.revisor = nuevoRevisor(f, t, loc)
	}
	f.revisor.pintar()
	return f.revisor.objeto
}

func nuevoRevisor(f *Folio, t func(string, i18n.Vars) string, loc i18n.Locale) *revisor {
	r := &revisor{f: f, t: t, loc: loc, activa: -1}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	flecha := func(i iconos.Ed, clave string, d int) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(iconos.Ninguno, r.tr(clave), func() { r.ir(d) })
		b.Glifo, b.TamIcono = iconos.TrazoEd(i, 16, "text-muted", v), 16
		return b
	}
	r.anterior = flecha(iconos.EdChevronIzq, "revision.previous", -1)
	r.siguiente = flecha(iconos.EdChevronDer, "revision.next", 1)
	r.contador = ui.NuevoRotulo("", tema.TextoXS, ui.Normal, "text-muted")
	nav := container.New(filaCentro{hueco: tema.Espacio(1)}, r.anterior, r.siguiente,
		container.New(layout.NewCustomPaddedLayout(0, 0, tema.Espacio(1), 0), r.contador))
	r.lista = container.New(ui.Columna{Hueco: tema.Espacio(2)})
	todos := func(aceptar bool) func() {
		verbo, pregunta := "review.rejectAll", "review.rejectAll.confirm"
		if aceptar {
			verbo, pregunta = "review.acceptAll", "review.acceptAll.confirm"
		}
		return func() {
			if f.ctx == nil {
				return
			}
			f.ctx.Confirmar(r.objeto, verbo, pregunta, !aceptar, func() {
				f.ResolverCambios(aceptar)
				r.activa = -1
				f.ResaltarCambio(-1)
				r.pintar()
			})
		}
	}
	r.aceptar = ui.NuevoBoton(r.tr("revision.acceptAll"), todos(true)).Con(ui.Fantasma, ui.TallaSM)
	r.rechazar = ui.NuevoBoton(r.tr("revision.rejectAll"), todos(false)).Con(ui.Fantasma, ui.TallaSM)
	// `border-border mt-auto flex gap-1.5 border-t pt-2`.
	raya := canvas.NewRectangle(tema.Color("border", v))
	raya.SetMinSize(fyne.NewSize(0, tema.Borde))
	pie := container.New(ui.Columna{}, raya, container.New(layout.NewCustomPaddedLayout(tema.Espacio(2), 0, 0, 0),
		container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, r.aceptar, r.rechazar)))
	medio := container.New(layout.NewCustomPaddedLayout(tema.Espacio(2), tema.Espacio(2), 0, 0), container.NewVScroll(r.lista))
	r.objeto = container.New(layout.NewCustomPaddedLayout(MargenCuerpo, MargenCuerpo, MargenCuerpo, MargenCuerpo),
		container.NewBorder(nav, pie, nil, nil, medio))
	return r
}

func (r *revisor) tr(k string) string { return r.trv(k, nil) }

func (r *revisor) trv(k string, v i18n.Vars) string {
	if r.t == nil {
		return k
	}
	return r.t(k, v)
}

// ir es anterior/siguiente en ciclo; sin foco, arranca por los extremos.
func (r *revisor) ir(d int) {
	total := len(r.f.CambiosRastreados())
	if total == 0 {
		return
	}
	desde := r.activa
	if desde < 0 || desde >= total {
		desde = map[bool]int{true: -1, false: 0}[d > 0]
	}
	r.enfocar((desde + d + total) % total)
}

// enfocar resalta el cambio i en el folio y lo marca en la lista.
func (r *revisor) enfocar(i int) {
	r.activa = i
	r.f.ResaltarCambio(i)
	r.pintar()
}

// resolverUno acepta o rechaza uno y pasa al que ocupa ahora su sitio, o a ninguno.
func (r *revisor) resolverUno(i int, aceptar bool) {
	r.f.ResolverCambio(i, aceptar)
	if n := len(r.f.CambiosRastreados()); n > 0 {
		r.enfocar(min(i, n-1))
		return
	}
	r.activa = -1
	r.f.ResaltarCambio(-1)
	r.pintar()
}

// programar rehace la lista [PausaRevision] después del último cambio del documento.
func (r *revisor) programar() {
	r.tanda++
	tanda := r.tanda
	r.f.Programar(PausaRevision, func() {
		if tanda == r.tanda {
			r.pintar()
		}
	})
}

// pintar rehace el contador y la lista con los cambios de ahora.
func (r *revisor) pintar() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cs := r.f.CambiosRastreados()
	total := len(cs)
	if r.activa >= total {
		r.activa = -1
	}
	switch {
	case total == 0:
		r.contador.Texto = r.tr("revision.none")
	case r.activa < 0:
		r.contador.Texto = r.trv("revision.position", i18n.Vars{"index": r.tr("revision.emptyPosition"), "total": total})
	default:
		r.contador.Texto = r.trv("revision.position", i18n.Vars{"index": r.activa + 1, "total": total})
	}
	r.contador.Refresh()
	for _, b := range []*ui.BotonIcono{r.anterior, r.siguiente} {
		b.Inerte = total == 0
		b.Refresh()
	}
	inerte := total == 0 || !r.f.editable()
	for _, b := range []*ui.Boton{r.aceptar, r.rechazar} {
		b.Inerte = inerte
		b.Refresh()
	}
	r.lista.Objects = nil
	if total == 0 {
		r.lista.Add(ui.NuevoParrafo(r.tr("revision.empty"), tema.TextoSM, ui.Normal, "text-muted", v).A(anchoCuerpoPanel))
	}
	for i, c := range cs {
		r.lista.Add(r.tarjeta(i, c, v))
	}
	r.lista.Refresh()
}

// tarjeta es el `<article class="rounded-md border p-2">` de un cambio: su cabecera y el
// extracto, que llevan al cambio, y «Aceptar» y «Rechazar».
func (r *revisor) tarjeta(i int, c rico.Cambio, v tema.Variante) fyne.CanvasObject {
	ancho := anchoCuerpoPanel - 2*tema.Borde - 2*tema.Espacio(2)
	icono, clave := iconos.EdMas, "revision.insertion"
	if !c.Insercion {
		icono, clave = iconos.EdMenos, "revision.deletion"
	}
	var cuando string
	if c.TS > 0 {
		cuando = i18n.FormatTimeAgo(r.loc, time.UnixMilli(c.TS), time.Now())
	}
	cab := &cabeceraCambio{
		glifo: iconos.TrazoEd(icono, 14, "text", v),
		tipo:  ui.Texto(r.tr(clave), tema.TextoXS, ui.SemiNegrita, tema.Color("text", v)),
		autor: ui.Texto("", tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)), textoAutor: "· " + c.Autor,
		cuando: ui.Texto(cuando, tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)),
	}
	extracto := ui.NuevoParrafo("«"+c.Texto+"»", tema.TextoXS, ui.Normal, "text-muted", v).A(ancho)
	var cuerpo fyne.CanvasObject = extracto
	if !c.Insercion {
		cuerpo = container.New(&tachado{p: extracto}, extracto)
	}
	ir := nuevaZonaPulsable(r.tr("revision.goTo"), func() { r.enfocar(i) },
		container.New(ui.Columna{}, container.New(cab, cab.objetos()...), conArriba(tema.Espacio(1), cuerpo)))
	accion := func(e iconos.Ed, clave string, aceptar bool) *ui.Boton {
		b := ui.NuevoBoton(r.tr(clave), func() { r.resolverUno(i, aceptar) }).Con(ui.Fantasma, ui.TallaXS)
		b.Glifo, b.TamIcono, b.Rotulo = iconos.TrazoEd(e, 14, "text-muted", v), 14, r.tr(clave+".title")
		b.Atenuado = true // `ACTION_BTN`
		b.Inerte = !r.f.editable()
		return b
	}
	acciones := container.New(ui.Fila{Hueco: tema.Espacio(1)},
		accion(iconos.EdTic, "revision.accept", true), accion(iconos.EdCerrar, "revision.reject", false))
	contenido := container.New(ui.Columna{}, ir, conArriba(tema.Espacio(1.5), acciones))
	// Sin fondo propio se ve el del panel; `bg-selected` es translúcido y va compuesto
	// sobre él, que la caja pinta el relleno encima del borde (ver `ui.Caja`).
	fondo, borde := tema.Color("surface", v), tema.Color("border", v)
	if i == r.activa {
		fondo, borde = tokens.SobreSRGB(tema.Color("selected", v), fondo), tema.Color("primary", v)
	}
	caja := ui.NuevaCaja(fondo, borde, tema.RadioMD)
	return container.New(&tarjetaComentario{caja: caja}, append(caja.Objetos(), contenido)...)
}

// cabeceraCambio es `flex items-baseline gap-1.5`: el glifo, el tipo en `text-xs
// font-semibold`, «· autor» en `sytue-meta-dense truncate` y la antigüedad al final
// (`ml-auto shrink-0 pl-1.5`). Un glifo no tiene línea base: la suya es su borde de abajo,
// como la de cualquier caja sin texto en un flex alineado por la base.
type cabeceraCambio struct {
	glifo       *iconos.Dibujo
	tipo, autor *canvas.Text
	cuando      *canvas.Text
	textoAutor  string
	base, alto  float32
}

func (c *cabeceraCambio) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{c.glifo.Objeto(), c.tipo, c.autor, c.cuando}
}

func (c *cabeceraCambio) medir() {
	c.base = max(14, lineaBase(tema.TextoXS), lineaBase(tema.Texto2XS))
	c.alto = c.base + max(0, tema.TextoXS.Linea-lineaBase(tema.TextoXS), tema.Texto2XS.Linea-lineaBase(tema.Texto2XS))
}

func (c *cabeceraCambio) MinSize([]fyne.CanvasObject) fyne.Size {
	c.medir()
	return fyne.NewSize(0, c.alto)
}

func (c *cabeceraCambio) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.medir()
	hueco := tema.Espacio(1.5)
	c.glifo.Colocar(0, c.base-14, 14)
	x := 14 + hueco
	ui.ColocarTexto(c.tipo, tema.TextoXS, x, c.base-lineaBase(tema.TextoXS))
	x += ui.Medir(c.tipo.Text, tema.TextoXS, ui.SemiNegrita).Width + hueco
	fin := t.Width
	if c.cuando.Text != "" {
		w := ui.Medir(c.cuando.Text, tema.Texto2XS, ui.Normal).Width
		fin -= w
		ui.ColocarTexto(c.cuando, tema.Texto2XS, fin, c.base-lineaBase(tema.Texto2XS))
		fin -= tema.Espacio(1.5) + hueco
	}
	ui.Truncar(c.autor, c.textoAutor, tema.Texto2XS, ui.Normal, max(fin-x, 0))
	ui.ColocarTexto(c.autor, tema.Texto2XS, x, c.base-lineaBase(tema.Texto2XS))
}

// tachado pinta el `line-through` de un párrafo: una raya por línea, del ancho de lo
// escrito, a la altura del tachado del folio.
type tachado struct {
	p     *ui.Parrafo
	rayas []*canvas.Rectangle
}

func (t *tachado) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (t *tachado) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(s)
	ancho := s.Width
	if t.p.Ancho > 0 {
		ancho = t.p.Ancho
	}
	lineas := ui.LineasDe(t.p, ancho)
	for len(t.rayas) < len(lineas) {
		r := canvas.NewRectangle(t.p.Tinta)
		t.rayas = append(t.rayas, r)
	}
	e := t.p.Escalon
	for i, r := range t.rayas {
		if i >= len(lineas) {
			r.Hide()
			continue
		}
		r.FillColor = t.p.Tinta
		r.Move(fyne.NewPos(0, float32(i)*e.Linea+lineaBase(e)-e.Tam*0.3))
		r.Resize(fyne.NewSize(ui.Medir(lineas[i], e, t.p.Peso).Width, max(e.Tam/16, 1)))
		r.Show()
	}
}

// zonaPulsable es el `<button class="block w-full rounded-sm text-left">` que lleva al
// cambio: se pulsa, se enfoca con el tabulador y lleva el anillo de foco.
type zonaPulsable struct {
	widget.BaseWidget
	rotulo    string
	alPulsar  func()
	contenido fyne.CanvasObject
	enfocado  bool
}

var (
	_ fyne.Tappable   = (*zonaPulsable)(nil)
	_ fyne.Focusable  = (*zonaPulsable)(nil)
	_ fyne.Accessible = (*zonaPulsable)(nil)
)

func nuevaZonaPulsable(rotulo string, alPulsar func(), contenido fyne.CanvasObject) *zonaPulsable {
	z := &zonaPulsable{rotulo: rotulo, alPulsar: alPulsar, contenido: contenido}
	z.ExtendBaseWidget(z)
	return z
}

func (z *zonaPulsable) Tapped(*fyne.PointEvent) { z.alPulsar() }
func (z *zonaPulsable) FocusGained()            { z.enfocado = true; z.Refresh() }
func (z *zonaPulsable) FocusLost()              { z.enfocado = false; z.Refresh() }
func (z *zonaPulsable) TypedRune(r rune) {
	if r == ' ' {
		z.alPulsar()
	}
}

func (z *zonaPulsable) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(z, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		z.alPulsar()
	}
}
func (z *zonaPulsable) AccessibilityLabel() string             { return z.rotulo }
func (z *zonaPulsable) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (z *zonaPulsable) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &zonaRenderer{z: z, aro: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v)}
	r.objs = r.aro.Antes(z.contenido)
	return r
}

type zonaRenderer struct {
	z    *zonaPulsable
	aro  *ui.Aro
	objs []fyne.CanvasObject
}

func (r *zonaRenderer) MinSize() fyne.Size           { return r.z.contenido.MinSize() }
func (r *zonaRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *zonaRenderer) Destroy()                     {}
func (r *zonaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.z.contenido.Move(fyne.NewPos(0, 0))
	r.z.contenido.Resize(t)
}

func (r *zonaRenderer) Refresh() {
	r.aro.Pintar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.aro.Ver(r.z.enfocado)
	r.z.contenido.Refresh()
}

// filaCentro es `flex items-center gap-N`: de izquierda a derecha, cada uno a su medida y
// centrado en el alto del más alto.
type filaCentro struct{ hueco float32 }

func (f filaCentro) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return ui.Fila{Hueco: f.hueco}.MinSize(objs)
}

func (f filaCentro) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := float32(0)
	for _, o := range objs {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + f.hueco
	}
}
