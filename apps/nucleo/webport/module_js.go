//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package webport

import "git.sytue.eu/sytue/one/apps/nucleo/webport/rpc"

// Module is the one RPC transport for the crypto kernel and lazy processors.
// It lives in webport/rpc so the vault core can serve without linking the host
// port protocol; this alias keeps every processor on the same type.
type Module = rpc.Module

func Serve(module Module) { rpc.Serve(module) }
