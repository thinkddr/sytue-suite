// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// ImportarTabla crea una tabla nueva en la base a partir de un CSV ya leído y
// tipado (domain.LeerCSV): sus campos, y sus filas en UNA sentencia. Solo el
// propietario (crea esquema). Todo o nada: una fila inválida deshace la importación.
func (s *Store) ImportarTabla(ctx context.Context, a domain.Actor, base, nombre string, t domain.TablaCSV) (domain.Tabla, []domain.Campo, error) {
	nombre, err := domain.LimpiarNombre(nombre)
	if err != nil {
		return domain.Tabla{}, nil, err
	}
	tabla := domain.Tabla{BaseID: base, Name: nombre, Role: domain.RolPropietario}
	var campos []domain.Campo
	err = s.en(ctx, a, func(tx pgx.Tx) error {
		if err := autorizar(ctx, tx, a, base, "", domain.AccEsquema); err != nil {
			return err
		}
		if err := crearTabla(ctx, tx, a, &tabla); err != nil {
			return err
		}
		for _, f := range t.Campos {
			if _, err := domain.ValidarEntradaCampo(f); err != nil {
				return err
			}
			c := domain.Campo{TableID: tabla.ID, Name: f.Name, Kind: f.Kind}
			if err := crearCampo(ctx, tx, a, base, &c); err != nil {
				return err
			}
			campos = append(campos, c)
		}
		filas := make([]map[string]json.RawMessage, 0, len(t.Filas))
		for _, fila := range t.Filas {
			m := make(map[string]json.RawMessage, len(campos))
			for j, v := range fila {
				if v != nil {
					m[campos[j].ID] = v
				}
			}
			if err := domain.TamanoDatos(m); err != nil {
				return err
			}
			filas = append(filas, m)
		}
		if len(filas) > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO base_records (tenant, base_id, table_id, data, created_by, updated_by)
				SELECT $1::text, $2::uuid, $3::uuid, d, $4::text, $4::text FROM jsonb_array_elements($5::jsonb) d`,
				a.Tenant, base, tabla.ID, a.Sub, jsonDe(filas)); err != nil {
				return err
			}
		}
		return auditar(ctx, tx, a, base, &tabla.ID, nil, nil, "table.import", nil,
			jsonDe(map[string]int{"rows": len(filas), "fields": len(campos)}))
	})
	return tabla, campos, err
}

// Exportacion es un TRAMO de una tabla listo para serializar: hasta domain.MaxPagina
// registros, en el orden por defecto, con el texto de los registros que enlazan.
type Exportacion struct {
	Tabla   string
	Fields  []domain.Campo
	Records []domain.Registro
	// Labels nombra cada registro enlazado desde este tramo por su campo principal
	// (etiquetaDe: el primero con valor), por id de registro.
	Labels map[string]string
}

// ExportarTabla recorre todos los registros de la tabla (lector) por tramos y entrega
// cada uno a `fn`; siempre hay al menos un tramo, aunque la tabla esté vacía. Cada tramo
// es una transacción corta que VUELVE a autorizar: no hay transacción abierta mientras el
// cliente descarga despacio, y quitarle el acceso a alguien corta su exportación en el
// tramo siguiente. El cursor hace que lo que se borre o añada entretanto no repita ni
// salte registros. Deja constancia en la traza al empezar: una exportación es la salida
// de datos de la organización y se audita como tal, se termine de descargar o no.
func (s *Store) ExportarTabla(ctx context.Context, a domain.Actor, tabla string, fn func(Exportacion) error) error {
	cursor := ""
	for tramo := 0; ; tramo++ {
		var ex Exportacion
		var siguiente string
		err := s.en(ctx, a, func(tx pgx.Tx) error {
			base, err := autorizarTabla(ctx, tx, a, tabla, domain.AccLeer)
			if err != nil {
				return err
			}
			var total int64
			if err := tx.QueryRow(ctx, `SELECT name, record_count FROM base_tables WHERE id = $1 AND tenant = $2`, tabla, a.Tenant).Scan(&ex.Tabla, &total); err != nil {
				return err
			}
			if ex.Fields, err = camposDe(ctx, tx, a, tabla); err != nil {
				return err
			}
			if ex.Records, siguiente, err = leerRegistros(ctx, tx, a, tabla, ex.Fields, domain.ConfigVista{}, domain.MaxPagina, cursor); err != nil {
				return err
			}
			if ex.Labels, err = etiquetasDeEnlaces(ctx, tx, a, ex.Fields, ex.Records); err != nil {
				return err
			}
			if tramo > 0 {
				return nil
			}
			return auditar(ctx, tx, a, base, &tabla, nil, nil, "table.export", nil, jsonDe(map[string]int64{"rows": total}))
		})
		if err != nil {
			return fmt.Errorf("exportar: %w", err)
		}
		if err := fn(ex); err != nil {
			return err
		}
		if cursor = siguiente; cursor == "" {
			return nil
		}
	}
}
