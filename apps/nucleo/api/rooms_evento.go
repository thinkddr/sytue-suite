// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// SalaDeEvento crea —o recupera, si ya existe— la sala de videollamada de Sytue Rooms
// ligada a un evento de Sytue Calendar por su UID, y devuelve el enlace para entrar.
//
// Es lo que hace el BFF de Calendar en la web (`pages/api/conference.ts`): el evento no
// «contiene» la reunión, la REFERENCIA (RFC 7986 CONFERENCE). El UID tiene que ser el
// mismo que se guarde en el evento: con otro, cada reapertura crearía una sala nueva y
// el enlace repartido dejaría de servir.
//
// `publica` es la raíz pública de Rooms (ServiceURL(apex, "rooms")); el enlace es el de
// [EnlaceReunion].
func (r *Rooms) SalaDeEvento(ctx context.Context, uid, titulo, publica string) (string, error) {
	if strings.TrimSpace(titulo) == "" {
		titulo = "Reunión"
	}
	sala, err := PostJSON[struct {
		ID      string `json:"id"`
		LinkKey string `json:"link_key"`
	}](ctx, r.c, "/rooms/from-source", map[string]string{"kind": "calendar", "ref_id": uid, "title": titulo})
	if err != nil {
		return "", err
	}
	// Con la clave: la reunión de un evento es suelta y sus invitados entran por el enlace.
	return EnlaceReunion(publica, sala.ID, sala.LinkKey), nil
}

// EnlaceReunion es `enlaceReunion` de la web (@sytue/config): `https://rooms.<dominio>/?rooms=<id>`,
// en el host PROPIO de Rooms y en su raíz, que es lo que decidió el dueño el 2026-09-25.
// `meet` era el parámetro anterior y ya no se genera; la web lo sigue leyendo como alias.
// `clave` es la de una reunión suelta (decidido por el dueño el 2026-10-16: «Invitados y
// quien tenga el enlace»): sin ella solo entran quien la convocó y quien ya estuvo dentro.
func EnlaceReunion(publica, id, clave string) string {
	enlace := strings.TrimRight(publica, "/") + "/?rooms=" + url.QueryEscape(id)
	if clave != "" {
		enlace += "&clave=" + url.QueryEscape(clave)
	}
	return enlace
}

// ClaveDeReunion es la clave que trae el enlace de una reunión ("" si no lleva).
func ClaveDeReunion(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("clave"))
}

// reUUIDReunion es la forma del id de una sala: el enlace lo puede escribir cualquiera,
// y lo que no es un id no llega al servicio.
var reUUIDReunion = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// LeerReunion es la reunión de un enlace [EnlaceReunion] (`?rooms=<id>`, o el antiguo
// parámetro `meet`) que llega a la app de Rooms por cualquiera de sus dos puertas:
//
//   - por el esquema, `sytue-rooms:/?rooms=<id>`: lo que deja [DesdeLaWeb] cuando la
//     página de la reunión abre la app en escritorio;
//   - tal cual, `https://rooms.<apex>/?rooms=<id>`: el App Link de Android y el
//     Universal Link de iOS, que el sistema entrega a la app sin pasar por el navegador.
//
// Solo la raíz de Rooms (con o sin su prefijo) y, por https, solo el host de Rooms de
// esta suite.
func LeerReunion(apex string, u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}
	switch u.Scheme {
	case Esquema("rooms"):
	case "https":
		if !strings.EqualFold(u.Hostname(), hostDe["rooms"]+"."+apex) {
			return "", false
		}
	default:
		return "", false
	}
	if strings.Trim(strings.TrimPrefix(u.Path, rutaDe["rooms"]), "/") != "" {
		return "", false
	}
	for _, clave := range []string{"rooms", "meet"} {
		if id := strings.TrimSpace(u.Query().Get(clave)); reUUIDReunion.MatchString(id) {
			return id, true
		}
	}
	return "", false
}
