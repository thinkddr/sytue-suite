// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// AutorizarRecepcion is the native ingress setup step after recovery and device
// verification. An already matching active root does not ask again. The returned
// authority is metadata; the worker retains and revalidates its generation.
func AutorizarRecepcion(ctx context.Context, resources *Resources[*mx.Client], panel *Panel, roomsURL string) (matrixproof.Authority, error) {
	if resources == nil || panel == nil || resources.Devices.Isolated == nil {
		return matrixproof.Authority{}, ErrPrivateResources
	}
	authority, _, err := authorizeStorageReception(ctx, resources, nativeStorageApproval(resources, panel), roomsURL)
	return authority, err
}

// ConfirmarAlmacen is RootConsent.Approve for this panel's verified device.
// Call off the UI thread. Closing the prompt or changing accounts cancels it;
// showing it never creates a challenge, signs a request or enrolls a root.
func (c *Panel) ConfirmarAlmacen(ctx context.Context, request matrixproof.RootEnrollment) error {
	if !request.Scope.Valid() || !request.Target.Valid() {
		return ErrPrivateResources
	}
	var dialog *ui.Dialogo
	var once sync.Once
	result := make(chan error, 1)
	finish := func(err error) { once.Do(func() { result <- err }) }
	fyne.Do(func() {
		if err := ctx.Err(); err != nil {
			finish(err)
			return
		}
		client, err := c.Cliente(ctx)
		if err != nil || !c.Verificado() || c.lienzo() == nil {
			finish(ErrPrivateResources)
			return
		}
		if !storageConsentIdentity(client, request) {
			finish(ErrPrivateResources)
			return
		}
		cancel := func() { finish(context.Canceled); dialog.Cerrar() }
		approve := func() { finish(ctx.Err()); dialog.Cerrar() }
		dialog = ui.Confirmacion(c.t("matrix.allowStorageTitle"), c.t("action.cancel"), c.t("matrix.allowStorageAction"), false,
			c.parrafo(c.t("matrix.allowStorageBody")), cancel, approve)
		dialog.Mostrar(c.lienzo())
	})
	defer fyne.Do(func() {
		if dialog != nil {
			dialog.Cerrar()
		}
	})
	select {
	case err := <-result:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func storageConsentIdentity(client interface {
	Identity() (string, string)
	DeviceID() string
}, request matrixproof.RootEnrollment,
) bool {
	user, tenant := client.Identity()
	return user == request.UserID && tenant == request.Scope.Tenant && client.DeviceID() == request.Target.DeviceID
}
