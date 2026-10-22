// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// fakeStore es una implementación en memoria de dataStore para ejercitar la
// lógica del Service sin PostgreSQL. Cada campo *Err fuerza un error concreto y
// los *Rec capturan la última entrada recibida para las aserciones.
type fakeStore struct {
	// audited son los eventos de auditoría registrados durante el test.
	audited []store.AuditEntry
	// memberCount son los miembros de la org: de ahí salen los puestos de suite.
	memberCount int
	org         domain.Organization
	orgErr      error
	updOrg      domain.Organization
	updOrgErr   error

	project       domain.Project
	projectErr    error
	newProj       *store.NewProjectRecord
	clonedFrom    uuid.UUID
	clonedTo      uuid.UUID
	clonedEnv     int
	clonedEnvErr  error
	maxPerOrg     int
	projects      []domain.Project
	updProj       *store.UpdateProjectRecord
	updProjErr    error
	delProjErr    error
	setInstallID  int64
	setInstallErr error

	build     domain.Build
	buildErr  error
	newBuild  *store.NewBuildRecord
	dep       domain.Deployment
	depErr    error
	newDep    *store.NewDeploymentRecord
	deps      []domain.Deployment
	promote   domain.Deployment
	promErr   error
	cancelErr error
	// Cancelación de la compilación asociada al despliegue.
	canceledBuild  *uuid.UUID
	cancelBuildErr error

	dom      domain.Domain
	domErr   error
	domHost  string
	domToken string
	doms     []domain.Domain
	rmDomErr error

	env       domain.EnvVar
	envErr    error
	envRec    *store.EnvVarRecord
	envs      []domain.EnvVar
	delEnvErr error

	svc         domain.ManagedService
	svcErr      error
	newSvc      *store.NewServiceRecord
	svcs        []domain.ManagedService
	delSvcErr   error
	reencolados []uuid.UUID
	// Lo último que pidió cambiar UpdateService, para poder afirmar que el
	// nombre y las claves llegan tal cual al store en vez de solo que no falla.
	svcCfgNombre *string
	svcCfg       map[string]string

	usageAgg []store.UsageAggregate
	usageErr error
	usageDet []domain.UsageRecord

	billingAcct domain.BillingAccount
	billingErr  error
	newInvoice  *domain.Invoice
	invoice     domain.Invoice
	invoiceErr  error
	invoices    []domain.Invoice
	setPlan     domain.Plan
	setLimit    *float64

	resolved    store.ResolvedRepo
	resolvedAll []store.ResolvedRepo

	activeBuilds, activePreviews int
	resolveErr                   error
	webhookDup                   bool
	webhookErr                   error
	webhookRepo                  uuid.UUID

	// authz: tokens de API y memberships.
	projSecretEnc []byte
	projSecretErr error
	apiTok        domain.APIToken
	apiTokErr     error
	newTok        *store.NewAPITokenRecord
	apiToks       []domain.APIToken
	delTokErr     error
	resolvedTok   store.ResolvedToken
	resolveTokErr error
	membership    domain.Membership
	membershipErr error
	upsertSub     string
	upsertRole    domain.Role
	memberships   []domain.Membership
	delMemberErr  error
	role          domain.Role
	roleErr       error
	invitations   []domain.MemberInvitation
	invitation    domain.MemberInvitation
	invitationErr error
	accepted      bool

	newAlert          *store.NewAlertPolicyRecord
	updAlert          *store.UpdateAlertPolicyRecord
	alertPolicy       domain.AlertPolicy
	alertErr          error
	confirmedAlertTok string
}

func (f *fakeStore) EnsureOrganization(_ context.Context, _ string, _ domain.Plan, _ string) (domain.Organization, error) {
	return f.org, f.orgErr
}

func (f *fakeStore) GetOrganization(context.Context) (domain.Organization, error) {
	return f.org, f.orgErr
}

// Puestos de suite derivados: los tests declaran cuántos miembros tiene la org.
func (f *fakeStore) CountMemberships(context.Context) (int, error) {
	return f.memberCount, nil
}

func (f *fakeStore) UpdateOrganization(_ context.Context, _ *string, _ *domain.Plan, _ *string) (domain.Organization, error) {
	return f.updOrg, f.updOrgErr
}

func (f *fakeStore) CreateProject(_ context.Context, rec store.NewProjectRecord, maxPerOrg int) (domain.Project, error) {
	f.newProj = &rec
	f.maxPerOrg = maxPerOrg
	return f.project, f.projectErr
}

func (f *fakeStore) GetProject(_ context.Context, _ uuid.UUID) (domain.Project, error) {
	return f.project, f.projectErr
}

func (f *fakeStore) CloneEnvVars(_ context.Context, from, to uuid.UUID) (int, error) {
	f.clonedFrom, f.clonedTo = from, to
	return f.clonedEnv, f.clonedEnvErr
}

func (f *fakeStore) ListProjects(context.Context) ([]domain.Project, error) {
	return f.projects, f.projectErr
}

func (f *fakeStore) UpdateProject(_ context.Context, _ uuid.UUID, upd store.UpdateProjectRecord) (domain.Project, error) {
	f.updProj = &upd
	return f.project, f.updProjErr
}
func (f *fakeStore) DeleteProject(context.Context, uuid.UUID) error { return f.delProjErr }
func (f *fakeStore) SetProjectInstallation(_ context.Context, _ uuid.UUID, installationID int64) error {
	f.setInstallID = installationID
	return f.setInstallErr
}

func (f *fakeStore) EnqueueBuild(_ context.Context, rec store.NewBuildRecord) (domain.Build, error) {
	f.newBuild = &rec
	return f.build, f.buildErr
}

func (f *fakeStore) GetBuild(context.Context, uuid.UUID) (domain.Build, error) {
	return f.build, f.buildErr
}

func (f *fakeStore) ListBuilds(context.Context, uuid.UUID, store.PageCursor, int) (store.BuildPage, error) {
	return store.BuildPage{}, f.buildErr
}

func (f *fakeStore) CreateDeployment(_ context.Context, rec store.NewDeploymentRecord) (domain.Deployment, error) {
	f.newDep = &rec
	return f.dep, f.depErr
}

func (f *fakeStore) GetDeployment(context.Context, uuid.UUID) (domain.Deployment, error) {
	return f.dep, f.depErr
}

func (f *fakeStore) ListDeployments(context.Context, uuid.UUID, store.PageCursor, int) (store.DeploymentPage, error) {
	return store.DeploymentPage{Deployments: f.deps}, f.depErr
}

func (f *fakeStore) GetActivePreview(_ context.Context, _ uuid.UUID, pr int) (domain.Deployment, error) {
	if f.depErr != nil {
		return domain.Deployment{}, f.depErr
	}
	for _, d := range f.deps {
		if d.Target == domain.TargetPreview && d.PRNumber != nil && *d.PRNumber == pr &&
			d.Status != domain.DeployRetired && d.Status != domain.DeployCanceled {
			return d, nil
		}
	}
	return domain.Deployment{}, domain.ErrNotFound
}

func (f *fakeStore) Promote(context.Context, uuid.UUID) (domain.Deployment, error) {
	return f.promote, f.promErr
}
func (f *fakeStore) CancelDeployment(context.Context, uuid.UUID) error { return f.cancelErr }
func (f *fakeStore) AddDomain(_ context.Context, _ uuid.UUID, hostname, verifyToken string) (domain.Domain, error) {
	f.domHost = hostname
	f.domToken = verifyToken
	return f.dom, f.domErr
}

func (f *fakeStore) ListDomains(context.Context, uuid.UUID) ([]domain.Domain, error) {
	return f.doms, f.domErr
}

func (f *fakeStore) RemoveDomain(_ context.Context, _ uuid.UUID) (domain.Domain, error) {
	return domain.Domain{}, f.rmDomErr
}

func (f *fakeStore) SetDomainPrimary(_ context.Context, _ uuid.UUID) (domain.Domain, error) {
	return f.dom, nil
}

// canceledBuild recuerda QUÉ compilación se mandó parar: sin esto no se puede afirmar
// que cancelar un despliegue deja de gastar minutos de compilación.
func (f *fakeStore) CancelBuild(_ context.Context, id uuid.UUID) (string, error) {
	f.canceledBuild = &id
	return "job-1", f.cancelBuildErr
}
func (f *fakeStore) SetBuildJobName(context.Context, string, uuid.UUID, string) error { return nil }
func (f *fakeStore) BuildStatus(context.Context, string, uuid.UUID) (domain.BuildStatus, error) {
	return domain.BuildQueued, nil
}

// audited recoge las acciones auditadas para poder afirmar QUÉ deja rastro.
func (f *fakeStore) RecordAudit(_ context.Context, e store.AuditEntry) error {
	f.audited = append(f.audited, e)
	return nil
}

func (f *fakeStore) ListAudit(context.Context, store.AuditFilter, store.PageCursor, int) (store.AuditPage, error) {
	return store.AuditPage{}, nil
}
func (f *fakeStore) ExportAudit(context.Context, store.AuditFilter, io.Writer) error { return nil }
func (f *fakeStore) RecordUsage(context.Context, store.UsageInput) error             { return nil }
func (f *fakeStore) UpsertEnvVar(_ context.Context, rec store.EnvVarRecord) (domain.EnvVar, error) {
	f.envRec = &rec
	return f.env, f.envErr
}

func (f *fakeStore) ListEnvVars(context.Context, uuid.UUID) ([]domain.EnvVar, error) {
	return f.envs, f.envErr
}
func (f *fakeStore) DeleteEnvVar(context.Context, uuid.UUID) error { return f.delEnvErr }
func (f *fakeStore) ActiveWork(context.Context, uuid.UUID, *int) (int, int, error) {
	return f.activeBuilds, f.activePreviews, nil
}

func (f *fakeStore) EnvVarScope(context.Context, uuid.UUID) (domain.EnvScope, error) {
	return domain.ScopeAll, f.delEnvErr
}

func (f *fakeStore) ProvisionService(_ context.Context, rec store.NewServiceRecord, _ int) (domain.ManagedService, error) {
	f.newSvc = &rec
	return f.svc, f.svcErr
}

func (f *fakeStore) GetService(context.Context, uuid.UUID) (domain.ManagedService, error) {
	return f.svc, f.svcErr
}

func (f *fakeStore) ListServices(context.Context, uuid.UUID) ([]domain.ManagedService, error) {
	return f.svcs, f.svcErr
}
func (f *fakeStore) DeleteService(context.Context, uuid.UUID) error { return f.delSvcErr }

func (f *fakeStore) UpdateServiceConfig(
	_ context.Context, _ uuid.UUID, name *string, config map[string]string,
) (domain.ManagedService, error) {
	f.svcCfgNombre, f.svcCfg = name, config
	return f.svc, f.svcErr
}

// reencolados guarda los ids que se devolvieron a 'provisioning': es lo que
// verifica que una integración alcanza a lo YA aprovisionado.
func (f *fakeStore) ReprovisionServices(_ context.Context, ids []uuid.UUID) error {
	f.reencolados = append(f.reencolados, ids...)
	return nil
}

func (f *fakeStore) SummarizeUsage(context.Context, time.Time, time.Time) ([]store.UsageAggregate, error) {
	return f.usageAgg, f.usageErr
}

func (f *fakeStore) ListUsage(context.Context, time.Time, time.Time, int) ([]domain.UsageRecord, error) {
	return f.usageDet, f.usageErr
}

func (f *fakeStore) GetOrCreateBillingAccount(context.Context) (domain.BillingAccount, error) {
	return f.billingAcct, f.billingErr
}

func (f *fakeStore) SetPlan(_ context.Context, plan domain.Plan) (domain.BillingAccount, error) {
	f.setPlan = plan
	f.billingAcct.Plan = plan
	return f.billingAcct, f.billingErr
}

func (f *fakeStore) SetSpendLimit(_ context.Context, limitEur *float64) (domain.BillingAccount, error) {
	f.setLimit = limitEur
	f.billingAcct.SpendLimitEur = limitEur
	return f.billingAcct, f.billingErr
}

func (f *fakeStore) CreateInvoice(_ context.Context, in domain.Invoice) (domain.Invoice, bool, error) {
	f.newInvoice = &in
	if f.invoiceErr != nil {
		return domain.Invoice{}, false, f.invoiceErr
	}
	// Idempotencia: si ya hay una factura "existente" cargada, devuélvela (simula
	// el ON CONFLICT DO NOTHING + SELECT del store); created=false → el llamador NO
	// no genera una segunda factura en un re-disparo del mismo periodo.
	if f.invoice.ID != uuid.Nil {
		return f.invoice, false, nil
	}
	return in, true, nil
}

func (f *fakeStore) GetInvoice(context.Context, uuid.UUID) (domain.Invoice, error) {
	return f.invoice, f.invoiceErr
}

func (f *fakeStore) ListInvoices(context.Context, time.Time, uuid.UUID, int) ([]domain.Invoice, error) {
	return f.invoices, f.invoiceErr
}

func (f *fakeStore) ResolveRepos(context.Context, string) ([]store.ResolvedRepo, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	if f.resolvedAll != nil {
		return f.resolvedAll, nil
	}
	return []store.ResolvedRepo{f.resolved}, nil
}

func (f *fakeStore) RecordWebhook(_ context.Context, _ string, projectID uuid.UUID, _, _, _ string, _ []byte) (bool, error) {
	f.webhookRepo = projectID
	return f.webhookDup, f.webhookErr
}

func (f *fakeStore) GetProjectWebhookSecretEnc(context.Context, uuid.UUID) ([]byte, error) {
	return f.projSecretEnc, f.projSecretErr
}

func (f *fakeStore) CreateAPIToken(_ context.Context, rec store.NewAPITokenRecord) (domain.APIToken, error) {
	f.newTok = &rec
	return f.apiTok, f.apiTokErr
}

func (f *fakeStore) ListAPITokens(context.Context) ([]domain.APIToken, error) {
	return f.apiToks, f.apiTokErr
}
func (f *fakeStore) DeleteAPIToken(context.Context, uuid.UUID) error { return f.delTokErr }
func (f *fakeStore) ResolveAPIToken(context.Context, string) (store.ResolvedToken, error) {
	return f.resolvedTok, f.resolveTokErr
}

func (f *fakeStore) ListMemberships(context.Context) ([]domain.Membership, error) {
	return f.memberships, f.membershipErr
}

func (f *fakeStore) UpsertMembership(_ context.Context, sub string, role domain.Role) (domain.Membership, error) {
	f.upsertSub, f.upsertRole = sub, role
	return f.membership, f.membershipErr
}

func (f *fakeStore) DeleteMembership(context.Context, string) error { return f.delMemberErr }

func (f *fakeStore) GetMembershipRole(context.Context, string) (domain.Role, error) {
	return f.role, f.roleErr
}

func (f *fakeStore) ListMemberInvitations(context.Context) ([]domain.MemberInvitation, error) {
	return f.invitations, f.invitationErr
}

func (f *fakeStore) UpsertMemberInvitation(context.Context, string, domain.Role, string, time.Time) (domain.MemberInvitation, error) {
	return f.invitation, f.invitationErr
}

func (f *fakeStore) DeleteMemberInvitation(context.Context, uuid.UUID) (domain.MemberInvitation, error) {
	return f.invitation, f.invitationErr
}

func (f *fakeStore) DeleteMemberInvitationByEmail(context.Context, string) error {
	return f.invitationErr
}

func (f *fakeStore) AcceptMemberInvitation(context.Context, string, string) (domain.Membership, bool, error) {
	return f.membership, f.accepted, f.invitationErr
}

func (f *fakeStore) CreateAlertPolicy(_ context.Context, rec store.NewAlertPolicyRecord) (domain.AlertPolicy, error) {
	f.newAlert = &rec
	if f.alertErr != nil {
		return domain.AlertPolicy{}, f.alertErr
	}
	p := f.alertPolicy
	p.Event = rec.Event
	p.ChannelURL = rec.ChannelURL
	p.Enabled = rec.Enabled
	p.Confirmed = rec.Confirmed
	return p, nil
}

func (f *fakeStore) UpdateAlertPolicy(_ context.Context, _ uuid.UUID, upd store.UpdateAlertPolicyRecord) (domain.AlertPolicy, error) {
	f.updAlert = &upd
	return f.alertPolicy, f.alertErr
}

func (f *fakeStore) ConfirmAlertPolicyByToken(_ context.Context, tokenHash string) (domain.AlertPolicy, error) {
	f.confirmedAlertTok = tokenHash
	if f.alertErr != nil {
		return domain.AlertPolicy{}, f.alertErr
	}
	if f.newAlert != nil && f.newAlert.ConfirmTokenHash != nil && *f.newAlert.ConfirmTokenHash == tokenHash {
		p := f.alertPolicy
		p.Confirmed = true
		return p, nil
	}
	return domain.AlertPolicy{}, domain.ErrNotFound
}

var _ dataStore = (*fakeStore)(nil)

// capturePub captura los eventos publicados (tipos de evento).
type capturePub struct {
	types []string
	err   error
}

func (c *capturePub) Publish(_ context.Context, ev events.Event) error {
	if c.err != nil {
		return c.err
	}
	c.types = append(c.types, ev.Type)
	return nil
}

var _ Publisher = (*capturePub)(nil)

var errBoom = errors.New("boom")
