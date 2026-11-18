# Preserved originals — 2026-11-15

The five original repositories remain unchanged beside this workspace. They are recovery copies; new plugin development belongs under this repository's `packages/`.

The sibling `backup/magpie-monorepo-20261115/` also contains:

- `<package>.bundle`: verified Git bundle of every original ref, including history.
- `<package>/working-source.tar.gz`: actual tracked and nonignored untracked files at migration start, including uncommitted changes.
- `<package>/staged.patch`, `unstaged.patch`, `status.txt`: the original index/worktree distinction and untracked list.
- `inventory.json`: original paths, hashes, modes, refs and ignored-file disposition.
- `git-bundles.json`: bundle hashes and sizes.

[source-map.json](source-map.json) maps all 667 original source files into the monorepo. Its `sha256` is the original content, not the relocated content after path repairs. Where there was a second move, `initialTarget` preserves the first destination. Per-package relocation records explain edits.

Ignored build outputs and caches were recorded and left in the original repositories. They were not imported as source. No original repository, dirty change, Git branch or stash was removed. No history rewrite was used to assemble this workspace.

To recover independently of the originals, clone the relevant bundle into a fresh directory and use the saved working-source archive and patches to reconstruct its recorded dirty state. Do not extract over a repository containing new work. Keep this backup until the consolidated repository and its eventual remote copy are accepted.
