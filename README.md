# keyward

Ferramenta multiplataforma (Windows, Linux e macOS) para gerenciar sua configuração SSH —
hosts em `~/.ssh/config` e chaves, incluindo controle de expiração/rotação — sem depender de
servidor central ou conta na nuvem. A mesma lógica de negócio é exposta através de três
interfaces: CLI, TUI e GUI desktop.

> Status: em desenvolvimento (MVP). Veja a spec completa em
> [`docs/specs/ssh-config-manager.md`](docs/specs/ssh-config-manager.md) para todas as decisões
> de design e o que ainda está em aberto.

## Status do desenvolvimento

- [x] `core` — lógica de negócio (parsing/escrita de `~/.ssh/config`, geração de chaves,
  metadata/reconciliação, backup export/import)
- [x] Lint/CI — `golangci-lint` (com `depguard` verificando a fronteira do Wails) + GitHub Actions
- [x] `cmd/cli` — CLI via Cobra, cobrindo `key`, `host` e `backup`
- [x] `cmd/tui` — TUI via Bubble Tea, cobrindo hosts (listar/adicionar/editar/remover), chaves
  (listar com destaque de expiração/gerar/detalhe/editar metadata/registrar órfã/unregister),
  configurações e backup (export/import com resolução de conflito item a item)
- [x] `internal/adapter/gui` — GUI desktop via Wails v3, cobrindo hosts, chaves (com destaque de
  expiração/gerar/registrar órfã/editar metadata/unregister), configurações e backup
  (export/import com resolução de conflito item a item) — paridade funcional com a TUI. Backend
  (`app_*.go`/`dto_*.go`) tem 40 testes automatizados; a interação real do frontend (renderização,
  diálogos nativos de arquivo, cliques) só foi verificada por build/execução sem erro, não por
  teste de UI interativo — não há test runner de frontend configurado (decisão consciente, para
  não empilhar mais risco de toolchain sobre o do próprio Wails v3, ainda em beta)
- [ ] Empacotamento/distribuição por plataforma (MSI, .deb/.rpm, .dmg)

## Funcionalidades

- Leitura/parsing de `~/.ssh/config`, seguindo diretivas `Include` recursivamente, com a mesma
  visão que o próprio `ssh` usa.
- Adição e substituição de hosts preservando formatação e comentários dos blocos não alterados.
- Geração de chaves SSH (ed25519 por padrão, RSA 4096+ opcional), com passphrase opcional.
- Metadata própria por chave (rótulo, data de expiração, notas), associada por fingerprint —
  sobrevive a renomear/mover o arquivo de chave.
- Reconciliação entre chaves em disco e metadata registrada, com alerta de chaves expirando ou
  expiradas.
- Backup/export e import de configuração, chaves e metadata em um pacote único (`.tar.gz`), com
  detecção de conflitos e tratamento do pacote importado como entrada não confiável.

## Arquitetura

```
core/                      → lógica de negócio pura (parsing, geração de chaves, metadata, backup)
cmd/cli/                   → interface de linha de comando (Cobra)
cmd/tui/                   → interface de terminal interativa (Bubble Tea)
internal/adapter/gui/      → aplicação desktop (Wails) — único pacote que importa "wails"
internal/adapter/gui/frontend/ → frontend web da GUI (HTML/CSS/JS)
```

`core` não depende de nenhuma interface e expõe sua API pública como interfaces Go
(`ConfigService`, `KeyService`, `BackupService`), consumidas por CLI, TUI e GUI. Mais detalhes em
[`docs/specs/ssh-config-manager.md`](docs/specs/ssh-config-manager.md#3-arquitetura-proposta).

## Requisitos

- Go 1.26.5+

## Uso

```bash
make build   # compila todos os binários
make test    # roda os testes
make vet     # go vet
make lint    # golangci-lint (requer instalação prévia)
make check   # vet + build + test + lint

go run ./cmd/cli --help   # lista os comandos disponíveis da CLI
go run ./cmd/tui          # abre a interface interativa de terminal
```

```bash
cd internal/adapter/gui
wails3 dev          # abre a GUI em modo desenvolvimento (hot reload do frontend)
wails3 task build   # build de produção (backend + frontend + binário nativo em bin/)
wails3 task run     # roda o binário já buildado
```

A CLI (`cmd/cli`) já cobre todos os métodos de `KeyService`, `ConfigService` e `BackupService` —
`keyward key ...`, `keyward host ...`, `keyward backup ...`. A TUI (`cmd/tui`) e a GUI
(`internal/adapter/gui`) cobrem o mesmo conjunto de operações, cada uma na sua interface.

Para apontar a TUI ou a GUI para um `~/.ssh` e `metadata.json` de teste (sem tocar no ambiente
real do usuário), use as variáveis de ambiente `KEYWARD_CONFIG`, `KEYWARD_KEY_DIR` e
`KEYWARD_METADATA` — ver [`cmd/tui/main.go`](cmd/tui/main.go) e
[`internal/adapter/gui/main.go`](internal/adapter/gui/main.go).

A GUI requer o CLI do Wails v3 (`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`) e
Node.js/npm no PATH — rode `wails3 doctor` pra conferir dependências de sistema por plataforma
(WebView2 no Windows, GTK3+WebKit2GTK no Linux, Xcode Command Line Tools no macOS). `go build
./...`/`go test ./...` na raiz do repo cobrem o pacote Go da GUI normalmente; só os comandos
`wails3 ...` acima exigem rodar a partir de `internal/adapter/gui/`.

## Segurança

- Chaves privadas são criadas com permissão restrita ao usuário (`0600` em Linux/macOS; no
  Windows há um gap conhecido de ACL, documentado na spec).
- Nenhum dado é enviado para servidores externos — toda operação é local.
- O pacote de backup **não é criptografado** no MVP; ao incluir chaves privadas em um export, o
  app avisa explicitamente antes.
- Ações destrutivas (sobrescrever ou remover arquivos existentes) sempre exigem confirmação
  explícita do usuário.

Veja a [seção 6 da spec](docs/specs/ssh-config-manager.md#6-considerações-de-segurança) para a
lista completa de considerações e riscos aceitos conscientemente.

## Licença

Ainda não definida.
