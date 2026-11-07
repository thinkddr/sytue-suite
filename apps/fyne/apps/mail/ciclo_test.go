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

// La carpeta externa a la vista se vuelve a pedir cada 30 s (el proveedor no avisa),
// pero no en segundo plano ni con algo abierto; al volver, sigue. VISTO EN ROJO
// (2026-11-06) sin mirar `segundoPlano` en el reloj: el que ya estaba armado leía el
// proveedor con la app en el fondo.
func TestLaCarpetaExternaSePausaEnSegundoPlano(t *testing.T) {
	s := &correoFalso{cuentas: cuentaGmailNueva()}
	b := correoDePrueba(t, s).b
	b.ctx = context.Background()
	var relojes []func()
	b.Esperar = func(_ time.Duration, f func()) func() {
		relojes = append(relojes, f)
		return func() {}
	}
	b.cargarExternas()
	b.ElegirExterna("nueva", "INBOX")
	if s.lecturas() != 1 || len(relojes) != 1 {
		t.Fatalf("abrir la carpeta: %d lecturas, %d relojes", s.lecturas(), len(relojes))
	}
	b.EnSegundoPlano()
	relojes[0]()
	if s.lecturas() != 1 {
		t.Fatal("el fondo siguió leyendo el proveedor")
	}
	b.EnPrimerPlano()
	b.hiloID = "abierto"
	relojes[len(relojes)-1]()
	if s.lecturas() != 1 {
		t.Fatal("con un mensaje abierto se recargó la lista debajo")
	}
	b.hiloID = ""
	relojes[len(relojes)-1]()
	if s.lecturas() != 2 {
		t.Fatalf("el refresco no se reanudó: %d lecturas", s.lecturas())
	}
}

func TestFondoNoCancelaEnvioExternoEnCamino(t *testing.T) {
	s := &correoFalso{sent: api.ExternalMailSent{}}
	b, rd := redactorDeCuenta(t, s)
	rd.r.B.Para, rd.r.B.Cuerpo = "to@example.eu", "mensaje sintético"
	var trabajo func()
	b.Lanzar = func(f func()) { trabajo = f }
	b.enviar(rd, false)
	b.EnSegundoPlano()
	trabajo()
	if s.sends != 1 || b.redaccion != nil {
		t.Fatal("ir al fondo impidió el envío iniciado")
	}
}
