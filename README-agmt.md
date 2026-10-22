# AGMT Fork - Changes Documentation

## Purpose

This fork contains modifications to the base buchgr/bazel-remote project to support modern AWS authentication methods and operational requirements.

**Key Driver:** Enterprise security policies increasingly require AWS SSO (IAM Identity Center) and credential_process-based authentication, prohibiting the use of long-lived IAM access keys. The original implementation used the minio-go library, which has limited AWS authentication support and cannot leverage the full AWS SDK credential chain. Migrating to the official AWS SDK for Go v2 enables:

- **AWS SSO (IAM Identity Center)** - Modern federated authentication
- **credential_process** - Custom credential providers and secret manager integration
- **Web Identity Tokens** - For Kubernetes/EKS workloads
- **Automatic credential refresh** - For temporary credentials
- **All standard AWS authentication methods** - Without manual configuration

The base image was also updated to include a shell, which is required for credential_process to execute external credential commands.

## Changes Made

### 1. AWS SDK Migration (January 2026)

**Changed:** S3 backend from minio/minio-go library to AWS SDK for Go v2

**Files Modified:**
- `cache/s3proxy/s3proxy.go` - Rewritten to use AWS SDK v2
- `cache/s3proxy/auth_methods.go` - Deleted (no longer needed)
- `cache/s3proxy/BUILD.bazel` - Updated dependencies
- `config/s3.go` - Removed authentication config fields
- `config/config.go` - Removed S3 auth validation
- `config/proxy.go` - Removed minio credential handling
- `utils/flags/flags.go` - Removed S3 authentication flags
- `go.mod` / `go.sum` - Replaced minio with AWS SDK v2

**Rationale:**
- Enable AWS SSO authentication for developers with security policies prohibiting IAM access keys
- Use standard AWS credential chain (SSO, environment variables, IAM roles, shared credentials, etc.)
- Simplify configuration by removing manual authentication setup
- Support all AWS authentication methods automatically

**Impact:**
- All S3 authentication flags removed from CLI
- Users authenticate via standard AWS methods before running bazel-remote
- No configuration changes needed for IAM role-based auth (EC2/ECS/Lambda)

### 2. Docker Base Image Update

**Changed:** Container base image from distroless `base-nossl` to `debug` variant

**Files Modified:**
- `MODULE.bazel` - Updated `oci.pull()` configurations for both amd64 and arm64
- `BUILD.bazel` - Updated comments about base image

**Before:**
```starlark
image = "gcr.io/distroless/base-nossl-debian12"
tag = "nonroot-amd64" / "nonroot-arm64"
```

**After:**
```starlark
image = "gcr.io/distroless/base-debian12"
tag = "debug-nonroot-amd64" / "debug-nonroot-arm64"
```

**Rationale:**
- **Primary:** Enable AWS `credential_process` authentication method, which requires a shell to execute external credential commands
- Allows custom credential providers (scripts that fetch credentials from secret managers, SAML providers, etc.)
- Enables `docker exec` access to running containers for operational debugging and troubleshooting
- Maintains CA certificates required for AWS SDK S3 connections
- Maintains nonroot user security model (UID 65532)
- Minimal size increase (~7MB, from 44MB to 51MB)

**credential_process Support:**
The AWS SDK v2 supports sourcing credentials from external processes configured in `~/.aws/config`:
```ini
[profile myprofile]
credential_process = /usr/local/bin/custom-credential-helper
```
This requires a shell (`sh`) to execute the credential command, which is provided by the busybox included in the debug image variant.

**Security Note:** Debug images include additional utilities that increase attack surface. Use with appropriate security controls.

### 3. Debian 13 Base (October 2026)

**Changed:** Base image from `base-debian12` to `base-debian13`, pinned by digest. See [ACORN-15711](https://augmenta.atlassian.net/browse/ACORN-15711).

**Files Modified:**
- `MODULE.bazel` - Both `oci.pull()` entries
- `MODULE.bazel.lock` - The `oci` extension entries
- `docker/push_to_ecr` - New. Checks the image and pushes it to ECR. See below.

**After:**
```starlark
image = "gcr.io/distroless/base-debian13"
digest = "sha256:343e68d5..."  # tag = "debug-nonroot-amd64"
digest = "sha256:cc0ab6d9..."  # tag = "debug-nonroot-arm64"
```

**Rationale:**
- Go before 1.27.3 trusts the compromised eMudhra CA chains. Upstream's Go 1.27.3 rejects their intermediates by key hash ([golang/go@90617fb](https://github.com/golang/go/commit/90617fb7ceb59cf08bee7826bb51b8c8fb8c11de)), whatever the CA bundle says, so this fork keeps upstream's Go version.
- The Debian 12 base ships `emSign ECC Root CA - G3` and `AAA Certificate Services` in its CA bundle. It will not get a fix, because distroless stopped building Debian 12 on 2026-09-10 ([GoogleContainerTools/distroless@3833bee](https://github.com/GoogleContainerTools/distroless/commit/3833bee1a)). The Debian 13 bundle has neither root.
- A digest makes the base reproducible. The tag stays next to it to record where the digest came from. To move to a newer base, read the tag's current digest with `docker buildx imagetools inspect gcr.io/distroless/base-debian13:debug-nonroot-amd64`.

**Impact:** The amd64 image grows from 52.3MB to 55.9MB.

## Building and Publishing the Image

No workflow publishes the image, and the `push_to_dockerhub_*` targets in `BUILD.bazel` push to upstream's Docker Hub. Build it locally and push it to ECR by hand, on a trusted network, for amd64 only.

1. Check out the `agmt1` commit you are publishing, with a clean tree. The binary records the commit hash, and the image tag names it.
2. In this repo's devcontainer, build the image and load it into Docker:
   ```
   bazel run //:bazel-remote-image-tarball
   ```
   The devcontainer uses the host's Docker, so this loads `buchgr/bazel-remote-cache:tmp-amd64` on the host.
3. On the host, check the image, then push it. The devcontainer has no AWS CLI, so this step cannot run there. Run it from the same clean checkout:
   ```
   docker/push_to_ecr --dry-run
   docker/push_to_ecr
   ```
   The script stops before it pushes unless all of these hold:
   - The image's startup log says it was built with the Go version that `go_sdk.download()` names in `MODULE.bazel`, from the checked-out commit.
   - `-h` lists `"cgo"` among the supported zstd implementations.
   - The image's CA bundle holds neither `emSign ECC Root CA - G3` nor `AAA Certificate Services`.
   - Docker Engine is 29.9.2 or later and AWS CLI is 2.38.4 or later. Older releases of both trust the compromised chains, and both send the ECR credential.

   It then tags the image `agmt1-<short sha>`, logs in to ECR, pushes, and prints the image's ECR digest. `--dry-run` runs the checks and stops. The ECR repository's tags are immutable, so a pushed tag cannot be pushed again.
4. agmt-bootstrap's driver names the bazel-remote image tag that it starts by default. After the push, move it to the new tag.

## Impact

### Container Image
- **Size:** 55.9MB for amd64. The debug variant took it from 44.2MB to 51.3MB, and Debian 13 adds about 3.6MB.
- **Functionality:** Added shell and debugging utilities via busybox
- **Security:** Slightly increased attack surface (debug variant trade-off)
- **Operations:** Improved troubleshooting capabilities
- **Authentication:** Enables credential_process for custom credential providers
