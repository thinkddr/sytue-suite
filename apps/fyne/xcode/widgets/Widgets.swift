// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import Foundation
import SwiftUI
import WidgetKit

struct SytueWidgetConfiguration {
    let bundle: String
    let product: String
    let name: String
    let group: String
    let actions: [String: URL]

    init(bundle source: Bundle = .main) {
        let metadata = source.infoDictionary ?? [:]
        bundle = metadata["SytueWidgetBundle"] as? String ?? ""
        product = metadata["SytueWidgetProduct"] as? String ?? ""
        name = metadata["CFBundleDisplayName"] as? String ?? "Sytue"
        group = metadata["SytueWidgetGroup"] as? String ?? ""
        let scheme = metadata["SytueWidgetScheme"] as? String ?? ""
        actions = SytueWidgetActions.routes(product: product, scheme: scheme,
                                          declared: metadata["SytueWidgetActions"] as? [String] ?? [])
    }

    var descriptionKey: String { "app." + product + ".description" }

    var brandColor: Color { SytueWidgetPalette.brand(for: product) }

    var productSymbol: String { SytueWidgetPalette.symbol(for: product) }

    func reading(at date: Date) -> SytueWidgetReading {
        guard !group.isEmpty,
              let container = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: group) else { return .hidden }
        return SytueWidgetSnapshots.reading(directory: container.appendingPathComponent("sytue"), bundle: bundle,
                                            product: product, now: Int64(date.timeIntervalSince1970))
    }
}

private enum SytueWidgetPalette {
    static func brand(for product: String) -> Color {
        switch product {
        case "calendar": return Color(red: 0.584, green: 0.231, blue: 0.906) // #953be7
        case "mail":     return Color(red: 0.863, green: 0.204, blue: 0.314) // #dc3450
        case "drive":    return Color(red: 0.086, green: 0.451, blue: 0.302) // #16734d
        case "notes":    return Color(red: 0.663, green: 0.122, blue: 0.706) // #a91fb4
        case "rooms":    return Color(red: 0.263, green: 0.220, blue: 0.792) // #4338ca
        case "contacts": return Color(red: 0.055, green: 0.455, blue: 0.565) // #0e7490
        case "docs":     return Color(red: 0.161, green: 0.408, blue: 0.925) // #2968ec
        case "sheets":   return Color(red: 0.302, green: 0.486, blue: 0.059) // #4d7c0f
        case "slides":   return Color(red: 0.690, green: 0.325, blue: 0.129) // #b05321
        case "pdf":      return Color(red: 0.847, green: 0.231, blue: 0.208) // #d83b35
        case "bases":    return Color(red: 0.541, green: 0.294, blue: 0.165) // #8a4b2a
        case "erp":      return Color(red: 0.706, green: 0.325, blue: 0.035) // #b45309
        case "photos":   return Color(red: 0.184, green: 0.541, blue: 0.165) // #2f8a2a
        case "cast":     return Color(red: 0.624, green: 0.071, blue: 0.224) // #9f1239
        default:         return Color(red: 0.761, green: 0.255, blue: 0.047) // #c2410c
        }
    }

    static func symbol(for product: String) -> String {
        switch product {
        case "calendar": return "calendar"
        case "mail":     return "envelope.fill"
        case "drive":    return "externaldrive.fill"
        case "notes":    return "note.text"
        case "rooms":    return "bubble.left.and.bubble.right.fill"
        case "contacts": return "person.crop.circle.fill"
        case "docs":     return "doc.text.fill"
        case "sheets":   return "tablecells.fill"
        case "slides":   return "play.rectangle.fill"
        case "pdf":      return "signature"
        case "bases":    return "tablecells"
        case "erp":      return "briefcase.fill"
        case "photos":   return "photo.on.rectangle.angled"
        case "cast":     return "play.tv.fill"
        default:         return "square.grid.2x2.fill"
        }
    }

    static func color(hex: String, fallback: Color) -> Color {
        guard SytueWidgetSnapshot.isHexColor(hex),
              let rgb = UInt32(hex.dropFirst(), radix: 16) else { return fallback }
        let r = Double((rgb >> 16) & 0xff) / 255.0
        let g = Double((rgb >> 8) & 0xff) / 255.0
        let b = Double(rgb & 0xff) / 255.0
        return Color(red: r, green: g, blue: b)
    }
}

private enum SytueWidgetDates {
    static func monthTitle(for date: Date) -> String {
        let formatter = DateFormatter()
        formatter.locale = .autoupdatingCurrent
        formatter.setLocalizedDateFormatFromTemplate("MMMM")
        return formatter.string(from: date).localizedUppercase
    }

    static func shortDayHeader(for date: Date) -> String {
        let formatter = DateFormatter()
        formatter.locale = .autoupdatingCurrent
        formatter.setLocalizedDateFormatFromTemplate("EEEEdMMM")
        return formatter.string(from: date)
    }

    static func itemTime(startAt: Int64, endAt: Int64, isAllDay: Bool, reference: Date) -> String {
        guard startAt > 0 else { return "" }
        let start = Date(timeIntervalSince1970: TimeInterval(startAt))
        let calendar = Calendar.autoupdatingCurrent
        let sameDay = calendar.isDate(start, inSameDayAs: reference)
        if isAllDay {
            if sameDay { return "" }
            let dayFmt = DateFormatter()
            dayFmt.locale = .autoupdatingCurrent
            dayFmt.setLocalizedDateFormatFromTemplate("dMMM")
            return dayFmt.string(from: start)
        }
        let timeFmt = DateFormatter()
        timeFmt.locale = .autoupdatingCurrent
        timeFmt.dateStyle = .none
        timeFmt.timeStyle = .short
        if sameDay {
            if endAt > startAt {
                let end = Date(timeIntervalSince1970: TimeInterval(endAt))
                if calendar.isDate(end, inSameDayAs: start) {
                    return timeFmt.string(from: start) + "–" + timeFmt.string(from: end)
                }
            }
            return timeFmt.string(from: start)
        }
        let dateTimeFmt = DateFormatter()
        dateTimeFmt.locale = .autoupdatingCurrent
        dateTimeFmt.setLocalizedDateFormatFromTemplate("dMMMjm")
        return dateTimeFmt.string(from: start)
    }

    static func compactTimestamp(_ epoch: Int64, reference: Date) -> String {
        guard epoch > 0 else { return "" }
        let date = Date(timeIntervalSince1970: TimeInterval(epoch))
        let calendar = Calendar.autoupdatingCurrent
        let fmt = DateFormatter()
        fmt.locale = .autoupdatingCurrent
        if calendar.isDate(date, inSameDayAs: reference) {
            fmt.dateStyle = .none
            fmt.timeStyle = .short
        } else {
            fmt.setLocalizedDateFormatFromTemplate("dMMM")
        }
        return fmt.string(from: date)
    }

    struct MonthLayout {
        let year: Int
        let month: Int
        let todayDay: Int
        let weekdayHeaders: [String]
        let weeks: [[Int]]
    }

    static func monthLayout(for date: Date) -> MonthLayout {
        var calendar = Calendar.autoupdatingCurrent
        calendar.locale = .autoupdatingCurrent
        let comps = calendar.dateComponents([.year, .month, .day], from: date)
        let year = comps.year ?? 2026
        let month = comps.month ?? 1
        let todayDay = comps.day ?? 1

        let rawSymbols = calendar.veryShortStandaloneWeekdaySymbols
        let firstIdx = max(0, min(rawSymbols.count - 1, calendar.firstWeekday - 1))
        var headers = [String]()
        if rawSymbols.count == 7 {
            for offset in 0..<7 {
                headers.append(rawSymbols[(firstIdx + offset) % 7])
            }
        } else {
            headers = ["M", "T", "W", "T", "F", "S", "S"]
        }

        let firstOfMonth = calendar.date(from: DateComponents(year: year, month: month, day: 1)) ?? date
        let firstWeekday = calendar.component(.weekday, from: firstOfMonth)
        let leadingEmpty = (firstWeekday - calendar.firstWeekday + 7) % 7
        let totalDays = SytueWidgetSnapshot.daysInMonth(year: year, month: month)

        var cells = Array(repeating: 0, count: leadingEmpty)
        if totalDays > 0 {
            for d in 1...totalDays {
                cells.append(d)
            }
        }
        while cells.count % 7 != 0 {
            cells.append(0)
        }
        var weeks = [[Int]]()
        var index = 0
        while index < cells.count {
            weeks.append(Array(cells[index..<min(index + 7, cells.count)]))
            index += 7
        }
        return MonthLayout(year: year, month: month, todayDay: todayDay, weekdayHeaders: headers, weeks: weeks)
    }
}

struct SytueWidgetEntry: TimelineEntry {
    let date: Date
    let snapshot: SytueWidgetSnapshot?
    // La lectura falló por la protección de datos: se pinta el marco con el marcador.
    var isProtected = false

    // Las filas que siguen valiendo a la fecha de ESTA entrada (widgets.Vigentes).
    var items: [SytueWidgetItem] {
        SytueWidgetTimeline.currentItems(snapshot, at: date, calendar: .autoupdatingCurrent)
    }

    // «Actualizado hace 20 min», calculado para la fecha de la entrada; nada si el dato es
    // fresco (menos de 15 min).
    var staleLabel: String? {
        guard let snapshot, snapshot.isStale(now: Int64(date.timeIntervalSince1970)) else { return nil }
        let relative = RelativeDateTimeFormatter()
        relative.dateTimeStyle = .named
        relative.unitsStyle = .short
        return String(format: NSLocalizedString("widgets.updatedAgo", comment: ""),
                      relative.localizedString(for: snapshot.updatedDate, relativeTo: date))
    }
}

struct SytueWidgetProvider: TimelineProvider {
    let configuration = SytueWidgetConfiguration()

    func placeholder(in context: Context) -> SytueWidgetEntry {
        SytueWidgetEntry(date: Date(), snapshot: nil)
    }

    func getSnapshot(in context: Context, completion: @escaping (SytueWidgetEntry) -> Void) {
        let date = Date()
        guard !context.isPreview else { return completion(SytueWidgetEntry(date: date, snapshot: nil)) }
        let reading = configuration.reading(at: date)
        completion(SytueWidgetEntry(date: date, snapshot: reading.snapshot, isProtected: reading.isProtected))
    }

    // Formato 2 (ADR 0078): el snapshot vive hasta 24 h, así que la línea de tiempo se
    // precalcula entera —el «Actualizado hace X», las medianoches, el fin de cada evento y
    // la caducidad sin datos— con `.atEnd`: sin recargas periódicas que gasten el
    // presupuesto. Recarga la app (reloadAllTimelines) cuando publica o borra.
    func getTimeline(in context: Context, completion: @escaping (Timeline<SytueWidgetEntry>) -> Void) {
        let date = Date()
        let reading = configuration.reading(at: date)
        switch SytueWidgetTimeline.plan(for: reading, now: date, calendar: .autoupdatingCurrent) {
        case .entries(let dates, let expiry):
            let entries = dates.map { SytueWidgetEntry(date: $0, snapshot: reading.snapshot) }
                + [SytueWidgetEntry(date: expiry, snapshot: nil)]
            completion(Timeline(entries: entries, policy: .atEnd))
        case .protected(let retry):
            // No es «no hay datos»: con el teléfono bloqueado el contenedor no se lee. Nada
            // de entrada vacía de 24 h; se vuelve a pedir en 5 min.
            completion(Timeline(entries: [SytueWidgetEntry(date: date, snapshot: nil, isProtected: true)],
                                policy: .after(retry)))
        case .empty:
            completion(Timeline(entries: [SytueWidgetEntry(date: date, snapshot: nil)], policy: .never))
        }
    }
}

// Action widgets contain no account data. Their static timeline never opens the
// shared container and requires neither background work nor a signed-in account.
struct SytueWidgetActionProvider: TimelineProvider {
    func placeholder(in context: Context) -> SytueWidgetEntry {
        SytueWidgetEntry(date: Date(), snapshot: nil)
    }

    func getSnapshot(in context: Context, completion: @escaping (SytueWidgetEntry) -> Void) {
        completion(placeholder(in: context))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<SytueWidgetEntry>) -> Void) {
        completion(Timeline(entries: [placeholder(in: context)], policy: .never))
    }
}

private struct SytueWidgetAccentable: ViewModifier {
    let accented: Bool

    func body(content: Content) -> some View {
        if #available(iOSApplicationExtension 16.0, macOSApplicationExtension 14.0, *) {
            content.widgetAccentable(accented)
        } else {
            content
        }
    }
}

private extension View {
    func sytueAccentable(_ enabled: Bool = true) -> some View {
        modifier(SytueWidgetAccentable(accented: enabled))
    }
}

private struct SytueWidgetCounterView: View {
    let counter: SytueWidgetCounter
    let brand: Color
    var compact = false

    var body: some View {
        counterContent
            .lineLimit(2)
            .minimumScaleFactor(0.75)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(counter.kind == "unread"
                ? Text(String.localizedStringWithFormat(NSLocalizedString("notifications.unread", comment: ""), counter.value))
                : Text(LocalizedStringKey(counter.localizationKey)))
            .accessibilityValue(counter.kind == "unread" ? Text("") : accessibleValue)
            .privacySensitive()
    }

    @ViewBuilder private var counterContent: some View {
        if compact {
            if counter.kind == "unread" {
                Text(String.localizedStringWithFormat(NSLocalizedString("notifications.unread", comment: ""), counter.value))
                    .font(.caption.weight(.semibold))
            } else {
                HStack(alignment: .firstTextBaseline, spacing: 4) {
                    value.font(.subheadline.weight(.bold)).foregroundStyle(brand).sytueAccentable()
                    Text(LocalizedStringKey(counter.localizationKey))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        } else {
            VStack(alignment: .leading, spacing: 2) {
                if counter.kind == "unread" {
                    Text(counter.value, format: .number.notation(.compactName))
                        .font(.system(.title2, design: .rounded).weight(.bold))
                        .foregroundStyle(brand)
                        .sytueAccentable()
                    Text(String.localizedStringWithFormat(NSLocalizedString("notifications.unread", comment: ""), counter.value))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                } else {
                    value
                        .font(.system(.title2, design: .rounded).weight(.bold))
                        .foregroundStyle(brand)
                        .sytueAccentable()
                    Text(LocalizedStringKey(counter.localizationKey))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
    }

    private var value: Text {
        counter.kind == "storage_percent"
            ? Text(Double(counter.value) / 100, format: .percent.precision(.fractionLength(0)))
            : Text(counter.value, format: .number.notation(.compactName))
    }

    private var accessibleValue: Text {
        counter.kind == "storage_percent"
            ? Text(Double(counter.value) / 100, format: .percent.precision(.fractionLength(0)))
            : Text(counter.value, format: .number)
    }
}

private struct SytueStorageBarView: View {
    let percent: Int64
    let brand: Color
    var compact = false

    private var barColor: Color {
        if percent >= 95 { return Color(red: 0.86, green: 0.18, blue: 0.22) }
        if percent >= 80 { return Color(red: 0.85, green: 0.47, blue: 0.02) }
        return brand
    }

    var body: some View {
        VStack(alignment: .leading, spacing: compact ? 3 : 5) {
            HStack(spacing: 6) {
                Text(LocalizedStringKey("widgets.storage"))
                    .font(.caption2.weight(.medium))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Spacer(minLength: 4)
                Text(Double(percent) / 100.0, format: .percent.precision(.fractionLength(0)))
                    .font(.caption2.weight(.bold))
                    .foregroundStyle(barColor)
                    .sytueAccentable()
            }
            GeometryReader { geo in
                let fraction = CGFloat(min(100, max(0, percent))) / 100.0
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(Color.primary.opacity(0.10))
                    Capsule()
                        .fill(barColor)
                        .frame(width: max(fraction > 0 ? 6 : 0, geo.size.width * fraction))
                        .sytueAccentable()
                }
            }
            .frame(height: compact ? 5 : 7)
        }
        .privacySensitive()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(LocalizedStringKey("widgets.storage")))
        .accessibilityValue(Text(Double(percent) / 100.0, format: .percent.precision(.fractionLength(0))))
    }
}

private struct SytueWidgetActionLabel: View {
    let action: String

    var body: some View {
        Label(LocalizedStringKey(SytueWidgetActions.label(for: action)),
              systemImage: SytueWidgetActions.symbol(for: action))
    }
}

private struct SytueQuickActionsBar: View {
    let configuration: SytueWidgetConfiguration
    var iconOnly = false

    private var secondaryActions: [String] {
        SytueWidgetActions.order.filter { $0 != "open" && configuration.actions[$0] != nil }
    }

    var body: some View {
        HStack(spacing: iconOnly ? 6 : 8) {
            ForEach(secondaryActions, id: \.self) { action in
                if let destination = configuration.actions[action] {
                    Link(destination: destination) {
                        if iconOnly {
                            Image(systemName: SytueWidgetActions.symbol(for: action))
                                .font(.system(size: 11, weight: .semibold))
                                .foregroundStyle(configuration.brandColor)
                                .frame(width: 24, height: 24)
                                .background(configuration.brandColor.opacity(0.14))
                                .clipShape(Circle())
                                .sytueAccentable()
                                .accessibilityLabel(Text(LocalizedStringKey(SytueWidgetActions.label(for: action))))
                        } else {
                            HStack(spacing: 4) {
                                Image(systemName: SytueWidgetActions.symbol(for: action))
                                    .font(.system(size: 10, weight: .semibold))
                                Text(LocalizedStringKey(SytueWidgetActions.label(for: action)))
                                    .font(.caption2.weight(.semibold))
                                    .lineLimit(1)
                                    .minimumScaleFactor(0.8)
                            }
                            .foregroundStyle(configuration.brandColor)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 4)
                            .background(configuration.brandColor.opacity(0.13))
                            .clipShape(Capsule())
                            .sytueAccentable()
                        }
                    }
                }
            }
        }
    }
}

private struct SytueCalendarMonthGridView: View {
    let date: Date
    let calendarData: SytueWidgetCalendar?
    let brand: Color
    var compact = false

    var body: some View {
        let layout = SytueWidgetDates.monthLayout(for: date)
        let matchingSnapshot = calendarData.flatMap { cal in
            (cal.year == layout.year && cal.month == layout.month) ? cal : nil
        }
        let busySet = Set(matchingSnapshot?.busyDays ?? [])
        let dayColors = matchingSnapshot?.dayColors ?? [:]
        let cellSize: CGFloat = compact ? 15 : 18
        let fontSize: CGFloat = compact ? 9.5 : 10.5

        VStack(alignment: .leading, spacing: compact ? 2 : 3) {
            HStack(spacing: 0) {
                ForEach(Array(layout.weekdayHeaders.enumerated()), id: \.offset) { _, symbol in
                    Text(symbol)
                        .font(.system(size: compact ? 8.5 : 9.5, weight: .semibold, design: .rounded))
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity)
                }
            }
            ForEach(Array(layout.weeks.enumerated()), id: \.offset) { _, week in
                HStack(spacing: 0) {
                    ForEach(Array(week.enumerated()), id: \.offset) { _, day in
                        if day == 0 {
                            Color.clear
                                .frame(maxWidth: .infinity, minHeight: cellSize, maxHeight: cellSize)
                        } else {
                            let isToday = day == layout.todayDay
                            let isBusy = busySet.contains(day)
                            let dotColor = SytueWidgetPalette.color(hex: dayColors[String(day)] ?? "", fallback: brand)
                            VStack(spacing: 1) {
                                ZStack {
                                    if isToday {
                                        Circle()
                                            .fill(brand)
                                            .frame(width: cellSize, height: cellSize)
                                            .sytueAccentable()
                                    }
                                    Text("\(day)")
                                        .font(.system(size: fontSize, weight: isToday ? .bold : (isBusy ? .semibold : .regular), design: .rounded))
                                        .foregroundStyle(isToday ? Color.white : Color.primary)
                                }
                                .frame(height: cellSize)
                                if !compact {
                                    Circle()
                                        .fill(isBusy ? dotColor : Color.clear)
                                        .frame(width: 3.5, height: 3.5)
                                        .privacySensitive()
                                } else if isBusy && !isToday {
                                    Circle()
                                        .fill(dotColor)
                                        .frame(width: 3, height: 3)
                                        .privacySensitive()
                                } else {
                                    Color.clear.frame(width: 3, height: 3)
                                }
                            }
                            .frame(maxWidth: .infinity)
                        }
                    }
                }
            }
        }
    }
}

private struct SytueCalendarEventRow: View {
    let item: SytueWidgetItem
    let brand: Color
    let referenceDate: Date
    var showSubtitle = false

    var body: some View {
        let accent = SytueWidgetPalette.color(hex: item.colorText, fallback: brand)
        let timeLabel = SytueWidgetDates.itemTime(
            startAt: item.startAt,
            endAt: item.endAt,
            isAllDay: item.isAllDay,
            reference: referenceDate
        )
        HStack(alignment: .center, spacing: 7) {
            RoundedRectangle(cornerRadius: 2, style: .continuous)
                .fill(accent)
                .frame(width: 3.5, height: showSubtitle && !item.subtitleText.isEmpty ? 28 : 18)
                .sytueAccentable()
            VStack(alignment: .leading, spacing: 1) {
                Text(item.title)
                    .font(.caption.weight(.semibold))
                    .lineLimit(1)
                if showSubtitle && !item.subtitleText.isEmpty {
                    Text(item.subtitleText)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                } else if !timeLabel.isEmpty {
                    Text(timeLabel)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 2)
            if showSubtitle && !timeLabel.isEmpty {
                Text(timeLabel)
                    .font(.system(size: 10, weight: .medium, design: .rounded))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            } else if !item.badgeText.isEmpty {
                Text(item.badgeText)
                    .font(.system(size: 9.5, weight: .semibold, design: .rounded))
                    .foregroundStyle(accent)
                    .padding(.horizontal, 5)
                    .padding(.vertical, 2)
                    .background(accent.opacity(0.14))
                    .clipShape(Capsule())
            }
        }
        .privacySensitive()
    }
}

private struct SytueProductItemRow: View {
    let product: String
    let item: SytueWidgetItem
    let brand: Color
    let referenceDate: Date
    var showSubtitle = true

    private var accent: Color {
        SytueWidgetPalette.color(hex: item.colorText, fallback: brand)
    }

    private var monogram: String {
        let source = !item.badgeText.isEmpty && product == "contacts" ? item.badgeText : item.title
        let trimmed = source.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let first = trimmed.first else { return "•" }
        return String(first).localizedUppercase
    }

    @ViewBuilder private var leadingIndicator: some View {
        switch product {
        case "notes", "erp":
            Image(systemName: item.isDone ? "checkmark.circle.fill" : "circle")
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(item.isDone ? Color.secondary : accent)
                .sytueAccentable()
        case "mail", "contacts":
            ZStack(alignment: .topTrailing) {
                Circle()
                    .fill(accent.opacity(0.16))
                    .frame(width: 22, height: 22)
                    .overlay(
                        Text(monogram)
                            .font(.system(size: 10, weight: .bold, design: .rounded))
                            .foregroundStyle(accent)
                    )
                if item.isUnread {
                    Circle()
                        .fill(accent)
                        .frame(width: 6.5, height: 6.5)
                        .sytueAccentable()
                }
            }
        case "rooms":
            ZStack {
                RoundedRectangle(cornerRadius: 6, style: .continuous)
                    .fill(accent.opacity(0.15))
                    .frame(width: 20, height: 20)
                Image(systemName: "number")
                    .font(.system(size: 9.5, weight: .bold))
                    .foregroundStyle(accent)
            }
        case "drive":
            Image(systemName: item.badgeText == "DIR" ? "folder.fill" : "doc.fill")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(accent)
                .frame(width: 18)
                .sytueAccentable()
        case "cast":
            Image(systemName: item.badgeText == "LIVE" ? "dot.radiowaves.left.and.right" : "play.circle.fill")
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(item.badgeText == "LIVE" ? Color(red: 0.88, green: 0.16, blue: 0.22) : accent)
                .frame(width: 18)
                .sytueAccentable()
        case "pdf":
            Image(systemName: item.badgeText == "FIRMAR" ? "signature" : "doc.richtext")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(accent)
                .frame(width: 18)
                .sytueAccentable()
        case "photos":
            Image(systemName: "photo.fill")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(accent)
                .frame(width: 18)
                .sytueAccentable()
        default:
            RoundedRectangle(cornerRadius: 2, style: .continuous)
                .fill(accent)
                .frame(width: 3.5, height: showSubtitle && !item.subtitleText.isEmpty ? 24 : 16)
                .sytueAccentable()
        }
    }

    var body: some View {
        let timeText = SytueWidgetDates.compactTimestamp(item.startAt, reference: referenceDate)
        let urgentBadge = item.badgeText == "LIVE" || item.badgeText == "FIRMAR"

        HStack(alignment: .center, spacing: 8) {
            leadingIndicator
            VStack(alignment: .leading, spacing: 1) {
                Text(item.title)
                    .font(.caption.weight(item.isUnread ? .bold : .semibold))
                    .foregroundStyle(item.isDone ? .secondary : .primary)
                    .lineLimit(1)
                if showSubtitle && !item.subtitleText.isEmpty {
                    Text(item.subtitleText)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 4)
            if !item.badgeText.isEmpty && !(product == "contacts" && item.badgeText.count <= 2) {
                Text(item.badgeText)
                    .font(.system(size: 9.5, weight: .bold, design: .rounded))
                    .foregroundStyle(urgentBadge ? Color.white : accent)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(urgentBadge ? Color(red: 0.86, green: 0.18, blue: 0.22) : accent.opacity(0.14))
                    .clipShape(Capsule())
                    .sytueAccentable(urgentBadge)
            } else if !timeText.isEmpty {
                Text(timeText)
                    .font(.system(size: 10, weight: .medium, design: .rounded))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        }
        .padding(.vertical, 2)
        .privacySensitive()
    }
}

private struct SytueWidgetBackground: ViewModifier {
    @Environment(\.widgetFamily) var family
    let brand: Color

    private var isAccessory: Bool {
        #if os(iOS)
        if #available(iOSApplicationExtension 16.0, *) {
            return [.accessoryCircular, .accessoryRectangular, .accessoryInline].contains(family)
        }
        #endif
        return false
    }

    @ViewBuilder private func legacy(_ content: Content) -> some View {
        if isAccessory {
            content
        } else {
            content
                .padding(14)
                .background(
                    ZStack {
                        Color.secondary.opacity(0.08)
                        LinearGradient(
                            colors: [brand.opacity(0.12), Color.clear],
                            startPoint: .topLeading,
                            endPoint: .bottomTrailing
                        )
                    }
                )
        }
    }

    func body(content: Content) -> some View {
        if #available(iOSApplicationExtension 17.0, macOSApplicationExtension 14.0, *) {
            if isAccessory {
                content.containerBackground(.fill.tertiary, for: .widget)
            } else {
                content.containerBackground(for: .widget) {
                    ZStack {
                        Rectangle().fill(.background)
                        LinearGradient(
                            colors: [brand.opacity(0.13), brand.opacity(0.03), Color.clear],
                            startPoint: .topLeading,
                            endPoint: .bottomTrailing
                        )
                    }
                }
            }
        } else {
            legacy(content)
        }
    }
}

struct SytueWidgetView: View {
    @Environment(\.widgetFamily) var family
    let entry: SytueWidgetEntry
    let configuration = SytueWidgetConfiguration()

    private var isExtraLarge: Bool {
        #if os(iOS)
        return family == .systemExtraLarge
        #else
        if #available(macOSApplicationExtension 14.0, *) {
            return family == .systemExtraLarge
        }
        return false
        #endif
    }

    private var storagePercent: Int64? {
        entry.snapshot?.counters.first(where: { $0.kind == "storage_percent" })?.value
    }

    private var nonStorageCounters: [SytueWidgetCounter] {
        (entry.snapshot?.counters ?? []).filter { $0.kind != "storage_percent" }
    }

    private func headerBar(compact: Bool = false, showActions: Bool = false) -> some View {
        HStack(spacing: 7) {
            Image("Icon")
                .resizable()
                .scaledToFit()
                .frame(width: compact ? 20 : 22, height: compact ? 20 : 22)
                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                .accessibilityHidden(true)
            Text(configuration.name)
                .font(compact ? .subheadline.weight(.semibold) : .headline)
                .lineLimit(1)
                .minimumScaleFactor(0.75)
            Spacer(minLength: 4)
            if let badgeCounter = nonStorageCounters.first {
                Text(badgeCounter.value, format: .number.notation(.compactName))
                    .font(.system(size: 10.5, weight: .bold, design: .rounded))
                    .foregroundStyle(configuration.brandColor)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(configuration.brandColor.opacity(0.15))
                    .clipShape(Capsule())
                    .sytueAccentable()
                    .privacySensitive()
            }
            if showActions {
                SytueQuickActionsBar(configuration: configuration, iconOnly: true)
            }
        }
    }

    // MARK: - Graphical Calendar Views

    private var calendarSmall: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 4) {
                Text(SytueWidgetDates.monthTitle(for: entry.date))
                    .font(.system(size: 11, weight: .bold, design: .rounded))
                    .foregroundStyle(configuration.brandColor)
                    .lineLimit(1)
                    .minimumScaleFactor(0.75)
                    .sytueAccentable()
                Spacer(minLength: 2)
                Image("Icon")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 15, height: 15)
                    .clipShape(RoundedRectangle(cornerRadius: 3.5, style: .continuous))
                    .accessibilityHidden(true)
            }
            SytueCalendarMonthGridView(
                date: entry.date,
                calendarData: entry.snapshot?.calendar,
                brand: configuration.brandColor,
                compact: true
            )
            if let nextEvent = entry.items.first {
                Spacer(minLength: 0)
                HStack(spacing: 4) {
                    Circle()
                        .fill(SytueWidgetPalette.color(hex: nextEvent.colorText, fallback: configuration.brandColor))
                        .frame(width: 5, height: 5)
                        .sytueAccentable()
                    Text(nextEvent.title)
                        .font(.system(size: 10, weight: .semibold))
                        .lineLimit(1)
                }
                .privacySensitive()
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private var calendarMedium: some View {
        let items = entry.items
        return HStack(alignment: .top, spacing: 12) {
            VStack(alignment: .leading, spacing: 4) {
                Text(SytueWidgetDates.monthTitle(for: entry.date))
                    .font(.system(size: 11, weight: .bold, design: .rounded))
                    .foregroundStyle(configuration.brandColor)
                    .lineLimit(1)
                    .sytueAccentable()
                SytueCalendarMonthGridView(
                    date: entry.date,
                    calendarData: entry.snapshot?.calendar,
                    brand: configuration.brandColor,
                    compact: true
                )
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)

            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    Image("Icon")
                        .resizable()
                        .scaledToFit()
                        .frame(width: 18, height: 18)
                        .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                        .accessibilityHidden(true)
                    Text(SytueWidgetDates.shortDayHeader(for: entry.date))
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .minimumScaleFactor(0.75)
                    Spacer(minLength: 2)
                    SytueQuickActionsBar(configuration: configuration, iconOnly: true)
                }

                if !items.isEmpty {
                    VStack(alignment: .leading, spacing: 5) {
                        ForEach(Array(items.prefix(3).enumerated()), id: \.offset) { _, item in
                            SytueCalendarEventRow(
                                item: item,
                                brand: configuration.brandColor,
                                referenceDate: entry.date,
                                showSubtitle: false
                            )
                        }
                    }
                } else if let counters = entry.snapshot?.counters, !counters.isEmpty {
                    Spacer(minLength: 0)
                    HStack(spacing: 12) {
                        ForEach(Array(counters.prefix(2)), id: \.kind) { counter in
                            SytueWidgetCounterView(counter: counter, brand: configuration.brandColor)
                        }
                    }
                    Spacer(minLength: 0)
                } else {
                    Spacer(minLength: 0)
                    Text(LocalizedStringKey(configuration.descriptionKey))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(3)
                    Spacer(minLength: 0)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private var calendarLarge: some View {
        let items = entry.items
        return VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image("Icon")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 22, height: 22)
                    .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                    .accessibilityHidden(true)
                Text(SytueWidgetDates.monthTitle(for: entry.date))
                    .font(.subheadline.weight(.bold))
                    .foregroundStyle(configuration.brandColor)
                    .lineLimit(1)
                    .sytueAccentable()
                Spacer(minLength: 4)
                SytueQuickActionsBar(configuration: configuration, iconOnly: false)
            }
            SytueCalendarMonthGridView(
                date: entry.date,
                calendarData: entry.snapshot?.calendar,
                brand: configuration.brandColor,
                compact: false
            )
            Divider()
            if !items.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    ForEach(Array(items.prefix(4).enumerated()), id: \.offset) { _, item in
                        SytueCalendarEventRow(
                            item: item,
                            brand: configuration.brandColor,
                            referenceDate: entry.date,
                            showSubtitle: true
                        )
                    }
                }
            } else if let counters = entry.snapshot?.counters, !counters.isEmpty {
                HStack(spacing: 16) {
                    ForEach(Array(counters.prefix(3)), id: \.kind) { counter in
                        SytueWidgetCounterView(counter: counter, brand: configuration.brandColor)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
            } else {
                Text(LocalizedStringKey(configuration.descriptionKey))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private var calendarExtraLarge: some View {
        let items = entry.items
        return HStack(alignment: .top, spacing: 18) {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 8) {
                    Image("Icon")
                        .resizable()
                        .scaledToFit()
                        .frame(width: 24, height: 24)
                        .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                        .accessibilityHidden(true)
                    Text(SytueWidgetDates.monthTitle(for: entry.date))
                        .font(.headline.weight(.bold))
                        .foregroundStyle(configuration.brandColor)
                        .sytueAccentable()
                }
                SytueCalendarMonthGridView(
                    date: entry.date,
                    calendarData: entry.snapshot?.calendar,
                    brand: configuration.brandColor,
                    compact: false
                )
                Spacer(minLength: 0)
                SytueQuickActionsBar(configuration: configuration, iconOnly: false)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)

            VStack(alignment: .leading, spacing: 7) {
                Text(SytueWidgetDates.shortDayHeader(for: entry.date))
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.secondary)
                if !items.isEmpty {
                    ForEach(Array(items.prefix(6).enumerated()), id: \.offset) { _, item in
                        SytueCalendarEventRow(
                            item: item,
                            brand: configuration.brandColor,
                            referenceDate: entry.date,
                            showSubtitle: true
                        )
                    }
                } else if let counters = entry.snapshot?.counters, !counters.isEmpty {
                    HStack(spacing: 16) {
                        ForEach(Array(counters.prefix(3)), id: \.kind) { counter in
                            SytueWidgetCounterView(counter: counter, brand: configuration.brandColor)
                        }
                    }
                } else {
                    Text(LocalizedStringKey(configuration.descriptionKey))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    @ViewBuilder private var calendarContent: some View {
        if isExtraLarge {
            calendarExtraLarge
        } else if family == .systemLarge {
            calendarLarge
        } else if family == .systemMedium {
            calendarMedium
        } else {
            calendarSmall
        }
    }

    // MARK: - General Visual Product Views (Mail, Drive, Notes, Rooms, Contacts, Docs, Sheets, Slides, PDF, Bases, ERP, Photos, Cast, Hub)

    private var productSmall: some View {
        let items = entry.items
        return VStack(alignment: .leading, spacing: 6) {
            headerBar(compact: true, showActions: false)
            if let pct = storagePercent {
                SytueStorageBarView(percent: pct, brand: configuration.brandColor, compact: true)
            }
            if !items.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(Array(items.prefix(storagePercent != nil ? 1 : 2).enumerated()), id: \.offset) { _, item in
                        SytueProductItemRow(
                            product: configuration.product,
                            item: item,
                            brand: configuration.brandColor,
                            referenceDate: entry.date,
                            showSubtitle: false
                        )
                    }
                }
                Spacer(minLength: 0)
            } else if let counter = nonStorageCounters.first {
                Spacer(minLength: 0)
                SytueWidgetCounterView(counter: counter, brand: configuration.brandColor)
                Spacer(minLength: 0)
            } else if storagePercent == nil {
                // Missing, locked, expired and signed-out states share one neutral
                // presentation. It never invents zero counts or an account state.
                Spacer(minLength: 0)
                Text(LocalizedStringKey(configuration.descriptionKey))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(3)
                Spacer(minLength: 0)
                SytueWidgetActionLabel(action: "open")
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(configuration.brandColor)
                    .sytueAccentable()
            } else {
                Spacer(minLength: 0)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private var productMedium: some View {
        let items = entry.items
        return VStack(alignment: .leading, spacing: 6) {
            headerBar(compact: false, showActions: true)
            if let pct = storagePercent {
                SytueStorageBarView(percent: pct, brand: configuration.brandColor, compact: true)
            }
            if !items.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(Array(items.prefix(storagePercent != nil ? 2 : 3).enumerated()), id: \.offset) { _, item in
                        SytueProductItemRow(
                            product: configuration.product,
                            item: item,
                            brand: configuration.brandColor,
                            referenceDate: entry.date,
                            showSubtitle: items.count <= 2
                        )
                    }
                }
                Spacer(minLength: 0)
            } else if !nonStorageCounters.isEmpty {
                Spacer(minLength: 0)
                HStack(alignment: .top, spacing: 14) {
                    ForEach(Array(nonStorageCounters.prefix(3)), id: \.kind) { counter in
                        SytueWidgetCounterView(counter: counter, brand: configuration.brandColor)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                Spacer(minLength: 0)
            } else if storagePercent == nil {
                Spacer(minLength: 0)
                Text(LocalizedStringKey(configuration.descriptionKey))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                Spacer(minLength: 0)
                SytueQuickActionsBar(configuration: configuration, iconOnly: false)
            } else {
                Spacer(minLength: 0)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private var productLarge: some View {
        let items = entry.items
        return VStack(alignment: .leading, spacing: 8) {
            headerBar(compact: false, showActions: true)
            if !nonStorageCounters.isEmpty {
                HStack(alignment: .top, spacing: 14) {
                    ForEach(Array(nonStorageCounters.prefix(3)), id: \.kind) { counter in
                        SytueWidgetCounterView(counter: counter, brand: configuration.brandColor, compact: !items.isEmpty)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
            }
            if let pct = storagePercent {
                SytueStorageBarView(percent: pct, brand: configuration.brandColor, compact: false)
            }
            if !items.isEmpty {
                Divider()
                VStack(alignment: .leading, spacing: 6) {
                    ForEach(Array(items.prefix(6).enumerated()), id: \.offset) { _, item in
                        SytueProductItemRow(
                            product: configuration.product,
                            item: item,
                            brand: configuration.brandColor,
                            referenceDate: entry.date,
                            showSubtitle: true
                        )
                    }
                }
                Spacer(minLength: 0)
            } else if entry.snapshot == nil {
                Spacer(minLength: 0)
                VStack(alignment: .leading, spacing: 10) {
                    Image(systemName: configuration.productSymbol)
                        .font(.system(size: 28, weight: .semibold))
                        .foregroundStyle(configuration.brandColor)
                        .sytueAccentable()
                    Text(LocalizedStringKey(configuration.descriptionKey))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(3)
                    SytueQuickActionsBar(configuration: configuration, iconOnly: false)
                }
                Spacer(minLength: 0)
            } else {
                Spacer(minLength: 0)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private var productExtraLarge: some View {
        let items = entry.items
        return HStack(alignment: .top, spacing: 18) {
            VStack(alignment: .leading, spacing: 10) {
                headerBar(compact: false, showActions: false)
                if !nonStorageCounters.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(Array(nonStorageCounters.prefix(3)), id: \.kind) { counter in
                            SytueWidgetCounterView(counter: counter, brand: configuration.brandColor)
                        }
                    }
                } else if entry.snapshot == nil {
                    Text(LocalizedStringKey(configuration.descriptionKey))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(4)
                }
                if let pct = storagePercent {
                    SytueStorageBarView(percent: pct, brand: configuration.brandColor, compact: false)
                }
                Spacer(minLength: 0)
                SytueQuickActionsBar(configuration: configuration, iconOnly: false)
            }
            .frame(maxWidth: 200, maxHeight: .infinity, alignment: .topLeading)

            VStack(alignment: .leading, spacing: 6) {
                if !items.isEmpty {
                    ForEach(Array(items.prefix(6).enumerated()), id: \.offset) { _, item in
                        SytueProductItemRow(
                            product: configuration.product,
                            item: item,
                            brand: configuration.brandColor,
                            referenceDate: entry.date,
                            showSubtitle: true
                        )
                    }
                }
                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    @ViewBuilder private var regularContent: some View {
        if configuration.product == "calendar" {
            calendarContent
        } else if isExtraLarge {
            productExtraLarge
        } else if family == .systemLarge {
            productLarge
        } else if family == .systemMedium {
            productMedium
        } else {
            productSmall
        }
    }

    // MARK: - iOS Lock Screen Accessory Views

    #if os(iOS)
    @available(iOSApplicationExtension 16.0, *)
    @ViewBuilder private var accessoryContent: some View {
        switch family {
        case .accessoryInline:
            if let firstItem = entry.items.first {
                Label(firstItem.title, systemImage: configuration.productSymbol)
                    .privacySensitive()
            } else if let counter = entry.snapshot?.counters.first {
                Label {
                    Text(counter.kind == "storage_percent" ? "\(counter.value)%" : "\(counter.value)")
                } icon: {
                    Image(systemName: configuration.productSymbol)
                }
                .privacySensitive()
            } else {
                Label(configuration.name, systemImage: configuration.productSymbol)
            }
        case .accessoryCircular:
            ZStack {
                AccessoryWidgetBackground()
                if configuration.product == "calendar" {
                    let day = Calendar.autoupdatingCurrent.component(.day, from: entry.date)
                    VStack(spacing: 0) {
                        Image(systemName: "calendar")
                            .font(.system(size: 10, weight: .semibold))
                        Text("\(day)")
                            .font(.system(size: 18, weight: .bold, design: .rounded))
                    }
                } else if let counter = entry.snapshot?.counters.first {
                    VStack(spacing: 1) {
                        Image(systemName: configuration.productSymbol)
                            .font(.system(size: 11, weight: .semibold))
                        Text(counter.value, format: .number.notation(.compactName))
                            .font(.system(size: 14, weight: .bold, design: .rounded))
                            .privacySensitive()
                    }
                } else {
                    Image(systemName: configuration.productSymbol)
                        .font(.title3)
                }
            }
        default:
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Image(systemName: configuration.productSymbol)
                        .font(.caption.weight(.semibold))
                    Text(configuration.product == "calendar"
                         ? SytueWidgetDates.shortDayHeader(for: entry.date)
                         : configuration.name)
                        .font(.headline)
                        .lineLimit(1)
                }
                if !entry.items.isEmpty {
                    ForEach(Array(entry.items.prefix(2).enumerated()), id: \.offset) { _, item in
                        Text(item.title)
                            .font(.caption)
                            .lineLimit(1)
                            .privacySensitive()
                    }
                } else if let counter = entry.snapshot?.counters.first {
                    SytueWidgetCounterView(counter: counter, brand: configuration.brandColor, compact: true)
                } else {
                    SytueWidgetActionLabel(action: "open").font(.caption)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
    #endif

    // El contenido con, debajo, de cuándo es el dato si ya no es fresco. Con la lectura
    // protegida, el marco con el marcador: no se sabe qué hay, no «no hay nada».
    @ViewBuilder private var framedContent: some View {
        VStack(alignment: .leading, spacing: 2) {
            regularContent
            if let stale = entry.staleLabel {
                Text(stale)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
            }
        }
        .redacted(reason: entry.isProtected ? .placeholder : [])
    }

    @ViewBuilder var body: some View {
        #if os(iOS)
        if #available(iOSApplicationExtension 16.0, *),
           [.accessoryRectangular, .accessoryCircular, .accessoryInline].contains(family) {
            accessoryContent
                .redacted(reason: entry.isProtected ? .placeholder : [])
                .modifier(SytueWidgetBackground(brand: configuration.brandColor))
                .widgetURL(configuration.actions["open"])
        } else {
            framedContent
                .modifier(SytueWidgetBackground(brand: configuration.brandColor))
                .widgetURL(configuration.actions["open"])
        }
        #else
        framedContent
            .modifier(SytueWidgetBackground(brand: configuration.brandColor))
            .widgetURL(configuration.actions["open"])
        #endif
    }
}

struct SytueActionWidgetView: View {
    @Environment(\.widgetFamily) var family
    let action: String
    let configuration = SytueWidgetConfiguration()

    private var title: some View {
        SytueWidgetActionLabel(action: action).lineLimit(2).minimumScaleFactor(0.8)
    }

    @ViewBuilder private var content: some View {
        #if os(iOS)
        if #available(iOSApplicationExtension 16.0, *), family == .accessoryInline {
            SytueWidgetActionLabel(action: action)
        } else if #available(iOSApplicationExtension 16.0, *), family == .accessoryCircular {
            ZStack {
                AccessoryWidgetBackground()
                Image(systemName: SytueWidgetActions.symbol(for: action)).font(.title2)
            }
            .accessibilityLabel(Text(LocalizedStringKey(SytueWidgetActions.label(for: action))))
        } else if #available(iOSApplicationExtension 16.0, *), family == .accessoryRectangular {
            VStack(alignment: .leading, spacing: 2) {
                Text(configuration.name).font(.headline).lineLimit(1)
                title.font(.caption)
            }
        } else {
            regular
        }
        #else
        regular
        #endif
    }

    private var regular: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image("Icon")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 24, height: 24)
                    .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                    .accessibilityHidden(true)
                Text(configuration.name)
                    .font(.headline)
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
            }
            Spacer(minLength: 0)
            HStack(spacing: 8) {
                ZStack {
                    Circle()
                        .fill(configuration.brandColor.opacity(0.16))
                        .frame(width: 32, height: 32)
                    Image(systemName: SytueWidgetActions.symbol(for: action))
                        .font(.system(size: 14, weight: .bold))
                        .foregroundStyle(configuration.brandColor)
                        .sytueAccentable()
                }
                Text(LocalizedStringKey(SytueWidgetActions.label(for: action)))
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(2)
                    .minimumScaleFactor(0.8)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }

    var body: some View {
        content
            .modifier(SytueWidgetBackground(brand: configuration.brandColor))
            .widgetURL(configuration.actions[action])
    }
}

private func summaryFamilies() -> [WidgetFamily] {
    #if os(iOS)
    if #available(iOSApplicationExtension 16.0, *) {
        return [
            .systemSmall, .systemMedium, .systemLarge, .systemExtraLarge,
            .accessoryRectangular, .accessoryCircular, .accessoryInline,
        ]
    }
    return [.systemSmall, .systemMedium, .systemLarge, .systemExtraLarge]
    #else
    if #available(macOSApplicationExtension 14.0, *) {
        return [.systemSmall, .systemMedium, .systemLarge, .systemExtraLarge]
    }
    return [.systemSmall, .systemMedium, .systemLarge]
    #endif
}

private func actionFamilies() -> [WidgetFamily] {
    #if os(iOS)
    if #available(iOSApplicationExtension 16.0, *) {
        return [.systemSmall, .systemMedium, .accessoryCircular, .accessoryRectangular, .accessoryInline]
    }
    #endif
    return [.systemSmall, .systemMedium]
}

struct SytueSummaryWidget: Widget {
    private let configuration = SytueWidgetConfiguration()

    init() {}

    var body: some WidgetConfiguration {
        // Keep the installed kind stable when adding action widgets to the bundle.
        StaticConfiguration(kind: configuration.bundle + ".widgets", provider: SytueWidgetProvider()) { entry in
            SytueWidgetView(entry: entry)
        }
        .configurationDisplayName(Text(configuration.name))
        .description(Text(LocalizedStringKey(configuration.descriptionKey)))
        .supportedFamilies(summaryFamilies())
    }
}

private func actionConfiguration(_ action: String) -> some WidgetConfiguration {
    let configuration = SytueWidgetConfiguration()
    return StaticConfiguration(kind: configuration.bundle + ".widgets." + action, provider: SytueWidgetActionProvider()) { _ in
        SytueActionWidgetView(action: action)
    }
    .configurationDisplayName(Text(LocalizedStringKey(SytueWidgetActions.label(for: action))))
    .description(Text(configuration.name))
    .supportedFamilies(actionFamilies())
}

// WidgetKit requires a parameterless init for each widget type. Keep distinct
// types for the actions while sharing their provider, view and configuration.
struct SytueSearchWidget: Widget {
    var body: some WidgetConfiguration {
        actionConfiguration("search")
    }
}

struct SytueComposeWidget: Widget {
    var body: some WidgetConfiguration {
        actionConfiguration("compose")
    }
}

struct SytueCreateWidget: Widget {
    var body: some WidgetConfiguration {
        actionConfiguration("create")
    }
}

@main
struct SytueWidgets: WidgetBundle {
    var body: some Widget {
        SytueSummaryWidget()
        // The signed target gets these flags from its FyneApp.toml. WidgetBundle
        // only permits availability conditions, not runtime metadata conditions.
        #if SYTUE_WIDGET_ACTION_SEARCH
        SytueSearchWidget()
        #endif
        #if SYTUE_WIDGET_ACTION_COMPOSE
        SytueComposeWidget()
        #endif
        #if SYTUE_WIDGET_ACTION_CREATE
        SytueCreateWidget()
        #endif
    }
}
