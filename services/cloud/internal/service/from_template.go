// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// ---------------------------------------------------------------------------
// Alta de proyecto desde plantilla — UNA transacción, todo o nada
// ---------------------------------------------------------------------------
//
// Hasta aquí, crear un proyecto con base de datos y variables eran CUATRO o más
// peticiones POST encadenadas por el cliente, cada una con su transacción: si
// fallaba la tercera, quedaba un proyecto huérfano con servicios a medias que
// nadie limpiaba y que además ya se estaba facturando. Y el proyecto recién
// creado no desplegaba nada: la única acción disponible era «volver a desplegar»
// una app que nunca había desplegado.
//
// ApplyTemplate compone los mismos pasos DENTRO de una sola transacción
// (store.InTemplateTx) y termina encolando el primer despliegue.

// templateStore es el seam transaccional del store: *store.Store lo cumple. Se
// toma por aserción sobre s.store (en vez de ampliar dataStore) para no tocar
// service.go.
type templateStore interface {
	InTemplateTx(ctx context.Context, fn func(store.TemplateOps) error) error
}

// La aserción sobre s.store es en caliente: esta comprobación en compilación es
// la que impide que una deriva de firma la convierta en un 500 silencioso.
var _ templateStore = (*store.Store)(nil)

// ApplyTemplateInput son los datos del alta desde plantilla. Todo lo demás
// (framework, comandos de build, puerto, sonda, servicios y variables) sale del
// manifiesto: el usuario no reconfigura a mano lo que la plantilla ya sabe.
type ApplyTemplateInput struct {
	TemplateID string
	Slug       string
	Name       string
	RepoURL    string
	// Answers son las respuestas a las variables con origin=ask, por clave. Se
	// valida contra lo que DECLARA la plantilla: ni falta ninguna ni sobra ninguna.
	Answers map[string]string
	// InstanceSize pisa el tamaño sugerido por la plantilla ("" = el suyo).
	InstanceSize string
	// ActorSub es el subject de quien crea (dueño del proyecto y autor del deploy).
	ActorSub string
}

// TemplateApplied es lo que quedó creado. NUNCA lleva el valor de una variable
// secreta: los secretos generados salen de aquí cifrados hacia la base de datos
// y no vuelven (domain.EnvVar en blanco cuando is_secret).
type TemplateApplied struct {
	Project    domain.Project          `json:"project"`
	Services   []domain.ManagedService `json:"services"`
	Env        []domain.EnvVar         `json:"env"`
	Build      domain.Build            `json:"build"`
	Deployment domain.Deployment       `json:"deployment"`
	TemplateID string                  `json:"template_id"`
	// TemplateVersion deja registrado con qué revisión del catálogo se creó.
	TemplateVersion string `json:"template_version"`
}

// ApplyTemplate crea el proyecto de una plantilla ENTERO y de una vez: proyecto,
// servicios gestionados, variables de entorno y el primer despliegue encolado.
// Si algo falla, no queda nada (una sola transacción).
//
// Devuelve además el secreto de webhook git en claro UNA vez, igual que
// CreateProject; el handler decide a quién se lo enseña.
func (s *Service) ApplyTemplate(ctx context.Context, in ApplyTemplateInput) (TemplateApplied, string, error) {
	t := tenancy.MustFrom(ctx)
	if s.sealer == nil {
		// El alta cifra sí o sí (secreto de webhook, y las variables generadas):
		// sin cifrador se para aquí, no a mitad de la transacción.
		return TemplateApplied{}, "", errors.New("service: no hay cifrador configurado")
	}
	tpl, ok := domain.StarterTemplateByID(strings.TrimSpace(in.TemplateID))
	if !ok {
		return TemplateApplied{}, "", domain.ErrNotFound
	}
	if !domain.ValidSlug(in.Slug) {
		return TemplateApplied{}, "", fmt.Errorf("%w: slug de proyecto inválido", domain.ErrInvalidInput)
	}
	// Anti-SSRF: la repo_url la fetchea el Job de build server-side.
	if err := domain.ValidateRepoURL(in.RepoURL); err != nil {
		return TemplateApplied{}, "", err
	}
	size := strings.TrimSpace(in.InstanceSize)
	if size == "" {
		size = tpl.InstanceSize
	}
	if err := domain.ValidateInstanceSize(size); err != nil {
		return TemplateApplied{}, "", err
	}
	// Las respuestas se validan ANTES de abrir nada: una clave que la plantilla no
	// declara es inyección de variables de entorno arbitrarias en el pod, no un
	// descuido que se pueda ignorar.
	env, err := s.templateEnvRecords(tpl, in.Answers)
	if err != nil {
		return TemplateApplied{}, "", err
	}
	seam, ok := s.store.(templateStore)
	if !ok {
		return TemplateApplied{}, "", errors.New("service: el store no soporta el alta atómica desde plantilla")
	}

	// Mismas verjas que Deploy y por el mismo motivo: esto termina encolando un
	// build y un despliegue, o sea, cómputo facturable. Si el alta desde plantilla
	// no las comprobara sería la puerta lateral que las esquiva.
	if err := s.checkDeployAllowed(ctx); err != nil {
		return TemplateApplied{}, "", err
	}
	if _, err := s.EnsureOrg(ctx, ""); err != nil {
		return TemplateApplied{}, "", err
	}
	org, err := s.store.GetOrganization(ctx)
	if err != nil {
		return TemplateApplied{}, "", err
	}
	quota := domain.QuotasFor(org.Plan)

	// Secreto de webhook git por proyecto (aleatorio de 256 bits), cifrado en
	// reposo; en claro solo viaja de vuelta esta vez.
	webhookSecret, _, err := GenerateAPIToken()
	if err != nil {
		return TemplateApplied{}, "", err
	}
	whEnc, err := s.sealer.SealString(webhookSecret)
	if err != nil {
		return TemplateApplied{}, "", fmt.Errorf("service: cifrar secreto de webhook: %w", err)
	}

	out := TemplateApplied{TemplateID: tpl.ID, TemplateVersion: tpl.Version}
	err = seam.InTemplateTx(ctx, func(ops store.TemplateOps) error {
		var e error
		out, e = applyTemplateTx(ctx, ops, applyPlan{
			tpl: tpl, in: in, size: size, env: env, quota: quota, whEnc: whEnc,
			url: func(p domain.Project) string { return s.DeploymentURL(p, domain.TargetProduction, nil) },
		})
		return e
	})
	if err != nil {
		return TemplateApplied{}, "", err
	}

	// Fuera de la transacción: avisos y auditoría. Publicarlos dentro no los haría
	// más atómicos (el bus vive en otra transacción) y sí anunciaría trabajo que
	// todavía podría deshacerse.
	for _, m := range out.Services {
		_ = s.emit(ctx, EventServiceRequested, t.Slug, map[string]any{
			"service_id": m.ID, "kind": string(m.Kind), "project_id": m.ProjectID,
		})
	}
	_ = s.emit(ctx, EventBuildEnqueued, t.Slug, map[string]any{
		"build_id": out.Build.ID, "deployment_id": out.Deployment.ID, "project_id": out.Project.ID,
	})
	// Auditoría: el QUÉ (plantilla, repo, servicios), jamás el valor de una
	// variable — ni siquiera el de las que ha tecleado el usuario.
	s.audit(ctx, "project.create_from_template", out.Project.Slug, map[string]any{
		"template_id": tpl.ID, "template_version": tpl.Version,
		"repo_url": out.Project.RepoURL, "services": len(out.Services), "env": len(out.Env),
	})
	return out, webhookSecret, nil
}

// checkDeployAllowed aplica las dos verjas de negocio que protegen el cómputo:
// tope de gasto del mes y suscripción a la suite. Son las mismas de Deploy.
func (s *Service) checkDeployAllowed(ctx context.Context) error {
	within, err := s.WithinSpendLimit(ctx)
	if err != nil {
		return err
	}
	if !within {
		return domain.ErrSpendLimitExceeded
	}
	ok, err := s.MeetsSuiteRequirement(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrSuiteRequired
	}
	return nil
}

// applyPlan es todo lo ya resuelto y validado que consume el cuerpo transaccional.
type applyPlan struct {
	tpl   domain.TemplateManifest
	in    ApplyTemplateInput
	size  string
	env   []store.EnvVarRecord
	quota domain.PlanQuota
	whEnc []byte
	// url deriva la URL pública de producción del proyecto ya creado.
	url func(domain.Project) string
}

// applyTemplateTx escribe el alta completa con las operaciones atadas a UNA
// transacción. Cualquier error aborta el conjunto: el llamante no tiene que
// limpiar nada porque no llega a existir nada.
func applyTemplateTx(ctx context.Context, ops store.TemplateOps, p applyPlan) (TemplateApplied, error) {
	name := strings.TrimSpace(p.in.Name)
	if name == "" {
		name = p.in.Slug
	}
	out := TemplateApplied{TemplateID: p.tpl.ID, TemplateVersion: p.tpl.Version}
	proj, err := ops.CreateProject(ctx, store.NewProjectRecord{
		Slug:             p.in.Slug,
		Name:             name,
		RepoURL:          strings.TrimSpace(p.in.RepoURL),
		GitProvider:      domain.GitGeneric,
		ProductionRef:    "main",
		Framework:        p.tpl.Framework,
		BuildStrategy:    p.tpl.Build.Strategy,
		InstallCmd:       p.tpl.Build.Install,
		BuildCommand:     p.tpl.Build.Build,
		OutputDir:        p.tpl.Build.OutputDir,
		AppPort:          p.tpl.AppPort,
		HealthPath:       p.tpl.HealthPath,
		InstanceSize:     p.size,
		OwnerSub:         p.in.ActorSub,
		WebhookSecretEnc: p.whEnc,
	}, p.quota.MaxProjects)
	if err != nil {
		return TemplateApplied{}, err
	}
	out.Project = proj

	out.Services = make([]domain.ManagedService, 0, len(p.tpl.Services))
	for _, kind := range p.tpl.Services {
		m, err := ops.ProvisionService(ctx, store.NewServiceRecord{
			ProjectID: proj.ID,
			Kind:      kind,
			Name:      string(kind),
			Config:    serviceDefaults(kind, p.quota),
		}, p.quota.MaxManagedServices)
		if err != nil {
			return TemplateApplied{}, err
		}
		out.Services = append(out.Services, m)
	}

	out.Env = make([]domain.EnvVar, 0, len(p.env))
	for _, rec := range p.env {
		rec.ProjectID = proj.ID
		v, err := ops.UpsertEnvVar(ctx, rec)
		if err != nil {
			return TemplateApplied{}, err
		}
		out.Env = append(out.Env, v)
	}

	// El primer despliegue: la razón por la que el proyecto nace ejecutándose y no
	// esperando a que alguien pulse «volver a desplegar» algo que nunca desplegó.
	build, err := ops.EnqueueBuild(ctx, store.NewBuildRecord{
		ProjectID:   proj.ID,
		GitRef:      proj.ProductionRef,
		Framework:   proj.Framework,
		TriggeredBy: p.in.ActorSub,
	})
	if err != nil {
		return TemplateApplied{}, err
	}
	out.Build = build
	dep, err := ops.CreateDeployment(ctx, store.NewDeploymentRecord{
		ProjectID: proj.ID,
		BuildID:   &build.ID,
		Target:    domain.TargetProduction,
		GitRef:    proj.ProductionRef,
		URL:       p.url(proj),
		CreatedBy: p.in.ActorSub,
	})
	if err != nil {
		return TemplateApplied{}, err
	}
	out.Deployment = dep
	return out, nil
}

// serviceDefaults son los ajustes del plan para el servicio, espejo de los que
// pone ProvisionService cuando se aprovisiona suelto (tamaño del volumen y
// retención de copias). storage/auth no llevan ninguno: los resuelve el
// aprovisionamiento entero.
func serviceDefaults(kind domain.ServiceKind, quota domain.PlanQuota) map[string]string {
	if kind != domain.ServicePostgres {
		return map[string]string{}
	}
	return map[string]string{
		"pvc_size":              quota.PostgresPVCSize,
		"backup_retention_days": strconv.Itoa(quota.BackupRetentionDays),
	}
}

// templateEnvRecords traduce las variables DECLARADAS por la plantilla a filas
// listas para persistir, resolviendo cada una según su origen:
//
//   - value       → literal del manifiesto (nunca secreto: vive en el repositorio).
//   - generate    → secreto aleatorio de 256 bits que genera LA PLATAFORMA y se
//     guarda cifrado. El usuario no inventa contraseñas ni las teclea.
//   - ask         → la respuesta del usuario, validada contra lo declarado.
//   - from_service→ NO se persiste: la inyecta el controlador desde las
//     credenciales del servicio gestionado. Aquí solo estaba declarada.
func (s *Service) templateEnvRecords(tpl domain.TemplateManifest, answers map[string]string) ([]store.EnvVarRecord, error) {
	if err := validateTemplateAnswers(tpl, answers); err != nil {
		return nil, err
	}
	out := make([]store.EnvVarRecord, 0, len(tpl.Env))
	for _, e := range tpl.Env {
		rec := store.EnvVarRecord{Key: e.Key, IsSecret: e.Secret, Scope: domain.ScopeAll}
		var plain string
		switch e.Origin {
		case domain.EnvOriginFromService:
			continue
		case domain.EnvOriginValue:
			plain = e.Value
		case domain.EnvOriginAsk:
			plain = answers[e.Key]
		case domain.EnvOriginGenerate:
			secret, _, err := GenerateAPIToken() // 256 bits de aleatoriedad
			if err != nil {
				return nil, err
			}
			plain, rec.IsSecret = secret, true
		default:
			// Inalcanzable: el catálogo no carga con un origen desconocido.
			return nil, fmt.Errorf("%w: origen de variable desconocido %q", domain.ErrInvalidInput, e.Origin)
		}
		if rec.IsSecret {
			enc, err := s.sealer.SealString(plain)
			if err != nil {
				return nil, fmt.Errorf("service: cifrar variable de plantilla: %w", err)
			}
			rec.ValueEnc = enc
		} else {
			rec.ValuePlain = plain
		}
		out = append(out, rec)
	}
	return out, nil
}

// validateTemplateAnswers comprueba que las respuestas son EXACTAMENTE las que
// la plantilla pide: ni de menos ni de más.
//
// Lo segundo es lo que importa de verdad: sin ello, `answers` sería un canal
// para escribir cualquier variable de entorno en el pod de la app —incluidas las
// que el controlador inyecta (DATABASE_URL) o las que dirigen el runtime—, con
// la firma de un alta perfectamente legítima.
func validateTemplateAnswers(tpl domain.TemplateManifest, answers map[string]string) error {
	asked := make(map[string]bool, len(tpl.Env))
	for _, e := range tpl.Env {
		if e.Origin == domain.EnvOriginAsk {
			asked[e.Key] = true
		}
	}
	for k := range answers {
		if !asked[k] {
			return fmt.Errorf("%w: la plantilla %s no declara la variable %q", domain.ErrInvalidInput, tpl.ID, k)
		}
	}
	for _, e := range tpl.Env {
		if e.Origin == domain.EnvOriginAsk && strings.TrimSpace(answers[e.Key]) == "" {
			return fmt.Errorf("%w: falta el valor de %s (%s)", domain.ErrInvalidInput, e.Key, e.Prompt)
		}
	}
	return nil
}
