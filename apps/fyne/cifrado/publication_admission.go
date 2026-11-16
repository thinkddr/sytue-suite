// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// nativePublicationBinder retains the original product resources and approved
// isolated authority. Domain StartBound invokes it on each actual writer; the
// SDK uses that same stable WorkLease and never borrows SDK1's historical root.
func nativePublicationBinder(resources *Resources[*mx.Client], rooms *api.Rooms, authority matrixproof.Authority) (func(context.Context) (context.Context, error), error) {
	if resources == nil || rooms == nil || resources.Devices.Isolated == nil {
		return nil, ErrPrivateResources
	}
	return resources.Devices.Isolated.StoragePublicationBinder(resources.PrivateStorageScope(), authority, resources.WorkLease(), rooms, resources.Journal, resources.Revalidate)
}

// The approved binder belongs to this exact resources lifetime. It carries
// public authority and the original SDK/session; it is never a global writer.
func (r *Resources[D]) bindStoragePublication(ctx context.Context, bind offline.WorkContextBinder) error {
	if bind == nil {
		return ErrPrivateResources
	}
	if err := r.Check(ctx); err != nil {
		return err
	}
	r.publicationMu.Lock()
	defer r.publicationMu.Unlock()
	if err := r.Check(ctx); err != nil {
		return err
	}
	r.publicationBind = func(ctx context.Context) (context.Context, error) {
		if err := r.Check(ctx); err != nil {
			return nil, err
		}
		bound, err := bind(ctx)
		if err != nil {
			return nil, err
		}
		if bound == nil {
			return nil, ErrPrivateResources
		}
		if err := r.Check(bound); err != nil {
			return nil, err
		}
		return bound, nil
	}
	return nil
}

func (r *Resources[D]) storagePublicationBinder() offline.WorkContextBinder {
	r.publicationMu.RLock()
	defer r.publicationMu.RUnlock()
	return r.publicationBind
}
