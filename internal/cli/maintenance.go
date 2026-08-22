package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rmpato/pogo/internal/config"
	"github.com/rmpato/pogo/internal/selfupdate"
	"github.com/rmpato/pogo/internal/version"
)

func newConfigCmd(app *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or create the configuration file",
		Long: `pogo runs correctly with no configuration at all. This command exists for
when you want to change something: what gets redacted, how much of a body is
kept, which curl to call, whether to check for releases.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(*cobra.Command, []string) error {
			fmt.Println(config.File())
			return nil
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:          "path",
		Short:        "Print the configuration file path",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(*cobra.Command, []string) error {
			fmt.Println(config.File())
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:          "init",
		Short:        "Write a configuration file with every default spelled out",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(*cobra.Command, []string) error {
			if err := app.store.Save(); err != nil {
				return err
			}
			fmt.Println("wrote", config.File())
			return nil
		},
	})

	return cmd
}

func newWhereCmd(*app) *cobra.Command {
	return &cobra.Command{
		Use:   "where",
		Short: "Print where pogo keeps its data",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			fmt.Println(config.DataDir())
		},
	}
}

func newUpdateCmd(*app) *cobra.Command {
	var check bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Install the latest release",
		Long: `update downloads the latest release for this platform, verifies it against
the published checksums, and replaces the running binary.

Checking for a release is the only thing pogo does over the network that you
did not ask for. It is cached, interactive-only, and switched off with
POGO_NO_UPDATE_CHECK=1 or 'update.disabled: true' in the config file.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(*cobra.Command, []string) error {
			if check {
				if code := selfupdate.CheckCLI("pogo", version.Version, os.Stdout, os.Stderr); code != 0 {
					return exitCode(code)
				}
				return nil
			}
			if code := selfupdate.CLI("pogo", version.Version, os.Stdout, os.Stderr); code != 0 {
				return exitCode(code)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "report whether a newer release exists, without installing it")
	return cmd
}
