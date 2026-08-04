# Gerando binários

`make build` (`go build ./...`) só valida que tudo compila — não emite um binário nomeado e
utilizável. Para gerar um executável, use `go build -o` apontando para o pacote `main` desejado.

## CLI e TUI

Ambos têm binário pronto para uso hoje.

Build para a plataforma atual:

```bash
go build -o bin/keyward     ./cmd/cli
go build -o bin/keyward-tui ./cmd/tui
```

No Windows, use extensão `.exe`:

```bash
go build -o bin/keyward.exe     ./cmd/cli
go build -o bin/keyward-tui.exe ./cmd/tui
```

## Cross-compile

`cmd/cli` e `cmd/tui` dependem só de `core` + bibliotecas puramente Go (`x/crypto/ssh`,
`kevinburke/ssh_config`, e no caso da TUI, `charmbracelet/bubbletea`/`bubbles`/`lipgloss`) — sem
cgo. Isso permite cross-compile direto, sem toolchain externa, via `GOOS`/`GOARCH`:

```bash
GOOS=linux   GOARCH=amd64 go build -o bin/keyward-linux-amd64        ./cmd/cli
GOOS=darwin  GOARCH=arm64 go build -o bin/keyward-darwin-arm64       ./cmd/cli
GOOS=windows GOARCH=amd64 go build -o bin/keyward-windows-amd64.exe  ./cmd/cli

GOOS=linux   GOARCH=amd64 go build -o bin/keyward-tui-linux-amd64        ./cmd/tui
GOOS=darwin  GOARCH=arm64 go build -o bin/keyward-tui-darwin-arm64       ./cmd/tui
GOOS=windows GOARCH=amd64 go build -o bin/keyward-tui-windows-amd64.exe  ./cmd/tui
```

> A GUI (`internal/adapter/gui`, via Wails) não vale essa mesma facilidade de cross-compile —
> Wails empacota assets nativos por plataforma (WebView2/GTK+WebKit2GTK/WebKit) e depende de um
> tooling próprio, não só `go build`. Ver seção seguinte.

## GUI (Wails v3)

Diferente de CLI/TUI, o `main.go` da GUI mora dentro de `internal/adapter/gui/` (não em
`cmd/gui/`) — é uma exigência do tooling do `wails3` v3.0.0-beta.3, que espera `main.go`,
`Taskfile.yml` e `build/` no mesmo diretório (não usa `wails.json` como a v2). O pacote continua
dentro do módulo único do repo — `go build ./...`/`go test ./...` na raiz funcionam normalmente.

Pré-requisitos: CLI do Wails v3 (`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`),
Node.js/npm no PATH, e dependências de sistema por plataforma (rode `wails3 doctor` pra
confirmar) — WebView2 no Windows, `gtk3`+`webkit2gtk` no Linux, Xcode Command Line Tools no
macOS. Build da GUI usa `CGO_ENABLED=1` — ruptura consciente do princípio "binário estático sem
cgo" que vale só para CLI/TUI (spec, seção 3.2), aceita porque `internal/adapter/gui` é isolado
do resto do projeto pela regra de `depguard`.

```bash
cd internal/adapter/gui
wails3 dev                     # modo desenvolvimento, hot reload do frontend
wails3 task build              # build de produção — binário nativo em bin/keyward(.exe)
wails3 task run                # roda o binário já buildado
wails3 generate bindings       # regenera os bindings TS em frontend/bindings/ após mudar app_*.go
```

Não há cross-compile simples para a GUI (webview nativo por plataforma); `wails3 task` inclui
targets Docker (`wails3 task setup:docker`/`build:docker`) para cross-compilar Linux a partir de
outro SO, quando necessário.

## Empacotamento e release

Cada release publica **um arquivo compactado por sistema operacional**, contendo as três
interfaces, mais um instalador único no Linux e no Windows:

| Asset | Conteúdo |
|---|---|
| `keyward_<v>_linux_amd64.tar.gz` | `keyward`, `keyward-tui`, `keyward-gui` |
| `keyward_<v>_windows_amd64.zip` | `keyward.exe`, `keyward-tui.exe`, `keyward-gui.exe` |
| `keyward_<v>_darwin_universal.tar.gz` | `keyward`, `keyward-tui`, `keyward-gui.app/` |
| `keyward.deb` / `keyward.rpm` | os três em `/usr/local/bin` + lançador da GUI |
| `keyward-amd64-installer.exe` | os três em `Program Files`, no `PATH`, + atalho da GUI |
| `checksums.txt` | cobre todos os acima |

O macOS sai como **universal binary** (amd64 + arm64 via `lipo`): um arquivo só, rodando nativo
tanto em Intel quanto em Apple Silicon. Linux e Windows são amd64.

### Como o pipeline é montado

[`.github/workflows/release.yml`](../.github/workflows/release.yml) dispara em push de tag
`vX.Y.Z` e tem dois jobs:

1. **`build`** — matriz de três runners nativos. Cada um é autossuficiente: builda a GUI com
   `wails3 task build` (cgo + webview nativo), builda CLI e TUI com `go build`, monta o arquivo
   compactado do seu SO e o instalador correspondente.
2. **`release`** — baixa os artefatos dos três e roda o GoReleaser.

O **GoReleaser não builda nem arquiva nada** aqui (`builds: - skip: true` em
[`.goreleaser.yaml`](../.goreleaser.yaml)) — ele cuida só do changelog agrupado por prefixo de
commit, do `checksums.txt` e da criação do Release. Dois motivos concretos para essa divisão:

- o bundle `keyward-gui.app` do macOS é um **diretório**, e o GoReleaser não percorre diretórios
  recursivamente nos globs de `archives.files`;
- os instaladores precisam dos três binários no mesmo job que os empacota, o que não encaixa com
  o GoReleaser buildando CLI/TUI num job separado depois.

Como cada runner nativo já precisa dos três binários para o instalador, montar o arquivo
compactado ali é uma linha de `tar`/`Compress-Archive`.

> As tasks `create:deb`/`create:rpm`/`create:nsis:installer` do Wails **não** são usadas: todas
> têm `deps: build`, que rebuildaria a GUI por cima de `bin/keyward` — onde, no nosso layout,
> mora a CLI. O workflow chama `generate:deb`/`generate:rpm` e o `makensis` diretamente.

Testar localmente sem publicar nada (não precisa de tag real):

```bash
goreleaser check
```

Cortar um release:

```bash
git tag vX.Y.Z
git push --tags   # dispara o workflow de release
```

**Fora de escopo por decisão consciente (ver spec seção 7)**: AppImage, `.dmg` e `.pkg` (o macOS
distribui pelo `.tar.gz`); ARM no Linux/Windows; Homebrew tap/Scoop/AUR próprio; e assinatura de
código (Authenticode/notarization) — sem certificado disponível hoje, os binários e instaladores
vão gerar aviso do SmartScreen/Gatekeeper, e no macOS o `.app` não assinado exige liberar em
Ajustes › Privacidade e Segurança na primeira execução.
