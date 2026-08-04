package main

// GetSettings retorna a configuração atual do app (com defaults se
// metadata.json ainda não existir).
func (a *App) GetSettings() (AppSettingsDTO, error) {
	settings, err := a.keySvc.Settings()
	if err != nil {
		return AppSettingsDTO{}, mapErr(err)
	}
	return appSettingsToDTO(settings), nil
}

// UpdateSettings persiste uma nova AppSettings — afeta diretamente o
// destaque de expiração em ListKeys (AlertThresholdDays) e o algoritmo
// default de GenerateKey (DefaultAlgorithm) quando não especificado.
func (a *App) UpdateSettings(settings AppSettingsDTO) error {
	return mapErr(a.keySvc.UpdateSettings(settings.toCore()))
}
