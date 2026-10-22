/**
 * Los mensajes de error del SDK, en los cinco idiomas de Sytue.
 *
 * No es un adorno de internacionalización: «los errores dicen QUÉ hacer» ES la
 * función que este paquete vende, así que en un idioma que no entiendes deja de
 * existir. Un `relation "posts" does not exist` traducido a «no existe la tabla,
 * créala desde el panel» solo ayuda a quien lee ese idioma.
 *
 * Inglés es la base y el defecto; el idioma se resuelve del entorno (`SYTUE_LANG`
 * o `LANG`, que es lo que ya trae cualquier contenedor) y se puede fijar a mano
 * con `createClient({ language: 'fr' })`.
 */

export type Language =
  | 'en'
  | 'bg'
  | 'cs'
  | 'da'
  | 'de'
  | 'el'
  | 'es'
  | 'et'
  | 'fi'
  | 'fr'
  | 'ga'
  | 'hr'
  | 'hu'
  | 'is'
  | 'it'
  | 'lt'
  | 'lv'
  | 'mt'
  | 'nb'
  | 'nl'
  | 'pl'
  | 'pt'
  | 'ro'
  | 'sk'
  | 'sl'
  | 'sv';

/** Idiomas soportados, para validar sin repetir la lista. */
export const LANGUAGES: readonly Language[] = [
  'en',
  'bg',
  'cs',
  'da',
  'de',
  'el',
  'es',
  'et',
  'fi',
  'fr',
  'ga',
  'hr',
  'hu',
  'is',
  'it',
  'lt',
  'lv',
  'mt',
  'nb',
  'nl',
  'pl',
  'pt',
  'ro',
  'sk',
  'sl',
  'sv',
];

/** Claves de mensaje. Cada una es una situación con un siguiente paso distinto. */
export interface Messages {
  /** No hay servicio de base de datos conectado al proyecto. */
  noDatabase: string;
  /** No se pudo ni contactar con la API de datos. */
  unreachable: string;
  /** La tabla no existe. `{table}` */
  noTable: (table: string) => string;
  /** La columna no existe. `{table}` `{detail}` */
  noColumn: (table: string, detail: string) => string;
  /** Valor duplicado en una columna única. `{detail}` */
  duplicate: (detail: string) => string;
  /** Clave ajena rota en cualquiera de los dos sentidos. `{detail}` */
  foreignKey: (detail: string) => string;
  /** Falta un campo obligatorio. `{detail}` */
  required: (detail: string) => string;
  /** Un valor no encaja con el tipo de la columna. `{detail}` */
  badType: (detail: string) => string;
  /** 401/403: casi siempre una política RLS. `{table}` */
  denied: (table: string) => string;
  /** Cualquier otro error con status. `{status}` `{table}` */
  generic: (status: number, table: string) => string;
}

const en: Messages = {
  noDatabase:
    'This project has no database yet. Create one in the Sytue DevX dashboard ' +
    '(Services → Postgres) and deploy again: credentials are injected on deploy, not before.',
  unreachable:
    'Could not reach the database. If you are on your own machine, remember the credentials only ' +
    'exist inside Sytue DevX: use `createClient({ restUrl, restToken })`.',
  noTable: (t) =>
    `There is no table called “${t}”. Check the name or create it from the dashboard (Database → SQL).`,
  noColumn: (t, d) => `That column does not exist in “${t}”. ${d}`.trim(),
  duplicate: (d) =>
    `A row with that value already exists and the column does not allow repeats. ${d}`.trim(),
  foreignKey: (d) =>
    `That row points to one that does not exist (or other rows depend on it). ${d}`.trim(),
  required: (d) => `A required field is missing. ${d}`.trim(),
  badType: (d) => `A value does not match the column type. ${d}`.trim(),
  denied: (t) =>
    `No permission on “${t}”. This is usually a row-level security (RLS) policy hiding or blocking ` +
    `these rows; review it in the dashboard (Database → Tables).`,
  generic: (s, t) => `Error ${s} while querying “${t}”.`,
};

const es: Messages = {
  noDatabase:
    'Este proyecto todavía no tiene base de datos. Créala en el panel de Sytue DevX ' +
    '(Servicios → Postgres) y vuelve a desplegar: las credenciales se inyectan al desplegar, no antes.',
  unreachable:
    'No se pudo hablar con la base de datos. Si estás en tu ordenador, recuerda que las credenciales ' +
    'solo existen dentro de Sytue DevX: usa `createClient({ restUrl, restToken })`.',
  noTable: (t) =>
    `No existe la tabla «${t}». Revisa el nombre o créala desde el panel (Base de datos → SQL).`,
  noColumn: (t, d) => `Esa columna no existe en «${t}». ${d}`.trim(),
  duplicate: (d) => `Ya hay una fila con ese valor y la columna no admite repetidos. ${d}`.trim(),
  foreignKey: (d) =>
    `Esa fila apunta a otra que no existe (o hay filas que dependen de ella). ${d}`.trim(),
  required: (d) => `Falta un campo obligatorio. ${d}`.trim(),
  badType: (d) => `Un valor no tiene el tipo que espera la columna. ${d}`.trim(),
  denied: (t) =>
    `Sin permiso para «${t}». Suele ser una política de seguridad de fila (RLS) que no deja ver ni ` +
    `tocar estas filas; revísala en el panel (Base de datos → Tablas).`,
  generic: (s, t) => `Error ${s} al consultar «${t}».`,
};

const fr: Messages = {
  noDatabase:
    "Ce projet n'a pas encore de base de données. Créez-la dans le tableau de bord Sytue DevX " +
    '(Services → Postgres) puis redéployez : les identifiants sont injectés au déploiement, pas avant.',
  unreachable:
    'Impossible de joindre la base de données. Si vous êtes sur votre machine, rappelez-vous que les ' +
    "identifiants n'existent que dans Sytue DevX : utilisez `createClient({ restUrl, restToken })`.",
  noTable: (t) =>
    `La table « ${t} » n'existe pas. Vérifiez le nom ou créez-la depuis le tableau de bord (Base de données → SQL).`,
  noColumn: (t, d) => `Cette colonne n'existe pas dans « ${t} ». ${d}`.trim(),
  duplicate: (d) =>
    `Une ligne avec cette valeur existe déjà et la colonne n'accepte pas les doublons. ${d}`.trim(),
  foreignKey: (d) =>
    `Cette ligne pointe vers une autre qui n'existe pas (ou d'autres en dépendent). ${d}`.trim(),
  required: (d) => `Un champ obligatoire manque. ${d}`.trim(),
  badType: (d) => `Une valeur ne correspond pas au type de la colonne. ${d}`.trim(),
  denied: (t) =>
    `Pas d'autorisation sur « ${t} ». C'est presque toujours une politique de sécurité au niveau ligne ` +
    `(RLS) ; vérifiez-la dans le tableau de bord (Base de données → Tables).`,
  generic: (s, t) => `Erreur ${s} lors de la requête sur « ${t} ».`,
};

const de: Messages = {
  noDatabase:
    'Dieses Projekt hat noch keine Datenbank. Lege sie im Sytue-Cloud-Dashboard an ' +
    '(Dienste → Postgres) und deploye erneut: Zugangsdaten werden beim Deploy eingespielt, nicht vorher.',
  unreachable:
    'Die Datenbank war nicht erreichbar. Wenn du auf deinem eigenen Rechner bist: die Zugangsdaten ' +
    'existieren nur innerhalb von Sytue DevX — nutze `createClient({ restUrl, restToken })`.',
  noTable: (t) =>
    `Es gibt keine Tabelle „${t}“. Prüfe den Namen oder lege sie im Dashboard an (Datenbank → SQL).`,
  noColumn: (t, d) => `Diese Spalte gibt es in „${t}“ nicht. ${d}`.trim(),
  duplicate: (d) =>
    `Es gibt bereits eine Zeile mit diesem Wert, und die Spalte erlaubt keine Duplikate. ${d}`.trim(),
  foreignKey: (d) =>
    `Diese Zeile verweist auf eine, die es nicht gibt (oder andere hängen an ihr). ${d}`.trim(),
  required: (d) => `Ein Pflichtfeld fehlt. ${d}`.trim(),
  badType: (d) => `Ein Wert passt nicht zum Typ der Spalte. ${d}`.trim(),
  denied: (t) =>
    `Keine Berechtigung für „${t}“. Meist ist es eine Row-Level-Security-Richtlinie (RLS); prüfe sie ` +
    `im Dashboard (Datenbank → Tabellen).`,
  generic: (s, t) => `Fehler ${s} bei der Abfrage von „${t}“.`,
};

const it: Messages = {
  noDatabase:
    'Questo progetto non ha ancora un database. Crealo nel pannello di Sytue DevX ' +
    '(Servizi → Postgres) e ridistribuisci: le credenziali vengono iniettate al deploy, non prima.',
  unreachable:
    'Non è stato possibile raggiungere il database. Se sei sul tuo computer, ricorda che le credenziali ' +
    'esistono solo dentro Sytue DevX: usa `createClient({ restUrl, restToken })`.',
  noTable: (t) =>
    `Non esiste la tabella «${t}». Controlla il nome o creala dal pannello (Database → SQL).`,
  noColumn: (t, d) => `Quella colonna non esiste in «${t}». ${d}`.trim(),
  duplicate: (d) =>
    `Esiste già una riga con quel valore e la colonna non ammette duplicati. ${d}`.trim(),
  foreignKey: (d) =>
    `Quella riga punta a un'altra che non esiste (o altre dipendono da lei). ${d}`.trim(),
  required: (d) => `Manca un campo obbligatorio. ${d}`.trim(),
  badType: (d) => `Un valore non corrisponde al tipo della colonna. ${d}`.trim(),
  denied: (t) =>
    `Nessun permesso su «${t}». Di solito è una policy di sicurezza a livello di riga (RLS); ` +
    `controllala nel pannello (Database → Tabelle).`,
  generic: (s, t) => `Errore ${s} durante la query su «${t}».`,
};

const bg: Messages = {
  noDatabase:
    'Този проект все още няма база данни. Създайте я в таблото на Sytue DevX (Услуги → Postgres) и разположете отново: данните за достъп се вмъкват при разполагане, не преди това.',
  unreachable:
    'Базата данни не можа да бъде достигната. Ако работите на собствения си компютър, помнете, че данните за достъп съществуват само вътре в Sytue DevX: използвайте `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Няма таблица с име „${table}“. Проверете името или я създайте от таблото (База данни → SQL).`.trim(),
  noColumn: (table, detail) => `Тази колона не съществува в „${table}“. ${detail}`.trim(),
  duplicate: (detail) =>
    `Вече има ред с тази стойност, а колоната не допуска повторения. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Този ред сочи към несъществуващ ред (или други редове зависят от него). ${detail}`.trim(),
  required: (detail) => `Липсва задължително поле. ${detail}`.trim(),
  badType: (detail) => `Стойност не отговаря на типа на колоната. ${detail}`.trim(),
  denied: (table) =>
    `Няма права върху „${table}“. Обикновено това е политика за сигурност на ниво ред (RLS), която скрива или блокира тези редове; проверете я в таблото (База данни → Таблици).`.trim(),
  generic: (status, table) => `Грешка ${status} при заявка към „${table}“.`.trim(),
};

const cs: Messages = {
  noDatabase:
    'Tento projekt zatím nemá databázi. Vytvořte ji v panelu Sytue DevX (Služby → Postgres) a nasaďte znovu: přihlašovací údaje se vkládají při nasazení, ne dříve.',
  unreachable:
    'K databázi se nepodařilo připojit. Pokud pracujete na vlastním počítači, pamatujte, že přihlašovací údaje existují jen uvnitř Sytue DevX: použijte `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Tabulka „${table}“ neexistuje. Zkontrolujte název nebo ji vytvořte v panelu (Databáze → SQL).`.trim(),
  noColumn: (table, detail) => `Tento sloupec v tabulce „${table}“ neexistuje. ${detail}`.trim(),
  duplicate: (detail) =>
    `Řádek s touto hodnotou už existuje a sloupec nepovoluje duplicity. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Řádek odkazuje na řádek, který neexistuje (nebo na něm závisí jiné řádky). ${detail}`.trim(),
  required: (detail) => `Chybí povinné pole. ${detail}`.trim(),
  badType: (detail) => `Hodnota neodpovídá typu sloupce. ${detail}`.trim(),
  denied: (table) =>
    `Bez oprávnění k tabulce „${table}“. Obvykle jde o politiku zabezpečení na úrovni řádků (RLS), která tyto řádky skrývá nebo blokuje; zkontrolujte ji v panelu (Databáze → Tabulky).`.trim(),
  generic: (status, table) => `Chyba ${status} při dotazu na tabulku „${table}“.`.trim(),
};

const da: Messages = {
  noDatabase:
    'Projektet har endnu ingen database. Opret en i Sytue DevX-panelet (Tjenester → Postgres), og deploy igen: loginoplysningerne indsættes ved deploy, ikke før.',
  unreachable:
    'Databasen kunne ikke nås. Sidder du på din egen computer, så husk, at loginoplysningerne kun findes inde i Sytue DevX: brug `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Der findes ingen tabel ved navn “${table}”. Tjek navnet, eller opret den fra panelet (Database → SQL).`.trim(),
  noColumn: (table, detail) => `Den kolonne findes ikke i “${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Der findes allerede en række med den værdi, og kolonnen tillader ikke gentagelser. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Rækken peger på en række, der ikke findes (eller andre rækker afhænger af den). ${detail}`.trim(),
  required: (detail) => `Et påkrævet felt mangler. ${detail}`.trim(),
  badType: (detail) => `En værdi passer ikke til kolonnens type. ${detail}`.trim(),
  denied: (table) =>
    `Ingen tilladelse til “${table}”. Det er som regel en rækkesikkerhedspolitik (RLS), der skjuler eller blokerer disse rækker; tjek den i panelet (Database → Tabeller).`.trim(),
  generic: (status, table) => `Fejl ${status} ved forespørgsel på “${table}”.`.trim(),
};

const el: Messages = {
  noDatabase:
    'Αυτό το έργο δεν έχει ακόμη βάση δεδομένων. Δημιουργήστε μία στον πίνακα του Sytue DevX (Υπηρεσίες → Postgres) και αναπτύξτε ξανά: τα διαπιστευτήρια εισάγονται κατά την ανάπτυξη, όχι πριν.',
  unreachable:
    'Δεν ήταν δυνατή η επικοινωνία με τη βάση δεδομένων. Αν είστε στον δικό σας υπολογιστή, θυμηθείτε ότι τα διαπιστευτήρια υπάρχουν μόνο μέσα στο Sytue DevX: χρησιμοποιήστε `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Δεν υπάρχει πίνακας με το όνομα «${table}». Ελέγξτε το όνομα ή δημιουργήστε τον από τον πίνακα ελέγχου (Βάση δεδομένων → SQL).`.trim(),
  noColumn: (table, detail) => `Αυτή η στήλη δεν υπάρχει στον «${table}». ${detail}`.trim(),
  duplicate: (detail) =>
    `Υπάρχει ήδη γραμμή με αυτή την τιμή και η στήλη δεν επιτρέπει επαναλήψεις. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Αυτή η γραμμή δείχνει σε μία που δεν υπάρχει (ή άλλες γραμμές εξαρτώνται από αυτήν). ${detail}`.trim(),
  required: (detail) => `Λείπει ένα υποχρεωτικό πεδίο. ${detail}`.trim(),
  badType: (detail) => `Μια τιμή δεν ταιριάζει με τον τύπο της στήλης. ${detail}`.trim(),
  denied: (table) =>
    `Δεν υπάρχει δικαίωμα στον «${table}». Συνήθως πρόκειται για πολιτική ασφάλειας επιπέδου γραμμής (RLS) που κρύβει ή μπλοκάρει αυτές τις γραμμές· ελέγξτε την στον πίνακα ελέγχου (Βάση δεδομένων → Πίνακες).`.trim(),
  generic: (status, table) => `Σφάλμα ${status} κατά το ερώτημα στον «${table}».`.trim(),
};

const et: Messages = {
  noDatabase:
    'Sellel projektil pole veel andmebaasi. Looge see Sytue DevXi juhtpaneelil (Teenused → Postgres) ja juurutage uuesti: pääsuandmed lisatakse juurutamisel, mitte enne.',
  unreachable:
    'Andmebaasiga ei saanud ühendust. Kui töötate oma arvutis, pidage meeles, et pääsuandmed on olemas ainult Sytue DevXi sees: kasutage `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Tabelit nimega „${table}“ pole. Kontrollige nime või looge see juhtpaneelilt (Andmebaas → SQL).`.trim(),
  noColumn: (table, detail) => `Sellist veergu tabelis „${table}“ pole. ${detail}`.trim(),
  duplicate: (detail) =>
    `Sellise väärtusega rida on juba olemas ja veerg ei luba kordusi. ${detail}`.trim(),
  foreignKey: (detail) =>
    `See rida viitab reale, mida pole olemas (või teised read sõltuvad sellest). ${detail}`.trim(),
  required: (detail) => `Kohustuslik väli puudub. ${detail}`.trim(),
  badType: (detail) => `Väärtus ei vasta veeru tüübile. ${detail}`.trim(),
  denied: (table) =>
    `Tabelile „${table}“ pole õigusi. Tavaliselt on see reataseme turvapoliitika (RLS), mis neid ridu peidab või blokeerib; kontrollige seda juhtpaneelil (Andmebaas → Tabelid).`.trim(),
  generic: (status, table) => `Viga ${status} tabeli „${table}“ päringul.`.trim(),
};

const fi: Messages = {
  noDatabase:
    'Projektilla ei ole vielä tietokantaa. Luo se Sytue DevXin hallintapaneelissa (Palvelut → Postgres) ja julkaise uudelleen: tunnukset lisätään julkaisun yhteydessä, ei ennen sitä.',
  unreachable:
    'Tietokantaan ei saatu yhteyttä. Jos olet omalla koneellasi, muista että tunnukset ovat olemassa vain Sytue DevXin sisällä: käytä `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Taulua ”${table}” ei ole. Tarkista nimi tai luo taulu hallintapaneelista (Tietokanta → SQL).`.trim(),
  noColumn: (table, detail) => `Tätä saraketta ei ole taulussa ”${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Rivi tällä arvolla on jo olemassa, eikä sarake salli toistoja. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Rivi viittaa riviin, jota ei ole (tai muut rivit riippuvat siitä). ${detail}`.trim(),
  required: (detail) => `Pakollinen kenttä puuttuu. ${detail}`.trim(),
  badType: (detail) => `Arvo ei vastaa sarakkeen tyyppiä. ${detail}`.trim(),
  denied: (table) =>
    `Ei oikeuksia tauluun ”${table}”. Yleensä kyse on rivitason suojauskäytännöstä (RLS), joka piilottaa tai estää nämä rivit; tarkista se hallintapaneelista (Tietokanta → Taulut).`.trim(),
  generic: (status, table) => `Virhe ${status} kyselyssä tauluun ”${table}”.`.trim(),
};

const ga: Messages = {
  noDatabase:
    'Níl aon bhunachar sonraí ag an tionscadal seo fós. Cruthaigh ceann i bpainéal Sytue DevX (Seirbhísí → Postgres) agus imlonnaigh arís: insteallfar na dintiúir ag am imlonnaithe, ní roimhe sin.',
  unreachable:
    'Níorbh fhéidir teacht ar an mbunachar sonraí. Má tá tú ar do mheaisín féin, cuimhnigh nach ann do na dintiúir ach laistigh de Sytue DevX: úsáid `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Níl aon tábla darb ainm “${table}”. Seiceáil an t-ainm nó cruthaigh é ón bpainéal (Bunachar sonraí → SQL).`.trim(),
  noColumn: (table, detail) => `Níl an colún sin i “${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Tá ró leis an luach sin ann cheana agus ní cheadaíonn an colún athrá. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Díríonn an ró sin ar ró nach ann dó (nó tá rónna eile ag brath air). ${detail}`.trim(),
  required: (detail) => `Tá réimse riachtanach ar iarraidh. ${detail}`.trim(),
  badType: (detail) => `Ní mheaitseálann luach cineál an cholúin. ${detail}`.trim(),
  denied: (table) =>
    `Gan cead ar “${table}”. De ghnáth is polasaí slándála ar leibhéal an ró (RLS) é seo a cheileann nó a bhlocálann na rónna seo; athbhreithnigh é sa phainéal (Bunachar sonraí → Táblaí).`.trim(),
  generic: (status, table) => `Earráid ${status} agus “${table}” á cheistiú.`.trim(),
};

const hr: Messages = {
  noDatabase:
    'Ovaj projekt još nema bazu podataka. Stvorite je na nadzornoj ploči Sytue DevXa (Usluge → Postgres) i ponovno implementirajte: vjerodajnice se ubacuju pri implementaciji, ne prije.',
  unreachable:
    'Bazu podataka nije bilo moguće dosegnuti. Ako radite na vlastitom računalu, zapamtite da vjerodajnice postoje samo unutar Sytue DevXa: upotrijebite `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Ne postoji tablica pod nazivom „${table}”. Provjerite naziv ili je stvorite s nadzorne ploče (Baza podataka → SQL).`.trim(),
  noColumn: (table, detail) => `Taj stupac ne postoji u tablici „${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Redak s tom vrijednošću već postoji, a stupac ne dopušta ponavljanja. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Taj redak upućuje na redak koji ne postoji (ili drugi reci ovise o njemu). ${detail}`.trim(),
  required: (detail) => `Nedostaje obavezno polje. ${detail}`.trim(),
  badType: (detail) => `Vrijednost ne odgovara tipu stupca. ${detail}`.trim(),
  denied: (table) =>
    `Nema dozvole za „${table}”. Obično je riječ o pravilu sigurnosti na razini retka (RLS) koje skriva ili blokira te retke; provjerite ga na nadzornoj ploči (Baza podataka → Tablice).`.trim(),
  generic: (status, table) => `Pogreška ${status} pri upitu na „${table}”.`.trim(),
};

const hu: Messages = {
  noDatabase:
    'Ennek a projektnek még nincs adatbázisa. Hozzon létre egyet a Sytue DevX vezérlőpultján (Szolgáltatások → Postgres), és telepítse újra: a hitelesítő adatok a telepítéskor kerülnek be, nem előtte.',
  unreachable:
    'Az adatbázis nem érhető el. Ha a saját gépén dolgozik, ne feledje, hogy a hitelesítő adatok csak a Sytue DevXon belül léteznek: használja a `createClient({ restUrl, restToken })` hívást.',
  noTable: (table) =>
    `Nincs „${table}” nevű tábla. Ellenőrizze a nevet, vagy hozza létre a vezérlőpultról (Adatbázis → SQL).`.trim(),
  noColumn: (table, detail) =>
    `Ez az oszlop nem létezik a(z) „${table}” táblában. ${detail}`.trim(),
  duplicate: (detail) =>
    `Már létezik sor ezzel az értékkel, és az oszlop nem enged ismétlődést. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Ez a sor egy nem létező sorra mutat (vagy más sorok függenek tőle). ${detail}`.trim(),
  required: (detail) => `Hiányzik egy kötelező mező. ${detail}`.trim(),
  badType: (detail) => `Egy érték nem felel meg az oszlop típusának. ${detail}`.trim(),
  denied: (table) =>
    `Nincs jogosultság a(z) „${table}” táblához. Ez általában egy sorszintű biztonsági (RLS) szabály, amely elrejti vagy blokkolja ezeket a sorokat; ellenőrizze a vezérlőpulton (Adatbázis → Táblák).`.trim(),
  generic: (status, table) => `${status} hiba a(z) „${table}” lekérdezésekor.`.trim(),
};

const is: Messages = {
  noDatabase:
    'Þetta verkefni er ekki enn með gagnagrunn. Búðu hann til í stjórnborði Sytue DevX (Þjónustur → Postgres) og settu upp aftur: aðgangsupplýsingar eru settar inn við uppsetningu, ekki fyrr.',
  unreachable:
    'Ekki náðist samband við gagnagrunninn. Ef þú ert á eigin tölvu, mundu að aðgangsupplýsingarnar eru aðeins til innan Sytue DevX: notaðu `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Engin tafla heitir „${table}“. Athugaðu heitið eða búðu hana til í stjórnborðinu (Gagnagrunnur → SQL).`.trim(),
  noColumn: (table, detail) => `Þessi dálkur er ekki til í „${table}“. ${detail}`.trim(),
  duplicate: (detail) =>
    `Röð með þessu gildi er þegar til og dálkurinn leyfir ekki endurtekningar. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Þessi röð vísar í röð sem er ekki til (eða aðrar raðir eru háðar henni). ${detail}`.trim(),
  required: (detail) => `Nauðsynlegan reit vantar. ${detail}`.trim(),
  badType: (detail) => `Gildi passar ekki við tegund dálksins. ${detail}`.trim(),
  denied: (table) =>
    `Engin heimild fyrir „${table}“. Yfirleitt er þetta öryggisregla á raðastigi (RLS) sem felur eða hindrar þessar raðir; athugaðu hana í stjórnborðinu (Gagnagrunnur → Töflur).`.trim(),
  generic: (status, table) => `Villa ${status} við fyrirspurn í „${table}“.`.trim(),
};

const lt: Messages = {
  noDatabase:
    'Šis projektas dar neturi duomenų bazės. Sukurkite ją Sytue DevX valdymo skydelyje (Paslaugos → Postgres) ir diegkite iš naujo: prisijungimo duomenys įterpiami diegiant, ne anksčiau.',
  unreachable:
    'Nepavyko pasiekti duomenų bazės. Jei dirbate savo kompiuteryje, atminkite, kad prisijungimo duomenys egzistuoja tik Sytue DevX viduje: naudokite `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Lentelės „${table}“ nėra. Patikrinkite pavadinimą arba sukurkite ją valdymo skydelyje (Duomenų bazė → SQL).`.trim(),
  noColumn: (table, detail) => `Tokio stulpelio lentelėje „${table}“ nėra. ${detail}`.trim(),
  duplicate: (detail) =>
    `Eilutė su tokia reikšme jau yra, o stulpelis neleidžia pasikartojimų. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Ši eilutė nurodo į neegzistuojančią eilutę (arba nuo jos priklauso kitos eilutės). ${detail}`.trim(),
  required: (detail) => `Trūksta privalomo lauko. ${detail}`.trim(),
  badType: (detail) => `Reikšmė neatitinka stulpelio tipo. ${detail}`.trim(),
  denied: (table) =>
    `Nėra teisių į „${table}“. Dažniausiai tai eilučių lygio saugumo (RLS) taisyklė, slepianti ar blokuojanti šias eilutes; patikrinkite ją valdymo skydelyje (Duomenų bazė → Lentelės).`.trim(),
  generic: (status, table) => `Klaida ${status} vykdant užklausą į „${table}“.`.trim(),
};

const lv: Messages = {
  noDatabase:
    'Šim projektam vēl nav datubāzes. Izveidojiet to Sytue DevX panelī (Pakalpojumi → Postgres) un izvietojiet vēlreiz: piekļuves dati tiek ievietoti izvietošanas brīdī, ne agrāk.',
  unreachable:
    'Datubāzi neizdevās sasniegt. Ja strādājat savā datorā, atcerieties, ka piekļuves dati pastāv tikai Sytue DevX iekšienē: izmantojiet `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Tabula “${table}” nepastāv. Pārbaudiet nosaukumu vai izveidojiet to panelī (Datubāze → SQL).`.trim(),
  noColumn: (table, detail) => `Šādas kolonnas tabulā “${table}” nav. ${detail}`.trim(),
  duplicate: (detail) =>
    `Rinda ar šādu vērtību jau pastāv, un kolonna nepieļauj atkārtojumus. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Šī rinda norāda uz neesošu rindu (vai no tās ir atkarīgas citas rindas). ${detail}`.trim(),
  required: (detail) => `Trūkst obligāta lauka. ${detail}`.trim(),
  badType: (detail) => `Vērtība neatbilst kolonnas tipam. ${detail}`.trim(),
  denied: (table) =>
    `Nav atļaujas tabulai “${table}”. Parasti tā ir rindu līmeņa drošības (RLS) politika, kas slēpj vai bloķē šīs rindas; pārbaudiet to panelī (Datubāze → Tabulas).`.trim(),
  generic: (status, table) => `Kļūda ${status}, vaicājot tabulu “${table}”.`.trim(),
};

const mt: Messages = {
  noDatabase:
    "Dan il-proġett għad m'għandux database. Oħloq waħda fid-dashboard ta' Sytue DevX (Servizzi → Postgres) u erġa' iddeplojja: il-kredenzjali jiddaħħlu meta tiddeplojja, mhux qabel.",
  unreachable:
    'Id-database ma setgħetx tintlaħaq. Jekk qiegħed fuq il-kompjuter tiegħek, ftakar li l-kredenzjali jeżistu biss ġo Sytue DevX: uża `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `M'hemm l-ebda tabella jisimha “${table}”. Iċċekkja l-isem jew oħloqha mid-dashboard (Database → SQL).`.trim(),
  noColumn: (table, detail) => `Dik il-kolonna ma teżistix f'“${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Diġà hemm ringiela b'dak il-valur u l-kolonna ma tippermettix ripetizzjonijiet. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Dik ir-ringiela tipponta lejn waħda li ma teżistix (jew ringieli oħra jiddependu minnha). ${detail}`.trim(),
  required: (detail) => `Hemm qasam meħtieġ nieqes. ${detail}`.trim(),
  badType: (detail) => `Valur ma jaqbilx mat-tip tal-kolonna. ${detail}`.trim(),
  denied: (table) =>
    `L-ebda permess fuq “${table}”. Normalment din hi politika ta' sigurtà fil-livell tar-ringiela (RLS) li taħbi jew timblokka dawn ir-ringieli; irrevediha fid-dashboard (Database → Tabelli).`.trim(),
  generic: (status, table) =>
    `Żball ${status} waqt li kienet qed tiġi kkwerjata “${table}”.`.trim(),
};

const nb: Messages = {
  noDatabase:
    'Prosjektet har ingen database ennå. Opprett en i Sytue DevX-panelet (Tjenester → Postgres) og distribuer på nytt: påloggingsdetaljene settes inn ved distribusjon, ikke før.',
  unreachable:
    'Databasen kunne ikke nås. Sitter du på din egen maskin, husk at påloggingsdetaljene bare finnes inne i Sytue DevX: bruk `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Det finnes ingen tabell som heter «${table}». Sjekk navnet eller opprett den fra panelet (Database → SQL).`.trim(),
  noColumn: (table, detail) => `Den kolonnen finnes ikke i «${table}». ${detail}`.trim(),
  duplicate: (detail) =>
    `Det finnes allerede en rad med den verdien, og kolonnen tillater ikke duplikater. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Raden peker på en rad som ikke finnes (eller andre rader avhenger av den). ${detail}`.trim(),
  required: (detail) => `Et påkrevd felt mangler. ${detail}`.trim(),
  badType: (detail) => `En verdi passer ikke til kolonnens type. ${detail}`.trim(),
  denied: (table) =>
    `Ingen tilgang til «${table}». Det er som regel en radnivåsikkerhetspolicy (RLS) som skjuler eller blokkerer disse radene; sjekk den i panelet (Database → Tabeller).`.trim(),
  generic: (status, table) => `Feil ${status} ved spørring mot «${table}».`.trim(),
};

const nl: Messages = {
  noDatabase:
    'Dit project heeft nog geen database. Maak er een aan in het Sytue DevX-dashboard (Services → Postgres) en deploy opnieuw: de inloggegevens worden bij het deployen ingevoegd, niet eerder.',
  unreachable:
    'De database was niet bereikbaar. Werk je op je eigen computer, bedenk dan dat de inloggegevens alleen binnen Sytue DevX bestaan: gebruik `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Er is geen tabel met de naam “${table}”. Controleer de naam of maak hem aan vanuit het dashboard (Database → SQL).`.trim(),
  noColumn: (table, detail) => `Die kolom bestaat niet in “${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Er bestaat al een rij met die waarde en de kolom staat geen duplicaten toe. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Die rij verwijst naar een rij die niet bestaat (of andere rijen hangen ervan af). ${detail}`.trim(),
  required: (detail) => `Er ontbreekt een verplicht veld. ${detail}`.trim(),
  badType: (detail) => `Een waarde past niet bij het type van de kolom. ${detail}`.trim(),
  denied: (table) =>
    `Geen rechten op “${table}”. Meestal is dit een row-level-securitybeleid (RLS) dat deze rijen verbergt of blokkeert; controleer het in het dashboard (Database → Tabellen).`.trim(),
  generic: (status, table) => `Fout ${status} bij het bevragen van “${table}”.`.trim(),
};

const pl: Messages = {
  noDatabase:
    'Ten projekt nie ma jeszcze bazy danych. Utwórz ją w panelu Sytue DevX (Usługi → Postgres) i wdróż ponownie: dane dostępowe są wstrzykiwane przy wdrożeniu, nie wcześniej.',
  unreachable:
    'Nie udało się połączyć z bazą danych. Jeśli pracujesz na własnym komputerze, pamiętaj, że dane dostępowe istnieją tylko wewnątrz Sytue DevX: użyj `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Nie ma tabeli „${table}”. Sprawdź nazwę lub utwórz ją w panelu (Baza danych → SQL).`.trim(),
  noColumn: (table, detail) => `Ta kolumna nie istnieje w „${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Wiersz z tą wartością już istnieje, a kolumna nie dopuszcza duplikatów. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Ten wiersz wskazuje na wiersz, który nie istnieje (albo inne wiersze od niego zależą). ${detail}`.trim(),
  required: (detail) => `Brakuje wymaganego pola. ${detail}`.trim(),
  badType: (detail) => `Wartość nie pasuje do typu kolumny. ${detail}`.trim(),
  denied: (table) =>
    `Brak uprawnień do „${table}”. Zwykle to polityka bezpieczeństwa na poziomie wierszy (RLS), która ukrywa lub blokuje te wiersze; sprawdź ją w panelu (Baza danych → Tabele).`.trim(),
  generic: (status, table) => `Błąd ${status} podczas odpytywania „${table}”.`.trim(),
};

const pt: Messages = {
  noDatabase:
    'Este projeto ainda não tem base de dados. Crie-a no painel do Sytue DevX (Serviços → Postgres) e volte a implementar: as credenciais são injetadas ao implementar, não antes.',
  unreachable:
    'Não foi possível contactar a base de dados. Se está no seu computador, lembre-se de que as credenciais só existem dentro do Sytue DevX: use `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Não existe a tabela «${table}». Reveja o nome ou crie-a a partir do painel (Base de dados → SQL).`.trim(),
  noColumn: (table, detail) => `Essa coluna não existe em «${table}». ${detail}`.trim(),
  duplicate: (detail) =>
    `Já existe uma linha com esse valor e a coluna não admite repetidos. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Essa linha aponta para outra que não existe (ou há linhas que dependem dela). ${detail}`.trim(),
  required: (detail) => `Falta um campo obrigatório. ${detail}`.trim(),
  badType: (detail) => `Um valor não tem o tipo que a coluna espera. ${detail}`.trim(),
  denied: (table) =>
    `Sem permissão para «${table}». Costuma ser uma política de segurança ao nível da linha (RLS) que impede ver ou alterar estas linhas; reveja-a no painel (Base de dados → Tabelas).`.trim(),
  generic: (status, table) => `Erro ${status} ao consultar «${table}».`.trim(),
};

const ro: Messages = {
  noDatabase:
    'Acest proiect nu are încă o bază de date. Creați una în panoul Sytue DevX (Servicii → Postgres) și redistribuiți: datele de acces se injectează la distribuire, nu înainte.',
  unreachable:
    'Baza de date nu a putut fi contactată. Dacă lucrați pe propriul calculator, țineți minte că datele de acces există doar în interiorul Sytue DevX: folosiți `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Nu există niciun tabel numit „${table}”. Verificați numele sau creați-l din panou (Bază de date → SQL).`.trim(),
  noColumn: (table, detail) => `Această coloană nu există în „${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Există deja un rând cu această valoare, iar coloana nu permite duplicate. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Acest rând indică spre unul care nu există (sau alte rânduri depind de el). ${detail}`.trim(),
  required: (detail) => `Lipsește un câmp obligatoriu. ${detail}`.trim(),
  badType: (detail) => `O valoare nu corespunde tipului coloanei. ${detail}`.trim(),
  denied: (table) =>
    `Fără permisiune pe „${table}”. De obicei este o politică de securitate la nivel de rând (RLS) care ascunde sau blochează aceste rânduri; verificați-o în panou (Bază de date → Tabele).`.trim(),
  generic: (status, table) => `Eroare ${status} la interogarea „${table}”.`.trim(),
};

const sk: Messages = {
  noDatabase:
    'Tento projekt zatiaľ nemá databázu. Vytvorte ju v paneli Sytue DevX (Služby → Postgres) a nasaďte znova: prihlasovacie údaje sa vkladajú pri nasadení, nie skôr.',
  unreachable:
    'K databáze sa nepodarilo pripojiť. Ak pracujete na vlastnom počítači, pamätajte, že prihlasovacie údaje existujú len vnútri Sytue DevX: použite `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Tabuľka „${table}“ neexistuje. Skontrolujte názov alebo ju vytvorte v paneli (Databáza → SQL).`.trim(),
  noColumn: (table, detail) => `Tento stĺpec v tabuľke „${table}“ neexistuje. ${detail}`.trim(),
  duplicate: (detail) =>
    `Riadok s touto hodnotou už existuje a stĺpec nepovoľuje duplicity. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Riadok odkazuje na riadok, ktorý neexistuje (alebo od neho závisia iné riadky). ${detail}`.trim(),
  required: (detail) => `Chýba povinné pole. ${detail}`.trim(),
  badType: (detail) => `Hodnota nezodpovedá typu stĺpca. ${detail}`.trim(),
  denied: (table) =>
    `Bez oprávnenia k tabuľke „${table}“. Zvyčajne ide o politiku zabezpečenia na úrovni riadkov (RLS), ktorá tieto riadky skrýva alebo blokuje; skontrolujte ju v paneli (Databáza → Tabuľky).`.trim(),
  generic: (status, table) => `Chyba ${status} pri dopyte na tabuľku „${table}“.`.trim(),
};

const sl: Messages = {
  noDatabase:
    'Ta projekt še nima podatkovne baze. Ustvarite jo na nadzorni plošči Sytue DevX (Storitve → Postgres) in znova uvedite: poverilnice se vstavijo ob uvedbi, ne prej.',
  unreachable:
    'Podatkovne baze ni bilo mogoče doseči. Če delate na svojem računalniku, ne pozabite, da poverilnice obstajajo samo znotraj Sytue DevX: uporabite `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Tabela z imenom »${table}« ne obstaja. Preverite ime ali jo ustvarite z nadzorne plošče (Podatkovna baza → SQL).`.trim(),
  noColumn: (table, detail) => `Ta stolpec v tabeli »${table}« ne obstaja. ${detail}`.trim(),
  duplicate: (detail) =>
    `Vrstica s to vrednostjo že obstaja in stolpec ne dovoljuje ponovitev. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Ta vrstica kaže na vrstico, ki ne obstaja (ali pa so od nje odvisne druge vrstice). ${detail}`.trim(),
  required: (detail) => `Manjka obvezno polje. ${detail}`.trim(),
  badType: (detail) => `Vrednost se ne ujema s tipom stolpca. ${detail}`.trim(),
  denied: (table) =>
    `Ni dovoljenja za »${table}«. Običajno gre za varnostni pravilnik na ravni vrstic (RLS), ki te vrstice skriva ali blokira; preverite ga na nadzorni plošči (Podatkovna baza → Tabele).`.trim(),
  generic: (status, table) => `Napaka ${status} pri poizvedbi na »${table}«.`.trim(),
};

const sv: Messages = {
  noDatabase:
    'Projektet har ingen databas ännu. Skapa en i Sytue DevXs kontrollpanel (Tjänster → Postgres) och driftsätt igen: inloggningsuppgifterna läggs in vid driftsättningen, inte innan.',
  unreachable:
    'Databasen kunde inte nås. Om du sitter på din egen dator: kom ihåg att inloggningsuppgifterna bara finns inuti Sytue DevX, använd `createClient({ restUrl, restToken })`.',
  noTable: (table) =>
    `Det finns ingen tabell som heter ”${table}”. Kontrollera namnet eller skapa den från kontrollpanelen (Databas → SQL).`.trim(),
  noColumn: (table, detail) => `Den kolumnen finns inte i ”${table}”. ${detail}`.trim(),
  duplicate: (detail) =>
    `Det finns redan en rad med det värdet och kolumnen tillåter inte dubbletter. ${detail}`.trim(),
  foreignKey: (detail) =>
    `Raden pekar på en rad som inte finns (eller andra rader är beroende av den). ${detail}`.trim(),
  required: (detail) => `Ett obligatoriskt fält saknas. ${detail}`.trim(),
  badType: (detail) => `Ett värde matchar inte kolumnens typ. ${detail}`.trim(),
  denied: (table) =>
    `Ingen behörighet till ”${table}”. Det är oftast en radnivåsäkerhetspolicy (RLS) som döljer eller blockerar raderna; kontrollera den i kontrollpanelen (Databas → Tabeller).`.trim(),
  generic: (status, table) => `Fel ${status} vid fråga mot ”${table}”.`.trim(),
};

const CATALOGS: Record<Language, Messages> = {
  en,
  bg,
  cs,
  da,
  de,
  el,
  es,
  et,
  fi,
  fr,
  ga,
  hr,
  hu,
  is,
  it,
  lt,
  lv,
  mt,
  nb,
  nl,
  pl,
  pt,
  ro,
  sk,
  sl,
  sv,
};

/** Catálogo de un idioma; inglés si no se reconoce. */
export function messages(language?: string): Messages {
  return CATALOGS[normalizeLanguage(language)];
}

/**
 * Normaliza lo que venga —`es`, `es-ES`, `fr_FR.UTF-8`, `C`— a un idioma
 * soportado. Cualquier cosa que no lo sea cae en inglés, que es la base.
 */
export function normalizeLanguage(raw?: string): Language {
  const code = (raw ?? '').trim().toLowerCase().replace('_', '-').split(/[-.]/)[0] ?? '';
  return (LANGUAGES as readonly string[]).includes(code) ? (code as Language) : 'en';
}
