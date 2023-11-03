package autoscaler

import (
	"context"
	"fmt"
	"time"

	_ "github.com/openshift/api/machine/v1"
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/scale"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type Options struct {
	ScaleUpInterval   time.Duration
	ScaleDownInterval time.Duration
	MinWarm           int
	MaxWarm           int
}

func Run(ctx context.Context, opts *Options) error {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.JSONEncoder(func(o *zapcore.EncoderConfig) {
		o.EncodeTime = zapcore.RFC3339TimeEncoder
	})))

	log := ctrl.LoggerFrom(ctx)
	log.Info("Starting autoscaler controller")

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = "Autoscaler controller"

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	if err := (&ScaleUpReconciler{
		Client:   mgr.GetClient(),
		Interval: opts.ScaleUpInterval,
		MinWarm:  opts.MinWarm,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to setup scale up reconciler: %w", err)
	}

	if err := (&ScaleDownReconciler{
		Client:   mgr.GetClient(),
		Interval: opts.ScaleDownInterval,
		MaxWarm:  opts.MaxWarm,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to setup scale down reconciler: %w", err)
	}

	// Start the controllers
	log.Info("starting manager")
	return mgr.Start(ctx)
}
