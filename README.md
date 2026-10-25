# App Volume for OpenDeck

A Windows plugin for [OpenDeck](https://github.com/nekename/OpenDeck) that turns your stream controller into a per-app volume mixer.

- Keys automatically show the icons of apps that are **currently playing sound**
- **Press** a key to select that app (white dashed border), press it again to mute
- **Turn a knob** to change the selected app's volume, press the knob to mute
- The volume % is shown in the corner of each icon
- Optional **master volume knob** for the whole computer

Built and tested with an **Ajazz AKP03E (rev. 2)** using the [opendeck-akp03](https://github.com/4ndv/opendeck-akp03) device plugin, but it uses the standard Stream Deck SDK, so it should work with other devices supported by OpenDeck.

## Install

1. Download `AppVolume-OpenDeck-plugin.zip` from the [Releases](../../releases) page.
2. In OpenDeck: **Plugins → Install from file**, pick the zip (don't unzip it).
3. Drag **App Slot** onto the keys you want to use for apps.
4. Drag **Selected App Volume (knob)** onto a knob.

## Actions

| Action | Where | What it does |
|---|---|---|
| App Slot | Key | Shows an app that's playing sound. Press to select, press again to mute. |
| Selected App Volume (knob) | Knob | Turn to change the selected app's volume (2% per tick by default). Press to mute. |
| Master Volume (knob) | Knob | Turn to change the computer's overall volume (default output device). Press to mute. Shows the Windows volume popup. |
| Selected App Volume Up / Down | Key | Changes the selected app's volume (5% per press by default). |
| Selected App Mute | Key | Mutes or unmutes the selected app. |

The step size can be changed in the action's settings in OpenDeck.

Apps fill the slots from the top-left. An app stays on its key for 30 seconds after it goes quiet so keys don't jump around when you pause something. Sessions are grouped per program across all active output devices.

## Build from source

Requires Go 1.24+. Cross-compiles from any OS:

```sh
./build.sh
```

The plugin zip ends up in `dist/`. On non-Windows systems the plugin runs with a fake audio backend, which is handy for testing (`go run ./tools/fakehost <plugin-binary> out.png` simulates OpenDeck and renders the keys).

## License

MIT
