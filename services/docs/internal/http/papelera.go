// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// LA PAPELERA DE DOCS (decidido por el dueño, 2026-09-28).
//
// Borrar un documento ya lo mandaba a una papelera (`deleted_at`) que nadie podía
// ver: ni restaurar ni vaciar. Aquí se abre, y se ata a la de Drive, porque un
// documento vive como su .odt/.ods/.odp en la unidad de su dueño:
//
//   - desde Docs: tirar, restaurar o purgar llama a Drive ANTES de cambiar el
//     documento. Si Drive falla, no se cambia nada y la persona lo ve; si lo que
//     falla después es Docs, la cola de Drive trae el cambio igualmente.
//   - desde Drive: su cola entrega aquí el estado del fichero (desdeDrive).
//
// Desde Drive nada se borra para siempre: un fichero purgado deja su documento en
// la papelera de Docs, y lo vacía su dueño.
//
// Las dos mitades son idempotentes y se aplican por ESTADO, no por evento, así que
// un cambio que vuelve de rebote (Docs tira → Drive tira → la cola avisa a Docs)
// no hace nada.

// papeleraStore es lo que la papelera necesita de la persistencia.
type papeleraStore interface {
	ListarPapelera(ctx context.Context, ownerSub string, kind domain.Kind, despues *store.CursorPapelera, limit int) ([]domain.Document, error)
	DocumentoEnPapelera(ctx context.Context, id, ownerSub string) (*domain.Document, error)
	RestaurarDocumento(ctx context.Context, id string) error
	PurgarDocumento(ctx context.Context, id string) error
	SincronizarConDrive(ctx context.Context, fileID, ownerSub string, kind domain.Kind, estado string) error
	PapeleraCaducada(ctx context.Context, antes time.Time, despues *store.CursorPapelera, limit int) ([]store.Caducado, error)
	SigueCaducado(ctx context.Context, id string, antes time.Time) (bool, error)
	PurgarCaducado(ctx context.Context, id string, antes time.Time) error
	SinReconciliar(ctx context.Context) ([]domain.Document, error)
}

func papeleraDe(s *store.Store) papeleraStore {
	if s == nil {
		return nil
	}
	return s
}

// ficheroDe es el id del fichero de Drive del que nació el documento, o "".
func ficheroDe(doc *domain.Document) string {
	id, ok := strings.CutPrefix(doc.SourceRef, "drive:")
	if !ok {
		return ""
	}
	return id
}

// errDeDrive es un fallo al hablar con Drive: antes de cambiar nada en Docs.
type errDeDrive struct{ err error }

func (e errDeDrive) Error() string { return e.err.Error() }
func (e errDeDrive) Unwrap() error { return e.err }

// ficheroDelDocumento aplica `accion` al fichero de Drive del documento, si lo
// tiene y la papelera compartida está configurada. Un documento sin fichero solo
// llama al purgarlo, para que Drive diga si su dueño está bajo retención legal.
func (h *Handler) ficheroDelDocumento(ctx context.Context, doc *domain.Document, accion string) error {
	if !h.drive.PapeleraActiva() {
		return nil
	}
	id := ficheroDe(doc)
	if id == "" {
		if accion != drive.AccionPurgar {
			return nil
		}
		id = drive.SinFichero
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	if err := h.drive.AccionFichero(ctx, t.Slug, doc.OwnerSub, string(doc.Kind), id, accion); err != nil {
		return errDeDrive{err}
	}
	return nil
}

// errorDePapelera traduce el fallo. Si fue al hablar con Drive, nada ha cambiado en
// Docs y se dice; el resto de fallos va por `fail`.
func errorDePapelera(w http.ResponseWriter, r *http.Request, err error) {
	var deDrive errDeDrive
	switch {
	case errors.Is(err, drive.ErrNombreOcupado):
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "file_name_taken",
			"Sytue Drive already has a file with this name in its folder. Rename or move it and try again."))
	case errors.Is(err, drive.ErrRetencion):
		httpx.Fail(w, r, httpx.NewError(http.StatusLocked, "legal_hold",
			"This document is under legal hold and cannot be permanently deleted."))
	case errors.As(err, &deDrive):
		log.From(r.Context()).Error("papelera: hablar con Drive", "error", err.Error())
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Sytue Drive is unavailable; nothing was changed."))
	default:
		fail(w, r, err)
	}
}

// kindDeLaPeticion es el `?kind=` opcional de la papelera: Calcus ve y vacía solo
// sus hojas.
func kindDeLaPeticion(w http.ResponseWriter, r *http.Request) (domain.Kind, bool) {
	k := domain.Kind(r.URL.Query().Get("kind"))
	if k != "" && !k.Valid() {
		fail(w, r, domain.ErrInvalidKind)
		return "", false
	}
	return k, true
}

// listarPapelera — GET /api/v1/trash[?kind=].
func (h *Handler) listarPapelera(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	kind, ok := kindDeLaPeticion(w, r)
	if !ok {
		return
	}
	docs, err := h.papelera.ListarPapelera(r.Context(), subj.Sub, kind, nil, 200)
	if err != nil {
		fail(w, r, err)
		return
	}
	if docs == nil {
		docs = []domain.Document{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"documents": docs})
}

// enPapelera carga el documento {id} de la papelera de quien pide.
func (h *Handler) enPapelera(w http.ResponseWriter, r *http.Request) (*domain.Document, bool) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return nil, false
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return nil, false
	}
	doc, err := h.papelera.DocumentoEnPapelera(r.Context(), id, subj.Sub)
	if err != nil {
		fail(w, r, err)
		return nil, false
	}
	return doc, true
}

// restaurarDePapelera — POST /api/v1/trash/{id}/restore.
func (h *Handler) restaurarDePapelera(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.enPapelera(w, r)
	if !ok {
		return
	}
	if err := h.ficheroDelDocumento(r.Context(), doc, drive.AccionRestaurar); err != nil {
		errorDePapelera(w, r, err)
		return
	}
	if err := h.papelera.RestaurarDocumento(r.Context(), doc.ID); err != nil {
		errorDePapelera(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// purgar borra para siempre un documento de la papelera: primero su fichero (si
// ya está en la papelera de Drive; uno vivo no se toca, y una retención legal lo
// impide todo), después la fila y por último sus ficheros de S3.
func (h *Handler) purgar(ctx context.Context, doc *domain.Document) error {
	if err := h.ficheroDelDocumento(ctx, doc, drive.AccionPurgar); err != nil {
		return err
	}
	if err := h.papelera.PurgarDocumento(ctx, doc.ID); err != nil {
		return err
	}
	h.borrarDeS3(ctx, doc.ID)
	return nil
}

// borrarDeS3 quita los snapshots y exports del documento. Sin la fila ya no los
// lee nadie: si falla, quedan huérfanos y no roto.
func (h *Handler) borrarDeS3(ctx context.Context, id string) {
	if h.objects == nil {
		return
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return
	}
	if err := h.objects.DeletePrefix(ctx, t, "docs/"+id+"/"); err != nil {
		log.From(ctx).Warn("papelera: borrar de S3", "doc", id, "error", err.Error())
	}
}

// purgarDePapelera — DELETE /api/v1/trash/{id}. Definitivo.
func (h *Handler) purgarDePapelera(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.enPapelera(w, r)
	if !ok {
		return
	}
	if err := h.purgar(r.Context(), doc); err != nil {
		errorDePapelera(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// Lo que vaciar hace en una petición: el resto, en la siguiente (`next`). Cada
// documento son hasta dos llamadas a Drive, y la petición no puede pasar del
// RequestTimeout del router.
const (
	vaciarPorPeticion = 50
	vaciarPresupuesto = 15 * time.Second
)

// vaciarPapelera — DELETE /api/v1/trash[?kind=&after=]. Definitivo. Lo que no se
// puede borrar (una retención legal, Drive caído) se queda y se cuenta en `failed`,
// y `next` sigue DESPUÉS de ello: vaciar no se atasca en lo que no puede borrar.
func (h *Handler) vaciarPapelera(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	kind, ok := kindDeLaPeticion(w, r)
	if !ok {
		return
	}
	despues, ok := cursorDePapelera(r.URL.Query().Get("after"))
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid cursor."))
		return
	}
	docs, err := h.papelera.ListarPapelera(r.Context(), subj.Sub, kind, despues, vaciarPorPeticion)
	if err != nil {
		fail(w, r, err)
		return
	}
	limite := time.Now().Add(vaciarPresupuesto)
	purgados, fallidos, vistos := 0, 0, 0
	for i := range docs {
		if time.Now().After(limite) {
			break
		}
		if err := h.purgar(r.Context(), &docs[i]); err != nil && !errors.Is(err, domain.ErrNotFound) {
			log.From(r.Context()).Warn("papelera: vaciar", "doc", docs[i].ID, "error", err.Error())
			fallidos++
		} else {
			purgados++
		}
		vistos = i + 1
	}
	// Queda algo si se cortó por tiempo o si la página vino llena.
	siguiente := ""
	if vistos > 0 && (vistos < len(docs) || len(docs) == vaciarPorPeticion) {
		ultimo := docs[vistos-1]
		siguiente = ultimo.DeletedAt.UTC().Format(time.RFC3339Nano) + "|" + ultimo.ID
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"purged": purgados, "failed": fallidos, "next": siguiente})
}

// cursorDePapelera lee el `after` de vaciar: «<deleted_at RFC 3339>|<id>».
func cursorDePapelera(v string) (*store.CursorPapelera, bool) {
	if v == "" {
		return nil, true
	}
	en, id, ok := strings.Cut(v, "|")
	t, err := time.Parse(time.RFC3339Nano, en)
	if !ok || err != nil {
		return nil, false
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, false
	}
	return &store.CursorPapelera{BorradoEn: t, ID: id}, true
}

// kindDeFichero es el tipo de documento que nace en un fichero con ese nombre.
func kindDeFichero(nombre string) (domain.Kind, bool) {
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(nombre)), ".")
	for k, e := range extensionNativa {
		if e == ext {
			return k, true
		}
	}
	return "", false
}

// desdeDrive — POST /internal/drive/files {tenant, files: [{id, state, owner_sub, name}]}.
//
// Lo llama la cola de Drive con el secreto propio de Docs y Drive. Aplica el ESTADO de cada
// fichero al documento de su dueño nacido de él; lo que no tiene documento no hace
// nada. Contesta 204 solo si lo aplicó todo: si no, Drive lo reintenta entero, y
// repetir lo ya aplicado no cambia nada.
func (h *Handler) desdeDrive(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Tenant string `json:"tenant"`
		Files  []struct {
			ID       string `json:"id"`
			State    string `json:"state"`
			OwnerSub string `json:"owner_sub"`
			Name     string `json:"name"`
		} `json:"files"`
	}
	if err := httpx.DecodeJSON(w, r, &c); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !tenancy.ValidSlug(c.Tenant) || len(c.Files) > 1000 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A valid tenant and at most 1000 files are required."))
		return
	}
	ctx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: c.Tenant})
	fallidos := 0
	for _, f := range c.Files {
		kind, ok := kindDeFichero(f.Name)
		if _, err := uuid.Parse(f.ID); err != nil || !ok || f.OwnerSub == "" {
			continue
		}
		switch f.State {
		case store.DriveVivo, store.DrivePapelera, store.DrivePurgado:
		default:
			continue
		}
		// Un fichero que falla no frena a los demás; al final se contesta 500 y Drive
		// repite el lote, que para lo ya aplicado no cambia nada.
		if err := h.papelera.SincronizarConDrive(ctx, f.ID, f.OwnerSub, kind, f.State); err != nil {
			log.From(ctx).Error("papelera: aplicar el estado de Drive", "file_id", logsafe.Text(f.ID), "error", err.Error())
			fallidos++
		}
	}
	if fallidos > 0 {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("Some files could not be applied."))
		return
	}
	httpx.NoContent(w)
}

// RetencionPapelera es lo que un documento se queda en la papelera de Docs antes de
// borrarse solo: 30 días, los mismos que la de Drive (decidido por el dueño,
// 2026-09-28).
const RetencionPapelera = 30 * 24 * time.Hour

// InicioCaducidad es desde cuándo cuenta esa retención. Lo borrado ANTES no se veía
// en ninguna papelera —la vista nació ese día— y no se podía restaurar: su reloj de
// 30 días empieza entonces y no el día que se borró. Así nadie pierde para siempre
// algo que nunca pudo recuperar, y la reconciliación de la papelera heredada tiene
// un mes para contarlo (revisión de seguridad, 2026-09-28).
var InicioCaducidad = time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)

// presupuestoCaducada acota una vuelta de la purga automática: cada documento es una
// llamada a Drive, y la vuelta siguiente sigue desde el principio.
const presupuestoCaducada = 10 * time.Minute

// Caducidad es el recuento de una vuelta de la purga automática.
type Caducidad struct {
	Purgados int
	// Retenidos: una retención legal sobre su dueño.
	Retenidos int
	// Vivos: su fichero sigue vivo en la unidad; el documento se queda.
	Vivos int
	// Errores: Drive no contestó lo que se esperaba; se reintenta la hora siguiente.
	Errores int
}

// PurgarPapeleraCaducada borra para siempre lo que lleva en la papelera más de
// RetencionPapelera (contada desde InicioCaducidad como pronto), de todas las
// organizaciones, página a página y sin atascarse en lo que no puede borrar. Cada
// documento pasa antes por Drive: la retención legal sobre su dueño lo para —tenga
// o no fichero— y, si su fichero sigue en la papelera de Drive, se purga con él. Si
// el fichero sigue vivo en la unidad, el documento se queda. Sin la papelera
// compartida no se sabe si hay retención, y no se borra nada.
func (h *Handler) PurgarPapeleraCaducada(ctx context.Context) (Caducidad, error) {
	var c Caducidad
	if h.papelera == nil || !h.drive.PapeleraActiva() {
		return c, errors.New("papelera: sin Drive no se sabe si hay retención legal; no se purga nada")
	}
	antes := time.Now().Add(-RetencionPapelera)
	if InicioCaducidad.After(antes) {
		// Hasta el 29/10 no caduca nada: el reloj empezó el 29/09 para todo.
		return c, nil
	}
	fin := time.Now().Add(presupuestoCaducada)
	var despues *store.CursorPapelera
	for time.Now().Before(fin) {
		lote, err := h.papelera.PapeleraCaducada(ctx, antes, despues, 500)
		if err != nil {
			return c, err
		}
		for _, cad := range lote {
			h.purgarCaducado(ctx, cad, antes, &c)
		}
		if len(lote) < 500 {
			break
		}
		ultimo := lote[len(lote)-1].Doc
		despues = &store.CursorPapelera{BorradoEn: *ultimo.DeletedAt, ID: ultimo.ID}
	}
	return c, nil
}

func (h *Handler) purgarCaducado(ctx context.Context, cad store.Caducado, antes time.Time, c *Caducidad) {
	tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: cad.Tenant})
	doc := cad.Doc
	// Se vuelve a mirar JUSTO antes de ir a Drive: si lo restauraron desde que se
	// listó, no se purga su fichero.
	if ok, err := h.papelera.SigueCaducado(tctx, doc.ID, antes); err != nil || !ok {
		return
	}
	id := ficheroDe(&doc)
	if id == "" {
		id = drive.SinFichero
	}
	res, err := h.drive.ResultadoFichero(tctx, cad.Tenant, doc.OwnerSub, string(doc.Kind), id, drive.AccionPurgar, time.Time{})
	switch {
	case errors.Is(err, drive.ErrRetencion):
		c.Retenidos++
		return
	case err != nil:
		c.Errores++
		log.From(ctx).Warn("papelera caducada: Drive", "doc", doc.ID, "error", err.Error())
		return
	case res == drive.ResultadoVivo:
		c.Vivos++
		return
	case res != drive.ResultadoHecho && res != drive.ResultadoNoAplica:
		c.Errores++
		log.From(ctx).Warn("papelera caducada: respuesta inesperada de Drive", "doc", doc.ID, "result", res)
		return
	}
	if err := h.papelera.PurgarCaducado(tctx, doc.ID, antes); err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			c.Errores++
			log.From(ctx).Warn("papelera caducada: purgar", "doc", doc.ID, "error", err.Error())
		}
		return
	}
	h.borrarDeS3(tctx, doc.ID)
	c.Purgados++
}

// Reconciliacion es el recuento de `docs reconciliar-papelera` en una organización.
type Reconciliacion struct {
	// Candidatos: documentos en la papelera con un fichero de Drive que no ha seguido.
	Candidatos int
	// APapelera: ficheros que van (en seco) o fueron a la papelera de Drive.
	APapelera int
	// YaEstaban: el fichero ya estaba en la papelera de Drive.
	YaEstaban int
	// Modificados: el fichero cambió después de borrarse el documento (alguien lo
	// sigue usando). No se tocan.
	Modificados int
	// NoAplica: no hay fichero, no es del dueño del documento o no es de su tipo.
	NoAplica int
	Errores  int
}

// ReconciliarPapelera manda a la papelera de Drive los ficheros de los documentos
// que se borraron en Docs ANTES de que las dos papeleras fueran una (decidido por el
// dueño, 2026-09-28), en la organización `org`. Sin `aplicar`, solo cuenta: Drive
// contesta qué haría sin hacerlo. Nunca purga nada, Drive solo mueve un fichero del
// dueño del documento y no toca uno modificado después de borrarse el documento.
func (h *Handler) ReconciliarPapelera(ctx context.Context, org string, aplicar bool) (Reconciliacion, error) {
	var r Reconciliacion
	if !tenancy.ValidSlug(org) {
		return r, fmt.Errorf("organización inválida: %q", org)
	}
	if h.papelera == nil || !h.drive.PapeleraActiva() {
		return r, errors.New("falta la papelera compartida con Drive (SYTUE_DOCS_DRIVE_URL y SYTUE_DOCS_DRIVE_INTERNAL_SECRET)")
	}
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: org})
	docs, err := h.papelera.SinReconciliar(ctx)
	if err != nil {
		return r, err
	}
	accion := drive.AccionComprobar
	if aplicar {
		accion = drive.AccionTirar
	}
	for _, d := range docs {
		r.Candidatos++
		var borrado time.Time
		if d.DeletedAt != nil {
			borrado = *d.DeletedAt
		}
		res, err := h.drive.ResultadoFichero(ctx, org, d.OwnerSub, string(d.Kind), ficheroDe(&d), accion, borrado)
		switch {
		case err != nil:
			r.Errores++
			log.From(ctx).Warn("reconciliar papelera", "doc", d.ID, "error", err.Error())
		case res == drive.ResultadoHecho:
			r.APapelera++
		case res == drive.ResultadoYaEstaba:
			r.YaEstaban++
		case res == drive.ResultadoModificado:
			r.Modificados++
		default:
			r.NoAplica++
		}
	}
	return r, nil
}

// PapeleraCompartida dice si la papelera de Docs está atada a la de Drive (URL y
// secreto): sin ella no hay caducidad automática.
func (h *Handler) PapeleraCompartida() bool { return h.papelera != nil && h.drive.PapeleraActiva() }
