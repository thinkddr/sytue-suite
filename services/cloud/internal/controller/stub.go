// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// StubPlatform es una Platform para dev/local que NO toca Kubernetes (no hay
// clúster en dev local). Registra las acciones en el log y considera todo listo,
// permitiendo recorrer el flujo deploy → ready end-to-end. En producción se
// sustituye por la implementación con client-go contra Kapsule.
type StubPlatform struct{}

// NewStubPlatform construye una StubPlatform.
func NewStubPlatform() *StubPlatform { return &StubPlatform{} }

func (StubPlatform) EnsureNamespace(ctx context.Context, namespace, tenant string, _ domain.PlanQuota) error {
	log.From(ctx).Debug("stub-platform: asegurar namespace", "namespace", namespace)
	return nil
}

func (StubPlatform) ApplyApp(ctx context.Context, spec AppSpec) (bool, error) {
	log.From(ctx).Info("stub-platform: aplicar app (dev)",
		"namespace", spec.Namespace, "image", spec.Image, "replicas", spec.Replicas,
		"hosts", spec.Hostnames, "env_keys", len(spec.Env), "secret_keys", len(spec.Secrets))
	return true, nil
}

func (StubPlatform) RetireApp(ctx context.Context, namespace, deploymentID string) error {
	log.From(ctx).Info("stub-platform: retirar app (dev)", "namespace", namespace, "deployment", deploymentID)
	return nil
}

func (StubPlatform) VerifyDomain(ctx context.Context, hostname, token, ingressTarget string) (bool, error) {
	log.From(ctx).Info("stub-platform: verificar dominio (dev, autoverificado)",
		"hostname", hostname, "target", ingressTarget)
	return true, nil
}

func (StubPlatform) EnsureTLS(ctx context.Context, req TLSRequest) (bool, error) {
	log.From(ctx).Info("stub-platform: emitir TLS (dev, autoemitido)",
		"hostname", req.Hostname, "issuer", req.Issuer)
	return true, nil
}

func (StubPlatform) RemoveDomainTLS(ctx context.Context, tenant, hostname string) error {
	log.From(ctx).Info("stub-platform: eliminar TLS de dominio (dev)", "tenant", tenant, "hostname", hostname)
	return nil
}

// StubProvisioner aprovisiona servicios gestionados en dev devolviendo
// configuración y credenciales simuladas pero coherentes (DSN local, bucket,
// rest_url de PostgREST...), para ejercitar la inyección de credenciales. En
// producción se sustituye por la implementación contra Scaleway RDB / Object
// Storage / Sytue ID.
type StubProvisioner struct {
	cfg config.Config
}

// NewStubProvisioner construye un StubProvisioner.
func NewStubProvisioner(cfg config.Config) *StubProvisioner { return &StubProvisioner{cfg: cfg} }

func (p *StubProvisioner) Provision(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error) {
	log.From(ctx).Info("stub-provisioner: aprovisionar servicio (dev)",
		"kind", string(svc.Kind), "tenant", svc.Tenant)
	switch svc.Kind {
	case domain.ServicePostgres:
		db := fmt.Sprintf("svc_%s", shortID(svc.ID.String()))
		creds, _ := json.Marshal(map[string]string{
			"DATABASE_URL":     fmt.Sprintf("postgres://app:dev@postgres:5432/%s?sslmode=disable", db),
			"POSTGREST_URL":    fmt.Sprintf("http://postgrest-%s:3000", shortID(svc.ID.String())),
			"POSTGREST_APIKEY": "dev-" + shortID(svc.ID.String()),
		})
		return ProvisionResult{
			Config: map[string]string{
				"engine":    "postgres",
				"database":  db,
				"rest_url":  fmt.Sprintf("https://%s.db.sytue.localhost", svc.ProjectID),
				"pooling":   "true",
				"backups":   "daily",
				"branching": "enabled",
			},
			Credentials: creds,
		}, nil
	case domain.ServiceStorage:
		bucket := fmt.Sprintf("sytue-app-%s", shortID(svc.ID.String()))
		creds, _ := json.Marshal(map[string]string{
			"S3_ENDPOINT":   p.cfg.S3Endpoint,
			"S3_BUCKET":     bucket,
			"S3_ACCESS_KEY": "dev-access-" + shortID(svc.ID.String()),
			"S3_SECRET_KEY": "dev-secret-" + shortID(svc.ID.String()),
		})
		return ProvisionResult{
			Config:      map[string]string{"bucket": bucket, "region": p.cfg.S3Region},
			Credentials: creds,
		}, nil
	case domain.ServiceFunctions:
		host := HostnameDeFuncion(svc.ProjectID, svc.Name, p.cfg.AppsDomain)
		return ProvisionResult{Config: map[string]string{
			"runtime": "kube-deployment", "url": "https://" + host, "host": host, "state": "running",
		}}, nil
	case domain.ServiceCron:
		return ProvisionResult{Config: map[string]string{
			"scheduler": "kube-cronjob", "schedule": svc.Config["schedule"],
		}}, nil
	case domain.ServiceJob:
		return ProvisionResult{Config: map[string]string{"runtime": "kube-job", "state": "succeeded"}}, nil
	case domain.ServiceWorker:
		return ProvisionResult{Config: map[string]string{
			"runtime": "kube-deployment", "replicas": "1", "state": "running",
		}}, nil
	case domain.ServiceAuth:
		// Aislamiento por proyecto vía tenant/claim (NO sub-issuer). Sytue ID sirve
		// discovery OIDC estándar en /.well-known/openid-configuration, sin rutas /realms/.
		tenant := fmt.Sprintf("project-%s", svc.ProjectID)
		creds, _ := json.Marshal(map[string]string{
			"OIDC_ISSUER":        p.cfg.OIDCIssuer, // issuer nativo de Sytue ID (sin /realms/)
			"OIDC_CLIENT_ID":     "app-" + shortID(svc.ID.String()),
			"OIDC_CLIENT_SECRET": "dev-" + shortID(svc.ID.String()),
			"OIDC_TENANT":        tenant,
		})
		return ProvisionResult{
			Config:      map[string]string{"tenant": tenant, "provider": "sytue-id"},
			Credentials: creds,
		}, nil
	default:
		return ProvisionResult{}, fmt.Errorf("controller: tipo de servicio no soportado: %s", svc.Kind)
	}
}

func (p *StubProvisioner) Deprovision(ctx context.Context, svc domain.ManagedService) error {
	log.From(ctx).Info("stub-provisioner: desaprovisionar servicio (dev)",
		"kind", string(svc.Kind), "tenant", svc.Tenant)
	return nil
}

func (p *StubProvisioner) RestorePostgres(ctx context.Context, svc domain.ManagedService, backupName string) error {
	log.From(ctx).Info("stub-provisioner: restore de postgres (dev, no-op)",
		"tenant", svc.Tenant, "backup", backupName)
	return nil
}

// BackupPostgresNow: en dev no hay CronJob que clonar, así que se registra y ya.
func (p *StubProvisioner) BackupPostgresNow(ctx context.Context, svc domain.ManagedService) error {
	log.From(ctx).Info("stub-provisioner: copia inmediata de postgres (dev, no-op)", "tenant", svc.Tenant)
	return nil
}

// RunServiceNow: en dev no hay CronJob ni Job que clonar —lo que hace la versión
// de verdad es duplicar la plantilla del programado para lanzarla ya—, así que se
// registra y ya. Sin este método `*StubProvisioner` deja de satisfacer
// `ServiceProvisioner` y el controlador NO COMPILA: el modo dev y el real
// comparten interfaz a propósito, para que añadir una operación obligue a
// decidir qué hace en los dos.
func (p *StubProvisioner) RunServiceNow(ctx context.Context, svc domain.ManagedService) error {
	log.From(ctx).Info("stub-provisioner: ejecución manual de tarea (dev, no-op)",
		"tenant", svc.Tenant, "kind", svc.Kind)
	return nil
}

func shortID(s string) string {
	s = removeDashes(s)
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func removeDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
