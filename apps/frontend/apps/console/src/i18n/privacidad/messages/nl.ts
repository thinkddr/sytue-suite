/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El registro de solicitudes RGPD y la supresión física de una cuenta: traducción
 * del inglés (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const nl: AppCatalog<typeof en> = {
  'privacidad.titulo': 'Privacyverzoeken',
  'privacidad.descripcion':
    'Wat mensen uw organisatie vragen over hun persoonsgegevens (AVG, artikelen 15 tot en met 21). Registreer elk verzoek, antwoord binnen een maand en voer een wissing hier uit.',
  'privacidad.registrar': 'Verzoek registreren',
  'privacidad.filtro': 'Tonen',
  'privacidad.filtro.abiertas': 'Open',
  'privacidad.filtro.vencidas': 'Te laat',
  'privacidad.filtro.todas': 'Alle',
  'privacidad.resumen.abiertas': 'Open verzoeken',
  'privacidad.resumen.vencidas': 'Termijn verstreken',
  'privacidad.vacia': 'Geen verzoeken om te tonen.',
  'privacidad.errorCarga': 'Het register kon niet worden geladen.',
  'privacidad.col.persona': 'Persoon',
  'privacidad.col.tipo': 'Verzoek',
  'privacidad.col.recibida': 'Ontvangen',
  'privacidad.col.plazo': 'Termijn',
  'privacidad.col.estado': 'Status',
  'privacidad.tipo.access': 'Inzage in hun gegevens',
  'privacidad.tipo.rectification': 'Rectificatie',
  'privacidad.tipo.erasure': 'Wissing',
  'privacidad.tipo.restriction': 'Beperking van de verwerking',
  'privacidad.tipo.portability': 'Overdraagbaarheid',
  'privacidad.tipo.objection': 'Bezwaar',
  'privacidad.canal.email': 'E-mail',
  'privacidad.canal.web': 'Webformulier',
  'privacidad.canal.letter': 'Brief',
  'privacidad.canal.phone': 'Telefoon',
  'privacidad.canal.in_person': 'Persoonlijk',
  'privacidad.canal.other': 'Anders',
  'privacidad.estado.received': 'Ontvangen',
  'privacidad.estado.in_progress': 'In behandeling',
  'privacidad.estado.completed': 'Beantwoord',
  'privacidad.estado.rejected': 'Geweigerd',
  'privacidad.plazo.vencida': 'Te laat',
  'privacidad.plazo.pronto': 'Verloopt binnenkort',
  'privacidad.plazo.prorrogada': 'Verlengd',
  'privacidad.alta.quien': 'Wie vraagt het',
  'privacidad.alta.miembro': 'Iemand van de organisatie',
  'privacidad.alta.fuera': 'Iemand van buiten',
  'privacidad.alta.elegirMiembro': 'Kies de persoon',
  'privacidad.alta.correo': 'Hun e-mailadres',
  'privacidad.alta.nombre': 'Hun naam (optioneel)',
  'privacidad.alta.tipo': 'Wat ze vragen',
  'privacidad.alta.canal': 'Hoe het binnenkwam',
  'privacidad.alta.recibida': 'Datum van ontvangst',
  'privacidad.alta.recibidaAyuda':
    'De termijn van een maand loopt vanaf deze datum, niet vanaf vandaag.',
  'privacidad.alta.identidad': 'Identiteit gecontroleerd',
  'privacidad.alta.metodo': 'Hoe die is gecontroleerd',
  'privacidad.alta.notas': 'Notities',
  'privacidad.alta.error.subject': 'Geef aan wie het verzoek doet.',
  'privacidad.alta.error.email': 'Voer een geldig e-mailadres in.',
  'privacidad.alta.error.date':
    'De datum moet in het afgelopen jaar liggen en niet in de toekomst.',
  'privacidad.alta.error.upstream': 'Het verzoek kon niet worden geregistreerd.',
  'privacidad.alta.hecha': 'Verzoek geregistreerd.',
  'privacidad.detalle.plazo': 'Antwoorden vóór {fecha}',
  'privacidad.detalle.resolucion': 'Antwoord aan de persoon',
  'privacidad.detalle.resolucionAyuda':
    'Verplicht om het verzoek te sluiten: dit wordt aan de persoon meegedeeld.',
  'privacidad.detalle.guardar': 'Opslaan',
  'privacidad.detalle.guardado': 'Verzoek bijgewerkt.',
  'privacidad.detalle.cerrada': 'Dit verzoek is gesloten en kan niet meer veranderen.',
  'privacidad.detalle.prorroga': 'Met twee maanden verlengen',
  'privacidad.detalle.prorrogaAyuda':
    'Eenmalig en vóór de termijn verstrijkt. De persoon moet de reden horen: schrijf die hier.',
  'privacidad.detalle.prorrogar': 'Verlengen',
  'privacidad.detalle.historia': 'Geschiedenis',
  'privacidad.evento.created': 'Geregistreerd',
  'privacidad.evento.updated': 'Bijgewerkt',
  'privacidad.evento.extended': 'Termijn verlengd',
  'privacidad.evento.erasureStarted': 'Wissing gestart',
  'privacidad.evento.erasureCompleted': 'Wissing voltooid',
  'privacidad.supresion.titulo': 'Account wissen',
  'privacidad.supresion.explica':
    'Het account wordt direct en onherroepelijk gesloten en de aanmeldmethoden worden verwijderd. Binnen enkele minuten wordt het profiel geanonimiseerd en wordt verwijderd wat andere diensten over de persoon bewaren. Wat de persoon voor de organisatie heeft gemaakt (mailbox, bestanden, afspraken) blijft bij de organisatie.',
  'privacidad.supresion.confirmar': 'Typ {correo} om te bevestigen',
  'privacidad.supresion.iniciada': 'Wissing gestart. Die is binnen enkele minuten klaar.',
  'privacidad.supresion.enCurso': 'De wissing loopt.',
  'privacidad.supresion.hecha': 'Het account is gewist.',
  'privacidad.supresion.sinCuenta':
    'De persoon heeft geen account in de organisatie: hier is geen account om te wissen.',
  'privacidad.supresion.noMiembro': 'De persoon is geen lid meer van de organisatie.',
  'privacidad.supresion.error': 'Het account kon niet worden gewist.',
  'privacidad.error.upstream': 'De wijziging kon niet worden opgeslagen.',
  'privacidad.cancelar': 'Annuleren',
};
