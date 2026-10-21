// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// APROBACIONES CON UMBRAL Y CUATRO OJOS (migración 0095).
//
// Los verbos (0092) dicen quién puede asentar. Esto dice que, a partir de
// cierto importe, no basta con poder: hace falta que lo mire otra persona.
//
// El dinero va en CÉNTIMOS ENTEROS y siempre con su divisa al lado. Si la
// divisa del documento no es la del umbral, NO se convierte —no hay fuente de
// tipos decidida— y se pide aprobación igual: el lado seguro.

var (
	// ErrRequiereAprobacion es un documento por encima del umbral sin una
	// aprobación viva.
	ErrRequiereAprobacion = errors.New("este importe necesita la aprobación de otra persona")
	// ErrCuatroOjos es intentar aprobar lo propio.
	ErrCuatroOjos = errors.New("no puedes aprobar un documento que creaste o cuya aprobación pediste")
	// ErrAprobacionResuelta es aprobar o rechazar algo que ya se resolvió.
	ErrAprobacionResuelta = errors.New("esa aprobación ya está resuelta")
)

// ImporteDelAsiento es lo que «vale» un documento a efectos de umbral: la suma
// del debe, que en partida doble es igual a la del haber y es el importe total
// del documento. En CÉNTIMOS enteros, como todo el dinero de este ERP.
//
// Se calcula del asiento y no de un campo del documento a propósito: el asiento
// es lo que va al libro, así que es el único importe que no se puede esquivar
// poniendo la cifra en otro campo.
func ImporteDelAsiento(a domain.Asiento) int64 {
	var total int64
	for _, ap := range a.Apuntes {
		total += ap.Debe
	}
	return total
}

// Umbral es el importe a partir del cual un módulo pide segunda persona.
type Umbral struct {
	Modulo  string `json:"modulo"`
	Importe int64  `json:"importe"` // céntimos
	Moneda  string `json:"moneda"`
}

// Umbrales son los umbrales puestos, por módulo.
func (s *Store) Umbrales(ctx context.Context) ([]Umbral, error) {
	out := make([]Umbral, 0, 8)
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT modulo, importe, moneda FROM umbral_aprobacion WHERE tenant = current_setting('app.tenant', true) ORDER BY modulo`)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var u Umbral
			if err := filas.Scan(&u.Modulo, &u.Importe, &u.Moneda); err != nil {
				return err
			}
			out = append(out, u)
		}
		return filas.Err()
	})
	return out, err
}

// GuardarUmbral pone o quita (importe < 0) el umbral de un módulo.
func (s *Store) GuardarUmbral(ctx context.Context, modulo string, importe int64, moneda, por string) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if importe < 0 {
			_, err := tx.Exec(ctx, `DELETE FROM umbral_aprobacion WHERE tenant = current_setting('app.tenant', true) AND modulo = $1`, modulo)
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO umbral_aprobacion (tenant, modulo, importe, moneda, actualizado_por)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4)
			ON CONFLICT (tenant, modulo) DO UPDATE
			   SET importe = EXCLUDED.importe, moneda = EXCLUDED.moneda,
			       actualizado_por = EXCLUDED.actualizado_por, actualizado_en = now()`,
			modulo, importe, moneda, por)
		return err
	})
}

// exigirAprobacion decide si un documento puede pasar al libro.
//
// Va DENTRO de la transacción que asienta, como el período: entre leer el
// umbral fuera y escribir cabe que alguien lo cambie.
//
// La aprobación vale para ESTE importe y ESTA divisa. Si el documento cambió
// después de aprobarse, lo aprobado ya no es lo que hay y vuelve a hacer falta
// una vuelta — que es justo lo que evita el truco de aprobar barato y editar.
func exigirAprobacion(ctx context.Context, tx pgx.Tx, m *catalogo.Modelo, registroID string, importe int64, moneda, huella string) error {
	var umbral int64
	var monedaUmbral string
	err := tx.QueryRow(ctx, `
		SELECT importe, moneda FROM umbral_aprobacion WHERE tenant = current_setting('app.tenant', true) AND modulo = $1`, m.Modulo).
		Scan(&umbral, &monedaUmbral)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // sin umbral, nada que aprobar
	}
	if err != nil {
		return err
	}
	// Divisas distintas: no se convierte (no hay fuente de tipos decidida) y no
	// se deja pasar. Un umbral que se salta por escribir el importe en otra
	// moneda no es un umbral.
	distintaMoneda := moneda != monedaUmbral
	if !distintaMoneda && importe < umbral {
		return nil
	}

	var aprobado int64
	var monedaAprobada, huellaAprobada string
	err = tx.QueryRow(ctx, `
		SELECT importe, moneda, huella FROM aprobacion
		 WHERE tenant = current_setting('app.tenant', true) AND registro_id = $1::uuid AND estado = 'aprobada'
		 ORDER BY resuelta_en DESC LIMIT 1`, registroID).Scan(&aprobado, &monedaAprobada, &huellaAprobada)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequiereAprobacion
	}
	if err != nil {
		return err
	}
	// El importe solo no basta: con el mismo total se cambiaba el cliente, la
	// cuenta o las líneas después de aprobar (NEG-13). La huella es la del
	// documento entero; una aprobación de antes de la 0101 no la tiene y se pide
	// otra vez.
	if aprobado != importe || monedaAprobada != moneda || huellaAprobada == "" || huellaAprobada != huella {
		return ErrRequiereAprobacion
	}
	return nil
}

// huellaDe resume el contenido de un documento para atarle su aprobación.
// json.Marshal ordena las claves de un mapa, así que la misma data da siempre
// la misma huella.
func huellaDe(data map[string]any) string {
	crudo, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	suma := sha256.Sum256(crudo)
	return hex.EncodeToString(suma[:])
}

// exigirAprobacionAlEmitir aplica el umbral a los documentos que se emiten
// cambiando su estado en vez de asentándolos: es la misma factura y vale lo
// mismo, así que pide la misma segunda persona (NEG-11).
func exigirAprobacionAlEmitir(ctx context.Context, tx pgx.Tx, m *catalogo.Modelo, ids []string, estado string) error {
	if m.Contable == nil || (estado != "confirmado" && estado != "pagado") {
		return nil
	}
	var hay bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM umbral_aprobacion WHERE tenant = current_setting('app.tenant', true) AND modulo = $1)`,
		m.Modulo).Scan(&hay); err != nil || !hay {
		return err
	}
	for _, id := range ids {
		var r Registro
		err := escanearRegistro(tx.QueryRow(ctx, `
			SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at
			  FROM records
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL
			 FOR UPDATE`, id, m.Clave), &r)
		if errors.Is(err, ErrNoEncontrado) {
			continue // el UPDATE no lo tocará
		}
		if err != nil {
			return err
		}
		if r.Estado == "confirmado" || r.Estado == "pagado" {
			continue // ya emitido: pasar de confirmado a pagado no emite nada nuevo
		}
		etiqueta := r.Etiqueta
		if etiqueta == "" {
			etiqueta = r.Ref
		}
		a, err := domain.AsientoDe(m, etiqueta, r.Data)
		if err != nil {
			return err
		}
		if err := exigirAprobacion(ctx, tx, m, id, ImporteDelAsiento(a), a.Moneda, huellaDe(r.Data)); err != nil {
			return err
		}
	}
	return nil
}

// Aprobacion es una solicitud tal como sale a la API.
type Aprobacion struct {
	ID            string     `json:"id"`
	Modelo        string     `json:"modelo"`
	RegistroID    string     `json:"registro_id"`
	Importe       int64      `json:"importe"`
	Moneda        string     `json:"moneda"`
	Estado        string     `json:"estado"`
	SolicitadaPor string     `json:"solicitada_por"`
	SolicitadaEn  time.Time  `json:"solicitada_en"`
	CreadaPor     string     `json:"creada_por,omitempty"`
	ResueltaPor   string     `json:"resuelta_por,omitempty"`
	ResueltaEn    *time.Time `json:"resuelta_en,omitempty"`
	Motivo        string     `json:"motivo,omitempty"`
}

// PedirAprobacion abre la solicitud de un documento, con su importe y su divisa
// congelados. El importe lo calcula quien llama a partir del documento (es el
// mismo que iría al libro), no lo manda el cliente: un importe de fuera sería
// la forma más fácil de pasar por debajo del umbral.
func (s *Store) PedirAprobacion(ctx context.Context, m *catalogo.Modelo, registroID string, importe int64, moneda, por string) (Aprobacion, error) {
	var a Aprobacion
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		var dueno string
		var data map[string]any
		if err := tx.QueryRow(ctx, `
			SELECT coalesce(owner_sub, ''), data FROM records
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL`,
			registroID, m.Clave).Scan(&dueno, &data); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoEncontrado
			}
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO aprobacion (tenant, modelo, registro_id, importe, moneda,
			                        solicitada_por, creada_por, huella)
			VALUES (current_setting('app.tenant', true), $1, $2::uuid, $3, $4, $5, $6, $7)
			ON CONFLICT (tenant, registro_id) WHERE estado = 'pendiente'
			DO UPDATE SET importe = EXCLUDED.importe, moneda = EXCLUDED.moneda, huella = EXCLUDED.huella
			RETURNING id::text, modelo, registro_id::text, importe, moneda, estado,
			          solicitada_por, solicitada_en, creada_por`,
			m.Clave, registroID, importe, moneda, por, dueno, huellaDe(data)).
			Scan(&a.ID, &a.Modelo, &a.RegistroID, &a.Importe, &a.Moneda, &a.Estado,
				&a.SolicitadaPor, &a.SolicitadaEn, &a.CreadaPor)
	})
	return a, err
}

// ResolverAprobacion aprueba o rechaza, con CUATRO OJOS: quien resuelve no
// puede ser quien creó el documento ni quien pidió la aprobación.
//
// Es incondicional y no depende de ningún ajuste: una aprobación que puede
// firmarse a sí misma no aprueba nada. Ni siquiera el owner de la organización
// se la salta — el rol dice qué puedes hacer, no que puedas ser dos personas.
func (s *Store) ResolverAprobacion(ctx context.Context, id, estado, motivo, por string) (Aprobacion, error) {
	var a Aprobacion
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		var solicitante, dueno, actual string
		if err := tx.QueryRow(ctx, `
			SELECT solicitada_por, creada_por, estado FROM aprobacion
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid FOR UPDATE`, id).Scan(&solicitante, &dueno, &actual); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoEncontrado
			}
			return err
		}
		if actual != "pendiente" {
			return ErrAprobacionResuelta
		}
		if por == "" || por == solicitante || por == dueno {
			return ErrCuatroOjos
		}
		return tx.QueryRow(ctx, `
			UPDATE aprobacion
			   SET estado = $2, resuelta_por = $3, resuelta_en = now(), motivo = $4
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid
			RETURNING id::text, modelo, registro_id::text, importe, moneda, estado,
			          solicitada_por, solicitada_en, creada_por, resuelta_por, resuelta_en, motivo`,
			id, estado, por, motivo).
			Scan(&a.ID, &a.Modelo, &a.RegistroID, &a.Importe, &a.Moneda, &a.Estado,
				&a.SolicitadaPor, &a.SolicitadaEn, &a.CreadaPor, &a.ResueltaPor, &a.ResueltaEn, &a.Motivo)
	})
	return a, err
}

// Aprobaciones lista las solicitudes de un estado ("" = todas), las más
// recientes primero. Tope de 200.
func (s *Store) Aprobaciones(ctx context.Context, estado string) ([]Aprobacion, error) {
	out := make([]Aprobacion, 0, 16)
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		sql := `SELECT id::text, modelo, registro_id::text, importe, moneda, estado,
			solicitada_por, solicitada_en, creada_por, resuelta_por, resuelta_en, motivo
			FROM aprobacion
			WHERE tenant = current_setting('app.tenant', true)`
		args := []any{}
		if estado != "" {
			sql += ` AND estado = $1`
			args = append(args, estado)
		}
		sql += ` ORDER BY solicitada_en DESC LIMIT 200`
		filas, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var a Aprobacion
			if err := filas.Scan(&a.ID, &a.Modelo, &a.RegistroID, &a.Importe, &a.Moneda, &a.Estado,
				&a.SolicitadaPor, &a.SolicitadaEn, &a.CreadaPor, &a.ResueltaPor, &a.ResueltaEn, &a.Motivo); err != nil {
				return err
			}
			out = append(out, a)
		}
		return filas.Err()
	})
	return out, err
}
