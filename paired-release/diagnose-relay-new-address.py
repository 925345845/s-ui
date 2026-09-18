#!/usr/bin/env python3
"""Temporarily bind one fresh /128; compare it with an existing source for 60s.

Requires Linux root and iproute2. No route/firewall/service changes. The address
successfully added by this script is deleted on exit. Captures only in-memory
Ethernet/IPv6 headers, printing counters, never payloads or proxy credentials.
"""
import argparse
import collections
import concurrent.futures
import ipaddress
import json
import os
import secrets
import signal
import socket
import subprocess
import threading
import time

TARGETS = ("2606:4700:4700::1111", "2001:4860:4860::8888")


def run(args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=5)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result.stdout.strip()


def packet_event(frame, outgoing, sources):
    if len(frame) < 14:
        return None
    offset, protocol = 14, int.from_bytes(frame[12:14], "big")
    while protocol in (0x8100, 0x88a8):
        if len(frame) < offset + 4:
            return None
        protocol = int.from_bytes(frame[offset+2:offset+4], "big")
        offset += 4
    if protocol != 0x86dd or len(frame) < offset + 40:
        return None
    source = str(ipaddress.IPv6Address(frame[offset+8:offset+24]))
    dest = str(ipaddress.IPv6Address(frame[offset+24:offset+40]))
    protocol = frame[offset+6]
    offset += 40
    for _ in range(8):
        if protocol not in (0, 43, 60, 44, 51):
            break
        if len(frame) < offset + 2:
            return None
        size = 8 if protocol == 44 else (frame[offset+1]+2)*4 if protocol == 51 else (frame[offset+1]+1)*8
        if len(frame) < offset + size:
            return None
        if protocol == 44 and int.from_bytes(frame[offset+2:offset+4], "big") & 0xfff8:
            return None
        protocol = frame[offset]
        offset += size
    direction = "OUT" if outgoing else "IN"
    if protocol == 58 and len(frame) >= offset + 24 and frame[offset] in (135, 136):
        target = str(ipaddress.IPv6Address(frame[offset+8:offset+24]))
        if target in sources:
            kind = "NS" if frame[offset] == 135 else "NA"
            return f"{target} {direction}_{kind}"
    if protocol == 6 and len(frame) >= offset + 20:
        sport = int.from_bytes(frame[offset:offset+2], "big")
        dport = int.from_bytes(frame[offset+2:offset+4], "big")
        flags = frame[offset+13]
        if outgoing and source in sources and dest in TARGETS and dport == 443 and flags & 2:
            return f"{source} -> {dest} OUT_SYN"
        if not outgoing and dest in sources and source in TARGETS and sport == 443:
            if flags & 4:
                return f"{dest} <- {source} IN_RST"
            if flags & 0x12 == 0x12:
                return f"{dest} <- {source} IN_SYN_ACK"
    return None


def probe(source, target):
    started = time.monotonic()
    try:
        with socket.socket(socket.AF_INET6, socket.SOCK_STREAM) as connection:
            connection.settimeout(5)
            connection.bind((source, 0))
            connection.connect((target, 443))
        result = "TCP_OK"
    except OSError as error:
        result = f"TCP_FAIL {error}"
    return f"{source} -> {target} {result} {time.monotonic()-started:.2f}s"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--interface", default="eth0")
    parser.add_argument("--prefix", required=True)
    parser.add_argument("--baseline", required=True)
    args = parser.parse_args()
    network = ipaddress.IPv6Network(args.prefix, strict=False)
    baseline = str(ipaddress.IPv6Address(args.baseline))
    if network.prefixlen > 120:
        parser.error("use the provider allocation prefix, for example /64")
    devices = json.loads(run(["ip", "-j", "-6", "addr", "show", "dev", args.interface]))
    existing = {str(ipaddress.IPv6Address(a["local"])) for d in devices for a in d.get("addr_info", [])}
    if baseline not in existing:
        parser.error("baseline must already be assigned to the specified interface")
    new_ip = None
    for _ in range(100):
        candidate = str(ipaddress.IPv6Address(int(network.network_address) | secrets.randbits(128-network.prefixlen)))
        if candidate not in existing and candidate != str(network.network_address):
            new_ip = candidate
            break
    if new_ip is None:
        raise RuntimeError("could not select an unused address")
    if os.geteuid() != 0:
        parser.error("run as root")
    stop = threading.Event()
    counts = collections.Counter()
    lock = threading.Lock()
    capture = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(3))
    capture.bind((args.interface, 0))
    capture.settimeout(0.5)

    def collect():
        while not stop.is_set():
            try:
                frame, meta = capture.recvfrom(256)
            except socket.timeout:
                continue
            except OSError:
                return
            event = packet_event(frame, meta[2] == socket.PACKET_OUTGOING, {baseline, new_ip})
            if event:
                with lock:
                    counts[event] += 1

    thread = threading.Thread(target=collect, daemon=True)
    thread.start()
    added = False
    try:
        run(["ip", "-6", "addr", "add", new_ip+"/128", "dev", args.interface])
        added = True
        start = time.monotonic()
        print(f"BASELINE={baseline}\nNEW={new_ip}/128\nObserve 60s; do NOT create a panel batch during this test.", flush=True)
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as workers:
            for age in (3, 20, 40, 60):
                time.sleep(max(0, start + age - time.monotonic()))
                print(f"\n=== AGE {time.monotonic()-start:.1f}s ===", flush=True)
                print(run(["ip", "-6", "-o", "addr", "show", "dev", args.interface, "to", new_ip+"/128"]), flush=True)
                for target in TARGETS:
                    results = [workers.submit(probe, source, target) for source in (baseline, new_ip)]
                    for result in results:
                        print(result.result(), flush=True)
                with lock:
                    snapshot = sorted(counts.items())
                print("Cumulative header counters:", flush=True)
                for key, value in snapshot:
                    print(f"  {key}={value}", flush=True)
    finally:
        stop.set()
        thread.join(timeout=2)
        capture.close()
        if added:
            run(["ip", "-6", "addr", "del", new_ip+"/128", "dev", args.interface])
            print(f"CLEANED {new_ip}; existing addresses unchanged", flush=True)


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, lambda *_: (_ for _ in ()).throw(KeyboardInterrupt()))
    try:
        main()
    except KeyboardInterrupt:
        print("Stopped", flush=True)
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"ERROR: {error}", flush=True)
        raise SystemExit(1)
