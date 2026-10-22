// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// FirmaPreparada es la fila que liga los dos pasos de una firma con el certificado
// del sistema (migración 0007, ADR 0051).
type FirmaPreparada struct {
	ID     string
	Sujeto string
	Caduca time.Time
	// Huella es SHA-256 del PDF preparado: la app lo devuelve y tiene que ser ESE.
	Huella       []byte
	HuecoInicio  int
	HuecoFin     int
	Atributos    []byte
	Certificados []byte
	// Reserva es el en_curso_hasta que puso TomarFirmaPreparada: soltar solo quita
	// ESA reserva, no la de otra vuelta que la tomara al caducar la primera.
	Reserva time.Time
}

var (
	// ErrFirmaNoDisponible: la preparación no existe, es de otra persona, ha
	// caducado o la está completando otra petición. Las cuatro se contestan igual a
	// propósito: distinguirlas le diría a quien prueba identificadores cuáles
	// existen.
	ErrFirmaNoDisponible = errors.New("store: la firma preparada ya no está disponible")
	// ErrDemasiadasFirmas: la persona ya tiene demasiadas preparaciones abiertas.
	ErrDemasiadasFirmas = errors.New("store: demasiadas firmas preparadas a la vez")
)

// GuardarFirmaPreparada apunta una preparación y devuelve su identificador.
//
// Antes borra las caducadas de la organización y cuenta las que la persona tiene
// abiertas: cada una aparca una cadena de certificados durante diez minutos, y sin
// tope un bucle de preparaciones llenaría la tabla.
func (s *Store) GuardarFirmaPreparada(ctx context.Context, f FirmaPreparada, maximo int) (string, error) {
	var id string
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM pdf_firmas_preparadas WHERE caduca < now()`); err != nil {
			return err
		}
		var abiertas int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM pdf_firmas_preparadas WHERE sujeto = $1`, f.Sujeto).Scan(&abiertas); err != nil {
			return err
		}
		if abiertas >= maximo {
			return ErrDemasiadasFirmas
		}
		return tx.QueryRow(ctx, `
			INSERT INTO pdf_firmas_preparadas
			    (tenant, sujeto, caduca, huella, hueco_inicio, hueco_fin, atributos, certificados)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6, $7)
			RETURNING id::text`,
			f.Sujeto, f.Caduca, f.Huella, f.HuecoInicio, f.HuecoFin, f.Atributos, f.Certificados,
		).Scan(&id)
	})
	if err != nil && !errors.Is(err, ErrDemasiadasFirmas) {
		return "", fmt.Errorf("store: guardar la firma preparada: %w", err)
	}
	return id, err
}

// TomarFirmaPreparada reserva la preparación `plazo` para completarla, y la
// devuelve. Solo si es de `sujeto`, no ha caducado y nadie la tiene reservada: es
// UNA sentencia, así que dos vueltas a la vez no pueden tomar la misma. `id` llega
// ya validado como UUID por la capa HTTP.
func (s *Store) TomarFirmaPreparada(ctx context.Context, id, sujeto string, plazo time.Duration) (FirmaPreparada, error) {
	f := FirmaPreparada{ID: id, Sujeto: sujeto}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE pdf_firmas_preparadas
			SET en_curso_hasta = now() + make_interval(secs => $3)
			WHERE id = $1::uuid AND sujeto = $2 AND caduca > now()
			  AND (en_curso_hasta IS NULL OR en_curso_hasta < now())
			RETURNING caduca, huella, hueco_inicio, hueco_fin, atributos, certificados, en_curso_hasta`,
			id, sujeto, plazo.Seconds(),
		).Scan(&f.Caduca, &f.Huella, &f.HuecoInicio, &f.HuecoFin, &f.Atributos, &f.Certificados, &f.Reserva)
	})
	if isNoRows(err) {
		return FirmaPreparada{}, ErrFirmaNoDisponible
	}
	if err != nil {
		return FirmaPreparada{}, fmt.Errorf("store: tomar la firma preparada: %w", err)
	}
	return f, nil
}

// SoltarFirmaPreparada quita la reserva: la vuelta no terminó (el sello no llegó,
// la firma no era de esa clave) y la persona puede reintentar sin volver a empezar.
// Solo quita la reserva `reserva`: una vuelta lenta cuya reserva caducó no puede
// soltar la de la siguiente y dejar que una tercera la tome a la vez.
func (s *Store) SoltarFirmaPreparada(ctx context.Context, id string, reserva time.Time) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE pdf_firmas_preparadas SET en_curso_hasta = NULL
			WHERE id = $1::uuid AND en_curso_hasta = $2`, id, reserva)
		return err
	})
}

// GastarFirmaPreparada la borra: ya hay firma, y el mismo resumen no vale dos veces.
func (s *Store) GastarFirmaPreparada(ctx context.Context, id string) error {
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM pdf_firmas_preparadas WHERE id = $1::uuid`, id)
		return err
	})
}

// PurgarFirmasCaducadas borra las caducadas de TODAS las organizaciones. Va en
// una transacción de mantenimiento, que la política de la 0007 solo deja borrar.
func (s *Store) PurgarFirmasCaducadas(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM pdf_firmas_preparadas WHERE caduca < now()`)
		n = tag.RowsAffected()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("store: purgar las firmas preparadas: %w", err)
	}
	return n, nil
}
