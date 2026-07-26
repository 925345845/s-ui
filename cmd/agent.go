package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	monitoragent "github.com/Hhz0823/1s-ui/agent"
)

func runAgent(panelURL, token string, interval time.Duration, insecure, once bool) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	config := monitoragent.ClientConfig{PanelURL: panelURL, Token: token, Interval: interval, Insecure: insecure}
	var err error
	if once {
		err = monitoragent.SendOnce(ctx, config)
	} else {
		err = monitoragent.Run(ctx, config)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent error:", err)
		os.Exit(1)
	}
}
