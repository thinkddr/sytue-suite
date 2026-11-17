/**
 * CreateGroupWizard — alta guiada de un grupo, manual o dinámico.
 *
 * Antes era un modal con nombre, descripción y un interruptor «grupo dinámico» que
 * desplegaba el constructor de la regla y un botón opcional de previsualizar: se
 * podía crear un grupo dinámico sin haber mirado nunca a quién metía, y uno manual
 * nacía vacío aunque ya supieras a quién querías dentro. Ahora son seis pasos:
 * tipo, nombre, miembros (o regla), quién entra, repaso y hecho.
 *
 * Las reglas del recorrido:
 *  - Nada se crea hasta el repaso: cancelar en cualquier paso no deja rastro.
 *  - En un grupo dinámico la previsualización (`/groups/preview`) se pide SIEMPRE
 *    al pasar de la regla a «quién entra», y cambiar la regla la invalida.
 *  - Un error de Sytue ID vuelve al paso que lo corrige (409 = nombre; 400 = la
 *    regla en un dinámico, el nombre en uno manual) y se pinta allí.
 *
 * Un grupo manual se crea y luego se le añaden las personas una a una (el contrato
 * de Sytue ID no admite miembros en el alta); si alguna falla, el paso final lo
 * dice por persona en vez de dar el grupo por completo.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Button,
  ChoiceCard,
  FieldGroup,
  FieldHint,
  Icon,
  InlineHelp,
  Input,
  ReviewSummary,
  Wizard,
} from '@sytue/ui';
import type { ReviewItem, WizardStep } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import type { AdminGroup, AdminGroupMember, DirectoryUser } from '../lib/types.js';
import { builderToRule, describeRule, emptyBuilderRule, type BuilderRule } from '../lib/rules.js';
import { useUserSearch } from '../lib/useUserSearch.js';
import { fetchPreview, PeopleList, RuleBuilder, type PreviewResult } from './GroupRule.js';

export const PASO_TIPO = 0;
export const PASO_NOMBRE = 1;
export const PASO_MIEMBROS = 2;
export const PASO_QUIEN = 3;
export const PASO_REPASO = 4;
export const PASO_HECHO = 5;

type Tipo = 'manual' | 'dynamic';

/** Error de la API, atado al paso donde se corrige. */
interface ErrorPaso {
  paso: number;
  mensaje: string;
}

/**
 * Lleva el status de la API al paso que lo corrige. El 409 siempre es el nombre
 * (ya hay un grupo así); el 400 es la regla en un dinámico y el nombre en uno
 * manual (lo único que valida Sytue ID). Lo demás se queda en el repaso.
 */
export function pasoDelError(status: string, tipo: Tipo): number {
  if (status === '409') return PASO_NOMBRE;
  if (status === '400') return tipo === 'dynamic' ? PASO_MIEMBROS : PASO_NOMBRE;
  return PASO_REPASO;
}

/** Una condición sin valor no filtra a nadie: no se deja avanzar con ella. */
export function reglaCompleta(rule: BuilderRule): boolean {
  return (
    rule.conditions.length > 0 &&
    rule.conditions.every((c) =>
      c.op === 'in' ? c.value.split(',').some((v) => v.trim()) : c.value.trim() !== '',
    )
  );
}

function comoMiembro(u: DirectoryUser): AdminGroupMember {
  return { userId: u.id, email: u.email, name: u.fullName };
}

export interface CreateGroupWizardProps {
  t: T;
  locale: Locale;
  /** Directorio que ya trae la página (primeras filas) para elegir miembros. */
  users: DirectoryUser[];
  /** El grupo ya existe (se llama una vez, con el recuento de miembros real). */
  onCreated: (group: AdminGroup) => void;
  /** «Abrir el grupo» del paso final. */
  onOpenGroup: (group: AdminGroup) => void;
  onClose: () => void;
  /** Pie del diálogo donde va la botonera (alcanzable sobre el teclado del móvil). */
  footerTarget?: Element | null;
  /**
   * Avisa de cuándo hay una petición en vuelo: el diálogo no debe cerrarse (Escape,
   * clic fuera, «×») mientras se crea el grupo o se añaden sus miembros, o el grupo
   * nacería aunque el admin creyera haber cancelado.
   */
  onBusyChange?: (busy: boolean) => void;
}

export default function CreateGroupWizard({
  t,
  locale,
  users,
  onCreated,
  onOpenGroup,
  onClose,
  footerTarget,
  onBusyChange,
}: CreateGroupWizardProps) {
  const [step, setStep] = useState(PASO_TIPO);
  const [tipo, setTipo] = useState<Tipo>('manual');
  const [name, setName] = useState('');
  const [desc, setDesc] = useState('');
  const [elegidos, setElegidos] = useState<DirectoryUser[]>([]);
  const [rule, setRule] = useState<BuilderRule>(emptyBuilderRule);
  const [preview, setPreview] = useState<PreviewResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ErrorPaso | null>(null);
  const [creado, setCreado] = useState<AdminGroup | null>(null);
  const [fallidos, setFallidos] = useState<DirectoryUser[]>([]);

  useEffect(() => onBusyChange?.(busy), [busy, onBusyChange]);

  const dinamico = tipo === 'dynamic';
  const nombre = name.trim();
  const errorEn = (paso: number) => (error?.paso === paso ? error.mensaje : '');

  const idsElegidos = useMemo(() => new Set(elegidos.map((u) => u.id)), [elegidos]);
  const { query, setQuery, candidates, buscando } = useUserSearch(users, idsElegidos);

  /** Quién entraría: la previsualización en un dinámico, la selección en uno manual. */
  const entran: AdminGroupMember[] = dinamico
    ? (preview?.members ?? [])
    : elegidos.map(comoMiembro);
  const cuantos = dinamico ? (preview?.count ?? 0) : elegidos.length;

  function cambiarRegla(r: BuilderRule) {
    setRule(r);
    // La lista que se vio ya no es la de esta regla: se vuelve a pedir al avanzar.
    setPreview(null);
    if (error?.paso === PASO_MIEMBROS) setError(null);
  }

  function reiniciar() {
    setTipo('manual');
    setName('');
    setDesc('');
    setElegidos([]);
    setRule(emptyBuilderRule());
    setPreview(null);
    setError(null);
    setCreado(null);
    setFallidos([]);
    setQuery('');
    setStep(PASO_TIPO);
  }

  async function previsualizar() {
    if (busy || !reglaCompleta(rule)) return;
    setBusy(true);
    setError(null);
    try {
      setPreview(await fetchPreview(rule));
      setStep(PASO_QUIEN);
    } catch (err) {
      const status = err instanceof Error ? err.message : '';
      setError({
        paso: PASO_MIEMBROS,
        mensaje: t(status === '400' ? 'groups.toast.ruleInvalid' : 'common.tryAgain'),
      });
    } finally {
      setBusy(false);
    }
  }

  async function crear() {
    if (busy || !nombre || (dinamico && !preview)) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch('/api/admin/groups', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          name: nombre,
          description: desc.trim(),
          ...(dinamico ? { membership_rule: builderToRule(rule) } : {}),
        }),
      });
      if (!res.ok) {
        const status = String(res.status);
        const paso = pasoDelError(status, tipo);
        setError({
          paso,
          mensaje:
            status === '409'
              ? t('groups.toast.nameTaken')
              : status === '400'
                ? t(dinamico ? 'groups.toast.ruleInvalid' : 'groups.toast.checkName')
                : t('common.tryAgain'),
        });
        setStep(paso);
        return;
      }
      const group = (await res.json()) as AdminGroup;
      // Las personas elegidas se añaden una a una; las que fallen se cuentan aparte
      // para que el paso final no dé el grupo por completo.
      const malos: DirectoryUser[] = [];
      if (!dinamico) {
        for (const u of elegidos) {
          try {
            const r = await fetch(`/api/admin/groups/${encodeURIComponent(group.id)}/members`, {
              method: 'POST',
              headers: { 'content-type': 'application/json' },
              body: JSON.stringify({ user_id: u.id }),
            });
            if (!r.ok) malos.push(u);
          } catch {
            malos.push(u);
          }
        }
      }
      const final: AdminGroup = {
        ...group,
        dynamic: dinamico || Boolean(group.dynamic),
        memberCount: dinamico ? (group.memberCount ?? cuantos) : elegidos.length - malos.length,
      };
      setFallidos(malos);
      setCreado(final);
      onCreated(final);
      setStep(PASO_HECHO);
    } catch {
      setError({ paso: PASO_REPASO, mensaje: t('common.tryAgain') });
    } finally {
      setBusy(false);
    }
  }

  const stepTipo: WizardStep = {
    id: 'type',
    title: t('groups.wizard.step.type'),
    heading: t('groups.wizard.type.heading'),
    description: t('groups.wizard.type.description'),
    content: (
      <div
        role="radiogroup"
        aria-label={t('groups.wizard.type.label')}
        class="flex max-w-2xl flex-col gap-2"
      >
        <ChoiceCard
          name="group-type"
          value="manual"
          icon="userCheck"
          title={t('groups.wizard.type.manual')}
          description={t('groups.wizard.type.manualBody')}
          selected={!dinamico}
          onSelect={() => {
            setTipo('manual');
            setError(null);
          }}
        />
        <ChoiceCard
          name="group-type"
          value="dynamic"
          icon="tag"
          title={t('groups.wizard.type.dynamic')}
          description={t('groups.wizard.type.dynamicBody')}
          selected={dinamico}
          onSelect={() => {
            setTipo('dynamic');
            setError(null);
          }}
        />
      </div>
    ),
  };

  const errNombre = errorEn(PASO_NOMBRE);
  const stepNombre: WizardStep = {
    id: 'name',
    title: t('groups.wizard.step.name'),
    heading: t('groups.wizard.name.heading'),
    description: t('groups.wizard.name.description'),
    canContinue: nombre !== '',
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <FieldGroup label={t('tenants.form.name')} htmlFor="new-group-name" required>
          <Input
            id="new-group-name"
            autocomplete="off"
            maxLength={128}
            value={name}
            placeholder={t('groups.namePlaceholder')}
            invalid={Boolean(errNombre)}
            aria-describedby="new-group-name-hint"
            onInput={(e) => {
              setName(e.currentTarget.value);
              if (error?.paso === PASO_NOMBRE) setError(null);
            }}
          />
          <FieldHint id="new-group-name-hint" tone={errNombre ? 'danger' : 'neutral'}>
            {errNombre || t('groups.wizard.name.hint')}
          </FieldHint>
        </FieldGroup>
        <FieldGroup
          label={`${t('groups.description')} (${t('common.optional')})`}
          htmlFor="new-group-desc"
        >
          <Input
            id="new-group-desc"
            autocomplete="off"
            maxLength={256}
            value={desc}
            aria-describedby="new-group-desc-hint"
            onInput={(e) => setDesc(e.currentTarget.value)}
          />
          <FieldHint id="new-group-desc-hint">{t('groups.wizard.name.descHint')}</FieldHint>
        </FieldGroup>
      </div>
    ),
  };

  const errRegla = errorEn(PASO_MIEMBROS);
  const stepMiembros: WizardStep = dinamico
    ? {
        id: 'rule',
        title: t('groups.wizard.step.rule'),
        heading: t('groups.wizard.rule.heading'),
        description: t('groups.dynamicHelp'),
        canContinue: reglaCompleta(rule),
        nextLabel: t('groups.wizard.rule.previewNext'),
        onNext: () => void previsualizar(),
        content: (
          <div class="flex max-w-2xl flex-col gap-4">
            {/* Bloqueada mientras se previsualiza: una edición a mitad de la petición
                dejaría ver la lista de la regla anterior como si fuera la de esta. */}
            <fieldset disabled={busy} class="m-0 min-w-0 border-0 p-0">
              <RuleBuilder rule={rule} onChange={cambiarRegla} idPrefix="new-rule" t={t} />
            </fieldset>
            {/* La frase se escribe a la vez que la regla: así se lee lo que se pide. */}
            <InlineHelp tone="info" title={t('groups.wizard.review.rule')}>
              <span aria-live="polite">{describeRule(rule, t, locale)}</span>
            </InlineHelp>
            {errRegla ? (
              <InlineHelp tone="danger" title={t('groups.wizard.preview.failed')}>
                {errRegla}
              </InlineHelp>
            ) : !reglaCompleta(rule) ? (
              <p class="sytue-meta">{t('groups.wizard.rule.incomplete')}</p>
            ) : null}
          </div>
        ),
      }
    : {
        id: 'members',
        title: t('groups.wizard.step.members'),
        heading: t('groups.wizard.members.heading'),
        description: t('groups.wizard.members.description'),
        content: (
          <div class="flex max-w-2xl flex-col gap-4">
            <FieldGroup label={t('groups.addMember')} htmlFor="new-group-picker">
              <Input
                id="new-group-picker"
                type="search"
                autocomplete="off"
                value={query}
                placeholder={t('roles.assignPlaceholder')}
                leadingIcon={<Icon name="search" size={16} />}
                aria-describedby="new-group-picker-hint"
                onInput={(e) => setQuery(e.currentTarget.value)}
              />
              <FieldHint id="new-group-picker-hint">{t('roles.assignHint')}</FieldHint>
            </FieldGroup>
            {candidates.length === 0 ? (
              <p class="text-text-subtle text-sm" aria-live="polite">
                {buscando ? t('common.loading') : t('roles.noCandidates')}
              </p>
            ) : (
              <ul class="flex flex-col gap-1" aria-label={t('groups.wizard.members.candidates')}>
                {candidates.map((u) => (
                  <li key={u.id}>
                    <button
                      type="button"
                      class="hover:bg-bg-subtle sytue-focus-ring flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left"
                      aria-label={t('groups.wizard.members.addAria', { name: u.fullName })}
                      onClick={() => setElegidos((prev) => [...prev, u])}
                    >
                      <Icon
                        name="plus"
                        size={14}
                        class="text-text-subtle shrink-0"
                        aria-hidden="true"
                      />
                      <span class="min-w-0 flex-1">
                        <span class="text-text block truncate text-sm font-medium">
                          {u.fullName}
                        </span>
                        <span class="text-text-muted block truncate text-xs">{u.email}</span>
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <div class="border-border-subtle flex flex-col gap-2 border-t pt-3">
              <p class="text-text text-sm font-medium" aria-live="polite">
                {t('groups.wizard.members.chosen', { count: elegidos.length })}
              </p>
              {elegidos.length > 0 ? (
                <ul class="flex flex-col gap-1" aria-label={t('groups.members')}>
                  {elegidos.map((u) => (
                    <li
                      key={u.id}
                      class="border-border flex items-center gap-2 rounded-md border px-2 py-1"
                    >
                      <span class="min-w-0 flex-1">
                        <span class="text-text block truncate text-sm font-medium">
                          {u.fullName}
                        </span>
                        <span class="text-text-muted block truncate text-xs">{u.email}</span>
                      </span>
                      <Button
                        variant="ghost"
                        size="sm"
                        aria-label={t('groups.wizard.members.removeAria', { name: u.fullName })}
                        onClick={() => setElegidos((prev) => prev.filter((it) => it.id !== u.id))}
                      >
                        <Icon name="close" size={14} />
                      </Button>
                    </li>
                  ))}
                </ul>
              ) : (
                <p class="sytue-meta">{t('groups.wizard.members.optional')}</p>
              )}
            </div>
          </div>
        ),
      };

  const resto = cuantos - entran.length;
  const stepQuien: WizardStep = {
    id: 'preview',
    title: t('groups.wizard.step.preview'),
    heading: t('groups.wizard.preview.heading'),
    description: t('groups.wizard.preview.description'),
    // Sin previsualización no hay repaso: es la garantía de «ver quién entra».
    canContinue: !dinamico || preview !== null,
    content: (
      <div class="flex max-w-2xl flex-col gap-3">
        {cuantos === 0 ? (
          <InlineHelp
            tone={dinamico ? 'warning' : 'neutral'}
            title={t(dinamico ? 'groups.noRuleMatches' : 'groups.wizard.preview.nobodyManual')}
          >
            {t(
              dinamico
                ? 'groups.wizard.preview.nobodyRuleBody'
                : 'groups.wizard.preview.nobodyManualBody',
            )}
          </InlineHelp>
        ) : (
          <>
            <p class="text-text text-sm font-medium" aria-live="polite">
              {t('groups.wizard.preview.count', { count: cuantos })}
            </p>
            <PeopleList people={entran} label={t('groups.wizard.step.preview')} tall />
            {resto > 0 ? (
              <p class="sytue-meta">{t('groups.wizard.preview.more', { count: resto })}</p>
            ) : null}
          </>
        )}
        {dinamico ? (
          <InlineHelp tone="info" title={t('groups.wizard.review.rule')}>
            {describeRule(rule, t, locale)}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  const summary: ReviewItem[] = [
    {
      label: t('groups.wizard.review.type'),
      value: t(dinamico ? 'groups.dynamic' : 'groups.wizard.review.manual'),
      onEdit: () => setStep(PASO_TIPO),
    },
    { label: t('tenants.form.name'), value: nombre || '—', onEdit: () => setStep(PASO_NOMBRE) },
    {
      label: t('groups.description'),
      value: desc.trim() || t('groups.noDescription'),
      onEdit: () => setStep(PASO_NOMBRE),
    },
    dinamico
      ? {
          label: t('groups.wizard.review.rule'),
          value: describeRule(rule, t, locale),
          onEdit: () => setStep(PASO_MIEMBROS),
        }
      : {
          label: t('groups.members'),
          value:
            elegidos.length > 0
              ? elegidos.map((u) => u.fullName).join(', ')
              : t('groups.wizard.preview.nobodyManual'),
          onEdit: () => setStep(PASO_MIEMBROS),
        },
    {
      label: t('groups.wizard.step.preview'),
      value: t('groups.memberCount', { count: cuantos }),
      onEdit: () => setStep(dinamico ? PASO_MIEMBROS : PASO_QUIEN),
    },
  ];

  const errRepaso = errorEn(PASO_REPASO);
  const stepRepaso: WizardStep = {
    id: 'review',
    title: t('groups.wizard.step.review'),
    heading: t('groups.wizard.review.heading'),
    description: t('groups.wizard.review.description'),
    nextLabel: t('groups.create'),
    canContinue: nombre !== '' && (!dinamico || preview !== null),
    onNext: () => void crear(),
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        <ReviewSummary items={summary} />
        {errRepaso ? (
          <InlineHelp tone="danger" title={t('groups.toast.createFailed')}>
            {errRepaso}
          </InlineHelp>
        ) : null}
      </div>
    ),
  };

  const stepHecho: WizardStep = {
    id: 'done',
    title: t('groups.wizard.step.done'),
    heading: t('groups.wizard.done.heading'),
    done: true,
    content: (
      <div class="flex max-w-2xl flex-col gap-4">
        {creado ? (
          <InlineHelp
            tone="success"
            title={t(creado.dynamic ? 'groups.toast.dynamicCreated' : 'groups.toast.created')}
          >
            {t('groups.toast.createdHint', { name: creado.name })}{' '}
            {t('groups.memberCount', { count: creado.memberCount })}
          </InlineHelp>
        ) : null}
        {fallidos.length > 0 ? (
          <InlineHelp
            tone="warning"
            title={t('groups.wizard.done.someFailed', { count: fallidos.length })}
          >
            <span class="block">{fallidos.map((u) => u.fullName).join(', ')}</span>
            <span class="block">{t('groups.wizard.done.someFailedBody')}</span>
          </InlineHelp>
        ) : null}
        <div class="border-border-subtle flex flex-wrap items-center gap-2 border-t pt-4">
          {creado ? (
            <Button variant="primary" onClick={() => onOpenGroup(creado)}>
              {t('groups.wizard.done.open')}
            </Button>
          ) : null}
          <Button variant="secondary" onClick={reiniciar}>
            {t('groups.wizard.done.another')}
          </Button>
          <Button variant="ghost" onClick={onClose}>
            {t('common.close')}
          </Button>
        </div>
      </div>
    ),
  };

  return (
    <Wizard
      steps={[stepTipo, stepNombre, stepMiembros, stepQuien, stepRepaso, stepHecho]}
      current={step}
      onCurrentChange={setStep}
      busy={busy}
      onCancel={onClose}
      footerTarget={footerTarget ?? null}
    />
  );
}
