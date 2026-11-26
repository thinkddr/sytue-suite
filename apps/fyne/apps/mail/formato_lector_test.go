// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"image"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestElLectorConFormatoBloqueaLasRemotas: con formato, el HTML saneado se pinta con
// la prosa de Notepads; las imágenes remotas no se piden hasta que la persona las deja
// ver, y entonces la tarjeta se rehace. Sin ConFormato, la misma tarjeta es de texto.
func TestElLectorConFormatoBloqueaLasRemotas(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	tr := traductor(t)
	l := NuevoLector(tr, "es")
	m := MensajeDe(api.MailMessage{ID: "m1", From: "boletin@ejemplo.eu", To: []string{"yo@ejemplo.eu"}})
	html := SanearHTML(`<p>Hola <b>Ana</b></p><img src="https://img.ejemplo.eu/a.png"><script>x()</script>`)
	l.Cuerpo = func(string) (Cuerpo, bool) { return Cuerpo{Texto: "Hola Ana", HTML: html, ConFormato: true}, true }

	sin := nuevaTarjetaMensaje(l, m, true)
	if sin.conFormato() != nil {
		t.Fatal("sin ConFormato la tarjeta debe seguir en texto")
	}

	var pedidas []string
	visibles := false
	l.Imagen = func(_ Mensaje, src string, remotas bool) image.Image {
		if remotas {
			pedidas = append(pedidas, src)
		}
		return nil
	}
	l.ImagenesVisibles = func(Mensaje) bool { return visibles }
	l.AlVerImagenes = func(Mensaje, bool) { visibles = true; l.VersionImagenes++ }
	c := nuevaTarjetaMensaje(l, m, true)
	f := c.conFormato()
	if f == nil || f.prosa == nil || !f.remotas || strings.Contains(html, "script") {
		t.Fatalf("formato: %+v · %s", f, html)
	}
	aviso, _, _, _, alto := c.medidaCuerpo(600)
	if len(aviso) == 0 || alto <= 0 || !strings.Contains(strings.Join(aviso, " "), "imágenes remotas") {
		t.Fatalf("aviso de imágenes bloqueadas: %q %v", aviso, alto)
	}
	f.prosa.AltoPara(500)
	if len(pedidas) != 0 {
		t.Fatalf("se pidieron remotas sin permiso: %v", pedidas)
	}
	f.ver.AlPulsar()
	f2 := c.conFormato()
	if f2 == f || f2.remotas {
		t.Fatal("tras «Mostrar imágenes» la tarjeta se rehace sin el aviso")
	}
	f2.prosa.AltoPara(500)
	if !slices.Contains(pedidas, "https://img.ejemplo.eu/a.png") {
		t.Fatalf("la imagen remota no se pidió: %v", pedidas)
	}
	if got := RemotasDe(html); len(got) != 1 {
		t.Fatalf("RemotasDe: %v", got)
	}
}

// TestLaTarjetaDeInvitacion: con invitación, encima del cuerpo va su resumen y
// Sí/Quizá/No; contestar llama con el PARTSTAT.
func TestLaTarjetaDeInvitacion(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	tr := traductor(t)
	l := NuevoLector(tr, "es")
	m := MensajeDe(api.MailMessage{ID: "m1", From: "luis@ejemplo.eu"})
	l.Cuerpo = func(string) (Cuerpo, bool) { return Cuerpo{Texto: "Te invito."}, true }
	inv := &ResumenInvitacion{Resumen: "Revisión", Organizador: "luis@ejemplo.eu", Contestable: true}
	var dicho string
	l.Invitacion = func(Mensaje) (*ResumenInvitacion, string, bool, string) { return inv, dicho, false, "" }
	l.AlContestar = func(_ Mensaje, r string) { dicho = r }
	c := nuevaTarjetaMensaje(l, m, true)
	f := c.conFormato()
	if f == nil || f.invitacion == nil || f.prosa != nil {
		t.Fatalf("invitación: %+v", f)
	}
	_, lineas, _, _, alto := c.medidaCuerpo(600)
	if len(lineas) != 1 || alto < f.invitacion.MinSize().Height {
		t.Fatalf("el texto va debajo de la invitación: %v %v", lineas, alto)
	}
	l.AlContestar(m, "ACCEPTED")
	if dicho != "ACCEPTED" || c.conFormato() == f {
		t.Fatalf("contestar: %q", dicho)
	}
}
