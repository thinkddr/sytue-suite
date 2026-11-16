// SPDX-License-Identifier: Apache-2.0
package erp

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
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

var (
	ErrERP         = errors.New("erp: invalid private record")
	ErrUnsupported = errors.New("erp: operation requires shared server capability")
)

type Config = records.Config

type Repository struct {
	core  *records.Repository
	scope storage.Scope
}

func SourceKind(kind string) bool {
	switch kind {
	case "record", "activity":
		return true
	default:
		return false
	}
}

func recordsConfiguration(cfg Config) (Config, error) {
	scope := cfg.Scope
	if !scope.Valid() || scope.Product != "erp" {
		return Config{}, ErrERP
	}
	cfg.Product = "erp"
	cfg.EntityKind = "org.sytue.erp.entity.v1"
	cfg.ReceiptKind = "org.sytue.erp.snapshot.v1"
	cfg.BinaryKind, cfg.ValidateBinary = nil, nil
	cfg.ValidateRow = func(_ context.Context, kind, id string, raw []byte) error {
		if !SourceKind(kind) || !storage.Identifier(id) || !json.Valid(raw) {
			return ErrERP
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
				return ErrERP
			}
			raw, err := catalog.ReadJSON(ctx, entry)
			if err != nil {
				return err
			}
			valid := json.Valid(raw)
			clear(raw)
			if !valid {
				return ErrERP
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
		return nil, ErrERP
	}
	cfg.Isolated = r.core
	return records.NewIdentityCutover(cfg)
}

func (r *Repository) Catalogo(ctx context.Context) (api.ERPCatalogo, error) {
	if err := ctx.Err(); err != nil {
		return api.ERPCatalogo{}, err
	}
	return api.ERPCatalogo{Modulos: []api.ERPModulo{}, Activos: []string{}}, nil
}

func (r *Repository) MisPermisos(ctx context.Context) (api.ERPPermisos, error) {
	if err := ctx.Err(); err != nil {
		return api.ERPPermisos{}, err
	}
	return api.ERPPermisos{
		Administrador: true,
		Modulos:       map[string]string{},
		Verbos:        map[string][]string{},
	}, nil
}

func (r *Repository) Listar(ctx context.Context, modelo, q, cursor string, limite int) (api.ERPPagina, error) {
	return r.ListarCon(ctx, modelo, api.ERPFiltro{Q: q, Cursor: cursor, Limite: limite})
}

func (r *Repository) ListarCon(parent context.Context, modelo string, f api.ERPFiltro) (api.ERPPagina, error) {
	ctx, done, err := r.core.Start(parent)
	if err != nil {
		return api.ERPPagina{}, err
	}
	defer done()
	catalog, err := r.core.Catalog()
	if err != nil {
		return api.ERPPagina{}, err
	}
	entries, err := catalog.Entries()
	if err != nil {
		return api.ERPPagina{}, err
	}
	var regs []api.ERPRegistro
	for _, entry := range entries {
		if entry.Kind != "record" || entry.State != "live" {
			continue
		}
		raw, err := catalog.ReadJSON(ctx, entry)
		if err != nil {
			return api.ERPPagina{}, err
		}
		var reg api.ERPRegistro
		err = json.Unmarshal(raw, &reg)
		clear(raw)
		if err != nil {
			return api.ERPPagina{}, ErrERP
		}
		if reg.Modelo != modelo {
			continue
		}
		if f.Estado != "" && reg.Estado != f.Estado {
			continue
		}
		if f.Q != "" && !buscar.Contiene(reg.Etiqueta, f.Q) {
			continue
		}
		regs = append(regs, reg)
	}
	slices.SortFunc(regs, func(a, b api.ERPRegistro) int {
		if order := b.CreadoEn.Compare(a.CreadoEn); order != 0 {
			return order
		}
		return strings.Compare(b.ID, a.ID)
	})
	if f.Limite > 0 && len(regs) > f.Limite {
		regs = regs[:f.Limite]
	}
	return api.ERPPagina{Registros: regs}, ctx.Err()
}

func (r *Repository) Estadisticas(ctx context.Context, modelo string) (api.ERPEstadisticas, error) {
	pag, err := r.ListarCon(ctx, modelo, api.ERPFiltro{})
	if err != nil {
		return api.ERPEstadisticas{}, err
	}
	porEstado := map[string]int{}
	for _, reg := range pag.Registros {
		porEstado[reg.Estado]++
	}
	return api.ERPEstadisticas{
		Total:     len(pag.Registros),
		PorEstado: porEstado,
	}, nil
}

func (r *Repository) Obtener(ctx context.Context, modelo, id string) (api.ERPRegistro, error) {
	raw, err := r.core.ReadRaw(ctx, "record", id)
	if err != nil {
		return api.ERPRegistro{}, err
	}
	defer clear(raw)
	var reg api.ERPRegistro
	if err := json.Unmarshal(raw, &reg); err != nil || (modelo != "" && reg.Modelo != modelo) {
		return api.ERPRegistro{}, ErrERP
	}
	return reg, nil
}

func (r *Repository) Crear(ctx context.Context, modelo string, data map[string]any) (api.ERPRegistro, error) {
	if data == nil {
		data = map[string]any{}
	}
	now := time.Now().UTC()
	etiqueta, _ := data["nombre"].(string)
	if etiqueta == "" {
		etiqueta = modelo
	}
	reg := api.ERPRegistro{
		ID:        uuid.NewString(),
		Modelo:    modelo,
		Etiqueta:  etiqueta,
		Data:      data,
		CreadoEn:  now,
		EditadoEn: now,
	}
	raw, err := r.core.Mutate(ctx, "record-create", reg, func(_ context.Context) (string, string, string, []byte, error) {
		val, err := json.Marshal(reg)
		return "record", reg.ID, "live", val, err
	})
	clear(raw)
	return reg, err
}

func (r *Repository) Actualizar(ctx context.Context, modelo, id string, data map[string]any) (api.ERPRegistro, error) {
	var updated api.ERPRegistro
	raw, err := r.core.Mutate(ctx, "record-update", id, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "record", id)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		if err := json.Unmarshal(existing, &updated); err != nil || (modelo != "" && updated.Modelo != modelo) {
			return "", "", "", nil, ErrERP
		}
		updated.Data = data
		if nombre, ok := data["nombre"].(string); ok && nombre != "" {
			updated.Etiqueta = nombre
		}
		updated.EditadoEn = time.Now().UTC()
		val, err := json.Marshal(updated)
		return "record", id, "live", val, err
	})
	clear(raw)
	return updated, err
}

func (r *Repository) CambiarEstado(ctx context.Context, modelo, id, estado string) (api.ERPRegistro, error) {
	var updated api.ERPRegistro
	raw, err := r.core.Mutate(ctx, "record-state", id, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "record", id)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		if err := json.Unmarshal(existing, &updated); err != nil || (modelo != "" && updated.Modelo != modelo) {
			return "", "", "", nil, ErrERP
		}
		updated.Estado = estado
		updated.EditadoEn = time.Now().UTC()
		val, err := json.Marshal(updated)
		return "record", id, "live", val, err
	})
	clear(raw)
	return updated, err
}

func (r *Repository) Borrar(ctx context.Context, _, id string) error {
	raw, err := r.core.Mutate(ctx, "record-delete", id, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "record", id)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var rawMap map[string]any
		if err := json.Unmarshal(existing, &rawMap); err != nil {
			return "", "", "", nil, ErrERP
		}
		rawMap["deleted_at"] = time.Now().UTC()
		encoded, err := json.Marshal(rawMap)
		return "record", id, "trash", encoded, err
	})
	clear(raw)
	return err
}

func (r *Repository) Restaurar(ctx context.Context, _, id string) error {
	raw, err := r.core.Mutate(ctx, "record-restore", id, func(ctx context.Context) (string, string, string, []byte, error) {
		existing, err := r.core.ReadRaw(ctx, "record", id)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(existing)
		var rawMap map[string]any
		if err := json.Unmarshal(existing, &rawMap); err != nil {
			return "", "", "", nil, ErrERP
		}
		delete(rawMap, "deleted_at")
		encoded, err := json.Marshal(rawMap)
		return "record", id, "live", encoded, err
	})
	clear(raw)
	return err
}

func (r *Repository) Duplicar(ctx context.Context, modelo, id string) (api.ERPRegistro, error) {
	orig, err := r.Obtener(ctx, modelo, id)
	if err != nil {
		return api.ERPRegistro{}, err
	}
	return r.Crear(ctx, orig.Modelo, orig.Data)
}

func (r *Repository) Asentar(context.Context, string, string) (string, error) {
	return "", ErrUnsupported
}

func (r *Repository) Exportar(context.Context, string, string, string, []string) (io.ReadCloser, string, error) {
	return nil, "", ErrUnsupported
}

func (r *Repository) Imprimir(context.Context, string, string) (io.ReadCloser, error) {
	return nil, ErrUnsupported
}

func (r *Repository) Facturae(context.Context, string, string) (io.ReadCloser, string, error) {
	return nil, "", ErrUnsupported
}

func (r *Repository) UBL(context.Context, string, string) (io.ReadCloser, string, error) {
	return nil, "", ErrUnsupported
}

func (r *Repository) RegistrosVerifactu(context.Context, string, string) (api.ERPFacturaVerifactu, error) {
	return api.ERPFacturaVerifactu{}, ErrUnsupported
}

func (r *Repository) SubsanarVerifactu(context.Context, string, string) (api.ERPFacturaVerifactu, error) {
	return api.ERPFacturaVerifactu{}, ErrUnsupported
}
