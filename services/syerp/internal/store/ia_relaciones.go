// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
)

// RelacionIA comes from a catalog field and a model-proposed name, not from a
// client-supplied module. The HTTP layer resolves model and module beforehand.
type RelacionIA struct {
	Campo, Modelo, Modulo, Nombre string
}

type RelacionIAResuelta struct {
	ID, Etiqueta string
}

// ResolverRelacionesIA evaluates the actor's source and destination permissions and
// selects candidates in one statement and one tenant transaction, after the AI
// call. A revocation committed before the statement is visible; a concurrent
// later revocation is ordered after this observation, not fenced through the
// HTTP response. A database failure returns no relation, never open defaults.
func (s *Store) ResolverRelacionesIA(ctx context.Context, sub string, roles []string, moduloOrigen string, relaciones []RelacionIA) (map[string]RelacionIAResuelta, bool, error) {
	if sub == "" || moduloOrigen == "" || len(relaciones) > 32 {
		return nil, false, errors.New("erp: invalid relation request")
	}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, false, err
	}
	cols, err := prepararColumnasRelacionesIA(relaciones)
	if err != nil {
		return nil, false, err
	}
	permitidos, permitidosOrigen := nivelesPermitidosIA()
	admin := permisos.EsAdministradorDeLaOrganizacion(roles)
	out := make(map[string]RelacionIAResuelta)
	origenConocido, origenPermitido := false, false
	err = s.pool.InTxOpts(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			WITH solicitadas AS (
			  SELECT * FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[])
			    AS r(campo, modelo, modulo, nombre, defecto)
			)
			SELECT coalesce(r.campo, ''), coalesce(r.modulo, ''), coalesce(p.nivel, ''), coalesce(origen.nivel, ''), coalesce(c.id, ''), coalesce(c.etiqueta, '')
			  FROM (SELECT 1) base
			  LEFT JOIN permisos origen ON origen.tenant=$1 AND origen.sub=$7 AND origen.modulo=$10
			  LEFT JOIN solicitadas r ON true
			  LEFT JOIN permisos p ON p.tenant=$1 AND p.sub=$7 AND p.modulo=r.modulo
			  LEFT JOIN LATERAL (
			    SELECT id::text AS id, coalesce(nullif(etiqueta, ''), ref) AS etiqueta
			      FROM records
			     WHERE tenant=$1 AND model=r.modelo AND deleted_at IS NULL
			       AND plegar(etiqueta) LIKE '%' || plegar(r.nombre) || '%'
			       AND ($8::boolean OR coalesce(p.nivel, r.defecto) = ANY($9::text[]))
			       AND ($8::boolean OR coalesce(origen.nivel, $11) = ANY($12::text[]))
			     ORDER BY created_at DESC, id DESC LIMIT 1
			  ) c ON true`, tenant, cols.campos, cols.modelos, cols.modulos, cols.nombres, cols.defaults, sub, admin, permitidos, moduloOrigen, permisos.PorDefecto(moduloOrigen), permitidosOrigen)
		if err != nil {
			return err
		}
		defer rows.Close()
		var scanErr error
		origenConocido, origenPermitido, scanErr = scanRelacionesIA(rows, roles, moduloOrigen, out)
		return scanErr
	})
	if err != nil {
		return nil, false, err
	}
	if !origenConocido {
		return nil, false, errors.New("erp: missing source permission")
	}
	return out, origenPermitido, nil
}

type columnasRelacionesIA struct {
	campos, modelos, modulos, nombres, defaults []string
}

func prepararColumnasRelacionesIA(relaciones []RelacionIA) (columnasRelacionesIA, error) {
	cols := columnasRelacionesIA{
		campos:   make([]string, 0, len(relaciones)),
		modelos:  make([]string, 0, len(relaciones)),
		modulos:  make([]string, 0, len(relaciones)),
		nombres:  make([]string, 0, len(relaciones)),
		defaults: make([]string, 0, len(relaciones)),
	}
	seen := make(map[string]bool, len(relaciones))
	for _, relation := range relaciones {
		if relation.Campo == "" || relation.Modelo == "" || relation.Modulo == "" || relation.Nombre == "" || len(relation.Nombre) > 20000 || seen[relation.Campo] {
			return columnasRelacionesIA{}, errors.New("erp: invalid relation request")
		}
		seen[relation.Campo] = true
		cols.campos = append(cols.campos, relation.Campo)
		cols.modelos = append(cols.modelos, relation.Modelo)
		cols.modulos = append(cols.modulos, relation.Modulo)
		cols.nombres = append(cols.nombres, relation.Nombre)
		cols.defaults = append(cols.defaults, permisos.PorDefecto(relation.Modulo))
	}
	return cols, nil
}

func nivelesPermitidosIA() ([]string, []string) {
	var permitidos, permitidosOrigen []string
	for _, nivel := range permisos.Niveles {
		if permisos.Alcanza(nivel, permisos.Lectura) {
			permitidos = append(permitidos, nivel)
		}
		if permisos.Alcanza(nivel, permisos.Usuario) {
			permitidosOrigen = append(permitidosOrigen, nivel)
		}
	}
	return permitidos, permitidosOrigen
}

func scanRelacionesIA(rows pgx.Rows, roles []string, moduloOrigen string, out map[string]RelacionIAResuelta) (bool, bool, error) {
	origenConocido, origenPermitido := false, false
	for rows.Next() {
		var campo, modulo, nivel, nivelOrigen, id, etiqueta string
		if err := rows.Scan(&campo, &modulo, &nivel, &nivelOrigen, &id, &etiqueta); err != nil {
			return false, false, err
		}
		niveles := map[string]string{}
		if nivel != "" {
			niveles[modulo] = nivel
		}
		if nivelOrigen != "" {
			niveles[moduloOrigen] = nivelOrigen
		}
		permitido := permisos.Alcanza(permisos.Nivel(roles, niveles, moduloOrigen), permisos.Usuario)
		if origenConocido && permitido != origenPermitido {
			return false, false, errors.New("erp: inconsistent source permission")
		}
		origenConocido, origenPermitido = true, permitido
		if id != "" && permisos.Alcanza(permisos.Nivel(roles, niveles, modulo), permisos.Lectura) && permitido {
			out[campo] = RelacionIAResuelta{ID: id, Etiqueta: etiqueta}
		}
	}
	return origenConocido, origenPermitido, rows.Err()
}
