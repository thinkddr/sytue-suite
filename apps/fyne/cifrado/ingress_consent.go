// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// IngressConsent retains only the public authority accepted for one exact
// verified resources lifetime. BeforeOpen belongs outside RepositoryCycle.Open;
// consumers still revalidate this generation against Rooms for every operation.
type IngressConsent struct {
	roomsURL  string
	mu        sync.Mutex
	resources *Resources[*mx.Client]
	device    *mx.Client
	scope     storage.Scope
	target    storage.PublicationTarget
	authority matrixproof.Authority
}

func NewIngressConsent(roomsURL string) *IngressConsent { return &IngressConsent{roomsURL: roomsURL} }

func (c *IngressConsent) BeforeOpen(ctx context.Context, resources *Resources[*mx.Client], approve StorageApproval) error {
	authority, target, err := authorizeStorageReception(ctx, resources, approve, c.roomsURL)
	if err != nil {
		return err
	}
	bind, err := nativePublicationBinder(resources, api.NewRooms(c.roomsURL, resources.HTTP(), nil), authority)
	if err != nil {
		return err
	}
	if err := resources.bindStoragePublication(ctx, bind); err != nil {
		return err
	}
	scope := resources.PrivateStorageScope()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resources, c.device, c.scope, c.target, c.authority = resources, resources.Devices.Isolated, scope, target, authority
	return nil
}

func authorizeStorageReception(ctx context.Context, resources *Resources[*mx.Client], approve StorageApproval, roomsURL string) (matrixproof.Authority, storage.PublicationTarget, error) {
	var empty matrixproof.Authority
	var target storage.PublicationTarget
	if resources == nil || approve == nil || roomsURL == "" || resources.Devices.Isolated == nil {
		return empty, target, ErrPrivateResources
	}
	if err := resources.Revalidate(ctx); err != nil {
		return empty, target, err
	}
	target, err := resources.Devices.Isolated.StoragePublicationTarget()
	if err != nil {
		return empty, target, err
	}
	scope := resources.PrivateStorageScope()
	flow := ingress.RootConsent{Scope: scope, Target: target, Device: resources.Devices.Isolated, Service: api.NewRooms(roomsURL, resources.HTTP(), nil), Lease: resources.WorkLease(), Authorize: resources.Revalidate, Approve: approve}
	authority, err := flow.Ensure(ctx)
	if err != nil {
		return empty, target, err
	}
	if err := resources.Revalidate(ctx); err != nil {
		return empty, target, err
	}
	if authority.RequireActive(scope, authority.Generation) != nil || resources.Devices.Isolated.CheckStoragePublicationTarget(target) != nil {
		return empty, target, ErrPrivateResources
	}
	return authority, target, nil
}

// Authority grants no publication permit and never reads keys or reconstructs
// consent from a server response. A different session/device cannot borrow it.
func (c *IngressConsent) Authority(ctx context.Context, resources *Resources[*mx.Client]) (matrixproof.Authority, error) {
	if resources == nil {
		return matrixproof.Authority{}, ErrPrivateResources
	}
	if err := resources.Revalidate(ctx); err != nil {
		return matrixproof.Authority{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if resources != c.resources || resources.Devices.Isolated != c.device || resources.PrivateStorageScope() != c.scope || c.authority.RequireActive(c.scope, c.authority.Generation) != nil {
		return matrixproof.Authority{}, ErrPrivateResources
	}
	if err := c.device.CheckStoragePublicationTarget(c.target); err != nil {
		return matrixproof.Authority{}, err
	}
	return c.authority, nil
}

// BeforeOpenPanel uses the same native root consent for hosts with their own
// presenter. Call off the UI thread; the panel dispatches its confirmation.
func (c *IngressConsent) BeforeOpenPanel(ctx context.Context, resources *Resources[*mx.Client], panel *Panel) error {
	return c.BeforeOpen(ctx, resources, nativeStorageApproval(resources, panel))
}
