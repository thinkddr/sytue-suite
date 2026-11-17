// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// Only authenticated typed projections and immutable revision identities are
// cached. Unknown relationship JSON remains in its original encrypted record.
type membershipProjection struct {
	Entry     records.Entry
	Row       model.MembershipRow
	Signature string
	Scope     storage.Scope
}

func membershipRevision(entry records.Entry) string {
	raw, _ := json.Marshal(entry)
	defer clear(raw)
	return planBytesHash(raw)
}

func (r *Repository) retireAlbumProjections() {
	r.mediaMu.Lock()
	defer r.mediaMu.Unlock()
	r.albumProjectionsRetired = true
	r.membershipCache = nil
	r.albumCheckpoints = nil
}

type albumCheckpointProjection struct {
	Set     albumMemberSet
	Files   []records.RetainedFile
	Members []model.MembershipRow
}

func (r *Repository) rememberMemberships(entries []records.Entry, rows []model.Row) error {
	byID := map[string]records.Entry{}
	for _, entry := range entries {
		if entry.Kind == "album_asset" {
			byID[entry.ID] = entry
		}
	}
	cache := make(map[string]membershipProjection, len(byID))
	for _, row := range rows {
		if row.Kind != "album_asset" {
			continue
		}
		var member model.MembershipRow
		if json.Unmarshal(row.JSON, &member) != nil {
			return ErrSource
		}
		entry, ok := byID[row.ID]
		if !ok {
			return ErrSource
		}
		cache[row.ID] = membershipProjection{Entry: entry, Row: member, Signature: membershipRevision(entry), Scope: r.scope}
	}
	r.mediaMu.Lock()
	if !r.albumProjectionsRetired {
		r.membershipCache = cache
	}
	r.mediaMu.Unlock()
	return nil
}

func (r *Repository) indexedMemberships(ctx context.Context, catalog records.Catalog) (map[string]membershipProjection, error) {
	entries, err := catalog.Entries()
	if err != nil {
		return nil, err
	}
	r.mediaMu.RLock()
	previous := r.membershipCache
	r.mediaMu.RUnlock()
	current := map[string]membershipProjection{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Kind != "album_asset" {
			continue
		}
		signature := membershipRevision(entry)
		if cached, ok := previous[entry.ID]; ok && cached.Scope == r.scope && cached.Signature == signature {
			current[entry.ID] = cached
			continue
		}
		raw, err := catalog.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		var member model.MembershipRow
		err = json.Unmarshal(raw, &member)
		clear(raw)
		if err != nil || member.Tenant != r.scope.Tenant {
			return nil, ErrSource
		}
		current[entry.ID] = membershipProjection{Entry: entry, Row: member, Signature: signature, Scope: r.scope}
	}
	r.mediaMu.Lock()
	if !r.albumProjectionsRetired {
		r.membershipCache = current
	}
	r.mediaMu.Unlock()
	return current, nil
}

// The candidate catalog has already authenticated this complete immutable
// entry. Only graph validation may reuse its previously verified projection;
// ordinary Snapshot and edit paths continue to read the exact retained JSON.
func (r *Repository) membershipValidationRow(ctx context.Context, entry records.Entry) (model.Row, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.Row{}, false, err
	}
	r.mediaMu.RLock()
	cached, ok := r.membershipCache[entry.ID]
	retired := r.albumProjectionsRetired
	r.mediaMu.RUnlock()
	if !ok || retired || cached.Scope != r.scope || cached.Signature != membershipRevision(entry) {
		return model.Row{}, false, nil
	}
	raw, err := json.Marshal(cached.Row)
	if err != nil {
		return model.Row{}, false, err
	}
	if err := ctx.Err(); err != nil {
		clear(raw)
		return model.Row{}, false, err
	}
	return model.Row{Kind: entry.Kind, ID: entry.ID, JSON: raw}, true, nil
}
