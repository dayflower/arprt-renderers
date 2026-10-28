# Development guide for agents

This repository maintains renderers provided by the ARPRT project. ARPRT is an
independent, unofficial Kubernetes controller for TRMNL BYOS, with no affiliation
with or endorsement by TRMNL.

- Use ARPRT for the brand name in prose and display text. Preserve lowercase
  `arprt` in technical identifiers such as repository names, images and paths.
- Keep implementation, dependencies, Dockerfile, fixtures, examples, version and
  renderer-specific release tooling/tests under `renderers/<name>/`. Each renderer
  has its own image and release tags; do not synchronize versions with ARPRT or
  another renderer.
- Read README.md, notes/DEVELOP.md, notes/RELEASE.md and the renderer README before
  changing behavior or packaging. Protocol ownership stays with ARPRT.
- Keep Ruff configuration global. Normal CI runs shared checks and all renderer
  workflows; a renderer release calls only that renderer's verification workflow.
- Keep credentials, private hosts/registries and real device identities out of
  the repository. Use synthetic fixtures and offline automated tests.
- Preserve the Job security contract and atomic output replacement. No runtime
  dependency installation or writable temporary directory outside output.
- Run `make check` and the relevant renderer's
  `make -C renderers/<name> container-check` before completing executable
  changes. Describe architecture and cluster/physical-test limits.
