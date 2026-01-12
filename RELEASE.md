# Release Process for Amnezia Box

This document describes how to build and release AWG versions synced with upstream sing-box.

## Quick Reference

**Three release platforms (from `build-awg.yml`):**
- `linux-amd64`
- `entware-mipsel` (softfloat)
- `entware-aarch64`

**Release naming:** `v{upstream-version}-awg2.0` (e.g., `v1.13.0-beta.4-awg2.0`)

## Release Steps

### 1. Check upstream version

```bash
# Fetch upstream tags
git fetch upstream --tags

# See available upstream versions
git tag -l 'v1.13*' | sort -V | tail -10

# Check what's new since last sync
git log --oneline v1.13.0-beta.2..v1.13.0-beta.4
```

### 2. Rebase AWG commits onto new upstream version

```bash
# Rebase from old version to new version
# Format: git rebase --onto <new-base> <old-base>
git rebase --onto v1.13.0-beta.4 v1.13.0-beta.2
```

**If conflicts occur (usually `go.sum`):**
```bash
git checkout --theirs go.sum
git add go.sum
git rebase --continue
```

### 3. Sync dependencies

```bash
# Update Go modules
go mod tidy

# Update vendor directory
go mod vendor

# Apply AWG vendor patches (counter tag support)
./patches/amneziawg-go/apply.sh
```

### 4. Verify build

```bash
go build -tags "with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_awg" ./cmd/sing-box
```

### 5. Commit and push

```bash
git add -A
git commit -m "chore: sync dependencies after rebase to v1.13.0-beta.4"

# Force push required after rebase
git push --force-with-lease origin alpha
```

### 6. Trigger release build

```bash
gh workflow run build-awg.yml \
  --repo hoaxisr/amnezia-box \
  --ref alpha \
  -f version=1.13.0-beta.4 \
  -f prerelease=true
```

### 7. Monitor build

```bash
# Check workflow status
gh run list --repo hoaxisr/amnezia-box --limit 3

# Watch specific run
gh run watch --repo hoaxisr/amnezia-box <run-id>
```

## Troubleshooting

### go.sum conflicts during rebase
Always resolve with `--theirs` and run `go mod tidy` after rebase completes.

### Vendor inconsistency errors
Run `go mod vendor` to regenerate vendor directory, then reapply patches.

### gh CLI targets upstream repo
Always specify `--repo hoaxisr/amnezia-box` for workflow operations.

## Branch Strategy

- `alpha` → syncs with upstream `dev-next` (development/beta releases)
- `main` → syncs with upstream `stable-next` (stable releases)
