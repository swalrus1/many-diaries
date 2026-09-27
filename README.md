# Many Diaries

Backup and search your digital footprint from many sources. See `doc/` for architecture and design decisions.

## Supported data sources

- **Obsidian** — backs up a local vault (notes as files).
- **Instagram** — backs up your posts, reels, and stories (live + archive) via a browser `sessionid`; the key is not stored, you paste it on every sync.

## Run

```sh
just run   # builds and starts on :8080 (requires `storage.disk.root` in many-diaries.yaml)
```
