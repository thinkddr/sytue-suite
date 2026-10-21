// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

func tlsFixture(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	h := httptest.NewTLSServer(nil)
	defer h.Close()
	roots := x509.NewCertPool()
	roots.AddCert(h.Certificate())
	return h.TLS.Clone(), roots
}

func serverAddress(t *testing.T, addr net.Addr, security string) Server {
	t.Helper()
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		t.Fatal(err)
	}
	var p int
	if _, err := fmt.Sscan(port, &p); err != nil {
		t.Fatal(err)
	}
	return Server{Host: host, Port: p, Username: "test@example.org", Security: security}
}

func permitFixture(t *testing.T) {
	t.Helper()
	// Serial tests only. Production policy remains unchanged outside this test.
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(ip net.IP) bool { return !ip.IsLoopback() }))
}

func scriptServer(t *testing.T, encrypted bool, script func(net.Conn)) (Server, *x509.CertPool) {
	t.Helper()
	config, roots := tlsFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := listener
	if encrypted {
		ln = tls.NewListener(listener, config)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				script(conn)
			}()
		}
	}()
	t.Cleanup(func() { _ = ln.Close(); <-done; wg.Wait() })
	security := "tls"
	if !encrypted {
		security = "starttls"
	}
	return serverAddress(t, ln.Addr(), security), roots
}

func smtpScript(received chan<- []byte, commands *atomic.Int32, dropAfterData bool) func(net.Conn) {
	return func(conn net.Conn) {
		c := textproto.NewConn(conn)
		_ = c.PrintfLine("220 test ESMTP")
		for {
			line, err := c.ReadLine()
			if err != nil {
				return
			}
			verb, _, _ := strings.Cut(line, " ")
			switch verb {
			case "EHLO":
				_ = c.PrintfLine("250-test\r\n250-AUTH PLAIN\r\n250 SIZE 26214400")
			case "AUTH":
				_ = c.PrintfLine("235 authenticated")
			case "NOOP", "MAIL", "RCPT":
				if verb != "NOOP" {
					commands.Add(1)
				}
				_ = c.PrintfLine("250 OK")
			case "DATA":
				commands.Add(1)
				_ = c.PrintfLine("354 send mail")
				data, err := io.ReadAll(c.DotReader())
				if err != nil {
					return
				}
				received <- data
				if dropAfterData {
					return
				}
				_ = c.PrintfLine("250 queued")
			case "QUIT":
				_ = c.PrintfLine("221 bye")
				return
			default:
				_ = c.PrintfLine("500 unsupported")
			}
		}
	}
}

func popScript(raw string, mutations *atomic.Int32) func(net.Conn) {
	return func(conn net.Conn) {
		c := textproto.NewConn(conn)
		_ = c.PrintfLine("+OK test POP3")
		for {
			line, err := c.ReadLine()
			if err != nil {
				return
			}
			verb, _, _ := strings.Cut(line, " ")
			switch verb {
			case "USER", "PASS":
				_ = c.PrintfLine("+OK")
			case "STAT":
				_ = c.PrintfLine("+OK 1 %d", len(raw))
			case "UIDL":
				_ = c.PrintfLine("+OK\r\n1 stable-uid\r\n.")
			case "TOP":
				_ = c.PrintfLine("-ERR TOP unsupported")
			case "RETR":
				_ = c.PrintfLine("+OK %d octets", len(raw))
				w := c.DotWriter()
				_, _ = io.WriteString(w, raw)
				_ = w.Close()
			case "DELE":
				mutations.Add(1)
				_ = c.PrintfLine("-ERR forbidden")
			case "QUIT":
				_ = c.PrintfLine("+OK")
				return
			default:
				_ = c.PrintfLine("-ERR unsupported")
			}
		}
	}
}

func TestProtocolConnectionTestNeverSendsOrDeletes(t *testing.T) {
	permitFixture(t)
	var smtpMutations, popMutations atomic.Int32
	received := make(chan []byte, 2)
	incoming, roots := scriptServer(t, true, popScript("From: a@example.com\r\nSubject: one\r\n\r\nbody\r\n", &popMutations))
	outgoing, _ := scriptServer(t, true, smtpScript(received, &smtpMutations, false))
	p := &protocolClient{account: Account{Provider: "pop3", Address: "test@example.org", Incoming: incoming, Outgoing: outgoing, Password: "fixture-password"}, roots: roots}
	if _, err := p.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	if smtpMutations.Load() != 0 || popMutations.Load() != 0 || len(received) != 0 {
		t.Fatal("connection test modified mail")
	}
	page, err := p.Messages(context.Background(), "INBOX", "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "stable-uid" || page.Items[0].Text != "" {
		t.Fatalf("POP3 list: %+v %v", page, err)
	}
	m, err := p.Message(context.Background(), "INBOX", "stable-uid")
	if err != nil || !strings.Contains(m.Text, "body") {
		t.Fatalf("POP3 read: %+v %v", m, err)
	}
	if err := p.Action(context.Background(), Action{Action: "delete"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("POP3 mutation: %v", err)
	}
	if popMutations.Load() != 0 {
		t.Fatal("POP3 deleted server copy")
	}
}

func TestSMTPPrivacyAndAmbiguousSubmission(t *testing.T) {
	permitFixture(t)
	for _, drop := range []bool{false, true} {
		var commands atomic.Int32
		received := make(chan []byte, 2)
		s, roots := scriptServer(t, true, smtpScript(received, &commands, drop))
		p := &protocolClient{account: Account{Address: "test@example.org", Outgoing: s, Password: "fixture"}, roots: roots}
		_, err := p.Send(context.Background(), Outgoing{To: []Address{{Email: "a@example.com"}}, Bcc: []Address{{Email: "hidden@example.net"}}, Subject: "test", Text: "body"})
		if drop && !errors.Is(err, ErrSendUnknown) || !drop && err != nil {
			t.Fatalf("drop=%v: %v", drop, err)
		}
		data := <-received
		if strings.Contains(string(data), "hidden@example.net") || len(received) != 0 || commands.Load() != 4 {
			t.Fatal("privacy violation or duplicate submission")
		}
	}
}

func TestTLSAndSSRFBarriersBeforeCredentials(t *testing.T) {
	p := &protocolClient{account: Account{Provider: "imap", Incoming: Server{Host: "127.0.0.1", Port: 993, Security: "tls"}, Password: "do-not-send"}}
	if _, err := p.imap(context.Background()); !errors.Is(err, ErrBlocked) {
		t.Fatalf("loopback reached: %v", err)
	}
	permitFixture(t)
	var credentials atomic.Int32
	s, _ := scriptServer(t, true, func(conn net.Conn) {
		reader := bufio.NewReader(conn)
		if _, err := reader.ReadString('\n'); err == nil {
			credentials.Add(1)
		}
	})
	p.account.Outgoing = s
	if _, err := p.smtp(context.Background()); !errors.Is(err, ErrTLS) {
		t.Fatalf("untrusted certificate accepted: %v", err)
	}
	if credentials.Load() != 0 {
		t.Fatal("credentials sent before TLS validation")
	}
	plain, _ := scriptServer(t, false, func(conn net.Conn) {
		c := textproto.NewConn(conn)
		_ = c.PrintfLine("220 test ESMTP")
		for {
			line, err := c.ReadLine()
			if err != nil {
				return
			}
			if strings.HasPrefix(line, "AUTH ") {
				credentials.Add(1)
			}
			_ = c.PrintfLine("250-test\r\n250 AUTH PLAIN") // No STARTTLS capability.
		}
	})
	p.account.Outgoing = plain
	if _, err := p.smtp(context.Background()); !errors.Is(err, ErrTLS) {
		t.Fatalf("STARTTLS downgrade accepted: %v", err)
	}
	if credentials.Load() != 0 {
		t.Fatal("credentials leaked over cleartext")
	}
}

func TestIMAPReadPaginationFlagsAndMailboxIdentity(t *testing.T) {
	permitFixture(t)
	config, roots := tlsFixture(t)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", config)
	if err != nil {
		t.Fatal(err)
	}
	backend := imapmemserver.New()
	user := imapmemserver.NewUser("test@example.org", "fixture")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	if err := user.Create("Archive", nil); err != nil {
		t.Fatal(err)
	}
	backend.AddUser(user)
	server := imapserver.New(&imapserver.Options{NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
		return backend.NewSession(), nil, nil
	}})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	p := &protocolClient{account: Account{Provider: "imap", Address: "test@example.org", Incoming: serverAddress(t, listener.Addr(), "tls"), Password: "fixture"}, roots: roots}
	c, err := p.imap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 52; i++ {
		raw := fmt.Sprintf("From: Sender <sender@example.net>\r\nTo: test@example.org\r\nSubject: Message %d\r\n\r\nbody %d\r\n", i, i)
		cmd := c.Append("INBOX", int64(len(raw)), nil)
		if _, err := io.WriteString(cmd, raw); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	_ = c.Close()
	folders, err := p.Folders(context.Background())
	if err != nil || len(folders) != 2 {
		t.Fatalf("folders: %+v %v", folders, err)
	}
	first, err := p.Messages(context.Background(), "INBOX", "")
	if err != nil || len(first.Items) != 50 || first.NextCursor == "" || first.Items[0].Subject != "Message 52" {
		t.Fatalf("first page: count=%d cursor=%s err=%v", len(first.Items), first.NextCursor, err)
	}
	second, err := p.Messages(context.Background(), "INBOX", first.NextCursor)
	if err != nil || len(second.Items) != 2 || second.NextCursor != "" {
		t.Fatalf("second page: %+v %v", second, err)
	}
	id := first.Items[0].ID
	m, err := p.Message(context.Background(), "INBOX", id)
	if err != nil || !strings.Contains(m.Text, "body 52") || m.Seen {
		t.Fatalf("peek changed state or lost body: %+v %v", m, err)
	}
	if err := p.Action(context.Background(), Action{FolderID: "INBOX", MessageID: id, Action: "read"}); err != nil {
		t.Fatal(err)
	}
	m, err = p.Message(context.Background(), "INBOX", id)
	if err != nil || !m.Seen {
		t.Fatalf("read flag not synchronized: %+v %v", m, err)
	}
	if err := p.Action(context.Background(), Action{FolderID: "INBOX", MessageID: id, Action: "move", Destination: "Archive"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsafe global expunge fallback: %v", err)
	}
	if err := user.Create("Sent", nil); err != nil {
		t.Fatal(err)
	}
	var commands atomic.Int32
	received := make(chan []byte, 2)
	outgoing, _ := scriptServer(t, true, smtpScript(received, &commands, false))
	p.account.Outgoing = outgoing
	sent, err := p.Send(context.Background(), Outgoing{To: []Address{{Email: "recipient@example.com"}}, Bcc: []Address{{Email: "hidden@example.com"}}, Subject: "copy to Sent", Text: "saved after acceptance"})
	if err != nil || sent.SentCopySaved == nil || !*sent.SentCopySaved {
		t.Fatalf("Sent copy not saved: %+v %v", sent, err)
	}
	<-received
	sentPage, err := p.Messages(context.Background(), "Sent", "")
	if err != nil || len(sentPage.Items) != 1 || !sentPage.Items[0].Seen {
		t.Fatalf("Sent folder: %+v %v", sentPage, err)
	}
	copyMessage, err := p.Message(context.Background(), "Sent", sentPage.Items[0].ID)
	if err != nil || copyMessage.Subject != "copy to Sent" {
		t.Fatalf("Sent content: %+v %v", copyMessage, err)
	}
	// Losing the IMAP Sent folder after SMTP acceptance is still a successful
	// send, with an explicit warning. It must not become a retryable send error.
	if err := user.Delete("Sent"); err != nil {
		t.Fatal(err)
	}
	sent, err = p.Send(context.Background(), Outgoing{To: []Address{{Email: "recipient@example.com"}}, Text: "accepted without copy"})
	if err != nil || sent.SentCopySaved == nil || *sent.SentCopySaved {
		t.Fatalf("SMTP success confused with Sent failure: %+v %v", sent, err)
	}
	<-received
	if err := user.Delete("INBOX"); err != nil {
		t.Fatal(err)
	}
	if err := user.Create("INBOX", &imap.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Message(context.Background(), "INBOX", id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale UID reused in recreated mailbox: %v", err)
	}
}

func TestIMAPOAuthUsesConsentTokenWithoutPasswordFallback(t *testing.T) {
	permitFixture(t)
	var verified atomic.Bool
	s, roots := scriptServer(t, true, func(conn net.Conn) {
		c := textproto.NewConn(conn)
		_ = c.PrintfLine("* OK [CAPABILITY IMAP4rev1 AUTH=XOAUTH2 SASL-IR] fixture")
		for {
			line, err := c.ReadLine()
			if err != nil {
				return
			}
			parts := strings.Fields(line)
			switch {
			case len(parts) == 4 && parts[1] == "AUTHENTICATE" && parts[2] == "XOAUTH2":
				payload, decodeErr := base64.StdEncoding.DecodeString(parts[3])
				if decodeErr == nil && string(payload) == "user=test@example.org\x01auth=Bearer fixture-consented-token\x01\x01" {
					verified.Store(true)
				}
				_ = c.PrintfLine("%s OK authenticated", parts[0])
			case len(parts) >= 2 && parts[1] == "CAPABILITY":
				_ = c.PrintfLine("* CAPABILITY IMAP4rev1 AUTH=XOAUTH2 SASL-IR\r\n%s OK capabilities", parts[0])
			default:
				return
			}
		}
	})
	p := &protocolClient{account: Account{Provider: "imap", Address: "test@example.org", Incoming: s, AccessToken: "fixture-consented-token", Password: "never-fallback"}, roots: roots}
	c, err := p.imap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	if !verified.Load() {
		t.Fatal("OAuth consent token was not used")
	}
}
