/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Bucle del anfitrión de un script: el lado de la página, COMÚN a los productos. Arranca
 * el sandbox, le pasa el código, entrega cada llamada al `atender` del producto y APLICA
 * los límites. Es la autoridad: el iframe y el Worker son hostiles, y todo plazo y tope se
 * hace cumplir AQUÍ (matar el iframe mata al Worker), no dentro.
 *
 * Qué pone cada producto: su lista cerrada de operaciones, las cotas de ESTA ejecución
 * (`maxOps`, `maxMs`) y `atender`, que traduce una llamada ya validada en forma a un valor,
 * un rechazo o un límite. Qué NO puede cambiar: nada de lo de abajo.
 *
 * Reglas, todas con terminación forzosa y resultado distinto:
 *  - tiempo: pasado `maxMs` se destruye el sandbox (`timeout`);
 *  - operaciones: más de `maxOps` llamadas (`limit`/`max_ops`);
 *  - mensajes: más de LIMITES.maxPorSegundo por segundo, o más de maxPendientes sin
 *    contestar (`limit`/`rate`, `limit`/`pendientes`); mensaje demasiado grande (`limit`/`mensaje`);
 *  - salida: más de LIMITES.maxSalida caracteres (`limit`/`salida`);
 *  - mensaje con forma inválida, prototipo manipulado u operación fuera de la lista
 *    (`denied`/`mensaje_invalido`);
 *  - cualquier llamada que el producto rechaza aborta la ejecución (`denied`/<motivo>): un
 *    script que intenta lo que no puede no sigue como si nada;
 *  - lo que se devuelve al script pasa SIEMPRE por `sanearDato` (`limit`/`limite_datos` si
 *    no cabe): el producto no puede saltárselo.
 * La memoria no se puede medir desde fuera de un Worker de otro origen: la acota el
 * tiempo, más los topes de salida y de datos devueltos. ponytail: sin tope de bytes del
 * heap del Worker; si hiciera falta, un Worker por ejecución en un proceso aparte.
 */
import { LIMITES, leerMensaje, sanearDato } from './protocolo';
import type { Escalar, MensajeDeSandbox, Respuesta } from './protocolo';

export type Resultado = 'ok' | 'error' | 'denied' | 'timeout' | 'limit';

export interface ResultadoScript {
  outcome: Resultado;
  /** Código corto y estable (`a-z0-9_.-`): lo que se audita. Nunca texto del script. */
  errorCode: string;
  /** Texto para quien ejecutó; puede citar al script, no se audita. */
  message: string;
  durationMs: number;
  ops: number;
  output: string[];
}

/** Lo que el anfitrión necesita del sandbox (iframe real, o un doble en las pruebas). */
export interface Sandbox {
  /** Registra el receptor de mensajes del sandbox (datos sin validar). */
  recibir: (fn: (datos: unknown) => void) => void;
  enviar: (mensaje: unknown) => void;
  /** Mata el sandbox y todo lo que corre dentro. Idempotente. */
  destruir: () => void;
}

/** Lo que el producto contesta a una llamada. */
export type Atencion =
  /** Se devuelve al script (saneado). */
  | { tipo: 'valor'; valor: unknown }
  /** No se permite: aborta la ejecución como `denied`. */
  | { tipo: 'rechazo'; codigo: string; mensaje: string }
  /** Se pasa de un tope del producto: aborta como `limit`. */
  | { tipo: 'limite'; codigo: string; mensaje: string };

export interface OpcionesAisladas<Op extends string> {
  codigo: string;
  /** Lista CERRADA de operaciones del producto. */
  operaciones: readonly Op[];
  maxOps: number;
  maxMs: number;
  /** Atiende una llamada. Si lanza, la ejecución acaba como `error`/`fallo_anfitrion`. */
  atender: (op: Op, args: readonly Escalar[]) => Atencion | Promise<Atencion>;
  crearSandbox: () => Sandbox;
  ahora?: () => number;
}

/** Espera máxima a que el sandbox diga «ready». */
const ARRANQUE_MS = 5000;
const RE_CODIGO = /[^a-z0-9_.-]/g;
const corto = (s: string, n: number): string => s.slice(0, n);

export function ejecutarAislado<Op extends string>(
  o: OpcionesAisladas<Op>,
): Promise<ResultadoScript> {
  const ahora = o.ahora ?? (() => performance.now());
  const inicio = ahora();
  const salida: string[] = [];
  let ops = 0;
  let caracteres = 0;
  let pendientes = 0;
  const marcas: number[] = [];

  return new Promise<ResultadoScript>((resolver) => {
    const sandbox = o.crearSandbox();
    let fin = false;
    let reloj: ReturnType<typeof setTimeout> | undefined;
    // Si el sandbox no llega a decir «ready», no se queda esperando para siempre.
    let arranque: ReturnType<typeof setTimeout> | undefined = setTimeout(
      () => terminar('error', 'sandbox_no_arranca', 'El entorno aislado no arrancó.'),
      ARRANQUE_MS,
    );
    // Las llamadas se atienden de una en una y en orden.
    let cola: Promise<void> = Promise.resolve();

    const terminar = (outcome: Resultado, errorCode: string, message: string): void => {
      if (fin) return;
      fin = true;
      clearTimeout(reloj);
      clearTimeout(arranque);
      sandbox.destruir();
      resolver({
        outcome,
        errorCode: corto(errorCode.toLowerCase().replace(RE_CODIGO, '_'), 64),
        message: corto(message, 500),
        durationMs: Math.max(0, Math.round(ahora() - inicio)),
        ops,
        output: salida,
      });
    };

    const responder = (r: Respuesta): void => {
      if (!fin) sandbox.enviar(r);
    };

    const fallar = (
      seq: number,
      outcome: Resultado,
      code: string,
      message: string,
      detalle = message,
    ): void => {
      responder({ t: 'ret', seq, ok: false, code, message });
      terminar(outcome, code, detalle);
    };

    const atender = async (seq: number, op: Op, args: readonly Escalar[]): Promise<void> => {
      // Un script puede lanzar muchas llamadas sin esperar ninguna. Si una se deniega
      // o salta un tope, la ejecución termina AHÍ: las que ya estaban en cola no se
      // atienden. Sin esto seguían llegando al producto —borrados y altas reales en
      // Bases— después de haber informado de que el script estaba parado.
      if (fin) return;
      let a: Atencion;
      try {
        a = await o.atender(op, args);
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'fallo del editor';
        fallar(seq, 'error', 'fallo_anfitrion', corto(msg, 200), msg);
        return;
      }
      if (a.tipo === 'rechazo') fallar(seq, 'denied', a.codigo, a.mensaje);
      else if (a.tipo === 'limite') fallar(seq, 'limit', a.codigo, a.mensaje);
      else {
        const dato = sanearDato(a.valor);
        if (dato === undefined)
          fallar(seq, 'limit', 'limite_datos', 'Los datos superan el máximo.');
        else responder({ t: 'ret', seq, ok: true, value: dato });
      }
    };

    const alMensaje = (datos: unknown): void => {
      if (fin) return;
      // Tope de ritmo ANTES de mirar el contenido: lo barato primero.
      const t = ahora();
      marcas.push(t);
      while (marcas.length > 0 && t - marcas[0]! > 1000) marcas.shift();
      if (marcas.length > LIMITES.maxPorSegundo) {
        terminar('limit', 'rate', 'Demasiados mensajes por segundo.');
        return;
      }
      const leido = leerMensaje(datos, o.operaciones);
      if (!leido.ok) {
        if (leido.motivo === 'grande') terminar('limit', 'mensaje', 'Mensaje demasiado grande.');
        else terminar('denied', 'mensaje_invalido', 'Mensaje del sandbox con forma no válida.');
        return;
      }
      const m: MensajeDeSandbox<Op> = leido.mensaje;
      switch (m.t) {
        case 'ready':
          if (reloj !== undefined) break; // un segundo «ready» no reinicia nada
          clearTimeout(arranque);
          arranque = undefined;
          sandbox.enviar({ t: 'init', code: o.codigo });
          reloj = setTimeout(
            () => terminar('timeout', 'timeout', `Tiempo máximo agotado (${o.maxMs} ms).`),
            o.maxMs,
          );
          break;
        case 'log':
          caracteres += m.text.length;
          if (caracteres > LIMITES.maxSalida) {
            terminar('limit', 'salida', 'Demasiada salida.');
            break;
          }
          salida.push(m.text);
          break;
        case 'call':
          ops += 1;
          if (ops > o.maxOps) {
            terminar('limit', 'max_ops', `Más de ${o.maxOps} operaciones.`);
            break;
          }
          pendientes += 1;
          if (pendientes > LIMITES.maxPendientes) {
            terminar('limit', 'pendientes', 'Demasiadas llamadas sin esperar.');
            break;
          }
          cola = cola
            .then(() => atender(m.seq, m.op, m.args))
            .finally(() => {
              pendientes -= 1;
            });
          break;
        case 'done':
          // Lo que quede en cola se atiende antes de dar por buena la ejecución.
          void cola.then(() => terminar('ok', '', ''));
          break;
        case 'fail':
          // El iframe avisa así de que cortó al Worker por pasarse de mensajes o de tamaño.
          if (m.name === 'LimitError') {
            terminar('limit', 'rate', 'Demasiados mensajes o demasiado grandes.');
            break;
          }
          void cola.then(() => terminar('error', 'script_error', `${m.name}: ${m.message}`));
          break;
      }
    };
    sandbox.recibir(alMensaje);
  });
}
