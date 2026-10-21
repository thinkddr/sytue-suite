// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package social

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

const (
	// colaDeSalida es cuántas tramas puede acumular una conexión antes de darse
	// por lenta y cerrarse.
	colaDeSalida = 64
	// maxOrdenBytes acota la orden entrante: 500 caracteres de cuerpo caben de
	// sobra en 8 KiB aunque vengan escapados.
	maxOrdenBytes = 8 << 10
	// cadaPing mantiene viva la conexión y detecta pares muertos.
	cadaPing = 30 * time.Second
)

// esperaDeEscritura acota cada escritura al socket. Es variable para que el
// test del cliente lento la alargue: si no, una escritura que caduca también
// echa al cliente y el test no vería si el cierre por cola llena funciona.
var esperaDeEscritura = 10 * time.Second

// Cliente es una conexión WebSocket al chat de un vídeo, ya autenticada y con el
// acceso resuelto por quien la aceptó.
type Cliente struct {
	svc      *Servicio
	conn     *websocket.Conn
	quien    Quien
	video    Video
	permisos Permisos
	salida   chan Trama
	// cerrando garantiza un solo cierre por «cliente lento».
	cerrando sync.Once
	// bloqueados son las personas cuyos mensajes esta conexión no recibe.
	mu         sync.RWMutex
	bloqueados map[string]bool
}

func (c *Cliente) bloquea(sub string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bloqueados[sub]
}

func (c *Cliente) bloquear(sub string, puesto bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bloqueados == nil {
		c.bloqueados = map[string]bool{}
	}
	if puesto {
		c.bloqueados[sub] = true
	} else {
		delete(c.bloqueados, sub)
	}
}

// NuevoCliente envuelve una conexión aceptada. `quien` y `video` vienen de la
// capa HTTP tras verificar el token y resolver el acceso.
func (s *Servicio) NuevoCliente(conn *websocket.Conn, quien Quien, video Video, permisos Permisos) *Cliente {
	return &Cliente{svc: s, conn: conn, quien: quien, video: video, permisos: permisos, salida: make(chan Trama, colaDeSalida)}
}

// encolar deja una trama para el bucle de escritura sin bloquear; si la cola
// está llena, la conexión es lenta y se cierra para no frenar a la sala.
//
// El cierre va aparte: `Close` hace el apretón de manos completo y espera la
// respuesta del otro extremo, y un cliente que no lee tampoco contesta, así que
// hacerlo aquí bloquearía la difusión a toda la sala.
func (c *Cliente) encolar(t Trama) {
	select {
	case c.salida <- t:
	default:
		c.cerrando.Do(func() {
			go func() {
				// Close escribe la trama 1008 y espera la respuesta; si el escritor
				// lleva la conexión atascada en un par que no lee, ni la trama sale.
				// Entonces se corta sin ceremonia: un cliente que no lee no verá el
				// motivo, pero deja de frenar a la sala, que es lo que importa.
				if err := c.conn.Close(websocket.StatusPolicyViolation, "cliente lento"); err != nil {
					_ = c.conn.CloseNow()
				}
			}()
		})
	}
}

// Servir atiende la conexión hasta que el cliente cuelga, el contexto termina o
// el token caduca (`expira`; cero = sin límite). Entra en la sala, difunde el
// recuento de espectadores a la entrada y a la salida, y cierra limpio.
func (c *Cliente) Servir(ctx context.Context, expira time.Time) {
	hub := c.svc.hub
	// Antes de entrar en la sala: desde el primer mensaje, sin los bloqueados. Si
	// la lectura falla se entra igual: el historial REST ya los filtra.
	if c.svc.store != nil && c.svc.store.pool != nil {
		if lista, err := c.svc.store.bloqueados(ctx, c.quien.Sub); err == nil {
			for _, b := range lista {
				c.bloquear(b.Sub, true)
			}
		} else {
			log.From(ctx).Warn("cast/chat: no se pudieron leer los bloqueos", "error", err)
		}
	}
	n := hub.alta(c.video.Tenant, c.video.ID, c)
	hub.Difundir(c.video.Tenant, c.video.ID, tramaEspectadores(n))
	defer func() {
		quedan := hub.baja(c.video.Tenant, c.video.ID, c)
		hub.Difundir(c.video.Tenant, c.video.ID, tramaEspectadores(quedan))
	}()

	// El token caduca: cierre con 1008 y su motivo, no cancelando el contexto de
	// lectura, que corta el socket sin trama de cierre y el cliente no sabe por qué.
	if !expira.IsZero() {
		caduca := time.AfterFunc(time.Until(expira), func() {
			_ = c.conn.Close(websocket.StatusPolicyViolation, "token caducado")
		})
		defer caduca.Stop()
	}

	// Contexto propio para despertar al escritor en cuanto la lectura termina: si
	// no, se quedaría esperando al siguiente ping y la sala tardaría medio minuto
	// en descontar al que se fue.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	escritorListo := make(chan struct{})
	go c.escribir(ctx, escritorListo)
	c.leer(ctx)
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
	cancel()
	<-escritorListo
}

// escribir drena la cola de salida y emite pings periódicos.
func (c *Cliente) escribir(ctx context.Context, listo chan<- struct{}) {
	defer close(listo)
	tic := time.NewTicker(cadaPing)
	defer tic.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-c.salida:
			wctx, cancel := context.WithTimeout(ctx, esperaDeEscritura)
			err := wsjson.Write(wctx, c.conn, t)
			cancel()
			if err != nil {
				return
			}
			if t.cerrar {
				if err := c.conn.Close(websocket.StatusPolicyViolation, "sin acceso"); err != nil {
					_ = c.conn.CloseNow()
				}
				return
			}
		case <-tic.C:
			pctx, cancel := context.WithTimeout(ctx, esperaDeEscritura)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// leer procesa las órdenes del cliente hasta el cierre.
func (c *Cliente) leer(ctx context.Context) {
	c.conn.SetReadLimit(maxOrdenBytes)
	for {
		var o Orden
		if err := wsjson.Read(ctx, c.conn, &o); err != nil {
			if websocket.CloseStatus(err) == -1 && ctx.Err() == nil {
				log.From(ctx).Debug("cast/chat: lectura terminada", "error", err)
			}
			return
		}
		c.atender(ctx, o)
	}
}

// atender despacha una orden. Lo único que el cliente puede pedir es enviar.
func (c *Cliente) atender(ctx context.Context, o Orden) {
	if o.Type != OrdenEnviar {
		c.encolar(tramaError("bad_request", "Unknown order."))
		return
	}
	// Cada envío vuelve a resolver el acceso: quien pierde el vídeo (lo borran, lo
	// cierran, le quitan de la lista) o ve el chat apagado deja de escribir en el
	// acto, no cuando caduque su token. Es una consulta por mensaje, y el ritmo ya
	// está acotado a uno por segundo por persona.
	enviar := func() (Mensaje, error) { return c.svc.EnviarChat(ctx, c.quien, c.video.ID, o.Body) }
	if c.svc.acceso == nil {
		// Sin resolutor (las pruebas de la conexión) no hay contra qué revalidar.
		enviar = func() (Mensaje, error) { return c.svc.enviarChat(ctx, c.quien, c.video, o.Body) }
	}
	if _, err := enviar(); err != nil {
		t := tramaError(codigoDe(err), err.Error())
		// Ya no lo ve: tras decirle por qué, se cierra, como al caducar el token.
		t.cerrar = errors.Is(err, ErrNoExiste)
		c.encolar(t)
	}
}

// codigoDe es el código de error del sobre canónico para una trama de error.
func codigoDe(err error) string {
	switch {
	case errors.Is(err, ErrEntradaInvalida):
		return "bad_request"
	case errors.Is(err, ErrDemasiadoRapido):
		return "too_many_requests"
	case errors.Is(err, ErrDesactivado):
		return "chat_disabled"
	case errors.Is(err, ErrBloqueado):
		return "blocked"
	case errors.Is(err, ErrProhibido):
		return "forbidden"
	case errors.Is(err, ErrNoExiste):
		return "not_found"
	}
	return "internal"
}
