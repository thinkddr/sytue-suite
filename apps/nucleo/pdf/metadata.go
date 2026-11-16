// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"strings"
	"time"
)

// MaxIndexedPages bounds text extraction while retaining the complete page list.
const MaxIndexedPages = 500

// PageIndex is the shared page description used by device and HTTP PDF views.
type PageIndex struct {
	// Numero es 1-based, como lo ve el usuario en su visor.
	Numero int `json:"numero"`
	// Ancho y Alto en puntos, ya con la rotación aplicada.
	Ancho float64 `json:"ancho"`
	Alto  float64 `json:"alto"`
	// Rotacion en grados, tal cual la declara el documento.
	Rotacion int `json:"rotacion"`
	// Caja es el MediaBox ordenado [x0 y0 x1 y1]. Lo necesita el editor para
	// pasar de las coordenadas del ratón a las del PDF: con un origen que no sea
	// (0,0) —que existe— un resaltado caería desplazado por el margen.
	Caja [4]float64 `json:"caja"`
	// Extracto son las primeras palabras de la página, para reconocerla. Vacío
	// en una página que solo tiene imágenes, que también es información útil.
	// Ausente (nil) en la que no se llegó a leer: `/pdf/content` corta por tiempo
	// (presupuestoExtractos), y una página sin leer NO es una página sin texto.
	Extracto *string `json:"extracto,omitempty"`
}

// IndexPages builds page metadata with a bounded text-extraction budget.
func IndexPages(doc *PDFDoc, hasta int, presupuesto time.Duration) []PageIndex {
	n := min(doc.NumPaginas(), hasta)
	out := make([]PageIndex, 0, n)
	inicio := time.Now()
	for i := 0; i < n; i++ {
		p, err := doc.Pagina(i)
		if err != nil {
			continue
		}
		pi := PageIndex{
			Numero:   i + 1,
			Ancho:    p.Ancho,
			Alto:     p.Alto,
			Rotacion: p.Rotacion,
			Caja:     p.Caja,
		}
		// La primera SIEMPRE: es la que dice si el documento es un escaneo sin texto.
		if i < MaxIndexedPages && (presupuesto == 0 || i == 0 || time.Since(inicio) < presupuesto) {
			texto, err := doc.TextoPlano(i)
			if err != nil {
				// Una página ilegible no invalida el índice: se devuelve con el
				// extracto vacío, que sigue permitiendo elegirla por número y tamaño.
				texto = ""
			}
			e := Excerpt(texto, 120)
			pi.Extracto = &e
		}
		out = append(out, pi)
	}
	return out
}

// Excerpt collapses whitespace and truncates on a word boundary.
func Excerpt(texto string, maxRunas int) string {
	// strings.Fields ya parte por unicode.IsSpace y colapsa los espacios seguidos:
	// es exactamente lo que hacía el FieldsFunc con su lambda.
	limpio := strings.Join(strings.Fields(texto), " ")
	if limpio == "" {
		return ""
	}
	runas := []rune(limpio)
	if len(runas) <= maxRunas {
		return limpio
	}
	corte := string(runas[:maxRunas])
	if i := strings.LastIndexByte(corte, ' '); i > maxRunas/2 {
		corte = corte[:i]
	}
	return corte + "…"
}

// Summary describes a PDF for the shared studio without persisting its contents.
func Summary(doc *PDFDoc, budget time.Duration) map[string]any {
	return map[string]any{
		"paginas": IndexPages(doc, doc.NumPaginas(), budget), "total": doc.NumPaginas(),
		"indice": doc.IndiceDelDocumento(), "enlaces": doc.EnlacesDelDocumento(),
		"adjuntos": doc.AdjuntosDelDocumento(), "anotaciones": doc.AnotacionesDelDocumento(),
		"campos": doc.CamposDelFormulario(), "firmas": doc.FirmasDelDocumento(),
		"cifrado": doc.EstaCifrado(), "permisos": doc.PermisosDelDocumento(), "huella": doc.Huella(),
	}
}

// ArchivalChange reports when an edit removes or lowers the declared PDF/A level.
func ArchivalChange(before *PDFDoc, after []byte) string {
	if before == nil {
		return ""
	}
	previous := before.DeclaraPDFA()
	if previous == "" {
		return ""
	}
	next := ""
	if doc, err := LeerPDF(after); err == nil {
		next = doc.DeclaraPDFA()
	}
	switch {
	case next == "":
		return "perdido"
	case strings.HasSuffix(previous, "a") && strings.HasSuffix(next, "b"):
		return "rebajado"
	default:
		return ""
	}
}
