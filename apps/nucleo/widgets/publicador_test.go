// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package widgets

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// banco es un Publicador de Mail con un reloj falso y una Fuente que cuenta sus
// lecturas. La vista captura la época del disco, como haría una suite recién adoptada.
type banco struct {
	t        *testing.T
	reloj    *RelojDePrueba
	pub      *Publicador
	lecturas int
	valor    int64
	err      error
	durante  func() // corre dentro de la Fuente, a mitad de la lectura
}

func nuevoBanco(t *testing.T, desde time.Time) *banco {
	t.Helper()
	fixture(t)
	b := &banco{t: t, reloj: NuevoRelojDePrueba(desde), valor: 1}
	b.pub = NuevoPublicador(b.reloj.Opciones(OpcionesPublicador{
		Vista: func() View { return NewView("eu.sytue.mail", "mail", Capture(ajustes.Leer().Epoca)) },
		Fuente: func(context.Context) (Detail, error) {
			b.lecturas++
			if b.durante != nil {
				f := b.durante
				b.durante = nil
				f()
			}
			if b.err != nil {
				return Detail{}, b.err
			}
			return Detail{Counters: []Counter{{Kind: "unread", Value: b.valor}}}, nil
		},
	}))
	return b
}

func (b *banco) publicado() int64 {
	b.t.Helper()
	s, err := Read("eu.sytue.mail")
	if err != nil {
		return -1
	}
	return s.Counters[0].Value
}

func TestPublicadorAgrupaUnaRafagaEnUnaRonda(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	for range 10 {
		b.pub.Avisar()
		b.reloj.Avanzar(100 * time.Millisecond)
	}
	b.reloj.Avanzar(2 * time.Second)
	if b.lecturas != 1 || b.publicado() != 1 {
		t.Fatalf("10 avisos en 1 s hacen %d lecturas (publicado %d)", b.lecturas, b.publicado())
	}
	// Una ráfaga continua no aplaza para siempre: como tarde, EsperaMax.
	b.lecturas = 0
	for range 6 {
		b.pub.Avisar()
		b.reloj.Avanzar(time.Second)
	}
	if b.lecturas != 1 {
		t.Fatalf("una ráfaga continua de 6 s hizo %d lecturas; quiero 1 a los 5 s", b.lecturas)
	}
}

func TestPublicadorUnAvisoDuranteLaRondaHaceExactamenteOtra(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.durante = func() {
		// Llega una mutación mientras se lee: tres avisos, una sola vuelta más.
		b.valor = 2
		b.pub.Avisar()
		b.pub.Avisar()
		b.pub.Avisar()
	}
	b.pub.Avisar()
	b.reloj.Avanzar(time.Minute)
	if b.lecturas != 2 {
		t.Fatalf("avisos durante la ronda: %d lecturas; quiero 2", b.lecturas)
	}
	if b.publicado() != 2 {
		t.Fatalf("en disco queda %d, no lo último (2)", b.publicado())
	}
}

func TestPublicadorNoSeLeeConLaVistaPrivada(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.pub.o.Vista = func() View { return View{} }
	b.pub.Avisar()
	b.reloj.Avanzar(time.Minute)
	if b.lecturas != 0 {
		t.Fatal("sin sesión adoptada se ha leído del servicio")
	}
}

// TestPublicadorRepublicaSinRedSoloConLaCacheLimpia: tras una renovación (la misma
// época, otra marca) lo publicado sigue valiendo y republicar no pide nada. Pero si
// hubo un aviso que no llegó a publicarse —sucio—, se lee: nunca se publica la caché
// de antes de una mutación.
func TestPublicadorRepublicaSinRedSoloConLaCacheLimpia(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.pub.Avisar()
	b.reloj.Avanzar(2 * time.Second)
	entrar(t, "ana") // renovación: la marca cambia, la época no
	if _, err := Read("eu.sytue.mail"); err != nil {
		t.Fatalf("v2 lleva la época: una renovación no oculta lo publicado (%v)", err)
	}
	b.pub.Republicar()
	b.reloj.Avanzar(0)
	if b.lecturas != 1 || b.publicado() != 1 {
		t.Fatalf("republicar con la caché limpia: %d lecturas, publicado %d", b.lecturas, b.publicado())
	}
	// Una ronda falla (la red) después de una mutación: queda sucio.
	b.valor, b.err = 5, errors.New("sin red")
	b.pub.Avisar()
	b.reloj.Avanzar(2 * time.Second)
	b.err = nil
	entrar(t, "ana")
	b.pub.Republicar()
	b.reloj.Avanzar(2 * time.Second)
	if b.lecturas != 3 || b.publicado() != 5 {
		t.Fatalf("republicar sucio publicó la caché vieja: %d lecturas, publicado %d", b.lecturas, b.publicado())
	}
}

// TestPublicadorSoloGuardaCacheTrasPublicar: una ronda cuya escritura falla (la
// identidad cambió a mitad de la lectura) no deja sus datos en la caché.
func TestPublicadorSoloGuardaCacheTrasPublicar(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.pub.Avisar()
	b.reloj.Avanzar(2 * time.Second)
	b.valor = 7
	b.durante = func() { entrar(t, "bea") }
	b.pub.Avisar()
	b.reloj.Avanzar(2 * time.Second)
	if b.pub.cache == nil || b.pub.cache.Counters[0].Value != 1 || !b.pub.sucio {
		t.Fatalf("la ronda fallida dejó en caché sus datos: %+v sucio=%v", b.pub.cache, b.pub.sucio)
	}
	b.valor = 8
	b.pub.Republicar()
	b.reloj.Avanzar(2 * time.Second)
	if b.publicado() != 8 {
		t.Fatalf("republicar tras el fallo escribió %d, no una lectura nueva", b.publicado())
	}
}

func TestPublicadorPausarNoEsTerminal(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.pub.Avisar()
	b.pub.Pausar()
	b.pub.Avisar()
	b.pub.AvisarEscritura()
	b.pub.Republicar()
	b.reloj.Avanzar(time.Hour)
	if b.lecturas != 0 {
		t.Fatalf("en pausa se ha leído %d veces", b.lecturas)
	}
	b.pub.Reanudar()
	b.reloj.Avanzar(2 * time.Second)
	if b.lecturas != 1 || b.publicado() != 1 {
		t.Fatalf("tras reanudar no se publica: %d lecturas", b.lecturas)
	}
}

func TestPublicadorBorraSuWidgetSiRechazanLaSesionYNoSiFallaLaRed(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	scope := Capture(ajustes.Leer().Epoca)
	if err := publicarContadores(context.Background(), "eu.sytue.calendar", "calendar", scope, nil, Vigencia); err != nil {
		t.Fatal(err)
	}
	b.pub.Avisar()
	b.reloj.Avanzar(2 * time.Second)
	b.err = &net.OpError{Op: "dial", Err: errors.New("sin red")}
	b.pub.Avisar()
	b.reloj.Avanzar(2 * time.Second)
	if b.publicado() != 1 {
		t.Fatal("un error de red ha borrado lo publicado")
	}
	for _, rechazo := range []error{
		&api.Error{Status: 401, URL: "https://mail.example/mailboxes"},
		errors.Join(errors.New("invalid_grant"), auth.ErrLoginRequired),
	} {
		if err := publicarContadores(context.Background(), "eu.sytue.mail", "mail", scope, []Counter{{Kind: "unread", Value: 1}}, Vigencia); err != nil {
			t.Fatal(err)
		}
		b.err = rechazo
		b.pub.Avisar()
		b.reloj.Avanzar(2 * time.Second)
		if _, err := Read("eu.sytue.mail"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%v: el widget de la app sigue publicado (%v)", rechazo, err)
		}
		if _, err := Read("eu.sytue.calendar"); err != nil {
			t.Fatalf("%v: el rechazo de Mail ha borrado el widget de Calendar (%v)", rechazo, err)
		}
	}
}

// TestPublicadorAgrupaLasEscrituras: 20 escrituras en 60 s dan como mucho 3 rondas.
func TestPublicadorAgrupaLasEscrituras(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	for range 20 {
		b.pub.AvisarEscritura()
		b.reloj.Avanzar(3 * time.Second)
	}
	if b.lecturas > 3 || b.lecturas == 0 {
		t.Fatalf("20 escrituras en 60 s hicieron %d rondas; quiero entre 1 y 3", b.lecturas)
	}
	// Al final de la ventana se publica lo último.
	b.valor = 9
	b.pub.AvisarEscritura()
	b.reloj.Avanzar(time.Minute)
	if b.publicado() != 9 {
		t.Fatalf("la última escritura de la ventana no llegó: %d", b.publicado())
	}
}

// El eco de la señal de cambios: el autoguardado de una nota escribe cada 3 s y a cada
// escritura le llega, un segundo después, su propio `datos`. Las rondas siguen siendo
// las de las escrituras (entre 1 y 3 por minuto), y una señal sin escritura propia
// reciente se lee tras el antirrebote normal. VISTO EN ROJO con AvisarDelServidor
// llamando a Avisar: 20 rondas en el minuto.
func TestPublicadorElEcoDeUnaEscrituraRespetaElMinimo(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	for range 20 {
		b.pub.AvisarEscritura()
		b.reloj.Avanzar(time.Second)
		b.pub.AvisarDelServidor()
		b.reloj.Avanzar(2 * time.Second)
	}
	if b.lecturas > 3 || b.lecturas == 0 {
		t.Fatalf("20 escrituras con su eco en 60 s hicieron %d rondas; quiero entre 1 y 3", b.lecturas)
	}
	b.reloj.Avanzar(time.Minute)
	antes := b.lecturas
	b.valor = 4
	b.pub.AvisarDelServidor() // de otro aparato, sin escritura propia en la ventana
	b.reloj.Avanzar(AntirreboteWidget)
	if b.lecturas != antes+1 || b.publicado() != 4 {
		t.Fatalf("una señal sin escritura propia no se leyó tras el antirrebote: %d rondas, publicado %d", b.lecturas-antes, b.publicado())
	}
}

// Una escritura aislada con su eco es UNA ronda: el eco llega antes de que corra el
// antirrebote y deja apuntada una espera al final de la ventana de MinEscrituras, que
// la ronda del antirrebote ya cubre. Y un cambio de otra persona pasada VentanaEco no
// espera a esa ventana. VISTO EN ROJO sin anular la espera al final de la ronda (2
// rondas: a 1,5 s y a 30 s) y con el eco contado desde ultimaEscritura durante toda
// la ventana de MinEscrituras (el cambio ajeno a los 10 s se leía a los 30).
func TestPublicadorUnaEscrituraConSuEcoEsUnaRonda(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.pub.AvisarEscritura()
	b.reloj.Avanzar(200 * time.Millisecond)
	b.pub.AvisarDelServidor() // su eco
	b.reloj.Avanzar(time.Minute)
	if b.lecturas != 1 {
		t.Fatalf("una escritura con su eco hizo %d rondas; quiero 1", b.lecturas)
	}
	b.pub.AvisarEscritura()
	b.reloj.Avanzar(200 * time.Millisecond)
	b.pub.AvisarDelServidor()
	b.reloj.Avanzar(10 * time.Second)
	antes := b.lecturas
	b.valor = 7
	b.pub.AvisarDelServidor() // un compañero, 10 s después de la escritura propia
	b.reloj.Avanzar(AntirreboteWidget)
	if b.lecturas != antes+1 || b.publicado() != 7 {
		t.Fatalf("el cambio ajeno pasada VentanaEco no se leyó tras el antirrebote: %d rondas, publicado %d", b.lecturas-antes, b.publicado())
	}
}

func TestPublicadorLateYCambiaDeDiaAMedianoche(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 23, 59, 0, 0, time.Local))
	b.pub.Avisar()
	b.reloj.Avanzar(2 * time.Second)
	if b.lecturas != 1 {
		t.Fatal("no se publicó")
	}
	b.reloj.Avanzar(70 * time.Second) // pasa la medianoche + 5 s, y el antirrebote
	if b.lecturas != 2 {
		t.Fatalf("a medianoche no se vuelve a leer: %d", b.lecturas)
	}
	b.reloj.Avanzar(LatidoWidget + 2*time.Second)
	if b.lecturas != 3 {
		t.Fatalf("el latido no vuelve a leer con la app viva: %d", b.lecturas)
	}
}

func TestRefrescarYaEsperaYDevuelveElResultado(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.valor = 4
	if err := b.pub.RefrescarYa(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.publicado() != 4 {
		t.Fatalf("RefrescarYa no publicó: %d", b.publicado())
	}
	b.pub.Pausar()
	if err := b.pub.RefrescarYa(context.Background()); !errors.Is(err, ErrPrivate) {
		t.Fatalf("RefrescarYa en pausa: %v", err)
	}
}

// TestPublicarPendienteNoEsperaNiAlAntirreboteNiALaVentana: al pasar al fondo, una
// escritura aplazada al final de su ventana de 30 s (y otra en pleno antirrebote) se
// publica ya, sin mover el reloj. Sin nada pendiente, no se lee.
func TestPublicarPendienteNoEsperaNiAlAntirreboteNiALaVentana(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.pub.AvisarEscritura()
	b.reloj.Avanzar(10 * time.Second)
	if b.lecturas != 1 || b.publicado() != 1 {
		t.Fatalf("la primera escritura no se publicó: %d lecturas", b.lecturas)
	}
	if err := b.pub.PublicarPendiente(context.Background()); err != nil || b.lecturas != 1 {
		t.Fatalf("sin nada pendiente se ha leído: %d lecturas, %v", b.lecturas, err)
	}
	b.valor = 2
	b.pub.AvisarEscritura() // dentro de la ventana: queda para dentro de 20 s
	if err := b.pub.PublicarPendiente(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.lecturas != 2 || b.publicado() != 2 {
		t.Fatalf("lo aplazado no se publica al pasar al fondo: %d lecturas, publicado %d", b.lecturas, b.publicado())
	}
	// Lo que se canceló no vuelve a leer al vencer su plazo.
	b.reloj.Avanzar(time.Minute)
	if b.lecturas != 2 {
		t.Fatalf("la ventana cancelada ha vuelto a leer: %d", b.lecturas)
	}
	b.valor = 3
	b.pub.Avisar() // en pleno antirrebote
	if err := b.pub.PublicarPendiente(context.Background()); err != nil || b.publicado() != 3 {
		t.Fatalf("lo que esperaba al antirrebote no se publica: %d, %v", b.publicado(), err)
	}
}

// TestRefrescarEnFondoNoLeeDosVeces: lanzada en frío para la tarea de refresco, la app
// ya tiene apuntada la ronda del arranque (Reanudar); la tarea la publica ya en vez de
// hacer otra, y el antirrebote cancelado no vuelve a leer. Sin nada pendiente, la tarea
// sí lee: a eso viene.
//
// VISTO EN ROJO: con RefrescarYa en vez de RefrescarEnFondo, dos lecturas.
func TestRefrescarEnFondoNoLeeDosVeces(t *testing.T) {
	b := nuevoBanco(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	b.pub.Reanudar() // el arranque
	if err := b.pub.RefrescarEnFondo(context.Background()); err != nil {
		t.Fatal(err)
	}
	b.reloj.Avanzar(10 * time.Second)
	if b.lecturas != 1 || b.publicado() != 1 {
		t.Fatalf("el arranque y la tarea han leído %d veces (publicado %d)", b.lecturas, b.publicado())
	}
	b.valor = 2
	if err := b.pub.RefrescarEnFondo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.lecturas != 2 || b.publicado() != 2 {
		t.Fatalf("sin nada pendiente la tarea no lee: %d lecturas, publicado %d", b.lecturas, b.publicado())
	}
}
