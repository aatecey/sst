package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/sst/ion/cmd/sst/cli"
	"github.com/sst/ion/cmd/sst/mosaic/ui"
	"github.com/sst/ion/internal/util"
	"github.com/sst/ion/pkg/global"
	"github.com/sst/ion/pkg/project"
	"github.com/sst/ion/pkg/tunnel"
	"golang.org/x/sync/errgroup"
)

var CmdTunnel = &cli.Command{
	Name: "tunnel",
	Description: cli.Description{
		Short: "",
		Long:  strings.Join([]string{}, "\n"),
	},
	Run: func(c *cli.Cli) error {
		proj, err := c.InitProject()
		if err != nil {
			return err
		}
		state, err := proj.GetCompleted(c.Context)
		if err != nil {
			return err
		}
		if len(state.Tunnels) == 0 {
			return util.NewReadableError(nil, "No tunnels found for stage "+proj.App().Stage)
		}
		var tun project.Tunnel
		for _, item := range state.Tunnels {
			tun = item
		}
		subnets := strings.Join(tun.Subnets, ",")
		// run as root
		tunnelCmd := exec.Command(
			"sudo", "-n", "-E",
			tunnel.BINARY_PATH, "tunnel", "start",
			"--subnets", subnets,
			"--host", tun.IP,
			"--user", tun.Username,
		)
		util.SetProcessGroupID(tunnelCmd)
		tunnelCmd.Env = append(os.Environ(), "SSH_PRIVATE_KEY="+tun.PrivateKey)
		tunnelCmd.Stdout = os.Stdout
		tunnelCmd.Stderr = os.Stderr
		slog.Info("starting tunnel", "cmd", tunnelCmd.Args)
		err = tunnelCmd.Start()
		time.Sleep(time.Second * 1)
		fmt.Println("tunneling through", tun.IP, "for")
		for _, subnet := range tun.Subnets {
			fmt.Println("-", subnet)
		}
		<-c.Context.Done()
		util.TerminateProcess(tunnelCmd.Process.Pid)
		tunnelCmd.Wait()
		return nil
	},
	Children: []*cli.Command{
		{
			Name: "install",
			Description: cli.Description{
				Short: "Install the tunnel",
				Long: strings.Join([]string{
					"Install the tunnel.",
					"",
					"This will install the tunnel on your system.",
					"",
					"This is required for the tunnel to work.",
				}, "\n"),
			},
			Run: func(c *cli.Cli) error {
				currentUser, err := user.Current()
				if err != nil {
					return err
				}
				if currentUser.Uid != "0" {
					return util.NewReadableError(nil, "You need to run this command as root")
				}
				err = tunnel.Install()
				if err != nil {
					return err
				}
				ui.Success("Tunnel installed successfully.")
				return nil
			},
		},
		{
			Name: "start",
			Description: cli.Description{
				Short: "Start the tunnel",
				Long: strings.Join([]string{
					"Start the tunnel.",
					"",
					"This will start the tunnel.",
					"",
					"This is required for the tunnel to work.",
				}, "\n"),
			},
			Hidden: true,
			Flags: []cli.Flag{
				{
					Name: "subnets",
					Type: "string",
					Description: cli.Description{
						Short: "The subnet to use for the tunnel",
						Long:  "The subnet to use for the tunnel",
					},
				},
				{
					Name: "host",
					Type: "string",
					Description: cli.Description{
						Short: "The host to use for the tunnel",
						Long:  "The host to use for the tunnel",
					},
				},
				{
					Name: "port",
					Type: "string",
					Description: cli.Description{
						Short: "The port to use for the tunnel",
						Long:  "The port to use for the tunnel",
					},
				},
				{
					Name: "user",
					Type: "string",
					Description: cli.Description{
						Short: "The user to use for the tunnel",
						Long:  "The user to use for the tunnel",
					},
				},
			},
			Run: func(c *cli.Cli) error {
				err := global.EnsureTun2Socks()
				if err != nil {
					return err
				}
				subnets := strings.Split(c.String("subnets"), ",")
				host := c.String("host")
				port := c.String("port")
				user := c.String("user")
				if port == "" {
					port = "22"
				}
				slog.Info("starting tunnel", "subnet", subnets, "host", host, "port", port)
				var wg errgroup.Group
				wg.Go(func() error {
					defer c.Cancel()
					return tunnel.StartProxy(
						c.Context,
						user,
						host+":"+port,
						[]byte(os.Getenv("SSH_PRIVATE_KEY")),
					)
				})
				wg.Go(func() error {
					defer c.Cancel()
					return tunnel.Start(c.Context, subnets...)
				})
				slog.Info("tunnel started")
				wg.Wait()
				return nil
			},
		},
	},
}