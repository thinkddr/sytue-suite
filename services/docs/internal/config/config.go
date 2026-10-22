// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración de Sytue Docs desde el entorno
// (12-factor), reutilizando platform/pkg/config. Prefijo: SYTUE_DOCS.
package config

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

// Config es la configuración completa del servicio Docs.
type Config struct {
	Base config.Base

	// PostgreSQL (único datastore).
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool (SYTUE_DOCS_DB_MAX_CONNS).
	DBMaxConns int

	// Sytue ID (OIDC): issuer + audiencia (client_id del BFF sytue-docs).
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string

	// HubBaseURL/HubInternalSecret conectan con Sytue Hub para avisar en el
	// centro de notificaciones (topBar y apps nativas) de que alguien compartió
	// un documento. Mismos defaults que calendar.
	HubBaseURL             string
	HubInternalSecret      string
	RoomsURL               string
	RoomsCallAccessSecret  string
	EncryptedDocumentCalls bool
	// IDBaseURL es Sytue ID por dentro del clúster: de ahí sale el correo del dueño
	// de un documento, con la sesión de quien le pide editarlo.
	IDBaseURL string
	// IDDirectorioSecret abre POST /internal/directory/resolve de Sytue ID: la
	// reconstrucción de avisos (`docs avisos-historicos`) lo usa para saber de quién es
	// hoy la dirección de una mención. Vacío ⇒ las menciones no se reconstruyen.
	IDDirectorioSecret string
	// IDOrgsSecret abre POST /internal/people/orgs de Sytue ID: en qué organizaciones
	// está quien recibe una petición de firma de fuera, para aplicar su política (ADR
	// 0059). Sin él no se pide firma fuera.
	IDOrgsSecret string
	// SMTP* es el buzón con el que sale el correo de «te piden editar» al dueño,
	// como en Calendar y el ERP. Sin SMTPFrom no sale correo: el aviso del Hub llega igual.
	SMTPAddr, SMTPFrom, SMTPUser, SMTPPassword, SMTPTLSServerName string
	// OIDCAcceptedAudiences: la suite comparte sesión (cookie única), así que el
	// token puede traer la audiencia de OTRO app de la suite. Se acepta si su aud
	// contiene ALGUNA (allowlist first-party).
	OIDCAcceptedAudiences []string

	// Object Storage S3 (bucket docs-blobs): blobs/exports.
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool
	// S3Remote es el almacén remoto (S3_REMOTE_*), donde va todo tenant salvo
	// los de S3LocalTenants; nil = todo a S3_*. Ver objectstore.Remote (ADR 0038).
	S3Remote       *objectstore.Config
	S3LocalTenants []string
	S3Bucket       string

	// SnapshotInlineLimit: tope (bytes) de snapshot inline en PG antes de ir a S3.
	SnapshotInlineLimit int

	// SecretEncKey es la clave AES-256 (32 bytes ya decodificados) con la que el
	// conector de datos externos cifra las credenciales at-rest. Opcional: si está
	// vacía el servicio arranca igual, pero rechaza crear conexiones con
	// credenciales (ver internal/connectors.Sealer). Origen: SYTUE_DOCS_SECRET_ENC_KEY
	// en base64 estándar.
	SecretEncKey []byte

	// DragonflyURL (redis://...) es el pub/sub EFÍMERO para el fan-out
	// cross-réplica de las operaciones colaborativas. Opcional (degradado a Postgres).
	DragonflyURL string
	// DragonflyPassword autentica contra Dragonfly. Va SEPARADA de la URL porque
	// la URL no es un secreto y la contraseña sí: aquella viaja como `value:` en el
	// manifiesto y esta por `secretKeyRef`. VACÍA => sin autenticación, que es el
	// comportamiento previo.
	DragonflyPassword string

	// OCRURL es Sytue OCR, el servicio de reconocimiento (ADR-0034). Vacío = la
	// interfaz NO ofrece «reconocer texto», que es lo correcto: ofrecerlo y
	// fallar es peor que no ofrecerlo. El chart lo pasa siempre, así que vacío
	// significa un despliegue a medias y no una decisión.
	OCRURL string
	// OCRTimeout por página. Una A4 a 300 ppp tarda entre uno y cuatro segundos;
	// el margen cubre las densas y las que llegan con la cola llena.
	OCRTimeout time.Duration

	// SelloURL es la autoridad de sellado de tiempo (RFC 3161) para las firmas.
	// Vacía = las firmas salen SIN sello, y la interfaz lo dice: prueban quién
	// firmó pero no cuándo, así que dejan de verificarse cuando el certificado
	// caduca. Qué autoridad usar es una decisión de negocio —tiene que ser un
	// prestador cualificado de la lista de confianza de la UE—, anotada en
	// docs/business.md.
	SelloURL string
	// SelloPolitica es el OID de política que exigen algunas autoridades.
	SelloPolitica string
	// SelloUsuario y SelloClave, para las que piden autenticación básica.
	SelloUsuario, SelloClave string
	// SelloTimeout acota la espera. Una autoridad colgada no puede colgar una
	// firma: pasado el plazo se firma sin sello.
	SelloTimeout time.Duration
	// SelloAnclas son las raíces (PEM) de la cadena de la autoridad, contra las que
	// se AUTENTICA cada token. Las autoridades hablan HTTP plano, y sin anclas un
	// intermediario podría colar un token fabricado. Vacía = sin autenticar, y el
	// arranque lo avisa; con texto que no sea ni un certificado, no arranca.
	SelloAnclas *x509.CertPool
	// FirmaAnclas son las raíces (PEM) en las que se confía al VERIFICAR las firmas
	// de un PDF recibido (pdfverificar.go). Vacía = la verificación dice si el
	// documento cambió, pero no quién lo firmó: la confianza sale «sin anclas».
	// Nunca se cae al almacén del sistema, que no es la lista de confianza de la UE.
	FirmaAnclas *x509.CertPool

	// IntegrationsURL es la raíz INTERNA del Sytue Integration Gateway, destino del
	// origen de datos "gateway" del conector (Power Query → SaaS de terceros). Es
	// config de SERVICIO (no de conexión) para no reenviar el JWT del usuario a un
	// host que elija el admin. Origen: SYTUE_DOCS_INTEGRATIONS_URL.
	IntegrationsURL string

	// BasesURL es la raíz INTERNA de Sytue Bases, destino del origen de datos "bases" del
	// conector (Calcus lee una tabla o una vista con un token de conexión de solo
	// lectura). Config de SERVICIO, como IntegrationsURL. Origen: SYTUE_DOCS_BASES_URL.
	BasesURL string

	// DriveURL es la raíz INTERNA de Sytue Drive, donde Docs deja el fichero de
	// cada documento nuevo (ver internal/drive y docs/architecture.md#docs-en-drive). Vacía ⇒
	// los documentos se crean como siempre, solo en Docs, y NO aparecen en la
	// unidad de nadie: es un despliegue a medias, no una opción de producto, pero
	// se degrada en vez de impedir crear documentos. Origen: SYTUE_DOCS_DRIVE_URL.
	DriveURL string
	// DriveTimeout acota cada llamada a Drive. Está en el camino de «crear
	// documento», que es una pulsación con la persona mirando: un Drive colgado
	// tiene que fallar rápido y decirlo, no dejar la pantalla pensando medio minuto.
	DriveTimeout time.Duration
	// DriveInternalSecret es el secreto PROPIO de las dos puertas entre Docs y Drive
	// de la papelera compartida (Drive mig 0027). Vacío, cada papelera va por su lado.
	// Origen: SYTUE_DOCS_DRIVE_INTERNAL_SECRET.
	DriveInternalSecret string

	// PublicURL es la raíz PÚBLICA de Sytue Docs (https://docs.sytue.eu), la que
	// ve un navegador. La usa el catálogo WOPI para componer las URL con las que
	// un host abre cada tipo de fichero, así que no vale la interna: quien sigue
	// esas URL es el navegador de la persona, no otro servicio del clúster.
	// VACÍA ⇒ el catálogo responde 503 en vez de anunciar direcciones inservibles.
	// Origen: SYTUE_DOCS_PUBLIC_URL, como el resto de la suite. Se leyó un tiempo
	// de SYTUE_DOCS_DOCS_PUBLIC_URL —el prefijo lo pone el loader, y el nombre lo
	// repetía— así que el chart pasaba `DOCS_PUBLIC_URL` y el binario no la veía
	// NUNCA: WOPI entero quedó apagado en producción sin un solo error.
	PublicURL string

	// WopiHosts son los hosts WOPI que esta instalación acepta abrir, separados
	// por comas (se admite `*.dominio`). VACÍA ⇒ solo el Sytue Drive de la propia
	// suite. Es una lista y no «cualquiera» a propósito: sin ella, cualquier
	// `WOPISrc` convertiría a Sytue Docs en el convertidor —y el proxy— gratuito
	// de quien pasara por ahí.
	WopiHosts []string
}

// Load carga y valida la configuración del servicio.
func Load() (Config, error) {
	base, err := config.LoadBase("SYTUE_DOCS", "sytue-docs")
	if err != nil {
		return Config{}, err
	}
	// Puerto canónico de Sytue Docs en dev (CONVENTIONS §6): 8083.
	if base.HTTPAddr == ":8080" {
		base.HTTPAddr = ":8083"
	}

	l := config.NewLoader("SYTUE_DOCS")
	cfg := Config{
		Base:        base,
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:   l.GetDefault("OIDC_ISSUER", "https://id.sytue.eu"),
		OIDCJWKSURL:  l.Get("OIDC_JWKS_URL"),
		OIDCAudience: l.GetDefault("OIDC_AUDIENCE", "sytue-docs"),

		HubBaseURL:             l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		HubInternalSecret:      l.HubInternalSecret(base.Env),
		RoomsURL:               l.GetDefault("ROOMS_URL", "http://rooms:8085"),
		RoomsCallAccessSecret:  l.Get("ROOMS_CALL_ACCESS_SECRET"),
		EncryptedDocumentCalls: l.GetBoolDefault("ENCRYPTED_DOCUMENT_CALLS", false),
		IDBaseURL:              l.GetDefault("ID_BASE_URL", "http://id:8081"),
		IDDirectorioSecret:     l.Get("ID_DIRECTORIO_SECRET"),
		IDOrgsSecret:           l.Get("ID_ORGS_SECRET"),
		SMTPAddr:               l.Get("SMTP_ADDR"),
		SMTPFrom:               l.Get("SMTP_FROM"),
		SMTPUser:               l.Get("SMTP_USER"),
		SMTPPassword:           l.Get("SMTP_PASSWORD"),
		SMTPTLSServerName:      l.Get("SMTP_TLS_SERVER_NAME"),
		OIDCAcceptedAudiences:  l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", config.FirstPartyAudiences()),

		S3Endpoint:  l.Get("S3_ENDPOINT"),
		S3Region:    l.GetDefault("S3_REGION", "fr-par"),
		S3AccessKey: l.Get("S3_ACCESS_KEY"),
		S3SecretKey: l.Get("S3_SECRET_KEY"),
		S3UseSSL:    l.GetBoolDefault("S3_USE_SSL", false),
		S3Remote: objectstore.Remote(objectstore.Config{
			Endpoint:     l.Get("S3_REMOTE_ENDPOINT"),
			Region:       l.GetDefault("S3_REMOTE_REGION", "fr-par"),
			AccessKey:    l.Get("S3_REMOTE_ACCESS_KEY"),
			SecretKey:    l.Get("S3_REMOTE_SECRET_KEY"),
			Bucket:       l.Get("S3_REMOTE_BUCKET"),
			UseSSL:       l.GetBoolDefault("S3_REMOTE_USE_SSL", true),
			StorageClass: l.GetDefault("S3_REMOTE_STORAGE_CLASS", "ONEZONE_IA"),
		}),
		S3LocalTenants: l.GetCSV("S3_LOCAL_TENANTS"),
		S3Bucket:       l.GetDefault("S3_BUCKET", "docs-blobs"),

		SnapshotInlineLimit: l.GetIntDefault("SNAPSHOT_INLINE_LIMIT", 256*1024),
		DragonflyURL:        l.Get("DRAGONFLY_URL"),
		DragonflyPassword:   l.Get("DRAGONFLY_PASSWORD"),
		OCRURL:              l.Get("OCR_URL"),
		OCRTimeout:          l.GetDurationDefault("OCR_TIMEOUT", 90*time.Second),
		SelloURL:            l.Get("SELLO_URL"),
		SelloPolitica:       l.Get("SELLO_POLITICA"),
		SelloUsuario:        l.Get("SELLO_USUARIO"),
		SelloClave:          l.Get("SELLO_CLAVE"),
		SelloTimeout:        l.GetDurationDefault("SELLO_TIMEOUT", 20*time.Second),
		IntegrationsURL:     l.GetDefault("INTEGRATIONS_URL", "http://integrations:8093"),
		BasesURL:            l.GetDefault("BASES_URL", "http://bases:8102"),
		DriveURL:            l.GetDefault("DRIVE_URL", "http://drive:8082"),
		DriveTimeout:        l.GetDurationDefault("DRIVE_TIMEOUT", 15*time.Second),
		DriveInternalSecret: l.SecretoOpcional(base.Env, "DRIVE_INTERNAL_SECRET", config.DevDocsDriveInternalSecret),
		PublicURL:           l.GetDefault("PUBLIC_URL", ""),
		WopiHosts:           splitLista(l.GetDefault("WOPI_ALLOWED_HOSTS", "")),
	}

	// Clave de cifrado de credenciales de conectores (base64 → 32 bytes AES-256).
	// Opcional; si se aporta, debe decodificar a exactamente 32 bytes (fallo rápido
	// ante una clave mal configurada, en vez de arrancar con cifrado roto).
	if raw := l.Get("SECRET_ENC_KEY"); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SYTUE_DOCS_SECRET_ENC_KEY: base64 inválido: %w", err)
		}
		if len(key) != 32 {
			return Config{}, fmt.Errorf("SYTUE_DOCS_SECRET_ENC_KEY: debe medir 32 bytes (AES-256), tiene %d", len(key))
		}
		cfg.SecretEncKey = key
	}
	if pemAnclas := l.Get("SELLO_ANCLAS"); pemAnclas != "" {
		cfg.SelloAnclas = x509.NewCertPool()
		if !cfg.SelloAnclas.AppendCertsFromPEM([]byte(pemAnclas)) {
			return Config{}, fmt.Errorf("SYTUE_DOCS_SELLO_ANCLAS: no trae ningún certificado PEM")
		}
	}
	if pemAnclas := l.Get("FIRMA_ANCLAS"); pemAnclas != "" {
		cfg.FirmaAnclas = x509.NewCertPool()
		if !cfg.FirmaAnclas.AppendCertsFromPEM([]byte(pemAnclas)) {
			return Config{}, fmt.Errorf("SYTUE_DOCS_FIRMA_ANCLAS: no trae ningún certificado PEM")
		}
	}
	if cfg.Base.Env.IsProd() && len(cfg.SecretEncKey) != 32 {
		return Config{}, fmt.Errorf("SYTUE_DOCS_SECRET_ENC_KEY es obligatoria en prod (AES-256)")
	}

	if cfg.EncryptedDocumentCalls && (cfg.RoomsURL == "" || cfg.RoomsCallAccessSecret == "") {
		return Config{}, fmt.Errorf("encrypted document calls require ROOMS_URL and ROOMS_CALL_ACCESS_SECRET")
	}
	return cfg, l.Err()
}

// S3Configured indica si hay credenciales S3 (Object Storage habilitado).
func (c Config) S3Configured() bool {
	return c.S3Endpoint != "" && c.S3AccessKey != "" && c.S3SecretKey != ""
}

// splitLista parte una lista separada por comas descartando lo vacío.
func splitLista(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
