package main

import "github.com/felipehrs/keyward/core"

// App é o serviço Wails bound ao frontend — traduz entre o core e os DTOs
// da GUI (ver dto.go e demais dto_*.go). Não contém lógica de negócio
// própria, só tradução (spec seção 3.3). Ações destrutivas são executadas
// incondicionalmente quando chamadas — a confirmação explícita é
// responsabilidade do frontend (ConfirmDialog), nunca desta camada.
type App struct {
	configSvc core.ConfigService
	keySvc    core.KeyService
	backupSvc core.BackupService
}

func newApp(configSvc core.ConfigService, keySvc core.KeyService, backupSvc core.BackupService) *App {
	return &App{configSvc: configSvc, keySvc: keySvc, backupSvc: backupSvc}
}
