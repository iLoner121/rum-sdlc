# RumSDLC architecture

Core workflow rules remain in `internal/core` and persistence in `internal/store`.
`internal/app` exposes device/project operations; CLI and the optional daemon use
those same operations. Neither reads source trees to infer delivery or approval.

T18 introduces the following boundaries on its implementation branch:

- `internal/release` owns the program release baseline, build identity and strict
  semantic version comparison. The workflow protocol version is independent.
- `internal/update` owns public release queries, bounded package verification and
  device-local state in `HOME/updates`. It imports neither app nor installation.
- app exposes check/configuration/acknowledgement; service runs an independent,
  default-disabled hourly worker. CLI displays deduplicated terminal notices.
- CLI owns the native installation coordinator and detached helper. The helper
  uses the original installation transaction, after old processes release their
  leases. `ExpectedActive` is checked inside the exclusive installer gate.
- `.github/workflows/release.yml`, `.github/scripts/release.py` and
  `cmd/release-plan` are RumSDLC repository release policy; they are not embedded
  or installed in managed projects.

Stable releases identify a fixed source commit. Development/unknown/dirty builds
are distinguishable. New device update files never change project state,
project rules, meta sync or the independent core protocol.

Long-term policy: [release and update](../decisions/20261111-release-update-policy.md).
User commands and recovery: [usage](../../internal/assets/docs/usage.md).
