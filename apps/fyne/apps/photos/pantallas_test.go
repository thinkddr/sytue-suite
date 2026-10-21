// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// La ventana con la que se midió producción: 1280×800, es-ES, tema claro.
const (
	ventanaAncho float32 = 1280
	ventanaAlto  float32 = 800
)

func vistaDePrueba(t *testing.T, teca *Fototeca) Vista {
	t.Helper()
	tr, err := Traductor("es")
	if err != nil {
		t.Fatalf("no se pudo montar el traductor: %v", err)
	}
	return Vista{T: tr, Teca: teca, Ventana: ventanaAncho, Variante: tema.Claro}
}

// fotosDePrueba son las de un mes, con nombres y fechas reales para que los rótulos
// midan lo que miden.
func fotosDePrueba(n int) []api.Photo {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	fotos := make([]api.Photo, n)
	for i := range fotos {
		fotos[i] = api.Photo{
			ID: fmt.Sprintf("f%02d", i), Name: fmt.Sprintf("IMG_%04d.jpg", 1000+i),
			Width: 4032, Height: 3024, SizeBytes: 2_400_000,
			TakenAt: base.AddDate(0, 0, -i), TakenFromEXIF: i%4 != 0,
			Favorite: i%5 == 0, Camera: "Pixel 9 Pro", Lens: "6.9 mm f/1.7",
		}
	}
	return fotos
}

// tecaCargada mete una foto de verdad —un degradado, no un color plano— por cada id.
func tecaCargada(fotos []api.Photo) *Fototeca {
	f := NuevaFototeca()
	datos := pngDe(degradado(512, 384))
	for _, p := range fotos {
		f.Poner(p.ID, datos)
	}
	return f
}

func degradado(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			m.Set(x, y, color.RGBA{
				R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 140, A: 255,
			})
		}
	}
	return m
}

// guardar deja la captura en SYTUE_CAPTURA para poder MIRARLA. Un guardián de píxel
// dice si un número cuadra; en una galería, mirarla es la otra mitad del trabajo.
func guardar(t *testing.T, nombre string, img image.Image) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURA")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("no se pudo crear %s: %v", dir, err)
	}
	f, err := os.Create(filepath.Join(dir, nombre+".png"))
	if err != nil {
		t.Fatalf("no se pudo crear la captura: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("no se pudo escribir la captura: %v", err)
	}
}

// pintaPantalla pinta una pantalla entera a la ventana de referencia y devuelve la
// captura, con las miniaturas YA preparadas.
//
// A ×1 y no a ×2, al revés que los guardianes de píxel. Aquí no se mide un medio
// píxel: se comprueba que la pantalla se pinta entera sin reventar y se guarda para
// mirarla. A ×2 son 2560×1600 por captura y setenta y dos capturas, y bajo `-race`
// eso eran 404 s de suite — un coste que el pre-push y el CI pagan en cada empujón
// para no enterarse de nada más.
func pintaPantalla(t *testing.T, teca *Fototeca, o fyne.CanvasObject) image.Image {
	t.Helper()
	return pintaPantallaA(t, teca, o, ventanaAncho, ventanaAlto)
}

// pintaPantallaA es pintaPantalla a otra ventana: la de móvil, 360×740.
func pintaPantallaA(t *testing.T, teca *Fototeca, o fyne.CanvasObject, w, h float32) image.Image {
	t.Helper()
	o.Resize(fyne.NewSize(w, h))
	pintaA(t, 1, w, h, o)
	if teca != nil {
		teca.Esperar()
	}
	return pintaA(t, 1, w, h, o)
}

// TestLasDiezPantallasEnSusSeisEstados es el barrido: cada pantalla en cada estado, sin
// que nada entre en pánico y con la captura guardada.
//
// El pánico NO es una preocupación teórica: `tema.Color` revienta con un nombre de
// token desconocido AL PINTAR y no al compilar, así que una pantalla que nadie haya
// pintado puede tener un token inventado dentro. Este barrido es lo que lo saca.
func TestLasDiezPantallasEnSusSeisEstados(t *testing.T) {
	fotos := fotosDePrueba(21)
	teca := tecaCargada(fotos)
	base := vistaDePrueba(t, teca)
	estados := map[string]Estado{
		"exito": Exito, "cargando": Cargando, "vacia": Vacia,
		"error": Error, "sinconexion": SinConexion, "sinpermiso": SinPermiso,
	}
	if conRaza {
		// Ver conRaza: este barrido busca pánicos y tokens inventados, no carreras, y
		// con `-race` se comía más de la mitad de los diez minutos del paquete.
		t.Skip("con -race el barrido de capturas no cabe en el tiempo del paquete; corre sin él")
	}
	for nombre, construir := range pantallasDePrueba(fotos, base) {
		for etiqueta, e := range estados {
			t.Run(nombre+"/"+etiqueta, func(t *testing.T) {
				v := base
				v.Estado = e
				img := pintaPantalla(t, teca, construir(v))
				guardar(t, nombre+"-"+etiqueta, img)
			})
		}
	}
}

// TestLasPantallasAAnchoDeMovil pinta cada pantalla en su estado bueno a 360×740, que es
// el ancho de móvil con el que se compara la web responsive: ahí los rótulos de los
// botones pasan a ser solo icono, la galería cambia de tesela y el editor pone el panel
// debajo.
func TestLasPantallasAAnchoDeMovil(t *testing.T) {
	if conRaza {
		t.Skip("con -race el barrido de móvil no cabe en el tiempo del paquete; corre sin él")
	}
	const w, h = 360, 740
	fotos := fotosDePrueba(21)
	teca := tecaCargada(fotos)
	base := vistaDePrueba(t, teca)
	base.Ventana = w
	for nombre, construir := range pantallasDePrueba(fotos, base) {
		t.Run(nombre, func(t *testing.T) {
			guardar(t, nombre+"-movil", pintaPantallaA(t, teca, construir(base), w, h))
		})
	}
}

// TestLasPantallasEnOscuro pinta cada pantalla en su estado bueno con el tema oscuro
// elegido (el del menú de cuenta, que manda sobre el del sistema): así un token que solo
// existe en claro, o un color literal que no cambia, sale en la captura.
func TestLasPantallasEnOscuro(t *testing.T) {
	if conRaza {
		t.Skip("con -race el barrido de capturas no cabe en el tiempo del paquete; corre sin él")
	}
	tema.Elegir("oscuro")
	t.Cleanup(func() { tema.Elegir("") })
	fotos := fotosDePrueba(21)
	teca := tecaCargada(fotos)
	base := vistaDePrueba(t, teca)
	base.Variante = tema.Oscuro
	for nombre, construir := range pantallasDePrueba(fotos, base) {
		t.Run(nombre, func(t *testing.T) {
			img := pintaPantalla(t, teca, construir(base))
			guardar(t, nombre+"-oscuro", img)
			// Una esquina de la ventana: el papel oscuro y no el claro.
			if got := px(img, 2, int(ventanaAlto)-2); cerca(got, tema.Color("bg", tema.Claro)) {
				t.Errorf("abajo a la izquierda hay %v, el papel CLARO, con el tema oscuro elegido", got)
			}
		})
	}
}

// pantallasDePrueba son las diez pantallas (más la selección, favoritas y «por aquí»)
// con datos de verdad.
func pantallasDePrueba(fotos []api.Photo, base Vista) map[string]func(Vista) fyne.CanvasObject {
	grupos := PorMes(fotos, base.T)
	album := api.Album{ID: "a1", Title: "Verano en Cadaqués", CoverAssetID: "f00", Count: 21}

	return map[string]func(Vista) fyne.CanvasObject{
		"cuadricula": func(v Vista) fyne.CanvasObject {
			return PantallaGaleria(v, Galeria{Grupos: grupos})
		},
		"favoritas": func(v Vista) fyne.CanvasObject {
			return PantallaGaleria(v, Galeria{Grupos: grupos, SoloFavoritas: true})
		},
		"seleccion": func(v Vista) fyne.CanvasObject {
			return PantallaGaleria(v, Galeria{
				Grupos: grupos, EnSeleccion: true,
				Seleccion: map[string]bool{"f00": true, "f01": true, "f02": true},
			})
		},
		"albumes": func(v Vista) fyne.CanvasObject {
			return PantallaAlbumes(v, Albumes{Lista: []api.Album{album, album, album}})
		},
		"album": func(v Vista) fyne.CanvasObject {
			return PantallaAlbum(v, AlbumAbierto{Album: album, Fotos: fotos})
		},
		"papelera": func(v Vista) fyne.CanvasObject {
			return PantallaPapelera(v, Papelera{Fotos: fotos[:8]})
		},
		"repetidas": func(v Vista) fyne.CanvasObject {
			return PantallaRepetidas(v, Repetidas{Grupos: [][]api.Photo{fotos[:3], fotos[3:5]}})
		},
		"parecidas": func(v Vista) fyne.CanvasObject {
			return PantallaEncontradas(v, Encontradas{Referencias: fotos[:1], Fotos: fotos[1:13]})
		},
		"cerca": func(v Vista) fyne.CanvasObject {
			return PantallaEncontradas(v, Encontradas{
				Referencias: fotos[:1], Fotos: fotos[1:9],
				Cerca: true, Coordenadas: "42,2887, 3,2779",
			})
		},
		"publica": func(v Vista) fyne.CanvasObject {
			return PantallaPublica(v, album.Title, album.Count, fotos[:12], nil)
		},
		"visor": func(v Vista) fyne.CanvasObject {
			// Los mismos mandos que el visor de la galería en la web (`caso-photos-visor`).
			return PantallaVisor(v, Visor{
				Foto: fotos[0], ConFicha: true,
				Descripcion: "Una cala con barcas al atardecer.", HaySiguiente: true,
				ConFavorita: true, ConBorrar: true, ConDescribir: true,
			})
		},
		"editor": func(v Vista) fyne.CanvasObject {
			r := conValor(api.RecetaVacia(), api.PasoExposicion, 0.5)
			l := NuevoLienzoEditor()
			l.Poner(nrgba(bandas(600, 400)), r, FondoCrema)
			return PantallaEditor(v, Editor{Foto: fotos[0], Receta: r, Sucia: true, Lienzo: l, Cargando: v.Estado == Cargando})
		},
		"editor-ia": func(v Vista) fyne.CanvasObject {
			exp, temp := 0.3, 0.4
			return PantallaEditor(v, Editor{
				Foto: fotos[0], Receta: api.RecetaVacia(), PestanaIA: true,
				IAError: v.Traduce("editor.aiFailed"),
				IAResultado: &api.EdicionIA{
					Explicacion: "Más luz y un tono algo más cálido.",
					Ajustes:     api.AjustesIA{Exposicion: &exp, Temperatura: &temp},
				},
			})
		},
	}
}

// TestLaCuadriculaMideLoQueMideProduccion ata la galería a los números del volcado: la
// caja de contenido de 1216 y el max-w-7xl de 1280, ambos con rem de 16.
//
// Chromium con la hoja real (2026-10-04): grid en x=32, contenido 1216;
// en las otras páginas, px-4 deja contenido 1248 en x=16.
func TestLaCuadriculaMideLoQueMideProduccion(t *testing.T) {
	x, ancho := CajaGaleria(1280)
	if x != 32 {
		t.Errorf("la caja de la galería empieza en %v y Chromium la mide en 32", x)
	}
	if ancho != 1216 {
		t.Errorf("la caja mide %v y Chromium mide 1216", ancho)
	}
	// Y la de las otras nueve, con max-w-7xl y px-4.
	x, ancho = CajaPagina(1280)
	if x != 16 || ancho != 1248 {
		t.Errorf("la caja de página mide %v en x=%v y Chromium mide 1248 en 16", ancho, x)
	}
}

// TestLaNavSonCincoPestanasDe32: Chromium (2026-10-04), con la hoja compartida
// real, rem de 16 y los cinco rótulos de Nav.tsx anteriores a Compartido conmigo.
func TestLaNavSonCincoPestanasDe32(t *testing.T) {
	v := vistaDePrueba(t, nil)
	n := Nav(v, SecFotos, nil)
	if len(n.Objects) != 6 {
		t.Fatalf("la nav tiene %d pestañas y Nav.tsx pinta 6", len(n.Objects))
	}
	const total = 450.578125
	anchos := []float32{71.09375, 90.65625, 88.765625, 96.265625, 87.796875}
	cinco := append(append([]fyne.CanvasObject{}, n.Objects[:SecCompartidos]...), n.Objects[SecCompartidos+1:]...)
	for i, o := range cinco {
		m := o.MinSize()
		if m.Height != 32 || m.Width < anchos[i]-0.25 || m.Width > anchos[i]+0.25 {
			t.Errorf("la pestaña %d mide %v; Chromium da %.3f × 32", i, m, anchos[i])
		}
	}
	nueva := n.Objects[SecCompartidos].MinSize()
	if nueva.Height != 32 {
		t.Errorf("Compartido conmigo mide %v y h-8 son 32", nueva.Height)
	}
	if got := n.MinSize().Width - nueva.Width - HuecoNav; got < total-0.25 || got > total+0.25 {
		t.Errorf("las cinco pestañas miden %v; Chromium da %v", got, float32(total))
	}
}

// TestElGlifoDeRepetidasEsElDeNavTsx: «repetidas» es un glifo PROPIO de esta app y no
// está en el catálogo compartido —en la web tampoco—, así que su guardián es este:
// compara la `d` con la que hay escrita en Nav.tsx.
//
// Sin él, el glifo se copia una vez y se queda atrás para siempre, que es exactamente
// lo que le pasó a los doce `middleware.ts`.
func TestElGlifoDeRepetidasEsElDeNavTsx(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/photos/src/components/Nav.tsx")
	i := strings.Index(fuente, "function GlifoRepetidas")
	if i < 0 {
		t.Fatal("Nav.tsx ya no declara `GlifoRepetidas`: el espejo hay que rehacerlo")
	}
	m := regexp.MustCompile(`<(?:path|Glifo) d="([^"]+)"`).FindStringSubmatch(fuente[i:])
	if m == nil {
		t.Fatal("`GlifoRepetidas` ya no tiene un `d=…`")
	}
	if m[1] != glifoRepetidas {
		t.Errorf("el glifo de «repetidas» ha cambiado en Nav.tsx:\n  web:  %s\n  aquí: %s", m[1], glifoRepetidas)
	}
}

// TestLaPestanaActivaLlevaFondoYLaOtraNo: `bg-bg-subtle` en la activa y nada en las
// demás, que es lo único que distingue dónde estás.
func TestLaPestanaActivaLlevaFondoYLaOtraNo(t *testing.T) {
	v := vistaDePrueba(t, nil)
	for _, c := range []struct {
		activa bool
		quiero color.NRGBA
		que    string
	}{
		{true, tema.Color("bg-subtle", tema.Claro), "bg-bg-subtle"},
		{false, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, "el lienzo, sin fondo propio"},
	} {
		p := NuevaPestanaNav(v.Traduce("nav.photos"), iconosNav[SecFotos], c.activa, nil)
		m := MedidaPestanaNav(p)
		p.Resize(m)
		img := pinta(t, m.Width+8, m.Height+8, p)
		// Una esquina de dentro de la pestaña, lejos del icono y del rótulo.
		got := px(img, int(m.Width*escala)-4, int(m.Height*escala)/2)
		if !cerca(got, c.quiero) {
			t.Errorf("con activa=%v la pestaña pinta %v y toca %s (%v)", c.activa, got, c.que, c.quiero)
		}
	}
}

// TestLaTeselaSeleccionadaLlevaSuAnillo: el `ring-2 ring-primary` va POR FUERA de la
// caja (es un box-shadow, no un borde), así que ni encoge la foto ni la desplaza.
func TestLaTeselaSeleccionadaLlevaSuAnillo(t *testing.T) {
	lado := float32(201.078)
	teca := tecaCargada([]api.Photo{{ID: "a"}})
	m := NuevaMiniatura(api.Photo{ID: "a", Name: "a.jpg"}, teca)
	m.Seleccionando, m.Seleccionada, m.Rotulo = true, true, "Seleccionar a.jpg"
	m.Resize(fyne.NewSize(lado, lado))
	// Se pinta con margen a la izquierda para que el anillo, que sale de la caja,
	// quepa dentro del lienzo.
	desplazada := m
	desplazada.Move(fyne.NewPos(8, 8))
	pinta(t, lado+24, lado+24, desplazada)
	teca.Esperar()
	img := pinta(t, lado+24, lado+24, desplazada)
	// Un píxel a la izquierda del borde de la tesela: ahí va el anillo.
	got := px(img, int(7*escala), int((8+lado/2)*escala))
	if quiero := tema.Color("primary", tema.Claro); !cerca(got, quiero) {
		t.Errorf("a un píxel por fuera de la tesela hay %v y el anillo es `primary` %v", got, quiero)
	}
	guardar(t, "tesela-seleccionada", img)
}

// TestElArcoDeLaTeselaSeFundeYNoSeEnnegrece es el guardián de un fallo que NINGÚN
// número enseñó: el arco de la esquina salía como una línea casi negra porque
// `image.RGBA` es alfa PREMULTIPLICADO y `redondear` bajaba solo el byte del alfa,
// dejando píxeles con R por encima de A que desbordan al desmultiplicar.
//
// Se comprueba en el mapa de bits y no en la captura: un píxel del arco tiene que
// quedar entre el negro y el color de la foto, nunca por debajo.
func TestElArcoDeLaTeselaSeFundeYNoSeEnnegrece(t *testing.T) {
	// Una fuente gris uniforme: así el único gradiente posible en la esquina es el
	// del desvanecido.
	gris := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := range 256 {
		for x := range 256 {
			gris.Set(x, y, color.RGBA{R: 200, G: 200, B: 200, A: 255})
		}
	}
	m := recortarYEscalar(pngDe(gris), 200, 200, 8)
	for _, p := range [][2]int{{1, 2}, {2, 1}, {2, 2}, {3, 3}} {
		c := m.RGBAAt(p[0], p[1])
		// Premultiplicado: ningún canal puede pasar del alfa.
		if c.R > c.A || c.G > c.A || c.B > c.A {
			t.Errorf("en (%d,%d) hay %v con alfa %d: un canal por encima del alfa desborda "+
				"al desmultiplicar y pinta casi negro", p[0], p[1], c, c.A)
		}
		// Y el color desmultiplicado tiene que seguir siendo el gris de la fuente.
		// Solo donde el alfa da para comprobarlo: premultiplicar y volver a dividir
		// con enteros pierde precisión, y con un alfa de 19 el redondeo ya vale 13
		// niveles. Ahí no se ve nada; lo que se veía era el desbordamiento.
		if c.A >= 64 {
			if recto := int(c.R) * 255 / int(c.A); recto < 194 || recto > 206 {
				t.Errorf("en (%d,%d) el color sin premultiplicar es %d y la fuente es 200",
					p[0], p[1], recto)
			}
		}
	}
}

// TestLaPapeleraVaEnLasMismasFilas: la papelera era la única rejilla con la proporción
// de cada foto; ahora todas las de Photos son las filas justificadas de la galería.
func TestLaPapeleraVaEnLasMismasFilas(t *testing.T) {
	v := vistaDePrueba(t, nil)
	p := PantallaPapelera(v, Papelera{Fotos: fotosDePrueba(3)})
	if !tieneLayout(p, func(l fyne.Layout) bool { _, ok := l.(Filas); return ok }) {
		t.Error("la papelera no usa las filas justificadas")
	}
}

// tieneLayout dice si algún contenedor del árbol usa un layout que cumple eso.
func tieneLayout(o fyne.CanvasObject, es func(fyne.Layout) bool) bool {
	c, ok := o.(*fyne.Container)
	if !ok {
		return false
	}
	if c.Layout != nil && es(c.Layout) {
		return true
	}
	for _, h := range c.Objects {
		if tieneLayout(h, es) {
			return true
		}
	}
	return false
}

// TestElRecuentoUsaElPluralDelIdioma: «1 foto» y «21 fotos» salen del catálogo, no de
// concatenar una «s».
func TestElRecuentoUsaElPluralDelIdioma(t *testing.T) {
	v := vistaDePrueba(t, nil)
	uno, varias := Recuento(v, "albums.count", 1), Recuento(v, "albums.count", 21)
	if uno == varias {
		t.Errorf("«1» y «21» dan el mismo texto (%q): el plural no se está aplicando", uno)
	}
	if !strings.Contains(varias, "21") {
		t.Errorf("el recuento de 21 es %q y no lleva el número", varias)
	}
}

// TestLosSeisEstadosDicenAlgoDistinto: los cinco que no son éxito tienen que pintar
// algo, y textos distintos entre sí. Sin esto, «sin conexión» y «sin permiso» podrían
// enseñar lo mismo y nadie lo notaría.
func TestLosSeisEstadosDicenAlgoDistinto(t *testing.T) {
	v := vistaDePrueba(t, nil)
	vistos := map[string]Estado{}
	for _, e := range []Estado{Cargando, Vacia, Error, SinConexion, SinPermiso} {
		w := v
		w.Estado = e
		cuerpo := CuerpoDeEstado(w, EstadoVacio(w, "gallery.empty.title", "gallery.empty.description", nil))
		if cuerpo == nil {
			t.Errorf("el estado %d no pinta nada", e)
			continue
		}
		texto := textoDe(cuerpo)
		if otro, ya := vistos[texto]; ya {
			t.Errorf("los estados %d y %d dicen lo mismo (%q)", otro, e, texto)
		}
		vistos[texto] = e
	}
	w := v
	w.Estado = Exito
	if CuerpoDeEstado(w, nil) != nil {
		t.Error("el estado de éxito pinta un cuerpo de estado: tiene que dejar paso a los datos")
	}
}

// textoDe saca lo que dice un cuerpo de estado, sea un `EmptyState` o un párrafo.
func textoDe(o fyne.CanvasObject) string {
	switch t := o.(type) {
	case *ui.Vacio:
		return t.Titulo + "|" + t.Descripcion
	case *fcanvas.Text:
		return t.Text
	}
	return fmt.Sprintf("%T", o)
}

// TestElDeslizadorSeMueveConLasFlechas: el editor tiene cuatro y sin teclado no los
// mueve quien no puede arrastrar.
func TestElDeslizadorSeMueveConLasFlechas(t *testing.T) {
	d := ui.NuevoDeslizador(-5, 5, 0, nil)
	d.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if d.Valor <= 0 {
		t.Errorf("la flecha derecha dejó el valor en %v y tenía que subir", d.Valor)
	}
	d.Poner(100)
	if d.Valor != 5 {
		t.Errorf("el valor se fue a %v y el tope es 5", d.Valor)
	}
	d.Poner(-100)
	if d.Valor != -5 {
		t.Errorf("el valor se fue a %v y el suelo es −5", d.Valor)
	}
}

// TestLosAjustesSonLosCuatroDeEditorTsx: los cuatro deslizadores en el orden de
// `DESLIZADORES` y con los recorridos de `RANGOS`, leídos de la web.
func TestLosAjustesSonLosCuatroDeEditorTsx(t *testing.T) {
	editor := webref.Read(t, "apps/frontend/apps/photos/src/components/Editor.tsx")
	receta := webref.Read(t, "apps/frontend/apps/photos/src/editor/receta.ts")
	orden := "['" + strings.Join(Deslizadores[:], "', '") + "']"
	if !strings.Contains(editor, "DESLIZADORES: Deslizador[] = "+orden) {
		t.Errorf("Editor.tsx ya no pinta los deslizadores en el orden %s", orden)
	}
	for _, tipo := range Deslizadores {
		lo, hi := rangoAjuste(tipo)
		if l := fmt.Sprintf("%s: { valor: [%g, %g] }", tipo, lo, hi); !strings.Contains(receta, l) {
			t.Errorf("receta.ts ya no dice %q", l)
		}
	}
}

// TestLosMesesSalenDelIdioma: el rótulo de un mes lo formatea el catálogo de fechas y
// no un `Format()` de Go con nombres en inglés.
func TestLosMesesSalenDelIdioma(t *testing.T) {
	v := vistaDePrueba(t, nil)
	g := PorMes(fotosDePrueba(3), v.T)
	if len(g) == 0 {
		t.Fatal("no salió ningún mes")
	}
	quiero := i18n.FormatMonthYear("es", time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if g[0].Titulo != quiero {
		t.Errorf("el mes se llama %q y el catálogo de fechas dice %q", g[0].Titulo, quiero)
	}
}

// TestLaCabeceraDelAlbumEncogeElTituloYParteLosMandos: a 1280 todo en una fila y el
// título entero; a ancho de teléfono el título se trunca (puede no caber ni la elipsis)
// y los mandos se parten en su
// hueco sin salirse, como `truncate` y el `ml-auto flex-wrap` de AlbumView.tsx.
func TestLaCabeceraDelAlbumEncogeElTituloYParteLosMandos(t *testing.T) {
	v := vistaDePrueba(t, nil)
	al := api.Album{ID: "a", Title: "Verano en Cadaqués con la familia", Shared: true}
	cabecera := func(ancho float32) (*fyne.Container, *cabeceraAlbum) {
		p := PantallaAlbum(v, AlbumAbierto{Album: al, Fotos: fotosDePrueba(3)})
		// La cabecera es el segundo bloque de la pila de dentro del marco.
		c := p.Objects[1].(*fyne.Container).Objects[1].(*fyne.Container)
		l := c.Layout.(*cabeceraAlbum)
		c.Resize(fyne.NewSize(ancho, ui.AltoA(c, ancho)))
		return c, l
	}
	_, l := cabecera(1092)
	if l.titulo.Text != al.Title {
		t.Errorf("a 1092 el título sale como %q y cabe entero", l.titulo.Text)
	}
	c, l := cabecera(328)
	if l.titulo.Text == al.Title || (l.titulo.Text != "" && !strings.HasSuffix(l.titulo.Text, "…")) {
		t.Errorf("a 328 el título no se trunca (%q) y no cabe", l.titulo.Text)
	}
	if d := l.derecha; d.Position().X+d.Size().Width > 328.01 || ui.AltoA(c, 328) <= tema.TextoLG.Linea {
		t.Errorf("los mandos acaban en %v (el ancho es 328) y la cabecera mide %v: no se partieron",
			d.Position().X+d.Size().Width, ui.AltoA(c, 328))
	}
}

// TestElAreaDeLaPeticionMideLoDeLaWeb: el `<textarea rows={3} class="p-2 text-xs">` de
// Sytue AI mide 66 de alto en Chromium (tres líneas de 16, p-2 y dos bordes), y la
// marca de agua se va en cuanto hay texto.
func TestElAreaDeLaPeticionMideLoDeLaWeb(t *testing.T) {
	a := NuevaAreaPeticion(vistaDePrueba(t, nil))
	if h := a.MinSize().Height; h != 66 {
		t.Errorf("el área mide %v de alto y la web, 66", h)
	}
	if a.marca.Hidden {
		t.Fatal("vacía no enseña la marca de agua")
	}
	a.Editor.SetText("Más cálida")
	if !a.marca.Hidden || a.Texto() != "Más cálida" {
		t.Errorf("con texto la marca sigue (%v) o el texto es %q", !a.marca.Hidden, a.Texto())
	}
}

// TestElMenuDeFondosLlevaSuMuestra: delante de cada fondo, `h-3 w-3 rounded-full
// border` con su relleno y su borde, y en el damero sus cuadros de 3 px recortados al
// círculo: el de arriba en el centro, pintado; el hueco de la esquina, del fondo; y el
// cuadro de la baldosa de abajo, que se sale del círculo, blanco fuera de él.
func TestElMenuDeFondosLlevaSuMuestra(t *testing.T) {
	for _, c := range []struct {
		f    FondoLienzo
		x, y float32
		want color.NRGBA
	}{
		{FondoOscuro, 5.25, 5.25, FondoOscuro.colorFondo()},
		{FondoOscuro, 5.25, 0.75, bordeMuestra[FondoOscuro]},
		{FondoCrema, 5.25, 5.25, FondoCrema.colorFondo()},
		{FondoDamero, 5.5, 2.5, cuadroDamero},
		{FondoDamero, 2.5, 2.5, FondoDamero.colorFondo()},
		{FondoDamero, 5.25, 5.25, FondoDamero.colorFondo()},
		{FondoDamero, 10.5, 11.5, blanco}, // a más de 6 del centro: fuera, aunque caiga en un cuadro
	} {
		img := pintaA(t, 2, LadoGlifoFondo, LadoGlifoFondo, muestraFondo(c.f, LadoGlifoFondo, 6).Objeto())
		if got := px(img, int(c.x*2), int(c.y*2)); !cerca(got, c.want) {
			t.Errorf("muestra %s en (%.2f, %.2f): %v, quería %v", c.f, c.x, c.y, got, c.want)
		}
	}
}

// TestLaMuestraElegidaCreceUnDiezPorCiento: h-6 w-6 son 24 px y scale-110
// extiende el radio a 13,2. A 12,5 del centro cae su borde y el exterior sin elegir.
func TestLaMuestraElegidaCreceUnDiezPorCiento(t *testing.T) {
	v := vistaDePrueba(t, nil)
	for _, c := range []struct {
		elegida bool
		want    color.NRGBA
	}{
		{true, bordeMuestra[FondoOscuro]},
		{false, blanco},
	} {
		m := NuevaMuestraFondo(v, FondoOscuro, c.elegida, nil)
		m.Move(fyne.NewPos(10, 10))
		m.Resize(fyne.NewSize(LadoMuestraFondo, LadoMuestraFondo))
		img := pintaA(t, 2, 42, 42, m)
		// A 12,5 px del centro: dentro del borde de la muestra crecida (radio 13,2, borde
		// de 1,1) y fuera de la de siempre (radio 12). Es el píxel de dispositivo 69
		// entero, lejos del antialias de los dos bordes del anillo del borde.
		centro := 10 + LadoMuestraFondo/2
		if got := px(img, int((centro+12.5)*2), int(centro*2)); !cerca(got, c.want) {
			t.Errorf("elegida=%v: a 12,5 px del centro hay %v y se esperaba %v", c.elegida, got, c.want)
		}
	}
	if EscalaMuestra(false, true) != 1.05 || EscalaMuestra(true, true) != 1.1 {
		t.Error("`hover:scale-105` en las demás y `scale-110` en la elegida, también con el puntero encima")
	}
}

// TestLaCabeceraDelMesSeParteEnUnTelefono: Chromium con las clases reales y rem
// de 16 (2026-10-04). A 360 el contenido de 328 reparte 109,688 / 95,609 / 98,703;
// los tres hijos quedan en dos líneas y el encabezado mide 52; en escritorio, 36.
func TestLaCabeceraDelMesSeParteEnUnTelefono(t *testing.T) {
	v := vistaDePrueba(t, nil)
	g := GrupoMes{Titulo: "septiembre de 2026", Aproximadas: 4, Fotos: fotosDePrueba(2)}
	cab := cabeceraMes(v, g, Galeria{EnSeleccion: true, Seleccion: map[string]bool{}})
	fila, ok := cab.(*fyne.Container).Objects[1].(*fyne.Container)
	if !ok {
		t.Fatal("la cabecera ya no lleva la fila dentro de su fondo")
	}
	f, ok := fila.Layout.(FilaEncoge)
	if !ok {
		t.Fatal("la cabecera del mes no encoge")
	}
	lejos := func(a, b float32) bool { return a-b > 0.75 || b-a > 0.75 }
	for i, want := range []float32{109.6875, 95.609375, 98.703125} {
		if got := f.Anchos(328)[i]; lejos(got, want) {
			t.Errorf("a 328 el hijo %d mide %.2f y en la web %.1f", i, got, want)
		}
	}
	if h := ui.AltoA(cab, 328); lejos(h, 52) {
		t.Errorf("a 328 la cabecera mide %.2f de alto y en la web 52", h)
	}
	if h := ui.AltoA(cab, 1216); lejos(h, 36) {
		t.Errorf("a 1216 la cabecera mide %.2f de alto y en la web 36", h)
	}
	b, _ := fila.Objects[2].(*BotonMes)
	if l := lineasBotonMes(b.Texto, 98.703125); len(l) != 2 {
		t.Errorf("«Seleccionar el mes» a 98,703 sale en %d líneas: %q", len(l), l)
	}
}

// TestElVeloDelRecorteOscureceTambienElPanel: el `shadow-[0_0_0_9999px_rgba(0,0,0,0.55)]`
// del marco no lo recorta nadie en la web y oscurece también la cabecera y el panel, no
// solo el hueco del lienzo. Se mira un punto del panel y otro de la cabecera: con el
// recorte, el papel al 45 %.
func TestElVeloDelRecorteOscureceTambienElPanel(t *testing.T) {
	v := vistaDePrueba(t, nil)
	pinta := func(recortando bool) image.Image {
		l := NuevoLienzoEditor()
		l.Poner(nrgba(bandas(600, 400)), api.RecetaVacia(), FondoCrema)
		l.Recortando = recortando
		l.Marco = &Marco{X: 0.25, Y: 0.25, W: 0.5, H: 0.5}
		return pintaPantalla(t, nil, PantallaEditor(v, Editor{Foto: fotosDePrueba(1)[0], Receta: api.RecetaVacia(), Lienzo: l}))
	}
	claro, oscuro := pinta(false), pinta(true)
	for _, p := range []image.Point{{1250, 780}, {640, 5}} {
		c, o := px(claro, p.X, p.Y), px(oscuro, p.X, p.Y)
		want := color.NRGBA{R: uint8(float32(c.R)*0.45 + 0.5), G: uint8(float32(c.G)*0.45 + 0.5), B: uint8(float32(c.B)*0.45 + 0.5), A: 255}
		if !cerca(o, want) {
			t.Errorf("en %v, con el recorte, hay %v y el velo al 55 %% sobre %v daría %v", p, o, c, want)
		}
	}
}

// buscarDesenfocado encuentra la caja con `backdrop-blur` y su posición en la ventana.
func buscarDesenfocado(x fyne.CanvasObject, base fyne.Position) (*fyne.Container, fyne.Position) {
	c, ok := x.(*fyne.Container)
	if !ok {
		return nil, base
	}
	p := base.Add(c.Position())
	if _, es := c.Layout.(capaDesenfocada); es {
		return c, p
	}
	for _, h := range c.Objects {
		if d, o := buscarDesenfocado(h, p); d != nil {
			return d, o
		}
	}
	return nil, base
}

// TestElRotuloSeParteEnUnTelefono: Chromium con las clases reales de PhotosApp.tsx
// y rem de 16 (2026-10-04): alto móvil 101; tamaño natural 291,594 × 63,875.
//
// Esas medidas eran CON el borde de 1 px del rótulo y el icono de 15. El rótulo ya no
// lleva borde y el icono mide 16: los altos de aquí —99 en el teléfono y 61,875 a su
// ancho natural— son los que mide Chromium ahora (arnés de medidas, 2026-10-04, con otra
// descripción: 99 y 61,88); el ancho natural de ESTA descripción es DERIVADO del de
// entonces (−2 del borde, +1 del icono), no vuelto a medir.
func TestElRotuloSeParteEnUnTelefono(t *testing.T) {
	lejos := func(a, b float32) bool { return a-b > 0.75 || b-a > 0.75 }
	v := vistaDePrueba(t, nil)
	d := Visor{Foto: fotosDePrueba(1)[0], Descripcion: "Una cala con barcas al atardecer."}
	v.Ventana = 360
	r, _ := rotuloDescripcion(v, d)
	if h := ui.AltoA(r, 180); lejos(h, 99) {
		t.Errorf("a 360 el rótulo mide %.2f de alto y en la web 99", h)
	}
	v.Ventana = 1280
	r, natural := rotuloDescripcion(v, d)
	if lejos(natural, 290.59375) {
		t.Errorf("a 1280 el rótulo mide %.2f de ancho y en la web 290,594", natural)
	}
	if h := ui.AltoA(r, natural); lejos(h, 61.875) {
		t.Errorf("a 1280 el rótulo mide %.2f de alto y en la web 61,875", h)
	}
}

// TestElTamanyoRealEsLaMedidaDeLaFotoYSaleDelOriginal: la web pinta el `<img>` a
// `foto.width` px de CSS. Aquí también, por teselas de 512 y con los píxeles del
// ORIGINAL: a ×1, cada píxel de la pantalla es el suyo, también en la segunda tesela y
// en la última esquina, que es donde se verían las costuras.
func TestElTamanyoRealEsLaMedidaDeLaFotoYSaleDelOriginal(t *testing.T) {
	original := image.NewNRGBA(image.Rect(0, 0, 1200, 900))
	for y := range 900 {
		for x := range 1200 {
			original.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 13), B: uint8((x + y) % 2 * 255), A: 255})
		}
	}
	v := vistaDePrueba(t, nil)
	l := NuevoLienzoVisor(v, Visor{Foto: api.Photo{ID: "f", Width: 1200, Height: 900}, TamanyoReal: true, Original: original})
	if m := l.MedidaReal(); m != fyne.NewSize(1200, 900) {
		t.Fatalf("a tamaño real mide %v y la web, el ancho y el alto de la foto", m)
	}
	l.Resize(fyne.NewSize(1200, 900))
	img := pintaA(t, 1, 1200, 900, l)
	for _, p := range []image.Point{{3, 5}, {513, 1}, {700, 600}, {1199, 899}} {
		if got, want := px(img, p.X, p.Y), original.NRGBAAt(p.X, p.Y); !cerca(got, want) {
			t.Errorf("en %v hay %v y en el original %v", p, got, want)
		}
	}
}

// TestLaExplicacionDeSytueAIVaEnCursiva: la explicación de la tarjeta del resultado es
// `italic` en la web, y aquí la Figtree Italic (ui.Cursiva), no la recta.
func TestLaExplicacionDeSytueAIVaEnCursiva(t *testing.T) {
	v := vistaDePrueba(t, nil)
	tarjeta := tarjetaResultadoIA(v, Editor{IAResultado: &api.EdicionIA{Explicacion: "Más luz."}})
	var p *ui.Parrafo
	var buscar func(fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Parrafo:
			if strings.Contains(x.Texto, "Más luz.") {
				p = x
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		}
	}
	buscar(tarjeta)
	if p == nil || p.Peso != ui.Cursiva {
		t.Fatalf("la explicación de Sytue AI no va en cursiva: %+v", p)
	}
}

// TestElAvisoYSueltaParaAnyadirMidenLoQueLaWeb: Chromium con rem de 16, la hoja
// compartida y las clases reales de estadoSubida: contenido 1216, altos 34/36,
// texto de 169,281 centrado en x=523,359 (2026-10-04).
func TestElAvisoYSueltaParaAnyadirMidenLoQueLaWeb(t *testing.T) {
	v := vistaDePrueba(t, nil)
	lejos := func(a, b float32) bool { return a-b > 0.5 || b-a > 0.5 }
	if h := ui.AltoA(estadoSubida(v, "Se han añadido 3 fotos"), 1216); lejos(h, 34) {
		t.Errorf("el aviso mide %.2f de alto y en la web 34", h)
	}
	s := estadoSoltar(v)
	if h := ui.AltoA(s, 1216); lejos(h, 36) {
		t.Errorf("«Suelta para añadir» mide %.2f de alto y en la web 36", h)
	}
	s.Resize(fyne.NewSize(1216, ui.AltoA(s, 1216)))
	var texto *ui.Rotulo
	var x float32
	var buscar func(fyne.CanvasObject, float32)
	buscar = func(o fyne.CanvasObject, base float32) {
		switch e := o.(type) {
		case *ui.Rotulo:
			texto, x = e, base+e.Position().X
		case *fyne.Container:
			for _, h := range e.Objects {
				buscar(h, base+e.Position().X)
			}
		}
	}
	buscar(s, -s.Position().X)
	if texto == nil || texto.Texto != v.Traduce("gallery.dropHere") || texto.Token != "primary-text" {
		t.Fatalf("«Suelta para añadir» no lleva su texto en primary-text: %+v", texto)
	}
	if lejos(x, 523.359375) {
		t.Errorf("el texto empieza a %.2f del borde y en la web a 523,359", x)
	}
}

// TestLaCabeceraDelMesTieneElFondoDeLaWeb: el `bg-*` del `<h2>` del mes en PhotosApp.tsx
// es el que pinta Fyne. El 2026-09-28 era `bg-surface`, una banda blanca sobre el
// fondo de la página, y se cambió a `bg-bg` en los dos lados. VISTO EN ROJO con
// `fondoCabeceraMes = "surface"`.
func TestLaCabeceraDelMesTieneElFondoDeLaWeb(t *testing.T) {
	app := webref.Read(t, "apps/frontend/apps/photos/src/components/PhotosApp.tsx")
	m := regexp.MustCompile(`'bg-([a-z-]+) [^']*sytue-overline sticky`).FindStringSubmatch(app)
	if m == nil {
		t.Fatal("no encuentro el `<h2>` del mes en PhotosApp.tsx")
	}
	v := vistaDePrueba(t, nil)
	cab := cabeceraMes(v, GrupoMes{Titulo: "septiembre de 2026", Fotos: fotosDePrueba(1)}, Galeria{})
	fondo, ok := cab.(*fyne.Container).Objects[0].(*fcanvas.Rectangle)
	if !ok {
		t.Fatal("la cabecera ya no lleva su fondo delante")
	}
	if fondo.FillColor != tema.Color(m[1], v.Variante) {
		t.Errorf("la web pinta la cabecera con `bg-%s` y Fyne con %v", m[1], fondo.FillColor)
	}
}

// TestLasPestanasDelEditorMiden38 conserva py-2.5 + text-xs + border-b-2
// de Editor.tsx. Antes el alto retenía literalmente la línea de 14 px del tema viejo.
func TestLasPestanasDelEditorMiden38(t *testing.T) {
	if AltoPestanaEditor != 38 {
		t.Errorf("la pestaña mide %v; Chromium da 38", AltoPestanaEditor)
	}
}
