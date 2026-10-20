# podkop: the settings a sysupgrade keeps hold the DNS of the LAN as podkop's backend took
# it: dnsmasq asks 127.0.0.42 only (/etc/config/dhcp). The archive is made while the
# backend runs. An image without podkop would come up with neither the router nor the
# LAN resolving names, and no way to fetch podkop again. A script goes into the archive
# that gives dnsmasq back at the first boot of the new system when podkop is not there;
# where podkop is, it does nothing and is gone. sysupgrade carries its own
# 10_disable_services the same way.
# sysupgrade sources this file (include /lib/upgrade) and runs the hook with the file that
# lists what it keeps; a backup of the settings (sysupgrade -b, the one of LuCI) too.

podkop_dns_back() {
	local list="$1" script=/etc/uci-defaults/95-podkop-dns
	# a listing of what would be kept (sysupgrade -l) changes nothing
	[ "${CONF_BACKUP_LIST:-0}" = 1 ] && return 0
	case " $(uci -q get dhcp.@dnsmasq[0].server) " in
	*' 127.0.0.42 '*) ;;
	*) return 0 ;;
	esac
	cat > "$script" <<'EOF'
#!/bin/sh
# podkop: put into the settings a sysupgrade kept by /lib/upgrade/podkop.sh. Without podkop
# on this system, dnsmasq gets back what podkop's backend took (what it kept under the
# podkop_* options, the servers added since, the resolvers of the WAN when no server is
# for every name); with podkop the backend keeps it.
[ -x /usr/bin/podkop ] && exit 0
S='dhcp.@dnsmasq[0]'
current=$(uci -q get $S.server)
case " $current " in
*' 127.0.0.42 '*) ;;
*) exit 0 ;;
esac
servers=$(uci -q get $S.podkop_server)
for server in $current; do
	[ "$server" = 127.0.0.42 ] && continue
	case " $servers " in
	*" $server "*) ;;
	*) servers="$servers $server" ;;
	esac
done
uci -q delete $S.server
general=0
for server in $servers; do
	uci add_list $S.server="$server"
	case "$server" in
	/*) ;;
	*) general=1 ;;
	esac
done
noresolv=$(uci -q get $S.podkop_noresolv)
# no server for every name: the resolvers of the WAN are read
[ $general = 0 ] && noresolv=0
uci set $S.noresolv="${noresolv:-0}"
cachesize=$(uci -q get $S.podkop_cachesize)
uci set $S.cachesize="${cachesize:-150}"
for option in podkop_server podkop_noresolv podkop_cachesize podkop_taken; do
	uci -q delete $S.$option
done
uci commit dhcp
exit 0
EOF
	echo "$script" >> "$list"
}

if [ -n "$sysupgrade_init_conffiles" ]; then
	sysupgrade_init_conffiles="$sysupgrade_init_conffiles podkop_dns_back"
fi
