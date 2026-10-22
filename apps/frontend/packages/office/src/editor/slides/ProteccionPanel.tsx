/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel inferior de la protección de la presentación: qué se bloquea (la diapositiva activa,
 * el objeto seleccionado, el patrón y el tema) y con qué contraseña. La protección la cumple
 * el servidor (domain/proteccion_diapos.go); la lógica pura está en proteger.ts y aquí solo
 * el pintado. Quien no puede cambiar la protección ve el estado y, si hay contraseña, el
 * campo para desbloquear.
 */
import type { JSX } from 'preact';
import { Checkbox } from '@sytue/ui';
import { useT } from '../context';
import type { SlidesProtection } from '../types';

const FOCUS_RING = 'sytue-focus-ring';
const FIELD = `border-border bg-surface text-text h-6 min-h-[24px] w-40 rounded-sm border px-1 text-xs ${FOCUS_RING}`;
const BTN = `text-text-muted hover:bg-bg-subtle inline-flex h-6 min-h-[24px] items-center gap-1 rounded-sm px-2 text-xs ${FOCUS_RING}`;

function Casilla(props: {
  label: string;
  checked: boolean;
  disabled: boolean;
  onChange: () => void;
}): JSX.Element {
  return (
    <label class="text-text-muted flex items-center gap-1 text-xs">
      <Checkbox checked={props.checked} disabled={props.disabled} onChange={props.onChange} />
      {props.label}
    </label>
  );
}

/** Un campo de contraseña que entrega su valor al pulsar Intro o al salir, y se vacía. */
function Clave(props: { label: string; onEnter: (clave: string) => void }): JSX.Element {
  const entrega = (el: HTMLInputElement): void => {
    if (el.value === '') return;
    props.onEnter(el.value);
    el.value = '';
  };
  return (
    <input
      type="password"
      class={FIELD}
      autocomplete="new-password"
      placeholder={props.label}
      aria-label={props.label}
      onKeyDown={(e) => {
        if (e.key === 'Enter') entrega(e.currentTarget);
      }}
      onChange={(e) => entrega(e.currentTarget)}
    />
  );
}

export function ProteccionPanel(props: {
  protect: SlidesProtection | undefined;
  /** La diapositiva activa y el objeto seleccionado están bloqueados. */
  slideLocked: boolean;
  boxLocked: boolean;
  /** Hay un objeto seleccionado que se pueda bloquear. */
  hayCaja: boolean;
  /** Propietario, o con la contraseña presentada: puede cambiar la protección. */
  puede: boolean;
  /** Protege la presentación, con la contraseña tecleada ('' = sin contraseña). */
  onProtect: (clave: string) => void;
  onToggleSlide: () => void;
  onToggleBox: () => void;
  onToggleMaster: () => void;
  onPassword: (clave: string) => void;
  onUnlock: (clave: string) => void;
  onOff: () => void;
}): JSX.Element {
  const t = useT();
  const { protect: p, puede } = props;
  return (
    <div class="border-border bg-surface flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-t px-3 py-1.5">
      <span class="text-text text-xs font-semibold">{t('slides.protect')}</span>
      {!p ? (
        <>
          <Clave label={t('slides.protect.password')} onEnter={props.onProtect} />
          <button type="button" class={BTN} onClick={() => props.onProtect('')}>
            {t('slides.protect.on')}
          </button>
        </>
      ) : (
        <>
          <Casilla
            label={t('slides.protect.slide')}
            checked={props.slideLocked}
            disabled={!puede}
            onChange={props.onToggleSlide}
          />
          <Casilla
            label={t('slides.protect.box')}
            checked={props.boxLocked}
            disabled={!puede || !props.hayCaja}
            onChange={props.onToggleBox}
          />
          <Casilla
            label={t('slides.protect.master')}
            checked={p.master === true}
            disabled={!puede}
            onChange={props.onToggleMaster}
          />
          {puede ? (
            <>
              <Clave label={t('slides.protect.password')} onEnter={props.onPassword} />
              <button type="button" class={BTN} onClick={props.onOff}>
                {t('slides.protect.off')}
              </button>
            </>
          ) : (
            p.hasPassword === true && (
              <Clave label={t('slides.protect.unlock')} onEnter={props.onUnlock} />
            )
          )}
        </>
      )}
    </div>
  );
}
