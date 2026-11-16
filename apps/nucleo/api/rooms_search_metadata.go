// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"net/url"
	"strconv"

	roomsearch "git.sytue.eu/sytue/one/apps/nucleo/rooms/search"
)

func (r *Rooms) MessageMetadata(ctx context.Context, cursor string, limit int) (roomsearch.Page, error) {
	return JSON[roomsearch.Page](ctx, r.c, Request{Path: "/messages/metadata", Query: url.Values{"cursor": {cursor}, "limit": {strconv.Itoa(limit)}}})
}

func (r *Rooms) MessageMetadataVisible(ctx context.Context, identities []roomsearch.Identity) ([]roomsearch.Metadata, error) {
	out, err := PostJSON[struct {
		Messages []roomsearch.Metadata `json:"messages"`
	}](ctx, r.c, "/messages/metadata/visible", map[string]any{"messages": identities})
	return out.Messages, err
}
