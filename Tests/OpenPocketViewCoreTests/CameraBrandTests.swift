import Testing

@testable import OpenPocketViewCore

@Suite struct CameraBrandTests {
    @Test func nanoResolvesFromIdOrName() {
        #expect(CameraModel.resolve(modelId: 0x0019, name: "Renamed camera") == .default)
        #expect(CameraModel.resolve(modelId: nil, name: "OsmoNano-TEST") == .default)
        #expect(CameraModel.default.liveViewEnableReceiver == 0x41)
        #expect(CameraModel.default.usesNanoLiveViewGate)
        #expect(!CameraModel.default.hasGimbal)
        #expect(!CameraModel.default.supportsTapFocus)
        #expect(CameraModel.default.zoomStops == [1])
    }

    @Test func unsupportedIdsCannotBecomeNanoThroughTheirNames() {
        for id in [0x0010, 0x0015, 0x0017, 0x0020, 0x0021, 0x0022, 0x0070, 0x007E] {
            let model = CameraModel.resolve(modelId: id, name: "OsmoNano-TEST")
            #expect(model.family == .other)
            #expect(!model.usesCapturedLiveEnable)
        }
    }

    @Test func unknownAndRebadgedDevicesAreExcluded() {
        #expect(CameraModel.resolve(modelId: 0x0019, name: "Xtra Atto").family == .other)
        for name in ["", "DJI camera", "Osmo Pocket 4 Pro", "OsmoAction6", "Xtra Atto"] {
            #expect(CameraModel.resolve(modelId: nil, name: name).family == .other)
        }
        let brand = CameraBrand.of(address: "EC:9E:EA:00:00:00", name: "OsmoNano", djiCid: true)
        #expect(
            CameraModel.resolve(modelId: 0x0019, name: "OsmoNano", brand: brand).family == .other)
    }
}
