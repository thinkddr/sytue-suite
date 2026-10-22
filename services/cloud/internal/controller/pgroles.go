// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// Claves del Secret de credenciales de un Sytue Postgres que toca la reparación.
// El resto (POSTGRES_PASSWORD, PGRST_JWT_SECRET) NO se tocan jamás: son la conexión
// de la app del cliente y la firma de sus JWT; reescribirlas rompería su aplicación.
const (
	authPasswordKey = "PGRST_AUTHENTICATOR_PASSWORD"
	restDSNKey      = "PGRST_DB_URI"
)

// pgRolesAnnotation/pgRolesVersion marcan una instancia de Sytue Postgres cuyo
// PostgREST ya publica con authenticator/web_anon. Van en dos sitios:
//
//   - en el Secret de credenciales: es el marcador de "ya migrada" que hace que el
//     paso de reconciliación la salte (las instancias nuevas nacen marcadas);
//   - en el pod template de PostgREST: cambiar el Secret no reinicia pods, así que
//     la anotación es lo que fuerza el rollout de una instancia heredada.
//
// Bumpear pgRolesVersion vuelve a pasar la migración por toda la flota (y fuerza un
// rollout de PostgREST): es la palanca si el SQL de roles cambia.
const (
	pgRolesAnnotation = "sytue.eu/pgroles"
	pgRolesVersion    = "v1"
)

// pgManagedSelector selecciona los Secret de credenciales de los Sytue Postgres
// gestionados en CUALQUIER namespace. Son las etiquetas que pone managedLabels; el
// descubrimiento va contra Kubernetes y no contra la base de datos porque el estado
// que hay que reparar (Secret + Deployment) vive en el clúster, y así alcanza también
// a lo que el control plane no tenga registrado.
const pgManagedSelector = "app.kubernetes.io/managed-by=sytue-cloud,sytue.eu/service-kind=" + string(domain.ServicePostgres)

// postgresRoleRepairer es la capacidad OPCIONAL de reparar los roles de PostgREST de
// las instancias YA aprovisionadas. Solo la implementa el KubeProvisioner: el
// StubProvisioner de dev no tiene clúster que reparar. Se descubre por type-assert
// (idiom del repo, ver connector.SearchCapability en integrations) en vez de engordar
// la interfaz ServiceProvisioner con un método que una implementación no puede honrar.
type postgresRoleRepairer interface {
	RepairPostgresRoles(ctx context.Context) error
}

var _ postgresRoleRepairer = (*KubeProvisioner)(nil)

// reconcilePostgresRoles es el paso del bucle de reconciliación: no-op si el
// provisioner no sabe reparar (dev), delegación si sabe.
func (c *Controller) reconcilePostgresRoles(ctx context.Context) error {
	r, ok := c.provider.(postgresRoleRepairer)
	if !ok {
		return nil
	}
	return r.RepairPostgresRoles(ctx)
}

// RepairPostgresRoles migra a authenticator/web_anon las instancias de Sytue
// Postgres aprovisionadas ANTES de que PostgREST dejara de publicar con el rol
// dueño. Sin esto la corrección solo alcanzaba a las instancias nuevas, por dos
// motivos independientes: el entrypoint de Postgres corre
// /docker-entrypoint-initdb.d SOLO con el volumen vacío (un clúster ya
// inicializado nunca crea los roles) y un servicio 'ready' no vuelve a
// aprovisionarse jamás (su Deployment de PostgREST conserva PGRST_DB_ANON_ROLE:
// app). Es decir: toda instancia anterior seguiría con CRUD anónimo sobre los datos
// del cliente, indefinidamente.
//
// Coste en régimen permanente: UN List por ciclo y cero escrituras (las instancias
// migradas —y las nuevas, que nacen marcadas— se descartan por la anotación del
// Secret, sin un solo Get). Los errores se acumulan por instancia: una rota no
// impide reparar las demás.
func (p *KubeProvisioner) RepairPostgresRoles(ctx context.Context) error {
	namespaces, err := p.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: tenantNamespaceSelector})
	if err != nil {
		return fmt.Errorf("controller: listar namespaces de tenant para Sytue Postgres: %w", err)
	}
	var errs []error
	for i := range namespaces.Items {
		ns := &namespaces.Items[i]
		tenant := strings.TrimPrefix(ns.Name, "tenant-")
		if err := ownedTenantNamespace(ns, tenant); err != nil {
			errs = append(errs, err)
			continue
		}
		list, err := p.cs.CoreV1().Secrets(ns.Name).List(ctx, metav1.ListOptions{LabelSelector: pgManagedSelector})
		if err != nil {
			errs = append(errs, fmt.Errorf("controller: listar credenciales de Sytue Postgres en %s: %w", ns.Name, err))
			continue
		}
		for j := range list.Items {
			sec := &list.Items[j]
			// El sufijo descarta el Secret de plataforma (sytue-platform-s3), que lleva
			// estas mismas etiquetas por venir del backup de un servicio postgres.
			if sec.Annotations[pgRolesAnnotation] == pgRolesVersion || !strings.HasSuffix(sec.Name, "-creds") {
				continue
			}
			if rerr := p.repairPostgresInstance(ctx, sec); rerr != nil {
				errs = append(errs, fmt.Errorf("%s/%s: %w", sec.Namespace, sec.Name, rerr))
			}
		}
	}
	return errors.Join(errs...)
}

// repairPostgresInstance migra UNA instancia. Todos los pasos son idempotentes y el
// marcador se escribe al final: un fallo a mitad se reintenta entero en el ciclo
// siguiente en vez de dar por migrada una instancia que sigue publicando con el rol
// dueño (que es exactamente el fallo que este paso viene a arreglar).
func (p *KubeProvisioner) repairPostgresInstance(ctx context.Context, sec *corev1.Secret) error {
	name := strings.TrimSuffix(sec.Name, "-creds")
	ns := sec.Namespace
	dsn := string(sec.Data[restDSNKey])
	if dsn == "" {
		return nil // no es el Secret de credenciales de un Sytue Postgres
	}

	// 1) Contraseña propia del rol de conexión + DSN que entra como 'authenticator' en
	// vez de como el dueño. Solo si falta: en un reintento NO se regenera, porque el
	// Job pudo haberla fijado ya en la base y quedarían desincronizadas.
	if len(sec.Data[authPasswordKey]) == 0 {
		pwd, err := randToken(24)
		if err != nil {
			return err
		}
		u, err := url.Parse(dsn)
		if err != nil {
			return fmt.Errorf("controller: %s ilegible: %w", restDSNKey, err)
		}
		u.User = url.UserPassword(postgrestRole, pwd)
		sec.Data[authPasswordKey] = []byte(pwd)
		sec.Data[restDSNKey] = []byte(u.String())
		if err := p.upsertSecret(ctx, ns, sec); err != nil {
			return err
		}
	}

	// 2) Job que crea los roles en la base ya inicializada. upsertJob solo crea si no
	// existe: eso da el exactamente-una-vez sin marcador en base de datos (un Job es
	// run-to-completion; su nombre es el candado).
	if err := p.upsertJob(ctx, ns, p.pgRolesJob(name, ns, sec.Labels)); err != nil {
		return err
	}

	// 3) PostgREST con PGRST_DB_ANON_ROLE=web_anon y, sobre todo, con la anotación del
	// pod template que fuerza el rollout: sin reiniciar, los pods vivos seguirían con
	// el DSN del dueño que leyeron al arrancar y no se habría arreglado nada.
	if err := p.upsertDeployment(ctx, ns, p.postgrestDeployment(name, ns, sec.Labels)); err != nil {
		return err
	}

	// 4) Marcador, al final.
	if sec.Annotations == nil {
		sec.Annotations = map[string]string{}
	}
	sec.Annotations[pgRolesAnnotation] = pgRolesVersion
	if err := p.upsertSecret(ctx, ns, sec); err != nil {
		return err
	}
	log.From(ctx).Info("roles de PostgREST reparados en instancia heredada", "namespace", ns, "instance", name)
	return nil
}

// pgRolesJob ejecuta postgrestRolesSQL —el MISMO script que el ConfigMap de init—
// contra el Postgres del servicio. La conexión va por PGHOST/PGPASSWORD (libpq), que
// es lo único que distingue este camino del de init; la contraseña del authenticator
// llega por envFrom y psql la interpola con :'pw', nunca concatenada en el SQL.
//
// Postura de seguridad idéntica a la del backup/restore: el namespace del tenant
// fuerza PodSecurity 'restricted'. UID 999 (usuario postgres de la imagen oficial),
// sin token de ServiceAccount (el pod no necesita la API de K8s, así no puede
// enumerar Secrets del namespace), capabilities a cero y seccomp RuntimeDefault.
func (p *KubeProvisioner) pgRolesJob(name, namespace string, labels map[string]string) *batchv1.Job {
	runAsNonRoot := true
	allowPrivEsc := false
	pgUID := int64(999)
	automountSA := false
	// Tolerante a que Postgres tarde en levantar (o esté reiniciándose): reintenta con
	// backoff en vez de darse por vencido a la primera.
	backoff := int32(10)
	ttl := int32(86400) // se recoge solo un día después de terminar

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-pgroles", Namespace: namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				// Las etiquetas gestionadas (incluida la de proyecto) son las que la
				// NetworkPolicy del proyecto exige para dejar salir al 5432.
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyOnFailure,
					RuntimeClassName:             tenantRuntimeClassPtr(),
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             tenantImagePullSecrets(),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &runAsNonRoot,
						RunAsUser:      &pgUID,
						RunAsGroup:     &pgUID,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name:    "pgroles",
						Image:   p.postgresImage(),
						Command: []string{"sh", "-c", postgrestRolesSQL},
						Env: []corev1.EnvVar{
							{Name: "PGHOST", Value: name + "-postgres"},
							{Name: "PGPASSWORD", ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: name + "-creds"},
									Key:                  "POSTGRES_PASSWORD",
								},
							}},
						},
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: name + "-creds"},
							},
						}},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             &runAsNonRoot,
							AllowPrivilegeEscalation: &allowPrivEsc,
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:              resource.MustParse("50m"),
								corev1.ResourceMemory:           resource.MustParse("64Mi"),
								corev1.ResourceEphemeralStorage: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:              resource.MustParse("500m"),
								corev1.ResourceMemory:           resource.MustParse("256Mi"),
								corev1.ResourceEphemeralStorage: resource.MustParse("256Mi"),
							},
						},
					}},
				},
			},
		},
	}
}
