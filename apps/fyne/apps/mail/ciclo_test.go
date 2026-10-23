// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestMailPausaListasSinCancelarCorreoNiBorrador(t *testing.T) {
	s := servicioDeEjemplo()
	b, pendientes := buzonDePrueba(t, s)
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	b.Arrancar(ctx)
	b.redaccion = &redaccion{r: &Redactor{B: Borrador{Asunto: "sin terminar", Cuerpo: "texto propio"}}}
	b.hiloID = "h1"
	antes := len(*pendientes)
	reloj := (*pendientes)[antes-1]
	b.EnSegundoPlano()
	for range 6 {
		reloj()     // Un temporizador ya encolado también debe quedarse quieto.
		b.llegada() // El flujo sigue recibiendo; el aviso no necesita leer la lista.
	}
	if s.pidio("Changes") != 0 || len(*pendientes) != antes || ctx.Err() != nil {
		t.Fatalf("el fondo hizo lecturas o canceló el correo: %v, timers %d", s.llamadas, len(*pendientes))
	}
	b.EnPrimerPlano()
	b.EnPrimerPlano() // Dos avisos del SO no crean otra cadena.
	if s.pidio("Changes") != 1 || b.hiloID != "h1" || b.redaccion.r.B.Cuerpo != "texto propio" {
		t.Fatalf("volver no agrupó la actualización o perdió lo abierto: %v", s.llamadas)
	}
	(*pendientes)[len(*pendientes)-1]()
	if s.pidio("Changes") != 2 {
		t.Fatal("el sondeo de respaldo no se reanudó")
	}
	t.Log("6 intervalos + 6 llegadas en segundo plano: 0 lecturas; al volver: 1 lectura")
}

func TestMailReconciliarNoDuplicaUnDeltaEnCamino(t *testing.T) {
	s := servicioDeEjemplo()
	b, _ := buzonDePrueba(t, s)
	b.Arrancar(context.Background())
	var trabajo []func()
	b.Lanzar = func(f func()) { trabajo = append(trabajo, f) }
	b.llegada()
	b.EnSegundoPlano()
	b.EnPrimerPlano()
	if len(trabajo) != 1 || !b.otraVuelta {
		t.Fatal("volver lanzó un segundo delta concurrente")
	}
	trabajo[0]()
	if len(trabajo) != 2 || b.otraVuelta {
		t.Fatal("el delta terminado no dejó una única reconciliación")
	}
	trabajo[1]()
	if len(trabajo) != 2 || b.deltaEnCurso {
		t.Fatal("la reconciliación creó otra cadena")
	}
}

func TestCorreoExternoPausaLaListaYConservaElRedactor(t *testing.T) {
	s := &correoFalso{}
	c := correoDePrueba(t, s)
	c.b.correoAbierto = c
	var relojes []func()
	c.b.Esperar = func(_ time.Duration, f func()) func() {
		relojes = append(relojes, f)
		return func() {}
	}
	c.programar()
	reloj := relojes[0]
	c.b.EnSegundoPlano()
	for range 2 {
		reloj()
	}
	if s.reads != 0 || len(relojes) != 1 || c.ctx.Err() != nil {
		t.Fatal("el fondo siguió consultando o canceló operaciones del correo externo")
	}
	c.redactando = true
	c.texto.PonerTexto("borrador externo")
	c.b.EnPrimerPlano()
	if s.reads != 0 || c.texto.Texto() != "borrador externo" || !c.redactando {
		t.Fatal("volver interrumpió la redacción")
	}
	c.redactando = false
	relojes[len(relojes)-1]()
	if s.reads != 1 {
		t.Fatal("la lista externa no se reanudó tras terminar de redactar")
	}
}

func TestFondoNoCancelaEnvioExternoEnCamino(t *testing.T) {
	s := &correoFalso{sent: api.ExternalMailSent{}}
	c := correoDePrueba(t, s)
	c.b.correoAbierto = c
	c.redactando = true
	c.campos["to"].PonerTexto("to@example.eu")
	c.texto.PonerTexto("mensaje sintético")
	var trabajo func()
	c.b.Lanzar = func(f func()) { trabajo = f }
	c.enviar()
	c.b.EnSegundoPlano()
	trabajo()
	if s.sends != 1 || c.hayBorrador() || c.ctx.Err() != nil {
		t.Fatal("ir al fondo impidió el envío iniciado")
	}
}
