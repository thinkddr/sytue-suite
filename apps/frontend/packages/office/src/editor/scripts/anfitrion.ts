/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Anfitrión de la ejecución de un script EN DOCS. El bucle que arranca el sandbox y hace
 * cumplir los límites (tiempo, operaciones, ritmo, tamaño, salida, forma de los mensajes)
 * es el común, `ejecutarAislado` de `@sytue/scripts`. Aquí vive lo que es de Docs: validar
 * el manifiesto, traducir cada llamada con `traducirLlamada` (registro común de comandos
 * y validadores de las macros) y despacharla por el editor.
 *
 * Cualquier llamada denegada por la API aborta la ejecución (`denied`/<motivo>): un
 * script que intenta lo que no puede no sigue como si nada.
 */
import { ejecutarAislado } from '@sytue/scripts';
import type { Atencion, Resultado, ResultadoScript, Sandbox } from '@sytue/scripts';

import { traducirLlamada } from './api';
import type { ContextoApi } from './api';
import { validarManifiesto } from './manifiesto';
import type { ManifiestoScript } from './manifiesto';
import { OPERACIONES } from './protocolo';
import type { MacroStep } from '../macros';

export type { Resultado, ResultadoScript, Sandbox };

/** Lo que el editor ofrece a los scripts. Cada función es opcional según el tipo de documento. */
export interface Anfitrion {
  despachar: (paso: MacroStep) => void | Promise<void>;
  seleccionar?: (hoja: string, ref: string) => void;
  seleccion?: () => { hoja: string; ref: string } | null;
  /** `rango`: la lectura es de varias celdas (matriz); si no, de una (escalar). */
  leer?: (
    que: 'cells' | 'text' | 'slides' | 'selection' | 'title',
    hoja?: string,
    ref?: string,
    rango?: boolean,
  ) => unknown;
}

export interface OpcionesEjecucion {
  codigo: string;
  /** Manifiesto de origen no fiable: se vuelve a validar aquí. */
  manifiesto: unknown;
  contexto: Omit<ContextoApi, 'manifiesto' | 'seleccion' | 'puedeSeleccionar'>;
  anfitrion: Anfitrion;
  crearSandbox: () => Sandbox;
  /** `false` si nadie ha pulsado «ejecutar» (disparo automático): exige `unattended`. */
  interactivo?: boolean;
  ahora?: () => number;
}

export function ejecutarScript(o: OpcionesEjecucion): Promise<ResultadoScript> {
  const v = validarManifiesto(o.manifiesto);
  const vacio = (outcome: Resultado, errorCode: string, message: string): ResultadoScript => ({
    outcome,
    errorCode,
    message,
    durationMs: 0,
    ops: 0,
    output: [],
  });
  if (!v.ok) return Promise.resolve(vacio('denied', 'manifiesto_invalido', v.error));
  const manifiesto: ManifiestoScript = v.manifiesto;
  if (o.interactivo === false && !manifiesto.unattended)
    return Promise.resolve(
      vacio('denied', 'no_interactivo', 'El manifiesto no admite ejecución sin interacción.'),
    );

  const ctx: ContextoApi = {
    ...o.contexto,
    manifiesto,
    ...(o.anfitrion.seleccion ? { seleccion: o.anfitrion.seleccion } : {}),
    puedeSeleccionar: o.anfitrion.seleccionar !== undefined,
  };
  const valor = (v: unknown): Atencion => ({ tipo: 'valor', valor: v });

  return ejecutarAislado({
    codigo: o.codigo,
    operaciones: OPERACIONES,
    maxOps: manifiesto.max_ops,
    maxMs: manifiesto.max_ms,
    crearSandbox: o.crearSandbox,
    ...(o.ahora ? { ahora: o.ahora } : {}),
    atender: async (op, args) => {
      const p = traducirLlamada(op, args, ctx);
      if (p.tipo === 'rechazo') return p;
      if (p.tipo === 'despachar') {
        await o.anfitrion.despachar(p.paso);
        return valor(null);
      }
      if (p.tipo === 'seleccionar') {
        o.anfitrion.seleccionar!(p.hoja, p.ref);
        return valor(null);
      }
      const bruto = o.anfitrion.leer
        ? await o.anfitrion.leer(p.que, p.hoja, p.ref, p.operacion === 'read.range')
        : undefined;
      return bruto === undefined
        ? {
            tipo: 'rechazo',
            codigo: 'no_disponible',
            mensaje: 'Este editor no ofrece esa lectura.',
          }
        : valor(bruto);
    },
  });
}
