// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// Imágenes de los servicios gestionados, PINNEADAS a versión fija: los tags
// flotantes (:latest, o el rolling `18`) rompen la reproducibilidad y meten
// cambios de versión no auditados en cada re-pull. Los pines de PostgREST/mc
// coinciden con deploy/compose (una sola fuente de versión). En prod, ops debería
// sobreescribirlas por digest sha256.
const (
	managedPostgresImage  = "postgres:18.4@sha256:3a82e1f56c8f0f5616a11103ac3d47e632c3938698946a7ad26da0df1334744a"
	managedPostgrestImage = "postgrest/postgrest:v14.16@sha256:bea1c76a856fa39d1e542d25911cf95d02fe2bf971992d033044ff209f1504b8"
	managedRcloneImage    = "rclone/rclone:1.75.0@sha256:b06aed988cf5967de7c25be5925240983981c757f4ed1ac9d2fa659d51d60548"
)

// Roles con los que PostgREST publica la base de datos del cliente, con el mismo
// patrón que ya usa deploy/compose (docker-compose.yml + postgres/init/01-databases.sh):
//
//   - postgrestRole ("authenticator") es el rol de CONEXIÓN: LOGIN, NOINHERIT y sin
//     privilegios propios; solo puede asumir el rol del peticionario.
//   - postgrestAnonRole ("web_anon") es el rol de las peticiones SIN JWT: NOLOGIN y
//     con USAGE sobre el esquema public y nada más. Lo que se publique de forma
//     anónima tiene que concederlo el cliente (GRANT SELECT ... TO web_anon).
//
// Antes PostgREST conectaba —y resolvía el rol anónimo— como "app", que es el
// POSTGRES_USER del entrypoint, o sea el DUEÑO de la base de datos: el JWT era
// decorativo porque la ruta sin token ya tenía CRUD completo sobre todo.
const (
	postgrestRole     = "authenticator"
	postgrestAnonRole = "web_anon"
	// postgrestInitScript es la clave/fichero del script de arranque que crea esos
	// roles. El entrypoint de la imagen oficial de Postgres ejecuta
	// /docker-entrypoint-initdb.d/* en la primera inicialización del clúster.
	postgrestInitScript = "10-postgrest-roles.sh"
)

// postgrestRolesSQL crea los roles de PostgREST en el Postgres del tenant. La
// contraseña llega por entorno (el Secret de credenciales va montado con envFrom)
// y se interpola con :'pw', que psql cita como literal: nunca se concatena en el
// texto del SQL. El ALTER ROLE va FUERA del bloque DO a propósito: psql no
// sustituye variables dentro de una cadena con comillas de dólar.
//
// Es el MISMO texto en los dos caminos que lo ejecutan —el ConfigMap montado en
// /docker-entrypoint-initdb.d (instancia nueva) y el Job de pgroles.go (instancia
// ya aprovisionada, donde el entrypoint nunca lo correrá)—, así que es
// IDEMPOTENTE: correrlo dos veces, o sobre una base ya migrada, no falla. Dos
// textos separados acabarían divergiendo, que es exactamente cómo nació este
// agujero. La única diferencia entre ambos caminos es el destino de la conexión,
// que viaja por PGHOST/PGPASSWORD (mecanismo nativo de libpq), no en el script.
//
// Sin `set -e` a propósito: si el fichero no llega ejecutable, el entrypoint lo
// hace `source` y un `set -e` contaminaría el resto del arranque; ON_ERROR_STOP ya
// aborta con código != 0.
const postgrestRolesSQL = `#!/bin/sh
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v pw="$` + authPasswordKey + `" <<'EOSQL'
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '` + postgrestAnonRole + `') THEN
    CREATE ROLE ` + postgrestAnonRole + ` NOLOGIN;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '` + postgrestRole + `') THEN
    CREATE ROLE ` + postgrestRole + ` LOGIN NOINHERIT;
  END IF;
END
$$;
ALTER ROLE ` + postgrestRole + ` PASSWORD :'pw';
GRANT ` + postgrestAnonRole + ` TO ` + postgrestRole + `;
GRANT USAGE ON SCHEMA public TO ` + postgrestAnonRole + `;
EOSQL
`

// initScriptMode es el modo del script montado en /docker-entrypoint-initdb.d:
// legible y EJECUTABLE por cualquiera (el pod corre con uid 999). Si no fuese
// ejecutable el entrypoint lo haría `source` en su propio shell.
var initScriptMode = int32(0o555)

// KubeProvisioner es la implementación de producción de ServiceProvisioner: crea
// los recursos de Kubernetes que materializan los servicios gestionados de Sytue
// Cloud en el namespace del tenant y devuelve las credenciales (en claro; el
// controller las cifra con AES-256-GCM antes de persistir).
//
//   - postgres: despliega una instancia Sytue Postgres (18) + PostgREST (API REST
//     automática) con un Secret de credenciales generadas, Deployment y Service.
//   - storage: aprovisiona un bucket S3 dedicado (delegado al plano de Object
//     Storage de Scaleway vía el Provisioner de almacenamiento; aquí se generan
//     las claves y se registra la configuración).
//   - auth: aísla por tenant/claim en Sytue ID (sin sub-issuer).
//
// Es la contraparte real del StubProvisioner de dev.
type KubeProvisioner struct {
	cs  kubernetes.Interface
	cfg config.Config
	// storage delega el aprovisionamiento de buckets S3 (Scaleway Object Storage).
	storage BucketProvisioner
	// creds resuelve las credenciales (descifradas, aplanadas por clave) de los
	// servicios ready del proyecto para inyectarlas en una función igual que en un
	// deployment normal (DATABASE_URL, S3_*). nil en dev/tests sin inyección.
	creds CredsResolver
}

// CredsResolver devuelve las credenciales (descifradas y aplanadas por clave, p.
// ej. DATABASE_URL, S3_*) de los servicios gestionados ready de un proyecto, para
// inyectarlas en una función igual que en un deployment. Lo construye
// NewServiceCredsResolver sobre el store + sealer.
//
// `target` es el destino del servicio que las pide: una función o un worker de
// vista previa reciben las credenciales de los servicios de vista previa, y los
// de producción las de producción.
type CredsResolver func(ctx context.Context, tenant string, projectID uuid.UUID, target domain.DeployTarget) (map[string]string, error)

// credsStore es la porción del store que lee las credenciales cifradas de los
// servicios ready de un proyecto (cross-tenant).
type credsStore interface {
	ResolveServiceCreds(ctx context.Context, tenant string, projectID uuid.UUID, target domain.DeployTarget) (map[string][]byte, error)
}

// NewServiceCredsResolver construye un CredsResolver sobre el store + sealer: lee
// las credenciales cifradas de los servicios ready del proyecto, las descifra y
// las aplana con la MISMA convención de claves que la inyección en deployments
// (injectServiceCreds), de modo que una función reciba DATABASE_URL/S3_* idénticas.
func NewServiceCredsResolver(st credsStore, sealer *crypto.Sealer) CredsResolver {
	return func(ctx context.Context, tenant string, projectID uuid.UUID, target domain.DeployTarget) (map[string]string, error) {
		encs, err := st.ResolveServiceCreds(ctx, tenant, projectID, target)
		if err != nil {
			return nil, err
		}
		out := make(map[string]string, len(encs)*4)
		for kind, enc := range encs {
			plain, derr := sealer.Open(enc)
			if derr != nil {
				return nil, fmt.Errorf("controller: descifrar credenciales de servicio: %w", derr)
			}
			injectServiceCreds(out, kind, plain)
		}
		return out, nil
	}
}

// BucketProvisioner crea/elimina buckets S3 dedicados por servicio de storage.
// En prod habla con la API de Object Storage de Scaleway; se inyecta para no
// acoplar el provisioner de K8s al SDK de S3.
type BucketProvisioner interface {
	CreateBucket(ctx context.Context, name, region string) (accessKey, secretKey string, err error)
	DeleteBucket(ctx context.Context, name, region string) error
}

// NewKubeProvisioner construye el provisioner sobre Kapsule. creds inyecta las
// credenciales de los servicios del proyecto en las funciones (nil = sin inyección).
func NewKubeProvisioner(cfg config.Config, storage BucketProvisioner, creds CredsResolver) (*KubeProvisioner, error) {
	restCfg, err := kubeRestConfig(cfg.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("controller: crear clientset (provisioner): %w", err)
	}
	return &KubeProvisioner{cs: cs, cfg: cfg, storage: storage, creds: creds}, nil
}

// newKubeProvisionerFor permite inyectar un clientset (fake en tests).
func newKubeProvisionerFor(cs kubernetes.Interface, cfg config.Config, storage BucketProvisioner, creds CredsResolver) *KubeProvisioner {
	return &KubeProvisioner{cs: cs, cfg: cfg, storage: storage, creds: creds}
}

func (p *KubeProvisioner) postgresImage() string {
	if p.cfg.ManagedPostgresImage != "" {
		return p.cfg.ManagedPostgresImage
	}
	return managedPostgresImage
}

func (p *KubeProvisioner) postgrestImage() string {
	if p.cfg.ManagedPostgrestImage != "" {
		return p.cfg.ManagedPostgrestImage
	}
	return managedPostgrestImage
}

func (p *KubeProvisioner) rcloneImage() string {
	if p.cfg.ManagedRcloneImage != "" {
		return p.cfg.ManagedRcloneImage
	}
	return managedRcloneImage
}

// Provision materializa un servicio gestionado. Implementa ServiceProvisioner.
func (p *KubeProvisioner) Provision(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	// Aísla el proyecto en la red ANTES de crear nada: es lo que deja que la app
	// (y DB Studio) alcancen esta base de datos sin abrir el namespace entero. Se
	// afirma también aquí, y no solo en ApplyApp, porque un proyecto puede tener
	// base de datos sin ninguna app desplegada.
	if err := ensureProjectNetworkPolicy(ctx, p.cs, "tenant-"+svc.Tenant, svc.Tenant, svc.ProjectID.String()); err != nil {
		return ProvisionResult{}, err
	}
	switch svc.Kind {
	case domain.ServicePostgres:
		return p.provisionPostgres(ctx, svc)
	case domain.ServiceStorage:
		return p.provisionStorage(ctx, svc)
	case domain.ServiceFunctions:
		return p.provisionFunctions(ctx, svc)
	case domain.ServiceCron:
		return p.provisionCron(ctx, svc)
	case domain.ServiceJob:
		return p.provisionJob(ctx, svc)
	case domain.ServiceWorker:
		return p.provisionWorker(ctx, svc)
	case domain.ServiceAuth:
		return p.provisionAuth(svc)
	case domain.ServiceApp:
		return p.provisionApp(ctx, svc)
	default:
		return ProvisionResult{}, fmt.Errorf("controller: tipo de servicio no soportado: %s", svc.Kind)
	}
}

// Deprovision elimina los recursos de un servicio gestionado. Implementa
// ServiceProvisioner.
func (p *KubeProvisioner) Deprovision(ctx context.Context, svc domain.ManagedService) error {
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	switch svc.Kind {
	case domain.ServicePostgres:
		bg := metav1.DeletePropagationBackground
		opts := metav1.DeleteOptions{PropagationPolicy: &bg}
		_ = p.cs.BatchV1().CronJobs(namespace).Delete(ctx, name+"-pg-backup", opts)
		_ = p.cs.BatchV1().Jobs(namespace).Delete(ctx, name+"-pg-restore", opts)
		_ = p.cs.NetworkingV1().Ingresses(namespace).Delete(ctx, name+"-postgrest", opts)
		_ = p.cs.AppsV1().Deployments(namespace).Delete(ctx, name+"-postgres", opts)
		_ = p.cs.AppsV1().Deployments(namespace).Delete(ctx, name+"-postgrest", opts)
		_ = p.cs.CoreV1().Services(namespace).Delete(ctx, name+"-postgres", opts)
		_ = p.cs.CoreV1().Services(namespace).Delete(ctx, name+"-postgrest", opts)
		_ = p.cs.CoreV1().Secrets(namespace).Delete(ctx, name+"-creds", opts)
		_ = p.cs.CoreV1().ConfigMaps(namespace).Delete(ctx, name+"-init", opts)
		_ = p.cs.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name+"-data", opts)
		return nil
	case domain.ServiceStorage:
		if p.storage != nil {
			bucket := storageBucketName(svc)
			if err := p.storage.DeleteBucket(ctx, bucket, p.cfg.S3Region); err != nil {
				return fmt.Errorf("controller: eliminar bucket: %w", err)
			}
		}
		return nil
	case domain.ServiceCron:
		bg := metav1.DeletePropagationBackground
		_ = p.cs.BatchV1().CronJobs(namespace).Delete(ctx, name+"-cron", metav1.DeleteOptions{PropagationPolicy: &bg})
		return nil
	case domain.ServiceJob:
		bg := metav1.DeletePropagationBackground
		_ = p.cs.BatchV1().Jobs(namespace).Delete(ctx, name+"-job", metav1.DeleteOptions{PropagationPolicy: &bg})
		return nil
	case domain.ServiceWorker:
		bg := metav1.DeletePropagationBackground
		opts := metav1.DeleteOptions{PropagationPolicy: &bg}
		_ = p.cs.AppsV1().Deployments(namespace).Delete(ctx, name+"-worker", opts)
		_ = p.cs.CoreV1().Secrets(namespace).Delete(ctx, name+"-worker-creds", opts)
		return nil
	case domain.ServiceApp:
		return p.deprovisionApp(ctx, namespace, name)
	case domain.ServiceFunctions:
		bg := metav1.DeletePropagationBackground
		opts := metav1.DeleteOptions{PropagationPolicy: &bg}
		_ = p.cs.NetworkingV1().Ingresses(namespace).Delete(ctx, name+"-fn", opts)
		_ = p.cs.AppsV1().Deployments(namespace).Delete(ctx, name+"-fn", opts)
		_ = p.cs.CoreV1().Services(namespace).Delete(ctx, name+"-fn", opts)
		_ = p.cs.CoreV1().Secrets(namespace).Delete(ctx, name+"-fn-creds", opts)
		return nil
	default:
		return nil
	}
}

// provisionCron materializa un batchv1.CronJob por servicio cron en el namespace
// del tenant desde svc.Config (schedule + image + command). PodSecurity
// 'restricted' (el namespace lo exige), concurrencyPolicy=Forbid (una ejecución a
// la vez), startingDeadlineSeconds y history limits acotados. La validación de
// schedule/image ocurre antes en la capa service (fail-closed pre-persistencia).
func (p *KubeProvisioner) provisionCron(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	labels := managedLabels(svc)
	schedule := svc.Config["schedule"]

	startDeadline := int64(120)
	backoff := int32(3)
	successHist := int32(3)
	failHist := int32(3)
	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-cron", Namespace: namespace, Labels: labels},
		Spec: batchv1.CronJobSpec{
			Schedule:                   schedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			StartingDeadlineSeconds:    &startDeadline,
			SuccessfulJobsHistoryLimit: &successHist,
			FailedJobsHistoryLimit:     &failHist,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: batchv1.JobSpec{
					BackoffLimit: &backoff,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec:       p.cronPodSpec(svc),
					},
				},
			},
		},
	}
	if err := p.upsertCronJob(ctx, namespace, cron); err != nil {
		return ProvisionResult{}, err
	}
	log.From(ctx).Info("CronJob gestionado aprovisionado", "service_id", svc.ID, "schedule", schedule)
	return ProvisionResult{Config: map[string]string{
		"scheduler": "kube-cronjob",
		"schedule":  schedule,
		"cronjob":   cron.Name,
	}}, nil
}

// cronPodSpec decide con qué corre la tarea: con la imagen del proyecto, o —si
// la tarea solo LLAMA A UNA URL— con la imagen de plataforma que trae curl.
//
// Lo segundo es el caso mayoritario (es como se usa Cloud Scheduler, y como
// llegan las tareas migradas desde Google) y antes obligaba a empaquetar una
// imagen propia para hacer un POST. La URL ya vino validada anti-SSRF por la
// capa de servicio: aquí solo se ejecuta.
func (p *KubeProvisioner) cronPodSpec(svc domain.ManagedService) corev1.PodSpec {
	target := strings.TrimSpace(svc.Config["url"])
	if target == "" {
		return managedTaskPodSpec(svc.Config["image"], shellCommand(svc.Config["command"]))
	}
	method := strings.ToUpper(strings.TrimSpace(svc.Config["method"]))
	if method == "" {
		method = "POST"
	}
	// --fail hace que un 5xx cuente como ejecución FALLIDA: sin él, una tarea que
	// golpea un endpoint roto se vería siempre en verde. Los reintentos los pone
	// el BackoffLimit del Job, no curl.
	args := []string{
		"--fail-with-body", "--silent", "--show-error", "--location",
		"--max-time", "300", "--request", method, target,
	}
	if body := svc.Config["body"]; body != "" {
		args = append(args, "--data-raw", body)
	}
	if ct := strings.TrimSpace(svc.Config["content_type"]); ct != "" {
		args = append(args, "--header", "Content-Type: "+ct)
	}
	spec := managedTaskPodSpec(p.cfg.HTTPTaskImage, nil)
	spec.Containers[0].Args = args
	return spec
}

// provisionJob materializa un batchv1.Job puntual run-to-completion desde
// svc.Config (image + command + parallelism/completions/backoff_limit/
// active_deadline_seconds), con el mismo SecurityContext 'restricted' del cron.
// Idempotente: si el Job ya existe no se re-aplica (su spec es inmutable). El
// estado de ejecución (running/succeeded/failed) se lee con JobState.
func (p *KubeProvisioner) provisionJob(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	labels := managedLabels(svc)

	parallelism := configInt32(svc.Config["parallelism"], 1, 1, 50)
	completions := configInt32(svc.Config["completions"], 1, 1, 50)
	backoff := configInt32(svc.Config["backoff_limit"], 3, 0, 20)
	ttl := int32(86400) // recolecta el Job + sus pods 24 h tras terminar (no retiene la cuota del tenant).
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-job", Namespace: namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			Parallelism:             &parallelism,
			Completions:             &completions,
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       managedTaskPodSpec(svc.Config["image"], shellCommand(svc.Config["command"])),
			},
		},
	}
	if d := configInt64(svc.Config["active_deadline_seconds"], 0, 86400); d > 0 {
		job.Spec.ActiveDeadlineSeconds = &d
	}
	if err := p.upsertJob(ctx, namespace, job); err != nil {
		return ProvisionResult{}, err
	}
	log.From(ctx).Info("Job gestionado aprovisionado", "service_id", svc.ID, "parallelism", parallelism)
	return ProvisionResult{Config: map[string]string{
		"runtime": "kube-job",
		"job":     job.Name,
		"state":   "running",
	}}, nil
}

// provisionWorker materializa un proceso PERMANENTE: un Deployment con la imagen
// del proyecto y su comando, SIN Service ni Ingress.
//
// Que no haya Service es la diferencia entera con una función: un worker no
// atiende peticiones, consume de una cola. Darle un puerto y una dirección para
// «uniformar» sería publicar en internet algo que nadie debe llamar.
//
// Las credenciales de los servicios del proyecto se inyectan igual que a una
// función —un consumidor necesita hablar con la base de datos tanto como una
// API—, y las réplicas salen de `config.replicas` con tope: es el único
// paralelismo que tiene sentido en un consumidor de cola.
func (p *KubeProvisioner) provisionWorker(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	labels := managedLabels(svc)
	replicas := configInt32(svc.Config["replicas"], 1, 0, 20)

	secretName := name + "-worker-creds"
	withSecret := false
	if p.creds != nil {
		injected, err := p.creds(ctx, svc.Tenant, svc.ProjectID, svc.Target)
		if err != nil {
			return ProvisionResult{}, fmt.Errorf("controller: resolver credenciales del worker: %w", err)
		}
		if len(injected) > 0 {
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace, Labels: labels},
				Type:       corev1.SecretTypeOpaque,
				StringData: injected,
			}
			if err := p.upsertSecret(ctx, namespace, sec); err != nil {
				return ProvisionResult{}, err
			}
			withSecret = true
		}
	}

	podSpec := managedTaskPodSpec(svc.Config["image"], shellCommand(svc.Config["command"]))
	// Un pod de Job termina; el de un worker no debe: `Never` en un Deployment lo
	// rechaza el API server, y además reiniciar es justo lo que se quiere cuando
	// un consumidor se cae.
	podSpec.RestartPolicy = corev1.RestartPolicyAlways
	if withSecret && len(podSpec.Containers) > 0 {
		podSpec.Containers[0].EnvFrom = append(podSpec.Containers[0].EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			},
		})
	}
	selector := map[string]string{"sytue.eu/managed": name + "-worker"}
	podLabels := mergeLabels(labels, selector)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-worker", Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec:       podSpec,
			},
		},
	}
	if err := p.upsertDeployment(ctx, namespace, dep); err != nil {
		return ProvisionResult{}, err
	}
	log.From(ctx).Info("Worker gestionado aprovisionado", "service_id", svc.ID, "replicas", replicas)
	return ProvisionResult{Config: map[string]string{
		"runtime":  "kube-deployment",
		"replicas": strconv.Itoa(int(replicas)),
		"state":    "running",
	}}, nil
}

// JobState lee el estado de ejecución de un servicio job (running/succeeded/
// failed) desde el batchv1.Job en el clúster.
//
// ponytail: primitiva de lectura, no cableada aún a un refresco periódico ni a un
// endpoint GET /services/{id}/status. Provision fija state=running en Config una
// vez; para reflejar el estado vivo por API, llamar a JobState desde el handler de
// cloud-api (ya tiene client-go en internal/runtime) o refrescar Config en
// reconcileServices. No se automatiza aquí: sin demanda de UI el poll es coste sin
// uso (YAGNI).
func (p *KubeProvisioner) JobState(ctx context.Context, svc domain.ManagedService) (string, error) {
	namespace := "tenant-" + svc.Tenant
	job, err := p.cs.BatchV1().Jobs(namespace).Get(ctx, svcResourceName(svc)+"-job", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("controller: estado Job: %w", err)
	}
	return jobState(job), nil
}

// jobState mapea el status de un batchv1.Job a running/succeeded/failed.
func jobState(job *batchv1.Job) string {
	if job.Status.Succeeded > 0 {
		return "succeeded"
	}
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return "failed"
		}
	}
	return "running"
}

// provisionFunctions materializa una función gestionada como un contenedor HTTP
// pequeño, reusando la maquinaria de despliegue (NO un runtime FaaS nuevo): un
// Deployment (PodSecurity 'restricted' como la app/cron), su Service ClusterIP y
// un Ingress bajo un host propio (<fn>.<projID>.fn.<appsdomain>). La imagen sale
// del pipeline de build del proyecto (o BYO de Wave 6) y se valida contra el repo
// del tenant en la capa service (ValidateImageRef). Las credenciales de los
// servicios del proyecto (DATABASE_URL, S3_*) se inyectan como en un deployment
// normal vía un Secret propio, para que la función hable con la BD/storage.
//
// ponytail: scale-to-zero (KEDA) DIFERIDO —minReplicas fijo a 1—; requiere KEDA en
// el clúster + ADR. Al añadirlo: anotar el Deployment y crear un ScaledObject.
func (p *KubeProvisioner) provisionFunctions(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	fnName := dnsSafe(svc.Name)
	labels := managedLabels(svc)
	host := HostnameDeFuncion(svc.ProjectID, fnName, p.cfg.AppsDomain)

	// Credenciales de los servicios del proyecto en un Secret propio (como un
	// deployment). Solo las del propio proyecto/tenant (ResolveServiceCreds está
	// acotado): sin secrets ajenos. Sin resolver (dev/tests) no se inyecta nada.
	secretName := name + "-fn-creds"
	withSecret := false
	if p.creds != nil {
		injected, err := p.creds(ctx, svc.Tenant, svc.ProjectID, svc.Target)
		if err != nil {
			return ProvisionResult{}, fmt.Errorf("controller: resolver credenciales de la función: %w", err)
		}
		if len(injected) > 0 {
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace, Labels: labels},
				Type:       corev1.SecretTypeOpaque,
				StringData: injected,
			}
			if err := p.upsertSecret(ctx, namespace, sec); err != nil {
				return ProvisionResult{}, err
			}
			withSecret = true
		}
	}

	dep := p.functionDeployment(name, namespace, svc.Config["image"], secretName, svc.Config["command"], labels, withSecret)
	if err := p.upsertDeployment(ctx, namespace, dep); err != nil {
		return ProvisionResult{}, err
	}
	if err := p.upsertService(ctx, namespace, p.serviceFor(name+"-fn", namespace, labels, name, "fn", 80, appContainerPort)); err != nil {
		return ProvisionResult{}, err
	}
	if err := p.ensureFunctionIngress(ctx, namespace, name, labels, host); err != nil {
		return ProvisionResult{}, err
	}

	state := "pending"
	if d, err := p.cs.AppsV1().Deployments(namespace).Get(ctx, name+"-fn", metav1.GetOptions{}); err == nil {
		state = functionState(d)
	}
	log.From(ctx).Info("Función gestionada aprovisionada", "service_id", svc.ID, "host", host)
	return ProvisionResult{Config: map[string]string{
		"runtime": "kube-deployment",
		"url":     "https://" + host,
		"host":    host,
		"state":   state,
	}}, nil
}

// functionDeployment construye el Deployment de una función: contenedor HTTP en
// appContainerPort con el mismo SecurityContext 'restricted' y recursos modestos
// que un pod de app, minReplicas fijo a 1 (scale-to-zero diferido). envFrom del
// Secret de credenciales del proyecto solo si withSecret (no referenciar un Secret
// inexistente haría fallar el arranque del pod).
func (p *KubeProvisioner) functionDeployment(name, namespace, image, secretName, command string, labels map[string]string, withSecret bool) *appsv1.Deployment {
	replicas := int32(1)
	runAsNonRoot := true
	readOnlyRoot := true
	allowPrivEsc := false
	automountSA := false
	runAsUser := int64(1000)
	tmpSize := resource.MustParse("256Mi")
	selector := map[string]string{"sytue.eu/managed": name + "-fn"}
	podLabels := mergeLabels(labels, selector)

	c := corev1.Container{
		Name:  "function",
		Image: image,
		Ports: []corev1.ContainerPort{{ContainerPort: appContainerPort, Name: "http"}},
		Env:   []corev1.EnvVar{{Name: "TMPDIR", Value: "/tmp"}},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &runAsNonRoot,
			RunAsUser:                &runAsUser,
			AllowPrivilegeEscalation: &allowPrivEsc,
			ReadOnlyRootFilesystem:   &readOnlyRoot,
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
				corev1.ResourceEphemeralStorage: resource.MustParse("512Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}},
		// TCP-socket (no HTTP GET /): una función HTTP genérica puede servir solo un
		// sub-path o devolver no-2xx en «/»; con GET / nunca quedaría Ready (Service sin
		// endpoints, 503). TCP comprueba que el puerto escucha = la función arrancó.
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(appContainerPort)},
			},
			InitialDelaySeconds: 3, PeriodSeconds: 5, FailureThreshold: 6,
		},
	}
	if cmd := shellCommand(command); len(cmd) > 0 {
		c.Command = cmd
	}
	if withSecret {
		c.EnvFrom = []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}},
		}}
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-fn", Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					RuntimeClassName:             tenantRuntimeClassPtr(),
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             tenantImagePullSecrets(),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &runAsNonRoot,
						RunAsUser:      &runAsUser,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{c},
					Volumes: []corev1.Volume{{
						Name:         "tmp",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &tmpSize}},
					}},
				},
			},
		},
	}
}

// ensureFunctionIngress enruta el host propio de la función a su Service con TLS
// gestionado por cert-manager (cluster-issuer), reusando el patrón de applyIngress.
func (p *KubeProvisioner) ensureFunctionIngress(ctx context.Context, namespace, name string, labels map[string]string, host string) error {
	pathType := networkingv1.PathTypePrefix
	className := "traefik"
	svcName := name + "-fn"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"traefik.ingress.kubernetes.io/router.entrypoints": "websecure",
				"traefik.ingress.kubernetes.io/router.tls":         "true",
				"cert-manager.io/cluster-issuer":                   p.cfg.CertIssuer,
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &className,
			TLS: []networkingv1.IngressTLS{{
				Hosts:      []string{host},
				SecretName: "tls-" + dnsSafe(host),
			}},
			Rules: []networkingv1.IngressRule{{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: svcName,
									Port: networkingv1.ServiceBackendPort{Number: 80},
								},
							},
						}},
					},
				},
			}},
		},
	}
	return p.upsertIngress(ctx, namespace, ing)
}

// FunctionState lee el estado (running/failed/pending) del Deployment de una
// función gestionada. Primitiva de lectura como JobState (no cableada a un poll).
func (p *KubeProvisioner) FunctionState(ctx context.Context, svc domain.ManagedService) (string, error) {
	namespace := "tenant-" + svc.Tenant
	dep, err := p.cs.AppsV1().Deployments(namespace).Get(ctx, svcResourceName(svc)+"-fn", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("controller: estado de función: %w", err)
	}
	return functionState(dep), nil
}

// functionState mapea el status del Deployment de una función a running/failed/
// pending: hay réplica disponible ⇒ running; ReplicaFailure=True ⇒ failed; si no,
// aún arrancando ⇒ pending.
func functionState(dep *appsv1.Deployment) string {
	if dep.Status.AvailableReplicas > 0 {
		return "running"
	}
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue {
			return "failed"
		}
	}
	return "pending"
}

// managedTaskPodSpec construye el PodSpec de un cron/job gestionado con
// SecurityContext 'restricted' (runAsNonRoot, seccomp RuntimeDefault, drop ALL,
// sin escalado de privilegios) —el namespace del tenant lo exige o el admission
// rechaza el pod—, sin token de ServiceAccount y con requests/limits modestos
// dentro del LimitRange del tenant. command vacío usa el entrypoint de la imagen.
func managedTaskPodSpec(image string, command []string) corev1.PodSpec {
	runAsNonRoot := true
	allowPrivEsc := false
	automountSA := false
	uid := int64(1000)
	tmpSize := resource.MustParse("256Mi")
	c := corev1.Container{
		Name:  "task",
		Image: image,
		Env:   []corev1.EnvVar{{Name: "TMPDIR", Value: "/tmp"}},
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
				corev1.ResourceCPU:              resource.MustParse("1"),
				corev1.ResourceMemory:           resource.MustParse("512Mi"),
				corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}},
	}
	if len(command) > 0 {
		c.Command = command
	}
	return corev1.PodSpec{
		RestartPolicy:                corev1.RestartPolicyOnFailure,
		RuntimeClassName:             tenantRuntimeClassPtr(),
		AutomountServiceAccountToken: &automountSA,
		ImagePullSecrets:             tenantImagePullSecrets(),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot:   &runAsNonRoot,
			RunAsUser:      &uid,
			RunAsGroup:     &uid,
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Containers: []corev1.Container{c},
		Volumes: []corev1.Volume{{
			Name:         "tmp",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &tmpSize}},
		}},
	}
}

// shellCommand envuelve un comando de shell en `sh -c` (patrón scheduled-task de
// Coolify/Scaleway Jobs). Vacío ⇒ nil (usa el entrypoint de la imagen).
func shellCommand(cmd string) []string {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	return []string{"sh", "-c", cmd}
}

// configInt32 parses a configuration integer with a default and clamps it to
// [lo, hi]. Typed bounds guarantee that the result fits in int32. Parse in 64
// bits so large values clamp consistently on both 32- and 64-bit hosts.
func configInt32(s string, def, lo, hi int32) int32 {
	n := int64(def)
	if s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			n = v
		}
	}
	if n < int64(lo) {
		return lo
	}
	if n > int64(hi) {
		return hi
	}
	return int32(n)
}

// configInt64 lee un entero positivo de config con default, acotado a [1, hi]; 0
// si default es 0 y no hay valor (sin límite de deadline).
func configInt64(s string, def, hi int) int64 {
	n := def
	if s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			n = v
		}
	}
	if n <= 0 {
		return int64(def)
	}
	if n > hi {
		n = hi
	}
	return int64(n)
}

// provisionPostgres despliega Sytue Postgres + PostgREST en el namespace del
// tenant con credenciales generadas al azar, y devuelve DATABASE_URL/POSTGREST_*.
func (p *KubeProvisioner) provisionPostgres(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	dbName := fmt.Sprintf("svc_%s", shortID(svc.ID.String()))
	user := "app"
	password, err := randToken(24)
	if err != nil {
		return ProvisionResult{}, err
	}
	// Contraseña PROPIA del rol de conexión de PostgREST: nunca la del dueño.
	restPassword, err := randToken(24)
	if err != nil {
		return ProvisionResult{}, err
	}
	jwtSecret, err := randToken(32)
	if err != nil {
		return ProvisionResult{}, err
	}

	pgHost := name + "-postgres." + namespace + ".svc.cluster.local"
	dsn := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", user, password, pgHost, dbName)
	restDSN := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", postgrestRole, restPassword, pgHost, dbName)
	restURL := fmt.Sprintf("http://%s-postgrest.%s.svc.cluster.local:3000", name, namespace)

	pvcSize := svc.Config["pvc_size"]
	if pvcSize == "" {
		pvcSize = "5Gi"
	}
	retentionDays := 7
	if d := svc.Config["backup_retention_days"]; d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			retentionDays = n
		}
	}
	publicREST := configTruthy(svc.Config["public_rest"])
	labels := managedLabels(svc)

	// Secret con las credenciales (montado por Postgres y PostgREST). PGRST_DB_URI
	// es la del rol 'authenticator', NO la del dueño: PostgREST no debe poder tocar
	// nada que el cliente no haya concedido explícitamente.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name + "-creds", Namespace: namespace, Labels: labels,
			// Nace ya marcada: la reparación de instancias heredadas (pgroles.go) la
			// salta sin tocarla.
			Annotations: map[string]string{pgRolesAnnotation: pgRolesVersion},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"POSTGRES_DB":       dbName,
			"POSTGRES_USER":     user,
			"POSTGRES_PASSWORD": password,
			restDSNKey:          restDSN,
			"PGRST_JWT_SECRET":  jwtSecret,
			authPasswordKey:     restPassword,
		},
	}
	if err := p.upsertSecret(ctx, namespace, secret); err != nil {
		return ProvisionResult{}, err
	}

	// Script de arranque que crea authenticator/web_anon. En un ConfigMap (no lleva
	// secreto: la contraseña la lee del entorno) montado en
	// /docker-entrypoint-initdb.d del pod de Postgres.
	initCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-init", Namespace: namespace, Labels: labels},
		Data:       map[string]string{postgrestInitScript: postgrestRolesSQL},
	}
	if err := p.upsertConfigMap(ctx, namespace, initCM); err != nil {
		return ProvisionResult{}, err
	}

	// PersistentVolumeClaim para los datos de Postgres.
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-data", Namespace: namespace, Labels: labels},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(pvcSize)},
			},
		},
	}
	if err := p.upsertPVC(ctx, namespace, pvc); err != nil {
		return ProvisionResult{}, err
	}

	// Deployment de Postgres 18.
	pgDep := p.postgresDeployment(name, namespace, labels)
	if err := p.upsertDeployment(ctx, namespace, pgDep); err != nil {
		return ProvisionResult{}, err
	}
	if err := p.upsertService(ctx, namespace, p.serviceFor(name+"-postgres", namespace, labels, name, "postgres", 5432, 5432)); err != nil {
		return ProvisionResult{}, err
	}

	// Deployment de PostgREST (API REST automática sobre el esquema).
	restDep := p.postgrestDeployment(name, namespace, labels)
	if err := p.upsertDeployment(ctx, namespace, restDep); err != nil {
		return ProvisionResult{}, err
	}
	if err := p.upsertService(ctx, namespace, p.serviceFor(name+"-postgrest", namespace, labels, name, "postgrest", 3000, 3000)); err != nil {
		return ProvisionResult{}, err
	}

	if err := p.ensurePostgresBackup(ctx, namespace, name, labels, svc, retentionDays); err != nil {
		return ProvisionResult{}, err
	}
	if publicREST {
		host := fmt.Sprintf("%s.db.%s", svc.ProjectID, p.cfg.AppsDomain)
		if err := p.ensurePostgRESTIngress(ctx, namespace, name, labels, host); err != nil {
			return ProvisionResult{}, err
		}
	}

	log.From(ctx).Info("Sytue Postgres + PostgREST aprovisionados", "service_id", svc.ID, "namespace", namespace)

	creds, _ := json.Marshal(map[string]string{
		"DATABASE_URL":  dsn,
		"POSTGREST_URL": restURL,
		"PGRST_JWT":     jwtSecret,
	})
	restPublic := fmt.Sprintf("https://%s.db.%s", svc.ProjectID, p.cfg.AppsDomain)
	if !publicREST {
		restPublic = restURL
	}
	return ProvisionResult{
		Config: map[string]string{
			"engine":      "postgres",
			"version":     "18",
			"database":    dbName,
			"rest_url":    restPublic,
			"pooling":     "true",
			"backups":     fmt.Sprintf("daily:%dd", retentionDays),
			"branching":   "enabled",
			"public_rest": strconv.FormatBool(publicREST),
		},
		Credentials: creds,
	}, nil
}

// provisionStorage crea un bucket S3 dedicado (Scaleway Object Storage) y
// devuelve las claves de acceso.
func (p *KubeProvisioner) provisionStorage(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	bucket := storageBucketName(svc)
	var accessKey, secretKey string
	if p.storage != nil {
		ak, sk, err := p.storage.CreateBucket(ctx, bucket, p.cfg.S3Region)
		if err != nil {
			return ProvisionResult{}, fmt.Errorf("controller: crear bucket: %w", err)
		}
		accessKey, secretKey = ak, sk
	} else {
		ak, err := randToken(16)
		if err != nil {
			return ProvisionResult{}, err
		}
		sk, err := randToken(32)
		if err != nil {
			return ProvisionResult{}, err
		}
		accessKey, secretKey = "SCW"+ak, sk
	}
	creds, _ := json.Marshal(map[string]string{
		"S3_ENDPOINT":   p.cfg.S3Endpoint,
		"S3_REGION":     p.cfg.S3Region,
		"S3_BUCKET":     bucket,
		"S3_ACCESS_KEY": accessKey,
		"S3_SECRET_KEY": secretKey,
	})
	return ProvisionResult{
		Config:      map[string]string{"bucket": bucket, "region": p.cfg.S3Region},
		Credentials: creds,
	}, nil
}

// provisionAuth aísla por tenant/claim en Sytue ID (sin sub-issuer; issuer nativo).
func (p *KubeProvisioner) provisionAuth(svc domain.ManagedService) (ProvisionResult, error) {
	tenant := fmt.Sprintf("project-%s", svc.ProjectID)
	clientSecret, err := randToken(32)
	if err != nil {
		return ProvisionResult{}, err
	}
	creds, _ := json.Marshal(map[string]string{
		"OIDC_ISSUER":        p.cfg.OIDCIssuer,
		"OIDC_CLIENT_ID":     "app-" + shortID(svc.ID.String()),
		"OIDC_CLIENT_SECRET": clientSecret,
		"OIDC_TENANT":        tenant,
	})
	return ProvisionResult{
		Config:      map[string]string{"tenant": tenant, "provider": "sytue-id"},
		Credentials: creds,
	}, nil
}

// ---------------------------------------------------------------------------
// Construcción de recursos
// ---------------------------------------------------------------------------

func (p *KubeProvisioner) postgresDeployment(name, namespace string, labels map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	runAsNonRoot := true
	allowPrivEsc := false
	runAsUser := int64(999) // usuario postgres de la imagen oficial
	fsGroup := int64(999)
	automountSA := false
	selector := map[string]string{"sytue.eu/managed": name + "-postgres"}
	podLabels := mergeLabels(labels, selector)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-postgres", Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					RuntimeClassName:             tenantRuntimeClassPtr(),
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             tenantImagePullSecrets(),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &runAsNonRoot,
						RunAsUser:      &runAsUser,
						FSGroup:        &fsGroup,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name:  "postgres",
						Image: p.postgresImage(),
						Ports: []corev1.ContainerPort{{ContainerPort: 5432, Name: "postgres"}},
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: name + "-creds"},
							},
						}},
						Env: []corev1.EnvVar{{Name: "PGDATA", Value: "/var/lib/postgresql/data/pgdata"}},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             &runAsNonRoot,
							AllowPrivilegeEscalation: &allowPrivEsc,
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
							SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:              resource.MustParse("100m"),
								corev1.ResourceMemory:           resource.MustParse("256Mi"),
								corev1.ResourceEphemeralStorage: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:              resource.MustParse("1"),
								corev1.ResourceMemory:           resource.MustParse("1Gi"),
								corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "data", MountPath: "/var/lib/postgresql/data"},
							{Name: "init", MountPath: "/docker-entrypoint-initdb.d", ReadOnly: true},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{Command: []string{"pg_isready", "-U", "app"}},
							},
							InitialDelaySeconds: 5, PeriodSeconds: 10,
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name + "-data"},
							},
						},
						{
							// Roles de PostgREST: el entrypoint de la imagen oficial corre
							// los ficheros de este directorio al inicializar el clúster.
							// 0555 para que los ejecute en vez de hacerles `source`.
							Name: "init",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: name + "-init"},
									DefaultMode:          &initScriptMode,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (p *KubeProvisioner) postgrestDeployment(name, namespace string, labels map[string]string) *appsv1.Deployment {
	replicas := int32(2)
	runAsNonRoot := true
	readOnlyRoot := true
	allowPrivEsc := false
	runAsUser := int64(1000)
	automountSA := false
	selector := map[string]string{"sytue.eu/managed": name + "-postgrest"}
	podLabels := mergeLabels(labels, selector)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-postgrest", Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
					// Cambiar el Secret NO reinicia los pods (envFrom se inyecta al
					// arrancar): sin esta anotación, un PostgREST heredado seguiría
					// corriendo con el DSN del dueño aunque el Secret ya estuviera
					// migrado. En el pod template de una instancia heredada no existe, así
					// que aplicarla fuerza el rollout UNA vez; reaplicarla no cambia nada.
					Annotations: map[string]string{pgRolesAnnotation: pgRolesVersion},
				},
				Spec: corev1.PodSpec{
					RuntimeClassName:             tenantRuntimeClassPtr(),
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             tenantImagePullSecrets(),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &runAsNonRoot,
						RunAsUser:      &runAsUser,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name:  "postgrest",
						Image: p.postgrestImage(),
						Ports: []corev1.ContainerPort{{ContainerPort: 3000, Name: "http"}},
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: name + "-creds"},
							},
						}},
						Env: []corev1.EnvVar{
							{Name: "PGRST_DB_SCHEMA", Value: "public"},
							// Rol de las peticiones SIN JWT. Era "app" —el dueño de la base
							// de datos—, así que la ruta anónima tenía CRUD completo y el
							// PGRST_JWT_SECRET no defendía nada.
							{Name: "PGRST_DB_ANON_ROLE", Value: postgrestAnonRole},
							{Name: "PGRST_SERVER_PORT", Value: "3000"},
						},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             &runAsNonRoot,
							AllowPrivilegeEscalation: &allowPrivEsc,
							ReadOnlyRootFilesystem:   &readOnlyRoot,
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
								corev1.ResourceEphemeralStorage: resource.MustParse("512Mi"),
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstr.FromInt32(3000)},
							},
							InitialDelaySeconds: 3, PeriodSeconds: 5,
						},
					}},
				},
			},
		},
	}
}

func (p *KubeProvisioner) serviceFor(name, namespace string, labels map[string]string, base, comp string, port, target int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"sytue.eu/managed": base + "-" + comp},
			Ports: []corev1.ServicePort{{
				Name: comp, Port: port, TargetPort: intstr.FromInt32(target), Protocol: corev1.ProtocolTCP,
			}},
		},
	}
}

// ---------------------------------------------------------------------------
// upserts idempotentes
// ---------------------------------------------------------------------------

func (p *KubeProvisioner) upsertSecret(ctx context.Context, ns string, s *corev1.Secret) error {
	api := p.cs.CoreV1().Secrets(ns)
	existing, err := api.Get(ctx, s.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, s, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Secret gestionado")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Secret gestionado: %w", err)
	}
	s.ResourceVersion = existing.ResourceVersion
	_, uerr := api.Update(ctx, s, metav1.UpdateOptions{})
	if uerr != nil {
		return fmt.Errorf("controller: actualizar Secret gestionado: %w", uerr)
	}
	return nil
}

func (p *KubeProvisioner) upsertConfigMap(ctx context.Context, ns string, cm *corev1.ConfigMap) error {
	api := p.cs.CoreV1().ConfigMaps(ns)
	existing, err := api.Get(ctx, cm.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, cm, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear ConfigMap gestionado")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar ConfigMap gestionado: %w", err)
	}
	cm.ResourceVersion = existing.ResourceVersion
	if _, uerr := api.Update(ctx, cm, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar ConfigMap gestionado: %w", uerr)
	}
	return nil
}

func (p *KubeProvisioner) upsertPVC(ctx context.Context, ns string, pvc *corev1.PersistentVolumeClaim) error {
	api := p.cs.CoreV1().PersistentVolumeClaims(ns)
	_, err := api.Get(ctx, pvc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, pvc, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear PVC")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar PVC: %w", err)
	}
	return nil // PVCs son en gran medida inmutables; no se re-aplica
}

func (p *KubeProvisioner) upsertDeployment(ctx context.Context, ns string, dep *appsv1.Deployment) error {
	api := p.cs.AppsV1().Deployments(ns)
	existing, err := api.Get(ctx, dep.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, dep, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Deployment gestionado")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Deployment gestionado: %w", err)
	}
	dep.ResourceVersion = existing.ResourceVersion
	_, uerr := api.Update(ctx, dep, metav1.UpdateOptions{})
	if uerr != nil {
		return fmt.Errorf("controller: actualizar Deployment gestionado: %w", uerr)
	}
	return nil
}

func (p *KubeProvisioner) upsertService(ctx context.Context, ns string, svc *corev1.Service) error {
	api := p.cs.CoreV1().Services(ns)
	existing, err := api.Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, svc, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Service gestionado")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Service gestionado: %w", err)
	}
	svc.ResourceVersion = existing.ResourceVersion
	svc.Spec.ClusterIP = existing.Spec.ClusterIP
	_, uerr := api.Update(ctx, svc, metav1.UpdateOptions{})
	if uerr != nil {
		return fmt.Errorf("controller: actualizar Service gestionado: %w", uerr)
	}
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func svcResourceName(svc domain.ManagedService) string {
	return "svc-" + shortID(svc.ID.String())
}

// storageBucketName deriva el nombre del bucket S3 dedicado del servicio a partir
// del ID COMPLETO del servicio (UUID sin guiones), no de shortID(): shortID recorta
// a 10 hex (~40 bits) y el espacio de nombres de bucket S3 es GLOBAL, así que dos
// tenants podían colisionar y Deprovision borrar el bucket ajeno. El UUID íntegro es
// único y estable. El resultado (prefijo "sytue-app-" + 32 hex minúsculas = 42 chars)
// cumple SIEMPRE las reglas de nombre S3 (3-63 chars, [a-z0-9-], empieza/acaba en
// alfanumérico), por lo que no requiere validación en runtime.
func storageBucketName(svc domain.ManagedService) string {
	return "sytue-app-" + removeDashes(svc.ID.String())
}

func managedLabels(svc domain.ManagedService) map[string]string {
	labels := managedLabelsBase(svc)
	// Solo los servicios de vista previa llevan la etiqueta de destino. Así los
	// de producción que ya existen conservan EXACTAMENTE sus etiquetas —añadirles
	// una cambiaría la plantilla de sus pods y reiniciaría la base de datos de
	// cada cliente al desplegar esto— y quien busca «el Postgres del proyecto»
	// puede excluir los de vista previa con `sytue.eu/target!=preview`, que
	// también casa con los que no tienen la etiqueta.
	if svc.Target == domain.TargetPreview {
		labels[targetLabel] = string(domain.TargetPreview)
	}
	return labels
}

// targetLabel marca los recursos de un servicio gestionado de vista previa.
const targetLabel = "sytue.eu/target"

func managedLabelsBase(svc domain.ManagedService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of":    "sytue-one",
		"app.kubernetes.io/managed-by": "sytue-cloud",
		"sytue.eu/component":           "cloud-managed-service",
		"sytue.eu/tenant":              svc.Tenant,
		"sytue.eu/service-kind":        string(svc.Kind),
		// Id del servicio: es lo que permite pedir los logs de ESTA función y no
		// de la de al lado. Sin él, dos funciones del mismo proyecto comparten
		// todas las etiquetas y no hay selector que las distinga.
		"sytue.eu/service-id": svc.ID.String(),
		// Clave de emparejamiento con la app del mismo proyecto: sin ella la
		// NetworkPolicy del proyecto no selecciona estos pods y la app no llega a su
		// base de datos.
		projectLabel: svc.ProjectID.String(),
	}
}

func mergeLabels(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// randToken genera un token aleatorio url-safe de n bytes de entropía. Devuelve
// error si el CSPRNG del SO falla: NUNCA se debe continuar con un secreto
// constante/predecible (passwords de DB, JWT y OIDC client secret dependen de él).
func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("controller: generar entropía segura: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func configTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func s3EndpointURL(cfg config.Config) string {
	scheme := "https"
	if !cfg.S3UseSSL {
		scheme = "http"
	}
	return scheme + "://" + strings.TrimPrefix(cfg.S3Endpoint, "https://")
}

// ensurePostgresBackup programa un CronJob diario pg_dump→Object Storage.
func (p *KubeProvisioner) ensurePostgresBackup(ctx context.Context, namespace, name string, labels map[string]string, svc domain.ManagedService, retentionDays int) error {
	if p.cfg.S3Bucket == "" || p.cfg.S3BackupAccessKey == "" || p.cfg.S3BackupSecretKey == "" {
		return nil // sin credenciales S3 dedicadas de backup: no copiar claves de plataforma al namespace
	}
	if err := p.upsertPlatformS3Secret(ctx, namespace, labels); err != nil {
		return err
	}
	prefix := fmt.Sprintf("backups/%s/%s", svc.Tenant, shortID(svc.ID.String()))
	// rclone y no `mc`: MinIO archivó `mc` en noviembre de 2025 y además es
	// AGPL-3.0, prohibida por el firewall clean-room. rclone es MIT, se mantiene
	// activamente y hace lo mismo. Se configura por VARIABLES DE ENTORNO
	// (RCLONE_CONFIG_SCW_*), así que desaparece el fichero ~/.mc y con él el
	// apaño de HOME=/work que existía solo para poder escribirlo.
	uploadScript := fmt.Sprintf(`set -e
export RCLONE_CONFIG_SCW_TYPE=s3
export RCLONE_CONFIG_SCW_PROVIDER=Other
export RCLONE_CONFIG_SCW_ENDPOINT="$S3_ENDPOINT"
export RCLONE_CONFIG_SCW_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export RCLONE_CONFIG_SCW_SECRET_ACCESS_KEY="$S3_SECRET_KEY"
obj="%s/$(date -u +%%Y%%m%%dT%%H%%M%%SZ).sql.gz"
rclone copyto /work/backup.sql.gz "scw:$S3_BUCKET/$obj"
rclone delete --min-age %dd "scw:$S3_BUCKET/%s/" 2>/dev/null || true
`, prefix, retentionDays, prefix)

	// El namespace del tenant fuerza PodSecurity 'restricted' (kubeplatform.go): sin
	// estos SecurityContext el admission RECHAZA el pod y los backups nunca corren.
	// UID 999 = usuario postgres de la imagen oficial (pg_dump corre non-root); rclone
	// corre con ese mismo UID (no requiere ninguno concreto). FSGroup 999 hace el
	// emptyDir /work escribible por ambos contenedores. AutomountServiceAccountToken
	// false: el pod de backup no necesita la API de K8s y así no puede enumerar
	// Secrets del namespace (defensa en profundidad sobre las claves S3 de backup).
	runAsNonRoot := true
	allowPrivEsc := false
	backupUID := int64(999)
	automountSA := false
	podSC := &corev1.PodSecurityContext{
		RunAsNonRoot:   &runAsNonRoot,
		RunAsUser:      &backupUID,
		RunAsGroup:     &backupUID,
		FSGroup:        &backupUID,
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
	containerSC := &corev1.SecurityContext{
		RunAsNonRoot:             &runAsNonRoot,
		AllowPrivilegeEscalation: &allowPrivEsc,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}

	// Programación elegida por el cliente, con el defecto de siempre cuando no la
	// hay: a las 3 de la mañana. No se valida aquí —lo hace la capa service con el
	// mismo parser que un cron normal—, pero sí se cae al defecto si llegara vacía:
	// un CronJob sin schedule lo rechaza el API server y el servicio se quedaría
	// sin copias sin que nadie se entere.
	schedule := strings.TrimSpace(svc.Config["backup_schedule"])
	if schedule == "" {
		schedule = domain.DefaultBackupSchedule
	}

	workSize := resource.MustParse("5Gi")
	jobResources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("50m"),
			corev1.ResourceMemory:           resource.MustParse("128Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("1"),
			corev1.ResourceMemory:           resource.MustParse("512Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("5Gi"),
		},
	}

	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-pg-backup", Namespace: namespace, Labels: labels},
		Spec: batchv1.CronJobSpec{
			Schedule: schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyOnFailure,
							RuntimeClassName:             tenantRuntimeClassPtr(),
							AutomountServiceAccountToken: &automountSA,
							ImagePullSecrets:             tenantImagePullSecrets(),
							SecurityContext:              podSC,
							InitContainers: []corev1.Container{{
								Name:  "dump",
								Image: p.postgresImage(),
								Command: []string{
									"sh", "-c",
									// --clean --if-exists: el dump emite DROP ... IF EXISTS antes de
									// cada CREATE, así el restore (psql -v ON_ERROR_STOP=1) es
									// re-aplicable sobre una DB viva (esquema+datos ya presentes),
									// que es el caso normal de recuperación; sin esto psql aborta
									// en el primer 'relation already exists'.
									"PGPASSWORD=\"$POSTGRES_PASSWORD\" pg_dump --clean --if-exists -h " + name + "-postgres -U \"$POSTGRES_USER\" \"$POSTGRES_DB\" | gzip > /work/backup.sql.gz",
								},
								EnvFrom: []corev1.EnvFromSource{{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: name + "-creds"},
									},
								}},
								SecurityContext: containerSC,
								Resources:       jobResources,
								VolumeMounts:    []corev1.VolumeMount{{Name: "work", MountPath: "/work"}},
							}},
							Containers: []corev1.Container{{
								Name:    "upload",
								Image:   p.rcloneImage(),
								Command: []string{"sh", "-c", uploadScript},
								EnvFrom: []corev1.EnvFromSource{{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: "sytue-platform-s3"},
									},
								}},
								SecurityContext: containerSC,
								Resources:       jobResources,
								VolumeMounts:    []corev1.VolumeMount{{Name: "work", MountPath: "/work"}},
							}},
							Volumes: []corev1.Volume{{
								Name: "work",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &workSize},
								},
							}},
						},
					},
				},
			},
		},
	}
	return p.upsertCronJob(ctx, namespace, cron)
}

// BackupPostgresNow dispara un backup INMEDIATO del Sytue Postgres del servicio.
//
// # Clona el CronJob en vez de repetirlo
//
// La copia diaria ya está descrita en `<name>-pg-backup` con su script, su
// SecurityContext, su Secret de credenciales y su retención. Escribir aquí un Job
// equivalente habría duplicado todo eso —y el duplicado envejece: el día que
// cambie la imagen de rclone o el prefijo S3, el backup manual seguiría con lo
// viejo sin que nada fallara—. Es además lo que hace `kubectl create job
// --from=cronjob/...`, así que no es un truco: es el patrón de Kubernetes.
//
// Si nunca se creó el CronJob (instalación sin Object Storage de plataforma) no
// hay nada que clonar y se dice: un backup que no existe no puede quedar como
// «lanzado».
func (p *KubeProvisioner) BackupPostgresNow(ctx context.Context, svc domain.ManagedService) error {
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	cronName := name + "-pg-backup"

	cron, err := p.cs.BatchV1().CronJobs(namespace).Get(ctx, cronName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: este servicio no tiene copias configuradas", domain.ErrNotFound)
		}
		return fmt.Errorf("controller: leer CronJob de backup: %w", err)
	}

	// El sufijo con la hora hace el nombre único y además legible en `kubectl get
	// jobs`: quien mire la lista sabe cuándo se pidió cada copia.
	jobName := fmt.Sprintf("%s-manual-%s", cronName, time.Now().UTC().Format("20060102t150405"))
	ttl := int32(24 * 60 * 60) // el Job terminado se limpia solo al día siguiente
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        jobName,
			Namespace:   namespace,
			Labels:      cron.Spec.JobTemplate.Labels,
			Annotations: map[string]string{"cronjob.kubernetes.io/instantiate": "manual"},
		},
		Spec: *cron.Spec.JobTemplate.Spec.DeepCopy(),
	}
	job.Spec.TTLSecondsAfterFinished = &ttl
	if _, err := p.cs.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return ignoreAlreadyExists(err, "crear Job de backup manual")
	}
	log.From(ctx).Info("Copia de seguridad lanzada a mano", "service_id", svc.ID, "job", jobName)
	return nil
}

// RunServiceNow lanza AHORA una tarea que normalmente espera: la programada, a
// su hora; la puntual, a que alguien la vuelva a crear.
//
// Sin esto, probar un cron recién escrito era esperar a la hora o retocar el
// horario para que cayera pronto y devolverlo después —con el riesgo evidente de
// dejarlo mal—, y repetir un job terminado obligaba a borrarlo y recrearlo,
// perdiendo su historial. Es el «Run now» de Coolify.
//
// Los dos casos crean un Job nuevo con nombre propio en vez de tocar el objeto
// existente: la spec de un Job es INMUTABLE, y reprogramar el CronJob para
// forzar una ejecución cambiaría la configuración que el usuario guardó.
func (p *KubeProvisioner) RunServiceNow(ctx context.Context, svc domain.ManagedService) error {
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	// El sufijo con la hora hace el nombre único y legible en `kubectl get jobs`:
	// mismo criterio que la copia manual de Postgres.
	stamp := time.Now().UTC().Format("20060102t150405")
	ttl := int32(24 * 60 * 60)

	var spec batchv1.JobSpec
	var labels map[string]string
	switch svc.Kind {
	case domain.ServiceCron:
		cron, err := p.cs.BatchV1().CronJobs(namespace).Get(ctx, name+"-cron", metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("%w: la tarea aún no está aprovisionada", domain.ErrNotFound)
			}
			return fmt.Errorf("controller: leer CronJob: %w", err)
		}
		spec = *cron.Spec.JobTemplate.Spec.DeepCopy()
		labels = cron.Spec.JobTemplate.Labels
	case domain.ServiceJob:
		prev, err := p.cs.BatchV1().Jobs(namespace).Get(ctx, name+"-job", metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("%w: la tarea aún no está aprovisionada", domain.ErrNotFound)
			}
			return fmt.Errorf("controller: leer Job: %w", err)
		}
		spec = *prev.Spec.DeepCopy()
		labels = prev.Labels
		// Un Job ya materializado trae el selector y las etiquetas que le puso el
		// controlador de batch. Copiarlas tal cual crea un Job cuyos pods pertenecen
		// —según las etiquetas— al Job viejo: el API server lo rechaza, y si no lo
		// hiciera sería peor (dos Jobs peleándose por los mismos pods).
		spec.Selector = nil
		spec.ManualSelector = nil
		for _, k := range []string{
			"controller-uid", "job-name",
			"batch.kubernetes.io/controller-uid", "batch.kubernetes.io/job-name",
		} {
			delete(spec.Template.Labels, k)
		}
	default:
		return fmt.Errorf("%w: solo se pueden ejecutar tareas programadas y puntuales", domain.ErrInvalidInput)
	}

	spec.TTLSecondsAfterFinished = &ttl
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-run-%s", name, stamp),
			Namespace:   namespace,
			Labels:      labels,
			Annotations: map[string]string{"cronjob.kubernetes.io/instantiate": "manual"},
		},
		Spec: spec,
	}
	if _, err := p.cs.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return ignoreAlreadyExists(err, "crear ejecución manual")
	}
	log.From(ctx).Info("Tarea lanzada a mano", "service_id", svc.ID, "job", job.Name)
	return nil
}

// RestorePostgres restaura un backup elegido sobre el Sytue Postgres del servicio
// (INVERSO de ensurePostgresBackup): un Job de UNA sola ejecución en el namespace
// del tenant que baja el objeto de Object Storage y lo re-inyecta con psql. Es
// DESTRUCTIVO. El objeto S3 se compone SIEMPRE en servidor a partir del prefijo
// propio del servicio (backups/<tenant>/<shortID>/) + el nombre de backup: el
// llamador no controla tenant/servicio/prefijo. Defensa en profundidad: se rechaza
// cualquier nombre con separadores de ruta o '..' (la capa service ya lo valida con
// patrón estricto; esto lo hace autoprotegido si se invoca directo).
//
// Está cableado de punta a punta: la capa service (RestoreServiceBackup) valida,
// audita y emite EventServiceRestore; el suscriptor de cloud-controller
// (cmd/cloud-controller/main.go) resuelve el servicio bajo el tenant del evento con
// controller.DispatchServiceRestore y llama aquí.
func (p *KubeProvisioner) RestorePostgres(ctx context.Context, svc domain.ManagedService, backupName string) error {
	if p.cfg.S3Bucket == "" || p.cfg.S3BackupAccessKey == "" || p.cfg.S3BackupSecretKey == "" {
		return fmt.Errorf("controller: restore sin credenciales S3 dedicadas de backup (S3_BACKUP_*)")
	}
	if strings.ContainsAny(backupName, `/\`) || strings.Contains(backupName, "..") {
		return fmt.Errorf("controller: nombre de backup inválido")
	}
	namespace := "tenant-" + svc.Tenant
	name := svcResourceName(svc)
	labels := managedLabels(svc)
	if err := p.upsertPlatformS3Secret(ctx, namespace, labels); err != nil {
		return err
	}
	// Objeto compuesto EN SERVIDOR: prefijo propio del servicio + nombre validado. Se
	// pasa por env (BACKUP_OBJECT), no interpolado en el texto del script (sin riesgo
	// de inyección de shell aunque el nombre llevase metacaracteres).
	obj := fmt.Sprintf("backups/%s/%s/%s", svc.Tenant, shortID(svc.ID.String()), backupName)
	downloadScript := `set -e
export RCLONE_CONFIG_SCW_TYPE=s3
export RCLONE_CONFIG_SCW_PROVIDER=Other
export RCLONE_CONFIG_SCW_ENDPOINT="$S3_ENDPOINT"
export RCLONE_CONFIG_SCW_ACCESS_KEY_ID="$S3_ACCESS_KEY"
export RCLONE_CONFIG_SCW_SECRET_ACCESS_KEY="$S3_SECRET_KEY"
rclone copyto "scw:$S3_BUCKET/$BACKUP_OBJECT" /work/backup.sql.gz
`
	restoreCmd := "gunzip -c /work/backup.sql.gz | PGPASSWORD=\"$POSTGRES_PASSWORD\" psql -v ON_ERROR_STOP=1 -h " +
		name + "-postgres -U \"$POSTGRES_USER\" \"$POSTGRES_DB\""

	// MISMA postura que el backup: el namespace del tenant fuerza PodSecurity
	// 'restricted'. UID 999 = usuario postgres de la imagen (psql corre non-root); rclone
	// corre con ese mismo UID. FSGroup 999 hace /work escribible por ambos. Sin token
	// de ServiceAccount: el pod no necesita la API de K8s (no puede enumerar Secrets).
	runAsNonRoot := true
	allowPrivEsc := false
	restoreUID := int64(999)
	automountSA := false
	backoff := int32(0) // sin reintentos: una restauración destructiva no se relanza sola.
	ttl := int32(86400) // recolecta el Job + pods 24 h tras terminar (no retiene cuota).
	podSC := &corev1.PodSecurityContext{
		RunAsNonRoot:   &runAsNonRoot,
		RunAsUser:      &restoreUID,
		RunAsGroup:     &restoreUID,
		FSGroup:        &restoreUID,
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
	containerSC := &corev1.SecurityContext{
		RunAsNonRoot:             &runAsNonRoot,
		AllowPrivilegeEscalation: &allowPrivEsc,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}

	workSize := resource.MustParse("5Gi")
	jobResources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("50m"),
			corev1.ResourceMemory:           resource.MustParse("128Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("1"),
			corev1.ResourceMemory:           resource.MustParse("512Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("5Gi"),
		},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-pg-restore", Namespace: namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					RuntimeClassName:             tenantRuntimeClassPtr(),
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             tenantImagePullSecrets(),
					SecurityContext:              podSC,
					// Descarga (rclone) como initContainer: corre a completitud ANTES de que
					// psql intente restaurar (garantiza que /work/backup.sql.gz existe).
					InitContainers: []corev1.Container{{
						Name:    "download",
						Image:   p.rcloneImage(),
						Command: []string{"sh", "-c", downloadScript},
						// Sin HOME: rclone se configura por entorno y no escribe fichero
						// alguno. Ese apaño existía solo para el ~/.mc de MinIO Client.
						Env: []corev1.EnvVar{{Name: "BACKUP_OBJECT", Value: obj}},
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "sytue-platform-s3"},
							},
						}},
						SecurityContext: containerSC,
						Resources:       jobResources,
						VolumeMounts:    []corev1.VolumeMount{{Name: "work", MountPath: "/work"}},
					}},
					Containers: []corev1.Container{{
						Name:    "restore",
						Image:   p.postgresImage(),
						Command: []string{"sh", "-c", restoreCmd},
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: name + "-creds"},
							},
						}},
						SecurityContext: containerSC,
						Resources:       jobResources,
						VolumeMounts:    []corev1.VolumeMount{{Name: "work", MountPath: "/work"}},
					}},
					Volumes: []corev1.Volume{{
						Name:         "work",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &workSize}},
					}},
				},
			},
		},
	}
	// Un Job es inmutable y run-to-completion: para permitir re-restaurar, se borra el
	// Job de restore previo (y sus pods) antes de crear el nuevo. Best-effort: si no
	// existe, el Delete es un no-op.
	// ponytail: el Delete es asíncrono (Background); un re-restore MUY seguido (antes de
	// que el Job previo se elimine) puede chocar y quedar en no-op por ignoreAlreadyExists.
	// Aceptable mientras el subscriber del evento de restore NO esté cableado (diferido a
	// Kapsule): al cablearlo, o el consumidor reintenta ante error (y devolvemos error en
	// AlreadyExists), o se usa borrado en foreground + espera acotada a NotFound. El caso
	// vivo hoy (primer restore) siempre crea el Job.
	bg := metav1.DeletePropagationBackground
	_ = p.cs.BatchV1().Jobs(namespace).Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &bg})
	if _, err := p.cs.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return ignoreAlreadyExists(err, "crear Job de restore")
	}
	return nil
}

// upsertPlatformS3Secret materializa las credenciales S3 que el CronJob de backup
// usa para subir el dump al bucket de backups. NUNCA se montan en el pod de la app
// (la app monta solo su Secret app-<id>, y sus pods corren con
// AutomountServiceAccountToken=false, así que no pueden leer este Secret vía la API
// de K8s). Solo el pod del CronJob lo referencia.
//
// Si ops ha acuñado una key ACOTADA al prefijo backups/ del bucket (S3_BACKUP_*),
// se usa esa; si no, cae a las claves de plataforma de alcance amplio.
//
// ponytail: residual acotado — sin S3_BACKUP_* el CronJob monta las claves de
// plataforma (amplias, pero solo en el pod del CronJob, jamás en la app). Cierre
// pleno: acuñar una key limitada a arn:aws:s3:::<bucket>/backups/* (vía Scaleway
// IAM, ya cableado en s3_bucket.go, o provista por ops) y fijar S3_BACKUP_*. No se
// automatiza aquí para no gestionar el ciclo de vida de una key compartida de
// backups (rotación/reconcile); ops la acuña una vez.
func (p *KubeProvisioner) upsertPlatformS3Secret(ctx context.Context, namespace string, labels map[string]string) error {
	accessKey, secretKey := p.cfg.S3BackupAccessKey, p.cfg.S3BackupSecretKey
	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("controller: S3_BACKUP_ACCESS_KEY y S3_BACKUP_SECRET_KEY son obligatorias para copias de seguridad")
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sytue-platform-s3", Namespace: namespace, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"S3_ENDPOINT":   s3EndpointURL(p.cfg),
			"S3_ACCESS_KEY": accessKey,
			"S3_SECRET_KEY": secretKey,
			"S3_BUCKET":     p.cfg.S3Bucket,
		},
	}
	return p.upsertSecret(ctx, namespace, sec)
}

func (p *KubeProvisioner) ensurePostgRESTIngress(ctx context.Context, namespace, name string, labels map[string]string, host string) error {
	pathType := networkingv1.PathTypePrefix
	className := "traefik"
	svcName := name + "-postgrest"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-postgrest",
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"traefik.ingress.kubernetes.io/router.entrypoints": "websecure",
				"traefik.ingress.kubernetes.io/router.tls":         "true",
				// Sin esta anotación cert-manager no emite nada y Traefik sirve su
				// certificado autofirmado: la única superficie que publica datos del
				// cliente a Internet era justo la que no tenía TLS válido.
				"cert-manager.io/cluster-issuer": p.cfg.CertIssuer,
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &className,
			TLS: []networkingv1.IngressTLS{{
				Hosts:      []string{host},
				SecretName: "tls-" + dnsSafe(host),
			}},
			Rules: []networkingv1.IngressRule{{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: svcName,
									Port: networkingv1.ServiceBackendPort{Number: 3000},
								},
							},
						}},
					},
				},
			}},
		},
	}
	return p.upsertIngress(ctx, namespace, ing)
}

func (p *KubeProvisioner) upsertCronJob(ctx context.Context, ns string, cj *batchv1.CronJob) error {
	api := p.cs.BatchV1().CronJobs(ns)
	existing, err := api.Get(ctx, cj.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, cj, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear CronJob")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar CronJob: %w", err)
	}
	cj.ResourceVersion = existing.ResourceVersion
	_, uerr := api.Update(ctx, cj, metav1.UpdateOptions{})
	if uerr != nil {
		return fmt.Errorf("controller: actualizar CronJob: %w", uerr)
	}
	return nil
}

// upsertJob crea el Job si no existe. Un batchv1.Job es run-to-completion y su
// spec (template/selector) es inmutable: si ya existe no se re-aplica (no se
// re-lanza una ejecución en curso o terminada en cada reconcile).
func (p *KubeProvisioner) upsertJob(ctx context.Context, ns string, job *batchv1.Job) error {
	api := p.cs.BatchV1().Jobs(ns)
	_, err := api.Get(ctx, job.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, job, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Job")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Job: %w", err)
	}
	return nil
}

func (p *KubeProvisioner) upsertIngress(ctx context.Context, ns string, ing *networkingv1.Ingress) error {
	api := p.cs.NetworkingV1().Ingresses(ns)
	existing, err := api.Get(ctx, ing.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, ing, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Ingress PostgREST")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Ingress PostgREST: %w", err)
	}
	ing.ResourceVersion = existing.ResourceVersion
	_, uerr := api.Update(ctx, ing, metav1.UpdateOptions{})
	if uerr != nil {
		return fmt.Errorf("controller: actualizar Ingress PostgREST: %w", uerr)
	}
	return nil
}
