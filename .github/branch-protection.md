# Recommended Branch Protection

Configure `main` as the protected default branch.

Required settings:

- Require pull requests before merging.
- Require at least one approving review.
- Require review from CODEOWNERS.
- Dismiss stale approvals when new commits are pushed.
- Require status checks from `.github/workflows/ci.yml`.
- Require branches to be up to date before merging.
- Block force pushes.
- Block branch deletion.
- Use squash or rebase merge consistently.
- Restrict direct pushes to maintainers for emergency use only.

