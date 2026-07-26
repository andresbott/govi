// Package player implements the govi playback window: a GLFW-owned OpenGL
// context where libmpv renders video frames and Gio draws the control
// overlay into the same framebuffer, so video stays on the GPU end to end.
package player

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/gpu"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/andresbott/govi/internal/logging"
	mpv "github.com/gen2brain/go-mpv"
	"github.com/go-gl/gl/v3.1/gles2"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	// appID is the window class / Wayland app id; it has to match the basename
	// of the installed desktop entry (zarf/govi.desktop) for the shell to
	// associate window and launcher icon.
	appID         = "govi"
	initialWidth  = 1280
	initialHeight = 720
	// idleFrame caps how long the loop sleeps when no video frame or input
	// arrives, so the Gio overlay keeps animating even while paused.
	idleFrame = 0.05
)

// desktopGL selects the (core, desktop) OpenGL backend instead of OpenGL ES;
// Gio's renderer expects ES everywhere except macOS.
const desktopGL = runtime.GOOS == "darwin"

func init() {
	// GLFW event handling and GL contexts must stay on the main OS thread.
	runtime.LockOSThread()
}

type Player struct {
	log    *slog.Logger
	window *glfw.Window
	mpv    *mpv.Mpv
	render *mpv.RenderContext

	gpuCtx gpu.GPU
	router input.Router
	ops    op.Ops
	theme  *material.Theme

	placeholderSrc paint.ImageOp
	paused         bool
	muted          bool
	overlay        overlayKind
	keymap         map[keyChord]actionID
	actions        map[actionID]action

	// context-menu state: items rebuilt on open, position in framebuffer px.
	menuPos   f32.Point
	menuItems []*menuItem

	// confirmPath is the file the delete-confirmation overlay is asking about.
	confirmPath string

	// preferences state: user shortcut overrides in config syntax, the save
	// callback injected via Config, the row widgets rebuilt on open, the slot
	// currently capturing a key press, and a user-facing message line.
	// prefsWanted is the requested state of the preferences window; the loop
	// reconciles it with prefsWin (see prefsWindowAction).
	overrides     map[actionID][]string
	saveShortcuts func(map[string][]string) error
	prefsWanted   bool
	prefsWin      *prefsWindow
	prefsRows     []*prefsRow
	prefsList     widget.List
	capture       *prefsCapture
	prefsMsg      string

	// pl lists the sibling videos of the last opened file for next/previous
	// navigation; nil when the source has no folder (URL) or scanning failed.
	pl *playlist

	// pf warms the OS page cache for the entries around the current one, so
	// next/previous does not wait on cold-storage reads.
	pf prefetcher

	// infoCache holds the info overlay's sections, re-read from mpv once per
	// second while the overlay is open (see loop).
	infoCache       []infoSection
	infoRefreshedAt time.Time

	// osdText is the transient status flash (volume level, playlist position)
	// drawn until osdUntil passes; see osd.go. Playback progress is not among
	// them — the control bar reports that (controls.go).
	osdText  string
	osdUntil time.Time

	// lastRepeat is when each action last fired, so held-down keys are
	// throttled to the action's own repeat interval (see dispatchKey).
	lastRepeat map[actionID]time.Time

	// control-bar state (see controls.go): the seek bar's knob position, the
	// two buttons, when pointer input last arrived (the bar auto-hides
	// controlsHideAfter later) and the fade-in's origin. Zero lastInput means the
	// pointer has not moved yet, so the bar starts hidden.
	progress   widget.Float
	playBtn    widget.Clickable
	volumeBtn  widget.Clickable
	lastInput  time.Time
	revealedAt time.Time

	// windowed geometry remembered while fullscreen, restored on exit.
	winX, winY, winW, winH int

	// needsRender is set from mpv's render thread when a new video frame is
	// ready and consumed on the main loop.
	needsRender atomic.Bool

	// idle mirrors mpv's idle-active property (no file loaded), published
	// from the mpv event goroutine and read by the render loop / overlay.
	idle atomic.Bool

	// eofPending is set from the mpv event goroutine when the current file
	// ended on its own (see autoadvance.go) and consumed on the main loop,
	// which then plays the next playlist entry.
	eofPending atomic.Bool

	// obsPos/obsDur are the observed playback position and duration
	// (see playback.go).
	obsPos atomic.Uint64
	obsDur atomic.Uint64
}

// overlayKind is which single overlay (if any) is currently shown.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayInfo
	overlayHelp
	overlayMenu
	overlayConfirm
)

// Config is the player's runtime configuration, free of any config-library
// or YAML types. Shortcuts holds only the actions the user explicitly set
// (action id -> key strings); absent actions fall back to built-in defaults.
// SaveShortcuts, when non-nil, persists a replacement override map to disk;
// it is injected by app/cmd so the player never touches YAML.
type Config struct {
	Shortcuts        map[string][]string
	PlaceholderImage string
	SaveShortcuts    func(map[string][]string) error
}

// Run opens a player window and plays the given file until the window closes
// or ctx is cancelled (SIGINT/SIGTERM from app/cmd).
func Run(ctx context.Context, path string, cfg Config) error {
	p := &Player{log: slog.Default()}
	p.actions = actionByID()

	overrides := make(map[actionID][]string, len(cfg.Shortcuts))
	for id, keys := range cfg.Shortcuts {
		overrides[actionID(id)] = keys
	}
	km, err := buildKeymap(overrides)
	if err != nil {
		return fmt.Errorf("build keymap: %w", err)
	}
	p.keymap = km
	p.overrides = overrides
	p.saveShortcuts = cfg.SaveShortcuts

	start := time.Now()

	if err := p.initWindow(); err != nil {
		return err
	}
	defer glfw.Terminate()
	defer p.window.Destroy()
	p.log.Debug("window and GL context ready", "elapsed", time.Since(start))

	if err := p.initGio(); err != nil {
		return err
	}
	defer p.gpuCtx.Release()
	p.log.Debug("gio renderer ready", "elapsed", time.Since(start))

	img := loadPlaceholder(cfg.PlaceholderImage, p.log)
	p.placeholderSrc = paint.NewImageOp(img)

	if err := p.initMpv(); err != nil {
		return err
	}
	p.log.Debug("mpv initialized", "elapsed", time.Since(start))

	// Pump mpv's event queue for log messages on its own goroutine. libmpv
	// forbids terminate_destroy while a thread is in wait_event, so shut
	// down in order: quit -> pump drains and returns on shutdown event ->
	// terminate. The render context must also be freed before terminate.
	logsDone := make(chan struct{})
	go func() {
		forwardMpvLogs(p)
		close(logsDone)
	}()
	defer func() {
		_ = p.mpv.Command([]string{"quit"}) // best-effort shutdown
		select {
		case <-logsDone:
		case <-time.After(3 * time.Second):
			p.log.Warn("mpv log pump did not stop in time")
		}
		p.render.Free()
		p.mpv.TerminateDestroy()
	}()

	// Stop warming neighbours before mpv goes away; the reads are best-effort
	// and must not outlive the player.
	defer p.pf.stop()

	p.registerCallbacks()

	if path != "" {
		if err := p.mpv.Command([]string{"loadfile", path}); err != nil {
			return fmt.Errorf("loadfile %q: %w", path, err)
		}
		p.setPlaylist(path)
		p.log.Info("playing", "file", path, "startup", time.Since(start))
	} else {
		p.log.Info("started on idle screen", "startup", time.Since(start))
	}

	return p.loop(ctx)
}

func (p *Player) initWindow() error {
	if err := glfw.Init(); err != nil {
		return fmt.Errorf("init glfw: %w", err)
	}

	// The backbuffer must stay linear: mpv already outputs sRGB-encoded
	// pixels, so an sRGB-encoding framebuffer would gamma-encode them a
	// second time (washed-out colors). On GLES the sRGB EGL surface cannot
	// be toggled off per draw, so don't request one — Gio detects the
	// linear framebuffer and gamma-corrects its overlay through its own
	// intermediate sRGB FBO. On desktop GL, Gio instead expects an
	// sRGB-capable backbuffer and enables FRAMEBUFFER_SRGB only while it
	// draws; encoding is off while mpv renders (see frame()).
	if desktopGL {
		glfw.WindowHint(glfw.SRGBCapable, glfw.True)
	}
	glfw.WindowHint(glfw.ScaleToMonitor, glfw.True)
	glfw.WindowHint(glfw.CocoaRetinaFramebuffer, glfw.True)
	// WM_CLASS must match StartupWMClass in zarf/govi.desktop, otherwise
	// the desktop shell cannot tie the window to the .desktop entry (no icon in
	// the task bar, no pinning). Without these hints GLFW derives the class from
	// the window title, which changes with the playing file. Hints are sticky,
	// so the preferences window inherits the same class — intended, it belongs
	// to the same application.
	glfw.WindowHintString(glfw.X11ClassName, appID)
	glfw.WindowHintString(glfw.X11InstanceName, appID)
	if desktopGL {
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 3)
		glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
		glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	} else {
		glfw.WindowHint(glfw.ContextCreationAPI, glfw.EGLContextAPI)
		glfw.WindowHint(glfw.ClientAPI, glfw.OpenGLESAPI)
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 0)
	}

	window, err := glfw.CreateWindow(initialWidth, initialHeight, appID, nil, nil)
	if err != nil {
		glfw.Terminate()
		return fmt.Errorf("create window: %w", err)
	}
	p.window = window
	p.window.MakeContextCurrent()
	glfw.SwapInterval(1)

	if desktopGL {
		err = gl.Init()
	} else {
		err = gles2.Init()
	}
	if err != nil {
		return fmt.Errorf("init gl: %w", err)
	}
	if desktopGL {
		// Note: GL_FRAMEBUFFER_SRGB stays disabled here. Gio enables it for
		// its own pass and restores it; leaving it on globally would make
		// mpv's already-encoded output get gamma-encoded twice.
		// Set up default VAO, required for the forward-compatible core profile.
		var defVBA uint32
		gl.GenVertexArrays(1, &defVBA)
		gl.BindVertexArray(defVBA)
	}
	return nil
}

func (p *Player) initGio() error {
	p.theme = material.NewTheme()
	p.theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	gpuCtx, err := gpu.New(gpu.OpenGL{ES: !desktopGL, Shared: true})
	if err != nil {
		return fmt.Errorf("init gio gpu: %w", err)
	}
	p.gpuCtx = gpuCtx
	return nil
}

func (p *Player) initMpv() error {
	p.mpv = mpv.New()

	for k, v := range map[string]string{
		"vo":         "libmpv",
		"volume-max": "100",
		"idle":       "yes",
		// Priority list instead of plain "auto": every vendor is still probed
		// (see AGENTS.md), but cheap-to-fail probes come first so the CUDA
		// probe (~650ms of cuInit on machines without an NVIDIA GPU) only
		// runs when nothing faster matched. vdpau stays behind nvdec so
		// NVIDIA machines pick NVDEC over the legacy VDPAU path. The trailing
		// "auto" keeps coverage for anything not listed explicitly.
		"hwdec":        "videotoolbox,d3d11va,d3d11va-copy,vaapi,vaapi-copy,nvdec,nvdec-copy,vdpau,vdpau-copy,auto",
		"hwdec-codecs": "all",
	} {
		if err := p.mpv.SetOptionString(k, v); err != nil {
			return fmt.Errorf("set mpv option %s=%s: %w", k, v, err)
		}
	}

	// Ask libmpv for log messages matching the logger's verbosity; the pump
	// goroutine forwards them (component=mpv). This is the primary tool for
	// diagnosing startup latency: hwdec probing, audio init and first-frame
	// timing all show up here at debug/verbose level.
	minLevel := slog.LevelError
	for _, l := range []slog.Level{logging.LevelTrace, slog.LevelDebug, slog.LevelInfo, slog.LevelWarn} {
		if p.log.Enabled(context.Background(), l) {
			minLevel = l
			break
		}
	}
	if err := p.mpv.RequestLogMessages(logging.MpvRequestLevel(minLevel)); err != nil {
		return fmt.Errorf("request mpv log messages: %w", err)
	}

	if err := p.mpv.Initialize(); err != nil {
		return fmt.Errorf("init mpv: %w", err)
	}

	// Observe idle-active so the overlay can draw the idle screen whenever no
	// file is loaded (launch, after stop, or end-of-file). userdata 1 tags
	// this observation in the event loop.
	if err := p.mpv.ObserveProperty(1, "idle-active", mpv.FormatFlag); err != nil {
		return fmt.Errorf("observe idle-active: %w", err)
	}

	// Observe position and duration so the render loop reads them from the
	// cache: a synchronous property read from this thread blocks on mpv's
	// 200 ms render timeout after a seek (see playback.go).
	if err := p.observePlayback(); err != nil {
		return err
	}

	render, err := p.mpv.NewRenderContextGL(func(name string) unsafe.Pointer {
		return glfw.GetProcAddress(name)
	})
	if err != nil {
		return fmt.Errorf("create mpv render context: %w", err)
	}
	p.render = render

	// The callback runs on an mpv thread: only flag the new frame and wake
	// the main loop, never touch mpv or GL from here.
	p.render.SetUpdateCallback(func() {
		p.needsRender.Store(true)
		glfw.PostEmptyEvent()
	})
	return nil
}

func (p *Player) loop(ctx context.Context) error {
	loopStart := time.Now()
	firstFrame := false
	defer p.destroyPrefsWindow()

	// A signal can arrive while the loop sleeps in WaitEventsTimeout, so wake
	// it as soon as ctx is cancelled instead of waiting out the timeout.
	// PostEmptyEvent is one of GLFW's thread-safe calls.
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			glfw.PostEmptyEvent()
		case <-stopped:
		}
	}()

	for !p.window.ShouldClose() {
		glfw.WaitEventsTimeout(idleFrame)

		if err := ctx.Err(); err != nil {
			p.log.Info("shutting down", "reason", err)
			return nil
		}

		// Create or destroy the preferences window here: GLFW forbids it from
		// inside a callback. Its close button revokes the request first.
		if p.prefsWin != nil && p.prefsWin.win.ShouldClose() {
			p.prefsWanted = false
		}
		p.syncPrefsWindow()

		// Continue with the next video when the current one ended (played
		// through, or seeked past its end). Done here rather than in the event
		// pump so loadfile stays on the main loop.
		p.handleEndOfFile()

		// Acknowledge mpv's update flag when a new video frame is ready.
		if p.needsRender.CompareAndSwap(true, false) {
			p.render.Update()
			if !firstFrame {
				firstFrame = true
				p.log.Info("first video frame ready", "since-loadfile", time.Since(loopStart))
			}
		}

		if p.overlay == overlayInfo {
			if time.Since(p.infoRefreshedAt) >= time.Second || p.infoCache == nil {
				p.infoCache = p.infoSections()
				p.infoRefreshedAt = time.Now()
			}
		} else {
			p.infoCache = nil
		}

		// Render every iteration so the overlay stays responsive even while
		// playback is paused and no new video frames arrive.
		frameStart := time.Now()
		if err := p.frame(); err != nil {
			return err
		}
		logging.Trace(p.log, "frame rendered", "took", time.Since(frameStart))

		// The preferences window renders in the same iteration, on its own
		// context (unvsynced, so it never throttles the video window).
		if p.prefsWin != nil {
			if err := p.prefsFrame(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Player) frame() error {
	scale, _ := p.window.GetContentScale()
	width, height := p.window.GetFramebufferSize()
	sz := image.Point{X: width, Y: height}

	clear()

	// mpv paints the whole viewport (flipY: GL's origin is bottom-left).
	if err := p.render.RenderGL(0, width, height, true); err != nil {
		return fmt.Errorf("mpv render: %w", err)
	}

	p.ops.Reset()
	gtx := layout.Context{
		Ops:    &p.ops,
		Now:    time.Now(),
		Source: p.router.Source(),
		Metric: unit.Metric{
			PxPerDp: scale,
			PxPerSp: scale,
		},
		Constraints: layout.Exact(sz),
	}
	p.layoutUI(gtx)

	// Gio composites the overlay into the same (default) framebuffer.
	if err := p.gpuCtx.Frame(gtx.Ops, gpu.OpenGLRenderTarget{}, sz); err != nil {
		return fmt.Errorf("gio frame: %w", err)
	}
	p.router.Frame(&p.ops)

	p.window.SwapBuffers()
	p.render.ReportSwap()
	return nil
}

func clear() {
	if desktopGL {
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT)
	} else {
		gles2.ClearColor(0, 0, 0, 1)
		gles2.Clear(gles2.COLOR_BUFFER_BIT)
	}
}

func (p *Player) togglePause() {
	p.paused = !p.paused
	p.log.Debug("toggle pause", "paused", p.paused)
	if err := p.mpv.SetProperty("pause", mpv.FormatFlag, p.paused); err != nil {
		p.log.Error("set pause property", "paused", p.paused, "err", err)
	}
}

// openFile plays path and rebuilds the playlist from its folder, so
// next/previous navigate the new file's siblings. Non-local sources (URLs)
// clear the playlist.
func (p *Player) openFile(path string) {
	p.loadFile(path)
	p.setPlaylist(path)
}

// setPlaylist rebuilds the playlist from path's folder and warms the page
// cache for its neighbours, so the first next/previous is already in memory.
func (p *Player) setPlaylist(path string) {
	p.pl = scanPlaylist(path)
	p.pf.start(p.pl.neighbors())
}

// loadFile asks mpv to play path without touching the playlist (used for
// playlist navigation itself). Errors are logged, not fatal: mpv emits its
// own error and returns to idle for unplayable files.
func (p *Player) loadFile(path string) {
	// Any pending end-of-file belongs to the file being replaced: the user (or
	// an auto-advance already handled) picked this one, so the loop must not
	// advance again on top of it and skip an entry. Done before the mpv guard
	// so the flag is dropped even on the test path.
	p.eofPending.Store(false)
	if p.mpv == nil { // unit tests build a Player without mpv
		return
	}
	if err := p.mpv.Command([]string{"loadfile", path}); err != nil {
		p.log.Error("loadfile", "path", path, "err", err)
		return
	}
	p.log.Info("playing", "file", path)
}

// stop returns to the idle screen. mpv's stop unloads the current file.
func (p *Player) stop() {
	if err := p.mpv.Command([]string{"stop"}); err != nil {
		p.log.Error("mpv stop", "err", err)
	}
}

// showProgress brings the control bar up on demand (its own action), so the
// position is reachable without moving the mouse or disturbing playback.
func (p *Player) showProgress() {
	p.revealControls(time.Now())
}

// changeVolume nudges mpv's volume by delta (percent), clamped by mpv's
// own volume-max (set to 100 in initMpv), and flashes the resulting level.
func (p *Player) changeVolume(delta int) {
	if err := p.mpv.Command([]string{"add", "volume", fmt.Sprintf("%d", delta)}); err != nil {
		p.log.Error("mpv add volume", "delta", delta, "err", err)
		return
	}
	p.flashVolume()
}

// toggleMute flips mpv's mute flag and flashes the new state.
func (p *Player) toggleMute() {
	p.muted = !p.muted
	if err := p.mpv.SetProperty("mute", mpv.FormatFlag, p.muted); err != nil {
		p.log.Error("set mute property", "muted", p.muted, "err", err)
		return
	}
	p.flashVolume()
}

// toggleOverlay opens ov, or closes it if it is already open. Rendering is
// added in a later piece; the state field is wired now so dispatch is stable.
func (p *Player) toggleOverlay(ov overlayKind) {
	if p.overlay == ov {
		p.overlay = overlayNone
	} else {
		p.overlay = ov
	}
}

// openMenu builds the context menu appropriate to the current state and shows
// it at pos (framebuffer pixels).
func (p *Player) openMenu(pos f32.Point) {
	p.menuItems = p.buildMenu(p.idle.Load())
	p.menuPos = pos
	p.overlay = overlayMenu
}

// closeMenu hides the context menu.
func (p *Player) closeMenu() {
	if p.overlay == overlayMenu {
		p.overlay = overlayNone
	}
	p.menuItems = nil
}

// trackList reads and parses mpv's current track-list.
func (p *Player) trackList() []trackInfo {
	v, err := p.mpv.GetProperty("track-list", mpv.FormatNode)
	if err != nil {
		p.log.Error("get track-list", "err", err)
		return nil
	}
	return parseTrackList(v)
}

// setTrack selects a track by kind ("audio"->aid, "video"->vid, "sub"->sid).
// An id <= 0 disables that track ("no").
func (p *Player) setTrack(kind string, id int64) {
	prop := map[string]string{"audio": "aid", "video": "vid", "sub": "sid"}[kind]
	if prop == "" {
		return
	}
	value := fmt.Sprintf("%d", id)
	if id <= 0 {
		value = "no"
	}
	if err := p.mpv.SetPropertyString(prop, value); err != nil {
		p.log.Error("set track", "prop", prop, "value", value, "err", err)
	}
}

// setAspect sets video-aspect-override.
func (p *Player) setAspect(value string) {
	if err := p.mpv.SetPropertyString("video-aspect-override", value); err != nil {
		p.log.Error("set aspect", "value", value, "err", err)
	}
}

// setZoom sets video-zoom from a user-facing factor.
func (p *Player) setZoom(factor float64) {
	if err := p.mpv.SetProperty("video-zoom", mpv.FormatDouble, zoomToVideoZoom(factor)); err != nil {
		p.log.Error("set zoom", "factor", factor, "err", err)
	}
}

// rectangle is a screen-space box used to pick the monitor a window occupies.
type rectangle struct{ x, y, w, h int }

// pickMonitor returns the index of the monitor the window overlaps most, or 0
// if there is no overlap (e.g. window off-screen).
func pickMonitor(win rectangle, monitors []rectangle) int {
	best, bestArea := 0, -1
	for i, m := range monitors {
		ox := overlap1D(win.x, win.x+win.w, m.x, m.x+m.w)
		oy := overlap1D(win.y, win.y+win.h, m.y, m.y+m.h)
		area := ox * oy
		if area > bestArea {
			best, bestArea = i, area
		}
	}
	return best
}

func overlap1D(a0, a1, b0, b1 int) int {
	lo := a0
	if b0 > lo {
		lo = b0
	}
	hi := a1
	if b1 < hi {
		hi = b1
	}
	if hi <= lo {
		return 0
	}
	return hi - lo
}

// toggleFullscreen switches the window between windowed and borderless
// fullscreen on the monitor it mostly occupies, remembering the windowed
// geometry so it can be restored.
func (p *Player) toggleFullscreen() {
	if p.window.GetMonitor() != nil {
		p.window.SetMonitor(nil, p.winX, p.winY, p.winW, p.winH, 0)
		return
	}
	p.winX, p.winY = p.window.GetPos()
	p.winW, p.winH = p.window.GetSize()

	monitors := glfw.GetMonitors()
	if len(monitors) == 0 {
		return
	}
	rects := make([]rectangle, len(monitors))
	for i, m := range monitors {
		mx, my := m.GetPos()
		mode := m.GetVideoMode()
		rects[i] = rectangle{mx, my, mode.Width, mode.Height}
	}
	idx := pickMonitor(rectangle{p.winX, p.winY, p.winW, p.winH}, rects)
	m := monitors[idx]
	mode := m.GetVideoMode()
	mx, my := m.GetPos()
	p.window.SetMonitor(m, mx, my, mode.Width, mode.Height, mode.RefreshRate)
}
