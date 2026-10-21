// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// ficherosFalso es Drive en memoria: apunta lo que se le pide.
type ficherosFalso struct {
	llamadas []string
	err      error
}

// El selector del sistema puede seguir abierto al navegar o cambiar de cuenta.
// Volver a la misma sala no recupera la autorización de aquel compositor.
func TestSelectorAdjuntoConservaSuAmbito(t *testing.T) {
	for _, cambio := range []string{"sala", "ida y vuelta", "cuenta", "organización"} {
		t.Run(cambio, func(t *testing.T) {
			sa, _ := salaDePrueba(t, servicioConDos())
			fs := &ficherosFalso{}
			org := "org-a"
			sa.Org = func() string { return org }
			sa.Ficheros = fs
			sa.Arrancar(context.Background())
			ruta := filepath.Join(t.TempDir(), "privado.txt")
			if err := os.WriteFile(ruta, []byte("privado"), 0o600); err != nil {
				t.Fatal(err)
			}
			abierto, continuar := make(chan struct{}), make(chan struct{})
			sa.ElegirFicheros = func() ([]string, error) { close(abierto); <-continuar; return []string{ruta}, nil }
			var pendiente []func()
			sa.Lanzar = func(f func()) { pendiente = append(pendiente, f) }
			sa.SubirDelEquipo()
			if len(pendiente) != 1 {
				t.Fatalf("selector no iniciado: %d", len(pendiente))
			}
			fin := make(chan struct{})
			go func(f func()) { defer close(fin); f() }(pendiente[0])
			esperarAdjunto(t, abierto)
			switch cambio {
			case "sala":
				sa.abrir(Canal{ID: "d1", Nombre: "Ana", Clase: Directo})
			case "ida y vuelta":
				sa.abrir(Canal{ID: "d1", Nombre: "Ana", Clase: Directo})
				sa.abrir(Canal{ID: "c1", Nombre: "equipo"})
			case "cuenta":
				sa.Olvidar()
				sa.Yo = "otra-persona"
				sa.abrir(Canal{ID: "c1", Nombre: "equipo"})
			case "organización":
				org = "org-b"
			}
			antes := len(pendiente)
			close(continuar)
			esperarAdjunto(t, fin)
			fyne.DoAndWait(func() {}) // Procesa el retorno real del selector desde su goroutine.
			if len(pendiente) != antes || len(fs.llamadas) != 0 || len(sa.P.Canal.Compositor.Pendientes) != 0 {
				t.Fatalf("el selector antiguo inició una subida en otro ámbito: trabajos %d→%d, %v", antes, len(pendiente), fs.llamadas)
			}
		})
	}
}

func esperarAdjunto(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("operación de adjunto bloqueada")
	}
}

// Simula un recibo que llega tarde aunque el transporte haya aceptado el fichero.
// Cancelar la interfaz debe retirar también su descriptor temporal, sin enviar nada.
type subidaConReciboTardio struct {
	*ficherosFalso
	inicio, continuar chan struct{}
	mu                sync.Mutex
	pendientes        map[string]bool
}

func (f *subidaConReciboTardio) UploadToChannel(_ context.Context, _, nombre, tipo string, r io.Reader, n int64) (api.File, error) {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return api.File{}, err
	}
	close(f.inicio)
	<-f.continuar
	f.mu.Lock()
	f.pendientes["privado"] = true
	f.mu.Unlock()
	return api.File{ID: "privado", Name: nombre, ContentType: tipo, SizeBytes: n}, nil
}

func (f *subidaConReciboTardio) ForgetUpload(_, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.pendientes, id)
}

func (f *subidaConReciboTardio) ForgetUploads(string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.pendientes)
}

func TestSubidaAdjuntaTardiaNoCruzaSalaNiCuenta(t *testing.T) {
	for _, cambiarCuenta := range []bool{false, true} {
		t.Run(map[bool]string{false: "sala", true: "cuenta"}[cambiarCuenta], func(t *testing.T) {
			s := servicioConDos()
			sa, _ := salaDePrueba(t, s)
			ruta := filepath.Join(t.TempDir(), "privado.txt")
			if err := os.WriteFile(ruta, []byte("privado"), 0o600); err != nil {
				t.Fatal(err)
			}
			fs := &subidaConReciboTardio{ficherosFalso: &ficherosFalso{}, inicio: make(chan struct{}), continuar: make(chan struct{}), pendientes: map[string]bool{}}
			sa.Ficheros, sa.ElegirFicheros = fs, func() ([]string, error) { return []string{ruta}, nil }
			sa.Arrancar(context.Background())
			var trabajos []func()
			sa.Lanzar = func(f func()) { trabajos = append(trabajos, f) }
			sa.SubirDelEquipo()
			trabajos[0]() // Devuelve el selector y encola la subida real.
			fin := make(chan struct{})
			go func(f func()) { defer close(fin); f() }(trabajos[1])
			esperarAdjunto(t, fs.inicio)
			if cambiarCuenta {
				sa.Olvidar()
				sa.Yo = "otra-persona"
			}
			sa.abrir(Canal{ID: "d1", Nombre: "Ana", Clase: Directo})
			close(fs.continuar)
			esperarAdjunto(t, fin)
			fyne.DoAndWait(func() {})
			fs.mu.Lock()
			n := len(fs.pendientes)
			fs.mu.Unlock()
			if n != 0 || len(sa.P.Canal.Compositor.Pendientes) != 0 || len(s.envios) != 0 {
				t.Fatalf("recibo antiguo dejó clave o envío en otro ámbito: %d, %+v, %+v", n, sa.P.Canal.Compositor.Pendientes, s.envios)
			}
		})
	}
}

func (f *ficherosFalso) UploadToChannel(_ context.Context, canal, nombre, tipo string, r io.Reader, n int64) (api.File, error) {
	b, _ := io.ReadAll(r)
	f.llamadas = append(f.llamadas, "subir "+canal+" "+nombre+" "+tipo+" "+string(b))
	return api.File{ID: "f-" + nombre, Name: nombre, ContentType: tipo, SizeBytes: n}, f.err
}

func (f *ficherosFalso) ChannelContent(_ context.Context, canal, id string, mini bool) (io.ReadCloser, error) {
	f.llamadas = append(f.llamadas, "bajar "+canal+" "+id+map[bool]string{true: " mini", false: ""}[mini])
	var b bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	img.Set(0, 0, color.Black)
	_ = png.Encode(&b, img)
	return io.NopCloser(&b), f.err
}

func (f *ficherosFalso) SaveChannelFileToMyDrive(_ context.Context, canal, id string) (api.File, error) {
	f.llamadas = append(f.llamadas, "guardar "+canal+" "+id)
	return api.File{ID: "copia"}, f.err
}

// La miniatura se escala sin deformar.
func TestMedidaMiniatura(t *testing.T) {
	if MedidaMiniatura(400, 200) != MedidaMiniatura(800, 400) || MedidaMiniatura(100, 50).Width != 102 {
		t.Error("la miniatura no se escala a `max-h-64 max-w-[16rem]` sin deformar")
	}
}

// Un mensaje con adjuntos pinta la miniatura de su imagen —la pide UNA vez al canal, no
// en cada repintado— y la tarjeta del fichero, que se guarda en «Mi unidad».
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): sin apuntar la miniatura
// como pedida, cada repintado del historial la vuelve a bajar.
func TestLosAdjuntosDeUnMensaje(t *testing.T) {
	s := servicioConDos()
	s.mensajes["c1"][0].Attachments = []api.RoomsAttachment{
		{FileID: "img", Name: "foto.png", ContentType: "image/png"},
		{FileID: "doc", Name: "acta.pdf", ContentType: "application/pdf", SizeBytes: 2048},
	}
	sa, _ := salaDePrueba(t, s)
	// La miniatura FALLA: una que falla se queda en su hueco igual que una que está en
	// camino, y es el caso que se ve sin hilos. Una que llega ya se queda en la caché.
	fs := &ficherosFalso{err: errors.New("caído")}
	sa.Ficheros = fs
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	f := filaDe(t, h, "m1")
	test.WidgetRenderer(f)
	if f.adjuntos == nil || len(h.Mensajes[0].Adjuntos) != 2 {
		t.Fatal("la fila no lleva sus adjuntos")
	}
	h.Refresh()
	h.Refresh()
	n := 0
	for _, l := range fs.llamadas {
		if l == "bajar c1 img mini" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("la miniatura se pide %d veces: %v", n, fs.llamadas)
	}
	fs.err = nil
	sa.GuardarEnDrive("c1", h.Mensajes[0].Adjuntos[1])
	if fs.llamadas[len(fs.llamadas)-1] != "guardar c1 doc" {
		t.Errorf("guardar en mi unidad: %v", fs.llamadas)
	}
	fs.err = errors.New("caído")
	sa.GuardarEnDrive("c1", h.Mensajes[0].Adjuntos[1])
}

// Subir del equipo pone la ficha en la caja, sube al ÁMBITO del canal y, subido, el
// mensaje sale con su `file_id` aunque no lleve texto; al enviar la caja se vacía.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con Enviar pidiendo texto como antes, el
// mensaje de solo adjuntos no sale.
func TestSubirUnAdjuntoYEnviarlo(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "notas.txt")
	if err := os.WriteFile(ruta, []byte("hola"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	fs := &ficherosFalso{}
	sa.Ficheros, sa.ElegirFicheros = fs, func() ([]string, error) { return []string{ruta}, nil }
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	sa.SubirDelEquipo()
	if len(c.Pendientes) != 1 || c.Pendientes[0].Subido == nil || !strings.HasPrefix(fs.llamadas[0], "subir c1 notas.txt text/plain") {
		t.Fatalf("la subida deja %+v: %v", c.Pendientes, fs.llamadas)
	}
	if !c.puedeEnviar() {
		t.Fatal("con un adjunto subido y sin texto no se puede enviar")
	}
	c.Someter()
	if len(s.envios) != 1 || len(s.envios[0].FileIDs) != 1 || s.envios[0].FileIDs[0] != "f-notas.txt" || len(c.Pendientes) != 0 {
		t.Errorf("sale %+v y quedan %d pendientes", s.envios, len(c.Pendientes))
	}
}

// subidaAMedias lee la mitad, mira la caja y sigue: es donde la web enseña su «50%». Si
// entretanto la han cortado, lo apunta y no sigue.
type subidaAMedias struct {
	*ficherosFalso
	mirar    func()
	cortadas int
}

func (f *subidaAMedias) UploadToChannel(ctx context.Context, canal, nombre, tipo string, r io.Reader, n int64) (api.File, error) {
	if _, err := io.ReadFull(r, make([]byte, n/2)); err != nil {
		return api.File{}, err
	}
	f.mirar()
	if err := ctx.Err(); err != nil {
		f.cortadas++
		return api.File{}, err
	}
	return f.ficherosFalso.UploadToChannel(ctx, canal, nombre, tipo, r, n)
}

// A medio subir, la ficha dice cuánto lleva; subida, ya no dice nada.
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): subiendo el fichero sin
// ConProgreso, a medias la ficha sigue en «0%»; y sin cancelar al quitar la ficha, la
// subida sigue hasta el final.
func TestLaFichaDiceCuantoLlevaSubido(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "acta.pdf")
	if err := os.WriteFile(ruta, bytes.Repeat([]byte("x"), 200), 0o600); err != nil {
		t.Fatal(err)
	}
	sa, _ := salaDePrueba(t, servicioConDos())
	c := sa.P.Canal.Compositor
	var aMedias string
	fs := &subidaAMedias{ficherosFalso: &ficherosFalso{}}
	fs.mirar = func() { aMedias = NuevaFichaPendiente(sa.P.T, c.Pendientes[0], nil).estado() }
	sa.Ficheros, sa.ElegirFicheros = fs, func() ([]string, error) { return []string{ruta}, nil }
	sa.Arrancar(context.Background())
	sa.SubirDelEquipo()
	if aMedias != "50%" {
		t.Errorf("a medio subir la ficha dice %q", aMedias)
	}
	if len(c.Pendientes) != 1 || c.Pendientes[0].Subido == nil || NuevaFichaPendiente(sa.P.T, c.Pendientes[0], nil).estado() != "" {
		t.Errorf("subido queda %+v", c.Pendientes)
	}

	// Quitar la ficha a medio subir corta la subida, como el AbortController de la web.
	c.PonerPendientes(nil)
	fs.mirar = func() { test.Tap(c.objPendientes[0].(*FichaPendiente).quitar) }
	sa.SubirDelEquipo()
	if fs.cortadas != 1 || len(c.Pendientes) != 0 {
		t.Errorf("quitar a medias: %d cortadas, quedan %+v", fs.cortadas, c.Pendientes)
	}
}

// «Elegir de Sytue Drive» abre el selector de la suite en modo Drive y el fichero
// elegido entra en la caja por referencia, ya subido, sin repetirse.
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin el `ContainsFunc` de adjuntarDeDrive,
// elegir dos veces el mismo fichero deja dos fichas.
func TestAdjuntarDesdeDrive(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.ListarDrive = func(context.Context, string, string, string) (api.PaginaSuite, error) {
		return api.PaginaSuite{Elegibles: []api.Elegible{{ID: "fd", Nombre: "plan.xlsx", URL: "u", Bytes: 9}}}, nil
	}
	sa.Arrancar(context.Background())
	sa.ElegirDeDrive()
	sel := sa.selectorDrive
	if sel != nil && sel.Textos.Pie != sa.P.T.T("drive.byReference", nil) {
		t.Errorf("el selector va sin su nota de pie: %q", sel.Textos.Pie)
	}
	if sel == nil || sel.Origen != api.ElegirDrive {
		t.Fatal("no se abre el selector de Drive")
	}
	e := api.Elegible{ID: "fd", Nombre: "plan.xlsx", URL: "u", Bytes: 9}
	sel.AlElegir(e)
	sel.AlElegir(e)
	c := sa.P.Canal.Compositor
	if len(c.Pendientes) != 1 || c.Pendientes[0].Subido == nil || c.Pendientes[0].Subido.FileID != "fd" || !c.puedeEnviar() {
		t.Errorf("la caja queda con %+v", c.Pendientes)
	}
}

// TestAbrirSoloSiCasanElNombreYElTipo (FYC-01, decidido por el dueño: «Abrir solo si
// casan»): un «.html» declarado `application/pdf` solo se descarga, y se guarda con la
// extensión del tipo declarado; un PDF de verdad se abre.
func TestAbrirSoloSiCasanElNombreYElTipo(t *testing.T) {
	for _, c := range []struct {
		a             Adjunto
		abre          bool
		nombreEnDisco string
	}{
		{Adjunto{Nombre: "acta.pdf", Tipo: "application/pdf"}, true, "acta.pdf"},
		{Adjunto{Nombre: "ACTA.PDF", Tipo: "application/pdf"}, true, "ACTA.PDF"},
		{Adjunto{Nombre: "notas.txt", Tipo: "text/plain; charset=utf-8"}, true, "notas.txt"},
		{Adjunto{Nombre: "factura.html", Tipo: "application/pdf"}, false, "factura.html.pdf"},
		{Adjunto{Nombre: "leeme.hta", Tipo: "text/plain"}, false, "leeme.hta.txt"},
		{Adjunto{Nombre: "foto.png", Tipo: "image/png"}, false, "foto.png"},
		{Adjunto{Nombre: "copia.zip", Tipo: "application/zip"}, false, "copia.zip"},
	} {
		if got := c.a.SeAbre(); got != c.abre {
			t.Errorf("%s (%s): se abre=%v, quiero %v", c.a.Nombre, c.a.Tipo, got, c.abre)
		}
		if got := c.a.nombreEnDisco(); got != c.nombreEnDisco {
			t.Errorf("%s (%s): en disco %q, quiero %q", c.a.Nombre, c.a.Tipo, got, c.nombreEnDisco)
		}
	}
	// Y la tarjeta no ofrece «Abrir» cuando no casan.
	v := &VistaAdjuntos{T: traductor(t)}
	for _, c := range []struct {
		a    Adjunto
		abre bool
	}{
		{Adjunto{ID: "1", Nombre: "factura.html", Tipo: "application/pdf"}, false},
		{Adjunto{ID: "2", Nombre: "acta.pdf", Tipo: "application/pdf"}, true},
	} {
		hay := false
		var buscar func(o fyne.CanvasObject)
		buscar = func(o fyne.CanvasObject) {
			if b, ok := o.(*ui.BotonIcono); ok && strings.Contains(b.Rotulo, c.a.Nombre) && b.Icono == iconos.FlechaArribaDerecha {
				hay = true
			}
			if k, ok := o.(*fyne.Container); ok {
				for _, h := range k.Objects {
					buscar(h)
				}
			}
		}
		buscar(v.fichero(c.a))
		if hay != c.abre {
			t.Errorf("%s: botón Abrir=%v, quiero %v", c.a.Nombre, hay, c.abre)
		}
	}
}
