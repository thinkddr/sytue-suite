// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command drive es Sytue Drive de escritorio y móvil.
//
// Es SU PROPIA aplicación, no una pantalla de un binario que las contenga a todas:
// `docs/CONVENTIONS.md` §13.6 excepción 2. Lo que en la web es ir a otro subdominio,
// aquí es invocar a la otra app por su esquema de URL, que está declarado en
// `FyneApp.toml` y del que la cadena de empaquetado hace las cinco copias que cada
// sistema necesita.
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// CGO y sin servidor gráfico, es `go test ./apps/drive/...`, que pinta lo mismo con el
// pintor de software y compara los píxeles.
// paridad-web: apps/frontend/apps/drive/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
	privateStorage "git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
)

// Medidas de la ventana al abrir: el ancho canónico de la suite y un alto que deja ver
// una docena de filas de 51.
const (
	anchoInicial = 1280
	altoInicial  = 800
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

	tr, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Drive())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El nombre del producto es una MARCA y no se traduce: es el mismo en los 26
	// idiomas, igual que en la web.
	w := a.NewWindow("Sytue Drive")
	w.SetPadded(false)

	red, cliente := s.Conexion("drive", drive.SinGuardarSinConexion...)
	// El widget nativo lo lee su propia Fuente (la raíz y la cuota), SIN la caché de
	// trabajar sin conexión, del Drive privado que monte la vista (WidgetDesdeDrivePrivado)
	// y se vuelve a publicar tras cada escritura en él. Sin la vista montada (en segundo
	// plano, sin la clave), se conserva lo publicado.
	s.PublicadorWidget(s.FuenteWidget(tr))
	slot := container.NewStack()
	cryptoT := func(key string) string { return tr.T(key, nil) }
	store, storeErr := ajustes.Carpeta()
	nuevoPanel := func() *cifrado.Panel {
		panel := &cifrado.Panel{
			Base:   mx.Config{SytueApex: s.Apex(), StoreDir: filepath.Join(store, "drive-matrix"), DeviceName: "Sytue Drive · " + runtime.GOOS},
			Lienzo: w.Canvas, Traducir: cryptoT,
			Copiar: func(text string) { a.Clipboard().SetContent(text) },
			Abrir:  func(target string) error { return api.AbrirFuera(a, target) },
		}
		if storeErr != nil {
			panel.Base.StoreDir = ""
		}
		return panel
	}
	abrirSistema := s.AbrirDriveEnSistema
	if archivos := conectarArchivos(s); archivos != nil {
		abrirSistema = func() error { return archivos.abrir(s.AbrirDriveEnSistema) }
	}
	var control *drivePrivado
	notify := func(err error) {
		// Keep provider details and private source names out of notices/logs: solo
		// el código corto de la comprobación que falló (cifrado.CodigoFallo).
		codigo := cifrado.CodigoFallo(err)
		if codigo != "" {
			slog.Warn("drive: el Drive privado no abrió", "codigo", codigo)
		}
		if control != nil && control.pantalla != nil {
			control.pantalla.AvisarCifrado(cryptoT("matrix.failed"))
		} else if control != nil {
			view := ui.NuevoVacioError(cryptoT("matrix.failed"), codigo)
			view.Accion = ui.NuevoBoton(tr.T("action.retry", nil), func() {
				if c := control.ciclo; c != nil {
					control.preparar(c.ctx)
				} else if control.mostrar != nil {
					control.mostrar()
				}
			})
			slot.Objects = []fyne.CanvasObject{view}
			slot.Refresh()
			ui.RecolocarBarraSuite(slot)
		}
	}
	control = &drivePrivado{
		avisar:    notify,
		identidad: func() (auth.IdentitySnapshot, bool) { return s.Sesion.SnapshotIdentity(), s.Estado().Dentro() },
		bloqueado: func(dentro bool) {
			ui.AlAtras(w.Canvas(), nil)
			if dentro {
				view := ui.NuevoVacio(cryptoT("matrix.title"), cryptoT("matrix.subtitle"))
				view.Accion = ui.NuevoBoton(cryptoT("matrix.connect"), func() {
					if control.mostrar != nil {
						control.mostrar()
					}
				})
				slot.Objects = []fyne.CanvasObject{view}
			} else {
				view := ui.NuevoVacio(tr.T("account.signedOutTitle", nil), tr.T("account.signedOutDescription", nil))
				view.Accion = ui.NuevoBoton(tr.T("account.signIn", nil), func() { go func() { _ = s.Entrar(context.Background()) }() })
				slot.Objects = []fyne.CanvasObject{view}
			}
			slot.Refresh()
			ui.RecolocarBarraSuite(slot)
		},
		montar: func(ctx context.Context, repo repositorioPrivado) (*drive.Pantalla, error) {
			bound, err := cifrado.SessionHTTP(ctx)
			if err != nil {
				return nil, err
			}
			account := api.NewDrive(s.URL("drive"), bound, nil)
			data, err := account.WithPrivateData(repo)
			if err != nil {
				return nil, err
			}
			// Lo que se sube, renombra, mueve o borra va al Drive privado, que no pasa por
			// HTTP: avisa al widget como ClienteConAviso. Y el widget lee ESTE Drive mientras
			// esté abierto: el del servidor de la cuenta está congelado.
			data = data.ConAviso(s.AvisarEscritura)
			s.WidgetDesdeDrivePrivado(ctx, data, account)
			// UI handlers acquire their request context from screen.Vida below.
			screen := drive.NuevaPantalla(tr, data, red, w) //nolint:contextcheck // The presenter retains ctx as Vida; UI callbacks do not accept contexts.
			screen.Cuenta = account
			screen.Vida = ctx
			screen.Directorio = api.NewDirectorio(s.URL("id"), bound, cliente)
			screen.Grupos = s.Sesion.Grupos
			// «Combinar PDF» con la misma fachada privada que la pantalla: los PDF se
			// leen del repositorio cifrado y se unen en el dispositivo, como en CertiPDF.
			if pdf, err := api.NewPDF(s.URL("docs"), s.URL("drive"), bound, nil).WithPrivateDrive(repo); err == nil {
				screen.ConPDF(pdf.ConAviso(s.AvisarEscritura))
			}
			integraciones := api.NewIntegraciones(s.URL("integrations"), bound, nil)
			// Connections use consent; legacy server-side file import stays inactive.
			screen.AutorizarApp = func(ctx context.Context, template, instance string) (string, error) {
				return s.AutorizarApp(ctx, integraciones, template, instance, "native-storage")
			}
			screen.AutorizarAppNueva = func(ctx context.Context, template string) (string, error) {
				return s.AutorizarAppNueva(ctx, integraciones, template, "native-storage")
			}
			screen.AutorizarNube = func(ctx context.Context, provider string) (string, error) {
				return s.AutorizarNube(ctx, integraciones, provider)
			}
			screen.Apex = s.Apex()
			screen.Org = func() string { return repo.PrivateStorageScope().Tenant }
			screen.Entrar = func() error { return s.Entrar(ctx) }
			screen.ConfigurarSistema = func() { configurarSistema(s, tr, w, abrirSistema) } //nolint:contextcheck // The OS configuration dialog owns a separate interactive lifetime.
			montarSelector(w, screen)
			slot.Objects = []fyne.CanvasObject{screen.Objeto()} //nolint:contextcheck // UI callbacks use screen.Vida, already bound to ctx.
			slot.Refresh()
			ui.RecolocarBarraSuite(slot)
			go func() {
				if runtime.GOOS == "ios" && sistema.ArchivosDisponible() {
					return
				}
				err := documentos.Servir(ctx, data, documentos.Textos{Titulo: a.Metadata().Name, SinApp: tr.T("shell.documentsProvider.openApp", nil)})
				if err != nil && !errors.Is(err, documentos.ErrNoDisponible) && ctx.Err() == nil {
					slog.Warn("drive: private document provider unavailable")
				}
			}()
			screen.Arrancar(true) //nolint:contextcheck // Requests use screen.Vida; Arrancar is a UI lifecycle operation.
			return screen, nil
		},
	}
	// ofrecerBoveda: con el Drive de esta cuenta abierto en Matrix, ofrece pasarlo
	// a la bóveda directa, la misma de la web (ADR 0086). Con confirmación; Matrix
	// no se toca, y al terminar la app vuelve a elegir y abre la bóveda.
	ofrecerBoveda := func(ctx context.Context, resources *cifrado.Resources[*mx.Client], repository *cifrado.PreparedDriveRepository) {
		pasar := func() {
			paso := cifrado.NuevoPasoBoveda(ctx, cryptoT)
			dialogo := ui.NuevoDialogo(cryptoT("vault.moveTitle"), paso.Objeto())
			dialogo.RotuloCerrar = tr.T("ui.close", nil)
			paso.Cancelar = dialogo.Cerrar
			paso.Migrar = func(ctx context.Context) error {
				fabrica, err := cifrado.NuevaFabricaBoveda(s, "drive")
				if err != nil {
					return err
				}
				consulta := cifrado.ConsultaDriveDeSesion(s, s.Sesion.SnapshotIdentity())
				return cifrado.MigrarDriveABoveda(s.Sesion.WithAuthorizationIntent(ctx), cifrado.MigracionDrive{
					Matrix: repository.Repository, Copiar: cifrado.CopiarDesdeMatrix(resources.Devices.Isolated),
					AbrirBoveda: func(ctx context.Context, vaciar *api.PQVaultExpect) (*pqvault.Vault, error) {
						return cifrado.AbrirBovedaDeSesion(ctx, s, "drive", vaciar)
					},
					Fabrica: fabrica, DriveURL: s.URL("drive"), Arrendamiento: resources.WorkLease(),
					PendientesMatrix: consulta.PendientesMatrix,
				})
			}
			//nolint:contextcheck // Volver a elegir abre una vida nueva para la cuenta; la de este Drive de Matrix se cierra.
			paso.Hecho = func() {
				dialogo.Cerrar()
				control.reabrir()
			}
			dialogo.Mostrar(w.Canvas())
		}
		control.cuandoLista(func(p *drive.Pantalla) {
			p.OfrecerCifrado(cryptoT("vault.moveTitle"), cryptoT("vault.moveOffer"), cryptoT("vault.moveAction"), pasar)
		})
	}
	var cierreAnterior <-chan struct{}
	iniciarMatrix := func(ctx context.Context, ready func(context.Context, repositorioPrivado, func(context.Context) error)) error {
		legacy, isolated := nuevoPanel(), nuevoPanel()
		control.panel = isolated
		control.instalar() //nolint:contextcheck // UI events retain the original presenter's c.ctx; installing callbacks starts no request.
		failed := func(err error) {
			if ctx.Err() == nil {
				notify(err)
			}
		}
		legacy.Avisar = func(string) { failed(api.ErrPrivateDriveOperation) }
		isolated.Avisar = legacy.Avisar
		login := func(ctx context.Context, cfg mx.Config) (*mx.Client, error) {
			bound, err := cifrado.SessionHTTP(ctx)
			if err != nil {
				return nil, err
			}
			broker := api.NewRooms(s.URL("rooms"), bound, nil)
			if runtime.GOOS == "ios" || runtime.GOOS == "android" {
				return mx.LoginNative(ctx, cfg, broker, s.MatrixBrowserReturn)
			}
			return mx.Login(ctx, cfg, func(target string) error {
				opened := make(chan error, 1)
				fyne.Do(func() { opened <- api.AbrirFuera(a, target) })
				select {
				case err := <-opened:
					return err
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}
		legacy.Login, isolated.Login = login, login
		// Quien no puede abrir Matrix vuelve a la copia que guardó el servidor al
		// congelar, importada en la bóveda como lo haría la web (ADR 0086). Matrix
		// no se borra: lo escrito después sigue allí, sin leer.
		isolated.SoltarFuente = func(ctx context.Context) error {
			if err := cifrado.AdoptarCopiaDrive(ctx, s); err != nil {
				return err
			}
			fyne.Do(control.reabrir)
			return nil
		}
		isolated.SoltarSinIdentificar = true
		factory, err := cifrado.NewNativeFactory(s, "drive")
		if err != nil {
			return err
		}
		var repository repositorioPrivado
		consent := cifrado.NewIngressConsent(s.URL("rooms"))
		panels, err := cifrado.NewStoragePanels(cifrado.StoragePanelsConfig{
			Legacy: legacy, Isolated: isolated, Factory: factory,
			State: func(ctx context.Context, scope privateStorage.Scope) (privateStorage.IdentityCutoverState, error) {
				bound, err := cifrado.SessionHTTP(ctx)
				if err != nil {
					return privateStorage.IdentityCutoverState{}, err
				}
				return api.NewRooms(s.URL("rooms"), bound, nil).MatrixStorageMigration(ctx, scope)
			},
			Bootstrap: func(ctx context.Context, version privateStorage.IdentityVersion) (mx.Bootstrap, error) {
				bound, err := cifrado.SessionHTTP(ctx)
				if err != nil {
					return mx.Bootstrap{}, err
				}
				return api.NewRooms(s.URL("rooms"), bound, nil).MatrixStorageBootstrapForIdentity(ctx, "drive", version)
			},
			Ready: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) {
				if repository != nil {
					ready(ctx, repository, resources.Check)
					return
				}
				control.autorizarAlmacen(ctx, func(ctx context.Context) error { return consent.BeforeOpenPanel(ctx, resources, isolated) }, func() {
					prepared, err := cifrado.NewDriveRepository(ctx, resources, s.URL("drive"), s.URL("rooms"))
					if err != nil {
						failed(err)
						return
					}
					repository = prepared
					ready(ctx, repository, resources.Check)
					ofrecerBoveda(ctx, resources, prepared)
				})
			},
			Failed: failed,
		})
		if err != nil {
			return err
		}
		previous := cierreAnterior
		started := false
		start := func(startContext context.Context) {
			if ctx.Err() != nil {
				return
			}
			if previous != nil {
				select {
				case <-previous:
				default:
					return
				}
			}
			if err := panels.Start(startContext); err != nil {
				failed(err)
				return
			}
			started = true
		}
		control.reintentar = func() {
			if started {
				panels.Retry()
			} else {
				start(s.Sesion.WithAuthorizationIntent(ctx))
			}
		}
		control.mostrar = func() {
			if started {
				panels.Show()
			} else {
				control.reintentar()
			}
		}
		control.cerrarPaneles = func(drain func()) { //nolint:contextcheck // UI teardown cancels both device lifetimes; the joined cleanup deliberately outlives that cancellation.
			panels.Invalidate()
			legacy.AlCerrar = func() func() { return drain }
			isolated.AlCerrar = legacy.AlCerrar
			legacy.Cerrar()
			isolated.Cerrar()
			closed := make(chan struct{})
			cierreAnterior = closed
			go func() { //nolint:contextcheck // Teardown joins both SDKs after the view was cancelled; native keys and journals remain persisted.
				if previous != nil {
					<-previous
				}
				_ = legacy.Retire(context.Background())
				_ = isolated.Retire(context.Background())
				close(closed)
			}()
		}
		intent := s.Sesion.WithAuthorizationIntent(ctx)
		if previous != nil {
			go func() {
				select {
				case <-previous:
					fyne.Do(func() { start(intent) })
				case <-ctx.Done():
				}
			}()
		} else {
			start(intent)
		}
		return nil
	}
	// La bóveda directa (ADR 0086): la misma que usa la web. Se desbloquea con la
	// clave de Sytue ID del dispositivo (PuertaBoveda), sin Matrix ni contraseña
	// nueva, y su diario es el suyo (sesion.DiarioBoveda), nunca el de Matrix.
	var iniciarBoveda func(context.Context, func(context.Context, repositorioPrivado, func(context.Context) error)) error
	iniciarBoveda = func(ctx context.Context, ready func(context.Context, repositorioPrivado, func(context.Context) error)) error {
		fabrica, err := cifrado.NuevaFabricaBoveda(s, "drive")
		if err != nil {
			return err
		}
		failed := func(err error) {
			if ctx.Err() == nil {
				notify(err)
			}
		}
		// abierta y fallida solo se tocan en el hilo de la interfaz. fallida: la
		// puerta ya entregó su bóveda y abrir los recursos falló; reintentar
		// empieza con una puerta nueva.
		var abierta *cifrado.RecursosBoveda
		fallida := false
		//nolint:contextcheck // La puerta vive lo que la cuenta (Iniciar(ctx)); sus botones no reciben contexto.
		puerta, err := cifrado.PuertaDriveDeSesion(s, cryptoT, func(entrega context.Context, vault *pqvault.Vault) {
			if ctx.Err() != nil || abierta != nil {
				vault.Close()
				return
			}
			go func() {
				recursos, repository, err := cifrado.AbrirDriveBoveda(entrega, fabrica, s.URL("drive"), vault)
				fyne.Do(func() {
					if err != nil {
						fallida = true
						failed(err)
						return
					}
					if ctx.Err() != nil || abierta != nil {
						go func() { repository.Close(); recursos.Close() }()
						return
					}
					abierta = recursos
					ready(recursos.Context(), repository, recursos.Check) //nolint:contextcheck // El repositorio vive lo que la app sobre esa cuenta (testigo persistente), como Resources.Context en Matrix.
				})
			}()
		})
		if err != nil {
			return err
		}
		control.panel = nil
		enseñarPuerta := func() {
			if control.pantalla == nil {
				slot.Objects = []fyne.CanvasObject{puerta.Objeto()}
				slot.Refresh()
				ui.RecolocarBarraSuite(slot)
			}
		}
		control.reintentar = func() { //nolint:contextcheck // Reintentar usa la vida de la puerta, atada a ctx en Iniciar.
			switch {
			case abierta != nil:
			case fallida:
				puerta.Cerrar()
				if err := iniciarBoveda(ctx, ready); err != nil {
					failed(err)
				}
			default:
				enseñarPuerta()
				puerta.Reintentar() //nolint:contextcheck // Reintentar usa la vida de la puerta, atada a ctx en Iniciar.
			}
		}
		control.mostrar = enseñarPuerta
		control.cerrarPaneles = func(drain func()) {
			puerta.Cerrar()
			recursos := abierta
			go func() {
				if drain != nil {
					drain()
				}
				recursos.Close()
			}()
		}
		enseñarPuerta()
		puerta.Iniciar(ctx)
		return nil
	}
	// Por cuenta: la bóveda si la web ya llevó ahí su Drive y no quedan subidas de
	// Matrix por terminar; si no, Matrix como hasta ahora (cifrado.ElegirAlmacenDrive).
	control.iniciar = func(ctx context.Context, ready func(context.Context, repositorioPrivado, func(context.Context) error)) error {
		decidiendo := false
		var decidir func()
		decidir = func() {
			if decidiendo || ctx.Err() != nil {
				return
			}
			decidiendo = true
			control.reintentar, control.mostrar = decidir, decidir
			go func() {
				almacen, owner, err := cifrado.ElegirDriveDeSesion(ctx, s)
				fyne.Do(func() {
					decidiendo = false
					if ctx.Err() != nil || s.Sesion.SnapshotIdentity() != owner {
						return
					}
					if err == nil {
						if almacen == cifrado.DriveEnBoveda {
							err = iniciarBoveda(ctx, ready)
						} else {
							err = iniciarMatrix(ctx, ready)
						}
					}
					if err != nil {
						control.reintentar, control.mostrar = decidir, decidir
						notify(err)
					}
				})
			}()
		}
		decidir()
		return nil
	}
	conEscena := false
	s.AlCambiar(func(e sesion.Estado) {
		if !conEscena {
			return
		}
		// Observe intermediate logout/account events even when credentials moved again
		// before this UI callback runs (A -> B -> A).
		if control.dentro && (!e.Dentro() || e.Sujeto != control.cuenta.Subject) {
			control.cerrar()
			control.dentro = false
		}
		control.cambiar()
	})
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		control.cuandoLista(func(p *drive.Pantalla) { p.Subir(rutasLocales(uris)) })
	})
	// «Suelta para subir aquí» mientras se arrastran por encima. Es solo el aviso: donde
	// el sistema no avisa (Wayland) la subida al soltar funciona igual.
	soltar.AlArrastrar(w, func(activa bool) {
		control.cuandoLista(func(p *drive.Pantalla) { p.Arrastrando(activa) })
	})
	// Los atajos de la web (`lib/atajos.ts`): el lienzo solo los recibe si no hay nada
	// enfocado —o sea, fuera de un campo—; y con un diálogo o un menú encima, no.
	drive.EngancharAtajos(w.Canvas(), control.cuandoLista, control.siLista)
	w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Drive).Montar(slot))
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))
	w.SetOnClosed(control.cerrar)
	// Con interfaz: lanzada por iOS solo para el widget, ni importador ni explorador.
	s.CuandoHayaEscena(func() {
		conEscena = true
		control.bloqueado(false)
		control.cambiar()
	})
	s.RutaPropia(func(u *url.URL) bool {
		if u == nil {
			return false
		}
		if u.Scheme == "file" || u.Scheme == "content" || (u.Scheme == "" && filepath.IsAbs(u.Path)) {
			return true
		}
		if u.Query().Get("open-system") == "1" && strings.Trim(u.Path, "/") == "" && u.Host == "" {
			return true
		}
		if u.Query().Get("connect-storage") == "1" && strings.Trim(u.Path, "/") == "" && u.Host == "" {
			return true
		}
		if _, _, ok := drive.DestinoDeEnlace(u); ok {
			return true
		}
		if _, ok := api.LeerPeticionDrive(u); ok {
			return true
		}
		if _, ok := api.LeerCompartido(u); ok {
			return true
		}
		if _, ok := drive.SeccionDeEnlace(u); ok {
			return true
		}
		return false
	})
	// «Abrir en la aplicación» del explorador web: la carpeta y el fichero, por ruta.
	s.AlAbrirURL(func(u *url.URL) {
		if u == nil {
			return
		}
		control.cuandoLista(func(p *drive.Pantalla) {
			if u.Query().Get("open-system") == "1" && strings.Trim(u.Path, "/") == "" && u.Host == "" {
				go func() {
					if err := abrirSistema(); err != nil {
						slog.Warn("drive: no se pudo abrir en el sistema", "error", err)
					}
				}()
				return
			}
			if u.Query().Get("connect-storage") == "1" && strings.Trim(u.Path, "/") == "" && u.Host == "" {
				fyne.Do(p.AbrirAlmacenamiento)
			} else if carpeta, fichero, ok := drive.DestinoDeEnlace(u); ok {
				fyne.Do(func() { p.IrAEnlace(carpeta, fichero, func() { s.NoEncontrado() }) })
			} else if id, ok := api.LeerPeticionDrive(u); ok {
				fyne.Do(func() { p.AtenderPeticion(id) }) // el aviso de una petición de edición
			} else if id, ok := api.LeerCompartido(u); ok {
				fyne.Do(func() { p.AbrirCompartido(id) }) // el aviso de «compartido contigo»
			} else if sec, ok := drive.SeccionDeEnlace(u); ok {
				fyne.Do(func() { p.IrASeccion(sec) })
			} else if u.Scheme == "file" || u.Scheme == "content" || (u.Scheme == "" && filepath.IsAbs(u.Path)) {
				go func() {
					uriStr := u.String()
					if u.Scheme == "" {
						uriStr = "file://" + u.Path
					}
					uri, err := storage.ParseURI(uriStr)
					if err != nil {
						return
					}
					r, err := storage.Reader(uri)
					if err != nil {
						return
					}
					nombre := uri.Name()
					var tam int64 = -1
					if u.Scheme == "file" || (u.Scheme == "" && u.Path != "") {
						cleanPath := filepath.Clean(u.Path)
						//nolint:gosec // G703: Intencionalmente inspecciona el tamaño del fichero elegido por el usuario.
						if fi, err := os.Stat(cleanPath); err == nil {
							tam = fi.Size()
							if nombre == "" {
								nombre = fi.Name()
							}
						}
					}
					if nombre == "" {
						nombre = "archivo"
					}
					entrada := drive.Fluido(nombre, tam, r)
					fyne.Do(func() {
						if p.Vida != nil && p.Vida.Err() != nil {
							_ = r.Close()
							return
						}
						p.SubirEntradas([]drive.Entrada{entrada})
					})
				}()
			} else if strings.Trim(u.Path, "/") != "" || strings.Trim(u.Opaque, "/") != "" {
				s.NoEncontrado() // con un receptor apuntado, la sesión ya no sabe qué no es de nadie
			}
		})
	})
	s.Arrancar(context.Background())
	ejecutarDrive(a, w, s, tr)
}

// elegir abre el selector de ficheros DEL SISTEMA —el mismo que abre el navegador con
// `<input type="file" multiple>`, que es lo que hay que igualar— y le pasa a `listo`
// lo elegido en la rutina de la interfaz. El selector bloquea, así que va aparte.
//
// ponytail: sin selector del sistema (un Linux sin zenity ni kdialog) no se abre nada y
// queda soltar los ficheros en la ventana. El móvil tiene el suyo (`selector_movil.go`).
func elegir(listo func([]string)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("drive: el selector de ficheros ha fallado", "error", err)
			}
			return
		}
		if len(rutas) > 0 {
			fyne.Do(func() { listo(rutas) })
		}
	}()
}

// rutasLocales se queda con lo que es del disco —un fichero o una CARPETA, que se sube
// con su árbol—. Lo demás —un `content://` de Android, un enlace soltado desde el
// navegador— no se puede abrir con `os.Open`: el `content://` del selector de Android
// entra por `selector_movil.go`, como flujo.
func rutasLocales(uris []fyne.URI) []string {
	var rutas []string
	for _, u := range uris {
		if u == nil || u.Scheme() != "file" {
			continue
		}
		if _, err := os.Stat(u.Path()); err == nil {
			rutas = append(rutas, u.Path())
		}
	}
	return rutas
}
