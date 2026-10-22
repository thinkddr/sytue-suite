// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

const protocolTimeout = 45 * time.Second

type protocolClient struct {
	account Account
	// Tests install their own trust root, never disable certificate validation.
	roots *x509.CertPool
}

// NewProtocolClient uses the suite's SSRF dialer for every TCP connection. No
// insecure transport, proxy, certificate override or automatic send retry exists.
func NewProtocolClient(a Account) Client {
	if a.Provider == "yahoo" {
		a.Incoming = Server{Host: "imap.mail.yahoo.com", Port: 993, Username: a.Address, Security: "tls"}
		a.Outgoing = Server{Host: "smtp.mail.yahoo.com", Port: 465, Username: a.Address, Security: "tls"}
	}
	return &protocolClient{account: a}
}

// boundedConn enforces an overall deadline even when protocol libraries reset
// their command deadlines, and bounds data accepted from an untrusted server.
type boundedConn struct {
	net.Conn
	until     time.Time
	remaining int64
	stop      func() bool
	once      sync.Once
}

func (c *boundedConn) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		return 0, ErrTooLarge
	}
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.Conn.Read(p)
	c.remaining -= int64(n)
	return n, err
}

func (c *boundedConn) bound(t time.Time) time.Time {
	if t.IsZero() || t.After(c.until) {
		return c.until
	}
	return t
}
func (c *boundedConn) SetDeadline(t time.Time) error     { return c.Conn.SetDeadline(c.bound(t)) }
func (c *boundedConn) SetReadDeadline(t time.Time) error { return c.Conn.SetReadDeadline(c.bound(t)) }

func (c *boundedConn) SetWriteDeadline(t time.Time) error { return c.Conn.SetWriteDeadline(c.bound(t)) }

func (c *boundedConn) Close() error {
	c.once.Do(func() {
		if c.stop != nil {
			c.stop()
		}
	})
	return c.Conn.Close()
}

func validServer(s Server, smtp bool) bool {
	if s.Host == "" || s.Host != strings.TrimSpace(s.Host) || strings.ContainsAny(s.Host, "/\\\x00\r\n@%") || s.Port < 1 || s.Port > 65535 || !validHeader(s.Username) {
		return false
	}
	return s.Security == "tls" || smtp && s.Security == "starttls"
}

func protocolError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ssrfhttp.ErrBlocked) {
		return ErrBlocked
	}
	if errors.Is(err, ErrTooLarge) {
		return ErrTooLarge
	}
	var certErr *tls.CertificateVerificationError
	var recordErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &recordErr) {
		return ErrTLS
	}
	return ErrUnavailable
}

func (p *protocolClient) dial(ctx context.Context, s Server, smtp bool) (net.Conn, error) {
	if !validServer(s, smtp) {
		return nil, ErrInvalid
	}
	until := time.Now().Add(protocolTimeout)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(until) {
		until = deadline
	}
	dialCtx, cancel := context.WithDeadline(ctx, until)
	defer cancel()
	if err := ssrfhttp.ValidateHost(dialCtx, s.Host); err != nil {
		return nil, protocolError(err)
	}
	raw, err := ssrfhttp.SafeDialContext(dialCtx, "tcp", net.JoinHostPort(s.Host, strconv.Itoa(s.Port)))
	if err != nil {
		return nil, protocolError(err)
	}
	c := &boundedConn{Conn: raw, until: until, remaining: MaxMessageBytes + (8 << 20)}
	c.stop = context.AfterFunc(ctx, func() { _ = raw.Close() })
	_ = c.SetDeadline(until)
	if s.Security == "starttls" {
		return c, nil
	}
	secure := tls.Client(c, p.tlsConfig(s.Host))
	if err := secure.HandshakeContext(dialCtx); err != nil {
		_ = c.Close()
		return nil, protocolError(err)
	}
	return secure, nil
}

func (p *protocolClient) tlsConfig(host string) *tls.Config {
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, RootCAs: p.roots}
}

func (p *protocolClient) username(s Server) string {
	if s.Username != "" {
		return s.Username
	}
	return p.account.Address
}

func (p *protocolClient) Test(ctx context.Context) (Profile, error) {
	if _, err := parsedAddress(Address{Email: p.account.Address}); err != nil {
		return Profile{}, err
	}
	if p.account.Provider == "pop3" {
		c, err := p.pop(ctx)
		if err != nil {
			return Profile{}, err
		}
		_ = c.Close()
	} else {
		c, err := p.imap(ctx)
		if err != nil {
			return Profile{}, err
		}
		_ = c.Close()
	}
	c, err := p.smtp(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer func() { _ = c.Close() }()
	// This is a connection test: no MAIL, RCPT or DATA command is sent.
	if err := c.Noop(); err != nil {
		return Profile{}, protocolError(err)
	}
	return Profile{Address: p.account.Address}, nil
}

func (p *protocolClient) Folders(ctx context.Context) ([]Folder, error) {
	if p.account.Provider == "pop3" {
		return p.popFolders(ctx)
	}
	return p.imapFolders(ctx)
}

func (p *protocolClient) Messages(ctx context.Context, folder, cursor string) (Page, error) {
	if p.account.Provider == "pop3" {
		return p.popMessages(ctx, folder, cursor)
	}
	return p.imapMessages(ctx, folder, cursor)
}

func (p *protocolClient) Message(ctx context.Context, folder, id string) (Message, error) {
	if p.account.Provider == "pop3" {
		return p.popMessage(ctx, folder, id)
	}
	return p.imapMessage(ctx, folder, id)
}

func (p *protocolClient) Action(ctx context.Context, a Action) error {
	if p.account.Provider == "pop3" {
		return ErrUnsupported
	}
	return p.imapAction(ctx, a)
}
