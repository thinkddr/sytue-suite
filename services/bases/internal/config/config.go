// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue Bases desde el entorno
// (12-factor), sin prefijo. La lista de variables es CERRADA: el gate de
// helm-render-test.sh exige que el chart pase solo lo que algún config.go lee
// con Get/Require/SecretoOpcional literal.
package config

import (
	"errors"
	"fmt"
	"os"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
)

// ServiceName es el nombre lógico del servicio (atributo de logs y telemetría).
const ServiceName = "sytue-bases"

// DefaultPort es el puerto HTTP de Sytue Bases.
const DefaultPort = "8102"

// devAltchaSecret es el de desarrollo: público a propósito, y rechazado fuera de dev.
const devAltchaSecret = "dev-bases-altcha-secret-32-bytes!!" //nolint:gosec // G101: de desarrollo, público

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=...").
var Version = "dev"

// Config es la configuración completa de la API de Sytue Bases.
type Config struct {
	pcfg.Base

	DatabaseURL string
	DBMaxConns  int

	// OIDC: JWT de Sytue ID. Se acepta el token si su aud contiene ALGUNA de las
	// audiencias de primera parte.
	OIDCIssuer            string
	OIDCJWKSURL           string
	OIDCAcceptedAudiences []string

	// AltchaSecret firma los retos de los formularios públicos. En dev cae a uno público;
	// fuera de dev es obligatorio (un reto firmado con una clave pública no frena a nadie).
	AltchaSecret string

	// PublicURL es la raíz pública de la web de Bases (https://bases.sytue.eu): el host
	// de los enlaces de los avisos (hubnotify.EnlaceRegistroDeBase).
	PublicURL string
	// HubBaseURL y HubInternalSecret avisan en el centro de notificaciones de los
	// comentarios y las menciones.
	HubBaseURL        string
	HubInternalSecret string
	// IDBaseURL es Sytue ID por dentro del clúster: de ahí salen el nombre de quien
	// comenta y la persona detrás de una mención, con la sesión de quien pregunta.
	IDBaseURL string

	// Almacén de objetos de los adjuntos (bucket bases-blobs), con las mismas variables
	// que los demás servicios. Opcional: sin credenciales, el servicio arranca y los
	// adjuntos responden 503.
	S3 objectstore.Config
}

// S3Configurado dice si hay almacén de objetos.
func (c Config) S3Configurado() bool {
	return c.S3.Endpoint != "" && c.S3.AccessKey != "" && c.S3.SecretKey != ""
}

// Cargar lee la configuración.
func Cargar() (Config, error) {
	l := pcfg.NewLoader("")
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Config{}, err
	}
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}
	// PORT (compose y chart) manda sobre HTTP_ADDR.
	if port := l.Get("PORT"); port != "" {
		base.HTTPAddr = ":" + port
	} else if base.HTTPAddr == "" || base.HTTPAddr == ":8080" {
		base.HTTPAddr = ":" + DefaultPort
	}
	cfg := Config{
		Base:        base,
		DatabaseURL: l.Require("DATABASE_URL"),
		DBMaxConns:  l.GetIntDefault("DB_MAX_CONNS", 10),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences()),

		AltchaSecret: l.SecretoOpcional(base.Env, "BASES_ALTCHA_SECRET", devAltchaSecret),

		PublicURL:         l.GetDefault("PUBLIC_URL", "https://bases.sytue.localhost"),
		HubBaseURL:        l.GetDefault("HUB_BASE_URL", "http://hub:8092"),
		HubInternalSecret: l.HubInternalSecret(base.Env),
		IDBaseURL:         l.GetDefault("ID_BASE_URL", "http://id:8081"),

		S3: objectstore.Config{
			Endpoint:  l.Get("S3_ENDPOINT"),
			Region:    l.GetDefault("S3_REGION", "fr-par"),
			AccessKey: l.Get("S3_ACCESS_KEY"),
			SecretKey: l.Get("S3_SECRET_KEY"),
			UseSSL:    l.GetBoolDefault("S3_USE_SSL", false),
			Bucket:    l.GetDefault("S3_BUCKET", "bases-blobs"),
			Remote: objectstore.Remote(objectstore.Config{
				Endpoint:     l.Get("S3_REMOTE_ENDPOINT"),
				Region:       l.GetDefault("S3_REMOTE_REGION", "fr-par"),
				AccessKey:    l.Get("S3_REMOTE_ACCESS_KEY"),
				SecretKey:    l.Get("S3_REMOTE_SECRET_KEY"),
				Bucket:       l.Get("S3_REMOTE_BUCKET"),
				UseSSL:       l.GetBoolDefault("S3_REMOTE_USE_SSL", true),
				StorageClass: l.GetDefault("S3_REMOTE_STORAGE_CLASS", "ONEZONE_IA"),
			}),
			LocalTenants: l.GetCSV("S3_LOCAL_TENANTS"),
		},
	}
	if err := l.Err(); err != nil {
		return Config{}, fmt.Errorf("configuración: %w", err)
	}
	if base.Env != pcfg.EnvDev && cfg.AltchaSecret == "" {
		return Config{}, errors.New("BASES_ALTCHA_SECRET es obligatorio fuera de desarrollo")
	}
	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}
	return cfg, nil
}
