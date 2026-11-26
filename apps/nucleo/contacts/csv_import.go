// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

var _ api.ContactsData = (*Repository)(nil)

func (r *Repository) ImportCSV(ctx context.Context, book string, raw []byte) (int, int, error) {
	parsed, skipped, err := FilasCSV(r.scope, book, raw)
	if err != nil {
		return 0, 0, err
	}
	snapshot, err := r.Snapshot(ctx)
	if err != nil {
		return 0, 0, err
	}
	if _, err := snapshot.QueryBook(book, ""); err != nil {
		return 0, 0, err
	}
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	imported := 0
	for i, person := range parsed {
		if err := ctx.Err(); err != nil {
			return imported, skipped, err
		}
		_, err := r.mutateAggregate(ctx, "csv-row", struct {
			Book, Digest string
			Row          int
		}{book, fingerprint, i}, func(current *Snapshot) (Aggregate, error) {
			return current.PrepararFilaCSV(book, person, time.Now().UTC())
		})
		if errors.Is(err, ErrFilaCSVRepetida) {
			skipped++
			continue
		}
		if err != nil {
			return imported, skipped, err
		}
		imported++
	}
	return imported, skipped, nil
}

func (s *Snapshot) csvDuplicate(book string, candidate card.Contact) (bool, error) {
	addresses := map[string]bool{}
	for _, email := range candidate.Emails {
		addresses[strings.ToLower(strings.TrimSpace(email.Email))] = true
	}
	duplicate := false
	err := s.cadaVisible(func(person *ContactView) bool {
		if person.AddressBookID != book {
			return true
		}
		if person.UID == candidate.UID {
			duplicate = true
			return false
		}
		for _, email := range person.Emails {
			if addresses[strings.ToLower(strings.TrimSpace(email.Email))] {
				duplicate = true
				return false
			}
		}
		return true
	})
	if err != nil {
		return false, err
	}
	return duplicate, nil
}
