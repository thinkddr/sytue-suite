// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"
)

// RegistrarCaptchaResuelto apunta una firma de reto ALTCHA ya usada y devuelve si
// era NUEVA. Es el almacén de repeticiones COMPARTIDO entre réplicas que el
// verificador ALTCHA (altcha.ConRepeticiones) usa en lugar de su memoria de
// proceso: así una solución de proof-of-work no se puede reusar una vez por cada
// réplica (ID-01). Firma la interfaz altcha.Repeticiones.
//
// INSERT … ON CONFLICT DO NOTHING: si la firma ya estaba, RowsAffected es 0 y la
// solución se rechaza como repetida. Las caducadas las barre CleanupExpired.
func (s *Store) RegistrarCaptchaResuelto(ctx context.Context, firma string, caduca time.Time) (bool, error) {
	tag, err := s.DB().Exec(ctx,
		`INSERT INTO captcha_used (signature, expires_at) VALUES ($1, $2)
		 ON CONFLICT (signature) DO NOTHING`, firma, caduca)
	if err != nil {
		return false, errf("RegistrarCaptchaResuelto", err)
	}
	return tag.RowsAffected() > 0, nil
}
