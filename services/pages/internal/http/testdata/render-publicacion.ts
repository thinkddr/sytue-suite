/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { publicar } from "../../../../../apps/frontend/packages/pages-render/src/exportar/index";
const { ir, datos, base, vista } = JSON.parse(await Bun.stdin.text());
const salida = publicar(ir, datos, {
  base,
  vista,
  hash: (b) => new Bun.CryptoHasher("sha256").update(b).digest("hex"),
});
process.stdout.write(
  JSON.stringify(
    salida.archivos.map((f) => ({
      ruta: f.ruta,
      contenido:
        typeof f.contenido === "string"
          ? f.contenido
          : new TextDecoder().decode(f.contenido),
    })),
  ),
);
