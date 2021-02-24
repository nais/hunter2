package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nais/hunter2/pkg/metrics"

	"github.com/nais/hunter2/pkg/google"
	"github.com/nais/hunter2/pkg/kubernetes"
	"github.com/nais/hunter2/pkg/synchronizer"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// Configuration options
const (
	KubeconfigPath             = "kubeconfig-path"
	BindAddress                = "bind-address"
	Debug                      = "debug"
	GoogleProjectID            = "google-project-id"
	GooglePubsubSubscriptionID = "google-pubsub-subscription-id"
	ReportInterval             = "report-interval"
)

func init() {
	viper.SetEnvPrefix("HUNTER2")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	flag.String(BindAddress, "127.0.0.1:8080", "Bind address for application.")
	flag.Bool(Debug, false, "enables debug logging")
	flag.String(GoogleProjectID, "", "GCP project ID.")
	flag.String(GooglePubsubSubscriptionID, "", "GCP subscription ID for the PubSub topic to consume from.")
	flag.String(KubeconfigPath, "", "path to Kubernetes config file")
	flag.Duration(ReportInterval, 5*time.Minute, "How often to collect number of Kubernetes secrets in cluster")

	flag.Parse()

	err := viper.BindPFlags(flag.CommandLine)
	if err != nil {
		panic(err)
	}
}

func main() {
	setupLogging()

	stopChan := make(chan struct{}, 1)

	go serve(viper.GetString(BindAddress))
	go handleSigterm(stopChan)

	clientSet, err := kubernetes.NewClient(viper.GetString(KubeconfigPath))
	if err != nil {
		log.Fatalf("getting kubernetes clientset: %v", err)
	}

	ctx := context.Background()
	googleProjectID := viper.GetString(GoogleProjectID)
	googlePubsubSubscriptionID := viper.GetString(GooglePubsubSubscriptionID)

	resourceManagerClient, err := google.NewResourceManagerClient(ctx)
	if err != nil {
		log.Fatalf("getting pubsubclient: %v", err)
	}

	pubsubClient, err := google.NewPubSubClient(ctx, googleProjectID, googlePubsubSubscriptionID, resourceManagerClient)
	if err != nil {
		log.Fatalf("getting resource manager client: %v", err)
	}

	secretManagerClient, err := google.NewSecretManagerClient(ctx)
	if err != nil {
		log.Fatalf("getting secret manager client: %v", err)
	}

	syncer := synchronizer.NewSynchronizer(log.NewEntry(log.StandardLogger()), secretManagerClient, clientSet)

	secretCounter := time.NewTicker(1 * time.Second)

	messages := pubsubClient.Consume(ctx)
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				log.Errorf("lost connection to pubsub; retrying...")
				time.Sleep(time.Second * 5)
				messages = pubsubClient.Consume(ctx)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			err := syncer.Sync(ctx, msg)
			cancel()
			if err != nil {
				log.Errorf("synchronizing secret: %v", err)
			}
		case <-secretCounter.C:
			log.Debugf("reporting total number of managed secrets...")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			secrets, err := syncer.ManagedSecrets(ctx)
			cancel()

			secretCounter.Reset(viper.GetDuration(ReportInterval))

			if err != nil {
				log.Errorf("list managed secrets from cluster: %s", err)
			} else {
				metrics.ManagedSecrets.Set(float64(len(secrets)))
			}
		case <-stopChan:
			return
		}
	}
}

func setupLogging() {
	log.SetFormatter(&log.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	})
	log.SetOutput(os.Stdout)
	if viper.GetBool(Debug) {
		log.SetLevel(log.DebugLevel)
	}
}

// Provides health check and metrics routes
func serve(address string) {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	prometheus.MustRegister(metrics.Requests)
	prometheus.MustRegister(metrics.GoogleSecretManagerResponseTime)
	prometheus.MustRegister(metrics.ManagedSecrets)
	metrics.InitLabels()

	http.Handle("/metrics", promhttp.Handler())

	log.Infof("server started on %s", address)
	log.Fatal(http.ListenAndServe(address, nil))
}

// Handles SIGTERM and exits
func handleSigterm(stopChan chan struct{}) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("received SIGTERM. Terminating...")
	close(stopChan)
}
