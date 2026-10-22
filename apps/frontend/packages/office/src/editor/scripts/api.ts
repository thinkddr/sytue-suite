/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * API `sytue.v1` de los scripts, vista desde el anfitrión: traduce cada llamada del
 * sandbox a UNA de tres cosas —despachar un comando del registro común, leer un dato que
 * el manifiesto declara, o un rechazo con motivo— y no hace nada más. Es la superficie
 * estable y versionada (`v1`); añadir operaciones es compatible, quitar o cambiar una no.
 *
 * `run` pasa por los MISMOS validadores que reproducen una macro (`validarPaso`: comando
 * existente, válido para este tipo de documento, permiso de ahora, valor admitido) y,
 * además, por el manifiesto: el comando ha de figurar en él, y si edita, el manifiesto ha
 * de declarar escritura y la selección ha de caer en el ámbito. Esto NO sustituye a la
 * protección del servidor: lo despachado va por los dispatch de siempre y acaba en
 * `/ops`, que reautoriza ACL y protección en cada operación. PURO.
 */
import { COMANDOS, esComando, esComandoDeScript } from '../comandos';
import { MAX_VALOR, validarPaso } from '../macros';
import type { EntornoMacro, MacroStep } from '../macros';
import { celdasDe, dentroDelAmbito } from './manifiesto';
import type { Lectura, ManifiestoScript } from './manifiesto';
import { LIMITES } from './protocolo';
import type { Escalar, Operacion } from './protocolo';

export type CodigoRechazo =
  | 'args' // argumentos de forma o tipo incorrectos
  | 'comando_no_permitido' // no figura en el manifiesto
  | 'comando_vetado' // existe, pero los scripts no pueden usarlo (IA, diálogos, compartir…)
  | 'comando_invalido' // el registro, el tipo de documento o el valor no lo admiten
  | 'solo_lectura' // edita y ahora no se puede editar
  | 'escritura_no_declarada' // edita y el manifiesto no declara escritura
  | 'lectura_no_declarada' // lee algo que el manifiesto no declara
  | 'ambito' // fuera de las hojas o rangos del manifiesto
  | 'limite_datos' // pide más datos de los admitidos
  | 'no_disponible'; // el anfitrión no ofrece esa operación aquí

export type Peticion =
  | { tipo: 'despachar'; paso: MacroStep }
  | { tipo: 'seleccionar'; hoja: string; ref: string }
  | { tipo: 'leer'; que: Lectura; operacion: Operacion; hoja?: string; ref?: string }
  | { tipo: 'rechazo'; codigo: CodigoRechazo; mensaje: string };

export interface ContextoApi {
  manifiesto: ManifiestoScript;
  entorno: EntornoMacro;
  /** Selección actual de la hoja (para acotar la escritura por ámbito). */
  seleccion?: () => { hoja: string; ref: string } | null;
  /** El anfitrión puede mover la selección (solo hojas de cálculo). */
  puedeSeleccionar?: boolean;
}

const no = (codigo: CodigoRechazo, mensaje: string): Peticion => ({
  tipo: 'rechazo',
  codigo,
  mensaje,
});

const cadena = (x: Escalar | undefined): x is string => typeof x === 'string';

/**
 * ¿Caben en el ámbito TODAS las celdas que `fillcells` va a escribir? Sus destinos
 * son las claves de `values`. Lo que no se pueda leer se da por fuera: no se despacha
 * algo cuyo destino no se ha podido comprobar.
 */
function destinosDentro(
  valor: string | undefined,
  hoja: string,
  ambito: Parameters<typeof dentroDelAmbito>[2],
): boolean {
  let destinos: unknown;
  try {
    destinos = (JSON.parse(valor ?? '{}') as { values?: unknown }).values;
  } catch {
    return false;
  }
  if (destinos === undefined) return true; // sin celdas no escribe nada
  if (destinos === null || typeof destinos !== 'object') return false;
  return Object.keys(destinos).every((ref) => dentroDelAmbito(hoja, ref, ambito));
}

/** Traduce una llamada del sandbox. Nunca lanza. */
export function traducirLlamada(
  op: Operacion,
  args: readonly Escalar[],
  ctx: ContextoApi,
): Peticion {
  const { manifiesto: m, entorno } = ctx;
  switch (op) {
    case 'run': {
      const [cmd, valor, ...resto] = args;
      if (!cadena(cmd) || resto.length > 0 || (valor !== undefined && !cadena(valor)))
        return no('args', 'run(comando, valor?) espera cadenas.');
      if (valor !== undefined && valor.length > MAX_VALOR)
        return no('args', 'El valor es demasiado grande.');
      if (!m.commands.includes(cmd))
        return no('comando_no_permitido', `El manifiesto no declara el comando «${cmd}».`);
      if (!esComando(cmd)) return no('comando_invalido', `El comando «${cmd}» no existe.`);
      // Segunda barrera: aunque un manifiesto llegara aquí sin validar, lo que el registro no
      // marca para scripts (IA, diálogos, compartir, protección…) no se despacha.
      if (!esComandoDeScript(cmd))
        return no('comando_vetado', `«${cmd}» no está permitido en scripts.`);
      const edita = COMANDOS[cmd].edits || (cmd === 'copy' && valor === 'cut');
      if (edita && !m.write)
        return no('escritura_no_declarada', `«${cmd}» edita y el manifiesto no declara escritura.`);
      const paso: MacroStep =
        valor === undefined
          ? { editor: entorno.kind, cmd }
          : { editor: entorno.kind, cmd, value: valor };
      const motivo = validarPaso(paso, entorno);
      if (motivo === 'soloLectura')
        return no('solo_lectura', 'El documento está en solo lectura para ti.');
      if (motivo) return no('comando_invalido', `«${cmd}» no se puede aplicar aquí (${motivo}).`);
      const acotado = Boolean(m.scope.sheets || m.scope.ranges);
      if (edita && acotado && entorno.kind === 'spreadsheet') {
        const sel = ctx.seleccion?.() ?? null;
        if (!sel || !dentroDelAmbito(sel.hoja, sel.ref, m.scope))
          return no('ambito', 'La selección está fuera del ámbito del manifiesto.');
        // `fillcells` no escribe en la selección: lleva sus celdas de destino en el
        // valor. Con mirar solo la selección, un manifiesto acotado a A1 escribía en
        // cualquier celda de la hoja, y el consentimiento enseñaba «A1».
        if (cmd === 'fillcells' && !destinosDentro(valor, sel.hoja, m.scope))
          return no('ambito', 'Alguna celda de destino está fuera del ámbito del manifiesto.');
      }
      return { tipo: 'despachar', paso };
    }
    case 'select': {
      const [hoja, ref, ...resto] = args;
      if (!cadena(hoja) || !cadena(ref) || resto.length > 0)
        return no('args', 'select(hoja, rango) espera cadenas.');
      if (entorno.kind !== 'spreadsheet' || !ctx.puedeSeleccionar)
        return no('no_disponible', 'Seleccionar solo existe en las hojas de cálculo.');
      if (!m.read.includes('selection') && !m.write)
        return no('lectura_no_declarada', 'Seleccionar exige declarar «selection» o escritura.');
      if (celdasDe(ref) === 0) return no('args', 'Rango no válido.');
      if (!dentroDelAmbito(hoja, ref, m.scope))
        return no('ambito', 'Fuera del ámbito del manifiesto.');
      return { tipo: 'seleccionar', hoja, ref };
    }
    case 'read.cell':
    case 'read.range': {
      const [hoja, ref, ...resto] = args;
      if (!cadena(hoja) || !cadena(ref) || resto.length > 0)
        return no('args', 'La lectura espera (hoja, referencia).');
      if (!m.read.includes('cells'))
        return no('lectura_no_declarada', 'El manifiesto no declara «cells».');
      const n = celdasDe(ref);
      if (n === 0 || (op === 'read.cell' && n !== 1)) return no('args', 'Referencia no válida.');
      if (n > LIMITES.maxCeldas)
        return no('limite_datos', `Máximo ${LIMITES.maxCeldas} celdas por lectura.`);
      if (!dentroDelAmbito(hoja, ref, m.scope))
        return no('ambito', 'Fuera del ámbito del manifiesto.');
      return { tipo: 'leer', que: 'cells', operacion: op, hoja, ref };
    }
    default: {
      const que: Lectura =
        op === 'read.text'
          ? 'text'
          : op === 'read.slides'
            ? 'slides'
            : op === 'read.title'
              ? 'title'
              : 'selection';
      if (args.length > 0) return no('args', 'Esta lectura no lleva argumentos.');
      if (!m.read.includes(que))
        return no('lectura_no_declarada', `El manifiesto no declara «${que}».`);
      return { tipo: 'leer', que, operacion: op };
    }
  }
}
