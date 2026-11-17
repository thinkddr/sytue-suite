// SPDX-License-Identifier: Apache-2.0
package domain

import (
	"fmt"
	"strings"

	"github.com/emersion/go-vcard"

	card "git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

const bdayLayout = "2006-01-02"

func EncodeContact(c Contact, base string) (string, error) { return card.EncodeContact(c, base) }
func DecodeContact(data string) (Contact, error)           { return card.DecodeContact(data) }
func ImportCard(data string) (Contact, string, error)      { return card.ImportCard(data) }
func EncodeGroup(g Group, base string) (string, error)     { return card.EncodeGroup(g, base) }
func DecodeGroup(data string) (Group, error)               { return card.DecodeGroup(data) }
func KindOf(data string) Kind                              { return card.KindOf(data) }
func CleanUID(uid string) string                           { return card.CleanUID(uid) }

func parseSingle(data string) (vcard.Card, error) {
	dec := vcard.NewDecoder(strings.NewReader(data))
	c, err := dec.Decode()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidVCard, err)
	}
	return c, nil
}
