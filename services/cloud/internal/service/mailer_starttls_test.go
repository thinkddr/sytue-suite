// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
)

// CLOUD-19: un servidor que no anuncia STARTTLS (o alguien en medio que lo
// quita) no recibe el aviso en claro.
func TestAlertaPorCorreoSinSTARTTLSNoSale(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	entregado := make(chan bool, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		rd := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 smtp ESMTP\r\n"))
		data := false
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				entregado <- data
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case data && cmd == ".":
				_, _ = conn.Write([]byte("250 ok\r\n"))
			case data:
			case strings.HasPrefix(cmd, "EHLO"):
				_, _ = conn.Write([]byte("250-smtp\r\n250 8BITMIME\r\n"))
			case cmd == "DATA":
				data = true
				_, _ = conn.Write([]byte("354 adelante\r\n"))
			case cmd == "QUIT":
				_, _ = conn.Write([]byte("221 adiós\r\n"))
				entregado <- data
				return
			default:
				_, _ = conn.Write([]byte("250 ok\r\n"))
			}
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	cfg := config.Config{SMTPHost: host, SMTPPort: port, SMTPFrom: "avisos@sytue.eu"}
	err = sendAlertMail(context.Background(), cfg, "ana@example.eu", "caído", "web caída")
	if err == nil {
		t.Fatal("sin STARTTLS el aviso no puede salir")
	}
	if <-entregado {
		t.Fatal("el aviso llegó a DATA en claro")
	}
}
