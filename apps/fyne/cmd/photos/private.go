// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"net/url"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	photoView "git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	// photosDrive es el Drive donde viven los originales: en Matrix o en la
	// bóveda directa, según la cuenta (cifrado.AplicacionDrive). La galería sube y
	// lee por él, sea cual sea.
	photosDrive struct {
		repository *privateDrive.Repository
		resources  cifrado.Cuenta
	}
	// photosPresenter es la galería montada para una cuenta. Sin repositorio va por
	// la API del servidor (fuente `legacy`, como la web); con él, por Matrix.
	photosPresenter struct {
		app        *photoView.App
		client     *api.Photos
		repository *privatePhotos.Repository
		resources  cifrado.Cuenta
	}
	photosStorage struct {
		slot            *fyne.Container
		photos          *cifrado.AplicacionRegistros[*privatePhotos.Repository]
		drive           *cifrado.AplicacionDrive
		driveRepository atomic.Pointer[photosDrive]
		presenter       atomic.Pointer[photosPresenter]
		// pendiente es el enlace que llegó sin galería montada (la app arrancando por
		// una fila o la acción del widget): se atiende al montarla.
		pendiente    atomic.Pointer[url.URL]
		needed       bool
		neededBy     auth.IdentitySnapshot
		driveFailure error
	}
)

func newPhotosStorage(suite *sesion.Suite, application fyne.App, window fyne.Window, translate *i18n.Translator) (*photosStorage, error) {
	host := &photosStorage{}
	text := func(key string) string { return translate.T(key, nil) }
	var err error
	mountDrive := func(resources cifrado.Cuenta, repository *privateDrive.Repository) cifrado.ApplicationView {
		access := &photosDrive{repository: repository, resources: resources}
		host.driveRepository.Store(access)
		// Por la API (la galería sin repositorio) el Drive privado solo hacía falta
		// para subir: ya abierto, su panel sobra encima de la galería.
		if current := host.presenter.Load(); current != nil && current.repository == nil && host.needed {
			host.needed = false
			host.render(text)
		}
		// La copia que esperaba por el Drive cifrado sigue ya (ADR 0086).
		if current := host.presenter.Load(); current != nil {
			fyne.Do(current.app.DriveCifradoAbierto)
		}
		return cifrado.ApplicationView{Object: ui.NuevoVacio(text("matrix.title"), ""), Detach: func() { host.driveRepository.CompareAndSwap(access, nil) }}
	}
	// Los originales van a la bóveda directa cuando la web ya llevó ahí el Drive
	// de esa persona, y si no, a Matrix como hasta ahora (ADR 0086).
	host.drive, host.driveFailure = cifrado.NuevaAplicacionDrive(cifrado.AplicacionDriveConfig{
		Matrix: cifrado.PrivateApplicationConfig[*privateDrive.Repository]{
			Suite: suite, Application: application, Window: window, Product: "drive", Translate: text,
			CanStart: func() bool { return host.needed && host.neededBy == suite.Sesion.SnapshotIdentity() },
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privateDrive.Repository], error) {
				return cifrado.NewDriveApplicationPlan(ctx, resources, suite.URL("drive"), suite.URL("rooms"))
			},
			Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privateDrive.Repository) (cifrado.ApplicationView, error) {
				return mountDrive(resources, repository), nil
			},
			State: func(state cifrado.ApplicationState, _ error) {
				if state == cifrado.ApplicationReady && host.photos != nil {
					host.photos.Retry()
				}
			},
		},
		MontarBoveda: func(_ context.Context, resources cifrado.Cuenta, repository *privateDrive.Repository) (cifrado.ApplicationView, error) {
			return mountDrive(resources, repository), nil
		},
	})
	montar := func(almacen almacenPhotos) (cifrado.ApplicationView, error) {
		presenter, err := newPhotosPresenter(application, window, suite, translate, almacen)
		if err != nil {
			return cifrado.ApplicationView{}, err
		}
		host.presenter.Store(presenter)
		if u := host.pendiente.Swap(nil); u != nil {
			fyne.Do(func() { host.atender(u) })
		}
		return cifrado.ApplicationView{Object: presenter.app.Objeto(), Detach: func() { presenter.app.Cerrar(); host.presenter.CompareAndSwap(presenter, nil) }}, nil
	}
	// Los registros propios de Photos (álbumes, favoritas, ediciones) siguen la
	// regla de las apps de registros (ADR 0086): con la fuente del servidor en
	// `legacy`, la galería va por la API, como la web, sin Matrix y sin congelar
	// nada; con la fuente ya copiada a Matrix, por Matrix, donde están sus datos.
	host.photos, err = cifrado.NuevaAplicacionRegistros(cifrado.AplicacionRegistrosConfig[*privatePhotos.Repository]{
		Matrix: cifrado.PrivateApplicationConfig[*privatePhotos.Repository]{
			Suite: suite, Application: application, Window: window, Product: "photos", Translate: text,
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privatePhotos.Repository], error) {
				pins, err := privatePhotos.NewDriveOriginalPinsProvider(func(ctx context.Context) (*privateDrive.Repository, error) {
					return host.originalRepository(ctx, resources, text)
				})
				if err != nil {
					return cifrado.ApplicationPlan[*privatePhotos.Repository]{}, err
				}
				return photosPlan(ctx, resources, suite, pins)
			},
			//nolint:contextcheck // The presenter retains its own mounted lifetime, not this short mount request.
			Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privatePhotos.Repository) (cifrado.ApplicationView, error) {
				return montar(almacenPhotos{cuenta: resources, identity: resources.Identity, repository: repository, resolveDrive: func(ctx context.Context) (*privateDrive.Repository, error) {
					// El mismo resolve del proveedor de pins: la galería sube por el
					// Drive oculto de esta app, no por el cliente legible.
					return host.originalRepository(ctx, resources, text)
				}}) //nolint:contextcheck // The presenter owns its retained resource lifetime, not this short mount callback.
			},
			State: func(state cifrado.ApplicationState, _ error) {
				if state == cifrado.ApplicationReady {
					host.needed = false
				}
				host.render(text)
			},
		},
		Modo: func(ctx context.Context, session api.Session, scope storage.Scope) (string, error) {
			state, err := api.NewPhotos(suite.URL("photos"), session, nil).PrivateStorageState(ctx, scope)
			return state.Mode, err
		},
		API: func(cuenta cifrado.Cuenta) (cifrado.ApplicationView, error) {
			if err := cuenta.Check(cuenta.Context()); err != nil {
				return cifrado.ApplicationView{}, err
			}
			// Sin Drive oculto: por la API los originales van al Drive del servidor,
			// que es donde el servicio de Photos los lee para indexarlos. Salvo si ese
			// Drive ya está en almacenamiento privado: entonces van, cifrados, al Drive
			// privado de la cuenta (driveDeSubida).
			identity := suite.Sesion.SnapshotIdentity()
			return montar(almacenPhotos{cuenta: cuenta, identity: identity, driveDeSubida: host.driveDeSubida(cuenta, identity, suite.URL("drive"), text), galeriaCifrada: host.galeriaCifrada(cuenta, identity, suite.URL("drive"), text)}) //nolint:contextcheck // La galería vive lo que la cuenta.
		},
	})
	if err != nil {
		if host.drive != nil {
			host.drive.Close()
		}
		return nil, err
	}
	host.slot = container.NewStack(host.photos.Objeto())
	return host, nil
}

func (h *photosStorage) render(text func(string) string) {
	if h.photos == nil {
		return
	}
	h.slot.Objects = []fyne.CanvasObject{h.photos.Objeto()}
	if h.needed {
		var status fyne.CanvasObject = ui.NuevoVacio(text("matrix.failed"), "")
		if h.drive != nil {
			status = h.drive.Objeto()
		}
		h.slot.Objects = []fyne.CanvasObject{container.NewBorder(status, nil, nil, nil, h.photos.Objeto())}
	}
	h.slot.Refresh()
	ui.RecolocarBarraSuite(h.slot)
}

func (h *photosStorage) Close() {
	h.photos.Close()
	if h.drive != nil {
		h.drive.Close()
	}
}

func photosPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite, pins privatePhotos.OriginalPinAuthority) (cifrado.ApplicationPlan[*privatePhotos.Repository], error) {
	var empty cifrado.ApplicationPlan[*privatePhotos.Repository]
	gateway := api.NewDrive(suite.URL("drive"), resources.HTTP(), nil)
	plans, err := privatePhotos.NewSourcePlanStore(privatePhotos.SourcePlanConfig{Context: ctx, Scope: resources.PrivateStorageScope(), Crypto: resources.Devices.Isolated, Gateway: gateway, Journal: resources.Journal, WorkLease: resources.WorkLease(), Originals: pins})
	if err != nil {
		return empty, err
	}
	source, err := privatePhotos.NewPlannedHTTPSource(resources.PrivateStorageScope(), api.NewPhotos(suite.URL("photos"), resources.HTTP(), nil), plans)
	if err != nil {
		plans.Close() //nolint:contextcheck // Constructor failure has started no source work.
		return empty, err
	}
	plan, err := cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*privatePhotos.Repository]{Gateway: gateway, Source: source, Service: api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil), New: privatePhotos.New, Historical: privatePhotos.NewHistoricalSource})
	if err != nil {
		plans.Close() //nolint:contextcheck // Constructor failure has started no source work.
		return empty, err
	}
	if resources.Devices.RequireLegacy {
		plan.Validate = func(ctx context.Context, repository *privatePhotos.Repository) error {
			_, err := repository.AuditBinaries(ctx)
			return err
		}
	}
	return plan, nil
}

func (h *photosStorage) originalRepository(ctx context.Context, resources *cifrado.Resources[*mx.Client], text func(string) string) (*privateDrive.Repository, error) {
	return h.drivePrivado(ctx, resources, resources.Identity, text)
}

// drivePrivado es el Drive privado de la cuenta (la bóveda o Matrix, lo que elija
// cifrado.AplicacionDrive). Si aún no está montado, lo pide —la galería enseña
// arriba su panel para desbloquearlo— y devuelve ErrNotReady.
func (h *photosStorage) drivePrivado(ctx context.Context, cuenta cifrado.Cuenta, identity auth.IdentitySnapshot, text func(string) string) (*privateDrive.Repository, error) {
	if err := cuenta.Check(ctx); err != nil {
		return nil, err
	}
	if access := h.driveRepository.Load(); access != nil {
		if err := access.resources.Check(ctx); err != nil {
			return nil, err
		}
		return access.repository, nil
	}
	//nolint:contextcheck // Device setup/retry retains its own witnessed resource lifetime, not the failed pin request.
	fyne.Do(func() {
		if cuenta.Check(cuenta.Context()) == nil {
			h.needed = true
			h.neededBy = identity
			h.render(text)
			if h.drive != nil {
				h.drive.Retry()
			}
		}
	})
	if h.driveFailure != nil {
		return nil, h.driveFailure
	}
	return nil, privatePhotos.ErrNotReady
}

// driveDeSubida es App.DriveDeSubida de la galería por la API (ADR 0086): el Drive
// del servidor mientras su fuente siga en `legacy`, como la web; si no —la persona ya
// tiene su Drive en la bóveda—, el Drive privado de la cuenta (vaultfiles.Store en
// la bóveda, por cifrado.AplicacionDrive), al que se sube cifrado y que la galería del
// servidor no indexa. Sin él abierto todavía, photoView.ErrDriveCifradoCerrado (y la
// galería enseña arriba cómo desbloquearlo).
func (h *photosStorage) driveDeSubida(cuenta cifrado.Cuenta, identity auth.IdentitySnapshot, driveURL string, text func(string) string) func(context.Context) (*api.Drive, error) {
	drive := h.driveCifrado(cuenta, identity, driveURL, text)
	return func(ctx context.Context) (*api.Drive, error) { return drive(ctx, true) }
}

// galeriaCifrada es App.GaleriaCifrada (ADR 0086): el Drive privado de la cuenta si su
// Drive está en la bóveda, para leer y escribir el índice de la galería cifrada;
// (nil, nil) si sigue en el servidor. Sin `abrir`, no pide desbloquearlo.
func (h *photosStorage) galeriaCifrada(cuenta cifrado.Cuenta, identity auth.IdentitySnapshot, driveURL string, text func(string) string) func(context.Context, bool) (*api.Drive, error) {
	drive := h.driveCifrado(cuenta, identity, driveURL, text)
	return func(ctx context.Context, abrir bool) (*api.Drive, error) {
		d, err := drive(ctx, abrir)
		if err != nil || !d.Privado() {
			return nil, err
		}
		return d, nil
	}
}

func (h *photosStorage) driveCifrado(cuenta cifrado.Cuenta, identity auth.IdentitySnapshot, driveURL string, text func(string) string) func(context.Context, bool) (*api.Drive, error) {
	return func(ctx context.Context, abrir bool) (*api.Drive, error) {
		if err := cuenta.Check(ctx); err != nil {
			return nil, err
		}
		scope := cuenta.PrivateStorageScope()
		scope.Product = "drive"
		servidor := api.NewDrive(driveURL, cuenta.HTTP(), nil)
		state, err := servidor.PrivateStorageState(ctx, scope)
		if err != nil {
			return nil, err
		}
		if state.Validate(scope) != nil {
			return nil, privateDrive.ErrSource
		}
		if state.Mode == "legacy" {
			return servidor, nil
		}
		if !abrir && h.driveRepository.Load() == nil {
			// Sin pedir desbloquearlo: la galería lo ofrece («Mostrar»).
			if h.driveFailure != nil {
				return nil, photoView.ErrDriveCifradoNoDisponible
			}
			return nil, photoView.ErrDriveCifradoCerrado
		}
		if _, err := h.drivePrivado(ctx, cuenta, identity, text); err != nil {
			switch {
			case h.driveFailure != nil:
				// Sin Drive privado en este dispositivo: no hay nada que desbloquear.
				return nil, errors.Join(photoView.ErrDriveCifradoNoDisponible, err)
			case errors.Is(err, privatePhotos.ErrNotReady):
				return nil, photoView.ErrDriveCifradoCerrado
			default:
				return nil, err
			}
		}
		return servidor.WithPrivateData(galeriaDriveData{
			resolve: func(ctx context.Context) (*privateDrive.Repository, error) {
				return h.drivePrivado(ctx, cuenta, identity, text)
			},
			scope: scope,
		})
	}
}

// esEnlaceDePhotos: lo que atender sabe abrir (el álbum público, uno compartido, uno
// propio o la acción «Añadir fotos» del widget).
func esEnlaceDePhotos(u *url.URL) bool {
	return photoView.TokenDeEnlace(u) != "" || photoView.AlbumDeEnlace(u) != "" ||
		photoView.AlbumPropioDeEnlace(u) != "" || api.LeerAccion("photos", u) == "upload"
}

// atender abre lo que pide un enlace en la galería montada; sin ella, lo guarda para
// cuando se monte. En el hilo de la interfaz.
func (h *photosStorage) atender(u *url.URL) {
	current := h.presenter.Load()
	if current == nil {
		if esEnlaceDePhotos(u) {
			h.pendiente.Store(u)
		}
		return
	}
	if token := photoView.TokenDeEnlace(u); token != "" {
		current.app.AbrirPublico(token)
	}
	if album := photoView.AlbumDeEnlace(u); album != "" {
		current.app.AbrirCompartido(album)
	}
	if album := photoView.AlbumPropioDeEnlace(u); album != "" {
		current.app.AbrirAlbum(album)
	}
	if api.LeerAccion("photos", u) == "upload" {
		current.app.AnyadirFotos()
	}
}
