// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Estrategia de build alternativa a Dockerfile+Kaniko: Cloud Native Buildpacks
// (CNB). El Job corre el lifecycle de CNB (binario `creator`) con un builder de
// Paketo (Apache-2.0), que AUTO-DETECTA el lenguaje y produce la imagen OCI SIN
// Dockerfile (paridad DigitalOcean App Platform / Heroku / Coolify Nixpacks). No
// se usa kpack (controller pesado): es un Job directo, reusando el patrón del Job
// de Kaniko (mismo Secret del Registry, SC restrictivo rootless, logs, GC por TTL).
//
// El lifecycle NO clona git: opera sobre un directorio local. Un initContainer
// con git prepara el contexto en /workspace (emptyDir compartido) y luego el
// contenedor `creator` construye y empuja al Container Registry con las mismas
// credenciales que Kaniko (Secret sytue-registry, montado donde el keychain de
// go-containerregistry lo lee: $DOCKER_CONFIG/config.json).

const (
	// cnbDockerConfigDir es donde se monta el Secret del Registry; DOCKER_CONFIG
	// apunta aquí para que el lifecycle (go-containerregistry) autentique el push.
	cnbDockerConfigDir = "/cnb-docker-config"

	// GitHub is the sole temporary non-EU forge exception. Pinning its published
	// Ed25519 host key prevents a stolen DNS route from turning an SSH deploy key
	// into a man-in-the-middle credential. When the EU forge is enabled it gets a
	// separately reviewed key; never replace this with ssh-keyscan at runtime.
	githubKnownHosts = "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n"

	// registry-cred: el lifecycle de CNB NO se corre como un único `creator` (que
	// fundía build no confiable + push con la credencial en el mismo contenedor).
	// Se parte en dos: las fases que ejecutan código de buildpack no confiable
	// (detector + builder) corren en un initContainer SIN la credencial del
	// registry; y las fases de confianza que la necesitan (analyzer read + exporter
	// write) corren en el contenedor principal CON la credencial. Así un buildpack
	// malicioso en la fase build no puede leer la credencial de push del registry
	// compartido (que permitiría empujar a repos de otros tenants).
	//
	// cnbBuildScript: detector (produce group.toml/plan.toml) + builder (ejecuta los
	// build scripts de los buildpacks). Sin credencial; opera sobre /workspace + /layers.
	cnbBuildScript = `set -e
/cnb/lifecycle/detector -app="$APP" -layers=/layers -group=/layers/group.toml -plan=/layers/plan.toml -log-level=info
/cnb/lifecycle/builder -app="$APP" -layers=/layers -group=/layers/group.toml -plan=/layers/plan.toml -log-level=info`

	// cnbExportScript: analyzer (escribe analyzed.toml con la run image; lee el
	// registro para la imagen previa si existe) + exporter (ensambla y EMPUJA la
	// imagen OCI). Extrae el digest de report.toml a /dev/termination-log — la MISMA
	// vía que Kaniko (--digest-file) para que podTerminationDigest lo recoja. El grep
	// es best-effort (|| true): sin digest, el deploy cae a la tag por-commit.
	// ponytail: la superficie exacta de flags/fases del lifecycle depende de la
	// versión del builder de Paketo pinneado (CNB_BUILDER_IMAGE); se valida contra un
	// build vivo en go-live, igual que se re-pinnea KANIKO_IMAGE por digest.
	cnbExportScript = `set -e
RUN_FLAG=""
[ -z "$RUN_IMAGE" ] || RUN_FLAG="-run-image=$RUN_IMAGE"
/cnb/lifecycle/analyzer -layers=/layers -analyzed=/layers/analyzed.toml $RUN_FLAG "$DEST"
/cnb/lifecycle/exporter -app="$APP" -layers=/layers -group=/layers/group.toml -analyzed=/layers/analyzed.toml -report=/tmp/report.toml "$DEST"
grep -o 'sha256:[a-f0-9]*' /tmp/report.toml | head -n1 > /dev/termination-log || true`

	// gitCloneScript clona EXACTAMENTE el ref/commit pedido en /workspace, con
	// las mismas 3 modalidades de auth para Dockerfile y CNB: deploy-key SSH, token HTTPS
	// efímero de la GitHub App (credential helper por env, NUNCA en args/URL) o
	// clone público. La rama SSH usa una clave de host fijada, nunca TOFU ni
	// StrictHostKeyChecking=no. Todo lo escribible queda en /workspace.
	gitCloneScript = `set -eu
cd /workspace
git init -q
if [ -f /secret/git/id_rsa ]; then
  export GIT_SSH_COMMAND="ssh -i /secret/git/id_rsa -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/secret/git/known_hosts -o IdentitiesOnly=yes"
  git remote add origin "$SSH_REPO"
else
  git remote add origin "$HTTPS_REPO"
  [ -z "${GIT_TOKEN:-}" ] || git config credential.helper '!f(){ echo username=x-access-token; echo "password=$GIT_TOKEN"; };f'
fi
git -c protocol.version=2 fetch --no-tags --depth 1 origin "$GIT_REF"
git checkout -q --detach FETCH_HEAD
# El helper solo referencia una variable efímera, pero tampoco debe quedar en el
# contexto que ejecutará código de tenant.
git config --unset-all credential.helper 2>/dev/null || true`
)

// buildpacksJob traduce el JobSpec a un Job que corre el lifecycle de CNB. Espeja
// el Job de Kaniko en seguridad (rootless uid 1000, rootfs de solo lectura, sin
// escalada, sin token de SA) y en el montaje del Secret del Registry, pero con la
// forma del lifecycle: un initContainer clona el contexto y el contenedor
// `creator` construye y empuja. Todas las rutas escribibles son emptyDir (rootfs
// de solo lectura): /workspace (fuente), /layers, /cache y /tmp (report.toml).
func (c *KubeJobClient) buildpacksJob(spec JobSpec) *batchv1.Job {
	backoff := int32(0)
	ttl := int32(3600)
	activeDeadline := int64(1800)
	automountSA := false

	labels := map[string]string{
		"app.kubernetes.io/part-of":    "sytue-one",
		"app.kubernetes.io/managed-by": "sytue-cloud",
		"sytue.eu/component":           "cloud-build",
		"sytue.eu/strategy":            "buildpacks",
	}
	for k, v := range spec.Labels {
		labels[k] = v
	}

	volumes := []corev1.Volume{
		{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "layers", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "cache", VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: ptrQuantity(c.cacheVolumeSize)},
		}},
		{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
	if spec.RegistrySecret != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "docker-config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: spec.RegistrySecret,
					Items:      []corev1.KeyToPath{{Key: ".dockerconfigjson", Path: "config.json"}},
				},
			},
		})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name:         "docker-config",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}

	// Volúmenes de trabajo del lifecycle, compartidos entre las fases (initContainer
	// de build y contenedor de export): /workspace (fuente), /layers, /cache, /tmp.
	buildMounts := []corev1.VolumeMount{
		{Name: "workspace", MountPath: "/workspace"},
		{Name: "layers", MountPath: "/layers"},
		{Name: "cache", MountPath: "/cache"},
		{Name: "tmp", MountPath: "/tmp"},
	}

	// Fase de BUILD no confiable (initContainer): detector + builder ejecutan los
	// scripts de los buildpacks. NO monta la credencial del registry (registry-cred).
	cnbBuild := corev1.Container{
		Name:            "cnb-build",
		Image:           spec.Image, // builder de Paketo (CNB_BUILDER_IMAGE), pinneado en config
		Command:         []string{"/bin/sh", "-c", cnbBuildScript},
		SecurityContext: restrictedContainerSecurityContext(),
		Env: []corev1.EnvVar{
			{Name: "APP", Value: firstNonEmpty(spec.AppDir, "/workspace")},
		},
		Resources:    buildResourceRequirements(),
		VolumeMounts: buildMounts,
	}

	// Fase de EXPORT de confianza (contenedor principal): analyzer + exporter
	// ensamblan y EMPUJAN la imagen. Único contenedor con la credencial de push
	// (DOCKER_CONFIG → Secret del Registry); no ejecuta código de buildpack.
	cnbExport := corev1.Container{
		Name:            "cnb-export",
		Image:           spec.Image,
		Command:         []string{"/bin/sh", "-c", cnbExportScript},
		SecurityContext: restrictedContainerSecurityContext(),
		Env: []corev1.EnvVar{
			{Name: "APP", Value: firstNonEmpty(spec.AppDir, "/workspace")},
			{Name: "DEST", Value: spec.Destination},
			{Name: "RUN_IMAGE", Value: spec.RunImage},
			{Name: "DOCKER_CONFIG", Value: cnbDockerConfigDir},
		},
		Resources: buildResourceRequirements(),
		VolumeMounts: append(append([]corev1.VolumeMount{}, buildMounts...),
			corev1.VolumeMount{Name: "docker-config", MountPath: cnbDockerConfigDir, ReadOnly: true}),
	}

	// initContainer de clonado: prepara el contexto git en /workspace. Reusa los
	// Secrets de credenciales git (deploy-key SSH / token HTTPS) que Submit crea.
	gitClone := corev1.Container{
		Name:            "git-clone",
		Image:           spec.GitImage, // imagen mínima con git (CNB_GIT_IMAGE), pinneada
		Command:         []string{"/bin/sh", "-c", gitCloneScript},
		SecurityContext: restrictedContainerSecurityContext(),
		Env: []corev1.EnvVar{
			{Name: "HOME", Value: "/workspace"},
			{Name: "GIT_REF", Value: spec.GitRef},
			{Name: "HTTPS_REPO", Value: spec.RepoURL},
			{Name: "SSH_REPO", Value: sshRepoURL(spec.RepoURL)},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "workspace", MountPath: "/workspace"},
			{Name: "tmp", MountPath: "/tmp"},
		},
	}

	// Credenciales git para el clonado (espejan el Job de Kaniko):
	//   - deploy-key SSH montada en /secret/git/id_rsa (el script usa GIT_SSH_COMMAND).
	//   - token HTTPS efímero de la GitHub App como env GIT_TOKEN desde un Secret
	//     (SecretKeyRef): NUNCA en args, URL ni logs; el credential helper lo lee.
	if spec.GitSSHKeyContent != "" {
		gitClone.VolumeMounts = append(gitClone.VolumeMounts, corev1.VolumeMount{
			Name: "git-ssh", MountPath: "/secret/git", ReadOnly: true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "git-ssh",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  gitSSHSecretName(spec.Name),
					DefaultMode: ptrInt32(0o400),
					Items: []corev1.KeyToPath{
						{Key: "id_rsa", Path: "id_rsa", Mode: ptrInt32(0o400)},
						{Key: "known_hosts", Path: "known_hosts", Mode: ptrInt32(0o444)},
					},
				},
			},
		})
	}
	if spec.GitToken != "" {
		gitClone.Env = append(gitClone.Env, corev1.EnvVar{
			Name: "GIT_TOKEN",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: gitTokenSecretName(spec.Name)},
				Key:                  "password",
			}},
		})
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   spec.Namespace,
			Labels:      labels,
			Annotations: spec.Annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			ActiveDeadlineSeconds:   &activeDeadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             registryImagePullSecrets(spec.RegistrySecret),
					SecurityContext:              restrictedPodSecurityContext(),
					// Clonado y build no confiable (sin cred) como initContainers, en
					// orden; el export (con cred) como contenedor principal, tras ellos.
					InitContainers: []corev1.Container{gitClone, cnbBuild},
					Containers:     []corev1.Container{cnbExport},
					Volumes:        volumes,
				},
			},
		},
	}
}

func registryImagePullSecrets(secretName string) []corev1.LocalObjectReference {
	if secretName == "" {
		return nil
	}
	return []corev1.LocalObjectReference{{Name: secretName}}
}

// buildResourceRequirements son los requests/limits compartidos por los Jobs de
// build (Kaniko y CNB): límites acotados para no dejar que una build agote el nodo.
func buildResourceRequirements() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}
