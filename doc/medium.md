*Design decisions for the medium layer.*

- Each medium is an in-process Go package implementing a `Medium` interface (`Name`, `ConfigPanel`, `HandleAction`). A medium defines only its **config panel** (an HTML fragment + form actions); the web layer owns full page composition.
- Metadata index: SQLite via `modernc.org/sqlite` (pure Go, no CGO).
- The index is self-sufficient: never re-reads record blobs to answer metadata queries.
- Index persistence: SQLite file stored as a blob via the storage layer; hidden behind the `MetadataIndex` interface so it can later be swapped for a managed relational DB.
- Single-writer assumption (local-first, one running instance); atomic upload via temp key + server-side copy.
- Record IDs are stable per medium; mediums must check `HasRecord` and skip already-indexed records so re-syncs never re-download blobs.
- Record attrs: medium-specific JSON stored in the index (e.g. Instagram stores `permalink` and `storage_key`) — keeps blob fetches unnecessary for linking back to source or persisted data.
- Mediums so far: `obsidian` (vault walk), `instagram` (private API via sessionid; posts/reels, live stories, story archive; sessionid is **not** persisted — asked on every sync).
- Instagram client details (all against `i.instagram.com`): auth = `sessionid` cookie + `Authorization: Bearer IGT:2:<base64({ds_user_id, sessionid, should_use_header_over_cookies})>` (instagrapi's scheme). Feed: v1 REST `feed/user/{uid}/` primary (full `max_id` pagination), GraphQL `IGProfileTimelineQuery` fallback (works when v1 is rate-limited but capped at ~33 recent items — Instagram ignores its pagination variables). GraphQL and `reels_media_stream` responses are NDJSON streams: a shell line followed by one deferred chunk per media; chunks lack `taken_at` (filled from the shell's grid items). All API requests must not follow redirects (Instagram 302s blocked requests to HTML pages).
