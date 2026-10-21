/**
 * ErpPermisosPanel — quién ve y quién toca cada módulo de Sytue ERP.
 *
 * El ERP es de toda la organización: una persona entra y ve lo que su nivel en
 * cada módulo le abre, como los permisos de aplicación de Odoo. Cuatro niveles
 * (sin acceso, lectura, usuario, administrador) y un defecto por módulo: los
 * sensibles (contabilidad, nóminas, obligaciones…) empiezan cerrados para los
 * miembros y el resto, abiertos. Owner y admin administran todo y no se tocan.
 *
 * Lo que coincide con el defecto no se guarda: cambiar un defecto alcanza a quien
 * nadie tocó a mano.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  Icon,
  Input,
  SectionHeader,
  Select,
  ToastProvider,
  cx,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import type { ModuloErp, NivelErp, PersonaErp } from '../lib/erp.js';
import { contiene } from '@sytue/i18n';

interface Props {
  locale: Locale;
  niveles: NivelErp[];
  modulos: ModuloErp[];
  personas: PersonaErp[];
  /** Motivo por el que no hay datos: 403 sin rol, 502 si el ERP no contesta. */
  problema?: number | undefined;
}

const esAdmin = (p: PersonaErp) => p.roles.includes('owner') || p.roles.includes('admin');

const ROTULO_NIVEL = {
  ninguno: 'erp.level.ninguno',
  lectura: 'erp.level.lectura',
  usuario: 'erp.level.usuario',
  administrador: 'erp.level.administrador',
} as const;

function Inner({ niveles, modulos, personas, problema, t }: Props & { t: T }) {
  const toast = useToast();
  const [lista, setLista] = useState(personas);
  const [filtro, setFiltro] = useState('');
  const [elegida, setElegida] = useState(personas.find((p) => !esAdmin(p))?.sub ?? '');
  const [borrador, setBorrador] = useState<Record<string, NivelErp>>({});
  const [guardando, setGuardando] = useState(false);

  const persona = lista.find((p) => p.sub === elegida);
  const nivelesDe = (p: PersonaErp): Record<string, NivelErp> =>
    Object.fromEntries(modulos.map((m) => [m.clave, p.modulos?.[m.clave] ?? m.por_defecto]));
  const actual = persona ? { ...nivelesDe(persona), ...borrador } : {};
  const cambiado = Object.keys(borrador).length > 0;

  const visibles = useMemo(() => {
    const q = filtro.trim().toLowerCase();
    return q ? lista.filter((p) => contiene(p.nombre, q) || contiene(p.email, q)) : lista;
  }, [lista, filtro]);

  if (problema !== undefined) {
    return (
      <Card>
        <CardBody>
          <EmptyState
            title={t(problema === 403 ? 'erp.error.forbidden' : 'erp.error.unavailable')}
            description={t(
              problema === 403 ? 'erp.error.forbiddenBody' : 'erp.error.unavailableBody',
            )}
          />
        </CardBody>
      </Card>
    );
  }

  async function guardar() {
    if (!persona) return;
    setGuardando(true);
    const res = await fetch('/api/admin/erp-permisos', {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        sub: persona.sub,
        email: persona.email,
        nombre: persona.nombre,
        modulos: actual,
      }),
    });
    setGuardando(false);
    if (!res.ok) {
      toast.notify({ title: t('erp.toast.failed'), tone: 'danger' });
      return;
    }
    const distintos = Object.fromEntries(
      modulos
        .filter((m) => actual[m.clave] !== m.por_defecto)
        .map((m) => [m.clave, actual[m.clave]!]),
    );
    setLista((prev) => prev.map((p) => (p.sub === persona.sub ? { ...p, modulos: distintos } : p)));
    setBorrador({});
    toast.notify({ title: t('erp.toast.saved'), tone: 'success' });
  }

  return (
    <div class="grid gap-6 lg:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]">
      <Card>
        <CardBody>
          <SectionHeader
            icon={<Icon name="users" size={20} />}
            title={t('erp.people.title')}
            subtitle={t('erp.people.subtitle')}
          />
          <Input
            value={filtro}
            placeholder={t('erp.filter')}
            aria-label={t('erp.filter')}
            class="mt-4 mb-3"
            onInput={(e) => setFiltro((e.target as HTMLInputElement).value)}
          />
          <ul class="flex flex-col gap-1">
            {visibles.map((p) => (
              <li key={p.sub}>
                <button
                  type="button"
                  aria-current={p.sub === elegida ? 'true' : undefined}
                  onClick={() => {
                    setElegida(p.sub);
                    setBorrador({});
                  }}
                  class={cx(
                    'sytue-focus-ring flex w-full flex-col rounded-md px-3 py-2 text-left',
                    p.sub === elegida ? 'bg-selected' : 'hover:bg-bg-subtle',
                  )}
                >
                  <span class="flex items-center justify-between gap-2">
                    <span class="text-text truncate text-sm font-medium">
                      {p.nombre || p.email}
                    </span>
                    {esAdmin(p) ? <Badge tone="info">{t('erp.admin')}</Badge> : null}
                  </span>
                  <span class="text-text-muted truncate text-xs">{p.email}</span>
                </button>
              </li>
            ))}
          </ul>
        </CardBody>
      </Card>

      <Card>
        <CardBody>
          {!persona ? (
            <EmptyState title={t('erp.pick')} description={t('erp.levels.help')} />
          ) : (
            <>
              <SectionHeader
                icon={<Icon name="receipt" size={20} />}
                title={persona.nombre || persona.email}
                subtitle={t('erp.modules.subtitle')}
              />
              <p class="text-text-muted mt-3 text-sm">{t('erp.levels.help')}</p>
              {esAdmin(persona) ? <p class="text-text mt-3 text-sm">{t('erp.adminBody')}</p> : null}
              <table class="mt-4 w-full text-sm">
                <caption class="sr-only">{t('erp.modules.title')}</caption>
                <thead>
                  <tr class="text-text-muted border-border border-b text-left text-xs">
                    <th scope="col" class="py-2 font-medium">
                      {t('erp.col.module')}
                    </th>
                    <th scope="col" class="py-2 font-medium">
                      {t('erp.col.level')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {modulos.map((m) => (
                    <tr key={m.clave} class="border-border border-b last:border-0">
                      <th scope="row" class="text-text py-2 pr-4 text-left font-normal">
                        {m.rotulo}
                      </th>
                      <td class="py-1.5">
                        <Select
                          aria-label={m.rotulo}
                          disabled={esAdmin(persona)}
                          value={esAdmin(persona) ? 'administrador' : actual[m.clave]}
                          onChange={(e) =>
                            setBorrador((b) => ({
                              ...b,
                              [m.clave]: e.currentTarget.value as NivelErp,
                            }))
                          }
                        >
                          {niveles.map((n) => (
                            <option key={n} value={n}>
                              {t(ROTULO_NIVEL[n])}
                              {n === m.por_defecto ? ` · ${t('erp.default')}` : ''}
                            </option>
                          ))}
                        </Select>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <p class="text-text-subtle mt-3 text-xs">{t('erp.obligationsNote')}</p>
              {esAdmin(persona) ? null : (
                <div class="mt-4 flex justify-end gap-2">
                  <Button
                    variant="ghost"
                    disabled={!cambiado || guardando}
                    onClick={() => setBorrador({})}
                  >
                    {t('common.cancel')}
                  </Button>
                  <Button
                    variant="primary"
                    disabled={!cambiado}
                    loading={guardando}
                    onClick={() => void guardar()}
                  >
                    {t('common.save')}
                  </Button>
                </div>
              )}
            </>
          )}
        </CardBody>
      </Card>
    </div>
  );
}

export default function ErpPermisosPanel(props: Props) {
  const t = useT(props.locale);
  return (
    <ToastProvider>
      <Inner {...props} t={t} />
    </ToastProvider>
  );
}
