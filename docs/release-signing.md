# Windows release signing

SyncHub's Windows release workflow can Authenticode-sign both
`SyncHub.exe` and `SyncHub-for-Agents-Setup-x64.exe`. Unsigned releases still
include `SHA256SUMS.txt` for download integrity, but Windows SmartScreen can
show unknown-publisher warnings until a valid publisher signature and reputation
exist.

## Certificate options

Use one of these signing setups:

- **PFX code-signing certificate**: easiest path for GitHub-hosted runners.
- **Installed certificate thumbprint**: useful only when the runner already has
  the private key available in its certificate store.
- **Microsoft Trusted Signing / Azure Trusted Signing**: CI-friendly long-term
  option, but it requires a separate workflow integration.

An EV certificate usually gains SmartScreen reputation faster. An OV
certificate still removes the unsigned-publisher state, but SmartScreen
reputation can take time to build.

## GitHub release environment configuration

The release job runs in the `release` environment. Configure the following
environment secrets and variables for the repository that publishes releases.

### Environment secrets

| Secret | Purpose |
| --- | --- |
| `WINDOWS_CERTIFICATE_BASE64` | Base64-encoded `.pfx` code-signing certificate. |
| `WINDOWS_CERTIFICATE_PASSWORD` | Password for the `.pfx` certificate. |

### Environment variables

| Variable | Recommended value | Purpose |
| --- | --- | --- |
| `WINDOWS_REQUIRE_SIGNING` | `true` | Fails the release if signing inputs are missing or invalid. |
| `WINDOWS_TIMESTAMP_SERVER` | `http://timestamp.digicert.com` | RFC 3161 timestamp server used by `signtool`. |
| `WINDOWS_SIGN_THUMBPRINT` | certificate thumbprint | Optional alternative to `WINDOWS_CERTIFICATE_BASE64` when the private key is preinstalled on the runner. |

## Encoding a PFX certificate

Run this locally, then paste the generated text into
`WINDOWS_CERTIFICATE_BASE64`:

```powershell
$bytes = [IO.File]::ReadAllBytes("C:\path\to\publisher-certificate.pfx")
[Convert]::ToBase64String($bytes) |
  Set-Content "C:\path\to\windows-certificate-base64.txt" -Encoding ascii
```

Store the PFX password in `WINDOWS_CERTIFICATE_PASSWORD`.

## Release behavior

When signing is configured, `scripts/release/windows.ps1`:

1. Writes the PFX to a temporary file under `bin/`.
2. Runs `signtool sign` for `SyncHub.exe`.
3. Runs `signtool verify /pa /all` for `SyncHub.exe`.
4. Builds the NSIS installer.
5. Runs `signtool sign` for `SyncHub-for-Agents-Setup-x64.exe`.
6. Runs `signtool verify /pa /all` for the installer.
7. Deletes the temporary PFX file in the cleanup block.

If `WINDOWS_REQUIRE_SIGNING=true`, release packaging fails instead of publishing
an unsigned installer when signing cannot be completed.

## Local verification

After downloading a release asset, verify the signature with:

```powershell
Get-AuthenticodeSignature .\SyncHub-for-Agents-Setup-x64.exe
```

Expected:

```text
Status : Valid
```

If the Windows SDK is installed, also run:

```powershell
signtool verify /pa /all .\SyncHub-for-Agents-Setup-x64.exe
```

Passing signature verification proves publisher identity and file integrity for
the signed artifact. It does not guarantee that SmartScreen will immediately stop
showing reputation warnings for a newly issued certificate.
