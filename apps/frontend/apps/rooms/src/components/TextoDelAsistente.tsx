/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El texto que devuelve el asistente, pintado con su formato.
 *
 * Rooms MUESTRA la respuesta, no la inserta en un editor: aquí no hay comandos
 * que ejecutar, hay que dibujarla. El resumen del canal se pintaba con
 * `whitespace-pre-wrap`, o sea en crudo, así que un modelo que responde con
 * viñetas y negrita —y responden así— dejaba a la vista `- **Ana**: pendiente`.
 *
 * SE DIBUJA DESDE LOS BLOQUES DE `@sytue/ai`, nunca desde HTML del modelo: los
 * tipos son cerrados, así que lo peor que puede llegar es un párrafo. Es la
 * misma regla que sigue el editor de Docus, con otra pintura.
 */
import type { JSX } from 'preact';
import { bloquesDe, type Trozo } from '@sytue/ai';

function Tramos({ trozos }: { trozos: Trozo[] }): JSX.Element {
  return (
    <>
      {trozos.map((t, i) => {
        if (t.negrita) return <strong key={i}>{t.texto}</strong>;
        if (t.cursiva) return <em key={i}>{t.texto}</em>;
        return <span key={i}>{t.texto}</span>;
      })}
    </>
  );
}

export function TextoDelAsistente({ texto }: { texto: string }): JSX.Element {
  const bloques = bloquesDe(texto);
  // Sin bloques —respuesta vacía o solo espacios— se devuelve el crudo: es
  // preferible enseñar algo raro a enseñar un hueco en blanco sin explicación.
  if (bloques.length === 0) return <p class="text-text text-sm whitespace-pre-wrap">{texto}</p>;

  const salida: JSX.Element[] = [];
  let lista: JSX.Element[] = [];
  const cerrarLista = (clave: number): void => {
    if (lista.length === 0) return;
    salida.push(
      <ul key={`ul${clave}`} class="text-text ml-4 list-disc space-y-1 text-sm">
        {lista}
      </ul>,
    );
    lista = [];
  };

  bloques.forEach((b, i) => {
    if (b.tipo === 'li') {
      lista.push(
        <li key={i}>
          <Tramos trozos={b.trozos} />
        </li>,
      );
      return;
    }
    cerrarLista(i);
    if (b.tipo === 'p') {
      salida.push(
        <p key={i} class="text-text text-sm">
          <Tramos trozos={b.trozos} />
        </p>,
      );
      return;
    }
    // Un resumen de canal no lleva jerarquía de documento: los tres niveles de
    // título se pintan igual, en negrita y con aire encima. Distinguir h1 de h3
    // en un cuadro de cinco líneas es ruido.
    salida.push(
      <p key={i} class="text-text mt-2 text-sm font-semibold first:mt-0">
        <Tramos trozos={b.trozos} />
      </p>,
    );
  });
  cerrarLista(bloques.length);

  return <div class="flex flex-col gap-2">{salida}</div>;
}
