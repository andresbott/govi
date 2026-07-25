package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/andresbott/govi/app/metainfo"
	"github.com/andresbott/govi/app/player"
	"github.com/andresbott/govi/internal/logging"
	"github.com/spf13/cobra"
)

// Execute is the entry point for the command line.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Uninstall the handler once the first signal has been seen, so a second
	// one gets the default behaviour (terminate) even if the graceful shutdown
	// of mpv or the GL window wedges.
	go func() {
		<-ctx.Done()
		stop()
	}()
	if err := newRootCommand().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runPlayer is the player entry point, indirected so tests can substitute it
// (the real one opens a window). It must receive the command's context: that
// is what makes Ctrl-C quit, since Execute replaces the default SIGINT
// behaviour with cancellation.
var runPlayer = player.Run

func newRootCommand() *cobra.Command {
	var logLevel string
	var configPath string

	cmd := &cobra.Command{
		Use:           "govi [file]",
		Short:         "govi: minimalistic video player based on mpv",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			level, err := logging.ParseLevel(logLevel)
			if err != nil {
				return err
			}
			slog.SetDefault(logging.New(cmd.ErrOrStderr(), level))
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				p, err := defaultConfigPath()
				if err != nil {
					return err
				}
				configPath = p
			}
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			pc := cfg.toPlayerConfig()
			cfgPath := configPath
			pc.SaveShortcuts = func(sc map[string][]string) error {
				return saveShortcuts(cfgPath, sc)
			}
			return runPlayer(cmd.Context(), path, pc)
		},
	}

	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		_ = cmd.Help()
		return nil
	})

	cmd.PersistentFlags().StringVar(&logLevel, "log", "warn",
		"log level: trace, debug, info, warn or error")
	cmd.PersistentFlags().StringVar(&configPath, "config", "",
		"path to config file (default: <user config dir>/govi/config.yaml)")

	cmd.AddCommand(
		versionCmd(),
	)

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print version information",
		Run: func(cmd *cobra.Command, args []string) {
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Version:    %s\n", metainfo.Version)
			_, _ = fmt.Fprintf(out, "Build date: %s\n", metainfo.BuildTime)
			_, _ = fmt.Fprintf(out, "Commit sha: %s\n", metainfo.ShaVer)
			_, _ = fmt.Fprintf(out, "Compiler:   %s\n", runtime.Version())
		},
	}
}
