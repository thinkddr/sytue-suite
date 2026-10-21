// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del envoltorio de Tesseract. Se prueban las DOS piezas que son
// nuestras y pueden fallar en silencio: interpretar el TSV que devuelve el motor
// y decidir qué idiomas se aceptan. El motor en sí no se prueba aquí —es
// Apache-2.0 y tiene los suyos—, y depender de que esté instalado convertiría
// estos tests en algo que solo pasa dentro del contenedor.
package tesseract

import (
	"errors"
	"strings"
	"testing"
)

// tsvDePrueba imita lo que emite `tesseract … tsv`: una cabecera y una fila por
// nivel, donde solo el nivel 5 es una palabra.
const tsvDePrueba = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"1\t1\t0\t0\t0\t0\t0\t0\t2480\t3508\t-1\t\n" +
	"2\t1\t1\t0\t0\t0\t100\t200\t900\t40\t-1\t\n" +
	"5\t1\t1\t1\t1\t1\t100\t200\t180\t38\t96.4\tFACTURA\n" +
	"5\t1\t1\t1\t1\t2\t300\t200\t120\t38\t95.1\t2026\n" +
	"5\t1\t1\t1\t2\t1\t100\t260\t210\t38\t88.7\tImporte\n" +
	"5\t1\t2\t1\t1\t1\t100\t900\t60\t30\t12.3\tmancha\n" +
	"5\t1\t1\t1\t1\t3\t500\t200\t100\t38\t99.0\t   \n"

func TestPalabrasDeTSVSoloTomaPalabras(t *testing.T) {
	palabras := palabrasDeTSV(tsvDePrueba)
	if len(palabras) != 3 {
		t.Fatalf("palabras = %d, quería 3: %+v", len(palabras), palabras)
	}
	if palabras[0].Texto != "FACTURA" || palabras[0].X != 100 || palabras[0].Ancho != 180 {
		t.Errorf("primera palabra = %+v", palabras[0])
	}
}

// TestPalabrasDeTSVDescartaLaBasuraDelEscaneo cubre lo que ensucia una búsqueda:
// las manchas del papel que el motor interpreta como letras y admite que no se
// las cree. Meterlas en la capa de texto hace que el documento devuelva
// resultados que no existen.
func TestPalabrasDeTSVDescartaLaBasuraDelEscaneo(t *testing.T) {
	for _, p := range palabrasDeTSV(tsvDePrueba) {
		if p.Texto == "mancha" {
			t.Errorf("se coló una palabra con confianza %.1f (mínimo %.0f)", p.Confianza, confianzaMinima)
		}
		if strings.TrimSpace(p.Texto) == "" {
			t.Error("se coló una palabra vacía")
		}
	}
}

// TestPalabrasDeTSVAgrupaPorRenglon comprueba que dos palabras de la misma línea
// comparten número y una de otra línea no. De ahí sale reconstruir párrafos.
func TestPalabrasDeTSVAgrupaPorRenglon(t *testing.T) {
	p := palabrasDeTSV(tsvDePrueba)
	if p[0].Linea != p[1].Linea {
		t.Errorf("«%s» y «%s» están en la misma línea y salieron en %d y %d",
			p[0].Texto, p[1].Texto, p[0].Linea, p[1].Linea)
	}
	if p[2].Linea == p[0].Linea {
		t.Errorf("«%s» está en otra línea y salió en la misma (%d)", p[2].Texto, p[2].Linea)
	}
}

func TestPalabrasDeTSVToleraBasura(t *testing.T) {
	for _, entrada := range []string{
		"", "cabecera sola\n", "5\t1\n", "5\t1\t1\t1\t1\t1\tx\ty\tz\tw\tq\ttexto\n",
		"5\t1\t1\t1\t1\t1\t10\t10\t0\t0\t99\tancho cero\n",
	} {
		if p := palabrasDeTSV(entrada); len(p) != 0 {
			t.Errorf("con %q salieron palabras: %+v", entrada, p)
		}
	}
}

// --- Idiomas -----------------------------------------------------------------

func motorDePrueba(instalados ...string) *Motor {
	m := &Motor{instalados: map[string]bool{}}
	for _, l := range instalados {
		m.instalados[l] = true
	}
	return m
}

// TestIdiomaDesconocidoSeRechaza es el check que evita el peor resultado
// posible: reconocer un documento en español con el modelo inglés no da error,
// da un texto plausible y mal — y nadie lo revisa porque parece que funcionó.
func TestIdiomaDesconocidoSeRechaza(t *testing.T) {
	m := motorDePrueba("spa", "eng")
	if _, err := m.normalizarIdiomas("cat"); !errors.Is(err, ErrIdiomaDesconocido) {
		t.Errorf("un idioma no instalado debe rechazarse, dio: %v", err)
	}
	if _, err := m.normalizarIdiomas("spa+rus"); !errors.Is(err, ErrIdiomaDesconocido) {
		t.Error("basta con que UNO no esté para rechazar la lista")
	}
}

func TestIdiomasPorDefecto(t *testing.T) {
	if got, _ := motorDePrueba("spa", "eng", "fra").normalizarIdiomas(""); got != "spa+eng" {
		t.Errorf("por defecto = %q, quería «spa+eng»", got)
	}
	if got, _ := motorDePrueba("deu").normalizarIdiomas(""); got != "deu" {
		t.Errorf("con un solo idioma instalado debería usarlo: %q", got)
	}
}

func TestIdiomasValidosSeAceptan(t *testing.T) {
	m := motorDePrueba("spa", "eng", "fra", "deu", "ita")
	got, err := m.normalizarIdiomas(" SPA + eng ")
	if err != nil {
		t.Fatalf("normalizarIdiomas: %v", err)
	}
	if got != "spa+eng" {
		t.Errorf("= %q, quería «spa+eng» (normalizado a minúsculas y sin espacios)", got)
	}
}

// TestIdiomaConCaracteresRarosSeRechaza cubre la frontera de confianza: el valor
// acaba en la línea de comandos de otro proceso, y aunque no pase por un shell,
// algo como «--tessdata-dir» ahí sería una opción más de Tesseract.
func TestIdiomaConCaracteresRarosSeRechaza(t *testing.T) {
	m := motorDePrueba("spa")
	for _, malo := range []string{"../etc", "spa;rm", "-l", "spa spa", "sp4"} {
		if _, err := m.normalizarIdiomas(malo); err == nil {
			t.Errorf("%q debería rechazarse", malo)
		}
	}
}

func TestDemasiadosIdiomasSeRechaza(t *testing.T) {
	m := motorDePrueba("spa", "eng", "fra", "deu", "ita")
	if _, err := m.normalizarIdiomas("spa+eng+fra+deu+ita"); err == nil {
		t.Error("con cinco idiomas a la vez el reconocimiento empeora y tarda: debe rechazarse")
	}
}
