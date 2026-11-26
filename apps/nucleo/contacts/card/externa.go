// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package card

import (
	"fmt"

	"github.com/emersion/go-vcard"
)

// ImportCardConUID es ImportCard para una tarjeta que llega de una cuenta sincronizada
// (CardDAV, Google, Microsoft; `local/agenda`): conserva su UID —es el que une la copia
// de este equipo con la del servidor— y solo pone uno nuevo si no lo trae.
func ImportCardConUID(data string) (Contact, string, error) {
	card, err := parseSingle(data)
	if err != nil {
		return Contact{}, "", err
	}
	fotoEnLinea(card)
	uid := cleanURN(card.Value(vcard.FieldUID))
	if uid == "" {
		uid = NewUID()
	}
	card.SetValue(vcard.FieldUID, ensureURN(uid))
	base, err := marshal(card)
	if err != nil {
		return Contact{}, "", fmt.Errorf("%w: %w", ErrInvalidVCard, err)
	}
	c, err := DecodeContact(base)
	if err != nil {
		return Contact{}, "", err
	}
	c.UID = uid
	out, err := EncodeContact(c, base)
	return c, out, err
}
