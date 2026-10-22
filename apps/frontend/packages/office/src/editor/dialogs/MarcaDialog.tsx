/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La marca de la organización: nombre, fondo, color del texto, acento y las tipografías
 * de títulos y cuerpo. Solo quien administra la organización llega aquí (el servicio lo
 * exige: PUT /api/themes). Guardar NO pisa la marca anterior: crea una versión nueva y
 * las anteriores se conservan, de modo que un documento puede volver a la que usó.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Modal, Select, useToast } from '@sytue/ui';
import type { DocBrandVersion } from '@sytue/api-clients';
import type { DocsT } from '../../i18n';
import { MAX_THEME_NAME, THEME_FONTS, normalizeHex } from '../slides/temas';

export interface MarcaDialogProps {
  open: boolean;
  onClose: () => void;
  t: DocsT;
  /** La vigente, para partir de ella; ausente = la organización aún no tiene marca. */
  actual: DocBrandVersion | null;
  /** Se llama con el número de la versión guardada, para refrescar quien pintó la lista. */
  onSaved: (version: number) => void;
}

const POR_DEFECTO = { name: '', bg: '#FFFFFF', ink: '#0A0A0A', accent: '#1F2937' };

export function MarcaDialog({ open, onClose, t, actual, onSaved }: MarcaDialogProps): JSX.Element {
  const toast = useToast();
  const [m, setM] = useState({
    ...POR_DEFECTO,
    heading_font: '',
    body_font: '',
    ...(actual
      ? {
          name: actual.name,
          bg: actual.bg,
          ink: actual.ink,
          accent: actual.accent,
          heading_font: actual.heading_font ?? '',
          body_font: actual.body_font ?? '',
        }
      : {}),
  });
  const [guardando, setGuardando] = useState(false);
  const siguiente = (actual?.version ?? 0) + 1;
  const fuentes = (f: string): readonly string[] =>
    f && !THEME_FONTS.includes(f) ? [f, ...THEME_FONTS] : THEME_FONTS;

  const guardar = async (): Promise<void> => {
    setGuardando(true);
    try {
      const res = await fetch('/api/themes', {
        method: 'PUT',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(m),
      });
      if (!res.ok) throw new Error(String(res.status));
      const { version } = (await res.json()) as { version: number };
      toast.notify({ tone: 'success', title: t('brand.msg.saved', { version }) });
      onSaved(version);
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('brand.msg.failed') });
    } finally {
      setGuardando(false);
    }
  };

  // Los campos de la suite (`FieldGroup`, `Input`, `Select`) y no los de 24 px de la
  // cinta (`FIELD`): en el teléfono todos medían 24 con el dedo, y con `justify-between`
  // cada campo de color empezaba en una x distinta según lo largo de su rótulo.
  const fuente = (clave: 'heading_font' | 'body_font', rotulo: string): JSX.Element => (
    <FieldGroup label={rotulo} htmlFor={`marca-${clave}`}>
      <Select
        id={`marca-${clave}`}
        value={m[clave]}
        onChange={(e) => setM({ ...m, [clave]: e.currentTarget.value })}
      >
        <option value="">{t('brand.fontDefault')}</option>
        {fuentes(m[clave]).map((f) => (
          <option key={f} value={f}>
            {f}
          </option>
        ))}
      </Select>
    </FieldGroup>
  );

  // Un color: el rótulo (que parte de línea si es largo) y, en su columna a la derecha,
  // la muestra con su filete —el fondo #FFFFFF no se veía sobre el panel— y el hex.
  const color = (clave: 'bg' | 'ink' | 'accent', rotulo: string): JSX.Element => {
    const valor = clave === 'accent' ? (normalizeHex(m.accent) ?? m.accent) : m[clave];
    const poner = (crudo: string): boolean => {
      const hex = normalizeHex(crudo);
      if (hex) setM({ ...m, [clave]: hex });
      return Boolean(hex);
    };
    return (
      <div class="flex items-center justify-between gap-3">
        <label class="text-text min-w-0 text-sm font-medium" for={`marca-${clave}`}>
          {rotulo}
        </label>
        <div class="flex shrink-0 items-center gap-2">
          <input
            type="color"
            aria-label={rotulo}
            class="border-border rounded-control sytue-focus-ring h-9 w-11 cursor-pointer border bg-transparent p-0.5 pointer-coarse:min-h-[max(44px,2.75rem)]"
            value={valor}
            // `change` y no `input`: el selector dispara `input` en cada paso del arrastre.
            onChange={(e) => void poner(e.currentTarget.value)}
          />
          <Input
            id={`marca-${clave}`}
            class="w-28 font-mono"
            value={valor}
            maxLength={7}
            spellcheck={false}
            onChange={(e) => {
              if (!poner(e.currentTarget.value)) e.currentTarget.value = valor;
            }}
          />
        </div>
      </div>
    );
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('brand.title')}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('action.cancel')}
          </Button>
          <Button onClick={() => void guardar()} disabled={guardando || !m.name.trim()}>
            {t('brand.save')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        <FieldGroup label={t('brand.name')} htmlFor="marca-nombre">
          <Input
            id="marca-nombre"
            value={m.name}
            maxLength={MAX_THEME_NAME}
            onInput={(e) => setM({ ...m, name: e.currentTarget.value })}
          />
        </FieldGroup>
        <div class="flex flex-col gap-2">
          {color('bg', t('brand.bg'))}
          {color('ink', t('brand.ink'))}
          {color('accent', t('brand.accent'))}
        </div>
        {fuente('heading_font', t('brand.headingFont'))}
        {fuente('body_font', t('brand.bodyFont'))}
        <p class="sytue-meta">{t('brand.version', { version: siguiente })}</p>
      </div>
    </Modal>
  );
}
