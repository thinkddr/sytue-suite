// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validation

import devicevalidation "git.sytue.eu/sytue/one/apps/nucleo/validation"

// NIF dice si s es un NIF español válido (DNI, NIE, NIF especial o de persona
// jurídica), con su letra o dígito de control. La implementación vive en
// `apps/nucleo/validation`, que comparte el ERP de Sytue Office.
func NIF(s string) bool { return devicevalidation.NIF(s) }
