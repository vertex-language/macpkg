# Sample App (`SampleApp.app`)

A native macOS Cocoa application built, codesigned, packaged into a `.dmg`, and compiled into a `.pkg` using the `macpkg` pure-Go toolchain.

## Features
- **Native Cocoa UI**: Frosted glass vibrancy (`NSVisualEffectView`), standard rounded window, and responsive buttons.
- **Interactive Controls**: Click counter and GitHub link launcher.
- **Valid Code Signature**: Signed with ad-hoc identity (`"-"`) and Hardened Runtime (`CS_RUNTIME`), verified by Apple `codesign`.
- **Formats Generated**:
  - `SampleApp.app`: Application bundle ready to double-click.
  - `SampleApp.dmg`: Apple Disk Image mountable in Finder.
  - `SampleApp.pkg`: Flat package installer executable via `Installer.app`.

## Building
```bash
./build.sh
```

## Running
```bash
open dist/SampleApp.app
```
