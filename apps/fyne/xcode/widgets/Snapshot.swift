// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import Foundation

// Only declared, local actions are allowed. Neither a snapshot nor its counters
// can supply a URL, a query, an account or a destination outside the host app.
enum SytueWidgetActions {
    // «Buscar» ya no es una acción de los widgets (2026-11-24, ADR 0078): el dueño los
    // quitó porque no aportaban. Un `search` que siga declarado en un FyneApp.toml no da
    // ruta ni widget.
    static let order = ["open", "compose", "create", "upload"]

    // Las acciones rápidas de cada producto, además de «open» (ADR 0078): cada una abre un
    // formulario, una pregunta o el selector del sistema que la persona confirma. La misma
    // tabla en generar.py, android-widgets.py y WidgetSnapshot.accionPermitida (Java).
    static let allowed: [String: Set<String>] = [
        "mail": ["compose"], "calendar": ["create"], "contacts": ["create"], "notes": ["create"],
        "photos": ["upload"],
    ]

    // El rótulo traducido de una acción en su producto: «create» es «Crear evento» en
    // Calendar, «Nuevo contacto» en Contacts y «Nueva nota» en Notes.
    static func label(for action: String, product: String = "calendar") -> String {
        switch (action, product) {
        case ("create", "contacts"): return "widgets.newContact"
        case ("create", "notes"): return "widgets.newNote"
        case ("upload", _): return "widgets.addPhotos"
        default: return "widgets." + action
        }
    }

    static func symbol(for action: String, product: String = "calendar") -> String {
        switch (action, product) {
        case ("compose", _): return "square.and.pencil"
        case ("create", "contacts"): return "person.crop.circle.badge.plus"
        case ("create", "notes"): return "note.text.badge.plus"
        case ("create", _): return "calendar.badge.plus"
        case ("upload", _): return "photo.on.rectangle.angled"
        default: return "arrow.up.right"
        }
    }

    // A dónde lleva una fila (formato 3): su ruta dentro de ESTA app, que la app escribió
    // con widgets.Ruta (el día de un evento, `?date=AAAA-MM-DD`; una nota, un contacto, un
    // documento…). Se le antepone el esquema firmado de la extensión, así que no puede
    // llevar a otra app, a la web ni a otra cuenta; una ruta fuera de su forma ya dejó el
    // snapshot sin enseñar al leerlo. Sin ruta, nil: la fila abre la app.
    static func route(of item: SytueWidgetItem, product: String, scheme: String) -> URL? {
        guard !product.isEmpty, scheme == "sytue-" + product, !item.routeText.isEmpty,
              SytueWidgetSnapshot.isValidRoute(item.routeText) else { return nil }
        return URL(string: scheme + ":" + item.routeText)
    }

    static func routes(product: String, scheme: String, declared: [String]) -> [String: URL] {
        guard !product.isEmpty, scheme == "sytue-" + product else { return [:] }
        var routes = [String: URL]()
        for action in declared {
            if action == "open" {
                routes[action] = URL(string: scheme + ":/")
            } else if allowed[product]?.contains(action) == true {
                routes[action] = URL(string: scheme + ":/?action=" + action)
            }
        }
        return routes
    }
}

struct SytueWidgetCounter: Decodable {
    let kind: String
    let value: Int64

    var localizationKey: String {
        switch kind {
        case "files", "events", "documents", "contacts": return "dataflow.item." + kind
        case "unread": return "notifications.unread"
        case "storage_percent": return "widgets.storage"
        default: return "widgets." + kind
        }
    }
}

struct SytueWidgetItem: Decodable, Equatable {
    let title: String
    let subtitle: String?
    let badge: String?
    let color: String?
    let start_at: Int64?
    let end_at: Int64?
    let all_day: Bool?
    let unread: Bool?
    let done: Bool?
    // La ruta de la fila dentro de la propia app (widgets.Item.Route en Go).
    var route: String? = nil

    var subtitleText: String { subtitle ?? "" }
    var routeText: String { route ?? "" }
    var badgeText: String { badge ?? "" }
    var colorText: String { color ?? "" }
    var startAt: Int64 { start_at ?? 0 }
    var endAt: Int64 { end_at ?? 0 }
    var isAllDay: Bool { all_day ?? false }
    var isUnread: Bool { unread ?? false }
    var isDone: Bool { done ?? false }
}

struct SytueWidgetCalendar: Decodable, Equatable {
    let year: Int
    let month: Int
    let busy_days: [Int]?
    let day_colors: [String: String]?

    var busyDays: [Int] { busy_days ?? [] }
    var dayColors: [String: String] { day_colors ?? [:] }
}

// Lo que el lector comprueba FUERA del snapshot (widgets.Entorno en Go, formato 3): la
// época de identidad (primera línea de `sesion.epoca`), la marca con que se confirmó
// (cuarta), la `sesion.marca` actual, si hay un `*.cerrada` y el bloqueo de la suite.
struct SytueWidgetEntorno: Equatable {
    let epoca: String
    let confirmada: String
    let marca: String
    let cerrada: Bool
    let bloqueo: Bool

    var permite: Bool {
        !bloqueo && !cerrada && !epoca.isEmpty && !marca.isEmpty && confirmada == marca
    }
}

struct SytueWidgetSnapshot: Decodable {
    // El contrato común con Go y Java (apps/nucleo/widgets/testdata/contrato.tsv): solo el
    // formato 3, como mucho 24 h, y a partir de 15 min el widget dice de cuándo es el dato.
    // Cada fila puede llevar su ruta dentro de la app, de como mucho `maxRouteBytes`.
    static let version = 3
    static let maxAge: Int64 = 86400
    static let fresco: Int64 = 900
    static let maxRouteBytes = 512

    let version: Int
    let product: String
    let generation: String
    let updated_at: Int64
    let expires_at: Int64
    let active: Bool
    let counters: [SytueWidgetCounter]
    let items: [SytueWidgetItem]?
    let calendar: SytueWidgetCalendar?

    var visualItems: [SytueWidgetItem] { items ?? [] }

    static let kinds: Set<String> = [
        "unread", "files", "events", "documents", "contacts", "notes", "rows",
        "pending", "channels", "photos", "episodes", "apps", "calendars", "address_books",
        "storage_percent",
    ]

    private static let maxTimestamp: Int64 = 10_000_000_000

    static func daysInMonth(year: Int, month: Int) -> Int {
        switch month {
        case 1, 3, 5, 7, 8, 10, 12: return 31
        case 4, 6, 9, 11: return 30
        case 2:
            let leap = (year % 4 == 0 && year % 100 != 0) || (year % 400 == 0)
            return leap ? 29 : 28
        default: return 0
        }
    }

    static func isHexColor(_ value: String) -> Bool {
        let bytes = Array(value.utf8)
        guard bytes.count == 7, bytes[0] == 0x23 else { return false }
        for b in bytes.dropFirst() {
            let isDigit = b >= 0x30 && b <= 0x39
            let isLowerHex = b >= 0x61 && b <= 0x66
            if !isDigit && !isLowerHex { return false }
        }
        return true
    }

    // widgets.ValidRoute: vacía, o de 1 a `maxRouteBytes` bytes que empiezan por UNA barra,
    // solo con letras y cifras ASCII, `-._~/?=&` y escapes `%XX` en mayúsculas, y sin el
    // marcador de cifrado escapado.
    static func isValidRoute(_ value: String) -> Bool {
        let bytes = Array(value.utf8)
        if bytes.isEmpty { return true }
        guard bytes.count <= maxRouteBytes, bytes[0] == 0x2F, !(bytes.count > 1 && bytes[1] == 0x2F),
              !value.uppercased().contains("ENC%3AV1%3A") else { return false }
        func unreserved(_ b: UInt8) -> Bool {
            (b >= 0x61 && b <= 0x7A) || (b >= 0x41 && b <= 0x5A) || (b >= 0x30 && b <= 0x39)
                || b == 0x2D || b == 0x2E || b == 0x5F || b == 0x7E
        }
        func upperHex(_ b: UInt8) -> Bool { (b >= 0x30 && b <= 0x39) || (b >= 0x41 && b <= 0x46) }
        var i = 0
        while i < bytes.count {
            let b = bytes[i]
            if unreserved(b) || b == 0x2F || b == 0x3F || b == 0x3D || b == 0x26 {
                i += 1
            } else if b == 0x25, i + 2 < bytes.count, upperHex(bytes[i + 1]), upperHex(bytes[i + 2]) {
                i += 3
            } else {
                return false
            }
        }
        return true
    }

    private static func validText(_ value: String, maxBytes: Int) -> Bool {
        let bytes = Array(value.utf8)
        guard bytes.count <= maxBytes, !value.lowercased().contains("enc:v1:") else { return false }
        for b in bytes where b < 0x20 || b == 0x7f {
            return false
        }
        return true
    }

    func isVisible(product expectedProduct: String, entorno: SytueWidgetEntorno, now: Int64) -> Bool {
        guard entorno.permite, version == Self.version, product == expectedProduct, active,
              !generation.isEmpty, generation.utf8.count <= 128, generation == entorno.epoca,
              updated_at > 0, updated_at <= now + 5, expires_at > now,
              expires_at > updated_at, expires_at - updated_at <= Self.maxAge,
              counters.count <= 12, visualItems.count <= 8 else { return false }
        var seen = Set<String>()
        for counter in counters {
            guard Self.kinds.contains(counter.kind), counter.value >= 0,
                  counter.value <= 1_000_000_000_000,
                  (counter.kind != "storage_percent" || counter.value <= 100),
                  seen.insert(counter.kind).inserted else { return false }
        }
        for item in visualItems {
            guard !item.title.trimmingCharacters(in: .whitespaces).isEmpty,
                  Self.validText(item.title, maxBytes: 120),
                  Self.validText(item.subtitleText, maxBytes: 160),
                  Self.validText(item.badgeText, maxBytes: 24),
                  (item.colorText.isEmpty || Self.isHexColor(item.colorText)),
                  item.startAt >= 0, item.startAt <= Self.maxTimestamp,
                  item.endAt >= 0, item.endAt <= Self.maxTimestamp,
                  (item.endAt == 0 || item.startAt == 0 || item.endAt >= item.startAt),
                  Self.isValidRoute(item.routeText) else { return false }
        }
        if let cal = calendar {
            guard (1970...2200).contains(cal.year), (1...12).contains(cal.month) else { return false }
            let maxDay = Self.daysInMonth(year: cal.year, month: cal.month)
            guard cal.busyDays.count <= 31, cal.dayColors.count <= 31 else { return false }
            var previous = 0
            for day in cal.busyDays {
                guard day >= 1, day <= maxDay, day > previous else { return false }
                previous = day
            }
            for (key, color) in cal.dayColors {
                guard let day = Int(key), String(day) == key, day >= 1, day <= maxDay,
                      Self.isHexColor(color) else { return false }
            }
        }
        return true
    }

    // Snapshot.Rancio: a partir de aquí el widget dice «Actualizado hace X».
    func isStale(now: Int64) -> Bool { now - updated_at >= Self.fresco }

    var updatedDate: Date { Date(timeIntervalSince1970: TimeInterval(updated_at)) }
}

enum SytueWidgetTimeline {
    static func expiry(snapshot: SytueWidgetSnapshot?, now: Date) -> Date? {
        guard let snapshot else { return nil }
        let date = Date(timeIntervalSince1970: TimeInterval(snapshot.expires_at))
        return date > now ? date : nil
    }

    // widgets.Vigentes: en Calendar, un evento con hora vale mientras no haya terminado (o
    // no haya empezado); uno de día completo es FLOTANTE —medianoche UTC— y vale hasta la
    // medianoche local que sigue a su último día, con el final exclusivo. Sin fechas, vale.
    static func isCurrent(_ item: SytueWidgetItem, now: Int64, calendar: Calendar) -> Bool {
        if item.startAt == 0 && item.endAt == 0 { return true }
        guard item.isAllDay else { return item.endAt > now || item.startAt >= now }
        guard let end = allDayEnd(item, calendar: calendar) else { return false }
        return now < Int64(end.timeIntervalSince1970)
    }

    static func allDayEnd(_ item: SytueWidgetItem, calendar: Calendar) -> Date? {
        let last = item.endAt > item.startAt ? item.endAt - 1 : item.startAt
        var utc = Calendar(identifier: .gregorian)
        guard let zone = TimeZone(identifier: "UTC") else { return nil }
        utc.timeZone = zone
        let day = utc.dateComponents([.year, .month, .day], from: Date(timeIntervalSince1970: TimeInterval(last)))
        guard let start = calendar.date(from: DateComponents(year: day.year, month: day.month, day: day.day)) else { return nil }
        return calendar.date(byAdding: .day, value: 1, to: start)
    }

    static func currentItems(_ snapshot: SytueWidgetSnapshot?, at date: Date, calendar: Calendar) -> [SytueWidgetItem] {
        guard let snapshot else { return [] }
        guard snapshot.product == "calendar" else { return snapshot.visualItems }
        let now = Int64(date.timeIntervalSince1970)
        return snapshot.visualItems.filter { isCurrent($0, now: now, calendar: calendar) }
    }

    // Las entradas de una línea de tiempo `.atEnd`, precalculadas para que el widget cambie
    // sin que nadie lo recargue: ahora; a los 15, 30 y 45 min y a la hora de la última
    // comprobación, y después cada hora (el «Actualizado hace X»); cada medianoche (el
    // «hoy» y la rejilla del mes); cuando termina cada evento (Vigentes). La última, la
    // caducidad, va sin datos y la añade quien llama. Como mucho `maxEntries` en total.
    static let maxEntries = 40

    static func entryDates(snapshot: SytueWidgetSnapshot, now: Date, calendar: Calendar) -> [Date] {
        guard let expiry = Self.expiry(snapshot: snapshot, now: now) else { return [] }
        var dates = Set<Date>()
        let updated = snapshot.updatedDate
        for minutes in [15, 30, 45] {
            dates.insert(updated.addingTimeInterval(TimeInterval(minutes * 60)))
        }
        var hour = updated.addingTimeInterval(3600)
        while hour < expiry {
            dates.insert(hour)
            hour = hour.addingTimeInterval(3600)
        }
        var midnight = calendar.startOfDay(for: now)
        while let next = calendar.date(byAdding: .day, value: 1, to: midnight), next < expiry {
            dates.insert(next)
            midnight = next
        }
        if snapshot.product == "calendar" {
            for item in snapshot.visualItems {
                if item.isAllDay {
                    if let end = allDayEnd(item, calendar: calendar) { dates.insert(end) }
                } else if item.endAt > 0 {
                    dates.insert(Date(timeIntervalSince1970: TimeInterval(item.endAt)))
                } else if item.startAt > 0 {
                    dates.insert(Date(timeIntervalSince1970: TimeInterval(item.startAt + 1)))
                }
            }
        }
        let future = dates.filter { $0 > now && $0 < expiry }.sorted()
        return [now] + future.prefix(maxEntries - 2)
    }

    // Qué línea de tiempo pide WidgetKit (Widgets.swift la traduce).
    enum Plan: Equatable {
        // Las fechas con datos y, al final, la caducidad sin ellos; se vuelve a leer a los
        // `reread` (o en la caducidad, si llega antes).
        case entries([Date], expiry: Date)
        // La lectura falló por la protección de datos (el teléfono bloqueado): no es «no
        // hay datos». Se pinta el marco con el marcador y se vuelve a pedir en 5 min, sin
        // la entrada vacía de 24 h que borraría lo que ya había.
        case protected(retry: Date)
        // Sin datos que enseñar: se vuelve a mirar a los `reread`, por si la app publicó
        // con su recarga sin presupuesto.
        case empty
    }

    static let protectedRetry: TimeInterval = 5 * 60

    // Cada cuánto vuelve WidgetKit a leer el snapshot aunque la app no lo recargue: lo que
    // publicaron la tarea de refresco o el aviso silencioso con la app al fondo llega aunque
    // su reloadAllTimelines se quedara sin presupuesto. Es lo que la app pide a iOS para esa
    // tarea (sesion.CadaRefrescoEnFondo); WidgetKit lo toma como un mínimo.
    static let reread: TimeInterval = 15 * 60

    static func rereadDate(now: Date, expiry: Date? = nil) -> Date {
        let next = now.addingTimeInterval(reread)
        guard let expiry else { return next }
        return min(next, expiry)
    }

    static func plan(for reading: SytueWidgetReading, now: Date, calendar: Calendar) -> Plan {
        switch reading {
        case .visible(let snapshot):
            guard let expiry = Self.expiry(snapshot: snapshot, now: now) else { return .empty }
            return .entries(entryDates(snapshot: snapshot, now: now, calendar: calendar), expiry: expiry)
        case .protected:
            return .protected(retry: now.addingTimeInterval(protectedRetry))
        case .hidden:
            return .empty
        }
    }
}

// El resultado de leer: visible, oculto (no hay, caducado, de otra sesión, bloqueo de la
// suite…) o protegido (el sistema no deja leer el contenedor: dispositivo bloqueado).
enum SytueWidgetReading {
    case visible(SytueWidgetSnapshot)
    case hidden
    case protected

    var snapshot: SytueWidgetSnapshot? {
        if case .visible(let snapshot) = self { return snapshot }
        return nil
    }

    var isProtected: Bool {
        if case .protected = self { return true }
        return false
    }
}

enum SytueWidgetSnapshots {
    private static let requiredFields: Set<String> = [
        "version", "product", "generation", "updated_at", "expires_at", "active", "counters",
    ]
    private static let allowedFields: Set<String> = requiredFields.union(["items", "calendar"])
    private static let itemAllowedFields: Set<String> = [
        "title", "subtitle", "badge", "color", "start_at", "end_at", "all_day", "unread", "done", "route",
    ]
    private static let calendarRequiredFields: Set<String> = ["year", "month"]
    private static let calendarAllowedFields: Set<String> = ["year", "month", "busy_days", "day_colors"]

    private static func validObjectStructure(_ object: [String: Any]) -> Bool {
        let keys = Set(object.keys)
        guard requiredFields.isSubset(of: keys), keys.isSubset(of: allowedFields),
              !object.values.contains(where: { $0 is NSNull }),
              let rows = object["counters"] as? [[String: Any]],
              rows.allSatisfy({ Set($0.keys) == ["kind", "value"] }) else { return false }
        if let rawItems = object["items"] {
            guard let itemRows = rawItems as? [[String: Any]] else { return false }
            for row in itemRows {
                let rowKeys = Set(row.keys)
                guard rowKeys.contains("title"), rowKeys.isSubset(of: itemAllowedFields),
                      !row.values.contains(where: { $0 is NSNull }) else { return false }
            }
        }
        if let rawCal = object["calendar"] {
            guard let calObj = rawCal as? [String: Any] else { return false }
            let calKeys = Set(calObj.keys)
            guard calendarRequiredFields.isSubset(of: calKeys),
                  calKeys.isSubset(of: calendarAllowedFields),
                  !calObj.values.contains(where: { $0 is NSNull }) else { return false }
            if let colors = calObj["day_colors"], !(colors is [String: String]) {
                return false
            }
        }
        return true
    }

    // All paths are derived from signed bundle metadata. JSON cannot name a
    // file, account, server or URL. The extension never opens the Keychain.
    static func load(directory: URL, bundle: String, product: String, now: Int64) -> SytueWidgetSnapshot? {
        reading(directory: directory, bundle: bundle, product: product, now: now).snapshot
    }

    static func reading(directory: URL, bundle: String, product: String, now: Int64) -> SytueWidgetReading {
        guard bundle.range(of: #"^eu\.sytue\.[a-z][a-z0-9]*$"#, options: .regularExpression) != nil else { return .hidden }
        do {
            let before = try entorno(directory)
            guard before.permite else { return .hidden }
            let data = try read(directory.appendingPathComponent("widgets").appendingPathComponent(bundle + ".json"), limit: 16 << 10)
            guard let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                  validObjectStructure(object),
                  let snapshot = try? JSONDecoder().decode(SytueWidgetSnapshot.self, from: data),
                  snapshot.isVisible(product: product, entorno: before, now: now),
                  try entorno(directory) == before else { return .hidden }
            return .visible(snapshot)
        } catch let error where isProtected(error) {
            return .protected
        } catch {
            return .hidden
        }
    }

    // Formato 3: la época y su confirmación de `sesion.epoca`, la `sesion.marca`, el
    // bloqueo de `ajustes.json` y si queda algún `*.cerrada` (un cierre de sesión).
    static func entorno(_ directory: URL) throws -> SytueWidgetEntorno {
        let marker = try read(directory.appendingPathComponent("sesion.marca"), limit: 1024)
        let epoch = try read(directory.appendingPathComponent("sesion.epoca"), limit: 1024)
        let lines = (String(data: epoch, encoding: .utf8) ?? "").components(separatedBy: "\n")
        let marca = String(data: marker, encoding: .utf8) ?? ""
        guard lines.count == 5, lines[0].utf8.count == 32, lines[4].isEmpty else {
            return SytueWidgetEntorno(epoca: "", confirmada: "", marca: marca, cerrada: true, bloqueo: true)
        }
        return SytueWidgetEntorno(epoca: lines[0], confirmada: lines[3], marca: marca,
                                  cerrada: closed(directory), bloqueo: try !unlocked(directory))
    }

    // Listar no depende de la protección de datos; si no se puede, como si hubiera cierre.
    private static func closed(_ directory: URL) -> Bool {
        guard let names = try? FileManager.default.contentsOfDirectory(atPath: directory.path) else { return true }
        return names.contains { $0.hasSuffix(".cerrada") }
    }

    // NSFileProtectionComplete con el dispositivo bloqueado: el fichero existe pero no se
    // puede abrir. Distinto de «no existe», que es no tener datos.
    static func isProtected(_ error: Error) -> Bool {
        if let cocoa = error as? CocoaError, cocoa.code == .fileReadNoPermission { return true }
        let ns = error as NSError
        return ns.domain == NSPOSIXErrorDomain && (ns.code == 1 || ns.code == 13) // EPERM, EACCES
    }

    private static func unlocked(_ directory: URL) throws -> Bool {
        let url = directory.appendingPathComponent("ajustes.json")
        struct Preferences: Decodable { let bloqueo: Bool? }
        let data: Data
        do {
            data = try read(url, limit: 16 << 10)
        } catch let error as CocoaError where error.code == .fileReadNoSuchFile || error.code == .fileNoSuchFile {
            // A fresh profile may not have changed any preference yet.
            return true
        }
        // Malformed settings stay private; a protected read propagates as protected.
        guard let preferences = try? JSONDecoder().decode(Preferences.self, from: data) else { return false }
        return preferences.bloqueo != true
    }

    private enum ReadError: Error { case oversized }

    private static func read(_ url: URL, limit: Int) throws -> Data {
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        guard let data = try handle.read(upToCount: limit + 1), data.count <= limit else { throw ReadError.oversized }
        return data
    }
}
