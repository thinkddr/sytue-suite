/**
 * UserMailboxes — direcciones de correo de un miembro: la principal y sus alias.
 *
 * La dirección se asigna sola al dar de alta (nombre.apellidos@dominio-de-la-org),
 * así que esto es para las excepciones, que son justo las que necesitan pantalla:
 * el comercial que quiere ser `ventas@`, quien cambia de apellido y necesita seguir
 * recibiendo en el viejo, y los dos homónimos a los que el desempate automático
 * dejó con direcciones que nadie acierta a la primera.
 *
 * Va en su propio componente y no dentro de UsersPanel porque se carga bajo demanda
 * —solo al abrir la ficha de alguien— y porque mantiene su propio estado de red.
 */
import { useEffect, useState } from 'preact/hooks';
import { Badge, Button, Icon, Input, useToast } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import type { OrgMailbox } from '../lib/admin.js';

interface Props {
  userId: string;
  /** El traductor baja como prop, como en el resto del panel: useT necesita el
   * idioma y quien lo conoce es la página, no este trozo de la ficha. */
  t: T;
}

export default function UserMailboxes({ userId, t }: Props) {
  const toast = useToast();
  const [boxes, setBoxes] = useState<OrgMailbox[]>([]);
  const [mailDomain, setMailDomain] = useState('');
  const [cargando, setCargando] = useState(true);
  const [nueva, setNueva] = useState('');
  const [guardando, setGuardando] = useState(false);

  async function recargar(uid: string) {
    const res = await fetch(`/api/admin/mailboxes?user_id=${encodeURIComponent(uid)}`, {
      headers: { accept: 'application/json' },
    });
    if (!res.ok) throw new Error(String(res.status));
    const data = (await res.json()) as { mailboxes?: OrgMailbox[]; mail_domain?: string };
    setBoxes(data.mailboxes ?? []);
    setMailDomain(data.mail_domain ?? '');
  }

  useEffect(() => {
    let vivo = true;
    setCargando(true);
    void (async () => {
      try {
        await recargar(userId);
      } catch {
        if (vivo) setBoxes([]);
      } finally {
        if (vivo) setCargando(false);
      }
    })();
    return () => {
      vivo = false;
    };
  }, [userId]);

  // Un solo manejador para las tres acciones: las tres piden lo mismo (una
  // dirección), se equivocan igual (409 si ya está cogida) y terminan igual
  // (recargar la lista). Separarlas habría triplicado el mismo bloque de errores.
  async function actuar(accion: () => Promise<Response>, exito: string) {
    if (guardando) return;
    setGuardando(true);
    try {
      const res = await accion();
      if (!res.ok) throw new Error(String(res.status));
      await recargar(userId);
      setNueva('');
      toast.notify({ tone: 'success', title: exito });
    } catch (err) {
      const code = err instanceof Error ? err.message : '';
      toast.notify({
        tone: 'danger',
        title: t('users.mailbox.failed'),
        description: code === '409' ? t('users.mailbox.taken') : t('users.mailbox.invalid'),
      });
    } finally {
      setGuardando(false);
    }
  }

  const cuerpo = (address: string) =>
    ({
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ address }),
    }) satisfies RequestInit;

  const principal = boxes.find((b) => b.primary);
  const alias = boxes.filter((b) => !b.primary);

  return (
    <div>
      <p class="text-text mb-1 text-sm font-semibold">{t('users.mailbox.title')}</p>
      {cargando ? (
        <p class="text-text-muted text-sm">{t('common.loading')}</p>
      ) : (
        <div class="flex flex-col gap-1.5">
          {principal ? (
            <div class="flex min-w-0 items-center gap-2">
              <span class="text-text min-w-0 flex-1 truncate font-mono text-sm">
                {principal.address}
              </span>
              <Badge tone="success">{t('users.mailbox.primary')}</Badge>
            </div>
          ) : (
            <p class="text-text-muted text-sm">{t('users.mailbox.none')}</p>
          )}

          {alias.map((a) => (
            <div key={a.id} class="flex min-w-0 items-center gap-2">
              <span class="text-text-muted min-w-0 flex-1 truncate font-mono text-sm">
                {a.address}
              </span>
              <Button
                variant="ghost"
                size="sm"
                disabled={guardando}
                onClick={() =>
                  void actuar(
                    () =>
                      fetch(`/api/admin/users/${encodeURIComponent(userId)}/mailbox`, {
                        ...cuerpo(a.address),
                      }),
                    t('users.mailbox.changed'),
                  )
                }
              >
                {t('users.mailbox.makePrimary')}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                disabled={guardando}
                aria-label={t('users.mailbox.remove')}
                onClick={() =>
                  void actuar(
                    () =>
                      fetch(`/api/admin/mailboxes/${encodeURIComponent(a.id)}`, {
                        method: 'DELETE',
                      }),
                    t('users.mailbox.aliasRemoved'),
                  )
                }
              >
                <Icon name="close" />
              </Button>
            </div>
          ))}

          {principal ? (
            <>
              <div class="mt-1 flex items-center gap-1.5">
                <Input
                  aria-label={t('users.mailbox.addAlias')}
                  placeholder={t('users.mailbox.addPlaceholder')}
                  value={nueva}
                  onInput={(e) => setNueva(e.currentTarget.value)}
                />
                {mailDomain ? (
                  <span class="text-text-muted shrink-0 font-mono text-sm">@{mailDomain}</span>
                ) : null}
                <Button
                  variant="secondary"
                  size="sm"
                  loading={guardando}
                  disabled={!nueva.trim()}
                  onClick={() =>
                    void actuar(
                      () =>
                        fetch(`/api/admin/users/${encodeURIComponent(userId)}/mailbox`, {
                          ...cuerpo(nueva.trim()),
                          method: 'PUT',
                        }),
                      t('users.mailbox.aliasAdded'),
                    )
                  }
                >
                  {t('users.mailbox.addAlias')}
                </Button>
              </div>
              <p class="text-text-muted text-xs">{t('users.mailbox.hint')}</p>
            </>
          ) : null}
        </div>
      )}
    </div>
  );
}
