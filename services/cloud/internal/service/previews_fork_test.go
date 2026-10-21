// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// Un pull request de un FORK no genera vista previa; uno del propio repositorio,
// sí (CLOUD-01, decisión del dueño del 2026-10-06: «sin forks y sin producción»).
//
// El test recorre el camino entero del webhook —el cuerpo tal y como lo manda la
// forja, el parser y HandleGitEvent— y no solo el campo: lo que se quiere fijar es
// que el código de alguien sin permiso de escritura NO acaba construido y
// desplegado, y eso solo se ve mirando si se llegó a crear el despliegue.
func TestPRDeForkNoGeneraVistaPrevia(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	casos := []struct {
		nombre    string
		proveedor domain.GitProvider
		evento    string
		cuerpo    string
		despliega bool
	}{
		{
			nombre: "GitHub: PR desde un fork", proveedor: domain.GitHub, evento: "pull_request",
			cuerpo: `{"action":"opened","number":7,
				"pull_request":{"title":"t","head":{"sha":"` + sha + `","ref":"parche",
					"repo":{"id":222,"full_name":"mallory/web"}}},
				"repository":{"id":111,"full_name":"acme/web","clone_url":"https://github.com/acme/web.git"}}`,
		},
		{
			nombre: "GitHub: PR del propio repositorio", proveedor: domain.GitHub, evento: "pull_request",
			cuerpo: `{"action":"opened","number":7,
				"pull_request":{"title":"t","head":{"sha":"` + sha + `","ref":"rama",
					"repo":{"id":111,"full_name":"acme/web"}}},
				"repository":{"id":111,"full_name":"acme/web","clone_url":"https://github.com/acme/web.git"}}`,
			despliega: true,
		},
		{
			// El fork se borró con el PR abierto: la forja manda `head.repo: null`.
			nombre: "GitHub: PR con el repositorio de cabeza a null", proveedor: domain.GitHub, evento: "pull_request",
			cuerpo: `{"action":"synchronize","number":7,
				"pull_request":{"title":"t","head":{"sha":"` + sha + `","ref":"parche","repo":null}},
				"repository":{"id":111,"full_name":"acme/web","clone_url":"https://github.com/acme/web.git"}}`,
		},
		{
			// Un payload que no dice de dónde viene la cabeza no demuestra nada.
			nombre: "GitHub: PR sin repositorio de cabeza", proveedor: domain.GitHub, evento: "pull_request",
			cuerpo: `{"action":"opened","number":7,
				"pull_request":{"title":"t","head":{"sha":"` + sha + `","ref":"parche"}},
				"repository":{"id":111,"full_name":"acme/web","clone_url":"https://github.com/acme/web.git"}}`,
		},
		{
			// Sin ids (una forja compatible que no los mande): decide el nombre.
			nombre: "Gitea: mismo nombre completo, sin ids", proveedor: domain.GitHub, evento: "pull_request",
			cuerpo: `{"action":"opened","number":7,
				"pull_request":{"title":"t","head":{"sha":"` + sha + `","ref":"rama","repo":{"full_name":"Acme/Web"}}},
				"repository":{"full_name":"acme/web","clone_url":"https://github.com/acme/web.git"}}`,
			despliega: true,
		},
		{
			nombre: "GitLab: MR desde un fork", proveedor: domain.GitLab, evento: "Merge Request Hook",
			cuerpo: `{"object_attributes":{"action":"open","iid":7,"title":"t","source_branch":"parche",
					"last_commit":{"id":"` + sha + `"},"source_project_id":222,"target_project_id":111},
				"project":{"git_http_url":"https://github.com/acme/web.git"}}`,
		},
		{
			nombre: "GitLab: MR del propio proyecto", proveedor: domain.GitLab, evento: "Merge Request Hook",
			cuerpo: `{"object_attributes":{"action":"open","iid":7,"title":"t","source_branch":"rama",
					"last_commit":{"id":"` + sha + `"},"source_project_id":111,"target_project_id":111},
				"project":{"git_http_url":"https://github.com/acme/web.git"}}`,
			despliega: true,
		},
		{
			nombre: "GitLab: MR sin proyecto de origen", proveedor: domain.GitLab, evento: "Merge Request Hook",
			cuerpo: `{"object_attributes":{"action":"open","iid":7,"title":"t","source_branch":"parche",
					"last_commit":{"id":"` + sha + `"},"target_project_id":111},
				"project":{"git_http_url":"https://github.com/acme/web.git"}}`,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			pid := uuid.New()
			proyecto := domain.Project{ID: pid, Slug: "web", ProductionRef: "main"}
			fs := &fakeStore{
				memberCount: 1,
				resolved:    store.ResolvedRepo{Tenant: "acme", Project: proyecto},
				project:     proyecto,
				build:       domain.Build{ID: uuid.New()},
				dep:         domain.Deployment{ID: uuid.New()},
			}
			s := newTestService(fs, &capturePub{})

			ev, err := ParseWebhook(c.proveedor, c.evento, []byte(c.cuerpo))
			if err != nil {
				t.Fatalf("ParseWebhook: %v", err)
			}
			dep, ignorado, err := s.HandleGitEvent(context.Background(), fs.resolved, ev, "entrega-1", []byte(c.cuerpo))
			if err != nil {
				t.Fatalf("HandleGitEvent: %v", err)
			}

			if !c.despliega {
				if dep != nil || !ignorado {
					t.Errorf("un PR de fork devolvió un despliegue (dep=%v, ignorado=%v)", dep, ignorado)
				}
				if fs.newBuild != nil || fs.newDep != nil {
					t.Errorf("un PR de fork llegó a encolar build=%v y despliegue=%v", fs.newBuild, fs.newDep)
				}
				return
			}
			if dep == nil || ignorado {
				t.Fatalf("un PR del propio repositorio debe generar vista previa (dep=%v, ignorado=%v)", dep, ignorado)
			}
			if fs.newDep == nil || fs.newDep.Target != domain.TargetPreview {
				t.Errorf("el despliegue creado no es una vista previa: %+v", fs.newDep)
			}
			if fs.newBuild == nil || fs.newBuild.CommitSHA != sha {
				t.Errorf("no se encoló el build del commit del PR: %+v", fs.newBuild)
			}
		})
	}
}

// Solo una base de datos o un almacén pueden crearse «para vistas previas»; el
// destino por defecto es producción.
func TestDestinoDeUnServicio(t *testing.T) {
	t.Parallel()
	casos := []struct {
		kind    domain.ServiceKind
		target  domain.DeployTarget
		quiere  domain.DeployTarget
		rechaza bool
	}{
		{kind: domain.ServicePostgres, target: "", quiere: domain.TargetProduction},
		{kind: domain.ServicePostgres, target: domain.TargetProduction, quiere: domain.TargetProduction},
		{kind: domain.ServicePostgres, target: domain.TargetPreview, quiere: domain.TargetPreview},
		{kind: domain.ServiceStorage, target: domain.TargetPreview, quiere: domain.TargetPreview},
		{kind: domain.ServiceApp, target: domain.TargetPreview, rechaza: true},
		{kind: domain.ServiceFunctions, target: domain.TargetPreview, rechaza: true},
		{kind: domain.ServiceWorker, target: domain.TargetPreview, rechaza: true},
		{kind: domain.ServicePostgres, target: "all", rechaza: true},
	}
	for _, c := range casos {
		in := ProvisionServiceInput{Kind: c.kind, Target: c.target}
		err := validateServiceTarget(&in)
		if c.rechaza {
			if err == nil {
				t.Errorf("%s con destino %q debía rechazarse", c.kind, c.target)
			}
			continue
		}
		if err != nil || in.Target != c.quiere {
			t.Errorf("%s con destino %q: quedó %q, err=%v; se esperaba %q", c.kind, c.target, in.Target, err, c.quiere)
		}
	}
}
