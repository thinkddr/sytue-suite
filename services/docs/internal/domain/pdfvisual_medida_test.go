// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sonda de COSTE de la comparación visual. No es una comprobación: es con lo que se
// midieron las cifras de `docs/paridad-pdf.md`, y queda aquí para volver a medirlas en
// vez de citarlas de memoria. Sin `SYTUE_PDF_MEDIR_VISUAL` no hace nada.
//
//	SYTUE_PDF_MEDIR_VISUAL=20 go test ./services/docs/internal/domain \
//	  -run TestMedirComparacionVisual -v -count=1
//
// El valor es el número de páginas. Cada una lleva 56 líneas de texto de 10 pt y una
// tabla de rectángulos; el segundo documento cambia una palabra por página.
package domain

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMedirComparacionVisual(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("SYTUE_PDF_MEDIR_VISUAL"))
	if n == 0 {
		t.Skip("sin SYTUE_PDF_MEDIR_VISUAL: no se mide nada")
	}
	pagina := func(palabra string) paginaVisual {
		var sb strings.Builder
		for l := 0; l < 56; l++ {
			sb.WriteString(texto(56, 790-float64(l)*13.5, 10,
				fmt.Sprintf("Linea %d del contrato: el plazo de %s dias se contara desde la firma y sera prorrogable.", l, palabra)))
		}
		for c := 0; c < 12; c++ {
			fmt.Fprintf(&sb, "0.85 g %d 40 40 18 re f\n", 56+c*44)
		}
		return a4(sb.String())
	}
	var pa, pb []paginaVisual
	for i := 0; i < n; i++ {
		pa = append(pa, pagina("treinta"))
		pb = append(pb, pagina("sesenta"))
	}
	a, b := abrir(t, pdfVisual(t, pa...)), abrir(t, pdfVisual(t, pb...))

	var antes, despues runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&antes)
	var ru0, ru1 syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru0)
	ini := time.Now()
	r, err := CompararVisual(context.Background(), a, b, OpcionesVisual{})
	pared := time.Since(ini)
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru1)
	runtime.ReadMemStats(&despues)
	if err != nil {
		t.Fatal(err)
	}
	cpu := time.Duration(ru1.Utime.Nano()+ru1.Stime.Nano()) - time.Duration(ru0.Utime.Nano()+ru0.Stime.Nano())
	px := 0
	for _, p := range r.Paginas {
		px += p.PixelesDiferentes
	}
	t.Logf("%d páginas A4 a ×1,5: %v de reloj, %v de CPU (%.0f ms/página), %d MiB asignados, "+
		"pico de memoria del proceso (maxrss) %d MiB, heap en uso al terminar %d MiB; %d páginas con diferencias, %d px",
		n, pared.Round(time.Millisecond), cpu.Round(time.Millisecond), float64(pared.Milliseconds())/float64(n),
		(despues.TotalAlloc-antes.TotalAlloc)>>20, maxrssMiB(ru1), despues.HeapInuse>>20, len(r.Paginas), px)

	// Y una página con su resaltado, que es lo que pide la interfaz al mirar una.
	ini = time.Now()
	d, err := CompararVisualPagina(a, b, 1, OpcionesVisual{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("una página con resaltado: %v, PNG de %d KiB, %d regiones",
		time.Since(ini).Round(time.Millisecond), len(d.Imagen)>>10, len(d.Regiones))
}

// maxrssMiB: en Linux `Maxrss` son KiB.
func maxrssMiB(ru syscall.Rusage) int64 { return ru.Maxrss >> 10 }
