# keyward

Gerenciador de configuração SSH para Windows, Linux e macOS: cuida dos hosts do seu
`~/.ssh/config` e das suas chaves — incluindo **controle de expiração e rotação**, que o OpenSSH
sozinho não oferece para chaves comuns. Tudo é local: sem servidor, sem conta, sem telemetria.

A mesma lógica de negócio é exposta por três interfaces, que você escolhe conforme a situação:

| Binário | Interface | Para quê |
|---|---|---|
| `keyward` | linha de comando | scripts, automação, uso pontual |
| `keyward-tui` | terminal interativo | navegar e editar sem decorar flags, direto no terminal |
| `keyward-gui` | app desktop | uso do dia a dia com mouse e diálogos nativos |

O que ele faz:

- Lê `~/.ssh/config` **seguindo `Include` recursivamente**, com a mesma visão que o `ssh` usa.
- Adiciona e substitui blocos `Host` preservando formatação e comentários dos blocos não tocados.
- Gera chaves (ed25519 por padrão, RSA 4096+ opcional), com passphrase opcional.
- Mantém metadata própria por chave (rótulo, expiração, notas), associada por **fingerprint** —
  sobrevive a renomear ou mover o arquivo da chave.
- Reconcilia o que está em disco com o que está registrado, e destaca chaves expiradas ou
  expirando em breve.
- Exporta e importa hosts + chaves + metadata em um pacote `.tar.gz` único, com detecção de
  conflitos.

Spec completa (pt-BR, com todas as decisões de design e seus motivos):
[`docs/specs/ssh-config-manager.md`](docs/specs/ssh-config-manager.md).

## Instalação

Baixe na [página de releases](https://github.com/felipehrs/keyward/releases). Cada release
publica **um arquivo por sistema operacional**, contendo as três interfaces, mais um instalador
no Linux e no Windows:

| Asset | Conteúdo |
|---|---|
| `keyward_<versão>_linux_amd64.tar.gz` | `keyward`, `keyward-tui`, `keyward-gui` |
| `keyward_<versão>_windows_amd64.zip` | `keyward.exe`, `keyward-tui.exe`, `keyward-gui.exe` |
| `keyward_<versão>_darwin_universal.tar.gz` | `keyward`, `keyward-tui`, `keyward-gui.app/` |
| `keyward.deb` / `keyward.rpm` | os três em `/usr/local/bin` + lançador da GUI |
| `keyward-amd64-installer.exe` | os três em `Program Files`, no `PATH`, + atalho da GUI |
| `checksums.txt` | cobre todos os acima |

O macOS sai como **universal binary** (Intel + Apple Silicon no mesmo arquivo). Linux e Windows
são amd64.

> Esse formato consolidado vale a partir da `v0.2.0-beta.3`. Releases anteriores (`v0.1.0`)
> publicavam um arquivo por binário/arquitetura, e os instaladores traziam só a GUI.

### Linux

Pacote (recomendado — resolve as dependências da GUI e instala o lançador no menu):

```bash
sudo apt install ./keyward.deb
```

```bash
sudo dnf install ./keyward.rpm
```

Ou o tarball, se preferir não instalar nada no sistema:

```bash
tar xzf keyward_<versão>_linux_amd64.tar.gz && sudo install -m 0755 keyward keyward-tui keyward-gui /usr/local/bin/
```

`keyward` e `keyward-tui` não têm dependência nenhuma. A **GUI** precisa de GTK 4 e WebKitGTK 6.0
(`libgtk-4-1` + `libwebkitgtk-6.0-4` no Debian/Ubuntu; `gtk4` + `webkitgtk6.0` no Fedora) — o
`.deb`/`.rpm` declara isso, o tarball não.

### Windows

Rode `keyward-amd64-installer.exe`: instala as três interfaces, adiciona a pasta ao `PATH` da
máquina e cria o atalho da GUI. Abra um terminal **novo** depois de instalar, para que o `PATH`
atualizado valha.

Ou extraia o `.zip` e coloque os três `.exe` onde preferir. A GUI usa o WebView2, já presente por
padrão no Windows 11 (o instalador inclui o bootstrapper para versões mais antigas).

### macOS

```bash
tar xzf keyward_<versão>_darwin_universal.tar.gz && sudo install -m 0755 keyward keyward-tui /usr/local/bin/ && mv keyward-gui.app /Applications/
```

Os binários **não são assinados** — ver [Limitações](#limitações-atuais).

### A partir do código

`keyward` e `keyward-tui` são Go puro, sem cgo:

```bash
go install github.com/felipehrs/keyward/cmd/cli@latest
```

> Isso instala com o nome `cli` (e `tui`), não `keyward`. Para os nomes definitivos, use
> `go build -o keyward ./cmd/cli` a partir de um clone.

A GUI exige o CLI do Wails v3 e Node.js — ver [`docs/build.md`](docs/build.md).

## Uso

### CLI

```bash
keyward --help
```

**Hosts** (`~/.ssh/config`):

```bash
keyward host list
```

```bash
keyward host add prod --host-name 10.0.0.5 --user deploy --identity-file ~/.ssh/id_ed25519
```

```bash
keyward host replace prod --host-name 10.0.0.9 --user deploy
```

`host replace` substitui o **bloco inteiro**: o que você não passar por flag some do bloco.
`--file` aponta para outro arquivo que não o `~/.ssh/config` (útil com `Include`).

**Chaves e expiração:**

```bash
keyward key generate --label "prod deploy" --expires-at 2027-01-31
```

```bash
keyward key list
```

```bash
keyward key get SHA256:abc...
```

```bash
keyward key update SHA256:abc... --expires-at 2028-01-31
```

```bash
keyward key register ~/.ssh/id_antiga --label "chave herdada"
```

```bash
keyward key settings set --alert-threshold-days 30 --default-algorithm ed25519
```

`key generate` também aceita `--algorithm rsa --rsa-bits 4096`, `--passphrase-stdin`,
`--directory`, `--file-name` e `--overwrite`. `key register` traz para a metadata uma chave que já
existia em disco; `key unregister` faz o contrário, **sem apagar arquivo nenhum**.

`keyward key list` sai ordenado por proximidade de expiração e **retorna código 1** quando há
chave expirada ou expirando dentro do limite configurado — dá para usar direto num cron ou no
`.bashrc` como alerta.

Rotação é composição, não um comando próprio: `key generate` a nova, aponte o host para ela com
`host replace`, e `key unregister` a antiga.

**Backup:**

```bash
keyward backup export backup.tar.gz --all-hosts --all-keys
```

```bash
keyward backup preview-import backup.tar.gz
```

```bash
keyward backup import backup.tar.gz --on-host-conflict skip --on-key-conflict rename
```

`--all-keys` inclui **só metadata e chave pública**. Material privado só sai com
`--key-with-private <fingerprint>`, explicitamente, uma chave por vez — e o pacote **não é
criptografado**. `preview-import` mostra exatamente o que o import faria, sem escrever nada.

### TUI

```bash
keyward-tui
```

Navegação por teclado, cobrindo hosts, chaves, configurações e backup. O import resolve conflito
**item a item**, diferente da CLI.

### GUI

Abra pelo atalho no menu/Iniciar, ou rode `keyward-gui`. Paridade funcional com a TUI, com
diálogos nativos de arquivo para export/import.

### Apontando para um ambiente de teste

A TUI e a GUI aceitam `KEYWARD_CONFIG`, `KEYWARD_KEY_DIR` e `KEYWARD_METADATA` para trabalhar
sobre um `~/.ssh` de mentira, sem tocar no ambiente real:

```bash
KEYWARD_CONFIG=/tmp/lab/config KEYWARD_KEY_DIR=/tmp/lab/keys KEYWARD_METADATA=/tmp/lab/metadata.json keyward-tui
```

### Onde ficam os dados

- Hosts: `~/.ssh/config` — o seu de sempre, o keyward não cria um formato paralelo.
- Chaves: `~/.ssh/`.
- Metadata: `~/.config/keyward/metadata.json` no Linux,
  `~/Library/Application Support/keyward/metadata.json` no macOS,
  `%APPDATA%\keyward\metadata.json` no Windows.

## Limitações atuais

Conhecidas e conscientes — a maioria está registrada com o "porquê" nas
[seções 6 a 8 da spec](docs/specs/ssh-config-manager.md#6-considerações-de-segurança).

**Segurança**

- **Backup não é criptografado.** Um export com `--key-with-private` grava a chave privada em
  texto claro dentro do `.tar.gz`. O app avisa antes; proteger o arquivo é com você.
- **Gap de ACL no Windows.** Chaves privadas são criadas com `0600`, mas no Windows isso só
  alterna o atributo read-only — a proteção real vem da ACL herdada de `%USERPROFILE%\.ssh`.
- **Binários e instaladores não são assinados** (sem certificado Authenticode/Apple Developer ID).
  Espere aviso do SmartScreen no Windows; no macOS, o `.app` exige liberar em Ajustes ›
  Privacidade e Segurança na primeira execução.
- **Sem lock em `metadata.json`.** A escrita é atômica (não trunca o arquivo), mas duas interfaces
  abertas ao mesmo tempo podem fazer uma sobrescrever a outra. Use uma por vez.

**Funcionalidades**

- A **CLI não tem `host remove`** — a remoção existe no `core`, na TUI e na GUI, mas não foi
  exposta como comando.
- A CLI **não lê** `KEYWARD_CONFIG`/`KEYWARD_KEY_DIR`/`KEYWARD_METADATA`; sempre trabalha sobre o
  ambiente real. Só TUI e GUI aceitam essas variáveis.
- No `backup import` da CLI, a política de conflito é **única para todos os itens**
  (`--on-host-conflict`/`--on-key-conflict`). Resolução item a item só na TUI e na GUI.
- `host add`/`host replace` reescrevem o bloco alterado com **formatação limpa**: comentários e
  espaçamento originais *daquele bloco* se perdem. Os demais blocos ficam byte a byte intactos.
- Sem `known_hosts`, sem integração com `ssh-agent`, sem certificados SSH assinados por CA, sem
  hardware token (YubiKey/FIDO2), sem sincronização em nuvem e sem `lastUsedAt` (última vez que a
  chave foi usada). Tudo fora do escopo do v1 — spec, seção 8.

**Distribuição e maturidade**

- **Só amd64** no Linux e no Windows; não há build ARM para essas plataformas. macOS é universal.
- Sem Homebrew tap, Scoop bucket, AppImage, `.dmg`/`.pkg` ou AUR próprio.
- Os instaladores `.deb`/`.rpm` e NSIS são gerados e publicados pelo CI a cada tag, e o conteúdo
  dos pacotes é conferido — mas **nenhuma instalação real foi validada ponta a ponta** ainda
  (`PATH` após o instalador do Windows, lançador da GUI no Linux, desinstalação).
- A GUI roda sobre **Wails v3, ainda em beta** (`v3.0.0-beta.3`).
- O backend da GUI tem 40 testes automatizados; o **frontend não tem test runner** — renderização
  e cliques foram verificados manualmente, não por teste.
- **Licença ainda não definida** (ver abaixo) — na prática, ninguém tem permissão explícita para
  redistribuir o código.

## Status do desenvolvimento

- [x] `core` — lógica de negócio (parsing/escrita de `~/.ssh/config`, geração de chaves,
  metadata/reconciliação, backup export/import)
- [x] Lint/CI — `golangci-lint` (com `depguard` verificando a fronteira do Wails) + GitHub Actions
- [x] `cmd/cli` — CLI via Cobra, cobrindo `key`, `host` e `backup`
- [x] `cmd/tui` — TUI via Bubble Tea, cobrindo hosts, chaves (com destaque de expiração),
  configurações e backup (export/import com resolução de conflito item a item)
- [x] `internal/adapter/gui` — GUI desktop via Wails v3, com paridade funcional com a TUI e
  formulários em modal
- [x] Empacotamento/distribuição — um arquivo compactado por SO com as três interfaces, mais
  instalador único no Linux (`.deb`/`.rpm`) e no Windows (NSIS). Um GitHub Release por tag
  `vX.Y.Z`, via [`.github/workflows/release.yml`](.github/workflows/release.yml): runners nativos
  montam os artefatos e o [GoReleaser](https://goreleaser.com/) cuida de changelog, checksums e
  publicação.

## Desenvolvimento

```
core/                          → lógica de negócio pura (parsing, chaves, metadata, backup)
cmd/cli/                       → CLI (Cobra)
cmd/tui/                       → TUI (Bubble Tea)
internal/adapter/gui/          → app desktop (Wails) — único pacote que importa "wails"
internal/adapter/gui/frontend/ → frontend web da GUI (HTML/CSS/TS)
```

`core` não depende de nenhuma interface e expõe sua API pública como interfaces Go
(`ConfigService`, `KeyService`, `BackupService`), consumidas por CLI, TUI e GUI. A fronteira do
Wails é verificada por `depguard` no CI. Mais detalhes na
[seção 3 da spec](docs/specs/ssh-config-manager.md#3-arquitetura-proposta).

Requer Go 1.26.5+.

```bash
make check
```

```bash
go run ./cmd/cli --help
```

`make check` roda `vet + build + test + lint`; `make test` roda só os testes. Para buildar
binários nomeados, a GUI e os pacotes de release, ver [`docs/build.md`](docs/build.md).

## Licença

Ainda não definida.
