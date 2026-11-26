// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"

	// AVIF y TIFF registrados, como en Sytue Office (solo en la prueba: la pantalla no
	// enlaza `local/imagenes`, lo hace la app).
	_ "git.sytue.eu/sytue/one/apps/fyne/local/imagenes"
)

// heicFalsa son unos bytes que empiezan como una HEIC (la caja `ftyp`) y que ningún
// decodificador lee: solo el «sistema» de la prueba.
var heicFalsa = append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), make([]byte, 64)...)

// opaca es una imagen sin transparencia de w×h.
func opaca(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	return img
}

func codificar(t *testing.T, f func(*bytes.Buffer) error) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := f(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Las notas de imagen leen lo mismo que Photos (`local/imagenes`): AVIF y TIFF, que se
// guardan en JPEG (opacas) o PNG (con transparencia) para que se vean en cualquier
// edición, BMP, que se guarda en PNG aunque sea opaco (una captura no debe llevar las
// manchas del JPEG), y HEIC pasada a JPEG por el sistema (FicherosLocales.HEIC); sin con
// qué pasarla, se dice que este equipo no sabe abrirla. Una de la web pequeña sigue tal
// cual.
func TestLasNotasDeImagenLeenAVIFTIFFBMPYHEIC(t *testing.T) {
	ctx := t.Context()
	ahora := time.UnixMilli(1700000000000)
	jpegDelSistema := codificar(t, func(b *bytes.Buffer) error { return jpeg.Encode(b, opaca(64, 48), nil) })
	var pedidas []string
	f := &FicherosLocales{HEIC: func(_ context.Context, nombre string, datos []byte) (string, []byte, error) {
		pedidas = append(pedidas, nombre)
		if !bytes.Equal(datos, heicFalsa) {
			return "", nil, errors.New("el sistema no sabe")
		}
		return strings.TrimSuffix(nombre, filepath.Ext(nombre)) + ".jpg", jpegDelSistema, nil
	}}
	avif, err := os.ReadFile("../../local/imagenes/testdata/foto.avif")
	if err != nil {
		t.Fatal(err)
	}
	conAlfa := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	conAlfa.Set(3, 3, color.NRGBA{R: 200, A: 100})
	casos := []struct {
		nombre       string
		datos        []byte
		tipo, salida string
	}{
		{"foto.avif", avif, "", ".jpg"},
		{"escaneo.tiff", codificar(t, func(b *bytes.Buffer) error { return tiff.Encode(b, opaca(300, 200), nil) }), "image/jpeg", "escaneo.jpg"},
		{"recorte.tif", codificar(t, func(b *bytes.Buffer) error { return tiff.Encode(b, conAlfa, nil) }), "image/png", "recorte.png"},
		{"IMG_0001.HEIC", heicFalsa, "image/jpeg", "IMG_0001.jpg"},
		{"Captura.BMP", codificar(t, func(b *bytes.Buffer) error { return bmp.Encode(b, opaca(120, 80)) }), "image/png", "Captura.png"},
		{"plano.png", codificar(t, func(b *bytes.Buffer) error { return png.Encode(b, conAlfa) }), "image/png", "plano.png"},
	}
	dir := t.TempDir()
	for _, c := range casos {
		ruta := filepath.Join(dir, c.nombre)
		if err := os.WriteFile(ruta, c.datos, 0o600); err != nil {
			t.Fatal(err)
		}
		if !f.esImagenLeible(ruta) {
			t.Errorf("%s: soltarlo no lo toma por imagen", c.nombre)
		}
		a, err := f.adjuntoDeFichero(ctx, ruta, ahora)
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		b, _ := a.contenido()
		_, formato, err := miniatura.Cabecera(b)
		if err != nil || !slices.Contains(formatosWeb, formato) || "image/"+formato != a.Type {
			t.Errorf("%s: se guarda como %s (%s, %v): no la leería cualquier edición", c.nombre, a.Type, formato, err)
		}
		if c.tipo != "" && a.Type != c.tipo || !strings.HasSuffix(a.Name, c.salida) {
			t.Errorf("%s: %s %s, quiero %s %s", c.nombre, a.Type, a.Name, c.tipo, c.salida)
		}
	}
	if c := casos[len(casos)-1]; !bytes.Equal(mustBytes(t, f, filepath.Join(dir, c.nombre)), c.datos) {
		t.Error("un PNG pequeño ya no va tal cual")
	}
	if !slices.Equal(pedidas, []string{"IMG_0001.HEIC"}) {
		t.Errorf("al sistema solo va la HEIC: %v", pedidas)
	}

	// Sin con qué pasarla a JPEG: ni se toma por imagen al soltarla, ni se lee.
	sin := &FicherosLocales{NoEsImagen: "no se lee"}
	if sin.esImagenLeible("IMG_0001.HEIC") || !sin.esImagenLeible("escaneo.TIF") {
		t.Error("sin HEIC, una .heic no es una imagen que se sepa leer")
	}
	_, err = sin.adjuntoDeFichero(ctx, filepath.Join(dir, "IMG_0001.HEIC"), ahora)
	p := NuevaPantalla(trInterno(t), "es", nil, nil, nil)
	p.FicherosLocales = sin
	if !errors.Is(err, errSinHEIC) || p.textoFalloImagen(err, false) != TextosImagenURL["sinHEIC"][0] {
		t.Errorf("una HEIC sin conversor: %v → %q", err, p.textoFalloImagen(err, false))
	}
	if got := p.textoFalloImagen(ErrNoEsImagen, false); got != "no se lee" {
		t.Errorf("lo que no es una imagen: %q", got)
	}
}

func mustBytes(t *testing.T, f *FicherosLocales, ruta string) []byte {
	t.Helper()
	a, err := f.adjuntoDeFichero(t.Context(), ruta, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := a.contenido()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// imagenesFalsas es FicherosLocales.BajarImagen de mentira: lo que devuelve cada
// dirección y cuántas veces se pidió.
type imagenesFalsas struct {
	mu      sync.Mutex
	por     map[string][]byte
	fallos  map[string]error
	pedidas []string
	// lenta, si no es nil, retiene la bajada de esa dirección hasta que se cierre.
	lenta  chan struct{}
	cuelga string
}

func (x *imagenesFalsas) bajar(_ context.Context, d string) ([]byte, error) {
	if x.lenta != nil && d == x.cuelga {
		<-x.lenta
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.pedidas = append(x.pedidas, d)
	if err, ok := x.fallos[d]; ok {
		return nil, err
	}
	if b, ok := x.por[d]; ok {
		return b, nil
	}
	return nil, errors.New("404")
}

func (x *imagenesFalsas) cuantas() int {
	x.mu.Lock()
	defer x.mu.Unlock()
	return len(x.pedidas)
}

// La nota de imagen por URL en Sytue Office: pintarla NO baja nada —la tarjeta dice de
// dónde es y ofrece «Cargar la imagen»—; pulsarlo la baja una vez y la guarda como adjunto
// de la nota con su dirección por origen, junto a lo que ya llevara, y desde entonces la
// tarjeta la pinta de dentro, sin red. Una nota de solo lectura no ofrece cargarla, y en
// la edición conectada (sin FicherosLocales) la tarjeta la pide como siempre.
func TestLaNotaDeImagenPorURLSeCargaAPeticion(t *testing.T) {
	const dir = "https://imagenes.example.com/planos/plano.png"
	png40 := codificar(t, func(b *bytes.Buffer) error { return png.Encode(b, opaca(40, 20)) })
	falsas := &imagenesFalsas{por: map[string][]byte{dir: png40}}
	viejo := nuevoAdjunto("acta.pdf", "application/pdf", []byte("%PDF-1.7"), time.Now())
	data, _ := conAdjuntos(json.RawMessage(`{"url":"`+dir+`"}`), []Adjunto{viejo})
	nota := api.Note{ID: "u1", NotebookID: "c1", Kind: "image", Title: "Plano", Data: data}
	p, falso := pantallaConServicio(t, map[string]string{"PUT /notes/u1": `{"id":"u1","notebook_id":"c1","kind":"image"}`}, nota)
	p.FicherosLocales = &FicherosLocales{BajarImagen: falsas.bajar}
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()

	n, _ := p.nota("u1")
	cuerpo := p.lienzo.tarjeta(n).cuerpo(tema.Claro)
	textos := textosInternos(cuerpo)
	if !slices.Contains(textos, "Cargar la imagen") || !strings.Contains(strings.Join(textos, " "), "imagenes.example.com") {
		t.Fatalf("la tarjeta sin la imagen: %v", textos)
	}
	if falsas.cuantas() != 0 || len(falso.rutas()) != 0 {
		t.Fatalf("pintarla pidió %v / %v", falsas.pedidas, falso.rutas())
	}
	if p.barra.AlImagenURL == nil || p.barra.RotuloImagenURL != "Añadir una imagen de una dirección web…" {
		t.Errorf("el «…» del cuaderno no ofrece la imagen de una dirección: %q", p.barra.RotuloImagenURL)
	}

	var boton *ui.Boton
	recorrerInterno(cuerpo, func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == "Cargar la imagen" {
			boton = b
		}
	})
	if boton == nil {
		t.Fatal("sin el botón")
	}
	test.Tap(boton)
	esperarVeces(t, falso, "PUT /notes/u1", 1)
	put := falso.ultima(t, "PUT /notes/u1")
	raw, _ := json.Marshal(put["data"])
	var d struct {
		URL         string    `json:"url"`
		Attachments []Adjunto `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d.URL != dir || len(d.Attachments) != 2 || d.Attachments[0].Name != "acta.pdf" || d.Attachments[1].Origen != dir ||
		d.Attachments[1].Type != "image/png" || d.Attachments[1].Name != "plano.png" {
		t.Fatalf("la nota con la imagen dentro: %s", raw)
	}
	if b, _ := d.Attachments[1].contenido(); !bytes.Equal(b, png40) {
		t.Error("la imagen bajada no va tal cual")
	}
	if falsas.cuantas() != 1 {
		t.Errorf("se bajó %d veces", falsas.cuantas())
	}

	// Con la imagen dentro, la tarjeta la pinta sin pedir nada.
	cargada := DeNota(api.Note{ID: "u1", NotebookID: "c1", Kind: "image", Title: "Plano", Data: json.RawMessage(raw)})
	if a, ok := imagenIncrustada(cargada); !ok || a.Origen != dir {
		t.Fatalf("la de su dirección es la de dentro: %+v", a)
	}
	p.notas = []Nota{cargada}
	p.Repintar()
	p.cargarImagenAdjunta(d.Attachments[1])
	if got := altoCuerpo(p.lienzo.tarjeta(cargada).cuerpo(tema.Claro), 300.7); got < 100 {
		t.Errorf("la imagen cargada no se pinta: %.1f", got)
	}
	// Otra dirección en la misma nota no se toma por esa imagen.
	otra := cargada
	otra.URL = "https://otra.example.com/x.png"
	if _, ok := imagenIncrustada(otra); ok {
		t.Error("la imagen de una dirección vale para otra")
	}

	// De solo lectura: se dice, sin botón.
	p.notas = DeNotas([]api.Note{nota})
	p.lienzo.SoloLectura = true
	n, _ = p.nota("u1")
	if textos := textosInternos(p.lienzo.tarjeta(n).cuerpo(tema.Claro)); slices.Contains(textos, "Cargar la imagen") {
		t.Errorf("de solo lectura no se carga: %v", textos)
	}

	// La edición conectada la pide como siempre.
	conectada, _ := pantallaConServicio(t, nil, nota)
	pedida := ""
	conectada.Objeto()
	conectada.Repintar()
	conectada.lienzo.Imagen = func(u string) imagenRemota { pedida = u; return imagenRemota{} }
	n, _ = conectada.nota("u1")
	conectada.lienzo.tarjeta(n).cuerpo(tema.Claro)
	if pedida != dir || conectada.barra.AlImagenURL != nil {
		t.Errorf("sin FicherosLocales la tarjeta pide %q como siempre, sin «…» nuevo", pedida)
	}
}

// «Añadir una imagen de una dirección web…»: la dirección que se escribe se baja al
// pulsar «Cargar la imagen» y la nota de imagen nace con ella dentro y su `data.url`; si
// no se puede, el diálogo dice por qué (solo https, «Sin conexión», no es una imagen) y no
// se crea nada; una dirección que no es https ni se pide; y si se cierra el diálogo
// mientras baja, al llegar tampoco se crea.
func TestLaImagenDeUnaDireccionNaceDentroDeLaNota(t *testing.T) {
	const buena = "https://example.com/fotos/Mudanza%20cocina.webp?v=2"
	const lenta = "https://example.com/fotos/lenta.png"
	// Un TIFF (que no es de la web) con la extensión de otro: manda lo que es.
	escaneo := codificar(t, func(b *bytes.Buffer) error { return tiff.Encode(b, opaca(300, 200), nil) })
	falsas := &imagenesFalsas{
		por: map[string][]byte{buena: escaneo, lenta: codificar(t, func(b *bytes.Buffer) error { return png.Encode(b, opaca(20, 20)) })},
		fallos: map[string]error{
			"https://example.com/sin-red.png": ErrImagenSinConexion,
			"https://192.168.1.9/router.png":  ErrImagenNoPermitida,
			"https://example.com/pagina":      ErrNoEsImagen,
		},
	}
	p, falso := pantallaConServicio(t, map[string]string{"POST /notebooks/c1/notes": `{"id":"n9","notebook_id":"c1","kind":"image"}`})
	p.FicherosLocales = &FicherosLocales{BajarImagen: falsas.bajar, NoEsImagen: "no se lee"}
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()

	escribirYCargar := func(direccion string) []string {
		t.Helper()
		p.dialogoImagenURL()
		capa := w.Canvas().Overlays().Top()
		if capa == nil {
			t.Fatal("el diálogo no se abre")
		}
		var campo *ui.Entrada
		var cargar *ui.Boton
		recorrerInterno(capa, func(o fyne.CanvasObject) {
			if e, ok := o.(*ui.Entrada); ok {
				campo = e
			}
			if b, ok := o.(*ui.Boton); ok && b.Texto == "Cargar la imagen" {
				cargar = b
			}
		})
		campo.PonerTexto(direccion)
		test.Tap(cargar)
		esperar(t, func() bool {
			fyne.DoAndWait(func() {})
			return !cargar.Cargando
		})
		fyne.DoAndWait(func() {})
		capa = w.Canvas().Overlays().Top()
		if capa == nil {
			return nil
		}
		var vistos []string
		recorrerInterno(capa, func(o fyne.CanvasObject) {
			if x, ok := o.(*canvas.Text); ok && x.Text != "" {
				vistos = append(vistos, x.Text)
			}
		})
		for _, o := range w.Canvas().Overlays().List() {
			w.Canvas().Overlays().Remove(o)
		}
		return vistos
	}
	for direccion, quiero := range map[string]string{
		"http://example.com/a.png":        "Solo se cargan imágenes",
		"https://192.168.1.9/router.png":  "Solo se cargan imágenes",
		"https://example.com/sin-red.png": "«Sin conexión»",
		"https://example.com/pagina":      "no se lee",
		"https://example.com/caida.png":   "No se ha podido bajar la imagen",
	} {
		if vistos := strings.Join(escribirYCargar(direccion), " "); !strings.Contains(vistos, quiero) {
			t.Errorf("%s: el diálogo dice %q, quiero %q", direccion, vistos, quiero)
		}
	}
	if slices.Contains(falsas.pedidas, "http://example.com/a.png") {
		t.Error("una dirección http se pidió")
	}
	if rs := falso.rutas(); len(rs) != 0 {
		t.Fatalf("un fallo creó algo: %v", rs)
	}

	// Cerrar con el aspa mientras baja (lo mismo que Escape, el velo o «Cancelar»): al
	// llegar no se crea nada.
	falsas.lenta, falsas.cuelga = make(chan struct{}), lenta
	p.dialogoImagenURL()
	var campo *ui.Entrada
	var cargar, aspa *ui.Boton
	recorrerInterno(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.Entrada); ok {
			campo = e
		}
		if b, ok := o.(*ui.Boton); ok && b.Texto == "Cargar la imagen" {
			cargar = b
		}
		if b, ok := o.(*ui.Boton); ok && b.SoloIcono && b.Rotulo == p.tr.T("common.close", nil) {
			aspa = b
		}
	})
	campo.PonerTexto(lenta)
	test.Tap(cargar)
	test.Tap(aspa)
	if w.Canvas().Overlays().Top() != nil {
		t.Fatal("el aspa no cierra el diálogo mientras baja")
	}
	close(falsas.lenta)
	esperar(t, func() bool {
		fyne.DoAndWait(func() {})
		return !cargar.Cargando
	})
	// Crear iría enseguida, en su rutina: se le da tiempo de sobra.
	time.Sleep(300 * time.Millisecond)
	fyne.DoAndWait(func() {})
	if rs := falso.rutas(); len(rs) != 0 {
		t.Fatalf("cerrado mientras bajaba, se creó igual: %v", rs)
	}
	falsas.lenta = nil

	if vistos := escribirYCargar("  " + buena + " "); vistos != nil {
		t.Fatalf("con la imagen, el diálogo se cierra: %v", vistos)
	}
	esperarVeces(t, falso, "POST /notebooks/c1/notes", 1)
	post := falso.ultima(t, "POST /notebooks/c1/notes")
	raw, _ := json.Marshal(post["data"])
	var d struct {
		URL         string    `json:"url"`
		Attachments []Adjunto `json:"attachments"`
	}
	_ = json.Unmarshal(raw, &d)
	if post["kind"] != "image" || post["title"] != "Mudanza cocina" || d.URL != buena ||
		len(d.Attachments) != 1 || d.Attachments[0].Origen != buena || d.Attachments[0].Type != "image/jpeg" {
		t.Fatalf("la nota nueva: %v %s", post, raw)
	}
	if a, ok := imagenIncrustada(DeNota(api.Note{Kind: "image", Data: json.RawMessage(raw)})); !ok || a.Origen != buena {
		t.Errorf("la nota nueva no se ve sin red: %+v", a)
	}
}

// «Volver a cargar la imagen», en el «…» de una nota de imagen que ya lleva dentro la de su
// dirección: la baja otra vez por el mismo camino y SOLO si llega bien sustituye a la de
// dentro, que deja la nota (los demás adjuntos siguen) y la memoria. Si falla, se dice por
// qué y la nota no se toca; si llega la misma, tampoco, y se dice. No se ofrece en una
// nota sin la imagen dentro (para eso está «Cargar la imagen»), de solo lectura ni en la
// edición conectada.
func TestVolverACargarLaImagenSoloLaCambiaSiLlegaBien(t *testing.T) {
	const dir = "https://imagenes.example.com/planos/plano.png"
	vieja := codificar(t, func(b *bytes.Buffer) error { return png.Encode(b, opaca(40, 20)) })
	nueva := codificar(t, func(b *bytes.Buffer) error { return png.Encode(b, opaca(60, 30)) })
	acta := nuevoAdjunto("acta.pdf", "application/pdf", []byte("%PDF-1.7"), time.Now())
	dentro := nuevoAdjunto("plano.png", "image/png", vieja, time.Now())
	dentro.Origen = dir
	data, _ := conAdjuntos(json.RawMessage(`{"url":"`+dir+`"}`), []Adjunto{acta, dentro})
	nota := api.Note{ID: "u1", NotebookID: "c1", Kind: "image", Title: "Plano", Data: data}
	sinBajar := api.Note{ID: "u2", NotebookID: "c1", Kind: "image", Title: "Otro", Data: json.RawMessage(`{"url":"` + dir + `"}`)}
	falsas := &imagenesFalsas{por: map[string][]byte{}, fallos: map[string]error{dir: ErrImagenSinConexion}}
	p, falso := pantallaConServicio(t, map[string]string{"PUT /notes/u1": `{"id":"u1","notebook_id":"c1","kind":"image"}`}, nota, sinBajar)
	p.FicherosLocales = &FicherosLocales{BajarImagen: falsas.bajar}
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()

	rotulo := TextosImagenURL["recargar"][0]
	recargar := func(id string) *ui.ElementoMenu {
		n, _ := p.nota(id)
		for _, e := range p.menuNota(n, false).Elementos {
			if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == rotulo {
				return it
			}
		}
		return nil
	}
	if recargar("u2") != nil {
		t.Error("sin la imagen dentro se ofrece volver a cargarla")
	}
	e := recargar("u1")
	if e == nil {
		t.Fatal("el «…» de la nota con la imagen dentro no ofrece volver a cargarla")
	}
	// Lo que se suelta de memoria al cambiarla.
	p.cargarImagenAdjunta(dentro)
	avisos := func() []string {
		var out []string
		fyne.DoAndWait(func() {
			for _, a := range p.avisos.Avisos() {
				out = append(out, a.Titulo+"|"+a.Descripcion)
			}
		})
		return out
	}
	pulsar := func(e *ui.ElementoMenu, avisosAntes int) []string {
		t.Helper()
		fyne.DoAndWait(e.AlPulsar)
		var vistos []string
		esperar(t, func() bool {
			bajando := true
			fyne.DoAndWait(func() { bajando = p.bajandoImagen["u1"] })
			vistos = avisos()
			return !bajando && len(vistos) > avisosAntes
		})
		return vistos[avisosAntes:]
	}

	// Falla (aquí, «Sin conexión»): se dice, y la nota no se toca.
	if got := pulsar(e, 0); len(got) != 1 || got[0] != rotulo+"|"+TextosImagenURL["sinConexion"][0] {
		t.Errorf("al fallar se dice %v", got)
	}
	// Llega la misma: no se escribe nada, y se dice.
	delete(falsas.fallos, dir)
	falsas.por[dir] = vieja
	if got := pulsar(recargar("u1"), 1); len(got) != 1 || got[0] != TextosImagenURL["igual"][0]+"|" {
		t.Errorf("con la misma imagen se dice %v", got)
	}
	if rs := falso.rutas(); len(rs) != 0 {
		t.Fatalf("sin una imagen nueva se guardó: %v", rs)
	}

	// Llega otra: sustituye a la de dentro, que se suelta; el acta sigue.
	falsas.por[dir] = nueva
	fyne.DoAndWait(recargar("u1").AlPulsar)
	esperarVeces(t, falso, "PUT /notes/u1", 1)
	raw, _ := json.Marshal(falso.ultima(t, "PUT /notes/u1")["data"])
	var d struct {
		URL         string    `json:"url"`
		Attachments []Adjunto `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d.URL != dir || len(d.Attachments) != 2 || d.Attachments[0].ID != acta.ID || d.Attachments[1].Origen != dir || d.Attachments[1].ID == dentro.ID {
		t.Fatalf("la nota tras volver a cargar: %s", raw)
	}
	if b, _ := d.Attachments[1].contenido(); !bytes.Equal(b, nueva) {
		t.Error("la imagen de dentro no es la nueva")
	}
	esperar(t, func() bool {
		suelta := false
		fyne.DoAndWait(func() { _, sigue := p.imagenes["adjunto:"+dentro.ID]; suelta = !sigue && !p.bajandoImagen["u1"] })
		return suelta
	})
	if falsas.cuantas() != 3 {
		t.Errorf("se bajó %d veces", falsas.cuantas())
	}

	// De solo lectura, y en la edición conectada, no se ofrece.
	p.lienzo.SoloLectura = true
	if recargar("u1") != nil {
		t.Error("de solo lectura se ofrece volver a cargarla")
	}
	conectada, _ := pantallaConServicio(t, nil, nota)
	conectada.Objeto()
	conectada.Repintar()
	n, _ := conectada.nota("u1")
	for _, e := range conectada.menuNota(n, false).Elementos {
		if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == rotulo {
			t.Error("la edición conectada ofrece volver a cargar la imagen")
		}
	}
}

// TestLosTextosDeLaImagenPorURLCuadran: los textos propios de la nota de imagen por URL
// están en los 26 idiomas (C-12), sin claves de más ni de menos y con su %s.
func TestLosTextosDeLaImagenPorURLCuadran(t *testing.T) {
	for _, f := range (i18n.Propios{Base: TextosImagenURL, Traducciones: traduccionesImagenURL}).Faltan() {
		t.Error(f)
	}
	if got := textoImagenURL("de", "cargar"); got != "Bild laden" {
		t.Errorf("alemán: %q", got)
	}
}
