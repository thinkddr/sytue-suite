// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// COMPARTIR CON OTRAS ORGANIZACIONES (ADR 0068, sobre el 0040 y el 0052)
//
// La base vive en la organización ANFITRIONA. La persona de fuera es una entrada
// `external` de su ACL y su token trae SU organización, así que el servicio tiene que
// decidir en qué organización se atiende cada petición. Lo decide ÉL, nunca el cliente:
//
//  1. ¿En qué organizaciones es invitada esta persona? Es la única lectura entre
//     organizaciones (política `base_permissions_externo`: solo SELECT, solo sus filas).
//  2. Si lo que pide no existe en su organización y sí en una de esas, la petición sigue
//     en la anfitriona con Actor.Externo: allí solo cuentan sus entradas `external`.
//
// Que el recurso exista en la anfitriona no da nada: lo que abre es la ACL de la base,
// que se resuelve después, en la transacción de la operación, como siempre.

const (
	// maxAnfitrionas acota en cuántas organizaciones se busca un recurso por petición.
	maxAnfitrionas = 20
	// vidaAnfitrionas es cuánto se recuerda la lista de anfitrionas de una persona. No
	// retrasa una REVOCACIÓN (la ACL se comprueba en cada operación); solo que una
	// invitación recién hecha tarde como mucho esto en poder abrirse desde OTRA réplica
	// (la que invita olvida lo recordado de esa persona en el acto).
	vidaAnfitrionas = 30 * time.Second
	// maxRecordadas acota la memoria del recuerdo: al llenarse, se vacía.
	maxRecordadas = 10000
)

// recursos son las clases de recurso por las que entra una petición y su tabla. La
// tabla sale SIEMPRE de este mapa, nunca de la petición.
var recursos = map[string]string{
	"base":    "bases",
	"table":   "base_tables",
	"record":  "base_records",
	"view":    "base_views",
	"comment": "base_comments",
}

type anfitrionasDe struct {
	orgs   []string
	caduca time.Time
}

// recuerdo de anfitrionas por `sub`. ponytail: en memoria y por réplica; basta porque
// lo que autoriza no es esto sino la ACL de cada operación.
type recuerdoAnfitrionas struct {
	mu sync.Mutex
	m  map[string]anfitrionasDe
}

func (r *recuerdoAnfitrionas) leer(sub string) ([]string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.m[sub]
	if !ok || time.Now().After(v.caduca) {
		return nil, false
	}
	return v.orgs, true
}

func (r *recuerdoAnfitrionas) guardar(sub string, orgs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil || len(r.m) >= maxRecordadas {
		r.m = map[string]anfitrionasDe{}
	}
	r.m[sub] = anfitrionasDe{orgs: orgs, caduca: time.Now().Add(vidaAnfitrionas)}
}

// olvidar tira lo recordado de una persona: tras invitarla, la réplica que la invita
// no la hace esperar.
func (r *recuerdoAnfitrionas) olvidar(sub string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, sub)
}

// anfitrionas son las organizaciones (≠ la propia) donde `sub` es invitada.
func (s *Store) anfitrionas(ctx context.Context, sub, propia string) ([]string, error) {
	if s.pool == nil || sub == "" {
		return nil, nil
	}
	orgs, ok := s.externos.leer(sub)
	if !ok {
		err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.bases_externo', $1, true)`, sub); err != nil {
				return err
			}
			rows, err := tx.Query(ctx, `SELECT DISTINCT tenant FROM base_permissions
				WHERE principal_kind = 'external' AND principal_id = $1 ORDER BY tenant LIMIT $2`, sub, maxAnfitrionas)
			if err != nil {
				return err
			}
			orgs, err = pgx.CollectRows(rows, pgx.RowTo[string])
			return err
		})
		if err != nil {
			return nil, err
		}
		s.externos.guardar(sub, orgs)
	}
	out := make([]string, 0, len(orgs))
	for _, o := range orgs {
		if o != propia && tenancy.ValidSlug(o) {
			out = append(out, o)
		}
	}
	return out, nil
}

// existeEn dice si el recurso existe en esa organización.
func (s *Store) existeEn(ctx context.Context, org, tabla, id string) (bool, error) {
	var hay bool
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, org); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+pgx.Identifier{tabla}.Sanitize()+` WHERE id = $1 AND tenant = $2)`, id, org).Scan(&hay)
	})
	return hay, err
}

// Anfitrion resuelve en qué organización se atiende la petición de `a` sobre el
// recurso (clase, id): "" si es la suya —lo normal— o la anfitriona si el recurso no
// existe en la suya y sí en una donde es invitada. Devolver una anfitriona NO concede
// nada: quien llama sigue con Actor{Tenant: anfitriona, Externo: true} y la ACL decide.
func (s *Store) Anfitrion(ctx context.Context, a domain.Actor, clase, id string) (string, error) {
	tabla, ok := recursos[clase]
	if !ok || a.Externo || !tenancy.ValidSlug(a.Tenant) {
		return "", nil
	}
	orgs, err := s.anfitrionas(ctx, a.Sub, a.Tenant)
	if err != nil || len(orgs) == 0 {
		return "", err
	}
	if propio, err := s.existeEn(ctx, a.Tenant, tabla, id); err != nil || propio {
		return "", err
	}
	for _, org := range orgs {
		hay, err := s.existeEn(ctx, org, tabla, id)
		if err != nil {
			return "", err
		}
		if hay {
			return org, nil
		}
	}
	return "", nil
}

// basesDeFuera son las bases que otras organizaciones han compartido con `a`, con su
// rol allí y `host_org`. Una anfitriona que falla no tumba la lista propia.
func (s *Store) basesDeFuera(ctx context.Context, a domain.Actor) ([]domain.Base, error) {
	orgs, err := s.anfitrionas(ctx, a.Sub, a.Tenant)
	if err != nil {
		return nil, err
	}
	var out []domain.Base
	for _, org := range orgs {
		bs, err := s.listarBasesEn(ctx, domain.Actor{Sub: a.Sub, Tenant: org, Externo: true})
		if err != nil {
			continue
		}
		for i := range bs {
			bs[i].HostOrg = org
		}
		out = append(out, bs...)
	}
	return out, nil
}
