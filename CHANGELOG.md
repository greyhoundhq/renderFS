# Changelog

## Unreleased

- Breaking: switched templating engine to Gonja; `Options.Context` is now `map[string]any` and `RenderBytes` accepts a strict flag.
- Added: `Options.StrictVariables` to enforce undefined variables.
- Breaking: `Copy` now returns `Stats`, and `Writer` includes `Open` to support identical detection.
- Added: `RenderBytes` helper for rendering template file content consistently.
