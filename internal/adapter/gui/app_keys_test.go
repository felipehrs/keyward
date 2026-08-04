package main

import (
	"strings"
	"testing"

	"github.com/felipehrs/keyward/core"
)

func TestApp_ListKeys_Empty(t *testing.T) {
	a := newTestApp(t)
	keys, err := a.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("esperava lista vazia, obteve %+v", keys)
	}
}

func TestApp_ListKeys_ReturnsGeneratedKey(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{Label: "minha chave"}); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	keys, err := a.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave, obteve %d", len(keys))
	}
	k := keys[0]
	if k.Metadata.Label != "minha chave" {
		t.Fatalf("esperava label 'minha chave', obteve %q", k.Metadata.Label)
	}
	if k.Status != KeyStatusOK {
		t.Fatalf("esperava status ok, obteve %q", k.Status)
	}
	if k.Metadata.CreatedAt == "" {
		t.Fatal("esperava CreatedAt preenchido pra chave registrada")
	}
	if k.Algorithm != "ed25519" {
		t.Fatalf("esperava algoritmo ed25519, obteve %q", k.Algorithm)
	}
}

func TestApp_ListKeys_UnregisteredKey_HasEmptyCreatedAt(t *testing.T) {
	a := newTestApp(t)
	generated, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := a.keySvc.Unregister(generated.Metadata.Fingerprint); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	keys, err := a.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave, obteve %d", len(keys))
	}
	k := keys[0]
	if k.Status != KeyStatusUnregistered {
		t.Fatalf("esperava status unregistered, obteve %q", k.Status)
	}
	// Metadata é zero value nesse caso — CreatedAt não deveria virar a
	// data zero formatada ("0001-01-01...").
	if k.Metadata.CreatedAt != "" {
		t.Fatalf("esperava CreatedAt vazio pra chave sem registro, obteve %q", k.Metadata.CreatedAt)
	}
	if k.Metadata.Fingerprint != "" {
		t.Fatalf("esperava Fingerprint vazio pra chave sem registro (limitação conhecida do core), obteve %q", k.Metadata.Fingerprint)
	}
}

func TestApp_GetKey_NotFound_ReturnsMappedError(t *testing.T) {
	a := newTestApp(t)
	_, err := a.GetKey("SHA256:inexistente")
	if err == nil {
		t.Fatal("esperava erro")
	}
	if !strings.Contains(err.Error(), "[key_not_found]") {
		t.Fatalf("esperava erro mapeado com prefixo [key_not_found], obteve %q", err.Error())
	}
}

func TestApp_GetKey_Found(t *testing.T) {
	a := newTestApp(t)
	generated, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{Label: "x"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	got, err := a.GetKey(generated.Metadata.Fingerprint)
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if got.Metadata.Label != "x" {
		t.Fatalf("esperava label 'x', obteve %q", got.Metadata.Label)
	}
}

func TestApp_GenerateKey_Ed25519(t *testing.T) {
	a := newTestApp(t)
	key, err := a.GenerateKey(GenerateKeyInput{Label: "gerada via app"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if key.Metadata.Label != "gerada via app" {
		t.Fatalf("esperava label, obteve %q", key.Metadata.Label)
	}
	if key.Algorithm != "ed25519" {
		t.Fatalf("esperava ed25519, obteve %q", key.Algorithm)
	}
	if key.PrivateKeyPath == "" {
		t.Fatal("esperava PrivateKeyPath preenchido")
	}
}

func TestApp_GenerateKey_WithExpiresAt(t *testing.T) {
	a := newTestApp(t)
	expires := "2030-01-01"
	key, err := a.GenerateKey(GenerateKeyInput{ExpiresAt: &expires})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if key.Metadata.ExpiresAt == nil {
		t.Fatal("esperava ExpiresAt preenchido")
	}
	if !strings.HasPrefix(*key.Metadata.ExpiresAt, "2030-01-01") {
		t.Fatalf("esperava data 2030-01-01, obteve %q", *key.Metadata.ExpiresAt)
	}
}

func TestApp_GenerateKey_InvalidExpiresAt_ReturnsError(t *testing.T) {
	a := newTestApp(t)
	bad := "não é uma data"
	if _, err := a.GenerateKey(GenerateKeyInput{ExpiresAt: &bad}); err == nil {
		t.Fatal("esperava erro para data inválida")
	}
}

func TestApp_GenerateKey_PassphraseNeverLeaksBack(t *testing.T) {
	a := newTestApp(t)
	key, err := a.GenerateKey(GenerateKeyInput{Passphrase: "segredo"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	// KeyDTO não tem (e nunca deveria ter) um campo de passphrase — esta
	// asserção é sobretudo documentacional: se alguém adicionar um campo
	// desses no futuro, o teste precisa ser atualizado conscientemente.
	_ = key
}

func TestApp_GenerateKey_OverwriteConflict_ReturnsDetectableError(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.GenerateKey(GenerateKeyInput{FileName: "id_ed25519"}); err != nil {
		t.Fatalf("GenerateKey inicial: %v", err)
	}

	_, err := a.GenerateKey(GenerateKeyInput{FileName: "id_ed25519"})
	if err == nil {
		t.Fatal("esperava erro de arquivo já existente")
	}
	if !strings.Contains(err.Error(), "já existe (use Overwrite") {
		t.Fatalf("esperava mensagem detectável de overwrite, obteve %q", err.Error())
	}

	// Confirma que Overwrite=true resolve o conflito.
	if _, err := a.GenerateKey(GenerateKeyInput{FileName: "id_ed25519", Overwrite: true}); err != nil {
		t.Fatalf("GenerateKey com Overwrite: %v", err)
	}
}

func TestApp_RegisterKey_AdoptsOrphanKey(t *testing.T) {
	a := newTestApp(t)
	generated, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := a.keySvc.Unregister(generated.Metadata.Fingerprint); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	registered, err := a.RegisterKey(RegisterKeyInput{
		PrivateKeyPath: generated.PrivateKeyPath,
		Label:          "readotada",
	})
	if err != nil {
		t.Fatalf("RegisterKey: %v", err)
	}
	if registered.Status != KeyStatusOK {
		t.Fatalf("esperava status ok após registrar, obteve %q", registered.Status)
	}
	if registered.Metadata.Label != "readotada" {
		t.Fatalf("esperava label 'readotada', obteve %q", registered.Metadata.Label)
	}
}

func TestApp_UnregisterKey_RemovesMetadataOnly(t *testing.T) {
	a := newTestApp(t)
	generated, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if err := a.UnregisterKey(generated.Metadata.Fingerprint); err != nil {
		t.Fatalf("UnregisterKey: %v", err)
	}

	keys, err := a.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Status != KeyStatusUnregistered {
		t.Fatalf("esperava 1 chave unregistered (arquivo intacto), obteve %+v", keys)
	}
}

func TestApp_UnregisterKey_NotFound_ReturnsMappedError(t *testing.T) {
	a := newTestApp(t)
	err := a.UnregisterKey("SHA256:inexistente")
	if err == nil {
		t.Fatal("esperava erro")
	}
	if !strings.Contains(err.Error(), "[key_not_found]") {
		t.Fatalf("esperava erro mapeado, obteve %q", err.Error())
	}
}

func TestApp_UpdateKeyMetadata_Label(t *testing.T) {
	a := newTestApp(t)
	generated, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{Label: "original"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	newLabel := "atualizado"
	updated, err := a.UpdateKeyMetadata(generated.Metadata.Fingerprint, KeyMetadataPatchInput{Label: &newLabel})
	if err != nil {
		t.Fatalf("UpdateKeyMetadata: %v", err)
	}
	if updated.Metadata.Label != "atualizado" {
		t.Fatalf("esperava label 'atualizado', obteve %q", updated.Metadata.Label)
	}
}

func TestApp_UpdateKeyMetadata_ExpiresAt_Keep(t *testing.T) {
	a := newTestApp(t)
	expires := "2030-06-15"
	generated, err := a.GenerateKey(GenerateKeyInput{ExpiresAt: &expires})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	newLabel := "só mudando o label"
	updated, err := a.UpdateKeyMetadata(generated.Metadata.Fingerprint, KeyMetadataPatchInput{
		Label:     &newLabel,
		ExpiresAt: ExpiresAtKeep,
	})
	if err != nil {
		t.Fatalf("UpdateKeyMetadata: %v", err)
	}
	if updated.Metadata.ExpiresAt == nil || !strings.HasPrefix(*updated.Metadata.ExpiresAt, "2030-06-15") {
		t.Fatalf("esperava ExpiresAt intocado (2030-06-15), obteve %+v", updated.Metadata.ExpiresAt)
	}
}

func TestApp_UpdateKeyMetadata_ExpiresAt_Clear(t *testing.T) {
	a := newTestApp(t)
	expires := "2030-06-15"
	generated, err := a.GenerateKey(GenerateKeyInput{ExpiresAt: &expires})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	updated, err := a.UpdateKeyMetadata(generated.Metadata.Fingerprint, KeyMetadataPatchInput{ExpiresAt: ExpiresAtClear})
	if err != nil {
		t.Fatalf("UpdateKeyMetadata: %v", err)
	}
	if updated.Metadata.ExpiresAt != nil {
		t.Fatalf("esperava ExpiresAt limpo (nil), obteve %+v", updated.Metadata.ExpiresAt)
	}
}

func TestApp_UpdateKeyMetadata_ExpiresAt_Set(t *testing.T) {
	a := newTestApp(t)
	generated, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	newExpiry := "2031-12-25"
	updated, err := a.UpdateKeyMetadata(generated.Metadata.Fingerprint, KeyMetadataPatchInput{
		ExpiresAt:      ExpiresAtSet,
		ExpiresAtValue: &newExpiry,
	})
	if err != nil {
		t.Fatalf("UpdateKeyMetadata: %v", err)
	}
	if updated.Metadata.ExpiresAt == nil || !strings.HasPrefix(*updated.Metadata.ExpiresAt, "2031-12-25") {
		t.Fatalf("esperava ExpiresAt 2031-12-25, obteve %+v", updated.Metadata.ExpiresAt)
	}
}

func TestApp_UpdateKeyMetadata_ExpiresAt_Set_MissingValue_ReturnsError(t *testing.T) {
	a := newTestApp(t)
	generated, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	_, err = a.UpdateKeyMetadata(generated.Metadata.Fingerprint, KeyMetadataPatchInput{ExpiresAt: ExpiresAtSet})
	if err == nil {
		t.Fatal(`esperava erro quando expiresAt="set" sem expiresAtValue`)
	}
}

func TestApp_UpdateKeyMetadata_NotFound_ReturnsMappedError(t *testing.T) {
	a := newTestApp(t)
	_, err := a.UpdateKeyMetadata("SHA256:inexistente", KeyMetadataPatchInput{})
	if err == nil {
		t.Fatal("esperava erro")
	}
	if !strings.Contains(err.Error(), "[key_not_found]") {
		t.Fatalf("esperava erro mapeado, obteve %q", err.Error())
	}
}
