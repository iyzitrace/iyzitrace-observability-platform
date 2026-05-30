# Binary Assets Policy

Binary assets are allowed only when they directly improve documentation or
release usability.

## Current Policy

- Existing demo videos under `docs/videos/**` remain tracked in Git.
- New binary files larger than 10 MiB require maintainer approval.
- Prefer compressed screenshots or externally hosted videos for new material.
- Do not commit generated archives, local database files, private keys, or
  rendered release artifacts.

## Validation

Run:

```bash
sh scripts/check-binary-assets.sh
```

The check allows existing tracked videos and fails on unexpected large binary
files.
