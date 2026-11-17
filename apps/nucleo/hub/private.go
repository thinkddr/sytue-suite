// SPDX-License-Identifier: Apache-2.0
// Package hub provides the private client-side notification repository. It does
// not switch the existing Hub inbox or make server-generated notices private.
package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const noticeKind = "notice"

var ErrNotice = errors.New("hub: invalid private notification")

// Notice is private data: all fields, including scope and read state, are
// persisted only inside the encrypted record. ID is allocated by the caller
// and retained across retries; it is not an authorization credential.
type Notice struct {
	Version          int               `json:"v"`
	Scope            storage.Scope     `json:"scope"`
	ID               string            `json:"id"`
	Source           string            `json:"source"`
	Type             string            `json:"type"`
	Level            string            `json:"level"`
	Title            string            `json:"title"`
	Body             string            `json:"body,omitempty"`
	URL              string            `json:"url,omitempty"`
	Params           map[string]string `json:"params,omitempty"`
	Read             bool              `json:"read"`
	ReadAt           *time.Time        `json:"read_at,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	Organization     string            `json:"organization,omitempty"`
	OrganizationName string            `json:"organization_name,omitempty"`
	Historic         bool              `json:"historic,omitempty"`
}

func (n Notice) validate(scope storage.Scope, id string) error {
	if !scope.Valid() || scope.Product != "hub" || n.Scope != scope || n.Version != 1 || n.ID != id || !storage.Identifier(id) || !validNoticeTextFields(n) || n.CreatedAt.IsZero() || !validNoticeParams(n.Params) {
		return ErrNotice
	}
	if n.Level != "info" && n.Level != "success" && n.Level != "warning" && n.Level != "error" {
		return ErrNotice
	}
	if len(n.Organization) > 255 || len(n.OrganizationName) > 1024 || (!n.Read && n.ReadAt != nil) {
		return ErrNotice
	}
	if !validNoticeURL(n.URL) {
		return ErrNotice
	}
	return nil
}

func validNoticeTextFields(n Notice) bool {
	return strings.TrimSpace(n.Source) != "" && len(n.Source) <= 64 && strings.TrimSpace(n.Type) != "" && len(n.Type) <= 100 && strings.TrimSpace(n.Title) != "" && len(n.Title) <= 800 && len(n.Body) <= 4000
}

func validNoticeParams(params map[string]string) bool {
	if len(params) > 8 {
		return false
	}
	for k, v := range params {
		if strings.TrimSpace(k) == "" || len(k) > 32 || len(v) > 1200 {
			return false
		}
	}
	return true
}

func validNoticeURL(rawURL string) bool {
	if rawURL == "" {
		return true
	}
	u, err := url.Parse(rawURL)
	return err == nil && len(rawURL) <= 2048 && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}

type Config = records.Config

type Repository struct {
	core  *records.Repository
	scope storage.Scope
}

// New fixes the product, domain and validators regardless of caller input.
// Source must implement the shared verified migration contract; no legacy Hub
// source adapter is provided yet.
func New(cfg Config) (*Repository, error) {
	scope := cfg.Scope
	if !scope.Valid() || scope.Product != "hub" {
		return nil, ErrNotice
	}
	cfg.Product = "hub"
	cfg.EntityKind = "org.sytue.hub.entity.v1"
	cfg.ReceiptKind = "org.sytue.hub.snapshot.v1"
	cfg.BinaryKind, cfg.ValidateBinary = nil, nil
	cfg.ValidateRow = func(_ context.Context, kind, id string, raw []byte) error {
		_, err := decode(scope, kind, id, raw)
		return err
	}
	cfg.ValidateCatalog = func(ctx context.Context, catalog records.Catalog) error {
		_, err := readCatalog(ctx, scope, catalog)
		return err
	}
	core, err := records.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Repository{core: core, scope: scope}, nil
}

func decode(scope storage.Scope, kind, id string, raw []byte) (Notice, error) {
	var n Notice
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	canonical, canonicalErr := matrixproof.CanonicalBytes(json.RawMessage(raw))
	clear(canonical)
	if kind != noticeKind || decoder.Decode(&n) != nil || !json.Valid(raw) || canonicalErr != nil || n.validate(scope, id) != nil {
		return Notice{}, ErrNotice
	}
	return n, nil
}

func (r *Repository) PrivateStorageScope() storage.Scope   { return r.scope }
func (r *Repository) Initialize(ctx context.Context) error { return r.core.Initialize(ctx) }
func (r *Repository) ImportLegacySnapshot(ctx context.Context) error {
	return r.core.ImportLegacySnapshot(ctx)
}

func (r *Repository) RefreshPrivateData(ctx context.Context) error {
	return r.core.RefreshPrivateData(ctx)
}
func (r *Repository) Close()                          { r.core.Close() }
func (r *Repository) Drain(ctx context.Context) error { return r.core.Drain(ctx) }
func (r *Repository) Resume() error                   { return r.core.Resume() }
func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	return r.core.Pending(ctx)
}

// List rejects malformed, conflicting or incomplete catalogs instead of hiding
// entries. Sorting is deterministic, including simultaneous timestamps.
func (r *Repository) List(parent context.Context) ([]Notice, error) {
	ctx, done, err := r.core.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	catalog, err := r.core.Catalog()
	if err != nil {
		return nil, err
	}
	return readCatalog(ctx, r.scope, catalog)
}

func readCatalog(ctx context.Context, scope storage.Scope, catalog records.Catalog) ([]Notice, error) {
	entries, err := catalog.Entries()
	if err != nil {
		return nil, err
	}
	notices := make([]Notice, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.Binary || entry.State != "live" || seen[entry.ID] {
			return nil, ErrNotice
		}
		seen[entry.ID] = true
		raw, err := catalog.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		n, err := decode(scope, entry.Kind, entry.ID, raw)
		clear(raw)
		if err != nil {
			return nil, err
		}
		notices = append(notices, n)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(notices, func(a, b Notice) int {
		if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(b.ID, a.ID)
	})
	return notices, nil
}

// Put creates a notice only if its ID has not already been used for different
// content. Retrying the same accepted notice never resets its read state.
func (r *Repository) Put(ctx context.Context, n Notice) error {
	n.Params = maps.Clone(n.Params)
	if n.validate(r.scope, n.ID) != nil || n.Read {
		return ErrNotice
	}
	raw, err := r.core.Mutate(ctx, "notice-create", n, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, noticeKind, n.ID)
		if err == nil {
			defer clear(existing)
			old, e := decode(r.scope, noticeKind, n.ID, existing)
			if e != nil {
				return "", "", "", nil, e
			}
			old.Read = false
			old.ReadAt = nil
			a, _ := json.Marshal(old)
			b, _ := json.Marshal(n)
			defer clear(a)
			defer clear(b)
			if !bytes.Equal(a, b) {
				return "", "", "", nil, records.ErrConflict
			}
			// Do not publish an older unread revision over the existing one.
			return "", "", "", nil, errAlreadyPresent
		}
		if !errors.Is(err, records.ErrNotFound) {
			return "", "", "", nil, err
		}
		value, err := json.Marshal(n)
		return noticeKind, n.ID, "live", value, err
	})
	clear(raw)
	if err != nil && !errors.Is(err, errAlreadyPresent) {
		return err
	}
	current, err := r.current(ctx, n.ID)
	if err != nil {
		return err
	}
	current.Read, current.ReadAt = false, nil
	a, _ := json.Marshal(current)
	b, _ := json.Marshal(n)
	defer clear(a)
	defer clear(b)
	if !bytes.Equal(a, b) {
		return records.ErrConflict
	}
	return ctx.Err()
}

var errAlreadyPresent = errors.New("hub: notification already present")

// MarkRead changes only an existing verified notice. Conflicting heads are
// returned to the caller for resolution rather than silently discarded.
func (r *Repository) MarkRead(ctx context.Context, id string) error {
	if !storage.Identifier(id) {
		return ErrNotice
	}
	raw, err := r.core.Mutate(ctx, "notice-read", id, func(ctx context.Context) (string, string, string, []byte, error) {
		data, err := r.core.ReadRaw(ctx, noticeKind, id)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(data)
		n, err := decode(r.scope, noticeKind, id, data)
		if err != nil {
			return "", "", "", nil, err
		}
		if n.Read {
			return "", "", "", nil, errAlreadyPresent
		}
		n.Read = true
		now := time.Now().UTC()
		n.ReadAt = &now
		value, err := json.Marshal(n)
		return noticeKind, id, "live", value, err
	})
	clear(raw)
	if err != nil && !errors.Is(err, errAlreadyPresent) {
		return err
	}
	current, err := r.current(ctx, id)
	if err != nil {
		return err
	}
	if !current.Read {
		return records.ErrConflict
	}
	return ctx.Err()
}

func (r *Repository) current(ctx context.Context, id string) (Notice, error) {
	raw, err := r.core.ReadRaw(ctx, noticeKind, id)
	if err != nil {
		return Notice{}, err
	}
	defer clear(raw)
	return decode(r.scope, noticeKind, id, raw)
}
