// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mime

import "testing"

func TestParseUnsubscribeOneClick(t *testing.T) {
	raw := []byte("From: news@ex.com\r\n" +
		"List-Unsubscribe: <https://ex.com/u?id=1>, <mailto:unsub@ex.com?subject=stop%20it>\r\n" +
		"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\nhola")
	u := ParseUnsubscribe(raw)
	if u.HTTPS != "https://ex.com/u?id=1" {
		t.Fatalf("HTTPS = %q", u.HTTPS)
	}
	if u.Mailto != "unsub@ex.com" {
		t.Fatalf("Mailto = %q", u.Mailto)
	}
	if u.MailtoSubject != "stop it" {
		t.Fatalf("MailtoSubject = %q", u.MailtoSubject)
	}
	if !u.OneClick {
		t.Fatal("OneClick debería ser true")
	}
	if !u.Has() {
		t.Fatal("Has() debería ser true")
	}
}

// Sin List-Unsubscribe-Post no hay one-click aunque haya URL https.
func TestParseUnsubscribeNoOneClickWithoutPost(t *testing.T) {
	raw := []byte("List-Unsubscribe: <https://ex.com/u>\r\n\r\nx")
	u := ParseUnsubscribe(raw)
	if u.HTTPS != "https://ex.com/u" || u.OneClick {
		t.Fatalf("u = %+v, quiero https sin one-click", u)
	}
}

// http en claro se ignora (no se sigue un enlace inseguro); queda solo el mailto.
func TestParseUnsubscribeIgnoresPlainHTTP(t *testing.T) {
	raw := []byte("List-Unsubscribe: <http://ex.com/u>, <mailto:u@ex.com>\r\n\r\nx")
	u := ParseUnsubscribe(raw)
	if u.HTTPS != "" {
		t.Fatalf("HTTPS = %q, quiero vacío (http en claro se ignora)", u.HTTPS)
	}
	if u.Mailto != "u@ex.com" {
		t.Fatalf("Mailto = %q", u.Mailto)
	}
	if u.OneClick {
		t.Fatal("sin https no puede haber one-click")
	}
}

func TestParseUnsubscribeNone(t *testing.T) {
	if u := ParseUnsubscribe([]byte("Subject: hola\r\n\r\nx")); u.Has() {
		t.Fatalf("mensaje sin cabecera => Has()=false, got %+v", u)
	}
	if u := ParseUnsubscribe([]byte("no es un mensaje")); u.Has() {
		t.Fatalf("basura => vacío, got %+v", u)
	}
}
