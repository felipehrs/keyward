# keyward

Ferramenta multiplataforma (Windows, Linux e macOS) para gerenciar sua configuração SSH —
hosts em `~/.ssh/config` e chaves, incluindo controle de expiração/rotação — sem depender de
servidor central ou conta na nuvem. A mesma lógica de negócio é exposta através de três
interfaces: CLI, TUI e GUI desktop.

> Status: em desenvolvimento (MVP). Veja a spec completa em
> [`docs/specs/ssh-config-manager.md`](docs/specs/ssh-config-manager.md) para todas as decisões
> de design e o que ainda está em aberto.

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
```

CLI e TUI ainda estão em construção — o `core` já implementa boa parte da lógica de negócio do
MVP (parsing/escrita de config, geração de chaves, metadata e backup/restore).

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
