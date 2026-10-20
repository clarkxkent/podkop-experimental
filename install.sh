#!/bin/sh
# shellcheck shell=dash

# podkop-exp: the Go backend of podkop, published apart from itdoginfo/podkop
REPO="https://api.github.com/repos/podkop-net/podkop-experimental/releases/latest"
DOWNLOAD_DIR="/tmp/podkop"
COUNT=3

# Cached flag to switch between ipk or apk package managers
PKG_IS_APK=0
command -v apk >/dev/null 2>&1 && PKG_IS_APK=1

rm -rf "$DOWNLOAD_DIR"
mkdir -p "$DOWNLOAD_DIR"

msg() {
    printf "\033[32;1m%s\033[0m\n" "$1"
}

pkg_is_installed () {
    local pkg_name="$1"

    if [ "$PKG_IS_APK" -eq 1 ]; then
        # grep -q should work without change based on example from documentation
        # apk list --installed --providers dnsmasq
        # <dnsmasq> dnsmasq-full-2.90-r3 x86_64 {feeds/base/package/network/services/dnsmasq} (GPL-2.0) [installed]
        apk list --installed | grep -q "$pkg_name"
    else
        opkg list-installed | grep -q "$pkg_name"
    fi
}

pkg_remove() {
    local pkg_name="$1"

    if [ "$PKG_IS_APK" -eq 1 ]; then
        # TODO: check --force-depends flag
        # Nothing here: https://openwrt.org/docs/guide-user/additional-software/opkg-to-apk-cheatsheet
        apk del "$pkg_name"
    else
        opkg remove --force-depends "$pkg_name"
    fi
}

pkg_list_update() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        apk update
    else
        opkg update
    fi
}

pkg_install() {
    local pkg_file="$1"

    if [ "$PKG_IS_APK" -eq 1 ]; then
        # Can't install without flag based on info from documentation
        # If you're installing a non-standard (self-built) package, use the --allow-untrusted option:
        apk add --allow-untrusted "$pkg_file"
    else
        opkg install "$pkg_file"
    fi
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

    msg "Continue? (yes/no)"

    while true; do
            read -r -p '' CONFIG_UPDATE
            case $CONFIG_UPDATE in

            yes|y|Y)
                mv /etc/config/podkop /etc/config/podkop-070
                wget -O /etc/config/podkop https://raw.githubusercontent.com/podkop-net/podkop-experimental/refs/heads/main/podkop/files/etc/config/podkop
                msg "Podkop config has been reset to default. Your old config saved in /etc/config/podkop-070"
                break
                ;;
            *)
                msg "Exit"
                exit 1
                ;;
        esac
    done
}

main() {
    check_system

    /usr/sbin/ntpd -q -p 194.190.168.1 -p 216.239.35.0 -p 216.239.35.4 -p 162.159.200.1 -p 162.159.200.123

    pkg_list_update || { echo "Packages list update failed"; exit 1; }

    if pkg_is_installed podkop-exp; then
        msg "podkop-exp is already installed. Upgrading..."
    elif [ -f "/etc/init.d/podkop" ]; then
        msg "podkop is installed: it is replaced by podkop-exp, its settings stay"
    else
        msg "Installing podkop-exp..."
    fi

    if command -v curl >/dev/null 2>&1; then
        check_response=$(curl -s "$REPO")

        if echo "$check_response" | grep -q 'API rate limit '; then
            msg "You've reached the GitHub rate limit. Repeat in five minutes."
            exit 1
        fi
    fi

    local grep_url_pattern pkg_ext arch
    if [ "$PKG_IS_APK" -eq 1 ]; then
        pkg_ext=apk
    else
        pkg_ext=ipk
    fi
    grep_url_pattern="https://[^\"[:space:]]*\.$pkg_ext"
    # podkop-exp is built for each architecture (the name of a package ends with the one of
    # the router, "_" written as "-"); the packages of LuCI are the same for all
    arch=$(pkg_arch | tr '_' '-')
    if [ -z "$arch" ]; then
        msg "Unknown package architecture"
        exit 1
    fi
    msg "Package architecture: $arch"

    wget -qO- "$REPO" | grep -o "$grep_url_pattern" | while read -r url; do
        filename=$(basename "$url")
        case "$filename" in
            podkop-exp-*-"$arch"."$pkg_ext" | luci-app-podkop-exp-* | luci-i18n-podkop-exp-*) ;;
            *) continue ;;
        esac
        filepath="$DOWNLOAD_DIR/$filename"

        attempt=0
        while [ $attempt -lt $COUNT ]; do
            msg "Download $filename (count $((attempt+1)))..."
            if wget -q -O "$filepath" "$url"; then
                if [ -s "$filepath" ]; then
                    msg "$filename successfully downloaded"
                    break
                fi
            fi
            msg "Download error for $filename. Retrying..."
            rm -f "$filepath"
            attempt=$((attempt+1))
        done

        if [ $attempt -eq $COUNT ]; then
            msg "Failed to download $filename after $COUNT attempts"
        fi
    done

    # Check if any files were downloaded
    if ! ls "$DOWNLOAD_DIR"/*podkop* >/dev/null 2>&1; then
        msg "No packages were downloaded successfully"
        exit 1
    fi
    if ! ls "$DOWNLOAD_DIR"/podkop-exp-*-"$arch"."$pkg_ext" >/dev/null 2>&1; then
        msg "No podkop-exp package for the architecture $arch in the release"
        exit 1
    fi

    retire_sing_box
    retire_podkop

    for pkg in podkop-exp luci-app-podkop-exp; do
        file=""
        # the version follows the name: podkop-exp-v0.8.0-r1-..., luci-app-podkop-exp-v0.8.0-r1-all...
        for f in "$DOWNLOAD_DIR"/"$pkg"-[v0-9]*; do
            if [ -f "$f" ]; then
                file=$(basename "$f")
                break
            fi
        done
        if [ -n "$file" ]; then
            msg "Installing $file..."
            if [ "$pkg" = "podkop-exp" ] && [ "$PKG_IS_APK" -eq 1 ]; then
                # shellcheck disable=SC2086
                apk add --allow-untrusted "$DOWNLOAD_DIR/$file" $APK_CHANGES
            else
                pkg_install "$DOWNLOAD_DIR/$file"
            fi || {
                # the page would come without its backend
                msg "The install of $file failed"
                [ "$pkg" = "podkop-exp" ] && exit 1
            }
            sleep 3
        fi
    done

    ru=""
    for f in "$DOWNLOAD_DIR"/luci-i18n-podkop-exp-ru*; do
        if [ -f "$f" ]; then
            ru=$(basename "$f")
            break
        fi
    done
    if [ -n "$ru" ]; then
        # the translation of podkop's page (luci-i18n-podkop-ru) has the same files: removed
        # with podkop above, or still there with apk
        if [ -n "$RU_WAS" ] || pkg_is_installed luci-i18n-podkop; then
                msg "Upgrading Russian translation..."
                pkg_remove luci-i18n-podkop*
                pkg_install "$DOWNLOAD_DIR/$ru"
        else
            msg "Русский язык интерфейса ставим? y/n (Install the Russian interface language?)"
            while true; do
                read -r -p '' RUS
                case $RUS in
                y)
                    pkg_remove luci-i18n-podkop*
                    pkg_install "$DOWNLOAD_DIR/$ru"
                    break
                    ;;
                n)
                    break
                    ;;
                *)
                    echo "Введите y или n"
                    ;;
                esac
            done
        fi
    fi

    find "$DOWNLOAD_DIR" -type f -name '*podkop*' -exec rm {} \;
}

check_system() {
    # Get router model
    MODEL=$(cat /tmp/sysinfo/model)
    msg "Router model: $MODEL"

    # Check OpenWrt version
    openwrt_version=$(cat /etc/openwrt_release | grep DISTRIB_RELEASE | cut -d"'" -f2 | cut -d'.' -f1)
    if [ "$openwrt_version" = "23" ]; then
        msg "OpenWrt 23.05 не поддерживается начиная с podkop 0.5.0"
        msg "Для OpenWrt 23.05 используйте podkop версии 0.4.11 или устанавливайте зависимости и podkop вручную"
        msg "Подробности: https://podkop.net/docs/install/#%d1%83%d1%81%d1%82%d0%b0%d0%bd%d0%be%d0%b2%d0%ba%d0%b0-%d0%bd%d0%b0-2305"
        exit 1
    fi

    # Check available space
    AVAILABLE_SPACE=$(df /overlay | awk 'NR==2 {print $4}')
    REQUIRED_SPACE=15360 # 15MB in KB

    if [ "$AVAILABLE_SPACE" -lt "$REQUIRED_SPACE" ]; then
        msg "Error: Insufficient space in flash"
        msg "Available: $((AVAILABLE_SPACE/1024))MB"
        msg "Required: $((REQUIRED_SPACE/1024))MB"
        exit 1
    fi

    if ! nslookup google.com >/dev/null 2>&1; then
        msg "DNS is not working."
        exit 1
    fi

    # Check version
    if command -v podkop > /dev/null 2>&1; then
        local version
        version=$(/usr/bin/podkop show_version 2> /dev/null)
        if [ -n "$version" ]; then
            version=$(echo "$version" | sed 's/^v//')
            local major
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

    # what takes the forwarding of dnsmasq over as soon as it is installed: the CONFLICTS of
    # the package, which apk does not read
    local conflict answer
    for conflict in https-dns-proxy nextdns luci-app-xray; do
        pkg_is_installed "$conflict" || continue
        msg "Conflicting package detected: $conflict. Remove?"
        read -r -p '' answer || answer=""
        case $answer in
        yes|y|Y)
            case $conflict in
            luci-app-xray)
                pkg_remove luci-app-xray
                pkg_remove luci-i18n-xray*
                ;;
            *)
                pkg_remove "luci-app-$conflict"
                pkg_remove "$conflict"
                pkg_remove "luci-i18n-$conflict*"
                ;;
            esac
            ;;
        *)
            msg "Exit"
            exit 1
            ;;
        esac
    done
}

pkg_arch() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        apk --print-arch
    else
        # the last architecture of opkg is the one of the router (all and noarch go first)
        opkg print-architecture | awk '$2 != "all" && $2 != "noarch" { arch = $2 } END { print arch }'
    fi
}

# podkop 0.7 ran the sing-box of a package of its own: sing-box, sing-box-tiny or
# podkop-engine(-full). podkop-exp has sing-box inside and uses none of them, nor their
# settings or files. podkop-engine serves podkop 0.7 alone and goes with it. A sing-box
# package is asked about only when it is podkop 0.7's: podkop 0.7 is there (its backend is
# a shell script) and no other package needs it (homeproxy, passwall); one another package
# needs stays without a question. Called once the packages are downloaded: while podkop
# 0.7 runs, the router resolves names through sing-box.
APK_CHANGES=""
RU_WAS=""
retire_sing_box() {
    local pkg engines="" boxes="" kept="" remove answer old_config others shell_podkop=""
    [ "$(head -c 2 /usr/bin/podkop 2>/dev/null)" = "#!" ] && shell_podkop=1
    for pkg in podkop-engine podkop-engine-full sing-box sing-box-tiny; do
        if [ "$PKG_IS_APK" -eq 1 ]; then
            apk list --installed 2>/dev/null | grep -q "^$pkg-[0-9]" || continue
        else
            opkg list-installed | grep -q "^$pkg - " || continue
        fi
        case $pkg in
        podkop-engine*) engines="$engines $pkg" ;;
        *)
            [ -n "$shell_podkop" ] || continue
            others=$(needed_by "$pkg" | tr '\n' ' ')
            if [ -n "$others" ]; then
                msg "$pkg stays: ${others}need it"
                kept="$kept $pkg"
            else
                boxes="$boxes $pkg"
            fi
            ;;
        esac
    done
    [ -z "$engines$boxes$kept" ] && return

    # where podkop 0.7 wrote the configuration of that sing-box, proxy links and all
    old_config=$(uci -q get podkop.settings.config_path)
    remove="$engines"
    if [ -n "$boxes" ]; then
        msg "podkop-exp has sing-box inside and does not use the package$boxes. Remove it? (Y/n)"
        read -r -p '' answer || answer=""
        case $answer in
        n|N|no|No) kept="$kept$boxes" ;;
        *) remove="$remove$boxes" ;;
        esac
    fi
    [ -x /etc/init.d/podkop ] && service podkop stop
    if [ -n "$kept" ]; then
        # podkop 0.7 had the service of that sing-box run its configuration (enabled, the
        # conffile at its config_path, only the autostart off): the next upgrade of the
        # package would start a second sing-box on it. The service is turned off; nothing
        # else of sing-box is changed.
        if [ -n "$old_config" ] && [ "$(uci -q get sing-box.main.conffile)" = "$old_config" ]; then
            uci -q set sing-box.main.enabled=0 && uci -q commit sing-box
            /etc/init.d/sing-box stop 2>/dev/null
            msg "sing-box stays; its service, which ran the configuration of podkop 0.7, is turned off"
        fi
    elif [ -n "$old_config" ]; then
        rm -f "$old_config"
    fi
    [ -n "$remove" ] && msg "Removing:$remove"
    if [ "$PKG_IS_APK" -eq 0 ]; then
        # shellcheck disable=SC2086
        [ -n "$remove" ] && opkg remove --force-depends $remove
        # a sing-box that stays was installed for podkop 0.7: opkg removes a package it
        # installed as a dependency ("Auto-Installed") once the upgrade of podkop leaves
        # nothing that needs it. The mark goes, as if the user had asked for the package.
        for pkg in $kept; do
            sed -i "/^Package: $pkg\$/,/^\$/{/^Auto-Installed: yes\$/d}" /usr/lib/opkg/status
        done
        return
    fi
    # apk changes the packages in the transaction that installs podkop-exp: a package
    # podkop 0.7 depends on cannot be removed before. A sing-box that stays is asked for
    # there: nothing needs it any more, and apk removes what nothing needs.
    for pkg in $remove; do
        APK_CHANGES="$APK_CHANGES !$pkg"
    done
    for pkg in $kept; do
        APK_CHANGES="$APK_CHANGES $pkg"
    done
}

# the packages besides podkop's own that need a package, a name a line
needed_by() {
    if [ "$PKG_IS_APK" -eq 1 ]; then
        # "<pkg>-<version> is required by:" and then the packages, with their versions
        apk info -r "$1" 2>/dev/null | sed '1d' | grep -v -e '^$' -e '^podkop-[v0-9]' -e '^luci-app-podkop-[v0-9]'
    else
        # "<pkg> <version> depends on <name>"
        opkg whatdepends "$1" 2>/dev/null | awk '/ depends on / {print $1}' | grep -v -x -e podkop -e luci-app-podkop
    fi
}

# podkop (itdoginfo/podkop: 0.7 with its shell backend, or the Go backend under that name)
# is replaced by podkop-exp: the same service, settings and files, and its page by the
# page of podkop-exp. The settings stay: /etc/config/podkop changed is kept by opkg and
# apk when the package goes, and one opkg took is put back before podkop-exp comes.
retire_podkop() {
    local pkg old=""
    for pkg in podkop luci-app-podkop luci-i18n-podkop-ru; do
        if [ "$PKG_IS_APK" -eq 1 ]; then
            apk list --installed 2>/dev/null | grep -q "^$pkg-[v0-9]" || continue
        else
            opkg list-installed | grep -q "^$pkg - " || continue
        fi
        old="$old $pkg"
    done
    [ -z "$old" ] && return
    msg "Replacing with podkop-exp:$old"
    # the translation of podkop's page goes now: the one of podkop-exp comes without asking
    case " $old " in
    *" luci-i18n-podkop-ru "*) RU_WAS=1 ;;
    esac
    [ -x /etc/init.d/podkop ] && service podkop stop
    if [ "$PKG_IS_APK" -eq 0 ]; then
        cp /etc/config/podkop "$DOWNLOAD_DIR/podkop.config" 2>/dev/null
        # shellcheck disable=SC2086
        opkg remove --force-depends $old
        [ -f /etc/config/podkop ] || cp "$DOWNLOAD_DIR/podkop.config" /etc/config/podkop 2>/dev/null
        return
    fi
    # apk: in the transaction that installs podkop-exp, whose files they share
    for pkg in $old; do
        APK_CHANGES="$APK_CHANGES !$pkg"
    done
}

main