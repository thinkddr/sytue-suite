// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// LAS CAPTURAS DE LA APP: el cajón del teléfono con cuentas de correo externas, que es
// la captura del dueño de MAIL-15 (390 de ancho, el de un iPhone), y el carril de
// escritorio con las mismas cuentas. Sin `SYTUE_CAPTURAS` no guardan nada y solo
// comprueban que pintan.

// direccionLarga es la de la captura del dueño, que se cortaba en «…@outlook.cor».
const direccionLarga = "daniel.delgador@outlook.com"

// carpetasDeOutlook son las de la captura: Recibidos y Spam con dos sin leer, el resto
// a cero y la Bandeja de salida con su rol.
var carpetasDeOutlook = []api.ExternalMailFolder{
	{ID: "in", Name: "Inbox", Role: "inbox", Unread: 2},
	{ID: "dr", Name: "Drafts", Role: "drafts"},
	{ID: "se", Name: "Sent Items", Role: "sent"},
	{ID: "ar", Name: "Archive", Role: "archive"},
	{ID: "sp", Name: "Junk Email", Role: "junk", Unread: 2},
	{ID: "de", Name: "Deleted Items", Role: "trash"},
	{ID: "ob", Name: "Outbox", Role: "outbox"},
}

// cuentaExterna es una cuenta de Outlook conectada con esa dirección.
func cuentaExterna(id, direccion string) api.ExternalMailAccount {
	return api.ExternalMailAccount{
		InstanceID: id, Provider: "microsoft", Address: direccion, Connected: true, Services: []string{"mail"},
		Capabilities: api.ExternalMailCapabilities{Read: true, Send: true, Actions: true},
	}
}

// cajonConCuentas monta Mail en una ventana de `ancho`×`alto` con esas cuentas (todas
// con las carpetas de Outlook) y, en el teléfono, el cajón abierto.
func cajonConCuentas(t *testing.T, ancho, alto float32, cuentas ...api.ExternalMailAccount) (*Buzon, fyne.Window) {
	t.Helper()
	ui.SinMovimiento = true
	t.Cleanup(func() { ui.SinMovimiento = false })
	carpetas := map[string][]api.ExternalMailFolder{}
	for _, c := range cuentas {
		carpetas[c.InstanceID] = carpetasDeOutlook
	}
	s := &correoFalso{cuentas: api.ExternalMailAccounts{Items: cuentas}, carpetas: carpetas}
	b := correoDePrueba(t, s).b
	w := test.NewWindow(b.P.Objeto())
	w.SetPadded(false)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(ancho, alto))
	b.ponerCarpetas([]api.Mailbox{
		{ID: "sytue-inbox", Name: "Inbox", Role: "inbox", UnreadEmails: 3},
		{ID: "sytue-junk", Name: "Junk", Role: "junk", UnreadEmails: 24},
		{ID: "sytue-trash", Name: "Trash", Role: "trash", UnreadEmails: 2},
	})
	b.carpetaID, b.P.Menu.CarpetaID = "sytue-inbox", "sytue-inbox"
	b.cargarExternas()
	if ancho < ui.Estrecho {
		b.P.ponerCajon(true)
	}
	b.P.Refrescar()
	return b, w
}

// lateralDe es el menú montado en la ventana.
func lateralDe(t *testing.T, w fyne.Window) *ui.Lateral {
	t.Helper()
	var l *ui.Lateral
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		if x, es := o.(*ui.Lateral); es && l == nil {
			l = x
		}
	})
	if l == nil {
		t.Fatal("no hay menú lateral")
	}
	return l
}

// recorrer visita el árbol: contenedores, desplazadores y lo que dibujan los widgets.
func recorrer(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	if o == nil || !o.Visible() {
		return
	}
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			recorrer(h, f)
		}
	case *container.Scroll:
		recorrer(x.Content, f)
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			recorrer(h, f)
		}
	}
}

func guardarCaptura(t *testing.T, img image.Image, nombre string) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, nombre+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura en %s/%s.png", dir, nombre)
}

func TestLasCapturasDeLaApp(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	larga := cuentaExterna("larga", direccionLarga)
	casos := []struct {
		nombre      string
		ancho, alto float32
		modo        string
		cuentas     []api.ExternalMailAccount
	}{
		{"movil-cajon-cuenta-externa", 390, 844, "claro", []api.ExternalMailAccount{larga}},
		{"movil-estrecho-cajon-cuenta-externa", 300, 650, "claro", []api.ExternalMailAccount{larga}},
		{"movil-cajon-cuenta-externa-oscuro", 390, 844, "oscuro", []api.ExternalMailAccount{larga}},
		{"movil-cajon-tres-cuentas", 390, 844, "claro", []api.ExternalMailAccount{
			larga, cuentaExterna("trabajo", "ana@trabajo.eu"), cuentaExterna("otra", "una.direccion.muy.larga.de.verdad@empresa-con-nombre-largo.eu"),
		}},
		{"escritorio-cuenta-externa", 1280, 800, "claro", []api.ExternalMailAccount{larga}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tema.Elegir(c.modo)
			_, w := cajonConCuentas(t, c.ancho, c.alto, c.cuentas...)
			img := w.Canvas().Capture()
			if img.Bounds().Dx() == 0 {
				t.Fatal("la captura ha salido vacía")
			}
			guardarCaptura(t, img, "mail-"+c.nombre)
		})
	}
	// La página de ajustes (`MailSettings.tsx`): en ancho, el botón a la derecha de cada
	// fila; en el teléfono, debajo de su texto.
	for _, c := range []struct {
		nombre      string
		ancho, alto float32
		modo        string
	}{
		{"escritorio-ajustes", 1280, 800, "claro"},
		{"escritorio-ajustes-oscuro", 1280, 800, "oscuro"},
		{"movil-ajustes", 390, 844, "claro"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			tema.Elegir(c.modo)
			s := servicioConTodas()
			fin := time.Now().AddDate(0, 0, 10)
			s.vacaciones = api.MailVacation{Enabled: true, Subject: "Fuera", EndsAt: &fin}
			b, w, _ := buzonConVentana(t, s, fyne.NewSize(c.ancho, c.alto))
			b.firma = "Ana"
			b.verAjustes()
			img := w.Canvas().Capture()
			if img.Bounds().Dx() == 0 {
				t.Fatal("la captura ha salido vacía")
			}
			guardarCaptura(t, img, "mail-"+c.nombre)
		})
	}
	// «Opciones de búsqueda» (MAIL-07), abierto con una consulta escrita: los campos
	// rellenos con lo que decía y colgando del embudo junto al buscador.
	for _, c := range []struct {
		nombre      string
		ancho, alto float32
		modo        string
	}{
		{"escritorio-opciones-busqueda", 1280, 800, "claro"},
		{"escritorio-opciones-busqueda-oscuro", 1280, 800, "oscuro"},
		{"movil-opciones-busqueda", 390, 844, "claro"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			tema.Elegir(c.modo)
			b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(c.ancho, c.alto))
			bandeja := b.P.Bandeja
			bandeja.Consulta = `from:ana subject:"informe anual" has:attachment after:2026-01-01 factura`
			r := test.WidgetRenderer(bandeja).(*bandejaRenderer)
			r.busqueda.PonerTexto(bandeja.Consulta)
			bandeja.abrirOpcionesBusqueda(r.opciones)
			img := w.Canvas().Capture()
			if img.Bounds().Dx() == 0 {
				t.Fatal("la captura ha salido vacía")
			}
			guardarCaptura(t, img, "mail-"+c.nombre)
		})
	}
}

// MAIL-15: en el cajón del teléfono, más estrecho que los 260 del carril, una dirección
// larga se salía por la derecha y con ella el contenido entero del cuerpo: los galones
// quedaban fuera, los contadores debajo de la barra de desplazamiento («2» cortado en
// Recibidos y Spam) y el rótulo de la sección llegaba al borde. Ahora todo cabe en el
// cajón, la barra va en el canalón derecho y la dirección conserva su dominio.
//
// VISTO EN ROJO (2026-11-09) con el desplazador de antes (dentro de los dos canalones y
// tan ancho como su fila más ancha), y con la fila de la cuenta en dos líneas.
func TestElCajonDelTelefonoCabeConUnaCuentaLarga(t *testing.T) {
	// 300 de ancho: el cajón se queda en 244, como en la captura del dueño.
	b, w := cajonConCuentas(t, 300, 650, cuentaExterna("larga", direccionLarga))
	l := lateralDe(t, w)
	d := fyne.CurrentApp().Driver()
	borde := d.AbsolutePositionForObject(l).X + l.Size().Width
	canal := tema.Espacio(4)
	if l.Size().Width >= tema.AnchoLateral {
		t.Fatalf("el cajón mide %v: en este teléfono es más estrecho que el carril", l.Size().Width)
	}
	// El desplazador llega al borde: su barra se pinta en el canalón, no encima.
	var desplaza *container.Scroll
	recorrer(l, func(o fyne.CanvasObject) {
		if x, es := o.(*container.Scroll); es && desplaza == nil {
			desplaza = x
		}
	})
	if desplaza == nil {
		t.Fatal("el cuerpo del menú no desplaza")
	}
	if x := d.AbsolutePositionForObject(desplaza).X + desplaza.Size().Width; x != borde {
		t.Errorf("el desplazador acaba en x = %v y el cajón en %v: la barra pisa los contadores", x, borde)
	}
	var cuenta *ui.FilaLateral
	contadores := 0
	recorrer(l, func(o fyne.CanvasObject) {
		f, es := o.(*ui.FilaLateral)
		if !es {
			return
		}
		x := d.AbsolutePositionForObject(f).X + f.Size().Width
		if x > borde-canal+0.01 {
			t.Errorf("la fila %q acaba en x = %v, dentro del canalón de la barra (%v)", f.Rotulo, x, borde-canal)
		}
		if f.Contador != "" {
			contadores++
		}
		if f.Rotulo == direccionLarga {
			cuenta = f
		}
	})
	if cuenta == nil {
		t.Fatal("no está la cuenta en el cajón")
	}
	if contadores < 4 {
		t.Errorf("solo %d filas con contador: faltan los de la cuenta", contadores)
	}
	// La dirección en una línea, con el dominio entero y los puntos en medio.
	var textos []string
	for _, o := range test.WidgetRenderer(cuenta).Objects() {
		if txt, es := o.(*canvas.Text); es && txt.Visible() && txt.Text != "" {
			textos = append(textos, txt.Text)
		}
	}
	if len(textos) != 1 || !strings.HasSuffix(textos[0], "@outlook.com") {
		t.Errorf("la cuenta se pinta %q: en una línea y con «@outlook.com» entero", textos)
	}
	// Una sola cuenta: abierta, con sus carpetas a menos sangría que «Buzones».
	if !b.P.Menu.cuentaAbierta("larga") {
		t.Error("una cuenta sola arranca plegada")
	}
	_, hijos := ramaDe(t, b.P.Menu.seccionExternas(tema.Claro), direccionLarga)
	if len(hijos) != len(carpetasDeOutlook) {
		t.Fatalf("la cuenta abierta tiene %d carpetas", len(hijos))
	}
	if g := GlifoCarpeta("outbox"); g != BandejaSalidaD {
		t.Errorf("la Bandeja de salida lleva %q", g)
	}
	// Las carpetas a cero no pintan contador, pero se ven.
	for _, h := range hijos {
		if (h.Contador == "") != (h.Rotulo != b.P.T.T("folder.inbox", nil) && h.Rotulo != b.P.T.T("folder.junk", nil)) {
			t.Errorf("la carpeta %q lleva contador %q", h.Rotulo, h.Contador)
		}
	}
}
