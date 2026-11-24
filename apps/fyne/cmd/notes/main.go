// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command notes es Sytue Notepads de escritorio y móvil.
//
// Es SU PROPIA aplicación, no una pantalla de un binario que las contenga a todas:
// `docs/CONVENTIONS.md` §13.6 excepción 2. Lo que en la web es ir a otro subdominio,
// aquí es invocar a la otra app por su esquema de URL, que está declarado en
// `FyneApp.toml` y del que la cadena de empaquetado hace las cinco copias que cada
// sistema necesita.
//
// El contenido de la cuenta vive en el repositorio PRIVADO del dispositivo verificado:
// aquí ya no se construye el cliente HTTP legible del servicio. Sin dispositivo
// verificado no se ve contenido —los paneles de cifrado piden verificar y recuperar—
// y ninguna operación cae al texto en claro. Las funciones que necesitan contenido
// legible en el servidor (previsualizar un `doc`, el asistente de IA, compartir) no se
// ofrecen: la fachada privada las rechaza y no existe un interruptor que las simule.
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// CGO y sin servidor gráfico, es `go test ./apps/notes/...`, que pinta lo mismo con el
// pintor de software y compara los píxeles.
// paridad-web: apps/frontend/apps/notes/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/vinculados"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Medidas de la ventana al abrir: el ancho canónico de la suite y un alto que deja ver
// el árbol de cuadernos entero sin desplazar.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

// montarNotepads es el camino REAL de arranque de la app: arma el presentador sobre el
// dueño registrado —el mismo que cuenta y drena las fuentes locales— y nunca construye
// el servicio HTTP legible. Las funciones que mandarían contenido al servidor
// (previsualizar un `doc`, el asistente de IA, compartir, enlaces públicos) se quedan
// sin cablear hasta que exista su versión privada: no se simulan.
func montarNotepads(ctx context.Context, repository *notepadsRepository, tr *i18n.Translator, l i18n.Locale, red *offline.Monitor, win fyne.Window, abrirCalendar func(string)) (*notes.Pantalla, error) {
	if repository == nil {
		return nil, cifrado.ErrPrivateResources
	}
	p, err := notes.NuevaPantallaPrivada(ctx, repository.local, tr, l, repository, red, win)
	if err != nil {
		return nil, err
	}
	p.AbrirCalendar = abrirCalendar
	recordarConBiometria(p, tr)
	return p, nil
}

// montarNotepadsAPI es la app sobre la API del servicio, como la web (ADR 0086,
// «clientes nativos»): para la cuenta cuya fuente de Notes sigue `legacy`. Sin
// almacén local: la cola y los borradores sin conexión son del diario cifrado, que
// esta cuenta no usa.
func montarNotepadsAPI(ctx context.Context, svc *api.Notes, tr *i18n.Translator, l i18n.Locale, red *offline.Monitor, win fyne.Window, abrirCalendar func(string)) *notes.Pantalla {
	p := notes.NuevaPantallaConContexto(ctx, tr, l, svc, red, win)
	p.AbrirCalendar = abrirCalendar
	recordarConBiometria(p, tr)
	return p
}

// recordarConBiometria: abrir un cuaderno cifrado con Touch ID, Face ID o Windows
// Hello, si el sistema los tiene (decisión del dueño, 2026-09-24): la clave
// derivada, en el almacén seguro.
func recordarConBiometria(p *notes.Pantalla, tr *i18n.Translator) {
	if metodo, _ := escritorio.Biometria(); metodo != "" {
		p.Recordar = &notes.Recordador{
			Metodo: metodo, Motivo: tr.T("shell.lock.reasonNotebook", nil),
			Verificar: escritorio.Verificar,
			Leer:      auth.LeerSecreto, Guardar: auth.GuardarSecreto, Borrar: auth.BorrarSecreto,
		}
	}
}

// interceptarCierre keeps the window and its editor available when a pending
// encrypted edit has not yet reached its original journal. A later close gesture
// retries the same preparation; the interceptor is removed only on success.
func interceptarCierre(w interface {
	SetCloseIntercept(func())
	Close()
}, preparar func() bool,
) {
	w.SetCloseIntercept(func() {
		if !preparar() {
			return
		}
		w.SetCloseIntercept(nil)
		w.Close()
	})
}

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)

	l := s.Idioma()
	tr, err := i18n.New(l, i18n.Common(), catalogos.Notes())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El nombre del producto es una MARCA y no se traduce: es el mismo en los 26
	// idiomas, igual que en la web.
	w := a.NewWindow("Sytue Notepads")
	w.SetPadded(false)
	// El widget nativo lo lee su propia Fuente, SIN la caché de trabajar sin conexión,
	// y se vuelve a publicar tras cada escritura de Notepads.
	s.PublicadorWidget(s.FuenteWidget(tr))
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))
	// El cliente HTTP de la suite y su MONITOR de conectividad. Con la cuenta en Matrix
	// solo se usa el monitor (el contenido va cifrado por el repositorio del dispositivo
	// verificado); con la cuenta en la API, el cliente es el de la web.
	red, cliente := s.Conexion("notes")

	// El presentador privado en curso y el enlace que llegó antes de montarse: se
	// atiende al montar, como las props `notebookInicial`/`vinculoInicial` de la isla.
	var current *notes.Pantalla
	var pendiente *url.URL

	barra := ui.NuevaBarraSuite(tr, s, iconos.Notes)
	// vista engancha lo común de las dos formas de montar: la suite, los servicios
	// vinculados, los atajos y el enlace que llegó antes de montarse.
	vista := func(p *notes.Pantalla, check func(context.Context) error) cifrado.ApplicationView {
		p.VincularAutorizacionSuite(s.Sesion.WithAuthorizationIntent, check)
		// Las páginas del proveedor sin copiarlas ni convertirlas a notas nativas.
		p.ConServiciosVinculados(vinculados.Nuevo(s, w, tr, "notes").Abrir)
		barra.CederAtajoSi(p.EnfocarCaptura)
		current = p
		p.Arrancar()
		if u := pendiente; u != nil {
			pendiente = nil
			p.AtenderEnlace(u)
		}
		return cifrado.ApplicationView{
			Object: p.Objeto(),
			Detach: func() {
				p.Cerrar()
				if current == p {
					current = nil
				}
			},
			Join: p.Join,
		}
	}
	private, err := cifrado.NuevaAplicacionRegistros(cifrado.AplicacionRegistrosConfig[*notepadsRepository]{
		Matrix: cifrado.PrivateApplicationConfig[*notepadsRepository]{
			Suite: s, Application: a, Window: w, Product: "notes",
			Translate: func(key string) string { return tr.T(key, nil) },
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*notepadsRepository], error) {
				return notesPlan(ctx, resources, s)
			},
			//nolint:contextcheck // The presenter retains its own mounted lifetime, not this short mount request.
			Mount: func(ctx context.Context, resources *cifrado.Resources[*mx.Client], repository *notepadsRepository) (cifrado.ApplicationView, error) {
				organization := repository.PrivateStorageScope().Tenant
				p, err := montarNotepads(ctx, repository, tr, l, red, w, func(ruta string) { _ = api.AbrirEn(a, s.Apex(), "calendar", organization, ruta) }) //nolint:contextcheck // The presenter retains its own mounted lifetime; future gestures must not inherit the mount request.
				if err != nil {
					return cifrado.ApplicationView{}, err
				}
				return vista(p, resources.Check), nil
			},
		},
		Soltar: cifrado.SoltarEn(s.URL("notes")),
		Modo: func(ctx context.Context, session api.Session, scope storage.Scope) (string, error) {
			state, err := api.NewNotes(s.URL("notes"), session, nil).PrivateStorageState(ctx, scope)
			return state.Mode, err
		},
		API: func(cuenta cifrado.Cuenta) (cifrado.ApplicationView, error) {
			organization := cuenta.PrivateStorageScope().Tenant
			svc := api.NewNotes(s.URL("notes"), cuenta.HTTP(), s.ClienteConAviso(cliente))
			p := montarNotepadsAPI(cuenta.Context(), svc, tr, l, red, w, func(ruta string) { _ = api.AbrirEn(a, s.Apex(), "calendar", organization, ruta) })
			return vista(p, cuenta.Check), nil
		},
	})
	if err != nil {
		w.SetContent(ui.NuevoVacio(tr.T("matrix.failed", nil), ""))
	} else {
		w.SetContent(barra.Montar(private.Objeto()))
		w.SetOnClosed(private.Close)
	}
	// Soltar texto o un enlace sobre el cuaderno crea la nota, como en la web.
	soltar.AlSoltar(w, func(x soltar.Soltado) {
		if current != nil {
			current.Soltar(x.Texto, x.URLs, x.Posicion)
		}
	})
	// Los enlaces entrantes: `?notebook=&note=&h=` y `/notebooks/<id>` abren esa nota, y
	// `?link=<tipo>:<id>&label=` crea la nota vinculada, como `pages/index.astro`.
	// Antes del montaje se quedan pendientes y sacan los paneles de verificación al
	// frente, que el buzón de Mail también espera al suyo.
	s.AlAbrirURL(func(u *url.URL) {
		fyne.Do(func() {
			if current != nil && current.AtenderEnlace(u) {
				return
			}
			if current == nil {
				if _, ok := notes.LeerEntrante(u); ok {
					pendiente = u
					private.Show()
					return
				}
			}
			if strings.Trim(u.Path, "/") != "" || strings.Trim(u.Opaque, "/") != "" {
				s.NoEncontrado() // con un receptor apuntado, la sesión ya no sabe qué no es de nadie
			}
		})
	})
	// Con interfaz: lanzada por iOS solo para el widget no se pide nada.
	s.CuandoHayaEscena(func() {
		if private != nil {
			interceptarCierre(w, func() bool { return current == nil || current.PrepararRetirada() })
			private.Start()
		}
	})
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
