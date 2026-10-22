// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Plantillas, temas y marca de Sytue Docs: lo que la galería de «Crear nuevo» y el grupo
// «Tema» de la cinta piden al servicio (GET /templates, GET /themes, PUT /brand,
// POST /documents/{id}/theme). Los mismos cables que los BFF `api/templates.ts`,
// `api/themes.ts` y `api/create.ts` de la web.

// DocTemplateVersion es una versión publicada de una plantilla de la organización; la
// primera de la lista es la vigente.
type DocTemplateVersion struct {
	No int    `json:"no"`
	ID string `json:"id"`
}

// DocTemplate es una plantilla de la galería: de Sytue (clave «carta», «presupuesto»…) o
// de la organización (clave «org:<id del documento>», con sus versiones).
type DocTemplate struct {
	Key         string               `json:"key"`
	Kind        DocKind              `json:"kind"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Versions    []DocTemplateVersion `json:"versions,omitempty"`
}

// DocTemplates es la respuesta de la galería.
type DocTemplates struct {
	Templates    []DocTemplate `json:"templates"`
	OrgTemplates []DocTemplate `json:"org_templates"` //nolint:tagliatelle // el cable de la web
	CanManage    bool          `json:"can_manage"`    //nolint:tagliatelle // el cable de la web
}

// Templates pide la galería, con nombre y descripción en el idioma `locale`.
func (d *Docs) Templates(ctx context.Context, locale string) (DocTemplates, error) {
	return JSON[DocTemplates](ctx, d.c, Request{Path: "/templates", Query: url.Values{"locale": {locale}}})
}

// DocBrand es una versión de la marca de la organización.
type DocBrand struct {
	Version     int    `json:"version,omitempty"`
	Name        string `json:"name"`
	Bg          string `json:"bg"`
	Ink         string `json:"ink"`
	Accent      string `json:"accent"`
	HeadingFont string `json:"heading_font,omitempty"` //nolint:tagliatelle // el cable de la web
	BodyFont    string `json:"body_font,omitempty"`    //nolint:tagliatelle // el cable de la web
}

// DocTheme es un tema del catálogo común (solo importa su id: el nombre sale del catálogo
// de idiomas).
type DocTheme struct {
	ID string `json:"id"`
}

// DocThemes es la respuesta de `GET /themes`: el catálogo, la marca vigente (nil si la
// organización no tiene), sus versiones y si quien mira puede cambiarla.
type DocThemes struct {
	Themes    []DocTheme `json:"themes"`
	Brand     *DocBrand  `json:"brand"`
	Versions  []DocBrand `json:"versions"`
	CanManage bool       `json:"can_manage"` //nolint:tagliatelle // el cable de la web
}

// IDTemaMarca es el id con que la marca de la organización se pide como tema.
const IDTemaMarca = "marca"

// Themes pide el catálogo de temas y la marca de la organización.
func (d *Docs) Themes(ctx context.Context) (DocThemes, error) {
	return JSON[DocThemes](ctx, d.c, Request{Path: "/themes"})
}

// SaveBrand guarda una versión NUEVA de la marca (solo quien administra la organización)
// y devuelve su número; las anteriores se conservan.
func (d *Docs) SaveBrand(ctx context.Context, m DocBrand) (int, error) {
	m.Version = 0
	datos, err := json.Marshal(m)
	if err != nil {
		return 0, err
	}
	out, err := JSON[struct {
		Version int `json:"version"`
	}](ctx, d.c, Request{
		Method: http.MethodPut, Path: "/brand", Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	return out.Version, err
}

// DocNuevo es el alta de un documento desde una plantilla (`POST /documents` con
// `template`): el SERVICIO pone tipo, título y contenido.
type DocNuevo struct {
	Template string `json:"template"`
	Title    string `json:"title,omitempty"`
	Locale   string `json:"locale,omitempty"`
	// TemplateVersion elige la versión publicada de una plantilla de la organización;
	// vacía = la vigente.
	TemplateVersion string `json:"template_version,omitempty"` //nolint:tagliatelle // el cable de la web
	// Theme es un id del catálogo o «marca»; BrandVersion elige una versión de la marca
	// (0 = la vigente).
	Theme        string `json:"theme,omitempty"`
	BrandVersion int    `json:"brand_version,omitempty"` //nolint:tagliatelle // el cable de la web
}

// CreateFromTemplate da de alta un documento con el contenido de una plantilla.
func (d *Docs) CreateFromTemplate(ctx context.Context, n DocNuevo) (Document, error) {
	out, err := PostJSON[docResp](ctx, d.c, "/documents", n)
	return out.Document, err
}

// ApplyTheme aplica un tema (del catálogo o «marca») a un documento que ya existe, sin
// tocar su contenido, y dice si algo cambió (`changed`). El servicio guarda antes una
// versión «Antes del tema …», así que restaurarla lo deshace.
func (d *Docs) ApplyTheme(ctx context.Context, id, tema string, versionMarca int) (bool, error) {
	datos, err := json.Marshal(struct {
		Theme        string `json:"theme"`
		BrandVersion int    `json:"brand_version,omitempty"` //nolint:tagliatelle // el cable de la web
	}{tema, versionMarca})
	if err != nil {
		return false, err
	}
	out, err := JSON[struct {
		Changed bool `json:"changed"`
	}](ctx, d.c, Request{
		Method: http.MethodPost, Path: "/documents/" + url.PathEscape(id) + "/theme", Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	return out.Changed, err
}

// DocThemeChange es un cambio que aplicar un tema haría (`domain.CambioTema`): Scope es
// headings, table_headers, body_font, sheet_header, slide_theme o logo; el cliente lo
// traduce.
type DocThemeChange struct {
	Scope string `json:"scope"`
	Sheet string `json:"sheet,omitempty"`
	Count int    `json:"count"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// DocThemePreview es lo que cambiaría, sin haber escrito nada; Version es la de la marca.
type DocThemePreview struct {
	Changed bool             `json:"changed"`
	Changes []DocThemeChange `json:"changes"`
	Version int              `json:"version"`
}

// PreviewTheme pregunta qué cambiaría aplicar ese tema (`preview:true`): no escribe ni
// deja punto de restauración.
func (d *Docs) PreviewTheme(ctx context.Context, id, tema string, versionMarca int) (DocThemePreview, error) {
	datos, err := json.Marshal(struct {
		Theme        string `json:"theme"`
		BrandVersion int    `json:"brand_version,omitempty"` //nolint:tagliatelle // el cable de la web
		Preview      bool   `json:"preview"`
	}{tema, versionMarca, true})
	if err != nil {
		return DocThemePreview{}, err
	}
	return JSON[DocThemePreview](ctx, d.c, Request{
		Method: http.MethodPost, Path: "/documents/" + url.PathEscape(id) + "/theme", Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}
