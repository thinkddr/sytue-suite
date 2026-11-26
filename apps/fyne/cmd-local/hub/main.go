// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command hub es el Hub de Sytue Office («Sytue Office»), la edición local de la suite
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.15): el lanzador de las demás apps de la edición,
// lo de hoy y lo reciente de todas y la búsqueda entre ellas, sin cuenta, sin cifrado y
// sin red.
//
// Comparado con `cmd/hub`: no hay sesión, ni avisos del servidor, ni actividad,
// dispositivos, descargas o «Tus datos» (HU-08). Lo que hace:
//
//   - HU-01: una tesela por cada app de la edición instalada JUNTO al Hub (su
//     `sytue-office-<app>` en la misma carpeta, o su `.app` en macOS); pulsarla la
//     lanza. No hay caída a la web.
//   - HU-03: se queda en la bandeja del sistema al cerrar la ventana, con lo de hoy y
//     las apps, y puede abrirse al iniciar sesión (residente.go). Abrirlo otra vez trae
//     la copia viva (la copia única de la cáscara, `local/instancia.go`).
//   - HU-04: «Hoy», con los eventos de Calendar, los recordatorios y tareas de Notepads
//     y el correo sin leer de Mail (hoy.go).
//   - HU-05: lo reciente de Docus, Calcus, Diapos, Notepads y Drive, y la búsqueda en
//     el contenido de los documentos, el correo, las notas, los contactos, los eventos,
//     las fotos, las bases y los ficheros, leyendo los almacenes de cada app en solo
//     lectura (`nucleo/local/indice`). Un resultado se abre en su app con
//     `sytueoffice-<producto>:/abrir?…`.
//   - HU-06: en Linux, donde el sistema no guarda avisos programados, avisa los
//     recordatorios de Calendar y Notepads cuando su app está cerrada (residente.go).
//   - HU-07: «Traer desde Sytue» importa el ZIP de «Tus datos» de la edición conectada
//     (traer.go, `nucleo/local/traer`).
//   - «Exportar todo»: lo de todas las apps en formatos abiertos, de una vez y con ellas
//     cerradas, con las mismas funciones de cada una (exportar.go, `nucleo/local/exportar`).
//   - HU-02: los Ajustes de la edición, los mismos de todas las apps (`local`).
//
// En el móvil no hay Hub: cada app va por separado.
package main

import (
	"log/slog"
	"os"
	"runtime"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

func main() {
	a := local.NuevaApp("")
	movil := local.Tactil()
	// El Hub vive en la bandeja: abrirlo otra vez trae la ventana de la copia viva (la
	// copia única de la cáscara, en local.Abrir).
	l, err := local.Abrir(a, "one")
	if err != nil {
		slog.Error("hub", "error", err)
		os.Exit(1)
	}
	// Solo el catálogo común: las descripciones de las apps. Lo demás es propio.
	t, err := l.Traductor()
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	nombre := texto(t.Locale(), "nombre")
	if m := a.Metadata(); m.Name != "" {
		nombre = m.Name
	}
	w := a.NewWindow(nombre)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))

	var apps []Instalada
	if !movil {
		exe, _ := os.Executable()
		apps = Instaladas(exe, runtime.GOOS)
	}
	p := NuevaPortada(t, l.Espacio.Rutas.Datos, apps)
	p.Movil = movil
	p.Lanzar = func(i Instalada, enlace string) error { return Lanzar(i, enlace, runtime.GOOS) }
	p.Aviso = func(titulo, cuerpo string) { local.Info(w, titulo, cuerpo) }
	traedor := NuevoTraedor(l, w, p)
	exportador := NuevoExportador(l, w)

	mostrar := func() {
		w.Show()
		w.RequestFocus()
	}
	w.SetContent(barra(l, w, nombre, p, traedor, exportador))
	w.SetOnClosed(l.Cerrar)

	enBandeja := !movil && residente(a, w, p, nombre, AccionesBandeja{
		Mostrar: mostrar,
		Abrir:   p.AbrirHoy,
		Lanzar:  func(i Instalada) { p.abrirApp(i, "") },
	})
	// HU-06: en Linux nadie guarda los avisos programados de las apps cerradas.
	if runtime.GOOS == "linux" {
		v := &Vigia{Datos: l.Espacio.Rutas.Datos, Abierta: abierta, Avisar: avisarComo(a, p, mostrar)}
		v.Vigilar()
	}

	// Lo de hoy y lo reciente cambian mientras se usan las otras apps: se relee al volver
	// al Hub y, residente, cada cuarto de hora (la bandeja enseña lo de hoy).
	a.Lifecycle().SetOnEnteredForeground(p.Recargar)
	p.Recargar()
	if enBandeja {
		go recargarCada(p)
	}
	if enBandeja && slices.Contains(os.Args[1:], escritorio.ArgBandeja) {
		a.Run() // arrancado al iniciar sesión: en la bandeja, sin ventana
		return
	}
	w.ShowAndRun()
}

// barra es la de las apps de la edición (`local.Montar`) con el nombre de la edición y
// el buscador de todas las apps en medio.
func barra(l *local.App, w fyne.Window, nombre string, p *Portada, tr *Traedor, ex *Exportador) fyne.CanvasObject {
	marca := iconos.Marca(iconos.One, 24, tema.De(l.A.Settings().ThemeVariant())).Objeto()
	rotulo := canvas.NewText(nombre, theme.Color(theme.ColorNameForeground))
	rotulo.TextStyle = fyne.TextStyle{Bold: true}
	// Los botones de la barra de la edición (local.BotonDeBarra), como en las demás apps.
	traerB := local.BotonDeBarra(texto(l.Idioma(), "traer.boton"), iconos.Descargar, tr.Empezar)
	exportarB := local.BotonDeBarra(texto(l.Idioma(), "exportar.boton"), iconos.Subir, ex.Empezar)
	var nube *ui.Boton
	nube = local.BotonDeBarra(l.T("nube"), iconos.Nube, func() { l.MenuNube(w, nube) })
	ajustes := local.BotonDeBarra(l.T("ajustes"), iconos.Ajustes, func() { l.MostrarAjustes(w) })
	izquierda := container.NewHBox(container.NewCenter(marca), rotulo, ui.Espaciador(0))
	b := container.NewBorder(nil, nil, izquierda, container.NewHBox(traerB, exportarB, nube, ajustes), container.NewPadded(p.Buscador()))
	return container.NewBorder(container.NewVBox(container.NewPadded(b), widget.NewSeparator()), nil, nil, nil, p.Objeto())
}
