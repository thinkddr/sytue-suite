// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdfdoc

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const lorem = "La accesibilidad no se añade al final: se escribe con el documento. "

func ejemplo(t *testing.T) (*Doc, []byte) {
	t.Helper()
	d := Nuevo(Opciones{Titulo: "Prueba de fuentes", Idioma: "es"})
	d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: "Título de la prueba", Negrita: true}}, Tam: 20, Despues: 8})
	for i := 0; i < 40; i++ {
		d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: lorem}, {Texto: "Énfasis ", Cursiva: true}, {Texto: strings.Repeat("palabra ", 12)}}, Despues: 6})
	}
	d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: "Ελληνικά: Ωμέγα · Български: Жизнь · Ő ș ț ł"}}})
	b, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("SYTUE_PDF_VOLCAR"); dir != "" { // para mirarlo con pdftoppm
		_ = os.WriteFile(filepath.Join(dir, "pdfdoc-ejemplo.pdf"), b, 0o600)
	}
	return d, b
}

// Cada entrada de la xref tiene que apuntar a «N 0 obj», y el startxref a la tabla.
func TestXrefCuadra(t *testing.T) {
	_, b := ejemplo(t)
	sx := regexp.MustCompile(`startxref\n(\d+)\n%%EOF\n$`).FindSubmatch(b)
	if sx == nil {
		t.Fatal("sin startxref")
	}
	off, _ := strconv.Atoi(string(sx[1]))
	if !bytes.HasPrefix(b[off:], []byte("xref\n0 ")) {
		t.Fatalf("startxref no apunta a la tabla: %q", b[off:off+10])
	}
	filas := regexp.MustCompile(`(\d{10}) 0{5} n \n`).FindAllSubmatch(b[off:], -1)
	if len(filas) < 10 {
		t.Fatalf("pocas entradas de xref: %d", len(filas))
	}
	for i, f := range filas {
		o, _ := strconv.Atoi(string(f[1]))
		if !bytes.HasPrefix(b[o:], []byte(strconv.Itoa(i+1)+" 0 obj\n")) {
			t.Errorf("objeto %d: la xref apunta a %q", i+1, b[o:o+12])
		}
	}
}

func TestLasLineasNoDesbordanElMargen(t *testing.T) {
	d := Nuevo(Opciones{})
	texto := strings.Repeat("Anticonstitucionalmente ", 30) + strings.Repeat("W", 200) + " fin"
	for _, tam := range []float64{9, 11, 20} {
		ps := d.piezas([]Fragmento{{Texto: texto}, {Texto: texto, Negrita: true}}, tam, nil)
		ancho := PageW - 2*Margin
		ls := d.romper(ps, tam, ancho)
		if len(ls) < 5 {
			t.Fatalf("tam %v: solo %d líneas", tam, len(ls))
		}
		for i, l := range ls {
			if l.ancho > ancho+0.01 {
				t.Errorf("tam %v, línea %d: %.1f pt > %.1f", tam, i, l.ancho, ancho)
			}
			if n := len(l.piezas); n > 0 && l.piezas[n-1].espacio {
				t.Errorf("tam %v, línea %d acaba en espacio", tam, i)
			}
		}
	}
}

func TestSinGlifoNoSePintaNotdef(t *testing.T) {
	d := Nuevo(Opciones{})
	d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: "Hola 漢字 mundo " + string(rune(0xad)) + string(rune(0x200b))}}})
	if d.Avisos()['漢'] != 1 || d.Avisos()['字'] != 1 {
		t.Errorf("avisos = %v", d.Avisos())
	}
	if strings.Contains(d.cur.contenido.String(), "<0000") || strings.Contains(d.cur.contenido.String(), "0000>") {
		t.Error("se ha pintado el glifo 0 (.notdef)")
	}
}

func TestPaginaYNoSaleDelPapel(t *testing.T) {
	d, b := ejemplo(t)
	if d.Paginas() < 2 {
		t.Fatalf("40 párrafos deberían ocupar más de una página, hay %d", d.Paginas())
	}
	if n := bytes.Count(b, []byte("/Type /Page ")); n != d.Paginas() {
		t.Errorf("páginas en el fichero = %d, escritas = %d", n, d.Paginas())
	}
}

// Con las herramientas de poppler (si están): las fuentes van incrustadas como Type 0
// Identity-H, y el texto se recupera igual —incluidos griego y cirílico— por el /ToUnicode.
func TestPopplerLeeLasFuentesYElTexto(t *testing.T) {
	for _, bin := range []string{"pdffonts", "pdftotext"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip(bin + " no está instalado")
		}
	}
	_, b := ejemplo(t)
	f := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(f, b, 0o600); err != nil {
		t.Fatal(err)
	}
	fonts, err := exec.Command("pdffonts", f).CombinedOutput()
	if err != nil {
		t.Fatalf("pdffonts: %v\n%s", err, fonts)
	}
	filas := strings.Split(strings.TrimSpace(string(fonts)), "\n")[2:]
	if len(filas) < 3 {
		t.Fatalf("esperaba al menos 3 fuentes (Figtree normal, negrita y cursiva, reserva), hay:\n%s", fonts)
	}
	for _, fila := range filas {
		if !strings.Contains(fila, "CID TrueType") || !strings.Contains(fila, "Identity-H") || !regexp.MustCompile(`\byes +yes +yes\b`).MatchString(fila) {
			t.Errorf("fuente no incrustada como subconjunto con ToUnicode: %s", fila)
		}
	}
	txt, err := exec.Command("pdftotext", f, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v\n%s", err, txt)
	}
	for _, quiero := range []string{"Título de la prueba", "se escribe con el documento", "Ελληνικά: Ωμέγα", "Български: Жизнь", "Ő ș ț ł"} {
		if !strings.Contains(string(txt), quiero) {
			t.Errorf("el texto extraído no contiene %q", quiero)
		}
	}
}

// completo escribe un documento con todo lo que pieza 2 etiqueta: encabezados (con un
// salto de nivel), párrafos con enlaces, tramos de otro idioma, listas anidadas, cita y regla.
func completo(titulo string) *Doc {
	d := Nuevo(Opciones{Titulo: titulo, Idioma: "es"})
	d.Parrafo(Parrafo{Rol: "H1", Fragmentos: []Fragmento{{Texto: titulo, Negrita: true}}, Tam: 22, Despues: 8})
	d.Parrafo(Parrafo{Fragmentos: []Fragmento{
		{Texto: "Consulta la "},
		{Texto: "guía de accesibilidad", Enlace: "https://sytue.eu/accesibilidad?a=1&b=2"},
		{Texto: " o escribe a "},
		{Texto: "hola@sytue.eu", Enlace: "mailto:hola@sytue.eu", Negrita: true},
		{Texto: ". En griego se dice "},
		{Texto: "προσβασιμότητα"},
		{Texto: " y en francés "},
		{Texto: "accessibilité", Idioma: "fr", Cursiva: true},
		{Texto: "."},
	}, Despues: 6})
	d.Parrafo(Parrafo{Rol: "H4", Fragmentos: []Fragmento{{Texto: "Un subtítulo que salta de H1 a H4"}}, Tam: 13, Despues: 4})
	d.ItemLista(0, false, Parrafo{Fragmentos: []Fragmento{{Texto: "Primer elemento"}}})
	d.ItemLista(1, true, Parrafo{Fragmentos: []Fragmento{{Texto: "Subelemento uno"}}})
	d.ItemLista(1, true, Parrafo{Fragmentos: []Fragmento{{Texto: "Subelemento dos, con un texto algo más largo para que dé la vuelta a la línea y compruebe el sangrado colgante del marcador " + lorem}}})
	d.ItemLista(0, false, Parrafo{Fragmentos: []Fragmento{{Texto: "Segundo elemento"}}})
	d.ItemLista(3, false, Parrafo{Fragmentos: []Fragmento{{Texto: "Un nivel que salta de golpe"}}})
	d.Regla()
	d.Parrafo(Parrafo{Cita: true, Fragmentos: []Fragmento{{Texto: "Lo que se mide, mejora. " + lorem}}})
	for i := 0; i < 30; i++ {
		d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: lorem + lorem + lorem, Subrayado: i%7 == 0, Tachado: i%11 == 3}}, Despues: 6})
	}
	d.Parrafo(Parrafo{Rol: "H2", Fragmentos: []Fragmento{{Texto: "Segunda sección"}}, Tam: 16})
	d.Parrafo(Parrafo{Fragmentos: []Fragmento{{Texto: "Final."}}})
	return d
}

func TestVolcarCompleto(t *testing.T) {
	dir := os.Getenv("SYTUE_PDF_VOLCAR")
	if dir == "" {
		t.Skip("solo con SYTUE_PDF_VOLCAR")
	}
	b, err := completo("Documento accesible de prueba").Bytes()
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "pdfdoc-completo.pdf"), b, 0o600)
}
