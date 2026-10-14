#!/bin/sh
# Match Nginx's runtime resolver to the DNS server provided by Docker or K3s.
set -eu
resolver="$(awk '$1 == "nameserver" { print $2; exit }' /etc/resolv.conf)"
test -n "$resolver"
sed -i "s/127\\.0\\.0\\.11/${resolver}/g" /etc/nginx/conf.d/default.conf
