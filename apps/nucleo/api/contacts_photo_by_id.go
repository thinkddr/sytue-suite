// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"strings"
)

// ContactsPhotoReader is an optional private-data capability. The supplied
// revision must identify the card whose photo is returned; implementations
// must reject a missing or mismatched revision. The returned buffer belongs to
// the caller even on error and must not alias shared or retained provider data;
// the caller may clear it after use or cancellation.
type ContactsPhotoReader interface {
	PhotoByID(context.Context, string, string) ([]byte, error)
}

// PhotoByID reads a private photo for a contact and its displayed card revision.
// It never resolves the ID through a UID or falls back to the legacy service.
func (c *Contacts) PhotoByID(ctx context.Context, id, etag string) ([]byte, error) {
	if c == nil || c.data == nil || ctx == nil || strings.TrimSpace(id) == "" || strings.TrimSpace(etag) == "" {
		return nil, ErrPrivateContactsOperation
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, ok := c.data.(ContactsPhotoReader)
	if !ok {
		return nil, ErrPrivateContactsOperation
	}
	photo, err := reader.PhotoByID(ctx, id, etag)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		clear(photo)
		return nil, err
	}
	return photo, nil
}

// PhotoForContact binds a private photo to the displayed contact and card
// revision. Only a genuinely legacy client uses the existing UID endpoint;
// an unavailable private capability never falls back to it.
func (c *Contacts) PhotoForContact(ctx context.Context, contact Contact) ([]byte, error) {
	if c == nil || ctx == nil {
		return nil, ErrPrivateContactsOperation
	}
	if c.data != nil {
		if strings.TrimSpace(contact.AddressBookID) == "" {
			return nil, ErrPrivateContactsOperation
		}
		return c.PhotoByID(ctx, contact.ID, contact.ETag)
	}
	if strings.TrimSpace(CleanUID(contact.UID)) == "" {
		return nil, ErrPrivateContactsOperation
	}
	photo, err := c.Photo(ctx, contact.UID)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		clear(photo)
		return nil, err
	}
	return photo, nil
}
