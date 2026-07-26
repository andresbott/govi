# Agent notes for govi

## Read the agent docs first

Before implementation work, read `docs/agents/architecture.md` and the doc
for the area you're changing (`docs/agents/player.md` for anything in
`app/player`, `features.md` for capability status, `testing.md` /
`releasing.md` for gates). They record decisions, invariants, and deliberate
gaps that the code alone doesn't show.

## Hardware portability

This player is intended to work on **every desktop combination** of OS, GPU
vendor, and display stack (NVIDIA / AMD / Intel, X11 / Wayland, Linux / macOS /
Windows). Do not tune or hardcode behavior for the machine you happen to be
running on.

In particular:

- **Checking NVIDIA (CUDA/NVDEC) is a must.** The hwdec probe order must keep
  NVIDIA paths even though probing CUDA on a non-NVIDIA machine wastes
  ~650 ms failing `cuInit()`. Never "fix" startup latency by removing or
  skipping NVIDIA/CUDA probing, and never pin `hwdec` to a vendor-specific
  value like `vaapi` or `vaapi-copy`.
- Acceptable optimizations must preserve full hardware coverage (e.g. probing
  asynchronously, reordering probes based on the detected GPU at runtime, or
  caching a previous successful probe result) — not dropping vendors.
- The same applies to audio (PipeWire / PulseAudio / ALSA / CoreAudio / WASAPI)
  and windowing: prefer auto-detection over pinning to what works locally.

## Verify with logs before screenshots

When confirming a fix or checking behavior, **reach for logs first** — app/mpv
log output, temporary log lines, or a test assertion tell you what happened
without opening a window. Use screen grabbing and screenshots as a **last
resort**, only when what you need to confirm is genuinely visual (layout,
rendering, a UI glitch) that no log can show. Reaching for a screenshot means
the launch rule below applies.

## Launching govi requires asking the user first

Any task that needs a **running instance of govi** — taking a screenshot,
verifying a visual change, reproducing a UI bug, manual smoke-testing — must
**stop and ask the user before opening it**, and must **never close it**: the
user closes the app manually when the task is done.

The question you ask must spell out:

- **What the task is** — what you're about to do and why a live app is needed
  (e.g. "open govi to screenshot the player controls").
- **Whether the user has to do anything** — list any manual steps you need from
  them (load a file, click play, resize the window), or say plainly that no
  interaction is required.
- **That they close the app afterwards** — you will not close it for them.

**Why:** Claude has in the past randomly closed application windows and
destroyed the user's open work. Keeping the app's lifecycle in the user's hands
is the safeguard; your role is to ask first, then work with the running
instance and leave closing it to the user.
