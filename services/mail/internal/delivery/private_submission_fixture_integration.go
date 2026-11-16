// SPDX-License-Identifier: Apache-2.0
//go:build integration

package delivery

import (
	"crypto/tls"
	"crypto/x509"
	"log/slog"
)

// NewPrivateFixtureSender is absent from production builds. It trusts only the
// explicit disposable test CA and retains the normal strict private SMTP path.
func NewPrivateFixtureSender(logger *slog.Logger, host, port string, roots *x509.CertPool) *SMTPSender {
	sender := NewSMTPSenderConRelay(logger, &Relay{Host: host, Port: port})
	sender.privateTLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}
	return sender
}
