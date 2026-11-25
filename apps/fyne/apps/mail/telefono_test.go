// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"errors"
	"net"
	"net/url"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Lo que la auditoría estética del 2026-11-24 vio en Mail en el teléfono.

// TestLaListaQueFallaOfreceReintentar: un fallo del servicio que no es una respuesta
// HTTP salía como «Sin conexión» y sin «Reintentar», con la red funcionando. Ahora es
// «No se pudo cargar el correo» con su botón; y «Sin conexión», cuando de verdad no se
// llega al servidor, también lo lleva, porque la red vuelve sin avisar.
//
// VISTO EN ROJO con el `estadoDe` y el vacío de antes: el `serverFail` sale «Sin
// conexión» y ninguno de los dos casos tiene botón.
func TestLaListaQueFallaOfreceReintentar(t *testing.T) {
	sinRed := &url.Error{
		Op: "Get", URL: "https://mail.sytue.eu/api/v1/messages",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("no route to host")},
	}
	for _, c := range []struct {
		nombre string
		err    error
		estado Estado
		titulo string
	}{
		{"servidor", errors.New("mail: Email/query: serverFail"), Error, "list.errorTitle"},
		{"red", sinRed, SinConexion, "offline.status"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			s := servicioConTodas()
			s.errLista = c.err
			b, _, _ := buzonConVentana(t, s, fyne.NewSize(390, 844))
			if b.P.Bandeja.Estado != c.estado {
				t.Fatalf("la lista queda en %d y tocaba %d", b.P.Bandeja.Estado, c.estado)
			}
			x := vacioVisible(b.P.Bandeja)
			if x == nil || x.Titulo != b.P.T.T(c.titulo, nil) {
				t.Fatalf("el vacío dice %+v", x)
			}
			boton, ok := x.Accion.(*ui.Boton)
			if !ok || boton.Texto != b.P.T.T("common.retry", nil) {
				t.Fatalf("sin «Reintentar»: %#v", x.Accion)
			}
			s.errLista = nil
			antes := s.pidio("Messages")
			test.Tap(boton)
			if s.pidio("Messages") == antes || b.P.Bandeja.Estado != Listo {
				t.Errorf("«Reintentar» no vuelve a pedir la lista: estado %d", b.P.Bandeja.Estado)
			}
		})
	}
}

// TestLaRespuestaAbrePorElPrincipio: al responder, «Para» enseñaba el FINAL de la
// dirección («…nombre-largo.eu>», sin el nombre) porque el cursor se quedaba al final y
// el campo corría el texto hasta él; y el asunto, que no corría, se pintaba más allá de
// su fila hasta el borde de la hoja. Los dos abren por el principio y el asunto queda
// recortado en su caja.
//
// VISTO EN ROJO con el cursor al final en `Poner` y el asunto en `NuevoEditorPlano`: el
// cursor de «Para» queda en la runa 77, con el texto corrido 328 px, y el asunto no
// lleva desplazador.
func TestLaRespuestaAbrePorElPrincipio(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(390, 844))
	para := "Daniela Fernández de la Torre <daniela.fernandez@empresa-con-nombre-largo.eu>"
	b.abrirRedactor(Borrador{
		Para:   para,
		Asunto: "Re: Presupuesto de la reforma de la cocina y el baño del piso de la calle Mayor",
	})
	r := b.redaccion.r
	if r.B.Para != para || r.para.editor.Texto() != para {
		t.Fatalf("«Para» cambia lo escrito: %q / %q", r.B.Para, r.para.editor.Texto())
	}
	for nombre, e := range map[string]*ui.EditorPlano{"para": r.para.editor, "asunto": r.asunto} {
		if e.CursorRow != 0 || e.CursorColumn != 0 || e.Desplazamiento() != 0 {
			t.Errorf("«%s» abre con el cursor en %d:%d y corrido %v", nombre, e.CursorRow, e.CursorColumn, e.Desplazamiento())
		}
		if e.Scroll != fyne.ScrollHorizontalOnly {
			t.Errorf("«%s» no recorta lo que no cabe en su fila (scroll %v)", nombre, e.Scroll)
		}
	}
	// Con el foco en el campo, poner un valor deja el cursor al final, donde se escribe.
	r.para.enfocado = true
	r.para.Poner("ana@sytue.eu")
	if r.para.editor.CursorColumn != len("ana@sytue.eu") {
		t.Errorf("con el foco el cursor queda en %d", r.para.editor.CursorColumn)
	}
}

// TestLaCasillaSeApartaDelAvatarConElDedo: en el teléfono la casilla se ve siempre, y a
// 4 px del punto y del avatar se leían como una sola mancha; un dedo que se quedaba corto
// abría el correo. Con el dedo la casilla va a `gap-3` (el hueco de la fila de Contacts)
// y recoge el toque en ese hueco y en todo el alto; con ratón, el `gap-1` de la web.
//
// VISTO EN ROJO con el hueco fijo de antes: el avatar está a 4 px de la casilla también
// con el dedo, y tocar ese hueco abre la conversación.
func TestLaCasillaSeApartaDelAvatarConElDedo(t *testing.T) {
	for _, tactil := range []bool{false, true} {
		restaurar := ui.TactilDePrueba(tactil)
		f := NuevaFilaConversacion()
		marcadas, abiertas := 0, 0
		f.AlMarcar, f.AlAbrir = func() { marcadas++ }, func() { abiertas++ }
		f.Poner(Conversacion{HiloID: "h", SinLeer: true}, "Persona m1", "Calendario", "9:30")
		w := test.NewWindow(f)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(390, AltoFilaConversacion()))
		r := test.WidgetRenderer(f).(*filaRenderer)
		hueco := r.punto.Position().X - (ui.InicioFila() + ui.LadoEleccion)
		quiere := HuecoDelantero
		if tactil {
			quiere = HuecoBloque
		}
		if hueco != quiere {
			t.Errorf("táctil=%v: de la casilla al punto hay %v y tocaba %v", tactil, hueco, quiere)
		}
		if got := AnchoTextoFila(390); got != 390-MargenFila-r.nombre.Position().X {
			t.Errorf("táctil=%v: la columna de texto mide %v y la fila deja %v", tactil, got, 390-MargenFila-r.nombre.Position().X)
		}
		// Un toque en el hueco entre la casilla y el punto, por encima del cuadrado.
		test.TapCanvas(w.Canvas(), fyne.NewPos(ui.InicioFila()+ui.LadoEleccion+quiere/2, 6))
		if tactil && (marcadas != 1 || abiertas != 0) || !tactil && (marcadas != 0 || abiertas != 1) {
			t.Errorf("táctil=%v: tocar el hueco marca %d y abre %d", tactil, marcadas, abiertas)
		}
		w.Close()
		restaurar()
	}
}
