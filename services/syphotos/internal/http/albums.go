package http

import (
	"fmt"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

type albumJSON struct {
	ID           uuid.UUID  `json:"id"`
	Title        string     `json:"title"`
	CoverAssetID *uuid.UUID `json:"coverAssetId"`
	Count        int        `json:"count"`
	// Shared solo va en la vista de su dueña: dice si el álbum tiene enlace. El
	// enlace en sí solo sale al generarlo (POST …/share): no se guarda (FILE-10).
	Shared    bool      `json:"shared,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func aAlbumJSON(al domain.Album) albumJSON {
	return albumJSON{
		ID: al.ID, Title: al.Titulo, CoverAssetID: al.PortadaID, Count: al.Fotos,
		Shared:    al.Compartido,
		CreatedAt: al.CreadoEn, UpdatedAt: al.ActualizadoEn,
	}
}

func albumDeRuta(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "album"))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: id de álbum inválido", domain.ErrInvalidInput)
	}
	return id, nil
}

// GET /api/v1/albums.
func (h *Handlers) Albumes(w http.ResponseWriter, r *http.Request) {
	lista, err := h.store.Albumes(r.Context(), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	items := make([]albumJSON, 0, len(lista))
	for _, al := range lista {
		items = append(items, aAlbumJSON(al))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// POST /api/v1/albums {title}.
func (h *Handlers) CrearAlbum(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Title string `json:"title"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	al, err := h.store.CrearAlbum(r.Context(), sub(r), cuerpo.Title)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, aAlbumJSON(al))
}

// GET /api/v1/albums/{album}.
func (h *Handlers) Album(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	al, err := h.store.Album(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, aAlbumJSON(al))
}

// PATCH /api/v1/albums/{album} {title, coverAssetId?}.
func (h *Handlers) RenombrarAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	var cuerpo struct {
		Title        string     `json:"title"`
		CoverAssetID *uuid.UUID `json:"coverAssetId"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// La dueña, o quien lo tiene compartido para editar: lo decide el store (edicion.go).
	al, err := h.store.RenombrarAlbum(r.Context(), id, sub(r), cuerpo.Title, cuerpo.CoverAssetID)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if al.Duenyo != sub(r) {
		// El enlace es una credencial de la dueña: quien edita no lo ve. Y a ella se le
		// dice quién le ha cambiado el título, que no lo hace a escondidas.
		al.Compartido = false
		h.avisarRenombrado(r, al)
	}
	httpx.JSON(w, r, http.StatusOK, aAlbumJSON(al))
}

// DELETE /api/v1/albums/{album}.
func (h *Handlers) BorrarAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if err := h.store.BorrarAlbum(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/albums/{album}/assets.
func (h *Handlers) FotosDelAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	fotos, err := h.store.FotosDelAlbum(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	items := make([]activoJSON, 0, len(fotos))
	for _, a := range fotos {
		items = append(items, aJSON(a))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// POST /api/v1/albums/{album}/assets {ids: [...]}.
func (h *Handlers) AnyadirAlAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	var cuerpo struct {
		IDs []uuid.UUID `json:"ids"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(cuerpo.IDs) > 1000 {
		httpx.Fail(w, r, traducir(fmt.Errorf("%w: más de 1000 fotos de una vez", domain.ErrInvalidInput)))
		return
	}
	duenya, editora, err := h.store.DuenyaParaEditar(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if editora {
		h.anyadirComoEditora(w, r, id, duenya, cuerpo.IDs)
		return
	}
	n, err := h.store.AnyadirAlAlbum(r.Context(), id, sub(r), cuerpo.IDs)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]int{"added": n})
}

// DELETE /api/v1/albums/{album}/assets/{id}.
func (h *Handlers) QuitarDelAlbum(w http.ResponseWriter, r *http.Request) {
	album, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	foto, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// La dueña o quien lo edita: el store lo comprueba en su transacción.
	if err := h.store.QuitarDelAlbum(r.Context(), album, sub(r), foto); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/trash.
func (h *Handlers) Papelera(w http.ResponseWriter, r *http.Request) {
	fotos, err := h.store.Papelera(r.Context(), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	items := make([]activoJSON, 0, len(fotos))
	for _, a := range fotos {
		items = append(items, aJSON(a))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// POST /api/v1/assets/{id}/restore.
func (h *Handlers) Restaurar(w http.ResponseWriter, r *http.Request) {
	id, err := idDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.Restaurar(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/trash        vacía la papelera entera
// DELETE /api/v1/trash/{id}   borra una foto de la papelera, para siempre
//
// Una papelera de la que no se puede sacar nada no es una papelera: lo borrado
// se quedaba ahí para siempre y sus derivadas ocupando sitio.
//
// NO TOCA DRIVE: el original sigue en su carpeta. Aquí se va el índice, o sea
// la foto deja de estar en la galería; reindexar esa carpeta la traería de
// vuelta, que es lo que tiene que pasar.
func (h *Handlers) Purgar(w http.ResponseWriter, r *http.Request) {
	var id *uuid.UUID
	if chi.URLParam(r, "id") != "" {
		v, err := idDeRuta(r)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		id = &v
	}
	purgadas, err := h.store.Purgar(r.Context(), sub(r), id)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	h.purgarDerivadas(r, purgadas)
	httpx.JSON(w, r, http.StatusOK, map[string]int{"deleted": len(purgadas)})
}

// purgarDerivadas se lleva del almacén lo generado para fotos que ya no
// existen. El trabajo lo hace `service.Limpieza`, que comparte con el worker de
// retención; aquí solo se sabe de qué organización es la petición.
func (h *Handlers) purgarDerivadas(r *http.Request, purgadas []store.Purgada) {
	if len(purgadas) == 0 || h.limpieza == nil {
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		return
	}
	fallos := 0
	for _, p := range purgadas {
		fallos += h.limpieza.BorrarDerivadas(r.Context(), t.Slug, p.ID, p.Huella)
	}
	if fallos > 0 {
		log.From(r.Context()).Warn("quedaron derivadas sin borrar de fotos purgadas",
			"fotos", len(purgadas), "fallos", fallos)
	}
}
