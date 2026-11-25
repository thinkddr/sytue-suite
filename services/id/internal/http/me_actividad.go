// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"sort"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// «Actividad reciente» de Mi cuenta: lo que ha pasado en la cuenta —inicios de
// sesión, cambios de contraseña, del segundo factor, de las passkeys…— con su fecha
// y la dirección desde la que se hizo. Es lo que una persona mira cuando le llega un
// aviso de seguridad (avisos_seguridad.go) y quiere saber si fue ella.
//
// La fuente es el log de auditoría, que ya se escribe en cada uno de esos sitios; lo
// nuevo es la lectura. Y no se expone la acción tal cual: el nombre interno del log
// cambia cuando cambia quien lo escribe, y la pantalla no tiene por qué enterarse.
// `clasesDeActividad` traduce las acciones a una clase PÚBLICA y estable, y es a la
// vez la lista blanca: lo que no está aquí —lo que la persona hizo como
// administradora, por ejemplo— no sale.
var clasesDeActividad = map[string]string{
	"auth.login.success":             "signin",
	"auth.webauthn.success":          "signin",
	"auth.federated.login":           "signin",
	"auth.saml.login":                "signin",
	"auth.login.locked":              "signin_locked",
	"user.password.changed":          "password_changed",
	"user.password.reset":            "password_changed",
	"mfa.totp.enabled":               "mfa_enabled",
	"mfa.totp.disabled":              "mfa_disabled",
	"mfa.totp.recovery_regenerated":  "recovery_codes",
	"webauthn.credential.registered": "passkey_added",
	"webauthn.credential.removed":    "passkey_removed",
	"webauthn.credential.renamed":    "passkey_renamed",
	"app_password.created":           "app_password_created",
	"app_password.revoked":           "app_password_revoked",
	"session.revoked":                "sessions_closed",
	"session.revoked_others":         "sessions_closed",
	"device.trusted":                 "device_trusted",
	"device.revoked":                 "device_removed",
	"user.recovery_email.verified":   "recovery_changed",
	"user.recovery_email.removed":    "recovery_changed",
	"user.phone.changed":             "recovery_changed",
	"user.username.changed":          "username_changed",
}

const (
	// diasDeActividad: la ventana que enseña la pantalla. Lo que un aviso de
	// seguridad hace venir a mirar es de los últimos días; tres meses dejan ver
	// también el cambio que nadie recuerda haber hecho.
	diasDeActividad = 90
	// topeDeActividad: lo que cabe en una tarjeta. Quien necesita el log entero lo
	// tiene en «Exportar tus datos».
	topeDeActividad = 50
)

// accionesDeActividad es la lista blanca de acciones en un orden fijo, para que la
// consulta sea siempre la misma (y su plan, el mismo).
func accionesDeActividad() []string {
	out := make([]string, 0, len(clasesDeActividad))
	for a := range clasesDeActividad {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// sucesoPublico es una fila de la respuesta.
type sucesoPublico struct {
	ID   int64     `json:"id"`
	Kind string    `json:"kind"`
	At   time.Time `json:"at"`
	IP   string    `json:"ip,omitempty"`
}

// handleMeActivity — GET /api/v1/me/activity — la actividad reciente de la cuenta de
// quien llama. Va en el grupo `persona` (ruta de seguridad de la propia cuenta) y,
// por estar bajo /me/, solo la atiende un client de la suite: una app de terceros con
// `openid profile` no tiene por qué saber desde dónde entra alguien.
func (s *Server) handleMeActivity(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	filas, err := s.store.ActividadDePersona(r.Context(), c.Subject, accionesDeActividad(), diasDeActividad, topeDeActividad)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	out := make([]sucesoPublico, 0, len(filas))
	for _, f := range filas {
		clase, ok := clasesDeActividad[f.Action]
		if !ok {
			continue // la consulta ya filtra; esto solo evita una clase vacía
		}
		out = append(out, sucesoPublico{ID: f.ID, Kind: clase, At: f.OccurredAt.UTC(), IP: f.IP})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"events": out, "days": diasDeActividad})
}
