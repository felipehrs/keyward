# Gerando binários

`make build` (`go build ./...`) só valida que tudo compila — não emite um binário nomeado e
utilizável. Para gerar um executável, use `go build -o` apontando para o pacote `main` desejado.

## CLI e TUI

Ambos têm binário pronto para uso hoje (`internal/adapter/gui` ainda é placeholder — veja a
seção "Status do desenvolvimento" do [`README.md`](../README.md)).

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

> A GUI (`internal/adapter/gui`, via Wails) não vai valer essa mesma facilidade de cross-compile
> quando for implementada — Wails empacota assets nativos por plataforma. Isso ficará documentado
> aqui quando a GUI sair do estágio de placeholder.

## Empacotamento

Ainda não há target de release/cross-compile automatizado no `Makefile`, nem empacotamento por
plataforma (MSI, `.deb`/`.rpm`, `.dmg`) — está listado como pendente na seção "Status do
desenvolvimento" do [`README.md`](../README.md).
