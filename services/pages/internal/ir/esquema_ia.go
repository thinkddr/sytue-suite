// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:generate go run ./generar

package ir

import (
	"encoding/json"
)

// GenerarEsquemaIA construye el JSON Schema estricto para las secciones generadas por IA.
func GenerarEsquemaIA() ([]byte, error) {
	// Definiciones estrictas del subconjunto de nodos para la IA
	defs := make(map[string]any)

	// Tipos permitidos para la IA
	tiposPermitidos := []string{"section", "frame", "text", "image", "button", "icon", "form", "table", "product_grid"}

	// Helpers de esquema estricto (todo required, additionalProperties: false, opcionales admiten null)
	str := map[string]any{"type": "string"}
	entero := map[string]any{"type": "integer"}
	booleano := map[string]any{"type": "boolean"}
	numero := map[string]any{"type": "number"}

	opcional := func(schema any) any {
		return map[string]any{
			"anyOf": []any{
				schema,
				map[string]any{"type": "null"},
			},
		}
	}

	enumStrings := func(vals []string) any {
		arr := make([]any, 0, len(vals))
		for _, v := range vals {
			arr = append(arr, v)
		}
		return map[string]any{"type": "string", "enum": arr}
	}

	// Layout
	defs["layout"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"direction": enumStrings([]string{"row", "column"}),
			"gap":       entero,
			"row_gap":   opcional(entero),
			"padding":   map[string]any{"type": "array", "items": entero, "minItems": 4, "maxItems": 4},
			"align":     enumStrings([]string{"start", "center", "end", "stretch"}),
			"justify":   enumStrings([]string{"start", "center", "end", "space-between", "space-around"}),
			"wrap":      booleano,
			"columns":   opcional(entero),
		},
		"required": []string{"direction", "gap", "padding", "align", "justify", "wrap"},
	}

	// Medida
	defs["medida"] = map[string]any{
		"anyOf": []any{
			entero,
			map[string]any{"type": "string", "enum": []any{"fill", "hug"}},
		},
	}

	// Tamano
	defs["tamano"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"width":        map[string]any{"$ref": "#/$defs/medida"},
			"height":       map[string]any{"$ref": "#/$defs/medida"},
			"min_width":    opcional(entero),
			"max_width":    opcional(entero),
			"min_height":   opcional(entero),
			"max_height":   opcional(entero),
			"aspect_ratio": opcional(numero),
		},
		"required": []string{"width", "height"},
	}

	// Enlace
	defs["enlace"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"page":    opcional(str),
			"anchor":  opcional(str),
			"url":     opcional(str),
			"new_tab": opcional(booleano),
			"product": opcional(str),
			"file":    opcional(str),
			"legal":   opcional(enumStrings([]string{"notice", "privacy", "cookies", "terms", "withdrawal"})),
			"cart":    opcional(booleano),
		},
	}

	// EstiloTexto
	defs["estilo_texto"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"preset":         opcional(enumStrings(EstilosTexto)),
			"font":           opcional(str),
			"size":           opcional(entero),
			"line_height":    opcional(numero),
			"weight":         opcional(enumStrings([]string{"400", "500", "600", "700"})),
			"letter_spacing": opcional(numero),
			"align":          opcional(enumStrings([]string{"start", "center", "end", "justify"})),
			"transform":      opcional(enumStrings([]string{"none", "uppercase"})),
			"italic":         opcional(booleano),
		},
	}

	// Estilo
	defs["estilo"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"background": opcional(str),
			"color":      opcional(str),
			"border": opcional(map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"width": entero,
					"color": str,
					"style": enumStrings([]string{"solid", "dashed", "dotted"}),
				},
				"required": []string{"width", "color", "style"},
			}),
			"radius":   opcional(map[string]any{"anyOf": []any{entero, enumStrings([]string{"$sm", "$md", "$lg"})}}),
			"opacity":  opcional(numero),
			"text":     opcional(map[string]any{"$ref": "#/$defs/estilo_texto"}),
			"overflow": opcional(enumStrings([]string{"visible", "hidden"})),
		},
	}

	// Breakpoint Delta
	defs["delta"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"hidden":  opcional(booleano),
			"size":    opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"layout":  opcional(map[string]any{"$ref": "#/$defs/layout"}),
			"style":   opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"columns": opcional(entero),
		},
	}

	// Puntos (Breakpoints)
	defs["puntos"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"tablet": opcional(map[string]any{"$ref": "#/$defs/delta"}),
			"mobile": opcional(map[string]any{"$ref": "#/$defs/delta"}),
		},
	}

	// Campo de formulario
	defs["campo_formulario"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":           str,
			"kind":         enumStrings([]string{"text", "email", "tel", "textarea", "select", "checkbox", "date", "time", "number"}),
			"label":        str,
			"required":     opcional(booleano),
			"options":      opcional(map[string]any{"type": "array", "items": str}),
			"max_length":   opcional(entero),
			"autocomplete": opcional(str),
			"min":          opcional(numero),
			"max":          opcional(numero),
		},
		"required": []string{"id", "kind", "label"},
	}

	// Nodos individuales para IA
	// section
	defs["section"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":          str,
			"type":        map[string]any{"type": "string", "const": "section"},
			"name":        opcional(str),
			"kind":        enumStrings(TiposSeccion),
			"anchor":      opcional(str),
			"full_bleed":  booleano,
			"layout":      map[string]any{"$ref": "#/$defs/layout"},
			"style":       opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints": opcional(map[string]any{"$ref": "#/$defs/puntos"}),
			"children": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/$defs/nodo"},
			},
		},
		"required": []string{"id", "type", "kind", "full_bleed", "layout", "children"},
	}

	// frame
	defs["frame"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":          str,
			"type":        map[string]any{"type": "string", "const": "frame"},
			"name":        opcional(str),
			"tag":         enumStrings([]string{"div", "header", "footer", "nav", "article", "aside", "ul", "ol", "li"}),
			"layout":      map[string]any{"$ref": "#/$defs/layout"},
			"size":        opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"style":       opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints": opcional(map[string]any{"$ref": "#/$defs/puntos"}),
			"link":        opcional(map[string]any{"$ref": "#/$defs/enlace"}),
			"children": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/$defs/nodo"},
			},
		},
		"required": []string{"id", "type", "tag", "layout", "children"},
	}

	// text
	defs["text"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":          str,
			"type":        map[string]any{"type": "string", "const": "text"},
			"name":        opcional(str),
			"tag":         enumStrings([]string{"h1", "h2", "h3", "h4", "h5", "h6", "p", "span", "blockquote"}),
			"content":     str,
			"size":        opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"style":       opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints": opcional(map[string]any{"$ref": "#/$defs/puntos"}),
		},
		"required": []string{"id", "type", "tag", "content"},
	}

	// image (solo library o placeholder para IA)
	defs["image"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":   str,
			"type": map[string]any{"type": "string", "const": "image"},
			"name": opcional(str),
			"src": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"library":     opcional(str),
					"placeholder": opcional(enumStrings([]string{"photo", "portrait", "product", "landscape", "logo"})),
				},
			},
			"alt":         str,
			"decorative":  opcional(booleano),
			"fit":         enumStrings([]string{"cover", "contain"}),
			"priority":    opcional(booleano),
			"size":        opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"style":       opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints": opcional(map[string]any{"$ref": "#/$defs/puntos"}),
			"link":        opcional(map[string]any{"$ref": "#/$defs/enlace"}),
		},
		"required": []string{"id", "type", "src", "alt", "fit"},
	}

	// button
	defs["button"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":            str,
			"type":          map[string]any{"type": "string", "const": "button"},
			"name":          opcional(str),
			"label":         str,
			"variant":       enumStrings([]string{"primary", "secondary", "ghost"}),
			"action":        map[string]any{"$ref": "#/$defs/enlace"},
			"icon":          opcional(enumStrings(Iconos)),
			"icon_position": opcional(enumStrings([]string{"start", "end"})),
			"size":          opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"style":         opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints":   opcional(map[string]any{"$ref": "#/$defs/puntos"}),
		},
		"required": []string{"id", "type", "label", "variant", "action"},
	}

	// icon
	defs["icon"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":          str,
			"type":        map[string]any{"type": "string", "const": "icon"},
			"name":        enumStrings(Iconos),
			"label":       opcional(str),
			"size":        opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"style":       opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints": opcional(map[string]any{"$ref": "#/$defs/puntos"}),
		},
		"required": []string{"id", "type", "name"},
	}

	// form
	defs["form"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":          str,
			"type":        map[string]any{"type": "string", "const": "form"},
			"name":        opcional(str),
			"form":        str,
			"layout":      map[string]any{"$ref": "#/$defs/layout"},
			"fields":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/campo_formulario"}},
			"submit":      str,
			"success":     str,
			"consent":     str,
			"size":        opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"style":       opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints": opcional(map[string]any{"$ref": "#/$defs/puntos"}),
		},
		"required": []string{"id", "type", "form", "layout", "fields", "submit", "success", "consent"},
	}

	// table
	defs["table"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":          str,
			"type":        map[string]any{"type": "string", "const": "table"},
			"name":        opcional(str),
			"caption":     str,
			"header":      map[string]any{"type": "array", "items": str},
			"rows":        map[string]any{"type": "array", "items": map[string]any{"type": "array", "items": str}},
			"size":        opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"style":       opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints": opcional(map[string]any{"$ref": "#/$defs/puntos"}),
		},
		"required": []string{"id", "type", "caption", "header", "rows"},
	}

	// product_grid
	defs["product_grid"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":   str,
			"type": map[string]any{"type": "string", "const": "product_grid"},
			"name": opcional(str),
			"source": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"all":      opcional(booleano),
					"category": opcional(str),
					"products": opcional(map[string]any{"type": "array", "items": str}),
				},
			},
			"limit":            entero,
			"columns":          entero,
			"show_price":       booleano,
			"show_add_to_cart": booleano,
			"size":             opcional(map[string]any{"$ref": "#/$defs/tamano"}),
			"style":            opcional(map[string]any{"$ref": "#/$defs/estilo"}),
			"breakpoints":      opcional(map[string]any{"$ref": "#/$defs/puntos"}),
		},
		"required": []string{"id", "type", "source", "limit", "columns", "show_price", "show_add_to_cart"},
	}

	// Unión nodo
	nodosRefs := make([]any, 0, len(tiposPermitidos))
	for _, t := range tiposPermitidos {
		nodosRefs = append(nodosRefs, map[string]any{"$ref": "#/$defs/" + t})
	}
	defs["nodo"] = map[string]any{
		"anyOf": nodosRefs,
	}

	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"title":                "EsquemaSeccionesIA",
		"description":          "Subconjunto estricto del IR de Sytue Web para generación de secciones mediante IA",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"sections": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/$defs/section"},
			},
		},
		"required": []string{"sections"},
		"$defs":    defs,
	}

	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
