---
title: Standalone Go monitor
description: Video-only Osmo Nano monitoring on a shared IPv4 network.
---

`Apps/NanoMonitor/` is an independent Go module. It receives Nano AVC preview
and opens a video-only FFplay window. The viewer does not depend on the mobile apps. macOS Bluetooth setup uses
CoreBluetooth and requires Xcode Command Line Tools with cgo enabled; it does
not require an Apple development account. Other platforms support viewing
already configured cameras with `CGO_ENABLED=0`.

## Prepare the camera

Connect the computer to the target WPA2 router, attach Nano to its powered
vision dock and finish initial DJI Mimo activation if needed. Disconnect Mimo.
On macOS, configure the camera from the repository root:

```sh
just nano-monitor setup -list
just nano-monitor setup -interface en0 -monitor
```

Use the router-connected interface. With several cameras, add `-device` using
the listed name or Bluetooth ID. Enter the network name and password in the
native dialogs, then approve Nano's connection request if shown. The password
is hidden and is not saved in command arguments, logs or files.

Setup checks the same Nano on the LAN before opening the viewer. If its address
is known from the router, add `-camera 192.168.10.42`. A failed check can leave
Nano in station mode: check client isolation, the interface and macOS Local
Network permission. To request its own Wi-Fi again, run
`just nano-monitor setup -restore-ap` and verify restoration on Nano. A request
acceptance is not proof that the mode change completed.

Keep Nano in Video mode and disconnect other camera apps. If Multiview was used
for provisioning, remove the tile before closing that workflow so the camera
remains on the shared network. Obtain its address from the router or use the
bounded candidate search below. Guest/client isolation must be disabled.

## Run

Install Go 1.26 or newer and FFplay. The Go module itself has no external Go
dependencies. From the repository root:

```sh
just nano-monitor-check
just nano-monitor -camera 192.168.10.42
```

If macOS blocks the bare executable, launch its app bundle from the module:

```sh
cd Apps/NanoMonitor
just mac-run -interface en0 -camera 192.168.10.42
```

Allow Local Network access if prompted. The app keeps route/permission failures
visible for 20 seconds; after granting access, run it again. Logs are saved to
`bin/monitor.log`. Its README explains optional Apple-issued signing for stable
permission tracking across builds. Do not run several viewers for one camera.

For candidate discovery and an explicit network interface:

```sh
just nano-monitor -discover -interface en0
just nano-monitor -interface en0 -camera 192.168.10.42 -name OsmoNano-TEST
```

Replace the example address and name. `-name` refers to the camera's own name,
not the router SSID. The supported identity format starts with `OsmoNano`.
The name check is not cryptographic device authentication.

Esc closes the viewer, F toggles fullscreen and Ctrl+C cancels the terminal
session. No footage is recorded to disk and no shooting-setting commands are
provided. Only 1 monitor should connect to a camera at a time.

The program can also be copied out of the repository and built from its own
folder with `just build` or `go build -o bin/nano-monitor ./cmd/nano-monitor`.
Its README describes the domain, application ports and platform adapters.

## Validation scope

The automated checks cover framing, fragmented video, recovery policy,
cancellation and subprocess handling. A short physical Nano-on-router run displayed real AVC video through the
macOS app bundle. Long-run stability, latency and AP restoration are still
unverified. The viewer waits for the camera’s preview preparation reply before
sending the enable command. After 2 unsuccessful picture requests, the program preserves the player and
tries a fresh, identity-checked connection. At most 2 new connections are
allowed per run; exhaustion still ends with an error. It does not change Wi-Fi
settings. Old retransmissions no longer rewind ACKs or discard a current frame.

The player uses packet-arrival timestamps, skips stream-info probing and uses
1 decoding thread. Its compressed-frame queue holds at most 8 units. It does
not replay received data against an invented recording timeline or discard
H.264 reference frames to catch up. Late decoded pictures can be dropped.
The operator confirmed near-immediate response in the physical check. The
synthetic burst test checks backlog drainage, not camera-to-screen delay.
