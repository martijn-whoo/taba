## Services

- **`taba svc --json` lists each task's `tags`** as its declaration writes them,
  so a consumer no longer reads `dekit.yaml` and its fragments itself. A `tags`
  value that is not a list of strings is now refused, as dekit refuses it
  ([#NNN](https://github.com/lcorneliussen/taba/issues/NNN)).
