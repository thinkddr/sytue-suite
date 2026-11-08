/**
 * La píldora del mes (`EventChip`) lleva la hora de inicio delante del título, en la
 * convención del idioma y solo desde `sm:`. EventChip no tiene estado ni efectos, así
 * que se llama como función y se recorre el árbol que devuelve: sin DOM.
 */
import { describe, expect, test } from 'bun:test';
import type { VNode } from 'preact';
import { EventChip } from './CalendarGrid.js';

type Nodo = VNode<{ class?: string; children?: unknown }>;

/** Los `<span>` del árbol, con su clase y su texto. */
function spans(n: unknown, out: { clase: string; texto: string }[] = []): typeof out {
  if (!n || typeof n !== 'object') return out;
  if (Array.isArray(n)) {
    for (const h of n) spans(h, out);
    return out;
  }
  const v = n as Nodo;
  const hijos = v.props?.children;
  if (v.type === 'span') {
    out.push({
      clase: String(v.props.class ?? ''),
      texto: typeof hijos === 'string' ? hijos : '',
    });
  }
  spans(hijos, out);
  return out;
}

const evento = { href: '/e1.ics', uid: 'u1', summary: 'Comité', start: '20260910T080500Z' };

describe('EventChip', () => {
  const chip = (locale: 'es' | 'en', expanded?: boolean) =>
    spans(
      EventChip({
        locale,
        // GridEvent trae más campos; la píldora solo lee estos.
        event: evento as never,
        date: new Date(2026, 8, 10, 8, 5),
        color: '#1e50a8',
        ...(expanded ? { expanded } : {}),
        onClick: () => {},
      }),
    );

  test('la hora de inicio va delante del título, solo desde sm:', () => {
    const s = chip('es');
    const hora = s.findIndex((x) => x.texto === '08:05');
    expect(hora).toBeGreaterThan(-1);
    expect(s[hora]!.clase).toContain('hidden sm:inline');
    expect(s.findIndex((x) => x.texto === 'Comité')).toBeGreaterThan(hora);
  });

  test('en la convención del idioma', () => {
    expect(chip('en').some((x) => /^0?8:05/.test(x.texto))).toBe(true);
  });

  test('la variante expandida la enseña siempre', () => {
    const hora = chip('es', true).find((x) => x.texto === '08:05');
    expect(hora?.clase).not.toContain('hidden');
  });
});
