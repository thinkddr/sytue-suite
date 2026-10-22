// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir

import "strings"

// Tabla de campos del IR: espejo EXACTO de campos.ts (mismos nombres y mismos descriptores; lo
// vigila TestEspejoDeCampos). El formato de los descriptores está explicado allí: `s`, `b`, `t`,
// `i`, `n`, `e:<enumerado>`, `o:<objeto>`, `u:<unión>`, `a:<d>`, `m:<d>`, tuplas `i[4]`, y los
// tipos con nombre `textos`, `ricos`, `nodo`, `medida`, `radio` y `fondo`; sufijos `|null`
// (anulable) y `!` (obligatorio).

// base son las claves que llevan todos los nodos.
var base = map[string]string{
	"id": "s!", "type": "e:tipo_nodo!", "name": "s", "locked": "t", "hidden": "t",
	"size": "o:tamano", "style": "o:estilo", "breakpoints": "o:puntos",
}

func conBase(propias map[string]string) map[string]string {
	t := make(map[string]string, len(base)+len(propias))
	for k, v := range base {
		t[k] = v
	}
	for k, v := range propias {
		t[k] = v
	}
	return t
}

// CamposNodo da, por tipo de nodo, sus campos y descriptores.
var CamposNodo = map[string]map[string]string{
	"section": conBase(map[string]string{
		"kind": "e:tipo_seccion!", "anchor": "s", "full_bleed": "b!", "layout": "o:layout!", "children": "a:nodo!",
	}),
	"frame": conBase(map[string]string{
		"tag": "e:etiqueta_marco!", "layout": "o:layout!", "children": "a:nodo!", "link": "u:enlace",
	}),
	"text": conBase(map[string]string{"tag": "e:etiqueta_texto!", "content": "ricos!"}),
	"image": conBase(map[string]string{
		"src": "u:imagen!", "alt": "textos!", "decorative": "t", "fit": "e:ajuste!", "link": "u:enlace", "priority": "t",
	}),
	"button": conBase(map[string]string{
		"label": "textos!", "variant": "e:variante!", "action": "u:enlace!", "icon": "s", "icon_position": "e:posicion_icono",
	}),
	"icon":  conBase(map[string]string{"name": "s!", "label": "textos"}),
	"embed": conBase(map[string]string{"provider": "e:proveedor!", "ref": "s!", "title": "textos!"}),
	"form": conBase(map[string]string{
		"form": "s!", "layout": "o:layout!", "fields": "a:o:campo!",
		"submit": "textos!", "success": "textos!", "consent": "textos!",
	}),
	"table": conBase(map[string]string{"caption": "textos!", "header": "a:textos!", "rows": "a:a:textos!"}),
	"product_grid": conBase(map[string]string{
		"source": "u:fuente_productos!", "limit": "i!", "columns": "i!", "show_price": "b!", "show_add_to_cart": "b!",
	}),
	"cart_button":       conBase(map[string]string{"label": "textos!"}),
	"language_switcher": conBase(map[string]string{"display": "e:modo_idioma!"}),
	"menu": conBase(map[string]string{
		"items": "a:o:menu_item!", "orientation": "e:orientacion!", "collapse_below": "e:colapso!", "label": "textos",
	}),
	"instance": conBase(map[string]string{"component": "s!", "overrides": "m:o:override"}),
}

// CamposObjeto da, por objeto que no es un nodo, sus campos y descriptores. Las variantes de una
// unión se llaman <unión>_<discriminante>.
var CamposObjeto = map[string]map[string]string{
	"sitio": {
		"version": "i!", "languages": "o:idiomas!", "theme": "o:tema!", "header": "nodo|null!",
		"footer": "nodo|null!", "pages": "a:o:pagina!", "components": "a:o:componente!", "seo": "o:seo!",
	},
	"idiomas": {"default": "s!", "others": "a:s!"},
	"tema": {
		"colors": "m:s!", "fonts": "o:fuentes!", "text_styles": "o:estilos_texto!", "radius": "o:radios!", "container": "i!",
	},
	"fuentes": {"heading": "s!", "body": "s!"},
	"estilos_texto": {
		"h1": "o:estilo_tipografico!", "h2": "o:estilo_tipografico!", "h3": "o:estilo_tipografico!",
		"h4": "o:estilo_tipografico!", "h5": "o:estilo_tipografico!", "h6": "o:estilo_tipografico!",
		"body": "o:estilo_tipografico!", "small": "o:estilo_tipografico!", "label": "o:estilo_tipografico!",
		"button": "o:estilo_tipografico!",
	},
	"estilo_tipografico": {
		"font": "e:familia!", "size": "i[3]!", "line_height": "n!", "weight": "e:peso!", "letter_spacing": "n!",
		"transform": "e:mayusculas",
	},
	"radios": {"sm": "i!", "md": "i!", "lg": "i!"},
	"pagina": {
		"id": "s!", "slug": "textos!", "title": "textos!", "description": "textos!", "sections": "a:nodo!",
		"show_header": "b!", "show_footer": "b!", "noindex": "t", "og_image": "u:imagen",
	},
	"tamano": {
		"width": "medida!", "height": "medida!", "min_width": "i", "max_width": "i", "min_height": "i",
		"max_height": "i", "aspect_ratio": "n",
	},
	"layout": {
		"direction": "e:direccion!", "gap": "i!", "row_gap": "i", "padding": "i[4]!", "align": "e:alinear!",
		"justify": "e:repartir!", "wrap": "b!", "columns": "i",
	},
	"estilo": {
		"background": "fondo", "color": "s", "border": "o:borde", "radius": "radio", "shadow": "e:sombra",
		"opacity": "n", "text": "o:estilo_texto", "overflow": "e:desborde",
	},
	"borde":        {"width": "i!", "color": "s!", "style": "e:trazo!"},
	"fondo_imagen": {"image": "u:imagen!", "fit": "e:ajuste!", "position": "n[2]!", "overlay": "s"},
	"estilo_texto": {
		"preset": "e:estilo_texto", "font": "e:familia", "size": "i", "weight": "e:peso", "line_height": "n",
		"letter_spacing": "n", "align": "e:alinear_texto", "transform": "e:transformar", "italic": "b",
	},
	"puntos": {"tablet": "o:delta", "mobile": "o:delta"},
	"delta": {
		"hidden": "b", "size": "o:tamano_delta", "layout": "o:layout_delta", "style": "o:estilo_delta", "columns": "i",
	},
	"tamano_delta": {
		"width": "medida", "height": "medida", "min_width": "i|null", "max_width": "i|null",
		"min_height": "i|null", "max_height": "i|null", "aspect_ratio": "n|null",
	},
	"layout_delta": {
		"direction": "e:direccion", "gap": "i", "row_gap": "i", "padding": "i[4]", "align": "e:alinear",
		"justify": "e:repartir", "wrap": "b", "columns": "i",
	},
	"estilo_delta": {
		"background": "fondo|null", "color": "s", "border": "o:borde|null", "radius": "radio",
		"shadow": "e:sombra|null", "opacity": "n", "text": "o:estilo_texto", "overflow": "e:desborde",
	},
	"tramo": {"text": "s!", "bold": "t", "italic": "t", "underline": "t", "link": "u:enlace"},
	"campo": {
		"id": "s!", "kind": "e:tipo_campo!", "label": "textos!", "required": "b!", "options": "a:textos",
		"max_length": "i", "min": "i", "max": "i", "autocomplete": "e:autocompletar",
	},
	"menu_item": {"id": "s!", "label": "textos!", "link": "u:enlace!", "children": "a:o:menu_item"},
	"override": {
		"text": "ricos", "label": "textos", "image": "u:imagen", "alt": "textos", "link": "u:enlace|null",
		"icon": "s", "hidden": "b", "style": "o:estilo",
	},
	"componente": {"id": "s!", "name": "s!", "root": "nodo!"},
	"seo": {
		"site_name": "textos!", "og_image": "u:imagen", "favicon": "u:imagen", "noindex": "t", "business": "o:negocio",
	},
	"negocio": {
		"type": "e:tipo_negocio!", "phone": "s", "email": "s", "address": "o:direccion", "geo": "o:geo",
		"opening_hours": "a:o:horario",
	},
	"direccion":                 {"street": "s!", "city": "s!", "postal_code": "s!", "region": "s", "country": "s!"},
	"geo":                       {"lat": "n!", "lon": "n!"},
	"horario":                   {"days": "a:e:dia!", "opens": "s!", "closes": "s!"},
	"enlace_page":               {"page": "s!", "anchor": "s"},
	"enlace_anchor":             {"anchor": "s!"},
	"enlace_url":                {"url": "s!", "new_tab": "b"},
	"enlace_product":            {"product": "s!"},
	"enlace_file":               {"file": "s!"},
	"enlace_legal":              {"legal": "e:legal!"},
	"enlace_cart":               {"cart": "t!"},
	"imagen_asset":              {"asset": "s!"},
	"imagen_library":            {"library": "s!"},
	"imagen_placeholder":        {"placeholder": "e:relleno!"},
	"fuente_productos_all":      {"all": "t!"},
	"fuente_productos_category": {"category": "s!"},
	"fuente_productos_products": {"products": "a:s!"},
}

// Uniones da, por unión de objetos, sus claves discriminantes en orden (ver UNIONES en campos.ts).
var Uniones = map[string][]string{
	"enlace":           {"page", "anchor", "url", "product", "file", "legal", "cart"},
	"imagen":           {"asset", "library", "placeholder"},
	"fuente_productos": {"all", "category", "products"},
}

// Enumerados da los valores admitidos de cada enumerado: cadenas, salvo peso (enteros).
var Enumerados = map[string][]any{
	"tipo_nodo":      cadenas(TiposNodo),
	"tipo_seccion":   cadenas(TiposSeccion),
	"estilo_texto":   cadenas(EstilosTexto),
	"proveedor":      cadenas(ProveedoresIncrustado),
	"familia":        {"heading", "body"},
	"peso":           {400, 500, 600, 700, 800},
	"mayusculas":     {"uppercase"},
	"direccion":      {"row", "column"},
	"alinear":        {"start", "center", "end", "baseline"},
	"repartir":       {"start", "center", "end", "space-between", "space-around", "space-evenly"},
	"sombra":         {"none", "sm", "md", "lg"},
	"desborde":       {"visible", "hidden"},
	"trazo":          {"solid", "dashed"},
	"ajuste":         {"cover", "contain"},
	"alinear_texto":  {"start", "center", "end", "justify"},
	"transformar":    {"none", "uppercase"},
	"etiqueta_marco": {"div", "header", "footer", "nav", "article", "aside", "ul", "ol", "li"},
	"etiqueta_texto": {"h1", "h2", "h3", "h4", "h5", "h6", "p", "span", "blockquote"},
	"relleno":        {"photo", "portrait", "product", "landscape", "logo"},
	"variante":       {"primary", "secondary", "ghost"},
	"posicion_icono": {"start", "end"},
	"tipo_campo":     {"text", "email", "tel", "textarea", "select", "checkbox", "date", "time", "number"},
	"autocompletar": {
		"name", "given-name", "family-name", "organization", "email", "tel", "street-address",
		"postal-code", "address-level2", "country-name",
	},
	"modo_idioma": {"code", "name"},
	"orientacion": {"horizontal", "vertical"},
	"colapso":     {"never", "tablet", "mobile"},
	"legal":       {"notice", "privacy", "cookies", "terms", "withdrawal"},
	"tipo_negocio": {
		"Organization", "LocalBusiness", "Store", "Restaurant", "MedicalClinic", "LodgingBusiness",
		"EducationalOrganization", "ProfessionalService", "HomeAndConstructionBusiness",
	},
	"dia": {"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"},
}

func cadenas(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func limpio(d string) string {
	d = strings.TrimSuffix(d, "!")
	d = strings.TrimSuffix(d, "|null")
	return d
}

func obligatorio(d string) bool {
	return strings.HasSuffix(d, "!")
}
