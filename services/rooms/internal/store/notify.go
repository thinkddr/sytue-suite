// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

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
			`SELECT keywords, dnd_start, dnd_end, paused_until FROM user_notify_prefs
			 WHERE tenant=$1 AND subject=$2`,
			t.Slug, subject).Scan(&p.Keywords, &p.DNDStart, &p.DNDEnd, &p.PausedUntil)
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
	// La pausa no se toca aquí (SetNotifyPause), pero se devuelve la vigente para que
	// quien guarda las palabras clave no la pierda de vista.
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO user_notify_prefs (tenant, subject, keywords, dnd_start, dnd_end)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant, subject)
			 DO UPDATE SET keywords=EXCLUDED.keywords, dnd_start=EXCLUDED.dnd_start, dnd_end=EXCLUDED.dnd_end
			 RETURNING paused_until`,
			t.Slug, subject, kws, p.DNDStart, p.DNDEnd).Scan(&p.PausedUntil)
	})
	if err != nil {
		return domain.NotifyPrefs{}, err
	}
	p.Keywords = kws
	return p, nil
}

// SetNotifyPause fija (o quita, con nil) la pausa de avisos del usuario y devuelve sus
// preferencias completas. El servicio ya la ha saneado (NormalizeNotifyPause).
func (s *Store) SetNotifyPause(ctx context.Context, subject string, until *time.Time) (domain.NotifyPrefs, error) {
	t := tenancy.MustFrom(ctx)
	p := domain.NotifyPrefs{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO user_notify_prefs (tenant, subject, paused_until)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (tenant, subject) DO UPDATE SET paused_until=EXCLUDED.paused_until
			 RETURNING keywords, dnd_start, dnd_end, paused_until`,
			t.Slug, subject, until).Scan(&p.Keywords, &p.DNDStart, &p.DNDEnd, &p.PausedUntil)
	})
	if p.Keywords == nil {
		p.Keywords = []string{}
	}
	return p, err
}
