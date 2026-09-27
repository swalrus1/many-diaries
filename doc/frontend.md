*Design decisions for the frontend layer.*

- Go HTTP server (`net/http`).
- MVP: server-rendered pages via Go `html/template`; no JS build pipeline.
- UI styling: Pico.css (classless), vendored and served by the Go server — mediums emit plain semantic HTML, no class names to coordinate.
- Medium pages are composed uniformly by the web layer: medium config panel + shared activity panel (Frappe Charts heatmap, vendored, fed by `/api/stats?medium=<name>`); `/stats` reuses the same panel unfiltered.
- Post-MVP: SvelteKit embedded in the binary via `embed.FS` (single self-contained executable), introduced when the query engine needs an interactive UI.
- Query engine (post-MVP): SQLite FTS5 over the metadata index.
