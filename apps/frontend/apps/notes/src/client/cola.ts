// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/*
 * Captura y lectura sin conexión.
 *
 * Lo que se captura sin red NO se pierde: va a una cola en IndexedDB
 * (@sytue/offline, por persona). El envío automático está pausado hasta disponer
 * de una confirmación atómica compatible con escritores de versiones anteriores.
 *
 * Y para leer sin red: la lista de cuadernos y las notas de cada cuaderno ya
 * abierto se guardan al llegar, y se sirven de aquí cuando la red falla.
 */
import type { Note, NoteInput, Notebook } from '@sytue/api-clients';
import { admiteEnlacesEnEnvio } from './notesApi.js';
import { cargarHtml } from './html.js';
import {
  deleteOfflineValue,
  listOfflineValues,
  readOfflineValue,
  writeOfflineValue,
} from '@sytue/offline';

const COLA = 'notes.capturas';
const CUADERNOS = 'notes.cuadernos';
const NOTAS = 'notes.notas';
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
  return r
    .map(({ id, value }) => {
      const input = value?.input;
      const data = input?.data;
      const keys =
        data && typeof data === 'object' && !Array.isArray(data)
          ? Object.keys(data).filter(
              (key) => key.toLowerCase().replaceAll('ſ', 's') === 'client_id',
            )
          : [];
      const valid =
        typeof id === 'string' &&
        id !== '' &&
        value?.client_id === id &&
        admiteEnlacesEnEnvio(input) &&
        keys.length === 1 &&
        keys[0] === 'client_id' &&
        data?.client_id === id;
      return valid ? value : { ...value, client_id: id, rechazada: true };
    })
    .sort((a, b) => a.at - b.at);
}

/**
 * El replay está pausado hasta contar con un CAS de IndexedDB que también cerque
 * pestañas antiguas. La fuente queda intacta y no se crean cuadernos ni notas.
 */
export function vaciar(_owner: string, _envio: Envio): Promise<Set<string> | null> {
  return Promise.resolve(new Set());
}

// --- Lectura sin conexión ----------------------------------------------------

export function guardarCuadernos(owner: string, lista: Notebook[]): Promise<void> {
  return writeOfflineValue(owner, CUADERNOS, 'lista', lista);
}

// Lo guardado también espera al parser de HTML: ninguna nota llega a la interfaz antes
// que él (`html.ts`).
export async function leerCuadernos(owner: string): Promise<Notebook[] | null> {
  const [lista] = await Promise.all([
    readOfflineValue<Notebook[]>(owner, CUADERNOS, 'lista'),
    cargarHtml(),
  ]);
  return lista;
}

export function guardarNotas(owner: string, notebookId: string, notas: Note[]): Promise<void> {
  return writeOfflineValue(owner, NOTAS, notebookId, notas);
}

export async function leerNotas(owner: string, notebookId: string): Promise<Note[] | null> {
  const [notas] = await Promise.all([
    readOfflineValue<Note[]>(owner, NOTAS, notebookId),
    cargarHtml(),
  ]);
  return notas;
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
