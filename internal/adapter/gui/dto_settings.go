package main

import "github.com/felipehrs/keyward/core"

// AppSettingsDTO espelha core.AppSettings — sem armadilhas de
// serialização.
type AppSettingsDTO struct {
	AlertThresholdDays int    `json:"alertThresholdDays"`
	DefaultAlgorithm   string `json:"defaultAlgorithm"`
}

func appSettingsToDTO(s core.AppSettings) AppSettingsDTO {
	return AppSettingsDTO{
		AlertThresholdDays: s.AlertThresholdDays,
		DefaultAlgorithm:   string(s.DefaultAlgorithm),
	}
}

func (in AppSettingsDTO) toCore() core.AppSettings {
	return core.AppSettings{
		AlertThresholdDays: in.AlertThresholdDays,
		DefaultAlgorithm:   core.Algorithm(in.DefaultAlgorithm),
	}
}
