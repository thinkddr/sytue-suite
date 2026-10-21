// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Bloquear a una persona (docs/decisions.md, «Cast: denuncia y bloqueo»).
//
// Hay dos bloqueos y los dos se aplican en el servidor:
//
//   - Personal: quien bloquea deja de ver los comentarios y los mensajes de la
//     otra persona y de recibir sus avisos; y la otra persona deja de poder
//     comentar y escribir en los vídeos de quien la bloqueó.
//   - De organización: lo ponen quienes la administran, y esa persona deja de
//     poder comentar y escribir en cualquier vídeo de la organización.
//
// Bloquear no borra nada, y se deshace.

// Bloqueado es una fila de una lista de bloqueos.
type Bloqueado struct {
	Sub       string    `json:"sub"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Bloquear bloquea a una persona que aparece en el vídeo (su dueño o quien ha
// escrito en él). Con `org`, para toda la organización del vídeo: solo quien la
// administra.
func (s *Servicio) Bloquear(ctx context.Context, quien Quien, videoID uuid.UUID, sub string, org bool) error {
	if quien.Anonimo || quien.Sub == "" {
		return ErrProhibido
	}
	if sub == quien.Sub {
		return fmt.Errorf("%w: no puedes bloquearte", ErrEntradaInvalida)
	}
	v, _, err := s.resolver(ctx, quien, videoID)
	if err != nil {
		return err
	}
	if org {
		// El rol del token vale solo en su organización, que ha de ser la del vídeo.
		if !quien.Admin || quien.Tenant != v.Tenant {
			return ErrProhibido
		}
		return s.store.bloquearEnOrg(ctx, v, sub, quien.Sub)
	}
	if err := s.store.bloquear(ctx, v, quien.Sub, sub); err != nil {
		return err
	}
	s.hub.bloqueo(quien.Sub, sub, true)
	return nil
}

// Desbloquear quita un bloqueo personal de quien pregunta. Quitar uno que no
// existe no es un error.
func (s *Servicio) Desbloquear(ctx context.Context, quien Quien, sub string) error {
	if quien.Anonimo || quien.Sub == "" {
		return ErrProhibido
	}
	if err := s.store.desbloquear(ctx, quien.Sub, sub); err != nil {
		return err
	}
	s.hub.bloqueo(quien.Sub, sub, false)
	return nil
}

// Bloqueados es la lista personal de quien pregunta.
func (s *Servicio) Bloqueados(ctx context.Context, quien Quien) ([]Bloqueado, error) {
	if quien.Anonimo || quien.Sub == "" {
		return nil, ErrProhibido
	}
	return s.store.bloqueados(ctx, quien.Sub)
}

// BloqueadosEnOrg es la lista de la organización de quien la administra.
func (s *Servicio) BloqueadosEnOrg(ctx context.Context, quien Quien) ([]Bloqueado, error) {
	if err := modera(quien); err != nil {
		return nil, err
	}
	return s.store.bloqueadosEnOrg(ctx, quien.Tenant)
}

// DesbloquearEnOrg quita un bloqueo de la organización de quien la administra.
func (s *Servicio) DesbloquearEnOrg(ctx context.Context, quien Quien, sub string) error {
	if err := modera(quien); err != nil {
		return err
	}
	return s.store.desbloquearEnOrg(ctx, quien.Tenant, sub, quien.Sub)
}

// QuienesBloquean dice cuáles de `candidatos` tienen bloqueada a esa persona:
// son a quienes NO hay que mandarles sus avisos.
func (s *Servicio) QuienesBloquean(ctx context.Context, sub string, candidatos []string) (map[string]bool, error) {
	return s.store.quienesBloquean(ctx, sub, candidatos)
}

// ── Store ───────────────────────────────────────────────────────────────────

// comoPersona fija `app.cast_persona` en la transacción: lo que deja leer los
// bloqueos en los que esa persona es un extremo.
func comoPersona(ctx context.Context, tx pgx.Tx, sub string) error {
	if _, err := tx.Exec(ctx, "SELECT set_config('app.cast_persona', $1, true)", sub); err != nil {
		return fmt.Errorf("social: set_config app.cast_persona: %w", err)
	}
	return nil
}

func (s *store) enPersona(ctx context.Context, sub string, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := comoPersona(ctx, tx, sub); err != nil {
			return err
		}
		return fn(tx)
	})
}

// vetadoTx dice si esa persona no puede escribir en el vídeo: la bloqueó su dueño
// o la organización. Va en la transacción que escribe, ya en el tenant del vídeo.
func vetadoTx(ctx context.Context, tx pgx.Tx, v Video, sub string) error {
	if err := comoPersona(ctx, tx, sub); err != nil {
		return err
	}
	var vetado bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM org_blocks WHERE tenant = $1 AND blocked_sub = $2)
		    OR EXISTS (SELECT 1 FROM person_blocks WHERE blocker_sub = $3 AND blocked_sub = $2)`,
		v.Tenant, sub, v.OwnerSub).Scan(&vetado)
	if err != nil {
		return fmt.Errorf("social: comprobar bloqueo: %w", err)
	}
	if vetado {
		return ErrBloqueado
	}
	return nil
}

func (s *store) bloquear(ctx context.Context, v Video, quien, sub string) error {
	return s.enTenant(ctx, v, func(tx pgx.Tx) error {
		nombre, err := personaEn(ctx, tx, v, sub)
		if err != nil {
			return err
		}
		if err := comoPersona(ctx, tx, quien); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO person_blocks (blocker_sub, blocked_sub, blocked_name) VALUES ($1, $2, $3)
			ON CONFLICT (blocker_sub, blocked_sub) DO NOTHING`, quien, sub, nombre)
		return err
	})
}

func (s *store) desbloquear(ctx context.Context, quien, sub string) error {
	return s.enPersona(ctx, quien, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM person_blocks WHERE blocker_sub = $1 AND blocked_sub = $2`, quien, sub)
		return err
	})
}

func leerBloqueados(rows pgx.Rows, err error) ([]Bloqueado, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Bloqueado, error) {
		var b Bloqueado
		return b, r.Scan(&b.Sub, &b.Name, &b.CreatedAt)
	})
}

func (s *store) bloqueados(ctx context.Context, quien string) ([]Bloqueado, error) {
	var out []Bloqueado
	err := s.enPersona(ctx, quien, func(tx pgx.Tx) (err error) {
		out, err = leerBloqueados(tx.Query(ctx, `
			SELECT blocked_sub, blocked_name, created_at FROM person_blocks
			 WHERE blocker_sub = $1 ORDER BY created_at DESC, blocked_sub LIMIT 500`, quien))
		return err
	})
	return out, err
}

func (s *store) quienesBloquean(ctx context.Context, sub string, candidatos []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(candidatos) == 0 {
		return out, nil
	}
	err := s.enPersona(ctx, sub, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT blocker_sub FROM person_blocks WHERE blocked_sub = $1 AND blocker_sub = ANY($2)`, sub, candidatos)
		if err != nil {
			return err
		}
		subs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		for _, b := range subs {
			out[b] = true
		}
		return err
	})
	return out, err
}

func bloquearEnOrgTx(ctx context.Context, tx pgx.Tx, tenant, sub, nombre, actor string) error {
	if sub == "" || sub == actor {
		return fmt.Errorf("%w: persona a bloquear", ErrEntradaInvalida)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO org_blocks (tenant, blocked_sub, blocked_name, created_by) VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant, blocked_sub) DO NOTHING`, tenant, sub, nombre, actor)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	return auditar(ctx, tx, tenant, actor, "block.created", "person:"+sub, map[string]any{})
}

func (s *store) bloquearEnOrg(ctx context.Context, v Video, sub, actor string) error {
	return s.enTenant(ctx, v, func(tx pgx.Tx) error {
		nombre, err := personaEn(ctx, tx, v, sub)
		if err != nil {
			return err
		}
		return bloquearEnOrgTx(ctx, tx, v.Tenant, sub, nombre, actor)
	})
}

func (s *store) bloqueadosEnOrg(ctx context.Context, tenant string) ([]Bloqueado, error) {
	var out []Bloqueado
	err := s.enOrg(ctx, tenant, func(tx pgx.Tx) (err error) {
		out, err = leerBloqueados(tx.Query(ctx, `
			SELECT blocked_sub, blocked_name, created_at FROM org_blocks
			 WHERE tenant = $1 ORDER BY created_at DESC, blocked_sub LIMIT 500`, tenant))
		return err
	})
	return out, err
}

func (s *store) desbloquearEnOrg(ctx context.Context, tenant, sub, actor string) error {
	return s.enOrg(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM org_blocks WHERE tenant = $1 AND blocked_sub = $2`, tenant, sub)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return auditar(ctx, tx, tenant, actor, "block.removed", "person:"+sub, map[string]any{})
	})
}
