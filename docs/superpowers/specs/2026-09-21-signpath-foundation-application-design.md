# SignPath Foundation Application Design

## Goal

Prepare SyncHub for Agents for a SignPath Foundation open-source code-signing application. The repository should satisfy the public project prerequisites before a maintainer submits personal or organizational contact information through SignPath's application form.

## Assumptions

- Canonical repository: `qinqingxu/SyncHub-for-Agents`.
- Mirror repository: `jelllove/SyncHub-for-Agents` remains a synchronized mirror.
- The maintainer is unavailable during this session, so no personal identity, legal organization details, private email, or SignPath account credentials will be submitted or invented.

## SignPath Foundation requirements observed

SignPath Foundation requires eligible OSS projects to have an OSI-approved license, active maintenance, existing releases, documented functionality, no malware/hacking-tool purpose, repository/team control, and a public **Code signing policy** on the project home page or download page. The policy must mention that free code signing is provided by SignPath.io with certificate by SignPath Foundation, team roles, and privacy behavior.

## Selected approach

Prepare the application package but do not submit the external form without maintainer-provided contact details.

1. Add a repository Code signing policy page describing SignPath Foundation signing, team roles, release approval, and privacy behavior.
2. Link the Code signing policy from `README.md` so it is present on the project home page.
3. Add a SignPath Foundation application draft with public project details and a section listing the maintainer-provided fields still needed.
4. Add repository checks so the policy and draft remain discoverable.

## Non-goals

- Do not submit the SignPath form with guessed personal data.
- Do not add secrets or private certificates.
- Do not change release signing workflow yet.
- Do not claim SignPath approval before it is granted.

## Validation

- Add repocheck tests for the Code signing policy and SignPath application draft.
- Run `go test ./tools/repocheck -count=1`.
- Run `node scripts/dev.mjs docs`.
- Run `node scripts/dev.mjs check`.

## Maintainer handoff

After the repository policy and draft are merged, the maintainer can submit the SignPath application using the draft. Required private fields include the applicant name, contact email, SignPath account details, and any identity/team confirmations requested by SignPath.
