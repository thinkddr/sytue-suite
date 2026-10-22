/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Modal y Popover reales, sin servicio: formularios largos y pies con cuatro
 * acciones para medir recortes y desplazamiento en teléfonos y ventanas bajas.
 * Solo lo importa la ruta de desarrollo, como CintaDemo.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Modal, Popover } from '@sytue/ui';
import { docsT } from '../../i18n';

export type ModoSuperposicion = 'center' | 'right' | 'sheet' | 'popover';

export function SuperposicionesDemo({ modo }: { modo: ModoSuperposicion }): JSX.Element {
  const t = docsT('de');
  const [open, setOpen] = useState(true);
  const [resultado, setResultado] = useState('');
  const acciones = [
    { id: 'svg', label: t('drawing.export.svg') },
    { id: 'pdf', label: t('drawing.export.pdf') },
    { id: 'cancel', label: t('action.cancel') },
    { id: 'save', label: t('action.save') },
  ];
  const botones = (inicio: number, fin: number): JSX.Element[] =>
    acciones.slice(inicio, fin).map((accion) => (
      <Button
        key={accion.id}
        data-demo-action={accion.id}
        variant={accion.id === 'save' ? 'primary' : 'secondary'}
        onClick={() => setResultado(accion.id)}
      >
        {accion.label}
      </Button>
    ));

  return (
    <main>
      <p data-demo-result aria-live="polite">
        {resultado}
      </p>
      {modo === 'popover' ? (
        <div class="fixed right-4 bottom-4">
          <Popover
            open={open}
            onOpenChange={setOpen}
            label="Mobile popover"
            trigger={(props) => <Button {...props}>Open</Button>}
          >
            <div class="grid gap-2">
              {Array.from({ length: 40 }, (_, i) => (
                <Button
                  key={i}
                  data-demo-action={`row-${i}`}
                  onClick={() => setResultado(`row-${i}`)}
                >
                  {t('action.apply')} {i + 1}
                </Button>
              ))}
            </div>
          </Popover>
        </div>
      ) : (
        <Modal
          open={open}
          onClose={() => setOpen(false)}
          title="Mobile modal with a long heading for a narrow viewport"
          placement={modo === 'right' ? 'right' : 'center'}
          footer={
            modo === 'center' ? (
              botones(0, 4)
            ) : (
              <div data-demo-footer class="flex w-full flex-wrap justify-between gap-2">
                <div class="flex flex-wrap gap-2">{botones(0, 2)}</div>
                <div class="flex flex-wrap gap-2">{botones(2, 4)}</div>
              </div>
            )
          }
        >
          <div class="grid gap-4">
            {Array.from({ length: 12 }, (_, i) => (
              <FieldGroup key={i} label={`Field ${i + 1}`} htmlFor={`demo-field-${i}`}>
                <Input id={`demo-field-${i}`} data-demo-field />
              </FieldGroup>
            ))}
          </div>
        </Modal>
      )}
    </main>
  );
}
