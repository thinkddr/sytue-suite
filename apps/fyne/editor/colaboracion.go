// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// La pestaña «Colaboración» (`Colaboracion.tsx`, §6.3): comentarios, control de cambios
// (solo en el documento), versiones y compartir. Lo que cada botón HACE lo pone el
// producto en el contexto; la pestaña es la misma para los tres, como en la web.
func cuerpoColaboracion(c *Contexto) fyne.CanvasObject {
	nuevo := c.accion(iconos.EdComentarioNuevo, "comments.new", c.SinRed, c.NuevoComentario)
	visibles := c.Panel == PanelComentarios
	rotulo := "comments.show"
	if visibles {
		rotulo = "comments.hide"
	}
	mostrar := NuevoBotonCinta(iconos.EdComentarios, c.Traducir(rotulo), func() { c.AbrirPanel(PanelComentarios) })
	mostrar.Activo = visibles

	guardar := NuevoBotonCinta(iconos.EdHistorial, c.Traducir("versions.save"), nil)
	guardar.AlPulsar = c.preguntarVersion(guardar)
	guardar.Inerte = c.SinRed || guardar.AlPulsar == nil
	historial := NuevoChipCinta(c.Traducir("versions.historyShort"), c.Traducir("versions.history"), nil)
	historial.AlPulsar = func() { c.abrirHistorial(historial) }
	historial.Inerte = c.SinRed || c.Versiones == nil

	versiones := []fyne.CanvasObject{guardar, historial}
	if c.Actividad != nil { // solo el propietario: el servicio da 403 al resto
		actividad := NuevoChipCinta(c.Traducir("audit.history"), c.Traducir("audit.history"), nil)
		actividad.AlPulsar = func() { c.abrirActividad(actividad) }
		actividad.Inerte = c.SinRed
		versiones = append(versiones, actividad)
	}
	grupos := []fyne.CanvasObject{NuevoGrupo(c.Traducir("group.comments"), nuevo, mostrar), SeparadorGrupos()}
	if c.Tipo == Documento {
		grupos = append(grupos, c.grupoRevision(), SeparadorGrupos())
	}
	return Grupos(append(grupos,
		NuevoGrupo(c.Traducir("group.versions"), versiones...),
		SeparadorGrupos(),
		NuevoGrupo(c.Traducir("group.share"), c.botonCompartir()),
	)...)
}

// grupoRevision es «Revisión»: activar el control de cambios, abrir el panel de revisión
// y aceptar o rechazar todo, con confirmación porque es masivo.
func (c *Contexto) grupoRevision() fyne.CanvasObject {
	rastrear := c.accion(iconos.EdRevision, "review.track", c.SoloLectura, c.AlternarRastreo)
	rastrear.Activo = c.EnRastreo != nil && c.EnRastreo()
	abierto := c.Panel == PanelRevision
	rotulo := "review.panel"
	if abierto {
		rotulo = "review.panelHide"
	}
	panel := NuevoBotonCinta(iconos.EdPanelDerecho, c.Traducir(rotulo), func() { c.AbrirPanel(PanelRevision) })
	panel.Activo = abierto
	resolver := func(aceptar bool) func() {
		if c.ResolverCambios == nil {
			return nil
		}
		return func() { c.ConfirmarResolver(rastrear, aceptar) }
	}
	aceptar := c.accion(iconos.EdTic, "review.acceptAll", c.SoloLectura, resolver(true))
	rechazar := c.accion(iconos.EdCerrar, "review.rejectAll", c.SoloLectura, resolver(false))
	return NuevoGrupo(c.Traducir("group.review"), rastrear, panel, aceptar, rechazar)
}

// ConfirmarResolver es `resolverCambios`: pregunta, y con el sí acepta (o rechaza) todos
// los cambios y lo dice con un aviso —o que no había ninguno—. Rechazar va en peligro.
func (c *Contexto) ConfirmarResolver(desde fyne.CanvasObject, aceptar bool) {
	verbo, pregunta, hecho := "review.rejectAll", "review.rejectAll.confirm", "review.rejected"
	if aceptar {
		verbo, pregunta, hecho = "review.acceptAll", "review.acceptAll.confirm", "review.accepted"
	}
	c.Confirmar(desde, verbo, pregunta, !aceptar, func() {
		if c.ResolverCambios == nil || c.SoloLectura {
			return
		}
		tono, titulo := ui.AvisoInfo, "review.nothing"
		if c.ResolverCambios(aceptar) {
			tono, titulo = ui.AvisoExito, hecho
		}
		if c.Notificar != nil {
			c.Notificar(tono, c.Traducir(titulo))
		}
		c.refrescar()
	})
}

// botonCompartir es el icono de «Compartir» de la cinta: el mismo panel que la cabecera,
// colgado de él (`align="start"`).
func (c *Contexto) botonCompartir() *BotonCinta {
	b := NuevoBotonCinta(iconos.EdCompartir, c.Traducir("share.action"), nil)
	if c.Compartir != nil {
		b.AlPulsar = func() { c.Compartir(b) }
	}
	b.Inerte = c.SinRed || c.Compartir == nil
	return b
}

// accion es un botón de la cinta que llama a un gancho del producto; sin gancho, inerte.
func (c *Contexto) accion(i iconos.Ed, rotulo string, inerte bool, f func()) *BotonCinta {
	b := NuevoBotonCinta(i, c.Traducir(rotulo), f)
	b.Inerte = inerte || f == nil
	return b
}

// preguntarVersion es `guardarVersion` de la web, el mismo en el menú Archivo y en
// Colaboración: pregunta la etiqueta, OPCIONAL (`permitirVacio`), y cancelar no guarda
// nada. nil sin gancho, para que el botón salga inerte.
func (c *Contexto) preguntarVersion(desde fyne.CanvasObject) func() {
	if c.GuardarVersion == nil {
		return nil
	}
	return func() {
		lienzo := fyne.CurrentApp().Driver().CanvasForObject(desde)
		if lienzo == nil {
			return
		}
		ui.PreguntaOpcional(c.Traducir("versions.labelPrompt"), "", c.Traducir("ui.cancel"), c.Traducir("ui.save"),
			c.GuardarVersion).Mostrar(lienzo)
	}
}

// abrirHistorial es el Dropdown de «Historial»: pide las versiones al abrir
// (`loadVersions`) y, al elegir una, confirma antes de restaurar.
func (c *Contexto) abrirHistorial(disparador *ChipCinta) {
	if c.Versiones == nil {
		return
	}
	c.Versiones(func(vs []api.DocVersion) {
		var elems []fyne.CanvasObject
		if len(vs) == 0 {
			// Texto estático, sin `role=menuitem`: no recibe el foco del menú.
			elems = append(elems, ui.NuevoParrafo(c.Traducir("versions.empty"), tema.TextoSM, ui.Normal, "text-muted",
				tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
		}
		for _, v := range vs {
			nombre := v.Label
			if nombre == "" {
				nombre = c.Traducir("versions.untitled")
			}
			e := ui.NuevoElementoMenu(nombre+" — "+i18n.FormatDateTime(c.Loc, v.CreatedAt), iconos.Ninguno,
				func() { c.confirmarRestaurar(disparador, v) })
			elems = append(elems, e)
		}
		m := ui.NuevoMenu(elems...)
		m.AlInicio = true
		m.Rotulo = c.Traducir("versions.history")
		disparador.mostrarMenu(m)
	})
}

// abrirActividad es el desplegable «Actividad» (solo el propietario): quién cambió qué y
// cuándo, una línea por grupo de operaciones y sin el contenido de lo escrito. Pide la
// lista al abrir; un fallo dice que no se pudo cargar, no «sin actividad».
func (c *Contexto) abrirActividad(disparador *ChipCinta) {
	if c.Actividad == nil {
		return
	}
	c.Actividad(func(es []api.DocActividad, err error) {
		tv := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		parrafo := func(texto string) fyne.CanvasObject {
			return ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, "text-muted", tv)
		}
		var elems []fyne.CanvasObject
		switch {
		case err != nil:
			elems = append(elems, parrafo(c.Traducir("audit.error")))
		case len(es) == 0:
			elems = append(elems, parrafo(c.Traducir("audit.empty")))
		}
		for _, e := range es {
			elems = append(elems, ui.NuevoParrafo(e.Descripcion()+" · "+i18n.FormatDateTime(c.Loc, e.At),
				tema.TextoSM, ui.Normal, "text", tv))
		}
		m := ui.NuevoMenu(elems...)
		m.AlInicio = true
		m.Rotulo = c.Traducir("audit.history")
		disparador.mostrarMenu(m)
	})
}

// confirmarRestaurar pregunta antes de volver a una versión.
func (c *Contexto) confirmarRestaurar(desde fyne.CanvasObject, v api.DocVersion) {
	if c.RestaurarVersion == nil {
		return
	}
	c.Confirmar(desde, "versions.restoreAction", "versions.restoreConfirm", false, func() { c.RestaurarVersion(v) },
		OpcionConfirmar{Titulo: "versions.restoreTitle"})
}

// OpcionConfirmar es lo que una confirmación puede añadir: un título propio (una clave;
// sin él, el verbo) y las variables de la pregunta.
type OpcionConfirmar struct {
	Titulo string
	Vars   i18n.Vars
}

// Confirmar es el `confirmar` de la web (`useConfirm`): el título es el verbo —u
// `OpcionConfirmar.Titulo`, como el `titulo` de la web—, el botón el verbo, y el cuerpo
// la pregunta en `text-sm leading-relaxed`. `si` va tras cerrar el diálogo.
func (c *Contexto) Confirmar(desde fyne.CanvasObject, verbo, pregunta string, peligro bool, si func(), opc ...OpcionConfirmar) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(desde)
	if lienzo == nil {
		return
	}
	var o OpcionConfirmar
	if len(opc) > 0 {
		o = opc[0]
	}
	titulo := verbo
	if o.Titulo != "" {
		titulo = o.Titulo
	}
	relajado := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: tema.TextoSM.Tam * 1.625}
	// Sin ancho fijo: se reparte al del cuerpo. Y con sus variables: «¿Borrar la macro
	// «{name}»?» salía tal cual.
	cuerpo := ui.NuevoParrafo(c.Traducir(pregunta, o.Vars), relajado, ui.Normal, "text",
		tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	var d *ui.Dialogo
	d = ui.Confirmacion(c.Traducir(titulo), c.Traducir("ui.cancel"), c.Traducir(verbo),
		peligro, cuerpo, func() { d.Cerrar() }, func() { d.Cerrar(); si() })
	d.Mostrar(lienzo)
}

// ---- ChipCinta ----

// MedidaChipCinta es CHIP_BTN: `h-7 gap-1 px-2 text-xs` con el chevrón `sm` (14) detrás
// y el borde de un píxel. Con el dedo, 44 de alto, como todo lo de la cinta.
func MedidaChipCinta(texto string) fyne.Size {
	return fyne.NewSize(ui.Medir(texto, tema.TextoXS, ui.Normal).Width+tema.Espacio(1)+IconoChevron+2*tema.Espacio(2),
		ui.SueloTactil(LadoBotonCinta))
}

// ChipCinta es el disparador de «Historial»: `border border-border bg-surface rounded-sm
// text-text hover:bg-bg-subtle disabled:opacity-40`, con su chevrón.
//
// Plano es el «Más» de la banda (Ribbon.tsx): sin caja, como los botones de la cinta —con
// el borde se leía como un desplegable más—, en `text-muted`, `hover:bg-bg-subtle` y, con
// la banda desplegada (Abierto, `aria-expanded`), en `selected` con el chevrón hacia arriba.
type ChipCinta struct {
	activable
	Texto          string
	Plano, Abierto bool
	// menu es el desplegable que abre, si abre uno: mientras está abierto, el chip lo dice
	// con el relleno `selected` (`aria-expanded:bg-selected` de CHIP_BTN), como «Más».
	menu *ui.Menu
}

// abierto es Abierto, o el menú que cuelga del chip a la vista.
func (b *ChipCinta) abierto() bool { return b.Abierto || (b.menu != nil && b.menu.Abierto()) }

// mostrarMenu abre ese menú colgado del chip y se lo apunta, para pintarse abierto.
func (b *ChipCinta) mostrarMenu(m *ui.Menu) {
	b.menu = m
	m.Mostrar(b)
	b.Refresh()
}

var _ fyne.Widget = (*ChipCinta)(nil)

// NuevoChipCinta crea el chip con su texto y su nombre accesible, ya traducidos.
func NuevoChipCinta(texto, rotulo string, alPulsar func()) *ChipCinta {
	b := &ChipCinta{Texto: texto}
	b.Rotulo, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MinSize es la medida declarada.
func (b *ChipCinta) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return MedidaChipCinta(b.Texto)
}

// CreateRenderer compone la caja, el texto, el chevrón y el aro.
func (b *ChipCinta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &chipRenderer{
		b:     b,
		caja:  ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioSM),
		aro:   ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		texto: ui.Texto(b.Texto, tema.TextoXS, ui.Normal, tema.Color("text", v)),
		glifo: iconos.Trazo(iconos.ChevronAbajo, IconoChevron, "text", v),
	}
	objs := append(r.caja.Objetos(), r.texto, r.glifo.Objeto())
	if b.Plano {
		// El chevrón del «Más» es el del editor (`chevronDown`/`chevronUp`), con su par.
		r.glifo = iconos.TrazoEd(iconos.EdChevronAbajo, IconoChevron, "text-muted", v)
		r.arriba = iconos.TrazoEd(iconos.EdChevronArriba, IconoChevron, "text-muted", v)
		objs = append(r.caja.Objetos(), r.texto, r.glifo.Objeto(), r.arriba.Objeto())
	}
	r.objeto = r.aro.Antes(objs...)
	return r
}

type chipRenderer struct {
	b      *ChipCinta
	caja   *ui.Caja
	aro    *ui.Aro
	texto  *canvas.Text
	glifo  *iconos.Dibujo
	arriba *iconos.Dibujo // el chevrón del «Más» abierto
	objeto []fyne.CanvasObject
}

func (r *chipRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *chipRenderer) Objects() []fyne.CanvasObject { return r.objeto }
func (r *chipRenderer) Destroy()                     {}

func (r *chipRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	x := tema.Espacio(2)
	ui.ColocarTexto(r.texto, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
	x += ui.Medir(r.b.Texto, tema.TextoXS, ui.Normal).Width + tema.Espacio(1)
	r.glifo.Colocar(x, (t.Height-IconoChevron)/2, IconoChevron)
	if r.arriba != nil {
		r.arriba.Colocar(x, (t.Height-IconoChevron)/2, IconoChevron)
	}
}

func (r *chipRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.b.Plano {
		r.pintarPlano(v)
		return
	}
	fondo, borde, tinta := tema.Color("surface", v), tema.Color("border", v), tema.Color("text", v)
	switch {
	case r.b.abierto():
		fondo = tema.Color("selected", v)
	case r.b.Encima() && !r.b.Inerte:
		fondo = tema.Color("bg-subtle", v)
	}
	if r.b.Inerte {
		// `disabled:opacity-40` sin opacidad de grupo: aplanado contra la cinta.
		borde, tinta = al40(borde, v), al40(tinta, v)
	}
	r.caja.Pintar(fondo, borde)
	r.texto.Color = tinta
	r.texto.Refresh()
	r.glifo.TenirCon(tinta)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
}

// pintarPlano es el «Más»: las reglas de BOTON_DE_BARRA con `aria-expanded` en lugar de
// `aria-pressed`.
func (r *chipRenderer) pintarPlano(v tema.Variante) {
	fondo, tinta := color.NRGBA{}, tema.Color("text-muted", v)
	switch {
	case r.b.abierto():
		fondo, tinta = tema.Color("selected", v), tema.Color("text", v)
	case r.b.Encima():
		fondo, tinta = tema.Color("bg-subtle", v), tema.Color("text", v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.texto.Color = tinta
	r.texto.Refresh()
	r.glifo.TenirCon(tinta)
	r.arriba.TenirCon(tinta)
	abajo, arriba := r.glifo.Objeto(), r.arriba.Objeto()
	if r.b.abierto() {
		abajo, arriba = arriba, abajo
	}
	abajo.Show()
	arriba.Hide()
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
}

// al40 es un color al 40 % sobre la superficie de la cinta.
func al40(c color.NRGBA, v tema.Variante) color.NRGBA {
	c.A = uint8(float32(c.A) * 0.4)
	return tokens.SobreSRGB(c, tema.Color("surface", v))
}
