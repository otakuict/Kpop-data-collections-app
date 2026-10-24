#!/bin/sh
# Match Nginx's runtime resolver to the DNS server provided by Docker or K3s.
set -eu
resolv_conf="${1:-/etc/resolv.conf}"
nginx_conf="${2:-/etc/nginx/conf.d/default.conf}"
resolver="$(awk '$1 == "nameserver" { print $2; exit }' "$resolv_conf")"
test -n "$resolver"
sed -i.bak "s/127\\.0\\.0\\.11/${resolver}/g" "$nginx_conf"
rm -f "$nginx_conf.bak"

# Nginx's resolver does not use the search domains in resolv.conf. Use the
# namespace-qualified Service name when running inside Kubernetes.
namespace_domain="$(awk '$1 == "search" { for (i = 2; i <= NF; i++) if ($i ~ /^[^.]+[.]svc[.]/) { print $i; exit } }' "$resolv_conf")"
if [ -n "$namespace_domain" ]; then
  sed -i.bak "s|http://backend:8080|http://backend.${namespace_domain}:8080|g" "$nginx_conf"
  rm -f "$nginx_conf.bak"
fi
