// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// coordenadasDePrueba son las de producción.
var coordenadasDePrueba = api.Coordenadas{
	Domain:    "sytue.eu",
	IMAP:      api.ServidorCliente{Host: "mail.sytue.eu", Port: 993, TLS: "implicit"},
	SMTP:      api.ServidorCliente{Host: "mail.sytue.eu", Port: 465, TLS: "implicit"},
	CalDAVURL: "https://calendar.sytue.eu/dav", CardDAVURL: "https://contacts.sytue.eu/dav",
	WebDAVURL: "https://drive.sytue.eu/dav", AppleProfileURL: "https://id.sytue.eu/account/apple.mobileconfig",
}

// pintaDispositivos monta la pantalla a ese ancho y la deja en `SYTUE_CAPTURAS`.
func pintaDispositivos(t *testing.T, d *Dispositivos, ventana float32, nombre string) *dispositivosRenderer {
	t.Helper()
	r := test.WidgetRenderer(d).(*dispositivosRenderer)
	ven := test.NewWindow(d)
	t.Cleanup(ven.Close)
	ven.SetPadded(false)
	d.Resize(fyne.NewSize(ventana, r.alto(ventana)))
	ven.Resize(fyne.NewSize(ventana, r.alto(ventana)))
	guardar(t, ven.Canvas().Capture(), nombre)
	return r
}

// TestDispositivosMideLoQueMideLaWeb ata el primer paso a producción
// (`medidas/portal/dispositivos.*`), con los 56 de la topbar descontados.
// Remedido en Chromium 2026-10-04: ConnectDevices real, CSS actual, raíz16.
//
// Visto en ROJO a mano el 2026-09-23 (en el frontal): con `HuecoAsistente` a 14 en vez
// de 21 todo sube desde el encabezado, y el test lo dice pieza a pieza.
func TestDispositivosMideLoQueMideLaWeb(t *testing.T) {
	type caja struct{ x, y, w, h float32 }
	casos := []struct {
		ventana                         float32
		pasos, encabezado, primera, ult caja
		continuar                       caja
		circulos                        [3]float32
	}{
		{
			ventana:    1280,
			pasos:      caja{152, 135.5, 976, 28},
			encabezado: caja{152, 187.5, 976, 30},
			primera:    caja{152, 269.5, 378, 113.75},
			ult:        caja{152, 646.75, 378, 131.625},
			continuar:  caja{1032.625, 819.375, 95.375, 36},
			circulos:   [3]float32{156, 580.65625, 1005.3125},
		},
		{
			ventana:    390,
			pasos:      caja{16, 158.25, 358, 28},
			encabezado: caja{16, 210.25, 358, 60},
			primera:    caja{16, 344.25, 358, 113.75},
			ult:        caja{16, 1098.75, 358, 153.625},
			continuar:  caja{278.625, 1293.375, 95.375, 36},
			circulos:   [3]float32{20, 183, 346},
		},
		{
			ventana:    360,
			pasos:      caja{16, 158.25, 328, 28},
			encabezado: caja{16, 210.25, 328, 60},
			primera:    caja{16, 344.25, 328, 113.75},
			ult:        caja{16, 1164.625, 328, 171.5},
			continuar:  caja{248.625, 1377.125, 95.375, 36},
			circulos:   [3]float32{20, 168, 316},
		},
	}
	cerca := func(a, b, tol float32) bool { return a-b <= tol && b-a <= tol }
	for _, c := range casos {
		d := NuevosDispositivos(castellano(t))
		r := pintaDispositivos(t, d, c.ventana, "dispositivos-"+map[float32]string{1280: "ancho", 390: "estrecho", 360: "movil"}[c.ventana])
		mira := func(nombre string, o fyne.CanvasObject, quiero caja) {
			if quiero.w == 0 {
				return
			}
			p, s := o.Position(), o.Size()
			if !cerca(p.X, quiero.x, 0.1) || !cerca(p.Y, quiero.y, 0.1) || !cerca(s.Width, quiero.w, 0.5) || !cerca(s.Height, quiero.h, 0.1) {
				t.Errorf("a %v, %s está en (%v, %v) y mide %v × %v; en producción (%v, %v) y %v × %v",
					c.ventana, nombre, p.X, p.Y, s.Width, s.Height, quiero.x, quiero.y, quiero.w, quiero.h)
			}
		}
		mira("el progreso", r.pasos, c.pasos)
		mira("el encabezado del paso", r.encabezado, c.encabezado)
		mira("la primera opción", r.opciones[0], c.primera)
		mira("la última opción", r.opciones[len(r.opciones)-1], c.ult)
		// «Continuar» va a la derecha, así que su x lleva el ancho del rótulo: 55 en el
		// volcado (hinting) y 55,45 aquí.
		c.continuar.x -= r.siguiente.Size().Width - c.continuar.w
		mira("«Continuar»", r.siguiente, c.continuar)
		// El círculo de cada paso va a `px-1` del inicio de su tramo. Los dos últimos
		// dependen del ancho de «Comprobación», que el volcado de producción mide 78 con
		// el hinting de su Chromium (avances redondeados letra a letra, `REGLAS` §3) y
		// aquí 75,84: la tolerancia es ese 3 %, no la aritmética del `flex`, que sí es
		// exacta —a 390, donde ese rótulo no se enseña, los tres caen clavados—.
		xs, _ := r.pasos.Tramos(r.pasos.Size().Width)
		for i, quiero := range c.circulos {
			if got := r.pasos.Position().X + xs[i] + 4; !cerca(got, quiero, 2.4) {
				t.Errorf("a %v, el círculo del paso %d está en x = %v y en producción en %v", c.ventana, i+1, got, quiero)
			}
		}
	}
}

// TestElAsistenteVaYVuelve recorre el asistente como lo haría alguien: sin elegir no se
// avanza; eligiendo Thunderbird salen sus instrucciones y «Copiar» manda el valor con
// su aviso; el último paso resume lo que comprobar; «Conectar otro dispositivo» vuelve
// al principio sin nada elegido.
//
// Visto en ROJO a mano el 2026-09-23 (en el frontal): dejando «Continuar» siempre
// activo, el test dice que se avanza sin haber elegido nada.
func TestElAsistenteVaYVuelve(t *testing.T) {
	d := NuevosDispositivos(castellano(t))
	d.Estado, d.Coordenadas, d.Correo = Listo, coordenadasDePrueba, "ada@acme.eu"
	var copiado, aviso string
	d.AlCopiar = func(v, k string) { copiado, aviso = v, k }
	r := pintaDispositivos(t, d, 1280, "dispositivos-paso1")

	if !r.siguiente.Inerte {
		t.Fatal("sin elegir nada, «Continuar» deja avanzar")
	}
	r.opciones[3].Tapped(nil) // Thunderbird
	if d.Plataforma != Thunderbird || r.siguiente.Inerte {
		t.Fatalf("elegir Thunderbird deja la plataforma en %q y «Continuar» inerte=%v", d.Plataforma, r.siguiente.Inerte)
	}
	r.siguiente.Tapped(nil)
	pintaDispositivos(t, d, 1280, "dispositivos-paso2")
	if d.Paso != 1 {
		t.Fatalf("tras «Continuar» se está en el paso %d", d.Paso+1)
	}
	var filas []*FilaCopia
	for _, o := range r.objetos {
		if f, ok := o.(*FilaCopia); ok {
			filas = append(filas, f)
		}
	}
	// La dirección suelta y las tres del calendario y los contactos: los cinco del
	// correo van plegados en «Ver los valores manuales».
	if len(filas) != 4 {
		t.Fatalf("hay %d filas de copiar a la vista y Thunderbird enseña cuatro", len(filas))
	}
	filas[0].boton.Tapped(nil)
	if copiado != "ada@acme.eu" || aviso != "devices.copied.address" {
		t.Errorf("«Copiar» manda %q con el aviso %q", copiado, aviso)
	}

	r.siguiente.Tapped(nil)
	pintaDispositivos(t, d, 1280, "dispositivos-paso3")
	res, ok := r.cuerpo.(tope).b.(*columna).hijos[0].(*resumen)
	if d.Paso != 2 || !ok || len(res.claves) != 2 {
		t.Fatalf("en el paso %d el resumen tiene que decir dos cosas (correo y calendario)", d.Paso+1)
	}
	otra := r.cuerpo.(tope).b.(*columna).hijos[2].(fijo).o
	otra.(fyne.Tappable).Tapped(nil)
	if d.Paso != 0 || d.Plataforma != "" {
		t.Errorf("«Conectar otro dispositivo» deja el paso %d y la plataforma %q", d.Paso+1, d.Plataforma)
	}
}

// TestSinCoordenadasNoSeInventaNada: si Sytue ID no contesta, la pantalla lo dice con su
// botón de reintentar en vez de enseñar servidores vacíos; y la vista, al entrar, las
// pide y las deja puestas.
func TestSinCoordenadasNoSeInventaNada(t *testing.T) {
	v := NuevaVista(castellano(t), nil, time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	v.cargarCoordenadas() // sin fuente
	if !v.disp.sinDatos() {
		t.Fatalf("sin fuente la pantalla queda en %v", v.disp.Estado)
	}
	v.disp.Show() // la vista la tiene escondida detrás del inicio
	r := pintaDispositivos(t, v.disp, 1280, "dispositivos-sin-datos")
	if _, ok := r.cuerpo.(*bSeccion); !ok {
		t.Errorf("sin datos el cuerpo es %T y tiene que ser la sección del aviso", r.cuerpo)
	}

	v.ConCoordenadas(func(context.Context) (api.Coordenadas, error) { return coordenadasDePrueba, nil })
	v.cargarCoordenadas()
	if v.disp.Estado != Listo || v.disp.Coordenadas.IMAP.Port != 993 {
		t.Errorf("con fuente la pantalla queda en %v con %+v", v.disp.Estado, v.disp.Coordenadas.IMAP)
	}
}

// TestLaPoliticaEsLaDelIdioma: el enlace del `DataFlow` va a la raíz en inglés y con el
// idioma delante en los demás, como `DataFlow.tsx`.
func TestLaPoliticaEsLaDelIdioma(t *testing.T) {
	if got := PoliticaDePrivacidad("sytue.eu", "en"); got != "https://sytue.eu/privacy" {
		t.Errorf("en inglés sale %q", got)
	}
	if got := PoliticaDePrivacidad("sytue.eu", "es"); got != "https://sytue.eu/es/privacy" {
		t.Errorf("en español sale %q", got)
	}
}
