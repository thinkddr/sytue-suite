// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ---------------------------------------------------------------------------
// Paso de release: lo que corre ANTES de arrancar la versión nueva
// ---------------------------------------------------------------------------
//
// Es el sitio de una migración de base de datos, y sin él una app con esquema
// versionado no se puede desplegar bien: migrar al arrancar el contenedor hace
// que TODAS las réplicas migren a la vez sobre la misma base —y si falla, el pod
// entra en CrashLoopBackOff sin decir por qué—, y migrar a mano por la terminal
// es un paso manual en cada despliegue, que es justo lo que un PaaS quita.
//
// # El estado del Job ES el estado
//
// No hay columna nueva ni máquina de estados en la base: el reconciliador
// pregunta por el Job en cada ciclo. Si no existe, lo crea; si existe, mira cómo
// va. Eso lo hace idempotente y resistente a que el controlador se reinicie a
// mitad, que es exactamente lo que se necesita de un paso que puede tardar
// minutos.

// releaseResourceName es el nombre del Job de release de un deployment. Comparte
// el sufijo con los objetos de la app (mismo `shortID`) para que en `kubectl get
// jobs` se vea de un vistazo a qué despliegue pertenece.
func releaseResourceName(spec AppSpec) string {
	return "release-" + shortID(spec.DeploymentID)
}

// RunRelease crea el Job de release si hace falta y devuelve en qué punto está.
// Implementa Platform.
//
// El Job hereda de la app TODO lo que le da sentido: la misma imagen (para que la
// migración sea la de ESTA versión del código), las mismas variables y el mismo
// Secret de credenciales (para que hable con la misma base de datos) y el mismo
// SecurityContext restringido, que es lo que el namespace del tenant exige.
//
// `backoffLimit: 0` a propósito: una migración que falló a medias no mejora
// repitiéndose sola. Se para, el despliegue queda en error con el motivo, y quien
// lo arregla decide.
func (p *KubePlatform) RunRelease(ctx context.Context, spec AppSpec) (ReleaseState, error) {
	name := releaseResourceName(spec)
	api := p.cs.BatchV1().Jobs(spec.Namespace)

	job, err := api.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return releaseStateOf(job), nil
	}
	if !apierrors.IsNotFound(err) {
		return ReleaseRunning, fmt.Errorf("controller: consultar Job de release: %w", err)
	}

	// No existe: se crea. El Secret con las credenciales lo materializa ApplyApp
	// con el nombre de la app, y aquí se referencia el MISMO —así la migración ve
	// exactamente el entorno que verá la app, sin una segunda copia que se
	// desincronice—. Existe ya porque applySecret corre antes que esto.
	if err := p.applySecret(ctx, spec, appResourceName(spec)); err != nil {
		return ReleaseRunning, err
	}
	if _, err := api.Create(ctx, p.releaseJob(spec, name), metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Otra réplica del controlador se adelantó: no es un error, es la
			// carrera normal entre dos reconciliadores.
			return ReleaseRunning, nil
		}
		return ReleaseRunning, fmt.Errorf("controller: crear Job de release: %w", err)
	}
	log.From(ctx).Info("paso de release lanzado", "namespace", spec.Namespace, "job", name)
	return ReleaseRunning, nil
}

// releaseStateOf traduce el estado de un Job a las tres respuestas que el
// reconciliador entiende.
//
// `Failed > 0` con backoffLimit 0 significa que el pod terminó mal una vez, que
// es definitivo. Sin `Succeeded` ni `Failed`, sigue corriendo (o el pod aún se
// está creando, que para quien espera es lo mismo).
func releaseStateOf(job *batchv1.Job) ReleaseState {
	switch {
	case job.Status.Succeeded > 0:
		return ReleaseSucceeded
	case job.Status.Failed > 0:
		return ReleaseFailed
	default:
		return ReleaseRunning
	}
}

// releaseJob construye el Job del paso de release.
func (p *KubePlatform) releaseJob(spec AppSpec, name string) *batchv1.Job {
	runAsNonRoot := true
	readOnlyRoot := true
	allowPrivEsc := false
	runAsUser := int64(1000)
	automountSA := false
	backoff := int32(0)
	// El Job terminado se limpia solo al día siguiente: sus logs son lo que
	// explica un despliegue fallido, así que no se borra en el acto, pero tampoco
	// se queda para siempre ocupando el namespace del cliente.
	ttl := int32(24 * 60 * 60)
	// Tope de duración: una migración colgada bloquearía el despliegue para
	// siempre y el reconciliador seguiría esperándola en cada ciclo.
	deadline := int64(30 * 60)
	tmpSize := resource.MustParse("256Mi")

	env := make([]corev1.EnvVar, 0, len(spec.Env)+1)
	for k, v := range spec.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}
	env = append(env, corev1.EnvVar{Name: "TMPDIR", Value: "/tmp"})

	size := domain.InstanceSizeOrDefault(spec.Size.Name)
	labels := appLabels(spec)
	labels["sytue.eu/step"] = "release"

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: spec.Namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			ActiveDeadlineSeconds:   &deadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             tenantImagePullSecrets(),
					Containers: []corev1.Container{{
						Name:  "release",
						Image: spec.Image,
						// `sh -c` porque lo que el usuario escribe es una orden de
						// shell («npm run migrate && npm run seed»), no un argv. La
						// imagen es suya y el contenedor también: no hay nada que
						// filtrar aquí que no pudiera hacer ya en su Dockerfile.
						Command: []string{"/bin/sh", "-c", spec.ReleaseCommand},
						Env:     env,
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: appResourceName(spec),
								},
							},
						}},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             &runAsNonRoot,
							RunAsUser:                &runAsUser,
							AllowPrivilegeEscalation: &allowPrivEsc,
							ReadOnlyRootFilesystem:   &readOnlyRoot,
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
						},
						// Los mismos recursos que la app: una migración corre el
						// código del mismo proyecto y no tiene por qué caber en
						// menos de lo que necesita la aplicación.
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(size.CPURequest),
								corev1.ResourceMemory: resource.MustParse(size.MemoryRequest),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(size.CPULimit),
								corev1.ResourceMemory: resource.MustParse(size.MemoryLimit),
							},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}},
					}},
					Volumes: []corev1.Volume{{
						Name: "tmp",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &tmpSize},
						},
					}},
				},
			},
		},
	}
}

// RunRelease del stub: en dev no hay clúster donde ejecutar nada, así que el paso
// se da por hecho. Devolver «running» dejaría todo despliegue local atascado para
// siempre esperando un Job que nadie va a crear.
func (StubPlatform) RunRelease(ctx context.Context, spec AppSpec) (ReleaseState, error) {
	if spec.ReleaseCommand != "" {
		log.From(ctx).Info("stub-platform: paso de release omitido (dev, sin clúster)",
			"comando", spec.ReleaseCommand)
	}
	return ReleaseSucceeded, nil
}
