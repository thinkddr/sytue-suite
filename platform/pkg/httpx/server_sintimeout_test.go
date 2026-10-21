// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import "testing"

// 0 es «el valor por defecto»; SinTimeout, sin límite de verdad. Antes no había
// forma de pedir lo segundo y quien escribía 0 se llevaba 30 s.
func TestSinTimeoutQuitaElLimite(t *testing.T) {
	s := NewServer(ServerConfig{Addr: ":0", ReadTimeout: SinTimeout, WriteTimeout: SinTimeout})
	if s.httpServer.ReadTimeout > 0 || s.httpServer.WriteTimeout > 0 {
		t.Fatalf("SinTimeout dejó límites: lectura %v, escritura %v", s.httpServer.ReadTimeout, s.httpServer.WriteTimeout)
	}
	d := NewServer(ServerConfig{Addr: ":0"})
	if d.httpServer.ReadTimeout <= 0 || d.httpServer.WriteTimeout <= 0 {
		t.Fatal("sin configurar debería llevar los límites por defecto")
	}
}
