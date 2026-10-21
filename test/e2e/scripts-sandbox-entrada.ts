/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * Entrada que compila `scripts-sandbox.mjs` para el navegador: expone lo que prueba, de
 * Docs y de Bases (los dos sobre el mismo runtime, `@sytue/scripts`).
 */
import {
  crearSandboxIframe,
  ejecutarScript,
} from "../../apps/frontend/packages/office/src/editor/scripts/index";
import { ejecutarScriptDeBase } from "../../apps/frontend/apps/bases/src/lib/scripts/anfitrion";
import type { ApiDeScripts } from "../../apps/frontend/apps/bases/src/lib/scripts/anfitrion";
import { crearSandboxIframe as crearSandboxDeBases } from "../../apps/frontend/apps/bases/src/lib/scripts/sandbox";
import { HttpError } from "../../apps/frontend/packages/api-clients/src/core/http";

/* ---------- Bases: una API de mentira que anota lo que el anfitrión le pide ---------- */
const BASE = "00000000-0000-4000-8000-00000000000b";
const T1 = "11111111-1111-4111-8111-111111111111";
const AJENA = "99999999-9999-4999-8999-999999999999";
const R_AJENO = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const CAMPO = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";

const registro = (id: string, table_id: string) => ({
  id,
  table_id,
  data: { [CAMPO]: "SECRETO-DE-OTRA-BASE" },
  links: {},
  rev: 1,
  created_by: "x",
  updated_by: "x",
  created_at: "",
  updated_at: "",
});

async function ejecutarBases(o: {
  codigo: string;
  manifiesto: unknown;
  extra: { prohibido?: boolean };
}) {
  const llamadas: string[] = [];
  const paso = <R>(n: string, r: R, prohibido = false): Promise<R> => {
    llamadas.push(n);
    return prohibido
      ? Promise.reject(
          new HttpError(403, "Forbidden", "/x", '{"code":"forbidden"}'),
        )
      : Promise.resolve(r);
  };
  const api: ApiDeScripts = {
    listTables: () =>
      paso("listTables", [
        {
          id: T1,
          base_id: BASE,
          name: "Clientes",
          position: 0,
          fields: [
            {
              id: CAMPO,
              table_id: T1,
              name: "Nombre",
              kind: "text",
              position: 0,
            },
          ],
        },
      ]),
    listViews: () => paso("listViews", []),
    listRecords: (t) =>
      paso("listRecords", {
        fields: [],
        records: [registro("r", t)],
        role: "editor",
      }),
    getRecord: (id) =>
      paso("getRecord", registro(id, id === R_AJENO ? AJENA : T1)),
    createRecord: (t) =>
      paso("createRecord", registro("nuevo", t), o.extra.prohibido),
    updateRecord: (id) =>
      paso("updateRecord", registro(id, T1), o.extra.prohibido),
    deleteRecord: () => paso("deleteRecord", undefined, o.extra.prohibido),
  };
  const r = await ejecutarScriptDeBase({
    codigo: o.codigo,
    manifiesto: o.manifiesto,
    baseId: BASE,
    api,
    crearSandbox: () => crearSandboxDeBases(document),
  });
  return { r, llamadas };
}

(window as unknown as Record<string, unknown>).SytueScripts = {
  ejecutarScript,
  crearSandbox: () => crearSandboxIframe(document),
  ejecutarBases,
};
