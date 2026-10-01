# Kigumi for Visual Studio Code

Syntax highlighting for `.kg` files and a client for the Kigumi language
server (`kigumi lsp`): diagnostics as you type, hover types, go to
definition, go to implementation, find all references, rename (`F2`),
quick fixes for diagnostics that carry one (unused import, missing `mut`,
unqualified variant, discarded `Result`), formatting, document and
workspace symbols (`Ctrl+T`) and completion.

Diagnostics cover the whole workspace, not only open files: the extension
activates as soon as the workspace contains a `.kg` file, the server checks
the workspace module right after initialization, and it re-checks when
`.kg` files change on disk. Errors in files you never opened show up in the
Problems view and the explorer.

## Setup

1. Build the server: `go install .` in the repository root,
   or point `kigumi.serverPath` at the built executable.
2. The server needs the std stubs. It looks at `kigumi.stdRoot`, then
   `$KIGUMI_STD`, then `<workspace>/std`, then the `std` directory
   next to the executable.
3. From the repository root: `pnpm install && pnpm build` (the extension is
   a pnpm workspace package; esbuild bundles `vscode-languageclient` into
   `out/extension.js`, so the `.vsix` carries no `node_modules`). Then open
   `editors/vscode` in VS Code and press F5, or run `pnpm package` and
   install the `.vsix` with `code --install-extension`.

Settings: `kigumi.serverPath`, `kigumi.stdRoot`, `kigumi.trace.server`.
Commands: `Kigumi: Restart Language Server`, `Kigumi: Show Language Server Log`.

The "Kigumi Language Server" output channel (View > Output) collects the
server's log messages: the workspace and std root it picked, a warning when
the std stubs are missing, module load and checker failures, and anything
the server prints to stderr. Set `kigumi.trace.server` to `messages` or
`verbose` to see the LSP traffic there too.
