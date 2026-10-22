// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// KubeJobClient es la implementación de producción de JobClient sobre Kapsule
// (Kubernetes) con client-go. Programa Jobs de Kaniko rootless, sigue su estado
// hasta terminación y recoge los logs del Pod. Es la contraparte real de la
// interfaz JobClient que el StubRunner simula en dev.
//
// Seguridad (CONVENTIONS §0.7): el Pod corre rootless con securityContext
// restrictivo (sin privilegios, sistema de ficheros raíz de solo lectura salvo
// los volúmenes de trabajo de Kaniko, capacidades eliminadas, seccomp
// RuntimeDefault) y sin montar el token de la ServiceAccount.
type KubeJobClient struct {
	cs kubernetes.Interface
	// pollInterval es la cadencia de sondeo del estado del Job.
	pollInterval time.Duration
	// buildkitImage es la imagen de BuildKit rootless (Apache-2.0), fijada por
	// digest en producción vía config.
	buildkitImage string
	// craneImage es la imagen de crane (go-containerregistry, Apache-2.0) del
	// contenedor de push, que empuja el tarball OCI construido sin credencial.
	craneImage string
	// cacheDir monta el directorio de cache de capas de Kaniko.
	cacheVolumeSize string
}

// KubeJobConfig parametriza el cliente de Jobs.
type KubeJobConfig struct {
	// KubeconfigPath vacío => configuración in-cluster (ServiceAccount montada).
	KubeconfigPath string
	BuildkitImage  string
	CraneImage     string
	PollInterval   time.Duration
}

// NewKubeJobClient construye el cliente de Jobs sobre Kapsule. Con
// KubeconfigPath vacío usa la configuración in-cluster (el pod del builder corre
// dentro de Kapsule con una ServiceAccount con permisos sobre Jobs/Pods en el
// namespace de builds).
func NewKubeJobClient(cfg KubeJobConfig) (*KubeJobClient, error) {
	restCfg, err := restConfig(cfg.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("builder: crear clientset: %w", err)
	}
	return newKubeJobClientFor(cs, cfg), nil
}

// newKubeJobClientFor permite inyectar un clientset (fake en tests).
func newKubeJobClientFor(cs kubernetes.Interface, cfg KubeJobConfig) *KubeJobClient {
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 3 * time.Second
	}
	img := cfg.BuildkitImage
	if img == "" {
		img = "moby/buildkit:v0.32.1-rootless@sha256:66e5fe5b25509626c0e628c6e57badbcc2e8a62218ba3e1358f1427e90de6a88"
	}
	crane := cfg.CraneImage
	if crane == "" {
		crane = "gcr.io/go-containerregistry/crane:v0.21.8@sha256:e85fa14e99e5ef7351822ed887ce322bc1e22f9ef8e9eb54ebf2d7a9473bfef9"
	}
	return &KubeJobClient{cs: cs, pollInterval: poll, buildkitImage: img, craneImage: crane, cacheVolumeSize: "2Gi"}
}

// restConfig deriva la configuración REST de Kubernetes: in-cluster si la ruta
// está vacía, o del kubeconfig indicado en caso contrario.
func restConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath == "" {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("builder: config in-cluster: %w", err)
		}
		return cfg, nil
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("builder: cargar kubeconfig %q: %w", kubeconfigPath, err)
	}
	return cfg, nil
}

// Submit crea el Job de build (Kaniko o lifecycle de CNB) y devuelve su nombre.
// Implementa JobClient.
func (c *KubeJobClient) Submit(ctx context.Context, spec JobSpec) (string, error) {
	job := c.jobFor(spec)
	created, err := c.cs.BatchV1().Jobs(spec.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("builder: crear Job de build: %w", err)
	}
	// Frameworks gestionados: materializa el Dockerfile generado en un ConfigMap
	// que el Pod monta en /workspace/Dockerfile.sytue. OwnerReference al Job para
	// que el recolector de basura lo elimine en cascada (vía TTL o Cleanup del Job).
	if spec.DockerfileContent != "" {
		cm := dockerfileConfigMap(spec, created)
		if _, err := c.cs.CoreV1().ConfigMaps(spec.Namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			// Sin el ConfigMap el Pod no puede montar el Dockerfile: aborta y limpia el Job.
			_ = c.Cleanup(context.WithoutCancel(ctx), spec.Namespace, created.Name)
			return "", fmt.Errorf("builder: crear ConfigMap del Dockerfile: %w", err)
		}
	}
	if spec.GitSSHKeyContent != "" {
		sec := gitSSHSecret(spec, created)
		if _, err := c.cs.CoreV1().Secrets(spec.Namespace).Create(ctx, sec, metav1.CreateOptions{}); err != nil {
			_ = c.Cleanup(context.WithoutCancel(ctx), spec.Namespace, created.Name)
			return "", fmt.Errorf("builder: crear Secret de deploy key: %w", err)
		}
	}
	if spec.GitToken != "" {
		sec := gitTokenSecret(spec, created)
		if _, err := c.cs.CoreV1().Secrets(spec.Namespace).Create(ctx, sec, metav1.CreateOptions{}); err != nil {
			_ = c.Cleanup(context.WithoutCancel(ctx), spec.Namespace, created.Name)
			return "", fmt.Errorf("builder: crear Secret de token de GitHub App: %w", err)
		}
	}
	log.From(ctx).Info("Job de Kaniko creado", "job", created.Name, "namespace", spec.Namespace)
	return created.Name, nil
}

// Wait bloquea hasta que el Job termina (éxito o fallo) o se cancela el
// contexto, y devuelve el resultado junto con los logs del Pod. Implementa
// JobClient.
func (c *KubeJobClient) Wait(ctx context.Context, namespace, name string, isCanceled func() bool) (bool, []byte, error) {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		if isCanceled != nil && isCanceled() {
			logs := c.collectLogs(context.WithoutCancel(ctx), namespace, name)
			_ = c.Cleanup(context.WithoutCancel(ctx), namespace, name)
			return false, logs, fmt.Errorf("build cancelada")
		}
		job, err := c.cs.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil, fmt.Errorf("builder: consultar Job %s: %w", name, err)
		}
		if done, ok := jobResult(job); ok {
			logs := c.collectLogs(ctx, namespace, name)
			digest := c.podTerminationDigest(ctx, namespace, name)
			if digest != "" {
				logs = append(logs, []byte("\n__digest__="+digest)...)
			}
			return done, logs, nil
		}
		select {
		case <-ctx.Done():
			// Intenta capturar logs parciales antes de rendirse.
			logs := c.collectLogs(context.WithoutCancel(ctx), namespace, name)
			return false, logs, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Cleanup elimina el Job y sus Pods (propagación en segundo plano). Implementa
// JobClient.
func (c *KubeJobClient) Cleanup(ctx context.Context, namespace, name string) error {
	policy := metav1.DeletePropagationBackground
	err := c.cs.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("builder: eliminar Job %s: %w", name, err)
	}
	return nil
}

// jobResult examina las condiciones del Job. ok=false si aún no ha terminado.
func jobResult(job *batchv1.Job) (succeeded, ok bool) {
	for _, cond := range job.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			continue
		}
		switch cond.Type {
		case batchv1.JobComplete:
			return true, true
		case batchv1.JobFailed:
			return false, true
		default: // Suspended/FailureTarget/SuccessCriteriaMet: aún no terminal.
		}
	}
	return false, false
}

// collectLogs recupera los logs del Pod del Job (best-effort). Kaniko emite el
// progreso y errores de la build por stdout.
func (c *KubeJobClient) collectLogs(ctx context.Context, namespace, jobName string) []byte {
	pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil || len(pods.Items) == 0 {
		return nil
	}
	// Usa el Pod más reciente del Job.
	pod := pods.Items[len(pods.Items)-1]
	rc, err := c.cs.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return nil
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 8<<20)) // límite 8 MiB
	if err != nil {
		return data
	}
	return data
}

// podTerminationDigest lee el digest que Kaniko escribe en termination-log.
func (c *KubeJobClient) podTerminationDigest(ctx context.Context, namespace, jobName string) string {
	pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil || len(pods.Items) == 0 {
		return ""
	}
	pod := pods.Items[len(pods.Items)-1]
	// El digest lo escribe en /dev/termination-log quien construye la imagen:
	// Kaniko como initContainer (path Dockerfile) o el exporter de CNB como
	// contenedor principal (path buildpacks). Se recorren ambos y se toma el primer
	// mensaje de terminación que contenga un sha256.
	statuses := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
	for _, cs := range statuses {
		if cs.State.Terminated == nil || cs.State.Terminated.Message == "" {
			continue
		}
		if d := extractDigest(strings.TrimSpace(cs.State.Terminated.Message)); d != "" {
			return d
		}
	}
	return ""
}

func extractDigest(s string) string {
	if i := strings.Index(s, "sha256:"); i >= 0 {
		d := s[i:]
		if j := strings.IndexAny(d, " \t\n\r"); j > 0 {
			d = d[:j]
		}
		return d
	}
	return ""
}

// jobFor elige la forma del Job según la estrategia del JobSpec: el lifecycle de
// CNB (buildpacks) o Kaniko (Dockerfile). Ambos comparten el securityContext
// restrictivo, el Secret del Registry, los labels y el patrón de limpieza.
func (c *KubeJobClient) jobFor(spec JobSpec) *batchv1.Job {
	if spec.Buildpacks {
		return c.buildpacksJob(spec)
	}
	return c.buildJob(spec)
}

// restrictedContainerSecurityContext es el securityContext rootless y sin
// privilegios que comparten TODOS los contenedores de build (executor de Kaniko,
// lifecycle de CNB e initContainer de clonado): non-root uid 1000, rootfs de solo
// lectura, sin escalada de privilegios, todas las capabilities eliminadas y
// seccomp RuntimeDefault (CONVENTIONS §0.7). Un único origen para que las dos
// estrategias de build no puedan derivar en su postura de seguridad.
func restrictedContainerSecurityContext() *corev1.SecurityContext {
	runAsNonRoot := true
	readOnlyRoot := true
	allowPrivEsc := false
	runAsUser := int64(1000)
	return &corev1.SecurityContext{
		RunAsNonRoot:             &runAsNonRoot,
		RunAsUser:                &runAsUser,
		AllowPrivilegeEscalation: &allowPrivEsc,
		ReadOnlyRootFilesystem:   &readOnlyRoot,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

// restrictedPodSecurityContext es el PodSecurityContext rootless compartido por
// los Jobs de build (non-root uid/fsGroup 1000, seccomp RuntimeDefault).
func restrictedPodSecurityContext() *corev1.PodSecurityContext {
	runAsNonRoot := true
	runAsUser := int64(1000)
	fsGroup := int64(1000)
	return &corev1.PodSecurityContext{
		RunAsNonRoot:   &runAsNonRoot,
		RunAsUser:      &runAsUser,
		FSGroup:        &fsGroup,
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

// buildJob traduce el JobSpec declarativo a un batchv1.Job con securityContext
// restrictivo, sin token de ServiceAccount montado, límites de recursos y el
// Secret del Container Registry montado en /kaniko/.docker.
func (c *KubeJobClient) buildJob(spec JobSpec) *batchv1.Job {
	backoff := int32(0)           // sin reintentos: una build falla, se marca fallida
	ttl := int32(3600)            // recolector de basura del Job tras 1h
	activeDeadline := int64(1800) // 30 min máximo de ejecución del Pod
	automountSA := false

	labels := map[string]string{
		"app.kubernetes.io/part-of":    "sytue-one",
		"app.kubernetes.io/managed-by": "sytue-cloud",
		"sytue.eu/component":           "cloud-build",
		"sytue.eu/stack":               "go",
	}
	for k, v := range spec.Labels {
		labels[k] = v
	}

	// Primer initContainer: clona con la credencial efímera y host key fijada.
	// El builder que ejecuta código del tenant corre después sin credencial git.
	gitClone := corev1.Container{
		Name:            "git-clone",
		Image:           spec.GitImage,
		Command:         []string{"/bin/sh", "-c", gitCloneScript},
		SecurityContext: restrictedContainerSecurityContext(),
		Resources:       buildResourceRequirements(),
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

	// Contenedor de BUILD (segundo initContainer): BuildKit ejecuta el RUN no
	// confiable del Dockerfile y escribe la imagen como tarball en /workspace. NO
	// monta la credencial del registry (registry-cred): un RUN malicioso no puede
	// leerla. El push lo hace después el contenedor `push` con la credencial.
	//
	// BuildKit y no Kaniko: Google archivó Kaniko en junio de 2025 —sin versiones
	// ni parches— y esto compila código de terceros, que es donde menos conviene
	// un motor sin mantener.
	//
	// `buildctl-daemonless.sh` levanta un buildkitd efímero para este único build
	// y termina con él; no hay demonio compartido entre tenants.
	//
	// --oci-worker-no-process-sandbox es OBLIGATORIO aquí: el modo rootless normal
	// exige seccomp/apparmor `unconfined`, que PodSecurity `restricted` prohíbe en
	// este namespace. Renuncia al sandbox de proceso DENTRO del contenedor, no al
	// aislamiento del contenedor: el pod sigue sin privilegios, sin capabilities y
	// con RuntimeDefault, y es exactamente la misma postura que tenía Kaniko, que
	// tampoco sandboxeaba el RUN. Cada build corre en su propio Job efímero.
	container := corev1.Container{
		Name:    "build",
		Image:   firstNonEmpty(spec.Image, c.buildkitImage),
		Command: []string{"buildctl-daemonless.sh"},
		Args:    spec.Args,
		Env: []corev1.EnvVar{
			{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
			// XDG_RUNTIME_DIR y HOME en el emptyDir: buildkitd escribe ahí su
			// estado y siendo non-root no tiene otro sitio escribible.
			{Name: "XDG_RUNTIME_DIR", Value: "/workspace/.buildkit"},
			{Name: "HOME", Value: "/workspace"},
		},
		SecurityContext: restrictedContainerSecurityContext(),
		Resources:       buildResourceRequirements(),
		VolumeMounts: []corev1.VolumeMount{
			{Name: "workspace", MountPath: "/workspace"},
			{Name: "cache", MountPath: "/cache"},
		},
	}

	volumes := []corev1.Volume{
		{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
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
					Items: []corev1.KeyToPath{
						{Key: ".dockerconfigjson", Path: "config.json"},
					},
				},
			},
		})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name:         "docker-config",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}

	// Frameworks gestionados: el Dockerfile generado (ConfigMap creado en Submit)
	// se monta de solo lectura sobre el /workspace compartido en la ruta que Kaniko
	// espera. subPath monta solo ese fichero y no vuelve /workspace de solo lectura.
	if spec.DockerfileContent != "" {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "dockerfile",
			MountPath: generatedDockerfilePath,
			SubPath:   generatedDockerfileName,
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "dockerfile",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapNameFor(spec.Name)},
				},
			},
		})
	}
	if spec.GitSSHKeyContent != "" {
		gitClone.VolumeMounts = append(gitClone.VolumeMounts, corev1.VolumeMount{
			Name:      "git-ssh",
			MountPath: "/secret/git",
			ReadOnly:  true,
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
	// GitHub App: solo el initContainer de clone recibe el token HTTPS efímero.
	// Kaniko no lo monta ni lo recibe como env.
	if spec.GitToken != "" {
		secretRef := func(key string) *corev1.EnvVarSource {
			return &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: gitTokenSecretName(spec.Name)},
				Key:                  key,
			}}
		}
		gitClone.Env = append(gitClone.Env,
			corev1.EnvVar{Name: "GIT_TOKEN", ValueFrom: secretRef("password")})
	}

	// Contenedor de PUSH: crane empuja al Container Registry el tarball OCI que
	// Kaniko construyó en /workspace. ES el único que monta la credencial de push
	// (docker-config) — y NO ejecuta código de tenant, así que la credencial no es
	// alcanzable por el build. /workspace de solo lectura (solo lee el tarball);
	// /tmp escribible (crane hace streaming de capas). El digest lo escribió Kaniko
	// (init) en su termination-log (ver podTerminationDigest, que lee initContainers).
	push := corev1.Container{
		Name:            "push",
		Image:           c.craneImage,
		Args:            []string{"push", kanikoTarPath, spec.Destination},
		SecurityContext: restrictedContainerSecurityContext(),
		Resources:       buildResourceRequirements(),
		Env: []corev1.EnvVar{
			{Name: "DOCKER_CONFIG", Value: pushDockerConfigDir},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "workspace", MountPath: "/workspace", ReadOnly: true},
			{Name: "tmp", MountPath: "/tmp"},
			{Name: "docker-config", MountPath: pushDockerConfigDir, ReadOnly: true},
		},
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
					// Clone con cred y build sin cred corren secuencialmente; el push
					// (con credencial de registry) solo empieza al terminar ambos.
					InitContainers: []corev1.Container{gitClone, container},
					Containers:     []corev1.Container{push},
					Volumes:        volumes,
				},
			},
		},
	}
}

// pushDockerConfigDir es donde el contenedor de push monta la credencial del
// registry; DOCKER_CONFIG apunta aquí para que crane autentique el push. NUNCA se
// monta en el contenedor de build no confiable.
const pushDockerConfigDir = "/push-docker-config"

// ptrQuantity parsea una cantidad de recursos y devuelve su puntero (nil si
// inválida, dejando el EmptyDir sin límite explícito).
func ptrQuantity(s string) *resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return nil
	}
	return &q
}

// configMapNameFor deriva el nombre del ConfigMap que porta el Dockerfile
// generado, a partir del nombre del Job (determinista, para montaje y limpieza).
func configMapNameFor(jobName string) string { return jobName + "-dockerfile" }

// dockerfileConfigMap construye el ConfigMap con el Dockerfile generado. Lleva un
// OwnerReference al Job para que el recolector de basura de Kubernetes lo elimine
// en cascada cuando el Job desaparece (Cleanup o TTLSecondsAfterFinished), sin
// dejar ConfigMaps huérfanos a escala.
func dockerfileConfigMap(spec JobSpec, owner *batchv1.Job) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapNameFor(spec.Name),
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "sytue-cloud",
				"sytue.eu/component":           "cloud-build",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1",
				Kind:       "Job",
				Name:       owner.Name,
				UID:        owner.UID,
			}},
		},
		Data: map[string]string{generatedDockerfileName: spec.DockerfileContent},
	}
}

func gitSSHSecretName(jobName string) string { return jobName + "-git-ssh" }

func gitSSHSecret(spec JobSpec, owner *batchv1.Job) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitSSHSecretName(spec.Name),
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "sytue-cloud",
				"sytue.eu/component":           "cloud-build",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1",
				Kind:       "Job",
				Name:       owner.Name,
				UID:        owner.UID,
			}},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"id_rsa":      spec.GitSSHKeyContent,
			"known_hosts": githubKnownHosts,
		},
	}
}

func gitTokenSecretName(jobName string) string { return jobName + "-git-token" }

// gitTokenSecret porta la credencial HTTPS efímera de la GitHub App (basic auth
// x-access-token:<installation-token>). OwnerReference al Job para que el GC de
// Kubernetes lo borre en cascada al desaparecer el Job (Cleanup / TTL): el token
// no sobrevive a la build.
func gitTokenSecret(spec JobSpec, owner *batchv1.Job) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitTokenSecretName(spec.Name),
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "sytue-cloud",
				"sytue.eu/component":           "cloud-build",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1",
				Kind:       "Job",
				Name:       owner.Name,
				UID:        owner.UID,
			}},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"username": "x-access-token",
			"password": spec.GitToken,
		},
	}
}

func ptrInt32(v int32) *int32 { return &v }
