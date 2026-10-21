// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// buildpacksSpec es un JobSpec de estrategia buildpacks para tests.
func buildpacksSpec() JobSpec {
	return JobSpec{
		Name:           "build-cnb1",
		Namespace:      "sytue-builds",
		Image:          "paketobuildpacks/builder-jammy-base:test",
		GitImage:       "alpine/git:test",
		Destination:    "rg.fr-par.scw.cloud/sytue/app:abc123",
		AppDir:         "/workspace",
		RegistrySecret: "sytue-registry",
		Buildpacks:     true,
		RepoURL:        "https://github.com/acme/app",
		GitRef:         "abc123",
		Labels:         map[string]string{"sytue.eu/tenant": "acme"},
	}
}

func containerByName(cs []corev1.Container, name string) *corev1.Container {
	for i := range cs {
		if cs[i].Name == name {
			return &cs[i]
		}
	}
	return nil
}

// TestBuildpacksJobShape verifica la forma del Job de CNB partido en fases: la
// fase de build no confiable (detector+builder) en un initContainer SIN la
// credencial del registry, y el export (analyzer+exporter, con credencial) en el
// contenedor principal — más el initContainer de clonado. Sin Dockerfile.
func TestBuildpacksJobShape(t *testing.T) {
	c := newKubeJobClientFor(fake.NewSimpleClientset(), KubeJobConfig{BuildkitImage: "kaniko:test"})
	spec := buildpacksSpec()
	spec.RunImage = "paketobuildpacks/run-jammy-base:test"
	job := c.buildpacksJob(spec)

	// Fase de build (initContainer): detector + builder, imagen builder de Paketo.
	build := containerByName(job.Spec.Template.Spec.InitContainers, "cnb-build")
	if build == nil {
		t.Fatal("falta el initContainer 'cnb-build' (fase de build no confiable)")
	}
	if build.Image != "paketobuildpacks/builder-jammy-base:test" {
		t.Errorf("imagen del builder incorrecta: %s", build.Image)
	}
	if len(build.Command) < 3 || !strings.Contains(build.Command[2], "/cnb/lifecycle/detector") ||
		!strings.Contains(build.Command[2], "/cnb/lifecycle/builder") {
		t.Errorf("command de la fase build inesperado: %v", build.Command)
	}
	if !hasEnv(build.Env, "APP", "/workspace") {
		t.Errorf("falta APP=/workspace en cnb-build: %v", build.Env)
	}
	// registry-cred: la fase de build no confiable NO monta la credencial del registry.
	if mountByName(build.VolumeMounts, "docker-config") != nil {
		t.Error("cnb-build (código de buildpack no confiable) NO debe montar la credencial del registry")
	}

	// Fase de export (contenedor principal): analyzer + exporter, con la credencial.
	export := containerByName(job.Spec.Template.Spec.Containers, "cnb-export")
	if export == nil {
		t.Fatal("falta el contenedor 'cnb-export' (fase de export con credencial)")
	}
	if len(export.Command) < 3 || !strings.Contains(export.Command[2], "/cnb/lifecycle/exporter") ||
		!strings.Contains(export.Command[2], "termination-log") {
		t.Errorf("command de la fase export inesperado: %v", export.Command)
	}
	if !hasEnv(export.Env, "DEST", "rg.fr-par.scw.cloud/sytue/app:abc123") ||
		!hasEnv(export.Env, "RUN_IMAGE", "paketobuildpacks/run-jammy-base:test") ||
		!hasEnv(export.Env, "DOCKER_CONFIG", cnbDockerConfigDir) {
		t.Errorf("env del export inesperado: %v", export.Env)
	}
	// Solo el export monta la credencial del registry.
	if m := mountByName(export.VolumeMounts, "docker-config"); m == nil || m.MountPath != cnbDockerConfigDir || !m.ReadOnly {
		t.Errorf("cnb-export debe montar la credencial en %s (ro): %+v", cnbDockerConfigDir, export.VolumeMounts)
	}

	// initContainer que clona el contexto git en /workspace, ANTES de la fase build.
	git := containerByName(job.Spec.Template.Spec.InitContainers, "git-clone")
	if git == nil {
		t.Fatal("falta el initContainer 'git-clone' (git context)")
	}
	if git.Image != "alpine/git:test" {
		t.Errorf("imagen de git incorrecta: %s", git.Image)
	}
	if len(git.Command) < 3 || !strings.Contains(git.Command[2], "git init") ||
		!strings.Contains(git.Command[2], "fetch") {
		t.Errorf("command de clonado inesperado: %v", git.Command)
	}
	if !hasEnv(git.Env, "GIT_REF", "abc123") || !hasEnv(git.Env, "HTTPS_REPO", "https://github.com/acme/app") {
		t.Errorf("env del clonado inesperado: %v", git.Env)
	}
	// Orden de initContainers: clonado ANTES de la fase de build.
	if ic := job.Spec.Template.Spec.InitContainers; len(ic) != 2 || ic[0].Name != "git-clone" || ic[1].Name != "cnb-build" {
		t.Errorf("orden de initContainers inesperado: %v", initNames(ic))
	}

	// Secret del Registry montado como volumen docker-config.
	var regMounted bool
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "docker-config" && v.Secret != nil && v.Secret.SecretName == "sytue-registry" {
			regMounted = true
		}
	}
	if !regMounted {
		t.Error("el Secret del Registry no se montó en docker-config")
	}

	// Sin Dockerfile: no debe existir el volumen del Dockerfile generado.
	if hasVolume(job, "dockerfile") {
		t.Error("buildpacks no debe montar Dockerfile")
	}

	// SC restrictivo rootless en las tres fases y el Pod; sin token de SA.
	assertRestrictedRootless(t, build.SecurityContext)
	assertRestrictedRootless(t, export.SecurityContext)
	assertRestrictedRootless(t, git.SecurityContext)
	psc := job.Spec.Template.Spec.SecurityContext
	if psc == nil || psc.RunAsNonRoot == nil || !*psc.RunAsNonRoot {
		t.Error("el Pod debe correr rootless (RunAsNonRoot)")
	}
	if sa := job.Spec.Template.Spec.AutomountServiceAccountToken; sa == nil || *sa {
		t.Error("no debe montar el token de ServiceAccount")
	}
	if l := job.Labels["sytue.eu/tenant"]; l != "acme" {
		t.Errorf("label de tenant no propagado: %q", l)
	}
}

func initNames(cs []corev1.Container) []string {
	out := make([]string, len(cs))
	for i := range cs {
		out[i] = cs[i].Name
	}
	return out
}

// TestBuildpacksJobGitCreds verifica que las credenciales git (deploy-key SSH /
// token HTTPS efímero) se cablean en el initContainer sin exponerse en args/URL.
func TestBuildpacksJobGitCreds(t *testing.T) {
	c := newKubeJobClientFor(fake.NewSimpleClientset(), KubeJobConfig{})

	// Deploy-key SSH: volumen git-ssh + montaje de solo lectura.
	spec := buildpacksSpec()
	spec.GitSSHKeyContent = "PRIVATE-KEY"
	job := c.buildpacksJob(spec)
	if !hasVolume(job, "git-ssh") {
		t.Error("con deploy-key debe montarse el volumen git-ssh")
	}
	git := containerByName(job.Spec.Template.Spec.InitContainers, "git-clone")
	if m := mountByName(git.VolumeMounts, "git-ssh"); m == nil || m.MountPath != "/secret/git" || !m.ReadOnly {
		t.Errorf("montaje de la deploy-key incorrecto: %+v", git.VolumeMounts)
	}
	if strings.Contains(gitCloneScript, "StrictHostKeyChecking=no") ||
		!strings.Contains(gitCloneScript, "StrictHostKeyChecking=yes") ||
		!strings.Contains(gitCloneScript, "UserKnownHostsFile=/secret/git/known_hosts") {
		t.Error("el clone SSH debe verificar la clave de host fijada")
	}

	// Token HTTPS de la GitHub App: env GIT_TOKEN desde Secret (nunca en args/URL).
	spec = buildpacksSpec()
	spec.GitToken = "ghs_x"
	job = c.buildpacksJob(spec)
	git = containerByName(job.Spec.Template.Spec.InitContainers, "git-clone")
	var tokenRef bool
	for _, e := range git.Env {
		if e.Name == "GIT_TOKEN" {
			if e.Value != "" {
				t.Error("GIT_TOKEN no debe ir en claro; debe venir de un SecretKeyRef")
			}
			if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil && e.ValueFrom.SecretKeyRef.Key == "password" {
				tokenRef = true
			}
		}
	}
	if !tokenRef {
		t.Errorf("GIT_TOKEN debe referenciar el Secret del token (key password): %v", git.Env)
	}
	if got := strings.Join(git.Command, " ") + strings.Join(git.Args, " "); strings.Contains(got, "ghs_x") {
		t.Error("el token no debe aparecer en command/args del clonado")
	}
}

// TestSubmitBuildpacksSelectsJob verifica que Submit programa el Job de CNB
// (initContainer de clonado) y NO materializa ConfigMap de Dockerfile, pero SÍ
// crea el Secret del token git cuando procede.
func TestSubmitBuildpacksSelectsJob(t *testing.T) {
	cs := fake.NewSimpleClientset()
	c := newKubeJobClientFor(cs, KubeJobConfig{})
	ctx := context.Background()

	spec := buildpacksSpec()
	spec.GitToken = "ghs_x"
	name, err := c.Submit(ctx, spec)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	job, err := cs.BatchV1().Jobs(spec.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("el Job no se creó: %v", err)
	}
	if len(job.Spec.Template.Spec.InitContainers) == 0 {
		t.Fatal("el Job de buildpacks debe tener el initContainer de clonado")
	}
	// No debe crearse ConfigMap de Dockerfile para buildpacks.
	if _, err := cs.CoreV1().ConfigMaps(spec.Namespace).Get(ctx, configMapNameFor(name), metav1.GetOptions{}); err == nil {
		t.Error("buildpacks no debe materializar ConfigMap de Dockerfile")
	}
	// Sí debe crearse el Secret del token git (lo usa el initContainer).
	if _, err := cs.CoreV1().Secrets(spec.Namespace).Get(ctx, gitTokenSecretName(name), metav1.GetOptions{}); err != nil {
		t.Errorf("debe crearse el Secret del token git: %v", err)
	}
}

// TestCNBAppDir verifica que el directorio -app del lifecycle respeta el root_dir
// del monorepo (ContextSub) y cae a /workspace cuando no hay subdirectorio.
func TestCNBAppDir(t *testing.T) {
	if got := cnbAppDir("apps/web/"); got != "/workspace/apps/web" {
		t.Errorf("-app debe respetar root_dir: %q", got)
	}
	if got := cnbAppDir(""); got != "/workspace" {
		t.Errorf("sin root_dir, -app debe ser /workspace: %q", got)
	}
	if got := cnbAppDir("/"); got != "/workspace" {
		t.Errorf("root_dir '/' debe normalizar a /workspace: %q", got)
	}
}

func hasEnv(env []corev1.EnvVar, name, value string) bool {
	for _, e := range env {
		if e.Name == name && e.Value == value {
			return true
		}
	}
	return false
}

func mountByName(ms []corev1.VolumeMount, name string) *corev1.VolumeMount {
	for i := range ms {
		if ms[i].Name == name {
			return &ms[i]
		}
	}
	return nil
}

func assertRestrictedRootless(t *testing.T, sc *corev1.SecurityContext) {
	t.Helper()
	if sc == nil {
		t.Fatal("securityContext ausente")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("debe correr rootless (RunAsNonRoot)")
	}
	if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		t.Error("rootfs debe ser de solo lectura")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("no debe permitir escalada de privilegios")
	}
	if sc.Capabilities == nil || len(sc.Capabilities.Drop) == 0 || sc.Capabilities.Drop[0] != "ALL" {
		t.Error("debe eliminar todas las capabilities")
	}
}
