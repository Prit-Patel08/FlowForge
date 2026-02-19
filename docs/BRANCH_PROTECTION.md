# Branch Protection Baseline

Configure `main` with required status checks before merge.

Required checks:
- `backend`
- `dashboard`
- `sbom`

Recommended settings:
- Require pull request before merging
- Require approvals: 1+
- Dismiss stale approvals on new commits
- Require branches to be up to date before merging
- Do not allow bypassing required checks for non-admin users

This keeps release quality tied to build/test/race/staticcheck/govulncheck, dashboard build, and SBOM generation.
