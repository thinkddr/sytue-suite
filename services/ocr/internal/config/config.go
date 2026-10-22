// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración de Sytue OCR desde el entorno
// (12-factor), sobre el cargador común de platform/pkg/config.
//
// NO HAY NINGÚN SECRETO AQUÍ, y no es casualidad: el motor está en la imagen y
// no se llama a nada de fuera. Sin clave que guardar, sin clave que filtrar.
package config

import (
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio.
const ServiceName = "sytue-ocr"

// Version la inyecta el linker en build.
var Version = "dev"

// Config es la configuración completa de Sytue OCR.
type Config struct {
	pcfg.Base

	// OIDC valida el JWT del usuario, que Sytue Docs reenvía tal cual. No hay
	// secreto compartido entre servicios: quien pide un OCR es alguien con
	// sesión, y queda atribuido.
	OIDCIssuer            string
	OIDCJWKSURL           string
	OIDCAudience          string
	OIDCAcceptedAudiences []string

	// Timeout por página. Un escaneo a 600 ppp de un plano puede tardar; el tope
	// impide que una sola petición ate un núcleo indefinidamente.
	Timeout time.Duration

	// MaxConcurrentes acota cuántos reconocimientos corren a la vez.
	//
	// Existe porque Tesseract usa la CPU a tope y NO respeta el límite del pod:
	// con veinte peticiones simultáneas, el planificador de Kubernetes estrangula
	// el contenedor y las veinte tardan veinte veces más en vez de encolarse. Es
	// mejor una cola corta que un montón de trabajos a cámara lenta.
	MaxConcurrentes int
}

// Load lee y valida la configuración.
func Load() (Config, error) {
	var c Config
	base, err := pcfg.LoadBase("SYTUE_OCR", ServiceName)
	if err != nil {
		return c, err
	}
	c.Base = base

	l := pcfg.NewLoader("SYTUE_OCR")
	c.OIDCIssuer = strings.TrimRight(l.Require("OIDC_ISSUER"), "/")
	c.OIDCJWKSURL = l.Get("OIDC_JWKS_URL")
	c.OIDCAudience = l.GetDefault("OIDC_AUDIENCE", "sytue-ocr")
	// La suite comparte sesión, así que el token que reenvía Docs trae SU
	// audiencia, no la de este servicio. Sin esta lista, todas las peticiones se
	// rechazarían con 401 y el OCR no funcionaría nunca.
	c.OIDCAcceptedAudiences = l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences("sytue-ocr"))
	if c.OIDCJWKSURL == "" && c.OIDCIssuer != "" {
		c.OIDCJWKSURL = c.OIDCIssuer + "/.well-known/jwks.json"
	}

	c.Timeout = l.GetDurationDefault("TIMEOUT", 60*time.Second)
	c.MaxConcurrentes = l.GetIntDefault("MAX_CONCURRENTES", 2)
	if c.MaxConcurrentes < 1 {
		c.MaxConcurrentes = 1
	}
	return c, l.Err()
}
