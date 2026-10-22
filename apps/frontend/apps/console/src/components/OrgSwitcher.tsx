/**
 * OrgSwitcher — cambiar de organización sin tener que pedirlo por correo.
 *
 * Quien pertenece a dos organizaciones tenía la más ANTIGUA como activa y
 * ninguna forma de cambiarla: si en ella es `member`, esta consola le respondía
 * 403 en las dieciséis páginas aunque fuera `owner` de la otra.
 *
 * Se pinta SOLO con dos o más organizaciones. Con una sola, un selector es una
 * pregunta que no tiene respuesta alternativa: ocupa sitio, invita a pulsarlo y
 * no lleva a ninguna parte. Con una sola se enseña el nombre y ya está.
 *
 * Cambiar exige rehacer el login SSO —el `tenant` va firmado dentro del token
 * que este navegador ya tiene—, pero eso no es volver a escribir la contraseña:
 * la sesión de Sytue ID sigue viva y el viaje es una redirección que se ve como
 * una recarga.
 */
import { useEffect, useState } from 'preact/hooks';
import { Dropdown, Icon, MenuItem, MenuSeparator, Spinner } from '@sytue/ui';
import type { T } from '../i18n/index.js';

import type { SytueRole } from '../lib/types.js';
import { partirRuta } from '@sytue/config';
import { r } from '../lib/rutas.js';

/**
 * La página ACTUAL sin la org del path: lo que va en `returnTo` al rehacer el
 * login para cambiar de organización (ADR 0056). Fuera del prefijo de la app no
 * hay org que pelar —la consola de plataforma— y ahí se devuelve la ruta tal cual.
 */
function rutaSinOrg(): string {
  const base = import.meta.env.BASE_URL.replace(/\/+$/, '');
  const ruta = window.location.pathname;
  if (!base || !ruta.startsWith(`${base}/`)) return ruta;
  const { resto } = partirRuta(ruta.slice(base.length));
  return resto ? `${base}/${resto}` : base;
}

interface MyOrg {
  id: string;
  slug: string;
  name: string;
  role: SytueRole;
  active: boolean;
}

/** El rol, por su clave del catálogo. Un `role.${o.role}` interpolado no lo
 *  puede comprobar el compilador, que es justo lo que evita que un rol nuevo
 *  salga en pantalla como su propia clave. */
const ROLE_KEY = {
  owner: 'role.owner',
  admin: 'role.admin',
  member: 'role.member',
  billing: 'role.billing',
} as const;

export default function OrgSwitcher({ t }: { t: T }) {
  const [orgs, setOrgs] = useState<MyOrg[] | null>(null);
  const [cambiando, setCambiando] = useState('');
  const [error, setError] = useState(false);

  useEffect(() => {
    let vivo = true;
    void (async () => {
      try {
        const res = await fetch('/api/me/organizations');
        if (!res.ok) throw new Error('http');
        const body = (await res.json()) as { organizations?: MyOrg[] };
        if (vivo) setOrgs(Array.isArray(body.organizations) ? body.organizations : []);
      } catch {
        if (vivo) setOrgs([]);
      }
    })();
    return () => {
      vivo = false;
    };
  }, []);

  const activa = orgs?.find((o) => o.active);

  const cambiar = async (org: MyOrg) => {
    if (org.active || cambiando) return;
    setCambiando(org.id);
    setError(false);
    try {
      const res = await fetch('/api/me/organizations', {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ org_id: org.id }),
      });
      if (!res.ok) throw new Error('http');
      // Rehacer el login SSO para que el token nuevo lleve la organización
      // nueva. Sin esto la pantalla recargaría con el `tenant` de antes y el
      // cambio parecería no haber funcionado.
      //
      // El returnTo va SIN la org del path (ADR 0056): el `tenant` que trae la
      // URL es el de la sesión que se acaba de abandonar, y devolverse a
      // `/admin/<org-vieja>/…` serviría la página con la org nueva en el token y
      // la vieja en la barra —el middleware no redirige cuando difieren a
      // propósito—. Sin org, la canónica con la org NUEVA entra sola en el 302.
      window.location.assign(r(`/auth/login?returnTo=${encodeURIComponent(rutaSinOrg())}`));
    } catch {
      setCambiando('');
      setError(true);
    }
  };

  // Mientras carga no se pinta un esqueleto: esto vive en el pie de la barra
  // lateral y un bloque que aparece y desaparece ahí es más ruidoso que el
  // medio segundo que tarda.
  if (orgs === null || orgs.length === 0) return null;

  if (orgs.length === 1) {
    return (
      <p class="text-sidebar-fg-muted truncate px-2 py-1 text-xs" title={orgs[0]?.name}>
        {orgs[0]?.name}
      </p>
    );
  }

  return (
    <div class="px-1 py-0.5">
      <Dropdown
        label={t('org.switch')}
        trigger={({ onClick, ...aria }) => (
          <button
            type="button"
            onClick={onClick}
            class="text-sidebar-fg hover:bg-sidebar-hover sytue-focus-ring-chrome flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors duration-150 focus-visible:outline-none"
            {...aria}
          >
            <Icon name="building" size={16} />
            <span class="min-w-0 flex-1">
              <span class="sytue-overline text-sidebar-fg-muted block">{t('org.active')}</span>
              <span class="block truncate text-sm font-medium">
                {activa?.name ?? t('org.none')}
              </span>
            </span>
            {cambiando ? <Spinner size={14} /> : <Icon name="chevronDown" size={14} />}
          </button>
        )}
      >
        <div class="w-64">
          <p class="sytue-overline text-text-subtle px-3 pt-2 pb-1">{t('org.switch')}</p>
          {orgs.map((o) => (
            <MenuItem
              key={o.id}
              onClick={() => void cambiar(o)}
              icon={<Icon name="building" size={18} />}
            >
              <span class="min-w-0 flex-1">
                <span class="block truncate">{o.name}</span>
                <span class="text-text-subtle block truncate text-xs">
                  {t(ROLE_KEY[o.role] ?? 'role.member')}
                </span>
              </span>
              {o.active ? <Icon name="check" size={16} /> : null}
            </MenuItem>
          ))}
          {error ? (
            <>
              <MenuSeparator />
              <p class="text-danger px-3 py-2 text-xs">{t('org.switchFailed')}</p>
            </>
          ) : null}
        </div>
      </Dropdown>
    </div>
  );
}
