// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Ponme al día» manda a Sytue AI lo mismo que el BFF de la web —la instrucción, «autor:
// texto» en orden y el modo interactivo sin razonar— y enseña la respuesta; sin
// asistente, dice que no está configurado.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con el contexto cortado por el PRINCIPIO
// (`s[:TopeResumen]`) el caso largo pierde el último mensaje.
func TestPonmeAlDia(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	var pedida api.PeticionIA
	sa.IA = func(_ context.Context, q api.PeticionIA) (string, error) {
		pedida = q
		return "- **Ana** saluda\n- Beto dice vale", nil
	}
	sa.Arrancar(context.Background())
	sa.PonmeAlDia()
	if pedida.Accion != "resumir" || pedida.Modo != "interactivo" || pedida.Esfuerzo != "none" || pedida.Instruccion != InstruccionResumen ||
		!strings.HasPrefix(pedida.Contexto, "Ana: hola\n") || pedida.Idioma != "es" {
		t.Errorf("se pidió %+v", pedida)
	}
	if sa.resumen != "- **Ana** saluda\n- Beto dice vale" || sa.P.Canal.Cabecera.Resumiendo {
		t.Errorf("se enseña %q, resumiendo=%v", sa.resumen, sa.P.Canal.Cabecera.Resumiendo)
	}
	// Las viñetas se pintan desde los bloques: «Ana» en negrita y sin los asteriscos.
	x := NuevoTextoAsistente(sa.resumen)
	test.WidgetRenderer(x).Layout(fyne.NewSize(400, 100))
	var textos []string
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *canvas.Text:
			textos = append(textos, x.Text)
			// `<strong>` es `bolder` sobre 400: la negrita de verdad, 700.
			if x.Text == "Ana" && x.FontSource != tema.Negra {
				t.Error("«Ana» no va en negrita")
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				mirar(h)
			}
		}
	}
	for _, o := range x.objs {
		mirar(o)
	}
	if got := strings.Join(textos, " "); got != "• Ana saluda • Beto dice vale" {
		t.Errorf("el resumen se pinta %q", got)
	}
	sa.pregunta.Cerrar()

	sa.IA = nil
	sa.PonmeAlDia()
	if sa.resumen != sa.P.T.T("channel.summaryOff", nil) {
		t.Errorf("sin asistente se enseña %q", sa.resumen)
	}
	sa.pregunta.Cerrar()

	largo := make([]api.RoomsMessage, 0, 300)
	for range 300 {
		largo = append(largo, api.RoomsMessage{AuthorName: "Ana", Body: strings.Repeat("x", 60)})
	}
	largo = append(largo, api.RoomsMessage{AuthorName: "Beto", Body: "el último"})
	if c := ContextoResumen(largo); len([]rune(c)) != TopeResumen || !strings.HasSuffix(c, "Beto: el último") {
		t.Errorf("el contexto largo mide %d y acaba en %q", len([]rune(c)), c[len(c)-20:])
	}
}

// Quien administra el equipo edita el tema en la banda: el botón lo abre, Intro guarda
// (y el servicio recibe el tema nuevo) y Escape lo cierra sin tocar nada. Quien no
// administra ve el tema como texto.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `PuedeEditarTema` en `abrir`, el botón
// no sale y el menú no ofrece «Editar el tema».
func TestEditarElTemaEnLaBanda(t *testing.T) {
	s := servicioConDos()
	s.rolEquipo = "admin"
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	cab := sa.P.Canal.Cabecera
	if !cab.PuedeEditarTema || cab.botonTema == nil {
		t.Fatal("un admin no tiene el botón del tema")
	}
	test.Tap(cab.botonTema)
	if !cab.EditandoTema || cab.editorTema == nil || cab.editorTema.Texto() != "Guardias" {
		t.Fatalf("el botón no abre el campo con el tema: %v", cab.EditandoTema)
	}
	cab.editorTema.PonerTexto("  Guardias de octubre ")
	cab.editorTema.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if cab.EditandoTema || !s.pidio("UpdateChannel(c1,tema=Guardias de octubre)") || cab.Canal.Tema != "Guardias de octubre" {
		t.Errorf("Intro deja editando=%v, tema %q: %v", cab.EditandoTema, cab.Canal.Tema, s.llamadas)
	}
	sa.EditarTema()
	cab.editorTema.PonerTexto("otro")
	cab.editorTema.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if cab.EditandoTema || s.veces("UpdateChannel(c1,tema=otro)") != 0 {
		t.Errorf("Escape guardó o no cerró: %v", s.llamadas)
	}

	s2 := servicioConDos()
	sa2, _ := salaDePrueba(t, s2)
	sa2.Arrancar(context.Background())
	if sa2.P.Canal.Cabecera.PuedeEditarTema || sa2.P.Canal.Cabecera.botonTema != nil {
		t.Error("quien no administra puede editar el tema")
	}
}
