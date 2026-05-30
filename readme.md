<!-- ───────────── project header ───────────── -->

<p align="center">
  <!-- logo placeholder -->
  <img src="https://raw.githubusercontent.com/GoSeoTaxi/cli-ssh2proxy/refs/heads/main/actions/workflows/ci.yml/logo.png" height="110" alt="ssh2proxy logo">
</p>

<p align="center">
<!--   <a href="https://github.com/GoSeoTaxi/cli-ssh2proxy/actions"><img alt="CI" src="https://github.com/GoSeoTaxi/cli-ssh2proxy/actions/workflows/ci.yml/badge.svg"></a>-->
  <a href="https://goreportcard.com/report/github.com/GoSeoTaxi/cli-ssh2proxy"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/GoSeoTaxi/cli-ssh2proxy"></a>
  <a href="https://pkg.go.dev/github.com/GoSeoTaxi/cli-ssh2proxy"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/GoSeoTaxi/cli-ssh2proxy.svg"></a>
  <a href="https://github.com/GoSeoTaxi/cli-ssh2proxy/releases"><img alt="GitHub release" src="https://img.shields.io/github/v/release/GoSeoTaxi/cli-ssh2proxy?logo=github"></a>
  <a href="https://github.com/GoSeoTaxi/cli-ssh2proxy/blob/main/LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

<h3 align="center">
  SSH-powered, self-healing SOCKS5/HTTP proxy with zero-friction metrics — packaged as a single statically-linked binary.
</h3>

## What is **ssh2proxy**? <!-- 2 -->

`ssh2proxy` is a tiny, batteries-included command-line tool that lets you:

* **Expose a local SOCKS5 and/or HTTP CONNECT proxy** that forwards all traffic through a secure SSH tunnel.
* **Reconnect automatically** whenever the upstream SSH server drops, with exponential back-off and keep-alive pings.
* **Ship structured JSON logs** (Zap) instead of plain `printf`, ready for ingest into Loki, Splunk, ELK, or your favorite stack.
* **See live runtime telemetry**—bandwidth, goroutine count, open connections, memory and CPU usage—without Prometheus or sidecars.
* **Cross-compile with a single `make`** into fully static binaries for Linux, macOS (Intel & Apple Silicon) and Windows.

> **Heads-up:** a built-in TUN mode (full-tunnel via `tun2socks`) is planned but not polished yet—currently marked as experimental and disabled by default.
>

## ✨ Key Features

- ✅ **SOCKS5 & HTTP CONNECT gateways** – instant drop-in proxy endpoints for browsers, CLI tools, and mobile apps.
- ✅ **DNS-over-SSH tunnel** – every lookup is resolved through the same encrypted channel, eliminating ISP or hotspot leaks.
- ✅ **Self-healing SSH transport** – automatic keep-alive + exponential-backoff reconnect; you rarely have to restart the binary.
- ✅ **Structured JSON logs** – powered by Uber’s *zap* for painless ingestion in Loki, ELK, or any observability stack.
- ✅ **Built-in runtime metrics** – periodic emission of traffic speed, open connections, goroutine count, memory & CPU usage.
- ✅ **Static cross-platform releases** – single-file binaries for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64.exe`.
- ✅ **Zero external runtime deps** – no Docker, no Python, no obscure shared libraries. You need only a working SSH server.
- ✅ **Graceful shutdown** – `Ctrl-C` or SIGTERM drains listeners, closes tunnels, and tears down child processes in ~2 seconds.
- ✅ **Embeddable *tun2socks*** – pre-compiled helpers shipped as Go `embed` assets (🔬 *full-tunnel mode is experimental*).
- ✅ **Configuration via `.env`, flags, or CI secrets** – flexible for both local hacking and production containers.

<!-- ───────────── 4. Status of TUN / full-tunnel mode ───────────── -->

## ⚠️ TUN (full-tunnel) support — **beta**

`ssh2proxy` can **optionally** spin up an embedded copy of
[`tun2socks`](https://github.com/xjasonlyu/tun2socks) to push **all** system
traffic (not only TCP) through the SSH channel.  
This is handy for CLI tools, Docker containers, or Windows apps that do not
understand SOCKS/HTTP proxies.

| Platform | Status | Notes |
|----------|--------|-------|
| **Linux**   | ✅ _Works_ | Requires CAP\_NET\_ADMIN or root. IPv6 is not tunneled yet. |
| **macOS**   | 🟡 _Experimental_ | Uses `utun` devices. Packet filter rules are **not** auto-configured. |
| **Windows** | 🟡 _Experimental_ | Ships with embedded `wintun.dll`. Needs admin rights the first time to install driver. |
| **FreeBSD / others** | ❌ _Unsupported_ | Pull requests welcome! |

### Known limitations

* MTU is statically set to **1500** — jumbo frames will be fragmented.
* DNS leak protection is rudimentary; prefer the built-in SOCKS/HTTP modes if
  you need bullet-proof privacy.
* Error handling is basic: if `tun2socks` crashes the proxy just logs and
  exits. Automatic restart will be addressed in a future release.
* Mobile OSes (Android/iOS) are **out of scope** for now.

### Call for testers 🧑‍🔬

We need help to make full-tunnel mode rock solid:

* Try it on unusual kernels or custom VPN setups and open issues for anything
  flaky.
* Benchmark throughput on ARM SBCs (Raspberry Pi, Rock Pi, etc.) and post your
  numbers.
* Review the YAML template in [`internal/tun/run_external.go`](./internal/tun/run_external.go)
  — suggestions for smarter routing rules are highly appreciated.

If you bump into problems, please attach logs with `DEBUG=true`.  



| Feature / Sub-system          | Status | Notes / Roadmap                                                |
|-------------------------------|:------:|----------------------------------------------------------------|
| **SOCKS5 proxy**              | ✅      | Fully functional, listens on `--socks` address.                |
| **HTTP CONNECT proxy**        | ✅      | Works on `--http` address, supports HTTPS tunnels.             |
| **DNS over SSH tunnel**       | ✅      | Custom resolver sends UDP queries through the SSH channel.     |
| **Auto-reconnect (SSH)**      | ✅      | Exponential back-off & keep-alive pings (`keepalive@openssh`). |
| **Structured JSON logs**      | ✅      | Powered by Uber *zap*, respects `--debug`.                     |
| **Runtime metrics**           | ✅      | Traffic, goroutines, open conns, mem & CPU every 30 s.         |
| **Cross-platform builds**     | ✅      | `make app` → Linux/macOS/Windows × amd64 & arm64.              |
| **Embedded `tun2socks` bins** | ✅      | Pre-built & vendored under `internal/tun/bins/`.               |
| **TUN / full-tunnel mode**    | 🚧     | Works on Linux/macOS; Windows Wintun dll embedded—needs QA.    |
| **Prometheus exporter**       | ❌     | Planned – expose metrics on `/metrics`.                       |
| **System service templates**  | ❌     | systemd & Windows Service descriptors TBD.                     |
| **GUI tray controller**       | ❌     | Nice-to-have for macOS/Win; contributions welcome.             |

### 6. Pre-built binaries

Below is the download matrix for every official release.  
Each archive is a **single self-contained executable**—no extra libraries required.

| OS&nbsp;/&nbsp;Arch      | File name                          | Size (≈) | SHA-256 checksum |
|--------------------------|------------------------------------|----------|------------------|
| Linux x86-64             | `ssh2proxy-linux_amd64`            | 26 MB | `fe621fd6cecf1ec52ae60529db1421a3de5bc58007ab4ec5b5f94bb071eb2d70` |
| Linux ARM64 (aarch64)    | `ssh2proxy-linux_arm64`            | 26 MB | `203e354827e4164bf819250387ddc61f96186b38d4f0a926642d88ca6ef3ce0c` |
| macOS Intel (x86-64)     | `ssh2proxy-darwin_amd64`           | 27 MB | `927c690e58868532911a3aa62fa8bddf04aceb22c31f6bd4cb88265f2122ca55` |
| macOS Apple Silicon      | `ssh2proxy-darwin_arm64`           | 25 MB | `13e106a970de831e636b6a1e69f2a367206918caa93a17ab32f364644ea42081` |
| Windows x86-64           | `ssh2proxy-windows_amd64.exe`      | 27 MB | `e0f0013bbf94bab8cb51029118fc4b791f5f5c6ef50a4e156b3ad1a35d5e6e3d` |

> ⚠️ The **TUN / full-tunnel** mode ships experimental `tun2socks` helpers embedded inside each build.  
> If you only need SOCKS5/HTTP proxying you can ignore them.

## 7. Versioning and auto-update

The binary now embeds build metadata (`Version`, `Commit`, `BuildDate`) via linker flags.

```bash
./ssh2proxy --version
```

Output example:

```text
ssh2proxy version=v1.4.2 commit=ab12cd3 build_date=2026-04-12T11:05:22Z go=go1.25.6 os=linux arch=amd64
```

### Startup auto-update (safe MVP)

The updater checks GitHub Releases **before** SSH/proxy listeners are started.  
If a newer version is found and `AUTO_UPDATE=true`, the binary is downloaded, verified, installed, and then the process is restarted with the same arguments.

Config (`.env`):

```dotenv
AUTO_UPDATE=false
AUTO_UPDATE_ALLOW_PRERELEASE=false
AUTO_UPDATE_CHECK_INTERVAL_SEC=0
AUTO_UPDATE_TIMEOUT_SEC=15
```

Behavior notes:

- `AUTO_UPDATE_CHECK_INTERVAL_SEC=0` means check only on startup.
- Releases are always fetched from `https://github.com/GoSeoTaxi/cli-ssh2proxy` (fixed in code).
- `checksums.txt` is mandatory; update is rejected if it is missing.
- A lock file is used next to the executable, so concurrent instances do not update in parallel.
- Linux/macOS: update is applied by atomic rename over the executable.
- Windows: running `.exe` is staged as `ssh2proxy.exe.new`; a helper process swaps files after exit and starts the updated binary.
- If download/checksum/install fails, startup continues with the current binary (no destructive in-place mutation before verification).

### Release requirements

Every GitHub release must include:

- platform binaries (`ssh2proxy-linux_amd64`, `...`, `ssh2proxy-windows_amd64.exe`);
- `checksums.txt` with SHA-256 for each binary.

Helper targets:

```bash
make app
make checksums
# or both:
make release-artifacts
```

Without `checksums.txt`, auto-update is intentionally blocked for security reasons.

## 8. Go runtime GC and memory tuning (optional)

`GOGC` and `GOMEMLIMIT` are **runtime** variables, not build/linker flags.  
Set them in the process environment that starts `ssh2proxy` (shell, systemd unit, Docker env, etc.).

```bash
GOGC=off GOMEMLIMIT=125MiB ./ssh2proxy
```

Notes:

- `GOGC=off` disables heap-growth GC triggering.
- `GOMEMLIMIT=125MiB` sets a **soft** memory target for Go-managed memory.
- With `GOMEMLIMIT` set, the runtime may still run GC even when `GOGC=off`.
- `125MiB` is not a strict process RSS cap: total OS-visible memory can be higher due to non-Go allocations and runtime/OS overhead.
