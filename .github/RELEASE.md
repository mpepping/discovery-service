# Release Process

This document describes how to create a new release of the Discovery Service.

## Prerequisites

### GitHub Repository Permissions

The release workflow requires the following permissions to be configured in your GitHub repository:

#### 1. Workflow Permissions (REQUIRED)

Go to: **Repository Settings → Actions → General → Workflow permissions**

Set to: **Read and write permissions**

This allows GitHub Actions to:
- Create releases
- Upload release artifacts
- Write release notes

**OR** ensure the workflow has `permissions: contents: write` in the workflow file (already configured).

#### 2. Enable GitHub Actions (if not already enabled)

Go to: **Repository Settings → Actions → General → Actions permissions**

Set to: **Allow all actions and reusable workflows**

## Creating a Release

### 1. Update Version

Ensure your code is ready for release and all changes are committed.

### 2. Create and Push a Tag

Create a tag following semantic versioning (v0.0.1, v1.0.0, etc.):

```bash
# Create a tag
git tag v0.1.0

# Push the tag to GitHub
git push origin v0.1.0
```

### 3. Automatic Build Process

Once the tag is pushed, GitHub Actions will automatically:

1. **Checkout the code** at the tagged version
2. **Set up Go 1.23** build environment
3. **Build binaries** for:
   - Linux AMD64 (`discovery-service-linux-amd64`)
   - Linux ARM64 (`discovery-service-linux-arm64`)
4. **Generate SHA256 checksums** for each binary
5. **Create a GitHub Release** with:
   - Release notes
   - Binary downloads
   - Checksum files

### 4. Monitor the Build

Go to: **Repository → Actions → Release workflow**

Watch the build progress. The workflow typically takes 2-3 minutes.

### 5. Verify the Release

After the workflow completes:

1. Go to: **Repository → Releases**
2. Find your new release (e.g., `v0.1.0`)
3. Verify all artifacts are present:
   - `discovery-service-linux-amd64`
   - `discovery-service-linux-amd64.sha256`
   - `discovery-service-linux-arm64`
   - `discovery-service-linux-arm64.sha256`
   - `checksums.txt`

## Release Artifacts

Each release includes:

### Binaries

- **discovery-service-linux-amd64** - 64-bit Intel/AMD Linux binary
- **discovery-service-linux-arm64** - 64-bit ARM Linux binary (for ARM servers, Raspberry Pi 4+)

### Checksums

- **discovery-service-linux-amd64.sha256** - SHA256 checksum for AMD64 binary
- **discovery-service-linux-arm64.sha256** - SHA256 checksum for ARM64 binary
- **checksums.txt** - Combined checksums file

### Verifying Downloads

Users can verify downloaded binaries using:

```bash
# Verify AMD64
sha256sum -c discovery-service-linux-amd64.sha256

# Verify ARM64
sha256sum -c discovery-service-linux-arm64.sha256
```

## Troubleshooting

### Workflow fails with "Resource not accessible by integration"

**Solution**: Update workflow permissions
1. Go to: **Repository Settings → Actions → General → Workflow permissions**
2. Set to: **Read and write permissions**
3. Click **Save**
4. Re-run the failed workflow

### Tag already exists error

**Solution**: Delete and recreate the tag

```bash
# Delete local tag
git tag -d v0.1.0

# Delete remote tag
git push origin :refs/tags/v0.1.0

# Create new tag
git tag v0.1.0

# Push new tag
git push origin v0.1.0
```

### Build fails

**Solution**: Check the Actions tab for detailed error logs
1. Go to: **Repository → Actions**
2. Click on the failed workflow run
3. Click on the failed job
4. Review the logs for specific errors

## Version Numbering

Follow [Semantic Versioning](https://semver.org/):

- **v0.0.1** - Initial development releases
- **v0.1.0** - Minor version bump (new features, backward compatible)
- **v1.0.0** - Major version (stable release)
- **v1.1.0** - Minor update (new features)
- **v1.1.1** - Patch update (bug fixes)

### Pre-release Versions

For pre-releases, use suffixes:
- **v0.1.0-alpha.1** - Alpha release
- **v0.1.0-beta.1** - Beta release
- **v0.1.0-rc.1** - Release candidate

## Example: Creating Your First Release

```bash
# Ensure you're on the main branch and up to date
git checkout main
git pull origin main

# Create a tag for version 0.1.0
git tag -a v0.1.0 -m "Initial release"

# Push the tag
git push origin v0.1.0

# Watch the build at: https://github.com/mpepping/discovery-service/actions
```

After the workflow completes (2-3 minutes), your release will be available at:
`https://github.com/mpepping/discovery-service/releases/tag/v0.1.0`
