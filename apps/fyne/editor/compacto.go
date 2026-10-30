// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"strconv"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// ponerCompacto conserva una banda de comandos en el teléfono apaisado. Las otras
// tres filas consumían 120 px y Calcus dejaba de enseñar incluso una sola celda.
// La medida es la del área segura del canvas, antes de descontar la barra de suite.
func (m *Marco) ponerCompacto() {
	compacto := CompactoHorizontal(m)
	if compacto != m.compacto {
		menuCabeceraAbierto := m.cabecera.menu != nil && m.cabecera.menu.Abierto()
		m.cinta.cerrarMenusCompactos()
		if m.cabecera.menu != nil {
			m.cabecera.menu.Cerrar()
		}
		if m.renombrar != nil {
			m.renombrar.Cerrar()
			m.renombrar = nil
		}
		m.conservarFocoCompacto(compacto, menuCabeceraAbierto)
		m.compacto, m.cinta.compacta = compacto, compacto
		if _, propios := m.ctx.Lienzo.(ComandosCompactos); propios {
			m.cinta.reconstruir()
		}
	}
	m.cinta.archivo.antes = nil
	if compacto {
		m.cinta.archivo.antes = m.opcionesCompactas
		m.cabecera.Hide()
		m.estado.Hide()
	} else {
		m.cabecera.Show()
		m.estado.Show()
	}
}

// CompactoHorizontal comparte entre el cromo y el lienzo la decisión de conservar
// altura para editar en un teléfono apaisado. El teclado en vertical no la activa.
func CompactoHorizontal(o fyne.CanvasObject) bool {
	ventana, util := o.Size(), o.Size()
	if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil {
		ventana = c.Size()
		_, util = c.InteractiveArea()
	}
	return officeCompacto(ventana, util)
}

// El foco no puede quedarse en las filas retiradas ni en el selector al volver.
// Archivo existe en ambas disposiciones y conserva el acceso al documento.
func (m *Marco) conservarFocoCompacto(compacto, menuCabeceraAbierto bool) {
	c := fyne.CurrentApp().Driver().CanvasForObject(m)
	if c == nil {
		return
	}
	reubicar := func() {
		c.Unfocus()
		ui.Enfocar(c, m.cinta.archivo)
	}
	// La banda se reconstruye al cambiar de modo. Su foco no puede quedarse en
	// un mando retirado, incluidos los mandos propios de ComandosCompactos.
	if c.Focused() == nil || focoEnBanda(m.cinta.cuerpo, c.Focused()) {
		reubicar()
		return
	}
	if !compacto {
		if c.Focused() == m.cinta.selector {
			reubicar()
		}
		return
	}
	// Cerrar el menú ya retiró su foco y pudo restaurar nil. Conservar su
	// origen antes del cierre permite retirar también la cabecera en ese caso.
	if menuCabeceraAbierto || m.cabecera.titulo.Enfocado() {
		reubicar()
		return
	}
	ocultos := []fyne.Focusable{
		m.cabecera.volver, m.cabecera.titulo, m.cabecera.compartir, m.cabecera.descargar,
		m.estado.menos, m.estado.mas, m.estado.deslizador,
	}
	for _, p := range m.cinta.tiras {
		ocultos = append(ocultos, p)
	}
	for _, f := range ocultos {
		if c.Focused() == f {
			reubicar()
			return
		}
	}
}

func focoEnBanda(o fyne.CanvasObject, foco fyne.Focusable) bool {
	if o == nil || foco == nil {
		return false
	}
	if f, ok := o.(fyne.Focusable); ok && f == foco {
		return true
	}
	var hijos []fyne.CanvasObject
	switch x := o.(type) {
	case *fyne.Container:
		hijos = x.Objects
	case *Grupo:
		hijos = x.Controles
	}
	for _, h := range hijos {
		if focoEnBanda(h, foco) {
			return true
		}
	}
	return false
}

func officeCompacto(ventana, util fyne.Size) bool {
	// Un teclado en vertical puede dejar el área útil más ancha que alta.
	// La orientación la dan los límites reales de la ventana, no los del teclado.
	return ui.EsTactil() && ventana.Width > ventana.Height && util.Height > 0 && util.Height < 400
}

// opcionesCompactas añade al mismo Archivo las acciones de las filas retiradas.
// Los callbacks y sus permisos siguen siendo los de la cabecera y el contexto.
func (m *Marco) opcionesCompactas() []fyne.CanvasObject {
	c := m.ctx
	item := func(clave string, inerte bool, hacer func()) *ui.ElementoMenu {
		e := ui.NuevoElementoMenu(c.Traducir(clave), iconos.Ninguno, hacer)
		e.Inerte = inerte
		return e
	}
	guardado, _ := m.cabecera.textoChip(time.Now())
	rotulo := ui.NuevoElementoMenu(c.Titulo+" · "+guardado, iconos.Ninguno, nil)
	rotulo.Inerte = true
	compartir := item("header.share", c.SinRed || c.Compartir == nil, func() {
		if c.Compartir != nil {
			c.Compartir(m.cinta.archivo)
		}
	})
	formato := c.formato()
	zoom := ui.NuevoElementoMenu(strconv.Itoa(c.Zoom)+" %", iconos.Ninguno, nil)
	zoom.Inerte = true
	return []fyne.CanvasObject{
		rotulo, compartir,
		item("action.undo", c.SoloLectura || !formato.PuedeDeshacer, func() { c.Mandar(Deshacer, "") }),
		item("action.redo", c.SoloLectura || !formato.PuedeRehacer, func() { c.Mandar(Rehacer, "") }),
		ui.NuevoSeparadorMenu(), zoom,
		item("status.zoom.out", c.Zoom <= ZoomMin, m.estado.menos.AlPulsar),
		item("status.zoom.in", c.Zoom >= ZoomMax, m.estado.mas.AlPulsar),
		ui.NuevoSeparadorMenu(),
	}
}

func (m *Marco) renombrarCompacto() {
	c := fyne.CurrentApp().Driver().CanvasForObject(m)
	if c == nil || m.ctx.AlRenombrar == nil {
		return
	}
	campo := ui.NuevaEntrada(m.ctx.Traducir("header.title.label"))
	campo.PonerTexto(m.ctx.Titulo)
	// Cambiar el título recorre el mismo callback que el campo de Cabecera.
	campo.AlCambiar(m.cabecera.titulo.PonerTexto)
	d := ui.NuevoDialogo(m.ctx.Traducir("file.rename"), campo)
	d.RotuloCerrar, d.Foco = m.ctx.Traducir("ui.close"), campo
	m.renombrar = d
	d.Mostrar(c)
}

func (c *Cinta) cerrarMenusCompactos() {
	if c.menuPestanas != nil {
		c.menuPestanas.Cerrar()
		c.menuPestanas = nil
	}
	if c.archivo.menu != nil {
		c.archivo.menu.Cerrar()
	}
	c.plegarMas()
}

// abrirPestanas selecciona la pestaña sin reservar otra fila ni expandir comandos.
func (c *Cinta) abrirPestanas() {
	if c.menuPestanas != nil && c.menuPestanas.Abierto() {
		c.menuPestanas.Cerrar()
		return
	}
	visibles := c.registro.Visibles(c.ctx.Tipo)
	elementos := make([]fyne.CanvasObject, 0, len(visibles))
	for i, p := range visibles {
		e := ui.NuevoElementoMenu(c.ctx.Traducir(p.Rotulo), iconos.Ninguno, func() { c.Elegir(i) })
		e.Marcada = i == c.activa
		elementos = append(elementos, e)
	}
	c.menuPestanas = ui.NuevoMenu(elementos...)
	c.menuPestanas.Rotulo = c.ctx.Traducir("ribbon.label")
	c.selector.mostrarMenu(c.menuPestanas)
}

func (r *cintaRenderer) colocarCompacta(t fyne.Size) {
	c := r.c
	c.tira.Hide()
	c.selector.Show()
	c.sepMas.Hide()
	c.mas.Hide()
	r.lineaT.Hide()
	r.fondo.Resize(t)
	r.lineaB.Show()
	r.lineaB.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.lineaB.Resize(fyne.NewSize(t.Width, tema.Borde))
	alto := t.Height - tema.Borde
	archivo := c.archivo.MinSize()
	// El margen original se calculó para la tira de 44; centrar el botón en 53.
	c.envolturaArchivo.Layout = ui.Margen{Arr: max((alto-archivo.Height)/2, 0), Abj: max((alto-archivo.Height)/2, 0)}
	c.envolturaArchivo.Move(fyne.NewPos(MargenTira, 0))
	c.envolturaArchivo.Resize(fyne.NewSize(archivo.Width, alto))
	x := MargenTira + archivo.Width + HuecoTira
	if a := c.activaTira(); a != nil {
		c.selector.Texto = a.Rotulo
	}
	c.selector.Refresh()
	ancho := max(ui.SueloTactilBarra, c.selector.MinSize().Width)
	c.selector.Move(fyne.NewPos(x, (alto-ui.SueloTactilBarra)/2))
	c.selector.Resize(fyne.NewSize(ancho, ui.SueloTactilBarra))
	x += ancho + HuecoTira
	if c.cuerpo != nil {
		c.banda.Show()
		ponerEstrecha(c.cuerpo, true)
		c.banda.Move(fyne.NewPos(x, 0))
		c.banda.Resize(fyne.NewSize(max(t.Width-x, 0), alto))
	} else {
		c.banda.Hide()
	}
}

// Evitar que el contenedor Archivo conserve el relleno del modo apaisado al volver.
func (c *Cinta) restaurarArchivo() {
	alto := AltoDeTira() - tema.Borde
	margen := max((alto-c.archivo.MinSize().Height)/2, 0)
	c.envolturaArchivo.Layout = ui.Margen{Arr: margen, Abj: margen}
	c.envolturaArchivo.Refresh()
}
