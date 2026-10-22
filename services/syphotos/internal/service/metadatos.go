// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
)

// ES UN RELLENO CON FECHA DE CADUCIDAD. El cliente de Drive leía `contentType` y
// `sizeBytes` cuando Drive contesta `content_type` y `size_bytes`, así que toda foto
// indexada hasta el arreglo se guardó con el tipo vacío y el tamaño a 0.
//
// Photos y Drive viven en bases distintas: no hay un `UPDATE` que cruce, y leer de
// Drive pide el token de la dueña. Por eso se repara al paso, con su sesión, cuando
// abre su galería. Se retira cuando
//
//	SELECT count(*) FROM assets WHERE media_type = '' OR bytes = 0
//
// dé 0 en producción, o solo queden fotos cuyo original ya no está en Drive.

// loteMetadatos es cuántas fotos se leen de la base por vuelta.
const loteMetadatos = 200

// Completado resume una pasada de CompletarMetadatos.
type Completado struct {
	// Arreglados son las fotos que ya tienen tipo y tamaño.
	Arreglados int
	// SinOriginal son las que Drive ya no tiene o la persona ya no ve: se quedan como
	// están, porque no hay de dónde sacar el dato.
	SinOriginal int
}

// CompletarMetadatos rellena el tipo y el tamaño de las fotos de `duenyo` que se
// indexaron sin ellos, preguntándole a Drive con su token.
//
// Se para en el PRIMER error que no sea «no está»: con el token caducado o Drive
// caído fallarían todas, y seguir sería martillear a Drive para nada. Lo que quede
// se arregla en la siguiente pasada.
func (i *Ingesta) CompletarMetadatos(ctx context.Context, token, duenyo string) (Completado, error) {
	var c Completado
	despues := uuid.Nil
	for {
		pendientes, err := i.store.SinTipoNiTamanyo(ctx, duenyo, despues, loteMetadatos)
		if err != nil {
			return c, err
		}
		for _, p := range pendientes {
			despues = p.ID
			f, err := i.drive.Metadatos(ctx, token, p.DriveID)
			if errors.Is(err, drive.ErrNoEncontrado) {
				c.SinOriginal++
				continue
			}
			if err != nil {
				return c, err
			}
			if f.ContentType == "" && f.Bytes == 0 {
				c.SinOriginal++
				continue
			}
			err = i.store.CompletarTipoYTamanyo(ctx, p.ID, duenyo, f.ContentType, f.Bytes)
			if errors.Is(err, domain.ErrNotFound) {
				// Otra pasada o una reindexación llegó antes: ya está bien.
				continue
			}
			if err != nil {
				return c, err
			}
			c.Arreglados++
		}
		if len(pendientes) < loteMetadatos {
			return c, nil
		}
	}
}
