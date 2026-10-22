// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// marcarDeInternet pone `com.apple.quarantine`: «0081» es «descargado» y que
// Gatekeeper pregunte; después, la hora y quién lo bajó.
func marcarDeInternet(ruta string) error {
	valor := fmt.Sprintf("0081;%08x;Sytue;", time.Now().Unix())
	return unix.Setxattr(ruta, "com.apple.quarantine", []byte(valor), 0)
}
