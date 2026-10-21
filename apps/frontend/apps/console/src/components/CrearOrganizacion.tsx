/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Alta manual de una organización desde la consola de plataforma: la cara de
 * `POST /api/v1/platform/orgs`, que existía sin formulario. Opcionalmente concede
 * plazas de prueba en el mismo gesto (`seat-grants`, tipo `trial`): sin plazas, una
 * organización recién creada no emite tokens en producción (`orgEntitled`).
 *
 * Solo se ofrece con una elevación VIVA. Quien decide es Sytue ID —allowlist,
 * segundo factor y elevación en cada petición—; esto es para no enseñar un botón
 * que solo sabría devolver 403.
 */
import { useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Modal } from '@sytue/ui';
import type { ConsoleMessageKey as MessageKey, T } from '../i18n/index.js';
import type { PlatformActivation } from '../lib/platform.js';

/**
 * POST al BFF de plataforma. A diferencia de `call` en PlatformPanel, un 403 NO
 * recarga: aquí se enseña como error y lo tecleado se conserva, en vez de perderlo
 * al volver a la pantalla de elevarse.
 */
function pedir(path: string, body: unknown): Promise<Response> {
  return fetch(`/api/platform${path}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
}

/** Qué decir cuando Sytue ID rechaza el alta (sus mensajes vienen en inglés). */
function errorDeAlta(status: number): MessageKey {
  if (status === 403) return 'platform.create.error.forbidden';
  if (status === 409) return 'platform.create.error.conflict';
  if (status === 400) return 'platform.create.error.invalid';
  return 'platform.toast.failed';
}

/** AAAA-MM-DD dentro de `dias` días: lo que da y espera `<input type="date">`. */
const dentroDe = (dias: number): string =>
  new Date(Date.now() + dias * 86_400_000).toISOString().slice(0, 10);

function CrearOrganizacionModal({ t, onClose }: { t: T; onClose: (creada: boolean) => void }) {
  const [slug, setSlug] = useState('');
  const [nombre, setNombre] = useState('');
  const [propietaria, setPropietaria] = useState('');
  const [plazas, setPlazas] = useState(1);
  const [prueba, setPrueba] = useState(0);
  const [caduca, setCaduca] = useState(() => dentroDe(30));
  const [motivo, setMotivo] = useState('');
  const [enviando, setEnviando] = useState(false);
  const [error, setError] = useState('');
  const [creada, setCreada] = useState<{ slug: string; prueba: number } | null>(null);
  const invalido =
    slug.trim() === '' ||
    plazas < 1 ||
    prueba < 0 ||
    prueba > 10000 ||
    (prueba > 0 && (caduca === '' || motivo.trim() === ''));

  async function crear() {
    if (invalido || enviando) return;
    setEnviando(true);
    setError('');
    const res = await pedir('/orgs', {
      slug: slug.trim(),
      name: nombre.trim(),
      owner_email: propietaria.trim(),
      seats: plazas,
    }).catch(() => null);
    if (!res?.ok) {
      setError(t(errorDeAlta(res?.status ?? 503)));
      setEnviando(false);
      return;
    }
    const org = (await res.json()) as { id: string; slug: string };
    let concedidas = 0;
    if (prueba > 0) {
      const g = await pedir(`/orgs/${org.id}/seat-grants`, {
        seats: prueba,
        kind: 'trial',
        reason: motivo.trim(),
        expires_at: `${caduca}T00:00:00Z`,
      }).catch(() => null);
      // La organización ya existe: repetir el alta daría 409. Se dice qué faltó.
      if (g?.ok) concedidas = prueba;
      else setError(t('platform.create.error.trial'));
    }
    setCreada({ slug: org.slug, prueba: concedidas });
    setEnviando(false);
  }

  const cerrar = () => onClose(creada !== null);
  return (
    <Modal open title={t('platform.create.title')} onClose={cerrar}>
      <div class="sy-form">
        {creada ? (
          <p role="status">
            {t('platform.create.done', { slug: creada.slug })}
            {creada.prueba > 0 && (
              <>
                <br />
                {t('platform.create.doneTrial', { count: creada.prueba })}
              </>
            )}
          </p>
        ) : (
          <>
            <FieldGroup
              label={t('platform.create.slug')}
              htmlFor="alta-slug"
              help={t('platform.create.slugHelp')}
              required
            >
              <Input
                id="alta-slug"
                value={slug}
                onInput={(e) => setSlug((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
            <FieldGroup label={t('platform.create.name')} htmlFor="alta-nombre">
              <Input
                id="alta-nombre"
                value={nombre}
                onInput={(e) => setNombre((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
            <FieldGroup
              label={t('platform.create.owner')}
              htmlFor="alta-propietaria"
              help={t('platform.create.ownerHelp')}
            >
              <Input
                id="alta-propietaria"
                type="email"
                autocomplete="off"
                value={propietaria}
                onInput={(e) => setPropietaria((e.target as HTMLInputElement).value)}
              />
            </FieldGroup>
            <FieldGroup label={t('platform.seats.count')} htmlFor="alta-plazas" required>
              <Input
                id="alta-plazas"
                type="number"
                min="1"
                value={String(plazas)}
                onInput={(e) => setPlazas(Number((e.target as HTMLInputElement).value))}
              />
            </FieldGroup>
            <FieldGroup label={t('platform.create.trial')} htmlFor="alta-prueba">
              <Input
                id="alta-prueba"
                type="number"
                min="0"
                max="10000"
                value={String(prueba)}
                onInput={(e) => setPrueba(Number((e.target as HTMLInputElement).value))}
              />
            </FieldGroup>
            {prueba > 0 && (
              <>
                <FieldGroup label={t('platform.create.trialEnds')} htmlFor="alta-caduca" required>
                  <Input
                    id="alta-caduca"
                    type="date"
                    value={caduca}
                    onInput={(e) => setCaduca((e.target as HTMLInputElement).value)}
                  />
                </FieldGroup>
                <FieldGroup label={t('platform.seats.reason')} htmlFor="alta-motivo" required>
                  <Input
                    id="alta-motivo"
                    value={motivo}
                    onInput={(e) => setMotivo((e.target as HTMLInputElement).value)}
                  />
                </FieldGroup>
              </>
            )}
          </>
        )}
        {error && (
          <p class="sy-warning" role="alert">
            {error}
          </p>
        )}
        <div class="sy-form__actions">
          <Button variant="secondary" onClick={cerrar}>
            {creada ? t('common.close') : t('common.cancel')}
          </Button>
          {!creada && (
            <Button disabled={invalido || enviando} onClick={() => void crear()}>
              {t('platform.create.submit')}
            </Button>
          )}
        </div>
      </div>
    </Modal>
  );
}

export function CrearOrganizacion({
  t,
  activation,
}: {
  t: T;
  activation: PlatformActivation | null;
}) {
  const [abierto, setAbierto] = useState(false);
  if (activation?.effective_status !== 'active') return null;
  return (
    <>
      <Button onClick={() => setAbierto(true)}>{t('platform.create.open')}</Button>
      {abierto && (
        <CrearOrganizacionModal
          t={t}
          onClose={(creada) => {
            // Recargar trae la organización nueva a la lista con sus cifras reales.
            if (creada) window.location.reload();
            else setAbierto(false);
          }}
        />
      )}
    </>
  );
}
