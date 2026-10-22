// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"cmp"
	"context"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// «Pedir acceso de edición» de algo compartido en lectura (platform/pkg/peticiones):
// se le pide a quien lo compartió, que lo concede con un clic desde el sitio al que
// lleva el aviso. Al conceder, `drive.access.granted` le quita la franja a quien lo
// pidió por el flujo del Hub.

// nombreDe es el nombre del fichero o la carpeta `id`, o "" si no se encuentra.
func (s *Service) nombreDe(ctx context.Context, id uuid.UUID) string {
	if f, err := s.store.GetFile(ctx, id); err == nil {
		return f.Name
	}
	if f, err := s.store.GetFolder(ctx, id); err == nil {
		return f.Name
	}
	return ""
}

func (s *Service) avisador() peticiones.Avisador {
	return peticiones.Avisador{Hub: s.hubNotifier, IDBaseURL: s.cfg.IDBaseURL, Correo: s.cfg.Correo}
}

// PedirEdicion deja la petición de `p` sobre `recurso` y avisa la primera vez a quien
// se lo compartió. Con escritura ya, no pide nada (yaPuede); sin compartición, ErrNotFound.
func (s *Service) PedirEdicion(ctx context.Context, recurso uuid.UUID, groups []string, p peticiones.Peticion, token string) (yaPuede bool, err error) {
	sh, err := s.store.ShareForResource(ctx, p.Sujeto, groups, recurso)
	if err != nil {
		return false, err
	}
	if sh.Permission == domain.PermWrite {
		return true, nil
	}
	nueva, err := s.store.PedirEdicion(ctx, recurso, p)
	if err != nil || !nueva {
		return false, err
	}
	quien, nombre := p.Quien(), s.nombreDe(store.WithOwner(ctx, sh.CreatedBy), recurso)
	s.avisador().Avisar(ctx, peticiones.Aviso{
		Tenant: tenancy.MustFrom(ctx).Slug, Para: sh.CreatedBy, Source: "drive", Type: "drive.access.requested",
		Titulo: quien + " asks to edit “" + nombre + "”", URL: hubnotify.EnlacePeticionDrive(s.cfg.PublicURL, tenancy.MustFrom(ctx).Slug, recurso.String()),
		Recurso: recurso.String(), Params: map[string]string{"actor": quien, "name": nombre}, Token: token,
		Asunto: quien + " asks to edit “" + nombre + "”",
		Cuerpo: quien + " is asking for permission to edit “" + nombre + "”, which you shared. Open Sytue Drive to grant it with one click:",
	})
	return false, nil
}

// PeticionesEdicion son las pendientes de `recurso`. Solo quien puede conceder.
func (s *Service) PeticionesEdicion(ctx context.Context, recurso uuid.UUID, sub string) ([]peticiones.Peticion, error) {
	return s.store.PeticionesEdicion(ctx, recurso, sub)
}

// ResolverPeticion concede (escritura) o rechaza. Al conceder avisa a quien lo pidió.
func (s *Service) ResolverPeticion(ctx context.Context, recurso uuid.UUID, sub, sujeto string, concede bool) error {
	if err := s.store.ResolverPeticionEdicion(ctx, recurso, sub, sujeto, concede); err != nil {
		return err
	}
	if !concede {
		return nil
	}
	actor := ""
	if c, ok := authn.FromContext(ctx); ok {
		actor = cmp.Or(strings.TrimSpace(c.Name), c.Email)
	}
	nombre := s.nombreDe(store.WithOwner(ctx, sub), recurso)
	s.avisador().Avisar(ctx, peticiones.Aviso{
		Tenant: tenancy.MustFrom(ctx).Slug, Para: sujeto, Source: "drive", Type: "drive.access.granted",
		Titulo: "You can now edit “" + nombre + "”", URL: hubnotify.EnlaceCompartido(s.cfg.PublicURL, tenancy.MustFrom(ctx).Slug, recurso.String()),
		Recurso: recurso.String(), Params: map[string]string{"actor": actor, "name": nombre},
	})
	return nil
}

// PedidasPor son, de `recursos`, los que `sub` ya tiene pedidos.
func (s *Service) PedidasPor(ctx context.Context, sub string, recursos []uuid.UUID) (map[uuid.UUID]bool, error) {
	return s.store.PedidasPor(ctx, sub, recursos)
}
