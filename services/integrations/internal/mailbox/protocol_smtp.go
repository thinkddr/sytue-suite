// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mailbox

import (
	"context"
	"errors"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// xoauth2 implements the provider's SASL mechanism using the existing SASL
// interface. A challenge is an authentication failure, never a request to retry
// passwords or expose the provider's token response.
type xoauth2 struct{ username, token string }

func (a xoauth2) Start() (string, []byte, error) {
	if !validHeader(a.username) || !validHeader(a.token) {
		return "", nil, ErrInvalid
	}
	return "XOAUTH2", []byte("user=" + a.username + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}
func (a xoauth2) Next([]byte) ([]byte, error) { return nil, ErrAuth }

func oauthClient(username, token, host string, port int, supports func(string) bool) (sasl.Client, error) {
	if !validHeader(username) || !validHeader(token) {
		return nil, ErrInvalid
	}
	if supports("OAUTHBEARER") {
		return sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{Username: username, Token: token, Host: host, Port: port}), nil
	}
	if supports("XOAUTH2") {
		return xoauth2{username: username, token: token}, nil
	}
	return nil, ErrUnsupported
}

func (p *protocolClient) smtp(ctx context.Context) (*smtp.Client, error) {
	s := p.account.Outgoing
	conn, err := p.dial(ctx, s, true)
	if err != nil {
		return nil, err
	}
	var c *smtp.Client
	if s.Security == "starttls" {
		c, err = smtp.NewClientStartTLS(conn, p.tlsConfig(s.Host))
		if err != nil {
			_ = conn.Close()
			return nil, ErrTLS // No fallback to clear text, even if STARTTLS is absent.
		}
	} else {
		c = smtp.NewClient(conn)
	}
	c.CommandTimeout, c.SubmissionTimeout = protocolTimeout, protocolTimeout
	var auth sasl.Client
	if p.account.AccessToken != "" {
		auth, err = oauthClient(p.username(s), p.account.AccessToken, s.Host, s.Port, c.SupportsAuth)
	} else {
		password := p.account.SMTPPassword
		if password == "" {
			password = p.account.Password
		}
		switch {
		case password == "":
			err = ErrAuth
		case c.SupportsAuth("PLAIN"):
			auth = sasl.NewPlainClient("", p.username(s), password)
		case c.SupportsAuth("LOGIN"):
			auth = sasl.NewLoginClient(p.username(s), password)
		default:
			err = ErrUnsupported
		}
	}
	if err == nil {
		err = c.Auth(auth)
	}
	if err != nil {
		_ = c.Close()
		if errors.Is(err, ErrUnsupported) {
			return nil, err
		}
		return nil, ErrAuth
	}
	return c, nil
}

func (p *protocolClient) Send(ctx context.Context, out Outgoing) (SendResult, error) {
	raw, err := EncodeMessage(p.account.Address, out)
	if err != nil {
		return SendResult{}, err
	}
	c, err := p.smtp(ctx)
	if err != nil {
		return SendResult{}, err
	}
	defer func() { _ = c.Close() }()
	if size, ok := c.MaxMessageSize(); ok && size > 0 && len(raw) > size {
		return SendResult{}, ErrTooLarge
	}
	if err := c.Mail(p.account.Address, &smtp.MailOptions{Size: int64(len(raw))}); err != nil {
		return SendResult{}, protocolError(err)
	}
	for _, recipients := range [][]Address{out.To, out.Cc, out.Bcc} {
		for _, recipient := range recipients {
			if err := c.Rcpt(recipient.Email, nil); err != nil {
				return SendResult{}, protocolError(err)
			}
		}
	}
	w, err := c.Data()
	if err != nil {
		return SendResult{}, protocolError(err)
	}
	// After DATA starts a disconnected socket cannot prove non-delivery. Never
	// retry automatically, nor send a truncated body by closing a failed writer.
	if _, err := w.Write(raw); err != nil {
		return SendResult{}, ErrSendUnknown
	}
	if err := w.Close(); err != nil {
		var rejected *smtp.SMTPError
		if errors.As(err, &rejected) {
			return SendResult{}, ErrUnavailable
		}
		return SendResult{}, ErrSendUnknown
	}
	// Delivery is already accepted. Saving Sent is a separate outcome: failure
	// must never become a send error that encourages duplicate delivery.
	saved := p.saveSent(ctx, raw)
	return SendResult{SentCopySaved: &saved}, nil
}
