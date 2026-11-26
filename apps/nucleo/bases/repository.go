// SPDX-License-Identifier: Apache-2.0
package bases

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases/dominio"
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
	// Actividad es el registro de actividad local (B-06, actividad.go). nil, lo de
	// siempre: no se anota nada y ListAudit falla cerrado.
	Actividad *Actividad
	// SoltarAdjuntos: borrar un adjunto suelta además su fichero (la ficha queda en la
	// papelera sin él), y la próxima compactación del almacén lo quita del disco. Es
	// para Sytue Office, donde un adjunto borrado no se recupera. Falso, lo de siempre.
	SoltarAdjuntos bool
}

func SourceKind(kind string) bool {
	switch kind {
	// comment y file son los comentarios y las fichas de adjunto de un registro
	// (B-04); la fuente del servicio no los trae, el dispositivo los escribe.
	case "base", "table", "field", "view", "record", "comment", "file":
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
	// Los bytes de un adjunto son un fichero retenido del motor, citado desde su ficha.
	cfg.BinaryKind = func(kind string) bool { return kind == binarioAdjunto }
	cfg.ValidateBinary = func(_ context.Context, row records.SnapshotRow) error {
		if row.Kind != binarioAdjunto || row.Size > dominio.MaxBytesAdjunto {
			return ErrBases
		}
		return row.Validate()
	}
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
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return api.BasesBase{}, ErrBases
	}
	entrada := struct {
		Nombre string `json:"name"`
	}{nombre}
	return crear(ctx, r, "base-create", "base", entrada, func(context.Context) (string, api.BasesBase, error) {
		now := time.Now().UTC()
		b := api.BasesBase{
			ID:        uuid.NewString(),
			Name:      nombre,
			OwnerSub:  r.scope.Subject,
			Role:      api.BasesOwner,
			CreatedAt: now,
			UpdatedAt: now,
		}
		return b.ID, b, nil
	})
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

// UpdateBase, RenameTable y RenameField llevan en la entrada lo que escriben: con solo
// el id, un cambio cortado dejaba pendiente esa entrada y el siguiente cambio del mismo
// objeto retomaba el cortado y se perdía (ver «Repetir una operación», esquema.go).
func (r *Repository) UpdateBase(ctx context.Context, id, nombre, descripcion string) error {
	entrada := []string{id, nombre, descripcion}
	raw, err := r.mutar(ctx, "base-update", entrada, func(ctx context.Context) (string, string, string, []byte, error) {
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

func (r *Repository) trashRow(ctx context.Context, op, kind, id string, quitar ...string) error {
	raw, err := r.mutar(ctx, op, id, func(ctx context.Context) (string, string, string, []byte, error) {
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
		for _, k := range quitar {
			delete(rawMap, k)
		}
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

func (r *Repository) RenameTable(ctx context.Context, tabla, nombre string) error {
	raw, err := r.mutar(ctx, "table-rename", []string{tabla, nombre}, func(ctx context.Context) (string, string, string, []byte, error) {
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

func (r *Repository) RenameField(ctx context.Context, campo, nombre string) error {
	raw, err := r.mutar(ctx, "field-rename", []string{campo, nombre}, func(ctx context.Context) (string, string, string, []byte, error) {
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
