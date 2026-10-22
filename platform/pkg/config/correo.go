// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package config

import "git.sytue.eu/sytue/one/platform/pkg/correo"

// Correo es el buzón de envío del servicio: SMTP_ADDR, SMTP_FROM, SMTP_USER,
// SMTP_PASSWORD y SMTP_TLS_SERVER_NAME, con el prefijo del Loader. Es la MISMA forma
// en todos los servicios que mandan correo (Calendar, Docs, el ERP y los avisos de
// Drive, Notepads y Mail), que es lo que deja al chart ponerla con un solo helper
// (`sytue.correoEnv`). Sin SMTP_FROM, `Activo()` es false y no sale nada.
func (l *Loader) Correo() correo.Config {
	return correo.Config{
		Addr: l.Get("SMTP_ADDR"), From: l.Get("SMTP_FROM"),
		User: l.Get("SMTP_USER"), Password: l.Get("SMTP_PASSWORD"),
		TLSServerName: l.Get("SMTP_TLS_SERVER_NAME"),
	}
}
