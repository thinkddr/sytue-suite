// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"

	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
)

func (f *fakeService) MessageMetadata(context.Context, string, string, int) (roomsearch.Page, error) {
	return roomsearch.Page{}, nil
}

func (f *fakeService) MessageMetadataVisible(context.Context, string, []roomsearch.Identity) ([]roomsearch.Metadata, error) {
	return nil, nil
}
