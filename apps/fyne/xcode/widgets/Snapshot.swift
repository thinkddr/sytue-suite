// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import Foundation

// Only declared, local actions are allowed. Neither a snapshot nor its counters
// can supply a URL, a query, an account or a destination outside the host app.
enum SytueWidgetActions {
    static let order = ["open", "compose", "create", "search"]

    static func label(for action: String) -> String {
        action == "search" ? "search.label" : "widgets." + action
    }

    static func symbol(for action: String) -> String {
        switch action {
        case "search": return "magnifyingglass"
        case "compose": return "square.and.pencil"
        case "create": return "calendar.badge.plus"
        default: return "arrow.up.right"
        }
    }

    static func routes(product: String, scheme: String, declared: [String]) -> [String: URL] {
        guard !product.isEmpty, scheme == "sytue-" + product else { return [:] }
        var routes = [String: URL]()
        for action in declared {
            if action == "open" { routes[action] = URL(string: scheme + ":/") }
            if action == "search" { routes[action] = URL(string: scheme + ":/?action=search") }
            if action == "compose", product == "mail" {
                routes[action] = URL(string: scheme + ":/?action=compose")
            }
            if action == "create", product == "calendar" {
                routes[action] = URL(string: scheme + ":/?action=create")
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

    var subtitleText: String { subtitle ?? "" }
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

struct SytueWidgetSnapshot: Decodable {
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

    private static func validText(_ value: String, maxBytes: Int) -> Bool {
        let bytes = Array(value.utf8)
        guard bytes.count <= maxBytes, !value.lowercased().contains("enc:v1:") else { return false }
        for b in bytes where b < 0x20 || b == 0x7f {
            return false
        }
        return true
    }

    func isVisible(product expectedProduct: String, generation expectedGeneration: String, now: Int64) -> Bool {
        guard version == 1, product == expectedProduct, active,
              !expectedGeneration.isEmpty, generation == expectedGeneration,
              updated_at > 0, updated_at <= now + 5, expires_at > now,
              expires_at > updated_at, expires_at - updated_at <= 1800,
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
                  (item.endAt == 0 || item.startAt == 0 || item.endAt >= item.startAt) else { return false }
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
}

enum SytueWidgetTimeline {
    static func expiry(snapshot: SytueWidgetSnapshot?, now: Date) -> Date? {
        guard let snapshot else { return nil }
        let date = Date(timeIntervalSince1970: TimeInterval(snapshot.expires_at))
        return date > now ? date : nil
    }
}

enum SytueWidgetSnapshots {
    private static let requiredFields: Set<String> = [
        "version", "product", "generation", "updated_at", "expires_at", "active", "counters",
    ]
    private static let allowedFields: Set<String> = requiredFields.union(["items", "calendar"])
    private static let itemAllowedFields: Set<String> = [
        "title", "subtitle", "badge", "color", "start_at", "end_at", "all_day", "unread", "done",
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
        guard bundle.range(of: #"^eu\.sytue\.[a-z][a-z0-9]*$"#, options: .regularExpression) != nil,
              let marker = try? read(directory.appendingPathComponent("sesion.marca"), limit: 1024),
              let generation = String(data: marker, encoding: .utf8), !generation.isEmpty,
              unlocked(directory),
              let data = try? read(directory.appendingPathComponent("widgets").appendingPathComponent(bundle + ".json"), limit: 16 << 10),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              validObjectStructure(object),
              let snapshot = try? JSONDecoder().decode(SytueWidgetSnapshot.self, from: data),
              snapshot.isVisible(product: product, generation: generation, now: now),
              (try? read(directory.appendingPathComponent("sesion.marca"), limit: 1024)) == marker,
              unlocked(directory) else { return nil }
        return snapshot
    }

    private static func unlocked(_ directory: URL) -> Bool {
        let url = directory.appendingPathComponent("ajustes.json")
        struct Preferences: Decodable { let bloqueo: Bool? }
        do {
            let data = try read(url, limit: 16 << 10)
            return try JSONDecoder().decode(Preferences.self, from: data).bloqueo != true
        } catch let error as CocoaError where error.code == .fileReadNoSuchFile || error.code == .fileNoSuchFile {
            // A fresh profile may not have changed any preference yet. Other
            // errors (including protected files while locked) stay private.
            return true
        } catch { return false }
    }

    private enum ReadError: Error { case oversized }

    private static func read(_ url: URL, limit: Int) throws -> Data {
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        guard let data = try handle.read(upToCount: limit + 1), data.count <= limit else { throw ReadError.oversized }
        return data
    }
}
