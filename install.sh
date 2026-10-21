#!/bin/sh
# shellcheck shell=dash

# install.sh: podkop-exp of the latest release of podkop-net/podkop-experimental. Checks the
# router (OpenWrt, memory, kmod-nft-tproxy, a package for its architecture, the flash),
# removes what podkop-exp cannot work beside, after asking, and installs podkop-exp, its
# page and, if wanted, the Russian translation. The settings of podkop stay.

REPO="https://api.github.com/repos/podkop-net/podkop-experimental/releases/latest"
DOWNLOAD_DIR="/tmp/podkop"
COUNT=3
# the backend and sing-box inside it run in about 30-40 MB; below this a router has no room
# left for the rest of the system
RAM_MIN_MB=80
# what the page, the translation and the dependencies of podkop-exp take, with a margin, KB
RESERVE_KB=1024

PKG_IS_APK=0
command -v apk >/dev/null 2>&1 && PKG_IS_APK=1

rm -rf "$DOWNLOAD_DIR"
mkdir -p "$DOWNLOAD_DIR"

msg() {
    printf "\033[32;1m%s\033[0m\n" "$1"
}

err() {
    printf "\033[31;1m%s\033[0m\n" "$1"
}

die() {
    err "$1"
    rm -rf "$DOWNLOAD_DIR"
    exit 1
}

# ask QUESTION: 0 for yes; anything else, an empty answer or the end of the input is no
ask() {
    local answer
    msg "$1 (y/N)"
    read -r answer || return 1
    case "$answer" in
    y | Y | yes | Yes | д | Д | да | Да) return 0 ;;
    esac
    return 1
}

# the names of the installed packages, a name a line
installed_names() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        sed -n 's/^P://p' /lib/apk/db/installed 2>/dev/null
    else
        opkg list-installed 2>/dev/null | awk '{print $1}'
    fi
}

pkg_is_installed() {
    installed_names | grep -qx "$1"
}

pkg_remove() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        apk del "$@"
    else
        opkg remove --force-depends "$@"
    fi
}

# the scripts of podkop 0.7 stop its services when it goes, and say "Command failed: ubus
# call service delete ... (Not found)" for one that does not run: those lines are left out
quiet() {
    grep -v '^Command failed: ubus call service delete'
}

pkg_list_update() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        apk update
    else
        opkg update
    fi
}

pkg_install() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        # a package that is not from the feeds of OpenWrt
        apk add --allow-untrusted "$@"
    else
        # shellcheck disable=SC2086
        opkg install $OPKG_SPACE "$@"
    fi
}

# the package architectures of the router, the one of its release of OpenWrt first, then
# those its package manager takes. "apk --print-arch" is the architecture of apk itself
# (aarch64 on an aarch64_cortex-a53 router): it comes last.
pkg_archs() {
    {
        sed -n "s/^DISTRIB_ARCH='\(.*\)'$/\1/p" /etc/openwrt_release
        if [ "$PKG_IS_APK" -eq 1 ]; then
            cat /etc/apk/arch 2>/dev/null
            apk --print-arch
        else
            # "arch <name> <priority>": the highest priority first, all and noarch left out
            opkg print-architecture 2>/dev/null | awk '$2 != "all" && $2 != "noarch" { print $3, $2 }' | sort -rn | awk '{ print $2 }'
        fi
    } | awk 'NF && !seen[$0]++'
}

# the packages besides podkop's own that need a package, a name a line
needed_by() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        # "<pkg>-<version> is required by:" and then the packages, with their versions
        apk info -r "$1" 2>/dev/null | sed '1d' | grep -v -e '^$' -e '^podkop-[v0-9]' -e '^luci-app-podkop-[v0-9]'
    else
        # "Root set:", the names, "What depends on root set" and then a line a package:
        # "<tab><pkg> <version><tab>depends on <name>" (tabs, not spaces)
        opkg whatdepends "$1" 2>/dev/null | awk '/depends on/ && $1 != "What" {print $1}' | grep -v -x -e podkop -e luci-app-podkop
    fi
}

# --- the flash

# the file system the packages are written to: the one of /overlay, or of / without it
flash_fs() {
    awk '$2 == "/overlay" { o = $3 } $2 == "/" { r = $3 } END { print (o != "" ? o : r) }' /proc/mounts
}

# ubifs and jffs2 compress what they keep: a package takes about half of its size there.
# Measured for the Go binary of sing-box on ubifs (zstd, zlib, lzo) and jffs2: 40-58%.
fs_compresses() {
    case "$(flash_fs)" in
    ubifs | jffs2) return 0 ;;
    esac
    return 1
}

# free KB of the flash. ubifs counts the space of its journal as taken until it is written
# out: without the sync df shows megabytes less right after a large write or removal.
free_kb() {
    sync
    df -kP /overlay 2>/dev/null | awk 'NR == 2 { print $4 }' | grep . || df -kP / | awk 'NR == 2 { print $4 }'
}

# what a package of the size $1 KB (unpacked) needs on the flash, KB
need_kb() {
    if fs_compresses; then
        echo $(($1 * 50 / 100))
    else
        echo $(($1 * 115 / 100))
    fi
}

# the files of an installed package, a path a line, without the leading /
pkg_files() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        # "<pkg>-<version> contains:" and then the paths
        apk info -L "$1" 2>/dev/null | sed '1d' | grep -v '^$'
    else
        cut -f1 "/usr/lib/opkg/info/$1.list" 2>/dev/null | sed 's|^/||'
    fi
}

# KB the removal of an installed package frees on the flash: its files that are written
# there. A package of the firmware image lives in /rom, and its removal frees nothing; on a
# compressing file system a file takes 40% of its size or more, and 40% is counted.
pkg_freed_kb() {
    local upper="" kb
    [ -d /overlay/upper ] && upper=/overlay/upper
    kb=$(pkg_files "$1" | while read -r f; do
        [ -f "$upper/$f" ] && [ ! -L "$upper/$f" ] && wc -c < "$upper/$f"
    done | awk '{ s += $1 } END { print int(s / 1024) }')
    if fs_compresses; then
        echo $((kb * 40 / 100))
    else
        echo "$kb"
    fi
}

# the unpacked size of a downloaded package, KB
file_installed_kb() {
    local bytes=""
    case "$1" in
    *.ipk) bytes=$(tar -xzOf "$1" ./control.tar.gz 2>/dev/null | tar -xzO ./control 2>/dev/null | sed -n 's/^Installed-Size: *//p') ;;
    *.apk) bytes=$(apk adbdump "$1" 2>/dev/null | sed -n 's/^ *installed-size: *//p' | head -n 1) ;;
    esac
    case "$bytes" in
    '' | *[!0-9]*)
        # a Go binary packs to about a third
        bytes=$(($(wc -c < "$1") * 3))
        ;;
    esac
    echo $((bytes / 1024))
}

mb() {
    awk -v k="$1" 'BEGIN { printf "%.1f MB", k / 1024 }'
}

# --- the checks

check_system() {
    local release major ram_mb

    MODEL=$(cat /tmp/sysinfo/model 2>/dev/null)
    msg "Router model: $MODEL"

    release=$(sed -n "s/^DISTRIB_RELEASE='\(.*\)'$/\1/p" /etc/openwrt_release)
    major=$(echo "$release" | cut -d. -f1)
    case "$release" in
    SNAPSHOT* | *SNAPSHOT) ;;
    *)
        if ! [ "$major" -ge 24 ] 2>/dev/null; then
            die "OpenWrt $release: podkop-exp needs OpenWrt 24.10 or newer"
        fi
        ;;
    esac

    ram_mb=$(awk '/^MemTotal:/ { print int($2 / 1024) }' /proc/meminfo)
    if [ "$ram_mb" -lt "$RAM_MIN_MB" ]; then
        die "The router has $ram_mb MB of memory: podkop-exp needs $RAM_MIN_MB MB or more"
    fi
    msg "Memory: $ram_mb MB"

    if ! nslookup api.github.com >/dev/null 2>&1; then
        die "DNS is not working: api.github.com is not resolved"
    fi

    # Check version
    if command -v podkop > /dev/null 2>&1; then
        local version
        version=$(/usr/bin/podkop show_version 2> /dev/null)
        if [ -n "$version" ]; then
            version=$(echo "$version" | sed 's/^v//')
            local minor
            major=$(echo "$version" | cut -d. -f1)
            minor=$(echo "$version" | cut -d. -f2)

            # Compare version: must be >= 0.7.0. Grouped: "a || b && c" is "(a || b) && c"
            # in sh, and took every 0.8 for older than 0.7. The patch is not compared:
            # "0-dev" is no number.
            if [ "$major" -gt 0 ] 2>/dev/null || { [ "$major" -eq 0 ] 2>/dev/null && [ "$minor" -ge 7 ] 2>/dev/null; }; then
                msg "Podkop version >= 0.7.0"
            else
                msg "Podkop version < 0.7.0"
                update_config
            fi
        else
            msg "Unknown podkop version"
            update_config
        fi
    fi
}

# kmod-nft-tproxy carries the transparent proxy: installed, or one the package manager can
# install for this kernel. A firmware built apart or a snapshot may have no kernel modules
# of its kernel in the feeds.
check_tproxy() {
    local out
    pkg_is_installed kmod-nft-tproxy && return
    if [ "$PKG_IS_APK" -eq 1 ]; then
        out=$(apk add --simulate kmod-nft-tproxy 2>&1)
    else
        out=$(opkg install --noaction kmod-nft-tproxy 2>&1)
    fi || {
        echo "$out" | tail -n 5
        die "kmod-nft-tproxy cannot be installed, and podkop-exp needs it for the transparent proxy. The feeds of the firmware have no such module for its kernel."
    }
    msg "kmod-nft-tproxy can be installed"
}

# the release: the URLs of podkop-exp for this architecture, of the page and of its Russian
# translation (PKG_URL, PAGE_URL, RU_URL)
find_release() {
    local ext archs arch dashed urls
    ext=ipk
    [ "$PKG_IS_APK" -eq 1 ] && ext=apk
    archs=$(pkg_archs | tr '\n' ' ')
    [ -n "$archs" ] || die "Unknown package architecture"

    wget -q -O "$DOWNLOAD_DIR/release.json" "$REPO" || die "The latest release is not available: $REPO"
    if grep -q 'API rate limit' "$DOWNLOAD_DIR/release.json"; then
        die "You've reached the GitHub rate limit. Repeat in an hour."
    fi
    TAG=$(sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "$DOWNLOAD_DIR/release.json" | head -n 1)
    urls=$(grep -o "https://[^\"[:space:]]*\.$ext" "$DOWNLOAD_DIR/release.json")
    # podkop-exp is built for each architecture (the name of a package ends with the one of
    # the router, "_" written as "-"); the packages of LuCI are the same for all
    PKG_URL=""
    for arch in $archs; do
        dashed=$(echo "$arch" | tr '_' '-')
        PKG_URL=$(echo "$urls" | grep "/podkop-exp-[^/]*-$dashed\.$ext\$" | head -n 1)
        [ -n "$PKG_URL" ] && break
    done
    [ -n "$PKG_URL" ] || die "Release $TAG has no podkop-exp for the architecture ${archs% } of this router"
    msg "Package architecture: $arch"
    PAGE_URL=$(echo "$urls" | grep "/luci-app-podkop-exp-[^/]*\.$ext\$" | head -n 1)
    RU_URL=$(echo "$urls" | grep "/luci-i18n-podkop-exp-ru[^/]*\.$ext\$" | head -n 1)
    msg "Release $TAG"
}

download() {
    local url="$1" file attempt=0
    file="$DOWNLOAD_DIR/$(basename "$url")"
    while [ $attempt -lt $COUNT ]; do
        if wget -q -O "$file" "$url" && [ -s "$file" ]; then
            echo "$file"
            return 0
        fi
        rm -f "$file"
        attempt=$((attempt + 1))
    done
    return 1
}

# --- what goes

# what podkop-exp cannot work beside, installed: podkop and its page (podkop-exp has its
# service, settings and files), podkop-engine (the sing-box of podkop 0.7), and what takes
# the DNS of dnsmasq over as soon as it is installed (the CONFLICTS of the package, which apk
# does not read)
incompatible() {
    installed_names | grep -x -E 'podkop|luci-app-podkop|luci-i18n-podkop-ru|podkop-engine|podkop-engine-full|https-dns-proxy|luci-app-https-dns-proxy|luci-i18n-https-dns-proxy-.*|nextdns|luci-app-nextdns|luci-i18n-nextdns-.*|luci-app-xray|luci-i18n-xray-.*'
}

# the installed sing-box packages, by name or by what they provide: podkop-exp has sing-box
# inside and uses none of them
sing_boxes() {
    {
        installed_names | grep -E '^sing-box'
        if [ "$PKG_IS_APK" -eq 1 ]; then
            awk -F: '$1 == "P" { name = $2 } $1 == "p" && $2 ~ /(^| )sing-box([ =<>]|$)/ { print name }' /lib/apk/db/installed 2>/dev/null
        else
            grep -l -E '^Provides:.*(^|[ ,])sing-box([ ,(]|$)' /usr/lib/opkg/info/*.control 2>/dev/null | sed 's|.*/||; s|\.control$||'
        fi
    } | grep -v -x -E 'podkop-engine|podkop-engine-full' | sort -u
}

in_list() {
    case " $2 " in
    *" $1 "*) return 0 ;;
    esac
    return 1
}

# REMOVE: what goes, asked; KEEP_BOXES: the sing-box packages that stay. Exits when the
# answer is no or the flash is too small.
plan() {
    local found pkg need free freed=0 extra=0 boxes="" others new_kb

    found=$(incompatible | tr '\n' ' ')
    found=${found% }
    REMOVE=""
    if [ -n "$found" ]; then
        msg "podkop-exp cannot work beside: $found"
        case " $found " in
        *" podkop "*) msg "The settings of podkop (/etc/config/podkop) are kept for podkop-exp" ;;
        esac
        ask "Remove them?" || die "Nothing is changed"
        REMOVE="$found"
    fi

    new_kb=$(file_installed_kb "$PKG_FILE")
    need=$(($(need_kb "$new_kb") + RESERVE_KB))
    free=$(free_kb)
    # an upgrade replaces the files of the installed podkop-exp
    for pkg in $REMOVE podkop-exp; do
        pkg_is_installed "$pkg" && freed=$((freed + $(pkg_freed_kb "$pkg")))
    done
    msg "Flash ($(flash_fs)): free $(mb "$free"), podkop-exp needs $(mb "$need"), the removal frees $(mb "$freed")"

    KEEP_BOXES=""
    for pkg in $(sing_boxes); do
        others=$(needed_by "$pkg" | grep -v -x -e podkop -e luci-app-podkop | tr '\n' ' ')
        if [ -n "$others" ]; then
            KEEP_BOXES="$KEEP_BOXES $pkg"
            msg "$pkg stays: needed by ${others% }"
        else
            boxes="$boxes $pkg"
            extra=$((extra + $(pkg_freed_kb "$pkg")))
        fi
    done

    if [ $((free + freed)) -ge "$need" ]; then
        KEEP_BOXES="$KEEP_BOXES$boxes"
        return
    fi
    if [ -z "$boxes" ] || [ $((free + freed + extra)) -lt "$need" ]; then
        die "Not enough flash: podkop-exp needs $(mb "$need"), and at most $(mb $((free + freed + extra))) is free even after the removals. Free $(mb $((need - free - freed - extra))) more."
    fi
    msg "Not enough flash without removing$boxes: podkop-exp has sing-box inside and does not use it; the removal frees $(mb "$extra")"
    ask "Remove$boxes?" || die "Nothing is changed"
    REMOVE="$REMOVE$boxes"
}

# --- the changes

APK_CHANGES=""
RU_WAS=""
# podkop 0.7 had the service of its sing-box run its configuration (enabled, the conffile
# at its config_path, only the autostart off): the next upgrade of a sing-box that stays
# would start a second sing-box on it. The service is turned off; nothing else of sing-box
# is changed. With no sing-box left, the configuration (proxy links and all) is deleted.
# Called once the packages are removed: the configuration is a conffile of sing-box, which
# opkg checks when sing-box goes. OLD_CONFIG is taken before, while podkop 0.7 is there.
OLD_CONFIG=""
retire_old_config() {
    local pkg old_config="$OLD_CONFIG"
    [ -n "$old_config" ] || return
    for pkg in $KEEP_BOXES; do
        if [ "$(uci -q get sing-box.main.conffile)" = "$old_config" ]; then
            uci -q set sing-box.main.enabled=0 && uci -q commit sing-box
            /etc/init.d/sing-box stop 2>/dev/null
            msg "$pkg stays; its service, which ran the configuration of podkop 0.7, is turned off"
        fi
        return
    done
    rm -f "$old_config"
}

remove_packages() {
    local pkg
    # the translation was there: its upgrade comes without a question
    case " $REMOVE " in
    *" luci-i18n-podkop-ru "*) RU_WAS=1 ;;
    esac
    pkg_is_installed luci-i18n-podkop-exp-ru && RU_WAS=1
    # podkop goes: its service stops first. The service of an installed podkop-exp is left
    # to the package: an install of the same version changes nothing and starts nothing.
    case " $REMOVE " in
    *" podkop "*) [ -x /etc/init.d/podkop ] && /etc/init.d/podkop stop 2>/dev/null ;;
    esac
    # podkop 0.7: its backend is a shell script
    [ "$(head -c 2 /usr/bin/podkop 2>/dev/null)" = "#!" ] && OLD_CONFIG=$(uci -q get podkop.settings.config_path)
    if [ "$PKG_IS_APK" -eq 1 ]; then
        # apk changes the packages in the transaction that installs podkop-exp: podkop shares
        # its files, and a package podkop 0.7 depends on cannot go before it. A sing-box that
        # stays is asked for there: nothing may need it any more, and apk removes what
        # nothing needs.
        for pkg in $REMOVE; do
            APK_CHANGES="$APK_CHANGES !$pkg"
        done
        for pkg in $KEEP_BOXES; do
            APK_CHANGES="$APK_CHANGES $pkg"
        done
        retire_old_config
        return
    fi
    if [ -n "$REMOVE" ]; then
        msg "Removing: $REMOVE"
        cp /etc/config/podkop "$DOWNLOAD_DIR/podkop.config" 2>/dev/null
        # shellcheck disable=SC2086
        opkg remove --force-depends $REMOVE 2>&1 | quiet
        [ -f /etc/config/podkop ] || cp "$DOWNLOAD_DIR/podkop.config" /etc/config/podkop 2>/dev/null
    fi
    retire_old_config
    # a sing-box that stays may have come as a dependency of podkop 0.7: opkg removes a
    # package it installed so ("Auto-Installed") once nothing needs it. The mark goes, as if
    # the user had asked for the package.
    for pkg in $KEEP_BOXES; do
        sed -i "/^Package: $pkg\$/,/^\$/{/^Auto-Installed: yes\$/d}" /usr/lib/opkg/status
    done
}

update_config() {
    printf "\033[48;5;196m\033[1m╔══════════════════════════════════════════════════════════════════════╗\033[0m\n"
    printf "\033[48;5;196m\033[1m║ ! Обнаружена старая версия podkop.                                   ║\033[0m\n"
    printf "\033[48;5;196m\033[1m║ Если продолжите обновление, вам потребуется настроить Podkop заново. ║\033[0m\n"
    printf "\033[48;5;196m\033[1m║ Старая конфигурация будет сохранена в /etc/config/podkop-070         ║\033[0m\n"
    printf "\033[48;5;196m\033[1m║ Подробности: https://github.com/podkop-net/podkop-experimental       ║\033[0m\n"
    printf "\033[48;5;196m\033[1m║ Точно хотите продолжить?                                             ║\033[0m\n"
    printf "\033[48;5;196m\033[1m╚══════════════════════════════════════════════════════════════════════╝\033[0m\n"

    echo ""

    printf "\033[48;5;196m\033[1m╔══════════════════════════════════════════════════════════════════════╗\033[0m\n"
    printf "\033[48;5;196m\033[1m║ ! Detected old podkop version.                                       ║\033[0m\n"
    printf "\033[48;5;196m\033[1m║ If you continue the update, you will need to RECONFIGURE podkop.     ║\033[0m\n"
    printf "\033[48;5;196m\033[1m║ Your old configuration will be saved to /etc/config/podkop-070       ║\033[0m\n"
    printf "\033[48;5;196m\033[1m║ Details: https://github.com/podkop-net/podkop-experimental           ║\033[0m\n"
    printf "\033[48;5;196m\033[1m║ Are you sure you want to continue?                                   ║\033[0m\n"
    printf "\033[48;5;196m\033[1m╚══════════════════════════════════════════════════════════════════════╝\033[0m\n"

    ask "Continue?" || die "Exit"
    mv /etc/config/podkop /etc/config/podkop-070
    wget -O /etc/config/podkop https://raw.githubusercontent.com/podkop-net/podkop-experimental/refs/heads/main/podkop/files/etc/config/podkop
    msg "Podkop config has been reset to default. Your old config saved in /etc/config/podkop-070"
}

main() {
    local page ru

    check_system

    /usr/sbin/ntpd -q -p 194.190.168.1 -p 216.239.35.0 -p 216.239.35.4 -p 162.159.200.1 -p 162.159.200.123

    pkg_list_update || die "Packages list update failed"
    check_tproxy
    find_release

    msg "Downloading podkop-exp $TAG..."
    PKG_FILE=$(download "$PKG_URL") || die "Failed to download $(basename "$PKG_URL")"
    page=""
    [ -n "$PAGE_URL" ] && page=$(download "$PAGE_URL")
    ru=""
    [ -n "$RU_URL" ] && ru=$(download "$RU_URL")

    plan
    remove_packages

    # opkg takes the unpacked size for what a package needs: on a compressing file system
    # that is about twice what it takes, and the check of plan() counts the real share
    OPKG_SPACE=""
    fs_compresses && OPKG_SPACE=--force-space

    msg "Installing $(basename "$PKG_FILE")..."
    if [ "$PKG_IS_APK" -eq 1 ]; then
        {
            # shellcheck disable=SC2086
            apk add --allow-untrusted "$PKG_FILE" $APK_CHANGES 2>&1
            echo $? > "$DOWNLOAD_DIR/apk.rc"
        } | quiet
        [ "$(cat "$DOWNLOAD_DIR/apk.rc")" = 0 ]
    else
        pkg_install "$PKG_FILE"
    fi || die "The install of $(basename "$PKG_FILE") failed"

    if [ -n "$page" ]; then
        msg "Installing $(basename "$page")..."
        pkg_install "$page" || err "The install of $(basename "$page") failed"
    fi

    if [ -n "$ru" ]; then
        # the translation of podkop's page (luci-i18n-podkop-ru) has the same files: removed
        # with podkop above, or still there with apk
        if [ -n "$RU_WAS" ] || ask "Русский язык интерфейса ставим? (Install the Russian interface language?)"; then
            pkg_is_installed luci-i18n-podkop-ru && pkg_remove luci-i18n-podkop-ru
            pkg_install "$ru"
        fi
    fi

    rm -rf "$DOWNLOAD_DIR"
    msg "podkop-exp $TAG is installed. Clear the cache of the browser before opening its page in LuCI."
}

main
