// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Adjuntos (ADR 0068, bases-2, pieza 4). El store lleva las FICHAS y decide quién puede
// qué; los bytes los mueve la capa HTTP contra el almacén de objetos con la clave que
// sale de aquí. Subir son tres pasos, para que ni un fallo a medias deje basura:
//
//  1. ReservarAdjunto autoriza, comprueba límites y ANOTA la clave en la cola de purga
//     con una hora de margen.
//  2. La capa HTTP sube el objeto.
//  3. ConfirmarAdjunto vuelve a autorizar y a comprobar (con el registro bloqueado),
//     crea la ficha y quita la clave de la cola.
//
// Si el 2 o el 3 no llegan, la purga borra el objeto. Y al borrar una ficha —sola o en
// cascada con su registro, campo, tabla o base— un trigger anota su clave en esa misma
// cola (migración 0015): el objeto no sobrevive a su ficha.

// ObjetoAdjunto dice dónde está un adjunto en el almacén. No sale nunca del servicio.
type ObjetoAdjunto struct {
	Tenant string // el prefijo de organización con el que se subió
	Clave  string // bases/<base>/<adjunto>; la miniatura es Clave + SufijoMiniatura
}

// SufijoMiniatura distingue el objeto de la miniatura del de su adjunto.
const SufijoMiniatura = ".thumb"

// ReservaAdjunto es una subida autorizada y todavía sin confirmar.
type ReservaAdjunto struct {
	ID, registro, campo, base, tabla string
	Objeto                           ObjetoAdjunto
}

// lugarDeAdjunto resuelve la tabla y la base del registro, autoriza `acc` y comprueba
// que `campo` es un campo de adjuntos de ESA tabla: un campo de otra tabla, de otra
// base o de otra organización no existe aquí.
func lugarDeAdjunto(ctx context.Context, tx pgx.Tx, a domain.Actor, registro, campo string, acc domain.Accion) (base, tabla string, err error) {
	if e := errNoFilas(tx.QueryRow(ctx, `SELECT table_id::text, base_id::text FROM base_records WHERE id = $1 AND tenant = $2`, registro, a.Tenant).Scan(&tabla, &base)); e != nil {
		return "", "", e
	}
	if e := autorizar(ctx, tx, a, base, tabla, acc); e != nil {
		return "", "", e
	}
	if campo == "" {
		return base, tabla, nil
	}
	campos, e := camposDe(ctx, tx, a, tabla)
	if e != nil {
		return "", "", e
	}
	if !slices.ContainsFunc(campos, func(c domain.Campo) bool { return c.ID == campo && c.Kind == domain.TipoAdjunto }) {
		return "", "", domain.ErrNoExiste
	}
	return base, tabla, nil
}

// hayHuecoDeAdjunto comprueba el número de adjuntos del campo y la cuota de la base.
//
// ponytail: la cuota se comprueba con el REGISTRO bloqueado, no la base: dos subidas a la
// vez a registros distintos pueden pasarse de la cuota por un fichero. Bloquear la base
// si esa holgura (≤ 20 MiB por réplica y subida simultánea) importa.
func hayHuecoDeAdjunto(ctx context.Context, tx pgx.Tx, a domain.Actor, base, registro, campo string, tam int64) error {
	if tam <= 0 || tam > domain.MaxBytesAdjunto {
		return fmt.Errorf("%w: adjunto demasiado grande", domain.ErrLimite)
	}
	var n int
	var usado int64
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM base_attachments WHERE tenant = $1 AND record_id = $2 AND field_id = $3),
		       (SELECT coalesce(sum(size), 0) FROM base_attachments WHERE tenant = $1 AND base_id = $4)`,
		a.Tenant, registro, campo, base).Scan(&n, &usado); err != nil {
		return err
	}
	if n >= domain.MaxAdjuntosPorCampo || usado+tam > domain.MaxBytesAdjuntosPorBase {
		return domain.ErrLimite
	}
	return nil
}

// PuedeAdjuntar dice si el actor puede subir un fichero a ese campo de ese registro. Es
// la comprobación barata que la capa HTTP hace ANTES de leer el cuerpo; reservar y
// confirmar la repiten.
func (s *Store) PuedeAdjuntar(ctx context.Context, a domain.Actor, registro, campo string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		_, _, err := lugarDeAdjunto(ctx, tx, a, registro, campo, domain.AccEscribirReg)
		return err
	})
}

// ReservarAdjunto autoriza una subida de `tam` bytes al campo de adjuntos `campo` del
// registro (editor) y reserva su clave.
func (s *Store) ReservarAdjunto(ctx context.Context, a domain.Actor, registro, campo string, tam int64) (ReservaAdjunto, error) {
	r := ReservaAdjunto{ID: uuid.NewString(), registro: registro, campo: campo}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		var err error
		if r.base, r.tabla, err = lugarDeAdjunto(ctx, tx, a, registro, campo, domain.AccEscribirReg); err != nil {
			return err
		}
		if err := hayHuecoDeAdjunto(ctx, tx, a, r.base, registro, campo, tam); err != nil {
			return err
		}
		r.Objeto = ObjetoAdjunto{Tenant: a.Tenant, Clave: "bases/" + r.base + "/" + r.ID}
		_, err = tx.Exec(ctx, `INSERT INTO base_attachment_purge (tenant, store_tenant, object_key, after) VALUES ($1, $1, $2, now() + interval '1 hour')`, a.Tenant, r.Objeto.Clave)
		return err
	})
	return r, err
}

// ConfirmarAdjunto crea la ficha de una subida ya hecha. Vuelve a autorizar y a
// comprobar los límites con el registro bloqueado: dos subidas a la vez no pasan las dos
// por el último hueco, y quien perdió el permiso mientras subía no deja nada.
func (s *Store) ConfirmarAdjunto(ctx context.Context, a domain.Actor, r ReservaAdjunto, nombre, mime, sha256 string, tam int64, miniatura bool) (domain.Adjunto, error) {
	adj := domain.Adjunto{ID: r.ID, RecordID: r.registro, FieldID: r.campo, Name: nombre, Mime: mime, Size: tam, HasThumb: miniatura, CreatedBy: a.Sub}
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		base, tabla, err := lugarDeAdjunto(ctx, tx, a, r.registro, r.campo, domain.AccEscribirReg)
		if err != nil {
			return err
		}
		if base != r.base {
			return domain.ErrNoExiste
		}
		if _, err := tx.Exec(ctx, `SELECT 1 FROM base_records WHERE id = $1 AND tenant = $2 FOR NO KEY UPDATE`, r.registro, a.Tenant); err != nil {
			return err
		}
		if err := hayHuecoDeAdjunto(ctx, tx, a, base, r.registro, r.campo, tam); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO base_attachments (id, tenant, store_tenant, base_id, table_id, record_id, field_id, name, mime, size, sha256, has_thumb, created_by)
			VALUES ($1, $2, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING created_at`,
			r.ID, a.Tenant, base, tabla, r.registro, r.campo, nombre, mime, tam, sha256, miniatura, a.Sub).Scan(&adj.CreatedAt); err != nil {
			return err
		}
		// Ya tiene ficha: deja de estar pendiente de purga.
		if _, err := tx.Exec(ctx, `DELETE FROM base_attachment_purge WHERE tenant = $1 AND object_key = $2`, a.Tenant, r.Objeto.Clave); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, &r.registro, &r.campo, "attachment.add", nil,
			jsonDe(map[string]any{"attachment": r.ID, "name": nombre, "mime": mime, "size": tam, "sha256": sha256}))
	})
	return adj, err
}

// AdjuntoParaLeer devuelve la ficha de un adjunto y dónde está su objeto, para quien
// puede LEER el registro (lector). El adjunto ha de ser de ESE registro: no basta con
// conocer su id.
func (s *Store) AdjuntoParaLeer(ctx context.Context, a domain.Actor, registro, adjunto string) (domain.Adjunto, ObjetoAdjunto, error) {
	var adj domain.Adjunto
	var obj ObjetoAdjunto
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		base, _, err := lugarDeAdjunto(ctx, tx, a, registro, "", domain.AccLeer)
		if err != nil {
			return err
		}
		if err := errNoFilas(tx.QueryRow(ctx, `
			SELECT id::text, record_id::text, field_id::text, name, mime, size, has_thumb, created_by, created_at, store_tenant
			  FROM base_attachments WHERE id = $1 AND record_id = $2 AND tenant = $3`, adjunto, registro, a.Tenant).
			Scan(&adj.ID, &adj.RecordID, &adj.FieldID, &adj.Name, &adj.Mime, &adj.Size, &adj.HasThumb, &adj.CreatedBy, &adj.CreatedAt, &obj.Tenant)); err != nil {
			return err
		}
		obj.Clave = "bases/" + base + "/" + adj.ID
		return nil
	})
	return adj, obj, err
}

// BorrarAdjunto borra un adjunto de un registro (editor). El objeto lo borra la purga.
func (s *Store) BorrarAdjunto(ctx context.Context, a domain.Actor, registro, adjunto string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		base, tabla, err := lugarDeAdjunto(ctx, tx, a, registro, "", domain.AccEscribirReg)
		if err != nil {
			return err
		}
		var campo, nombre string
		if err := errNoFilas(tx.QueryRow(ctx, `DELETE FROM base_attachments WHERE id = $1 AND record_id = $2 AND tenant = $3 RETURNING field_id::text, name`,
			adjunto, registro, a.Tenant).Scan(&campo, &nombre)); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, &registro, &campo, "attachment.delete", jsonDe(map[string]string{"attachment": adjunto, "name": nombre}), nil)
	})
}

// cargarAdjuntos pone en cada registro las fichas de sus adjuntos, por campo.
func cargarAdjuntos(ctx context.Context, tx pgx.Tx, a domain.Actor, campos []domain.Campo, regs []domain.Registro, ids []string) error {
	if len(ids) == 0 || !slices.ContainsFunc(campos, func(c domain.Campo) bool { return c.Kind == domain.TipoAdjunto }) {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, record_id::text, field_id::text, name, mime, size, has_thumb, created_by, created_at
		  FROM base_attachments WHERE tenant = $1 AND record_id = ANY($2::uuid[])
		 ORDER BY record_id, field_id, created_at, id LIMIT $3`, a.Tenant, ids, len(ids)*domain.MaxAdjuntosPorCampo*domain.MaxCamposPorTabla)
	if err != nil {
		return err
	}
	idx := make(map[string]int, len(regs))
	for i, r := range regs {
		idx[r.ID] = i
	}
	var x domain.Adjunto
	_, err = pgx.ForEachRow(rows, []any{&x.ID, &x.RecordID, &x.FieldID, &x.Name, &x.Mime, &x.Size, &x.HasThumb, &x.CreatedBy, &x.CreatedAt}, func() error {
		r := &regs[idx[x.RecordID]]
		if r.Attachments == nil {
			r.Attachments = map[string][]domain.Adjunto{}
		}
		r.Attachments[x.FieldID] = append(r.Attachments[x.FieldID], x)
		return nil
	})
	return err
}

// PurgarAdjuntos borra del almacén los objetos anotados en la cola cuyo plazo ha
// pasado y, borrados, quita su fila. Devuelve cuántos purgó. `borrar` ha de ser
// idempotente: un objeto que ya no está no es un error.
func (s *Store) PurgarAdjuntos(ctx context.Context, borrar func(ctx context.Context, obj ObjetoAdjunto) error) (int, error) {
	type pendiente struct {
		id     int64
		tenant string
		obj    ObjetoAdjunto
	}
	var cola []pendiente
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.bases_publico', 'on', true)`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, tenant, store_tenant, object_key FROM base_attachment_purge WHERE after <= now() ORDER BY after LIMIT 100`)
		if err != nil {
			return err
		}
		cola, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (p pendiente, err error) {
			return p, row.Scan(&p.id, &p.tenant, &p.obj.Tenant, &p.obj.Clave)
		})
		return err
	})
	hechos := 0
	for _, p := range cola {
		// Primero el objeto y luego la fila: si el almacén falla, se reintenta.
		if e := borrar(ctx, p.obj); e != nil {
			err = errors.Join(err, e)
			continue
		}
		e := s.en(ctx, domain.Actor{Sub: "system:purge", Tenant: p.tenant}, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM base_attachment_purge WHERE id = $1 AND tenant = $2`, p.id, p.tenant)
			return err
		})
		if e != nil {
			err = errors.Join(err, e)
			continue
		}
		hechos++
	}
	return hechos, err
}
