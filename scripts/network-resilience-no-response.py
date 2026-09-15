#!/usr/bin/env python3
"""Accept DNS TCP/UDP traffic without returning a DNS response."""

from __future__ import annotations

import argparse
import selectors
import signal
import socket


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--listen", required=True)
    parser.add_argument("--port", type=int, default=53)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    selector = selectors.DefaultSelector()
    running = True

    def stop(_signum: int, _frame: object) -> None:
        nonlocal running
        running = False

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)

    tcp = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    tcp.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    tcp.bind((args.listen, args.port))
    tcp.listen()
    tcp.setblocking(False)
    selector.register(tcp, selectors.EVENT_READ, "accept")

    udp = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    udp.bind((args.listen, args.port))
    udp.setblocking(False)
    selector.register(udp, selectors.EVENT_READ, "udp")

    clients: set[socket.socket] = set()
    print(f"accepting DNS without responses on {args.listen}:{args.port}", flush=True)
    try:
        while running:
            for key, _events in selector.select(timeout=1):
                if key.data == "accept":
                    client, peer = tcp.accept()
                    client.setblocking(False)
                    clients.add(client)
                    selector.register(client, selectors.EVENT_READ, "client")
                    print(f"accepted tcp peer={peer[0]}:{peer[1]}", flush=True)
                elif key.data == "udp":
                    payload, peer = udp.recvfrom(65535)
                    print(
                        f"discarded udp bytes={len(payload)} peer={peer[0]}:{peer[1]}",
                        flush=True,
                    )
                else:
                    client = key.fileobj
                    assert isinstance(client, socket.socket)
                    payload = client.recv(65535)
                    if payload:
                        peer = client.getpeername()
                        print(
                            f"held tcp bytes={len(payload)} peer={peer[0]}:{peer[1]}",
                            flush=True,
                        )
                    else:
                        selector.unregister(client)
                        clients.discard(client)
                        client.close()
    finally:
        for client in clients:
            selector.unregister(client)
            client.close()
        selector.unregister(tcp)
        selector.unregister(udp)
        tcp.close()
        udp.close()
        selector.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
