# OpenHanako Markdown editor

The modules in `src/features/preview/editor` and the associated widget styles are adapted from [liliMozi/openhanako](https://github.com/liliMozi/openhanako/tree/main/desktop/src/react/editor), accessed 2026-10-14. Upstream copyright and license apply; the Apache License 2.0 is included in `OPENHANAKO-LICENSE` in this directory.

Ported: Markdown highlighting, inline and block decorations, code fence selection handling, formatting commands, task checkboxes, code block toolbars, links, images, horizontal rules, and table editing.

Local changes: browser-only image/Markdown adapters, scoped color and typography tokens, Chinese control labels, sanitized table rendering, table widget position identity, and integration with this project's SHA-256 save/conflict handling. The bottom character-count/action bar is intentionally omitted at the user's request. Desktop platform APIs, agent tools, cover generation and attachment filesystem operations are not imported.
