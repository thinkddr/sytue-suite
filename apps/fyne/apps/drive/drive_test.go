// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los guardianes de este paquete comparan contra los números MEDIDOS en producción
// (`test/e2e/medidas-web.mjs drive`, 1280×800, es-ES, DSF 1, corporate.sytue.eu
// v3.3.9), no contra lo que calcula este código. Un test que compara la
// implementación consigo misma pasa siempre.
//
// Lo medido y dónde sale en la captura `drive/archivos.ancho.png`:
//
//	fila del explorador       51 de alto, 999 de ancho (y=154, 205, 256…)
//	cabecera de columnas      31,5 — su casilla en y=113,25
//	cabecera de vista         48,5 — su primer control en y=65,75 y la ruta en 66,63
//	conmutador de vista       58 × 30, de x=1201 a 1259
//	buscador                  28 × 196, en x=833,25
//	«Nuevo» / «Subir»         28 de alto, en x=1036,25 y x=1119,5
//	carril lateral            260, con filas de 33,25 × 231 separadas por 1,75
//	casilla                   14 × 14

func TestMain(m *testing.M) {
	// No `test.NewTempApp(t)`: rompe TestLaLetraEsFigtree en otro fichero.
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	// Sin animaciones (`prefers-reduced-motion`). En el driver de pruebas el reloj de las
	// animaciones corre en SU rutina y refresca la rueda o el esqueleto mientras esta
	// captura el lienzo: una carrera que en la app no existe (ahí el reloj va en la rutina
	// de la interfaz) y que se vio en `TestCompartirConUnaPersonaYConUnEnlace`.
	ui.SinMovimiento = true
	os.Exit(m.Run())
}

func traductor(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Drive())
	if err != nil {
		t.Fatalf("no se pudo cargar el catálogo: %v", err)
	}
	return tr
}

// casi compara con la tolerancia de un dieciseisavo de píxel, que es la del reparto en
// coma flotante y NO la de «se parece bastante».
func casi(t *testing.T, que string, tengo, quiero float32) {
	t.Helper()
	if d := tengo - quiero; d > 1.0/16 || d < -1.0/16 {
		t.Errorf("%s: %v, y en producción %v", que, tengo, quiero)
	}
}

// Medidas con raíz de 16 px: h-8=32, w-56=224, px-6=24;
// conmutador 2×32 + dos bordes, cabecera 34 + py-2.5×2 + border-b.
// El glifo de 36 px fija la fila básica a 36 + py-2×2 + border-b=53.
// ---- Las medidas declaradas, contra las clases CSS actuales ----

func TestLasMedidasDelCromoSonLasDeProduccion(t *testing.T) {
	casi(t, "el alto de la cabecera de vista", AltoCabeceraVista, 55)
	casi(t, "la línea de la cabecera (el conmutador manda)", AltoLineaCabecera, 34)
	casi(t, "el alto de un control del cromo `h-8`", AltoControlCromo, 32)
	casi(t, "el canalón `sm:px-6`", CanalonCabecera(1280), 24)
	casi(t, "el canalón `px-4` por debajo de sm", CanalonCabecera(390), 16)
	casi(t, "el ancho del buscador `sm:w-56`", AnchoBuscador, 224)

	m := MedidaConmutadorVista()
	casi(t, "el ancho del conmutador de vista", m.Width, 66)
	casi(t, "el alto del conmutador de vista", m.Height, 34)

	// La fila y la cabecera de columnas las declara el catálogo; aquí se comprueba
	// que esta app usa ESAS y no unas propias.
	casi(t, "el alto de la fila del explorador", ui.MedidaFilaDrive(), 53)
	casi(t, "el alto de la cabecera de columnas", ui.AltoCabeceraDrive, 36)
	casi(t, "el alto de la fila de la papelera", MedidaFilaPapelera(), 53)
}

// TestLaCabeceraColocaSusControlesDondeProduccion arma la cabecera a 1280 y comprueba
// las x y las y del volcado del DOM, una a una.
func TestLaCabeceraColocaSusControlesDondeProduccion(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Ventana = 1280
	cab := e.cabecera(tema.Claro)
	// El ancho de la zona de contenido: 1280 menos el carril de 260.
	const ancho = 1280 - tema.AnchoLateral
	cab.Resize(fyne.NewSize(ancho, AltoCabeceraVista))

	c, ok := cab.(*fyne.Container)
	if !ok {
		t.Fatal("la cabecera ya no es un contenedor")
	}
	lay, ok := c.Layout.(*CabeceraVista)
	if !ok {
		t.Fatal("la cabecera ya no usa su propio layout")
	}

	// Las x del volcado son absolutas en la ventana; aquí la cabecera empieza en el
	// carril, así que se le restan los 260.
	const carril = tema.AnchoLateral
	casi(t, "la ruta empieza en x=281", carril+lay.Ruta.Position().X, 284)
	casi(t, "y la ruta se centra en la línea de 30, en y=66,63",
		lay.Ruta.Position().Y, 12)

	if len(lay.Acciones) != 3 {
		t.Fatalf("la cabecera tiene %d acciones y en producción hay tres (buscador, Nuevo, Subir)", len(lay.Acciones))
	}
	buscador, conmutador := lay.Acciones[0], lay.Conmutador
	casi(t, "el buscador mide 196 de ancho", buscador.Size().Width, 224)
	casi(t, "el buscador mide 28 de alto", buscador.Size().Height, 32)
	casi(t, "el conmutador acaba en x=1259", carril+conmutador.Position().X+conmutador.Size().Width, 1256)
	casi(t, "los controles de 28 se centran en y=65,75",
		buscador.Position().Y, 11)

	// El guardián, invertido a mano: con el conmutador fuera, la línea sería de 28 y
	// TODO subiría un píxel. Es el error que este test existe para atrapar.
	if AltoLineaCabecera == AltoControlCromo {
		t.Fatal("la línea de la cabecera mide lo mismo que un control: falta el borde del conmutador")
	}
}

// ---- Las siete fases ----

// TestCadaFasePintaLoSuyo recorre los siete estados y comprueba que ninguno se queda en
// blanco y que ninguno pinta lo del vecino. Es lo que evita que un estado de error se
// enseñe como una lista vacía.
func TestCadaFasePintaLoSuyo(t *testing.T) {
	tr := traductor(t)
	casos := []struct {
		fase  Fase
		busca string
	}{
		{Vacia, tr.T("files.empty.title", nil)},
		{Fallo, tr.T("files.error.loadTitle", nil)},
		{SinSesion, tr.T("files.error.sessionTitle", nil)},
		{Prohibida, tr.T("forbidden.title", nil)},
		{SinConexion, tr.T("offline.status", nil)},
	}
	for _, c := range casos {
		e := NuevoExplorador(tr)
		e.Fase, e.Ventana = c.fase, 1280
		obj := e.Objeto(tema.Claro)
		obj.Resize(fyne.NewSize(1020, 744))
		if !contieneTexto(obj, c.busca) {
			t.Errorf("la fase %d no escribe %q", c.fase, c.busca)
		}
	}

	// Cargando pinta el esqueleto: nueve filas más los tres rótulos de columna.
	e := NuevoExplorador(tr)
	e.Fase, e.Ventana = Cargando, 1280
	if n := cuentaRectangulos(e.esqueletoCuerpo(tema.Claro)); n < FilasEsqueleto {
		t.Errorf("el esqueleto pinta %d bloques y tiene que pintar al menos %d filas", n, FilasEsqueleto)
	}
}

// TestSinConexionApagaLasMutaciones es el contrato offline: sin red no se sube, no se
// crea y no se selecciona. Se comprueba en el estado de los controles, no en el texto.
func TestSinConexionApagaLasMutaciones(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = SinConexion, 1280
	if !e.SinRed() {
		t.Fatal("la fase SinConexion no se reconoce como sin red")
	}
	cab := e.cabecera(tema.Claro)
	c, _ := cab.(*fyne.Container)
	lay, _ := c.Layout.(*CabeceraVista)
	for i, o := range lay.Acciones[1:] {
		b, ok := o.(*ui.Boton)
		if !ok {
			continue
		}
		if !b.Inerte {
			t.Errorf("el botón %d (%q) sigue activo sin conexión", i+1, b.Texto)
		}
	}
	// Y la lista no ofrece casillas: marcar sin poder mover ni borrar no lleva a nada.
	e.Fase = SinConexion
	if e.Marcados() != 0 {
		t.Error("hay selección en el estado sin conexión")
	}
}

// TestFaseDeTraduceElError es la cuenta que decide qué pantalla se ve. Va con su
// inversión: sin red NO puede salir «fallo de red», que es el error que se ve como un
// servicio caído cuando lo que pasa es que no hay wifi.
func TestFaseDeTraduceElError(t *testing.T) {
	// Errores de verdad del cliente: `FaseDe` los reconoce con `errors.As`, así que un
	// tipo propio no valdría y el test pasaría sin comprobar nada.
	err401 := &api.Error{Status: 401}
	err403 := &api.Error{Status: 403}
	err500 := &api.Error{Status: 500}
	casos := []struct {
		nombre    string
		err       error
		hayRed    bool
		elementos int
		quiero    Fase
	}{
		{"sin red gana a todo", err500, false, 0, SinConexion},
		{"401 es la sesión", err401, true, 0, SinSesion},
		{"403 es permiso", err403, true, 0, Prohibida},
		{"500 es fallo", err500, true, 0, Fallo},
		{"sin error y sin elementos, vacío", nil, true, 0, Vacia},
		{"con elementos, éxito", nil, true, 3, Exito},
	}
	for _, c := range casos {
		if got := FaseDe(c.err, c.hayRed, c.elementos); got != c.quiero {
			t.Errorf("%s: sale %d y quiero %d", c.nombre, got, c.quiero)
		}
	}
}

// ---- La papelera ----

// TestLaCuentaAtrasDeLaPapelera comprueba las tres ramas de `purgePhrase` y el umbral
// de urgencia, con el reloj fijado a mano.
func TestLaCuentaAtrasDeLaPapelera(t *testing.T) {
	tr := traductor(t)
	p := NuevaVistaPapelera(tr)
	p.Ahora = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	dentroDe := func(d time.Duration) Elemento {
		return Elemento{Nombre: "x.pdf", Tamano: 10, Purga: p.Ahora.Add(d)}
	}
	if frase := p.frasePurga(dentroDe(2 * time.Hour)); frase != tr.T("purge.today", nil) {
		t.Errorf("hoy: %q", frase)
	}
	if frase := p.frasePurga(dentroDe(25 * time.Hour)); frase != tr.T("purge.tomorrow", nil) {
		t.Errorf("mañana: %q", frase)
	}
	if frase := p.frasePurga(Elemento{Nombre: "x"}); frase != "" {
		t.Errorf("sin fecha de purga no se afirma nada, y se ha escrito %q", frase)
	}
	// Tres días o menos es urgente; cuatro, no.
	if !Urgente(p.Ahora, p.Ahora.Add(3*24*time.Hour)) {
		t.Error("tres días no se marca como urgente")
	}
	if Urgente(p.Ahora, p.Ahora.Add(5*24*time.Hour)) {
		t.Error("cinco días se marca como urgente")
	}
}

// ---- El píxel ----

// TestLaFilaDelExploradorSePintaDondeProduccion captura la lista con el pintor de
// software —sin CGO y sin servidor gráfico— y comprueba DOS cosas en los píxeles: que
// el hairline de una fila cae en la fila 50 (o sea que la fila mide 51, no 50 ni 53) y
// que su color es el `--sytue-border` al 60 % que se midió en producción.
//
// Con `SYTUE_CAPTURA=/ruta.png` deja la imagen, que es la que se compara con
// `drive/archivos.ancho.png`.
func TestLaFilaDelExploradorSePintaDondeProduccion(t *testing.T) {
	const ancho = 999
	f := ui.NuevaFilaDrive("Contratos", "")
	f.Carpeta = true
	f.Tamano, f.Fecha = "—", "6 sept 2026"

	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ancho, ui.MedidaFilaDrive()))
	img := w.Canvas().Capture()

	if b := img.Bounds(); float32(b.Dy()) != ui.MedidaFilaDrive() {
		t.Fatalf("la fila se pinta con %d de alto y declara %v", b.Dy(), ui.MedidaFilaDrive())
	}
	// El hairline es la ÚLTIMA fila de píxeles, y por encima está el fondo.
	borde := px(img, 10, img.Bounds().Dy()-1)
	fondo := px(img, 10, img.Bounds().Dy()-3)
	if cerca(borde, fondo) {
		t.Errorf("no hay filete abajo: el último píxel (%v) es igual que el fondo (%v)", borde, fondo)
	}
	// (227,230,232) es `--sytue-border` opaco sobre blanco; aquí va al 60 % sobre `bg`
	// (248,245,243), así que sale más claro. Lo que se comprueba es que NO es el
	// borde a plena fuerza ni el fondo: es el `border-border/60` de la web.
	if borde.R < fondo.R-40 {
		t.Errorf("el filete pinta %v, demasiado oscuro para un `border-border/60`", borde)
	}
	guardar(t, img)
}

// El tercer estado del «seleccionar todo» NO se prueba aquí.
//
// `Eleccion.Parcial` está encargado a quien remata el catálogo (2026-09-22), así que
// esta app pasa `Algunas` a `ui.CabeceraDrive` —el campo ya existe— y el guardián del
// píxel va con el widget, no con la pantalla. El `// ponytail:` sigue puesto en
// `ui/fila.go`.

// ---- Andamiaje ----

func px(im image.Image, x, y int) color.NRGBA {
	r, g, b, a := im.At(x, y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// cerca admite ±1 nivel de 255: el alfa se cuantiza y ningún gate puede exigir byte a
// byte (ADR 0048).
func cerca(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

func guardar(t *testing.T, img image.Image) {
	t.Helper()
	destino := os.Getenv("SYTUE_CAPTURA")
	if destino == "" {
		return
	}
	f, err := os.Create(destino)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura escrita en %s", destino)
}

// recorrer visita el árbol entero, bajando también por el renderer de cada widget: los
// textos de `ui.Vacio` y de un botón viven ahí y no en un contenedor.
func recorrer(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
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

// contieneTexto busca un rótulo en el árbol. Es lo que permite comprobar que una fase
// escribe SU texto sin depender de dónde lo coloque el layout.
func contieneTexto(o fyne.CanvasObject, busca string) bool {
	encontrado := false
	recorrer(o, func(x fyne.CanvasObject) {
		if t, ok := x.(*canvas.Text); ok && t.Text == busca {
			encontrado = true
		}
	})
	return encontrado
}

func cuentaRectangulos(o fyne.CanvasObject) int {
	n := 0
	recorrer(o, func(x fyne.CanvasObject) {
		if _, ok := x.(*canvas.Rectangle); ok {
			n++
		}
	})
	return n
}

// TestLaPantallaEnteraSePinta monta el carril y el explorador con datos y los captura a
// 1280×800, que es la medida del barrido de la web.
//
// No es solo para mirar la imagen. Pintar la pantalla ENTERA es lo único que descubre un
// token de color mal escrito: `tema.Color` entra en pánico con un nombre desconocido AL
// PINTAR y no al compilar, así que una rama que no se pinte nunca en un test se cae la
// primera vez que alguien la abre (ya pasó con `sidebar-hover` contra
// `sidebar-hover-bg`). Aquí se pinta cada fase, en los dos temas.
//
// Con `SYTUE_CAPTURA=/ruta.png` deja además la imagen de la fase de éxito en claro, que
// es la que se compara con `medidas/drive/archivos.ancho.png`.
func TestLaPantallaEnteraSePinta(t *testing.T) {
	tr := traductor(t)
	const ancho, alto = 1280, 800

	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		for _, fase := range []Fase{Cargando, Exito, Vacia, Fallo, SinSesion, Prohibida, SinConexion} {
			for _, rejilla := range []bool{false, true} {
				img := capturaPantalla(t, tr, v, fase, rejilla, ancho, alto)
				if b := img.Bounds(); b.Dx() != ancho || b.Dy() != alto {
					t.Fatalf("la captura mide %dx%d", b.Dx(), b.Dy())
				}
				if v == tema.Claro && fase == Exito && !rejilla {
					// El carril ocupa los primeros 260 y es de tinta.
					if got := px(img, 100, 300); !cerca(got, tema.Color("sidebar-bg", v)) {
						t.Errorf("el carril pinta %v y el token `sidebar-bg` es %v", got, tema.Color("sidebar-bg", v))
					}
					// Y la cabecera de vista va sobre `surface`, a la derecha del carril.
					if got := px(img, 600, 10); !cerca(got, tema.Color("surface", v)) {
						t.Errorf("la cabecera de vista pinta %v y `surface` es %v", got, tema.Color("surface", v))
					}
					guardar(t, img)
				}
			}
		}
	}

	// Y la papelera, que es la otra pantalla portada.
	for _, fase := range []Fase{Cargando, Exito, Vacia, Fallo, SinConexion} {
		p := NuevaVistaPapelera(tr)
		p.Fase, p.Ventana = fase, ancho
		p.Ahora = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
		p.Elementos = elementosDePrueba(p.Ahora)
		w := test.NewWindow(p.Objeto(tema.Claro))
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ancho-tema.AnchoLateral, alto))
		w.Canvas().Capture()
		w.Close()
	}
}

// capturaPantalla monta carril + explorador como lo hace la app y devuelve el lienzo.
func capturaPantalla(t *testing.T, tr *i18n.Translator, v tema.Variante, fase Fase, rejilla bool,
	ancho, alto float32,
) image.Image {
	t.Helper()
	e := NuevoExplorador(tr)
	e.Fase, e.Ventana, e.Rejilla = fase, ancho, rejilla
	e.Elementos = elementosDePrueba(time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC))

	lat := NuevoLateral(tr, MiUnidad, nil)
	lat.Cuota = &Cuota{Usado: 21 << 20, Tope: 5 << 30}
	arm := ui.NuevoArmazon(e.Objeto(v))
	arm.Menu = lat.Objeto(v)

	w := test.NewWindow(arm)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ancho, alto))
	return w.Canvas().Capture()
}

// elementosDePrueba son las diez filas de la captura de producción, con los mismos
// nombres: así las dos imágenes se comparan fila a fila.
func elementosDePrueba(ahora time.Time) []Elemento {
	carpetas := []string{"Attachments", "Contratos", "Diseño", "Documentación", "Fotografías", "Fotos", "Operaciones"}
	out := make([]Elemento, 0, len(carpetas)+3)
	for _, n := range carpetas {
		out = append(out, Elemento{ID: n, Nombre: n, Carpeta: true, Tamano: -1, Modificado: ahora})
	}
	out = append(out,
		Elemento{ID: "pdf", Nombre: "PDF", Carpeta: true, Tamano: -1, Modificado: ahora},
		Elemento{ID: "docs", Nombre: "Sytue Docs", Carpeta: true, Tamano: -1, Modificado: ahora},
		Elemento{
			ID: "md", Nombre: "Notas de la reunión.md", Tamano: 4096,
			Modificado: ahora, Purga: ahora.Add(48 * time.Hour),
		},
	)
	return out
}

// abridorDoble apunta a dónde se ha mandado a la persona, sin sistema operativo debajo.
type abridorDoble struct{ urls []string }

func (a *abridorDoble) OpenURL(u *url.URL) error {
	a.urls = append(a.urls, u.String())
	return nil
}

// TestUnFicheroAbreEnSuApp: lo que manda `docs/CONVENTIONS.md` §13.6. Pulsar un fichero
// ya no lo baja siempre: si hay un producto de la suite dueño de ese formato, se le pasa
// la REFERENCIA y se abre allí. Lo que no reclama nadie se sigue descargando, y por eso
// `abrirEnSuApp` devuelve false — que es lo que aquí se comprueba, y no `abrir`, que
// lanzaría la descarga contra un servicio nulo.
//
// La ruta se arma con las MIGAS cuando el servicio no la manda (una carpeta normal),
// porque CertiPDF abre por ruta y sin ella su página no tiene qué abrir.
//
// Visto en rojo el 2026-09-22 devolviendo `false` a secas desde `abrirEnSuApp`: los tres
// primeros casos cayeron a la vez.
func TestUnFicheroAbreEnSuApp(t *testing.T) {
	for _, c := range []struct {
		nombre string
		quiero string
	}{
		{"acta.odt", "sytue-docs:/open-from-drive?file=f1&volver=%2FContratos%2F2026"},
		{"cuentas.xlsx", "sytue-sheets:/open-from-drive?file=f1&volver=%2FContratos%2F2026"},
		{"charla.pptx", "sytue-slides:/open-from-drive?file=f1&volver=%2FContratos%2F2026"},
		{"informe.pdf", "sytue-pdf:/pdf?path=%2FContratos%2F2026%2Finforme.pdf&volver=%2FContratos%2F2026"},
		{"playa.jpg", "sytue-photos:/abrir?file=f1&volver=%2FContratos%2F2026"},
		{"copia.zip", ""},
		{"Notas de la reunión.md", ""},
	} {
		a := &abridorDoble{}
		p := NuevaPantalla(traductor(t), nil, nil, nil)
		p.App, p.Apex = a, "sytue.eu"
		p.explor.Ruta = []Elemento{{ID: "c1", Nombre: "Contratos"}, {ID: "c2", Nombre: "2026"}}

		abierto := p.abrirEnSuApp(Elemento{ID: "f1", Nombre: c.nombre})
		if c.quiero == "" {
			if abierto || len(a.urls) != 0 {
				t.Errorf("%s no tiene app dueña y se ha abierto en %v", c.nombre, a.urls)
			}
			continue
		}
		if !abierto || len(a.urls) != 1 || a.urls[0] != c.quiero {
			t.Errorf("%s abre %v, quiero [%s]", c.nombre, a.urls, c.quiero)
		}
	}

	// En una búsqueda la ruta la manda el servicio y es la que vale: el fichero puede
	// estar en cualquier sitio del árbol, no en la carpeta que se esté mirando.
	a := &abridorDoble{}
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	p.App, p.Apex = a, "sytue.eu"
	p.explor.Ruta = []Elemento{{ID: "c1", Nombre: "Contratos"}}
	p.abrirEnSuApp(Elemento{ID: "f2", Nombre: "acta.odt", Ruta: "/Otra/acta.odt"})
	if len(a.urls) != 1 || !strings.Contains(a.urls[0], "volver=%2FOtra") {
		t.Errorf("en una búsqueda manda la ruta del servicio: %v", a.urls)
	}
}
