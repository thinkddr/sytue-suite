// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

var _ api.ContactsPhotoReader = (*Repository)(nil)

// PhotoByID reads the displayed contact and its photo from one private
// snapshot. The ETag is a card revision check, not a global write fence.
func (r *Repository) PhotoByID(parent context.Context, id, etag string) ([]byte, error) {
	var photo []byte
	err := r.core.ProjectCatalog(parent, func(ctx context.Context, catalog records.Catalog) error {
		snapshot, err := readCatalog(ctx, r.scope, catalog)
		if err != nil {
			return err
		}
		photo, err = snapshot.queryPhotoByID(id, etag)
		return err
	})
	if err != nil {
		clear(photo)
		return nil, err
	}
	return photo, nil
}

func (s *Snapshot) queryPhotoByID(id, etag string) ([]byte, error) {
	if strings.TrimSpace(etag) == "" {
		return nil, queryError(http.StatusBadRequest, "contact_etag_required")
	}
	person, err := s.FindContact(id)
	if err != nil {
		return nil, err
	}
	if person.ETag != etag {
		return nil, queryError(http.StatusPreconditionFailed, "contact_etag_changed")
	}
	value, err := card.PhotoDataURI(person.VCard)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, queryError(http.StatusNotFound, "private_contact_photo_not_found")
	}
	photo, _, err := card.PhotoBytes(value)
	return photo, err
}
