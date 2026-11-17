// SPDX-License-Identifier: Apache-2.0
// Package smtptest provides a disposable loopback TLS SMTP receiver. It never
// contacts a provider and holds accepted synthetic messages only in memory.
package smtptest

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type (
	Config struct{ LoseDataACK, LoseQuitACK bool }
	Server struct {
		Host, Port     string
		Roots          *x509.CertPool
		CertificatePEM []byte
		Received       chan []byte
		accepted       atomic.Int32
		config         Config
		listener       net.Listener
		connections    sync.Map
	}
)

func New(t *testing.T, config Config) *Server {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	encoded, err := x509.CreateCertificate(rand.Reader, certificate, certificate, public, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(encoded)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	serverTLS := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{{Certificate: [][]byte{encoded}, PrivateKey: key}}}
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Host: "localhost", Port: port, Roots: roots, CertificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded}), Received: make(chan []byte, 1024), config: config, listener: listener}
	t.Cleanup(server.Close)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			server.connections.Store(connection, struct{}{})
			go server.serve(connection, serverTLS)
		}
	}()
	return server
}

func (s *Server) Accepted() int32 { return s.accepted.Load() }
func (s *Server) Close() {
	_ = s.listener.Close()
	s.connections.Range(func(key, _ any) bool {
		if conn, ok := key.(net.Conn); ok {
			_ = conn.Close()
		}
		return true
	})
}

func (s *Server) serve(connection net.Conn, config *tls.Config) {
	original := connection
	defer func() { _ = original.Close(); s.connections.Delete(original) }()
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	writer := bufio.NewWriter(connection)
	reader := textproto.NewReader(bufio.NewReader(connection))
	reply := func(text string) bool {
		if _, err := writer.WriteString(text + "\r\n"); err != nil {
			return false
		}
		return writer.Flush() == nil
	}
	if !reply("220 localhost ESMTP") {
		return
	}
	encrypted := false
	for {
		line, err := reader.ReadLine()
		if err != nil {
			return
		}
		switch {
		case strings.HasPrefix(line, "EHLO "):
			if !ehloReply(encrypted, reply) {
				return
			}
		case line == "STARTTLS":
			if !reply("220 Begin TLS") {
				return
			}
			secure := tls.Server(connection, config)
			if secure.HandshakeContext(context.Background()) != nil {
				return
			}
			connection = secure
			writer = bufio.NewWriter(secure)
			reader = textproto.NewReader(bufio.NewReader(secure))
			encrypted = true
		case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
			if !encrypted || !reply("250 OK") {
				return
			}
		case line == "DATA":
			if !s.handleData(encrypted, reader, reply) {
				return
			}
		case line == "QUIT":
			if !s.config.LoseQuitACK {
				reply("221 Bye")
			}
			return
		default:
			if !reply("500 Unsupported") {
				return
			}
		}
	}
}

func ehloReply(encrypted bool, reply func(string) bool) bool {
	if !encrypted {
		return reply("250-localhost\r\n250 STARTTLS")
	}
	return reply("250 localhost")
}

func (s *Server) handleData(encrypted bool, reader *textproto.Reader, reply func(string) bool) bool {
	if !encrypted || !reply("354 Send message") {
		return false
	}
	raw, err := reader.ReadDotBytes()
	if err != nil {
		return false
	}
	s.accepted.Add(1)
	select {
	case s.Received <- raw:
	default:
		clear(raw)
		return false
	}
	if s.config.LoseDataACK {
		return false
	}
	return reply("250 Message accepted")
}
