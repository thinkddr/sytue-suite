/**
 * AccesoConexion — quién puede consultar una conexión (NEG-06, decidido por el dueño el
 * 2026-10-16: «lista por conexión»).
 *
 * Una conexión habla con el tercero con la credencial de la ORGANIZACIÓN: con
 * Databricks, eso es cualquier SQL contra el almacén de la empresa. La consultan los
 * administradores y quien la creó, siempre; aquí se nombra a las demás personas y
 * grupos que también puedan. La regla la aplica el servicio en cada consulta: esto
 * solo edita la lista.
 *
 * Los textos van en los 26 idiomas aunque la consola siga en cinco: los pidió así el
 * dueño (ver `i18n/acceso/index.ts`).
 */
import { useEffect, useState } from 'preact/hooks';
import { Badge, Button, Icon, Input, Modal, Select, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useTAcceso } from '../i18n/acceso/index.js';
import { useUserSearch } from '../lib/useUserSearch.js';
import type { AdminGroup, DirectoryUser } from '../lib/types.js';

/** Una entrada de la lista, tal como la guarda el servicio. */
interface Lector {
  kind: 'user' | 'group';
  id: string;
  name?: string;
}

interface Props {
  locale: Locale;
  conexion: { id: string; name: string };
  onClose: () => void;
}

const SIN_PRECARGA: DirectoryUser[] = [];

export default function AccesoConexion({ locale, conexion, onClose }: Props) {
  const t = useTAcceso(locale);
  const toast = useToast();
  const [lista, setLista] = useState<Lector[] | null>(null);
  const [fallo, setFallo] = useState(false);
  const [grupos, setGrupos] = useState<AdminGroup[]>([]);
  const [grupo, setGrupo] = useState('');
  const [guardando, setGuardando] = useState(false);
  const base = `/api/admin/apps/instances/${encodeURIComponent(conexion.id)}/readers`;

  useEffect(() => {
    fetch(base, { headers: { accept: 'application/json' } })
      .then((r) => {
        if (!r.ok) throw new Error('readers');
        return r.json() as Promise<{ readers?: Lector[] }>;
      })
      .then((r) => setLista(r.readers ?? []))
      .catch(() => setFallo(true));
    // Los grupos de verdad o ninguno: el BFF cae a datos de demostración sin servicio,
    // y ofrecer un grupo que no existe sería abrir la conexión a nadie.
    fetch('/api/admin/groups', { headers: { accept: 'application/json' } })
      .then((r) => (r.ok ? (r.json() as Promise<{ live?: boolean; groups?: AdminGroup[] }>) : null))
      .then((r) => setGrupos(r?.live ? (r.groups ?? []) : []))
      .catch(() => setGrupos([]));
  }, [base]);

  const personas = new Set((lista ?? []).filter((l) => l.kind === 'user').map((l) => l.id));
  const { query, setQuery, candidates, buscando } = useUserSearch(SIN_PRECARGA, personas);
  const gruposLibres = grupos.filter(
    (g) => !(lista ?? []).some((l) => l.kind === 'group' && l.id === g.name),
  );

  function anadir(l: Lector) {
    setLista((prev) => [...(prev ?? []), l]);
  }

  async function guardar() {
    if (!lista) return;
    setGuardando(true);
    const res = await fetch(base, {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ readers: lista }),
    }).catch(() => null);
    setGuardando(false);
    if (!res?.ok) {
      toast.notify({ title: t('acceso.error'), tone: 'danger' });
      return;
    }
    toast.notify({ title: t('acceso.guardada'), tone: 'success' });
    onClose();
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={t('acceso.titulo', { nombre: conexion.name })}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('acceso.cancelar')}
          </Button>
          <Button variant="primary" disabled={!lista || guardando} onClick={() => void guardar()}>
            {t('acceso.guardar')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-5">
        <p class="text-text-muted text-sm leading-relaxed">{t('acceso.ayuda')}</p>

        {fallo ? (
          <p class="text-danger text-sm">{t('acceso.errorCarga')}</p>
        ) : !lista ? (
          <p class="sytue-meta">{t('acceso.cargando')}</p>
        ) : lista.length === 0 ? (
          <p class="text-text-muted text-sm">{t('acceso.vacia')}</p>
        ) : (
          <ul class="flex flex-col gap-1">
            {lista.map((l) => (
              <li
                key={`${l.kind}:${l.id}`}
                class="border-border flex items-center gap-2 rounded-md border px-3 py-1.5"
              >
                <Icon
                  name={l.kind === 'group' ? 'users' : 'user'}
                  size={14}
                  class="text-text-subtle shrink-0"
                  aria-hidden="true"
                />
                <span class="text-text min-w-0 flex-1 truncate text-sm">{l.name || l.id}</span>
                <Badge tone="neutral">
                  {l.kind === 'group' ? t('acceso.grupo') : t('acceso.persona')}
                </Badge>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('acceso.quitar', { nombre: l.name || l.id })}
                  onClick={() =>
                    setLista((prev) =>
                      (prev ?? []).filter((x) => !(x.kind === l.kind && x.id === l.id)),
                    )
                  }
                >
                  <Icon name="close" size={14} aria-hidden="true" />
                </Button>
              </li>
            ))}
          </ul>
        )}

        {lista ? (
          <>
            <div class="flex flex-col gap-2">
              <label class="text-text text-sm font-medium" for="acceso-persona">
                {t('acceso.anadirPersona')}
              </label>
              <Input
                id="acceso-persona"
                value={query}
                placeholder={t('acceso.buscar')}
                onInput={(e) => setQuery(e.currentTarget.value)}
              />
              {query.trim() ? (
                candidates.length === 0 ? (
                  <p class="sytue-meta">
                    {buscando ? t('acceso.cargando') : t('acceso.sinCandidatos')}
                  </p>
                ) : (
                  <ul class="flex flex-col gap-1">
                    {candidates.map((u) => (
                      <li key={u.id}>
                        <button
                          type="button"
                          class="hover:bg-bg-subtle sytue-focus-ring flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left"
                          onClick={() => {
                            anadir({ kind: 'user', id: u.id, name: u.fullName });
                            setQuery('');
                          }}
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
                )
              ) : null}
            </div>

            <div class="flex flex-col gap-2">
              <label class="text-text text-sm font-medium" for="acceso-grupo">
                {t('acceso.anadirGrupo')}
              </label>
              {grupos.length === 0 ? (
                <p class="sytue-meta">{t('acceso.sinGrupos')}</p>
              ) : (
                <div class="flex gap-2">
                  <Select
                    id="acceso-grupo"
                    class="min-w-0 flex-1"
                    value={grupo}
                    onChange={(e) => setGrupo(e.currentTarget.value)}
                  >
                    <option value="">{t('acceso.elegirGrupo')}</option>
                    {gruposLibres.map((g) => (
                      <option key={g.id} value={g.name}>
                        {g.name}
                      </option>
                    ))}
                  </Select>
                  <Button
                    variant="secondary"
                    disabled={!grupo}
                    onClick={() => {
                      // Por su NOMBRE: es lo que trae el token de quien consulta.
                      anadir({ kind: 'group', id: grupo, name: grupo });
                      setGrupo('');
                    }}
                  >
                    {t('acceso.anadir')}
                  </Button>
                </div>
              )}
            </div>
          </>
        ) : null}
      </div>
    </Modal>
  );
}
