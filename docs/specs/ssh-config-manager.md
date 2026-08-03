# Spec: keyward (SSH Config Manager)

**Status:** Rascunho
**Data:** 2026-08-03
**Autor:** —

> Nome do projeto/binário: `keyward` (decidido em 2026-08-03). Usado em todos os exemplos deste
> documento.

---

## 1. Objetivo

Fornecer uma ferramenta multiplataforma (Windows, Linux e macOS) para gerenciar a configuração
SSH do usuário — hosts em `~/.ssh/config`, chaves e seus ciclos de vida — reduzindo o trabalho
manual de editar arquivos texto e o risco de esquecer chaves antigas, sem validade definida, em
uso por tempo indeterminado. O projeto expõe a mesma lógica de negócio através de três interfaces
(CLI, TUI e GUI desktop), para atender desde uso em scripts/automação até uso interativo do
dia a dia.

---

## 2. Público-alvo

- Desenvolvedores e profissionais de infraestrutura que gerenciam múltiplos hosts, servidores e
  contas SSH (pessoais, de trabalho, de clientes).
- Usuários que já têm familiaridade com `~/.ssh/config` e chaves SSH, mas quer maior visibilidade
  e controle sobre o que existe, para quê serve, e há quanto tempo cada chave está em uso.
- Uso individual, local, sem dependência de servidor central ou conta na nuvem.

---

## 3. Arquitetura proposta

### 3.1 Visão geral

```
                      ┌─────────────────────────┐
                      │  keyward-core (biblioteca) │
                      │  parsing, geração de       │
                      │  chaves, metadata,          │
                      │  backup/restore              │
                      │  (define interfaces públicas) │
                      └────────────┬─────────────┘
                                   │ depende de (nunca o inverso)
              ┌────────────────────┼────────────────────┐
              │                    │                    │
      ┌───────▼──────┐    ┌───────▼──────┐     ┌───────▼──────────────┐
      │  CLI (Cobra)  │    │ TUI (Bubble  │     │ adapter/gui (Wails)   │
      │  keyward ...  │    │ Tea)         │     │ único ponto que        │
      │               │    │              │     │ importa "wails"         │
      └───────────────┘    └──────────────┘     ├───────────────────────┤
                                                  │ frontend/ (HTML/CSS/JS)│
                                                  └───────────────────────┘
```

- **`core`**: módulo Go com toda a lógica de negócio (parsing/escrita de `~/.ssh/config`,
  geração de chaves, leitura/escrita da metadata de expiração, backup/restore, validações de
  segurança). Não depende de nenhuma das três interfaces — é testável isoladamente. Expõe sua
  API pública como **interfaces Go** (não structs concretas), consumidas por CLI, TUI e GUI.
- **CLI**: usa [Cobra](https://github.com/spf13/cobra) para comandos e subcomandos
  (`keyward key generate`, `keyward host list`, `keyward backup export`, etc.). Voltada para uso
  em scripts e automação.
- **TUI**: usa [Bubble Tea](https://github.com/charmbracelet/bubbletea) para uma interface de
  terminal interativa (navegação por listas de hosts/chaves, formulários simples).
- **GUI**: aplicação desktop nativa via [Wails](https://wails.io/) (backend Go + frontend web
  renderizado em webview nativo). Decisão registrada em 2026-08-03 — ver seção 3.3 para como a
  dependência do Wails é isolada do restante do projeto.

### 3.2 Decisões técnicas

- **Linguagem:** Go — compila para binário único estático por plataforma, sem runtime externo,
  o que facilita distribuir CLI, TUI e GUI a partir da mesma base de código para Windows, Linux
  e macOS.
- **Módulo Go:** `hrs.dev.br/keyward` (decidido em 2026-08-03, `go.mod` inicializado com
  Go 1.26.5). Pacotes internos são referenciados a partir desse path (ex.
  `hrs.dev.br/keyward/core`, `hrs.dev.br/keyward/cmd/cli`).
- **SSH:** `golang.org/x/crypto/ssh` e `golang.org/x/crypto/ssh/agent` para geração de chaves,
  parsing de formatos e integração futura com `ssh-agent`. Confirmado na prática (2026-08-03):
  `ssh.MarshalPrivateKey`/`MarshalPrivateKeyWithPassphrase` geram chaves ed25519/RSA em formato
  OpenSSH 100% em Go puro, sem depender do binário `ssh-keygen`.
- **Parsing de `~/.ssh/config`:** `github.com/kevinburke/ssh_config`.
- **UUID:** `github.com/google/uuid`, usado para `KeyMetadata.ID`.
- **Persistência:** arquivos locais (sem banco de dados externo) — detalhado na seção 4.
- **GUI:** [Wails](https://wails.io/) v3. Decisão consciente de aceitar dois riscos conhecidos
  em troca de melhor controle visual: (1) bus factor concentrado no mantenedor principal do
  projeto; (2) fragmentação de versões do WebKitGTK entre distribuições Linux, que pode exigir
  flags de build específicas por distro (`wails3 doctor` orienta caso a caso). Ambos os riscos
  motivam o isolamento descrito em 3.3 — se algum deles se materializar de forma grave, a troca
  de toolkit de GUI deve ficar restrita a uma única camada do projeto.

### 3.3 Isolamento da GUI (portabilidade futura)

Como o Wails carrega mais risco externo (webview do SO, tooling JS/CSS, projeto de mantenedor
único) do que Cobra ou Bubble Tea, a GUI é a interface mais provável de precisar ser trocada no
futuro. A regra arquitetural para isso é: **nenhum pacote fora da camada de GUI pode importar
algo do Wails, e a camada de GUI nunca implementa lógica de negócio — só tradução.**

Estrutura de pacotes proposta:

```
/core                      → lógica de negócio pura, só depende da stdlib e de x/crypto/ssh
    keys.go                    (interface KeyService, structs de domínio)
    config.go                  (interface ConfigService)
    backup.go                  (interface BackupService)
/cmd/cli                   → depende de /core (via Cobra)
/cmd/tui                   → depende de /core (via Bubble Tea)
/internal/adapter/gui      → único lugar do repositório autorizado a importar "wails"
    app.go                     (struct bound ao frontend, métodos chamam /core)
    dto.go                     (structs de transporte para o frontend — não reusa structs do /core)
/internal/adapter/gui/frontend → HTML/CSS/JS, fala só com app.go via bindings gerados pelo Wails
```

Regras concretas:

1. **Direção de dependência única**: `core` não importa nada de `internal/adapter/gui`, `cmd/cli`
   ou `cmd/tui`. A dependência é sempre "de fora para dentro".
2. **Contrato explícito via interfaces**: `core` expõe `KeyService`, `ConfigService`,
   `BackupService` (nomes de exemplo) como interfaces. CLI, TUI e o adapter da GUI consomem essas
   interfaces — nenhum deles depende de tipos concretos internos do `core`.
3. **DTOs próprios da GUI**: os dados trocados com o frontend (JSON via bindings do Wails) são
   modelados em `internal/adapter/gui/dto.go`, convertidos a partir dos tipos do `core`. Isso
   evita que uma mudança no modelo de domínio quebre o frontend silenciosamente, e vice-versa.
4. **`internal/adapter/gui` é descartável por definição**: se um dia for necessário trocar Wails
   por outra coisa (Fyne, Tauri, o que for), a expectativa é reescrever **somente** esse pacote e
   o `frontend/` — `core`, `cmd/cli` e `cmd/tui` não deveriam precisar de nenhuma alteração além,
   possivelmente, de pequenos ajustes na interface pública se algo estiver faltando.
5. **Verificação de fronteira**: adicionar um check simples de CI/lint (ex. `go list` + grep, ou
   uma regra do `depguard`) que falha o build se `github.com/wailsapp/wails` aparecer importado
   fora de `internal/adapter/gui`. Isso transforma a regra 1 em algo verificável, não só uma
   convenção de código.

O que essa separação **não** resolve: o próprio HTML/CSS/JS do frontend é, por natureza,
específico do Wails (ou de qualquer engine de webview usada) — trocar de toolkit de GUI sempre
vai exigir reconstruir a camada visual em si. O ganho aqui é garantir que isso fique contido em
`internal/adapter/gui/` + `frontend/`, sem vazar para o resto do projeto.

---

## 4. Modelo de dados

### 4.1 Fontes de verdade existentes (não duplicadas pelo app)

O app **lê e escreve diretamente** em `~/.ssh/config` e no diretório `~/.ssh/` (arquivos de
chave pública/privada), respeitando o formato padrão do OpenSSH. Ele não mantém uma cópia
paralela dessas informações — apenas as interpreta.

### 4.2 Metadata própria do app

Como chaves SSH comuns não carregam data de expiração nativamente, o app mantém um arquivo de
metadata **separado do `~/.ssh/`**, para não interferir em ferramentas externas (OpenSSH, Git,
etc.) que também leem esse diretório.

- **Localização:** diretório de config do usuário por plataforma (via
  [`os.UserConfigDir()`](https://pkg.go.dev/os#UserConfigDir) do Go), ex.:
  - Linux: `~/.config/keyward/metadata.json`
  - macOS: `~/Library/Application Support/keyward/metadata.json`
  - Windows: `%AppData%\keyward\metadata.json`
- **Formato:** JSON (legível, versionável, fácil de fazer backup manual).

### Entidade: `KeyMetadata`

| Campo         | Tipo      | Obrigatório | Descrição                                                        |
|---------------|-----------|-------------|--------------------------------------------------------------------|
| id            | uuid      | sim         | Identificador único do registro                                   |
| fingerprint   | string    | sim         | Fingerprint SHA256 da chave pública — usado para associar o registro à chave real em disco |
| keyPath       | string    | sim         | Caminho do arquivo de chave privada (ex. `~/.ssh/id_ed25519`)      |
| label         | string    | não         | Apelido definido pelo usuário (ex. "GitHub pessoal")               |
| algorithm     | string    | sim         | `ed25519`, `rsa`, etc.                                            |
| createdAt     | datetime  | sim         | Data de criação da chave (capturada no momento da geração pelo app; se importada, definida manualmente pelo usuário) |
| expiresAt     | datetime  | não         | Data de expiração definida pelo usuário — se ausente, chave sem validade controlada |
| notes         | string    | não         | Anotações livres do usuário                                        |

> `lastUsedAt` foi avaliado e **removido do MVP** (decisão de 2026-08-03) — ver seção 8.

### Entidade: `AppSettings`

| Campo              | Tipo   | Obrigatório | Descrição                                          |
|--------------------|--------|-------------|------------------------------------------------------|
| alertThresholdDays | int    | sim         | Quantos dias antes do `expiresAt` o app deve alertar (padrão: 30) |
| defaultAlgorithm   | string | sim         | Algoritmo padrão sugerido ao gerar chave (padrão: `ed25519`) |

> A associação entre `KeyMetadata` e a chave real em disco é feita pelo `fingerprint`, não pelo
> caminho isoladamente — isso evita que o registro fique órfão se o usuário renomear ou mover o
> arquivo, desde que o app consiga re-escanear e reconciliar pelo fingerprint.

---

## 5. Funcionalidades do MVP

### 5.1 Leitura/parsing do `~/.ssh/config`

1. O sistema deve parsear `~/.ssh/config` respeitando a sintaxe padrão do OpenSSH, seguindo
   diretivas `Include` **recursivamente por padrão** (decisão de 2026-08-03), replicando o
   comportamento real do próprio OpenSSH — a visão exibida pelo app deve corresponder exatamente
   à configuração que o `ssh` de fato usa, sem exigir opt-in do usuário para isso.
2. O sistema deve listar todos os hosts configurados, com suas opções (`HostName`, `User`,
   `Port`, `IdentityFile`, etc.).
3. O sistema deve identificar, para cada host, qual(is) chave(s) de identidade estão associadas
   (via `IdentityFile`).
4. O sistema deve tratar o arquivo como somente leitura por padrão ao listar — qualquer escrita
   (ex. adicionar/editar host) deve ser uma ação explícita do usuário. **Implementado** via
   `ConfigService.AddHost(path, spec)` e `ConfigService.ReplaceHost(path, oldPatterns, newSpec)`
   (decisão de 2026-08-03) — ambos escopados a um único arquivo (nunca atravessam `Include`
   procurando um host em outro lugar), com escrita atômica (arquivo temporário + `os.Rename`,
   mesmo padrão de `metadata.json`, extraído para um helper compartilhado
   `atomicWriteFile` em `core/atomicfile.go`).
   - `AddHost` insere um bloco novo, inserindo-o antes de um bloco catch-all `Host *` final
     quando ele existir (para que o novo host tenha precedência, respeitando a semântica
     "primeiro valor obtido vence" do próprio ssh_config); erra se já existir, no mesmo arquivo,
     um bloco com os mesmos `Patterns` (sem upsert — use `ReplaceHost` para isso).
   - `ReplaceHost` localiza um bloco existente por igualdade exata de `Patterns` e o substitui
     inteiro; erra se não encontrar (sem criar).
   - Se `path` apontar para um arquivo ainda não referenciado por nenhuma diretiva `Include`, ele
     é criado mesmo assim e fica **órfão** (sem efeito no `ssh` real) até algo garantir esse
     `Include` — `AddHost`/`ReplaceHost` são primitivas de baixo nível, não gerenciam `Include`.
   - `IdentityFile` é compactado de volta para `~/` quando o caminho estiver sob o home atual
     (`compactHome`, inverso de `expandHome`), para não gravar um caminho absoluto específico
     desta máquina — importante para portabilidade (ex. um backup restaurado em outra máquina).
5. O sistema deve preservar comentários e formatação original do arquivo ao fazer edições
   (não reescrever o arquivo inteiro de forma destrutiva). **Trade-off aceito** (decisão de
   2026-08-03, após investigar a biblioteca `kevinburke/ssh_config`): blocos `Host` **não
   tocados** por uma operação de escrita são preservados byte-a-byte (a lib mantém o texto
   original de cada linha internamente). Um bloco **adicionado ou substituído**, porém, usa
   formatação limpa e padronizada (sem indentação customizada, sem preservar comentários
   próprios daquele bloco) — a biblioteca não permite editar um node já existente campo a campo
   preservando o resto do bloco (o valor bruto original sempre vence na serialização, sem setter
   público para alterar isso). Editar um único campo dentro de um bloco existente, preservando
   tudo mais nesse bloco, fica fora de escopo — exigiria um parser/serializador próprio. Um
   efeito colateral observado na prática: um comentário que precede visualmente um bloco
   catch-all `Host *` pode ficar associado, na verdade, ao final do bloco anterior no arquivo —
   ao inserir um novo host antes do catch-all, esse comentário pode passar a aparecer acima do
   host novo em vez do `Host *` a que originalmente se referia. Cosmético (não afeta o
   comportamento do `ssh`), mas vale saber.

### 5.2 Geração de novas chaves SSH

1. O sistema deve gerar chaves ed25519 por padrão, com opção de escolher RSA (mínimo 4096 bits)
   quando necessário por compatibilidade.
2. O sistema deve permitir definir passphrase na geração (recomendado por padrão, nunca
   obrigatório para não travar automação).
3. O sistema deve salvar a chave gerada em `~/.ssh/` seguindo a convenção de nomes do OpenSSH.
4. O sistema deve, ao gerar uma chave, criar automaticamente um registro correspondente em
   `KeyMetadata` (com `createdAt` preenchido).
5. O sistema deve aplicar permissões de arquivo corretas na criação (ver seção 6).
6. O sistema pode oferecer, opcionalmente, associar a nova chave a um host existente ou novo no
   `~/.ssh/config` durante o mesmo fluxo. **Fora desta leva do `KeyService`** (decisão de
   2026-08-03) — depende de `ConfigService` ganhar uma API de escrita preservando formatação
   (spec 5.1.5), que ainda não existe.

### 5.3 Gerenciamento de expiração/rotação de chaves

1. O sistema deve permitir ao usuário definir (ou editar) a data de expiração de uma chave
   existente. **Implementado via `KeyService.UpdateMetadata(fingerprint, patch)`** (decisão de
   2026-08-03) — método único e genérico cobrindo `label`/`notes`/`expiresAt`, em vez de um
   `SetExpiration` dedicado + setters separados. `patch.ExpiresAt` usa `**time.Time` para
   distinguir "não alterar" (nil) de "remover expiração" (ponteiro para nil).
2. O sistema deve escanear as chaves em `~/.ssh/` e reconciliar com os registros de
   `KeyMetadata` existentes (por fingerprint), sinalizando chaves sem metadata associada.
3. O sistema deve listar chaves ordenadas por proximidade da expiração.
4. O sistema deve alertar (CLI: código de saída/mensagem; TUI/GUI: destaque visual) chaves que
   expiram dentro do `alertThresholdDays` configurado, e chaves já expiradas.
5. O sistema deve oferecer um fluxo de "rotação": gerar uma nova chave, e opcionalmente marcar a
   antiga como substituída (sem excluir automaticamente a chave antiga do disco).
   **Decisão de 2026-08-03: sem método `RotateKey` dedicado** — a UI compõe `GenerateKey` +
   `UpdateMetadata` (ex. anotando "substitui X" em `notes`), sem persistir um vínculo formal.
   Evita estender o schema de `KeyMetadata` (que não tem campo tipo `supersededBy`/`replaces`)
   depois de já ter sido tratado como decidido.
6. O sistema não deve excluir chaves do disco automaticamente em nenhuma circunstância — exclusão
   é sempre ação explícita e confirmada pelo usuário.

### 5.4 Backup e export/import de configurações

**Implementado** (decisão de 2026-08-03) via `BackupService` (`core/backup.go`,
`backup_manifest.go`, `backup_archive.go`, `backup_export.go`, `backup_import.go`), com
granularidade **por host** — reaproveita `ConfigService.AddHost`/`ReplaceHost` (seção 5.1.4) em
vez de copiar arquivos de config inteiros. Pacote é um `.tar.gz` (`manifest.json` + `keys/<nome>`
+ `keys/<nome>.pub`), montado via stdlib (`archive/tar`, `compress/gzip`).

1. O sistema deve permitir exportar `~/.ssh/config`, as chaves selecionadas e a metadata
   associada para um pacote único. `ExportOptions.Hosts []Host` seleciona hosts explicitamente
   (chamador filtra a partir de `ListHosts()`); `Keys []KeySelection{Fingerprint,
   IncludePrivate}` seleciona chaves — as duas dimensões (quais chaves entram, e se o material
   privado de cada uma entra) são ortogonais.
   `SourceFile`/`IdentityFile` de cada host são gravados no manifest já passados por
   `compactHome()` na máquina de origem (`~/...` quando sob o home de origem, path absoluto
   intacto caso contrário — decisão de 2026-08-03: incluir Include externo com o path absoluto
   original, não pular). No import, são reidratados com `expandHome()` contra o home da máquina
   de **destino**, nunca da origem — testado explicitamente (roundtrip entre "duas máquinas"
   simuladas via `HOME`/`USERPROFILE` diferentes).
2. O sistema deve, por padrão, **não** incluir chaves privadas sem confirmação explícita do
   usuário no momento do export (ver seção 6). Quando incluídas, saem em **texto claro dentro do
   pacote** (decisão de 2026-08-03: sem criptografia embutida no MVP) — a proteção do arquivo
   fica sob responsabilidade do usuário.
3. O sistema deve permitir importar um pacote de backup gerado por ele mesmo, restaurando config,
   chaves e metadata. Fluxo de duas etapas: `PreviewImport(srcPath)` classifica cada host/chave
   sem escrever nada (`HostsToAdd`/`HostsUnchanged`/`HostConflicts`,
   `KeysToAdd`/`KeysUnchanged`/`KeyConflicts`); `Import(srcPath, resolutions)` aplica, usando
   `ImportResolutions` para decidir cada conflito — e também para cherry-pick (excluir itens sem
   conflito do import, decisão de 2026-08-03).
4. O sistema deve detectar conflitos na importação (ex. host ou chave já existente) e pedir
   confirmação antes de sobrescrever. Três tipos de conflito implementados:
   - **Host divergente ou fora do home do usuário atual**: nunca aplicado automaticamente —
     mesmo um host cujo destino resolvido caia fora do home (`ExternalPath=true`) exige
     `HostResolutionApply` explícito, mesmo sem conflito de conteúdo.
   - **Chave já registrada localmente** (mesmo fingerprint): `Overwrite` atualiza
     `Label`/`Notes`/`ExpiresAt`/`Algorithm` do pacote, mas **mantém `ID`/`KeyPath`/`CreatedAt`
     locais** (decisão de 2026-08-03 — `CreatedAt` é um fato sobre a chave, não sobre o evento de
     import); o arquivo em disco nunca é reescrito (mesmo fingerprint = mesmo material).
   - **Colisão de nome de arquivo com chave de fingerprint diferente**: três resoluções — Skip,
     Overwrite, ou **importar sob nome alternativo** (`<nome>.imported-<hex curto do
     fingerprint>`, decisão de 2026-08-03), preservando as duas chaves.
   Um `sourceFile` que alega portabilidade (`~/...`) mas resolve fora do home atual ao expandir é
   tratado como **rejeição dura do pacote inteiro** (sinal de manifest adulterado), não como mais
   um conflito a confirmar. O fingerprint de cada chave é sempre reconferido a partir do `.pub`
   extraído do pacote contra o que o manifest declara — erro se não bater.
5. O sistema pode oferecer export apenas da metadata (sem chaves) para casos de sincronização
   leve entre máquinas onde as chaves já existem localmente. `IncludePrivate=false` em todas as
   `KeySelection` cobre esse caso — nenhum arquivo de chave entra no pacote, só o registro; no
   import, a chave aparece como `KeyStatusMissingFile` até o arquivo existir localmente (reusa a
   reconciliação já existente do `KeyService`, sem estado novo).

---

## 6. Considerações de segurança

- **Permissões de arquivo:** chaves privadas geradas devem ser criadas com permissão `0600`
  (Linux/macOS) e equivalente restrito no Windows (ACL apenas para o usuário atual), replicando
  o comportamento esperado pelo próprio OpenSSH.
  **Decisão de 2026-08-03 sobre o gap no Windows**: `os.WriteFile` com `0600` no Windows só
  alterna o atributo read-only do arquivo — não restringe acesso via ACL de fato (diferente de
  Linux/macOS, onde os bits POSIX funcionam como esperado). Avaliamos três opções: (a) aceitar o
  gap documentado, dependendo da ACL herdada de `%USERPROFILE%\.ssh`; (b) implementar ACL
  manualmente via `golang.org/x/sys/windows`; (c) usar `github.com/hectane/go-acl` (manutenção
  mínima, último commit relevante ~2023). **Escolhido (a)** — mesmo padrão de hardening adiado já
  usado para backup sem criptografia. Fica como ponto a revisitar antes de um v1.0 (seção 7).
- **Metadata não contém segredos:** o arquivo `metadata.json` nunca armazena a chave privada em
  si, apenas fingerprint e informações descritivas — pode ser tratado com sensibilidade menor
  que `~/.ssh/`, mas ainda deve ter permissões restritas ao usuário.
- **Concorrência em `metadata.json`:** CLI, TUI e GUI podem rodar como processos separados ao
  mesmo tempo. **Decisão de 2026-08-03: sem lock de arquivo no MVP** — escrita concorrente pode
  corromper o arquivo (mitigado parcialmente pela escrita atômica via `os.Rename`, que evita
  truncamento, mas não evita uma escrita sobrescrever a outra). Risco aceito por ora, documentado
  como limitação conhecida; uso típico é uma interface por vez.
- **Export/backup:**
  - **Decisão de 2026-08-03**: o MVP **não** criptografa o pacote de backup — chaves privadas
    incluídas saem em texto claro dentro do `.tar.gz`/`.zip`. Escopo reduzido conscientemente;
    criptografia (candidato natural: [age](https://age-encryption.org/), por ser biblioteca Go
    pura, sem binário externo) fica para uma versão futura — ver seção 8.
  - O sistema deve exibir um aviso explícito e de alto contraste antes de incluir chaves privadas
    em um export, deixando claro que o arquivo resultante não é protegido e orientando o usuário
    a armazená-lo com segurança (ex. criptografar externamente, não deixar em sync de nuvem sem
    proteção adicional).
- **Import/backup como entrada não confiável:** um pacote `.tar.gz` pode ter sido editado à mão
  antes de importado. `BackupService.Import`/`PreviewImport` (implementado 2026-08-03) tratam
  isso com várias camadas: extração rejeita qualquer entrada de tar com nome absoluto, com `..`,
  ou que não seja arquivo regular/diretório (proteção clássica contra "tar slip"); nome de
  arquivo de chave (`fileName`) é validado como nome puro, sem diretórios; fingerprint de cada
  chave é reconferido a partir do `.pub` extraído; um `sourceFile` de host que alega portabilidade
  (`~/...`) mas escapa do home ao expandir rejeita o pacote inteiro (ver seção 5.4.4).
- **Sem transmissão externa:** nenhuma chave, metadata ou configuração é enviada para servidores
  externos — toda a operação é local ao dispositivo do usuário (alinhado à natureza da
  ferramenta, sem backend).
- **Confirmação para ações destrutivas:** qualquer ação que sobrescreva ou remova arquivos
  existentes (`~/.ssh/config`, chaves) deve pedir confirmação explícita, com preview do que será
  alterado quando possível.

---

## 7. Pontos em aberto

- [x] ~~Escolha final entre Wails e Fyne para a GUI~~ — **decidido em 2026-08-03: Wails**, com
  isolamento obrigatório em `internal/adapter/gui` (ver seção 3.3) para mitigar o risco de
  bus factor do projeto e de fragmentação do WebKitGTK no Linux.
- [x] ~~Nomes finais das interfaces públicas do `core`~~ — **decidido em 2026-08-03**:
  `ConfigService` (parsing/escrita do `~/.ssh/config`), `KeyService` (geração, escaneamento e
  expiração/rotação de chaves) e `BackupService` (export/import). Sem esquema de versionamento
  dedicado — são API interna do módulo `core`, evoluem como qualquer pacote Go normal (mudança
  breaking = entrada no CHANGELOG, sem SemVer separado enquanto o projeto estiver pré-v1.0.0).
- [x] ~~Ferramenta para a verificação de fronteira~~ — **decidido em 2026-08-03**: `depguard`
  via `golangci-lint`, com regra bloqueando `github.com/wailsapp/wails` fora de
  `internal/adapter/gui/**`, rodando no CI.
- [x] ~~Mecanismo de criptografia do pacote de backup~~ — **decidido em 2026-08-03: nenhum no
  MVP**, exporta em texto claro com aviso explícito. Encriptação (via `age`) fica para versão
  futura (seção 8). Formato do arquivo (`.tar.gz` vs `.zip`) segue como detalhe de implementação
  sem impacto arquitetural, escolha livre na hora de codar.
- [x] ~~Estratégia de detecção de `lastUsedAt`~~ — **decidido em 2026-08-03: fora do MVP**, campo
  removido do schema de `KeyMetadata` por ora (seção 8).
- [x] ~~Nome definitivo do projeto/binário~~ — **decidido em 2026-08-03: `keyward`**.
- [x] ~~Se `Include` deve ser seguido recursivamente por padrão~~ — **decidido em 2026-08-03:
  sim, sempre**, para que a visão do app corresponda ao comportamento real do OpenSSH.
- [x] ~~Gap de ACL no Windows para chaves privadas~~ — **decidido em 2026-08-03: aceitar o gap
  por ora**, documentado na seção 6. Revisitar antes de um v1.0.
- [x] ~~`RotateKey`: método dedicado ou composição?~~ — **decidido em 2026-08-03: composição**
  (`GenerateKey` + `UpdateMetadata`), sem novo método nem campo em `KeyMetadata` (seção 5.3.5).
- [x] ~~Formato da API de edição de metadata de uma chave~~ — **decidido em 2026-08-03:
  `UpdateMetadata(fingerprint, patch)` genérico** (seção 5.3.1).
- [x] ~~Proteger `metadata.json` contra escrita concorrente entre CLI/TUI/GUI?~~ — **decidido em
  2026-08-03: adiar**, documentado na seção 6 como limitação conhecida.
- [x] ~~API de escrita do `ConfigService`~~ — **decidido/implementado em 2026-08-03**:
  `AddHost`/`ReplaceHost` (seção 5.1.4), escopados a um único arquivo, sem upsert. Trade-off
  aceito: blocos não tocados preservados byte-a-byte, bloco adicionado/substituído sai com
  formatação limpa (a lib `kevinburke/ssh_config` não permite editar um node existente
  preservando o resto do bloco — ver seção 5.1.5).
- [x] ~~Granularidade do `BackupService` (por host vs. blob de arquivo)~~ — **decidido em
  2026-08-03: por host**, usando `AddHost`/`ReplaceHost` como primitivas — motivo direto de ter
  implementado a escrita do `ConfigService` antes deste passo.
- [x] ~~Includes fora de `~/.ssh/` no backup~~ — **decidido em 2026-08-03: incluir com o path
  absoluto original**, não pular. Import trata como entrada não confiável (seção 6).
- [x] ~~Colisão de nome de arquivo de chave no import~~ — **decidido em 2026-08-03**: além de
  Skip/Overwrite, oferece **importar sob nome alternativo**, preservando as duas chaves.
- [ ] Estratégia de empacotamento/distribuição por plataforma (ex. instalador MSI no Windows,
  pacote `.deb`/`.rpm`/AUR no Linux, `.dmg`/Homebrew no macOS) — **adiado deliberadamente**: só
  faz sentido decidir com um binário funcional em mãos; entra na lista de pontos em aberto de
  novo quando o MVP estiver implementado.

---

## 8. Fora de escopo (v1)

- Certificados SSH assinados por CA (expiração nativa via protocolo) — possível evolução futura,
  mencionada aqui apenas como direção, não como requisito do MVP.
- Sincronização em nuvem entre múltiplas máquinas.
- Gerenciamento de `known_hosts`.
- Integração direta com `ssh-agent` para carregamento automático de chaves (pode ser considerado
  em versão futura, já que a biblioteca `golang.org/x/crypto/ssh/agent` dá suporte a isso).
- Suporte a hardware tokens (YubiKey, FIDO2/U2F) para chaves SSH.
- Multiusuário ou qualquer forma de compartilhamento de configuração entre contas/pessoas.
- Criptografia do pacote de backup — exportação de chaves privadas sai em texto claro no MVP,
  com aviso explícito ao usuário. Candidato natural para versão futura: `age`.
- Campo `lastUsedAt` em `KeyMetadata` (última vez que uma chave foi usada) — detecção confiável
  exigiria parsing de logs específicos por SO (journald, Event Log, `auth.log`), frágil entre
  versões/distros e por vezes dependente de permissão elevada. Fica fora do MVP.
