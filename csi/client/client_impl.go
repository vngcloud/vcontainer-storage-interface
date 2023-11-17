package client

import (
	"github.com/vngcloud/vcontainer-sdk/client"
	"github.com/vngcloud/vcontainer-sdk/vcontainer"

	"k8s.io/klog/v2"
)

func LogCfg(authOpts AuthOpts) {
	// If not comment, those fields are empty
	klog.V(5).Infof("Identity-URL: %s", authOpts.IdentityURL)
	klog.V(5).Infof("Compute-URL: %s", authOpts.ComputeURL)
	klog.V(5).Infof("Blockstorage-URL: %s", authOpts.BlockstorageURL)
	klog.V(5).Infof("Portal-URL: %s", authOpts.PortalURL)
	klog.V(5).Infof("Client-ID: %s", authOpts.ClientID)
}

func NewVContainerClient(authOpts *AuthOpts) (*client.ProviderClient, error) {
	provider, _ := vcontainer.NewClient(authOpts.IdentityURL)
	err := vcontainer.Authenticate(provider, authOpts.ToOAuth2Options())

	return provider, err
}
