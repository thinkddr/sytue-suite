// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
	_, alto := tarjetaKPIEsqueleto(tema.Claro, ui.NuevosBloquesEsqueleto(), 280)
	cerca(t, "la tarjeta del resumen sin datos", alto, AltoKPI)
}

// esqueletosDe cuenta las barras de carga —sueltas o de un `ui.BloquesEsqueleto`— y las
// ruedas de un objeto, y devuelve los grupos.
func esqueletosDe(o fyne.CanvasObject) (esqueletos, ruedas int, grupos []*ui.BloquesEsqueleto) {
	recorrer(o, func(x fyne.CanvasObject) {
		switch e := x.(type) {
		case *ui.Esqueleto:
			esqueletos++
		case *ui.BloquesEsqueleto:
			esqueletos += len(e.Bloques())
			grupos = append(grupos, e)
		case *ui.Rueda:
			ruedas++
		}
	})
	return esqueletos, ruedas, grupos
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
	n, ruedas, grupos := esqueletosDe(o)
	if ruedas != 0 {
		t.Errorf("la lista que carga pinta %d ruedas", ruedas)
	}
	// Todas las barras en UN grupo, que late a una. VISTO EN ROJO con una
	// `ui.Esqueleto` por barra: «0 grupos».
	if n != FilasEsqueletoLista*cols || len(grupos) != 1 {
		t.Fatalf("la lista que carga pinta %d barras en %d grupos y deberían ser %d×%d en uno", n, len(grupos), FilasEsqueletoLista, cols)
	}
	celda := (900 - 2*tema.Borde - 2*EsqueletoPX - float32(cols-1)*EsqueletoHueco) / float32(cols)
	anchos := map[float32]bool{}
	for _, b := range grupos[0].Bloques() {
		anchos[b.Size().Width] = true
		if w := b.Size().Width; w <= 0 || w >= celda {
			t.Errorf("una barra mide %g de una celda de %g: todas enteras parecen ladrillos", w, celda)
		}
	}
	if len(anchos) < 3 {
		t.Errorf("las barras solo tienen %d anchos distintos", len(anchos))
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
	n, ruedas, grupos := esqueletosDe(portada)
	if ruedas != 0 || n != 3*3 || len(grupos) != 1 {
		t.Errorf("la portada que carga pinta %d barras en %d grupos y %d ruedas; deberían ser 9 en uno y 0", n, len(grupos), ruedas)
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

// TestLasCifrasQueCarganLatenAUna: con el resumen en camino, las tres cifras son UN
// grupo de carga —un latido y un «Cargando»— y cada barra sigue donde estaba la del
// `ui.Esqueleto` suelto: la mitad del ancho útil de su tarjeta, `1.25rem` de alto y
// centrada en la caja de línea de `text-xl`. VISTO EN ROJO con la barra suelta: «3
// barras en 0 grupos».
func TestLasCifrasQueCarganLatenAUna(t *testing.T) {
	b := portadaArnes(t, 1280)
	b.Resumen, b.CargandoResumen = nil, true
	o, alto := b.resumen(tema.Claro, 854)
	w := test.NewWindow(o)
	defer w.Close()
	w.Resize(fyne.NewSize(854, alto))
	o.Resize(fyne.NewSize(854, alto))
	n, _, grupos := esqueletosDe(o)
	if n != 3 || len(grupos) != 1 {
		t.Fatalf("con el resumen en camino hay %d barras en %d grupos; deberían ser 3 en uno", n, len(grupos))
	}
	rej := ui.RejillaTarjetas(b.Ventana, HuecoTarjetas, CortesResumen)
	dentro := rej.AnchoColumna(854) - 2*tema.Borde - 2*TarjetaPX
	xs := make([]float32, 0, len(grupos[0].Bloques()))
	for _, q := range grupos[0].Bloques() {
		cerca(t, "el ancho de la cifra que carga", q.Size().Width, float32(int(dentro*0.5+0.5)))
		cerca(t, "su alto", q.Size().Height, AltoBarraCifra)
		p := fyne.CurrentApp().Driver().AbsolutePositionForObject(q)
		xs = append(xs, p.X)
	}
	// Las tres barras, una por tarjeta y a una columna de distancia.
	for i := 1; i < len(xs); i++ {
		cerca(t, "de una cifra a la siguiente", xs[i]-xs[i-1], rej.AnchoColumna(854)+HuecoTarjetas)
	}
}

// TestLaFacturacionQueCargaLateConLasPiezasComunes: mientras llega la serie, la barra
// de la tarjeta de facturación por mes es un bloque de UN `ui.BloquesEsqueleto` —el
// pulso común y un «Cargando» que la web dice con el `aria-busy` de la tarjeta— y sigue
// donde estaba la del `ui.Esqueleto` suelto: bajo el título y su `mb-3`, a todo el ancho
// útil y al alto de las barras. VISTO EN ROJO con la barra suelta: «1 barras en 0
// grupos».
func TestLaFacturacionQueCargaLateConLasPiezasComunes(t *testing.T) {
	for _, ancho := range []float32{854, 328} {
		b := portadaArnes(t, 1280)
		b.Hoy = hoyFacturacion
		b.Facturacion = FacturacionMensual{Cargando: true}
		o, alto := b.tarjetaFacturacion(tema.Claro, ancho)
		if o == nil {
			t.Fatal("mientras carga la serie la tarjeta no se pinta")
		}
		w := test.NewWindow(o)
		w.Resize(fyne.NewSize(ancho, alto))
		o.Resize(fyne.NewSize(ancho, alto))
		n, _, grupos := esqueletosDe(o)
		if n != 1 || len(grupos) != 1 {
			w.Close()
			t.Fatalf("a %g de ancho la facturación que carga pinta %d barras en %d grupos; debería ser una en uno", ancho, n, len(grupos))
		}
		if grupos[0].AccessibilityLabel() == "" {
			t.Error("la facturación que carga no dice «Cargando»")
		}
		d := fyne.CurrentApp().Driver()
		q := grupos[0].Bloques()[0]
		p := d.AbsolutePositionForObject(q).Subtract(d.AbsolutePositionForObject(o))
		x0 := tema.Borde + TarjetaPX
		cerca(t, "la x de la barra que carga", p.X, x0)
		cerca(t, "su y, bajo el título", p.Y, x0+tema.TextoXS.Linea+TituloParteMB)
		cerca(t, "su ancho", q.Size().Width, ancho-2*x0)
		cerca(t, "su alto, el de las barras", q.Size().Height, AltoBarras)
		w.Close()
	}
}

// TestElERPNoMontaEsqueletosSueltos: toda carga del ERP late con las piezas comunes
// (`ui.FilasEsqueleto`, `ui.BloquesEsqueleto`), que anuncian «Cargando» una vez; un
// `ui.NuevoEsqueleto*` suelto late a su aire y no dice nada. La barra de la facturación
// por mes era la última.
func TestElERPNoMontaEsqueletosSueltos(t *testing.T) {
	fuentes, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, ruta := range fuentes {
		if strings.HasSuffix(ruta, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), ruta, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok {
				if x, ok := s.X.(*ast.Ident); ok && x.Name == "ui" && strings.HasPrefix(s.Sel.Name, "NuevoEsqueleto") {
					t.Errorf("%s monta un ui.%s suelto: va con ui.FilasEsqueleto o ui.BloquesEsqueleto", ruta, s.Sel.Name)
				}
			}
			return true
		})
	}
}
