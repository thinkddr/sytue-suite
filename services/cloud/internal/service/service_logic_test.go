// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// testKeyB64 es una clave AES-256 (32 bytes) para el Sealer en tests.
const testKeyB64 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func newTestService(fs *fakeStore, pub Publisher) *Service {
	sealer, err := crypto.NewSealer(testKeyB64)
	if err != nil {
		panic(err)
	}
	return &Service{
		cfg: config.Config{
			AppsDomain:        "apps.sytue.eu",
			PreviewDomain:     "preview.sytue.eu",
			IngressTarget:     "ingress.sytue.eu",
			S3Region:          "fr-par",
			RegistryURL:       "rg.fr-par.scw.cloud/sytue",
			MaxProjectsPerOrg: 10,
		},
		store:  fs,
		sealer: sealer,
		pub:    pub,
	}
}

func tenantCtx(t *testing.T) context.Context {
	t.Helper()
	tn, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	return tenancy.Into(context.Background(), tn)
}

func TestEnsureOrgDefaultsName(t *testing.T) {
	fs := &fakeStore{org: domain.Organization{Name: "acme"}}
	s := newTestService(fs, nil)
	if _, err := s.EnsureOrg(tenantCtx(t), ""); err != nil {
		t.Fatalf("EnsureOrg: %v", err)
	}
}

func TestSetPlanByOperatorInvalidPlan(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	bad := domain.Plan("enterprise")
	if _, err := s.SetPlanByOperator(tenantCtx(t), bad); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("plan inválido debe rechazarse, got %v", err)
	}
}

func TestUpdateOrgValid(t *testing.T) {
	fs := &fakeStore{updOrg: domain.Organization{Name: "x"}}
	s := newTestService(fs, nil)
	if _, err := s.UpdateOrg(tenantCtx(t), nil, nil); err != nil {
		t.Fatalf("UpdateOrg válido: %v", err)
	}
	if _, err := s.SetPlanByOperator(tenantCtx(t), domain.PlanFree); err != nil {
		t.Fatalf("SetPlanByOperator válido: %v", err)
	}
}

func TestCreateProjectValidation(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	ctx := tenantCtx(t)
	if _, _, err := s.CreateProject(ctx, CreateProjectInput{Slug: "Invalid Slug!"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("slug inválido: %v", err)
	}
	if _, _, err := s.CreateProject(ctx, CreateProjectInput{Slug: "ok", Framework: domain.Framework("weird")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("framework inválido: %v", err)
	}
}

func TestCreateProjectDefaultsAndDelegates(t *testing.T) {
	fs := &fakeStore{org: domain.Organization{Plan: domain.PlanPro}, project: domain.Project{Slug: "web"}}
	s := newTestService(fs, nil)
	_, secret, err := s.CreateProject(tenantCtx(t), CreateProjectInput{Slug: "web", RepoURL: "  https://example.com/y.git "})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if secret == "" {
		t.Fatal("CreateProject debe devolver el secreto de webhook en claro una vez")
	}
	if len(fs.newProj.WebhookSecretEnc) == 0 {
		t.Fatal("CreateProject debe persistir el secreto de webhook cifrado")
	}
	if fs.newProj == nil {
		t.Fatal("no se llamó a CreateProject en el store")
	}
	if fs.newProj.Name != "web" {
		t.Errorf("Name por defecto = slug, got %q", fs.newProj.Name)
	}
	if fs.newProj.GitProvider != domain.GitGeneric {
		t.Errorf("GitProvider por defecto = generic, got %q", fs.newProj.GitProvider)
	}
	if fs.newProj.ProductionRef != "main" {
		t.Errorf("ProductionRef por defecto = main, got %q", fs.newProj.ProductionRef)
	}
	if fs.newProj.Framework != domain.FrameworkDetect {
		t.Errorf("Framework por defecto = auto, got %q", fs.newProj.Framework)
	}
	if fs.newProj.RepoURL != "https://example.com/y.git" {
		t.Errorf("RepoURL debe trimearse, got %q", fs.newProj.RepoURL)
	}
	if fs.maxPerOrg != 10 {
		t.Errorf("maxPerOrg propagado, got %d", fs.maxPerOrg)
	}
}

func TestCreateProjectOrgError(t *testing.T) {
	fs := &fakeStore{orgErr: errBoom}
	s := newTestService(fs, nil)
	if _, _, err := s.CreateProject(tenantCtx(t), CreateProjectInput{Slug: "web", RepoURL: "https://example.com/y.git"}); !errors.Is(err, errBoom) {
		t.Fatalf("error de EnsureOrg debe propagarse, got %v", err)
	}
}

func TestUpdateProjectInvalidFramework(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	bad := domain.Framework("nope")
	if _, err := s.UpdateProject(tenantCtx(t), uuid.New(), store.UpdateProjectRecord{Framework: &bad}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("framework inválido: %v", err)
	}
}

func TestDeleteProjectEmitsEvent(t *testing.T) {
	fs := &fakeStore{}
	pub := &capturePub{}
	s := newTestService(fs, pub)
	if err := s.DeleteProject(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != EventDeployRetired {
		t.Fatalf("esperaba evento retired, got %v", pub.types)
	}
	// Error del store no emite evento.
	fs.delProjErr = errBoom
	pub.types = nil
	if err := s.DeleteProject(tenantCtx(t), uuid.New()); !errors.Is(err, errBoom) {
		t.Fatalf("error debe propagarse, got %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatalf("no debe emitir evento tras error, got %v", pub.types)
	}
}

func TestDeployValidation(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	ctx := tenantCtx(t)
	if _, _, err := s.Deploy(ctx, DeployInput{Target: domain.DeployTarget("weird")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("target inválido: %v", err)
	}
	if _, _, err := s.Deploy(ctx, DeployInput{Target: domain.TargetPreview}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("preview sin PR: %v", err)
	}
}

func TestDeployHappyPath(t *testing.T) {
	fs := &fakeStore{
		// Org con un miembro ⇒ un puesto de suite: sin ninguno, Free no despliega.
		memberCount: 1,
		project:     domain.Project{Slug: "web", ProductionRef: "main"},
		build:       domain.Build{ID: uuid.New()},
		dep:         domain.Deployment{ID: uuid.New()},
	}
	pub := &capturePub{}
	s := newTestService(fs, pub)
	dep, build, err := s.Deploy(tenantCtx(t), DeployInput{
		ProjectID: uuid.New(),
		Target:    domain.TargetProduction,
		CommitSHA: "abc1234",
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if dep.ID == uuid.Nil || build.ID == uuid.Nil {
		t.Fatal("deployment/build vacíos")
	}
	// GitRef vacío toma la rama de producción.
	if fs.newBuild.GitRef != "main" {
		t.Errorf("GitRef por defecto = ProductionRef, got %q", fs.newBuild.GitRef)
	}
	// La URL se derivó a producción.
	if fs.newDep.URL != "https://web.apps.sytue.eu" {
		t.Errorf("URL derivada, got %q", fs.newDep.URL)
	}
	if len(pub.types) != 1 || pub.types[0] != EventBuildEnqueued {
		t.Fatalf("esperaba build.enqueued, got %v", pub.types)
	}
}

func TestDeployProjectNotFound(t *testing.T) {
	fs := &fakeStore{projectErr: errBoom}
	s := newTestService(fs, nil)
	if _, _, err := s.Deploy(tenantCtx(t), DeployInput{Target: domain.TargetProduction}); !errors.Is(err, errBoom) {
		t.Fatalf("error de GetProject debe propagarse, got %v", err)
	}
}

// Un .env que no se puede volver a leer no sirve de nada: el valor con un espacio
// se corta, el que lleva almohadilla se convierte en comentario y el que trae un
// salto de línea inventa claves nuevas con el resto. Por eso el entrecomillado no
// es cosmético.
func TestDotenvQuote(t *testing.T) {
	t.Parallel()
	// Sin nada raro dentro, va tal cual: entrecomillar todo haría ilegible un
	// fichero que la gente abre para leerlo.
	for _, v := range []string{"abc", "123", "clave-con-guiones", "a=b", "/ruta/x"} {
		if got := dotenvQuote(v); got != v {
			t.Errorf("dotenvQuote(%q) = %q; no hacía falta tocarlo", v, got)
		}
	}
	casos := map[string]string{
		"":                `""`,
		"con espacio":     `"con espacio"`,
		"con#almohadilla": `"con#almohadilla"`,
		"con\"comilla":    `"con\"comilla"`,
		"con\nsalto":      `"con\nsalto"`, // el salto REAL sale como los dos caracteres \n
		`con\barra`:       `"con\\barra"`, // y la barra, duplicada
		"$(rm -rf /)":     `"$(rm -rf /)"`,
	}
	for in, want := range casos {
		if got := dotenvQuote(in); got != want {
			t.Errorf("dotenvQuote(%q) = %q, quiero %q", in, got, want)
		}
	}
}

// Clonar tiene que pasar por el alta normal: es lo que aplica la cuota de la
// organización y valida el slug. Un camino propio se saltaría las dos cosas y
// nadie lo notaría hasta tener el proyecto de más.
func TestCloneProjectPasaPorElAltaNormal(t *testing.T) {
	src := domain.Project{
		ID: uuid.New(), Slug: "web", Name: "Web", RepoURL: "https://git.sytue.eu/acme/web.git",
		ProductionRef: "main", BuildCommand: "bun run build", InstanceCount: 3,
	}
	// Con plan Pro, igual que el test del alta normal: la cuota sale de
	// `QuotasFor(org.Plan)`, así que sin plan la organización cae en el gratuito y
	// el 10 de abajo sería 3. Y ESE es el valor que delata el fallo que este test
	// vigila: si el clon dejara de pasar por el alta, no llegaría ninguna cuota.
	fs := &fakeStore{project: src, org: domain.Organization{Plan: domain.PlanPro}}
	s := newTestService(fs, nil)

	if _, _, _, err := s.CloneProject(tenantCtx(t), src.ID, "web-staging", ""); err != nil {
		t.Fatalf("CloneProject: %v", err)
	}
	if fs.newProj == nil {
		t.Fatal("el clon no pasó por CreateProject")
	}
	if fs.newProj.Slug != "web-staging" {
		t.Errorf("Slug = %q, quiero el destino", fs.newProj.Slug)
	}
	// La configuración que cuesta repetir a mano es justo la que tiene que viajar.
	if fs.newProj.RepoURL != src.RepoURL || fs.newProj.BuildCommand != src.BuildCommand || fs.newProj.InstanceCount != src.InstanceCount {
		t.Errorf("la configuración no se heredó: %+v", *fs.newProj)
	}
	// Sin nombre propio, hereda el del origen (y no queda vacío).
	if fs.newProj.Name != src.Name {
		t.Errorf("Name = %q, quiero heredar %q", fs.newProj.Name, src.Name)
	}
	// El secreto de webhook es PROPIO: compartirlo ataría los dos proyectos a la
	// misma credencial.
	if len(fs.newProj.WebhookSecretEnc) == 0 {
		t.Error("el clon no generó su propio secreto de webhook")
	}
	// Y la cuota se aplica igual que en un alta normal: la del PLAN de la
	// organización, que es lo que impide que duplicar sea la puerta trasera para
	// saltarse el límite de proyectos.
	if fs.maxPerOrg != domain.QuotasFor(domain.PlanPro).MaxProjects {
		t.Errorf("maxPerOrg = %d; el clon debe contar para la cuota del plan", fs.maxPerOrg)
	}
	// Las variables se leen DEL ORIGEN. Invertir los argumentos machacaría la
	// configuración del proyecto bueno con la del clon, que es el peor final
	// posible para un botón que promete no tocar nada.
	if fs.clonedFrom != src.ID {
		t.Errorf("origen de las variables = %s, quiero el proyecto de partida %s", fs.clonedFrom, src.ID)
	}
	if fs.clonedTo == uuid.Nil {
		t.Error("no se copiaron las variables al proyecto nuevo")
	}
}

// Un slug inválido no llega a crear nada: lo corta la misma validación del alta.
func TestCloneProjectRechazaSlugInvalido(t *testing.T) {
	src := domain.Project{ID: uuid.New(), Slug: "web", RepoURL: "https://git.sytue.eu/acme/web.git"}
	fs := &fakeStore{project: src}
	s := newTestService(fs, nil)
	if _, _, _, err := s.CloneProject(tenantCtx(t), src.ID, "Web Staging!", ""); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("slug inválido debe dar ErrInvalidInput, got %v", err)
	}
	if fs.newProj != nil {
		t.Error("se creó un proyecto con un slug inválido")
	}
}

// Promover una preview NO puede recompilar: si el deployment nuevo saliera sin
// imagen, el controlador lanzaría una build y produciría un artefacto distinto
// del que se revisó, que es justo lo que este verbo existe para evitar.
func TestPromotePreviewReusaLaImagen(t *testing.T) {
	proj := domain.Project{ID: uuid.New(), Slug: "web"}
	prev := domain.Deployment{
		ID: uuid.New(), ProjectID: proj.ID, Target: domain.TargetPreview,
		Status: domain.DeployReady, ImageRef: "reg.sytue.eu/acme/web:abc123",
		CommitSHA: "abc123", GitRef: "refs/heads/rama",
	}
	fs := &fakeStore{project: proj, dep: prev}
	pub := &capturePub{}
	s := newTestService(fs, pub)

	if _, err := s.PromotePreview(tenantCtx(t), prev.ID); err != nil {
		t.Fatalf("PromotePreview: %v", err)
	}
	if fs.newDep == nil {
		t.Fatal("no se creó el deployment de producción")
	}
	if fs.newDep.ImageRef != prev.ImageRef {
		t.Errorf("ImageRef = %q, quiero la de la preview (%q): sin ella se recompila", fs.newDep.ImageRef, prev.ImageRef)
	}
	if fs.newDep.Target != domain.TargetProduction {
		t.Errorf("Target = %q, quiero production", fs.newDep.Target)
	}
	// Nada de builds: promover no cuesta minutos de compilación.
	if fs.newBuild != nil {
		t.Error("promover encoló una build; debía reusar la imagen ya construida")
	}
	if len(pub.types) != 1 || pub.types[0] != EventDeployRequested {
		t.Fatalf("esperaba deploy.requested, got %v", pub.types)
	}
}

// El rollback y esto son verbos distintos, y confundirlos es lo que tenía el
// menú del panel prometiendo sobre previews algo que devolvía 409 siempre.
func TestPromotePreviewRechazaProduccionYNoLista(t *testing.T) {
	proj := domain.Project{ID: uuid.New(), Slug: "web"}
	prod := domain.Deployment{
		ID: uuid.New(), ProjectID: proj.ID, Target: domain.TargetProduction,
		Status: domain.DeployReady, ImageRef: "reg.sytue.eu/acme/web:v1",
	}
	s := newTestService(&fakeStore{project: proj, dep: prod}, nil)
	if _, err := s.PromotePreview(tenantCtx(t), prod.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("promover una producción debe dar ErrInvalidState, got %v", err)
	}

	building := domain.Deployment{
		ID: uuid.New(), ProjectID: proj.ID, Target: domain.TargetPreview,
		Status: domain.DeployBuilding, ImageRef: "reg.sytue.eu/acme/web:v2",
	}
	s2 := newTestService(&fakeStore{project: proj, dep: building}, nil)
	if _, err := s2.PromotePreview(tenantCtx(t), building.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Errorf("promover una preview a medio construir debe dar ErrInvalidState, got %v", err)
	}
}

// Un proyecto pausado no tiene instancias: reiniciarlo no puede pasar por hecho.
// El corte va ANTES de tocar el clúster, así que este caso ni siquiera necesita
// runtime — y por eso el test lo alcanza sin uno.
func TestRestartAppRechazaProyectoPausado(t *testing.T) {
	fs := &fakeStore{project: domain.Project{ID: uuid.New(), Slug: "web", Paused: true}}
	s := newTestService(fs, nil)
	if _, err := s.RestartApp(tenantCtx(t), uuid.New()); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("reiniciar un proyecto pausado debe dar ErrInvalidState, got %v", err)
	}
}

func TestRollbackEmits(t *testing.T) {
	fs := &fakeStore{promote: domain.Deployment{ID: uuid.New()}}
	pub := &capturePub{}
	s := newTestService(fs, pub)
	if _, err := s.Rollback(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != EventDeployRequested {
		t.Fatalf("esperaba deploy.requested, got %v", pub.types)
	}
	fs.promErr = errBoom
	if _, err := s.Rollback(tenantCtx(t), uuid.New()); !errors.Is(err, errBoom) {
		t.Fatalf("error debe propagarse, got %v", err)
	}
}

func TestAddDomainValidation(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	if _, err := s.AddDomain(tenantCtx(t), uuid.New(), "not a host"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("hostname inválido: %v", err)
	}
}

func TestAddDomainGeneratesToken(t *testing.T) {
	fs := &fakeStore{dom: domain.Domain{ID: uuid.New(), Hostname: "app.example.com"}}
	pub := &capturePub{}
	s := newTestService(fs, pub)
	if _, err := s.AddDomain(tenantCtx(t), uuid.New(), "app.example.com"); err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	if fs.domHost != "app.example.com" {
		t.Errorf("hostname pasado al store, got %q", fs.domHost)
	}
	if len(fs.domToken) < len("sytue-verify=") || fs.domToken[:13] != "sytue-verify=" {
		t.Errorf("token de verificación mal formado, got %q", fs.domToken)
	}
	if len(pub.types) != 1 || pub.types[0] != EventDomainAdded {
		t.Fatalf("esperaba domain.added, got %v", pub.types)
	}
}

func TestSetEnvValidation(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	ctx := tenantCtx(t)
	if _, err := s.SetEnv(ctx, SetEnvInput{Key: "lower-case"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("clave inválida: %v", err)
	}
	if _, err := s.SetEnv(ctx, SetEnvInput{Key: "OK", Scope: domain.EnvScope("weird")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("scope inválido: %v", err)
	}
}

func TestSetEnvPlainAndSecret(t *testing.T) {
	fs := &fakeStore{env: domain.EnvVar{Key: "API"}}
	s := newTestService(fs, nil)
	ctx := tenantCtx(t)

	// Plano: value en claro, sin cifrar.
	if _, err := s.SetEnv(ctx, SetEnvInput{Key: "API_URL", Value: "https://x"}); err != nil {
		t.Fatalf("SetEnv plano: %v", err)
	}
	if fs.envRec.ValuePlain != "https://x" || len(fs.envRec.ValueEnc) != 0 {
		t.Errorf("valor plano mal almacenado: plain=%q enc=%v", fs.envRec.ValuePlain, fs.envRec.ValueEnc)
	}
	if fs.envRec.Scope != domain.ScopeAll {
		t.Errorf("scope por defecto = all, got %q", fs.envRec.Scope)
	}

	// Secreto: value cifrado (no en claro).
	if _, err := s.SetEnv(ctx, SetEnvInput{Key: "TOKEN", Value: "s3cr3t", IsSecret: true}); err != nil {
		t.Fatalf("SetEnv secreto: %v", err)
	}
	if len(fs.envRec.ValueEnc) == 0 || fs.envRec.ValuePlain != "" {
		t.Errorf("secreto debe ir cifrado, got enc=%v plain=%q", fs.envRec.ValueEnc, fs.envRec.ValuePlain)
	}
	// El texto plano no debe aparecer en el blob cifrado.
	if string(fs.envRec.ValueEnc) == "s3cr3t" {
		t.Error("el secreto no está cifrado")
	}
}

func TestProvisionServiceValidation(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	if _, err := s.ProvisionService(tenantCtx(t), ProvisionServiceInput{Kind: domain.ServiceKind("weird")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("kind inválido: %v", err)
	}
}

// TestProvisionFunctionsImageAnchored verifica que una función (como cron/job)
// exige una imagen anclada al repo del propio proyecto (<registry>/<slug>):
// aislamiento cross-tenant. Imagen ajena ⇒ rechazada; imagen del repo ⇒ aceptada.
func TestProvisionFunctionsImageAnchored(t *testing.T) {
	fs := &fakeStore{
		project: domain.Project{Slug: "web"},
		svc:     domain.ManagedService{ID: uuid.New()},
	}
	s := newTestService(fs, &capturePub{})

	if _, err := s.ProvisionService(tenantCtx(t), ProvisionServiceInput{
		Kind: domain.ServiceFunctions, ProjectID: uuid.New(),
		Config: map[string]string{"image": "rg.fr-par.scw.cloud/sytue/otro:v1"},
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("imagen fuera del repo del proyecto debe rechazarse, got %v", err)
	}
	if _, err := s.ProvisionService(tenantCtx(t), ProvisionServiceInput{
		Kind: domain.ServiceFunctions, ProjectID: uuid.New(),
		Config: map[string]string{"image": "rg.fr-par.scw.cloud/sytue/web:v1"},
	}); err != nil {
		t.Fatalf("imagen del repo del proyecto debe aceptarse: %v", err)
	}
	// Sin imagen ⇒ rechazada.
	if _, err := s.ProvisionService(tenantCtx(t), ProvisionServiceInput{
		Kind: domain.ServiceFunctions, ProjectID: uuid.New(),
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("función sin imagen debe rechazarse, got %v", err)
	}
	// Nombre que no es etiqueta DNS (dnsSafe lo colapsaría / label inválida) ⇒
	// rechazado en el límite, aun con imagen válida.
	if _, err := s.ProvisionService(tenantCtx(t), ProvisionServiceInput{
		Kind: domain.ServiceFunctions, ProjectID: uuid.New(), Name: "My.Fn",
		Config: map[string]string{"image": "rg.fr-par.scw.cloud/sytue/web:v1"},
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("nombre de función no-DNS debe rechazarse, got %v", err)
	}
}

func TestProvisionServiceDefaultsName(t *testing.T) {
	fs := &fakeStore{svc: domain.ManagedService{ID: uuid.New()}}
	pub := &capturePub{}
	s := newTestService(fs, pub)
	kind := domain.ServicePostgres
	if _, err := s.ProvisionService(tenantCtx(t), ProvisionServiceInput{Kind: kind}); err != nil {
		t.Fatalf("ProvisionService: %v", err)
	}
	if fs.newSvc.Name != string(kind) {
		t.Errorf("Name por defecto = kind, got %q", fs.newSvc.Name)
	}
	if len(pub.types) != 1 || pub.types[0] != EventServiceRequested {
		t.Fatalf("esperaba service.requested, got %v", pub.types)
	}
}

func TestDeleteServiceEmits(t *testing.T) {
	fs := &fakeStore{}
	pub := &capturePub{}
	s := newTestService(fs, pub)
	if err := s.DeleteService(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != EventServiceDeleted {
		t.Fatalf("esperaba service.deleted, got %v", pub.types)
	}
	fs.delSvcErr = errBoom
	pub.types = nil
	if err := s.DeleteService(tenantCtx(t), uuid.New()); !errors.Is(err, errBoom) {
		t.Fatalf("error debe propagarse, got %v", err)
	}
}

func TestUsageDefaultsWindow(t *testing.T) {
	fs := &fakeStore{}
	s := newTestService(fs, nil)
	if _, err := s.UsageSummary(tenantCtx(t), time.Time{}, time.Time{}); err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if _, err := s.UsageDetail(tenantCtx(t), time.Time{}, time.Time{}, 0); err != nil {
		t.Fatalf("UsageDetail: %v", err)
	}
}

func TestEmitErrorPropagates(t *testing.T) {
	pub := &capturePub{err: errBoom}
	s := newTestService(&fakeStore{}, pub)
	// DeleteProject devuelve el error de emit.
	if err := s.DeleteProject(tenantCtx(t), uuid.New()); !errors.Is(err, errBoom) {
		t.Fatalf("emit error debe propagarse en DeleteProject, got %v", err)
	}
}

func TestEmitNilPublisher(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	// Sin publisher, emit es un no-op y DeleteProject no falla.
	if err := s.DeleteProject(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("con pub nil no debe fallar: %v", err)
	}
}

func TestIngressTarget(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	if got := s.IngressTarget(); got != "ingress.sytue.eu" {
		t.Fatalf("IngressTarget = %q", got)
	}
}

func TestVerifyWebhookResolveError(t *testing.T) {
	fs := &fakeStore{resolveErr: errBoom}
	s := newTestService(fs, nil)
	if _, err := s.VerifyWebhook(context.Background(), "https://x/y.git", nil, "", "", ""); !errors.Is(err, errBoom) {
		t.Fatalf("error de ResolveRepos debe propagarse, got %v", err)
	}
}

// CLOUD-05: otra organización registra la URL del repositorio de la víctima. Su
// fila sale primero, pero el push de la víctima va firmado con el secreto de la
// víctima: se despliega en la víctima, y solo en ella.
func TestWebhookConLaMismaURLEnDosOrganizaciones(t *testing.T) {
	sealer, err := crypto.NewSealer(testKeyB64)
	if err != nil {
		t.Fatal(err)
	}
	encAtacante, _ := sealer.SealString("secreto-atacante")
	encVictima, _ := sealer.SealString("secreto-victima")
	victima := uuid.New()
	fs := &fakeStore{resolvedAll: []store.ResolvedRepo{
		{Tenant: "mallory", Project: domain.Project{ID: uuid.New()}, WebhookSecretEnc: encAtacante},
		{Tenant: "acme", Project: domain.Project{ID: victima}, WebhookSecretEnc: encVictima},
	}}
	s := newTestService(fs, nil)
	body := []byte(`{"ref":"refs/heads/main"}`)
	rr, err := s.VerifyWebhook(context.Background(), "https://github.com/acme/web.git", body, ghSig256("secreto-victima", body), "", "")
	if err != nil {
		t.Fatalf("el push legítimo de la víctima se rechazó: %v", err)
	}
	if rr.Tenant != "acme" || rr.Project.ID != victima {
		t.Fatalf("se resolvió %s/%s, no la víctima", rr.Tenant, rr.Project.ID)
	}
	if _, err := s.VerifyWebhook(context.Background(), "https://github.com/acme/web.git", body, ghSig256("otro", body), "", ""); !errors.Is(err, domain.ErrSignatureInvalid) {
		t.Fatalf("firma que no es de nadie: %v", err)
	}
}

func TestHandleGitEventDuplicate(t *testing.T) {
	fs := &fakeStore{
		resolved:   store.ResolvedRepo{Tenant: "acme", Project: domain.Project{ID: uuid.New(), ProductionRef: "main"}},
		webhookDup: true,
	}
	s := newTestService(fs, &capturePub{})
	dep, ignored, err := s.HandleGitEvent(context.Background(), fs.resolved, domain.GitEvent{Ref: "refs/heads/main"}, "d1", nil)
	if err != nil {
		t.Fatalf("HandleGitEvent: %v", err)
	}
	if dep != nil || !ignored {
		t.Fatalf("evento duplicado debe ignorarse sin deployment, got dep=%v ignored=%v", dep, ignored)
	}
}

func TestHandleGitEventProductionPush(t *testing.T) {
	pid := uuid.New()
	fs := &fakeStore{
		memberCount: 1, // un miembro ⇒ un puesto de suite
		resolved:    store.ResolvedRepo{Tenant: "acme", Project: domain.Project{ID: pid, Slug: "web", ProductionRef: "main"}},
		project:     domain.Project{ID: pid, Slug: "web", ProductionRef: "main"},
		build:       domain.Build{ID: uuid.New()},
		dep:         domain.Deployment{ID: uuid.New()},
	}
	pub := &capturePub{}
	s := newTestService(fs, pub)
	dep, ignored, err := s.HandleGitEvent(context.Background(), fs.resolved, domain.GitEvent{
		Ref: "refs/heads/main", CommitSHA: "abc1234", Pusher: "alice",
	}, "d1", []byte("{}"))
	if err != nil {
		t.Fatalf("HandleGitEvent: %v", err)
	}
	if dep == nil || ignored {
		t.Fatalf("push a producción debe desplegar, got dep=%v ignored=%v", dep, ignored)
	}
}

func TestHandleGitEventOtherBranchIgnored(t *testing.T) {
	pid := uuid.New()
	fs := &fakeStore{
		resolved: store.ResolvedRepo{Tenant: "acme", Project: domain.Project{ID: pid, ProductionRef: "main"}},
	}
	s := newTestService(fs, &capturePub{})
	dep, ignored, err := s.HandleGitEvent(context.Background(), fs.resolved, domain.GitEvent{Ref: "refs/heads/dev"}, "d1", nil)
	if err != nil {
		t.Fatalf("HandleGitEvent: %v", err)
	}
	if dep != nil || !ignored {
		t.Fatalf("push a otra rama debe ignorarse, got dep=%v ignored=%v", dep, ignored)
	}
}

func TestHandleGitEventPreviewOpen(t *testing.T) {
	pid := uuid.New()
	fs := &fakeStore{
		memberCount: 1, // un miembro ⇒ un puesto de suite
		resolved:    store.ResolvedRepo{Tenant: "acme", Project: domain.Project{ID: pid, Slug: "web", ProductionRef: "main"}},
		project:     domain.Project{ID: pid, Slug: "web", ProductionRef: "main"},
		build:       domain.Build{ID: uuid.New()},
		dep:         domain.Deployment{ID: uuid.New()},
	}
	s := newTestService(fs, &capturePub{})
	dep, ignored, err := s.HandleGitEvent(context.Background(), fs.resolved, domain.GitEvent{
		PRNumber: 5, Action: "opened", Ref: "feature/x", CommitSHA: "c1abc12",
	}, "d1", nil)
	if err != nil {
		t.Fatalf("HandleGitEvent PR: %v", err)
	}
	if dep == nil || ignored {
		t.Fatalf("PR abierto debe desplegar preview, got dep=%v ignored=%v", dep, ignored)
	}
	if fs.newDep.Target != domain.TargetPreview {
		t.Errorf("target debe ser preview, got %q", fs.newDep.Target)
	}
}

func TestHandleGitEventPreviewClosedRetires(t *testing.T) {
	pid := uuid.New()
	pr := 9
	activePreview := domain.Deployment{ID: uuid.New(), Target: domain.TargetPreview, PRNumber: &pr, Status: domain.DeployStatus("ready")}
	fs := &fakeStore{
		resolved: store.ResolvedRepo{Tenant: "acme", Project: domain.Project{ID: pid, ProductionRef: "main"}},
		deps:     []domain.Deployment{activePreview},
	}
	s := newTestService(fs, &capturePub{})
	dep, ignored, err := s.HandleGitEvent(context.Background(), fs.resolved, domain.GitEvent{
		PRNumber: pr, Action: "closed", Ref: "feature/x",
	}, "d1", nil)
	if err != nil {
		t.Fatalf("HandleGitEvent PR closed: %v", err)
	}
	if dep == nil || ignored {
		t.Fatalf("PR cerrado debe retirar preview activa, got dep=%v ignored=%v", dep, ignored)
	}
}

// ---------------------------------------------------------------------------
// Cursor keyset (token opaco)
// ---------------------------------------------------------------------------

func TestPageCursorRoundTrip(t *testing.T) {
	c := store.PageCursor{
		CreatedAt: time.Date(2026, 5, 1, 12, 30, 45, 123456000, time.UTC),
		ID:        uuid.New(),
	}
	tok := encodePageCursor(&c)
	if tok == "" {
		t.Fatal("encodePageCursor devolvió token vacío")
	}
	got, err := decodePageCursor(tok)
	if err != nil {
		t.Fatalf("decodePageCursor: %v", err)
	}
	if !got.CreatedAt.Equal(c.CreatedAt) || got.ID != c.ID {
		t.Fatalf("round-trip alterado: %+v != %+v", got, c)
	}
	if enc := encodePageCursor(nil); enc != "" {
		t.Fatalf("cursor nil debe codificar a vacío, got %q", enc)
	}
	if after, err := decodePageCursor(""); err != nil || !after.IsZero() {
		t.Fatalf("token vacío debe ser primera página, got %+v err=%v", after, err)
	}
}

func TestPageCursorCorruptTokens(t *testing.T) {
	// "%%%": base64 inválido; "bm90LWpzb24": base64("not-json") no es JSON;
	// "e30": base64("{}") sin timestamp (cero) también se rechaza.
	for _, bad := range []string{"%%%", "bm90LWpzb24", "e30"} {
		if _, err := decodePageCursor(bad); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("decodePageCursor(%q): err=%v, quiero ErrInvalidInput", bad, err)
		}
	}
}

// TestDeployExigeSuscripcionALaSuite fija la regla comercial: Sytue Cloud no se
// vende suelto. Una organización en plan Free SIN puestos de suite contratados no
// puede desplegar, y el error es el de dominio (que la API traduce a 402), no un
// fallo genérico.
func TestDeployExigeSuscripcionALaSuite(t *testing.T) {
	fs := &fakeStore{
		org:         domain.Organization{Plan: domain.PlanFree},
		memberCount: 0, // organización sin miembros ⇒ sin puestos de suite
		project:     domain.Project{Slug: "web", ProductionRef: "main"},
		build:       domain.Build{ID: uuid.New()},
		dep:         domain.Deployment{ID: uuid.New()},
	}
	s := newTestService(fs, &capturePub{})
	_, _, err := s.Deploy(tenantCtx(t), DeployInput{
		ProjectID: uuid.New(), CommitSHA: "abc1234", Target: domain.TargetProduction,
	})
	if !errors.Is(err, domain.ErrSuiteRequired) {
		t.Fatalf("sin suscripción a la suite el deploy debe rechazarse con ErrSuiteRequired, dio: %v", err)
	}
}

// TestPuestosDeSuiteSeDerivanDeLosMiembros fija la regla: los puestos facturables
// salen de cuánta gente hay en la organización, no de un número tecleado. Añadir a
// alguien al equipo añade su puesto sin que nadie tenga que acordarse.
func TestPuestosDeSuiteSeDerivanDeLosMiembros(t *testing.T) {
	for _, n := range []int{0, 1, 7} {
		fs := &fakeStore{memberCount: n}
		s := newTestService(fs, &capturePub{})
		got, err := s.SuiteSeats(tenantCtx(t))
		if err != nil {
			t.Fatalf("SuiteSeats: %v", err)
		}
		if got != n {
			t.Errorf("con %d miembros, puestos = %d", n, got)
		}
		// Y el requisito del plan Free se satisface justo cuando hay alguien.
		ok, err := s.MeetsSuiteRequirement(tenantCtx(t))
		if err != nil {
			t.Fatalf("MeetsSuiteRequirement: %v", err)
		}
		if ok != (n >= 1) {
			t.Errorf("con %d miembros, cumple=%v", n, ok)
		}
	}
}

// TestAuditoriaDeIdentidadYCredenciales fija que las acciones que un auditor mira
// primero —quién entra y sale de la organización, y qué credenciales de máquina se
// emiten o revocan— dejan rastro. Antes NINGUNA de ellas se auditaba: el registro
// solo cubría facturación, dominios y despliegues.
func TestAuditoriaDeIdentidadYCredenciales(t *testing.T) {
	t.Run("alta y baja de miembro", func(t *testing.T) {
		fs := &fakeStore{memberCount: 2}
		s := newTestService(fs, &capturePub{})
		if _, err := s.AddMember(tenantCtx(t), "u-nuevo", domain.RoleAdmin); err != nil {
			t.Fatalf("AddMember: %v", err)
		}
		if err := s.RemoveMember(tenantCtx(t), "u-nuevo"); err != nil {
			t.Fatalf("RemoveMember: %v", err)
		}
		assertAudited(t, fs, "member.upsert", "member.remove")
		// El rol concedido tiene que constar: es el dato que revisa el auditor.
		for _, e := range fs.audited {
			if e.Action == "member.upsert" && e.Detail["role"] != "admin" {
				t.Errorf("el alta debe registrar el rol concedido, got %v", e.Detail["role"])
			}
		}
	})

	t.Run("emisión y revocación de token, sin filtrar el secreto", func(t *testing.T) {
		fs := &fakeStore{}
		s := newTestService(fs, &capturePub{})
		tok, err := s.CreateAPIToken(tenantCtx(t), CreateTokenInput{
			Name: "ci", Scope: domain.ScopeDeploy,
		})
		if err != nil {
			t.Fatalf("CreateAPIToken: %v", err)
		}
		if err := s.RevokeAPIToken(tenantCtx(t), tok.ID); err != nil {
			t.Fatalf("RevokeAPIToken: %v", err)
		}
		assertAudited(t, fs, "token.create", "token.revoke")
		// El registro de auditoría NO puede convertirse en un almacén de credenciales.
		for _, e := range fs.audited {
			for k, v := range e.Detail {
				if str, ok := v.(string); ok && str != "" && str == tok.Plaintext {
					t.Fatalf("el secreto del token se filtró en la auditoría (campo %q)", k)
				}
			}
		}
	})
}

// assertAudited comprueba que se registraron exactamente esas acciones, en orden.
func assertAudited(t *testing.T, fs *fakeStore, want ...string) {
	t.Helper()
	got := make([]string, 0, len(fs.audited))
	for _, e := range fs.audited {
		got = append(got, e.Action)
	}
	if len(got) != len(want) {
		t.Fatalf("acciones auditadas = %v, quiero %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("acción %d = %q, quiero %q", i, got[i], want[i])
		}
	}
}
