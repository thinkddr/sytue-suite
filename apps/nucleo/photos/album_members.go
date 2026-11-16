// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const (
	albumPageKind        = "album-page"
	albumIndexKind       = "album-index"
	albumMembersField    = "private_members"
	albumPageMembers     = 64
	albumPageTargetBytes = 1 << 20
)

type albumMemberSet struct {
	Version int                  `json:"v"`
	Root    records.RetainedFile `json:"root"`
}

type albumPageReference struct {
	File  records.RetainedFile `json:"file"`
	Count int                  `json:"count"`
}

// A checkpoint has direct page references, never a chain of prior mutations.
// History remains in prior album revisions; opening the current set downloads
// only its manifest and current pages, regardless of the number of past edits.
type albumManifest struct {
	Version int                  `json:"v"`
	Scope   storage.Scope        `json:"scope"`
	Album   string               `json:"album"`
	Pages   []albumPageReference `json:"pages"`
}

type albumMemberPage struct {
	Version  int               `json:"v"`
	Scope    storage.Scope     `json:"scope"`
	Album    string            `json:"album"`
	Members  []json.RawMessage `json:"members"`
	retained *records.RetainedFile
}

func clearAlbumPage(page albumMemberPage) {
	for _, raw := range page.Members {
		clear(raw)
	}
}

func (r *Repository) albumSet(raw []byte) (*albumMemberSet, error) {
	var row model.AlbumRow
	if json.Unmarshal(raw, &row) != nil {
		return nil, ErrSource
	}
	if len(row.PrivateMembers) == 0 {
		return nil, nil //nolint:nilnil // No checkpoint means the original retained relation rows remain authoritative.
	}
	set, err := r.decodeAlbumSet(row.PrivateMembers)
	return &set, err
}

func (r *Repository) decodeAlbumSet(raw []byte) (albumMemberSet, error) {
	var set albumMemberSet
	if json.Unmarshal(raw, &set) != nil || set.Version != 1 || !r.albumFileShape(set.Root, albumIndexKind) {
		return set, ErrSource
	}
	return set, nil
}

func (r *Repository) albumFileShape(file records.RetainedFile, kind string) bool {
	return file.Version == 1 && file.Scope == r.scope && file.Target.Valid() && file.Link.Valid() && file.Link.RoomID == file.Target.RoomID && storage.Identifier(file.Revision) && file.Row.Kind == kind && file.Row.Validate() == nil
}

func readAlbumFile(ctx context.Context, catalog records.Catalog, file records.RetainedFile, value any) error {
	reader, ok := catalog.(records.RetainedFileContentCatalog)
	if !ok {
		return ErrSource
	}
	body := storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error { return reader.ReadRetainedFile(ctx, file, out) })
	defer func() { _ = body.Close() }()
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(value); err != nil {
		return err
	}
	// EOF also joins the SDK's final size/digest check. A valid JSON prefix is
	// never accepted when the authenticated body is truncated or has a suffix.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return ErrSource
	}
	return ctx.Err()
}

func (r *Repository) readAlbumManifest(ctx context.Context, catalog records.Catalog, album string, set albumMemberSet) (albumManifest, error) {
	var manifest albumManifest
	if err := readAlbumFile(ctx, catalog, set.Root, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Version != 1 || manifest.Scope != r.scope || manifest.Album != album {
		return manifest, ErrSource
	}
	seen := map[storage.Reference]bool{set.Root.Link: true}
	for _, page := range manifest.Pages {
		if page.Count <= 0 || page.Count > albumPageMembers || !r.albumFileShape(page.File, albumPageKind) || seen[page.File.Link] {
			return manifest, ErrSource
		}
		seen[page.File.Link] = true
	}
	return manifest, nil
}

func (r *Repository) readAlbumPages(ctx context.Context, catalog records.Catalog, album string, set albumMemberSet) ([]albumMemberPage, error) {
	manifest, err := r.readAlbumManifest(ctx, catalog, album, set)
	if err != nil {
		return nil, err
	}
	pages := make([]albumMemberPage, 0, len(manifest.Pages))
	success := false
	defer func() {
		if !success {
			for _, page := range pages {
				clearAlbumPage(page)
			}
		}
	}()
	seen := map[string]bool{}
	for _, ref := range manifest.Pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var page albumMemberPage
		if err := readAlbumFile(ctx, catalog, ref.File, &page); err != nil {
			clearAlbumPage(page)
			return nil, err
		}
		pages = append(pages, page)
		if page.Version != 1 || page.Scope != r.scope || page.Album != album || len(page.Members) != ref.Count {
			return nil, ErrSource
		}
		for _, raw := range page.Members {
			member, err := r.decodeAlbumMember(album, raw)
			if err != nil || seen[member.AssetID] {
				return nil, ErrSource
			}
			seen[member.AssetID] = true
		}
		file := ref.File
		pages[len(pages)-1].retained = &file
	}
	success = true
	return pages, nil
}

func (r *Repository) decodeAlbumMember(album string, raw []byte) (model.MembershipRow, error) {
	var member model.MembershipRow
	if json.Unmarshal(raw, &member) != nil || member.AlbumID != album || member.Tenant != r.scope.Tenant {
		return member, ErrSource
	}
	id, err := model.MembershipID(member.AlbumID, member.AssetID)
	if err != nil || model.ValidateRow(r.scope, model.Row{Kind: "album_asset", ID: id, JSON: raw}) != nil {
		return member, ErrSource
	}
	return member, nil
}

func (r *Repository) validateAlbumMemberships(ctx context.Context, catalog records.Catalog, raw []byte, assets map[string]bool, reuseProjection ...bool) error {
	set, err := r.albumSet(raw)
	if err != nil || set == nil {
		return err
	}
	var album model.AlbumRow
	if json.Unmarshal(raw, &album) != nil {
		return ErrSource
	}
	if len(reuseProjection) > 0 && reuseProjection[0] {
		reused, err := r.reuseAlbumValidation(ctx, catalog, album.ID, *set, assets)
		if err != nil || reused {
			return err
		}
	}
	pages, err := r.readAlbumPages(ctx, catalog, album.ID, *set)
	if err != nil {
		return err
	}
	defer func() {
		for _, page := range pages {
			clearAlbumPage(page)
		}
	}()
	projection := albumCheckpointProjection{Set: *set, Files: []records.RetainedFile{set.Root}, Members: []model.MembershipRow{}}
	for _, page := range pages {
		if page.retained == nil {
			return ErrSource
		}
		projection.Files = append(projection.Files, *page.retained)
		for _, raw := range page.Members {
			member, err := r.decodeAlbumMember(album.ID, raw)
			if err != nil || !assets[member.AssetID] {
				return ErrSource
			}
			projection.Members = append(projection.Members, member)
		}
	}
	r.mediaMu.Lock()
	if !r.albumProjectionsRetired {
		if r.albumCheckpoints == nil {
			r.albumCheckpoints = map[string]albumCheckpointProjection{}
		}
		r.albumCheckpoints[album.ID] = projection
	}
	r.mediaMu.Unlock()
	return nil
}

func (r *Repository) reuseAlbumValidation(ctx context.Context, catalog records.Catalog, album string, set albumMemberSet, assets map[string]bool) (bool, error) {
	r.mediaMu.RLock()
	cached, reused := r.albumCheckpoints[album]
	retired := r.albumProjectionsRetired
	r.mediaMu.RUnlock()
	if !reused || retired || cached.Set != set {
		return false, nil
	}
	retained, ok := catalog.(records.RetainedFileCatalog)
	if !ok {
		return false, ErrSource
	}
	for _, file := range cached.Files {
		if err := retained.ValidateRetainedFile(ctx, file); err != nil {
			return false, err
		}
	}
	for _, member := range cached.Members {
		if !assets[member.AssetID] || member.Tenant != r.scope.Tenant || member.AlbumID != album {
			return false, ErrSource
		}
	}
	return true, ctx.Err()
}

func (r *Repository) validatePrivateAlbums(ctx context.Context, catalog records.Catalog, rows []model.Row, validationOnly bool) error {
	assets := map[string]bool{}
	for _, row := range rows {
		if row.Kind == "asset" {
			assets[row.ID] = true
		}
	}
	for _, row := range rows {
		if row.Kind != "album" {
			continue
		}
		if err := r.validateAlbumMemberships(ctx, catalog, row.JSON, assets, validationOnly); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) checkpointMembers(ctx context.Context, catalog records.Catalog, album model.AlbumRow) ([]model.MembershipRow, error) {
	set, err := r.decodeAlbumSet(album.PrivateMembers)
	if err != nil {
		return nil, err
	}
	r.mediaMu.RLock()
	projection, cached := r.albumCheckpoints[album.ID]
	r.mediaMu.RUnlock()
	if cached && projection.Set == set {
		retained, ok := catalog.(records.RetainedFileCatalog)
		if !ok {
			return nil, ErrSource
		}
		for _, file := range projection.Files {
			if err := retained.ValidateRetainedFile(ctx, file); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return slices.Clone(projection.Members), nil
	}
	pages, err := r.readAlbumPages(ctx, catalog, album.ID, set)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, page := range pages {
			clearAlbumPage(page)
		}
	}()
	members := []model.MembershipRow{}
	for _, page := range pages {
		for _, raw := range page.Members {
			var member model.MembershipRow
			if json.Unmarshal(raw, &member) != nil {
				return nil, ErrSource
			}
			members = append(members, member)
		}
	}
	return members, nil
}

func (r *Repository) legacyAlbumPages(ctx context.Context, album string) ([]albumMemberPage, error) {
	catalog, err := r.engine.Catalog()
	if err != nil {
		return nil, err
	}
	indexed, err := r.indexedMemberships(ctx, catalog)
	if err != nil {
		return nil, err
	}
	type retainedMember struct {
		row model.MembershipRow
		raw json.RawMessage
	}
	members := []retainedMember{}
	defer func() {
		for _, member := range members {
			clear(member.raw)
		}
	}()
	for _, projection := range indexed {
		if projection.Row.AlbumID != album {
			continue
		}
		raw, err := catalog.ReadJSON(ctx, projection.Entry)
		if err != nil {
			return nil, err
		}
		var member model.MembershipRow
		if json.Unmarshal(raw, &member) != nil {
			clear(raw)
			return nil, ErrSource
		}
		if _, err := r.decodeAlbumMember(album, raw); err != nil {
			clear(raw)
			return nil, err
		}
		members = append(members, retainedMember{member, raw})
	}
	slices.SortFunc(members, func(a, b retainedMember) int {
		if a.row.Position < b.row.Position {
			return -1
		}
		if a.row.Position > b.row.Position {
			return 1
		}
		return strings.Compare(a.row.AssetID, b.row.AssetID)
	})
	pages := []albumMemberPage{}
	for _, member := range members {
		if len(pages) == 0 || albumPageFull(pages[len(pages)-1], len(member.raw)) {
			pages = append(pages, albumMemberPage{Version: 1, Scope: r.scope, Album: album, Members: []json.RawMessage{}})
		}
		pages[len(pages)-1].Members = append(pages[len(pages)-1].Members, bytes.Clone(member.raw))
	}
	return pages, nil
}

// The byte target bounds ordinary pages, not existing album or row sizes. A
// larger historical row remains intact on its own page; no prefix is retained.
func albumPageFull(page albumMemberPage, additional int) bool {
	if len(page.Members) >= albumPageMembers {
		return true
	}
	if len(page.Members) == 0 {
		return false
	}
	size := int64(additional)
	for _, member := range page.Members {
		size += int64(len(member))
	}
	return size > albumPageTargetBytes
}

type albumChange struct {
	Album  string   `json:"album"`
	Add    []string `json:"add,omitempty"`
	Remove string   `json:"remove,omitempty"`
}

type albumChangeOrigin struct {
	Version   int                       `json:"v"`
	Scope     storage.Scope             `json:"scope"`
	Target    storage.PublicationTarget `json:"target"`
	Operation string                    `json:"operation"`
}

type albumChangeResult struct {
	Operation string `json:"operation"`
	Added     int    `json:"added"`
}

func (r *Repository) AddToAlbum(ctx context.Context, album string, ids []string) (int, error) {
	return r.changeAlbum(ctx, albumChange{Album: album, Add: slices.Clone(ids)})
}

func (r *Repository) RemoveFromAlbum(ctx context.Context, album, asset string) error {
	if _, err := model.MembershipID(album, asset); err != nil {
		return model.ErrInvalidInput
	}
	_, err := r.changeAlbum(ctx, albumChange{Album: album, Remove: asset})
	return err
}

func (r *Repository) changeAlbum(ctx context.Context, change albumChange) (int, error) {
	input, err := json.Marshal(change)
	if err != nil {
		return 0, err
	}
	defer clear(input)
	operation := "album-members-v1_" + planBytesHash(input)
	result, err := r.engine.MutateWithContent(ctx, "album-members-v1", change, func(ctx context.Context) (string, string, string, []byte, error) {
		raw, err := r.engine.ReadRaw(ctx, "album", change.Album)
		if err != nil {
			return "", "", "", nil, err
		}
		var album model.AlbumRow
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &album) != nil || json.Unmarshal(raw, &fields) != nil || album.ID != change.Album || album.Tenant != r.scope.Tenant || album.Owner != r.scope.Subject {
			clear(raw)
			return "", "", "", nil, ErrSource
		}
		clear(raw)
		if album.DeletedAt != nil {
			return "", "", "", nil, ErrNotFound
		}
		for _, id := range change.Add {
			if _, err := r.asset(ctx, id); err != nil {
				return "", "", "", nil, err
			}
		}
		fields["updated_at"], _ = json.Marshal(time.Now().UTC())
		prepared, err := json.Marshal(fields)
		return "album", change.Album, "live", prepared, err
	}, r.albumChangeContent(change, operation))
	defer clear(result)
	if err != nil {
		return 0, err
	}
	var value struct {
		Result albumChangeResult `json:"private_members_result"`
	}
	if json.Unmarshal(result, &value) != nil || value.Result.Operation != operation || value.Result.Added < 0 {
		return 0, ErrSource
	}
	return value.Result.Added, nil
}

func (r *Repository) albumChangeContent(change albumChange, operation string) records.MutationContent {
	return records.MutationContent{
		Capture: func(_ context.Context, raw []byte, target storage.PublicationTarget) ([]byte, error) {
			var row model.AlbumRow
			if json.Unmarshal(raw, &row) != nil || row.ID != change.Album {
				return nil, ErrSource
			}
			return json.Marshal(albumChangeOrigin{Version: 1, Scope: r.scope, Target: target, Operation: operation})
		},
		Prepare: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) ([]byte, error) {
			if err := r.albumChangeIntent(raw, captured, target, change.Album, operation); err != nil {
				return nil, err
			}
			return r.prepareAlbumMembers(ctx, raw, change, operation, target)
		},
		Validate: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) error {
			if err := r.albumChangeIntent(raw, captured, target, change.Album, operation); err != nil {
				return err
			}
			if set, err := r.albumSet(raw); err != nil || set == nil {
				return ErrSource
			}
			catalog, err := r.engine.Catalog()
			if err != nil {
				return err
			}
			assets, err := r.assets(ctx)
			if err != nil {
				return err
			}
			ids := map[string]bool{}
			for _, asset := range assets {
				ids[asset.ID] = true
			}
			return r.validateAlbumMemberships(ctx, catalog, raw, ids)
		},
		Acknowledge: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion) error {
			if err := r.albumChangeIntent(raw, captured, target, change.Album, operation); err != nil {
				return err
			}
			return r.acknowledgeAlbumMembers(ctx, raw, change.Album, operation, accepted)
		},
	}
}

func (r *Repository) albumChangeIntent(raw, captured []byte, target storage.PublicationTarget, album, operation string) error {
	var row model.AlbumRow
	var origin albumChangeOrigin
	if json.Unmarshal(raw, &row) != nil || json.Unmarshal(captured, &origin) != nil || row.ID != album || row.Tenant != r.scope.Tenant || row.Owner != r.scope.Subject || row.DeletedAt != nil || origin.Version != 1 || origin.Scope != r.scope || origin.Target != target || origin.Operation != operation {
		return ErrSource
	}
	_, err := r.albumSet(raw)
	return err
}

func (r *Repository) loadAlbumPages(ctx context.Context, raw []byte, album string) ([]albumMemberPage, error) {
	set, err := r.albumSet(raw)
	if err != nil {
		return nil, err
	}
	catalog, err := r.engine.Catalog()
	if err != nil {
		return nil, err
	}
	var pages []albumMemberPage
	if set == nil {
		pages, err = r.legacyAlbumPages(ctx, album)
	} else {
		pages, err = r.readAlbumPages(ctx, catalog, album, *set)
	}
	if err != nil {
		return nil, err
	}
	return pages, nil
}

func (r *Repository) patchAlbumMembers(change albumChange, pagesInput *[]albumMemberPage, now time.Time) (int, error) {
	pages := *pagesInput
	defer func() { *pagesInput = pages }()
	seen := map[string]bool{}
	position := 0
	for i := range pages {
		kept := pages[i].Members[:0]
		for _, body := range pages[i].Members {
			member, err := r.decodeAlbumMember(change.Album, body)
			if err != nil {
				return 0, err
			}
			if member.Position >= position {
				position = member.Position + 1
			}
			if member.AssetID == change.Remove {
				clear(body)
				continue
			}
			seen[member.AssetID] = true
			kept = append(kept, body)
		}
		clear(pages[i].Members[len(kept):])
		pages[i].Members = kept
	}
	added := 0
	for _, id := range change.Add {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := model.MembershipID(change.Album, id); err != nil {
			return 0, model.ErrInvalidInput
		}
		member, _ := json.Marshal(model.MembershipRow{Tenant: r.scope.Tenant, AlbumID: change.Album, AssetID: id, Position: position, AddedAt: now})
		if len(pages) == 0 || albumPageFull(pages[len(pages)-1], len(member)) {
			pages = append(pages, albumMemberPage{Version: 1, Scope: r.scope, Album: change.Album, Members: []json.RawMessage{}})
		}
		position++
		added++
		pages[len(pages)-1].Members = append(pages[len(pages)-1].Members, member)
	}
	return added, nil
}

func (r *Repository) prepareAlbumMembers(ctx context.Context, raw []byte, change albumChange, operation string, target storage.PublicationTarget) ([]byte, error) {
	pages, err := r.loadAlbumPages(ctx, raw, change.Album)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, page := range pages {
			clearAlbumPage(page)
		}
	}()
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, ErrSource
	}
	var now time.Time
	if json.Unmarshal(fields["updated_at"], &now) != nil || now.IsZero() {
		return nil, ErrSource
	}
	added, err := r.patchAlbumMembers(change, &pages, now)
	if err != nil {
		return nil, err
	}
	file, err := r.stageAlbumCheckpoint(ctx, pages, change.Album, operation, target)
	if err != nil {
		return nil, err
	}
	fields[albumMembersField], _ = json.Marshal(albumMemberSet{Version: 1, Root: file})
	fields["private_members_result"], _ = json.Marshal(albumChangeResult{Operation: operation, Added: added})
	return json.Marshal(fields)
}

func (r *Repository) stageAlbumCheckpoint(ctx context.Context, pages []albumMemberPage, album, operation string, target storage.PublicationTarget) (records.RetainedFile, error) {
	manifest := albumManifest{Version: 1, Scope: r.scope, Album: album, Pages: []albumPageReference{}}
	for _, page := range pages {
		if len(page.Members) == 0 {
			continue
		}
		encoded, err := json.Marshal(page)
		if err != nil {
			return records.RetainedFile{}, err
		}
		descriptor := records.SnapshotRow{Kind: albumPageKind, ID: planBytesHash(encoded), Size: int64(len(encoded)), SHA256: planBytesHash(encoded)}
		var file records.RetainedFile
		if page.retained != nil && page.retained.Row == descriptor {
			file = *page.retained
		} else {
			file, err = r.engine.StageRetainedFile(ctx, operation, target, descriptor, func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(encoded)), nil })
		}
		clear(encoded)
		if err != nil {
			return records.RetainedFile{}, err
		}
		manifest.Pages = append(manifest.Pages, albumPageReference{File: file, Count: len(page.Members)})
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return records.RetainedFile{}, err
	}
	defer clear(encoded)
	descriptor := records.SnapshotRow{Kind: albumIndexKind, ID: planBytesHash(encoded), Size: int64(len(encoded)), SHA256: planBytesHash(encoded)}
	file, err := r.engine.StageRetainedFile(ctx, operation, target, descriptor, func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(encoded)), nil })
	if err != nil {
		return records.RetainedFile{}, err
	}
	return file, nil
}

func (r *Repository) acknowledgeAlbumMembers(ctx context.Context, raw []byte, album, operation string, accepted storage.ObjectVersion) error {
	set, err := r.albumSet(raw)
	if err != nil || set == nil {
		return ErrSource
	}
	catalog, err := r.engine.Catalog()
	if err != nil {
		return err
	}
	manifest, err := r.readAlbumManifest(ctx, catalog, album, *set)
	if err != nil {
		return err
	}
	files := []records.RetainedFile{set.Root}
	for _, page := range manifest.Pages {
		files = append(files, page.File)
	}
	for first := 0; first < len(files); first += 256 {
		batch := files[first:min(first+256, len(files))]
		if err := r.engine.AcknowledgeRetainedFiles(ctx, operation, batch, accepted, func(selected []byte) ([]records.RetainedFile, error) {
			actual, err := r.albumSet(selected)
			if err != nil || actual == nil || *actual != *set {
				return nil, ErrSource
			}
			return slices.Clone(batch), nil
		}); err != nil {
			return err
		}
	}
	return nil
}

var _ api.PhotosAlbumMembershipData = (*Repository)(nil)
