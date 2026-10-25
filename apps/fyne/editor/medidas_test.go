// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// escala es a la que se comprueban los píxeles: ×2, donde los medios píxeles de la
// escala de la suite salen exactos (`apps/fyne/ui/ui_test.go` lo explica con las
// mediciones).
const escala float32 = 2

func TestMain(m *testing.M) {
	a := test.NewApp()
	a.Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	código := m.Run()
	a.Quit()
	os.Exit(código)
}

// pinta captura los objetos sobre un lienzo blanco, a ×2 y sin relleno.
func pinta(t *testing.T, w, h float32, objs ...fyne.CanvasObject) image.Image {
	t.Helper()
	fondo := fcanvas.NewRectangle(color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	fondo.Resize(fyne.NewSize(w, h))
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewWithoutLayout(append([]fyne.CanvasObject{fondo}, objs...)...))
	c.SetScale(escala)
	c.Resize(fyne.NewSize(w, h))
	img := c.Capture()
	if ruta := os.Getenv("SYTUE_CAPTURA"); ruta != "" {
		f, err := os.Create(ruta)
		if err == nil {
			defer f.Close()
			_ = png.Encode(f, img)
		}
	}
	return img
}

func px(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// cerca compara dos colores con un canal de margen: el alfa se cuantiza y ningún gate
// puede exigir byte a byte (`docs/roadmap.md`, huecos medidos).
func cerca(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// TestLasBandasMidenLoQueElDom usa EditorDemo real, Chromium remoto 2026-10-04,
// CSS1Compat, raíz de 16 y Figtree cargada. editor.css conserva bandas literales;
// las dimensiones interiores proceden de las clases de Ribbon y StatusBar.
func TestLasBandasMidenLoQueElDom(t *testing.T) {
	casos := []struct {
		que    string
		got    float32
		quiero float32
	}{
		{"la cabecera", AltoCabecera, 48 - 0},
		{"la tira de pestañas", AltoTira, 80 - 48},
		{"la banda de grupos", AltoBanda, 148 - 80},
		{"la barra de estado", AltoEstado, 800 - 772},
		// El carril mide 40: borde1 + dos márgenes3.5 + botón32.
		{"el carril", AnchoCarril, 2*3.5 + tema.Borde + 32},
		{"el botón del carril", LadoBotonCarril, 32},
		{"el paso del carril", LadoBotonCarril + HuecoCarril, 200 - 164},
		// RibbonButton: h-7 w-7, medido 28 × 28.
		{"el botón de la cinta", LadoBotonCinta, 28},
		{"el paso de los botones de la cinta", LadoBotonCinta + HuecoControl, 322 - 292},
		{"el glifo de la cinta", IconoBotonCinta, 18},
		// min-h-11: fichas44 y botones28 centrados.
		{"la fila del grupo", AltoFilaGrupo, 44},
		{"el hueco entre pestañas", HuecoTira, 140.92188 - 81.20313 - 55.71875},
		{"el margen de la tira", MargenTira, 8},
		{"el margen de una pestaña", MargenPestana, 12},
		{"el margen de la cabecera", MargenCabecera, 12},
		{"el hueco de la cabecera", HuecoCabecera, 8},
		{"el botón de la cabecera", AltoBotonCabecera, 32},
		{"el título", AltoTitulo, 28},
		{"el panel", AnchoPanel, 288},
		{"el botón del zoom", LadoBotonEstado, 24},
		{"el deslizador", AnchoDeslizador, 96},
		{"el alto del range", AltoDeslizador, 20},
		{"el hueco del porcentaje", AnchoPorciento, 40},
	}
	for _, c := range casos {
		if c.got != c.quiero {
			t.Errorf("%s: aquí mide %v y el DOM de producción dice %v", c.que, c.got, c.quiero)
		}
	}
}

// TestLaFilaDeControlesCaeDondeElDom reproduce la cuenta vertical de la banda ENTERA y
// la compara con la posición medida de un botón de la cinta.
//
// Medido el 2026-10-16 en el banco con la cinta nueva: banda en y=136 (cabecera de la
// suite 56, la del editor 48, tira 32), la fila de controles en 142 y el primer botón en
// 150; la banda mide 67 y su línea, 68. En EditorDemo, sin la barra de la suite: banda en
// 80, `pt-1.5`=6 pone la fila en 86 y `min-h-11`=44 centra el botón h-7=28 en 94.
func TestLaFilaDeControlesCaeDondeElDom(t *testing.T) {
	const bandaY = 80 // EditorDemo: header 48 + tira 32
	fila := bandaY + ArribaGrupo
	if fila != 86 {
		t.Errorf("la fila de controles sale en y=%v y el DOM la pone en 86", fila)
	}
	boton := fila + (AltoFilaGrupo-LadoBotonCinta)/2
	if boton != 94 {
		t.Errorf("el botón de la cinta sale en y=%v y el DOM lo pone en 94", boton)
	}
	// El grupo llena la banda sin su línea: 6 + 44 + 2 + 11 + 4. Con el `py-1` que tenía la
	// banda le quedaban 59 y el rótulo pisaba la fila.
	if g := ArribaGrupo + AltoFilaGrupo + HuecoControl + AltoRotuloGrupo + AbajoGrupo; g != AltoGrupo() {
		t.Errorf("el grupo pide %v y la banda le da %v", g, AltoGrupo())
	}
}

// TestElBotonDeLaCintaMideLoQueDiceMedir es el patrón de `ui.TestElBotonMide…`
// aplicado al botón de la cinta: lo declarado, lo que pide a Fyne, y lo PINTADO.
func TestElBotonDeLaCintaMideLoQueDiceMedir(t *testing.T) {
	b := NuevoBotonCinta(1, "Negrita", nil)
	b.Activo = true // el fondo `selected` es relleno opaco y se puede rastrear
	declarada := MedidaBotonCinta()
	if got := b.MinSize(); got != declarada {
		t.Errorf("MinSize() = %v y MedidaBotonCinta dice %v", got, declarada)
	}
	b.Resize(declarada)
	img := pinta(t, declarada.Width+10, declarada.Height+10, b)
	// El centro del botón es fondo `selected` sobre blanco; la esquina, blanco.
	centro := px(img, int(declarada.Width*escala/2), int(declarada.Height*escala/2))
	if centro == (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Error("el botón activo no ha pintado su fondo: el token `selected` no llega")
	}
	// La caja pintada de h-7 w-7 mide 28 × 28, a ×2 son 56 filas.
	ancho, alto := extension(img, centro)
	if ancho != 56 || alto != 56 {
		t.Errorf("el botón pinta %d×%d filas de dispositivo y 28 a ×2 son 56×56", ancho, alto)
	}
}

// extension mide la caja de píxeles de ese color.
func extension(img image.Image, c color.NRGBA) (ancho, alto int) {
	b := img.Bounds()
	x0, x1, y0, y1 := 9999, -1, 9999, -1
	for y := range b.Dy() {
		for x := range b.Dx() {
			if cerca(px(img, x, y), c) {
				x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
			}
		}
	}
	if x1 < 0 {
		return 0, 0
	}
	return x1 - x0 + 1, y1 - y0 + 1
}

// TestLaCintaPintaSusDosLineas comprueba que la línea de la tira va DENTRO de sus 32 y
// no debajo: es lo que hace que las pestañas midan 31 y no 32, y lo que se rompe en
// cuanto alguien suma el borde por fuera.
func TestLaCintaPintaSusDosLineas(t *testing.T) {
	ctx := &Contexto{Tipo: Documento, Zoom: 100}
	c := NuevaCinta(ctx, Genericas())
	// En escritorio conserva la altura habitual; móvil usa una banda compacta.
	c.Resize(fyne.NewSize(800, AltoCinta()))
	img := pinta(t, 820, AltoCinta()+10, c)

	borde := tema.Color("border", tema.Claro)
	for _, caso := range []struct {
		que string
		y   float32
	}{
		{"la de la tira", AltoTira - tema.Borde},
		{"la de la banda", AltoCinta() - tema.Borde},
	} {
		if got := px(img, 300, int(caso.y*escala)); !cerca(got, borde) {
			t.Errorf("%s línea de la cinta pinta %v en y=%v y el token `border` es %v", caso.que, got, caso.y, borde)
		}
	}
}

// TestLasPestanasSeColocanDondeElDom comprueba las X de la tira contra las medidas:
// Archivo va en x8, mide 69,20313 × 24; Inicio empieza en 81,20313, con gap-1=4.
//
// Hasta el 2026-09-22 este guardián decía que «Archivo» no existía en nativo, «porque es
// un menú del sistema». No lo es: ninguna app nativa pone menú de sistema, y en Windows
// y Linux no habría habido dónde descargar un documento.
func TestLasPestanasSeColocanDondeElDom(t *testing.T) {
	ctx := &Contexto{Tipo: Documento, Zoom: 100, T: func(k string, _ i18n.Vars) string {
		return map[string]string{
			"ribbon.tab.home":   "Inicio",
			"ribbon.tab.review": "Revisar",
			"ribbon.tab.view":   "Vista",
			"file.menu":         "Archivo",
		}[k]
	}}
	c := NuevaCinta(ctx, Genericas())
	if a := c.archivo.MinSize(); a.Width < 69.20313-0.2*7 || a.Width > 69.20313+0.2*7 || a.Height != 24 {
		t.Errorf("«Archivo» mide %v y el DOM dice 69,20313 × 24", a)
	}
	r := c.CreateRenderer()
	r.Layout(fyne.NewSize(1280, AltoCinta()))
	// La pestaña va dentro de su fila deslizante, que empieza tras «Archivo».
	if x := c.tira.Position().X + c.tiras[0].Position().X; x < 81.20313-1.4 || x > 81.20313+1.4 {
		t.Errorf("la primera pestaña empieza en x=%v y en el DOM en 81,20313", x)
	}
	// Anchos leídos de Archivo-1280; px-3, text-sm y el peso de cada pestaña.
	quiero := map[string]float32{"Inicio": 55.71875, "Revisar": 65.92188, "Vista": 53.60938}
	for _, p := range c.tiras {
		m := p.MinSize()
		q, hay := quiero[p.Rotulo]
		if !hay && p.Rotulo != "Revisar" {
			t.Fatalf("pestaña inesperada %q", p.Rotulo)
		}
		if !hay {
			if m.Height != AltoTira-tema.Borde {
				t.Errorf("la pestaña %q mide %v de alto y el DOM dice 31", p.Rotulo, m.Height)
			}
			continue
		}
		// La tolerancia es por CARÁCTER y no absoluta, porque el desvío que queda
		// es proporcional al rótulo: medido aquí, «Inicio» y «Vista» caen dentro de
		// 1 px y «Colaboración» se queda 2,27 px corta (−3,2 %). No es el `ceil` del
		// conformador —eso se quitó (ADR 0048)— ni el 1/64 de harfbuzz, que da 0,23 %
		// como mucho: la web pinta con «Figtree Variable» y aquí se usan las cinco
		// instancias estáticas, y un eje `wght` interpolado no tiene por qué dar los
		// mismos avances que la instancia. Está en el informe como hueco MEDIDO.
		//
		// 0,2 px por carácter deja pasar ese desvío y sigue cazando lo que importa:
		// un escalón equivocado (text-base14 frente a text-sm13) o un peso equivocado no caben aquí.
		tolerancia := 0.2 * float32(len([]rune(p.Rotulo)))
		if d := m.Width - q; d > tolerancia || d < -tolerancia {
			t.Errorf("la pestaña %q mide %v, el DOM dice %v y la tolerancia es ±%v",
				p.Rotulo, m.Width, q, tolerancia)
		}
		if m.Height != AltoTira-tema.Borde {
			t.Errorf("la pestaña %q mide %v de alto y el DOM dice 31", p.Rotulo, m.Height)
		}
	}
}

// TestLosSeisEstadosSePintan: toda pantalla de la suite tiene que saber pintar los
// seis, y este los recorre uno a uno. Un estado que devolviera nil o entrara en pánico
// —un token de color inventado lo hace AL PINTAR— se cae aquí y no en producción.
func TestLosSeisEstadosSePintan(t *testing.T) {
	for _, e := range []Estado{Cargando, Vacio, Error, SinConexion, SinPermiso, Listo} {
		ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: e}
		o := PantallaEstado(ctx)
		if o == nil {
			t.Fatalf("el estado %d no pinta nada", e)
		}
		o.Resize(fyne.NewSize(400, 300))
		pinta(t, 400, 300, o)
	}
}

// TestElMarcoApilaLasBandas comprueba la composición vertical entera: cabecera, cinta,
// franja de solo lectura y barra de estado, en ese orden y con sus alturas.
func TestElMarcoApilaLasBandas(t *testing.T) {
	ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: Listo, SoloLectura: true}
	m := NuevoMarco(ctx, Genericas(), nil)
	m.Resize(fyne.NewSize(1200, 800))
	r := m.CreateRenderer()
	r.Layout(fyne.NewSize(1200, 800))

	if got := m.cabecera.Position().Y; got != 0 {
		t.Errorf("la cabecera empieza en y=%v y tiene que ir arriba del todo", got)
	}
	if got := m.cinta.Position().Y; got != AltoCabecera {
		t.Errorf("la cinta empieza en y=%v y la cabecera mide %v", got, AltoCabecera)
	}
	if m.franja == nil {
		t.Fatal("en solo lectura tiene que haber franja de aviso: sin ella, pulsar negrita no hace nada y parece roto")
	}
	if got := m.franja.Position().Y; got != AltoCabecera+AltoCinta() {
		t.Errorf("la franja empieza en y=%v y la cabecera más la cinta son %v", got, AltoCabecera+AltoCinta())
	}
	if got := m.carril.Position().Y; got != AltoCabecera+AltoCinta()+m.franja.MinSize().Height {
		t.Errorf("el carril empieza en y=%v y debería ir bajo la franja", got)
	}

	// Y la barra de estado, ABAJO. Este trozo existe por un fallo real: la barra se
	// construía dentro de `Objects()`, que Fyne llama tanto al pintar como al
	// colocar, así que se colocaba una instancia y se pintaba OTRA —la pintada salía
	// en (0,0), encima de la cabecera—. No lo cazó ningún guardián de medida, porque
	// todos miraban bandas que sí se colocaban; se vio en la captura.
	//
	// Además de la posición se comprueba la IDENTIDAD: que la barra que se coloca sea
	// la misma que va en `Objects()`. Es lo que de verdad falló, y una barra nueva en
	// cada llamada volvería a pasar la comprobación de posición sin arreglar nada.
	if got := m.estado.Position().Y; got != 800-AltoEstado {
		t.Errorf("la barra de estado está en y=%v y la ventana mide 800: tiene que ir abajo", got)
	}
	objs := r.Objects()
	if objs[len(objs)-1] != fyne.CanvasObject(m.estado) {
		t.Error("la barra de estado que se pinta no es la que se coloca: se está construyendo en Objects()")
	}
}

// TestElCromoSeApagaEnSoloLectura es el guardián de la regla que manda sobre todas: en
// solo lectura, un comando NO llega al lienzo.
//
// Se ha visto en ROJO a mano quitando el `if c.SoloLectura` de `Contexto.Mandar`:
// entonces el comando llega, `mandados` vale 1 y el test falla. El cortafuegos está en
// el contexto y no en cada botón a propósito — un botón que se olvide no puede
// corromper nada.
func TestElCromoSeApagaEnSoloLectura(t *testing.T) {
	l := &lienzoDePrueba{}
	ctx := &Contexto{Tipo: Documento, Zoom: 100, Lienzo: l, SoloLectura: true}
	ctx.Mandar(Negrita, "")
	if l.mandados != 0 {
		t.Errorf("en solo lectura ha llegado %d comando(s) al lienzo: el documento se podría corromper", l.mandados)
	}
	ctx.SoloLectura = false
	ctx.Mandar(Negrita, "")
	if l.mandados != 1 {
		t.Errorf("fuera de solo lectura tienen que llegar: han llegado %d", l.mandados)
	}
}

// TestElRegistroRespetaElOrdenDeLaWeb comprueba la costura que Calcus y Diapos usan:
// añadir una pestaña de la taxonomía la deja EN SU SITIO —entre Inicio y Revisar, para
// «datos»—, con el rótulo y los tipos de `pestanas.json`, que es el orden de la cinta web.
func TestElRegistroRespetaElOrdenDeLaWeb(t *testing.T) {
	r := Genericas()
	// Añadidas en desorden: el sitio lo da la taxonomía, no el orden de llamada.
	r.Anadir("formulas", func(*Contexto) fyne.CanvasObject { return nil })
	r.Anadir("datos", func(*Contexto) fyne.CanvasObject { return nil })
	ids := make([]string, 0, len(r.pestanas))
	for _, p := range r.pestanas {
		ids = append(ids, p.ID)
	}
	quiero := []string{"inicio", "datos", "formulas", "revisar", "vista"}
	for i := range quiero {
		if i >= len(ids) || ids[i] != quiero[i] {
			t.Fatalf("el registro queda %v y la taxonomía pone %v", ids, quiero)
		}
	}
	// Y con un documento, las de Calcus no se ven; con una hoja, sí.
	for _, p := range r.Visibles(Documento) {
		if p.ID == "datos" || p.ID == "formulas" {
			t.Errorf("la pestaña %q de Calcus se ve en un documento de Docus", p.ID)
		}
	}
	if got := len(r.Visibles(Hoja)); got != len(quiero) {
		t.Errorf("en una hoja se ven %d pestañas y la taxonomía dice %d", got, len(quiero))
	}
	// Una pestaña que la taxonomía no conoce es un error de programación, no una pestaña.
	defer func() {
		if recover() == nil {
			t.Error("añadir una pestaña inventada debería parar")
		}
	}()
	r.Anadir("inventada", nil)
}

// lienzoDePrueba es un [Lienzo] que solo cuenta.
type lienzoDePrueba struct{ mandados int }

func (l *lienzoDePrueba) Objeto() fyne.CanvasObject     { return ui.NuevoVacio("", "") }
func (l *lienzoDePrueba) Cargar(string) bool            { return true }
func (l *lienzoDePrueba) Serializar() string            { return "" }
func (l *lienzoDePrueba) Estadisticas() (int, int, int) { return 1, 1, 0 }
func (l *lienzoDePrueba) Mandar(Comando, string)        { l.mandados++ }
func (l *lienzoDePrueba) Formato() Formato              { return Formato{} }

// TestElMarcoPasaDeCargandoAlLienzo: un documento que llega de la red pasa por
// «cargando» antes de estar listo, y al estarlo el centro tiene que ser el LIENZO.
//
// VISTO EN ROJO (2026-09-22, en el frontal): con la comprobación vieja
// (`m.centro == nil`) el centro seguía siendo el estado de «Abriendo…» y el test lo
// decía. Deshecho.
func TestElMarcoPasaDeCargandoAlLienzo(t *testing.T) {
	obj := ui.NuevoVacio("lienzo", "")
	ctx := &Contexto{Tipo: Presentacion, Zoom: 100, Estado: Cargando, Lienzo: &lienzoFijo{obj}}
	m := NuevoMarco(ctx, Genericas(), nil)
	if m.centro == fyne.CanvasObject(obj) {
		t.Fatal("cargando, el centro no puede ser todavía el lienzo")
	}
	ctx.Estado = Listo
	ctx.Refrescar()
	if m.centro != fyne.CanvasObject(obj) {
		t.Error("listo el documento, el centro tiene que ser el lienzo y sigue siendo el estado")
	}
}

// TestLaCabeceraSigueAlContexto: el título que llega después de montar y el chip de
// guardado se ven en cuanto el contexto cambia.
func TestLaCabeceraSigueAlContexto(t *testing.T) {
	ctx := &Contexto{Tipo: Presentacion, Zoom: 100, Estado: Listo}
	m := NuevoMarco(ctx, Genericas(), nil)
	r := test.WidgetRenderer(m.cabecera).(*cabeceraRenderer)
	ctx.Titulo, ctx.Guardado = "Trimestre", Sucio
	ctx.Refrescar()
	if m.cabecera.titulo.Texto() != "Trimestre" {
		t.Errorf("el título de la cabecera es %q y el del contexto «Trimestre»", m.cabecera.titulo.Texto())
	}
	if r.estado.Text != "header.save.dirty" {
		t.Errorf("el chip dice %q con el documento sucio", r.estado.Text)
	}
}

type lienzoFijo struct{ o fyne.CanvasObject }

func (l *lienzoFijo) Objeto() fyne.CanvasObject     { return l.o }
func (l *lienzoFijo) Cargar(string) bool            { return true }
func (l *lienzoFijo) Serializar() string            { return "" }
func (l *lienzoFijo) Estadisticas() (int, int, int) { return 1, 1, 0 }
func (l *lienzoFijo) Mandar(Comando, string)        {}
func (l *lienzoFijo) Formato() Formato              { return Formato{} }

// TestLaCintaReconstruidaSeColoca: tras `Reconstruir` —el marco la rehace con cada
// cambio del documento— las pestañas nuevas están en su sitio y con su tamaño, no
// amontonadas en (0, 0). VISTO EN ROJO (2026-09-23, en el frontal): sin el `Layout` del
// `Refresh`, las tres pestañas medían 0 × 0 y la banda de Diapos salía vacía al cambiar
// de diapositiva.
func TestLaCintaReconstruidaSeColoca(t *testing.T) {
	ctx := &Contexto{Tipo: Documento, Zoom: 100, T: func(k string, _ i18n.Vars) string { return k }}
	c := NuevaCinta(ctx, Genericas())
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(800, AltoCinta()))
	c.Reconstruir()
	for i, p := range c.tiras {
		if p.Size().Width == 0 || (i > 0 && p.Position().X == 0) {
			t.Errorf("la pestaña %d está en %v con %v tras reconstruir", i, p.Position(), p.Size())
		}
	}
}

// TestLaCabeceraCabeEnUnMovil: por debajo de `md` la cabecera es la de `max-md:` de la web
// —sin marca, sin filete, sin «Descargar»—, el título se queda lo que dejan el chip y
// «Compartir» y nada se pisa. Chromium mide los avances de Guardado hace 25 s
// (107.08876 a 12/400) y Compartir (60.55406 a 13/600). Las clases de la
// cabecera dan x48 y un ancho de 110.35718 a 390. VISTO EN ROJO sin el camino estrecho: a 360 «Descargar» tapaba el chip y
// «Compartir» se montaba sobre el título.
func TestLaCabeceraCabeEnUnMovil(t *testing.T) {
	textos := map[string]string{
		"header.share": "Compartir", "header.download": "Descargar", "header.save.agoSeconds": "Guardado hace 25 s",
	}
	ctx := &Contexto{
		Tipo: Documento, Zoom: 100, Estado: Listo, Guardado: Limpio, Titulo: "Propuesta de servicio — Nordika",
		T: func(k string, _ i18n.Vars) string {
			if v, ok := textos[k]; ok {
				return v
			}
			return k
		},
	}
	c := NuevaCabecera(ctx, func() {})
	c.guardadoEn = time.Now().Add(-25 * time.Second)
	r := test.WidgetRenderer(c).(*cabeceraRenderer)
	for _, ancho := range []float32{360, 390} {
		c.Resize(fyne.NewSize(ancho, AltoCabecera))
		r.Layout(c.Size())
		tit, com := c.titulo, c.compartir
		finTitulo := tit.Position().X + tit.Size().Width
		finChip := r.estado.Position().X + ui.Medir(r.estado.Text, tema.TextoXS, ui.Normal).Width
		switch {
		case c.descargar.Visible() || c.marca.Objeto().Visible():
			t.Errorf("a %.0f siguen «Descargar» o la marca", ancho)
		case r.punto.Position().X < finTitulo+HuecoCabecera-0.5:
			t.Errorf("a %.0f el chip empieza en %.2f y el título acaba en %.2f", ancho, r.punto.Position().X, finTitulo)
		case com.Position().X < finChip+HuecoCabecera-0.5:
			t.Errorf("a %.0f «Compartir» empieza en %.2f y el chip acaba en %.2f (%q)", ancho, com.Position().X, finChip, r.estado.Text)
		case com.Position().X+com.Size().Width > ancho-MargenCabecera+0.5:
			t.Errorf("a %.0f «Compartir» se sale: acaba en %.2f", ancho, com.Position().X+com.Size().Width)
		case tit.Size().Width < tema.Espacio(24)-0.5 || tit.Size().Width > 0.4*ancho+0.5:
			t.Errorf("a %.0f el título mide %.2f, fuera de min-w-24 y max-w-[40vw]", ancho, tit.Size().Width)
		}
	}
	if x, w := c.titulo.Position().X, c.titulo.Size().Width; abs32(x-48) > 0.5 || abs32(w-110.35718) > 2 {
		t.Errorf("a 390 el título va en x %.2f con %.2f de ancho, y la cuenta CSS con avances Chromium lo pone en 48 con 110,35718", x, w)
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// TestLaCintaMovilMantieneUnaFilaVisible: como CertiPDF, la banda móvil
// permanece visible y usa todo el ancho para desplazar sus controles, sin Más.
func TestLaCintaMovilMantieneUnaFilaVisible(t *testing.T) {
	ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: Listo, T: func(k string, _ i18n.Vars) string { return k }}
	m := NuevoMarco(ctx, Genericas(), nil)
	w := test.NewTempWindow(t, m)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(360, 740))
	c := m.cinta
	if c.Alto(360) != AltoDeTira()+AltoBandaEstrecha || !c.banda.Visible() || c.mas.Visible() {
		t.Fatalf("la banda móvil debe estar visible sin expansión: alto %.1f", c.Alto(360))
	}
	if m.cinta.Size().Height != c.Alto(360) || c.banda.Size().Width != 360 {
		t.Errorf("la banda no conserva su ancho y alto: %v", c.banda.Size())
	}
	for _, o := range objetosDeBanda(c.cuerpo) {
		if g, ok := o.(*Grupo); ok && (g.Position().Y != 0 || g.conRotulo()) {
			t.Errorf("el grupo %q está en y=%.1f con rótulo %v", g.Rotulo, g.Position().Y, g.conRotulo())
		}
	}
	c.Elegir(len(c.tiras) - 1)
	ultima := c.tiras[len(c.tiras)-1]
	d := fyne.CurrentApp().Driver()
	ini := d.AbsolutePositionForObject(ultima).X
	finTira := d.AbsolutePositionForObject(c.tira).X + c.tira.Size().Width
	if fin := ini + ultima.Size().Width; fin > finTira+0.5 || ini < d.AbsolutePositionForObject(c.tira).X-0.5 {
		t.Errorf("la pestaña elegida va de %.1f a %.1f fuera de la tira", ini, fin)
	}
	w.Resize(fyne.NewSize(1280, 800))
	if c.Alto(1280) != AltoCinta() {
		t.Error("en escritorio se conserva la altura de la cinta")
	}
}
