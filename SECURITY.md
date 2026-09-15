# Política de segurança

## Reportando vulnerabilidades

Não abra issue pública para uma vulnerabilidade. Abra um [GitHub Security Advisory privado](https://github.com/andre28abr/Peapod/security/advisories/new) ou escreva para o mantenedor pelo e-mail do [perfil](https://github.com/andre28abr).

A resposta sai em até 7 dias. É um projeto de portfólio sem SLA comercial: a gravidade pauta a prioridade.

## Escopo

O Peapod existe para conter código não confiável. Por isso, **é vulnerabilidade reportável** qualquer forma de um sandbox: sair do isolamento do container ou da microVM; alcançar a rede quando ela está desligada; contornar a allowlist de domínios do proxy; ler ou escrever arquivos do host fora dos volumes montados; escapar dos limites de CPU, memória ou processos; alterar ou apagar a trilha de auditoria; e também injeção de comando pelas ferramentas MCP, pela CLI ou pelo dashboard web, e dependências vulneráveis que afetem o projeto (rode `govulncheck` antes; o CI já roda).

**Não são vulnerabilidades:** limitações documentadas dos engines (por exemplo, o restore de checkpoint no OrbStack), o driver `mock`, que não isola nada por definição e existe para testes, e o app ser assinado ad-hoc.

## Defesas existentes

- Rede `none` por padrão; saída só por proxy com allowlist explícita.
- Limites de CPU, memória e PIDs por sandbox; ambientes efêmeros.
- Trilha de auditoria de cada comando executado.
- Drivers isolados por interface, com capacidades opcionais detectadas em tempo de execução.
- CI com `go vet`, testes com `-race` e `govulncheck`.

## Histórico de divulgações

Nenhuma vulnerabilidade reportada até o momento.
