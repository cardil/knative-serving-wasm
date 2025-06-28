package k8s

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// CheckConnection will return an error of connecting to Kubernetes, or nil if
// a connection can be established.
func CheckConnection() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	config, err := loader.ClientConfig()
	if err != nil {
		return err
	}

	var client *kubernetes.Clientset

	if client, err = kubernetes.NewForConfig(config); err != nil {
		return err
	}

	// Perform a lightweight API call to verify actual connectivity
	_, err = client.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	return nil
}
