#!/usr/bin/env bash
# 发版脚本。main 分支受保护（必须走 PR），所以发版分两步：
#
#   make release      → scripts/release.sh prepare
#       把 changelog 分支重置到 origin/main，生成并编辑 CHANGELOG，
#       提交后推送 changelog 分支并开 PR（已有 PR 则复用）。
#   （在 GitHub 上合并 PR）
#   make release-tag  → scripts/release.sh tag [VERSION]
#       从 origin/main 的 CHANGELOG.md 顶部读出版本号，
#       在 origin/main 上打（签名）tag 并推送，触发镜像与数据发布。
#
# changelog 分支是一次性的：每次发版都会从 origin/main 重建并强推，
# 不要在上面放其他改动。
set -euo pipefail

RELEASE_BRANCH=changelog
REMOTE=origin

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { printf "${BLUE}%s${NC}\n" "$*"; }
ok() { printf "${GREEN}✓ %s${NC}\n" "$*"; }
warn() { printf "${YELLOW}%s${NC}\n" "$*"; }
die() {
	printf "${RED}Error: %s${NC}\n" "$*" >&2
	exit 1
}

require() {
	command -v "$1" >/dev/null 2>&1 || die "$1 not found, install it with: $2"
}

require_clean_tree() {
	if [ -n "$(git status --porcelain)" ]; then
		die "working directory has uncommitted changes, commit or stash them first"
	fi
}

confirm() {
	printf "${YELLOW}%s [y/N] ${NC}" "$1"
	read -r reply
	[ "$reply" = "y" ] || [ "$reply" = "Y" ]
}

# version_gt A B：A 严格大于 B（均为不带 v 的 X.Y.Z）
version_gt() {
	local a_major a_minor a_patch b_major b_minor b_patch
	IFS=. read -r a_major a_minor a_patch <<<"$1"
	IFS=. read -r b_major b_minor b_patch <<<"$2"
	[ "$a_major" -ne "$b_major" ] && { [ "$a_major" -gt "$b_major" ]; return; }
	[ "$a_minor" -ne "$b_minor" ] && { [ "$a_minor" -gt "$b_minor" ]; return; }
	[ "$a_patch" -gt "$b_patch" ]
}

tag_exists() {
	git rev-parse -q --verify "refs/tags/$1" >/dev/null
}

prepare() {
	require git-cliff "brew install git-cliff"
	require gh "brew install gh"
	require_clean_tree

	info "Fetching $REMOTE..."
	git fetch --quiet --tags "$REMOTE"

	local latest_tag
	latest_tag=$(git describe --tags --abbrev=0 "$REMOTE/main" 2>/dev/null || echo "none")

	info "════════════════════════════════════════"
	info "         Release New Version"
	info "════════════════════════════════════════"
	printf "${CYAN}Current latest tag: ${GREEN}%s${NC}\n" "$latest_tag"
	printf "${YELLOW}Enter new version: ${NC}"
	local version
	read -r version
	[ -n "$version" ] || die "version cannot be empty"
	[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "invalid version format '$version', expected v1.0.0"
	tag_exists "$version" && die "tag $version already exists"
	if [ "$latest_tag" != "none" ] && ! version_gt "${version#v}" "${latest_tag#v}"; then
		die "new version $version must be greater than $latest_tag"
	fi

	# 全局变量：EXIT trap 在函数返回后才执行，local 变量那时已失效
	ORIG_BRANCH=$(git branch --show-current)

	# 结束时（含失败、Ctrl-C）丢弃未提交的 CHANGELOG 改动并回到原分支
	cleanup() {
		git checkout --quiet -- CHANGELOG.md 2>/dev/null || true
		if [ -n "$ORIG_BRANCH" ] && [ "$(git branch --show-current)" != "$ORIG_BRANCH" ]; then
			git switch --quiet "$ORIG_BRANCH" || true
		fi
	}
	trap cleanup EXIT
	trap 'exit 130' INT TERM

	info "Resetting $RELEASE_BRANCH to $REMOTE/main..."
	git switch --quiet -C "$RELEASE_BRANCH" "$REMOTE/main"

	info "Updating CHANGELOG.md..."
	git cliff --unreleased --tag "$version" --prepend CHANGELOG.md 2>/dev/null

	if git diff --quiet CHANGELOG.md; then
		warn "⚠ No user-facing changes since $latest_tag, CHANGELOG.md unchanged."
		warn "Nothing to merge; tag $REMOTE/main directly with:"
		printf "    make release-tag VERSION=%s\n" "$version"
		return
	fi

	git --no-pager diff --stat CHANGELOG.md
	warn "CHANGELOG.md updated. Review/edit it now (e.g. translate English entries),"
	warn "then press Enter to commit and open a PR, or Ctrl-C to abort."
	read -r _

	git add CHANGELOG.md
	# chore 类型会被 cliff.toml 跳过，这个提交不会出现在下个版本的 CHANGELOG 里
	git commit --quiet -m "chore(release): $version"

	info "Pushing $RELEASE_BRANCH..."
	git push --quiet --force-with-lease "$REMOTE" "$RELEASE_BRANCH"

	local pr_url
	pr_url=$(gh pr list --head "$RELEASE_BRANCH" --base main --state open --json url --jq '.[0].url // empty')
	if [ -n "$pr_url" ]; then
		ok "Updated existing PR: $pr_url"
	else
		pr_url=$(gh pr create --base main --head "$RELEASE_BRANCH" \
			--title "chore(release): $version" \
			--body "Update CHANGELOG.md for $version. After merging, run \`make release-tag\` to tag and publish.")
		ok "Opened PR: $pr_url"
	fi

	echo ""
	warn "Next steps:"
	echo "  1. Merge the PR above into main (squash merge keeps the title 'chore(release): $version')"
	echo "  2. Run: make release-tag"
}

tag() {
	require_clean_tree

	info "Fetching $REMOTE..."
	git fetch --quiet --tags "$REMOTE"

	local version="${1:-}"
	if [ -z "$version" ]; then
		local heading
		heading=$(git show "$REMOTE/main:CHANGELOG.md" | grep -m1 -oE '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' || true)
		[ -n "$heading" ] || die "cannot read a version from $REMOTE/main:CHANGELOG.md; pass VERSION=vX.Y.Z"
		version="v$(echo "$heading" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')"
	fi
	[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "invalid version format '$version', expected v1.0.0"

	if tag_exists "$version"; then
		die "tag $version already exists. Has the release PR been merged yet? (CHANGELOG.md on $REMOTE/main still shows the previous version)"
	fi

	local latest_tag
	latest_tag=$(git describe --tags --abbrev=0 "$REMOTE/main" 2>/dev/null || echo "none")
	if [ "$latest_tag" != "none" ] && ! version_gt "${version#v}" "${latest_tag#v}"; then
		die "version $version must be greater than $latest_tag"
	fi

	info "Tag $version will point at $REMOTE/main:"
	git --no-pager log -1 --format='  %h %s (%an, %ar)' "$REMOTE/main"
	confirm "Create and push tag $version?" || {
		warn "Aborted"
		exit 1
	}

	if git config user.signingkey >/dev/null 2>&1 && command -v gpg >/dev/null 2>&1 &&
		git tag -s "$version" "$REMOTE/main" -m "Release $version" 2>/dev/null; then
		ok "Signed tag $version created (Verified ✓)"
	else
		git tag -a "$version" "$REMOTE/main" -m "Release $version"
		ok "Tag $version created"
		warn "💡 Tip: configure a GPG key to get the Verified badge"
	fi

	info "Pushing tag to $REMOTE..."
	git push --quiet "$REMOTE" "$version"
	ok "Release $version completed"
}

case "${1:-}" in
prepare) prepare ;;
tag) tag "${2:-}" ;;
*)
	echo "usage: $0 prepare | tag [VERSION]" >&2
	exit 2
	;;
esac
