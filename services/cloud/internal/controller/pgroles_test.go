// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

const (
	legacyOwnerPwd = "duenio-no-tocar"
	legacyJWT      = "jwt-no-tocar"
)

// seedLegacyPostgres deja en el fake el estado EXACTO de una instancia
// aprovisionada antes del arreglo: Secret sin PGRST_AUTHENTICATOR_PASSWORD (con el
// DSN del dueño en PGRST_DB_URI) y PostgREST corriendo con PGRST_DB_ANON_ROLE=app,
// sin anotación de migración en el pod template.
func seedLegacyPostgres(t *testing.T, p *KubeProvisioner, cs *fake.Clientset, ns, name string) map[string]string {
	t.Helper()
	ctx := context.Background()
	tenant := strings.TrimPrefix(ns, "tenant-")
	_, _ = cs.CoreV1().Namespaces().Create(ctx, managedTenantNamespace(tenant), metav1.CreateOptions{})
	labels := managedLabels(domain.ManagedService{
		Tenant: tenant, ProjectID: uuid.New(), Kind: domain.ServicePostgres,
	})
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-creds", Namespace: ns, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"POSTGRES_DB":       []byte("svc_deadbeef"),
			"POSTGRES_USER":     []byte("app"),
			"POSTGRES_PASSWORD": []byte(legacyOwnerPwd),
			"PGRST_JWT_SECRET":  []byte(legacyJWT),
			restDSNKey: []byte("postgres://app:" + legacyOwnerPwd + "@" + name +
				"-postgres." + ns + ".svc.cluster.local:5432/svc_deadbeef?sslmode=disable"),
		},
	}
	if _, err := cs.CoreV1().Secrets(ns).Create(ctx, sec, metav1.CreateOptions{}); err != nil {
		t.Fatalf("sembrar Secret heredado: %v", err)
	}
	dep := p.postgrestDeployment(name, ns, labels)
	dep.Spec.Template.Annotations = nil // heredado: sin marca de migración
	for i, e := range dep.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "PGRST_DB_ANON_ROLE" {
			dep.Spec.Template.Spec.Containers[0].Env[i].Value = "app" // el rol DUEÑO
		}
	}
	if _, err := cs.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		t.Fatalf("sembrar Deployment heredado: %v", err)
	}
	return labels
}

// (a)+(c)+(d): una instancia heredada se repara entera —Secret parcheado, Job de
// roles creado con SQL idempotente y sin la contraseña en el texto, y PostgREST
// reaplicado con web_anon MÁS la anotación que fuerza el rollout— sin tocar ni la
// contraseña del dueño ni el secreto JWT.
func TestRepairPostgresRolesMigraInstanciaHeredada(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	const ns, name = "tenant-acme", "svc-abc1234567"
	seedLegacyPostgres(t, p, cs, ns, name)

	if err := p.RepairPostgresRoles(ctx); err != nil {
		t.Fatalf("RepairPostgresRoles: %v", err)
	}

	// Secret: DSN de authenticator con contraseña propia; (d) el resto intacto.
	sec, err := cs.CoreV1().Secrets(ns).Get(ctx, name+"-creds", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret tras reparar: %v", err)
	}
	pwd := string(sec.Data[authPasswordKey])
	if pwd == "" {
		t.Fatal("no se generó contraseña para el rol authenticator")
	}
	uri := string(sec.Data[restDSNKey])
	if !strings.HasPrefix(uri, "postgres://"+postgrestRole+":") {
		t.Errorf("%s debe conectar como %q, es %q", restDSNKey, postgrestRole, uri)
	}
	if strings.Contains(uri, legacyOwnerPwd) || !strings.Contains(uri, pwd) {
		t.Errorf("el DSN de PostgREST debe llevar SU contraseña, no la del dueño: %q", uri)
	}
	if !strings.HasSuffix(uri, "@"+name+"-postgres."+ns+".svc.cluster.local:5432/svc_deadbeef?sslmode=disable") {
		t.Errorf("el DSN reescrito perdió host/base de datos: %q", uri)
	}
	if string(sec.Data["POSTGRES_PASSWORD"]) != legacyOwnerPwd || string(sec.Data["PGRST_JWT_SECRET"]) != legacyJWT {
		t.Error("la reparación no puede tocar POSTGRES_PASSWORD ni PGRST_JWT_SECRET: rompería la app del cliente")
	}
	if sec.Annotations[pgRolesAnnotation] != pgRolesVersion {
		t.Errorf("el Secret debe quedar marcado como migrado, anotaciones=%v", sec.Annotations)
	}

	// Job de roles: SQL idempotente, contraseña interpolada por psql (nunca en el texto).
	job, err := cs.BatchV1().Jobs(ns).Get(ctx, name+"-pgroles", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Job de roles no creado: %v", err)
	}
	c := job.Spec.Template.Spec.Containers[0]
	sql := strings.Join(c.Command, " ")
	for _, want := range []string{
		"IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '" + postgrestAnonRole + "')",
		"IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '" + postgrestRole + "')",
		"ALTER ROLE " + postgrestRole + " PASSWORD :'pw'",
		"GRANT " + postgrestAnonRole + " TO " + postgrestRole,
		"GRANT USAGE ON SCHEMA public TO " + postgrestAnonRole,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("el SQL del Job debe contener %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, pwd) || strings.Contains(sql, legacyOwnerPwd) {
		t.Error("ninguna contraseña puede concatenarse en el texto del SQL")
	}
	if strings.Contains(sql, "GRANT SELECT") {
		t.Errorf("web_anon no debe recibir SELECT: lo concede el cliente:\n%s", sql)
	}
	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyOnFailure ||
		*job.Spec.BackoffLimit < 5 || job.Spec.TTLSecondsAfterFinished == nil {
		t.Error("el Job debe tolerar que Postgres tarde en levantar y recogerse solo")
	}
	if am := job.Spec.Template.Spec.AutomountServiceAccountToken; am == nil || *am {
		t.Error("el pod del Job no debe montar el token de ServiceAccount")
	}
	// La contraseña del dueño llega por referencia al Secret, no como valor.
	pgpass := envByName(c.Env, "PGPASSWORD")
	if pgpass == nil || pgpass.Value != "" || pgpass.ValueFrom == nil ||
		pgpass.ValueFrom.SecretKeyRef.Key != "POSTGRES_PASSWORD" {
		t.Errorf("PGPASSWORD debe venir de la clave POSTGRES_PASSWORD del Secret: %+v", pgpass)
	}

	// PostgREST: rol anónimo web_anon y anotación que fuerza el rollout (sin ella los
	// pods vivos seguirían con el DSN del dueño que leyeron al arrancar).
	dep, err := cs.AppsV1().Deployments(ns).Get(ctx, name+"-postgrest", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment de PostgREST: %v", err)
	}
	anon := envByName(dep.Spec.Template.Spec.Containers[0].Env, "PGRST_DB_ANON_ROLE")
	if anon == nil || anon.Value != postgrestAnonRole {
		t.Errorf("PGRST_DB_ANON_ROLE=%v, se esperaba %q", anon, postgrestAnonRole)
	}
	if dep.Spec.Template.Annotations[pgRolesAnnotation] != pgRolesVersion {
		t.Errorf("falta la anotación de rollout en el pod template: %v", dep.Spec.Template.Annotations)
	}
}

// (b) Idempotencia: una instancia ya migrada no provoca NI UNA escritura, y correr
// el paso dos veces no recrea el Job ni vuelve a rotar la contraseña.
func TestRepairPostgresRolesEsIdempotente(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	const ns, name = "tenant-acme", "svc-abc1234567"
	seedLegacyPostgres(t, p, cs, ns, name)

	if err := p.RepairPostgresRoles(ctx); err != nil {
		t.Fatalf("RepairPostgresRoles (1ª): %v", err)
	}
	sec, err := cs.CoreV1().Secrets(ns).Get(ctx, name+"-creds", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret migrado: %v", err)
	}
	job, err := cs.BatchV1().Jobs(ns).Get(ctx, name+"-pgroles", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Job de roles: %v", err)
	}

	cs.ClearActions()
	if err := p.RepairPostgresRoles(ctx); err != nil {
		t.Fatalf("RepairPostgresRoles (2ª): %v", err)
	}
	for _, a := range cs.Actions() {
		if a.GetVerb() != "list" {
			t.Errorf("una instancia ya migrada no debe provocar escrituras: %s %s", a.GetVerb(), a.GetResource().Resource)
		}
	}
	sec2, err := cs.CoreV1().Secrets(ns).Get(ctx, name+"-creds", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret tras 2ª pasada: %v", err)
	}
	if !bytes.Equal(sec2.Data[authPasswordKey], sec.Data[authPasswordKey]) ||
		!bytes.Equal(sec2.Data[restDSNKey], sec.Data[restDSNKey]) {
		t.Error("la 2ª pasada rotó credenciales de una instancia ya migrada")
	}
	job2, err := cs.BatchV1().Jobs(ns).Get(ctx, name+"-pgroles", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Job tras 2ª pasada: %v", err)
	}
	if job2.UID != job.UID || job2.CreationTimestamp != job.CreationTimestamp {
		t.Error("el Job de roles se recreó: la migración debe correr exactamente una vez")
	}
}

// Una instancia aprovisionada por el camino normal nace ya marcada: el paso de
// reparación no la toca (ni un Get). Y el Secret de plataforma, que arrastra las
// mismas etiquetas gestionadas, tampoco se confunde con una instancia heredada.
func TestRepairPostgresRolesIgnoraInstanciasNuevasYSecretosAjenos(t *testing.T) {
	cs := fake.NewSimpleClientset(managedTenantNamespace("acme"))
	cfg := testCfg()
	cfg.S3BackupAccessKey = "AK-backup"
	cfg.S3BackupSecretKey = "SK-backup"
	p := newKubeProvisionerFor(cs, cfg, nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServicePostgres}
	if _, err := p.Provision(ctx, svc); err != nil {
		t.Fatalf("Provision postgres: %v", err)
	}
	// Secret de plataforma con etiquetas de servicio postgres (lo crea el backup).
	if err := p.upsertPlatformS3Secret(ctx, "tenant-acme", managedLabels(svc)); err != nil {
		t.Fatalf("Secret de plataforma: %v", err)
	}

	cs.ClearActions()
	if err := p.RepairPostgresRoles(ctx); err != nil {
		t.Fatalf("RepairPostgresRoles: %v", err)
	}
	for _, a := range cs.Actions() {
		if a.GetVerb() != "list" {
			t.Errorf("no debe tocarse nada: %s %s", a.GetVerb(), a.GetResource().Resource)
		}
	}
	if _, err := cs.BatchV1().Jobs("tenant-acme").Get(ctx, "sytue-platform-s3-pgroles", metav1.GetOptions{}); err == nil {
		t.Error("el Secret de plataforma no es una instancia de Postgres")
	}
}

// El paso está cableado y es no-op con un provisioner que no sabe repararse (dev).
func TestReconcilePostgresRolesOpcional(t *testing.T) {
	c := newTestController(&fakeCtrlStore{}, &recPlatform{}, &StubProvisioner{})
	if err := c.reconcilePostgresRoles(context.Background()); err != nil {
		t.Fatalf("el paso debe ser no-op sin capacidad de reparación: %v", err)
	}
}

// CLOUD-11 / INF-06 — RepairPostgresRoles no lista Secrets en NamespaceAll (""):
// recorre únicamente los namespaces tenant-* verificados como propiedad de Sytue.
func TestRepairPostgresRolesNoListaSecretsEnNamespaceAll(t *testing.T) {
	cs := fake.NewSimpleClientset(managedTenantNamespace("acme"))
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	if err := p.RepairPostgresRoles(context.Background()); err != nil {
		t.Fatalf("RepairPostgresRoles: %v", err)
	}
	for _, a := range cs.Actions() {
		if a.GetResource().Resource == "secrets" && a.GetNamespace() == "" {
			t.Fatal("RepairPostgresRoles listó Secrets en NamespaceAll (\"\") en vez de por namespace tenant-*")
		}
	}
}

func envByName(env []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range env {
		if env[i].Name == name {
			return &env[i]
		}
	}
	return nil
}
