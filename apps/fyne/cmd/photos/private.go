// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
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
)

type (
	photosDrive struct {
		repository *privateDrive.Repository
		resources  *cifrado.Resources[*mx.Client]
	}
	photosPresenter struct {
		app        *photoView.App
		repository *privatePhotos.Repository
		resources  *cifrado.Resources[*mx.Client]
	}
	photosStorage struct {
		slot            *fyne.Container
		photos          *cifrado.PrivateApplication[*privatePhotos.Repository]
		drive           *cifrado.PrivateApplication[*privateDrive.Repository]
		driveRepository atomic.Pointer[photosDrive]
		presenter       atomic.Pointer[photosPresenter]
		needed          bool
		neededBy        auth.IdentitySnapshot
		driveFailure    error
	}
)

func newPhotosStorage(suite *sesion.Suite, application fyne.App, window fyne.Window, translate *i18n.Translator) (*photosStorage, error) {
	host := &photosStorage{}
	text := func(key string) string { return translate.T(key, nil) }
	var err error
	host.drive, host.driveFailure = cifrado.NewPrivateApplication(cifrado.PrivateApplicationConfig[*privateDrive.Repository]{
		Suite: suite, Application: application, Window: window, Product: "drive", Translate: text,
		CanStart: func() bool { return host.needed && host.neededBy == suite.Sesion.SnapshotIdentity() },
		Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*privateDrive.Repository], error) {
			return cifrado.NewDriveApplicationPlan(ctx, resources, suite.URL("drive"), suite.URL("rooms"))
		},
		Mount: func(_ context.Context, resources *cifrado.Resources[*mx.Client], repository *privateDrive.Repository) (cifrado.ApplicationView, error) {
			access := &photosDrive{repository: repository, resources: resources}
			host.driveRepository.Store(access)
			return cifrado.ApplicationView{Object: ui.NuevoVacio(text("matrix.title"), ""), Detach: func() { host.driveRepository.CompareAndSwap(access, nil) }}, nil
		},
		State: func(state cifrado.ApplicationState, _ error) {
			if state == cifrado.ApplicationReady && host.photos != nil {
				host.photos.Retry()
			}
		},
	})
	host.photos, err = cifrado.NewPrivateApplication(cifrado.PrivateApplicationConfig[*privatePhotos.Repository]{
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
			presenter, err := newPhotosPresenter(application, window, suite, translate, resources, repository, func(ctx context.Context) (*privateDrive.Repository, error) {
				// El mismo resolve del proveedor de pins: la galería sube por el
				// Drive oculto de esta app, no por el cliente legible.
				return host.originalRepository(ctx, resources, text)
			}) //nolint:contextcheck // The presenter owns its retained resource lifetime, not this short mount callback.
			if err != nil {
				return cifrado.ApplicationView{}, err
			}
			host.presenter.Store(presenter)
			return cifrado.ApplicationView{Object: presenter.app.Objeto(), Detach: func() { presenter.app.Cerrar(); host.presenter.CompareAndSwap(presenter, nil) }}, nil
		},
		State: func(state cifrado.ApplicationState, _ error) {
			if state == cifrado.ApplicationReady {
				host.needed = false
			}
			host.render(text)
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
	if err := resources.Check(ctx); err != nil {
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
		if resources.Check(resources.Context()) == nil {
			h.needed = true
			h.neededBy = resources.Identity
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
