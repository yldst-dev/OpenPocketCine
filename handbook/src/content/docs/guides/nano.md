---
title: Osmo Nano
description: Supported camera profile and controls for the Nano-only branch.
---

This branch supports **DJI Osmo Nano only** on iOS and Android. Pocket, Action,
360, drone and Xtra profiles are excluded from discovery and connection. Older
protocol surveys and release notes remain historical references.

## Connection and picture

Pair over Bluetooth, read the camera Wi-Fi credentials, join its Wi-Fi and open
the UDP datalink. Nano uses model ID `0x0019`, live-enable receiver `0x41`, and
its `0x02/0x09` live-view gate. Enable-once and watchdog recovery are retained.
The AVC stream still uses length-based assembly across transport groups and
the private metadata handling described in [Live view](../protocol/live-view/).

Saved non-Nano cameras do not appear in the connection list. Nano credentials
remain in the existing platform credential stores. Renamed Nano cameras can be
recognized by their model ID; an unknown model is not assumed to be Nano.

## Controls and looks

- Video recording and Photo shutter control remain available.
- Photo uses `0x05`; Pocket photo `0x17` and Live Photo `0x4D` are not offered.
- Format changes require the camera's reported capability table.
- Color options are Normal 8-bit, Normal 10-bit and D-Log M.
- The bundled manufacturer conversion is Nano D-Log M to Rec.709.
- Creative looks, imported LUTs, monitoring assists and media playback remain.
- Nano has no gimbal, autofocus or camera zoom controls.

Application and package identifiers remain OpenPocketCine so the existing
platform builds and saved Nano settings continue to use the same identifiers.

## Verification

Run `just check`, `just native-check` and `just android-check`. Before release,
verify pairing, reconnect, AVC picture, recording start/stop, Photo and color
changes with a physical Nano and a real phone for each platform. Simulator and
unit tests cannot prove the Bluetooth or camera Wi-Fi path.

Physical regression of this Nano-only branch is pending. Earlier multi-model
qualification does not qualify this refactor.
