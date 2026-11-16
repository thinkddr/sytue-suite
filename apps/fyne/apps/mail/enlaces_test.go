// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// TestUnEnlacePartidoEnDosLineasAbreEntero: una dirección más ancha que la columna se
// parte por caracteres (`partirLarga`), y los dos trozos tienen que abrir la MISMA
// dirección entera: abrir media sería abrir otro sitio.
func TestUnEnlacePartidoEnDosLineasAbreEntero(t *testing.T) {
	test.NewTempApp(t)
	url := "https://sytue.eu/" + strings.Repeat("camino/", 12) + "fin"
	texto := "Mira " + url + " y ya.\n\nFirma"
	lineas := repartirPre(texto, tema.TextoBase, 0, 160)
	tramos := tramosDeLineas(texto, lineas, true)
	var trozos, juntos []string
	for _, tr := range tramos {
		for _, x := range tr {
			if x.Destino != "" {
				trozos = append(trozos, x.Texto)
				if x.Destino != url {
					t.Errorf("el trozo %q abre %q y no la dirección entera", x.Texto, x.Destino)
				}
			}
			juntos = append(juntos, x.Texto)
		}
	}
	if len(trozos) < 2 || strings.Join(trozos, "") != url {
		t.Errorf("la dirección no queda repartida en sus líneas: %q", trozos)
	}
	if strings.Join(juntos, "") != strings.Join(lineas, "") {
		t.Error("los tramos no rehacen las líneas")
	}
	// Un cuerpo cortado por `recortar` justo en una dirección no la enlaza a medias.
	largo := strings.Repeat("a", maxCuerpo-12) + " https://sytue.eu/" + strings.Repeat("x", 40)
	cortado := recortar(largo)
	for _, tr := range tramosDeLineas(cortado, []string{cortado}, true)[0] {
		if tr.Destino != "" {
			t.Errorf("la dirección cortada se enlaza: %q", tr.Destino)
		}
	}
	// Sin enlazar (el extracto), cada línea es un solo tramo de texto.
	for i, tr := range tramosDeLineas(texto, lineas, false) {
		if len(tr) != 1 || tr[0].Destino != "" || tr[0].Texto != lineas[i] {
			t.Errorf("sin enlazar, la línea %d sale %+v", i, tr)
		}
	}
}

// TestPulsarUnEnlaceDelCuerpoLoAbre: en la tarjeta, el enlace se pinta en
// `primary-text` subrayado y pulsar ENCIMA lo abre; pulsar en el texto de al lado no
// hace nada, y el extracto (cortado) no se enlaza.
func TestPulsarUnEnlaceDelCuerpoLoAbre(t *testing.T) {
	tr := traductor(t)
	l := NuevoLector(tr, "es")
	var abiertos []string
	l.AlAbrirEnlace = func(d string) { abiertos = append(abiertos, d) }
	l.Cuerpo = func(string) (Cuerpo, bool) {
		return Cuerpo{Texto: "Hola, mira www.sytue.eu/precios o escribe a ana@sytue.eu."}, true
	}
	tj := nuevaTarjetaMensaje(l, MensajeDe(api.MailMessage{ID: "m1", From: "ana@ejemplo.eu"}), true)
	tj.ancho = 600
	w := test.NewWindow(tj)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(600, 400))
	r := test.WidgetRenderer(tj).(*tarjetaRenderer)

	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var enlaces []string
	for _, p := range r.parrafo {
		if p.TextStyle.Underline {
			enlaces = append(enlaces, p.Text)
			if p.Color != tema.Color("primary-text", v) {
				t.Errorf("el enlace %q no va en primary-text", p.Text)
			}
		}
	}
	if strings.Join(enlaces, "|") != "www.sytue.eu/precios|ana@sytue.eu" {
		t.Fatalf("enlaces pintados: %q", enlaces)
	}
	if len(tj.zonas) != 2 {
		t.Fatalf("dianas: %+v", tj.zonas)
	}
	for _, z := range tj.zonas {
		centro := fyne.NewPos(z.pos.X+z.tam.X/2, z.pos.Y+z.tam.Y/2)
		fyne.DoAndWait(func() { tj.Tapped(&fyne.PointEvent{Position: centro}) })
	}
	if strings.Join(abiertos, " ") != "https://www.sytue.eu/precios mailto:ana@sytue.eu" {
		t.Errorf("abiertos: %q", abiertos)
	}
	// El «Hola» del principio no es un enlace: pulsarlo no abre nada ni pliega.
	z := tj.zonas[0]
	fyne.DoAndWait(func() { tj.Tapped(&fyne.PointEvent{Position: fyne.NewPos(z.pos.X-40, z.pos.Y+2)}) })
	if len(abiertos) != 2 || !tj.Expandida {
		t.Errorf("pulsar fuera del enlace: abiertos %q, desplegada %v", abiertos, tj.Expandida)
	}
	// El puntero sobre el enlace es la mano.
	tj.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(z.pos.X+1, z.pos.Y+1)}})
	if tj.Cursor() != desktop.PointerCursor {
		t.Error("sobre un enlace el cursor no es la mano")
	}

	// Mientras llega el cuerpo se ve el extracto, que no se enlaza.
	l.Cuerpo = func(string) (Cuerpo, bool) { return Cuerpo{}, false }
	tj.Mensaje.Extracto = "Mira https://sytue.eu/cortad"
	fyne.DoAndWait(tj.Refresh)
	if len(tj.zonas) != 0 {
		t.Errorf("el extracto se enlaza: %+v", tj.zonas)
	}
}

// TestUnCorreoSoloHTMLConservaSusDestinos: sin parte de texto, el cuerpo es el HTML
// pasado a texto, y el destino de cada enlace queda escrito junto a su texto para que
// se vea —y se pueda pulsar— el dominio REAL.
func TestUnCorreoSoloHTMLConservaSusDestinos(t *testing.T) {
	c := CuerpoDe(eml.Mensaje{HTML: `<p>Hola.</p><p><a href="https://id.ejemplo.example/r?t=1">Restablece tu contraseña</a></p>`}, nil)
	if c.Texto != "Hola.\nRestablece tu contraseña <https://id.ejemplo.example/r?t=1>" || !c.ConFormato {
		t.Errorf("cuerpo %q", c.Texto)
	}
	c = CuerpoExterno(api.ExternalMailMessage{HTML: `<a href="mailto:ana@sytue.eu">Ana</a>`}, nil)
	if c.Texto != "Ana <ana@sytue.eu>" {
		t.Errorf("cuerpo externo %q", c.Texto)
	}
}

// TestAbrirEnlaceSoloDejaPasarLaWeb: una web va al navegador por `api.AbrirFuera`;
// cualquier otro esquema se queda sin abrir aunque llegara aquí.
func TestAbrirEnlaceSoloDejaPasarLaWeb(t *testing.T) {
	s := servicioConTodas()
	b := buzonConAvisos(t, s)
	a := &abridorFalso{}
	b.Abridor = a
	b.abrirEnlace("https://sytue.eu/precios")
	b.abrirEnlace("javascript:alert(1)")
	b.abrirEnlace("file:///etc/passwd")
	if strings.Join(a.abiertas, " ") != "https://sytue.eu/precios" {
		t.Errorf("abiertas: %q", a.abiertas)
	}
	if b.P.Lector.AlAbrirEnlace == nil {
		t.Error("el lector no tiene con qué abrir los enlaces")
	}
}

// TestUnMailtoDelCuerpoAbreElRedactor: se escribe desde Sytue, con la dirección en
// «Para», y no desde el programa de correo del sistema.
func TestUnMailtoDelCuerpoAbreElRedactor(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	a := &abridorFalso{}
	b.Abridor = a
	b.abrirEnlace("mailto:ana@sytue.eu")
	dialogoAbierto(t, w.Canvas())
	if b.redaccion == nil || b.redaccion.r.B.Para != "ana@sytue.eu" || len(a.abiertas) != 0 {
		t.Errorf("redactor %v, abiertas %q", b.redaccion != nil, a.abiertas)
	}
}
