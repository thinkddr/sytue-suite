// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del smarthost de salida. Lo que fijan no es «que envíe»: es que las dos
// decisiones peligrosas del relé no se pierdan en un refactor.
//
//  1. Sin relé se entrega por MX. Es el comportamiento de siempre y el que usan
//     los despliegues con IP propia; romperlo por añadir el relé sería cambiar
//     el camino de entrega de todo el mundo sin querer.
//  2. Con relé, TLS es OBLIGATORIO. Hacia el smarthost viajan usuario y
//     contraseña: el «mejor entregar sin cifrar que no entregar» que vale para
//     un MX ajeno aquí significaría publicar la credencial en la red.
package delivery

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

// servidorFalso acepta una conexión y responde el saludo mínimo, SIN anunciar
// STARTTLS. Sirve para comprobar a dónde se conecta el emisor y qué hace cuando
// no hay cifrado disponible.
func servidorFalso(t *testing.T) (string, string, chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	visto := make(chan string, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// 220 y nada más: sin EHLO con STARTTLS, el cliente no puede cifrar.
				_, _ = c.Write([]byte("220 falso ESMTP\r\n"))
				buf := make([]byte, 256)
				n, _ := c.Read(buf)
				select {
				case visto <- string(buf[:n]):
				default:
				}
				_, _ = c.Write([]byte("250 ok\r\n"))
			}(c)
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	return host, port, visto
}

func TestRelayExigeTLSYNoCaeEnClaro(t *testing.T) {
	host, port, _ := servidorFalso(t)
	s := NewSMTPSenderConRelay(slog.New(slog.NewTextHandler(nil, nil)), &Relay{
		Host: host, Port: port, Username: "usuario", Password: "secreto",
	})
	s.timeout = 3 * time.Second

	_, err := s.Send(context.Background(), "yo@ejemplo.test",
		[]string{"tu@otrodominio.test"}, strings.NewReader("Subject: x\r\n\r\nhola"))
	if err == nil {
		t.Fatal("sin STARTTLS el relé debe fallar, no entregar en claro con la credencial")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("el error debe decir que falta STARTTLS, dijo: %v", err)
	}
}

func TestRelaySeSaltaElMXDelDestino(t *testing.T) {
	host, port, visto := servidorFalso(t)
	s := NewSMTPSenderConRelay(slog.New(slog.NewTextHandler(nil, nil)), &Relay{
		Host: host, Port: port,
	})
	s.timeout = 3 * time.Second

	// El dominio destino NO existe: si el emisor resolviera su MX no llegaría a
	// ninguna parte. Que el servidor falso reciba la conexión demuestra que el
	// relé sustituye al MX y no se limita a añadirse como alternativa.
	_, _ = s.Send(context.Background(), "yo@ejemplo.test",
		[]string{"tu@dominio-que-no-existe.invalid"}, strings.NewReader("Subject: x\r\n\r\nhola"))

	select {
	case <-visto:
		// Conectó con el relé: correcto.
	case <-time.After(3 * time.Second):
		t.Fatal("el emisor no se conectó al relé: sigue resolviendo el MX del destino")
	}
}

func TestSinRelaySigueEntregandoPorMX(t *testing.T) {
	s := NewSMTPSenderConRelay(slog.New(slog.NewTextHandler(nil, nil)), nil)
	if s.relay != nil {
		t.Fatal("sin configuración de relé el emisor no debe tener smarthost")
	}
	// Y el constructor de siempre tampoco, que es el que usa quien no configura
	// nada: si esto cambiara, todos los despliegues existentes cambiarían de
	// camino de entrega sin tocar su configuración.
	if NewSMTPSender(slog.New(slog.NewTextHandler(nil, nil))).relay != nil {
		t.Fatal("NewSMTPSender no puede traer relé por defecto")
	}
}

// Un relé sin puerto es un error de configuración fácil de cometer y silencioso:
// se conectaría al 25, que es justo el que el ISP bloquea y el que el relé no
// suele escuchar.
func TestRelaySinPuertoUsa587(t *testing.T) {
	s := NewSMTPSenderConRelay(slog.New(slog.NewTextHandler(nil, nil)), &Relay{Host: "relay.ejemplo"})
	if s.relay == nil || s.relay.Port != "587" {
		t.Fatalf("puerto por defecto = %v, quiero 587", s.relay)
	}
}
