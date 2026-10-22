/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Formulario «Filtrar por condición» del desplegable de una columna del autofiltro:
 * elige el operador y sus operandos (valor, rango, N, color) y devuelve una
 * `FilterCondition`. No filtra nada por sí mismo: la evalúa sheets/filtro.ts y la
 * guarda quien lo monta (el autofiltro compartido o una vista personal).
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Input, Select } from '@sytue/ui';
import { useT } from '../context';
import type { FilterCondition, FilterOp } from '../types';
import { FILTER_OPS, operandCount, sanitizeFilterCond } from './filtro';

interface Props {
  /** La condición que ya tiene la columna, si la tiene. */
  current?: FilterCondition | undefined;
  /** Colores de relleno y de texto que hay en la columna (para `fill` y `fontColor`). */
  fills: readonly string[];
  fonts: readonly string[];
  /** `null` = quitar la condición. */
  onApply: (cond: FilterCondition | null) => void;
  onBack: () => void;
}

export function FiltroCondicion({ current, fills, fonts, onApply, onBack }: Props): JSX.Element {
  const t = useT();
  const [op, setOp] = useState<FilterOp>(current?.op ?? 'gt');
  const [a, setA] = useState(current?.a ?? '');
  const [b, setB] = useState(current?.b ?? '');
  const [n, setN] = useState(String(current?.n ?? 10));
  const [percent, setPercent] = useState(current?.percent === true);
  const [color, setColor] = useState(current?.color ?? '');

  const draft: Record<string, unknown> = { op, a, b, n: Number(n), percent, color };
  const cond = sanitizeFilterCond(draft);
  const k = operandCount(op);
  const colores = op === 'fill' ? fills : fonts;
  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'Enter' && cond) onApply(cond);
  };

  return (
    <div class="flex w-64 flex-col gap-1.5 px-2 py-1.5">
      <div class="sytue-overline text-text-subtle">{t('filtro.byCondition')}</div>
      <Select
        size="sm"
        value={op}
        aria-label={t('filtro.byCondition')}
        onChange={(e) => {
          const next = e.currentTarget.value as FilterOp;
          setOp(next);
          setColor('');
        }}
      >
        {FILTER_OPS.map((o) => (
          <option key={o} value={o}>
            {t(`filtro.op.${o}`)}
          </option>
        ))}
      </Select>
      {k >= 1 && (
        <Input
          size="sm"
          value={a}
          aria-label={t('common.value')}
          placeholder={t('common.value')}
          onInput={(e) => setA(e.currentTarget.value)}
          onKeyDown={onKey}
        />
      )}
      {k === 2 && (
        <div class="flex items-center gap-1.5">
          <span class="text-text-muted text-xs">{t('filtro.and')}</span>
          <Input
            size="sm"
            value={b}
            aria-label={t('common.value')}
            placeholder={t('common.value')}
            onInput={(e) => setB(e.currentTarget.value)}
            onKeyDown={onKey}
          />
        </div>
      )}
      {(op === 'top' || op === 'bottom') && (
        <div class="flex items-center gap-1.5">
          <Input
            size="sm"
            type="number"
            min={1}
            value={n}
            aria-label={t('filtro.count')}
            onInput={(e) => setN(e.currentTarget.value)}
            onKeyDown={onKey}
          />
          <Select
            size="sm"
            value={percent ? 'p' : 'n'}
            aria-label={t('filtro.count')}
            onChange={(e) => setPercent(e.currentTarget.value === 'p')}
          >
            <option value="n">{t('filtro.count')}</option>
            <option value="p">{t('filtro.percent')}</option>
          </Select>
        </div>
      )}
      {(op === 'fill' || op === 'fontColor') && (
        <div class="flex flex-wrap gap-1" role="radiogroup" aria-label={t('filtro.color')}>
          {colores.map((c) => (
            <button
              key={c}
              type="button"
              role="radio"
              aria-checked={color === c}
              aria-label={c}
              title={c}
              class={`h-5 w-5 rounded border ${color === c ? 'border-primary ring-primary ring-2' : 'border-border'}`}
              style={{ backgroundColor: c }}
              onClick={() => setColor(c)}
            />
          ))}
        </div>
      )}
      <div class="flex gap-1.5 pt-0.5">
        <Button
          variant="secondary"
          size="xs"
          class="flex-1"
          disabled={!cond}
          onClick={() => cond && onApply(cond)}
        >
          {t('action.apply')}
        </Button>
        {current && (
          <Button variant="ghost" size="xs" onClick={() => onApply(null)}>
            {t('filtro.clear')}
          </Button>
        )}
        <Button variant="ghost" size="xs" onClick={onBack}>
          {t('filtro.back')}
        </Button>
      </div>
    </div>
  );
}
