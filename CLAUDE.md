# keyward

Ferramenta multiplataforma (Windows/Linux/macOS) para gerenciar `~/.ssh/config`, chaves SSH e
seus ciclos de vida (expiração/rotação), com CLI, TUI e GUI desktop compartilhando a mesma
lógica de negócio. Spec completa: `docs/specs/ssh-config-manager.md` (pt-BR, contém todas as
decisões arquiteturais e seus motivos — leia antes de mudanças estruturais).

## Módulo e build

- Módulo Go: `github.com/felipehrs/keyward`, Go 1.26.5.
- `make check` roda `vet build test lint` (usa `golangci-lint`). `make lint` sozinho requer
  `golangci-lint` instalado.
- `go test ./...` para rodar só os testes.

## Arquitetura (regra principal)

```
core/                      → lógica de negócio pura, só stdlib + x/crypto/ssh + kevinburke/ssh_config
cmd/cli/                   → CLI via Cobra, depende de core
cmd/tui/                   → TUI via Bubble Tea, depende de core
internal/adapter/gui/      → único pacote autorizado a importar "wails"; só tradução, sem lógica de negócio
internal/adapter/gui/frontend/ → HTML/CSS/JS, fala só com app.go via bindings do Wails
```

- Dependência é sempre de fora para dentro: `core` nunca importa `cmd/*` ou `internal/adapter/gui`.
- `core` expõe API pública como **interfaces** (`ConfigService`, `KeyService`, `BackupService`),
  não structs concretas — CLI/TUI/GUI consomem as interfaces.
- Regra de fronteira do Wails é verificada por `depguard` no `.golangci.yml` (falha o lint se
  `github.com/wailsapp/wails` for importado fora de `internal/adapter/gui/**`).
- GUI usa DTOs próprios (`internal/adapter/gui/dto.go`), nunca reexpõe structs do `core`
  diretamente ao frontend.

## Estado atual do código

`core` já implementa boa parte do MVP (parsing/escrita de `~/.ssh/config`, geração de chaves,
metadata/reconciliação, backup export/import). `cmd/cli`, `cmd/tui` e `internal/adapter/gui` são
ainda placeholders/esqueletos — a lógica de negócio deve continuar sendo construída em `core`
primeiro, com as interfaces consumindo-a depois.

## Convenções específicas deste projeto

- Escrita de arquivo é sempre atômica (arquivo temporário + `os.Rename`), via helper
  `atomicWriteFile` em `core/atomicfile.go` — reusar esse helper para qualquer escrita nova em
  disco, não reimplementar.
- `IdentityFile`/`sourceFile` são armazenados compactados para `~/...` quando sob o home atual
  (`compactHome`/`expandHome`) para portabilidade entre máquinas — importante em qualquer código
  que grave paths de host ou de chave.
- Import de backup trata o pacote como **entrada não confiável**: proteção contra tar slip, nomes
  de arquivo validados, fingerprint sempre reconferido a partir do `.pub` extraído. Qualquer nova
  rotina de import deve manter esse nível de desconfiança.
- Ações destrutivas (sobrescrever/remover arquivos existentes) exigem confirmação explícita nas
  camadas de interface — `core` não decide isso sozinho, só expõe os dados para a decisão.
- Comentários de código e a spec são em pt-BR; siga esse idioma em comentários/docs deste
  repositório.
- Decisões conhecidas e riscos aceitos (não são bugs a corrigir sem discussão): sem lock de
  concorrência em `metadata.json`; sem criptografia no pacote de backup; gap de ACL no Windows
  para chaves privadas (`0600` só alterna read-only lá). Ver seção 6/7 da spec antes de "corrigir"
  qualquer um desses.
