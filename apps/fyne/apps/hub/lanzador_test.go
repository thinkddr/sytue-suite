// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// La ventana con la que se midió la web: 1280×800, es-ES, tema claro, DSF 1.
const (
	ventana = 1280
	// Números MEDIDOS en producción (`medidas/portal/inicio.ancho.json`). Son el
	// contrato: los guardianes comparan contra ELLOS, no contra lo que calcula este
	// código.
	cajaX        = 24         // px-6, Chromium remoto/Figtree/rem16, 2026-10-04
	cajaAncho    = 1232       // 1280 − 2 × 24
	altoCabecera = 64         // sobretítulo16 + mt-1.5=6 + h1=42
	colTesela    = 236.796875 // cinco columnas con gap-3
	altoTesela   = 125.875    // descripción en una línea
	colTarjeta   = 394.65625  // tres columnas con gap-6
	altoAccesos  = 494        // ocho filas
	altoRecient  = 471        // seis filas
	altoHoyVacio = 165        // sin eventos y con pie de correo
)

// cerca compara con la tolerancia de la cuantización a 1/64 del navegador.
func cerca(a, b float32) bool { return a-b < 0.02 && b-a < 0.02 }

// lanzadorDePrueba monta la portada con los datos de la captura de producción —seis
// recientes y la agenda sin eventos— y las dieciséis teselas actuales, incluido
// Sytue ID, como el lanzador web. En cinco columnas ocupan cuatro filas.
func lanzadorDePrueba(t *testing.T) *Lanzador {
	t.Helper()
	l := NuevoLanzador(traductor(t), time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	// La suite menos el Hub y Sytue Admin, como el lanzador real.
	for _, p := range Productos {
		switch p.Marca.Clave() {
		case "one", "console":
			continue
		}
		l.Apps = append(l.Apps, p)
	}
	l.Agenda = Agenda{Estado: SinNada, HayCorreo: true, SinLeer: 17}
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	for i := range 6 {
		l.Reciente.Items = append(l.Reciente.Items, api.HubHit{
			Source: "drive", ID: fmt.Sprint(i), Title: "Auditoría · no borrar (32).odt",
			Snippet: "1.0 KiB", URL: "https://drive.sytue.eu/", Timestamp: &ayer,
		})
	}
	l.Reciente.Estado = Listo
	return l
}

// pinta mete la portada en una ventana sin cromo y devuelve la captura.
func pinta(t *testing.T, l *Lanzador, ancho float32) (image.Image, *lanzadorRenderer) {
	t.Helper()
	w := test.NewWindow(l)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	r := test.WidgetRenderer(l).(*lanzadorRenderer)
	l.Resize(fyne.NewSize(ancho, r.alto(ancho)))
	w.Resize(fyne.NewSize(ancho, r.alto(ancho)))
	return w.Canvas().Capture(), r
}

// TestLaPortadaMideLoQueMideLaWeb es el guardián de geometría: compara la medida
// DECLARADA de cada pieza con el número que se sacó del DOM de producción.
//
// Visto en ROJO a mano el 2026-09-22: poniendo `EscalonDescripcion.Linea` a 15,75 —el
// interlineado de la escala en vez del de `leading-snug`— la tesela pasa a medir
// 115,5 y el test lo dice con los dos números.
func TestLaPortadaMideLoQueMideLaWeb(t *testing.T) {
	l := lanzadorDePrueba(t)
	_, r := pinta(t, l, ventana)

	x, ancho := CajaContenido(ventana)
	if x != cajaX || ancho != cajaAncho {
		t.Errorf("la caja de contenido es x=%v w=%v y en producción x=%v w=%v",
			x, ancho, cajaX, cajaAncho)
	}
	if got := r.cabecera.alto(ancho); !cerca(got, altoCabecera) {
		t.Errorf("la cabecera mide %v y en producción %v", got, altoCabecera)
	}

	rj := ui.RejillaTarjetas(ventana, HuecoRejilla, cortesApps)
	if n := rj.Columnas(ancho); n != 5 {
		t.Errorf("la rejilla tiene %d columnas y en producción 5", n)
	}
	if got := rj.AnchoColumna(ancho); !cerca(got, colTesela) {
		t.Errorf("la columna mide %v y en producción %v", got, colTesela)
	}
	if got := AltoTesela(1); !cerca(got, altoTesela) {
		t.Errorf("la tesela mide %v de alto y en producción %v", got, altoTesela)
	}

	rt := ui.RejillaTarjetas(ventana, HuecoTarjetas, cortesTarjetas)
	if got := rt.AnchoColumna(ancho); !cerca(got, colTarjeta) {
		t.Errorf("la tarjeta mide %v de ancho y en producción %v", got, colTarjeta)
	}
	if got := r.tAccesos.alto(r.altoCuerpoAccesos()); !cerca(got, altoAccesos) {
		t.Errorf("«Accesos rápidos» mide %v y en producción %v", got, altoAccesos)
	}
	if got := r.recientes.alto(r.altoCuerpoReciente(ancho)); !cerca(got, altoRecient) {
		t.Errorf("«Reciente en toda la suite» mide %v y en producción %v", got, altoRecient)
	}
	if got := r.hoy.alto(r.altoCuerpoHoy()); !cerca(got, altoHoyVacio) {
		t.Errorf("«Hoy» mide %v y en producción %v", got, altoHoyVacio)
	}

	// El reparto de las tarjetas (web, PR #834): Hoy y Accesos apiladas en la primera
	// columna, Reciente arriba en las otras dos. La cabecera de cada tarjeta está a un
	// `border` de su esquina. VISTO EN ROJO con la disposición vieja: Reciente caía en
	// x = 21 con 1238 de ancho.
	hoy, acc, rec := r.hoy.cabecera.Position(), r.tAccesos.cabecera.Position(), r.recientes.cabecera.Position()
	if !cerca(acc.X, hoy.X) || !cerca(acc.Y, hoy.Y+altoHoyVacio+HuecoTarjetas) {
		t.Errorf("«Accesos» en %v; tiene que ir debajo de «Hoy» (%v) con `gap-6`", acc, hoy)
	}
	if !cerca(rec.Y, hoy.Y) || !cerca(rec.X-tema.Borde, cajaX+colTarjeta+HuecoTarjetas) {
		t.Errorf("«Reciente» en %v; tiene que ir arriba a la derecha de «Hoy» (%v)", rec, hoy)
	}
	if got := r.recientes.cabecera.Size().Width + 2*tema.Borde; !cerca(got, cajaAncho-colTarjeta-HuecoTarjetas) {
		t.Errorf("«Reciente» mide %v de ancho y en la web 818,3 (`lg:col-span-2`)", got)
	}
}

// TestLaPortadaPintaLosTokens comprueba los PÍXELES, no las cuentas: el borde de una
// tesela tiene que ser el `--sytue-border` que se midió en el inspector, y su
// interior la superficie.
func TestLaPortadaPintaLosTokens(t *testing.T) {
	l := lanzadorDePrueba(t)
	img, r := pinta(t, l, ventana)
	guardar(t, img, "lanzador-claro")

	px := func(x, y int) color.NRGBA {
		c, g, b, a := img.At(x, y).RGBA()
		return color.NRGBA{R: uint8(c >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	}
	igual := func(a, b color.NRGBA) bool {
		d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
		return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
	}

	// El papel de fondo.
	if got, quiero := px(2, 2), tema.Color("bg", tema.Claro); !igual(got, quiero) {
		t.Errorf("el fondo pinta %v y el token `bg` es %v", got, quiero)
	}

	// El borde izquierdo de Mail: `border-subtle` (tinta al 7 %) sobre la superficie
	// blanca de la tarjeta de recurso, que ya no lleva el tinte de la marca.
	tesela := r.teselas[0]
	bx := int(tesela.Position().X)
	by := int(tesela.Position().Y + tesela.Size().Height/2)
	produccion := color.NRGBA{R: 238, G: 239, B: 238, A: 255}
	if got := px(bx, by); !igual(got, produccion) {
		t.Errorf("el borde de la tesela en (%d,%d) pinta %v y en producción es %v",
			bx, by, got, produccion)
	}
	if got, quiero := px(bx+1, by), (color.NRGBA{R: 255, G: 255, B: 255, A: 255}); !igual(got, quiero) {
		t.Errorf("dentro del borde pinta %v y la superficie es %v", got, quiero)
	}
}

// TestLaSeccionEsUnaTarjetaBlancaQueNoSePulsa: las tres secciones del lanzador son
// `Card` —la superficie blanca, sin el gris ni el azul pastel al pasar el puntero que
// tuvo `.hub-section-card`— y siguen siendo contenedores: ni se pulsan ni se enfocan, y
// no tapan a los controles de dentro.
func TestLaSeccionEsUnaTarjetaBlancaQueNoSePulsa(t *testing.T) {
	l := lanzadorDePrueba(t)
	var destino Destino
	l.AlIr = func(d Destino) { destino = d }
	_, r := pinta(t, l, ventana)
	for _, tarjeta := range []*tarjeta{r.hoy, r.tAccesos, r.recientes} {
		if _, pulsable := any(tarjeta).(fyne.Tappable); pulsable {
			t.Fatal("la sección se ha convertido en un botón")
		}
		if _, enfocable := any(tarjeta).(fyne.Focusable); enfocable {
			t.Fatal("la sección añade una parada de teclado sin acción")
		}
		if tarjeta.fondo() != "surface" {
			t.Errorf("la sección pinta `%s` y `Card` es la superficie blanca", tarjeta.fondo())
		}
	}
	r.accesos[0].MouseIn(nil)
	test.Tap(r.accesos[0])
	if destino != ABuscar {
		t.Error("la superficie de sección bloquea el control hijo")
	}
	// El color de grupo va en el icono de cada acceso, por turno, sobre relleno sólido.
	for i, a := range r.accesos {
		if a.Tono != tonosAcceso[i%len(tonosAcceso)] {
			t.Errorf("el acceso %d lleva el tono %q y por turno le toca %q", i, a.Tono, tonosAcceso[i%len(tonosAcceso)])
		}
		caja := test.WidgetRenderer(a).(*accesoRenderer).caja
		relleno, _ := ui.TonoSolido(a.Tono, tema.Claro)
		objs := caja.Objetos()
		if got := objs[len(objs)-1].(*canvas.Rectangle).FillColor; got != color.Color(relleno) {
			t.Errorf("el icono del acceso %d pinta %v y el sólido de su tono es %v", i, got, relleno)
		}
	}
}

// TestLaPortadaEnSusSeisEstados pinta los seis y comprueba que cada uno enseña algo
// distinto: un estado que se pinta igual que otro no informa de nada.
//
// Las capturas quedan en `SYTUE_CAPTURA` para mirarlas de verdad — una pantalla no se
// da por buena con números solamente.
func TestLaPortadaEnSusSeisEstados(t *testing.T) {
	estados := []struct {
		nombre string
		e      Estado
	}{
		{"carga", Cargando},
		{"exito", Listo},
		{"vacio", SinNada},
		{"error", Fallo},
		{"sin-conexion", SinConexion},
		{"prohibido", Prohibido},
	}
	visto := map[string]string{}
	for _, caso := range estados {
		t.Run(caso.nombre, func(t *testing.T) {
			l := lanzadorDePrueba(t)
			l.Reciente.Estado = caso.e
			l.Agenda.Estado = caso.e
			if caso.e != Listo {
				l.Reciente.Items = nil
			}
			img, r := pinta(t, l, ventana)
			guardar(t, img, "lanzador-"+caso.nombre)

			// La rejilla de productos se pinta SIEMPRE: es lo que hace que el Hub
			// siga sirviendo de puerta aunque el servicio esté caído.
			if len(r.teselas) != len(l.Apps) || len(l.Apps) != 17 {
				t.Errorf("hay %d teselas de %d apps y tiene que haber 17 en cualquier estado", len(r.teselas), len(l.Apps))
			}
			// El alto que distingue es el de la tarjeta de lo reciente, no el de la
			// página: desde que va al lado de Hoy y Accesos (PR #834 de la web), la
			// columna de la izquierda es la más alta y la página mide lo mismo.
			_, ancho := CajaContenido(ventana)
			rt := ui.RejillaTarjetas(ventana, HuecoTarjetas, cortesTarjetas)
			firma := fmt.Sprintf("esq=%d filas=%d aviso=%v reciente=%.2f",
				len(r.esqRec), len(r.filas), r.avisoRec != nil,
				r.recientes.alto(r.altoCuerpoReciente(ancho-rt.Paso(ancho))))
			if otro, repe := visto[firma]; repe && caso.e != Fallo {
				t.Errorf("el estado %q se pinta igual que %q: %s", caso.nombre, otro, firma)
			}
			visto[firma] = caso.nombre
		})
	}
}

// TestLaRejillaSigueLosCortesDeLaVentana: `sm:` y `lg:` miden la VENTANA y con el rem
// inicial de 16, no la caja ni el rem de 14 de la suite.
func TestLaRejillaSigueLosCortesDeLaVentana(t *testing.T) {
	for _, caso := range []struct {
		ventana float32
		cols    int
	}{{360, 2}, {390, 2}, {639, 2}, {640, 3}, {1023, 3}, {1024, 5}, {1280, 5}} {
		_, ancho := CajaContenido(caso.ventana)
		if n := ui.RejillaTarjetas(caso.ventana, HuecoRejilla, cortesApps).Columnas(ancho); n != caso.cols {
			t.Errorf("a %v de ventana salen %d columnas y la web pone %d", caso.ventana, n, caso.cols)
		}
	}
}

// guardar deja el PNG en el DIRECTORIO `SYTUE_CAPTURAS` para poder MIRARLO. Sin la
// variable no escribe nada: un test no ensucia el árbol de quien lo corre.
//
// En plural a propósito: `SYTUE_CAPTURA` (singular) ya la usan `pantalla` y `ui` para
// UN fichero, y correr el módulo entero con la misma variable los tumba a los dos con
// «is a directory». Aquí son siete PNG, uno por estado.
func guardar(t *testing.T, img image.Image, nombre string) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		return
	}
	f, err := os.Create(dir + "/" + nombre + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestLaPortadaRespetaLoQueEligioLaOrganizacion: un producto dado a una aplicación de
// terceros lo dice en su tesela y abre ESA aplicación —desde la tesela y desde su
// acceso rápido, sin la ruta de Sytue—; y las aplicaciones conectadas salen en su propia
// rejilla, «Conectado», que empuja todo lo de debajo lo que mide.
//
// Visto en ROJO a mano el 2026-09-23 (en el frontal): con `abrirProducto` sin mirar los
// sustitutos, la tesela de Calcus abre la app nativa y el test lo dice.
func TestLaPortadaRespetaLoQueEligioLaOrganizacion(t *testing.T) {
	l := lanzadorDePrueba(t)
	sin := test.WidgetRenderer(l).(*lanzadorRenderer).alto(ventana)

	var abiertas []string
	var saltos []string
	l.AlAbrirURL = func(u string) { abiertas = append(abiertas, u) }
	l.AlAbrir = func(p Producto, ruta string) { saltos = append(saltos, p.Marca.Clave()+ruta) }
	l.Sustitutos = map[string]api.Sustituto{"sheets": {Name: "Google", Href: "https://sheets.google.com"}}
	svg := base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="4" fill="#e03"/></svg>`))
	l.Conectadas = []api.AppConectada{{
		Key: "i1", Name: "Holded", Description: "Facturación y contabilidad",
		Href: "https://app.holded.com", IconURL: "data:image/svg+xml;base64," + svg,
	}}
	l.Refresh()
	img, r := pinta(t, l, ventana)
	guardar(t, img, "inicio-conectadas")

	for _, x := range r.teselas {
		if x.Producto.Marca.Clave() != "sheets" {
			continue
		}
		if !strings.HasSuffix(x.Descripcion, " · Google") {
			t.Errorf("la tesela de Calcus dice «%s» y no nombra a Google", x.Descripcion)
		}
		x.Tapped(nil)
	}
	if len(abiertas) != 1 || abiertas[0] != "https://sheets.google.com" || len(saltos) != 0 {
		t.Errorf("pulsar Calcus abre %v y salta a %v; tenía que abrir Google", abiertas, saltos)
	}
	if len(r.conectadas) != 1 || r.rotuloCon == nil {
		t.Fatalf("hay %d aplicaciones conectadas pintadas y la organización tiene una", len(r.conectadas))
	}
	r.conectadas[0].Tapped(nil)
	if abiertas[len(abiertas)-1] != "https://app.holded.com" {
		t.Errorf("pulsar Holded abre %v", abiertas[len(abiertas)-1])
	}
	quiero := sin + tema.TextoXS.Linea + MargenRotulo + AltoTeselaConectada(1) + MargenSeccion
	if got := r.alto(ventana); !cerca(got, quiero) {
		t.Errorf("con una conectada la portada mide %v y tiene que medir %v", got, quiero)
	}
}

// enMarco pone la forma Marco para un test: los guardianes de este paquete corren en
// Carril, que es con la que se midieron.
func enMarco(t *testing.T) {
	t.Helper()
	tema.ElegirForma("marco")
	t.Cleanup(tema.FormaDeLosGuardianes)
}

// hojaA es lo que mide la portada en Marco con esa ventana: la hoja se separa 10 del
// borde por los dos lados, salvo por debajo de 768, donde va a sangre.
func hojaA(ventana float32) float32 {
	if ventana < ui.Estrecho {
		return ventana
	}
	return ventana - 2*tema.Forma("sheet-gap")
}

// finDelCuerpo es dónde acaba lo último que hay colocado en la portada de Marco.
func finDelCuerpo(r *lanzadorRenderer) float32 {
	fin := func(o fyne.CanvasObject) float32 { return o.Position().Y + o.Size().Height }
	ultima := fin(r.teselas[len(r.teselas)-1])
	if !r.l.dosColumnas() {
		ultima += MargenSeccion
	}
	return max(ultima, fin(r.hoy), fin(r.recientes))
}

// TestLaPortadaDeMarcoEsBienvenidaYTablero: en la forma Marco el Hub es otra portada (las
// variantes `marco:` de `Launcher.tsx`). Saluda por la hora en vez de decir «Tus
// aplicaciones»; desde 1024 va en dos columnas, 1,9 a 1 con 16 entre ellas —784 y 412 a
// 1280, medido por la web—; las apps son teselas compactas de 62 en tres columnas a 10;
// «Hoy» y «Reciente» van apiladas a la derecha, a 10, y «Reciente» enseña tres; y los
// accesos rápidos pierden su tarjeta y son las píldoras de bajo el saludo. Por debajo de
// 1024, una columna, con las teselas que quepan de 160.
//
// VISTO EN ROJO sin la rama de Marco en `Layout`: las teselas salen en cinco columnas de
// 126 de alto y «Hoy» debajo de ellas. Y con `MargenYMarco` en 32: el cuerpo arranca 12
// más abajo.
func TestLaPortadaDeMarcoEsBienvenidaYTablero(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/portal/src/components/Launcher.tsx")
	reciente := webref.Read(t, "apps/frontend/apps/portal/src/lib/reciente.ts")
	hoja := webref.Read(t, "apps/frontend/packages/config/src/theme.css")
	for _, c := range []struct{ fuente, trozo, porque string }{
		{hoja, `@custom-variant marco (:root:not([data-forma='carril']) &);`, "la variante `marco:`"},
		{hoja, `@custom-variant carril ([data-forma='carril'] &);`, "la variante `carril:`"},
		{web, `hora >= 5 && hora < 12 ? 'morning' : hora >= 12 && hora < 20 ? 'afternoon' : 'evening';`, "los tramos del saludo"},
		{web, `<div class="marco:py-5 mx-auto w-full max-w-[var(--container-content,1280px)] px-6 py-8">`, "el margen de la página"},
		{web, `<header class="marco:mb-3 mb-8">`, "el margen de la cabecera"},
		{web, `<span class="carril:hidden">{saludo}</span>`, "el saludo, solo en Marco"},
		{web, `<span class="marco:hidden">{t('launcher.heading')}</span>`, "«Tus aplicaciones», solo en Carril"},
		{web, `marco:lg:grid-cols-[minmax(0,1.9fr)_minmax(260px,1fr)] marco:lg:items-start marco:lg:gap-4`, "las dos columnas del cuerpo y lo que las separa"},
		{web, `class="marco:lg:last:mb-0 mb-8"`, "la última sección, sin margen en dos columnas"},
		{web, `marco:grid-cols-[repeat(auto-fill,minmax(160px,1fr))] marco:gap-2.5 marco:lg:grid-cols-3`, "la rejilla de apps"},
		{web, `rounded-card flex h-full flex-col items-start gap-3 border p-4 transition-colors`, "la tesela, con el radio de tarjeta"},
		{web, `'marco:flex-row marco:items-center marco:px-3 marco:py-2.5 marco:[&>span:first-child]:scale-105',`, "la tesela compacta"},
		{web, `marco:line-clamp-1 line-clamp-2 text-sm leading-snug`, "la descripción, a una línea"},
		{web, `'marco:gap-2.5 marco:lg:grid-cols-1 grid items-start gap-6 lg:grid-cols-3',`, "«Hoy» y «Reciente», apiladas a 10"},
		{web, "class={`marco:py-1 flex items-baseline gap-3 px-4 py-2 ${pasado ? 'opacity-60' : ''}`}", "el evento, a 4"},
		{web, `marco:py-2 flex items-center gap-3 border-t px-4 py-2.5 text-sm`, "el pie de correo, a 8"},
		{web, `{recientesVisibles(recent!, forma).map((item) => {`, "«Reciente» pinta solo las que tocan"},
		{reciente, `export const RECIENTES_EN_MARCO = 3;`, "«Reciente» enseña tres"},
		{reciente, `return forma === 'marco' ? recientes.slice(0, RECIENTES_EN_MARCO) : recientes;`, "y las demás no se pintan"},
		{web, `marco:py-2 flex items-start gap-3 px-4 py-3 transition-colors`, "la fila de «Reciente», a 8"},
		{web, `<section aria-label={t('launcher.quickActions')} class="marco:hidden min-w-0">`, "los accesos, sin tarjeta en Marco"},
	} {
		if !strings.Contains(c.fuente, c.trozo) {
			t.Errorf("la web ya no dice %q (%s): la portada de Marco de aquí ha dejado de ser la suya", c.trozo, c.porque)
		}
	}
	for hora, quiero := range map[int]string{0: "evening", 4: "evening", 5: "morning", 11: "morning", 12: "afternoon", 19: "afternoon", 20: "evening", 23: "evening"} {
		if got := MomentoDelDia(hora); got != quiero {
			t.Errorf("a las %d el saludo es %q y la web dice %q", hora, got, quiero)
		}
	}

	enMarco(t)
	l := lanzadorDePrueba(t) // a las 12:00
	tarde := l.T.T("launcher.greeting.afternoon", nil)
	if got := l.titulo(); got != tarde || tarde == "launcher.greeting.afternoon" {
		t.Errorf("sin nombre el título es %q; es el saludo solo (%q)", got, tarde)
	}
	l.Nombre = "Ana"
	_, r := pinta(t, l, hojaA(1280))
	if got := r.cabecera.rotulo; got != tarde+", Ana" {
		t.Errorf("el título es %q y a las 12 es %q", got, tarde+", Ana")
	}
	if len(r.accesos) != 0 || r.tAccesos != nil {
		t.Error("en Marco los accesos rápidos no llevan tarjeta")
	}

	// Dos columnas: 1196 de la caja de 1212 menos los 16, a 1,9 y 1.
	x, ancho := CajaContenido(hojaA(1280))
	principal, lado := columnasMarco(ancho)
	if x != 24 || ancho != 1212 || principal < 783 || principal > 784 || lado < 412 || lado > 413 {
		t.Fatalf("la caja está en %v y mide %v, con columnas de %v y %v; la web mide 24, 1212, 784 y 412", x, ancho, principal, lado)
	}
	a, b, c, d := r.teselas[0], r.teselas[1], r.teselas[2], r.teselas[3]
	if a.Size().Height != 62 || a.Position().X != x || b.Position().Y != a.Position().Y || c.Position().Y != a.Position().Y || d.Position().Y == a.Position().Y {
		t.Errorf("las teselas miden %v de alto y la cuarta está en %v; son de 62 y van de tres en tres", a.Size().Height, d.Position())
	}
	if hueco := b.Position().X - a.Position().X - a.Size().Width; !cerca(hueco, 10) {
		t.Errorf("entre teselas hay %v y son 10", hueco)
	}
	if salto := d.Position().Y - a.Position().Y - a.Size().Height; !cerca(salto, 10) {
		t.Errorf("entre filas de teselas hay %v y son 10", salto)
	}
	if fin := c.Position().X + c.Size().Width; fin > x+principal+0.01 || fin < x+principal-1 {
		t.Errorf("la tercera tesela acaba en %v y la columna en %v", fin, x+principal)
	}
	hoy, rec := r.hoy, r.recientes
	if !cerca(hoy.Position().X, x+principal+16) || !cerca(hoy.Size().Width, lado) || rec.Position().X != hoy.Position().X || !cerca(rec.Size().Width, lado) {
		t.Errorf("«Hoy» está en %v con %v de ancho y «Reciente» en %v con %v; van a la derecha, en la columna de %v", hoy.Position(), hoy.Size().Width, rec.Position(), rec.Size().Width, lado)
	}
	// La página empieza a 20 (`marco:py-5`); bajo el saludo, a 16, las píldoras de 32; y a
	// 12 (`marco:mb-3`) el cuerpo, con las dos columnas arrancando a la vez.
	tira := 20 + r.cabecera.alto(ancho) + 16
	cuerpo := tira + 32 + 12
	if got := r.tiraPildoras.Position(); !cerca(got.X, x) || !cerca(got.Y, tira) {
		t.Errorf("las píldoras están en %v y van en (%v, %v)", got, x, tira)
	}
	if !cerca(hoy.Position().Y, cuerpo) || !cerca(a.Position().Y, cuerpo+tema.TextoXS.Linea+MargenRotulo) {
		t.Errorf("«Hoy» empieza en %v y las teselas en %v; el cuerpo arranca en %v, y las teselas bajo su rótulo", hoy.Position().Y, a.Position().Y, cuerpo)
	}
	if hueco := rec.Position().Y - hoy.Position().Y - hoy.Size().Height; !cerca(hueco, 10) {
		t.Errorf("entre «Hoy» y «Reciente» hay %v y son 10", hueco)
	}
	// «Reciente» enseña tres de las seis, a 8 por arriba y por abajo; el evento va a 4 y
	// el pie de correo a 8.
	if len(r.filas) != 3 || !cerca(MedidaReciente(true), 16+tema.TextoRaiz.Linea+tema.TextoSM.Linea) {
		t.Errorf("«Reciente» enseña %d filas de %v; son tres, con 8 de margen", len(r.filas), MedidaReciente(true))
	}
	if !cerca(MedidaEvento(1), 8+tema.TextoSM.Linea) || !cerca(MedidaCorreo(), tema.Borde+16+max(iconos.MarcaXS, tema.TextoSM.Linea)) {
		t.Errorf("el evento mide %v y el pie de correo %v; van a 4 y a 8 de margen", MedidaEvento(1), MedidaCorreo())
	}
	if got, quiero := finDelCuerpo(r)+r.altoPie(), r.alto(hojaA(1280)); !cerca(got, quiero) {
		t.Errorf("la portada acaba en %v y declara %v", got, quiero)
	}

	// La tesela compacta: la marca a la izquierda, al 105 %, y los dos textos a su derecha.
	tr := test.WidgetRenderer(a).(*teselaRenderer)
	marca, nombre, desc := tr.marca.Objeto(), tr.nombre, tr.desc[0]
	if marca.Size() != fyne.NewSize(42, 42) || !cerca(marca.Position().X, 1+12-1) || !cerca(marca.Position().Y, (62-42)/2) {
		t.Errorf("la marca mide %v y está en %v; son 42, a 12 del borde y centrada en los 62", marca.Size(), marca.Position())
	}
	if nombre.Position().X < 1+12+40+12-0.01 || desc.Position().X != nombre.Position().X || desc.Position().Y <= nombre.Position().Y || tr.desc[1].Visible() {
		t.Errorf("el nombre está en %v y la descripción en %v; van a 12 de la marca, una sobre otra y a una línea", nombre.Position(), desc.Position())
	}

	// Por debajo de 1024, una columna: las apps y, a su `mb-8`, «Hoy» y «Reciente».
	estrecha := lanzadorDePrueba(t)
	_, re := pinta(t, estrecha, hojaA(1000))
	xe, anchoE := CajaContenido(hojaA(1000))
	ultima := re.teselas[len(re.teselas)-1]
	if re.hoy.Position().X != xe || !cerca(re.hoy.Size().Width, anchoE) || !cerca(re.hoy.Position().Y, ultima.Position().Y+62+32) {
		t.Errorf("a 1000 «Hoy» está en %v con %v de ancho; va a 32 bajo las apps (%v), a todo el ancho (%v)", re.hoy.Position(), re.hoy.Size().Width, ultima.Position().Y+62, anchoE)
	}
	if cols := re.rejillaApps(hojaA(1000)).Columnas(anchoE); cols != 5 {
		t.Errorf("a 1000 caben %d columnas de 160 y son 5", cols)
	}
	if got, quiero := finDelCuerpo(re)+re.altoPie(), re.alto(hojaA(1000)); !cerca(got, quiero) {
		t.Errorf("a 1000 la portada acaba en %v y declara %v", got, quiero)
	}
	for ventana, quiero := range map[float32]int{390: 2, 360: 1} {
		movil := lanzadorDePrueba(t)
		movil.Nombre = "Maximiliano Alejandro"
		_, rm := pinta(t, movil, hojaA(ventana))
		_, an := CajaContenido(hojaA(ventana))
		if cols := rm.rejillaApps(hojaA(ventana)).Columnas(an); cols != quiero {
			t.Errorf("a %v las apps van en %d columnas y la web las pone en %d", ventana, cols, quiero)
		}
		// El saludo no cabe en una línea y se parte, como el `<h1>` de la web.
		if n := len(rm.cabecera.titulo); n < 2 {
			t.Errorf("a %v el saludo con un nombre largo ocupa %d líneas y no cabe en una", ventana, n)
		}
		for _, linea := range rm.cabecera.titulo {
			if w := ui.Medir(linea.Text, ui.EscalonTitulo(1), ui.PesoTitulo(1)).Width; w > an {
				t.Errorf("a %v la línea %q del saludo mide %v y la página %v", ventana, linea.Text, w, an)
			}
		}
		// Y las píldoras bajan de línea (`flex-wrap`), a 8, sin salirse.
		if rm.altoPildoras < 2*32+8 {
			t.Errorf("a %v las píldoras ocupan %v de alto: no han bajado de línea", ventana, rm.altoPildoras)
		}
		for _, p := range rm.pildoras {
			if fin := p.Position().X + p.Size().Width; fin > an+0.01 {
				t.Errorf("a %v una píldora acaba en %v y la página mide %v", ventana, fin, an)
			}
		}
	}
}

// TestTodoElDiaOcupaDosLineas: la hora de un evento es un `w-12` sin `whitespace-nowrap`,
// y «Todo el día» no cabe: «día» baja a una segunda línea y la fila crece —41 en Marco,
// medido en la web— con el resumen arriba. Con un evento de día entero, cinco con hora y
// el pie de correo, «Hoy» mide los 297 de la web a 1280. La cuenta de auditoría no tiene
// eventos, así que esto solo se ve aquí.
//
// VISTO EN ROJO con la hora en una sola línea: la fila mide 26 y «Hoy» 282.
func TestTodoElDiaOcupaDosLineas(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/portal/src/components/Launcher.tsx")
	for _, trozo := range []string{
		`<span class="text-text-muted w-12 shrink-0 text-xs tabular-nums">`,
		`{e.allDay ? t('today.allDay') : formatTime(locale, e.start)}`,
		"class={`marco:py-1 flex items-baseline gap-3 px-4 py-2 ${pasado ? 'opacity-60' : ''}`}",
	} {
		if !strings.Contains(web, trozo) {
			t.Errorf("la web ya no dice %q: la fila de evento de aquí ha dejado de ser la suya", trozo)
		}
	}
	es, err := i18n.New("es", i18n.Common(), catalogos.Portal())
	if err != nil {
		t.Fatal(err)
	}
	conEventos := func() *Lanzador {
		l := lanzadorDePrueba(t)
		l.T = es
		a := func(hora, minuto int) time.Time { return time.Date(2026, 9, 21, hora, minuto, 0, 0, time.UTC) }
		l.Agenda = Agenda{Estado: Listo, HayCorreo: true, SinLeer: 3, Eventos: []EventoDeHoy{
			{ID: "1", Resumen: "Reunión de equipo", Inicio: a(9, 30), Fin: a(10, 0)},
			{ID: "0", Resumen: "Cierre del trimestre", TodoElDia: true},
			{ID: "2", Resumen: "Revisión del presupuesto 2027", Inicio: a(11, 0), Fin: a(11, 30)},
			{ID: "3", Resumen: "Comida con Marta Soler", Inicio: a(14, 0), Fin: a(15, 0)},
			{ID: "4", Resumen: "Demo del piloto para Cooperativa Norte", Inicio: a(16, 30), Fin: a(17, 0)},
			{ID: "5", Resumen: "Formación: firma con certificado", Inicio: a(18, 0), Fin: a(19, 0)},
		}}
		return l
	}

	enMarco(t)
	_, r := pinta(t, conEventos(), hojaA(1280))
	entero, conHora := r.filasHoy[0].(*FilaEvento), r.filasHoy[1].(*FilaEvento)
	er := test.WidgetRenderer(entero).(*eventoRenderer)
	if len(er.hora) != 2 || er.hora[0].Text != "Todo el" || er.hora[1].Text != "día" {
		t.Fatalf("la hora del evento de día entero ocupa %d líneas; son «Todo el» y «día»", len(er.hora))
	}
	if salto := er.hora[1].Position().Y - er.hora[0].Position().Y; !cerca(salto, tema.TextoXS.Linea) {
		t.Errorf("entre las dos líneas de la hora hay %v y es su interlineado, %v", salto, tema.TextoXS.Linea)
	}
	if got := entero.Size().Height; !cerca(got, 41) || !cerca(conHora.Size().Height, 26) {
		t.Errorf("la fila de día entero mide %v y la de una hora %v; en la web son 41 y 26", got, conHora.Size().Height)
	}
	if n := len(test.WidgetRenderer(conHora).(*eventoRenderer).hora); n != 1 {
		t.Errorf("«9:30» ocupa %d líneas y cabe en una", n)
	}
	if got := r.hoy.Size().Height; got < 296 || got > 298 {
		t.Errorf("«Hoy» mide %v con un evento de día entero, cinco con hora y el correo; en la web son 297", got)
	}

	// Es de la fila y no de la forma: en Carril parte igual, con su `py-2`.
	tema.FormaDeLosGuardianes()
	_, rc := pinta(t, conEventos(), ventana)
	if got := rc.filasHoy[0].(*FilaEvento).Size().Height; !cerca(got, 16+33) {
		t.Errorf("en Carril la fila de día entero mide %v y son 49", got)
	}
}

// TestLasAccionesDelDiaSonPildorasConMas: en Marco los accesos rápidos son cuatro
// píldoras —la primera, la principal, con su icono— y un «Más» que despliega las otras
// tres. «Buscar» no está: lo lleva la barra. Con el teclado, Intro abre y enfoca la
// primera opción, las flechas recorren y Escape cierra y devuelve el foco a «Más».
//
// VISTO EN ROJO quitando `AlInicio` (el menú se alinea por la derecha) y dándole a
// `Mostrar` otro disparador (Escape deja el foco en el aire).
func TestLasAccionesDelDiaSonPildorasConMas(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/portal/src/components/Launcher.tsx")
	for _, trozo := range []string{
		`const ACCIONES_DEL_DIA = ['quick.compose', 'quick.newDoc', 'quick.upload', 'quick.newEvent'];`,
		`const ACCIONES_EN_MAS = ['quick.download', 'quick.devices', 'quick.data'];`,
		`'rounded-control border-border bg-surface text-text hover:border-border-strong sytue-focus-ring inline-flex h-8 items-center gap-1 border px-3.5 text-sm font-semibold shadow-[var(--shadow-xs)] transition-colors pointer-coarse:min-h-[max(44px,2.75rem)]';`,
		`class="carril:hidden mt-4 flex flex-wrap gap-2"`,
		`<Button key={qa.clave} href={qa.href} size="sm" leadingIcon={qa.icon()}>`,
		`label={t('launcher.more')}`,
		`align="start"`,
		`<Icon name="chevronDown" size={14} />`,
		`<MenuItem key={qa.clave} href={qa.href} icon={qa.icon()} onClick={cerrar}>`,
	} {
		if !strings.Contains(web, trozo) {
			t.Errorf("la web ya no dice %q: las píldoras de aquí han dejado de ser las suyas", trozo)
		}
	}

	enMarco(t)
	l := lanzadorDePrueba(t)
	var abiertos []string
	l.AlAbrir = func(p Producto, ruta string) { abiertos = append(abiertos, p.Marca.Clave()+ruta) }
	var idos []Destino
	l.AlIr = func(d Destino) { idos = append(idos, d) }
	_, r := pinta(t, l, hojaA(1280))
	tr := l.T.T

	if len(r.pildoras) != 5 {
		t.Fatalf("hay %d píldoras y son cuatro y «Más»", len(r.pildoras))
	}
	principal, es := r.pildoras[0].(*ui.Boton)
	if !es || principal.Texto != tr("quick.compose", nil) || principal.Variante != ui.Primario || principal.Talla != ui.TallaSM || principal.Glifo == nil {
		t.Errorf("la primera píldora es %#v; es el botón principal de talla `sm` con su icono", r.pildoras[0])
	}
	var x float32
	for i, o := range r.pildoras {
		if o.Size().Height != 32 || !cerca(o.Position().X, x) || o.Position().Y != 0 {
			t.Errorf("la píldora %d mide %v y está en %v; son de 32, en una línea y a 8 (%v)", i, o.Size(), o.Position(), x)
		}
		x += o.Size().Width + 8
		if i == 0 {
			continue
		}
		p, es := o.(*PildoraDia)
		quiero := slices.Concat(AccionesDelDia, []string{"launcher.more"})[i]
		if !es || p.Rotulo != tr(quiero, nil) || p.Galon != (i == 4) {
			t.Errorf("la píldora %d es %#v; es %q, y solo «Más» lleva galón", i, o, tr(quiero, nil))
		}
	}
	// 14 de margen y el borde a cada lado del rótulo; «Más», además, el galón a 4.
	mas := r.pildoras[4].(*PildoraDia)
	if got, quiero := mas.MinSize().Width, 2+28+ui.Medir(mas.Rotulo, tema.TextoSM, ui.SemiNegrita).Width+4+14; !cerca(got, quiero) {
		t.Errorf("«Más» mide %v de ancho y son %v", got, quiero)
	}

	test.Tap(r.pildoras[1].(*PildoraDia))
	if len(abiertos) != 1 || abiertos[0] != "docs" {
		t.Errorf("«Nuevo documento» ha abierto %v; abre Docs", abiertos)
	}

	// El menú, con el teclado.
	c := fyne.CurrentApp().Driver().CanvasForObject(mas)
	c.Focus(mas)
	mas.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	for i, clave := range slices.Concat(AccionesEnMas, AccionesEnMas[:1]) { // las flechas circulan
		e, es := c.Focused().(*ui.ElementoMenu)
		if !es || e.Texto != tr(clave, nil) || e.Glifo == nil {
			t.Fatalf("tras %d flechas el foco está en %#v; quiero la opción %q, con su icono", i, c.Focused(), tr(clave, nil))
		}
		e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	}
	// Cuelga de «Más», alineado con su borde izquierdo (`align="start"`).
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject
	if got, quiero := abs(r.menu), abs(mas).AddXY(0, 32+ui.SeparacionMenu); !cerca(got.X, quiero.X) || !cerca(got.Y, quiero.Y) {
		t.Errorf("el menú se despliega en %v y cuelga de «Más» por la izquierda, en %v", got, quiero)
	}
	c.Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if n := len(c.Overlays().List()); n != 0 || c.Focused() != fyne.Focusable(mas) {
		t.Errorf("tras Escape quedan %d capas y el foco está en %T; cierra y vuelve a «Más»", n, c.Focused())
	}

	// Y con el ratón: «Dispositivos» lleva a su pantalla.
	test.Tap(mas)
	c.Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	c.Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(idos) != 1 || idos[0] != ADispositivos || len(c.Overlays().List()) != 0 {
		t.Errorf("«Dispositivos» ha ido a %v con %d capas abiertas; va a su pantalla y recoge el menú", idos, len(c.Overlays().List()))
	}
}

// TestConPocasAppsRecienteBajaASuColumna: con seis apps o menos —dos filas de tres— y en
// dos columnas, «Reciente» pasa a la columna de las apps, a todo su ancho y 16 bajo la
// rejilla, y a la derecha queda solo «Hoy». Con siete vuelve bajo «Hoy». En una columna
// el orden no cambia.
//
// VISTO EN ROJO con `PocasApps` en 5: con seis, «Reciente» se queda bajo «Hoy».
func TestConPocasAppsRecienteBajaASuColumna(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/portal/src/components/Launcher.tsx")
	for _, trozo := range []string{
		`const pocas = apps.length <= 6;`,
		`<div class={cx('min-w-0', pocas && 'marco:lg:col-start-1 marco:lg:row-start-1')}>`,
		`pocas && 'marco:lg:contents',`,
		`pocas && 'marco:lg:col-start-2 marco:lg:row-span-2 marco:lg:row-start-1',`,
		`pocas && 'marco:lg:col-start-1 marco:lg:row-start-2',`,
	} {
		if !strings.Contains(web, trozo) {
			t.Errorf("la web ya no dice %q: el reparto con pocas apps de aquí ha dejado de ser el suyo", trozo)
		}
	}

	enMarco(t)
	x, ancho := CajaContenido(hojaA(1280))
	principal, lado := columnasMarco(ancho)
	for n, baja := range map[int]bool{6: true, 7: false} {
		l := lanzadorDePrueba(t)
		l.Apps = l.Apps[:n]
		_, r := pinta(t, l, hojaA(1280))
		hoy, rec := r.hoy, r.recientes
		rejilla := r.teselas[n-1].Position().Y + 62
		if !cerca(hoy.Position().X, x+principal+16) || !cerca(hoy.Size().Width, lado) {
			t.Errorf("con %d apps «Hoy» está en %v con %v de ancho; va a la derecha", n, hoy.Position(), hoy.Size().Width)
		}
		if baja {
			if rec.Position().X != x || !cerca(rec.Size().Width, principal) || !cerca(rec.Position().Y, rejilla+16) {
				t.Errorf("con %d apps «Reciente» está en %v con %v de ancho; va bajo la rejilla (%v) a 16, con los %v de su columna", n, rec.Position(), rec.Size().Width, rejilla, principal)
			}
		} else if rec.Position().X != hoy.Position().X || !cerca(rec.Position().Y, hoy.Position().Y+hoy.Size().Height+10) {
			t.Errorf("con %d apps «Reciente» está en %v; va bajo «Hoy», a 10", n, rec.Position())
		}
		if got, quiero := finDelCuerpo(r)+r.altoPie(), r.alto(hojaA(1280)); !cerca(got, quiero) {
			t.Errorf("con %d apps la portada acaba en %v y declara %v", n, got, quiero)
		}
	}

	l := lanzadorDePrueba(t)
	l.Apps = l.Apps[:6]
	_, r := pinta(t, l, hojaA(1000))
	hoy, rec := r.hoy, r.recientes
	if !cerca(hoy.Position().Y, r.teselas[5].Position().Y+62+32) || !cerca(rec.Position().Y, hoy.Position().Y+hoy.Size().Height+10) {
		t.Errorf("a 1000 con seis apps «Hoy» está en %v y «Reciente» en %v; en una columna el orden no cambia", hoy.Position(), rec.Position())
	}
}

// Al rehacer la portada, las cabeceras todavía no tienen ancho. Medirlas como una
// fila hacía que la tarjeta terminara antes que su contenido en un teléfono: las
// últimas filas se veían sobre el color del lienzo en vez de sobre surface.
func TestLasTarjetasMovilesPintanPapelBajoTodoSuContenido(t *testing.T) {
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir(""); tema.FormaDeLosGuardianes() })
	es, err := i18n.New("es", i18n.Common(), catalogos.Portal())
	if err != nil {
		t.Fatal(err)
	}
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		for _, ancho := range []float32{320, 375, 390} {
			// El Scroll pregunta primero el mínimo y después coloca. No montar una
			// ventana aquí: sus repintados posteriores ya recuerdan el ancho y pueden
			// ocultar la primera medida corta de una cabecera recién creada.
			inicial := lanzadorDePrueba(t)
			inicial.T, inicial.ancho = es, ancho
			primero := inicial.CreateRenderer().(*lanzadorRenderer)
			altoInicial := primero.alto(ancho)
			primero.Layout(fyne.NewSize(ancho, altoInicial))
			if despues := primero.alto(ancho); !cerca(altoInicial, despues) {
				t.Errorf("%s a %g: el primer mínimo reserva%g pero tras colocar necesita%g", forma, ancho, altoInicial, despues)
			}
			for _, card := range []*tarjeta{primero.hoy, primero.recientes} {
				if got, want := card.Size().Height, card.alto(func() float32 {
					if card == primero.hoy {
						return primero.altoCuerpoHoy()
					}
					_, caja := CajaContenido(ancho)
					return primero.altoCuerpoReciente(caja)
				}()); !cerca(got, want) {
					t.Errorf("%s a %g: la primera tarjeta pinta%g pero su cabecera y cuerpo necesitan%g", forma, ancho, got, want)
				}
			}
			primero.Destroy()
			l := lanzadorDePrueba(t)
			l.T = es
			img, r := pinta(t, l, ancho)
			for _, card := range []*tarjeta{r.hoy, r.recientes} {
				if got, want := card.cabecera.Size().Height, ui.MedidaCabeceraApp(card.cabecera).Height; !cerca(got, want) {
					t.Errorf("%s a %g: la cabecera pinta%g pero necesita%g", forma, ancho, got, want)
				}
			}
			ultima := r.filas[len(r.filas)-1]
			if fin, borde := ultima.Position().Y+ultima.Size().Height, r.recientes.Position().Y+r.recientes.Size().Height; fin > borde-MargenTarjeta {
				t.Errorf("%s a %g: Reciente acaba en%g pero la última fila llega a%g", forma, ancho, borde, fin)
			}
			// En el margen del cuerpo, fuera de texto, iconos y bordes redondeados.
			x := int(r.recientes.Position().X + 8)
			y := int(ultima.Position().Y + ultima.Size().Height/2)
			cr, cg, cb, ca := img.At(x, y).RGBA()
			pixel := color.NRGBA{R: uint8(cr >> 8), G: uint8(cg >> 8), B: uint8(cb >> 8), A: uint8(ca >> 8)}
			if !parecido(pixel, tema.Color("surface", tema.Claro)) {
				t.Errorf("%s a %g: bajo la última fila hay%v en vez de surface", forma, ancho, pixel)
			}
			guardar(t, img, fmt.Sprintf("hub-papel-%s-%g", forma, ancho))
		}
	}
}

// TestEnCarrilLaPortadaEsLaDeSiempre: «Tus aplicaciones», los accesos en su tarjeta y las
// teselas en columna, con el radio de tarjeta de la forma.
func TestEnCarrilLaPortadaEsLaDeSiempre(t *testing.T) {
	l := lanzadorDePrueba(t)
	l.Nombre = "Ana"
	_, r := pinta(t, l, ventana)
	if got, quiero := r.cabecera.rotulo, l.T.T("launcher.heading", nil); got != quiero {
		t.Errorf("en Carril el título es %q y es %q", got, quiero)
	}
	if len(r.accesos) != 8 || r.tAccesos == nil || len(r.pildoras) != 0 {
		t.Errorf("en Carril hay %d accesos rápidos en su tarjeta y %d píldoras; son ocho y ninguna", len(r.accesos), len(r.pildoras))
	}
	if len(r.filas) != 6 || !cerca(MedidaReciente(true), 2*AltoReciente+tema.TextoRaiz.Linea+tema.TextoSM.Linea) {
		t.Errorf("en Carril «Reciente» enseña %d filas de %v; son las seis, a `py-3`", len(r.filas), MedidaReciente(true))
	}
	tr := test.WidgetRenderer(r.teselas[0]).(*teselaRenderer)
	if tr.marca.Objeto().Size() != fyne.NewSize(40, 40) || tr.nombre.Position().Y <= tr.marca.Objeto().Position().Y+40 {
		t.Error("en Carril la tesela va en columna: la marca de 40 arriba y el nombre debajo")
	}
}

// TestLaTarjetaDeProductoCambiaElBordeConElPuntero: `hover:border-border-strong` vuelve a
// aplicarse en la web (la regla de `.hub-product-card` ya no fija el borde), y aquí.
func TestLaTarjetaDeProductoCambiaElBordeConElPuntero(t *testing.T) {
	l := lanzadorDePrueba(t)
	_, r := pinta(t, l, ventana)
	x := r.teselas[0]
	reposo := x.borde(tema.Claro)
	x.MouseIn(nil)
	if encima := x.borde(tema.Claro); encima == reposo {
		t.Errorf("con el puntero encima el borde sigue en %v; quiero `border-strong`", encima)
	}
	x.MouseOut()
}

// ApplyThemeTo pide Layout antes de Refresh. La transición de Marco a Carril
// medía la tarjeta de accesos que la forma anterior todavía no había construido.
func TestLaRotacionDeFormaRecomponeElHubAntesDeMedir(t *testing.T) {
	enMarco(t)
	l := lanzadorDePrueba(t)
	l.Resize(fyne.NewSize(568, 320))
	r := test.WidgetRenderer(l).(*lanzadorRenderer)
	for _, forma := range []string{"carril", "marco", "carril"} {
		tema.ElegirForma(forma)
		r.Layout(l.Size())
		if (r.tAccesos == nil) != tema.EsMarco() || (r.tiraPildoras != nil) != tema.EsMarco() {
			t.Fatalf("%s: el Hub conserva la composición de la forma anterior", forma)
		}
	}
}
