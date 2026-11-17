/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The person's wrapped Matrix recovery keys (ADR 0082, 0084), reachable from EVERY app
 * with a Matrix gate: each app re-exports these handlers bound to its own product, like
 * `rutaBootstrapMatrixPrivado`. The BFF passes wrappers through and never sees a key:
 * they are wrapped and opened in the browser with the unlock key of the person's Sytue
 * ID sign-in (and, the older ones, with what that sign-in computed for them).
 *
 * Account: without `identity_version=2` the Rooms account (the one Rooms and the legacy
 * private storage share); with it, this app's isolated storage account. The product
 * comes from the app, never from the browser.
 */
import type { APIRoute } from 'astro';
import { appDomains } from '@sytue/config';
import { isStorageProduct, type MatrixStorageProduct } from '@sytue/config/matrix';
import { ChatClient, bearer, upstreamStatus } from '@sytue/api-clients';
import { serverEnv, type EnvLike } from '../config.js';
import { sesionApi } from './app-bff.js';
import type { Bff } from './bff.js';

type MatrixRecoveryAccount = Parameters<ChatClient['matrixRecoveryWrappers']>[0];
type MatrixRecoveryWrapper = Awaited<
  ReturnType<ChatClient['matrixRecoveryWrappers']>
>['wrappers'][number];

const recoveryKeyShape = /^[1-9A-HJ-NP-Za-km-z]{4}(?: [1-9A-HJ-NP-Za-km-z]{1,4}){8,24}$/;
const base64 = /^[A-Za-z0-9+/]+={0,2}$/;
const base64url = /^[A-Za-z0-9_-]*$/;

/** Only the fields of format v1, with their shapes: nothing else reaches the service. */
function wrapperDTO(input: unknown): MatrixRecoveryWrapper | null {
  const w = input as Partial<MatrixRecoveryWrapper> | null;
  if (!w || typeof w !== 'object' || w.v !== 1) return null;
  if (
    typeof w.key_id !== 'string' ||
    !/^[A-Za-z0-9._~-]{1,255}$/.test(w.key_id) ||
    typeof w.credential_id !== 'string' ||
    w.credential_id.length > 1024 ||
    !base64url.test(w.credential_id) ||
    typeof w.nonce !== 'string' ||
    !base64.test(w.nonce) ||
    typeof w.ciphertext !== 'string' ||
    w.ciphertext.length > 4096 ||
    !base64.test(w.ciphertext)
  )
    return null;
  const kdf = w.kdf;
  if (
    w.kind === 'sytue-id' &&
    kdf?.alg === 'sytue-id-unlock-hkdf-sha256' &&
    w.credential_id === '' &&
    typeof kdf.hint === 'string' &&
    base64.test(kdf.hint)
  )
    return {
      v: 1,
      kind: 'sytue-id',
      key_id: w.key_id,
      credential_id: '',
      kdf: { alg: 'sytue-id-unlock-hkdf-sha256', hint: kdf.hint },
      nonce: w.nonce,
      ciphertext: w.ciphertext,
    };
  if (w.kind === 'password' && kdf?.alg === 'argon2id' && w.credential_id === '') {
    if (
      ![kdf.m, kdf.t, kdf.p].every(Number.isSafeInteger) ||
      typeof kdf.salt !== 'string' ||
      !base64.test(kdf.salt)
    )
      return null;
    return {
      v: 1,
      kind: 'password',
      key_id: w.key_id,
      credential_id: '',
      kdf: { alg: 'argon2id', m: kdf.m, t: kdf.t, p: kdf.p, salt: kdf.salt },
      nonce: w.nonce,
      ciphertext: w.ciphertext,
    };
  }
  if (
    w.kind === 'passkey' &&
    kdf?.alg === 'webauthn-prf-hkdf-sha256' &&
    w.credential_id !== '' &&
    typeof kdf.prf_salt === 'string' &&
    base64.test(kdf.prf_salt)
  )
    return {
      v: 1,
      kind: 'passkey',
      key_id: w.key_id,
      credential_id: w.credential_id,
      kdf: { alg: 'webauthn-prf-hkdf-sha256', prf_salt: kdf.prf_salt },
      nonce: w.nonce,
      ciphertext: w.ciphertext,
    };
  return null;
}

export function rutasRecuperacionMatrix(options: {
  bff: Bff;
  env: EnvLike;
  /** The app's storage product: its isolated account is the only other one it reaches. */
  product: MatrixStorageProduct;
  fetchImpl?: typeof fetch;
}): {
  /** `<base>/api/matrix/recovery-wrappers` */
  wrappers: { GET: APIRoute; PUT: APIRoute };
  /** `<base>/api/matrix/recovery-key`: read once to migrate the retired ADR 0081 copy. */
  legacyKey: { GET: APIRoute };
} {
  if (!isStorageProduct(options.product)) throw new Error('unsupported_matrix_product');
  const product = options.product,
    sessionFor = sesionApi(options.bff);
  const env = serverEnv(options.env);
  const baseUrl = env.SYTUE_ROOMS_INTERNAL_URL ?? env.SYTUE_ROOMS_API_URL ?? appDomains.rooms;
  const headers = { 'cache-control': 'no-store', vary: 'Cookie' };
  const client = (accessToken: string) =>
    new ChatClient({
      baseUrl,
      auth: bearer(accessToken),
      ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
    });
  /** The account from the query, or a Response refusing it. */
  const account = (
    request: Request,
    tenant: string | undefined,
  ): MatrixRecoveryAccount | undefined | Response => {
    const versions = new URL(request.url).searchParams.getAll('identity_version');
    if (versions.length === 0) return undefined;
    if (versions.length > 1 || versions[0] !== '2')
      return Response.json({ error: 'matrix_identity_version' }, { status: 400, headers });
    if (!tenant) return Response.json({ error: 'organisation_required' }, { status: 409, headers });
    return { product, identity_version: 2 };
  };
  const unavailable = (error: unknown) =>
    Response.json({ error: 'matrix_unavailable' }, { status: upstreamStatus(error), headers });

  const GET: APIRoute = async (context) => {
    const session = await sessionFor(context);
    if (!session) return Response.json({ error: 'unauthenticated' }, { status: 401, headers });
    const target = account(context.request, session.user.tenant);
    if (target instanceof Response) return target;
    try {
      const data = await client(session.accessToken).matrixRecoveryWrappers(
        target,
        context.request.signal,
      );
      if (!Array.isArray(data.wrappers))
        return Response.json({ error: 'matrix_unavailable' }, { status: 502, headers });
      const wrappers: MatrixRecoveryWrapper[] = [];
      for (const raw of data.wrappers) {
        const dto = wrapperDTO(raw);
        if (!dto) continue;
        if (typeof raw.id === 'string') dto.id = raw.id;
        if (typeof raw.created_at === 'string') dto.created_at = raw.created_at;
        wrappers.push(dto);
      }
      return Response.json(
        { wrappers, legacy_escrow: target === undefined && data.legacy_escrow === true },
        { headers },
      );
    } catch (error) {
      return unavailable(error);
    }
  };
  const PUT: APIRoute = async (context) => {
    const session = await sessionFor(context);
    if (!session) return Response.json({ error: 'unauthenticated' }, { status: 401, headers });
    const target = account(context.request, session.user.tenant);
    if (target instanceof Response) return target;
    if (Number(context.request.headers.get('content-length')) > 8192)
      return Response.json({ error: 'invalid_input' }, { status: 413, headers });
    const text = await context.request.text().catch(() => '');
    if (text.length > 8192)
      return Response.json({ error: 'invalid_input' }, { status: 413, headers });
    let parsed: unknown = null;
    try {
      parsed = JSON.parse(text);
    } catch {
      /* invalid below */
    }
    const wrapper = wrapperDTO(parsed);
    if (!wrapper) return Response.json({ error: 'invalid_input' }, { status: 400, headers });
    try {
      const { id } = await client(session.accessToken).putMatrixRecoveryWrapper(wrapper, target);
      return Response.json({ id }, { headers });
    } catch (error) {
      return unavailable(error);
    }
  };
  const legacyGET: APIRoute = async (context) => {
    const session = await sessionFor(context);
    if (!session) return Response.json({ error: 'unauthenticated' }, { status: 401, headers });
    try {
      const { recovery_key } = await client(session.accessToken).matrixRecoveryKey(
        context.request.signal,
      );
      if (typeof recovery_key !== 'string' || !recoveryKeyShape.test(recovery_key))
        return Response.json({ error: 'matrix_unavailable' }, { status: 502, headers });
      return Response.json({ recovery_key }, { headers });
    } catch (error) {
      return unavailable(error);
    }
  };
  return { wrappers: { GET, PUT }, legacyKey: { GET: legacyGET } };
}
