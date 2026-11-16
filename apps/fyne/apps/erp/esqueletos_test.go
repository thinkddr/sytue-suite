// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// esqueletosWeb es el fichero de la web que estas medidas copian.
const esqueletosWeb = "../../../frontend/apps/erp/src/components/Esqueletos.tsx"

// TestLosEsqueletosMidenLoDeLaWeb lee Esqueletos.tsx y comprueba que las constantes y
// las clases que fijan la geometría son las que aquí se copian: si la web cambia el
// número de filas, los anchos o un relleno, esto se pone rojo antes de que las dos
// listas dejen de parecerse.
func TestLosEsqueletosMidenLoDeLaWeb(t *testing.T) {
	src, err := os.ReadFile(esqueletosWeb)
	if err != nil {
		t.Fatalf("no se puede leer la web: %v", err)
	}
	web := string(src)
	entero := func(nombre string) int {
		m := regexp.MustCompile(`export const ` + nombre + ` = (\d+);`).FindStringSubmatch(web)
		if m == nil {
			t.Fatalf("la web ya no declara %s", nombre)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if n := entero("FILAS_ESQUELETO_LISTA"); n != FilasEsqueletoLista {
		t.Errorf("la web pinta %d filas y Fyne %d", n, FilasEsqueletoLista)
	}
	if n := entero("COLUMNAS_ESQUELETO_MOVIL"); n != ColumnasEsqueletoMovil {
		t.Errorf("en el móvil la web pinta %d columnas y Fyne %d", n, ColumnasEsqueletoMovil)
	}
	m := regexp.MustCompile(`ANCHOS_ESQUELETO: readonly string\[\] = \[([^\]]*)\]`).FindStringSubmatch(web)
	if m == nil {
		t.Fatal("la web ya no declara ANCHOS_ESQUELETO")
	}
	porcentajes := regexp.MustCompile(`'(\d+)%'`).FindAllStringSubmatch(m[1], -1)
	anchos := make([]float32, 0, len(porcentajes))
	for _, x := range porcentajes {
		n, _ := strconv.Atoi(x[1])
		anchos = append(anchos, float32(n)/100)
	}
	if len(anchos) != len(AnchosEsqueleto) {
		t.Fatalf("la web tiene %d anchos y Fyne %d", len(anchos), len(AnchosEsqueleto))
	}
	for i, a := range anchos {
		if a != AnchosEsqueleto[i] {
			t.Errorf("el ancho %d es %g en la web y %g en Fyne", i, a, AnchosEsqueleto[i])
		}
	}
	// Las clases de las que salen las medidas de aquí.
	for _, clase := range []string{
		`class="flex items-center gap-4 px-3 py-3"`,  // EsqueletoPX, EsqueletoPY, EsqueletoHueco
		`<Skeleton width={anchoEsqueleto(f + c)} />`, // 1rem de alto
		`class="mb-1 flex h-5 items-center"`,         // la fila de la insignia
		`<Skeleton width="40%" height="0.75rem" />`,  // AltoBarraRotulo
		`class="flex h-7.5 items-center"`,            // la caja de línea de text-xl
		`<Skeleton width="60%" height="1.25rem" />`,  // AltoBarraCifra
		`<Skeleton width="50%" height="1.25rem" />`,  // la cifra que carga
		`class="mt-1 flex h-4 items-center"`,         // la de text-xs
		`<Card class="p-4" aria-hidden="true">`,      // TarjetaPX
		`grid grid-cols-1 gap-3 sm:grid-cols-3`,      // CortesResumen y HuecoTarjetas
	} {
		if !strings.Contains(web, clase) {
			t.Errorf("la web ya no lleva %s: revisa las medidas de esqueletos.go", clase)
		}
	}

	rem := float32(tema.Rem)
	cerca(t, "una fila del esqueleto (py-3 + 1rem)", AltoFilaEsqueleto, 2.5*rem)
	cerca(t, "la caja del esqueleto", AltoEsqueletoLista, 2+6*2.5*rem+5)
	cerca(t, "la fila de la insignia (h-5)", ui.MedidaInsignia("").Height, 1.25*rem)
	cerca(t, "la caja de línea de la cifra (h-7.5)", tema.TextoXL.Linea, 1.875*rem)
	cerca(t, "la de la pista (h-4)", tema.TextoXS.Linea, rem)
	cerca(t, "la tarjeta del resumen", AltoKPI, 2*tema.Borde+2*rem+1.25*rem+0.25*rem+1.875*rem+0.25*rem+rem)
	_, alto := tarjetaKPIEsqueleto(tema.Claro, 280)
	cerca(t, "la tarjeta del resumen sin datos", alto, AltoKPI)
}

// esqueletosDe cuenta los bloques de carga y las ruedas de un objeto.
func esqueletosDe(o fyne.CanvasObject) (esqueletos, ruedas int, barras []*ui.Esqueleto) {
	recorrer(o, func(x fyne.CanvasObject) {
		switch e := x.(type) {
		case *ui.Esqueleto:
			esqueletos++
			barras = append(barras, e)
		case *ui.Rueda:
			ruedas++
		}
	})
	return esqueletos, ruedas, barras
}

// TestLaListaQueCargaPintaLaFormaDeLaTabla: seis filas con una barra por columna de
// `modelo.lista`, y en el móvil dos; ni una rueda, y el cuerpo mide lo de la web.
func TestLaListaQueCargaPintaLaFormaDeLaTabla(t *testing.T) {
	l := NuevaLista(traductor(t))
	l.Modelo = catalogoEjemplo().Modulos[0].Modelos[0]
	l.Ventana, l.Fase = 1280, Cargando
	cols := len(l.Modelo.Lista)
	if cols < 3 {
		t.Fatalf("el modelo de ejemplo tiene %d columnas y la prueba pide al menos 3", cols)
	}
	o, alto := l.cuerpo(tema.Claro, 900)
	cerca(t, "el cuerpo que carga", alto, AltoEsqueletoLista)
	w := test.NewWindow(o)
	w.Resize(fyne.NewSize(900, alto+20))
	guardarPNG(t, w.Canvas().Capture(), "erp-lista-cargando.png")
	w.Close()
	n, ruedas, barras := esqueletosDe(o)
	if ruedas != 0 {
		t.Errorf("la lista que carga pinta %d ruedas", ruedas)
	}
	if n != FilasEsqueletoLista*cols {
		t.Errorf("la lista que carga pinta %d barras y deberían ser %d×%d", n, FilasEsqueletoLista, cols)
	}
	for _, b := range barras {
		if b.Fraccion <= 0 || b.Fraccion >= 1 {
			t.Errorf("una barra ocupa %g de su celda: todas iguales parecen ladrillos", b.Fraccion)
		}
	}
	// Con filas ya pintadas, recargar no las cambia por el esqueleto.
	l.Pagina = paginaEjemplo()
	if o, _ := l.cuerpo(tema.Claro, 900); func() bool { n, _, _ := esqueletosDe(o); return n > 0 }() {
		t.Error("una recarga con filas a la vista pinta el esqueleto encima")
	}

	m := NuevaLista(traductor(t))
	m.Modelo, m.Ventana, m.Fase = l.Modelo, 360, Cargando
	o, _ = m.cuerpo(tema.Claro, 328)
	if n, _, _ := esqueletosDe(o); n != FilasEsqueletoLista*ColumnasEsqueletoMovil {
		t.Errorf("en el móvil la lista que carga pinta %d barras y deberían ser %d", n, FilasEsqueletoLista*ColumnasEsqueletoMovil)
	}
}

// TestLaPortadaQueCargaPintaLosIndicadores: sin catálogo, los tres indicadores en su
// rejilla; con catálogo y el resumen en camino, la cifra de cada uno es una barra y no
// un «0,00 €»; si el resumen no llega, vuelve lo neutro.
func TestLaPortadaQueCargaPintaLosIndicadores(t *testing.T) {
	b := portadaArnes(t, 1280)
	b.Fase = Cargando
	portada := b.Objeto(tema.Claro)
	w := test.NewWindow(portada)
	w.Resize(fyne.NewSize(1000, 240))
	guardarPNG(t, w.Canvas().Capture(), "erp-portada-cargando.png")
	w.Close()
	n, ruedas, _ := esqueletosDe(portada)
	if ruedas != 0 || n != 3*3 {
		t.Errorf("la portada que carga pinta %d barras y %d ruedas; deberían ser 9 y 0", n, ruedas)
	}

	b = portadaArnes(t, 1280)
	b.Resumen, b.CargandoResumen = nil, true
	o, _ := b.resumen(tema.Claro, 854)
	if n, _, _ := esqueletosDe(o); n != 3 {
		t.Errorf("con el resumen en camino hay %d barras y deberían ser 3", n)
	}
	if contiene(textosDe(o), "0,00 €") {
		t.Error("con el resumen en camino la portada enseña «0,00 €» como si fuera un dato")
	}
	b.CargandoResumen = false
	o, _ = b.resumen(tema.Claro, 854)
	if n, _, _ := esqueletosDe(o); n != 0 {
		t.Errorf("sin resumen que esperar quedan %d barras latiendo", n)
	}
	if !contiene(textosDe(o), b.t("dashboard.doubleEntry")) {
		t.Error("sin resumen la portada no vuelve a lo neutro")
	}
}
