// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/avisos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// # El Hub residente (HU-03 y HU-06)
//
// Como el Hub conectado (`cmd/hub`), el de la edición se queda en la bandeja del sistema
// al cerrar su ventana —la barra de menús en macOS, el área de notificación en Windows,
// el StatusNotifierItem en Linux— y puede arrancar solo al iniciar sesión, en la bandeja
// y sin ventana («Abrir al iniciar sesión», apagado de serie: lo enciende la persona).
// Sin nadie que pinte la bandeja (un Linux sin StatusNotifierItem) no se esconde: se
// perdería la app.
//
// La bandeja enseña lo de hoy (los eventos con su hora, el correo sin leer) y abre cada
// app de la edición. En Linux, además, el Hub residente avisa los recordatorios de
// Calendar y Notepads cuando su app está cerrada (§5.12): ahí el sistema no guarda
// avisos programados, y cada app los avisa solo mientras está abierta.

// MaxHoyBandeja son las líneas de hoy que caben en el menú del sistema.
const MaxHoyBandeja = 5

// LargoBandeja es lo más largo de una línea del menú: un título entero lo ensancha.
const LargoBandeja = 48

func recortarRotulo(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > LargoBandeja {
		return strings.TrimSpace(string(r[:LargoBandeja-1])) + "…"
	}
	return s
}

// AccionesBandeja es lo que hacen las entradas del menú.
type AccionesBandeja struct {
	// Mostrar trae la ventana del Hub.
	Mostrar func()
	// Abrir abre una línea de hoy en su app; Lanzar, una app en su portada.
	Abrir  func(indice.Elemento)
	Lanzar func(Instalada)
}

// MenuBandeja es el menú de la bandeja: «Abrir Sytue Office», lo de hoy y las apps.
// Quien lo monta le añade detrás «Abrir al iniciar sesión»; «Salir» lo pone Fyne.
func MenuBandeja(l i18n.Locale, nombre string, h indice.Hoy, apps []Instalada, a AccionesBandeja) *fyne.Menu {
	tx := func(clave string, vars ...string) string { return texto(l, clave, vars...) }
	items := []*fyne.MenuItem{fyne.NewMenuItem(tx("bandeja.abrir", "nombre", nombre), func() {
		if a.Mostrar != nil {
			a.Mostrar()
		}
	})}

	var hoy []*fyne.MenuItem
	for _, f := range FilasHoy(l, h) {
		if len(hoy) >= MaxHoyBandeja {
			break
		}
		e := f.Elemento
		rotulo := recortarRotulo(cmpOr(e.Titulo, "—"))
		if f.Cuando != "" && e.Producto != "mail" {
			rotulo = f.Cuando + "  " + rotulo
		}
		hoy = append(hoy, fyne.NewMenuItem(rotulo, func() {
			if a.Abrir != nil {
				a.Abrir(e)
			}
		}))
	}
	if len(hoy) > 0 {
		items = append(items, fyne.NewMenuItemSeparator())
		titulo := fyne.NewMenuItem(tx("hoy"), nil)
		titulo.Disabled = true
		items = append(items, titulo)
		items = append(items, hoy...)
	}

	if len(apps) > 0 {
		sub := make([]*fyne.MenuItem, 0, len(apps))
		for _, i := range apps {
			sub = append(sub, fyne.NewMenuItem(i.Marca.Nombre(), func() {
				if a.Lanzar != nil {
					a.Lanzar(i)
				}
			}))
		}
		r := fyne.NewMenuItem(tx("bandeja.apps"), nil)
		r.ChildMenu = fyne.NewMenu("", sub...)
		items = append(items, fyne.NewMenuItemSeparator(), r)
	}
	return fyne.NewMenu(nombre, items...)
}

// alIniciarSesion es la casilla «Abrir al iniciar sesión»: arranca el Hub en la bandeja
// (`escritorio.ArgBandeja`) al entrar en la sesión del sistema.
func alIniciarSesion(l i18n.Locale, m fyne.AppMetadata, menu *fyne.Menu) *fyne.MenuItem {
	item := fyne.NewMenuItem(texto(l, "bandeja.alIniciar"), nil)
	item.Checked = escritorio.ArrancaAlIniciarSesion(m.ID)
	item.Action = func() {
		exe, err := os.Executable()
		if err == nil {
			err = escritorio.AlIniciarSesion(m.ID, m.Name, exe, !item.Checked)
		}
		if err != nil {
			slog.Warn("hub: no se ha podido cambiar el arranque al iniciar sesión", "error", err)
			return
		}
		item.Checked = !item.Checked
		menu.Refresh()
	}
	return item
}

// residente deja el Hub en la bandeja con su menú, que se rehace cada vez que cambia lo
// de hoy. Dice si lo ha hecho.
func residente(a fyne.App, w fyne.Window, p *Portada, nombre string, acciones AccionesBandeja) bool {
	bandeja, ok := a.(desktop.App)
	if !ok || a.Driver().Device().IsMobile() || !escritorio.HayBandeja() {
		return false
	}
	l := p.T.Locale()
	menu := fyne.NewMenu(nombre)
	var delSistema []*fyne.MenuItem
	// Ni en la variante de la Mac App Store: el agente de inicio se escribiría dentro del
	// contenedor del sandbox, donde launchd no lo lee.
	if m := a.Metadata(); m.ID != "" && !escritorio.DeTienda {
		delSistema = append(delSistema, fyne.NewMenuItemSeparator(), alIniciarSesion(l, m, menu))
	}
	montar := func(h indice.Hoy) {
		hecho := MenuBandeja(l, nombre, h, p.Apps, acciones)
		menu.Items = slices.Concat(hecho.Items, delSistema)
		menu.Refresh()
	}
	montar(p.Hoy())
	anterior := p.AlCambiarHoy
	p.AlCambiarHoy = func(h indice.Hoy) {
		if anterior != nil {
			anterior(h)
		}
		montar(h)
	}
	bandeja.SetSystemTrayMenu(menu)
	w.SetCloseIntercept(w.Hide)
	return true
}

// cadaHoy es cada cuánto se relee lo de hoy con el Hub en la bandeja.
const cadaHoy = 15 * time.Minute

// recargarCada relee la portada (y con ella la bandeja) cada cadaHoy, para siempre.
func recargarCada(p *Portada) {
	for {
		time.Sleep(cadaHoy)
		p.Recargar()
	}
}

// ----- Los recordatorios con las apps cerradas (HU-06) -----

// cadaAvisos es cada cuánto se vuelven a leer los recordatorios: los de las apps se
// reprograman cada cuarto de hora; aquí, algo antes, para que un cambio hecho en la app
// justo antes de cerrarla llegue a tiempo.
const cadaAvisos = 5 * time.Minute

// Vigia avisa los recordatorios de Calendar y Notepads cuando su app está cerrada.
type Vigia struct {
	Datos string
	// Abierta dice si la app de ese producto está corriendo (entonces avisa ella).
	Abierta func(producto string) bool
	// Avisar enseña el aviso de la app de ese producto.
	Avisar func(producto string, a avisos.Aviso)
	Ahora  func() time.Time

	avisos *avisos.Avisos
}

// productoDeEnlace es el producto de un `sytueoffice-<producto>:…`.
func productoDeEnlace(enlace string) string {
	esquema, _, _ := strings.Cut(enlace, ":")
	return strings.TrimPrefix(esquema, espacio.PrefijoEsquema)
}

// avisado es lo que decide el vigía al llegar la hora de un aviso: avisarlo él, o no
// porque su app ya está abierta y lo avisa ella.
func (v *Vigia) avisado(a avisos.Aviso) {
	p := productoDeEnlace(a.Enlace)
	if v.Abierta != nil && v.Abierta(p) {
		return
	}
	if v.Avisar != nil {
		v.Avisar(p, a)
	}
}

// Reprogramar lee los recordatorios y los deja en relojes del proceso.
func (v *Vigia) Reprogramar() {
	if v.avisos == nil {
		v.avisos = avisos.Abrir("", &avisos.Temporizador{Avisar: v.avisado})
	}
	ahora := time.Now
	if v.Ahora != nil {
		ahora = v.Ahora
	}
	por, fallos := indice.Avisos(v.Datos, ahora())
	for fuente, err := range fallos {
		slog.Warn("hub: no se han podido leer los recordatorios", "fuente", fuente, "error", err)
	}
	for _, origen := range []string{"calendar", "notes"} {
		if _, ok := por[origen]; !ok {
			continue
		}
		if err := v.avisos.Reprogramar(origen, por[origen], ahora()); err != nil {
			slog.Warn("hub: no se han podido programar los recordatorios", "origen", origen, "error", err)
		}
	}
}

// Programados son los avisos en marcha de un origen.
func (v *Vigia) Programados(origen string) []avisos.Aviso {
	if v.avisos == nil {
		return nil
	}
	return v.avisos.Programados(origen)
}

// Vigilar reprograma ahora y cada cadaAvisos, para siempre.
func (v *Vigia) Vigilar() {
	go func() {
		for {
			v.Reprogramar()
			time.Sleep(cadaAvisos)
		}
	}()
}

// avisarComo enseña el aviso con el nombre de la app dueña; pulsarlo la abre en lo
// avisado (o, sin ella instalada, trae el Hub).
func avisarComo(a fyne.App, p *Portada, mostrar func()) func(string, avisos.Aviso) {
	return func(producto string, av avisos.Aviso) {
		nombre := a.Metadata().Name
		if i, ok := p.instalada(producto); ok {
			nombre = i.Marca.Nombre()
		}
		err := escritorio.Avisar(escritorio.Aviso{
			ID: a.Metadata().ID, Nombre: nombre, Esquema: espacio.Esquema("one"),
			Titulo: av.Titulo, Cuerpo: av.Cuerpo,
			AlPulsar: func() {
				fyne.Do(func() {
					if i, ok := p.instalada(producto); ok {
						p.abrirApp(i, av.Enlace)
						return
					}
					mostrar()
				})
			},
		})
		if err != nil {
			slog.Warn("hub: no se ha podido avisar", "producto", producto, "error", err)
		}
	}
}
