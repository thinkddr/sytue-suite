/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El grupo «Tema» de la cinta, común a Docus, Calcus y Diapos: aplica uno de los temas
 * del catálogo común o la marca de la organización al documento abierto. Los temas los da
 * el servicio (GET /api/themes) y aplicarlos (ctx.applyTheme) solo cambia la presentación:
 * el contenido no se toca y queda un punto de restauración. Quien administra la
 * organización ve además «Editar la marca…», que abre su diálogo.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { DocThemesResponse } from '@sytue/api-clients';
import { ActualizarMarcaDialog } from '../dialogs/ActualizarMarcaDialog';
import { MarcaDialog } from '../dialogs/MarcaDialog';
import { NOMBRE_DE_TEMA } from '../temas-nombres';
import type { EditorContext } from '../types';
import { RibbonGroup, RibbonSelect } from './primitives';

const EDITAR_MARCA = '__editar-marca';
const ACTUALIZAR_MARCA = '__actualizar-marca';

/** Carga el catálogo y la marca; la marca puede cambiar (se guarda una versión nueva) y
 *  `recargar` lo vuelve a pedir. */
export function useTemas(): { datos: DocThemesResponse | null; recargar: () => void } {
  const [datos, setDatos] = useState<DocThemesResponse | null>(null);
  const [n, setN] = useState(0);
  useEffect(() => {
    let vigente = true;
    fetch('/api/themes', { credentials: 'include' })
      .then((res) => (res.ok ? (res.json() as Promise<DocThemesResponse>) : null))
      .then((d) => vigente && setDatos(d))
      .catch(() => undefined);
    return () => {
      vigente = false;
    };
  }, [n]);
  return { datos, recargar: () => setN((v) => v + 1) };
}

export function TemaGroup({ ctx }: { ctx: EditorContext }): JSX.Element | null {
  const { t, previewTheme: prever } = ctx;
  const { datos, recargar } = useTemas();
  const [marcaAbierta, setMarcaAbierta] = useState(false);
  const [actualizando, setActualizando] = useState(false);
  if (!datos) return null;
  const marca = datos.brand;
  const opciones = [
    { value: '', label: t('theme.placeholder') },
    ...(marca
      ? [
          {
            value: 'marca',
            label: t('theme.brand', { name: marca.name ?? '', version: marca.version ?? 1 }),
          },
        ]
      : []),
    ...(marca && prever ? [{ value: ACTUALIZAR_MARCA, label: t('theme.update') }] : []),
    ...datos.themes.map((x) => ({
      value: x.id,
      label: NOMBRE_DE_TEMA[x.id] ? t(NOMBRE_DE_TEMA[x.id]!) : x.id,
    })),
    ...(datos.can_manage ? [{ value: EDITAR_MARCA, label: t('theme.editBrand') }] : []),
  ];
  return (
    <>
      <RibbonGroup label={t('theme.group')}>
        <RibbonSelect
          label={t('theme.select')}
          width={150}
          value=""
          disabled={ctx.soloLectura || ctx.connection === 'offline'}
          options={opciones}
          onSelect={(v) => {
            if (v === EDITAR_MARCA) setMarcaAbierta(true);
            else if (v === ACTUALIZAR_MARCA) setActualizando(true);
            else if (v) void ctx.applyTheme(v);
          }}
        />
      </RibbonGroup>
      {actualizando && prever && (
        <ActualizarMarcaDialog
          t={t}
          onClose={() => setActualizando(false)}
          previsualizar={() => prever('marca')}
          aplicar={() => ctx.applyTheme('marca')}
        />
      )}
      {marcaAbierta && (
        <MarcaDialog
          open
          t={t}
          actual={datos.versions[0] ?? null}
          onClose={() => setMarcaAbierta(false)}
          onSaved={recargar}
        />
      )}
    </>
  );
}
