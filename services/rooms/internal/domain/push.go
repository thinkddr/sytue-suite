// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "strings"

// PushSubscription es una suscripción Web Push (RFC 8291) de un navegador/dispositivo
// del usuario (Ola 18). El servidor cifra el payload con las claves públicas del cliente
// (P256dh + Auth) y lo entrega al servicio de push del fabricante en Endpoint.
type PushSubscription struct {
	// Endpoint es la URL https del servicio de push (única por suscripción).
	Endpoint string `json:"endpoint"`
	// P256dh es la clave pública P-256 del cliente (base64url, punto sin comprimir de 65 bytes).
	P256dh string `json:"p256dh"`
	// Auth es el secreto de autenticación del cliente (base64url, 16 bytes).
	Auth string `json:"auth"`
}

// PushTarget es una suscripción a la que enviar un push, con su dueño (Subject) para
// poder limpiarla si el servicio de push la reporta caduca (404/410).
type PushTarget struct {
	Subject  string
	Endpoint string
	P256dh   string
	Auth     string
}

// NotificationRecipient identifica a un usuario destinatario de una notificación de mensaje.
type NotificationRecipient struct {
	Subject   string
	Mentioned bool
}

// Topes de una suscripción push (evitan abuso y filas gigantes).
const (
	maxPushEndpointLen = 2048
	maxPushKeyLen      = 256
)

// ValidPushSubscription valida la forma de una suscripción antes de persistirla: endpoint
// https no vacío y acotado, y claves base64url no vacías y acotadas. La validación fina
// (SSRF del endpoint, decodificación de las claves) la hace la capa de servicio.
func ValidPushSubscription(s PushSubscription) bool {
	ep := strings.TrimSpace(s.Endpoint)
	if ep == "" || len(ep) > maxPushEndpointLen || !strings.HasPrefix(ep, "https://") {
		return false
	}
	if s.P256dh == "" || len(s.P256dh) > maxPushKeyLen {
		return false
	}
	if s.Auth == "" || len(s.Auth) > maxPushKeyLen {
		return false
	}
	return true
}
