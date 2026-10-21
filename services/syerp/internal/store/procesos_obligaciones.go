// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

type ProcesoObligacion struct {
	Fase       int    `json:"fase"`
	Referencia string `json:"referencia"`
}

// PreparacionObligaciones obtiene contadores y progreso en una sola transacción
// para todo el calendario; no hace una petición por obligación.
func (s *Store) PreparacionObligaciones(ctx context.Context, desde, hasta time.Time, periodos []string) (map[string]map[string]int, map[string]ProcesoObligacion, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, nil, err
	}
	var pendientes map[string]map[string]int
	procesos := map[string]ProcesoObligacion{}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		pendientes, err = pendientesEnTx(ctx, tx, tenant, desde, hasta)
		if err != nil {
			return err
		}
		filas, err := tx.Query(ctx, `SELECT clave, periodo, fase, referencia
			FROM procesos_obligaciones WHERE tenant = $1 AND periodo = ANY($2::text[])`, tenant, periodos)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var clave, periodo string
			var p ProcesoObligacion
			if err := filas.Scan(&clave, &periodo, &p.Fase, &p.Referencia); err != nil {
				return err
			}
			procesos[clave+":"+periodo] = p
		}
		return filas.Err()
	})
	return pendientes, procesos, err
}

// FacturasPendientesProceso pagina el mismo conjunto que cuenta Pendientes:
// borradores y facturas confirmadas/pagadas que aún no tienen asiento. El rango
// se aplica ANTES del límite; una factura de otro período nunca ocupa su página.
func (s *Store) FacturasPendientesProceso(ctx context.Context, desde, hasta time.Time, modelos []string, cursor string, limite int) (Pagina, error) {
	pag := Pagina{Registros: []Registro{}}
	if len(modelos) == 0 {
		return pag, nil
	}
	for _, modelo := range modelos {
		if modelo != "factura_venta" && modelo != "factura_compra" {
			return pag, domain.ErrDatos
		}
	}
	if limite <= 0 || limite > 100 {
		limite = 30
	}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return pag, err
	}
	marca, id, _, err := abrirPaginacion(cursor, true)
	if err != nil {
		return pag, fmt.Errorf("cursor: %w", domain.ErrDatos)
	}
	if cursor != "" {
		if _, err := uuid.Parse(id); err != nil {
			return pag, fmt.Errorf("cursor: %w", domain.ErrDatos)
		}
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT r.id::text, r.model, r.ref, r.etiqueta, r.estado, r.data, r.created_at, r.updated_at
			FROM records r WHERE r.tenant = $1 AND r.model = ANY($2::text[])
			AND r.deleted_at IS NULL AND r.data->>'fecha' BETWEEN $3 AND $4
			AND (`+facturaPendienteSQL+`)
			AND ($5::timestamptz IS NULL OR (r.created_at, r.id) < ($5::timestamptz, $6::uuid))
			ORDER BY r.created_at DESC, r.id DESC LIMIT $7`,
			tenant, modelos, desde.Format(time.DateOnly), hasta.Format(time.DateOnly), nuloSiCero(marca), nuloSiVacio(id), limite+1)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var r Registro
			if err := escanearRegistro(filas, &r); err != nil {
				return err
			}
			pag.Registros = append(pag.Registros, r)
		}
		return filas.Err()
	})
	if len(pag.Registros) > limite {
		ultimo := pag.Registros[limite-1]
		pag.Registros = pag.Registros[:limite]
		pag.Siguiente = cerrarCursor(ultimo.CreadoEn, ultimo.ID)
	}
	return pag, err
}

func (s *Store) ProcesoObligacion(ctx context.Context, clave, periodo string) (ProcesoObligacion, error) {
	var out ProcesoObligacion
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT fase, referencia FROM procesos_obligaciones WHERE tenant = current_setting('app.tenant', true) AND clave = $1 AND periodo = $2`, clave, periodo).Scan(&out.Fase, &out.Referencia)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return out, err
}

// AvanzarProceso exige secuencia y comprobantes reales dentro de la transacción.
// La referencia es la declarada por quien presenta; no un CSV verificado por AEAT.
func (s *Store) AvanzarProceso(ctx context.Context, clave, periodo string, fase int, referencia string, documentos []string, por string) (ProcesoObligacion, error) {
	var out ProcesoObligacion
	if fase < 1 || fase > 4 || utf8.RuneCountInString(referencia) > 200 {
		return out, domain.ErrDatos
	}
	if len(documentos) == 0 {
		documentos = []string{"justificante"}
	}
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO procesos_obligaciones (tenant,clave,periodo) VALUES (current_setting('app.tenant',true),$1,$2) ON CONFLICT DO NOTHING`, clave, periodo); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT fase, referencia FROM procesos_obligaciones WHERE tenant = current_setting('app.tenant',true) AND clave=$1 AND periodo=$2 FOR UPDATE`, clave, periodo).Scan(&out.Fase, &out.Referencia); err != nil {
			return err
		}
		if fase <= out.Fase {
			return nil
		}
		if fase != out.Fase+1 {
			return fmt.Errorf("completa el paso anterior: %w", domain.ErrDatos)
		}
		if fase == 4 {
			if strings.TrimSpace(referencia) == "" {
				return fmt.Errorf("indica la referencia del justificante: %w", domain.ErrDatos)
			}
			for _, doc := range documentos {
				var hay bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adjuntos WHERE tenant=current_setting('app.tenant',true) AND modelo='obligacion' AND registro=$1)`, clave+":"+periodo+":"+doc).Scan(&hay); err != nil {
					return err
				}
				if !hay {
					return fmt.Errorf("guarda el documento %s antes de terminar: %w", doc, domain.ErrDatos)
				}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE procesos_obligaciones SET fase=$3, referencia=$4, actualizado_por=$5, documentos=$6, updated_at=now() WHERE tenant=current_setting('app.tenant',true) AND clave=$1 AND periodo=$2`, clave, periodo, fase, strings.TrimSpace(referencia), por, documentos); err != nil {
			return err
		}
		out = ProcesoObligacion{Fase: fase, Referencia: strings.TrimSpace(referencia)}
		return nil
	})
	return out, err
}
