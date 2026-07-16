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

apt-get update
apt-get install -y craftdeck
