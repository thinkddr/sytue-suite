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
 * the selected intent; callbacks and refreshes can never replace a later login.
 * Tombstones are separate cookies, so an old refresh cannot erase revocation. */
export class SessionSlots {
  readonly selectionCookie: string;
  readonly legacyCookie: string;
  private readonly attrs: Record<string, unknown>;
  constructor(
    private readonly secret: string,
    private readonly secure: boolean,
    maxAge: number,
  ) {
    this.attrs = { path: '/', httpOnly: true, secure, sameSite: 'lax', maxAge };
    this.selectionCookie = this.name('selection');
    this.legacyCookie = hostCookieName(SESSION_COOKIE, secure);
  }
  private name(suffix: string): string {
    return hostCookieName(SESSION_COOKIE + '_' + suffix, this.secure);
  }
  stateCookie(id: string): string {
    if (!isSessionGeneration(id)) throw Error('invalid session intent');
    return this.name('oidc_' + id);
  }
  private slotCookie(id: string): string {
    return this.name('slot_' + id);
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
    const cookie = ctx.cookies.get(this.slotCookie(ref.id))?.value;
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
        const cookie = ctx.cookies.get(this.slotCookie(selected.candidate.id))?.value;
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
    const keep = new Set([active?.ref.id, selection.candidate.id]);
    const prefix = this.name('');
    for (const field of (ctx.request.headers.get('cookie') ?? '').split(';')) {
      const name = field.trim().split('=', 1)[0] ?? '';
      if (!name.startsWith(prefix)) continue;
      const slot = /^(?:slot|completed|cancelled|pending|oidc)_([a-f0-9]{64})$/.exec(
        name.slice(prefix.length),
      );
      if (slot?.[1] && !keep.has(slot[1]))
        ctx.cookies.delete(name, { path: '/', secure: this.secure });
    }
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
    ctx.cookies.set(this.slotCookie(captured.ref.id), await seal(stored, this.secret), this.attrs);
  }
  async complete(ctx: BffContext, id: string, session: SytueSession): Promise<void> {
    if (!isSessionGeneration(id) || session.generation !== id)
      throw Error('invalid session completion');
    await this.persist(ctx, { ref: { id }, session }, session);
    ctx.cookies.set(this.name('completed_' + id), '1', this.attrs);
    ctx.cookies.delete(this.name('pending_' + id), { path: '/', secure: this.secure });
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
      ctx.cookies.delete(this.slotCookie(id), { path: '/', secure: this.secure });
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
