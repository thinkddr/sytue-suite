/**
 * Piezas compartidas de la membresía dinámica de un grupo: el constructor de la
 * regla, la petición de previsualización al BFF y la lista de quién entraría. Las
 * usan el detalle del grupo (editar o convertir la regla) y el asistente de alta,
 * que no deben dibujar la misma regla de dos formas distintas.
 */
import { Button, Icon, Input, Select } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { AdminGroupMember, RuleAttribute, RuleOp } from '../lib/types.js';
import { builderToRule, RULE_ATTRIBUTES, RULE_OPS, type BuilderRule } from '../lib/rules.js';

/** Resultado de la previsualización de una regla (contrato del BFF /groups/preview). */
export interface PreviewResult {
  count: number;
  members: AdminGroupMember[];
}

/**
 * Constructor de reglas de membresía dinámica: selector AND/OR + lista de condiciones
 * (atributo · operador · valor). El operador `in` acepta varios valores separados por
 * comas. Accesible: cada control tiene etiqueta asociada. La validación fuerte la aplica
 * Sytue ID al guardar/previsualizar.
 */
export function RuleBuilder({
  rule,
  onChange,
  idPrefix,
  t,
}: {
  rule: BuilderRule;
  onChange: (r: BuilderRule) => void;
  idPrefix: string;
  t: T;
}) {
  function setCond(i: number, patch: Partial<BuilderRule['conditions'][number]>) {
    onChange({
      ...rule,
      conditions: rule.conditions.map((c, idx) => (idx === i ? { ...c, ...patch } : c)),
    });
  }
  return (
    <div class="flex flex-col gap-3">
      <div class="flex items-center gap-2">
        <label class="text-text text-sm font-medium" for={`${idPrefix}-match`}>
          {t('groups.rule.include')}
        </label>
        <Select
          id={`${idPrefix}-match`}
          value={rule.match}
          class="w-40"
          onChange={(e) =>
            onChange({ ...rule, match: e.currentTarget.value === 'any' ? 'any' : 'all' })
          }
        >
          <option value="all">{t('groups.rule.matchAll')}</option>
          <option value="any">{t('groups.rule.matchAny')}</option>
        </Select>
      </div>

      <ul class="flex flex-col gap-2">
        {rule.conditions.map((c, i) => (
          <li
            key={i}
            class="border-border bg-surface flex flex-wrap items-center gap-2 rounded-md border p-2"
          >
            <Select
              value={c.attribute}
              aria-label={t('groups.rule.attribute')}
              class="w-40"
              onChange={(e) => setCond(i, { attribute: e.currentTarget.value as RuleAttribute })}
            >
              {RULE_ATTRIBUTES.map((a) => (
                <option key={a.value} value={a.value}>
                  {t(a.label)}
                </option>
              ))}
            </Select>
            <Select
              value={c.op}
              aria-label={t('groups.rule.operator')}
              class="w-40"
              onChange={(e) => setCond(i, { op: e.currentTarget.value as RuleOp })}
            >
              {RULE_OPS.map((o) => (
                <option key={o.value} value={o.value}>
                  {t(o.label)}
                </option>
              ))}
            </Select>
            <Input
              class="min-w-40 flex-1"
              value={c.value}
              aria-label={t('groups.rule.value')}
              placeholder={t(c.op === 'in' ? 'groups.rule.valueListHint' : 'groups.rule.value')}
              onInput={(e) => setCond(i, { value: e.currentTarget.value })}
            />
            <Button
              variant="ghost"
              size="sm"
              aria-label={t('groups.rule.removeCondition')}
              disabled={rule.conditions.length <= 1}
              onClick={() =>
                onChange({ ...rule, conditions: rule.conditions.filter((_, idx) => idx !== i) })
              }
            >
              <Icon name="close" size={14} />
            </Button>
          </li>
        ))}
      </ul>

      <div>
        <Button
          variant="secondary"
          size="sm"
          leadingIcon={<Icon name="plus" size={14} />}
          onClick={() =>
            onChange({
              ...rule,
              conditions: [...rule.conditions, { attribute: 'department', op: 'eq', value: '' }],
            })
          }
        >
          {t('groups.rule.addCondition')}
        </Button>
      </div>
    </div>
  );
}

/** Botón de previsualización + recuento y muestra de a quién incluiría la regla. */
export function PreviewBlock({
  preview,
  loading,
  onPreview,
  t,
}: {
  preview: PreviewResult | null;
  loading: boolean;
  onPreview: () => void;
  t: T;
}) {
  return (
    <div class="flex flex-col gap-2">
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="secondary" size="sm" loading={loading} onClick={onPreview}>
          {t('groups.rule.preview')}
        </Button>
        {preview ? (
          <span class="text-text-muted text-sm">
            {preview.count === 0
              ? t('groups.rule.previewNone')
              : t('groups.rule.previewCount', { count: preview.count })}
          </span>
        ) : null}
      </div>
      {preview && preview.members.length > 0 ? <PeopleList people={preview.members} /> : null}
    </div>
  );
}

/**
 * Previsualiza una regla contra el tenant (BFF), SIN guardar nada. Lanza con el
 * status HTTP como mensaje si Sytue ID la rechaza (400 = regla inválida).
 */
export async function fetchPreview(rule: BuilderRule): Promise<PreviewResult> {
  const res = await fetch('/api/admin/groups/preview', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ membership_rule: builderToRule(rule) }),
  });
  if (!res.ok) throw new Error(String(res.status));
  const data = (await res.json()) as Partial<PreviewResult>;
  return { count: data.count ?? 0, members: data.members ?? [] };
}

/** Nombre y correo de cada persona, en una lista con altura acotada. */
export function PeopleList({
  people,
  label,
  tall = false,
}: {
  people: AdminGroupMember[];
  label?: string;
  tall?: boolean;
}) {
  return (
    <ul
      aria-label={label}
      class={`border-border overflow-auto rounded-md border ${tall ? 'max-h-64' : 'max-h-32'}`}
    >
      {people.map((m) => (
        <li key={m.userId} class="border-border flex flex-col border-b px-2 py-1 last:border-0">
          <span class="text-text truncate text-sm font-medium">{m.name}</span>
          <span class="text-text-muted truncate text-xs">{m.email}</span>
        </li>
      ))}
    </ul>
  );
}
