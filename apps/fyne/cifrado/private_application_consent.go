// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"sync"

	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// StorageApproval is UI consent for one exact verified isolated device. It
// cannot sign, enroll or change a root; callers keep RootConsent.Ensure's checks.
type StorageApproval func(context.Context, matrixproof.RootEnrollment) error

func nativeStorageApproval(resources *Resources[*mx.Client], panel *Panel) StorageApproval {
	return func(parent context.Context, request matrixproof.RootEnrollment) error {
		if resources == nil || panel == nil || request.Scope != resources.PrivateStorageScope() || request.Target != resources.isolated.Target {
			return ErrPrivateResources
		}
		ctx, cancel := context.WithCancel(parent)
		defer cancel()
		stop := context.AfterFunc(resources.Context(), cancel) //nolint:contextcheck // Approval inherits the operation and also stops when its retained resources retire.
		defer stop()
		if err := checkStorageApproval(ctx, resources, panel, request); err != nil {
			return err
		}
		if err := panel.ConfirmarAlmacen(ctx, request); err != nil {
			return err
		}
		return checkStorageApproval(ctx, resources, panel, request)
	}
}

func checkStorageApproval(ctx context.Context, resources *Resources[*mx.Client], panel *Panel, request matrixproof.RootEnrollment) error {
	if err := resources.Revalidate(ctx); err != nil {
		return err
	}
	client, err := panel.Cliente(ctx)
	if err != nil || client != resources.Devices.Isolated {
		return ErrPrivateResources
	}
	target, err := client.StoragePublicationTarget()
	if err != nil {
		return err
	}
	user, _ := client.Identity()
	if request.Target != target || request.UserID != user {
		return ErrPrivateResources
	}
	return resources.Check(ctx)
}

func nativeStorageBeforeOpen(approvals *sync.Map, before func(context.Context, *Resources[*mx.Client], StorageApproval) error) func(context.Context, *Resources[*mx.Client]) error {
	if before == nil {
		return nil
	}
	return func(ctx context.Context, resources *Resources[*mx.Client]) error {
		value, found := approvals.Load(resources)
		if !found {
			return ErrPrivateResources
		}
		approve, ok := value.(StorageApproval)
		if !ok {
			return ErrPrivateResources
		}
		return before(ctx, resources, approve)
	}
}
