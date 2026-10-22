/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Formulario «Comentar» del editor de dibujo: un comentario del documento anclado a
 * la forma seleccionada (por su id, que no cambia al moverla) o, sin selección, a una
 * coordenada del lienzo. Se guarda con el mismo modelo de comentarios que los de
 * texto (`ctx.addComment`, ancla `{dibujo, nodo | x,y, text}`) y aparece en el panel
 * «Comentarios» con su cita y un botón para ir al dibujo. Solo está disponible en un
 * dibujo YA insertado y para formas que ya están guardadas en él: el id de una forma
 * recién dibujada, sin aplicar, no existe todavía en el documento.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Textarea } from '@sytue/ui';
import { useT } from '../context';

export type DestinoComentario = { nodo: string; nombre: string } | { x: number; y: number };

export function ComentarDibujo({
  destino,
  nombre,
  pendienteDeGuardar,
  ancho,
  alto,
  onComentar,
}: {
  /** La forma seleccionada, o `null` para comentar una coordenada. */
  destino: { nodo: string; nombre: string } | null;
  /** Nombre que se enseña del destino. */
  nombre: string;
  /** La forma aún no está en el documento (se dibujó en esta sesión). */
  pendienteDeGuardar: boolean;
  ancho: number;
  alto: number;
  onComentar: (cuerpo: string, destino: DestinoComentario) => Promise<void>;
}): JSX.Element {
  const t = useT();
  const [texto, setTexto] = useState('');
  const [x, setX] = useState(Math.round(ancho / 2));
  const [y, setY] = useState(Math.round(alto / 2));
  const [estado, setEstado] = useState<'' | 'enviando' | 'hecho' | 'error'>('');
  const bloqueado = destino !== null && pendienteDeGuardar;
  const enviar = async (e: Event): Promise<void> => {
    e.preventDefault();
    const cuerpo = texto.trim();
    if (cuerpo === '' || bloqueado) return;
    setEstado('enviando');
    try {
      await onComentar(cuerpo, destino ?? { x, y });
      setTexto('');
      setEstado('hecho');
    } catch {
      setEstado('error');
    }
  };
  return (
    <form class="flex flex-col gap-1.5" onSubmit={(e) => void enviar(e)}>
      <h4 class="m-0 text-sm font-semibold">
        {destino ? t('drawing.comment.on', { name: nombre }) : t('drawing.comment.atPoint')}
      </h4>
      {!destino && (
        <div class="grid grid-cols-2 gap-2">
          <FieldGroup label={t('drawing.prop.x')} htmlFor="dib-com-x">
            <Input
              id="dib-com-x"
              size="sm"
              type="number"
              min={0}
              max={ancho}
              value={String(x)}
              onChange={(e) =>
                setX(Math.min(ancho, Math.max(0, Number(e.currentTarget.value) || 0)))
              }
            />
          </FieldGroup>
          <FieldGroup label={t('drawing.prop.y')} htmlFor="dib-com-y">
            <Input
              id="dib-com-y"
              size="sm"
              type="number"
              min={0}
              max={alto}
              value={String(y)}
              onChange={(e) =>
                setY(Math.min(alto, Math.max(0, Number(e.currentTarget.value) || 0)))
              }
            />
          </FieldGroup>
        </div>
      )}
      <Textarea
        rows={2}
        aria-label={t('drawing.comment.label')}
        placeholder={t('drawing.comment.placeholder')}
        value={texto}
        disabled={bloqueado}
        onInput={(e) => {
          setTexto(e.currentTarget.value);
          setEstado('');
        }}
      />
      {bloqueado && <p class="text-text-muted m-0 text-xs">{t('drawing.comment.unsaved')}</p>}
      <div class="flex items-center justify-end gap-2">
        <span class="text-text-muted text-xs" role="status" aria-live="polite">
          {estado === 'hecho'
            ? t('drawing.comment.sent')
            : estado === 'error'
              ? t('drawing.comment.error')
              : ''}
        </span>
        <Button
          type="submit"
          size="sm"
          disabled={bloqueado || estado === 'enviando' || texto.trim() === ''}
        >
          {t('drawing.comment.send')}
        </Button>
      </div>
    </form>
  );
}
