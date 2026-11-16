// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"github.com/google/uuid"

	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
)

func (f *fakeStore) MessageMetadata(context.Context, string, *roomsearch.Position, int, []uuid.UUID) ([]roomsearch.Metadata, error) {
	return nil, nil
}

func (f *fakeStore) SearchIdentityActive(context.Context, string) (bool, error) { return true, nil }
