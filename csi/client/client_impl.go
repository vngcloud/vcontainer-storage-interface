package client

import (
	"github.com/cuongpiger/joat/utils"
	"github.com/vngcloud/vcontainer-sdk/client"
	"github.com/vngcloud/vcontainer-sdk/vcontainer"

	"k8s.io/klog/v2"
)

func LogCfg(authOpts AuthOpts) {
	// If not comment, those fields are empty
	klog.V(5).Infof("Identity-URL: %s", authOpts.IdentityURL)
	klog.V(5).Infof("vServer-URL: %s", authOpts.VServerURL)
	klog.V(5).Infof("Client-ID: %s", authOpts.ClientID)
}

func NewVContainerClient(authOpts *AuthOpts) (*client.ProviderClient, error) {
	identityUrl := utils.NormalizeURL(authOpts.IdentityURL) + "v2"
	provider, _ := vcontainer.NewClient(identityUrl)
	err := vcontainer.Authenticate(provider, authOpts.ToOAuth2Options())

	return provider, err
}
