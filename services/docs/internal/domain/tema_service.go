// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"context"
	"fmt"
)

// ResultadoTema es lo que devuelve aplicar un tema a un documento que ya existe.
type ResultadoTema struct {
	Changed bool          // false si el contenido ya lo tenía aplicado
	Seq     int64         // seq del documento tras aplicarlo
	Ops     []SequencedOp // lo escrito, para difundirlo a los colaboradores conectados
}

// AplicarTema aplica `t` al contenido vigente de un documento (requiere editor). Escribe
// UNA operación `set` del registro de contenido, por la misma puerta que cualquier edición,
// así que la protección de hojas y de secciones se cumple como siempre. `antes` se llama
// una sola vez y SOLO si hay algo que cambiar, justo antes de escribir: es donde el
// servicio guarda el punto de restauración. Si `antes` falla, no se escribe nada.
func (s *Service) AplicarTema(
	ctx context.Context, subj Subject, docID string, t TemaComun,
	hydrate func(ctx context.Context, blobKey string) (*State, error),
	antes func(ctx context.Context) error,
) (*ResultadoTema, error) {
	doc, _, err := s.Authorize(ctx, docID, subj, RoleEditor)
	if err != nil {
		return nil, err
	}
	// Sin redactar: es el estado del servicio, y la contraseña heredada de una hoja no
	// debe perderse por tematizarla.
	actual, seq, err := s.load(ctx, docID, hydrate)
	if err != nil {
		return nil, err
	}
	op, cambia, err := OpTema(doc.Kind, actual, t)
	if err != nil {
		return nil, err
	}
	if !cambia {
		return &ResultadoTema{Seq: seq}, nil
	}
	if antes != nil {
		if err := antes(ctx); err != nil {
			return nil, err
		}
	}
	seqd, nuevaSeq, err := s.appendProtegido(ctx, doc, subj, []Op{op}, nil)
	if err != nil {
		return nil, err
	}
	return &ResultadoTema{Changed: true, Seq: nuevaSeq, Ops: seqd}, nil
}

// PrevisualizarTema dice qué cambiaría AplicarTema con `t` sobre el contenido vigente, sin
// escribir nada ni dejar punto de restauración (requiere editor: es lo que va a aplicar).
func (s *Service) PrevisualizarTema(
	ctx context.Context, subj Subject, docID string, t TemaComun,
	hydrate func(ctx context.Context, blobKey string) (*State, error),
) ([]CambioTema, error) {
	doc, _, err := s.Authorize(ctx, docID, subj, RoleEditor)
	if err != nil {
		return nil, err
	}
	actual, _, err := s.load(ctx, docID, hydrate)
	if err != nil {
		return nil, err
	}
	contenido := actual.contentString(ContentKey(doc.Kind))
	if contenido == "" {
		return nil, fmt.Errorf("%w: el documento no tiene contenido que tematizar", ErrInvalidFormat)
	}
	return PrevisualizarTema(doc.Kind, contenido, t)
}
