# Sobre o autor

## André Augusto Azarias De Souza

→ [LinkedIn](https://linkedin.com/in/andreaugusto-azariasdesouza) · [GitHub](https://github.com/andre28abr) · [Profile completo](https://github.com/andre28abr)

---

## Resumo

Profissional com mais de 18 anos de experiência em **gestão administrativa, compliance, governança da informação e proteção de dados pessoais**, com atuação integrada entre áreas administrativas, tecnologia da informação e conformidade regulatória.

Formação dupla em **Direito (Anhanguera)** e **Análise e Desenvolvimento de Sistemas (Mackenzie)**, complementada por especializações em LGPD, Direito Digital, Segurança Digital e Liderança Ágil.

Exerceu por quase duas décadas a função de **Gerente Administrativo e Encarregado de Dados (DPO)** em organização do setor de saúde suplementar, com atuação na organização da governança, adequação à LGPD, controle documental e apoio às áreas administrativas e tecnológicas.

Atualmente em transição de carreira, com **disponibilidade imediata**, busca posições em DPO (Encarregado de Dados), Compliance, Governança & GRC, Privacy Engineering ou Security Analyst com viés regulatório.

---

## Por que esse projeto existe

O **Peapod** nasceu como exercício pessoal de portfólio com três objetivos:

1. **Levar privacidade e menor privilégio para onde dados sensíveis vão passar a seguir: a execução de código por IA.** Enquanto o [SentinelBR](https://github.com/andre28abr/SentinelBR-platform) cuida da *infraestrutura* (servidores) e o [VigiaOS](https://github.com/andre28abr/VigiaOS) cuida da *estação de trabalho*, o Peapod cuida do ambiente onde um **agente de IA roda código** — isolando cada execução, cortando a rede por padrão e registrando tudo. Os mesmos princípios de LGPD/governança, num problema de 2026.

2. **Traduzir exigências de privacidade e segurança em decisões de produto concretas.** Rede `none` por padrão (*minimização*), allowlist de egresso via proxy (*menor privilégio*), trilha de auditoria de cada comando (*auditabilidade*, no espírito de um ROPA), ambientes efêmeros (*retenção mínima*) e limites de CPU/memória/PID — cada escolha técnica reflete um princípio regulatório, não um detalhe de implementação.

3. **Exercitar orquestração de projeto técnico complexo com auxílio de IA generativa.** A skill emergente do mercado pós-2024 não é "decorar sintaxe" — é saber **definir requisitos, validar arquitetura, traduzir necessidades de negócio em especificações técnicas** e usar IA pra acelerar a entrega. O Peapod reúne um núcleo em **Go** com 3 backends de isolamento (Docker/Podman, microVM da Apple, mock), **servidor MCP** (12 ferramentas), **CLI**, **dashboard web** e **app nativo SwiftUI** — tudo gerenciado nesse modelo, com testes verdes.

---

## Atuação neste projeto

**Papel:** Product Owner técnico, com auxílio de assistentes de IA generativa para a etapa de codificação.

**Entregas pessoais (sem auxílio de IA):**
- Definição de **requisitos, escopo e roadmap** — incluindo a decisão estratégica de **abandonar** o caminho inicial (clonar o OrbStack, batalha perdida contra produto fechado) e **pivotar** para "sandboxes descartáveis para agentes de IA", onde o trabalho difícil de sistemas vira diferencial.
- **Validação da arquitetura**: a costura `Driver` (oci / apple-container / libkrun / mock), capacidades opcionais detectadas por *interface assertion* (checkpoint, logs, stats, diff), e um núcleo `Manager` único servindo MCP, CLI, web e app.
- **Tradução de princípios de privacidade/menor privilégio** em requisitos funcionais: rede desligada por padrão, allowlist de egresso, trilha de auditoria por sandbox, efemeridade e limites de recurso.
- **Decisão de plataforma**: começar pelo Docker/OrbStack (pragmático, roda hoje), com um driver de **microVM da Apple** validado ao vivo para isolamento por-VM — e **honestidade sobre os limites do engine** (ex.: o *restore* de checkpoint CRIU está quebrado no OrbStack; documentado, não escondido).
- **Review e decisões de trade-off** em cada fase: *freeze* via `docker pause` (memória preservada) em vez de checkpoint-em-disco; allowlist no nível de proxy HTTP(S) vs. rede interna + sidecar; app nativo em janela vs. menu-bar; etc.

**Etapa de codificação:** orquestrada com auxílio de IA generativa, sob direção e revisão do autor. A stack (Go, SwiftUI, Model Context Protocol) foi escolhida pela aderência ao caso de uso — execução isolada, agentes de IA, macOS — não por domínio prático prévio em escrita de código de produção.

---

## Formação relevante para o domínio

### Formação acadêmica

- **Bacharelado em Direito** — Anhanguera Educacional
- **Análise e Desenvolvimento de Sistemas** — Universidade Presbiteriana Mackenzie

### Pós-graduações ligadas a Privacy / Security / Tech

- **Privacidade e Proteção de Dados Pessoais (LGPD)** — Faculdade Focus
- **Direito, Inovação e Tecnologia** — Faculdade CERS
- **Direito Digital** — Legale Educacional
- **Segurança Digital, Governança e Gestão de Dados** — PUCRS

### Certificações ligadas ao tema deste projeto

- **DPO – Data Protection Officer (LGPD)** — CERS (2020)
- **Cybersecurity Essentials** — Cisco (2022)
- **Cibersegurança – Ameaças e Táticas de Prevenção** — FGV (2023)
- **Crise Cibernética e Continuidade de Negócios** — FGV (2023)
- **Fundamentos na Lei Geral de Proteção de Dados** — Certiprof Summit (2023)
- **Data Mapping: da Teoria à Prática** — IbiJus (2023)
- **AI for Leaders** — StartSe University (2024)
- **Visual Law** — Legale Educacional (2023)

---

## Outros projetos

**[SentinelBR](https://github.com/andre28abr/SentinelBR-platform)** — Plataforma open-source de **SIEM + LGPD** para PMEs brasileiras. Coleta logs e inventário de servidores Linux via agente Go (gRPC mTLS), detecção em tempo real (regras Sigma-style + YARA + OSV.dev), resposta automatizada (SOAR-lite) e compliance LGPD nativa, multi-tenant.

**[VigiaOS](https://github.com/andre28abr/VigiaOS)** — Suíte de **segurança, privacidade e LGPD** para a estação de trabalho (Fedora Workstation, GTK4 + libadwaita): hardening, antivírus, integridade de arquivos, controles de privacidade e relatórios de conformidade, tudo em português.

**[Plataforma LGPD](https://github.com/andre28abr/lgpd-platform)** — Plataforma web multi-tenant que **treina, avalia e opera** a conformidade com a LGPD: diagnóstico de maturidade, ROPA (Art. 37), RIPD (Art. 38), direitos do titular (Art. 18) e resposta a incidentes (Art. 48).

**SC Platform** *(privado, sob NDA — disponível para apresentação em entrevistas mediante solicitação)* — Plataforma SaaS multi-tenant pra gestão de licitações públicas brasileiras (PNCP em tempo real, simulador FSM da Lei 14.133, robô de lances, extração de PDF com IA local, CRM, Telegram). 75k+ linhas, 420 testes.

---

→ **[LinkedIn](https://linkedin.com/in/andreaugusto-azariasdesouza)** · [GitHub](https://github.com/andre28abr)
