// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"context"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los dos diálogos de las vistas del menú: «Próximos cumpleaños» (BirthdaysModal.tsx) y
// la «Papelera» (`TrashModal` de ContactsApp.tsx). Los dos son `md` y con las filas de
// `Avatar sm` + dos líneas que usa la web para listar personas dentro de un diálogo.
//
//	cumpleaños  `ul rounded-md border divide-y divide-border/60 max-h-[60dvh]`       hoja
//	            fila `flex gap-3 px-3 py-2`, `hover:bg-bg-subtle`, hoy `bg-selected`
//	            nombre `text-sm font-medium`, fecha `text-xs text-muted`, a la
//	            derecha «Hoy» (`text-primary-text font-semibold`) o «En N días»
//	papelera    `p text-sm text-muted` y `ul gap-2` de cajas `rounded-md border p-3`  hoja
//	            con «Restaurar» (`secondary sm`) y «Eliminar» (`ghost sm`); al
//	            eliminar pregunta EN LA FILA: «¿Eliminar para siempre?», «Sí,
//	            eliminar» (`danger sm`) y «No»
var (
	// HuecoFilaCumple es el `gap-3` de una fila de cumpleaños, HuecoFilaPapelera el
	// `gap-2.5` de la de la papelera, y los rellenos, el `px-3 py-2` de la primera.
	HuecoFilaCumple    = tema.Espacio(3)
	HuecoFilaPapelera  = tema.Espacio(2.5)
	RellenoFilaCumpleX = tema.Espacio(3)
	RellenoFilaCumpleY = tema.Espacio(2)
	// RellenoCajaPapelera es el `p-3` de cada caja de la papelera, y HuecoPapelera el
	// `gap-2` entre ellas (y entre la frase y la lista).
	RellenoCajaPapelera = tema.Espacio(3)
	HuecoPapelera       = tema.Espacio(2)
	// RellenoAvisoDialogo es el `py-6` del texto suelto de un diálogo vacío o con error,
	// y RellenoRuedaDialogo el `py-8` de la rueda mientras carga.
	RellenoAvisoDialogo = tema.Espacio(6)
	RellenoRuedaDialogo = tema.Espacio(8)
)

// FraccionListaCumples es el `max-h-[60dvh]` de la lista de cumpleaños.
const FraccionListaCumples = 0.6

// ---- Cumpleaños ----

// cuando es «Hoy», «Mañana» o «En N días».
func (p *Pantalla) cuando(dias int) string {
	switch dias {
	case 0:
		return p.t("birthdays.today", nil)
	case 1:
		return p.t("birthdays.tomorrow", nil)
	}
	return p.t("birthdays.inDays", i18n.Vars{"count": dias})
}

// dialogoCumples es BirthdaysModal: los cumpleaños del año que viene, del más cercano
// al más lejano. Pulsar uno cierra el diálogo y abre su ficha.
func (p *Pantalla) dialogoCumples(lienzo fyne.Canvas, cs []Cumpleanos, abrir func(api.Contact)) *ui.Dialogo {
	var d *ui.Dialogo
	var cuerpo fyne.CanvasObject
	if len(cs) == 0 {
		cuerpo = ui.NuevoRotulo(p.t("birthdays.empty", nil), tema.TextoSM, ui.Normal, "text-muted")
	} else {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		filas := make([]fyne.CanvasObject, 0, 2*len(cs))
		for i, c := range cs {
			if i > 0 {
				filas = append(filas, filete(tema.Color("border", v), 0.6))
			}
			contacto := c.Contacto
			f := nuevaFilaCumple(p, c, func() { d.Cerrar(); abrir(contacto) })
			filas = append(filas, f)
		}
		lista := container.NewVScroll(container.New(ui.Columna{}, filas...))
		tope := FraccionListaCumples * lienzo.Size().Height
		cuerpo = enCajaConBorde(container.New(&altoMaximo{Max: tope}, lista), 0, v)
	}
	d = ui.NuevoDialogo(p.t("birthdays.title", nil), cuerpo)
	d.RotuloCerrar = p.t("ui.close", nil)
	return d
}

// filaCumple es el `<button>` de un cumpleaños.
type filaCumple struct {
	ui.Pulsable
	nombre, fecha, cuando string
	hoy                   bool
}

// AltoFilaCumple es `py-2` y el avatar `sm` (32), que es lo más alto de la fila.
var AltoFilaCumple = 2*RellenoFilaCumpleY + ui.MedidaAvatar(ui.AvatarSM).Height

func nuevaFilaCumple(p *Pantalla, c Cumpleanos, alPulsar func()) *filaCumple {
	fecha := i18n.FormatDayMonthLong(p.locale(), c.Fecha)
	if c.Edad > 0 {
		fecha += " · " + p.t("birthdays.turns", i18n.Vars{"age": c.Edad})
	}
	f := &filaCumple{nombre: c.Contacto.DisplayName(), fecha: fecha, cuando: p.cuando(c.Dias), hoy: c.Dias == 0}
	f.Nombre, f.AlPulsar = f.nombre, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaCumple) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaCumpleRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		avatar: ui.NuevoAvatar(f.nombre).Con(ui.AvatarSM),
		nombre: ui.Texto(f.nombre, tema.TextoSM, ui.Medio, tema.Color("text", v)),
		fecha:  ui.Texto(f.fecha, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
		cuando: ui.Texto(f.cuando, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
		aro:    ui.NuevoAro(0, ui.SinHueco, v).Abierto(),
	}
	if f.hoy {
		r.cuando = ui.Texto(f.cuando, tema.TextoXS, ui.SemiNegrita, tema.Color("primary-text", v))
	}
	r.Refresh()
	return r
}

type filaCumpleRenderer struct {
	f                     *filaCumple
	fondo                 *canvas.Rectangle
	avatar                *ui.Avatar
	nombre, fecha, cuando *canvas.Text
	aro                   *ui.Aro
}

func (r *filaCumpleRenderer) MinSize() fyne.Size { return fyne.NewSize(0, AltoFilaCumple) }
func (r *filaCumpleRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.fondo, r.avatar, r.nombre, r.fecha, r.cuando}, r.aro.Objetos()...)
}
func (r *filaCumpleRenderer) Destroy() {}

func (r *filaCumpleRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	// `sytue-focus-ring-inner`: el anillo va POR DENTRO (`inset`), que el borde de la
	// lista lo recortaría. Es el aro abierto colocado alrededor de la caja encogida.
	g := tema.AnilloFoco
	r.aro.ColocarEn(fyne.NewPos(g, g), fyne.NewSize(max(t.Width-2*g, 0), max(t.Height-2*g, 0)))
	av := ui.MedidaAvatar(ui.AvatarSM)
	r.avatar.Move(fyne.NewPos(RellenoFilaCumpleX, (t.Height-av.Height)/2))
	r.avatar.Resize(av)
	derecha := t.Width - RellenoFilaCumpleX
	ancho := ui.Medir(r.f.cuando, tema.TextoXS, pesoCuando(r.f.hoy)).Width
	ui.ColocarTexto(r.cuando, tema.TextoXS, derecha-ancho, (t.Height-tema.TextoXS.Linea)/2)
	x := RellenoFilaCumpleX + av.Width + HuecoFilaCumple
	cabe := max(derecha-ancho-HuecoFilaCumple-x, 0)
	y := (t.Height - tema.TextoSM.Linea - tema.TextoXS.Linea) / 2
	ui.Truncar(r.nombre, r.f.nombre, tema.TextoSM, ui.Medio, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoSM, x, y)
	ui.Truncar(r.fecha, r.f.fecha, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(r.fecha, tema.TextoXS, x, y+tema.TextoSM.Linea)
}

func pesoCuando(hoy bool) ui.Peso {
	if hoy {
		return ui.SemiNegrita
	}
	return ui.Normal
}

func (r *filaCumpleRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// `hover:bg-bg-subtle` va detrás de `bg-selected` en la hoja, así que gana.
	switch {
	case r.f.Encima():
		r.fondo.FillColor = tema.Color("bg-subtle", v)
	case r.f.hoy:
		r.fondo.FillColor = tema.Color("selected", v)
	default:
		r.fondo.FillColor = color.NRGBA{}
	}
	r.fondo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.Layout(r.f.Size())
}

// ---- Papelera ----

// papelera es el estado de TrashModal mientras está abierto.
type papelera struct {
	p          *Pantalla
	lienzo     fyne.Canvas
	d          *ui.Dialogo
	cuerpo     *fyne.Container
	cargando   bool
	fallo      bool
	elementos  []api.TrashItem
	ocupado    string // el id con una petición en vuelo
	confirmado string // el id que pregunta «¿Eliminar para siempre?»

	alRestaurar, alPurgar func(it api.TrashItem, listo func(error))
}

func (p *Pantalla) nuevaPapelera(lienzo fyne.Canvas) *papelera {
	x := &papelera{p: p, lienzo: lienzo, cargando: true, cuerpo: container.New(ui.Columna{})}
	cerrar := ui.NuevoBoton(p.t("common.close", nil), nil).Con(ui.Secundario, ui.TallaMD)
	x.d = ui.NuevoDialogo(p.t("trash.title", nil), x.cuerpo).Con(ui.DialogoMD, cerrar)
	cerrar.AlPulsar = x.d.Cerrar
	x.d.RotuloCerrar = p.t("ui.close", nil)
	x.pintar()
	return x
}

// cargada pone lo que devolvió el servicio.
func (x *papelera) cargada(elementos []api.TrashItem, err error) {
	x.cargando, x.fallo, x.elementos = false, err != nil, elementos
	x.pintar()
}

func (x *papelera) quitar(id string) {
	for i, it := range x.elementos {
		if it.ID == id {
			x.elementos = append(x.elementos[:i:i], x.elementos[i+1:]...)
			return
		}
	}
}

// avisoCentrado es el `<p class="py-6 text-center text-sm text-muted">`.
func avisoCentrado(texto string) fyne.CanvasObject {
	return container.New(ui.Margen{Arr: RellenoAvisoDialogo, Abj: RellenoAvisoDialogo},
		container.NewCenter(ui.NuevoRotulo(texto, tema.TextoSM, ui.Normal, "text-muted")))
}

func (x *papelera) pintar() {
	p := x.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var hijos []fyne.CanvasObject
	switch {
	case x.cargando:
		// ponytail: la rueda no anuncia su `label` («Cargando papelera»): `ui.Rueda` no
		// es `fyne.Accessible` todavía.
		hijos = []fyne.CanvasObject{container.New(ui.Margen{Arr: RellenoRuedaDialogo, Abj: RellenoRuedaDialogo},
			container.NewCenter(ui.NuevaRueda(LadoRuedaMenu, "text-muted", v)))}
	case x.fallo:
		hijos = []fyne.CanvasObject{avisoCentrado(p.t("trash.loadFailed", nil))}
	case len(x.elementos) == 0:
		hijos = []fyne.CanvasObject{avisoCentrado(p.t("trash.empty", nil))}
	default:
		filas := make([]fyne.CanvasObject, 0, len(x.elementos))
		for _, it := range x.elementos {
			filas = append(filas, x.fila(it, v))
		}
		hijos = []fyne.CanvasObject{
			// Parte de línea como el `<p>` de la web: en la hoja de 390 se salía del panel.
			ui.NuevoParrafo(p.t("trash.description", nil), tema.TextoSM, ui.Normal, "text-muted", v),
			container.New(ui.Columna{Hueco: HuecoPapelera}, filas...),
		}
	}
	x.cuerpo.Objects = []fyne.CanvasObject{container.New(ui.Columna{Hueco: HuecoPapelera}, hijos...)}
	x.cuerpo.Refresh()
	recolocar(x.lienzo, x.d)
}

// fila es una caja de la papelera: la persona a la izquierda y sus acciones.
func (x *papelera) fila(it api.TrashItem, v tema.Variante) fyne.CanvasObject {
	p := x.p
	meta := it.PrimaryEmail
	if meta == "" {
		meta = it.Organization
	}
	if meta == "" {
		meta = "—"
	}
	persona := personaDeFila(it.FormattedName, meta, v)
	enVuelo := x.ocupado != ""
	boton := func(clave string, var_ ui.Variante, f func()) *ui.Boton {
		b := ui.NuevoBoton(p.t(clave, nil), f).Con(var_, ui.TallaSM)
		b.Inerte = enVuelo
		return b
	}
	var rotulo fyne.CanvasObject
	var acciones []fyne.CanvasObject
	if x.confirmado == it.ID {
		si := boton("trash.confirmYes", ui.Peligro, func() { x.purgar(it) })
		si.Cargando, si.Inerte = x.ocupado == it.ID, enVuelo && x.ocupado != it.ID
		rotulo = ui.NuevoRotulo(p.t("trash.confirmPurge", nil), tema.TextoXS, ui.Normal, "text-muted")
		acciones = []fyne.CanvasObject{
			si,
			boton("trash.confirmNo", ui.Secundario, func() { x.confirmado = ""; x.pintar() }),
		}
	} else {
		restaurar := boton("trash.restore", ui.Secundario, func() { x.restaurar(it) })
		restaurar.Cargando, restaurar.Inerte = x.ocupado == it.ID, enVuelo && x.ocupado != it.ID
		acciones = []fyne.CanvasObject{
			restaurar,
			boton("common.delete", ui.Fantasma, func() { x.confirmado = it.ID; x.pintar() }),
		}
	}
	hueco := tema.Espacio(1.5)
	if x.lienzo != nil && x.lienzo.Size().Width < ui.Estrecho {
		// `max-md:flex-wrap`: la persona entera arriba y las acciones debajo, a la
		// derecha (la pregunta, a la izquierda). En una fila, «Restaurar» y «Eliminar»
		// dejaban el nombre en 102 y la confirmación lo hacía desaparecer.
		if rotulo == nil {
			rotulo = layout.NewSpacer()
		}
		abajo := container.New(filaEntre{}, rotulo, container.New(filaCentradaHueco{Hueco: hueco}, acciones...))
		return enCajaConBorde(container.New(ui.Columna{Hueco: tema.Espacio(2)}, persona, abajo), RellenoCajaPapelera, v)
	}
	if rotulo != nil {
		acciones = append([]fyne.CanvasObject{rotulo}, acciones...)
	}
	fila := container.New(filaEntre{}, persona,
		container.New(filaCentradaHueco{Hueco: hueco}, acciones...))
	return enCajaConBorde(fila, RellenoCajaPapelera, v)
}

func (x *papelera) restaurar(it api.TrashItem) {
	if x.alRestaurar == nil {
		return
	}
	x.ocupado = it.ID
	x.pintar()
	x.alRestaurar(it, func(err error) {
		x.ocupado = ""
		if err == nil {
			x.quitar(it.ID)
		}
		x.pintar()
	})
}

func (x *papelera) purgar(it api.TrashItem) {
	if x.alPurgar == nil {
		return
	}
	x.ocupado = it.ID
	x.pintar()
	x.alPurgar(it, func(err error) {
		x.ocupado = ""
		if err == nil {
			x.quitar(it.ID)
			x.confirmado = ""
		}
		x.pintar()
	})
}

// personaDeFila es el `flex items-center gap-2.5` con el avatar `sm`, el nombre
// (`text-sm font-medium`) y la línea `.sytue-meta` de debajo, truncados.
func personaDeFila(nombre, meta string, v tema.Variante) fyne.CanvasObject {
	return personaCon(ui.NuevoAvatar(nombre).Con(ui.AvatarSM), nombre, meta, v)
}

// personaCon es lo mismo con otro delantero (los avatares solapados de un grupo).
func personaCon(delantero fyne.CanvasObject, nombre, meta string, v tema.Variante) fyne.CanvasObject {
	n := ui.Texto(nombre, tema.TextoSM, ui.Medio, tema.Color("text", v))
	m := ui.Texto(meta, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	return container.New(&personaLayout{nombre: nombre, meta: meta, n: n, m: m}, delantero, n, m)
}

type personaLayout struct {
	nombre, meta string
	n, m         *canvas.Text
}

func (l *personaLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	av := objs[0].MinSize()
	texto := max(ui.Medir(l.nombre, tema.TextoSM, ui.Medio).Width, ui.Medir(l.meta, tema.TextoXS, ui.Normal).Width)
	return fyne.NewSize(av.Width+HuecoFilaPapelera+texto, max(av.Height, tema.TextoSM.Linea+tema.TextoXS.Linea))
}

func (l *personaLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	av := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, (t.Height-av.Height)/2))
	objs[0].Resize(av)
	x := av.Width + HuecoFilaPapelera
	cabe := max(t.Width-x, 0)
	y := (t.Height - tema.TextoSM.Linea - tema.TextoXS.Linea) / 2
	ui.Truncar(l.n, l.nombre, tema.TextoSM, ui.Medio, cabe)
	ui.ColocarTexto(l.n, tema.TextoSM, x, y)
	ui.Truncar(l.m, l.meta, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(l.m, tema.TextoXS, x, y+tema.TextoSM.Linea)
}

// ---- Piezas de los dos ----

// enCajaConBorde mete un hijo en una caja `rounded-md border` sin fondo propio (se ve
// el del diálogo), con ese relleno por dentro.
func enCajaConBorde(dentro fyne.CanvasObject, relleno float32, v tema.Variante) fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&enCaja{caja: caja, margen: relleno + tema.Borde}, append(caja.Objetos(), dentro)...)
}

// filete es un `divide-y` de un píxel con ese alfa sobre el color del borde
// (`divide-border/60`).
func filete(c color.NRGBA, alfa float32) fyne.CanvasObject {
	c.A = uint8(float32(c.A)*alfa + 0.5)
	r := canvas.NewRectangle(c)
	r.SetMinSize(fyne.NewSize(0, tema.Borde))
	return r
}

// altoMaximo deja a su hijo lo que pida hasta un tope: el `max-h-*` de un bloque que
// se desplaza por dentro.
type altoMaximo struct{ Max float32 }

func (a *altoMaximo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var alto float32
	for _, o := range objs {
		if s, es := o.(*container.Scroll); es && s.Content != nil {
			alto = max(alto, s.Content.MinSize().Height)
			continue
		}
		alto = max(alto, o.MinSize().Height)
	}
	if a.Max > 0 {
		alto = min(alto, a.Max)
	}
	return fyne.NewSize(0, alto)
}

func (a *altoMaximo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// filaCentradaHueco es `flex items-center gap-*` para las acciones de una fila.
type filaCentradaHueco struct{ Hueco float32 }

func (f filaCentradaHueco) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return ui.Fila{Hueco: f.Hueco}.MinSize(objs)
}

func (f filaCentradaHueco) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var x float32
	for _, o := range objs {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + f.Hueco
	}
}

// ---- En la App ----

// verCumples abre «Próximos cumpleaños»: los de TODO el año, no los 30 del contador.
func (a *App) verCumples() *ui.Dialogo {
	lienzo := a.lienzo()
	if lienzo == nil {
		return nil
	}
	cs := ProximosCumpleanos(a.todos, time.Now(), 365)
	d := a.Pantalla.dialogoCumples(lienzo, cs, a.elegir)
	d.Mostrar(lienzo)
	return d
}

// verPapelera abre la papelera y la carga.
func (a *App) verPapelera() *papelera {
	lienzo := a.lienzo()
	if lienzo == nil || a.svc == nil {
		return nil
	}
	x := a.Pantalla.nuevaPapelera(lienzo)
	x.alRestaurar = func(it api.TrashItem, listo func(error)) {
		a.peticion(func(ctx context.Context) error { return a.svc.Restore(ctx, it.ID) },
			"trash.restored", "trash.restoreFailed", "common.tryAgain", func(err error) {
				listo(err)
				if err == nil {
					a.recargarTodo()
				}
			})
	}
	x.alPurgar = func(it api.TrashItem, listo func(error)) {
		a.peticion(func(ctx context.Context) error { return a.svc.Purge(ctx, it.ID) },
			"trash.purged", "trash.purgeFailed", "common.tryAgain", func(err error) {
				listo(err)
				if err == nil {
					a.recargarTodo()
				}
			})
	}
	a.mostrarEscritura("papelera", x.d, lienzo)
	a.enFondo(func() {
		ctx, fin := a.ctx()
		elementos, err := a.svc.Trash(ctx, 0) // sin tope: el del servicio, como la web
		fin()
		fyne.Do(func() { x.cargada(elementos, err) })
	})
	return x
}

// recargarTodo es el `onChanged` de la papelera: la libreta y los destacados.
func (a *App) recargarTodo() {
	if a.Pantalla.Activa != nil {
		a.recargar(*a.Pantalla.Activa)
	}
	a.enFondo(a.cargarFavoritos)
}
