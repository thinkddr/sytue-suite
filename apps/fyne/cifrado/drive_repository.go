// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PreparedDriveRepository preserves the existing Drive presenter contract while
// sharing its source plan with other native consumers of private Drive files.
type PreparedDriveRepository struct {
	*privateDrive.Repository
	ciclo        *RepositoryCycle[*privateDrive.Repository]
	retireSource func(context.Context) error
}

// NewDriveApplicationPlan borrows the verified device pair. Historical readers
// remain registered until their work is drained and authenticated; devices and
// encrypted journals are always retained by the shared application host.
func NewDriveApplicationPlan(ctx context.Context, resources *Resources[*mx.Client], driveURL, roomsURL string) (ApplicationPlan[*privateDrive.Repository], error) {
	var empty ApplicationPlan[*privateDrive.Repository]
	if resources == nil || resources.PrivateStorageScope().Product != "drive" {
		return empty, ErrPrivateResources
	}
	if err := resources.Check(ctx); err != nil {
		return empty, err
	}
	gateway := api.NewDrive(driveURL, resources.HTTP(), nil)
	open := func(client *mx.Client, bind offline.WorkContextBinder) (*privateDrive.Repository, error) {
		return privateDrive.New(privateDrive.Config{Context: ctx, BindPublicationContext: bind, Scope: resources.PrivateStorageScope(), Crypto: client, Gateway: gateway, Source: gateway, Journal: resources.Journal, WorkLease: resources.WorkLease()}) //nolint:contextcheck // Constructor retains ctx and starts no IO.
	}
	target, err := open(resources.Devices.Isolated, resources.storagePublicationBinder())
	if err != nil {
		return empty, err
	}
	var legacy *privateDrive.Repository
	if resources.Devices.RequireLegacy {
		legacy, err = open(resources.Devices.Legacy, nil)
		if err != nil {
			target.Close() //nolint:contextcheck // Cancel an unused repository lifetime; construction started no IO.
			return empty, err
		}
	}
	//nolint:contextcheck // Cancel only unused constructor lifetimes before registration or IO.
	unused := func() {
		target.Close()
		if legacy != nil {
			legacy.Close()
		}
	}
	sources := []offline.PrivateWork{}
	if legacy != nil {
		sources = append(sources, legacy)
	}
	retained, err := NewRetainedSources(resources, sources...)
	if err != nil {
		unused()
		return empty, err
	}
	config := privateDrive.IdentityCutoverConfig{Scope: resources.PrivateStorageScope(), Service: api.NewRooms(roomsURL, resources.HTTP(), nil), Legacy: legacy, Isolated: target, Authorize: resources.Revalidate}
	if legacy != nil {
		config.CopyFile = func(ctx context.Context, destination storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
			return resources.Devices.Legacy.MigrateStorageFileContentAt(ctx, resources.Devices.Isolated, gateway, resources.Journal, job, destination, ref, object, revision)
		}
	}
	cutover, err := privateDrive.NewIdentityCutover(config)
	if err != nil {
		unused()
		return empty, err
	}
	return ApplicationPlan[*privateDrive.Repository]{Repository: target, Prepare: func(ctx context.Context, _ *privateDrive.Repository) error {
		if err := retained.Register(ctx); err != nil {
			return err
		}
		return cutover.Prepare(ctx)
	}, Retire: retained.Retire}, nil
}

func NewDriveRepository(ctx context.Context, resources *Resources[*mx.Client], driveURL, roomsURL string) (*PreparedDriveRepository, error) {
	plan, err := NewDriveApplicationPlan(ctx, resources, driveURL, roomsURL)
	if err != nil {
		return nil, err
	}
	cycle, err := NewRepositoryCycle(resources, RepositoryConfig[*privateDrive.Repository]{Create: func(context.Context) (*privateDrive.Repository, error) { return plan.Repository, nil }, Prepare: plan.Prepare}) //nolint:contextcheck // The cycle retains the resources' presenter lifetime, not this constructor request.
	if err != nil {
		plan.Repository.Close() //nolint:contextcheck // Construction has started no IO and owns only its repository lifetime.
		_ = plan.Retire(context.WithoutCancel(ctx))
		return nil, err
	}
	return &PreparedDriveRepository{Repository: plan.Repository, ciclo: cycle, retireSource: plan.Retire}, nil
}
func (r *PreparedDriveRepository) Initialize(ctx context.Context) error { return r.ciclo.Open(ctx) }

//nolint:contextcheck // Detached cleanup joins cancelled presenter work; it never grants new write authority or deletes retained WAL.
func (r *PreparedDriveRepository) Close() {
	ctx := context.Background()
	if _, err := r.ciclo.RetirePreservingWork(ctx); err != nil {
		return
	}
	_ = r.retireSource(ctx)
}
