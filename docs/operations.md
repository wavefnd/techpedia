# Operations and recovery

## Daily checks

Check container health, disk capacity, recent errors, backup freshness and
administrator review queues. Statistics at `/admin/analytics` aggregate screen
views, browser/server round-trip time, countries and editions. They are not
unique visitors, precise geolocation or ICMP ping. A trusted proxy must replace
the country header. Avoid collecting more personal data just to explain a chart.

## Back up data

```sh
./backup.sh
```

The script briefly stops the application, archives the entire `data/` directory,
writes a SHA-256 sidecar, then restarts an application that was previously running.
It includes database/auth/search/media data and any proxy data under that directory.
It does **not** back up `.env`; store an encrypted configuration copy separately.
Protect archives as private data and copy them off-host under your retention policy.
A successful archive is not a tested restoration.

## Restore carefully

Use only a trusted, verified archive created by your installation. The checksum
checks accidental damage, not the trustworthiness of a malicious archive.
Restore first into an isolated installation:

```sh
./restore.sh backups/techpedia-data-YYYYMMDDTHHMMSSZ.tar.gz --yes
```

This replaces current runtime data. The script stops services, preserves the
existing data in a pre-restore archive, validates paths/checksum when provided,
extracts the backup and rebuilds/restarts. Do not run as root unnecessarily.
Keep the source version and bootstrap owner identity consistent with the backup.
Check login, editions, article history, Techdata links and images after recovery.

## Source transfer versus data transfer

`move.sh` creates a **private** source-transfer archive including Git history;
it excludes runtime data, credentials and build caches. It is not a public-source
exporter and must not be attached to a public release. Rebuild at the destination.

For a public source release, use a reviewed tracked archive as described in
[source management](source-control.md), not a working-directory transfer.
Moving source does not copy the existing database or change an installation's
content license.

## Update failure

A failed source fetch or build does not justify deleting `.env`, `data/`, or
Git history. Read the error, retain the running container where possible, and use
`./restart.sh --no-pull` only when the checked-out source is known to be correct.
Do not force-reset a dirty production tree.
