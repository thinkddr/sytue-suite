// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Photos desde el entorno del
// proceso (12-factor). Reutiliza el cargador común de platform/pkg/config y
// respeta EXACTAMENTE los nombres de variable del compose. Sin secretos en git.
package config

import (
	"os"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/exif"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

// exifPorDefecto sale del parser, que es quien sabe cuánto ocupa un APP1.
const exifPorDefecto = exif.BytesDeCabecera

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-syphotos"

// DefaultPort es el puerto HTTP de Sytue Photos (CONVENTIONS §6). 8097 es el
// siguiente libre tras el OCR interno (8096).
const DefaultPort = "8097"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=...").
var Version = "dev"

// Config es la configuración completa de Sytue Photos.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://photos.sytue.eu).
	PublicURL string

	// DatabaseURL es el DSN PostgreSQL. Única fuente de verdad.
	DatabaseURL string
	// DBMaxConns es el tamaño máximo del pool.
	DBMaxConns int

	// DriveBaseURL es dónde vive Sytue Drive, que custodia los ORIGINALES.
	// Sytue Photos no funciona sin él: no es una integración opcional, es dónde
	// están las fotos.
	DriveBaseURL string
	// DriveTimeout acota una lectura. Generoso porque una foto puede pesar
	// decenas de MB y el original se lee entero para generar las derivadas; la
	// lectura de CABECERA, que es la operación frecuente, tarda milisegundos.
	DriveTimeout time.Duration

	// SytueAIURL y SytueAIToken son la vía SERVERLESS de vectorización: la
	// descripción de la foto la escribe la visión de gemma-4 (/assist) y el
	// vector de esa descripción sale de qwen3-embedding-8b (/embeddings).
	// Sustituye a la GPU de lote (deploy/fotos-gpu, retirada 2026-09-19).
	// Sin URL, el lote serverless no corre (el resto del servicio sí).
	SytueAIURL   string
	SytueAIToken string

	// HubBaseURL y HubInternalSecret son el centro de notificaciones: el aviso de
	// «X ha compartido un álbum contigo» (`photos.shared`).
	HubBaseURL        string
	HubInternalSecret string
	// IDBaseURL es Sytue ID por dentro del clúster: con la sesión de quien comparte,
	// comprueba que la persona es de su organización (`directory:resolve`).
	IDBaseURL string

	// OIDC configura la validación de tokens de Sytue ID (JWKS).
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// OIDCAcceptedAudiences: la suite comparte sesión, así que el token puede
	// traer la audiencia de OTRA app. Se acepta si su aud contiene ALGUNA
	// (allowlist first-party).
	OIDCAcceptedAudiences []string

	// TrashRetention es cuánto permanece en la papelera lo borrado antes de que
	// el worker lo destruya. A cero, nada caduca y la papelera solo se vacía a
	// mano: es una decisión de despliegue, no de código.
	TrashRetention time.Duration
	// TrashGCInterval es cada cuánto despierta el worker. La retención se mide
	// en días y esto en horas: despertar más a menudo no purga antes, solo
	// consulta más.
	TrashGCInterval time.Duration

	// BytesDeCabecera es cuánto se lee del original para sacar el EXIF. El valor
	// por defecto sale del propio parser; se deja configurable porque hay
	// ficheros —algunos RAW, algunos HEIC— que colocan sus metadatos más
	// adelante, y subirlo es más barato que leer el fichero entero.
	BytesDeCabecera int64

	// LadosDeDerivada son los tamaños que se generan al ingerir. Salen del
	// dominio y no de una constante escrita aquí: dos listas se separan.
	LadosDeDerivada []int

	// S3 es dónde viven las DERIVADAS. Bucket propio y no el de Drive: lo que
	// hay aquí es caché regenerable, así que su ciclo de vida —caducidad,
	// clase de almacenamiento, exclusión de la copia de seguridad— es distinto
	// del de un fichero de alguien, y mezclarlos obliga a distinguirlos por
	// prefijo en cada regla.
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool
	// S3Remote es el almacén remoto (S3_REMOTE_*), donde va todo tenant salvo
	// los de S3LocalTenants; nil = todo a S3_*. Ver objectstore.Remote (ADR 0038).
	S3Remote       *objectstore.Config
	S3LocalTenants []string
}

// Load construye la configuración leyendo el entorno.
func Load() (Config, error) {
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Config{}, err
	}
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}

	l := pcfg.NewLoader("")

	lados := make([]int, 0, len(domain.Variantes()))
	for _, v := range domain.Variantes() {
		if n, ok := v.LadoMayor(); ok {
			lados = append(lados, n)
		}
	}

	cfg := Config{
		Base:        base,
		PublicURL:   l.GetDefault("PUBLIC_URL", "https://photos.sytue.localhost"),
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		DriveBaseURL: l.GetDefault("DRIVE_URL", "http://drive:8082"),
		DriveTimeout: l.GetDurationDefault("DRIVE_TIMEOUT", 2*time.Minute),

		SytueAIURL:   l.Get("SYTUE_AI_URL"),
		SytueAIToken: l.Get("SYTUE_AI_TOKEN"),

		HubBaseURL:        l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		HubInternalSecret: l.HubInternalSecret(base.Env),
		IDBaseURL:         l.GetDefault("ID_BASE_URL", "http://id:8081"),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-syphotos"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		TrashRetention:  l.GetDurationDefault("TRASH_RETENTION", 30*24*time.Hour),
		TrashGCInterval: l.GetDurationDefault("TRASH_GC_INTERVAL", time.Hour),

		BytesDeCabecera: int64(l.GetIntDefault("EXIF_HEADER_BYTES", exifPorDefecto)),
		LadosDeDerivada: lados,

		S3Endpoint:  l.GetDefault("S3_ENDPOINT", "minio:9000"),
		S3Region:    l.GetDefault("S3_REGION", "fr-par"),
		S3AccessKey: l.Require("S3_ACCESS_KEY"),
		S3SecretKey: l.Require("S3_SECRET_KEY"),
		S3Bucket:    l.GetDefault("S3_BUCKET", "sytue-dev-syphotos-derivadas"),
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
	}

	// El esquema del endpoint, si lo trae, manda sobre S3_USE_SSL (como en Drive).
	cfg.S3Endpoint, cfg.S3UseSSL = pcfg.NormalizeS3Endpoint(cfg.S3Endpoint, cfg.S3UseSSL)

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":" + DefaultPort
	}

	if err := l.Err(); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}
	return cfg, nil
}
