# CLAUDE.md

Orientações para o Claude Code trabalhar neste repositório. Leia antes de agir.
Referência completa: [README.md](README.md) (visão geral), [docs/MANUAL.md](docs/MANUAL.md) (técnico), [docs/GUIA.md](docs/GUIA.md) (sem jargão).

## O que é

**Peapod** — sandboxes Linux **isolados, descartáveis e auditados** para rodar código não confiável ou gerado por IA.
Dirigido por **MCP** (12 ferramentas), **CLI**, **dashboard web** e **app nativo de macOS** (SwiftUI, com o binário
embutido). Princípios que viram código: rede desligada por padrão, allowlist de egresso por proxy à prova de bypass,
trilha de auditoria de cada comando, ambientes efêmeros, limites de CPU/memória/PID. Documentação em português,
código e mensagens de commit em inglês.

**Stack:** Go 1.26 (módulo `peapod`, única dependência direta: `github.com/modelcontextprotocol/go-sdk`) · Swift 6 / SwiftUI
(app, macOS 13+) · backends: Docker/Podman via OrbStack (`oci`), Apple `container` (`apple`), mock em memória (`mock`).
Distribuição: `Peapod.dmg` nas releases do GitHub e tap Homebrew `andre28abr/peapod` (fórmula em `Formula/peapod.rb`).

## Como rodar / testar

```bash
go build -o bin/peapod ./cmd/peapod            # CLI (bin/ é ignorado pelo git)
bin/peapod --backend mock sandbox create alpine # tudo funciona sem daemon com --backend mock
bin/peapod ui                                   # dashboard web
bin/peapod mcp                                  # servidor MCP por stdio (o .mcp.json da raiz faz isso via go run)

go test -race ./...                             # 27 testes, driver mock — não precisa de OrbStack
go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@latest ./...   # o CI exige os dois limpos
cd ui-native && ./build.sh                      # Peapod.app + Peapod.dmg (só macOS; artefatos ignorados pelo git)
```

Variáveis: `PEAPOD_BACKEND` (oci | apple | mock), `PEAPOD_REAP_TTL` (ex.: `30m`, ceifa sandboxes ociosos),
`PEAPOD_IDLE_PAUSE_TTL` (pausa ociosos). Lista completa no MANUAL.

## Estrutura

- `cmd/peapod/` — CLI (cobra-less, subcomandos à mão) e ponto de entrada de `ui` e `mcp`. Testes de CLI aqui.
- `internal/sandbox/` — o **núcleo**: `Manager` (ciclo de vida, histórico, reap, snapshots, preview envs, multi-serviço)
  sobre a interface `Driver`. Capacidades opcionais (logs, stats, checkpoint, diff) são interfaces separadas detectadas
  por *type assertion*; o Manager devolve erro claro quando o backend não implementa.
- `internal/driver/oci/` — Docker/Podman (OrbStack); firewall = rede interna + sidecar proxy. `apple/` — microVM da Apple.
  `mock/` — em memória, usado por todos os testes; implementa só o essencial (sem logs/stats/checkpoint/diff, de propósito).
- `internal/proxy/` — proxy HTTP(S) de egresso com allowlist e anti-SSRF (recusa privados/loopback).
- `internal/mcpserver/` — as 12 ferramentas MCP sobre o Manager. Testado pelo protocolo com cliente em memória.
- `internal/web/` — dashboard web (HTTP + HTML servido pelo binário). `internal/backend/` — seleção de backend. `internal/version/` — versão.
- `ui-native/` — `Peapod.swift` (app), `Icon.swift`, `Diagrams.swift`, `build.sh`. Compilado no CI em macOS.
- `docs/` — GUIA, MANUAL, `index.html` (GitHub Pages), demo.gif e imagens. `scripts/` — gravação da demo (vhs).

## Regras que não podem quebrar

- **Fail-closed.** Rede `none` é o padrão; `allow` sem rede interna não existe; portas publicadas com `allow` são
  rejeitadas. Qualquer mudança em política de rede precisa de teste no `Manager` e no proxy.
- **Toda execução deixa histórico.** `Exec` grava a entrada de auditoria com o argv original, mesmo com timeout/watchdog.
- **As 12 ferramentas MCP têm nome e contrato estáveis.** Renomear quebra `.mcp.json` de todo mundo; há teste que
  confere a lista. Erros do núcleo chegam ao agente como *tool error* (`IsError`), nunca como falha de protocolo.
- **Um núcleo, quatro frentes.** CLI, web, MCP e app chamam o mesmo `Manager`; lógica de negócio não mora em `cmd/` nem no Swift.
- **Sem dependências novas sem motivo.** O módulo tem uma dependência direta; manter assim é decisão de projeto.
- Nunca commite `bin/`, `ui-native/Peapod.app`, `Peapod.dmg`, `.icns` (já ignorados) nem `.claude/settings.local.json`.

## Release

1. `internal/version` recebe a versão; commit `Release: bump version to X.Y.Z`.
2. Tag `vX.Y.Z`, `cd ui-native && ./build.sh`, publicar `Peapod.dmg` na release do GitHub.
3. `Formula/peapod.rb`: `url` da tag + `sha256` do tarball; espelhar no tap `andre28abr/homebrew-peapod`.

## Convenções

- Commits em inglês, imperativo, com prefixo de área quando ajuda: `Formula: …`, `Firewall: …`, `Release: …`, `Review batch N: …`.
- `main` é a única branch; releases por tag. CI precisa estar verde antes de taggear.
- Docs e UI em português (pt-BR); código, comentários e mensagens de erro em inglês.
