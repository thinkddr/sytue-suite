// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func restoreCfg() (cfg config.Config) {
	cfg = testCfg()
	cfg.S3Bucket = "sytue-backups"
	cfg.S3AccessKey = "AK"
	cfg.S3SecretKey = "SK"
	return cfg
}

func TestRestorePostgresJobShape(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, restoreCfg(), nil, nil)
	ctx := context.Background()

	id := uuid.New()
	svc := domain.ManagedService{ID: id, Tenant: "acme", ProjectID: uuid.New(), Kind: domain.ServicePostgres}
	if err := p.RestorePostgres(ctx, svc, "20260720T030000Z.sql.gz"); err != nil {
		t.Fatalf("RestorePostgres: %v", err)
	}

	ns := "tenant-acme"
	name := svcResourceName(svc)
	job, err := cs.BatchV1().Jobs(ns).Get(ctx, name+"-pg-restore", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Job de restore no creado en el namespace del tenant: %v", err)
	}

	// Job de UNA sola ejecución: sin reintentos, RestartPolicy Never, TTL de GC.
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Errorf("BackoffLimit = %v, se esperaba 0", job.Spec.BackoffLimit)
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished == 0 {
		t.Errorf("falta TTLSecondsAfterFinished")
	}
	pod := job.Spec.Template.Spec
	if pod.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("RestartPolicy = %q, se esperaba Never", pod.RestartPolicy)
	}
	// Postura de seguridad: rootless uid 999, sin token de ServiceAccount.
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Errorf("AutomountServiceAccountToken debe ser false")
	}
	if pod.SecurityContext == nil || pod.SecurityContext.RunAsUser == nil || *pod.SecurityContext.RunAsUser != 999 {
		t.Errorf("RunAsUser debe ser 999: %+v", pod.SecurityContext)
	}
	if pod.SecurityContext.RunAsNonRoot == nil || !*pod.SecurityContext.RunAsNonRoot {
		t.Errorf("RunAsNonRoot debe ser true")
	}

	// initContainer descarga (rclone), main container restaura (postgres/psql).
	if len(pod.InitContainers) != 1 || pod.InitContainers[0].Image != managedRcloneImage {
		t.Fatalf("initContainer de descarga inesperado: %+v", pod.InitContainers)
	}
	if len(pod.Containers) != 1 || pod.Containers[0].Image != managedPostgresImage {
		t.Fatalf("container de restore inesperado: %+v", pod.Containers)
	}
	dl := pod.InitContainers[0]
	rc := pod.Containers[0]
	if csx := rc.SecurityContext; csx == nil || csx.Capabilities == nil ||
		len(csx.Capabilities.Drop) != 1 || csx.Capabilities.Drop[0] != "ALL" {
		t.Errorf("el container de restore debe soltar TODAS las capabilities: %+v", rc.SecurityContext)
	}

	// El comando de restore es el INVERSO del backup: gunzip | psql contra el service.
	cmd := strings.Join(rc.Command, " ")
	if !strings.Contains(cmd, "gunzip -c /work/backup.sql.gz") || !strings.Contains(cmd, "psql") ||
		!strings.Contains(cmd, name+"-postgres") {
		t.Errorf("comando de restore inesperado: %q", cmd)
	}

	// El objeto S3 se compone EN SERVIDOR: backups/<tenant>/<shortID>/<backup>. El
	// nombre viaja por env, no interpolado en el script (sin inyección de shell).
	wantObj := "backups/acme/" + shortID(id.String()) + "/20260720T030000Z.sql.gz"
	var gotObj string
	for _, e := range dl.Env {
		if e.Name == "BACKUP_OBJECT" {
			gotObj = e.Value
		}
	}
	if gotObj != wantObj {
		t.Errorf("BACKUP_OBJECT = %q, se esperaba %q (compuesto en servidor)", gotObj, wantObj)
	}
}

func TestRestorePostgresRejectsTraversalName(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, restoreCfg(), nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{ID: uuid.New(), Tenant: "acme", Kind: domain.ServicePostgres}

	// Defensa en profundidad: un nombre con traversal se rechaza y no crea Job.
	if err := p.RestorePostgres(ctx, svc, "../otro-tenant/x.sql.gz"); err == nil {
		t.Fatal("RestorePostgres aceptó un nombre con traversal")
	}
	name := svcResourceName(svc)
	if _, err := cs.BatchV1().Jobs("tenant-acme").Get(ctx, name+"-pg-restore", metav1.GetOptions{}); err == nil {
		t.Fatal("se creó un Job pese al nombre inválido")
	}
}
