---
title: Standalone Go monitor
description: Video-only Osmo Nano monitoring on a shared IPv4 network.
---

`Apps/NanoMonitor/` is an independent Go module. It receives Nano AVC preview
and opens a video-only FFplay window. Swift, Xcode, an Apple development account
and the mobile apps are not needed to build or run the viewer.

## Prepare the camera

Nano must already be connected to the same WPA2 router as the computer. The
viewer does not configure the camera's Wi-Fi or pair it over Bluetooth. Initial
station-mode setup is described in the experimental
[Multiview guide](../multiview-prototype/). A direct connection to the Nano's
own Wi-Fi is different from putting the Nano on a shared router.

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
cancellation and subprocess handling. Physical Nano-on-router validation is
still required; the earlier Mac iPad-app confirmation is not proof of this Go
viewer. The program exits after bounded recovery instead of silently leaving
an apparently healthy frozen session.
