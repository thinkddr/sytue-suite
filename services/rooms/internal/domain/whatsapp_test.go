// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

const avisoWhatsApp = `{"object":"whatsapp_business_account","entry":[{"id":"WABA","changes":[{"field":"messages","value":{
  "messaging_product":"whatsapp","metadata":{"display_phone_number":"34910000000","phone_number_id":"1234567890"},
  "contacts":[{"profile":{"name":"Ana García"},"wa_id":"34600111222"}],
  "messages":[
    {"from":"34600111222","id":"wamid.A","timestamp":"1767225600","type":"text","text":{"body":"Hola, ¿abrís mañana?"}},
    {"from":"34600111222","id":"wamid.B","timestamp":"1767225601","type":"image","image":{"caption":"El ticket","id":"m1"}},
    {"from":"34600111222","id":"wamid.C","timestamp":"1767225602","type":"location","location":{"latitude":39.4699,"longitude":-0.3763,"name":"Tienda"}},
    {"from":"no-es-un-numero","id":"wamid.D","type":"text","text":{"body":"x"}},
    {"from":"34600111222","id":"wamid.E","type":"unsupported"}
  ]}},
  {"field":"messages","value":{"metadata":{"phone_number_id":"1234567890"},"statuses":[{"id":"wamid.X","status":"read"}]}}]}]}`

// Only routing and deduplication metadata is retained. Content, media, location
// and contact names never enter the value the service can persist.
func TestParseWhatsAppWebhook(t *testing.T) {
	msgs, err := ParseWhatsAppWebhook([]byte(avisoWhatsApp))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("mensajes = %d, quiero 4: %+v", len(msgs), msgs)
	}
	if m := msgs[0]; m.WaID != "34600111222" ||
		m.PhoneNumberID != "1234567890" || m.MessageID != "wamid.A" || m.At.Unix() != 1767225600 {
		t.Errorf("texto = %+v", m)
	}
	retained := fmt.Sprintf("%+v", msgs)
	for _, private := range []string{"Ana García", "Hola", "El ticket", "Tienda", "39.4699", "m1"} {
		if strings.Contains(retained, private) {
			t.Fatalf("provider content retained: %q", retained)
		}
	}
}

// Sin la firma de Meta con el secreto de la app no se publica nada: la URL del aviso
// no basta para escribir en nombre de un cliente.
func TestValidWhatsAppSignature(t *testing.T) {
	cuerpo := []byte(avisoWhatsApp)
	mac := hmac.New(sha256.New, []byte("secreto-app"))
	mac.Write(cuerpo)
	firma := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !ValidWhatsAppSignature("secreto-app", cuerpo, firma) {
		t.Fatal("la firma buena no pasa")
	}
	for nombre, c := range map[string]struct{ secreto, firma string }{
		"otro secreto":  {"otro", firma},
		"sin prefijo":   {"secreto-app", strings.TrimPrefix(firma, "sha256=")},
		"hex roto":      {"secreto-app", "sha256=zz"},
		"secreto vacío": {"", firma},
	} {
		if ValidWhatsAppSignature(c.secreto, cuerpo, c.firma) {
			t.Errorf("%s: pasa una firma que no vale", nombre)
		}
	}
	if ValidWhatsAppSignature("secreto-app", append(cuerpo, ' '), firma) {
		t.Error("un cuerpo alterado pasa con la firma del original")
	}
}
