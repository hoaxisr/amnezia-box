NAME = sing-box
COMMIT = $(shell git rev-parse --short HEAD)
TAGS ?= with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_tailscale,with_awg

GOHOSTOS = $(shell go env GOHOSTOS)
GOHOSTARCH = $(shell go env GOHOSTARCH)
VERSION=$(shell CGO_ENABLED=0 GOOS=$(GOHOSTOS) GOARCH=$(GOHOSTARCH) go run github.com/sagernet/sing-box/cmd/internal/read_tag@latest)

PARAMS = -v -trimpath -ldflags "-X 'github.com/sagernet/sing-box/constant.Version=$(VERSION)' -s -w -buildid="
MAIN_PARAMS = $(PARAMS) -tags "$(TAGS)"
MAIN = ./cmd/sing-box
PREFIX ?= $(shell go env GOPATH)

.PHONY: test release docs build

build:
	export GOTOOLCHAIN=local && \
	go build $(MAIN_PARAMS) $(MAIN)

race:
	export GOTOOLCHAIN=local && \
	go build -race $(MAIN_PARAMS) $(MAIN)

ci_build:
	export GOTOOLCHAIN=local && \
	go build $(PARAMS) $(MAIN) && \
	go build $(MAIN_PARAMS) $(MAIN)

generate_completions:
	go run -v --tags "$(TAGS),generate,generate_completions" $(MAIN)

install:
	go build -o $(PREFIX)/bin/$(NAME) $(MAIN_PARAMS) $(MAIN)

fmt:
	@gofumpt -l -w .
	@gofmt -s -w .
	@gci write --custom-order -s standard -s "prefix(github.com/sagernet/)" -s "default" .

fmt_install:
	go install -v mvdan.cc/gofumpt@v0.8.0
	go install -v github.com/daixiang0/gci@latest

lint:
	GOOS=linux golangci-lint run ./...
	GOOS=android golangci-lint run ./...
	GOOS=windows golangci-lint run ./...
	GOOS=darwin golangci-lint run ./...
	GOOS=freebsd golangci-lint run ./...

lint_install:
	go install -v github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0

proto:
	@go run ./cmd/internal/protogen
	@gofumpt -l -w .
	@gofumpt -l -w .

proto_install:
	go install -v google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install -v google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

update_certificates:
	go run ./cmd/internal/update_certificates

release:
	go run ./cmd/internal/build goreleaser release --clean --skip publish
	mkdir dist/release
	mv dist/*.tar.gz \
		dist/*.zip \
		dist/*.deb \
		dist/*.rpm \
		dist/*_amd64.pkg.tar.zst \
		dist/*_arm64.pkg.tar.zst \
		dist/release
	ghr --replace --draft --prerelease -p 5 "v${VERSION}" dist/release
	rm -r dist/release

release_repo:
	go run ./cmd/internal/build goreleaser release -f .goreleaser.fury.yaml --clean

release_install:
	go install -v github.com/tcnksm/ghr@latest

update_android_version:
	go run ./cmd/internal/update_android_version

build_android:
	cd ../sing-box-for-android && ./gradlew :app:clean :app:assemblePlayRelease :app:assembleOtherRelease && ./gradlew --stop

upload_android:
	mkdir -p dist/release_android
	cp ../sing-box-for-android/app/build/outputs/apk/play/release/*.apk dist/release_android
	cp ../sing-box-for-android/app/build/outputs/apk/other/release/*-universal.apk dist/release_android
	ghr --replace --draft --prerelease -p 5 "v${VERSION}" dist/release_android
	rm -rf dist/release_android

release_android: lib_android update_android_version build_android upload_android

publish_android:
	cd ../sing-box-for-android && ./gradlew :app:publishPlayReleaseBundle && ./gradlew --stop

# TODO: find why and remove `-destination 'generic/platform=iOS'`
# TODO: remove xcode clean when fix control widget fixed
build_ios:
	cd ../sing-box-for-apple && \
	rm -rf build/SFI.xcarchive && \
	xcodebuild clean -scheme SFI && \
	xcodebuild archive -scheme SFI -configuration Release -destination 'generic/platform=iOS' -archivePath build/SFI.xcarchive -allowProvisioningUpdates

upload_ios_app_store:
	cd ../sing-box-for-apple && \
	xcodebuild -exportArchive -archivePath build/SFI.xcarchive -exportOptionsPlist SFI/Upload.plist -allowProvisioningUpdates

export_ios_ipa:
	cd ../sing-box-for-apple && \
	xcodebuild -exportArchive -archivePath build/SFI.xcarchive -exportOptionsPlist SFI/Export.plist -allowProvisioningUpdates -exportPath build/SFI && \
	cp build/SFI/sing-box.ipa dist/SFI.ipa

upload_ios_ipa:
	cd dist && \
	cp SFI.ipa "SFI-${VERSION}.ipa" && \
	ghr --replace --draft --prerelease "v${VERSION}" "SFI-${VERSION}.ipa"

release_ios: build_ios upload_ios_app_store

build_macos:
	cd ../sing-box-for-apple && \
	rm -rf build/SFM.xcarchive && \
	xcodebuild archive -scheme SFM -configuration Release -archivePath build/SFM.xcarchive -allowProvisioningUpdates

upload_macos_app_store:
	cd ../sing-box-for-apple && \
	xcodebuild -exportArchive -archivePath build/SFM.xcarchive -exportOptionsPlist SFI/Upload.plist -allowProvisioningUpdates

release_macos: build_macos upload_macos_app_store

build_macos_standalone:
	cd ../sing-box-for-apple && \
	rm -rf build/SFM.System.xcarchive && \
	xcodebuild archive -scheme SFM.System -configuration Release -archivePath build/SFM.System.xcarchive -allowProvisioningUpdates

build_macos_dmg:
	rm -rf dist/SFM
	mkdir -p dist/SFM
	cd ../sing-box-for-apple && \
	rm -rf build/SFM.System && \
	rm -rf build/SFM.dmg && \
	xcodebuild -exportArchive \
		-archivePath "build/SFM.System.xcarchive" \
		-exportOptionsPlist SFM.System/Export.plist -allowProvisioningUpdates \
		-exportPath "build/SFM.System" && \
	create-dmg \
		--volname "sing-box" \
		--volicon "build/SFM.System/SFM.app/Contents/Resources/AppIcon.icns" \
		--icon "SFM.app" 0 0 \
 		--hide-extension "SFM.app" \
 		--app-drop-link 0 0 \
 		--skip-jenkins \
		"../sing-box/dist/SFM/SFM.dmg" "build/SFM.System/SFM.app"

notarize_macos_dmg:
	xcrun notarytool submit "dist/SFM/SFM.dmg" --wait \
	  --keychain-profile "notarytool-password" \
  	  --no-s3-acceleration

upload_macos_dmg:
	cd dist/SFM && \
	cp SFM.dmg "SFM-${VERSION}-universal.dmg" && \
	ghr --replace --draft --prerelease "v${VERSION}" "SFM-${VERSION}-universal.dmg"

upload_macos_dsyms:
	pushd ../sing-box-for-apple/build/SFM.System.xcarchive && \
	zip -r SFM.dSYMs.zip dSYMs && \
	mv SFM.dSYMs.zip ../../../sing-box/dist/SFM && \
	popd && \
	cd dist/SFM && \
	cp SFM.dSYMs.zip "SFM-${VERSION}-universal.dSYMs.zip" && \
	ghr --replace --draft --prerelease "v${VERSION}" "SFM-${VERSION}-universal.dSYMs.zip"

release_macos_standalone: build_macos_standalone build_macos_dmg notarize_macos_dmg upload_macos_dmg upload_macos_dsyms

build_tvos:
	cd ../sing-box-for-apple && \
	rm -rf build/SFT.xcarchive && \
	xcodebuild archive -scheme SFT -configuration Release -archivePath build/SFT.xcarchive -allowProvisioningUpdates

upload_tvos_app_store:
	cd ../sing-box-for-apple && \
	xcodebuild -exportArchive -archivePath "build/SFT.xcarchive" -exportOptionsPlist SFI/Upload.plist -allowProvisioningUpdates

export_tvos_ipa:
	cd ../sing-box-for-apple && \
	xcodebuild -exportArchive -archivePath "build/SFT.xcarchive" -exportOptionsPlist SFI/Export.plist -allowProvisioningUpdates -exportPath build/SFT && \
	cp build/SFT/sing-box.ipa dist/SFT.ipa

upload_tvos_ipa:
	cd dist && \
	cp SFT.ipa "SFT-${VERSION}.ipa" && \
	ghr --replace --draft --prerelease "v${VERSION}" "SFT-${VERSION}.ipa"

release_tvos: build_tvos upload_tvos_app_store

update_apple_version:
	go run ./cmd/internal/update_apple_version

update_macos_version:
	MACOS_PROJECT_VERSION=$(shell go run -v ./cmd/internal/app_store_connect next_macos_project_version) go run ./cmd/internal/update_apple_version

release_apple: lib_ios update_apple_version release_ios release_macos release_tvos release_macos_standalone

release_apple_beta: update_apple_version release_ios release_macos release_tvos

publish_testflight:
	go run -v ./cmd/internal/app_store_connect publish_testflight

prepare_app_store:
	go run -v ./cmd/internal/app_store_connect prepare_app_store

publish_app_store:
	go run -v ./cmd/internal/app_store_connect publish_app_store

test:
	@go test -v ./... && \
	cd test && \
	go mod tidy && \
	go test -v -tags "$(TAGS_TEST)" .

test_stdio:
	@go test -v ./... && \
	cd test && \
	go mod tidy && \
	go test -v -tags "$(TAGS_TEST),force_stdio" .

lib_android:
	go run ./cmd/internal/build_libbox -target android

lib_android_debug:
	go run ./cmd/internal/build_libbox -target android -debug

lib_apple:
	go run ./cmd/internal/build_libbox -target apple

lib_ios:
	go run ./cmd/internal/build_libbox -target apple -platform ios -debug

lib:
	go run ./cmd/internal/build_libbox -target android
	go run ./cmd/internal/build_libbox -target ios

lib_install:
	go install -v github.com/sagernet/gomobile/cmd/gomobile@v0.1.8
	go install -v github.com/sagernet/gomobile/cmd/gobind@v0.1.8

docs:
	venv/bin/mkdocs serve

publish_docs:
	venv/bin/mkdocs gh-deploy -m "Update" --force --ignore-version --no-history

docs_install:
	python -m venv venv
	source ./venv/bin/activate && pip install --force-reinstall mkdocs-material=="9.*" mkdocs-static-i18n=="1.2.*"

clean:
	rm -rf bin dist sing-box
	rm -f $(shell go env GOPATH)/sing-box

update:
	git fetch
	git reset FETCH_HEAD --hard
	git clean -fdx

# =============================================================================
# Upstream Sync Commands
# =============================================================================

UPSTREAM_REPO = https://github.com/SagerNet/sing-box.git
CURRENT_BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
UPSTREAM_BRANCH = $(if $(filter main,$(CURRENT_BRANCH)),stable-next,dev-next)
PATCHES_DIR = patches/awg

.PHONY: sync-setup sync-check export-patches apply-patches sync-full

# Initial setup: add upstream remote
sync-setup:
	@git remote add upstream $(UPSTREAM_REPO) 2>/dev/null || true
	@git fetch upstream
	@echo "Upstream remote configured"

# Check for new upstream commits
sync-check: sync-setup
	@echo "Checking upstream status..."
	@echo "Current branch: $(CURRENT_BRANCH)"
	@echo "Upstream branch: $(UPSTREAM_BRANCH)"
	@echo ""
	@echo "Local HEAD:"
	@git log --oneline -1 HEAD
	@echo ""
	@echo "Upstream $(UPSTREAM_BRANCH):"
	@git log --oneline -1 upstream/$(UPSTREAM_BRANCH)
	@echo ""
	@echo "New commits from upstream:"
	@git log --oneline HEAD..upstream/$(UPSTREAM_BRANCH) 2>/dev/null | head -15 || echo "  (none)"
	@echo ""
	@BEHIND=$$(git rev-list --count HEAD..upstream/$(UPSTREAM_BRANCH) 2>/dev/null || echo "?"); \
	echo "Status: $$BEHIND commits behind upstream"

# Export AWG patches for backup/review
export-patches:
	@echo "Exporting AWG patches..."
	@rm -rf $(PATCHES_DIR)
	@mkdir -p $(PATCHES_DIR)
	@MERGE_BASE=$$(git merge-base HEAD upstream/$(UPSTREAM_BRANCH) 2>/dev/null || echo ""); \
	if [ -n "$$MERGE_BASE" ]; then \
		git format-patch $$MERGE_BASE..HEAD -o $(PATCHES_DIR) --numbered; \
	else \
		echo "No common ancestor found, exporting recent commits"; \
		git format-patch -10 HEAD -o $(PATCHES_DIR) --numbered; \
	fi
	@echo "Exported $$(ls $(PATCHES_DIR) 2>/dev/null | wc -l) patches to $(PATCHES_DIR)/"
	@ls -la $(PATCHES_DIR)/ 2>/dev/null || true

# Apply patches from backup (emergency recovery)
apply-patches:
	@echo "Applying patches from $(PATCHES_DIR)/..."
	@git am --3way $(PATCHES_DIR)/*.patch || { \
		echo "Patch application failed"; \
		echo "Run 'git am --abort' to undo, then fix manually"; \
		exit 1; \
	}
	@echo "Patches applied"

# Full sync: rebase current branch onto upstream
sync-full: sync-setup export-patches
	@echo "Syncing $(CURRENT_BRANCH) with upstream/$(UPSTREAM_BRANCH)..."
	@git rebase upstream/$(UPSTREAM_BRANCH) || { \
		echo ""; \
		echo "Rebase conflicts detected!"; \
		echo ""; \
		echo "To resolve:"; \
		echo "  1. Fix conflicts in the listed files"; \
		echo "  2. git add <fixed-files>"; \
		echo "  3. git rebase --continue"; \
		echo ""; \
		echo "To abort: git rebase --abort"; \
		echo "Patches are backed up in $(PATCHES_DIR)/"; \
		exit 1; \
	}
	@echo ""
	@echo "Sync complete!"

# Show sync status and help
sync-help:
	@echo "Upstream Sync Commands:"
	@echo ""
	@echo "  make sync-setup      - Add upstream remote"
	@echo "  make sync-check      - Check for new upstream commits"
	@echo "  make export-patches  - Export AWG commits as patch files"
	@echo "  make sync-full       - Sync current branch with upstream"
	@echo "  make apply-patches   - Apply patches from backup"
	@echo ""
	@echo "Branch mapping:"
	@echo "  alpha -> upstream/dev-next (alpha releases)"
	@echo "  main  -> upstream/stable-next (stable releases)"
	@echo ""
	@echo "Workflow:"
	@echo "  1. make sync-check       # See what's new"
	@echo "  2. make sync-full        # Sync and rebase"
	@echo "  3. make build            # Verify build works"
	@echo ""
