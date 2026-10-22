// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tienda

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pagos"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// WEB-03. Antes de tocar nada con una notificación, el estado que da la pasarela ha de ser
// el del pago esperado: su importe, su id en los metadatos, el modo y el comercio de la
// pasarela guardada.
func TestVerificarPagoContraLoGuardado(t *testing.T) {
	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Servicio{keyCipher: kc}
	pasarela := func(proveedor string, creds map[string]string) *domain.Gateway {
		t.Helper()
		b, _ := json.Marshal(creds)
		enc, err := kc.Encrypt(string(b))
		if err != nil {
			t.Fatal(err)
		}
		return &domain.Gateway{Provider: proveedor, Mode: pagos.ModoPrueba, CredentialsEnc: enc}
	}
	pago := &domain.Payment{ID: uuid.New(), AmountCents: 250}
	estado := func(comercio string, cambia func(*pagos.Estado)) pagos.Estado {
		e := pagos.Estado{
			Estado:    pagos.EstadoPagado,
			Importe:   250,
			Divisa:    pagos.DivisaEUR,
			Modo:      pagos.ModoPrueba,
			Comercio:  comercio,
			Metadatos: map[string]string{pagos.MetadatoPago: pago.ID.String()},
		}
		if cambia != nil {
			cambia(&e)
		}
		return e
	}

	mollie := pasarela("mollie", map[string]string{"api_key": "test_x", "profile_id": "pfl_tienda"})
	if err := s.verificarPago(pago, mollie, estado("pfl_tienda", nil)); err != nil {
		t.Fatalf("el pago esperado no casa: %v", err)
	}
	for nombre, cambia := range map[string]func(*pagos.Estado){
		"otro importe":  func(e *pagos.Estado) { e.Importe = 1 },
		"otro pago":     func(e *pagos.Estado) { e.Metadatos[pagos.MetadatoPago] = uuid.NewString() },
		"otro modo":     func(e *pagos.Estado) { e.Modo = pagos.ModoReal },
		"otra divisa":   func(e *pagos.Estado) { e.Divisa = "USD" },
		"otro comercio": func(e *pagos.Estado) { e.Comercio = "pfl_otro" },
	} {
		if s.verificarPago(pago, mollie, estado("pfl_tienda", cambia)) == nil {
			t.Errorf("Mollie, %s: casa y no debería", nombre)
		}
	}
	// Sin perfil guardado, el de la propia lectura (la clave solo ve los pagos de su perfil).
	if err := s.verificarPago(pago, pasarela("mollie", map[string]string{"api_key": "test_x"}), estado("pfl_cualquiera", nil)); err != nil {
		t.Errorf("Mollie sin perfil guardado: %v", err)
	}
	adyen := pasarela("adyen", map[string]string{"merchant_account": "TiendaECOM"})
	if s.verificarPago(pago, adyen, estado("TiendaECOM", nil)) != nil || s.verificarPago(pago, adyen, estado("OtraECOM", nil)) == nil {
		t.Error("Adyen no compara la cuenta de comercio con la guardada")
	}
	redsys := pasarela("redsys", map[string]string{"merchant_code": "123456789", "terminal": "1"})
	if s.verificarPago(pago, redsys, estado("123456789:1", nil)) != nil || s.verificarPago(pago, redsys, estado("999999999:1", nil)) == nil {
		t.Error("Redsys no compara FUC y terminal con los guardados")
	}
}
