// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El disparador es el de `refreshTrigger`: «/palabra» solo con el texto entero, «:algo»
// con dos letras o más, y «@algo» pegado al cursor aunque haya texto detrás.
//
// VISTO EN ROJO (2026-09-23, en el frontal): mirando el texto entero y no lo de antes
// del cursor, «@an y más» no dispara nada.
func TestElDisparadorEsElDeLaWeb(t *testing.T) {
	casos := []struct {
		texto  string
		cursor int
		tipo   TipoDisparador
		q      string
	}{
		{"/to", 3, DisparaComando, "to"},
		{"/", 1, DisparaComando, ""},
		{"/topic nuevo", 12, SinDisparador, ""},
		{"hola :sm", 8, DisparaEmoji, "sm"},
		{"hola :s", 7, SinDisparador, ""},
		{"hola @an y más", 8, DisparaMencion, "an"},
		{"correo@dominio", 14, SinDisparador, ""},
		{"@", 1, DisparaMencion, ""},
	}
	for _, c := range casos {
		tipo, q := Disparador(c.texto, c.cursor)
		if tipo != c.tipo || q != c.q {
			t.Errorf("Disparador(%q, %d) = %v %q; quiero %v %q", c.texto, c.cursor, tipo, q, c.tipo, c.q)
		}
	}
}

// Las sugerencias se acotan como en la web: las especiales delante y 10 en total; los
// comandos casan también por su alias, y los de servidor van detrás.
func TestLasSugerenciasSeAcotan(t *testing.T) {
	roster := make([]Miembro, 0, 10)
	for _, n := range []string{"Ana", "Anabel", "Juana", "Mariana", "Diana", "Susana", "Tatiana", "Luciana", "Adriana", "Silvana"} {
		roster = append(roster, Miembro{ID: n, Nombre: n})
	}
	if got := SugerenciasPara(DisparaMencion, "an", roster, nil); len(got) != 8 || got[0].Tipo != SugierePersona {
		t.Errorf("«@an» da %d sugerencias, la primera %v", len(got), got)
	}
	if got := SugerenciasPara(DisparaMencion, "", roster, nil); len(got) != 10 || got[0].Nombre != "channel" || got[3].Tipo != SugierePersona {
		t.Errorf("«@» da %+v", got)
	}
	remotos := []api.RoomsCommand{{Name: "desplegar"}}
	got := SugerenciasPara(DisparaComando, "d", nil, remotos)
	nombres := make([]string, 0, len(got))
	for _, s := range got {
		nombres = append(nombres, s.Nombre)
	}
	if !slices.Equal(nombres, []string{"dnd", "msg", "desplegar"}) {
		t.Errorf("«/d» da %v", nombres)
	}
	if got := SugerenciasPara(DisparaEmoji, "smi", nil, nil); len(got) == 0 || got[0].Glifo != "😄" {
		t.Errorf("«:smi» da %+v", got)
	}
}

// teclear escribe en la caja como si se pulsaran las teclas.
func teclear(c *Compositor, s string) {
	for _, r := range s {
		c.area.TypedRune(r)
	}
}

// Escribir «@an» abre la lista con las personas del canal; bajar y pulsar Intro cambia
// lo tecleado por el nombre entero, y al enviar la mención viaja.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `borrar` sin el «+1» de la arroba en
// Reemplazo, la caja queda «hola @@Beto Ruiz ».
func TestLaArrobaCompletaElNombreYLaMencionViaja(t *testing.T) {
	s := servicioConDos()
	s.roster = []api.RoomsMember{{Subject: "ana", Name: "Ana Ruiz"}, {Subject: "beto", Name: "Beto Ruiz"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	teclear(c, "hola @ruiz")
	if !c.SugerenciasAbiertas() || len(c.sug.Items) != 2 {
		t.Fatalf("«@ruiz» no abre la lista con las dos personas: %+v", c.sug)
	}
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if c.Texto() != "hola @Beto Ruiz " || c.SugerenciasAbiertas() {
		t.Fatalf("elegir deja %q con la lista abierta=%v", c.Texto(), c.SugerenciasAbiertas())
	}
	if s.pidio("Post(c1,hola @Beto Ruiz )") {
		t.Fatal("el Intro que elige también envió")
	}
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(s.envios) != 1 || !slices.Equal(s.envios[0].Mentions, []string{"beto"}) {
		t.Errorf("la mención no viaja: %+v", s.envios)
	}
	teclear(c, "fin :tada")
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyTab})
	if c.Texto() != "fin 🎉 " {
		t.Errorf("el tabulador en «:tada» deja %q", c.Texto())
	}
	teclear(c, "@a")
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if c.SugerenciasAbiertas() {
		t.Error("Escape no cierra la lista")
	}
}

// Los comandos se despachan en vez de enviarse: «/shrug» y «/me» salen transformados,
// uno que no existe avisa y no sale, «/topic» cambia el tema, uno de servidor lo ejecuta
// el servicio y «/help» abre la lista.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el caso `ComandoDesconocido` en
// Someter, «/nada» se publica como texto.
func TestLosComandosSeDespachan(t *testing.T) {
	s := servicioConDos()
	s.comandos = []api.RoomsCommand{{Name: "desplegar", Description: "Despliega"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	enviar := func(txt string) {
		c.PonerTexto(txt)
		c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	}
	enviar("/shrug vale")
	if !s.pidio(`Post(c1,vale ¯\_(ツ)_/¯)`) {
		t.Errorf("/shrug: %v", s.llamadas)
	}
	enviar("/me saluda")
	if !s.pidio("Post(c1,/me saluda)") {
		t.Errorf("/me: %v", s.llamadas)
	}
	enviar("/nada")
	if c.aviso == "" || c.Texto() != "/nada" || s.pidio("Post(c1,/nada)") {
		t.Errorf("/nada avisa %q y deja %q: %v", c.aviso, c.Texto(), s.llamadas)
	}
	enviar("/topic Nuevo tema")
	if !s.pidio("UpdateChannel(c1,tema=Nuevo tema)") || sa.P.Canal.Cabecera.Canal.Tema != "Nuevo tema" || c.aviso != "" {
		t.Errorf("/topic: %v, cabecera %q", s.llamadas, sa.P.Canal.Cabecera.Canal.Tema)
	}
	enviar("/desplegar prod")
	if !s.pidio("ExecuteCommand(c1,/desplegar prod)") {
		t.Errorf("comando de servidor: %v", s.llamadas)
	}
	// Con «/help» a secas el Intro ELIGE la sugerencia (la lista está abierta) y deja
	// «/help »: el segundo Intro es el que lo ejecuta, como en la web.
	enviar("/help")
	if c.Texto() != "/help " {
		t.Errorf("Intro con la lista abierta deja %q", c.Texto())
	}
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if c.Texto() != "/" || !c.SugerenciasAbiertas() || len(c.sug.Items) != len(Comandos)+1 {
		t.Errorf("/help deja %q con %d sugerencias", c.Texto(), len(c.sug.Items))
	}
	// El compositor del hilo no despacha comandos con efecto: avisa.
	c.AlComando = nil
	enviar("/away ")
	if c.aviso != sa.P.T.T("command.unavailable", i18n.Vars{"name": "away"}) {
		t.Errorf("sin despachador /away avisa %q", c.aviso)
	}
}

// Envolver es `aplicarFormato` sobre la selección.
func TestEnvolverEsAplicarFormato(t *testing.T) {
	casos := []struct {
		sel     string
		m       MarcaFormato
		teclear string
		atras   int
		linea   bool
	}{
		{"hola", MarcaNegrita, "**hola**", 0, false},
		{"**hola**", MarcaNegrita, "hola", 0, false},
		{"", MarcaNegrita, "****", 2, false},
		{"", MarcaCodigo, "``", 1, false},
		{"web", MarcaEnlace, "[web]()", 1, false},
		{"", MarcaEnlace, "[]()", 3, false},
		{"a\nb", MarcaLista, "- a\n- b", 0, false},
		{"> a\n> b", MarcaCita, "a\nb", 0, false},
		{"", MarcaCita, "> ", 0, true},
	}
	for _, c := range casos {
		te, at, li := Envolver(c.sel, c.m)
		if te != c.teclear || at != c.atras || li != c.linea {
			t.Errorf("Envolver(%q, %v) = %q, %d, %v; quiero %q, %d, %v", c.sel, c.m, te, at, li, c.teclear, c.atras, c.linea)
		}
	}
}

// «Aa» despliega los siete botones y los atajos valen siempre: ⌘B sobre la selección la
// pone en negrita, y sin selección deja el cursor entre las marcas.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `atras` del cursor, «x» se escribe
// detrás de las cuatro marcas: «****x».
func TestLaBarraDeFormatoYSusAtajos(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	antes := len(c.botones)
	c.AlternarFormato()
	if len(c.botones) != antes+len(marcasDeBarra) || !c.FormatoVisible {
		t.Fatalf("«Aa» deja %d botones de %d", len(c.botones), antes)
	}
	teclear(c, "hola")
	c.area.TypedShortcut(&fyne.ShortcutSelectAll{})
	c.area.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyB, Modifier: fyne.KeyModifierSuper})
	if c.Texto() != "**hola**" {
		t.Fatalf("⌘B sobre «hola» deja %q", c.Texto())
	}
	c.PonerTexto("")
	c.AplicarMarca(MarcaNegrita)
	teclear(c, "x")
	if c.Texto() != "**x**" {
		t.Errorf("negrita sin selección y «x» deja %q", c.Texto())
	}
	if _, ok := MarcaDeAtajo(&desktop.CustomShortcut{KeyName: fyne.KeyB, Modifier: fyne.KeyModifierControl | fyne.KeyModifierAlt}); ok {
		t.Error("Ctrl+Alt+B no es un atajo de formato")
	}
	if m, ok := MarcaDeAtajo(&desktop.CustomShortcut{KeyName: fyne.KeyX, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}); !ok || m != MarcaTachado {
		t.Error("Ctrl+Mayús+X no es tachado")
	}
}

// «/me saluda» de Ana se pinta «* Ana saluda», todo en cursiva y el nombre en la tinta
// del texto.
func TestElEmoteLlevaElAutor(t *testing.T) {
	m := maquetadorDe("/me saluda", "Ana", nil, 600)
	textos := make([]string, 0, len(m.piezas))
	for _, p := range m.piezas {
		textos = append(textos, p.texto)
	}
	if !slices.Equal(textos, []string{"*", " ", "Ana", " ", "saluda"}) || m.piezas[2].tinta != "text" || m.piezas[4].estilo != estCursiva {
		t.Errorf("el emote se maqueta %q, %+v", textos, m.piezas)
	}
}

// @here en un canal de ocho o más pide confirmación antes de salir.
func TestAvisarATodosPideConfirmacion(t *testing.T) {
	s := servicioConDos()
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		s.roster = append(s.roster, api.RoomsMember{Subject: n, Name: n})
	}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	sa.Enviar("@here reunión")
	if s.pidio("Post(c1,@here reunión)") || sa.pregunta == nil {
		t.Fatalf("@here sale sin preguntar: %v", s.llamadas)
	}
	sa.pregunta.Cerrar()
	if !MencionaATodos("hola @all") || MencionaATodos("correo@here.com") || MencionaATodos("@@here") {
		t.Error("MencionaATodos no es el patrón del servicio")
	}
}
