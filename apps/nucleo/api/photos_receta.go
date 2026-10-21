// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
)

// La receta de revelado de una foto: la lista ORDENADA de pasos que el editor aplica
// sin tocar el original. Es la de `editor/receta.ts` en la web y la de
// `services/syphotos/internal/domain/receta.go` en el servicio; la forma canónica y la
// huella tienen que salir byte a byte iguales en los tres, o el editor enseña una foto y
// se descarga otra (los vectores del test son los que imprimió Go).

// VersionReceta es el formato que entiende esta compilación.
const VersionReceta = 1

// Los tipos de paso.
const (
	PasoRecorte     = "recorte"     // x, y, ancho, alto en fracción del lienzo
	PasoGiro        = "giro"        // grados: 90, 180, 270
	PasoEspejo      = "espejo"      // eje: «h» o «v»
	PasoExposicion  = "exposicion"  // pasos de diafragma, −5..+5
	PasoContraste   = "contraste"   // −1..+1
	PasoSaturacion  = "saturacion"  // −1..+1
	PasoTemperatura = "temperatura" // −1..+1 (frío → cálido)
)

// PasoReceta es una operación con sus parámetros.
type PasoReceta struct {
	Tipo   string             `json:"tipo"`
	Params map[string]float64 `json:"params,omitempty"`
	Eje    string             `json:"eje,omitempty"`
}

// Receta es la lista de pasos. El orden importa: recortar y luego girar no da lo mismo
// que girar y luego recortar.
type Receta struct {
	Version int          `json:"version"`
	Pasos   []PasoReceta `json:"pasos"`
}

// RecetaVacia es la de una foto sin editar, con su huella como cualquier otra: es con la
// que se pide el ORIGINAL (la derivada vigente lleva la receta horneada).
func RecetaVacia() Receta { return Receta{Version: VersionReceta, Pasos: []PasoReceta{}} }

// Canonica es la serialización estable de la que sale la huella: claves ordenadas y los
// números como CADENA con seis decimales, para que el ruido del último bit de un
// deslizador no sea una derivada nueva.
func (r Receta) Canonica() string {
	pasos := make([]map[string]any, 0, len(r.Pasos))
	for _, p := range r.Pasos {
		m := map[string]any{"tipo": p.Tipo}
		if p.Eje != "" {
			m["eje"] = p.Eje
		}
		if len(p.Params) > 0 {
			params := make(map[string]string, len(p.Params))
			for n, v := range p.Params {
				params[n] = fmt.Sprintf("%.6f", v)
			}
			m["params"] = params
		}
		pasos = append(pasos, m)
	}
	// encoding/json ordena las claves de un mapa: es lo que hace `canonica` en Go.
	b, _ := json.Marshal(map[string]any{"version": r.Version, "pasos": pasos})
	return string(b)
}

// Huella identifica el RESULTADO de aplicar la receta: doce hexadecimales del SHA-256
// de la forma canónica. Con ella se nombran las derivadas.
func (r Receta) Huella() string {
	suma := sha256.Sum256([]byte(r.Canonica()))
	return hex.EncodeToString(suma[:])[:12]
}

// Igual dice si dos recetas dan la misma imagen, que es lo que decide si hay algo sin
// guardar.
func (r Receta) Igual(o Receta) bool { return r.Canonica() == o.Canonica() }

// Clonar copia la receta para cambiarla sin tocar la de quien la tiene.
func (r Receta) Clonar() Receta {
	c := Receta{Version: r.Version, Pasos: make([]PasoReceta, len(r.Pasos))}
	for i, p := range r.Pasos {
		c.Pasos[i] = PasoReceta{Tipo: p.Tipo, Eje: p.Eje}
		if p.Params != nil {
			c.Pasos[i].Params = make(map[string]float64, len(p.Params))
			for k, v := range p.Params {
				c.Pasos[i].Params[k] = v
			}
		}
	}
	return c
}

// EsDeGeometria dice si el paso mueve píxeles de sitio (recorte, giro, espejo) en vez
// de cambiar su color.
func (p PasoReceta) EsDeGeometria() bool {
	return slices.Contains([]string{PasoRecorte, PasoGiro, PasoEspejo}, p.Tipo)
}

// LeerReceta trae la receta vigente de una foto.
func (p *Photos) LeerReceta(ctx context.Context, id string) (Receta, error) {
	r, err := JSON[struct {
		Recipe Receta `json:"recipe"`
	}](ctx, p.c, Request{Path: "/assets/" + url.PathEscape(id) + "/recipe"})
	if r.Recipe.Version == 0 {
		r.Recipe = RecetaVacia()
	}
	if r.Recipe.Pasos == nil {
		r.Recipe.Pasos = []PasoReceta{}
	}
	return r.Recipe, err
}

// GuardarReceta manda la receta ENTERA y devuelve la huella con la que el servicio
// nombra las derivadas nuevas.
func (p *Photos) GuardarReceta(ctx context.Context, id string, r Receta) (string, error) {
	cuerpo, err := json.Marshal(map[string]any{"recipe": r})
	if err != nil {
		return "", err
	}
	g, err := JSON[struct {
		Fingerprint string `json:"fingerprint"`
	}](ctx, p.c, Request{
		Method: http.MethodPut, Path: "/assets/" + url.PathEscape(id) + "/recipe", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	return g.Fingerprint, err
}
