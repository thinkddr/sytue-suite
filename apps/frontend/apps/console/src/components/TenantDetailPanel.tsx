/**
 * TenantDetailPanel — ficha de una organización: datos, recursos mapeados y sus
 * usuarios y grupos. Reúne la vista de un tenant de Sytue ID.
 * docs/CONVENTIONS.md §9.
 */
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Badge,
  Avatar,
  EmptyState,
  Icon,
  Table,
  SectionHeader,
} from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import type { DirectoryUser, Group, Tenant } from '../lib/types.js';
import { roleLabelKey, roleTone } from '../lib/format.js';
import { r } from '../lib/rutas.js';

// «info» y no «primary»: mismo par claro, pero con reasignación en modo oscuro.
const planTone = { starter: 'neutral', business: 'info', enterprise: 'info' } as const;

export default function TenantDetailPanel({
  tenant,
  users,
  groups,
  locale,
}: {
  tenant: Tenant;
  users: DirectoryUser[];
  groups: Group[];
  locale: Locale;
}) {
  const t = useT(locale);
  const userColumns: TableColumn<DirectoryUser>[] = [
    {
      key: 'user',
      header: t('users.col.user'),
      cell: (u) => (
        <span class="flex min-w-0 items-center gap-2">
          <Avatar name={u.fullName} size="xs" />
          <span class="min-w-0">
            <span class="text-text block truncate font-medium" title={u.fullName}>
              {u.fullName}
            </span>
            <span class="text-text-muted block truncate text-sm" title={u.email}>
              {u.email}
            </span>
          </span>
        </span>
      ),
    },
    {
      key: 'roles',
      header: t('users.col.roles'),
      cell: (u) => (
        <span class="flex flex-wrap gap-1">
          {u.roles.map((r) => (
            <Badge key={r} tone={roleTone[r]}>
              {t(roleLabelKey[r])}
            </Badge>
          ))}
        </span>
      ),
    },
    {
      key: 'mfa',
      header: 'MFA',
      cell: (u) =>
        u.mfaEnabled ? (
          <Badge tone="success">{t('common.yes')}</Badge>
        ) : (
          <Badge tone="warning">{t('common.no')}</Badge>
        ),
    },
    {
      key: 'status',
      header: t('users.col.status'),
      cell: (u) => (
        <Badge tone={u.enabled ? 'success' : 'neutral'}>
          {u.enabled ? t('users.statusActive') : t('users.statusDisabled')}
        </Badge>
      ),
    },
  ];

  return (
    <div>
      <SectionHeader
        level={1}
        icon={<Icon name="building" size={18} />}
        title={
          <span class="truncate" title={tenant.name}>
            {tenant.name}
          </span>
        }
        subtitle={
          <span class="font-mono">
            org/{tenant.slug} · {tenant.namespace}
          </span>
        }
        actions={
          <>
            <Badge tone={planTone[tenant.plan]}>{tenant.plan}</Badge>
            <Badge tone={tenant.status === 'active' ? 'success' : 'warning'}>
              {tenant.status === 'active'
                ? t('tenants.statusActive')
                : t('tenants.statusSuspended')}
            </Badge>
          </>
        }
      />
      <div class="px-4 py-4">
        <a
          href={r('/tenants')}
          class="text-primary-text mb-4 inline-flex items-center gap-1 text-sm font-semibold hover:underline"
        >
          <Icon name="chevronLeft" size={14} /> {t('nav.tenants')}
        </a>

        <div class="mb-4 grid grid-cols-2 gap-4 lg:grid-cols-4">
          <Card raised={false}>
            <CardBody>
              <p class="text-text-muted text-sm">{t('nav.users')}</p>
              <p class="sytue-h3 text-text mt-1 tabular-nums">{tenant.userCount}</p>
            </CardBody>
          </Card>
          <Card raised={false}>
            <CardBody>
              <p class="text-text-muted text-sm">{t('nav.groups')}</p>
              <p class="sytue-h3 text-text mt-1 tabular-nums">{groups.length}</p>
            </CardBody>
          </Card>
          <Card raised={false}>
            <CardBody>
              <p class="text-text-muted text-sm">{t('tenants.mailDomain')}</p>
              <p class="text-text mt-1 truncate font-mono text-base" title={tenant.mailDomain}>
                {tenant.mailDomain}
              </p>
            </CardBody>
          </Card>
          <Card raised={false}>
            <CardBody>
              <p class="text-text-muted text-sm">{t('tenants.region')}</p>
              <p class="text-text mt-1 text-base">fr-par · UE</p>
            </CardBody>
          </Card>
        </div>

        <Card class="mb-4">
          <CardHeader>
            <CardTitle>{t('tenants.detail.usersTitle')}</CardTitle>
          </CardHeader>
          <CardBody class="p-0">
            <Table
              columns={userColumns}
              rows={users}
              rowKey={(u) => u.id}
              caption={t('tenants.detail.usersCaption', { name: tenant.name })}
              emptyContent={
                <EmptyState
                  title={t('tenants.detail.usersEmptyTitle')}
                  description={t('tenants.detail.usersEmptyDescription')}
                  illustration={<Icon name="user" size={28} />}
                />
              }
            />
          </CardBody>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('nav.groups')}</CardTitle>
          </CardHeader>
          {groups.length === 0 ? (
            <CardBody class="p-0">
              <EmptyState
                title={t('tenants.detail.groupsEmptyTitle')}
                description={t('tenants.detail.groupsEmptyDescription')}
                illustration={<Icon name="users" size={28} />}
              />
            </CardBody>
          ) : (
            <CardBody class="flex flex-col gap-2">
              {groups.map((g) => (
                <div
                  key={g.path}
                  class="border-border flex items-center justify-between gap-2 border-b pb-2 last:border-0 last:pb-0"
                >
                  <span class="text-text min-w-0 truncate font-mono text-sm" title={g.path}>
                    {g.path}
                  </span>
                  <span class="flex shrink-0 items-center gap-2">
                    {g.mappings.chatWorkspace ? (
                      <Badge tone="neutral">
                        {t('tenants.detail.workspace', { name: g.mappings.chatWorkspace })}
                      </Badge>
                    ) : null}
                    <span class="text-text-muted text-sm tabular-nums">
                      {t('groups.memberCount', { count: g.memberCount })}
                    </span>
                  </span>
                </div>
              ))}
            </CardBody>
          )}
        </Card>
      </div>
    </div>
  );
}
