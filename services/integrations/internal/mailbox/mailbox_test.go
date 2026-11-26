// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
)

// TestElServidorNoLlegaAlBucleLocal: el cliente movido al núcleo lleva aquí el
// marcador anti-SSRF del servidor: una cuenta que apunta a 127.0.0.1 se queda en
// ErrBlocked sin abrir ninguna conexión ni mandar la contraseña.
func TestElServidorNoLlegaAlBucleLocal(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var conexiones atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conexiones.Add(1)
			_ = c.Close()
		}
	}()
	puerto := ln.Addr().(*net.TCPAddr).Port
	c := NewProtocolClient(Account{
		Provider: "imap", Address: "a@example.org", Password: "do-not-send",
		Incoming: Server{Host: "127.0.0.1", Port: puerto, Security: "tls"},
		Outgoing: Server{Host: "127.0.0.1", Port: puerto, Security: "tls"},
	})
	if _, err := c.Test(context.Background()); !errors.Is(err, ErrBlocked) {
		t.Fatalf("loopback reached: %v", err)
	}
	if conexiones.Load() != 0 {
		t.Fatal("se abrió una conexión al bucle local")
	}
}
