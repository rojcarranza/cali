# ADR 0002: YAML library for cali.yaml

Status: accepted (2026-10-16)

## Context

`cali.yaml` is hand-edited and committed. Cali reads it on every command and rewrites it only for `add`, `remove` and `import`. The spec (section 3) requires unknown keys to be an error and asks that comments be preserved on rewrite if the library allows it. The Go standard library has no YAML support, so this is the one dependency the file formats force (the lock is JSON and needs none).

## Decision

Use `go.yaml.in/yaml/v3`, version `v3.0.5` (verified: `go get go.yaml.in/yaml/v3@v3.0.5` resolves; module time 2026-07-26). It is the YAML organization's continuation of the archived `gopkg.in/yaml.v3`, a pure-Go library with the same API, and it builds into a single static binary on macOS, Windows and Linux.

- Reading: decode with `yaml.Decoder.KnownFields(true)` so unknown keys are an error.
- Rewriting: decode into `yaml.Node`, edit the node tree, encode with indent 2.

Alternatives considered:
- `gopkg.in/yaml.v3`: same code, but the repository is archived; rejected as unmaintained.
- `goccy/go-yaml`: maintained and has comment-preserving paths, but a larger surface and a different API; no advantage that Cali needs.
- `sigs.k8s.io/yaml`: converts through JSON and drops comments.

## Comment preservation (spec section 3 "Verify")

Tested in a scratch module outside this repo (`go.yaml.in/yaml/v3@v3.0.5`):

- Round trip through `yaml.Node` (decode, append a new skill entry to the `skills` mapping, encode): head comments, line comments and comments above keys all survive, and a head comment attached to the new node is emitted. Only whitespace before an inline comment is normalized (`targets: [a, b]   # c` became `targets: [a, b] # c`). Flow sequences stay flow style.
- Round trip through a typed struct (`Unmarshal` into a struct, `Marshal` back): comments are lost.

Consequence: yes, comments survive a rewrite, but only if the writer edits the `yaml.Node` tree. Cali must never regenerate `cali.yaml` from a typed struct. Validation (unknown keys, version) uses a separate typed decode of the same bytes.

## Consequences

- One new dependency, to be added to `go.mod` by the task that first uses it.
- Rewrites may normalize inline-comment spacing and indentation to 2 spaces; the content and comments are kept.
- Node-tree editing is more code than struct marshaling; it is confined to the manifest package.
