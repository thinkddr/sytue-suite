// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(darwin && !ios) && !(linux && !android)

package sistema

func menosMovimiento() bool { return false }
