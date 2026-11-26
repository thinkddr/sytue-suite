// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/office/plantillas"
)

// Lo que el servicio de Docs hace por HTTP y aquí hace la biblioteca: papelera, «Hacer
// una copia», plantillas, temas y bloques (PLAN-EDICION-LOCAL O-09). Son las interfaces
// opcionales de `api` (docs_opcionales.go): la fachada les delega en cuanto las ve.

var (
	_ api.DocsTrashData     = (*Biblioteca)(nil)
	_ api.DocsCopyData      = (*Biblioteca)(nil)
	_ api.DocsTemplatesData = (*Biblioteca)(nil)
	_ api.DocsThemesData    = (*Biblioteca)(nil)
	_ api.DocsBlocksData    = (*Biblioteca)(nil)
)

// ErrSinOrganizacion: la marca es de una organización, y en local no hay ninguna.
var ErrSinOrganizacion = errors.New("biblioteca: la marca de la organización necesita un servicio")

// maxPapelera es lo que lista la papelera, como el servicio.
const maxPapelera = 200

// ----- Papelera -----

// TrashedDocuments lista lo que hay en la papelera (de ese tipo, si lo hay), lo último
// tirado primero.
func (b *Biblioteca) TrashedDocuments(_ context.Context, kind api.DocKind) ([]api.Document, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []api.Document{}
	for _, r := range b.docs {
		if d := r.Documento; d.DeletedAt != nil && (kind == "" || d.Kind == kind) {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, c api.Document) int {
		if x := c.DeletedAt.Compare(*a.DeletedAt); x != 0 {
			return x
		}
		return cmp.Compare(a.ID, c.ID)
	})
	if len(out) > maxPapelera {
		out = out[:maxPapelera]
	}
	return out, nil
}

// enPapelera es el registro de un documento que está en la papelera. Con b.mu.
func (b *Biblioteca) enPapelera(id string) (*registro, error) {
	r, ok := b.docs[id]
	if !ok || r.Documento.DeletedAt == nil {
		return nil, noEncontrado(id)
	}
	return r, nil
}

// RestoreDocument saca el documento de la papelera. Si entretanto se abrió otro documento
// desde el mismo fichero, es un 409 `already_restored`, como en el servicio: dos
// documentos vivos no pueden estar anclados a un fichero.
func (b *Biblioteca) RestoreDocument(_ context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.enPapelera(id)
	if err != nil {
		return err
	}
	if f := r.Documento.SourceRef; f != "" {
		for _, otro := range b.docs {
			if otro != r && otro.Documento.DeletedAt == nil && otro.Documento.SourceRef == f {
				return &api.Error{Status: http.StatusConflict, URL: "/trash/" + id + "/restore", Body: `{"error":{"code":"already_restored"}}`}
			}
		}
	}
	r.Documento.DeletedAt = nil
	r.Documento.UpdatedAt = time.Now().UTC()
	return b.guardar(r)
}

// PurgeDocument borra para siempre un documento de la papelera: su fichero y sus versiones.
func (b *Biblioteca) PurgeDocument(_ context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := b.enPapelera(id); err != nil {
		return err
	}
	return b.purgar(id)
}

// purgar quita el documento id del disco y del índice. Con b.mu.
func (b *Biblioteca) purgar(id string) error {
	if err := os.Remove(b.ruta(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = os.RemoveAll(b.rutaVersiones(id))
	delete(b.docs, id)
	delete(b.ops, id)
	return nil
}

// EmptyTrash vacía la papelera (de ese tipo, si lo hay) de una vez: en local no hay
// páginas, así que nunca queda «siguiente».
func (b *Biblioteca) EmptyTrash(_ context.Context, kind api.DocKind, _ string) (int, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fallidos := 0
	for id, r := range b.docs {
		if d := r.Documento; d.DeletedAt != nil && (kind == "" || d.Kind == kind) {
			if b.purgar(id) != nil {
				fallidos++
			}
		}
	}
	return fallidos, "", nil
}

// ----- Copia -----

// CopyDocument es «Hacer una copia»: un documento nuevo con el contenido de ahora del
// original, reescrito a nombre de un actor nuevo (como `semillaDeCopia` del servicio).
// Comentarios, versiones y el fichero de origen no viajan: son del original.
func (b *Biblioteca) CopyDocument(_ context.Context, original api.Document, titulo string) (api.Document, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(original.ID)
	if err != nil {
		return api.Document{}, err
	}
	st, err := r.cargarEstado()
	if err != nil {
		return api.Document{}, err
	}
	tok, err := model.NewToken()
	if err != nil {
		return api.Document{}, err
	}
	ops := model.NewState().Diff(st, "copia:"+tok[:16], 0, 0)
	return b.nuevoCon(r.Documento.Kind, cmp.Or(strings.TrimSpace(titulo), r.Documento.Title), "", ops)
}

// nuevoCon crea un documento con esas operaciones ya aplicadas. Con b.mu.
func (b *Biblioteca) nuevoCon(kind api.DocKind, titulo, fuente string, ops []model.Op) (api.Document, error) {
	if kind != b.tipo {
		return api.Document{}, model.ErrInvalidKind
	}
	ahora := time.Now().UTC()
	r := &registro{Documento: api.Document{
		ID: uuid.NewString(), Kind: kind, Title: titulo, OwnerSub: b.persona.ID,
		CreatedAt: ahora, UpdatedAt: ahora, SourceRef: fuente,
	}, estado: model.NewState()}
	seq := int64(0)
	for _, op := range ops {
		if _, err := r.estado.Apply(op); err != nil {
			return api.Document{}, err
		}
		seq++
	}
	r.estado.Seq = seq
	r.Documento.Seq = seq
	if err := b.guardar(r); err != nil {
		return api.Document{}, err
	}
	b.docs[r.Documento.ID] = r
	return r.Documento, nil
}

// ----- Plantillas -----

// DocumentTemplates es la galería de Sytue en el idioma pedido. En local no hay
// plantillas de organización.
func (b *Biblioteca) DocumentTemplates(_ context.Context, locale string) (api.DocTemplates, error) {
	lista, err := plantillas.Listar(locale)
	if err != nil {
		return api.DocTemplates{}, err
	}
	out := api.DocTemplates{Templates: make([]api.DocTemplate, 0, len(lista)), OrgTemplates: []api.DocTemplate{}}
	for _, p := range lista {
		out.Templates = append(out.Templates, api.DocTemplate{Key: p.Key, Kind: api.DocKind(p.Kind), Name: p.Name, Description: p.Description})
	}
	return out, nil
}

// CreateDocumentFromTemplate da de alta un documento con el contenido de la plantilla y,
// si se pide, el tema aplicado (`semillaDeCreacion` del servicio).
func (b *Biblioteca) CreateDocumentFromTemplate(_ context.Context, n api.DocNuevo) (api.Document, error) {
	clave := strings.TrimSpace(n.Template)
	if strings.HasPrefix(clave, "org:") {
		return api.Document{}, ErrSinOrganizacion
	}
	info, contenido, err := plantillas.Generar(clave, n.Locale)
	if errors.Is(err, plantillas.ErrNoExiste) {
		return api.Document{}, &api.Error{Status: http.StatusBadRequest, URL: "/documents", Body: "Unknown template."}
	}
	if err != nil {
		return api.Document{}, err
	}
	op, err := model.ContentOp(info.Kind, contenido)
	if err != nil {
		return api.Document{}, err
	}
	ops := []model.Op{op}
	if th := strings.TrimSpace(n.Theme); th != "" {
		tema, err := b.tema(th)
		if err != nil {
			return api.Document{}, err
		}
		st := model.NewState()
		if _, err := st.Apply(op); err != nil {
			return api.Document{}, err
		}
		if opTema, cambia, err := model.OpTema(info.Kind, st, tema); err != nil {
			return api.Document{}, err
		} else if cambia {
			ops = append(ops, opTema)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nuevoCon(api.DocKind(info.Kind), cmp.Or(strings.TrimSpace(n.Title), info.Name), "", ops)
}

// ----- Temas -----

// tema resuelve un id del catálogo común. La marca no existe en local.
func (b *Biblioteca) tema(id string) (model.TemaComun, error) {
	if id == api.IDTemaMarca {
		return model.TemaComun{}, &api.Error{Status: http.StatusNotFound, URL: "/themes"}
	}
	t, ok := model.TemaPorID(id, nil)
	if !ok {
		return model.TemaComun{}, &api.Error{Status: http.StatusBadRequest, URL: "/themes", Body: "Unknown theme."}
	}
	return t, nil
}

// DocumentThemes es el catálogo común; sin marca de organización.
func (b *Biblioteca) DocumentThemes(context.Context) (api.DocThemes, error) {
	out := api.DocThemes{Versions: []api.DocBrand{}}
	for _, t := range model.CatalogoTemas() {
		out.Themes = append(out.Themes, api.DocTheme{ID: t.ID})
	}
	return out, nil
}

// SaveDocumentBrand: sin organización no hay marca que guardar.
func (b *Biblioteca) SaveDocumentBrand(context.Context, api.DocBrand) (int, error) {
	return 0, ErrSinOrganizacion
}

// PreviewDocumentTheme dice qué cambiaría aplicar el tema, sin escribir nada.
func (b *Biblioteca) PreviewDocumentTheme(_ context.Context, id, tema string, _ int) (api.DocThemePreview, error) {
	t, err := b.tema(tema)
	if err != nil {
		return api.DocThemePreview{}, err
	}
	b.mu.Lock()
	r, err := b.vivo(id)
	var contenido string
	kind := b.tipo
	if err == nil {
		var st *model.State
		if st, err = r.cargarEstado(); err == nil {
			kind = r.Documento.Kind
			contenido = st.Contenido(model.Kind(kind))
		}
	}
	b.mu.Unlock()
	if err != nil {
		return api.DocThemePreview{}, err
	}
	cambios, err := model.PrevisualizarTema(model.Kind(kind), contenido, t)
	if err != nil {
		return api.DocThemePreview{}, err
	}
	out := api.DocThemePreview{Changed: len(cambios) > 0, Changes: make([]api.DocThemeChange, 0, len(cambios))}
	for _, c := range cambios {
		out.Changes = append(out.Changes, api.DocThemeChange{Scope: c.Scope, Sheet: c.Sheet, Count: c.Count, From: c.From, To: c.To})
	}
	return out, nil
}

// ApplyDocumentTheme aplica el tema sin tocar el contenido y, antes, guarda una versión
// «Antes del tema …» (restaurarla lo deshace), como el servicio.
func (b *Biblioteca) ApplyDocumentTheme(ctx context.Context, id, tema string, _ int) (bool, error) {
	t, err := b.tema(tema)
	if err != nil {
		return false, err
	}
	b.mu.Lock()
	r, err := b.vivo(id)
	if err != nil {
		b.mu.Unlock()
		return false, err
	}
	st, err := r.cargarEstado()
	if err != nil {
		b.mu.Unlock()
		return false, err
	}
	op, cambia, err := model.OpTema(model.Kind(r.Documento.Kind), st, t)
	b.mu.Unlock()
	if err != nil || !cambia {
		return false, err
	}
	if _, err := b.CreateDocumentVersion(ctx, id, "Antes del tema "+t.ID); err != nil {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := b.aplicar(id, []model.Op{op}, op.Dot.Actor, 0, false); err != nil {
		return false, err
	}
	return true, nil
}

// ----- Bloques -----

// DocumentBlocks son los bloques de Sytue de ese tipo; en local no hay de organización.
func (b *Biblioteca) DocumentBlocks(_ context.Context, kind api.DocKind, locale string) (api.DocBlocks, error) {
	if !model.Kind(kind).Valid() {
		return api.DocBlocks{}, fmt.Errorf("%w: %q", model.ErrInvalidKind, kind)
	}
	lista, err := plantillas.ListarBloques(model.Kind(kind), locale)
	if err != nil {
		return api.DocBlocks{}, err
	}
	out := api.DocBlocks{Blocks: make([]api.DocBlock, 0, len(lista)), OrgBlocks: []api.DocBlock{}}
	for _, p := range lista {
		out.Blocks = append(out.Blocks, api.DocBlock{Key: p.Key, Name: p.Name})
	}
	return out, nil
}

// DocumentBlock es el contenido de un bloque, neutro (sin marca: no hay organización).
func (b *Biblioteca) DocumentBlock(_ context.Context, clave, locale string) (string, error) {
	_, contenido, err := plantillas.GenerarBloque(clave, locale, nil)
	if errors.Is(err, plantillas.ErrNoExiste) {
		return "", &api.Error{Status: http.StatusNotFound, URL: "/blocks/" + clave}
	}
	return contenido, err
}
