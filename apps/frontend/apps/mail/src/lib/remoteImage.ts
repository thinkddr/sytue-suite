/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PROXY DE IMÁGENES REMOTAS DE SYTUE MAIL (FE-03).
 *
 * El cuerpo de un correo se pinta en un iframe `srcdoc` que hereda la CSP del
 * documento padre. Para poder cerrar `img-src` en Mail sin romper «Mostrar
 * imágenes», las imágenes remotas que la persona decide desbloquear se piden a
 * nuestro BFF (`/api/remote-image?url=…`), que:
 *
 *   1. exige sesión válida (nadie de fuera usa el BFF como proxy abierto);
 *   2. valida la URL y todas las IP resueltas por DNS (solo `https:` en el
 *      puerto 443 y ninguna IP privada, de loopback, link-local o reservada,
 *      revalidando cada salto de redirección);
 *   3. exige un `Content-Type` de imagen rasterizada (`image/*` sin SVG);
 *   4. corta la descarga si supera el tope de tamaño (`TOPE_BYTES_IMAGEN_REMOTA`).
 *
 * Además, el servidor del remitente ya no recibe la IP ni el User-Agent del
 * navegador de quien lee el correo.
 */
import { lookup } from 'node:dns/promises';
import { isIP } from 'node:net';

/** Formatos rasterizados que el navegador solo decodifica como píxeles (sin SVG). */
export const IMAGEN_REMOTA_SEGURA =
  /^image\/(jpeg|png|gif|webp|avif|bmp|x-icon|vnd\.microsoft\.icon)$/;

/** Tope por imagen remota (10 MiB). */
export const TOPE_BYTES_IMAGEN_REMOTA = 10 * 1024 * 1024;

const MAX_REDIRECCIONES = 3;
const TIMEOUT_MS = 10_000;

export type ResolverDns = (hostname: string) => Promise<readonly string[]>;

async function resolverDnsPorDefecto(hostname: string): Promise<readonly string[]> {
  const res = await lookup(hostname, { all: true, verbatim: true });
  return res.map((r) => r.address);
}

function esIpv4PrivadaOReservada(ip: string): boolean {
  const partes = ip.split('.');
  if (partes.length !== 4) return true;
  const octetos = partes.map((p) => (/^\d+$/.test(p) ? Number(p) : -1));
  if (octetos.some((n) => n < 0 || n > 255)) return true;
  const [a, b, c] = octetos as [number, number, number, number];
  if (a === 0 || a === 10 || a === 127) return true;
  if (a === 100 && b >= 64 && b <= 127) return true; // CGNAT 100.64.0.0/10
  if (a === 169 && b === 254) return true; // link-local / metadata 169.254.0.0/16
  if (a === 172 && b >= 16 && b <= 31) return true; // RFC 1918 172.16.0.0/12
  if (a === 192 && b === 0 && (c === 0 || c === 2)) return true; // 192.0.0.0/24 y TEST-NET-1
  if (a === 192 && b === 168) return true; // RFC 1918 192.168.0.0/16
  if (a === 198 && (b === 18 || b === 19)) return true; // benchmark 198.18.0.0/15
  if (a === 198 && b === 51 && c === 100) return true; // TEST-NET-2
  if (a === 203 && b === 0 && c === 113) return true; // TEST-NET-3
  if (a >= 224) return true; // multicast y reservadas
  return false;
}

/**
 * ¿Es `ip` una dirección privada, de loopback, link-local, multicast, de
 * documentación o mal formada? Ante la duda, devuelve `true` (bloquear).
 */
export function esIpPrivadaOReservada(rawIp: string): boolean {
  const ip = rawIp
    .trim()
    .replace(/^\[|\]$/g, '')
    .toLowerCase();
  const version = isIP(ip);
  if (version === 4) return esIpv4PrivadaOReservada(ip);
  if (version !== 6) return true;

  if (ip === '::' || ip === '::1' || ip.startsWith('0:0:0:0:0:0:0:')) {
    return true;
  }
  // IPv4 mapeada en IPv6 (::ffff:a.b.c.d o ::ffff:7f00:0001).
  const mapeadaDecimal = /^::ffff:(\d+\.\d+\.\d+\.\d+)$/.exec(ip);
  if (mapeadaDecimal) return esIpv4PrivadaOReservada(mapeadaDecimal[1]!);
  const mapeadaHex = /^::ffff:([0-9a-f]{1,4}):([0-9a-f]{1,4})$/.exec(ip);
  if (mapeadaHex) {
    const hi = Number.parseInt(mapeadaHex[1]!, 16);
    const lo = Number.parseInt(mapeadaHex[2]!, 16);
    const v4 = `${(hi >> 8) & 0xff}.${hi & 0xff}.${(lo >> 8) & 0xff}.${lo & 0xff}`;
    return esIpv4PrivadaOReservada(v4);
  }
  // Unique local (fc00::/7), link-local (fe80::/10), multicast (ff00::/8) y doc (2001:db8::/32).
  if (/^(fc|fd|fe[89ab]|ff)/.test(ip)) return true;
  if (/^2001:0?db8:/.test(ip)) return true;
  return false;
}

function esNombreInterno(hostname: string): boolean {
  const h = hostname
    .trim()
    .replace(/^\[|\]$/g, '')
    .toLowerCase();
  if (!h) return true;
  if (
    h === 'localhost' ||
    h.endsWith('.localhost') ||
    h.endsWith('.local') ||
    h.endsWith('.internal') ||
    h.endsWith('.svc') ||
    h.endsWith('.cluster.local')
  ) {
    return true;
  }
  if (isIP(h) === 0 && !h.includes('.')) return true;
  return false;
}

/**
 * Valida que `rawUrl` sea una URL `https:` en el puerto por defecto y que su
 * host resuelva exclusivamente a direcciones IP públicas.
 */
export async function validarUrlImagenRemota(
  rawUrl: string,
  resolverDns: ResolverDns = resolverDnsPorDefecto,
): Promise<URL> {
  let u: URL;
  try {
    u = new URL(rawUrl.trim());
  } catch {
    throw new Error('invalid_url');
  }
  if (u.protocol !== 'https:') throw new Error('https_required');
  if (u.username || u.password) throw new Error('credentials_not_allowed');
  if (u.port !== '' && u.port !== '443') throw new Error('port_not_allowed');

  const host = u.hostname.replace(/^\[|\]$/g, '').toLowerCase();
  if (esNombreInterno(host)) throw new Error('ssrf_blocked');

  if (isIP(host) !== 0) {
    if (esIpPrivadaOReservada(host)) throw new Error('ssrf_blocked');
    return u;
  }

  let direcciones: readonly string[];
  try {
    direcciones = await resolverDns(host);
  } catch {
    throw new Error('dns_error');
  }
  if (direcciones.length === 0 || direcciones.some((ip) => esIpPrivadaOReservada(ip))) {
    throw new Error('ssrf_blocked');
  }
  return u;
}

export type ResultadoImagenRemota =
  | { ok: true; bytes: Uint8Array; contentType: string }
  | { ok: false; status: number; error: string };

export interface OpcionesImagenRemota {
  fetchImpl?: typeof fetch;
  resolverDns?: ResolverDns;
  topeBytes?: number;
  maxRedirects?: number;
  signal?: AbortSignal;
}

/**
 * Descarga una imagen remota aplicando anti-SSRF en cada salto, filtro de
 * `Content-Type` (`image/*` rasterizada) y tope de tamaño en streaming.
 */
export async function descargarImagenRemota(
  rawUrl: string,
  opciones: OpcionesImagenRemota = {},
): Promise<ResultadoImagenRemota> {
  const fetchImpl = opciones.fetchImpl ?? fetch;
  const resolverDns = opciones.resolverDns ?? resolverDnsPorDefecto;
  const topeBytes = opciones.topeBytes ?? TOPE_BYTES_IMAGEN_REMOTA;
  const maxRedirects = opciones.maxRedirects ?? MAX_REDIRECCIONES;

  let actual = rawUrl;
  for (let salto = 0; salto <= maxRedirects; salto++) {
    let destino: URL;
    try {
      destino = await validarUrlImagenRemota(actual, resolverDns);
    } catch (err) {
      const codigo = err instanceof Error ? err.message : 'invalid_url';
      const status = codigo === 'ssrf_blocked' ? 403 : 400;
      return { ok: false, status, error: codigo };
    }

    let res: Response;
    try {
      const timeout = AbortSignal.timeout(TIMEOUT_MS);
      const signal = opciones.signal ? AbortSignal.any([opciones.signal, timeout]) : timeout;
      res = await fetchImpl(destino.toString(), {
        method: 'GET',
        redirect: 'manual',
        credentials: 'omit',
        referrerPolicy: 'no-referrer',
        headers: {
          accept: 'image/avif,image/webp,image/png,image/jpeg,image/gif,*/*;q=0.1',
        },
        signal,
      });
    } catch {
      return { ok: false, status: 502, error: 'upstream_error' };
    }

    if (res.status >= 300 && res.status < 400) {
      void res.body?.cancel();
      const loc = res.headers.get('location');
      if (!loc || salto === maxRedirects) {
        return { ok: false, status: 502, error: 'too_many_redirects' };
      }
      try {
        actual = new URL(loc, destino).toString();
      } catch {
        return { ok: false, status: 400, error: 'invalid_redirect' };
      }
      continue;
    }

    if (!res.ok) {
      void res.body?.cancel();
      return { ok: false, status: 502, error: 'upstream_status' };
    }

    const tipo = (res.headers.get('content-type') ?? '').split(';')[0]!.trim().toLowerCase();
    if (!IMAGEN_REMOTA_SEGURA.test(tipo)) {
      void res.body?.cancel();
      return { ok: false, status: 415, error: 'unsupported_media_type' };
    }

    const lenRaw = res.headers.get('content-length');
    if (lenRaw !== null) {
      const len = Number.parseInt(lenRaw, 10);
      if (Number.isFinite(len) && len > topeBytes) {
        void res.body?.cancel();
        return { ok: false, status: 413, error: 'image_too_large' };
      }
    }

    if (!res.body) {
      return { ok: false, status: 502, error: 'empty_body' };
    }

    const reader = res.body.getReader();
    const trozos: Uint8Array[] = [];
    let total = 0;
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        if (!value) continue;
        total += value.byteLength;
        if (total > topeBytes) {
          await reader.cancel();
          return { ok: false, status: 413, error: 'image_too_large' };
        }
        trozos.push(value);
      }
    } catch {
      return { ok: false, status: 502, error: 'read_error' };
    }

    const bytes = new Uint8Array(total);
    let offset = 0;
    for (const t of trozos) {
      bytes.set(t, offset);
      offset += t.byteLength;
    }
    return { ok: true, bytes, contentType: tipo };
  }

  return { ok: false, status: 502, error: 'too_many_redirects' };
}

/**
 * Neutraliza cualquier referencia a `/api/remote-image` que ya viniera escrita
 * en el HTML del mensaje, para que un remitente no pueda saltarse el botón
 * «Mostrar imágenes» apuntando directamente al proxy de nuestro origen (`'self'`).
 */
export function neutralizarProxyEnCuerpo(html: string): string {
  return html.replace(/\/api\/remote-image\b/gi, '/api/blocked-image');
}

/**
 * Reescribe las URL `https://…` de imágenes remotas del cuerpo HTML (`src`,
 * `background`, `poster`, `srcset` y `url(…)` en estilos) para que pasen por el
 * proxy del BFF cuando la persona pulsa «Mostrar imágenes».
 */
export function reescribirImagenesRemotas(
  html: string,
  proxyUrlDe: (urlRemota: string) => string,
): string {
  const conAtributos = html.replace(
    /(\s(?:src|background|poster)\s*=\s*)(["'])(https:\/\/[^"'\s>]+)\2/gi,
    (_m, prefijo: string, comilla: string, url: string) =>
      `${prefijo}${comilla}${proxyUrlDe(url)}${comilla}`,
  );
  const conSrcset = conAtributos.replace(
    /(\ssrcset\s*=\s*)(["'])([^"']+)\2/gi,
    (_m, prefijo: string, comilla: string, valor: string) => {
      const reescrito = valor
        .split(',')
        .map((entrada) => {
          const limpio = entrada.trim();
          if (!/^https:\/\//i.test(limpio)) return entrada;
          const partes = limpio.split(/\s+/);
          const url = partes[0]!;
          const resto = partes.slice(1).join(' ');
          return resto ? `${proxyUrlDe(url)} ${resto}` : proxyUrlDe(url);
        })
        .join(', ');
      return `${prefijo}${comilla}${reescrito}${comilla}`;
    },
  );
  return conSrcset.replace(
    /url\(\s*(["']?)(https:\/\/[^"')]+)\1\s*\)/gi,
    (_m, comilla: string, url: string) => {
      const q = comilla || '"';
      return `url(${q}${proxyUrlDe(url)}${q})`;
    },
  );
}
