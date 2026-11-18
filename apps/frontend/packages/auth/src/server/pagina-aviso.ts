/**
 * La página de aviso del BFF: un documento suelto, en el idioma de la persona, con
 * un título, una frase y UN enlace para seguir. La usan las respuestas a las que se
 * llega NAVEGANDO —el 403 del guard, el callback que no puede terminar—, donde un
 * JSON o un texto plano dejan a la persona delante de un callejón sin salida (y
 * Safari ofrece descargarlos como fichero).
 *
 * ponytail: documento suelto con los tokens en línea, NO componentes de
 * @sytue/ui. El BFF es una librería de servidor sin Preact ni el CSS de la app
 * (cada app lo compila con su propio hash), así que no hay bundle que reutilizar;
 * traerse preact-render-to-string y @sytue/ui a `@sytue/auth` por una pantalla de
 * aviso costaría más que las quince líneas de estilo que hay aquí. Si algún día
 * la suite sirve una hoja de estilo compartida en una URL estable, esto se
 * sustituye por un `<link>` y el marcado se queda igual.
 */
import { brand, fontFamily, themeColors } from '@sytue/config';
import { LOCALE_COOKIE, htmlLangAttrs, isLocale, negotiateLocale, type Locale } from '@sytue/i18n';
import type { BffContext } from './bff.js';

/** Idioma de la página: la elección explícita (cookie) > Accept-Language > inglés. */
export function idiomaDeLaPeticion(ctx: BffContext): Locale {
  // La preferencia explícita se lee por el JAR de cookies —el mismo por el que el
  // BFF lee la sesión— y no por la cabecera cruda: `Cookie` es un nombre de
  // cabecera prohibido, así que hay entornos que la descartan en silencio y el
  // idioma elegido por el usuario se perdería sin dar la cara. El resto de la
  // negociación (Accept-Language > inglés) lo sigue decidiendo `negotiateLocale`.
  const elegida = ctx.cookies.get(LOCALE_COOKIE)?.value;
  return isLocale(elegida)
    ? elegida
    : negotiateLocale(
        ctx.request.headers.get('accept-language'),
        ctx.request.headers.get('cookie'),
      );
}

function escapar(t: string): string {
  return t.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}

export interface Aviso {
  locale: Locale;
  titulo: string;
  descripcion: string;
  accion: string;
  /** A dónde lleva el enlace. Lo construye el servidor; se escapa igualmente. */
  href: string;
  status: number;
  headers?: Record<string, string>;
}

export function paginaDeAviso({
  locale,
  titulo,
  descripcion,
  accion,
  href,
  status,
  headers,
}: Aviso): Response {
  const { lang, dir } = htmlLangAttrs(locale);
  const html = `<!doctype html>
<html lang="${lang}" dir="${dir}" data-theme="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>${escapar(titulo)} — Sytue One</title>
<style>
:root{color-scheme:light;--bg:${themeColors.light.bg};--fg:${themeColors.light.text};--muted:${themeColors.light['text-muted']};--link:${brand.primary[700]};--ring:${brand.primary[500]}}
*{box-sizing:border-box}
body{margin:0;min-height:100dvh;display:grid;place-items:center;padding:2rem;
background:var(--bg);color:var(--fg);font-family:${fontFamily.sans};
-webkit-font-smoothing:antialiased}
main{max-width:34rem;text-align:center}
h1{margin:0;font-size:1.5rem;line-height:1.3;font-weight:600;letter-spacing:-0.01em}
p{margin:0.75rem 0 0;font-size:1rem;line-height:1.6;color:var(--muted)}
a{display:inline-block;margin-top:1.75rem;color:var(--link);font-size:1rem;font-weight:600;
text-decoration:underline;text-underline-offset:3px;border-radius:0.25rem}
a:focus-visible{outline:2px solid var(--ring);outline-offset:3px}
</style>
</head>
<body>
<main>
<h1>${escapar(titulo)}</h1>
<p>${escapar(descripcion)}</p>
<a href="${escapar(href)}">${escapar(accion)}</a>
</main>
</body>
</html>`;
  return new Response(html, {
    status,
    headers: {
      ...headers,
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-store',
    },
  });
}
