package com.opencapture.openpocketcine.session

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

class NanoProfileTest {
    @Test fun defaultsMatchNanoWireProfile() {
        val model = CameraModel.default
        assertEquals("nano", model.family)
        assertEquals(0x41, model.liveViewEnableReceiver)
        assertEquals(listOf(1.0), model.zoomStops)
        assertFalse(model.hasGimbal)
        assertFalse(model.supportsFocusMode)
        assertEquals(100, model.isoAutoRangeFloor)
    }

    @Test fun colorWireValuesRoundTrip() {
        for ((mode, wire) in listOf(
            CameraCommands.COLOR_NORMAL to 0x00,
            CameraCommands.COLOR_NORMAL10 to 0x3F,
            CameraCommands.COLOR_DLOG_M to 0x3D,
        )) {
            assertEquals(wire, CameraCommands.wireColorMode(mode))
            assertEquals(mode, CameraCommands.parseColorMode(wire))
        }
        assertEquals(-1, CameraCommands.parseColorMode(0x41))
    }

    @Test fun photoUsesNanoEncodingOnly() {
        assertEquals(0x05, CameraCommands.photoModeRaw("Osmo Nano"))
        assertFalse(CameraCommands.isPhotoMode(0x17))
        assertFalse(CameraCommands.isPhotoMode(0x4D))
    }

    @Test fun missingModelJsonIsNotTreatedAsNano() {
        assertEquals("other", CameraModel.fromJson(null).family)
        assertEquals("other", CameraModel.fromJson("{}").family)
    }
}
