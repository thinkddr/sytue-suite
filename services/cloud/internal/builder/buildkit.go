// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
)

// JobSpec es la especificación declarativa de un Job de Kaniko (independiente
// del cliente de Kubernetes). El KubeRunner la traduce a un batchv1.Job; el
// StubRunner la registra para tests/dev. Kaniko corre rootless en Kapsule con
// securityContext restrictivo (CONVENTIONS §0.7), cachea capas y empuja al
// Container Registry de Scaleway.
type JobSpec struct {
	Name        string
	Namespace   string
	Image       string // moby/buildkit:vX-rootless (Apache-2.0)
	Args        []string
	Labels      map[string]string
	Annotations map[string]string
	// RegistrySecret es el nombre del Secret de Kubernetes con las credenciales
	// de PUSH del Container Registry. Seguridad (registry-cred): NUNCA se monta en
	// el contenedor de build que ejecuta código de tenant no confiable (RUN del
	// Dockerfile / scripts de buildpack) — solo en el contenedor de PUSH (crane /
	// exporter), que no ejecuta código de tenant. Así un RUN malicioso no puede leer
	// la credencial y empujar imágenes a repos de OTROS tenants (shared registry).
	RegistrySecret string
	// Destination es la referencia OCI destino (rg.../tenant/app:tag). El build
	// (Kaniko --no-push / lifecycle) la nombra en el artefacto local; el contenedor
	// de push la usa como destino real del push con la credencial.
	Destination string
	// DockerfileContent, si no es vacío, es el Dockerfile generado por Sytue Cloud
	// (frameworks gestionados: astro/node/go/static) que el runner materializa en
	// /workspace/Dockerfile.sytue vía ConfigMap antes de Kaniko. Vacío para
	// 'docker' (usa el Dockerfile del repo) y 'auto' (detección en cluster).
	DockerfileContent string
	// GitSSHKeyContent monta una deploy key en /secret/git/id_rsa (repos privados).
	GitSSHKeyContent string
	// GitToken es un installation token efímero de la GitHub App. Si no es vacío,
	// solo el initContainer de clonado lo lee desde un Secret — NUNCA aparece en
	// args, URL ni logs, y el contenedor que ejecuta código del tenant no lo monta.
	GitToken string

	// Buildpacks selecciona la forma de Job del lifecycle de CNB (buildpacksJob)
	// en vez de Kaniko (buildJob). Con Buildpacks, Image es el builder de Paketo,
	// GitImage la imagen del initContainer de clonado, y RepoURL/GitRef alimentan
	// el clonado (el lifecycle no clona; opera local). AppDir es el directorio de
	// la app en /workspace (respeta el root_dir del monorepo) y RunImage la imagen
	// base de ejecución (vacío ⇒ se deriva del builder).
	Buildpacks bool
	GitImage   string
	RepoURL    string
	GitRef     string
	AppDir     string
	RunImage   string
}

// JobClient programa y espera Jobs de Kaniko en Kapsule. La implementación real
// (KubeRunner) usa client-go; aquí definimos la interfaz para desacoplar.
type JobClient interface {
	// Submit crea el Job y devuelve su nombre.
	Submit(ctx context.Context, spec JobSpec) (string, error)
	// Wait bloquea hasta que el Job termina y devuelve éxito + logs.
	Wait(ctx context.Context, namespace, name string, isCanceled func() bool) (succeeded bool, logs []byte, err error)
	// Cleanup elimina el Job y sus Pods.
	Cleanup(ctx context.Context, namespace, name string) error
}

// LogSink persiste los logs de una build (Object Storage S3, build-cache bucket).
type LogSink interface {
	PutBuildLog(ctx context.Context, tenant, buildID string, logs []byte) (key string, err error)
}

// KubeRunner ejecuta builds de Kaniko vía un JobClient de Kapsule.
type KubeRunner struct {
	cfg  config.Config
	jobs JobClient
	logs LogSink
}

// NewKubeRunner construye el runner sobre Kapsule.
func NewKubeRunner(cfg config.Config, jobs JobClient, logs LogSink) *KubeRunner {
	return &KubeRunner{cfg: cfg, jobs: jobs, logs: logs}
}

// Run programa un Job de Kaniko, espera su fin y persiste el log. Implementa
// builder.Runner.
//
//nolint:contextcheck // la limpieza en defer usa context.Background A PROPÓSITO: el ctx de la build puede estar ya cancelado.
func (k *KubeRunner) Run(ctx context.Context, req KanikoRequest) KanikoResult {
	l := log.From(ctx).With("component", "kaniko", "build_id", req.BuildID)
	spec := k.specFor(req)

	name, err := k.jobs.Submit(ctx, spec)
	if err != nil {
		return KanikoResult{Err: fmt.Errorf("programar Job Kaniko: %w", err)}
	}
	if req.OnJobStarted != nil {
		req.OnJobStarted(name)
	}
	defer func() {
		// Limpieza best-effort del Job tras finalizar: usa un contexto propio a
		// propósito, porque el ctx de la petición puede estar ya cancelado.
		if cerr := k.jobs.Cleanup(context.Background(), spec.Namespace, name); cerr != nil {
			l.Warn("no se pudo limpiar el Job de build", "error", cerr)
		}
	}()

	ok, logs, err := k.jobs.Wait(ctx, spec.Namespace, name, req.IsCanceled)
	logsKey := ""
	if len(logs) > 0 && k.logs != nil {
		if key, lerr := k.logs.PutBuildLog(ctx, req.Tenant, req.BuildID.String(), logs); lerr != nil {
			l.Warn("no se pudo guardar el log de build", "error", lerr)
		} else {
			logsKey = key
		}
	}
	if err != nil {
		return KanikoResult{LogsKey: logsKey, Err: fmt.Errorf("esperar Job Kaniko: %w", err)}
	}
	if !ok {
		if req.IsCanceled != nil && req.IsCanceled() {
			return KanikoResult{LogsKey: logsKey, JobName: name, Err: fmt.Errorf("build cancelada")}
		}
		return KanikoResult{LogsKey: logsKey, JobName: name, Err: fmt.Errorf("la build de Kaniko falló")}
	}

	imageRef := imageRefWithDigest(req.Destination, logs, name)
	return KanikoResult{ImageRef: imageRef, LogsKey: logsKey, JobName: name}
}

// specFor construye el JobSpec a partir de la solicitud de build. Kaniko usa la
// CLI del executor (Apache-2.0); buildpacks, la del lifecycle de CNB (Apache-2.0).
func (k *KubeRunner) specFor(req KanikoRequest) JobSpec {
	ref := refOrCommit(req)
	name := "build-" + req.BuildID.String()[:8] + "-" + fmt.Sprint(time.Now().Unix())
	spec := JobSpec{ //nolint:gosec // G101: "sytue-registry" es el nombre de un Secret de k8s (imagePullSecret), no una credencial embebida.
		Name:      name,
		Namespace: k.cfg.BuildNamespace,
		// Image vacío A PROPÓSITO en Kaniko: el runner usa firstNonEmpty(spec.Image,
		// c.kanikoImage), así la KanikoImage de config (KANIKO_IMAGE, pinneable por
		// digest en prod) manda. Fijar aquí ":latest" la anulaba siempre.
		RegistrySecret:   "sytue-registry",
		Destination:      req.Destination,
		GitSSHKeyContent: req.GitSSHKey,
		GitToken:         req.GitHTTPSToken,
		GitImage:         k.cfg.CNBGitImage,
		RepoURL:          req.RepoURL,
		GitRef:           ref,
		Labels: map[string]string{
			"app.kubernetes.io/part-of": "sytue-one",
			"sytue.eu/component":        "cloud-build",
			"sytue.eu/tenant":           req.Tenant,
		},
	}
	if req.Buildpacks {
		// CNB: builder de Paketo + initContainer de clonado. Las fases no confiables
		// (detector+builder) corren sin credencial; el export (con cred) va aparte.
		spec.Buildpacks = true
		spec.Image = k.cfg.CNBBuilderImage
		spec.AppDir = cnbAppDir(req.ContextSub)
		spec.RunImage = k.cfg.CNBRunImage
		return spec
	}
	spec.Args = buildctlArgs(req, ref)
	spec.DockerfileContent = req.DockerfileContent
	return spec
}

// cnbAppDir es el directorio de la app dentro de /workspace sobre el que opera el
// lifecycle de CNB: la raíz del contexto clonado o, en monorepos, el root_dir
// (ContextSub). El buildpack auto-detecta el lenguaje sobre él.
func cnbAppDir(contextSub string) string {
	if sub := strings.Trim(contextSub, "/"); sub != "" {
		return "/workspace/" + sub
	}
	return "/workspace"
}

// kanikoTarPath es donde el build escribe la imagen como tarball en el emptyDir
// /workspace (compartido con el contenedor de push). Al escribir a fichero en vez
// de al registry, el contenedor que ejecuta el RUN no confiable NO necesita —ni
// monta— la credencial del registry; la empuja después `crane`. Cierra el
// registry-cred cross-tenant en el camino por defecto.
const kanikoTarPath = "/workspace/image.sytue.tar"

// buildctlArgs traduce la solicitud a la CLI de BuildKit.
//
// Se conserva la propiedad de seguridad del montaje anterior, que es la que de
// verdad importa: la salida va a un TARBALL en el emptyDir, no al registry, así
// que el contenedor que ejecuta el RUN no confiable del Dockerfile NO monta la
// credencial del registry. El push lo hace después `crane`, en un contenedor
// aparte que sí la tiene. Un RUN malicioso no puede leerla ni empujar nada.
func buildctlArgs(req KanikoRequest, ref string) []string {
	// El repositorio ya fue clonado por un initContainer aislado que sí puede leer
	// la credencial git y verifica la host key SSH. El build solo recibe el árbol
	// local: ni deploy keys ni tokens llegan al proceso que ejecuta el Dockerfile.
	_ = ref
	// El subdirectorio de un monorepo se resuelve moviendo el CONTEXTO, que es
	// como lo expresa BuildKit; Kaniko tenía una opción propia para lo mismo.
	contexto := "/workspace"
	if sub := strings.Trim(req.ContextSub, "/"); sub != "" {
		contexto = "/workspace/" + sub
	}
	args := make([]string, 0, 8+len(req.BuildArgs))
	args = append(args,
		"build",
		"--frontend=dockerfile.v0",
		"--local=context="+contexto,
		"--local=dockerfile="+contexto,
		"--opt=filename="+req.Dockerfile,
		// type=docker + dest: la imagen sale como tarball al emptyDir compartido.
		// Equivale al --no-push --tar-path de Kaniko y es lo que mantiene la
		// credencial del registry fuera de este contenedor.
		"--output=type=docker,name="+req.Destination+",dest="+kanikoTarPath+
			",rewrite-timestamp=true",
		// Reproducibilidad: sin esto cada build daría un digest distinto para las
		// mismas entradas. rewrite-timestamp (arriba) reescribe las marcas de las
		// capas a este mismo valor.
		"--opt=build-arg:SOURCE_DATE_EPOCH=0",
		// Sin caché entre tenants: compartir capas entre builds de clientes
		// distintos filtraría contenido de unos a otros.
		"--export-cache=type=inline",
	)
	for kk, vv := range req.BuildArgs {
		args = append(args, fmt.Sprintf("--opt=build-arg:%s=%s", kk, vv))
	}
	return args
}

func sshRepoURL(raw string) string {
	if strings.HasPrefix(raw, "git@") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	path := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
	if strings.EqualFold(u.Hostname(), "github.com") {
		return "git@" + u.Host + ":" + path + ".git"
	}
	// Los hosts SSH adicionales se admiten solo cuando ya vienen en forma
	// git@host:path y su host key dispone de una entrada revisada específica.
	return raw
}

func refOrCommit(req KanikoRequest) string {
	if req.CommitSHA != "" {
		return req.CommitSHA
	}
	return branchRef(req.GitRef)
}

func branchRef(ref string) string {
	const prefix = "refs/heads/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	if ref == "" {
		return "main"
	}
	return ref
}

// imageRefWithDigest combina destino:tag con digest sha256 si está en logs.
func imageRefWithDigest(destination string, logs []byte, _ string) string {
	digest := extractDigestFromLogs(logs)
	if digest == "" {
		return destination
	}
	base := destination
	if i := strings.LastIndex(base, "@"); i > 0 {
		base = base[:i]
	}
	if i := strings.LastIndex(base, ":"); i > strings.LastIndex(base, "/") {
		base = base[:i]
	}
	return base + "@" + digest
}

func extractDigestFromLogs(logs []byte) string {
	s := string(logs)
	if i := strings.LastIndex(s, "__digest__="); i >= 0 {
		return extractDigest(s[i+len("__digest__="):])
	}
	return extractDigest(s)
}
