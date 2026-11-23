// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// appConCifrada es una App con una foto cifrada indexada (IMG_1, 30×20) entre dos del
// servidor, una más reciente y otra más antigua, y la línea del servidor ya entera. Las
// Fototecas apuntan lo que piden a la red.
func appConCifrada(t *testing.T) (*App, *almacenCifrado, *[]string) {
	t.Helper()
	test.NewTempApp(t)
	var mu sync.Mutex
	alServidor := &[]string{}
	red := func(_ context.Context, id, _ string) ([]byte, error) {
		mu.Lock()
		*alServidor = append(*alServidor, id)
		mu.Unlock()
		return nil, errors.New("sin red en la prueba")
	}
	teca, grande := NuevaFototeca(), NuevaFototeca()
	teca.Pedir, grande.Pedir = red, red
	grande.Variante = VarianteGrande
	v := vistaDePrueba(t, teca)
	v.TecaGrande = grande
	a := Nueva(nil, v, nil)
	a.lanzar = func(f func()) { f() }
	a.GaleriaCifrada = func(context.Context, bool) (*api.Drive, error) { return nil, ErrDriveCifradoCerrado }
	a.desviarCifradas()
	almacen, _ := galeriaDePrueba(t, a)
	tomada := a.cif.indice.Fotos[0].Entrada.Tomada
	a.g.fotos = []api.Photo{
		{ID: "s1", Name: "nueva.jpg", TakenAt: tomada.Add(24 * time.Hour), Width: 4, Height: 3},
		{ID: "s2", Name: "vieja.jpg", TakenAt: tomada.Add(-24 * time.Hour), Width: 4, Height: 3},
	}
	a.g.fin = true
	return a, almacen, alServidor
}

// TestVisorPasaPorLaLineaMezclada: el visor a pantalla completa recorre la línea que se
// ve, con las cifradas en su sitio: de la del servidor a la cifrada y a la siguiente, y
// vuelta. Una cifrada se abre en el mismo visor, no en un diálogo, y su foto grande se
// descifra en el dispositivo sin pedir nada a la red.
func TestVisorPasaPorLaLineaMezclada(t *testing.T) {
	a, _, alServidor := appConCifrada(t)
	cifrada := idDeLinea(a.cif.indice.Fotos[0].Fichero.ID)

	a.abrirEnGaleria("s1")
	if a.vis == nil || !enLaFoto(a.vis, "s1") {
		t.Fatal("no se abrió la del servidor")
	}
	ids := func() string { return idsDeLinea(a.vis.lista()) }
	if got := ids(); got != "s1 "+cifrada+" s2" {
		t.Fatalf("lista del visor = %q", got)
	}
	a.siguiente()
	if !enLaFoto(a.vis, cifrada) || a.textoPosicion(a.vis) != "2 / 3" {
		t.Fatalf("tras s1 va la cifrada; está en %d (%q)", a.vis.i, a.textoPosicion(a.vis))
	}
	a.siguiente()
	if !enLaFoto(a.vis, "s2") {
		t.Fatal("tras la cifrada va s2")
	}
	a.anterior()
	if !enLaFoto(a.vis, cifrada) {
		t.Fatal("hacia atrás, otra vez la cifrada")
	}
	a.cerrarVisor()

	// Abierta desde su tesela, en el visor y en su sitio.
	a.abrirDeLinea(FotoDeLinea(a.cif.indice.Fotos[0]))
	if a.vis == nil || a.vis.i != 1 || !enLaFoto(a.vis, cifrada) {
		t.Fatal("la cifrada no se abrió en el visor")
	}
	// La foto grande del visor, descifrada en el dispositivo.
	grande, err := a.v.TecaGrande.Pedir(t.Context(), cifrada, VarianteGrande)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(grande)); err != nil || cfg.Width != 30 || cfg.Height != 20 {
		t.Fatalf("la grande mide %+v (%v)", cfg, err)
	}
	// Y la foto entera del tamaño real.
	a.alternarTamanyoReal()
	if a.vis.original == nil || a.vis.original.img.Bounds().Dx() != 30 {
		t.Fatal("el tamaño real no tiene la foto entera descifrada")
	}
	if slices.Contains(*alServidor, cifrada) {
		t.Fatalf("se pidió la cifrada a la red: %q", *alServidor)
	}
}

// TestVisorDeCifradaSinLoDelServidor: con una cifrada, el visor no ofrece lo que pasa
// por el servidor (parecidas, describir, girar, la papelera, preguntar, la galería del
// sistema) y la ficha dice por qué con el candado; sí la favorita, sus álbumes cifrados
// (marcar es meterla), «Descargar» y «Editar». Con una del servidor, todo sigue.
func TestVisorDeCifradaSinLoDelServidor(t *testing.T) {
	a, almacen, _ := appConCifrada(t)
	a.DestinoGaleria = GaleriaImagenes
	a.Galeria = nuevaGaleria(t)
	foto := a.cif.indice.Fotos[0]
	cifrada := idDeLinea(foto.Fichero.ID)
	album, err := a.cif.galeria.CrearAlbum(t.Context(), "Cifrado")
	if err != nil {
		t.Fatal(err)
	}
	a.cif.indice.Albumes = append(a.cif.indice.Albumes, album)

	a.abrirEnGaleria("s1")
	servidor := a.datosVisor(a.vis, api.Photo{ID: "s1"})
	if servidor.Cifrada || !slices.Contains(rotulosDeMandos(a.v, servidor), a.v.Traduce("viewer.similar")) {
		t.Fatal("una del servidor tiene sus mandos")
	}

	a.abrirEnGaleria(cifrada)
	f, _ := a.vis.foto()
	d := a.datosVisor(a.vis, f)
	if !d.Cifrada || !d.ConFavorita || d.ConBorrar || d.ConDescribir || d.AlSistema != nil || d.AlPreguntar != nil || d.AlParecidas != nil || d.AlGirar != nil {
		t.Fatalf("visor de una cifrada = %+v", d)
	}
	rotulos := rotulosDeMandos(a.v, d)
	for _, fuera := range []string{"viewer.similar", "viewer.describe", "viewer.rotate", "viewer.delete"} {
		if slices.Contains(rotulos, a.v.Traduce(fuera)) {
			t.Errorf("una cifrada no ofrece %s: %q", fuera, rotulos)
		}
	}
	for _, dentro := range []string{"gallery.favorite", "viewer.download", "gallery.encrypted.albums"} {
		if !slices.Contains(rotulos, a.v.Traduce(dentro)) {
			t.Errorf("falta %s: %q", dentro, rotulos)
		}
	}
	ficha := textosConDesplazamiento(FichaDelVisor(a.v, d))
	if !slices.Contains(ficha, a.v.Traduce("gallery.encrypted.note")) || slices.Contains(ficha, a.v.Traduce("viewer.ask")) {
		t.Fatalf("ficha de una cifrada = %q", ficha)
	}

	// Los atajos tampoco: Supr no la manda a la papelera, ni Sytue AI la describe.
	a.teclaVisor(&fyne.KeyEvent{Name: fyne.KeyDelete})
	a.describirFoto()
	a.girar()
	if a.vis == nil || !enLaFoto(a.vis, cifrada) || a.vis.describiendo || a.vis.girando {
		t.Fatal("un atajo hizo con la cifrada lo que pasa por el servidor")
	}

	// La favorita y el álbum, solo en el índice de la bóveda.
	d.AlFavorita()
	if !a.cif.indice.Fotos[0].Entrada.Favorita {
		t.Fatal("la favorita no se apuntó")
	}
	var lista []api.Album
	d.AlMenuAlbumes(func(l []api.Album) { lista = l })
	if len(lista) != 1 || lista[0].ID != album.ID || d.EnAlbumes[album.ID] || d.AlNuevoAlbum == nil {
		t.Fatalf("menú de álbumes = %+v, %v", lista, d.EnAlbumes)
	}
	d.AlAnyadirA(lista[0])
	if len(a.cif.indice.DelAlbum(album.ID)) != 1 {
		t.Fatal("no se metió en el álbum cifrado")
	}
	f, _ = a.vis.foto()
	d = a.datosVisor(a.vis, f)
	if !d.EnAlbumes[album.ID] || !f.Favorite {
		t.Fatalf("el visor no ve lo que cambió: %+v, %v", f, d.EnAlbumes)
	}
	d.AlAnyadirA(lista[0])
	if len(a.cif.indice.DelAlbum(album.ID)) != 0 {
		t.Fatal("desmarcarlo la saca del álbum")
	}
	if almacen.ficheros[foto.Fichero.ID].Version != 1 {
		t.Fatal("la favorita y los álbumes no tocan la foto")
	}

	// Desde el álbum cifrado, el visor pasa por las fotos del álbum.
	f, _ = a.vis.foto()
	a.datosVisor(a.vis, f).AlAnyadirA(lista[0])
	a.cerrarVisor()
	a.cif.album = album.ID
	a.abrirVisor(desdeAlbumCifrado, 0)
	if !enLaFoto(a.vis, cifrada) || len(a.vis.lista()) != 1 {
		t.Fatal("el visor del álbum cifrado no tiene su foto")
	}
}

// TestDescargarUnaCifradaSoloVaAlDestino: «Descargar» de una cifrada escribe la foto
// descifrada solo por Guardar —ni en el Drive, ni en un fichero temporal—, con el
// nombre y el formato pedidos.
func TestDescargarUnaCifradaSoloVaAlDestino(t *testing.T) {
	a, almacen, alServidor := appConCifrada(t)
	cifrada := idDeLinea(a.cif.indice.Fotos[0].Fichero.ID)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	type guardada struct {
		nombre string
		datos  []byte
	}
	var guardadas []guardada
	a.Guardar = func(nombre string, datos []byte) (string, error) {
		guardadas = append(guardadas, guardada{nombre, bytes.Clone(datos)})
		return filepath.Join("descargas", nombre), nil
	}
	almacen.mu.Lock()
	antes := len(almacen.ficheros)
	almacen.mu.Unlock()

	a.abrirEnGaleria(cifrada)
	f, _ := a.vis.foto()
	a.datosVisor(a.vis, f).AlDescargar("png")
	a.descargar(f, "jpeg")
	if len(guardadas) != 2 || guardadas[0].nombre != "IMG_1.png" || guardadas[1].nombre != "IMG_1.jpg" {
		t.Fatalf("guardadas = %d %+v", len(guardadas), guardadas)
	}
	for _, g := range guardadas {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(g.datos))
		if err != nil || cfg.Width != 30 || cfg.Height != 20 {
			t.Fatalf("%s no es la foto descifrada: %+v (%v)", g.nombre, cfg, err)
		}
	}
	almacen.mu.Lock()
	despues := len(almacen.ficheros)
	almacen.mu.Unlock()
	if despues != antes {
		t.Fatalf("descargar escribió en el Drive: %d → %d", antes, despues)
	}
	if entradas, _ := os.ReadDir(tmp); len(entradas) != 0 {
		t.Fatalf("descargar dejó ficheros temporales: %v", entradas)
	}
	if slices.Contains(*alServidor, cifrada) {
		t.Fatalf("descargar una cifrada pidió a la red: %q", *alServidor)
	}
}

// textosConDesplazamiento es textosDePantalla bajando también por los desplazamientos.
func textosConDesplazamiento(o fyne.CanvasObject) []string {
	switch x := o.(type) {
	case *fyne.Container:
		out := make([]string, 0, len(x.Objects))
		for _, h := range x.Objects {
			out = append(out, textosConDesplazamiento(h)...)
		}
		return out
	case *container.Scroll:
		return textosConDesplazamiento(x.Content)
	}
	return textosDePantalla(o)
}

// rotulosDeMandos son los nombres accesibles de los mandos de la cabecera del visor.
func rotulosDeMandos(v Vista, d Visor) []string {
	mandos := mandosVisor(v, d)
	out := make([]string, 0, 2*len(mandos))
	for _, h := range mandos {
		switch x := h.(type) {
		case *ui.BotonIcono:
			out = append(out, x.Rotulo)
		case *ui.Boton:
			out = append(out, x.Texto, x.Rotulo)
		}
	}
	return out
}
