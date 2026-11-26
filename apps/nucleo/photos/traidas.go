// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// # Lo traído de una copia de Sytue (HU-07)
//
// «Traer desde Sytue» de Sytue Office (`nucleo/local/traer`) lleva a la fototeca las
// fotos y los álbumes de una copia de «Tus datos». Lo que sabe la copia y el fichero no
// dice se pone con lo de aquí, todo aditivo (la conectada nunca lo llama):
//
//   - las FECHAS de una foto copiada (una HEIC que entra como el JPEG que saca el
//     sistema): la de captura si su cabecera no la trae y la de subida, que es donde la
//     ponía Sytue en la línea de tiempo (PonerFechas). Las enlazadas las llevan ya al
//     enlazarlas (OrigenExterno.TomadoEn y SubidoEn);
//   - el ORIGEN de un álbum («sytue:<id>», CrearAlbumDe), que se guarda en su fila y no
//     cambia al renombrarlo: traerlo otra vez lo reconoce por él, no por su título, así
//     que ni duplica uno renombrado ni junta dos que se llaman igual (AlbumesPorOrigen).

// PonerFechas pone a la foto id su fecha de captura, si no tiene ninguna, y la de
// subida. Las nil no se tocan.
func (r *Repository) PonerFechas(ctx context.Context, id string, tomada, subida *time.Time) error {
	if tomada != nil && tomada.IsZero() {
		tomada = nil
	}
	if subida != nil && subida.IsZero() {
		subida = nil
	}
	if tomada == nil && subida == nil {
		return nil
	}
	var input []string
	for _, t := range []*time.Time{tomada, subida} {
		if t == nil {
			input = append(input, "")
		} else {
			input = append(input, t.UTC().Format(time.RFC3339Nano))
		}
	}
	return r.mutateAsset(ctx, "dates", id, append([]string{id}, input...), false, func(fields map[string]json.RawMessage) {
		if v := fields["taken_at"]; tomada != nil && (len(v) == 0 || string(v) == "null") {
			fields["taken_at"], _ = json.Marshal(tomada.UTC())
		}
		if subida != nil {
			fields["uploaded_at"], _ = json.Marshal(subida.UTC())
		}
	})
}

// CrearAlbumDe crea el álbum title con su origen (de dónde vino: «sytue:<id>»), que no
// cambia al renombrarlo.
func (r *Repository) CrearAlbumDe(ctx context.Context, title, origen string) (api.Album, error) {
	origen = strings.TrimSpace(origen)
	if origen == "" || len(origen) > 200 {
		return api.Album{}, model.ErrInvalidInput
	}
	return r.crearAlbum(ctx, title, origen)
}

// AlbumDeOrigen es un álbum que vino de fuera, visto por su origen.
type AlbumDeOrigen struct {
	ID string
	// Borrado: la persona lo borró. Traerlo otra vez no lo devuelve.
	Borrado bool
}

// AlbumesPorOrigen son los álbumes que vinieron de fuera, por su origen, también los
// borrados. Si dos tienen el mismo, cuenta el vivo.
func (r *Repository) AlbumesPorOrigen(parent context.Context) (map[string]AlbumDeOrigen, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	versions, err := r.engine.Selected("album")
	if err != nil {
		return nil, err
	}
	out := map[string]AlbumDeOrigen{}
	for _, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		var row model.AlbumRow
		err = json.Unmarshal(raw, &row)
		clear(raw)
		if err != nil || row.Owner != r.scope.Subject || row.Tenant != r.scope.Tenant {
			return nil, ErrSource
		}
		if row.Origin == "" {
			continue
		}
		if ya, ok := out[row.Origin]; ok && !ya.Borrado {
			continue
		}
		out[row.Origin] = AlbumDeOrigen{ID: row.ID, Borrado: row.DeletedAt != nil}
	}
	return out, nil
}
