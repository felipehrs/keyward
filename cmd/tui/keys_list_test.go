package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

func TestKeysListModel_Init_LoadsKeys(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	if _, err := keySvc.GenerateKey(core.GenerateKeyOptions{Label: "minha chave"}); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := newKeysListModel(keySvc)
	results := resolveMsgs(startAsyncCmdFor(t, m.Init()))
	loaded, ok := findKeysLoaded(results)
	if !ok {
		t.Fatal("esperava keysLoadedMsg")
	}
	if loaded.err != nil {
		t.Fatalf("keysLoadedMsg.err: %v", loaded.err)
	}
	if len(loaded.keys) != 1 || loaded.keys[0].Metadata.Label != "minha chave" {
		t.Fatalf("esperava 1 chave 'minha chave', obteve %+v", loaded.keys)
	}

	m.Update(loaded)
	if len(m.list.Items()) != 1 {
		t.Fatalf("esperava 1 item na lista, obteve %d", len(m.list.Items()))
	}
}

func TestKeyItem_Title_HighlightsExpiredAndExpiringSoon(t *testing.T) {
	expired := keyItem{key: core.Key{Metadata: core.KeyMetadata{Label: "a"}, IsExpired: true}}
	if got := expired.Title(); got == "a" {
		t.Fatalf("título de chave expirada deveria conter destaque, obteve %q", got)
	}

	expiring := keyItem{key: core.Key{Metadata: core.KeyMetadata{Label: "b"}, IsExpiringSoon: true}}
	if got := expiring.Title(); got == "b" {
		t.Fatalf("título de chave expirando deveria conter destaque, obteve %q", got)
	}

	ok := keyItem{key: core.Key{Metadata: core.KeyMetadata{Label: "c"}}}
	if got := ok.Title(); got != "c" {
		t.Fatalf("título de chave OK não deveria ter sufixo, obteve %q", got)
	}
}

func TestKeyItem_Description_IncludesFileName(t *testing.T) {
	withPrivate := keyItem{key: core.Key{PrivateKeyPath: "/home/user/.ssh/id_ed25519"}}
	if got := withPrivate.Description(); !strings.Contains(got, "id_ed25519") {
		t.Fatalf("esperava nome do arquivo 'id_ed25519' na descrição, obteve %q", got)
	}

	// KeyStatusUnregistered não tem Metadata.Fingerprint (limitação do
	// core) — o nome do arquivo é o único identificador legível disponível
	// nesse caso, então precisa aparecer mesmo sem fingerprint.
	unregistered := keyItem{key: core.Key{
		Status:         core.KeyStatusUnregistered,
		PrivateKeyPath: "/home/user/.ssh/id_rsa_backup",
	}}
	if got := unregistered.Description(); !strings.Contains(got, "id_rsa_backup") {
		t.Fatalf("esperava nome do arquivo pra chave sem registro, obteve %q", got)
	}

	onlyPublic := keyItem{key: core.Key{PublicKeyPath: "/home/user/.ssh/id_ed25519.pub"}}
	if got := onlyPublic.Description(); !strings.Contains(got, "id_ed25519.pub") {
		t.Fatalf("esperava fallback pro nome da chave pública, obteve %q", got)
	}

	empty := keyItem{key: core.Key{}}
	if got := empty.Description(); !strings.Contains(got, "-") {
		t.Fatalf("esperava '-' quando não há nenhum path, obteve %q", got)
	}
}

func TestKeysListModel_Esc_PopsScreen(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newKeysListModel(keySvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esperava popScreenMsg")
	}
	if _, ok := cmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg")
	}
}

func TestKeysListModel_Enter_PushesDetailForRegisteredKey(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	if _, err := keySvc.GenerateKey(core.GenerateKeyOptions{Label: "minha chave"}); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	m := newKeysListModel(keySvc)
	loaded, _ := findKeysLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	if _, ok := push.screen.(*keyDetailModel); !ok {
		t.Fatalf("esperava *keyDetailModel, obteve %T", push.screen)
	}
}

func TestKeysListModel_Enter_UnregisteredKey_ShowsErrInsteadOfDetail(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	generated, err := keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := keySvc.Unregister(generated.Metadata.Fingerprint); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	m := newKeysListModel(keySvc)
	loaded, _ := findKeysLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava um tea.Cmd")
	}
	if _, ok := cmd().(errMsg); !ok {
		t.Fatal("esperava errMsg pra chave sem registro de metadata")
	}
}

func TestKeysListModel_RegisterOrphan_PushesRegisterForm(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	generated, err := keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := keySvc.Unregister(generated.Metadata.Fingerprint); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	m := newKeysListModel(keySvc)
	loaded, _ := findKeysLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	form, ok := push.screen.(*keyRegisterFormModel)
	if !ok {
		t.Fatalf("esperava *keyRegisterFormModel, obteve %T", push.screen)
	}
	if form.privateKeyPath != generated.PrivateKeyPath {
		t.Fatalf("esperava privateKeyPath %q, obteve %q", generated.PrivateKeyPath, form.privateKeyPath)
	}
}

func TestKeysListModel_Settings_PushesSettingsScreen(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newKeysListModel(keySvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	if _, ok := push.screen.(*settingsModel); !ok {
		t.Fatalf("esperava *settingsModel, obteve %T", push.screen)
	}
}

func findKeysLoaded(msgs []tea.Msg) (keysLoadedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(keysLoadedMsg); ok {
			return m, true
		}
	}
	return keysLoadedMsg{}, false
}
