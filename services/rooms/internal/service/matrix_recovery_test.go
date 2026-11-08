// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Las dos primeras las generó encodeRecoveryKey de matrix-js-sdk (claves de 32
// bytes a 7 y a 0): la custodia acepta lo que el navegador produce y nada más.
func TestLaCustodiaSoloAdmiteClavesDeRecuperacion(t *testing.T) {
	for _, key := range []string{
		"EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP",
		"EsSz ygLv VP1b xF1C v7kE eBQx MxDP buG5 w25T L3b6 hfyG Kkrd",
	} {
		if !recoveryKeyShape.MatchString(key) {
			t.Errorf("rechaza una clave del SDK: %q", key)
		}
	}
	for _, key := range []string{
		"",
		"hola",
		"EsT0 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP", // 0 no es base58
		"EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP\n",
		" EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP",
	} {
		if recoveryKeyShape.MatchString(key) {
			t.Errorf("admite algo que no es una clave: %q", key)
		}
	}
	// Lo que no tiene la forma ni llega a mirar quién pide.
	if err := (&Service{}).StoreMatrixRecoveryKey(context.Background(), "sub", "hola"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("StoreMatrixRecoveryKey(hola) = %v; quiero ErrInvalidInput", err)
	}
}
