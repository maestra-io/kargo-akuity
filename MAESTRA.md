# Maestra patches

This fork carries the patches below on top of the upstream Kargo tag in the
branch name (`maestra/vX.Y.Z`). Images are released by pushing
`vX.Y.Z-maestra.N` (`.github/workflows/release-maestra.yaml`). On a rebase onto
a new upstream tag, re-apply every patch and keep its test.

| Patch | Files | Why |
|---|---|---|
| Project `kargo-admin` Role is read-only on `configmaps`, `serviceaccounts`, `roles`, `rolebindings`; no `secrets` | `pkg/controller/management/projects/projects.go` (`ensureDefaultUserRoles`), test `TestReconciler_ensureDefaultUserRoles_PromotionRequestPermissions` | Upstream grants `*` (incl. escalate/bind, AVD-KSV-0050). Projects are GitOps-managed, API secret management is off, nobody is mapped to this Role. The reconcile loop rewrites existing Roles on upgrade. issues-maestra#1694 |
| Freight opt-out of auto-promotion via label (#19) | Stage controller | `kargo.maestra.io/auto-promotion: disabled` for hotfix Freight |
| Deny Freight downgrades unless the Promotion is a rollback (#16) | API / webhook | Prevent accidental rollbacks |
| `X-Kargo-Act-As-Email` attribution for k8s-token actions (#7) | `pkg/server` | kargo-mcp actions are attributed to the human caller |
| Prometheus metrics for the API server (#8, #9) | `pkg/server` | Observability |
| `git-merge-pr`: transient GitHub 5xx are retryable (#5) | promotion step | Flaky promotions |
| UI: Maestra mascot instead of the Kargo logo (#6) | `ui/` | Branding |
| CI: Maestra ECR release, RunsOn, amd64 only, pinned actions, cosign + SBOM, nora pull-through (#12, #17, #18, #20, #22) | `.github/workflows/`, `Dockerfile` | Build and supply chain |
