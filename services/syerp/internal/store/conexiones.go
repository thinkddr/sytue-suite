// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// Conexion es la configuración de un tercero para una organización. El secreto
// NO sale nunca de aquí en claro: `Secreto` solo se rellena en la lectura
// interna que verifica un webhook.
type Conexion struct {
	Proveedor  string         `json:"proveedor"`
	Instancia  string         `json:"instancia,omitempty"`
	TieneFirma bool           `json:"tiene_firma"`
	Config     map[string]any `json:"config"`
	Activo     bool           `json:"activo"`
	Secreto    string         `json:"-"`
}

// GuardarConexion crea o actualiza la conexión a un proveedor. El secreto se
// sella antes de tocar la base; si llega vacío, se conserva el que hubiera (así
// se puede cambiar la instancia sin volver a pegar la clave de firma).
func (s *Store) GuardarConexion(ctx context.Context, c Conexion, cifra *keycrypto.Cipher, sub string) error {
	sellado := ""
	if c.Secreto != "" {
		if !cifra.Enabled() {
			return fmt.Errorf("no hay clave de sellado configurada: %w", domain.ErrDatos)
		}
		var err error
		if sellado, err = cifra.Encrypt(c.Secreto); err != nil {
			return fmt.Errorf("sellar secreto: %w", err)
		}
	}
	cfg, err := json.Marshal(c.Config)
	if err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO conexiones (tenant, proveedor, instancia, secreto_enc, config, activo, creado_por)
			VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6)
			ON CONFLICT (tenant, proveedor) DO UPDATE SET
				instancia   = EXCLUDED.instancia,
				secreto_enc = CASE WHEN EXCLUDED.secreto_enc <> '' THEN EXCLUDED.secreto_enc
				                   ELSE conexiones.secreto_enc END,
				config      = EXCLUDED.config,
				activo      = EXCLUDED.activo,
				updated_at  = now()`,
			c.Proveedor, c.Instancia, sellado, cfg, c.Activo, sub)
		return err
	})
}

// Conexion lee la conexión a un proveedor. Devuelve el secreto ya ABIERTO porque
// el único que llama es la verificación del webhook; no hay ruta que lo exponga.
func (s *Store) Conexion(ctx context.Context, proveedor string, cifra *keycrypto.Cipher) (Conexion, error) {
	var c Conexion
	var sellado string
	var cfg []byte
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT proveedor, instancia, secreto_enc, config, activo
			  FROM conexiones WHERE tenant = current_setting('app.tenant', true) AND proveedor = $1`, proveedor).
			Scan(&c.Proveedor, &c.Instancia, &sellado, &cfg, &c.Activo)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoEncontrado
		}
		return err
	})
	if err != nil {
		return Conexion{}, err
	}
	if err := json.Unmarshal(cfg, &c.Config); err != nil {
		return Conexion{}, err
	}
	c.TieneFirma = sellado != ""
	if sellado != "" && cifra.Enabled() {
		if c.Secreto, err = cifra.Decrypt(sellado); err != nil {
			return Conexion{}, fmt.Errorf("abrir secreto de %s: %w", proveedor, err)
		}
	}
	return c, nil
}

// ListarConexiones devuelve las conexiones de la organización, SIN secretos.
func (s *Store) ListarConexiones(ctx context.Context) ([]Conexion, error) {
	var out []Conexion
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT proveedor, instancia, secreto_enc <> '', config, activo
			  FROM conexiones
			 WHERE tenant = current_setting('app.tenant', true)
			 ORDER BY proveedor`)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var c Conexion
			var cfg []byte
			if err := filas.Scan(&c.Proveedor, &c.Instancia, &c.TieneFirma, &cfg, &c.Activo); err != nil {
				return err
			}
			if err := json.Unmarshal(cfg, &c.Config); err != nil {
				return err
			}
			out = append(out, c)
		}
		return filas.Err()
	})
	return out, err
}

// MarcarEvento registra un evento entrante. Devuelve false si YA estaba: es el
// control de «una sola vez» frente a los reintentos del proveedor, y lo garantiza
// la clave primaria, no este código.
func (s *Store) MarcarEvento(ctx context.Context, proveedor, eventoID, tipo string) (bool, error) {
	nuevo := false
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		etiq, err := tx.Exec(ctx, `
			INSERT INTO eventos_entrantes (tenant, proveedor, evento_id, tipo)
			VALUES (current_setting('app.tenant', true), $1, $2, $3)
			ON CONFLICT DO NOTHING`, proveedor, eventoID, tipo)
		if err != nil {
			return err
		}
		nuevo = etiq.RowsAffected() == 1
		return nil
	})
	return nuevo, err
}

// UpsertPorExterno crea o actualiza un registro identificado por el id que tiene
// en el sistema de origen (`externo_id`). Es lo que hace que importar dos veces
// no duplique: la segunda pasada reconoce lo que ya trajo la primera.
func (s *Store) UpsertPorExterno(ctx context.Context, m *catalogo.Modelo, externoID string,
	data map[string]any, etiqueta, estado, ownerSub string,
) (creado bool, err error) {
	err = s.escritura(ctx, func(tx pgx.Tx) error {
		if err := bloquearOrigenCompra(ctx, tx, m.Clave, externoID); err != nil {
			return err
		}
		crudo, err := json.Marshal(data)
		if err != nil {
			return err
		}
		filtro, err := json.Marshal(map[string]string{"externo_id": externoID})
		if err != nil {
			return err
		}
		var id string
		err = tx.QueryRow(ctx, `
			SELECT id::text FROM records
			 WHERE tenant = current_setting('app.tenant', true) AND model = $1 AND data @> $2::jsonb AND deleted_at IS NULL
			 LIMIT 1`, m.Clave, filtro).Scan(&id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			ref := ""
			if m.Secuencia != "" {
				if ref, err = reservarRef(ctx, tx, "doc:"+m.Clave, m.Secuencia, periodoDe(data)); err != nil {
					return err
				}
			}
			if estado == "" {
				estado = domain.EstadoInicial(m)
			}
			creado = true
			_, err = tx.Exec(ctx, `
				INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
				VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, $5, $6)`,
				m.Clave, ref, etiqueta, estado, crudo, ownerSub)
			return err
		case err != nil:
			return err
		default:
			// Actualizar NO pisa el estado si el origen no trae ninguno: el flujo
			// local puede haber avanzado más que el del sistema de origen.
			_, err = tx.Exec(ctx, `
				UPDATE records
				   SET data = $2,
				       etiqueta = CASE WHEN $3 <> '' THEN $3 ELSE etiqueta END,
				       estado = CASE WHEN $4 <> '' THEN $4 ELSE estado END,
				       updated_at = now()
				 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, id, crudo, etiqueta, estado)
			return err
		}
	})
	return creado, err
}

// BuscarPorExterno localiza un documento por su identificador en el tercero.
func (s *Store) BuscarPorExterno(ctx context.Context, modelo, externoID string) (Registro, error) {
	var r Registro
	filtro, err := json.Marshal(map[string]string{"externo_id": externoID})
	if err != nil {
		return r, err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return escanearRegistro(tx.QueryRow(ctx, `
			SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at
			  FROM records
			 WHERE tenant = current_setting('app.tenant', true) AND model = $1 AND data @> $2::jsonb AND deleted_at IS NULL
			 LIMIT 1`, modelo, filtro), &r)
	})
	return r, err
}

// BuscarPorRef localiza un documento por su referencia (FV-2026-00001).
func (s *Store) BuscarPorRef(ctx context.Context, modelo, ref string) (Registro, error) {
	var r Registro
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return escanearRegistro(tx.QueryRow(ctx, `
			SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at
			  FROM records
			 WHERE tenant = current_setting('app.tenant', true) AND model = $1 AND ref = $2 AND deleted_at IS NULL`, modelo, ref), &r)
	})
	return r, err
}

var (
	// ErrEventoRepetido es un evento del proveedor que ya se procesó.
	ErrEventoRepetido = errors.New("evento ya procesado")
	// ErrImporteNoCasa es un cobro por un importe distinto del de la factura.
	ErrImporteNoCasa = errors.New("el importe cobrado no es el de la factura")
)

// RegistrarCobro marca el documento como pagado y escribe el asiento del cobro:
// entra dinero en el banco y deja de deberlo el cliente.
//
// Es un asiento aparte del de la factura, como debe ser: facturar y cobrar son
// dos hechos con dos fechas, y meterlos en el mismo apunte hace imposible saber
// qué está pendiente de cobro.
//
// El evento de Stripe se anota en la MISMA transacción (NEG-25): anotado antes y
// aparte, un cobro que fallaba al escribirse se perdía, porque el reintento de
// Stripe ya se daba por repetido. Y solo se cobra una factura abierta y por su
// importe: una ya pagada o anulada no recibe un segundo asiento, y un importe que
// no casa no la da por pagada entera.
func (s *Store) RegistrarCobro(ctx context.Context, documentoID, eventoID, tipoEvento, cuentaBanco, cuentaCliente,
	diario, concepto string, centimos int64, fecha time.Time, ownerSub string,
) (Asiento, error) {
	var out Asiento
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		etiq, err := tx.Exec(ctx, `
			INSERT INTO eventos_entrantes (tenant, proveedor, evento_id, tipo)
			VALUES (current_setting('app.tenant', true), 'stripe', $1, $2)
			ON CONFLICT DO NOTHING`, eventoID, tipoEvento)
		if err != nil {
			return err
		}
		if etiq.RowsAffected() == 0 {
			return ErrEventoRepetido
		}
		// El tercero del apunte es el cliente de la factura, para que el mayor de
		// clientes cuadre con lo que cada uno debe.
		var clienteID, estado string
		var total int64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(data->>'cliente', ''), estado, `+centimosSQL("data->'total'")+`
			  FROM records WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid
			   AND model = 'factura_venta' AND deleted_at IS NULL
			 FOR UPDATE`, documentoID).Scan(&clienteID, &estado, &total); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoEncontrado
			}
			return err
		}
		if estado != "borrador" && estado != "confirmado" {
			return ErrFacturaNoAbierta
		}
		if centimos != total {
			return ErrImporteNoCasa
		}
		// Banco al debe, cliente al haber: el cobro cancela la deuda. Sin origen:
		// el origen de la factura es su propio asiento, y el índice único por
		// documento no admite dos.
		out, err = escribirAsiento(ctx, tx, diario, fecha, concepto, "", ownerSub, []Apunte{
			{Cuenta: cuentaBanco, Concepto: concepto, Debe: centimos},
			{Cuenta: cuentaCliente, Concepto: concepto, Haber: centimos, Tercero: clienteID},
		})
		if err != nil {
			return err
		}
		if err := s.emitirVerifactu(ctx, tx, "factura_venta", []string{documentoID}, "pagado", ownerSub); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE records SET estado = 'pagado', updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, documentoID); err != nil {
			return err
		}
		out.OrigenID = documentoID
		return nil
	})
	return out, err
}

// BuscarPorNIF localiza un registro por su campo `nif`, sin distinguir
// mayúsculas, espacios, guiones ni el prefijo de país del NIF-IVA: la factura
// electrónica de un proveedor trae «ESB12345678» y la ficha, «B-12345678».
// Devuelve el más antiguo si hay varios, como BuscarPorEtiqueta.
func (s *Store) BuscarPorNIF(ctx context.Context, modelo, nif string) (string, error) {
	var id string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id::text FROM (
			  SELECT id, created_at, upper(regexp_replace(data->>'nif', '[^A-Za-z0-9]', '', 'g')) AS nif
			    FROM records
			   WHERE tenant = current_setting('app.tenant', true)
			     AND model = $1 AND data ? 'nif' AND deleted_at IS NULL
			) c
			 WHERE c.nif = ANY($2) OR (c.nif ~ '^[A-Z]{2}' AND substr(c.nif, 3) = ANY($2))
			 ORDER BY created_at
			 LIMIT 1`, modelo, domain.VariantesNIF(nif)).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoEncontrado
	}
	return id, err
}

// BuscarPorEtiqueta localiza un registro por su nombre legible, sin distinguir
// mayúsculas. Es lo que permite que un CSV traiga «Cliente: ACME» en vez de un
// identificador que nadie tiene fuera de este ERP. Devuelve el más antiguo si
// hay varios: es determinista, y dos contactos con el mismo nombre son un
// problema de datos que la importación no puede resolver.
func (s *Store) BuscarPorEtiqueta(ctx context.Context, modelo, etiqueta string) (string, error) {
	var id string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id::text FROM records
			 WHERE tenant = current_setting('app.tenant', true)
			   AND model = $1 AND lower(etiqueta) = lower($2) AND deleted_at IS NULL
			 ORDER BY created_at
			 LIMIT 1`, modelo, etiqueta).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoEncontrado
	}
	return id, err
}
