// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// La política de contacto con gente de fuera de cada organización (mig 0083).

func politicaTx(ctx context.Context, tx pgx.Tx, tenant string) (domain.PoliticaContactoExterno, error) {
	p := domain.PoliticaContactoExterno{Modo: domain.ContactoAbierto, Confianza: []string{}}
	err := tx.QueryRow(ctx,
		`SELECT modo, confianza, actualizada FROM politica_contacto_externo WHERE tenant = $1`, tenant).
		Scan(&p.Modo, &p.Confianza, &p.Actualizada)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	return p, err
}

// PoliticaContactoExterno lee la política de la organización del contexto; sin fila,
// abierta.
func (s *Store) PoliticaContactoExterno(ctx context.Context) (domain.PoliticaContactoExterno, error) {
	t := tenancy.MustFrom(ctx)
	var p domain.PoliticaContactoExterno
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		p, err = politicaTx(ctx, tx, t.Slug)
		return err
	})
	return p, err
}

// GuardarPoliticaContactoExterno guarda la política (ya validada) de la organización
// del contexto y la devuelve como queda.
func (s *Store) GuardarPoliticaContactoExterno(ctx context.Context, p domain.PoliticaContactoExterno, por string) (domain.PoliticaContactoExterno, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.PoliticaContactoExterno
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO politica_contacto_externo (tenant, modo, confianza, actualizada, actualizada_por)
			 VALUES ($1, $2, $3, now(), $4)
			 ON CONFLICT (tenant) DO UPDATE SET modo = EXCLUDED.modo, confianza = EXCLUDED.confianza,
			     actualizada = now(), actualizada_por = EXCLUDED.actualizada_por`,
			t.Slug, p.Modo, p.Confianza, por); err != nil {
			return err
		}
		var err error
		out, err = politicaTx(ctx, tx, t.Slug)
		return err
	})
	return out, err
}

// ContactoPermitido dice si las políticas de las DOS organizaciones dejan que alguien
// de `propia` entre en una conversación de `anfitriona`: cada una tiene que admitir a
// la otra. Lee cada fila en su organización, con su RLS, en una transacción.
func (s *Store) ContactoPermitido(ctx context.Context, propia, anfitriona string) (bool, error) {
	if propia == anfitriona {
		return true, nil
	}
	var ok bool
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, propia); err != nil {
			return err
		}
		pp, err := politicaTx(ctx, tx, propia)
		if err != nil {
			return err
		}
		if !pp.Admite(anfitriona) {
			return nil
		}
		if err := pg.SetTenant(ctx, tx, anfitriona); err != nil {
			return err
		}
		pa, err := politicaTx(ctx, tx, anfitriona)
		if err != nil {
			return err
		}
		ok = pa.Admite(propia)
		return nil
	})
	return ok, err
}
