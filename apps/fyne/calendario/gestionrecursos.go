// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La gestión de salas y recursos: ResourcesModal.tsx, un `Modal size="lg"` que se abre
// desde el botón del edificio en «Mis calendarios» (solo `admin`/`owner` y con red).
// Arriba la ayuda; luego el alta o la edición —la clase con dos ChoiceCard `dense`, que
// en la edición pasa a una línea porque no se puede cambiar—, nombre, aforo y ubicación
// en dos columnas desde `sm`, correo y «aceptar automáticamente» en su caja; y abajo,
// tras un filete, el catálogo con editar y dar de baja. Pie: «Hecho».
//
// ponytail: sin la confirmación de baja, porque la web tampoco la pide.

// relajadaSM es `text-sm leading-relaxed` (1,625), el de la descripción de una
// SettingsSection.
var relajadaSM = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}

// ajustadaXS es `text-xs leading-snug` (1,375), la explicación bajo el conmutador.
var ajustadaXS = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.375 * tema.TextoXS.Tam}

// gestorRecursos es el estado vivo del diálogo.
type gestorRecursos struct {
	a      *App
	d      *ui.Dialogo
	cuerpo *fyne.Container
	// ayuda se crea una vez: mide su alto con el ancho que ya tiene.
	ayuda *ui.AyudaEnLinea

	lista    []api.Resource
	cargando bool

	// El formulario: `id` vacío es un alta.
	id, clase                    string
	nombre, aforo, lugar, correo *ui.Entrada
	auto                         *ui.Conmutador
	errNombre                    string
	guardar, cancelar            *ui.Boton
}

// gestionarRecursos abre el diálogo y pide el catálogo.
func (a *App) gestionarRecursos() {
	c := a.lienzo()
	if c == nil || a.Cal == nil {
		return
	}
	g := a.nuevoGestorRecursos()
	g.d.Mostrar(c)
	g.cargar()
}

func (a *App) nuevoGestorRecursos() *gestorRecursos {
	p := a.P
	g := &gestorRecursos{a: a, cargando: true, clase: api.RecursoSala}
	g.nombre = ui.NuevaEntrada(p.t("resources.namePlaceholder", nil))
	g.nombre.AlCambiar(func(string) {
		if g.errNombre != "" {
			g.errNombre, g.nombre.Invalida = "", false
			g.rehacer()
		}
	})
	g.aforo = ui.NuevaEntrada(p.t("resources.capacityPlaceholder", nil))
	g.lugar = ui.NuevaEntrada(p.t("resources.locationPlaceholder", nil))
	g.correo = ui.NuevaEntrada(p.t("resources.emailPlaceholder", nil))
	g.auto = ui.NuevoConmutador(p.t("resources.autoAccept", nil), func(bool) { g.rehacer() })
	g.auto.Encendido = true
	g.guardar = ui.NuevoBoton("", g.enviar).Con(ui.Primario, ui.TallaMD)
	g.cancelar = ui.NuevoBoton(p.t("common.cancel", nil), g.limpiar).Con(ui.Secundario, ui.TallaMD)

	g.ayuda = ui.NuevaAyudaEnLinea(p.t("resources.helpBody", nil)).ConTono(ui.AyudaInfo).ConTitulo(p.t("resources.helpTitle", nil))
	g.cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(6)})
	hecho := ui.NuevoBoton(p.t("common.done", nil), func() { g.d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	g.d = ui.NuevoDialogo(p.t("resources.title", nil), g.cuerpo).Con(ui.DialogoLG, hecho)
	g.d.RotuloCerrar = p.t("common.close", nil)
	g.d.Foco = g.nombre
	g.rehacer()
	return g
}

// cargar es el `listResources()` del `useEffect`: el catálogo sin franja.
func (g *gestorRecursos) cargar() {
	a := g.a
	a.lanzar(func() {
		ctx, parar := context.WithTimeout(context.Background(), plazo)
		defer parar()
		l, err := a.Cal.Resources(ctx, time.Time{}, time.Time{})
		a.enUI(func() {
			g.cargando = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, "toast.resourcesLoadFailed")
			} else {
				g.lista = l
			}
			g.rehacer()
		})
	})
}

// anchoCuerpo es lo que el diálogo `lg` deja dentro: 720 (o la ventana menos su `p-4`)
// menos los bordes y el `px-6`.
func (g *gestorRecursos) anchoCuerpo() float32 {
	var lienzo fyne.Size
	if c := g.a.lienzo(); c != nil {
		lienzo = c.Size()
	}
	return ui.AnchoCuerpoTalla(ui.DialogoLG, lienzo)
}

func (g *gestorRecursos) rehacer() {
	p := g.a.P
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := g.anchoCuerpo()
	anchoColumna := ancho
	if g.ventana() >= CorteSM {
		anchoColumna = (ancho - espacio3) / 2
	}

	titulo, desc := "resources.formNewTitle", "resources.formNewDesc"
	if g.id != "" {
		titulo, desc = "resources.formEditTitle", "resources.formEditDesc"
	}
	g.guardar.Texto = p.t("resources.create", nil)
	if g.id != "" {
		g.guardar.Texto = p.t("common.save", nil)
	}
	g.guardar.Refresh()
	formulario := seccionAjustes(p.t(titulo, nil), p.t(desc, nil), ancho, v,
		g.bloqueClase(ancho, v),
		ui.GrupoCampo(p.t("resources.name", nil), false, g.conPista(g.nombre, g.pistaNombre(), ancho), "", ""),
		container.New(&dosColumnas{Hueco: espacio3, Corte: CorteSM, ventana: g.ventana},
			ui.GrupoCampo(p.t("resources.capacity", nil), false, g.conPista(g.aforo, p.t("resources.capacityHint", nil), anchoColumna), "", ""),
			ui.GrupoCampo(p.t("resources.location", nil), false, g.conPista(g.lugar, p.t("resources.locationHint", nil), anchoColumna), "", "")),
		ui.GrupoCampo(p.t("resources.email", nil), false, g.conPista(g.correo, p.t("resources.emailHint", nil), ancho), "", ""),
		g.cajaAuto(ancho, v),
		g.acciones(),
	)
	g.cuerpo.Objects = []fyne.CanvasObject{g.ayuda, formulario, g.catalogo(ancho, v)}
	g.cuerpo.Refresh()
	if g.d != nil {
		g.d.Recolocar()
	}
}

// ventana es la de la app, que es lo que miran los cortes `sm:`.
func (g *gestorRecursos) ventana() float32 {
	if c := g.a.lienzo(); c != nil {
		return c.Size().Width
	}
	return 1280
}

// pistaNombre es la ayuda del nombre o, si falta, su error en el mismo sitio.
func (g *gestorRecursos) pistaNombre() string {
	if g.errNombre != "" {
		return g.errNombre
	}
	return g.a.P.t("resources.nameHint", nil)
}

// conPista es el control con su FieldHint debajo (`gap-1.5` del FieldGroup).
func (g *gestorRecursos) conPista(e *ui.Entrada, texto string, ancho float32) fyne.CanvasObject {
	pista := ui.NuevaPistaCampo(texto)
	pista.Ancho = ancho
	if e == g.nombre && g.errNombre != "" {
		pista.Tono = ui.AyudaPeligro
	}
	return container.New(ui.Columna{Hueco: espacio1p5}, e, pista)
}

// bloqueClase son las dos ChoiceCard del alta, o la línea «Sala · el tipo no se puede
// cambiar» de la edición.
func (g *gestorRecursos) bloqueClase(ancho float32, v tema.Variante) fyne.CanvasObject {
	p := g.a.P
	if g.id != "" {
		icono := iconos.Trazo(iconoRecurso(g.clase), iconos.TallaSM, "text-muted", v).Objeto()
		texto := ui.NuevoParrafo(p.t("resources.kindFixed", i18n.Vars{"kind": p.t(claveClase(g.clase), nil)}),
			tema.TextoSM, ui.Normal, "text-muted", v)
		return container.New(filaElastica{Hueco: espacio2, Elastico: 1}, icono, texto)
	}
	tarjeta := func(clase, titulo, desc string, icono iconos.Icono) *ui.TarjetaEleccion {
		t := ui.NuevaTarjetaEleccion(p.t(titulo, nil), p.t(desc, nil), func() { g.clase = clase; g.rehacer() }).A(ancho)
		t.Densa, t.Icono, t.Elegida = true, icono, g.clase == clase
		return t
	}
	// `<legend class="mb-1 text-sm font-semibold">` y `gap-2` entre las tarjetas. La
	// leyenda de un `<fieldset>` no es hija del flex: entre ella y la primera tarjeta
	// solo está su `mb-1` (medido: 3,5).
	leyenda := alto(rotuloFuerte(p.t("resources.legend", nil)), tema.TextoSM.Linea)
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, leyenda, container.New(ui.Columna{Hueco: espacio2},
		tarjeta(api.RecursoSala, "resources.roomTitle", "resources.roomDesc", iconos.Edificio),
		tarjeta(api.RecursoEquipo, "resources.equipmentTitle", "resources.equipmentDesc", iconos.Rejilla)))
}

// claveClase es `KIND_LABEL`.
func claveClase(clase string) string {
	if clase == api.RecursoEquipo {
		return "resources.kindEquipment"
	}
	return "resources.kindRoom"
}

// cajaAuto es `border-border rounded-md border px-3 py-2.5` con el conmutador y su
// explicación (`mt-1 text-xs leading-snug text-text-muted`).
func (g *gestorRecursos) cajaAuto(ancho float32, v tema.Variante) fyne.CanvasObject {
	clave := "resources.autoAcceptOff"
	if g.auto.Encendido {
		clave = "resources.autoAcceptOn"
	}
	interior := ancho - 2*(tema.Borde+espacio3)
	texto := ui.NuevoParrafo(g.a.P.t(clave, nil), ajustadaXS, ui.Normal, "text-muted", v).A(interior)
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conCaja{caja: caja, X: espacio3, Y: espacio2p5},
		append(caja.Objetos(), container.New(ui.Columna{Hueco: tema.Espacio(1)}, g.auto, texto))...)
}

// acciones es `flex justify-end gap-2`: «Cancelar» solo al editar, y el principal.
func (g *gestorRecursos) acciones() fyne.CanvasObject {
	botones := []fyne.CanvasObject{g.guardar}
	if g.id != "" {
		botones = []fyne.CanvasObject{g.cancelar, g.guardar}
	}
	return container.New(&pieRepartido{}, botones...)
}

// catalogo es la segunda sección: `border-border border-t pt-5`.
func (g *gestorRecursos) catalogo(ancho float32, v tema.Variante) fyne.CanvasObject {
	p := g.a.P
	var dentro fyne.CanvasObject
	switch {
	case g.cargando:
		dentro = ui.NuevoParrafo(p.t("common.loading", nil), tema.TextoSM, ui.Normal, "text-muted", v)
	case len(g.lista) == 0:
		vacio := ui.NuevoVacio(p.t("resources.emptyTitle", nil), p.t("resources.emptyDescription", nil))
		vacio.Ilustracion = iconos.Trazo(iconos.Edificio, 32, "text-subtle", v).Objeto()
		vacio.Accion = ui.NuevoBoton(p.t("resources.emptyAction", nil), func() {
			if c := g.a.lienzo(); c != nil {
				c.Focus(g.nombre)
			}
		}).Con(ui.Secundario, ui.TallaMD)
		dentro = vacio
	default:
		filas := make([]fyne.CanvasObject, len(g.lista))
		for i, r := range g.lista {
			filas[i] = g.filaCatalogo(r, v)
		}
		dentro = container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...)
	}
	seccion := seccionAjustes(p.t("resources.catalogueTitle", nil), p.t("resources.catalogueDesc", nil), ancho, v, dentro)
	filete := canvas.NewRectangle(tema.Color("border", v))
	return container.New(&conFileteArriba{Pad: tema.Espacio(5)}, filete, seccion)
}

// filaCatalogo es un `<li>` del catálogo: icono, el nombre y debajo aforo y ubicación en
// tono apagado, la clase en una insignia y los botones de editar y dar de baja.
func (g *gestorRecursos) filaCatalogo(r api.Resource, v tema.Variante) fyne.CanvasObject {
	p := g.a.P
	var partes []string
	if r.Capacity > 0 {
		partes = append(partes, p.t("resources.people", i18n.Vars{"count": r.Capacity}))
	}
	if r.Location != "" {
		partes = append(partes, r.Location)
	}
	detalle := strings.Join(partes, " · ")
	icono := iconos.Trazo(iconoRecurso(r.Kind), iconos.TallaSM, "text-muted", v).Objeto()
	nombre := container.New(&dosLineas{a: r.Name, b: detalle},
		ui.Texto(r.Name, tema.TextoSM, ui.Normal, tema.Color("text", v)),
		ui.Texto(detalle, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)))
	icBoton := func(i iconos.Icono, rotulo string, f func()) *ui.Boton {
		b := &ui.Boton{Variante: ui.Fantasma, Talla: ui.TallaSM, SoloIcono: true, Icono: i, TamIcono: iconos.TallaSM, Rotulo: rotulo, AlPulsar: f}
		b.ExtendBaseWidget(b)
		return b
	}
	editar := icBoton(iconos.Ajustes, p.t("resources.edit", i18n.Vars{"name": r.Name}), func() { g.editar(r) })
	quitar := icBoton(iconos.Cerrar, p.t("resources.remove", i18n.Vars{"name": r.Name}), func() { g.quitar(r) })
	fila := container.New(filaElastica{Hueco: espacio2, Elastico: 1}, icono, nombre,
		ui.NuevaInsignia(p.t(claveClase(r.Kind), nil), ui.InsigniaNeutra), editar, quitar)
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conCaja{caja: caja, X: espacio2p5, Y: espacio2}, append(caja.Objetos(), fila)...)
}

// editar es `edit(r)`: el formulario con los datos del recurso.
func (g *gestorRecursos) editar(r api.Resource) {
	g.id, g.clase = r.ID, r.Kind
	g.nombre.PonerTexto(r.Name)
	g.aforo.PonerTexto("")
	if r.Capacity > 0 {
		g.aforo.PonerTexto(strconv.Itoa(r.Capacity))
	}
	g.lugar.PonerTexto(r.Location)
	g.correo.PonerTexto(r.Email)
	g.auto.Encendido = r.AutoAccept
	g.auto.Refresh()
	g.errNombre, g.nombre.Invalida = "", false
	g.rehacer()
}

// limpiar es `setForm(EMPTY_FORM)`.
func (g *gestorRecursos) limpiar() {
	g.id, g.clase = "", api.RecursoSala
	for _, e := range []*ui.Entrada{g.nombre, g.aforo, g.lugar, g.correo} {
		e.PonerTexto("")
	}
	g.auto.Encendido = true
	g.auto.Refresh()
	g.errNombre, g.nombre.Invalida = "", false
	g.rehacer()
}

// enviar es `save()`.
func (g *gestorRecursos) enviar() {
	a, p := g.a, g.a.P
	nombre := strings.TrimSpace(g.nombre.Texto())
	if nombre == "" {
		g.errNombre, g.nombre.Invalida = p.t("resources.nameRequired", nil), true
		g.rehacer()
		if c := a.lienzo(); c != nil {
			c.Focus(g.nombre)
		}
		return
	}
	aforo, err := strconv.Atoi(strings.TrimSpace(g.aforo.Texto()))
	if err != nil || aforo < 0 {
		aforo = 0 // `Math.max(0, parseInt(v) || 0)`
	}
	in := api.ResourceInput{
		Kind: g.clase, Name: nombre, Capacity: aforo, Location: strings.TrimSpace(g.lugar.Texto()),
		Email: strings.TrimSpace(g.correo.Texto()), AutoAccept: g.auto.Encendido,
	}
	id := g.id
	g.guardar.Cargando = true
	g.guardar.Refresh()
	var l []api.Resource
	a.escribir(func(ctx context.Context) error {
		var err error
		if id != "" {
			_, err = a.Cal.UpdateResource(ctx, id, in)
		} else {
			_, err = a.Cal.CreateResource(ctx, in)
		}
		if err == nil {
			l, err = a.Cal.Resources(ctx, time.Time{}, time.Time{})
		}
		return err
	}, func(err error) {
		g.guardar.Cargando = false
		if err != nil {
			a.avisar(ui.AvisoPeligro, "toast.resourceSaveFailed")
			g.rehacer()
			return
		}
		if id != "" {
			a.avisar(ui.AvisoExito, "toast.resourceUpdated")
		} else {
			a.avisar(ui.AvisoExito, "toast.resourceCreated")
		}
		g.lista = l
		g.limpiar()
	})
}

// quitar es `remove(r)`.
func (g *gestorRecursos) quitar(r api.Resource) {
	a := g.a
	a.escribir(func(ctx context.Context) error { return a.Cal.DeleteResource(ctx, r.ID) }, func(err error) {
		if err != nil {
			a.avisar(ui.AvisoPeligro, "toast.resourceRemoveFailed")
			return
		}
		quedan := g.lista[:0:0]
		for _, x := range g.lista {
			if x.ID != r.ID {
				quedan = append(quedan, x)
			}
		}
		g.lista = quedan
		a.avisar(ui.AvisoExito, "toast.resourceRemoved")
		if g.id == r.ID {
			g.limpiar()
			return
		}
		g.rehacer()
	})
}

// ---- Piezas ----

// seccionAjustes es `SettingsSection plain`: `flex flex-col gap-3` con el `h2`
// (`sytue-h4`) y su descripción (`mt-1 text-sm leading-relaxed text-text-muted`)
// encima de lo de dentro.
func seccionAjustes(titulo, desc string, ancho float32, v tema.Variante, hijos ...fyne.CanvasObject) fyne.CanvasObject {
	cabeza := container.New(ui.Columna{Hueco: tema.Espacio(1)},
		ui.NuevoParrafo(titulo, tema.TextoLG, ui.SemiNegrita, "text", v).A(ancho),
		ui.NuevoParrafo(desc, relajadaSM, ui.Normal, "text-muted", v).A(min(ancho, 42*tema.Rem))) // `max-w-2xl`
	return container.New(ui.Columna{Hueco: espacio3}, append([]fyne.CanvasObject{cabeza}, hijos...)...)
}

// dosLineas es `flex min-w-0 flex-col`: el nombre en `text-sm truncate` y, debajo, el
// detalle en `text-xs truncate text-text-muted`. Iban en una sola línea y en la hoja de
// 390 se recortaban los dos («Auditorio de la planta b......»).
type dosLineas struct{ a, b string }

func (d *dosLineas) alto() float32 {
	if d.b == "" {
		return tema.TextoSM.Linea
	}
	return tema.TextoSM.Linea + tema.TextoXS.Linea
}

func (d *dosLineas) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, d.alto()) }

func (d *dosLineas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	ta, ok1 := objs[0].(*canvas.Text)
	tb, ok2 := objs[1].(*canvas.Text)
	if !ok1 || !ok2 {
		return
	}
	y := (t.Height - d.alto()) / 2
	ui.Truncar(ta, d.a, tema.TextoSM, ui.Normal, t.Width)
	ui.ColocarTexto(ta, tema.TextoSM, 0, y)
	tb.Hidden = d.b == ""
	ui.Truncar(tb, d.b, tema.TextoXS, ui.Normal, t.Width)
	ui.ColocarTexto(tb, tema.TextoXS, 0, y+tema.TextoSM.Linea)
}
