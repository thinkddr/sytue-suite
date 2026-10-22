// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Slides es el cliente de Sytue Diapos. Las presentaciones son documentos de Sytue Docs
// (services/docs, `/api/v1/documents`) con `kind=presentation`; su contenido vive ENTERO
// en un registro LWW, `doc:slides`, cuyo valor es una CADENA con el JSON del modelo
// (`JSON.stringify(SlidesModel)`, ver packages/office/src/editor/types.ts).
//
// Todo lo de este fichero lleva el prefijo Slides: comparte paquete con el cliente de
// Docus (docs.go), que escribe otra línea de trabajo a la vez.
type Slides struct{ c *Client }

// NewSlides crea el cliente sobre la raíz del servicio de Docs (ServiceURL(apex, "slides")
// en nativo; el proxy «/svc/docs» en la web).
func NewSlides(servicio string, s Session, h *http.Client) *Slides {
	return &Slides{c: New(servicio+"/api/v1", s, h)}
}

const (
	slidesKind = "presentation"
	// SlidesContentKey es CONTENT_KEY.presentation de la web.
	SlidesContentKey = "doc:slides"
	// SlidesPagina es el tamaño de página del listado (el servicio admite hasta 200).
	SlidesPagina = 100
)

// SlidesDocument es la ficha de una presentación (domain.Document del servicio).
type SlidesDocument struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	OwnerSub  string    `json:"owner_sub"`
	Seq       int64     `json:"seq"`
	Starred   bool      `json:"starred"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SourceRef string    `json:"source_ref,omitempty"`
}

// SlidesDot es la identidad de una escritura CRDT: (actor, contador).
type SlidesDot struct {
	Actor   string `json:"a"`
	Counter uint64 `json:"c"`
}

// SlidesContent es el contenido de una presentación tal y como está en el servidor, con
// lo que hace falta para escribir encima sin pisar a nadie.
type SlidesContent struct {
	// Modelo es el JSON del modelo (ya sin la envoltura de cadena); "" si nadie ha
	// escrito todavía.
	Modelo string
	// Dot es quién escribió Modelo. Guardar compara con él: si ha cambiado, otro ha
	// escrito entretanto.
	Dot SlidesDot
	// Lamport es el reloj más alto que ha visto el documento; la siguiente escritura va
	// por encima.
	Lamport uint64
	Seq     int64
}

// ErrSlidesConflict: otro ha escrito el documento desde que se cargó. Lo guardado NO se
// ha mandado; quien llama recarga y decide.
var ErrSlidesConflict = errors.New("api: la presentación ha cambiado en el servidor")

// ErrSlidesTooBig: el guardado no cabe en el cuerpo que acepta el servicio (1 MiB,
// platform/pkg/httpx.DefaultMaxBodyBytes). No se ha mandado nada.
var ErrSlidesTooBig = errors.New("api: la presentación no cabe en un guardado")

const slidesMaxCuerpo = 1 << 20

// List devuelve una página de presentaciones, la más reciente primero, y el cursor de la
// siguiente ("" si no hay más).
func (s *Slides) List(ctx context.Context, q, cursor string) ([]SlidesDocument, string, error) {
	query := url.Values{"kind": {slidesKind}, "limit": {strconv.Itoa(SlidesPagina)}}
	if q != "" {
		query.Set("q", q)
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	// GET y no Consulta: el método QUERY mandaría los filtros en el cuerpo, y el
	// listado de Docs los lee de la URL.
	r, err := JSON[struct {
		Documents  []SlidesDocument `json:"documents"`
		NextCursor string           `json:"next_cursor"`
	}](ctx, s.c, Request{Path: "/documents", Query: query})
	return r.Documents, r.NextCursor, err
}

// Create crea una presentación vacía. El servicio le crea además su fichero en la
// unidad de la persona, y puede devolver el título desambiguado.
func (s *Slides) Create(ctx context.Context, titulo string) (SlidesDocument, error) {
	r, err := PostJSON[struct {
		Document SlidesDocument `json:"document"`
	}](ctx, s.c, "/documents", map[string]string{"kind": slidesKind, "title": titulo})
	return r.Document, err
}

// Get devuelve la ficha y el ROL de quien pregunta ("viewer", "commenter", "editor",
// "owner"): solo los dos últimos pueden escribir.
func (s *Slides) Get(ctx context.Context, id string) (SlidesDocument, string, error) {
	r, err := JSON[struct {
		Document SlidesDocument `json:"document"`
		Role     string         `json:"role"`
	}](ctx, s.c, Request{Path: slidesRuta(id, "")})
	return r.Document, r.Role, err
}

// SlidesPuedeEscribir es Role.CanWrite del servicio.
func SlidesPuedeEscribir(rol string) bool { return rol == "editor" || rol == "owner" }

// Rename cambia el título.
func (s *Slides) Rename(ctx context.Context, id, titulo string) error {
	cuerpo, err := json.Marshal(map[string]string{"title": titulo})
	if err != nil {
		return err
	}
	return s.sinCuerpo(ctx, Request{
		Method: http.MethodPatch, Path: slidesRuta(id, ""), Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Delete elimina la presentación.
func (s *Slides) Delete(ctx context.Context, id string) error {
	return s.sinCuerpo(ctx, Request{Method: http.MethodDelete, Path: slidesRuta(id, "")})
}

// Content carga el contenido: el registro `doc:slides` del estado consolidado.
func (s *Slides) Content(ctx context.Context, id string) (SlidesContent, error) {
	r, err := JSON[struct {
		Seq   int64 `json:"seq"`
		State struct {
			Lamport   uint64 `json:"lamport"`
			Registers map[string]struct {
				Value   json.RawMessage `json:"value"`
				Dot     SlidesDot       `json:"dot"`
				Lamport uint64          `json:"lamport"`
			} `json:"registers"`
		} `json:"state"`
	}](ctx, s.c, Request{Path: slidesRuta(id, "/state")})
	if err != nil {
		return SlidesContent{}, err
	}
	out := SlidesContent{Lamport: r.State.Lamport, Seq: r.Seq}
	reg, ok := r.State.Registers[SlidesContentKey]
	if !ok {
		return out, nil
	}
	out.Dot = reg.Dot
	// El valor es una cadena JSON con el modelo dentro. Si no lo es, no es una
	// presentación que sepamos leer: error, no «vacía» (guardar encima la borraría).
	if err := json.Unmarshal(reg.Value, &out.Modelo); err != nil {
		return SlidesContent{}, errors.New("api: el registro " + SlidesContentKey + " no es una cadena")
	}
	return out, nil
}

// Save escribe el modelo por el mismo camino que la web: un `set` LWW del registro
// entero (`POST /documents/{id}/ops`). `base` es lo que se cargó; `actor` identifica esta
// sesión de edición y `contador` crece con cada guardado.
//
// Antes de escribir vuelve a mirar el servidor: si el registro ya no es el que se cargó,
// devuelve ErrSlidesConflict con lo que hay ahora y NO escribe. La web se entera de los
// cambios ajenos por WebSocket; aquí no hay, y un LWW a ciegas pisaría el trabajo de otro.
//
// La escritura lleva `base_seq` (la seq que se cargó): si otro escribe entre esa lectura y
// el POST, el servicio contesta 409 y el resultado es el mismo ErrSlidesConflict, sin ventana.
func (s *Slides) Save(ctx context.Context, id string, base SlidesContent, actor string, contador uint64, modelo string) (SlidesContent, error) {
	// Solo lo que cambió, como parche (docs_parche.go): el servicio lo aplica sobre lo último
	// que tiene y no hay conflicto con lo que otra persona haya tocado en otras cajas. Si no se
	// deja decir así, o el servicio no entiende parches, abajo va el modelo entero.
	if opID, err := nuevoOpID(); err == nil {
		p, err := mandarParche(ctx, s.c, id, SlidesContentKey, base.Modelo, modelo, opID, base.Seq)
		switch {
		case err == nil && p.SinEfecto:
			return SlidesContent{Modelo: modelo, Dot: base.Dot, Lamport: base.Lamport, Seq: p.Seq}, nil
		case err == nil:
			return SlidesContent{Modelo: modelo, Dot: SlidesDot(p.Dot), Lamport: p.Lamport, Seq: p.Seq}, nil
		case !sinParches(err):
			return SlidesContent{}, err
		}
	}
	ahora, err := s.Content(ctx, id)
	if err != nil {
		return SlidesContent{}, err
	}
	if ahora.Dot != base.Dot {
		return ahora, ErrSlidesConflict
	}
	dot := SlidesDot{Actor: actor, Counter: contador}
	lamport := max(ahora.Lamport, base.Lamport) + 1
	type operacion struct {
		Kind    string    `json:"kind"`
		Dot     SlidesDot `json:"dot"`
		Key     string    `json:"key"`
		Value   string    `json:"value"` // la cadena dentro de la cadena, como JSON.stringify
		Lamport uint64    `json:"lamport"`
	}
	type lote struct {
		Ops     []operacion `json:"ops"`
		BaseSeq int64       `json:"base_seq,omitempty"`
	}
	// Sin el escapado de HTML de json.Marshal: cada «<» del texto de una caja serían seis
	// bytes, y el cuerpo que acepta el servicio tiene tope (la web corta en 900 000).
	var cuerpo bytes.Buffer
	enc := json.NewEncoder(&cuerpo)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(lote{
		Ops:     []operacion{{Kind: "set", Dot: dot, Key: SlidesContentKey, Value: modelo, Lamport: lamport}},
		BaseSeq: base.Seq,
	}); err != nil {
		return SlidesContent{}, err
	}
	if cuerpo.Len() > slidesMaxCuerpo {
		return SlidesContent{}, ErrSlidesTooBig
	}
	r, err := JSON[struct {
		Seq int64 `json:"seq"`
	}](ctx, s.c, Request{
		Method: http.MethodPost, Path: slidesRuta(id, "/ops"), Body: cuerpo.Bytes(),
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	var http409 *Error
	if errors.As(err, &http409) && http409.Status == http.StatusConflict {
		// Otro escribió entre la lectura y el POST: se devuelve lo que hay ahora.
		if ahora, errLeer := s.Content(ctx, id); errLeer == nil {
			return ahora, ErrSlidesConflict
		}
		return SlidesContent{}, ErrSlidesConflict
	}
	if err != nil {
		return SlidesContent{}, err
	}
	return SlidesContent{Modelo: modelo, Dot: dot, Lamport: lamport, Seq: r.Seq}, nil
}

func (s *Slides) sinCuerpo(ctx context.Context, r Request) error {
	resp, err := s.c.Do(ctx, r)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func slidesRuta(id, resto string) string { return "/documents/" + url.PathEscape(id) + resto }
