/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LO QUE UNA PÁGINA DE OFIMÁTICA HACE ANTES DE PINTARSE, una sola vez.
 *
 * Las pantallas viven en `vistas/` y cada app (`apps/<producto>/src/pages`) solo las
 * monta. Pero un componente de Astro no puede contestar con una redirección: eso lo
 * hace el frontmatter de la PÁGINA. Por eso lo que acaba en una `Response` —el salto
 * al login, el de la forma canónica con la organización (ADR 0056), el 404 de un
 * enlace que no existe— está aquí, y la página lo llama y lo devuelve si toca:
 *
 *     const sesion = await entrar(Astro, { canonica: true });
 *     if (sesion instanceof Response) return sesion;
 *
 * Estaba escrito cuatro veces, una por página, idéntico.
 */
import type { AstroGlobal } from 'astro';
import type { DocKind } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { canonicaConOrg, returnToCanonico } from '@sytue/config';
import { bff, docsBaseUrl, toBffContext } from '../lib/auth';
import { kindDeDocumento } from '../lib/docsProxy';
import { cabecerasDeEnlace } from '../lib/enlaceClave';
import { editorDeOtroProducto } from '../lib/producto';

/**
 * La sesión de quien abre la página, o la redirección que toca.
 *
 * `canonica`: la ruta está en `sinOrg` del middleware —su nombre colisiona con el
 * namespace de organizaciones (`edit`, `trash`, `pdf`)— y, con sesión, se salta a la
 * forma con la org activa. Sin ese salto la isla hidrataría creyendo que `edit` ES la
 * organización y «copiar enlace» daría `/docs/edit/edit?id=…`.
 */
export async function entrar(
  Astro: AstroGlobal,
  { canonica = false }: { canonica?: boolean } = {},
): Promise<SytueSession | Response> {
  const sesion = await bff.getSession(toBffContext(Astro));
  if (!sesion) {
    // La entrada org-ful llega reescrita con la marca ?__org=; el returnTo sale
    // canónico, no en la forma interna, o tras el login la barra se queda con ella.
    const returnTo = returnToCanonico(Astro.url, bff.basePath);
    const login = Astro.redirect(
      `${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`,
    );
    login.headers.set('cache-control', 'no-store');
    return login;
  }
  if (canonica) {
    const destino = canonicaConOrg(Astro.url, bff.basePath, sesion.user.tenant);
    if (destino) return Astro.redirect(destino);
  }
  return sesion;
}

/**
 * La entrada del editor (`/edit?id=…`): sesión, forma canónica, el id y su tipo.
 *
 * `kind` es el de la app que la sirve (Calcus abre hojas). Un documento de OTRO tipo
 * se manda a la app de su producto con la misma org: el enlace lo pudo componer
 * alguien que no sabía el tipo —un aviso viejo, una búsqueda, un «abrir» genérico— y
 * abrir una presentación en Calcus no es una opción. Sin `kind`, la app conjunta,
 * que los abre todos. Si el servicio no dice el tipo (caído, sin permiso), se abre
 * aquí y el editor dirá lo que pase, como siempre.
 */
export async function entrarEnDocumento(
  Astro: AstroGlobal,
  kind?: DocKind,
): Promise<{ sesion: SytueSession; documentId: string; kind: DocKind | null } | Response> {
  const sesion = await entrar(Astro, { canonica: true });
  if (sesion instanceof Response) return sesion;
  const documentId = Astro.url.searchParams.get('id');
  if (!documentId) return Astro.redirect('/');
  const real = await kindDeDocumento(sesion.tokens.accessToken, documentId);
  const otra = editorDeOtroProducto(kind, real, {
    id: documentId,
    volver: Astro.url.searchParams.get('volver'),
    org: Astro.locals.org ?? sesion.user.tenant ?? null,
  });
  if (otra) return Astro.redirect(otra);
  return { sesion, documentId, kind: real };
}

/** Un documento compartido por enlace tal y como lo da el servicio para leerlo. */
export interface VistaCompartida {
  id: string;
  title: string;
  kind: 'document' | 'spreadsheet' | 'presentation';
  role: 'viewer' | 'commenter' | 'editor';
  html: string;
}

/**
 * El documento de `/s/{token}`, `null` si el enlace pide contraseña, o el 404.
 *
 * Cualquier fallo —token caducado, revocado, inventado o el servicio caído— se
 * contesta con el MISMO 404: distinguirlos diría qué tokens son válidos.
 */
export async function vistaCompartida(
  Astro: AstroGlobal,
): Promise<VistaCompartida | null | Response> {
  const token = Astro.params.token ?? '';
  let vista: VistaCompartida | null = null;
  let pideClave = false;
  try {
    const res = await fetch(
      `${docsBaseUrl().replace(/\/$/, '')}/api/v1/share/${encodeURIComponent(token)}/view`,
      { headers: { accept: 'application/json', ...cabecerasDeEnlace(Astro, token) } },
    );
    if (res.ok) vista = (await res.json()) as VistaCompartida;
    else if (res.status === 401) pideClave = true;
  } catch {
    // servicio caído: igual que un token que no existe.
  }
  if (!vista && !pideClave) return new Response(null, { status: 404 });
  // Lo que se ve depende del token: ninguna caché intermedia lo guarda.
  Astro.response.headers.set('cache-control', 'no-store');
  return vista;
}
