/**
 * DashboardPanel — panel de administración de Sytue Admin: métricas de
 * identidad, organizaciones recientes y últimos eventos de auditoría. Solo
 * @sytue/ui. docs/CONVENTIONS.md §8.
 */
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Badge,
  EmptyState,
  Icon,
  SectionHeader,
} from '@sytue/ui';
import type { IconName } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { useT, type ConsoleMessageKey } from '../i18n/index.js';
import type { AuditEvent, ConsoleMetric, Tenant } from '../lib/types.js';
import type { OrgInsights } from '../lib/admin.js';
import { r } from '../lib/rutas.js';

// Tonos con par claro/oscuro completo (tokens *-bg/*-text; «primary» del Badge
// no se reasigna en oscuro, «info» sí y en claro rinde idéntico).
const planTone = { starter: 'neutral', business: 'info', enterprise: 'info' } as const;

/**
 * Un aviso accionable: qué pasa, cuántos, y a dónde se va a arreglarlo.
 *
 * Solo se pinta si el número es > 0. Una lista fija de avisos donde la mitad
 * dicen «0 pendientes» enseña a no mirarla, y entonces el día que uno de ellos
 * sube a 40 tampoco se mira.
 */
function Aviso({
  icon,
  texto,
  href,
  cta,
  tone,
}: {
  icon: IconName;
  texto: string;
  href: string;
  cta: string;
  tone: 'warning' | 'neutral';
}) {
  return (
    <li class="border-border flex items-center gap-3 border-b px-4 py-2.5 last:border-0">
      <Icon
        name={icon}
        size={16}
        class={tone === 'warning' ? 'text-warning-text shrink-0' : 'text-text-subtle shrink-0'}
        aria-hidden="true"
      />
      <span class="text-text min-w-0 flex-1 text-sm">{texto}</span>
      <a href={href} class="text-primary-text shrink-0 text-sm font-semibold hover:underline">
        {cta}
      </a>
    </li>
  );
}

export default function DashboardPanel({
  metrics,
  tenants,
  events,
  insights,
  locale,
}: {
  metrics: ConsoleMetric[];
  tenants: Tenant[];
  events: AuditEvent[];
  insights?: OrgInsights;
  locale: Locale;
}) {
  const t = useT(locale);

  // Se construyen aquí y no en el JSX para poder preguntar cuántos hay antes de
  // decidir si la tarjeta existe siquiera.
  const i = insights;
  const avisos = !i
    ? []
    : [
        i.without_mfa > 0 && (
          <Aviso
            key="mfa"
            icon="shield"
            tone="warning"
            texto={t('dashboard.todo.withoutMfa', { count: i.without_mfa })}
            href={r('/users')}
            cta={t('dashboard.todo.review')}
          />
        ),
        i.invitations_expired > 0 && (
          <Aviso
            key="inv"
            icon="userCheck"
            tone="warning"
            texto={t('dashboard.todo.invitationsExpired', { count: i.invitations_expired })}
            href={r('/users')}
            cta={t('dashboard.todo.reinvite')}
          />
        ),
        i.unverified_email > 0 && (
          <Aviso
            key="mail"
            icon="info"
            tone="neutral"
            texto={t('dashboard.todo.unverified', { count: i.unverified_email })}
            href={r('/users')}
            cta={t('dashboard.todo.review')}
          />
        ),
        i.suspended > 0 && (
          <Aviso
            key="susp"
            icon="user"
            tone="neutral"
            texto={t('dashboard.todo.suspended', { count: i.suspended })}
            href={r('/users')}
            cta={t('dashboard.todo.review')}
          />
        ),
      ].filter(Boolean);

  return (
    <div>
      <SectionHeader
        level={1}
        title={t('page.dashboard')}
        subtitle={t('dashboard.subtitle', { realm: 'sytue' })}
      />
      <div class="px-4 py-4">
        <div class="mb-4 grid grid-cols-2 gap-4 lg:grid-cols-4">
          {metrics.map((m) => (
            <Card key={m.labelKey} raised={false}>
              <CardBody class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p
                    class="text-text-muted truncate text-sm"
                    title={t(m.labelKey as ConsoleMessageKey)}
                  >
                    {t(m.labelKey as ConsoleMessageKey)}
                  </p>
                  <p class="sytue-h3 text-text mt-1 tabular-nums">{m.value}</p>
                  {m.hintKey ? (
                    <p class="sytue-meta mt-0.5">{t(m.hintKey as ConsoleMessageKey)}</p>
                  ) : null}
                </div>
              </CardBody>
            </Card>
          ))}
        </div>

        {/*
          Qué hay que mirar hoy. Va ARRIBA del todo y solo con avisos de verdad:
          las métricas de abajo dicen cuánta gente hay, que no cambia nada de lo
          que uno vaya a hacer al abrir esto.
        */}
        {avisos.length > 0 && (
          <Card class="mb-4">
            <CardHeader>
              <CardTitle>{t('dashboard.todo.title')}</CardTitle>
            </CardHeader>
            <CardBody class="p-0">
              <ul>{avisos}</ul>
            </CardBody>
          </Card>
        )}

        <div class="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>{t('dashboard.tenants.title')}</CardTitle>
              <a
                href={r('/tenants')}
                class="text-primary-text text-sm font-semibold hover:underline"
              >
                {t('dashboard.tenants.viewAll')}
              </a>
            </CardHeader>
            <CardBody class="p-0">
              {tenants.length === 0 ? (
                <EmptyState
                  title={t('dashboard.tenants.emptyTitle')}
                  description={t('dashboard.tenants.emptyDescription')}
                  illustration={<Icon name="building" size={28} />}
                  action={
                    <a href={r('/tenants')}>
                      <Button
                        variant="primary"
                        size="sm"
                        leadingIcon={<Icon name="plus" size={14} />}
                      >
                        {t('dashboard.tenants.newTenant')}
                      </Button>
                    </a>
                  }
                />
              ) : (
                <ul>
                  {tenants.map((org) => (
                    <li
                      key={org.slug}
                      class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-4 py-2.5 transition-colors last:border-0"
                    >
                      {/* Icono suelto, sin tesela: el mismo dibujo se repetía
                          idéntico en todas las filas, así que no distinguía, no
                          agrupaba y no indicaba estado — y encima usaba un token
                          de estado (`info-bg`) para decorar. Mismo criterio que
                          EmptyState en @sytue/ui. */}
                      <Icon name="building" size={16} class="text-text-subtle shrink-0" />
                      <a
                        href={r(`/tenants/${org.slug}`)}
                        class="text-text min-w-0 flex-1 truncate font-medium hover:underline"
                        title={org.name}
                      >
                        {org.name}
                      </a>
                      <Badge tone={planTone[org.plan]} class="shrink-0">
                        {org.plan}
                      </Badge>
                      {/* El recuento de usuarios se retira en <640px: con el nombre
                          de la organización y el plan, la fila pedía más de 412px y
                          era ella la que estiraba la tarjeta. El dato completo está
                          a un toque, en la ficha de la organización. */}
                      <span class="text-text-muted shrink-0 text-sm tabular-nums max-sm:hidden">
                        {t('dashboard.tenants.userCount', { count: org.userCount })}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </CardBody>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('dashboard.activity.title')}</CardTitle>
              <a href={r('/audit')} class="text-primary-text text-sm font-semibold hover:underline">
                {t('dashboard.activity.viewAudit')}
              </a>
            </CardHeader>
            <CardBody class="p-0">
              {events.length === 0 ? (
                <EmptyState
                  title={t('dashboard.activity.emptyTitle')}
                  description={t('dashboard.activity.emptyDescription')}
                  illustration={<Icon name="audit" size={28} />}
                />
              ) : (
                <ul>
                  {events.slice(0, 6).map((e) => (
                    <li
                      key={e.id}
                      class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-4 py-2.5 transition-colors last:border-0"
                    >
                      {/* Estado con icono además del color (WCAG 1.4.1: no solo-color). */}
                      <Badge tone={e.ok ? 'success' : 'danger'}>
                        <Icon
                          name={e.ok ? 'check' : 'danger'}
                          size={12}
                          class="mr-1"
                          aria-hidden="true"
                        />
                        {e.source}
                        <span class="sr-only">
                          {e.ok ? t('dashboard.activity.ok') : t('dashboard.activity.failed')}
                        </span>
                      </Badge>
                      <span
                        class="text-text min-w-0 flex-1 truncate text-base"
                        title={e.target ? `${e.type} · ${e.target}` : e.type}
                      >
                        <span class="font-mono text-sm">{e.type}</span>
                        {e.target ? <span class="text-text-muted"> · {e.target}</span> : null}
                      </span>
                      <span class="text-text-muted shrink-0 text-sm tabular-nums">
                        {formatDateTime(locale, e.at)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </CardBody>
          </Card>
        </div>
      </div>
    </div>
  );
}
