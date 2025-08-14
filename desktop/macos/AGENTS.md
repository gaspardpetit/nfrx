# AGENTS.md

## Scope
These instructions apply to the macOS menu bar application located in this directory.

## Build & Testing
- The project uses Swift and AppKit with a minimum macOS deployment target of 12.0.
- No automated tests are required yet; manual validation happens on macOS using Xcode.

## Assets
- Binary assets such as icons are committed as base64-encoded files (`.png.b64`).
- Decode them back to their original filenames before building the app.

## Code Style
- Format Swift sources with Xcode's default formatting.
- Keep functions small and well named.
