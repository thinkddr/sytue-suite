// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/runtime"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// cloudService es la superficie mínima de la capa de aplicación que consumen los
// handlers HTTP. La satisface *service.Service (producción) y un fake en los
// tests, permitiendo ejercitar los handlers sin PostgreSQL. No cambia la firma
// pública de NewRouter (Deps sigue recibiendo el *service.Service concreto).
type cloudService interface {
	EnsureOrg(ctx context.Context, name string) (domain.Organization, error)
	UpdateOrg(ctx context.Context, name, notifyWebhookURL *string) (domain.Organization, error)
	CreateProject(ctx context.Context, in service.CreateProjectInput) (domain.Project, string, error)
	ProjectWebhookSecret(ctx context.Context, id uuid.UUID) (string, error)
	RotateProjectWebhookSecret(ctx context.Context, id uuid.UUID) (string, error)
	ProtectPreviews(ctx context.Context, id uuid.UUID) (string, error)
	UnprotectPreviews(ctx context.Context, id uuid.UUID) error
	WebhookURL() string
	PublicURL() string
	// AppCatalog es el catálogo de apps open-source, ya resuelto contra el
	// registro configurado: el handler no tiene por qué saber de dónde salen las
	// imágenes.
	AppCatalog() (domain.AppCatalog, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error)
	UpdateProject(ctx context.Context, id uuid.UUID, upd store.UpdateProjectRecord) (domain.Project, error)
	ProbeHealthPaths(ctx context.Context, id uuid.UUID) (service.HealthProbeReport, error)
	SuggestProjectSlug(ctx context.Context, lang string) (string, bool, error)
	CheckSlug(ctx context.Context, slug, lang string) (service.SlugCheck, error)
	ConnectGitHubApp(ctx context.Context, projectID uuid.UUID, installationID int64) error
	ConnectGitHubAppWithOwnership(ctx context.Context, projectID uuid.UUID, installationID int64, code string) error
	DisconnectGitHubApp(ctx context.Context, projectID uuid.UUID) error
	AuditExec(ctx context.Context, projectID, deploymentID uuid.UUID, duration time.Duration)
	StartGitHubInstall(ctx context.Context, projectID uuid.UUID, actorSub string) (string, error)
	ConsumeGitHubInstall(ctx context.Context, state, actorSub string) (uuid.UUID, error)
	DeleteProject(ctx context.Context, id uuid.UUID) error
	CloneProject(ctx context.Context, srcID uuid.UUID, slug, name string) (domain.Project, string, int, error)
	Deploy(ctx context.Context, in service.DeployInput) (domain.Deployment, domain.Build, error)
	ListDeployments(ctx context.Context, projectID uuid.UUID, cursor string, limit int) (service.DeploymentPage, error)
	GetDeployment(ctx context.Context, id uuid.UUID) (domain.Deployment, error)
	Rollback(ctx context.Context, deploymentID uuid.UUID) (domain.Deployment, error)
	PromotePreview(ctx context.Context, deploymentID uuid.UUID) (domain.Deployment, error)
	CancelDeployment(ctx context.Context, id uuid.UUID) error
	ListBuilds(ctx context.Context, projectID uuid.UUID, cursor string, limit int) (service.BuildPage, error)
	GetBuild(ctx context.Context, id uuid.UUID) (domain.Build, error)
	BuildLog(ctx context.Context, id uuid.UUID, q, level string, maxLines int) (string, error)
	AddDomain(ctx context.Context, projectID uuid.UUID, hostname string) (domain.Domain, error)
	ListDomains(ctx context.Context, projectID uuid.UUID) ([]domain.Domain, error)
	RemoveDomain(ctx context.Context, id uuid.UUID) error
	SetDomainPrimary(ctx context.Context, id uuid.UUID) (domain.Domain, error)
	CancelBuild(ctx context.Context, id uuid.UUID) error
	ListAudit(ctx context.Context, f store.AuditFilter, cursor string, limit int) (service.AuditPage, error)
	ExportAudit(ctx context.Context, f store.AuditFilter, w io.Writer) error
	StreamDeploymentLogs(ctx context.Context, projectID, deploymentID uuid.UUID, w io.Writer) error
	SearchLogs(ctx context.Context, projectID uuid.UUID, f store.RuntimeLogFilter, cursor string, limit int) (service.LogPage, error)
	ExportLogs(ctx context.Context, projectID uuid.UUID, f store.RuntimeLogFilter, w io.Writer) error
	ListStorageObjects(ctx context.Context, projectID uuid.UUID, prefix string) ([]service.StorageObject, error)
	PresignStorageURL(ctx context.Context, projectID uuid.UUID, key, method string, expiry time.Duration) (string, error)
	DeleteStorageObject(ctx context.Context, projectID uuid.UUID, key string) error
	ProjectStorage(ctx context.Context, projectID uuid.UUID) (service.StorageInfo, error)
	IngressTarget() string
	SetEnv(ctx context.Context, in service.SetEnvInput) (domain.EnvVar, error)
	ListEnv(ctx context.Context, projectID uuid.UUID) ([]domain.EnvVar, error)
	ExportEnv(ctx context.Context, projectID uuid.UUID, target domain.DeployTarget) (string, error)
	DeleteEnv(ctx context.Context, id uuid.UUID) error
	EnvScope(ctx context.Context, id uuid.UUID) (domain.EnvScope, error)
	ProvisionService(ctx context.Context, in service.ProvisionServiceInput) (domain.ManagedService, error)
	ListServices(ctx context.Context, projectID uuid.UUID) ([]domain.ManagedService, error)
	GetService(ctx context.Context, id uuid.UUID) (domain.ManagedService, error)
	UpdateService(ctx context.Context, in service.UpdateServiceInput) (domain.ManagedService, error)
	DeleteService(ctx context.Context, id uuid.UUID) error
	ListServiceBackups(ctx context.Context, serviceID uuid.UUID) ([]service.Backup, error)
	RestoreServiceBackup(ctx context.Context, serviceID uuid.UUID, backupName string) error
	BackupServiceNow(ctx context.Context, serviceID uuid.UUID) error
	RunServiceNow(ctx context.Context, serviceID uuid.UUID) error
	DownloadServiceBackupURL(ctx context.Context, serviceID uuid.UUID, backupName string) (string, error)
	ServiceLogs(ctx context.Context, serviceID uuid.UUID, lines int) ([]domain.LogLine, error)
	RestartApp(ctx context.Context, projectID uuid.UUID) (int, error)
	QuotaStatus(ctx context.Context) (service.QuotaReport, error)
	UsageSummary(ctx context.Context, from, to time.Time) ([]store.UsageAggregate, error)
	UsageDetail(ctx context.Context, from, to time.Time, limit int) ([]domain.UsageRecord, error)
	BillingAccount(ctx context.Context) (domain.BillingAccount, error)
	SetSpendLimit(ctx context.Context, limitEur *float64) (domain.BillingAccount, error)
	CurrentSpend(ctx context.Context) (float64, error)
	GenerateInvoice(ctx context.Context, from, to time.Time) (domain.Invoice, error)
	GetInvoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error)
	ListInvoices(ctx context.Context, afterPeriod time.Time, afterID uuid.UUID, limit int) ([]domain.Invoice, error)
	HandleGitEvent(ctx context.Context, rr store.ResolvedRepo, ev domain.GitEvent, deliveryID string, raw []byte) (*domain.Deployment, bool, error)
	VerifyWebhook(ctx context.Context, repoURL string, body []byte, githubSig, githubSigLegacy, gitlabToken string) (store.ResolvedRepo, error)
	CreateAPIToken(ctx context.Context, in service.CreateTokenInput) (domain.APIToken, error)
	ListAPITokens(ctx context.Context) ([]domain.APIToken, error)
	RevokeAPIToken(ctx context.Context, id uuid.UUID) error
	ResolveAPIToken(ctx context.Context, plaintext string) (store.ResolvedToken, error)
	ListMembers(ctx context.Context) ([]domain.Membership, error)
	AddMember(ctx context.Context, sub string, role domain.Role) (domain.Membership, error)
	RemoveMember(ctx context.Context, sub string) error
	EffectiveRole(ctx context.Context, sub string) (domain.Role, bool, error)
	ListMemberInvitations(ctx context.Context) ([]domain.MemberInvitation, error)
	InviteMember(ctx context.Context, bearer, email string, role domain.Role, invitedBy string) (service.InviteMemberResult, error)
	RevokeMemberInvitation(ctx context.Context, bearer string, id uuid.UUID) error
	AcceptPendingMember(ctx context.Context, sub, email string) (bool, error)
	CreateUptimeCheck(ctx context.Context, in service.CreateUptimeCheckInput) (domain.UptimeCheck, error)
	ListUptimeChecks(ctx context.Context, projectID uuid.UUID) ([]domain.UptimeCheck, error)
	GetUptimeCheck(ctx context.Context, id uuid.UUID) (domain.UptimeCheck, error)
	UpdateUptimeCheck(ctx context.Context, id uuid.UUID, upd store.UpdateUptimeCheckRecord) (domain.UptimeCheck, error)
	DeleteUptimeCheck(ctx context.Context, id uuid.UUID) error
	CreateAlertPolicy(ctx context.Context, in service.CreateAlertPolicyInput) (domain.AlertPolicy, error)
	ListAlertPolicies(ctx context.Context) ([]domain.AlertPolicy, error)
	UpdateAlertPolicy(ctx context.Context, id uuid.UUID, upd store.UpdateAlertPolicyRecord) (domain.AlertPolicy, error)
	DeleteAlertPolicy(ctx context.Context, id uuid.UUID) error
}

// Handlers agrupa los manejadores de la API REST de Sytue Cloud.
type Handlers struct {
	svc         cloudService
	cfg         config.Config
	execSession runtime.ExecSession
}

var _ cloudService = (*service.Service)(nil)

// toAPIError traduce errores de dominio a *httpx.Error canónicos.
func toAPIError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict.WithMessage("A resource with that name already exists.")
	case errors.Is(err, domain.ErrDomainTaken):
		return httpx.ErrConflict.WithMessage("That domain is already in use by another project.")
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest.WithMessage(messageOf(err, "Entrada inválida."))
	case errors.Is(err, domain.ErrInvalidState):
		return httpx.ErrConflict.WithMessage("That operation is not valid in the resource's current state.")
	case errors.Is(err, domain.ErrIdentityUnavailable):
		return httpx.ErrUnavailable.WithMessage("Sytue ID is unavailable; try again.")
	case errors.Is(err, domain.ErrQuotaExceeded):
		return httpx.NewError(http.StatusPaymentRequired, "quota_exceeded", "You have reached the limit of your plan.")
	case errors.Is(err, domain.ErrQueueFull):
		return httpx.ErrTooManyRequests.WithMessage("There is already too much work queued; wait for it to finish.")
	case errors.Is(err, domain.ErrSpendLimitExceeded):
		return httpx.NewError(http.StatusPaymentRequired, "spend_limit_exceeded", "You have gone over your spending limit for this month.")
	case errors.Is(err, domain.ErrSuiteRequired):
		return httpx.NewError(http.StatusPaymentRequired, "suite_subscription_required",
			"Sytue Cloud requiere una suscripción a la suite Sytue (Mail, Drive, Docs y Rooms), 6 € por usuario y mes.")
	case errors.Is(err, domain.ErrPermission):
		return httpx.ErrForbidden
	case errors.Is(err, domain.ErrSignatureInvalid):
		return httpx.ErrUnauthorized.WithMessage("Invalid webhook signature.")
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}

// messageOf extrae el texto de un error de dominio envuelto (para %w mensajes).
func messageOf(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	msg := err.Error()
	// Los errores de validación se construyen como "<dominio>: <detalle>"; nos
	// quedamos con el detalle legible si lo hay.
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
		return capitalize(msg[i+2:]) + "."
	}
	return fallback
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// parseID extrae y valida el UUID del parámetro de ruta "id".
func parseID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid identifier.")
	}
	return id, nil
}

// subFromCtx devuelve el subject del usuario autenticado.
func subFromCtx(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok {
		return c.Subject
	}
	return ""
}

// ---------------------------------------------------------------------------
// Organización
// ---------------------------------------------------------------------------

// GetOrg devuelve la organización del tenant (creándola si es la primera vez).
func (h *Handlers) GetOrg(w http.ResponseWriter, r *http.Request) {
	org, err := h.svc.EnsureOrg(r.Context(), "")
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, org)
}

type updateOrgRequest struct {
	Name             *string `json:"name,omitempty"`
	Plan             *string `json:"plan,omitempty"`
	NotifyWebhookURL *string `json:"notify_webhook_url,omitempty"`
}

// UpdateOrg actualiza el nombre y la URL de aviso de la organización (admin/owner).
func (h *Handlers) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	var req updateOrgRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// El plan no se elige aquí: cambia tras un pago confirmado o lo cambia el
	// operador (`cloud-api plan`). Antes un admin se ponía `team` y sus cuotas
	// sin pasar por el cobro (decisión del dueño, 2026-10-17).
	if req.Plan != nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The plan changes after a confirmed payment or by the operator."))
		return
	}
	org, err := h.svc.UpdateOrg(r.Context(), req.Name, req.NotifyWebhookURL)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, org)
}

// ---------------------------------------------------------------------------
// Proyectos
// ---------------------------------------------------------------------------

type createProjectRequest struct {
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	RepoURL       string `json:"repo_url"`
	GitProvider   string `json:"git_provider"`
	ProductionRef string `json:"production_ref"`
	Framework     string `json:"framework"`
	BuildStrategy string `json:"build_strategy"`
	RootDir       string `json:"root_dir"`
	BuildCommand  string `json:"build_command"`
	OutputDir     string `json:"output_dir"`
	InstallCmd    string `json:"install_command"`
	// AppPort / HealthPath: 0 y "" = defaults (8080 y "/").
	AppPort    int    `json:"app_port"`
	HealthPath string `json:"health_path"`
	// InstanceSize / InstanceCount: "" y 0 = defaults ('basic' y 1).
	InstanceSize  string `json:"instance_size"`
	InstanceCount int    `json:"instance_count"`
	// Autoescalado: 0 = defaults (techo automático 5× las instancias y 75 % de CPU).
	AutoscaleMaxCount  int `json:"autoscale_max_count"`
	AutoscaleCPUTarget int `json:"autoscale_cpu_target"`
}

// projectWithSecret embebe el proyecto y añade el secreto de webhook + la URL del
// endpoint. Los campos del proyecto se promueven al nivel superior (embed sin tag
// json), de modo que la forma de respuesta no cambia respecto a devolver el
// proyecto pelado; solo se AÑADEN webhook_secret/webhook_url cuando procede.
type projectWithSecret struct {
	domain.Project
	WebhookSecret string `json:"webhook_secret,omitempty"`
	WebhookURL    string `json:"webhook_url,omitempty"`
}

// CreateProject crea un proyecto y devuelve, UNA sola vez, el secreto del webhook
// git del proyecto (para pegarlo en el proveedor) y la URL del endpoint.
func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, secret, err := h.svc.CreateProject(r.Context(), service.CreateProjectInput{
		Slug:               strings.TrimSpace(req.Slug),
		Name:               strings.TrimSpace(req.Name),
		RepoURL:            req.RepoURL,
		GitProvider:        domain.GitProvider(req.GitProvider),
		ProductionRef:      req.ProductionRef,
		Framework:          domain.Framework(req.Framework),
		BuildStrategy:      domain.BuildStrategy(req.BuildStrategy),
		RootDir:            req.RootDir,
		BuildCommand:       req.BuildCommand,
		OutputDir:          req.OutputDir,
		InstallCmd:         req.InstallCmd,
		AppPort:            req.AppPort,
		HealthPath:         strings.TrimSpace(req.HealthPath),
		InstanceSize:       strings.TrimSpace(req.InstanceSize),
		InstanceCount:      req.InstanceCount,
		AutoscaleMaxCount:  req.AutoscaleMaxCount,
		AutoscaleCPUTarget: req.AutoscaleCPUTarget,
		OwnerSub:           subFromCtx(r),
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// El secreto del webhook se revela al HUMANO que crea el proyecto (para pegarlo
	// en el proveedor git), NUNCA a un token de API: un token de scope write —que
	// por diseño no puede desplegar (requireScope(ScopeDeploy))— podría leerlo aquí
	// y firmar un push contra el endpoint público /webhooks/git para disparar un
	// deploy, escalando write→deploy. Espeja la regla de GetProject (token => sin
	// secreto); el humano ya puede desplegar por API, así que no gana capacidad.
	resp := projectWithSecret{Project: p, WebhookURL: h.svc.WebhookURL()}
	if tokenFromCtx(r.Context()) == nil {
		resp.WebhookSecret = secret
	}
	httpx.JSON(w, r, http.StatusCreated, resp)
}

// ListProjects lista los proyectos del tenant.
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := h.svc.ListProjects(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if ps == nil {
		ps = []domain.Project{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"projects": ps})
}

// GetProject devuelve un proyecto por ID.
func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := h.svc.GetProject(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// El secreto del webhook (para pegar en el proveedor git) se revela solo a un
	// usuario admin/owner; nunca a un token de API ni a un miembro raso.
	if h.humanHasRole(r, "admin", "owner") {
		if secret, sErr := h.svc.ProjectWebhookSecret(r.Context(), id); sErr == nil && secret != "" {
			httpx.JSON(w, r, http.StatusOK, projectWithSecret{
				Project: p, WebhookSecret: secret, WebhookURL: h.svc.WebhookURL(),
			})
			return
		}
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

type updateProjectRequest struct {
	Name             *string `json:"name,omitempty"`
	RepoURL          *string `json:"repo_url,omitempty"`
	ProductionRef    *string `json:"production_ref,omitempty"`
	Framework        *string `json:"framework,omitempty"`
	BuildStrategy    *string `json:"build_strategy,omitempty"`
	RootDir          *string `json:"root_dir,omitempty"`
	BuildCommand     *string `json:"build_command,omitempty"`
	OutputDir        *string `json:"output_dir,omitempty"`
	InstallCmd       *string `json:"install_command,omitempty"`
	NotifyWebhookURL *string `json:"notify_webhook_url,omitempty"`
	AppPort          *int    `json:"app_port,omitempty"`
	HealthPath       *string `json:"health_path,omitempty"`
	InstanceSize     *string `json:"instance_size,omitempty"`
	InstanceCount    *int    `json:"instance_count,omitempty"`
	// Autoescalado: techo (0 = automático) y objetivo de CPU (0 = vuelve al 75 %).
	AutoscaleMaxCount  *int `json:"autoscale_max_count,omitempty"`
	AutoscaleCPUTarget *int `json:"autoscale_cpu_target,omitempty"`
	// ReleaseCommand se ejecuta antes de arrancar la versión nueva (migraciones).
	// Cadena vacía = quitarlo.
	ReleaseCommand *string `json:"release_command,omitempty"`
	// Paused apaga el proyecto sin borrarlo (cero réplicas, cero factura).
	Paused *bool `json:"paused,omitempty"`
	// HTTPRules sustituye ENTERAS las redirecciones y cabeceras del borde.
	HTTPRules *domain.HTTPRules `json:"http_rules,omitempty"`
	DeployKey *string           `json:"deploy_key,omitempty"`
}

// validarCambiosDeProyecto comprueba los campos opcionales del PATCH. Son ocho
// bloques con la misma forma —«si viene, valídalo»— y en línea convertían el
// handler en un muro donde no se veía lo que de verdad hace: leer, validar,
// guardar. Devuelve el primer error; el handler lo traduce a 400.
func validarCambiosDeProyecto(req updateProjectRequest) error {
	comprobar := []func() error{
		func() error { return siViene(req.AppPort, domain.ValidateAppPort) },
		func() error { return siViene(req.HealthPath, domain.ValidateHealthPath) },
		func() error { return siViene(req.InstanceSize, domain.ValidateInstanceSize) },
		func() error { return siViene(req.InstanceCount, domain.ValidateInstanceCount) },
		func() error { return siViene(req.AutoscaleMaxCount, domain.ValidateAutoscaleMaxCount) },
		func() error { return siViene(req.AutoscaleCPUTarget, domain.ValidateAutoscaleCPUTarget) },
		func() error { return siViene(req.ReleaseCommand, domain.ValidateReleaseCommand) },
	}
	for _, c := range comprobar {
		if err := c(); err != nil {
			return err
		}
	}
	return nil
}

// siViene aplica la validación solo si el campo llegó en el PATCH. Un puntero
// nil significa «no lo toques», que no es lo mismo que un valor vacío.
func siViene[T any](v *T, valida func(T) error) error {
	if v == nil {
		return nil
	}
	return valida(*v)
}

// redefineEjecucion indica si la petición toca alguno de los campos que deciden
// QUÉ CÓDIGO SE EJECUTA o A DÓNDE VA EL TRÁFICO PÚBLICO del proyecto. Es la
// lista que gobierna canRedefineExecution; el resto de campos (nombre, puerto,
// ruta de salud, tamaño, réplicas, autoescalado, pausa, webhook de aviso) siguen
// abiertos a cualquier miembro, que es como se configura un proyecto a diario.
//
// Al añadir un campo nuevo a updateProjectRequest hay que decidir de qué lado
// cae. Si define procedencia del código, cómo se construye, qué se ejecuta o a
// dónde se redirige a un visitante, va aquí.
func (q updateProjectRequest) redefineEjecucion() bool {
	return q.RepoURL != nil || q.ProductionRef != nil || q.DeployKey != nil ||
		q.RootDir != nil || q.InstallCmd != nil || q.BuildCommand != nil ||
		q.OutputDir != nil || q.ReleaseCommand != nil ||
		q.Framework != nil || q.BuildStrategy != nil ||
		q.HTTPRules != nil
}

// UpdateProject aplica una actualización parcial de la configuración.
func (h *Handlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateProjectRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// La verja va por CAMPO, no por ruta: la ruta entera en admin/owner rompería
	// el camino corriente —un `member` ajustando el tamaño que le recomienda
	// `GET /projects/{id}/sizing`, que sí puede ver—. Ver canRedefineExecution
	// para el porqué de la lista.
	if req.redefineEjecucion() && !h.canRedefineExecution(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage(
			"Changing the repository, build/release commands or edge redirects requires an admin or owner, or a token with deploy scope.",
		))
		return
	}
	upd := store.UpdateProjectRecord{
		Name: req.Name, RepoURL: req.RepoURL, ProductionRef: req.ProductionRef,
		RootDir: req.RootDir, BuildCommand: req.BuildCommand,
		OutputDir: req.OutputDir, InstallCmd: req.InstallCmd,
		NotifyWebhookURL: req.NotifyWebhookURL, DeployKey: req.DeployKey,
		AppPort: req.AppPort, HealthPath: req.HealthPath,
		InstanceSize: req.InstanceSize, InstanceCount: req.InstanceCount,
		AutoscaleMaxCount: req.AutoscaleMaxCount,
		ReleaseCommand:    req.ReleaseCommand,
		Paused:            req.Paused,
		HTTPRules:         req.HTTPRules,
	}
	// Validación ANTES de tocar la BD: el CHECK de la migración es la última red,
	// pero un 400 explicativo es mejor que un error de constraint.
	if err := validarCambiosDeProyecto(req); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if req.AutoscaleCPUTarget != nil {
		// 0 significa «vuelve al defecto», y el CHECK de la columna no admite 0
		// (a diferencia del techo, donde 0 ES un valor válido: automático). Se
		// traduce aquí para que el reset no salga como un 400 de constraint.
		v := *req.AutoscaleCPUTarget
		if v == 0 {
			v = domain.DefaultCPUTarget
		}
		upd.AutoscaleCPUTarget = &v
	}
	if req.Framework != nil {
		f := domain.Framework(*req.Framework)
		upd.Framework = &f
	}
	if req.BuildStrategy != nil {
		bs := domain.BuildStrategy(*req.BuildStrategy)
		upd.BuildStrategy = &bs
	}
	p, err := h.svc.UpdateProject(r.Context(), id, upd)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

type connectGitHubRequest struct {
	InstallationID int64 `json:"installation_id"`
}

// ConnectGitHubApp conecta una instalación de la GitHub App al proyecto para
// clonar su repo privado sin deploy-key manual y comentar la preview en el PR.
func (h *Handlers) ConnectGitHubApp(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req connectGitHubRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.ConnectGitHubApp(r.Context(), id, req.InstallationID); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"connected": true, "installation_id": req.InstallationID})
}

// DisconnectGitHubApp desconecta la GitHub App del proyecto.
func (h *Handlers) DisconnectGitHubApp(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DisconnectGitHubApp(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// DeleteProject borra un proyecto (admin/owner).
func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteProject(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Deployments + Builds
// ---------------------------------------------------------------------------

type createDeploymentRequest struct {
	CommitSHA string `json:"commit_sha"`
	GitRef    string `json:"git_ref"`
	Target    string `json:"target"`
	PRNumber  *int   `json:"pr_number,omitempty"`
	ImageRef  string `json:"image_ref,omitempty"`
}

// CreateDeployment dispara un despliegue manual (build + deploy) de un proyecto.
func (h *Handlers) CreateDeployment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createDeploymentRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target := domain.DeployTarget(req.Target)
	if target == "" {
		target = domain.TargetProduction
	}
	// Una preview la lanza un member; producción, solo admin/owner (CLOUD-04).
	if target != domain.TargetPreview && !h.canDeployProduction(r) {
		failProductionDeploy(w, r)
		return
	}
	dep, build, err := h.svc.Deploy(r.Context(), service.DeployInput{
		ProjectID: id,
		CommitSHA: req.CommitSHA,
		GitRef:    req.GitRef,
		Target:    target,
		PRNumber:  req.PRNumber,
		ImageRef:  req.ImageRef,
		CreatedBy: subFromCtx(r),
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"deployment": dep, "build": build})
}

// ListDeployments lista los deployments de un proyecto con paginación keyset
// (?cursor=<opaco>&limit=N). Devuelve `next_cursor` cuando hay más resultados;
// un cursor vacío o ausente pide la primera página.
func (h *Handlers) ListDeployments(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	page, err := h.svc.ListDeployments(r.Context(), id, queryCursor(r), queryLimit(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	deps := page.Deployments
	if deps == nil {
		deps = []domain.Deployment{}
	}
	resp := map[string]any{"deployments": deps}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// GetDeployment devuelve un deployment por ID.
func (h *Handlers) GetDeployment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dep, err := h.svc.GetDeployment(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, dep)
}

// Rollback promociona un deployment de producción anterior.
func (h *Handlers) Rollback(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dep, err := h.svc.Rollback(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, dep)
}

// CloneProject crea un proyecto igual que otro (configuración + variables).
//
// Devuelve el secreto de webhook del proyecto NUEVO en la respuesta, igual que el
// alta normal: es la única vez que se puede leer en claro.
func (h *Handlers) CloneProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	proj, secret, copied, err := h.svc.CloneProject(r.Context(), id, strings.TrimSpace(req.Slug), strings.TrimSpace(req.Name))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"project": proj, "webhook_secret": secret, "env_vars_copied": copied,
	})
}

// PromotePreview publica en producción la imagen de una preview.
//
// Responde 201 y no 200: a diferencia del rollback, esto CREA un deployment
// nuevo —el que va a servir producción— y el cliente necesita su id para
// seguirlo.
func (h *Handlers) PromotePreview(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dep, err := h.svc.PromotePreview(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, dep)
}

// CancelDeployment cancela un deployment vivo.
func (h *Handlers) CancelDeployment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.CancelDeployment(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ListBuilds lista las builds de un proyecto con paginación keyset
// (?cursor=<opaco>&limit=N). Devuelve `next_cursor` cuando hay más resultados;
// un cursor vacío o ausente pide la primera página.
func (h *Handlers) ListBuilds(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	page, err := h.svc.ListBuilds(r.Context(), id, queryCursor(r), queryLimit(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	bs := page.Builds
	if bs == nil {
		bs = []domain.Build{}
	}
	resp := map[string]any{"builds": bs}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// GetBuild devuelve una build por ID.
func (h *Handlers) GetBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	b, err := h.svc.GetBuild(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, b)
}

// GetBuildLogs devuelve el log de una build como TEXTO PLANO. Lo lee del Object
// Storage por logs_key bajo el tenant del contexto (RLS: GetBuild valida la
// pertenencia). Filtrado server-side opcional: ?q= (subcadena, case-insensitive),
// ?level= (nivel de log si la línea lo lleva) y ?limit= (tope de líneas). 404 "aún
// no disponible" si la build no existe, el log no está listo (build en curso /
// logs_key vacío) o el objeto no está en S3. Lo consume el LogsPanel vía el BFF.
func (h *Handlers) GetBuildLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	query := r.URL.Query()
	limit := 0
	if v := query.Get("limit"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil {
			limit = n
		}
	}
	logs, err := h.svc.BuildLog(r.Context(), id, query.Get("q"), query.Get("level"), limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, logs)
}

// ---------------------------------------------------------------------------
// Dominios
// ---------------------------------------------------------------------------

type addDomainRequest struct {
	Hostname string `json:"hostname"`
}

// AddDomain asocia un dominio personalizado a un proyecto.
func (h *Handlers) AddDomain(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req addDomainRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	d, err := h.svc.AddDomain(r.Context(), id, req.Hostname)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Instrucciones de configuración DNS para el cliente.
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"domain": d,
		"dns": map[string]any{
			"cname_target": h.svc.IngressTarget(),
			"txt_record":   "_sytue-challenge." + d.Hostname,
			"txt_value":    d.VerifyToken,
		},
	})
}

// ListDomains lista los dominios de un proyecto.
func (h *Handlers) ListDomains(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ds, err := h.svc.ListDomains(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if ds == nil {
		ds = []domain.Domain{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"domains": ds})
}

// RemoveDomain elimina un dominio de un proyecto.
func (h *Handlers) RemoveDomain(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.RemoveDomain(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Variables de entorno / secretos
// ---------------------------------------------------------------------------

type setEnvRequest struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"is_secret"`
	Scope    string `json:"scope"`
}

// SetEnv crea/actualiza una variable de entorno o secreto.
func (h *Handlers) SetEnv(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req setEnvRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Las variables de producción son con las que arranca producción: cambiarlas
	// es desplegar (CLOUD-04). Las de preview las toca un member o un token write.
	if domain.EnvScope(req.Scope) != domain.ScopePreview && !h.canDeployProduction(r) {
		failProductionDeploy(w, r)
		return
	}
	v, err := h.svc.SetEnv(r.Context(), service.SetEnvInput{
		ProjectID: id,
		Key:       strings.TrimSpace(req.Key),
		Value:     req.Value,
		IsSecret:  req.IsSecret,
		Scope:     domain.EnvScope(req.Scope),
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, v)
}

// ListEnv lista las variables de un proyecto (valores de secretos ocultos).
func (h *Handlers) ListEnv(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	vs, err := h.svc.ListEnv(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if vs == nil {
		vs = []domain.EnvVar{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"env": vs})
}

// ExportEnv descarga las variables del proyecto como fichero `.env`.
//
// Va por su propia ruta y no como formato de `GET /env` porque el listado lo ve
// cualquier miembro (con los secretos ocultos) y esto los revela: son dos
// permisos distintos y tienen que seguir siéndolo.
func (h *Handlers) ExportEnv(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	target := domain.TargetProduction
	if r.URL.Query().Get("target") == string(domain.TargetPreview) {
		target = domain.TargetPreview
	}
	body, err := h.svc.ExportEnv(r.Context(), id, target)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// `.env.production` / `.env.preview`: el nombre dice a qué entorno pertenece,
	// que es lo primero que hay que saber para no arrancar local con lo que no es.
	w.Header().Set("Content-Disposition", `attachment; filename=".env.`+string(target)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, body)
}

// DeleteEnv borra una variable.
func (h *Handlers) DeleteEnv(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	scope, err := h.svc.EnvScope(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if scope != domain.ScopePreview && !h.canDeployProduction(r) {
		failProductionDeploy(w, r)
		return
	}
	if err := h.svc.DeleteEnv(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Servicios gestionados
// ---------------------------------------------------------------------------

type provisionServiceRequest struct {
	Kind   string            `json:"kind"`
	Name   string            `json:"name"`
	Config map[string]string `json:"config,omitempty"`
	// Target: "production" (por defecto) o "preview". Un servicio de vista
	// previa entrega sus credenciales SOLO a las vistas previas del proyecto.
	Target string `json:"target,omitempty"`
}

// ProvisionService aprovisiona un servicio gestionado para un proyecto.
func (h *Handlers) ProvisionService(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req provisionServiceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	kind := domain.ServiceKind(req.Kind)
	target := domain.DeployTarget(strings.TrimSpace(req.Target))
	// Un servicio con comando ejecuta código con las credenciales del proyecto en
	// cuanto se crea (un job corre en el acto, un worker arranca solo): en
	// producción es desplegar, y lo hacen admin/owner (CLOUD-04).
	if domain.ServiceRunsCommand(kind) && target != domain.TargetPreview && !h.canDeployProduction(r) {
		failProductionDeploy(w, r)
		return
	}
	m, err := h.svc.ProvisionService(r.Context(), service.ProvisionServiceInput{
		ProjectID: id,
		Kind:      kind,
		Name:      strings.TrimSpace(req.Name),
		Config:    req.Config,
		Target:    target,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, m)
}

// ListServices lista los servicios gestionados de un proyecto.
func (h *Handlers) ListServices(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ms, err := h.svc.ListServices(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if ms == nil {
		ms = []domain.ManagedService{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"services": ms})
}

// GetService devuelve un servicio gestionado por ID.
func (h *Handlers) GetService(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m, err := h.svc.GetService(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, m)
}

// UpdateService cambia el nombre y/o la configuración de un servicio gestionado.
//
// El `config` que llega se MEZCLA con el que hay: mandar el objeto entero desde
// el cliente borraría lo que escribió el aprovisionamiento (host, bucket, URL
// pública), que es información que el panel necesita y el usuario no tiene.
func (h *Handlers) UpdateService(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req struct {
		Name   *string           `json:"name,omitempty"`
		Config map[string]string `json:"config,omitempty"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Name == nil && len(req.Config) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("No changes requested."))
		return
	}
	m, err := h.svc.UpdateService(r.Context(), service.UpdateServiceInput{
		ServiceID: id, Name: req.Name, Config: req.Config,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// 202: el cambio queda guardado y el servicio vuelve a la cola; el clúster lo
	// recoge en el siguiente ciclo del reconciliador.
	httpx.JSON(w, r, http.StatusAccepted, m)
}

// DeleteService marca un servicio gestionado para eliminación (admin/owner).
func (h *Handlers) DeleteService(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteService(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ListServiceBackups lista los backups de un servicio Sytue Postgres. Lectura bajo
// RLS (GetService valida la pertenencia al tenant → aislamiento) del prefijo S3
// propio del servicio; el cliente no controla el prefijo. 409 si el servicio no es
// postgres.
func (h *Handlers) ListServiceBackups(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bs, err := h.svc.ListServiceBackups(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if bs == nil {
		bs = []service.Backup{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"backups": bs})
}

type restoreServiceRequest struct {
	Backup string `json:"backup"`
}

// RestoreServiceBackup restaura un backup elegido sobre el Postgres del servicio
// (destructivo; admin/owner + scope deploy). El cuerpo lleva SOLO el nombre de
// fichero del backup; el objeto S3 se compone en servidor. 400 nombre inválido, 404
// servicio/backup, 409 servicio no-postgres.
func (h *Handlers) RestoreServiceBackup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req restoreServiceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.RestoreServiceBackup(r.Context(), id, strings.TrimSpace(req.Backup)); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"restoring": true, "backup": req.Backup})
}

// ---------------------------------------------------------------------------
// Uso / facturación
// ---------------------------------------------------------------------------

// GetUsage devuelve el uso agregado del tenant en el periodo (?from=&to=).
func (h *Handlers) GetUsage(w http.ResponseWriter, r *http.Request) {
	from, to := periodFromQuery(r)
	agg, err := h.svc.UsageSummary(r.Context(), from, to)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if agg == nil {
		agg = []store.UsageAggregate{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"usage": agg, "from": from, "to": to})
}

// GetUsageDetail lista los registros de uso del tenant en el periodo.
func (h *Handlers) GetUsageDetail(w http.ResponseWriter, r *http.Request) {
	from, to := periodFromQuery(r)
	recs, err := h.svc.UsageDetail(r.Context(), from, to, queryLimit(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if recs == nil {
		recs = []domain.UsageRecord{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"records": recs})
}

// ---------------------------------------------------------------------------
// Webhooks git
// ---------------------------------------------------------------------------

const maxWebhookBody = 2 << 20 // 2 MiB

// GitWebhook recibe un webhook de push/PR, valida la firma, normaliza el evento
// y dispara build+deploy (producción o preview por PR). No usa auth de Sytue ID.
func (h *Handlers) GitWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The webhook body is too large or could not be read."))
		return
	}

	provider, eventType, deliveryID := webhookHeaders(r)
	if provider == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unrecognised webhook provider."))
		return
	}

	// Parseamos primero para conocer el repo destino y, con él, resolver el
	// secreto DEL PROYECTO con el que verificar la firma. El repo del payload no
	// se "confía" antes de verificar: solo elige el secreto; sin ese secreto por
	// proyecto, un cuerpo falsificado no produce una firma válida (cierra la
	// falsificación cross-tenant del secreto global).
	ev, err := service.ParseWebhook(provider, eventType, body)
	if err != nil {
		// Eventos ignorados (no push/PR) responden 200 para no reintentar.
		if errors.Is(err, domain.ErrInvalidState) {
			httpx.JSON(w, r, http.StatusOK, map[string]any{"ignored": true})
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	rr, err := h.svc.VerifyWebhook(r.Context(), ev.RepoURL, body,
		r.Header.Get("X-Hub-Signature-256"), r.Header.Get("X-Hub-Signature"),
		r.Header.Get("X-Gitlab-Token"))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		// Repo no conectado a ningún proyecto: 202 silencioso (no es error del emisor).
		httpx.JSON(w, r, http.StatusAccepted, map[string]any{"matched": false})
		return
	case err != nil:
		httpx.Fail(w, r, toAPIError(err)) // firma inválida => 401
		return
	}

	dep, ignored, err := h.svc.HandleGitEvent(r.Context(), rr, ev, deliveryID, body)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.JSON(w, r, http.StatusAccepted, map[string]any{"matched": false})
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if ignored || dep == nil {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"ignored": true})
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"deployment": dep})
}

// webhookHeaders detecta proveedor, tipo de evento y delivery id desde cabeceras.
func webhookHeaders(r *http.Request) (provider domain.GitProvider, eventType, deliveryID string) {
	if e := r.Header.Get("X-GitHub-Event"); e != "" {
		return domain.GitHub, e, r.Header.Get("X-GitHub-Delivery")
	}
	if e := r.Header.Get("X-Gitlab-Event"); e != "" {
		return domain.GitLab, e, r.Header.Get("X-Gitlab-Event-UUID")
	}
	return "", "", ""
}

// ---------------------------------------------------------------------------
// Helpers de query
// ---------------------------------------------------------------------------

func queryLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n
}

// queryCursor extrae el token opaco de continuación keyset (?cursor=).
func queryCursor(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("cursor"))
}

// periodFromQuery parsea ?from= y ?to= (RFC3339). Vacíos -> último mes.
func periodFromQuery(r *http.Request) (time.Time, time.Time) {
	var from, to time.Time
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}
