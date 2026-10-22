// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// AppSpec es la especificación de despliegue de una app de cliente: imagen,
// réplicas, puerto, variables/secretos ya resueltos, dominios y aislamiento.
// El Platform la traduce a objetos de Kapsule (Deployment/Service/Ingress +
// ResourceQuota/NetworkPolicy) o a un Serverless Container.
type AppSpec struct {
	Tenant      string
	Namespace   string // tenant-<slug>
	ProjectSlug string
	// ProjectID es el UUID del proyecto. Es la clave con la que las NetworkPolicy
	// emparejan la app con los servicios gestionados de SU proyecto: el slug no
	// sirve porque el aprovisionador de servicios solo conoce el ProjectID.
	ProjectID    string
	DeploymentID string
	Target       domain.DeployTarget
	Image        string
	Replicas     int
	// AutoscaleMax / AutoscaleCPUTarget son el techo y el objetivo del HPA, ya
	// RESUELTOS por el proyecto (domain.Project.EffectiveAutoscale*): el
	// controlador no vuelve a aplicar reglas de defecto. 0 = el controlador cae a
	// sus valores históricos, que es lo que ven las specs construidas a mano.
	AutoscaleMax       int
	AutoscaleCPUTarget int
	// Port es el puerto en el que escucha la app (Service, probes y targetPort).
	Port int
	// HealthPath es la ruta HTTP de readiness/liveness. Vacío = "/".
	HealthPath string
	// Size son los recursos del contenedor (tamaño de instancia del catálogo).
	// Viaja resuelto para que el controlador no tenga que consultar el catálogo.
	Size domain.InstanceSize
	// Env son las variables (no sensibles) a inyectar como env del contenedor.
	Env map[string]string
	// Secrets son los valores sensibles (variables marcadas como secreto +
	// credenciales de servicios gestionados) a inyectar vía Secret de Kubernetes.
	Secrets map[string]string
	// Hostnames es el conjunto de hosts que enruta el Ingress (URL prod/preview +
	// dominios propios activos).
	Hostnames []string
	Labels    map[string]string
	// ReleaseCommand se ejecuta ANTES de arrancar esta versión, con esta misma
	// imagen y este mismo entorno (el sitio de una migración de base de datos).
	// Vacío = no hay paso de release y el despliegue va directo, como siempre.
	ReleaseCommand string
	// HTTPRules son las redirecciones y cabeceras del proyecto, aplicadas en el
	// borde (Traefik) antes de llegar a la app. Vacías = nada que aplicar.
	HTTPRules domain.HTTPRules
	// Paused apaga la app sin retirarla: cero réplicas y sin autoescalado. El
	// Ingress, el Service y el certificado se quedan, así que reanudar no vuelve a
	// pasar por Let's Encrypt ni cambia ninguna URL.
	Paused bool
	// PreviewAuthHash es la línea htpasswd («usuario:$2y$...») con la que se cierra
	// una preview. Ya viene calculada: la contraseña en claro no entra nunca en el
	// clúster. Vacía = preview abierta, que es el comportamiento histórico.
	//
	// Solo aplica a `target: preview`: cerrar producción con una contraseña básica
	// sería romper el sitio del cliente.
	PreviewAuthHash string
}

// ReleaseState es en qué punto está el paso de release de un deployment.
type ReleaseState string

const (
	// ReleaseRunning: el Job existe y aún no ha terminado.
	ReleaseRunning ReleaseState = "running"
	// ReleaseSucceeded: terminó con éxito; se puede arrancar la versión nueva.
	ReleaseSucceeded ReleaseState = "succeeded"
	// ReleaseFailed: terminó mal. La versión nueva NO debe arrancar: si la
	// migración falló, la app nueva correría contra un esquema que no es el suyo.
	ReleaseFailed ReleaseState = "failed"
)

// TLSRequest pide a la plataforma asegurar un certificado para un hostname.
type TLSRequest struct {
	Hostname string
	Issuer   string // ClusterIssuer de cert-manager
	Tenant   string
}

// Platform es la abstracción del clúster Kapsule (client-go en prod). Todas las
// operaciones son idempotentes (apply declarativo).
type Platform interface {
	// EnsureNamespace crea el namespace tenant-<slug> con ResourceQuota,
	// LimitRange y NetworkPolicy deny-by-default si no existe. quota fija los
	// topes según el plan de la organización (Ola C).
	EnsureNamespace(ctx context.Context, namespace, tenant string, quota domain.PlanQuota) error
	// ApplyApp aplica el Deployment/Service/Ingress de la app y devuelve si el
	// despliegue está disponible (réplicas listas).
	ApplyApp(ctx context.Context, spec AppSpec) (ready bool, err error)
	// RunRelease ejecuta (o consulta) el paso de release del deployment: un Job de
	// una sola ejecución con la imagen y el entorno de esta versión. Idempotente:
	// crea el Job la primera vez y en las siguientes llamadas solo mira su estado,
	// que es lo que el reconciliador necesita para esperar sin guardar nada.
	RunRelease(ctx context.Context, spec AppSpec) (ReleaseState, error)
	// RetireApp elimina los objetos de un deployment (preview cerrado, rollback).
	RetireApp(ctx context.Context, namespace, deploymentID string) error
	// VerifyDomain comprueba que el hostname apunta al ingress (CNAME) y/o que el
	// registro TXT de verificación está presente.
	VerifyDomain(ctx context.Context, hostname, token, ingressTarget string) (bool, error)
	// EnsureTLS aplica el Certificate de cert-manager y devuelve si ya está emitido.
	EnsureTLS(ctx context.Context, req TLSRequest) (issued bool, err error)
	// RemoveDomainTLS elimina Certificate y Secret TLS de un hostname.
	RemoveDomainTLS(ctx context.Context, tenant, hostname string) error
}

// ProvisionResult es el desenlace del aprovisionamiento de un servicio gestionado:
// configuración pública (host, db, bucket, rest_url) + credenciales sensibles
// (en claro; el controller las cifra antes de persistir).
type ProvisionResult struct {
	Config      map[string]string
	Credentials []byte // p. ej. JSON con DSN, claves S3, API key
}

// ServiceProvisioner materializa los servicios gestionados de Sytue Cloud sobre
// la infraestructura UE: Sytue Postgres (+PostgREST), Storage (S3), Functions,
// Cron y Auth (realm de Sytue ID por proyecto). En prod habla con Scaleway RDB /
// Object Storage / Sytue ID; en dev, con un stub.
type ServiceProvisioner interface {
	Provision(ctx context.Context, svc domain.ManagedService) (ProvisionResult, error)
	Deprovision(ctx context.Context, svc domain.ManagedService) error
	// RestorePostgres restaura un backup elegido sobre el Sytue Postgres del
	// servicio (materializa un Job de restore en el namespace del tenant). Firma
	// espejo del método concreto de KubeProvisioner.
	RestorePostgres(ctx context.Context, svc domain.ManagedService, backupName string) error
	// BackupPostgresNow dispara una copia inmediata clonando el CronJob diario.
	BackupPostgresNow(ctx context.Context, svc domain.ManagedService) error
	RunServiceNow(ctx context.Context, svc domain.ManagedService) error
}

// serviceGetter carga un servicio gestionado bajo RLS del tenant en contexto. Lo
// satisface *store.Store; en tests, un fake. No se reutiliza storeAPI porque esa
// interfaz no expone GetService.
type serviceGetter interface {
	GetService(ctx context.Context, id uuid.UUID) (domain.ManagedService, error)
}

// DispatchServiceRestore materializa un evento EventServiceRestore: carga el
// servicio bajo el tenant del evento (RLS ⇒ un id de otro tenant da ErrNotFound,
// nunca dispara sobre el servicio equivocado), exige kind postgres y dispara
// RestorePostgres en la plataforma. La ejecución viva del Job la corre Kapsule;
// aquí solo se cablea la llamada a través de la interfaz.
func DispatchServiceRestore(ctx context.Context, getter serviceGetter, prov ServiceProvisioner, tenant string, serviceID uuid.UUID, backupName string) error {
	t, err := tenancy.New(tenant)
	if err != nil {
		return err
	}
	ctx = tenancy.Into(ctx, t)
	svc, err := getter.GetService(ctx, serviceID)
	if err != nil {
		return err
	}
	if svc.Kind != domain.ServicePostgres {
		return fmt.Errorf("controller: restore solo aplica a postgres, no %s", svc.Kind)
	}
	return prov.RestorePostgres(ctx, svc, backupName)
}

// DispatchServiceBackup materializa un EventServiceBackup: mismas garantías que
// el restore —el servicio se resuelve bajo el tenant DEL EVENTO, así que un id de
// otra organización da ErrNotFound en vez de disparar sobre el servicio
// equivocado— y solo aplica a postgres.
func DispatchServiceBackup(ctx context.Context, getter serviceGetter, prov ServiceProvisioner, tenant string, serviceID uuid.UUID) error {
	t, err := tenancy.New(tenant)
	if err != nil {
		return err
	}
	ctx = tenancy.Into(ctx, t)
	svc, err := getter.GetService(ctx, serviceID)
	if err != nil {
		return err
	}
	if svc.Kind != domain.ServicePostgres {
		return fmt.Errorf("controller: la copia solo aplica a postgres, no %s", svc.Kind)
	}
	return prov.BackupPostgresNow(ctx, svc)
}

// DispatchServiceRun materializa un EventServiceRun (ejecutar una tarea ahora).
// El servicio se resuelve bajo el tenant DEL EVENTO, igual que sus hermanos; el
// filtro por kind lo pone el provisioner, que es quien sabe qué objeto clonar.
func DispatchServiceRun(ctx context.Context, getter serviceGetter, prov ServiceProvisioner, tenant string, serviceID uuid.UUID) error {
	t, err := tenancy.New(tenant)
	if err != nil {
		return err
	}
	ctx = tenancy.Into(ctx, t)
	svc, err := getter.GetService(ctx, serviceID)
	if err != nil {
		return err
	}
	return prov.RunServiceNow(ctx, svc)
}
