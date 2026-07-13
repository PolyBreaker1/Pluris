# Vendored CodeMirror 6 bundle

`codemirror-pluris.js` is a self-hosted, prebuilt CodeMirror 6 bundle used by
`web/static/code-editor.js` (the `PlurisCodeEditor` wrapper). Pluris runs no
npm build at runtime and loads no CDN scripts, so this file is built once
outside the repo and committed as a plain static asset served by Echo's
`/static` route (`console/server/server.go`, `e.Static("/static", "web/static")`).

## Build date

2026-07-10

## License

CodeMirror 6 (all `@codemirror/*` packages below) is MIT licensed. Full
license text: https://github.com/codemirror/dev/blob/main/LICENSE

```
Copyright (C) 2018-2026 by Marijn Haverbeke <marijnh@gmail.com> and others

Permission is hereby granted, free of charge, to any person obtaining a
copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be included
in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```

## Exact package versions (pinned)

| Package                        | Version |
| ------------------------------- | ------- |
| `@codemirror/state`             | 6.7.1   |
| `@codemirror/view`              | 6.43.6  |
| `@codemirror/language`          | 6.12.4  |
| `@codemirror/commands`          | 6.10.4  |
| `@codemirror/autocomplete`      | 6.20.3  |
| `@codemirror/search`            | 6.7.1   |
| `@codemirror/lang-json`         | 6.0.2   |
| `@codemirror/lang-yaml`         | 6.1.3   |
| `@codemirror/legacy-modes`      | 6.5.3   |
| `@codemirror/theme-one-dark`    | 6.1.3   |

## Bundler

`esbuild` 0.28.1, invoked directly (no rollup config needed — esbuild's
IIFE output was sufficient for a single global-exposing bundle).

- Target: `es2020`
- Format: `iife`
- Minified, no sourcemap shipped (kept the reproduction simple; regenerate
  locally with `--sourcemap` if debugging the vendor bundle itself is ever
  needed — do not commit a sourcemap without a reason, it roughly doubles
  the file size)

## Reproduction

Build happens OUTSIDE this repo (a scratch npm project), and only the
resulting single-file bundle + this README are committed.

```sh
mkdir -p /tmp/cm-build && cd /tmp/cm-build
npm init -y
npm install \
  @codemirror/state@6.7.1 \
  @codemirror/view@6.43.6 \
  @codemirror/language@6.12.4 \
  @codemirror/commands@6.10.4 \
  @codemirror/autocomplete@6.20.3 \
  @codemirror/search@6.7.1 \
  @codemirror/lang-json@6.0.2 \
  @codemirror/lang-yaml@6.1.3 \
  @codemirror/legacy-modes@6.5.3 \
  @codemirror/theme-one-dark@6.1.3
npm install -D esbuild@0.28.1

cat > entry.js <<'EOF'
import { EditorState, Compartment } from "@codemirror/state";
import {
  EditorView, keymap, lineNumbers, highlightActiveLine,
  highlightActiveLineGutter, highlightSpecialChars, drawSelection,
  dropCursor, rectangularSelection, crosshairCursor, highlightWhitespace,
  placeholder,
} from "@codemirror/view";
import {
  defaultHighlightStyle, syntaxHighlighting, indentOnInput,
  bracketMatching, foldGutter, foldKeymap, StreamLanguage,
} from "@codemirror/language";
import {
  defaultKeymap, history, historyKeymap, indentWithTab,
} from "@codemirror/commands";
import {
  autocompletion, completionKeymap, closeBrackets, closeBracketsKeymap,
} from "@codemirror/autocomplete";
import { searchKeymap, highlightSelectionMatches } from "@codemirror/search";
import { json } from "@codemirror/lang-json";
import { yaml } from "@codemirror/lang-yaml";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { oneDark } from "@codemirror/theme-one-dark";

const basicSetup = [
  lineNumbers(), highlightActiveLineGutter(), highlightSpecialChars(),
  history(), foldGutter(), drawSelection(), dropCursor(),
  EditorState.allowMultipleSelections.of(true), indentOnInput(),
  syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
  bracketMatching(), closeBrackets(),
  rectangularSelection(), crosshairCursor(), highlightActiveLine(),
  highlightSelectionMatches(),
  keymap.of([
    ...closeBracketsKeymap, ...defaultKeymap, ...searchKeymap,
    ...historyKeymap, ...foldKeymap, ...completionKeymap, indentWithTab,
  ]),
];

window.CM6 = {
  EditorState, EditorView, Compartment, basicSetup, keymap,
  json, yaml,
  shellStreamLanguage: StreamLanguage.define(shell), StreamLanguage,
  autocompletion, oneDark, highlightWhitespace, placeholder,
};
EOF

npx esbuild entry.js --bundle --minify --format=iife \
  --target=es2020 --outfile=codemirror-pluris.js

cp codemirror-pluris.js <pluris-repo>/web/static/vendor/codemirror/codemirror-pluris.js
```

## Global API surface (`window.CM6`)

| Export                | What it is                                                        |
| ---------------------- | ------------------------------------------------------------------ |
| `EditorState`          | `@codemirror/state` EditorState class                              |
| `EditorView`           | `@codemirror/view` EditorView class                                |
| `Compartment`          | `@codemirror/state` Compartment (for reconfigurable extensions)    |
| `basicSetup`           | Array of extensions — a basicSetup-equivalent (gutter, history, bracket matching, search, keymaps, etc.). Deliberately EXCLUDES `autocompletion()` — the wrapper (`code-editor.js`) adds it per-mount so it can pass `{override: [completionSource]}` when the caller supplies one |
| `keymap`               | `@codemirror/view` keymap facet helper                              |
| `json`                 | `@codemirror/lang-json` language support function                  |
| `yaml`                 | `@codemirror/lang-yaml` language support function                  |
| `shellStreamLanguage`  | Pre-defined `StreamLanguage` instance for bash/shell (legacy mode)  |
| `StreamLanguage`       | `@codemirror/language` StreamLanguage class (for custom modes)     |
| `autocompletion`       | `@codemirror/autocomplete` autocompletion() extension factory       |
| `oneDark`              | `@codemirror/theme-one-dark` theme extension                        |
| `highlightWhitespace`  | `@codemirror/view` highlightWhitespace() extension factory          |
| `placeholder`          | `@codemirror/view` placeholder() extension factory                  |

## Bundle size

`codemirror-pluris.js` is ~412 KB minified (well under the 800 KB budget
in the task brief). No further size work (per-language code splitting,
brotli, etc.) was needed.
