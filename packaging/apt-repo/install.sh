#!/bin/sh
# One-liner installer: curl -fsSL https://apt.apple-farm.online/install.sh | sudo bash
#
# Registers this apt repository (signed with craftdeck-archive-keyring.gpg,
# published alongside this script by .github/workflows/release.yml) and
# installs the craftdeck package. Idempotent -- safe to re-run.
set -e

if [ "$(id -u)" -ne 0 ]; then
	echo "이 스크립트는 root 권한이 필요합니다 -- sudo로 다시 실행해주세요." >&2
	exit 1
fi

REPO_URL="https://apt.apple-farm.online"
KEYRING=/usr/share/keyrings/craftdeck.gpg
SOURCES_LIST=/etc/apt/sources.list.d/craftdeck.list

curl -fsSL "$REPO_URL/craftdeck-archive-keyring.gpg" -o "$KEYRING"
echo "deb [signed-by=$KEYRING] $REPO_URL trixie main" > "$SOURCES_LIST"

# The Adoptium repo has to be registered *before* `apt-get install craftdeck`
# runs, not from that package's own postinst -- postinst executes nested
# inside this same apt-get transaction, and a second apt-get call from
# there would deadlock on the dpkg lock this one already holds rather than
# just failing (see packaging/scripts/postinst's comment). Registering it
# here instead lets apt resolve craftdeck's Adoptium Depends (temurin-*-jre)
# as part of this single transaction, same as any normal package dependency.
ADOPTIUM_KEYRING=/usr/share/keyrings/adoptium.gpg
ADOPTIUM_LIST=/etc/apt/sources.list.d/adoptium.list
if [ ! -f "$ADOPTIUM_LIST" ]; then
	curl -fsSL https://packages.adoptium.net/artifactory/api/gpg/key/public | gpg --dearmor -o "$ADOPTIUM_KEYRING"
	echo "deb [signed-by=$ADOPTIUM_KEYRING] https://packages.adoptium.net/artifactory/deb $(awk -F= '/^VERSION_CODENAME/{print $2}' /etc/os-release) main" \
		> "$ADOPTIUM_LIST"
fi

apt-get update
apt-get install -y craftdeck
