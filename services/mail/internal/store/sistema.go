// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// AsegurarAlias deja `alias` entregando al buzón de `destino`, que tiene que
// existir ya. Es idempotente: si el alias ya entrega a esa cuenta no hace nada, y
// si entrega a OTRA no la toca y lo dice (nunca se le quita una dirección a nadie).
// `permitido` decide si la organización del destino puede tener ese alias; es el
// mismo criterio que aplica la API (domain.AliasDomainAllowed).
//
// Es para las direcciones de la suite que no son de ninguna persona —el remitente
// de Calendar, calendario@<dominio>—, que sin buzón devolvían «550 No such user» a
// sus rebotes.
func (s *Store) AsegurarAlias(ctx context.Context, alias, destino string, permitido func(ResolvedAddress) bool) error {
	dst, err := s.LookupAddress(ctx, destino)
	if err != nil {
		return fmt.Errorf("store: alias %s: el destino %s: %w", alias, destino, err)
	}
	switch ya, err := s.LookupAddress(ctx, alias); {
	case err == nil && ya.AccountID == dst.AccountID:
		return nil
	case err == nil:
		return fmt.Errorf("store: alias %s: ya entrega a otra cuenta: %w", alias, domain.ErrAliasExists)
	case !errors.Is(err, domain.ErrNoSuchUser):
		return err
	}
	if !permitido(dst) {
		return fmt.Errorf("store: alias %s: su dominio no es de la organización de %s", alias, destino)
	}
	_, err = s.CreateAlias(tenancy.Into(ctx, tenancy.Tenant{Slug: dst.Tenant}), dst.AccountID, alias)
	return err
}
