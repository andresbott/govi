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
