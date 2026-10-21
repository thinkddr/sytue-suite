// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/base64"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// recPlatform registra las llamadas del reconciliador para las aserciones.
// Embebe StubPlatform (reutiliza VerifyDomain/EnsureTLS) y añade grabación de
// EnsureNamespace/ApplyApp/RetireApp más un log de orden compartido (opcional)
// para verificar secuencias entre pasos. El reconcile es síncrono: sin locks.
type recPlatform struct {
	StubPlatform
	applyReady   bool
	ensured      []string  // namespaces asegurados
	applied      []string  // DeploymentID aplicados
	appliedImage []string  // imagen aplicada por cada ApplyApp (misma posición que applied)
	retired      []string  // "namespace/DeploymentID" retirados
	order        *[]string // secuencia compartida con el provisioner (opcional)
}

func (p *recPlatform) EnsureNamespace(_ context.Context, namespace, _ string, _ domain.PlanQuota) error {
	p.ensured = append(p.ensured, namespace)
	if p.order != nil {
		*p.order = append(*p.order, "ensure:"+namespace)
	}
	return nil
}

func (p *recPlatform) ApplyApp(_ context.Context, spec AppSpec) (bool, error) {
	p.applied = append(p.applied, spec.DeploymentID)
	p.appliedImage = append(p.appliedImage, spec.Image)
	if p.order != nil {
		*p.order = append(*p.order, "apply:"+spec.DeploymentID)
	}
	return p.applyReady, nil
}

func (p *recPlatform) RetireApp(_ context.Context, namespace, deploymentID string) error {
	p.retired = append(p.retired, namespace+"/"+deploymentID)
	return nil
}

// recProvisioner registra el orden de las llamadas a Provision.
type recProvisioner struct {
	order *[]string
}

func (p *recProvisioner) Provision(_ context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	*p.order = append(*p.order, "provision:"+svc.Tenant)
	return ProvisionResult{Config: map[string]string{"ok": "1"}, Credentials: []byte("{}")}, nil
}

func (p *recProvisioner) Deprovision(context.Context, domain.ManagedService) error { return nil }

func (p *recProvisioner) RestorePostgres(context.Context, domain.ManagedService, string) error {
	return nil
}

func (p *recProvisioner) RunServiceNow(context.Context, domain.ManagedService) error { return nil }

func (p *recProvisioner) BackupPostgresNow(context.Context, domain.ManagedService) error {
	return nil
}

// statusCall captura una escritura de estado de deployment.
type statusCall struct {
	id     uuid.UUID
	status domain.DeployStatus
	errMsg string
}

// fakeCtrlStore es un storeAPI en memoria. Embebe la interfaz: los métodos NO
// implementados panican si el reconciliador los toca (falla ruidosamente).
type fakeCtrlStore struct {
	storeAPI

	pendingServices []domain.ManagedService
	pendingDeploys  []store.PendingDeployment
	retirable       []store.RetirableDeployment
	readyProds      []store.PendingDeployment
	computeWindows  []store.ComputeUsage
	cohortUsage     []domain.ProjectUsage
	build           domain.Build
	// panicOnListServices simula un pánico dentro de un paso (no de un elemento).
	panicOnListServices bool

	// Lo que el store devuelve al resolver la inyección de un despliegue.
	envVars  []store.ResolvedEnvVar
	creds    map[domain.DeployTarget]map[string][]byte
	withheld store.PreviewWithheld
	// credsTargets son los destinos con los que se pidieron credenciales.
	credsTargets []domain.DeployTarget

	markedReady      []uuid.UUID
	statusSet        []statusCall
	tornDown         []uuid.UUID
	usage            []store.UsageInput
	publishedCohorts []domain.Cohort
}

func (f *fakeCtrlStore) ClaimComputeUsage(context.Context, time.Time, time.Duration, time.Duration, int) ([]store.ComputeUsage, error) {
	return f.computeWindows, nil
}

func (f *fakeCtrlStore) RecordUsage(_ context.Context, in store.UsageInput) error {
	f.usage = append(f.usage, in)
	return nil
}

// Benchmarks: el ciclo completo los recalcula, así que hasta las pruebas que no
// van de eso tienen que poder atravesarlos.
func (f *fakeCtrlStore) CollectCohortUsage(context.Context, time.Time) ([]domain.ProjectUsage, error) {
	return f.cohortUsage, nil
}

func (f *fakeCtrlStore) PublishCohorts(_ context.Context, c []domain.Cohort) error {
	f.publishedCohorts = append(f.publishedCohorts, c...)
	return nil
}

func (f *fakeCtrlStore) ListPendingServices(context.Context, int) ([]domain.ManagedService, error) {
	if f.panicOnListServices {
		panic("store: fila corrupta")
	}
	return f.pendingServices, nil
}

func (f *fakeCtrlStore) ListPendingDeployments(context.Context, int) ([]store.PendingDeployment, error) {
	return f.pendingDeploys, nil
}

func (f *fakeCtrlStore) ListPendingDomains(context.Context, int) ([]domain.Domain, error) {
	return nil, nil
}

func (f *fakeCtrlStore) MarkServiceReady(_ context.Context, _ string, id uuid.UUID, _ map[string]string, _ []byte) error {
	f.markedReady = append(f.markedReady, id)
	return nil
}

func (f *fakeCtrlStore) ListRetirableDeployments(context.Context, int) ([]store.RetirableDeployment, error) {
	return f.retirable, nil
}

func (f *fakeCtrlStore) MarkDeploymentTornDown(_ context.Context, _ string, id uuid.UUID) error {
	f.tornDown = append(f.tornDown, id)
	return nil
}

func (f *fakeCtrlStore) ListReadyProductions(context.Context, time.Time, int) ([]store.PendingDeployment, error) {
	return f.readyProds, nil
}

func (f *fakeCtrlStore) GetBuild(context.Context, uuid.UUID) (domain.Build, error) {
	return f.build, nil
}

func (f *fakeCtrlStore) SetDeploymentStatus(_ context.Context, _ string, id uuid.UUID, status domain.DeployStatus, _, errMsg string) error {
	f.statusSet = append(f.statusSet, statusCall{id: id, status: status, errMsg: errMsg})
	return nil
}

func (f *fakeCtrlStore) ResolveEnvForDeploy(context.Context, string, uuid.UUID, domain.DeployTarget) ([]store.ResolvedEnvVar, error) {
	return f.envVars, nil
}

func (f *fakeCtrlStore) ResolveServiceCreds(_ context.Context, _ string, _ uuid.UUID, target domain.DeployTarget) (map[string][]byte, error) {
	f.credsTargets = append(f.credsTargets, target)
	if c, ok := f.creds[target]; ok {
		return c, nil
	}
	return map[string][]byte{}, nil
}

func (f *fakeCtrlStore) PreviewWithheld(context.Context, string, uuid.UUID) (store.PreviewWithheld, error) {
	return f.withheld, nil
}

func (f *fakeCtrlStore) GetOrganizationPlanByTenant(context.Context, string) (domain.Plan, error) {
	return domain.PlanFree, nil
}

func (f *fakeCtrlStore) ListDomains(context.Context, uuid.UUID) ([]domain.Domain, error) {
	return nil, nil
}

func newTestController(fs storeAPI, plat Platform, prov ServiceProvisioner) *Controller {
	sealer, err := crypto.NewSealer(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		panic(err)
	}
	return &Controller{
		cfg: testCfg(), store: fs, sealer: sealer,
		platform: plat, provider: prov, wake: make(chan struct{}, 1),
	}
}

func hasStatus(calls []statusCall, id uuid.UUID, st domain.DeployStatus) bool {
	for _, c := range calls {
		if c.id == id && c.status == st {
			return true
		}
	}
	return false
}

// Gap 1: reconcileTeardown desmonta los objetos de los deployments terminados
// (canceled por cierre de PR / borrado de proyecto; retired por rollback).
func TestReconcileTeardownRetiresTerminated(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	fs := &fakeCtrlStore{retirable: []store.RetirableDeployment{
		{ID: id1, Tenant: "acme", Target: domain.TargetPreview},
		{ID: id2, Tenant: "beta", Target: domain.TargetProduction},
	}}
	plat := &recPlatform{}
	c := newTestController(fs, plat, nil)

	if err := c.reconcileTeardown(context.Background()); err != nil {
		t.Fatalf("reconcileTeardown: %v", err)
	}
	want := []string{"tenant-acme/" + id1.String(), "tenant-beta/" + id2.String()}
	if !slices.Equal(plat.retired, want) {
		t.Errorf("RetireApp por (namespace,id): got %v want %v", plat.retired, want)
	}
	if len(plat.ensured) != 0 || len(plat.applied) != 0 {
		t.Errorf("teardown no debe asegurar namespaces ni aplicar apps")
	}
	// Marca-una-vez: cada deployment desmontado se fija torn_down_at para salir
	// del conjunto y no reintentarse (desacopla el drenaje del LIMIT/ventana).
	if !slices.Equal(fs.tornDown, []uuid.UUID{id1, id2}) {
		t.Errorf("torn_down tras RetireApp: got %v want %v", fs.tornDown, []uuid.UUID{id1, id2})
	}
}

// Gap 2: tras un rollback, el estado deseado es la producción ready más reciente:
// reconcileProduction reaplica la deseada y retira (objetos + estado) las otras.
func TestReconcileProductionRollbackSwitch(t *testing.T) {
	proj := domain.Project{ID: uuid.New(), Tenant: "acme", Slug: "web", ProductionRef: "main"}
	bid := uuid.New()
	build := domain.Build{ID: bid, Status: domain.BuildSucceeded, ImageRef: "rg/web@sha256:abc"}
	mkDep := func() domain.Deployment {
		return domain.Deployment{
			ID: uuid.New(), Tenant: "acme", ProjectID: proj.ID, BuildID: &bid,
			Target: domain.TargetProduction, Status: domain.DeployReady, URL: "https://web.apps.sytue.eu",
		}
	}
	desired, older := mkDep(), mkDep()
	fs := &fakeCtrlStore{
		build: build,
		// Orden (project_id, updated_at DESC): la primera es la deseada.
		readyProds: []store.PendingDeployment{
			{Deployment: desired, Project: proj},
			{Deployment: older, Project: proj},
		},
	}
	plat := &recPlatform{applyReady: true}
	c := newTestController(fs, plat, nil)

	if err := c.reconcileProduction(context.Background()); err != nil {
		t.Fatalf("reconcileProduction: %v", err)
	}
	if !slices.Equal(plat.applied, []string{desired.ID.String()}) {
		t.Errorf("solo la producción deseada debe (re)aplicarse; applied=%v", plat.applied)
	}
	wantRetire := []string{"tenant-acme/" + older.ID.String()}
	if !slices.Equal(plat.retired, wantRetire) {
		t.Errorf("la producción anterior debe retirarse; retired=%v want %v", plat.retired, wantRetire)
	}
	if !hasStatus(fs.statusSet, older.ID, domain.DeployRetired) {
		t.Errorf("la producción anterior debe marcarse retired; statusSet=%v", fs.statusSet)
	}
	if hasStatus(fs.statusSet, desired.ID, domain.DeployRetired) {
		t.Errorf("la producción deseada NO debe retirarse")
	}
}

// BYO-image (D5): un deployment con ImageRef (imagen ya construida) se despliega
// SIN build —reusa ApplyApp con esa imagen y llega a ready— y no consulta ninguna
// build (BuildID nil). imageFor devuelve la imagen externa directamente.
func TestReconcileDeploymentBYOImageSkipsBuild(t *testing.T) {
	proj := domain.Project{ID: uuid.New(), Tenant: "acme", Slug: "web", ProductionRef: "main"}
	dep := domain.Deployment{
		ID: uuid.New(), Tenant: "acme", ProjectID: proj.ID,
		Target: domain.TargetPreview, Status: domain.DeployQueued,
		URL:      "https://web-pr-7.preview.sytue.eu",
		ImageRef: "rg.fr-par.scw.cloud/sytue/web@sha256:beef", // BYO, sin BuildID
		PRNumber: intptr(7),
	}
	fs := &fakeCtrlStore{}
	plat := &recPlatform{applyReady: true}
	c := newTestController(fs, plat, nil)

	c.reconcileDeployment(context.Background(), dep, proj)

	if !slices.Equal(plat.applied, []string{dep.ID.String()}) {
		t.Fatalf("debe aplicarse la app BYO; applied=%v", plat.applied)
	}
	if !slices.Equal(plat.appliedImage, []string{dep.ImageRef}) {
		t.Errorf("ApplyApp debe usar la imagen BYO; got %v want %v", plat.appliedImage, []string{dep.ImageRef})
	}
	if !hasStatus(fs.statusSet, dep.ID, domain.DeployReady) {
		t.Errorf("el deployment BYO debe llegar a ready; statusSet=%v", fs.statusSet)
	}
	if hasStatus(fs.statusSet, dep.ID, domain.DeployError) {
		t.Errorf("BYO no debe marcar error (imagen ya provista, sin build)")
	}
}

func intptr(n int) *int { return &n }

// Gap 3: reconcileServices asegura el namespace del tenant ANTES de aprovisionar
// (si no, Secret/PVC nacerían sin ResourceQuota/LimitRange).
func TestReconcileServicesEnsuresNamespaceBeforeProvision(t *testing.T) {
	svcID := uuid.New()
	fs := &fakeCtrlStore{pendingServices: []domain.ManagedService{
		{ID: svcID, Tenant: "acme", Kind: domain.ServicePostgres, Status: domain.ServiceProvisioning},
	}}
	var order []string
	plat := &recPlatform{order: &order}
	prov := &recProvisioner{order: &order}
	c := newTestController(fs, plat, prov)

	if err := c.reconcileServices(context.Background()); err != nil {
		t.Fatalf("reconcileServices: %v", err)
	}
	want := []string{"ensure:tenant-acme", "provision:acme"}
	if !slices.Equal(order, want) {
		t.Errorf("EnsureNamespace debe preceder a Provision; order=%v want %v", order, want)
	}
	if !slices.Contains(fs.markedReady, svcID) {
		t.Errorf("el servicio aprovisionado debe marcarse ready; markedReady=%v", fs.markedReady)
	}
}

// panicProvisioner reproduce el pánico real: resource.MustParse revienta ante un
// pvc_size que no es una cantidad de Kubernetes.
type panicProvisioner struct{ provisioned []string }

func (p *panicProvisioner) Provision(_ context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	if svc.Config["pvc_size"] == "grande" {
		panic(`quantities must match the regular expression '^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$'`)
	}
	p.provisioned = append(p.provisioned, svc.Tenant)
	return ProvisionResult{Config: map[string]string{"ok": "1"}}, nil
}

func (p *panicProvisioner) Deprovision(context.Context, domain.ManagedService) error { return nil }

func (p *panicProvisioner) RestorePostgres(context.Context, domain.ManagedService, string) error {
	return nil
}

func (p *panicProvisioner) RunServiceNow(context.Context, domain.ManagedService) error { return nil }

func (p *panicProvisioner) BackupPostgresNow(context.Context, domain.ManagedService) error {
	return nil
}

// capLogger captura los avisos del reconciliador (implementa logLike).
type capLogger struct{ warns []string }

func (l *capLogger) Info(string, ...any)       {}
func (l *capLogger) Warn(msg string, _ ...any) { l.warns = append(l.warns, msg) }
func (l *capLogger) warned(msg string) bool    { return slices.Contains(l.warns, msg) }

// D5 — un solo inquilino podía tumbar el controlador de TODOS: no había un solo
// recover() en el servicio, así que un `pvc_size: "grande"` hacía pánico en
// resource.MustParse y dejaba cloud-controller en CrashLoop permanente. El bucle
// tiene que sobrevivir al pánico, seguir con el resto de la cola y con los pasos
// siguientes.
func TestReconcileSobreviveAlPanicoDeUnInquilino(t *testing.T) {
	proj := domain.Project{ID: uuid.New(), Tenant: "acme", Slug: "web"}
	dep := domain.Deployment{
		ID: uuid.New(), Tenant: "acme", ProjectID: proj.ID, Target: domain.TargetPreview,
		Status: domain.DeployQueued, URL: "https://web-pr-1.preview.sytue.eu",
		ImageRef: "rg/web@sha256:abc",
	}

	t.Run("un servicio envenenado no para la cola ni el ciclo", func(t *testing.T) {
		sano := uuid.New()
		fs := &fakeCtrlStore{
			pendingServices: []domain.ManagedService{
				{
					ID: uuid.New(), Tenant: "hostil", Kind: domain.ServicePostgres,
					Status: domain.ServiceProvisioning,
					Config: map[string]string{"pvc_size": "grande"},
				},
				{ID: sano, Tenant: "acme", Kind: domain.ServicePostgres, Status: domain.ServiceProvisioning},
			},
			pendingDeploys: []store.PendingDeployment{{Deployment: dep, Project: proj}},
		}
		plat := &recPlatform{applyReady: true}
		prov := &panicProvisioner{}
		c := newTestController(fs, plat, prov)
		l := &capLogger{}

		c.reconcile(context.Background(), l) // no debe propagarse el pánico

		if !slices.Contains(prov.provisioned, "acme") {
			t.Errorf("el servicio del inquilino sano debe aprovisionarse igualmente; provisioned=%v", prov.provisioned)
		}
		if !slices.Contains(fs.markedReady, sano) {
			t.Errorf("el servicio sano debe marcarse ready; markedReady=%v", fs.markedReady)
		}
		// El servicio envenenado no avanza (el aviso, con service_id y tenant, lo
		// emite el logger contextual de reconcileServices, no el del ciclo).
		if len(fs.markedReady) != 1 {
			t.Errorf("el servicio envenenado no puede marcarse ready; markedReady=%v", fs.markedReady)
		}
		if !slices.Equal(plat.applied, []string{dep.ID.String()}) {
			t.Errorf("los pasos siguientes deben ejecutarse igual; applied=%v", plat.applied)
		}
	})

	t.Run("un paso que revienta no se lleva los siguientes", func(t *testing.T) {
		fs := &fakeCtrlStore{
			panicOnListServices: true,
			pendingDeploys:      []store.PendingDeployment{{Deployment: dep, Project: proj}},
		}
		plat := &recPlatform{applyReady: true}
		c := newTestController(fs, plat, &panicProvisioner{})
		l := &capLogger{}

		c.reconcile(context.Background(), l)

		if !l.warned("reconciliar servicios") {
			t.Errorf("el paso que revienta debe registrarse; warns=%v", l.warns)
		}
		if !slices.Equal(plat.applied, []string{dep.ID.String()}) {
			t.Errorf("el paso de deployments debe correr pese al pánico del anterior; applied=%v", plat.applied)
		}
	})
}

// PORT es la convención con la que la plataforma le dice a la app dónde escuchar
// (12-factor: Heroku, DigitalOcean, Coolify hacen lo mismo). Estaba fijado a 8080
// mientras el resto del spec ya usaba el puerto configurado del proyecto, así que
// una app que respetase $PORT —la mayoría de las de Node— habría escuchado en un
// puerto y habría sido sondeada en otro: reinicio en bucle sin motivo aparente.
func TestPortInyectadoSigueAlProyecto(t *testing.T) {
	c := newTestController(&fakeCtrlStore{}, &recPlatform{}, nil)
	dep := domain.Deployment{ID: uuid.New(), Tenant: "acme", Target: domain.TargetProduction}

	t.Run("puerto configurado", func(t *testing.T) {
		env, _, err := c.resolveInjection(context.Background(), dep,
			domain.Project{Slug: "web", AppPort: 3000})
		if err != nil {
			t.Fatalf("resolveInjection: %v", err)
		}
		if env["PORT"] != "3000" {
			t.Errorf("PORT=%q; la app escucharía donde no se la sondea", env["PORT"])
		}
	})

	t.Run("sin configurar usa el defecto", func(t *testing.T) {
		env, _, err := c.resolveInjection(context.Background(), dep, domain.Project{Slug: "web"})
		if err != nil {
			t.Fatalf("resolveInjection: %v", err)
		}
		if env["PORT"] != strconv.Itoa(domain.DefaultAppPort) {
			t.Errorf("PORT=%q, esperado el defecto %d", env["PORT"], domain.DefaultAppPort)
		}
	})
}
