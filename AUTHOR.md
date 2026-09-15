# Sobre o autor

## André Augusto Azarias de Souza

→ [LinkedIn](https://linkedin.com/in/andreaugusto-azariasdesouza) · [GitHub](https://github.com/andre28abr) · contato@azariasdesouza.com

---

## Quem é

Gestor com **18 anos de atuação como Gerente Administrativo e Encarregado de Dados (DPO)** em organização do setor de saúde suplementar, ambiente regulado pela ANS e pela LGPD. Participou de decisões de diretoria, conduziu a relação com hospitais e operadoras, liderou a modernização dos sistemas administrativos e de segurança da informação e coordenou o programa de adequação à LGPD da organização, com dados sensíveis de saúde sob o Art. 11.

Formado em **Direito** e em **Análise e Desenvolvimento de Sistemas**, com pós-graduações em segurança digital, governança de dados, privacidade, direito digital e liderança ágil.

Atua na interseção entre **Compliance, GRC, privacidade e segurança da informação**: mapeamento de dados e ROPA (Art. 37), RIPD/DPIA (Art. 38), direitos do titular (Art. 18), gestão de operadores e terceiros (Art. 39), resposta a incidentes (Art. 48), interface com a ANPD, e os frameworks NIST CSF, CIS Controls e ISO/IEC 27001/27701. Trabalha com o princípio de que proteção de dados é também arquitetura: Security by Design, Zero Trust, defesa em camadas e menor privilégio.

Desde 2025 conduz, como **product owner técnico**, projetos open-source de segurança e privacidade em Python, Go, Rust e Swift, com a codificação orquestrada por assistentes de IA generativa sob sua direção e revisão. Desenvolve **automações de processos com n8n** e é autor de cinco livros publicados, entre eles *Da Norma à Liderança*, sobre atualização profissional em GRC.

---

## Automação de processos com n8n

Projeta e opera **automações de processos de negócio e jurídicos em n8n**, self-hosted em Docker Compose, com foco em privacidade: processamento local, gravação em disco restrita a pastas definidas, sem envio de dados a serviços de terceiros. Entre o que já construiu:

- **Triagem automática de publicações judiciais**: busca de hora em hora no DJEN (Comunica CNJ) por OAB, classificação por urgência, cálculo de prazo provisório em dias úteis, contexto do processo via DataJud e painel web de tratamento por advogado.
- **Onboarding de clientes**: formulário web que gera em segundos procuração, declaração de hipossuficiência e contrato de honorários em PDF (Gotenberg), com registro do cliente para os fluxos seguintes.
- **Portal e páginas servidas pelo próprio n8n** via webhooks, instaladores para Mac e Windows, variante para servidor com HTTPS automático e autenticação (Caddy) e rotina de backup.
- **Integrações com APIs públicas** (DJEN, DataJud, BrasilAPI) e desenho de fluxos com Code nodes, banco JSON local e controle de estado entre execuções.

---

## Por que esse projeto existe

O **Peapod** nasceu como exercício pessoal de portfólio com três objetivos:

1. **Levar privacidade e menor privilégio para onde dados sensíveis vão passar a seguir: a execução de código por IA.** Enquanto o [SentinelBR](https://github.com/andre28abr/SentinelBR-platform) cuida da *infraestrutura* (servidores) e o [VigiaOS](https://github.com/andre28abr/VigiaOS) cuida da *estação de trabalho*, o Peapod cuida do ambiente onde um **agente de IA roda código**, isolando cada execução, cortando a rede por padrão e registrando tudo. Os mesmos princípios de LGPD/governança, num problema de 2026.

2. **Traduzir exigências de privacidade e segurança em decisões de produto concretas.** Rede `none` por padrão (*minimização*), allowlist de egresso via proxy (*menor privilégio*), trilha de auditoria de cada comando (*auditabilidade*, no espírito de um ROPA), ambientes efêmeros (*retenção mínima*) e limites de CPU/memória/PID, cada escolha técnica reflete um princípio regulatório, não um detalhe de implementação.

3. **Exercitar orquestração de projeto técnico complexo com auxílio de IA generativa.** A skill emergente do mercado pós-2024 não é "decorar sintaxe", é saber **definir requisitos, validar arquitetura, traduzir necessidades de negócio em especificações técnicas** e usar IA pra acelerar a entrega. O Peapod reúne um núcleo em **Go** com 3 backends de isolamento (Docker/Podman, microVM da Apple, mock), **servidor MCP** (12 ferramentas), **CLI**, **dashboard web** e **app nativo SwiftUI**, tudo gerenciado nesse modelo, com testes verdes.

---

## Atuação neste projeto

**Papel:** Product Owner técnico, com auxílio de assistentes de IA generativa para a etapa de codificação.

**Entregas pessoais (sem auxílio de IA):**
- Definição de **requisitos, escopo e roadmap**, incluindo a decisão estratégica de **abandonar** o caminho inicial (clonar o OrbStack, batalha perdida contra produto fechado) e **pivotar** para "sandboxes descartáveis para agentes de IA", onde o trabalho difícil de sistemas vira diferencial.
- **Validação da arquitetura**: a costura `Driver` (oci / apple-container / libkrun / mock), capacidades opcionais detectadas por *interface assertion* (checkpoint, logs, stats, diff), e um núcleo `Manager` único servindo MCP, CLI, web e app.
- **Tradução de princípios de privacidade/menor privilégio** em requisitos funcionais: rede desligada por padrão, allowlist de egresso, trilha de auditoria por sandbox, efemeridade e limites de recurso.
- **Decisão de plataforma**: começar pelo Docker/OrbStack (pragmático, roda hoje), com um driver de **microVM da Apple** validado ao vivo para isolamento por-VM, e **honestidade sobre os limites do engine** (ex.: o *restore* de checkpoint CRIU está quebrado no OrbStack; documentado, não escondido).
- **Review e decisões de trade-off** em cada fase: *freeze* via `docker pause` (memória preservada) em vez de checkpoint-em-disco; allowlist no nível de proxy HTTP(S) vs. rede interna + sidecar; app nativo em janela vs. menu-bar; etc.

**Etapa de codificação:** orquestrada com auxílio de IA generativa, sob direção e revisão do autor. A stack (Go, SwiftUI, Model Context Protocol) foi escolhida pela aderência ao caso de uso (execução isolada, agentes de IA, macOS), não por domínio prático prévio em escrita de código de produção.

---

## Outros projetos

**[SentinelBR](https://github.com/andre28abr/SentinelBR-platform)**: plataforma open-source de **SIEM + LGPD** para PMEs brasileiras. Agente Go com gRPC e mTLS, detecção em tempo real, resposta automatizada e compliance LGPD nativa, multi-tenant. 225 testes entre servidor, agente e frontend; CI em 16 jobs.

**[VigiaOS](https://github.com/andre28abr/VigiaOS)**: suíte de **segurança, privacidade e LGPD** para a estação de trabalho (Fedora Workstation, GTK4 + libadwaita), com 13 ferramentas defensivas, módulos de detecção e resposta e laboratório educacional com termo de uso. 1460 testes em Python e 28 em Rust.

**[Plataforma LGPD](https://github.com/andre28abr/lgpd-platform)**: plataforma web multi-tenant que **treina, avalia e certifica** os setores de uma empresa em LGPD e dá ao DPO as ferramentas de operação: ROPA, RIPD, direitos do titular e incidentes. 121 testes, 95% de cobertura.

**[Uptend](https://github.com/andre28abr/Uptend)**: app nativo de macOS para **configurar e manter o Mac** e **auditar servidores Linux**: coletor portátil, relatórios, correlação com CVEs, MITRE ATT&CK, lente LGPD e playbook de hardening com rollback. Swift 6, 428 testes.

**[banana](https://github.com/andre28abr/banana)**: editor **local-first** de notas Markdown, código e PDF, com vault cifrado (Argon2id + AES-256-GCM). Tauri 2, Rust e Svelte 5, 393 testes.

**SC Platform** *(privado, disponível para apresentação mediante solicitação)*: SaaS multi-tenant para gestão de licitações públicas, com PNCP em tempo real, simulador da Lei 14.133/2021, robô de lances em três modos, extração de PDF com IA local, CRM e Telegram. Cerca de 75 mil linhas e 547 testes.

**AUGRAZ** *(privado, produto da empresa do autor)*: plataforma de compliance **LGPD + ISO 27001** para assessoria de proteção de dados: 11 módulos por empresa-cliente (ROPA, canal do titular, incidentes, comunicações com a ANPD, fornecedores, treinamentos), biblioteca dos 93 controles do Anexo A da ISO/IEC 27001:2022 com Gap Analysis, relatórios imprimíveis e geradores de política de privacidade, aviso de cookies e termos de uso. Flask, testes em SQLite e PostgreSQL, CI com lint e auditoria de dependências.

**Site AUGRAZ** *(privado, protótipo ainda não publicado)*: site institucional em HTML e PHP com formulário de contato em PDO e prepared statements, credenciais fora do repositório e `.htaccess` com HTTPS forçado, bloqueio de arquivos sensíveis e cabeçalhos de segurança (HSTS, nosniff, X-Frame-Options, Referrer-Policy, Permissions-Policy).

---

→ **[LinkedIn](https://linkedin.com/in/andreaugusto-azariasdesouza)** · [GitHub](https://github.com/andre28abr)
