*Design decisions for the storage layer.*

- Custom S3-like Go interface (`Get`/`Put`/`List`/`Delete`) is the contract.
- Disk connector (MVP): Go stdlib `os`/`io/fs`.
- Cloud connectors (post-MVP): `gocloud.dev/blob` behind the same interface.
