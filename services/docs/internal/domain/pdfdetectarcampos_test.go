// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la detección de campos. Lo que se comprueba es que distinga las
// TRES formas en que un impreso pide que escribas —la raya, la caja y el
// cuadradito— y que el nombre salga de la etiqueta que hay al lado: un
// formulario cuyos campos se llaman «texto1, texto2» obliga a adivinar tanto al
// rellenarlo como al leer las respuestas.
package domain

import "testing"

// TestDetectarProponeLoQueUnImpresoPide es el check de la pieza.
func TestDetectarProponeLoQueUnImpresoPide(t *testing.T) {
	// Una solicitud: etiqueta + raya, etiqueta + caja, y un cuadradito.
	contenido := "BT /F1 10 Tf 10 80 Td (Nombre:) Tj ET\n" +
		"0.5 w 60 78 m 190 78 l S\n" + // raya de escritura
		"BT /F1 10 Tf 10 55 Td (DNI:) Tj ET\n" +
		"40 50 120 16 re S\n" + // caja de escritura
		"BT /F1 10 Tf 10 25 Td (Acepto) Tj ET\n" +
		"60 22 12 12 re S" // casilla

	d, err := LeerPDF(pdfConContenido(t, contenido,
		`/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >>`))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	props, err := d.DetectarCamposPDF(0)
	if err != nil {
		t.Fatalf("DetectarCamposPDF: %v", err)
	}

	var texto, casillas int
	for _, p := range props {
		switch p.Tipo {
		case CampoTexto:
			texto++
		case CampoCasilla:
			casillas++
		// El PDF de este test solo lleva texto y casillas. Los demás tipos se
		// ENUMERAN —no van por `default`— para que añadir uno nuevo a `TipoCampo`
		// no pase por aquí en silencio (exhaustive), y aun así se marcan como
		// error: la heurística geométrica no puede proponerlos, porque una lista
		// o una firma no se distinguen de un recuadro mirando la página.
		case CampoOpcion, CampoLista, CampoDesplegable, CampoBoton, CampoFirma:
			t.Errorf("propuesta de tipo %q, que la detección no debería proponer", p.Tipo)
		}
	}
	if casillas != 1 {
		t.Errorf("casillas = %d, quería 1 (el cuadradito de 12×12): %+v", casillas, props)
	}
	if texto < 1 {
		t.Errorf("campos de texto = %d, quería al menos la caja de 120×16: %+v", texto, props)
	}
	// El orden es el de rellenado: de arriba abajo.
	for i := 1; i < len(props); i++ {
		if props[i].Rect[1] > props[i-1].Rect[1] {
			t.Errorf("el orden no es de arriba abajo: %+v", props)
			break
		}
	}
}

// TestPropuestaClasificaPorLaForma: es donde vive la decisión, y se prueba
// directa porque cada umbral separa dos cosas que en papel se parecen.
func TestPropuestaClasificaPorLaForma(t *testing.T) {
	casos := []struct {
		nombre string
		r      rectPagina
		quiero TipoCampo
		vale   bool
	}{
		{"raya de escritura", rectPagina{60, 78, 190, 79}, CampoTexto, true},
		{"caja de una línea", rectPagina{40, 50, 160, 66}, CampoTexto, true},
		{"casilla cuadrada", rectPagina{60, 22, 72, 34}, CampoCasilla, true},
		// Los que NO son campos y se le parecen:
		{"raya corta (un subrayado)", rectPagina{60, 78, 90, 79}, "", false},
		{"caja de página entera (el marco)", rectPagina{20, 20, 580, 800}, "", false},
		{"punto suelto", rectPagina{10, 10, 12, 12}, "", false},
		{"columna alta y estrecha", rectPagina{10, 10, 24, 300}, "", false},
	}
	for _, c := range casos {
		p, ok := propuestaDe(c.r)
		if ok != c.vale {
			t.Errorf("%s: aceptado=%v, quería %v", c.nombre, ok, c.vale)
			continue
		}
		if ok && p.Tipo != c.quiero {
			t.Errorf("%s: tipo = %q, quería %q", c.nombre, p.Tipo, c.quiero)
		}
	}
}

// TestElCampoSobreUnaRayaVaEncima: escribir DENTRO de una raya de un punto de
// alto es escribir en nada. El campo tiene que quedar sobre ella, que es donde
// se escribe en el papel.
func TestElCampoSobreUnaRayaVaEncima(t *testing.T) {
	p, ok := propuestaDe(rectPagina{60, 78, 190, 79})
	if !ok {
		t.Fatal("una raya de 130×1 es una línea de escritura")
	}
	if p.Rect[1] < 79 {
		t.Errorf("rect = %v: el campo debe empezar POR ENCIMA de la raya", p.Rect)
	}
	if alto := p.Rect[3] - p.Rect[1]; alto < 10 {
		t.Errorf("alto = %v: no cabe una línea de texto", alto)
	}
}

// TestNoSeProponeUnCampoDondeYaHayUno: duplicarlo deja dos cajas superpuestas y
// quien rellena no sabe en cuál escribe.
func TestNoSeProponeUnCampoDondeYaHayUno(t *testing.T) {
	base, err := AnadirCamposPDF(docDePrueba(t, "impreso"), []CampoNuevo{
		{Tipo: CampoTexto, Nombre: "ya", Pagina: 0, Rect: [4]float64{40, 50, 160, 66}},
	})
	if err != nil {
		t.Fatalf("AnadirCamposPDF: %v", err)
	}
	d, err := LeerPDF(base)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	props, err := d.DetectarCamposPDF(0)
	if err != nil {
		t.Fatalf("DetectarCamposPDF: %v", err)
	}
	for _, p := range props {
		if solapaAlguno(p.Rect, []rectPagina{{40, 50, 160, 66}}) {
			t.Errorf("se propuso un campo encima del que ya existe: %+v", p)
		}
	}
}

// TestLaEtiquetaSaleDelTextoDeAlLado: de aquí sale un nombre que significa algo.
func TestLaEtiquetaSaleDelTextoDeAlLado(t *testing.T) {
	frag := []FragmentoEditable{
		{Texto: "Nombre:", X: 10, Y: 80},
		{Texto: "lejos", X: 400, Y: 80},
		{Texto: "Domicilio . . .", X: 10, Y: 100},
	}
	if got := etiquetaCercana([4]float64{60, 75, 200, 89}, frag); got != "Nombre" {
		t.Errorf("etiqueta = %q, quería «Nombre» (a la izquierda, misma línea)", got)
	}
	// Los dos puntos y los puntos de relleno de una línea de puntos sobran.
	if got := trimEtiqueta("Domicilio . . . :"); got != "Domicilio" {
		t.Errorf("trimEtiqueta = %q", got)
	}
}

// TestDetectarNoRevientaConUnaPaginaVacia: un escaneo no tiene geometría, y eso
// no es un error.
func TestDetectarNoRevientaConUnaPaginaVacia(t *testing.T) {
	base, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 40, 40, false), Nombre: "s.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	d, err := LeerPDF(base)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	props, err := d.DetectarCamposPDF(0)
	if err != nil {
		t.Fatalf("DetectarCamposPDF: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("propuestas = %+v, quería ninguna en un escaneo", props)
	}
	if _, err := d.DetectarCamposPDF(9); err == nil {
		t.Errorf("una página que no existe debe fallar")
	}
}
