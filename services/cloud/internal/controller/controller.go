// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package controller implementa el reconciliador de Sytue Cloud: lleva el estado
// deseado (deployments, dominios, servicios gestionados) al estado real del
// clúster Kapsule. Despliega apps de clientes en namespaces tenant (o Serverless
// Containers), configura dominios + TLS (cert-manager), gestiona previews por PR
// y aplica ResourceQuota/NetworkPolicy; además inyecta las credenciales de los
// servicios gestionados en los Deployments del proyecto.
//
// El Platform es una interfaz: la implementación de producción usa client-go
// contra Kapsule; en dev se usa un stub que registra las acciones. El bucle de
// reconciliación es idempotente y se ejecuta cada ReconcileInterval, además de
// reaccionar a los eventos del bus (events.Channel "cloud").
package controller

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// reactiveDebounce agrupa una ráfaga de NOTIFY (build + deploy + domain del mismo
// push) en una sola reconciliación reactiva.
const reactiveDebounce = 300 * time.Millisecond

// storeAPI es la superficie de persistencia que consume el reconciliador. La
// satisface *store.Store (producción) y un fake en tests, permitiendo ejercitar
// el bucle sin PostgreSQL.
type storeAPI interface {
	ListPendingServices(ctx context.Context, limit int) ([]domain.ManagedService, error)
	MarkServiceReady(ctx context.Context, tenant string, id uuid.UUID, config map[string]string, credsEnc []byte) error
	FinalizeServiceDeletion(ctx context.Context, tenant string, id uuid.UUID) error
	ListPendingDeployments(ctx context.Context, limit int) ([]store.PendingDeployment, error)
	ListRetirableDeployments(ctx context.Context, limit int) ([]store.RetirableDeployment, error)
	ListReadyProductions(ctx context.Context, since time.Time, limit int) ([]store.PendingDeployment, error)
	GetBuild(ctx context.Context, id uuid.UUID) (domain.Build, error)
	SetDeploymentStatus(ctx context.Context, tenant string, id uuid.UUID, status domain.DeployStatus, url, errMsg string) error
	MarkDeploymentTornDown(ctx context.Context, tenant string, id uuid.UUID) error
	Promote(ctx context.Context, id uuid.UUID) (domain.Deployment, error)
	ResolveEnvForDeploy(ctx context.Context, tenant string, projectID uuid.UUID, target domain.DeployTarget) ([]store.ResolvedEnvVar, error)
	ResolveServiceCreds(ctx context.Context, tenant string, projectID uuid.UUID, target domain.DeployTarget) (map[string][]byte, error)
	PreviewWithheld(ctx context.Context, tenant string, projectID uuid.UUID) (store.PreviewWithheld, error)
	ListDomains(ctx context.Context, projectID uuid.UUID) ([]domain.Domain, error)
	ListPendingDomains(ctx context.Context, limit int) ([]domain.Domain, error)
	SetDomainStatus(ctx context.Context, tenant string, id uuid.UUID, status domain.DomainStatus) error
	GetOrganizationPlanByTenant(ctx context.Context, tenant string) (domain.Plan, error)
	ClaimComputeUsage(ctx context.Context, now time.Time, minWindow, maxWindow time.Duration, limit int) ([]store.ComputeUsage, error)
	RecordUsage(ctx context.Context, in store.UsageInput) error
	RecordResourceSamples(ctx context.Context, in []store.ResourceSampleInput) error
	PurgeResourceSamples(ctx context.Context, before time.Time) (int64, error)
	CollectCohortUsage(ctx context.Context, since time.Time) ([]domain.ProjectUsage, error)
	PublishCohorts(ctx context.Context, cohorts []domain.Cohort) error
	ListProjectsToPurge(ctx context.Context, limit int) ([]store.ProjectToPurge, error)
	MarkProjectPurged(ctx context.Context, id uuid.UUID) error
}

// Controller reconcilia el estado del control plane con Kapsule.
type Controller struct {
	cfg      config.Config
	store    storeAPI
	sealer   *crypto.Sealer
	platform Platform
	provider ServiceProvisioner
	// purgeRepo borra las imágenes de un slug del registry (CLOUD-08).
	purgeRepo func(ctx context.Context, slug string) error
	// wake dispara una reconciliación reactiva; buffer 1 => coalescing.
	wake chan struct{}
	// lastSample espacia el muestreo de uso real, que va MUCHO más lento que el
	// ciclo (ver sampleEvery en rightsizing.go). Con mutex porque una
	// reconciliación reactiva puede solaparse con la del ticker.
	sampleMu   sync.Mutex
	lastSample time.Time
	// lastCohorts espacia el recálculo de los benchmarks anónimos, que va aún más
	// lento que el muestreo (ver cohortEvery en rightsizing.go).
	cohortMu    sync.Mutex
	lastCohorts time.Time
}

// New construye el Controller.
func New(cfg config.Config, svc *service.Service, platform Platform, provider ServiceProvisioner) *Controller {
	return &Controller{
		cfg:      cfg,
		store:    svc.Store(),
		sealer:   svc.Sealer(),
		platform: platform,
		provider: provider,
		wake:     make(chan struct{}, 1),

		purgeRepo: newRegistryPurger(cfg),
	}
}

// Wake solicita una reconciliación reactiva sin bloquear. Varias llamadas
// mientras hay una señal pendiente colapsan en una sola pasada (el canal tiene
// buffer 1): evita la tormenta de reconciles ante ráfagas de eventos.
func (c *Controller) Wake() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// retireWindow es la ventana de recencia (sobre updated_at) que acota
// reconcileProduction: solo (re)aplica producciones ready modificadas dentro de
// ella (las conmutaciones recientes: deploy/Promote/rollback bumpean updated_at),
// evitando reaplicar cada ciclo las producciones estables (cuello a escala).
// Cubre varios ciclos del ticker para sobrevivir a reinicios breves, suelo 5 min.
// El teardown ya NO usa ventana: torn_down_at marca cada deployment desmontado
// exactamente una vez (ver reconcileTeardown), así que no depende de la recencia.
func (c *Controller) retireWindow() time.Duration {
	if w := 8 * c.cfg.ReconcileInterval; w > 5*time.Minute {
		return w
	}
	return 5 * time.Minute
}

// Run ejecuta el bucle de reconciliación hasta que se cancele el contexto.
func (c *Controller) Run(ctx context.Context) error {
	l := log.From(ctx).With("component", "cloud-controller")
	l.Info("controller arrancado", "interval", c.cfg.ReconcileInterval.String())
	ticker := time.NewTicker(c.cfg.ReconcileInterval)
	defer ticker.Stop()

	// Primera reconciliación inmediata.
	c.reconcile(ctx, l)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.reconcile(ctx, l)
		case <-c.wake:
			// Reactivo: un NOTIFY del bus despierta el bucle sin esperar al ticker
			// (menor latencia de deploy/rollback). Debounce corto para agrupar la
			// ráfaga; el poll de 15s sigue como red de seguridad.
			if !sleepCtx(ctx, reactiveDebounce) {
				return nil
			}
			c.reconcile(ctx, l)
		}
	}
}

// sleepCtx duerme d o hasta que se cancela el contexto; devuelve false si se canceló.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// guard ejecuta fn conteniendo cualquier pánico y devolviéndolo como error (con
// la traza, que si no se perdería). El reconciliador procesa datos de TODOS los
// inquilinos en un mismo proceso: un pánico sin contener no falla una operación,
// tumba el binario y lo deja en CrashLoop, con lo que nadie más se reconcilia. Es
// decir, un inquilino cualquiera podía dejar sin servicio a los demás (bastaba un
// `pvc_size` que resource.MustParse no supiera leer). La contención es genérica a
// propósito: cubre la clase entera, no el pánico conocido.
//
// NO sustituye a la validación de entrada, que va antes y en su sitio (capa
// service, al persistir): esto es la red, no el suelo.
func guard(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pánico contenido: %v\n%s", r, debug.Stack())
		}
	}()
	return fn()
}

// reconcile ejecuta un ciclo completo. Cada paso es idempotente y aísla sus
// errores —y sus pánicos: un paso que revienta se registra y los demás siguen
// corriendo—. Orden: acceso runtime (incluye tenants heredados) → servicios
// (asegura namespaces + aprovisiona) → roles de
// PostgREST (repara instancias heredadas, ver pgroles.go) → deployments
// (build/deploy) → producción (fija la producción deseada viva) → teardown
// (desmonta objetos de deployments terminados) → dominios (TLS).
func (c *Controller) reconcile(ctx context.Context, l logLike) {
	for _, step := range []struct {
		msg string
		fn  func(context.Context) error
	}{
		{"reconciliar acceso runtime de tenants", c.reconcileTenantRuntimeAccess},
		{"reconciliar servicios", c.reconcileServices},
		{"reparar roles de PostgREST", c.reconcilePostgresRoles},
		{"reconciliar deployments", c.reconcileDeployments},
		{"reconciliar producción", c.reconcileProduction},
		{"retirar deployments", c.reconcileTeardown},
		{"reconciliar dominios", c.reconcileDomains},
		{"purgar imágenes de proyectos borrados", c.reconcilePurge},
		{"medir cómputo", c.reconcileCompute},
		{"muestrear uso real", c.reconcileResourceSamples},
		{"recalcular benchmarks", c.reconcileCohorts},
	} {
		if err := guard(func() error { return step.fn(ctx) }); err != nil {
			l.Warn(step.msg, "error", err)
		}
	}
}

// reconcileServices aprovisiona/elimina servicios gestionados pendientes.
func (c *Controller) reconcileServices(ctx context.Context) error {
	pending, err := c.store.ListPendingServices(ctx, 50)
	if err != nil {
		return err
	}
	for _, m := range pending {
		l := log.From(ctx).With("service_id", m.ID, "kind", string(m.Kind), "tenant", m.Tenant)
		switch m.Status { //nolint:exhaustive // Ready/Failed son estados terminales: no se reconcilian.
		case domain.ServiceProvisioning:
			// El namespace del tenant (con ResourceQuota/LimitRange) debe existir
			// ANTES de aprovisionar: si el proyecto pide Postgres/Storage antes de que
			// un deployment cree tenant-<slug>, los Secret/PVC nacerían sin cuota.
			tnt, err := tenancy.New(m.Tenant)
			if err != nil {
				l.Warn("tenant inválido en servicio", "error", err)
				continue
			}
			if err := c.platform.EnsureNamespace(ctx, tnt.Namespace(), m.Tenant, c.tenantQuota(ctx, m.Tenant)); err != nil {
				l.Warn("asegurar namespace antes de aprovisionar", "error", err)
				continue
			}
			// Contención POR SERVICIO: el aprovisionador parsea configuración del
			// cliente (pvc_size, etc.). Un valor que haga pánico falla ESTE servicio
			// y el bucle sigue con el resto; sin esto se llevaba por delante todo el
			// paso, ciclo tras ciclo.
			var res ProvisionResult
			if err := guard(func() (perr error) { res, perr = c.provider.Provision(ctx, m); return perr }); err != nil {
				l.Warn("aprovisionar servicio fallido", "error", err)
				continue
			}
			// Cifra las credenciales en reposo antes de persistirlas. Sin credenciales
			// (cron/job/functions) => creds_enc NULL: Seal(nil) NO es no-op (daría un
			// blob no-nulo que ResolveServiceCreds inyectaría como secreto vacío en las
			// apps del proyecto).
			var credsEnc []byte
			if len(res.Credentials) > 0 {
				credsEnc, err = c.sealer.Seal(res.Credentials)
				if err != nil {
					l.Warn("cifrar credenciales de servicio", "error", err)
					continue
				}
			}
			if err := c.store.MarkServiceReady(ctx, m.Tenant, m.ID, res.Config, credsEnc); err != nil {
				l.Warn("marcar servicio ready", "error", err)
				continue
			}
			l.Info("servicio gestionado aprovisionado")
		case domain.ServiceDeleting:
			if err := guard(func() error { return c.provider.Deprovision(ctx, m) }); err != nil {
				l.Warn("desaprovisionar servicio", "error", err)
				continue
			}
			if err := c.store.FinalizeServiceDeletion(ctx, m.Tenant, m.ID); err != nil {
				l.Warn("finalizar borrado de servicio", "error", err)
				continue
			}
			l.Info("servicio gestionado eliminado")
		}
	}
	return nil
}

// reconcileDeployments lleva los deployments no terminales a 'ready' (o 'error').
func (c *Controller) reconcileDeployments(ctx context.Context) error {
	pending, err := c.store.ListPendingDeployments(ctx, 50)
	if err != nil {
		return err
	}
	for _, pd := range pending {
		// reconcileDeployment vive en deploy.go (despliegue + inyección de creds).
		// Contenido por deployment: uno envenenado no detiene la cola de los demás.
		if err := guard(func() error {
			c.reconcileDeployment(ctx, pd.Deployment, pd.Project)
			return nil
		}); err != nil {
			log.From(ctx).Warn("reconciliar deployment", "error", err, "deployment_id", pd.Deployment.ID)
		}
	}
	return nil
}

// reconcileDomains verifica DNS y gestiona la emisión de TLS de dominios.
func (c *Controller) reconcileDomains(ctx context.Context) error {
	pending, err := c.store.ListPendingDomains(ctx, 50)
	if err != nil {
		return err
	}
	for _, d := range pending {
		l := log.From(ctx).With("domain_id", d.ID, "hostname", d.Hostname, "tenant", d.Tenant)
		switch d.Status { //nolint:exhaustive // Active/Failed son estados terminales: no se reconcilian.
		case domain.DomainPending:
			ok, err := c.platform.VerifyDomain(ctx, d.Hostname, d.VerifyToken, c.cfg.IngressTarget)
			if err != nil {
				l.Warn("verificar dominio", "error", err)
				continue
			}
			if !ok {
				continue // sigue pendiente hasta que el cliente configure DNS
			}
			if err := c.store.SetDomainStatus(ctx, d.Tenant, d.ID, domain.DomainVerified); err != nil {
				if errors.Is(err, domain.ErrDomainTaken) {
					// Otro proyecto lo verificó antes: esta reclamación se cierra.
					_ = c.store.SetDomainStatus(ctx, d.Tenant, d.ID, domain.DomainFailed)
				}
				l.Warn("marcar dominio verificado", "error", err)
				continue
			}
			l.Info("dominio verificado; solicitando TLS")
		case domain.DomainVerified:
			// Aplica el Ingress + Certificate de cert-manager y comprueba emisión.
			ok, err := c.platform.EnsureTLS(ctx, TLSRequest{
				Hostname: d.Hostname, Issuer: c.cfg.CertIssuer, Tenant: d.Tenant,
			})
			if err != nil {
				l.Warn("emitir TLS", "error", err)
				continue
			}
			if !ok {
				continue // certificado aún emitiéndose
			}
			if err := c.store.SetDomainStatus(ctx, d.Tenant, d.ID, domain.DomainActive); err != nil {
				l.Warn("marcar dominio activo", "error", err)
				continue
			}
			l.Info("dominio activo con TLS")
		}
	}
	return nil
}

// logLike es el subconjunto de slog.Logger que usamos (facilita tests).
type logLike interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}
