// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// Calidades de un vídeo ya creado: verlas con lo que ocupa cada una (Gestionar) y
// cambiarlas. Lo que se añade lo genera un trabajo `qualities`; lo que se quita lo
// borra ese mismo trabajo, que es el único que reescribe el master del HLS, así
// que dos cambios seguidos no se pisan.

// CalidadesDe devuelve el vídeo (solo quien lo edita) y la estimación de cada
// calidad con su duración y dimensiones de verdad, si ya se midieron.
func (s *Servicio) CalidadesDe(ctx context.Context, quien Quien, id uuid.UUID) (domain.Video, domain.Estimacion, error) {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return domain.Video{}, domain.Estimacion{}, err
	}
	return v, EstimacionDe(v), nil
}

// EstimacionDe es la estimación de un vídeo existente.
func EstimacionDe(v domain.Video) domain.Estimacion {
	ancho, alto := 0, 0
	if v.Width != nil && v.Height != nil {
		ancho, alto = int(*v.Width), int(*v.Height)
	}
	return domain.Estimar(v.BytesOriginal, v.DurationMS, ancho, alto)
}

// ElegirCalidades cambia la elección. Tiene que haber al menos una y todas las
// ha de permitir el original. Si el vídeo está listo y lo generado ya no
// coincide con lo elegido, encola el trabajo que lo pone al día; mientras se
// sube o se procesa solo se guarda: el trabajo de la subida la leerá.
func (s *Servicio) ElegirCalidades(ctx context.Context, quien Quien, id uuid.UUID, elegidas []int) (domain.Video, error) {
	v, err := s.editable(ctx, quien, id)
	if err != nil {
		return domain.Video{}, err
	}
	qs, err := domain.ValidarCalidades(elegidas, domain.OpcionesDe(v))
	if err != nil {
		return domain.Video{}, err
	}
	return s.store.ElegirCalidades(ctx, v.Tenant, v.ID, domain.AInt32(qs), PideTrabajoDeCalidades)
}

// PideTrabajoDeCalidades dice si un vídeo necesita un trabajo `qualities`: está
// listo y lo que hay en alguna escalera no es lo elegido. La del HLS solo cuenta
// en los subidos: el HLS de una grabación es el del directo, de una calidad.
func PideTrabajoDeCalidades(v domain.Video) bool {
	if v.Status != domain.StatusReady {
		return false
	}
	objetivo := v.Elegidas()
	if !slices.Equal(ordenadas(v.CalidadesWebM()), objetivo) {
		return true
	}
	return v.Kind == domain.KindUpload && v.HlsKey != "" && !slices.Equal(ordenadas(v.CalidadesHLS()), objetivo)
}

// ordenadas de mayor a menor, como las devuelve domain.Resolver.
func ordenadas(qs []int) []int {
	out := slices.Clone(qs)
	slices.SortFunc(out, func(a, b int) int { return b - a })
	return out
}
