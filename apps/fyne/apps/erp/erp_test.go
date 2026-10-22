// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

// Chromium, Figtree y raíz de 16 px: el arnés ERP real del 2026-10-04 mide
// bienvenida/lista/ficha con datos, a 1280 y en móvil. Los controles del cajón
// conservan sus clases CSS de ErpApp.tsx, y encima lo que app.css le pone a
// `#erp-cajon` por su id: `padding: 1.25rem 1rem` y `min-height: 2.75rem` en sus
// botones y entradas. Ninguna expectativa se obtiene de los helpers nativos que se
// están comprobando.
const (
	menuMedido      = 260         // el `<nav id="erp-cajon">`
	entradaMedida   = 44          // `min-height: 2.75rem` de `#erp-cajon li > button`
	areaMedida      = 28          // texto xs (16) y `py-1.5`
	botonPaletaAlto = 44          // `min-height: 2.75rem` de `#erp-cajon > button`
	botonIAAlto     = 44          // ídem: gana a su `h-8`
	cabeceraMedida  = float32(83) // lista desktop: dos filas en el DOM del arnés
	buscadorMedido  = 32          // `input[type=search]`, `w-44`
	anchoBuscador   = 176         // `w-44` a 16 px por rem
	pildoraMedida   = 24          // píldora `size="xs"`
	anchoContenido  = 976         // máximo `max-w-5xl` (1024) menos `px-6`
	filaEntradaX    = 16          // el `1rem` del `padding` de `#erp-cajon`
	anchoEntrada    = 227         // 260 menos el filete y dos veces 16
	yInicioMedido   = 196         // 20 + aspa (44) + mb-3 + paleta (44) + mb-4 + Sytue AI (44) + mb-4
)

// anchoEstrecho es la primera colocación del guardián de truncado: estrecho de verdad,
// para que algo se recorte, y NO cero —a cero `Resize` no llama a `Layout`—.
var anchoEstrecho = fyne.NewSize(160, 744)

func traductor(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// catalogoEjemplo es un catálogo pequeño con la forma del de producción: dos áreas, una
// con estados y dinero.
func catalogoEjemplo() Catalogo {
	return Catalogo{
		Activos: []string{"facturacion", "contactos"},
		Modulos: []Modulo{
			{Clave: "facturacion", Etiqueta: "Ventas", Modelos: []Modelo{{
				Clave: "factura_venta", Modulo: "facturacion", Etiqueta: "Facturas de cliente",
				Lista: []string{"ref", "cliente", "total", "estado"},
				Campos: []Campo{
					{Clave: "cliente", Tipo: api.ERPRelacion, Etiqueta: "Cliente"},
					{Clave: "total", Tipo: api.ERPDinero, Etiqueta: "Total"},
				},
				Estados: []api.ERPOpcion{
					{Valor: "borrador", Etiqueta: "Borrador"},
					{Valor: "pagado", Etiqueta: "Pagado"},
				},
			}}},
			{Clave: "contactos", Etiqueta: "Contactos", Modelos: []Modelo{{
				Clave: "cliente", Modulo: "contactos", Etiqueta: "Clientes",
				Lista:  []string{"ref", "nombre"},
				Campos: []Campo{{Clave: "nombre", Tipo: "texto", Etiqueta: "Nombre"}},
			}}},
		},
	}
}

// --- Las medidas -----------------------------------------------------------

// TestElCarrilMideLoDeLaWeb: el menú del ERP no es el de la suite y sus números son
// suyos. Con los 260 y los 33,25 de `ui.Lateral` y `ui.FilaLateral`, un catálogo de
// treinta y tres áreas desborda la ventana; por eso este menú es propio.
//
// VISTO EN ROJO (en el piloto, 2026-09-22): poniendo `EntradaPY = tema.Espacio(2)` —el
// `py-2` del menú de la suite en vez del `py-1.5` del ERP— el test dice
// «AltoEntrada = 29.75, la web mide 26.25». Deshecho.
func TestElCarrilMideLoDeLaWeb(t *testing.T) {
	casos := []struct {
		que         string
		got, quiero float32
	}{
		{"AnchoMenu", AnchoMenu, menuMedido},
		{"AltoEntrada", AltoEntrada, entradaMedida},
		{"AltoCabeceraArea", AltoCabeceraArea, areaMedida},
		{"AltoBotonPaleta", AltoBotonPaleta, botonPaletaAlto},
		{"AltoBotonIA", AltoBotonIA, botonIAAlto},
		{"MenuPX", MenuPX, filaEntradaX},
	}
	for _, c := range casos {
		if c.got != c.quiero {
			t.Errorf("%s = %v, la web mide %v", c.que, c.got, c.quiero)
		}
	}
}

// TestLaListaMideLoDeLaWeb: la cabecera, el buscador y las píldoras.
func TestLaListaMideLoDeLaWeb(t *testing.T) {
	l := NuevaLista(traductor(t))
	l.Modelo, l.Pagina, l.Ventana, l.Fase = facturas(), paginaEjemplo(), 1280, Exito
	l.Total, l.SumaDinero, l.AlNueva = 4, 440696, func() {}
	_, altoCab := l.cabecera(tema.Claro, anchoContenido)
	casos := []struct {
		que         string
		got, quiero float32
	}{
		{"la cabecera, que salta de fila", altoCab, cabeceraMedida},
		{"AltoBuscador", AltoBuscador, buscadorMedido},
		{"AnchoBuscador", AnchoBuscador, anchoBuscador},
		{"AltoPildora", AltoPildora, pildoraMedida},
		{"AnchoContenidoLista − px-6", AnchoContenidoLista - 2*MargenLista, anchoContenido},
	}
	for _, c := range casos {
		if c.got != c.quiero {
			t.Errorf("%s = %v, la web mide %v", c.que, c.got, c.quiero)
		}
	}
}

// TestElCarrilColocaDondeLaWeb pinta el menú de verdad y comprueba DÓNDE cae la primera
// entrada, que es lo que encadena todas las medidas de arriba: el relleno del cajón, el
// aspa con su `mb-3`, el botón de paleta con su `mb-4` y el de Sytue AI con el suyo.
func TestElCarrilColocaDondeLaWeb(t *testing.T) {
	p := pantallaDePrueba(t)
	m := p.menu
	p.rellenarMenu()
	obj := m.Objeto(tema.Claro)
	obj.Resize(fyne.NewSize(AnchoMenu, 800))

	inicio := buscarEntrada(obj, "Inicio")
	if inicio == nil {
		t.Fatal("no se ha pintado la entrada de Inicio")
	}
	if got := inicio.Position(); got.X != filaEntradaX || got.Y != yInicioMedido {
		t.Errorf("«Inicio» en (%v, %v); la web la pone en (%v, %v)",
			got.X, got.Y, filaEntradaX, yInicioMedido)
	}
	if got := inicio.Size().Width; got != anchoEntrada {
		t.Errorf("«Inicio» mide %v de ancho, la web %v", got, anchoEntrada)
	}
}

func buscarEntrada(o fyne.CanvasObject, rotulo string) fyne.CanvasObject {
	if e, ok := o.(*Entrada); ok && e.Rotulo == rotulo {
		return e
	}
	if c, ok := o.(*fyne.Container); ok {
		for _, h := range c.Objects {
			if x := buscarEntrada(h, rotulo); x != nil {
				return x
			}
		}
	}
	return nil
}

// --- El dominio ------------------------------------------------------------

// TestLaCeldaFormateaComoLaWeb es el guardián de `celda` de formato.ts. Lo que vigila
// de verdad es el DINERO: viaja en céntimos y se enseña en euros, y equivocarse ahí es
// el único fallo de este paquete que nadie ve hasta que alguien cobra de más.
//
// VISTO EN ROJO (en el piloto): quitando el `/100` de `i18n.FormatCurrency` sale
// «123.456,56 €» donde tocan «1234,56 €», y el vector de los 26 idiomas se cae con él
// («€5.05» donde Intl dice «€0.05»). Deshecho.
func TestLaCeldaFormateaComoLaWeb(t *testing.T) {
	etiquetas := map[string]string{"c1": "Ferretería Lobo"}
	casos := []struct {
		que    string
		campo  Campo
		valor  any
		quiero string
	}{
		// Ojo con el separador de millar: el español NO agrupa por debajo de 10.000
		// (`min_grupo` 2 en formato.json), así que 1234,56 va SIN punto y 12.345,67 con
		// él. Comprobado contra Intl en el piloto, que es quien manda aquí.
		{"dinero sin agrupar", Campo{Tipo: api.ERPDinero}, float64(123456), "1234,56 €"},
		{"dinero agrupado", Campo{Tipo: api.ERPDinero}, float64(1234567), "12.345,67 €"},
		{"dinero negativo", Campo{Tipo: api.ERPDinero}, float64(-500), "-5,00 €"},
		{"dinero cero", Campo{Tipo: api.ERPDinero}, float64(0), "0,00 €"},
		{"entero", Campo{Tipo: api.ERPEntero}, float64(12345), "12.345"},
		{"decimal conserva ceros", Campo{Tipo: api.ERPDecimal}, "2.50", "2,50"},
		{"decimal sin agrupar", Campo{Tipo: api.ERPDecimal}, "1234.5", "1234,5"},
		{"decimal agrupado", Campo{Tipo: api.ERPDecimal}, "12345.5", "12.345,5"},
		{
			"fecha",
			Campo{Tipo: api.ERPFecha},
			"2026-09-22",
			i18n.FormatDate("es", time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)),
		},
		{"booleano sí", Campo{Tipo: api.ERPBooleano}, true, "✓"},
		{"booleano no", Campo{Tipo: api.ERPBooleano}, false, SinValor},
		{"sin valor", Campo{Tipo: "texto"}, nil, SinValor},
		{"relación resuelta", Campo{Tipo: api.ERPRelacion}, "c1", "Ferretería Lobo"},
		// La fila apuntada está borrada: se DICE, en vez de enseñar un identificador.
		{"relación huérfana", Campo{Tipo: api.ERPRelacion}, "c9", SinValor},
		{"selección", Campo{
			Tipo:     api.ERPSeleccion,
			Opciones: []api.ERPOpcion{{Valor: "a", Etiqueta: "Alta"}},
		}, "a", "Alta"},
	}
	for _, c := range casos {
		t.Run(c.que, func(t *testing.T) {
			if got := Celda(c.campo, c.valor, "es", etiquetas); got != c.quiero {
				t.Errorf("Celda = %q, la web escribe %q", got, c.quiero)
			}
		})
	}
}

// TestElOrdenCiclaComoLaWeb: primero descendente, luego ascendente, luego ninguno.
// Pulsar OTRA columna empieza de nuevo en descendente.
func TestElOrdenCiclaComoLaWeb(t *testing.T) {
	var o Orden
	o = o.Siguiente("total")
	if o != (Orden{Campo: "total"}) {
		t.Fatalf("primer toque = %+v, se esperaba descendente por total", o)
	}
	if o = o.Siguiente("total"); o != (Orden{Campo: "total", Asc: true}) {
		t.Fatalf("segundo toque = %+v, se esperaba ascendente", o)
	}
	if o = o.Siguiente("total"); o != (Orden{}) {
		t.Fatalf("tercer toque = %+v, se esperaba sin orden", o)
	}
	o = Orden{Campo: "total", Asc: true}
	if got := o.Siguiente("ref"); got != (Orden{Campo: "ref"}) {
		t.Fatalf("otra columna = %+v, se esperaba descendente por ref", got)
	}
}

// TestLaPaletaBuscaSinTildes: quien escribe «fabricacion» encuentra «Fabricación», que
// es lo que hace el `normalize('NFD')` de la web.
func TestLaPaletaBuscaSinTildes(t *testing.T) {
	p := NuevaPaleta(traductor(t), []Modelo{
		{Clave: "orden", Etiqueta: "Órdenes de fabricación"},
		{Clave: "cliente", Etiqueta: "Clientes"},
	})
	p.Escribir("fabricacion")
	c := p.Coincidencias()
	if len(c) != 1 || c[0].Clave != "orden" {
		t.Fatalf("«fabricacion» encuentra %v, se esperaba solo la orden", claves(c))
	}
	p.Escribir("")
	if len(p.Coincidencias()) != 2 {
		t.Error("sin consulta se enseñan todos")
	}
}

func claves(ms []Modelo) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Clave)
	}
	return out
}

// TestElMenuNoEnsenaLoQueElServicioVaAProhibir: los permisos son POR MÓDULO y un área
// sin lectura NO se ofrece. Enseñarla es prometer algo que el servicio va a contestar
// con un 403, y es lo que tenía la pantalla medida.
//
// VISTO EN ROJO (en el piloto): haciendo que `PuedeLeer` devuelva siempre `true` el
// test dice que se ofrecen 2 áreas donde toca 1. Deshecho.
func TestElMenuNoEnsenaLoQueElServicioVaAProhibir(t *testing.T) {
	p := pantallaDePrueba(t)
	// Mientras no se sabe, se enseña todo y manda el servicio: es lo que hace la web.
	if got := len(p.AreasVisibles()); got != 2 {
		t.Fatalf("sin saber permisos se ven %d áreas, se esperaban 2", got)
	}
	p.SabePermisos = true
	p.Permisos = Permisos{Modulos: map[string]string{
		"facturacion": api.ERPLectura,
		"contactos":   api.ERPNinguno,
	}}
	visibles := p.AreasVisibles()
	if len(visibles) != 1 || visibles[0].Clave != "facturacion" {
		t.Fatalf("con permiso solo en facturación se ven %d áreas (%v)", len(visibles), visibles)
	}
}

// --- Los seis estados ------------------------------------------------------

// TestElPermisoDenegadoNoSeLeeComoVacio es la DIFERENCIA DELIBERADA con la web, y por
// eso tiene guardián propio.
//
// Con un 403 la lista no está vacía, es que no se puede ver. La web pintaba una banda
// roja con el respaldo en inglés del servicio y DEBAJO «Aquí todavía no hay nada. Crea
// el primero con Nuevo.»; desde el PR #840 pinta el estado vacío con candado y las dos
// claves del ERP, sin banda. VISTO EN ROJO devolviendo la banda.
func TestElPermisoDenegadoNoSeLeeComoVacio(t *testing.T) {
	tr := traductor(t)
	l := NuevaLista(tr)
	l.Modelo = catalogoEjemplo().Modulos[0].Modelos[0]
	l.Ventana = 1280
	l.Fase = Prohibida

	textos := textosDe(l.Objeto(tema.Claro))
	titulo, vacio := tr.T("error.sinPermiso.titulo", nil), tr.T("list.empty", nil)
	if !contiene(textos, titulo) {
		t.Errorf("con un 403 no se dice «%s»; se pintó %v", titulo, textos)
	}
	if contiene(textos, vacio) {
		t.Errorf("con un 403 se sigue diciendo «%s»", vacio)
	}
	if l.bandaAviso(tema.Claro) != nil {
		t.Error("con un 403 sigue la banda roja encima del estado vacío")
	}
}

// TestCadaFaseDeLaListaPintaAlgoDistinto: los seis estados existen y ninguno es el
// mismo dibujo que otro. Un estado que no se distingue no es un estado.
func TestCadaFaseDeLaListaPintaAlgoDistinto(t *testing.T) {
	vistos := map[string][]Fase{}
	for _, f := range []Fase{Cargando, Exito, Vacia, Fallo, SinConexion, Prohibida} {
		l := NuevaLista(traductor(t))
		l.Modelo = catalogoEjemplo().Modulos[0].Modelos[0]
		l.Ventana = 1280
		l.Fase = f
		if f == Exito {
			l.Pagina = paginaEjemplo()
		}
		clave := strings.Join(textosDe(l.Objeto(tema.Claro)), "|")
		vistos[clave] = append(vistos[clave], f)
	}
	for clave, fases := range vistos {
		if len(fases) > 1 {
			t.Errorf("las fases %v pintan lo mismo (%q)", fases, clave)
		}
	}
}

// TestFaseDeOrdenaComoLaWeb: sin red no se enseña un error de red, y un 403 es permiso
// y no un reintento.
func TestFaseDeOrdenaComoLaWeb(t *testing.T) {
	prohibido := &api.Error{Status: 403}
	casos := []struct {
		que       string
		err       error
		hayRed    bool
		elementos int
		quiero    Fase
	}{
		{"sin red manda la red", prohibido, false, 0, SinConexion},
		{"403 es permiso", prohibido, true, 0, Prohibida},
		{"otro error es fallo", &api.Error{Status: 500}, true, 0, Fallo},
		{"cero filas es vacío", nil, true, 0, Vacia},
		{"con filas es éxito", nil, true, 3, Exito},
	}
	for _, c := range casos {
		if got := FaseDe(c.err, c.hayRed, c.elementos); got != c.quiero {
			t.Errorf("%s: FaseDe = %v, se esperaba %v", c.que, got, c.quiero)
		}
	}
}

// TestLaListaNoPintaMandosQueNoHacenNada: esta ola es de solo lectura, así que «Nuevo»
// e «Importar» NO están. Un botón que no hace nada miente más que uno que no está, y en
// un ERP el que miente es «Nuevo».
//
// «Importar CSV» no se pinta: pide el diálogo de ficheros del sistema, que todavía no
// hay. Y «Nuevo» solo con escritura en el módulo, como en la web (`soloLectura`).
func TestLaListaNoPintaMandosQueNoHacenNada(t *testing.T) {
	tr := traductor(t)
	l := NuevaLista(tr)
	l.Modelo = catalogoEjemplo().Modulos[0].Modelos[0]
	l.Ventana = 1280
	l.Fase = Exito
	l.Pagina = paginaEjemplo()
	l.AlNueva = func() {}
	l.SoloLectura = true
	textos := textosDe(l.Objeto(tema.Claro))
	for _, clave := range []string{"action.new", "list.import"} {
		if contiene(textos, tr.T(clave, nil)) {
			t.Errorf("se pinta «%s» sin escritura en el módulo", tr.T(clave, nil))
		}
	}
	l.SoloLectura = false
	if !contiene(textosDe(l.Objeto(tema.Claro)), tr.T("action.new", nil)) {
		t.Error("con escritura en el módulo no se ofrece «Nuevo»")
	}
}

// cabeceraDe es la cabecera de la lista a 854, el ancho del contenido a 1280.
func cabeceraDe(l *Lista) fyne.CanvasObject {
	c, _ := l.cabecera(tema.Claro, 854)
	return c
}

func paginaEjemplo() Pagina {
	return Pagina{
		Registros: []Registro{
			{
				ID: "1", Ref: "FV-2026-00041", Etiqueta: "Ferretería Lobo", Estado: "pagado",
				Data: map[string]any{"cliente": "c1", "total": float64(123456)},
			},
			{
				ID: "2", Ref: "FV-2026-00042", Etiqueta: "Talleres Sur", Estado: "borrador",
				Data: map[string]any{"cliente": "c2", "total": float64(9900)},
			},
		},
		Etiquetas: map[string]string{"c1": "Ferretería Lobo", "c2": "Talleres Sur"},
	}
}

func pantallaDePrueba(t *testing.T) *Pantalla {
	t.Helper()
	p := NuevaPantalla(traductor(t), nil, nil)
	p.Catalogo = catalogoEjemplo()
	p.paleta.Modelos = p.Modelos()
	return p
}

// textosDe recoge el texto VISIBLE del árbol, que es lo que una persona lee.
//
// Lo de «visible» no es un detalle: `ui.Tabla` lleva siempre su rótulo de «sin
// resultados» colgado del renderer y solo lo esconde, así que recoger el árbol entero
// hacía creer que una tabla con dos filas dice «Aquí todavía no hay nada».
func textosDe(o fyne.CanvasObject) []string {
	var out []string
	recorrer(o, func(x fyne.CanvasObject) {
		if v, ok := x.(*canvas.Text); ok && v.Visible() && v.Text != "" {
			out = append(out, v.Text)
		}
	})
	return out
}

func contiene(xs []string, s string) bool {
	for _, x := range xs {
		if x == s || strings.Contains(x, s) {
			return true
		}
	}
	return false
}

func recorrer(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	if !o.Visible() {
		return
	}
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			recorrer(h, f)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			recorrer(h, f)
		}
	}
}

// guardarPNG deja el dibujo en disco para MIRARLO. Un test que describe el resultado de
// memoria no ha visto nada; el fallo más caro de esta sesión pasó todos los tests.
func guardarPNG(t *testing.T, img image.Image, nombre string) {
	t.Helper()
	destino := os.Getenv("SYTUE_PNG")
	if destino == "" {
		return
	}
	f, err := os.Create(destino + "/" + nombre)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestLaPantallaEnteraSePinta monta la aplicación con el pintor de software —sin CGO y
// sin servidor gráfico— y guarda el PNG para MIRARLO. Comprueba además lo que un
// dibujo en blanco no delata solo: que el carril está donde toca y que hay tinta.
//
// VISTO EN ROJO (en el piloto): dejando el `canvas.NewRectangle` del fondo fuera del
// carril, el test dice que la columna del menú no es de tinta. Deshecho.
func TestLaPantallaEnteraSePinta(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Fase = Exito
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 744))

	img := w.Canvas().Capture()
	guardarPNG(t, img, "erp-portada.png")
	if img.Bounds().Dx() == 0 {
		t.Fatal("el lienzo salió vacío")
	}
	// En ancho lo que se ve a la izquierda es el carril de iconos, de 72 y de su color;
	// el menú de módulos es un cajón y está cerrado: a su derecha ya es `main`.
	rd, gd, bd, _ := img.At(4, 400).RGBA()
	esperado := tema.Color("rail", tema.Claro)
	if dif(rd>>8, uint32(esperado.R))+dif(gd>>8, uint32(esperado.G))+dif(bd>>8, uint32(esperado.B)) > 3 {
		t.Errorf("el carril de iconos pinta (%d,%d,%d) y el token `rail` es (%d,%d,%d)",
			rd>>8, gd>>8, bd>>8, esperado.R, esperado.G, esperado.B)
	}
	if img.At(int(AnchoCarril)+4, 400) == img.At(4, 400) {
		t.Error("pasado el carril de iconos sigue su color: el menú de módulos no es un cajón cerrado")
	}
}

// dif es la distancia entre dos niveles. El alfa se cuantiza y por eso ningún gate
// puede exigir byte a byte: el margen es de ±1 nivel de 255 por canal.
func dif(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestLaListaConFilasSePinta guarda también la lista con datos: es la pantalla que el
// barrido de producción NO pudo medir, porque la cuenta de auditoría recibe un 403.
func TestLaListaConFilasSePinta(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Fase = Exito
	p.lista.Modelo = catalogoEjemplo().Modulos[0].Modelos[0]
	p.lista.Pagina = paginaEjemplo()
	p.lista.Fase = Exito
	p.lista.Total = 2
	p.lista.SumaDinero = 133356
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 744))
	guardarPNG(t, w.Canvas().Capture(), "erp-lista.png")

	textos := textosDe(p.Objeto())
	// El acumulado va en euros, no en céntimos: es el número que más caro sale mal.
	if !contiene(textos, "1333,56") {
		t.Errorf("el acumulado no sale en euros; se pintó %v", textos)
	}
}

// TestLaPaletaSePinta guarda la paleta abierta.
func TestLaPaletaSePinta(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Fase = Exito
	p.abierta = true
	p.paleta.Escribir("cli")
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 744))
	img := w.Canvas().Capture()
	guardarPNG(t, img, "erp-paleta.png")
	// El velo ATENÚA lo de detrás. Se comprueba contra la misma pantalla sin paleta,
	// no contra un color escrito a mano: un velo transparente pasa cualquier test que
	// solo mire que «hay algo pintado», y es justo lo que pasó.
	sin := pantallaDePrueba(t)
	sin.Fase = Exito
	w2 := test.NewWindow(sin.Objeto())
	defer w2.Close()
	w2.Resize(fyne.NewSize(1280, 744))
	limpio := w2.Canvas().Capture()
	if img.At(700, 600) == limpio.At(700, 600) {
		t.Error("con la paleta abierta el fondo se pinta igual: el velo no atenúa")
	}
	// «cli» casa también con «Facturas de cliente»: la web filtra por `includes`, no
	// por prefijo, y eso es lo que hace que la paleta encuentre lo que no se sabe
	// nombrar entero.
	if c := p.paleta.Coincidencias(); len(c) != 2 {
		t.Errorf("«cli» encuentra %v, se esperaban los dos que llevan «cliente»", claves(c))
	}
}

// TestNingunRotuloSeComeASiMismo es el guardián del fallo que TODOS los demás tests
// dejaron pasar y que solo se vio mirando el PNG: el título de la lista había
// desaparecido.
//
// La causa es la que `ui.Truncar` avisa en su propia documentación: hay que pasarle
// SIEMPRE el texto entero. Estos layouts le pasaban lo que ya había pintado, y como
// Fyne llama a `Layout` antes de dar a la caja su ancho de verdad, el primer paso a lo
// ancho de cero dejaba «…» y todos los siguientes truncaban sobre ese «…».
//
// Por eso el test coloca DOS VECES —primero estrecho, luego ancho—: una sola pasada
// pasa siempre.
//
// Y el estrecho es **160, no 0**. La primera versión de este guardián usaba
// `Resize(0, 0)` y salía VERDE con el fallo puesto: un `fyne.Container` nace ya a
// tamaño cero y `Resize` al mismo tamaño no llama a `Layout`, así que no había primera
// pasada que estropeara nada. Un guardián que no se ha visto en rojo no guarda nada, y
// este estuvo media hora sin guardar.
//
// VISTO EN ROJO (en el piloto, 2026-09-22, ya con los 160): con el
// `ui.Truncar(c.titulo, c.titulo.Text, …)` de antes dice que el título de la lista no
// sobrevive a dos colocaciones. Deshecho.
func TestNingunRotuloSeComeASiMismo(t *testing.T) {
	tr := traductor(t)
	l := NuevaLista(tr)
	l.Modelo = catalogoEjemplo().Modulos[0].Modelos[0]
	l.Ventana = 1280
	l.Fase = Exito
	l.Pagina = paginaEjemplo()
	l.Total, l.SumaDinero = 2, 133356

	b := NuevaBienvenida(tr)
	b.Modulos, b.Activos = catalogoEjemplo().Modulos, catalogoEjemplo().Activos
	b.Ventana = 1280
	b.Fase = Exito

	p := NuevaPaleta(tr, b.Modulos[0].Modelos)

	casos := map[string]fyne.CanvasObject{
		"lista":    l.Objeto(tema.Claro),
		"portada":  b.Objeto(tema.Claro),
		"paleta":   p.Objeto(tema.Claro, fyne.NewSize(1280, 744)),
		"cabecera": cabeceraDe(l),
	}
	for nombre, obj := range casos {
		t.Run(nombre, func(t *testing.T) {
			// El paso estrecho es el que Fyne hace de verdad antes del resize bueno.
			obj.Resize(anchoEstrecho)
			obj.Resize(fyne.NewSize(854, 744))
			for _, s := range textosDe(obj) {
				if s == "…" || (strings.HasSuffix(s, "…") && len([]rune(s)) <= 2) {
					t.Errorf("tras colocar dos veces el rótulo es %q: se trunca sobre lo truncado", s)
				}
			}
		})
	}
	// Y lo concreto: el título de la lista sigue entero.
	cab := cabeceraDe(l)
	cab.Resize(anchoEstrecho)
	cab.Resize(fyne.NewSize(854, AltoCabeceraLista))
	if !contiene(textosDe(cab), l.Modelo.Etiqueta) {
		t.Errorf("el título de la lista no sobrevive a dos colocaciones: %v", textosDe(cab))
	}
}

// TestLosBotonesDelCarrilSonPerfiles: «Ir a…» y «Sytue AI» son perfiles sobre la tinta
// del carril con su velo (`bg-chrome-hover`), no cajas rellenas. Se mira el píxel del
// CENTRO de cada uno —fuera del texto, en el hueco de la izquierda— y tiene que ser el
// velo del menú compuesto sobre el carril.
//
// VISTO EN ROJO (frontal, 2026-09-23): con el relleno transparente de antes, `ui.Caja`
// deja ver su rectángulo de borde y el centro de «Ir a…» sale del color de
// `chrome-border` sobre el carril.
func TestLosBotonesDelCarrilSonPerfiles(t *testing.T) {
	carril := sobre(tema.Color("sidebar-hover-bg", tema.Claro), tema.Color("sidebar-bg", tema.Claro))
	for nombre, w := range map[string]fyne.CanvasObject{
		"paleta": NuevoBotonPaleta("Ir a…", "⌘K", "", nil),
		"ia":     NuevoBotonIA("Sytue AI", nil),
	} {
		v := test.NewWindow(w)
		v.SetPadded(false)
		v.Resize(fyne.NewSize(238, 32))
		img := v.Canvas().Capture()
		v.Close()
		r, g, b, _ := img.At(img.Bounds().Dx()/16+4, img.Bounds().Dy()/2).RGBA()
		if dif(r>>8, uint32(carril.R))+dif(g>>8, uint32(carril.G))+dif(b>>8, uint32(carril.B)) > 6 {
			t.Errorf("%s: el interior pinta (%d,%d,%d) y el velo sobre el carril es (%d,%d,%d)", nombre, r>>8, g>>8, b>>8,
				carril.R, carril.G, carril.B)
		}
	}
}

// TestElGalonDeUnAreaVaALaDerecha: el `›` de un área va pegado a la derecha también
// después de repintar.
//
// VISTO EN ROJO (frontal, 2026-09-23): sin volver a colocar el galón nuevo que crea
// `Refresh`, se pinta en (0, 0), encima del nombre del área.
func TestElGalonDeUnAreaVaALaDerecha(t *testing.T) {
	c := NuevaCabeceraArea("Ventas", false, nil)
	c.Resize(fyne.NewSize(anchoEntrada, AltoCabeceraArea))
	r := test.WidgetRenderer(c)
	r.Refresh()
	galon := r.Objects()[len(r.Objects())-1]
	if x := galon.Position().X; x < anchoEntrada/2 {
		t.Errorf("el galón está en x %v; va a la derecha (%v)", x, anchoEntrada-EntradaPX-LadoGalonArea)
	}
}

// TestA360HayUnaSolaHamburguesa: por debajo de 768 el cajón del ERP se abre con su
// botón de `main` (`#erp-cajon-boton`), «el único mando de navegación» de la web. El
// armazón ponía además su barra móvil con otra hamburguesa (visto en una ventana real
// el 2026-09-27). VISTO EN ROJO sin `SinBarraMovil`.
func TestA360HayUnaSolaHamburguesa(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Ventana = fyne.NewSize(360, 740)
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	w.SetPadded(false)
	w.Resize(p.Ventana)
	var cajones, barras int
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		switch o.(type) {
		case *BotonCajon:
			cajones++
		case *ui.BarraMovil:
			barras++
		}
	})
	if cajones != 1 || barras != 0 {
		t.Errorf("a 360 hay %d botones del cajón del ERP y %d barras móviles; quiero 1 y 0", cajones, barras)
	}
}
