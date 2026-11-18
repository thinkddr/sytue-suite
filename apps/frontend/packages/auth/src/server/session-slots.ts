/* SPDX-License-Identifier: Apache-2.0 */
import {
  isSessionGeneration,
  sessionOwnerHeader,
  sessionChangedHeader,
  type SessionOwnerDescriptor,
} from '@sytue/config/session-owner';
import type { SytueSession } from '../types.js';
import type { BffContext, CookieJar } from './bff.js';
import { SESSION_COOKIE, hostCookieName, seal, unseal } from './cookies.js';

export interface SessionSlotRef {
  readonly id: string;
  readonly legacy?: true;
}
export interface SessionSelection {
  readonly v: 1;
  readonly candidate: SessionSlotRef;
  readonly fallback?: SessionSlotRef;
}
interface StoredSlot {
  v: 1;
  slot: string;
  session: SytueSession;
}
export interface SelectedSession {
  readonly ref: SessionSlotRef;
  readonly session: SytueSession;
  readonly selection?: SessionSelection;
}
export function newSessionGeneration(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  const value = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  bytes.fill(0);
  return value;
}
function reference(value: unknown): value is SessionSlotRef {
  if (!value || typeof value !== 'object') return false;
  const ref = value as SessionSlotRef;
  return isSessionGeneration(ref.id) && (ref.legacy === undefined || ref.legacy === true);
}
function sessionValue(value: unknown): value is SytueSession {
  if (!value || typeof value !== 'object') return false;
  const s = value as SytueSession;
  return (
    !!s.user &&
    typeof s.user.id === 'string' &&
    !!s.user.id &&
    !!s.tokens &&
    typeof s.tokens.accessToken === 'string' &&
    typeof s.clientId === 'string' &&
    isSessionGeneration(s.generation)
  );
}
export function sessionOwner(session: SytueSession): SessionOwnerDescriptor | null {
  if (!sessionValue(session) || !session.user.tenant) return null;
  return Object.freeze({
    v: 1,
    actor: session.user.id,
    tenant: session.user.tenant,
    client_id: session.clientId,
    generation: session.generation!,
  });
}
export function sessionChangedResponse(): Response {
  return Response.json(
    { error: 'session_changed' },
    { status: 409, headers: { 'cache-control': 'no-store', [sessionChangedHeader]: '1' } },
  );
}
/** Tokens are stored under an immutable original slot. Only login START writes
 * the selected intent (plus `adopt`, for two logins that started concurrently and
 * never saw each other); callbacks and refreshes can never replace a later login.
 * Tombstones are separate cookies, so an old refresh cannot erase revocation. */
/** Caracteres de sesión sellada por cookie: deja margen al nombre (~90) bajo 4096. */
const SLOT_CHUNK = 3800;
const SLOT_MAX_CHUNKS = 4;

export class SessionSlots {
  readonly selectionCookie: string;
  readonly legacyCookie: string;
  private readonly attrs: Record<string, unknown>;
  private readonly prefix: string;
  /**
   * `namespace` (el client OIDC de la app) separa las cookies de cada BFF. Todas son
   * `__Host-` —Path=/ obligatorio, así que la ruta no puede separarlas— y con el
   * mismo nombre en todas las apps, las que comparten host (el de organización,
   * `<org>.<dominio>/mail`, `/calcus`, `/photos`…) se pisaban: el login de una
   * reescribía la selección de las demás y borraba sus ranuras, y el refresco de
   * cada una volvía a escribir la suya, así que la cabecera `Cookie` crecía con una
   * sesión entera por app hasta pasar los límites del navegador y de los proxies.
   * Sin `namespace`, los nombres de siempre.
   */
  constructor(
    private readonly secret: string,
    private readonly secure: boolean,
    maxAge: number,
    namespace?: string,
  ) {
    if (namespace !== undefined && !/^[a-z0-9-]{1,40}$/.test(namespace))
      throw Error('invalid session namespace');
    this.prefix = SESSION_COOKIE + '_' + (namespace ? namespace + '_' : '');
    this.attrs = { path: '/', httpOnly: true, secure, sameSite: 'lax', maxAge };
    this.selectionCookie = this.name('selection');
    this.legacyCookie = hostCookieName(SESSION_COOKIE, secure);
  }
  private name(suffix: string): string {
    return hostCookieName(this.prefix + suffix, this.secure);
  }
  /**
   * Las cookies de la familia de la sesión que esta petición trae y ya no sirven:
   * las de intentos que no son `keep` en el espacio de esta app y, con espacio
   * propio, TODAS las del espacio compartido de antes (nadie las lee ya). Solo se
   * borra: lo peor que puede pasar con una respuesta tardía es tener que volver a
   * entrar, nunca recuperar una sesión.
   */
  private sweep(ctx: BffContext, keep: ReadonlySet<string | undefined>): void {
    const own = this.name('');
    const shared = hostCookieName(SESSION_COOKIE + '_', this.secure);
    const family = /^(?:(slot|completed|cancelled|pending|oidc)_([a-f0-9]{64})(?:\.[1-9])?)$/;
    const old =
      /^(?:selection|(?:slot|completed|cancelled|pending|oidc|revoked)_[a-f0-9]{64}(?:\.[1-9])?)$/;
    for (const field of (ctx.request.headers.get('cookie') ?? '').split(';')) {
      const name = field.trim().split('=', 1)[0] ?? '';
      if (name.startsWith(own)) {
        const match = family.exec(name.slice(own.length));
        if (match?.[2] && !keep.has(match[2]))
          ctx.cookies.delete(name, { path: '/', secure: this.secure });
      } else if (own !== shared && name.startsWith(shared) && old.test(name.slice(shared.length)))
        ctx.cookies.delete(name, { path: '/', secure: this.secure });
    }
  }
  stateCookie(id: string): string {
    if (!isSessionGeneration(id)) throw Error('invalid session intent');
    return this.name('oidc_' + id);
  }
  private slotCookie(id: string): string {
    return this.name('slot_' + id);
  }
  // LA SESIÓN EN TROZOS. Un navegador descarta en silencio una cookie cuyo nombre
  // y valor pasen de 4096 bytes, y la sesión sellada (usuario + tres tokens) ronda
  // ese tamaño: con la cuenta de administración de desarrollo medía 4144 y el
  // callback devolvía al login una y otra vez, sin ningún error que lo dijera. Por
  // encima de SLOT_CHUNK caracteres se reparte en `<slot>`, `<slot>.1`, `<slot>.2`…;
  // por debajo es una sola cookie con el nombre de siempre, así que las sesiones ya
  // abiertas se siguen leyendo.
  private readSlot(jar: CookieJar, id: string): string | undefined {
    const base = this.slotCookie(id);
    const first = jar.get(base)?.value;
    if (!first) return undefined;
    let value = first;
    for (let i = 1; i < SLOT_MAX_CHUNKS; i++) {
      const part = jar.get(`${base}.${i}`)?.value;
      if (!part) break;
      value += part;
    }
    return value;
  }
  private writeSlot(ctx: BffContext, id: string, value: string): void {
    const base = this.slotCookie(id);
    const parts: string[] = [];
    for (let i = 0; i < value.length; i += SLOT_CHUNK) parts.push(value.slice(i, i + SLOT_CHUNK));
    if (parts.length > SLOT_MAX_CHUNKS) throw Error('session too large');
    parts.forEach((part, i) => ctx.cookies.set(i === 0 ? base : `${base}.${i}`, part, this.attrs));
    // Un trozo de una sesión anterior más larga se concatenaría con la nueva.
    for (let i = parts.length; i < SLOT_MAX_CHUNKS; i++)
      if (ctx.cookies.get(`${base}.${i}`))
        ctx.cookies.delete(`${base}.${i}`, { path: '/', secure: this.secure });
  }
  private deleteSlot(ctx: BffContext, id: string): void {
    const base = this.slotCookie(id);
    ctx.cookies.delete(base, { path: '/', secure: this.secure });
    for (let i = 1; i < SLOT_MAX_CHUNKS; i++)
      if (ctx.cookies.get(`${base}.${i}`))
        ctx.cookies.delete(`${base}.${i}`, { path: '/', secure: this.secure });
  }
  private revoked(jar: CookieJar, id: string): boolean {
    return jar.get(this.name('revoked_' + id))?.value === '1';
  }
  private cancelled(jar: CookieJar, id: string): boolean {
    return jar.get(this.name('cancelled_' + id))?.value === '1';
  }
  private completed(jar: CookieJar, id: string): boolean {
    return jar.get(this.name('completed_' + id))?.value === '1';
  }
  private async legacyID(cookie: string): Promise<string> {
    const key = await crypto.subtle.importKey(
      'raw',
      new TextEncoder().encode(this.secret),
      { name: 'HMAC', hash: 'SHA-256' },
      false,
      ['sign'],
    );
    const bytes = new Uint8Array(
      await crypto.subtle.sign(
        'HMAC',
        key,
        new TextEncoder().encode('sytue.auth.legacy-session-generation.v1\0' + cookie),
      ),
    );
    const id = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
    bytes.fill(0);
    return id;
  }
  async selection(ctx: BffContext): Promise<SessionSelection | null> {
    const raw = ctx.cookies.get(this.selectionCookie)?.value;
    if (!raw) return null;
    const value = await unseal<SessionSelection>(raw, this.secret);
    if (
      !value ||
      value.v !== 1 ||
      !reference(value.candidate) ||
      (value.fallback !== undefined && !reference(value.fallback))
    )
      return null;
    return value;
  }
  private async readRef(ctx: BffContext, ref: SessionSlotRef): Promise<SytueSession | null> {
    if (this.revoked(ctx.cookies, ref.id)) return null;
    const cookie = this.readSlot(ctx.cookies, ref.id);
    if (cookie) {
      const stored = await unseal<StoredSlot>(cookie, this.secret);
      return stored?.v === 1 && stored.slot === ref.id && sessionValue(stored.session)
        ? stored.session
        : null;
    }
    if (!ref.legacy) return null;
    const anchor = ctx.cookies.get(this.legacyCookie)?.value;
    if (!anchor) return null;
    const original = await unseal<SytueSession>(anchor, this.secret);
    if (!original?.user?.id || !original.tokens?.accessToken || !original.clientId) return null;
    const id =
      original.generation === undefined ? await this.legacyID(anchor) : original.generation;
    if (!isSessionGeneration(id) || id !== ref.id) return null;
    const session = { ...original, generation: id };
    if (!sessionValue(session)) return null;
    // Persist the assigned generation before any token refresh. The legacy
    // anchor remains immutable; no late request writes selection or base cookie.
    try {
      await this.persist(ctx, { ref, session }, session);
    } catch {
      // Response headers may already be sent when read inside an Astro layout component.
    }
    return session;
  }
  async read(ctx: BffContext): Promise<SelectedSession | null> {
    const pointerCookie = ctx.cookies.get(this.selectionCookie)?.value;
    if (pointerCookie) {
      const selected = await this.selection(ctx);
      if (!selected || this.revoked(ctx.cookies, selected.candidate.id)) return null;
      if (
        this.completed(ctx.cookies, selected.candidate.id) ||
        !this.cancelled(ctx.cookies, selected.candidate.id)
      ) {
        const cookie = this.readSlot(ctx.cookies, selected.candidate.id);
        if (cookie) {
          const session = await this.readRef(ctx, selected.candidate);
          return session ? { ref: selected.candidate, session, selection: selected } : null;
        }
        if (this.completed(ctx.cookies, selected.candidate.id)) return null;
      }
      // A lost completed cookie must not revive a previous account. Only an
      // explicit cancellation or an uncompleted, still-pending intent falls back.
      if (
        !this.cancelled(ctx.cookies, selected.candidate.id) &&
        ctx.cookies.get(this.name('pending_' + selected.candidate.id))?.value !== '1'
      )
        return null;
      if (!selected.fallback) return null;
      const session = await this.readRef(ctx, selected.fallback);
      return session ? { ref: selected.fallback, session, selection: selected } : null;
    }
    const anchor = ctx.cookies.get(this.legacyCookie)?.value;
    if (!anchor) return null;
    const original = await unseal<SytueSession>(anchor, this.secret);
    if (!original) return null;
    const id =
      original.generation === undefined ? await this.legacyID(anchor) : original.generation;
    if (!isSessionGeneration(id)) return null;
    const ref: SessionSlotRef = { id, legacy: true };
    const session = await this.readRef(ctx, ref);
    return session ? { ref, session } : null;
  }
  async begin(ctx: BffContext): Promise<SessionSelection> {
    const active = await this.read(ctx);
    const selection: SessionSelection = {
      v: 1,
      candidate: { id: newSessionGeneration() },
      ...(active ? { fallback: active.ref } : {}),
    };
    // Clean obsolete token/state slots only at login START. Refreshes and
    // callbacks never garbage-collect another intent. Keep revocation tombstones
    // until expiry, since an older response can still carry that slot.
    // Los trozos (`.1`, `.2`…) de una ranura vieja también: antes se quedaban.
    this.sweep(ctx, new Set([active?.ref.id, selection.candidate.id]));
    ctx.cookies.set(this.selectionCookie, await seal(selection, this.secret), this.attrs);
    ctx.cookies.delete(this.legacyCookie, { path: '/', secure: this.secure });
    ctx.cookies.set(this.name('pending_' + selection.candidate.id), '1', {
      ...this.attrs,
      maxAge: 1800,
    });
    return selection;
  }
  async persist(
    ctx: BffContext,
    captured: Pick<SelectedSession, 'ref' | 'session'>,
    session: SytueSession,
  ): Promise<void> {
    if (!reference(captured.ref) || !sessionValue(session)) throw Error('invalid session slot');
    const stored: StoredSlot = { v: 1, slot: captured.ref.id, session };
    this.writeSlot(ctx, captured.ref.id, await seal(stored, this.secret));
  }
  async complete(ctx: BffContext, id: string, session: SytueSession): Promise<void> {
    if (!isSessionGeneration(id) || session.generation !== id)
      throw Error('invalid session completion');
    await this.persist(ctx, { ref: { id }, session }, session);
    ctx.cookies.set(this.name('completed_' + id), '1', this.attrs);
    ctx.cookies.delete(this.name('pending_' + id), { path: '/', secure: this.secure });
    // Completado el intento (el seleccionado: lo comprueba `canComplete` o `adopt`),
    // la sesión anterior —la de reserva— ya no la lee nadie: `read` solo cae a ella
    // mientras el intento está pendiente o cancelado. Sin esto, cada vuelta de
    // desbloqueo (ADR 0084) dejaba DOS sesiones enteras en la cabecera `Cookie`
    // hasta el siguiente login. Solo se borra lo que trae ESTA petición: un login
    // empezado después no está aquí y no se toca.
    this.sweep(ctx, new Set([id]));
  }
  canComplete(ctx: BffContext, id: string): Promise<boolean> {
    return this.selection(ctx).then(
      (selected) =>
        selected?.candidate.id === id &&
        !this.revoked(ctx.cookies, id) &&
        !this.cancelled(ctx.cookies, id) &&
        !this.completed(ctx.cookies, id),
    );
  }
  private pending(jar: CookieJar, id: string): boolean {
    return (
      jar.get(this.name('pending_' + id))?.value === '1' &&
      !this.revoked(jar, id) &&
      !this.cancelled(jar, id) &&
      !this.completed(jar, id)
    );
  }
  /**
   * DOS LOGINS QUE EMPEZARON A LA VEZ. Una página sin sesión dispara varios: cada
   * petición en paralelo que recibe un 401 navega a `/auth/login` (Fotos lanza la
   * línea de tiempo, los álbumes y la cuenta a la vez), y la puerta del cifrado
   * puede sumar el suyo. Cada `/auth/login` escribe su intento como seleccionado,
   * y el navegador guarda las cookies de las respuestas EN EL ORDEN EN QUE LLEGAN,
   * no en el de las navegaciones: puede quedar seleccionado el intento de una
   * navegación ya cancelada mientras la que sigue viva vuelve con el suyo. Sin
   * sesión anterior a la que caer, ese callback acababa en un `409` en JSON a
   * pantalla completa y la app no llegaba a cargar.
   *
   * Se distingue de un callback viejo por las cookies que siguen vivas: un login
   * que empieza DESPUÉS de otro ve su estado y su `pending_` en la petición y los
   * borra (`begin`), así que si los dos siguen ahí es que ninguno vio al otro.
   * Entonces el que vuelve con su code (ligado a su PKCE) pasa a ser el
   * seleccionado y el otro queda cancelado. Nunca por encima de un intento ya
   * completado (sería cambiar de cuenta) ni de uno revocado por un logout.
   */
  async adopt(ctx: BffContext, id: string): Promise<boolean> {
    if (!isSessionGeneration(id)) return false;
    const selected = await this.selection(ctx);
    if (!selected || selected.candidate.id === id) return false;
    const other = selected.candidate.id;
    if (!this.pending(ctx.cookies, id)) return false;
    if (this.completed(ctx.cookies, other) || this.revoked(ctx.cookies, other)) return false;
    const adopted: SessionSelection = {
      v: 1,
      candidate: { id },
      ...(selected.fallback ? { fallback: selected.fallback } : {}),
    };
    ctx.cookies.set(this.selectionCookie, await seal(adopted, this.secret), this.attrs);
    this.cancel(ctx, other);
    ctx.cookies.delete(this.stateCookie(other), { path: '/', secure: this.secure });
    return true;
  }
  /** ¿Hay OTRO login distinto de `id` esperando su callback, o un logout de por medio? */
  async superseded(ctx: BffContext, id: string | undefined): Promise<boolean> {
    const selected = await this.selection(ctx);
    if (!selected) return false;
    const candidate = selected.candidate.id;
    if (this.revoked(ctx.cookies, candidate)) return true;
    return candidate !== id && this.pending(ctx.cookies, candidate);
  }
  cancel(ctx: BffContext, id: string): void {
    if (!isSessionGeneration(id) || this.completed(ctx.cookies, id)) return;
    ctx.cookies.set(this.name('cancelled_' + id), '1', this.attrs);
    ctx.cookies.delete(this.name('pending_' + id), { path: '/', secure: this.secure });
  }
  async revokeCurrent(ctx: BffContext): Promise<void> {
    const selected = await this.selection(ctx),
      active = await this.read(ctx);
    const ids = new Set<string>();
    if (selected) ids.add(selected.candidate.id);
    if (active) ids.add(active.ref.id);
    for (const id of ids) {
      ctx.cookies.set(this.name('revoked_' + id), '1', this.attrs);
      this.deleteSlot(ctx, id);
    }
    // Never touch a selected pointer in a logout response. A newer login may
    // already own it by the time this response reaches the browser.
    ctx.cookies.delete(this.legacyCookie, { path: '/', secure: this.secure });
  }
  guard(ctx: BffContext, selected: SelectedSession | null): Response | null {
    const expected = ctx.request.headers.get(sessionOwnerHeader);
    if (expected === null) return null;
    if (!selected)
      return Response.json(
        { error: 'unauthorized' },
        { status: 401, headers: { 'cache-control': 'no-store' } },
      );
    return isSessionGeneration(expected) && selected.session.generation === expected
      ? null
      : sessionChangedResponse();
  }
}
