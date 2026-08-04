package main

// ListHosts lista os hosts de ~/.ssh/config (e Includes recursivos).
func (a *App) ListHosts() ([]HostDTO, error) {
	hosts, err := a.configSvc.ListHosts()
	if err != nil {
		return nil, mapErr(err)
	}
	return hostsToDTO(hosts), nil
}

// AddHost cria um novo bloco Host no arquivo principal. Não é uma ação
// destrutiva — o core recusa se já existir um bloco com os mesmos
// Patterns, nunca sobrescreve — por isso não exige confirmação no
// frontend antes de chamar (diferente de ReplaceHost/RemoveHost).
func (a *App) AddHost(spec HostSpecInput) error {
	return mapErr(a.configSvc.AddHost("", spec.toCore()))
}

// ReplaceHost substitui um bloco Host existente por inteiro. Ação
// destrutiva (descarta as diretivas atuais do bloco) — a confirmação
// explícita é responsabilidade do frontend (ConfirmDialog), nunca desta
// camada.
func (a *App) ReplaceHost(in ReplaceHostInput) error {
	return mapErr(a.configSvc.ReplaceHost(in.SourceFile, in.OldPatterns, in.NewSpec.toCore()))
}

// RemoveHost remove um bloco Host por inteiro. Ação destrutiva — mesma
// regra de confirmação de ReplaceHost.
func (a *App) RemoveHost(in RemoveHostInput) error {
	return mapErr(a.configSvc.RemoveHost(in.SourceFile, in.Patterns))
}
