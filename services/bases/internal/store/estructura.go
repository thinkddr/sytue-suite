// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// camposDe lee los campos de una tabla en su orden.
func camposDe(ctx context.Context, tx pgx.Tx, a domain.Actor, tabla string) ([]domain.Campo, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, table_id::text, name, kind, options, coalesce(link_table_id::text, ''), position
		  FROM base_fields WHERE tenant = $1 AND table_id = $2 ORDER BY position, created_at, id LIMIT $3`,
		a.Tenant, tabla, domain.MaxCamposPorTabla)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Campo{}
	for rows.Next() {
		c, err := leerCampo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// leerCampo lee una fila de base_fields (id, table_id, name, kind, options, link_table_id, position).
func leerCampo(rows pgx.Rows) (domain.Campo, error) {
	var c domain.Campo
	var opts []byte
	if err := rows.Scan(&c.ID, &c.TableID, &c.Name, &c.Kind, &opts, &c.LinkTableID, &c.Position); err != nil {
		return c, err
	}
	var o struct {
		Choices []string    `json:"choices"`
		Formula string      `json:"formula"`
		Result  domain.Tipo `json:"result"`
	}
	_ = json.Unmarshal(opts, &o)
	c.Choices, c.Formula, c.Result = o.Choices, o.Formula, o.Result
	return c, nil
}

// TablaConCampos es una tabla con su esquema, lo que pide la interfaz de una vez.
type TablaConCampos struct {
	domain.Tabla
	Fields []domain.Campo `json:"fields"`
}

// ListarTablas devuelve las tablas de la base a las que el actor puede acceder,
// con sus campos. Quien solo tiene acceso a una tabla no ve las demás.
func (s *Store) ListarTablas(ctx context.Context, a domain.Actor, base string) ([]TablaConCampos, error) {
	out := []TablaConCampos{}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		owner, acl, err := cargarACL(ctx, tx, a, base)
		if err != nil {
			return err
		}
		if owner != a.Sub && len(acl) == 0 {
			return domain.ErrNoExiste // sin ningún acceso: indistinguible de no existir
		}
		rows, err := tx.Query(ctx, `SELECT id::text, base_id::text, name, position FROM base_tables WHERE tenant = $1 AND base_id = $2 ORDER BY position, created_at, id LIMIT $3`,
			a.Tenant, base, domain.MaxTablasPorBase)
		if err != nil {
			return err
		}
		var tablas []domain.Tabla
		for rows.Next() {
			var t domain.Tabla
			if err := rows.Scan(&t.ID, &t.BaseID, &t.Name, &t.Position); err != nil {
				rows.Close()
				return err
			}
			tablas = append(tablas, t)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, t := range tablas {
			rol, ok := domain.ResolverRol(owner, acl, a, t.ID)
			if !ok {
				continue
			}
			t.Role = rol
			campos, err := camposDe(ctx, tx, a, t.ID)
			if err != nil {
				return err
			}
			out = append(out, TablaConCampos{Tabla: t, Fields: campos})
		}
		return nil
	})
	return out, err
}

// CrearTabla añade una tabla a la base (propietario).
func (s *Store) CrearTabla(ctx context.Context, a domain.Actor, base, nombre string) (domain.Tabla, error) {
	nombre, err := domain.LimpiarNombre(nombre)
	if err != nil {
		return domain.Tabla{}, err
	}
	t := domain.Tabla{BaseID: base, Name: nombre, Role: domain.RolPropietario}
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, base, "", domain.AccEsquema); err != nil {
			return err
		}
		return crearTabla(ctx, tx, a, &t)
	})
	return t, err
}

func crearTabla(ctx context.Context, tx pgx.Tx, a domain.Actor, t *domain.Tabla) error {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_tables WHERE tenant = $1 AND base_id = $2`, a.Tenant, t.BaseID).Scan(&n); err != nil {
		return err
	}
	if n >= domain.MaxTablasPorBase {
		return domain.ErrLimite
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO base_tables (tenant, base_id, name, position) VALUES ($1, $2, $3, $4) RETURNING id::text, position`,
		a.Tenant, t.BaseID, t.Name, n).Scan(&t.ID, &t.Position); err != nil {
		return err
	}
	return auditar(ctx, tx, a, t.BaseID, &t.ID, nil, nil, "table.create", nil, jsonDe(map[string]string{"name": t.Name}))
}

// RenombrarTabla cambia el nombre (propietario).
func (s *Store) RenombrarTabla(ctx context.Context, a domain.Actor, tabla, nombre string) error {
	nombre, err := domain.LimpiarNombre(nombre)
	if err != nil {
		return err
	}
	return s.en(ctx, a, func(tx pgx.Tx) error {
		base, err := autorizarTabla(ctx, tx, a, tabla, domain.AccEsquema)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE base_tables SET name = $3 WHERE id = $1 AND tenant = $2`, tabla, a.Tenant, nombre); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "table.rename", nil, jsonDe(map[string]string{"name": nombre}))
	})
}

// BorrarTabla borra la tabla con sus campos, registros y vistas (propietario). Si
// otra tabla tiene un campo de enlace hacia ella, o un registro suyo está enlazado,
// NO se borra (ErrReferenciado): primero hay que quitar el campo de enlace.
func (s *Store) BorrarTabla(ctx context.Context, a domain.Actor, tabla string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		base, err := autorizarTabla(ctx, tx, a, tabla, domain.AccEsquema)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM base_tables WHERE id = $1 AND tenant = $2`, tabla, a.Tenant); err != nil {
			if esFK(err) {
				return domain.ErrReferenciado
			}
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, nil, "table.delete", nil, nil)
	})
}

// CrearCampo añade un campo a la tabla (propietario).
func (s *Store) CrearCampo(ctx context.Context, a domain.Actor, tabla string, in domain.EntradaCampo) (domain.Campo, error) {
	in, err := domain.ValidarEntradaCampo(in)
	if err != nil {
		return domain.Campo{}, err
	}
	c := domain.Campo{TableID: tabla, Name: in.Name, Kind: in.Kind, Choices: in.Choices, LinkTableID: in.LinkTableID, Formula: in.Formula, Result: in.Result}
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		base, err := autorizarTabla(ctx, tx, a, tabla, domain.AccEsquema)
		if err != nil {
			return err
		}
		if c.Kind != domain.TipoFormula {
			return crearCampo(ctx, tx, a, base, &c)
		}
		// Una fórmula se valida contra el esquema de TODA la base (sus agregados miran
		// otras tablas) y, una vez creada, se calcula para los registros que ya hay.
		esquema, err := esquemaDeBase(ctx, tx, a, base)
		if err != nil {
			return err
		}
		formulas := 0
		for _, x := range esquema[tabla] {
			if x.Kind == domain.TipoFormula {
				formulas++
			}
		}
		if formulas >= domain.MaxFormulasPorTabla {
			return domain.ErrLimite
		}
		if err := comprobarFormula(esquema, c); err != nil {
			return err
		}
		if err := crearCampo(ctx, tx, a, base, &c); err != nil {
			return err
		}
		esquema[tabla] = append(esquema[tabla], c)
		r := &recalculo{tx: tx, a: a, base: base, esquema: esquema, quedan: presupuestoRecalculo, parches: map[string]map[string]json.RawMessage{}}
		return r.tablaEntera(ctx, tabla)
	})
	return c, err
}

func crearCampo(ctx context.Context, tx pgx.Tx, a domain.Actor, base string, c *domain.Campo) error {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_fields WHERE tenant = $1 AND table_id = $2`, a.Tenant, c.TableID).Scan(&n); err != nil {
		return err
	}
	if n >= domain.MaxCamposPorTabla {
		return domain.ErrLimite
	}
	if c.Kind == domain.TipoRelacion {
		// El destino ha de ser una tabla de ESTA base. La clave foránea compuesta lo
		// garantiza también, pero así el error es un 404 y no un 500.
		b, err := baseDeTabla(ctx, tx, a, c.LinkTableID)
		if err != nil || b != base {
			return domain.ErrNoExiste
		}
	}
	opciones := []byte(`{}`)
	switch c.Kind { //nolint:exhaustive // el resto no tiene opciones
	case domain.TipoSeleccion, domain.TipoMultiple:
		opciones = jsonDe(map[string][]string{"choices": c.Choices})
	case domain.TipoFormula:
		opciones = jsonDe(map[string]any{"formula": c.Formula, "result": c.Result})
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO base_fields (tenant, base_id, table_id, name, kind, options, link_table_id, position)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::uuid, $8) RETURNING id::text, position`,
		a.Tenant, base, c.TableID, c.Name, c.Kind, opciones, ptr(c.LinkTableID), n).Scan(&c.ID, &c.Position); err != nil {
		return err
	}
	return auditar(ctx, tx, a, base, &c.TableID, nil, &c.ID, "field.create", nil, jsonDe(map[string]any{"name": c.Name, "kind": c.Kind}))
}

// RenombrarCampo cambia el nombre de un campo (propietario). El tipo no cambia.
func (s *Store) RenombrarCampo(ctx context.Context, a domain.Actor, campo, nombre string) error {
	nombre, err := domain.LimpiarNombre(nombre)
	if err != nil {
		return err
	}
	return s.en(ctx, a, func(tx pgx.Tx) error {
		tabla, base, err := tablaDeCampo(ctx, tx, a, campo)
		if err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccEsquema); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE base_fields SET name = $3 WHERE id = $1 AND tenant = $2`, campo, a.Tenant, nombre); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, &campo, "field.rename", nil, jsonDe(map[string]string{"name": nombre}))
	})
}

func tablaDeCampo(ctx context.Context, tx pgx.Tx, a domain.Actor, campo string) (tabla, base string, err error) {
	err = tx.QueryRow(ctx, `SELECT table_id::text, base_id::text FROM base_fields WHERE id = $1 AND tenant = $2`, campo, a.Tenant).Scan(&tabla, &base)
	return tabla, base, errNoFilas(err)
}

// BorrarCampo borra el campo y sus valores (propietario). Los enlaces del campo se
// van con él.
func (s *Store) BorrarCampo(ctx context.Context, a domain.Actor, campo string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		tabla, base, err := tablaDeCampo(ctx, tx, a, campo)
		if err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccEsquema); err != nil {
			return err
		}
		// Un campo que usa una fórmula (de esta tabla o, por un enlace, de otra) no se
		// borra: la fórmula quedaría apuntando a nada. Primero se cambia o se borra ella.
		esquema, err := esquemaDeBase(ctx, tx, a, base)
		if err != nil {
			return err
		}
		for _, campos := range esquema {
			for _, c := range campos {
				f, err := domain.AnalizarFormula(c.Formula)
				if c.Kind != domain.TipoFormula || c.ID == campo || err != nil {
					continue
				}
				if slices.ContainsFunc(f.Refs, func(r domain.RefFormula) bool { return r.Campo == campo || r.Enlace == campo }) {
					return domain.ErrReferenciado
				}
			}
		}
		// Quita también las referencias a ese campo en las vistas y su valor de cada registro.
		if _, err := tx.Exec(ctx, `UPDATE base_records SET data = data - $3::text WHERE tenant = $1 AND table_id = $2 AND data ? $3::text`, a.Tenant, tabla, campo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM base_fields WHERE id = $1 AND tenant = $2`, campo, a.Tenant); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, &campo, "field.delete", nil, nil)
	})
}
