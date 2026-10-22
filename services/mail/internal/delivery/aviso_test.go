// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// El aviso de un correo nuevo abre SU conversación en el host de Mail. Salía como
// `https://mail.sytue.localhost/#msg-<id>`: otro host, sin `/mail` y con un fragmento
// que la web no lee.
func TestElAvisoDeUnCorreoAbreSuConversacion(t *testing.T) {
	hilo := uuid.MustParse("0b6a4c8e-1f2d-4e3a-9b5c-7d8e9f0a1b2c")
	n := avisoDeLlegada("https://mail.sytue.eu", "acme", "ana", domain.Message{
		ID: uuid.New(), ThreadID: hilo, FromAddr: "bea@x.es", FromName: "Bea", Subject: "Presupuesto",
	})
	if n.URL != "https://mail.sytue.eu/mail/acme/?thread="+hilo.String() {
		t.Fatalf("enlace = %q", n.URL)
	}
	if n.Params["from"] != "Bea" || n.Body != "Presupuesto" || n.Subject != "ana" || n.Tenant != "acme" {
		t.Fatalf("aviso = %+v", n)
	}
}
