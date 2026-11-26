// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Photos es el cliente de Sytue Photos (services/syphotos, `/api/v1`). OJO: esta API
// habla en camelCase («takenAt», «nextCursor»), al revés que el resto de la suite.
type Photos struct {
	c    *Client
	data PhotosData
}

// NewPhotos crea el cliente sobre la raíz del servicio (ServiceURL(apex, "photos")).
func NewPhotos(servicio string, s Session, h *http.Client) *Photos {
	return &Photos{c: New(servicio+"/api/v1", s, h)}
}

// Las variantes de una foto que genera el servicio, por su lado mayor.
const (
	PhotoThumb   = "miniatura" // 512 px
	PhotoPreview = "previa"    // 2048 px
	// PhotoLarge es la del visor: la que pide `urlDeVariante(id, 'grande')` en la web.
	PhotoLarge = "grande"
)

// Photo es una foto de la galería.
type Photo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	TakenAt     time.Time `json:"takenAt"`
	// TakenFromEXIF: la fecha viene del EXIF de la foto. Sin él, `TakenAt` es la fecha
	// de SUBIDA, y la galería lo dice en la cabecera del mes («21 por fecha de
	// subida»): una descarga vieja parecería de hoy sin explicación.
	TakenFromEXIF bool   `json:"takenAtFromExif"`
	Favorite      bool   `json:"favorite"`
	Description   string `json:"description,omitempty"`
	Camera        string `json:"camera,omitempty"`
	Lens          string `json:"lens,omitempty"`
	// Tags son las etiquetas que Sytue AI vio en la foto, junto a la descripción.
	Tags []string `json:"tags,omitempty"`
	// Location es dónde se hizo, del GPS del EXIF; nil si la foto no lo trae.
	Location *PhotoPlace `json:"location,omitempty"`
	// DriveFileID es el original en Sytue Drive, que es donde viven los ficheros.
	DriveFileID string `json:"driveFileId"`
	// DeletedAt y PurgeAt solo vienen en la papelera: cuándo se llevó allí y cuándo
	// la borra para siempre el servicio. PurgeAt nil es que la organización no
	// caduca la papelera, y entonces no hay fecha que prometer.
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
	PurgeAt   *time.Time `json:"purgeAt,omitempty"`
	// DurationMs es la duración de un vídeo (ContentType «video/…»), en milisegundos;
	// 0 en una foto. Solo la da la fototeca de Sytue Office (FO-09).
	DurationMs int64 `json:"durationMs,omitempty"`
	// Externo es la ruta del original cuando vive FUERA de la fototeca, en una carpeta
	// del equipo enlazada sin copiarlo (FO-09); vacío, lo de siempre. EstadoExterno dice
	// si ese fichero ya no está (ExternoFalta) o ya no es el mismo (ExternoCambiado). No
	// viajan: los pone el repositorio del dispositivo al leer.
	Externo       string `json:"-"`
	EstadoExterno string `json:"-"`
}

// Los estados de un original externo (Photo.EstadoExterno). Vacío: está en su sitio.
const (
	ExternoFalta    = "falta"
	ExternoCambiado = "cambiado"
)

// EsVideo dice si es un vídeo de la galería.
func (f Photo) EsVideo() bool { return strings.HasPrefix(f.ContentType, "video/") }

// DiasParaPurga son los días que le quedan en la papelera, redondeados hacia arriba
// (a 30 h de la purga quedan «2 días», no 1), y 0 si ya le toca: el servicio purga
// por lotes y puede llegar un poco tarde. ok es false si no hay fecha de purga.
func (f Photo) DiasParaPurga(ahora time.Time) (dias int, ok bool) {
	if f.PurgeAt == nil {
		return 0, false
	}
	falta := f.PurgeAt.Sub(ahora)
	if falta <= 0 {
		return 0, true
	}
	return int((falta + 24*time.Hour - 1) / (24 * time.Hour)), true
}

// PhotoPlace son las coordenadas de una foto.
type PhotoPlace struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Timeline devuelve una página de la galería, de más nueva a más vieja. cursor vacío es
// la primera; siguiente vacío, la última. Con soloFavoritas es la MISMA línea filtrada, y
// con `texto` la misma acotada a lo que coincida por nombre o descripción. `tambien` son
// los términos con los que Sytue AI amplía una búsqueda que no encontró nada.
func (p *Photos) Timeline(ctx context.Context, cursor string, limite int, soloFavoritas bool, texto string, tambien ...string) (fotos []Photo, siguiente string, err error) {
	if p.data != nil {
		return p.data.Timeline(ctx, cursor, limite, soloFavoritas, texto, tambien...)
	}
	return p.linea(ctx, cursor, "", limite, soloFavoritas, texto, tambien)
}

// TimelineDesde es la PRIMERA página de la galería empezando en una fecha —`2019`,
// `2019-07` o `2019-07-14`—: la foto más reciente de ese periodo y hacia atrás. Las
// páginas siguientes se piden con Timeline y el cursor que devuelve, como siempre.
func (p *Photos) TimelineDesde(ctx context.Context, desde string, limite int, soloFavoritas bool) (fotos []Photo, siguiente string, err error) {
	if p.data != nil {
		if d, ok := p.data.(PhotosCalendarData); ok {
			return d.TimelineDesde(ctx, desde, limite, soloFavoritas)
		}
		// Saltar a una fecha lo resuelve el índice del servicio, que no tiene la
		// galería privada.
		return nil, "", ErrPrivatePhotosOperation
	}
	return p.linea(ctx, "", desde, limite, soloFavoritas, "", nil)
}

func (p *Photos) linea(ctx context.Context, cursor, desde string, limite int, soloFavoritas bool, texto string, tambien []string) (fotos []Photo, siguiente string, err error) {
	q := url.Values{"limit": {strconv.Itoa(limite)}}
	if len(tambien) > 0 {
		q.Set("tambien", strings.Join(tambien, ","))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	} else if desde != "" {
		q.Set("desde", desde)
	}
	if soloFavoritas {
		q.Set("favoritas", "1")
	}
	if texto != "" {
		q.Set("q", texto)
	}
	r, err := JSON[struct {
		Items      []Photo `json:"items"`
		NextCursor string  `json:"nextCursor"`
	}](ctx, p.c, Request{Path: "/timeline", Query: q})
	return r.Items, r.NextCursor, err
}

// TimelineYear es un año de la galería con cuántas fotos tiene.
type TimelineYear struct {
	Year  int `json:"year"`
	Count int `json:"count"`
}

// TimelineYears son los años con fotos, del más reciente al más antiguo: la barra para
// saltar a una fecha. Con soloFavoritas, los de «Favoritas».
func (p *Photos) TimelineYears(ctx context.Context, soloFavoritas bool) ([]TimelineYear, error) {
	if p.data != nil {
		if d, ok := p.data.(PhotosCalendarData); ok {
			return d.TimelineYears(ctx, soloFavoritas)
		}
		return nil, ErrPrivatePhotosOperation
	}
	q := url.Values{}
	if soloFavoritas {
		q.Set("favoritas", "1")
	}
	r, err := JSON[struct {
		Years []TimelineYear `json:"years"`
	}](ctx, p.c, Request{Path: "/timeline/years", Query: q})
	return r.Years, err
}

// Indexar mete en la galería un fichero que YA está en Sytue Drive y devuelve la foto.
// Los originales viven en Drive (decisión de almacén de Sytue Fotos), así que subir es
// de dos pasos: primero el fichero entra en la carpeta «Fotos» del Drive de la persona
// y luego se indexa con el id que dio Drive, igual que hace client/subida.ts.
//
// ES IDEMPOTENTE: indexar dos veces el mismo fichero no duplica nada.
func (p *Photos) Indexar(ctx context.Context, ficheroDrive string) (Photo, error) {
	if p.data != nil {
		if ingest, ok := p.data.(PhotosIngestData); ok {
			return ingest.Indexar(ctx, ficheroDrive)
		}
		return Photo{}, ErrPrivatePhotosOperation
	}
	cuerpo, err := json.Marshal(map[string]string{"driveFileId": ficheroDrive})
	if err != nil {
		return Photo{}, err
	}
	return JSON[Photo](ctx, p.c, Request{
		Method: http.MethodPost, Path: "/assets",
		Body: cuerpo, Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Variant es la imagen de una foto en ese tamaño (JPEG, o PNG si la original lo era).
// Cerrarla es cosa de quien llama.
func (p *Photos) Variant(ctx context.Context, id, variante string) (io.ReadCloser, error) {
	return p.VariantDe(ctx, id, variante, "")
}

// VariantDe es Variant con la HUELLA de la receta (`?f=`): la que devolvió Rotar. Sin
// ella el servicio sirve la última que tenga guardada, que tras girar es la de antes.
func (p *Photos) VariantDe(ctx context.Context, id, variante, huella string) (io.ReadCloser, error) {
	if p.data != nil {
		if media, ok := p.data.(PhotosMediaData); ok {
			return media.VariantDe(ctx, id, variante, huella)
		}
		return nil, ErrPrivatePhotosOperation
	}
	var q url.Values
	if huella != "" {
		q = url.Values{"f": {huella}}
	}
	return p.imagen(ctx, "/assets/"+url.PathEscape(id)+"/variants/"+url.PathEscape(variante), q)
}

// Export es la foto con su receta aplicada en ese formato (`jpeg` o `png`), la de
// «Descargar». Cerrarla es cosa de quien llama.
func (p *Photos) Export(ctx context.Context, id, formato string) (io.ReadCloser, error) {
	if p.data != nil {
		exporter, ok := p.data.(PhotosExportData)
		if !ok {
			return nil, ErrPrivatePhotosOperation
		}
		return exporter.Export(ctx, id, formato)
	}
	return p.imagen(ctx, "/assets/"+url.PathEscape(id)+"/export", url.Values{"formato": {formato}})
}

func (p *Photos) imagen(ctx context.Context, ruta string, q url.Values) (io.ReadCloser, error) {
	resp, err := p.c.Do(ctx, Request{Path: ruta, Query: q, Header: http.Header{"Accept": {"image/*"}}})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Photo es una foto por su id: la de partida de «parecidas» y «por aquí».
func (p *Photos) Photo(ctx context.Context, id string) (Photo, error) {
	if p.data != nil {
		return p.data.Photo(ctx, id)
	}
	return JSON[Photo](ctx, p.c, Request{Path: "/assets/" + url.PathEscape(id)})
}

// SetDescription guarda la descripción y las etiquetas: el texto alternativo de la
// foto, que es lo que también se busca.
func (p *Photos) SetDescription(ctx context.Context, id, descripcion string, etiquetas []string) error {
	if p.data != nil {
		return p.data.SetDescription(ctx, id, descripcion, etiquetas)
	}
	if etiquetas == nil {
		etiquetas = []string{}
	}
	return p.mandar(ctx, http.MethodPut, "/assets/"+url.PathEscape(id)+"/description",
		map[string]any{"description": descripcion, "tags": etiquetas})
}

// Rotar gira la foto 90° a la derecha: un paso «giro» más al final de su receta, que el
// servicio guarda entera. Devuelve la huella nueva, que es con la que se vuelve a pedir
// la imagen (VariantDe).
func (p *Photos) Rotar(ctx context.Context, id string) (string, error) {
	pending, err := p.EdicionPendiente(ctx, id)
	if err != nil {
		return "", err
	}
	if pending != nil {
		return p.GuardarRecetaDesde(ctx, id, pending.Receta, pending.Base)
	}
	r, err := p.LeerReceta(ctx, id)
	if err != nil {
		return "", err
	}
	base := r.Huella()
	r.Pasos = append(r.Pasos, PasoReceta{Tipo: PasoGiro, Params: map[string]float64{"grados": 90}})
	return p.GuardarRecetaDesde(ctx, id, r, base)
}

// Similar son las fotos que más se parecen a una, por el vector de la imagen.
func (p *Photos) Similar(ctx context.Context, id string, limite int) ([]Photo, error) {
	return p.lista(ctx, "/assets/"+url.PathEscape(id)+"/similar", url.Values{"limit": {strconv.Itoa(limite)}})
}

// SimilarMany son las que se parecen a VARIAS a la vez (hasta doce, como la web).
func (p *Photos) SimilarMany(ctx context.Context, ids []string, limite int) ([]Photo, error) {
	return p.lista(ctx, "/similar", url.Values{
		"ids": {strings.Join(ids[:min(len(ids), 12)], ",")}, "limit": {strconv.Itoa(limite)},
	})
}

// Nearby son las hechas cerca de donde se hizo esa.
func (p *Photos) Nearby(ctx context.Context, id string, limite int) ([]Photo, error) {
	if d, ok := p.data.(PhotosDiscoveryData); ok {
		return d.Nearby(ctx, id, limite)
	}
	return p.lista(ctx, "/assets/"+url.PathEscape(id)+"/nearby", url.Values{"limit": {strconv.Itoa(limite)}})
}

func (p *Photos) lista(ctx context.Context, ruta string, q url.Values) ([]Photo, error) {
	r, err := JSON[struct {
		Items []Photo `json:"items"`
	}](ctx, p.c, Request{Path: ruta, Query: q})
	return r.Items, err
}

// Themes son los montones de fotos que se parecen entre sí: las sugerencias de álbum.
func (p *Photos) Themes(ctx context.Context) ([][]Photo, error) {
	r, err := JSON[struct {
		Themes []struct {
			Items []Photo `json:"items"`
		} `json:"themes"`
	}](ctx, p.c, Request{Path: "/themes"})
	montones := make([][]Photo, 0, len(r.Themes))
	for _, m := range r.Themes {
		montones = append(montones, m.Items)
	}
	return montones, err
}

// Memory son las fotos de un día como hoy de un año anterior.
type Memory struct {
	Year  int     `json:"year"`
	Items []Photo `json:"items"`
}

// Memories son los recuerdos de hoy, del año más reciente al más antiguo.
func (p *Photos) Memories(ctx context.Context) ([]Memory, error) {
	if d, ok := p.data.(PhotosCalendarData); ok {
		return d.Memories(ctx)
	}
	r, err := JSON[struct {
		Memories []Memory `json:"memories"`
	}](ctx, p.c, Request{Path: "/memories"})
	return r.Memories, err
}

// mandar hace una escritura con cuerpo JSON y tira la respuesta.
func (p *Photos) mandar(ctx context.Context, metodo, ruta string, cuerpo any) error {
	var datos []byte
	if cuerpo != nil {
		var err error
		if datos, err = json.Marshal(cuerpo); err != nil {
			return err
		}
	}
	resp, err := p.c.Do(ctx, Request{
		Method: metodo, Path: ruta, Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// SetFavorite marca o desmarca una foto como favorita.
func (p *Photos) SetFavorite(ctx context.Context, id string, favorita bool) error {
	if p.data != nil {
		return p.data.SetFavorite(ctx, id, favorita)
	}
	cuerpo, err := json.Marshal(map[string]bool{"favorite": favorita})
	if err != nil {
		return err
	}
	resp, err := p.c.Do(ctx, Request{
		Method: http.MethodPut, Path: "/assets/" + url.PathEscape(id) + "/favorite",
		Body: cuerpo, Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Trash manda una foto a la papelera. El fichero sigue en Sytue Drive.
func (p *Photos) Trash(ctx context.Context, id string) error {
	if p.data != nil {
		return p.data.Trash(ctx, id)
	}
	resp, err := p.c.Do(ctx, Request{Method: http.MethodDelete, Path: "/assets/" + url.PathEscape(id)})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// FotosEnPapelera es la papelera con su plazo. RetentionDays son los días que pasa
// una foto allí antes de borrarse para siempre; 0, la organización no la caduca y
// solo se vacía a mano.
type FotosEnPapelera struct {
	Items         []Photo `json:"items"`
	RetentionDays int     `json:"retentionDays"`
}

// Papelera son las fotos quitadas de la galería, que siguen ahí hasta que alguien las
// restaura, las borra para siempre o caduca su plazo. No pagina: el servicio las
// devuelve todas.
func (p *Photos) Papelera(ctx context.Context) (FotosEnPapelera, error) {
	if p.data != nil {
		items, err := p.data.Papelera(ctx)
		return FotosEnPapelera{Items: items}, err
	}
	return JSON[FotosEnPapelera](ctx, p.c, Request{Path: "/trash"})
}

// Restaurar devuelve una foto de la papelera a la galería.
func (p *Photos) Restaurar(ctx context.Context, id string) error {
	if p.data != nil {
		return p.data.Restaurar(ctx, id)
	}
	resp, err := p.c.Do(ctx, Request{Method: http.MethodPost, Path: "/assets/" + url.PathEscape(id) + "/restore"})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// RestaurarTodo devuelve a la galería TODO lo que hay en la papelera y dice cuántas
// (`POST /trash/restore`, una sentencia en el servicio). Con el almacén privado no hay
// ruta de lote: se restauran una a una, que es lo que el almacén sabe hacer.
func (p *Photos) RestaurarTodo(ctx context.Context) (int, error) {
	if p.data != nil {
		items, err := p.data.Papelera(ctx)
		if err != nil {
			return 0, err
		}
		n := 0
		for _, f := range items {
			if err := p.data.Restaurar(ctx, f.ID); err != nil {
				return n, err
			}
			n++
		}
		return n, nil
	}
	r, err := JSON[struct {
		Restored int `json:"restored"`
	}](ctx, p.c, Request{Method: http.MethodPost, Path: "/trash/restore"})
	return r.Restored, err
}

// Purgar borra para siempre. Con id vacío vacía la papelera entera. Devuelve cuántas se
// han ido, que es lo que se le dice a la persona. El fichero sigue en Sytue Drive: aquí
// se va el índice, y reindexar esa carpeta la traería de vuelta.
func (p *Photos) Purgar(ctx context.Context, id string) (int, error) {
	if d, ok := p.data.(PhotosPurgeData); ok {
		return d.Purgar(ctx, id)
	}
	ruta := "/trash"
	if id != "" {
		ruta += "/" + url.PathEscape(id)
	}
	r, err := JSON[struct {
		Deleted int `json:"deleted"`
	}](ctx, p.c, Request{Method: http.MethodDelete, Path: ruta})
	return r.Deleted, err
}

// Album es una colección de fotos de la persona. CoverAssetID es la foto de portada
// («null» si el álbum está vacío) y Count, cuántas tiene.
type Album struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CoverAssetID string `json:"coverAssetId"`
	Count        int    `json:"count"`
	// Shared dice si tiene enlace público. El enlace solo sale al generarlo
	// (ShareAlbum): el servicio guarda su huella, no el enlace (FILE-10).
	Shared bool `json:"shared,omitempty"`
	// UpdatedAt es la última modificación: el orden «recientes» de «Álbumes», que es el
	// del servicio (`updated_at DESC`).
	UpdatedAt time.Time `json:"updatedAt"`
	// CompartidoPor es de quién es un álbum de OTRA persona que me lo ha compartido para
	// editar (va en «Añadir a un álbum…»); vacío en los míos. No viaja: lo pone la app.
	CompartidoPor string `json:"-"`
}

// Rotulo es como se nombra en «Añadir a un álbum…»: el título y, si es de otra persona,
// de quién, como la web (`${title} · ${compartidoPor}`).
func (a Album) Rotulo() string {
	if a.CompartidoPor != "" {
		return a.Title + " · " + a.CompartidoPor
	}
	return a.Title
}

// Albums son los álbumes de la persona, del más nuevo al más viejo.
func (p *Photos) Albums(ctx context.Context) ([]Album, error) {
	if p.data != nil {
		if albums, ok := p.data.(PhotosAlbumData); ok {
			return albums.Albums(ctx)
		}
		return nil, ErrPrivatePhotosOperation
	}
	r, err := JSON[struct {
		Items []Album `json:"items"`
	}](ctx, p.c, Request{Path: "/albums"})
	return r.Items, err
}

// CreateAlbum crea un álbum vacío con ese título.
func (p *Photos) CreateAlbum(ctx context.Context, titulo string) (Album, error) {
	if p.data != nil {
		if albums, ok := p.data.(PhotosAlbumData); ok {
			return albums.CreateAlbum(ctx, titulo)
		}
		return Album{}, ErrPrivatePhotosOperation
	}
	return PostJSON[Album](ctx, p.c, "/albums", map[string]string{"title": titulo})
}

// Album es un álbum por su id, con su recuento y su enlace si está compartido.
func (p *Photos) Album(ctx context.Context, id string) (Album, error) {
	if p.data != nil {
		if albums, ok := p.data.(PhotosAlbumData); ok {
			return albums.Album(ctx, id)
		}
		return Album{}, ErrPrivatePhotosOperation
	}
	return JSON[Album](ctx, p.c, Request{Path: "/albums/" + url.PathEscape(id)})
}

// DeleteAlbum borra el álbum. Las fotos se quedan: un álbum solo las agrupa.
func (p *Photos) DeleteAlbum(ctx context.Context, id string) error {
	if p.data != nil {
		if albums, ok := p.data.(PhotosAlbumData); ok {
			return albums.DeleteAlbum(ctx, id)
		}
		return ErrPrivatePhotosOperation
	}
	return p.mandar(ctx, http.MethodDelete, "/albums/"+url.PathEscape(id), nil)
}

// AddToAlbum mete esas fotos en el álbum y devuelve cuántas entraron (las que ya
// estaban no cuentan).
func (p *Photos) AddToAlbum(ctx context.Context, album string, ids []string) (int, error) {
	if p.data != nil {
		if albums, ok := p.data.(PhotosAlbumMembershipData); ok {
			return albums.AddToAlbum(ctx, album, ids)
		}
		return 0, ErrPrivatePhotosOperation
	}
	r, err := PostJSON[struct {
		Added int `json:"added"`
	}](ctx, p.c, "/albums/"+url.PathEscape(album)+"/assets", map[string][]string{"ids": ids})
	return r.Added, err
}

// RemoveFromAlbum saca una foto del álbum. La foto sigue en la galería.
func (p *Photos) RemoveFromAlbum(ctx context.Context, album, id string) error {
	if p.data != nil {
		if albums, ok := p.data.(PhotosAlbumMembershipData); ok {
			return albums.RemoveFromAlbum(ctx, album, id)
		}
		return ErrPrivatePhotosOperation
	}
	return p.mandar(ctx, http.MethodDelete, "/albums/"+url.PathEscape(album)+"/assets/"+url.PathEscape(id), nil)
}

// RenameAlbum le cambia el título a un álbum: a uno mío o a uno que me han compartido
// para editar.
func (p *Photos) RenameAlbum(ctx context.Context, id, titulo string) error {
	if p.data != nil {
		if albums, ok := p.data.(PhotosAlbumData); ok {
			return albums.RenameAlbum(ctx, id, titulo)
		}
		return ErrPrivatePhotosOperation
	}
	return p.mandar(ctx, http.MethodPatch, "/albums/"+url.PathEscape(id), map[string]string{"title": titulo})
}

// PonerPortada elige la foto de portada de un álbum. Es el mismo PATCH que renombrar
// (`ponerPortada` en photosApi.ts): el servicio exige el título en cada cambio, así que
// va el de ahora. La foto tiene que estar en el álbum; si no, el servicio contesta 400.
func (p *Photos) PonerPortada(ctx context.Context, id, titulo, foto string) error {
	if d, ok := p.data.(PhotosAlbumCoverData); ok {
		return d.PonerPortada(ctx, id, titulo, foto)
	}
	return p.mandar(ctx, http.MethodPatch, "/albums/"+url.PathEscape(id),
		map[string]string{"title": titulo, "coverAssetId": foto})
}

// ShareAlbum abre el enlace público del álbum y devuelve su token (`/p/{token}`).
func (p *Photos) ShareAlbum(ctx context.Context, id string) (string, error) {
	r, err := JSON[struct {
		Token string `json:"token"`
	}](ctx, p.c, Request{Method: http.MethodPost, Path: "/albums/" + url.PathEscape(id) + "/share"})
	return r.Token, err
}

// UnshareAlbum cierra el enlace: quien lo tenga deja de ver el álbum.
func (p *Photos) UnshareAlbum(ctx context.Context, id string) error {
	return p.mandar(ctx, http.MethodDelete, "/albums/"+url.PathEscape(id)+"/share", nil)
}

// AlbumPhotos son las fotos de un álbum, en su orden.
func (p *Photos) AlbumPhotos(ctx context.Context, album string) ([]Photo, error) {
	if p.data != nil {
		if albums, ok := p.data.(PhotosAlbumData); ok {
			return albums.AlbumPhotos(ctx, album)
		}
		return nil, ErrPrivatePhotosOperation
	}
	r, err := JSON[struct {
		Items []Photo `json:"items"`
	}](ctx, p.c, Request{Path: "/albums/" + url.PathEscape(album) + "/assets"})
	return r.Items, err
}

// PublicAlbum es un álbum compartido tal como lo ve quien tiene el enlace: su título,
// cuántas fotos tiene y, de cada una, lo MÍNIMO para pintar la cuadrícula (id, nombre,
// medidas y fecha). Ni cámara, ni lugar, ni si es favorita: el servicio no lo manda.
type PublicAlbum struct {
	Title string
	Count int
	Items []Photo
}

// PublicAlbum trae el álbum compartido por su token, el de `/p/{token}` en la web. Va
// SIN sesión aunque este cliente la tenga: la credencial es el token del enlace, quien
// lo abre puede no tener cuenta, y un enlace abierto no tiene por qué saber quién lo
// mira. Un token que no existe (o que ya se cerró) es un *Error 404.
func (p *Photos) PublicAlbum(ctx context.Context, token string) (PublicAlbum, error) {
	r, err := JSON[struct {
		Album struct {
			Title string `json:"title"`
			Count int    `json:"count"`
		} `json:"album"`
		Items []Photo `json:"items"`
	}](ctx, p.anonimo(), Request{Path: "/public/albums/" + url.PathEscape(token)})
	return PublicAlbum{Title: r.Album.Title, Count: r.Album.Count, Items: r.Items}, err
}

// PublicVariant es la imagen de una foto del álbum compartido, también sin sesión. El
// servicio solo la sirve si la foto está EN ese álbum. Cerrarla es cosa de quien llama.
func (p *Photos) PublicVariant(ctx context.Context, token, id, variante string) (io.ReadCloser, error) {
	resp, err := p.anonimo().Do(ctx, Request{
		Path:   "/public/albums/" + url.PathEscape(token) + "/assets/" + url.PathEscape(id) + "/variants/" + url.PathEscape(variante),
		Header: http.Header{"Accept": {"image/*"}},
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// PersonasDelAlbum es la ruta de «compartir con una persona» de un álbum (su dueña).
// Hoy solo para ver: el servicio rechaza otro nivel.
func (p *Photos) PersonasDelAlbum(id string) Personas {
	return NuevasPersonas(p.c, "/albums/"+url.PathEscape(id)+"/people")
}

// AlbumCompartido es un álbum de «Compartido conmigo»: quién lo compartió (su `sub`) y,
// abierto, lo mínimo de sus fotos, como el del enlace.
type AlbumCompartido struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	CoverID *string `json:"coverAssetId"`
	Count   int     `json:"count"`
	Por     string  `json:"sharedBy"`
	// Permiso es con qué nivel me lo compartieron: NivelVer o NivelEditar.
	Permiso string `json:"permission"`
}

// AlbumesCompartidos son los álbumes que otras personas han compartido conmigo.
func (p *Photos) AlbumesCompartidos(ctx context.Context) ([]AlbumCompartido, error) {
	r, err := JSON[struct {
		Items []AlbumCompartido `json:"items"`
	}](ctx, p.c, Request{Path: "/shared/albums"})
	return r.Items, err
}

// AlbumCompartidoConmigo abre uno de ellos, con sus fotos. Si ya no está compartido
// conmigo, *Error 404.
func (p *Photos) AlbumCompartidoConmigo(ctx context.Context, id string) (AlbumCompartido, []Photo, error) {
	r, err := JSON[struct {
		Album AlbumCompartido `json:"album"`
		Items []Photo         `json:"items"`
	}](ctx, p.c, Request{Path: "/shared/albums/" + url.PathEscape(id)})
	return r.Album, r.Items, err
}

// VarianteCompartida es la imagen de una foto de un álbum compartido conmigo. El
// servicio solo la sirve si el álbum está compartido conmigo y la foto está en él.
func (p *Photos) VarianteCompartida(ctx context.Context, album, id, variante string) (io.ReadCloser, error) {
	return p.imagen(ctx, "/shared/albums/"+url.PathEscape(album)+"/assets/"+url.PathEscape(id)+"/variants/"+url.PathEscape(variante), nil)
}

// anonimo es el mismo servicio sin la sesión.
func (p *Photos) anonimo() *Client { return New(p.c.base, nil, p.c.http) }

// Duplicates son los grupos de fotos que se repiten: ráfagas y reintentos del mismo
// momento. El servicio PROPONE y no borra —quien decide es quien mira—, así que esto
// solo devuelve los grupos.
func (p *Photos) Duplicates(ctx context.Context, limite int) ([][]Photo, error) {
	if d, ok := p.data.(PhotosDiscoveryData); ok {
		return d.Duplicates(ctx, limite)
	}
	r, err := JSON[struct {
		Groups []struct {
			Items []Photo `json:"items"`
		} `json:"groups"`
	}](ctx, p.c, Request{Path: "/duplicates", Query: url.Values{"limit": {strconv.Itoa(limite)}}})
	grupos := make([][]Photo, 0, len(r.Groups))
	for _, g := range r.Groups {
		grupos = append(grupos, g.Items)
	}
	return grupos, err
}

// PhotosSettings son los ajustes de organización en Sytue Photos: si la organización
// está excluida del lote automático de IA y si quien pregunta puede cambiarlo.
type PhotosSettings struct {
	ExcludeAIBatch   bool `json:"excludeAiBatch"`
	CanManageAIBatch bool `json:"canManageAiBatch"`
}

// Settings consulta los ajustes de la organización en Sytue Photos.
func (p *Photos) Settings(ctx context.Context) (PhotosSettings, error) {
	return JSON[PhotosSettings](ctx, p.c, Request{Path: "/settings"})
}

// SetExcludeAIBatch activa o desactiva la exclusión de la organización en el lote de IA.
func (p *Photos) SetExcludeAIBatch(ctx context.Context, exclude bool) (PhotosSettings, error) {
	cuerpo, err := json.Marshal(map[string]bool{"excludeAiBatch": exclude})
	if err != nil {
		return PhotosSettings{}, err
	}
	return JSON[PhotosSettings](ctx, p.c, Request{
		Method: http.MethodPut,
		Path:   "/settings",
		Body:   cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}
