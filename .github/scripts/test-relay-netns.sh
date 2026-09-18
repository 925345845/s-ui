#!/usr/bin/env bash
set -euo pipefail

test_binary="$(mktemp /tmp/relay-netns-test.XXXXXX)"
test_namespace="relay-ci-$$"
namespace_created=false
cleanup() {
  if "$namespace_created"; then sudo ip netns del "$test_namespace"; fi
  rm -f -- "$test_binary"
}
trap cleanup EXIT
go test -c -race -ldflags=-checklinkname=0 -o "$test_binary" ./service
sudo ip netns add "$test_namespace"
namespace_created=true
sudo ip -n "$test_namespace" link add relaytest0 type dummy
sudo ip -n "$test_namespace" link set relaytest0 up
sudo ip -n "$test_namespace" -6 addr add 2001:db8:abcd:1234::1/64 dev relaytest0 nodad
sudo ip netns exec "$test_namespace" env SUI_RELAY_NETNS_TEST=1 \
  "$test_binary" -test.v -test.run '^TestRelayLinux(AddressLifecycle|PartialCreation)$' -test.timeout=60s
