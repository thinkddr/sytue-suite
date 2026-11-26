// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// # Volcar la fototeca (el plan B de Sytue Office)
//
// Sytue Office pasa su fototeca del motor de registros a SQLite (`local/fototeca`, §5.6
// del plan de la edición local). Volcar le da TODO lo que guarda este repositorio, ya
// comprobado por el motor al abrirlo: cada foto con la huella y el sitio de su original y
// los ficheros de sus derivadas (también las de cada receta guardada), y cada álbum con
// sus miembros en orden, también los de fotos que están en la papelera. Solo lee, y la
// edición conectada no lo llama.

// FotoVolcada es una foto de la fototeca con todo lo que guarda.
type FotoVolcada struct {
	// Fila es la de la foto, sin lo que cita sus ficheros (PrivateMedia, PrivateExternal).
	Fila model.AssetRow
	// Huella es la del original (SHA-256 en hexadecimal, minúsculas): retenido, enlazado o
	// el que se apuntó al purgarla. Vacía si no la tiene.
	Huella string
	// Retenido: el original es un fichero del almacén, con esa huella.
	Retenido bool
	// Ruta y Modificado son los del original enlazado (FO-09); Ruta vacía si no lo es.
	Ruta       string
	Modificado time.Time
	// Derivadas son los ficheros de las derivadas: los de la receta vacía y los de cada
	// receta guardada. Vacías en una purgada.
	Derivadas []DerivadaVolcada
	// Receta es la guardada (nil: sin editar), con su huella.
	Receta       *model.Receta
	HuellaReceta string
}

// DerivadaVolcada es el fichero de una derivada de una foto.
type DerivadaVolcada struct {
	// Receta es la huella de la receta de la que sale.
	Receta   string
	Variante model.Variante
	// Formato es «jpg» o «png».
	Formato string
	// SHA256 y Bytes son los del fichero.
	SHA256 string
	Bytes  int64
}

// AlbumVolcado es un álbum, también uno borrado, con sus miembros en orden.
type AlbumVolcado struct {
	// Fila es la del álbum, sin lo que cita sus páginas (PrivateMembers).
	Fila model.AlbumRow
	// Miembros son los de fotos que no se han purgado, por posición.
	Miembros []model.MembershipRow
}

// Volcar llama a foto con cada foto (también las de la papelera y las purgadas), con
// cuántas van y cuántas hay, y después a album con cada álbum. Si una devuelve error,
// para con él.
func (r *Repository) Volcar(parent context.Context, foto func(f FotoVolcada, hechas, total int) error, album func(AlbumVolcado) error) error {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	if _, err := r.engine.Catalog(); err != nil {
		return err
	}
	versions, err := r.engine.Selected("asset")
	if err != nil {
		return err
	}
	vivas := map[string]bool{}
	for i, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return err
		}
		f, err := r.fotoVolcada(ctx, raw)
		clear(raw)
		if err != nil {
			return err
		}
		if f.Fila.PurgedAt == nil {
			vivas[f.Fila.ID] = true
		}
		if err := foto(f, i+1, len(versions)); err != nil {
			return err
		}
	}
	graph, err := r.albumGraph(ctx)
	if err != nil {
		return err
	}
	for _, row := range graph.albums {
		a := AlbumVolcado{Fila: row}
		a.Fila.PrivateMembers = nil
		for _, m := range graph.members[row.ID] {
			if vivas[m.AssetID] {
				a.Miembros = append(a.Miembros, m)
			}
		}
		if err := album(a); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// fotoVolcada es la foto de la fila raw con sus ficheros y su receta.
func (r *Repository) fotoVolcada(ctx context.Context, raw []byte) (FotoVolcada, error) {
	asset, media, err := r.ingestedAsset(raw)
	if err != nil {
		return FotoVolcada{}, err
	}
	_, externa, err := r.externalAsset(raw)
	if err != nil {
		return FotoVolcada{}, err
	}
	if asset.Owner != r.scope.Subject || asset.Tenant != r.scope.Tenant {
		return FotoVolcada{}, ErrSource
	}
	f := FotoVolcada{Huella: strings.ToLower(asset.OriginalSHA256)}
	var inicial *recipeRenditions
	switch {
	case media != nil:
		f.Huella, f.Retenido, inicial = strings.ToLower(media.Request.SHA256), true, &media.Renditions
	case externa != nil:
		f.Huella, f.Ruta, f.Modificado, inicial = strings.ToLower(externa.Request.SHA256), externa.Path, externa.ModTime, &externa.Renditions
	}
	asset.PrivateMedia, asset.PrivateExternal = nil, nil
	f.Fila = asset
	if asset.PurgedAt != nil {
		return f, nil
	}
	if inicial != nil {
		f.Derivadas = derivadasVolcadas(*inicial)
	}
	r.mediaMu.RLock()
	editID := r.editsByAsset[asset.ID]
	r.mediaMu.RUnlock()
	if editID == "" {
		return f, nil
	}
	edicion, err := r.engine.ReadRaw(ctx, "edit", editID)
	if err != nil {
		return FotoVolcada{}, err
	}
	defer clear(edicion)
	row, err := r.decodeEdit(edicion, asset.ID, editID)
	if err != nil {
		return FotoVolcada{}, err
	}
	receta := row.Recipe
	f.Receta, f.HuellaReceta = &receta, row.Fingerprint
	sets, err := renditionFields(edicion)
	if err != nil {
		return FotoVolcada{}, err
	}
	for huella, encoded := range sets {
		set, err := r.decodeRenditions(encoded, asset.ID, huella)
		if err != nil {
			return FotoVolcada{}, err
		}
		f.Derivadas = append(f.Derivadas, derivadasVolcadas(set)...)
	}
	return f, nil
}

// derivadasVolcadas son los ficheros de un juego de derivadas.
func derivadasVolcadas(set recipeRenditions) []DerivadaVolcada {
	out := make([]DerivadaVolcada, 0, len(set.Files))
	for _, variante := range model.Variantes() {
		file, ok := set.Files[variante]
		if !ok {
			continue
		}
		out = append(out, DerivadaVolcada{Receta: set.Fingerprint, Variante: variante, Formato: set.Format, SHA256: strings.ToLower(file.Row.SHA256), Bytes: file.Row.Size})
	}
	return out
}
