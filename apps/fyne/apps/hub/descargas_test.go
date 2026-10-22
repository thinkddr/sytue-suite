// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const mb = 1024 * 1024

// catalogoDeEjemplo es el del arnés con el que se midió la web: dos para macOS y tres
// para lo demás, todos de Sytue Hub salvo uno.
// sumaDMG es la suma de lo que baja el doble («DMG»): la que trae el catálogo.
const sumaDMG = "5d6b5c297a9d9a08b374f9d81ea803eec55c6ca189fc842aed44168af14e7687"

var catalogoDeEjemplo = api.HubDescargas{Version: "v3.6.0", Instaladores: []api.Instalador{
	{Fichero: "sytue-hub-windows-amd64-installer.exe", Plataforma: "windows", Bytes: 52 * mb, SHA256: sumaDMG},
	{Fichero: "sytue-mail-macos-universal.dmg", Plataforma: "macos", Bytes: 7654605, SHA256: sumaDMG},
	{Fichero: "sytue-hub-linux-amd64.tar.xz", Plataforma: "linux", Bytes: 38 * mb, SHA256: sumaDMG},
	{Fichero: "sytue-hub-macos-universal.dmg", Plataforma: "macos", Bytes: 84 * mb, SHA256: sumaDMG},
	{Fichero: "sytue-hub-android.apk", Plataforma: "android", Bytes: 41 * mb, SHA256: sumaDMG},
}}

// Remedido 2026-10-04 en Chromium remoto, CSS1Compat, rem16 y Figtree cargada.
// TestDescargasMideLoQueMideLaWeb ata la pantalla al marcado de `descargas.astro` con
// la hoja del portal, medido con Chromium en el frontal (`--font-render-hinting=none`)
// a 1280 y a 360 de ventana, sin la topbar: la `section` empieza en y = 0.
//
// Visto en ROJO a mano (en el frontal): con el `px-4` de escritorio cambiado por el
// `px-6` de las pantallas de ajustes (`CajaAjustes`), todo sale 7 px a la derecha y
// 14 más estrecho.
func TestDescargasMideLoQueMideLaWeb(t *testing.T) {
	type caja struct{ x, y, w, h float32 }
	casos := []struct {
		ventana                        float32
		titulo, entrada, version       caja
		destacadas, tarjeta0, tarjeta1 caja
		enlace0, resto, lista          caja
		enlaceFila0, nombreFila1       caja
	}{
		{
			ventana: 1280,
			titulo:  caja{144, 32, 992, 42}, entrada: caja{144, 78, 992, 22}, version: caja{144, 104, 992, 16},
			destacadas: caja{144, 144, 992, 30},
			tarjeta0:   caja{144, 186, 322.65625, 106}, tarjeta1: caja{478.65625, 186, 322.671875, 106},
			enlace0: caja{161, 253, 72.390625, 22},
			resto:   caja{144, 316, 992, 30}, lista: caja{144, 358, 992, 208},
			enlaceFila0: caja{1046.609375, 382, 72.390625, 22}, nombreFila1: caja{161, 440, 76.640625, 22},
		},
		{
			ventana: 360,
			titulo:  caja{16, 32, 328, 84}, entrada: caja{16, 120, 328, 88}, version: caja{16, 212, 328, 16},
			destacadas: caja{16, 252, 328, 30},
			tarjeta0:   caja{16, 294, 328, 106}, tarjeta1: caja{16, 412, 328, 106},
			enlace0: caja{33, 361, 72.390625, 22},
			resto:   caja{16, 542, 328, 30}, lista: caja{16, 584, 328, 208},
			enlaceFila0: caja{254.609375, 608, 72.390625, 22}, nombreFila1: caja{33, 666, 76.640625, 22},
		},
	}
	// Los anchos de rótulo salen a ≤0,17 del navegador (ADR 0048 §texto); la geometría
	// apilada, a la cuantización de 1/64 que el navegador acumula.
	cerca := func(a, b, tol float32) bool { return a-b <= tol && b-a <= tol }
	for _, c := range casos {
		d := NuevasDescargas(castellano(t))
		d.Estado, d.Catalogo, d.Suya = Listo, catalogoDeEjemplo, "macos"
		r := test.WidgetRenderer(d).(*descargasRenderer)
		ven := test.NewWindow(d)
		ven.SetPadded(false)
		d.Resize(fyne.NewSize(c.ventana, r.alto(c.ventana)))
		ven.Resize(fyne.NewSize(c.ventana, r.alto(c.ventana)))
		guardar(t, ven.Canvas().Capture(), "descargas-"+map[float32]string{1280: "ancho", 360: "estrecho"}[c.ventana])

		mira := func(nombre string, o fyne.CanvasObject, quiero caja) {
			const tol = 0.1
			p, s := o.Position(), o.Size()
			if !cerca(p.X, quiero.x, tol) || !cerca(p.Y, quiero.y, tol) || !cerca(s.Width, quiero.w, 0.5) || !cerca(s.Height, quiero.h, tol) {
				t.Errorf("a %v, %s está en (%v, %v) y mide %v × %v; en la web (%v, %v) y %v × %v",
					c.ventana, nombre, p.X, p.Y, s.Width, s.Height, quiero.x, quiero.y, quiero.w, quiero.h)
			}
		}
		mira("el título", r.titulo, c.titulo)
		mira("la entrada", r.entrada, c.entrada)
		mira("la versión", r.version, c.version)
		mira("«Para tu macOS»", r.tituloDestacados, c.destacadas)
		mira("la primera tarjeta", r.tarjetas[0].caja.Objetos()[0], c.tarjeta0)
		mira("la segunda tarjeta", r.tarjetas[1].caja.Objetos()[0], c.tarjeta1)
		mira("«Descargar» de la primera tarjeta", r.tarjetas[0].enlace, c.enlace0)
		mira("«Otras plataformas»", r.tituloResto, c.resto)
		mira("la lista", r.lista.Objetos()[0], c.lista)
		mira("«Descargar» de la primera fila", r.filas[0].enlace, c.enlaceFila0)
		// El nombre de la fila es un texto: su CAJA DE LÍNEA empieza donde lo pone
		// ColocarTexto menos el centrado del glifo, así que se compara la x y el ancho.
		if p, s := r.filas[1].nombre.Position(), r.filas[1].nombre.Size(); !cerca(p.X, c.nombreFila1.x, 0.1) || !cerca(s.Width, c.nombreFila1.w, 0.5) {
			t.Errorf("a %v, el nombre de la segunda fila está en x = %v y mide %v; en la web %v y %v",
				c.ventana, p.X, s.Width, c.nombreFila1.x, c.nombreFila1.w)
		}
		if quiero := map[float32]float32{1280: 598, 360: 824}[c.ventana]; !cerca(r.alto(c.ventana), quiero, 0.1) {
			t.Errorf("a %v la página mide %v de alto; en la web %v", c.ventana, r.alto(c.ventana), quiero)
		}
		ven.Close()
	}
}

// TestDescargasVaciaDiceElMotivo: sin nada publicado, la caja gris con el motivo que
// dio el Hub, medida como la web a 360.
func TestDescargasVaciaDiceElMotivo(t *testing.T) {
	d := NuevasDescargas(castellano(t))
	d.Estado, d.Catalogo = Listo, api.HubDescargas{Version: "v3.6.0", Motivo: api.DescargasSinInstaladores}
	r := test.WidgetRenderer(d).(*descargasRenderer)
	d.Resize(fyne.NewSize(360, r.alto(360)))
	if r.vacio == nil || !strings.HasPrefix(r.vacioTexto.Texto, "Ninguna versión") {
		t.Fatalf("sin instaladores no sale la caja con «sin release»: %+v", r.vacioTexto)
	}
	if p, s := r.vacio.Objetos()[0].Position(), r.vacio.Objetos()[0].Size(); p.Y != 252 || s.Height != 152 {
		t.Errorf("la caja está en y = %v y mide %v; en el DOM Chromium 252 y 152", p.Y, s.Height)
	}
	d.Catalogo.Motivo = api.DescargasSinConfigurar
	d.Refresh()
	if !strings.HasPrefix(r.vacioTexto.Texto, "Este despliegue") {
		t.Errorf("sin credencial dice %q", r.vacioTexto.Texto)
	}
}

// TestLoQueDiceCadaInstalador: el nombre sale del catálogo de marcas —también de los
// nombres viejos—, el tamaño va como `toFixed` y el reparto pone lo de esta máquina
// primero, cada mitad por producto y plataforma. Es `productoDe`/`tamano` de la web.
func TestLoQueDiceCadaInstalador(t *testing.T) {
	for nombre, quiero := range map[string]string{
		"sytue-calcus-linux-amd64.tar.xz":         "Sytue Calcus",
		"sytue-certipdf-macos-universal.dmg":      "Sytue CertiPDF",
		"sytue-notes-windows-amd64-installer.exe": "Sytue Notepads",
		"sytue-hub-android.apk":                   "Sytue Hub",
		"sytue-nuevo-android.apk":                 "Sytue Nuevo",
		"SytueOne-macos-universal.dmg":            "Sytue One",
		"sytue-one-linux-amd64.tar.xz":            "Sytue One",
		"sytue-drive-windows-amd64-installer.exe": "Sytue Drive",
		"cosa-rara-android.apk":                   "Cosa Rara",
	} {
		if got := ProductoDeFichero(nombre); got != quiero {
			t.Errorf("ProductoDeFichero(%q) = %q; quiero %q", nombre, got, quiero)
		}
	}
	for bytes, quiero := range map[int64]string{0: "macOS", 7654605: "macOS · 7,3 MB", 10 * mb: "macOS · 10 MB", 84*mb + mb/2: "macOS · 85 MB"} {
		if got := MetaInstalador("es", api.Instalador{Plataforma: "macos", Bytes: bytes}); got != quiero {
			t.Errorf("MetaInstalador(%d) = %q; quiero %q", bytes, got, quiero)
		}
	}
	if PlataformaPropia("darwin") != "macos" || PlataformaPropia("ios") != "" || PlataformaPropia("android") != "android" {
		t.Error("la plataforma propia no es la de la web")
	}
	destacados, resto := Repartir(catalogoDeEjemplo.Instaladores, "macos", "")
	orden := make([]string, 0, len(destacados)+len(resto))
	for _, i := range append(destacados, resto...) {
		orden = append(orden, i.Fichero)
	}
	if strings.Join(orden, " ") != "sytue-hub-macos-universal.dmg sytue-mail-macos-universal.dmg "+
		"sytue-hub-android.apk sytue-hub-linux-amd64.tar.xz sytue-hub-windows-amd64-installer.exe" {
		t.Errorf("orden: %v", orden)
	}
}

// descargadorDoble contesta la lista y sirve cada instalador, o falla.
type descargadorDoble struct {
	mu     sync.Mutex
	fallo  error
	pedido string
}

func (d *descargadorDoble) Descargas(context.Context) (api.HubDescargas, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return catalogoDeEjemplo, d.fallo
}

func (d *descargadorDoble) Descargar(_ context.Context, i api.Instalador) (io.ReadCloser, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pedido = i.Fichero
	if d.fallo != nil {
		return nil, "", d.fallo
	}
	return io.NopCloser(strings.NewReader("DMG")), i.Fichero, nil
}

// TestLaDescargaAcabaEnLaCarpeta: entrar pide la lista, pulsar baja el instalador a la
// carpeta de descargas con su nombre y, mientras baja, su enlace no se deja pulsar otra
// vez. Si el Hub no contesta, la pantalla lo dice en vez de quedarse cargando.
//
// Visto en ROJO a mano (en el frontal): sin el `delete` de `Bajando` al acabar, el
// enlace se queda inerte para siempre y el test lo dice.
func TestLaDescargaAcabaEnLaCarpeta(t *testing.T) {
	dir := t.TempDir()
	doble := &descargadorDoble{}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)).ConDescargas(doble)
	v.carpeta = func() (string, error) { return dir, nil }
	v.Ir(ADescargas)
	esperar := func(que string, hecho func() bool) {
		t.Helper()
		for range 200 {
			fyne.DoAndWait(func() {}) // lo que la carga manda pintar corre en la rutina del test
			v.mu.Lock()
			ok := hecho()
			v.mu.Unlock()
			if ok {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal(que)
	}
	esperar("la lista no llega", func() bool { return v.bajadas.Estado == Listo })

	v.descargarInstalador(catalogoDeEjemplo.Instaladores[3])
	esperar("la descarga no acaba", func() bool { return !v.bajadas.Bajando["sytue-hub-macos-universal.dmg"] })
	datos, err := os.ReadFile(filepath.Join(dir, "sytue-hub-macos-universal.dmg"))
	if err != nil || string(datos) != "DMG" {
		t.Fatalf("el instalador no está en la carpeta: %q, %v", datos, err)
	}

	doble.mu.Lock()
	doble.fallo = errors.New("el Hub no contesta")
	doble.mu.Unlock()
	v.Ir(ADescargas)
	esperar("un Hub caído deja la pantalla cargando", func() bool { return v.bajadas.Estado == Fallo })
	r := test.WidgetRenderer(v.bajadas).(*descargasRenderer)
	if r.aviso == nil {
		t.Error("con el Hub caído no sale el aviso de fallo")
	}
}

// portapapelesQueFalla se traga lo que se le da.
type portapapelesQueFalla struct{}

func (portapapelesQueFalla) Content() string   { return "" }
func (portapapelesQueFalla) SetContent(string) {}

// TestCopiarDiceSiNoSeHaPodido: la fila de copiar de «Conecta tus dispositivos» avisa
// en verde si el valor ha llegado al portapapeles y en rojo —«No se ha podido copiar»—
// si no, que es el `catch` de la web.
func TestCopiarDiceSiNoSeHaPodido(t *testing.T) {
	v := NuevaVista(castellano(t), nil, time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	v.portapapeles = test.NewClipboard
	v.copiar("imap.sytue.eu", "devices.copied.imap")
	v.portapapeles = func() fyne.Clipboard { return portapapelesQueFalla{} }
	v.copiar("imap.sytue.eu", "devices.copied.imap")
	avisos := v.pilaAvisos.Avisos()
	if len(avisos) != 2 {
		t.Fatalf("hay %d avisos; quiero 2", len(avisos))
	}
	if avisos[0].Tono != ui.AvisoExito || avisos[1].Tono != ui.AvisoPeligro ||
		avisos[1].Titulo != "No se ha podido copiar" {
		t.Errorf("avisos: %q (%v) y %q (%v)", avisos[0].Titulo, avisos[0].Tono, avisos[1].Titulo, avisos[1].Tono)
	}
}

// TestSoloSeDestacaLoQueArrancaAqui es el guardián de «para tu sistema» de la web
// (descargas.test.ts): un Linux ARM no ve destacado el de x86_64, un universal sirve a
// todos y Windows en ARM emula x64. VISTO EN ROJO destacando por plataforma sola.
func TestSoloSeDestacaLoQueArrancaAqui(t *testing.T) {
	lista := []api.Instalador{
		{Fichero: "sytue-one-linux-amd64.tar.xz", Plataforma: "linux", Arquitectura: "amd64"},
		{Fichero: "sytue-one-linux-arm64.tar.xz", Plataforma: "linux", Arquitectura: "arm64"},
		{Fichero: "sytue-one-macos-universal.dmg", Plataforma: "macos", Arquitectura: "universal"},
		{Fichero: "sytue-one-windows-amd64-installer.exe", Plataforma: "windows", Arquitectura: "amd64"},
	}
	for _, c := range []struct{ plataforma, arq, quiero string }{
		{"linux", "arm64", "sytue-one-linux-arm64.tar.xz"},
		{"linux", "amd64", "sytue-one-linux-amd64.tar.xz"},
		{"macos", "arm64", "sytue-one-macos-universal.dmg"},
		{"windows", "arm64", "sytue-one-windows-amd64-installer.exe"},
	} {
		d, _ := Repartir(lista, c.plataforma, c.arq)
		var got []string
		for _, i := range d {
			got = append(got, i.Fichero)
		}
		if strings.Join(got, " ") != c.quiero {
			t.Errorf("%s/%s destaca %v; quiero %s", c.plataforma, c.arq, got, c.quiero)
		}
	}
}

// TestUnInstaladorQueNoEsElDelCatalogoNoSeQueda (FYS-10): con una suma distinta de la
// del catálogo, el fichero no queda en Descargas; con la buena, sí.
func TestUnInstaladorQueNoEsElDelCatalogoNoSeQueda(t *testing.T) {
	v := NuevaVista(traductor(t), nil, time.Now())
	dir := t.TempDir()
	v.carpeta = func() (string, error) { return dir, nil }
	v.ConDescargas(&descargadorDoble{})
	malo := api.Instalador{Fichero: "sytue-hub.dmg", SHA256: strings.Repeat("0", 64)}
	if _, err := v.guardarInstalador(malo); !errors.Is(err, ErrSumaInstalador) {
		t.Fatalf("una suma distinta: %v", err)
	}
	if quedan, _ := os.ReadDir(dir); len(quedan) != 0 {
		t.Fatalf("queda en Descargas: %v", quedan)
	}
	if _, err := v.guardarInstalador(api.Instalador{Fichero: "sytue-hub.dmg", SHA256: sumaDMG}); err != nil {
		t.Fatalf("con la suma buena: %v", err)
	}
}
