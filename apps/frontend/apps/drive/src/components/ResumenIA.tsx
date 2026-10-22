/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «¿De qué va este fichero?» sin abrirlo.
 *
 * ES DE SOLO LECTURA A PROPÓSITO. El resumen se mira y se copia; no se guarda
 * en ningún sitio ni toca el fichero. Un resumen escrito encima de un documento
 * es un documento estropeado, y aquí no hay nada que revertir porque no hay
 * nada que escribir.
 *
 * El texto llega en Markdown —el modelo escribe así aunque se le pida que no— y
 * se pinta con `bloquesDe`, el mismo analizador que usan el editor y las notas.
 * Nunca se inyecta HTML: esto es texto de un tercero.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Icon, Modal, Spinner } from '@sytue/ui';
import { bloquesDe, textoPlano } from '@sytue/ai';
import type { useT } from '../i18n/index.js';

interface Props {
  t: ReturnType<typeof useT>;
  /** Ruta absoluta del fichero en Drive. */
  path: string;
  nombre: string;
  onClose: () => void;
}

export function ResumenIA({ t, path, nombre, onClose }: Props): JSX.Element {
  const [texto, setTexto] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(true);
  const [copiado, setCopiado] = useState(false);
  const [accion, setAccion] = useState<'resumir' | 'puntos'>('resumir');

  useEffect(() => {
    let vivo = true;
    setBusy(true);
    setError(null);
    setCopiado(false);
    void (async () => {
      try {
        const res = await fetch('/drive/api/ai', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ path, accion }),
        });
        const cuerpo: unknown = res.ok ? await res.json() : null;
        const salida =
          cuerpo !== null && typeof cuerpo === 'object' && 'text' in cuerpo
            ? String(cuerpo.text).trim()
            : '';
        if (!vivo) return;
        if (salida === '') setError(t('ai.failed'));
        else setTexto(salida);
      } catch {
        if (vivo) setError(t('ai.failed'));
      } finally {
        if (vivo) setBusy(false);
      }
    })();
    // Cambiar de acción vuelve a pedir; `vivo` descarta la respuesta de la
    // petición anterior, que si no puede llegar la segunda y pisarla la primera.
    return () => {
      vivo = false;
    };
  }, [path, accion, t]);

  const bloques = texto === '' ? [] : bloquesDe(texto);

  return (
    <Modal
      open
      onClose={onClose}
      title={t('ai.summaryTitle')}
      size="md"
      // En el pie del diálogo y no al final del cuerpo: con su filete, a todo el ancho
      // en el teléfono y a la vista aunque el resumen sea largo.
      footer={
        <>
          <Button type="button" variant="ghost" onClick={onClose}>
            {t('preview.close')}
          </Button>
          {!busy && error === null ? (
            <Button
              type="button"
              variant="secondary"
              leadingIcon={<Icon name="check" size={16} />}
              onClick={() => {
                // El texto PLANO, sin los asteriscos del Markdown: lo que se pega
                // en un correo tiene que leerse, no llevar marcadores dentro.
                void navigator.clipboard.writeText(textoPlano(bloques));
                setCopiado(true);
              }}
            >
              {copiado ? t('ai.copied') : t('ai.copy')}
            </Button>
          ) : null}
        </>
      }
    >
      <p class="text-text-muted mb-3 truncate text-sm">{nombre}</p>

      {/* Párrafo o puntos: es la misma pregunta con dos formas de respuesta, y
          cambiarla no debe obligar a cerrar y volver a abrir. */}
      <div role="group" aria-label={t('ai.summaryTitle')} class="mb-3 flex gap-2">
        <Button
          type="button"
          size="sm"
          variant={accion === 'resumir' ? 'primary' : 'secondary'}
          aria-pressed={accion === 'resumir'}
          disabled={busy}
          onClick={() => setAccion('resumir')}
        >
          {t('ai.asParagraph')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant={accion === 'puntos' ? 'primary' : 'secondary'}
          aria-pressed={accion === 'puntos'}
          disabled={busy}
          onClick={() => setAccion('puntos')}
        >
          {t('ai.asBullets')}
        </Button>
      </div>

      {busy ? (
        <div class="flex items-center justify-center gap-2 py-8">
          <Spinner size={20} />
          <span class="text-text-muted text-sm">{t('ai.working')}</span>
        </div>
      ) : error !== null ? (
        <p role="alert" class="text-danger-text text-sm">
          {error}
        </p>
      ) : (
        <div class="text-text flex flex-col gap-2 text-sm">
          {bloques.map((b, i) => {
            const contenido = b.trozos.map((tr, j) =>
              tr.negrita ? (
                <strong key={j}>{tr.texto}</strong>
              ) : tr.cursiva ? (
                <em key={j}>{tr.texto}</em>
              ) : (
                <span key={j}>{tr.texto}</span>
              ),
            );
            if (b.tipo === 'li') {
              return (
                <p key={i} class="flex gap-2">
                  <span aria-hidden="true">•</span>
                  <span class="min-w-0">{contenido}</span>
                </p>
              );
            }
            return b.tipo === 'p' ? (
              <p key={i}>{contenido}</p>
            ) : (
              <p key={i} class="font-semibold">
                {contenido}
              </p>
            );
          })}
        </div>
      )}
    </Modal>
  );
}
