/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS VACÍOS SALEN DE `EmptyState`, Y SU MARCA SE PIDE POR NOMBRE.
 *
 * Un mismo «no hay nada» se pintaba de cuatro maneras: un `<p>` gris suelto (con su
 * propio `py-*` y su tamaño de letra), un `<p>` centrado, un `EmptyState` con un icono
 * a 28, 32, 36 o 40 y otro con la ilustración a 96 o 160. Y los fallos de carga salían
 * a veces con la misma cara que el vacío: un fallo leído como «no hay nada».
 *
 * Aquí se cuentan, por ámbito (`apps/<app>` o `packages/<paquete>`):
 *
 * 1. los `<p>` cuyo único contenido es un `t('….empty|none|noResults…')` (o un ternario
 *    que empieza por uno): el vacío escrito a mano. Va `<EmptyState compact title=…>`;
 * 2. los `illustration={<Icon …>}` / `illustration={<Illustration …>}`: la marca con
 *    tamaño libre. Va `icon=` o `spot=`, que fijan el tamaño (y `tone="error"`, que
 *    pone el glifo de peligro, si es un fallo).
 *
 * SI ESTE TEST TE PARA: no añadas un vacío suelto; usa `EmptyState` de '@sytue/ui'.
 * Las cifras de abajo son lo que QUEDA por migrar y se comparan POR IGUALDAD: si
 * bajas una, bájala aquí; si un ámbito llega a [0, 0], bórralo.
 */
import { expect, test } from 'bun:test';

const RAIZ = new URL('../../../../', import.meta.url).pathname; // apps/frontend/

const COMILLA = `['"\`]`;
/** `<p …>{t('x.empty')}</p>` o `<p …>{cond ? t('x.emptyQuery') : t('y')}</p>`. */
const P_VACIO = new RegExp(
  String.raw`<p\b[^>]*>\s*\{(?:[^{}<?]*\?\s*)?t\(\s*${COMILLA}[\w.-]*\.(?:empty|none|noResults)(?:Query|Yet)?${COMILLA}[^<]*?\}\s*<\/p>`,
  'g',
);
const MARCA_LIBRE = /\billustration=\{\s*<(?:Icon|Illustration)\b/g;

/**
 * Ámbito → [`<p>` vacíos sueltos, marcas libres]. Las apps de `ADOPTADAS` ya están a
 * cero y NO pueden aparecer aquí.
 */
const PENDIENTES: Record<string, [number, number]> = {
  'packages/office': [24, 0],
};

const ADOPTADAS = [
  'mail',
  'drive',
  'contacts',
  'calendar',
  'notes',
  'photos',
  'rooms',
  'account',
  'bases',
  'cast',
  'cloud-dashboard',
  'console',
  'erp',
  'pages',
  'portal',
];

async function contar(): Promise<Record<string, [number, number]>> {
  const cuenta: Record<string, [number, number]> = {};
  const glob = new Bun.Glob('{apps,packages}/*/src/**/*.{ts,tsx,astro}');
  for await (const rel of glob.scan({ cwd: RAIZ })) {
    if (rel.includes('.test.')) continue;
    // Sin comentarios: la documentación también escribe `illustration={<Icon …>}`.
    const codigo = (await Bun.file(RAIZ + rel).text()).replace(
      /\/\*[\s\S]*?\*\/|^\s*\/\/.*$/gm,
      '',
    );
    const vacios = codigo.match(P_VACIO)?.length ?? 0;
    const marcas = codigo.match(MARCA_LIBRE)?.length ?? 0;
    if (!vacios && !marcas) continue;
    const ambito = rel.split('/').slice(0, 2).join('/');
    const [v, m] = cuenta[ambito] ?? [0, 0];
    cuenta[ambito] = [v + vacios, m + marcas];
  }
  return cuenta;
}

test('el patrón reconoce el vacío suelto y deja pasar lo que no lo es', () => {
  const casos: [string, number][] = [
    [`<p class="text-text-muted text-sm">{t('trash.empty')}</p>`, 1],
    [`<p class="x">\n  {q.trim() ? t('forward.emptyQuery') : t('forward.empty')}\n</p>`, 1],
    [`<p>{t("search.noResults", { term })}</p>`, 1],
    [`<p>{t('message.noneYet')}</p>`, 1],
    // El cuerpo de un diálogo de «Vaciar» no es un vacío.
    [`<p class="x">{t('trash.emptyAllBody', { count: 3 })}</p>`, 0],
    [`<p>{vaciando ? t('trash.emptyConfirm') : t('trash.purgeConfirm')}</p>`, 0],
    // Un texto con un vacío de respaldo tampoco.
    [`<p>{extracto(v, t('conflict.empty'))}</p>`, 0],
    [`<EmptyState compact title={t('trash.empty')} />`, 0],
  ];
  for (const [codigo, n] of casos)
    expect([codigo, codigo.match(P_VACIO)?.length ?? 0]).toEqual([codigo, n]);
});

test('los vacíos salen de EmptyState y su marca va por nombre', async () => {
  const cuenta = await contar();
  for (const app of ADOPTADAS) expect([app, cuenta[`apps/${app}`]]).toEqual([app, undefined]);
  expect(cuenta).toEqual(PENDIENTES);
});
