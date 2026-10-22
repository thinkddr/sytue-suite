/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Actualizar a la marca vigente»: antes de aplicar la última versión de la marca a un
 * documento ya creado enseña, en lenguaje llano, qué cambiará (títulos, cabeceras de tabla,
 * tipografía, cabecera de hoja o tema de la presentación). Es solo presentación: el
 * contenido no se toca, el servicio deja el punto de restauración «Antes de la marca vN»
 * y aplicarla otra vez no cambia nada. El diff lo calcula el servicio (`preview`), el
 * mismo código que escribe, así que lo que se lee aquí es lo que se aplica.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Modal } from '@sytue/ui';
import type { DocsKey, DocsT } from '../../i18n';
import type { CambioTema, TemaPrevia } from '../types';

export interface ActualizarMarcaDialogProps {
  t: DocsT;
  onClose: () => void;
  previsualizar: () => Promise<TemaPrevia | null>;
  aplicar: () => Promise<void>;
}

const CLAVE_DE_CAMBIO: Record<Exclude<CambioTema['scope'], 'logo'>, DocsKey> = {
  headings: 'theme.change.headings',
  table_headers: 'theme.change.table_headers',
  body_font: 'theme.change.body_font',
  sheet_header: 'theme.change.sheet_header',
  slide_theme: 'theme.change.slide_theme',
};

/** El texto de un cambio; un valor vacío es «el de por defecto». */
export function lineaDeCambio(t: DocsT, c: CambioTema): string {
  if (c.scope === 'logo') return t(c.to ? 'theme.change.logoSet' : 'theme.change.logoRemoved');
  const de = c.from || t('theme.change.unset');
  return t(CLAVE_DE_CAMBIO[c.scope], {
    count: c.count,
    from: de,
    to: c.to ?? '',
    sheet: c.sheet ?? '',
  });
}

export function ActualizarMarcaDialog({
  t,
  onClose,
  previsualizar,
  aplicar,
}: ActualizarMarcaDialogProps): JSX.Element {
  const [previa, setPrevia] = useState<TemaPrevia | null | 'cargando'>('cargando');
  const [aplicando, setAplicando] = useState(false);
  useEffect(() => {
    let vigente = true;
    void previsualizar().then((p) => vigente && setPrevia(p));
    return () => {
      vigente = false;
    };
    // `previsualizar` llega como flecha nueva en cada render: se pide una vez, al abrir.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const hay = previa !== 'cargando' && previa !== null && previa.changed;
  return (
    <Modal
      open
      onClose={onClose}
      title={t('theme.update.title')}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('action.cancel')}
          </Button>
          <Button
            disabled={!hay || aplicando}
            onClick={() => {
              setAplicando(true);
              void aplicar().finally(onClose);
            }}
          >
            {t('theme.update.apply')}
          </Button>
        </>
      }
    >
      {previa === 'cargando' ? (
        <p class="text-text-muted text-sm" aria-busy="true">
          {t('theme.update.loading')}
        </p>
      ) : previa === null ? (
        <p class="text-sm">{t('theme.update.failed')}</p>
      ) : !previa.changed ? (
        <p class="text-sm">{t('theme.update.none')}</p>
      ) : (
        <div class="flex flex-col gap-3">
          <p class="text-text-muted text-sm">
            {t('theme.update.intro', { version: previa.version })}
          </p>
          <ul class="flex list-disc flex-col gap-1 pl-5 text-sm">
            {previa.changes.map((c) => (
              <li key={`${c.scope}:${c.sheet ?? ''}`}>{lineaDeCambio(t, c)}</li>
            ))}
          </ul>
        </div>
      )}
    </Modal>
  );
}
