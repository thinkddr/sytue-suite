// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build apple_keychain_qa && darwin && tienda

package main

import (
	"fmt"
	"os"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

func main() {
	if len(os.Args) != 3 {
		os.Exit(2)
	}
	if err := auth.ProbeKeychainApple(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "KEYCHAIN_QA_FAIL", os.Args[1], err)
		os.Exit(1)
	}
	fmt.Println("KEYCHAIN_QA_PASS", os.Args[1])
}
