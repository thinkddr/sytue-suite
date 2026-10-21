// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package odata es el driver TabularSource para servicios OData v4 (SAP S/4HANA
// OData y OData genérico): trae colecciones de entity sets a Sytue Sheets como
// tablas. Implementa el contrato connector.Driver (Test) y la capacidad opcional
// connector.TabularSource (Query), más un Schema que parsea el $metadata (CSDL
// XML) a entity sets/campos para descubrimiento.
//
// Todo el egress sale por rt.HTTP (el único cliente de la pasarela: SSRF +
// rate-limit + auditoría + inyección del Bearer del vault). El driver NUNCA
// construye su propio http.Client. La paginación server-driven de OData se sigue
// por `@odata.nextLink`, acotada a filas/páginas máximas y AL MISMO HOST del
// service root (un nextLink cross-host se rechaza: filtraría el Bearer inyectado).
package odata

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// Kind es la clave del Registry con la que se registra este driver.
const Kind = "odata"

// ConfigServiceURL es la clave de Instance.Config con la raíz del servicio OData
// (p. ej. https://sap.example.com/sap/opu/odata4/sap/api_business_partner/).
const ConfigServiceURL = "service_url"

const (
	maxRespBytes    = 16 << 20 // tope de lectura por página (defensa anti-OOM)
	maxMetadataByte = 8 << 20  // tope de lectura del $metadata
	maxPages        = 1000     // tope de saltos de nextLink (defensa anti-bucle)
	defaultMaxRows  = 10000    // tope de filas acumuladas si la spec no lo fija
)

// Driver es el conector OData v4. No tiene estado: cada operación usa el Runtime
// inyectado. Su valor cero es utilizable; New lo devuelve por simetría con el
// resto de drivers.
type Driver struct{}

// New construye el driver (se cablea en el Registry del gateway).
func New() *Driver { return &Driver{} }

// Kind identifica al driver en el Registry.
func (*Driver) Kind() string { return Kind }

// Test verifica auth + conectividad haciendo GET al documento de servicio (la
// raíz OData), usando EXCLUSIVAMENTE rt.HTTP. No devuelve datos.
func (*Driver) Test(ctx context.Context, inst connector.Instance, rt connector.Runtime) error {
	root, err := serviceRoot(inst)
	if err != nil {
		return err
	}
	resp, err := doGet(ctx, rt, root, "application/json")
	if err != nil {
		return fmt.Errorf("odata: test: %w", err)
	}
	defer drainClose(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("odata: la raíz del servicio respondió HTTP %d", resp.StatusCode)
	}
	return nil
}

// Query trae una colección de un entity set como Table. spec es odataQuery
// (dataset/entity set + $select/$filter/$top/$skip); sigue la paginación
// `@odata.nextLink` hasta agotar filas/páginas. Egress por rt.HTTP.
func (*Driver) Query(ctx context.Context, inst connector.Instance, rt connector.Runtime, spec json.RawMessage) (connector.Table, error) {
	root, err := serviceRoot(inst)
	if err != nil {
		return connector.Table{}, err
	}
	var q odataQuery
	if len(spec) > 0 {
		if err := json.Unmarshal(spec, &q); err != nil {
			return connector.Table{}, fmt.Errorf("%w: la spec no es JSON válido", connector.ErrBadQuery)
		}
	}
	if err := q.validate(); err != nil {
		return connector.Table{}, err
	}
	records, err := fetchAll(ctx, rt, root, q.buildURL(root), q.maxRows())
	if err != nil {
		return connector.Table{}, err
	}
	return connector.BuildTable(records, q.Select, q.maxRows()), nil
}

// Schema parsea el $metadata (CSDL XML) del servicio a entity sets con sus campos
// (tipo lógico derivado del tipo Edm). Sirve al descubrimiento de datasets. Egress
// por rt.HTTP.
func (*Driver) Schema(ctx context.Context, inst connector.Instance, rt connector.Runtime) (Metadata, error) {
	root, err := serviceRoot(inst)
	if err != nil {
		return Metadata{}, err
	}
	resp, err := doGet(ctx, rt, root+"$metadata", "application/xml")
	if err != nil {
		return Metadata{}, fmt.Errorf("odata: metadata: %w", err)
	}
	defer drainClose(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Metadata{}, fmt.Errorf("odata: $metadata respondió HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataByte+1))
	if err != nil {
		return Metadata{}, fmt.Errorf("odata: leer $metadata: %w", err)
	}
	if int64(len(body)) > maxMetadataByte {
		return Metadata{}, fmt.Errorf("odata: $metadata demasiado grande")
	}
	return parseMetadata(body)
}

// --- Consulta ----------------------------------------------------------------

// odataQuery es la spec de una consulta OData (JSON opaco para el núcleo). Dataset
// y EntitySet son sinónimos (dataset es el nombre común en la API de datos);
// Filter es una expresión OData $filter cruda; Top/Skip pagina; MaxRows acota el
// total acumulado en cliente (independiente de la paginación del servidor).
type odataQuery struct {
	Dataset   string   `json:"dataset,omitempty"`
	EntitySet string   `json:"entity_set,omitempty"`
	Select    []string `json:"select,omitempty"`
	Filter    string   `json:"filter,omitempty"`
	Top       int      `json:"top,omitempty"`
	Skip      int      `json:"skip,omitempty"`
	MaxRows   int      `json:"max_rows,omitempty"`
}

func (q odataQuery) entity() string {
	if strings.TrimSpace(q.EntitySet) != "" {
		return strings.TrimSpace(q.EntitySet)
	}
	return strings.TrimSpace(q.Dataset)
}

func (q odataQuery) maxRows() int {
	if q.MaxRows > 0 {
		return q.MaxRows
	}
	return defaultMaxRows
}

func (q odataQuery) validate() error {
	e := q.entity()
	if e == "" {
		return fmt.Errorf("%w: falta 'dataset' (entity set)", connector.ErrBadQuery)
	}
	if strings.ContainsAny(e, "/?#") {
		return fmt.Errorf("%w: nombre de entity set inválido", connector.ErrBadQuery)
	}
	if q.Top < 0 || q.Skip < 0 {
		return fmt.Errorf("%w: top/skip no pueden ser negativos", connector.ErrBadQuery)
	}
	return nil
}

// buildURL compone la URL de la PRIMERA página: raíz + entity set + parámetros
// OData ($select/$filter/$top/$skip). Los saltos siguientes se toman verbatim del
// nextLink (ver fetchAll).
func (q odataQuery) buildURL(root string) string {
	u := strings.TrimRight(root, "/") + "/" + url.PathEscape(q.entity())
	var ps []string
	if len(q.Select) > 0 {
		ps = append(ps, "$select="+url.QueryEscape(strings.Join(q.Select, ",")))
	}
	if strings.TrimSpace(q.Filter) != "" {
		ps = append(ps, "$filter="+url.QueryEscape(q.Filter))
	}
	if q.Top > 0 {
		ps = append(ps, "$top="+strconv.Itoa(q.Top))
	}
	if q.Skip > 0 {
		ps = append(ps, "$skip="+strconv.Itoa(q.Skip))
	}
	if len(ps) > 0 {
		u += "?" + strings.Join(ps, "&")
	}
	return u
}

// odataPage es la envoltura de una colección OData v4: los objetos en "value" y el
// enlace a la página siguiente (server-driven paging).
type odataPage struct {
	Value    []map[string]any `json:"value"`
	NextLink string           `json:"@odata.nextLink"`
}

// fetchAll recorre las páginas siguiendo nextLink hasta agotar filas o páginas.
// Acumula los registros (sin las anotaciones @odata.* de control). Cada salto se
// re-valida por SSRF en rt.HTTP y se exige que quede en el MISMO host que la raíz.
func fetchAll(ctx context.Context, rt connector.Runtime, root, first string, maxRows int) ([]map[string]any, error) {
	rootHost := hostOf(root)
	out := make([]map[string]any, 0, 128)
	next := first
	for page := 0; next != "" && page < maxPages; page++ {
		resp, err := doGet(ctx, rt, next, "application/json")
		if err != nil {
			// Preserva ssrfhttp.ErrBlocked / errores de transporte (errors.Is en la capa HTTP).
			return nil, fmt.Errorf("odata: %w", err)
		}
		body, err := readOK(resp)
		if err != nil {
			return nil, err
		}
		var pg odataPage
		if err := json.Unmarshal(body, &pg); err != nil {
			return nil, fmt.Errorf("odata: la respuesta no es una colección JSON: %w", err)
		}
		for _, rec := range pg.Value {
			out = append(out, stripControl(rec))
			if len(out) >= maxRows {
				return out[:maxRows], nil
			}
		}
		nl := strings.TrimSpace(pg.NextLink)
		if nl == "" {
			break
		}
		if next, err = resolveNext(root, rootHost, nl); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// resolveNext resuelve un @odata.nextLink (absoluto o relativo a la raíz) y exige
// que apunte al MISMO host que la raíz. Un nextLink que cambie de host filtraría
// el Bearer inyectado a otro destino: se bloquea.
func resolveNext(root, rootHost, nl string) (string, error) {
	nu, err := url.Parse(nl)
	if err != nil {
		return "", fmt.Errorf("odata: nextLink inválido: %w", err)
	}
	if !nu.IsAbs() {
		base, err := url.Parse(root)
		if err != nil {
			return "", fmt.Errorf("odata: raíz inválida: %w", err)
		}
		nu = base.ResolveReference(nu)
	}
	if nu.Scheme != "http" && nu.Scheme != "https" {
		return "", fmt.Errorf("odata: nextLink con esquema no permitido")
	}
	if !strings.EqualFold(nu.Hostname(), rootHost) {
		return "", fmt.Errorf("odata: nextLink cambia de host (%s → %s): bloqueado", rootHost, nu.Hostname())
	}
	return nu.String(), nil
}

// --- $metadata (CSDL v4) -----------------------------------------------------

// Metadata es el esquema descubierto: los entity sets del servicio con sus campos.
type Metadata struct {
	EntitySets []EntitySet `json:"entity_sets"`
}

// EntitySet es una colección expuesta por el servicio y los campos de su tipo.
type EntitySet struct {
	Name       string  `json:"name"`
	EntityType string  `json:"entity_type"`
	Fields     []Field `json:"fields"`
}

// Field es una propiedad de un entity type con su tipo lógico (derivado del Edm).
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// edmx modela lo justo del CSDL v4 para extraer entity types y sets. encoding/xml
// empareja por nombre LOCAL (ignora el prefijo/espacio de nombres edmx:), así que
// no hace falta declarar los namespaces.
type edmx struct {
	XMLName      xml.Name `xml:"Edmx"`
	DataServices struct {
		Schemas []struct {
			Namespace   string `xml:"Namespace,attr"`
			EntityTypes []struct {
				Name       string `xml:"Name,attr"`
				Properties []struct {
					Name string `xml:"Name,attr"`
					Type string `xml:"Type,attr"`
				} `xml:"Property"`
			} `xml:"EntityType"`
			Containers []struct {
				EntitySets []struct {
					Name       string `xml:"Name,attr"`
					EntityType string `xml:"EntityType,attr"`
				} `xml:"EntitySet"`
			} `xml:"EntityContainer"`
		} `xml:"Schema"`
	} `xml:"DataServices"`
}

// parseMetadata decodifica el CSDL y resuelve, para cada entity set, los campos de
// su entity type (mapeando los tipos Edm a tipos lógicos).
func parseMetadata(body []byte) (Metadata, error) {
	var doc edmx
	if err := xml.Unmarshal(body, &doc); err != nil {
		return Metadata{}, fmt.Errorf("odata: $metadata no es XML CSDL válido: %w", err)
	}
	// Índice de entity types por nombre local (para resolver el tipo de cada set).
	type fieldList = []Field
	types := map[string]fieldList{}
	for _, sch := range doc.DataServices.Schemas {
		for _, et := range sch.EntityTypes {
			fields := make([]Field, 0, len(et.Properties))
			for _, p := range et.Properties {
				fields = append(fields, Field{Name: p.Name, Type: edmToLogical(p.Type)})
			}
			types[et.Name] = fields
			if sch.Namespace != "" {
				types[sch.Namespace+"."+et.Name] = fields
			}
		}
	}
	var out Metadata
	for _, sch := range doc.DataServices.Schemas {
		for _, cont := range sch.Containers {
			for _, es := range cont.EntitySets {
				fields := types[es.EntityType]
				if fields == nil {
					fields = types[localName(es.EntityType)]
				}
				out.EntitySets = append(out.EntitySets, EntitySet{
					Name:       es.Name,
					EntityType: es.EntityType,
					Fields:     fields,
				})
			}
		}
	}
	return out, nil
}

// edmToLogical mapea un tipo Edm (Edm.Int32, Edm.String…) al tipo lógico de la
// suite (number/text/boolean/datetime/json), coherente con la inferencia de Table.
func edmToLogical(edm string) string {
	switch localName(edm) {
	case "Int16", "Int32", "Int64", "Byte", "SByte", "Decimal", "Double", "Single":
		return "number"
	case "Boolean":
		return "boolean"
	case "Date", "DateTimeOffset", "TimeOfDay", "Duration":
		return "datetime"
	case "String", "Guid", "Binary":
		return "text"
	default:
		return "text"
	}
}

// localName devuelve el último segmento de un nombre cualificado (Ns.Type → Type).
func localName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// --- helpers de red ----------------------------------------------------------

// serviceRoot valida y normaliza (con barra final) la raíz del servicio OData.
func serviceRoot(inst connector.Instance) (string, error) {
	raw := strings.TrimSpace(inst.Config[ConfigServiceURL])
	if raw == "" {
		return "", fmt.Errorf("%w: falta %q en la config de la instancia", connector.ErrBadQuery, ConfigServiceURL)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w: %q no es una URL http(s) válida", connector.ErrBadQuery, ConfigServiceURL)
	}
	if !strings.HasSuffix(raw, "/") {
		raw += "/"
	}
	return raw, nil
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// doGet emite un GET por rt.HTTP con el Accept dado. No valida el status (lo hacen
// los llamantes según el contexto).
func doGet(ctx context.Context, rt connector.Runtime, rawURL, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")
	return rt.HTTP.Do(req)
}

// readOK exige 2xx y devuelve el cuerpo acotado a maxRespBytes.
func readOK(resp *http.Response) ([]byte, error) {
	defer drainClose(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("odata: el origen respondió HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, fmt.Errorf("odata: leer respuesta: %w", err)
	}
	if int64(len(body)) > maxRespBytes {
		return nil, fmt.Errorf("odata: respuesta demasiado grande (> %d bytes)", maxRespBytes)
	}
	return body, nil
}

// stripControl elimina las anotaciones de control OData (@odata.etag, @odata.id…)
// de un registro para no contaminar las columnas.
func stripControl(rec map[string]any) map[string]any {
	for k := range rec {
		if strings.HasPrefix(k, "@") {
			delete(rec, k)
		}
	}
	return rec
}

// drainClose consume y cierra el cuerpo para reutilizar la conexión.
func drainClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

// Compile-time: satisface Driver y la capacidad opcional TabularSource.
var (
	_ connector.Driver        = (*Driver)(nil)
	_ connector.TabularSource = (*Driver)(nil)
)
