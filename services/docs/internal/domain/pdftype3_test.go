// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de las fuentes de tipo 3. El fallo que cubren no es un error: es una
// página que sale EN BLANCO con el texto perfectamente colocado y sin dibujar,
// que es lo que pasaba antes de ejecutar los flujos de /CharProcs.
package domain

import (
	"fmt"
	"strings"
	"testing"
)

// pdfConType3 arma un PDF cuyo único texto usa una fuente de tipo 3 con un solo
// glifo, «cuadro», que pinta un cuadrado de 700×700 unidades de glifo.
func pdfConType3(t *testing.T, contenido, matrizFuente, extraFuente string) []byte {
	t.Helper()
	glifo := "1000 0 d0 0 0 700 700 re f"
	return pdfConContenido(t, contenido,
		`/Resources << /Font << /F3 5 0 R >> >>`,
		`<< /Type /Font /Subtype /Type3 /FontBBox [0 0 750 750] `+
			`/FontMatrix `+matrizFuente+` /CharProcs 6 0 R `+
			`/Encoding << /Differences [65 /cuadro] >> `+
			`/FirstChar 65 /LastChar 65 /Widths [1000] `+extraFuente+` >>`,
		`<< /cuadro 7 0 R >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(glifo), glifo))
}

// TestType3DibujaSusGlifos es el check central: antes se reservaba el avance y no
// se pintaba nada, así que el SVG salía sin un solo `<path>`.
func TestType3DibujaSusGlifos(t *testing.T) {
	svg := renderizar(t, pdfConType3(t,
		"BT /F3 20 Tf 10 40 Td (A) Tj ET",
		"[0.001 0 0 0.001 0 0]", ""))
	if !strings.Contains(svg, "<path") {
		t.Fatalf("el glifo de tipo 3 no se dibujó: %s", primerasLineas(svg))
	}
	// El cuadrado del glifo mide 700 unidades sobre una matriz de 0.001 y un cuerpo
	// de 20 puntos: 700 × 0.001 × 20 = 14 puntos de lado.
	if !strings.Contains(svg, "h700v700h-700Z") {
		t.Errorf("el camino del glifo no es el esperado: %s", primerasLineas(svg))
	}
}

func TestType3DifferencesRechazaIndicesFueraDeRango(t *testing.T) {
	for _, inicio := range []string{"4294967361", "4294967361.0"} {
		t.Run(inicio, func(t *testing.T) {
			datos := pdfConContenido(t,
				"BT /F3 20 Tf 10 40 Td (A) Tj ET",
				`/Resources << /Font << /F3 5 0 R >> >>`,
				`<< /Type /Font /Subtype /Type3 /FontBBox [0 0 750 750] `+
					`/FontMatrix [0.001 0 0 0.001 0 0] /CharProcs 6 0 R `+
					`/Encoding << /Differences [`+inicio+` /cuadro] >> `+
					`/FirstChar 65 /LastChar 65 /Widths [1000] >>`,
				`<< /cuadro 7 0 R >>`,
				fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream",
					len("1000 0 d0 0 0 700 700 re f"), "1000 0 d0 0 0 700 700 re f"))
			svg := renderizar(t, datos)
			if strings.Contains(svg, "h700v700h-700Z") {
				t.Fatalf("un índice fuera de rango se interpretó como el código 65: %s", primerasLineas(svg))
			}
		})
	}
}

// TestType3AplicaSuFontMatrix comprueba que la matriz del documento MANDA. Es lo
// que separa un glifo del tamaño correcto de uno mil veces más grande o más
// pequeño: en tipo 3 el espacio del glifo no es 1/1000, es lo que diga el
// fichero.
func TestType3AplicaSuFontMatrix(t *testing.T) {
	// Con una matriz diez veces mayor, el mismo glifo tiene que salir diez veces
	// más grande: la escala aparece en la transformación, no en el camino.
	normal := renderizar(t, pdfConType3(t,
		"BT /F3 20 Tf 10 40 Td (A) Tj ET", "[0.001 0 0 0.001 0 0]", ""))
	grande := renderizar(t, pdfConType3(t,
		"BT /F3 20 Tf 10 40 Td (A) Tj ET", "[0.01 0 0 0.01 0 0]", ""))
	if normal == grande {
		t.Fatal("cambiar /FontMatrix no cambió nada: no se está aplicando")
	}
	if !strings.Contains(normal, "matrix(0.02 0 0 -0.02 10 60)") {
		t.Errorf("matriz con 0.001 inesperada: %s", primerasLineas(normal))
	}
	if !strings.Contains(grande, "matrix(0.2 0 0 -0.2 10 60)") {
		t.Errorf("matriz con 0.01 inesperada: %s", primerasLineas(grande))
	}
}

// TestType3AvanzaAunqueFalteElDibujo: un código sin entrada en /CharProcs deja un
// hueco, NO descoloca el resto de la línea.
func TestType3AvanzaAunqueFalteElDibujo(t *testing.T) {
	// «B» tiene ANCHURA declarada pero no está en /Differences, así que no tiene
	// dibujo. Es el caso real: la fuente sabe cuánto ocupa y el glifo falta.
	svg := renderizar(t, pdfConContenido(t,
		"BT /F3 20 Tf 10 40 Td (BA) Tj ET",
		`/Resources << /Font << /F3 5 0 R >> >>`,
		`<< /Type /Font /Subtype /Type3 /FontBBox [0 0 750 750] `+
			`/FontMatrix [0.001 0 0 0.001 0 0] /CharProcs 6 0 R `+
			`/Encoding << /Differences [65 /cuadro] >> `+
			`/FirstChar 65 /LastChar 66 /Widths [1000 1000] >>`,
		`<< /cuadro 7 0 R >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream",
			len("1000 0 d0 0 0 700 700 re f"), "1000 0 d0 0 0 700 700 re f")))
	if !strings.Contains(svg, "<path") {
		t.Fatalf("la «A» debía dibujarse aunque la «B» no: %s", primerasLineas(svg))
	}
	// La «B» mide 1000 milésimas y a 20 puntos son 20 de avance: la «A» arranca
	// en x=30. Con el avance perdido caería sobre la «B», en x=10.
	if !strings.Contains(svg, "matrix(0.02 0 0 -0.02 30 60)") {
		t.Errorf("la «A» no avanzó lo que ocupa la «B»: %s", primerasLineas(svg))
	}
}

// TestType3InvisibleNoDibuja: el modo de render 3 es la capa de OCR, y ejecutar
// los flujos para no pintar nada sería trabajo tirado.
func TestType3InvisibleNoDibuja(t *testing.T) {
	svg := renderizar(t, pdfConType3(t,
		"BT 3 Tr /F3 20 Tf 10 40 Td (A) Tj ET", "[0.001 0 0 0.001 0 0]", ""))
	if strings.Contains(svg, "h700v700h-700Z") {
		t.Errorf("el modo 3 no debe dibujar el glifo: %s", primerasLineas(svg))
	}
}

// TestType3SinCharProcsNoRompe: un documento al que le falta la mitad de la
// fuente tiene que seguir abriéndose.
func TestType3SinCharProcsNoRompe(t *testing.T) {
	datos := pdfConContenido(t,
		"BT /F3 20 Tf 10 40 Td (A) Tj ET",
		`/Resources << /Font << /F3 5 0 R >> >>`,
		`<< /Type /Font /Subtype /Type3 /FontMatrix [0.001 0 0 0.001 0 0] `+
			`/Encoding << /Differences [65 /cuadro] >> /FirstChar 65 /LastChar 65 /Widths [1000] >>`)
	svg := renderizar(t, datos)
	if !strings.HasSuffix(svg, "</svg>") {
		t.Errorf("una fuente de tipo 3 sin /CharProcs no debe romper el render")
	}
}

// TestType3NoSeRecursaSinFin: un glifo que vuelve a usar SU PROPIA fuente sería
// una recursión infinita, y un fichero manipulado puede pedirlo.
func TestType3NoSeRecursaSinFin(t *testing.T) {
	glifo := "1000 0 d0 BT /F3 10 Tf (A) Tj ET"
	datos := pdfConContenido(t,
		"BT /F3 20 Tf 10 40 Td (A) Tj ET",
		`/Resources << /Font << /F3 5 0 R >> >>`,
		`<< /Type /Font /Subtype /Type3 /FontMatrix [0.001 0 0 0.001 0 0] `+
			`/CharProcs 6 0 R /Encoding << /Differences [65 /cuadro] >> `+
			`/FirstChar 65 /LastChar 65 /Widths [1000] `+
			`/Resources << /Font << /F3 5 0 R >> >> >>`,
		`<< /cuadro 7 0 R >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(glifo), glifo))

	hecho := make(chan string, 1)
	go func() {
		d, err := LeerPDF(datos)
		if err != nil {
			hecho <- ""
			return
		}
		svg, _ := d.RenderizarPaginaSVG(0, OpcionesRender{})
		hecho <- svg
	}()
	select {
	case svg := <-hecho:
		if svg != "" && !strings.HasSuffix(svg, "</svg>") {
			t.Error("el SVG salió mal cerrado")
		}
	case <-timeoutCorto():
		t.Fatal("un glifo de tipo 3 que se usa a sí mismo colgó el render")
	}
}
