// SPDX-License-Identifier: Apache-2.0
package application

import (
	"context"
	"testing"

	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type matrixCrypto struct{ privateMail.Crypto }

func TestModoDeBovedaSinNadaDeMatrix(t *testing.T) {
	copyFile := func(context.Context, storage.PublicationTarget, string, storage.Reference, string, string) (storage.Reference, error) {
		return storage.Reference{}, nil
	}
	for name, c := range map[string]struct {
		cfg  Config
		want bool
	}{
		"Matrix sin origen":             {Config{}, true},
		"Matrix con origen":             {Config{RequireLegacy: true, Legacy: matrixCrypto{}}, true},
		"Matrix pide origen y no lo da": {Config{RequireLegacy: true}, false},
		"bóveda":                        {Config{Vault: true}, true},
		"bóveda que pide origen":        {Config{Vault: true, RequireLegacy: true, Legacy: matrixCrypto{}}, false},
		"bóveda con almacén de Matrix":  {Config{Vault: true, Legacy: matrixCrypto{}}, false},
		"bóveda con copiador de Matrix": {Config{Vault: true, CopyFile: copyFile}, false},
	} {
		if got := c.cfg.validMode(); got != c.want {
			t.Errorf("%s: %v; quiero %v", name, got, c.want)
		}
	}
}
