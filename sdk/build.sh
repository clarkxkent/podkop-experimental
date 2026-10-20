#!/bin/sh
# Builds the package podkop-exp for one architecture inside an image openwrt/sdk (the CI job
# "podkop" runs it for each architecture of the matrix, sdk/gen-matrix.py):
#
#   docker run --rm --user root -e PODKOP_VERSION=0.8.0 -v <repo>:/src:ro -v <out>:/out \
#     [-v <cache>:/builder/dl] openwrt/sdk:<target>-<subtarget>-v<release> sh /src/sdk/build.sh
#
# Output: /out/podkop-exp-<version>-r<N>-<pkgarch>.<ipk|apk>, "_" of the architecture written as
# "-" (the names of the release, which install.sh looks for), and /out/build.log.
set -eu
SRC=${SRC:-/src}
OUT=${OUT:-/out}

if [ "$(id -u)" = 0 ]; then
	# the download cache CI restores belongs to the runner: Go must add modules to it
	[ -d /builder/dl ] && chown -R buildbot:buildbot /builder/dl
	chown buildbot:buildbot "$OUT"
	exec runuser -u buildbot -- env HOME=/builder SRC="$SRC" OUT="$OUT" PODKOP_VERSION="${PODKOP_VERSION:-}" sh "$0" "$@"
fi

cd /builder
# the feeds base (ca-bundle) and packages (Go) only, at the commits of this SDK
grep -E '^src-git(-full)? (base|packages) ' feeds.conf.default | sed 's/^src-git-full /src-git /' > feeds.conf
n=0
until ./scripts/feeds update base packages > /dev/null; do
	n=$((n + 1))
	[ $n -lt 5 ] || { echo "feeds update failed $n times" >&2; exit 1; }
	echo "feeds update failed, once more in 20 s" >&2
	sleep 20
done
sh "$SRC/sdk/golang-feed.sh"
./scripts/feeds install -p base ca-certificates > /dev/null

mkdir -p package/feeds/utilities
rm -rf package/feeds/utilities/podkop
cp -a "$SRC/podkop" package/feeds/utilities/podkop
mkdir -p package/feeds/utilities/podkop/src
cp -a "$SRC/src/backend" package/feeds/utilities/podkop/src/backend

rm -f .config
make defconfig > /dev/null 2>&1
echo 'CONFIG_PACKAGE_podkop-exp=m' >> .config
make defconfig > /dev/null 2>&1
grep -q '^CONFIG_PACKAGE_podkop-exp=m' .config || { echo "podkop-exp is not selected: a dependency is missing in the feeds" >&2; exit 1; }
pkgarch=$(sed -n 's/^CONFIG_TARGET_ARCH_PACKAGES="\(.*\)"$/\1/p' .config)
dashed=$(echo "$pkgarch" | tr '_' '-')

# opkg takes the version with the letter the releases of the project carry, apk digits only
version=${PODKOP_VERSION:-}
if [ -n "$version" ] && ! grep -q '^CONFIG_USE_APK=y' .config; then
	version=v$version
fi
rm -rf bin/packages
if ! PODKOP_VERSION=$version make package/podkop/compile -j"$(nproc)" V=s > "$OUT/build.log" 2>&1; then
	echo "BUILD FAILED ($pkgarch)"
	tail -40 "$OUT/build.log"
	exit 1
fi
found=0
find bin/packages \( -name 'podkop-exp[-_]*.ipk' -o -name 'podkop-exp[-_]*.apk' \) > /tmp/podkop-packages
while read -r f; do
	name=$(basename "$f")
	ext=${name##*.}
	case "$name" in
	*"_$pkgarch.ipk") name=$(echo "$name" | tr '_' '-') ;;
	*) name="${name%.*}-$dashed.$ext" ;;
	esac
	cp "$f" "$OUT/$name"
	ls -l "$OUT/$name"
	found=$((found + 1))
done < /tmp/podkop-packages
[ $found -eq 1 ] || { echo "expected one package podkop-exp, found $found" >&2; exit 1; }
