// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package service contiene la lógica de aplicación del control plane de Sytue
// Cloud: orquesta el store, el cifrado de secretos, la derivación de URLs de
// apps/previews y la publicación de eventos que disparan builds y despliegues.
package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/forge"
	"git.sytue.eu/sytue/one/services/cloud/internal/github"
	"git.sytue.eu/sytue/one/services/cloud/internal/identity"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// EventChannel es el canal lógico de eventos de Sytue Cloud. Canónico en el
// store (emisor del outbox); aquí se re-exporta para el Dispatcher/Subscriber.
const EventChannel = store.EventChannel

// Tipos de evento del control plane (notación punteada).
const (
	EventBuildEnqueued    = "cloud.build.enqueued"
	EventDeployRequested  = "cloud.deploy.requested"
	EventDeployRetired    = "cloud.deploy.retired"
	EventServiceRequested = "cloud.service.requested"
	EventServiceDeleted   = "cloud.service.deleted"
	EventServiceRestore   = "cloud.service.restore.requested"
	EventServiceBackup    = "cloud.service.backup.requested"
	EventServiceRun       = "cloud.service.run.requested"
	EventDomainAdded      = "cloud.domain.added"
	EventDomainRemoved    = "cloud.domain.removed"
	EventBuildCanceled    = "cloud.build.canceled"
)

// Publisher publica eventos de dominio en el bus (outbox sobre PostgreSQL).
type Publisher interface {
	Publish(ctx context.Context, ev events.Event) error
}

// dataStore es la superficie mínima de persistencia que consume el Service. La
// satisface *store.Store (producción) y un fake en tests, permitiendo ejercitar
// la lógica de aplicación sin PostgreSQL. No cambia la firma pública de New.
type dataStore interface {
	EnsureOrganization(ctx context.Context, name string, plan domain.Plan, region string) (domain.Organization, error)
	GetOrganization(ctx context.Context) (domain.Organization, error)
	UpdateOrganization(ctx context.Context, name *string, plan *domain.Plan, notifyWebhookURL *string) (domain.Organization, error)
	CreateProject(ctx context.Context, rec store.NewProjectRecord, maxPerOrg int) (domain.Project, error)
	GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	UpdateProject(ctx context.Context, id uuid.UUID, upd store.UpdateProjectRecord) (domain.Project, error)
	SetProjectInstallation(ctx context.Context, projectID uuid.UUID, installationID int64) error
	DeleteProject(ctx context.Context, id uuid.UUID) error
	EnqueueBuild(ctx context.Context, rec store.NewBuildRecord) (domain.Build, error)
	GetBuild(ctx context.Context, id uuid.UUID) (domain.Build, error)
	ListBuilds(ctx context.Context, projectID uuid.UUID, after store.PageCursor, limit int) (store.BuildPage, error)
	CreateDeployment(ctx context.Context, rec store.NewDeploymentRecord) (domain.Deployment, error)
	GetDeployment(ctx context.Context, id uuid.UUID) (domain.Deployment, error)
	ListDeployments(ctx context.Context, projectID uuid.UUID, after store.PageCursor, limit int) (store.DeploymentPage, error)
	GetActivePreview(ctx context.Context, projectID uuid.UUID, pr int) (domain.Deployment, error)
	Promote(ctx context.Context, id uuid.UUID) (domain.Deployment, error)
	CancelDeployment(ctx context.Context, id uuid.UUID) error
	AddDomain(ctx context.Context, projectID uuid.UUID, hostname, verifyToken string) (domain.Domain, error)
	ListDomains(ctx context.Context, projectID uuid.UUID) ([]domain.Domain, error)
	RemoveDomain(ctx context.Context, id uuid.UUID) (domain.Domain, error)
	SetDomainPrimary(ctx context.Context, id uuid.UUID) (domain.Domain, error)
	CancelBuild(ctx context.Context, id uuid.UUID) (jobName string, err error)
	SetBuildJobName(ctx context.Context, tenant string, id uuid.UUID, jobName string) error
	BuildStatus(ctx context.Context, tenant string, id uuid.UUID) (domain.BuildStatus, error)
	RecordAudit(ctx context.Context, in store.AuditEntry) error
	ListAudit(ctx context.Context, f store.AuditFilter, after store.PageCursor, limit int) (store.AuditPage, error)
	ExportAudit(ctx context.Context, f store.AuditFilter, w io.Writer) error
	UpsertEnvVar(ctx context.Context, rec store.EnvVarRecord) (domain.EnvVar, error)
	ListEnvVars(ctx context.Context, projectID uuid.UUID) ([]domain.EnvVar, error)
	DeleteEnvVar(ctx context.Context, id uuid.UUID) error
	ActiveWork(ctx context.Context, projectID uuid.UUID, pr *int) (builds, previews int, err error)
	EnvVarScope(ctx context.Context, id uuid.UUID) (domain.EnvScope, error)
	CloneEnvVars(ctx context.Context, from, to uuid.UUID) (int, error)
	ProvisionService(ctx context.Context, rec store.NewServiceRecord, maxManaged int) (domain.ManagedService, error)
	GetService(ctx context.Context, id uuid.UUID) (domain.ManagedService, error)
	ListServices(ctx context.Context, projectID uuid.UUID) ([]domain.ManagedService, error)
	UpdateServiceConfig(ctx context.Context, id uuid.UUID, name *string, config map[string]string) (domain.ManagedService, error)
	ReprovisionServices(ctx context.Context, ids []uuid.UUID) error
	DeleteService(ctx context.Context, id uuid.UUID) error
	SummarizeUsage(ctx context.Context, from, to time.Time) ([]store.UsageAggregate, error)
	ListUsage(ctx context.Context, from, to time.Time, limit int) ([]domain.UsageRecord, error)
	RecordUsage(ctx context.Context, in store.UsageInput) error
	GetOrCreateBillingAccount(ctx context.Context) (domain.BillingAccount, error)
	SetPlan(ctx context.Context, plan domain.Plan) (domain.BillingAccount, error)
	SetSpendLimit(ctx context.Context, limitEur *float64) (domain.BillingAccount, error)
	CreateInvoice(ctx context.Context, in domain.Invoice) (domain.Invoice, bool, error)
	GetInvoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error)
	ListInvoices(ctx context.Context, afterPeriod time.Time, afterID uuid.UUID, limit int) ([]domain.Invoice, error)
	ResolveRepos(ctx context.Context, repoURL string) ([]store.ResolvedRepo, error)
	RecordWebhook(ctx context.Context, tenant string, projectID uuid.UUID, provider, deliveryID, eventType string, payload []byte) (bool, error)
	GetProjectWebhookSecretEnc(ctx context.Context, id uuid.UUID) ([]byte, error)
	CreateAPIToken(ctx context.Context, rec store.NewAPITokenRecord) (domain.APIToken, error)
	ListAPITokens(ctx context.Context) ([]domain.APIToken, error)
	DeleteAPIToken(ctx context.Context, id uuid.UUID) error
	ResolveAPIToken(ctx context.Context, tokenHash string) (store.ResolvedToken, error)
	ListMemberships(ctx context.Context) ([]domain.Membership, error)
	// CountMemberships da los puestos observados de la suite (derivados, no tecleados).
	CountMemberships(ctx context.Context) (int, error)
	UpsertMembership(ctx context.Context, sub string, role domain.Role) (domain.Membership, error)
	DeleteMembership(ctx context.Context, sub string) error
	GetMembershipRole(ctx context.Context, sub string) (domain.Role, error)
	ListMemberInvitations(ctx context.Context) ([]domain.MemberInvitation, error)
	UpsertMemberInvitation(ctx context.Context, email string, role domain.Role, invitedBy string, expiresAt time.Time) (domain.MemberInvitation, error)
	DeleteMemberInvitation(ctx context.Context, id uuid.UUID) (domain.MemberInvitation, error)
	DeleteMemberInvitationByEmail(ctx context.Context, email string) error
	AcceptMemberInvitation(ctx context.Context, sub, email string) (domain.Membership, bool, error)
	CreateAlertPolicy(ctx context.Context, rec store.NewAlertPolicyRecord) (domain.AlertPolicy, error)
	UpdateAlertPolicy(ctx context.Context, id uuid.UUID, upd store.UpdateAlertPolicyRecord) (domain.AlertPolicy, error)
	ConfirmAlertPolicyByToken(ctx context.Context, tokenHash string) (domain.AlertPolicy, error)
}

// Service es la fachada de aplicación de Sytue Cloud.
type Service struct {
	cfg    config.Config
	store  dataStore
	pstore *store.Store
	sealer *crypto.Sealer
	pub    Publisher
	// blobs es el Object Storage S3 para leer logs de build (nil si no está
	// disponible: BuildLog degrada a "aún no disponible"). Solo lo usa cloud-api.
	blobs       *objectstore.Client
	logStreamer deploymentLogStreamer
	// gh es el cliente de GitHub App (nil = integración deshabilitada; todo lo
	// que dependa de ella degrada a no-op). Lo cablean los dos binarios que la
	// usan (cloud-api para el comentario de PR, cloud-builder para el clone).
	gh *github.Client
	// identity delega cuentas, tokens de invitación y pertenencia a la suite en
	// Sytue ID. Cloud solo conserva su rol operativo y el cómputo facturable.
	identity IdentityDirectory
	// cache es la cache efímera compartida (Dragonfly). Guarda la resolución de
	// los tokens de API, que ocurre en CADA petición autenticada por token.
	//
	// NIL es un estado normal —sin DRAGONFLY_URL no hay cache— y el tipo tolera el
	// receptor nil: se resuelve contra PostgreSQL igual que antes. La cache
	// acelera, nunca decide quién entra.
	cache *cache.Client
	// forge habla con Sytue Git en nombre del usuario para crearle un repositorio
	// (la tercera vía del alta, ver scaffold.go). NIL es un estado normal: sin
	// credenciales de OAuth de la forja, esa vía no se ofrece.
	forge *forge.Client
}

// UsarCache cablea la cache efímera compartida (nil la deshabilita).
func (s *Service) UsarCache(c *cache.Client) { s.cache = c }

// SetForge cablea el cliente de Sytue Git (nil deshabilita crear repositorios).
func (s *Service) SetForge(f *forge.Client) { s.forge = f }

// SetLogStreamer cablea el streamer de logs en vivo (cloud-api).
func (s *Service) SetLogStreamer(ls deploymentLogStreamer) { s.logStreamer = ls }

// SetGitHub cablea el cliente de GitHub App (nil deshabilita la integración).
func (s *Service) SetGitHub(gh *github.Client) { s.gh = gh }

// IdentityDirectory es la mínima API de identidad que necesita Cloud. Mantenerla
// como interfaz hace testeable la saga sin red y evita acoplar la lógica a HTTP.
type IdentityDirectory interface {
	ResolveMember(ctx context.Context, bearer, email string) (identity.Member, bool, error)
	Invite(ctx context.Context, bearer, email, role string) (time.Time, error)
	RevokeInvitation(ctx context.Context, bearer, email string) error
}

// SetIdentity cablea Sytue ID. Sin cliente las invitaciones fallan cerradas; el
// resto del control-plane continúa disponible.
func (s *Service) SetIdentity(id IdentityDirectory) { s.identity = id }

// GitHub expone el cliente de GitHub App (nil si no está configurada). Lo usa el
// builder para acuñar un installation token con el que clonar repos privados.
func (s *Service) GitHub() *github.Client { return s.gh }

// New construye el servicio.
func New(cfg config.Config, st *store.Store, sealer *crypto.Sealer, pub Publisher, blobs *objectstore.Client) *Service {
	return &Service{cfg: cfg, store: st, pstore: st, sealer: sealer, pub: pub, blobs: blobs}
}

// Store expone el store concreto (lo usan los workers builder/controller).
func (s *Service) Store() *store.Store { return s.pstore }

// Config expone la configuración (URLs de dominios, etc.).
func (s *Service) Config() config.Config { return s.cfg }

// Sealer expone el cifrador (lo usa el controller para descifrar secretos).
func (s *Service) Sealer() *crypto.Sealer { return s.sealer }

// emit publica un evento ignorando errores no fatales (solo log aguas arriba).
func (s *Service) emit(ctx context.Context, evType, tenant string, payload any) error {
	if s.pub == nil {
		return nil
	}
	ev, err := events.NewEvent(EventChannel, evType, tenant, payload)
	if err != nil {
		return fmt.Errorf("service: construir evento: %w", err)
	}
	return s.pub.Publish(ctx, ev)
}

// ---------------------------------------------------------------------------
// Organización
// ---------------------------------------------------------------------------

// EnsureOrg crea/obtiene la organización del tenant activo (alta idempotente).
func (s *Service) EnsureOrg(ctx context.Context, name string) (domain.Organization, error) {
	t := tenancy.MustFrom(ctx)
	if name == "" {
		name = t.Slug
	}
	return s.store.EnsureOrganization(ctx, name, domain.PlanFree, s.cfg.S3Region)
}

// GetOrg devuelve la organización del tenant.
func (s *Service) GetOrg(ctx context.Context) (domain.Organization, error) {
	return s.store.GetOrganization(ctx)
}

// UpdateOrg actualiza el nombre y la URL de aviso de deploy de la organización.
// El plan no pasa por aquí: lo cambia SetPlanByOperator (o el cobro, cuando lo haya).
func (s *Service) UpdateOrg(ctx context.Context, name, notifyWebhookURL *string) (domain.Organization, error) {
	if err := validateNotifyURL(notifyWebhookURL); err != nil {
		return domain.Organization{}, err
	}
	return s.store.UpdateOrganization(ctx, name, nil, notifyWebhookURL)
}

// SetPlanByOperator cambia el plan de la organización y el de su cuenta de cobro.
// Es la vía del operador (`cloud-api plan`); la API pública no la expone porque
// el plan solo cambia tras un pago confirmado o por el operador (2026-10-17).
func (s *Service) SetPlanByOperator(ctx context.Context, plan domain.Plan) (domain.Organization, error) {
	if !domain.ValidPlan(plan) {
		return domain.Organization{}, domain.ErrInvalidInput
	}
	org, err := s.store.UpdateOrganization(ctx, nil, &plan, nil)
	if err != nil {
		return domain.Organization{}, err
	}
	if _, err := s.store.SetPlan(ctx, plan); err != nil {
		return domain.Organization{}, err
	}
	s.audit(ctx, "billing.set_plan", string(plan), map[string]any{"by": "operator"})
	return org, nil
}

// validateNotifyURL valida la URL destino de avisos de deploy si se está fijando
// (no nil). Una cadena vacía la limpia (desactiva el aviso) y no se valida.
// Anti-SSRF: rechaza esquemas peligrosos y destinos internos (domain).
func validateNotifyURL(u *string) error {
	if u == nil || *u == "" {
		return nil
	}
	return domain.ValidateWebhookURL(*u)
}

// ---------------------------------------------------------------------------
// Proyectos
// ---------------------------------------------------------------------------

// CreateProjectInput son los datos de creación de un proyecto.
type CreateProjectInput struct {
	Slug               string
	Name               string
	RepoURL            string
	GitProvider        domain.GitProvider
	ProductionRef      string
	Framework          domain.Framework
	BuildStrategy      domain.BuildStrategy
	RootDir            string
	BuildCommand       string
	OutputDir          string
	InstallCmd         string
	AppPort            int
	HealthPath         string
	InstanceSize       string
	InstanceCount      int
	AutoscaleMaxCount  int
	AutoscaleCPUTarget int
	OwnerSub           string
}

// CreateProject valida y crea un proyecto. La organización debe existir (se crea
// perezosamente si no estaba). Genera un secreto de webhook git POR PROYECTO,
// lo persiste cifrado y devuelve el claro UNA vez (webhookSecret) para pegarlo en
// el proveedor git; en reposo solo queda el ciphertext.
func (s *Service) CreateProject(ctx context.Context, in CreateProjectInput) (proj domain.Project, webhookSecret string, err error) {
	if !domain.ValidSlug(in.Slug) {
		return domain.Project{}, "", fmt.Errorf("%w: slug de proyecto inválido", domain.ErrInvalidInput)
	}
	if err := errors.Join(domain.ValidateAppPort(in.AppPort), domain.ValidateHealthPath(in.HealthPath),
		domain.ValidateProjectDir(in.RootDir), domain.ValidateProjectDir(in.OutputDir)); err != nil {
		return domain.Project{}, "", err
	}
	if err := domain.ValidateInstanceSize(in.InstanceSize); err != nil {
		return domain.Project{}, "", err
	}
	if err := domain.ValidateInstanceCount(in.InstanceCount); err != nil {
		return domain.Project{}, "", err
	}
	if err := domain.ValidateAutoscaleMaxCount(in.AutoscaleMaxCount); err != nil {
		return domain.Project{}, "", err
	}
	if err := domain.ValidateAutoscaleCPUTarget(in.AutoscaleCPUTarget); err != nil {
		return domain.Project{}, "", err
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = in.Slug
	}
	if in.GitProvider == "" {
		in.GitProvider = domain.GitGeneric
	}
	if in.ProductionRef == "" {
		in.ProductionRef = "main"
	}
	if in.Framework == "" {
		in.Framework = domain.FrameworkDetect
	}
	if !domain.ValidFramework(in.Framework) {
		return domain.Project{}, "", fmt.Errorf("%w: framework inválido", domain.ErrInvalidInput)
	}
	if !domain.ValidBuildStrategy(in.BuildStrategy) {
		return domain.Project{}, "", fmt.Errorf("%w: estrategia de build inválida", domain.ErrInvalidInput)
	}
	// Anti-SSRF: la repo_url la fetchea el Job de build server-side; validar
	// esquema/host y rechazar destinos internos/metadata antes de persistir.
	if err := domain.ValidateRepoURL(in.RepoURL); err != nil {
		return domain.Project{}, "", err
	}
	if _, err := s.EnsureOrg(ctx, ""); err != nil {
		return domain.Project{}, "", err
	}
	org, err := s.store.GetOrganization(ctx)
	if err != nil {
		return domain.Project{}, "", err
	}
	quota := domain.QuotasFor(org.Plan)
	// Secreto de webhook por proyecto (aleatorio de 256 bits), cifrado en reposo.
	// Reutiliza el generador de tokens (mismo formato prefijado, apto para pegar).
	secret, _, err := GenerateAPIToken()
	if err != nil {
		return domain.Project{}, "", err
	}
	enc, err := s.sealer.SealString(secret)
	if err != nil {
		return domain.Project{}, "", fmt.Errorf("service: cifrar secreto de webhook: %w", err)
	}
	p, err := s.store.CreateProject(ctx, store.NewProjectRecord{
		Slug:               in.Slug,
		Name:               in.Name,
		RepoURL:            strings.TrimSpace(in.RepoURL),
		GitProvider:        in.GitProvider,
		ProductionRef:      in.ProductionRef,
		Framework:          in.Framework,
		BuildStrategy:      in.BuildStrategy,
		AppPort:            in.AppPort,
		HealthPath:         in.HealthPath,
		InstanceSize:       in.InstanceSize,
		InstanceCount:      in.InstanceCount,
		AutoscaleMaxCount:  in.AutoscaleMaxCount,
		AutoscaleCPUTarget: in.AutoscaleCPUTarget,
		RootDir:            in.RootDir,
		BuildCommand:       in.BuildCommand,
		OutputDir:          in.OutputDir,
		InstallCmd:         in.InstallCmd,
		OwnerSub:           in.OwnerSub,
		WebhookSecretEnc:   enc,
	}, quota.MaxProjects)
	if err != nil {
		return domain.Project{}, "", err
	}
	// Alta de proyecto: crea recursos que se facturan y una superficie desplegable.
	s.audit(ctx, "project.create", p.Slug, map[string]any{"repo_url": p.RepoURL})
	return p, secret, nil
}

// RotateProjectWebhookSecret genera un secreto de webhook NUEVO para el proyecto,
// lo persiste cifrado y devuelve el claro una vez.
//
// Sin esto, un secreto filtrado no tenía arreglo: se escribía al crear el
// proyecto y no había ningún camino para cambiarlo, así que la única salida era
// rehacer el proyecto —con su dominio, sus variables y sus servicios—.
//
// El de antes deja de valer EN EL ACTO. Es lo correcto para lo que esto es (una
// credencial comprometida), y la consecuencia hay que decirla en la interfaz: los
// push del proveedor git fallarán la verificación de firma hasta que alguien
// pegue el secreto nuevo allí.
func (s *Service) RotateProjectWebhookSecret(ctx context.Context, id uuid.UUID) (string, error) {
	// Se comprueba que el proyecto existe y es de este tenant ANTES de generar
	// nada: la RLS del UPDATE ya lo haría, pero así el 404 llega sin haber
	// producido una credencial que nadie va a usar.
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return "", err
	}
	secret, _, err := GenerateAPIToken()
	if err != nil {
		return "", err
	}
	enc, err := s.sealer.SealString(secret)
	if err != nil {
		return "", fmt.Errorf("service: cifrar secreto de webhook: %w", err)
	}
	setter, ok := s.store.(interface {
		SetProjectWebhookSecretEnc(ctx context.Context, id uuid.UUID, enc []byte) error
	})
	if !ok {
		return "", fmt.Errorf("service: el store no sabe rotar el secreto de webhook")
	}
	if err := setter.SetProjectWebhookSecretEnc(ctx, id, enc); err != nil {
		return "", err
	}
	// Rotar una credencial es un hecho de seguridad: va al registro de auditoría
	// con el proyecto, nunca con el valor.
	s.audit(ctx, "project.webhook_secret.rotate", p.Slug, nil)
	return secret, nil
}

// ProjectWebhookSecret descifra y devuelve el secreto de webhook de un proyecto
// (para revelarlo al admin). Devuelve "" si el proyecto no tiene secreto propio
// (creado antes de los secretos por proyecto). ErrNotFound si no existe.
func (s *Service) ProjectWebhookSecret(ctx context.Context, id uuid.UUID) (string, error) {
	enc, err := s.store.GetProjectWebhookSecretEnc(ctx, id)
	if err != nil {
		return "", err
	}
	if len(enc) == 0 {
		return "", nil
	}
	return s.sealer.OpenString(enc)
}

// WebhookURL es la URL pública del endpoint de webhooks git (para configurar el
// PublicURL es la raíz del panel. La usa el retorno del consentimiento de la
// forja para devolver al usuario a donde empezó.
func (s *Service) PublicURL() string { return s.cfg.PublicURL }

// ForgeEnabled dice si esta instalación puede crear repositorios en Sytue Git.
// Se expone para que la INTERFAZ pueda explicar la ausencia en vez de esconder la
// opción: un botón que no está y nadie sabe por qué es una función perdida.
func (s *Service) ForgeEnabled() bool { return s.forge != nil }

// proveedor). Deriva del PublicURL del panel.
func (s *Service) WebhookURL() string {
	return strings.TrimSuffix(s.cfg.PublicURL, "/") + "/webhooks/git"
}

// GetProject devuelve un proyecto por ID.
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error) {
	return s.store.GetProject(ctx, id)
}

// ListProjects lista los proyectos del tenant.
func (s *Service) ListProjects(ctx context.Context) ([]domain.Project, error) {
	return s.store.ListProjects(ctx)
}

// UpdateProject aplica una actualización parcial.
func (s *Service) UpdateProject(ctx context.Context, id uuid.UUID, upd store.UpdateProjectRecord) (domain.Project, error) {
	if upd.Framework != nil && !domain.ValidFramework(*upd.Framework) {
		return domain.Project{}, fmt.Errorf("%w: framework inválido", domain.ErrInvalidInput)
	}
	if upd.BuildStrategy != nil && !domain.ValidBuildStrategy(*upd.BuildStrategy) {
		return domain.Project{}, fmt.Errorf("%w: estrategia de build inválida", domain.ErrInvalidInput)
	}
	if upd.RepoURL != nil {
		// Anti-SSRF: validar la nueva repo_url igual que en la creación.
		if err := domain.ValidateRepoURL(*upd.RepoURL); err != nil {
			return domain.Project{}, err
		}
	}
	for _, dir := range []*string{upd.RootDir, upd.OutputDir} {
		if dir != nil {
			if err := domain.ValidateProjectDir(*dir); err != nil {
				return domain.Project{}, err
			}
		}
	}
	// Anti-SSRF: la URL de aviso la contacta el worker server-side.
	if err := validateNotifyURL(upd.NotifyWebhookURL); err != nil {
		return domain.Project{}, err
	}
	if upd.ReleaseCommand != nil {
		// Solo el tope: el contenido es una orden de shell que corre en la imagen
		// del propio cliente, así que filtrarla por palabras sería teatro.
		//
		// Cambiarlo NO reencola la producción a propósito (no está en la lista de
		// `touchProductionIfRuntimeChanged`): el comando de release se ejecuta al
		// DESPLEGAR, y relanzar una migración solo porque alguien editó el campo
		// sería justo lo que nadie espera.
		if err := domain.ValidateReleaseCommand(*upd.ReleaseCommand); err != nil {
			return domain.Project{}, err
		}
	}
	if upd.HTTPRules != nil {
		// Aquí se para la inyección de cabeceras (un `\r\n` en un valor escribe
		// cabeceras de más) y las regex del cliente, que no existen: el dominio
		// traduce rutas y escapa lo demás.
		if err := domain.ValidateHTTPRules(*upd.HTTPRules); err != nil {
			return domain.Project{}, err
		}
	}
	if upd.DeployKey != nil {
		if err := s.applyDeployKeyUpdate(&upd); err != nil {
			return domain.Project{}, err
		}
	}
	proj, err := s.store.UpdateProject(ctx, id, upd)
	if err != nil {
		return domain.Project{}, err
	}
	// Repositorio, rama, deploy key y comandos deciden qué se ejecuta: queda
	// quién los cambió (CLOUD-14). Solo los nombres, nunca los valores.
	s.audit(ctx, "project.update", id.String(), map[string]any{"fields": changedFields(upd)})
	return proj, nil
}

// changedFields lista los campos que trae una actualización parcial (los
// punteros no nil), para la auditoría.
func changedFields(upd store.UpdateProjectRecord) []string {
	v := reflect.ValueOf(upd)
	var out []string
	for i := range v.NumField() {
		if f := v.Field(i); (f.Kind() == reflect.Pointer || f.Kind() == reflect.Map || f.Kind() == reflect.Slice) && !f.IsNil() {
			out = append(out, v.Type().Field(i).Name)
		}
	}
	return out
}

// applyDeployKeyUpdate valida y cifra la deploy key antes de persistirla.
func (s *Service) applyDeployKeyUpdate(upd *store.UpdateProjectRecord) error {
	if upd.DeployKey == nil {
		return nil
	}
	key := strings.TrimSpace(*upd.DeployKey)
	if key == "" {
		empty := []byte{}
		upd.RepoCredsEnc = &empty
		return nil
	}
	if !strings.Contains(key, "PRIVATE KEY") {
		return fmt.Errorf("%w: deploy key inválida", domain.ErrInvalidInput)
	}
	enc, err := s.sealer.SealString(key)
	if err != nil {
		return fmt.Errorf("service: cifrar deploy key: %w", err)
	}
	upd.RepoCredsEnc = &enc
	return nil
}

// DeleteProject borra un proyecto y dispara la retirada de sus deployments.
func (s *Service) DeleteProject(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	if err := s.store.DeleteProject(ctx, id); err != nil {
		return err
	}
	// Baja de proyecto: es destructiva (retira despliegues y recursos asociados),
	// así que tiene que quedar quién la pidió.
	s.audit(ctx, "project.delete", id.String(), nil)
	return s.emit(ctx, EventDeployRetired, t.Slug, map[string]any{"project_id": id})
}

// ---------------------------------------------------------------------------
// Builds + Deployments
// ---------------------------------------------------------------------------

// DeployInput son los datos para disparar un despliegue (build + deploy).
type DeployInput struct {
	ProjectID uuid.UUID
	CommitSHA string
	// CommitMessage es el asunto del commit/PR sin sanear: Deploy lo pasa por
	// domain.SanitizeCommitMessage porque lo escribe quien hace el push.
	CommitMessage string
	GitRef        string
	Target        domain.DeployTarget
	PRNumber      *int
	CreatedBy     string
	// ImageRef despliega una imagen externa BYO sin encolar build (digest o tag).
	ImageRef string
}

// Deploy encola una build y crea el deployment asociado en estado 'queued'.
// Con ImageRef salta la build y pasa directo a deploy (BYO).
func (s *Service) Deploy(ctx context.Context, in DeployInput) (domain.Deployment, domain.Build, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidTarget(in.Target) {
		return domain.Deployment{}, domain.Build{}, fmt.Errorf("%w: target inválido", domain.ErrInvalidInput)
	}
	if in.Target == domain.TargetPreview && in.PRNumber == nil {
		return domain.Deployment{}, domain.Build{}, fmt.Errorf("%w: preview requiere pr_number", domain.ErrInvalidInput)
	}
	proj, err := s.store.GetProject(ctx, in.ProjectID)
	if err != nil {
		return domain.Deployment{}, domain.Build{}, err
	}

	// Choke point de facturación: si el tenant tiene un tope de gasto y lo ha
	// superado este mes, se rechaza el nuevo despliegue ANTES de encolar el build
	// (o de crear el deployment BYO, que también consume cómputo). Tope nil = sin
	// tope: WithinSpendLimit no consulta el uso (barato en el caso común).
	within, err := s.WithinSpendLimit(ctx)
	if err != nil {
		return domain.Deployment{}, domain.Build{}, err
	}
	if !within {
		return domain.Deployment{}, domain.Build{}, domain.ErrSpendLimitExceeded
	}

	// Requisito de suscripción: Sytue Cloud no se vende suelto. El plan Free exige
	// al menos un puesto de la suite (Mail/Drive/Docs/Rooms). Se comprueba en el
	// MISMO punto que el tope de gasto —justo antes de comprometer cómputo— para
	// que no haya una puerta lateral que despliegue sin suscripción.
	ok, err := s.MeetsSuiteRequirement(ctx)
	if err != nil {
		return domain.Deployment{}, domain.Build{}, err
	}
	if !ok {
		return domain.Deployment{}, domain.Build{}, domain.ErrSuiteRequired
	}

	// Topes de cola (CLOUD-06): la cola de builds es una sola para todas las
	// organizaciones, y cada preview pide su propio certificado a Let's Encrypt,
	// cuyo límite semanal es del dominio entero. Una organización no puede
	// llenar ninguna de las dos.
	if err := s.checkQueue(ctx, in); err != nil {
		return domain.Deployment{}, domain.Build{}, err
	}

	// BYO: imagen externa ya construida, sin build Kaniko. Debe residir en el
	// registry de la plataforma (EU) y llevar tag/digest.
	if in.ImageRef != "" {
		if err := domain.ValidateImageRef(in.ImageRef, s.cfg.RegistryURL, proj.Slug); err != nil {
			return domain.Deployment{}, domain.Build{}, err
		}
		url := s.DeploymentURL(proj, in.Target, in.PRNumber)
		dep, err := s.store.CreateDeployment(ctx, store.NewDeploymentRecord{
			ProjectID:     in.ProjectID,
			Target:        in.Target,
			CommitSHA:     in.CommitSHA,
			CommitMessage: domain.SanitizeCommitMessage(in.CommitMessage),
			GitRef:        in.GitRef,
			PRNumber:      in.PRNumber,
			URL:           url,
			CreatedBy:     in.CreatedBy,
			ImageRef:      in.ImageRef,
		})
		if err != nil {
			return domain.Deployment{}, domain.Build{}, err
		}
		_ = s.emit(ctx, EventDeployRequested, t.Slug, map[string]any{
			"deployment_id": dep.ID, "project_id": proj.ID, "image_ref": in.ImageRef,
		})
		s.audit(ctx, "deploy.byo", dep.ID.String(), map[string]any{"image": in.ImageRef})
		return dep, domain.Build{}, nil
	}
	if in.GitRef == "" {
		in.GitRef = proj.ProductionRef
	}
	// CommitSHA y GitRef pueden venir del cuerpo de un webhook (controlado por
	// quien conozca el secreto): validarlos antes de usarlos como ref del contexto
	// git de Kaniko y como tag de imagen OCI (evita refs/tags manipulados).
	if in.CommitSHA != "" && !domain.ValidCommitSHA(in.CommitSHA) {
		return domain.Deployment{}, domain.Build{}, fmt.Errorf("%w: commit SHA inválido", domain.ErrInvalidInput)
	}
	if !domain.ValidGitRef(in.GitRef) {
		return domain.Deployment{}, domain.Build{}, fmt.Errorf("%w: git ref inválido", domain.ErrInvalidInput)
	}

	build, err := s.store.EnqueueBuild(ctx, store.NewBuildRecord{
		ProjectID:   in.ProjectID,
		CommitSHA:   in.CommitSHA,
		GitRef:      in.GitRef,
		Framework:   proj.Framework,
		TriggeredBy: in.CreatedBy,
	})
	if err != nil {
		return domain.Deployment{}, domain.Build{}, err
	}

	url := s.DeploymentURL(proj, in.Target, in.PRNumber)
	dep, err := s.store.CreateDeployment(ctx, store.NewDeploymentRecord{
		ProjectID:     in.ProjectID,
		BuildID:       &build.ID,
		Target:        in.Target,
		CommitSHA:     in.CommitSHA,
		CommitMessage: domain.SanitizeCommitMessage(in.CommitMessage),
		GitRef:        in.GitRef,
		PRNumber:      in.PRNumber,
		URL:           url,
		CreatedBy:     in.CreatedBy,
	})
	if err != nil {
		return domain.Deployment{}, domain.Build{}, err
	}

	_ = s.emit(ctx, EventBuildEnqueued, t.Slug, map[string]any{
		"build_id": build.ID, "deployment_id": dep.ID, "project_id": proj.ID,
	})
	// También los que entran por webhook o por URL: created_by dice de dónde.
	s.audit(ctx, "deploy.create", dep.ID.String(), map[string]any{
		"project_id": proj.ID.String(), "target": string(in.Target), "git_ref": in.GitRef,
		"commit": in.CommitSHA, "created_by": in.CreatedBy,
	})
	return dep, build, nil
}

// checkQueue aplica los topes de cola de Deploy (CLOUD-06).
func (s *Service) checkQueue(ctx context.Context, in DeployInput) error {
	builds, previews, err := s.store.ActiveWork(ctx, in.ProjectID, in.PRNumber)
	if err != nil {
		return err
	}
	if (in.ImageRef == "" && builds >= domain.MaxActiveBuildsPerOrg) ||
		(in.Target == domain.TargetPreview && previews >= domain.MaxLivePreviewsPerProject) {
		return domain.ErrQueueFull
	}
	return nil
}

// DeploymentURL deriva la URL pública de un deployment según su target.
//   - production: https://<slug>.apps.sytue.eu
//   - preview:    https://<slug>-pr-<n>.preview.sytue.eu
func (s *Service) DeploymentURL(proj domain.Project, target domain.DeployTarget, pr *int) string {
	if target == domain.TargetPreview && pr != nil {
		host := fmt.Sprintf("%s-pr-%d.%s", proj.Slug, *pr, s.cfg.PreviewDomain)
		return "https://" + host
	}
	return "https://" + proj.Slug + "." + s.cfg.AppsDomain
}

// GetBuild devuelve una build por ID.
func (s *Service) GetBuild(ctx context.Context, id uuid.UUID) (domain.Build, error) {
	return s.store.GetBuild(ctx, id)
}

// maxBuildLogBytes acota la lectura del log de build en memoria (los logs reales
// llegan a decenas de miles de líneas; un objeto corrupto/gigante no debe tumbar
// la API). ponytail: tope fijo; si hiciera falta, servir por rangos/streaming.
const maxBuildLogBytes = 8 << 20 // 8 MiB

// BuildLog devuelve el texto del log de una build leyéndolo del Object Storage
// por su logs_key. La build se lee bajo RLS (GetBuild valida que pertenece al
// tenant del contexto) y el objeto S3 se lee con ese mismo tenant. Devuelve
// ErrNotFound (→404 "aún no disponible") si la build no existe, no tiene log
// todavía (logs_key vacío / build en curso), no hay Object Storage, o el objeto
// no está en S3.
//
// Filtrado server-side (stream + filter, memoria acotada): q casa por subcadena
// case-insensitive; level casa el nivel como token (si la línea lo lleva). Sin
// filtros devuelve el log completo hasta maxLines (con tope por defecto). La lectura
// de S3 se acota además a maxBuildLogBytes (LimitReader), así una línea patológica
// o un log gigante nunca cargan sin cota en memoria.
func (s *Service) BuildLog(ctx context.Context, id uuid.UUID, q, level string, maxLines int) (string, error) {
	b, err := s.store.GetBuild(ctx, id)
	if err != nil {
		return "", err
	}
	if b.LogsKey == "" || s.blobs == nil {
		return "", domain.ErrNotFound
	}
	t := tenancy.MustFrom(ctx)
	rc, err := s.blobs.Get(ctx, t, b.LogsKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("service: leer log de build: %w", err)
	}
	defer func() { _ = rc.Close() }()
	return filterLogLines(io.LimitReader(rc, maxBuildLogBytes), q, level, maxLines), nil
}

// BuildPage es una página de builds con el token de continuación ya codificado
// (opaco) para la siguiente petición ("" si no hay más).
type BuildPage struct {
	Builds     []domain.Build
	NextCursor string
}

// ListBuilds lista las builds de un proyecto con paginación keyset. El cursor
// es un token opaco devuelto por la página anterior (vacío = inicio).
func (s *Service) ListBuilds(ctx context.Context, projectID uuid.UUID, cursor string, limit int) (BuildPage, error) {
	after, err := decodePageCursor(cursor)
	if err != nil {
		return BuildPage{}, err
	}
	page, err := s.store.ListBuilds(ctx, projectID, after, limit)
	if err != nil {
		return BuildPage{}, err
	}
	return BuildPage{Builds: page.Builds, NextCursor: encodePageCursor(page.Next)}, nil
}

// GetDeployment devuelve un deployment por ID.
func (s *Service) GetDeployment(ctx context.Context, id uuid.UUID) (domain.Deployment, error) {
	return s.store.GetDeployment(ctx, id)
}

// DeploymentPage es una página de deployments con el token de continuación ya
// codificado (opaco) para la siguiente petición ("" si no hay más).
type DeploymentPage struct {
	Deployments []domain.Deployment
	NextCursor  string
}

// ListDeployments lista los deployments de un proyecto con paginación keyset.
// El cursor es un token opaco devuelto por la página anterior (vacío = inicio).
func (s *Service) ListDeployments(ctx context.Context, projectID uuid.UUID, cursor string, limit int) (DeploymentPage, error) {
	after, err := decodePageCursor(cursor)
	if err != nil {
		return DeploymentPage{}, err
	}
	page, err := s.store.ListDeployments(ctx, projectID, after, limit)
	if err != nil {
		return DeploymentPage{}, err
	}
	return DeploymentPage{Deployments: page.Deployments, NextCursor: encodePageCursor(page.Next)}, nil
}

// cursorWire es la forma serializada de un store.PageCursor (claves cortas).
type cursorWire struct {
	T time.Time `json:"t"`
	I uuid.UUID `json:"i"`
}

// encodePageCursor serializa un cursor keyset a un token opaco URL-safe ("" si
// no hay más páginas). Formato interno (JSON base64): el cliente lo trata como
// una cadena opaca.
func encodePageCursor(c *store.PageCursor) string {
	if c == nil {
		return ""
	}
	b, err := json.Marshal(cursorWire{T: c.CreatedAt, I: c.ID})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodePageCursor invierte encodePageCursor. Un token vacío es la primera
// página; un token corrupto es entrada inválida (400), no un fallo de servidor.
func decodePageCursor(token string) (store.PageCursor, error) {
	if token == "" {
		return store.PageCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return store.PageCursor{}, fmt.Errorf("%w: cursor inválido", domain.ErrInvalidInput)
	}
	var w cursorWire
	if err := json.Unmarshal(raw, &w); err != nil || w.T.IsZero() {
		return store.PageCursor{}, fmt.Errorf("%w: cursor inválido", domain.ErrInvalidInput)
	}
	return store.PageCursor{CreatedAt: w.T, ID: w.I}, nil
}

// Rollback promociona un deployment de producción anterior (debe estar 'ready').
// Rollback re-promociona una producción previa (ya construida) a activa. ponytail:
// EXENTO a propósito del tope de gasto de Deploy — el rollback reusa una imagen ya
// construida (no gasta build_minutes) y una app en marcha consume cómputo con o sin
// él; bloquearlo atraparía a un tenant sobre-tope en un deploy roto sin vía de
// recuperación. El tope pausa gasto NUEVO (Deploy), no la recuperación.
func (s *Service) Rollback(ctx context.Context, deploymentID uuid.UUID) (domain.Deployment, error) {
	t := tenancy.MustFrom(ctx)
	dep, err := s.store.Promote(ctx, deploymentID)
	if err != nil {
		return domain.Deployment{}, err
	}
	_ = s.emit(ctx, EventDeployRequested, t.Slug, map[string]any{"deployment_id": dep.ID})
	s.audit(ctx, "deploy.rollback", dep.ID.String(), map[string]any{"from": deploymentID.String()})
	return dep, nil
}

// CloneProject crea un proyecto nuevo con la MISMA configuración y las MISMAS
// variables que otro, y devuelve también su secreto de webhook y cuántas
// variables copió.
//
// Es el «Clone app» de DigitalOcean, y sirve para lo que todo el mundo acaba
// haciendo a mano: levantar un staging idéntico a producción, o probar un cambio
// de configuración sin tocar el proyecto bueno. Repetir a ojo el repo, la rama,
// los comandos de compilación, el tamaño, el puerto, la sonda y —sobre todo— las
// veinte variables de entorno es exactamente donde se cuela el error que luego
// cuesta una tarde encontrar.
//
// Lo que NO se clona, y por qué:
//   - Dominios: un hostname apunta a un sitio y solo a uno.
//   - Servicios gestionados: cada uno cuesta dinero; clonar un proyecto no puede
//     duplicar la factura sin que nadie lo haya pedido. Los datos de una base se
//     copian con backup + restore, que es un verbo distinto y consciente.
//   - Despliegues e historial: son del proyecto que los hizo.
//   - Secreto del webhook: se genera uno propio (compartirlo ataría los dos
//     proyectos a la misma credencial).
func (s *Service) CloneProject(ctx context.Context, srcID uuid.UUID, slug, name string) (domain.Project, string, int, error) {
	src, err := s.store.GetProject(ctx, srcID) // bajo RLS
	if err != nil {
		return domain.Project{}, "", 0, err
	}
	if name == "" {
		name = src.Name
	}
	// Reusa el alta normal: valida el slug, aplica la cuota de proyectos de la
	// organización y sella el secreto de webhook. Un camino propio se saltaría las
	// tres cosas y nadie lo notaría hasta tener el proyecto de más.
	proj, secret, err := s.CreateProject(ctx, CreateProjectInput{
		Slug:               slug,
		Name:               name,
		RepoURL:            src.RepoURL,
		GitProvider:        src.GitProvider,
		ProductionRef:      src.ProductionRef,
		Framework:          src.Framework,
		BuildStrategy:      src.BuildStrategy,
		RootDir:            src.RootDir,
		BuildCommand:       src.BuildCommand,
		OutputDir:          src.OutputDir,
		InstallCmd:         src.InstallCmd,
		AppPort:            src.AppPort,
		HealthPath:         src.HealthPath,
		InstanceSize:       src.InstanceSize,
		InstanceCount:      src.InstanceCount,
		AutoscaleMaxCount:  src.AutoscaleMaxCount,
		AutoscaleCPUTarget: src.AutoscaleCPUTarget,
		OwnerSub:           actorSub(ctx),
	})
	if err != nil {
		return domain.Project{}, "", 0, err
	}
	// Lo que el alta no sabe llevar: se aplica encima. Va DESPUÉS de crear porque
	// son campos de configuración avanzada que CreateProjectInput no expone, no
	// porque puedan fallar por separado.
	if src.ReleaseCommand != "" || len(src.HTTPRules.Redirects) > 0 || len(src.HTTPRules.Headers) > 0 {
		rules := src.HTTPRules
		upd := store.UpdateProjectRecord{HTTPRules: &rules}
		if src.ReleaseCommand != "" {
			rc := src.ReleaseCommand
			upd.ReleaseCommand = &rc
		}
		if p2, uerr := s.store.UpdateProject(ctx, proj.ID, upd); uerr == nil {
			proj = p2
		}
	}
	copied, err := s.store.CloneEnvVars(ctx, src.ID, proj.ID)
	if err != nil {
		// El proyecto ya existe: devolverlo con la cuenta a cero y el error sería
		// ambiguo. Se dice que falló la copia de variables, que es lo que pasó, y
		// el proyecto se queda creado (borrarlo aquí destruiría el secreto de
		// webhook que el llamador ya no volverá a ver).
		return proj, secret, 0, fmt.Errorf("clonar variables: %w", err)
	}
	s.audit(ctx, "project.clone", proj.ID.String(), map[string]any{
		"from": src.ID.String(), "env_vars": copied,
	})
	return proj, secret, copied, nil
}

// PromotePreview publica en producción la imagen que ya está corriendo en un
// preview, sin volver a compilarla.
//
// Es el «Promote to Production» de Vercel, y lo que faltaba para que revisar en
// una preview sirviera de algo: cuando la rama es la buena, el único camino era
// mergear y esperar otra compilación entera de lo MISMO —los mismos minutos de
// build, cobrados otra vez, para producir un artefacto idéntico— con la
// posibilidad de que lo que llegue a producción no sea exactamente lo que se
// revisó (una dependencia que se movió entre las dos compilaciones basta).
//
// Lo que NO se hereda son las variables de entorno: se resuelven por target en el
// momento de aplicar, así que el deployment nuevo recibe las de PRODUCCIÓN. Lo
// único que viaja del preview es la imagen.
//
// Exento del tope de gasto por el mismo motivo que el rollback: no compila nada
// —reusa un artefacto ya pagado— y una app en marcha consume cómputo con esto o
// sin esto.
func (s *Service) PromotePreview(ctx context.Context, deploymentID uuid.UUID) (domain.Deployment, error) {
	t := tenancy.MustFrom(ctx)
	src, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return domain.Deployment{}, err
	}
	if src.Target != domain.TargetPreview {
		return domain.Deployment{}, fmt.Errorf("%w: solo se promociona una preview; para volver a una producción anterior está el rollback", domain.ErrInvalidState)
	}
	if src.Status != domain.DeployReady {
		return domain.Deployment{}, fmt.Errorf("%w: la preview todavía no está lista", domain.ErrInvalidState)
	}
	image, err := s.deployedImage(ctx, src)
	if err != nil {
		return domain.Deployment{}, err
	}
	proj, err := s.store.GetProject(ctx, src.ProjectID)
	if err != nil {
		return domain.Deployment{}, err
	}
	dep, err := s.store.CreateDeployment(ctx, store.NewDeploymentRecord{
		ProjectID: src.ProjectID,
		// El build de origen viaja para no perder la trazabilidad: desde la
		// producción se llega al log de la compilación que la produjo, que es de
		// donde salió la imagen aunque la haya construido otra rama.
		BuildID:       src.BuildID,
		Target:        domain.TargetProduction,
		CommitSHA:     src.CommitSHA,
		CommitMessage: src.CommitMessage,
		GitRef:        src.GitRef,
		URL:           s.DeploymentURL(proj, domain.TargetProduction, nil),
		CreatedBy:     actorSub(ctx),
		ImageRef:      image,
	})
	if err != nil {
		return domain.Deployment{}, err
	}
	_ = s.emit(ctx, EventDeployRequested, t.Slug, map[string]any{
		"deployment_id": dep.ID, "project_id": proj.ID, "image_ref": image,
	})
	s.audit(ctx, "deploy.promote_preview", dep.ID.String(), map[string]any{
		"from_deployment": src.ID, "image": image,
	})
	return dep, nil
}

// deployedImage devuelve la imagen que ESTÁ corriendo en un deployment: la suya
// propia (BYO) o la que produjo su build.
//
// Un preview listo siempre tiene una de las dos; si no la tuviera, promoverlo
// crearía una producción sin imagen que el controlador dejaría en error, así que
// se rechaza aquí con un motivo legible.
func (s *Service) deployedImage(ctx context.Context, dep domain.Deployment) (string, error) {
	if dep.ImageRef != "" {
		return dep.ImageRef, nil
	}
	if dep.BuildID == nil {
		return "", fmt.Errorf("%w: ese despliegue no tiene una imagen que publicar", domain.ErrInvalidState)
	}
	b, err := s.store.GetBuild(ctx, *dep.BuildID)
	if err != nil {
		return "", err
	}
	if b.Status != domain.BuildSucceeded || b.ImageRef == "" {
		return "", fmt.Errorf("%w: la compilación de esa preview no dejó imagen", domain.ErrInvalidState)
	}
	return b.ImageRef, nil
}

// appRestarter es la parte del runtime que sabe reiniciar. Va como aserción
// sobre el streamer que ya está inyectado —igual que el tailer de logs de
// servicios— en vez de como dependencia nueva: es el mismo cliente contra el
// mismo clúster, y añadir un campo obligaría a tocar el arranque y los tests de
// todo lo demás para nada.
type appRestarter interface {
	RestartDeployment(ctx context.Context, namespace, selector string) (int, error)
}

// RestartApp reinicia las instancias de la producción activa sin cambiar de
// versión. Devuelve cuántas se reiniciaron.
//
// Faltaba la salida al caso más vulgar de todos: la app sigue publicada y
// correcta, pero el proceso se ha quedado tonto —memoria comida, una conexión
// zombi, un fichero temporal a medias—. Lo único que había era volver a
// desplegar, que recompila y cobra minutos de build para acabar corriendo el
// mismo commit, o un rollback, que además cambia lo que hay publicado.
//
// Un proyecto pausado no se reinicia: no hay nada corriendo, y hacerlo pasar por
// «hecho» dejaría a quien lo pulsa esperando un efecto que no va a llegar.
func (s *Service) RestartApp(ctx context.Context, projectID uuid.UUID) (int, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	if proj.Paused {
		return 0, fmt.Errorf("%w: el proyecto está pausado; reanúdalo antes de reiniciarlo", domain.ErrInvalidState)
	}
	// La producción activa se resuelve BAJO RLS: el id del deployment no lo elige
	// el cliente, así que no hay forma de reiniciar la app de otro.
	dep, err := s.pstore.GetActiveProduction(ctx, projectID)
	if err != nil {
		return 0, err
	}
	r, ok := s.logStreamer.(appRestarter)
	if !ok {
		return 0, nil // dev sin clúster: la respuesta ya viaja marcada como no-live
	}
	n, err := r.RestartDeployment(ctx, "tenant-"+dep.Tenant, "sytue.eu/deployment="+dep.ID.String())
	if err != nil {
		return 0, err
	}
	s.audit(ctx, "app.restart", projectID.String(), map[string]any{"deployment_id": dep.ID, "restarted": n})
	return n, nil
}

// CancelDeployment cancela un deployment vivo Y la compilación que lo estaba
// produciendo, si todavía vive.
//
// Antes solo marcaba el deployment: quien paraba un despliegue atascado se quedaba con
// el trabajo de compilación corriendo en el clúster, y los minutos de compilación SE
// FACTURAN. Cancelar tenía que dejar de costar dinero, que es lo que cualquiera espera
// del botón.
//
// El fallo al cancelar la compilación no tumba la petición: el deployment ya está
// cancelado —que es lo que se pidió— y devolver error haría que la interfaz dijera que
// no se pudo cuando sí se pudo. Lo normal, además, es que ya haya terminado, y eso llega
// como ErrInvalidState.
func (s *Service) CancelDeployment(ctx context.Context, id uuid.UUID) error {
	d, errDep := s.store.GetDeployment(ctx, id)
	if err := s.store.CancelDeployment(ctx, id); err != nil {
		return err
	}
	if errDep == nil && d.BuildID != nil {
		if err := s.CancelBuild(ctx, *d.BuildID); err != nil && !errors.Is(err, domain.ErrInvalidState) {
			s.audit(ctx, "build.cancel.failed", d.BuildID.String(), map[string]any{"deployment_id": id, "error": err.Error()})
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Dominios
// ---------------------------------------------------------------------------

// AddDomain asocia un dominio personalizado a un proyecto y genera el token de
// verificación DNS (TXT). El controller verifica y emite TLS vía cert-manager.
func (s *Service) AddDomain(ctx context.Context, projectID uuid.UUID, hostname string) (domain.Domain, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidHostname(hostname) {
		return domain.Domain{}, fmt.Errorf("%w: hostname inválido", domain.ErrInvalidInput)
	}
	// SEGURIDAD (anti domain-hijack cross-tenant): las zonas de la plataforma
	// (apps/preview) resuelven por comodín al ingress compartido, así que la
	// verificación DNS sería vacua para ellas. Los hosts canónicos <slug>.apps.… los
	// provisiona la plataforma; NUNCA son reclamables por un tenant vía AddDomain (si
	// no, un tenant ligaría <otro-slug>.apps.sytue.eu a SU proyecto y secuestraría el
	// tráfico/cert del otro). Solo se aceptan dominios propios (custom) del cliente.
	if h := strings.ToLower(strings.TrimSuffix(hostname, ".")); reservedPlatformHost(h, s.cfg.AppsDomain) || reservedPlatformHost(h, s.cfg.PreviewDomain) {
		return domain.Domain{}, fmt.Errorf("%w: no puedes reclamar un host bajo el dominio de la plataforma; usa un dominio propio", domain.ErrInvalidInput)
	}
	token := "sytue-verify=" + strings.ReplaceAll(uuid.NewString(), "-", "")
	d, err := s.store.AddDomain(ctx, projectID, hostname, token)
	if err != nil {
		return domain.Domain{}, err
	}
	s.audit(ctx, "domain.add", d.ID.String(), map[string]any{"project_id": projectID.String(), "hostname": d.Hostname})
	_ = s.emit(ctx, EventDomainAdded, t.Slug, map[string]any{"domain_id": d.ID, "hostname": d.Hostname})
	return d, nil
}

// reservedPlatformHost indica si host es la zona de la plataforma o un subdominio
// suyo (host == zone o *.zone). zone vacío ⇒ nunca reservado. Case-insensitive; host
// ya viene en minúsculas y sin punto final.
func reservedPlatformHost(host, zone string) bool {
	zone = strings.ToLower(strings.TrimSuffix(zone, "."))
	if zone == "" {
		return false
	}
	return host == zone || strings.HasSuffix(host, "."+zone)
}

// ListDomains lista los dominios de un proyecto.
func (s *Service) ListDomains(ctx context.Context, projectID uuid.UUID) ([]domain.Domain, error) {
	return s.store.ListDomains(ctx, projectID)
}

// RemoveDomain elimina un dominio de un proyecto y notifica al controller.
func (s *Service) RemoveDomain(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	d, err := s.store.RemoveDomain(ctx, id)
	if err != nil {
		return err
	}
	_ = s.emit(ctx, EventDomainRemoved, t.Slug, map[string]any{
		"domain_id": d.ID, "hostname": d.Hostname, "project_id": d.ProjectID,
	})
	s.audit(ctx, "domain.remove", d.Hostname, map[string]any{"id": d.ID})
	return nil
}

// SetDomainPrimary marca un dominio como principal del proyecto.
func (s *Service) SetDomainPrimary(ctx context.Context, id uuid.UUID) (domain.Domain, error) {
	d, err := s.store.SetDomainPrimary(ctx, id)
	if err != nil {
		return domain.Domain{}, err
	}
	s.audit(ctx, "domain.set_primary", d.Hostname, map[string]any{"id": d.ID})
	return d, nil
}

// CancelBuild cancela una build en cola o en curso.
func (s *Service) CancelBuild(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	jobName, err := s.store.CancelBuild(ctx, id)
	if err != nil {
		return err
	}
	_ = s.emit(ctx, EventBuildCanceled, t.Slug, map[string]any{"build_id": id, "job_name": jobName})
	s.audit(ctx, "build.cancel", id.String(), nil)
	return nil
}

// AuditPage es una página de auditoría con el token de continuación ya codificado
// (opaco) para la siguiente petición ("" si no hay más).
type AuditPage struct {
	Events     []store.AuditRecord
	NextCursor string
}

// ListAudit devuelve una página de la auditoría del tenant filtrada por
// actor/acción/rango, con paginación keyset. El cursor es un token opaco devuelto
// por la página anterior (vacío = primera página).
func (s *Service) ListAudit(ctx context.Context, f store.AuditFilter, cursor string, limit int) (AuditPage, error) {
	after, err := decodePageCursor(cursor)
	if err != nil {
		return AuditPage{}, err
	}
	page, err := s.store.ListAudit(ctx, f, after, limit)
	if err != nil {
		return AuditPage{}, err
	}
	return AuditPage{Events: page.Events, NextCursor: encodePageCursor(page.Next)}, nil
}

// ExportAudit vuelca la auditoría filtrada del tenant como CSV en w. Se delega al
// store para escribirlo en STREAMING: un export de compliance puede ser de decenas
// de miles de filas y no tiene por qué caber en memoria.
func (s *Service) ExportAudit(ctx context.Context, f store.AuditFilter, w io.Writer) error {
	return s.store.ExportAudit(ctx, f, w)
}

func (s *Service) audit(ctx context.Context, action, resource string, detail map[string]any) {
	sub := actorSub(ctx)
	if detail == nil {
		detail = map[string]any{}
	}
	_ = s.store.RecordAudit(ctx, store.AuditEntry{ActorSub: sub, Action: action, Resource: resource, Detail: detail})
}

// AuditExec deja en la auditoría una sesión de terminal en un pod: con duración
// negativa es la apertura; si no, el cierre (CLOUD-14).
func (s *Service) AuditExec(ctx context.Context, projectID, deploymentID uuid.UUID, duration time.Duration) {
	if duration < 0 {
		s.audit(ctx, "exec.open", deploymentID.String(), map[string]any{"project_id": projectID.String()})
		return
	}
	s.audit(ctx, "exec.close", deploymentID.String(), map[string]any{
		"project_id": projectID.String(), "seconds": int(duration.Seconds()),
	})
}

func actorSub(ctx context.Context) string {
	if c, ok := authn.FromContext(ctx); ok {
		return c.Subject
	}
	return ""
}

// IngressTarget es el CNAME al que el cliente debe apuntar su dominio.
func (s *Service) IngressTarget() string { return s.cfg.IngressTarget }

// ---------------------------------------------------------------------------
// Variables de entorno / secretos
// ---------------------------------------------------------------------------

// SetEnvInput son los datos para fijar una variable de entorno o secreto.
type SetEnvInput struct {
	ProjectID uuid.UUID
	Key       string
	Value     string
	IsSecret  bool
	Scope     domain.EnvScope
}

// SetEnv crea/actualiza una variable. Si IsSecret, cifra el valor en reposo.
func (s *Service) SetEnv(ctx context.Context, in SetEnvInput) (domain.EnvVar, error) {
	if !domain.ValidEnvKey(in.Key) {
		return domain.EnvVar{}, fmt.Errorf("%w: clave de variable inválida (use MAYUS_CON_GUION_BAJO)", domain.ErrInvalidInput)
	}
	if in.Scope == "" {
		in.Scope = domain.ScopeAll
	}
	if !domain.ValidScope(in.Scope) {
		return domain.EnvVar{}, fmt.Errorf("%w: scope inválido", domain.ErrInvalidInput)
	}
	rec := store.EnvVarRecord{
		ProjectID: in.ProjectID,
		Key:       in.Key,
		IsSecret:  in.IsSecret,
		Scope:     in.Scope,
	}
	if in.IsSecret {
		enc, err := s.sealer.SealString(in.Value)
		if err != nil {
			return domain.EnvVar{}, fmt.Errorf("service: cifrar secreto: %w", err)
		}
		rec.ValueEnc = enc
	} else {
		rec.ValuePlain = in.Value
	}
	v, err := s.store.UpsertEnvVar(ctx, rec)
	if err != nil {
		return domain.EnvVar{}, err
	}
	s.audit(ctx, "env.set", in.ProjectID.String(), map[string]any{"key": in.Key, "scope": string(in.Scope), "secret": in.IsSecret})
	return v, nil
}

// ListEnv lista las variables de un proyecto (sin valores de secretos).
func (s *Service) ListEnv(ctx context.Context, projectID uuid.UUID) ([]domain.EnvVar, error) {
	return s.store.ListEnvVars(ctx, projectID)
}

// ExportEnv devuelve las variables del proyecto en formato `.env`, con los
// secretos EN CLARO.
//
// Es la simétrica de la importación, que existe desde hace tres tandas: se podían
// pegar veinte variables de golpe y no había forma de recuperarlas. Sin esto,
// levantar el proyecto en local significa copiar los valores a mano de una
// pantalla que además oculta los secretos — o sea, no poder.
//
// Se resuelven POR ENTORNO, con la misma consulta que usa un despliegue: lo que
// baja es exactamente lo que la app recibe en producción (o en preview), no una
// mezcla de todo. Las credenciales de los servicios gestionados NO entran: las
// inyecta el controlador al desplegar y su DSN apunta dentro del clúster, así que
// en un `.env` local no valdrían; la cadena de conexión se obtiene en la pantalla
// de la base de datos.
//
// Revela secretos, así que va con admin/owner, solo humanos, y queda auditado.
func (s *Service) ExportEnv(ctx context.Context, projectID uuid.UUID, target domain.DeployTarget) (string, error) {
	if target != domain.TargetPreview {
		target = domain.TargetProduction
	}
	t := tenancy.MustFrom(ctx)
	vars, err := s.pstore.ResolveEnvForDeploy(ctx, t.Slug, projectID, target)
	if err != nil {
		return "", err
	}
	// Orden alfabético: un fichero que cambia de orden en cada descarga es
	// imposible de comparar con el anterior, que es justo lo que se hace con un
	// .env cuando algo no arranca.
	sort.Slice(vars, func(i, j int) bool { return vars[i].Key < vars[j].Key })

	var b strings.Builder
	b.WriteString("# " + string(target) + "\n")
	for _, v := range vars {
		value := v.Value
		if v.IsSecret {
			plain, derr := s.sealer.OpenString(v.ValueEnc)
			if derr != nil {
				return "", fmt.Errorf("service: descifrar %s: %w", v.Key, derr)
			}
			value = plain
		}
		b.WriteString(v.Key + "=" + dotenvQuote(value) + "\n")
	}
	s.audit(ctx, "env.export", projectID.String(), map[string]any{
		"target": string(target), "count": len(vars),
	})
	return b.String(), nil
}

// dotenvQuote entrecomilla un valor si hace falta para que vuelva a leerse igual.
//
// Sin comillas, un valor con un espacio o con una almohadilla se lee cortado —o
// como comentario— al volver a cargarlo, y un salto de línea rompe el fichero
// entero convirtiendo el resto del valor en claves inventadas. El criterio es el
// del parser que ya tiene la importación: comillas dobles, y dentro se escapan la
// barra, la comilla y el salto.
func dotenvQuote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\n\r\"'#$`\\") {
		return v
	}
	// El orden importa: la barra se duplica PRIMERO, o al escapar la comilla se
	// volvería a tocar la barra que acabamos de meter. NewReplacer no encadena
	// (aplica cada regla una vez, de izquierda a derecha sobre el original), así
	// que aquí está resuelto; se deja dicho porque con un bucle de Replace sí
	// pasaría.
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

// EnvScope devuelve el ámbito de una variable (para decidir quién la borra).
func (s *Service) EnvScope(ctx context.Context, id uuid.UUID) (domain.EnvScope, error) {
	return s.store.EnvVarScope(ctx, id)
}

// DeleteEnv borra una variable.
func (s *Service) DeleteEnv(ctx context.Context, id uuid.UUID) error {
	if err := s.store.DeleteEnvVar(ctx, id); err != nil {
		return err
	}
	s.audit(ctx, "env.delete", id.String(), nil)
	return nil
}

// ---------------------------------------------------------------------------
// Servicios gestionados
// ---------------------------------------------------------------------------

// ProvisionServiceInput son los datos para aprovisionar un servicio gestionado.
type ProvisionServiceInput struct {
	ProjectID uuid.UUID
	Kind      domain.ServiceKind
	Name      string
	Config    map[string]string
	// Target: a qué despliegues entrega sus credenciales. Vacío = producción.
	Target domain.DeployTarget
}

// ProvisionService crea un servicio gestionado (postgres/storage/functions/cron/
// auth) en estado 'provisioning'; el controller lo materializa e inyecta sus
// credenciales en los deployments del proyecto.
func (s *Service) ProvisionService(ctx context.Context, in ProvisionServiceInput) (domain.ManagedService, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidServiceKind(in.Kind) {
		return domain.ManagedService{}, fmt.Errorf("%w: tipo de servicio inválido", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = string(in.Kind)
	}
	if err := validateServiceTarget(&in); err != nil {
		return domain.ManagedService{}, err
	}
	org, err := s.store.GetOrganization(ctx)
	if err != nil {
		return domain.ManagedService{}, err
	}
	quota := domain.QuotasFor(org.Plan)
	if in.Config == nil {
		in.Config = map[string]string{}
	}
	switch in.Kind {
	case domain.ServicePostgres:
		// El tamaño de disco viene del mapa de configuración LIBRE, así que hasta
		// aquí llegaba tal cual: un "grande" se persistía sin una queja y reventaba
		// después en el reconciliador. La contención de pánicos sigue estando, pero
		// es la red y no el suelo — sin esto el servicio se queda en 'provisioning'
		// para siempre y el cliente no se entera de por qué. El error sale AQUÍ,
		// donde hay alguien mirando.
		if err := domain.ValidatePVCSizeForPlan(in.Config["pvc_size"], quota.PostgresPVCSize); err != nil {
			return domain.ManagedService{}, err
		}
		if in.Config["pvc_size"] == "" {
			in.Config["pvc_size"] = quota.PostgresPVCSize
		}
		// La programación de la copia la elige el cliente; si viene, tiene que ser
		// un cron válido: una expresión mal escrita deja el CronJob rechazado por
		// el API server y el servicio SIN COPIAS, en silencio.
		if sch := strings.TrimSpace(in.Config["backup_schedule"]); sch != "" {
			if !domain.ValidCronSchedule(sch) {
				return domain.ManagedService{}, fmt.Errorf(
					"%w: la programación de la copia debe ser una expresión cron válida", domain.ErrInvalidInput,
				)
			}
			in.Config["backup_schedule"] = sch
		}
		if in.Config["backup_retention_days"] == "" {
			in.Config["backup_retention_days"] = strconv.Itoa(quota.BackupRetentionDays)
		}
	case domain.ServiceCron, domain.ServiceJob, domain.ServiceFunctions, domain.ServiceWorker:
		if err := s.validateTaskService(ctx, in); err != nil {
			return domain.ManagedService{}, err
		}
	case domain.ServiceApp:
		// La app se elige del catálogo por su id; la imagen no viaja nunca desde el
		// cliente (ADR 0029). Se comprueba AQUÍ, donde hay alguien mirando: si se
		// dejara para el reconciliador, un id mal escrito dejaría el servicio en
		// «provisioning» para siempre sin decir por qué.
		if err := s.validateCatalogApp(in.Config["app_id"]); err != nil {
			return domain.ManagedService{}, err
		}
		if err := s.prepararApp(ctx, &in); err != nil {
			return domain.ManagedService{}, err
		}
	case domain.ServiceStorage, domain.ServiceAuth:
		// Sin defaults ni validación extra: el aprovisionamiento los resuelve entero.
	}
	m, err := s.store.ProvisionService(ctx, store.NewServiceRecord{
		ProjectID: in.ProjectID,
		Kind:      in.Kind,
		Name:      in.Name,
		Config:    in.Config,
		Target:    in.Target,
	}, quota.MaxManagedServices)
	if err != nil {
		return domain.ManagedService{}, err
	}
	// Aparecer cambia el entorno de las hermanas: las que se integran con esta
	// vuelven a la cola de aprovisionamiento para recogerla.
	s.reencolarIntegrantes(ctx, in.ProjectID, in.Config["app_id"])
	_ = s.emit(ctx, EventServiceRequested, t.Slug, map[string]any{
		"service_id": m.ID, "kind": string(m.Kind), "project_id": m.ProjectID,
	})
	s.audit(ctx, "service.provision", m.ID.String(), map[string]any{
		"kind": string(m.Kind), "project_id": m.ProjectID.String(), "command": in.Config["command"] != "",
	})
	return m, nil
}

// validateServiceTarget normaliza el destino del servicio y rechaza los que no
// tienen sentido.
//
// Solo una base de datos o un almacén pueden ser «de vista previa»: son los que
// existen para dar credenciales a la app, y una vista previa necesita los suyos
// para no tocar los de producción. Una función, una tarea o una app del catálogo
// no son un recurso de la vista previa sino otra pieza que corre —y una app del
// catálogo guarda sus datos en el Postgres de producción—, así que admitirlas
// aquí reabriría por otro lado lo que este campo cierra.
func validateServiceTarget(in *ProvisionServiceInput) error {
	switch in.Target {
	case "", domain.TargetProduction:
		in.Target = domain.TargetProduction
		return nil
	case domain.TargetPreview:
		if in.Kind == domain.ServicePostgres || in.Kind == domain.ServiceStorage {
			return nil
		}
		return fmt.Errorf("%w: solo una base de datos o un almacén pueden crearse para vistas previas",
			domain.ErrInvalidInput)
	default:
		return fmt.Errorf("%w: destino de servicio inválido (production|preview)", domain.ErrInvalidInput)
	}
}

// prepararApp resuelve lo que una app del catálogo necesita saber del PROYECTO,
// que es lo que el reconciliador no sabe: si ya está puesta y con qué hostname se
// publica.
//
// # Una sola instancia de cada app por proyecto
//
// No es una limitación de la implementación, es lo que hace que las otras dos
// promesas signifiquen algo. «Se conectan solas» necesita que «la caché del
// proyecto» sea UNA —con dos, la integración elegiría una por orden de listado, y
// el mismo proyecto arrancaría distinto en cada despliegue—, y el hostname
// público se deriva del id del catálogo, así que la segunda pisaría a la primera.
// Quien de verdad quiera dos levanta otro proyecto, que además las aísla.
//
// # El hostname se decide aquí
//
// Porque aquí está el slug del proyecto y el dominio de apps, y el reconciliador
// no tiene ninguno de los dos. Viaja en Config —que es público, no sensible— así
// que el panel puede enseñar la URL desde el primer segundo, sin esperar a que el
// Ingress exista.
func (s *Service) prepararApp(ctx context.Context, in *ProvisionServiceInput) error {
	appID := in.Config["app_id"]
	hermanas, err := s.store.ListServices(ctx, in.ProjectID)
	if err != nil {
		return err
	}
	for _, h := range hermanas {
		if h.Kind == domain.ServiceApp && h.Config["app_id"] == appID && h.Status != domain.ServiceDeleting {
			return fmt.Errorf("%w: este proyecto ya tiene %s levantada", domain.ErrInvalidInput, appID)
		}
	}

	cat, err := s.AppCatalog()
	if err != nil {
		return err
	}
	for _, a := range cat.Apps {
		if a.ID != appID {
			continue
		}
		// Lo que la app necesita para arrancar tiene que estar YA. Descubrirlo en
		// el contenedor significa un pod reiniciándose sin decir por qué; aquí es
		// una frase que dice exactamente qué falta y dónde crearlo.
		if err := exigeRequisitos(a, hermanas); err != nil {
			return err
		}
		if a.WebPort > 0 {
			in.Config["web_host"] = domain.HostnameDeApp(in.ProjectID, appID, s.cfg.AppsDomain)
			in.Config["web_port"] = strconv.Itoa(a.WebPort)
		}
	}
	return nil
}

// exigeRequisitos comprueba que el proyecto tiene los servicios gestionados que
// la app declara en `needs`.
//
// Se mira el estado además del tipo: un Postgres a medio aprovisionar todavía no
// tiene credenciales, así que la app arrancaría contra un servidor que no
// responde. Esperar a que esté listo es cosa de quien lo crea, no un estado que
// esta app deba aguantar.
func exigeRequisitos(a domain.CatalogApp, hermanas []domain.ManagedService) error {
	for _, n := range a.Needs {
		if n != domain.AppNeedPostgres {
			continue
		}
		listo := false
		for _, h := range hermanas {
			// El de producción: una app del catálogo no guarda sus datos en la base
			// de las vistas previas.
			if h.Kind == domain.ServicePostgres && h.Status == domain.ServiceReady && h.Target != domain.TargetPreview {
				listo = true
				break
			}
		}
		if !listo {
			return fmt.Errorf(
				"%w: %s guarda sus datos en PostgreSQL y este proyecto no tiene ninguno listo. "+
					"Crea antes un Sytue Postgres desde la pestaña Base de datos",
				domain.ErrInvalidInput, a.Name,
			)
		}
	}
	return nil
}

// reencolarIntegrantes devuelve a 'provisioning' las apps del proyecto que
// declaran integración con appID, porque acaba de aparecer o de irse.
//
// Es la mitad que hace real «se integran aunque los elijas de uno en uno». Sin
// esto la integración solo funcionaría en un orden —crear primero la caché y
// después el CMS—, y al revés no pasaría nada: ni error, ni aviso, ni conexión.
// La causa es la asimetría de `reconcileServices`, que solo aprovisiona lo que
// está en 'provisioning': un servicio 'ready' no se vuelve a mirar nunca.
//
// No devuelve error a propósito. Lo que se pidió —crear o borrar el servicio— ya
// está hecho y confirmado; fallar aquí haría que el cliente reintentara y creara
// un segundo servicio. Se avisa en el log y el siguiente cambio del proyecto
// vuelve a intentarlo.
func (s *Service) reencolarIntegrantes(ctx context.Context, projectID uuid.UUID, appID string) {
	if appID == "" || projectID == uuid.Nil {
		return
	}
	cat, err := s.AppCatalog()
	if err != nil {
		log.From(ctx).Warn("catálogo de apps al reencolar integraciones", "error", err)
		return
	}
	hermanas, err := s.store.ListServices(ctx, projectID)
	if err != nil {
		log.From(ctx).Warn("listar servicios al reencolar integraciones", "error", err)
		return
	}
	afectadas := domain.IntegratorsOf(cat, appID)
	var ids []uuid.UUID
	for _, h := range hermanas {
		if h.Kind != domain.ServiceApp {
			continue
		}
		if slices.Contains(afectadas, h.Config["app_id"]) {
			ids = append(ids, h.ID)
		}
	}
	if err := s.store.ReprovisionServices(ctx, ids); err != nil {
		log.From(ctx).Warn("reencolar integraciones", "error", err, "servicios", len(ids))
	}
}

// AppCatalog devuelve el catálogo de apps open-source resuelto contra el
// registro configurado.
func (s *Service) AppCatalog() (domain.AppCatalog, error) {
	return domain.LoadAppCatalog(s.cfg.RegistryURL)
}

// validateCatalogApp comprueba que el id pedido existe en el catálogo Y que su
// imagen está replicada.
//
// Distingue los dos casos porque son dos respuestas distintas para quien lo lee:
// «esa app no existe» se arregla escribiendo bien el nombre, y «todavía no está
// disponible» no se arregla desde el panel.
func (s *Service) validateCatalogApp(id string) error {
	cat, err := s.AppCatalog()
	if err != nil {
		return err
	}
	for _, a := range cat.Apps {
		if a.ID == id {
			return nil
		}
	}
	for _, pendiente := range cat.PendingMirror {
		if pendiente == id {
			return fmt.Errorf("%w: la app %q todavía no está disponible", domain.ErrInvalidInput, id)
		}
	}
	return fmt.Errorf("%w: no existe la app %q en el catálogo", domain.ErrInvalidInput, id)
}

// validateTaskService valida los servicios que EJECUTAN algo (cron, job,
// function): su horario, su destino y la imagen con la que corren.
func (s *Service) validateTaskService(ctx context.Context, in ProvisionServiceInput) error {
	if in.Kind == domain.ServiceCron && !domain.ValidCronSchedule(in.Config["schedule"]) {
		return fmt.Errorf("%w: cron requiere 'schedule' válido (cron o @macro)", domain.ErrInvalidInput)
	}
	// Tarea programada que solo LLAMA A UNA URL: no hay imagen que construir ni
	// que validar, porque la ejecuta la imagen de plataforma. La URL pasa por el
	// mismo filtro anti-SSRF que un webhook de deploy: sin él, un cron sería la
	// forma más cómoda de sondear la red interna del clúster cada minuto.
	if in.Kind == domain.ServiceCron && strings.TrimSpace(in.Config["url"]) != "" {
		if err := domain.ValidateWebhookURL(in.Config["url"]); err != nil {
			return err
		}
		if m := strings.ToUpper(strings.TrimSpace(in.Config["method"])); m != "" && !validHTTPTaskMethod(m) {
			return fmt.Errorf("%w: método HTTP no admitido en una tarea programada: %s", domain.ErrInvalidInput, m)
		}
		return nil
	}
	// El nombre de una función es la etiqueta DNS de su host público y del cert
	// TLS (<name>.<projID>.fn.<dominio>): DEBE ser una etiqueta DNS-1123 válida, o
	// (1) un nombre >63 chars da un cert ACME rechazado (función inalcanzable por
	// HTTPS sin error) y (2) dos nombres que dnsSafe colapsa al mismo host (p.ej.
	// 'api'/'Api', 'my.fn') son filas distintas del índice único pero el MISMO
	// host ⇒ dos Ingress en pugna por el Secret TLS. ValidSlug ya impone esa regla.
	if in.Kind == domain.ServiceFunctions && !domain.ValidSlug(in.Name) {
		return fmt.Errorf("%w: el nombre de la función debe ser una etiqueta DNS válida (minúsculas [a-z0-9-], 1-40 caracteres)", domain.ErrInvalidInput)
	}
	// cron/job/functions corren una imagen: DEBE residir en el repo del propio
	// proyecto (aislamiento); resolver el slug vía el ProjectID (bajo RLS) para
	// anclarla con ValidateImageRef.
	proj, err := s.store.GetProject(ctx, in.ProjectID)
	if err != nil {
		return err
	}
	return domain.ValidateImageRef(in.Config["image"], s.cfg.RegistryURL, proj.Slug)
}

// validHTTPTaskMethod admite los métodos con los que una tarea programada puede
// llamar a una aplicación. Se deja fuera todo lo que no sea una llamada normal
// (TRACE, CONNECT): una tarea automática no tiene por qué poder hacerlas.
func validHTTPTaskMethod(m string) bool {
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD":
		return true
	default:
		return false
	}
}

// UpdateServiceInput es un cambio parcial sobre un servicio gestionado.
type UpdateServiceInput struct {
	ServiceID uuid.UUID
	// Name nil = sin cambio.
	Name *string
	// Config se MEZCLA con la que hay: solo las claves presentes.
	Config map[string]string
}

// UpdateService cambia la configuración de un servicio gestionado y lo devuelve a
// la cola de aprovisionamiento.
//
// Hasta ahora un servicio era inmutable: equivocarse en la programación de un
// cron obligaba a borrarlo y crearlo otra vez —perdiendo su historial— y el
// horario de la copia de la base de datos no se podía cambiar nunca.
//
// La validación es la MISMA que en el alta (validateTaskService, el parser de
// cron, el anclaje de la imagen al repo del tenant): un servicio editado tiene
// que cumplir exactamente lo que cumpliría uno recién creado, o el aislamiento
// entre inquilinos dependería de por qué camino se llegó.
func (s *Service) UpdateService(ctx context.Context, in UpdateServiceInput) (domain.ManagedService, error) {
	cur, err := s.store.GetService(ctx, in.ServiceID)
	if err != nil {
		return domain.ManagedService{}, err
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return domain.ManagedService{}, fmt.Errorf("%w: el nombre no puede quedar vacío", domain.ErrInvalidInput)
	}
	// La validación mira la configuración RESULTANTE (la de ahora mezclada con el
	// cambio): validar solo lo que llega dejaría pasar un cron al que se le quita
	// la URL y se queda sin destino.
	merged := make(map[string]string, len(cur.Config)+len(in.Config))
	for k, v := range cur.Config {
		merged[k] = v
	}
	for k, v := range in.Config {
		merged[k] = v
	}
	name := cur.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if err := s.validateServiceConfig(ctx, cur, name, merged); err != nil {
		return domain.ManagedService{}, err
	}
	m, err := s.store.UpdateServiceConfig(ctx, in.ServiceID, in.Name, in.Config)
	if err != nil {
		return domain.ManagedService{}, err
	}
	s.audit(ctx, "service.update", in.ServiceID.String(), map[string]any{"kind": string(cur.Kind)})
	return m, nil
}

// validateServiceConfig aplica a un servicio YA EXISTENTE las mismas reglas que
// al crearlo.
func (s *Service) validateServiceConfig(
	ctx context.Context, cur domain.ManagedService, name string, cfg map[string]string,
) error {
	switch cur.Kind {
	case domain.ServiceCron, domain.ServiceJob, domain.ServiceFunctions, domain.ServiceWorker:
		return s.validateTaskService(ctx, ProvisionServiceInput{
			ProjectID: cur.ProjectID, Kind: cur.Kind, Name: name, Config: cfg,
		})
	case domain.ServicePostgres:
		org, err := s.store.GetOrganization(ctx)
		if err != nil {
			return err
		}
		quota := domain.QuotasFor(org.Plan)
		if err := domain.ValidatePVCSizeForPlan(cfg["pvc_size"], quota.PostgresPVCSize); err != nil {
			return err
		}
		if sch := strings.TrimSpace(cfg["backup_schedule"]); sch != "" && !domain.ValidCronSchedule(sch) {
			return fmt.Errorf("%w: la programación de la copia debe ser una expresión cron válida",
				domain.ErrInvalidInput)
		}
		return nil
	case domain.ServiceApp:
		// La app la fija el catálogo y su id NO se puede cambiar: sería otra app
		// distinta con el historial de esta.
		if id, ok := cfg["app_id"]; ok && id != cur.Config["app_id"] {
			return fmt.Errorf("%w: la app del catálogo no se puede cambiar; crea otra", domain.ErrInvalidInput)
		}
		return nil
	case domain.ServiceStorage, domain.ServiceAuth:
		return nil
	default:
		return nil
	}
}

// GetService devuelve un servicio gestionado por ID.
func (s *Service) GetService(ctx context.Context, id uuid.UUID) (domain.ManagedService, error) {
	return s.store.GetService(ctx, id)
}

// ListServices lista los servicios gestionados de un proyecto.
func (s *Service) ListServices(ctx context.Context, projectID uuid.UUID) ([]domain.ManagedService, error) {
	return s.store.ListServices(ctx, projectID)
}

// DeleteService marca un servicio gestionado para eliminación.
func (s *Service) DeleteService(ctx context.Context, id uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	// Se lee ANTES de borrarlo: después ya no se sabe qué app era ni de qué
	// proyecto, y las hermanas se quedarían apuntando a un host que ya no existe
	// —arrancando contra una caché muerta— sin que nada lo dijera.
	borrado, err := s.store.GetService(ctx, id)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if err := s.store.DeleteService(ctx, id); err != nil {
		return err
	}
	if borrado.Kind == domain.ServiceApp {
		s.reencolarIntegrantes(ctx, borrado.ProjectID, borrado.Config["app_id"])
	}
	s.audit(ctx, "service.delete", id.String(), map[string]any{"kind": string(borrado.Kind)})
	return s.emit(ctx, EventServiceDeleted, t.Slug, map[string]any{"service_id": id})
}

// ---------------------------------------------------------------------------
// Uso / facturación
// ---------------------------------------------------------------------------

// UsageSummary devuelve el uso agregado del tenant en [from, to).
func (s *Service) UsageSummary(ctx context.Context, from, to time.Time) ([]store.UsageAggregate, error) {
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.AddDate(0, -1, 0)
	}
	return s.store.SummarizeUsage(ctx, from, to)
}

// UsageDetail lista los registros de uso del tenant en [from, to).
func (s *Service) UsageDetail(ctx context.Context, from, to time.Time, limit int) ([]domain.UsageRecord, error) {
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.AddDate(0, -1, 0)
	}
	return s.store.ListUsage(ctx, from, to, limit)
}
