/**
 * `sytue.table('posts')` — leer y escribir en la base del proyecto sin SQL, sin
 * cadena de conexión y sin pool.
 *
 * Por debajo habla con PostgREST, que es lo que Sytue DevX aprovisiona junto a
 * cada Sytue Postgres. Pero eso no aparece por ninguna parte de la API: quien
 * empieza no tiene por qué saber qué es PostgREST, ni qué es `eq.`, ni por qué
 * un `PATCH` sin filtro es peligroso.
 *
 * Tres decisiones que definen el tono de todo el SDK:
 *
 *  1. **Los verbos son los de la vida real**: `add`, `update`, `remove`, `find`,
 *     `all`, `first`. Nada de `insert().select().single()`.
 *  2. **Ningún borrado ni actualización sin destinatario.** `update` y `remove`
 *     exigen el identificador de la fila; no existe la forma de vaciar una tabla
 *     por accidente, que es el error más caro que comete alguien que empieza.
 *  3. **Los errores dicen qué hacer.** «relation "posts" does not exist» se
 *     convierte en «No existe la tabla "posts"…, revisa el nombre o créala».
 */
import { SytueError, type CloudConfig } from './config.js';
import { messages } from './messages.js';

/** Una fila. Sin tipar, es un objeto normal. */
export type Row = Record<string, unknown>;

/** Valor admitido en un filtro de igualdad. */
export type Scalar = string | number | boolean | null;

export interface TableOptions {
  /** Columna que identifica cada fila. Por defecto `id`. */
  key?: string;
}

/**
 * Traduce los errores de PostgreSQL/PostgREST a algo accionable, en el idioma
 * del proyecto. Los códigos son los SQLSTATE de PostgreSQL, que PostgREST pasa
 * tal cual: son estables entre versiones y no dependen del texto del mensaje.
 */
function explain(status: number, body: unknown, table: string, language?: string): string {
  const m = messages(language);
  const e = (body ?? {}) as { code?: string; message?: string; hint?: string };
  const detail = e.message ?? '';
  switch (e.code) {
    case '42P01':
      return m.noTable(table);
    case '42703':
      return m.noColumn(table, detail);
    case '23505':
      return m.duplicate(detail);
    case '23503':
      return m.foreignKey(detail);
    case '23502':
      return m.required(detail);
    case '22P02':
      return m.badType(detail);
  }
  if (status === 401 || status === 403) return m.denied(table);
  if (status === 404) return m.noTable(table);
  return detail || m.generic(status, table);
}

/**
 * Consulta en construcción. Cada método devuelve una consulta NUEVA, así que
 * guardarla en una variable y reutilizarla no arrastra filtros de antes —un
 * fallo clásico de las APIs encadenadas que mutan.
 */
export class Query<T extends Row = Row> {
  constructor(
    private readonly cfg: CloudConfig,
    private readonly table: string,
    private readonly opts: TableOptions,
    private readonly params: [string, string][] = [],
  ) {}

  private with(...extra: [string, string][]): Query<T> {
    return new Query<T>(this.cfg, this.table, this.opts, [...this.params, ...extra]);
  }

  /** Filtra por igualdad: `where({ publicado: true, autor: 'ana' })`. */
  where(match: Record<string, Scalar>): Query<T> {
    return this.with(
      ...Object.entries(match).map(([col, val]): [string, string] => [
        col,
        val === null ? 'is.null' : `eq.${val}`,
      ]),
    );
  }

  /** Busca texto dentro de una columna, sin distinguir mayúsculas. */
  search(column: string, text: string): Query<T> {
    // El `*` va dentro del valor, no en la sintaxis: quien llama escribe la
    // palabra, no un patrón.
    return this.with([column, `ilike.*${text}*`]);
  }

  /** Ordena de más nuevo a más viejo. Por defecto mira `created_at`. */
  newest(column = 'created_at'): Query<T> {
    return this.with(['order', `${column}.desc`]);
  }

  /** Ordena de más viejo a más nuevo. */
  oldest(column = 'created_at'): Query<T> {
    return this.with(['order', `${column}.asc`]);
  }

  /** Como mucho N filas. */
  limit(n: number): Query<T> {
    return this.with(['limit', String(n)]);
  }

  /** Se salta las N primeras (para paginar junto a `limit`). */
  skip(n: number): Query<T> {
    return this.with(['offset', String(n)]);
  }

  /** Todas las filas que cumplan lo pedido. */
  async all(): Promise<T[]> {
    return request<T[]>(this.cfg, this.table, { method: 'GET', params: this.params });
  }

  /** La primera, o `null` si no hay ninguna. Nunca lanza por «no hay resultados». */
  async first(): Promise<T | null> {
    const rows = await this.limit(1).all();
    return rows[0] ?? null;
  }

  /** Cuántas filas cumplen lo pedido, sin traérselas. */
  async count(): Promise<number> {
    const res = await rawRequest(this.cfg, this.table, {
      method: 'GET',
      params: [...this.params, ['select', this.opts.key ?? 'id'], ['limit', '1']],
      headers: { prefer: 'count=exact' },
    });
    // PostgREST devuelve el total en Content-Range: «0-0/42».
    const total = (res.headers.get('content-range') ?? '').split('/')[1];
    const n = Number(total);
    return Number.isFinite(n) ? n : 0;
  }
}

/** Acceso a una tabla. Lo devuelve `sytue.table('posts')`. */
export class Table<T extends Row = Row> {
  constructor(
    private readonly cfg: CloudConfig,
    private readonly name: string,
    private readonly opts: TableOptions = {},
  ) {}

  private get key(): string {
    return this.opts.key ?? 'id';
  }

  private query(): Query<T> {
    return new Query<T>(this.cfg, this.name, this.opts);
  }

  where(match: Record<string, Scalar>): Query<T> {
    return this.query().where(match);
  }
  search(column: string, text: string): Query<T> {
    return this.query().search(column, text);
  }
  newest(column?: string): Query<T> {
    return this.query().newest(column);
  }
  oldest(column?: string): Query<T> {
    return this.query().oldest(column);
  }
  limit(n: number): Query<T> {
    return this.query().limit(n);
  }
  all(): Promise<T[]> {
    return this.query().all();
  }
  first(): Promise<T | null> {
    return this.query().first();
  }
  count(): Promise<number> {
    return this.query().count();
  }

  /** Una fila por su identificador, o `null` si no está. */
  async find(id: Scalar): Promise<T | null> {
    return this.where({ [this.key]: id }).first();
  }

  /** Crea una fila (o varias) y devuelve lo guardado, ya con su id. */
  async add(row: Partial<T>): Promise<T>;
  async add(rows: Partial<T>[]): Promise<T[]>;
  async add(input: Partial<T> | Partial<T>[]): Promise<T | T[]> {
    const rows = await request<T[]>(this.cfg, this.name, {
      method: 'POST',
      body: input,
      // `return=representation` es lo que hace que `add` devuelva la fila con su
      // id en vez de un 201 vacío: sin esto, para saber qué acabas de crear hay
      // que hacer una segunda consulta, y no es evidente por cuál.
      headers: { prefer: 'return=representation' },
    });
    return Array.isArray(input) ? rows : (rows[0] as T);
  }

  /**
   * Cambia los campos indicados de UNA fila y devuelve cómo queda.
   *
   * Exige el identificador a propósito: un update sin filtro reescribe la tabla
   * entera, y es el accidente más caro de quien empieza.
   */
  async update(id: Scalar, changes: Partial<T>): Promise<T | null> {
    const rows = await request<T[]>(this.cfg, this.name, {
      method: 'PATCH',
      params: [[this.key, `eq.${id}`]],
      body: changes,
      headers: { prefer: 'return=representation' },
    });
    return rows[0] ?? null;
  }

  /** Borra UNA fila. Devuelve si existía. Mismo motivo que en `update`. */
  async remove(id: Scalar): Promise<boolean> {
    const rows = await request<T[]>(this.cfg, this.name, {
      method: 'DELETE',
      params: [[this.key, `eq.${id}`]],
      headers: { prefer: 'return=representation' },
    });
    return rows.length > 0;
  }
}

interface Req {
  method: string;
  params?: [string, string][];
  body?: unknown;
  headers?: Record<string, string>;
}

async function rawRequest(cfg: CloudConfig, table: string, req: Req): Promise<Response> {
  if (!cfg.restUrl) throw new SytueError(messages(cfg.language).noDatabase);
  const url = new URL(`${cfg.restUrl.replace(/\/$/, '')}/${encodeURIComponent(table)}`);
  for (const [k, v] of req.params ?? []) url.searchParams.append(k, v);

  const headers: Record<string, string> = { accept: 'application/json', ...req.headers };
  if (cfg.restToken) headers.authorization = `Bearer ${cfg.restToken}`;
  if (req.body !== undefined) headers['content-type'] = 'application/json';

  let res: Response;
  try {
    res = await fetch(url, {
      method: req.method,
      headers,
      ...(req.body === undefined ? {} : { body: JSON.stringify(req.body) }),
    });
  } catch (cause) {
    throw new SytueError(messages(cfg.language).unreachable, cause);
  }
  if (!res.ok) {
    const detail: unknown = await res.json().catch(() => undefined);
    throw new SytueError(explain(res.status, detail, table, cfg.language), detail);
  }
  return res;
}

async function request<T>(cfg: CloudConfig, table: string, req: Req): Promise<T> {
  const res = await rawRequest(cfg, table, req);
  if (res.status === 204) return [] as T;
  return (await res.json()) as T;
}
