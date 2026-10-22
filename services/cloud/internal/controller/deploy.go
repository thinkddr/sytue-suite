// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// reconcileDeployment lleva un deployment no terminal a 'ready' (o 'error'):
// asegura el namespace del tenant, resuelve env + secretos + credenciales de
// servicios gestionados, los inyecta y aplica la app en Kapsule. Idempotente.
func (c *Controller) reconcileDeployment(ctx context.Context, dep domain.Deployment, proj domain.Project) {
	l := log.From(ctx).With("deployment_id", dep.ID, "tenant", dep.Tenant, "project", proj.Slug,
		"target", string(dep.Target))

	// El deployment necesita una imagen construida. Si la build aún no terminó,
	// se queda en 'building' hasta el siguiente ciclo.
	image, ready := c.imageFor(ctx, dep)
	if !ready {
		_ = c.store.SetDeploymentStatus(ctx, dep.Tenant, dep.ID, domain.DeployBuilding, "", "")
		return
	}
	if image == "" {
		_ = c.store.SetDeploymentStatus(ctx, dep.Tenant, dep.ID, domain.DeployError, "",
			"la build no produjo una imagen")
		return
	}

	tnt, err := tenancy.New(dep.Tenant)
	if err != nil {
		l.Warn("tenant inválido en deployment", "error", err)
		return
	}
	if err := c.platform.EnsureNamespace(ctx, tnt.Namespace(), dep.Tenant, c.tenantQuota(ctx, dep.Tenant)); err != nil {
		l.Warn("asegurar namespace", "error", err)
		return
	}

	spec, err := c.buildSpec(ctx, dep, proj, image, tnt.Namespace())
	if err != nil {
		l.Warn("resolver inyección de configuración", "error", err)
		return
	}

	// Una vista previa no hereda los secretos ni los servicios de producción. Si
	// eso la deja sin algo, se DICE mientras arranca: sin el aviso, una app que
	// no sube sin su clave se queda en «desplegando» y nadie sabe por qué.
	_ = c.store.SetDeploymentStatus(ctx, dep.Tenant, dep.ID, domain.DeployDeploying, "", c.previewNotice(ctx, dep, proj))

	// Paso de release ANTES de arrancar la versión nueva: es donde va la migración
	// de la base de datos. Si no termina bien, la app nueva NO arranca —correría
	// contra un esquema que no es el suyo, que es peor que no desplegar—.
	if !c.releaseDone(ctx, dep, spec, l) {
		return
	}

	appReady, err := c.platform.ApplyApp(ctx, spec)
	if err != nil {
		_ = c.store.SetDeploymentStatus(ctx, dep.Tenant, dep.ID, domain.DeployError, "", err.Error())
		l.Warn("aplicar app", "error", err)
		return
	}
	if !appReady {
		return // sigue 'deploying' hasta que las réplicas estén listas
	}
	if err := c.markReady(ctx, dep); err != nil {
		l.Warn("marcar deployment ready", "error", err)
		return
	}
	l.Info("deployment listo", "url", dep.URL)
}

// releaseDone gobierna el paso de release: devuelve true cuando se puede seguir
// con el arranque de la versión nueva.
//
// Sin comando configurado no hay paso y sigue todo igual que siempre, que es el
// caso de la inmensa mayoría de proyectos.
//
// Mientras el Job corre, el deployment se queda en 'deploying' y el reconciliador
// vuelve a preguntar en el ciclo siguiente: el estado del Job es el estado del
// paso, así que no hace falta guardar nada ni recordar por dónde iba si el
// controlador se reinicia a mitad.
func (c *Controller) releaseDone(ctx context.Context, dep domain.Deployment, spec AppSpec, l logLike) bool {
	if spec.ReleaseCommand == "" {
		return true
	}
	state, err := c.platform.RunRelease(ctx, spec)
	if err != nil {
		// Un fallo al hablar con el clúster no es una migración fallida: no se
		// marca error, se reintenta en el siguiente ciclo.
		l.Warn("paso de release", "error", err)
		return false
	}
	switch state {
	case ReleaseSucceeded:
		return true
	case ReleaseFailed:
		// El motivo detallado está en el log del Job, que el usuario ya puede leer;
		// aquí se deja el titular para que la pantalla diga algo útil sin abrirlo.
		_ = c.store.SetDeploymentStatus(ctx, dep.Tenant, dep.ID, domain.DeployError, "",
			"el comando de release falló; la versión nueva no se ha arrancado")
		l.Warn("paso de release fallido; el despliegue se detiene")
		return false
	default:
		return false // sigue corriendo: se espera al próximo ciclo
	}
}

// markReady fija un deployment como ready. Para producción conmuta atómicamente:
// promociona este deployment y retira las producciones anteriores del proyecto
// (sus objetos los desmonta reconcileTeardown), reusando el MISMO camino que el
// rollback (Promote). Así hay siempre una única producción viva por proyecto.
func (c *Controller) markReady(ctx context.Context, dep domain.Deployment) error {
	if dep.Target != domain.TargetProduction {
		return c.store.SetDeploymentStatus(ctx, dep.Tenant, dep.ID, domain.DeployReady, dep.URL, "")
	}
	tctx, err := c.tenantCtx(ctx, dep.Tenant)
	if err != nil {
		return err
	}
	_, err = c.store.Promote(tctx, dep.ID)
	return err
}

// buildSpec construye el AppSpec de un deployment (imagen ya resuelta, env +
// secretos + credenciales de servicios inyectados y hostnames enrutados). No toca
// el clúster ni el estado del deployment; lo comparten el despliegue normal y la
// reaplicación del estado deseado de producción.
func (c *Controller) buildSpec(ctx context.Context, dep domain.Deployment, proj domain.Project, image, namespace string) (AppSpec, error) {
	env, secrets, err := c.resolveInjection(ctx, dep, proj)
	if err != nil {
		return AppSpec{}, err
	}
	return AppSpec{
		Tenant:       dep.Tenant,
		Namespace:    namespace,
		ProjectSlug:  proj.Slug,
		ProjectID:    proj.ID.String(),
		DeploymentID: dep.ID.String(),
		Target:       dep.Target,
		Image:        image,
		// La escala y el tamaño salen del PROYECTO, no del deployment: son
		// configuración de la app, no propiedades de la build. Así, subir
		// instancias surte efecto en la siguiente reconciliación sin volver a
		// desplegar, y un rollback a un deployment antiguo no revierte la escala.
		Replicas:           proj.EffectiveInstanceCount(),
		AutoscaleMax:       proj.EffectiveAutoscaleMax(),
		AutoscaleCPUTarget: proj.EffectiveAutoscaleCPUTarget(),
		Size:               proj.EffectiveInstanceSize(),
		Port:               proj.EffectiveAppPort(),
		HealthPath:         proj.EffectiveHealthPath(),
		Env:                env,
		Secrets:            secrets,
		Hostnames:          c.hostnamesFor(ctx, dep, proj),
		Labels:             appPodLabels(dep, proj),
		ReleaseCommand:     proj.ReleaseCommand,
		// Solo tiene efecto en previews (el propio KubePlatform lo ignora en
		// producción), pero viaja siempre: filtrar aquí por target repartiría la
		// misma regla en dos sitios.
		PreviewAuthHash: proj.PreviewAuthHash,
		Paused:          proj.Paused,
		HTTPRules:       proj.HTTPRules,
	}, nil
}

// reconcileProduction lleva el tráfico de producción al estado deseado: la
// producción READY más reciente de cada proyecto (por updated_at). (Re)aplica sus
// objetos —imprescindible tras un rollback, que promociona un deployment cuyos
// objetos se habían desmontado— y retira cualquier otra producción ready
// remanente (dedup). Solo mira la ventana de recencia: reaplicar cada producción
// estable en cada ciclo sería un cuello a escala; deploy y Promote/rollback
// bumpean updated_at, así que las conmutaciones caen dentro. Idempotente (SSA).
func (c *Controller) reconcileProduction(ctx context.Context) error {
	rows, err := c.store.ListReadyProductions(ctx, time.Now().Add(-c.retireWindow()), 100)
	if err != nil {
		return err
	}
	// rows viene ordenado por updated_at DESC: la primera aparición de cada
	// proyecto es su producción deseada (la más reciente); las siguientes del
	// mismo proyecto, producciones a retirar. Dedup por proyecto (no por filas
	// consecutivas: el orden es global por recencia, no agrupado por proyecto).
	seen := make(map[uuid.UUID]bool, len(rows))
	for _, pd := range rows {
		if !seen[pd.Deployment.ProjectID] {
			seen[pd.Deployment.ProjectID] = true
			c.applyDesiredProduction(ctx, pd.Deployment, pd.Project)
			continue
		}
		c.retireProduction(ctx, pd.Deployment)
	}
	return nil
}

// applyDesiredProduction (re)aplica los objetos de la producción deseada SIN
// tocar su estado (ya está ready). Sin imagen construida no hay nada que
// reaplicar. Errores no fatales: se reintenta en el siguiente ciclo/evento.
func (c *Controller) applyDesiredProduction(ctx context.Context, dep domain.Deployment, proj domain.Project) {
	l := log.From(ctx).With("deployment_id", dep.ID, "tenant", dep.Tenant, "project", proj.Slug)
	image, ready := c.imageFor(ctx, dep)
	if !ready || image == "" {
		return
	}
	tnt, err := tenancy.New(dep.Tenant)
	if err != nil {
		l.Warn("tenant inválido en producción", "error", err)
		return
	}
	if err := c.platform.EnsureNamespace(ctx, tnt.Namespace(), dep.Tenant, c.tenantQuota(ctx, dep.Tenant)); err != nil {
		l.Warn("asegurar namespace", "error", err)
		return
	}
	spec, err := c.buildSpec(ctx, dep, proj, image, tnt.Namespace())
	if err != nil {
		l.Warn("construir spec de producción", "error", err)
		return
	}
	if _, err := c.platform.ApplyApp(ctx, spec); err != nil {
		l.Warn("reaplicar producción deseada", "error", err)
	}
}

// retireProduction desmonta los objetos de una producción no deseada y la marca
// retired (idempotente). Al dejar de ser 'ready' no vuelve a considerarse deseada.
func (c *Controller) retireProduction(ctx context.Context, dep domain.Deployment) {
	l := log.From(ctx).With("deployment_id", dep.ID, "tenant", dep.Tenant)
	tnt, err := tenancy.New(dep.Tenant)
	if err != nil {
		l.Warn("tenant inválido en producción", "error", err)
		return
	}
	if err := c.platform.RetireApp(ctx, tnt.Namespace(), dep.ID.String()); err != nil {
		l.Warn("retirar producción anterior", "error", err)
		return
	}
	if err := c.store.SetDeploymentStatus(ctx, dep.Tenant, dep.ID, domain.DeployRetired, "", ""); err != nil {
		l.Warn("marcar producción retirada", "error", err)
	}
}

// reconcileTeardown desmonta los objetos (Deployment/Service/Ingress/HPA/Secret)
// de los deployments terminados —canceled por cierre de PR o borrado de proyecto,
// retired por promoción/rollback— para no dejarlos huérfanos en el clúster. Tras
// RetireApp fija torn_down_at (marca-una-vez): el deployment sale del conjunto y
// no se reintenta, así una ráfaga de >LIMIT terminados se drena en varios ciclos
// sin caducar sin desmontarse. Idempotente: RetireApp tolera NotFound.
func (c *Controller) reconcileTeardown(ctx context.Context) error {
	rows, err := c.store.ListRetirableDeployments(ctx, 100)
	if err != nil {
		return err
	}
	for _, r := range rows {
		l := log.From(ctx).With("deployment_id", r.ID, "tenant", r.Tenant, "target", string(r.Target))
		tnt, err := tenancy.New(r.Tenant)
		if err != nil {
			l.Warn("tenant inválido en deployment retirable", "error", err)
			continue
		}
		if err := c.platform.RetireApp(ctx, tnt.Namespace(), r.ID.String()); err != nil {
			l.Warn("retirar deployment", "error", err)
			continue // sin marcar torn_down_at: se reintenta el próximo ciclo
		}
		if err := c.store.MarkDeploymentTornDown(ctx, r.Tenant, r.ID); err != nil {
			l.Warn("marcar deployment desmontado", "error", err)
		}
	}
	return nil
}

// imageFor obtiene la imagen de la build asociada. ready=false si la build aún
// no ha terminado (o falló, en cuyo caso image="" para marcar error).
func (c *Controller) imageFor(ctx context.Context, dep domain.Deployment) (image string, ready bool) {
	if dep.ImageRef != "" {
		return dep.ImageRef, true
	}
	if dep.BuildID == nil {
		return "", true
	}
	tctx, err := c.tenantCtx(ctx, dep.Tenant)
	if err != nil {
		return "", false
	}
	b, err := c.store.GetBuild(tctx, *dep.BuildID)
	if err != nil {
		return "", false
	}
	switch b.Status {
	case domain.BuildSucceeded:
		return b.ImageRef, true
	case domain.BuildFailed, domain.BuildCanceled:
		return "", true // terminal sin imagen -> error de deployment
	default:
		return "", false // queued/running -> esperar
	}
}

// resolveInjection construye los mapas de env (no sensible) y secrets (sensible)
// a inyectar: variables del proyecto según target + credenciales de los servicios
// gestionados ready del proyecto (descifradas en memoria).
func (c *Controller) resolveInjection(ctx context.Context, dep domain.Deployment, proj domain.Project) (env, secrets map[string]string, err error) {
	env = map[string]string{}
	secrets = map[string]string{}

	vars, err := c.store.ResolveEnvForDeploy(ctx, dep.Tenant, proj.ID, dep.Target)
	if err != nil {
		return nil, nil, err
	}
	// vars llega con lo compartido primero y lo propio del destino después, así
	// que el último valor de cada clave es el que manda. Y cada clave vive en UN
	// solo mapa: `env` se escribe en línea en el contenedor y gana a `envFrom`, de
	// modo que un valor compartido en claro taparía al secreto propio de la vista
	// previa si se dejaran los dos.
	for _, v := range vars {
		if v.IsSecret {
			plain, derr := c.sealer.OpenString(v.ValueEnc)
			if derr != nil {
				return nil, nil, derr
			}
			secrets[v.Key] = plain
			delete(env, v.Key)
		} else {
			env[v.Key] = v.Value
			delete(secrets, v.Key)
		}
	}

	// Credenciales de servicios gestionados (DSN de Sytue Postgres, claves S3,
	// API key de auth, etc.) se inyectan como secretos con un prefijo por tipo.
	// Solo las de los servicios del MISMO destino que el despliegue: una vista
	// previa no recibe la base de datos de producción (CLOUD-01).
	creds, err := c.store.ResolveServiceCreds(ctx, dep.Tenant, proj.ID, dep.Target)
	if err != nil {
		return nil, nil, err
	}
	for kind, enc := range creds {
		plain, derr := c.sealer.Open(enc)
		if derr != nil {
			return nil, nil, derr
		}
		injectServiceCreds(secrets, kind, plain)
	}

	// Metadatos estándar disponibles para la app.
	env["SYTUE_PROJECT"] = proj.Slug
	env["SYTUE_DEPLOYMENT_TARGET"] = string(dep.Target)
	// PORT es la convención con la que la plataforma le DICE a la app dónde
	// escuchar (12-factor, igual que Heroku o DigitalOcean). Tiene que ser el
	// puerto configurado, no una constante: una app que respete $PORT —la mayoría
	// de las de Node— escucharía en 8080 mientras las sondas y el Service apuntan
	// al puerto elegido, y moriría en bucle sin decir por qué.
	env["PORT"] = strconv.Itoa(proj.EffectiveAppPort())
	return env, secrets, nil
}

// previewNotice es el aviso que acompaña a una vista previa mientras arranca: qué
// secretos y qué servicios de producción NO ha recibido y no tienen equivalente
// propio. Vacío en producción y cuando no falta nada.
//
// Solo lleva NOMBRES de clave y tipos de servicio, nunca valores. Y un fallo al
// calcularlo no detiene el despliegue: el aviso es una ayuda, no una condición.
func (c *Controller) previewNotice(ctx context.Context, dep domain.Deployment, proj domain.Project) string {
	if dep.Target != domain.TargetPreview {
		return ""
	}
	w, err := c.store.PreviewWithheld(ctx, dep.Tenant, proj.ID)
	if err != nil {
		log.From(ctx).Warn("calcular lo que la vista previa no recibe", "error", err, "deployment_id", dep.ID)
		return ""
	}
	return domain.PreviewWithheldNotice(w.SecretKeys, w.ServiceKinds)
}

// injectServiceCreds vuelca las credenciales de un servicio gestionado en el mapa
// de secretos con claves convencionales (p. ej. DATABASE_URL para postgres).
func injectServiceCreds(secrets map[string]string, kind string, raw []byte) {
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		// Si no es JSON estructurado, se inyecta como una sola variable.
		secrets["SYTUE_"+normalizeKey(kind)+"_CREDENTIALS"] = string(raw)
		return
	}
	for k, v := range m {
		secrets[k] = v
	}
}

func normalizeKey(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z':
			out = append(out, ch-('a'-'A'))
		case ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			out = append(out, ch)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// hostnamesFor devuelve los hosts que enruta el Ingress: la URL canónica del
// deployment más los dominios propios activos (solo para producción).
func (c *Controller) hostnamesFor(ctx context.Context, dep domain.Deployment, proj domain.Project) []string {
	base := stripScheme(dep.URL)
	if dep.Target != domain.TargetProduction {
		return []string{base}
	}
	tctx, err := c.tenantCtx(ctx, dep.Tenant)
	if err != nil {
		return []string{base}
	}
	doms, err := c.store.ListDomains(tctx, proj.ID)
	if err != nil {
		return []string{base}
	}
	var primary []string
	var rest []string
	for _, d := range doms {
		if d.Status != domain.DomainActive {
			continue
		}
		if d.IsPrimary {
			primary = append(primary, d.Hostname)
		} else {
			rest = append(rest, d.Hostname)
		}
	}
	hosts := make([]string, 0, 1+len(primary)+len(rest))
	hosts = append(hosts, base)
	hosts = append(hosts, primary...)
	hosts = append(hosts, rest...)
	return hosts
}

// tenantCtx deriva un contexto con el tenant fijado para llamadas tenant-scoped
// del store desde el controller (cross-tenant por naturaleza).
func (c *Controller) tenantCtx(ctx context.Context, tenant string) (context.Context, error) {
	t, err := tenancy.New(tenant)
	if err != nil {
		return nil, err
	}
	return tenancy.Into(ctx, t), nil
}

func stripScheme(u string) string {
	for _, p := range []string{"https://", "http://"} {
		if len(u) >= len(p) && u[:len(p)] == p {
			return u[len(p):]
		}
	}
	return u
}

// appPodLabels son las etiquetas de los pods de una app de cliente.
//
// `sytue.eu/project-id` NO es decorativa ni redundante con el slug: es la etiqueta
// por la que allow-project-<id> (ensureProjectNetworkPolicy) empareja la app con
// LOS SERVICIOS GESTIONADOS DE SU PROYECTO. Sin ella la política no selecciona
// estos pods, así que no reciben el egress hacia el 5432/3000 y la base de datos
// del proyecto queda inalcanzable — justo el fallo que esa política venía a cerrar.
// El slug no sirve para emparejar: el aprovisionador de servicios solo conoce el
// ProjectID, de modo que el UUID es el único identificador que ambos lados tienen.
//
// Está en su propia función para que el test pueda comprobar el emparejamiento de
// verdad, evaluando el selector de la política contra estas etiquetas.
func appPodLabels(dep domain.Deployment, proj domain.Project) map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of": "sytue-one",
		"sytue.eu/component":        "cloud-app",
		"sytue.eu/tenant":           dep.Tenant,
		"sytue.eu/project":          proj.Slug,
		"sytue.eu/target":           string(dep.Target),
		projectLabel:                proj.ID.String(),
	}
}
