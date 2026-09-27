*This doc describes a high-level architecture of the application.*

*Design decisions are recorded in per-layer docs: `doc/storage.md`, `doc/medium.md`, `doc/frontend.md`.*

The application is split onto several layers. Here they are from low to high level:

1. **Storage layer.** A unified interface that allows persisting footprint data and supporting metadata.
    1.1. **Storage connectors.** A storage connector provides S3-like interface to a storage managed by Many Diaries, such as disk, S3, google drive.
2. **Medium layer.** Manages sources of data, uses **storage** for backing it up.
    2.1. **Mediums.** A medium represents a source of digital footprint data. Medium implementation uses **the storage layer** to persist footprint data. A medium defines:
        - What a **record** is (e.g. a social media post, a file in a self-hosted knowledge base, a message in a messenger service), what metadata it holds, how can a user read the data.
        - How to fetch the data, including configuration of access to the data source.
        - How to persist the data.
    2.2. **Record inteface.** Providea a unified read-only access to the **records** of all **mediums**.
    2.3. **Metadata index.** A component that stores and queries record metadata (e.g. `created_at`). It is self-sufficient: it never requires reading record blobs to answer metadata queries. How the index is persisted is an implementation detail hidden behind the `MetadataIndex` interface; the current implementation is a SQLite file persisted as a blob via the **storage layer** (see `doc/medium.md`), and it may be replaced with a managed relational DB in the future.
3. **Frontend.** UI.
    3.1. **Query engine.** Allows searching records.
    3.2. **Stats.** Aggregates and displays statistics across mediums.
    3.3. **Medium management.** Exposes a UI for configuring and syncing mediums.
