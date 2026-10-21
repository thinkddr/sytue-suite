// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package publicar

import (
	_ "embed" // permitido.json va dentro del binario.
	"encoding/json"
	"fmt"
	"regexp"
)

// permitidoJSON es LA lista de lo que puede llevar una web publicada
// (PAGES-ENCARGO A2, §7.4): elementos y atributos, esquemas de URL, orígenes de
// iframe, scripts, CSS, sitemap y robots. Es única: la aplica el validador de
// salida de este paquete y la leen los tests de oro del renderizador
// (@sytue/pages-render), así que lo que el generador emite y lo que el servicio
// acepta no pueden divergir.
//
// Los nombres de atributo van en MINÚSCULAS (`viewbox`): los tokenizadores de HTML
// los pliegan. `{L}` en las rutas es la alternativa de los idiomas del sitio y
// `$PAGES_CAST_ORIGIN` el origen de Cast de la configuración.
//
//go:embed permitido.json
var permitidoJSON []byte

// Permitido es permitido.json leído.
type Permitido struct {
	Version int               `json:"version"`
	Rutas   []string          `json:"rutas"`
	Tipos   map[string]string `json:"tipos"`
	Limites struct {
		Ficheros   int   `json:"ficheros"`
		BytesHTML  int64 `json:"bytes_html"`
		BytesTotal int64 `json:"bytes_total"`
	} `json:"limites"`
	HTML struct {
		AtributosGlobales []string            `json:"atributos_globales"`
		PrefijosGlobales  []string            `json:"prefijos_globales"`
		Elementos         map[string][]string `json:"elementos"`
		Valores           map[string][]string `json:"valores"`
		AtributosURL      []string            `json:"atributos_url"`
		AtributosSrcset   []string            `json:"atributos_srcset"`
		Esquemas          []string            `json:"esquemas"`
		FormAction        string              `json:"form_action"`
		IframeOrigenes    []string            `json:"iframe_origenes"`
		Scripts           []ScriptPermitido   `json:"scripts"`
	} `json:"html"`
	SVG struct {
		AtributosComunes []string            `json:"atributos_comunes"`
		Elementos        map[string][]string `json:"elementos"`
	} `json:"svg"`
	CSS struct {
		ReglasAt        []string `json:"reglas_at"`
		URLPrefijos     []string `json:"url_prefijos"`
		DataURLMaxBytes int      `json:"data_url_max_bytes"`
		Prohibido       []string `json:"prohibido"`
	} `json:"css"`
	VistaPrevia struct {
		Franja struct {
			Elemento string `json:"elemento"`
			Clase    string `json:"clase"`
			Role     string `json:"role"`
		} `json:"franja"`
		SinElementos []string `json:"sin_elementos"`
	} `json:"vista_previa"`
	Sitemap struct {
		Elementos []string            `json:"elementos"`
		Atributos map[string][]string `json:"atributos"`
	} `json:"sitemap"`
	Robots []string `json:"robots"`
}

// ScriptPermitido es una de las tres formas de <script> que admite una web: el
// paquete de islas embebido, el de la analítica y el JSON-LD en línea.
type ScriptPermitido struct {
	Type    string `json:"type,omitempty"`
	Src     string `json:"src,omitempty"`
	Defer   bool   `json:"defer,omitempty"`
	EnLinea string `json:"en_linea,omitempty"`
}

// CargarPermitido lee la lista embebida y comprueba que sus expresiones compilan.
func CargarPermitido() (*Permitido, error) {
	var p Permitido
	if err := json.Unmarshal(permitidoJSON, &p); err != nil {
		return nil, fmt.Errorf("publicar: permitido.json: %w", err)
	}
	exprs := append([]string{p.HTML.FormAction}, p.Rutas...)
	for _, s := range p.HTML.Scripts {
		if s.Src != "" {
			exprs = append(exprs, s.Src)
		}
	}
	for _, e := range exprs {
		if _, err := regexp.Compile(e); err != nil {
			return nil, fmt.Errorf("publicar: permitido.json: expresión %q: %w", e, err)
		}
	}
	return &p, nil
}
