// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"math"
	"testing"
)

// TestLasCoordenadasVanYVuelven es el guardián que la web NO tiene y que más falta
// hace: [ASVG] tiene que ser la inversa EXACTA de [APDF] en las cuatro rotaciones.
//
// Convertir mal no se ve en un test de tipos ni en una captura: se ve cuando alguien
// resalta un párrafo y la marca aparece media página más arriba, o cuando censura un
// nombre y el rectángulo negro cae sobre otro. Como las dos funciones se usan en
// sentidos opuestos —el puntero entra por APDF, las anotaciones ya guardadas salen por
// ASVG—, un signo cambiado en UNA de las dos deja la otra correcta y el fallo solo
// aparece con /Rotate distinto de 0, que es el caso raro que nadie prueba a mano.
func TestLasCoordenadasVanYVuelven(t *testing.T) {
	t.Parallel()
	// Caja que NO empieza en el origen: un MediaBox con x0/y0 a cero esconde
	// justamente los errores de desplazamiento, que son la mitad de los posibles.
	caja := Rect{10, 20, 605, 862}
	puntos := [][2]float64{{0, 0}, {1, 1}, {123.5, 456.25}, {595, 842}}

	for _, rot := range []int{0, 90, 180, 270} {
		p := Pagina{Numero: 1, Ancho: 595, Alto: 842, Rotacion: rot, Caja: caja}
		for _, q := range puntos {
			px, py := APDF(p, q[0], q[1])
			sx, sy := ASVG(p, px, py)
			if math.Abs(sx-q[0]) > 1e-9 || math.Abs(sy-q[1]) > 1e-9 {
				t.Errorf("rotación %d: (%g,%g) → PDF(%g,%g) → SVG(%g,%g); esperaba volver al punto de partida",
					rot, q[0], q[1], px, py, sx, sy)
			}
		}
	}
}

// TestRectEnSVGOrdenaLasEsquinas comprueba que la caja sale con ancho y alto POSITIVOS
// aunque la rotación invierta un eje.
//
// Sin ordenar, a 180° un rectángulo perfectamente válido salía con ancho negativo, y un
// `canvas.Rectangle` con tamaño negativo en Fyne no se queja: no pinta nada. Una
// anotación que existe y no se ve es peor que una que no existe.
func TestRectEnSVGOrdenaLasEsquinas(t *testing.T) {
	t.Parallel()
	caja := Rect{0, 0, 595, 842}
	r := Rect{100, 200, 300, 500} // x0<x1, y0<y1, como los manda el motor

	for _, rot := range []int{0, 90, 180, 270} {
		p := Pagina{Numero: 1, Ancho: 595, Alto: 842, Rotacion: rot, Caja: caja}
		c := RectEnSVG(p, r)
		if c.Ancho <= 0 || c.Alto <= 0 {
			t.Errorf("rotación %d: caja %+v tiene lado no positivo", rot, c)
		}
		// El área se conserva: rotar no encoge nada.
		if area := c.Ancho * c.Alto; math.Abs(area-200*300) > 1e-9 {
			t.Errorf("rotación %d: área %g, esperaba 60000", rot, area)
		}
	}
}

// TestTramosDelIndice son los cuatro casos de `tramos.test.ts`, traídos enteros. El
// oráculo es la web: si los dos no parten el documento igual, el mismo botón da
// ficheros distintos según desde dónde se pulse.
func TestTramosDelIndice(t *testing.T) {
	t.Parallel()
	m := func(titulo string, pagina, nivel int) Marcador {
		return Marcador{Titulo: titulo, Pagina: pagina, Nivel: nivel}
	}

	casos := []struct {
		nombre string
		indice []Marcador
		total  int
		quiero []Tramo
	}{
		{
			nombre: "cada capítulo llega hasta el siguiente, y el último hasta el final",
			indice: []Marcador{m("Uno", 0, 0), m("Dos", 4, 0), m("Tres", 9, 0)},
			total:  20,
			// Sin el último caso, «Tres» se quedaría en su propia página y las once
			// restantes no saldrían en ninguna parte.
			quiero: []Tramo{{"Uno", "1-4"}, {"Dos", "5-9"}, {"Tres", "10-20"}},
		},
		{
			nombre: "los subcapítulos no abren tramo y los que no navegan tampoco",
			indice: []Marcador{m("Uno", 0, 0), m("Uno.a", 1, 1), m("sin destino", -1, 0), m("Dos", 5, 0)},
			total:  8,
			quiero: []Tramo{{"Uno", "1-5"}, {"Dos", "6-8"}},
		},
		{
			nombre: "un capítulo de una sola página se escribe sin rango",
			indice: []Marcador{m("A", 0, 0), m("B", 2, 0), m("C", 3, 0)},
			total:  4,
			// «3-3» es válido pero se lee como si faltara algo; el campo acepta «3».
			quiero: []Tramo{{"A", "1-2"}, {"B", "3"}, {"C", "4"}},
		},
		{
			nombre: "sin índice no hay nada que partir",
			indice: nil,
			total:  10,
			quiero: []Tramo{},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			hay := TramosDelIndice(c.indice, c.total)
			if len(hay) != len(c.quiero) {
				t.Fatalf("%d tramos, esperaba %d: %+v", len(hay), len(c.quiero), hay)
			}
			for i := range hay {
				if hay[i] != c.quiero[i] {
					t.Errorf("tramo %d: %+v, esperaba %+v", i, hay[i], c.quiero[i])
				}
			}
		})
	}
}

// TestTextoParaLeerAgrupaPorLineas comprueba lo único con miga de la función: que dos
// trozos de la MISMA línea se peguen con un espacio y no con un punto.
//
// El umbral es relativo al tamaño de letra a propósito. El segundo caso lo vigila: un
// pie de página de 6 pt y un título de 30 pt están a 20 puntos de distancia, que con un
// umbral fijo generoso serían «la misma línea» y se leerían pegados.
func TestTextoParaLeerAgrupaPorLineas(t *testing.T) {
	t.Parallel()

	t.Run("dos trozos de la misma línea van con espacio", func(t *testing.T) {
		t.Parallel()
		// Desordenados a propósito: el motor no garantiza orden de lectura.
		frags := []Fragmento{
			{Texto: "cliente", X: 60, Y: 700, Tamano: 11},
			{Texto: "Estimado", X: 10, Y: 700, Tamano: 11},
		}
		if hay, quiero := TextoParaLeer(frags), "Estimado cliente"; hay != quiero {
			t.Errorf("%q, esperaba %q", hay, quiero)
		}
	})

	t.Run("dos líneas distintas se separan con punto", func(t *testing.T) {
		t.Parallel()
		frags := []Fragmento{
			{Texto: "Primera", X: 10, Y: 700, Tamano: 11},
			{Texto: "Segunda", X: 10, Y: 680, Tamano: 11},
		}
		if hay, quiero := TextoParaLeer(frags), "Primera. Segunda"; hay != quiero {
			t.Errorf("%q, esperaba %q", hay, quiero)
		}
	})

	t.Run("se lee de arriba abajo aunque lleguen al revés", func(t *testing.T) {
		t.Parallel()
		frags := []Fragmento{
			{Texto: "abajo", X: 10, Y: 100, Tamano: 11},
			{Texto: "arriba", X: 10, Y: 700, Tamano: 11},
		}
		if hay, quiero := TextoParaLeer(frags), "arriba. abajo"; hay != quiero {
			t.Errorf("%q, esperaba %q", hay, quiero)
		}
	})

	t.Run("sin fragmentos no hay texto", func(t *testing.T) {
		t.Parallel()
		if hay := TextoParaLeer(nil); hay != "" {
			t.Errorf("%q, esperaba cadena vacía", hay)
		}
	})
}

// TestHerramientaDeMiraLaRegionYNoElIdioma es el caso que la web documenta con nombre y
// apellidos: un austriaco y un alemán escriben los dos `de` y sus herramientas no son
// la misma.
func TestHerramientaDeMiraLaRegionYNoElIdioma(t *testing.T) {
	t.Parallel()
	casos := []struct {
		etiqueta string
		quiero   string // nombre de la herramienta, o "" si no hay
	}{
		{"es-ES", "AutoFirma"},
		{"de-AT", "PDF-Over"},
		// Alemania NO tiene entrada: la salida es la lista de confianza de la UE, no
		// la herramienta austriaca por compartir idioma.
		{"de-DE", ""},
		// Sin región no se adivina nada.
		{"de", ""},
		{"es", ""},
		{"", ""},
		// La región llega en minúsculas de algunos sistemas.
		{"pt-pt", "Autenticação.gov"},
	}
	for _, c := range casos {
		h, ok := HerramientaDe(c.etiqueta)
		if c.quiero == "" {
			if ok {
				t.Errorf("%q: devolvió %q, esperaba ninguna", c.etiqueta, h.Nombre)
			}
			continue
		}
		if !ok || h.Nombre != c.quiero {
			t.Errorf("%q: devolvió %q (ok=%v), esperaba %q", c.etiqueta, h.Nombre, ok, c.quiero)
		}
	}
}

// TestLasHerramientasNacionalesSonOficialesYUnicas vigila las dos invariantes de la
// lista que un descuido rompe: un país repetido (la primera gana y la segunda es
// inalcanzable) y una URL que no sea https.
//
// No comprueba que el sitio exista —eso es red y no es cosa de un test unitario—, sino
// que nadie añada una entrada a medias.
func TestLasHerramientasNacionalesSonOficialesYUnicas(t *testing.T) {
	t.Parallel()
	vistos := map[string]bool{}
	for _, h := range HerramientasNacionales {
		if len(h.Pais) != 2 || h.Pais != string([]byte{h.Pais[0] &^ 32, h.Pais[1] &^ 32}) {
			t.Errorf("%q: el país no es ISO 3166-1 alfa-2 en mayúsculas", h.Pais)
		}
		if vistos[h.Pais] {
			t.Errorf("%q: país repetido; la segunda entrada es inalcanzable", h.Pais)
		}
		vistos[h.Pais] = true
		if h.Nombre == "" {
			t.Errorf("%q: sin nombre", h.Pais)
		}
		if len(h.URL) < 9 || h.URL[:8] != "https://" {
			t.Errorf("%q: %q no es https", h.Pais, h.URL)
		}
	}
}
