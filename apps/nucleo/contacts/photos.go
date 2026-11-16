// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

func (r *Repository) SetPhoto(ctx context.Context, uid, data string) error {
	canonical, err := card.ValidatePhotoDataURI(data)
	if err != nil {
		return err
	}
	_, err = r.mutateAggregate(ctx, "set-photo", struct{ UID, Data string }{uid, canonical}, func(snapshot *Snapshot) (Aggregate, error) {
		return snapshot.ChangePhoto(uid, canonical, false, time.Now().UTC())
	})
	return err
}

func (r *Repository) RemovePhoto(ctx context.Context, uid string) error {
	_, err := r.mutateAggregate(ctx, "remove-photo", struct{ UID string }{uid}, func(snapshot *Snapshot) (Aggregate, error) {
		return snapshot.ChangePhoto(uid, "", true, time.Now().UTC())
	})
	return err
}

func (s *Snapshot) ChangePhoto(uid, data string, remove bool, now time.Time) (Aggregate, error) {
	current, err := s.photoContact(uid)
	if err != nil {
		return Aggregate{}, err
	}
	var updated string
	if remove {
		updated, err = card.RemoveCardPhoto(current.VCard)
	} else {
		canonical, validationErr := card.ValidatePhotoDataURI(data)
		if validationErr != nil {
			return Aggregate{}, validationErr
		}
		updated, err = card.SetCardPhoto(current.VCard, canonical)
	}
	if err != nil {
		return Aggregate{}, err
	}
	change, err := editAggregate(s, "contact_state", current.ID)
	if err != nil {
		return Aggregate{}, err
	}
	for i, row := range change.Rows {
		switch row.Kind {
		case "card":
			change.Rows[i], err = patchRow(row, map[string]any{"data": updated, "size_bytes": len(updated), "has_photo": card.HasEmbeddedPhoto(updated), "etag": uuid.NewString(), "updated_at": now})
		case "contact":
			change.Rows[i], err = patchRow(row, map[string]any{"photo_url": "", "updated_at": now})
		}
		if err != nil {
			return Aggregate{}, err
		}
	}
	return change, change.Validate(s.Scope())
}
