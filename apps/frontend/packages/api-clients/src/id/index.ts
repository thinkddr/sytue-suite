/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue ID — SDK al servicio Go de identidad (id.sytue.eu, dev :8081).
 * OIDC/OAuth2 (Authorization Code + PKCE, refresh, JWKS, discovery), SCIM y
 * MFA/WebAuthn. Implementacion propia (clean-room) sobre librerias Apache-2.0
 * (ory/fosite o zitadel/oidc). NO es Keycloak. El flujo OIDC+PKCE lo realiza
 * @sytue/auth contra el BFF; este SDK cubre discovery, userinfo y consultas de
 * cuenta/perfil/organizaciones que las apps necesitan.
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

export interface IdOptions {
  /** URL base, p.ej. https://id.sytue.eu */
  baseUrl: string;
  /** Opcional: sin auth solo sirven discovery/JWKS (publicos). */
  auth?: AuthProvider | undefined;
  fetchImpl?: typeof fetch | undefined;
}

/** Documento de descubrimiento OIDC (RFC 8414 / OpenID Connect Discovery 1.0). */
export interface OidcDiscovery {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  userinfo_endpoint: string;
  jwks_uri: string;
  end_session_endpoint?: string;
  registration_endpoint?: string;
  scopes_supported?: string[];
  response_types_supported?: string[];
  code_challenge_methods_supported?: string[];
}

/** JSON Web Key Set (RFC 7517) publicado por Sytue ID. */
export interface Jwks {
  keys: JsonWebKey[];
}

/** Claims estandar de userinfo (OIDC) + extensiones de tenancy Sytue. */
export interface UserInfo {
  sub: string;
  email?: string;
  email_verified?: boolean;
  name?: string;
  given_name?: string;
  family_name?: string;
  preferred_username?: string;
  picture?: string;
  locale?: string;
  /** Organizaciones (tenants) a las que pertenece: slug -> rol. */
  organizations?: OrgMembership[];
}

/** Rol del usuario en una organizacion (tenant). docs/CONVENTIONS.md §8. */
export type OrgRole = 'owner' | 'admin' | 'member' | 'billing';

export interface OrgMembership {
  /** Slug del tenant (grupo `org/<slug>` en Sytue ID). */
  slug: string;
  displayName: string;
  role: OrgRole;
}

/** Credencial WebAuthn registrada para el usuario. */
export interface WebAuthnCredential {
  /** UUID de la credencial en Sytue ID: el que aceptan las rutas de esta API. */
  id: string;
  /** El identificador WebAuthn (base64url). Lo usa el navegador, no la API. */
  credential_id: string;
  name: string;
  created_at: string;
  last_used?: string | null;
}

export class IdClient {
  private readonly http: HttpClient;
  private discovery: OidcDiscovery | null = null;

  constructor(options: IdOptions) {
    this.http = new HttpClient({
      baseUrl: options.baseUrl,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /** Documento de descubrimiento OIDC (cacheado). Publico. */
  async discover(): Promise<OidcDiscovery> {
    if (this.discovery) return this.discovery;
    this.discovery = await this.http.json<OidcDiscovery>({
      path: '/.well-known/openid-configuration',
      anonymous: true,
    });
    return this.discovery;
  }

  /** JWKS publico para validar firmas de los JWT (lo consume cada servicio Go). */
  async jwks(): Promise<Jwks> {
    const disc = await this.discover();
    return this.http.json<Jwks>({ url: disc.jwks_uri, anonymous: true });
  }

  /** Claims del usuario autenticado (userinfo). Requiere token. */
  async userInfo(): Promise<UserInfo> {
    const disc = await this.discover();
    return this.http.json<UserInfo>({ url: disc.userinfo_endpoint });
  }

  /** Credenciales WebAuthn registradas (gestion de MFA en la cuenta).
   * El router de Sytue ID monta /webauthn DENTRO de /api/v1 pero FUERA de /me. */
  async webAuthnCredentials(): Promise<WebAuthnCredential[]> {
    // El servicio envuelve en `{credentials: [...]}` y llama `name` a lo que esto
    // pedía como `label`. Tal como estaba, la lista llegaba siempre vacía y los
    // rótulos `undefined`: nadie lo llamaba todavía, así que no rompía nada
    // visible, que es justo lo que lo hacía fácil de no ver.
    const res = await this.http.json<{ credentials?: WebAuthnCredential[] }>({
      path: '/api/v1/webauthn/credentials',
    });
    return res.credentials ?? [];
  }

  /** URL de fin de sesion (logout) OIDC, con redireccion opcional. */
  async endSessionUrl(
    postLogoutRedirectUri?: string,
    idTokenHint?: string,
  ): Promise<string | undefined> {
    const disc = await this.discover();
    if (!disc.end_session_endpoint) return undefined;
    return this.http.buildUrl({
      url: disc.end_session_endpoint,
      query: {
        post_logout_redirect_uri: postLogoutRedirectUri,
        id_token_hint: idTokenHint,
      },
    });
  }
}
