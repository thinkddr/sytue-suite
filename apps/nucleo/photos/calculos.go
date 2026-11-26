// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// Lo que el servicio de Photos calcula con su índice y aquí se calcula sobre la galería
// del dispositivo (PLAN-EDICION-LOCAL.md §6.8, FO-02): saltar a un año, los años de la
// línea de tiempo, los recuerdos, las repetidas y «Por aquí». Las reglas son las del
// servicio (`services/syphotos/internal/store`): mismas fechas, mismos topes, mismo
// radio. La galería personal entera vive ya en memoria en el motor de registros, así que
// recorrerla cuesta lo que recorrer la línea de tiempo.

// Topes de los cálculos, los del servicio.
const (
	// MaxPorRecuerdo: un recuerdo es una invitación a mirar, no la carpeta entera.
	MaxPorRecuerdo = 12
	// RadioDeCerca es hasta dónde llega «Por aquí», en kilómetros: el mismo sitio.
	RadioDeCerca = 2.0
	// TopeDeCerca acota la respuesta de «Por aquí».
	TopeDeCerca = 100
)

// fotoLocal es una foto viva de la galería con la huella de su original (vacía si no
// entró por la ingesta directa y no la tiene).
type fotoLocal struct {
	asset  model.AssetRow
	vista  api.Photo
	huella string
}

// vivas son las fotos fuera de la papelera, de más reciente a más antigua por la fecha
// de la galería (la que ordena la línea de tiempo).
func (r *Repository) vivas(parent context.Context, soloFavoritas bool) ([]fotoLocal, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	if _, err := r.engine.Catalog(); err != nil {
		return nil, err
	}
	versions, err := r.engine.Selected("asset")
	if err != nil {
		return nil, err
	}
	out := make([]fotoLocal, 0, len(versions))
	for _, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		asset, huella, err := r.huellaDeFila(raw)
		clear(raw)
		if err != nil {
			return nil, err
		}
		if asset.Owner != r.scope.Subject || asset.DeletedAt != nil || asset.PurgedAt != nil || soloFavoritas && !asset.Favorite {
			continue
		}
		f := fotoLocal{asset: asset, vista: photoView(asset), huella: huella}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b fotoLocal) int {
		if order := b.vista.TakenAt.Compare(a.vista.TakenAt); order != 0 {
			return order
		}
		return strings.Compare(b.vista.ID, a.vista.ID)
	})
	return out, ctx.Err()
}

// limiteDeFecha convierte `desde` —`2019`, `2019-07` o `2019-07-14`— en el primer
// instante del periodo siguiente, en UTC como la fecha guardada: la página empieza por
// la foto más reciente de ese periodo (`cursorDeFecha` del servicio).
func limiteDeFecha(desde string) (time.Time, error) {
	desde = strings.TrimSpace(desde)
	var (
		t   time.Time
		err error
	)
	switch len(desde) {
	case 4:
		t, err = time.Parse("2006", desde)
		t = t.AddDate(1, 0, 0)
	case 7:
		t, err = time.Parse("2006-01", desde)
		t = t.AddDate(0, 1, 0)
	case 10:
		t, err = time.Parse("2006-01-02", desde)
		t = t.AddDate(0, 0, 1)
	default:
		return time.Time{}, fmt.Errorf("%w: fecha %q, se espera AAAA, AAAA-MM o AAAA-MM-DD", model.ErrInvalidInput, desde)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: fecha %q ilegible", model.ErrInvalidInput, desde)
	}
	return t, nil
}

// TimelineDesde es la primera página de la línea de tiempo empezando en una fecha. Las
// siguientes se piden con Timeline y el cursor que devuelve, como siempre.
func (r *Repository) TimelineDesde(ctx context.Context, desde string, limit int, favorites bool) ([]api.Photo, string, error) {
	limite, err := limiteDeFecha(desde)
	if err != nil {
		return nil, "", err
	}
	// Un cursor con id vacío hace la comparación estricta: (fecha, id) < (límite, "")
	// es fecha < límite, y una foto hecha a las 00:00 del periodo siguiente no se cuela.
	raw, err := json.Marshal(timelineCursor{Scope: r.scope, TakenAt: limite})
	if err != nil {
		return nil, "", err
	}
	return r.timeline(ctx, base64.RawURLEncoding.EncodeToString(raw), true, limit, favorites, "")
}

// TimelineYears son los años con fotos, del más reciente al más antiguo, con la MISMA
// fecha que ordena la línea (en UTC): si contase con otra, un año podría ofrecerse y
// abrir otro.
func (r *Repository) TimelineYears(ctx context.Context, favorites bool) ([]api.TimelineYear, error) {
	fotos, err := r.vivas(ctx, favorites)
	if err != nil {
		return nil, err
	}
	return anyosDe(fotos), nil
}

// anyosDe cuenta por año las fotos, que vienen ya de más reciente a más antigua.
func anyosDe(fotos []fotoLocal) []api.TimelineYear {
	out := []api.TimelineYear{}
	for _, f := range fotos {
		anyo := f.vista.TakenAt.UTC().Year()
		if n := len(out); n > 0 && out[n-1].Year == anyo {
			out[n-1].Count++
			continue
		}
		out = append(out, api.TimelineYear{Year: anyo, Count: 1})
	}
	return out
}

// Memories son las fotos hechas un día como hoy en años anteriores, por año y del más
// reciente al más antiguo, con doce como mucho de cada año.
//
// Se compara mes y día, no «hace 365 días», y solo cuenta la fecha de CAPTURA: una foto
// colocada por su fecha de subida no se hizo ese día (`Recuerdos` del servicio). «Hoy»
// es el día de este equipo; la fecha de captura es la hora de pared del EXIF, que se
// guarda tal cual en UTC.
func (r *Repository) Memories(ctx context.Context) ([]api.Memory, error) {
	fotos, err := r.vivas(ctx, false)
	if err != nil {
		return nil, err
	}
	return recuerdosDe(fotos, time.Now()), nil
}

// recuerdosDe son los recuerdos de hoy entre fotos, de más reciente a más antigua.
func recuerdosDe(fotos []fotoLocal, hoy time.Time) []api.Memory {
	out := []api.Memory{}
	for _, f := range fotos {
		if !f.vista.TakenFromEXIF {
			continue
		}
		cuando := f.vista.TakenAt.UTC()
		if cuando.Month() != hoy.Month() || cuando.Day() != hoy.Day() || cuando.Year() >= hoy.Year() {
			continue
		}
		if n := len(out); n == 0 || out[n-1].Year != cuando.Year() {
			out = append(out, api.Memory{Year: cuando.Year()})
		}
		if m := &out[len(out)-1]; len(m.Items) < MaxPorRecuerdo {
			m.Items = append(m.Items, f.vista)
		}
	}
	return out
}

// Duplicates son los grupos de fotos con el MISMO original (la misma huella SHA-256):
// el mismo fichero importado dos veces, desde dos carpetas o con dos nombres. Propone
// y no borra. Cada grupo va con la primera que se importó delante, y los grupos de la
// foto más reciente a la más antigua. Las que no tienen huella (las indexadas desde
// Drive en la edición conectada) no se comparan.
func (r *Repository) Duplicates(ctx context.Context, limite int) ([][]api.Photo, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	fotos, err := r.vivas(ctx, false)
	if err != nil {
		return nil, err
	}
	return repetidasDe(fotos, limite), nil
}

// repetidasDe agrupa por huella las fotos, que vienen de más reciente a más antigua.
func repetidasDe(fotos []fotoLocal, limite int) [][]api.Photo {
	porHuella := map[string][]fotoLocal{}
	var orden []string
	for _, f := range fotos {
		if f.huella == "" {
			continue
		}
		if _, ya := porHuella[f.huella]; !ya {
			orden = append(orden, f.huella)
		}
		porHuella[f.huella] = append(porHuella[f.huella], f)
	}
	grupos := [][]api.Photo{}
	for _, h := range orden {
		grupo := porHuella[h]
		if len(grupo) < 2 {
			continue
		}
		slices.SortStableFunc(grupo, func(a, b fotoLocal) int {
			if order := a.asset.UploadedAt.Compare(b.asset.UploadedAt); order != 0 {
				return order
			}
			return strings.Compare(a.vista.ID, b.vista.ID)
		})
		vistas := make([]api.Photo, len(grupo))
		for i, f := range grupo {
			vistas[i] = f.vista
		}
		grupos = append(grupos, vistas)
		if len(grupos) >= limite {
			break
		}
	}
	return grupos
}

// Nearby son las fotos hechas a menos de RadioDeCerca de la dada, de la más próxima a
// la más lejana y sin ella. Una foto sin coordenadas no es un error: la respuesta es una
// lista vacía (`CercaDe` del servicio).
func (r *Repository) Nearby(ctx context.Context, id string, limite int) ([]api.Photo, error) {
	if limite <= 0 || limite > TopeDeCerca {
		limite = 48
	}
	fotos, err := r.vivas(ctx, false)
	if err != nil {
		return nil, err
	}
	return cercaDe(fotos, id, limite)
}

// cercaDe son las de fotos a menos de RadioDeCerca de la que tiene ese id.
func cercaDe(fotos []fotoLocal, id string, limite int) ([]api.Photo, error) {
	var centro *api.PhotoPlace
	for _, f := range fotos {
		if f.vista.ID == id {
			centro = f.vista.Location
			if centro == nil {
				return []api.Photo{}, nil
			}
			break
		}
	}
	if centro == nil {
		return nil, ErrNotFound
	}
	type medida struct {
		foto api.Photo
		km   float64
	}
	var cerca []medida
	for _, f := range fotos {
		lugar := f.vista.Location
		if f.vista.ID == id || lugar == nil {
			continue
		}
		if km := DistanciaKm(centro.Lat, centro.Lon, lugar.Lat, lugar.Lon); km <= RadioDeCerca {
			cerca = append(cerca, medida{f.vista, km})
		}
	}
	// Estable: dos fotos a la MISMA distancia (una ráfaga) no se barajan al recargar.
	slices.SortStableFunc(cerca, func(a, b medida) int {
		switch {
		case a.km < b.km:
			return -1
		case a.km > b.km:
			return 1
		}
		return 0
	})
	out := make([]api.Photo, 0, min(len(cerca), limite))
	for _, m := range cerca[:min(len(cerca), limite)] {
		out = append(out, m.foto)
	}
	return out, nil
}

// DistanciaKm es la fórmula del haversine sobre una Tierra esférica: el error frente a
// un elipsoide (un 0,3 %) es menor que el del GPS de un móvil.
func DistanciaKm(lat1, lon1, lat2, lon2 float64) float64 {
	const radioTierra = 6371.0
	rad := func(g float64) float64 { return g * math.Pi / 180 }
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return radioTierra * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// Purgar borra para siempre una foto de la papelera, o la papelera entera con id vacío,
// y devuelve cuántas se han ido. La foto deja de existir para la galería (ni papelera,
// ni repetidas, ni álbumes); el motor de registros no tiene recolector, así que su
// historial y su original siguen en el almacén hasta que se compacte.
func (r *Repository) Purgar(ctx context.Context, id string) (int, error) {
	ids := []string{id}
	if id == "" {
		papelera, err := r.Papelera(ctx)
		if err != nil {
			return 0, err
		}
		ids = ids[:0]
		for _, f := range papelera {
			ids = append(ids, f.ID)
		}
	}
	hechas := 0
	for _, uno := range ids {
		err := r.mutateAsset(ctx, "purge", uno, uno, true, func(fields map[string]json.RawMessage) {
			fields["purged_at"], _ = json.Marshal(time.Now().UTC())
			if r.liberarAlPurgar {
				soltarOriginal(fields)
			}
		})
		if err != nil {
			return hechas, err
		}
		hechas++
	}
	return hechas, nil
}

// PonerPortada elige la foto de portada de un álbum. La foto tiene que estar en él; el
// título no cambia (el servicio lo pide en cada PATCH, aquí sobra).
func (r *Repository) PonerPortada(parent context.Context, id, _, foto string) error {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return err
	}
	graph, err := r.albumGraph(ctx)
	done()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(graph.photos(id), func(p api.Photo) bool { return p.ID == foto }) {
		return model.ErrInvalidInput
	}
	return r.mutateAlbum(parent, "album-cover-v1", id, []string{id, foto}, func(fields map[string]json.RawMessage) {
		fields["cover_asset_id"], _ = json.Marshal(foto)
	})
}

var (
	_ api.PhotosCalendarData   = (*Repository)(nil)
	_ api.PhotosDiscoveryData  = (*Repository)(nil)
	_ api.PhotosPurgeData      = (*Repository)(nil)
	_ api.PhotosAlbumCoverData = (*Repository)(nil)
)

// soltarOriginal quita de la fila de una foto purgada lo que cita sus ficheros (el
// original retenido o el externo, con sus derivadas) y deja su huella (LiberarAlPurgar).
func soltarOriginal(fields map[string]json.RawMessage) {
	var con struct {
		Request struct {
			SHA256 string `json:"sha256"`
		} `json:"request"`
	}
	for _, campo := range []string{ingestedMediaField, externalMediaField} {
		raw, ok := fields[campo]
		if !ok {
			continue
		}
		if json.Unmarshal(raw, &con) == nil && con.Request.SHA256 != "" {
			fields["original_sha256"], _ = json.Marshal(strings.ToLower(con.Request.SHA256))
			delete(fields, campo)
		}
	}
}
