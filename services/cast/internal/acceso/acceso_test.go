// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package acceso

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// lectorFalso imita las tres políticas del store sobre un mapa de vídeos.
type lectorFalso struct {
	videos map[uuid.UUID]domain.Video
	lista  map[uuid.UUID][]string
}

func (l *lectorFalso) EnOrganizacion(_ context.Context, tenant string, id uuid.UUID) (domain.Video, error) {
	v, ok := l.videos[id]
	if !ok || v.Tenant != tenant {
		return domain.Video{}, domain.ErrNoExiste
	}
	return v, nil
}

func (l *lectorFalso) Publico(_ context.Context, id uuid.UUID) (domain.Video, error) {
	v, ok := l.videos[id]
	if !ok || !v.Visibility.Abierta() || !v.Status.Visible() {
		return domain.Video{}, domain.ErrNoExiste
	}
	return v, nil
}

func (l *lectorFalso) Invitado(_ context.Context, sub string, id uuid.UUID) (domain.Video, error) {
	v, ok := l.videos[id]
	if !ok || v.Visibility != domain.VisibilityPeople || !v.Status.Visible() {
		return domain.Video{}, domain.ErrNoExiste
	}
	for _, s := range l.lista[id] {
		if s == sub {
			return v, nil
		}
	}
	return domain.Video{}, domain.ErrNoExiste
}

func video(vis domain.Visibility, st domain.Status) domain.Video {
	return domain.Video{ID: uuid.New(), Tenant: "acme", OwnerSub: "ana", Visibility: vis, Status: st}
}

func TestResolver(t *testing.T) {
	publico := video(domain.VisibilityPublic, domain.StatusReady)
	oculto := video(domain.VisibilityUnlisted, domain.StatusLive)
	org := video(domain.VisibilityOrg, domain.StatusReady)
	personas := video(domain.VisibilityPeople, domain.StatusReady)
	procesando := video(domain.VisibilityPublic, domain.StatusProcessing)
	l := &lectorFalso{
		videos: map[uuid.UUID]domain.Video{publico.ID: publico, oculto.ID: oculto, org.ID: org, personas.ID: personas, procesando.ID: procesando},
		lista:  map[uuid.UUID][]string{personas.ID: {"carlos", "dana"}},
	}

	nadie := Quien{Anonimo: true}
	ana := Quien{Sub: "ana", Tenant: "acme"}
	bea := Quien{Sub: "bea", Tenant: "acme"}
	admin := Quien{Sub: "root", Tenant: "acme", Admin: true}
	carlos := Quien{Sub: "carlos", Tenant: "acme"}
	dana := Quien{Sub: "dana", Tenant: "globex"}
	// Owner de OTRA organización: sus roles no valen aquí.
	jefeAjeno := Quien{Sub: "jefe", Tenant: "globex", Admin: true}

	casos := []struct {
		nombre string
		quien  Quien
		video  domain.Video
		ve     bool
		edita  bool
	}{
		{"sin sesión ve lo público", nadie, publico, true, false},
		{"sin sesión ve lo oculto con el enlace", nadie, oculto, true, false},
		{"sin sesión no ve lo de la organización", nadie, org, false, false},
		{"sin sesión no ve lo de personas", nadie, personas, false, false},
		{"sin sesión no ve lo que aún se procesa", nadie, procesando, false, false},
		{"la dueña ve lo que aún se procesa y lo edita", ana, procesando, true, true},
		{"la dueña edita lo suyo", ana, org, true, true},
		{"alguien de la organización ve lo de la organización", bea, org, true, false},
		{"alguien de la organización no ve lo de personas si no está", bea, personas, false, false},
		{"alguien de la organización sí lo ve si está en la lista", carlos, personas, true, false},
		{"quien administra edita y modera lo de su organización", admin, org, true, true},
		{"quien administra ve lo de personas aunque no esté", admin, personas, true, true},
		{"una persona de otra organización lo ve si está en la lista", dana, personas, true, false},
		{"una persona de otra organización no ve lo de la organización", dana, org, false, false},
		{"una persona de otra organización ve lo público", dana, publico, true, false},
		{"un owner ajeno no edita nada", jefeAjeno, publico, true, false},
		{"un owner ajeno no ve lo de personas si no está", jefeAjeno, personas, false, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			v, p, err := Resolver(context.Background(), l, c.quien, c.video.ID)
			if !c.ve {
				if !errors.Is(err, domain.ErrNoExiste) {
					t.Fatalf("esperaba ErrNoExiste, err=%v permisos=%+v", err, p)
				}
				return
			}
			if err != nil {
				t.Fatalf("err=%v", err)
			}
			if v.ID != c.video.ID || !p.Ver {
				t.Fatalf("vídeo %s permisos %+v", v.ID, p)
			}
			if p.Editar != c.edita || p.Moderar != c.edita {
				t.Fatalf("editar/moderar=%v/%v, esperaba %v", p.Editar, p.Moderar, c.edita)
			}
		})
	}

	t.Run("lo que no existe no existe para nadie", func(t *testing.T) {
		for _, q := range []Quien{nadie, ana, admin, dana} {
			if _, _, err := Resolver(context.Background(), l, q, uuid.New()); !errors.Is(err, domain.ErrNoExiste) {
				t.Fatalf("%+v: %v", q, err)
			}
		}
	})
}

// lectorSinRLS devuelve cualquier vídeo en las lecturas de fuera, como haría una
// conexión que se salta la RLS (superusuario o BYPASSRLS).
type lectorSinRLS struct{ v domain.Video }

func (l lectorSinRLS) EnOrganizacion(context.Context, string, uuid.UUID) (domain.Video, error) {
	return domain.Video{}, domain.ErrNoExiste
}
func (l lectorSinRLS) Publico(context.Context, uuid.UUID) (domain.Video, error) { return l.v, nil }
func (l lectorSinRLS) Invitado(context.Context, string, uuid.UUID) (domain.Video, error) {
	return l.v, nil
}

// Sin la RLS de por medio, la regla sigue sin dejar ver desde fuera lo que no es
// público, ni lo que aún no se puede ver.
func TestResolverNoDependeSoloDeLaRLS(t *testing.T) {
	t.Parallel()
	anonimo := Quien{Anonimo: true}
	deFuera := Quien{Sub: "eva", Tenant: "otra"}
	for _, c := range []struct {
		nombre string
		v      domain.Video
		quien  Quien
	}{
		{"org a un anónimo", video(domain.VisibilityOrg, domain.StatusReady), anonimo},
		{"personas a un anónimo", video(domain.VisibilityPeople, domain.StatusReady), anonimo},
		{"público procesándose", video(domain.VisibilityPublic, domain.StatusProcessing), anonimo},
		{"org a alguien de fuera", video(domain.VisibilityOrg, domain.StatusReady), deFuera},
		{"personas sin terminar a alguien de fuera", video(domain.VisibilityPeople, domain.StatusFailed), deFuera},
	} {
		if _, _, err := Resolver(context.Background(), lectorSinRLS{c.v}, c.quien, c.v.ID); !errors.Is(err, domain.ErrNoExiste) {
			t.Errorf("%s: esperaba ErrNoExiste, got %v", c.nombre, err)
		}
	}
	// Y lo que sí es abierto se sigue viendo.
	v := video(domain.VisibilityUnlisted, domain.StatusReady)
	if _, p, err := Resolver(context.Background(), lectorSinRLS{v}, anonimo, v.ID); err != nil || !p.Ver {
		t.Fatalf("un oculto listo debería verse: %v %+v", err, p)
	}
}

// lectorSinFiltro es un store cuya lectura por organización NO filtra por
// organización: lo que pasaría si el SQL se quedara sin su predicado y la base no
// evaluara la RLS. `Resolver` no puede dar permisos sobre lo que devuelva.
type lectorSinFiltro struct{ lectorFalso }

func (l *lectorSinFiltro) EnOrganizacion(_ context.Context, _ string, id uuid.UUID) (domain.Video, error) {
	v, ok := l.videos[id]
	if !ok {
		return domain.Video{}, domain.ErrNoExiste
	}
	return v, nil
}

// Los roles del token valen SOLO en la organización del vídeo, y eso lo comprueba
// `Resolver`, no solo el store: quien administra OTRA organización no edita, y
// alguien de otra organización no ve un vídeo `org`.
func TestResolverNoDaPermisosFueraDeLaOrganizacion(t *testing.T) {
	org := video(domain.VisibilityOrg, domain.StatusReady)
	borrador := video(domain.VisibilityOrg, domain.StatusProcessing)
	publico := video(domain.VisibilityPublic, domain.StatusReady)
	l := &lectorSinFiltro{lectorFalso{videos: map[uuid.UUID]domain.Video{org.ID: org, borrador.ID: borrador, publico.ID: publico}}}

	adminDeFuera := Quien{Sub: "root", Tenant: "globex", Admin: true}
	tocayaDeFuera := Quien{Sub: "ana", Tenant: "globex"}
	alguienDeFuera := Quien{Sub: "zoe", Tenant: "globex"}
	for _, quien := range []Quien{adminDeFuera, tocayaDeFuera, alguienDeFuera} {
		for _, v := range []domain.Video{org, borrador} {
			if _, p, err := Resolver(context.Background(), l, quien, v.ID); !errors.Is(err, domain.ErrNoExiste) {
				t.Errorf("%s de %s resuelve un vídeo %s/%s de acme: %+v %v", quien.Sub, quien.Tenant, v.Visibility, v.Status, p, err)
			}
		}
		// Lo público lo ve como cualquiera, sin editar.
		_, p, err := Resolver(context.Background(), l, quien, publico.ID)
		if err != nil || !p.Ver || p.Editar || p.Moderar {
			t.Errorf("%s de %s sobre un vídeo público de acme: %+v %v", quien.Sub, quien.Tenant, p, err)
		}
	}
}
