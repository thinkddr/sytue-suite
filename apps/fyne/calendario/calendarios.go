// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"bytes"
	"context"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los calendarios: el menú «⋮» de cada uno en la barra lateral, CalendarModal.tsx
// (crear, renombrar, cambiar el color), DeleteCalendarModal y SubscribeModal.tsx
// («Añadir calendario por URL»), con los mismos avisos de la web.

// menuCalendario es el `<Dropdown>` del kebab de un calendario propio o suscrito.
func (a *App) menuCalendario(c api.Calendar, disparador fyne.CanvasObject) {
	p := a.P
	var m *ui.Menu
	elemento := func(clave string, alPulsar func()) *ui.ElementoMenu {
		return ui.NuevoElementoMenu(p.t(clave, nil), iconos.Ninguno, func() { m.Cerrar(); alPulsar() })
	}
	var elems []fyne.CanvasObject
	if c.ReadOnly {
		elems = append(elems, elemento("cal.refresh", func() { a.refrescar(c) }), ui.NuevoSeparadorMenu())
	}
	elems = append(elems,
		elemento("nav.rename", func() { a.formularioCalendario(&c) }),
		elemento("nav.changeColour", func() { a.formularioCalendario(&c) }))
	// Compartir con ACL: solo calendarios propios editables, no las suscripciones.
	if !c.ReadOnly {
		elems = append(elems, elemento("cal.share", func() { a.compartir(c) }))
	}
	if c.ID != "" {
		elems = append(elems, elemento("cal.downloadIcs", func() { a.descargarICS(c) }))
	}
	if !c.IsDefault {
		clave := "common.delete"
		if c.ReadOnly {
			clave = "cal.sub.delete"
		}
		borrar := elemento(clave, func() { a.confirmarBorrarCalendario(c) })
		borrar.Peligro = true
		elems = append(elems, ui.NuevoSeparadorMenu(), borrar)
	}
	m = ui.NuevoMenu(elems...)
	m.Rotulo = p.t("nav.calendarOptions", i18n.Vars{"name": c.Title()})
	m.Mostrar(disparador)
}

// refrescar es `refreshNow`: vuelve a leer ahora el .ics de una suscripción.
func (a *App) refrescar(c api.Calendar) {
	if a.P.SinConexion || c.ID == "" {
		return
	}
	a.escribir(func(ctx context.Context) error { return a.Cal.RefreshSubscription(ctx, c.ID) }, func(err error) {
		if err != nil {
			a.avisar(ui.AvisoPeligro, "cal.error.updateSub")
			return
		}
		a.avisar(ui.AvisoExito, "cal.sub.updated")
		a.RecargarCalendarios()
		a.Recargar()
	})
}

// descargarICS deja el calendario entero en la carpeta de descargas, con el nombre que
// pone el servicio en su Content-Disposition (`<nombre>.ics`).
//
// ponytail: sin aviso al terminar, como en la web, donde la descarga la enseña el
// navegador y no la app.
func (a *App) descargarICS(c api.Calendar) {
	nombre := strings.ReplaceAll(c.Title(), `"`, "'") + ".ics"
	a.escribir(func(ctx context.Context) error {
		ics, err := a.Cal.ExportICS(ctx, c.ID)
		if err != nil {
			return err
		}
		carpeta, err := ficheros.Carpeta()
		if err != nil {
			return err
		}
		_, err = ficheros.Guardar(carpeta, nombre, bytes.NewReader(ics))
		return err
	}, func(err error) {
		if err != nil {
			a.avisar(ui.AvisoPeligro, "toast.calendarSaveFailed")
		}
	})
}

// formularioCalendario es CalendarModal: nuevo (`c` nil) o renombrar y cambiar el color.
func (a *App) formularioCalendario(c *api.Calendar) {
	lienzo := a.lienzo()
	if lienzo == nil {
		return
	}
	p := a.P
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	editar := c != nil && c.ID != ""
	color := PaletaCalendario[0].Hex
	nombre := ui.NuevaEntrada(p.t("calendar.namePlaceholder", nil))
	if editar {
		nombre.PonerTexto(c.Title())
		if c.Color != "" {
			color = c.Color
		}
	}
	gNombre := nuevoGrupoVivo(p.t("calendar.name", nil), true, nombre, p.t("calendar.nameHint", nil))
	muestras := nuevasMuestras(p, color, false, func(hex string) { color = hex })
	var filas []fyne.CanvasObject
	if !editar {
		filas = append(filas, ui.NuevaAyudaEnLinea(p.t("calendar.helpBody", nil)).ConTono(ui.AyudaInfo).ConTitulo(p.t("calendar.helpTitle", nil)))
	}
	pista := ui.NuevoParrafo(p.t("calendar.colourHint", nil), tema.TextoXS, ui.Normal, "text-muted", v)
	filas = append(filas, gNombre.c, container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
		alto(rotuloFuerte(p.t("common.colour", nil)), tema.TextoSM.Linea), muestras, pista))

	titulo := "calendar.newTitle"
	if editar {
		titulo = "calendar.editTitle"
	}
	var d *ui.Dialogo
	var guardar *ui.Boton
	enviar := func() {
		n := strings.TrimSpace(nombre.Texto())
		if n == "" {
			nombre.Invalida = true
			gNombre.poner(p.t("calendar.nameRequired", nil))
			lienzo.Focus(nombre)
			return
		}
		guardar.Cargando = true
		guardar.Refresh()
		in := api.CalendarInput{DisplayName: n, Color: color}
		a.escribir(func(ctx context.Context) error {
			var err error
			if editar {
				_, err = a.Cal.UpdateCalendar(ctx, c.ID, in)
			} else {
				_, err = a.Cal.CreateCalendar(ctx, in)
			}
			return err
		}, func(err error) {
			guardar.Cargando = false
			guardar.Refresh()
			if err != nil {
				a.avisar(ui.AvisoPeligro, "toast.calendarSaveFailed")
				return
			}
			if editar {
				a.avisar(ui.AvisoExito, "toast.calendarUpdated")
			} else {
				a.avisar(ui.AvisoExito, "toast.calendarCreated")
			}
			d.Cerrar()
			a.RecargarCalendarios()
		})
	}
	nombre.AlCambiar(func(string) {
		if gNombre.err != "" {
			nombre.Invalida = false
			gNombre.poner("")
		}
	})
	nombre.AlTeclear = intro(enviar)
	guardar = ui.NuevoBoton(p.t("common.save", nil), enviar).Con(ui.Primario, ui.TallaMD)
	cancelar := ui.NuevoBoton(p.t("common.cancel", nil), func() { d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	d = ui.NuevoDialogo(p.t(titulo, nil), container.New(ui.Columna{Hueco: tema.Espacio(4)}, filas...)).
		Con(ui.DialogoSM, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, guardar))
	d.RotuloCerrar = p.t("common.close", nil)
	d.Foco = nombre
	d.Mostrar(lienzo)
}

// intro es el `onKeyDown` de Intro que envía el formulario.
func intro(f func()) func(*fyne.KeyEvent) bool {
	return func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			f()
			return true
		}
		return false
	}
}

// confirmarBorrarCalendario es DeleteCalendarModal: la frase completa con el nombre
// dentro —en alemán las comillas son „…“ y el verbo va al final—, y «Eliminar».
func (a *App) confirmarBorrarCalendario(c api.Calendar) {
	lienzo := a.lienzo()
	if lienzo == nil {
		return
	}
	p := a.P
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(p.t("calendar.deleteBody", i18n.Vars{"name": c.Title()}), tema.TextoBase, ui.Normal, "text", v)
	var d *ui.Dialogo
	var borrar *ui.Boton
	borrar = ui.NuevoBoton(p.t("common.delete", nil), func() {
		borrar.Cargando = true
		borrar.Refresh()
		a.escribir(func(ctx context.Context) error { return a.Cal.DeleteCalendar(ctx, c.ID) }, func(err error) {
			borrar.Cargando = false
			borrar.Refresh()
			if err != nil {
				a.avisar(ui.AvisoPeligro, "toast.calendarDeleteFailed")
				return
			}
			a.avisar(ui.AvisoExito, "toast.calendarDeleted")
			d.Cerrar()
			a.RecargarCalendarios()
		})
	}).Con(ui.Peligro, ui.TallaMD)
	cancelar := ui.NuevoBoton(p.t("common.cancel", nil), func() { d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	d = ui.NuevoDialogo(p.t("calendar.deleteTitle", nil), cuerpo).
		Con(ui.DialogoSM, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, borrar))
	d.RotuloCerrar = p.t("common.close", nil)
	d.Mostrar(lienzo)
}

// pareceURL es `looksLikeUrl`: http(s) con host. El servicio valida de verdad.
var pareceURL = regexp.MustCompile(`(?i)^https?://[^\s/]+`)

// formularioSuscripcion es SubscribeModal: un feed ICS externo como calendario de solo
// lectura, con Ciruela (el color de Calendar) por defecto. Bajo la frase, el
// `<DataFlow compact incoming={['events']}>`: se lee el .ics de esa dirección y de aquí
// no sale nada hacia ella.
func (a *App) formularioSuscripcion() {
	lienzo := a.lienzo()
	if lienzo == nil || a.P.SinConexion {
		return
	}
	p := a.P
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	color := PaletaCalendario[7].Hex
	enlace := ui.NuevaEntrada(p.t("subscribe.urlPlaceholder", nil))
	gEnlace := nuevoGrupoVivo(p.t("subscribe.url", nil), true, enlace, "")
	nombre := ui.NuevaEntrada(p.t("subscribe.namePlaceholder", nil))
	muestras := nuevasMuestras(p, color, false, func(hex string) { color = hex })
	filas := []fyne.CanvasObject{
		ui.NuevoParrafo(p.t("subscribe.intro", nil), tema.TextoSM, ui.Normal, "text-muted", v),
		ui.FlujoDatos(p.T, "", []string{"events"}, nil, true, a.abrirPolitica, ui.AnchoCuerpoTalla(ui.DialogoSM, lienzo.Size())),
		gEnlace.c, ui.GrupoCampo(p.t("subscribe.name", nil), false, nombre, "", ""),
		container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, alto(rotuloFuerte(p.t("common.colour", nil)), tema.TextoSM.Linea), muestras),
	}
	var d *ui.Dialogo
	var enviarB *ui.Boton
	enviar := func() {
		u := strings.TrimSpace(enlace.Texto())
		if !pareceURL.MatchString(u) {
			enlace.Invalida = true
			gEnlace.poner(p.t("subscribe.urlInvalid", nil))
			lienzo.Focus(enlace)
			return
		}
		n := strings.TrimSpace(nombre.Texto())
		if n == "" {
			n = p.t("subscribe.defaultName", nil)
		}
		enviarB.Cargando = true
		enviarB.Refresh()
		a.escribir(func(ctx context.Context) error {
			_, err := a.Cal.Subscribe(ctx, api.SubscriptionInput{URL: u, DisplayName: n, Color: color})
			return err
		}, func(err error) {
			enviarB.Cargando = false
			enviarB.Refresh()
			if err != nil {
				a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, p.t("toast.subscribeFailed", nil), p.t("toast.subscribeFailedHint", nil)))
				return
			}
			a.avisar(ui.AvisoExito, "toast.calendarSubscribed")
			d.Cerrar()
			a.RecargarCalendarios()
		})
	}
	enlace.AlCambiar(func(string) {
		if gEnlace.err != "" {
			enlace.Invalida = false
			gEnlace.poner("")
		}
	})
	nombre.AlTeclear = intro(enviar)
	enviarB = ui.NuevoBoton(p.t("subscribe.submit", nil), enviar).Con(ui.Primario, ui.TallaMD)
	cancelar := ui.NuevoBoton(p.t("common.cancel", nil), func() { d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	d = ui.NuevoDialogo(p.t("subscribe.title", nil), container.New(ui.Columna{Hueco: tema.Espacio(4)}, filas...)).
		Con(ui.DialogoSM, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, enviarB))
	d.RotuloCerrar = p.t("common.close", nil)
	d.Foco = enlace
	d.Mostrar(lienzo)
}

// grupoVivo es un `ui.GrupoCampo` que se rehace al cambiarle el error: la pieza del
// catálogo es de una sola vez, y un formulario tiene que poder decir «falta el nombre»
// junto al campo y quitarlo en cuanto se escribe.
type grupoVivo struct {
	c                  *fyne.Container
	rotulo, ayuda, err string
	obligatorio        bool
	control            fyne.CanvasObject
}

func nuevoGrupoVivo(rotulo string, obligatorio bool, control fyne.CanvasObject, ayuda string) *grupoVivo {
	g := &grupoVivo{rotulo: rotulo, obligatorio: obligatorio, control: control, ayuda: ayuda}
	// Una `Columna` y no un `Stack`: la columna pasa el ancho a sus hijos al medir
	// (`ui.AltoA`), y la ayuda parte de línea con él.
	g.c = container.New(ui.Columna{}, g.armar())
	return g
}

// armar es el grupo con la ayuda como `FieldHint`, que en la web parte de línea: la de
// `ui.GrupoCampo` es de una, y «Es como lo verás en tu lista de calendarios. Podrás
// cambiarlo después.» se salía 8 px de la hoja de 390.
func (g *grupoVivo) armar() *fyne.Container {
	c := ui.GrupoCampo(g.rotulo, g.obligatorio, g.control, "", g.err)
	if g.err == "" && g.ayuda != "" {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		snug := tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.375 * tema.TextoXS.Tam} // `text-xs leading-snug`
		c.Add(ui.NuevoParrafo(g.ayuda, snug, ui.Normal, "text-muted", v))
	}
	return c
}

// poner cambia el error (vacío lo quita) y hace que el diálogo vuelva a medirse.
func (g *grupoVivo) poner(err string) {
	g.err = err
	g.c.Objects = []fyne.CanvasObject{g.armar()}
	g.c.Refresh()
	if c := fyne.CurrentApp().Driver().CanvasForObject(g.c); c != nil {
		if arriba := c.Overlays().Top(); arriba != nil {
			arriba.Refresh()
		}
	}
}

// abrirPolitica es el enlace del `DataFlow`: la política de privacidad del sitio público
// de la suite, en el idioma de la app. La comparten «Suscribirse» y «Trae tus
// calendarios».
func (a *App) abrirPolitica() {
	if a.AbrirEnlace != nil {
		a.AbrirEnlace(ui.PoliticaDePrivacidad(a.apex(), a.P.locale()))
	}
}

// apex es el dominio de la suite; vacío, el de producción.
func (a *App) apex() string {
	if a.Apex == "" {
		return api.ApexProd
	}
	return a.Apex
}
