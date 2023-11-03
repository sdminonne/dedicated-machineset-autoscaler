package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/csrwng/dedicated-machineset-autoscaler/pkg/autoscaler"
)

func main() {
	cmd := NewAutoscalerCommand()
	_ = cmd.Execute()
}

func NewAutoscalerCommand() *cobra.Command {
	opts := &autoscaler.Options{
		ScaleUpInterval:   10 * time.Second,
		ScaleDownInterval: 30 * time.Second,
		MinWarm:           5,
		MaxWarm:           8,
		DryMode:           false,
	}
	cmd := &cobra.Command{
		Use: "autoscaler",
		Run: func(cmd *cobra.Command, args []string) {
			if err := autoscaler.Run(context.Background(), opts); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v", err)
				os.Exit(1)
			}
		},
	}
	flagSet := cmd.Flags()
	flagSet.BoolVarP(&opts.DryMode, "dry-mode", "n", opts.DryMode, "Don't scale-up and/or scale-down when set to true")
	flagSet.DurationVar(&opts.ScaleUpInterval, "scale-up-interval", opts.ScaleUpInterval, "Interval between reconciles for scaling up (specify quantity and unit ie. 30s, 2m, etc)")
	flagSet.DurationVar(&opts.ScaleDownInterval, "scale-down-interval", opts.ScaleDownInterval, "Interval between reconciles for scaling down (specify quantity and unit ie. 30s, 2m, etc)")
	flagSet.IntVar(&opts.MinWarm, "minimum-warm", opts.MinWarm, "Minimum warm sets of nodes (number of 2-node groups from different zones for a HostedCluster)")
	flagSet.IntVar(&opts.MaxWarm, "maximum-warm", opts.MaxWarm, "Maximum warm sets of nodes (number of 2-node groups from different zones for a HostedCluster)")

	return cmd
}
