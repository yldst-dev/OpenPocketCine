import Foundation

public enum CamCapShutter {
    public static let subscribeKey = "camcap_shutter"

    /// 1/N denoms in camera order. Empty if the blob is not a shutter table.
    public static func parseDenoms(_ value: [UInt8]) -> [Int] {
        guard let items = parseItems(value) else { return [] }
        var seen = Set<Int>()
        var out: [Int] = []
        for item in items {
            guard case .fraction(let denom) = item else { continue }
            guard (1...16_000).contains(denom), !seen.contains(denom) else { continue }
            seen.insert(denom)
            out.append(denom)
        }
        return out
    }

    /// Wheel options: camera list only. Until a cap push lands, show the live value.
    public static func wheelDenoms(available: [Int], current: Int) -> [Int] {
        if !available.isEmpty { return available }
        return (1...16_000).contains(current) ? [current] : []
    }

    public static func nearestDenom(_ current: Int, in denoms: [Int]) -> Int? {
        guard !denoms.isEmpty else { return nil }
        return denoms.min(by: { abs($0 - current) < abs($1 - current) })
    }

    /// Next / previous 1/N in camera order. nil at the end.
    public static func steppedDenom(from current: Int, steps: Int, available: [Int]) -> Int? {
        let list = wheelDenoms(available: available, current: current)
        let idx =
            list.firstIndex(of: current)
            ?? list.firstIndex(of: nearestDenom(current, in: list) ?? -1)
        guard let idx else { return nil }
        let next = idx + steps
        guard list.indices.contains(next), next != idx else { return nil }
        return list[next]
    }

    public static func label(_ denom: Int) -> String { "1/\(denom)" }

    public static func denom(from label: String) -> Int? {
        Int(label.replacingOccurrences(of: "1/", with: ""))
    }

    enum Item: Equatable, Sendable {
        case fraction(Int)
        case seconds(Int)
    }

    static func parseItems(_ value: [UInt8]) -> [Item]? {
        guard value.count >= 13, value[0] == 0x01 else { return nil }
        let inner = Int(value[1]) | (Int(value[2]) << 8)
        guard inner >= 10, 3 + inner <= value.count else { return nil }
        let body = Array(value[3..<(3 + inner)])
        if let items = itemsFromHeader(body) { return items }
        return scanItems(body)
    }

    private static func itemsFromHeader(_ body: [UInt8]) -> [Item]? {
        let count = Int(body[9])
        let payload = Array(body.dropFirst(10))
        guard count > 0, payload.count == count * 3 else { return nil }
        let items = decodeTriplets(payload)
        return items.isEmpty ? nil : items
    }

    /// Firmware that shifts the 10-byte header: take the densest run of 3-byte records.
    private static func scanItems(_ body: [UInt8]) -> [Item]? {
        var best: [Item] = []
        let maxOff = min(16, max(0, body.count - 6))
        for off in 0...maxOff {
            let slice = Array(body[off...])
            let n = slice.count / 3
            guard n >= 2 else { continue }
            let items = decodeTriplets(Array(slice.prefix(n * 3)))
            if fractionCount(items) > fractionCount(best) {
                best = items
            }
        }
        return fractionCount(best) >= 2 ? best : nil
    }

    private static func fractionCount(_ items: [Item]) -> Int {
        items.reduce(0) { count, item in
            if case .fraction = item { return count + 1 }
            return count
        }
    }

    private static func decodeTriplets(_ bytes: [UInt8]) -> [Item] {
        var items: [Item] = []
        var i = 0
        while i + 2 < bytes.count {
            let raw = Int(bytes[i]) | (Int(bytes[i + 1]) << 8)
            if raw & 0x8000 != 0 {
                let denom = raw & 0x7FFF
                if (1...16_000).contains(denom) {
                    items.append(.fraction(denom))
                }
            } else if (1...60).contains(raw) {
                items.append(.seconds(raw))
            }
            i += 3
        }
        return items
    }
}

/// `camcap_iso` — legal ISO indices for the current color / mode.
///
/// ```
/// 01 | innerLen:u16-LE | 00 | count:u8 | count × index
/// ```
/// D-Log2 example: `01 08 00 00 06 03 04 05 06 07 08` → 100…3200.
public enum CamCapIso {
    public static let subscribeKey = "camcap_iso"

    public static func parseIndices(_ value: [UInt8]) -> [IsoIndex] {
        guard value.count >= 5, value[0] == 0x01 else { return [] }
        let inner = Int(value[1]) | (Int(value[2]) << 8)
        guard inner >= 2, 3 + inner <= value.count else { return [] }
        let body = Array(value[3..<(3 + inner)])
        let count = Int(body[1])
        guard body[0] == 0, count >= 1, body.count >= 2 + count else { return [] }
        return body[2..<(2 + count)].compactMap { IsoIndex(rawValue: $0) }
    }

    public static func wheelIndices(available: [IsoIndex], fallback: [IsoIndex]) -> [IsoIndex] {
        available.isEmpty ? fallback : available
    }

    /// Native base ISO for the operator's current transfer. Decoration only —
    /// the wheel list stays `camcap_iso`.
    ///
    /// D-Log = 400, D-Log2 = 1600. Rec.709 / HLG have no published Pocket
    /// native base (OpenZCine stars Nikon 800/6400; do not invent a third).
    /// Uses `CameraStatus.monitorTransfer` (`colorMode` `@2`), not a tele hop SET.
    public static func baseISO(transfer: MonitorTransfer?) -> Int? {
        switch transfer {
        case .dlog: 400
        case .dlog2: 1600
        case .rec709, .hdr, .dlogm, nil: nil
        }
    }

    public static func baseISO(colorMode: ColorMode?) -> Int? {
        switch colorMode {
        case .dLog: 400
        case .dLog2: 1600
        default: nil
        }
    }

    public static func markedLabels(transfer: MonitorTransfer?) -> Set<String> {
        guard let iso = baseISO(transfer: transfer) else { return [] }
        return ["\(iso)"]
    }

    public static func markedLabels(colorMode: ColorMode?) -> Set<String> {
        markedLabels(transfer: colorMode.map(MonitorTransfer.init))
    }

    /// If the operator is still on `from`'s native ISO, hop to `to`'s native.
    /// Off-base or Auto stays put. Rec.709 / HDR have no native — no hop.
    /// `hopEnabled` is the ISO-sheet Native ISO toggle (default on).
    public static func nativeISOHop(
        from: ColorMode?, to: ColorMode, current: IsoIndex?,
        hopEnabled: Bool = true
    ) -> IsoIndex? {
        guard hopEnabled else { return nil }
        guard let from, from != to else { return nil }
        guard let fromBase = baseISO(colorMode: from),
            let toBase = baseISO(colorMode: to)
        else { return nil }
        guard let current, current != .auto, current.isoValue == fromBase else { return nil }
        return to.isoIndices.first { $0.isoValue == toBase }
    }
}

public enum CamCapColorMode {
    public static let subscribeKey = "camcap_color_mode"

    public static func parse(_ value: [UInt8], model: CameraModel? = nil) -> [ColorMode] {
        guard value.count >= 5, value[0] == 0x01 else { return [] }
        let inner = Int(value[1]) | (Int(value[2]) << 8)
        guard inner >= 2, 3 + inner <= value.count else { return [] }
        let body = Array(value[3..<(3 + inner)])
        let count = Int(body[0])
        guard count >= 1, body.count >= 1 + count else { return [] }
        return body[1..<(1 + count)].compactMap { ColorMode.fromWire($0, model: model) }
    }

    public static func wheel(
        available: [ColorMode], family: CameraBodyFamily
    ) -> [ColorMode] {
        wheel(available: available, order: ColorMode.available(for: family))
    }

    public static func wheel(available: [ColorMode], model: CameraModel) -> [ColorMode] {
        wheel(available: available, order: ColorMode.available(for: model))
    }

    /// Body order is the legal set. `camcap_color_mode` may subset it.
    /// It cannot add D-Log2 (or `0x17` D-Log) to a body that does not have them.
    private static func wheel(available: [ColorMode], order: [ColorMode]) -> [ColorMode] {
        guard !available.isEmpty else { return order }
        let have = Set(available)
        let ranked = order.filter { have.contains($0) }
        return ranked.isEmpty ? order : ranked
    }
}

public enum CamCapVideoFormat {
    public static let subscribeKey = "camcap_video_format"

    public static func pickerFormats(
        available: [VideoFormat], model: CameraModel?, shootingMode: Int
    ) -> [VideoFormat] {
        available
    }

    public static func allowsOperatorSet(
        _ format: VideoFormat,
        available: [VideoFormat],
        model: CameraModel?,
        shootingMode: Int
    ) -> Bool {
        let legal = pickerFormats(
            available: available, model: model, shootingMode: shootingMode)
        return !legal.isEmpty && legal.contains(format)
    }

    public static func parse(_ value: [UInt8]) -> [VideoFormat] {
        guard value.count >= 5, value[0] == 0x01 else { return [] }
        let inner = Int(value[1]) | (Int(value[2]) << 8)
        guard inner >= 2, 3 + inner <= value.count else { return [] }
        let body = Array(value[3..<(3 + inner)])
        let count = Int(body[0])
        guard count >= 1, body.count >= 1 + count * 3 else { return [] }
        var out: [VideoFormat] = []
        var seen = Set<VideoFormat>()
        var i = 1
        for _ in 0..<count {
            guard i + 2 < body.count else { break }
            if let format = VideoFormat.parseVideoParamV2(Array(body[i..<(i + 2)])),
                seen.insert(format).inserted
            {
                out.append(format)
            }
            i += 3
        }
        return out
    }

    public static func resolutions(
        available: [VideoFormat], current: VideoResolution?
    ) -> [VideoResolution] {
        resolutions(available: available, aspect: nil, current: current)
    }

    public static func resolutions(
        available: [VideoFormat], aspect: VideoAspect?, current: VideoResolution?
    ) -> [VideoResolution] {
        if available.isEmpty {
            // Read-only current pair. Do not offer 1080/4K tabs that would SET
            // a different resolution at the live fps (1080 240 → 4K 240).
            guard let current, aspect == nil || current.aspect == aspect else { return [] }
            return [current]
        }
        var seen = Set<VideoResolution>()
        var out: [VideoResolution] = []
        for format in available {
            if let aspect, format.resolution.aspect != aspect { continue }
            if seen.insert(format.resolution).inserted {
                out.append(format.resolution)
            }
        }
        if let current, !seen.contains(current), aspect == nil || current.aspect == aspect {
            out.insert(current, at: 0)
        }
        return out
    }

    public static func aspects(
        available: [VideoFormat], current: VideoAspect?
    ) -> [VideoAspect] {
        if available.isEmpty {
            return current.map { [$0] } ?? []
        }
        var seen = Set<VideoAspect>()
        var out: [VideoAspect] = []
        for format in available {
            guard let aspect = format.resolution.aspect else { continue }
            if seen.insert(aspect).inserted { out.append(aspect) }
        }
        if let current, !seen.contains(current) {
            out.insert(current, at: 0)
        }
        return out
    }

    public static func frameRates(
        available: [VideoFormat], resolution: VideoResolution, current: VideoFrameRate?
    ) -> [VideoFrameRate] {
        let rates = available.filter { $0.resolution == resolution }.map(\.frameRate)
        if rates.isEmpty {
            return current.map { [$0] } ?? []
        }
        return rates
    }
}

public enum CamCapIsoAutoMax {
    public static let subscribeKey = "camcap_iso_auto_max"

    public static func parse(_ value: [UInt8]) -> (base: Int, limits: [IsoLimit])? {
        guard !value.isEmpty else { return nil }
        if value[0] == 0x01 {
            return (0, [])
        }
        guard value.count >= 6, value[0] == 0x02 else { return nil }
        let inner = Int(value[1]) | (Int(value[2]) << 8)
        guard inner >= 3, 3 + inner <= value.count else { return nil }
        let body = Array(value[3..<(3 + inner)])
        let count = Int(body[0])
        guard count >= 1, body.count >= 1 + count + 2 else { return nil }
        let limits = body[1..<(1 + count)].compactMap { IsoLimit(rawValue: $0) }
        guard limits.count == count else { return nil }
        let baseAt = 1 + count
        let base = Int(UInt16(body[baseAt]) | (UInt16(body[baseAt + 1]) << 8))
        return (base, limits)
    }
}
