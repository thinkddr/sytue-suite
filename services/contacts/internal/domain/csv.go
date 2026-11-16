// SPDX-License-Identifier: Apache-2.0
package domain

import (
	"io"
	"time"

	card "git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

func ParseContactsCSV(r io.Reader) ([]Contact, int, error) { return card.ParseContactsCSV(r) }
func WriteContactsCSV(w io.Writer, contacts []Contact) error {
	return card.WriteContactsCSV(w, contacts)
}
func parseCSVBirthday(v string) (*time.Time, string) { return card.ParseCSVBirthday(v) }
