// SPDX-License-Identifier: Apache-2.0
package hubnotify

import (
	"net/url"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// TextoAvisoRooms contains no conversation content or participant information.
const TextoAvisoRooms = "Nuevo mensaje en Rooms"

// AvisoMensajeRooms is the only content Rooms sends to external destinations.
// The link opens the authenticated app; it carries identifiers, never keys or
// access grants. Invalid configuration fails closed instead of sending a broken
// link or falling back to the original message.
func AvisoMensajeRooms(publicURL, org, canal, mensaje string) string {
	u, err := url.Parse(publicURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !tenancy.ValidSlug(org) {
		return ""
	}
	id, err := uuid.Parse(canal)
	if err != nil || id == uuid.Nil {
		return ""
	}
	if mensaje != "" {
		id, err = uuid.Parse(mensaje)
		if err != nil || id == uuid.Nil {
			return ""
		}
	}
	return TextoAvisoRooms + "\n" + EnlaceMensaje(publicURL, org, canal, mensaje)
}
