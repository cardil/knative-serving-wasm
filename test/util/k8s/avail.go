package k8s

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// CheckConnection will return an error of connecting to Kubernetes, or nil if
// a connection can be established.
func CheckConnection() error {
	loadingRules := &clientcmd.ClientConfigLoadingRules{}
	overrides := &clientcmd.ConfigOverrides{}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	config, err := loader.ClientConfig()
	if err != nil {
		return err
	}

	if _, err = kubernetes.NewForConfig(config); err != nil {
		return err
	}
	return nil
}
