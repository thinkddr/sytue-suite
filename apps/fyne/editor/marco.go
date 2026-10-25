// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Marco es la rejilla de zonas del editor (`EditorLayout.tsx`): cabecera · cinta ·
// [franja de aviso] · [carril | panel | lienzo | ajustes] · barra de estado.
//
// Es la única pieza con estado propio: sabe qué zonas hay montadas y las recompone
// cuando el contexto cambia. Todo lo demás lee del contexto y se pinta.
type Marco struct {
	widget.BaseWidget

	ctx *Contexto

	cabecera *Cabecera
	cinta    *Cinta
	franja   *ui.FranjaSoloLectura
	carril   *Carril
	estado   *BarraEstado
	panel    *PanelLateral
	ajustes  *PanelLateral
	centro   fyne.CanvasObject
	// lienzo es el objeto del lienzo, pedido UNA vez: se conserva aunque el centro
	// enseñe un estado, para no perder su desplazamiento ni su selección.
	lienzo fyne.CanvasObject
	// zoomLienzo es el último zoom pintado: recomponer el marco a la misma medida no
	// vuelve a colocar su lienzo, y solo un cambio de zoom debe invalidarlo.
	zoomLienzo int
	// lienzoAtajo es la ventana donde ya está puesto Ctrl/Cmd+P.
	lienzoAtajo fyne.Canvas
	// buscarAbierto es si el panel «Buscar» estaba abierto en la última recomposición.
	buscarAbierto bool
}

// ResaltesDeBusqueda es el lienzo que pinta las coincidencias de «Buscar» y las quita
// al cerrarse el panel, como la web, que borra su pintura al desmontarlo. Al volver a
// abrirlo —con lo tecleado, que el panel conserva— las vuelve a pintar.
type ResaltesDeBusqueda interface {
	OcultarResaltes()
	MostrarResaltes()
}

// avisarBuscar avisa al lienzo cuando el panel «Buscar» se abre o se cierra.
func (m *Marco) avisarBuscar(abierto bool) {
	if abierto == m.buscarAbierto {
		return
	}
	m.buscarAbierto = abierto
	if l, ok := m.ctx.Lienzo.(ResaltesDeBusqueda); ok {
		if abierto {
			l.MostrarResaltes()
		} else {
			l.OcultarResaltes()
		}
	}
}

var _ fyne.Widget = (*Marco)(nil)

// NuevoMarco monta el editor entero. `alVolver` es el «Volver a tus documentos».
//
// El contexto se guarda por puntero y su `Refrescar` se cablea aquí: cualquier pieza
// que lo cambie repinta el marco, que es lo único que sabe recomponer las zonas.
func NuevoMarco(ctx *Contexto, r *Registro, alVolver func()) *Marco {
	m := &Marco{ctx: ctx, zoomLienzo: ctx.Zoom}
	m.ExtendBaseWidget(m)
	ctx.Refrescar = m.Recomponer
	ctx.Volver = alVolver
	m.cabecera = NuevaCabecera(ctx, alVolver)
	ctx.EnfocarTitulo = m.cabecera.EnfocarTitulo
	m.cinta = NuevaCinta(ctx, r)
	m.carril = NuevoCarril(ctx)
	// La barra se construye UNA vez y aquí.
	//
	// Estaba en `Objects()`, que Fyne llama en cada pintado Y en cada Layout: se
	// colocaba una instancia y se pintaba otra, así que la barra salía sin colocar
	// —en (0,0), encima de la cabecera—. Se vio en la captura, no en un test: los
	// guardianes de medida comprueban las bandas que sí se colocan.
	m.estado = NuevaBarraEstado(ctx)
	ctx.RefrescarEstado = m.estado.Refresh
	m.recomponer()
	return m
}

// AccessibilityRole lo anuncia como contenedor.
func (m *Marco) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Recomponer rehace las zonas que dependen del contexto y repinta.
//
// La cabecera y la barra de estado se construyen una vez y NO se rehacen, así que hay
// que refrescarlas aquí: sin esto, el chip de guardado se quedaba en «Guardado» con el
// documento sucio y la barra decía «Diapositiva 1» en la tercera, porque el `Refresh`
// del marco solo repinta el marco.
func (m *Marco) Recomponer() {
	m.recomponer()
	if m.lienzo != nil && m.zoomLienzo != m.ctx.Zoom {
		m.zoomLienzo = m.ctx.Zoom
		m.lienzo.Refresh()
	}
	m.cabecera.Sincronizar()
	m.cabecera.Refresh()
	m.estado.Refresh()
	m.Refresh()
}

func (m *Marco) recomponer() {
	c := m.ctx

	clave := c.MotivoSoloLectura
	if clave == "" {
		clave = "editor.readOnly"
	}
	m.franja = peticion.Franja(&c.Peticion, c.SoloLectura, c.Traducir(clave), func(k string, v i18n.Vars) string { return c.Traducir(k, v) })
	if m.franja == nil && !c.SoloLectura && c.BloqueoPropio != nil {
		if aviso, desbloquear := c.BloqueoPropio(); desbloquear != nil {
			m.franja = ui.NuevaFranjaSoloLectura(aviso).Con(c.Traducir("readOnly.edit"), desbloquear)
		}
	}

	anteriorPanel, anteriorAjustes := m.panel, m.ajustes
	m.panel = nil
	if c.Panel != SinPanel {
		for _, it := range itemsCarril {
			if it.p == c.Panel {
				m.panel = NuevoPanelLateral(c, c.Traducir(it.rotulo), cuerpoPanel(c, it.p), false)
				m.panel.conservarDesplazamiento(anteriorPanel)
			}
		}
	}

	m.avisarBuscar(c.Panel == PanelBuscar)

	m.ajustes = nil
	if c.Ajustes {
		m.ajustes = NuevoPanelLateral(c, c.Traducir("view.settingsPanel"), cuerpoAjustes(c), true)
		m.ajustes.conservarDesplazamiento(anteriorAjustes)
	}

	// El centro es el lienzo cuando el documento está listo, y el estado que toque en
	// los otros cinco casos. El lienzo NO se reconstruye al recomponer: se monta una
	// vez y guarda su propio desplazamiento y su selección.
	//
	// El lienzo se guarda aparte del centro: antes se comprobaba `m.centro == nil`, y
	// un documento que llegaba de la red pasaba por «cargando» primero, así que el
	// centro ya no era nil y se quedaba enseñando «Abriendo…» con el documento listo.
	if c.Estado == Listo && c.Lienzo != nil {
		if m.lienzo == nil {
			m.lienzo = c.Lienzo.Objeto()
		}
		m.centro = m.lienzo
	} else {
		m.centro = PantallaEstado(c)
	}
	m.cinta.Reconstruir()
}

// MinSize es lo que el editor necesita como mínimo: las bandas fijas más un hueco para
// el lienzo.
func (m *Marco) MinSize() fyne.Size {
	m.ExtendBaseWidget(m)
	alto := AltoCabecera + AltoCinta() + AltoEstado
	if m.franja != nil {
		alto += m.franja.MinSize().Height
	}
	return fyne.NewSize(anchoCarril()+AnchoPanel, alto+AltoCabeceraPanel)
}

// CreateRenderer compone el fondo y las zonas.
func (m *Marco) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &marcoRenderer{m: m, fondo: canvas.NewRectangle(tema.Color("surface", v))}
}

type marcoRenderer struct {
	m     *Marco
	fondo *canvas.Rectangle
}

func (r *marcoRenderer) MinSize() fyne.Size { return r.m.MinSize() }

func (r *marcoRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.fondo, r.m.cabecera, r.m.cinta}
	if r.m.franja != nil {
		objs = append(objs, r.m.franja)
	}
	objs = append(objs, r.m.carril)
	if r.m.panel != nil {
		objs = append(objs, r.m.panel)
	}
	if r.m.centro != nil {
		objs = append(objs, r.m.centro)
	}
	if r.m.ajustes != nil {
		objs = append(objs, r.m.ajustes)
	}
	return append(objs, r.m.estado)
}

func (r *marcoRenderer) Destroy() {}

// atajoImprimir es Ctrl/Cmd+P en la ventana del marco, para cuando el foco no lo tiene
// el lienzo (la cinta, un panel): se pone al colocarlo, que es cuando ya tiene ventana.
func (m *Marco) atajoImprimir() {
	c := fyne.CurrentApp().Driver().CanvasForObject(m)
	if c == nil || c == m.lienzoAtajo {
		return
	}
	m.lienzoAtajo = c
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyP, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) {
		if m.ctx.Imprimir != nil {
			m.ctx.Imprimir()
		}
	})
}

func (r *marcoRenderer) Layout(t fyne.Size) {
	r.m.atajoImprimir()
	m := r.m
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)

	banda := func(o fyne.CanvasObject, y, alto float32) float32 {
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, alto))
		return y + alto
	}
	y := banda(m.cabecera, 0, AltoCabecera)
	y = banda(m.cinta, y, m.cinta.Alto(t.Width))
	if m.franja != nil {
		y = banda(m.franja, y, m.franja.MinSize().Height)
	}

	altoCentro := max(t.Height-y-AltoEstado, 0)
	m.estado.Move(fyne.NewPos(0, t.Height-AltoEstado))
	m.estado.Resize(fyne.NewSize(t.Width, AltoEstado))

	x := float32(0)
	m.carril.Move(fyne.NewPos(x, y))
	m.carril.Resize(fyne.NewSize(anchoCarril(), altoCentro))
	x += anchoCarril()
	if r.colocarPanelesCompactos(t, x, y, altoCentro) {
		return
	}
	if m.panel != nil {
		m.panel.Move(fyne.NewPos(x, y))
		m.panel.Resize(fyne.NewSize(AnchoPanel, altoCentro))
		x += AnchoPanel
	}
	der := t.Width
	if m.ajustes != nil {
		der -= AnchoPanel
		m.ajustes.Move(fyne.NewPos(der, y))
		m.ajustes.Resize(fyne.NewSize(AnchoPanel, altoCentro))
	}
	if m.centro != nil {
		m.centro.Move(fyne.NewPos(x, y))
		m.centro.Resize(fyne.NewSize(max(der-x, 0), altoCentro))
	}
}

func (r *marcoRenderer) colocarPanelesCompactos(t fyne.Size, x, y, alto float32) bool {
	m := r.m
	// En una ventana estrecha los paneles sustituyen al lienzo. Al girar, el
	// mismo documento y sus controles vuelven al reparto de columnas si caben.
	paneles := float32(0)
	for _, p := range []*PanelLateral{m.panel, m.ajustes} {
		if p != nil {
			paneles += AnchoPanel
		}
	}
	compacto := paneles > 0 && (t.Width < ui.CorteMD || (ui.EsTactil() && t.Width-x-paneles < ui.AnchoContenidoMinimo))
	if m.centro != nil {
		if compacto {
			m.centro.Hide()
		} else {
			m.centro.Show()
		}
	}
	for _, p := range []*PanelLateral{m.panel, m.ajustes} {
		if p != nil {
			if compacto && p == m.panel && m.ajustes != nil {
				// No deja controles enfocables detrás del panel visible. Cerrar
				// ajustes devuelve el anterior sin borrar lo que estaba escrito.
				p.Hide()
			} else {
				p.Show()
			}
			if compacto {
				p.Move(fyne.NewPos(x, y))
				p.Resize(fyne.NewSize(max(t.Width-x, 0), alto))
			}
		}
	}
	return compacto
}

func (r *marcoRenderer) Refresh() {
	r.fondo.FillColor = tema.Color("surface", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.fondo.Refresh()
	// Recomponer cambia zonas (el panel, la franja, el centro): se colocan de nuevo, por
	// lo mismo que la banda de la cinta.
	r.Layout(r.m.Size())
	canvas.Refresh(r.m)
}

// ---- los seis estados ----

// PantallaEstado es lo que se pinta en el centro cuando el documento no está listo:
// los cinco estados que no son «éxito».
//
// Los seis van juntos y en un solo sitio a propósito: repartidos por cada pantalla es
// como se acaba con una que solo sabe pintar tres.
func PantallaEstado(c *Contexto) fyne.CanvasObject {
	switch c.Estado {
	case Cargando:
		return ui.NuevoVacio(c.Traducir("editor.opening"), "")
	case Vacio:
		return ui.NuevoVacio(c.Traducir("list.empty.title"), c.Traducir("list.empty.description"))
	case Error:
		return ui.NuevoVacio(c.Traducir("editor.error.title"), c.Traducir("editor.error.description"))
	case SinConexion:
		return ui.NuevoVacio(c.Traducir("list.offline.title"), c.Traducir("list.offline.description"))
	case SinPermiso:
		return ui.NuevoVacio(c.Traducir("forbidden.title"), c.Traducir("forbidden.description"))
	case SinSesion:
		return ui.NuevoVacio(c.Traducir("editor.error.title"), c.Traducir("pdf.error.session"))
	case Listo:
	}
	return ui.NuevoVacio(c.Traducir("list.empty.title"), c.Traducir("list.empty.description"))
}

// cuerpoPanel es el contenido de cada panel del carril.
//
// Buscar, Esquema y Comentarios son los tres del blueprint original; Revisión e IA
// enseñan por ahora su estado vacío con el motivo, que es honesto y útil: un panel que
// se abre y no dice nada parece roto.
//
// ponytail: los cinco paneles que faltan (revisiones, ajustes avanzados, datos
// externos, pasos de consulta y el de IA con su conversación) cuando el cliente de
// `apps/nucleo/api` tenga sus llamadas; hoy no hay de dónde sacar sus datos.
func cuerpoPanel(c *Contexto, p Panel) fyne.CanvasObject {
	if c.Paneles != nil {
		if o := c.Paneles(p); o != nil {
			return o
		}
	}
	var o fyne.CanvasObject
	switch p {
	case PanelBuscar:
		o = ui.NuevaEntrada(c.Traducir("find.placeholder"))
	case PanelEsquema:
		o = ui.NuevoVacio(c.Traducir("rail.outline.title"), c.Traducir("outline.empty"))
	case PanelComentarios:
		o = ui.NuevoVacio(c.Traducir("rail.comments.title"), c.Traducir("comments.empty"))
	case PanelRevision, PanelIA, SinPanel:
		o = ui.NuevoVacio(c.Traducir("panel.soon.titulo"), c.Traducir("panel.soon.texto"))
	}
	// `p-3`: el panel ya no rellena por su cuenta, lo hace cada cuerpo.
	return container.New(layout.NewCustomPaddedLayout(MargenPanel, MargenPanel, MargenPanel, MargenPanel), o)
}

// cuerpoAjustes es el panel derecho (§6.6): alineación y sangría del párrafo.
func cuerpoAjustes(c *Contexto) fyne.CanvasObject {
	f := c.formato()
	return container.New(layout.NewCustomPaddedLayout(MargenPanel, MargenPanel, MargenPanel, MargenPanel), NuevoGrupo(c.Traducir("settings.paragraph.heading"),
		c.boton(iconos.EdAlinearIzq, "align.left", f.Alineacion == Izquierda, false, AlinearIzq),
		c.boton(iconos.EdAlinearCentro, "align.center", f.Alineacion == Centro, false, AlinearCen),
		c.boton(iconos.EdAlinearDer, "align.right", f.Alineacion == Derecha, false, AlinearDer),
		c.boton(iconos.EdJustificar, "align.justify", f.Alineacion == Justificado, false, Justificar),
	))
}
