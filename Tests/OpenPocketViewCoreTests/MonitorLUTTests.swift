import Foundation
import Testing

@testable import OpenPocketViewCore

@Suite struct MonitorLUTTests {
    @Test func nanoCatalogContainsOnlyNanoConversion() {
        #expect(LUTSelection.djiCases == [.djiAuto, .djiDLogM])
        #expect(OfficialDJILUT.allCases == [.nanoDLogM])
    }

    @Test func autoFollowsNanoColor() {
        for mode in [ColorMode.normal, .normal10] {
            #expect(OfficialDJILUT.auto(colorMode: mode, family: .nano) == nil)
        }
        #expect(OfficialDJILUT.auto(colorMode: .dLogM, family: .nano) == .nanoDLogM)
        #expect(OfficialDJILUT.auto(colorMode: .dLogM, family: .other) == nil)
    }

    @Test func removedLooksDoNotLoadMissingResources() {
        for selection in [LUTSelection.officialDLog, .officialDLog2, .djiDLog, .djiDLog2] {
            #expect(
                LUTResolver.resolve(
                    selection: selection, colorMode: .dLogM,
                    hasCustomDLog: false, hasCustomDLog2: false) == .off)
        }
    }

    @Test func creativeAndCustomLooksRemainAvailable() {
        #expect(
            LUTResolver.resolve(
                selection: .creativeMono, colorMode: .normal,
                hasCustomDLog: false, hasCustomDLog2: false) == .creative(.mono))
        #expect(
            LUTResolver.resolve(
                selection: .customFile, colorMode: .normal,
                hasCustomDLog: false, hasCustomDLog2: false,
                customFileName: "../escape.cube") == .off)
    }

    @Test func nanoCubeShipsIdenticallyInBothShells() throws {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let file = OfficialDJILUT.nanoDLogM.fileName
        let ios = try Data(
            contentsOf: root.appendingPathComponent("ios/OpenPocketCine/Resources/" + file))
        let android = try Data(
            contentsOf: root.appendingPathComponent("Apps/Android/app/src/main/assets/luts/" + file)
        )
        #expect(ios == android)
        let cube = try CubeLUT.parse(String(decoding: ios, as: UTF8.self))
        #expect(cube.size == 33)
    }
}
