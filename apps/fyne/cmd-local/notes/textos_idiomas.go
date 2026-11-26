// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Las traducciones de `textos` (main.go). Un texto nuevo o cambiado va en los 26 idiomas:
// TestLaTablaDeNotepadsCuadra exige las mismas claves en todos. El trato y «cuaderno»
// son los del catálogo de Notepads en cada idioma.

package main

import "git.sytue.eu/sytue/one/apps/nucleo/i18n"

// traducciones son los textos de `textos` en los idiomas que no son el español ni el
// inglés, traducidos del inglés.
var traducciones = map[i18n.Locale]map[string]string{
	"bg": {
		"sin.calendar":    "Sytue Calendar от Sytue Office не е инсталирано на това устройство.",
		"volcado.cifrado": "Това копие е от криптирана тетрадка, а Sytue Office не отваря криптирани тетрадки. Декриптирайте я в приложението, в което сте я създали, и я експортирайте отново.",
		"imagen.anadir":   "Добавяне на изображение от това устройство…",
		"imagen.elegir":   "Изберете изображение за бележката",
		"imagen.noLeible": "Това не може да се прочете като изображение или е твърде голямо за бележка. Използвайте PNG, JPEG, GIF, WebP, AVIF, TIFF или HEIC.",
	},
	"cs": {
		"sin.calendar":    "Sytue Calendar ze Sytue Office není v tomto zařízení nainstalován.",
		"volcado.cifrado": "Tato záloha pochází ze šifrovaného sešitu a Sytue Office šifrované sešity neotevírá. Dešifrujte ho v aplikaci, ve které jste ho vytvořili, a znovu ho exportujte.",
		"imagen.anadir":   "Přidat obrázek z tohoto zařízení…",
		"imagen.elegir":   "Vyberte obrázek pro poznámku",
		"imagen.noLeible": "Nelze to přečíst jako obrázek, nebo je to na poznámku příliš velké. Použijte PNG, JPEG, GIF, WebP, AVIF, TIFF nebo HEIC.",
	},
	"da": {
		"sin.calendar":    "Sytue Calendar fra Sytue Office er ikke installeret på denne enhed.",
		"volcado.cifrado": "Denne sikkerhedskopi er fra en krypteret notesbog, og Sytue Office åbner ikke krypterede notesbøger. Dekryptér den i den app, hvor du oprettede den, og eksportér den igen.",
		"imagen.anadir":   "Tilføj et billede fra denne enhed…",
		"imagen.elegir":   "Vælg et billede til noten",
		"imagen.noLeible": "Det kan ikke læses som et billede, eller det er for stort til en note. Brug PNG, JPEG, GIF, WebP, AVIF, TIFF eller HEIC.",
	},
	"de": {
		"sin.calendar":    "Sytue Calendar von Sytue Office ist auf diesem Gerät nicht installiert.",
		"volcado.cifrado": "Diese Sicherung stammt aus einem verschlüsselten Notizbuch, und Sytue Office öffnet keine verschlüsselten Notizbücher. Entschlüsseln Sie es in der App, in der Sie es erstellt haben, und exportieren Sie es erneut.",
		"imagen.anadir":   "Bild von diesem Gerät hinzufügen…",
		"imagen.elegir":   "Wählen Sie ein Bild für die Notiz",
		"imagen.noLeible": "Das lässt sich nicht als Bild lesen oder ist für eine Notiz zu groß. Verwenden Sie PNG, JPEG, GIF, WebP, AVIF, TIFF oder HEIC.",
	},
	"el": {
		"sin.calendar":    "Το Sytue Calendar του Sytue Office δεν είναι εγκατεστημένο σε αυτή τη συσκευή.",
		"volcado.cifrado": "Αυτό το αντίγραφο είναι από κρυπτογραφημένο σημειωματάριο και το Sytue Office δεν ανοίγει κρυπτογραφημένα σημειωματάρια. Αποκρυπτογραφήστε το στην εφαρμογή όπου το δημιουργήσατε και εξαγάγετέ το ξανά.",
		"imagen.anadir":   "Προσθήκη εικόνας από αυτή τη συσκευή…",
		"imagen.elegir":   "Επιλέξτε μια εικόνα για τη σημείωση",
		"imagen.noLeible": "Δεν μπορεί να διαβαστεί ως εικόνα ή είναι πολύ μεγάλο για σημείωση. Χρησιμοποιήστε PNG, JPEG, GIF, WebP, AVIF, TIFF ή HEIC.",
	},
	"et": {
		"sin.calendar":    "Sytue Office'i rakendus Sytue Calendar pole selles seadmes installitud.",
		"volcado.cifrado": "See varukoopia on krüpteeritud märkmikust ja Sytue Office ei ava krüpteeritud märkmikke. Dekrüpteerige see rakenduses, kus selle lõite, ja eksportige uuesti.",
		"imagen.anadir":   "Lisa pilt sellest seadmest…",
		"imagen.elegir":   "Valige märkme jaoks pilt",
		"imagen.noLeible": "Seda ei saa pildina lugeda või on see märkme jaoks liiga suur. Kasutage PNG-, JPEG-, GIF-, WebP-, AVIF-, TIFF- või HEIC-vormingut.",
	},
	"fi": {
		"sin.calendar":    "Sytue Officen Sytue Calendar -sovellusta ei ole asennettu tälle laitteelle.",
		"volcado.cifrado": "Tämä varmuuskopio on salatusta muistikirjasta, eikä Sytue Office avaa salattuja muistikirjoja. Pura salaus sovelluksessa, jossa loit sen, ja vie se uudelleen.",
		"imagen.anadir":   "Lisää kuva tältä laitteelta…",
		"imagen.elegir":   "Valitse kuva muistiinpanoon",
		"imagen.noLeible": "Tätä ei voi lukea kuvana, tai se on liian suuri muistiinpanoon. Käytä PNG-, JPEG-, GIF-, WebP-, AVIF-, TIFF- tai HEIC-muotoa.",
	},
	"fr": {
		"sin.calendar":    "Sytue Calendar de Sytue Office n’est pas installée sur cet appareil.",
		"volcado.cifrado": "Cette sauvegarde provient d’un carnet chiffré, et Sytue Office n’ouvre pas les carnets chiffrés. Déchiffrez-le dans l’app où vous l’avez créé, puis exportez-le à nouveau.",
		"imagen.anadir":   "Ajouter une image de cet appareil…",
		"imagen.elegir":   "Choisissez une image pour la note",
		"imagen.noLeible": "Impossible de le lire comme une image, ou il est trop volumineux pour une note. Utilisez PNG, JPEG, GIF, WebP, AVIF, TIFF ou HEIC.",
	},
	"ga": {
		"sin.calendar":    "Níl Sytue Calendar de chuid Sytue Office suiteáilte ar an ngléas seo.",
		"volcado.cifrado": "Is ó leabhar nótaí criptithe an cúltaca seo, agus ní osclaíonn Sytue Office leabhair nótaí chriptithe. Díchriptigh é san aip inar chruthaigh tú é agus easpórtáil arís é.",
		"imagen.anadir":   "Cuir íomhá ón ngléas seo leis…",
		"imagen.elegir":   "Roghnaigh íomhá don nóta",
		"imagen.noLeible": "Ní féidir é a léamh mar íomhá, nó tá sé rómhór do nóta. Úsáid PNG, JPEG, GIF, WebP, AVIF, TIFF nó HEIC.",
	},
	"hr": {
		"sin.calendar":    "Aplikacija Sytue Calendar iz paketa Sytue Office nije instalirana na ovom uređaju.",
		"volcado.cifrado": "Ova je kopija iz šifrirane bilježnice, a Sytue Office ne otvara šifrirane bilježnice. Dešifrirajte je u aplikaciji u kojoj ste je izradili i ponovno je izvezite.",
		"imagen.anadir":   "Dodaj sliku s ovog uređaja…",
		"imagen.elegir":   "Odaberite sliku za bilješku",
		"imagen.noLeible": "Ne može se pročitati kao slika ili je preveliko za bilješku. Upotrijebite PNG, JPEG, GIF, WebP, AVIF, TIFF ili HEIC.",
	},
	"hu": {
		"sin.calendar":    "A Sytue Office Sytue Calendar alkalmazása nincs telepítve ezen az eszközön.",
		"volcado.cifrado": "Ez a mentés egy titkosított jegyzetfüzetből származik, a Sytue Office pedig nem nyit meg titkosított jegyzetfüzeteket. Oldja fel a titkosítást abban az alkalmazásban, amelyben létrehozta, majd exportálja újra.",
		"imagen.anadir":   "Kép hozzáadása erről az eszközről…",
		"imagen.elegir":   "Válasszon képet a jegyzethez",
		"imagen.noLeible": "Nem olvasható képként, vagy túl nagy egy jegyzethez. Használjon PNG, JPEG, GIF, WebP, AVIF, TIFF vagy HEIC formátumot.",
	},
	"is": {
		"sin.calendar":    "Sytue Calendar úr Sytue Office er ekki uppsett á þessu tæki.",
		"volcado.cifrado": "Þetta afrit er úr dulkóðaðri minnisbók og Sytue Office opnar ekki dulkóðaðar minnisbækur. Afkóðaðu hana í forritinu þar sem þú bjóst hana til og fluttu hana aftur út.",
		"imagen.anadir":   "Bæta við mynd úr þessu tæki…",
		"imagen.elegir":   "Veldu mynd fyrir minnispunktinn",
		"imagen.noLeible": "Ekki er hægt að lesa þetta sem mynd, eða það er of stórt fyrir minnispunkt. Notaðu PNG, JPEG, GIF, WebP, AVIF, TIFF eða HEIC.",
	},
	"it": {
		"sin.calendar":    "Sytue Calendar di Sytue Office non è installata su questo dispositivo.",
		"volcado.cifrado": "Questo backup proviene da un taccuino cifrato e Sytue Office non apre taccuini cifrati. Decifralo nell’app in cui l’hai creato ed esportalo di nuovo.",
		"imagen.anadir":   "Aggiungi un’immagine da questo dispositivo…",
		"imagen.elegir":   "Scegli un’immagine per la nota",
		"imagen.noLeible": "Non si può leggere come immagine, oppure è troppo grande per una nota. Usa PNG, JPEG, GIF, WebP, AVIF, TIFF o HEIC.",
	},
	"lt": {
		"sin.calendar":    "Sytue Office programa Sytue Calendar šiame įrenginyje neįdiegta.",
		"volcado.cifrado": "Ši atsarginė kopija yra iš šifruotos užrašų knygelės, o Sytue Office neatidaro šifruotų užrašų knygelių. Iššifruokite ją programoje, kurioje ją sukūrėte, ir eksportuokite dar kartą.",
		"imagen.anadir":   "Pridėti paveikslėlį iš šio įrenginio…",
		"imagen.elegir":   "Pasirinkite paveikslėlį užrašui",
		"imagen.noLeible": "Jo negalima perskaityti kaip paveikslėlio arba jis per didelis užrašui. Naudokite PNG, JPEG, GIF, WebP, AVIF, TIFF arba HEIC.",
	},
	"lv": {
		"sin.calendar":    "Sytue Office lietotne Sytue Calendar šajā ierīcē nav instalēta.",
		"volcado.cifrado": "Šī rezerves kopija ir no šifrētas piezīmju grāmatas, un Sytue Office neatver šifrētas piezīmju grāmatas. Atšifrējiet to lietotnē, kurā to izveidojāt, un eksportējiet vēlreiz.",
		"imagen.anadir":   "Pievienot attēlu no šīs ierīces…",
		"imagen.elegir":   "Izvēlieties attēlu piezīmei",
		"imagen.noLeible": "To nevar nolasīt kā attēlu, vai tas ir pārāk liels piezīmei. Izmantojiet PNG, JPEG, GIF, WebP, AVIF, TIFF vai HEIC.",
	},
	"mt": {
		"sin.calendar":    "Sytue Calendar ta’ Sytue Office mhix installata fuq dan it-tagħmir.",
		"volcado.cifrado": "Din il-kopja hija minn ktieb tan-noti kriptat, u Sytue Office ma jiftaħx kotba tan-noti kriptati. Iddekriptah fl-app fejn ħloqtu u erġa’ esportah.",
		"imagen.anadir":   "Żid immaġni minn dan it-tagħmir…",
		"imagen.elegir":   "Agħżel immaġni għan-nota",
		"imagen.noLeible": "Ma jistax jinqara bħala immaġni, jew huwa kbir wisq għal nota. Uża PNG, JPEG, GIF, WebP, AVIF, TIFF jew HEIC.",
	},
	"nb": {
		"sin.calendar":    "Sytue Calendar fra Sytue Office er ikke installert på denne enheten.",
		"volcado.cifrado": "Denne sikkerhetskopien er fra en kryptert notatbok, og Sytue Office åpner ikke krypterte notatbøker. Dekrypter den i appen der du opprettet den, og eksporter den på nytt.",
		"imagen.anadir":   "Legg til et bilde fra denne enheten…",
		"imagen.elegir":   "Velg et bilde til notatet",
		"imagen.noLeible": "Dette kan ikke leses som et bilde, eller det er for stort for et notat. Bruk PNG, JPEG, GIF, WebP, AVIF, TIFF eller HEIC.",
	},
	"nl": {
		"sin.calendar":    "Sytue Calendar van Sytue Office is niet op dit apparaat geïnstalleerd.",
		"volcado.cifrado": "Deze back-up komt uit een versleuteld notitieboek, en Sytue Office opent geen versleutelde notitieboeken. Ontsleutel het in de app waarin je het hebt gemaakt en exporteer het opnieuw.",
		"imagen.anadir":   "Afbeelding van dit apparaat toevoegen…",
		"imagen.elegir":   "Kies een afbeelding voor de notitie",
		"imagen.noLeible": "Dit kan niet als afbeelding worden gelezen, of het is te groot voor een notitie. Gebruik PNG, JPEG, GIF, WebP, AVIF, TIFF of HEIC.",
	},
	"pl": {
		"sin.calendar":    "Sytue Calendar z Sytue Office nie jest zainstalowany na tym urządzeniu.",
		"volcado.cifrado": "Ta kopia pochodzi z zaszyfrowanego notesu, a Sytue Office nie otwiera zaszyfrowanych notesów. Odszyfruj go w aplikacji, w której go utworzono, i wyeksportuj ponownie.",
		"imagen.anadir":   "Dodaj obraz z tego urządzenia…",
		"imagen.elegir":   "Wybierz obraz do notatki",
		"imagen.noLeible": "Nie można tego odczytać jako obrazu albo jest za duże na notatkę. Użyj formatu PNG, JPEG, GIF, WebP, AVIF, TIFF lub HEIC.",
	},
	"pt": {
		"sin.calendar":    "O Sytue Calendar do Sytue Office não está instalado neste dispositivo.",
		"volcado.cifrado": "Esta cópia é de um caderno cifrado, e o Sytue Office não abre cadernos cifrados. Decifre-o na app onde o criou e exporte-o de novo.",
		"imagen.anadir":   "Adicionar uma imagem deste dispositivo…",
		"imagen.elegir":   "Escolha uma imagem para a nota",
		"imagen.noLeible": "Não é possível lê-lo como imagem, ou é demasiado grande para uma nota. Use PNG, JPEG, GIF, WebP, AVIF, TIFF ou HEIC.",
	},
	"ro": {
		"sin.calendar":    "Sytue Calendar din Sytue Office nu este instalat pe acest dispozitiv.",
		"volcado.cifrado": "Această copie provine dintr-un carnet criptat, iar Sytue Office nu deschide carnete criptate. Decriptați-l în aplicația în care l-ați creat și exportați-l din nou.",
		"imagen.anadir":   "Adăugați o imagine de pe acest dispozitiv…",
		"imagen.elegir":   "Alegeți o imagine pentru notiță",
		"imagen.noLeible": "Nu poate fi citit ca imagine sau este prea mare pentru o notiță. Folosiți PNG, JPEG, GIF, WebP, AVIF, TIFF sau HEIC.",
	},
	"sk": {
		"sin.calendar":    "Sytue Calendar zo Sytue Office nie je v tomto zariadení nainštalovaný.",
		"volcado.cifrado": "Táto záloha pochádza zo šifrovaného zošita a Sytue Office šifrované zošity neotvára. Dešifrujte ho v aplikácii, v ktorej ste ho vytvorili, a znova ho exportujte.",
		"imagen.anadir":   "Pridať obrázok z tohto zariadenia…",
		"imagen.elegir":   "Vyberte obrázok pre poznámku",
		"imagen.noLeible": "Nedá sa prečítať ako obrázok alebo je na poznámku príliš veľký. Použite PNG, JPEG, GIF, WebP, AVIF, TIFF alebo HEIC.",
	},
	"sl": {
		"sin.calendar":    "Aplikacija Sytue Calendar iz paketa Sytue Office v tej napravi ni nameščena.",
		"volcado.cifrado": "Ta varnostna kopija je iz šifriranega zvezka, Sytue Office pa šifriranih zvezkov ne odpira. Dešifrirajte ga v aplikaciji, v kateri ste ga ustvarili, in ga znova izvozite.",
		"imagen.anadir":   "Dodaj sliko iz te naprave…",
		"imagen.elegir":   "Izberite sliko za zapisek",
		"imagen.noLeible": "Tega ni mogoče prebrati kot sliko ali pa je preveliko za zapisek. Uporabite PNG, JPEG, GIF, WebP, AVIF, TIFF ali HEIC.",
	},
	"sv": {
		"sin.calendar":    "Sytue Calendar från Sytue Office är inte installerad på den här enheten.",
		"volcado.cifrado": "Den här säkerhetskopian kommer från en krypterad anteckningsbok, och Sytue Office öppnar inte krypterade anteckningsböcker. Dekryptera den i appen där du skapade den och exportera den igen.",
		"imagen.anadir":   "Lägg till en bild från den här enheten…",
		"imagen.elegir":   "Välj en bild till anteckningen",
		"imagen.noLeible": "Det går inte att läsa som en bild, eller så är den för stor för en anteckning. Använd PNG, JPEG, GIF, WebP, AVIF, TIFF eller HEIC.",
	},
}
