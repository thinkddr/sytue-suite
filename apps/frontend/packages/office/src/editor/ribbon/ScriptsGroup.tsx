/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Grupo «Scripts» de la cinta, junto a «Macros» en la pestaña Vista (los tres tipos de
 * documento): un botón que abre el panel de scripts. El panel y la lógica viven en
 * dialogs/ScriptsDialog.tsx y en editor/scripts/; aquí solo el pegamento de cinta.
 *
 * El panel se baja al abrirlo por primera vez: con él vienen el anfitrión de scripts y
 * parse5 (sus lectores de HTML), ~240 KB que estaban en el editor de los tres tipos
 * aunque casi nadie lo abre. Si el trozo no llega, se dice y el siguiente clic repite.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { useToast } from '@sytue/ui';
import type { EditorContext } from '../types';
import { RibbonButton, RibbonGroup } from './primitives';
import type { ScriptsDialog as Dialogo } from '../dialogs/ScriptsDialog';

type DialogoDeScripts = typeof Dialogo;

let cargado: DialogoDeScripts | undefined;

export function ScriptsGroup({ ctx }: { ctx: EditorContext }): JSX.Element {
  const [abierto, setAbierto] = useState(false);
  const [ScriptsDialog, setScriptsDialog] = useState<DialogoDeScripts | undefined>(() => cargado);
  const toast = useToast();
  const abrir = (): void => {
    if (ScriptsDialog) {
      setAbierto(true);
      return;
    }
    import('../dialogs/ScriptsDialog').then(
      (m) => {
        cargado = m.ScriptsDialog;
        setScriptsDialog(() => m.ScriptsDialog);
        setAbierto(true);
      },
      () =>
        toast.notify({
          tone: 'danger',
          title: ctx.t('extdata.advice.offline.title'),
          description: ctx.t('extdata.advice.offline.action'),
        }),
    );
  };
  return (
    <RibbonGroup label={ctx.t('group.scripts')}>
      <RibbonButton
        commandId="scriptsManage"
        icon="shield"
        label={ctx.t('scripts.open')}
        active={abierto}
        onAction={abrir}
      />
      {abierto && ScriptsDialog && <ScriptsDialog ctx={ctx} onClose={() => setAbierto(false)} />}
    </RibbonGroup>
  );
}
