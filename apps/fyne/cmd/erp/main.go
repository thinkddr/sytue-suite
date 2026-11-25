// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command erp abre Sytue ERP en una ventana de verdad.
//
// Es SU PROPIA aplicación, como en la web es su propio subdominio: un binario
// instalable por producto (`docs/CONVENTIONS.md` §13.6). Su `client_id` en Sytue ID es
// `sytue-syerp` y su esquema de URL, el que declara `FyneApp.toml`.
//
// Para VERLA hace falta una máquina con pantalla; el piloto no la tiene. Lo que sí
// corre en cualquier sitio, sin CGO y sin servidor gráfico, es `go test ./apps/erp/...`,
// que pinta lo mismo con el pintor de software y compara los píxeles contra lo medido.
// paridad-web: apps/frontend/apps/erp/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/erp"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateERP "git.sytue.eu/sytue/one/apps/nucleo/erp"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)

	// El idioma lo decide el SISTEMA, igual que en la web lo decide el middleware:
	// nunca se detecta dentro de una vista. Uno que la suite no sirva cae al inglés.
	l := s.Idioma()
	t, err := erp.Traductor(l)
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}

	// El widget nativo lo lee su propia Fuente, sin depender de la pantalla, y se
	// vuelve a publicar tras cada escritura del cliente de arriba (ClienteConAviso).
	s.PublicadorWidget(s.FuenteWidget(t))

	// El título de la ventana es el NOMBRE DEL PRODUCTO y sale de `FyneApp.toml`, no
	// del catálogo: una marca no se traduce y no tiene clave en la web.
	nombre := "Sytue ERP"
	if m := fyne.CurrentApp().Metadata(); m.Name != "" {
		nombre = m.Name
	}
	w := a.NewWindow(nombre)
	w.SetPadded(false)
	barra := ui.NuevaBarraSuite(t, s, iconos.ERP)
	w.Resize(fyne.NewSize(1280, 744))

	var current *erp.Pantalla
	var pendingURL *url.URL

	atenderURL := func(p *erp.Pantalla, u *url.URL) {
		if m, ok := api.LeerPeticionERP(u); ok {
			p.AtenderPeticion(m)
		} else if v, ok := api.LeerVista(u); ok {
			if slices.Contains([]string{erp.VistaTareas, erp.VistaAprobaciones, erp.VistaLibro, erp.VistaBalance, erp.VistaObligaciones, erp.VistaBanco}, v) {
				p.IrA(v)
			}
		} else if strings.Trim(strings.TrimPrefix(u.Path, "/erp"), "/") != "" || strings.Trim(u.Opaque, "/") != "" {
			s.NoEncontrado()
		}
	}

	// mount monta la vista sobre la cuenta que toque: la API del servicio, como la web,
	// o los datos ya migrados a Matrix (ADR 0086, «clientes nativos»).
	mount := func(resources cifrado.Cuenta, facade *api.ERP) cifrado.ApplicationView {
		p := erp.NuevaPantalla(t, facade, nil)
		switch runtime.GOOS {
		case "js":
		default:
			p.ElegirFicheros = elegirFicheros
		}
		organization := resources.PrivateStorageScope().Tenant
		// Quien ha entrado: la cola de aprobaciones no le ofrece aprobar lo suyo.
		subject := resources.PrivateStorageScope().Subject
		p.Yo = func() string { return subject }
		// El cobro del punto de venta enviado y sin confirmar, en el almacén sin conexión
		// de la suite a nombre de esta cuenta: cerrar la app con un cobro incierto ya no
		// lo pierde (ERP-09). Sin almacén, vive lo que la pantalla, como antes.
		if almacen, err := sesion.Almacen(); err == nil {
			cuenta := sesion.IdentidadDeCuenta(s.URL("id"), subject, organization)
			p.CobrosTPV = erp.AlmacenCobroTPVDe(almacen, func() string { return cuenta })
		} else {
			slog.Warn("erp: sin almacén para el cobro pendiente del punto de venta", "error", err)
		}
		p.AbrirApp = func(producto, ruta string) { _ = api.AbrirEn(a, s.Apex(), producto, organization, ruta) }
		p.PoliticaURL = ui.PoliticaDePrivacidad(s.Apex(), l)
		p.Atajos(w.Canvas(), barra)
		current = p
		p.Arrancar()
		go func() {
			ctx, cancelar := context.WithTimeout(resources.Context(), 15*time.Second)
			defer cancelar()
			if yo, err := api.NewAccount(s.URL("id"), resources.HTTP(), nil).Me(ctx); err == nil {
				nombreCuenta := yo.Name
				if nombreCuenta == "" {
					nombreCuenta = yo.Email
				}
				fyne.Do(func() {
					if current == p {
						p.PonerNombre(nombreCuenta)
					}
				})
			}
		}()
		if u := pendingURL; u != nil {
			pendingURL = nil
			atenderURL(p, u)
		}
		return cifrado.ApplicationView{
			Object: p.Objeto(),
			Detach: func() {
				if current == p {
					current = nil
				}
			},
		}
	}

	private, err := cifrado.NuevaAplicacionRegistros(cifrado.AplicacionRegistrosConfig[*privateERP.Repository]{
		Matrix: cifrado.PrivateApplicationConfig[*privateERP.Repository]{
			Suite:       s,
			Application: a,
			Window:      w,
			Product:     "erp",
			Translate:   func(key string) string { return t.T(key, nil) },
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privateERP.Repository], error) {
				return erpPlan(ctx, resources, s, string(l))
			},
			Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privateERP.Repository) (cifrado.ApplicationView, error) {
				facade, err := api.NewERP(s.URL("erp"), resources.HTTP(), nil, string(l)).WithPrivateData(repository)
				if err != nil {
					return cifrado.ApplicationView{}, err
				}
				// El widget lee este repositorio mientras esté montado: el servidor de esta
				// cuenta está congelado desde que pasó a Matrix (ADR 0078). Vive lo que la cuenta
				// montada, no esta petición de montaje.
				//nolint:contextcheck // la vida es la de la cuenta (resources.Context).
				s.WidgetDesdeRegistrosPrivados(resources.Context(), resources.HTTP(), facade)
				return mount(resources, facade), nil //nolint:contextcheck // The presenter retains its own mounted lifetime, not this short mount request.
			},
		},
		Soltar: cifrado.SoltarEn(s.URL("erp")),
		Modo: func(ctx context.Context, session api.Session, scope storage.Scope) (string, error) {
			state, err := api.NewERP(s.URL("erp"), session, nil, string(l)).PrivateStorageState(ctx, scope)
			return state.Mode, err
		},
		API: func(cuenta cifrado.Cuenta) (cifrado.ApplicationView, error) {
			return mount(cuenta, api.NewERP(s.URL("erp"), cuenta.HTTP(), s.ClienteConAviso(nil), string(l))), nil
		},
	})

	if err != nil {
		w.SetContent(ui.NuevoVacioError(t.T("matrix.failed", nil), ""))
	} else {
		w.SetContent(barra.Montar(private.Objeto()))
		w.SetOnClosed(private.Close)
		// Con interfaz: lanzada por iOS solo para el widget no se pide nada.
		s.CuandoHayaEscena(func() {
			private.Start()
		})
	}

	// El aviso de una tarea asignada abre la vista de tareas (`?vista=tareas`), y el de
	// una aprobación pedida, la cola (`?vista=aprobaciones`), como la web.
	s.AlAbrirURL(func(u *url.URL) {
		fyne.Do(func() {
			if current != nil {
				atenderURL(current, u)
			} else {
				pendingURL = u
			}
		})
	})
	s.Arrancar(context.Background())
	w.ShowAndRun()
}

// elegirFicheros abre el selector del sistema —bloquea, así que va aparte— y le pasa a
// `listo` lo elegido en el hilo de la interfaz. Cancelar no hace nada.
func elegirFicheros(listo func([]string)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("erp: el selector de ficheros ha fallado", "error", err)
			}
			return
		}
		if len(rutas) > 0 {
			fyne.Do(func() { listo(rutas) })
		}
	}()
}
