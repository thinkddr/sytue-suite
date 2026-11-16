//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import "git.sytue.eu/sytue/one/apps/nucleo/webport"

// Register only from init in the owning domain's operation file.
var corePortRevocations webport.PortRevocations
