// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// COMPARAR DOS PDF: qué texto cambia en cada página o, con `modo=visual`, qué píxeles.
//
// Es el cuerpo de `/pdf/compare`, que antes montaba el servicio de Docs: con el Drive
// en la bóveda (ADR 0086) los dos documentos solo existen descifrados en el
// dispositivo, así que el estudio del navegador (`DeviceSession.Compare`) y la
// fachada local de las apps nativas comparan con la MISMA función y contestan el
// mismo JSON.

// VisualCompareTimeout acota una comparación visual de todo el documento. Se mira
// entre página y página (el intérprete no se interrumpe a medias); es menor que el
// plazo del router del servicio para que este devuelva lo hecho con `truncado`.
const VisualCompareTimeout = 25 * time.Second

// ParseVisualOptions lee y valida los ajustes de la comparación visual (`escala`,
// `tolerancia`, `vecindad`). Un valor fuera de rango se RECHAZA y no se recorta en
// silencio: quien pide una tolerancia de 900 se ha equivocado.
func ParseVisualOptions(fields map[string]string) (OpcionesVisual, error) {
	var o OpcionesVisual
	if s := strings.TrimSpace(fields["escala"]); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || !(v >= 0.25 && v <= 3) { // «!(…)»: NaN no es menor ni mayor que nada
			return o, inputError("The scale must be between 0.25 and 3.")
		}
		o.Escala = v
	}
	if s := strings.TrimSpace(fields["tolerancia"]); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > 255 {
			return o, inputError("The tolerance must be between 1 and 255.")
		}
		o.Tolerancia = v
	}
	if s := strings.TrimSpace(fields["vecindad"]); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 || v > 3 {
			return o, inputError("The neighbourhood must be between 0 and 3.")
		}
		o.Vecindad, o.SinVecindad = v, v == 0
	}
	return o, nil
}

// ApplyCompareOperation compara `a` con `b` y devuelve el JSON de `/pdf/compare`:
// por texto (`modo` vacío o `texto`), o por píxeles (`modo=visual`): sin `pagina`,
// el resumen de todo el documento; con `pagina`, esa página con su resaltado.
func ApplyCompareOperation(ctx context.Context, a, b *PDFDoc, fields map[string]string) ([]byte, error) {
	if a == nil || b == nil {
		return nil, inputError("Comparing needs exactly two PDFs.")
	}
	switch mode := strings.TrimSpace(fields["modo"]); mode {
	case "", "texto":
		diffs, err := CompararPDF(a, b)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"diferencias": diffs, "iguales": len(diffs) == 0})
	case "visual":
		return compareVisual(ctx, a, b, fields)
	default:
		return nil, inputError(`The mode must be "texto" or "visual".`)
	}
}

func compareVisual(ctx context.Context, a, b *PDFDoc, fields map[string]string) ([]byte, error) {
	opts, err := ParseVisualOptions(fields)
	if err != nil {
		return nil, err
	}
	if s := strings.TrimSpace(fields["pagina"]); s != "" {
		n, err := ParseOptionalInteger(s, 1, max(a.NumPaginas(), b.NumPaginas()))
		if err != nil {
			return nil, err
		}
		diff, err := CompararVisualPagina(a, b, n, opts)
		if err != nil {
			return nil, inputError(err.Error())
		}
		return json.Marshal(map[string]any{
			"modo": "visual", "paginas_a": a.NumPaginas(), "paginas_b": b.NumPaginas(),
			"pagina": diff,
		})
	}
	ctx, stop := context.WithTimeout(ctx, VisualCompareTimeout)
	defer stop()
	result, err := CompararVisual(ctx, a, b, opts)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"modo": "visual", "paginas_a": result.PaginasA, "paginas_b": result.PaginasB,
		"comparadas": result.Comparadas, "truncado": result.Truncado,
		"paginas": result.Paginas, "iguales": result.Iguales,
	})
}
