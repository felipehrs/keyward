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

Duas ferramentas, cada uma cuidando do que ela faz bem — ver `docs/specs/ssh-config-manager.md`
seção 7 para o raciocínio completo por trás dessa divisão:

- **`cmd/cli`/`cmd/tui`** (Go puro, sem cgo): [GoReleaser](https://goreleaser.com/), configurado
  em [`.goreleaser.yaml`](../.goreleaser.yaml). Cross-compile nativo do Go
  (`{linux,darwin,windows} × {amd64,arm64}`), gera `.tar.gz`/`.zip` por plataforma,
  `checksums.txt` e changelog agrupado por prefixo de commit (`feat`/`fix`).
- **GUI (Wails)**: cgo + webview nativo não são o forte do GoReleaser — continua usando o
  tooling do próprio Wails (`wails3 task package`/`package:dmg`, ver seção "GUI" acima), rodando
  numa matriz de runners nativos por SO no CI.

Os dois pipelines são orquestrados por um único workflow,
[`.github/workflows/release.yml`](../.github/workflows/release.yml), disparado por push de tag
`vX.Y.Z`: primeiro empacota a GUI nos três SOs, depois roda o GoReleaser (que recebe os
instaladores da GUI via `release.extra_files` e publica tudo — CLI, TUI e GUI — num único GitHub
Release).

Testar localmente sem publicar nada (não precisa de tag real):

```bash
goreleaser check                          # valida o .goreleaser.yaml
goreleaser release --snapshot --clean     # build completo em dist/, sem publicar
```

Cortar um release de verdade:

```bash
git tag vX.Y.Z
git push --tags   # dispara o workflow de release
```

**Fora de escopo por decisão consciente (ver spec seção 7)**: Homebrew tap/Scoop/AUR próprio
(só GitHub Releases por ora) e assinatura de código (Authenticode/notarization) — sem
certificado disponível hoje, os binários/instaladores vão gerar aviso do SmartScreen/Gatekeeper.
