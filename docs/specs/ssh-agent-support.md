# Spec: Suporte a ssh-agent (incluindo 1Password SSH Agent)

**Status:** Rascunho
**Data:** 2026-08-04
**Autor:** —

> Complementa `docs/specs/ssh-config-manager.md`. Não substitui nenhuma decisão lá registrada;
> estende o modelo de `Key` e `KeyMetadata` para cobrir chaves cuja origem não é um par de
> arquivos em `~/.ssh/`.

---

## 1. Objetivo

Hoje o `keyward` assume que toda chave é um par de arquivos em disco (`IdentityFile` +
`.pub`), gerenciado por `KeyService`/`metadata.json`. Isso deixa de fora um caso comum: chaves
cujo material privado nunca toca o disco do usuário e são oferecidas via **protocolo
ssh-agent** — seja o `ssh-agent` do OpenSSH com chaves carregadas manualmente, seja o
**1Password SSH Agent** (que expõe todas as chaves guardadas no cofre via um socket próprio),
seja um token de hardware (YubiKey/FIDO2) carregado num agente via `pkcs11`.

Esta spec cobre a extensão do `core` para **listar e catalogar** chaves de agente lado a lado
com chaves de arquivo, e como as três interfaces (CLI/TUI/GUI) exibem essa origem. Não cobre
autenticação de fato via agente durante uma conexão SSH — isso já é responsabilidade do
cliente `ssh` do sistema operacional, fora do escopo do `keyward`.

---

## 2. Requisitos funcionais

1. O `core` deve detectar a presença de um agente ssh ativo a partir da variável de ambiente
   `SSH_AUTH_SOCK` (Unix/macOS) e, no Windows, do named pipe `\\.\pipe\openssh-ssh-agent` (ou o
   pipe configurado, ver seção 7).
2. O `core` deve, quando um agente estiver acessível, listar as chaves atualmente carregadas
   nele via protocolo ssh-agent (`golang.org/x/crypto/ssh/agent`, `List()`), obtendo
   fingerprint, algoritmo e comentário de cada identidade.
3. O sistema deve unificar, em `ListKeys()`, chaves de arquivo (já existente) e chaves de
   agente numa única lista de `Key`, distinguíveis por um campo de origem (`Source`).
4. O usuário pode anotar uma chave de agente com `Label`/`Notes` da mesma forma que uma chave
   de arquivo, persistido em `metadata.json` por fingerprint — mesmo mecanismo de reconciliação
   já usado para `KeyStatusUnregistered`.
5. O sistema deve permitir vincular um host a uma chave de agente **sem exigir
   `IdentityFile`** — o vínculo é informativo, persistido numa nova entidade `HostMetadata`
   (seção 4), já que o OpenSSH oferece automaticamente todas as identidades do agente a menos
   que `IdentityFile`/`IdentitiesOnly` restrinjam.
6. Opcionalmente, o sistema pode escrever `IdentityFile <caminho-do-.pub>` no bloco `Host`
   quando o usuário quiser restringir explicitamente qual identidade do agente usar para aquele
   host — o OpenSSH aceita apontar `IdentityFile` para o `.pub` de uma chave sem a privada em
   disco, casando por fingerprint com o que o agente oferece.
7. O sistema deve identificar, quando possível, se o agente é o **1Password SSH Agent**
   especificamente (heurística: caminho conhecido do socket, ex.
   `~/.1password/agent.sock` no Linux/macOS ou `\\.\pipe\openssh-ssh-agent` reaproveitado pelo
   1Password no Windows/WSL) e rotular a origem como `agent:1password` em vez de `agent:generic`
   para exibição mais clara na UI.
8. Ações que dependem de material privado em disco — rotação (`GenerateKey` + substituição),
   `UpdateMetadata` de campos de expiração usados para alerta de rotação, remoção de arquivo —
   devem ficar indisponíveis/desabilitadas para chaves de origem `agent`. `Unregister` (remover
   só o registro de metadata) continua válido.
9. `ListKeys()` deve seguir funcionando normalmente (sem erro) quando nenhum agente estiver
   acessível — chaves de agente simplesmente não aparecem na lista, sem afetar chaves de
   arquivo.

---

## 3. Requisitos não-funcionais

- **Sem persistência de segredo**: o `core` nunca lê nem grava material de chave privada de
  origem `agent` — só metadados (fingerprint, algoritmo, comentário), obtidos via `List()` do
  protocolo ssh-agent, que não expõe a chave privada.
- **Timeout de conexão ao socket/pipe**: a consulta ao agente deve ter timeout curto (sugestão:
  500ms) para não travar `ListKeys()` caso o socket exista mas esteja com processo morto do
  outro lado.
- **Multiplataforma**: detecção de socket precisa cobrir Linux/macOS (`SSH_AUTH_SOCK`) e Windows
  (named pipe), coerente com o suporte multiplataforma já existente no projeto.
- **Falha graciosa**: erro ao conectar/consultar o agente é tratado como "nenhuma chave de
  agente disponível", nunca propagado como erro fatal de `ListKeys()` — mesmo padrão de
  tolerância já usado para "arquivo ausente = estado vazio" em `metadataStore.load`.
- **Sem lock de concorrência adicional**: reaproveita a mesma decisão já aceita na spec
  principal (seção 6/7) de não haver lock entre processos em `metadata.json`.

---

## 4. Modelo de dados

### Extensão de `Key` (`core/keys.go`)

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| Source | `KeySource` | sim | `KeySourceFile` (atual, default) ou `KeySourceAgent` |
| AgentName | string | não | Preenchido só quando `Source == KeySourceAgent`. Ex.: `"1password"`, `"openssh"`, `""` (agente não identificado) |

```go
type KeySource int

const (
    KeySourceFile  KeySource = iota // comportamento atual, inalterado
    KeySourceAgent                  // chave só existe carregada num ssh-agent
)
```

- `PublicKeyPath`/`PrivateKeyPath` ficam vazios (`""`) para `Source == KeySourceAgent` — não há
  arquivo. `Comment` continua preenchido (vem do agente).
- `KeyBits` é obtido quando o protocolo do agente expuser (varia por tipo de chave); pode ficar
  `0` se indisponível.
- `Status` ganha um quarto valor, `KeyStatusAgentOffline`, específico de `Source ==
  KeySourceAgent`: registro existe em `metadata.json` mas a identidade não está (mais) sendo
  oferecida pelo agente no momento da consulta — análogo a `KeyStatusMissingFile`, mas sem
  implicar arquivo ausente. Continua editável/removível nesse estado (só não há verificação de
  fingerprint contra um agente ativo). `KeyStatusUnregistered` continua valendo para uma
  identidade oferecida pelo agente sem registro correspondente em `metadata.json`.

### Extensão de `KeyMetadata` (`core/metadata.go`)

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| Source | `KeySource` | sim (novo) | Espelha `Key.Source` no registro persistido — necessário para a UI saber, mesmo sem o agente ativo no momento, que aquele registro é "gerenciado externamente" |
| AgentName | string | não | Espelha `Key.AgentName` |

- `KeyPath` continua existindo no struct por compatibilidade, mas fica vazio para registros com
  `Source == KeySourceAgent`.
- **Migração de `metadata.json`**: registros existentes não têm o campo `source` — na leitura
  (`metadataStore.load`), ausência do campo é tratada como `KeySourceFile` (default do tipo
  zero value), sem necessidade de bump de `metadataSchemaVersion`.

### Novo tipo: detecção de agente

```go
// AgentInfo descreve um agente ssh acessível no momento da consulta. Exposto
// via KeyService.DetectAgent() — shape reconciliado com o plano técnico
// (docs internos de implementação) para incluir Detected explicitamente,
// distinguindo "sem agente" de "agente presente sem chaves oferecidas".
type AgentInfo struct {
    Detected   bool
    SocketPath string    // caminho do socket Unix ou nome do pipe Windows; "" se Detected == false
    Name       string    // "1password", "openssh", "" se não identificado
}
```

Não é persistido — é resultado de uma consulta ao ambiente em tempo de execução, recalculado a
cada chamada de `DetectAgent()`, sempre dialando de novo (nunca reaproveitando a conexão aberta
por `ListKeys()`, mesmo quando ambos são chamados na mesma operação da UI — ver plano técnico).

### Novo tipo persistido: `HostMetadata`

Hoje `Host`/`HostSpec` descrevem só o bloco literal do `~/.ssh/config`, sem metadata própria.
Para vincular um host a uma chave de agente sem escrever `IdentityFile`, introduzimos uma nova
entidade persistida em `metadata.json`, no mesmo padrão de `KeyMetadata` (chave de reconciliação
estável, não o caminho em si):

```go
// HostMetadata associa um bloco Host do ~/.ssh/config a uma chave de agente,
// de forma puramente informativa — não afeta o que é escrito no arquivo de
// config. Reconciliado por HostKey (ver abaixo), não por índice/posição.
type HostMetadata struct {
    HostKey           string `json:"hostKey"`           // ver regra de identidade abaixo
    AgentKeyFingerprint string `json:"agentKeyFingerprint"`
    Notes             string `json:"notes,omitempty"`
}
```

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| HostKey | string | sim | Identidade estável do host — `strings.Join(Patterns, "\x00")` do bloco `Host`, já que `Host` não tem ID próprio (spec principal não introduz um). Se os `Patterns` do bloco mudarem, o vínculo se perde (mesma limitação que renomear um `Host` já tem hoje para qualquer outra anotação futura) |
| AgentKeyFingerprint | string | sim | Fingerprint da chave de agente vinculada — mesma chave usada para reconciliar `KeyMetadata` |
| Notes | string | não | Anotação livre do usuário sobre o vínculo |

- Persistido em `metadataFile.Hosts []HostMetadata`, ao lado de `Keys`. **Bump de
  `metadataSchemaVersion` não é necessário** — é um campo novo (`hosts`), lido como slice vazio
  quando ausente, mesmo padrão de tolerância já usado para `Settings`.
- `ConfigService` (spec principal) permanece inalterado — `HostMetadata` vive inteiramente do
  lado de `KeyService`/`metadata.json`, nunca é escrito no `~/.ssh/config`.
- Remoção: quando um `HostMetadata` referencia um `HostKey` que não corresponde a nenhum host
  atual (renomeado/removido) ou um `AgentKeyFingerprint` sem chave correspondente, o vínculo é
  tratado como órfão — exposto na UI para o usuário limpar manualmente (mesmo espírito de
  `KeyStatusMissingFile`), nunca removido silenciosamente.

---

## 5. Fluxos do usuário

### Fluxo principal: listar chaves com agente ativo

1. Usuário abre a lista de chaves (CLI `keyward key list`, TUI ou GUI).
2. `core` detecta `SSH_AUTH_SOCK`/pipe, conecta via `golang.org/x/crypto/ssh/agent`, chama
   `List()`.
3. Para cada identidade retornada, `core` calcula o fingerprint e reconcilia com
   `metadata.json` pelo mesmo mecanismo já usado para chaves de arquivo.
4. `core` também identifica o agente (heurística de socket) e marca `AgentName`.
5. Resultado final combina chaves de arquivo + chaves de agente, mas em **dois grupos
   separados** na UI: chaves de arquivo ordenadas por proximidade de expiração (como hoje), e
   chaves de agente numa seção própria (sem participar dessa ordenação, já que `ExpiresAt` nelas
   não tem o mesmo significado de "arquivo esquecido em disco"). `ListKeys()` no `core` pode
   retornar a lista unificada; a separação visual é responsabilidade da camada de UI (CLI
   também pode listar em duas seções ou usar `--source` para filtrar).
6. UI exibe cada chave de agente com um indicador visual de origem (ex.: badge "🔑 1Password" /
   "🔑 Agente SSH") e oculta ações de rotação/exclusão de arquivo para elas.

### Fluxo alternativo: nenhum agente acessível

- Se `SSH_AUTH_SOCK`/pipe não estiver definido, ou a conexão falhar, `ListKeys()` retorna
  normalmente só com chaves de arquivo — nenhum erro, nenhum aviso bloqueante. Um aviso
  discreto ("nenhum agente SSH detectado") pode aparecer na TUI/GUI, mas não na CLI (que deve
  manter saída limpa para scripting).

### Fluxo alternativo: anotar uma chave de agente

1. Usuário seleciona uma chave de origem `agent` sem registro (`KeyStatusUnregistered`) na
   lista.
2. Usuário preenche `Label`/`Notes` (não há `ExpiresAt` com o mesmo significado de "rotacionar
   arquivo", mas o campo continua disponível para o usuário anotar validade combinada
   externamente, ex. rotação no 1Password).
3. `core` cria o registro via `RegisterKey`-equivalente, com `Source = KeySourceAgent` e
   `KeyPath = ""`.

### Fluxo alternativo: vincular host a uma chave de agente

1. Usuário edita um host na TUI/GUI e escolhe, na lista de chaves disponíveis, uma chave de
   origem `agent`.
2. Por padrão o `keyward` **não escreve `IdentityFile`** no bloco `Host` — a vinculação é
   persistida como um registro `HostMetadata` (`HostKey` = patterns do host, `AgentKeyFingerprint`
   = fingerprint da chave), explicando na UI que a identidade é oferecida automaticamente pelo
   agente, sem alterar o `~/.ssh/config`.
3. Se o usuário pedir explicitamente para restringir (ex. múltiplas chaves no agente e quer
   forçar uma específica para aquele host), o `keyward` escreve `IdentityFile <path-do-.pub>` +
   `IdentitiesOnly yes` no bloco `Host` (via `ConfigService`, inalterado), **além de** manter o
   `HostMetadata` para exibição consistente na UI.
4. Se o usuário renomear os `Patterns` do host depois de vinculado, o `HostMetadata` correspondente
   fica órfão (nenhum `Host` atual casa com o `HostKey` salvo) — a UI sinaliza isso e oferece
   remover o vínculo, mas não tenta adivinhar o host renomeado automaticamente.

---

## 6. Critérios de aceite

- [ ] Dado um agente ssh-agent do OpenSSH rodando com uma chave carregada, quando o usuário
  roda `keyward key list`, então a chave aparece na lista com origem "agente" e sem
  `PrivateKeyPath`.
- [ ] Dado o 1Password SSH Agent ativo com chaves no cofre, quando `ListKeys()` é chamado,
  então as chaves aparecem com `AgentName == "1password"`.
- [ ] Dado nenhum agente configurado no ambiente, quando `ListKeys()` é chamado, então o
  retorno contém só chaves de arquivo, sem erro.
- [ ] Dado um registro de metadata com `Source == KeySourceAgent` cuja chave não está mais
  carregada no agente no momento da consulta, quando `ListKeys()` é chamado, então a chave
  aparece com `Status == KeyStatusAgentOffline`, continua editável/removível.
- [ ] Dado um `metadata.json` gravado antes desta feature (sem campo `source`), quando lido por
  uma versão do `keyward` com esta feature, então todos os registros existentes são
  interpretados como `KeySourceFile`, sem erro de parsing e sem necessidade de migração manual.
- [ ] Dado uma chave de origem `agent`, quando o usuário tenta uma ação de rotação/remoção de
  arquivo na CLI/TUI/GUI, então a ação é rejeitada ou fica indisponível na UI, com mensagem
  explicando que a chave é gerenciada externamente.
- [ ] Dado um host vinculado via `HostMetadata` a uma chave de agente, quando o usuário
  visualiza o host na TUI/GUI, então fica visualmente claro que a identidade não depende de
  `IdentityFile` local.
- [ ] Dado um `HostMetadata` cujo `HostKey` não corresponde a nenhum host atual (renomeado ou
  removido), quando o usuário visualiza a lista de hosts, então o vínculo órfão é sinalizado e
  pode ser removido pelo usuário, sem remoção automática silenciosa.
- [ ] Dado `keyward key list --source=agent`, quando executado, então a saída contém só chaves
  de origem agente; `keyward key list` sem a flag mantém a saída atual mais as chaves de agente
  (formato de agrupamento a definir no plano técnico).

---

## 7. Pontos em aberto

- [ ] **Detecção do 1Password especificamente**: a heurística de caminho de socket é frágil
  entre versões do 1Password e SOs (o caminho já mudou entre versões da ferramenta
  historicamente). Vale a pena tentar identificar via alguma extensão do protocolo, ou aceitar
  rótulo genérico "Agente SSH" quando a heurística falhar? (Decisão de produto: fallback
  genérico é aceitável — falta só confirmar a lista de caminhos conhecidos no plano técnico.)
- [ ] **Windows**: confirmar se `golang.org/x/crypto/ssh/agent` funciona diretamente sobre o
  named pipe do OpenSSH for Windows, ou se é necessária uma dependência adicional (ex.
  `Microsoft/go-winio`) para comunicação via named pipe nesse SO — questão técnica de
  viabilidade, para o plano técnico investigar.
- [ ] **Formato exato do agrupamento na CLI**: `keyward key list` sem flag deve imprimir duas
  seções (arquivo/agente) num único comando, ou a listagem de agente só aparece com
  `--source=agent` explícito? Definir no plano técnico junto com o formato de tabela.

---

## 8. Fora de escopo (v1)

- Autenticação de fato via agente durante uma conexão SSH — segue sendo responsabilidade do
  cliente `ssh` do sistema operacional.
- Suporte a hardware keys FIDO2/YubiKey nativas do OpenSSH (`ssh-keygen -t ed25519-sk`) fora do
  protocolo ssh-agent — tratado como spec separada.
- Adicionar/remover chaves de um agente (`ssh-add`/`ssh-add -d`) a partir do `keyward` — esta
  spec cobre só leitura/catalogação, não gestão do ciclo de vida do agente em si.
- Suporte a múltiplos agentes simultâneos (ex. OpenSSH local + 1Password ao mesmo tempo) — v1
  assume um agente por vez, resolvido pela variável/pipe padrão do ambiente.
- Sincronizar/atualizar automaticamente `HostMetadata` quando os `Patterns` de um host mudam —
  fica como vínculo órfão explícito para o usuário resolver (seção 5), não há tentativa de
  auto-relink.
