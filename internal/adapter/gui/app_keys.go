package main

// ListKeys lista as chaves reconciliadas entre disco e metadata.json,
// ordenadas por proximidade de expiração (mesma regra do core).
func (a *App) ListKeys() ([]KeyDTO, error) {
	keys, err := a.keySvc.ListKeys()
	if err != nil {
		return nil, mapErr(err)
	}
	return keysToDTO(keys), nil
}

// GetKey retorna uma única chave pelo fingerprint. Erro
// "[key_not_found] ..." se não houver chave em disco nem registro de
// metadata para ele (ver mapErr).
func (a *App) GetKey(fingerprint string) (KeyDTO, error) {
	key, err := a.keySvc.GetKey(fingerprint)
	if err != nil {
		return KeyDTO{}, mapErr(err)
	}
	return keyToDTO(key), nil
}

// GenerateKey gera um novo par de chaves. Se já existir um arquivo com o
// nome de destino e Overwrite==false, o core recusa com uma mensagem de
// texto (sem sentinela) contendo "já existe (use Overwrite" — o frontend
// detecta essa substring (mesma checagem que cmd/tui/key_form.go faz do
// lado da TUI) e oferece confirmar de novo com overwrite=true.
func (a *App) GenerateKey(in GenerateKeyInput) (KeyDTO, error) {
	opts, err := in.toCore()
	if err != nil {
		return KeyDTO{}, err
	}
	key, err := a.keySvc.GenerateKey(opts)
	if err != nil {
		return KeyDTO{}, mapErr(err)
	}
	return keyToDTO(key), nil
}

// RegisterKey adota uma chave encontrada em disco sem registro de
// metadata (KeyStatusUnregistered), a partir do caminho da chave privada.
func (a *App) RegisterKey(in RegisterKeyInput) (KeyDTO, error) {
	patch, err := in.toCorePatch()
	if err != nil {
		return KeyDTO{}, err
	}
	key, err := a.keySvc.RegisterKey(in.PrivateKeyPath, patch)
	if err != nil {
		return KeyDTO{}, mapErr(err)
	}
	return keyToDTO(key), nil
}

// UnregisterKey remove o registro de metadata de uma chave — nunca toca
// arquivos em disco. Ação destrutiva; a confirmação explícita é
// responsabilidade do frontend.
func (a *App) UnregisterKey(fingerprint string) error {
	return mapErr(a.keySvc.Unregister(fingerprint))
}

// UpdateKeyMetadata edita label/notes/expiresAt de uma chave já
// registrada. Erro "[key_not_found] ..." se o fingerprint não tiver
// registro de metadata (mesmo que exista em disco).
func (a *App) UpdateKeyMetadata(fingerprint string, patch KeyMetadataPatchInput) (KeyDTO, error) {
	corePatch, err := patch.toCore()
	if err != nil {
		return KeyDTO{}, err
	}
	key, err := a.keySvc.UpdateMetadata(fingerprint, corePatch)
	if err != nil {
		return KeyDTO{}, mapErr(err)
	}
	return keyToDTO(key), nil
}
