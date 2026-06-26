# Manual técnico do Peapod 🫛

> Referência técnica completa. Para uma introdução sem jargão, veja o guia
> **"Para todos"** (na mesma janela do app, ou em [docs/GUIA.md](GUIA.md)).

## Arquitetura

Tudo se apoia num núcleo fino (`Manager`) sobre uma interface `Driver` trocável.
As quatro superfícies de uso (MCP, CLI, dashboard web e app nativo) falam **só**
com o `Manager`; trocar de backend ou adicionar recurso não toca no topo.

```
        servidor MCP  ·  CLI  ·  dashboard web  ·  app nativo (SwiftUI)
                              │
                       Manager (núcleo)
                              │   interface Driver  ← a costura
        ┌──────────────┬──────┴────────┬──────────────┐
       oci          apple-container   libkrun         mock
   (docker/podman)   (microVM, m26)   (futuro)       (testes)
```

A `Spec` descreve um sandbox a criar (imagem, rede, workdir, env, limites de
recurso, mounts, portas). O `Manager` aplica defaults, gera o id (`pp_xxxxxxxx`),
rotula o contêiner com `peapod.*` e delega ao `Driver`.

## Backends (drivers)

- **`oci`** (padrão) — usa a CLI do `docker` ou `podman`. Isolamento em nível de
  contêiner dentro da VM compartilhada do runtime. Rápido; ótimo para código
  semi-confiável.
- **`apple-container`** — usa o `container` da Apple (macOS 26+): **uma microVM
  por sandbox** via Virtualization.framework. Isolamento mais forte. Não tem
  *commit* de imagem, então snapshot/fork retornam "não suportado".
- **`mock`** — em memória, para testes e desenvolvimento sem daemon.

Seleção: flag global `--backend oci|apple|mock` ou variável `PEAPOD_BACKEND`.

**Capacidades opcionais** (detectadas por *interface assertion*, cada driver
implementa só o que suporta): `Checkpointer` (checkpoint/restore), `Logger`
(logs), `Statser` (stats), `SnapshotDiffer` (diff). Hoje só o `oci` as implementa.

## Modelo de segurança / privacidade

- **Rede desligada por padrão** (`--net none`) — *minimização*. Alternativa:
  `egress` (saída liberada) ou allowlist por proxy.
- **Allowlist de domínio (à prova de bypass)** — `sandbox create --allow d1,d2`
  coloca o sandbox numa rede Docker `--internal` (sem rota para fora) e sobe um
  **proxy sidecar** ligado à rede interna *e* à de egresso — a única ponte para
  fora. O proxy (HTTP + HTTPS via CONNECT) só encaminha para os domínios listados
  (e subdomínios); o resto recebe 403. Como a única rede do sandbox é a interna,
  **mesmo um processo que ignore `HTTP(S)_PROXY` fica sem rota** — o allowlist não
  é burlável. O proxy roda o binário linux do peapod (`peapod-linux-<arch>`,
  embutido no app e instalado pela fórmula); sem ele, `--allow` **falha fechado**.
  Tudo é criado e destruído junto com o sandbox; `peapod proxy` segue disponível
  para uso avulso.
- **Limites de recurso** por sandbox — defaults: **2 CPUs, 1024 MB de RAM, 512
  PIDs** (`--cpus`/`--memory`/`--pids-limit` no driver oci).
- **Timeout de exec** — o caminho MCP aplica 120 s por padrão (`timeout_seconds`;
  use `-1` para sem limite).
- **Trilha de auditoria** — cada `exec` é gravado em
  `~/.peapod/history/<id>.jsonl` (comando, hora, código de saída, prévia da
  saída). Some quando o sandbox é destruído.
- **Efemeridade** — `reap` (por idade) e `pause-idle`/auto-pause (por inatividade)
  evitam acúmulo e consumo desnecessário.

## Ciclo de vida de um sandbox

1. **create** — inicia um contêiner de vida longa (`sleep infinity`), rotulado
   `peapod.managed=true`, `peapod.id`, `peapod.image`, `peapod.created`, etc.
2. **exec** — roda comandos; auto-resume se estiver pausado; grava na auditoria.
3. **snapshot / fork** — `commit` da imagem e novo sandbox a partir dela.
4. **pause / resume** — congela/descongela os processos na memória (`docker
   pause`/`unpause`), sem persistir em disco.
5. **destroy** — remove o contêiner e o histórico.

O **backend é a fonte da verdade**: o estado vive nos rótulos do contêiner, então
CLI, app e servidor MCP enxergam os mesmos sandboxes entre processos.

## Referência da CLI

```
peapod sandbox create <image> [--net none|egress] [--ports h:c,…] [--allow d1,d2]
peapod sandbox exec <id> <cmd>...
peapod sandbox logs <id> [--tail N]
peapod sandbox stats <id> [--json]
peapod sandbox history <id> [--json]
peapod sandbox pause|resume <id>
peapod sandbox checkpoint|restore <id> <name>   (experimental; engine com CRIU)
peapod sandbox snapshot <id> <name>
peapod sandbox fork <snapshot> [--name N] [--net none|egress]
peapod sandbox ls [--json]
peapod sandbox rm <id>

peapod snapshot ls | rm <ref> | prune [--max-age 24h] | diff <a> <b>
peapod up | down | ps [-f peapod.json]     # grupos multi-serviço
peapod preview up | status | down          # ambiente por branch do git
peapod proxy --allow d1,d2 [--addr :8899]  # proxy de allowlist de egresso
peapod reap [--max-age 30m]                # destrói sandboxes antigos
peapod pause-idle [--max-idle 15m]         # pausa sandboxes ociosos
peapod templates [--json]                  # lista os modelos
peapod ui [--addr 127.0.0.1:7070]          # dashboard web
peapod mcp                                 # servidor MCP (stdio)
peapod version
peapod --backend oci|apple|mock <command>
```

## Servidor MCP (12 ferramentas)

Inicie com `peapod mcp` (transporte stdio). Ferramentas expostas a agentes:

- `peapod_sandbox_create` — cria um sandbox (imagem, rede). Rede off por padrão.
- `peapod_exec` — roda um comando e captura stdout/stderr/código de saída.
- `peapod_write_file` / `peapod_read_file` — escreve/lê arquivo no sandbox.
- `peapod_list` — lista os sandboxes.
- `peapod_destroy` — destrói um sandbox.
- `peapod_snapshot` / `peapod_fork` — snapshot do filesystem e fork a partir dele.
- `peapod_snapshot_list` / `peapod_snapshot_remove` — gerencia snapshots.
- `peapod_snapshot_diff` — diff de arquivos entre dois snapshots.
- `peapod_history` — a trilha de auditoria (o que o agente rodou).

O `.mcp.json` na raiz do projeto registra o servidor automaticamente no Claude
Code ao abrir a pasta.

## Snapshots

- **snapshot** — `docker commit` para a imagem `peapod-snapshot:<nome>`.
- **fork** — cria um novo sandbox a partir de uma imagem de snapshot.
- **diff** — compara as listas de arquivos de dois snapshots (adicionados/removidos).
- **prune** — remove snapshots mais velhos que `--max-age` (lê o `CreatedAt`).

## Checkpoint / restore (experimental)

`sandbox checkpoint`/`restore` usam `docker checkpoint` (CRIU). **No OrbStack o
`create` funciona, mas o `restore` está quebrado** (erro de containerd); o
código está correto e funciona em engines com CRIU completo (ex.: Docker Engine
no Linux). Para "congelar e retomar" no dia a dia, use `pause`/`resume`.

## Preview envs (por branch do git)

`peapod preview up` cria um sandbox nomeado por `<repo>-<branch>`, com o repositório
montado em `/repo` e rede `egress`. `status` mostra o do branch atual; `down`
destrói. Útil para "ambientes de preview" locais por branch.

## Multi-serviço (peapod.json)

Um manifesto `peapod.json` descreve vários serviços:

```json
{
  "name": "demo",
  "services": {
    "api":   { "image": "python:3.12-slim", "ports": ["8000:8000"] },
    "cache": { "image": "redis:7" }
  }
}
```

`peapod up` cria o grupo (cada serviço como `<name>-<serviço>`), `ps` lista e
`down` derruba. Portas são publicadas com `-p` (driver oci).

## Dashboard web

`peapod ui` serve um painel em `http://127.0.0.1:7070`: listar, criar, pausar/retomar
e destruir sandboxes, e ver snapshots. Endpoints JSON: `/api/sandboxes`,
`/api/destroy`, `/api/pause`, `/api/resume`, `/api/snapshots`.

## App nativo (este)

SwiftUI, janela única, com o binário `peapod` **embutido** em
`Contents/Resources` (zero configuração). Localizado em pt-BR. Construído por
`ui-native/build.sh`, que também gera o `.icns`, o `.dmg`, assina ad-hoc e
incrementa `CFBundleVersion` a cada build (evita ícone em cache). Ele encontra o
`docker`/OrbStack aumentando o `PATH` ao chamar o `peapod`.

## Variáveis de ambiente

- `PEAPOD_BACKEND` — `oci` (padrão), `apple` ou `mock`.
- `PEAPOD_REAP_TTL` — se definido (ex.: `30m`), o servidor MCP destrói sandboxes
  antigos em segundo plano.
- `PEAPOD_IDLE_PAUSE_TTL` — se definido (ex.: `15m`), pausa sandboxes ociosos em
  segundo plano.
- `PEAPOD_BIN` — caminho do binário `peapod` (usado pelo app, se não embutido).

## Arquivos e dados

- `~/.peapod/history/<id>.jsonl` — trilha de auditoria por sandbox.
- `.mcp.json` (raiz do projeto) — registro do servidor MCP para o Claude Code.
- Imagens de snapshot: `peapod-snapshot:<nome>` no runtime.

## Build e desenvolvimento

```sh
go build -o bin/peapod ./cmd/peapod   # CLI/servidor
go test ./...                         # usa o driver mock; não precisa de daemon
go vet ./...
cd ui-native && ./build.sh            # gera Peapod.app + Peapod.dmg
```

CI (GitHub Actions) roda vet/test/build a cada push. Fórmula Homebrew em
`Formula/peapod.rb`.

## Licença

AGPL-3.0 © André Augusto Azarias De Souza.
