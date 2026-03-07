package daemon

import (
	"time"

	"control/internal/config"
)

func newCloudAuthRuntime(coreCfg config.CoreConfig, version string) (CloudAuthService, error) {
	cloudAuthPath, err := config.CloudAuthPath()
	if err != nil {
		return nil, err
	}
	store := newFileCloudAuthStore(cloudAuthPath)
	remote := newCloudOAuthRemoteClient(coreCfg.CloudBaseURL(), coreCfg.CloudClientID(), time.Duration(coreCfg.CloudTimeoutSeconds())*time.Second)
	svc := newCloudAuthService(store, remote, coreCfg.CloudBaseURL(), coreCfg.CloudClientID(), version)
	svc.browserBaseURL = coreCfg.CloudBrowserBaseURL()
	return svc, nil
}
