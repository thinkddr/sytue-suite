// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package card

import (
	"errors"
	"testing"
)

func TestEncodeRejectsUnreadableBase(t *testing.T) {
	encoders := []struct {
		name   string
		encode func(string) (string, error)
	}{
		{"contact", func(base string) (string, error) {
			return EncodeContact(Contact{UID: "person", FormattedName: "Ada"}, base)
		}},
		{"group", func(base string) (string, error) {
			return EncodeGroup(Group{UID: "group", Name: "Equipo"}, base)
		}},
	}
	for _, encoder := range encoders {
		t.Run(encoder.name, func(t *testing.T) {
			for _, base := range []string{"BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada\r\n", " \r\n"} {
				got, err := encoder.encode(base)
				if !errors.Is(err, ErrInvalidVCard) || got != "" {
					t.Fatalf("unreadable base: got %q, error %v; want empty result and ErrInvalidVCard", got, err)
				}
			}
			got, err := encoder.encode("")
			if err != nil || got == "" {
				t.Fatalf("empty base: got %q, error %v; want a newly encoded card", got, err)
			}
		})
	}
}
