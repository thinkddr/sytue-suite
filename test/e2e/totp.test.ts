/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `codigoTOTP` contra los vectores del RFC 6238 (apéndice B, HMAC-SHA1). El RFC da
 * ocho cifras; con seis, que es lo que pide Sytue ID, son sus seis últimas. Si este
 * cálculo se tuerce, todas las sondas con segundo factor se quedan en `/login/mfa`
 * sin decir por qué. Ejecutar: `bun run test` desde test/e2e.
 */
import { expect, test } from "bun:test";
import { codigoTOTP } from "./tests/helpers.ts";

// "12345678901234567890" en base32: la semilla SHA1 del apéndice B.
const SEMILLA = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ";

test.each([
  [59, "94287082"],
  [1111111109, "07081804"],
  [1111111111, "14050471"],
  [1234567890, "89005924"],
  [2000000000, "69279037"],
  [20000000000, "65353130"],
])("RFC 6238 en T=%d", (segundos, rfc) => {
  expect(codigoTOTP(SEMILLA, segundos * 1000)).toBe(rfc.slice(-6));
});

test("un código de seis cifras en lugar de la semilla se rechaza con su porqué", () => {
  expect(() => codigoTOTP("287082")).toThrow(/SEMILLA/);
});
