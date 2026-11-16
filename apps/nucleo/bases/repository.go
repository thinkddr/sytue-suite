// SPDX-License-Identifier: Apache-2.0
package bases

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

var (
	ErrBases       = errors.New("bases: invalid private record")
	ErrUnsupported = errors.New("bases: operation requires shared server capability")
)

type Config = records.Config

type Repository struct {
	core  *records.Repository
	scope storage.Scope
}

func SourceKind(kind string) bool {
	switch kind {
	case "base", "table", "field", "view", "record":
		return true
	default:
		return false
	}
}

func recordsConfiguration(cfg Config) (Config, error) {
	scope := cfg.Scope
	if !scope.Valid() || scope.Product != "bases" {
		return Config{}, ErrBases
	}
	cfg.Product = "bases"
	cfg.EntityKind = "org.sytue.bases.entity.v1"
	cfg.ReceiptKind = "org.sytue.bases.snapshot.v1"
	cfg.BinaryKind, cfg.ValidateBinary = nil, nil
	cfg.ValidateRow = func(_ context.Context, kind, id string, raw []byte) error {
		if !SourceKind(kind) || !storage.Identifier(id) || !json.Valid(raw) {
			return ErrBases
		}
		return nil
	}
	cfg.ValidateCatalog = func(ctx context.Context, catalog records.Catalog) error {
		entries, err := catalog.Entries()
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Binary || !SourceKind(entry.Kind) {
				return ErrBases
			}
			raw, err := catalog.ReadJSON(ctx, entry)
			if err != nil {
				return err
			}
			valid := json.Valid(raw)
			clear(raw)
			if !valid {
				return ErrBases
			}
		}
		return nil
	}
	return cfg, nil
}

func New(cfg Config) (*Repository, error) {
	configuration, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	core, err := records.New(configuration)
	if err != nil {
		return nil, err
	}
	return &Repository{core: core, scope: configuration.Scope}, nil
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

func (r *Repository) IdentityCutover(cfg records.IdentityCutoverConfig) (*records.IdentityCutover, error) {
	if r == nil || r.core == nil || cfg.Scope != r.scope || cfg.Isolated != nil {
		return nil, ErrBases
	}
	cfg.Isolated = r.core
	return records.NewIdentityCutover(cfg)
}

func (r *Repository) readByKind(parent context.Context, kind string) ([][]byte, error) {
	ctx, done, err := r.core.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	catalog, err := r.core.Catalog()
	if err != nil {
		return nil, err
	}
	entries, err := catalog.Entries()
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, entry := range entries {
		if entry.State != "live" || entry.Kind != kind {
			continue
		}
		raw, err := catalog.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, ctx.Err()
}

func (r *Repository) ListBases(ctx context.Context) ([]api.BasesBase, error) {
	raws, err := r.readByKind(ctx, "base")
	if err != nil {
		return nil, err
	}
	out := make([]api.BasesBase, 0, len(raws))
	for _, raw := range raws {
		var b api.BasesBase
		err := json.Unmarshal(raw, &b)
		clear(raw)
		if err != nil {
			return nil, ErrBases
		}
		if b.Role == "" {
			b.Role = api.BasesOwner
		}
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b api.BasesBase) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func (r *Repository) CreateBase(ctx context.Context, nombre string) (api.BasesBase, error) {
	now := time.Now().UTC()
	b := api.BasesBase{
		ID:        uuid.NewString(),
		Name:      strings.TrimSpace(nombre),
		OwnerSub:  r.scope.Subject,
		Role:      api.BasesOwner,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if b.Name == "" {
		return api.BasesBase{}, ErrBases
	}
	raw, err := r.core.Mutate(ctx, "base-create", b, func(_ context.Context) (string, string, string, []byte, error) {
		val, err := json.Marshal(b)
		return "base", b.ID, "live", val, err
	})
	clear(raw)
	return b, err
}

func (r *Repository) GetBase(ctx context.Context, id string) (api.BasesBase, error) {
	raw, err := r.core.ReadRaw(ctx, "base", id)
	if err != nil {
		return api.BasesBase{}, err
	}
	defer clear(raw)
	var b api.BasesBase
	if err := json.Unmarshal(raw, &b); err != nil {
		return api.BasesBase{}, ErrBases
	}
	if b.Role == "" {
		b.Role = api.BasesOwner
	}
	return b, nil
}

func (r *Repository) UpdateBase(ctx context.Context, id, nombre, descripcion string) error {
	raw, err := r.core.Mutate(ctx, "base-update", id, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "base", id)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var b api.BasesBase
		if err := json.Unmarshal(existing, &b); err != nil {
			return "", "", "", nil, ErrBases
		}
		b.Name = strings.TrimSpace(nombre)
		b.Description = strings.TrimSpace(descripcion)
		b.UpdatedAt = time.Now().UTC()
		val, err := json.Marshal(b)
		return "base", id, "live", val, err
	})
	clear(raw)
	return err
}

func (r *Repository) trashRow(ctx context.Context, op, kind, id string) error {
	raw, err := r.core.Mutate(ctx, op, id, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, kind, id)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var rawMap map[string]any
		if err := json.Unmarshal(existing, &rawMap); err != nil {
			return "", "", "", nil, ErrBases
		}
		rawMap["deleted_at"] = time.Now().UTC()
		encoded, err := json.Marshal(rawMap)
		return kind, id, "trash", encoded, err
	})
	clear(raw)
	return err
}

func (r *Repository) DeleteBase(ctx context.Context, id string) error {
	return r.trashRow(ctx, "base-delete", "base", id)
}

func (r *Repository) ListTables(ctx context.Context, base string) ([]api.BasesTable, error) {
	raws, err := r.readByKind(ctx, "table")
	if err != nil {
		return nil, err
	}
	fieldRaws, err := r.readByKind(ctx, "field")
	if err != nil {
		return nil, err
	}
	fieldsByTable := make(map[string][]api.BasesField)
	for _, fRaw := range fieldRaws {
		var f api.BasesField
		err := json.Unmarshal(fRaw, &f)
		clear(fRaw)
		if err != nil {
			return nil, ErrBases
		}
		fieldsByTable[f.TableID] = append(fieldsByTable[f.TableID], f)
	}
	out := make([]api.BasesTable, 0, len(raws))
	for _, raw := range raws {
		var t api.BasesTable
		err := json.Unmarshal(raw, &t)
		clear(raw)
		if err != nil {
			return nil, ErrBases
		}
		if t.BaseID != base {
			continue
		}
		if t.Role == "" {
			t.Role = api.BasesOwner
		}
		t.Fields = fieldsByTable[t.ID]
		slices.SortFunc(t.Fields, func(a, b api.BasesField) int { return a.Position - b.Position })
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b api.BasesTable) int { return a.Position - b.Position })
	return out, nil
}

func (r *Repository) CreateTable(ctx context.Context, base, nombre string) (api.BasesTable, error) {
	t := api.BasesTable{
		ID:     uuid.NewString(),
		BaseID: base,
		Name:   strings.TrimSpace(nombre),
		Role:   api.BasesOwner,
		Fields: []api.BasesField{},
	}
	if t.Name == "" {
		return api.BasesTable{}, ErrBases
	}
	raw, err := r.core.Mutate(ctx, "table-create", t, func(_ context.Context) (string, string, string, []byte, error) {
		val, err := json.Marshal(t)
		return "table", t.ID, "live", val, err
	})
	clear(raw)
	return t, err
}

func (r *Repository) RenameTable(ctx context.Context, tabla, nombre string) error {
	raw, err := r.core.Mutate(ctx, "table-rename", tabla, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "table", tabla)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var t api.BasesTable
		if err := json.Unmarshal(existing, &t); err != nil {
			return "", "", "", nil, ErrBases
		}
		t.Name = strings.TrimSpace(nombre)
		val, err := json.Marshal(t)
		return "table", tabla, "live", val, err
	})
	clear(raw)
	return err
}

func (r *Repository) DeleteTable(ctx context.Context, tabla string) error {
	return r.trashRow(ctx, "table-delete", "table", tabla)
}

func (r *Repository) CreateField(ctx context.Context, tabla string, campo api.BasesNewField) (api.BasesField, error) {
	f := api.BasesField{
		ID:          uuid.NewString(),
		TableID:     tabla,
		Name:        strings.TrimSpace(campo.Name),
		Kind:        campo.Kind,
		Choices:     campo.Choices,
		LinkTableID: campo.LinkTableID,
		Formula:     campo.Formula,
		Result:      campo.Result,
	}
	raw, err := r.core.Mutate(ctx, "field-create", f, func(_ context.Context) (string, string, string, []byte, error) {
		val, err := json.Marshal(f)
		return "field", f.ID, "live", val, err
	})
	clear(raw)
	return f, err
}

func (r *Repository) RenameField(ctx context.Context, campo, nombre string) error {
	raw, err := r.core.Mutate(ctx, "field-rename", campo, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "field", campo)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var f api.BasesField
		if err := json.Unmarshal(existing, &f); err != nil {
			return "", "", "", nil, ErrBases
		}
		f.Name = strings.TrimSpace(nombre)
		val, err := json.Marshal(f)
		return "field", campo, "live", val, err
	})
	clear(raw)
	return err
}

func (r *Repository) DeleteField(ctx context.Context, campo string) error {
	return r.trashRow(ctx, "field-delete", "field", campo)
}

func (r *Repository) ListRecords(ctx context.Context, tabla, _, _ string, limite int) (api.BasesRecordPage, error) {
	fieldRaws, err := r.readByKind(ctx, "field")
	if err != nil {
		return api.BasesRecordPage{}, err
	}
	var fields []api.BasesField
	for _, fRaw := range fieldRaws {
		var f api.BasesField
		err := json.Unmarshal(fRaw, &f)
		clear(fRaw)
		if err != nil {
			return api.BasesRecordPage{}, ErrBases
		}
		if f.TableID == tabla {
			fields = append(fields, f)
		}
	}
	slices.SortFunc(fields, func(a, b api.BasesField) int { return a.Position - b.Position })

	recRaws, err := r.readByKind(ctx, "record")
	if err != nil {
		return api.BasesRecordPage{}, err
	}
	var recs []api.BasesRecord
	for _, rRaw := range recRaws {
		var rec api.BasesRecord
		err := json.Unmarshal(rRaw, &rec)
		clear(rRaw)
		if err != nil {
			return api.BasesRecordPage{}, ErrBases
		}
		if rec.TableID == tabla {
			recs = append(recs, rec)
		}
	}
	slices.SortFunc(recs, func(a, b api.BasesRecord) int { return strings.Compare(a.ID, b.ID) })
	if limite > 0 && len(recs) > limite {
		recs = recs[:limite]
	}
	return api.BasesRecordPage{
		Fields:  fields,
		Records: recs,
		Role:    api.BasesOwner,
	}, nil
}

func (r *Repository) CreateRecord(ctx context.Context, tabla string, datos map[string]any, enlaces map[string][]string) (api.BasesRecord, error) {
	if datos == nil {
		datos = map[string]any{}
	}
	if enlaces == nil {
		enlaces = map[string][]string{}
	}
	now := time.Now().UTC()
	rec := api.BasesRecord{
		ID:        uuid.NewString(),
		TableID:   tabla,
		Data:      datos,
		Links:     enlaces,
		Rev:       1,
		CreatedBy: r.scope.Subject,
		UpdatedBy: r.scope.Subject,
		CreatedAt: now,
		UpdatedAt: now,
	}
	raw, err := r.core.Mutate(ctx, "record-create", rec, func(_ context.Context) (string, string, string, []byte, error) {
		val, err := json.Marshal(rec)
		return "record", rec.ID, "live", val, err
	})
	clear(raw)
	return rec, err
}

func (r *Repository) GetRecord(ctx context.Context, registro string) (api.BasesRecord, error) {
	raw, err := r.core.ReadRaw(ctx, "record", registro)
	if err != nil {
		return api.BasesRecord{}, err
	}
	defer clear(raw)
	var rec api.BasesRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return api.BasesRecord{}, ErrBases
	}
	return rec, nil
}

func (r *Repository) WriteCell(ctx context.Context, registro, campo string, celda api.BasesCell, _ string) (api.BasesCellResult, error) {
	var updated api.BasesRecord
	raw, err := r.core.Mutate(ctx, "record-cell", registro, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "record", registro)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		if err := json.Unmarshal(existing, &updated); err != nil {
			return "", "", "", nil, ErrBases
		}
		if updated.Data == nil {
			updated.Data = map[string]any{}
		}
		if updated.Links == nil {
			updated.Links = map[string][]string{}
		}
		if celda.Links != nil {
			updated.Links[campo] = celda.Links
		} else {
			updated.Data[campo] = celda.Value
		}
		updated.Rev++
		updated.UpdatedBy = r.scope.Subject
		updated.UpdatedAt = time.Now().UTC()
		val, err := json.Marshal(updated)
		return "record", registro, "live", val, err
	})
	clear(raw)
	return api.BasesCellResult{Record: updated}, err
}

func (r *Repository) DeleteRecord(ctx context.Context, registro string, _ bool) error {
	return r.trashRow(ctx, "record-delete", "record", registro)
}

func (r *Repository) ListViews(ctx context.Context, tabla string) ([]api.BasesView, error) {
	raws, err := r.readByKind(ctx, "view")
	if err != nil {
		return nil, err
	}
	var out []api.BasesView
	for _, raw := range raws {
		var v api.BasesView
		err := json.Unmarshal(raw, &v)
		clear(raw)
		if err != nil {
			return nil, ErrBases
		}
		if v.TableID == tabla {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b api.BasesView) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func (r *Repository) CreateView(ctx context.Context, tabla, nombre, tipo string, cfg api.BasesViewConfig) (api.BasesView, error) {
	v := api.BasesView{
		ID:        uuid.NewString(),
		TableID:   tabla,
		Name:      strings.TrimSpace(nombre),
		Kind:      tipo,
		Config:    cfg,
		CreatedBy: r.scope.Subject,
	}
	raw, err := r.core.Mutate(ctx, "view-create", v, func(_ context.Context) (string, string, string, []byte, error) {
		val, err := json.Marshal(v)
		return "view", v.ID, "live", val, err
	})
	clear(raw)
	return v, err
}

func (r *Repository) DeleteView(ctx context.Context, vista string) error {
	return r.trashRow(ctx, "view-delete", "view", vista)
}

func (r *Repository) ListFormLinks(context.Context, string) ([]api.BasesFormLink, error) {
	return nil, ErrUnsupported
}

func (r *Repository) CreateFormLink(context.Context, string, int) (api.BasesFormLink, error) {
	return api.BasesFormLink{}, ErrUnsupported
}

func (r *Repository) RevokeFormLink(context.Context, string) error {
	return ErrUnsupported
}

func (r *Repository) ListPermissions(context.Context, string) ([]api.BasesPermission, error) {
	return nil, ErrUnsupported
}

func (r *Repository) GrantPermission(context.Context, string, api.BasesGrant) (api.BasesPermission, error) {
	return api.BasesPermission{}, ErrUnsupported
}

func (r *Repository) RevokePermission(context.Context, string, string) error {
	return ErrUnsupported
}

func (r *Repository) MyGroups(context.Context) ([]string, error) {
	return []string{}, nil
}

func (r *Repository) TransferBase(context.Context, string, string) error {
	return ErrUnsupported
}

func (r *Repository) Export(context.Context, string, string) (io.ReadCloser, error) {
	return nil, ErrUnsupported
}
