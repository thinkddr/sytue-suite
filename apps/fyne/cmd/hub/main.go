// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command hub abre Sytue Hub, la puerta de entrada de la suite, en una ventana de
// verdad.
//
// Para VERLO hace falta una máquina con pantalla; el piloto no la tiene. Lo que sí
// corre en cualquier sitio, sin CGO y sin servidor gráfico, es
// `go test ./apps/hub/...`, que pinta las mismas pantallas con el pintor de software y
// comprueba los píxeles.
//
// # La barra de la suite, y lo que pone el menú de la ventana
//
// Arriba a la derecha van las tres piezas transversales —buscador, avisos y cuenta—,
// que es la barra de `ui.BarraSuite` (§13.6, corregido por el dueño el 2026-09-22).
// Lo que NO lleva es el selector de aplicaciones: ir de un producto a otro es cosa del
// sistema, no de una ventana. Aquí lo que la web pone en ese selector lo pone el MENÚ
// de la ventana, que es lo que un usuario de escritorio espera.
package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/apps/hub"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// Ventana inicial: la misma con la que se midió la web (1280×800), para que lo que se
// ve al abrir sea comparable con la captura de producción.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})
	// El Hub vive en la bandeja: abrirlo otra vez trae la ventana de la copia viva, en
	// vez de poner una segunda (`sesion.Traer`). Los enlaces los reenvía `sesion.Abrir`.
	if !slices.ContainsFunc(os.Args[1:], sesion.EsEnlaceDeLaSuite) && sesion.Traer(a) {
		return
	}

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")

	// El idioma lo decide el SISTEMA, con la misma pieza que el móvil y el resto de
	// la suite: `sistema.Idioma()` pregunta al sistema operativo y cae al entorno
	// POSIX. No se usa `lang.SystemLocale()` de Fyne porque eso sería una segunda
	// respuesta a la misma pregunta, y la otra es la que también sabe contestar en
	// Android, donde no hay variables de entorno.
	idioma := s.Idioma()
	t, err := i18n.New(idioma, i18n.Common(), catalogos.Console(), catalogos.Portal())
	if err != nil {
		panic(err) // los catálogos van embebidos: no puede fallar
	}

	v := hub.NuevaVista(t, a, time.Now()).
		Con(s.Hub). // el cliente del Hub ya viene con la sesión compartida
		ConBuzon(s.Hub).
		ConCuenta(s.Sesion).
		ConAlmacenamiento(api.NewDrive(s.URL("drive"), s.Sesion, nil)).
		ConAlta(api.NewAccount(s.URL("id"), s.Sesion, nil).AltaCompletada).
		ConExportador(s.Hub).
		ConDescargas(s.Hub).
		ConOrganizaciones(s).
		ConAltaGuiada(api.NewAlta(s.URL("id"), s.Sesion, nil), s.Entrar).
		ConIntegraciones(api.NewIntegraciones(s.URL("integrations"), s.Sesion, nil)).
		ConAgenda(hub.AgendaDeLaSuite{
			Calendario: api.NewCalendars(s.URL("calendar"), s.Sesion, nil),
			Correo:     api.NewMail(s.URL("mail"), s.Sesion, nil),
		})
	v.Widget = s.CapturarWidget
	v.ConDriveEnSistema(s.DriveEnSistema, s.PonerDriveEnSistema, s.AbrirDriveEnSistema)
	s.AlCambiar(func(sesion.Estado) { fyne.Do(func() { v.RefrescarDriveEnSistema(s.DriveEnSistema()) }) })
	w := a.NewWindow(t.T(hub.AInicio.ClaveTitulo(), nil))
	integraciones := api.NewIntegraciones(s.URL("integrations"), s.Sesion, nil)
	v.ConConexiones(w, integraciones, func(ctx context.Context, plantilla, instancia string) (string, error) {
		return s.AutorizarApp(ctx, integraciones, plantilla, instancia, "native-apps")
	})
	v.AlMostrar = func(d hub.Destino) { w.SetTitle(t.T(d.ClaveTitulo(), nil)) }
	w.SetPadded(false)
	// Los avisos flotantes van por ENCIMA del desplazador: dentro se irían al bajar.
	w.SetContent(ui.NuevaBarraSuite(t, s, iconos.One).Montar(container.NewStack(v.Desplazable(), v.Avisos())))
	v.Atajos(w.Canvas())
	ponerMenu(w, runtime.GOOS, menu(t, v.Ir))
	// El Hub se queda en la bandeja al cerrar la ventana: es la bandeja de avisos de la
	// suite, y así sigue avisando de lo de las apps cerradas (regla 4 de
	// `ui/avisosistema.go`). La misma lista de pantallas, que además trae la ventana.
	traer := func(d hub.Destino) {
		v.Ir(d)
		w.Show()
		w.RequestFocus()
	}
	bandeja := menu(t, traer)
	if m := a.Metadata(); m.ID != "" && !a.Driver().Device().IsMobile() {
		// Y el actualizador propio: el Hub es la app que se queda en la bandeja, así que
		// es quien mira si hay versión nueva (`sesion.ElementoActualizar`).
		// Ninguno de los dos en la variante de la Mac App Store: ahí actualiza la tienda
		// (un actualizador propio está prohibido), y el agente de inicio de sesión se
		// escribiría dentro del contenedor del sandbox, donde launchd no lo lee.
		if !escritorio.DeTienda {
			bandeja.Items = append(bandeja.Items, fyne.NewMenuItemSeparator(), alIniciarSesion(t, m, bandeja),
				s.ElementoActualizar(t, bandeja))
		}
		// «Pedir Touch ID al abrir» (o Windows Hello): bloquea la sesión de las trece.
		if b := s.ElementoBloqueo(t, bandeja); b != nil {
			bandeja.Items = append(bandeja.Items, b)
		}
	}
	enBandeja := sesion.Residente(a, w, bandeja)
	// Y el atajo global de la suite (Ctrl+Alt+Espacio; ⌃⌥⌘Espacio en macOS) trae su
	// buscador desde cualquier sitio. Sin él —otro programa tiene la combinación, o el
	// escritorio no ofrece atajos— la suite sigue igual.
	if !a.Driver().Device().IsMobile() {
		if _, err := escritorio.Atajo(a.Metadata().ID, t.T(hub.ABuscar.ClaveTitulo(), nil), func() {
			fyne.Do(func() { traer(hub.ABuscar) })
		}); err != nil {
			slog.Info("hub: sin atajo global", "motivo", err)
		}
	}
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	// Las demás apps llaman al Hub por su esquema: «Ver todas» del panel de avisos
	// lanza `sytue-one:/notificaciones`. Una ruta que no es de aquí abre la portada.
	s.AlAbrirURL(func(u *url.URL) {
		if strings.Trim(u.Path, "/") == "" {
			return // «ábrete» (`sesion.Traer`): la ventana ya sale, y se queda donde estaba
		}
		d, ok := hub.DestinoDeRuta(u.Path)
		v.Ir(d)
		if !ok {
			// Una ruta que el Hub no tiene: la página «no encontrada», y debajo ya el
			// inicio, que es a donde lleva su botón.
			s.NoEncontrado()
		}
	})
	// Quien entra DESPUÉS de abrir la ventana —desde el menú de cuenta o desde otra de
	// las apps— tiene que ver su portada, no el fallo de antes de entrar.
	dentro := s.Estado().Dentro()
	s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() && !dentro {
			v.Arrancar()
		}
		if e.Dentro() != dentro {
			avisosEnSegundoPlano(a, t, e.Dentro())
		}
		dentro = e.Dentro()
	})
	v.Arrancar()
	if dentro {
		avisosEnSegundoPlano(a, t, true)
	}
	s.Arrancar(context.Background())
	if enBandeja && slices.Contains(os.Args[1:], escritorio.ArgBandeja) {
		a.Run() // arrancado al iniciar sesión: en la bandeja, sin ventana
		return
	}
	w.ShowAndRun()
}

// avisosEnSegundoPlano arranca en Android, con alguien dentro, el servicio en primer
// plano que mantiene vivo al Hub con la app cerrada, y lo para al salir: es el papel que
// en escritorio hace la bandeja (regla 4 de `ui/avisosistema.go`). En los demás
// sistemas no hace nada.
func avisosEnSegundoPlano(a fyne.App, t *i18n.Translator, dentro bool) {
	if !a.Driver().Device().IsMobile() {
		return
	}
	go func() {
		var err error
		if dentro {
			err = escritorio.MantenerAvisos(a.Metadata().Name, t.T("notifications.title", nil))
		} else {
			err = escritorio.SoltarAvisos()
		}
		if err != nil {
			slog.Info("hub: el servicio de avisos en segundo plano no ha cambiado", "dentro", dentro, "error", err)
		}
	}()
}

// alIniciarSesion es la casilla «Abrir al iniciar sesión» del menú de la bandeja. El Hub
// es la bandeja de avisos de la suite: si arranca solo, los avisos de las apps cerradas
// llegan también antes de abrir ninguna (`escritorio.AlIniciarSesion`).
func alIniciarSesion(t *i18n.Translator, m fyne.AppMetadata, menu *fyne.Menu) *fyne.MenuItem {
	item := fyne.NewMenuItem(t.T("shell.launchAtLogin", nil), nil)
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

// menu es lo que sustituye al selector de aplicaciones: las ocho pantallas del portal,
// en el menú de la ventana y en el de la bandeja.
// ponerMenu pone el menú principal SOLO en macOS, que es donde vive en la barra del
// sistema. En Linux y Windows Fyne lo DIBUJA dentro de la ventana, encima de la barra de la
// suite: una franja clara de 20 px con «Sytue One apps» que la web no tiene (visto en una
// ventana real el 2026-09-27). Allí las mismas pantallas ya están en la bandeja.
func ponerMenu(w fyne.Window, goos string, m *fyne.Menu) {
	if goos == "darwin" {
		w.SetMainMenu(fyne.NewMainMenu(m))
	}
}

func menu(t *i18n.Translator, ir func(hub.Destino)) *fyne.Menu {
	// Solo las pantallas PORTADAS. Un menú con entradas que no llevan a ninguna
	// parte es peor que un menú corto: las que faltan siguen contadas como deuda en
	// el gate de paridad, que es donde se ven.
	destinos := hub.Portadas()
	elementos := make([]*fyne.MenuItem, 0, len(destinos))
	for _, d := range destinos {
		elementos = append(elementos, fyne.NewMenuItem(etiquetaMenu(t.T(d.ClaveTitulo(), nil)), func() { ir(d) }))
	}
	return fyne.NewMenu(t.T("shell.apps", nil), elementos...)
}

// etiquetaMenu quita la marca del título de una pantalla: en el menú de Sytue One,
// «Notificaciones — Sytue One» repite lo que ya dice el propio menú (medido en la bandeja
// de Windows). «Sytue One — Inicio» se queda en «Inicio».
func etiquetaMenu(titulo string) string {
	partes := strings.Split(titulo, " — ")
	quedan := partes[:0]
	for _, p := range partes {
		if strings.TrimSpace(p) != "Sytue One" {
			quedan = append(quedan, p)
		}
	}
	if len(quedan) == 0 {
		return titulo
	}
	return strings.Join(quedan, " — ")
}
