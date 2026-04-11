# Plan: Update diagrams on include changes

> Source PRD: `plans/0001. update diagrams on include changes/prd.md` and issue `#36`

## Architectural decisions

Durable decisions that apply across all phases:

- **Inputs**: Treat every local `.puml` file under the configured input root as a watchable source.
- **Public diagrams**: Only `.puml` files whose base name does not start with `_` are public entry diagrams. Only public entry diagrams appear in generated outputs and the UI.
- **Dependency model**: Maintain local include relationships between watched source files and derive a reverse mapping from any changed source to the public entry diagrams affected by that change.
- **Include resolution**: Resolve local include paths relative to the including file so nested folder layouts continue to work.
- **Regeneration behavior**: A change event regenerates the changed public diagram itself when applicable, plus every dependent public diagram exactly once.
- **Startup behavior**: Initial generation continues to happen before the server starts serving traffic, with the same public-diagram filtering and output-folder layout rules.
- **Output ownership**: Output cleanup remains keyed to public entry diagrams so deleted or renamed diagrams still remove their previously generated `.svg` and `.png` files.
- **Scope boundary**: No UI, websocket, HTTP route, or CLI changes are required for this feature.

---

## Phase 1: Separate watchable sources from public diagrams

**User stories**: 3, 4, 5, 10

### What to build

Introduce an explicit distinction between all watchable PlantUML sources and public entry diagrams. Startup generation and ongoing watching should operate on the full source set for change detection, while rendering and output publication remain limited to public entry diagrams.

### Acceptance criteria

- [ ] The system can enumerate all local `.puml` sources under the input tree, including underscore-prefixed helper files.
- [ ] The system can enumerate public entry diagrams as a separate subset that excludes underscore-prefixed files.
- [ ] Startup generation still renders only public entry diagrams and preserves the current output directory structure.
- [ ] Hidden helper files remain absent from generated standalone outputs and from any file-listing behavior derived from the output directory.

---

## Phase 2: Build local include dependency awareness

**User stories**: 1, 2, 7, 11, 12

### What to build

Add dependency discovery for local include relationships between watched source files. The system should be able to determine which public entry diagrams depend on a given source file, including transitive dependency chains through shared helper files.

### Acceptance criteria

- [ ] Local include references are discovered from watched source files and normalized to source files within the input tree.
- [ ] Relative include paths are resolved from the including file's directory.
- [ ] The system can answer which public entry diagrams are affected by a change to a shared helper file.
- [ ] Transitive include chains are supported so a deeply shared source still maps back to the correct public entry diagrams.
- [ ] Dependency discovery failures do not terminate the watcher; affected public diagrams are still handled conservatively.

---

## Phase 3: Regenerate affected public diagrams on source changes

**User stories**: 1, 2, 5, 6, 8, 9, 11

### What to build

Connect dependency awareness to watcher execution so a source-file change regenerates the correct set of public entry diagrams, with deduplication and existing cleanup behavior intact. Public diagrams should still rerender on direct edits, and helper-file edits should now refresh every affected public output.

### Acceptance criteria

- [ ] Editing a public entry diagram still regenerates that diagram's outputs.
- [ ] Editing an underscore-prefixed helper file regenerates every affected public entry diagram and does not attempt to render the helper file itself as a standalone diagram.
- [ ] Affected public diagrams are regenerated at most once per source-change handling cycle.
- [ ] Multiple public diagrams depending on the same helper file are all regenerated after that helper changes.
- [ ] Removing a public entry diagram still removes its previously generated outputs.

---

## Phase 4: Lock in behavior with focused tests

**User stories**: 1, 2, 3, 4, 6, 8, 12

### What to build

Add focused regression coverage around dependency resolution and watcher orchestration so include-aware regeneration remains stable without relying on a live PlantUML process in tests.

### Acceptance criteria

- [ ] Automated tests cover the distinction between watchable sources and public entry diagrams.
- [ ] Automated tests cover direct and transitive dependency expansion for local include relationships.
- [ ] Automated tests cover regeneration-set selection for helper-file changes and public-diagram changes.
- [ ] Automated tests cover deduplication and public-output cleanup behavior.
- [ ] The full project test suite passes after the feature is implemented.
