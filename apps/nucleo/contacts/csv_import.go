// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var _ api.ContactsData = (*Repository)(nil)

func (r *Repository) ImportCSV(ctx context.Context, book string, raw []byte) (int, int, error) {
	if len(raw) > 5<<20 {
		return 0, 0, queryError(http.StatusRequestEntityTooLarge, "csv_too_large")
	}
	parsed, skipped, err := card.ParseContactsCSV(bytes.NewReader(raw))
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
	scope, err := storage.EncodeScopeHeader(r.scope)
	if err != nil {
		return 0, 0, err
	}
	imported := 0
	for i, person := range parsed {
		if err := ctx.Err(); err != nil {
			return imported, skipped, err
		}
		person.UID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("sytue-contacts-csv-v1\x00"+scope+"\x00"+book+"\x00"+fingerprint+"\x00"+strconv.Itoa(i))).String()
		_, err := r.mutateAggregate(ctx, "csv-row", struct {
			Book, Digest string
			Row          int
		}{book, fingerprint, i}, func(current *Snapshot) (Aggregate, error) {
			duplicate, err := current.csvDuplicate(book, person)
			if err != nil {
				return Aggregate{}, err
			}
			if duplicate {
				return Aggregate{}, errCSVExisting
			}
			return current.NewPerson(book, person, "", time.Now().UTC())
		})
		if errors.Is(err, errCSVExisting) {
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

var errCSVExisting = errors.New("contacts: CSV row already exists")

func (s *Snapshot) csvDuplicate(book string, candidate card.Contact) (bool, error) {
	people, err := s.visiblePeople()
	if err != nil {
		return false, err
	}
	addresses := map[string]bool{}
	for _, email := range candidate.Emails {
		addresses[strings.ToLower(strings.TrimSpace(email.Email))] = true
	}
	for _, person := range people {
		if person.AddressBookID != book {
			continue
		}
		if person.UID == candidate.UID {
			return true, nil
		}
		for _, email := range person.Emails {
			if addresses[strings.ToLower(strings.TrimSpace(email.Email))] {
				return true, nil
			}
		}
	}
	return false, nil
}
