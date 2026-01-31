# bazel-remote Project Context

## Project Overview
This is the buchgr/bazel-remote repository - an HTTP/gRPC remote build cache server for Bazel that supports multiple storage backends including S3, GCS, Azure Blob, and local disk.

## Recent AWS SDK Migration (January 2026)

### What Was Done
Replaced the **minio/minio-go** library with the official **AWS SDK for Go v2** for S3 backend support.

**Reason**: Enable AWS SSO authentication support for developers who cannot use IAM access keys due to security policies.

### Key Changes

**Files Modified:**
- `cache/s3proxy/s3proxy.go` - Completely rewritten to use AWS SDK v2 client
- `config/s3.go` - Simplified to remove all authentication configuration
- `config/proxy.go` - Removed minio imports
- `config/config.go` - Removed S3 auth validation logic
- `utils/flags/flags.go` - Removed authentication-related CLI flags
- `go.mod` - Replaced minio with AWS SDK v2 dependencies
- `cache/s3proxy/auth_methods.go` - Deleted (no longer needed)

**S3 Configuration - Before vs After:**

Before (minio-based):
- Required explicit auth_method (access_key, iam_role, aws_credentials_file)
- Manual credential configuration
- Limited authentication methods

After (AWS SDK v2):
- No auth configuration needed in bazel-remote
- Uses standard AWS credential chain automatically
- Supports: AWS SSO, environment variables, shared credentials, IAM roles, web identity tokens, and all other AWS auth methods

**Remaining CLI Flags:**
- `--s3.endpoint` - S3 endpoint (optional for AWS, required for S3-compatible services)
- `--s3.bucket` - Bucket name (required)
- `--s3.prefix` - Object prefix (optional)
- `--s3.region` - AWS region (recommended)
- `--s3.disable_ssl` - Disable TLS (for local testing)
- `--s3.update_timestamps` - Update object timestamps on cache hit

**Removed Flags:**
- All authentication flags (auth_method, access_key_id, secret_access_key, session_token, signature_type, iam_role_endpoint, aws_profile, aws_shared_credentials_file, bucket_lookup_type, key_version, max_idle_conns)

### Authentication Now

Users authenticate using standard AWS methods:
```bash
# AWS SSO
aws sso login --profile myprofile
export AWS_PROFILE=myprofile

# Or environment variables
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...

# Or IAM roles (automatic on EC2/ECS/Lambda)
```

Then run bazel-remote:
```bash
./bazel-remote --s3.bucket my-cache --s3.region us-west-2
```

### Current Status
- ✅ Code migration complete
- ✅ Build passing (`bazel build //:bazel-remote`)
- ✅ Unit tests passing
- ⏳ Manual integration testing with real AWS S3 pending

### Testing Notes
- Only unit tests exist (object key formatting)
- No automated integration tests against real S3
- Manual testing required with AWS or LocalStack
- `minio-test-setup.md` needs updating for new authentication method

## Build System
- Uses Bazel with Go rules
- Go version: 1.24+
- AWS SDK v2 dependencies managed via go.mod
