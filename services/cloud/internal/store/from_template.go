// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// TemplateOps son las escrituras que compone el alta de un proyecto desde una
// plantilla. Es la MISMA superficie que ya ofrece *Store (las cinco firmas son
// idénticas a sus métodos públicos), solo que atada a una transacción abierta.
//
// Existe porque el alta desde plantilla es todo-o-nada y los métodos públicos
// abren una transacción cada uno: encadenarlos deja proyectos huérfanos con
// servicios a medias en cuanto falla el tercer paso.
type TemplateOps interface {
	CreateProject(ctx context.Context, rec NewProjectRecord, maxPerOrg int) (domain.Project, error)
	ProvisionService(ctx context.Context, rec NewServiceRecord, maxManaged int) (domain.ManagedService, error)
	UpsertEnvVar(ctx context.Context, rec EnvVarRecord) (domain.EnvVar, error)
	EnqueueBuild(ctx context.Context, rec NewBuildRecord) (domain.Build, error)
	CreateDeployment(ctx context.Context, rec NewDeploymentRecord) (domain.Deployment, error)
}

// *Store cumple TemplateOps tal cual (cada método abre su propia transacción):
// quien no necesite atomicidad lo sigue usando igual que hasta ahora.
var _ TemplateOps = (*Store)(nil)

// templateTx ata las cinco operaciones a UNA transacción ya abierta, con el
// tenant resuelto una sola vez.
type templateTx struct {
	tx     pgx.Tx
	tenant string
}

var _ TemplateOps = templateTx{}

func (o templateTx) CreateProject(ctx context.Context, rec NewProjectRecord, maxPerOrg int) (domain.Project, error) {
	return createProjectTx(ctx, o.tx, o.tenant, rec, maxPerOrg)
}

func (o templateTx) ProvisionService(ctx context.Context, rec NewServiceRecord, maxManaged int) (domain.ManagedService, error) {
	return provisionServiceTx(ctx, o.tx, o.tenant, rec, maxManaged)
}

func (o templateTx) UpsertEnvVar(ctx context.Context, rec EnvVarRecord) (domain.EnvVar, error) {
	return upsertEnvVarTx(ctx, o.tx, o.tenant, rec)
}

func (o templateTx) EnqueueBuild(ctx context.Context, rec NewBuildRecord) (domain.Build, error) {
	return enqueueBuildTx(ctx, o.tx, o.tenant, rec)
}

func (o templateTx) CreateDeployment(ctx context.Context, rec NewDeploymentRecord) (domain.Deployment, error) {
	return createDeploymentTx(ctx, o.tx, o.tenant, rec)
}

// InTemplateTx ejecuta fn dentro de UNA transacción con `app.tenant` fijado
// (SET LOCAL vía InTenantTx). Si fn devuelve error, no se persiste NADA de lo
// que haya escrito: ni proyecto, ni servicios, ni variables, ni el despliegue.
func (s *Store) InTemplateTx(ctx context.Context, fn func(TemplateOps) error) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return fn(templateTx{tx: tx, tenant: t.Slug})
	})
}
