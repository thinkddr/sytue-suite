// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/application"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type MailApplicationConfig struct {
	Gateway   *api.Drive
	Source    *api.Mail
	Rooms     *api.Rooms
	Authority matrixproof.Authority
}
type MailApplicationRepository = application.Application

var _ PrivateRepository = (*MailApplicationRepository)(nil)

// The shared engine owns enrollment, source capture, cutover and workers.
// Fyne supplies only its real resources and maintained native SDK file copier.
func NewMailApplicationPlan(ctx context.Context, resources *Resources[*mx.Client], cfg MailApplicationConfig) (ApplicationPlan[*MailApplicationRepository], error) {
	var plan ApplicationPlan[*MailApplicationRepository]
	if resources == nil || cfg.Gateway == nil || cfg.Source == nil || cfg.Rooms == nil {
		return plan, ErrPrivateResources
	}
	bindPublication, err := nativePublicationBinder(resources, cfg.Rooms, cfg.Authority)
	if err != nil {
		return plan, err
	}
	common := application.Config{
		Gateway: cfg.Gateway, Source: cfg.Source, Control: cfg.Source.PrivateIngress(),
		Metadata: cfg.Source, Usage: cfg.Gateway,
		Rooms: cfg.Rooms, Authority: cfg.Authority, Journal: resources.Journal,
		BindPublicationContext: bindPublication,
		Isolated:               resources.Devices.Isolated, Legacy: resources.Devices.Legacy,
		RequireLegacy: resources.Devices.RequireLegacy, Invalid: ErrPrivateResources,
	}
	if resources.Devices.RequireLegacy {
		common.CopyFile = privateMail.CopyPrivateFile(func(ctx context.Context, target storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
			return resources.Devices.Legacy.MigrateStorageFileContentAt(ctx, resources.Devices.Isolated, cfg.Gateway, resources.Journal, job, target, ref, object, revision)
		})
	}
	holder, err := application.New(ctx, resources, common)
	if err != nil {
		return plan, err
	}
	plan.Repository = holder
	plan.Prepare = func(ctx context.Context, repository *MailApplicationRepository) error { return repository.Prepare(ctx) }
	plan.Retire = holder.Retire
	return plan, nil
}
