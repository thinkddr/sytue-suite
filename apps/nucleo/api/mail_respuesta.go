// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"time"
)

// MailRespuesta es la respuesta automática de UNA cuenta, guardada en el servidor de esa
// cuenta (Sieve) y no en el servicio de Sytue: la de Sytue Office (PLAN-EDICION-LOCAL
// §6.9, M-10). La edición conectada sigue con [Mail.Vacation] y [Mail.SetVacation].
type MailRespuesta struct {
	// Cuenta es el identificador de la cuenta y Direccion su correo.
	Cuenta, Direccion string
	// Activa: responde ahora (o en las fechas puestas).
	Activa          bool
	Asunto, Mensaje string
	// Dias entre dos respuestas a la misma persona.
	Dias int
	// Desde y Hasta son el primer y el último día (incluidos); nil, sin límite. Solo con
	// ConFechas.
	Desde, Hasta *time.Time
	// ConFechas: el servidor admite fechas en la respuesta. Sin ellas, responde mientras
	// esté activada.
	ConFechas bool
	// Reglas es el script de la persona que sigue funcionando detrás de la respuesta (el
	// que estaba activo): al apagarla vuelve a quedar como estaba. Vacío si no hay.
	Reglas string
	// Bloqueada es el script de la persona que impide activarla sin tocar sus reglas (el
	// servidor no sabe combinarlas, o ya responden o rechazan correo por su cuenta). Vacío
	// si se puede.
	Bloqueada string
}

// MailRespuestas es la respuesta automática por cuenta, para quien la guarda en el
// servidor de cada cuenta. Es OPCIONAL (§5.7): *Mail no la cumple, y la pantalla de Mail
// sigue entonces con su diálogo de siempre.
type MailRespuestas interface {
	// CuentasRespuesta son las cuentas que PODRÍAN tenerla, sin preguntar a nadie: las de
	// un proveedor que se sabe que no la ofrece no salen.
	CuentasRespuesta() []MailAlias
	// LeerRespuesta pregunta al servidor de la cuenta (con un plazo corto: también es la
	// prueba de si la ofrece). Si no la ofrece, un error que es [ErrMailSinRespuesta].
	LeerRespuesta(ctx context.Context, cuenta string) (MailRespuesta, error)
	// GuardarRespuesta la escribe en el servidor de la cuenta, sin tocar las reglas que la
	// persona ya tenga allí.
	GuardarRespuesta(ctx context.Context, cuenta string, r MailRespuesta) error
}

// ErrMailSinRespuesta: el servidor de la cuenta no ofrece respuesta automática (o no se
// llega a él para saberlo). La pantalla no enseña la sección para esa cuenta.
var ErrMailSinRespuesta = errors.New("mail: el servidor de esta cuenta no ofrece respuesta automática")
