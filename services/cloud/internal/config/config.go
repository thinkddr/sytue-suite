// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Cloud (PaaS) desde el
// entorno del proceso (12-factor). Reutiliza el cargador común de
// platform/pkg/config y respeta EXACTAMENTE los nombres de variable definidos
// en deploy/compose (PORT, DATABASE_URL, OIDC_*, S3_*, REGISTRY_*, KUBE_*).
// Sin secretos en git.
//
// Una sola Config sirve a los tres binarios del servicio (cloud-api,
// cloud-builder, cloud-controller); cada uno usa el subconjunto que necesita.
package config

import (
	"fmt"
	"net"
	"os"
	"slices"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-cloud"

// Puertos canónicos (CONVENTIONS §6): API 8090, builder 8091.
const (
	DefaultAPIPort     = "8090"
	DefaultBuilderPort = "8091"
)

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=..."),
// y sirve de default cuando la variable de entorno VERSION no está definida.
var Version = "dev"

// DefaultClusterCIDRs son los CIDR internos de un clúster Scaleway Kapsule con
// los valores de fábrica: pods 100.64.0.0/14 y services 10.32.0.0/12
// (https://www.scaleway.com/en/docs/kubernetes — CNI por defecto). Se excluyen
// del egress a Internet de los tenants; sobreescribible con CLUSTER_CIDRS.
const DefaultClusterCIDRs = "100.64.0.0/14,10.32.0.0/12"

// Config es la configuración completa de Sytue Cloud.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública del panel de control (https://cloud.sytue.eu).
	PublicURL string
	// AppsDomain es el dominio base de las apps prod de clientes
	// (apps.sytue.eu → https://<app>.apps.sytue.eu).
	AppsDomain string
	// PreviewDomain es el dominio base de las previews por PR
	// (preview.sytue.eu → https://<proyecto>-pr-<n>.preview.sytue.eu).
	PreviewDomain string
	// IngressTarget es el CNAME al que apuntan los dominios propios de cliente.
	IngressTarget string

	// DatabaseURL es el DSN PostgreSQL (postgres://...). Única fuente de verdad.
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// OIDC configura la validación de tokens de Sytue ID (JWKS).
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// IdentityAPIURL es la base de Sytue ID para invitar/resolver miembros. Por
	// defecto coincide con OIDCIssuer; en Compose puede apuntar al servicio interno.
	IdentityAPIURL string

	// HubBaseURL/HubInternalSecret conectan con Sytue Hub para avisar en el
	// centro de notificaciones de la suite (topBar y apps nativas) de que un
	// despliegue quedó listo o falló. Mismos defaults que calendar.
	HubBaseURL        string
	HubInternalSecret string

	// S3 configura el Object Storage para cache de builds y logs (build-cache).
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool
	// S3Backup* son unas credenciales OPCIONALES ACOTADAS al prefijo backups/ del
	// bucket de plataforma (arn:aws:s3:::<bucket>/backups/*), que ops acuña una vez.
	// Si están, el CronJob de backup las monta en vez de las claves de plataforma de
	// alcance amplio (ver upsertPlatformS3Secret). Ausentes ⇒ residual acotado. Nunca
	// en git.
	S3BackupAccessKey string
	S3BackupSecretKey string

	// Scaleway IAM acuña credenciales S3 por-tenant ACOTADAS al bucket (Object
	// Storage es S3-compatible; IAM da applications + API keys, y la bucket policy
	// restringe la key a ese bucket). Sin SCWSecretKey+SCWOrgID la provisión de
	// storage sigue FAIL-CLOSED (no se entregan las claves root de la plataforma).
	// SCWSecretKey es el token de plataforma con derecho IAM. Nunca en git.
	SCWIAMURL    string
	SCWSecretKey string
	SCWOrgID     string

	// Registry es el Container Registry destino de las imágenes construidas
	// (Scaleway Registry en prod).
	RegistryURL      string
	RegistryUsername string
	RegistryPassword string
	// Managed*Image son los runtimes auxiliares de los servicios de cliente.
	// Producción los sobreescribe con copias firmadas en el registry privado;
	// los defaults solo mantienen cómodo el desarrollo local.
	ManagedPostgresImage  string
	ManagedPostgrestImage string
	ManagedRcloneImage    string

	// Kubernetes (Kapsule) — el controller despliega en él. KubeconfigPath vacío
	// usa el ServiceAccount in-cluster.
	KubeconfigPath string
	// Identidad del acceso en vivo: el controller vincula este rol y esta cuenta
	// SOLO dentro de namespaces de tenant. El chart de cloud-api publica los tres
	// valores juntos para que un override del ServiceAccount no desincronice RBAC.
	CloudAPIClusterRole    string
	CloudAPIServiceAccount string
	CloudAPINamespace      string

	// CloudControllerClusterRole, CloudControllerServiceAccount y
	// CloudControllerNamespace identifican el sujeto y el ClusterRole acotado a
	// cargas de tenant que cloud-controller vincula por namespace `tenant-<slug>`
	// mediante el RoleBinding `sytue-cloud-controller`.
	CloudControllerClusterRole    string
	CloudControllerServiceAccount string
	CloudControllerNamespace      string

	// AbiertoATerceros controla si Sytue Cloud admite organizaciones distintas de
	// TenantsPermitidos. Permanece en false hasta que los nodos de carga tengan
	// gVisor (runsc) activo.
	AbiertoATerceros  bool
	TenantsPermitidos []string
	// RunnerMode selecciona la implementación del PaaS: "kube" (producción, sobre
	// Kapsule vía client-go) o "stub" (dev/local sin clúster). Paridad dev/prod
	// (CONVENTIONS §15): misma interfaz Runner/Platform, distinta implementación.
	RunnerMode string
	// BuildkitImage es la imagen de BuildKit rootless (Apache-2.0), idealmente
	// fijada por digest en prod.
	BuildkitImage string
	// CraneImage es la imagen de crane (go-containerregistry, Apache-2.0) del
	// contenedor de PUSH: empuja al Container Registry el tarball OCI que el build
	// (Kaniko --no-push) produce SIN credencial. Aísla la credencial de push del
	// código de tenant no confiable (registry-cred). Pinneable por digest en prod.
	CraneImage string

	// CNB (Cloud Native Buildpacks) — estrategia de build alternativa al
	// Dockerfile+Kaniko: el lifecycle de CNB auto-detecta el lenguaje y construye
	// la imagen OCI SIN Dockerfile (paridad DigitalOcean App Platform / Heroku /
	// Coolify Nixpacks). Imágenes de infra, PINNEADAS (nunca :latest), documentadas
	// caso por caso como Kaniko:
	//   - CNBBuilderImage: builder de Paketo (Apache-2.0) que trae el lifecycle de
	//     CNB (/cnb/lifecycle/creator) + los buildpacks. El binario `creator` corre
	//     rootless (usuario cnb, uid 1000). No hay imagen de "lifecycle" aparte: el
	//     lifecycle vive DENTRO del builder (por eso no hay CNB_LIFECYCLE).
	//   - CNBGitImage: imagen mínima con git para el initContainer que clona el
	//     contexto (el lifecycle NO clona: opera sobre un directorio local). git es
	//     GPLv2, pero es una herramienta de infra EJECUTADA en el Job (no una
	//     dependencia de código enlazada), mismo criterio clean-room que Kaniko.
	//   - CNBRunImage: OPCIONAL. Imagen base de ejecución de la app. Vacío ⇒ se
	//     deriva de los metadatos del builder (io.buildpacks.builder.metadata),
	//     como hace `pack`; solo se fija para forzar otra run image.
	CNBBuilderImage string
	CNBGitImage     string
	CNBRunImage     string

	// HTTPTaskImage es la imagen con la que corre una tarea programada que solo
	// LLAMA A UNA URL (curl, licencia curl —permisiva, tipo MIT—). Existe porque
	// una tarea programada que golpea un endpoint es el caso más común del mundo
	// —es como se usa Cloud Scheduler y como llegan las migraciones desde Google—
	// y obligar a empaquetar una imagen propia para hacer un POST convertía cinco
	// minutos de trabajo en una tarde.
	HTTPTaskImage string

	// BuildNamespace es el namespace donde corren los Jobs de Kaniko.
	BuildNamespace string

	// AnalyticsIngestHost es el nombre DNS del colector de Sytue Analytics dentro
	// del clúster (p. ej. «analytics-ingest.sytue-one.svc.cluster.local»). Con él
	// se añade a cada app de cliente una ruta `/_sytue` EN SU PROPIO DOMINIO, que
	// es lo que hace la medición de primera parte: el sitio se identifica por el
	// Host, así que el script no lleva identificador y nadie puede inflar los
	// números de otro (ADR 0026).
	//
	// VACÍO = la ruta no se añade. El namespace de plataforma es configurable
	// (Release.Namespace del chart), así que no hay un valor por defecto honesto:
	// antes que adivinarlo y dejar a cada app con una ruta que no resuelve, la
	// medición se queda apagada hasta que alguien la configure.
	AnalyticsIngestHost string
	// SMTP* entrega las alertas cuyo canal es un `mailto:`. VACÍO = sin correo:
	// las alertas por webhook siguen funcionando y las de correo se omiten con un
	// aviso, en vez de fallar una y otra vez en el dispatcher.
	//
	// El remitente sale del BUZÓN configurado, no de quien dispara la alerta: el
	// servidor de correo de la casa rechaza un From que no sea suyo.
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	// CertIssuer es el ClusterIssuer de cert-manager para TLS de dominios.
	CertIssuer string
	// ClusterCIDRs son los CIDR de pods/services del clúster, que las
	// NetworkPolicy de tenant excluyen del egress a Internet (ipBlock.except)
	// además de RFC1918 y link-local. Default: Scaleway Kapsule
	// (DefaultClusterCIDRs). Formato CSV en CLUSTER_CIDRS.
	ClusterCIDRs []string

	// SecretEncKey es la clave (base64, 32 bytes) para cifrar secretos/credenciales
	// de servicios gestionados en reposo (AES-256-GCM). Nunca en git.
	SecretEncKey string

	// WebhookSecret valida la firma HMAC de los webhooks git entrantes.
	WebhookSecret string

	// GitHubApp* configuran la integración con una GitHub App (paridad
	// Coolify/Vercel): repos privados sin deploy-key manual + comentario de
	// preview en el PR. GitHubAppID es el App ID numérico; GitHubAppPrivateKey es
	// la clave privada PEM (RS256) con la que se firma el JWT de app. Ausentes ⇒
	// integración deshabilitada (no-op). La clave NUNCA en git/logs.
	//
	// GitHubAppClientID/Secret son las credenciales OAuth de la App
	// ("user-to-server"): el callback de instalación las usa para canjear el
	// `code` por un user access token y comprobar, contra GET /user/installations,
	// que la instalación que se liga PERTENECE al usuario que conecta (cierra el
	// confused-deputy cross-tenant de ligar un installation_id ajeno). Ausentes ⇒
	// el connect con verificación de propiedad queda deshabilitado (fail-closed).
	// El secret NUNCA en git/logs.
	GitHubAppID           int64
	GitHubAppPrivateKey   string
	GitHubAppClientID     string
	GitHubAppClientSecret string

	// ForgeURL y ForgeOAuth* conectan Sytue Cloud con SYTUE GIT para la tercera vía
	// del alta: crearle al usuario un repositorio nuevo con el esqueleto de su
	// stack (services/cloud/internal/forge).
	//
	// Son credenciales de una aplicación OAuth2 registrada EN la forja, no un token
	// de administrador: lo que Cloud puede hacer es exactamente lo que cada usuario
	// consiente, y él lo revoca desde su cuenta. Sin las tres, esa vía del alta no
	// se ofrece —y la interfaz lo dice, en vez de esconder el botón—.
	ForgeURL          string
	ForgeClientID     string
	ForgeClientSecret string
	// DragonflyURL es la cache efímera compartida (redis://dragonfly:6379). VACÍA
	// es un estado válido: sin ella los tokens de API se resuelven contra
	// PostgreSQL en cada petición, que es el comportamiento previo.
	DragonflyURL string
	// DragonflyPassword autentica contra Dragonfly. Va SEPARADA de la URL porque
	// la URL no es un secreto y la contraseña sí: aquella viaja como `value:` en el
	// manifiesto y esta por `secretKeyRef`. VACÍA => sin autenticación, que es el
	// comportamiento previo.
	DragonflyPassword string
	// CacheSigningKey sella la resolución de los tokens que se guarda en la cache.
	//
	// NO es una optimización ni un extra: `ResolveAPIToken` devuelve el valor
	// cacheado TAL CUAL como identidad autenticada, así que sin sello quien pueda
	// escribir en Dragonfly emite credenciales de cualquier tenant con cualquier
	// scope. VACÍA es un estado válido y seguro —no se cachea nada y todo se
	// resuelve contra PostgreSQL—, nunca uno que caches sin firmar.
	CacheSigningKey string

	// BuildTimeout es el plazo máximo de una build antes de marcarla fallida.
	BuildTimeout time.Duration
	// ReconcileInterval es el periodo del bucle del controller.
	ReconcileInterval time.Duration
	// MaxProjectsPerOrg limita los proyectos por organización (cuota plan free).
	MaxProjectsPerOrg int
}

// Load construye la configuración leyendo el entorno. defaultPort fija el puerto
// HTTP por defecto del binario que la carga (API 8090, builder 8091). Devuelve
// todos los errores de validación juntos.
func Load(defaultPort string) (Config, error) {
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Config{}, err
	}
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}

	l := pcfg.NewLoader("")

	cfg := Config{
		Base:          base,
		PublicURL:     l.GetDefault("PUBLIC_URL", "https://cloud.sytue.localhost"),
		AppsDomain:    l.GetDefault("APPS_DOMAIN", "apps.sytue.localhost"),
		PreviewDomain: l.GetDefault("PREVIEW_DOMAIN", "preview.sytue.localhost"),
		IngressTarget: l.GetDefault("INGRESS_TARGET", "ingress.sytue.eu"),

		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:     l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:    l.Get("OIDC_JWKS_URL"),
		OIDCAudience:   l.GetDefault("OIDC_AUDIENCE", "sytue-cloud"),
		IdentityAPIURL: l.Get("IDENTITY_API_URL"),

		HubBaseURL:        l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		HubInternalSecret: l.HubInternalSecret(base.Env),

		S3Endpoint:  l.GetDefault("S3_ENDPOINT", "minio:9000"),
		S3Region:    l.GetDefault("S3_REGION", "fr-par"),
		S3AccessKey: l.Require("S3_ACCESS_KEY"),
		S3SecretKey: l.Require("S3_SECRET_KEY"),
		S3Bucket:    l.GetDefault("S3_BUCKET", "sytue-dev-build-cache"),
		S3UseSSL:    l.GetBoolDefault("S3_USE_SSL", false),

		S3BackupAccessKey: l.Get("S3_BACKUP_ACCESS_KEY"),
		S3BackupSecretKey: l.Get("S3_BACKUP_SECRET_KEY"),

		SCWIAMURL:    l.GetDefault("SCW_IAM_URL", "https://api.scaleway.com/iam/v1alpha1"),
		SCWSecretKey: l.Get("SCW_SECRET_KEY"),
		SCWOrgID:     l.Get("SCW_ORG_ID"),

		RegistryURL:      l.GetDefault("REGISTRY_URL", "rg.fr-par.scw.cloud/sytue"),
		RegistryUsername: l.Get("REGISTRY_USERNAME"),
		RegistryPassword: l.Get("REGISTRY_PASSWORD"),
		// 18.6: la 18.4 arrastra CVE-2026-19385 (8.8) —quien crea objetos ejecuta
		// código como el usuario que corre pg_dump— y otros tres del 2026-08-13.
		ManagedPostgresImage:  l.GetDefault("MANAGED_POSTGRES_IMAGE", "postgres:18.6@sha256:06cad38a5d9f5d24b4d83d86def30795d5e4b757fedbf5281172b576dedcd941"),
		ManagedPostgrestImage: l.GetDefault("MANAGED_POSTGREST_IMAGE", "postgrest/postgrest:v14.16@sha256:bea1c76a856fa39d1e542d25911cf95d02fe2bf971992d033044ff209f1504b8"),
		ManagedRcloneImage:    l.GetDefault("MANAGED_RCLONE_IMAGE", "rclone/rclone:1.75.0@sha256:b06aed988cf5967de7c25be5925240983981c757f4ed1ac9d2fa659d51d60548"),

		SMTPHost:     l.Get("SMTP_HOST"),
		SMTPPort:     l.GetDefault("SMTP_PORT", "587"),
		SMTPUser:     l.Get("SMTP_USER"),
		SMTPPassword: l.Get("SMTP_PASSWORD"),
		SMTPFrom:     l.GetDefault("SMTP_FROM", "cloud@sytue.eu"),

		KubeconfigPath:                l.Get("KUBECONFIG"),
		CloudAPIClusterRole:           l.Get("CLOUD_API_CLUSTER_ROLE"),
		CloudAPIServiceAccount:        l.Get("CLOUD_API_SERVICE_ACCOUNT"),
		CloudAPINamespace:             l.Get("CLOUD_API_NAMESPACE"),
		CloudControllerClusterRole:    l.Get("CLOUD_CONTROLLER_CLUSTER_ROLE"),
		CloudControllerServiceAccount: l.Get("CLOUD_CONTROLLER_SERVICE_ACCOUNT"),
		CloudControllerNamespace:      l.Get("CLOUD_CONTROLLER_NAMESPACE"),
		AbiertoATerceros:              l.GetBoolDefault("ABIERTO_A_TERCEROS", false),
		TenantsPermitidos:             l.GetCSVDefault("TENANTS_PERMITIDOS", "sytue"),
		RunnerMode:                    l.GetDefault("RUNNER_MODE", ""),
		BuildkitImage:                 l.GetDefault("BUILDKIT_IMAGE", "moby/buildkit:v0.32.1-rootless@sha256:66e5fe5b25509626c0e628c6e57badbcc2e8a62218ba3e1358f1427e90de6a88"),
		HTTPTaskImage:                 l.GetDefault("HTTP_TASK_IMAGE", "curlimages/curl:8.21.0@sha256:7c12af72ceb38b7432ab85e1a265cff6ae58e06f95539d539b654f2cfa64bb13"),
		CraneImage:                    l.GetDefault("CRANE_IMAGE", "gcr.io/go-containerregistry/crane:v0.21.8@sha256:e85fa14e99e5ef7351822ed887ce322bc1e22f9ef8e9eb54ebf2d7a9473bfef9"),

		// Pinneadas por tag (no :latest); en prod, re-pinnear por digest tras
		// verificar con un build vivo (como KANIKO_IMAGE).
		CNBBuilderImage: l.GetDefault("CNB_BUILDER_IMAGE", "paketobuildpacks/builder-jammy-base:0.4.615@sha256:ff504f33d9ce9f3ed9f109e61bffbf145f80e98d90abfeb2172b96dbb4eceb92"),
		CNBGitImage:     l.GetDefault("CNB_GIT_IMAGE", "alpine/git:2.54.0@sha256:b4d933c7185b5cc9da7704f2c6e19d477f935ea31a50d9a017ffce2bbd1109a3"),
		CNBRunImage:     l.Get("CNB_RUN_IMAGE"),

		BuildNamespace:      l.GetDefault("BUILD_NAMESPACE", "sytue-builds"),
		AnalyticsIngestHost: l.Get("ANALYTICS_INGEST_HOST"),
		CertIssuer:          l.GetDefault("CERT_ISSUER", "sytue-letsencrypt"),
		ClusterCIDRs:        l.GetCSVDefault("CLUSTER_CIDRS", DefaultClusterCIDRs),

		SecretEncKey:  l.Require("SECRET_ENC_KEY"),
		WebhookSecret: l.Get("WEBHOOK_SECRET"),

		GitHubAppID:           int64(l.GetIntDefault("GITHUB_APP_ID", 0)),
		GitHubAppPrivateKey:   l.Get("GITHUB_APP_PRIVATE_KEY"),
		GitHubAppClientID:     l.Get("GITHUB_APP_CLIENT_ID"),
		GitHubAppClientSecret: l.Get("GITHUB_APP_CLIENT_SECRET"),
		ForgeURL:              l.Get("FORGE_URL"),
		ForgeClientID:         l.Get("FORGE_CLIENT_ID"),
		ForgeClientSecret:     l.Get("FORGE_CLIENT_SECRET"),
		DragonflyURL:          l.Get("DRAGONFLY_URL"),
		DragonflyPassword:     l.Get("DRAGONFLY_PASSWORD"),
		CacheSigningKey:       l.Get("CACHE_SIGNING_KEY"),

		BuildTimeout:      l.GetDurationDefault("BUILD_TIMEOUT", 20*time.Minute),
		ReconcileInterval: l.GetDurationDefault("RECONCILE_INTERVAL", 15*time.Second),
		MaxProjectsPerOrg: l.GetIntDefault("MAX_PROJECTS_PER_ORG", 100),
	}

	cfg.S3Endpoint, cfg.S3UseSSL = pcfg.NormalizeS3Endpoint(cfg.S3Endpoint, cfg.S3UseSSL)

	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":" + defaultPort
	}

	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}
	if cfg.IdentityAPIURL == "" {
		cfg.IdentityAPIURL = cfg.OIDCIssuer
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// UseKubeRunner indica si debe usarse la implementación de producción sobre
// Kapsule (client-go). Regla: si RUNNER_MODE está fijado, manda ("kube"/"stub");
// si no, se deduce del entorno (prod ⇒ kube, dev/staging ⇒ stub). Paridad
// dev/prod: misma interfaz, la selección es puramente de configuración.
func (c Config) UseKubeRunner() bool {
	switch c.RunnerMode {
	case "kube", "kubernetes", "prod":
		return true
	case "stub", "dev", "local":
		return false
	default:
		return c.Env.IsProd()
	}
}

// RequireWebhookSecret falla si en producción no hay HMAC global de webhooks.
//
// El endpoint de webhooks git es PÚBLICO y sin autenticar: la firma HMAC es lo
// único que separa un push real de un POST fabricado que dispara un build y un
// despliegue. Los proyectos nuevos llevan su propio secreto, pero los anteriores
// caen a este global, y vacío significaba «no exijas firma». Se comprueba al
// arrancar, no en la primera entrega, para que se vea al desplegar.
//
// NO vive en validate() porque este paquete lo comparten los tres binarios y
// solo la API sirve esa ruta: exigírselo también al builder y al controller los
// dejaba en CrashLoopBackOff sin que les faltara nada suyo. Y darles el secreto
// para callarlos sería repartir un HMAC a procesos que no lo usan.
func (c Config) RequireWebhookSecret() error {
	if c.Env.IsProd() && c.WebhookSecret == "" {
		return fmt.Errorf(
			"config: WEBHOOK_SECRET es obligatorio en producción " +
				"(lo materializa deploy/production/02b-secretos.sh en el Secret sytue-cloud-api)",
		)
	}
	return nil
}

// TenantAllowed informa de si el tenant puede operar en Sytue Cloud según el
// cerrojo AbiertoATerceros / TenantsPermitidos. En dev/test con Config{} en
// memoria (sin entorno prod ni lista explícita) no bloquea los tests unitarios.
func (c Config) TenantAllowed(tenant string) bool {
	if c.AbiertoATerceros {
		return true
	}
	if !c.Env.IsProd() && len(c.TenantsPermitidos) == 0 {
		return true
	}
	allowed := c.TenantsPermitidos
	if len(allowed) == 0 {
		allowed = []string{"sytue"}
	}
	return slices.Contains(allowed, tenant)
}

func (c Config) validate() error {
	if c.Env.IsProd() && (c.RegistryUsername == "" || c.RegistryPassword == "") {
		return fmt.Errorf("config: REGISTRY_USERNAME y REGISTRY_PASSWORD son obligatorios en producción")
	}
	if c.MaxProjectsPerOrg <= 0 {
		return fmt.Errorf("config: MAX_PROJECTS_PER_ORG debe ser > 0")
	}
	if c.BuildTimeout <= 0 {
		return fmt.Errorf("config: BUILD_TIMEOUT debe ser > 0")
	}
	// Un CIDR malformado dejaría al tenant sin la exclusión de redes internas
	// (la NetworkPolicy sería rechazada por el API server): falla en arranque.
	for _, cidr := range c.ClusterCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("config: CLUSTER_CIDRS: CIDR inválido %q", cidr)
		}
	}
	return nil
}
