// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// Entrada privada de avisos de One (ADR 0086, migración 0014).
//
// Con la fuente de una persona en `legacy`, un aviso va a `notifications` como
// siempre. Con la fuente congelada o ya privada, notify.Create lo manda aquí:
//
//  1. Se guarda su copia en `hub_private_notices`, en la misma transacción y bajo el
//     mismo cerrojo de persona que los topes. Desde ese momento el aviso existe y no
//     se pierde: ni por falta de clave, ni por soltar la fuente, ni por un fallo al
//     cifrar.
//  2. Si la persona ya dio de alta una clave de entrada para esa época, se cifra
//     para ella en la cola compartida (private_ingress_tasks + bodies), con el mismo
//     sellado `age` que Calendar, y suena una señal SIN contenido para que sus
//     aparatos la reclamen. Si todavía no hay clave, se queda en la copia: se cifra
//     en cuanto un aparato da de alta una (PrivateIngressQueue).
//  3. Un aparato reclama la entrega, la guarda en su almacén privado
//     (apps/nucleo/hub.IngressExecutor) y la confirma. En la misma transacción de
//     esa confirmación se borra la copia: desde ahí el aviso solo existe cifrado.
//  4. Si la persona suelta la fuente antes de que ningún aparato lo confirme, la
//     copia vuelve a `notifications` (ReleasePrivateStorage): lo que estaba cifrado
//     para una clave que esa persona ya no abre no se queda sin leer.
//
// Lo que nunca hace: tirar un aviso. Los de seguridad (security.*) siguen este
// mismo camino, sin excepción ni atajo.

// TipoEntradaAviso es el tipo de entrega de la entrada compartida para un aviso.
const TipoEntradaAviso = "hub.receive"

// AvisoCifrado es lo que viaja cifrado en los metadatos de la entrega (el cuerpo va
// vacío): el aviso entero, con su organización para atarlo al ámbito.
type AvisoCifrado struct {
	Version   int               `json:"v"`
	Tenant    string            `json:"tenant"`
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Type      string            `json:"type"`
	Level     string            `json:"level"`
	Title     string            `json:"title"`
	Body      string            `json:"body,omitempty"`
	URL       string            `json:"url,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// ErrEntradaPrivada: la entrada privada del Hub no tiene autoridad de Rooms
// configurada (sin ella no hay altas ni recibos verificables).
var ErrEntradaPrivada = errors.New("hub: a verified device ingress bridge is required")

// puedeSerDuena es lo mismo que la guarda de la 0015: un buzón por correo, un
// subject vacío o demasiado largo no tienen fuente privada.
func puedeSerDuena(subject string) bool {
	return subject != "" && len(subject) <= 1024 && !strings.HasPrefix(subject, "email:")
}

// AdmitirAvisoPrivado decide, dentro de la transacción de notify.Create y después de
// los topes, si el aviso va a la entrada privada. Devuelve false si la fuente de su
// destinatario es `legacy` (o no puede tener fuente): entonces el llamador lo inserta
// en `notifications` como siempre, en la misma transacción.
//
// La fila de estado se crea (legacy) y se lee FOR SHARE: la congelación la toma FOR
// UPDATE, así que no hay carrera entre decidir y escribir.
func AdmitirAvisoPrivado(ctx context.Context, tx pgx.Tx, tenant string, rec domain.NewNotification) (domain.Notification, bool, error) {
	if !puedeSerDuena(rec.Subject) {
		return domain.Notification{}, false, nil
	}
	scope := storage.Scope{Tenant: tenant, Subject: rec.Subject, Product: "hub"}
	if !scope.Valid() {
		return domain.Notification{}, false, nil
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, rec.Subject); err != nil {
		return domain.Notification{}, false, err
	}
	if err := pg.EnsurePrivateStorageState(ctx, tx, privateHubStateTable, tenant, rec.Subject, nil); err != nil {
		return domain.Notification{}, false, err
	}
	var mode string
	var epoch *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT mode, epoch FROM hub_private_storage_states WHERE tenant=$1 AND owner_sub=$2 FOR SHARE`,
		tenant, rec.Subject).Scan(&mode, &epoch); err != nil {
		return domain.Notification{}, false, err
	}
	if mode == "legacy" {
		return domain.Notification{}, false, nil
	}
	if epoch == nil || *epoch == uuid.Nil {
		return domain.Notification{}, false, ErrPrivateStorageEpoch
	}
	params := rec.Params
	if params == nil {
		params = map[string]string{}
	}
	n := domain.Notification{
		ID: uuid.New(), Tenant: tenant, Subject: rec.Subject, Source: rec.Source, Type: rec.Type,
		Level: rec.Level, Title: rec.Title, Body: rec.Body, URL: rec.URL, Params: params,
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO hub_private_notices (id, tenant, subject, epoch, source, type, level, title, body, url, params)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING created_at`,
		n.ID, tenant, rec.Subject, *epoch, string(rec.Source), rec.Type, string(rec.Level),
		rec.Title, rec.Body, rec.URL, params).Scan(&n.CreatedAt); err != nil {
		return domain.Notification{}, false, err
	}
	sellarPendientes(ctx, tx, scope, *epoch)
	return n, true, nil
}

// sellarPendientes cifra, en un punto de guardado, las copias aún sin cifrar de esa
// época si hay una clave de entrada activa. Si falla, se deshace solo el punto de
// guardado: el aviso sigue en la copia y se reintenta en la próxima operación de la
// cola o el próximo aviso. Nunca tumba la transacción de quien lo llama.
func sellarPendientes(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		log.From(ctx).Warn("hub: entrada privada, punto de guardado", "error", err)
		return
	}
	if _, err := sellarPendientesTx(ctx, sp, scope, epoch); err != nil {
		_ = sp.Rollback(ctx)
		log.From(ctx).Warn("hub: entrada privada, cifrar avisos pendientes (se quedan en la copia)", "error", err)
		return
	}
	if err := sp.Commit(ctx); err != nil {
		log.From(ctx).Warn("hub: entrada privada, cerrar punto de guardado", "error", err)
	}
}

func avisoConfig(scope storage.Scope, epoch uuid.UUID) pg.PrivateIngressConfig {
	return pg.PrivateIngressConfig{Scope: scope, Epoch: epoch, Sealed: func(ctx context.Context, tx pgx.Tx, _ string) error {
		return SenalEntradaTx(ctx, tx, scope.Tenant, scope.Subject)
	}}
}

// sellarPendientesTx devuelve cuántos avisos cifró. Sin clave activa, cero.
func sellarPendientesTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID) (int, error) {
	cfg := avisoConfig(scope, epoch)
	if err := pg.BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return 0, err
	}
	var generation int64
	err := tx.QueryRow(ctx, `SELECT key_generation FROM private_ingress_key_plans
		WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND revoked_at IS NULL
		ORDER BY key_generation DESC LIMIT 1 FOR SHARE`, scope.Tenant, scope.Subject, scope.Product, epoch).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	plan, err := pg.ReadPrivateIngressKey(ctx, tx, cfg, generation)
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT id, source, type, level, title, body, url, params, created_at
		FROM hub_private_notices WHERE tenant=$1 AND subject=$2 AND epoch=$3 AND sealed_at IS NULL
		ORDER BY created_at, id FOR UPDATE`, scope.Tenant, scope.Subject, epoch)
	if err != nil {
		return 0, err
	}
	var pendientes []AvisoCifrado
	for rows.Next() {
		a := AvisoCifrado{Version: 1, Tenant: scope.Tenant}
		var id uuid.UUID
		if err := rows.Scan(&id, &a.Source, &a.Type, &a.Level, &a.Title, &a.Body, &a.URL, &a.Params, &a.CreatedAt); err != nil {
			rows.Close()
			return 0, err
		}
		a.ID = id.String()
		a.CreatedAt = a.CreatedAt.UTC()
		pendientes = append(pendientes, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, a := range pendientes {
		metadata, err := json.Marshal(a)
		if err != nil {
			return 0, err
		}
		if err := pg.PlanPrivateIngress(ctx, tx, cfg, plan, a.ID, TipoEntradaAviso); err != nil {
			return 0, err
		}
		if _, _, err := pg.SealPrivateIngressInline(ctx, tx, cfg, plan, a.ID, TipoEntradaAviso, metadata, bytes.NewReader(nil), 0, 1); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE hub_private_notices SET sealed_at = now() WHERE tenant = $1 AND subject = $2 AND id = $3`, scope.Tenant, scope.Subject, a.ID); err != nil {
			return 0, err
		}
	}
	return len(pendientes), nil
}

// borrarConfirmadosTx borra la copia de los avisos cuya entrega ya confirmó un
// aparato: desde ese recibo solo existen cifrados en su almacén privado.
func borrarConfirmadosTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID) error {
	if err := pg.BindPrivateIngressScope(ctx, tx, avisoConfig(scope, epoch)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM hub_private_notices h
		WHERE h.tenant=$1 AND h.subject=$2 AND h.epoch=$3 AND h.sealed_at IS NOT NULL
		  AND EXISTS (SELECT 1 FROM private_ingress_tasks t
		               WHERE t.tenant=h.tenant AND t.owner_sub=h.subject AND t.product='hub'
		                 AND t.epoch=h.epoch AND t.id=h.id::text AND t.acknowledged_at IS NOT NULL)`,
		scope.Tenant, scope.Subject, epoch)
	return err
}

// devolverAvisosTx es la otra mitad de «soltar la fuente»: con la fuente ya en
// legacy (misma transacción), las copias aún sin confirmar vuelven a
// `notifications` con su id y su fecha. Lo que ya confirmó un aparato vive en su
// almacén privado, que soltar la fuente descarta por definición.
func devolverAvisosTx(ctx context.Context, tx pgx.Tx, tenant, subject string) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO notifications (id, tenant, subject, source, type, level, title, body, url, params, created_at, read, read_at)
		 SELECT id, tenant, subject, source, type, level, title, body, url, params, created_at, read_at IS NOT NULL, read_at
		   FROM hub_private_notices WHERE tenant = $1 AND subject = $2
		 ON CONFLICT (id) DO NOTHING`, tenant, subject); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM hub_private_notices WHERE tenant = $1 AND subject = $2`, tenant, subject)
	return err
}

// SenalEntradaTx avisa a las pantallas abiertas de la persona de que tiene entregas
// que reclamar, SIN contenido: viaja por el mismo NOTIFY que los avisos (el sobre de
// notify.wireEnvelope, como evento de estado `private_ingress`).
func SenalEntradaTx(ctx context.Context, tx pgx.Tx, tenant, subject string) error {
	payload, err := json.Marshal(struct {
		Tenant  string          `json:"tenant"`
		Subject string          `json:"subject"`
		Event   string          `json:"event"`
		Data    json.RawMessage `json:"data"`
	}{tenant, subject, "private_ingress", json.RawMessage(`{}`)})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT pg_notify('sytue_hub_notifications', $1)`, string(payload))
	return err
}

// PrivateIngressQueue es la cola de entrada privada de avisos para los aparatos de la
// persona: las mismas operaciones (alta, reclamo, cuerpo, recibo, relevo) que Mail y
// Calendar, sobre su autoridad de Rooms. Cada operación, además, cifra las copias
// pendientes si ya hay clave y borra las ya confirmadas.
func (s *Store) PrivateIngressQueue(authority callaccess.PrivateIngressAuthority) pg.PrivateIngressQueue {
	return pg.PrivateIngressQueue{Transaction: func(ctx context.Context, scope storage.Scope, epoch, kind string, operation func(pgx.Tx, pg.AuthorizedPrivateIngressConfig) error) error {
		if authority == nil {
			return ErrEntradaPrivada
		}
		if scope.Product != "hub" || !storage.MigrationEpoch(epoch) {
			return pg.ErrPrivateStorageScope
		}
		parsed, err := uuid.Parse(epoch)
		if err != nil {
			return err
		}
		return s.privateHubTx(ctx, scope.Subject, func(own storage.Scope, tx pgx.Tx) error {
			if own != scope {
				return pg.ErrPrivateStorageScope
			}
			activa, err := hubIngressEpoch(ctx, tx, scope, parsed, kind)
			if err != nil {
				return err
			}
			cfg := pg.PrivateIngressConfig{Scope: scope, Epoch: parsed, VerifyReceipt: func(ctx context.Context, claim ingress.Claim, receipt ingress.Receipt) error {
				return authority.VerifyStorageReceipt(ctx, scope, claim.Target, receipt.Commit)
			}}
			if err := operation(tx, pg.AuthorizedPrivateIngressConfig{PrivateIngressConfig: cfg, Authority: authority}); err != nil {
				return err
			}
			if !activa {
				return nil
			}
			// Tras un alta, lo que esperaba sin clave se cifra ya; tras un recibo, su
			// copia se va con él.
			sellarPendientes(ctx, tx, scope, parsed)
			return borrarConfirmadosTx(ctx, tx, scope, parsed)
		})
	}, Body: func(ctx context.Context, scope storage.Scope, id string) (io.ReadCloser, error) {
		var body io.ReadCloser
		err := s.privateHubTx(ctx, scope.Subject, func(own storage.Scope, tx pgx.Tx) error {
			if own != scope {
				return pg.ErrPrivateStorageScope
			}
			// La época sale del estado del dominio, nunca del cliente.
			var epoch uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT epoch FROM hub_private_storage_states WHERE tenant=$1 AND owner_sub=$2 AND mode IN ('frozen','private') FOR SHARE`, scope.Tenant, scope.Subject).Scan(&epoch); err != nil {
				return err
			}
			var err error
			body, err = pg.ReadPrivateIngressInlineBody(ctx, tx, pg.PrivateIngressConfig{Scope: scope, Epoch: epoch}, id)
			return err
		})
		return body, err
	}}
}

// hubIngressEpoch: el alta de la clave puede ir antes de congelar (fuente legacy);
// todo lo demás exige la época congelada de la persona. Devuelve si la fuente ya no
// es legacy (y hay copias que mirar).
func hubIngressEpoch(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, kind string) (bool, error) {
	if err := pg.EnsurePrivateStorageState(ctx, tx, privateHubStateTable, scope.Tenant, scope.Subject, nil); err != nil {
		return false, err
	}
	var mode string
	var current *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT mode, epoch FROM hub_private_storage_states WHERE tenant=$1 AND owner_sub=$2 FOR SHARE`, scope.Tenant, scope.Subject).Scan(&mode, &current); err != nil {
		return false, err
	}
	if (kind == "key.enroll" || kind == "enrollment") && mode == "legacy" {
		return false, nil
	}
	if (mode != "frozen" && mode != "private") || current == nil || *current != epoch {
		return false, ErrPrivateStorageEpoch
	}
	return true, nil
}
