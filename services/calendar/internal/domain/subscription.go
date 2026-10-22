// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Suscripción a un calendario ICS externo (RFC 5545 servido por HTTP): el
// «Añadir calendario por URL». Una suscripción es un calendario de SOLO LECTURA
// (Calendar.ReadOnly) cuyos VEVENT se obtienen de una URL http(s) y se cachean.
// Este fichero define el modelo y las reglas de negocio independientes de
// transporte/almacenamiento.
package domain

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// Errores de dominio específicos de las suscripciones. (Los generales viven en
// domain.go.)
var (
	// ErrReadOnly: intento de escribir un evento en un calendario de solo lectura
	// (una suscripción ICS externa). La capa HTTP/CalDAV lo mapea a 403.
	ErrReadOnly = errors.New("calendar: calendario de solo lectura (suscripción)")
	// ErrInvalidURL: la URL de suscripción no es http/https con host, o apunta a
	// un destino no permitido (SSRF). Mapea a 400.
	ErrInvalidURL = errors.New("calendar: URL de suscripción inválida")
)

// Límites de la cadencia de refresco de una suscripción. Un feed no se retrae
// más de una vez cada MinRefreshInterval (protege a los servidores de festivos/
// equipo y acota el trabajo del worker) ni menos de una vez cada
// MaxRefreshInterval (una suscripción no se queda indefinidamente rancia).
const (
	MinRefreshInterval     = 15 * time.Minute
	MaxRefreshInterval     = 7 * 24 * time.Hour
	DefaultRefreshInterval = time.Hour
)

// ClampRefreshInterval acota una cadencia de refresco a [Min, Max]. Un valor <= 0
// (no especificado) degrada al valor por defecto.
func ClampRefreshInterval(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultRefreshInterval
	}
	if d < MinRefreshInterval {
		return MinRefreshInterval
	}
	if d > MaxRefreshInterval {
		return MaxRefreshInterval
	}
	return d
}

// Subscription es el estado de sincronización de un calendario de suscripción
// ICS externa (1:1 con el calendario). Se expone al cliente para «gestionar»
// (renombrar/color/refrescar/eliminar) sin filtrar los validadores condicionales
// (ETag/Last-Modified son detalle interno del refresco; json:"-").
type Subscription struct {
	URL             string     `json:"url"`
	RefreshInterval int        `json:"refresh_interval_seconds"`
	LastFetchedAt   *time.Time `json:"last_fetched_at,omitempty"`
	NextRefreshAt   time.Time  `json:"next_refresh_at"`
	LastStatus      string     `json:"last_status,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	// Validadores condicionales del último 200 (RFC 9110): internos al refresco.
	ETag         string `json:"-"`
	LastModified string `json:"-"`
}

// ValidSubscriptionURL valida que una URL sea apta como feed ICS: esquema
// http/https y host no vacío. NO resuelve DNS (la barrera SSRF que valida las IPs
// vive en la capa de fetch); aquí solo se descartan esquemas peligrosos
// (file://, javascript:, data:, ...) y las URLs sin host. Clean-room.
func ValidSubscriptionURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Hostname() == "" {
		return false
	}
	// Sin credenciales embebidas (evita fugas y confusión de origen).
	if u.User != nil {
		return false
	}
	return true
}
