package daemon

import "control/internal/config"

func loadCoreConfigOrDefault() config.CoreConfig {
	cfg, err := config.LoadCoreConfig()
	if err != nil {
		return config.DefaultCoreConfig()
	}
	return cfg
}
