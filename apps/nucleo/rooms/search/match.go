// SPDX-License-Identifier: Apache-2.0
package search

import (
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// Document exists only on the authorized device. Body and attachment presence
// come from authenticated decrypted content, never a server text index.
type Document struct {
	Body        string    `json:"body"`
	AuthorSub   string    `json:"author_sub"`
	AuthorName  string    `json:"author_name"`
	ChannelSlug string    `json:"channel_slug"`
	ChannelName string    `json:"channel_name"`
	CreatedAt   time.Time `json:"created_at"`
	HasFile     bool      `json:"has_file"`
	Saved       bool      `json:"saved"`
	Pinned      bool      `json:"pinned"`
}

func (q Query) MetadataMatches(d Document) bool {
	if q.From != "" && d.AuthorSub != q.From && !buscar.Contiene(d.AuthorName, q.From) {
		return false
	}
	if q.In != "" && !buscar.Contiene(d.ChannelSlug, q.In) && !buscar.Contiene(d.ChannelName, q.In) {
		return false
	}
	if q.After != nil && d.CreatedAt.Before(*q.After) {
		return false
	}
	if q.Before != nil && !d.CreatedAt.Before(*q.Before) {
		return false
	}
	return (!q.Saved || d.Saved) && (!q.Pinned || d.Pinned)
}

func (q Query) Matches(d Document) bool {
	if !q.MetadataMatches(d) || (q.HasFile && !d.HasFile) {
		return false
	}
	if q.HasLink {
		body := strings.ToLower(d.Body)
		if !strings.Contains(body, "http://") && !strings.Contains(body, "https://") {
			return false
		}
	}
	body := buscar.Plegar(d.Body)
	for _, term := range q.Terms {
		if !strings.Contains(body, buscar.Plegar(term)) {
			return false
		}
	}
	for _, term := range q.Not {
		if strings.Contains(body, buscar.Plegar(term)) {
			return false
		}
	}
	return true
}
