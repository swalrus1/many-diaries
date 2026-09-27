*Design decisions for the medium layer.*

- Each medium is an in-process Go package implementing a `Medium` interface (`Name`, `ConfigPanel`, `HandleAction`). A medium defines only its **config panel** (an HTML fragment + form actions); the web layer owns full page composition.
- Metadata index: SQLite via `modernc.org/sqlite` (pure Go, no CGO).
- The index is self-sufficient: never re-reads record blobs to answer metadata queries.
- Index persistence: SQLite file stored as a blob via the storage layer; hidden behind the `MetadataIndex` interface so it can later be swapped for a managed relational DB.
- Single-writer assumption (local-first, one running instance); atomic upload via temp key + server-side copy.
