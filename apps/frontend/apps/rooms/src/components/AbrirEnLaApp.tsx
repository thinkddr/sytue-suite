/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El aviso de «esta reunión se abre en la app»: tras abrirla («Se ha abierto en la app de
 * Sytue Rooms»), o para ofrecerla donde probar el esquema a ciegas rompería la página
 * (ver lib/enLaApp.ts). Siempre con la salida de entrar aquí en el navegador, y con
 * «Abrir siempre en la app» para no volver a preguntar.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { Button, Checkbox, useFitInViewport } from '@sytue/ui';
import { recordarEnLaApp, siempreEnLaApp } from '../lib/enLaApp.js';
import type { T } from '../i18n/index.js';

interface Props {
  abierta: boolean;
  t: T;
  onAbrir: () => void;
  onAqui: () => void;
}

export function AbrirEnLaApp({ abierta, t, onAbrir, onAqui }: Props): JSX.Element {
  const [siempre, setSiempre] = useState(siempreEnLaApp());
  const ref = useRef<HTMLDivElement>(null);
  useFitInViewport(ref, true, { vertical: true });
  return (
    <div
      ref={ref}
      class="sytue-popover-surface border-border fixed bottom-[max(1.5rem,var(--sytue-safe-bottom,0px))] left-1/2 z-50 flex w-[min(28rem,calc(100%-2rem))] flex-col gap-3 rounded-lg border p-4 shadow-[var(--shadow-lg)]"
      style={{ transform: 'translateX(-50%)' }}
      role="status"
    >
      <p class="text-text text-sm">{abierta ? t('meet.inApp.opened') : t('meet.inApp.offer')}</p>
      <label class="text-text-muted flex items-center gap-2 text-sm">
        <Checkbox
          class="accent-primary"
          checked={siempre}
          onChange={(e) => {
            const si = e.currentTarget.checked;
            setSiempre(si);
            recordarEnLaApp(si);
          }}
        />
        {t('meet.inApp.always')}
      </label>
      <div class="flex flex-wrap justify-end gap-2">
        <Button size="sm" variant="secondary" onClick={onAqui}>
          {t('meet.inApp.stayHere')}
        </Button>
        <Button size="sm" variant="primary" onClick={onAbrir}>
          {t('meet.inApp.open')}
        </Button>
      </div>
    </div>
  );
}
