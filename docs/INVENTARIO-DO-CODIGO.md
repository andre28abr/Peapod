# Inventário do código: Peapod

> Gerado por `raiz/maquina/inventario.py` em 2026-09-20, direto do código. Não editar à mão: regenerar ao fechar um bloco de trabalho. Serve de mapa para quem vai mexer no projeto; a explicação do porquê está no README, no CLAUDE.md e nos docs do repositório. Testes são contados por função declarada; o pytest e o cargo podem reportar mais execuções por causa de parametrização.

## Resumo

| Linguagem | Números |
|---|---|
| Go | 10 pacotes, 85 funções/métodos exportados, 23 tipos exportados, 27 testes |
| Swift | 3 arquivos, 15 tipos, 0 testes |

## Go

| Pacote | O que é (doc) | Arquivos | Tipos exp. | Funções exp. | Testes |
|---|---|---|---|---|---|
| `cmd/peapod` |  | 1 | 0 | 0 | 5 |
| `internal/backend` | selects a sandbox driver by name. | 1 | 0 | 1 | 0 |
| `internal/driver/applecontainer` | implements sandbox.Driver on Apple's `container` CLI, | 1 | 1 | 15 | 0 |
| `internal/driver/mock` | is an in-memory sandbox.Driver for tests and daemon-less dev. | 1 | 1 | 15 | 0 |
| `internal/driver/oci` | implements sandbox.Driver on top of the docker (or podman) CLI. | 1 | 1 | 21 | 4 |
| `internal/mcpserver` | exposes the Peapod núcleo to AI agents over MCP: the way | 1 | 0 | 2 | 4 |
| `internal/proxy` | is an allowlisting HTTP/HTTPS forward proxy for sandboxes: | 1 | 1 | 4 | 4 |
| `internal/sandbox` |  | 3 | 19 | 25 | 8 |
| `internal/version` | is the single source of truth for Peapod's version string. | 1 | 0 | 0 | 0 |
| `internal/web` | serves a minimal local dashboard for Peapod over HTTP. | 1 | 0 | 2 | 2 |

## Swift

| Arquivo | Pasta | Tipos | Principais tipos · doc |
|---|---|---|---|
| `ui-native/Diagrams.swift` | ui-native | 0 |  |
| `ui-native/Icon.swift` | ui-native | 0 |  |
| `ui-native/Peapod.swift` | ui-native | 15 | Sandbox, Stat, HistoryEntry, Template, Flash, Model … · Last meaningful line of peapod's stderr, without the "peapod: " prefix. |

## Scripts de shell

| Arquivo | Primeira linha de comentário |
|---|---|
| `scripts/demo.sh` | A short scripted Peapod demo. Run it, or screen-record it to make a GIF. |
| `ui-native/build.sh` | Build a self-contained Peapod.app (with the peapod CLI bundled inside) and a |
