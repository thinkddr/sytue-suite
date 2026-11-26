// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func photoView(a model.AssetRow) api.Photo {
	asset := a.Activo()
	width, height := asset.Vistas()
	taken, captured := asset.FechaDeGaleria()
	value := api.Photo{ID: a.ID, Name: a.Name, ContentType: a.MediaType, SizeBytes: a.Bytes, Width: width, Height: height, TakenAt: taken, TakenFromEXIF: captured, Favorite: a.Favorite, Description: a.Description, Camera: a.Camera, Lens: a.Lens, Tags: slices.Clone(a.Tags), DriveFileID: a.DriveID}
	if a.DriveID == "private:"+a.ID {
		value.DriveFileID = ""
	}
	// Un original externo o un vídeo (FO-09); en las demás, cero, como siempre.
	value.DurationMs, value.Externo = a.DurationMs, rutaExternaDe(a)
	if a.Latitude != nil && a.Longitude != nil {
		value.Location = &api.PhotoPlace{Lat: *a.Latitude, Lon: *a.Longitude}
	}
	return value
}

func (r *Repository) assets(ctx context.Context) ([]model.AssetRow, error) {
	versions, err := r.engine.Selected("asset")
	if err != nil {
		return nil, err
	}
	values := make([]model.AssetRow, 0, len(versions))
	for _, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		var value model.AssetRow
		err = json.Unmarshal(raw, &value)
		clear(raw)
		if err != nil || value.Owner != r.scope.Subject {
			return nil, ErrSource
		}
		// Una foto purgada (Purgar) ya no existe para la galería, ni en la papelera.
		if value.PurgedAt != nil {
			continue
		}
		values = append(values, value)
	}
	return values, nil
}

func (r *Repository) Photo(ctx context.Context, id string) (api.Photo, error) {
	value, err := r.asset(ctx, id)
	if err != nil {
		return api.Photo{}, err
	}
	return conEstado(photoView(value)), nil
}

func (r *Repository) asset(ctx context.Context, id string) (model.AssetRow, error) {
	raw, err := r.engine.ReadRaw(ctx, "asset", id)
	defer clear(raw)
	if err != nil {
		return model.AssetRow{}, err
	}
	var value model.AssetRow
	if json.Unmarshal(raw, &value) != nil || value.ID != id || value.Tenant != r.scope.Tenant || value.Owner != r.scope.Subject {
		return model.AssetRow{}, ErrSource
	}
	if value.DeletedAt != nil {
		return model.AssetRow{}, ErrNotFound
	}
	return value, nil
}

type timelineCursor struct {
	Scope   storage.Scope `json:"scope"`
	TakenAt time.Time     `json:"taken_at"`
	ID      string        `json:"id"`
}

func (r *Repository) Timeline(ctx context.Context, cursor string, limit int, favorites bool, text string, also ...string) ([]api.Photo, string, error) {
	return r.timeline(ctx, cursor, false, limit, favorites, text, also...)
}

// timeline es Timeline; desdeFecha admite el cursor sintético de TimelineDesde, que no
// lleva id.
func (r *Repository) timeline(ctx context.Context, cursor string, desdeFecha bool, limit int, favorites bool, text string, also ...string) ([]api.Photo, string, error) {
	after, err := r.decodeTimelineCursor(cursor, desdeFecha)
	if err != nil {
		return nil, "", err
	}
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	if _, local := r.engine.VerifiedContentGeneration(); local && text == "" {
		// En el almacén local y sin texto que buscar, la página sale de la línea ya
		// proyectada (linea.go).
		return r.paginaProyectada(ctx, cursor != "", after, limit, favorites)
	}
	assets, err := r.assets(ctx)
	if err != nil {
		return nil, "", err
	}
	queries := append([]string{text}, also...)
	values := []api.Photo{}
	for _, asset := range assets {
		if asset.DeletedAt != nil || favorites && !asset.Favorite {
			continue
		}
		view := photoView(asset)
		if text != "" && !matchesPhotoText(asset, queries) {
			continue
		}
		if cursor != "" && (view.TakenAt.After(after.TakenAt) || view.TakenAt.Equal(after.TakenAt) && view.ID >= after.ID) {
			continue
		}
		values = append(values, view)
	}
	slices.SortFunc(values, compararEnLinea)
	return r.paginaDeLinea(values, limit)
}

// compararEnLinea es el orden de la línea de tiempo: de la más reciente a la más
// antigua, y por id a igual fecha.
func compararEnLinea(a, b api.Photo) int {
	if order := b.TakenAt.Compare(a.TakenAt); order != 0 {
		return order
	}
	return strings.Compare(b.ID, a.ID)
}

// paginaDeLinea corta values, ya en orden, en una página de limit con el cursor de la
// siguiente.
func (r *Repository) paginaDeLinea(values []api.Photo, limit int) ([]api.Photo, string, error) {
	next := ""
	if len(values) > limit {
		last := values[limit-1]
		raw, _ := json.Marshal(timelineCursor{Scope: r.scope, TakenAt: last.TakenAt, ID: last.ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
		values = values[:limit]
	}
	return conEstados(values), next, nil
}

func (r *Repository) decodeTimelineCursor(cursor string, desdeFecha bool) (timelineCursor, error) {
	return decodificarCursor(r.scope, cursor, desdeFecha)
}

// decodificarCursor lee el cursor de una página de la línea de tiempo de scope;
// desdeFecha admite el de TimelineDesde, sin id.
func decodificarCursor(scope storage.Scope, cursor string, desdeFecha bool) (timelineCursor, error) {
	var after timelineCursor
	if cursor == "" {
		return after, nil
	}
	if len(cursor) > 4096 {
		return after, ErrSource
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || json.Unmarshal(raw, &after) != nil || after.Scope != scope || after.ID == "" && !desdeFecha || after.TakenAt.IsZero() {
		return after, ErrSource
	}
	return after, nil
}

func matchesPhotoText(asset model.AssetRow, queries []string) bool {
	haystack := TextoDeBusqueda(asset.Name, asset.Description, asset.Tags)
	for _, q := range queries {
		if q != "" && strings.Contains(haystack, buscar.Plegar(q)) {
			return true
		}
	}
	return false
}

func (r *Repository) Papelera(ctx context.Context) ([]api.Photo, error) {
	assets, err := r.assets(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(assets, func(a, b model.AssetRow) int {
		if a.DeletedAt == nil && b.DeletedAt == nil {
			return 0
		}
		if a.DeletedAt == nil {
			return 1
		}
		if b.DeletedAt == nil {
			return -1
		}
		if order := b.DeletedAt.Compare(*a.DeletedAt); order != 0 {
			return order
		}
		return strings.Compare(b.ID, a.ID)
	})
	values := []api.Photo{}
	for _, asset := range assets {
		if asset.DeletedAt != nil {
			values = append(values, photoView(asset))
		}
	}
	return conEstados(values), nil
}

func (r *Repository) mutateAsset(ctx context.Context, operation, id string, input any, trash bool, patch func(map[string]json.RawMessage)) error {
	result, err := r.engine.Mutate(ctx, operation, input, func(ctx context.Context) (string, string, string, []byte, error) {
		raw, err := r.engine.ReadRaw(ctx, "asset", id)
		defer clear(raw)
		if err != nil {
			return "", "", "", nil, err
		}
		var row model.AssetRow
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &row) != nil || json.Unmarshal(raw, &fields) != nil || row.ID != id || row.Owner != r.scope.Subject {
			return "", "", "", nil, ErrSource
		}
		if (row.DeletedAt != nil) != trash || row.PurgedAt != nil {
			return "", "", "", nil, ErrNotFound
		}
		patch(fields)
		fields["updated_at"], _ = json.Marshal(time.Now().UTC())
		result, err := json.Marshal(fields)
		state := "live"
		if value := fields["deleted_at"]; len(value) > 0 && string(value) != "null" {
			state = "trash"
		}
		return "asset", id, state, result, err
	})
	clear(result)
	return err
}

func (r *Repository) SetFavorite(ctx context.Context, id string, favorite bool) error {
	return r.mutateAsset(ctx, "favorite", id, []any{id, favorite}, false, func(fields map[string]json.RawMessage) { fields["favorite"], _ = json.Marshal(favorite) })
}

func (r *Repository) SetDescription(ctx context.Context, id, description string, tags []string) error {
	if err := DescripcionValida(description, tags); err != nil {
		return err
	}
	return r.mutateAsset(ctx, "description", id, []any{id, description, tags}, false, func(fields map[string]json.RawMessage) {
		fields["description"], _ = json.Marshal(description)
		fields["tags"], _ = json.Marshal(tags)
	})
}

func (r *Repository) Trash(ctx context.Context, id string) error {
	return r.mutateAsset(ctx, "trash", id, id, false, func(fields map[string]json.RawMessage) { fields["deleted_at"], _ = json.Marshal(time.Now().UTC()) })
}

func (r *Repository) Restaurar(ctx context.Context, id string) error {
	return r.mutateAsset(ctx, "restore", id, id, true, func(fields map[string]json.RawMessage) { fields["deleted_at"] = json.RawMessage("null") })
}

// PhotoForDriveFile opens an already retained asset without asking the legacy
// indexer to inspect its original. New ingestion belongs to an authorized worker.
func (r *Repository) PhotoForDriveFile(ctx context.Context, file string) (api.Photo, error) {
	assets, err := r.assets(ctx)
	if err != nil {
		return api.Photo{}, err
	}
	for _, asset := range assets {
		if asset.DriveID == file && asset.DeletedAt == nil {
			return photoView(asset), nil
		}
	}
	return api.Photo{}, ErrNotFound
}
