# NGINX Certificates

This directory is intentionally empty in Git.

The local Docker Compose stack generates bootstrap TLS material at runtime when
certificates are missing. Those files are development-only and must not be
committed.

For production:

- Use certificates from a trusted CA.
- Rotate any certificate or key that was ever committed to Git.
- Store private keys in a dedicated secret manager or Kubernetes Secret.

