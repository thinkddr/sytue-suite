// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/platform/pkg/personas"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/config"
	"git.sytue.eu/sytue/one/services/cast/internal/cuota"
	"git.sytue.eu/sytue/one/services/cast/internal/diapositivas"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/social"
)

// Handlers agrupa los manejadores HTTP de Cast.
type Handlers struct {
	svc     *service.Servicio
	social  *social.Servicio
	directo Directo
	// diapositivas puede ir a nil: «Presentar con Cast» contesta 503 (PUT) o «sin presentación» (GET).
	diapositivas *diapositivas.Servicio
	cfg          config.Config
	verifier     *authn.Verifier
	// altcha es la prueba de trabajo de la denuncia sin sesión. nil = sin ella.
	altcha *altcha.Verifier
	// operador abre la cola de denuncias del operador. nil = cerrada.
	operador operador.Autorizar
}

// quienDe saca de los claims quién pregunta; sin claims, nadie (con su IP, para
// contar la vista). Los roles del token cuentan solo en su organización: `acceso`
// ya no los mira fuera de ella.
func quienDe(r *http.Request) service.Quien {
	ip := clientip.TrustedProxiesFrom(r.Context()).ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
	c, ok := authn.FromContext(r.Context())
	if !ok || c.Subject == "" {
		return service.Quien{Anonimo: true, IP: ip}
	}
	return service.Quien{
		Sub:    c.Subject,
		Tenant: c.Tenant,
		Nombre: c.Name,
		Admin:  c.HasAnyRole("owner", "admin"),
		Token:  authn.Token(r),
		IP:     ip,
	}
}

// traducir pasa los errores del dominio al sobre canónico.
func traducir(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNoExiste), errors.Is(err, social.ErrNoExiste):
		return httpx.ErrNotFound.Wrap(err)
	case errors.Is(err, social.ErrBloqueado):
		return httpx.ErrForbidden.WithCode("blocked").WithMessage("You cannot comment or chat on this video.").Wrap(err)
	case errors.Is(err, domain.ErrProhibido), errors.Is(err, social.ErrProhibido):
		return httpx.ErrForbidden.Wrap(err)
	case errors.Is(err, domain.ErrEntradaInvalida), errors.Is(err, social.ErrEntradaInvalida):
		return httpx.ErrBadRequest.WithMessage(mensajeDe(err)).Wrap(err)
	case errors.Is(err, domain.ErrConflicto):
		return httpx.ErrConflict.WithMessage(mensajeDe(err)).Wrap(err)
	case errors.Is(err, domain.ErrDemasiadoGrande):
		return httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "The file is larger than 8 GiB.").Wrap(err)
	case errors.Is(err, domain.ErrSinCuota):
		e := httpx.NewError(http.StatusInsufficientStorage, "quota_exceeded", "Not enough storage left in your plan.").Wrap(err)
		var sin *domain.SinCuota
		if errors.As(err, &sin) {
			e = e.WithDetails(map[string]any{"available_bytes": sin.Disponibles})
		}
		return e
	case errors.Is(err, service.ErrNoEsVideo):
		return httpx.ErrUnsupportedMedia.WithCode("not_video").WithMessage("The file is not a video.").Wrap(err)
	case errors.Is(err, service.ErrDriveNoContesta):
		// Antes que la cuota: envuelve los mismos errores del cliente de Drive.
		return httpx.ErrUnavailable.WithMessage("Sytue Drive did not answer. Try again in a moment.").Wrap(err)
	case errors.Is(err, service.ErrSinDrive):
		return httpx.ErrUnavailable.WithMessage("Uploading from Sytue Drive is not available on this deployment.").Wrap(err)
	case errors.Is(err, cuota.ErrRechazado), errors.Is(err, cuota.ErrNoDisponible):
		return httpx.ErrUnavailable.WithMessage("The storage quota could not be checked. Try again in a moment.").Wrap(err)
	case errors.Is(err, social.ErrDesactivado):
		return httpx.ErrForbidden.WithCode("disabled").WithMessage("Comments or chat are turned off for this video.").Wrap(err)
	case errors.Is(err, social.ErrDemasiadoRapido), errors.Is(err, personas.ErrDemasiadas):
		return httpx.ErrTooManyRequests.Wrap(err)
	case errors.Is(err, service.ErrSinDirectorio):
		return httpx.ErrUnavailable.WithMessage("Sharing with people is not available right now.").Wrap(err)
	case errors.Is(err, service.ErrSinAlmacen):
		return httpx.ErrUnavailable.WithMessage("Media storage is not available.").Wrap(err)
	case errors.Is(err, ErrNoDisponible):
		return httpx.ErrUnavailable.WithMessage("Live streaming is not available on this deployment.").Wrap(err)
	case errors.Is(err, tenancy.ErrNoTenant):
		return httpx.ErrForbidden.Wrap(err)
	}
	return httpx.ErrInternal.Wrap(err)
}

// mensajeDe deja el detalle del error de dominio sin su prefijo («cast: entrada
// inválida: »), que es lo que se le enseña a quien tecleó mal.
func mensajeDe(err error) string {
	msg := err.Error()
	if _, resto, ok := strings.Cut(msg, ": "); ok {
		if _, resto2, ok := strings.Cut(resto, ": "); ok {
			return resto2
		}
		return resto
	}
	return msg
}

func idDeRuta(r *http.Request, nombre string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, nombre))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithCode("invalid_id").WithMessage("Invalid identifier.").Wrap(err)
	}
	return id, nil
}

// limiteDe lee `?limit=`; 0 si no viene o no es un número (el servicio pone el suyo).
func limiteDe(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n
}

// uuidParam es un UUID opcional en un cuerpo JSON: acepta null o ausencia.
type uuidParam uuid.UUID

func (u *uuidParam) UnmarshalText(b []byte) error {
	id, err := uuid.ParseBytes(b)
	if err != nil {
		return err
	}
	*u = uuidParam(id)
	return nil
}

func (u *uuidParam) ptr() *uuid.UUID {
	if u == nil {
		return nil
	}
	id := uuid.UUID(*u)
	return &id
}

// ── DTO (CAST-ENCARGO §5.1, snake_case) ─────────────────────────────────────

type videoDTO struct {
	ID              uuid.UUID  `json:"id"`
	Tenant          string     `json:"tenant"`
	Owner           autorDTO   `json:"owner"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	Visibility      string     `json:"visibility"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	DurationMS      *int64     `json:"duration_ms"`
	Width           *int32     `json:"width"`
	Height          *int32     `json:"height"`
	PosterURL       *string    `json:"poster_url"`
	CommentsEnabled bool       `json:"comments_enabled"`
	ChatEnabled     bool       `json:"chat_enabled"`
	DownloadEnabled bool       `json:"download_enabled"`
	Live            *liveDTO   `json:"live"`
	Views           int64      `json:"views"`
	Failure         string     `json:"failure"`
	PublishedAt     *time.Time `json:"published_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Can             puedeDTO   `json:"can"`
	calidadesDTO
}

// calidadesDTO son las calidades del vídeo (alturas, de mayor a menor): las
// elegidas —ya resueltas contra lo que el original permite— y las que existen
// en cada escalera. Nunca null: una lista vacía es «ninguna».
type calidadesDTO struct {
	Qualities     []int `json:"qualities"`
	HLSQualities  []int `json:"hls_qualities"`
	WebMQualities []int `json:"webm_qualities"`
}

func calidadesDTOde(v domain.Video) calidadesDTO {
	return calidadesDTO{Qualities: v.Elegidas(), HLSQualities: v.CalidadesHLS(), WebMQualities: v.CalidadesWebM()}
}

type autorDTO struct {
	Sub  string `json:"sub"`
	Name string `json:"name"`
}

type liveDTO struct {
	Source      string     `json:"source"`
	StartedAt   *time.Time `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at"`
	ViewersNow  int32      `json:"viewers_now"`
	ViewersPeak int32      `json:"viewers_peak"`
}

type puedeDTO struct {
	Edit     bool `json:"edit"`
	Delete   bool `json:"delete"`
	Moderate bool `json:"moderate"`
}

func (h *Handlers) aDTO(v domain.Video, p service.Permisos) videoDTO {
	d := videoDTO{
		ID: v.ID, Tenant: v.Tenant, Owner: autorDTO{Sub: v.OwnerSub, Name: v.OwnerName},
		Kind: string(v.Kind), Status: string(v.Status), Visibility: string(v.Visibility),
		Title: v.Title, Description: v.Description,
		DurationMS: v.DurationMS, Width: v.Width, Height: v.Height,
		CommentsEnabled: v.CommentsEnabled, ChatEnabled: v.ChatEnabled, DownloadEnabled: v.DownloadEnabled,
		Views: v.Views, PublishedAt: v.PublishedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
		Can:          puedeDTO{Edit: p.Editar, Delete: p.Editar, Moderate: p.Moderar},
		calidadesDTO: calidadesDTOde(v),
	}
	if u := h.svc.PosterURL(v); u != "" {
		d.PosterURL = &u
	}
	if v.EsDirecto() {
		d.Live = &liveDTO{Source: string(v.LiveSource), StartedAt: v.LiveStartedAt, EndedAt: v.LiveEndedAt, ViewersNow: v.ViewersNow, ViewersPeak: v.ViewersPeak}
	}
	// Por qué falló es cosa de quien lo edita, no de quien lo mira.
	if p.Editar {
		d.Failure = v.Failure
	}
	return d
}

type paginaDTO struct {
	Videos     []videoDTO `json:"videos"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

// paginaDTOde pinta un listado. Los permisos de cada vídeo salen de la misma regla
// que `Ver`, sin volver a la base: en un listado quien lo edita es el dueño o
// quien administra la organización.
func (h *Handlers) paginaDTOde(p service.Pagina, quien service.Quien) paginaDTO {
	out := paginaDTO{Videos: h.listaDTO(p.Videos, quien), NextCursor: p.Siguiente}
	return out
}

func (h *Handlers) listaDTO(vs []domain.Video, quien service.Quien) []videoDTO {
	out := make([]videoDTO, 0, len(vs))
	for _, v := range vs {
		edita := !quien.Anonimo && quien.Tenant == v.Tenant && (quien.Sub == v.OwnerSub || quien.Admin)
		out = append(out, h.aDTO(v, service.Permisos{Ver: true, Editar: edita, Moderar: edita}))
	}
	return out
}

func soloDueno() service.Permisos { return service.Permisos{Ver: true, Editar: true, Moderar: true} }

// ── Público (§5.3) ──────────────────────────────────────────────────────────

// Explore — GET /public/explore?cursor=&limit=.
func (h *Handlers) Explore(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Explore(r.Context(), r.URL.Query().Get("cursor"), limiteDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"live":        h.listaDTO(p.Directos, service.Nadie),
		"videos":      h.listaDTO(p.Videos, service.Nadie),
		"next_cursor": p.Siguiente,
	})
}

// Canal — GET /public/c/{org}?cursor=&limit=.
func (h *Handlers) Canal(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "org")
	if !tenancy.ValidSlug(org) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	p, err := h.svc.Canal(r.Context(), org, r.URL.Query().Get("cursor"), limiteDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// ponytail: `name` es el slug hasta que Sytue ID exponga el nombre público de
	// una organización sin sesión; la web lo pinta con el del BFF si lo tiene.
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"org":         org,
		"name":        org,
		"videos":      h.listaDTO(p.Videos, service.Nadie),
		"next_cursor": p.Siguiente,
	})
}

// VerPublico — GET /public/videos/{id}: ficha de un vídeo public o unlisted.
func (h *Handlers) VerPublico(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	quien := service.Quien{Anonimo: true, IP: quienDe(r).IP}
	v, p, err := h.svc.Ver(r.Context(), quien, id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.aDTO(v, p))
}

// ReproducirPublico — GET /public/videos/{id}/play.
func (h *Handlers) ReproducirPublico(w http.ResponseWriter, r *http.Request) {
	h.reproducir(w, r, service.Quien{Anonimo: true, IP: quienDe(r).IP})
}

// OEmbed — GET /public/oembed?url=&maxwidth=&maxheight=&format=json (oEmbed 1.0,
// type video, con el <iframe> de /cast/embed/<id>).
func (h *Handlers) OEmbed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if f := q.Get("format"); f != "" && f != "json" {
		httpx.Fail(w, r, httpx.NewError(http.StatusNotImplemented, "unsupported_format", "Only JSON is supported."))
		return
	}
	id, ok := idDeURLDeVideo(q.Get("url"))
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("That URL is not a Sytue Cast video."))
		return
	}
	v, _, err := h.svc.Resolver(r.Context(), service.Nadie, id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	ancho, alto := 640, 360
	if v.Width != nil && v.Height != nil && *v.Width > 0 && *v.Height > 0 {
		alto = int(int64(ancho) * int64(*v.Height) / int64(*v.Width))
	}
	if m, _ := strconv.Atoi(q.Get("maxwidth")); m > 0 && m < ancho {
		alto = alto * m / ancho
		ancho = m
	}
	if m, _ := strconv.Atoi(q.Get("maxheight")); m > 0 && m < alto {
		ancho = ancho * m / alto
		alto = m
	}
	base := strings.TrimRight(h.cfg.PublicURL, "/")
	src := base + "/cast/embed/" + v.ID.String()
	out := map[string]any{
		"version":       "1.0",
		"type":          "video",
		"provider_name": "Sytue Cast",
		"provider_url":  base,
		"title":         v.Title,
		"author_name":   v.OwnerName,
		"width":         ancho,
		"height":        alto,
		"html": `<iframe src="` + src + `" width="` + strconv.Itoa(ancho) + `" height="` + strconv.Itoa(alto) +
			`" frameborder="0" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen title="` + escaparAtributo(v.Title) + `"></iframe>`,
	}
	// Sin thumbnail_url a propósito: el póster va por /media con un token que
	// caduca a las 6 h, y quien incrusta guarda la respuesta de oEmbed días.
	// ponytail: el relevo es una ruta pública y estable del póster para lo abierto.
	httpx.JSON(w, r, http.StatusOK, out)
}

// idDeURLDeVideo saca el id de una URL de la web (/cast/watch/<id>, /cast/embed/<id>)
// o de la API (/videos/<id>). Solo por la forma: el vídeo se resuelve después.
func idDeURLDeVideo(u string) (uuid.UUID, bool) {
	u = strings.TrimSpace(u)
	if u == "" {
		return uuid.Nil, false
	}
	partes := strings.Split(strings.Trim(u, "/"), "/")
	for i := len(partes) - 1; i >= 0; i-- {
		segmento, _, _ := strings.Cut(partes[i], "?")
		if id, err := uuid.Parse(segmento); err == nil {
			return id, true
		}
	}
	return uuid.Nil, false
}

func escaparAtributo(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(s)
}

// ── Con sesión (§5.2) ───────────────────────────────────────────────────────

// Ver — GET /api/v1/videos/{id}.
func (h *Handlers) Ver(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, p, err := h.svc.Ver(r.Context(), quienDe(r), id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.aDTO(v, p))
}

// Biblioteca — GET /api/v1/videos?scope=mine|org&status=&q=&cursor=&limit=.
func (h *Handlers) Biblioteca(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	quien := quienDe(r)
	scope := q.Get("scope")
	if scope != "" && scope != "mine" && scope != "org" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("scope must be 'mine' or 'org'."))
		return
	}
	p, err := h.svc.Biblioteca(r.Context(), quien, service.Filtro{
		Mios:   scope == "mine",
		Status: domain.Status(q.Get("status")),
		Q:      q.Get("q"),
		Cursor: q.Get("cursor"),
		Limite: limiteDe(r),
	})
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.paginaDTOde(p, quien))
}

// Compartidos — GET /api/v1/videos/shared?cursor=&limit=.
func (h *Handlers) Compartidos(w http.ResponseWriter, r *http.Request) {
	quien := quienDe(r)
	p, err := h.svc.Compartidos(r.Context(), quien, r.URL.Query().Get("cursor"), limiteDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.paginaDTOde(p, quien))
}

type altaDTO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	// Qualities: las calidades elegidas (alturas); vacía = la recomendada.
	Qualities []int `json:"qualities"`
}

// Crear — POST /api/v1/videos {title, description?, visibility?} → vídeo `uploading`.
func (h *Handlers) Crear(w http.ResponseWriter, r *http.Request) {
	var in altaDTO
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.svc.Crear(r.Context(), quienDe(r), service.Alta{
		Kind: domain.KindUpload, Title: in.Title, Description: in.Description, Visibility: domain.Visibility(in.Visibility),
		Qualities: in.Qualities,
	})
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, h.aDTO(v, soloDueno()))
}

// SubirOriginal — PUT /api/v1/videos/{id}/original (cuerpo = el fichero).
func (h *Handlers) SubirOriginal(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if r.ContentLength <= 0 {
		httpx.Fail(w, r, httpx.NewError(http.StatusLengthRequired, "length_required", "Content-Length is required."))
		return
	}
	tipo, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";")
	v, err := h.svc.SubirOriginal(r.Context(), quienDe(r), id, r.Body, r.ContentLength, strings.TrimSpace(tipo))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.aDTO(v, soloDueno()))
}

type cambiosDTO struct {
	Title           *string `json:"title"`
	Description     *string `json:"description"`
	Visibility      *string `json:"visibility"`
	CommentsEnabled *bool   `json:"comments_enabled"`
	ChatEnabled     *bool   `json:"chat_enabled"`
	DownloadEnabled *bool   `json:"download_enabled"`
}

// Actualizar — PATCH /api/v1/videos/{id}.
func (h *Handlers) Actualizar(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in cambiosDTO
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c := domain.Cambios{
		Title: in.Title, Description: in.Description,
		CommentsEnabled: in.CommentsEnabled, ChatEnabled: in.ChatEnabled, DownloadEnabled: in.DownloadEnabled,
	}
	if in.Visibility != nil {
		vis := domain.Visibility(*in.Visibility)
		c.Visibility = &vis
	}
	v, err := h.svc.Actualizar(r.Context(), quienDe(r), id, c)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.aDTO(v, soloDueno()))
}

// Borrar — DELETE /api/v1/videos/{id}.
func (h *Handlers) Borrar(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Borrar(r.Context(), quienDe(r), id); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// SubirPoster — PUT /api/v1/videos/{id}/poster (imagen ≤ 5 MiB).
func (h *Handlers) SubirPoster(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if r.ContentLength <= 0 {
		httpx.Fail(w, r, httpx.NewError(http.StatusLengthRequired, "length_required", "Content-Length is required."))
		return
	}
	tipo, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";")
	v, err := h.svc.SubirPoster(r.Context(), quienDe(r), id, r.Body, r.ContentLength, strings.TrimSpace(tipo))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, h.aDTO(v, soloDueno()))
}

// PosterDeFotograma — POST /api/v1/videos/{id}/poster:frame {at_ms}.
func (h *Handlers) PosterDeFotograma(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		AtMS int64 `json:"at_ms"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.svc.PosterDeFotograma(r.Context(), quienDe(r), id, in.AtMS)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, h.aDTO(v, soloDueno()))
}

// ── Reproducción ────────────────────────────────────────────────────────────

type playDTO struct {
	Kind    string  `json:"kind"`
	HLSURL  *string `json:"hls_url"`
	WebMURL *string `json:"webm_url"`
	// WebMQualities: los WebM que hay; cada uno en webm/<q>p.webm junto a webm_url.
	WebMQualities []int     `json:"webm_qualities"`
	PosterURL     *string   `json:"poster_url"`
	DownloadURL   *string   `json:"download_url,omitempty"`
	DurationMS    *int64    `json:"duration_ms"`
	Live          *LivePlay `json:"live,omitempty"`
}

func opcional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Reproducir — GET /api/v1/videos/{id}/play.
func (h *Handlers) Reproducir(w http.ResponseWriter, r *http.Request) {
	h.reproducir(w, r, quienDe(r))
}

func (h *Handlers) reproducir(w http.ResponseWriter, r *http.Request, quien service.Quien) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, _, play, err := h.svc.Reproducir(r.Context(), quien, id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	out := playDTO{
		Kind: string(play.Kind), HLSURL: opcional(play.HLSURL), WebMURL: opcional(play.WebMURL), WebMQualities: play.WebMCalidades,
		PosterURL: opcional(play.PosterURL), DownloadURL: opcional(play.DownloadURL), DurationMS: play.DurationMS,
	}
	if v.Status == domain.StatusLive {
		live, err := h.directo.Play(r.Context(), v, quien)
		if err != nil {
			httpx.Fail(w, r, traducir(err))
			return
		}
		out.Live = live
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// ── Personas ────────────────────────────────────────────────────────────────

type personaDTO struct {
	UserID   string    `json:"user_id"`
	Name     string    `json:"name"`
	Username string    `json:"username,omitempty"`
	AddedAt  time.Time `json:"added_at"`
}

func personaDTOde(p domain.Persona) personaDTO {
	return personaDTO{UserID: p.UserID, Name: p.Name, Username: p.Username, AddedAt: p.AddedAt}
}

// Personas — GET /api/v1/videos/{id}/people.
func (h *Handlers) Personas(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ps, err := h.svc.Personas(r.Context(), quienDe(r), id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	out := make([]personaDTO, 0, len(ps))
	for _, p := range ps {
		out = append(out, personaDTOde(p))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"people": out})
}

// Compartir — POST /api/v1/videos/{id}/people {"query":"@ana"}.
func (h *Handlers) Compartir(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Query string `json:"query"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := h.svc.Compartir(r.Context(), quienDe(r), id, in.Query)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, personaDTOde(p))
}

// Descompartir — DELETE /api/v1/videos/{id}/people/{user_id}.
func (h *Handlers) Descompartir(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.Descompartir(r.Context(), quienDe(r), id, chi.URLParam(r, "user_id")); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.NoContent(w)
}

// ── Cuota ───────────────────────────────────────────────────────────────────

// Cuota — GET /api/v1/quota: la cuota del plan según Drive más lo que ocupa Cast.
func (h *Handlers) Cuota(w http.ResponseWriter, r *http.Request) {
	q, err := h.svc.Cuota(r.Context(), quienDe(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"limit_bytes":     q.LimitBytes,
		"used_bytes":      q.UsedBytes,
		"available_bytes": q.AvailableBytes,
		"cast_bytes":      q.CastBytes,
	})
}
