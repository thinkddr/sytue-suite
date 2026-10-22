// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
)

// servidorDeChat acepta cada conexión como `quien` sobre `video` (sin base de
// datos: lo que aquí se prueba es la conexión, no el store) y devuelve el
// servidor y un abridor de clientes.
func servidorDeChat(t *testing.T, svc *Servicio, video Video, expira time.Time) (abrir func(sub string) *websocket.Conn, ctx context.Context) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		quien := Quien{Sub: r.URL.Query().Get("sub"), Tenant: video.Tenant, Nombre: "Ana"}
		svc.NuevoCliente(conn, quien, video, Permisos{Ver: true}).Servir(context.WithoutCancel(r.Context()), expira)
	}))
	t.Cleanup(srv.Close)
	// Holgado: en el CI, con el resto del repo compilando y probando a la vez, un
	// cierre que aquí tarda milisegundos se ha visto pasar de cinco segundos.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	abrir = func(sub string) *websocket.Conn {
		c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/?sub="+sub, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.CloseNow() })
		return c
	}
	return abrir, ctx
}

// leerTipo lee tramas hasta dar con una del tipo pedido (o un error).
func leerTipo(ctx context.Context, t *testing.T, c *websocket.Conn, tipo string) Trama {
	t.Helper()
	for {
		var tr Trama
		if err := wsjson.Read(ctx, c, &tr); err != nil {
			t.Fatalf("esperaba %q: %v", tipo, err)
		}
		if tr.Type == tipo || tr.Type == TramaError {
			return tr
		}
	}
}

func videoDePrueba(chat bool) Video {
	return Video{ID: uuid.New(), Tenant: "acme", OwnerSub: "duenya", ComentariosActivos: true, ChatActivo: chat}
}

// Al entrar y al salir, la sala recibe el recuento de espectadores.
func TestClienteAnunciaEspectadores(t *testing.T) {
	svc := New(nil, nil)
	abrir, ctx := servidorDeChat(t, svc, videoDePrueba(true), time.Time{})
	ana := abrir("ana")
	if tr := leerTipo(ctx, t, ana, TramaEspectadores); tr.Count == nil || *tr.Count != 1 {
		t.Fatalf("ana entra sola y ve %+v", tr)
	}
	bea := abrir("bea")
	if tr := leerTipo(ctx, t, ana, TramaEspectadores); tr.Count == nil || *tr.Count != 2 {
		t.Fatalf("con bea dentro ana ve %+v", tr)
	}
	if tr := leerTipo(ctx, t, bea, TramaEspectadores); tr.Count == nil || *tr.Count != 2 {
		t.Fatalf("bea ve %+v", tr)
	}
	_ = bea.Close(websocket.StatusNormalClosure, "")
	if tr := leerTipo(ctx, t, ana, TramaEspectadores); tr.Count == nil || *tr.Count != 1 {
		t.Fatalf("tras irse bea ana ve %+v", tr)
	}
}

// Una orden desconocida o un chat apagado contestan con una trama de error y la
// conexión sigue abierta.
func TestClienteContestaConErrorSinCerrar(t *testing.T) {
	svc := New(nil, nil)
	abrir, ctx := servidorDeChat(t, svc, videoDePrueba(false), time.Time{})
	ana := abrir("ana")
	leerTipo(ctx, t, ana, TramaEspectadores)

	if err := wsjson.Write(ctx, ana, Orden{Type: "typing"}); err != nil {
		t.Fatal(err)
	}
	if tr := leerTipo(ctx, t, ana, TramaError); tr.Code != "bad_request" {
		t.Fatalf("orden desconocida: %+v", tr)
	}
	if err := wsjson.Write(ctx, ana, Orden{Type: OrdenEnviar, Body: "hola"}); err != nil {
		t.Fatal(err)
	}
	if tr := leerTipo(ctx, t, ana, TramaError); tr.Code != "chat_disabled" {
		t.Fatalf("chat apagado: %+v", tr)
	}
	// Sigue viva: otra orden vuelve a contestar.
	if err := wsjson.Write(ctx, ana, Orden{Type: "x"}); err != nil {
		t.Fatal(err)
	}
	leerTipo(ctx, t, ana, TramaError)
}

// Una trama por encima del tope cierra la conexión (RFC 6455, 1009).
func TestClienteCierraAnteTramaEnorme(t *testing.T) {
	svc := New(nil, nil)
	abrir, ctx := servidorDeChat(t, svc, videoDePrueba(true), time.Time{})
	ana := abrir("ana")
	leerTipo(ctx, t, ana, TramaEspectadores)
	if err := wsjson.Write(ctx, ana, Orden{Type: OrdenEnviar, Body: strings.Repeat("a", maxOrdenBytes+1)}); err != nil {
		t.Fatal(err)
	}
	var tr Trama
	err := wsjson.Read(ctx, ana, &tr)
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("esperaba cierre 1009, got %v (%+v)", err, tr)
	}
}

// Cuando el token caduca, el servidor cierra con 1008 y la sala pierde un espectador.
func TestClienteCierraAlCaducarElToken(t *testing.T) {
	svc := New(nil, nil)
	v := videoDePrueba(true)
	abrir, ctx := servidorDeChat(t, svc, v, time.Now().Add(150*time.Millisecond))
	ana := abrir("ana")
	leerTipo(ctx, t, ana, TramaEspectadores)
	var tr Trama
	err := wsjson.Read(ctx, ana, &tr)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("esperaba cierre 1008 por token caducado, got %v", err)
	}
	plazo := time.Now().Add(2 * time.Second)
	for svc.Hub().Espectadores(v.Tenant, v.ID) != 0 && time.Now().Before(plazo) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := svc.Hub().Espectadores(v.Tenant, v.ID); n != 0 {
		t.Fatalf("la conexión caducada sigue en la sala: %d", n)
	}
}

// Un cliente que no lee se cierra en vez de frenar a la sala.
func TestClienteLentoSeCierraSinBloquear(t *testing.T) {
	// Sin esto, la escritura atascada caduca a los 10 s y también echa al
	// cliente: el test pasaría aunque el cierre por cola llena no existiera.
	// Sin carrera: los tests en paralelo del paquete esperan a los secuenciales.
	antes := esperaDeEscritura
	esperaDeEscritura = time.Minute
	t.Cleanup(func() { esperaDeEscritura = antes })
	svc := New(nil, nil)
	v := videoDePrueba(true)
	abrir, ctx := servidorDeChat(t, svc, v, time.Time{})
	lento := abrir("lento")
	// Hasta que llega su primera trama de espectadores el servidor puede no haberlo
	// dado de alta todavía: el Dial vuelve antes de que el handler corra. Sin esta
	// espera, bajo carga, el bucle de abajo veía la sala ya vacía, salía sin
	// difundir nada y el cliente agotaba el plazo esperando (ci-integration).
	leerTipo(ctx, t, lento, TramaEspectadores)
	// A partir de aquí, sin leer nada: se difunde hasta que el servidor lo echa de la sala. No vale
	// un número fijo de tramas: cuántas caben entre la cola y los búferes TCP de
	// las dos puntas depende de la máquina (un runner con búferes de socket
	// grandes se tragaba 16 MiB enteros, la cola nunca se llenaba y el cliente se
	// quedaba leyendo hasta agotar el plazo). Lo que se exige es que cada
	// difusión vuelva enseguida y que el cliente acabe fuera.
	cuerpo := strings.Repeat("x", 64<<10)
	// El cierre de coder/websocket espera hasta 5 s a que el escritor suelte el
	// socket o a que un par que no lee conteste: 15 s dan margen a un runner
	// cargado dentro de los 20 del contexto.
	plazo := time.Now().Add(15 * time.Second)
	for svc.Hub().Espectadores(v.Tenant, v.ID) != 0 {
		if time.Now().After(plazo) {
			t.Fatal("el cliente que no lee sigue en la sala")
		}
		inicio := time.Now()
		svc.Hub().Difundir(v.Tenant, v.ID, Trama{Type: TramaMensaje, Body: cuerpo})
		if d := time.Since(inicio); d > time.Second {
			t.Fatalf("Difundir se bloqueó %v con un cliente que no lee", d)
		}
		time.Sleep(time.Millisecond)
	}
	// El cliente acaba viendo el cierre al leer: la trama 1008 si el apretón de
	// manos pudo hacerse, o la conexión cortada si el escritor estaba atascado en
	// él (bajo carga pasa). Lo que no vale es que siga leyendo como si nada.
	var vio error
	for {
		var tr Trama
		if err := wsjson.Read(ctx, lento, &tr); err != nil {
			vio = err
			break
		}
	}
	if estado := websocket.CloseStatus(vio); estado != websocket.StatusPolicyViolation && (estado != -1 || errors.Is(vio, context.DeadlineExceeded)) {
		t.Fatalf("esperaba cierre 1008 o conexión cortada al cliente lento, got %v", vio)
	}
}

func TestCuerpoValido(t *testing.T) {
	t.Parallel()
	if _, err := cuerpoValido("   ", MaxMensaje); !errors.Is(err, ErrEntradaInvalida) {
		t.Errorf("vacío: %v", err)
	}
	if _, err := cuerpoValido(strings.Repeat("é", MaxMensaje+1), MaxMensaje); !errors.Is(err, ErrEntradaInvalida) {
		t.Errorf("largo en runas: %v", err)
	}
	// El tope es en runas, no en bytes: 500 «é» son 1000 bytes y valen.
	if got, err := cuerpoValido(" "+strings.Repeat("é", MaxMensaje)+" ", MaxMensaje); err != nil || len(got) != 2*MaxMensaje {
		t.Errorf("500 runas de dos bytes: %v (%d bytes)", err, len(got))
	}
}

func TestCursorIdaYVuelta(t *testing.T) {
	t.Parallel()
	c := cursorDe{fecha: time.Date(2026, 9, 27, 12, 0, 0, 123456789, time.UTC), id: uuid.New()}
	got, err := descifrarCursor(cifrarCursor(c))
	if err != nil || got == nil || !got.fecha.Equal(c.fecha) || got.id != c.id {
		t.Fatalf("ida y vuelta: %+v, %v", got, err)
	}
	if got, err := descifrarCursor(""); err != nil || got != nil {
		t.Fatalf("vacío = desde el principio: %+v, %v", got, err)
	}
	for _, malo := range []string{"!!!", "aG9sYQ", "MjAyNi0wMS0wMVQwMDowMDowMFp8bm8tdXVpZA"} {
		if _, err := descifrarCursor(malo); !errors.Is(err, ErrEntradaInvalida) {
			t.Errorf("%q: %v", malo, err)
		}
	}
}
