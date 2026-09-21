# SignPath Foundation Application Preparation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prepare SyncHub for Agents for a SignPath Foundation open-source code-signing application without submitting private maintainer information.

**Architecture:** Add a public Code signing policy and an application draft that satisfy SignPath Foundation's documented repository requirements. Guard the documents with repocheck so release-signing prerequisites stay visible and maintainable.

**Tech Stack:** Markdown documentation, Go `tools/repocheck` tests, existing `node scripts/dev.mjs docs` Markdown validation.

---

## File structure

- `docs/code-signing-policy.md`: Public policy required by SignPath Foundation; covers SignPath attribution, team roles, release approval, privacy behavior, and scope of signed artifacts.
- `docs/signpath-foundation-application.md`: Application draft with public project information and maintainer-provided fields still needed.
- `README.md`: Adds a short `Code signing policy` section that links to the dedicated policy.
- `tools/repocheck/workflow_test.go`: Adds a guard that verifies the SignPath policy/draft and README link remain present.

---

### Task 1: SignPath policy documentation

**Files:**
- Create: `docs/code-signing-policy.md`
- Create: `docs/signpath-foundation-application.md`
- Modify: `README.md`
- Modify: `tools/repocheck/workflow_test.go`

- [ ] **Step 1: Write the failing repocheck test**

Add this test to `tools/repocheck/workflow_test.go` after `TestWindowsReleaseSigningIsDocumented`:

```go
func TestSignPathFoundationApplicationMaterialsAreDocumented(t *testing.T) {
	for _, relative := range []string{
		"docs/code-signing-policy.md",
		"docs/signpath-foundation-application.md",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", relative)); err != nil {
			t.Fatalf("%s must exist: %v", relative, err)
		}
	}
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "Code signing policy") ||
		!strings.Contains(string(readme), "docs/code-signing-policy.md") {
		t.Fatal("README must link to the Code signing policy")
	}
	policy, err := os.ReadFile(filepath.Join("..", "..", "docs", "code-signing-policy.md"))
	if err != nil {
		t.Fatal(err)
	}
	policyText := string(policy)
	for _, required := range []string{
		"Code signing policy",
		"Free code signing provided by [SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org)",
		"Committers and reviewers",
		"Approvers",
		"This program will not transfer any information to other networked systems unless specifically requested by the user or the person installing or operating it",
		"SyncHub-for-Agents-Setup-x64.exe",
	} {
		if !strings.Contains(policyText, required) {
			t.Fatalf("Code signing policy must mention %q", required)
		}
	}
	draft, err := os.ReadFile(filepath.Join("..", "..", "docs", "signpath-foundation-application.md"))
	if err != nil {
		t.Fatal(err)
	}
	draftText := string(draft)
	for _, required := range []string{
		"qinqingxu/SyncHub-for-Agents",
		"MIT License",
		"v0.3.3",
		"SignPath Foundation application draft",
		"Maintainer-provided fields still needed",
		"Do not submit guessed personal data",
	} {
		if !strings.Contains(draftText, required) {
			t.Fatalf("SignPath application draft must mention %q", required)
		}
	}
}
```

- [ ] **Step 2: Run the new test and confirm it fails**

Run:

```powershell
$env:PATH='C:\Program Files\Go\bin;' + $env:PATH
Push-Location C:\XQQ\SyncHub-for-Agents-qinqingxu
go test ./tools/repocheck -run TestSignPathFoundationApplicationMaterialsAreDocumented -count=1
Pop-Location
```

Expected: FAIL because `docs/code-signing-policy.md` and `docs/signpath-foundation-application.md` do not exist.

- [ ] **Step 3: Create the Code signing policy**

Create `docs/code-signing-policy.md` with exactly this content:

```markdown
# Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io),
certificate by [SignPath Foundation](https://signpath.org).

SyncHub for Agents publishes Windows installers from the public repository at
`qinqingxu/SyncHub-for-Agents`. Signed artifacts must be built from source code
and release scripts in that repository. The project must not sign unrelated
third-party binaries as SyncHub artifacts.

## Signed artifacts

The project intends to sign these Windows release artifacts:

- `SyncHub.exe`
- `SyncHub-for-Agents-Setup-x64.exe`

Release packages can include unsigned upstream system libraries when required by
Windows or the application framework, but SyncHub's own executables and
installer should be signed.

## Team roles

- Committers and reviewers: maintainers with write access to
  `qinqingxu/SyncHub-for-Agents`.
- Approvers: repository administrators who approve release signing requests.
- Changes from non-committers must be reviewed through pull requests before
  signed release artifacts are produced.

## Release approval

Every signing request must correspond to a GitHub release tag and a successful
release workflow run. Signing approval is a human decision and does not bypass
required CI, code-owner review, or release checks.

## Privacy policy

This program will not transfer any information to other networked systems unless
specifically requested by the user or the person installing or operating it.
SyncHub uses a private Git repository selected by the user to synchronize agent
configuration and session resources. Credentials, tokens, machine identifiers,
logs, caches, and local-only state are excluded from synchronization by design.
```

- [ ] **Step 4: Create the SignPath application draft**

Create `docs/signpath-foundation-application.md` with exactly this content:

```markdown
# SignPath Foundation application draft

This draft collects public project information for a SignPath Foundation
application. Do not submit guessed personal data. The maintainer must provide
private contact and SignPath account details directly in the SignPath form.

## Public project information

| Field | Value |
| --- | --- |
| Project name | SyncHub for Agents |
| Canonical repository | `qinqingxu/SyncHub-for-Agents` |
| Repository URL | https://github.com/qinqingxu/SyncHub-for-Agents |
| Mirror repository | https://github.com/jelllove/SyncHub-for-Agents |
| License | MIT License |
| Latest release at preparation time | v0.3.3 |
| Release artifact to sign | `SyncHub-for-Agents-Setup-x64.exe` |
| Application type | Windows desktop tray application |

## Project description

SyncHub for Agents is a desktop tray app that keeps AI agent configuration and
session data synchronized across multiple computers using a private Git
repository controlled by the user. It excludes credentials, tokens, machine IDs,
logs, caches, and local-only state from synchronization.

## Eligibility notes

- The project is released under the MIT License.
- The project has public GitHub releases and Windows installer artifacts.
- The README documents product functionality and installation steps.
- The Code signing policy is available at `docs/code-signing-policy.md`.
- The project is not a hacking tool, vulnerability scanner, or exploit tool.

## Maintainer-provided fields still needed

- Applicant name.
- Applicant contact email.
- SignPath account email.
- Confirmation that all source repository maintainers use MFA.
- Confirmation that the applicant is authorized to represent the project.
- Any additional SignPath form fields or identity checks requested during
  application review.
```

- [ ] **Step 5: Link policy from README**

In `README.md`, insert this section before `## License`:

```markdown
## Code signing policy

SyncHub for Agents intends to use SignPath Foundation for open-source Windows
code signing. See [docs/code-signing-policy.md](docs/code-signing-policy.md) for
release signing scope, team roles, approval requirements, and privacy behavior.
```

- [ ] **Step 6: Run the test and confirm it passes**

Run:

```powershell
$env:PATH='C:\Program Files\Go\bin;' + $env:PATH
Push-Location C:\XQQ\SyncHub-for-Agents-qinqingxu
go test ./tools/repocheck -run TestSignPathFoundationApplicationMaterialsAreDocumented -count=1
Pop-Location
```

Expected: PASS.

- [ ] **Step 7: Commit the policy materials**

Run:

```powershell
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu add README.md docs\code-signing-policy.md docs\signpath-foundation-application.md tools\repocheck\workflow_test.go
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu commit -m "docs: prepare SignPath Foundation application" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 2: Final validation and synchronization

**Files:**
- No additional files beyond Task 1.

- [ ] **Step 1: Run targeted and full validation**

Run:

```powershell
$env:PATH='C:\Program Files\Go\bin;' + $env:PATH
Push-Location C:\XQQ\SyncHub-for-Agents-qinqingxu
go test ./tools/repocheck -count=1
node scripts/dev.mjs docs
node scripts/dev.mjs check
node scripts/dev.mjs verify
Pop-Location
```

Expected: all commands pass.

- [ ] **Step 2: Push via PR to qinqingxu**

Because `main` is protected, create a branch and PR:

```powershell
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu switch -c docs/signpath-application-20260921
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu push -u origin docs/signpath-application-20260921
gh pr create --repo qinqingxu/SyncHub-for-Agents --base main --head docs/signpath-application-20260921 --title "Prepare SignPath Foundation application" --body "Adds the Code signing policy and SignPath Foundation application draft required before applying for SignPath Foundation signing."
```

Expected: PR is created. Wait for required checks, approve with the alternate maintainer account, and merge.

- [ ] **Step 3: Sync mirror**

After the qinqingxu PR is merged, sync the same change to `jelllove/SyncHub-for-Agents` through a PR as done for previous syncs.

- [ ] **Step 4: Handoff**

Report the policy URL and application draft path to the maintainer, and list the remaining private SignPath form fields.
