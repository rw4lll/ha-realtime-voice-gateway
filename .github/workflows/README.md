# GitHub Actions CI/CD Workflows

This directory contains automated CI/CD pipelines for the HA Realtime Voice Gateway.

## Workflows

### 🚀 `docker-release.yml` - Release Build & Publish

**Triggers:**
- New GitHub Release published
- Manual workflow dispatch

**What it does:**
1. Validates semantic version from release tag
2. Builds multi-arch Docker images (amd64, arm64, armv7)
3. Pushes to GitHub Container Registry with multiple tags
4. Generates SBOM and build provenance attestations
5. Runs Trivy security vulnerability scan
6. Uploads security results to GitHub Security tab

**Image Tags Created:**

| Release Type | Example Tag | Images Created |
|-------------|-------------|----------------|
| Stable | `v1.2.3` | `1.2.3`, `1.2`, `1`, `latest` |
| Pre-release | `v1.2.3-beta.1` | `1.2.3-beta.1` only |

**Usage:**
```bash
# Create and push tag
git tag -a v1.0.0 -m "Release 1.0.0"
git push origin v1.0.0

# Create GitHub Release from tag
# → Workflow automatically triggers
```

### 🧪 `docker-ci.yml` - Continuous Integration

**Triggers:**
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop`
- Only when Docker/Go files change

**What it does:**
1. **Lint Dockerfile** with Hadolint
2. **Test build** for all architectures in parallel
3. **Test image** structure and startup
4. **Security scan** with Trivy
5. **Summary** report in PR/commit

**Jobs:**
- `lint-dockerfile` - Validates Dockerfile best practices
- `test-build` - Matrix build for amd64, arm64, armv7
- `test-image` - Loads and tests AMD64 image
- `security-scan` - Vulnerability scanning
- `build-summary` - Aggregates all results

## Configuration

### Secrets Required

No secrets needed! Uses automatic `GITHUB_TOKEN` for:
- Pushing to ghcr.io
- Creating attestations
- Uploading security scans

### Environment Variables

Both workflows use:
- `REGISTRY: ghcr.io`
- `IMAGE_NAME: ${{ github.repository }}`

This automatically pushes to: `ghcr.io/rw4lll/ha-realtime-voice-gateway`

## Build Optimizations

### Caching Strategy
- **GitHub Actions Cache** - `cache-from/cache-to: type=gha`
- **BuildKit Cache Mounts** - For Go modules and build cache
- **Layer Caching** - Optimized Dockerfile layer ordering

### Parallel Builds
- All architectures build in parallel matrix
- Typical build time: 3-5 minutes per architecture

### Build Features
- BuildKit advanced features enabled
- Multi-stage builds for minimal image size
- Static binary compilation
- Strip symbols and debug info

## Security Features

### Vulnerability Scanning
- **Tool:** Trivy by Aqua Security
- **Frequency:** Every release + CI build
- **Severity:** CRITICAL, HIGH, MEDIUM
- **Output:** SARIF format → GitHub Security tab

### Supply Chain Security
- **SBOM:** Software Bill of Materials generated
- **Provenance:** Build attestation with source commit
- **Signatures:** Container signing (future)

### Best Practices
- Non-root user (UID 65534)
- Scratch base image (no shell/OS)
- Static binary (no dynamic dependencies)
- CA certificates included for HTTPS

## Troubleshooting

### Release Build Fails

**Version validation error:**
```
Error: Invalid version format: 1.0
Expected: X.Y.Z or X.Y.Z-suffix
```
**Fix:** Use proper semver tag: `v1.0.0` (not `v1.0`)

**ARM build timeout:**
**Fix:** ARM builds can take longer. Increase timeout or build locally first.

### CI Build Fails

**Hadolint errors:**
**Fix:** Check Dockerfile against warnings in workflow logs

**Security scan fails:**
**Fix:** Review vulnerabilities in Security tab, update dependencies

**Build cache issues:**
**Fix:** Clear cache by running workflow with fresh checkout

## Manual Workflow Dispatch

### Release Build
1. Go to: Actions → Docker Release Build
2. Click: "Run workflow"
3. Select branch: `main`
4. Input tag: `v1.0.0`
5. Click: "Run workflow"

### CI Build
Automatically runs on push/PR, but can be manually triggered:
1. Go to: Actions → Docker CI
2. Click: "Run workflow"
3. Select branch
4. Click: "Run workflow"

## Monitoring

### Build Status
- Check Actions tab for workflow runs
- Green ✅ = Success
- Red ❌ = Failed (click for details)

### Image Registry
View published images:
```bash
# List all tags
gh api /users/rw4lll/packages/container/ha-realtime-voice-gateway/versions

# Or visit: https://github.com/rw4lll/ha-realtime-voice-gateway/pkgs/container/ha-realtime-voice-gateway
```

### Security Alerts
- Security tab shows vulnerability scan results
- Dependabot alerts for Go dependencies
- CodeQL analysis (if enabled)

## Local Testing

Test workflows locally with [act](https://github.com/nektos/act):

```bash
# Install act
brew install act

# Run CI workflow
act push

# Run release workflow
act release -e release-event.json
```

## Performance Metrics

Typical workflow times:
- **CI Workflow:** 8-12 minutes
  - Lint: 30 seconds
  - Build (3 platforms): 3-5 min each (parallel)
  - Test: 1 minute
  - Security scan: 2 minutes

- **Release Workflow:** 10-15 minutes
  - Build & push: 8-12 minutes
  - Security scan: 2-3 minutes

## Future Enhancements

- [ ] Container image signing with cosign
- [ ] Automated changelog generation
- [ ] Performance regression testing
- [ ] Integration tests with HA
- [ ] Automated version bumping
- [ ] Release notes automation

## References

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Docker Buildx Action](https://github.com/docker/build-push-action)
- [GitHub Container Registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Trivy Security Scanner](https://github.com/aquasecurity/trivy)
