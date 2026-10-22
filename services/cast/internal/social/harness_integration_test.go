// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package social

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
)

// montar levanta una base con las migraciones REALES del servicio: 0001 (vídeos y
// personas) y 0003 (lo social), con su RLS y sus políticas.
func montar(t *testing.T) *pgtest.Harness {
	t.Helper()
	return pgtest.New(t, db.Migrations, db.MigrationsDir)
}

// videoEn crea un vídeo en la organización, con el rol de aplicación y la RLS puesta.
func videoEn(t *testing.T, pool *pg.Pool, tenant, owner, visibility, status string) Video {
	t.Helper()
	v := Video{Tenant: tenant, OwnerSub: owner, ComentariosActivos: true, ChatActivo: true}
	err := pool.InTx(context.Background(), func(tx pgx.Tx) error {
		if err := pg.SetTenant(context.Background(), tx, tenant); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `
			INSERT INTO videos (tenant, owner_sub, kind, status, visibility, title, live_ended_at)
			VALUES ($1, $2, 'live', $3, $4, 'Prueba', now())
			RETURNING id`, tenant, owner, status, visibility).Scan(&v.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// accesoDePrueba es el doble de `acceso.Resolver`: conoce los vídeos que se le
// dan y decide como lo hará el de verdad — la organización del token ve los
// suyos, el dueño y quien tenga rol de administración moderan, y sin sesión solo
// se ven los public/unlisted.
type accesoDePrueba struct {
	videos map[uuid.UUID]Video
	// publicos son los que se pueden ver sin sesión.
	publicos map[uuid.UUID]bool
	// administra son los sub con rol admin/owner en su organización.
	administra map[string]bool
}

func nuevoAcceso() *accesoDePrueba {
	return &accesoDePrueba{videos: map[uuid.UUID]Video{}, publicos: map[uuid.UUID]bool{}, administra: map[string]bool{}}
}

func (a *accesoDePrueba) conoce(v Video, publico bool) Video {
	a.videos[v.ID] = v
	a.publicos[v.ID] = publico
	return v
}

func (a *accesoDePrueba) resolver(_ context.Context, quien Quien, id uuid.UUID) (Video, Permisos, error) {
	v, ok := a.videos[id]
	if !ok {
		return Video{}, Permisos{}, ErrNoExiste
	}
	if quien.Anonimo {
		if !a.publicos[id] {
			return Video{}, Permisos{}, ErrNoExiste
		}
		return v, Permisos{Ver: true}, nil
	}
	if quien.Tenant != v.Tenant && !a.publicos[id] {
		return Video{}, Permisos{}, ErrNoExiste
	}
	modera := quien.Sub == v.OwnerSub || (quien.Tenant == v.Tenant && a.administra[quien.Sub])
	return v, Permisos{Ver: true, Editar: modera, Moderar: modera}, nil
}

func persona(sub, tenant string) Quien {
	return Quien{Sub: sub, Tenant: tenant, Nombre: "Persona " + sub, Usuario: sub}
}

var nadie = Quien{Anonimo: true}

func ctxCorto(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}
