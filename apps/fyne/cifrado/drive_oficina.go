// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// El Drive de la bóveda para «Guardar en Drive» de Docus, Calcus y Diapos nativos
// (ADR 0086). Es lo que hacen Photos y PDF (AplicacionDrive), con dos diferencias:
//
//   - No se abre hasta que la persona pulsa «Guardar en Drive». Entonces, si este
//     dispositivo aún no tiene el Drive de la bóveda abierto, se enseña ENCIMA del
//     editor la puerta de la bóveda (PuertaBoveda: abrir con la clave de Sytue ID del
//     dispositivo, volver a entrar si no abre) y se dice que se guarde otra vez. Nunca
//     se abre por su cuenta al arrancar ni al abrir un documento.
//   - Si la cuenta va por Matrix (su Drive aún no ha pasado a la bóveda), la
//     ofimática no abre el Drive de Matrix: un «vault:» apuntaría a un fichero que
//     la web no ve. Se dice que termine de pasarlo desde Sytue Drive.
//
// Con la cuenta mandada a Matrix, la app de Matrix nunca arranca (CanStart): la
// ofimática no prepara el dispositivo para Matrix por guardar un documento.

import (
	"context"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/office/guardardrive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// DriveOficina es el Drive de la bóveda de una app de la ofimática. Sus métodos, salvo
// los de Destinos, se llaman desde el hilo de la interfaz.
type DriveOficina struct {
	app      *AplicacionDrive
	fallo    error
	driveURL string
	slot     *fyne.Container
	acceso   atomic.Pointer[accesoOficina]
	// identidad es quien tiene la sesión ahora.
	identidad func() auth.IdentitySnapshot
	dispatch  func(func())

	mu        sync.Mutex
	pedido    bool
	pedidoPor auth.IdentitySnapshot
}

// accesoOficina es el repositorio de la bóveda montado y la cuenta que lo abrió.
type accesoOficina struct {
	repo   guardardrive.Repositorio
	cuenta Cuenta
}

// NuevoDriveOficina prepara el Drive de la bóveda de una app de la ofimática. No abre
// nada. Si este dispositivo no puede tener Drive de la bóveda (sin almacén de claves,
// por ejemplo), lo recuerda: «Guardar en Drive» con el Drive en la bóveda lo dice.
func NuevoDriveOficina(suite *sesion.Suite, application fyne.App, window fyne.Window, translate func(string) string) *DriveOficina {
	d := &DriveOficina{slot: container.NewStack(), dispatch: fyne.Do}
	if suite == nil || suite.Sesion == nil {
		d.fallo = ErrPrivateResources
		return d
	}
	d.driveURL = suite.URL("drive")
	d.identidad = suite.Sesion.SnapshotIdentity
	d.app, d.fallo = NuevaAplicacionDrive(AplicacionDriveConfig{
		Matrix: PrivateApplicationConfig[*privateDrive.Repository]{
			Suite: suite, Application: application, Window: window, Product: "drive", Translate: translate,
			CanStart: d.puedeEmpezar,
			// Nunca llega a usarse: con la cuenta en Matrix, CanStart dice que no.
			Build: func(context.Context, *Resources[*mx.Client]) (ApplicationPlan[*privateDrive.Repository], error) {
				return ApplicationPlan[*privateDrive.Repository]{}, ErrPrivateResources
			},
			Mount: func(context.Context, *Resources[*mx.Client], *privateDrive.Repository) (ApplicationView, error) {
				return ApplicationView{}, ErrPrivateResources
			},
		},
		MontarBoveda: func(_ context.Context, cuenta Cuenta, repo *privateDrive.Repository) (ApplicationView, error) {
			acceso := &accesoOficina{repo: repo, cuenta: cuenta}
			d.acceso.Store(acceso)
			d.soltarPedido()
			return ApplicationView{Object: ui.NuevoVacio(translate("matrix.title"), ""), Detach: func() { d.acceso.CompareAndSwap(acceso, nil) }}, nil
		},
		AlDecidir: func(almacen AlmacenDrive) {
			if almacen == DriveEnMatrix {
				d.soltarPedido() // nada que desbloquear aquí: se dice al guardar
			}
		},
	})
	return d
}

// Objeto es la franja que va encima del editor: vacía hasta que hace falta
// desbloquear el Drive de la bóveda.
func (d *DriveOficina) Objeto() fyne.CanvasObject { return d.slot }

// Envolver pone la franja encima de `editor`.
func (d *DriveOficina) Envolver(editor fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(d.slot, nil, nil, nil, editor)
}

// Close desmonta lo de la bóveda.
func (d *DriveOficina) Close() {
	if d.app != nil {
		d.app.Close()
	}
}

// puedeEmpezar es el CanStart de la app: solo tras pulsar «Guardar en Drive», para
// quien lo pulsó, y nunca con la cuenta en Matrix.
func (d *DriveOficina) puedeEmpezar() bool {
	d.mu.Lock()
	pedido, por := d.pedido, d.pedidoPor
	d.mu.Unlock()
	if !pedido || d.identidad == nil || d.identidad() != por {
		return false
	}
	return d.app == nil || !d.app.EnMatrix()
}

func (d *DriveOficina) soltarPedido() {
	d.mu.Lock()
	d.pedido = false
	d.mu.Unlock()
	d.render()
}

func (d *DriveOficina) render() {
	d.mu.Lock()
	pedido := d.pedido
	d.mu.Unlock()
	d.slot.Objects = nil
	if pedido && d.app != nil {
		d.slot.Objects = []fyne.CanvasObject{d.app.Objeto()}
	}
	d.slot.Refresh()
}

// pedir enseña la puerta y la (re)intenta, para la cuenta de ahora.
func (d *DriveOficina) pedir() {
	if d.app == nil || d.identidad == nil {
		return
	}
	d.mu.Lock()
	d.pedido, d.pedidoPor = true, d.identidad()
	d.mu.Unlock()
	d.render()
	d.app.Retry()
}

// mismaPersona: la bóveda montada es de la cuenta que guarda.
func mismaPersona(a, b storage.Scope) bool {
	return a.Tenant == b.Tenant && a.Subject == b.Subject
}

// boveda es Destinos.Boveda para `cuenta`.
func (d *DriveOficina) boveda(cuenta Cuenta) func(context.Context) (guardardrive.Repositorio, error) {
	return func(ctx context.Context) (guardardrive.Repositorio, error) {
		if err := cuenta.Check(ctx); err != nil {
			return nil, err
		}
		if a := d.acceso.Load(); a != nil && mismaPersona(a.cuenta.PrivateStorageScope(), cuenta.PrivateStorageScope()) {
			if err := a.cuenta.Check(ctx); err == nil {
				return a.repo, nil
			}
		}
		if d.fallo != nil || d.app == nil {
			return nil, guardardrive.ErrBovedaNoDisponible
		}
		if d.app.EnMatrix() {
			return nil, guardardrive.ErrBovedaPendiente
		}
		//nolint:contextcheck // La puerta vive lo que la cuenta, no lo que esta petición.
		d.dispatch(func() {
			if cuenta.Check(cuenta.Context()) == nil {
				d.pedir()
			}
		})
		return nil, guardardrive.ErrBovedaCerrada
	}
}

// Destinos son los dos Drive de `cuenta` (la de la API del servicio de Docs) para
// «Guardar en Drive». Preguntar si el Drive está en la bóveda no pide ninguna clave.
func (d *DriveOficina) Destinos(cuenta Cuenta) *guardardrive.Destinos {
	if d == nil || cuenta == nil {
		return nil
	}
	servidor := api.NewDrive(d.driveURL, cuenta.HTTP(), nil)
	scope := cuenta.PrivateStorageScope()
	scope.Product = "drive"
	return &guardardrive.Destinos{
		DriveEnBoveda: func(ctx context.Context) (bool, error) {
			if err := cuenta.Check(ctx); err != nil {
				return false, err
			}
			state, err := servidor.PrivateStorageState(ctx, scope)
			if err != nil {
				return false, err
			}
			if state.Validate(scope) != nil {
				return false, privateDrive.ErrSource
			}
			return state.Mode != "legacy", nil
		},
		Servidor: servidor,
		Boveda:   d.boveda(cuenta),
	}
}
