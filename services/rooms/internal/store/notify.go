// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// GetNotifyPrefs lee las preferencias de notificación del usuario (palabras clave +
// ventana DND). Sin fila aún → preferencias por defecto (sin palabras, DND apagado).
// Estado PER-USUARIO (subject); aislado por tenant vía RLS (mig. 0012).
func (s *Store) GetNotifyPrefs(ctx context.Context, subject string) (domain.NotifyPrefs, error) {
	t := tenancy.MustFrom(ctx)
	p := domain.NotifyPrefs{Keywords: []string{}}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT keywords, dnd_start, dnd_end FROM user_notify_prefs
			 WHERE tenant=$1 AND subject=$2`,
			t.Slug, subject).Scan(&p.Keywords, &p.DNDStart, &p.DNDEnd)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if p.Keywords == nil {
		p.Keywords = []string{}
	}
	return p, err
}

// PutNotifyPrefs fija (upsert) las preferencias de notificación del usuario. El
// servicio ya las ha saneado (dedupe/recorte, DND acotado); el CHECK del rango DND es
// la red de seguridad a nivel de esquema.
func (s *Store) PutNotifyPrefs(ctx context.Context, subject string, p domain.NotifyPrefs) (domain.NotifyPrefs, error) {
	t := tenancy.MustFrom(ctx)
	kws := p.Keywords
	if kws == nil {
		kws = []string{}
	}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO user_notify_prefs (tenant, subject, keywords, dnd_start, dnd_end)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant, subject)
			 DO UPDATE SET keywords=EXCLUDED.keywords, dnd_start=EXCLUDED.dnd_start, dnd_end=EXCLUDED.dnd_end`,
			t.Slug, subject, kws, p.DNDStart, p.DNDEnd)
		return err
	})
	if err != nil {
		return domain.NotifyPrefs{}, err
	}
	p.Keywords = kws
	return p, nil
}
