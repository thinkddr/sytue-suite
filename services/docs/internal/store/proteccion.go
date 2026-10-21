// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// ProteccionIniciada dice si el documento ya tiene su estado de protección evaluado.
func (s *Store) ProteccionIniciada(ctx context.Context, docID string) (bool, error) {
	var hay bool
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM doc_proteccion WHERE document_id = $1)`, docID).Scan(&hay)
	})
	return hay, err
}

// IniciarProteccion guarda el estado deducido del contenido que ya tenía el documento.
// Idempotente y sin pisar: si otra petición se adelantó, su fila (que ya ha podido
// avanzar con escrituras) manda sobre este estado, que es más viejo.
func (s *Store) IniciarProteccion(ctx context.Context, docID string, est domain.EstadoProteccion) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		// El bloqueo del documento serializa esto con AppendOps.
		var seq int64
		if err := tx.QueryRow(ctx,
			`SELECT seq FROM documents WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, docID).Scan(&seq); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		var hay bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM doc_proteccion WHERE document_id = $1)`, docID).Scan(&hay); err != nil {
			return err
		}
		if hay {
			return nil
		}
		if err := guardarProteccion(ctx, tx, t.Slug, docID, est, est.Claves, nil); err != nil {
			return err
		}
		if est.Secuencia != nil {
			return guardarSecuencia(ctx, tx, docID, est.Secuencia)
		}
		return nil
	})
}

// IniciarSecuencia establece la copia de la secuencia de un documento con secciones
// (domain/proteccion_secuencia.go) a partir del estado completo `st`. Las operaciones
// escritas desde que se cargó `st` se le añaden aquí, con el documento bloqueado, para que
// la copia sea exactamente la del seq actual. Idempotente: si ya existe, no la pisa.
func (s *Store) IniciarSecuencia(ctx context.Context, docID string, st *domain.State) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		var seq int64
		if err := tx.QueryRow(ctx,
			`SELECT seq FROM documents WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, docID).Scan(&seq); err != nil {
			if isNoRows(err) {
				return domain.ErrNotFound
			}
			return err
		}
		var hay bool
		if err := tx.QueryRow(ctx, `
			SELECT secuencia IS NOT NULL OR modelo = '' FROM doc_proteccion WHERE document_id = $1`, docID).Scan(&hay); err != nil {
			if isNoRows(err) {
				return nil // sin fila no hay secciones que proteger
			}
			return err
		}
		if hay {
			return nil // ya establecida, o el documento ya no tiene secciones
		}
		copia := domain.CopiaSecuencia(st)
		rows, err := tx.Query(ctx,
			`SELECT op FROM doc_operations WHERE document_id = $1 AND seq > $2 ORDER BY seq ASC`, docID, st.Seq)
		if err != nil {
			return err
		}
		defer rows.Close()
		var cola []domain.Op
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var op domain.Op
			if err := json.Unmarshal(raw, &op); err != nil {
				return fmt.Errorf("op corrupta: %w", err)
			}
			cola = append(cola, op)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		domain.AplicarSecuencia(copia, cola)
		return guardarSecuencia(ctx, tx, docID, copia)
	})
}

// guardarSecuencia escribe (o, con nil, borra) la copia de la secuencia.
func guardarSecuencia(ctx context.Context, tx pgx.Tx, docID string, sec *domain.State) error {
	var valor *string
	if sec != nil {
		b, err := domain.SecuenciaJSON(sec)
		if err != nil {
			return err
		}
		v := string(b)
		valor = &v
	}
	_, err := tx.Exec(ctx, `UPDATE doc_proteccion SET secuencia = $2 WHERE document_id = $1`, docID, valor)
	return err
}

// cargarProteccion lee el estado dentro de la transacción de escritura.
func cargarProteccion(ctx context.Context, tx pgx.Tx, docID string) (domain.EstadoProteccion, bool, error) {
	est := domain.EstadoProteccion{Claves: map[string]domain.ClaveHash{}}
	var lamport, counter int64
	var secuencia *string
	// La copia de la secuencia solo se lee si hay secciones: es lo único que la usa.
	err := tx.QueryRow(ctx, `
		SELECT modelo, hay_registro, lamport, dot_actor, dot_counter,
		       CASE WHEN modelo <> '' THEN secuencia END
		FROM doc_proteccion WHERE document_id = $1`, docID).
		Scan(&est.Modelo, &est.HayRegistro, &lamport, &est.Dot.Actor, &counter, &secuencia)
	if isNoRows(err) {
		return est, false, nil
	}
	if err != nil {
		return est, false, err
	}
	est.Lamport, est.Dot.Counter = uint64(lamport), uint64(counter) //nolint:gosec // los guardó guardarProteccion tras comprobar el rango
	if secuencia != nil {
		if est.Secuencia, err = domain.SecuenciaDeJSON([]byte(*secuencia)); err != nil {
			return est, false, err
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT ambito, algoritmo, sal, vueltas, hash
		FROM doc_proteccion_claves WHERE document_id = $1`, docID)
	if err != nil {
		return est, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var ambito string
		var c domain.ClaveHash
		if err := rows.Scan(&ambito, &c.Algoritmo, &c.Sal, &c.Vueltas, &c.Hash); err != nil {
			return est, false, err
		}
		est.Claves[ambito] = c
	}
	return est, true, rows.Err()
}

// guardarProteccion escribe el estado y aplica los cambios de claves.
func guardarProteccion(ctx context.Context, tx pgx.Tx, tenant, docID string, est domain.EstadoProteccion, poner map[string]domain.ClaveHash, quitar []string) error {
	lamport, err := lamportInt64(est.Lamport)
	if err != nil {
		return err
	}
	counter, err := lamportInt64(est.Dot.Counter)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO doc_proteccion (document_id, tenant, modelo, hay_registro, lamport, dot_actor, dot_counter)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (document_id) DO UPDATE SET
			modelo = EXCLUDED.modelo, hay_registro = EXCLUDED.hay_registro,
			lamport = EXCLUDED.lamport, dot_actor = EXCLUDED.dot_actor,
			dot_counter = EXCLUDED.dot_counter, actualizada = now()`,
		docID, tenant, est.Modelo, est.HayRegistro, lamport, est.Dot.Actor, counter); err != nil {
		return fmt.Errorf("guardar protección: %w", err)
	}
	for _, ambito := range quitar {
		if _, err := tx.Exec(ctx,
			`DELETE FROM doc_proteccion_claves WHERE document_id = $1 AND ambito = $2`, docID, ambito); err != nil {
			return err
		}
	}
	for ambito, c := range poner {
		if _, err := tx.Exec(ctx, `
			INSERT INTO doc_proteccion_claves (document_id, tenant, ambito, algoritmo, sal, vueltas, hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (document_id, ambito) DO UPDATE SET
				algoritmo = EXCLUDED.algoritmo, sal = EXCLUDED.sal,
				vueltas = EXCLUDED.vueltas, hash = EXCLUDED.hash, creada = now()`,
			docID, tenant, ambito, c.Algoritmo, c.Sal, c.Vueltas, c.Hash); err != nil {
			return fmt.Errorf("guardar clave de protección: %w", err)
		}
	}
	return nil
}

// protegerEnTx aplica la regla de protección (hojas: contraseña y rangos; documentos de texto:
// secciones; presentaciones: bloqueos y contraseña) a un lote dentro de la transacción de
// escritura (documento ya bloqueado). Devuelve el resultado a persistir tras aceptar el
// lote (nil si el lote no toca lo protegible) y las operaciones saneadas.
func protegerEnTx(ctx context.Context, tx pgx.Tx, docID string, kind domain.Kind, rol domain.Role, quien domain.Subject, ops []domain.Op) (*domain.ResultadoProteccion, []domain.Op, error) {
	if !domain.ProtegeEscritura(kind, ops) {
		return nil, ops, nil
	}
	est, hay, err := cargarProteccion(ctx, tx, docID)
	if err != nil {
		return nil, nil, err
	}
	if !hay {
		return nil, nil, domain.ErrProteccionPendiente
	}
	p := domain.ProteccionDe(ctx)
	p.Quien = quien // de la sesión, no del cuerpo: es lo que decide los rangos editables
	var res domain.ResultadoProteccion
	switch kind {
	case domain.KindDocument:
		res, err = domain.AplicarSecciones(est, ops, rol.CanManage(), quien)
	case domain.KindPresentation:
		res, err = domain.AplicarDiapos(est, ops, rol.CanManage(), p)
	default:
		res, err = domain.AplicarProteccion(est, ops, rol.CanManage(), p)
	}
	if err != nil {
		return nil, nil, err
	}
	return &res, res.Ops, nil
}
