// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLasFrasesDeLaReglaSonLasDeLaWeb: condición, acción y la frase entera, con la
// enumeración del idioma.
func TestLasFrasesDeLaReglaSonLasDeLaWeb(t *testing.T) {
	tr := traductor(t)
	carpeta := func(string) string { return "Archivo" }
	if got := CondicionRegla(tr, "es", "newsletter@", "factura"); got != "Remitente contiene «newsletter@» y Asunto contiene «factura»" {
		t.Errorf("condición: %q", got)
	}
	if got := CondicionRegla(tr, "es", "", ""); got != tr.T("rules.conditionAny", nil) {
		t.Errorf("sin condición: %q", got)
	}
	if got := AccionRegla(tr, "es", carpeta, "ar", true, true); got != "Mover a Archivo, Marcar como leído y Destacar" {
		t.Errorf("acción: %q", got)
	}
	d := BorradorRegla{De: "banco@", MoverA: "ar", Leido: true}
	if got := FraseRegla(tr, "es", carpeta, d); got != "A los correos de «banco@»: voy a moverlos a Archivo y marcarlos como leídos." {
		t.Errorf("frase: %q", got)
	}
	if got := FraseRegla(tr, "es", carpeta, BorradorRegla{Asunto: "x"}); got != tr.T("rules.sentenceNothing", map[string]any{"who": tr.T("rules.whoSubject", map[string]any{"subject": "x"})}) {
		t.Errorf("sin acción: %q", got)
	}
}

// TestLasReglasSeCreanConElAsistente: sin condición no se sigue; «otra carpeta» sin
// elegir tampoco; al crear, la regla va activa con lo elegido y se vuelve a la lista.
func TestLasReglasSeCreanConElAsistente(t *testing.T) {
	s := servicioConTodas()
	s.reglas = []api.MailFilter{{ID: "r1", Name: "Boletines", Enabled: true, MatchFrom: "newsletter@", MoveTo: "ar"}}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	g := b.verReglas(nil)
	if !hayDialogo(w.Canvas(), tr.T("rules.title", nil)) || len(g.lista) != 1 {
		t.Fatalf("la lista: %v %+v", g.estado, g.lista)
	}
	g.empezar(BorradorRegla{})
	if !hayDialogo(w.Canvas(), tr.T("rules.newTitle", nil)) || hayDialogo(w.Canvas(), tr.T("rules.title", nil)) {
		t.Fatal("el asistente no sustituye a la lista")
	}
	if g.puedeSeguir() {
		t.Fatal("sin condición se deja seguir")
	}
	g.de.PonerTexto("banco@")
	g.irA(1)
	if g.puedeSeguir() {
		t.Fatal("sin acción se deja seguir")
	}
	g.borrador.Leido = true
	if !g.puedeSeguir() {
		t.Fatal("con «marcar como leído» no se deja seguir")
	}
	g.otra = true
	if g.puedeSeguir() {
		t.Fatal("con «otra carpeta» sin elegir se deja seguir")
	}
	g.borrador.MoverA = "pr"
	if !g.puedeSeguir() {
		t.Fatal("con destino y acción no se deja seguir")
	}
	g.irA(2)
	g.crear()
	if !slices.Contains(s.llamadas, "CreateFilter(,banco@,,pr,true,false)") {
		t.Fatalf("crear: %v", s.llamadas)
	}
	if g.paso != -1 || len(g.lista) != 2 || !hayDialogo(w.Canvas(), tr.T("rules.title", nil)) {
		t.Errorf("tras crear: paso %d, %d reglas", g.paso, len(g.lista))
	}
}

// TestLasReglasSeActivanYSeBorranConVuelta: el interruptor manda la regla entera y
// vuelve si el servidor dice que no; borrar también.
func TestLasReglasSeActivanYSeBorranConVuelta(t *testing.T) {
	s := servicioConTodas()
	s.reglas = []api.MailFilter{{ID: "r1", Enabled: true, MatchFrom: "a@"}}
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	g := b.verReglas(nil)
	g.alternar(g.lista[0])
	if g.lista[0].Enabled || !slices.Contains(s.llamadas, "UpdateFilter(r1,false)") {
		t.Fatalf("desactivar: %+v %v", g.lista[0], s.llamadas)
	}
	s.errTriaje = api.ErrMailTooLarge
	g.alternar(g.lista[0])
	if g.lista[0].Enabled {
		t.Error("el cambio rechazado se ha quedado puesto")
	}
	g.borrar(g.lista[0])
	if len(g.lista) != 1 {
		t.Error("el borrado rechazado ha quitado la regla")
	}
	if _, titulo := ultimoAviso(b); titulo != traductor(t).T("rules.deleteFailed", nil) {
		t.Errorf("aviso: %q", titulo)
	}
}

// TestCrearReglaDesdeUnCorreoAbreElAsistente: con el remitente como condición y nombre.
func TestCrearReglaDesdeUnCorreoAbreElAsistente(t *testing.T) {
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	c := b.P.Bandeja.Conversaciones[0]
	if b.P.Acciones.CrearRegla == nil {
		t.Fatal("«Crear regla» no se ofrece")
	}
	g := b.crearReglaDesde(c)
	de := c.Ultimo.De[0].Correo
	if !hayDialogo(w.Canvas(), traductor(t).T("rules.newTitle", nil)) || g.paso != 0 {
		t.Fatal("no se abre el asistente")
	}
	if g.de.Texto() != de || g.borrador.De != de || g.borrador.Nombre != de || !g.puedeSeguir() {
		t.Errorf("el remitente no está puesto: campo %q, borrador %+v", g.de.Texto(), g.borrador)
	}
}

// TestEnElMovilElAsistenteNoEnsanchaElDialogo: a 360 la botonera se sale por la
// derecha, como en la web, en vez de ensanchar el diálogo entero; y la fila de una
// regla recorta por letras.
func TestEnElMovilElAsistenteNoEnsanchaElDialogo(t *testing.T) {
	s := servicioConTodas()
	s.reglas = []api.MailFilter{{ID: "r1", Enabled: true, MatchFrom: "newsletter@example.org"}}
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(360, 740))
	g := b.verReglas(nil)
	if w := g.caja.MinSize().Width; w > g.ancho {
		t.Errorf("la lista pide %g y el cuerpo mide %g", w, g.ancho)
	}
	g.empezar(BorradorRegla{De: "newsletter@", MoverA: "ar"})
	g.irA(2)
	if w := g.caja.MinSize().Width; w > g.ancho {
		t.Errorf("el resumen pide %g y el cuerpo mide %g: la botonera ensancha el diálogo", w, g.ancho)
	}
}

// TestLaCapturaDeReglasSeGuarda (SYTUE_CAPTURAS): la lista y los tres pasos, en
// escritorio, y la lista y el primer paso en el móvil.
func TestLaCapturaDeReglasSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
		pre := "reglas"
		if tam.Width < 400 {
			pre = "movil-reglas"
		}
		s := servicioConTodas()
		s.reglas = []api.MailFilter{
			{ID: "r1", Name: "Boletines", Enabled: true, MatchFrom: "newsletter@", MoveTo: "ar", MarkRead: true},
			{ID: "r2", Enabled: false, MatchSubject: "factura", MarkFlagged: true},
		}
		b, w, _ := buzonConVentana(t, s, tam)
		g := b.verReglas(nil)
		guardar(t, filepath.Join(dir, pre+".png"), w.Canvas().Capture())
		g.empezar(BorradorRegla{})
		g.de.PonerTexto("newsletter@")
		guardar(t, filepath.Join(dir, pre+"-paso1.png"), w.Canvas().Capture())
		g.irA(1)
		g.borrador.MoverA = "tr"
		g.pintar()
		guardar(t, filepath.Join(dir, pre+"-paso2.png"), w.Canvas().Capture())
		g.borrador.MoverA = "ar"
		g.irA(2)
		guardar(t, filepath.Join(dir, pre+"-paso3.png"), w.Canvas().Capture())
	}
}

// TestElResumenLlevaEditar: las dos primeras filas del resumen llevan «Editar» (del
// catálogo común, no escrito a mano) y vuelven a su paso; la de «Cuándo» no lleva.
// Medido en la web a 1280: con «Editar» la fila mide 37,75 con su filete (36,75 sin
// él), el texto baja 3 px y la última, sin filete, 33,25.
func TestElResumenLlevaEditar(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	g := b.verReglas(nil)
	g.empezar(BorradorRegla{De: "banco@", Leido: true})
	g.irA(2)
	var editar []*ui.Enlace
	var filas []*fyne.Container
	var mirar func(fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.Enlace); ok && e.Texto == tr.T("ui.review.edit", nil) {
			editar = append(editar, e)
		}
		if c, ok := o.(*fyne.Container); ok {
			if _, es := c.Layout.(filaDeResumen); es {
				filas = append(filas, c)
			}
		}
		for _, h := range hijos(o) {
			mirar(h)
		}
	}
	mirar(g.caja)
	if len(editar) != 2 || len(filas) != 3 {
		t.Fatalf("«Editar» en %d filas de %d; quiero 2 de 3", len(editar), len(filas))
	}
	if editar[0].Texto != "Editar" || editar[0].Nombre != "Editar "+tr.T("rules.summaryWhich", nil) {
		t.Errorf("«Editar»: %q, nombre accesible %q", editar[0].Texto, editar[0].Nombre)
	}
	for i, quiero := range []float32{42, 42, 38} {
		if alto := filas[i].MinSize().Height + 2*margenYResumen; alto != quiero {
			t.Errorf("fila %d: %g de alto y en la web %g", i, alto, quiero)
		}
	}
	if y := editar[0].Position().Y; y != bajoEditar {
		t.Errorf("«Editar» en y=%g y el texto en la web a 3", y)
	}
	editar[1].AlPulsar()
	if g.paso != 1 {
		t.Errorf("«Editar» de «Qué hago» lleva al paso %d", g.paso)
	}
}
