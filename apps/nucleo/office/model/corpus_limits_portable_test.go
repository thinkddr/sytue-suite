// SPDX-License-Identifier: Apache-2.0
//go:build !linux

package model

import "os"

// Portable import/export tests still run; the external subprocess benchmark is
// skipped explicitly because these platforms cannot provide its Linux limits.
const corpusProcessLimits = false

func limitarMemoria(uint64)             {}
func memoriaMaxMB(*os.ProcessState) int { return 0 }
