// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"cmp"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// Compartir un álbum con UNA persona de la organización (store/personas.go):
//
//	GET    /albums/{album}/people          con quién está compartido (su dueña)
//	PUT    /albums/{album}/people/{sub}    {permission: read|write} — compartir o cambiar el nivel
//	DELETE /albums/{album}/people/{sub}    dejar de compartir
//	GET    /shared/albums                  «Compartido conmigo»
//	GET    /shared/albums/{album}          un álbum compartido conmigo, con sus fotos
//	GET    /shared/albums/{album}/assets/{id}/variants/{variante}
//
// La persona llega por su `sub`: la web lo saca del correo en su BFF con
// `directory:resolve`, igual que Drive, y la app nativa con el mismo directorio; aquí
// se vuelve a preguntar a Sytue ID que sea de la organización antes de guardarlo.
//
// Para ver o para editar (edicion.go: renombrar, quitar y añadir fotos, que se copian al
// Drive de la dueña). Las fotos se sirven como por el enlace —solo lo ya generado—; se
// generan al compartir, con la sesión de la dueña, y al añadirlas, con la de quien añade.

// maxSujeto acota el `sub` de la ruta: es un identificador, no texto libre.
const maxSujeto = 255

// VentanaAviso es cada cuánto, como mucho, se avisa a una persona de que se le ha
// compartido EL MISMO álbum —y se vuelve a preparar—: retirar y volver a compartir no le
// llena la campana ni pone al servicio a generar derivadas en bucle.
const VentanaAviso = 12 * time.Hour

type personaJSON struct {
	Sujeto  string    `json:"subject"`
	Permiso string    `json:"permission"`
	Creado  time.Time `json:"createdAt"`
}

// albumCompartidoJSON es un álbum de «Compartido conmigo»: sin el enlace de su dueña,
// con quién lo compartió.
type albumCompartidoJSON struct {
	ID           uuid.UUID  `json:"id"`
	Title        string     `json:"title"`
	CoverAssetID *uuid.UUID `json:"coverAssetId"`
	Count        int        `json:"count"`
	SharedBy     string     `json:"sharedBy"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	// Permission es con qué nivel me lo compartieron: `read` o `write` (edicion.go).
	Permission string `json:"permission"`
}

func aCompartidoJSON(al domain.Album) albumCompartidoJSON {
	return albumCompartidoJSON{
		ID: al.ID, Title: al.Titulo, CoverAssetID: al.PortadaID, Count: al.Fotos,
		SharedBy: al.Duenyo, UpdatedAt: al.ActualizadoEn, Permission: nivel(al.Editable),
	}
}

func sujetoDeRuta(r *http.Request) (string, error) {
	s := strings.TrimSpace(chi.URLParam(r, "sub"))
	if s == "" || len(s) > maxSujeto {
		return "", httpx.ErrBadRequest.WithCode("invalid_subject")
	}
	return s, nil
}

// GET /api/v1/albums/{album}/people.
func (h *Handlers) PersonasDelAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	ps, err := h.store.PersonasDelAlbum(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	out := make([]personaJSON, 0, len(ps))
	for _, p := range ps {
		out = append(out, personaJSON{Sujeto: p.Sujeto, Permiso: nivel(p.Editar), Creado: p.Creado})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"people": out})
}

// PUT /api/v1/albums/{album}/people/{sub} {permission?: "read"}.
func (h *Handlers) CompartirConPersona(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	sujeto, err := sujetoDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var cuerpo struct {
		Permission string `json:"permission"`
	}
	if err := httpx.DecodeJSON(w, r, &cuerpo); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if cuerpo.Permission != "" && cuerpo.Permission != "read" && cuerpo.Permission != "write" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_permission").WithMessage("permission must be read or write."))
		return
	}
	editar := cuerpo.Permission == "write"
	// Primero que el álbum es suyo: no se le pregunta a Sytue ID por nadie en nombre de
	// quien no puede compartirlo.
	if _, err := h.store.Album(r.Context(), id, sub(r)); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// El que decide si la persona es de la organización es Sytue ID, con la sesión de
	// quien comparte: un permiso a un identificador inventado no se guarda.
	if _, _, err := peticiones.Persona(r.Context(), h.cfg.IDBaseURL, authn.Token(r), sujeto); err != nil {
		if errors.Is(err, peticiones.ErrSinPersona) {
			httpx.Fail(w, r, httpx.ErrNotFound.WithCode("unknown_person"))
			return
		}
		httpx.Fail(w, r, httpx.ErrUnavailable.Wrap(err))
		return
	}
	nueva, titulo, err := h.store.CompartirConPersona(r.Context(), id, sub(r), sujeto, editar)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	org := tenancy.MustFrom(r.Context()).Slug
	if nueva && h.avisados.Primera(org+"|"+id.String()+"|"+sujeto) {
		// Lo que va a ver tiene que estar generado: con su sesión no hay original que leer.
		h.prepararElAlbumCompartido(r, id)
		c, _ := authn.FromContext(r.Context())
		h.hub.SendAsync(r.Context(), hubnotify.Compartido(org, sujeto, "photos", "photos.shared", titulo,
			cmp.Or(strings.TrimSpace(c.Name), c.Email), hubnotify.EnlaceAlbumCompartido(h.cfg.PublicURL, org, id.String())))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"subject": sujeto, "permission": nivel(editar), "created": nueva})
}

// nivel es el `permission` del contrato común: `write` si edita, `read` si solo ve.
func nivel(editar bool) string {
	if editar {
		return "write"
	}
	return "read"
}

// DELETE /api/v1/albums/{album}/people/{sub}.
func (h *Handlers) DejarDeCompartirConPersona(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	sujeto, err := sujetoDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.DejarDeCompartirConPersona(r.Context(), id, sub(r), sujeto); err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/shared/albums.
func (h *Handlers) AlbumesCompartidos(w http.ResponseWriter, r *http.Request) {
	lista, err := h.store.AlbumesCompartidos(r.Context(), sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	items := make([]albumCompartidoJSON, 0, len(lista))
	for _, al := range lista {
		items = append(items, aCompartidoJSON(al))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GET /api/v1/shared/albums/{album}: el álbum y lo mínimo de sus fotos, como el
// enlace —ni el fichero de Drive, ni la cámara, ni el lugar—.
func (h *Handlers) AlbumCompartido(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	al, err := h.store.AlbumCompartido(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	fotos, err := h.store.FotosDelAlbumPublico(r.Context(), al)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"album": aCompartidoJSON(al), "items": fotosPublicasJSON(fotos)})
}

// GET /api/v1/shared/albums/{album}/assets/{id}/variants/{variante}.
func (h *Handlers) VarianteCompartida(w http.ResponseWriter, r *http.Request) {
	id, err := albumDeRuta(r)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	al, err := h.store.AlbumCompartido(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	h.servirVarianteDelAlbum(w, r, al, cacheCompartida)
}
