package player

import (
	"github.com/andresbott/govi/internal/logging"
	mpv "github.com/gen2brain/go-mpv"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// forwardMpvLogs pumps libmpv's event queue, forwarding log messages to the
// logger under component=mpv and observed property changes (idle-active) to
// the player state. It returns when mpv shuts down. Run it on its own
// goroutine: mpv_wait_event must not be called from two threads, so this pump
// is the only consumer of the event queue.
func forwardMpvLogs(p *Player) {
	l := p.log.With("component", "mpv")
	for {
		ev := p.mpv.WaitEvent(-1)
		if ev == nil {
			return
		}
		switch ev.EventID {
		case mpv.EventShutdown:
			return
		case mpv.EventLogMsg:
			msg := ev.LogMessage()
			l.Log(nil, logging.MpvLevel(msg.Level), msg.Text, "prefix", msg.Prefix) //nolint:staticcheck // slog accepts a nil context
		case mpv.EventPropertyChange:
			prop := ev.Property()
			if prop.Name == "idle-active" {
				// FormatFlag comes back as int (0/1).
				if v, ok := prop.Data.(int); ok {
					p.idle.Store(v != 0)
					glfw.PostEmptyEvent() // wake the loop so the overlay repaints
				}
			}
		}
	}
}
