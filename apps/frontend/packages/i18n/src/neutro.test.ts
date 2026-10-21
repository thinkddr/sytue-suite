/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Forma neutra en los idiomas con género gramatical (decisión del dueño, 2026-09-28):
 * los textos que hablan de otras personas —quien recibe, quien envía, quien está
 * invitada, quien colabora, quien gestiona— no las marcan como hombres. «La persona que
 * lo recibe», construcciones impersonales, «administración» en vez de «el
 * administrador»… sin forzar el idioma.
 *
 * Esto vigila las palabras que ya se han sustituido, para que no vuelvan con la
 * próxima traducción. Las etiquetas de rol de la organización (Propietario/Vlastník,
 * Miembro/Člen…) se quedan: el idioma no tiene una forma neutra natural para un
 * rótulo de una palabra, y así se le dijo al dueño.
 *
 * Las excepciones se comparan por IGUALDAD: si una deja de coincidir, hay que borrarla.
 */
import { describe, expect, test } from 'bun:test';
import { existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

const PROHIBIDAS: Record<string, RegExp[]> = {
  cs: [
    /příjemc/i,
    /odesílatel/i,
    /(?<!\p{L})hosté(?!\p{L})/iu,
    /spolupracovník/i,
    /(?<!\p{L})(?:ten|tomu|toho), kdo(?!\p{L})/iu,
  ],
  sk: [
    /príjemc/i,
    /odosielateľ/i,
    /(?<!\p{L})hostia(?!\p{L})/iu,
    /spolupracovník/i,
    /(?<!\p{L})(?:ten|tomu|toho), kto(?!\p{L})/iu,
  ],
  pl: [/odbiorc/i, /nadawc/i, /(?<!\p{L})goście(?!\p{L})/iu, /współpracowni/i, /zarządc/i],
  hr: [
    /primatelj/i,
    /pošiljatelj/i,
    /(?<!\p{L})gosti(?!\p{L})/iu,
    /suradni[kc]/i,
    /upravitelj(?! scenarij)/i,
  ],
  sl: [
    /prejemni[kc]/i,
    /pošiljatelj/i,
    /(?<!\p{L})gostje(?!\p{L})/iu,
    /sodelav[ec]/i,
    /upravitelj(?! scenarij)/i,
  ],
  es: [
    /destinatari[oa]s?/i,
    /(?<!\p{L})invitados(?!\p{L})/iu,
    /colaborador/i,
    /(?<!\p{L})(?:un|el|al|del|los|otro|tus|sus) administrador/iu,
    /(?<!\p{L})(?:el|los|un|al|del) (?:propietario|miembro)s?(?!\p{L})/iu,
  ],
  pt: [
    /destinatári/i,
    /(?<!\p{L})convidados(?!\p{L})/iu,
    /colaborador/i,
    /(?<!\p{L})(?:um|o|os|ao|do|outro) administrador/iu,
    /(?<!\p{L})(?:o|os|um|do) (?:proprietário|membro)s?(?!\p{L})/iu,
  ],
  fr: [
    /destinataire/i,
    /(?<!\p{L})invités(?!\p{L})/iu,
    /collaborateur/i,
    /(?<!\p{L})(?:un|l’|l'|les|aux|d’un|d'un|autre) administrateur/iu,
    /vous êtes connecté(?!e)/i,
  ],
  it: [
    /destinatari/i,
    /(?<!\p{L})invitati(?!\p{L})/iu,
    /collaborator[ei]/i,
    /(?<!\p{L})(?:un|l’|l'|gli|agli|degli|altro) amministrator/iu,
  ],
  de: [
    /Empfänger/,
    /Teilnehmer(?!in)/,
    /Mitarbeiter(?!in)/,
    /(?<!\p{L})(?:ein|einen|der|die|den|dem|Die|Nur) Administrator(?:en)?(?!\p{L}|in)/u,
  ],
  lt: [
    /gavėj/i,
    /siuntėj/i,
    /svečia/i,
    /bendradarbis/i,
    /(?<!\p{L})nari(?:ai|ų|us|ams)(?!\p{L})/iu,
  ],
  lv: [
    /saņēmēj/i,
    /sūtītāj/i,
    /(?<!\p{L})viesi(?!\p{L})/iu,
    /līdzstrādniek/i,
    /dalībniek(?:i|u|iem)(?!\p{L})/iu,
  ],
  ro: [
    /destinatar/i,
    /colaborator/i,
    /administrator(?:ul|ului|ii|ilor|i)(?!\p{L})/iu,
    /(?<!\p{L})membri(?!\p{L})/iu,
  ],
  bg: [
    /получател/i,
    /подател/i,
    /(?<!\p{L})гост(?:и|ите)?(?!\p{L})/iu,
    /сътрудни[кц]/i,
    /администратор(?:а|ът|и|ите)(?!\p{L})/iu,
  ],
  el: [
    /παραλήπτ/i,
    /αποστολέα|αποστολείς/i,
    /προσκεκλημένοι/i,
    /συνεργάτ/i,
    /διαχειριστ(?:ή|ές|ών)(?!\p{L})/iu,
  ],
  is: [
    /viðtakand/i,
    /sendand/i,
    /(?<!\p{L})gestir(?!\p{L})/iu,
    /samstarfsaðil/i,
    /(?<!\p{L})meðlimir(?!\p{L})/iu,
    /stjórnand(?:a|ann|anum)(?!\p{L})/iu,
  ],
  mt: [
    /riċevitur/i,
    /mittent/i,
    /mistednin/i,
    /(?<!\p{L})membri(?!\p{L})/iu,
    /(?:lil |l-)amministratur/i,
    /illoggjat/i,
  ],
};

/** `app:clave` → por qué se queda. */
const EXCEPCIONES: Record<string, string> = {
  'is/cloud-dashboard:analytics.kpi.visitors':
    '«Gestir» son las visitas anónimas de una web en la analítica, no personas identificadas',
  'cs/office:share.role.owner.desc':
    '«včetně toho, kdo…»: «toho» es el pronombre neutro de la frase, no una persona',
  'sk/office:share.role.owner.desc':
    '«vrátane toho, kto…»: «toho» es el pronombre neutro de la frase, no una persona',
  'pl/mail:aliases.whatBody': '«skrzynki odbiorczej» es la bandeja de entrada, no una persona',
  'pl/office:extdata.field.audience':
    '«Odbiorcy» traduce el `audience` de OAuth: el término técnico',
  'hr/drive:storage.external.nas.commandHelp':
    '«upravitelj spremnika» es el programa que gestiona contenedores',
  'sl/drive:storage.external.nas.commandHelp':
    '«upravitelj vsebnikov» es el programa que gestiona contenedores',
  'hr/office:names.title':
    '«Upravitelj naziva» es el rótulo de la herramienta (Administrador de nombres de Calcus), no una persona',
  'sl/office:names.title':
    '«Upravitelj imen» es el rótulo de la herramienta (Administrador de nombres de Calcus), no una persona',
};

const FRONTEND = join(import.meta.dir, '..', '..', '..');
// El común, el de cada app y el de la ofimática (`packages/office`: Docus, Calcus,
// Diapos y CertiPDF), que se nombra `office`.
const carpetas = [
  join(import.meta.dir, 'messages'),
  join(FRONTEND, 'packages', 'office', 'src/i18n/messages'),
  // El panel «Quién puede consultar» de la consola, que va en 26 idiomas antes que ella.
  join(FRONTEND, 'apps', 'console', 'src/i18n/acceso/messages'),
  ...readdirSync(join(FRONTEND, 'apps'))
    .map((a) => join(FRONTEND, 'apps', a, 'src/i18n/messages'))
    .filter(existsSync),
];
const app = (d: string): string => {
  const rel = d.slice(FRONTEND.length + 1).split('/');
  return rel[0] === 'apps' || rel[1] === 'office' ? rel[1]! : 'i18n';
};

const textos = (v: unknown): string[] =>
  typeof v === 'string'
    ? [v]
    : v && typeof v === 'object'
      ? Object.values(v as Record<string, string>)
      : [];

describe('forma neutra', () => {
  test.each(Object.keys(PROHIBIDAS))('%s no marca como hombres a las demás personas', async (l) => {
    const vistas: string[] = [];
    for (const d of carpetas) {
      if (!existsSync(join(d, `${l}.ts`))) continue;
      const cat = ((await import(join(d, `${l}.ts`))) as Record<string, Record<string, unknown>>)[
        l
      ]!;
      for (const [clave, v] of Object.entries(cat))
        if (textos(v).some((t) => PROHIBIDAS[l]!.some((r) => r.test(t))))
          vistas.push(`${app(d)}:${clave}`);
    }
    const esperadas = Object.keys(EXCEPCIONES)
      .filter((k) => k.startsWith(`${l}/`))
      .map((k) => k.slice(3));
    expect(vistas.sort()).toEqual(esperadas.sort());
  });
});
