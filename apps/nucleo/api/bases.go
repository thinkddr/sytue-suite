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

// Bases es el cliente de Sytue Bases (services/bases, ADR 0068): bases, tablas, campos
// tipados, registros con enlaces, vistas guardadas, permisos y exportación. Es el
// espejo de `packages/api-clients/src/bases/index.ts`, ruta a ruta; el cable va en
// snake_case.
//
// El permiso lo decide SIEMPRE el servicio: el rol que devuelve sirve para pintar la
// interfaz, no para autorizar.
type Bases struct{ c *Client }

// NewBases crea el cliente sobre la raíz del servicio (ServiceURL(apex, "bases")).
func NewBases(servicio string, s Session, h *http.Client) *Bases {
	return &Bases{c: New(strings.TrimRight(servicio, "/")+"/api/v1", s, h)}
}

// Los roles de una base o de una tabla. Vacío en `BasesBase.Role` es «solo accede a
// alguna de sus tablas».
const (
	BasesOwner     = "owner"
	BasesEditor    = "editor"
	BasesCommenter = "commenter"
	BasesViewer    = "viewer"
)

// Los tipos de campo.
const (
	BasesKindText        = "text"
	BasesKindNumber      = "number"
	BasesKindDate        = "date"
	BasesKindCheckbox    = "checkbox"
	BasesKindSelect      = "select"
	BasesKindMultiselect = "multiselect"
	BasesKindLink        = "link"
	BasesKindFormula     = "formula"
	BasesKindAttachment  = "attachment"
)

// Los tipos de vista.
const (
	BasesGrid   = "grid"
	BasesKanban = "kanban"
	BasesForm   = "form"
)

// BasesBase es una base.
type BasesBase struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerSub    string `json:"owner_sub"`
	Role        string `json:"role"`
	// HostOrg es la organización que aloja una base compartida desde fuera; vacía en
	// las propias.
	HostOrg   string    `json:"host_org,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BasesField es un campo de una tabla.
type BasesField struct {
	ID          string   `json:"id"`
	TableID     string   `json:"table_id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Choices     []string `json:"choices,omitempty"`
	LinkTableID string   `json:"link_table_id,omitempty"`
	// Formula y Result solo en un campo de fórmula: la expresión (referencias por id) y
	// el tipo de su resultado (`number` o `text`).
	Formula  string `json:"formula,omitempty"`
	Result   string `json:"result,omitempty"`
	Position int    `json:"position"`
}

// BasesTable es una tabla con sus campos.
type BasesTable struct {
	ID       string       `json:"id"`
	BaseID   string       `json:"base_id"`
	Name     string       `json:"name"`
	Position int          `json:"position"`
	Role     string       `json:"role,omitempty"`
	Fields   []BasesField `json:"fields"`
}

// BasesAttachment es la ficha de un fichero adjunto; los bytes se piden aparte.
type BasesAttachment struct {
	ID        string    `json:"id"`
	RecordID  string    `json:"record_id"`
	FieldID   string    `json:"field_id"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	HasThumb  bool      `json:"has_thumb"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// BasesRecord es un registro. Data lleva lo que viaja por celda: string, float64, bool,
// []any (de cadenas) o nil.
type BasesRecord struct {
	ID          string                       `json:"id"`
	TableID     string                       `json:"table_id"`
	Data        map[string]any               `json:"data"`
	Links       map[string][]string          `json:"links"`
	Attachments map[string][]BasesAttachment `json:"attachments,omitempty"`
	Rev         int                          `json:"rev"`
	CreatedBy   string                       `json:"created_by"`
	UpdatedBy   string                       `json:"updated_by"`
	CreatedAt   time.Time                    `json:"created_at"`
	UpdatedAt   time.Time                    `json:"updated_at"`
}

// BasesRecordPage es una página de registros. NextCursor vacío = no hay más.
type BasesRecordPage struct {
	Fields        []BasesField  `json:"fields"`
	Records       []BasesRecord `json:"records"`
	Role          string        `json:"role"`
	NextCursor    string        `json:"next_cursor,omitempty"`
	Recalculating bool          `json:"recalculating,omitempty"`
}

// BasesFilter es una condición de una vista.
type BasesFilter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

// BasesSort es un criterio de orden de una vista (`asc` o `desc`).
type BasesSort struct {
	Field string `json:"field"`
	Dir   string `json:"dir"`
}

// BasesViewConfig es lo que una vista guarda.
type BasesViewConfig struct {
	Filters []BasesFilter `json:"filters,omitempty"`
	Sort    []BasesSort   `json:"sort,omitempty"`
	GroupBy string        `json:"group_by,omitempty"`
	Fields  []string      `json:"fields,omitempty"`
}

// BasesView es una vista guardada de una tabla.
type BasesView struct {
	ID        string          `json:"id"`
	TableID   string          `json:"table_id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Config    BasesViewConfig `json:"config"`
	CreatedBy string          `json:"created_by"`
}

// BasesPermission es un acceso concedido. PrincipalKind: `user`, `group`, `tenant` o
// `external` (alguien de otra organización).
type BasesPermission struct {
	ID             string    `json:"id"`
	TableID        string    `json:"table_id,omitempty"`
	PrincipalKind  string    `json:"principal_kind"`
	PrincipalID    string    `json:"principal_id"`
	PrincipalLabel string    `json:"principal_label,omitempty"`
	Role           string    `json:"role"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

// BasesGrant es lo que se manda al conceder: a una persona por `Email`, a un grupo por
// `PrincipalID`, a alguien de fuera por `Query` (su @usuario o su dirección).
type BasesGrant struct {
	PrincipalKind string `json:"principal_kind"`
	Role          string `json:"role"`
	PrincipalID   string `json:"principal_id,omitempty"`
	Email         string `json:"email,omitempty"`
	Query         string `json:"query,omitempty"`
	TableID       string `json:"table_id,omitempty"`
}

// BasesFormLink es un enlace público a un formulario. Token solo viene al crearlo.
type BasesFormLink struct {
	ID           string     `json:"id"`
	ViewID       string     `json:"view_id"`
	MaxResponses int        `json:"max_responses"`
	Responses    int        `json:"responses"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	Token        string     `json:"token,omitempty"`
}

// BasesNewField es un campo por crear.
type BasesNewField struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Choices     []string `json:"choices,omitempty"`
	LinkTableID string   `json:"link_table_id,omitempty"`
	Formula     string   `json:"formula,omitempty"`
	Result      string   `json:"result,omitempty"`
}

// BasesCell es una operación de celda: el valor (o los enlaces) y lo que se veía.
type BasesCell struct {
	Value    any      `json:"value,omitempty"`
	Links    []string `json:"links,omitempty"`
	Expected any      `json:"expected,omitempty"`
}

// BasesCellResult es lo que contesta una operación de celda. Conflict: se ha pisado un
// valor que quien escribe no había visto (gana el último).
type BasesCellResult struct {
	Record   BasesRecord `json:"record"`
	Conflict bool        `json:"conflict"`
	Replayed bool        `json:"replayed"`
}

// BasesRealtimeVersion es la versión del contrato de tiempo real que habla este cliente.
const BasesRealtimeVersion = 1

func (b *Bases) enviar(ctx context.Context, metodo, ruta string, cuerpo any) (*http.Response, error) {
	r := Request{Method: metodo, Path: ruta}
	if cuerpo != nil {
		datos, err := json.Marshal(cuerpo)
		if err != nil {
			return nil, err
		}
		r.Body, r.Header = datos, http.Header{"Content-Type": {"application/json"}}
	}
	return b.c.Do(ctx, r)
}

func (b *Bases) sinCuerpo(ctx context.Context, metodo, ruta string, cuerpo any) error {
	resp, err := b.enviar(ctx, metodo, ruta, cuerpo)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func basesJSON[T any](ctx context.Context, b *Bases, metodo, ruta string, cuerpo any) (T, error) {
	var out T
	resp, err := b.enviar(ctx, metodo, ruta, cuerpo)
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out, err
}

type nombreYDescripcion struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListBases son las bases a las que se tiene acceso.
func (b *Bases) ListBases(ctx context.Context) ([]BasesBase, error) {
	return basesJSON[[]BasesBase](ctx, b, http.MethodGet, "/bases", nil)
}

// CreateBase crea una base, de la que quien llama es propietario.
func (b *Bases) CreateBase(ctx context.Context, nombre string) (BasesBase, error) {
	return basesJSON[BasesBase](ctx, b, http.MethodPost, "/bases", nombreYDescripcion{Name: nombre})
}

// GetBase lee una base. Quien solo accede a alguna tabla recibe un 404.
func (b *Bases) GetBase(ctx context.Context, id string) (BasesBase, error) {
	return basesJSON[BasesBase](ctx, b, http.MethodGet, "/bases/"+url.PathEscape(id), nil)
}

// UpdateBase cambia el nombre y la descripción (solo el propietario).
func (b *Bases) UpdateBase(ctx context.Context, id, nombre, descripcion string) error {
	return b.sinCuerpo(ctx, http.MethodPatch, "/bases/"+url.PathEscape(id), nombreYDescripcion{nombre, descripcion})
}

// DeleteBase borra la base con sus tablas y registros (solo el propietario).
func (b *Bases) DeleteBase(ctx context.Context, id string) error {
	return b.sinCuerpo(ctx, http.MethodDelete, "/bases/"+url.PathEscape(id), nil)
}

// ListTables son las tablas de una base que quien llama puede leer.
func (b *Bases) ListTables(ctx context.Context, base string) ([]BasesTable, error) {
	return basesJSON[[]BasesTable](ctx, b, http.MethodGet, "/bases/"+url.PathEscape(base)+"/tables", nil)
}

// CreateTable crea una tabla (solo el propietario).
func (b *Bases) CreateTable(ctx context.Context, base, nombre string) (BasesTable, error) {
	return basesJSON[BasesTable](ctx, b, http.MethodPost, "/bases/"+url.PathEscape(base)+"/tables", map[string]string{"name": nombre})
}

// RenameTable renombra una tabla.
func (b *Bases) RenameTable(ctx context.Context, tabla, nombre string) error {
	return b.sinCuerpo(ctx, http.MethodPatch, "/tables/"+url.PathEscape(tabla), map[string]string{"name": nombre})
}

// DeleteTable borra una tabla y sus registros.
func (b *Bases) DeleteTable(ctx context.Context, tabla string) error {
	return b.sinCuerpo(ctx, http.MethodDelete, "/tables/"+url.PathEscape(tabla), nil)
}

// CreateField añade un campo a una tabla.
func (b *Bases) CreateField(ctx context.Context, tabla string, campo BasesNewField) (BasesField, error) {
	return basesJSON[BasesField](ctx, b, http.MethodPost, "/tables/"+url.PathEscape(tabla)+"/fields", campo)
}

// RenameField renombra un campo.
func (b *Bases) RenameField(ctx context.Context, campo, nombre string) error {
	return b.sinCuerpo(ctx, http.MethodPatch, "/fields/"+url.PathEscape(campo), map[string]string{"name": nombre})
}

// DeleteField borra un campo y sus valores.
func (b *Bases) DeleteField(ctx context.Context, campo string) error {
	return b.sinCuerpo(ctx, http.MethodDelete, "/fields/"+url.PathEscape(campo), nil)
}

// ListRecords pide UNA página de registros (nunca la tabla entera): `cursor` es el
// `NextCursor` de la anterior, `vista` una vista guardada (su filtro y su orden los
// ejecuta el servicio) y `limite` el tamaño de página. Vacíos o cero, no se mandan.
func (b *Bases) ListRecords(ctx context.Context, tabla, vista, cursor string, limite int) (BasesRecordPage, error) {
	q := url.Values{}
	if vista != "" {
		q.Set("view", vista)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limite > 0 {
		q.Set("limit", strconv.Itoa(limite))
	}
	return JSON[BasesRecordPage](ctx, b.c, Request{Path: "/tables/" + url.PathEscape(tabla) + "/records", Query: q})
}

// CreateRecord crea un registro.
func (b *Bases) CreateRecord(ctx context.Context, tabla string, datos map[string]any, enlaces map[string][]string) (BasesRecord, error) {
	if datos == nil {
		datos = map[string]any{}
	}
	cuerpo := map[string]any{"data": datos}
	if enlaces != nil {
		cuerpo["links"] = enlaces
	}
	return basesJSON[BasesRecord](ctx, b, http.MethodPost, "/tables/"+url.PathEscape(tabla)+"/records", cuerpo)
}

// GetRecord lee un registro.
func (b *Bases) GetRecord(ctx context.Context, registro string) (BasesRecord, error) {
	return basesJSON[BasesRecord](ctx, b, http.MethodGet, "/records/"+url.PathEscape(registro), nil)
}

// WriteCell escribe UN campo sin pedir la versión del registro: dos personas en celdas
// distintas no se estorban; en la misma gana quien escribe después y `Conflict` lo
// dice. `op` (UUID) hace la operación idempotente.
func (b *Bases) WriteCell(ctx context.Context, registro, campo string, celda BasesCell, op string) (BasesCellResult, error) {
	// `expected` viaja SIEMPRE, también cuando es null: es lo que se veía en la celda.
	cuerpo := map[string]any{"expected": celda.Expected}
	if celda.Links != nil {
		cuerpo["links"] = celda.Links
	} else {
		cuerpo["value"] = celda.Value
	}
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return BasesCellResult{}, err
	}
	return JSON[BasesCellResult](ctx, b.c, Request{
		Method: http.MethodPut,
		Path:   "/records/" + url.PathEscape(registro) + "/cells/" + url.PathEscape(campo),
		Query:  url.Values{"v": {strconv.Itoa(BasesRealtimeVersion)}},
		Body:   datos,
		Header: http.Header{"Content-Type": {"application/json"}, "Idempotency-Key": {op}},
	})
}

// DeleteRecord borra un registro. `forzar` quita además los enlaces que apuntan a él
// (solo el propietario); sin él, un registro enlazado contesta 409 `referenced`.
func (b *Bases) DeleteRecord(ctx context.Context, registro string, forzar bool) error {
	ruta := "/records/" + url.PathEscape(registro)
	if forzar {
		ruta += "?force=true"
	}
	return b.sinCuerpo(ctx, http.MethodDelete, ruta, nil)
}

// ListViews son las vistas guardadas de una tabla.
func (b *Bases) ListViews(ctx context.Context, tabla string) ([]BasesView, error) {
	return basesJSON[[]BasesView](ctx, b, http.MethodGet, "/tables/"+url.PathEscape(tabla)+"/views", nil)
}

// CreateView guarda una vista.
func (b *Bases) CreateView(ctx context.Context, tabla, nombre, tipo string, cfg BasesViewConfig) (BasesView, error) {
	return basesJSON[BasesView](ctx, b, http.MethodPost, "/tables/"+url.PathEscape(tabla)+"/views",
		map[string]any{"name": nombre, "kind": tipo, "config": cfg})
}

// DeleteView borra una vista.
func (b *Bases) DeleteView(ctx context.Context, vista string) error {
	return b.sinCuerpo(ctx, http.MethodDelete, "/views/"+url.PathEscape(vista), nil)
}

// ListFormLinks son los enlaces públicos de una vista de formulario.
func (b *Bases) ListFormLinks(ctx context.Context, vista string) ([]BasesFormLink, error) {
	return basesJSON[[]BasesFormLink](ctx, b, http.MethodGet, "/views/"+url.PathEscape(vista)+"/form-links", nil)
}

// CreateFormLink crea un enlace público (solo el propietario). Su `Token` no se puede
// volver a leer. `maximo` cero deja el del servicio.
func (b *Bases) CreateFormLink(ctx context.Context, vista string, maximo int) (BasesFormLink, error) {
	cuerpo := map[string]int{}
	if maximo > 0 {
		cuerpo["max_responses"] = maximo
	}
	return basesJSON[BasesFormLink](ctx, b, http.MethodPost, "/views/"+url.PathEscape(vista)+"/form-links", cuerpo)
}

// RevokeFormLink cierra un enlace público al instante.
func (b *Bases) RevokeFormLink(ctx context.Context, enlace string) error {
	return b.sinCuerpo(ctx, http.MethodDelete, "/form-links/"+url.PathEscape(enlace), nil)
}

// ListPermissions es quién tiene acceso a la base (solo el propietario).
func (b *Bases) ListPermissions(ctx context.Context, base string) ([]BasesPermission, error) {
	return basesJSON[[]BasesPermission](ctx, b, http.MethodGet, "/bases/"+url.PathEscape(base)+"/permissions", nil)
}

// GrantPermission concede acceso. 422 `not_a_member` si la dirección no es de la
// organización; 422 `is_member` si el «externo» resulta ser de la propia.
func (b *Bases) GrantPermission(ctx context.Context, base string, g BasesGrant) (BasesPermission, error) {
	return basesJSON[BasesPermission](ctx, b, http.MethodPost, "/bases/"+url.PathEscape(base)+"/permissions", g)
}

// RevokePermission retira un acceso.
func (b *Bases) RevokePermission(ctx context.Context, base, permiso string) error {
	return b.sinCuerpo(ctx, http.MethodDelete, "/bases/"+url.PathEscape(base)+"/permissions/"+url.PathEscape(permiso), nil)
}

// MyGroups son los grupos de quien llama, para ofrecerlos al compartir.
func (b *Bases) MyGroups(ctx context.Context) ([]string, error) {
	r, err := basesJSON[struct {
		Groups []string `json:"groups"`
	}](ctx, b, http.MethodGet, "/groups", nil)
	return r.Groups, err
}

// TransferBase da la propiedad a otra persona de la organización, por su dirección.
// Quien llama queda como editor.
func (b *Bases) TransferBase(ctx context.Context, base, email string) error {
	return b.sinCuerpo(ctx, http.MethodPost, "/bases/"+url.PathEscape(base)+"/transfer", map[string]string{"email": email})
}

// Export baja una tabla entera en `csv` o `json`. Cerrar el cuerpo es cosa de quien
// llama. Los campos de enlace salen con sus valores, no con ids.
func (b *Bases) Export(ctx context.Context, tabla, formato string) (io.ReadCloser, error) {
	resp, err := b.c.Do(ctx, Request{Path: "/tables/" + url.PathEscape(tabla) + "/export", Query: url.Values{"format": {formato}}})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
