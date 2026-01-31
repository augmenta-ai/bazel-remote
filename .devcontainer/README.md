# bazel-remote Development Container

This devcontainer provides a complete development environment for bazel-remote with:

- **Go 1.24** - Latest Go toolchain
- **Bazel** - Build system
- **Build tools** - gcc, g++, make
- **Git** - Version control
- **VS Code extensions** - Go, Bazel, GitHub Copilot

## Features

### AWS Credentials
Your local `~/.aws` directory is mounted into the container, so AWS SSO and other authentication methods work seamlessly.

### Ports
- `8080` - HTTP cache endpoint
- `9092` - gRPC cache endpoint

## Quick Start

1. Open this repository in VS Code
2. Click "Reopen in Container" when prompted (or use Command Palette: "Dev Containers: Reopen in Container")
3. Wait for the container to build and initialize
4. Build the project: `bazel build //:bazel-remote`

## Testing with AWS S3

```bash
# Authenticate with AWS SSO
aws sso login --profile your-profile

# Set your profile
export AWS_PROFILE=your-profile

# Run bazel-remote with S3 backend
./bazel-bin/bazel-remote_/bazel-remote \
  --s3.bucket your-bucket \
  --s3.region us-west-2
```

## Common Commands

```bash
# Build
bazel build //:bazel-remote

# Run tests
bazel test //...

# Run specific test
bazel test //cache/s3proxy:go_default_test

# Clean
bazel clean
```
