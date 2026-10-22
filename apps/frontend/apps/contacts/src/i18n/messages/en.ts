/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo INGLÉS de Sytue Contacts — FUENTE DE VERDAD (docs/CONVENTIONS.md
 * §13.1). Las cadenas nacen aquí y los otros cuatro idiomas las traducen. Una
 * clave nueva aquí rompe la compilación de es/fr/de/it hasta que la traduzcan.
 *
 * Inglés europeo (en-GB): «organisation», «favourite». El formateador ya usa
 * en-GB (`intlTag`) y mezclar ortografías en la misma interfaz canta.
 *
 * Terminología: la de Google Contacts / Outlook People en cada lengua, y la
 * MISMA que Mail y Calendar («contact», «label», «trash»): las tres apps se
 * usan seguidas y un contacto tiene que llamarse igual en las tres.
 */
import type { MessageEntry } from '@sytue/i18n';

export const en = {
  // --- Barra lateral ------------------------------------------------------
  'nav.newContact': 'New contact',
  'nav.views': 'Views',
  'nav.starred': 'Starred',
  'nav.birthdays': 'Birthdays',
  'birthdays.title': 'Upcoming birthdays',
  'birthdays.empty': 'No contact has a birthday saved.',
  'birthdays.today': 'Today',
  'birthdays.tomorrow': 'Tomorrow',
  'birthdays.inDays': { one: 'In 1 day', other: 'In {count} days' },
  'birthdays.turns': 'Turns {age}',
  'nav.trash': 'Trash',
  'nav.addressBooks': 'Address books',
  'nav.labels': 'Labels',
  'nav.newLabel': 'New label',
  'nav.newAddressBook': 'New address book',
  'nav.booksError': 'Address books could not be loaded.',
  'nav.booksEmpty': 'No address books available.',
  'nav.labelsError': 'Labels could not be loaded.',
  'nav.labelsEmpty': 'No labels yet.',

  // --- Comunes ------------------------------------------------------------
  'common.retry': 'Retry',
  'common.cancel': 'Cancel',
  'common.close': 'Close',
  'common.create': 'Create',
  'common.save': 'Save',
  'common.delete': 'Delete',
  'common.edit': 'Edit',
  'common.back': 'Back',
  'common.add': 'Add',
  'common.name': 'Name',
  'common.actionFailed': 'The action could not be completed',
  'common.tryAgain': 'Try again.',
  'common.tryAgainSoon': 'Try again in a few seconds.',

  // --- Lista de contactos -------------------------------------------------
  'list.region': 'Contact list',
  'list.title': 'Contacts',
  'list.search': 'Search contacts',
  'list.selectAll': 'Select all visible contacts',
  'list.select': 'Select {name}',
  'list.starredTitle': 'Starred',
  'list.reviewDuplicates': 'Review duplicates ({count})',
  'list.count': { one: '{count} contact', other: '{count} contacts' },
  'list.countFiltered': {
    one: '{shown} of {count} contact',
    other: '{shown} of {count} contacts',
  },
  'list.countWithLabel': '{summary} · {label}',
  'list.errorTitle': 'Contacts could not be loaded',
  'list.errorDescription':
    'There was a problem connecting to the server. Check your connection and try again.',
  'list.emptyTitle': 'No contacts',
  'list.emptySearch': 'No matches for your search. Try another term.',
  'list.emptyLabel':
    'The label “{label}” has no contacts. Assign it from an individual contact card.',
  'list.emptyBook': 'Your address book is empty. Create your first contact to get started.',
  'list.emptyStarred':
    'No starred contacts. Star the people you deal with most and they show up here.',
  'list.clearSearch': 'Clear search',
  'list.clearFilter': 'Clear filter',

  // --- Importar / exportar ------------------------------------------------
  'io.menuLabel': 'Import or export contacts',
  'io.button': 'Import / Export',
  'io.importVcard': 'Import vCard (.vcf)',
  'io.importCsv': 'Import CSV (Google/Outlook)',
  'io.exportVcard': 'Export vCard (.vcf)',
  'io.exportCsv': 'Export CSV',
  'io.exportFilename': 'contacts.vcf',
  'io.imported': { one: '{count} contact imported', other: '{count} contacts imported' },
  'io.skipped': {
    one: '{count} was skipped (duplicate or invalid).',
    other: '{count} were skipped (duplicates or invalid).',
  },
  'io.noneImported': 'No contacts were imported',
  'io.noneImportedCsv': 'The CSV had no valid new rows.',
  'io.noneImportedVcard': 'The .vcf file contained no valid cards.',
  'io.importProgress': 'Importing {done} of {total}',
  'io.vcardReadFailed': 'The .vcf file could not be read',
  'io.csvFailed': 'The CSV could not be imported',
  'io.csvFailedHint': 'Use an export from Google Contacts or Outlook.',

  // --- Etiquetas ----------------------------------------------------------
  'label.newTitle': 'New label',
  'label.renameTitle': 'Rename label',
  'label.namePlaceholder': 'Family, Work, Club…',
  'label.nameRequired': 'The label needs a name.',
  'label.created': 'Label created',
  'label.createFailed': 'The label could not be created',
  'label.actions': 'Label actions',
  'label.rename': 'Rename',
  'label.renamed': 'Label renamed',
  'label.renameFailed': 'The label could not be renamed',
  'label.delete': 'Delete label',
  'label.deleteConfirm':
    'Delete the label “{label}”? Its contacts keep all their data: only the label disappears.',
  'label.deleted': 'Label deleted',
  'label.deleteFailed': 'The label could not be deleted',
  'book.newTitle': 'New address book',
  'book.renameTitle': 'Rename address book',
  'book.namePlaceholder': 'Family, Suppliers, Personal…',
  'book.nameRequired': 'The address book needs a name.',
  'book.created': 'Address book created',
  'book.createFailed': 'The address book could not be created',
  'book.actions': 'Address book actions',
  'book.rename': 'Rename',
  'book.renamed': 'Address book renamed',
  'book.renameFailed': 'The address book could not be renamed',
  'book.delete': 'Delete address book',
  'book.deleteConfirm':
    'Delete the address book “{book}”? Its contacts are deleted too — this cannot be undone.',
  'book.deleted': 'Address book deleted',
  'book.deleteFailed': 'The address book could not be deleted',

  // --- Duplicados ---------------------------------------------------------
  'duplicates.title': 'Possible duplicates',
  'duplicates.empty': 'No duplicate contacts were found.',
  'duplicates.description':
    'Contacts that share a name, an email address or a phone number. Review each group and merge it so no card is repeated.',
  'duplicates.cards': { one: '{count} card', other: '{count} cards' },
  'duplicates.merge': 'Merge',

  // --- Combinar contactos -------------------------------------------------
  'merge.selectTitle': 'Merge with…',
  'merge.confirmTitle': 'Confirm merge',
  'merge.searchPlaceholder': 'Search the address book',
  'merge.searchLabel': 'Search for the contact to merge with',
  'merge.listLabel': 'Contacts in the address book',
  'merge.noCandidates': 'There is no other contact in this address book to merge with.',
  'merge.noMatches': 'No matches for your search.',
  'merge.explain':
    'The email addresses and phone numbers of “{loser}” will be added to “{survivor}”, its labels will move to the merged contact, and “{loser}” will then be deleted.',
  'merge.keepName': 'Name to keep',
  'merge.keepOrganization': 'Organisation to keep',
  'merge.resultEmails': { one: '{count} email address', other: '{count} email addresses' },
  'merge.resultPhones': { one: '{count} phone number', other: '{count} phone numbers' },
  'merge.result':
    'The resulting contact will have {emails} and {phones}. This action cannot be undone.',
  'merge.confirm': 'Merge contacts',
  'merge.done': 'Contacts merged',
  'merge.doneDescription': '“{loser}” has been merged into “{survivor}”.',
  'merge.partial': 'Incomplete merge',
  'merge.partialDescription':
    'The data was saved to “{survivor}”, but “{loser}” could not be deleted and remains a duplicate. Try merging them again.',
  'merge.failed': 'The contacts could not be merged',
  'merge.failedHint': 'Nothing has been deleted. Try again in a few seconds.',

  // --- Papelera -----------------------------------------------------------
  'trash.title': 'Trash',
  'trash.loading': 'Loading trash',
  'trash.loadFailed': 'The trash could not be loaded. Try again.',
  'trash.empty': 'The trash is empty.',
  'trash.description': 'Deleted contacts. Restore them or delete them permanently.',
  'trash.confirmPurge': 'Delete forever?',
  'trash.confirmYes': 'Yes, delete',
  'trash.confirmNo': 'No',
  'trash.restore': 'Restore',
  'trash.restored': 'Contact restored',
  'trash.restoreFailed': 'Could not restore',
  'trash.purged': 'Contact permanently deleted',
  'trash.purgeFailed': 'Could not delete',

  // --- Acciones en lote ---------------------------------------------------
  'batch.region': 'Actions for the selection',
  'batch.selected': { one: '{count} contact selected', other: '{count} contacts selected' },
  'batch.star': 'Star',
  'batch.starred': { one: '{count} contact starred', other: '{count} contacts starred' },
  'batch.unstar': 'Remove star',
  'batch.unstarred': 'Star removed',
  'batch.addToLabel': 'Add to label',
  'batch.removeFromLabel': 'Remove from label',
  'batch.addedTo': 'Added to “{label}”',
  'batch.removedFrom': 'Removed from “{label}”',
  'batch.toTrash': 'Move to trash',
  'batch.trashed': {
    one: '{count} contact moved to the trash',
    other: '{count} contacts moved to the trash',
  },

  // --- Ficha del contacto -------------------------------------------------
  'detail.emptyTitle': 'Select a contact',
  'detail.emptyDescription': 'Choose a contact from the list to see its card, or create a new one.',
  'detail.sendEmail': 'Send email',
  'detail.meeting': 'Create a meeting',
  'detail.meetingTitle': 'Meeting with {name}',
  'detail.copyEmail': 'Copy the email',
  'detail.emailCopied': 'Email copied',
  'detail.copyFailed': 'Could not copy',
  'detail.export': 'Export',
  'detail.merge': 'Merge with…',
  'detail.moreActions': 'More actions',
  'detail.infoTitle': 'Contact information',
  'detail.noLabels': 'No labels',
  'detail.removeLabel': 'Remove label {label}',
  'detail.addLabel': 'Add label',
  'detail.noLabelsAvailable': 'There are no labels. Create them from the sidebar.',
  'detail.allLabelsAssigned': 'This contact already has every label.',
  'detail.labelFailed': 'The label could not be updated',
  'detail.star': 'Star contact',
  'detail.unstar': 'Remove from starred',
  'detail.starFailed': 'The star could not be updated',

  // --- Foto ---------------------------------------------------------------
  'photo.change': 'Change contact photo',
  'photo.add': 'Add a photo to the contact',
  'photo.remove': 'Remove photo',
  'photo.presets': 'Preset colours',
  'photo.upload': 'Upload a photo…',
  'photo.updated': 'Photo updated',
  'photo.updateFailed': 'The photo could not be updated',
  'photo.removed': 'Photo removed',
  'photo.removeFailed': 'The photo could not be removed',
  'photo.errorFormat': 'Unsupported format. Use JPEG, PNG, WebP or GIF.',
  'photo.errorProcess': 'The image could not be processed.',
  'photo.errorTooLarge': 'The image is too large even after optimising it.',
  'photo.errorRead': 'The image could not be read.',

  // --- Campos -------------------------------------------------------------
  'field.email': 'Email',
  'field.phone': 'Phone',
  'field.organization': 'Organisation',
  'field.labels': 'Labels',
  'field.address': 'Address',
  'field.birthday': 'Birthday',
  'field.note': 'Note',

  // --- Formulario de contacto --------------------------------------------
  'form.editTitle': 'Edit contact',
  'form.newTitle': 'New contact',
  'form.preview': 'Preview',
  'form.fullName': 'Full name',
  'form.fullNamePlaceholder': 'First and last name',
  'form.ai': 'Paste a signature or card',
  'form.aiPlaceholder':
    'Ana Ruiz — Sales Director, Acme Ltd · ana.ruiz@acme.co.uk · +44 20 7123 4567',
  'form.aiFill': 'Fill in',
  'form.aiError': "Couldn't find contact details in that text.",
  'form.nameRequired': 'The contact needs a name.',
  'form.selectBook': 'Select an address book',
  'form.emailPlaceholder': 'name@domain.eu',
  'form.emailInvalid': 'Enter a valid email address (name@domain.eu).',
  'form.addEmail': 'Add email',
  'form.emailTypeLabel': 'Label of email address {index}',
  'form.emailLabel': 'Email address {index}',
  'form.removeEmail': 'Remove email address {index}',
  'form.phonePlaceholder': '+34 600 000 000',
  'form.addPhone': 'Add phone number',
  'form.phoneTypeLabel': 'Label of phone number {index}',
  'form.phoneLabel': 'Phone number {index}',
  'form.removePhone': 'Remove phone number {index}',
  'form.primary': 'Primary',
  'form.addAddress': 'Add address',
  'form.addressTypeLabel': 'Label of address {index}',
  'form.removeAddress': 'Remove address {index}',
  'form.street': 'Street and number',
  'form.streetLabel': 'Street, address {index}',
  'form.city': 'City',
  'form.cityLabel': 'City, address {index}',
  'form.region': 'Province or region',
  'form.regionLabel': 'Province or region, address {index}',
  'form.postalCode': 'Postcode',
  'form.postalCodeLabel': 'Postcode, address {index}',
  'form.country': 'Country',
  'form.countryLabel': 'Country, address {index}',
  'form.birthdayPartial': 'A birthday without a year is stored; pick a date to replace it.',
  'form.organizationPlaceholder': 'Company or team',
  'form.notePlaceholder': 'Additional information (optional)',
  'form.confirmDelete': 'Confirm deletion',
  'form.created': 'Contact created',
  'form.updated': 'Contact updated',
  'form.saveFailed': 'The contact could not be saved',
  // El borrado es SUAVE (el contacto queda en la papelera): decir «eliminado» a
  // secas hacía creer que no había vuelta atrás.
  'form.deleted': 'Contact moved to the trash',
  'form.deletedHint': 'You can restore it from Trash.',
  'form.deleteFailed': 'The contact could not be deleted',

  // --- Tipos de correo/teléfono/dirección --------------------------------
  'type.home': 'Home',
  'type.work': 'Work',
  'type.other': 'Other',
  // Clave aparte porque en español el adjetivo concuerda: «dirección» es
  // femenino («Otra») y «correo»/«teléfono» masculinos («Otro»).
  'type.otherAddress': 'Other',
  'type.mobile': 'Mobile',
  'type.fax': 'Fax',
} satisfies Record<string, MessageEntry>;
