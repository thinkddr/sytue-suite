// SPDX-License-Identifier: Apache-2.0
// Package limits owns Mail's native message-size policy without HTTP or UI.
package limits

import "git.sytue.eu/sytue/one/apps/nucleo/storage"

func MessageBytes(value int64) (int64, error) {
	if value == 0 {
		value = 50 << 20
	}
	if value < 0 || value > storage.MaxFileBytes {
		return 0, storage.ErrRecord
	}
	return value, nil
}
