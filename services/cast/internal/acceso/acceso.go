// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package acceso decide quién ve, edita y modera un vídeo de Sytue Cast. Es la
// única regla de visibilidad del servicio: la capa HTTP, lo social y los
// listados del store la aplican, ninguno la reescribe.
//
//   - Ver: el dueño y quien administra su organización, siempre (en cualquier
//     estado, que para eso lo editan). Los demás, solo si el vídeo ya se puede
//     ver (ready o live): public y unlisted, cualquiera, con o sin sesión; org,
//     la organización del vídeo; people, las personas de su lista, de la
//     organización que sean (ADR 0040).
//   - Editar: el dueño y quien administra (owner/admin) la organización del vídeo.
//   - Moderar (borrar comentarios y chat ajenos): los mismos que editan.
//
// Los roles del token valen SOLO en la organización del vídeo: un owner de otra
// organización que está en la lista de un vídeo `people` lo ve, y nada más.
package acceso

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// Quien es quien pregunta: lo que dice su token, o nadie.
type Quien struct {
	Sub    string
	Tenant string
	// Admin: rol owner o admin en SU organización (Tenant).
	Admin   bool
	Anonimo bool
}

// Permisos es lo que quien pregunta puede hacer con el vídeo.
type Permisos struct {
	Ver, Editar, Moderar bool
}

// Lector es lo que el resolutor necesita del store.
type Lector interface {
	EnOrganizacion(ctx context.Context, tenant string, id uuid.UUID) (domain.Video, error)
	Publico(ctx context.Context, id uuid.UUID) (domain.Video, error)
	Invitado(ctx context.Context, sub string, id uuid.UUID) (domain.Video, error)
}

// Resolver encuentra el vídeo y calcula los permisos de quien pregunta. Devuelve
// domain.ErrNoExiste cuando no existe o no puede verlo, sin distinguir.
//
// Va en tres lecturas excluyentes según quién pregunta, cada una por SU política
// de RLS, para no abrir nunca una transacción que vea más de lo que toca:
// la organización del token primero (lo normal), la pública si no está allí, y la
// de invitado en último lugar.
func Resolver(ctx context.Context, st Lector, quien Quien, id uuid.UUID) (domain.Video, Permisos, error) {
	if !quien.Anonimo && quien.Tenant != "" {
		v, err := st.EnOrganizacion(ctx, quien.Tenant, id)
		if err == nil {
			if v, p, err := conPermisos(v, quien); err == nil {
				return v, p, nil
			}
			// Está en su organización y no lo ve por la regla de dentro: le queda
			// la lista de personas, que se mira abajo con su política.
		} else if !errors.Is(err, domain.ErrNoExiste) {
			return domain.Video{}, Permisos{}, err
		}
	}
	// Las dos lecturas de fuera ya las filtra la RLS, y aun así se vuelve a mirar
	// aquí: con una conexión que se la salte (superusuario o BYPASSRLS, un error
	// de despliegue) un vídeo de organización saldría por la puerta pública.
	v, err := st.Publico(ctx, id)
	if err == nil {
		if !v.Visibility.Abierta() || !v.Status.Visible() {
			return domain.Video{}, Permisos{}, domain.ErrNoExiste
		}
		return v, Permisos{Ver: true}, nil
	}
	if !errors.Is(err, domain.ErrNoExiste) || quien.Anonimo || quien.Sub == "" {
		return domain.Video{}, Permisos{}, err
	}
	v, err = st.Invitado(ctx, quien.Sub, id)
	if err != nil {
		return domain.Video{}, Permisos{}, err
	}
	if v.Visibility != domain.VisibilityPeople || !v.Status.Visible() {
		return domain.Video{}, Permisos{}, domain.ErrNoExiste
	}
	return v, Permisos{Ver: true}, nil
}

// conPermisos aplica la regla a un vídeo leído en la organización del token.
// La lista de `people` de la propia organización no se consulta aquí: quien no
// es dueño ni administra y no está en ella cae a la lectura de invitado, que la
// comprueba con su política.
func conPermisos(v domain.Video, quien Quien) (domain.Video, Permisos, error) {
	// Ser el dueño o administrar vale SOLO en la organización del vídeo. El store
	// ya lee por organización; se vuelve a mirar aquí porque esta es la regla, y
	// no puede depender de que el SQL —o la RLS— de otro paquete la sostenga: un
	// owner de OTRA organización no edita, y nadie de fuera ve un vídeo `org`.
	if v.Tenant == "" || v.Tenant != quien.Tenant {
		return domain.Video{}, Permisos{}, domain.ErrNoExiste
	}
	edita := quien.Sub == v.OwnerSub || quien.Admin
	if edita {
		return v, Permisos{Ver: true, Editar: true, Moderar: true}, nil
	}
	if !v.Status.Visible() {
		return domain.Video{}, Permisos{}, domain.ErrNoExiste
	}
	switch v.Visibility {
	case domain.VisibilityPublic, domain.VisibilityUnlisted, domain.VisibilityOrg:
		return v, Permisos{Ver: true}, nil
	case domain.VisibilityPeople:
		// Está en su organización pero no en la lista o sí: lo dice la política de
		// invitado, que es la misma para dentro y para fuera.
		return domain.Video{}, Permisos{}, domain.ErrNoExiste
	}
	return domain.Video{}, Permisos{}, domain.ErrNoExiste
}
