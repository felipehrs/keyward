package main

import (
	"fmt"

	"github.com/felipehrs/keyward/core"
)

// KeySelectionInput espelha core.KeySelection.
type KeySelectionInput struct {
	Fingerprint    string `json:"fingerprint"`
	IncludePrivate bool   `json:"includePrivate"`
}

// ExportInput é a entrada de Export. Hosts vem completo (não só
// patterns/fingerprints) porque o frontend já tem a lista de ListHosts —
// evita reconsultar o core só pra montar a seleção.
type ExportInput struct {
	DestPath        string              `json:"destPath"`
	Hosts           []HostDTO           `json:"hosts"`
	Keys            []KeySelectionInput `json:"keys"`
	IncludeSettings bool                `json:"includeSettings"`
}

func (in ExportInput) toCore() core.ExportOptions {
	hosts := make([]core.Host, len(in.Hosts))
	for i, h := range in.Hosts {
		hosts[i] = h.toCore()
	}
	keys := make([]core.KeySelection, len(in.Keys))
	for i, k := range in.Keys {
		keys[i] = core.KeySelection{Fingerprint: k.Fingerprint, IncludePrivate: k.IncludePrivate}
	}
	return core.ExportOptions{Hosts: hosts, Keys: keys, IncludeSettings: in.IncludeSettings}
}

// HostConflictDTO espelha core.HostConflict.
type HostConflictDTO struct {
	Key             string   `json:"key"`
	Host            HostDTO  `json:"host"`
	DestinationFile string   `json:"destinationFile"`
	ExternalPath    bool     `json:"externalPath"`
	Existing        *HostDTO `json:"existing,omitempty"`
}

func hostConflictToDTO(c core.HostConflict) HostConflictDTO {
	dto := HostConflictDTO{
		Key:             c.Key,
		Host:            hostToDTO(c.Host),
		DestinationFile: c.DestinationFile,
		ExternalPath:    c.ExternalPath,
	}
	if c.Existing != nil {
		existing := hostToDTO(*c.Existing)
		dto.Existing = &existing
	}
	return dto
}

// KeyConflictKindDTO traduz core.KeyConflictKind (int/iota) pra string
// nomeada — mesma razão de KeyStatusDTO.
type KeyConflictKindDTO string

const (
	KeyConflictAlreadyRegistered KeyConflictKindDTO = "alreadyRegistered"
	KeyConflictFileNameCollision KeyConflictKindDTO = "fileNameCollision"
)

func keyConflictKindToDTO(k core.KeyConflictKind) KeyConflictKindDTO {
	if k == core.KeyConflictFileNameCollision {
		return KeyConflictFileNameCollision
	}
	return KeyConflictAlreadyRegistered
}

// KeyConflictDTO espelha core.KeyConflict.
type KeyConflictDTO struct {
	Key                 string             `json:"key"`
	Kind                KeyConflictKindDTO `json:"kind"`
	Metadata            KeyMetadataDTO     `json:"metadata"`
	HasPrivateKey       bool               `json:"hasPrivateKey"`
	DestinationPath     string             `json:"destinationPath"`
	ExistingFingerprint string             `json:"existingFingerprint,omitempty"`
}

func keyConflictToDTO(c core.KeyConflict) KeyConflictDTO {
	return KeyConflictDTO{
		Key:                 c.Key,
		Kind:                keyConflictKindToDTO(c.Kind),
		Metadata:            keyMetadataToDTO(c.Metadata),
		HasPrivateKey:       c.HasPrivateKey,
		DestinationPath:     c.DestinationPath,
		ExistingFingerprint: c.ExistingFingerprint,
	}
}

// ImportPreviewDTO espelha core.ImportPreview.
type ImportPreviewDTO struct {
	ManifestVersion int               `json:"manifestVersion"`
	CreatedAt       string            `json:"createdAt"`
	HostsToAdd      []HostDTO         `json:"hostsToAdd"`
	HostsUnchanged  []HostDTO         `json:"hostsUnchanged"`
	HostConflicts   []HostConflictDTO `json:"hostConflicts"`
	KeysToAdd       []KeyMetadataDTO  `json:"keysToAdd"`
	KeysUnchanged   []KeyMetadataDTO  `json:"keysUnchanged"`
	KeyConflicts    []KeyConflictDTO  `json:"keyConflicts"`
	Settings        *AppSettingsDTO   `json:"settings,omitempty"`
}

func keyMetadatasToDTO(ms []core.KeyMetadata) []KeyMetadataDTO {
	out := make([]KeyMetadataDTO, len(ms))
	for i, m := range ms {
		out[i] = keyMetadataToDTO(m)
	}
	return out
}

func importPreviewToDTO(p core.ImportPreview) ImportPreviewDTO {
	dto := ImportPreviewDTO{
		ManifestVersion: p.ManifestVersion,
		HostsToAdd:      hostsToDTO(p.HostsToAdd),
		HostsUnchanged:  hostsToDTO(p.HostsUnchanged),
		KeysToAdd:       keyMetadatasToDTO(p.KeysToAdd),
		KeysUnchanged:   keyMetadatasToDTO(p.KeysUnchanged),
	}
	if formatted := formatTime(&p.CreatedAt); formatted != nil {
		dto.CreatedAt = *formatted
	}
	dto.HostConflicts = make([]HostConflictDTO, len(p.HostConflicts))
	for i, c := range p.HostConflicts {
		dto.HostConflicts[i] = hostConflictToDTO(c)
	}
	dto.KeyConflicts = make([]KeyConflictDTO, len(p.KeyConflicts))
	for i, c := range p.KeyConflicts {
		dto.KeyConflicts[i] = keyConflictToDTO(c)
	}
	if p.Settings != nil {
		settings := appSettingsToDTO(*p.Settings)
		dto.Settings = &settings
	}
	return dto
}

// ImportResolutionsInput é a entrada de Import. Os valores dos mapas são
// strings nomeadas (não int) — mesma armadilha #3 de enum baseado em
// iota. Chave de host sem conflito: core.HostImportKey(sourceFile,
// patterns) (exposto via App.HostImportKey). Chave de host em conflito:
// HostConflictDTO.Key. Chave de qualquer chave SSH: sempre o Fingerprint.
type ImportResolutionsInput struct {
	Hosts map[string]string `json:"hosts"` // "skip" | "apply"
	Keys  map[string]string `json:"keys"`  // "skip" | "overwrite" | "importRenamed"
}

func (in ImportResolutionsInput) toCore() (core.ImportResolutions, error) {
	hosts := make(map[string]core.HostResolution, len(in.Hosts))
	for k, v := range in.Hosts {
		switch v {
		case "", "skip":
			hosts[k] = core.HostResolutionSkip
		case "apply":
			hosts[k] = core.HostResolutionApply
		default:
			return core.ImportResolutions{}, fmt.Errorf("resolução de host inválida: %q", v)
		}
	}
	keys := make(map[string]core.KeyResolution, len(in.Keys))
	for k, v := range in.Keys {
		switch v {
		case "", "skip":
			keys[k] = core.KeyResolutionSkip
		case "overwrite":
			keys[k] = core.KeyResolutionOverwrite
		case "importRenamed":
			keys[k] = core.KeyResolutionImportRenamed
		default:
			return core.ImportResolutions{}, fmt.Errorf("resolução de chave inválida: %q", v)
		}
	}
	return core.ImportResolutions{Hosts: hosts, Keys: keys}, nil
}

// ImportResultDTO espelha core.ImportResult. Error carrega o erro
// agregado que core.BackupService.Import retornaria — App.Import nunca
// deixa esse erro virar rejeição de Promise, pra não descartar o
// resultado parcial (ver app_backup.go).
type ImportResultDTO struct {
	HostsAdded          []HostDTO        `json:"hostsAdded"`
	HostsReplaced       []HostDTO        `json:"hostsReplaced"`
	HostsUnchanged      []HostDTO        `json:"hostsUnchanged"`
	HostsSkipped        []HostDTO        `json:"hostsSkipped"`
	KeysAdded           []KeyMetadataDTO `json:"keysAdded"`
	KeysUnchanged       []KeyMetadataDTO `json:"keysUnchanged"`
	KeysMetadataUpdated []KeyMetadataDTO `json:"keysMetadataUpdated"`
	KeysFileOverwritten []KeyMetadataDTO `json:"keysFileOverwritten"`
	KeysImportedRenamed []KeyMetadataDTO `json:"keysImportedRenamed"`
	KeysSkipped         []KeyMetadataDTO `json:"keysSkipped"`
	SettingsApplied     bool             `json:"settingsApplied"`
	Errors              []string         `json:"errors"`
	Error               *string          `json:"error,omitempty"`
}

func importResultToDTO(r core.ImportResult) ImportResultDTO {
	errs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		errs[i] = e.Error()
	}
	return ImportResultDTO{
		HostsAdded:          hostsToDTO(r.HostsAdded),
		HostsReplaced:       hostsToDTO(r.HostsReplaced),
		HostsUnchanged:      hostsToDTO(r.HostsUnchanged),
		HostsSkipped:        hostsToDTO(r.HostsSkipped),
		KeysAdded:           keyMetadatasToDTO(r.KeysAdded),
		KeysUnchanged:       keyMetadatasToDTO(r.KeysUnchanged),
		KeysMetadataUpdated: keyMetadatasToDTO(r.KeysMetadataUpdated),
		KeysFileOverwritten: keyMetadatasToDTO(r.KeysFileOverwritten),
		KeysImportedRenamed: keyMetadatasToDTO(r.KeysImportedRenamed),
		KeysSkipped:         keyMetadatasToDTO(r.KeysSkipped),
		SettingsApplied:     r.SettingsApplied,
		Errors:              errs,
	}
}
