// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sonda de COSTE y FIDELIDAD de la salida PNG. No es una comprobación: es la
// herramienta con la que se midieron las cifras del ADR-0033 (addendum de la
// salida rasterizada), y queda aquí para volver a medirlas en vez de citarlas de
// memoria. Sin `SYTUE_PDF_MEDIR` no hace nada.
//
// Mide, por página y contra el SVG del MISMO documento: tiempo, bytes reservados
// (`TotalAlloc`, que es lo que el recolector tendrá que barrer) y peso de la
// respuesta. Con `SYTUE_PDF_MEDIR_REF` compara además píxel a píxel con la
// referencia de poppler (`pdftoppm -r <72×escala> -png`), que no se usa en el
// servicio —es GPL— pero sirve de vara de medir.
//
//	SYTUE_PDF_MEDIR=a.pdf,b.pdf SYTUE_PDF_MEDIR_REF=/ref SYTUE_PDF_MEDIR_OUT=/out \
//	  go test ./services/docs/internal/domain -run TestMedirSalidaPNG -v
package domain

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMedirSalidaPNG(t *testing.T) {
	lista := os.Getenv("SYTUE_PDF_MEDIR")
	if lista == "" {
		t.Skip("sin SYTUE_PDF_MEDIR: no se mide nada")
	}
	escala, _ := strconv.ParseFloat(os.Getenv("SYTUE_PDF_MEDIR_ESCALA"), 64)
	if escala == 0 {
		escala = 2
	}
	paginas, _ := strconv.Atoi(os.Getenv("SYTUE_PDF_MEDIR_PAGINAS"))
	if paginas == 0 {
		paginas = 3
	}
	for _, f := range strings.Split(lista, ",") {
		datos, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		d, err := LeerPDF(datos)
		if err != nil {
			t.Logf("%s: %v", f, err)
			continue
		}
		base := strings.TrimSuffix(filepath.Base(f), ".pdf")
		for i := 0; i < min(paginas, d.NumPaginas()); i++ {
			t.Log(medirPagina(t, d, base, i, escala))
		}
	}
}

// medirPagina dibuja la página en las dos salidas y devuelve la línea del informe.
// Los tiempos son la MEDIANA de cinco vueltas: en una máquina compartida una
// vuelta suelta puede triplicarse por lo que haga el vecino.
func medirPagina(t *testing.T, d *PDFDoc, base string, i int, escala float64) string {
	t.Helper()
	opts := OpcionesRender{Escala: escala}
	// Calentamiento: el primer recorrido resuelve y guarda objetos del documento,
	// y no es justo cargárselo a la salida que vaya primero.
	_, _ = d.RenderizarPaginaSVG(i, opts)

	var pngB []byte
	var svg string
	aPNG := asignado(func() { pngB, _ = d.RenderizarPaginaPNG(i, opts) })
	aSVG := asignado(func() { svg, _ = d.RenderizarPaginaSVG(i, opts) })
	img, err := png.Decode(bytes.NewReader(pngB))
	if err != nil {
		return fmt.Sprintf("%s p%d: sin PNG", base, i+1)
	}
	// Dibujar y codificar por separado: son dos costes con remedios distintos.
	var r *rasterizador
	dDibujo := mediana(func() {
		p, _ := d.Pagina(i)
		r, _ = nuevoRasterizador(p.Ancho, p.Alto, opts.escala())
		_, _, _ = d.recorrerPagina(i, opts, r)
	})
	var rapido, defecto []byte
	dCodif := mediana(func() { defecto, _ = bytesPNG(r.dst) })
	dRapido := mediana(func() {
		var b bytes.Buffer
		_ = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, r.dst)
		rapido = b.Bytes()
	})
	dSVG := mediana(func() { svg, _ = d.RenderizarPaginaSVG(i, opts) })

	linea := fmt.Sprintf("%-22.22s p%d %4dx%-4d CPU: dibujo %5.0f ms codif-defecto %4.0f ms (%4.0f KiB) codif-rápida %4.0f ms (%4.0f KiB) asig %6.1f MiB"+
		" | SVG %5.0f ms asig %6.1f MiB %5.0f KiB | tinta %d",
		base, i+1, img.Bounds().Dx(), img.Bounds().Dy(),
		ms(dDibujo), ms(dCodif), float64(len(defecto))/1024, ms(dRapido), float64(len(rapido))/1024, mib(aPNG),
		ms(dSVG), mib(aSVG), float64(len(svg))/1024, tinta(img))
	if ref := referenciaPoppler(os.Getenv("SYTUE_PDF_MEDIR_REF"), base, i+1); ref != nil {
		media, pct := diferencia(img, ref)
		linea += fmt.Sprintf(" | poppler tinta %d dif-media %.2f dif>64 %.2f%%", tinta(ref), media, pct)
	}
	if out := os.Getenv("SYTUE_PDF_MEDIR_OUT"); out != "" {
		_ = os.WriteFile(filepath.Join(out, fmt.Sprintf("%s-%d.png", base, i+1)), pngB, 0o600)
	}
	return linea
}

// mediana devuelve la mediana de cinco vueltas del tiempo de CPU del proceso
// (usuario + sistema, recolector incluido). Es la cifra que importa —cuánta
// máquina cuesta una página— y la que menos se deja engañar por una máquina
// compartida: el reloj de pared mide también lo que hace el vecino.
func mediana(f func()) time.Duration {
	var v [5]time.Duration
	for k := range v {
		t0 := cpu()
		f()
		v[k] = cpu() - t0
	}
	slices.Sort(v[:])
	return v[len(v)/2]
}

func cpu() time.Duration {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// asignado son los bytes que reserva f: lo que el recolector tendrá que barrer.
func asignado(f func()) uint64 {
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
func mib(b uint64) float64       { return float64(b) / (1 << 20) }

// referenciaPoppler busca `<base>-<n>.png`; pdftoppm rellena con ceros el número
// según el total de páginas, así que se compara el número y no el nombre.
func referenciaPoppler(dir, base string, n int) image.Image {
	if dir == "" {
		return nil
	}
	cands, _ := filepath.Glob(filepath.Join(dir, base+"-*.png"))
	for _, c := range cands {
		sfx := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(c), base+"-"), ".png")
		if k, err := strconv.Atoi(sfx); err != nil || k != n {
			continue
		}
		fh, err := os.Open(c)
		if err != nil {
			return nil
		}
		img, err := png.Decode(fh)
		_ = fh.Close()
		if err != nil {
			return nil
		}
		return img
	}
	return nil
}

// tinta cuenta los píxeles oscuros: es lo que distingue una página con texto de
// un papel con rayas.
func tinta(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if (r+g+bl)/3 < 0x8000 {
				n++
			}
		}
	}
	return n
}

// diferencia es la diferencia media por canal (0-255) y el % de píxeles que se
// apartan más de 64 niveles en algún canal, que es lo que se ve a simple vista.
func diferencia(a, b image.Image) (float64, float64) {
	ba, bb := a.Bounds(), b.Bounds()
	w, h := min(ba.Dx(), bb.Dx()), min(ba.Dy(), bb.Dy())
	if w == 0 || h == 0 {
		return 0, 0
	}
	abs := func(p, q uint32) float64 {
		v := float64(p>>8) - float64(q>>8)
		if v < 0 {
			return -v
		}
		return v
	}
	var suma float64
	malos := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1, _ := a.At(ba.Min.X+x, ba.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			dr, dg, db := abs(r1, r2), abs(g1, g2), abs(b1, b2)
			suma += (dr + dg + db) / 3
			if dr > 64 || dg > 64 || db > 64 {
				malos++
			}
		}
	}
	return suma / float64(w*h), 100 * float64(malos) / float64(w*h)
}
