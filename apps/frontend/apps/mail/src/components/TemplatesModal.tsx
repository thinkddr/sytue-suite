/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * TemplatesModal — las plantillas de respuesta de la cuenta (las «Plantillas» de
 * Gmail, las «Respuestas rápidas» de Outlook). Lista las que hay, con el principio
 * de su texto para reconocerlas, y un único formulario debajo que sirve para crear
 * una nueva o para editar la elegida: dos formularios casi iguales invitan a que se
 * separen.
 *
 * Se guardan en el SERVIDOR (como la firma), así que nada se da por guardado hasta
 * que el servicio lo confirma: el formulario no se vacía si el alta falla, y el aviso
 * dice por qué cuando se sabe (nombre repetido, tope de cien).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  EmptyState,
  FieldGroup,
  FieldHint,
  InlineHelp,
  Input,
  Modal,
  Icon,
  SettingsSection,
  Spinner,
  Textarea,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import {
  motivoDePlantilla,
  TEMPLATE_NAME_MAX,
  TEMPLATES_MAX,
  type ReplyTemplate,
  type TemplateFailure,
} from '../client/mailApi.js';
import { usePrivateMailApi } from '../client/privateMailApi.js';
import { useT, type MailMessageKey, type T } from '../i18n/index.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  onClose: () => void;
  /** Aviso al padre de que las plantillas cambiaron (el redactor las ofrece). */
  onChanged: () => void;
}

/**
 * Tope del texto en el campo. El servicio acepta 16 KiB; 4000 caracteres no llegan
 * ni en el peor caso de UTF-8 (4 bytes cada uno), así que el `maxlength` nativo
 * deja fuera de alcance el rechazo por tamaño, como en el editor de la firma.
 */
const MAX_TEXTO = 4000;

/** El aviso de cada motivo por el que no se guarda. */
const AVISO_AL_GUARDAR: Record<TemplateFailure, MailMessageKey> = {
  exists: 'templates.nameDuplicate',
  limit: 'templates.errorLimit',
  invalid: 'templates.errorInvalid',
  gone: 'templates.errorGone',
  other: 'templates.errorSave',
};

export function TemplatesModal({ locale, open, onClose, onChanged }: Props): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  // Las plantillas son texto de la cuenta: con el correo privado viven cifradas en
  // el repositorio del dispositivo, como la firma; si no, en el servicio.
  const { listTemplates, createTemplate, updateTemplate, deleteTemplate } = usePrivateMailApi();
  const [plantillas, setPlantillas] = useState<ReplyTemplate[]>([]);
  const [cargando, setCargando] = useState(false);
  const [error, setError] = useState(false);
  const [reintento, setReintento] = useState(0);
  // La que se edita en el formulario; null = el formulario crea una nueva.
  const [editando, setEditando] = useState<ReplyTemplate | null>(null);
  const [nombre, setNombre] = useState('');
  const [texto, setTexto] = useState('');
  const [guardando, setGuardando] = useState(false);

  useEffect(() => {
    if (!open) return;
    let activo = true;
    setCargando(true);
    setError(false);
    listTemplates()
      .then(({ templates }) => activo && setPlantillas(templates))
      .catch(() => activo && setError(true))
      .finally(() => activo && setCargando(false));
    return () => {
      activo = false;
    };
  }, [open, reintento, listTemplates]);

  // Al cerrar se olvida lo que se estaba editando: reabrir el diálogo con el
  // formulario a medias de otra vez confunde más que ayuda.
  useEffect(() => {
    if (open) return;
    setEditando(null);
    setNombre('');
    setTexto('');
  }, [open]);

  // Validación en vivo: repetir un nombre es el fallo típico, y el servidor lo
  // rechazaría sin que se viera por qué hasta pulsar.
  const nombreLimpio = nombre.trim();
  const repetido =
    nombreLimpio !== '' &&
    plantillas.some(
      (p) => p.id !== editando?.id && p.name.toLowerCase() === nombreLimpio.toLowerCase(),
    );
  const enElTope = !editando && plantillas.length >= TEMPLATES_MAX;
  const sePuedeGuardar = nombreLimpio !== '' && !repetido && !enElTope && !guardando;

  function empezarEdicion(p: ReplyTemplate): void {
    setEditando(p);
    setNombre(p.name);
    setTexto(p.body);
    // El formulario está debajo de la lista: sin llevar el foco, con muchas
    // plantillas «Editar» no parecería hacer nada.
    requestAnimationFrame(() => document.getElementById('tpl-name')?.focus());
  }

  function cancelarEdicion(): void {
    setEditando(null);
    setNombre('');
    setTexto('');
  }

  async function guardar(): Promise<void> {
    if (!sePuedeGuardar) return;
    setGuardando(true);
    try {
      if (editando) {
        const { template } = await updateTemplate(editando.id, nombreLimpio, texto);
        setPlantillas((lista) => ordenar(lista.map((p) => (p.id === template.id ? template : p))));
        toast.notify({ tone: 'success', title: t('templates.saved') });
      } else {
        const { template } = await createTemplate(nombreLimpio, texto);
        setPlantillas((lista) => ordenar([...lista, template]));
        toast.notify({ tone: 'success', title: t('templates.created') });
      }
      cancelarEdicion();
      onChanged();
    } catch (err) {
      const motivo = motivoDePlantilla(err);
      toast.notify({ tone: 'danger', title: t(AVISO_AL_GUARDAR[motivo]) });
      // La que se editaba ya no existe (se borró desde otro equipo): se vuelve a
      // leer la lista para no seguir enseñándola.
      if (motivo === 'gone') {
        cancelarEdicion();
        setReintento((n) => n + 1);
      }
    } finally {
      setGuardando(false);
    }
  }

  // Borrado optimista: la fila se va al momento y vuelve si el servidor no la borra.
  function borrar(p: ReplyTemplate): void {
    const antes = plantillas;
    setPlantillas((lista) => lista.filter((x) => x.id !== p.id));
    if (editando?.id === p.id) cancelarEdicion();
    void deleteTemplate(p.id)
      .then(() => {
        toast.notify({ tone: 'success', title: t('templates.deleted') });
        onChanged();
      })
      .catch((err: unknown) => {
        // Si ya no estaba, el resultado es el que se quería: no se resucita.
        if (motivoDePlantilla(err) === 'gone') {
          onChanged();
          return;
        }
        setPlantillas(antes);
        toast.notify({ tone: 'danger', title: t('templates.errorDelete') });
      });
  }

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('settings.templatesTitle')}
      size="lg"
      footer={
        <Button variant="secondary" onClick={onClose}>
          {t('common.close')}
        </Button>
      }
    >
      <div class="flex flex-col gap-6">
        <InlineHelp tone="info" title={t('templates.whatTitle')}>
          {t('templates.whatBody')}
        </InlineHelp>

        <SettingsSection plain title={t('templates.yoursTitle')}>
          {cargando ? (
            <div class="text-text-subtle flex items-center gap-2 py-4">
              <Spinner size={16} label={t('templates.loading')} />
              <span class="text-sm" aria-hidden="true">
                {t('common.loading')}
              </span>
            </div>
          ) : error ? (
            <div class="flex flex-col items-start gap-2 py-2" role="alert">
              <p class="text-text flex items-start gap-2 text-sm">
                <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
                {t('templates.loadError')}
              </p>
              <Button size="sm" variant="secondary" onClick={() => setReintento((n) => n + 1)}>
                {t('common.retry')}
              </Button>
            </div>
          ) : plantillas.length === 0 ? (
            <EmptyState
              title={t('templates.emptyTitle')}
              description={t('templates.emptyDescription')}
              icon="copies"
            />
          ) : (
            <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
              {plantillas.map((p) => (
                <FilaDePlantilla
                  key={p.id}
                  t={t}
                  plantilla={p}
                  editandola={editando?.id === p.id}
                  onEdit={empezarEdicion}
                  onDelete={borrar}
                />
              ))}
            </ul>
          )}
        </SettingsSection>

        <SettingsSection
          plain
          title={t(editando ? 'templates.editTitle' : 'templates.newTitle')}
          class="border-border border-t pt-5"
        >
          <FieldGroup label={t('common.name')} htmlFor="tpl-name">
            <Input
              id="tpl-name"
              type="text"
              value={nombre}
              maxLength={TEMPLATE_NAME_MAX}
              placeholder={t('templates.namePlaceholder')}
              invalid={repetido}
              aria-describedby="tpl-name-hint"
              onInput={(e) => setNombre(e.currentTarget.value)}
            />
            <FieldHint id="tpl-name-hint" tone={repetido ? 'danger' : 'neutral'}>
              {repetido ? t('templates.nameDuplicate') : t('templates.nameHint')}
            </FieldHint>
          </FieldGroup>
          <FieldGroup label={t('templates.body')} htmlFor="tpl-body">
            <Textarea
              id="tpl-body"
              rows={6}
              value={texto}
              maxLength={MAX_TEXTO}
              placeholder={t('templates.bodyPlaceholder')}
              aria-describedby="tpl-body-hint"
              onInput={(e) => setTexto(e.currentTarget.value)}
            />
            <FieldHint id="tpl-body-hint">{t('templates.bodyHint')}</FieldHint>
          </FieldGroup>
          {enElTope ? <FieldHint tone="warning">{t('templates.errorLimit')}</FieldHint> : null}
          <div class="flex flex-wrap justify-end gap-2">
            {editando ? (
              <Button variant="ghost" disabled={guardando} onClick={cancelarEdicion}>
                {t('common.cancel')}
              </Button>
            ) : null}
            <Button
              variant="primary"
              {...(editando ? {} : { leadingIcon: <Icon name="plus" /> })}
              loading={guardando}
              disabled={!sePuedeGuardar}
              onClick={() => void guardar()}
            >
              {t(editando ? 'common.save' : 'templates.add')}
            </Button>
          </div>
        </SettingsSection>
      </div>
    </Modal>
  );
}

/** Por nombre y sin distinguir mayúsculas, como las devuelve el servicio. */
function ordenar(lista: ReplyTemplate[]): ReplyTemplate[] {
  return [...lista].sort((a, b) =>
    a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }),
  );
}

/** Una plantilla de la lista: nombre, principio del texto, editar y borrar. */
function FilaDePlantilla({
  t,
  plantilla,
  editandola,
  onEdit,
  onDelete,
}: {
  t: T;
  plantilla: ReplyTemplate;
  editandola: boolean;
  onEdit: (p: ReplyTemplate) => void;
  onDelete: (p: ReplyTemplate) => void;
}): JSX.Element {
  // Borrar no se deshace: se confirma en la propia fila, sin otro diálogo encima.
  const [confirmando, setConfirmando] = useState(false);
  const idConfirmar = `tpl-del-${plantilla.id}`;
  useEffect(() => {
    if (confirmando) document.getElementById(idConfirmar)?.focus();
  }, [confirmando, idConfirmar]);

  // Una línea basta para reconocerla; el texto entero está a un «Editar».
  const extracto = plantilla.body.replace(/\s+/g, ' ').trim();

  return (
    <li
      data-plantilla={plantilla.id}
      class={`flex items-center gap-3 px-3 py-2.5 max-md:flex-wrap ${editandola ? 'bg-selected' : ''}`}
    >
      <div class="min-w-0 flex-1">
        <p class="text-text truncate text-sm font-semibold">{plantilla.name}</p>
        {extracto ? <p class="text-text-muted truncate text-sm">{extracto}</p> : null}
      </div>
      {confirmando ? (
        <div class="flex shrink-0 items-center gap-1.5 max-md:basis-full max-md:justify-end">
          <span class="text-text-muted text-xs">{t('templates.confirmDelete')}</span>
          <Button id={idConfirmar} variant="danger" size="sm" onClick={() => onDelete(plantilla)}>
            {t('common.yes')}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setConfirmando(false)}>
            {t('common.no')}
          </Button>
        </div>
      ) : (
        <div class="flex shrink-0 items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            iconOnly
            aria-label={t('templates.edit', { name: plantilla.name })}
            title={t('templates.edit', { name: plantilla.name })}
            onClick={() => onEdit(plantilla)}
          >
            <Icon name="pen" size={16} />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            iconOnly
            class="text-text-subtle hover:text-danger-text"
            aria-label={t('templates.delete', { name: plantilla.name })}
            title={t('templates.delete', { name: plantilla.name })}
            onClick={() => setConfirmando(true)}
          >
            <Icon name="trash" size={16} />
          </Button>
        </div>
      )}
    </li>
  );
}
