// SPDX-License-Identifier: Apache-2.0
//go:build linux

package model

import (
	"os"
	"syscall"
)

const corpusProcessLimits = true

// limitarMemoria pone un tope al espacio de direcciones del proceso: pasado, el runtime de Go
// muere con «out of memory» en vez de que el núcleo mate el contenedor entero.
func limitarMemoria(tope uint64) {
	_ = syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: tope, Max: tope})
}

// memoriaMaxMB es lo más que ocupó en memoria un proceso hijo (Maxrss viene en KiB en Linux).
func memoriaMaxMB(ps *os.ProcessState) int {
	if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
		return int(ru.Maxrss / 1024)
	}
	return 0
}
