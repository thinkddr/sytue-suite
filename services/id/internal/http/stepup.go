// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// REAUTENTICACIÓN FUERTE (step-up) para las acciones de Sytue Admin que no se
// deshacen.
//
// Lo que había: el step-up del IdP se decide UNA vez, al emitir la credencial
// (authorize.go → requiresStepUp), y a partir de ahí el access token vale igual
// para leer el directorio que para rotar el secreto de un cliente OIDC. Con
// refresh, además, un access token recién emitido puede respaldarse en una
// sesión de hace dos semanas: `iat` es de hace un minuto y la persona se
// identificó hace catorce días. Un portátil desatendido con la consola abierta
// bastaba para rotar secretos, cambiar el dominio de correo de toda la
// plantilla o resetear el segundo factor de otra persona.
//
// Lo que exige esto, en la petición y no en el login:
//
//  1. que la sesión acredite SEGUNDO FACTOR (`amr`), y
//  2. que la persona se haya identificado HACE POCO (`auth_time`).
//
// Las dos juntas. Solo la primera no dice nada sobre quién está delante ahora
// mismo; solo la segunda deja pasar una contraseña reciente en un equipo ajeno.
//
// La respuesta es la del RFC 9470: 401 con `insufficient_user_authentication`
// y una cabecera `WWW-Authenticate` que dice QUÉ falta (`max_age`), para que el
// cliente sepa mandar a la persona a `/authorize?prompt=login&max_age=…` en vez
// de enseñar un «prohibido» que parece un fallo de permisos.
package http

import (
	"net/http"
	"strconv"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
)

// frescuraStepUp es cuánto vale una identificación para las acciones
// sensibles. Cinco minutos: lo bastante para encadenar varios cambios de la
// misma sesión de trabajo sin volver a pedir nada, y no tanto como para cubrir
// un café con la pantalla desbloqueada.
const frescuraStepUp = 5 * time.Minute

// errStepUp es la respuesta cuando falta reautenticación. Código estable para
// que el cliente lo distinga de un 403 de permisos: son cosas distintas —una se
// arregla identificándose otra vez, la otra pidiéndole el permiso a alguien—.
var errStepUp = httpx.NewError(http.StatusUnauthorized, "insufficient_user_authentication",
	"This action needs you to confirm your identity again.")

// requireStepUp exige segundo factor y una identificación reciente.
//
// Va DESPUÉS del gate de permisos de su grupo de rutas, nunca en su lugar: no
// autoriza a nadie, solo comprueba que quien ya estaba autorizado está delante
// ahora mismo.
func (s *Server) requireStepUp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := claimsFrom(r.Context())
		if !ok {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		fresca := !c.AuthTime.IsZero() && time.Since(c.AuthTime) <= frescuraStepUp
		if hasMFA(c.AMR) && fresca {
			next.ServeHTTP(w, r)
			return
		}
		// El operador necesita distinguir los dos motivos: «entró sin segundo
		// factor» y «hace demasiado que se identificó» se arreglan de formas
		// distintas. A quien llama se le contesta lo mismo en los dos casos.
		s.log.WarnContext(r.Context(), "step-up: acción sensible sin reautenticación",
			"user_id", c.Subject, "path", logsafe.Text(r.URL.Path), "amr", c.AMR,
			"mfa", hasMFA(c.AMR), "auth_time_fresca", fresca)
		// RFC 9470: decir qué falta, para que el cliente pueda pedirlo.
		w.Header().Set("WWW-Authenticate",
			`Bearer error="insufficient_user_authentication", `+
				`error_description="reauthentication required", `+
				`max_age=`+strconv.Itoa(int(frescuraStepUp.Seconds())))
		httpx.Fail(w, r, errStepUp)
	})
}
