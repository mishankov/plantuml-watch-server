## Problem Statement

The watch server currently treats underscore-prefixed `.puml` files as ignored inputs. That works for hiding partials and shared snippets from the UI, but it also means those files are not watched as change sources. When a renderable diagram includes one of those shared files, editing the shared file does not refresh the generated output for the diagrams that depend on it. The browser can therefore keep showing stale diagrams until a top-level diagram is edited manually.

## Solution

Treat every local `.puml` file as a watchable source, while preserving the existing rule that underscore-prefixed files are not standalone diagrams. Build dependency awareness for local include relationships so the server can determine which public diagrams depend on a changed source file. When any source changes, regenerate the changed public diagram itself when applicable and also regenerate every public diagram that depends on that source directly or transitively. Keep ignored include files hidden from the index and do not generate standalone output for them.

## User Stories

1. As a diagram author, I want a diagram that includes `_common.puml` to refresh automatically after `_common.puml` changes, so that shared edits are reflected without touching the top-level diagram file.
2. As a diagram author, I want diagrams with nested includes to refresh when any included file in the chain changes, so that transitive dependencies stay accurate.
3. As a diagram author, I want underscore-prefixed include files to remain hidden from the generated diagram list, so that helper sources do not appear as standalone diagrams.
4. As a diagram author, I want underscore-prefixed include files to avoid producing standalone `.svg` and `.png` outputs, so that the output directory only contains public diagrams.
5. As a diagram author, I want direct edits to a public diagram to keep regenerating that diagram as they do today, so that the existing editing workflow does not regress.
6. As a diagram author, I want dependent diagrams to be regenerated only once per change event, so that shared include edits do not trigger duplicate work for the same output.
7. As a diagram author, I want relative include paths to resolve from the including file, so that diagrams organized in nested folders behave correctly.
8. As a diagram author, I want deleted or renamed public diagrams to keep cleaning up their previously generated outputs, so that stale files are not left behind.
9. As a diagram author, I want include-file edits to trigger browser-visible output updates through the existing generated files, so that live viewing keeps working without UI changes.
10. As an operator, I want startup generation to account for include dependencies before the server begins serving requests, so that outputs are consistent from the first page load.
11. As a maintainer, I want dependency tracking to work for shared include files referenced by multiple diagrams, so that a single source edit refreshes every affected diagram.
12. As a maintainer, I want dependency-aware behavior covered by tests, so that future watcher changes do not silently break include updates.

## Implementation Decisions

- Introduce a distinction between watchable source files and public entry diagrams. All local `.puml` files under the input tree are watchable sources. Public entry diagrams are the subset whose base filename does not start with `_`.
- Add dependency discovery for local include relationships and maintain a reverse dependency index from source files to the public entry diagrams that depend on them.
- Resolve include paths relative to the including file's directory so nested diagram folders continue to work predictably.
- Support transitive dependency expansion so changes to a deeply shared include file still regenerate the correct top-level public diagrams.
- Preserve the existing output-location behavior for public diagrams, including nested output directories and orphaned-output cleanup when a public diagram stops generating a file.
- Preserve the current product rule that ignored helper files are not listed in the UI and do not get standalone generated outputs.
- When dependency information is incomplete or an include cannot be resolved, the watcher should still remain operational and attempt the affected public renders rather than crashing the process.
- Keep the implementation scoped to repository-local include relationships used by watched `.puml` files. The feature does not require UI, protocol, or CLI changes.

## Testing Decisions

- Good tests should validate externally visible behavior: which public diagrams are considered affected by a source change, whether ignored helper files stay hidden, and whether transitive dependencies are included in the regeneration set.
- Test the dependency-discovery and dependency-expansion logic as a deep module in isolation, using small synthetic file trees instead of relying on PlantUML execution.
- Test watcher orchestration with a stubbed renderer so change handling can verify affected diagrams and cleanup behavior without invoking Java.
- Keep assertions focused on observable outcomes such as affected source sets, generated-output ownership, and ignore semantics rather than internal map shapes.
- Follow the existing Go unit-test style already present in the repository as prior art for table-driven or focused behavior tests.

## Out of Scope

- Rendering underscore-prefixed include files as standalone diagrams.
- Changing the browser UI, websocket protocol, or download behavior.
- Replacing the current polling-based file-watch approach with filesystem notifications.
- Adding support for remote include URLs or non-local dependency sources beyond what is needed for local watched files.
- Broad refactors unrelated to include-aware regeneration.

## Further Notes

- This PRD assumes the issue's requested behavior applies to included local `.puml` sources in general, with underscore-prefixed files being the explicit failing case called out by the issue.
- If implementation uncovers multiple include syntaxes in active use, support should be expanded only as needed to cover local repository usage while preserving the same dependency-aware behavior.
- Source issue: https://github.com/mishankov/plantuml-watch-server/issues/36
