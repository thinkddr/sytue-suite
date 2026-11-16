// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(darwin && !ios) && !(linux && !android) && !windows

package sistema

func masContraste() bool { return false }
