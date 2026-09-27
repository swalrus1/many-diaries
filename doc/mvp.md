*This doc contains a plan for the MVP stage of the project.*

What needs to be done:
1. Storage.
    - Unified S3-like interface.
    - Disk implementation.
2. Medium.
    - Obsidian vault medium implementation:
        - Defines an HTML configuration page (Obsidian vault path) and a button to start sync. This webpage is exposed by the management service in the frontend layer.
        - A record is an file of any type.
    - Record abstraction. Record metadata: created_at.
    - Metadata index: stores record metadata (created_at), persisted as a SQLite blob via the storage layer; hidden behind the MetadataIndex interface.
3. Frontend.
    - A webapp serving different frontend services.
    - `/medium/...` - per-medium management panel, defined by the medium
    - `/stats` - dashboard with a github-like activity calendar reflecting whether any record was created on a given day.
