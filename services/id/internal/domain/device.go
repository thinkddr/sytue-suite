// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"slices"
	"strings"
	"time"
)

// Device es un dispositivo (navegador) desde el que un usuario inicia sesión
// (paridad Entra device-based Conditional Access). Se registra al primer login y se
// identifica en los siguientes por una cookie firmada. `Trusted` es POR (usuario,
// dispositivo): un dispositivo de confianza exime al login de una política de Acceso
// Condicional con `require_trusted_device`.
type Device struct {
	ID        string
	OrgID     string
	UserID    string
	DeviceID  string // identificador derivado (HMAC), único global; NO es el valor de la cookie
	Name      string
	UserAgent string
	Trusted   bool
	FirstSeen time.Time
	LastSeen  time.Time
	LastIP    string
	// Current marca, solo en listados (ListDevicesForUser), el dispositivo desde el que
	// se hace la petición (cookie presente y verificada). No se persiste.
	Current bool
	// UserEmail/UserName: solo en el listado admin por org (ListDevicesForOrg, join con
	// users) para mostrar al dueño. No se persisten en la tabla devices.
	UserEmail string
	UserName  string
}

// Etiquetas de dominio para separar los usos de la clave HMAC de dispositivo (firma de
// la cookie vs. derivación del device_id): nunca se reutiliza el mismo MAC para ambos.
const (
	deviceCookieLabel = "sytue-id/device-cookie/v1\x00"
	deviceIDLabel     = "sytue-id/device-id/v1\x00"
)

// SignDeviceCookie produce el valor de la cookie de dispositivo: el secreto de
// navegador `b` (opaco, generado con GenerateToken) más su HMAC-SHA256, separados por
// un punto. La cookie NO contiene el device_id (que se deriva aparte). httpOnly + esta
// firma ⇒ no falsificable sin la clave del servicio.
func SignDeviceCookie(key []byte, b string) string {
	return b + "." + deviceMAC(key, deviceCookieLabel, b)
}

// ParseDeviceCookie valida la firma de la cookie de dispositivo y devuelve el secreto
// de navegador `b`. Comparación en tiempo constante (hmac.Equal). ok=false si la cookie
// está malformada o la firma no verifica (posible falsificación) — se trata como
// dispositivo desconocido.
func ParseDeviceCookie(key []byte, cookie string) (string, bool) {
	i := strings.LastIndexByte(cookie, '.')
	if i <= 0 || i == len(cookie)-1 {
		return "", false
	}
	b, sig := cookie[:i], cookie[i+1:]
	want := deviceMAC(key, deviceCookieLabel, b)
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return "", false
	}
	return b, true
}

// DeriveDeviceID deriva el identificador estable del dispositivo por HMAC del secreto
// de navegador `b` LIGADO al usuario. Que user_id entre en el MAC hace el device_id
// único por usuario (dos cuentas en el mismo navegador ⇒ dispositivos distintos: la
// confianza es por usuario) y a la vez estable (mismo (usuario, navegador) ⇒ mismo id
// en cada login, sin duplicados). Opaco: no revela el secreto de la cookie.
func DeriveDeviceID(key []byte, userID, b string) string {
	return deviceMAC(key, deviceIDLabel, userID+"\x00"+b)
}

// deviceMAC calcula base64url(HMAC-SHA256(key, label || msg)) — sin padding, URL-safe.
func deviceMAC(key []byte, label, msg string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(label))
	m.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Valores del claim `device_trust` (docs/security.md §11.4.1). Es lo que un servicio
// puede exigir para un recurso `restricted` o `value_ip`.
const (
	// DeviceTrustNone: nada que garantice el dispositivo. Es lo que vale cualquier
	// token sin los requisitos de abajo, también los anteriores a este claim.
	DeviceTrustNone = "none"
	// DeviceTrustStandard: la sesión se abrió desde un dispositivo REGISTRADO de la
	// persona que ella misma marcó de confianza desde ese dispositivo (prueba de
	// posesión: la cookie firmada), Y se identificó con una passkey, el único factor
	// atado a un autenticador y resistente a phishing. Las dos cosas: un dispositivo
	// de confianza sin passkey lo puede declarar cualquiera que tenga la contraseña, y
	// una passkey sin dispositivo de confianza no dice nada del equipo desde el que se
	// usa.
	DeviceTrustStandard = "standard"
	// DeviceTrustManaged: dispositivo gestionado por la organización (Sytue Linux o
	// Sytue Android inscritos). RESERVADO: Sytue ID no tiene todavía inscripción de
	// dispositivos gestionados, así que no lo emite nunca. Se nombra para que nadie
	// reutilice el valor con otro significado.
	DeviceTrustManaged = "managed"
)

// EvaluarConfianzaDelDispositivo decide `device_trust` a partir de cómo se
// identificó la sesión (`amr`) y de si su dispositivo está hoy marcado de confianza.
func EvaluarConfianzaDelDispositivo(amr []string, dispositivoDeConfianza bool) string {
	if dispositivoDeConfianza && slices.Contains(amr, "webauthn") {
		return DeviceTrustStandard
	}
	return DeviceTrustNone
}
