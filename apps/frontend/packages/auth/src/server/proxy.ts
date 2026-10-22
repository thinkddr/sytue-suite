/**
 * Proxy genérico del BFF hacia el servicio Go correspondiente.
 *
 * El navegador tiene una cookie de sesión, no un token: aquí se cambia una cosa
 * por la otra —cookie validada, `Authorization: Bearer` hacia dentro— y el token
 * no baja nunca al cliente.
 *
 * EL CUERPO VA EN STREAMING en los dos sentidos (duplex: 'half').
 * No bufferiza en memoria para no tumbar la réplica con cargas pesadas.
 *
 * Hacia dentro reenvía: content-type, content-length, range, if-none-match, idempotency-key.
 * Hacia fuera reenvía: content-type, content-length, cache-control, etag, content-range, content-disposition.
 * Nunca reenvía cabeceras X-Sytue-*. `x-forwarded-for` solo en las escrituras sin sesión.
 * Soporta 304 Not Modified sin cuerpo, 400 si la ruta decodificada contiene `..` o `\`, y 503 legible si el servicio está caído.
 */
import type { APIRoute } from 'astro';
import type { Bff } from './bff.js';
import type { BffDeApp } from './app-bff.js';
import { toBffContext } from './app-bff.js';

export interface OpcionesProxyDeServicio {
  destino: string;
  bff: Bff | BffDeApp;
  /** Prefijo interno que se antepone a la ruta. Por defecto '/api/'. */
  prefijo?: string;
  /** Rutas que no requieren sesión (por ejemplo: ['/api/v1/public/']). Solo lectura GET/HEAD. */
  rutasPublicas?: string[];
  /**
   * Escrituras (POST) que el servicio admite SIN sesión, una expresión regular por
   * ruta exacta: una denuncia de quien mira la portada pública, por ejemplo. Van sin
   * token aunque haya sesión, con la IP de quien llama (`x-forwarded-for`) —el
   * servicio limita y deduplica por ella— y con el cuerpo acotado a
   * `MAX_ESCRITURA_PUBLICA`. Lo que haya que comprobar (una prueba de trabajo) lo
   * comprueba el servicio.
   */
  escriturasPublicas?: RegExp[];
  /** Rutas denegadas que nunca se reenvían al servicio interno. */
  denegar?: string[] | ((ruta: string) => boolean);
}

/** Tope del cuerpo de una escritura sin sesión: un JSON pequeño, no una subida. */
export const MAX_ESCRITURA_PUBLICA = 16 * 1024;

/** Cabeceras de la petición que el servicio necesita ver. */
const HACIA_DENTRO = [
  'content-type',
  'content-length',
  'range',
  'if-none-match',
  'idempotency-key',
] as const;

/** Cabeceras de la respuesta que se devuelven tal cual. */
const HACIA_FUERA = [
  'content-type',
  'content-length',
  'cache-control',
  'etag',
  'content-range',
  'content-disposition',
] as const;

/**
 * Si una ruta que viene del navegador puede salirse de donde se la reenvía (FE-01).
 *
 * Se mira DECODIFICADA DEL TODO: Astro entrega la ruta pasada por `decodeURI`, que deja
 * `%2F` tal cual. Y se mira lo que `fetch` hará con ella DESPUÉS de comprobar aquí: su
 * parser de URL lee `\` como `/` en http(s) y borra tabuladores y saltos de línea, así
 * que `a\..\x` o `.%09.` (que aquí no casan `..`) salen como `..`, y `web%09hooks` no
 * casa `webhooks` en una lista de denegadas pero sí lo es al salir. Ninguna ruta legítima
 * lleva una barra invertida ni un carácter de control. La usa todo BFF que componga un
 * destino con la ruta pedida.
 */
export function rutaQueEscapa(raw: string): boolean {
  let ruta: string;
  try {
    ruta = decodeURIComponent(raw);
  } catch {
    return true;
  }
  for (let i = 0; i < ruta.length; i++) {
    const c = ruta.charCodeAt(i);
    if (c < 0x20 || c === 0x5c) return true; // un carácter de control o una barra invertida
  }
  return ruta.split('/').includes('..');
}

export function crearProxyDeServicio(opts: OpcionesProxyDeServicio): APIRoute {
  const destino = opts.destino.replace(/\/$/, '');
  const prefijo = opts.prefijo ?? '/api/';

  return async (ctx) => {
    const raw = ctx.params.ruta ?? '';
    if (rutaQueEscapa(raw)) return new Response(null, { status: 400 });
    const ruta = `${prefijo}${raw}`.replace(/\/+/g, '/');

    if (opts.denegar) {
      const denegada =
        typeof opts.denegar === 'function'
          ? opts.denegar(ruta)
          : opts.denegar.some((d) => ruta.startsWith(d));
      if (denegada) {
        return new Response(null, { status: 403 });
      }
    }

    const esLectura = ctx.request.method === 'GET' || ctx.request.method === 'HEAD';
    const esEscrituraPublica =
      ctx.request.method === 'POST' && (opts.escriturasPublicas ?? []).some((re) => re.test(ruta));
    if (esEscrituraPublica) {
      // La longitud declarada, antes de leer nada: el servidor HTTP no entrega más
      // bytes de los que anuncia la cabecera.
      const largo = Number(ctx.request.headers.get('content-length') ?? Number.NaN);
      if (!Number.isFinite(largo)) return new Response(null, { status: 411 });
      if (largo > MAX_ESCRITURA_PUBLICA) return new Response(null, { status: 413 });
    }
    const esPublica =
      esEscrituraPublica ||
      (esLectura && (opts.rutasPublicas ?? []).some((pub) => ruta.startsWith(pub)));

    const bffInstancia = 'getSession' in opts.bff ? opts.bff : opts.bff.bff;
    const sesion = esPublica ? null : await bffInstancia.getSession(toBffContext(ctx));
    if (!sesion && !esPublica) {
      return new Response(null, { status: 401 });
    }

    const url = new URL(ctx.request.url);
    const cabeceras: Record<string, string> = sesion
      ? { authorization: `Bearer ${sesion.tokens.accessToken}` }
      : {};

    if (esEscrituraPublica) {
      const xff = ctx.request.headers.get('x-forwarded-for');
      if (xff) cabeceras['x-forwarded-for'] = xff;
    }

    for (const n of HACIA_DENTRO) {
      const v = ctx.request.headers.get(n);
      if (v) cabeceras[n] = v;
    }

    const conCuerpo = ctx.request.method !== 'GET' && ctx.request.method !== 'HEAD';

    let res: Response;
    try {
      res = await fetch(destino + ruta + url.search, {
        method: ctx.request.method,
        headers: cabeceras,
        body: conCuerpo ? ctx.request.body : undefined,
        // @ts-expect-error duplex es obligatorio al enviar un cuerpo en streaming
        duplex: 'half',
      });
    } catch {
      return new Response(
        JSON.stringify({ error: { code: 'unavailable', message: 'Service unavailable' } }),
        {
          status: 503,
          headers: { 'content-type': 'application/json' },
        },
      );
    }

    if (res.status === 304) {
      const salida304 = new Headers();
      for (const n of HACIA_FUERA) {
        const v = res.headers.get(n);
        if (v) salida304.set(n, v);
      }
      return new Response(null, { status: 304, headers: salida304 });
    }

    const salida = new Headers();
    for (const n of HACIA_FUERA) {
      const v = res.headers.get(n);
      if (v) salida.set(n, v);
    }
    salida.set('x-content-type-options', 'nosniff');
    return new Response(res.body, { status: res.status, headers: salida });
  };
}
