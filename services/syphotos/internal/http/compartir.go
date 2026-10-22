// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// POST /api/v1/albums/{album}/share
//
// Devuelve el token del enlace, creándolo la primera vez. Es idempotente:
// volver a pulsar «compartir» devuelve el MISMO enlace, no uno nuevo que
// rompería los ya mandados.
func (h *Handlers) CompartirAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	token, err := h.store.CompartirAlbum(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	h.prepararElAlbumCompartido(r, id)
	httpx.JSON(w, r, http.StatusOK, map[string]string{"token": token})
}

// DELETE /api/v1/albums/{album}/share.
func (h *Handlers) DejarDeCompartirAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if err := h.store.DejarDeCompartirAlbum(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TopeDePreparacion acota cuántas fotos se dejan listas al compartir un álbum.
const TopeDePreparacion = 200

// prepararElAlbumCompartido genera las derivadas del álbum EN SEGUNDO PLANO.
//
// POR QUÉ HACE FALTA: quien abre el enlace no tiene sesión, así que este
// servicio no puede pedirle a Drive el original en su nombre —Drive es quien
// comprueba los permisos, y un token de servicio aquí sería una puerta trasera
// a la unidad entera—. O sea: por el enlace solo se puede servir lo que YA
// está generado. Se genera ahora, con el token de quien comparte, que es el
// único momento en que lo tenemos y quiere justamente eso.
//
// El contexto se desliga de la petición: la respuesta sale enseguida —quien
// comparte quiere su enlace, no una barra de progreso— y el trabajo sigue.
//
// ponytail: hasta `TopeDePreparacion` fotos y una sola pasada. Un álbum mayor
// deja las últimas sin preparar y sus huecos se llenan cuando su dueña las
// mire; el sitio de arreglarlo del todo es una cola de trabajos, que este
// servicio todavía no tiene.
func (h *Handlers) prepararElAlbumCompartido(r *http.Request, album uuid.UUID) {
	token := bearerDe(r)
	if h.ingesta == nil || token == "" {
		return
	}
	al, err := h.store.Album(r.Context(), album, sub(r))
	if err != nil {
		return
	}
	fotos, err := h.store.FotosDelAlbum(r.Context(), album, sub(r))
	if err != nil || len(fotos) == 0 {
		return
	}
	if len(fotos) > TopeDePreparacion {
		fotos = fotos[:TopeDePreparacion]
	}
	// Se copian los datos que hacen falta: el contexto de la petición muere al
	// contestar y el suyo no puede depender de él.
	ctx, cancelar := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Minute)
	recetas := make([]domain.Receta, len(fotos))
	for i, f := range fotos {
		rec, err := h.store.Receta(r.Context(), f.ID, sub(r))
		if err != nil {
			rec = domain.RecetaVacia()
		}
		recetas[i] = rec
	}
	h.prepararEnSegundoPlano(ctx, cancelar, token, al.ID, fotos, recetas)
}

// prepararEnSegundoPlano genera las derivadas de `fotos` con sus recetas y el token de
// quien las pide, en una rutina propia que acaba con `cancelar`.
func (h *Handlers) prepararEnSegundoPlano(ctx context.Context, cancelar context.CancelFunc, token string,
	album uuid.UUID, fotos []domain.Activo, recetas []domain.Receta,
) {
	go func() {
		defer cancelar()
		fallos := 0
		for i, f := range fotos {
			if err := h.generarConContexto(ctx, token, f, recetas[i]); err != nil {
				fallos++
			}
		}
		if fallos > 0 {
			log.From(ctx).Warn("quedaron fotos sin preparar en un álbum compartido",
				"album", album, "fotos", len(fotos), "fallos", fallos)
		}
	}()
}

// generarConContexto es `generar` sin petición detrás: el mismo semáforo, para
// que preparar un álbum no compita con quien está mirando su galería.
func (h *Handlers) generarConContexto(ctx context.Context, token string, a domain.Activo, rec domain.Receta) error {
	select {
	case h.derivando <- struct{}{}:
		defer func() { <-h.derivando }()
	case <-ctx.Done():
		return ctx.Err()
	}
	return h.ingesta.Derivar(ctx, token, a, rec)
}

// ── La superficie pública: sin sesión, con el token por credencial ───────────

// fotoPublicaJSON es lo MÍNIMO para pintar la cuadrícula.
//
// No van ni el fichero de Drive, ni la cámara, ni el lugar, ni si es favorita:
// compartir un álbum es enseñar unas fotos, no publicar los metadatos de quien
// las hizo ni dónde estaba.
type fotoPublicaJSON struct {
	ID     uuid.UUID `json:"id"`
	Nombre string    `json:"name"`
	Ancho  int       `json:"width"`
	Alto   int       `json:"height"`
	Cuando time.Time `json:"takenAt"`
}

// fotosPublicasJSON es la cuadrícula de un álbum abierto por su enlace o compartido
// con quien lo mira: lo mismo en los dos.
func fotosPublicasJSON(fotos []store.FotoPublica) []fotoPublicaJSON {
	items := make([]fotoPublicaJSON, 0, len(fotos))
	for _, f := range fotos {
		ancho, alto := f.Activo.Vistas()
		cuando, _ := f.Activo.FechaDeGaleria()
		items = append(items, fotoPublicaJSON{
			ID: f.Activo.ID, Nombre: f.Activo.Nombre,
			Ancho: ancho, Alto: alto, Cuando: cuando,
		})
	}
	return items
}

// GET /public/albums/{token}.
func (h *Handlers) AlbumPublico(w http.ResponseWriter, r *http.Request) {
	al, fotos, err := h.albumDelEnlace(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	items := fotosPublicasJSON(fotos)
	noIndexar(w)
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"album": map[string]any{"title": al.Titulo, "count": len(items)},
		"items": items,
	})
}

// GET /public/albums/{token}/assets/{id}/variants/{variante}
//
// Solo SIRVE lo que ya está generado: por aquí no hay sesión con la que
// pedirle a Drive el original, así que no se puede fabricar nada. Lo que falte
// se prepara al compartir el álbum (ver prepararElAlbumCompartido) y, si aun
// así falta, es un 404 y un hueco, no un error de quien mira.
func (h *Handlers) VariantePublica(w http.ResponseWriter, r *http.Request) {
	al, err := h.store.AlbumPorToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	h.servirVarianteDelAlbum(w, r, al, cachePublica)
}

// Lo que se deja guardar en el navegador de quien mira. Por el enlace, para siempre: la
// clave lleva la huella de la receta y el token es la credencial. Por «Compartido
// conmigo», nada sin volver a preguntar: la verja es que el álbum SIGA compartido con
// esa persona, y una copia de un año seguiría enseñándoselo después de retirárselo.
const (
	cachePublica    = "private, max-age=31536000, immutable"
	cacheCompartida = "private, no-cache"
)

// servirVarianteDelAlbum sirve una foto de un álbum YA AUTORIZADO —por su enlace o
// porque se lo compartieron a quien pide—, solo si está en él y ya generada.
func (h *Handlers) servirVarianteDelAlbum(w http.ResponseWriter, r *http.Request, al domain.Album, cache string) {
	variante := domain.Variante(chi.URLParam(r, "variante"))
	if _, ok := variante.LadoMayor(); !ok {
		httpx.Fail(w, r, traducir(domain.ErrNotFound))
		return
	}
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(domain.ErrNotFound))
		return
	}
	// Que la foto esté EN ESE ÁLBUM es lo que autoriza a servirla: sin esta
	// comprobación, un álbum válido sería una licencia para pedir cualquier
	// identificador de la organización.
	f, err := h.store.FotoDelAlbumPublico(r.Context(), al, id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	huella := f.Huella
	if huella == "" {
		huella = huellaVacia
	}
	formato := "jpg"
	contentType := "image/jpeg"
	if service.EsPNG(f.Activo) {
		formato = "png"
		contentType = "image/png"
	}
	clave, err := (domain.Derivada{
		ActivoID: f.Activo.ID, Huella: huella, Variante: variante, Formato: formato,
	}).Clave()
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	rc, err := h.objetos.Get(r.Context(), tenancy.Tenant{Slug: al.Tenant}, clave)
	if errors.Is(err, objectstore.ErrNotFound) {
		httpx.Fail(w, r, traducir(domain.ErrNotFound))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rc.Close()

	noIndexar(w)
	w.Header().Set("Content-Type", contentType)
	// `private` siempre: una caché compartida guardando fotos de un álbum es justo lo
	// que nadie pidió al compartirlo.
	w.Header().Set("Cache-Control", cache)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.Copy(w, rc); err != nil {
		log.From(r.Context()).Warn("derivada pública cortada a mitad de envío",
			"asset", f.Activo.ID, "error", err)
	}
}

// albumDelEnlace resuelve el álbum y sus fotos a partir del token de la ruta.
func (h *Handlers) albumDelEnlace(r *http.Request) (domain.Album, []store.FotoPublica, error) {
	al, err := h.store.AlbumPorToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		return domain.Album{}, nil, err
	}
	fotos, err := h.store.FotosDelAlbumPublico(r.Context(), al)
	return al, fotos, err
}

// noIndexar: «cualquiera con el enlace» no es «cualquiera». Un álbum de
// familia en el índice de un buscador sería exactamente lo que nadie pidió al
// pulsar «compartir».
func noIndexar(w http.ResponseWriter) {
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
}
