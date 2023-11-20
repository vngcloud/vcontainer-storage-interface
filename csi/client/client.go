package client

import (
	"github.com/vngcloud/vcontainer-sdk/vcontainer/services/identity/v2/extensions/oauth2"
	"github.com/vngcloud/vcontainer-sdk/vcontainer/services/identity/v2/tokens"
)

type (
	AuthOpts struct {
		IdentityURL       string `gcfg:"identity-url" mapstructure:"identity-url" name:"identity-url"`
		ComputeURL        string `gcfg:"compute-url" mapstructure:"compute-url" name:"compute-url"`
		BlockstorageURL   string `gcfg:"blockstorage-url" mapstructure:"blockstorage-url" name:"blockstorage-url"`
		VBackUpGateWayURL string `gcfg:"vbackup-gateway-url" mapstructure:"vbackup-gateway-url" name:"vbackup-gateway-url"`
		PortalURL         string `gcfg:"portal-url" mapstructure:"portal-url" name:"portal-url"`
		ClientID          string `gcfg:"client-id" mapstructure:"client-id" name:"client-id"`
		ClientSecret      string `gcfg:"client-secret" mapstructure:"client-secret" name:"client-secret"`
		CAFile            string `gcfg:"ca-file" mapstructure:"ca-file" name:"ca-file"`
	}
)

func (s *AuthOpts) ToOAuth2Options() *oauth2.AuthOptions {
	return &oauth2.AuthOptions{
		ClientID:     s.ClientID,
		ClientSecret: s.ClientSecret,
		AuthOptionsBuilder: &tokens.AuthOptions{
			IdentityEndpoint: s.IdentityURL,
		},
	}
}
