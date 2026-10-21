/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel inferior de los subtítulos de un vídeo o un audio de diapositiva: sus pistas
 * WebVTT (idioma, la de por defecto, quitar) y subir un `.vtt` nuevo. La lógica pura
 * —validar, sanear, la única por defecto— está en subtitulos.ts; aquí solo la mecánica de
 * ficheros y el pintado.
 */
import { useRef, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import { Radio, useToast } from '@sytue/ui';
import { EditorIcon } from '../icons';
import { useT } from '../context';
import type { SlideBox, SlideTrack } from '../types';
import {
  MAX_TRACKS,
  MAX_VTT_CHARS,
  cleanVtt,
  langFromFileName,
  normalizeLang,
  sanitizeTracks,
  trackLabel,
  withDefaultTrack,
  withTrack,
  withoutTrack,
} from './subtitulos';

const FOCUS_RING = 'sytue-focus-ring';
const FIELD =
  'border-border bg-surface text-text h-6 min-h-[24px] rounded-sm border px-1 text-xs ' +
  FOCUS_RING;
const BTN = `text-text-muted hover:bg-bg-subtle inline-flex h-6 min-h-[24px] items-center gap-1 rounded-sm px-2 text-xs disabled:opacity-30 ${FOCUS_RING}`;

/** Idioma de la interfaz como punto de partida del campo «idioma» (`es`, `en-GB`…). */
function idiomaInicial(): string {
  const l = typeof document !== 'undefined' ? document.documentElement.lang : '';
  return normalizeLang(l) ?? 'es';
}

export function SubtitulosPanel(props: {
  box: SlideBox;
  /** Guarda la lista nueva en la caja (vacía = sin subtítulos). */
  onChange: (tracks: SlideTrack[]) => void;
}): JSX.Element {
  const t = useT();
  const { notify } = useToast();
  const [lang, setLang] = useState(idiomaInicial);
  const fileRef = useRef<HTMLInputElement>(null);
  const tracks = sanitizeTracks(props.box.tracks);

  const onFile = (e: Event): void => {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // permite volver a elegir el mismo fichero
    if (!file) return;
    // El idioma del campo manda; si está vacío o mal, el que sugiere el nombre del fichero.
    const idioma = normalizeLang(lang) ?? langFromFileName(file.name);
    if (!idioma) {
      notify({ tone: 'warning', title: t('slides.subs.badLang') });
      return;
    }
    void file.text().then((raw) => {
      const vtt = cleanVtt(raw);
      if (vtt === null) {
        notify({
          tone: 'warning',
          title: raw.length > MAX_VTT_CHARS ? t('slides.subs.tooBig') : t('slides.subs.invalid'),
        });
        return;
      }
      if (tracks.length >= MAX_TRACKS && !tracks.some((x) => x.lang === idioma)) {
        notify({ tone: 'warning', title: t('slides.subs.tooMany', { max: MAX_TRACKS }) });
        return;
      }
      props.onChange(withTrack(tracks, { lang: idioma, vtt }));
    });
  };

  return (
    <div class="border-border bg-surface flex max-h-44 shrink-0 flex-col border-t">
      <div class="flex items-center justify-between px-3 pt-1.5">
        <span class="text-text text-xs font-semibold">{t('slides.subs')}</span>
        <span class="sytue-meta-dense">{t('slides.subs.hint')}</span>
      </div>
      <ul class="min-h-0 flex-1 overflow-auto px-3 pt-1 pb-1">
        {tracks.length === 0 && <li class="sytue-meta">{t('slides.subs.none')}</li>}
        {tracks.map((tr) => (
          <li key={tr.lang} class="flex items-center gap-2 py-0.5">
            <span class="text-text min-w-0 flex-1 truncate text-xs">
              {trackLabel(tr)} <span class="text-text-muted">({tr.lang})</span>
            </span>
            <Radio
              name="slides-subs-default"
              label={t('slides.subs.default')}
              checked={tr.default === true}
              onChange={() => props.onChange(withDefaultTrack(tracks, tr.lang))}
            />
            <button
              type="button"
              class={BTN}
              aria-label={t('slides.subs.remove', { lang: tr.lang })}
              title={t('slides.subs.remove', { lang: tr.lang })}
              onClick={() => props.onChange(withoutTrack(tracks, tr.lang))}
            >
              <EditorIcon name="trash" size="sm" />
            </button>
          </li>
        ))}
      </ul>
      <div class="border-border flex shrink-0 items-center gap-2 border-t px-3 py-1">
        <label class="text-text-muted flex items-center gap-1 text-xs">
          {t('slides.subs.lang')}
          <input
            class={`${FIELD} w-20`}
            value={lang}
            maxLength={16}
            spellcheck={false}
            autocapitalize="off"
            onInput={(e) => setLang(e.currentTarget.value)}
          />
        </label>
        <button type="button" class={BTN} onClick={() => fileRef.current?.click()}>
          <EditorIcon name="plus" size="sm" /> {t('slides.subs.add')}
        </button>
        <input
          ref={fileRef}
          type="file"
          accept=".vtt,text/vtt"
          hidden
          tabIndex={-1}
          onChange={onFile}
        />
      </div>
    </div>
  );
}
