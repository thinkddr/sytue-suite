// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// ExportNotebook devuelve un cuaderno con sus notas para volcarlo fuera, y deja
// constancia de que alguien se lo llevó.
//
// SE APOYA EN GetNotebook Y ListNotes en vez de escribir su propia consulta: son
// las que ya deciden quién ve qué —cuaderno propio o de la organización, y notas
// privadas solo para su dueño— y duplicar ese predicado aquí sería tener dos
// definiciones de «quién puede leer esto» que se separan a la primera.
//
// LA TRAZA ES PARTE DE LA OPERACIÓN, no un extra: si no se puede anotar quién
// exportó, no se exporta. Una copia de todo un cuaderno que no deja rastro es
// justo lo que una auditoría viene a buscar después.
func (s *Store) ExportNotebook(
	ctx context.Context, id uuid.UUID, sub string,
) (domain.Notebook, []domain.Note, error) {
	nb, err := s.GetNotebook(ctx, id, sub)
	if err != nil {
		return domain.Notebook{}, nil, err
	}
	notas, err := s.ListNotes(ctx, id, sub)
	if err != nil {
		return domain.Notebook{}, nil, err
	}

	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// `notas` y el título en el detalle: lo que hace útil la línea después es
		// saber CUÁNTO salió y de dónde, no solo que hubo una exportación.
		return writeAuditTx(ctx, tx, t.Slug, sub, domain.AuditNotebookExport, id.String(),
			map[string]any{"titulo": nb.Title, "notas": len(notas)})
	})
	if err != nil {
		return domain.Notebook{}, nil, err
	}
	return nb, notas, nil
}

// ImportNotebook crea un cuaderno con sus notas a partir de un volcado leído.
//
// CADA PIEZA PASA POR SU `Normalize` y por las funciones de creación de siempre:
// un fichero traído de fuera es entrada de usuario como cualquier otra, y saltarse
// la validación «porque lo generamos nosotros» es exactamente cómo se cuela un
// contenido que la API nunca habría aceptado.
//
// LAS NOTAS QUE NO PASEN NO ABORTAN LA IMPORTACIÓN: se cuentan y se dicen. Un
// volcado de mil notas con una corrupta es mejor restaurarlo entero menos esa que
// dejarlo a medias o rechazarlo del todo — y quien importa necesita saber cuántas
// se quedaron fuera, no descubrirlo contando.
func (s *Store) ImportNotebook(
	ctx context.Context, sub string, nb domain.Notebook, notas []domain.Note,
) (domain.Notebook, int, int, error) {
	entrada := nb.AImportar()
	if err := entrada.Normalize(); err != nil {
		return domain.Notebook{}, 0, 0, err
	}
	// Reject an encrypted legacy archive before creating even an empty notebook.
	// A client must seal private Data locally; skipping these notes would lose content.
	if entrada.Encrypted {
		for _, note := range notas {
			input := note.AImportar()
			input.ExpectedCrypto, _ = json.Marshal(entrada.Crypto)
			if err := input.Normalize(true); err != nil {
				return domain.Notebook{}, 0, 0, domain.ErrEncryptedMigrationRequired
			}
		}
	}
	creado, err := s.CreateNotebook(ctx, sub, entrada)
	if err != nil {
		return domain.Notebook{}, 0, 0, err
	}

	puestas, descartadas := 0, 0
	for _, n := range notas {
		in := n.AImportar()
		in.ExpectedCrypto, _ = json.Marshal(creado.Crypto)
		if err := in.Normalize(creado.Encrypted); err != nil {
			descartadas++
			continue
		}
		if _, err := s.CreateNote(ctx, creado.ID, sub, in); err != nil {
			descartadas++
			continue
		}
		puestas++
	}

	// LA TRAZA VA AL FINAL, con las cuentas reales: lo útil después es saber
	// cuánto entró y cuánto se quedó fuera, no solo que hubo una importación. Si
	// no se puede anotar, se devuelve el error aunque el cuaderno ya exista:
	// mejor un error con cuaderno que un cuaderno importado sin rastro.
	t := tenancy.MustFrom(ctx)
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return writeAuditTx(ctx, tx, t.Slug, sub, domain.AuditNotebookImport, creado.ID.String(),
			map[string]any{"titulo": creado.Title, "notas": puestas, "descartadas": descartadas})
	})
	if err != nil {
		return domain.Notebook{}, 0, 0, err
	}
	return creado, puestas, descartadas, nil
}
