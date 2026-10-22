// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
)

// # Editar un álbum que otra persona me ha compartido (decisión del dueño, 2026-09-28)
//
// Quien tiene edición renombra el álbum, quita cualquier foto y añade fotos SUYAS por las
// mismas rutas que su dueña (`PATCH /albums/{album}`, `DELETE /albums/{album}/assets/{id}`,
// `POST /albums/{album}/assets`); DuenyaParaEditar decide quién puede. Lo que añade:
//
//   - Se COPIA al Drive de la dueña (Drive `/internal/photos/copies`, con el secreto de la
//     suite): el fichero nuevo es de ella, va a /Fotos y cuenta contra la cuota de la
//     organización. Si no cabe, 507 `quota_exceeded`, y lo que ya se había copiado se
//     queda en el álbum. Una foto que la dueña ya tiene (misma huella) no se copia otra vez.
//   - En Photos pasa a ser una foto DE LA DUEÑA, con la fecha, las medidas, la orientación
//     y la cámara de la original, y SIN el lugar, la descripción, las etiquetas ni la
//     marca de favorita de quien la añade: lo que Photos enseña de ella es solo lo que hace
//     falta para colocarla. El fichero de Drive es copia exacta, EXIF incluido, como ya lo
//     era en el Drive de la organización (que es de todo el tenant).
//   - Retirar el permiso no se lleva nada: lo copiado es de la dueña.
//   - A la dueña le llega el aviso `photos.album.added` —quién y cuántas—, uno por
//     persona y álbum cada VentanaAvisoAnyadidos.

// VentanaAvisoAnyadidos es cada cuánto, como mucho, se avisa a la dueña de que una misma
// persona le añade fotos a un mismo álbum.
const VentanaAvisoAnyadidos = time.Hour

// errSinCuota es la respuesta cuando la copia no cabe en el Drive de la organización.
var errSinCuota = httpx.NewError(http.StatusInsufficientStorage, "quota_exceeded",
	"There is no storage left in the organisation's Drive to copy the photos.")

// MaxAnyadirEditora es cuántas fotos añade de una vez quien edita: cada una es una copia
// en Drive, y una tanda de mil pasaría minutos copiando con el permiso comprobado al
// principio. La web y la app mandan lo que se ha marcado; más de esto se parte.
const MaxAnyadirEditora = 100

// anyadirComoEditora copia a la dueña las fotos de quien edita y las pone en el álbum.
//
// Nunca deja copias sueltas: lo copiado se añade al álbum aunque la tanda se corte a
// mitad (sin espacio, o un fallo de Drive), y lo que no se puede copiar —una foto cuyo
// original ya no está, o que no es suya en Drive— se salta, como en «añadir».
func (h *Handlers) anyadirComoEditora(w http.ResponseWriter, r *http.Request, album uuid.UUID, duenya string, ids []uuid.UUID) {
	if h.drive == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	if len(ids) > MaxAnyadirEditora {
		httpx.Fail(w, r, traducir(fmt.Errorf("%w: más de %d fotos de una vez", domain.ErrInvalidInput, MaxAnyadirEditora)))
		return
	}
	ctx := r.Context()
	yo := sub(r)
	org := tenancy.MustFrom(ctx).Slug
	var copias []domain.Activo
	var corte error
	vistas := map[uuid.UUID]bool{}
	for _, id := range ids {
		if vistas[id] {
			continue
		}
		vistas[id] = true
		// Solo fotos SUYAS y vivas: una ajena o en la papelera se salta.
		a, err := h.store.Activo(ctx, id, yo)
		if err != nil {
			continue
		}
		copia, _, err := h.drive.CopiarParaDuenya(ctx, h.cfg.HubInternalSecret, org, a.DriveID, duenya, yo)
		if errors.Is(err, drive.ErrNoEncontrado) {
			continue
		}
		if err != nil {
			corte = err
			break
		}
		nueva, err := h.store.AltaDeCopia(ctx, domain.Activo{
			Duenyo: duenya, DriveID: copia, Nombre: a.Nombre, Tipo: a.Tipo, Bytes: a.Bytes,
			Ancho: a.Ancho, Alto: a.Alto, Orientacion: a.Orientacion, TomadaEn: a.TomadaEn,
			Camara: a.Camara, Objetivo: a.Objetivo,
		})
		// Esa foto ya estaba en Photos y no es de la dueña, o la tiene en la papelera: no
		// se le da ni se le saca de ahí.
		if err != nil {
			continue
		}
		copias = append(copias, nueva)
	}
	idsCopias := make([]uuid.UUID, len(copias))
	for i, c := range copias {
		idsCopias[i] = c.ID
	}
	// Con `yo`: el store vuelve a comprobar en su transacción que sigo pudiendo editarlo.
	n, err := h.store.AnyadirAlAlbum(ctx, album, yo, idsCopias)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	if len(copias) > 0 && h.ingesta != nil {
		if token := bearerDe(r); token != "" {
			// Lo que va a verse en el álbum tiene que estar generado; con el token de quien
			// añade, que lee el Drive de la organización. Con el mismo tope que al compartir.
			prep := copias[:min(len(copias), TopeDePreparacion)]
			recetas := make([]domain.Receta, len(prep))
			for i := range recetas {
				recetas[i] = domain.RecetaVacia()
			}
			pctx, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
			h.prepararEnSegundoPlano(pctx, cancelar, token, album, prep, recetas)
		}
	}
	if n > 0 && h.anyadidos.Primera("anyadidas|"+org+"|"+album.String()+"|"+yo) {
		titulo := ""
		if al, err := h.store.Album(ctx, album, duenya); err == nil {
			titulo = al.Titulo
		}
		c, _ := authn.FromContext(ctx)
		actor := cmp.Or(strings.TrimSpace(c.Name), c.Email)
		h.hub.SendAsync(ctx, avisoDeAnyadidas(h.cfg.PublicURL, org, duenya, album.String(), titulo, actor, n))
	}
	switch {
	case errors.Is(corte, drive.ErrSinCuota):
		httpx.Fail(w, r, errSinCuota.WithParams(map[string]any{"added": n}))
	case corte != nil:
		httpx.Fail(w, r, fmt.Errorf("copiar la foto al Drive de la dueña: %w", corte))
	default:
		httpx.JSON(w, r, http.StatusOK, map[string]int{"added": n})
	}
}

// avisarRenombrado le dice a la dueña que alguien con edición ha renombrado su álbum
// (`photos.album.renamed`), uno por persona y álbum cada VentanaAvisoAnyadidos.
func (h *Handlers) avisarRenombrado(r *http.Request, al domain.Album) {
	ctx := r.Context()
	org, yo := tenancy.MustFrom(ctx).Slug, sub(r)
	if !h.anyadidos.Primera("renombrado|" + org + "|" + al.ID.String() + "|" + yo) {
		return
	}
	c, _ := authn.FromContext(ctx)
	quien := cmp.Or(strings.TrimSpace(c.Name), c.Email, "Alguien")
	h.hub.SendAsync(ctx, hubnotify.Notification{
		Tenant: org, Subject: al.Duenyo, Source: "photos", Type: "photos.album.renamed", Level: "info",
		Title:  fmt.Sprintf("%s ha renombrado tu álbum a «%s»", quien, al.Titulo),
		URL:    hubnotify.EnlaceAlbum(h.cfg.PublicURL, org, al.ID.String()),
		Params: map[string]string{"actor": quien, "name": al.Titulo},
	})
}

// avisoDeAnyadidas es el «X ha añadido N fotos a «álbum»» de la dueña.
func avisoDeAnyadidas(pub, org, duenya, album, titulo, actor string, n int) hubnotify.Notification {
	quien := cmp.Or(actor, "Alguien")
	return hubnotify.Notification{
		Tenant: org, Subject: duenya, Source: "photos", Type: "photos.album.added", Level: "info",
		Title:  fmt.Sprintf("%s ha añadido %d fotos a «%s»", quien, n, titulo),
		URL:    hubnotify.EnlaceAlbum(pub, org, album),
		Params: map[string]string{"actor": quien, "name": titulo, "count": strconv.Itoa(n)},
	}
}
