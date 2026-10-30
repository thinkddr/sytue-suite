/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo del sitio público en INGLÉS: la fuente de verdad (docs/CONVENTIONS.md
 * §13.1). Los otros cuatro idiomas derivan su TIPO de este objeto, así que una
 * clave nueva aquí rompe la compilación de es/fr/de/it en vez de aparecer sin
 * traducir en producción.
 *
 * Reglas del escaparate:
 *   - Los NOMBRES de producto (Sytue, One, Mail, Drive, Docs, Calendar, Contacts,
 *     Rooms, Cloud, ID) son marca: NO se traducen en ningún idioma.
 *   - Los reclamos sí: se adaptan, no se calcan. Un titular que en alemán no cabe
 *     no es una traducción válida.
 *   - Ortografía en-GB (somos una suite europea): «organisation», no «organization».
 */

export const en = {
  // --- Cromo del sitio ---------------------------------------------------
  'skip.content': 'Skip to content',
  'nav.products': 'Products',
  'nav.sovereignty': 'Sovereignty',
  'nav.standards': 'Open standards',
  'nav.pricing': 'Pricing',
  'footer.pricing': 'Pricing',
  'home.pricing.title': 'Three plans, the whole suite',
  'home.pricing.body':
    'The same products on all three: what changes is the storage per user. No cut-down editions.',
  'home.pricing.amount': '€5',
  'home.pricing.tax': 'excluding VAT',
  'home.pricing.per': '/User/Month',
  'home.pricing.cta': 'Talk to sales',
  'home.pricing.commitment.title': 'A commitment for life',
  'home.pricing.commitment.body':
    'The price per user goes up by €0.50 every four years, and nothing else: for life. You know it the day you join and it does not change.',
  'home.pricing.included.title': 'A single edition',
  'home.pricing.included.body':
    'No per-feature plans, no tiers, no cut-down editions: the whole suite is in the per-user price, with the same features for everyone.',
  'home.pricing.metered.title': 'What is billed separately',
  'home.pricing.metered.body': 'Extra storage: by usage or in blocks.',
  'home.pricing.storage.title': 'Storage is shared',
  'home.pricing.storage.body':
    'Each person’s quota adds to the organisation’s: what one does not use, the others enjoy. And the per-user quota can be adjusted from the organisation.',
  'nav.downloads': 'Downloads',
  'nav.aria': 'Site navigation',
  'nav.ariaMobile': 'Mobile navigation',
  'nav.home': 'Sytue One — Home',
  'nav.openMenu': 'Open the menu',
  'nav.closeMenu': 'Close the menu',
  'theme.toLight': 'Switch to light mode',
  'theme.toDark': 'Switch to dark mode',
  'lang.label': 'Language',
  'cta.start': 'Start free',
  'cta.signIn': 'Sign in',
  'cta.seeSuite': 'See the suite',

  // --- Portada -----------------------------------------------------------
  'meta.home.title': 'Sytue One — The sovereign European productivity suite',
  'meta.home.description':
    'Sytue One: the sovereign European productivity suite. Identity, mail, files, documents, calendar and chat, 100% in the EU and built on open standards.',
  'home.hero.lead': 'Your productivity,',
  'home.hero.accent': 'sovereign and European',
  'home.hero.subtitle':
    'Mail, files, documents, calendar, chat and video calls — integrated as one single application and hosted entirely in the EU.',
  'home.hero.note': 'No card required. Data in Scaleway (EU). Assisted migration.',
  'home.privacy.title': 'Your work is yours. So is your privacy.',
  'home.privacy.body': 'We do not sell your data or use it to build advertising profiles.',
  'home.privacy.anonymity':
    'Sytue Analytics protects your privacy with aggregate statistics, no tracking cookies and pseudonymous visitor identifiers that change daily and stay separate for each site.',
  'home.privacy.account':
    'Using the suite requires account details and access logs so we can provide the service and keep it secure.',
  'home.privacy.link': 'How we protect your privacy',

  'home.stats.aria': 'European footprint',
  'home.stats.regions': 'EU regions (fr-par · nl-ams · on-premise Madrid)',
  'home.stats.apps': 'Apps behind a single identity',
  'home.stats.standards': 'Open standards: OIDC, JMAP, CalDAV, CardDAV and WebDAV',

  'home.products.title': 'One suite, one experience',
  'home.products.subtitle':
    'Every product shares the same design, identity and shell. They look and behave as a single application.',

  'home.integration.title': 'Integration the way a suite should work',
  'home.integration.subtitle':
    'One sign-up opens everything. Automatic provisioning (SCIM) from Sytue ID.',
  'home.integration.sso.title': 'Single sign-on',
  'home.integration.sso.body': 'One identity (Sytue ID) for the whole suite. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Shared shell',
  'home.integration.shell.body':
    'Topbar, app switcher, search and global notifications in every product.',
  'home.integration.cross.title': 'Cross-app actions',
  'home.integration.cross.body':
    'Attach from Drive inside Mail, turn an email into a Calendar event and share it in Rooms without leaving the flow.',

  'home.sovereignty.title': 'Sovereignty, for real',
  'home.sovereignty.body':
    'This is not a marketing promise: it is the architecture. The infrastructure runs in European regions only, and everything is built on open, auditable protocols with no back doors.',
  'home.sovereignty.link': 'Why open standards',
  'home.sovereignty.eu.title': '100% in the EU',
  'home.sovereignty.eu.body':
    'All infrastructure in the EU: Scaleway (fr-par / nl-ams) plus an on-premise zone in Madrid. No US providers for personal data.',
  'home.sovereignty.gdpr.title': 'GDPR compliance',
  'home.sovereignty.gdpr.body':
    'DPA, records of processing, data subject request runbook and retention. Encrypted in transit (TLS 1.3) and at rest.',
  'home.sovereignty.lockin.title': 'No lock-in',
  'home.sovereignty.lockin.body':
    'Built on open standards: OIDC, JMAP, CalDAV, CardDAV and WebDAV. Your data stays portable.',

  'home.standards.overline': 'Original work by Sytue',
  'home.standards.title': 'Built on open standards',
  'home.standards.body':
    'Every Sytue product is original work, written in Go and Astro + Preact on open protocols (OIDC, JMAP, CalDAV, CardDAV, WebDAV). No third-party engines: your data is portable and your suite is sovereign.',

  'home.cta.title': 'Take back control of your work',
  'home.cta.body':
    'Start with your organisation today. Assisted migration from Google Workspace or Microsoft 365.',
  'home.cta.primary': 'Create my organisation',
  'home.cta.secondary': 'Talk to sales',

  // --- Rejilla de productos ----------------------------------------------
  'grid.discover': 'Discover',
  'product.mail.blurb': 'Private, European mail over JMAP — no trackers, no ads.',
  'product.drive.blurb': 'Files and collaboration, under your own brand.',
  'product.docs.blurb': 'Documents, spreadsheets and slides with real-time co-editing.',
  'product.calendar.blurb': 'A CalDAV calendar wired into the rest of the suite.',
  'product.contacts.blurb': 'CardDAV contacts, shared across your organisation.',
  'product.notes.blurb': 'Notebooks that hold anything: text, files, events, whole documents.',
  'product.photos.blurb':
    'Photo gallery and editor: your originals in Drive, your edits as a recipe.',
  'product.rooms.blurb': 'Channels, messages and team video calls in one place.',
  'product.cast.blurb':
    'Video on demand and live streaming: upload, broadcast and share, in the EU.',
  'product.pages.blurb':
    'Websites and online shops for your business, designed your way and published in the EU.',
  'product.bases.blurb':
    'Collaborative relational tables for customers, orders or stock, in the EU.',
  'product.cloud.blurb': 'A deployment and managed-data platform, in the EU.',
  'product.id.blurb': 'A single identity (SSO + MFA) with Sytue ID.',
  'product.one.blurb':
    'Your front page of the suite: apps, quick actions, recent activity and notices.',
  'product.sheets.blurb':
    'Spreadsheets with formulas, external data and charts, edited by many hands.',
  'product.slides.blurb': 'Presentations with your brand, themes and a presenter view.',
  'product.pdf.blurb': 'Read, annotate, sign and arrange your PDFs without them leaving Europe.',
  'product.console.blurb': 'Organisation administration: people, access, compliance and audit.',
  'product.erp.blurb':
    'Twenty-four modules and a double-entry ledger the database will not let you unbalance.',

  // --- Fichas de producto -------------------------------------------------
  'productPage.back': 'The whole suite',
  'productPage.features': 'Everything {name} does',
  'productPage.featuresPlanned': 'What {name} will do',
  'productPage.builtOn': 'Built on {standard}',
  'productPage.fromScratch':
    "The Docus, Calcus, Diapos and CertiPDF editor is written from scratch: Sytue's own engine, not a third-party integration.",
  'productPage.shot': 'The main screen of {name}, exactly as it looks when you sign in.',
  'productPage.shotFeatures': '{name} at work, with its features on screen.',
  'productPage.open': 'Open {name}',
  'productPage.enDesarrollo': 'This product is in development: it is not available yet.',
  'productPage.included':
    'Included in Sytue One: one identity (Sytue ID) and a single sign-up give access to every product. Data 100% in the EU (Scaleway) and portable over open standards.',

  'product.mail.tagline': 'Private, European mail with no trackers and no ads.',
  'product.mail.standard': 'JMAP (open standard)',
  'product.mail.p1': 'A fast inbox with threads, labels, filters and search.',
  'product.mail.p2': 'Attach files straight from Sytue Drive.',
  'product.mail.p3': 'Turn an email into a Calendar event.',
  'product.mail.p4':
    'Encrypted in transit (TLS 1.3). Sytue’s infrastructure is in the EU. The location of external services depends on their provider.',

  'product.drive.tagline': 'Your files and your collaboration, under your organisation’s brand.',
  'product.drive.standard': 'WebDAV (open standard)',
  'product.drive.p1': 'Upload, organise and share with controlled links (permission and expiry).',
  'product.drive.p2': 'Inline preview for documents and images.',
  'product.drive.p3': 'Attach to Mail without duplicating the file.',
  'product.drive.p4': 'Storage backed by S3 in European regions.',

  'product.docs.tagline': 'Documents, spreadsheets and slides with real-time co-editing.',
  'product.docs.standard': 'Real-time collaborative editing',
  'product.docs.p1': 'Write together, with live presence and instant changes.',
  'product.docs.p2': 'Spreadsheets with formulas and conditional formatting.',
  'product.docs.p3': 'Presentations that carry your brand.',
  'product.docs.p4': 'Everything saved in your European Drive.',

  'product.calendar.tagline': 'Your calendar, wired into the rest of the suite.',
  'product.calendar.standard': 'CalDAV (open standard)',
  'product.calendar.p1': 'Day, week and month views.',
  'product.calendar.p2': 'Recurring events and reminders.',
  'product.calendar.p3': 'Create events straight from an email in Mail.',
  'product.calendar.p4': 'Works with any CalDAV client.',

  'product.contacts.tagline': 'Your organisation’s contacts, shared and portable.',
  'product.contacts.standard': 'CardDAV (open standard)',
  'product.contacts.p1': 'A shared address book across your organisation.',
  'product.contacts.p2': 'Import and export in vCard.',
  'product.contacts.p3': 'Merge duplicates without losing data.',
  'product.contacts.p4': 'Works with any CardDAV client.',
  'product.notes.tagline': 'Everything you want to keep, in one place and in your hands.',
  'product.notes.standard': 'Own model on open standards (own REST API)',
  'product.notes.p1': 'Nested notebooks, on a free canvas, in columns or as a list.',
  'product.notes.p2':
    'Notes of any kind: text, to-dos, links, files, Calendar events, Docs documents.',
  'product.notes.p3': 'End-to-end encryption: the passphrase never leaves your browser.',
  'product.notes.p4': 'You decide what goes into search — and what never does.',

  'product.rooms.tagline': 'Channels, messages and team video calls, unified.',
  'product.rooms.standard': 'Real-time messaging and calls',
  'product.rooms.p1': 'Channels per team or project.',
  'product.rooms.p2': 'A video call embedded in every channel.',
  'product.rooms.p3': 'Mentions and notifications unified across the suite.',
  'product.rooms.p4': 'Searchable history from anywhere in Sytue One.',

  'product.cloud.p4': 'Custom domains with automatic TLS.',

  'product.cloud.p3': 'Managed S3 storage and functions.',
  'product.cloud.p2': 'Postgres database with an automatic REST API.',
  'product.cloud.p1': 'A preview for every PR and instant rollback.',
  'product.cloud.standard': 'Kubernetes-native PaaS',
  'product.cloud.tagline': 'Your code deployed and your data managed, in the European Union.',
  'product.id.tagline': 'One identity for the whole suite. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (open standard)',
  'product.id.p1': 'Single sign-on across every product.',
  'product.id.p2': 'MFA and passkeys (WebAuthn).',
  'product.id.p3': 'Automatic provisioning (SCIM) from your directory.',
  'product.id.p4': 'Profile and security in one place.',
  'product.one.tagline': 'All of Sytue One one click away, through the same door.',
  'product.one.standard': 'A single sign-in (Sytue ID) for the whole suite',
  'product.one.p1': 'A launcher with all your apps and the ones your organisation connects.',
  'product.one.p2': 'Search across the suite (⌘K) from any screen.',
  'product.one.p3':
    'Today’s agenda and unread mail on the front page, next to what is recent across the suite and your notifications.',
  'product.one.p4': 'Connect your devices and download the native apps.',
  'product.sheets.tagline': 'Numbers that make sense, in a sheet that works with you.',
  'product.sheets.standard': 'ODF/OOXML formats and real-time collaborative editing',
  'product.sheets.p1': 'Formulas, conditional formatting and data validation.',
  'product.sheets.p2': 'Bring in outside data and query it without leaving the sheet.',
  'product.sheets.p3': 'Charts, sparklines, slicers and data tables.',
  'product.sheets.p4': 'Export to ODS and XLSX; open Excel files.',
  'product.slides.tagline': 'Tell the story; the slide keeps up.',
  'product.slides.standard': 'ODF/OOXML formats and real-time collaborative editing',
  'product.slides.p1': 'Presentation themes with your organisation’s identity.',
  'product.slides.p2': 'Presenter view with notes and the next slide.',
  'product.slides.p3': 'Images, shapes, charts and SmartArt.',
  'product.slides.p4': 'Export to ODP, PPTX and PDF; open PowerPoint files.',
  'product.pdf.tagline': 'PDF, start to finish, in your browser.',
  'product.pdf.standard': 'PDF (ISO 32000) with standard annotations and forms',
  'product.pdf.p1': 'Highlight, underline, comment and draw on the document.',
  'product.pdf.p2': 'Forms: fill in fields or create your own.',
  'product.pdf.p3': 'Merge, extract, rotate, number and watermark.',
  'product.pdf.p4': 'Signature panel, permanent redaction and password-protected documents.',
  'product.console.tagline': 'Govern the suite without leaving one screen.',
  'product.console.standard': 'SCIM, SAML and OpenID Connect (open standards)',
  'product.console.p1': 'Users, groups (dynamic ones too) and roles.',
  'product.console.p2': 'SSO with your provider and SCIM provisioning.',
  'product.console.p3': 'Conditional access rules, devices and privileged access with approval.',
  'product.console.p4': 'Audit log you can export to your SIEM, and legal holds.',
  'product.erp.tagline': 'The whole ERP, with the model catalogue as a data file.',
  'product.erp.standard': 'Double-entry bookkeeping, a JSON model catalogue and a single REST API',
  'product.erp.p1':
    'Twenty-four modules and thirty-six models: contacts, sales, purchasing, inventory, manufacturing, invoicing, accounting, projects and HR.',
  'product.erp.p2':
    'Adding a module means writing a JSON file: no migration, no new route and no interface work.',
  'product.erp.p3':
    'Money is stored in whole cents, never in floating point, and the database enforces the balance.',
  'product.erp.p4':
    'Posted entries are untouchable: you correct them with another entry, which is what separates a ledger from a spreadsheet.',
  'product.one.f1':
    'A launcher with the suite’s apps and the third-party ones your organisation connects.',
  'product.one.f2':
    'Quick actions: write an email, new document, upload to Drive, create an event.',
  'product.one.f3':
    'Recent across the suite: the last things you touched in Drive, Docus, Mail or Rooms.',
  'product.one.f4': 'Global search with ⌘K or Ctrl K, with results from every app.',
  'product.one.f5': 'Notification centre and presence shared across the suite.',
  'product.one.f6':
    'Connect your devices: a profile for iPhone and Mac, Android and desktop, in two steps.',
  'product.one.f7': 'Downloads of the native desktop and mobile apps.',
  'product.one.f8': 'A welcome assistant that gets the organisation ready without manuals.',
  'product.mail.f1': 'Conversation threads with reading, reply and forward in one click.',
  'product.mail.f2':
    'Folders, colour labels and search with operators (from:, has:attachment, is:unread…).',
  'product.mail.f3': 'Automatic rules that archive, label or move mail as it arrives.',
  'product.mail.f4': 'Scheduled send and “Undo send”.',
  'product.mail.f5': 'Signatures, sender aliases and an automatic vacation reply.',
  'product.mail.f6': 'Shared and delegated mailboxes with read and send permissions.',
  'product.mail.f7':
    'Rich-text composer with attachments from Sytue Drive without duplicating files.',
  'product.mail.f8': 'Remote content blocked by default: nobody learns when you open an email.',
  'product.mail.f9': 'Signed mail (PGP and S/MIME) recognised and flagged; TLS in transit.',
  'product.mail.f10':
    'Access from Thunderbird, Outlook, Apple Mail or your phone too, with app passwords.',
  'product.mail.f11':
    'Recipient chips with the Contacts address book one click away, and a warning if you say «attached» without attaching.',
  'product.mail.f12':
    'Rules straight from an open email, empty the trash in one go, and keyboard shortcuts with «?».',
  'product.drive.f1': 'My drive, team drives, shared with me, recent, favourites and trash.',
  'product.drive.f2': 'Drag to upload; folders, move, copy, rename and multiple selection.',
  'product.drive.f3': 'Sharing links with two levels (view or edit), password and expiry.',
  'product.drive.f4': 'Version history with restore.',
  'product.drive.f5': 'Preview documents, images and PDFs without downloading.',
  'product.drive.f6':
    'Open and edit office documents in Docus, Calcus and Diapos; PDFs in CertiPDF.',
  'product.drive.f7': 'Sensitivity labels that block public links where they do not belong.',
  'product.drive.f8': 'Share to a Rooms channel or attach to Mail without duplicating the file.',
  'product.drive.f9': 'WebDAV access and a desktop client to see your files on your computer.',
  'product.drive.f10': 'S3 storage in European regions, with your quota always visible.',
  'product.docs.f1':
    'Real-time collaborative editing, with presence and without stepping on each other.',
  'product.docs.f2': 'A tabbed ribbon: Home, Insert, References, Mailings and Review.',
  'product.docs.f3': 'Tables, images, shapes, SmartArt, charts, symbols and watermarks.',
  'product.docs.f4': 'References: table of contents, footnotes, citations and bibliography.',
  'product.docs.f5': 'Mailings: data merge for letters and labels.',
  'product.docs.f6': 'Threaded comments, change review and versions.',
  'product.docs.f7':
    'Optional AI assistant to rewrite, translate or summarise, only if your organisation enables it.',
  'product.docs.f8': 'Export to ODT, DOCX and PDF; open Word files from Drive.',
  'product.docs.f9': 'Headers, footers, page numbering and reusable blocks.',
  'product.docs.f10': 'Automatic saving to your European Drive, with history.',
  'product.docs.f11':
    'Download from the front page as PDF, HTML, Markdown, OpenDocument or Microsoft Office.',
  'product.sheets.f1': 'Formulas with contextual help and instant recalculation.',
  'product.sheets.f2': 'Conditional formatting, data validation and locale-aware number formats.',
  'product.sheets.f3': 'Charts of several kinds, sparklines and slicers.',
  'product.sheets.f4': 'Data tables with totals and sorting; pivot table.',
  'product.sheets.f5': 'External data: connect a source and query it from the sheet.',
  'product.sheets.f6': 'Solver to find the value that meets a target.',
  'product.sheets.f7': 'Macros to automate repetitive tasks.',
  'product.sheets.f8': 'Real-time collaboration with presence and comments.',
  'product.sheets.f9': 'Filters, slicers and search inside the sheet.',
  'product.sheets.f10': 'Export to ODS, XLSX and PDF; open Excel files from Drive.',
  'product.slides.f1': 'Themes and palettes consistent with your organisation’s brand.',
  'product.slides.f2': 'A canvas with shapes, images, charts, SmartArt and connectors.',
  'product.slides.f3': 'Present full screen with presenter view and notes.',
  'product.slides.f4': 'Real-time collaboration with presence and comments.',
  'product.slides.f5': 'Slides with backgrounds, posters and layouts.',
  'product.slides.f6': 'Export to ODP, PPTX and PDF.',
  'product.slides.f7': 'Open PowerPoint files from Drive.',
  'product.slides.f8': 'Automatic saving with versions.',
  'product.pdf.f1': 'A fast viewer with outline, thumbnails, search and zoom to page or width.',
  'product.pdf.f2':
    'Annotations: highlight, underline, strike through, comment, draw, shapes, arrows and stamps.',
  'product.pdf.f3': 'Text boxes, measurements and area snapshots.',
  'product.pdf.f4': 'Forms: fill in existing fields or add your own.',
  'product.pdf.f5': 'Document signature panel.',
  'product.pdf.f6':
    'Permanent redaction: what you mark disappears from the file, not just from view.',
  'product.pdf.f7':
    'Pages: merge, extract, delete, rotate, number (Bates too), headers and footers.',
  'product.pdf.f8': 'Watermark, background and editable metadata.',
  'product.pdf.f9': 'Password-protected documents and permissions.',
  'product.pdf.f10': 'Read aloud and threaded comments.',
  'product.calendar.f1': 'Month, week, day and agenda views, with a mini calendar and “Today”.',
  'product.calendar.f2':
    'Daily, weekly or monthly repeats with an end; edit just this one, this and following, or all.',
  'product.calendar.f3': 'Reminders per event and notifications inside the suite.',
  'product.calendar.f4': 'Invitations with a reply (going, maybe, no) and follow-up.',
  'product.calendar.f5': '“Find a slot” free among the attendees.',
  'product.calendar.f6': 'Bookable rooms and resources with availability.',
  'product.calendar.f7': 'Shared calendars with permissions, and the ones others share with you.',
  'product.calendar.f8':
    'Subscribe to external calendars (ICS) and keep several colour-coded calendars.',
  'product.calendar.f9': 'Create an event from a Mail message or from a note.',
  'product.calendar.f10': 'Works with any CalDAV client: Apple Calendar, Thunderbird and more.',
  'product.calendar.f11':
    'Duplicate an event or copy it to your calendar, and download it as .ics for anyone outside Sytue.',
  'product.contacts.f1': 'Address books, including your organisation’s shared one.',
  'product.contacts.f2':
    'A full card: phones, emails, addresses, organisation, birthday, notes and photo.',
  'product.contacts.f3': 'Labels and favourites to group and find.',
  'product.contacts.f4': 'Duplicate detection and merging without losing data.',
  'product.contacts.f5': 'Import and export vCard and CSV; export a single contact.',
  'product.contacts.f6': 'Batch actions and a trash with recovery.',
  'product.contacts.f7': 'Instant search and autocomplete as you type in Mail.',
  'product.contacts.f8': 'Works with any CardDAV client.',
  'product.contacts.f9': 'Upcoming birthdays from your address book, flagging this month’s.',
  'product.contacts.f10':
    'Create a meeting with a contact or copy their email without leaving their card.',
  'product.notes.f1':
    'Nested notebooks with icon and colour; three layouts: free, columns and list.',
  'product.notes.f2':
    'Twelve note types: text, tasks, links, colour, image and references to Drive, Docus, Calendar, Rooms and other notes.',
  'product.notes.f3': 'An editor with formatting (bold, lists, links) that saves when you leave.',
  'product.notes.f4': 'Tags, pin, duplicate, move between notebooks, group and drag on the canvas.',
  'product.notes.f5': 'Visibility per notebook: private, personal, organisation or public link.',
  'product.notes.f6': 'Encryption in your browser for sensitive notebooks: Sytue cannot read them.',
  'product.notes.f7': 'You decide which notebooks enter the suite search.',
  'product.notes.f8': 'Backlinks: see which notes point to this one.',
  'product.notes.f9': 'Export a whole notebook to HTML, PDF, Markdown and more; import copies.',
  'product.notes.f10': 'Share a note to a Rooms channel; drop or paste to create notes.',
  'product.notes.f11':
    'Tables in the editor and Markdown shortcuts as you type: «- », «# », «> » and «```».',
  'product.notes.f12':
    'Move a note to another notebook, export a single note and sort the notebook however you like.',
  'product.photos.tagline': 'Your photos, in order and to your taste, without leaving Europe.',
  'product.photos.standard': 'Originals in Sytue Drive; editing engine in your browser',
  'product.photos.p1': 'A timeline by month with search by name.',
  'product.photos.p2': 'Albums with a cover; full-screen viewer.',
  'product.photos.p3': 'An editor with light, colour, rotate, mirror and crop, in real time.',
  'product.photos.p4': 'Every edit is a recipe: the original is never touched.',
  'product.photos.f1': 'A timeline grouped by month, with search by name and crisp thumbnails.',
  'product.photos.f2': 'Albums: create, rename, pick a cover, add from the viewer and remove.',
  'product.photos.f3': 'Full-screen viewer with keyboard navigation.',
  'product.photos.f4':
    'An editor with four sliders (exposure, contrast, saturation, temperature), rotate and mirror.',
  'product.photos.f5': 'Drag to crop and keyboard shortcuts; GPU preview with a CPU fallback.',
  'product.photos.f6': 'Every edit is a recipe stored apart: the Drive original is never modified.',
  'product.photos.f7':
    'Full-resolution export, rendered on the server with the same formulas you saw.',
  'product.photos.f8': 'Trash with restore.',
  'product.photos.f9': 'Originals live in your Sytue Drive; derivatives are a regenerable cache.',
  'product.photos.f10': 'EXIF data read from the file itself; everything in the European Union.',
  'product.photos.f11':
    'Multiple selection by month: to a new album, to the trash or downloaded in one go.',
  'product.photos.f12':
    'Slideshow, full size, rotate from the viewer and cropping with a fixed ratio.',
  'product.rooms.f1': 'Public and private channels in categories; direct messages and groups.',
  'product.rooms.f2': 'Threads, reactions, mentions, pins, saved messages and drafts.',
  'product.rooms.f3':
    'Formatting with an “Aa” bar and shortcuts: bold, italic, code, quotes, lists and links.',
  'product.rooms.f4': 'Attachments, images, GIFs and custom emoji; files from Drive.',
  'product.rooms.f5': 'Scheduled send, priority, read acknowledgement and “seen by”.',
  'product.rooms.f6': 'Slash commands and cards: polls and assignable tasks.',
  'product.rooms.f7':
    'Video calls inside the channel: camera, screen, raised hand, chat and recording.',
  'product.rooms.f8': 'Scheduled meetings and invite links.',
  'product.rooms.f9': 'Bots, webhooks and integrations with other tools.',
  'product.rooms.f10': 'Presence and custom status; per-channel notification settings.',
  'product.rooms.f11': 'Search the history, and from across the suite.',
  'product.rooms.f12': 'Real time over WebSocket, with reconnection and no lost messages.',
  'product.rooms.f13':
    'Quick channel switcher with Ctrl+G, mark everything as read, and shortcuts with «?».',
  'product.cast.tagline':
    'Your videos and your live streams, on demand and in real time, without leaving Europe.',
  'product.cast.standard':
    'HLS playback in the browser; RTMP ingest from any broadcasting software',
  'product.cast.p1': 'Upload a video and it is transcoded into several qualities for any screen.',
  'product.cast.p2': 'Go live from broadcasting software, from the browser or from the app.',
  'product.cast.p3':
    'Four levels of visibility: public, unlisted, organisation or specific people.',
  'product.cast.p4': 'Live chat and comments, with basic moderation by the owner.',
  'product.cast.f1': 'Upload videos of up to 8 GiB; the original is kept.',
  'product.cast.f2':
    'Adaptive playback in three qualities, with the browser choosing the best one.',
  'product.cast.f3':
    'Live streaming from broadcasting software over RTMP, with a stream key per person.',
  'product.cast.f4':
    'Live from the browser or app: camera and microphone. Screen sharing in supported browsers, desktop systems and Android; unavailable in the iOS/iPadOS app.',
  'product.cast.f5': 'Live recordings become videos on demand automatically.',
  'product.cast.f6': 'A public front page and a channel per organisation, indexable.',
  'product.cast.f7':
    'Unlisted links, organisation-only videos, or access for specific people by @username.',
  'product.cast.f8':
    'Live chat and threaded comments, moderated by the owner and the organisation admins.',
  'product.cast.f9': 'Embeddable player and oEmbed for public and unlisted videos.',
  'product.cast.f10':
    'Storage counted against your Sytue Drive quota; everything in the European Union.',
  'product.cast.f11':
    '“Upload to Cast” from Sytue Drive: a copy of the video goes to Cast and the file stays in Drive.',
  'product.pages.tagline':
    'Your company website and its shop, designed in a visual editor and published without leaving Europe.',
  'product.pages.standard':
    'Static, accessible HTML and CSS with no third-party cookies; export to Astro',
  'product.pages.p1':
    'A visual editor with frames that adapt to desktop, tablet and mobile on their own.',
  'product.pages.p2': 'Ten templates by sector, with real copy and public-domain photos.',
  'product.pages.p3': 'A shop with catalogue, cart and payments into your company account.',
  'product.pages.p4':
    'Your own domain with an automatic certificate, or an address on sites.sytue.eu.',
  'product.pages.f1':
    'A frame-based visual editor with auto layout, reusable components and ready-made sections.',
  'product.pages.f2':
    'Three breakpoints (desktop, tablet and mobile), each with its own adjustments.',
  'product.pages.f3': 'Visitor analytics without cookies, inside the site dashboard.',
  'product.pages.f4':
    'Multilingual websites with per-language addresses, hreflang and translation assisted by Sytue AI.',
  'product.pages.f5':
    'Images from your computer, Sytue Drive or Sytue Photos, with alt text asked for as you insert them.',
  'product.pages.f6':
    'A shop with variants, stock, VAT per product and payment through Mollie, Adyen, Redsys or Stripe.',
  'product.pages.f7':
    'Orders with a simplified invoice issued by Sytue ERP, and withdrawal from the website itself.',
  'product.pages.f8':
    'Forms with consent, a message inbox and notices to people in your organisation.',
  'product.pages.f9':
    'Named versions, a preview behind a secret link and rollback to any earlier publication.',
  'product.pages.f10':
    'Code export to Astro, React, Next.js and native apps, and catalogue export to Shopify, WooCommerce and other platforms.',
  'product.bases.tagline':
    'Linked tables to keep customers, orders or stock, with permissions and a record of every change.',
  'product.bases.standard':
    'Open CSV and JSON; schema and relations enforced by the database itself',
  'product.bases.p1':
    'Customers, orders or stock in linked tables, with no database knowledge needed.',
  'product.bases.p2': 'Share with your organization at the role you choose.',
  'product.bases.p3': 'Every change is recorded: who made it and what it was before.',
  'product.bases.p4': 'Your data stays in the EU, and you can export it whenever you like.',
  'product.bases.f1':
    'Tables with text, number, date, checkbox, single-choice and multiple-choice fields.',
  'product.bases.f2':
    'Relations between tables: link an order to its customer, and the database refuses to break the link.',
  'product.bases.f3':
    'An editable grid driven by keyboard, readable by screen readers and usable on a phone.',
  'product.bases.f4': 'Permissions per base and per table: owner, editor, commenter and reader.',
  'product.bases.f5': 'Import a CSV file as a new table and export to CSV or JSON.',
  'product.bases.f6':
    'An audit trail of every change: who, which record and field, before and after.',
  'product.cloud.f12': 'Usage, costs and European billing in the same console.',
  'product.cloud.f11': 'Members, API tokens and an exportable audit log.',
  'product.cloud.f10': 'Privacy-friendly web analytics, without cookies.',
  'product.cloud.f9': 'Scheduled tasks and project templates.',
  'product.cloud.f8': 'A terminal into the container; restart or pause the service.',
  'product.cloud.f7': 'Live logs, metrics and monitoring with alerts.',
  'product.cloud.f6': 'Environment variables and secrets per environment.',
  'product.cloud.f5': 'Custom domains with automatic TLS; redirects and headers.',
  'product.cloud.f4': 'S3 storage and managed functions.',
  'product.cloud.f3': 'Postgres databases with an automatic REST API and a SQL editor.',
  'product.cloud.f2': 'A preview for every PR and rollback to any deployment in one click.',
  'product.cloud.f1':
    'Projects from your repository (GitHub, GitLab or your own Git) with a deployment on every push.',
  'product.console.f1':
    'A dashboard with the state of the organisation and what deserves a look today.',
  'product.console.f2':
    'Users: onboarding, suspension, password and MFA reset, sessions, aliases and mailbox.',
  'product.console.f3': 'Static groups and rule-based dynamic groups; roles with permissions.',
  'product.console.f4': 'Privileged access (PIM): time-boxed activation with approval.',
  'product.console.f5':
    'SSO with SAML or OIDC providers, with a connection test; SCIM provisioning.',
  'product.console.f6': 'Conditional access rules and an inventory of trusted devices.',
  'product.console.f7':
    'Integrations: OIDC clients, SAML and third-party connections; product substitution.',
  'product.console.f8': 'Bring content: migration from other providers.',
  'product.console.f9': 'Document editor and Rooms channels you can administer.',
  'product.console.f10': 'Governance: legal holds, sensitivity labels and password policy.',
  'product.console.f11': 'Audit log with CSV export and forwarding to your SIEM; service logs.',
  'product.console.f12': 'Your own domain and brand: organisation logo and mail domain.',
  'product.id.f1':
    'Single sign-on (OpenID Connect and OAuth 2.0 with PKCE) for the suite and your apps.',
  'product.id.f2': 'Passkeys (WebAuthn) and TOTP MFA with recovery codes.',
  'product.id.f3': 'App passwords for IMAP and clients that do not speak OIDC.',
  'product.id.f4': 'Sessions and devices: revoke one or all the others.',
  'product.id.f5': 'SAML federation with your IdP, and Sytue as a SAML IdP for other apps.',
  'product.id.f6': 'SCIM provisioning from your directory.',
  'product.id.f7': 'Invitations, email verification and password recovery.',
  'product.id.f8': 'A profile for iPhone and Mac (.mobileconfig) in one tap.',
  'product.id.f9': 'Password policies and access rules set by the organisation.',
  'product.id.f10': 'Dynamic client registration (DCR) and standard discovery.',
  'product.erp.f1':
    'Twenty-four modules and thirty-six models in a single service and a single database.',
  'product.erp.f2':
    'The model catalogue is data: each module declares its fields, its states and its list columns.',
  'product.erp.f3': 'A new module is one JSON file — no DDL, no migration and no table lock.',
  'product.erp.f4':
    'One API for all thirty-six models, and one interface generated from the catalogue.',
  'product.erp.f5':
    'Double-entry accounting with amounts in whole cents; not a single floating-point figure.',
  'product.erp.f6':
    'The database checks the balance when the transaction commits, not the application code.',
  'product.erp.f7':
    'Closed entries are immutable: they cannot be edited, deleted or given new lines.',
  'product.erp.f8': 'One document, one entry: a retry or two open tabs cannot duplicate revenue.',
  'product.erp.f9':
    'Shared labels ship in all 26 languages, and an untranslated field stops the service from starting.',
  'product.erp.f10':
    'Index-backed listings: the first page of twenty thousand rows is served in under a millisecond.',

  // --- Descargas ----------------------------------------------------------
  'meta.downloads.title': 'Downloads — Sytue One for desktop and mobile',
  'meta.downloads.description':
    'Native Sytue One apps for macOS, Windows, Linux, iOS and Android. The whole suite on your machine, with each system’s own integrations.',
  'downloads.overline': 'Apps',
  'downloads.h1': 'Sytue One on your machine',
  'downloads.intro':
    'Real desktop and mobile applications: they install, live in the Dock or the Start menu, alert you through system notifications and open files from your machine.',
  'downloads.desktop.title': 'Desktop',
  'downloads.desktop.body':
    'One native application per product, drawing its own interface on the graphics card: no browser inside, so it starts fast and uses resources like a system app.',
  'downloads.apps.title': 'Just what you need',
  'downloads.apps.body':
    'Sytue One brings the whole working day. If you only want the drive, Sytue Drive installs on its own.',
  'downloads.includes': 'Includes:',
  'downloads.mobile.title': 'Mobile',
  'downloads.mobile.body':
    'Native iOS and Android applications, with system notifications, sharing and access to files and camera.',
  'downloads.selfhost.title': 'Is your organisation on its own domain?',
  'downloads.selfhost.body':
    'The native applications connect to sytue.eu. If your organisation runs its own installation, sign in with the browser on its domain: the suite works the same on the web.',
  'downloads.yourSystem': 'Your system',
  'downloads.signInToGet': 'Sign in to download',
  'downloads.store': 'View on {format}',
  'downloads.soon': 'Coming soon on {format}',
  'downloads.browser.text':
    'Prefer the browser? The whole suite works the same on the web, with nothing to install:',
  'downloads.browser.link': 'open Sytue One',
  'platform.macos.req': 'macOS 11 or later · Apple Silicon and Intel',
  'platform.windows.req': 'Windows 10 or later · 64-bit',
  'platform.linux.req': 'OpenGL 2.1 · 64-bit',
  'platform.ios.req': 'iOS 14 or later',
  'platform.android.req': 'Android 6.0 or later',

  // --- Pie ----------------------------------------------------------------
  'footer.tagline':
    'Every app in the suite is our own service, built on open, published protocols.',
  'footer.products': 'Products',
  'footer.resources': 'Resources',
  'footer.legal': 'Legal',
  'footer.suite': 'The suite',
  'footer.downloads': 'Download the apps',
  'footer.sales': 'Talk to sales',
  'footer.privacy': 'Privacy & GDPR',
  'footer.accessibility': 'Accessibility',
  'footer.openSource': 'Open source',
  'footer.legalNotice': 'Legal notice',
  'footer.terms': 'Terms',
  'footer.copyright':
    '© {year} Sytue · Data hosted in the EU (Scaleway fr-par / nl-ams · on-premise Madrid).',
  'footer.privacyNotice':
    'We do not sell your data or share it with third parties. We do not use it to profile you, for advertising, or to train or improve our services or anyone else’s. It is encrypted in transit and at rest, and only leaves our European infrastructure where strictly needed to bill you, to notify you while the app is closed (the push service sees only your device and the time, never the content) and in the integrations you enable, where the third party will ask your approval for the data it will use.',

  // --- 404 ----------------------------------------------------------------
  'meta.notFound.title': 'Page not found — Sytue One',
  'notFound.overline': 'Error 404',
  'notFound.title': 'We could not find this page',
  'notFound.body': 'The link may have changed. Head back home or explore the suite.',
  'notFound.home': 'Back to home',

  // --- Legal: privacidad ---------------------------------------------------
  'meta.legal.title': 'Legal notice — Sytue One',
  'meta.legal.description':
    'Identification details of the owner of www.sytue.eu and of the Sytue One service: Sytue Projects, S.L.',
  'aviso.title': 'Legal notice',
  'aviso.intro':
    'In compliance with Article 10 of Spanish Law 34/2002 of 11 July on information society services and electronic commerce (LSSI), these are the identification details of the owner of this website and of the Sytue One service.',
  'aviso.review.body':
    'The registry details in this notice have been verified against the Companies Register. The rest of the text is under legal review and its wording may change without changing what it describes.',
  'aviso.owner.title': 'Owner',
  'aviso.owner.name': 'Company name',
  'aviso.owner.form': 'Legal form',
  'aviso.owner.formValue':
    'Single-member private limited company (sociedad limitada unipersonal) under Spanish law',
  'aviso.owner.nif': 'Tax ID (NIF)',
  'aviso.owner.address': 'Registered office',
  'aviso.owner.registry': 'Registry entry',
  'aviso.owner.registryValue':
    'Madrid Companies Register (Registro Mercantil de Madrid), section 8, sheet M-887153, entry 1',
  'aviso.owner.email': 'Email',
  'aviso.owner.web': 'Website',
  'aviso.scope.title': 'Purpose',
  'aviso.scope.body':
    'This website presents the Sytue One suite and gives access to its applications. Use of the service is governed by the Terms of Service, and the processing of personal data by the Privacy Policy, both linked in the footer of every page.',
  'aviso.ip.title': 'Intellectual and industrial property',
  'aviso.ip.body':
    'The contents of this site (texts, design, code, and the Sytue and Sytue One marks and logos) belong to Sytue Projects, S.L. or are used under licence. Third-party product names shown as compatibility references are trademarks of their owners.',
  'aviso.liability.title': 'Liability',
  'aviso.liability.body':
    'Sytue Projects, S.L. strives to keep this site available and up to date, without guaranteeing the absence of interruptions or errors. It is not responsible for the content of third-party sites it links to.',
  'aviso.law.title': 'Applicable law',
  'aviso.law.body':
    'This notice is governed by Spanish law. If you are a consumer, you keep the rights and jurisdiction granted to you by the law of your country of residence in the European Union.',
  'aviso.updated': 'Last updated: September 2026.',
  'meta.accessibility.title': 'Accessibility statement — Sytue One',
  'meta.accessibility.description':
    'How accessible the Sytue One suite and this website are, what we measure, what still falls short and how to tell us.',
  'a11y.title': 'Accessibility statement',
  'a11y.intro':
    'Sytue wants every person to be able to use Sytue One and this website, whatever assistive technology they rely on. This statement covers www.sytue.eu and the web applications of the suite.',
  'a11y.standard.title': 'Standard we work to',
  'a11y.standard.body':
    'We design and build against WCAG 2.1 level AA, the level referenced by the European standard EN 301 549 and by the European Accessibility Act (Directive (EU) 2019/882).',
  'a11y.status.title': 'Conformance status',
  'a11y.status.body':
    'Partially conformant: most of the suite meets the standard, and the parts listed below do not yet fully meet it. We have not commissioned an independent audit yet.',
  'a11y.measures.title': 'What we do to get there',
  'a11y.measures.design':
    'One design system for the whole suite: text colours checked for contrast, a visible focus ring on every control and keyboard operation of menus, dialogs and lists.',
  'a11y.measures.ci':
    'Automated accessibility audits (Lighthouse) run in the integration pipeline on every change to the public website.',
  'a11y.measures.system':
    'The interface follows your system settings: dark or light theme, reduced motion and text size, without any fixed pixel sizes on form fields.',
  'a11y.measures.text':
    'Every image, icon and empty state carries a text alternative, and every page declares its language so screen readers pronounce it correctly.',
  'a11y.limits.title': 'Known limitations',
  'a11y.limits.body':
    'The spreadsheet grid, the slide canvas and PDF annotation are complex editors that still need work to be fully operable with a keyboard and a screen reader. Documents and files uploaded by users are shown as they are and are not remediated by us.',
  'a11y.feedback.title': 'Tell us what is not working',
  'a11y.feedback.body':
    'If something in the suite or on this website is hard or impossible for you to use, write to us and we will look into it and get back to you:',
  'a11y.enforcement.title': 'If you are not satisfied',
  'a11y.enforcement.body':
    'If you have reported a barrier and are not satisfied with our answer, you can contact the authority responsible for accessibility in your country under the European Accessibility Act.',
  'a11y.updated':
    'This statement was written in September 2026 and is reviewed when the suite changes materially.',
  'meta.openSource.title': 'Open source — Sytue One',
  'meta.openSource.description':
    "Sytue builds almost everything from scratch, but doesn't reinvent what already works well. The open-source projects the suite relies on.",
  'oss.title': 'Open source',
  'oss.intro':
    "Sytue builds almost everything from scratch: mail, the editors, the calendar. But it doesn't reinvent what already works well. These are the open-source projects some pieces of the suite rely on.",
  'oss.livekit.body': 'Real-time audio and video for Sytue Rooms calls runs on LiveKit.',
  'oss.postgres.body':
    'Every service in the suite stores its data in PostgreSQL, one database per organisation.',
  'meta.privacy.title': 'Privacy & GDPR — Sytue One',
  'meta.privacy.description':
    'How Sytue One processes personal data: controller, data, legal bases, retention, processors, security, international transfers and your GDPR rights.',
  'legal.overline': 'Legal',
  'legal.updated': 'Last updated: July 2026.',
  'legal.review.title': 'Under legal review.',
  'privacy.title': 'Privacy & GDPR',
  'privacy.review.body':
    'This policy describes exactly how we process data today. The specific retention periods will be completed in the final version, reviewed by our legal team.',
  'privacy.controller.title': 'Data controller',
  'privacy.controller.body':
    'Sytue Projects, S.L. (Spanish tax ID B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Spain; registered with the Madrid Companies Register, section 8, sheet M-887153), as the provider of the Sytue One suite, is the controller for the data of the service’s accounts and organisations. Data protection contact:',
  'privacy.data.title': 'What data we process',
  'privacy.data.account.label': 'Account data:',
  'privacy.data.account.body':
    'name, email address and organisation, managed by Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Your organisation’s content:',
  'privacy.data.content.body':
    'mail, files, documents, calendar and messages that your organisation hosts in the suite. That content belongs to your organisation.',
  'privacy.data.technical.label': 'Technical operating data:',
  'privacy.data.technical.body':
    'access logs and connection data needed for security, abuse prevention and running the service.',
  'privacy.data.note':
    'We do not sell data or use it for advertising. Sytue’s managed infrastructure and storage are in the European Union (Scaleway, fr-par and nl-ams regions, and an on-premise zone in Madrid), with encryption in transit (TLS 1.3) and at rest. Connections to external services send the data you authorize to their provider, subject to that provider’s location and terms.',
  'privacy.basis.title': 'Legal basis',
  'privacy.basis.body':
    'Processing relies on the legal bases in Article 6 GDPR: performance of the service contract with your organisation, our legitimate interest in security and abuse prevention, compliance with legal obligations and, where expressly requested, your consent. The detail basis by basis, processing by processing, will be part of the final text.',
  'privacy.retention.title': 'Retention',
  'privacy.retention.body':
    'We keep account data and content for as long as your organisation keeps the service active. When an account or organisation is closed, the data is deleted or anonymised within a reasonable period, except for what we must keep by law (invoicing, for example). Technical operating logs are purged periodically. The specific periods per category are set out in the final version. Content reports are kept while they are open and deleted twelve months after they are resolved.',
  'privacy.processors.title': 'Processors and sub-processors',
  'privacy.processors.body':
    'Our infrastructure provider is Scaleway (fr-par and nl-ams regions), acting as a processor and operating exclusively within the European Union; we also operate an on-premise zone in Madrid (Spain) on our own premises. For subscription payments we rely on Mollie B.V. (Netherlands), which processes only the data needed for the payment. To send the text message with the code that verifies the mobile number when an account is created, we rely on Sweego (Mindbaz SAS, France), which receives only that number and the code. We use no sub-processors outside the EEA. The list of sub-processors will be published and kept up to date, with advance notice of any addition.',
  'privacy.security.title': 'Security',
  'privacy.security.body':
    'We apply technical and organisational measures in line with the GDPR: encryption in transit (TLS 1.3) and at rest, isolation between organisations (multi-tenant with row-level security), strong authentication (MFA/WebAuthn), access logging and least privilege. No system is infallible: should a breach affect you, we will notify you within the periods set by the Regulation.',
  'privacy.cookies.title': 'Cookies',
  'privacy.cookies.body':
    'We use only technical cookies required for single sign-on and session security. We use no advertising or third-party tracking cookies, so there is no consent wall for tracking.',
  'privacy.transfers.title': 'International transfers',
  'privacy.transfers.body':
    'Processing and hosting managed by Sytue take place in European Union regions. External connections enabled by the user send authorized data to their provider; its location depends on that provider and its configuration. Notifications sent while the app or browser is closed may also use external services. The device’s push service (Google, Mozilla or Apple, depending on your browser or phone) sees an anonymous identifier for your device and the time of each notification — never the content, which travels encrypted end to end, and on iOS is sent empty. You can turn notifications off, and then not even that leaves.',
  'privacy.rights.title': 'Your rights',
  'privacy.rights.body1':
    'The GDPR grants you the rights of access, rectification, erasure, objection, restriction of processing and portability. You can exercise them by writing to',
  // `body2` ends on its preposition: `authority` is the link text (the official
  // EDPB list) and the full stop is in the template. Do NOT name a single
  // authority here — the same page is read across the whole EEA.
  'privacy.rights.body2':
    '; we reply within the periods set by the Regulation. You also have the right to lodge a complaint with',
  'privacy.rights.authority': 'your supervisory authority',
  'privacy.dpo.title': 'Data protection officer (DPO)',
  'privacy.dpo.body': 'For any data protection query, or to reach our data protection officer:',
  'privacy.minors.title': 'Minors',
  'privacy.minors.body':
    'Sytue One is a service for organisations and their professional users; it is not aimed at minors and we do not knowingly collect data from minors.',
  'privacy.changes.title': 'Changes to this policy',
  'privacy.changes.body':
    'If we update this policy we will change the “last updated” date and, where the changes are material, we will tell you through the service channels before they take effect.',

  // --- Legal: términos -----------------------------------------------------
  'meta.terms.title': 'Terms of service — Sytue One',
  'meta.terms.description':
    'Terms of service for Sytue One: provider, scope, accounts and organisations, acceptable use, availability, your data, intellectual property and applicable law.',
  'terms.title': 'Terms of service',
  'terms.review.body':
    'These terms describe the service and its use exactly as it works today. The specific contractual clauses (service level, liability and jurisdiction) will be completed in the final version, reviewed by our legal team.',
  'terms.provider.title': 'Who provides the service',
  'terms.provider.body':
    'Sytue One is provided by Sytue Projects, S.L. (Spanish tax ID B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Spain; registered with the Madrid Companies Register, section 8, sheet M-887153) from infrastructure located exclusively in the European Union (Scaleway, fr-par and nl-ams regions, and an on-premise zone in Madrid), and is governed by EU law.',
  'terms.scope.title': 'Scope of the service',
  'terms.scope.body':
    'Sytue One is a productivity suite for organisations: identity (Sytue ID), mail, files, documents, calendar, contacts and messaging with video calls. The service is contracted per organisation.',
  'terms.accounts.title': 'Accounts and organisations',
  'terms.accounts.body':
    'Each organisation manages its users, groups and permissions through Sytue ID (SSO + MFA). The organisation’s administrators are responsible for adding and removing users and for the use their users make of the service.',
  'terms.use.title': 'Acceptable use',
  'terms.use.body':
    'You agree to use the service lawfully and not to use it for unlawful activity, spam, malware distribution, infringement of third-party rights or any use that compromises the security or availability of the service for other organisations.',
  'terms.availability.title': 'Availability and support',
  'terms.availability.body1':
    'We work to offer high availability and carry out maintenance and improvements continuously. Specific service level commitments (SLA), where they apply to your plan, are set out in the service agreement. Support is handled through',
  'terms.data.title': 'Your data',
  'terms.data.body1':
    'The content your organisation hosts in the suite belongs to your organisation. It is built on open standards (OIDC, JMAP, CalDAV, CardDAV, WebDAV), so your data stays portable and you can take it with you. The processing of personal data is described on the page',
  'terms.ip.title': 'Intellectual property',
  'terms.ip.body':
    'The Sytue One software is original work by Sytue and is protected by the corresponding intellectual property rights. You keep ownership of the content your organisation hosts in the suite; we only process it in order to provide the service.',
  'terms.liability.title': 'Liability',
  'terms.liability.body':
    'We put every effort into a reliable and secure service. The applicable warranties and limitations of liability are set out in the service agreement; nothing in these terms excludes liabilities that the law does not allow to be excluded.',
  'terms.contact.title': 'Contracting and contact',
  'terms.contact.body1':
    'For contracting terms, pricing per organisation or any question about the service, write to',
  'terms.changes.title': 'Changes',
  'terms.changes.body':
    'We may update these terms to reflect changes in the service or in the law. Where the changes are material, we will tell you through the service channels before they take effect, updating the “last updated” date.',
  'terms.law.title': 'Applicable law',
  'terms.law.body':
    'The service is governed by European Union law and by the law of the provider’s country. The specific jurisdiction for dispute resolution is set in the final text. Regardless of that, you keep your right to lodge a complaint with your data protection supervisory authority.',

  // --- Datos estructurados (JSON-LD) --------------------------------------
  'seo.org.description':
    'Sytue One: the sovereign European productivity suite. Identity, mail, files, documents, calendar, chat, video calls and a PaaS, 100% in the EU.',
  'seo.org.slogan': 'Your productivity, sovereign and European.',
  'seo.software.description':
    'Sovereign European productivity suite: mail (JMAP), files (WebDAV), collaborative documents, calendar (CalDAV), contacts (CardDAV), chat, video calls and a PaaS. SSO with Sytue ID.',
  'seo.software.f1': 'Single sign-on (SSO) + MFA/WebAuthn',
  'seo.software.f2': 'Mail, files, documents, calendar, contacts, chat and video calls',
  'seo.software.f3': 'Deployment and managed-data platform',
  'seo.software.f4': 'Data 100% in the EU (Scaleway)',
  'seo.offer.description': 'Free trial, no card required.',
  'seo.downloads.description':
    'Native Sytue One suite apps for desktop (macOS, Windows, Linux) and mobile (iOS, Android).',

  // --- Precios: cuenta personal ---
  'home.pricing.personal.title': 'Free',
  'home.pricing.personal.freeStorage': '5 GB, for one person',
  'home.pricing.personal.freeBody':
    'Mail, files, photos, documents, calendar, contacts, notes and rooms. No card and no time limit.',
  'home.pricing.personal.note':
    'Your organisation is your email: no organisation administration and no ERP, and everything else, including Sytue Admin for your integrations and migrations.',
  'home.pricing.personal.cta': 'Create a free account',
  'grid.orgOnly': 'Organisations only',
  'grid.enDesarrollo': 'In development',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 GB per user',
  'home.pricing.paidBody':
    'For organisations, and also for a single person. The whole suite, with Sytue ERP and Sytue Admin to run your organisation.',
  'home.pricing.thirdParty':
    'Every plan lets you connect your own storage: Google Drive, Microsoft OneDrive, an S3 bucket (Scaleway, OVH, Hetzner, Wasabi, MinIO), a WebDAV server such as Nextcloud or Internxt, or your NAS on your own premises —no open ports, through a Sytue tunnel—. Backups and recovery of the data stored in connected storage are the responsibility of the customer (you), and Sytue® is released from any liability for data loss in storage connected to Sytue®.',
  'home.pricing.soon': 'Coming soon',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '€25',
  'home.pricing.enterprise.storage': '1 TB per user',
  'home.pricing.enterprise.support': 'With Premium Support.',
  'home.pricing.education': 'Educational institutions: 50% off Pro.',
};
