// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command cast es Sytue Cast de escritorio y móvil: vídeo bajo demanda y directo
// (ADR 0061).
//
// Es SU PROPIA aplicación (`docs/CONVENTIONS.md` §13.6). Los vídeos PERSONALES de
// una cuenta migrada viven en el repositorio privado del dispositivo verificado:
// aquí ya no se construye el cliente autenticado legible del servicio, y la
// reproducción sale del lector privado por rangos autenticados del SDK —sin URL
// legible del proveedor ni archivo temporal en claro. Los contenidos compartidos,
// públicos y en directo NO se convierten silenciosamente en contenido personal:
// sus secciones se quedan fail-closed hasta que exista la entrega cifrada a
// destinatarios (roadmap, `almacenamiento-suite-e2e`).
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// servidor gráfico, es `go test ./apps/cast/...`, que pinta las mismas pantallas con el
// pintor de software y, con `SYTUE_CAPTURA=/ruta`, deja las capturas en disco.
// paridad-web: apps/frontend/apps/cast/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"io"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/cast"
	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/altavoz"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCast "git.sytue.eu/sytue/one/apps/nucleo/cast"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Ventana inicial: la misma con la que se mide la web (1280×800).
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las apps de la suite.
	s := sesion.Abrir(a, "")
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)
	w := a.NewWindow("Sytue Cast")
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	tr, err := cast.Traductor(s.Idioma())
	if err != nil {
		panic(err) // el catálogo va embebido: si falla, es un error de compilación
	}
	// El widget nativo lo lee su propia Fuente, sin depender de la pantalla, y se
	// vuelve a publicar tras cada escritura del cliente de arriba (ClienteConAviso).
	s.PublicadorWidget(s.FuenteWidget(tr))
	movil := a.Driver().Device().IsMobile()

	// La app privada en curso y el enlace que llegó antes de montarse.
	var current *cast.App
	var pendiente *url.URL
	var views castViews
	if movil {
		// Suite keeps these callbacks for the app lifetime. Register once so
		// these callbacks do not retain or call a retired presenter.
		views.registerLifecycle(s)
	}

	// mount monta la vista sobre la cuenta que toque: la API del servicio, como la web,
	// o los datos ya migrados a Matrix (ADR 0086, «clientes nativos»).
	mount := func(resources cifrado.Cuenta, facade *api.Cast) cifrado.ApplicationView {
		teca := photos.NuevaFototeca()
		v := cast.Vista{T: tr, Ventana: anchoInicial, Variante: tema.De(a.Settings().ThemeVariant()), Teca: teca, Movil: movil}
		p := cast.Nueva(facade, v, w)
		p.Movil = movil
		p.SoltarCopia = sesion.SoltarElegido
		view := views.mount(p)
		teca.Pedir = p.PedirPoster
		// Un póster que llega repinta la página: las tarjetas lo piden a la caché al pintarse.
		teca.AlLlegar = view.poster(fyne.Do)
		// El presentador privado ya está dentro: existe sólo con la sesión montada.
		p.ConSesion = func() bool { return true }
		p.Yo = func() string { return resources.PrivateStorageScope().Subject }
		p.AlEntrar = func() { go func() { _ = s.Entrar(context.Background()) }() }
		p.WebDe = func(ruta string) string { return api.AppURL(s.Apex(), "cast", "", ruta) }
		organizacion := resources.PrivateStorageScope().Tenant
		p.AbrirDrive = func() { _ = api.AbrirEn(a, s.Apex(), "drive", organizacion, "/") }
		p.Elegir = func() ([]string, error) {
			return ficheros.ElegirConTipos(tr.T("upload.choose", nil), "mp4", "m4v", "mov", "mkv", "webm", "avi")
		}
		p.ElegirImagen = func() (string, error) {
			fs, err := ficheros.ElegirConTipos(tr.T("manage.posterUpload", nil), "jpg", "jpeg", "png", "webp")
			if err != nil || len(fs) == 0 {
				return "", err
			}
			return fs[0], nil
		}
		p.Guardar = func(nombre string, r io.Reader) (string, error) {
			carpeta, err := ficheros.Carpeta()
			if err != nil {
				return "", err
			}
			return ficheros.Guardar(carpeta, nombre, r)
		}
		// El sonido y el directo: la tarjeta del sistema y la llamada nativa, como Rooms.
		// La emisión sigue fail-closed en la fachada hasta su versión privada.
		if llamada.Disponible {
			p.Audio = altavoz.Sistema{}
			p.Conectar = func(u, token string, o llamada.Opciones) (cast.Sala, error) {
				l, err := llamada.Entrar(u, token, o)
				if err != nil {
					return nil, err
				}
				return l, nil
			}
			p.Camara = func() (camara.Fuente, error) {
				pedido := camara.PedidoPorOmision
				pedido.ID = p.CamaraElegida()
				return camara.Abrir(pedido)
			}
			if camara.PantallaDisponible {
				p.Pantalla = camara.AbrirPantalla
			}
			p.Camaras = camara.Dispositivos
			p.CamaraConcedida = camara.Concedido
			p.PedirMicro = altavoz.PedirMicro
		}
		// En el teléfono: la pantalla no se apaga mientras se ve o se emite, y al irse al
		// fondo el vídeo se pausa (y vuelve a entrar al directo al volver).
		if movil {
			p.PantallaEncendida = func(si bool) { sesion.PantallaEncendida(a, si) }
			// La imagen dentro de imagen del sistema. Si el teléfono la tiene se pregunta al
			// abrir cada vídeo: aquí la actividad todavía no existe y la respuesta sería «no».
			p.PiPMovil = sesion.ImagenEnImagen
			sesion.AlSucesoPiP(func(que int) { view.pip(que == sesion.PiPCerrada) })
			// El campo en el que se escribe, por encima del teclado: se mide cuando el teclado
			// ya ha salido, que tarda lo que tarda su animación (en el emulador, más de 400 ms).
			ui.AlEscribir = func(c fyne.Canvas) {
				go func() {
					// Hasta que el alto deja de crecer: durante la animación el teclado mide
					// solo lo que lleva, y con esa medida el campo quedaba a medias debajo.
					antes := float32(-1)
					for range 20 {
						time.Sleep(100 * time.Millisecond)
						// En unidades del lienzo, con la conversión de cada sistema.
						px := ui.AltoTeclado(c)
						if px > 0 && px == antes {
							fyne.Do(func() { p.MostrarCampo(px) })
							break
						}
						antes = px
					}
					// Y cuando se va, fuera el hueco que se le dejó debajo de la página.
					for sesion.AltoTeclado() > 0 {
						time.Sleep(300 * time.Millisecond)
					}
					fyne.Do(func() { p.MostrarCampo(0) })
				}()
			}
		}
		// Soltar un vídeo encima de la ventana lo prepara para subir.
		soltar.AlArrastrar(w, p.Arrastrando)
		current = p
		// La portada pública es texto legible del servidor: el presentador privado
		// abre en los vídeos personales del repositorio cifrado.
		p.Ir(cast.RutaGestionar)
		p.Arrancar()
		if u := pendiente; u != nil {
			pendiente = nil
			p.AbrirEnlace(u)
		}
		return cifrado.ApplicationView{
			Object: p.Objeto(),
			Detach: func() {
				wasCurrent := view.detach(teca)
				if wasCurrent && current == p {
					current = nil
				}
				p.Cerrar()
			},
		}
	}

	private, err := cifrado.NuevaAplicacionRegistros(cifrado.AplicacionRegistrosConfig[*privateCast.Repository]{
		Matrix: cifrado.PrivateApplicationConfig[*privateCast.Repository]{
			Suite: s, Application: a, Window: w, Product: "cast",
			Translate: func(key string) string { return tr.T(key, nil) },
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privateCast.Repository], error) {
				return castPlan(ctx, resources, s)
			},
			Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privateCast.Repository) (cifrado.ApplicationView, error) {
				facade, err := api.NewCast(s.URL("cast"), resources.HTTP(), nil).WithPrivateData(repository.Data())
				if err != nil {
					return cifrado.ApplicationView{}, err
				}
				return mount(resources, facade), nil //nolint:contextcheck // The presenter retains its own mounted lifetime, not this short mount request.
			},
		},
		Soltar: cifrado.SoltarEn(s.URL("cast")),
		Modo: func(ctx context.Context, session api.Session, scope storage.Scope) (string, error) {
			state, err := api.NewCast(s.URL("cast"), session, nil).PrivateStorageState(ctx, scope)
			return state.Mode, err
		},
		API: func(cuenta cifrado.Cuenta) (cifrado.ApplicationView, error) {
			return mount(cuenta, api.NewCast(s.URL("cast"), cuenta.HTTP(), nil)), nil
		},
	})
	if err != nil {
		w.SetContent(ui.NuevoVacio(tr.T("matrix.failed", nil), ""))
	} else {
		// Cast SÍ sirve sin sesión —la portada pública, los canales y los vídeos públicos—
		// y cada pantalla que la necesita ofrece entrar: la barra no la tapa con su página
		// «Iniciar sesión», que dejaba Explorar inalcanzable sin entrar.
		w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Cast).ConSinSesionPropia().Montar(private.Objeto()))
		w.SetOnClosed(private.Close)
	}
	// Un enlace a un vídeo, a un canal o a una pantalla de Cast llega como
	// `sytue-cast:/cast/watch/<id>` (o su dirección web): se abre aquí, y antes del
	// montaje espera al presentador privado del mismo usuario.
	s.RutaPropia(cast.EsEnlaceDeCast)
	s.AlAbrirURL(func(u *url.URL) {
		fyne.Do(func() {
			if current != nil {
				current.AbrirEnlace(u)
				return
			}
			if cast.EsEnlaceDeCast(u) {
				pendiente = u
				private.Show()
			}
		})
	})
	// Con interfaz: lanzada por iOS solo para el widget no se pide la portada.
	s.CuandoHayaEscena(func() {
		if private != nil {
			private.Start()
		}
	})
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
