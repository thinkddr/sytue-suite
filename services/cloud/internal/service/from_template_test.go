// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// La plantilla del catálogo con la que se ejercita el alta: aprovisiona un
// Postgres y declara SESSION_SECRET con origin=generate y dos variables
// from_service. Es decir, toca los cuatro caminos que importan.
const tplID = "go-api-postgres"

// ---------------------------------------------------------------------------
// Doble del store con transacción de verdad-a-medias: las escrituras van a un
// BORRADOR y solo pasan a `committed` si el cuerpo de la transacción termina sin
// error. Es el contrato que store.InTemplateTx delega en PostgreSQL, así que un
// paso que se escapara de la transacción (llamando al store suelto) quedaría a
// la vista: lo registraría el fakeStore embebido, no el borrador.
// ---------------------------------------------------------------------------

type txWrites struct {
	projects []store.NewProjectRecord
	services []store.NewServiceRecord
	env      []store.EnvVarRecord
	builds   []store.NewBuildRecord
	deploys  []store.NewDeploymentRecord
}

func (w txWrites) rows() int {
	return len(w.projects) + len(w.services) + len(w.env) + len(w.builds) + len(w.deploys)
}

type fakeTemplateStore struct {
	*fakeStore
	// failAt hace fallar la n-ésima escritura (1 = la primera); 0 = ninguna.
	failAt int
	// calls son las escrituras intentadas dentro de la transacción.
	calls int
	// committed es lo que quedó tras el commit (vacío si hubo rollback).
	committed txWrites
}

func (f *fakeTemplateStore) InTemplateTx(ctx context.Context, fn func(store.TemplateOps) error) error {
	_ = ctx
	ops := &fakeTemplateOps{owner: f}
	if err := fn(ops); err != nil {
		return err // rollback: el borrador se descarta entero
	}
	f.committed = ops.draft
	return nil
}

type fakeTemplateOps struct {
	owner *fakeTemplateStore
	draft txWrites
}

func (o *fakeTemplateOps) step() error {
	o.owner.calls++
	if o.owner.failAt == o.owner.calls {
		return fmt.Errorf("fallo simulado en el paso %d", o.owner.calls)
	}
	return nil
}

func (o *fakeTemplateOps) CreateProject(_ context.Context, rec store.NewProjectRecord, _ int) (domain.Project, error) {
	if err := o.step(); err != nil {
		return domain.Project{}, err
	}
	o.draft.projects = append(o.draft.projects, rec)
	return domain.Project{
		ID: uuid.New(), Tenant: "acme", Slug: rec.Slug, Name: rec.Name,
		RepoURL: rec.RepoURL, ProductionRef: rec.ProductionRef, Framework: rec.Framework,
		InstanceSize: rec.InstanceSize,
	}, nil
}

func (o *fakeTemplateOps) ProvisionService(_ context.Context, rec store.NewServiceRecord, _ int) (domain.ManagedService, error) {
	if err := o.step(); err != nil {
		return domain.ManagedService{}, err
	}
	o.draft.services = append(o.draft.services, rec)
	return domain.ManagedService{ID: uuid.New(), ProjectID: rec.ProjectID, Kind: rec.Kind, Name: rec.Name}, nil
}

func (o *fakeTemplateOps) UpsertEnvVar(_ context.Context, rec store.EnvVarRecord) (domain.EnvVar, error) {
	if err := o.step(); err != nil {
		return domain.EnvVar{}, err
	}
	o.draft.env = append(o.draft.env, rec)
	v := domain.EnvVar{
		ID: uuid.New(), ProjectID: rec.ProjectID, Key: rec.Key,
		Value: rec.ValuePlain, IsSecret: rec.IsSecret, Scope: rec.Scope,
	}
	if v.IsSecret {
		v.Value = "" // igual que el store real: el valor de un secreto no vuelve
	}
	return v, nil
}

func (o *fakeTemplateOps) EnqueueBuild(_ context.Context, rec store.NewBuildRecord) (domain.Build, error) {
	if err := o.step(); err != nil {
		return domain.Build{}, err
	}
	o.draft.builds = append(o.draft.builds, rec)
	return domain.Build{ID: uuid.New(), ProjectID: rec.ProjectID, GitRef: rec.GitRef, Status: domain.BuildQueued}, nil
}

func (o *fakeTemplateOps) CreateDeployment(_ context.Context, rec store.NewDeploymentRecord) (domain.Deployment, error) {
	if err := o.step(); err != nil {
		return domain.Deployment{}, err
	}
	o.draft.deploys = append(o.draft.deploys, rec)
	return domain.Deployment{
		ID: uuid.New(), ProjectID: rec.ProjectID, BuildID: rec.BuildID,
		Target: rec.Target, Status: domain.DeployQueued, URL: rec.URL,
	}, nil
}

// templateSvc arma un Service con el doble del store y un cifrador real.
func templateSvc(t *testing.T, failAt int) (*Service, *fakeTemplateStore, context.Context) {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	sealer, err := crypto.NewSealer(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	st := &fakeTemplateStore{
		fakeStore: &fakeStore{
			org:         domain.Organization{Slug: "acme", Plan: domain.PlanFree},
			memberCount: 1, // cumple el requisito de suite del plan Free
		},
		failAt: failAt,
	}
	svc := &Service{
		cfg:    config.Config{AppsDomain: "apps.sytue.test", PreviewDomain: "preview.sytue.test"},
		store:  st,
		sealer: sealer,
	}
	return svc, st, tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
}

func validInput() ApplyTemplateInput {
	return ApplyTemplateInput{
		TemplateID: tplID,
		Slug:       "mi-api",
		Name:       "Mi API",
		RepoURL:    "https://github.com/acme/mi-api",
		ActorSub:   "user-1",
	}
}

// (a) La razón de ser de la ola: si falla un paso intermedio no se persiste NADA.
// Se hace fallar el TERCERO (proyecto y Postgres ya escritos, variable a medias).
func TestApplyTemplateFalloIntermedioNoPersisteNada(t *testing.T) {
	svc, st, ctx := templateSvc(t, 3)

	_, _, err := svc.ApplyTemplate(ctx, validInput())
	if err == nil {
		t.Fatal("se esperaba error del tercer paso")
	}
	if n := st.committed.rows(); n != 0 {
		t.Fatalf("tras el fallo quedaron %d filas persistidas; se esperaban 0 (%+v)", n, st.committed)
	}
	if st.calls != 3 {
		t.Fatalf("se intentaron %d escrituras; se esperaban 3 (aborta en la que falla)", st.calls)
	}
	// Nada puede haberse escrito por fuera de la transacción: el store suelto no
	// registró ni un proyecto, ni un servicio, ni una variable.
	if st.newProj != nil || st.newSvc != nil || st.envRec != nil || st.newBuild != nil || st.newDep != nil {
		t.Fatal("hubo escrituras FUERA de la transacción del alta")
	}
}

// (b) Una variable origin=generate acaba cifrada, marcada como secreta, y su
// valor no aparece por ningún lado en la respuesta de la API.
func TestApplyTemplateGeneraSecretoCifradoYNoLoDevuelve(t *testing.T) {
	svc, st, ctx := templateSvc(t, 0)

	out, _, err := svc.ApplyTemplate(ctx, validInput())
	if err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}
	var rec store.EnvVarRecord
	for _, e := range st.committed.env {
		if e.Key == "SESSION_SECRET" {
			rec = e
		}
	}
	switch {
	case rec.Key == "":
		t.Fatal("no se persistió SESSION_SECRET")
	case !rec.IsSecret:
		t.Fatal("SESSION_SECRET debería ser secreta")
	case len(rec.ValueEnc) == 0:
		t.Fatal("SESSION_SECRET debería guardarse cifrada")
	case rec.ValuePlain != "":
		t.Fatal("SESSION_SECRET no puede guardarse en claro")
	}
	plain, err := svc.sealer.OpenString(rec.ValueEnc)
	if err != nil {
		t.Fatalf("descifrar SESSION_SECRET: %v", err)
	}
	if len(plain) < 40 { // prefijo + 256 bits en base64
		t.Fatalf("secreto generado demasiado corto: %d caracteres", len(plain))
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), plain) {
		t.Fatal("el secreto generado viaja en la respuesta de la API")
	}
	for _, v := range out.Env {
		if v.Key == "SESSION_SECRET" && v.Value != "" {
			t.Fatal("la respuesta trae el valor de una variable secreta")
		}
	}
	// Las variables from_service NO se persisten: las inyecta el controlador.
	for _, e := range st.committed.env {
		if e.Key == "DATABASE_URL" || e.Key == "POSTGREST_URL" {
			t.Fatalf("%s no debe persistirse: la inyecta el controlador", e.Key)
		}
	}
}

// (c) Una respuesta con una clave que la plantilla no declara se rechaza: es
// inyección de variables de entorno arbitrarias, no un descuido.
func TestApplyTemplateRechazaAnswersNoDeclaradas(t *testing.T) {
	svc, st, ctx := templateSvc(t, 0)

	in := validInput()
	in.Answers = map[string]string{"DATABASE_URL": "postgres://yo/mando"}
	_, _, err := svc.ApplyTemplate(ctx, in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v; se esperaba ErrInvalidInput", err)
	}
	if st.calls != 0 {
		t.Fatal("se abrió la transacción con respuestas inválidas")
	}
}

// (d) El alta deja un primer despliegue encolado (antes no había ninguno: el
// proyecto nacía sin nada que desplegar).
func TestApplyTemplateEncolaElPrimerDespliegue(t *testing.T) {
	svc, st, ctx := templateSvc(t, 0)

	out, secret, err := svc.ApplyTemplate(ctx, validInput())
	if err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}
	if len(st.committed.builds) != 1 || len(st.committed.deploys) != 1 {
		t.Fatalf("builds=%d deploys=%d; se esperaba 1 y 1", len(st.committed.builds), len(st.committed.deploys))
	}
	dep := st.committed.deploys[0]
	if dep.Target != domain.TargetProduction {
		t.Fatalf("target = %q; se esperaba production", dep.Target)
	}
	if dep.BuildID == nil || *dep.BuildID != out.Build.ID {
		t.Fatal("el despliegue no apunta a la build recién encolada")
	}
	if out.Deployment.Status != domain.DeployQueued {
		t.Fatalf("estado del despliegue = %q; se esperaba queued", out.Deployment.Status)
	}
	if dep.URL != "https://mi-api.apps.sytue.test" {
		t.Fatalf("url = %q", dep.URL)
	}
	// El proyecto se crea con los ajustes del manifiesto, no con los del cliente.
	proj := st.committed.projects[0]
	if proj.Framework != domain.FrameworkGo || proj.HealthPath != "/healthz" || proj.InstanceSize != "standard" {
		t.Fatalf("el proyecto no heredó los ajustes de la plantilla: %+v", proj)
	}
	// Y el Postgres de la plantilla queda aprovisionado con los ajustes del plan.
	if len(st.committed.services) != 1 || st.committed.services[0].Kind != domain.ServicePostgres {
		t.Fatalf("servicios = %+v; se esperaba un postgres", st.committed.services)
	}
	if st.committed.services[0].Config["pvc_size"] == "" {
		t.Fatal("el postgres se aprovisionó sin tamaño de volumen")
	}
	if secret == "" {
		t.Fatal("el alta debe devolver el secreto de webhook una vez")
	}
	if len(st.audited) != 1 || st.audited[0].Action != "project.create_from_template" {
		t.Fatalf("auditoría = %+v", st.audited)
	}
	if fmt.Sprint(st.audited[0].Detail) == "" || strings.Contains(fmt.Sprint(st.audited[0].Detail), secret) {
		t.Fatal("la auditoría no puede llevar secretos")
	}
}

// Plantilla desconocida: 404, no un alta a medias.
func TestApplyTemplateDesconocida(t *testing.T) {
	svc, _, ctx := templateSvc(t, 0)
	in := validInput()
	in.TemplateID = "no-existe"
	if _, _, err := svc.ApplyTemplate(ctx, in); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v; se esperaba ErrNotFound", err)
	}
}

// Las respuestas a origin=ask: exactamente las declaradas, ni una de menos ni
// una de más. El catálogo aún no trae ninguna plantilla con preguntas, así que
// la regla se ejercita con un manifiesto sintético.
func TestValidateTemplateAnswers(t *testing.T) {
	tpl := domain.TemplateManifest{
		ID: "con-pregunta",
		Env: []domain.TemplateEnv{
			{Key: "ADMIN_EMAIL", Origin: domain.EnvOriginAsk, Prompt: "Correo del administrador"},
			{Key: "NODE_ENV", Origin: domain.EnvOriginValue, Value: "production"},
		},
	}
	cases := []struct {
		name    string
		answers map[string]string
		wantErr bool
	}{
		{"exacta", map[string]string{"ADMIN_EMAIL": "a@b.eu"}, false},
		{"falta la requerida", nil, true},
		{"vacía no cuenta", map[string]string{"ADMIN_EMAIL": "   "}, true},
		{"clave no declarada", map[string]string{"ADMIN_EMAIL": "a@b.eu", "PATH": "/evil"}, true},
		{"responde a una que no se pregunta", map[string]string{"ADMIN_EMAIL": "a@b.eu", "NODE_ENV": "dev"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateTemplateAnswers(tpl, c.answers)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v; wantErr = %v", err, c.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v; se esperaba ErrInvalidInput", err)
			}
		})
	}
}
