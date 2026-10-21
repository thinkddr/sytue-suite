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

const fakeInstallToken = "ghs_supersecrettoken_do_not_leak" //nolint:gosec // token de prueba, no una credencial real.

// TestKanikoArgsHTTPSTokenNoLeak verifica que Kaniko siempre construye desde el
// contexto local ya clonado y no recibe credenciales ni URLs remotas.
func TestKanikoArgsHTTPSTokenNoLeak(t *testing.T) {
	req := KanikoRequest{
		RepoURL:       "https://github.com/octo/repo.git",
		GitHTTPSToken: fakeInstallToken,
		GitSSHKey:     "SHOULD-BE-IGNORED-WHEN-TOKEN-PRESENT",
		Dockerfile:    "Dockerfile",
		Destination:   "rg/x:tag",
	}
	args := buildctlArgs(req, "main")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--local=context=/workspace") {
		t.Fatalf("el build debe partir del contexto local: %v", args)
	}
	for _, a := range args {
		if strings.HasPrefix(a, "--git=") || strings.Contains(a, "github.com") {
			t.Fatalf("Kaniko no debe clonar ni recibir la URL del repositorio: %q", a)
		}
	}
	if strings.Contains(joined, fakeInstallToken) {
		t.Fatalf("el token NO debe aparecer en los args de Kaniko: %v", args)
	}
}

// TestBuildJobMountsTokenAsEnvFromSecret verifica que solo git-clone recibe el
// token por SecretKeyRef y que Kaniko no hereda la credencial.
func TestBuildJobMountsTokenAsEnvFromSecret(t *testing.T) {
	c := newKubeJobClientFor(fake.NewSimpleClientset(), KubeJobConfig{BuildkitImage: "kaniko:test"})
	job := c.buildJob(JobSpec{
		Name:      "build-tok",
		Namespace: "sytue-builds",
		Args:      []string{"--local=context=/workspace", "--destination=rg/x:tag"},
		GitToken:  fakeInstallToken,
		GitImage:  "git:test",
		RepoURL:   "https://github.com/octo/repo.git",
		GitRef:    "main",
	})
	clone := containerByName(job.Spec.Template.Spec.InitContainers, "git-clone")
	build := containerByName(job.Spec.Template.Spec.InitContainers, "build")
	if clone == nil || build == nil {
		t.Fatal("faltan los initContainers git-clone/build")
	}

	var token *corev1.EnvVar
	for i := range clone.Env {
		if clone.Env[i].Name == "GIT_TOKEN" {
			token = &clone.Env[i]
		}
		// Ninguna variable debe llevar el token como valor literal en el spec.
		if strings.Contains(clone.Env[i].Value, fakeInstallToken) {
			t.Fatalf("el token no debe estar en claro en env %q", clone.Env[i].Name)
		}
	}
	if token == nil {
		t.Fatalf("falta GIT_TOKEN en el clonador: %+v", clone.Env)
	}
	if token.ValueFrom == nil || token.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("GIT_TOKEN debe venir de un Secret (SecretKeyRef), no de un valor literal")
	}
	if token.ValueFrom.SecretKeyRef.Name != gitTokenSecretName("build-tok") || token.ValueFrom.SecretKeyRef.Key != "password" {
		t.Fatalf("SecretKeyRef de GIT_TOKEN incorrecto: %+v", token.ValueFrom.SecretKeyRef)
	}
	// El contenedor de build lleva su propia configuración (BUILDKITD_FLAGS,
	// HOME…), pero NINGUNA credencial: ni el token git ni la clave SSH. Se
	// comprueba por nombre en vez de exigir env vacío, que era lo que valía
	// cuando Kaniko no necesitaba configuración alguna.
	for _, e := range build.Env {
		switch e.Name {
		case "GIT_TOKEN", "GIT_SSH_KEY", "GIT_USERNAME", "GIT_PASSWORD":
			t.Fatalf("el build no debe recibir credenciales git: %+v", e)
		}
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			t.Fatalf("el build no debe leer ningún Secret: %+v", e)
		}
	}
	if mountByName(build.VolumeMounts, "git-ssh") != nil {
		t.Fatalf("el build no debe montar la clave SSH: %+v", build.VolumeMounts)
	}
}

// TestSubmitCreatesGitTokenSecret verifica que Submit materializa el Secret con
// la credencial HTTPS efímera (x-access-token:<token>) y un OwnerReference al Job
// para su recolección en cascada.
func TestSubmitCreatesGitTokenSecret(t *testing.T) {
	cs := fake.NewSimpleClientset()
	c := newKubeJobClientFor(cs, KubeJobConfig{})
	ctx := context.Background()

	name, err := c.Submit(ctx, JobSpec{Name: "build-tok", Namespace: "ns", GitToken: fakeInstallToken})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	sec, err := cs.CoreV1().Secrets("ns").Get(ctx, gitTokenSecretName(name), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("el Secret del token no se creó: %v", err)
	}
	if sec.StringData["username"] != "x-access-token" || sec.StringData["password"] != fakeInstallToken {
		t.Fatalf("credencial del Secret incorrecta: %+v", sec.StringData)
	}
	if len(sec.OwnerReferences) == 0 || sec.OwnerReferences[0].Name != name || sec.OwnerReferences[0].Kind != "Job" {
		t.Fatalf("el Secret debe tener OwnerReference al Job para GC: %+v", sec.OwnerReferences)
	}

	// Sin token: no se crea el Secret.
	if _, err := c.Submit(ctx, JobSpec{Name: "build-plain", Namespace: "ns"}); err != nil {
		t.Fatalf("Submit sin token: %v", err)
	}
	if _, err := cs.CoreV1().Secrets("ns").Get(ctx, gitTokenSecretName("build-plain"), metav1.GetOptions{}); err == nil {
		t.Fatalf("no debe crearse Secret de token cuando no hay GitToken")
	}
}
