// SPDX-License-Identifier: Apache-2.0
package authn

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwt"
)

func TestIdentificacionDelTokenLeeAuthTimeAMRYMFA(t *testing.T) {
	cuando := time.Now().Add(-time.Minute).Unix()
	tok, err := jwt.Parse([]byte(fmt.Sprintf(`{"sub":"ada","auth_time":%d,"amr":["pwd","otp"],"mfa_enabled":true}`, cuando)), jwt.WithVerify(false), jwt.WithValidate(false))
	if err != nil {
		t.Fatal(err)
	}
	c := claimsFromToken(tok)
	if c.AuthTime.Unix() != cuando || len(c.AMR) != 2 || c.AMR[1] != "otp" || c.MFAEnabled == nil || !*c.MFAEnabled {
		t.Fatal(c.AuthTime, c.AMR, c.MFAEnabled)
	}
	viejo, err := jwt.Parse([]byte(`{"sub":"ada","mfa_enabled":"true"}`), jwt.WithVerify(false), jwt.WithValidate(false))
	if err != nil {
		t.Fatal(err)
	}
	if c := claimsFromToken(viejo); !c.AuthTime.IsZero() || c.MFAEnabled != nil {
		t.Fatal("un claim ausente o de otro tipo no puede leerse como dato", c.AuthTime, c.MFAEnabled)
	}
}

func TestIdentificacionRecienteConSegundoFactorSiLaCuentaLoTiene(t *testing.T) {
	ahora := time.Now()
	si, no := true, false
	for _, tc := range []struct {
		nombre string
		c      Claims
		quiere bool
	}{
		{"reciente sin MFA en la cuenta", Claims{AuthTime: ahora.Add(-time.Minute), AMR: []string{"pwd"}, MFAEnabled: &no}, true},
		{"reciente con MFA y otp", Claims{AuthTime: ahora.Add(-time.Minute), AMR: []string{"pwd", "otp"}, MFAEnabled: &si}, true},
		{"reciente con MFA y passkey verificada", Claims{AuthTime: ahora.Add(-time.Minute), AMR: []string{"webauthn", "mfa"}, MFAEnabled: &si}, true},
		{"reciente con MFA y solo contraseña", Claims{AuthTime: ahora.Add(-time.Minute), AMR: []string{"pwd"}, MFAEnabled: &si}, false},
		{"token sin mfa_enabled y solo contraseña", Claims{AuthTime: ahora.Add(-time.Minute), AMR: []string{"pwd"}}, false},
		{"token sin mfa_enabled con otp", Claims{AuthTime: ahora.Add(-time.Minute), AMR: []string{"otp"}}, true},
		{"vieja", Claims{AuthTime: ahora.Add(-FrescuraReautenticacion - time.Second), AMR: []string{"otp"}, MFAEnabled: &no}, false},
		{"sin auth_time", Claims{AMR: []string{"otp"}, MFAEnabled: &no}, false},
		{"del futuro", Claims{AuthTime: ahora.Add(2 * time.Minute), AMR: []string{"otp"}, MFAEnabled: &no}, false},
		{"delegado", Claims{AuthTime: ahora.Add(-time.Minute), AMR: []string{"otp"}, MFAEnabled: &no, Delegado: true}, false},
	} {
		if got := tc.c.IdentificacionReciente(ahora); got != tc.quiere {
			t.Errorf("%s: %v", tc.nombre, got)
		}
	}
	ctx := Into(context.Background(), Claims{Subject: "ada", AuthTime: ahora.Add(-time.Minute), AMR: []string{"otp"}, MFAEnabled: &si})
	if prueba, ok := PrivateStorageReauthentication(ctx); !ok || prueba.AuthTime.IsZero() || len(prueba.AMR) != 1 {
		t.Fatal("la prueba de reautenticación no sale de los claims", prueba, ok)
	}
	if _, ok := PrivateStorageReauthentication(context.Background()); ok {
		t.Fatal("sin claims no hay reautenticación")
	}
}

// TestDeviceTrustDelToken: solo los valores que emite Sytue ID se leen como tales, y
// solo `standard` o `managed` dan un dispositivo de confianza. Lo ausente, lo vacío y
// lo desconocido no conceden nada.
func TestDeviceTrustDelToken(t *testing.T) {
	casos := []struct {
		json      string
		valor     string
		confianza bool
	}{
		{`{"sub":"ada","device_trust":"standard"}`, DeviceTrustStandard, true},
		{`{"sub":"ada","device_trust":"managed"}`, DeviceTrustManaged, true},
		{`{"sub":"ada","device_trust":"none"}`, DeviceTrustNone, false},
		{`{"sub":"ada"}`, "", false},
		{`{"sub":"ada","device_trust":"STANDARD"}`, "", false},
		{`{"sub":"ada","device_trust":true}`, "", false},
	}
	for _, c := range casos {
		tok, err := jwt.Parse([]byte(c.json), jwt.WithVerify(false), jwt.WithValidate(false))
		if err != nil {
			t.Fatal(err)
		}
		got := claimsFromToken(tok)
		if got.DeviceTrust != c.valor || got.DispositivoDeConfianza() != c.confianza {
			t.Errorf("%s: device_trust=%q confianza=%v, quiero %q y %v", c.json, got.DeviceTrust, got.DispositivoDeConfianza(), c.valor, c.confianza)
		}
	}
}
