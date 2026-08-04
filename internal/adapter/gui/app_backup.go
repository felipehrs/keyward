package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/felipehrs/keyward/core"
)

// Export grava um pacote .tar.gz com os hosts e chaves selecionados. Ação
// destrutiva quando IncludePrivate=true em alguma chave (a spec exige
// aviso de alto contraste antes) — a confirmação é responsabilidade do
// frontend, nunca desta camada.
func (a *App) Export(in ExportInput) error {
	return mapErr(a.backupSvc.Export(in.DestPath, in.toCore()))
}

// ChooseExportDestination abre o diálogo nativo de salvar arquivo do SO.
// Legítimo aqui — dentro de internal/adapter/gui — por ser interação de
// SO específica da GUI, não lógica de negócio.
func (a *App) ChooseExportDestination() (string, error) {
	return application.Get().Dialog.SaveFile().
		SetFilename("keyward-backup.tar.gz").
		AddFilter("Backup keyward (*.tar.gz)", "*.tar.gz").
		PromptForSingleSelection()
}

// PreviewImport lê srcPath e retorna tudo que Import faria, sem escrever
// nada — inclusive a resolução de destino de cada host e a detecção dos
// conflitos possíveis.
func (a *App) PreviewImport(srcPath string) (ImportPreviewDTO, error) {
	preview, err := a.backupSvc.PreviewImport(srcPath)
	if err != nil {
		return ImportPreviewDTO{}, mapErr(err)
	}
	return importPreviewToDTO(preview), nil
}

// ChooseImportSource abre o diálogo nativo de abrir arquivo do SO.
func (a *App) ChooseImportSource() (string, error) {
	return application.Get().Dialog.OpenFile().
		AddFilter("Backup keyward (*.tar.gz)", "*.tar.gz").
		PromptForSingleSelection()
}

// HostImportKey calcula a chave estável de um host sem conflito, usada
// pro frontend fazer cherry-pick (marcar Skip mesmo sem conflito) em
// ImportResolutionsInput.Hosts — mesma chave que o core usa internamente
// pra hosts em ImportPreviewDTO.HostsToAdd/HostsUnchanged.
func (a *App) HostImportKey(sourceFile string, patterns []string) string {
	return core.HostImportKey(sourceFile, patterns)
}

// Import aplica o pacote usando resolutions pra decidir cada conflito.
// Contrato especial: NUNCA deixa o erro agregado do core virar rejeição de
// Promise — result é sempre retornado (mesmo parcial, se houve falhas
// isoladas por item), com o erro agregado movido pra
// ImportResultDTO.Error. Só erro de tradução da entrada (resolutions
// malformada) rejeita normalmente, já que não vem do core.
func (a *App) Import(srcPath string, resolutions ImportResolutionsInput) (ImportResultDTO, error) {
	coreRes, err := resolutions.toCore()
	if err != nil {
		return ImportResultDTO{}, err
	}
	result, err := a.backupSvc.Import(srcPath, coreRes)
	dto := importResultToDTO(result)
	if err != nil {
		msg := err.Error()
		dto.Error = &msg
	}
	return dto, nil
}
