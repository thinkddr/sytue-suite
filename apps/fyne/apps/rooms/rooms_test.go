// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"crypto/sha256"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

// Referencia independiente con rem de 16 (2026-10-04): Chromium, la hoja
// compartida real y las clases de ChannelRow.tsx, Composer.tsx y StatusMenu.tsx.
// Los altos fijos salen de h-14/h-8/min-h-[2.5rem]. Se escriben los valores
// observados, sin llamar a los helpers que este guardián comprueba.
const (
	filaCarrilMedida   = 40   // h-6 + 2 × py-2
	cabeceraMedida     = 56   // la banda del canal (`h-14`)
	anchoCabecera      = 1020 // la columna de contenido a 1280
	accionMedida       = 32   // cada botón de la cabecera y de la barra del compositor
	cajaCompositor     = 992  // ancho fijo del fixture, con su filete
	areaMedida         = 978  // el `textarea` dentro de ella
	altoAreaMedido     = 40   // su `min-h-[2.5rem]`
	glifoCabeceraMedio = 16   // los iconos de la cabecera
	glifoBarraMedido   = 18   // los del compositor
	anchoUtilCarril    = 231  // ancho fijo del fixture de ChannelRow.tsx
)

// escala es a la que se comprueban los píxeles: DOS, que es donde los medios píxeles
// de esta escala salen exactos (ver `ui/ui_test.go`).
const escala float32 = 2

func traductor(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// pinta devuelve la captura de un lienzo BLANCO de w×h px de CSS con los objetos
// encima, a ×2. Sin relleno: por defecto el lienzo mete el `padding` del tema y
// desplaza todo, que es como se lee mal una captura.
func pinta(t *testing.T, w, h float32, objs ...fyne.CanvasObject) image.Image {
	t.Helper()
	fondo := fcanvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	fondo.Resize(fyne.NewSize(w, h))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(append([]fyne.CanvasObject{fondo}, objs...)...))
	c.SetScale(escala)
	c.Resize(fyne.NewSize(w, h))
	return c.Capture()
}

func px(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// cerca compara dos colores con un canal de margen: cuantizar el alfa deja ±1 de 255.
func cerca(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// caja devuelve el rectángulo de píxeles de DISPOSITIVO donde aparece un color.
func caja(img image.Image, c color.NRGBA) (x0, x1, y0, y1 int) {
	b := img.Bounds()
	x0, x1, y0, y1 = 1<<30, -1, 1<<30, -1
	for y := range b.Dy() {
		for x := range b.Dx() {
			if cerca(px(img, x, y), c) {
				x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
			}
		}
	}
	return x0, x1, y0, y1
}

// tinta cuenta cuántos píxeles NO son del fondo. Es lo que distingue un estado que
// pinta algo de uno que no pinta nada.
func tinta(img image.Image, fondo color.NRGBA) int {
	n := 0
	b := img.Bounds()
	for y := range b.Dy() {
		for x := range b.Dx() {
			if !cerca(px(img, x, y), fondo) {
				n++
			}
		}
	}
	return n
}

// ---- Los glifos ----

// elemento saca cada forma autocerrada de un SVG: `<path d="…" />`, `<circle … />`,
// `<rect … />`. El `<svg …>` de apertura y el `<Glifo …>` de la web no casan porque no
// se autocierran, que es justo lo que hace falta.
var elemento = regexp.MustCompile(`<(\w+)\s+([^>]*?)\s*/>`)

// formas normaliza un cuerpo de SVG a la lista de sus figuras, con los espacios
// colapsados: así el mismo dibujo con otro reflow de Prettier sigue siendo igual, y un
// atributo cambiado sigue siendo distinto.
func formas(s string) []string {
	casos := elemento.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(casos))
	for _, m := range casos {
		out = append(out, m[1]+" "+strings.Join(strings.Fields(m[2]), " "))
	}
	return out
}

// cuerpoDeLaWeb saca el cuerpo de un glifo de `glyphs.tsx` por el nombre de su función.
func cuerpoDeLaWeb(t *testing.T, fuente, nombre string) string {
	t.Helper()
	i := strings.Index(fuente, "export function "+nombre+"(")
	if i < 0 {
		t.Fatalf("glyphs.tsx ya no exporta %s: el espejo hay que rehacerlo", nombre)
	}
	resto := fuente[i:]
	fin := len(resto)
	for _, cierre := range []string{"</Glifo>", "</svg>"} {
		if j := strings.Index(resto, cierre); j >= 0 && j < fin {
			fin = j
		}
	}
	return resto[:fin]
}

// TestLosGlifosSonLosDeLaWeb es EL guardián de `glifos.go`: cada cuerpo, forma a forma,
// contra el `.tsx` del que sale.
//
// Se compara por IGUALDAD de la lista de figuras y no por pertenencia: así fallan
// también un dibujo redibujado a ojo, una figura de más y una que la web afine.
//
// VISTO EN ROJO (2026-09-22, en el piloto): cambiando el `r="1"` del primer círculo de
// `glifoMas` por `r="2"`, el test nombra el glifo y enseña las dos listas de figuras.
// Deshecho.
func TestLosGlifosSonLosDeLaWeb(t *testing.T) {
	const fuente = "apps/frontend/apps/rooms/src/components/glyphs.tsx"
	web := webref.Read(t, fuente)
	nuestros := map[string]string{
		"MoreGlyph":     glifoMas,
		"PinGlyph":      glifoChincheta,
		"BookmarkGlyph": glifoMarcador,
		"VideoGlyph":    glifoVideo,
		"EditGlyph":     glifoLapiz,
		"ThreadsGlyph":  glifoHilos,
		"TrashGlyph":    glifoPapelera,
		"ClockGlyph":    glifoReloj,
		"ClipGlyph":     glifoClip,
		"PriorityGlyph": glifoPrioridad,
		"EmojiGlyph":    glifoEmoji,
		"SendGlyph":     glifoEnviar,
		"StarGlyph":     glifoEstrella,
		"LinkGlyph":     glifoEnlace,
		"CopyGlyph":     glifoCopiar,
		"NoteGlyph":     glifoNota,
		"ForwardGlyph":  glifoReenviar,
		"UnreadGlyph":   glifoNoLeido,
		"SeenGlyph":     glifoVisto,
		"FileGlyph":     glifoFichero,
		"DownloadGlyph": glifoDescargar,
		"UploadGlyph":   glifoSubir,
		"ImageGlyph":    glifoImagen,
	}
	for nombre, cuerpo := range nuestros {
		suyas := formas(cuerpoDeLaWeb(t, web, nombre))
		mias := formas(cuerpo)
		if len(suyas) == 0 {
			t.Errorf("%s: no se ha leído ni una figura de %s: el patrón ya no vale", nombre, fuente)
			continue
		}
		if strings.Join(suyas, " | ") != strings.Join(mias, " | ") {
			t.Errorf("%s no es el de la web:\n  web:  %v\n  aquí: %v", nombre, suyas, mias)
		}
	}
}

// TestLaCamaraTachadaSigueEnMessageView: ese glifo no está en `glyphs.tsx` sino en
// línea dentro de `MessageView.tsx`, así que se comprueba que sus tres caminos siguen
// ahí. Es pertenencia y no igualdad a propósito: el fichero tiene además los del
// resto de la vista.
func TestLaCamaraTachadaSigueEnMessageView(t *testing.T) {
	const fuente = "apps/frontend/apps/rooms/src/components/MessageView.tsx"
	suyas := formas(webref.Read(t, fuente))
	if len(suyas) < 3 {
		t.Fatalf("solo se han leído %d figuras de %s: el patrón ya no vale", len(suyas), fuente)
	}
	tiene := map[string]bool{}
	for _, f := range suyas {
		tiene[f] = true
	}
	for _, f := range formas(glifoLlamadaFin) {
		if !tiene[f] {
			t.Errorf("la cámara tachada ya no lleva %q en %s", f, fuente)
		}
	}
}

// ---- La geometría ----

// TestLaFilaDelCarrilMide40 es el guardián de la medida que separa el carril de Rooms
// del de las otras cinco apps: `grid h-6 w-6` en vez de `h-5 w-5`.
//
// Comprueba las tres cosas del patrón: el número de la web, lo que el widget le pide a
// Fyne, y lo PINTADO.
//
// VISTO EN ROJO (2026-09-22, en el piloto): quitando `f.CajaIcono = CajaIconoFila` la
// fila declara 33,25 y pinta 33,5, y el test lo dice con los dos números por separado
// —la cuenta y los píxeles—. Deshecho.
func TestLaFilaDelCarrilMide40(t *testing.T) {
	l := &Lateral{T: traductor(t), Locale: "es", CanalID: "c1"}
	f := l.fila(Canal{ID: "c1", Nombre: "equipo"}, tema.Claro).(*FilaCanal)

	declarada := MedidaFilaCanal()
	if declarada.Height != filaCarrilMedida {
		t.Errorf("la fila declara %v de alto y en producción mide %v", declarada.Height, float32(filaCarrilMedida))
	}
	if got := f.MinSize().Height; got != declarada.Height {
		t.Errorf("MinSize() da %v y la medida declarada %v: hay dos cuentas donde tiene que haber una",
			got, declarada.Height)
	}

	// Y lo pintado: el alfa de sidebar-active-bg se compone sobre el carril.
	f.Resize(fyne.NewSize(anchoUtilCarril, declarada.Height))
	fondo := fcanvas.NewRectangle(tema.Color("sidebar-bg", tema.Claro))
	fondo.Resize(fyne.NewSize(anchoUtilCarril+10, declarada.Height+10))
	img := pinta(t, anchoUtilCarril+10, declarada.Height+10, fondo, f)
	_, _, y0, y1 := caja(img, ui.BordeSobre("sidebar-active-bg", "sidebar-bg", tema.Claro))
	if alto := float32(y1-y0+1) / escala; alto != filaCarrilMedida {
		t.Errorf("la fila pinta %v de alto (de y %d a %d a ×%g) y la web %v",
			alto, y0, y1, escala, float32(filaCarrilMedida))
	}
}

// TestLaFilaDeCanalEsLaDeChannelRow: la pastilla va fuera del botón, con pr-1.
// En una fila de 231, Chromium mide 227 sin contador y 203,828 con «10» (rem 16).
// El contador usa font-bold tabular-nums, igual que el conformador compartido.
//
// VISTO EN ROJO (2026-09-22, en el frontal): con `MargenDerFila = 0` los botones salen
// de 210 y 231, y con la pastilla dentro del botón —como la pone `ui.FilaLateral`— el
// de «#Aleatorio» mide los 231 enteros. Deshecho.
func TestLaFilaDeCanalEsLaDeChannelRow(t *testing.T) {
	tr := traductor(t)
	l := &Lateral{T: tr, Locale: "es"}
	sin := l.fila(Canal{ID: "e", Nombre: "equipo"}, tema.Claro).(*FilaCanal)
	con := l.fila(Canal{ID: "a", Nombre: "Aleatorio", SinLeer: 10}, tema.Claro).(*FilaCanal)
	if got := sin.AnchoBotonFila(anchoUtilCarril); got != 227 {
		t.Errorf("el botón de una fila sin no leídos mide %v y la web 227", got)
	}
	// La pastilla de «10» es su texto en negrita más el `px-1`: el ancho del botón sale
	// de medir, así que se admite medio píxel.
	if got := con.AnchoBotonFila(anchoUtilCarril); got < 203.828125-0.5 || got > 203.828125+0.5 {
		t.Errorf("el botón de una fila con 10 sin leer mide %v y la web 203,828", got)
	}
	if con.peso() != ui.SemiNegrita || sin.peso() != ui.Medio {
		t.Error("el no leído no sale en negrita, o el leído sí: es `font-semibold` contra `font-medium`")
	}
	silenciado := l.fila(Canal{ID: "s", Nombre: "ruido", SinLeer: 4, Silenciado: true}, tema.Claro).(*FilaCanal)
	if silenciado.peso() != ui.Medio {
		t.Error("un canal silenciado con no leídos sale en negrita: tiene que quedarse tenue")
	}

	// La pastilla, en píxeles: el primario del carril a la derecha del botón (el
	// `<Badge count>` dentro de `bg-sidebar`).
	con.Resize(fyne.NewSize(anchoUtilCarril, filaCarrilMedida))
	img := pinta(t, anchoUtilCarril, filaCarrilMedida, con)
	x := int((anchoUtilCarril - MargenDerFila - 2) * escala)
	if got, quiero := px(img, x, int(filaCarrilMedida/2*escala)), tema.Color("sidebar-primary", tema.Claro); !cerca(got, quiero) {
		t.Errorf("a 2 px del final de la pastilla hay %v y la web pinta %v", got, quiero)
	}
}

// TestLaCabeceraDelCanalMide56 fija el h-14 de la banda con rem de 16.
func TestLaCabeceraDelCanalMide56(t *testing.T) {
	if AltoCabecera != cabeceraMedida {
		t.Errorf("AltoCabecera = %v y en producción la banda mide %v", AltoCabecera, float32(cabeceraMedida))
	}
	c := NuevaCabecera(traductor(t), "es")
	c.Canal = Canal{ID: "c1", Nombre: "equipo", Tema: "Guardias, despliegues e incidencias"}
	c.Miembros, c.Caras = 2, []string{"Daniel Delgado", "Operaciones"}
	c.Resize(fyne.NewSize(anchoCabecera, AltoCabecera))

	img := pinta(t, anchoCabecera, AltoCabecera+10, c)
	x0, x1, y0, y1 := caja(img, tema.Color("header-bg", tema.Claro))
	if alto := float32(y1-y0+1) / escala; alto != cabeceraMedida {
		t.Errorf("la banda pinta %v de alto y la web %v", alto, float32(cabeceraMedida))
	}
	if ancho := float32(x1-x0+1) / escala; ancho != anchoCabecera {
		t.Errorf("la banda pinta %v de ancho y la columna de contenido mide %v", ancho, float32(anchoCabecera))
	}
	// Las acciones son las `h-8` de la web, no las `h-7` por defecto del cromo.
	if LadoAccionCabecera != accionMedida || GlifoCabecera != glifoCabeceraMedio {
		t.Errorf("las acciones son %v con glifo %v y la web las mide %v con glifo %v",
			LadoAccionCabecera, GlifoCabecera, float32(accionMedida), float32(glifoCabeceraMedio))
	}
}

// TestElAreaDeEscribirMide978x40 fija una caja de 992: dos bordes y px-1.5
// dejan 978 al área; min-h-[2.5rem] son 40.
func TestElAreaDeEscribirMide978x40(t *testing.T) {
	if got := AnchoAreaCompositor(cajaCompositor); got != areaMedida {
		t.Errorf("con una caja de %v el área queda en %v y la web mide %v",
			float32(cajaCompositor), got, float32(areaMedida))
	}
	if AltoAreaMinimo != altoAreaMedido {
		t.Errorf("AltoAreaMinimo = %v y `min-h-[2.5rem]` con el rem de 16 son %v",
			AltoAreaMinimo, float32(altoAreaMedido))
	}
	if LadoBotonBarra != accionMedida || GlifoBarra != glifoBarraMedido {
		t.Errorf("los botones de la barra son %v con glifo %v y la web los mide %v con glifo %v",
			LadoBotonBarra, GlifoBarra, float32(accionMedida), float32(glifoBarraMedido))
	}
	// Dos bordes + pt-1 (4) + área (40) + pt-0.5 (2) + barra (32) + pb-1.5 (6): 86.
	if got, quiero := AltoCompositor(AltoAreaMinimo), float32(86); got != quiero {
		t.Errorf("la caja de escribir mide %v y de la hoja salen %v", got, quiero)
	}
}

// TestElCompositorNaceConSuMarcaDeAgua es la regla 7 de `ui.go` aplicada aquí: Fyne no
// llama al `Refresh` del renderer antes del primer pintado y `widget.Entry` solo
// materializa su `PlaceHolder` en el suyo. Sin el `r.Refresh()` del `CreateRenderer`,
// la caja nace SIN decir dónde se escribe.
//
// VISTO EN ROJO (2026-09-22, en el piloto), y peor de lo que se esperaba: sin ese
// `r.Refresh()` no es que falte la marca de agua, es que el primer `Resize` REVIENTA
// —`scrollContainerRenderer.Layout` con el contenido a nil, SIGSEGV en
// `internal/widget/scroller.go:419`—, porque `widget.Entry` materializa su
// desplazador en su propio Refresh. Deshecho.
func TestElCompositorNaceConSuMarcaDeAgua(t *testing.T) {
	c := NuevoCompositor(traductor(t), "es")
	c.NombreCanal = "equipo"
	tam := fyne.NewSize(cajaCompositor+2*MargenCompositor, MedidaCompositor().Height)
	c.Resize(tam)

	recien := tinta(pinta(t, tam.Width, tam.Height, c), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	c.Refresh()
	despues := tinta(pinta(t, tam.Width, tam.Height, c), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if recien == 0 {
		t.Fatal("la caja de escribir no pinta NADA recién montada")
	}
	if recien != despues {
		t.Errorf("recién montada pinta %d píxeles y tras un Refresh %d: nace a medias", recien, despues)
	}
}

// ---- Los seis estados ----

// TestElHistorialTieneSusSeisEstados comprueba que los seis pintan algo DISTINTO. No
// es una formalidad: el fallo que esto caza es el de siempre —«no hay mensajes» y «no
// he podido preguntar» pintando lo mismo—, que hace que alguien conteste creyendo que
// nadie ha escrito.
func TestElHistorialTieneSusSeisEstados(t *testing.T) {
	tr := traductor(t)
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	visto := map[[32]byte]Estado{}
	for _, e := range []Estado{Cargando, Listo, Vacio, Error, SinConexion, SinPermiso} {
		h := NuevoHistorial(tr, "es")
		h.NombreCanal = "equipo"
		h.Estado = e
		h.Ahora = reloj()
		if e == Listo {
			h.Mensajes = ejemplo()
		}
		h.Resize(fyne.NewSize(600, 400))
		img := pinta(t, 600, 400, h)
		n := tinta(img, blanco)
		rgba := image.NewNRGBA(img.Bounds())
		draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
		huella := sha256.Sum256(rgba.Pix)
		if n == 0 {
			t.Errorf("el estado %d no pinta nada", e)
			continue
		}
		if otro, ya := visto[huella]; ya {
			t.Errorf("los estados %d y %d producen la misma captura (%d píxeles pintados)", otro, e, n)
		}
		visto[huella] = e
	}
}

// ---- La lógica pura ----

func reloj() func() time.Time {
	return func() time.Time { return time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC) }
}

func ejemplo() []Mensaje {
	base := time.Date(2026, 9, 13, 14, 33, 0, 0, time.UTC)
	dd := Autor{ID: "dd", Nombre: "Daniel Delgado"}
	op := Autor{ID: "op", Nombre: "Operaciones"}
	return []Mensaje{
		{ID: "1", Autor: dd, Tipo: Llamada, Creado: base},
		{ID: "2", Autor: dd, Tipo: Llamada, Terminada: true, Segundos: 60, Creado: base.Add(2 * time.Minute)},
		{ID: "3", Autor: op, Cuerpo: "Cerrada la incidencia de accesos del viernes.", Creado: base.Add(time.Hour)},
		{ID: "4", Autor: op, Cuerpo: "Queda documentado en el runbook.", Creado: base.Add(61 * time.Minute)},
		{
			ID: "5", Autor: dd, Cuerpo: "Mensaje de prueba", Creado: base.AddDate(0, 0, 1),
			Reacciones: []Reaccion{{Emoji: "👍", Veces: 2, Mia: true}},
		},
	}
}

// TestAgruparCanalesEsElDeLaWeb es `groupChannels`, sección a sección: un canal en una
// categoría CONOCIDA va solo en ella, uno con una categoría que ya no existe recae en
// la suya por clase, y Favoritos solo aparece si tiene algo.
func TestAgruparCanalesEsElDeLaWeb(t *testing.T) {
	canales := []Canal{
		{ID: "b", Nombre: "beta"},
		{ID: "a", Nombre: "alfa"},
		{ID: "f", Nombre: "favorito", Favorito: true},
		{ID: "d", Nombre: "Ana", Clase: Directo},
		{ID: "c", Nombre: "en categoría", CategoriaID: "cat1"},
		{ID: "z", Nombre: "huérfano", CategoriaID: "borrada"},
	}
	secciones := AgruparCanales(canales, []Categoria{{ID: "cat1", Nombre: "Proyectos"}})

	claves := make([]string, 0, len(secciones))
	for _, s := range secciones {
		claves = append(claves, s.Clave)
	}
	quiero := []string{ClaveFavoritos, ClaveCanales, "cat1", ClaveDirectos}
	if strings.Join(claves, ",") != strings.Join(quiero, ",") {
		t.Fatalf("las secciones salen %v y la web las ordena %v", claves, quiero)
	}
	// Canales: alfa, beta y el huérfano, por nombre.
	nombres := make([]string, 0, len(secciones[1].Canales))
	for _, c := range secciones[1].Canales {
		nombres = append(nombres, c.Nombre)
	}
	if strings.Join(nombres, ",") != "alfa,beta,huérfano" {
		t.Errorf("la sección de canales trae %v y tenía que traer alfa,beta,huérfano", nombres)
	}
	if n := len(secciones[2].Canales); n != 1 {
		t.Errorf("la categoría personalizada trae %d canales y solo le corresponde 1", n)
	}
}

// TestElContadorDeUnaSeccionRespetaElNivelDeAviso: un canal silenciado NO suma, y uno
// «solo menciones» suma únicamente esas. Sin esto, plegar una categoría con un canal
// silenciado deja una alarma permanente.
func TestElContadorDeUnaSeccionRespetaElNivelDeAviso(t *testing.T) {
	canales := []Canal{
		{ID: "1", Nombre: "a", SinLeer: 10},
		{ID: "2", Nombre: "b", SinLeer: 7, Silenciado: true},
		{ID: "3", Nombre: "c", SinLeer: 5, Menciones: 2, Nivel: AvisarMenciones},
		{ID: "4", Nombre: "d", SinLeer: 9, Nivel: AvisarNada},
	}
	s := AgruparCanales(canales, nil)[0]
	if s.SinLeer != 12 {
		t.Errorf("el contador de la sección da %d y tenían que sumar 12 (10 + 2 menciones)", s.SinLeer)
	}
}

// TestElSeparadorDeNoLeidos es `unreadDividerIndex`, con sus tres casos.
func TestElSeparadorDeNoLeidos(t *testing.T) {
	casos := []struct{ cuantos, total, quiero int }{
		{0, 10, -1}, // no hay no leídos
		{3, 10, 7},  // los tres últimos
		{20, 10, 0}, // más no leídos que cargados: todo lo visible es nuevo
		{10, 10, 0}, // justos
		{5, 0, -1},  // no hay mensajes
	}
	for _, c := range casos {
		if got := IndiceSeparadorNoLeidos(c.cuantos, c.total); got != c.quiero {
			t.Errorf("IndiceSeparadorNoLeidos(%d, %d) = %d y tenía que dar %d",
				c.cuantos, c.total, got, c.quiero)
		}
	}
}

// TestUnaTarjetaDeLlamadaNoAnclaGrupo es el caso que la web arregló a mano: una
// tarjeta de llamada es un aviso del SISTEMA, sin avatar ni nombre, y el mensaje del
// mismo autor que venía detrás salía huérfano —solo el texto— porque «continuaba» a
// la tarjeta.
func TestUnaTarjetaDeLlamadaNoAnclaGrupo(t *testing.T) {
	base := time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)
	dd := Autor{ID: "dd", Nombre: "Daniel Delgado"}
	ms := []Mensaje{
		{ID: "1", Autor: dd, Tipo: Llamada, Creado: base},
		{ID: "2", Autor: dd, Cuerpo: "y ya está", Creado: base.Add(time.Minute)},
		{ID: "3", Autor: dd, Cuerpo: "de verdad", Creado: base.Add(2 * time.Minute)},
		{ID: "4", Autor: dd, Cuerpo: "al día siguiente", Creado: base.AddDate(0, 0, 1)},
	}
	quiero := []bool{false, false, true, false}
	for i, q := range quiero {
		if got := Agrupado(ms, i); got != q {
			t.Errorf("Agrupado(%d) = %v y tenía que dar %v", i, got, q)
		}
	}
	// Y el separador de día va ante el primero y ante el cambio de fecha.
	for i, q := range []bool{true, false, false, true} {
		if got := CambiaElDia(ms, i); got != q {
			t.Errorf("CambiaElDia(%d) = %v y tenía que dar %v", i, got, q)
		}
	}
}

// TestElRotuloDeDiaUsaHoyYAyer, con el reloj congelado: en las fechas de más atrás cae
// la fecha larga del idioma, y NO se capitaliza a mano (en español el día va en
// minúscula a propósito).
func TestElRotuloDeDiaUsaHoyYAyer(t *testing.T) {
	tr := traductor(t)
	ahora := reloj()()
	if got := RotuloDia(tr, "es", ahora, ahora); got != tr.T("day.today", nil) {
		t.Errorf("hoy sale %q y tenía que ser %q", got, tr.T("day.today", nil))
	}
	ayer := ahora.AddDate(0, 0, -1)
	if got := RotuloDia(tr, "es", ayer, ahora); got != tr.T("day.yesterday", nil) {
		t.Errorf("ayer sale %q y tenía que ser %q", got, tr.T("day.yesterday", nil))
	}
	viejo := ahora.AddDate(0, 0, -3)
	got := RotuloDia(tr, "es", viejo, ahora)
	if got == tr.T("day.today", nil) || got == tr.T("day.yesterday", nil) {
		t.Errorf("una fecha de hace tres días sale como %q", got)
	}
	if got != strings.ToLower(got[:1])+got[1:] {
		t.Errorf("la fecha larga sale capitalizada (%q): en español el día va en minúscula", got)
	}
}

// TestLaDuracionDeUnaLlamadaTieneSusCuatroEscalones. Menos de un minuto tiene cadena
// propia porque «0 min» se lee como un fallo.
func TestLaDuracionDeUnaLlamadaTieneSusCuatroEscalones(t *testing.T) {
	tr := traductor(t)
	vistos := map[string]bool{}
	for _, s := range []int{30, 90, 3600, 3900} {
		d := DuracionLlamada(tr, s)
		if d == "" || strings.Contains(d, ".") {
			t.Errorf("con %d s la duración sale %q: o está vacía o es una clave sin traducir", s, d)
		}
		if vistos[d] {
			t.Errorf("con %d s la duración repite %q: dos escalones dan lo mismo", s, d)
		}
		vistos[d] = true
	}
}

// TestElContadorCortaEn99 es el `unread > 99 ? '99+'` de la web.
func TestElContadorCortaEn99(t *testing.T) {
	for n, quiero := range map[int]string{0: "0", 7: "7", 99: "99", 100: "99+", 4321: "99+"} {
		if got := Contador(n); got != quiero {
			t.Errorf("Contador(%d) = %q y tenía que dar %q", n, got, quiero)
		}
	}
}

// TestLaPantallaSeMontaEntera: el árbol completo se pinta sin pánico, y con canal
// abierto la banda cae donde acaba el carril. `tema.Color` entra en pánico con un
// token desconocido AL PINTAR, no al compilar, así que este test es además el que caza
// un nombre de token inventado en cualquiera de las piezas.
//
// La banda NO se busca por color: en tema claro `header-bg` y `sidebar-bg` son el
// MISMO #1a3541, así que rastrear ese color encuentra el carril entero (medido: daba
// una caja de 744 de alto empezando en x = 0). El alto de la banda lo comprueba
// `TestLaCabeceraDelCanalMide56` con los píxeles; aquí se comprueba dónde la coloca el
// armazón.
func TestLaPantallaSeMontaEntera(t *testing.T) {
	p := NuevaPantalla(traductor(t), "es")
	p.Reloj(reloj())
	p.Lateral.EstadoCanales = Listo
	p.Lateral.Canales = []Canal{{ID: "c1", Nombre: "equipo"}, {ID: "c2", Nombre: "Ana", Clase: Directo}}
	p.Canal.Historial.Estado = Listo
	p.Canal.Historial.Mensajes = ejemplo()

	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	o := p.Objeto()

	p.SinCanal()
	o.Resize(fyne.NewSize(1280, 744))
	sin := pinta(t, 1280, 744, o)
	if p.Canal.Cabecera.Visible() {
		t.Error("sin canal abierto se sigue pintando la banda de la cabecera")
	}
	if !p.Canal.Vacio.Visible() {
		t.Error("sin canal abierto no se enseña el estado vacío")
	}
	if tinta(sin, blanco) == 0 {
		t.Error("la pantalla sin canal no pinta nada")
	}

	p.Abrir(Canal{ID: "c1", Nombre: "equipo", Tema: "Guardias"})
	o.Resize(fyne.NewSize(1280, 744))
	con := pinta(t, 1280, 744, o)
	if !p.Canal.Cabecera.Visible() || p.Canal.Vacio.Visible() {
		t.Fatal("con un canal abierto no se ve la cabecera, o se sigue viendo el vacío")
	}
	// La columna empieza justo donde acaba el carril de 260: la topbar no existe en
	// nativo (§13.6) pero el carril sí, así que la banda no arranca en 0.
	if x := p.Canal.Position().X; x != tema.AnchoLateral {
		t.Errorf("la columna del canal empieza en x = %v y el carril mide %v", x, tema.AnchoLateral)
	}
	if h := p.Canal.Cabecera.Size().Height; h != cabeceraMedida {
		t.Errorf("la banda dentro de la pantalla mide %v y la web %v", h, float32(cabeceraMedida))
	}
	if tinta(con, blanco) == 0 {
		t.Error("la pantalla entera no pinta nada")
	}
}

// TestLaCapturaSeGuarda deja el PNG de la pantalla entera cuando se pide, que es como
// se mira al lado de la captura de la web (`medidas/rooms/canal.ancho.png`).
func TestLaCapturaSeGuarda(t *testing.T) {
	destino := os.Getenv("SYTUE_CAPTURA")
	if destino == "" {
		t.Skip("sin SYTUE_CAPTURA: los guardianes de píxeles ya corrieron arriba")
	}
	p := NuevaPantalla(traductor(t), "es")
	p.Reloj(reloj())
	p.Lateral.EstadoCanales = Listo
	p.Lateral.HilosSinLeer = 3
	p.Lateral.PuedeGestionarApps = true
	p.Lateral.Canales = []Canal{
		{ID: "al", Nombre: "Aleatorio", SinLeer: 10},
		{ID: "c1", Nombre: "equipo"},
		{ID: "ge", Nombre: "General"},
		{ID: "op", Nombre: "operaciones", Clase: Privado},
		{ID: "dm", Nombre: "Ana Ruiz", Clase: Directo},
	}
	p.Canal.Historial.Estado = Listo
	p.Canal.Historial.Mensajes = ejemplo()
	p.Canal.Historial.UltimaLlamadaID = "1"
	p.Canal.Historial.IndiceNoLeidos = IndiceSeparadorNoLeidos(2, len(ejemplo()))
	p.Canal.Cabecera.Miembros = 2
	p.Canal.Cabecera.Caras = []string{"Daniel Delgado", "Operaciones"}
	p.Abrir(Canal{ID: "c1", Nombre: "equipo", Tema: "Guardias, despliegues e incidencias"})

	w := test.NewWindow(p.Objeto())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 744))
	f, err := os.Create(destino)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura en %s", destino)
}

// ---- La franja de abajo: «escribiendo» y la caja de escribir ----

// El Composer actual, con rem de 16: border-t + pt-3 + caja de 86 + pb-3.
// La línea h-5 de «escribiendo» conserva su sitio encima del compositor.
const (
	franjaCompositorMedida = 111 // borde + pt-3 (12) + caja (86) + pb-3 (12)
	lineaEscribiendoMedida = 20  // `h-5`
)

// TestLaFranjaDeAbajoEsLaDeLaWeb: el filete y los 12 px a cada lado vertical de
// la caja forman una franja de 111. La línea h-5 reserva otros 20 sobre ella.
//
// VISTO EN ROJO (2026-09-22, en el frontal): volviendo a los 14 por lado y quitando
// `AltoEscribiendo` del Layout de la columna, la franja da 103,5, empieza en 640,5, el
// historial mide 591,5 en vez de 580 y la primera fila de la franja sale blanca, sin
// filete. Deshecho.
func TestLaFranjaDeAbajoEsLaDeLaWeb(t *testing.T) {
	if got := MedidaCompositor().Height; got != franjaCompositorMedida {
		t.Errorf("la franja del compositor mide %v y la web %v", got, float32(franjaCompositorMedida))
	}
	p := NuevaPantalla(traductor(t), "es")
	p.Reloj(reloj())
	p.Lateral.EstadoCanales = Listo
	p.Lateral.Canales = []Canal{{ID: "c1", Nombre: "equipo"}}
	p.Canal.Historial.Estado = Listo
	p.Canal.Historial.Mensajes = ejemplo()
	p.Abrir(Canal{ID: "c1", Nombre: "equipo"})
	o := p.Objeto()
	o.Resize(fyne.NewSize(1280, 744))
	img := pinta(t, 1280, 744, o)

	c := p.Canal
	if y := c.Compositor.Position().Y; y != 744-franjaCompositorMedida {
		t.Errorf("la franja empieza en y = %v y en la web a %v del borde", y, float32(franjaCompositorMedida))
	}
	quiero := float32(744 - cabeceraMedida - lineaEscribiendoMedida - franjaCompositorMedida)
	if h := c.Historial.Size().Height; h != quiero {
		t.Errorf("el historial mide %v de alto y en la web %v", h, quiero)
	}
	// El filete, en píxeles: la fila de dispositivo de la franja es de `border`.
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	y := int((744 - franjaCompositorMedida) * escala)
	if got := px(img, int((tema.AnchoLateral+500)*escala), y); !cerca(got, tema.Color("border", v)) {
		t.Errorf("en la primera fila de la franja hay %v y tenía que estar el filete %v",
			got, tema.Color("border", v))
	}
}

// TestElRotuloDeEscribiendoTieneSusTresPlantillas es `typingLabel`.
func TestElRotuloDeEscribiendoTieneSusTresPlantillas(t *testing.T) {
	tr := traductor(t)
	casos := map[string][]string{
		"":                                   nil,
		"Ana está escribiendo…":              {"Ana"},
		"Ana y Beto están escribiendo…":      {"Ana", "Beto"},
		"Varias personas están escribiendo…": {"Ana", "Beto", "Caro"},
	}
	for quiero, nombres := range casos {
		if got := RotuloEscribiendo(tr, nombres); got != quiero {
			t.Errorf("RotuloEscribiendo(%v) = %q y la web dice %q", nombres, got, quiero)
		}
	}
}

// TestTuEstadoMide68x24: Chromium mide 67,859 × 24 con «En línea», text-xs,
// el punto h-2, gap-1.5 y px-1.5/py-1. La cabecera del carril mide 48 con py-3.
//
// VISTO EN ROJO (2026-09-22, en el frontal): con `RellenoEstado = tema.Espacio(2)` el
// disparador mide 59,375 × 28 y la cabecera del carril 49. Deshecho.
func TestTuEstadoMide68x24(t *testing.T) {
	m := MedidaEstadoPropio(traductor(t).T("presence.online", nil))
	if m.Height != 24 || m.Width < 67.875-0.5 || m.Width > 67.875+0.5 {
		t.Errorf("«tu estado» mide %v y la web 67,859 × 24", m)
	}
	// Dentro del carril: el estado de 24 más py-3 (12 por cada lado) son 48.
	p := NuevaPantalla(traductor(t), "es")
	p.Lateral.EstadoCanales = Listo
	p.Refrescar()
	o := p.Objeto()
	o.Resize(fyne.NewSize(1280, 744))
	lat := p.armazon.Menu.(*ui.Lateral)
	if h := lat.Cabecera.MinSize().Height; h+2*tema.Espacio(3) != 48 {
		t.Errorf("la cabecera del carril mide %v y la web 48", h+2*tema.Espacio(3))
	}
}

// TestIrACanalLlevaSuTecla: el `kbd` de «Ctrl+G» va pegado al `px-2.5` de la derecha y
// el rótulo se corta antes de él, con el `gap-2` de por medio. Su filete es el
// `sidebar-fg-muted/40` compuesto sobre el carril.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin el `Fondo` de la tecla y sin restar el
// borde en su x, la tecla acaba en 204,75 y por dentro sale del color del filete
// ({84 108 118} en vez del carril {26 53 65}). Deshecho.
func TestIrACanalLlevaSuTecla(t *testing.T) {
	b := NuevoBotonIrACanal("Ir a un canal…", "Ir a un canal… (Ctrl+G)", nil)
	m := MedidaIrACanal()
	b.Resize(m)
	img := pinta(t, m.Width, m.Height, b)

	k := b.tecla
	if fin := k.Position().X + k.Size().Width; fin != m.Width-tema.Borde-CanalonIrACanal {
		t.Errorf("la tecla acaba en x = %v y la web la deja a %v del borde", fin, tema.Borde+CanalonIrACanal)
	}
	if k.Size() != ui.MedidaTecla(k) {
		t.Errorf("la tecla mide %v y su medida es %v", k.Size(), ui.MedidaTecla(k))
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// A ×2 el filete de 1 px son dos columnas de dispositivo: se mira la de dentro, que
	// no depende de hacia dónde redondee el borde de la caja.
	x, y := int(k.Position().X*escala)+1, int((k.Position().Y+k.Size().Height/2)*escala)
	if got, quiero := px(img, x, y), filetesIrACanal(v); !cerca(got, quiero) {
		t.Errorf("en el filete izquierdo de la tecla hay %v y la web pinta %v", got, quiero)
	}
	if got, quiero := px(img, x+2*int(escala), y-int(escala)*3), tema.Color("sidebar-bg", v); !cerca(got, quiero) {
		t.Errorf("dentro de la tecla hay %v y tenía que verse el carril %v", got, quiero)
	}
}

// TestLaReaccionPropiaEsAzulClaro: la píldora de TU reacción es `bg-selected`, el
// tinte del azul al 10 % sobre el papel, con el filete `primary`. Antes salía del color
// del filete entera, porque `selected` es translúcido y la caja pinta el filete debajo.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin componer el tinte, dentro sale
// {26 60 80} —tinta— en vez de {233 243 251}. Deshecho.
func TestLaReaccionPropiaEsAzulClaro(t *testing.T) {
	p := NuevaReaccion(traductor(t), Reaccion{Emoji: "👍", Veces: 2, Mia: true}, nil)
	m := MedidaReaccion(p.Reaccion)
	p.Resize(m)
	img := pinta(t, m.Width, m.Height, p)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	quiero := tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
	// Dentro, junto al borde izquierdo y a media altura, donde no hay texto.
	if got := px(img, int((tema.Borde+2)*escala), int(m.Height/2*escala)); !cerca(got, quiero) {
		t.Errorf("dentro de la píldora propia hay %v y la web pinta %v", got, quiero)
	}
}

// TestLaTiraLlevaNRespuestasYElEditado: un mensaje con respuestas lleva «N respuestas»
// en su tira, que abre el hilo; y uno editado lleva «(editado)» detrás del texto, en
// la misma línea mientras quepa. Dentro del hilo ni tira ni agrupación.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin enlace, «la tira no lleva «3
// respuestas»». Deshecho.
func TestLaTiraLlevaNRespuestasYElEditado(t *testing.T) {
	tr := traductor(t)
	m := Mensaje{
		ID: "m", Autor: Autor{ID: "a", Nombre: "Ana"}, Cuerpo: "hola", Respuestas: 3, Editado: true,
		Creado: time.Date(2026, 9, 22, 9, 0, 0, 0, time.Local),
	}
	var abierto string
	f := NuevaFilaMensaje(tr, "es", m, false, nil)
	f.AlAbrirHilo = func(x Mensaje) { abierto = x.ID }
	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(600, 120))
	f.Resize(fyne.NewSize(600, MedidaFilaMensaje(f, 600).Height))

	if f.enlaceHilo == nil || f.enlaceHilo.Rotulo != "3 respuestas" {
		t.Fatalf("la tira no lleva «3 respuestas»: %+v", f.enlaceHilo)
	}
	quiero := AltoFilaMensaje(false) + AireReacciones + AltoEnlaceHilo
	if got := MedidaFilaMensaje(f, 600).Height; got != quiero {
		t.Errorf("la fila con tira mide %v y son %v", got, quiero)
	}
	test.Tap(f.enlaceHilo)
	if abierto != "m" {
		t.Error("«3 respuestas» no abre el hilo")
	}
	if f.editado == nil || f.editado.Position().Y >= f.cuerpo.Position().Y+tema.TextoBase.Linea {
		t.Error("«(editado)» no va en la línea del texto")
	}

	h := NuevaFilaMensaje(tr, "es", m, false, nil)
	h.EnHilo = true
	h.AlAbrirHilo = f.AlAbrirHilo
	if h.conEnlaceHilo() || h.altoTira() != 0 {
		t.Error("dentro del hilo la fila no lleva tira de respuestas")
	}
	// `py-2` arriba y abajo, sin el `mt-2` ni el `mt-0.5` de la del canal.
	if got, quiero := MedidaFilaMensaje(h, 300).Height, 2*RellenoFilaHilo+tema.TextoSM.Linea+tema.TextoBase.Linea; got != quiero {
		t.Errorf("la fila del hilo mide %v y son %v", got, quiero)
	}
}

// TestElMarkdownEsElDeLaWeb es `parseMarkdown`: las marcas en línea, los bloques y las
// trampas de la web —el snake_case no es cursiva, la puntuación colgante no es parte
// del enlace, un `javascript:` no es un enlace, el protocolo pelado tampoco—.
//
// VISTO EN ROJO: sin la comprobación de los bordes de la cursiva (Go no tiene
// lookbehind y la web lo hace con `(?<![A-Za-z0-9_])`), «snake_case_name» salía como
// «0:snake | 2:case | 0:name» y la web da «0:snake_case_name».
func TestElMarkdownEsElDeLaWeb(t *testing.T) {
	tipos := func(ns []Nodo) string {
		out := make([]string, 0, len(ns))
		for _, n := range ns {
			out = append(out, strconv.Itoa(int(n.Tipo))+":"+n.Texto+n.Enlace)
		}
		return strings.Join(out, " | ")
	}
	casos := map[string]string{
		"hola **mundo** y _tú_":              "0:hola  | 1:mundo | 0: y  | 2:tú",
		"snake_case_name":                    "0:snake_case_name",
		"mira https://sytue.eu.":             "0:mira  | 6:https://sytue.eu | 0:.",
		"[pincha](javascript:alert(1))":      "0:[pincha](javascript:alert(1))",
		"solo https://. y ya":                "0:solo  | 0:https://. | 0: y ya",
		"`a**b**` ~~no~~":                    "4:a**b** | 0:  | 3:no",
		"[docs](https://sytue.eu/a)":         "6:docshttps://sytue.eu/a",
		"```go\nfmt.Println()\n```\ndespués": "5:fmt.Println() | 0:después",
	}
	for entrada, quiero := range casos {
		if got := tipos(ParsearCuerpo(entrada)); got != quiero {
			t.Errorf("ParsearCuerpo(%q)\n  = %s\n  y la web da %s", entrada, got, quiero)
		}
	}
	bl := ParsearCuerpo("# Título\n- uno\n- dos\n3. tres\n> citado")
	if len(bl) != 4 || bl[0].Tipo != NodoTitulo || bl[0].Nivel != 1 || len(bl[1].Items) != 2 ||
		bl[2].Tipo != NodoNumerada || bl[2].Inicio != 3 || bl[3].Tipo != NodoCita {
		t.Errorf("los bloques salen %+v", bl)
	}
	if got := Emojificar("vale :thumbsup: :no_existe:"); got != "vale 👍 :no_existe:" {
		t.Errorf("Emojificar da %q", got)
	}
}

// TestLosEmojiSonLosDeLaWeb compara la tabla de shortcodes con `EMOJI` de
// MessageBody.tsx, entrada a entrada y en los dos sentidos.
func TestLosEmojiSonLosDeLaWeb(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/rooms/src/components/MessageBody.tsx")
	i := strings.Index(web, "export const EMOJI: Record<string, string> = {")
	if i < 0 {
		t.Fatal("MessageBody.tsx ya no exporta EMOJI")
	}
	tabla := web[i : i+strings.Index(web[i:], "};")]
	suyos := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(?:'([^']+)'|([a-z0-9_+-]+)):\s*'([^']+)',`).FindAllStringSubmatch(tabla, -1) {
		suyos[m[1]+m[2]] = m[3]
	}
	if len(suyos) != len(emojiPorNombre) {
		t.Errorf("la web tiene %d shortcodes y aquí hay %d", len(suyos), len(emojiPorNombre))
	}
	// Y en el MISMO orden: es el del selector.
	i = 0
	for _, m := range regexp.MustCompile(`(?m)^\s*(?:'([^']+)'|([a-z0-9_+-]+)):\s*'([^']+)',`).FindAllStringSubmatch(tabla, -1) {
		if i < len(emojiLista) && emojiLista[i].nombre != m[1]+m[2] {
			t.Fatalf("el %dº es :%s: en la web y :%s: aquí", i+1, m[1]+m[2], emojiLista[i].nombre)
		}
		i++
	}
	for n, e := range suyos {
		if emojiPorNombre[n] != e {
			t.Errorf(":%s: es %q en la web y %q aquí", n, e, emojiPorNombre[n])
		}
	}
}

// TestElCuerpoPintaSusMarcas: la negrita va en semi-negrita, la mención en su chip
// azul y @here en el de aviso, y un párrafo largo reparte en renglones.
func TestElCuerpoPintaSusMarcas(t *testing.T) {
	m := maquetadorDe("hola **Ana** @Ana Ruiz y @here", "", []string{"Ana Ruiz"}, 600)
	var negrita, mencion, canal bool
	for _, p := range m.piezas {
		switch {
		case p.texto == "Ana" && p.peso == ui.SemiNegrita:
			negrita = true
		case p.texto == "@Ana Ruiz" && p.estilo == estMencion:
			mencion = true
		case p.texto == "@here" && p.estilo == estDeCanal:
			canal = true
		}
	}
	if !negrita || !mencion || !canal {
		t.Errorf("negrita %v, mención %v, @here %v; piezas %+v", negrita, mencion, canal, m.piezas)
	}
	if len(m.cajas) != 2 || m.cajas[0].relleno != "selected" || m.cajas[1].relleno != "warning-bg" {
		t.Errorf("los chips salen %+v", m.cajas)
	}
	alto, _ := MaquetarCuerpo(strings.Repeat("palabra ", 40), nil, 200)
	if alto <= tema.TextoBase.Linea {
		t.Errorf("un párrafo largo a 200 mide %v: no reparte", alto)
	}
}

// TestElMenuDeLaFilaSaleAlPasar: el «…» no se ve en reposo; el botón de 227
// conserva su ancho. Al pasar, aparece a la derecha de la estrella, junto a pr-1,
// con su nombre, y abre el menú colgando de él.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin contarlo en `anchoCola`, el botón
// seguía en 206,5 con el «…» encima de su rótulo, y son 183,75. Deshecho.
func TestElMenuDeLaFilaSaleAlPasar(t *testing.T) {
	var abierto fyne.CanvasObject
	l := &Lateral{T: traductor(t), Locale: "es", AlMenuCanal: func(_ Canal, d fyne.CanvasObject) { abierto = d }}
	f := l.fila(Canal{ID: "e", Nombre: "equipo"}, tema.Claro).(*FilaCanal)
	test.WidgetRenderer(f)
	f.Resize(fyne.NewSize(anchoUtilCarril, filaCarrilMedida))
	if f.conMenu() || !f.menu.Hidden {
		t.Fatal("el «…» se ve en reposo")
	}
	if got := f.AnchoBotonFila(anchoUtilCarril); got != 227 {
		t.Errorf("en reposo el botón mide %v y ChannelRow.tsx da 227", got)
	}
	f.MouseIn(&desktop.MouseEvent{})
	if !f.conMenu() || f.menu.Hidden {
		t.Fatal("al pasar por encima no sale el «…»")
	}
	if got, quiero := f.AnchoBotonFila(anchoUtilCarril), 227-2*LadoEstrella-HuecoCola; got != quiero {
		t.Errorf("con estrella y «…» el botón mide %v y son %v", got, quiero)
	}
	if f.menu.Position().X+LadoEstrella != anchoUtilCarril-MargenDerFila ||
		f.estrella.Position().X+LadoEstrella+HuecoCola != f.menu.Position().X {
		t.Errorf("la estrella en %v y el «…» en %v", f.estrella.Position().X, f.menu.Position().X)
	}
	if f.menu.Nombre != "Acciones de equipo" {
		t.Errorf("el «…» se anuncia como %q", f.menu.Nombre)
	}
	test.Tap(f.menu)
	if abierto != f.menu {
		t.Error("el «…» no abre el menú colgando de él")
	}
}
