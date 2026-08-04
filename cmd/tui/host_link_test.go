package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

func registerAgentKeyForTest(t *testing.T, keySvc core.KeyService, label string) core.Key {
	t.Helper()
	fileSvc := keySvc.(*core.FileKeyService)
	added, fingerprint := genEd25519Identity(t)
	fileSvc.AgentDial = startFakeAgent(t, added)

	registered, err := keySvc.RegisterAgentKey(fingerprint, core.KeyMetadataPatch{Label: &label})
	if err != nil {
		t.Fatalf("RegisterAgentKey: %v", err)
	}
	return registered
}

func TestBuildHostLinkItems_MarksOrphanWhenHostPatternsChanged(t *testing.T) {
	link := core.HostMetadata{HostKey: hostKey([]string{"bastion"}), AgentKeyFingerprint: "SHA256:x"}

	// Host ainda existe com os mesmos Patterns: vínculo não é órfão.
	matched := buildHostLinkItems([]core.Host{{Patterns: []string{"bastion"}}}, []core.HostMetadata{link}, nil)
	if len(matched) != 1 || matched[0].(hostLinkItem).orphan {
		t.Fatalf("esperava vínculo não-órfão quando host bate, obteve %+v", matched)
	}

	// Patterns do host renomeados: hostKey não bate mais com nenhum host
	// atual — vínculo deve ficar sinalizado como órfão.
	orphaned := buildHostLinkItems([]core.Host{{Patterns: []string{"bastion-renomeado"}}}, []core.HostMetadata{link}, nil)
	if len(orphaned) != 1 || !orphaned[0].(hostLinkItem).orphan {
		t.Fatalf("esperava vínculo órfão após renomear Patterns, obteve %+v", orphaned)
	}
}

func TestBuildHostLinkItems_ResolvesKeyLabelWhenAvailable(t *testing.T) {
	link := core.HostMetadata{HostKey: hostKey([]string{"bastion"}), AgentKeyFingerprint: "SHA256:x"}
	keys := []core.Key{{Metadata: core.KeyMetadata{Fingerprint: "SHA256:x", Label: "minha chave"}}}

	items := buildHostLinkItems([]core.Host{{Patterns: []string{"bastion"}}}, []core.HostMetadata{link}, keys)
	item := items[0].(hostLinkItem)
	if item.keyLabel != "minha chave" {
		t.Fatalf("esperava keyLabel resolvido pro label da chave, obteve %q", item.keyLabel)
	}
}

func TestHostLinksModel_Reload_LoadsLinksHostsAndKeys(t *testing.T) {
	configSvc, keySvc, _ := newTestServices(t)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	registered := registerAgentKeyForTest(t, keySvc, "chave 1")
	if err := keySvc.LinkHostKey(hostKey([]string{"bastion"}), registered.Metadata.Fingerprint, ""); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}

	m := newHostLinksModel(keySvc, configSvc)
	results := resolveMsgs(startAsyncCmdFor(t, m.Init()))
	var loaded hostLinkLoadedMsg
	var found bool
	for _, msg := range results {
		if l, ok := msg.(hostLinkLoadedMsg); ok {
			loaded, found = l, true
		}
	}
	if !found {
		t.Fatal("esperava hostLinkLoadedMsg")
	}
	if loaded.err != nil {
		t.Fatalf("reload: %v", loaded.err)
	}
	if len(loaded.links) != 1 || len(loaded.hosts) != 1 || len(loaded.keys) != 1 {
		t.Fatalf("esperava 1 link/host/key, obteve links=%d hosts=%d keys=%d", len(loaded.links), len(loaded.hosts), len(loaded.keys))
	}

	m.Update(loaded)
	items := m.list.Items()
	if len(items) != 1 {
		t.Fatalf("esperava 1 item na lista, obteve %d", len(items))
	}
	if items[0].(hostLinkItem).orphan {
		t.Fatal("vínculo não deveria ser órfão logo após LinkHostKey")
	}
}

func TestHostLinksModel_Delete_RequestsUnlinkConfirmation(t *testing.T) {
	configSvc, keySvc, _ := newTestServices(t)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	registered := registerAgentKeyForTest(t, keySvc, "chave 1")
	hk := hostKey([]string{"bastion"})
	if err := keySvc.LinkHostKey(hk, registered.Metadata.Fingerprint, ""); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}

	m := newHostLinksModel(keySvc, configSvc)
	results := resolveMsgs(startAsyncCmdFor(t, m.Init()))
	for _, msg := range results {
		if loaded, ok := msg.(hostLinkLoadedMsg); ok {
			m.Update(loaded)
		}
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("esperava requestConfirmMsg")
	}
	confirmMsg, ok := cmd().(requestConfirmMsg)
	if !ok {
		t.Fatalf("remover vínculo deveria exigir confirmação, obteve %T", confirmMsg)
	}

	result := resolveMsgs(startAsyncCmdFor(t, confirmMsg.onConfirm))
	var unlinked hostUnlinkedMsg
	var found bool
	for _, msg := range result {
		if u, ok := msg.(hostUnlinkedMsg); ok {
			unlinked, found = u, true
		}
	}
	if !found {
		t.Fatal("esperava hostUnlinkedMsg após confirmar")
	}
	if unlinked.err != nil {
		t.Fatalf("UnlinkHostKey: %v", unlinked.err)
	}

	links, err := keySvc.ListHostLinks()
	if err != nil || len(links) != 0 {
		t.Fatalf("esperava 0 vínculos após remover, obteve %v %+v", err, links)
	}
}

func TestHostLinksModel_New_PushesLinkForm(t *testing.T) {
	configSvc, keySvc, _ := newTestServices(t)
	m := newHostLinksModel(keySvc, configSvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	if _, ok := push.screen.(*hostLinkFormModel); !ok {
		t.Fatalf("esperava *hostLinkFormModel, obteve %T", push.screen)
	}
}

func TestHostLinkFormModel_TwoStepWizard_LinksHostToAgentKey(t *testing.T) {
	configSvc, keySvc, _ := newTestServices(t)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	registered := registerAgentKeyForTest(t, keySvc, "chave 1")

	m := newHostLinkFormModel(keySvc, configSvc)
	results := resolveMsgs(startAsyncCmdFor(t, m.Init()))
	var loaded hostLinkFormLoadedMsg
	var found bool
	for _, msg := range results {
		if l, ok := msg.(hostLinkFormLoadedMsg); ok {
			loaded, found = l, true
		}
	}
	if !found {
		t.Fatal("esperava hostLinkFormLoadedMsg")
	}
	if loaded.err != nil {
		t.Fatalf("Init: %v", loaded.err)
	}
	m.Update(loaded)

	if m.step != 0 {
		t.Fatalf("esperava step 0 (escolher host) no início, obteve %d", m.step)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("escolher host não deveria disparar Cmd — só avança pro passo seguinte")
	}
	if m.step != 1 {
		t.Fatalf("esperava step 1 (escolher chave) após escolher host, obteve %d", m.step)
	}
	if !strings.Contains(strings.Join(m.selectedHost.Patterns, ","), "bastion") {
		t.Fatalf("esperava selectedHost 'bastion', obteve %+v", m.selectedHost)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de submissão ao escolher a chave")
	}
	linkResults := resolveMsgs(startAsyncCmdFor(t, cmd))
	var linked hostLinkedMsg
	var linkedFound bool
	for _, msg := range linkResults {
		if l, ok := msg.(hostLinkedMsg); ok {
			linked, linkedFound = l, true
		}
	}
	if !linkedFound {
		t.Fatal("esperava hostLinkedMsg")
	}
	if linked.err != nil {
		t.Fatalf("LinkHostKey: %v", linked.err)
	}

	links, err := keySvc.ListHostLinks()
	if err != nil {
		t.Fatalf("ListHostLinks: %v", err)
	}
	if len(links) != 1 || links[0].AgentKeyFingerprint != registered.Metadata.Fingerprint {
		t.Fatalf("esperava 1 vínculo pra %q, obteve %+v", registered.Metadata.Fingerprint, links)
	}

	_, popCmd := m.Update(linked)
	if popCmd == nil {
		t.Fatal("esperava popScreenMsg após vincular com sucesso")
	}
	if _, ok := popCmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg após LinkHostKey bem-sucedido")
	}
}

func TestHostLinkFormModel_EligibleAgentKeys_ExcludesUnregistered(t *testing.T) {
	registeredMeta := core.Key{Source: core.KeySourceAgent, Status: core.KeyStatusOK, Metadata: core.KeyMetadata{Fingerprint: "a"}}
	offline := core.Key{Source: core.KeySourceAgent, Status: core.KeyStatusAgentOffline, Metadata: core.KeyMetadata{Fingerprint: "b"}}
	unregistered := core.Key{Source: core.KeySourceAgent, Status: core.KeyStatusUnregistered, Metadata: core.KeyMetadata{Fingerprint: "c"}}
	fileKey := core.Key{Source: core.KeySourceFile, Status: core.KeyStatusOK, Metadata: core.KeyMetadata{Fingerprint: "d"}}

	eligible := eligibleAgentKeys([]core.Key{registeredMeta, offline, unregistered, fileKey})
	if len(eligible) != 2 {
		t.Fatalf("esperava 2 chaves elegíveis (OK + AgentOffline), obteve %d: %+v", len(eligible), eligible)
	}
}
