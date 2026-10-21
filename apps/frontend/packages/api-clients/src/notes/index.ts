/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Notepads — SDK al servicio Go propio (notes.sytue.eu, dev :8089).
 * Cuadernos anidados, notas heterogéneas dentro de ellos, etiquetas con estado,
 * búsqueda y referencias cruzadas con el resto de la suite. Estado en
 * PostgreSQL. Autorización Bearer (JWT de Sytue ID).
 *
 * REST base: /api/v1
 *
 * Lo consumen DOS superficies: el BFF de la app de Notes y —para el otro extremo
 * de la referencia bidireccional— el de Sytue Calendar, que pregunta por las
 * notas de un evento con el token de quien mira.
 */
import { HttpClient, HttpError, type AuthProvider } from '../core/http.js';

const MAX_NOTE_PAGE_BYTES = 32 << 20;

export interface NotesOptions {
  /** URL base, p.ej. https://notes.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

/** Quién puede ver un cuaderno o una nota. */
export type NoteVisibility = 'private' | 'personal' | 'organization' | 'public';

/** Disposición del lienzo de un cuaderno. */
/** `tree` es la disposición por defecto: árbol de notas a la izquierda y la nota a la derecha. */
export type NotebookLayout = 'tree' | 'free' | 'columns' | 'list';

/** Tipo de una nota: decide cómo se pinta y qué hay en `data`. */
export type NoteKind =
  | 'text'
  | 'todo'
  | 'image'
  | 'file'
  | 'link'
  | 'color'
  | 'launcher'
  | 'crossref'
  | 'doc'
  | 'drive'
  | 'event'
  | 'room';

/** Destino de un enlace de una nota a un objeto de la suite. */
export type NoteLinkTarget =
  | 'calendar_event'
  | 'room_channel'
  | 'room_message'
  | 'docs_document'
  | 'drive_file'
  | 'notebook'
  | 'note';

/**
 * Parámetros de derivación de clave de un cuaderno cifrado. El servidor los
 * guarda y los sirve, pero no participa en el cifrado: la clave se deriva en el
 * navegador y la contraseña no sale de él.
 */
export interface NotesCrypto {
  v: 1;
  kdf: 'PBKDF2-SHA256';
  salt: string;
  iterations: number;
  /** Criptograma de verificación: valida la contraseña sin descifrar nada más. */
  check: string;
}

export interface NoteLink {
  target_kind: NoteLinkTarget;
  target_id: string;
  label?: string;
  url?: string;
}

export interface NoteState {
  tag_id: string;
  tag_name: string;
  state_id: string;
  name: string;
  emoji?: string;
  color: string;
}

export interface Notebook {
  daily_date?: string;
  /** En el Archivo: fuera del árbol y de la búsqueda, intacto. */
  archived_at?: string;
  id: string;
  owner_sub: string;
  parent_id?: string | null;
  title: string;
  emoji?: string;
  color: string;
  layout: NotebookLayout;
  visibility: NoteVisibility;
  indexable: boolean;
  encrypted: boolean;
  crypto?: NotesCrypto;
  room_id?: string;
  /** Solo lo recibe su dueño: es la credencial del enlace público. */
  public_token?: string;
  position: number;
  note_count: number;
  created_at: string;
  updated_at: string;
  /** Lo lee pero no escribe sus notas (un cuaderno público de otra persona). */
  read_only?: boolean;
  /** En ese caso, ya ha pedido editarlo (`FranjaPeticion`). */
  edit_requested?: boolean;
}

export interface Note {
  id: string;
  notebook_id: string;
  owner_sub: string;
  kind: NoteKind;
  title?: string;
  content?: string;
  /** Criptograma «encv1:…» en los cuadernos cifrados (el claro va vacío). */
  content_cipher?: string;
  data?: Record<string, unknown>;
  data_json?: string;
  x: number;
  y: number;
  col: number;
  position: number;
  width: number;
  group_id?: string | null;
  collapsed: boolean;
  checked: boolean;
  /** Cuándo se completó / se canceló (tachada y fuera del traslado). Nunca las dos. */
  checked_at?: string;
  cancelled_at?: string;
  pinned: boolean;
  visibility?: NoteVisibility;
  indexable?: boolean;
  states?: NoteState[];
  links?: NoteLink[];
  created_at: string;
  updated_at: string;
  /**
   * Solo en la respuesta de editar una tarea: la siguiente que creó al marcarla hecha,
   * o la que retiró al desmarcarla. Si viene, hay que volver a pedir notas y recordatorios.
   */
  repeated_into?: string;
}

/** Un día del diario con sus cuentas (`GET /daily`). `notes` solo si se pidieron. */
export interface DailyDay {
  date: string;
  notebook_id: string;
  total: number;
  pending: number;
  notes?: Note[];
}

/** Una nota de «Compartido conmigo»: el nivel con que me la dieron y quién. */
export interface SharedNote extends Note {
  permission: 'read' | 'write';
  /** `sub` de quien la compartió. El BFF le pone nombre en `shared_by_name`. */
  shared_by: string;
  shared_by_name?: string;
}

export interface NotebookInput {
  parent_id?: string | null;
  title: string;
  emoji?: string;
  color?: string;
  layout?: NotebookLayout;
  visibility?: NoteVisibility;
  indexable?: boolean;
  encrypted?: boolean;
  crypto?: NotesCrypto | null;
  room_id?: string;
  position?: number;
}

export interface NoteInput {
  cipher_version?: 2;
  expected_crypto?: NotesCrypto;
  kind?: NoteKind;
  title?: string;
  content?: string;
  content_cipher?: string;
  data?: Record<string, unknown>;
  x?: number;
  y?: number;
  col?: number;
  position?: number;
  width?: number;
  group_id?: string | null;
  collapsed?: boolean;
  checked?: boolean;
  /** Cancela (true) o reactiva (false) una tarea; sin él, se queda como está. */
  cancelled?: boolean;
  pinned?: boolean;
  visibility?: NoteVisibility;
  indexable?: boolean;
  links?: NoteLink[];
}

export interface NoteCipherMigration {
  cipher_version: 2;
  expected_updated_at: string;
  expected_crypto: NotesCrypto;
  content_cipher: string;
  data?: Record<string, unknown>;
}

/** A marked write must reach an endpoint that enforces encryption, including malformed inputs. */
export function noteWritePath(action: 'create' | 'update', id: string, input: NoteInput): string {
  const encrypted =
    input.cipher_version !== undefined ||
    input.expected_crypto !== undefined ||
    Boolean(input.content_cipher);
  const encoded = encodeURIComponent(id);
  return action === 'create'
    ? `/notebooks/${encoded}/${encrypted ? 'encrypted-notes' : 'notes'}`
    : `/notes/${encoded}${encrypted ? '/encrypted' : ''}`;
}

export type TaskState = 'open' | 'done' | 'cancelled';

/** Una tarea de la vista «Tareas», con lo justo de su cuaderno. */
export interface Task extends Note {
  notebook: string;
  daily_date?: string;
}

/** Un `#hashtag` escrito en las notas (sin el `#`) y cuántas notas lo llevan. */
export interface Hashtag {
  tag: string;
  count: number;
}

export interface NoteHit {
  id: string;
  notebook_id: string;
  notebook: string;
  title: string;
  snippet: string;
  kind: NoteKind;
  updated_at: string;
  /** Títulos (h1..h6) dentro de la nota, para enlazar a uno de ellos. */
  headings?: string[];
}

export interface TagState {
  id?: string;
  tag_id?: string;
  name: string;
  emoji?: string;
  color?: string;
  shortcut?: string;
  position?: number;
}

export interface Tag {
  id: string;
  name: string;
  position: number;
  states: TagState[];
}

export type FormatoExport = (typeof NotesClient.formatosExport)[number];

/**
 * Resultado de importar. `descartadas` viaja siempre: una importación que se come
 * notas en silencio es peor que una que falla, porque el hueco se descubre
 * semanas después y ya no hay fichero al que volver.
 */
/** El ensayo de una importación: lo que entraría, y cuánto se quedaría fuera. */
export interface PreviewResult {
  notebook: Notebook;
  notas: Note[];
  descartadas: number;
}

/**
 * Mi recordatorio en una nota (`domain.Reminder` del servicio). `due_at` es el
 * instante en UTC; `tzid`, la zona en que se calcula la repetición. Repite con una
 * RRULE sin DTSTART o, «flexible», `after_days` días después de completarla.
 */
export interface Reminder {
  note_id: string;
  notebook_id: string;
  due_at: string;
  all_day: boolean;
  rrule?: string;
  after_days?: number;
  tzid: string;
}

export interface ReminderInput {
  due_at: string;
  all_day: boolean;
  rrule: string;
  after_days: number;
  tzid: string;
}

export interface ImportResult {
  notebook: Notebook;
  importadas: number;
  descartadas: number;
}

export class NotesClient {
  private readonly http: HttpClient;

  constructor(options: NotesOptions) {
    this.http = new HttpClient({
      baseUrl: `${options.baseUrl.replace(/\/$/, '')}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /** Árbol completo de cuadernos visibles. */
  async listNotebooks(): Promise<Notebook[]> {
    const r = await this.http.json<{ notebooks: Notebook[] }>({ path: '/notebooks' });
    return r.notebooks ?? [];
  }

  dailyNotebook(date: string, title: string, carry = false): Promise<Notebook> {
    return this.http.postJson<Notebook>('/daily', { date, title, carry });
  }

  /** Las tareas de quien pregunta en todos sus cuadernos (`from`/`to`: RFC 3339). */
  async listTasks(state: TaskState = 'open', from?: string, to?: string): Promise<Task[]> {
    const r = await this.http.json<{ tasks: Task[] }>({
      path: '/tasks',
      query: { state, ...(from ? { from } : {}), ...(to ? { to } : {}) },
    });
    return r.tasks ?? [];
  }

  /** Lleva la tarea, con lo que cuelga de ella, al diario de `date`. */
  postponeTask(id: string, date: string, title: string): Promise<Notebook> {
    return this.http.postJson<Notebook>(`/notes/${encodeURIComponent(id)}/postpone`, {
      date,
      title,
    });
  }

  /**
   * Los días con notas de `[from, to]` (incluidos), con sus cuentas: los puntos de la
   * semana. Con `notes` trae además sus notas, para las vistas Semana y Días.
   */
  async dailyRange(from: string, to: string, notes = false): Promise<DailyDay[]> {
    const r = await this.http.json<{ days: DailyDay[] }>({
      path: '/daily',
      query: { from, to, ...(notes ? { notes: 1 } : {}) },
    });
    return r.days ?? [];
  }

  /** «Este día»: el mismo día y mes de años anteriores, del más reciente al más antiguo. */
  async thisDay(date: string): Promise<DailyDay[]> {
    const r = await this.http.json<{ days: DailyDay[] }>({
      path: '/daily/this-day',
      query: { date },
    });
    return r.days ?? [];
  }

  createNotebook(input: NotebookInput): Promise<Notebook> {
    return this.http.postJson<Notebook>('/notebooks', input);
  }

  getNotebook(id: string): Promise<Notebook> {
    return this.http.json<Notebook>({ path: `/notebooks/${encodeURIComponent(id)}` });
  }

  updateNotebook(id: string, input: NotebookInput): Promise<Notebook> {
    return this.http.json<Notebook>({
      method: 'PUT',
      path: `/notebooks/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(input),
    });
  }

  async deleteNotebook(id: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/notebooks/${encodeURIComponent(id)}` });
  }

  /** Archiva el cuaderno con su rama (`true`) o la devuelve al árbol (`false`). */
  async archiveNotebook(id: string, archivar: boolean): Promise<void> {
    await this.http.request({
      method: archivar ? 'POST' : 'DELETE',
      path: `/notebooks/${encodeURIComponent(id)}/archive`,
    });
  }

  /** «Fijadas»: las notas fijadas de todos los cuadernos visibles. */
  async pinnedNotes(): Promise<NoteHit[]> {
    const r = await this.http.json<{ notes: NoteHit[] }>({ path: '/notes/pinned' });
    return r.notes ?? [];
  }

  /** Los `#hashtags` de las notas visibles. Las notas de uno: `search('#plan')`. */
  async hashtags(): Promise<Hashtag[]> {
    const r = await this.http.json<{ hashtags: Hashtag[] }>({ path: '/hashtags' });
    return r.hashtags ?? [];
  }

  /** Notas de un cuaderno, con sus estados y enlaces ya adjuntos. */
  async listNotes(notebookId: string): Promise<Note[]> {
    const notes: Note[] = [];
    const seen = new Set<string>();
    let cursor = '';
    let legacyRead = false;
    for (;;) {
      const query = new URLSearchParams({
        limit: legacyRead ? '1' : '100',
        raw_data: '1',
        ...(cursor ? { cursor } : {}),
      });
      let page: { notes: Note[]; next_cursor?: string };
      try {
        page = await this.http.json({
          path: `/notebooks/${encodeURIComponent(notebookId)}/notes${legacyRead ? '' : '/page'}?${query}`,
          maxResponseBytes: MAX_NOTE_PAGE_BYTES,
        });
      } catch (error) {
        if (!legacyRead && error instanceof HttpError && error.status === 404) {
          // Restart: an older backend may ignore cursors and return the full legacy list.
          legacyRead = true;
          notes.length = 0;
          cursor = '';
          seen.clear();
          continue;
        }
        throw error;
      }
      notes.push(...(page.notes ?? []));
      if (!page.next_cursor) break;
      if (seen.has(page.next_cursor) || !page.notes?.length)
        throw new Error('notes: invalid cursor');
      seen.add(page.next_cursor);
      cursor = page.next_cursor;
    }
    return notes.sort(
      (a, b) =>
        Number(b.pinned) - Number(a.pinned) ||
        a.col - b.col ||
        a.position - b.position ||
        a.created_at.localeCompare(b.created_at) ||
        a.id.localeCompare(b.id),
    );
  }

  createNote(notebookId: string, input: NoteInput): Promise<Note> {
    return this.http.postJson<Note>(noteWritePath('create', notebookId, input), input);
  }

  migrateEncryptedContent(id: string, input: NoteCipherMigration): Promise<Note> {
    return this.http.postJson<Note>(`/notes/${encodeURIComponent(id)}/encrypted-content`, input);
  }

  updateNote(id: string, notebookId: string, input: NoteInput): Promise<Note> {
    return this.http.json<Note>({
      method: 'PUT',
      path: noteWritePath('update', id, input),
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ ...input, notebook_id: notebookId }),
    });
  }

  /**
   * Reubica una nota sin reenviar su contenido. Es la escritura del arrastrar y
   * soltar, que ocurre muchas veces por minuto: mandar el cuerpo entero en cada
   * soltar obligaría además a resanearlo.
   */
  async moveNote(
    id: string,
    pos: { x: number; y: number; col: number; position: number; group_id?: string | null },
  ): Promise<void> {
    await this.http.postJson<void>(`/notes/${encodeURIComponent(id)}/move`, pos);
  }

  /** Cambia la nota de cuaderno (con sus agrupadas); el destino decide la posición. */
  async moveNoteToNotebook(id: string, notebookId: string): Promise<void> {
    await this.http.postJson<void>(`/notes/${encodeURIComponent(id)}/move`, {
      notebook_id: notebookId,
    });
  }

  async deleteNote(id: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/notes/${encodeURIComponent(id)}` });
  }

  async restoreNote(id: string): Promise<void> {
    await this.http.postJson<void>(`/notes/${encodeURIComponent(id)}/restore`, {});
  }

  async purgeNote(id: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/notes/${encodeURIComponent(id)}/purge` });
  }

  /** «Compartido conmigo»: las notas que otras personas han compartido conmigo. */
  async listShared(): Promise<SharedNote[]> {
    const r = await this.http.json<{ notes: SharedNote[] }>({ path: '/shared/notes' });
    return r.notes ?? [];
  }

  async listTrash(limit = 100): Promise<Note[]> {
    const r = await this.http.json<{ notes: Note[] }>({ path: '/notes/trash', query: { limit } });
    return r.notes ?? [];
  }

  /** Busca en las notas indexables visibles. Sin `q`, devuelve las recientes. */
  async search(q: string, limit = 20, archivados = false): Promise<NoteHit[]> {
    const r = await this.http.json<{ notes: NoteHit[] }>({
      path: '/notes/search',
      // Sin `archived`, el servicio deja fuera lo archivado.
      query: archivados ? { q, limit, archived: 1 } : { q, limit },
    });
    return r.notes ?? [];
  }

  /**
   * El OTRO EXTREMO de la referencia bidireccional: las notas que cuelgan de un
   * objeto de la suite. La ficha de un evento de Sytue Calendar lo llama con el
   * token de quien mira, así que la autorización sigue siendo la de Notes.
   */
  async linkedNotes(targetKind: NoteLinkTarget, targetId: string, limit = 50): Promise<Note[]> {
    const r = await this.http.json<{ notes: Note[] }>({
      path: '/notes/linked',
      query: { target_kind: targetKind, target_id: targetId, limit },
    });
    return r.notes ?? [];
  }

  /**
   * Formatos de exportación. `md` y `markdown` son el mismo Markdown con otra
   * extensión; `onex` es el volcado reimportable y `pdf` el que se imprime.
   */
  static readonly formatosExport = ['md', 'markdown', 'txt', 'html', 'onex', 'pdf'] as const;

  /**
   * Descarga (o abre) un cuaderno exportado.
   *
   * Devuelve la `Response` SIN leerla: el PDF es binario y el volcado de un
   * cuaderno grande puede pesar, así que quien llama decide si lo pasa a blob, lo
   * reenvía tal cual o lo tira. Pasarlo por `text()` aquí romperia el PDF y
   * obligaría a tener el fichero entero en memoria dos veces.
   */
  exportNotebook(
    id: string,
    format: FormatoExport,
    view = false,
    /** Una sola nota del cuaderno, en vez del cuaderno entero. */
    noteId?: string,
  ): Promise<Response> {
    return this.http.request({
      path: `/notebooks/${encodeURIComponent(id)}/export`,
      query: { format, view: view ? '1' : undefined, note: noteId },
    });
  }

  /** Crea un cuaderno nuevo a partir de un `.onex`. No sobrescribe ninguno. */
  importNotebook(volcado: BodyInit): Promise<ImportResult> {
    return this.http.json<ImportResult>({
      method: 'POST',
      path: '/notebooks/import',
      body: volcado,
    });
  }

  /** Lee un `.onex` y devuelve lo que entraría —ya saneado— sin crear nada. */
  previewNotebook(volcado: BodyInit): Promise<PreviewResult> {
    return this.http.json<PreviewResult>({
      method: 'POST',
      path: '/notebooks/preview',
      body: volcado,
    });
  }

  async listTags(): Promise<Tag[]> {
    const r = await this.http.json<{ tags: Tag[] }>({ path: '/tags' });
    return r.tags ?? [];
  }

  saveTag(input: { name: string; states: TagState[] }, id?: string): Promise<Tag> {
    if (!id) return this.http.postJson<Tag>('/tags', input);
    return this.http.json<Tag>({
      method: 'PUT',
      path: `/tags/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(input),
    });
  }

  async deleteTag(id: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/tags/${encodeURIComponent(id)}` });
  }

  /** Marca un estado de etiqueta en una nota. `stateId` nulo lo quita. */
  async setNoteState(noteId: string, tagId: string, stateId: string | null): Promise<void> {
    await this.http.postJson<void>(`/notes/${encodeURIComponent(noteId)}/state`, {
      tag_id: tagId,
      state_id: stateId,
    });
  }

  /** Mis recordatorios pendientes: el indicador de cada fila. */
  async listReminders(): Promise<Reminder[]> {
    const r = await this.http.json<{ reminders: Reminder[] }>({ path: '/reminders' });
    return r.reminders ?? [];
  }

  /** Pone (o rearma) mi recordatorio en una nota. */
  setReminder(noteId: string, input: ReminderInput): Promise<Reminder> {
    return this.http.json<Reminder>({
      method: 'PUT',
      path: `/notes/${encodeURIComponent(noteId)}/reminder`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(input),
    });
  }

  async deleteReminder(noteId: string): Promise<void> {
    await this.http.request({
      method: 'DELETE',
      path: `/notes/${encodeURIComponent(noteId)}/reminder`,
    });
  }
}
