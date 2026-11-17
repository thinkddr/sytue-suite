/*
 * Ayudas de UI para el constructor de reglas de grupos dinámicos (cliente). Puro:
 * catálogo de atributos/operadores (espeja el de Sytue ID) y conversión entre la forma
 * de EDICIÓN del formulario (BuilderRule: cada condición con un único `value` string) y
 * el contrato de Sytue ID (MembershipRule: `in` usa `values[]`).
 */
import { formatList, type Locale } from '@sytue/i18n';
import type { ConsoleMessageKey, T } from '../i18n/index.js';
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

/** Cada atributo dentro de una frase («su departamento»), no como etiqueta de columna. */
const ATRIBUTO_EN_FRASE = {
  department: 'groups.sentence.attr.department',
  title: 'groups.sentence.attr.title',
  location: 'groups.sentence.attr.location',
  email: 'groups.sentence.attr.email',
  name: 'groups.sentence.attr.name',
} as const satisfies Record<RuleAttribute, ConsoleMessageKey>;

const OPERADOR_EN_FRASE = {
  eq: 'groups.sentence.op.eq',
  ne: 'groups.sentence.op.ne',
  contains: 'groups.sentence.op.contains',
  starts_with: 'groups.sentence.op.startsWith',
  in: 'groups.sentence.op.in',
} as const satisfies Record<RuleOp, ConsoleMessageKey>;

/**
 * La regla entera en una frase llana («Entran las personas si su departamento es
 * «Ventas» o su cargo contiene «jefe».»), al nivel de la de las políticas de acceso.
 * Las dos desplegables AND/OR y los operadores técnicos («is in (list)») obligaban
 * a leer la regla como una tabla; el repaso del asistente la enseña así. Las
 * condiciones a medio escribir (sin valor) no se cuentan: no filtran a nadie y
 * Sytue ID las rechazaría.
 */
export function describeRule(rule: BuilderRule, t: T, locale: Locale): string {
  const entre = (v: string) => t('groups.sentence.quoted', { value: v });
  const clausulas: string[] = [];
  for (const c of rule.conditions) {
    const valores =
      c.op === 'in'
        ? c.value
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean)
        : [c.value.trim()].filter(Boolean);
    if (valores.length === 0) continue;
    clausulas.push(
      t(OPERADOR_EN_FRASE[c.op], {
        attr: t(ATRIBUTO_EN_FRASE[c.attribute]),
        value: formatList(locale, valores.map(entre), 'disjunction'),
      }),
    );
  }
  if (clausulas.length === 0) return t('groups.sentence.empty');
  return t('groups.sentence.rule', {
    clauses: formatList(locale, clausulas, rule.match === 'any' ? 'disjunction' : 'conjunction'),
  });
}
