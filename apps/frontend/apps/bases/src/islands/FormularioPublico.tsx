/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El formulario público: lo que ve quien abre el enlace. Sin cromo de la suite, sin
 * sesión. Antes de enviar resuelve la prueba de trabajo ALTCHA en el navegador (sin
 * cookies ni terceros) y manda los valores; el servicio los vuelve a validar y rechaza
 * cualquier campo que la vista del formulario no incluya.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Checkbox, FieldGroup, Input, Select } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { BasesField, BasesValue } from '@sytue/api-clients/bases';
import { useT } from '../i18n/index.js';
import { resolverReto, type RetoAltcha } from '@sytue/config';
import { valorDeTexto } from '../lib/valores.js';

interface Props {
  locale: Locale;
  definicion: { name: string; fields: BasesField[] } | null;
}

export function FormularioPublico({ locale, definicion }: Props): JSX.Element {
  const t = useT(locale);
  const [texto, setTexto] = useState<Record<string, string>>({});
  const [marcas, setMarcas] = useState<Record<string, boolean>>({});
  const [varios, setVarios] = useState<Record<string, string[]>>({});
  const [estado, setEstado] = useState<'edit' | 'enviando' | 'enviado'>('edit');
  const [error, setError] = useState('');

  if (!definicion) {
    return (
      <main class="mx-auto flex max-w-lg flex-col gap-2 p-6">
        <h1 class="text-text text-xl font-semibold">{t('form.unavailable.title')}</h1>
        <p class="text-text-muted text-sm">{t('form.unavailable.body')}</p>
      </main>
    );
  }
  const campos = definicion.fields;

  function valores(): Record<string, BasesValue> | null {
    const v: Record<string, BasesValue> = {};
    for (const c of campos) {
      if (c.kind === 'checkbox') {
        if (marcas[c.id]) v[c.id] = true;
      } else if (c.kind === 'multiselect') {
        if ((varios[c.id] ?? []).length > 0) v[c.id] = varios[c.id]!;
      } else {
        const x = valorDeTexto(c, texto[c.id] ?? '');
        if (x === undefined) {
          setError(t('form.invalid', { name: c.name }));
          return null;
        }
        if (x !== null) v[c.id] = x;
      }
    }
    if (Object.keys(v).length === 0) {
      setError(t('form.empty'));
      return null;
    }
    return v;
  }

  async function enviar(e: Event): Promise<void> {
    e.preventDefault();
    setError('');
    const v = valores();
    if (!v) return;
    setEstado('enviando');
    // La ruta del propio formulario, leída aquí (no al renderizar: en el servidor no hay `window`).
    const base = window.location.pathname.replace(/\/$/, '');
    try {
      const r = await fetch(`${base}/reto`);
      if (!r.ok) throw new Error('reto');
      const carga = await resolverReto((await r.json()) as RetoAltcha);
      if (!carga) throw new Error('reto');
      const res = await fetch(`${base}/enviar`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ values: v, altcha: carga }),
      });
      if (res.status === 404) {
        window.location.reload(); // revocado o agotado mientras se rellenaba
        return;
      }
      if (!res.ok) throw new Error(String(res.status));
      setEstado('enviado');
      setTexto({});
      setMarcas({});
      setVarios({});
    } catch {
      setError(t('form.error'));
      setEstado('edit');
    }
  }

  if (estado === 'enviado') {
    return (
      <main class="mx-auto flex max-w-lg flex-col gap-3 p-6" aria-live="polite">
        <h1 class="text-text text-xl font-semibold">{t('form.sent.title')}</h1>
        <p class="text-text-muted text-sm">{t('form.sent.body')}</p>
        <div>
          <Button variant="secondary" onClick={() => setEstado('edit')}>
            {t('form.another')}
          </Button>
        </div>
      </main>
    );
  }

  return (
    <main class="mx-auto max-w-lg p-4 md:p-6">
      <h1 class="text-text mb-4 text-xl font-semibold">{definicion.name}</h1>
      <form class="flex flex-col gap-4" onSubmit={(e) => void enviar(e)} noValidate>
        {campos.map((c) => {
          const id = `f-${c.id}`;
          if (c.kind === 'checkbox') {
            return (
              <Checkbox
                key={c.id}
                class="min-h-[24px]"
                label={c.name}
                checked={marcas[c.id] === true}
                onChange={(e) => setMarcas({ ...marcas, [c.id]: e.currentTarget.checked })}
              />
            );
          }
          if (c.kind === 'multiselect') {
            return (
              <fieldset key={c.id} class="flex flex-col gap-2">
                <legend class="text-text mb-1 text-sm font-semibold">{c.name}</legend>
                {(c.choices ?? []).map((o) => (
                  <Checkbox
                    key={o}
                    class="min-h-[24px]"
                    label={o}
                    checked={(varios[c.id] ?? []).includes(o)}
                    onChange={(e) => {
                      const on = e.currentTarget.checked;
                      const ant = varios[c.id] ?? [];
                      setVarios({
                        ...varios,
                        [c.id]: on ? [...ant, o] : ant.filter((x) => x !== o),
                      });
                    }}
                  />
                ))}
              </fieldset>
            );
          }
          return (
            <FieldGroup key={c.id} label={c.name} htmlFor={id}>
              {c.kind === 'select' ? (
                <Select
                  id={id}
                  value={texto[c.id] ?? ''}
                  onChange={(e) => setTexto({ ...texto, [c.id]: e.currentTarget.value })}
                >
                  <option value="" />
                  {(c.choices ?? []).map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </Select>
              ) : (
                <Input
                  id={id}
                  type={c.kind === 'date' ? 'date' : 'text'}
                  inputMode={c.kind === 'number' ? 'decimal' : undefined}
                  maxLength={c.kind === 'text' ? 10000 : undefined}
                  value={texto[c.id] ?? ''}
                  onInput={(e) => setTexto({ ...texto, [c.id]: e.currentTarget.value })}
                />
              )}
            </FieldGroup>
          );
        })}
        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}
        <p class="text-text-muted text-xs">{t('form.antispam')}</p>
        <div>
          <Button type="submit" loading={estado === 'enviando'}>
            {estado === 'enviando' ? t('form.sending') : t('form.submit')}
          </Button>
        </div>
      </form>
    </main>
  );
}
