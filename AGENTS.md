# Unicast MPV

Unicast MPV is a daemon server that runs in the background, receives JSON RPC calls and translates them to MPV IPC calls.

What we want to do: perform a translation of the project from NodeJS TypeScript (inside ./nodejs/) to GO lang (inside ./golang/).

The original node-mpv dependency (currently cloned into ./node-mpv/) will be integrated into the GO lang application, on it's own isolated sub-package.

## Issue Tracker

When implementing issues, consult `issue-tracker.md` at the project root to know which issues are done (checked) and which are still missing (unchecked). Update the checkbox in `issue-tracker.md` to `[x]` when an issue is fully implemented and verified. Each issue file lives in `./issues/` and contains the full specification for that phase.
