// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Cast es el cliente de Sytue Cast (services/cast, ADR 0061). El cable va en
// snake_case, como el resto de la suite; la forma exacta es la de
// services/cast/internal/http (§5 del encargo).
//
// Tiene TRES puertas porque el servicio tiene tres:
//
//   - `/api/v1`, con sesión: la biblioteca, subir, gestionar, emitir, comentar.
//   - `/api/v1/public`, sin sesión y limitada por IP: la portada, el canal de una
//     organización y los vídeos públicos o sin listar. Las nativas entran por aquí y no
//     por `/public`, que el ingress no publica (la web lo usa por dentro del clúster).
//   - la raíz: los ficheros firmados de `/media/…` (póster, WebM) y el WebSocket del
//     chat en `/ws/…`.
type Cast struct {
	c, pub, raiz *Client
}

// NewCast crea el cliente sobre la raíz del servicio (ServiceURL(apex, "cast")).
func NewCast(servicio string, s Session, h *http.Client) *Cast {
	servicio = strings.TrimRight(servicio, "/")
	return &Cast{
		c:    New(servicio+"/api/v1", s, h),
		pub:  New(servicio+"/api/v1/public", nil, h),
		raiz: New(servicio, s, h),
	}
}

// Los estados de un vídeo (§4: el CHECK de `videos.status`).
const (
	CastStatusUploading  = "uploading"
	CastStatusProcessing = "processing"
	CastStatusReady      = "ready"
	CastStatusFailed     = "failed"
	CastStatusScheduled  = "scheduled"
	CastStatusLive       = "live"
	CastStatusEnded      = "ended"
)

// Las cuatro visibilidades (decisión 3 del dueño).
const (
	CastPublic   = "public"
	CastUnlisted = "unlisted"
	CastOrg      = "org"
	CastPeople   = "people"
)

// CastVisibilidades son las cuatro, en el orden del selector de la web.
var CastVisibilidades = []string{CastPublic, CastUnlisted, CastOrg, CastPeople}

// CastVideo es un vídeo, subido o en directo (§5.1).
type CastVideo struct {
	ID         string    `json:"id"`
	Tenant     string    `json:"tenant"`
	Owner      CastOwner `json:"owner"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Visibility string    `json:"visibility"`
	Title      string    `json:"title"`
	// Description puede ser larga (5000) y con saltos de línea.
	Description string `json:"description"`
	// DurationMS, Width y Height llegan a null hasta que el transcodificador los mide:
	// aquí quedan en cero.
	DurationMS int64 `json:"duration_ms"`
	Width      int   `json:"width"`
	Height     int   `json:"height"`
	// PosterURL es RELATIVA a la raíz del servicio (`/media/<id>/<token>/poster.jpg`) y
	// caduca a las 6 h; vacía sin póster.
	PosterURL       string     `json:"poster_url"`
	CommentsEnabled bool       `json:"comments_enabled"`
	ChatEnabled     bool       `json:"chat_enabled"`
	DownloadEnabled bool       `json:"download_enabled"`
	Live            *CastLive  `json:"live"`
	Views           int64      `json:"views"`
	Failure         string     `json:"failure"`
	PublishedAt     *time.Time `json:"published_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Can             CastCan    `json:"can"`
	// Qualities son las calidades elegidas (alturas, de mayor a menor), ya
	// recortadas a lo que da el original; HLSQualities y WebMQualities, las que
	// existen en cada escalera. Un vídeo que aún se procesa las tiene vacías.
	Qualities     []int `json:"qualities"`
	HLSQualities  []int `json:"hls_qualities"`
	WebMQualities []int `json:"webm_qualities"`
}

// CastOwner es quien subió o emite el vídeo.
type CastOwner struct {
	Sub  string `json:"sub"`
	Name string `json:"name"`
}

// CastLive es lo del directo; nil en un vídeo subido.
type CastLive struct {
	Source      string     `json:"source"`
	StartedAt   *time.Time `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at"`
	ViewersNow  int        `json:"viewers_now"`
	ViewersPeak int        `json:"viewers_peak"`
}

// CastCan es lo que puede hacer con el vídeo quien pregunta.
type CastCan struct {
	Edit     bool `json:"edit"`
	Delete   bool `json:"delete"`
	Moderate bool `json:"moderate"`
}

// EnDirecto dice si ahora mismo se está emitiendo.
func (v CastVideo) EnDirecto() bool { return v.Status == CastStatusLive }

// Fecha es la que se enseña: la de publicación, o la de alta si no se ha publicado.
func (v CastVideo) Fecha() time.Time {
	if v.PublishedAt != nil {
		return *v.PublishedAt
	}
	return v.CreatedAt
}

// CastPage es una página de un listado.
type CastPage struct {
	Videos     []CastVideo `json:"videos"`
	NextCursor string      `json:"next_cursor"`
}

// CastExplore es la portada pública: primero los directos, después los vídeos.
type CastExplore struct {
	Live       []CastVideo `json:"live"`
	Videos     []CastVideo `json:"videos"`
	NextCursor string      `json:"next_cursor"`
}

// CastChannel es el canal público de una organización.
type CastChannel struct {
	Org        string      `json:"org"`
	Name       string      `json:"name"`
	Videos     []CastVideo `json:"videos"`
	NextCursor string      `json:"next_cursor"`
}

// CastPlay es lo que hace falta para reproducir (§5.1). Las URL son relativas a la raíz
// del servicio; se abren con Media o MediaURL.
type CastPlay struct {
	Kind   string `json:"kind"`
	HLSURL string `json:"hls_url"`
	// WebMURL es el WebM de la calidad por defecto (720p si la hay). Los demás están
	// al lado, con el mismo token: WebMURLDe.
	WebMURL string `json:"webm_url"`
	// WebMQualities son los WebM que hay (alturas, de mayor a menor).
	WebMQualities []int         `json:"webm_qualities"`
	PosterURL     string        `json:"poster_url"`
	DownloadURL   string        `json:"download_url"`
	DurationMS    int64         `json:"duration_ms"`
	Live          *CastLiveConn `json:"live"`
}

// CastCalidadPorDefecto es la del WebM de siempre, la que traía webm_url antes de la
// escalera.
const CastCalidadPorDefecto = 720

// Calidades son los WebM que se pueden pedir, de mayor a menor. Un servicio de antes
// de la escalera no manda webm_qualities: entonces solo está el de webm_url, que es
// el de 720p.
func (p CastPlay) Calidades() []int {
	if len(p.WebMQualities) > 0 {
		return p.WebMQualities
	}
	if p.WebMURL != "" {
		return []int{CastCalidadPorDefecto}
	}
	return nil
}

// WebMURLDe es la ruta (relativa, como WebMURL) del WebM de una calidad:
// `/media/<id>/<token>/webm/<q>p.webm`. Vacía si esa calidad no está.
func (p CastPlay) WebMURLDe(q int) string {
	if p.WebMURL == "" || !slices.Contains(p.Calidades(), q) {
		return ""
	}
	if len(p.WebMQualities) == 0 {
		return p.WebMURL
	}
	dir := p.WebMURL[:strings.LastIndex(p.WebMURL, "/")+1]
	return dir + strconv.Itoa(q) + "p.webm"
}

// CastLiveConn es a qué SFU conectarse, con qué token y en qué sala: para ver (solo
// suscripción) o para emitir (`GET /lives/{id}/publish`).
type CastLiveConn struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	Room      string `json:"room"`
	Identity  string `json:"identity"`
}

// CastQuota es la cuota del plan según Drive, con lo que ocupa Cast.
type CastQuota struct {
	LimitBytes     int64 `json:"limit_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	CastBytes      int64 `json:"cast_bytes"`
}

// CastStreamKey es la clave de OBS de quien pregunta: solo se la dan a su dueño.
type CastStreamKey struct {
	RTMPURL   string `json:"rtmp_url"`
	StreamKey string `json:"stream_key"`
	IngressID string `json:"ingress_id"`
}

// CastPerson es alguien con acceso a un vídeo `people`.
type CastPerson struct {
	UserID   string    `json:"user_id"`
	Name     string    `json:"name"`
	Username string    `json:"username"`
	AddedAt  time.Time `json:"added_at"`
}

// CastAuthor firma un comentario o un mensaje del chat.
type CastAuthor struct {
	Sub      string `json:"sub"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// CastComment es un comentario bajo el vídeo. Los hilos son de un nivel: ReplyTo es el
// comentario raíz al que contesta.
type CastComment struct {
	ID        string     `json:"id"`
	Author    CastAuthor `json:"author"`
	Body      string     `json:"body"`
	ReplyTo   string     `json:"reply_to"`
	CreatedAt time.Time  `json:"created_at"`
	Deleted   bool       `json:"deleted"`
	Can       struct {
		Delete bool `json:"delete"`
	} `json:"can"`
}

// CastChatMessage es un mensaje del chat de un directo.
type CastChatMessage struct {
	ID        string     `json:"id"`
	Author    CastAuthor `json:"author"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
}

// CastNew es el alta de una subida o de un directo.
type CastNew struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	// Source solo en un directo: "rtmp" (OBS) o "webrtc" (desde la app).
	Source string `json:"source,omitempty"`
	// Qualities son las calidades elegidas al subir; vacía = la recomendada (todas
	// las que dé el original).
	Qualities []int `json:"qualities,omitempty"`
}

// CastChanges son los campos que se cambian; nil no se toca.
type CastChanges struct {
	Title           *string `json:"title,omitempty"`
	Description     *string `json:"description,omitempty"`
	Visibility      *string `json:"visibility,omitempty"`
	CommentsEnabled *bool   `json:"comments_enabled,omitempty"`
	ChatEnabled     *bool   `json:"chat_enabled,omitempty"`
	DownloadEnabled *bool   `json:"download_enabled,omitempty"`
}

// ---- La biblioteca (con sesión) ----

// Los dos alcances de la biblioteca.
const (
	CastMine     = "mine"
	CastOrgScope = "org"
)

// Videos es una página de la biblioteca: `scope` "mine" (los míos) u "org" (los de la
// organización que puedo ver). `status` y `q` filtran; vacíos, no.
func (c *Cast) Videos(ctx context.Context, scope, status, q, cursor string, limite int) (CastPage, error) {
	v := url.Values{}
	poner(v, "scope", scope)
	poner(v, "status", status)
	poner(v, "q", q)
	poner(v, "cursor", cursor)
	if limite > 0 {
		v.Set("limit", strconv.Itoa(limite))
	}
	return JSON[CastPage](ctx, c.c, Request{Path: "/videos", Query: v})
}

// Shared son los vídeos que otras personas han compartido conmigo por @usuario.
func (c *Cast) Shared(ctx context.Context, cursor string) (CastPage, error) {
	v := url.Values{}
	poner(v, "cursor", cursor)
	return JSON[CastPage](ctx, c.c, Request{Path: "/videos/shared", Query: v})
}

// Video es la ficha de un vídeo, con la ACL de quien pregunta.
func (c *Cast) Video(ctx context.Context, id string) (CastVideo, error) {
	return JSON[CastVideo](ctx, c.c, Request{Path: "/videos/" + url.PathEscape(id)})
}

// Create da de alta una subida (status uploading); el fichero va después con Upload.
func (c *Cast) Create(ctx context.Context, n CastNew) (CastVideo, error) {
	n.Source = ""
	return PostJSON[CastVideo](ctx, c.c, "/videos", n)
}

// Upload sube el original EN STREAMING: `cuerpo` no se lee entero en memoria. El
// servicio mira la cuota ANTES de aceptar (507 si no cabe; ver CastQuotaAvailable) y
// rechaza más de 8 GiB (413).
func (c *Cast) Upload(ctx context.Context, id string, cuerpo io.Reader, tamano int64, tipo string) (CastVideo, error) {
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	resp, err := c.c.Stream(ctx, http.MethodPut, "/videos/"+url.PathEscape(id)+"/original", nil,
		http.Header{"Content-Type": {tipo}}, cuerpo, tamano)
	if err != nil {
		return CastVideo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var v CastVideo
	err = json.NewDecoder(resp.Body).Decode(&v)
	return v, err
}

// Update cambia lo que venga en `cambios`.
func (c *Cast) Update(ctx context.Context, id string, cambios CastChanges) (CastVideo, error) {
	datos, err := json.Marshal(cambios)
	if err != nil {
		return CastVideo{}, err
	}
	return JSON[CastVideo](ctx, c.c, Request{
		Method: http.MethodPatch, Path: "/videos/" + url.PathEscape(id), Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Delete borra el vídeo (y libera su cuota).
func (c *Cast) Delete(ctx context.Context, id string) error {
	_, err := JSON[struct{}](ctx, c.c, Request{Method: http.MethodDelete, Path: "/videos/" + url.PathEscape(id)})
	return err
}

// UploadPoster pone un póster propio (JPEG, PNG o WebP de hasta 5 MiB).
func (c *Cast) UploadPoster(ctx context.Context, id string, datos []byte, tipo string) (CastVideo, error) {
	return JSON[CastVideo](ctx, c.c, Request{
		Method: http.MethodPut, Path: "/videos/" + url.PathEscape(id) + "/poster", Body: datos,
		Header: http.Header{"Content-Type": {tipo}},
	})
}

// PosterFrame pide el póster de un fotograma del propio vídeo; se hace en segundo plano.
func (c *Cast) PosterFrame(ctx context.Context, id string, enMS int64) (CastVideo, error) {
	return PostJSON[CastVideo](ctx, c.c, "/videos/"+url.PathEscape(id)+"/poster:frame", map[string]int64{"at_ms": enMS})
}

// People son las personas con acceso a un vídeo `people`.
func (c *Cast) People(ctx context.Context, id string) ([]CastPerson, error) {
	r, err := JSON[struct {
		People []CastPerson `json:"people"`
	}](ctx, c.c, Request{Path: "/videos/" + url.PathEscape(id) + "/people"})
	return r.People, err
}

// AddPerson da acceso a alguien por «@usuario», nombre o correo.
func (c *Cast) AddPerson(ctx context.Context, id, consulta string) (CastPerson, error) {
	return PostJSON[CastPerson](ctx, c.c, "/videos/"+url.PathEscape(id)+"/people", map[string]string{"query": consulta})
}

// RemovePerson le quita el acceso.
func (c *Cast) RemovePerson(ctx context.Context, id, userID string) error {
	_, err := JSON[struct{}](ctx, c.c, Request{
		Method: http.MethodDelete, Path: "/videos/" + url.PathEscape(id) + "/people/" + url.PathEscape(userID),
	})
	return err
}

// Play es lo que hace falta para reproducir con sesión: el WebM de la app, o la
// conexión al directo.
func (c *Cast) Play(ctx context.Context, id string) (CastPlay, error) {
	return JSON[CastPlay](ctx, c.c, Request{Path: "/videos/" + url.PathEscape(id) + "/play"})
}

// Quota es la cuota del plan, compartida con Drive.
func (c *Cast) Quota(ctx context.Context) (CastQuota, error) {
	return JSON[CastQuota](ctx, c.c, Request{Path: "/quota"})
}

// CastQuotaAvailable dice si `err` es el 507 de «no queda sitio» y cuántos bytes
// quedan (`details.available_bytes`).
func CastQuotaAvailable(err error) (int64, bool) {
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusInsufficientStorage {
		return 0, false
	}
	var d struct {
		Available int64 `json:"available_bytes"`
	}
	_ = json.Unmarshal(e.cuerpo().Details, &d)
	return d.Available, true
}

// ---- Subir desde Sytue Drive ----

// CastDriveFile es el fichero de Drive que se va a copiar a Cast
// (`GET /videos:from-drive`): lo que se enseña antes de confirmar.
type CastDriveFile struct {
	FileID      string `json:"file_id"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
	// Title es el nombre sin extensión, como mucho 200 caracteres.
	Title string `json:"title"`
}

// CastFromDrive es la petición de copia; título y visibilidad vacíos son los de por
// omisión del servicio (el nombre del fichero y `org`).
type CastFromDrive struct {
	FileID      string `json:"file_id"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	Qualities   []int  `json:"qualities,omitempty"`
}

// DriveFile dice qué se copiaría de Drive, sin crear nada. Da los mismos errores que
// FromDrive (415 no es un vídeo, 413 pasa de 8 GiB, 507 no cabe, 403/404 no se ve).
func (c *Cast) DriveFile(ctx context.Context, fileID string) (CastDriveFile, error) {
	return JSON[CastDriveFile](ctx, c.c, Request{Path: "/videos:from-drive", Query: url.Values{"file_id": {fileID}}})
}

// FromDrive crea el vídeo y deja la copia desde Drive en marcha en el servicio: vuelve
// con el vídeo en «subiendo» (202).
func (c *Cast) FromDrive(ctx context.Context, n CastFromDrive) (CastVideo, error) {
	return PostJSON[CastVideo](ctx, c.c, "/videos:from-drive", n)
}

// ---- Calidades ----

// CastQualityOption es una calidad con lo que ocuparía (su variante HLS y su WebM).
type CastQualityOption struct {
	Quality int `json:"quality"`
	// Allowed: el original llega a ella (o aún no se sabe).
	Allowed     bool  `json:"allowed"`
	Recommended bool  `json:"recommended"`
	Bytes       int64 `json:"bytes"`
}

// CastEstimate es cuánto ocupará cada calidad (`GET /videos:estimate`).
type CastEstimate struct {
	Options       []CastQualityOption `json:"options"`
	Recommended   []int               `json:"recommended"`
	OriginalBytes int64               `json:"original_bytes"`
	DurationMS    int64               `json:"duration_ms"`
	// Approximate: la duración se dedujo del tamaño del original, no se midió.
	Approximate bool `json:"approximate"`
}

// Bytes es lo que suman esas calidades.
func (e CastEstimate) Bytes(calidades []int) int64 {
	var n int64
	for _, o := range e.Options {
		if slices.Contains(calidades, o.Quality) {
			n += o.Bytes
		}
	}
	return n
}

// CastVideoQualities son las calidades de un vídeo ya creado, con su estimación
// (`GET /videos/{id}/qualities`).
type CastVideoQualities struct {
	CastEstimate
	Qualities     []int `json:"qualities"`
	HLSQualities  []int `json:"hls_qualities"`
	WebMQualities []int `json:"webm_qualities"`
	// Pending: lo generado aún no es lo elegido.
	Pending bool `json:"pending"`
}

// Estimate pide cuánto ocupará cada calidad de un original de ese tamaño. Duración y
// dimensiones, si se saben (0 si no): sin duración la estimación es aproximada.
func (c *Cast) Estimate(ctx context.Context, tamano, duracionMS int64, ancho, alto int) (CastEstimate, error) {
	v := url.Values{"size_bytes": {strconv.FormatInt(tamano, 10)}}
	if duracionMS > 0 {
		v.Set("duration_ms", strconv.FormatInt(duracionMS, 10))
	}
	if ancho > 0 && alto > 0 {
		v.Set("width", strconv.Itoa(ancho))
		v.Set("height", strconv.Itoa(alto))
	}
	return JSON[CastEstimate](ctx, c.c, Request{Path: "/videos:estimate", Query: v})
}

// Qualities son las calidades de un vídeo (solo quien lo edita).
func (c *Cast) Qualities(ctx context.Context, id string) (CastVideoQualities, error) {
	return JSON[CastVideoQualities](ctx, c.c, Request{Path: "/videos/" + url.PathEscape(id) + "/qualities"})
}

// SetQualities cambia la elección: lo que se añade se genera y lo que se quita se
// borra (y libera su espacio) en segundo plano.
func (c *Cast) SetQualities(ctx context.Context, id string, calidades []int) (CastVideoQualities, error) {
	datos, err := json.Marshal(map[string][]int{"qualities": calidades})
	if err != nil {
		return CastVideoQualities{}, err
	}
	return JSON[CastVideoQualities](ctx, c.c, Request{
		Method: http.MethodPut, Path: "/videos/" + url.PathEscape(id) + "/qualities", Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// ---- Emitir ----

// StreamKey es la clave de OBS de quien llama; se crea la primera vez que se pide.
func (c *Cast) StreamKey(ctx context.Context) (CastStreamKey, error) {
	return JSON[CastStreamKey](ctx, c.c, Request{Path: "/studio/stream-key"})
}

// RotateStreamKey invalida la clave y da otra: la vieja deja de funcionar en OBS.
func (c *Cast) RotateStreamKey(ctx context.Context) (CastStreamKey, error) {
	return JSON[CastStreamKey](ctx, c.c, Request{Method: http.MethodPost, Path: "/studio/stream-key:rotate"})
}

// ScheduleLive programa un directo. Con Source "webrtc" trae ya el token de emisor.
func (c *Cast) ScheduleLive(ctx context.Context, n CastNew) (CastVideo, *CastLiveConn, error) {
	r, err := PostJSON[struct {
		Video   CastVideo     `json:"video"`
		Publish *CastLiveConn `json:"publish"`
	}](ctx, c.c, "/lives", n)
	return r.Video, r.Publish, err
}

// PublishToken es el token de emisor de un directo (solo su dueño).
func (c *Cast) PublishToken(ctx context.Context, id string) (CastLiveConn, error) {
	return JSON[CastLiveConn](ctx, c.c, Request{Path: "/lives/" + url.PathEscape(id) + "/publish"})
}

// EndLive termina el directo; la grabación se queda como vídeo.
func (c *Cast) EndLive(ctx context.Context, id string) (CastVideo, error) {
	return JSON[CastVideo](ctx, c.c, Request{Method: http.MethodPost, Path: "/lives/" + url.PathEscape(id) + "/end"})
}

// ---- Comentarios y chat ----

// Comments es una página de comentarios, con sesión.
func (c *Cast) Comments(ctx context.Context, id, cursor string) ([]CastComment, string, error) {
	return c.comentarios(ctx, c.c, id, cursor)
}

// PublicComments es lo mismo sin sesión (vídeo public o unlisted).
func (c *Cast) PublicComments(ctx context.Context, id, cursor string) ([]CastComment, string, error) {
	return c.comentarios(ctx, c.pub, id, cursor)
}

func (c *Cast) comentarios(ctx context.Context, cli *Client, id, cursor string) ([]CastComment, string, error) {
	v := url.Values{}
	poner(v, "cursor", cursor)
	r, err := JSON[struct {
		Comments   []CastComment `json:"comments"`
		NextCursor string        `json:"next_cursor"`
	}](ctx, cli, Request{Path: "/videos/" + url.PathEscape(id) + "/comments", Query: v})
	return r.Comments, r.NextCursor, err
}

// Comment publica un comentario; `respondeA` vacío es uno raíz.
func (c *Cast) Comment(ctx context.Context, id, cuerpo, respondeA string) (CastComment, error) {
	in := map[string]string{"body": cuerpo}
	if respondeA != "" {
		in["reply_to"] = respondeA
	}
	return PostJSON[CastComment](ctx, c.c, "/videos/"+url.PathEscape(id)+"/comments", in)
}

// DeleteComment lo borra (su autor, el dueño del vídeo o la administración).
func (c *Cast) DeleteComment(ctx context.Context, id, comentario string) error {
	_, err := JSON[struct{}](ctx, c.c, Request{
		Method: http.MethodDelete, Path: "/videos/" + url.PathEscape(id) + "/comments/" + url.PathEscape(comentario),
	})
	return err
}

// ChatHistory es el historial del chat del directo desde `desde` (cero: todo).
func (c *Cast) ChatHistory(ctx context.Context, id string, desde time.Time) ([]CastChatMessage, error) {
	v := url.Values{}
	if !desde.IsZero() {
		v.Set("since", desde.UTC().Format(time.RFC3339Nano))
	}
	r, err := JSON[struct {
		Messages []CastChatMessage `json:"messages"`
	}](ctx, c.c, Request{Path: "/videos/" + url.PathEscape(id) + "/chat", Query: v})
	return r.Messages, err
}

// DeleteChat retira un mensaje del chat (moderación).
func (c *Cast) DeleteChat(ctx context.Context, id, mensaje string) error {
	_, err := JSON[struct{}](ctx, c.c, Request{
		Method: http.MethodDelete, Path: "/videos/" + url.PathEscape(id) + "/chat/" + url.PathEscape(mensaje),
	})
	return err
}

// CastChatFrame es una trama del WebSocket del chat: `message`, `deleted`, `viewers` o
// `error`.
type CastChatFrame struct {
	Type      string      `json:"type"`
	ID        string      `json:"id"`
	Author    *CastAuthor `json:"author"`
	Body      string      `json:"body"`
	CreatedAt *time.Time  `json:"created_at"`
	Count     int         `json:"count"`
	Code      string      `json:"code"`
	Message   string      `json:"message"`
}

// CastChat es el chat en vivo de un directo, abierto.
type CastChat struct {
	conn *websocket.Conn
}

// Chat abre el WebSocket del chat y entrega cada trama a `alLlegar` desde otra rutina
// hasta que se cierra o se cancela `ctx`; `alCerrar` dice por qué (nil si se cerró a
// propósito). El token va en la cabecera: fuera del navegador se puede.
func (c *Cast) Chat(ctx context.Context, id string, alLlegar func(CastChatFrame), alCerrar func(error)) (*CastChat, error) {
	token := ""
	if c.raiz.session != nil {
		t, err := c.raiz.token(ctx, c.raiz.base)
		if err != nil {
			return nil, err
		}
		token = t
	}
	conn, resp, err := websocket.Dial(ctx, c.raiz.WSURL("/ws/videos/"+url.PathEscape(id)+"/chat", nil), opcionesWS(c.raiz.http, token))
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("cast: chat: %w", err)
	}
	conn.SetReadLimit(64 << 10)
	go func() {
		for {
			var f CastChatFrame
			if err := wsjson.Read(ctx, conn, &f); err != nil {
				if alCerrar != nil {
					if websocket.CloseStatus(err) == websocket.StatusNormalClosure || ctx.Err() != nil {
						err = nil
					}
					alCerrar(err)
				}
				return
			}
			alLlegar(f)
		}
	}()
	return &CastChat{conn: conn}, nil
}

// Send manda un mensaje; la respuesta llega como trama `message` (o `error`).
func (ch *CastChat) Send(ctx context.Context, cuerpo string) error {
	return wsjson.Write(ctx, ch.conn, map[string]string{"type": "send", "body": cuerpo})
}

// Close cierra el chat.
func (ch *CastChat) Close() { _ = ch.conn.Close(websocket.StatusNormalClosure, "") }

// ---- Lo público (sin sesión) ----

// Explore es la portada pública: directos en curso y vídeos públicos de toda Sytue.
func (c *Cast) Explore(ctx context.Context, cursor string, limite int) (CastExplore, error) {
	v := url.Values{}
	poner(v, "cursor", cursor)
	if limite > 0 {
		v.Set("limit", strconv.Itoa(limite))
	}
	return JSON[CastExplore](ctx, c.pub, Request{Path: "/explore", Query: v, Anonymous: true})
}

// Channel es el canal público de una organización.
func (c *Cast) Channel(ctx context.Context, org, cursor string) (CastChannel, error) {
	v := url.Values{}
	poner(v, "cursor", cursor)
	return JSON[CastChannel](ctx, c.pub, Request{Path: "/c/" + url.PathEscape(org), Query: v, Anonymous: true})
}

// PublicVideo es la ficha de un vídeo public o unlisted, sin sesión.
func (c *Cast) PublicVideo(ctx context.Context, id string) (CastVideo, error) {
	return JSON[CastVideo](ctx, c.pub, Request{Path: "/videos/" + url.PathEscape(id), Anonymous: true})
}

// PublicPlay es Play sin sesión.
func (c *Cast) PublicPlay(ctx context.Context, id string) (CastPlay, error) {
	return JSON[CastPlay](ctx, c.pub, Request{Path: "/videos/" + url.PathEscape(id) + "/play", Anonymous: true})
}

// ---- Los ficheros firmados ----

// MediaURL es la dirección absoluta de una ruta de `/media/…` (la de poster_url o
// webm_url). Una dirección que ya es absoluta se devuelve tal cual.
func (c *Cast) MediaURL(ruta string) string {
	if strings.HasPrefix(ruta, "http://") || strings.HasPrefix(ruta, "https://") {
		return ruta
	}
	return c.raiz.URL(ruta, nil)
}

// Poster trae entero un póster (unos cientos de KB). El token firmado de la ruta es la
// credencial: la petición va sin sesión.
func (c *Cast) Poster(ctx context.Context, ruta string) ([]byte, error) {
	resp, err := c.media(ctx, http.MethodGet, ruta, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

func (c *Cast) media(ctx context.Context, metodo, ruta, rango string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, metodo, c.MediaURL(ruta), http.NoBody)
	if err != nil {
		return nil, err
	}
	if rango != "" {
		req.Header.Set("Range", rango)
	}
	resp, err := c.raiz.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		cuerpo, _ := io.ReadAll(io.LimitReader(resp.Body, maxError))
		_ = resp.Body.Close()
		return nil, &Error{Status: resp.StatusCode, URL: req.URL.String(), Body: string(cuerpo)}
	}
	return resp, nil
}

// Media abre un fichero de `/media/…` para LEERLO A SALTOS: un io.ReadSeeker que pide
// por Range solo lo que se lee, así el reproductor busca en un WebM de cientos de MB
// sin bajarlo entero. El tamaño lo da un HEAD al abrir.
func (c *Cast) Media(ctx context.Context, ruta string) (*CastMedia, error) {
	resp, err := c.media(ctx, http.MethodHead, ruta, "")
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	if resp.ContentLength < 0 {
		return nil, fmt.Errorf("cast: %s no dice su tamaño", ruta)
	}
	return &CastMedia{c: c, ctx: ctx, ruta: ruta, tamano: resp.ContentLength}, nil
}

// CastMedia es un fichero remoto leído por rangos. No es seguro para varias rutinas.
type CastMedia struct {
	c      *Cast
	ctx    context.Context //nolint:containedctx // vive lo que vive la reproducción
	ruta   string
	tamano int64
	pos    int64
	// El cuerpo abierto y dónde acaba: mientras se lee seguido se sigue del mismo; un
	// salto lo cierra y pide otro rango.
	cuerpo io.ReadCloser
	fin    int64
}

// TramoMedia es lo que se pide de una vez: bastante para un grupo de bloques de vídeo
// y poco para no bajar de más tras cada búsqueda.
const TramoMedia = 1 << 20

// Size es el tamaño del fichero.
func (m *CastMedia) Size() int64 { return m.tamano }

// Read lee de donde esté.
func (m *CastMedia) Read(p []byte) (int, error) {
	if m.pos >= m.tamano {
		return 0, io.EOF
	}
	if m.cuerpo == nil || m.pos >= m.fin {
		m.cerrar()
		fin := min(m.pos+max(TramoMedia, int64(len(p))), m.tamano)
		resp, err := m.c.media(m.ctx, http.MethodGet, m.ruta, fmt.Sprintf("bytes=%d-%d", m.pos, fin-1))
		if err != nil {
			return 0, err
		}
		if resp.StatusCode != http.StatusPartialContent {
			// Un servidor sin Range manda el fichero entero: se descarta hasta pos.
			if _, err := io.CopyN(io.Discard, resp.Body, m.pos); err != nil {
				_ = resp.Body.Close()
				return 0, err
			}
			fin = m.tamano
		}
		m.cuerpo, m.fin = resp.Body, fin
	}
	n, err := m.cuerpo.Read(p[:min(int64(len(p)), m.fin-m.pos)])
	m.pos += int64(n)
	if errors.Is(err, io.EOF) {
		m.cerrar()
		if n > 0 || m.pos < m.tamano {
			err = nil
		}
	}
	return n, err
}

// Seek mueve la posición; el siguiente Read pide el rango nuevo si hace falta.
func (m *CastMedia) Seek(desplazamiento int64, desde int) (int64, error) {
	var nueva int64
	switch desde {
	case io.SeekStart:
		nueva = desplazamiento
	case io.SeekCurrent:
		nueva = m.pos + desplazamiento
	case io.SeekEnd:
		nueva = m.tamano + desplazamiento
	default:
		return m.pos, errors.New("cast: Seek con un origen desconocido")
	}
	if nueva < 0 {
		return m.pos, errors.New("cast: Seek antes del principio")
	}
	if nueva != m.pos {
		m.cerrar()
	}
	m.pos = nueva
	return nueva, nil
}

// Close suelta la conexión abierta, si la hay.
func (m *CastMedia) Close() error {
	m.cerrar()
	return nil
}

func (m *CastMedia) cerrar() {
	if m.cuerpo != nil {
		_ = m.cuerpo.Close()
		m.cuerpo = nil
	}
}

// poner añade un parámetro solo si tiene valor.
func poner(v url.Values, clave, valor string) {
	if valor != "" {
		v.Set(clave, valor)
	}
}
