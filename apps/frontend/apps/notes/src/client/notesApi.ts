/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Llamadas del navegador al BFF de esta app (`/api/*`, cookie de sesión
 * httpOnly). El access token no vive aquí: lo pone el servidor. Todo lo que
 * devuelven estos endpoints ya pasó por la autorización del servicio Go.
 */
import { withBase } from '@sytue/ui';
import { noteWritePath } from '@sytue/api-clients';
import type {
  DailyDay,
  Hashtag,
  NoteLink,
  SharedNote,
  Note,
  NoteInput,
  NoteCipherMigration,
  NoteHit,
  NoteLinkTarget,
  Notebook,
  NotebookInput,
  Reminder,
  ReminderInput,
  Tag,
  TagState,
  Task,
  TaskState,
} from '@sytue/api-clients';

export type {
  DailyDay,
  Hashtag,
  Task,
  TaskState,
  SharedNote,
  Note,
  NoteInput,
  NoteHit,
  NoteLinkTarget,
  Notebook,
  NotebookInput,
  Reminder,
  ReminderInput,
  Tag,
  TagState,
};

/** Error del BFF con su estado, para que la interfaz distinga 404 de 500. */
export class NotesApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(`notes-api: ${status} ${code}`);
    this.name = 'NotesApiError';
  }
}

async function pedir<T>(path: string, init?: RequestInit): Promise<T> {
  // `withBase`: en producción la app se sirve bajo su prefijo (`/notes`) tanto en
  // su subdominio como en el host de la organización. Una ruta absoluta sin él
  // apunta a la raíz del host, que ahí es el 404 de Traefik.
  const res = await fetch(withBase(import.meta.env.BASE_URL, path), {
    credentials: 'include',
    headers: {
      accept: 'application/json',
      ...(init?.body ? { 'content-type': 'application/json' } : {}),
    },
    ...init,
  });
  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    throw new NotesApiError(res.status, body.error ?? 'error');
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
}

const cuerpo = (v: unknown): RequestInit => ({ body: JSON.stringify(v) });

// --- Cuadernos --------------------------------------------------------------

export async function listNotebooks(): Promise<Notebook[]> {
  return (await pedir<{ notebooks: Notebook[] }>('/api/notebooks')).notebooks ?? [];
}

export function getNotebook(id: string): Promise<Notebook> {
  return pedir<Notebook>(`/api/notebooks/${encodeURIComponent(id)}`, { cache: 'no-store' });
}

export function dailyNotebook(date: string, title: string, carry = false): Promise<Notebook> {
  return pedir<Notebook>('/api/daily', { method: 'POST', ...cuerpo({ date, title, carry }) });
}

/** La vista «Tareas»: las de todos los cuadernos, en un estado. */
export async function listTasks(state: TaskState): Promise<Task[]> {
  return (await pedir<{ tasks: Task[] }>(`/api/tasks?state=${state}`)).tasks ?? [];
}

/** «Posponer»: la tarea, con su subárbol, al diario de `date`. */
export function postponeTask(id: string, date: string, title: string): Promise<Notebook> {
  return pedir<Notebook>(`/api/notes/${encodeURIComponent(id)}/postpone`, {
    method: 'POST',
    ...cuerpo({ date, title }),
  });
}

/** Los días con notas de `[from, to]`; con `notes`, también sus notas. */
export async function dailyRange(from: string, to: string, notes = false): Promise<DailyDay[]> {
  const q = new URLSearchParams({ from, to, ...(notes ? { notes: '1' } : {}) });
  return (await pedir<{ days: DailyDay[] }>(`/api/daily?${q}`)).days ?? [];
}

/** «Este día»: el mismo día de años anteriores. */
export async function thisDay(date: string): Promise<DailyDay[]> {
  const q = new URLSearchParams({ this_day: date });
  return (await pedir<{ days: DailyDay[] }>(`/api/daily?${q}`)).days ?? [];
}

/** Un evento de la agenda del día, ya con su enlace a Sytue Calendar. */
export interface EventoAgenda {
  id: string;
  name: string;
  url?: string;
  start: string;
  end?: string;
  allDay?: boolean;
}

export async function agenda(date: string, from: string, to: string): Promise<EventoAgenda[]> {
  const q = new URLSearchParams({ date, from, to });
  return (await pedir<{ events: EventoAgenda[] }>(`/api/agenda?${q}`)).events ?? [];
}

/** Crea un evento de día completo en el calendario por defecto; devuelve el enlace. */
export async function crearEvento(date: string, summary: string): Promise<NoteLink> {
  return (
    await pedir<{ link: NoteLink }>('/api/agenda', { method: 'POST', ...cuerpo({ date, summary }) })
  ).link;
}

export function createNotebook(input: NotebookInput): Promise<Notebook> {
  return pedir<Notebook>('/api/notebooks', { method: 'POST', ...cuerpo(input) });
}

export function updateNotebook(id: string, input: NotebookInput): Promise<Notebook> {
  return pedir<Notebook>(`/api/notebooks/${encodeURIComponent(id)}`, {
    method: 'PUT',
    ...cuerpo(input),
  });
}

export function deleteNotebook(id: string): Promise<void> {
  return pedir<void>(`/api/notebooks/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

// --- Notas ------------------------------------------------------------------

/** «Compartido conmigo»: las notas que otras personas han compartido conmigo. */
export async function listShared(): Promise<SharedNote[]> {
  return (await pedir<{ notes: SharedNote[] }>('/api/shared')).notes ?? [];
}

export async function listNotes(notebookId: string, signal?: AbortSignal): Promise<Note[]> {
  const r = await pedir<{ notes: Note[] }>(
    `/api/notebooks/${encodeURIComponent(notebookId)}/notes`,
    { cache: 'no-store', ...(signal ? { signal } : {}) },
  );
  return (r.notes ?? []).map((n) =>
    n.data_json === undefined
      ? n
      : { ...n, data: JSON.parse(n.data_json) as Record<string, unknown> },
  );
}

export function createNote(notebookId: string, input: NoteInput): Promise<Note> {
  return pedir<Note>(`/api${noteWritePath('create', notebookId, input)}`, {
    method: 'POST',
    ...cuerpo(input),
  });
}

export function migrateEncryptedContent(
  id: string,
  input: NoteCipherMigration,
  signal?: AbortSignal,
): Promise<Note> {
  return pedir<Note>(`/api/notes/${encodeURIComponent(id)}/encrypted-content`, {
    method: 'POST',
    ...cuerpo(input),
    ...(signal ? { signal } : {}),
  });
}

export function updateNote(id: string, notebookId: string, input: NoteInput): Promise<Note> {
  return pedir<Note>(`/api${noteWritePath('update', id, input)}`, {
    method: 'PUT',
    ...cuerpo({ ...input, notebook_id: notebookId }),
  });
}

export function moveNote(
  id: string,
  pos: { x: number; y: number; col: number; position: number; group_id?: string | null },
): Promise<void> {
  return pedir<void>(`/api/notes/${encodeURIComponent(id)}/move`, {
    method: 'POST',
    ...cuerpo(pos),
  });
}

/** Cambia la nota de cuaderno (con sus agrupadas); el destino decide dónde cae. */
export function moveNoteToNotebook(id: string, notebookId: string): Promise<void> {
  return pedir<void>(`/api/notes/${encodeURIComponent(id)}/move`, {
    method: 'POST',
    ...cuerpo({ notebook_id: notebookId }),
  });
}

export function deleteNote(id: string): Promise<void> {
  return pedir<void>(`/api/notes/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

/** Archiva el cuaderno con su rama, o la devuelve al árbol. */
export function archiveNotebook(id: string, archivar: boolean): Promise<void> {
  return pedir<void>(`/api/notebooks/${encodeURIComponent(id)}/archive`, {
    method: archivar ? 'POST' : 'DELETE',
  });
}

export function restoreNote(id: string): Promise<void> {
  return pedir<void>(`/api/notes/${encodeURIComponent(id)}/restore`, { method: 'POST' });
}

export function purgeNote(id: string): Promise<void> {
  return pedir<void>(`/api/notes/${encodeURIComponent(id)}/purge`, { method: 'DELETE' });
}

export async function listTrash(): Promise<Note[]> {
  return (await pedir<{ notes: Note[] }>('/api/trash')).notes ?? [];
}

// --- Etiquetas --------------------------------------------------------------

export async function listTags(): Promise<Tag[]> {
  return (await pedir<{ tags: Tag[] }>('/api/tags')).tags ?? [];
}

export function saveTag(input: { name: string; states: TagState[] }, id?: string): Promise<Tag> {
  return id
    ? pedir<Tag>(`/api/tags/${encodeURIComponent(id)}`, { method: 'PUT', ...cuerpo(input) })
    : pedir<Tag>('/api/tags', { method: 'POST', ...cuerpo(input) });
}

export function deleteTag(id: string): Promise<void> {
  return pedir<void>(`/api/tags/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export function setNoteState(noteId: string, tagId: string, stateId: string | null): Promise<void> {
  return pedir<void>(`/api/notes/${encodeURIComponent(noteId)}/state`, {
    method: 'POST',
    ...cuerpo({ tag_id: tagId, state_id: stateId }),
  });
}

// --- Búsqueda ---------------------------------------------------------------

// --- Recordatorios -----------------------------------------------------------

export async function listReminders(): Promise<Reminder[]> {
  return (await pedir<{ reminders: Reminder[] }>('/api/reminders')).reminders ?? [];
}

export function setReminder(noteId: string, input: ReminderInput): Promise<Reminder> {
  return pedir<Reminder>(`/api/notes/${encodeURIComponent(noteId)}/reminder`, {
    method: 'PUT',
    ...cuerpo(input),
  });
}

export function deleteReminder(noteId: string): Promise<void> {
  return pedir<void>(`/api/notes/${encodeURIComponent(noteId)}/reminder`, { method: 'DELETE' });
}

export async function search(q: string, archivados = false): Promise<NoteHit[]> {
  const r = await pedir<{ notes: NoteHit[] }>(
    `/api/search?q=${encodeURIComponent(q)}${archivados ? '&archived=1' : ''}`,
  );
  return r.notes ?? [];
}

/** «Fijadas»: las notas fijadas de todos los cuadernos visibles. */
export async function pinnedNotes(): Promise<NoteHit[]> {
  return (await pedir<{ notes: NoteHit[] }>('/api/pinned')).notes ?? [];
}

/** Los `#hashtags` de las notas visibles, del más usado al menos. */
export async function hashtags(): Promise<Hashtag[]> {
  return (await pedir<{ hashtags: Hashtag[] }>('/api/hashtags')).hashtags ?? [];
}

// --- Lo que se puede incrustar (Drive, Docs, Calendar, Rooms) ---------------

/** Un candidato del selector, ya normalizado por el BFF. */
export interface Elegible {
  id: string;
  name: string;
  isDirectory?: boolean;
  path?: string;
  url?: string;
  meta?: string;
}

// `notes` somos nosotros mismos: enlazar una nota con OTRA es la referencia que
// Sytue Notepads hereda de BasKet Note Pads, y viaja por el mismo selector que
// todo lo demás porque el gesto es idéntico —«enséñame lo que puedo enganchar
// aquí y déjame elegir»—.
export type OrigenSuite = 'drive' | 'docs' | 'calendar' | 'rooms' | 'notes';

export async function listarDeLaSuite(
  kind: OrigenSuite,
  opts: { q?: string; path?: string; cursor?: string } = {},
): Promise<{ items: Elegible[]; nextCursor: string | null }> {
  const p = new URLSearchParams({ kind });
  if (opts.q) p.set('q', opts.q);
  if (opts.path) p.set('path', opts.path);
  if (opts.cursor) p.set('cursor', opts.cursor);
  return pedir<{ items: Elegible[]; nextCursor: string | null }>(`/api/suite?${p.toString()}`);
}

/** Publica el ENLACE de una nota en una sala de Rooms (nunca su contenido). */
export function compartirEnSala(input: {
  channel_id: string;
  notebook_id: string;
  note_id: string;
  title: string;
}): Promise<{ id: string }> {
  return pedir<{ id: string }>('/api/share', { method: 'POST', ...cuerpo(input) });
}

/**
 * QUÉ APUNTA A ESTA NOTA. La vuelta de la referencia.
 *
 * El enlace solo se veía desde el lado que lo creó: al abrir la nota apuntada no
 * había forma de saber quién la señalaba, y un enlace que solo se ve desde un
 * extremo es una nota suelta con una flecha, no una red. Es lo que BasKet Note
 * Pads hacía entre cestas.
 *
 * Se pide BAJO DEMANDA y no al pintar el lienzo: una pared de cuarenta notas
 * serían cuarenta peticiones para enseñar, casi siempre, nada.
 */
export function retroenlaces(noteId: string): Promise<{ notes: Note[] }> {
  return pedir<{ notes: Note[] }>(`/api/backlinks?note=${encodeURIComponent(noteId)}`);
}

/** El tipo de destino de enlace que corresponde a cada origen de la suite. */
export const DESTINO_POR_ORIGEN: Record<OrigenSuite, NoteLinkTarget> = {
  drive: 'drive_file',
  docs: 'docs_document',
  calendar: 'calendar_event',
  rooms: 'room_channel',
  notes: 'note',
};

/**
 * Formatos de exportación, EN EL ORDEN EN QUE SE OFRECEN: el nativo arriba.
 *
 * `nativo` es el HTML, y va primero porque es el único que se lleva el cuaderno
 * entero —disposición, colores, enlaces y notas cifradas— y del que se puede
 * volver. Lo demás son salidas hacia fuera: Markdown y texto para leerlo en otro
 * programa, PDF para imprimirlo, `.onex` para dárselo a una máquina.
 *
 * `ver` distingue lo que el navegador puede ABRIR de lo que solo puede bajar. El
 * HTML se queda fuera pese a ser el nativo: lleva contenido del cuaderno y
 * abrirlo en el origen de la app le daría el mismo origen que la sesión. Se
 * descarga y se abre desde el disco, donde no es nadie.
 */
export const FORMATOS_EXPORT = [
  { id: 'html', ver: false },
  { id: 'md', ver: false },
  { id: 'txt', ver: false },
  { id: 'pdf', ver: true },
  { id: 'onex', ver: false },
] as const;

export type FormatoExport = (typeof FORMATOS_EXPORT)[number]['id'];

/** URL de descarga o de vista de un cuaderno exportado. Sin formato sale el nativo. */
export function urlExport(
  notebookId: string,
  formato: FormatoExport,
  ver = false,
  /** Una sola nota, en vez del cuaderno entero. */
  noteId?: string,
): string {
  const p = new URLSearchParams({ format: formato });
  if (ver) p.set('view', '1');
  if (noteId) p.set('note', noteId);
  return withBase(
    import.meta.env.BASE_URL,
    `/api/notebooks/${encodeURIComponent(notebookId)}/export?${p.toString()}`,
  );
}

/** El ensayo de una importación: lo que entraría, y cuánto se quedaría fuera. */
export interface VistaPreviaVolcado {
  notebook: Notebook;
  notas: Note[];
  descartadas: number;
}

/**
 * Lee un `.onex` y devuelve lo que ENTRARÍA, sin crear nada: el ensayo saneado
 * de la importación, con las notas que no pasarían ya contadas.
 */
export function previsualizarVolcado(volcado: string): Promise<VistaPreviaVolcado> {
  return pedir<VistaPreviaVolcado>('/api/notebooks/import?preview=1', {
    method: 'POST',
    body: volcado,
  });
}

/** Crea un cuaderno nuevo a partir de un `.onex`. No sobrescribe ninguno. */
export function importarVolcado(
  volcado: string,
): Promise<{ notebook: Notebook; importadas: number; descartadas: number }> {
  return pedir<{ notebook: Notebook; importadas: number; descartadas: number }>(
    '/api/notebooks/import',
    { method: 'POST', body: volcado },
  );
}
