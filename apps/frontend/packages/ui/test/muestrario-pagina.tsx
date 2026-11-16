// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/**
 * La página del muestrario: `Muestrario` con los mandos de los tres ejes del tema.
 *
 * Solo para revisar a mano; no la sirve ninguna app. `test/e2e/compilar-muestrario.mjs`
 * la compila con su hoja en un directorio que se abre en el navegador sin servidor.
 *
 * Los ejes salen de la URL y van por `useTheme` —los mismos `setMode`, `setColor` y
 * `setForma` que el selector de apariencia de la suite—, así que lo que se ve es lo que
 * ve la persona que elige ese tema:
 *
 *   index.html?modo=dark&color=cobalto&forma=carril&abrir=dialogos&familia=campos,menus
 *
 * `abrir=dialogos:paleta` (o `confirmar`, `atajos`, `editor`) abre una sola capa de esa familia.
 *
 * `idioma` cambia el de los textos de los componentes (los de muestra van en español).
 */
import { render } from 'preact';
import { useEffect } from 'preact/hooks';
import { isLocale } from '@sytue/i18n';
import { I18nProvider } from '@sytue/i18n/preact';
import { Select, ThemeProvider, useTheme } from '../src/index.js';
import type { ThemeColor, ThemeForma, ThemeMode } from '../src/index.js';
import { COLORES, FAMILIAS, FORMAS, MODOS, Muestrario } from './muestrario.js';

const params = new URL(location.href).searchParams;
const elegido = <T extends string>(clave: string, validos: readonly T[]): T | undefined => {
  const v = params.get(clave);
  return validos.find((x) => x === v);
};

function Mandos() {
  const { mode, setMode, color, setColor, forma, setForma } = useTheme();
  useEffect(() => {
    const modo = elegido<ThemeMode>('modo', MODOS);
    const c = elegido<ThemeColor>('color', COLORES);
    const f = elegido<ThemeForma>('forma', FORMAS);
    if (modo) setMode(modo);
    if (c) setColor(c);
    if (f) setForma(f);
    // Solo al montar: después mandan los selectores.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return (
    <header class="bg-surface border-border sticky top-0 z-10 flex flex-wrap items-center gap-3 border-b px-4 py-2">
      <h1 class="text-text font-semibold">Muestrario de @sytue/ui</h1>
      <Select
        size="sm"
        aria-label="Modo"
        value={mode}
        onChange={(e) => setMode(e.currentTarget.value as ThemeMode)}
      >
        {MODOS.map((m) => (
          <option key={m} value={m}>
            {m}
          </option>
        ))}
      </Select>
      <Select
        size="sm"
        aria-label="Color"
        value={color}
        onChange={(e) => setColor(e.currentTarget.value as ThemeColor)}
      >
        {COLORES.map((c) => (
          <option key={c} value={c}>
            {c}
          </option>
        ))}
      </Select>
      <Select
        size="sm"
        aria-label="Forma"
        value={forma}
        onChange={(e) => setForma(e.currentTarget.value as ThemeForma)}
      >
        {FORMAS.map((f) => (
          <option key={f} value={f}>
            {f}
          </option>
        ))}
      </Select>
      <nav aria-label="Familias" class="flex flex-wrap gap-2">
        {FAMILIAS.map((f) => (
          <a key={f.clave} class="text-primary-text text-sm" href={`#familia-${f.clave}`}>
            {f.titulo}
          </a>
        ))}
      </nav>
    </header>
  );
}

const idioma = params.get('idioma');
const familias = params.get('familia')?.split(',').filter(Boolean);

render(
  <I18nProvider locale={isLocale(idioma) ? idioma : 'es'}>
    <ThemeProvider>
      <Mandos />
      <Muestrario familias={familias} abrir={params.get('abrir') ?? undefined} />
    </ThemeProvider>
  </I18nProvider>,
  document.body,
);
