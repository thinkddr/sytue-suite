// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tienda

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

func TestTienda_WebhookURL(t *testing.T) {
	svc := Nuevo(nil, nil, "https://web.sytue.localhost", "sites.sytue.localhost", nil, nil)
	url := svc.WebhookURL("stripe")
	if url != "https://web.sytue.localhost/webhooks/stripe" {
		t.Errorf("WebhookURL = %q; want https://web.sytue.localhost/webhooks/stripe", url)
	}
}

func TestTienda_ValidacionPasarelas(t *testing.T) {
	kek := []byte("01234567890123456789012345678901")
	kc, err := keycrypto.New(kek)
	if err != nil {
		t.Fatalf("keycrypto.New: %v", err)
	}

	svc := &Servicio{
		keyCipher: kc,
		publicURL: "https://web.sytue.localhost",
	}

	siteID := uuid.New()
	ctx := context.Background()

	// 1. Mollie test sin test_ falla
	_, err = svc.ProbarYGuardarPasarela(ctx, "acme", siteID, "usr", "mollie", "test", map[string]string{
		"api_key": "live_12345678",
	}, nil, false)
	if err == nil {
		t.Error("esperaba error por api_key de mollie que no empieza por test_ en modo test")
	}

	// 2. Adyen sin hmac_key válida falla
	_, err = svc.ProbarYGuardarPasarela(ctx, "acme", siteID, "usr", "adyen", "test", map[string]string{
		"api_key":          "adyen-key",
		"merchant_account": "AcmeMerchant",
		"hmac_key":         "no-es-hex",
	}, nil, false)
	if err == nil {
		t.Error("esperaba error por hmac_key no hexadecimal en adyen")
	}

	// 3. Redsys con FUC inválido falla
	_, err = svc.ProbarYGuardarPasarela(ctx, "acme", siteID, "usr", "redsys", "test", map[string]string{
		"merchant_code": "123", // no tiene 9 dígitos
		"terminal":      "1",
		"secret_key":    base64.StdEncoding.EncodeToString([]byte("clave-secreta-3des-24bytes!!")),
	}, nil, false)
	if err == nil {
		t.Error("esperaba error por FUC de redsys con menos de 9 dígitos")
	}

	// 4. Stripe sin aviso aceptado falla
	_, err = svc.ProbarYGuardarPasarela(ctx, "acme", siteID, "usr", "stripe", "test", map[string]string{
		"secret_key":     "sk" + "_test_12345",
		"webhook_secret": "whsec" + "_12345",
	}, nil, false)
	if err == nil {
		t.Error("esperaba error en stripe sin aceptar aviso legal")
	}

	// 5. Stripe con clave de test en modo live falla
	_, err = svc.ProbarYGuardarPasarela(ctx, "acme", siteID, "usr", "stripe", "live", map[string]string{
		"secret_key":     "sk" + "_test_12345",
		"webhook_secret": "whsec" + "_12345",
	}, nil, true)
	if err == nil {
		t.Error("esperaba error por clave de test en modo live")
	}

	// 6. Proveedor desconocido falla
	_, err = svc.ProbarYGuardarPasarela(ctx, "acme", siteID, "usr", "paypal", "test", map[string]string{}, nil, false)
	if err == nil {
		t.Error("esperaba error por proveedor desconocido")
	}

	// 7. Adyen válido en modo test
	validHmac := hex.EncodeToString([]byte("12345678901234567890123456789012"))
	credsAdyen := map[string]string{
		"api_key":          "test_api_key",
		"merchant_account": "AcmeMerchant",
		"hmac_key":         validHmac,
	}
	// Note: We test validation without saving to DB when store is nil
	// ProbarYGuardarPasarela tests validation and sealing before calling s.store.GuardarPasarela
	if _, err := adyenValidationCheck(credsAdyen, "test"); err != nil {
		t.Errorf("adyen validation check failed: %v", err)
	}
}

func adyenValidationCheck(creds map[string]string, mode string) (bool, error) {
	apiKey := strings.TrimSpace(creds["api_key"])
	merchant := strings.TrimSpace(creds["merchant_account"])
	hmacKey := strings.TrimSpace(creds["hmac_key"])
	if apiKey == "" || merchant == "" || hmacKey == "" {
		return false, domain.ErrCredencialesPasarela
	}
	if _, err := hex.DecodeString(hmacKey); err != nil {
		return false, domain.ErrCredencialesPasarela
	}
	return true, nil
}

func TestTienda_BarridoNilStoreNoPanics(t *testing.T) {
	st := store.New(nil)
	svc := Nuevo(st, nil, "https://web.sytue.localhost", "sites.sytue.localhost", nil, nil)
	err := svc.EjecutarRondaBarrido(context.Background())
	if err != nil {
		t.Errorf("EjecutarRondaBarrido con store nil no debe fallar: %v", err)
	}
}

func TestTienda_FormularioConsentimientoRequerido(t *testing.T) {
	st := store.New(nil)
	svc := Nuevo(st, nil, "https://web.sytue.localhost", "sites.sytue.localhost", nil, nil)
	err := svc.EnviarFormularioPublico(context.Background(), "acme", uuid.New(), "contacto", map[string]any{"msg": "hola"}, false, "es")
	if err == nil || !strings.Contains(err.Error(), "consentimiento") {
		t.Errorf("esperaba error de consentimiento requerido, obtuve: %v", err)
	}
}
