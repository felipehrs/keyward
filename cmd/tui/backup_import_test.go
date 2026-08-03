package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// setFakeHome aponta os.UserHomeDir() (via HOME/USERPROFILE) pra home —
// necessário porque core/backup_export.go compacta Host.SourceFile de
// volta pra "~/..." usando o home REAL do processo no momento do export
// (compactHome), e core/backup_import.go expande esse marcador usando o
// home do processo no momento do import (expandHome) — sem isso, dois
// t.TempDir() diferentes nunca "combinam", e todo host vira ExternalPath.
func setFakeHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

// newHomeServices constrói os três serviços com paths sob home/.ssh e
// ajusta HOME/USERPROFILE de acordo (ver setFakeHome) — usado nos testes
// deste arquivo que precisam exercitar o caminho "feliz" de host portável
// (~/.ssh/config), não só o caminho de ExternalPath.
func newHomeServices(t *testing.T, home string) (core.ConfigService, core.KeyService, core.BackupService) {
	t.Helper()
	setFakeHome(t, home)
	sshDir := filepath.Join(home, ".ssh")
	metadataPath := filepath.Join(home, "metadata.json")
	configSvc := core.NewFileConfigService(filepath.Join(sshDir, "config"))
	keySvc := core.NewFileKeyService(sshDir, metadataPath)
	backupSvc := core.NewFileBackupService(filepath.Join(sshDir, "config"), sshDir, metadataPath)
	return configSvc, keySvc, backupSvc
}

// buildExportedPackage cria, sob originHome, um pacote de export real (via
// BackupService.Export) contendo um host e uma chave (com material
// privado), pra usar como entrada de PreviewImport/Import nos testes deste
// arquivo. O caminho de destino do pacote fica fora de originHome (um
// t.TempDir() à parte), pra não ser confundido com o conteúdo do "home".
func buildExportedPackage(t *testing.T, originHome string) (dest string) {
	t.Helper()
	configSvc, keySvc, backupSvc := newHomeServices(t, originHome)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	generated, err := keySvc.GenerateKey(core.GenerateKeyOptions{Label: "chave export"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	hosts, err := configSvc.ListHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("ListHosts: %v %+v", err, hosts)
	}

	dest = filepath.Join(t.TempDir(), "export.tar.gz")
	opts := core.ExportOptions{
		Hosts: hosts, // SourceFile precisa vir de ListHosts (não construído à mão) — Export valida o path
		Keys:  []core.KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: true}},
	}
	if err := backupSvc.Export(dest, opts); err != nil {
		t.Fatalf("Export: %v", err)
	}
	return dest
}

func TestBackupImportModel_Path_EmptyBlocksSubmit(t *testing.T) {
	_, _, backupSvc := newTestServices(t)
	m := newBackupImportModel(backupSvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("não deveria disparar preview sem caminho de origem")
	}
	if m.validationErr == nil {
		t.Fatal("esperava validationErr definido")
	}
}

func TestBackupImportModel_Path_ValidPath_LoadsPreview(t *testing.T) {
	src := buildExportedPackage(t, t.TempDir())
	_, _, backupSvc := newHomeServices(t, t.TempDir())
	m := newBackupImportModel(backupSvc)
	m.srcPath.SetValue(src)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de PreviewImport")
	}
	results := resolveMsgs(startAsyncCmdFor(t, cmd))
	preview, ok := findBackupImportPreview(results)
	if !ok {
		t.Fatal("esperava backupImportPreviewMsg")
	}
	if preview.err != nil {
		t.Fatalf("PreviewImport: %v", preview.err)
	}
	if len(preview.preview.HostsToAdd) != 1 || len(preview.preview.KeysToAdd) != 1 {
		t.Fatalf("esperava 1 host e 1 chave a adicionar, obteve %+v", preview.preview)
	}

	m.Update(preview)
	if m.phase != importPhasePreview {
		t.Fatalf("esperava fase preview, obteve %d", m.phase)
	}
	if len(m.rows) != 2 { // 1 host to-add + 1 key to-add, sem settings no pacote
		t.Fatalf("esperava 2 linhas, obteve %d", len(m.rows))
	}
}

// setupPreviewModel importa o pacote de buildExportedPackage duas vezes no
// MESMO destino local (a primeira import "instala" host+chave; a segunda
// preview detecta os três tipos de conflito: host já existe com o mesmo
// conteúdo (unchanged, não conflito) — pra gerar um HostConflict de
// verdade, o teste re-exporta com HostName diferente antes do segundo
// import).
func setupConflictPreview(t *testing.T) (*backupImportModel, core.KeyService, core.ConfigService) {
	t.Helper()
	src := buildExportedPackage(t, t.TempDir())

	configSvc, keySvc, backupSvc := newHomeServices(t, t.TempDir())
	// Já existe localmente: host com Patterns iguais mas HostName diferente
	// (gera HostConflict), e uma chave com o MESMO fingerprint do pacote
	// não é possível replicar sem reimportar — então o conflito de chave
	// testado aqui é via colisão de nome de arquivo (outra chave, mesmo
	// nome de arquivo).
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "9.9.9.9"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	// Cria uma chave local com o mesmo nome de arquivo que o pacote usa
	// (id_ed25519, default), mas fingerprint diferente -> colisão de nome.
	if _, err := keySvc.GenerateKey(core.GenerateKeyOptions{FileName: "id_ed25519"}); err != nil {
		t.Fatalf("GenerateKey local: %v", err)
	}

	m := newBackupImportModel(backupSvc)
	m.srcPath.SetValue(src)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	preview, ok := findBackupImportPreview(resolveMsgs(startAsyncCmdFor(t, cmd)))
	if !ok {
		t.Fatalf("esperava backupImportPreviewMsg")
	}
	if preview.err != nil {
		t.Fatalf("PreviewImport: %v", preview.err)
	}
	m.Update(preview)
	return m, keySvc, configSvc
}

func TestBackupImportModel_Preview_DetectsHostAndKeyConflicts(t *testing.T) {
	m, _, _ := setupConflictPreview(t)

	var hasHostConflict, hasKeyFileCollision bool
	for _, r := range m.rows {
		if r.kind == rowHostConflict {
			hasHostConflict = true
		}
		if r.kind == rowKeyConflict && r.keyConflict.Kind == core.KeyConflictFileNameCollision {
			hasKeyFileCollision = true
		}
	}
	if !hasHostConflict {
		t.Fatal("esperava um HostConflict detectado")
	}
	if !hasKeyFileCollision {
		t.Fatal("esperava um KeyConflict de colisão de nome de arquivo detectado")
	}
}

func TestBackupImportModel_CycleCurrent_HostConflict_TogglesSkipApply(t *testing.T) {
	m, _, _ := setupConflictPreview(t)
	idx := indexOfRowKind(t, m.rows, rowHostConflict)
	m.cursor = idx

	if m.rows[idx].hostRes != core.HostResolutionSkip {
		t.Fatalf("default esperado Skip, obteve %v", m.rows[idx].hostRes)
	}
	m.cycleCurrent()
	if m.rows[idx].hostRes != core.HostResolutionApply {
		t.Fatalf("esperava Apply após ciclar, obteve %v", m.rows[idx].hostRes)
	}
	m.cycleCurrent()
	if m.rows[idx].hostRes != core.HostResolutionSkip {
		t.Fatalf("esperava voltar a Skip, obteve %v", m.rows[idx].hostRes)
	}
}

func TestBackupImportModel_CycleCurrent_KeyFileCollision_CyclesThreeStates(t *testing.T) {
	m, _, _ := setupConflictPreview(t)
	idx := indexOfRowKind(t, m.rows, rowKeyConflict)
	m.cursor = idx

	seq := []core.KeyResolution{}
	for range 4 {
		seq = append(seq, m.rows[idx].keyRes)
		m.cycleCurrent()
	}
	want := []core.KeyResolution{
		core.KeyResolutionSkip, core.KeyResolutionOverwrite, core.KeyResolutionImportRenamed, core.KeyResolutionSkip,
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("passo %d: esperava %v, obteve %v (sequência completa: %v)", i, want[i], seq[i], seq)
		}
	}
}

func TestBackupImportModel_CherryPick_HostToAdd_UsesHostImportKey(t *testing.T) {
	src := buildExportedPackage(t, t.TempDir())
	_, _, backupSvc := newHomeServices(t, t.TempDir())
	m := newBackupImportModel(backupSvc)
	m.srcPath.SetValue(src)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	preview, _ := findBackupImportPreview(resolveMsgs(startAsyncCmdFor(t, cmd)))
	m.Update(preview)

	idx := indexOfRowKind(t, m.rows, rowHostToAdd)
	m.cursor = idx
	m.cycleCurrent() // marca Skip (cherry-pick)

	res := m.resolutions()
	expectedKey := core.HostImportKey(m.rows[idx].host.SourceFile, m.rows[idx].host.Patterns)
	if got, ok := res.Hosts[expectedKey]; !ok || got != core.HostResolutionSkip {
		t.Fatalf("esperava Hosts[%q]=Skip, obteve %v (ok=%v)", expectedKey, got, ok)
	}
}

func TestBackupImportModel_RequestApply_DangerWhenSettingsOrOverwrite(t *testing.T) {
	m, _, _ := setupConflictPreview(t)

	// Sem nenhuma resolução perigosa ainda (tudo default Skip) — sem
	// settings no pacote (buildExportedPackage não inclui) — não deveria
	// ser danger.
	if m.summaryIsDanger() {
		t.Fatal("preview intocado (tudo Skip, sem settings) não deveria ser danger")
	}

	idx := indexOfRowKind(t, m.rows, rowHostConflict)
	m.rows[idx].hostRes = core.HostResolutionApply
	if !m.summaryIsDanger() {
		t.Fatal("Apply sobre HostConflict deveria tornar danger=true")
	}
}

func TestBackupImportModel_Apply_ProducesResult(t *testing.T) {
	m, _, configSvc := setupConflictPreview(t)

	hostIdx := indexOfRowKind(t, m.rows, rowHostConflict)
	m.rows[hostIdx].hostRes = core.HostResolutionApply

	cmd := m.requestApply()
	confirmMsg, ok := cmd().(requestConfirmMsg)
	if !ok {
		t.Fatalf("esperava requestConfirmMsg, obteve %T", confirmMsg)
	}
	if !confirmMsg.danger {
		t.Fatal("esperava danger=true (Apply sobre conflito de host)")
	}

	results := resolveMsgs(startAsyncCmdFor(t, confirmMsg.onConfirm))
	done, ok := findBackupImportDone(results)
	if !ok {
		t.Fatal("esperava backupImportDoneMsg")
	}
	if len(done.result.HostsReplaced) != 1 {
		t.Fatalf("esperava 1 host substituído, obteve %+v", done.result)
	}

	m.Update(done)
	if m.phase != importPhaseResult {
		t.Fatalf("esperava fase resultado, obteve %d", m.phase)
	}

	hosts, err := configSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	for _, h := range hosts {
		if h.Patterns[0] == "bastion" && h.HostName != "1.2.3.4" {
			t.Fatalf("esperava host substituído com HostName do pacote, obteve %+v", h)
		}
	}
}

func TestBackupImportModel_Esc_FromPath_PopsScreen(t *testing.T) {
	_, _, backupSvc := newTestServices(t)
	m := newBackupImportModel(backupSvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esperava popScreenMsg")
	}
	if _, ok := cmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg")
	}
}

func TestBackupImportModel_Esc_FromPreview_ReturnsToPathPhase(t *testing.T) {
	m, _, _ := setupConflictPreview(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("esc na fase de preview não deveria emitir Cmd — só troca de fase internamente")
	}
	if m.phase != importPhasePath {
		t.Fatalf("esperava voltar pra fase de path, obteve %d", m.phase)
	}
}

func indexOfRowKind(t *testing.T, rows []importRow, kind importRowKind) int {
	t.Helper()
	for i, r := range rows {
		if r.kind == kind {
			return i
		}
	}
	t.Fatalf("nenhuma linha do tipo %v encontrada", kind)
	return -1
}

func findBackupImportPreview(msgs []tea.Msg) (backupImportPreviewMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(backupImportPreviewMsg); ok {
			return m, true
		}
	}
	return backupImportPreviewMsg{}, false
}

func findBackupImportDone(msgs []tea.Msg) (backupImportDoneMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(backupImportDoneMsg); ok {
			return m, true
		}
	}
	return backupImportDoneMsg{}, false
}
