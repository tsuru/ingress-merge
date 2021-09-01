package main

import (
	goflag "flag"
	"log"
	"os"

	"github.com/spf13/cobra"
	ingress_merge "github.com/tsuru/ingress-merge"
	"k8s.io/api/node/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

var rootCmd = &cobra.Command{
	Use:   os.Args[0],
	Short: "Merge Ingress Controller",
	RunE: func(cmd *cobra.Command, args []string) error {
		metricsAddr, err := cmd.Flags().GetString("metrics-addr")
		if err != nil {
			return err
		}
		ingressClass, err := cmd.Flags().GetString("ingress-class")
		if err != nil {
			return err
		}
		ingressSelector, err := cmd.Flags().GetString("ingress-selector")
		if err != nil {
			return err
		}

		configMapSelector, err := cmd.Flags().GetString("configmap-selector")
		if err != nil {
			return err
		}

		ingressWatchIgnore, err := cmd.Flags().GetStringArray("ingress-watch-ignore")
		if err != nil {
			return err
		}

		configMapWatchIgnore, err := cmd.Flags().GetStringArray("configmap-watch-ignore")
		if err != nil {
			return err
		}
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:             scheme,
			MetricsBindAddress: metricsAddr,
			Port:               9443,
		})

		if err != nil {
			return err
		}

		if err = (&ingress_merge.IngressReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("IngressReconciler"),

			IngressClass:         ingressClass,
			IngressSelector:      ingressSelector,
			ConfigMapSelector:    configMapSelector,
			IngressWatchIgnore:   ingressWatchIgnore,
			ConfigMapWatchIgnore: configMapWatchIgnore,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "RpaasInstance")
			return err
		}

		setupLog.Info("starting manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			setupLog.Error(err, "problem running manager")
			return err
		}

		return nil
	},
}

func main() {
	logger := zap.New(zap.UseDevMode(true))
	ctrl.SetLogger(logger)
	rootCmd.PersistentFlags().AddGoFlagSet(goflag.CommandLine)
	err := goflag.CommandLine.Parse([]string{}) // prevents glog errors
	if err != nil {
		logger.Error(err, "Could not parse commmand line")
	}
	rootCmd.Flags().String(
		"metrics-addr",
		":8080",
		"The address the metric endpoint binds to.",
	)
	rootCmd.Flags().String(
		"ingress-class",
		"merge",
		"Process ingress resources with this `kubernetes.io/ingress.class` annotation.",
	)

	rootCmd.Flags().String(
		"ingress-selector",
		"",
		"Process ingress resources with labels matching this selector string.",
	)

	rootCmd.Flags().String(
		"configmap-selector",
		"",
		"Process configmap resources with labels matching this selector string.",
	)

	rootCmd.Flags().StringArray(
		"ingress-watch-ignore",
		[]string{},
		"Ignore ingress resources with matching annotations (can be specified multiple times).",
	)

	rootCmd.Flags().StringArray(
		"configmap-watch-ignore",
		[]string{},
		"Ignore configmap resources with matching annotations (can be specified multiple times).",
	)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
