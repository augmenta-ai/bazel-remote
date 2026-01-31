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

## Impact

### Container Image
- **Size:** +7MB (44.2MB → 51.3MB for amd64)
- **Functionality:** Added shell and debugging utilities via busybox
- **Security:** Slightly increased attack surface (debug variant trade-off)
- **Operations:** Improved troubleshooting capabilities
- **Authentication:** Enables credential_process for custom credential providers
