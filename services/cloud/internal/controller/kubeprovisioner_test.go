// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// fakeBucketProv es un BucketProvisioner de test (sin S3 real).
type fakeBucketProv struct{ created, deleted string }

func (f *fakeBucketProv) CreateBucket(_ context.Context, name, _ string) (string, string, error) {
	f.created = name
	return "AKtest", "SKtest", nil
}

func (f *fakeBucketProv) DeleteBucket(_ context.Context, name, _ string) error {
	f.deleted = name
	return nil
}

func TestProvisionPostgresCreatesResources(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()

	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServicePostgres,
	}
	res, err := p.Provision(ctx, svc)
	if err != nil {
		t.Fatalf("Provision postgres: %v", err)
	}

	// Credenciales JSON con DATABASE_URL y POSTGREST_URL.
	var creds map[string]string
	if err := json.Unmarshal(res.Credentials, &creds); err != nil {
		t.Fatalf("credenciales no son JSON: %v", err)
	}
	if creds["DATABASE_URL"] == "" || creds["POSTGREST_URL"] == "" {
		t.Errorf("faltan credenciales postgres/postgrest: %v", creds)
	}
	if res.Config["engine"] != "postgres" || res.Config["version"] != "18" {
		t.Errorf("config de servicio inesperada: %v", res.Config)
	}

	ns := "tenant-acme"
	name := svcResourceName(svc)
	// Secret, PVC, Deployments y Services de postgres + postgrest.
	if _, err := cs.CoreV1().Secrets(ns).Get(ctx, name+"-creds", metav1.GetOptions{}); err != nil {
		t.Errorf("Secret de credenciales no creado: %v", err)
	}
	if _, err := cs.CoreV1().PersistentVolumeClaims(ns).Get(ctx, name+"-data", metav1.GetOptions{}); err != nil {
		t.Errorf("PVC no creado: %v", err)
	}
	pg, err := cs.AppsV1().Deployments(ns).Get(ctx, name+"-postgres", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment postgres no creado: %v", err)
	}
	if pg.Spec.Template.Spec.Containers[0].Image != managedPostgresImage {
		t.Errorf("imagen postgres inesperada: %s", pg.Spec.Template.Spec.Containers[0].Image)
	}
	if _, err := cs.AppsV1().Deployments(ns).Get(ctx, name+"-postgrest", metav1.GetOptions{}); err != nil {
		t.Errorf("Deployment postgrest no creado: %v", err)
	}
	if _, err := cs.CoreV1().Services(ns).Get(ctx, name+"-postgres", metav1.GetOptions{}); err != nil {
		t.Errorf("Service postgres no creado: %v", err)
	}
	if _, err := cs.CoreV1().Services(ns).Get(ctx, name+"-postgrest", metav1.GetOptions{}); err != nil {
		t.Errorf("Service postgrest no creado: %v", err)
	}

	// Idempotencia.
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision idempotente falló: %v", err)
	}
}

// D1 — PostgREST publicaba la base de datos del cliente con el rol DUEÑO: conectaba
// como "app" (el POSTGRES_USER del entrypoint) y además lo usaba de rol anónimo, así
// que la ruta sin JWT tenía CRUD completo sobre todo. Contrato: ni el DSN ni el rol
// anónimo pueden ser el dueño; PostgREST entra como 'authenticator' y resuelve a
// 'web_anon', que solo tiene USAGE sobre public.
func TestPostgRESTNoUsaElRolDueño(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServicePostgres,
	}
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision postgres: %v", err)
	}
	ns, name := "tenant-acme", svcResourceName(svc)

	dep, err := cs.AppsV1().Deployments(ns).Get(ctx, name+"-postgrest", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment postgrest: %v", err)
	}
	anon := ""
	for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
		if e.Value == "app" {
			t.Errorf("el Deployment de PostgREST no puede referirse al rol dueño: %s=%q", e.Name, e.Value)
		}
		if e.Name == "PGRST_DB_ANON_ROLE" {
			anon = e.Value
		}
	}
	if anon != postgrestAnonRole {
		t.Errorf("PGRST_DB_ANON_ROLE=%q, se esperaba %q", anon, postgrestAnonRole)
	}

	sec, err := cs.CoreV1().Secrets(ns).Get(ctx, name+"-creds", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret de credenciales: %v", err)
	}
	uri := sec.StringData["PGRST_DB_URI"]
	if !strings.HasPrefix(uri, "postgres://"+postgrestRole+":") {
		t.Errorf("PGRST_DB_URI debe conectar como %q, es %q", postgrestRole, uri)
	}
	restPwd := sec.StringData["PGRST_AUTHENTICATOR_PASSWORD"]
	if restPwd == "" || restPwd == sec.StringData["POSTGRES_PASSWORD"] {
		t.Errorf("'authenticator' debe tener contraseña propia, no la del dueño")
	}
	// DATABASE_URL de la app (el dueño) sigue siendo suya y distinta de la de PostgREST.
	if strings.Contains(uri, sec.StringData["POSTGRES_PASSWORD"]) {
		t.Errorf("el DSN de PostgREST no puede llevar la contraseña del dueño")
	}

	// Los roles se crean al inicializar el clúster: script en ConfigMap montado en
	// /docker-entrypoint-initdb.d del pod de Postgres.
	cm, err := cs.CoreV1().ConfigMaps(ns).Get(ctx, name+"-init", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("ConfigMap de inicialización no creado: %v", err)
	}
	script := cm.Data[postgrestInitScript]
	for _, want := range []string{
		"CREATE ROLE " + postgrestAnonRole + " NOLOGIN",
		"CREATE ROLE " + postgrestRole + " LOGIN NOINHERIT",
		"GRANT USAGE ON SCHEMA public TO " + postgrestAnonRole,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("el script de arranque debe contener %q:\n%s", want, script)
		}
	}
	// web_anon no puede nacer con SELECT sobre nada: lo publica el cliente.
	if strings.Contains(script, "GRANT SELECT") {
		t.Errorf("web_anon no debe recibir SELECT por defecto:\n%s", script)
	}

	pg, err := cs.AppsV1().Deployments(ns).Get(ctx, name+"-postgres", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment postgres: %v", err)
	}
	mounted := false
	for _, m := range pg.Spec.Template.Spec.Containers[0].VolumeMounts {
		if m.MountPath == "/docker-entrypoint-initdb.d" {
			mounted = true
		}
	}
	if !mounted {
		t.Errorf("el script de roles debe montarse en /docker-entrypoint-initdb.d")
	}

	// Deprovision se lleva también el ConfigMap.
	if err := p.Deprovision(ctx, svc); err != nil {
		t.Fatalf("Deprovision: %v", err)
	}
	if _, err := cs.CoreV1().ConfigMaps(ns).Get(ctx, name+"-init", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("el ConfigMap de init debe borrarse en Deprovision, err=%v", err)
	}
}

// D2 — el Ingress que publica PostgREST en Internet no pedía certificado, así que
// Traefik servía su autofirmado sobre la única superficie que expone datos.
func TestPostgRESTIngressPublicoPideCertificado(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServicePostgres,
		Config: map[string]string{"public_rest": "true"},
	}
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision postgres público: %v", err)
	}
	ing, err := cs.NetworkingV1().Ingresses("tenant-acme").Get(ctx, svcResourceName(svc)+"-postgrest", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Ingress de PostgREST no creado: %v", err)
	}
	if got := ing.Annotations["cert-manager.io/cluster-issuer"]; got != testCfg().CertIssuer {
		t.Errorf("cluster-issuer de cert-manager = %q, se esperaba %q", got, testCfg().CertIssuer)
	}
	if len(ing.Spec.TLS) == 0 {
		t.Errorf("el Ingress público debe declarar TLS")
	}
}

// D4 — los servicios gestionados deben llevar la etiqueta de proyecto y quedar
// cubiertos por la NetworkPolicy del proyecto; si no, la app nunca alcanza su base
// de datos (o la alcanzan también los demás proyectos del tenant).
func TestProvisionEtiquetaYAislaElProyecto(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	projID := uuid.New()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: projID, Kind: domain.ServicePostgres,
	}
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision postgres: %v", err)
	}
	pg, err := cs.AppsV1().Deployments("tenant-acme").Get(ctx, svcResourceName(svc)+"-postgres", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment postgres: %v", err)
	}
	if got := pg.Spec.Template.Labels[projectLabel]; got != projID.String() {
		t.Errorf("los pods del servicio deben llevar %s=%s, llevan %q", projectLabel, projID, got)
	}
	if _, err := cs.NetworkingV1().NetworkPolicies("tenant-acme").
		Get(ctx, projectPolicyName(projID.String()), metav1.GetOptions{}); err != nil {
		t.Errorf("la NetworkPolicy del proyecto debe existir tras aprovisionar: %v", err)
	}
}

func TestProvisionStorageUsesBucketProvisioner(t *testing.T) {
	cs := fake.NewSimpleClientset()
	bp := &fakeBucketProv{}
	p := newKubeProvisionerFor(cs, testCfg(), bp, nil)
	svc := domain.ManagedService{ID: uuid.New(), Tenant: "acme", Kind: domain.ServiceStorage}

	res, err := p.Provision(context.Background(), svc)
	if err != nil {
		t.Fatalf("Provision storage: %v", err)
	}
	if bp.created == "" {
		t.Errorf("no se creó el bucket")
	}
	var creds map[string]string
	_ = json.Unmarshal(res.Credentials, &creds)
	if creds["S3_ACCESS_KEY"] != "AKtest" || creds["S3_BUCKET"] != bp.created {
		t.Errorf("credenciales S3 incorrectas: %v", creds)
	}
}

func TestProvisionAuthNoSubIssuer(t *testing.T) {
	p := newKubeProvisionerFor(fake.NewSimpleClientset(), testCfg(), nil, nil)
	svc := domain.ManagedService{ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServiceAuth}
	res, err := p.Provision(context.Background(), svc)
	if err != nil {
		t.Fatalf("Provision auth: %v", err)
	}
	var creds map[string]string
	_ = json.Unmarshal(res.Credentials, &creds)
	if creds["OIDC_ISSUER"] != "https://id.sytue.eu" {
		t.Errorf("issuer nativo esperado, es %q", creds["OIDC_ISSUER"])
	}
	if res.Config["provider"] != "sytue-id" {
		t.Errorf("provider inesperado: %v", res.Config)
	}
}

func TestDeprovisionStorage(t *testing.T) {
	bp := &fakeBucketProv{}
	p := newKubeProvisionerFor(fake.NewSimpleClientset(), testCfg(), bp, nil)
	svc := domain.ManagedService{ID: uuid.New(), Tenant: "acme", Kind: domain.ServiceStorage}
	if err := p.Deprovision(context.Background(), svc); err != nil {
		t.Fatalf("Deprovision: %v", err)
	}
	if bp.deleted == "" {
		t.Errorf("no se eliminó el bucket")
	}
}

func TestProvisionUnsupported(t *testing.T) {
	p := newKubeProvisionerFor(fake.NewSimpleClientset(), testCfg(), nil, nil)
	svc := domain.ManagedService{ID: uuid.New(), Tenant: "acme", Kind: domain.ServiceKind("nope")}
	if _, err := p.Provision(context.Background(), svc); err == nil {
		t.Fatalf("un tipo no soportado debe error")
	}
}

// assertRestrictedTaskPod comprueba el SecurityContext 'restricted' de un pod de
// cron/job gestionado (el namespace del tenant lo exige o el admission lo rechaza).
func assertRestrictedTaskPod(t *testing.T, spec corev1.PodSpec) {
	t.Helper()
	psc := spec.SecurityContext
	if psc == nil || psc.RunAsNonRoot == nil || !*psc.RunAsNonRoot {
		t.Errorf("pod debe runAsNonRoot")
	}
	if psc == nil || psc.SeccompProfile == nil || psc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("pod debe seccomp RuntimeDefault")
	}
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Errorf("pod no debe automontar token de ServiceAccount")
	}
	sc := spec.Containers[0].SecurityContext
	if sc == nil {
		t.Fatalf("container sin SecurityContext")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Errorf("allowPrivilegeEscalation debe ser false")
	}
	if sc.Capabilities == nil || len(sc.Capabilities.Drop) == 0 || sc.Capabilities.Drop[0] != "ALL" {
		t.Errorf("container debe drop ALL capabilities")
	}
	if sc.SeccompProfile == nil || sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("container debe seccomp RuntimeDefault")
	}
}

func TestProvisionCronCreatesCronJob(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServiceCron,
		Config: map[string]string{
			"schedule": "*/5 * * * *",
			"image":    "rg.fr-par.scw.cloud/sytue/task@sha256:abc",
			"command":  "echo hola",
		},
	}
	res, err := p.Provision(ctx, svc)
	if err != nil {
		t.Fatalf("Provision cron: %v", err)
	}
	if res.Config["schedule"] != "*/5 * * * *" || res.Config["scheduler"] != "kube-cronjob" {
		t.Errorf("config de cron inesperada: %v", res.Config)
	}

	ns, cjName := "tenant-acme", svcResourceName(svc)+"-cron"
	cj, err := cs.BatchV1().CronJobs(ns).Get(ctx, cjName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("CronJob no creado: %v", err)
	}
	if cj.Spec.Schedule != "*/5 * * * *" {
		t.Errorf("schedule del CronJob: %q", cj.Spec.Schedule)
	}
	if cj.Spec.ConcurrencyPolicy != batchv1.ForbidConcurrent {
		t.Errorf("concurrencyPolicy debe ser Forbid, got %q", cj.Spec.ConcurrencyPolicy)
	}
	if cj.Spec.StartingDeadlineSeconds == nil ||
		cj.Spec.SuccessfulJobsHistoryLimit == nil || cj.Spec.FailedJobsHistoryLimit == nil {
		t.Errorf("faltan startingDeadlineSeconds/history limits")
	}
	if cj.Spec.JobTemplate.Spec.BackoffLimit == nil {
		t.Errorf("falta backoffLimit")
	}
	if cmd := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command; len(cmd) != 3 || cmd[0] != "sh" || cmd[2] != "echo hola" {
		t.Errorf("command del cron inesperado: %v", cmd)
	}
	if img := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image; img != svc.Config["image"] {
		t.Errorf("image del cron: %q", img)
	}
	assertRestrictedTaskPod(t, cj.Spec.JobTemplate.Spec.Template.Spec)

	// Idempotencia.
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision cron idempotente: %v", err)
	}
	// Deprovision borra el CronJob.
	if err := p.Deprovision(ctx, svc); err != nil {
		t.Fatalf("Deprovision cron: %v", err)
	}
	if _, err := cs.BatchV1().CronJobs(ns).Get(ctx, cjName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("el CronJob debe borrarse en Deprovision, err=%v", err)
	}
}

func TestProvisionJobCreatesJobAndState(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", Kind: domain.ServiceJob,
		Config: map[string]string{
			"image":                   "rg.fr-par.scw.cloud/sytue/batch:v1",
			"command":                 "run.sh",
			"parallelism":             "3",
			"completions":             "3",
			"active_deadline_seconds": "600",
		},
	}
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision job: %v", err)
	}

	ns, jobName := "tenant-acme", svcResourceName(svc)+"-job"
	job, err := cs.BatchV1().Jobs(ns).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Job no creado: %v", err)
	}
	if job.Spec.Parallelism == nil || *job.Spec.Parallelism != 3 {
		t.Errorf("parallelism: %v", job.Spec.Parallelism)
	}
	if job.Spec.Completions == nil || *job.Spec.Completions != 3 {
		t.Errorf("completions: %v", job.Spec.Completions)
	}
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 600 {
		t.Errorf("activeDeadlineSeconds: %v", job.Spec.ActiveDeadlineSeconds)
	}
	assertRestrictedTaskPod(t, job.Spec.Template.Spec)

	// Estado leible desde el clúster: sin status ⇒ running.
	if st, err := p.JobState(ctx, svc); err != nil || st != "running" {
		t.Errorf("JobState inicial = running, got %q err=%v", st, err)
	}
	job.Status.Succeeded = 1
	if _, err := cs.BatchV1().Jobs(ns).Update(ctx, job, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("actualizar status del Job: %v", err)
	}
	if st, _ := p.JobState(ctx, svc); st != "succeeded" {
		t.Errorf("JobState = succeeded esperado, got %q", st)
	}

	if err := p.Deprovision(ctx, svc); err != nil {
		t.Fatalf("Deprovision job: %v", err)
	}
	if _, err := cs.BatchV1().Jobs(ns).Get(ctx, jobName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("el Job debe borrarse en Deprovision, err=%v", err)
	}
}

func TestJobStateMapping(t *testing.T) {
	if jobState(&batchv1.Job{}) != "running" {
		t.Errorf("job sin status debe ser running")
	}
	if jobState(&batchv1.Job{Status: batchv1.JobStatus{Succeeded: 1}}) != "succeeded" {
		t.Errorf("Succeeded>0 debe ser succeeded")
	}
	failed := &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{
		{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
	}}}
	if jobState(failed) != "failed" {
		t.Errorf("condición JobFailed=True debe ser failed")
	}
}

func TestProvisionFunctionsCreatesDeploymentServiceIngress(t *testing.T) {
	cs := fake.NewSimpleClientset()
	// Resolver de credenciales del proyecto (DATABASE_URL/S3), como un deployment.
	resolver := func(_ context.Context, tenant string, _ uuid.UUID, _ domain.DeployTarget) (map[string]string, error) {
		if tenant != "acme" {
			t.Errorf("resolver llamado con tenant ajeno: %q", tenant)
		}
		return map[string]string{"DATABASE_URL": "postgres://app@db/x", "S3_BUCKET": "b"}, nil
	}
	p := newKubeProvisionerFor(cs, testCfg(), nil, resolver)
	ctx := context.Background()
	projID := uuid.New()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: projID, Kind: domain.ServiceFunctions,
		Name:   "api",
		Config: map[string]string{"image": "rg.fr-par.scw.cloud/sytue/web:v1"},
	}
	res, err := p.Provision(ctx, svc)
	if err != nil {
		t.Fatalf("Provision functions: %v", err)
	}
	// Un solo segmento bajo `fn.` y el UUID DELANTE: es lo que permite cubrirlo
	// todo con `*.fn.<dominio>` —ACME no emite comodines de varios niveles— y lo
	// que hace fijo el corte entre proyecto y función, porque el UUID mide
	// siempre 36 caracteres. Con la función delante, dos pares distintos podrían
	// dar el mismo hostname y enrutar al cliente equivocado.
	wantHost := projID.String() + "-api.fn.apps.sytue.eu"
	if res.Config["host"] != wantHost || res.Config["runtime"] != "kube-deployment" {
		t.Errorf("config de función inesperada: %v (host esperado %s)", res.Config, wantHost)
	}

	ns, base := "tenant-acme", svcResourceName(svc)
	dep, err := cs.AppsV1().Deployments(ns).Get(ctx, base+"-fn", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment de función no creado: %v", err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas < 1 {
		t.Errorf("minReplicas>=1 esperado, got %v", dep.Spec.Replicas)
	}
	if img := dep.Spec.Template.Spec.Containers[0].Image; img != svc.Config["image"] {
		t.Errorf("imagen de función: %q", img)
	}
	if port := dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort; port != appContainerPort {
		t.Errorf("puerto HTTP de función = %d, esperado %d", port, appContainerPort)
	}
	assertRestrictedTaskPod(t, dep.Spec.Template.Spec)

	// Credenciales del proyecto inyectadas vía Secret propio + envFrom.
	if _, err := cs.CoreV1().Secrets(ns).Get(ctx, base+"-fn-creds", metav1.GetOptions{}); err != nil {
		t.Errorf("Secret de credenciales de función no creado: %v", err)
	}
	ef := dep.Spec.Template.Spec.Containers[0].EnvFrom
	if len(ef) != 1 || ef[0].SecretRef == nil || ef[0].SecretRef.Name != base+"-fn-creds" {
		t.Errorf("la función debe envFrom su Secret de credenciales: %v", ef)
	}

	// Service + Ingress bajo host propio.
	if _, err := cs.CoreV1().Services(ns).Get(ctx, base+"-fn", metav1.GetOptions{}); err != nil {
		t.Errorf("Service de función no creado: %v", err)
	}
	ing, err := cs.NetworkingV1().Ingresses(ns).Get(ctx, base+"-fn", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Ingress de función no creado: %v", err)
	}
	if ing.Spec.Rules[0].Host != wantHost {
		t.Errorf("host del Ingress: %q, esperado %q", ing.Spec.Rules[0].Host, wantHost)
	}

	// Estado legible desde el Deployment.
	if st, err := p.FunctionState(ctx, svc); err != nil || st != "pending" {
		t.Errorf("FunctionState inicial = pending (sin réplicas), got %q err=%v", st, err)
	}
	dep.Status.AvailableReplicas = 1
	if _, err := cs.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("actualizar status del Deployment: %v", err)
	}
	if st, _ := p.FunctionState(ctx, svc); st != "running" {
		t.Errorf("FunctionState = running esperado, got %q", st)
	}

	// Idempotencia.
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision función idempotente: %v", err)
	}

	// Deprovision limpia Deployment+Service+Ingress+Secret.
	if err := p.Deprovision(ctx, svc); err != nil {
		t.Fatalf("Deprovision función: %v", err)
	}
	if _, err := cs.AppsV1().Deployments(ns).Get(ctx, base+"-fn", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("el Deployment debe borrarse en Deprovision, err=%v", err)
	}
	if _, err := cs.NetworkingV1().Ingresses(ns).Get(ctx, base+"-fn", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("el Ingress debe borrarse en Deprovision, err=%v", err)
	}
	if _, err := cs.CoreV1().Services(ns).Get(ctx, base+"-fn", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("el Service debe borrarse en Deprovision, err=%v", err)
	}
}

// TestProvisionFunctionsNoResolverSkipsSecret verifica que sin resolver de
// credenciales (dev) la función se despliega sin Secret ni envFrom (no referencia
// un Secret inexistente, que haría fallar el arranque del pod).
func TestProvisionFunctionsNoResolverSkipsSecret(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServiceFunctions,
		Name: "api", Config: map[string]string{"image": "rg.fr-par.scw.cloud/sytue/web:v1"},
	}
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision functions sin resolver: %v", err)
	}
	ns, base := "tenant-acme", svcResourceName(svc)
	dep, err := cs.AppsV1().Deployments(ns).Get(ctx, base+"-fn", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment de función no creado: %v", err)
	}
	if ef := dep.Spec.Template.Spec.Containers[0].EnvFrom; len(ef) != 0 {
		t.Errorf("sin resolver no debe haber envFrom, got %v", ef)
	}
	if _, err := cs.CoreV1().Secrets(ns).Get(ctx, base+"-fn-creds", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("sin resolver no debe crearse Secret de credenciales, err=%v", err)
	}
}

// TestCadaServiceSeleccionaSusPods ata las dos mitades de un emparejamiento que
// vive en funciones distintas y no falla en voz alta cuando se separan: el
// `Selector` del Service y las etiquetas del pod que lo sirve.
//
// Es la clase de defecto que ya costó cara una vez —allow-project-<id>
// emparejaba por `sytue.eu/project-id` y los pods de app no la llevaban, así que
// la app no alcanzaba su base de datos—. Kubernetes no protesta: un Service sin
// endpoints simplemente no responde, y el fallo aparece a kilómetros, como un
// timeout de conexión.
//
// Este test no compara cadenas: EVALÚA el selector de cada Service contra las
// etiquetas reales de los pods que se crearon en el mismo aprovisionamiento.
func TestCadaServiceSeleccionaSusPods(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()

	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServicePostgres,
	}
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision postgres: %v", err)
	}

	ns := "tenant-acme"
	deps, err := cs.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("listar deployments: %v", err)
	}
	if len(deps.Items) == 0 {
		t.Fatal("el aprovisionamiento no creó ningún Deployment")
	}
	svcs, err := cs.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("listar services: %v", err)
	}
	if len(svcs.Items) == 0 {
		t.Fatal("el aprovisionamiento no creó ningún Service")
	}

	for i := range svcs.Items {
		s := &svcs.Items[i]
		if len(s.Spec.Selector) == 0 {
			continue // un ExternalName no selecciona pods
		}
		sel := labels.SelectorFromSet(s.Spec.Selector)
		matched := false
		for j := range deps.Items {
			if sel.Matches(labels.Set(deps.Items[j].Spec.Template.Labels)) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("el Service %q no selecciona NINGÚN pod de los que se crearon.\n"+
				"  selector: %s\n"+
				"Un Service sin endpoints no da error: simplemente no responde, y el fallo "+
				"aparece lejos, como un timeout de conexión.", s.Name, sel.String())
		}
	}
}
