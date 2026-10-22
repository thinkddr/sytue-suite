// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// PaginaRegistros es una página de registros con el esquema que los interpreta.
// NextCursor continúa la lectura; vacío = no hay más.
type PaginaRegistros struct {
	Fields     []domain.Campo    `json:"fields"`
	Records    []domain.Registro `json:"records"`
	Role       domain.Rol        `json:"role"`
	NextCursor string            `json:"next_cursor,omitempty"`
	// Recalculating: hay fórmulas de esta tabla pendientes de recalcular (formulas.go);
	// sus valores pueden ir un momento por detrás.
	Recalculating bool `json:"recalculating,omitempty"`
}

// ListarRegistros devuelve una página de registros de la tabla, aplicando
// opcionalmente una vista guardada (filtros y orden). Límite 1..500 (100 por
// defecto); `cursor` vacío = desde el principio, y después el `NextCursor` de la
// página anterior (cursor.go). Toda consulta lleva LIMIT y ninguna OFFSET: una página
// profunda cuesta lo que una primera.
func (s *Store) ListarRegistros(ctx context.Context, a domain.Actor, tabla, vista string, limite int, cursor string) (PaginaRegistros, error) {
	if limite <= 0 {
		limite = domain.PaginaPorDefecto
	}
	limite = min(limite, domain.MaxPagina)
	var out PaginaRegistros
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		base, err := baseDeTabla(ctx, tx, a, tabla)
		if err != nil {
			return err
		}
		rol, err := rolEn(ctx, tx, a, base, tabla)
		if err != nil {
			return err
		}
		if !domain.Permite(rol, domain.AccLeer) {
			return domain.ErrSinPermiso
		}
		out.Role = rol
		if out.Fields, err = camposDe(ctx, tx, a, tabla); err != nil {
			return err
		}
		var cfg domain.ConfigVista
		if vista != "" {
			var raw []byte
			if err := tx.QueryRow(ctx, `SELECT config FROM base_views WHERE id = $1 AND table_id = $2 AND tenant = $3`, vista, tabla, a.Tenant).Scan(&raw); err != nil {
				return errNoFilas(err)
			}
			_ = json.Unmarshal(raw, &cfg)
		}
		if out.Records, out.NextCursor, err = leerRegistros(ctx, tx, a, tabla, out.Fields, cfg, limite, cursor); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM base_recalc WHERE tenant = $1 AND table_id = $2)`, a.Tenant, tabla).Scan(&out.Recalculating)
	})
	return out, err
}

// maxBytesLectura acota lo que una sola lectura carga en memoria. Sin él, exportar una
// tabla llena (50.000 registros de hasta 60 KB) pediría ~3 GB al proceso y cualquier
// lector podría tumbar el servicio. Variable y no constante para que el test la baje.
//
// ponytail: techo fijo; exportación en streaming con cursor si una tabla legítima lo supera
// (mientras, se pagina por la API).
var maxBytesLectura = 64 << 20

// leerRegistros es LA consulta de registros de una tabla con la configuración de una
// vista (filtros y orden) y sus enlaces: la usan la rejilla, la exportación y el conector
// de solo lectura, así que todos ven exactamente lo mismo. Devuelve además el cursor de
// la página siguiente ("" si no hay más). Quien llama ya ha autorizado.
func leerRegistros(ctx context.Context, tx pgx.Tx, a domain.Actor, tabla string, campos []domain.Campo, cfg domain.ConfigVista, limite int, cursor string) ([]domain.Registro, string, error) {
	q := construirConsulta(campos, cfg)
	if cursor != "" {
		if err := q.despuesDe(cursor); err != nil {
			return nil, "", err
		}
	}
	// Uno de más: es como se sabe si hay otra página sin contar la tabla.
	args := append([]any{a.Tenant, tabla}, q.args...)
	args = append(args, limite+1)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT r.id::text, r.data, r.rev, r.created_by, r.updated_by, r.created_at, r.updated_at
		  FROM base_records r WHERE r.tenant = $1 AND r.table_id = $2%s
		 ORDER BY %s LIMIT $%d`, q.where, q.order, len(args)), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []domain.Registro{}
	var ids []string
	var leido int
	hayMas := false
	for rows.Next() {
		r := domain.Registro{TableID: tabla, Links: map[string][]string{}}
		var data []byte
		if err := rows.Scan(&r.ID, &data, &r.Rev, &r.CreatedBy, &r.UpdatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, "", err
		}
		if len(out) == limite {
			// El de más no se entrega (ni cuenta para el techo de bytes): solo dice que hay otra página.
			hayMas = true
			break
		}
		if leido += len(data); leido > maxBytesLectura {
			return nil, "", fmt.Errorf("%w: la lectura no cabe de una vez", domain.ErrLimite)
		}
		if err := json.Unmarshal(data, &r.Data); err != nil || r.Data == nil {
			r.Data = map[string]json.RawMessage{}
		}
		out = append(out, r)
		ids = append(ids, r.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	siguiente := ""
	if hayMas {
		siguiente = q.cursorDe(out[limite-1])
	}
	if err := cargarAdjuntos(ctx, tx, a, campos, out, ids); err != nil {
		return nil, "", err
	}
	return out, siguiente, cargarEnlaces(ctx, tx, a, out, ids)
}

func cargarEnlaces(ctx context.Context, tx pgx.Tx, a domain.Actor, regs []domain.Registro, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT from_record::text, field_id::text, to_record::text FROM base_links
		 WHERE tenant = $1 AND from_record = ANY($2::uuid[]) ORDER BY field_id, to_record LIMIT 100000`, a.Tenant, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	idx := make(map[string]int, len(regs))
	for i, r := range regs {
		idx[r.ID] = i
	}
	for rows.Next() {
		var desde, campo, hacia string
		if err := rows.Scan(&desde, &campo, &hacia); err != nil {
			return err
		}
		r := &regs[idx[desde]]
		r.Links[campo] = append(r.Links[campo], hacia)
	}
	return rows.Err()
}

// hayHueco comprueba el techo de registros de la tabla contra su contador (lo mantiene
// un trigger, migración 0005) y lo bloquea hasta el final de la transacción: dos altas a
// la vez no pasan las dos por el último hueco. Sin contar la tabla en cada alta.
func hayHueco(ctx context.Context, tx pgx.Tx, tenant, tabla string) error {
	var n int64
	if err := errNoFilas(tx.QueryRow(ctx, `SELECT record_count FROM base_tables WHERE id = $1 AND tenant = $2 FOR NO KEY UPDATE`, tabla, tenant).Scan(&n)); err != nil {
		return err
	}
	if n >= domain.MaxRegistrosPorTabla {
		return domain.ErrLimite
	}
	return nil
}

// CrearRegistro añade un registro (editor). links es {campo de enlace: ids}.
func (s *Store) CrearRegistro(ctx context.Context, a domain.Actor, tabla string, data map[string]json.RawMessage, links map[string][]string) (domain.Registro, error) {
	var r domain.Registro
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		base, err := autorizarTabla(ctx, tx, a, tabla, domain.AccEscribirReg)
		if err != nil {
			return err
		}
		campos, err := camposDe(ctx, tx, a, tabla)
		if err != nil {
			return err
		}
		limpio, err := domain.ValidarDatos(campos, data)
		if err != nil {
			return err
		}
		if err := domain.TamanoDatos(limpio); err != nil {
			return err
		}
		if err := hayHueco(ctx, tx, a.Tenant, tabla); err != nil {
			return err
		}
		r = domain.Registro{TableID: tabla, Data: limpio, Links: map[string][]string{}}
		if err := tx.QueryRow(ctx, `
			INSERT INTO base_records (tenant, base_id, table_id, data, created_by, updated_by)
			VALUES ($1, $2, $3, $4::jsonb, $5, $5) RETURNING id::text, rev, created_by, updated_by, created_at, updated_at`,
			a.Tenant, base, tabla, jsonDe(limpio), a.Sub).
			Scan(&r.ID, &r.Rev, &r.CreatedBy, &r.UpdatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return err
		}
		if _, err := guardarEnlaces(ctx, tx, a, campos, tabla, r.ID, links); err != nil {
			return err
		}
		for k, v := range links {
			r.Links[k] = v
		}
		if err := aplicarFormulas(ctx, tx, a, base, &r); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, &r.ID, nil, "record.create", nil, jsonDe(limpio))
	})
	return r, err
}

// guardarEnlaces sustituye los enlaces de los campos indicados. Cada id de destino
// ha de ser un registro de la tabla a la que apunta el campo; los de otro tenant ni
// se ven (RLS) y los de otra tabla o base no coinciden, y las claves foráneas
// compuestas lo repiten en la base de datos. Devuelve los enlaces anteriores.
func guardarEnlaces(ctx context.Context, tx pgx.Tx, a domain.Actor, campos []domain.Campo, tabla, registro string, links map[string][]string) (map[string][]string, error) {
	antes := map[string][]string{}
	for campoID, ids := range links {
		i := slices.IndexFunc(campos, func(c domain.Campo) bool { return c.ID == campoID && c.Kind == domain.TipoRelacion })
		if i < 0 {
			return nil, fmt.Errorf("%w: campo de enlace desconocido", domain.ErrInvalido)
		}
		destino := campos[i].LinkTableID
		ids, err := domain.ValidarEnlaces(ids)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_records WHERE tenant = $1 AND table_id = $2 AND id = ANY($3::uuid[])`,
				a.Tenant, destino, ids).Scan(&n); err != nil {
				return nil, err
			}
			if n != len(ids) {
				return nil, fmt.Errorf("%w: enlace a un registro que no existe", domain.ErrInvalido)
			}
		}
		rows, err := tx.Query(ctx, `SELECT to_record::text FROM base_links WHERE tenant = $1 AND field_id = $2 AND from_record = $3 ORDER BY to_record LIMIT $4`,
			a.Tenant, campoID, registro, domain.MaxEnlaces)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			antes[campoID] = append(antes[campoID], id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM base_links WHERE tenant = $1 AND field_id = $2 AND from_record = $3`, a.Tenant, campoID, registro); err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO base_links (tenant, field_id, from_record, from_table_id, to_record, to_table_id)
				SELECT $1::text, $2::uuid, $3::uuid, $4::uuid, x, $5::uuid FROM unnest($6::uuid[]) x`,
				a.Tenant, campoID, registro, tabla, destino, ids); err != nil {
				return nil, err
			}
		}
	}
	return antes, nil
}

// ActualizarRegistro cambia campos de un registro (editor). `rev` es la versión que
// el cliente vio: si otra persona escribió entretanto, ErrConflicto. Un valor
// vacío borra el campo; lo que no se menciona no se toca.
func (s *Store) ActualizarRegistro(ctx context.Context, a domain.Actor, registro string, rev int64, data map[string]json.RawMessage, links map[string][]string) (domain.Registro, error) {
	if rev < 1 {
		return domain.Registro{}, fmt.Errorf("%w: falta la versión", domain.ErrInvalido)
	}
	var r domain.Registro
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		var tabla, base string
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT table_id::text, base_id::text FROM base_records WHERE id = $1 AND tenant = $2`, registro, a.Tenant).Scan(&tabla, &base)); err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccEscribirReg); err != nil {
			return err
		}
		var actual []byte
		var revActual int64
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT data, rev FROM base_records WHERE id = $1 AND tenant = $2 FOR UPDATE`, registro, a.Tenant).Scan(&actual, &revActual)); err != nil {
			return err
		}
		if revActual != rev {
			return domain.ErrConflicto
		}
		campos, err := camposDe(ctx, tx, a, tabla)
		if err != nil {
			return err
		}
		parcial, err := domain.ValidarParcial(campos, data)
		if err != nil {
			return err
		}
		merged, cambios := fusionar(actual, parcial)
		if err := domain.TamanoDatos(merged); err != nil {
			return err
		}
		r = domain.Registro{ID: registro, TableID: tabla, Data: merged, Links: map[string][]string{}}
		if err := tx.QueryRow(ctx, `
			UPDATE base_records SET data = $3::jsonb, rev = rev + 1, updated_by = $4, updated_at = now()
			 WHERE id = $1 AND tenant = $2
			RETURNING rev, created_by, updated_by, created_at, updated_at`, registro, a.Tenant, jsonDe(merged), a.Sub).
			Scan(&r.Rev, &r.CreatedBy, &r.UpdatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return err
		}
		for _, c := range cambios {
			if err := auditar(ctx, tx, a, base, &tabla, &registro, &c.campo, "record.update", c.antes, c.despues); err != nil {
				return err
			}
		}
		antes, err := guardarEnlaces(ctx, tx, a, campos, tabla, registro, links)
		if err != nil {
			return err
		}
		for campo, ids := range links {
			if err := auditar(ctx, tx, a, base, &tabla, &registro, &campo, "record.link", jsonDe(antes[campo]), jsonDe(ids)); err != nil {
				return err
			}
		}
		return aplicarFormulas(ctx, tx, a, base, &r)
	})
	return r, err
}

// BorrarRegistro borra un registro (editor). Si otros registros lo enlazan, no se
// borra (ErrReferenciado) salvo `forzar`, que quita esos enlaces y exige ser
// propietario: deshace relaciones en tablas que quizá el editor ni ve.
func (s *Store) BorrarRegistro(ctx context.Context, a domain.Actor, registro string, forzar bool) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		var tabla, base string
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT table_id::text, base_id::text FROM base_records WHERE id = $1 AND tenant = $2`, registro, a.Tenant).Scan(&tabla, &base)); err != nil {
			return err
		}
		acc := domain.AccEscribirReg
		if forzar {
			acc = domain.AccEsquema
		}
		if err := autorizar(ctx, tx, a, base, tabla, acc); err != nil {
			return err
		}
		// El mismo orden de bloqueos que un alta (tabla y luego registros): el contador de la
		// tabla se toca en las dos, y al revés un alta que enlaza con este registro y este
		// borrado se esperarían la una a la otra.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM base_tables WHERE id = $1 AND tenant = $2 FOR NO KEY UPDATE`, tabla, a.Tenant); err != nil {
			return err
		}
		var enlaces int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM base_links WHERE tenant = $1 AND to_record = $2 LIMIT 1001) x`, a.Tenant, registro).Scan(&enlaces); err != nil {
			return err
		}
		if enlaces > 0 && !forzar {
			return domain.ErrReferenciado
		}
		// Quien enlazaba con él pierde un sumando: se recalcula después de borrar. Solo pasa
		// con `forzar` (sin él, un registro enlazado no se borra).
		var dependientes map[string][]string
		if enlaces > 0 {
			d, err := dependientesDe(ctx, tx, a, registro)
			if err != nil {
				return err
			}
			dependientes = d
		}
		if _, err := tx.Exec(ctx, `DELETE FROM base_links WHERE tenant = $1 AND to_record = $2`, a.Tenant, registro); err != nil {
			return err
		}
		var antes []byte
		if err := errNoFilas(tx.QueryRow(ctx, `DELETE FROM base_records WHERE id = $1 AND tenant = $2 RETURNING data`, registro, a.Tenant).Scan(&antes)); err != nil {
			if esFK(err) {
				return domain.ErrReferenciado
			}
			return err
		}
		if err := recalcularDependientes(ctx, tx, a, base, dependientes, enlaces > presupuestoRecalculo); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, &registro, nil, "record.delete", antes, jsonDe(map[string]int{"links_removed": enlaces}))
	})
}

// aplicarFormulas recalcula lo que depende del registro recién escrito y deja en `r` sus
// valores de fórmula ya calculados, que es lo que se devuelve al cliente.
func aplicarFormulas(ctx context.Context, tx pgx.Tx, a domain.Actor, base string, r *domain.Registro) error {
	parches, err := trasEscribir(ctx, tx, a, base, r.TableID, []string{r.ID})
	if err != nil {
		return err
	}
	for campo, v := range parches[r.ID] {
		if v == nil {
			delete(r.Data, campo)
		} else {
			r.Data[campo] = v
		}
	}
	return nil
}

// cambio es un campo que cambia de valor en una actualización, para la traza.
type cambio struct {
	campo          string
	antes, despues json.RawMessage
}

// fusionar aplica el cambio parcial sobre los datos actuales: un valor nil borra el
// campo y los que no cambian no cuentan. Los cambios salen ordenados por campo para
// que la traza sea determinista.
func fusionar(actual []byte, parcial map[string]json.RawMessage) (map[string]json.RawMessage, []cambio) {
	merged := map[string]json.RawMessage{}
	_ = json.Unmarshal(actual, &merged)
	var cambios []cambio
	for k, v := range parcial {
		ant := merged[k]
		if bytes.Equal(ant, v) {
			continue
		}
		cambios = append(cambios, cambio{k, ant, v})
		if v == nil {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	slices.SortFunc(cambios, func(x, y cambio) int { return strings.Compare(x.campo, y.campo) })
	return merged, cambios
}
