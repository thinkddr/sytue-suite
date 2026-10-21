// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/*
 * Captura y lectura sin conexión.
 *
 * Lo que se captura sin red NO se pierde: va a una cola en IndexedDB
 * (@sytue/offline, por persona) y sale en cuanto vuelve la conexión. Cada captura
 * lleva su `data.client_id`, y el servicio devuelve la nota que ya tenga con esa
 * marca (Store.CreateNote): reintentar nunca duplica, ni siquiera cuando la
 * primera llegó y lo que se perdió fue la respuesta.
 *
 * La cola se vacía UNA sola vez entre pestañas: con Web Locks, la que no consigue
 * el cerrojo no hace nada (la que lo tiene ya está enviando). Sin Web Locks, el
 * `client_id` sigue impidiendo duplicados.
 *
 * Y para leer sin red: la lista de cuadernos y las notas de cada cuaderno ya
 * abierto se guardan al llegar, y se sirven de aquí cuando la red falla.
 */
import type { Note, NoteInput, Notebook } from '@sytue/api-clients';
import {
  deleteOfflineValue,
  listOfflineValues,
  mutateOfflineValue,
  readOfflineValue,
  writeOfflineValue,
} from '@sytue/offline';

const COLA = 'notes.capturas';
const CUADERNOS = 'notes.cuadernos';
const NOTAS = 'notes.notas';
const CERROJO = 'sytue-notes-capturas';
/** Por aquí se avisan las pestañas de que la cola ha cambiado. */
export const CANAL = 'sytue-notes-capturas';

/** El cuaderno de un día que todavía no existe en el servicio: se crea al enviar. */
export const PREFIJO_DIA = 'dia:';

/**
 * La clave del almacén: persona Y organización. El `sub` es el mismo en todas las
 * organizaciones y todas viven en el mismo origen; sin la organización, lo
 * capturado en una saldría con el token de otra (y en su diario).
 */
export function claveAlmacen(sub: string, tenant: string | undefined): string {
  return `${sub}|${tenant ?? ''}`;
}

export interface CapturaPendiente {
  client_id: string;
  /** Cuaderno destino; vacío si es un día que hay que abrir antes (`date`). */
  notebook_id?: string;
  date?: string;
  /** Título del cuaderno del día, ya traducido, por si hay que crearlo. */
  title?: string;
  input: NoteInput;
  at: number;
  /** El servicio la rechazó (4xx): no se reintenta; se enseña para descartarla. */
  rechazada?: boolean;
}

export interface Envio {
  createNote: (notebookId: string, input: NoteInput) => Promise<Note>;
  dailyNotebook: (date: string, title: string, carry: boolean) => Promise<Notebook>;
  hoy: string;
}

/** Marca la entrada con su `client_id` sin perder lo que ya traiga `data`. */
export function conClientId(input: NoteInput, clientId: string): NoteInput {
  return { ...input, data: { ...input.data, client_id: clientId } };
}

/** Un 4xx que no se arregla reintentando. 401 (sesión), 408 y 429 sí se arreglan. */
export function esRechazo(err: unknown): boolean {
  const status = (err as { status?: unknown } | null)?.status;
  return (
    typeof status === 'number' &&
    status >= 400 &&
    status < 500 &&
    status !== 401 &&
    status !== 408 &&
    status !== 429
  );
}

/** `fetch` rechaza con TypeError cuando no hay red: es lo único que se encola al vuelo. */
export function esFalloDeRed(err: unknown): boolean {
  return err instanceof TypeError;
}

function avisar(): void {
  if (typeof BroadcastChannel === 'undefined') return;
  const c = new BroadcastChannel(CANAL);
  c.postMessage('cambio');
  c.close();
}

export async function encolar(owner: string, c: CapturaPendiente): Promise<void> {
  await writeOfflineValue(owner, COLA, c.client_id, c);
  avisar();
}

export async function descartar(owner: string, clientId: string): Promise<void> {
  await deleteOfflineValue(owner, COLA, clientId);
  avisar();
}

/** Las pendientes, de la más antigua a la más reciente: es el orden en que se escribieron. */
export async function pendientes(owner: string): Promise<CapturaPendiente[]> {
  const r = await listOfflineValues<CapturaPendiente>(owner, COLA);
  return r.map((x) => x.value).sort((a, b) => a.at - b.at);
}

/**
 * Envía la cola en orden. Para en el primer fallo de red (el resto fallaría igual)
 * y aparta las rechazadas. Devuelve los cuadernos que han recibido notas, o null
 * si otra pestaña ya la estaba enviando.
 */
export async function vaciar(owner: string, envio: Envio): Promise<Set<string> | null> {
  const locks = (globalThis.navigator as Navigator | undefined)?.locks;
  if (locks) {
    return locks.request(CERROJO, { ifAvailable: true }, (lock) =>
      lock ? enviarTodas(owner, envio) : null,
    );
  }
  return enviarTodas(owner, envio);
}

async function enviarTodas(owner: string, envio: Envio): Promise<Set<string>> {
  const tocados = new Set<string>();
  let cambio = false;
  for (const c of await pendientes(owner)) {
    if (c.rechazada) continue;
    try {
      let nb = c.notebook_id;
      if (!nb) {
        const dia = await envio.dailyNotebook(
          c.date ?? envio.hoy,
          c.title ?? '',
          c.date === envio.hoy,
        );
        nb = dia.id;
        tocados.add(`${PREFIJO_DIA}${c.date ?? envio.hoy}`);
      }
      await envio.createNote(nb, c.input);
      await deleteOfflineValue(owner, COLA, c.client_id);
      tocados.add(nb);
      cambio = true;
    } catch (err) {
      if (!esRechazo(err)) break;
      await mutateOfflineValue<CapturaPendiente>(owner, COLA, c.client_id, (prev) =>
        prev ? { ...prev, rechazada: true } : null,
      );
      cambio = true;
    }
  }
  if (cambio) avisar();
  return tocados;
}

// --- Lectura sin conexión ----------------------------------------------------

export function guardarCuadernos(owner: string, lista: Notebook[]): Promise<void> {
  return writeOfflineValue(owner, CUADERNOS, 'lista', lista);
}

export function leerCuadernos(owner: string): Promise<Notebook[] | null> {
  return readOfflineValue<Notebook[]>(owner, CUADERNOS, 'lista');
}

export function guardarNotas(owner: string, notebookId: string, notas: Note[]): Promise<void> {
  return writeOfflineValue(owner, NOTAS, notebookId, notas);
}

export function leerNotas(owner: string, notebookId: string): Promise<Note[] | null> {
  return readOfflineValue<Note[]>(owner, NOTAS, notebookId);
}

/**
 * El cuaderno de un día SIN red: el que ya se conozca de la lista guardada o, si
 * ese día aún no tiene cuaderno, uno provisional donde lo capturado se encola con
 * su fecha y se crea al volver la red (POST /daily).
 */
export function diaSinConexion(
  cuadernos: readonly Notebook[],
  owner: string,
  date: string,
  title: string,
): Notebook {
  const ahora = new Date().toISOString();
  return (
    cuadernos.find((n) => n.daily_date === date && n.owner_sub === owner) ?? {
      id: `${PREFIJO_DIA}${date}`,
      owner_sub: owner,
      title,
      daily_date: date,
      color: '',
      layout: 'list',
      visibility: 'personal',
      indexable: false,
      encrypted: false,
      position: 0,
      note_count: 0,
      created_at: ahora,
      updated_at: ahora,
    }
  );
}
