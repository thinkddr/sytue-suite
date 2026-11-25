// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

// RecordApplicationRepository binds the shared historical migration to a domain
// without exposing either its internal record engine or legacy CRUD to a screen.
type RecordApplicationRepository interface {
	PrivateRepository
	IdentityCutover(records.IdentityCutoverConfig) (*records.IdentityCutover, error)
}

type RecordApplicationConfig[R RecordApplicationRepository] struct {
	BindPublicationContext offline.WorkContextBinder
	Gateway                *api.Drive
	Source                 records.Source
	Service                transfer.CutoverService
	New                    func(records.Config) (R, error)
	Historical             func(records.Config) (*records.HistoricalSource, error)
	// OnWrite avisa tras cada escritura aceptada en el repositorio privado: el
	// Suite.AvisarEscritura de la app, que vuelve a publicar su widget (ADR 0078). Por
	// la API lo hace sesion.Suite.ClienteConAviso; aquí nada pasa por HTTP. Opcional.
	OnWrite func()
}

// NewRecordApplicationPlan reuses the same complete-source cutover and retained
// readers for every record domain. Its constructors fix their own validators.
//
//nolint:gocyclo // valida cada constructor del dominio antes de montar el plan.
func NewRecordApplicationPlan[R RecordApplicationRepository](ctx context.Context, resources *Resources[*mx.Client], cfg RecordApplicationConfig[R]) (ApplicationPlan[R], error) {
	var plan ApplicationPlan[R]
	if resources == nil || cfg.Gateway == nil || cfg.Source == nil || cfg.Service == nil || cfg.New == nil || cfg.Historical == nil {
		return plan, ErrPrivateResources
	}
	if err := resources.Check(ctx); err != nil {
		return plan, err
	}
	bind := resources.storagePublicationBinder()
	if cfg.BindPublicationContext != nil {
		if bind != nil {
			return plan, ErrPrivateResources
		}
		bind = cfg.BindPublicationContext
	}
	configuration := records.Config{Context: ctx, BindPublicationContext: bind, WorkLease: resources.WorkLease(), Scope: resources.PrivateStorageScope(), Crypto: resources.Devices.Isolated, Gateway: cfg.Gateway, Journal: resources.Journal, Source: cfg.Source, OnWrite: cfg.OnWrite}
	target, err := cfg.New(configuration)
	if err != nil {
		return plan, err
	}
	if nilReference(target) {
		return plan, ErrPrivateResources
	}
	var historical *records.HistoricalSource
	if resources.Devices.RequireLegacy {
		configuration.Crypto = resources.Devices.Legacy
		configuration.BindPublicationContext = nil
		configuration.OnWrite = nil
		historical, err = cfg.Historical(configuration)
		if err != nil {
			target.Close()
			return plan, err
		}
	}
	sources := []offline.PrivateWork{}
	if historical != nil {
		sources = append(sources, historical)
	}
	retained, err := NewRetainedSources(resources, sources...)
	if err != nil {
		target.Close()
		if historical != nil {
			historical.Close() //nolint:contextcheck // The historical reader has not started IO or registered work.
		}
		return plan, err
	}
	cutoverConfig := records.IdentityCutoverConfig{Scope: configuration.Scope, Service: cfg.Service, Legacy: historical, Authorize: resources.Revalidate}
	if historical != nil {
		cutoverConfig.CopyFile = func(ctx context.Context, destination storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
			return resources.Devices.Legacy.MigrateStorageFileContentAt(ctx, resources.Devices.Isolated, cfg.Gateway, resources.Journal, job, destination, ref, object, revision)
		}
	}
	cutover, err := target.IdentityCutover(cutoverConfig)
	if err != nil {
		target.Close()
		if historical != nil {
			historical.Close() //nolint:contextcheck // The historical reader has not started IO or registered work.
		}
		return plan, err
	}
	plan.Repository = target
	plan.Prepare = func(ctx context.Context, _ R) error {
		if err := retained.Register(ctx); err != nil {
			return err
		}
		return cutover.Prepare(ctx)
	}
	plan.Retire = retained.Retire
	return plan, nil
}
