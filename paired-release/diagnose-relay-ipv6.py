#!/usr/bin/env python3
"""Observe relay creation without changing addresses, routes or service settings.

Run on the VPS, then click Create once. Only TCP handshakes are sent. No proxy
credentials, application payloads, environment variables or database are read.
"""
import argparse
import concurrent.futures
import datetime
import json
import os
import shutil
import socket
import subprocess
import threading
import time

TARGETS = ("2606:4700:4700::1111", "2001:4860:4860::8888")
OUTPUT_LOCK = threading.Lock()


def report(message):
    with OUTPUT_LOCK:
        stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%H:%M:%SZ")
        print(f"[{stamp}] {message}", flush=True)


def command(args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=5)
    if result.returncode:
        raise RuntimeError(f"{args[0]}: {result.stderr.strip()}")
    return result.stdout.strip()


def addresses(interface):
    result = {}
    for device in json.loads(command(["ip", "-j", "-6", "addr", "show", "dev", interface])):
        for info in device.get("addr_info", []):
            if info.get("scope") == "global":
                result[info["local"]] = info
    return result


def tcp_probe(source, target, port=443):
    started = time.monotonic()
    try:
        with socket.socket(socket.AF_INET6, socket.SOCK_STREAM) as sock:
            sock.settimeout(5)
            sock.bind((source, 0))
            sock.connect((target, port))
        outcome = "OK"
    except OSError as error:
        outcome = f"FAIL {type(error).__name__}: {error}"
    return f"{source} -> [{target}]:{port} {outcome} ({time.monotonic()-started:.2f}s)"


def inspect_service():
    try:
        properties = command([
            "systemctl", "show", "s-ui", "-p", "MainPID", "-p", "PrivateNetwork",
            "-p", "NetworkNamespacePath", "-p", "RestrictAddressFamilies",
            "-p", "IPAddressAllow", "-p", "IPAddressDeny",
        ])
        report("SERVICE " + properties.replace("\n", " "))
        values = dict(line.split("=", 1) for line in properties.splitlines() if "=" in line)
        pid = int(values.get("MainPID", "0"))
        if pid:
            report(f"NETNS shell={os.readlink('/proc/self/ns/net')} panel={os.readlink(f'/proc/{pid}/ns/net')}")
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        report(f"SERVICE inspection unavailable: {error}")


def packet_headers(interface, stop):
    if not shutil.which("tcpdump"):
        report("PACKETS tcpdump not installed; continuing socket comparison")
        return
    # No -A/-X/-w: print headers only, bounded to 120 lines and target SYN/RST
    # packets plus neighbour discovery. Never print TCP application payloads.
    packet_filter = (
        "ip6 and ((icmp6 and (ip6[40] == 135 or ip6[40] == 136)) or "
        "(tcp port 443 and (host 2606:4700:4700::1111 or host 2001:4860:4860::8888) "
        "and (tcp[13] & 6 != 0)))"
    )
    # tcp[] access is IPv4-only on some libpcap versions: fixed IPv6 TCP
    # header offset is appropriate for the plain SYN packets sent here.
    packet_filter = packet_filter.replace("tcp[13]", "ip6[53]")
    try:
        process = subprocess.Popen(
            ["tcpdump", "-l", "-nn", "-t", "-i", interface, "-c", "120", packet_filter],
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
        )
    except OSError as error:
        report(f"PACKETS unavailable: {error}")
        return

    def reader():
        for line in process.stdout:
            report("PACKET " + line.rstrip())

    reader_thread = threading.Thread(target=reader, daemon=True)
    reader_thread.start()
    stop.wait()
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
    reader_thread.join(timeout=2)
    process.stdout.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--interface", default="eth0")
    parser.add_argument("--seconds", type=int, default=60)
    args = parser.parse_args()
    if not 10 <= args.seconds <= 120:
        parser.error("--seconds must be between 10 and 120")
    initial = addresses(args.interface)
    report("BEGIN read-only IPv6 creation diagnostic")
    inspect_service()
    report("ROUTES " + command(["ip", "-6", "route", "show"]).replace("\n", " | "))
    for source, info in initial.items():
        report(f"EXISTING {source}/{info['prefixlen']}")
    stop_capture = threading.Event()
    capture = threading.Thread(target=packet_headers, args=(args.interface, stop_capture), daemon=True)
    capture.start()
    pending = {}
    sampled = {}
    known = set(initial)
    observed_new = set()
    deadline = time.monotonic() + args.seconds
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            for source in list(initial)[:2]:
                pending[pool.submit(tcp_probe, source, TARGETS[0])] = source
            report("READY: now click Create ONCE in the panel; keep this terminal open.")
            while time.monotonic() < deadline:
                current = addresses(args.interface)
                now = time.monotonic()
                for source in current.keys() - known:
                    info = current[source]
                    observed_new.add(source)
                    report(f"ADDED {source}/{info['prefixlen']} tentative={info.get('tentative', False)} dadfailed={info.get('dadfailed', False)}")
                    if len(sampled) < 3:
                        sampled[source] = {"first": now, "last": 0, "attempts": 0}
                for source in known - current.keys():
                    report(f"REMOVED {source}")
                known = set(current)
                for future in list(pending):
                    if future.done():
                        report("PYTHON " + future.result())
                        del pending[future]
                for source, state in sampled.items():
                    if (source not in current or source in pending.values()
                            or now-state["last"] < 6 or state["attempts"] >= 3 or len(pending) >= 2):
                        continue
                    if current[source].get("tentative") or current[source].get("dadfailed"):
                        continue
                    target = TARGETS[state["attempts"] % 2]
                    state["attempts"] += 1
                    state["last"] = now
                    report(f"PROBE {source} age={now-state['first']:.1f}s attempt={state['attempts']}")
                    pending[pool.submit(tcp_probe, source, target)] = source
                time.sleep(0.5)
            for future in pending:
                report("PYTHON " + future.result())
    finally:
        stop_capture.set()
        capture.join(timeout=6)
    report(f"END observed_new={len(observed_new)} sampled={len(sampled)}; no addresses/routes/settings changed by this script")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        report(f"ERROR {error}")
        raise SystemExit(1)
