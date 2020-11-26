package kubernetes

import (
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // enables azure/gcp auth; for side effects only
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func NewClient(kubeconfigPath string) (*kubernetes.Clientset, error) {
	kubeconfig, err := getK8sConfig(kubeconfigPath)
	if err != nil {
		log.Fatalf("unable to initialize kubernetes config: %v", err)
	}

	clientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Errorf("unable to get kube client: %v", err)
	}

	return clientSet, err
}

func getK8sConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig == "" {
		log.Infof("using in-cluster configuration")
		return rest.InClusterConfig()
	} else {
		log.Infof("using configuration from '%s'", kubeconfig)
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
}
