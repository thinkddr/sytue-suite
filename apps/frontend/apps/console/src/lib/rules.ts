/*
 * Ayudas de UI para el constructor de reglas de grupos dinámicos (cliente). Puro:
 * catálogo de atributos/operadores (espeja el de Sytue ID) y conversión entre la forma
 * de EDICIÓN del formulario (BuilderRule: cada condición con un único `value` string) y
 * el contrato de Sytue ID (MembershipRule: `in` usa `values[]`).
 */
import type { ConsoleMessageKey } from '../i18n/index.js';
import type { MembershipRule, RuleAttribute, RuleOp } from './types.js';

/** Atributos seleccionables (clave de catálogo → valor del contrato). */
export const RULE_ATTRIBUTES: { value: RuleAttribute; label: ConsoleMessageKey }[] = [
  { value: 'department', label: 'rule.attr.department' },
  { value: 'title', label: 'rule.attr.title' },
  { value: 'location', label: 'rule.attr.location' },
  { value: 'email', label: 'rule.attr.email' },
  { value: 'name', label: 'rule.attr.name' },
];

/** Operadores seleccionables. */
export const RULE_OPS: { value: RuleOp; label: ConsoleMessageKey }[] = [
  { value: 'eq', label: 'rule.op.eq' },
  { value: 'ne', label: 'rule.op.ne' },
  { value: 'contains', label: 'rule.op.contains' },
  { value: 'starts_with', label: 'rule.op.startsWith' },
  { value: 'in', label: 'rule.op.in' },
];

/** Condición en forma de edición: un único `value` string (para `in`, valores por comas). */
export interface BuilderCondition {
  attribute: RuleAttribute;
  op: RuleOp;
  value: string;
}

export interface BuilderRule {
  match: 'all' | 'any';
  conditions: BuilderCondition[];
}

export function emptyBuilderRule(): BuilderRule {
  return { match: 'all', conditions: [{ attribute: 'department', op: 'eq', value: '' }] };
}

/** Contrato Sytue ID → forma de edición (los `values[]` de `in` se muestran por comas). */
export function ruleToBuilder(rule: MembershipRule): BuilderRule {
  return {
    match: rule.match === 'any' ? 'any' : 'all',
    conditions: (rule.conditions ?? []).map((c) => ({
      attribute: c.attribute,
      op: c.op,
      value: c.op === 'in' ? (c.values ?? []).join(', ') : (c.value ?? ''),
    })),
  };
}

/** Forma de edición → contrato Sytue ID (`in` parte el `value` por comas en `values[]`). */
export function builderToRule(b: BuilderRule): MembershipRule {
  return {
    match: b.match,
    conditions: b.conditions.map((c) =>
      c.op === 'in'
        ? {
            attribute: c.attribute,
            op: c.op,
            values: c.value
              .split(',')
              .map((s) => s.trim())
              .filter(Boolean),
          }
        : { attribute: c.attribute, op: c.op, value: c.value.trim() },
    ),
  };
}
