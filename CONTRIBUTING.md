# Contributing to SeedGrid

Thanks for your interest. This page is what you need to get a change merged.

## Principles

SeedGrid is documentation-driven. Before implementing, read the official documentation in
[`docs/`](docs/). When documents disagree, precedence is:

**ADR → Architecture → Requirements → other technical documents → Roadmap**

## How a change flows

1. Open an issue describing **what happens today** and **what changes**, in the operator's
   terms. Work starts from an issue, not from a pull request.
2. Write an ADR first when the change is architectural (`docs/ADR.md`).
3. Implement, with tests.
4. Update the documentation in the same pull request — and, for a change users notice, add a
   line under `[Unreleased]` in `CHANGELOG.md`.
5. Open the pull request with `Closes #<issue>`; CI must be green before review.

Nothing is done without matching documentation and tests (see the Definition of Done in
`docs/20 - Development Guide.md`).

## Non-negotiable rules

- **Docs travel with the code.** Any change to behaviour, a contract or a responsibility
  updates the corresponding document in the same pull request.
- **Architectural change = a new ADR**, before the implementation.
- **The database changes only through versioned migrations**; never by hand. UUID primary
  keys, soft delete, UTC timestamps.
- **No secrets in the code.** Configuration is external (`.env` / YAML) and never committed.
- **Official flows only.** The Portal talks only to the API; only the Control Plane touches
  the database; only the Agent talks to the Torrent Engine. Nodes never talk to each other.
- **Vocabulary matters.** A *Seed Node* is the machine; a *Worker* is an internal Control
  Plane component and never runs on a Node. See `docs/24 - Glossary.md`.

## What CI checks

Every push runs, per module (Control Plane, Agent, Portal, scripts): formatting, vet, lint,
build and tests. On top of that, the **policy checks** — rules that used to live only in
review:

| check | what it enforces | run it locally |
|---|---|---|
| `commitmsgcheck` | conventional commit subject (`feat:`, `fix(scope):`, …); no `Co-Authored-By` trailer | `cd tools && go run ./cmd/commitmsgcheck -range origin/main..HEAD` |
| `architecturepolicy` | the boundaries above: no database driver in the Agent, no cross-imports between Agent and Control Plane, no `Worker` declared in the Agent, network calls in the Portal only in `api.ts` | `cd tools && go run ./cmd/architecturepolicy -root ..` |
| `languagepolicy` | code in English: comments, log and error messages, test names and identifiers; the product speaks other languages only through its catalogs. Justified exceptions in `tools/language-allow.txt` | `cd tools && go run ./cmd/languagepolicy -root ..` |
| `check-docs-parity` | `docs/` and `docs/pt-BR/` have the same files and the same structure | `sh scripts/check-docs-parity.sh` |
| `check-homelab` | no addresses, machine names or runners from a private environment; examples use `192.168.0.x`, `2001:db8::/32`, `controller.example` | `sh scripts/check-homelab.sh` |
| `TestRepoProfiles` | every file under `profiles/` is a valid, canonical site profile with no credentials | part of the Control Plane tests |

Details and the reasoning behind each rule: `docs/20 - Development Guide.md`, section 29.

## Documentation in two languages

The documentation exists in English (`docs/`) and in Brazilian Portuguese (`docs/pt-BR/`),
one file per document in each. A pull request that changes a document updates **both**
versions. Write the change in whichever language you are comfortable with; the other one may
be a machine translation, as long as it keeps the structure.

CI checks that the two versions have the same files and the same skeleton — headings, code
blocks, tables, ADR and issue references (`sh scripts/check-docs-parity.sh`). It does not
check the wording; reviewers do.

Product vocabulary is never translated: Seed Node, Agent, Control Plane, Portal, Engine,
Worker, Scheduler, and the names of trackers and other sites.

## Commit messages

Conventional Commits: `type(scope): description`, with `type` one of `feat`, `fix`, `chore`,
`docs`, `refactor`, `test`, `perf`, `ci`, `build`, `style`, `revert`. Explain **why** in the
body. To check before you push, install the hook described in the Development Guide.

## Coding standards

See `docs/21 - Coding Standards.md`. Go follows `gofmt` / `go vet` / `golangci-lint`; the
Portal follows its TypeScript configuration (`strict`) and its tests.

## Contributing a site profile

If you have an account on a tracker SeedGrid does not know yet, you can share its **site
profile** — categories, types, resolutions and how the site wants uploads described — without
sharing anything of yours. Export it from the Portal and open a pull request; the steps are
in [`profiles/README.md`](profiles/README.md). A profile has no field for credentials, and
CI rejects any file that carries one.

## Reporting security issues

Not in a public issue — see [`SECURITY.md`](SECURITY.md).

## Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
