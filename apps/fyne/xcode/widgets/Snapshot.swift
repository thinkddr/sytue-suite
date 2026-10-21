// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import Foundation

// Only declared, local actions are allowed. Neither a snapshot nor its counters
// can supply a URL, a query, an account or a destination outside the host app.
enum SytueWidgetActions {
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
}

struct SytueWidgetSnapshot: Decodable {
    let version: Int
    let product: String
    let generation: String
    let updated_at: Int64
    let expires_at: Int64
    let active: Bool
    let counters: [SytueWidgetCounter]

    static let kinds: Set<String> = [
        "unread", "files", "events", "documents", "contacts", "notes", "rows",
        "pending", "channels", "photos", "episodes", "apps", "calendars", "address_books",
    ]

    func isVisible(product expectedProduct: String, generation expectedGeneration: String, now: Int64) -> Bool {
        guard version == 1, product == expectedProduct, active,
              !expectedGeneration.isEmpty, generation == expectedGeneration,
              updated_at > 0, updated_at <= now + 5, expires_at > now,
              expires_at > updated_at, expires_at - updated_at <= 1800,
              counters.count <= 12 else { return false }
        var seen = Set<String>()
        for counter in counters {
            guard Self.kinds.contains(counter.kind), counter.value >= 0,
                  counter.value <= 1_000_000_000_000,
                  seen.insert(counter.kind).inserted else { return false }
        }
        return true
    }
}

enum SytueWidgetSnapshots {
    private static let fields: Set<String> = [
        "version", "product", "generation", "updated_at", "expires_at", "active", "counters",
    ]

    // All paths are derived from signed bundle metadata. JSON cannot name a
    // file, account, server or URL. The extension never opens the Keychain.
    static func load(directory: URL, bundle: String, product: String, now: Int64) -> SytueWidgetSnapshot? {
        guard bundle.range(of: #"^eu\.sytue\.[a-z][a-z0-9]*$"#, options: .regularExpression) != nil,
              let marker = try? read(directory.appendingPathComponent("sesion.marca"), limit: 1024),
              let generation = String(data: marker, encoding: .utf8), !generation.isEmpty,
              unlocked(directory),
              let data = try? read(directory.appendingPathComponent("widgets").appendingPathComponent(bundle + ".json"), limit: 16 << 10),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              Set(object.keys) == fields,
              let rows = object["counters"] as? [[String: Any]],
              rows.allSatisfy({ Set($0.keys) == ["kind", "value"] }),
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
