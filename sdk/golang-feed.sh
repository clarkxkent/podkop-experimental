#!/bin/sh
# The Go of the SDK of OpenWrt 24.10 is too old for the backend (src/backend/go.mod asks for
# Go 1.25.5): lang/golang of the packages feed is taken at a commit of openwrt-25.12
# (Go 1.26.9, which fixes the vulnerabilities of net/http, crypto/tls and net/textproto of
# 1.26.8). Run in the directory of the SDK.
set -e
REPO=https://github.com/openwrt/packages.git
COMMIT=54fab7fa4224193a121f49a9f39dafd9323a0b2d
tmp=$(mktemp -d)
git -C "$tmp" init -q
git -C "$tmp" remote add origin "$REPO"
git -C "$tmp" sparse-checkout set lang/golang
n=0
until git -C "$tmp" fetch -q --depth 1 --filter=blob:none origin "$COMMIT"; do
	n=$((n + 1))
	[ $n -lt 5 ] || { echo "the fetch of lang/golang failed $n times" >&2; exit 1; }
	sleep 20
done
git -C "$tmp" checkout -q FETCH_HEAD
rm -rf feeds/packages/lang/golang
cp -a "$tmp/lang/golang" feeds/packages/lang/golang
rm -rf "$tmp"
./scripts/feeds update -i packages > /dev/null
./scripts/feeds install -p packages golang > /dev/null
