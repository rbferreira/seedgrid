# **SeedGrid**
# **Architecture Decision Records (ADR)**
Version: 1.0
Status: Draft
Document: ADR.md
# **1. Objetivo**
Este documento registra as principais decisões arquiteturais adotadas durante o desenvolvimento do SeedGrid.
Seu objetivo é preservar o contexto técnico que motivou cada decisão, facilitando evolução, manutenção e futuras revisões da plataforma.
# **2. Filosofia**
Uma decisão arquitetural representa uma escolha consciente entre diferentes alternativas possíveis.
Cada decisão registrada neste documento deverá responder:
- qual problema existia;
- quais alternativas foram consideradas;
- qual solução foi adotada;
- quais benefícios foram obtidos;
- quais limitações foram aceitas.
# **ADR-001**
# **Arquitetura Centralizada**
**Decisão**
O SeedGrid utilizará um Control Plane central responsável por coordenar todo o Cluster.
**Contexto**
Era possível adotar uma arquitetura distribuída, onde cada Node tomaria decisões locais.
Essa abordagem aumentaria significativamente a complexidade da plataforma.
**Decisão**
Toda inteligência permanecerá no Control Plane.
Os Nodes executarão apenas operações locais.
**Consequências**
Benefícios:
- arquitetura simples;
- decisões consistentes;
- monitoramento centralizado;
- menor acoplamento.
Limitações:
- dependência do Control Plane para operações administrativas.
# **ADR-002**
# **Nodes Independentes**
**Decisão**
Seed Nodes nunca estabelecerão comunicação direta entre si.
**Contexto**
Clusters distribuídos normalmente utilizam comunicação entre os próprios nós.
Entretanto, isso aumenta sincronização, descoberta de serviços e tratamento de falhas.
**Decisão**
Toda comunicação ocorrerá através do Control Plane.
**Consequências**
Benefícios:
- isolamento;
- simplicidade;
- escalabilidade;
- facilidade de substituição.
Limitações:
- inexistência de coordenação direta entre Nodes.
# **ADR-003**
# **Agent Stateless**
**Decisão**
O Agent não armazenará informações permanentes do Cluster.
**Contexto**
Era possível manter parte do estado distribuída entre os Nodes.
Essa abordagem dificultaria recuperação e sincronização.
**Decisão**
O Agent manterá apenas informações necessárias para sua operação local.
O Estado oficial permanecerá no Control Plane.
**Consequências**
Benefícios:
- substituição rápida;
- recuperação simples;
- menor risco de inconsistências.
Limitações:
- dependência do Control Plane para sincronização administrativa.
# **ADR-004**
# **Scheduler Centralizado**
**Decisão**
Todas as decisões de distribuição serão executadas pelo Scheduler central.
**Contexto**
Outra alternativa seria permitir decisões locais.
Isso dificultaria balanceamento global.
**Decisão**
O Scheduler utilizará visão completa do Cluster.
**Consequências**
Benefícios:
- melhor distribuição;
- decisões consistentes;
- algoritmos mais sofisticados.
Limitações:
- necessidade de comunicação com o Control Plane.
# **ADR-005**
# **Portal Desacoplado**
**Decisão**
O Portal comunicará exclusivamente com a API.
**Contexto**
Era possível permitir acesso direto aos Nodes.
Essa abordagem aumentaria acoplamento e complexidade de autenticação.
**Decisão**
Toda comunicação administrativa passará obrigatoriamente pela API.
**Consequências**
Benefícios:
- segurança;
- padronização;
- desacoplamento;
- simplicidade.
Limitações:
- dependência da API para todas as operações.
# **ADR-006**
# **PostgreSQL como Estado Oficial**
**Decisão**
O PostgreSQL armazenará todo o Estado do Cluster.
**Contexto**
Alternativas avaliadas:
- SQLite;
- bancos distribuídos;
- armazenamento baseado em arquivos.
**Decisão**
Utilizar PostgreSQL como única fonte oficial de persistência.
**Consequências**
Benefícios:
- consistência;
- confiabilidade;
- maturidade;
- facilidade de backup.
Limitações:
- necessidade de um serviço adicional.
# **3. Princípio**
Toda decisão registrada neste documento deverá permanecer válida até que uma nova ADR a substitua formalmente.
Alterações arquiteturais nunca deverão ocorrer de maneira implícita.

# **ADR-007**
# **Monitoramento Desacoplado**
**Decisão**
O sistema de monitoramento permanecerá independente da operação da plataforma.
**Contexto**
Era possível incorporar métricas e dashboards diretamente ao Control Plane.
Essa abordagem aumentaria o acoplamento entre operação e observabilidade.
**Decisão**
Prometheus, Grafana e demais componentes de observabilidade permanecerão separados do núcleo operacional.
**Consequências**
Benefícios:
- menor acoplamento;
- facilidade de substituição;
- adoção de padrões amplamente utilizados;
- maior flexibilidade.
Limitações:
- necessidade de componentes adicionais.

> Refinada e parcialmente superada pela ADR-016: a **coleta** de métricas permanece desacoplada (Prometheus), mas a **visualização** passa a viver no produto (Portal, via API do Control Plane), e o Grafana é removido.
# **ADR-008**
# **Alertas Baseados em Métricas e Eventos**
**Decisão**
O Alert Service utilizará exclusivamente métricas, eventos e estados conhecidos da plataforma.
**Contexto**
Uma alternativa seria analisar logs para geração de alertas.
Essa abordagem aumentaria complexidade, custo de processamento e imprevisibilidade.
**Decisão**
Logs permanecerão destinados exclusivamente à investigação técnica.
Alertas serão gerados apenas a partir de informações estruturadas.
**Consequências**
Benefícios:
- regras determinísticas;
- menor consumo de recursos;
- facilidade de testes;
- menor quantidade de falsos positivos.
Limitações:
- algumas condições só poderão ser identificadas durante análise manual dos logs.
# **ADR-009**
# **Backup Independente da Ferramenta**
**Decisão**
O SeedGrid não dependerá de uma solução específica de backup.
**Contexto**
Existem diversas ferramentas maduras para proteção de dados.
Incorporar uma solução específica criaria dependências desnecessárias.
**Decisão**
A plataforma definirá políticas, acompanhará execuções e apresentará informações ao operador, permanecendo independente da ferramenta utilizada.
**Consequências**
Benefícios:
- flexibilidade;
- compatibilidade;
- liberdade para o operador.
Limitações:
- implantação inicial exige configuração da ferramenta escolhida.
# **ADR-010**
# **Backup Opcional dos Torrents**
**Decisão**
O backup dos dados dos torrents será opcional.
**Contexto**
Os arquivos dos torrents podem representar grandes volumes de dados.
Em muitos cenários, seu custo de proteção supera seu valor operacional.
**Decisão**
A plataforma protegerá prioritariamente o Estado do Cluster.
Os dados dos torrents poderão seguir política independente definida pelo operador.
**Consequências**
Benefícios:
- simplicidade;
- menor custo operacional;
- maior flexibilidade.
Limitações:
- perda dos arquivos quando inexistir política de proteção.
# **ADR-011**
# **Componentes Efêmeros**
**Decisão**
Os componentes da infraestrutura deverão ser considerados substituíveis.
**Contexto**
Era possível projetar a plataforma assumindo equipamentos permanentes.
Essa abordagem aumenta dependência de hardware específico.
**Decisão**
Containers, Raspberry Pi, discos e máquinas virtuais poderão ser substituídos sempre que necessário.
**Consequências**
Benefícios:
- recuperação rápida;
- menor indisponibilidade;
- arquitetura resiliente.
Limitações:
- exige disciplina na documentação e nas políticas de backup.
# **ADR-012**
# **Comunicação Iniciada pelos Agents**
**Decisão**
Toda comunicação entre Seed Nodes e Control Plane será iniciada pelo Agent.
**Contexto**
Permitir conexões iniciadas pelo Control Plane exigiria abertura de portas adicionais, NAT reverso ou mecanismos de descoberta.
**Decisão**
Os Agents estabelecerão comunicação periódica e autenticada com o Control Plane.
**Consequências**
Benefícios:
- implantação simplificada;
- compatibilidade com redes domésticas;
- menor exposição de serviços;
- segurança aprimorada.
Limitações:
- comandos dependem da disponibilidade do canal iniciado pelo Agent.
# **ADR-013**
# **Arquitetura Orientada a Serviços**
**Decisão**
Cada responsabilidade principal será implementada como um serviço independente.
**Contexto**
Uma alternativa seria desenvolver uma aplicação monolítica.
Embora mais simples inicialmente, essa abordagem dificultaria evolução e manutenção.
**Decisão**
Portal, API, Scheduler, Alert Service, Monitoramento e demais componentes permanecerão modularizados.
**Consequências**
Benefícios:
- baixo acoplamento;
- manutenção simplificada;
- evolução independente;
- facilidade de testes.
Limitações:
- maior quantidade de componentes em execução.
# **ADR-014**
# **Entrega de Comandos por Polling na V1**
**Decisão**
Na versão 1.0, o Control Plane entregará comandos aos Agents através de uma fila de comandos durável, consultada periodicamente (polling) pelo Agent.
**Contexto**
Diversos documentos descrevem o fluxo lógico "Control Plane → Agent → Engine", enquanto o ADR-012 estabelece que toda comunicação é iniciada pelo Agent e que o Control Plane nunca abre conexões diretamente para o Node.
Era necessário definir explicitamente, para a V1, como um comando chega até o Agent sem violar o ADR-012.
Alternativas consideradas:
- polling de uma fila de comandos (Agent-iniciado);
- canal persistente WebSocket/SSE iniciado pelo Agent;
- conexão iniciada pelo Control Plane em direção ao Agent.
**Decisão**
O Agent consultará periodicamente um endpoint de fila de comandos do Control Plane, executará os comandos recebidos e confirmará o resultado.
O canal persistente (WebSocket/SSE), também iniciado pelo Agent, fica previsto para a V2 como evolução aditiva sobre o mesmo contrato de fila (ver Roadmap §13).
A conexão iniciada pelo Control Plane foi descartada por exigir portas de entrada nos Nodes, quebrar cenários com NAT/Nodes remotos e ampliar a superfície de segurança.
**Relação com o ADR-012**
Esta decisão refina o ADR-012 e o preserva integralmente: o polling é uma comunicação iniciada pelo Agent.
**Consequências**
Benefícios:
- respeita o modelo Agent-iniciado do ADR-012;
- funciona atrás de NAT e com Nodes remotos;
- fila durável, idempotente e auditável;
- comandos aguardam o Agent quando ele está offline;
- migração para o canal persistente é aditiva, sem redesenho.
Limitações:
- latência de entrega limitada ao intervalo de polling na V1 (mitigada pelo canal persistente na V2).
# **ADR-015**
# **Stack Tecnológica da V1**
**Decisão**
A versão 1.0 será implementada em Go (Control Plane e Agent) e React (Portal), sobre PostgreSQL 17 e ecossistema Docker.
**Contexto**
A documentação de engenharia (Development Guide e Coding Standards) é propositalmente agnóstica de linguagem. Entretanto, o Agent possui metas de recurso agressivas (CPU inferior a 2% em repouso, memória inferior a 150 MB) e deverá executar em Raspberry Pi 3B+ (ARM64), o que favorece uma linguagem compilada de baixo footprint.
Alternativas consideradas para o backend/Agent: Go, Python, Node/TypeScript.
**Decisão**
- Control Plane e Agent em Go (binário estático único, cross-compile ARM64 trivial, baixo consumo de memória).
- Portal em React com Tailwind CSS.
- Persistência em PostgreSQL 17; observabilidade com Prometheus, Grafana e Loki; Traefik como reverse proxy; empacotamento com Docker e Docker Compose (todos já fixados na documentação de Infraestrutura).
**Consequências**
Benefícios:
- footprint adequado aos Seed Nodes de baixo custo;
- distribuição simples (binário único) e cross-compilation direta para ARM64;
- ecossistema maduro para APIs, concorrência e observabilidade.
Limitações:
- define oficialmente tecnologias para a V1; alterações relevantes exigirão nova ADR.
# **ADR-016**
# **Fronteira entre Estado do Cluster (produto) e Observabilidade (infraestrutura)**
**Estado:** Aprovada

**Contexto**
A V1 subiu Prometheus + Grafana + node-exporter (ADR-015) e um dashboard "Cluster" no Grafana com contagens de Nodes/torrents/alertas. Ao mesmo tempo, o Portal já apresenta esse mesmo estado ao vivo, lendo a API. Surgiu a questão: por que três sistemas (Portal, Prometheus, Grafana) exibindo "saúde do Cluster"? Isso não deveria viver apenas no Portal/Control Plane?
A ADR-007 já mantém a observabilidade desacoplada do núcleo operacional; a ADR-006 fixa o PostgreSQL como estado oficial; a ADR-005 desacopla o Portal (consome só a API). O Monitoring.md §8 afirma que "o Grafana não armazenará informações permanentes do Cluster".

**Problema**
Há sobreposição real: o dashboard Grafana duplica o estado de domínio que já é responsabilidade do Portal/API. Isso confunde "onde está a verdade" e obriga o operador a alternar entre sistemas para a visão do dia a dia. É preciso definir a fronteira entre (a) estado/saúde do Cluster — dado de produto — e (b) observabilidade de infraestrutura — séries temporais, métricas de host, logs.

**Alternativas consideradas**
1. Status quo (ADR-007 literal): toda visualização, inclusive saúde do Cluster, via Grafana; Portal mínimo. Mantém a duplicação, torna o operador dependente do Grafana e contraria o Monitoring.md §8.
2. Concentrar tudo no Control Plane: o CP coleta, armazena e plota métricas; remover Prometheus e Grafana. Reinventa banco de séries temporais e scraping; perde PromQL/alerting maduros; alto custo.
3. Grafana para infra + Portal para domínio: mantém os três sistemas com fronteira; ainda exige operar e manter o Grafana apenas para gráficos de host.
4. Prometheus (+ Alertmanager) como motor de coleta/ingestão/alerta e o Control Plane/Portal como camada de visualização; **remover o Grafana** (recomendada/adotada).

**Decisão adotada** (alternativa 4)
- **Prometheus** permanece como motor **desacoplado** de coleta e armazenamento de métricas (scrape do `/metrics` do controller e dos node-exporters); é a fonte oficial de métricas. **Alertmanager** trata alertas por limiar de métrica (ex.: disco, temperatura, SMART).
- **Remover o Grafana.** A visualização de métricas passa a ser responsabilidade do **Portal** (produto), com uma biblioteca de gráficos.
- Para preservar a ADR-005 ("Portal só fala com a API"), **o Portal não consulta o Prometheus diretamente**: o **Control Plane** expõe endpoints de métricas (`/api/v1/metrics/...`) que consultam o Prometheus (PromQL) no servidor e devolvem séries em JSON. Fluxo: Portal → API → Prometheus.
- **Alertas unificados no Portal:** alertas de domínio seguem no Alert Worker (ADR-008); alertas por métrica do Alertmanager chegam ao CP por webhook e são persistidos como alertas SeedGrid, aparecendo no mesmo lugar.
- Estado/saúde de domínio do Cluster (Nodes, torrents, alertas, eventos) continua vindo do Postgres via API (reafirma ADR-006). Retenção do Prometheus configurada.
- **Loki (logs) — diferido com gatilho de reavaliação, não pendência aberta do MVP.** Sem Grafana, o Loki exigiria um visualizador (Grafana de volta ou página de logs no Portal) e um shipper por Node — custo de RAM/CPU/disco alto no RPi de 1 GB — para resolver busca de log cross-host, que a escala atual (2–3 Nodes, um operador) cobre com `journalctl`/`docker logs`. **Reavaliar quando:** a frota crescer (~5–10+ Nodes) e a busca por SSH virar dor recorrente, **ou** surgir necessidade de alerta por padrão de log. Se implementado, deve ser **Portal-native** (Portal → CP → Loki, como as métricas). Meio-termo barato até lá: persistência do journald + rotação de log do Docker.

**Consequências**
Benefícios:
- UI única e coesa (Portal), com auth/UX do produto; menos superfície operacional (sem Grafana);
- Prometheus segue no que faz melhor (scrape, TSDB, PromQL, alerting), permanecendo desacoplado e substituível;
- fonte única de verdade do estado do Cluster (Postgres via API); métricas via CP mantêm o Portal falando só com a API.
Limitações:
- o Portal passa a construir e manter gráficos (perde-se a exploração ad-hoc e a biblioteca de dashboards do Grafana);
- o Control Plane ganha acoplamento à API de consulta do Prometheus (trocar de TSDB exigiria reescrever os endpoints de métrica);
- reverte parcialmente a ADR-007 no ponto da visualização (dashboards passam a viver no produto), embora a coleta permaneça desacoplada.

**Relação com outras ADRs**
- Refina e **supera parcialmente a ADR-007**: mantém a coleta de observabilidade desacoplada (Prometheus), mas move a **visualização** para o núcleo do produto (Portal), revertendo o ponto da ADR-007 que mantinha dashboards fora do Control Plane. Remove o Grafana.
- Ajusta a ADR-015: **remove o Grafana** da stack, mantém o Prometheus e **adiciona o Alertmanager**; Loki adiado. Demais tecnologias inalteradas.
- Reafirma a ADR-005 (Portal só fala com a API — o CP é o gateway para o Prometheus) e a ADR-006 (Postgres como estado oficial).

# **ADR-017**
# **Onboarding zero-touch de Seed Node (join token, porta atribuída pelo CP, auto-registro no Prometheus)**
**Estado:** Aprovada

**Contexto**
Colocar um Seed Node novo (ex.: o primeiro Raspberry Pi) no Cluster hoje exige vários passos manuais e propensos a erro: copiar o token de registro, **escolher uma porta BitTorrent única** por Node, definir a senha da WebUI do qBittorrent, **adicionar o alvo no Prometheus** à mão e criar o port-forward no firewall. O operador quer o oposto: **rodar um comando no Node e ele aparecer no Control Plane já pronto**. As decisões existentes ajudam — ADR-012 (Agent inicia a comunicação), ADR-006 (Postgres é o estado oficial), ADR-016 (Prometheus é o motor de coleta, o CP é o gateway) — mas não cobrem o *provisionamento* de um Node.

**Problema**
Definir um fluxo de onboarding automático e seguro: como o Node prova que pode entrar; quem decide a porta BitTorrent (e como reaproveitá-la quando um Node sai); como o Node passa a ser raspado pelo Prometheus sem edição manual de config; e como o Node obtém o binário/artefatos sem depender do repositório privado.

**Alternativas consideradas**
1. Manter tudo manual (status quo): simples de código, mas não escala nem com 3 Nodes; contraria o objetivo do operador.
2. Token único compartilhado para todos os Nodes: já existe (`SEEDGRID_AGENT_REGISTRATION_TOKEN`), mas um segredo para toda a frota é frágil e não rastreável.
3. **Join tokens one-time (estilo kubeadm)** emitidos pelo CP, porta atribuída pelo CP, auto-registro no Prometheus via file-based service discovery, artefatos servidos pelo CP (recomendada/adotada).
4. Porta dinâmica com qBittorrent em rede host (sem mapeamento fixo): destravaria a porta pós-registro, mas reduz o isolamento de rede do container; preterida em favor de manter bridge com a porta entregue **antes** do `compose up`.

**Decisão adotada** (alternativa 3)
- **Join token one-time:** o operador gera um token descartável no CP (endpoint protegido; futuramente botão no Portal), com validade e uso único. O onboarding usa esse token — não o segredo compartilhado da frota. O token é o "hash de geração/verificação": o Node o apresenta, o CP valida e o consome.
- **Porta BitTorrent atribuída pelo CP:** o Agent não escolhe mais a porta. No **enroll**, o CP atribui a **menor porta livre** a partir de `6881`, considerando Nodes ativos (não removidos) **e** reservas pendentes. Ao remover um Node, sua porta volta ao pool (reuso) — evita a sequência escapar ao infinito com entra/sai de Nodes. A porta é devolvida no enroll, **antes** de subir o qBittorrent (mantém a rede bridge com mapeamento fixo — alternativa 4 preterida).
- **Auto-registro no Prometheus (file_sd):** o `prometheus.yml` passa a usar `file_sd_configs`; o CP **escreve o arquivo de targets** (num volume compartilhado com o Prometheus) sempre que a lista de Nodes muda (registro/heartbeat/remoção). O Prometheus recarrega o arquivo sozinho — sem editar config nem reiniciar. É o "trigger de Node novo → cadastra no Prometheus".
- **Segredos:** a credencial do Agent continua **emitida pelo CP** no registro (ADR-012, modelo atual). A **senha da WebUI do qBittorrent é gerada no próprio Node** e nunca sai dele (o CP não a conhece nem precisa). O único segredo que o operador injeta é o join token.
- **Entrega dos artefatos:** o CP (via nginx do Portal) serve o **script de bootstrap** e o **binário ARM64** do Agent. O Node roda um único comando (`curl … | sh`), sem clonar o repositório privado. Evita também o lixo de build (toolchain Go/cache) no Node.
- **Fluxo:** o script chama `POST /api/v1/agent/enroll` (com o join token) → o CP consome o token, reserva a menor porta livre e devolve `{bt_port, registration_token}` descartável → o script sobe o qBittorrent nessa porta e grava o `registration_token` no env do Agent → o Agent registra normalmente (ADR-012), o CP finaliza o Node com a porta reservada e passa a servi-lo no Prometheus. Resta ao operador apenas o **port-forward no firewall** (agnóstico, tarefa dele — o Portal mostra IP:porta).

**Consequências**
Benefícios:
- onboarding de **um comando**; o Node aparece no CP já com porta, credencial e métricas;
- porta como recurso gerenciado pelo CP (fonte única, com reuso), eliminando colisões manuais;
- Prometheus sempre coerente com a frota, sem edição de config;
- segredos com menor superfície: join token descartável + senha do engine local ao Node.
Limitações:
- mais estado/superfície no CP: tabela de join tokens, reserva/atribuição de porta e escritor de targets do Prometheus;
- exige um **volume compartilhado** entre os containers do CP e do Prometheus (o CP escreve o arquivo de targets);
- servir binário/script pelo CP acopla o artefato ARM64 ao deploy do CP.

**Adendo (set/2026) — o storage deixou de ser decisão de uma vez só.** O `bootstrap-node.sh` decidia o `/data` num único instante: se o disco não estava presente quando o comando rodou, o Node nascia com `/data` no cartão e **não havia caminho de volta** — pior, a auto-recuperação de hotplug (regra udev) só era instalada quando `/data` já tinha UUID naquele momento, de modo que o Node com disco anexado depois nascia justamente sem a proteção que mais lhe serve. Caso real: no onboarding do node-04 o HD foi plugado 9 minutos após o bootstrap, e o storage teve de ser montado, o container recriado e a regra udev instalada na mão. O script passa a aceitar **`--storage-only`**, que reexecuta apenas o passo de storage (montagem por UUID no `fstab`, instalação da recuperação de hotplug e recriação do Engine para o bind enxergar o disco novo), é idempotente e descobre o CP pelo `/etc/seedgrid-agent.env` — sem enroll, sem token. Não altera o fluxo de onboarding nem o contrato do enroll.

**Adendo (out/2026) — o enroll também devolve o índice do Node (#202).** A resposta de `POST /api/v1/agent/enroll` ganha `node_index`, ao lado de `bt_port`: a posição do Node na sequência de portas (`bt_port − 6880`, calculada por `node.IndexForPort`). É a origem do sufixo IPv6 determinístico do Node (ADR-038). O campo é aditivo — quem só lê `bt_port` não percebe — e não há estado novo: o índice herda a unicidade da porta (única entre Nodes ativos e reservas vivas) e volta ao pool junto com ela quando o Node é removido.

**Adendo (out/2026) — o bootstrap declara se o Node tem disco dedicado (#221).** O Agent exigia que `/data` fosse um *mountpoint* em **todo** Node, enquanto o bootstrap aceitava `/data` no disco do sistema quando não havia disco: um Node de disco único (VM, mini PC, NAS) ficava para sempre com `storage_ok = false`, fora do Scheduler e com `NODE_STORAGE` aberto, sem o operador ter feito nada de errado. A exigência só faz sentido para quem **declarou** disco dedicado, e inferir isso em tempo de execução erraria justamente onde a regra protege — um disco que não montou parece uma pasta local. O bootstrap passa a **gravar a declaração** no env do Agent, `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT`: `true` quando montou ou encontrou disco dedicado em `/data` (e sempre no `--storage-only`), `false` quando `/data` ficou no disco do sistema, com aviso no terminal. O Agent a lê como dado e nunca a reavalia; **ausente vale `true`**, então os Nodes existentes não mudam e o erro, se houver, é para o lado seguro. Uma 2ª passada do bootstrap nunca rebaixa um Node que já declarou disco. Não altera o contrato do enroll nem o do heartbeat (`storage_ok`/`storage_detail` seguem como eram) e não toca o Control Plane. Ver `docs/06 - Storage.md` §35.3.

**Relação com outras ADRs**
- Respeita a **ADR-012** (Agent inicia a comunicação — o Node puxa o enroll e o registro; o CP nunca conecta no Node).
- Estende a **ADR-016**: o Prometheus segue como motor desacoplado; adiciona file-based service discovery alimentado pelo CP.
- Reafirma a **ADR-006** (a porta e o estado do Node vivem no Postgres) e a **ADR-005** (o operador opera pelo Portal/API; artefatos servidos pelo front do produto).

# **ADR-018**
# **Migração de torrents entre Seed Nodes (cópia física, pull direto orquestrado pelo CP)**
**Estado:** Aprovada

**Contexto**
O operador precisa **realocar torrents entre Seed Nodes**: migrar o conteúdo de Nodes de teste para hardware novo (ex.: o primeiro Raspberry), esvaziar um Node com problema de saúde (drain) antes de removê-lo (ADR de deleção de Node), ou rebalancear quando um disco enche. Hoje um torrent é adicionado a um Node pelo Scheduler e lá permanece; não há como movê-lo.

**Problema**
Mover um torrent (metadados + **dados baixados**) de um Node origem para um Node destino, de forma orquestrada pelo Control Plane, decidindo (a) como o dado trafega, (b) de onde vem o metadado (`.torrent`), (c) como não perder dados se a migração falhar, e (d) como isso convive com o princípio "nada de comunicação direta entre Nodes".

**Alternativas consideradas**
1. **Re-download via swarm:** o CP adiciona o torrent no destino, que baixa do swarm (inclusive do origem, que seeda); ao completar, remove do origem. Respeita "nada direto entre Nodes" (o transporte é o próprio BitTorrent) e é simples. **Rejeitada** porque re-baixa os dados: gasta banda/tempo e, em **tracker privado**, registra um novo download (afeta ratio; risco de regra de hit-and-run).
2. **Relay dos dados via CP:** o CP intermedia a cópia origem→CP→destino. Mantém os Nodes sem se falarem, mas transforma o **Control Plane em data-plane** — GBs passando por ele, virando gargalo e ponto único. **Rejeitada** (anti-padrão; decisão explícita do operador: "nada de relay dos dados via CP").
3. **Cópia física com pull direto destino←origem, orquestrado e autorizado pelo CP** (recomendada/adotada).

**Decisão adotada** (alternativa 3)
- **Cópia física, sem re-download:** os dados já baixados são transferidos do disco do origem para o destino; o destino faz **recheck** (verifica 100% contra os hashes) e passa a **seedar imediatamente**, sem baixar do swarm. Preserva ratio em tracker privado e é rápido (LAN).
- **Transporte: pull direto destino←origem, orquestrado pelo CP.** O CP não intermedia os bytes. Fluxo:
  1. Pré-requisito: o CP tem o `.torrent` persistido (ver "Metadado" abaixo).
  2. O CP emite uma **autorização de transferência de uso único** (token + endpoint do Node origem + hash do torrent) e comanda o **Agent destino** a puxar os dados.
  3. O **Agent origem** **para o torrent** e só então expõe, temporariamente e **apenas para aquele token/torrent**, os arquivos daquele torrent num canal autenticado (HTTP no Agent, escopo mínimo). O **Agent destino** puxa para o seu storage path. *(**Refinado após a issue #8:** a parada foi acrescentada porque a cópia lia dados que o Engine ainda gravava. A ordem importa — a pausa acontece **antes** de o token ser autorizado, então não há janela em que o destino consiga puxar dado em voo; sem confirmação do estado parado o comando falha, e a migração aborta sem perda. Se a migração falhar depois, o **CP retoma** o torrent na origem, exceto quando ele está segurado pela admissão, cuja decisão é do Admission Worker, ADR-021/024.)*
  4. O destino adiciona o `.torrent` apontando para os dados copiados e faz **recheck**; ao reportar `SEEDING`, o CP considera a cópia válida. *(**Estendido pela ADR-031:** o critério passou a ser "o destino tem pelo menos o que a origem tinha" (`min_progress`), para suportar migração de torrent **incompleto**. Torrent completo se comporta como descrito aqui.)* *(**Movido pela #87:** quem espera o recheck e confere o critério deixou de ser o Agent — é o CP, observando o torrent no destino; o `DONE` do `migrate_pull` passou a significar "cópia feita, recheck disparado". Ver o **adendo ao fim desta ADR**.)*
  5. Só então o CP comanda o **origem a remover** o torrent (com dados) e atualiza o estado (`torrents.node_id` → destino; evento `TORRENT_MIGRATED`).
- **Exceção explícita e escopada ao "nada direto entre Nodes":** o princípio vale para a **operação em regime** (Nodes não coordenam entre si; quem decide é o CP). Uma **migração pontual, iniciada pelo operador, autorizada por token de uso único emitido pelo CP e restrita a um torrent**, é um caso à parte — o CP orquestra e autoriza; o dado trafega direto por eficiência. Documentado aqui como exceção consciente.
- **Metadado (`.torrent`):** o CP passa a **persistir o `.torrent`** (Postgres, `bytea` — casa com ADR-006, arquivos de KB a poucos MB). Para torrents vindos de magnet, o Agent **exporta o `.torrent` do engine** (`BT_backup`) depois que os metadados resolvem e envia ao CP. Assim todo torrent é migratável e o Cluster é resiliente (um Node novo pode re-semear a partir do CP).
- **Segurança de dados / rollback:** o origem **só remove após o destino confirmar `SEEDING`** *(ADR-031: confirmar **`min_progress`** — ter ao menos o que a origem tinha)*. Se a transferência ou o recheck falharem, o torrent permanece no origem (migração é abortada, sem perda). Operação idempotente por (torrent, destino). *(**Refinado após a issue #79:** ao abortar, o CP também **descarta a cópia deixada no destino** (`remove_torrent` com dados). "Sem perda" continua valendo — a cópia boa é a da origem, que é preservada e retomada; o que ficou no destino é rascunho de uma transferência que não valeu, e antes ninguém o removia (11 GB órfãos no incidente que originou a issue, mais um segundo registro do mesmo hash no Cluster). A limpeza só ocorre quando o pull chegou a ser enfileirado: migração que morre antes disso não copiou nada, e mandar um remove ali arriscaria apagar uma cópia legítima preexistente no destino.)*
- **Escopo de operação:** migrar **todos** os torrents de um Node (drain) ou um **subconjunto selecionado**; gatilhos manuais (operador) e, no futuro, automáticos (saúde/disco, via Scheduler).

**Consequências**
Benefícios:
- migração eficiente (sem re-download; preserva ratio em tracker privado); o CP não vira data-plane; drain habilita a deleção limpa de Node; `.torrent` no CP dá resiliência (re-provisionar Node do zero).
Limitações:
- introduz uma **exceção** (documentada e escopada) ao "nada direto entre Nodes"; o Agent ganha uma capacidade nova (servir/puxar arquivos sob autorização do CP), com superfície a proteger (token de uso único, escopo por torrent, TTL curto); o CP passa a guardar os `.torrent` (mais estado, porém pequeno).

**Relação com outras ADRs**
- Reafirma a **ADR-006** (Postgres é o estado oficial — o `.torrent` persistido vive nele) e a **ADR-012** (o CP orquestra; os Agents agem sob comando — o pull é iniciado pelo Agent destino a mando do CP).
- Abre **exceção escopada** ao princípio de isolamento entre Nodes (Architecture/Vision) para migração autorizada; a operação em regime permanece sem comunicação direta entre Nodes.
- Complementa a deleção de Node: o **drain** (migrar tudo) é o passo limpo antes de remover um Node.

> **Adendo (#87) — o recheck do destino é observado pelo Control Plane; o `migrate_pull` não o espera.**
>
> **Problema.** O `migrate_pull` esperava o recheck do destino num laço dentro do próprio comando (o watchdog da #79, com teto de 2 h). O Poller do Agent executa o lote de comandos **em série** e só busca o próximo quando o atual termina — então, durante a espera, o Node inteiro ficava surdo: `pause`/`resume`/`remove`/`update_agent` de **outros** torrents, e até o próximo lote. O limite de 2 h nem era técnico: era o preço de bloquear o Node.
>
> **Alternativas.** (1) *Comando assíncrono* (`IN_PROGRESS` + consulta de desfecho): muda o contrato de comandos da ADR-014 — estado novo de comando, novo canal de consulta. (2) *Lote com concorrência limitada*: dá concorrência ao Poller e exige serialização por torrent. (3) *Tirar a espera do comando* — **adotada**: o CP já é quem decide o desfecho da migração (só ele remove a origem), então passa a ser também quem espera o recheck. Menor mudança de contrato, e a única das três que não toca o Poller.
>
> **Decisão.**
> - O `migrate_pull` responde `DONE` assim que a cópia terminou e o recheck foi disparado (e o torrent iniciado, #79). **`DONE` deixou de significar "o destino está verificado".**
> - O reconciler passa a observar o torrent **no Node destino** — a linha viva do hash ali, alimentada pelo inventário (o `SyncStates` a adota assim que o Engine do destino tem o hash) — e só então conclui. Comprovado = **leitura posterior ao `DONE`** + estado que **prova o progresso** (`SEEDING`, `DOWNLOADING`, `QUEUED` ou `PAUSED` — lista de permissão; nunca `CHECKING`, `MOVING`, `UNKNOWN` ou `ERROR`) + progresso ≥ piso (`min_progress` − 0,5 %, ADR-031). A condição de estado é necessária: durante o recheck o qBittorrent reporta como progresso o avanço da **verificação**, não o que o torrent tem. O Agent anterior só exigia `SEEDING` no piso de fallback (0,999); com piso menor aceitava o progresso da própria verificação.
> - **O watchdog da #79 mudou de endereço, não de regra.** Estagnação (default **10 min** sem o progresso subir) → falha com o diagnóstico de sempre, *"recheck no destino travado: sem avanço há Xm"*. Teto absoluto, contado do `DONE` do pull (default **6 h**, antes 2 h) → *"destino não atingiu o progresso da origem"*. O efeito é o de antes: `fail()` — a origem é retomada e a cópia do destino é descartada, sem perda. Ambos são tunáveis com hot-reload (ADR-023): `migration.recheck_stall_window` e `migration.recheck_max_wait`. O estado do watchdog (melhor progresso, instante do último avanço) é efêmero, em memória, como o do Admission Worker: reiniciar o CP só abre uma janela nova de estagnação — nunca derruba uma migração saudável — e o teto vem do banco.
> - O prazo do token (30 min) passa a cobrir só a **transferência**. Antes, como a espera do recheck ficava dentro do comando, ele cortava a migração aos 30 min mesmo com o recheck andando — o teto de 2 h nunca era alcançável.
>
> **Contrato CP × Agent e compatibilidade.** Um campo **aditivo** no payload do `migrate_pull`: `async_recheck: true` — "quem observa o recheck sou eu". O CP **não ramifica** pela versão do Agent nem pelo tipo da resposta: observa o estado do torrent nos dois casos.
>
> | | Agent antigo (ignora o campo) | Agent novo |
> |---|---|---|
> | **CP novo** | espera o recheck e responde `DONE` já verificado; o CP encontra o destino pronto e conclui na leitura seguinte do inventário (≤ ~15 s). Nada muda para o operador — o Node segue ocupado durante o recheck até o Agent ser atualizado | responde `DONE` cedo; o CP observa e conclui |
> | **CP antigo** | contrato original, intacto | não recebe o campo → **mantém a espera no Agent** e só responde `DONE` com o destino verificado |
>
> A célula "Agent novo + CP antigo" é a razão de o campo existir: sem ele o Agent devolveria `DONE` cedo a um CP que o lê como "verificado" e removeria a origem sem a prova — o invariante central desta ADR. Rollout normal (ADR-027: o CP publica o binário, logo é atualizado primeiro) não exerce essa célula, mas a segurança dos dados não depende de a ordem ser respeitada. O caminho legado do Agent (espera + constantes do watchdog) fica até o contrato antigo não precisar mais ser atendido.
>
> **Disputa pelo mesmo torrent.** Sem o bloqueio, um comando sobre o torrent seria executado no destino **durante** o recheck (antes ficava na fila, atrás do `migrate_pull`): pausar derruba a verificação, remover apaga a cópia que o CP está prestes a dar por válida. O CP não impedia ações durante migração — só a admissão (ADR-021/024, `notMigratingPred`) e só sobre a linha da origem. Passa a recusar com **409 `migration_in_flight`** as ações do operador (`pause`/`resume`/`recheck`/`remove`) sobre (a) o torrent migrado e (b) a **cópia do mesmo hash no destino**, enquanto a migração está `PENDING`, e a migração dessa cópia de volta (`ErrAlreadyMigrating`). Migração `QUEUED` não trava nada: nada mexeu ainda, e um drain pode esperar na fila por horas. O Agent preserva a ordem do lote (serial): um comando para o mesmo hash roda depois do `Start` do pull, nunca no meio dele.
>
> **Consequências e limites.**
> - **A cópia ainda ocupa o Poller do destino** (proporcional ao tamanho e à banda do disco). O recheck era o único processo longo e sem tempo previsível do Engine; a cópia segue dentro do comando. Tirá-la também exigiria comando assíncrono (alternativa 1), não adotado.
> - Com o recheck fora do comando o teto pôde subir; o custo de um teto alto é a **vaga de `migration.max_concurrent`** ficar ocupada até lá (a fila de migrações anda mais devagar), não um Node surdo.
> - A verificação passa a depender de o inventário do destino chegar ao CP (a cada ~15 s). Sem leitura, vira estagnação e a migração aborta sem perda.
> - Nenhuma mudança de schema: o estado da verificação não é persistido, e `PENDING` passa a cobrir a transferência **e** a verificação.
> - **Sem saída, o 409 seria uma armadilha** — e ele agora dura o tempo da verificação (até 6 h). Ver "Cancelamento pelo operador", abaixo.
> - A verificação é legitimamente longa (horas, em disco lento). O backstop de migração `PENDING` parada que a ADR-033 prevê (`maintenance.migration_stuck_after`, 30 min) **não está implementado** — o Maintenance Worker atual só faz retenção —; se vier, tem de respeitar a janela da verificação, senão derruba a migração que o watchdog daqui mantém de pé.
>
> **Cancelamento pelo operador (`POST /api/v1/migrations/{id}/cancel`).** A recusa das ações durante a migração só é aceitável se o operador tiver como sair dela: na `main` não havia como cancelar uma migração (só criar e listar), e com a verificação podendo durar horas o torrent ficaria preso. O cancelamento é o **mesmo encerramento seguro do watchdog** — um único caminho (`abort`) serve o watchdog, o prazo do token, a falha do pull e o operador: migração `FAILED` com o motivo, origem retomada, cópia do destino descartada.
> - **Estado: `FAILED` com motivo `cancelada pelo operador`, não um estado novo.** O enum não muda (não há `CHECK`; nenhuma migration), e é o que importa: muitos leitores tratam "terminal" como `COMPLETED`/`FAILED` — a reconciliação de deleções do inventário (`reconcileMissing`), o Portal, a poda —, e um terceiro estado seria lido como "em voo" por todos os que não o conhecem. A API expõe `cancelled: true` (derivado do motivo) para o cliente distinguir "cancelada" de "falhou"; o Portal mostra "Cancelada".
> - **Permissão `torrents:migrate`** (Administrator e Publisher) — a mesma de migrar: cancelar descarta a cópia do destino, é o oposto da migração, e não deve ficar atrás de uma permissão mais fraca. Auditado (ADR-025): *"Cancelou a migração do torrent «X» do Node «A» para o Node «B»"*; as tentativas rejeitadas (409) também ficam, com o rótulo da rota.
> - **Em voo = `QUEUED` ou `PENDING`.** Qualquer outra coisa responde **409**: `migration_finished` (concluída, ou falhou sozinha) ou `migration_already_cancelled`. **Repetir é seguro** — o efeito (baixar comandos, enfileirar a limpeza) acontece uma única vez; a repetição só devolve o 409.
> - **O que acontece em cada fase.** `QUEUED`: nada foi enfileirado; vira `FAILED` e nunca é promovida. `PENDING` com `migrate_serve`/`migrate_pull` **ainda não reivindicados** pelo Agent: são baixados como `FAILED` para **nunca rodarem** (um pull esquecido na fila copiaria GBs de um torrent que ninguém mais quer migrar), e como nada rodou não há origem a retomar nem cópia a descartar — mandar um `remove_torrent` ao destino arriscaria apagar uma cópia legítima. **Durante a cópia** (o Agent destino está dentro do `migrate_pull`): o comando **não é interrompido** — o Agent não tem canal de cancelamento no meio de um comando e a ADR-014 não o prevê. A migração vira `FAILED` na hora, a origem é retomada, e o `remove_torrent` (com dados) é enfileirado **atrás** do pull no Poller serial do destino: a cópia termina e é descartada logo em seguida, o mesmo efeito do watchdog. **Durante a verificação do recheck**: igual, e o `remove_torrent` interrompe o recheck e libera o disco.
> - **Cancelar vs concluir nunca vencem juntos.** O reconciler concluindo e o operador cancelando podem se cruzar; sem proteção, um cancelamento que chegasse no instante em que a migração conclui sobrescreveria `COMPLETED` e mandaria apagar a **única** cópia (a do destino), com a origem já removida. As duas transições são **condicionais** (`PENDING → COMPLETED` e `QUEUED/PENDING → FAILED`, no `UPDATE`): quem perde não tem efeito algum — o `complete()` perdedor dá rollback (a reposição do torrent é desfeita e o `remove` da origem, enfileirado por último, nunca existiu); o `abort()` perdedor não enfileira nada. A transição, a baixa dos comandos e a limpeza commitam numa **única transação**.
> - **Limite:** o token de transferência (30 min) não é revogado no cancelamento — o Agent origem não tem comando de revogação. Segue de uso único e escopado ao hash, e a origem já foi retomada; um pull que ainda estivesse em curso lê dados que mudam, mas será descartado.
> - **A recusa aponta a saída.** O 409 `migration_in_flight` diz que dá para cancelar e **qual** migração (inclusive quando a ação foi sobre a cópia do destino, cuja migração o operador não teria como descobrir): *"…aguarde a conclusão ou cancele a migração (Portal: Alertas & Eventos → Migrações recentes → Cancelar migração; API: POST /api/v1/migrations/{id}/cancel)"*.
# **ADR-019**
# **Publicação de conteúdo local: upload → criação do `.torrent` no Engine + seeding, com trackers do operador (streaming pelo CP)**
**Estado:** Aprovada

**Contexto**
O operador quer **publicar conteúdo próprio** (ISOs, arquivos) direto do PC pelo Portal: o Cluster gera o `.torrent`, anuncia e passa a **semear**, sem precisar de um cliente BitTorrent local nem de um `.torrent` pronto. Na publicação escolhe **público** ou **privado** — este último anuncia num **tracker do próprio operador** (announce URL + passkey). Hoje o SeedGrid só *consome* torrents (magnet ou `.torrent` já existente); não *cria*.

**Problema**
(a) Como os **dados** (potencialmente vários GB) trafegam do browser até o Node, sem o browser falar direto com o Node (princípio de isolamento); (b) **onde e como** o `.torrent` é criado; (c) como **anunciar** (público vs tracker privado com passkey) sem vazar o segredo; (d) **onde** guardar as credenciais de tracker; (e) qual Node hospeda.

**Alternativas de transporte consideradas**
1. **Browser → Node direto:** eficiente, mas **expõe o Node** ao navegador (quebra o isolamento; mesma objeção do "download direto"). **Rejeitada.**
2. **Staging no CP + Node puxa** (reusa o pull da ADR-018): robusto a Node ocupado/offline, mas o **CP acumula o arquivo inteiro em disco**. **Rejeitada como padrão** (pico de disco no CP para upload pontual).
3. **Streaming pelo CP** (browser → CP → Node em fluxo), **adotada:** o CP recebe o multipart e **repassa em fluxo** ao Agent (não persiste o arquivo). O CP fica no caminho dos bytes **apenas durante o upload** — data-plane **transitório**, não relay em regime. Aceito conscientemente: é uma operação **pontual, iniciada pelo operador**, análoga à exceção escopada da ADR-018 (o "não relay via CP" que a ADR-018 veda é o tráfego **contínuo** de migração entre Nodes, não um upload interativo do operador).

**Decisão adotada**
- **Transporte:** streaming `browser → CP → Node`. O CP lê as partes do multipart e as reempacota como **tar stream** para o Agent (reusa `extractTar` da ADR-018); o Agent extrai no storage (`/data`). Autorização por **token de uso único** emitido pelo CP; canal de **recepção** no Agent (mesma família do `transfer.Server` da ADR-018, agora com um endpoint de *push*), com **path confinado** ao storage e **nome sanitizado** (anti-traversal).
- **Criação do `.torrent`: o próprio Engine cria** (qBittorrent `torrentcreator`, `startSeeding=true`) — sem biblioteca externa. O Agent dispara a criação (`sourcePath` no `/data`, `trackers`, `isPrivate`), aguarda concluir, **exporta o `.torrent` gerado** e o devolve ao CP, que o **persiste** em `torrent_files` (`source='created'`, casa com ADR-018/ADR-006). O torrent já nasce **SEEDING** no Node.
- **Escolha do Node:** o **Scheduler** decide por padrão (mesma elegibilidade/score do add normal); o operador pode **forçar** um Node destino (bypass explícito do Scheduler, registrado na decisão) — útil para fixar num disco específico.
- **Trackers do operador (announce):** credenciais cadastradas **por operador** no Perfil (nome + announce URL contendo o passkey). Na publicação: **Público** (sem tracker — descoberta por DHT/PeX) **ou** um tracker cadastrado (**privado**, `isPrivate=true`, DHT off). O announce é guardado **em texto** (decisão do operador — sem cifragem em repouso) e **não é devolvido na listagem** da API (só nome + host mascarado); o CP injeta o announce apenas no comando ao Agent na hora de publicar.
- **Persistência:** nova relação `trackers` (por usuário, announce em texto, flag de default) no Postgres (ADR-006). O `.torrent` criado entra em `torrent_files` como qualquer outro → o conteúdo publicado é **migratável** (ADR-018) e re-provisionável.

**Consequências**
Benefícios:
- o Portal vira ponto de **publicação** (não só consumo); reusa o Engine para criar torrents (sem dependência nova) e a infra de transferência/`.torrent` da ADR-018; conteúdo publicado é migratável e resiliente.
Limitações/custos:
- o CP fica **no caminho dos bytes durante o upload** (transitório; não é relay contínuo, mas consome banda/CPU do CP no intervalo); o Agent ganha uma **capacidade de recepção** (superfície a proteger: token de uso único, TTL curto, path confinado); o CP passa a **guardar segredos do operador** (passkeys de tracker) em texto (decisão consciente — sem cifragem em repouso), mitigado por **não devolver o announce na API** (só nome + host mascarado).

**Relação com outras ADRs**
- **Estende a ADR-018:** mesmo princípio de canal de dados **direto e autorizado por token do CP**; aqui o CP é a *origem* (push em streaming ao Agent) em vez de orquestrar um pull entre Nodes. Reforça que o "não relay via CP" veda o tráfego **contínuo** entre Nodes, não o upload pontual do operador.
- Reafirma **ADR-006** (`.torrent`, `trackers` e segredos vivem no Postgres) e **ADR-012** (o CP orquestra; o Agent age sob autorização).
- Complementa **ADR-017** (o Perfil do operador ganha a gestão de trackers).

# **ADR-020**
# **Integração com tracker externo (UNIT3D): upload automático via API, com metadados enriquecidos (mediainfo + TMDB)**
**Estado:** Aprovada — **parcialmente substituída pela ADR-037**

> A ADR-037 substitui a parte desta ADR que fixou a **taxonomia e o formato de nome do samaritano como se fossem do produto**: eles passam a ser perfil de tracker, dado contribuível. O **contrato com a API UNIT3D v8** definido aqui (endpoint, campos obrigatórios, auth por token na query) continua valendo integralmente.

**Contexto**
Alguns trackers do operador são **sites UNIT3D** (ex.: samaritano.cc). Neles, publicar não é só anunciar num announce URL: o release precisa ser **cadastrado no site** (catálogo) via um formulário/**API** que exige metadados — categoria, tipo, resolução, IDs de TMDB/IMDB, mediainfo, descrição. Hoje a ADR-019 cria o `.torrent` e semeia, mas o cadastro no site é 100% manual e trabalhoso. O operador quer que o SeedGrid **poste automaticamente** no site ao publicar, preenchendo os campos por inferência (com conferência humana antes do envio).

**Problema**
(a) O `.torrent` precisa nascer **idêntico ao que o site espera** (etiqueta `source`, `private=1`, announce com passkey) senão o infohash não bate; (b) **mediainfo** só pode ser extraído **onde o arquivo está** (no Node), não no CP nem no browser; (c) os **IDs** de categoria/tipo/resolução do UNIT3D e o **TMDB/IMDB** precisam ser inferidos do nome/arquivo; (d) onde guardar as **credenciais** do site (API token) e a chave do **TMDB**; (e) como evitar **poluir o catálogo** do tracker privado com uploads de teste.

**Alternativas consideradas**
1. **Cadastro manual no site** (status quo): zero automação, muito atrito. **Rejeitada** como objetivo, mantida como fallback.
2. **Ferramenta externa** (ex.: SamUploadAssistant rodando à parte): resolve, mas fica **fora** do SeedGrid — duplica infra, não reusa o Engine/Scheduler nem o `.torrent` já criado. **Rejeitada.**
3. **Integração nativa no SeedGrid** (**adotada**): o próprio fluxo de publicação (ADR-019) ganha um passo opcional de **postar via API do tracker**. O Agent extrai o mediainfo (está com o arquivo); o CP infere IDs/consulta o TMDB e monta o payload; o operador **confere/ajusta** no Portal; o CP faz o `POST /api/torrents/upload`.

**Decisão adotada**
- **`.torrent` compatível:** a relação `trackers` ganha campos de integração — `source` (etiqueta, ex.: `SAMARITANO`), `api_url` (base do site) e `api_token` (segredo do operador, **texto**, nunca devolvido na API). Ao publicar num tracker com `source`, o Engine cria o `.torrent` com esse `source` (qBittorrent `torrentcreator` já aceita `source`), casando o infohash com o do site.
- **mediainfo no Node:** o Agent roda o binário `mediainfo` sobre o arquivo publicado e devolve o texto ao CP (é o único ponto com acesso aos dados — coerente com "só o Agent fala com o Engine/dados").
- **TMDB no CP:** consulta é um serviço **global** do Control Plane (chave `TMDB_API_KEY` na config do CP, não por tracker), usado para resolver `tmdb`/`imdb` a partir de nome+ano parseados do release.
- **Inferência + conferência:** o CP parseia o nome (resolução → `resolution_id`, tipo → `type_id`, categoria → `category_id` via mapas do UNIT3D) e pré-preenche o formulário do Portal; **o operador revisa e confirma** antes do envio (nada é postado sem ação explícita). *Desde a ADR-037 (#63), o parser devolve só rótulos (`1080p`, `WEB-DL`) e se é série; o perfil do site traduz o rótulo para o id daquele site.*
- **Nome enriquecido pelo mediainfo:** quando o nome do arquivo é pobre (ex.: só "A Bruxa 2015"), o parser não tem resolução/codec/áudio para o nome padronizado. O `release.EnrichFromMediaInfo` lê o texto do mediainfo (largura/altura → resolução; Writing library/Format → `x264`/`x265`/`H.264`…; Format+canais da 1ª faixa de áudio → `DDP5.1`…) e **preenche o que faltou** (o nome tem prioridade — pode trazer a **fonte**, que o mediainfo não conhece). Resultado: "A Bruxa 2015" → "A Bruxa 2015 1080p DDP5.1 x264". O `POST /api/v1/publish/infer` recebe o mediainfo e devolve o `standard_name` já enriquecido.
- **Envio:** `POST {api_url}/api/torrents/upload?api_token=…` (multipart) com o `.torrent` + campos. A resposta traz a URL de download do `.torrent` já com passkey (confirmação). O SeedGrid **já está semeando** desde a criação (ADR-019).
- **Verificação de infohash pós-publicação (#23):** logo após o cadastro, o CP baixa o `.torrent` **oficial** (a URL retornada) e compara o infohash com o que o Node está seedando. Se divergirem (o site regenera o `.torrent` com o próprio `source`/piece size — se o nosso saiu sem isso, o tracker responde `InfoHash not found` e o Node nunca conta como seed), o CP **re-seeda automaticamente**: atualiza a linha para o hash oficial (preserva `tracker_id`/`published`/`media_info`), persiste o `.torrent` oficial e orquestra a troca no Node — adiciona o oficial (recheck contra os dados já em `/data` → SEEDING, sem re-download) e remove o infohash antigo **sem apagar os dados**. Emite `TORRENT_RESEEDED` (WARNING). É a rede de segurança que evita o caso "publiquei mas o site mostra 0 seeds".
- **Anti-poluição:** o POST real fica **atrás de confirmação explícita** no Portal e só é exercitado com **releases reais**; não há uploads de teste no catálogo (UNIT3D não tem dry-run).

**Consequências**
Benefícios:
- publicar num tracker UNIT3D vira **um fluxo só** dentro do SeedGrid (cria + semeia + cadastra no site), reusando Engine/Scheduler/`.torrent` da ADR-019; metadados preenchidos automaticamente com o operador no controle.
Limitações/custos:
- o CP passa a guardar **mais um segredo** do operador (API token do tracker, em texto — mesma decisão da ADR-019 para o passkey) e uma chave de terceiro (TMDB) na config; o Agent ganha dependência do binário **`mediainfo`**; a inferência de IDs é **heurística** (por isso a conferência humana é obrigatória); acopla-se ao contrato da **API UNIT3D v8** (mudanças de versão do tracker podem exigir ajuste).

**Relação com outras ADRs**
- **Estende a ADR-019:** é o passo opcional "cadastrar no site" após criar+semear; reusa `trackers`, o Engine e o `.torrent` persistido.
- Reafirma **ADR-012** (o CP orquestra e detém segredos; o Agent age localmente sobre os dados) e **ADR-006** (segredos no Postgres).

# **ADR-021**

**Título:** Controlador de admissão de downloads (limite de DL slots por conta, cluster-wide)

**Estado:** Aprovada

**Contexto**
O tracker (site UNIT3D, ex.: samaritano.cc) limita **downloads simultâneos por CONTA** (DL slots), não por cliente. A passkey é **compartilhada** pelos Seed Nodes do cluster (mesma conta). Com muitos torrents incompletos anunciando ao mesmo tempo, o tracker recusa peers a quase todos (announce `status 164` / "Download slot limit reached") e o download do cluster inteiro **trava**. A fila interna do qBittorrent (`max_active_downloads`) é **por Node** e não enxerga o total do cluster; com `dont_count_slow_torrents=true` ela ainda **amplifica** o problema (downloads parados não contam no teto → o qBt ativa todos → *thundering herd*).

**Problema**
Garantir que no máximo **L** torrents estejam baixando no cluster INTEIRO (L = limite de DL slots da conta), promovendo o próximo quando um slot abre, sem estourar o tracker nem depender de coordenação entre Nodes (ADR-012 veda comunicação Node↔Node).

**Alternativas consideradas**
1. **Dividir o teto por Node estaticamente** (`max_active_downloads = L/N` em cada qBt): não acompanha L dinâmico, desperdiça slots com divisão inteira e não realoca quando um Node fica ocioso.
2. **Confiar só na rotação anti-slow do qBt** (`dont_count_slow_torrents`): é por Node e causou o *thundering herd* que originou o incidente.
3. **Controlador de admissão no Control Plane (adotada):** um Worker (como o Alert Worker) mantém o teto cluster-wide dando *hold* (pause) no excedente e promovendo pela fila; os Nodes só executam `pause_torrent`/`resume_torrent` (ADR-014, polling).

**Decisão adotada**
- **Worker no CP** avalia periodicamente o pool de torrents "não-parados" (`DOWNLOADING` + `QUEUED`) do cluster. Se `> L`, dá **hold** no excedente de **menor progresso** (`pause_torrent` + marca `torrents.admission_paused_at`); se `< L`, **promove** os holds de **maior progresso** (`resume_torrent` + limpa a marca). Conta `DOWNLOADING`+`QUEUED` (não só `DOWNLOADING`): parar só os ativos deixaria o qBt repor pelos `queuedDL`.
- **admission-hold ≠ user-pause:** o hold vive em `admission_paused_at` (coluna própria, **imune ao SyncStates**, que sobrescreve `status`). O controlador só retoma o que **ele** pausou; um torrent pausado pelo operador nunca é tocado nem conta como ativo.
- **L é dinâmico:** extraído da mensagem do próprio tracker (UNIT3D erro 164 "You may at most download **N** torrents"), que o Agent lê do Engine (`/api/v2/torrents/trackers`) e reporta ao CP; persistido no tracker default. Enquanto nenhuma msg foi observada (ou expira o TTL), cai no fallback de config `SEEDGRID_MAX_CLUSTER_DOWNLOADS`. **Sem L conhecido, o controlador não age** (nunca pausa o cluster às cegas). *A API `/api/user` do UNIT3D NÃO expõe o limite — validado ao vivo; por isso a fonte é a própria msg do announce.*
- **Cinto-e-suspensório no Agent:** default `max_active_downloads=2` por Node e `dont_count_slow_torrents=false` (o teto vale de fato; o excedente fica em `queuedDL`, que não anuncia como leecher).

**Consequências / Limitações**
- Se os L não-parados se concentrarem num Node (limitado por `max_active_downloads`), o cluster pode subutilizar levemente L — aceitável (o crítico é não estourar). Refino futuro: o Scheduler distribuir melhor.
- O L observado depende de o cluster ter batido no limite ao menos uma vez (senão usa o fallback).
- ~~Prioridade de promoção **por progresso** (maior primeiro) é a política V1.~~ **Substituída pela elegibilidade peer-aware (abaixo).**
- **Rotação por taxa saudável (#22, implementada):** no teto, um download ativo cujo `dl_speed` fica abaixo de `SEEDGRID_ADMISSION_MIN_DL_RATE` (default 50 KiB/s) por `SEEDGRID_ADMISSION_SLOW_GRACE` (5 min) contínuo cede o slot a um da fila que possa andar (troca 1:1, teto preservado); o rotacionado fica `SEEDGRID_ADMISSION_SLOW_COOLDOWN` (10 min) fora da fila de promoção para não oscilar (thrashing). Estado em memória do worker (efêmero). `MIN_DL_RATE=0` desliga. Evolução futura: expor o limiar num painel de configuração do CP.
- **Elegibilidade peer-aware + rotação por torrent morto (evolução, implementada):** o critério de promoção por progresso puro era **cego a peers** — um torrent de alto progresso mas **sem ninguém no swarm** (0 seeds e 0 peers) monopolizava um slot sem nunca completar, enquanto torrents com seeds apodreciam na fila (observado ao vivo: um a 70% com 0/0 preso, e holds com 1-2 seeds parados). Correção em duas partes: **(1) promoção peer-aware** — a fila de promoção ordena **viável primeiro** (`num_seeds>0 OR num_leechers>0`), depois maior progresso como desempate (termina antes o que está mais perto **entre os que conseguem baixar**). **(2) rotação por morto** — um download ativo com **0 seeds E 0 peers** por `SEEDGRID_ADMISSION_DEAD_GRACE` (default 3 min) contínuo é rotacionado na hora (gatilho primário, antes da regra de taxa) e vai pro **fim da fila** por `SEEDGRID_ADMISSION_DEAD_COOLDOWN` (default 30 min) — não volta mesmo sendo o de maior progresso, dando a vez aos demais; se a fila de prontos esgota (todos tiveram a vez), os parqueados voltam para não deixar slot ocioso ("ou até a fila rotacionar"). `DEAD_GRACE=0` desliga. Estado em memória (efêmero).
- **Conectados vs swarm + backoff do morto (refino, implementado jul/2026 — migration 00030):** a rotação por morto media "0 seeds E 0 peers" pelo **swarm** (scrape do tracker), e o Agent colapsava swarm e conectados num campo só (`adapter.pick`), então o CP nunca via quantos peers estavam **de fato conectados**. Isso deixava passar o pior caso: um tracker que anuncia um **seeder fantasma** (registrado no scrape, mas que nunca responde) dá `num_seeds=1` → o torrent **nunca** é dado como morto, e como é "viável" e costuma ter alto progresso, volta ao **topo** da fila a cada `SLOW_COOLDOWN`, queimando um slot do teto L indefinidamente sem baixar 1 byte (observado ao vivo: `Blade.Runner.2049` a 98,79% com 0 conexões por 5 dias, ciclando resume→hold a cada ~15 min). Correção em duas partes: **(1)** o Agent passa a reportar os conectados em campos próprios (`num_seeds_connected`/`num_leechers_connected`, colunas novas; `num_seeds`/`num_leechers` seguem sendo o swarm que o Portal exibe) e a `rotateDead` julga pelos **conectados** — `COALESCE(num_seeds_connected, num_seeds, 0)` cai no swarm para Agent antigo, então a mudança é **aditiva e retrocompatível**, sem janela de deploy; **(2)** **backoff por reincidência** — cada rotação por morto seguida do mesmo torrent dobra o cooldown (30m → 1h → 2h…) até `SEEDGRID_ADMISSION_DEAD_BACKOFF_MAX` (default 4h; 0 desliga), e conectar em algum peer zera a contagem: um morto crônico se afasta sozinho da fila em vez de reaparecer a cada cooldown fixo. **A fila de promoção (`promotableHolds`) segue julgando pelo swarm de propósito:** ela ordena torrents em **hold** (pausados), que têm 0 conectados por definição — usar o conectado ali zeraria o critério peer-aware e devolveria a ordenação por progresso puro, o bug que este ADR corrigiu.

**Benefícios**
- Resolve o *thundering herd*/stall de forma permanente e cluster-wide, respeitando ADR-012/ADR-014 (o CP decide, o Agent executa por polling); L sem hardcode.

**Relação com outras ADRs**
- Reafirma **ADR-012** (o CP orquestra; o Agent age) e **ADR-014** (comandos por fila durável/polling).
- Complementa o **Scheduler** (colocação) atuando na **admissão** (quando cada download pode rodar).

# **ADR-022**

**Título:** Activity Feed — trilha de atividade de orquestração (separada de Eventos e Alertas)

**Estado:** Aprovada

**Contexto**
As decisões que o Control Plane toma sobre os torrents — dar *hold* (pause) num download por causa do teto de admissão (ADR-021), promover o próximo da fila quando abre um slot, agendar um torrent num Node (Scheduler), e as transições de ciclo de vida (começou a baixar, completou, passou a seedar) — hoje são **invisíveis** ao operador: o `admission` worker e o `scheduler` não emitem nada, e as transições de status são só um `UPDATE` silencioso no `SyncStates`. Só há **Alertas** (problemas que exigem atenção) e **Eventos** (fatos de ciclo de vida de infra: Node registrado/removido, torrent criado/migrado/removido/re-seedado). Falta uma visão do "por baixo do capô" — ver a orquestração acontecendo em tempo real. Além do valor operacional (confiança, depuração), é uma vitrine forte para o projeto como open source.

**Problema**
Expor, numa trilha própria e ao vivo, as decisões de orquestração e as transições de ciclo de vida dos torrents, **sem** misturar com Alertas (ruído em quem monitora problemas) nem com Eventos (semântica de infra), e sem violar ADR-012 (o CP decide/registra; o Agent executa).

**Alternativas consideradas**
1. **Reusar a tabela `events` com uma coluna `category`:** menos código, mas mistura conceitualmente a trilha de orquestração com os eventos de infra e polui a página de Eventos; o operador pediu explicitamente algo **separado**.
2. **Só logs (`journalctl`):** já existem, mas não são navegáveis pelo operador nem apresentáveis no Portal; some a "mágica".
3. **Trilha dedicada `activities` + página própria no Portal (adotada):** um domínio novo, append-only, gravado pelo CP nos pontos de decisão, exposto por endpoint próprio e por uma página de timeline ao vivo — visual e conceitualmente separado de Alertas/Eventos.

**Decisão adotada**
- **Domínio próprio `activities`** (append-only): cada registro tem `kind` (ex.: `admission_hold`, `admission_resume`, `scheduled`, `downloading`, `seeding`), referências soltas a `torrent_id`/`node_id` (soft-delete preserva os nomes por JOIN), `torrent_name` denormalizado (sobrevive a hard-delete), `message` legível e `detail` (JSONB) com o *porquê* (motivo do hold, score/razão do Scheduler, etc.).
- **Gravado pelo CP nos pontos de decisão**, sempre **best-effort** (falha ao registrar nunca derruba a orquestração): no `admission` worker (`holdOne`/`promoteOne` — cobre excedente e rotação por taxa), no fluxo de criação (decisão do Scheduler) e no `SyncStates` (transições observadas → `DOWNLOADING`/`SEEDING`, comparando o status anterior).
- **Trilha separada de Alertas e Eventos**: tabela, endpoint (`GET /api/v1/activities`) e página do Portal próprios. Nada de reaproveitar `events`.
- **Somente leitura para o operador**; sem canal Agent→CP novo (respeita ADR-012/ADR-014). Atualização no Portal por **polling** curto (não há SSE na V1; mesmo padrão das demais telas).

**Consequências / Limitações**
- Trilha append-only cresce com o tempo; volume é baixo (só decisões/transições, não amostras). Retenção/poda fica como evolução futura (análoga à poda de `torrent_stat_samples`).
- Alguma redundância entre `admission_resume` (decisão) e a transição observada `downloading` (efeito) é possível; mitigada registrando a transição para `DOWNLOADING` só a partir de `QUEUED`/`CHECKING` (o "começou a baixar" genuíno), não de `PAUSED` (retomada, já coberta pela decisão).
- Não substitui logs nem métricas; é uma camada de **narrativa** da orquestração.

**Benefícios**
- Transparência da orquestração em tempo real (confiança operacional + depuração) e vitrine para open source, sem poluir Alertas/Eventos.

**Relação com outras ADRs**
- Dá visibilidade às decisões da **ADR-021** (admissão) e do Scheduler, e às transições de ciclo de vida.
- Respeita **ADR-012** (o CP decide e registra; o Agent apenas executa) e **ADR-014** (sem canal novo; leitura por polling).
- Complementa, sem substituir, a trilha de **Eventos** (infra) e o **Alerting** (ADR-008).

# **ADR-023**

**Título:** Configuração dinâmica de tunáveis via banco, com hot-reload (painel de configuração)

**Estado:** Aprovada

**Contexto**
Todos os tunáveis de comportamento do Control Plane — intervalos dos Workers, limiares do controlador de admissão (ADR-021: teto de fallback, taxa mínima, graces/cooldowns de rotação por taxa e por torrent morto), regras de alerta (offline-after, saturação de disco, retenção de amostras) — eram lidos **uma única vez no boot** a partir de variáveis de ambiente (`config.go`). Ajustar qualquer parâmetro exigia editar o `.env` e **redeploy** do container; além disso, vários números (intervalo do reconciler, do prometheus-writer) estavam **hardcoded**, sem sequer env var. O operador quer um **painel no Portal** para editar esses parâmetros e ver o efeito **sem redeploy**, cada um com um descritivo.

**Problema**
Tornar os tunáveis de comportamento **editáveis em runtime** (hot-reload), com fonte de verdade previsível e sem violar as convenções (banco só via migrations; config externa ao código; Portal só fala com a API; só o CP acessa o banco). Sem transformar o `config.go` num god-module nem espalhar números mágicos.

**Alternativas consideradas**
1. **Continuar só com env + redeploy:** simples, mas não atende o requisito (mudar exige rebuild/restart; não há painel).
2. **Arquivo YAML recarregado por watch de filesystem:** hot-reload sem banco, mas o arquivo não é acessível pelo Portal (que só fala com a API), some da UI e reintroduz estado fora do Postgres (a fonte oficial do estado é o banco — ADR-015).
3. **Catálogo em código + overrides no banco + Store atômico (adotada):** um catálogo de metadata no código (fonte única: tipo, default, unidade, descrição, limites, env de seed), uma tabela `settings` (KV) com os overrides, e um `Store` que resolve o valor efetivo e o aplica em runtime. Editável pelo Portal via API; estado no Postgres; hot-reload instantâneo (processo único).

**Decisão adotada**
- **Catálogo em código** (`internal/settings/catalog.go`): cada tunável é um `Spec{Key, Kind, Category, Label, Description, Unit, Default, EnvVar, Min, Max}`. É a **fonte única** de metadata — alimenta seed, validação, painel e o doc `25 - Business Rules.md`. Separação da memória *centralize-tunables-not-logic*: o catálogo centraliza os **parâmetros**; a **lógica** que os consome fica no domínio (Workers).
- **Tabela `settings`** (migration `00027`, KV `key/value/updated_at/updated_by`): overrides que sobrepõem os defaults do catálogo. Reset ao default = remover a linha.
- **Precedência: o banco manda; o env só semeia.** No 1º boot, para cada tunável ainda sem linha cujo `EnvVar` esteja setado e válido, o `Store.Load` grava esse valor como seed inicial (compat com os `.env` atuais). Depois disso, o Portal é a fonte de verdade — mudar o `.env` não afeta mais.
- **Store thread-safe com hot-reload** (`internal/settings/store.go`): cache dos overrides num ponteiro atômico (leitura lock-free nos loops dos Workers); `Set`/`Reset` validam contra o `Spec` (tipo + min/max), gravam no banco e trocam o ponteiro (copy-on-write) → a mudança vale já na **próxima passada** de qualquer Worker.
- **Workers lêem o valor por passada** (não capturam no construtor): admissão e alertas recebem o `Store`; os que têm ticker recriam o intervalo (`ticker.Reset`) quando o tunável muda. O reconciler e o prometheus-writer recebem um *provider* `func() time.Duration`.
- **API + Portal:** `GET /api/v1/settings` (agrupado por categoria), `PUT /api/v1/settings/{key}` (valida, grava, aplica), `DELETE /api/v1/settings/{key}` (reset ao default). Página **Configurações** no Portal, cada campo com descritivo do catálogo. *(A mudança de tunável é registrada na trilha de **Auditoria** — ADR-025; ver nota abaixo.)*
- **Infra e segredo continuam em env** (imutáveis): DB URL, tokens, senha admin, API keys, prom URL, http addr, log level, migration port permanecem em `config.go`, fora do painel.

**Consequências / Limitações**
- Valores efetivos corrompidos (ex.: edição manual no banco fora da API) caem no **default do catálogo** no getter — o Worker nunca quebra por config inválida.
- Hot-reload é in-process (o CP é um processo único — ADR-015); não há pub/sub. Se um dia o CP for HA (múltiplas instâncias), cada uma precisaria recarregar (evolução futura: LISTEN/NOTIFY ou TTL de cache).
- ~~Mudanças de tunável são auditadas como **Eventos** (`SETTING_UPDATED`/`SETTING_RESET`).~~ **Superado pela ADR-025:** mudança de tunável é **ação do operador** → registrada na trilha de **Auditoria**, não em Eventos (que passam a ser só fatos de domínio). As emissões `SETTING_UPDATED`/`SETTING_RESET` foram removidas.

**Benefícios**
- Ajuste operacional (inclusive dos tunáveis de admissão da ADR-021) **sem redeploy**, com descritivo por parâmetro; fim dos números mágicos; catálogo único espelhado na doc de regras de negócio.

**Relação com outras ADRs**
- Torna dinâmicos os tunáveis da **ADR-021** (admissão) e da **ADR-008** (alertas), antes fixos no boot.
- Respeita **ADR-015** (estado oficial no Postgres; CP como único acessor do banco) e as convenções de migrations/config externa.
- Auditoria reusa a trilha de **Eventos** (infra), distinta do **Activity Feed** (ADR-022) e do **Alerting**.

**Adendo (out/2026) — o Agent também lê o catálogo (ADR-039, #144).** O catálogo deixou de ser só do Control Plane: as chaves marcadas `PerNode` (`agent.heartbeat_interval`, `agent.command_poll_interval`, `agent.log_level` e as `engine.*` da fila do Engine) são **entregues ao Agent** na resposta do heartbeat e admitem **override por Node** (tabela `node_settings`, migration `00044`). A precedência acima (banco > env > default) ganha um degrau no topo — **override do Node > valor de cluster > env do Node > default do código** — e o env do Node passa de fonte a **fallback** (só vale quando o Control Plane não manda nada). Estas chaves **não têm `EnvVar` de seed** neste catálogo: o env que as semeia seria o do **Node**, não o do Control Plane, e quem o relata é o próprio Agent, uma vez por Node (ADR-039 §7). Os Workers do Control Plane não mudam; o que muda é que o consumidor de um tunável pode agora estar do outro lado da rede, e por isso o valor é validado **dos dois lados** (o `Spec` no CP, a sanidade no Agent).

# **ADR-024**

**Título:** Modo de operação Manual/Automático — controle do operador sobre a admissão (por-node, com gate global)

**Estado:** Aprovada — **implementada**. Versionamento dos builds (tags git → coluna "Agent") entregue (v1.0.0). Modo Manual/Automático: migration `00028` (`clusters.operation_mode`/`nodes.operation_mode`); worker de admissão filtra por modo de Node (full_auto = comportamento vigente; Manual conta no L); fix da assimetria no resume do operador (limpa `admission_paused_at`); API (`PUT /cluster/operation-mode`, `PUT /nodes/{id}/operation-mode`, gate exposto no `GET /nodes`); Portal (interruptor global Full auto/Flex + coluna "Modo" SG/Manual no Dashboard). Testado em DB isolado. Follow-up opcional: alerta "acima do L" quando todos os Nodes estão em Manual (hoje o sistema apenas não age).

> **Correção de conformidade (jul/2026).** A implementação original respeitava o pause do operador em **qualquer** Node, contrariando a cláusula "Node SG → sistema tem prioridade". O efeito era uma saída sem volta da orquestração: o torrent pausado na mão não contava em `countActive` (não é DOWNLOADING/QUEUED) nem aparecia em `promotableHolds` (que exige `admission_paused_at` marcado), então ficava num limbo — nem rodava, nem era candidato — e o cluster operava abaixo do L com a fila aparentando estar travada. Diagnosticado com 16 downloads nesse estado e nenhuma decisão de admissão no feed por ~30h. Corrigido com `adoptOrphanPauses` no worker de admissão; a redação do "fix da assimetria" abaixo foi qualificada por modo de Node para não reintroduzir a ambiguidade.

**Contexto**
A admissão de downloads (ADR-021, evoluída com rotação peer-aware) é **autônoma**: o controlador mantém o cluster ≤ **L** (limite de downloads simultâneos da conta do tracker, compartilhado pelos Nodes) dando *hold* no excedente e promovendo/rotacionando sozinho. O operador quer poder **assumir o controle** — parar um download na mão, ou promover/iniciar um manualmente — sem o sistema desfazer. Hoje a prioridade operador-vs-sistema é **inconsistente**: um *pause* manual é respeitado (o controlador ignora `status=PAUSED` com `admission_paused_at` nulo, não conta nem toca), mas um *resume* manual de um torrent segurado é **desfeito** pela "correção de drift" (a cada ciclo o worker re-pausa qualquer hold que voltou a `DOWNLOADING`, achando que o próprio hold escapou).

**Problema**
Dar ao operador controle **explícito e previsível** sobre a admissão, com uma regra de prioridade clara, **sem quebrar o invariante ≤ L** do tracker (estourar o L faz o tracker recusar peers e travar downloads).

**Alternativas consideradas**
1. **Só interruptor global (cluster):** simples e preserva o L trivialmente, mas tudo-ou-nada — o operador não controla um Node específico mantendo o resto automático.
2. **Só por-node:** granular e intuitivo na UI, mas o L é da conta (cluster-wide) — um node manual pode estourar o teto; e não há um estado de topo deliberado ("estou no controle de alguns nodes").
3. **Gate global + controle por-node (adotada):** um estado de topo **Full auto/Flex** que destrava o controle por-Node **SG/Manual**. Combina a clareza/segurança do global com a granularidade do por-node.

**Decisão adotada**
- **Gate global "Full auto / Flex"** (estado do cluster):
  - **Full auto:** o SG controla tudo; os toggles por-Node ficam **travados** (garante que não há Node manual esquecido — full-auto é full-auto).
  - **Flex:** destrava; cada Node passa a ser **SG** (automático) ou **Manual** (do operador). Nodes deixados como SG seguem automáticos.
- **Prioridade definida pelo modo do Node:**
  - **Node Manual → operador soberano.** O controlador **nunca** pausa, promove, rotaciona nem re-pausa torrents desse Node. As ações do operador (pause/resume/start) valem, ponto.
  - **Node SG → sistema tem prioridade** (gerencia holds/rotações como na ADR-021).
- **Manual conta no L (invariante preservado):** os downloads ativos de um Node Manual **contam** no teto do cluster; o controlador reconcilia usando **apenas os Nodes SG**. Consequência: pausar um download num Node Manual libera um slot que o sistema **preenche promovendo um hold num Node SG** — o cluster fica no talo (≤ L) sem desperdiçar slot.
- **Caso de borda (responsabilidade do operador):** se **todos** os Nodes estiverem em Manual, ou um Node Manual sozinho exceder o L, o sistema não tem como compensar → ele **apenas avisa** (alerta "acima do L do tracker"), **não age**. É o preço aceito do controle por-Node.
- **Fix da assimetria atual (qualificado pelo modo do Node):** num **Node Manual**, um *resume* de um torrent segurado (`admission_paused_at` setado) **limpa `admission_paused_at`** (o operador assumiu aquele torrent) → a correção de drift não o re-pausa; ele passa a contar no L e o controlador rebalanceia o resto.
  - ⚠️ **Este fix NÃO promove o operador a soberano num Node SG** — quem manda ali é a cláusula de prioridade acima. A redação original dizia "'resume manual vence' fica coerente com 'pause manual vence'" sem qualificar por modo, o que contradizia a própria Decisão e foi lido como regra global na implementação (ver "Correção de conformidade" abaixo).
- **Pausa manual num Node SG é ADOTADA pelo sistema** (`adoptOrphanPauses`): vira admission-hold e volta para a fila de promoção, sendo promovida quando abrir slot. Corolário direto de "Node SG → sistema tem prioridade". Para que um pause fique de pé, o Node tem de ser posto em **Manual**. Um torrent **completo** (`progress >= 1`) pausado fica de fora: é um seeder que o operador parou, não um download na fila.
- **Volta de Manual → SG readota:** o que ficou parado na mão enquanto o Node era Manual reentra na fila assim que ele volta a SG. Não há hook na troca de modo — a adoção roda sobre o predicado de Node SG a cada ciclo, então a transição se resolve sozinha no ciclo seguinte.
- **Reuso e escopo:** o estado do modo (gate global + modo por-Node) é **estado do cluster/Node com hot-reload** (padrão da **ADR-023**); o worker de admissão consulta o modo do Node **antes** de agir. O Scheduler (escolha de Node no *add*) não muda (já há `force_node_id`).
- **UI (controles no Dashboard, para panorama):** o interruptor global **Full auto/Flex** no topo do Dashboard + uma **coluna "Modo" (SG/Manual) na tabela de Nodes do Dashboard** (chip + toggle por linha) — assim o operador vê o modo de **todos** os Nodes de uma vez, sem rolar entre os cards grandes da página Nodes. A coluna "Agent" (`agent_version`) é **mantida** — decidiu-se **versionar os builds de verdade** com **tags de versão** (ex.: `git describe --tags --abbrev=0` → `v1.0.0`, sem hash feio) nos scripts de build, em vez do default "dev" — então a versão vira informação útil. O card/detalhe do Node pode espelhar o chip do modo, mas o **controle e o panorama vivem no Dashboard**.

**Consequências / Limitações**
- Em Flex com Node(s) Manual, o operador **pode** estourar o L (por design) — mitigado por alerta, não por bloqueio.
- O worker de admissão passa a **filtrar por modo do Node** em todas as queries (contar ativos, escolher holds, promover, rotacionar) — muda a ADR-021 sem quebrá-la (modo Full auto = comportamento atual).
- Estado a mais para persistir/expor (modo global + por-Node) e testar (prioridade, contagem no L, caso de borda).

**Benefícios**
- Regra de prioridade **clara e previsível** (definida pelo modo do Node), consertando a assimetria atual; escape hatch de controle/segurança ("assumo o volante") sem abrir mão do invariante ≤ L nos Nodes automáticos.

**Relação com outras ADRs**
- **Estende a ADR-021** (admissão): adiciona o eixo de controle do operador; Full auto = comportamento vigente.
- **Reusa a ADR-023** (config/estado dinâmico com hot-reload) para o estado do modo.
- Não cria canal Agent→CP novo (respeita **ADR-012/ADR-014**): o CP decide, o Agent executa.

# **ADR-025**

**Título:** Audit Log — trilha de auditoria das ações do operador (no boundary da API, separada de Eventos/Activity/Alertas)

**Estado:** Aprovada

**Contexto**
O SeedGrid já tem três trilhas: **Alertas** (problemas), **Eventos** (fatos de ciclo de vida de infra/domínio — alguns disparados pelo operador, com um `by` no payload, muitos pelo sistema) e **Activity Feed** (decisões de orquestração — ADR-022). Falta o que um produto multi-operador / open source espera: uma **trilha de auditoria** — "quem fez o quê, quando, de onde" — uniforme e centrada no **ator**, cobrindo toda ação do operador na API, inclusive login e tentativas malsucedidas. Hoje isso é parcial e disperso: só um subconjunto das mutações emite Evento (e nem todo Evento tem ator), e o login é rastreado apenas dentro do `auth`.

**Problema**
Registrar, de forma **uniforme e completa**, todas as ações mutadoras autenticadas do operador (e o login), com ator/rota/resultado/origem/tempo, sem instrumentar handler por handler e sem misturar com as trilhas existentes (Eventos = fatos de domínio; Activity = orquestração; Alertas = problemas).

**Alternativas consideradas**
1. **Adicionar coluna `actor` em `events` e tratar eventos com ator como auditoria:** reusa infra, mas mistura auditoria (lente de acesso/accountability) com fatos de domínio, e **não é completo** — nem toda mutação emite Evento, e o login/tentativas ficam de fora. Mesma objeção da ADR-022 (não poluir Eventos).
2. **Instrumentar cada handler mutador com uma chamada de auditoria:** completo, mas repetitivo e fácil de esquecer num handler novo (o audit deixaria de ser confiável).
3. **Middleware automático no boundary + domínio `audit_log` próprio (adotada):** um interceptor sobre as rotas protegidas grava toda requisição **mutadora** (POST/PUT/DELETE) do operador; domínio/tabela/endpoint/página próprios. Completo por construção (uma rota nova já é auditada) e conceitualmente separado.

**Decisão adotada** (alternativa 3)
- **Domínio próprio `audit_log`** (append-only): `actor` (username autenticado), `action` (**descrição amigável em PT-BR** — ex.: "Alterou a configuração «Node offline após» para 120s", "Entrou no Portal"), `method`, `path` (concreto, com ids), `status_code`, `source_ip`, `created_at`. Sem corpo da requisição (privacidade/tamanho — ex.: nunca a senha nova).
- **Ação amigável, não crua:** o handler fixa a descrição via `httpx.SetAuditAction` (reusa o rótulo/valor que ele já conhece — catálogo de settings, modo escolhido, etc.); na falta, o middleware deriva um rótulo PT-BR da rota (mapa `method`+`path` → verbo). O `method`/`path` seguem gravados como **detalhe técnico** (rodapé discreto no Portal), então a leitura é amigável sem perder a rastreabilidade. Complementa os Eventos, que guardam o diff semântico (`from`/`to`).
- **Middleware automático:** o `protected` passa a envolver os handlers com um recorder que, **após** o handler, grava a entrada quando o método é mutador (POST/PUT/DELETE/PATCH) e há operador no contexto. Captura o status via o `statusWriter` já existente (mesmo usado pelo `RequestLogger`). **Best-effort e desacoplado:** a gravação usa contexto destacado com timeout curto (registra mesmo se o cliente desconectou) e **nunca** altera nem bloqueia a resposta; falha ao auditar só é logada. GET não é auditado (ruído de leitura).
- **Login auditado explicitamente** no `handleLogin` (sucesso e falha, com o username tentado), pois a rota de login não passa pelo `protected` (ainda não há operador no contexto). Complementa o rastreio de IP/UA que o `auth` já faz.
- **Somente leitura para o operador:** `GET /api/v1/audit` (protegido) + página **Auditoria** no Portal (timeline, filtro por ator/ação). Sem canal Agent→CP novo (respeita ADR-012/ADR-014).

**Consequências / Limitações**
- Trilha append-only cresce com o tempo; volume moderado (só mutações + login, não leituras). Retenção/poda fica como evolução futura (análoga às outras trilhas).
- A descrição amigável carrega o essencial (rótulo + valor, ex.: "…para 120s"); o `method`/`path`/`status` seguem gravados para o detalhe técnico. As mudanças **sistêmicas** do operador (tunáveis, modo do cluster/Node) deixam de emitir Evento — passam a viver **só** na Auditoria (Eventos = fatos de domínio: Node/torrent registrado/removido/migrado, storage, etc.).
- Overlap parcial com Eventos operador-iniciados é aceito e intencional (lentes diferentes: acesso vs fato de domínio).

**Benefícios**
- Accountability real ("quem fez o quê") — valor que sobe com multi-operador e abertura open source; completo por construção (middleware), sem depender de disciplina por-handler; separado e sem poluir Alertas/Eventos/Activity.

**Relação com outras ADRs**
- Complementa, sem substituir, **Eventos** (fatos de domínio), **ADR-022** (Activity Feed, orquestração) e o **Alerting** (ADR-008).
- Respeita **ADR-012/ADR-014** (leitura por polling; sem canal novo) e **ADR-006** (estado no Postgres).

# **ADR-026**

**Título:** Screenshots automáticos na publicação em tracker UNIT3D (extração via ffmpeg no Agent + upload para host de imagem)

**Estado:** Aprovada

**Contexto**
Trackers UNIT3D (ex.: samaritano.cc) exigem screenshots do conteúdo na descrição do upload — regra descoberta ao vivo (`postpone` de um upload real: "Seven Pounds 2008 480p BDRip AAC2.0 H.264-NewArtRiot" recusado citando a wiki de padronização, screenshots como exemplo). Hoje a ADR-020 monta a descrição só com a sinopse do TMDB; nenhuma imagem é gerada ou lembrada. O operador precisa extrair frames e colar BBCode `[img]` manualmente antes de cada publicação — 100% manual, esquecível, e bloqueia releases já aprovados no resto (nomenclatura, mediainfo).

**Problema**
(a) Extrair frames requer acesso ao **arquivo de mídia**, que só existe no Seed Node (ADR-012: só o Agent fala com os dados); (b) o Agent precisa de um binário novo (**ffmpeg**/ffprobe) além do `mediainfo` já exigido pela ADR-020, inclusive nos Nodes ARM64 (RPi 3B+); (c) as imagens precisam de um **host** externo (BBCode referencia uma URL, não pode embutir binário) — outro segredo de terceiro a guardar; (d) o resultado ainda passa pela **conferência humana** obrigatória no Portal (mesmo gate da ADR-020) antes do envio ao tracker.

**Alternativas consideradas**
1. **Status quo (manual):** zero automação — mantém o atrito e o risco de esquecimento (causa raiz do `postpone` real). Rejeitada como objetivo.
2. **CP extrai os frames:** violaria a ADR-012 (o CP não tem acesso aos dados do Node; teria que puxar o arquivo inteiro só para recortar frames — caro e fora do padrão). Rejeitada.
3. **Agent extrai (ffmpeg) + CP faz upload/injeta BBCode (adotada):** o Agent, que já tem o arquivo, extrai N frames JPEG e devolve os bytes ao CP pela fila de comandos existente (ADR-014); o CP (que já guarda os segredos de terceiro — TMDB, ADR-020) sobe cada frame no host de imagem e monta o BBCode. Mantém a separação de responsabilidades (Agent = dados locais, CP = orquestração/segredos) e reusa a infra existente.

**Decisão adotada** (alternativa 3)
- **Novo comando `extract_screenshots`** na fila do ADR-014 (`payload: {hash, screenshot_count}`): o Agent resolve o `ContentPath` do torrent no Engine (mesmo método `ContentPathOf` já usado na migração, ADR-018), acha o maior arquivo regular sob esse caminho (heurística já usada pelo `mediainfo`, ADR-020) e roda `ffprobe` (duração) + `ffmpeg` (um frame por timestamp) em **N pontos uniformemente distribuídos entre 10% e 90%** da duração (evita logo de abertura e créditos finais). Devolve os JPEGs em base64 no `result` do comando (sem coluna nova — `commands.result` já é `jsonb` sem limite prático). Best-effort por frame: falha isolada não aborta os demais; falha total (ex.: ffmpeg ausente) marca o comando `FAILED` com o motivo.
- **CP aguarda o resultado de forma bloqueante** (reusa/generaliza o `waitCommandDone` já usado no `prepare_upload` da ADR-019 — extraído para `command.Service.WaitDone`, com um `ResultOf` novo para ler o `result` de um comando concluído), decodifica os frames e chama o cliente **ptpimg** (`internal/ptpimg`, mesmo estilo do cliente `unit3d`): multipart upload autenticado por `api_key`, devolve a URL direta de cada imagem. Monta o BBCode (`[img]url[/img]`, um por linha).
- **Segredo do host de imagem:** `PTPIMG_API_KEY` segue o **mesmo padrão do TMDB** (ADR-020) — env var lida uma vez no boot do CP (`internal/config`), **não** entra no catálogo de tunáveis (não é comportamento, é segredo).
- **Quantidade de screenshots é tunável** (`publish.screenshot_count`, default 4, categoria nova `CatPublish`) no catálogo existente (ADR-023) — editável no painel do Portal sem redeploy, sem código novo no Portal para o tunável em si (o painel já é genérico).
- **Conferência humana preservada:** o Portal expõe um botão "Gerar screenshots" no fluxo de publicação (mesma tela da ADR-020) que chama o novo endpoint e **insere** o BBCode retornado no campo de descrição — o operador revisa/edita/remove antes de publicar, exatamente como já faz com nome/categoria/IDs inferidos. Nada é gerado ou enviado ao tracker sem essa ação explícita.
- **Escopo:** aplica-se tanto a publicações novas (ADR-019) quanto a torrents **já seedando** (o caso real que motivou esta ADR) — diferente do mediainfo, que só roda no momento do upload de conteúdo, o comando de screenshots pode ser disparado a qualquer momento, contra o torrent já no Engine.

**Consequências**
Benefícios:
- Elimina a causa raiz do `postpone` por screenshots ausentes/incompletos, sem abrir mão da conferência humana; reusa a fila de comandos (ADR-014) e o padrão de segredo de terceiro (ADR-020) — nenhum canal novo Agent↔CP.
Limitações/custos:
- Novo binário obrigatório no Agent (**ffmpeg**/ffprobe) em todos os Nodes, incluindo ARM64 (RPi 3B+) — mesma classe de custo operacional já aceita para o `mediainfo` (ADR-020).
- **ptpimg não tem API pública documentada oficialmente**; o formato usado (multipart, `file-upload[]`, resposta `[{code, ext}]`) é o convencionado por ferramentas de auto-upload de trackers privados e precisa de **validação ao vivo** no primeiro rollout (mesma cautela já registrada na ADR-021 para o `SlotLimit` do UNIT3D). Falha do host de imagem não deve travar a publicação — o operador sempre pode colar BBCode manual como fallback.
- Frames são gerados **on-demand**, não persistidos (sem tabela/coluna nova) — gerar de novo custa uma nova rodada de comando+upload.

> **Nota (2026-07-18):** o risco registrado acima se concretizou — **ptpimg.me saiu do ar** e nunca teve API documentada. A **ADR-030 supersede a escolha de host** (ptpimg → **imgbb**, com API pública), e a chave saiu de env para o catálogo (ADR-029). Todo o resto desta ADR (Agent extrai via ffmpeg, CP hospeda e monta BBCode, conferência humana) segue valendo.

**Relação com outras ADRs**
- Estende a **ADR-020** (publicação em tracker UNIT3D): mais um campo inferido (BBCode de screenshots) sujeito à mesma conferência humana antes do `POST /api/torrents/upload`.
- Reusa **ADR-014** (fila de comandos por polling) e generaliza a espera bloqueante introduzida pela **ADR-019** (`prepare_upload`/`waitCommandDone`).
- Reusa **ADR-023** (catálogo de tunáveis, hot-reload) para `publish.screenshot_count`. A chave do host de imagem, que aqui ficava em env, foi para o catálogo pela **ADR-029**.
- Reafirma **ADR-012** (Agent só age sobre dados locais; CP concentra segredos/orquestração).
- **Superada em parte pela ADR-030** quanto ao host de imagem (ptpimg → imgbb).

# **ADR-027**

**Título:** Atualização do Agent orquestrada pelo Control Plane (self-update por comando, rollout manual)

**Estado:** Aprovada

**Contexto**
Atualizar o Agent é hoje 100% manual: buildar no node-01, `pct pull` do binário, `pct push` para cada Node (e `scp` no meio quando o Node está no outro host Proxmox), `systemctl restart` um a um. O procedimento está documentado na skill `seedgrid-ops` justamente por não ser óbvio — inclui a armadilha de parar o Agent antes de sobrescrever (binário em uso dá `Text file busy` e o push falha em silêncio, deixando o binário ANTIGO no lugar). O custo é sentido a cada release (3× numa única sessão: v1.0.0 → v1.1.0 → v1.1.1) e cresce linearmente com a frota. Na entrega do refino da ADR-021 (jul/2026) o efeito ficou explícito: node-01 e node-02 receberam o binário novo e o **node-03 ficou para trás** (host inacessível no momento), sem que nada no sistema registrasse ou sinalizasse a divergência — o operador precisa lembrar.

**Problema**
(a) O CP **decide** e o Agent **executa** (ADR-012), e o CP nunca conecta no Node — logo o update tem de ser **Agent-iniciado**, como todo o resto (ADR-014, polling); (b) o binário publicado hoje em `/node/seedgrid-agent` (ADR-017) é **ARM64**, herança do RPi 3B+, enquanto a frota atual é **amd64**: um update que baixasse esse caminho quebraria todos os Nodes de uma vez — servir por **arquitetura** é pré-requisito, não refinamento; (c) trocar o binário de um processo em execução e reiniciá-lo é delicado (binário em uso, replace não-atômico, permissão para `systemctl`); (d) um binário ruim (arquitetura errada, download corrompido) derruba o Node, e o Agent — sendo o próprio processo — não pode se consertar depois de morto; (e) o operador precisa **ver** que há update disponível, por Node, sem consultar tag git na mão.

**Alternativas consideradas**
1. **Status quo (manual):** mantém o toil e a divergência silenciosa entre Nodes. Rejeitada como objetivo.
2. **CP faz push do binário (SSH/agente remoto):** violaria a ADR-012 (o CP passaria a conectar no Node) e exigiria credencial de acesso a cada Node guardada no CP — superfície nova e desnecessária. Rejeitada.
3. **Gerenciador de pacotes / imagem de container do Agent:** resolveria distribuição, mas o Agent roda como binário estático sob systemd (não em container) justamente para acessar o Engine e o disco do Node; migrar o modelo de execução é escopo maior que o problema. Rejeitada por ora.
4. **Comando `update_agent` na fila, rollout manual (adotada):** o Agent puxa o comando (ADR-014), baixa o binário da URL que o CP indicou, prova que ele executa, troca e sai — o systemd reergue. Reusa toda a infra existente (fila durável, artefatos servidos pelo Portal na ADR-017, catálogo de tunáveis da ADR-023) sem canal novo.

**Decisão adotada** (alternativa 4)
- **Novo comando `update_agent`** (`payload: {version, url, sha256}`): o CP enfileira; o Agent, no polling, baixa a URL, confere o **SHA-256**, e só então age. O CP **não** envia o binário pela fila (payload jsonb não é lugar para 6 MB): manda a URL, e o Agent busca — Agent-iniciado, coerente com a ADR-012.
- **Artefatos por arquitetura:** o build publica `/node/seedgrid-agent-<arch>` (amd64, arm64) + um manifesto `/node/agents.json` (`{version, binaries: {arch: {file, sha256}}}`), servidos pelo Portal como já ocorre na ADR-017. O caminho ambíguo `/node/seedgrid-agent` deixa de existir e o `bootstrap-node.sh` passa a resolver a arquitetura (`uname -m`) — isso **conserta de passagem** o onboarding de Node amd64, que hoje baixaria um binário ARM64. O CP lê o manifesto pela URL base configurada e compara com o `nodes.architecture`/`nodes.agent_version` que **já** persiste do inventário — nenhuma coluna nova.
- **Rollout manual, um Node por vez:** o Portal mostra "update disponível" por Node (comparando versão reportada × versão do manifesto) e um botão **Atualizar**; nada se move sozinho. Rejeitado o rollout automático nesta ADR: sem rollback por heartbeat, um binário ruim se propagaria pela frota inteira sem supervisão humana — e o ganho (não clicar) não paga o risco numa frota de 3 Nodes com 1 operador. O gancho para automatizar depois é o próprio endpoint.
- **Segurança da troca (o ponto crítico):** o Agent (1) baixa para o **mesmo diretório** do binário atual, resolvido por `os.Executable()` — `rename` só é atômico dentro do mesmo filesystem; (2) confere o SHA-256 contra o que o CP mandou; (3) roda o binário baixado com **`--version`** (flag nova) e exige saída 0 — é o **smoke-test** que prova que aquele binário EXECUTA naquele host, pegando arquitetura errada, corrupção e incompatibilidade de libc **antes** de qualquer troca; (4) guarda o atual como `.prev`; (5) faz `rename` atômico; (6) **confirma o comando ao CP** e só então **sai com código 0** — o systemd (`Restart=always`, já no unit) reergue o processo com o binário novo. Não é preciso invocar `systemctl` nem conceder permissão nova: sair é suficiente.
- **Proteção contra binário ruim:** o smoke-test pré-troca é a defesa principal (nada é trocado por um binário que não roda). Como suspensório, o unit ganha um `ExecStartPre` que testa `--version` do binário instalado e, falhando, restaura o `.prev` — cobre corrupção posterior à troca. **Rollback por heartbeat** (restaurar quando o Agent novo sobe mas para de reportar) fica **fora** do escopo: exigiria um supervisor externo ao Agent (timer systemd) e cobre um caso que o smoke não pega — bug de runtime — cujo custo hoje é entrar no Node e restaurar o `.prev` na mão.
- **Confirmar antes de morrer:** o ack vai ao CP **antes** do `exit(0)`, senão o comando ficaria eternamente `DISPATCHED` e o Portal mostraria falha num update que deu certo.
- **URL dos artefatos é tunável** (`agent.artifacts_url`, ADR-023): precisa ser alcançável **pelos Nodes** (não é um endereço interno de rede Docker). Vazia **desliga** a funcionalidade — sem badge, endpoint recusa —, escolha deliberada para o CP não inventar a própria URL pública e mandar o Node baixar de um endereço errado.

**Consequências**
Benefícios:
- Elimina o toil manual e, principalmente, a **divergência silenciosa**: a frota fora de versão passa a ser visível no Portal em vez de depender da memória do operador.
- Conserta o bug latente do onboarding (ADR-017) que serviria binário ARM64 para Node amd64.
- Reusa fila (ADR-014), artefatos (ADR-017) e catálogo (ADR-023); nenhum componente ou canal novo; nada viola a ADR-012.
Limitações/custos:
- **Chicken-and-egg**: um Agent anterior a esta ADR não conhece `update_agent` e responde `FAILED tipo de comando desconhecido` (observado ao vivo no node-02 na entrega) — inofensivo (o CP registra a falha; nada é trocado), mas cada Node existente precisa de **um** deploy manual até a primeira versão com self-update; do próximo em diante, é botão. Vale também para Node cujo bootstrap tenha rodado antes desta ADR.
- **Sem rollback automático por heartbeat**: binário que executa mas falha em runtime deixa o Node fora até intervenção manual (`.prev` está lá).
- O `ExecStartPre` vive no **unit systemd**, que o self-update não atualiza (ele troca o binário, não o unit): Nodes existentes só ganham o suspensório quando o unit for reinstalado — o smoke-test pré-troca, esse sim, vale desde o primeiro update.
- Um Node **offline** não atualiza (o comando espera na fila, durável, e é executado quando ele voltar — comportamento desejado, mas o operador vê "update disponível" até lá).
- O CP passa a depender do Portal para ler o manifesto (HTTP); Portal fora do ar = sem badge (o cluster segue operando normalmente).

**Relação com outras ADRs**
- Reusa **ADR-014** (fila de comandos durável por polling) e reafirma **ADR-012** (Agent puxa; o CP nunca conecta no Node).
- Estende **ADR-017** (onboarding zero-touch): os mesmos artefatos servidos pelo Portal passam a ser versionados por arquitetura e descritos por manifesto; corrige o binário ARM64 único.
- Reusa **ADR-023** (catálogo de tunáveis com hot-reload) para `agent.artifacts_url`.
- Complementa **ADR-024** (versionamento real dos builds via tag git): sem `agent_version` confiável não há como comparar versões — é o pré-requisito que a tornou possível.

# **ADR-028**

**Título:** Telemetria para fora do Portal — notificações por Telegram, Discord e webhook genérico

**Estado:** Aprovada

**Contexto**
Todo o conhecimento do SeedGrid sobre si mesmo morre dentro do Portal: alertas (ADR-008), eventos (ADR-006) e o feed de orquestração (ADR-022) só existem para quem está com a aba aberta. Não há **nenhum** canal de saída — um Node offline às 3h da manhã, um storage falhando ou um disco enchendo só são descobertos quando o operador resolve olhar. O Portal é um painel de consulta, não de vigilância: exige presença. Para um cluster que roda 24/7 sem ninguém na frente, a informação certa precisa ir atrás da pessoa.

**Problema**
(a) Quais fatos merecem notificação varia por operador e por momento — mandar tudo vira flood (o controlador de admissão sozinho gera vários holds por hora) e mandar de menos derrota o propósito; (b) cada canal tem credencial e formato próprios (Telegram: bot token + chat_id, Markdown; Discord: webhook URL, embed; genérico: JSON cru), e nenhum pode ficar hardcoded; (c) **onde guardar essas credenciais** conflita com o precedente: a ADR-026 estabeleceu que segredo **não** entra no catálogo de tunáveis (TMDB e ptpimg são env, lidos no boot); (d) um canal externo fora do ar (rede, rate-limit do Telegram, webhook morto) **não pode** quebrar o fluxo que gerou o alerta — notificar é efeito colateral, não parte da transação; (e) alerta que abre e resolve em loop (Node flapando) não pode transformar o celular do operador em metralhadora.

**Alternativas consideradas**
1. **Status quo (só Portal):** exige presença; o alerta existe mas não alcança ninguém. Rejeitada como objetivo.
2. **Alertmanager como canal de saída:** o Alertmanager já está no stack (ADR-016) e sabe notificar. Mas hoje ele fala com o CP na direção oposta (webhook `POST /api/v1/integrations/alertmanager`), e nem todo fato notificável é uma métrica Prometheus — eventos e orquestração nascem no domínio, não no scrape. Roteá-los ao Alertmanager só para voltar significa modelar tudo como métrica e manter uma segunda configuração de rotas fora do Portal. Rejeitada por inverter a fonte da verdade.
3. **Um binário/serviço notificador à parte:** componente novo para operar, com sua própria config e deploy, para uma função que é intrínseca ao CP (ele já sabe o que aconteceu e já tem o catálogo de config com hot-reload). Rejeitada por custo operacional desproporcional.
4. **Pacote `internal/notify` no CP, disparado nos pontos que já produzem alerta/evento/atividade (adotada):** reusa o que existe (catálogo ADR-023, trilhas ADR-006/008/022), não acrescenta processo nem canal de entrada, e mantém a decisão do que sai numa única tela.

**Decisão adotada** (alternativa 4)
- **Três canais, um `Sender` cada** (`internal/notify`): **Telegram** (Bot API `sendMessage`), **Discord** (POST na webhook URL) e **webhook genérico** (POST de JSON cru). O genérico é deliberado: destrava Slack, n8n, Home Assistant e o que vier **sem código novo por integração** — o Discord já é um webhook, então o custo marginal do canal genérico é quase zero. **Canal com credencial vazia = desligado**; não há flag "enabled" separada (um liga-desliga que pode divergir do preenchimento é uma inconsistência a mais para manter).
- **Credenciais no catálogo de tunáveis (ADR-023), com hot-reload — exceção EXPLÍCITA ao precedente da ADR-026.** O critério que a ADR-026 realmente estabeleceu não é "todo segredo vai para env", e sim que **credencial do sistema** (TMDB, ptpimg: infraestrutura do produto, igual para qualquer operador, sem a qual a feature não existe) não é comportamento e não pertence ao painel. Um bot token do Telegram é de outra natureza: é **configuração do usuário** — pertence à pessoa, muda quando ela quiser, e cada operador tem o seu. Exigir editar o `.env` no host do Control Plane e reiniciar o controller para trocar o próprio bot é atrito sem contrapartida de segurança (o `.env` não é mais protegido que o banco — ambos vivem na mesma máquina, e o banco já guarda o `api_token` do tracker). Consequência obrigatória: `Spec.Secret` marca o campo, e o valor **nunca** volta na API de leitura — `GET /settings` devolve apenas `has_value` (booleano), exatamente como `trackers.api_token` (ADR-020) já faz. `PUT` com valor **vazio = manter** (senão salvar qualquer outro campo apagaria o token); para **limpar** de fato, `DELETE /settings/{key}` (reset ao default, que é vazio) — que é também como se desliga um canal.
- **O operador escolhe o que chega, por trilha:** `notify.alerts_min_severity` (`OFF|CRITICAL|WARNING|INFO`, default `WARNING`), `notify.alerts_on_resolve` (default `true` — saber que **voltou** importa tanto quanto saber que caiu), `notify.events` e `notify.activities` (lista de tipos/kinds, `*` para todos, **vazio por default**). Os dois últimos nascem desligados de propósito: são as trilhas de alto volume, e o default tem de ser útil sem ser insuportável. **Auditoria (ADR-025) fica de fora**: são as ações do próprio operador — notificá-lo do que ele acabou de fazer é eco, não telemetria.
- **Anti-flood:** `notify.rate_limit_per_min` (default 20) descarta o excedente da janela e registra quantas mensagens foram suprimidas (silêncio sem explicação é pior que a mensagem que falta). Cobre o caso real do Node que flapa entre ONLINE/OFFLINE.
- **Best-effort, sempre:** o envio é assíncrono e nenhuma falha de canal externo se propaga para quem gerou o fato — um Telegram fora do ar não pode impedir o Alert Worker de registrar um Node offline no banco. Falha vira log, não erro de domínio.
- **Botão "enviar teste"** (`POST /api/v1/notify/test`): a única forma honesta de o operador saber que o token está certo é ver a mensagem chegar — sem isso, configurar é adivinhação até o primeiro incidente real.

**Consequências**
Benefícios:
- O SeedGrid deixa de exigir presença: o incidente vai atrás do operador, e a informação que já existia passa a ter destino.
- O webhook genérico transforma "integrar com X" em configuração, não em release do CP.
- Nenhum componente, processo ou canal de entrada novo; reusa catálogo e trilhas existentes.
Limitações/custos:
- **Abre exceção ao precedente da ADR-026**: o catálogo passa a poder guardar segredo. Mitigado pela distinção (credencial do sistema × configuração do usuário), pelo mascaramento na leitura e pelo precedente do `trackers.api_token`. Aceito conscientemente — se a distinção se mostrar frouxa, a ADR seguinte a substitui.
- Tokens passam a viver **no banco** (e nos backups dele) além do `.env`; o raio de exposição de um dump cresce.
- Entrega **sem prova de vida do canal real** enquanto não houver credencial: os testes usam servidor fake por canal (garantem o payload por canal, o mascaramento, o rate-limit e os filtros), mas "a mensagem chega no seu Telegram" só o botão de teste com token real demonstra.
- Notificação é **at-most-once**: sem fila durável, mensagem perdida numa janela de rede está perdida (o fato permanece no Portal). Fila/retry persistente fica para quando/se doer.

**Adendo (set/2026) — filtros por canal, e alerta fora do anti-flood.** A decisão original tinha os filtros e o teto por **cluster**, com despacho em fan-out para todo canal com credencial. Duas consequências não previstas apareceram no uso:

1. **Sem roteamento, um canal de automação vira lixeira.** Ao planejar disparar um fluxo externo (n8n) a partir de uma notificação, o endpoint receberia tudo o que o cluster notifica e descartaria o resto. Cada canal passa a poder ter os **seus** filtros, sob um toggle `notify.<canal>.custom` — desligado por padrão, de modo que quem já usava não vê diferença. O toggle existe em vez de "vazio = herda" porque vazio **já significa** "nenhum" nas trilhas.
2. **O anti-flood podia descartar um alerta.** O teto era uma janela única compartilhada por todas as trilhas e aplicada antes do fan-out: uma rajada de atividade podia consumir a cota e fazer um `CRITICAL` ser descartado — em silêncio, já que a supressão só virava `logger.Warn`. **Alerta passa a ser isento do teto em qualquer canal**, o teto vira por canal e a supressão vira métrica (`seedgrid_notify_suppressed_total`). O princípio: alerta existe para incomodar, e um limitador que descarta come justamente o que importava, de forma arbitrária. Contra tempestade, o remédio é o dedup (que já existe) e o conserto da regra ruidosa — se um dia precisar de proteção na entrega, que seja **coalescência**, nunca descarte.

Não altera o mecanismo da ADR (canais, trilhas, segredo no catálogo), só a granularidade da escolha e a prioridade entre trilhas.

**Relação com outras ADRs**
- Consome **ADR-008** (alertas), **ADR-006** (eventos) e **ADR-022** (feed de orquestração) — é saída dessas trilhas, sem alterá-las.
- Reusa **ADR-023** (catálogo com hot-reload) e o estende: `KindBool`, `Options` (enum validado) e `Spec.Secret`.
- **Abre exceção explícita à ADR-026** quanto a segredo no catálogo, com o critério registrado acima; o padrão da ADR-026 segue valendo para credencial do sistema (TMDB, ptpimg).
- Segue o mascaramento de segredo na API estabelecido pela **ADR-020** (`trackers.api_token`).
- Não interfere na **ADR-016** (Prometheus/Alertmanager), que continua cobrindo a telemetria de métricas.

> **Nota (2026-07-17):** a distinção "credencial do sistema fica em env / credencial do usuário vai ao catálogo" que esta ADR usou foi **superada pela ADR-029**: para viabilizar a abertura open-source, TODA credencial de terceiro atrelada à conta do operador — inclusive TMDB e ptpimg, aqui citadas como "do sistema" — passa a viver no catálogo. O mecanismo (Secret + mascaramento + seed-from-env) é o mesmo introduzido aqui.

# **ADR-029**

**Título:** Toda credencial de terceiro no Portal — pré-requisito da abertura open-source

**Estado:** Aprovada

**Contexto**
O SeedGrid caminha para ser publicado como open-source. Hoje as credenciais de serviços de terceiros que o Control Plane consome — **TMDB** (enriquecimento de metadados, ADR-020), **IGDB/Twitch** (jogos, #17) e **ptpimg** (host de screenshots, ADR-026) — são lidas de variáveis de ambiente no boot (`SEEDGRID_TMDB_API_KEY`, etc.), por decisão da ADR-026 ("segredo do sistema não entra no catálogo"). A ADR-028 acabou de abrir uma exceção para os canais de notificação, mas manteve TMDB/ptpimg em env, tratando-os como "credencial do sistema".

O problema com essa fronteira só fica visível no contexto de open-source: **essas chaves são da conta pessoal do mantenedor**. Um TMDB API key é vinculado a um cadastro no TMDB; o par IGDB é um app do Twitch de alguém; o ptpimg é a conta de um usuário num tracker privado. Publicar o repositório com essas chaves em env de exemplo, ou pior, embutidas, significa **vazar credenciais pessoais** ou entregar um produto que "só funciona com a conta do autor". Nenhum operador que clonar o SeedGrid tem — nem deveria herdar — as chaves do mantenedor.

**Problema**
(a) A distinção "sistema × usuário" da ADR-028 não sobrevive ao critério de open-source: do ponto de vista de quem clona, **toda** chave de terceiro é dele, não do "sistema"; (b) chave em env exige editar `.env` no host e reiniciar o controller — atrito para o operador e, para o projeto aberto, um convite a commitar `.env` por engano; (c) a migração não pode quebrar o deploy atual, que já tem as chaves em env há meses.

**Alternativas consideradas**
1. **Manter em env, documentar "preencha suas chaves" no README (status quo):** não vaza se o `.env` for gitignored, mas mantém o atrito e a fronteira arbitrária, e um `.env.example` com placeholders é fácil de preencher errado/commitar. Rejeitada: não resolve a causa (config de terceiro fora do lugar onde o operador configura tudo o mais).
2. **Chaves só no catálogo, sem seed:** limpo para quem clona, mas **quebraria o deploy atual** (as chaves em env deixariam de ser lidas). Rejeitada.
3. **Catálogo (ADR-023) com Secret + seed-from-env no 1º boot (adotada):** reusa exatamente o mecanismo que a ADR-028 introduziu (`Spec.Secret`, mascaramento na leitura, seed a partir do `EnvVar` na primeira subida). O deploy atual migra sozinho: no boot seguinte, cada chave em env semeia sua linha no banco e passa a ser editável no Portal; dali em diante o `.env` é ignorado. Quem clona começa com os campos vazios e os preenche na tela.

**Decisão adotada** (alternativa 3)
- **Nova categoria `integrations.*` no catálogo:** `integrations.tmdb_api_key` (Secret), `integrations.igdb_client_id`, `integrations.igdb_client_secret` (Secret), `integrations.ptpimg_api_key` (Secret). Cada uma com `EnvVar` apontando para a variável atual — o seed-from-env (ADR-023) migra o deploy existente no 1º boot, sem intervenção.
- **Os clientes passam a ler a chave do Store por chamada** (não mais um valor fixo do boot): `tmdb`/`igdb` recebem um `func() string` ligado ao catálogo; `Enabled()` e cada request releem — assim trocar a chave no Portal vale **sem redeploy** (hot-reload), e esvaziá-la **desliga** a integração na hora. O `ptpimg` é resolvido do Store no handler de screenshots.
- **Mascaramento idêntico ao da ADR-028:** o valor nunca volta na API de leitura (só `has_value`); PUT vazio mantém; DELETE limpa. A Auditoria registra o fato, não o valor.
- **A fronteira do que fica em env encolhe para a infra do PRÓPRIO deploy**, que não é credencial de terceiro nem "do autor": `POSTGRES_*` (o banco é local ao deploy), `SEEDGRID_ADMIN_*` (bootstrap do 1º admin) e `SEEDGRID_AGENT_REGISTRATION_TOKEN` (segredo compartilhado do enroll). Esses são propriedade de quem opera a instância, gerados por ele, e não têm dono externo — continuam em env.

**Consequências**
Benefícios:
- O repositório pode ser aberto sem nenhuma chave pessoal do mantenedor: `.env.example` fica só com a infra do deploy, e todas as integrações de terceiro se configuram na tela.
- Um operador novo tem **um** lugar para tudo que é dele (chaves, canais, tunáveis) — coerência que a fronteira anterior quebrava.
- Migração automática e reversível; o deploy atual não sente.
Limitações/custos:
- Reverte parte da ADR-026 e a distinção da ADR-028 — assumido conscientemente: o critério de open-source é mais forte que a estética "sistema × usuário".
- Mais segredos passam a viver no banco (e nos backups dele). Mesma classe de exposição já aceita na ADR-028; mitigada pelo mascaramento.
- Os clientes de integração ganham uma indireção (provider em vez de valor fixo) — custo pequeno de código para ganhar hot-reload.

**Relação com outras ADRs**
- **Supersede** a decisão de guardar credencial de terceiro em env da **ADR-026** e a distinção "sistema × usuário" da **ADR-028**; o mecanismo (Secret/mascaramento/seed) é o da ADR-028.
- Reusa **ADR-023** (catálogo, hot-reload, seed-from-env) e o mascaramento da **ADR-020** (`trackers.api_token`).
- Habilita a trilha de **abertura open-source** (checklist de pré-abertura no backlog): genericizar as chaves era um dos itens.

# **ADR-030**

**Título:** Host dos screenshots: trocar ptpimg por imgbb

**Estado:** Aprovada

**Contexto**
A ADR-026 escolheu o **ptpimg.me** para hospedar os screenshots que os trackers UNIT3D exigem na descrição do upload. Aquela ADR já registrou o risco explicitamente: *"ptpimg não tem API pública documentada oficialmente; o formato usado é o convencionado por ferramentas de auto-upload e precisa de validação ao vivo"*. O risco se concretizou da pior forma — **o ptpimg.me saiu do ar há meses**, e a integração nunca chegou a ser exercitada de verdade (a chave sequer estava configurada). Escolher um serviço sem contrato público foi um erro de julgamento: sem API documentada não há como depender dele nem saber quando quebra.

**Problema**
Precisamos de um host de imagens que: (a) tenha **API pública e documentada** (contrato estável, não engenharia reversa); (b) aceite o que o Agent já produz — frames **JPEG em base64** (ADR-026), para não mexer no fluxo Agent→CP; (c) devolva uma **URL direta** para montar o BBCode `[img]`; (d) autentique por **chave da conta do operador**, encaixando no padrão da ADR-029 (chave no painel, mascarada, não em env/código); (e) seja aceito na prática pelos trackers UNIT3D.

**Alternativas consideradas**
1. **Insistir no ptpimg:** fora do ar, sem API documentada. Rejeitada — é a causa desta ADR.
2. **imgbb (adotada):** API v1 pública e documentada (`POST https://api.imgbb.com/1/upload?key=`), aceita `image` em base64, devolve `data.url` (link direto), free, autenticação por API key. Confirmado como o host que a comunidade do samaritano usa de fato. Atende todos os critérios.
3. **Auto-hospedar (servir as imagens do próprio CP/Portal):** evitaria depender de terceiro, mas a URL precisa ser pública e estável para o tracker externo buscar — reintroduz a exposição do CP à internet (contra o desenho atual, CP interno) e vira responsabilidade de storage/retenção de imagem. Rejeitada por ora; reavaliar se surgir necessidade de não depender de host externo.

**Decisão adotada** (alternativa 2)
- **Novo cliente `internal/imgbb`** com a mesma interface do que foi removido: `Upload(ctx, apiKey, frames [][]byte) ([]string, error)`. Para cada frame, faz `POST multipart` com o campo `image` = **base64 do JPEG** para `https://api.imgbb.com/1/upload?key=<API_KEY>`, lê `data.url` da resposta e devolve as URLs **na ordem de entrada** (a ordem importa: os screenshots aparecem em sequência na descrição). imgbb envia **uma imagem por requisição** — o cliente itera; falha de um frame aborta com erro claro (melhor um upload consistente que uma descrição pela metade).
- **Sem `expiration`** na chamada = imagem **permanente** (screenshots de upload não podem sumir depois).
- **Chave no catálogo (ADR-029):** `integrations.imgbb_api_key` (Secret, mascarada, hot-reload), **substitui** `integrations.ptpimg_api_key`. Vazia desabilita a geração de screenshots, como antes.
- **O pacote `ptpimg` e sua chave são removidos** — código morto que apontava para um serviço extinto só induz a erro.
- **Fluxo Agent→CP inalterado:** o Agent segue extraindo N frames JPEG via ffmpeg e devolvendo base64 (ADR-026); só muda **para onde** o CP os envia e como lê a URL de volta.

**Consequências**
Benefícios:
- Integração passa a depender de um **contrato público** e de um serviço vivo; corrige a causa raiz do erro da ADR-026.
- Zero mudança no Agent; a troca é contida ao cliente e ao handler de screenshots.
- Chave no padrão da ADR-029 (painel, mascarada) — coerente com TMDB/IGDB.
Limitações/custos:
- Ainda **depende de um terceiro** (imgbb) — se ele cair, os screenshots param. Mitigado por ser um serviço com API estável e amplamente usado; a alternativa de auto-hospedar fica registrada caso um dia doa.
- **Validação ao vivo pendente até o operador criar a chave imgbb** (grátis) e rodar uma publicação real — desta vez com um serviço que tem API documentada, ao contrário da ADR-026.
- imgbb faz **um upload por requisição** (sem batch): N frames = N chamadas. Custo irrelevante para os poucos screenshots por publicação.

**Relação com outras ADRs**
- **Supersede a escolha de host da ADR-026** (ptpimg → imgbb); todo o resto da ADR-026 (Agent extrai via ffmpeg, CP hospeda e monta o BBCode, conferência humana) segue valendo.
- Segue a **ADR-029** para a credencial (chave de terceiro do operador no catálogo, mascarada).

> **Nota (2026-10-04):** a limitação "se ele cair, os screenshots param" ganhou resposta na **ADR-040**: o host deixa de ser único. Um tracker pode declarar no perfil o **host de imagem do próprio site**, e o imgbb passa a ser o **último elo de uma cadeia com fallback**, com o contrato e a chave global desta ADR inalterados. Ao implementar, a ADR-040 também fecha o vazamento da chave do imgbb pelo texto do erro de rede (a chave viaja na query, e o `*url.Error` carrega a URL inteira).

# **ADR-031**

**Título:** Migração de torrent incompleto (parcial) e trava de concorrência

**Estado:** Aprovada

**Contexto**
A ADR-018 desenhou a migração para torrent **completo**: o destino puxa os dados, faz recheck e o CP só finaliza quando o destino reporta `SEEDING`. Na prática o operador migra torrent **incompleto** — é justamente o que ele mais precisa quando um Node enche (node-03 a 97% de disco, incidente de 2026-07-18): tirar de lá um torrent que **não consegue completar por falta de espaço** e terminá-lo num Node com espaço. Foi feito na mão com `Os.Vingadores.2012` (preso no ciclo download→sem espaço→fila no node-03) e, migrado para o node-01, **completou normalmente**.

Hoje esse caminho falha, e falha **silenciosamente mal**.

**Problema**
Três defeitos encadeados, confirmados no código:

1. **Tar vazio devolvido como sucesso (causa raiz).** `handleTransfer` (`agent/internal/transfer/transfer.go:198`) escreve o cabeçalho HTTP 200 e só então chama `tarPath`. Se o conteúdo ainda não existe no disco (download não iniciado), o `os.Stat` de `tarPath` falha **depois** do 200 — mas o `defer tw.Close()` ainda emite o footer do tar. O destino recebe **um tar válido e vazio com status 200**; `extractTar` lê `io.EOF` na primeira entrada e devolve `nil`. **O pull "dá certo" sem ter copiado um único byte.**
2. **Re-download do swarm (o bug B3b do backlog — mesma causa raiz).** Com o pull "bem-sucedido", `migratePull` segue para `AddFile` + `Recheck`. O recheck acha 0% e o qBittorrent começa a **baixar tudo do swarm** — exatamente o que a ADR-018 existe para evitar (queima ratio em tracker privado). Só depois de 10 minutos o `waitSeeding` estoura e o comando vira `FAILED`, com o download já em curso.
3. **Critério de sucesso inadequado a parcial.** Mesmo com os dados parciais copiados corretamente, `waitSeeding` (`agent/internal/commands/commands.go:311`) exige `SEEDING && progress >= 0.999`. Um torrent a 60% nunca satisfaz isso: a migração é marcada `FAILED` embora a cópia tenha sido perfeita.

Some-se a isso a ausência de **trava de concorrência**: `DrainNode` enfileira uma migração por torrent de uma vez só, e todas puxam em paralelo — o que arrebenta o I/O do disco justamente no Node que já está em apuros.

**Alternativas consideradas**
1. **Bloquear migração de torrent incompleto** com erro claro na API/Portal. Simples e coerente com a leitura literal da ADR-018. **Rejeitada:** proíbe exatamente o caso de uso que motiva a migração (drenar Node cheio). Trataria o sintoma e deixaria o operador sem saída no cenário que mais dói.
2. **Deixar o destino re-baixar do swarm** quando o torrent está incompleto (aceitar o comportamento atual e apenas documentá-lo). **Rejeitada:** viola o princípio central da ADR-018 (sem re-download, preserva ratio) e é o oposto do que o operador quer.
3. **Suportar migração de parcial de primeira classe** (adotada): copiar o que existe, verificar por recheck, e deixar o destino continuar o download de onde parou.

**Decisão adotada** (alternativa 3)

- **Falhar cedo e alto quando não há o que copiar.** O handler passa a fazer `os.Stat` **antes** de escrever qualquer cabeçalho: conteúdo inexistente ou de tamanho zero devolve **`409 Conflict`** com mensagem legível ("torrent ainda não baixou dados — nada a migrar"), nunca um 200 com tar vazio. Um pull que não copia nada é um **erro**, jamais um sucesso silencioso.
- **Critério de sucesso proporcional ao progresso da origem.** O CP passa a enviar no payload do `migrate_pull` o campo **`min_progress`** — o progresso do torrent na origem no momento em que a migração foi criada, menos uma tolerância. O Agent destino, após o recheck, aguarda o progresso atingir `min_progress` (em vez de exigir `SEEDING`), e só então responde `DONE`. Torrent completo continua se comportando como antes (`min_progress ≈ 1.0` ⇒ exige seeding); torrent a 60% conclui a migração a 60%. *(**#87:** a espera pelo recheck e a conferência do piso passaram do Agent para o CP — ver o adendo ao fim da ADR-018; o piso é o mesmo, e o `DONE` do pull deixou de significar "verificado".)*
  - A tolerância existe porque o recheck pode invalidar a **peça de fronteira** (parcialmente escrita quando o download parou). Perder uma peça na borda é normal e não invalida a migração.
- **O destino continua o download.** Depois do recheck o torrent fica ativo no destino e retoma do swarm o que falta — comportamento correto e desejado aqui, ao contrário do re-download integral do problema 2. A economia de ratio da ADR-018 é preservada proporcionalmente: só o que **faltava** é baixado.
- **Remoção da origem segue a regra do operador.** O `complete()` do CP continua removendo o torrent da origem com os dados após o `DONE` do destino — o destino tem os mesmos bytes e é quem seguirá baixando. A regra registrada pelo operador ("manter destino, apagar origem se 100% seedando") passa a ler-se, na forma geral: **manter o destino, apagar a origem quando o destino confirmar que tem pelo menos o que a origem tinha.**
- **Trava de concorrência.** Novo tunável **`migration.max_concurrent`** no catálogo dinâmico (ADR-023, default **1**), e as migrações ganham o estado **`QUEUED`**: o reconciler promove `QUEUED → PENDING` (enfileirando os comandos de serve/pull) apenas enquanto houver folga sob o teto. O reconciler passa a ser **scheduler além de reconciliador**. Um drain de 40 torrents deixa de virar 40 pulls simultâneos.

**Consequências**
Benefícios:
- Destrava o caso de uso real: drenar Node cheio move torrents incompletos, que **terminam** no destino com espaço. É a peça que faltava para tratar a raiz do incidente de disco.
- Elimina o re-download silencioso do swarm — o defeito mais caro, porque queima ratio em tracker privado sem o operador perceber.
- Erro passa a ser legível ("nada a migrar") em vez de "arquivo zerado" seguido de falha 10 minutos depois.
- `max_concurrent` protege o I/O do disco justamente no Node em apuros, e é ajustável no painel sem redeploy.
Limitações/custos:
- **Transferência de arquivo esparso:** com pré-alocação desligada (default do qBittorrent), um torrent a 10% ocupa pouco em disco mas tem `size` total; o tar transfere o tamanho **completo** (buracos viram zeros na rede). A migração de um parcial pouco baixado é, portanto, mais cara em rede e disco de destino do que o progresso sugere. Aceito por ora — é LAN e a alternativa (tar esparso / mapa de peças) é bem mais complexa. **Registrado como limitação conhecida**, e é uma razão a mais para a admissão consciente de disco (item B1 do backlog) checar o espaço do destino **pelo tamanho total**, não pelo baixado.
- O critério `min_progress` é uma foto do instante da criação da migração: se a origem avançar entre a criação e o pull, o destino pode chegar com progresso maior que o exigido — inofensivo (o teste é um piso, não uma igualdade).
- `QUEUED` acrescenta um estado à máquina de migrações (mais uma transição a manter e testar).

**Relação com outras ADRs**
- **Estende a ADR-018**, que segue valendo integralmente no essencial (pull direto destino←origem autorizado por token de uso único, CP fora do data-plane, sem perda em caso de falha). O que muda é o **critério de conclusão**: de "destino está SEEDING" para "destino tem pelo menos o que a origem tinha".
- Usa o catálogo da **ADR-023** para `migration.max_concurrent` (hot-reload, sem redeploy).
- Pré-requisito prático da **admissão consciente de disco** (backlog B1): sem migrar parcial, drenar um Node cheio não resolve o torrent que está preso justamente por falta de espaço.

> **Correção de robustez — `complete()` atômico (jul/2026).** A finalização de uma migração tem três efeitos: enfileirar o `remove_torrent` na origem, repor/soft-deletar a linha do torrent e marcar a migração `COMPLETED`. O reconciler só tira a migração de `PENDING` quando ela vira `COMPLETED`; enquanto isso, ele reexecuta `complete()` a cada ciclo (~15s). Na implementação original o `remove` era enfileirado **primeiro** e fora de transação, então qualquer falha nos passos seguintes deixava a migração `PENDING` **com o remove já enfileirado** — e o ciclo seguinte reenfileirava outro, indefinidamente. Foi o que gerou **27.293 `remove_torrent`** para um único torrent (`Rick.and.Morty.S09E01`) em 5 dias: a reposição colidia com `uk_torrents_node_hash` porque o destino já tinha uma cópia viva do hash (o fix do `destHasRow` fechou essa colisão específica, mas não a fragilidade estrutural). Agora os três efeitos rodam numa **única transação** e o `remove` é enfileirado **por último**: falhou → rollback, nada enfileirado, retry limpo; sucedeu → 1 remove + `COMPLETED` commitam juntos, e o loop é impossível. Enqueue transacional via `command.Service.EnqueueTx`. Regressão coberta por testes de idempotência do reconciler (contados por hash do payload).

# **ADR-032**

**Título:** Admissão consciente de disco (bloquear + alertar por espaço)

**Estado:** Aprovada

**Contexto**
Nada no sistema olha o espaço em disco ao decidir onde um torrent vai. O Scheduler escolhe o Node por health score e nº de torrents (ADR-021 não toca disco); a migração (ADR-018/031) só valida ONLINE + endpoint. O resultado foi a **raiz de uma cascata** observada ao vivo (incidente 2026-07-18): um Node a 97% de disco (node-03) recebia/segurava torrents grandes que **nunca completavam** por falta de espaço — o ciclo `download → sem espaço → volta pra fila` ainda alimentava "seeds fantasmas" (torrent preso a 0% parecia vivo) e migrações que re-baixavam. A prova: `Os.Vingadores.2012`, preso nesse ciclo no node-03, **completou de primeira** ao ser migrado para um Node com espaço.

**Problema**
Impedir que um torrent seja **colocado** (add novo ou migração) num Node que não tem espaço para ele, e **avisar** quando um Node já está comprometido além do que o disco aguenta — decidindo (a) por qual número medir o espaço, (b) de onde vem o dado de disco, (c) o que fazer quando não cabe.

**Decisões (confirmadas pelo operador, 2026-07-18)**

- **(a) Medir sempre pelo TAMANHO TOTAL do torrent.** Um torrent incompleto **vai baixar por inteiro** e ocupar `size_bytes` cheios — reservar pelo baixado subestima e recria o problema. Formalização que evita **contar duas vezes** o que já está no disco:
  > **espaço projetado livre (Node)** = `heartbeats.disk_free_bytes` − Σ (por torrent vivo no Node) `size_bytes × (1 − progress)`
  >
  > O `disk_free_bytes` já reflete os bytes **já gravados**; o somatório é o que os torrents já presentes **ainda vão gravar** até completarem. Um torrent completo contribui 0 (já está no free). Um novo torrent (ou uma migração) "cabe" se `espaço projetado livre ≥ size_bytes + margem`.
- **(b) Dado de disco vem do HEARTBEAT.** `heartbeats.disk_free_bytes`/`disk_total_bytes` já existem (o Agent reporta o `/data` a cada ~5s). **Não** espera o inventário SMART detalhado (backlog F1) — este resolve a dor de imediato com o que já temos.
- **(c) BLOQUEAR *e* ALERTAR** (o operador pediu os dois, não um ou outro):
  - **Bloquear** no ponto de decisão:
    - **Migração (`Migrate`):** o tamanho é sempre conhecido (o torrent já esteve num Node). Se o destino não comporta `size_bytes + margem`, recusa com `ErrDestNoSpace` — mensagem legível, torrent fica onde está.
    - **Add via `.torrent` (`AddFile`):** o tamanho vem do parse (novo campo `Meta.Size`). O Scheduler passa a **excluir** Nodes sem espaço; se **nenhum** cabe, recusa o add (`ErrNoNodeWithSpace`).
    - **Add via magnet:** o tamanho **não** é conhecido no CP até os metadados resolverem no Agent → **não bloqueia no add**; o Scheduler apenas **prefere o Node mais folgado** (ranking por espaço projetado). Coberto pelo alerta + pela migração depois. Limitação consciente (ver Consequências).
  - **Alertar** de forma contínua: novo alerta **`NODE_DISK_FULL`** (WARNING, ADR-008) para todo Node ONLINE cujo **espaço projetado livre < margem** — ou seja, comprometeu-se com mais do que o disco conclui. Mensagem no idioma do operador: *"O Node X não tem espaço para concluir todos os torrents da fila (faltam ~N). Migre torrents para outro Node."* Deduplica por `(code, node_id)` e resolve sozinho quando folga. Pega o Node que **já** está estourado (o caso do node-03), que o bloqueio sozinho não desfaz.
- **Margem de segurança** = tunável **`admission.disk_min_free`** (bytes, ADR-023, default **5 GiB**): folga que não se deixa comprometer, para o SO/engine respirarem. É o mesmo número usado no "cabe?" e no gatilho do alerta.
- **Onde a lógica vive:** um pacote `internal/capacity` com a expressão do espaço projetado (fonte única) — consumido por Scheduler, migração e alerta, para os três nunca divergirem no cálculo.

**Alternativas consideradas**
1. **Só alertar** (não bloquear). Mais simples e nunca trava operação legítima, mas deixa o operador colocar torrent onde não cabe e correr atrás depois. **Rejeitada** por decisão explícita ("bloquear e alertar").
2. **Medir pelo baixado** (`size × (1−progress)` como necessidade do novo torrent). Subestima: o torrent cresce até o total e reintroduz o estouro. **Rejeitada** — o operador foi direto: "vai baixar todo e ocupar o espaço equivalente".
3. **Esperar o inventário SMART (F1)** para uma visão de disco por dispositivo. Mais rico, muito maior. **Adiado** — o heartbeat já traz o `/data`, que é o que importa aqui.
4. **Reserva transacional de espaço** (debitar um "orçamento" ao admitir e creditar ao concluir/remover). Preciso sob concorrência, mas exige nova tabela de reservas e reconciliação. **Rejeitada para a V1** — o espaço projetado calculado sob demanda a partir do estado real (`torrents` + heartbeat) é suficiente para 3 Nodes/1 operador e não acumula estado a sincronizar.

**Consequências**
Benefícios:
- Corta a raiz da cascata: torrent grande não é mais colocado onde não completa; o Node já estourado é sinalizado com uma ação clara (migrar).
- Reusa dado que já existe (heartbeat) — zero mudança no Agent.
- Um único número (`disk_min_free`) governa bloqueio e alerta, ajustável no painel sem redeploy.
Limitações/custos:
- **Magnet sem tamanho não é bloqueado no add** — só ganha o Node mais folgado e cai na rede do alerta/migração. Trackers privados usam `.torrent` (com tamanho), então o caminho comum é coberto; magnet público é o furo consciente.
- **`size_bytes` desconhecido** (torrent recém-adicionado, metadados não resolvidos) → a checagem é **pulada** (não dá para julgar) em vez de bloquear — evita falso positivo que travaria operação legítima. O alerta cobre o Node depois que o tamanho aparece.
- **Espaço projetado é uma estimativa sob concorrência:** dois adds/migrações quase simultâneos podem ambos "caber" e juntos estourar (não há lock de reserva). Aceito na V1 (`migration.max_concurrent=1` já serializa migrações; adds concorrentes de 1 operador são raros); o alerta pega o resultado.
- **Arquivo esparso (ADR-031):** um parcial ocupa menos em disco do que `size_bytes` sugere agora, mas **vai** ocupar o total — é justamente por isso que se mede pelo total. Coerente.

**Relação com outras ADRs**
- Complementa a **ADR-021** (admissão): aquela cuida de *quantos* baixam (teto L, peers); esta cuida de *onde cabem* (disco). Ortogonais.
- Fecha o par com a **ADR-031**: migrar parcial destrava drenar um Node cheio; a admissão consciente de disco **escolhe o destino que comporta** e **avisa** qual Node drenar.
- Usa o catálogo da **ADR-023** para `admission.disk_min_free` e o padrão de alerta da **ADR-008**.
- **Antecipa, não substitui, o inventário SMART (F1):** quando o Agent reportar discos/SMART, as regras `DISK_LOW`/`SMART_CRITICAL` somam-se a esta (falha de hardware ≠ falta de espaço).

# **ADR-033**

**Título:** Maintenance Worker — auto-cura agendada do Estado do Cluster (GC de mortos, reaper de comandos, reconciliação de migração órfã)

**Estado:** Proposta

**Contexto**
O Estado do Cluster acumula **resíduo** que nenhum caminho existente limpa. Três formas observadas ao vivo (incidente 2026-07-18):
- **Linhas de torrent soft-deletadas que nunca somem.** Toda remoção (operador, migração concluída, colisão de hash na adoção do `SyncStates`) faz *soft delete* (`deleted_at`), por design — mas nada faz o *hard delete* depois. O `status` dessas linhas congela no último valor sincronizado (o `SyncStates` filtra `deleted_at IS NULL`), gerando o efeito confuso de linhas "SEEDING" ou "DOWNLOADING" que não existem mais. Foram encontradas **68** dessas, de 05/07 a 18/07.
- **Comandos presos em `DISPATCHED`.** Um comando entregue ao Agent que nunca é confirmado (Agent reiniciou, torrent sumiu antes de executar) fica `DISPATCHED` **para sempre** — não há timeout que o mova para `FAILED`. Dois `pause_torrent` ficaram assim desde 13/07 (5 dias), mirando um torrent-fantasma já removido.
- **Migração `PENDING` travada.** O reconciler de 15s reprocessa migrações `PENDING`, mas se `complete()`/`fail()` erram de forma persistente, a migração nunca sai de `PENDING`. Foi a mecânica do loop de **27.293 `remove_torrent`** em 5 dias (a causa pontual foi corrigida tornando `complete()` atômico — ADR-031 —, mas nada garante que *outra* causa futura não prenda uma migração do mesmo jeito).

O padrão comum: **o estado desejado diverge do real e nenhum laço reconverge.** Cada bug de origem foi (ou será) corrigido, mas falta a **rede de segurança** que limpa o que escapar — sem exigir um operador olhando o banco.

**Problema**
Dar ao Control Plane um laço de **auto-cura periódico e idempotente** que (a) purgue resíduo morto além de uma janela de retenção, (b) expire comandos presos, e (c) destrave migrações órfãs — sem introduzir um processo/agendador externo novo, sem apagar o que ainda é histórico útil, e sem risco de tocar estado vivo.

**Alternativas consideradas**
1. **Cron externo / script de limpeza manual.** Um `cron` no host rodando SQL de GC. Simples, mas: vive fora do CP (outra coisa a operar, versionar, monitorar), não lê o catálogo de config com hot-reload (ADR-023), e SQL solto sem os invariantes do domínio é perigoso. **Rejeitada** por fragmentar a operação e duplicar conhecimento do schema fora do CP.
2. **Corrigir só as causas de origem, sem rede de segurança.** Já feito para o loop de remove (ADR-031). Mas trata cada bug reativamente e não limpa o resíduo já existente nem o de bugs futuros ainda desconhecidos. **Rejeitada** como solução completa — necessária, mas não suficiente.
3. **Purga imediata (hard delete no lugar do soft delete).** Eliminaria o acúmulo na fonte, mas destrói a janela de auditoria/rollback que o soft delete existe para dar, e acopla a decisão de "remover" à de "esquecer". **Rejeitada** — o valor do soft delete é justamente o adiamento.
4. **Maintenance Worker interno com rotinas idempotentes agendadas (adotada).** Um worker em background do CP, no mesmo padrão dos já existentes (Admission 20s, Migration reconciler 15s, Alert 30s): goroutine com ticker, intervalo e limiares como tunáveis (ADR-023, hot-reload), heartbeat exposto na API de saúde. Reusa tudo que já existe; o conceito **Maintenance Worker** já está previsto no Glossário.

**Decisão adotada** (alternativa 4)
Um novo worker `internal/maintenance`, iniciado pelo `seedgrid-controller` como os demais, rodando a cada `maintenance.interval` (default **1h** — resíduo não é urgente; o alvo é higiene, não latência). Cada passada executa três rotinas **independentes e idempotentes** (uma falhar não impede as outras; nenhuma age sobre estado vivo):

1. **GC de torrents soft-deletados** — *hard delete* das linhas com `deleted_at < now() − maintenance.soft_delete_retention` (default **7 dias**: mantém uma janela recente de auditoria/rollback no banco; a documentação de longo prazo vive fora, no registro de incidentes). O delete físico dispara os `ON DELETE CASCADE` já existentes (`torrent_files`, `torrent_migrations`, `torrent_stat_samples`); as referências `NO ACTION` que preservam histórico (`activities`, e `events` se aplicável) têm o vínculo **solto para NULL** antes, mantendo o log do cluster intacto; `torrent_scheduler` (efêmero) é deletado junto. **Guarda dura:** todo `DELETE` carrega `AND deleted_at IS NOT NULL` — impossível tocar uma linha viva.
2. **Reaper de comandos presos** — comando em `DISPATCHED` (entregue, nunca confirmado) há mais de `maintenance.command_stuck_after` (default **15m**) vira `FAILED` com erro explicativo. Idempotente por natureza (só age em `DISPATCHED`). Não mexe em `PENDING` (ainda não entregue — pode ser legítimo aguardando polling).
3. **Reconciliação de migração órfã** — migração `PENDING` parada há mais de `maintenance.migration_stuck_after` (default **30m**) sem convergir → `fail()` com motivo, liberando o torrent (que permanece na origem, sem perda — invariante da ADR-018). É *backstop* do reconciler de 15s, não substituto: só age no que ficou preso muito além do ciclo normal.

Todos os limiares são tunáveis do catálogo (ADR-023), editáveis no painel com hot-reload; `maintenance.interval = 0` desliga o worker inteiro (escape hatch). A primeira execução em produção limpa o resíduo já acumulado (os 64 mortos restantes após a limpeza manual do incidente).

**Consequências / Limitações**
- **Perda deliberada de histórico de banco além de 7 dias.** Uma linha morta há mais de 7 dias e suas stats/migração/arquivo somem de vez. É o trade-off aceito: o registro de longo prazo é responsabilidade da documentação de incidentes, não do estado operacional. Ajustável pelo tunável se a janela se mostrar curta.
- **Um novo worker para operar e testar** (estado a mais: nenhum; comportamento a mais: 3 rotinas). Mitigado por seguir o padrão já estabelecido e por cada rotina ser independente e idempotente.
- **Não substitui a correção de causas.** O reaper e a reconciliação tratam *sintomas* de bugs; a origem de cada um ainda deve ser corrigida (como a ADR-031 fez). O worker garante que um sintoma não persista por dias enquanto a causa não é achada.
- **GC compete por I/O com a operação.** Mitigado pela cadência larga (1h) e por operar em lote pequeno (o volume de mortos por hora é baixo); pode ganhar `LIMIT` por passada se necessário.

**Benefícios**
- **Auto-cura sem operador:** o resíduo dos três tipos deixa de exigir alguém olhando o banco — o que motivou o pedido ("como garantimos que não se repete e, se repetir, como limpar").
- **Rede de segurança para bugs futuros:** mesmo uma causa ainda desconhecida que prenda migração ou comando é contida em minutos/horas, não dias.
- **Reuso total:** nenhum processo/agendador novo; mesmo padrão de worker, mesmo catálogo de config, mesma API de saúde.

**Relação com outras ADRs**
- **Rede de segurança da ADR-031** (migração): a ADR-031 corrigiu a causa do loop na origem (`complete()` atômico); esta contém qualquer migração que ainda assim prenda.
- **Reusa a ADR-023** (catálogo de tunáveis com hot-reload) para intervalo e limiares, e a **ADR-014** (fila de comandos durável) — o reaper opera sobre o ciclo de vida do comando sem criar canal novo.
- **Não altera a ADR-006** (eventos) nem soft delete como conceito: o soft delete continua sendo como se "remove"; esta ADR só define **quando** o soft-deletado é finalmente esquecido.
- Preenche o **Maintenance Worker** já nomeado no Glossário (até aqui, conceito sem implementação).

# **ADR-034**

**Título:** Conectividade dos Seed Nodes via IPv6 sob CGNAT — qBittorrent em `network_mode: host` + `accept_ra=2`

**Estado:** Aprovada

**Contexto**
Os Seed Nodes rodam atrás de uma conexão residencial em **CGNAT** (IPv4 do carrier em `100.64.0.0/10`): não há IPv4 público próprio e **nenhuma porta de entrada v4 é encaminhável** — port-forward WAN→Node é inócuo. A connectability de entrada (ser *connectable* para o tracker/seeders) dependia, portanto, **exclusivamente do IPv6**, entregue pela operadora via SLAAC (RA anunciando `2001:db8:633:8e00::/64`, flag `auto`).

Dois fatores derrubaram o IPv6 na prática (incidente jul/2026):
- **Proxmox/LXC não formavam endereço v6 global.** Como o host roteia (`net.ipv6.conf.<iface>.forwarding=1`) e o Docker liga forwarding dentro do LXC, e `accept_ra` estava em `1` (host) / `0` (LXC), o kernel **ignora o Router Advertisement** — regra: interface que faz forwarding só aceita RA com `accept_ra=2`.
- **O qBittorrent roda em Docker, que não expõe IPv6 ao container por padrão** — mesmo com o LXC tendo v6, o processo do qBt não tinha endereço v6 para **anunciar** ao tracker.

Sintoma observado: torrents de tracker **privado** paravam em 0% (o público baixava normal). O tracker marcava a conta "porta fechada"; os seeders (seedboxes) **recusavam** o cliente inalcançável (RST μTP), e o qBittorrent reagia banindo o peer — bloqueios que apareciam como **"IP filter"** mesmo com o filtro do qBt desligado (sintoma, não causa). A raiz era **perda de conectividade IPv6**, não filtro/IDS/rede.

**Problema**
Restaurar a connectability dos Seed Nodes sob CGNAT, onde a única via de entrada possível é o IPv6, decidindo: (a) como host/LXC obtêm endereço v6 global de forma persistente, e (b) como o processo do qBittorrent (em container) passa a ter e anunciar esse v6.

**Alternativas consideradas**
1. **Port-forward IPv4 no roteador.** Inócuo sob CGNAT (sem IP público). Rejeitada.
2. **VPN com port-forward bindada no qBt** (AirVPN/Proton). Funciona independente da operadora, mas adiciona custo mensal, um hop e complexidade de bind/rota por Node. **Rejeitada como padrão** (plano B se a operadora tirar o IPv6).
3. **IPv6 no daemon do Docker** (`ipv6: true` + `fixed-cidr-v6` + NDP proxy). Com um único `/64` via SLAAC (sem prefix delegation), exige `ndppd` e subnetting frágil. Rejeitada pela complexidade.
4. **`network_mode: host` no container do qBt.** O container herda a pilha de rede do LXC (incluindo o v6 global), anuncia v6 naturalmente e fica alcançável em `[v6]:BTport`. O Agent já fala com o qBt em `127.0.0.1:8080` (não usa nome de serviço), então host networking não quebra nada; o `node-exporter` já usava esse modo. **Adotada.**

**Decisão adotada**
- **IPv6 no host e no LXC via `accept_ra=2`**, persistido em `/etc/sysctl.d/99-seedgrid-ipv6.conf` com `net.ipv6.conf.default.accept_ra=2` (garante interfaces criadas após o sysctl) **+** `net.ipv6.conf.<iface>.accept_ra=2` (vmbr0 no host, eth0 no LXC). Host e LXC passam a formar SLAAC no `/64` da operadora.
- **qBittorrent em `network_mode: host`** no `deploy/agent/docker-compose.yml` (remove-se o mapeamento `ports:`; o qBt liga 8080/WebUI e a porta BT direto nas interfaces do LXC, v4 e v6). O `listen_port` por Node segue fixado pelo Agent (ADR-014/017).
- **Requisito de rede (manual, fora do código):** IPv6 não tem NAT/port-forward, mas o firewall do roteador bloqueia inbound v6 por padrão → é preciso uma **regra de firewall v6 liberando inbound** na porta BT de cada Node (`[v6-do-node]:BTport` TCP+UDP). Substitui o antigo "encaminhar WAN v4 → Node" (inócuo sob CGNAT).

**Consequências**
Benefícios:
- Restaura a connectability sem custo (usa o IPv6 nativo da operadora); torrents privados voltam a baixar.
- `network_mode: host` é mais simples que NAT de porta (uma camada a menos) e já era o padrão do `node-exporter`.
- Persistência robusta a reboot via `default.accept_ra=2`.

Limitações/custos:
- **Depende do IPv6 da operadora.** Se ela deixar de anunciar o `/64`, os Nodes voltam a ficar inalcançáveis (plano B = VPN com port-forward).
- **Prefixo SLAAC pode rotacionar** (lifetime do RA ~2h): o endereço v6 do Node pode mudar; a regra de firewall v6 convém mirar o Node por algo estável (host/MAC), não o endereço literal — o announce do qBt se atualiza sozinho.
- **Sem NAT v6**, o Node fica diretamente alcançável por v6 — a segurança passa a depender inteiramente da regra de firewall v6 do roteador.

**Relação com outras ADRs**
- **Complementa a ADR-017** (porta BT única por Node): a porta segue a mesma; muda só o transporte de entrada (v6 em vez de port-forward v4).
- **Não altera a ADR-015** (stack) nem o contrato Agent↔qBt (ADR-014): o Agent continua em `127.0.0.1:8080`.
- Documenta a razão de o `deploy/agent/docker-compose.yml` usar host networking.

**Adendo (out/2026) — privacy extensions desligadas (#142).** O requisito de IPv6 acima tem uma exigência que não estava escrita: o Node precisa ter **um único endereço global em uso**. Com as privacy extensions do IPv6 (RFC 4941) ligadas — o padrão de várias distribuições, que gravam `use_tempaddr = 2` em `sysctl.d` —, o Node ganha, além do endereço estável, um endereço **temporário que rotaciona sozinho**, e é o temporário que o tráfego de saída usa. O operador libera no firewall o estável; o tracker vê outro, e a regra deixa de corresponder ao que de fato é anunciado.

O `bootstrap-node.sh` passa a gravar `net.ipv6.conf.{all,default,<interface>}.use_tempaddr = 0` em `/etc/sysctl.d/99-seedgrid-ipv6.conf` (o mesmo arquivo que persiste o `accept_ra=2` desta ADR), aplicar as chaves na hora e remover os temporários que já existam. É idempotente e preserva as demais linhas do arquivo. Não altera a decisão desta ADR (host networking + `accept_ra`): completa o requisito de rede dela. Ver `docs/19 - Deployment.md` §50.2.

**Adendo (out/2026) — sufixo determinístico (ADR-038, #202).** A limitação acima ("a regra de firewall v6 convém mirar o Node por algo estável, não o endereço literal") ganhou resposta: o Node passa a ter um sufixo IPv6 **determinístico** (`<prefixo>::5eed:<N>`, com `N` vindo da sequência de portas do Control Plane), de modo que o endereço deixa de depender do MAC, sobrevive a recriar o Node e é conhecido antes de ele existir. A rotação do **prefixo** pelo provedor segue valendo e segue coberta pela detecção de mudança de IPv6 (ADR-036).

# **ADR-035**

**Título:** Autorização por papéis (RBAC) para operadores — perfis admin, publisher, downloader e viewer

**Estado:** Aprovada

**Contexto**
Desde a V1 o Control Plane tem autenticação de operador (sessão Bearer, `users`/`sessions`) mas **nenhuma autorização**: qualquer usuário autenticado tem acesso total a todas as rotas protegidas (`RequireOperator` apenas injeta o operador no contexto). O schema de identidade já nasceu com as tabelas `roles`, `permissions`, `role_permissions` e `user_roles` (migration 00001), mas elas nunca foram povoadas nem verificadas — o `Bootstrap` só cria a role `Administrator` e o usuário admin inicial. A operação real do SeedGrid envolve mais gente (familiares/conhecidos consumindo conteúdo), e não há como delegar acesso limitado: um `viewer` não deveria conseguir remover torrents, nem um `downloader` publicar no tracker.

**Problema**
Definir um modelo de autorização para operadores do Portal/API que: (a) preserve o acesso total do admin existente; (b) introduza papéis de menor privilégio (publisher, downloader, viewer); (c) seja aplicado de forma obrigatória em todas as rotas protegidas; (d) permita gestão de usuários pela API sem quebrar os fluxos existentes.

**Alternativas consideradas**
1. **Autorização por flag booleana no usuário** (is_admin). Simples, mas não escala para 3 papéis de menor privilégio e não expressa permissões granulares por recurso/ação. Rejeitada.
2. **Middleware de role por rota** (ex.: `RequireRole("publisher")`). Funciona, mas casa cada rota a um único papel — um futuro papel intermediário exigiria reescrita das rotas. O schema existente (`permissions` + `role_permissions`) permite mapear rotas a permissões e papéis a conjuntos de permissões, desacoplando rota ↔ papel. Rejeitada como mecanismo primário.
3. **RBAC permission-based sobre o schema existente**: cada rota protegida exige uma permissão (`resource:action`); papéis agregam permissões via `role_permissions`; o middleware verifica `user.HasPermission(perm)`; admin resolve por wildcard. Reutiliza o schema da migration 00001, mantém granularidade e permite novos papéis sem tocar rotas. **Adotada.**

**Decisão adotada**
- **Quatro papéis de sistema** (seed na migration 00034): `Administrator` (acesso total), `Publisher` (gerencia conteúdo: torrents, trackers, publicação), `Downloader` (consome: lê e baixa), `Viewer` (somente leitura).
- **Permissões no formato `resource:action`**, seeds na mesma migration, associadas a papéis via `role_permissions`. Administrador resolve por wildcard (`*`) — não precisa de seed explícito para cada permissão nova.
- **`auth.User` passa a carregar `Roles` e `Permissions`** no `Authenticate` (JOIN com user_roles/role_permissions); ganha o método `HasPermission(perm)`.
- **Middleware `httpx.RequirePermission(perm)`** roda POR DENTRO do `RequireOperator` (já tem o usuário no contexto) e responde **403** quando a permissão falta. Cada rota protegida declara sua permissão ao ser registrada; rotas que exigem mais de uma permissão combinam middlewares.
- **`auth/password` (trocar a própria senha) exige apenas sessão** — todo operador autenticado pode trocar a própria senha.
- **Gestão de usuários (admin)**: novo pacote `internal/users` + rotas `GET/POST /api/v1/users` e `PUT/DELETE /api/v1/users/{id}` (criar, listar, atualizar, desabilitar, atribuir papéis, resetar senha) — todas com permissão `users:*`.
- **Bootstrap** passa a garantir os quatro papéis de sistema (idempotente) além do usuário admin.

**Consequências**
Benefícios:
- Menor privilégio por padrão: viewer não muta nada, downloader não publica, publisher não gerencia usuários/settings.
- Reutiliza o schema de identidade existente (migration 00001); migração 00034 é só seed idempotente.
- Rotas ↔ permissões ↔ papéis desacoplados: novo papel = combinar permissões existentes, sem tocar nas rotas.
- Auditoria (ADR-025) continua funcionando: o ator é o `auth.User.Username` já injetado.

Limitações/custos:
- Permissões são globais por operador (não por recurso/objeto) — ex.: um publisher não pode ser restrito a "só node-02". Se isso for necessário, vira ADR futura (ACL por objeto).
- Toda rota nova precisa declarar permissão explicitamente — a ausência de permissão numa rota protegida é erro de revisão, não negação por padrão (deny-by-default em rotas novas depende de disciplina no registro).
- Requer cuidado na migração do admin existente: o `Bootstrap` já associa `Administrator`, e o wildcard garante que nada quebre.

**Relação com outras ADRs**
- **Complementa a ADR-025** (auditoria): autorização é a outra metade do controle de acesso; o ator continua sendo o username do operador.
- **Não altera a ADR-014** (comandos) nem a autenticação de Agents (credencial própria do Node) — RBAC vale para operadores do Portal/API.
- **Reutiliza o schema da migration 00001** (identidade) — nenhuma mudança de tabelas, só seed.

# **4. Princípios**
Toda ADR deverá:
- registrar contexto;
- justificar a decisão;
- documentar consequências;
- permanecer versionada.
Uma ADR não descreve implementação, apenas decisões arquiteturais que orientam toda a evolução da plataforma.

# **5. Evolução das ADRs**
As ADRs representam decisões arquiteturais vigentes.
A evolução da arquitetura deverá ocorrer através da criação de novas ADRs, preservando o histórico das decisões anteriores.
Uma ADR existente não deverá ser modificada para alterar seu significado original.
Quando uma decisão deixar de ser válida, uma nova ADR deverá substituí-la explicitamente.
# **6. Quando Criar uma Nova ADR**
Uma nova ADR deverá ser criada sempre que houver mudanças que afetem a arquitetura da plataforma.
Exemplos:
- introdução de novos componentes;
- alteração significativa na comunicação entre serviços;
- substituição de tecnologias fundamentais;
- mudança na estratégia de armazenamento;
- alteração dos princípios arquiteturais;
- mudanças relevantes de segurança;
- alterações no modelo de implantação.
Mudanças de implementação não exigem novas ADRs.
# **7. Ciclo de Vida**
Cada ADR deverá possuir um estado.
Estados sugeridos:
Proposta.
Aprovada.
Substituída.
Obsoleta.
Arquivada.
O histórico deverá permanecer preservado para fins de rastreabilidade.
# **8. Numeração**
As ADRs deverão utilizar numeração sequencial.
Exemplos:
ADR-001.
ADR-002.
ADR-003.
A numeração nunca deverá ser reutilizada.
# **9. Substituição**
Quando uma ADR substituir outra, a relação deverá ser explicitamente documentada.
Exemplo:
ADR-018
Substitui:
ADR-009
Da mesma forma, a ADR substituída deverá registrar que foi superada por uma decisão posterior.
# **ADR-036**

**Título:** Como o Control Plane aprende que um Seed Node é alcançável de fora (connectability)

**Estado:** Aprovada — **registrada retroativamente**

> ⚠️ **Esta ADR foi escrita DEPOIS da implementação (#141), contrariando a §11 deste documento.** O registro fica: a decisão foi tomada no código primeiro, e a consequência principal — a semântica *grudenta* do sinal escolhido — só apareceu na verificação em produção, quando teria aparecido na seção "Alternativas consideradas" se a ADR viesse antes. A correção da lacuna está na issue #155.

**Contexto**
A ADR-034 estabeleceu que, sob CGNAT, a connectability de entrada de um Seed Node depende de IPv6: o port-forward IPv4 é inócuo e o operador precisa liberar inbound IPv6 na porta BitTorrent. O que ela **não** definiu foi como o Control Plane *sabe* se essa liberação está de fato valendo.

O buraco cobrou caro. Entre 31/07 e 01/09 de 2026 o provedor rotacionou o prefixo IPv6 delegado (`2001:db8:601:d300::/64` → `2001:db8:60b:c000::/64`). As regras de firewall do gateway usavam endereço literal e passaram a liberar endereços inexistentes. Três Nodes ficaram **semanas** sem receber uma única conexão — 553 torrents parados — enquanto o Portal os exibia ONLINE, verdes e com health alto. O único sistema que sabia da verdade era o tracker, fora do produto.

**Problema**
Definir (a) qual sinal o Control Plane usa para julgar alcançabilidade de entrada de um Node, (b) quem produz esse sinal, respeitando a ADR-012 (o Agent inicia a comunicação; o CP nunca conecta no Node), e (c) que informação o operador recebe para agir — saber que está inalcançável não basta se ele não souber **o que liberar**.

**Alternativas consideradas**

1. **Sonda ativa a partir do Control Plane** — o CP abre uma conexão TCP na porta BT de cada Node. Dá veredito **presente** e inequívoco, que é exatamente o que falta. Rejeitada por contrariar frontalmente a ADR-012 (o CP passaria a conectar no Node) e, pior, por medir o caminho errado: o CP está na **mesma LAN**, e no IPv6 dois hosts do mesmo `/64` se alcançam por descoberta de vizinhança, sem passar pela regra de firewall da WAN. A sonda daria verde justamente no cenário que se quer detectar.

2. **Probe externo (serviço de terceiro ou VPS próprio)** — mede o que importa, de fora da borda. Rejeitada para a V1: introduz dependência externa num produto que roda inteiro no homelab, e expõe endereço e porta da frota a terceiros. Continua sendo a única forma de veredito ativo verdadeiro, e fica registrada para reavaliação.

3. **Consultar o tracker (UNIT3D)** — ele já sonda por announce e sabe o estado por torrent. Rejeitada como mecanismo primário: acopla a saúde da plataforma a um tracker específico, e a informação chega no ritmo do announce alheio. Um Node sem torrents publicados ficaria sem veredito.

4. **Sinal auto-reportado pelo Engine (adotada)** — o Agent lê o `connection_status` da WebUI do qBittorrent, que expõe o `has_incoming_connections` do libtorrent, e o envia no heartbeat junto do IPv6 global da interface. Respeita a ADR-012 (Agent-iniciado, ADR-014), não depende de nada externo, e é dado que o Node já tem à mão.

**Decisão adotada** (alternativa 4)
- O **Agent** reporta no heartbeat dois campos: `public_ipv6` (endereço global roteável da interface física, descartando link-local, ULA e interfaces virtuais) e `engine_connectable` (`connected`/`firewalled`/`disconnected`).
- O **Control Plane** persiste ambos em `nodes`, mais `connectable_since` — o instante em que o veredito corrente começou (migration 00035).
- A regra **`NODE_FIREWALLED`** (WARNING) exige `firewalled` **sustentado** por `alerting.firewalled_grace` (default 30m). Motivo: o sinal nasce falso — todo Node está `firewalled` desde que o Engine sobe até o primeiro peer bater na porta. Sem carência, o alerta dispararia a cada restart do Engine, e alerta que grita à toa é alerta que o operador aprende a ignorar.
- **`NULL` é desconhecido, nunca "fechado".** Agent que não reporta o campo não gera alerta, e a métrica `seedgrid_node_connectable` é **omitida** em vez de emitir `0` — ausência de dado não pode virar acusação.
- O **Portal** exibe o endereço a liberar (`Inbound IPv6: [addr]:porta`), porque o alerta sem essa informação obrigaria o operador a entrar na máquina para agir.

**Consequências**

Benefícios:
- o CP passa a ter uma noção de alcançabilidade sem violar a ADR-012 nem depender de terceiros;
- o operador descobre **o que liberar** sem acessar o Node, e a mudança do endereço (rotação de prefixo) fica visível;
- Node novo cuja regra de firewall nunca existiu, ou nasceu errada, é detectado — a sessão começa `firewalled` e não sai.

Limitações — **a principal invalida parte da motivação**:
- ⚠️ **O sinal é grudento.** `has_incoming_connections` vira `true` na primeira conexão de entrada aceita e não volta atrás enquanto o Engine não reiniciar. Responde *"esta sessão já recebeu alguém alguma vez"*, não *"estou alcançável agora"*. Consequência direta: **a regra não teria pego o incidente descrito no Contexto**. Verificado em 06/09/2026 — node-01 e node-03, com Engine de pé desde 31/07, reportavam `connected` tendo atravessado todo o período em que estavam inalcançáveis. Fechar a lacuna exige um sinal que **decaia** (ex.: contagem de peers de entrada, flag `I`), cruzado com demanda real no swarm, porque seeder sem leecher tem zero conexões de entrada legitimamente. Ver **#155**.
- o veredito é por **Node** (sessão do Engine), não por torrent como no tracker;
- com privacy extensions ligadas há mais de um IPv6 global; a escolha é ordenada para ser estável entre heartbeats, e o `bootstrap-node.sh` desliga `use_tempaddr` para o endereço anunciado ser o mesmo que se libera.

**Relação com outras ADRs**
- Complementa a **ADR-034**, que estabeleceu a dependência de IPv6 sob CGNAT mas não definiu como observá-la.
- Respeita a **ADR-012** (Agent inicia a comunicação) e a **ADR-014** (polling): é justamente por isso que a alternativa 1 foi rejeitada.
- Usa o catálogo de tunáveis da **ADR-023** para a carência (`alerting.firewalled_grace`).
- O alerta segue o modelo da **ADR-008** (regras determinísticas avaliadas pelo Alert Worker, dedup por `code`+`node_id`).
# **ADR-037**

**Título:** Perfil de tracker como dado contribuível, não como código

**Estado:** Aprovada — **parcialmente implementada**: modelo de dados, descoberta de taxonomia, export/import, composição da descrição e host de imagem (#190, #196, #197, #192, #193) entregues; os perfis versionados no repositório e as demais filhas (#191) pendentes

**Contexto**
O SeedGrid publica em tracker UNIT3D desde a ADR-020, com POST real validado em produção contra o samaritano. O que a ADR-020 não definiu foi **onde mora o conhecimento específico daquele site** — e o resultado é que ele está espalhado por quatro lugares, nenhum deles dado:

| conhecimento | onde está hoje |
|---|---|
| taxonomia (IDs de category/type/resolution) | `portal/src/pages/publish/TrackerUploadPanel.tsx:22` — hardcoded **no frontend** |
| formato do nome do release | `control-plane/internal/release/standard.go` — o padrão do samaritano dentro do pacote genérico |
| shape da API | `control-plane/internal/unit3d/unit3d.go` — UNIT3D v8 fixo |
| credencial | tabela `trackers` (migration 00018): `name`, `announce`, `host`, `api_token`. Nenhum campo de perfil |

Enquanto o sistema tem um operador e um tracker, isso é uma verruga. O levantamento de set/2026 (`docs/Landscape.md` §4, issue #185) mudou a pergunta: **o projeto vai abrir o código**. Num produto open-source, os trackers que importam não são os do mantenedor — são os dos usuários, em sites onde o mantenedor não tem e não terá conta.

O `autobrr/upbrr` resolve o mesmo problema com um pacote Go por site (40+ deles). Funciona porque cada um foi escrito por um contribuidor **que tinha login naquele site**. É o modelo que a alternativa 2 descreve, e a razão de ele não servir aqui.

**Problema**
Definir (a) onde mora o conhecimento específico de cada tracker, (b) como esse conhecimento entra no sistema **sem depender de o mantenedor ter conta no site**, e (c) como ele é compartilhado entre instalações sem carregar credencial de ninguém junto.

**Alternativas consideradas**

1. **Manter como está** — taxonomia no frontend, um tracker por vez. Rejeitada: tracker nº 2 é uma release nova, e outro site UNIT3D com IDs diferentes quebra em silêncio, porque os IDs são números e o POST aceita qualquer um.

2. **Pacote Go por site (modelo upbrr)** — expressivo, tipado, testável, e a separação de responsabilidade deles é boa (`profile.go` / `taxonomy.go` / `rules.go` / `validation.go` / `description.go`). Rejeitada por três motivos, em ordem de peso: **(i)** só entra no produto o que o mantenedor conseguir escrever, e ele não tem conta na maioria dos sites — o teto do catálogo passa a ser a lista de logins de uma pessoa; **(ii)** contribuir exige escrever Go e abrir PR, e cada site novo exige uma release; **(iii)** derivar as tabelas da fonte deles é conversa de trabalho derivado numa abertura de código (upbrr é GPL-2.0-or-later, UNIT3D é AGPL-3.0).

3. **Delegar a publicação a uma ferramenta externa** (upbrr como dependência) — traria 100+ trackers sem manter nenhum. Rejeitada: dependência externa em alpha; o release deixaria de nascer semeado no cluster, que é o que a ADR-019 construiu; e a publicação deixaria de ser um passo da orquestração para virar um passo adjacente a ela — perdendo justamente a integração que diferencia o produto (`docs/Landscape.md` §4).

4. **Perfil como dado, em três camadas (adotada)** — o conhecimento do site vira registro exportável, produzido por quem tem a conta, e o código guarda só o que é genuinamente igual em toda a família.

**Decisão adotada** (alternativa 4)

O conhecimento de tracker se divide em três camadas, com fronteira explícita:

| camada | onde vive | quem produz | compartilhável |
|---|---|---|---|
| **família** — shape da API, auth, multipart | código Go (`internal/unit3d`) | mantenedor | n/a: é idêntico em todo site da família |
| **perfil do site** — taxonomia, formato de nome, regras, política de image host | **dado**: tabela própria, exportável em JSON | qualquer usuário com conta no site | **sim — é o ponto** |
| **config do operador** — `api_token`, announce com passkey, `anon`, `modq` | **dado**: nunca exportado | o operador | **nunca** |

- **`family` no perfil desde a primeira versão.** A V1 só aceita `unit3d`; o campo existe para que uma família nova (Gazelle) não exija remodelar o registro. Barato agora, caro depois.

- **Descoberta automática como bootstrap do perfil.** O `TorrentResource` do UNIT3D devolve, no mesmo item, o nome **e** o id de cada eixo (`category`/`category_id`, `type`/`type_id`, `resolution`/`resolution_id`). Percorrer algumas páginas do catálogo **do próprio tracker**, com o token do próprio operador, monta a tabela de IDs sem login em terceiro e sem copiar tabela de ninguém. O `unit3d.Browse` já consome esse endpoint (`GET /api/torrents/filter`) e hoje descarta os `*_id`.

- **Regras são opcionais e nascem vazias.** Publicar não depende delas; elas evitam erro, não habilitam envio. Evidência de que isso é viável na prática: o perfil do samaritano no upbrr **não tem `rules.go`** — nenhuma regra, e publica. Sites rígidos (Aither, Blutopia) têm; o nosso não precisa ter para funcionar.

- **Export/import de perfil em JSON.** A camada 3 fica **fora por construção** — tabela e estrutura separadas, não um filtro na serialização. Um perfil que vaza `api_token` por esquecimento de filtro é o modo de falha óbvio, e ele é eliminado pelo formato, não pela disciplina de quem exporta.

- **Distribuição: perfis versionados no repositório, contribuídos por PR.** Os perfis dos sites vivem como arquivos JSON no repo (`profiles/<family>/<site>.json`), e contribuir um tracker é abrir PR com um arquivo — sem escrever Go, sem o mantenedor ter conta no site, sem esperar release para quem clona. É o modelo de definições do Prowlarr/Jackett, e é o que fecha o ciclo aberto pela camada 2: descobrir (bootstrap) → ajustar → exportar → PR.

- **Duas origens, precedência explícita:** o repo **semeia** os perfis disponíveis; o banco guarda o perfil em uso e as edições locais do operador. Banco manda — a mesma precedência da ADR-023. Atualizar o SeedGrid traz perfis novos sem sobrescrever o que o operador ajustou.

- **Importação trata perfil como conteúdo não confiável:** validação de tipos e faixas na entrada, nada executável, nenhum campo livre indo direto para requisição. Vale para o import manual **e** para os perfis do repo — um perfil chega por PR de terceiro, e a revisão de PR não substitui validação em runtime.

- **A taxonomia sai do Portal.** `UNIT3D_CATEGORIES` deixa de existir no `TrackerUploadPanel.tsx`; o painel passa a renderizar o que o perfil declara. O `standard.go` vira o **default da família UNIT3D**, e o formato específico do site passa a vir do perfil.

- **Migração sem trabalho para o operador:** o samaritano existente vira o primeiro perfil, semeado a partir do que hoje está hardcoded, sem reconfigurar nada.

**Consequências**

Benefícios:
- tracker novo é **linha nova, não release nova** — e não passa pelo mantenedor;
- contribuir um tracker não exige escrever Go, ter conta conhecida por ninguém, nem esperar release: exporta o JSON;
- a separação credencial/perfil torna a contribuição **segura por construção**, que é pré-requisito para que ela aconteça;
- some a necessidade de derivar tabelas de projetos GPL/AGPL — os perfis nascem do site do próprio usuário;
- destrava, sem decisão nova, as issues #191 (regras), #192 (description por tracker) e #193 (image host por tracker), que hoje não têm onde se pendurar.

Limitações:
- ⚠️ **A descoberta só enxerga o que está no catálogo.** Categoria que ninguém nunca postou não aparece. Mitigação: descoberta incremental (rodar de novo completa) e edição manual do que faltar. Na prática, o que está no catálogo é o que o operador vai usar — mas um perfil recém-descoberto **não é completo por definição**, e a UI não pode sugerir que seja.
- **Regras e formato de nome não se descobrem.** Saem de wiki e de observação, ou seja: dependem de quem tem a conta. Um perfil contribuído sem regras publica igual — só não protege de erro.
- **Perfil errado publica errado**, e agora o erro pode vir de fora. O contrapeso é o preview congelado (#188): o operador vê o payload exato antes do POST, que a API do UNIT3D não permite testar de outro jeito (não há dry-run).
- **Família nova continua exigindo código.** Gazelle, standalone, auth por cookie e TOTP ficam fora do escopo desta ADR; o campo `family` só garante que caibam depois.
- Mais uma superfície de dado editável pelo usuário — validação na importação vira requisito, não zelo.

**Estado da implementação**

Entregue em #190 + #196 (migration 00043):
- **Modelo de dados.** Tabela `tracker_profiles`, **à parte** de `trackers` — a camada 3 fica fora por construção, porque a tabela do perfil não tem coluna de credencial (e dois testes guardam isso: um sobre o schema e a árvore de tipos, outro de ponta a ponta com um tracker que tem passkey e token). Colunas: `tracker_id` (um perfil vivo por tracker), `family`, `schema_version` e `taxonomy` em **JSONB** — escolhido sobre uma linha por termo porque a unidade de export, import e validação é o documento, porque eixo e seção novos (regras, image host, formato de nome) entram como coluna ou chave sem remodelar a tabela, e porque a taxonomia nunca é consultada por conteúdo. A integridade que o relacional daria de graça (id único por eixo, faixas) fica em `internal/trackerprofile`, em todo ponto de escrita, com a escrita serializada por `FOR UPDATE`.
- **Origem por termo** (`seed` | `discovered` | `manual`) decide quem pode mudar o nome: a descoberta **nunca sobrescreve `manual`** e nunca remove nada. A regra roda no servidor, sobre o estado travado, e não no Portal.
- **`kind` na categoria** (`movie`/`tv`/`game`): o painel de publicação deduzia "série" e "jogo" do número da categoria (2/3 e 5) — outra forma de taxonomia cravada no frontend, e só verdadeira num site. Passou a ser dado do perfil.
- **Migração do samaritano sem trabalho para o operador:** a migration semeia o tracker já cadastrado com a taxonomia que estava no Portal (mesmos ids, rótulos e ordem). **Só o samaritano** — semear a mesma lista num outro site UNIT3D perpetuaria o erro que esta ADR existe para eliminar. *Revisto na #63: a seed saiu para o produto não carregar dados de site nenhum; a mesma taxonomia vai em `profiles/unit3d/samaritano-cc.json`, importada como qualquer outro perfil.*
- **Descoberta** (`POST /trackers/{id}/discover-taxonomy`) só propõe; `apply-taxonomy` aplica, e é a ação auditada (ADR-025). O Portal deixou de ter `UNIT3D_CATEGORIES` e lê as opções do perfil; o pré-preenchimento a partir do parser passou a casar por **rótulo** e por **papel**, porque o parser devolve os IDs do samaritano.

Medido ao implementar (samaritano, 03/10/2026, 3 páginas = 150 torrents): a taxonomia que estava no Portal **já estava incompleta e divergia do site**. O catálogo mostra os tipos `PC` (50) e `Windows` (71), que o painel nunca ofereceu (jogos e programas são postados com eles), e nomes diferentes — `Series` e `Other` onde o Portal dizia `Séries` e `Outra`. É a evidência concreta da limitação já registrada acima, vista pelo outro lado: **o catálogo revela o que a transcrição à mão perdeu, mas só o que foi postado**.

Entregue em #197 (**sem migration**: a origem nova mora dentro do JSONB, e o arquivo é uma leitura do que já existe):
- **O formato de troca (`schema_version` 1).** Um arquivo JSON — o envelope de identificação mais o próprio perfil. Este é o arquivo que o código gera (nada à mão):

```json
{
  "format": "seedgrid.tracker-profile",
  "site": {
    "name": "Samaritano",
    "host": "tracker.exemplo.org"
  },
  "family": "unit3d",
  "schema_version": 1,
  "taxonomy": {
    "category": [
      {
        "id": 1,
        "name": "Filmes",
        "kind": "movie"
      },
      {
        "id": 2,
        "name": "Séries",
        "kind": "tv"
      },
      {
        "id": 5,
        "name": "Games",
        "kind": "game"
      }
    ],
    "resolution": [
      {
        "id": 2,
        "name": "2160p"
      },
      {
        "id": 3,
        "name": "1080p"
      }
    ],
    "type": [
      {
        "id": 3,
        "name": "Encode"
      },
      {
        "id": 4,
        "name": "WEB-DL"
      }
    ]
  }
}
```

  `format` (`seedgrid.tracker-profile`) identifica o arquivo: sem ele, um JSON qualquer com as mesmas chaves seria aceito. `site` é **só identificação** — o nome que o operador deu ao tracker e o host público (sem porta, caminho nem credencial) — e **não é aplicado a nada**: serve para o operador saber o que está importando e para o import avisar quando o arquivo é de outro site (os ids são do site). `family`, `schema_version` e `taxonomy` são o perfil. Chaves em minúsculas ASCII; eixos em ordem alfabética (é como o `encoding/json` ordena um mapa) e termos por `id`, então o mesmo perfil gera sempre os mesmos bytes — o que torna o arquivo versionado no repositório revisável por diff.
- **A camada 3 fica fora por construção, e o export não tem lista de campos.** O arquivo é a serialização do tipo `Document`, que **embute** o `Profile` em vez de listar os campos dele. Duas consequências, as duas deliberadas: (i) o tipo exportado não tem campo capaz de carregar credencial — não há filtro de serialização para alguém esquecer, só um tipo sem lugar para o segredo; (ii) uma seção nova do perfil (regras, description, image host) **é exportada sem tocar no export**, e ninguém precisa lembrar de exportá-la. O que o export lê de `trackers` é o nome e o host — nunca announce, `api_url`, token nem `source` — e o host é reduzido ao nome DNS: um valor que não passa na validação sai vazio, não sujo. Dois testes guardam isso, e os dois olham coisas diferentes. O **estrutural** percorre por reflexão a árvore de tipos do `Document` e reprova campo com nome de credencial, campo **opaco** (`interface{}`, bytes, `json.RawMessage` — carregariam qualquer coisa sem nome) e tipo com `MarshalJSON` próprio (emitiria o que quisesse sem a reflexão ver); há ainda um teste **do teste**, que prova que o verificador reprova cada um desses jeitos de vazar. O **textual** cria um tracker com `api_token`, announce com passkey, `api_url`, `source` e o perfil inteiro preenchido, exporta, e procura os **valores** no arquivo — nome de campo diz o que o código pretendia, o valor diz o que saiu —, além de conferir que só as chaves da lista aparecem, em qualquer nível.
- **`origin` não viaja.** O arquivo não leva a origem dos termos (`seed`/`discovered`/`manual`), o export a zera e o import **recusa** um arquivo que a traga. Três razões, em ordem de peso: **(i)** `origin` responde "quem pode mudar este nome *aqui*" — é a relação do termo com **este** operador; o `manual` de um usuário é conteúdo de terceiro para outro; **(ii)** o arquivo é não confiável, e deixar um arquivo declarar `origin: manual` seria deixar o autor dele decidir o que a descoberta do operador pode ou não corrigir, ou `origin: seed` para se passar pela migration; **(iii)** a precedência já existe do lado de quem importa. Por isso o import atribui a **sua** origem, `imported`: um quarto valor, com o mesmo efeito de `seed`/`discovered` no merge (não foi digitado por este operador, então a descoberta pode propor o nome que o site dele usa de fato) mas distinto, porque o operador precisa saber que aquele id **veio de fora e o site dele ainda não o confirmou** — o contrapeso que a limitação "perfil errado publica errado, e agora o erro pode vir de fora" pede. Não exige migration: `origin` mora no JSONB, sem `CHECK`.
- **Import: `ParseDocument` é a única porta.** Conteúdo não confiável, nesta ordem: tamanho (512 KiB; `413`), UTF-8, **forma** (chaves só `[a-z0-9_]`, sem chave repetida, no máximo 8 níveis, um valor só no topo), tipos (`DisallowUnknownFields`: campo desconhecido é erro), e conteúdo — `format`, `schema_version`, `family`, `site`, os eixos (conjunto fechado), `id` em `1..1000000` e único por eixo, nome com 1 a 64 caracteres sem controle, sem formatação invisível e sem as runas que **parecem espaço sem serem espaço** (os preenchimentos de Hangul U+115F, U+1160, U+3164 e U+FFA0, o Braille vazio U+2800, as vogais inerentes do Khmer, o juntador de grafema — um nome só delas parece vazio na tela e passa por "1 a 64 caracteres"), `kind` só em `category`, no máximo 256 termos por eixo. As regras de forma existem porque o `encoding/json` casa chave com campo **ignorando maiúsculas e até dobras Unicode** (uma chave com o sinal Kelvin, U+212A, casa com `kind`) e fica com a **última** de duas chaves repetidas em silêncio: um arquivo revisado num PR por quem leu a primeira seria importado pela segunda. Mensagens de erro nunca devolvem o valor recebido (ele pode ter megabytes). Nada do arquivo vira requisição ao tracker: o nome é rótulo, e os ids só viajam no POST de publicação — onde o preview congelado (#188) mostra o payload exato.
- **Uma seção sem tratamento é recusada (fail closed).** Exportar uma seção nova é de graça; **importá-la** exige validação e regra de merge. O import só aplica as seções registradas em `handledSections`; uma seção do `Profile` que não esteja lá e venha **preenchida** num arquivo é recusada em runtime, e um teste reprova a suíte quando alguém acrescenta uma seção ao `Profile` sem tratá-la — com a mensagem de o que fazer. É o preço, assumido, de uma seção nova não poder ser aceita sem conferência.
- **`schema_version`.** Sobe quando o formato muda de um jeito que um import antigo não entenderia: um campo que muda de forma **ou uma seção nova** — salvo a seção **opcional e aditiva** (a `description`, #192: ver abaixo). O export carimba a versão que o **código** emite (não a da linha do banco); o import aceita de `MinSchemaVersion` até a atual e recusa o resto com `unsupported_schema_version`, dizendo qual versão este SeedGrid entende. É o que transforma "perfil novo num SeedGrid velho" numa mensagem clara em vez de "campo desconhecido". Subir `SchemaVersion` sem baixar `MinSchemaVersion` obriga a escrever o upgrade da versão anterior; hoje há uma só versão, e portanto nenhum upgrade.
- **Pré-visualizar e aplicar são duas rotas** (`…/profile/import/preview` e `…/profile/import`), não uma rota com `dry_run`. Três motivos: **(i)** a trilha de auditoria (ADR-025) decide por método e caminho — a descoberta já é uma rota própria pelo mesmo motivo —, então uma rota de leitura fica fora dela sem remendo, e uma rota única com flag auditaria a pré-visualização como se fosse import; **(ii)** com uma flag, "esqueci a flag" **grava**; com duas rotas, a que é fácil de chamar sem querer nunca grava; **(iii)** o Portal já tem dois passos (arquivo → pré-visualização → confirmar), e cada um é uma chamada. As duas recebem **o mesmo corpo: o arquivo, byte a byte** (`curl --data-binary @perfil.json`). A pré-visualização devolve o plano — termo a termo `new`, `same`, `renamed` ou `protected`, mais contagens, avisos (inclusive o de arquivo de **outro site**, que **também sai quando a comparação não é possível** — arquivo sem host ou tracker sem host válido —, para um arquivo não desligar a conferência só por omitir o campo) e o que o arquivo não traz (`local_only`) — e o aviso de que o conteúdo veio de fora; **não grava nem cria a linha do perfil**. Não foi adotado um token que amarre a pré-visualização ao aplicar (o padrão do #188): lá o payload vai a um terceiro e não se desfaz, e o token impede postar o que ninguém conferiu; aqui a garantia que importa — não sobrescrever o `manual` — roda **no servidor, sobre a linha travada**, não sobre o que a tela viu, e a resposta do aplicar traz o plano como foi **executado**. Um token só acrescentaria um `409` para um risco que a regra já neutraliza.
- **A regra de convivência é a do merge da descoberta (#219), estendida — e mais estrita onde a descoberta já é opt-in.** Id ausente → **adiciona** (`imported`); mesmo nome → nada; nome diferente → **renomeia só o que veio da migration (`seed`) ou de um import anterior (`imported`)**. O que o operador digitou (`manual`) **e o que o site dele confirmou na descoberta (`discovered`)** é `protected`: o nome do site do operador vale mais que o de um arquivo de terceiro, e a descoberta já trata rename como opt-in — o import não pode ser mais permissivo que ela. **Falha fechada:** uma origem que o código não conhece (linha escrita por outra versão, ou à mão no banco) também é protegida, no import e na descoberta. O que o arquivo não traz fica **intocado** — o import nunca remove, porque ausência no arquivo não é evidência (o arquivo pode ser de um perfil incompleto, como o descoberto). "Sem ele pedir" tem forma: o operador pede **termo a termo** (`?overwrite=category:2`, repetível) e só vale para um termo protegido que o arquivo traga; um pedido sem correspondência vira aviso, nunca efeito. `kind` só **preenche** um papel vazio e nunca troca um que existe — descoberta não cria `kind`, então preencher é o ganho mais comum de importar sobre um perfil descoberto — e preencher só o `kind` não troca a origem (a mesma regra da edição manual). Um termo `protected` não tem nem o papel tocado.
- **Um termo novo não pode ofuscar o certo.** O painel de publicação casa o nome do release com o do termo (`findByLabel`) e pré-preenche a primeira categoria de cada papel. Um arquivo hostil — ou só descuidado — que traga `{id: 1, "Séries", tv}` num perfil que já tem Séries = 2 mudaria o que o painel escolhe, sem o operador ter decidido nada. Três defesas: **(i)** o arquivo com dois nomes que só diferem em caixa ou acento **dentro do mesmo eixo** é recusado (`taxonomy.category[1].name: nome repetido no eixo…`), com a mesma normalização do Portal (`NormalizeLabel` ↔ `normalizeLabel`: NFD sem marcas de acento, minúsculas, espaços colapsados — há teste de paridade); **(ii)** a pré-visualização avisa, **em cada linha**, o termo novo ou renomeado cujo nome colide com o de outro id do eixo — calculado sobre o estado em que o eixo FICARIA, não sobre o atual — e o termo que passaria a ser a **primeira** categoria de um papel que já tinha dono (vários termos com o mesmo papel são normais — séries e animes são `tv` —; só mudar o dono do padrão avisa); **(iii)** no Portal, `findByLabel` só devolve um termo quando o rótulo é **inequívoco** — com dois ids para o mesmo nome, não pré-preenche e o operador escolhe. Como o export passa pela mesma validação, um perfil local com nomes repetidos **não é exportado** (`409 profile_not_exportable`, com o motivo): o export nunca entrega um arquivo que o próprio import recusaria.
- **O nome do site no arquivo é o rótulo livre do operador — e pode conter uma credencial.** Quem cola o announce (ou só a passkey) no nome do tracker levaria a passkey para um arquivo feito para ser compartilhado. `siteLabel` **não limpa nem trunca** (truncar um announce de 90 caracteres deixaria os primeiros ~27, passkey inclusa): um nome que não passa em `CleanName`, ou que **parece conter uma credencial** — barra, `\`, `?` ou `=`, um UUID, ou 20 ou mais letras e dígitos seguidos —, é trocado pelo **host público** (se couber em 64 caracteres) ou por `tracker`. O limite de 20 é uma heurística, e a decisão é do operador: um fragmento menor colado no nome passa, e por isso o Portal diz o que o arquivo leva e pede para conferir antes de compartilhar.
- **Auditoria (ADR-025):** só o import aplicado é registrado («Importou um perfil de arquivo no tracker «Y»: N novo(s), M renomeado(s), K papel(éis) preenchido(s); P mantido(s) por serem do operador ou já confirmados pelo site dele»). **Nada que veio do arquivo entra na trilha**: o nome do site dele é texto de terceiro e, com as aspas «», forjaria o resto da linha. O nome do tracker passa pelo mesmo rótulo seguro e perde as aspas. A pré-visualização e o export são leitura e ficam fora.
- **RBAC no padrão dos endpoints de tracker:** exportar é `trackers:read` (o conteúdo já está na listagem de trackers, que o mesmo papel lê); pré-visualizar e importar são `trackers:write`. Tracker é de quem o cadastrou: o de outro operador não existe para quem pergunta (`404`).

Entregue em #192 (**sem migration**, **sem coluna nova**): a **composição da descrição** do upload sai do Portal e passa a ser uma **seção do perfil** do tracker.
- **O que mudou de lugar.** `portal/src/pages/publish/bbcode.ts` montava o BBCode (pôster, sinopse, tags, screenshots e MediaInfo; capa, título, sobre, gêneros e plataformas) — fora do alcance de qualquer cliente da API, sem onde dois sites divergirem e sem teste em Go, sendo o campo que o site publica na íntegra. Agora é `internal/description` (`ComposeMedia`/`ComposeGame`), chamado por `POST /api/v1/publish/description` (`publish:write`, fora da auditoria: só compõe texto). O Portal pede o texto e o entrega ao campo, que o operador continua editando — a conferência humana da ADR-020 não muda — e `buildBBCode`/`buildGameBBCode` foram removidos.
- **O que o perfil decide — e só isso.** A seção `description` é `{media_blocks, game_blocks, media_info_tag}`: **quais blocos entram e em que ordem** (omitir da lista é desligar; `[]` é nenhum) e a **tag que envolve o MediaInfo** (`mediainfo`, `code`, `pre` ou `spoiler`), que é o que varia de site para site. Os ids de bloco são um conjunto **fechado** e a tag é uma lista fechada: **não há template livre**. Foi a alternativa rejeitada de propósito — um perfil é conteúdo de terceiro, a tag é interpolada em BBCode que o site publica, e um campo de texto que chega num arquivo e vira markup é exatamente o "campo livre indo direto para requisição" que esta ADR proíbe. Quem precisar de um bloco que não existe abre PR com o bloco, não com um campo. *Desde a #74 a seção tem também `language` (`en` | `pt-BR`, vazio = `en`): o idioma do conteúdo do site — os títulos dos blocos e o idioma em que o TMDB responde (sinopse e gêneros, na publicação e na tela Explorar). Lista fechada, como a tag.*
- **Paridade medida, não afirmada.** Com a config default a saída é **byte a byte** a do TypeScript. As strings do teste foram calculadas **executando** o código TS original sobre 44 entradas, e não escritas à mão; o `trim()` do JS e o `strings.TrimSpace` do Go divergem (BOM e NEL), então o CP tem um `jsTrim` com o conjunto do JS.
- **Onde mora.** Na mesma coluna JSONB (`tracker_profiles.taxonomy`), como a chave reservada `description` **ao lado dos eixos**. O documento que toda linha já tem é, byte a byte, um documento novo sem a seção — daí não haver migration nem reescrita. A escolha foi entre isto e um envelope (`{"taxonomy": …, "description": …}`), que exigiria migrar todas as linhas ou adivinhar o formato de cada uma na leitura; a chave reservada não tem ambiguidade (um eixo nunca se chama `description`, o conjunto de eixos é fechado). **Leitura estrita:** um documento com `description` de tipo errado é erro, nunca "perfil vazio" — a próxima edição o regravaria por cima.
- **Edição por rota própria.** `PUT`/`DELETE /trackers/{id}/profile/description` (`trackers:write`), e não um campo opcional no `PUT …/profile`: o `PUT` do perfil é por substituição de eixos enviados, e um campo ausente apagar a seção seria exatamente o descuido que o contrato "ausente não é tocado" existe para impedir. `Edit`, `apply-taxonomy` e o import repassam a seção como a leram. A validação roda **no serviço**, em todo ponto de escrita — não só no handler.
- **Export/import.** O `Document` embute o `Profile`, então a seção é **exportada sem tocar no export**; o import precisou de validação (a mesma da edição manual, em `ParseDocument`) e de regra de merge — `description` entrou em `handledSections`, que é o que o teste de fail closed cobra. A seção é **uma unidade** sem origem por termo, e a regra é a do `manual` aplicada a ela inteira: perfil sem composição própria (ou com o default) → aplica; igual → nada; **diferente → protegida**, e só `?overwrite=description` a troca. O arquivo que não a traz não a toca.
- **`schema_version` não mudou.** A seção é opcional e aditiva e o export a omite no default, então um perfil sem ela continua sendo exatamente o arquivo da v1; subir a versão invalidaria todo arquivo já compartilhado por um acréscimo que quase nenhum perfil usa. **Isto abre uma exceção à regra «uma seção nova sobe a versão»**, escrita acima para a #197 — vale para seção **opcional e aditiva**; uma seção que mude o significado do que já existe continua subindo. O custo, assumido: um SeedGrid anterior à #192 recusa um arquivo com `description` por `campo desconhecido`, não por `unsupported_schema_version`.
- **Limitação: voltar de versão.** Um Control Plane anterior à #192 lê a coluna inteira como taxonomia e **não lê** o perfil de um tracker cuja composição foi personalizada (o objeto `description` não é uma lista de termos). É a contrapartida de não ter migration; mitigação: `DELETE …/profile/description` (ou apagar a chave) antes de voltar de versão. Quem nunca personalizou não é afetado.

Entregue em #193 (migration `00045`): o **host de imagem do tracker** sai de global e único e passa a ser dado do perfil, com fallback para o imgbb. A seção `image_host` é opcional e aditiva, de forma fechada (POST multipart com uma parte de arquivo, chave num header personalizado, URL da resposta por caminho com pontos sobre JSON). A chave do operador é camada 3 — entra por rota própria e fica presa ao host do `upload_url` vigente. A cadeia de upload tem orçamento de tempo e cada falha vira entrada no Feed (kind `screenshot_host_failed`). A seção viaja no export/import do perfil com a mesma regra de seção-unidade da `description`; `schema_version` não sobe. Detalhe na ADR-040.

Pendente: os perfis versionados no repositório — o **formato e o validador estão prontos** (o arquivo em `profiles/<family>/<site>.json` é exatamente este arquivo, e um teste de CI que passe cada um deles por `ParseDocument` é o que falta, junto da semeadura e do diretório) —, o `standard.go` como default da família com o formato de nome vindo do perfil, e regras (#191).

**Relação com outras ADRs**
- **Substitui parcialmente a ADR-020**, que fixou taxonomia e formato de nome do samaritano como se fossem do produto. O contrato com a API UNIT3D v8 continua valendo integralmente.
- **Afeta a ADR-030** (host de screenshots único): a política de image host passa a ser campo do perfil (#193).
- **Preserva a ADR-019** — o conteúdo continua nascendo, sendo semeado e sendo publicado no mesmo fluxo, com o Scheduler escolhendo o Node. Foi o principal motivo para rejeitar a alternativa 3.
- **Segue a ADR-029** na separação entre segredo e configuração: credencial de tracker nunca sai na API nem no export.

# **ADR-038**

**Título:** Endereço IPv6 estável e previsível por Seed Node — sufixo determinístico `::5eed:<N>`

**Estado:** Aprovada — implementação na #202

**Contexto**
A ADR-034 estabeleceu que um Seed Node sob CGNAT é alcançado por IPv6, e que o operador libera no firewall dele a entrada IPv6 na porta BitTorrent do Node. Essa regra aponta para um **endereço**. Por padrão o endereço SLAAC é **EUI-64**, derivado do MAC da interface, e isso cria três problemas:

1. **O endereço não sobrevive a recriar o Node.** Recriar o container de sistema, trocar a placa de rede ou migrar o Node de hospedeiro muda o MAC e, com ele, o endereço — e a regra de firewall passa a apontar para o vazio, em silêncio.
2. **O endereço é imprevisível antes de o Node existir.** O onboarding zero-touch (ADR-017) não consegue preparar a liberação de antemão: o operador só descobre o endereço depois de o Node subir, e entre subir e liberar o Node fica `firewalled`.
3. **Não há como trocar o endereço quando algo a montante o envenena.** Já aconteceu: um equipamento de borda passou a descartar exatamente o endereço EUI-64 de um Node, enquanto outro endereço global da **mesma** interface, do mesmo `/64`, alcançava a internet normalmente. A saída foi trocar o sufixo do Node — mas descobrindo o mecanismo na hora, com o Node inalcançável.

**Problema**
Definir como o Node escolhe a metade inferior (64 bits) do seu endereço IPv6 de modo que ela seja **estável** (independente de MAC, placa e hospedeiro), **previsível antes de o Node existir**, **única** no Cluster e que **continue acompanhando a rotação de prefixo** do provedor — que é a metade superior e que o provedor controla.

**Alternativas consideradas**

1. **Manter o EUI-64 (status quo).** Rejeitada pelos três problemas do Contexto. Funciona enquanto nada muda, e é justamente quando algo muda que a regra deixa de valer.

2. **Endereço estático completo** (`Address=`, `ip addr add`). Estável e previsível, mas **congela o prefixo**: quando o provedor rotaciona o `/64`, o Node continua anunciando um endereço que já não existe, e a falha é silenciosa — o cenário que a detecção de mudança de IPv6 (ADR-036) existe para pegar. Rejeitada.

3. **Endereço de privacidade estável (RFC 7217, `stable-privacy`).** Estável por rede e acompanha o prefixo, mas é **opaco** por construção: não dá para calculá-lo antes de o Node existir, que é o requisito 2. Rejeitada.

4. **Reserva em DHCPv6.** Resolveria os três, mas depende de o roteador do operador oferecer DHCPv6 com estado — o que a conexão residencial típica sob CGNAT (só SLAAC) não oferece. Rejeitada por exigir infraestrutura que o produto não pode presumir.

5. **Token de interface com sufixo determinístico (adotada).** O Node mantém o prefixo vindo do Router Advertisement e fixa a metade inferior. É suportado pelo systemd-networkd (`IPv6Token=`) e pelo NetworkManager (`ipv6.token`), e não toca na parte do endereço que o provedor controla.

Dentro da alternativa 5, **de onde vem o número do Node (`N`)**:

- *O operador digita.* Sujeito a erro e a colisão; contraria o "um comando" da ADR-017. Rejeitado.
- *Hash do hostname ou do MAC.* Pode colidir, e o MAC é justamente o que se quer deixar de usar. Rejeitado.
- *Um contador novo no Control Plane (coluna em `nodes`).* Resolve, mas é **estado novo** com migration, backfill e uma segunda sequência para manter coerente com a primeira, sendo que o Control Plane **já tem** um recurso único por Node, atribuído no enroll e devolvido ao bootstrap: a porta BitTorrent (ADR-017). Rejeitado.
- *Derivar da porta BitTorrent (adotado).* A porta já é única entre os Nodes ativos e as reservas vivas, sai da mesma resposta do enroll e volta ao pool quando o Node é removido. Reusar essa garantia dá a unicidade do índice de graça.

**Decisão adotada** (alternativa 5)

- **Convenção.** O endereço do Node é `<prefixo /64 anunciado no RA>::5eed:<N>`. O sufixo `5eed` é reconhecível a olho em `ip -6 addr` e não colide com os endereços estáticos usuais (`::1`, `::100`) nem com um EUI-64 (que contém `ff:fe`). `N` é escrito **em dígitos decimais** no último grupo do endereço — o Node 12 é `::5eed:12` —, de modo que o operador lê o número do Node direto do endereço, sem converter para hexadecimal. Cabe em quatro dígitos (`N` ≤ 9999); a sequência de portas do Control Plane tem 1000 posições.
- **O índice do Node é a posição dele na sequência de portas BitTorrent**: `N = porta_bt − 6880`. O Control Plane o devolve no enroll como `node_index`, junto de `bt_port` — mesma resposta, mesma reserva, mesma unicidade. **Não há migration:** nada novo é guardado, e a função `node.IndexForPort` é o único lugar que conhece a regra.
- **Os Nodes que já existem não mudam de endereço.** Quem já aplicou a convenção à mão — `::5eed:1` a `::5eed:4`, nos Nodes de porta 6881 a 6884 — fica exatamente no mesmo endereço: a regra acima reproduz esses números. Nenhum backfill, nenhuma troca de regra de firewall.
- **Mecanismo, por gerenciador de rede** (`bootstrap-node.sh`, depois do enroll e antes de subir o Engine):
  - **systemd-networkd** — drop-in `IPv6Token=static:::5eed:<N>` (junto do `IPv6AcceptRA=true` que a ADR-034 já exige);
  - **NetworkManager** — o keyfile da conexão ganha `addr-gen-mode=eui64` e `token=::5eed:<N>`, com backup e reversão se a aplicação falhar (ver `docs/19 - Deployment.md` §50.2 para as armadilhas, que não são óbvias);
  - **outro** — o script não adivinha: imprime as instruções para o gerenciador do operador, que é o que também permite a quem prefere configurar a rede à mão fazê-lo com o endereço certo.
- **Existir não basta: o endereço tem que renovar.** O critério de aceite de qualquer procedimento que aplique o token é observar o `valid_lft` do endereço **subir** depois de um Router Advertisement. Um endereço que existe mas não renova some sozinho quando o lifetime acaba; o `autoconf=0` produz exatamente esse estado (o endereço fica de pé com o lifetime que já tinha e expira em cerca de duas horas), e por isso **não é** a forma de remover o endereço EUI-64 que sobra — esse resíduo se apaga diretamente, com o `autoconf` preservado.
- **Opt-out.** `IPV6_SUFFIX=off` no bootstrap mantém o endereço que o gerenciador de rede escolher. É a saída de quem usa endereçamento delegado, DHCPv6 com estado ou uma política própria.
- **Não substitui a detecção de mudança de IPv6 (ADR-036).** O token fixa só o **sufixo**; o prefixo continua vindo do provedor, e uma rotação de `/64` ainda muda o endereço de todos os Nodes e ainda invalida as regras. O `NODE_IPV6_CHANGED` continua cobrindo o caso. O que muda é o **custo do conserto**: sobra **uma** incógnita (o prefixo novo) e o resto é aritmética — `<prefixo>::5eed:<N>`.

**Consequências**

Benefícios:
- as regras de entrada **sobrevivem a recriar o Node**, trocar a placa ou migrar de hospedeiro: o endereço não depende mais do MAC;
- o operador pode **preparar a liberação antes de o Node existir** — o próximo Node recebe a menor porta livre da sequência (a lista de Nodes do Portal mostra as portas em uso), e o endereço decorre dela;
- remover um Node devolve a porta e o índice ao pool; o Node que o substitui herda o **mesmo endereço**, e as regras seguem valendo sem edição (é intencional, e é a razão de o índice sair da porta);
- um endereço envenenado a montante tem remédio conhecido: trocar o sufixo é um procedimento, não uma investigação;
- o `public_ipv6` que o Agent reporta (ADR-036) passa a ser previsível, e a rotação de prefixo, mais fácil de tratar — o Control Plane poderá, no futuro, calcular os endereços novos em vez de apenas alertar.

Limitações:
- **O prefixo continua sendo do provedor.** Esta ADR não resolve rotação de prefixo; só barateia o conserto.
- **Endereço previsível é endereço fácil de mirar.** Quem conhece o prefixo adivinha `::5eed:1`, `::5eed:2`… A segurança já dependia inteiramente do firewall do roteador (ADR-034: não há NAT v6), e só a porta BitTorrent do Node é liberada; o ganho de enumerar Nodes é pequeno, mas existe, e quem não aceita o custo usa o opt-out.
- **O índice é derivado da porta.** Um Node registrado pelo token compartilhado, com porta fora da sequência do Control Plane (instalação manual), não tem índice — o bootstrap não aplica o token nele, e o operador usa o opt-out ou o endereço que preferir. Mudar a regra de atribuição de portas muda a regra de atribuição de endereços: as duas andam juntas de propósito.
- **O caminho do NetworkManager é o mais frágil.** Escreve-se o keyfile à mão porque a interface de linha de comando não consegue fazer isso (duas validações que se excluem), e ele depende de o keyfile ser o backend de gravação da conexão; num sistema em que ele é gerado (por exemplo por netplan), o script recua para instruções manuais.

**Relação com outras ADRs**
- **Complementa a ADR-034**: ela estabeleceu que o operador libera o endereço IPv6 do Node no firewall; esta o torna estável e previsível. O adendo dela (privacy extensions desligadas, #142) é pré-requisito: o token fixa o endereço SLAAC estável, mas um endereço temporário ainda precisa estar desligado para o tráfego de saída usá-lo.
- **Estende a ADR-017**: a porta BitTorrent atribuída no enroll passa a ser também a origem do índice do Node (`node_index` na resposta do enroll).
- **Preserva a ADR-036**: `public_ipv6` e `NODE_IPV6_CHANGED` seguem como estão.
- **Respeita a ADR-012**: o Control Plane só devolve um número; quem configura a rede é o Node, que puxou o enroll.

# **ADR-039**

**Título:** Configuração de comportamento do Agent e do Engine vem do Control Plane — valor de cluster com override por Node

**Estado:** Aprovada — implementação na #144

**Contexto**
Os ajustes de comportamento do Agent e do Engine (`MAX_ACTIVE_DOWNLOADS`, `HEARTBEAT_INTERVAL`, `COMMAND_POLL_INTERVAL`, `LOG_LEVEL` e a fila do Engine) eram **variáveis de ambiente por máquina**, lidas uma vez no boot do Agent (`agent/internal/config/config.go`). O `bootstrap-node.sh` escreve só as chaves de identidade; o resto cai no default do código. Consequência: **todo Node novo nasce diferente dos outros e ninguém percebe**. O node-04 entrou com `MAX_ACTIVE_DOWNLOADS=2` contra `10` nos outros três, e sem `HEARTBEAT_INTERVAL`/`COMMAND_POLL_INTERVAL`/`LOG_LEVEL` no `.env`.

Esses valores são comportamento de **cluster**, não de máquina — e a ADR-023 já criou o catálogo de tunáveis com hot-reload exatamente para isso. Só que o catálogo era do Control Plane: o Agent não o lia.

Mas a frota é **heterogênea de verdade**. Medição de campo (06/09/2026, issue #144): com os mesmos ~13 MB/s de download, o qBittorrent consome 0,5–2,9% de CPU nos três Nodes com Core i5 e **50,7% no node-04** (Atom x5-Z8350 @ 1,44 GHz — o custo de hashing de peças), com 809 MB em swap. Em 05/09 alguém aplicou `MAX_ACTIVE_DOWNLOADS=10` no node-04 "para alinhar com os outros" — exatamente o que uma automação ingênua faria — e foi preciso reverter para 3. Um valor único de cluster **reproduz o erro em escala**.

**Problema**
Fazer o comportamento do Agent/Engine ser decidido em **um lugar só** (o Control Plane, ADR-001), editável pelo Portal e visível por Node, **sem** apagar a diferença legítima entre máquinas, e **sem** derrubar nem sobrecarregar os Nodes que já estão em produção no dia da migração.

**Alternativas consideradas**

1. **Continuar com env por máquina e só automatizar o bootstrap** (escrever os mesmos valores em todo Node novo). Rejeitada: continua sendo configuração espalhada em N arquivos que ninguém audita, e o valor que o bootstrap escreveria seria o mesmo erro do node-04.
2. **Um valor único de cluster, sem exceção.** Rejeitada pela medição acima: o Atom e o i5 não aguentam o mesmo número de downloads.
3. **Derivar o teto da capacidade que o Node reporta** (`cpu_cores`, `mem_total_bytes`, `disk_io_util_pct`). Atraente, mas é uma heurística opaca: o operador não sabe por que o Node recebeu 3 e não 4, e o número certo depende de coisas que o heartbeat não vê (o Atom do node-04 também roda outros serviços). Fica como evolução futura — e o override por Node, que esta ADR cria, é o mecanismo onde ela se apoiaria.
4. **Valor de cluster (catálogo, ADR-023) com override por Node (adotada).** O catálogo continua sendo a fonte única de metadata e dá o valor de cluster; uma tabela nova guarda os desvios deliberados de cada Node.

Dentro da alternativa 4, **o canal de entrega ao Agent**:

- *Endpoint dedicado `GET /api/v1/agent/config`, consultado em loop próprio.* Funciona, mas é uma requisição a mais por ciclo, uma rota a mais e um loop a mais no Agent para entregar algo que muda uma vez por mês.
- *Comando `apply_config` na fila (ADR-014).* A fila é para **ações** (pausar, migrar, atualizar), não para estado: um comando perdido, descartado pela poda de retenção ou nunca enfileirado (Node que já existia) deixaria o Node com um valor antigo **sem ninguém saber**. Config é estado que precisa convergir, não evento que pode se perder.
- *Resposta do heartbeat (adotada).* O Agent já chama o Control Plane a cada ciclo, o canal já é autenticado e Agent-iniciado (ADR-012), e o Node recebe o **estado desejado completo a cada heartbeat** — se perder um, o próximo corrige. Zero requisição nova. O custo é acoplar config à resposta de um endpoint de liveness; fica contido porque a resolução é só leitura e **nunca derruba o heartbeat** (se falhar, a resposta sai sem o bloco de config e o Agent mantém o que tem).

**Decisão adotada** (alternativa 4, entrega pela resposta do heartbeat)

**1. Precedência** (do mais forte ao mais fraco):

> **override do Node > valor de cluster (catálogo, ADR-023) > env do Node > default do código**

O env do Node deixa de ser fonte e passa a ser **fallback**: vale só quando o Control Plane não manda nada (CP anterior a esta ADR, CP inalcançável, ou o Agent recém-iniciado antes do primeiro heartbeat). O default do código vale quando nem o env existe.

**2. Classificação das chaves.** O critério: o valor responde *"como ESTA máquina se liga ao resto"* (identidade/máquina → fica no `.env`) ou *"como o cluster quer que o Agent/Engine se comporte"* (comportamento → catálogo)? Uma chave de comportamento **pode** ter valor diferente por Node — para isso existe o override —, o que não a torna de máquina.

| Variável de ambiente | Classe | Onde vive |
|---|---|---|
| `SEEDGRID_CONTROL_PLANE_URL` | identidade (como falar com o CP — não tem como vir do CP) | `.env` |
| `SEEDGRID_AGENT_REGISTRATION_TOKEN` | identidade / segredo | `.env` |
| `SEEDGRID_AGENT_DATA_DIR` | máquina | `.env` |
| `SEEDGRID_AGENT_ENGINE` | máquina (qual adaptador de Engine o Node roda) | `.env` |
| `SEEDGRID_ENGINE_QBT_URL`, `_USERNAME`, `_PASSWORD` | máquina / segredo (credencial do Engine) | `.env` |
| `SEEDGRID_AGENT_STORAGE_PATH` | máquina | `.env` |
| `SEEDGRID_ENGINE_BT_PORT` | identidade (atribuída pelo CP no enroll, ADR-017; casa com o port forward do operador) | `.env` |
| `SEEDGRID_AGENT_MIGRATION_PORT` | máquina / rede | `.env` |
| `SEEDGRID_ENGINE_CONTAINER` | máquina (nome do container do Engine; contrato com o compose, #146) | `.env` |
| `SEEDGRID_DATA_PROPAGATION` | máquina (propagação do mount no compose, #143) | `deploy/agent/.env` |
| `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` (#221) | máquina (declaração do operador: o Node tem disco dedicado em `/data`?) | `.env` |
| `SEEDGRID_AGENT_HEARTBEAT_INTERVAL` | **comportamento** | catálogo `agent.heartbeat_interval` |
| `SEEDGRID_AGENT_COMMAND_POLL_INTERVAL` | **comportamento** | catálogo `agent.command_poll_interval` |
| `SEEDGRID_LOG_LEVEL` | **comportamento** | catálogo `agent.log_level` |
| `SEEDGRID_ENGINE_QUEUEING_ENABLED` | **comportamento** | catálogo `engine.queueing_enabled` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS` | **comportamento** | catálogo `engine.max_active_downloads` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS` | **comportamento** | catálogo `engine.max_active_uploads` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS` | **comportamento** | catálogo `engine.max_active_torrents` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING` | **comportamento** | catálogo `engine.max_active_checking` |
| `SEEDGRID_ENGINE_DONT_COUNT_SLOW` | **comportamento** | catálogo `engine.dont_count_slow` |
| `SEEDGRID_ENGINE_SLOW_DL_THRESHOLD` | **comportamento** | catálogo `engine.slow_dl_threshold` |
| `SEEDGRID_ENGINE_SLOW_UL_THRESHOLD` | **comportamento** | catálogo `engine.slow_ul_threshold` |
| `SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER` | **comportamento** | catálogo `engine.slow_inactive_timer` |

**3. Override por Node.** Tabela nova `node_settings` (migration `00044`): uma linha por `(node_id, key)`. O valor é validado pelo **mesmo `Spec`** do catálogo (tipo, faixa, opções) — não há segunda régua. Só chaves marcadas `PerNode` no catálogo aceitam override. **Remover o override devolve o Node ao valor de cluster**; não existe estado intermediário.

**4. Entrega e aplicação.** A resposta de `POST /api/v1/agent/heartbeat` ganha o bloco `agent_config` com os valores **efetivos daquele Node** (já resolvida a precedência 3 > 2) e uma `version` (hash dos valores). O Agent aplica **a quente**, sem reiniciar:

| O quê | Como o Agent aplica |
|---|---|
| `engine.*` (fila e rotação anti-stall) | `setPreferences` do Engine, só das chaves de fila — **não** mexe em porta BT nem em `save_path` (são da máquina) |
| `agent.heartbeat_interval` | recria o ticker do heartbeat |
| `agent.command_poll_interval` | recria o ticker do Poller de comandos |
| `agent.log_level` | `slog.LevelVar` |

**Nada de comportamento exige restart.** O que exige restart é o que continua no `.env` (URL do CP, credenciais, portas, caminhos) — e isso só muda de propósito, com intervenção no Node. O Agent **valida o que recebe** (tipo e sanidade: um intervalo ≤ 0 derrubaria um ticker) e rejeita o valor ruim mantendo o anterior; uma chave que ele não conhece (CP mais novo) é ignorada. Se o Engine estiver fora do ar na hora de aplicar, a aplicação é **repetida a cada heartbeat** até dar certo (antes, uma falha no boot nunca era retentada).

**5. O Agent reporta o que aplicou.** O heartbeat do Agent leva `config_version` — a mesma `version` que ele recebeu, **só depois de aplicar tudo com sucesso**. O Control Plane guarda em `nodes.agent_config_version` e o Portal compara com a versão efetiva: **em dia**, **aguardando o Agent** (mudou e ainda não chegou) ou **sem suporte** (Agent anterior a esta ADR, que nunca reportou). É o que torna visível a divergência que motivou a issue.

**6. Compatibilidade nos dois sentidos — o heartbeat decodifica com `DisallowUnknownFields`.** Um campo novo no **corpo** do heartbeat, mandado por um Agent novo a um Control Plane antigo, seria rejeitado com 400 e o Node ficaria `OFFLINE`. Por isso o Agent só envia campos novos **depois de o Control Plane provar que os entende**: `config_version` e `local_config` só entram no corpo quando a última resposta bem-sucedida trouxe `agent_config`, e qualquer falha de heartbeat zera esse estado (um rollback do Control Plane não deixa o Agent mandando campo que o CP desconhece). Resultado:

- **Agent velho + CP novo:** o Agent ignora `agent_config` e segue no env. Nada muda.
- **Agent novo + CP velho:** a resposta não traz `agent_config`; o Agent segue no env e **não manda** campo novo. Nada muda.
- **Rollback do Control Plane com Agents já em dia:** o Agent só descobre que o CP regrediu quando um heartbeat falha — o único que ainda leva `config_version` e toma `400` —, e na tentativa seguinte já não manda campo novo. Custa **um** heartbeat, não o Node (o limite de offline é de 90 s). O que já estava aplicado permanece.
- **Ordem de deploy** (ADR-027): Control Plane primeiro; Agents depois, um a um.

**7. Semeadura dos overrides — a migração dos Nodes que já existem.** Um Node em produção já tem valores no `.env` (hoje, node-04: 3 downloads; os demais: 10). Um Agent novo que passasse a obedecer só o cluster **mudaria o comportamento dele no primeiro heartbeat** — o oposto de "migração sem quebrar", e o erro de 05/09 repetido por um deploy. Decisão: o Control Plane **semeia** os overrides **uma vez por Node**, a partir do que o Agent relata, com a mesma cautela que a ADR-023 já aplica ao semear o catálogo a partir do env no primeiro boot (*"o banco manda; o env só semeia"*). Semeia-se a chave K do Node N **somente se todas** as condições valem:
   - é o **primeiro relato** de N (`nodes.agent_config_seeded_at` nulo; depois de semear — mesmo zero chaves — nunca mais);
   - o Agent relatou K porque o operador a **definiu explicitamente** no `.env` (um default do código não é decisão de ninguém — se fosse, todo Node novo nasceria preso nos defaults do código, o bug da issue);
   - **não existe override** de N para K (o operador que se antecipou e criou o override manda);
   - o valor de cluster de K **não foi definido explicitamente** pelo operador (ainda é o default do catálogo) — um valor de cluster explícito é decisão da frota, e o `.env` antigo de uma máquina não a contradiz;
   - o valor é **válido** pelo `Spec` e **difere** do valor de cluster (valor igual não vira override: seria ruído que impediria o Node de acompanhar o cluster depois).

   Enquanto o Control Plane ainda não semeou, ele manda `report_local: true` na resposta e o Agent **não aplica nada**: relata os valores explícitos do próprio env no heartbeat seguinte e só então aplica o que voltar. Isso fecha a janela em que o Node adotaria o valor de cluster antes de a semeadura protegê-lo. Cada semeadura gera um Evento `NODE_CONFIG_SEEDED` com as chaves e os valores. Desfazer = remover o override (e ele não reaparece: a semeadura é de uma vez só).

   Alternativas rejeitadas: *semear sempre, inclusive contra um valor de cluster explícito* — contradiria, em silêncio, uma decisão deliberada do operador; *não semear e exigir que o operador crie os overrides à mão antes de atualizar cada Agent* — é o procedimento **recomendado** (`docs/19 - Deployment.md`), mas depende de ninguém esquecer, e esquecer custa um Node sobrecarregado (a lição do node-04). A semeadura é a rede de segurança, não o caminho principal.

**8. RBAC e auditoria.** Valor de cluster: `settings:write` (rotas de `settings` da ADR-023, sem mudança). Override: `nodes:write` em `PUT`/`DELETE /api/v1/nodes/{id}/settings/{key}`; leitura com `nodes:read`. Toda mudança entra na trilha de **Auditoria** (ADR-025) com o antes e o depois, no padrão do restante: ação do operador → Auditoria; a semeadura, que é o sistema → Evento. Portal: botões gateados por `can()` (esconde o botão, não o dado — padrão do #216).

**Consequências**

Benefícios:
- O comportamento do Agent/Engine deixa de ser configuração espalhada: um valor de cluster, as exceções por Node visíveis e auditadas, e um selo de "em dia" por Node.
- Node novo **nasce igual ao cluster** sem o operador lembrar de nada.
- O Atom e o i5 convivem: o valor de cluster serve a maioria e o desvio do node-04 é um override explícito, visível no Portal, e não uma diferença oculta num arquivo.
- A mudança vale em até um heartbeat, sem SSH e sem restart.
- Reusa o catálogo (ADR-023), a validação (`Spec`), a auditoria (ADR-025) e o RBAC (ADR-035); nenhum canal ou componente novo. Respeita a ADR-012: o Agent puxa, o Control Plane nunca conecta no Node.

Limitações / custos:
- **Mudar o valor de cluster mexe em todos os Nodes sem override** no heartbeat seguinte. É o objetivo — e é por isso que a tela de Configurações lista os Nodes com valor próprio. Um valor de cluster ruim se propaga rápido; a rede de proteção é a validação por `Spec` e a faixa de cada chave.
- **Sem Control Plane no boot, vale o env.** Se o Node reinicia enquanto o Control Plane está fora (queda de energia que derruba tudo junto), ele aplica o valor do `.env` depois de algumas tentativas de heartbeat, e passa ao valor do CP quando este voltar. Por isso o `.env` deve continuar com um valor sensato (a semeadura o deixa igual ao efetivo nos casos que importam). Não há cache em disco do último valor do CP — foi considerado e deixado de fora para não criar uma terceira fonte de verdade.
- **O heartbeat passa a carregar config**: ~12 chaves por resposta (poucas centenas de bytes). Aceito.
- **A semeadura é de uma vez só**: um `.env` editado depois do primeiro relato **não** vira override (a precedência é override > cluster > env). Quem quer um valor próprio para o Node cria o override no Portal.
- **O override é lido a cada heartbeat** (uma consulta por Node por ciclo). Para as dezenas de Nodes que o produto visa, é irrelevante; se um dia não for, é um cache invalidado na escrita.
- Agents anteriores a esta ADR seguem **inteiramente no env** até serem atualizados (ADR-027); o Portal os marca "sem suporte".

**Relação com outras ADRs**
- **Estende a ADR-023**: o catálogo passa a ter valor **por Node** (`PerNode`) além do de cluster, e o consumidor de um tunável passa a ser também o Agent, não só os Workers. A categoria "Agent" do painel vira "Agent / Engine".
- **Respeita a ADR-012** (o CP decide, o Agent executa e puxa) e **não usa a fila da ADR-014**, pelo motivo acima (estado converge, evento se perde).
- **Depende da ordem de deploy da ADR-027**: Control Plane antes, Agents um a um.
- **Complementa a ADR-017**: o `.env` gerado no bootstrap fica só com identidade/máquina; o que era comportamento sai de lá.
- Auditoria (**ADR-025**) e RBAC (**ADR-035**) no padrão existente.

# **ADR-040**

**Título:** Host de imagem por tracker — descrição de forma fechada no perfil, chave do operador presa ao host, cadeia com fallback para o imgbb

**Estado:** Aprovada — implementada (#193)

**Contexto**
A ADR-030 fixou **um** host para os screenshots da publicação (imgbb, chave global no catálogo) e registrou o custo: se ele cair, os screenshots param. A ADR-037 decidiu que a política de host de imagem é **dado do perfil do tracker** (camada 2), e deixou a decisão de forma para a #193. Antes de desenhar, foi verificado o que existe de fato (04/10/2026):

- **O UNIT3D não bloqueia host de imagem nenhum.** Uma imagem de host fora da lista de permitidos do site é servida por um proxy de imagens, não recusada (`app/Helpers/Bbcode.php`, lista `whitelisted-image-urls`). O imgbb funciona em qualquer site da família. Host próprio é questão de **regra do site** (escrita pela staff, não aplicada pelo código) e de **resiliência** — não de compatibilidade.
- **Sites mantêm host próprio.** O samaritano roda um host de imagens num subdomínio, com contas próprias; o `autobrr/upbrr` carrega uploaders dedicados aos hosts de três trackers.
- **O contrato varia de host para host**, mesmo entre os que se dizem "compatíveis com ShareX":

| host | chave vai em | campo do arquivo | URL na resposta |
|---|---|---|---|
| host do samaritano | header `Authorization: Bearer <k>` | `file` | `url` (miniatura em `thumbnail_url`) |
| ShareX genérico (upbrr) | header `Authorization: <k>` | `file` (+ campo `title`) | `data.link` ou `link` |
| Chevereto (upbrr) | header `X-API-Key: <k>` | `source` | `image.url` |

- **O próprio host entrega a descrição.** O painel do host do samaritano oferece o download de um arquivo `.sxcu` — o formato declarativo de "uploader personalizado" do ShareX (método, URL, headers, campo do arquivo, onde está a URL na resposta).
- **A credencial é do host, não do tracker.** O token se cria na conta do host de imagens; não é o `api_token` do UNIT3D.
- **Dois defeitos latentes no caminho atual**, achados no levantamento: (i) o erro de rede do cliente imgbb é devolvido como `err.Error()` ao Portal e ao log, e o `*url.Error` do Go traz a URL inteira — com a chave na query (`?key=`); é o mesmo vazamento que a #220 fechou no cliente UNIT3D; (ii) o upload usa 60 s por frame, em série, dentro de um handler com `WriteTimeout` de 30 s, atrás de um proxy que dá 60 s ao `/api/`. Uma cadeia de hosts agravaria (ii).
- **O fluxo de screenshots não sabe para qual tracker é a publicação.** Nenhuma das rotas recebe o tracker; o painel de publicação tem o tracker em mãos e não o envia.

**Problema**
Definir (a) como um tracker declara o host do próprio site **sem código por host**; (b) como a credencial do operador chega a um host descrito por **conteúdo de terceiro** (o perfil é importável) sem que o perfil vire um canal de exfiltração da credencial; (c) como a publicação continua quando um host falha, **dentro do limite de tempo da requisição**; e (d) como a falha fica visível em vez de silenciosa.

**Alternativas consideradas**

1. **Manter o host único.** O imgbb funciona em todo UNIT3D, então nada quebra hoje. Rejeitada como solução completa: não atende site cuja regra exige o host próprio, e mantém o ponto único de falha que a ADR-030 já registrou. **O imgbb fica** — como padrão e como último elo.

2. **Um uploader em Go por host (modelo upbrr).** Rejeitada pelo mesmo motivo da alternativa 2 da ADR-037: só entra o host onde o mantenedor tem conta, e cada host novo exige release.

3. **O `.sxcu` inteiro como template no perfil** — método, URL, headers e campos arbitrários, corpo livre, expressão regular sobre a resposta. Expressivo, mas é exatamente o "campo livre indo direto para requisição" que a ADR-037 proíbe: um perfil importado montaria uma requisição qualquer, e uma regex vinda de terceiro é código com custo de execução imprevisível. Rejeitada.

4. **Prender o host de imagem ao domínio do tracker** (o host tem de ser o domínio do `api_url` ou um subdomínio dele). Sem estado e simples. Rejeitada como defesa principal: exclui host em domínio próprio (um dos três uploaders de tracker do upbrr fica em domínio separado do tracker), e "mesmo domínio registrável" exige a lista de sufixos públicos para não errar em `co.uk`.

5. **Descrição de forma fechada + chave presa ao host para o qual foi cadastrada + cadeia com orçamento de tempo (adotada).**

**Decisão adotada** (alternativa 5)

**1. Família em código, host em dado.** O código ganha **um** uploader genérico — multipart, um arquivo por requisição, chave num header — ao lado do cliente imgbb que já existe. O perfil ganha a seção opcional `image_host`:

```json
"image_host": {
  "name": "Host do site",
  "upload_url": "https://img.tracker.exemplo.org/api/v1/sharex/upload",
  "file_field": "file",
  "auth_header": "Authorization",
  "auth_prefix": "Bearer ",
  "url_path": "url",
  "thumbnail_path": "thumbnail_url"
}
```

A forma é **fechada**: sempre `POST multipart/form-data` com **uma** parte de arquivo (o frame JPEG, `image/jpeg`); a chave vai **só** no header `auth_header`, precedida de `auth_prefix`; nada de header extra, campo extra, query ou corpo livre; a resposta é JSON e a URL sai por **caminho com pontos** (`data.link`, `image.url`; segmento numérico indexa lista) — sem regex e sem template. Os três contratos da tabela cabem, exceto o campo extra `title` do ShareX genérico, que fica de fora até um host real exigi-lo. Host fora dessa forma (chave na query ou no corpo, resposta que não é JSON, upload em duas etapas) é **família nova**: PR com código, como a ADR-037 já prevê para família de tracker.

Nenhum campo da seção carrega a chave, e nenhum nome de campo a sugere (`auth_header` diz **onde** a chave vai, não qual é). O teste estrutural do export (ADR-037) já reprova campo com nome de credencial; a seção passa nele sem exceção.

**Validação** — em `ParseDocument` (import) **e** em todo ponto de escrita do serviço, como a seção `description`:
- `name`: a mesma regra dos nomes do perfil (`CleanName`). É rótulo: não vai a requisição, Feed nem auditoria.
- `upload_url`: até 2048 caracteres, absoluta, `https`, host como **nome DNS** (sem IP literal e sem `localhost`), sem porta explícita, sem usuário/senha, sem fragmento.
- `file_field`: `[A-Za-z0-9_.-]{1,64}`.
- `auth_header`: token de header (`[A-Za-z0-9-]{1,64}`), fora de `Host`, `Content-Type`, `Content-Length`, `Transfer-Encoding`, `Connection`, `Cookie`, `Upgrade`, `TE`, `Trailer` e `Proxy-*`, sem diferenciar maiúsculas.
- `auth_prefix`: 0 a 32 caracteres ASCII imprimíveis.
- `url_path`/`thumbnail_path`: 1 a 5 segmentos `[A-Za-z0-9_]{1,64}` separados por ponto; `thumbnail_path` é opcional (sem ele, a miniatura é a imagem cheia, como no imgbb).

**Edição, export e import** no padrão da seção `description` (ADR-037, #192): chave reservada `image_host` no JSONB do perfil, **sem migration** para a seção; rota própria `PUT`/`DELETE /api/v1/trackers/{id}/profile/image-host` (`trackers:write`, auditada); exportada sem tocar no export (o `Document` embute o `Profile`); importada só depois de entrar em `handledSections`, com a regra da seção-unidade — sem seção local → aplica; igual → nada; **diferente → protegida**, e só `?overwrite=image_host` a troca. **`schema_version` não sobe**: a seção é opcional e aditiva — a mesma exceção registrada para a `description`, com o mesmo custo (um SeedGrid anterior recusa o arquivo por "campo desconhecido").

**2. A chave do operador fica presa ao host para o qual foi cadastrada.** A chave é camada 3: colunas `trackers.image_host_key` e `trackers.image_host_key_host` (migration nova). Ela entra por rota própria — `PUT /api/v1/trackers/{id}/image-host-key` com `{"key": "…"}` e `DELETE` na mesma rota para apagar (`trackers:write`, auditadas; o texto da auditoria leva o host, nunca a chave) —, e não pelo `PUT` do tracker: a chave só faz sentido com uma seção `image_host` no perfil (sem ela, `409`), e uma rota própria dá à operação uma linha de auditoria e um erro que dizem o que aconteceu.

Ao gravar a chave, o Control Plane grava junto o **host do `upload_url` vigente** em `image_host_key_host`. Na hora de usar, o host do tracker só entra na cadeia se o host do `upload_url` atual for **exatamente** o gravado. Se o perfil mudou de host — por edição ou por import —, a chave **não é enviada**: o host é pulado com o motivo "chave cadastrada para outro host", e o operador cadastra de novo. A leitura do tracker devolve `has_image_host_key` e `image_host_key_host` (o host não é segredo), e o Portal mostra, no campo da chave, para onde ela vai ("enviada só para `img.tracker.exemplo.org`").

É o que responde ao item (b): a credencial vai **só para onde o operador a viu indo**. Um perfil importado pode trocar o `upload_url`, mas não leva a chave junto — casar por identidade (o host da hora do cadastro), não pela confiança no conteúdo do perfil. E, ao contrário da alternativa 4, serve para host em qualquer domínio.

**Redirecionamento não é seguido**: uma resposta `3xx` é falha. O Go descarta `Authorization` num redirecionamento para outro domínio, mas repassa header personalizado (`X-API-Key`) — seguir o redirecionamento levaria a chave para onde ninguém a cadastrou.

**3. Cadeia com fallback.** Ordem: **host do tracker** (seção presente, chave cadastrada e host conferido) → **imgbb** (chave global configurada). O primeiro host que hospedar **todos** os frames vence; os frames de uma publicação **nunca misturam hosts**, e o "tudo ou nada" por host é o mesmo da ADR-030 (melhor nenhuma descrição que uma pela metade). Um host que falha no frame *k* passa a vez, e o próximo recomeça do primeiro frame; as imagens que o host que falhou já tinha aceitado ficam **órfãs** lá — aceito: não há API comum de remoção, e são poucas.

- **Motivo da falha num conjunto fechado e sanitizado:** `chave cadastrada para outro host`, `HTTP <status>`, `redirecionamento recusado`, `tempo esgotado`, `falha de conexão`, `resposta sem URL`, `URL inválida na resposta`. **Nunca** o texto cru do erro: ele carrega a URL (no imgbb, a chave na query — o defeito (i), fechado aqui junto) e, numa mensagem do host, texto de terceiro.
- **Cada falha vira entrada no Feed** (ADR-022), um tipo novo (`screenshot_host_failed`) com o torrent, o tracker, o host (o **nome DNS** do `upload_url`, nunca o rótulo `name` do perfil, que é texto de terceiro), o motivo e o próximo host da fila. É o item (d): a falha do host do site deixa de ser invisível só porque o imgbb salvou a publicação.
- **Todos falharam** → o mesmo erro de hoje (`502 screenshots_failed`), agora com a lista de motivos por host; o operador continua podendo colar o BBCode à mão (ADR-026). **Nenhum host configurado** → o mesmo erro de hoje.
- **A resposta ganha `host`** (quem hospedou) **e `failures`** (`[{host, reason}]`), campos aditivos. O Portal mostra onde as imagens ficaram e o que falhou antes.
- **O tracker entra na requisição:** as rotas de screenshots que sobem imagens aceitam `tracker_id` opcional. Tracker de outro operador não existe para quem pergunta (`404`, como nas rotas de perfil). **Sem `tracker_id`, só o imgbb** — o comportamento atual, preservado para quem chama a API sem o campo. O painel de publicação passa a enviá-lo.
- **A URL devolvida por qualquer host é validada** antes de entrar no BBCode: absoluta, `https`, até 2048 caracteres, sem espaço, controle, `[`, `]` nem aspas. Ela é interpolada em `[url=…][img]…[/img][/url]`; sem a validação, a resposta de um host injetaria markup na descrição que o site publica. Vale para o imgbb também, que hoje é interpolado sem conferência.

**4. Orçamento de tempo.** A fase de upload tem **25 s no total**, abaixo dos 30 s de `WriteTimeout` do Control Plane e dos 60 s do proxy no `/api/`. Cada host recebe a **parte justa do que resta** — o restante dividido pelos hosts que ainda faltam —, e o tempo que um host não usa passa ao seguinte: com dois hosts, o do tracker tem até 12,5 s, e um host que trava não consome o tempo do fallback. O timeout de cada requisição é limitado pelo prazo do host. Foi considerado estender o prazo de escrita só nessas rotas (`http.ResponseController`) e abrir uma `location` própria no proxy; ficou de fora porque mexe em infraestrutura por uma fase que normalmente leva poucos segundos. Se host real se mostrar lento, o motivo `tempo esgotado` no Feed é o sinal para revisitar. A rota que extrai **e** sobe numa chamada só (`POST /torrents/{id}/screenshots`) mantém a limitação que já tinha (a extração pode sozinha passar do prazo); o Portal usa extração e upload em duas chamadas.

**Consequências**

Benefícios:
- Tracker com host próprio entra **sem código e sem release**, e quem tem conta no site contribui o host junto com o perfil (export/import, perfis do repositório) — o objetivo da ADR-037 estendido ao host de imagem.
- **Plano B real**: o imgbb cobre a falha do host do site, e vice-versa quando o imgbb cair num tracker com host próprio.
- Falha **visível** no Feed, com motivo legível.
- A credencial do host **não é exfiltrável por um perfil importado**, em qualquer domínio.
- Fecha o vazamento da chave do imgbb pelo texto do erro e a injeção de markup pela URL da resposta, que existiam antes desta ADR.
- A cadeia cabe no tempo que a requisição já tem; nenhuma mudança de infraestrutura.

Limitações / custos:
- **Só a forma fechada cabe.** Host que exige campo extra, chave fora de header, resposta não-JSON ou fluxo em duas etapas precisa de código.
- **Perfil que muda de host desliga a chave** até o operador cadastrá-la de novo. É o preço da defesa, e é deliberado.
- **Imagens órfãs** no host que falhou no meio da série.
- **A validação é do nome, não do endereço.** Um nome DNS que resolve para um IP interno passa (não há defesa contra DNS rebinding). Mitigações: sem IP literal, sem `localhost`, sem porta, e nada é enviado sem uma chave que o operador cadastrou **para aquele nome**.
- **25 s pode ser curto** para host lento com muitos frames; o sintoma aparece no Feed.
- **Sem conversor de `.sxcu`** nesta entrega: quem descreve o host transcreve seis campos do arquivo que o host entrega. Um conversor é a melhoria natural — e precisa separar, na entrada, o token que o `.sxcu` traz embutido nos headers (camada 3) da descrição (camada 2).
- **Validação ao vivo depende de uma conta no host do site.**

**Relação com outras ADRs**
- **Estende a ADR-030**: host único → cadeia; o imgbb vira o último elo, com contrato e chave global inalterados.
- **Implementa a "política de image host" da ADR-037** como seção opcional e aditiva do perfil — mesma forma de edição, export, import e merge da `description`, mesma exceção de `schema_version`, mesmo fail-closed de `handledSections`. A separação em camadas fica intacta: a descrição do host é camada 2; a chave, camada 3; o vínculo entre as duas é o host, gravado no cadastro.
- Falha no **Feed (ADR-022)**; rotas novas com **auditoria (ADR-025)** e **RBAC (ADR-035)** no padrão existente; segredo mascarado na leitura como manda a **ADR-029**.
- Preserva a **ADR-026**: o Agent segue extraindo os frames, o Control Plane hospeda e monta o BBCode, e o operador confere antes de publicar.

# **ADR-041**

**Título:** Internacionalização — o texto é montado onde é lido, a partir de código + dados; catálogo próprio no Portal, sem biblioteca

**Estado:** Aprovada — implementação em fatias (#31, #32, #33, #34, #57)

**Contexto**
O SeedGrid só existe em português, e passa a precisar de pelo menos português e inglês. O texto que o operador lê nasce hoje em três lugares, por três caminhos:

- **No Portal**, como literal: cerca de 900 trechos em 49 arquivos, sem nenhuma camada no meio.
- **Gravado pelo Control Plane**: alertas, eventos e atividades guardam uma frase pronta (`message`), em português, composta no instante da gravação. Ao lado dela já existem o **código** (`code`: `NODE_OFFLINE`, `NODE_DISK_FULL`, …) e os **dados** (`payload`). O Portal exibe `message || code`.
- **Composto pelo Control Plane para fora**: as notificações (Telegram/webhook), montadas por template a partir de código + payload — mas com rótulos em português fixos no código —, e as mensagens de erro da API (61 códigos, 268 pontos de emissão), em que o `error.code` é estável e o `error.message` é uma frase em português, muitas vezes com detalhe dinâmico.

Dois fatos pesam na decisão:

- **Frase gravada não se traduz.** O que está no banco foi escrito num idioma; quem lê pode querer outro, e dois operadores do mesmo cluster podem querer idiomas diferentes. Traduzir na gravação obrigaria a gravar em todos os idiomas, ou a escolher um para todos.
- **Frase gravada envelhece.** O projeto já pagou por isso: o endereço do Node escrito no texto do alerta ficava errado quando o endereço mudava, e a correção foi tirar o dado da frase e deixá-lo só no `payload`. O texto é uma *apresentação* dos dados; guardá-lo junto dos dados é guardar duas vezes, e uma das cópias fica velha.

Não há preferência de usuário no servidor: o Portal guarda o tema no navegador (`localStorage`).

**Decisão**

1. **O texto é montado onde é lido.** O contrato entre o Control Plane e quem exibe é **código + dados**. Vale para alertas, eventos, atividades, erros e ações da Auditoria. O Control Plane não traduz para o Portal.

2. **Portal: catálogo próprio, sem biblioteca.**
   - Um catálogo por idioma em `portal/src/i18n/` (`pt-BR.ts`, `en.ts`), como objetos TypeScript. O de `pt-BR` define o tipo; o de `en` tem de satisfazê-lo — **chave que falte ou sobre num idioma não compila**. Um teste confere também os parâmetros de cada frase.
   - `t(chave, parâmetros)` com interpolação nomeada e plural por `Intl.PluralRules`. Datas, números e tamanhos passam por `Intl` com o idioma ativo.
   - Valor com marcação própria dentro de uma frase (um hostname em fonte mono) fica como `{marcador}` no catálogo, e `rich(t(chave), { marcador: <span>…</span> })` põe o elemento no lugar dele. A frase é traduzida inteira, então o valor cai onde cada idioma o põe; a marcação nunca vira texto de catálogo.
   - Destaque num *trecho da própria frase* fica dentro da frase: `{strong:…}`, `{em:…}` e `{code:…}` ("Aqui você escolhe {strong:quais entram}"), desenhado pelo `rich`. O trecho é traduzido junto com o resto; separá-lo numa chave à parte esconderia o contexto de quem traduz.
   - **De onde vem o idioma:** a escolha do operador, guardada no navegador (`sg-lang`, como o tema) → o idioma do navegador, se for um dos suportados → **inglês**. Um seletor fica no menu; trocar não recarrega a página.
   - Chave nomeada pelo lugar e pelo papel (`nodes.add.title`), não pela frase.

3. **Alertas, eventos e atividades: `code` + `payload` são a fonte; `message` vira reserva.**
   - O Portal monta título e frase pelo catálogo, a partir do `code` e do `payload`.
   - O Control Plane **continua gravando `message`**, no idioma padrão do cluster: é o que lê quem consome a API sem catálogo e o que o Portal mostra para um código que ele ainda não conhece (Portal mais antigo que o Control Plane). Nenhum dado pode existir **só** no `message`.
   - **Teste de contrato:** a lista de códigos que o Control Plane emite e as chaves de `payload` que cada frase usa são conferidas contra os dois catálogos. Código novo sem entrada, ou frase que pede um dado que o `payload` não traz, reprova o CI.
     A lista mora em `control-plane/internal/messages` (`Registry`: trilha, código, as chaves de `payload` que toda emissão grava e — quando um código descreve situações diferentes — a chave do `payload` cujo valor escolhe a frase). Um teste lê o código-fonte do Control Plane e reprova código emitido sem entrada, entrada que nada emite, ou emissão cujo `payload` não traz uma chave prometida; o registro é exportado para `shared/messages.json`, contra o qual o Portal confere os catálogos. Motivos que eram texto em português (admissão, Scheduler, host de imagem) ganham um `reason_code` estável ao lado do texto antigo.
     Do lado do Portal, `lib/messages.ts` (`messageText`) monta a frase a partir de `alerts.messages.<trilha>.<código>` no catálogo — escolhendo a variante pela chave do `payload` — e cai no `message` gravado quando o código é desconhecido ou o `payload` de um registro antigo não traz um dado que a frase pede; `messages-contract.test.ts` confere os dois catálogos contra `shared/messages.json`.
     **Auditoria (#57):** a ação do operador segue o mesmo formato. Cada linha de `audit_log` grava `action_code` e `action_params` ao lado de `action`, que fica como a reserva em português. O handler nomeia a ação com `httpx.SetAudit(ctx, code, params, text)`; quando não nomeou (a requisição falhou antes), o middleware tira o código da rota (`route_*`, a tabela `routeActions`). Esses códigos são a trilha `audit` do `messages.Registry`, e as frases do Portal ficam em `alerts.messages.audit.<código>`. Um parâmetro que nomeia uma setting é traduzido pela chave dela, não pelo rótulo gravado.

4. **O que o Control Plane compõe para fora segue o idioma do cluster.**
   - **Notificações:** uma setting `notify.language` (`pt-BR` | `en`, padrão `en`) e um catálogo em Go com as mesmas chaves dos alertas/eventos. Notificação é mensagem para um canal, não para um leitor: o idioma é do cluster, não de cada operador.
     O catálogo é `control-plane/internal/notify/catalog.go`: um título curto por código (todo código de alerta, evento e atividade do `messages.Registry`, conferido por teste), os rótulos dos campos e as linhas da renderização; os modelos leem o idioma que o hook carimbou na mensagem. O motivo das atividades vem do `reason_code`; registro sem ele mostra o texto gravado.
   - **Erros da API:** `error.code` é o contrato. O Portal traduz os códigos que conhece; `error.message` segue existindo, no idioma do cluster, como reserva e como detalhe. Quando a frase precisar de um número ou nome, ele passa a vir em `error.params`, e não embutido no texto.
     Os códigos de erro fazem parte do `messages.Registry` (trilha `error`; o teste em AST lê o código de todo `WriteError`/`WriteErrorParams`). Só o `invalid_request` continua `generic`: ele cobre checagens de "campo X é obrigatório" que os formulários do Portal fazem antes de enviar, então o operador só o encontra chamando a API direto; o Portal mostra o `error.message` dele. Todo outro código tem frase própria (recurso que falta é `<recurso>_not_found`, corpo ilegível é `invalid_body`, falha interna aponta para o log do Control Plane). No Portal, o `api.ts` monta o `ApiError` com a frase de `api.errors.<código>`, de modo que todo `err.message` existente já sai traduzido; `{detail}` numa frase é o `error.message` do servidor (texto de erro de terceiro).
   - **Texto das settings:** o Control Plane continua mandando rótulo e descrição de cada setting (em português) para quem consome a API; o Portal mostra o seu, pela chave da setting (`settings.specs.<chave>`, um modelo com `{channel}` para as por canal, além dos rótulos das escolhas e dos nomes das categorias). O catálogo é exportado para `shared/settings.json` (`TestCatalogoExportado`) e `settings-contract.test.ts` reprova setting sem os dois textos nos dois idiomas. Avisos constantes que a API manda junto com uma resposta (o da importação de perfil) também são texto do próprio Portal.

5. **Idiomas da V1:** `pt-BR` e `en`. Acrescentar um terceiro é acrescentar um arquivo de catálogo no Portal e um no Control Plane.

**Consequências**

*Positivas*
- Dois operadores do mesmo cluster leem cada um no seu idioma, inclusive o histórico: um alerta de um ano atrás aparece em inglês para quem trocou o idioma hoje.
- O texto deixa de envelhecer por construção: a frase é sempre derivada dos dados atuais do registro.
- Tradução incompleta é erro de compilação ou de teste, não uma tela meio traduzida descoberta em produção.
- Nenhuma dependência nova no Portal, nenhum formato de arquivo novo, nenhuma etapa de build.

*Custos e limites aceitos*
- **Os registros antigos só ganham texto novo se o `payload` deles tiver os dados.** Onde não tiver, o Portal cai no `message` gravado, em português. Aceito: é histórico, e não se reescreve o passado.
- O catálogo próprio não tem o que as bibliotecas dão de graça: extração automática de chaves, formato ICU completo, carregamento sob demanda. Com dois idiomas e cerca de mil frases, o catálogo inteiro cabe no bundle; se o número de idiomas crescer, carregar sob demanda é uma mudança local ao `i18n/`.
- Há **dois catálogos** para os textos de alerta/evento (Portal e Control Plane), porque as notificações saem do Control Plane. O teste de contrato os mantém com as mesmas chaves; a frase pode divergir entre os dois, porque notificação e tela têm tamanhos diferentes.
- A extração é trabalhosa (49 arquivos) e vai em fatias. Enquanto durar, o Portal fica misto; um guarda no CI impede que uma tela já convertida volte a ter literal.

**Alternativas consideradas**
- **Biblioteca de i18n no Portal (i18next, react-intl, Lingui).** Resolve interpolação e plural, que `Intl` já resolve, e traz runtime, formato de catálogo próprio e, em duas delas, etapa de extração/compilação. A checagem de chaves pelo compilador — o que mais protege contra tradução incompleta — viria de configuração extra, não de graça. Rejeitada para dois idiomas.
- **O Control Plane traduz, pelo `Accept-Language`.** Mantém o Portal burro, mas o texto gravado continua num idioma só (o do instante da gravação) e continua envelhecendo; resolveria os erros da API e nada mais.
- **Gravar a frase em todos os idiomas.** Multiplica o que envelhece e exige migrar os registros a cada idioma novo.
- **Idioma por usuário, guardado no servidor.** Seguiria o operador entre navegadores, mas pede tabela, API e migração para um ganho pequeno; o navegador já resolve o tema assim. Fica para quando houver preferências de usuário por outro motivo.
- **Idioma padrão português.** O projeto nasce público e em inglês; quem instala e não escolhe nada vê inglês, e o navegador em português já leva ao português sem ninguém configurar.

**Implementação (fatias)**
1. Infraestrutura no Portal + telas de entrada (#31).
2. Demais telas, em lotes por tela, com o guarda no CI (#32).
3. Alertas, eventos e atividades montados no Portal + teste de contrato (#33).
4. `notify.language`, catálogo no Control Plane e `error.params` (#34).
5. Ações da Auditoria como código + dados (#57).

# **ADR-042**

**Título:** Control Plane instalado a partir de imagens publicadas e de um único arquivo Compose; o build no host fica como caminho de quem contribui

**Estado:** Aprovada — implementação em fatias (#30)

**Contexto**
Hoje o Control Plane é instalado clonando o repositório e rodando `scripts/bootstrap-controller.sh`. O script compila as imagens no próprio host (`seedgrid/controller:dev`, `seedgrid/portal:dev`, Deployment §42) e sobe três projetos Compose — `deploy/control-plane`, `deploy/observability` e `deploy/portal` — que só funcionam juntos porque o script os prepara antes:

- cria o volume externo `seedgrid-prom-targets` e o entrega ao usuário do controller;
- a rede `seedgrid-core` é criada por um projeto e declarada externa pelos outros dois;
- gera a senha do banco, o token de registro dos Agents, a senha do admin e o token do webhook interno, grava no `.env` e copia o token do webhook para um arquivo que o Alertmanager lê;
- exporta `VERSION` a partir do git, porque o Compose não roda git.

O node-exporter do host do Control Plane escuta em `172.18.0.1`, o gateway da `seedgrid-core`. O projeto de observabilidade fixa essa rede em `172.18.0.0/16`, mas a declara externa, e o Compose não aplica o plano de endereços de uma rede que ele não criou. O endereço só existe quando o Docker, por acaso, dá essa faixa à `seedgrid-core`: um host limpo dá, um host que já roda outras redes (um NAS com uma dúzia de containers) não dá.

Quem hospeda os próprios serviços em unRAID, ZimaOS, TrueNAS, Portainer ou numa VM simples espera outra coisa: um arquivo Compose que baixa imagens publicadas. O primeiro pedido sobre o SeedGrid num fórum da comunidade foi exatamente esse ("só de ter um compose file pra rodar já ficaria ótimo").

**Problema**
(a) Sem imagem publicada, toda instalação compila Go e o SPA no host, o que exige `git`, um clone, rede para baixar os módulos e minutos de CPU; num NAS nada disso é garantido. (b) Três projetos colados por um script não cabem numa interface de NAS que recebe um arquivo Compose. (c) Os segredos vêm do script, então sem ele o operador precisa inventar quatro valores, dois dos quais (senha do banco, token do webhook) ele nunca mais usa. (d) A imagem do Portal também é o servidor de download dos Nodes (ADR-017, ADR-027): uma imagem compilada em outro lugar continua tendo de levar os binários do Agent da mesma versão.

**Alternativas consideradas**
1. **Status quo, mais bem documentado.** Mantém o build no host. Não responde nem (a) nem (b). Rejeitada.
2. **Publicar imagens, manter os três projetos e o script.** Responde (a), não (b): o script continua colando tudo.
3. **Uma imagem tudo-em-um** (Postgres, controller, nginx, Prometheus e Alertmanager sob um supervisor). O formato de que os templates de NAS mais gostam. Rejeitada: esconde cinco processos atrás de um único estado de saúde, transforma atualizar o Postgres numa release do SeedGrid e quebra o modelo de um processo por container que o resto do deploy segue. Um template de NAS pode ser escrito em cima do arquivo Compose.
4. **Publicar no Docker Hub.** Limite de pull para usuário anônimo e uma segunda conta para administrar. O GHCR mora ao lado do repositório e do CI que compila as imagens.
5. **Imagens publicadas + um arquivo Compose (adotada).**

**Decisão**

1. **Imagens no GHCR, compiladas pelo CI a cada tag de release.**
   - `ghcr.io/rbferreira/seedgrid-controller` e `ghcr.io/rbferreira/seedgrid-portal`, multi-arquitetura (`linux/amd64`, `linux/arm64`).
   - Tags: a versão exata (`0.2.0`), a minor (`0.2`) e `latest`. Nada é publicado a partir de branch; uma tag de release é o único jeito de uma imagem passar a existir.
   - O Go faz cross-compile na plataforma do build (`GOARCH` vindo da plataforma de destino), sem emulação.
   - A versão vem da tag e é a mesma no controller, no Portal e nos binários do Agent que o Portal carrega.

2. **A imagem do Portal carrega os artefatos dos Nodes.** `/node/` serve os binários do Agent das duas arquiteturas, o `agents.json` e o `bootstrap-node.sh` exatamente como hoje (ADR-017, ADR-027), compilados na mesma execução do CI que o controller. Atualizar as imagens do Control Plane, portanto, também publica o Agent daquela versão para o self-update.

3. **Um arquivo Compose, `deploy/compose.yaml`, anexado a cada release do GitHub com as imagens fixadas naquela release.**
   - Um projeto (`seedgrid`): Postgres, controller, Portal, Prometheus, Alertmanager e o node-exporter do host.
   - A configuração do Prometheus e do Alertmanager vai embutida (`configs` do Compose com `content`), então o arquivo roda sozinho, sem um clone ao lado. Isso exige Docker Compose 2.23.1 ou mais novo.
   - A rede e os volumes pertencem ao projeto: nada para criar à mão. Os nomes dos volumes continuam os de hoje, então uma instalação existente pode passar para o arquivo novo e manter o banco e as métricas.
   - A porta do Portal é `SEEDGRID_HTTP_PORT` (padrão `80`): no unRAID e em vários sistemas de NAS a porta `80` é da interface web do próprio sistema.
   - As imagens dos outros serviços (Postgres, Prometheus, Alertmanager, node-exporter) ficam fixadas numa versão, não em `latest`.

4. **Segredos: o operador só escolhe o que ele digita.**
   - `SEEDGRID_ADMIN_PASSWORD` é o único valor obrigatório. É o que o operador digita no Portal.
   - A senha do banco, o token de registro dos Agents e o token do webhook interno são gerados na primeira subida por um serviço `init` de execução única (uma imagem pública que já existe, sem build), num volume próprio. O mesmo serviço entrega o volume de targets ao usuário do controller. Ele nunca sobrescreve um valor que já existe.
   - O Postgres lê a senha do arquivo (`POSTGRES_PASSWORD_FILE`). O controller ganha a forma `_FILE` das variáveis de segredo, a convenção do Docker: `SEEDGRID_X` vence quando definida, senão `SEEDGRID_X_FILE`. Uma instalação feita pelo `bootstrap-controller.sh` tem os valores no `.env` e não percebe mudança.

5. **Nenhum endereço fixado à mão.** O node-exporter do host do Control Plane sai da rede do host e entra na rede do projeto, lendo `/proc`, `/sys` e `/` do host em modo somente leitura, sem publicar porta. O endereço de escuta `172.18.0.1` e o plano de endereços fixo da rede deixam de existir.
   Custo aceito: os contadores de rede desse host passam a ser os do container. As métricas de rede que o Portal mostra são as dos Seed Nodes, que mantêm o node-exporter na rede do host.

6. **Atualizar é `docker compose pull && docker compose up -d`.** As migrations já rodam quando o controller sobe. Depois os Agents são atualizados pelo Portal, como antes (ADR-027).

7. **O build no host continua.** `bootstrap-controller.sh`, os scripts `build-*.sh` e o `Dockerfile.runtime` continuam funcionando para quem contribui, para quem roda um commit ainda sem release e para Docker dentro de container de sistema (LXC), onde `docker build` com `RUN` falha. Baixar uma imagem publicada também é uma saída para esse último caso: rodar uma imagem não esbarra na restrição do AppArmor em que o build esbarra.

**Fora do escopo**
- **O Agent em container.** Ele lê as métricas do disco do Node, mexe direto nos arquivos dos torrents e troca o próprio binário. Os motivos que a ADR-027 deu para mantê-lo como binário no systemd continuam valendo; um Agent em container é uma decisão própria.
- **Templates de NAS** (Community Applications do unRAID, ZimaOS, TrueNAS). Eles embrulham o arquivo Compose e as imagens, então vêm depois deles.
- **Adotar o qBittorrent que o Node já roda** (#32) e **outros Engines** (#31). Eles mudam o lado do Node; esta ADR só muda como o Control Plane é instalado.

**Consequências**

*Positivas*
- Instalar o Control Plane exige Docker e um arquivo. Sem clone, sem Go, sem Node.js, sem tempo de build na CPU de um NAS.
- As imagens são as mesmas em todo lugar: um relato de bug cita uma versão, e essa versão é um único conjunto de bytes.
- Um Control Plane `arm64` (um Raspberry Pi 4/5, um NAS ARM) passa a ser possível sem compilar nele.
- Some a dependência de `172.18.0.1`, que falha em qualquer host cujas redes do Docker não começaram nessa faixa.

*Custos e limites aceitos*
- **A release depende do CI e do GHCR.** Um workflow de release quebrado significa nenhuma imagem para aquela tag; o build no host continua lá.
- **O volume de segredos passa a fazer parte do backup.** Se ele se perder e o volume do Postgres não, o controller não consegue mais abrir o próprio banco. O guia de implantação diz isso junto das instruções de backup.
- **Compose 2.23.1 ou mais novo** para a configuração embutida. Num Compose mais antigo, o build no host continua funcionando.
- **Dois caminhos de instalação para manter funcionando.** Um job do CI sobe a pilha a partir do arquivo Compose e confere o `/healthz`, para o caminho publicado não se afastar em silêncio do que os desenvolvedores usam.
- Uma instalação que sai do `bootstrap-controller.sh` para o arquivo Compose precisa antes de `docker compose down` nos três projetos antigos: os nomes dos containers são os mesmos, e o Docker não cria um segundo container com um nome já em uso.

**Relação com outras ADRs**
- A **ADR-015** escolheu Docker e Compose para o empacotamento; esta ADR decide de onde vêm as imagens.
- **ADR-017** e **ADR-027**: a imagem do Portal continua servindo o onboarding e o self-update, agora compilada pelo CI.
- A **ADR-029** levou toda credencial de terceiro para o Portal; o que sobra no `.env` é a infraestrutura do próprio deploy, e esta ADR reduz o que o operador preenche à senha do admin.

**Implementação (fatias)**
1. Esta ADR.
2. Workflow de release: imagens multi-arquitetura no GHCR a cada tag, com os artefatos dos Nodes na imagem do Portal.
3. `deploy/compose.yaml` com o serviço `init`, as variáveis `_FILE` do controller, o node-exporter sem endereço fixo e o job do CI que sobe a pilha.
4. Guia de implantação e README: o arquivo Compose vira o Quick start; o build no host vai para uma seção de quem contribui.

# **ADR-043**

**Título:** Um Seed Node entra com o qBittorrent que já roda — nada muda na entrada e, uma vez no Cluster, ele é um Node como qualquer outro

**Status:** Aprovada — revisada antes do lançamento (#64). O primeiro desenho (#32) deixava o Engine com o operador para sempre; esta versão o substitui.

**Contexto**
O onboarding (ADR-017) pressupõe uma máquina vazia. O `bootstrap-node.sh` instala o Docker, monta um disco em `/data`, inicia **seu próprio** qBittorrent no container `seedgrid-engine` com uma senha gerada por ele, altera a rede (ADR-034, ADR-038) e instala o Agent. No primeiro contato, o Agent define as preferências do Engine: a porta BitTorrent que o Control Plane atribuiu no enroll, `save_path=/data`, sem UPnP, sem porta aleatória e a fila de download (ADR-039).

Quem testa o SeedGrid geralmente já roda qBittorrent, muitas vezes há anos, com milhares de torrents, port forward, regra de firewall, estrutura de pastas lida pelo servidor de mídia e configurações próprias de fila. Nessas máquinas, o bootstrap instala um segundo qBittorrent que disputa a porta da WebUI com o primeiro. Um Agent apontado manualmente para o existente reescreve sua porta e pasta de download. O usuário do fórum que pediu rTorrent (#31) pediu isso antes: coordenar os qBittorrents que já possuem.

A primeira versão desta ADR tratava esse Node como um segundo tipo de Node: o SeedGrid nunca escrevia suas preferências nem o reiniciava, e o Portal o marcava e riscava suas configurações. Isso não é entrar. A máquina entra para ser orquestrada; o que não pode acontecer é a própria entrada quebrar o que o operador montou.

**Problema**
(a) O bootstrap não tem caminho que preserve um Engine existente. (b) No primeiro contato, o Agent sobrescreve a porta, a pasta de download e a fila do Engine com os valores do Cluster. (c) O Agent lê dados dos torrents diretamente (migração, capturas de tela, MediaInfo, saúde do armazenamento); quando o Engine roda em container, os caminhos reportados (`/downloads/...`) diferem dos vistos pelo host (`/mnt/user/torrents/...`). (d) Na primeira passagem de admissão, um Engine com dezenas de downloads ativos teria a maioria pausada pelo Control Plane para respeitar o limite do tracker (ADR-021). (e) Reiniciar o Engine pelo Portal depende da política de reinício do container; num Engine que o SeedGrid não instalou, nada garante que ele volte. (f) A rede e os discos da máquina já estão configurados e funcionam; alterá-los quebra o que o operador montou.

**Alternativas consideradas**
1. **Apenas documentação** (apontar o Agent para o Engine existente manualmente). Mantém (b) a (f) como estão. Rejeitada.
2. **Importação:** instalar o Engine do SeedGrid e mover os torrents existentes para ele. Assume o controle da máquina, dobra o trabalho de disco e o operador perde a configuração que queria manter. Rejeitada.
3. **Um tipo de Node adotado** (a primeira versão desta ADR): o Agent orquestra os torrents, mas nunca grava as preferências do Engine nem o reinicia. Dois tipos de Node para manter funcionando, configurações que silenciosamente não se aplicam em alguns Nodes e um Node que nunca é de fato do Cluster. Rejeitada.
4. **Um caminho de entrada, não um tipo de Node.** O bootstrap lê o que o Engine já tem em vez de escolher; o que foi lido vira a configuração do Node no SeedGrid, então aplicá-la não muda nada; o Node começa em Manual para o operador revisar; depois disso, é gerenciado como qualquer outro Node.

**Decisão** (alternativa 4)

1. **Modo bootstrap para um Engine existente: `ENGINE=existing`.**
   - Recebe o endereço e as credenciais da WebUI (`QBT_URL`, padrão `http://127.0.0.1:8080`; `QBT_USERNAME`, `QBT_PASSWORD`). Valida esses dados, a versão do Engine e o reinício (item 7) **antes** de falar com o Control Plane: nenhum Node faz enroll se o Agent não conseguir usar o Engine, e uma recusa não altera nada.
   - Lê do Engine sua porta de escuta (`listen_port`) e sua pasta padrão de download (`save_path`).
   - Não instala o Engine, não altera sua senha, não monta discos nem instala a recuperação de hot-plug, e não mexe na rede (extensões de privacidade e sufixo IPv6). O Docker não é instalado. A opção `--network-only` continua disponível para quem quiser aplicar a ADR-038 nessa máquina.
   - Exportador de métricas do Node: se algo já responder na porta 9100, ele é usado; caso contrário, se houver Docker, apenas o exportador é iniciado; sem Docker, o bootstrap avisa e o Node entra sem métricas de host.
   - Engine suportado: qBittorrent **5.0 ou superior**. A 5.0 é a primeira com o criador de torrents, necessário para publicação (ADR-019); um Node que não publica seria novamente um segundo tipo de Node. A versão 4.x é recusada com o motivo e a recomendação de atualização.

2. **A porta é a que o Engine já possui.** O enroll aceita a porta lida pelo bootstrap e reserva **essa** porta para o Node, em vez da próxima livre na sequência; o Agent então a fixa, como em todo Node, para que o port forward do operador continue funcionando. Uma porta fora da sequência é aceita; o Node fica sem índice e sem sufixo IPv6 determinístico (ADR-038), que o bootstrap não aplica de qualquer forma. Uma porta já usada por outro Node é aceita com um aviso citando esse Node: dois Nodes atrás do mesmo IPv4 não podem compartilhar um port forward, mas dois em redes diferentes podem, e o Control Plane não consegue distinguir.

3. **A pasta de download é a que o Engine já possui.** O armazenamento do Node é a pasta do host definida no `save_path` do Engine, e o Agent fixa o `save_path` nela, assim como fixa o `/data` nos Nodes que instala. A verificação de saúde do armazenamento confirma se essa pasta tem permissão de escrita; não há exigência de disco dedicado (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false`), pois o disco não foi montado pelo SeedGrid.

4. **A fila do Engine vira as configurações do Node.** No primeiro contato, o Agent reporta as configurações atuais da fila do Engine (`engine.*`, ADR-039) em vez das variáveis de ambiente, e o Control Plane as semeia como overrides do Node (ADR-039 §7). Há duas diferenças em relação ao seeding de um Node comum, ambas porque o valor vem de um Engine em execução e não de um arquivo escrito por alguém: um valor do Cluster definido explicitamente pelo operador não prevalece sobre ele (caso contrário, o join alteraria o Engine), e um valor fora da faixa do catálogo é ajustado para caber nela em vez de ser ignorado (o "ilimitado" do qBittorrent, `-1`, torna-se o máximo do catálogo). O Agent segura a primeira aplicação até que o Engine tenha sido lido, assim como já a segura até o seeding. Os valores efetivos são então os que o Engine já tinha, então a primeira aplicação não muda nada; o operador edita ou limpa esses valores no Node como qualquer outro override. O Control Plane registra que o Node entrou dessa forma (`nodes.engine_adopted`), o que distingue o seeding e o modo inicial; nada mais lê esse campo.

5. **Uma coisa muda, e o bootstrap avisa antes do enroll: UPnP e porta aleatória são desligados,** como em todo Node — o Cluster depende de uma porta fixa com port forward manual. Se o Engine estava com UPnP ligado, o bootstrap alerta que o mapeamento criado pelo roteador via UPnP será perdido e a porta precisará ser encaminhada manualmente; os alertas de conectividade (ADR-034) também avisam se isso não for feito.

6. **Um Node adotado entra em Manual.** Seus torrents aparecem no Cluster exatamente como estão (o inventário já adota os hashes desconhecidos pelo Control Plane), e o controlador de admissão não pausa, inicia nem rotaciona nenhum deles até que o operador mova o Node para SG (ADR-024). O modo Manual só existe sob o gate Flex: se o Cluster estiver em Full auto, o registro do primeiro Node adotado muda o gate para Flex, gerando um evento explicativo. Os demais Nodes permanecem em SG, mantendo o comportamento anterior do Cluster. O modo Manual é uma etapa de revisão, não um estado definitivo: após o operador mover o Node para SG, ele passa a ser orquestrado como qualquer Node.

7. **O reinício funciona como em todo Node, e o bootstrap garante que seja possível.** A opção "reiniciar Engine" do Portal pede ao qBittorrent que encerre e depende de algo para trazê-lo de volta. O bootstrap identifica o que executa o Engine e garante seu reinício: um container recebe a política de reinício `unless-stopped` se sua política atual não o reiniciasse (e o bootstrap lembra que um arquivo Compose precisa da mesma linha, ou o próximo `docker compose up` desfaz a mudança); um serviço de sistema recebe um drop-in com `Restart=always`. Se não encontrar nenhum dos dois — qBittorrent iniciado manualmente, numa sessão desktop ou como serviço de usuário — o bootstrap recusa, explica como executá-lo e não altera nada.

8. **Mapeamento de caminhos entre o Engine e o host.** Quando o Engine roda em container, o bootstrap lê os bind mounts e grava o mapeamento (`SEEDGRID_ENGINE_PATH_MAP`, pares `caminho no Engine=caminho no host`); `ENGINE_PATH_MAP` sobrepõe essa configuração. O Agent traduz todo caminho reportado pelo Engine antes de ler dados (origem de migração, capturas de tela, MediaInfo) e todo caminho passado ao Engine (`save_path`, destino de migração, origem de publicação). Dados para migração podem vir do armazenamento do Node ou de qualquer pasta mapeada do host; um torrent fora dessas áreas é recusado com essa justificativa, como hoje. Nos Nodes instalados pelo SeedGrid, o mapa está vazio e cada caminho é igual em ambos os lados.

9. **Dados escritos pelo Agent pertencem ao dono do destino.** O Agent roda como root; um Engine em container geralmente não. Arquivos extraídos pelo Agent (migração, upload de publicação) recebem o dono da pasta onde são gravados, permitindo que o Engine continue escrevendo em um torrent parcialmente migrado. Nos Nodes instalados pelo SeedGrid, tudo é do root e nada muda.

10. **Nada no Portal distingue esse Node.** Sem selo, sem botão oculto, sem configuração exibida como não aplicada: a pasta e a porta são do Node, assim como cada Node tem as suas.

**Fora do escopo**
- **Outros Engines** (#31). Esta ADR mantém a interface do Engine igual à do qBittorrent; o rTorrent vai reusar esse modo de entrada.
- **Várias pastas de download como armazenamento.** O espaço livre e a saúde do disco são lidos apenas do armazenamento do Node. Um torrent mantido em outro disco continua sendo semeado e migrado, mas esse disco não é medido.
- **Outras preferências do Engine** (limites de velocidade, limites de conexão, categorias). O SeedGrid não as gerencia em nenhum Node e não passa a fazê-lo aqui.

**Consequências**

*Positivas*
- O caso mais comum — uma máquina que já roda o qBittorrent — entra no Cluster sem perder o port forward, as pastas ou as configurações de fila. Além disso, a primeira passagem de admissão não pausa os downloads.
- Continua havendo apenas um tipo de Node: um caminho para o Agent, um Portal e configurações que se aplicam a todos.
- O Agent aprende a trabalhar com caminhos que não controla. Isso será necessário de qualquer forma para um Engine em container (e para #31).

*Custos e limites aceitos*
- O UPnP é desativado na entrada; um operador que dependia dele precisará fazer o port forward manualmente.
- O qBittorrent 4.x não é suportado: o operador deve atualizar antes de entrar.
- O bootstrap altera a política de reinício do container ou serviço do operador. É a menor mudança necessária para que o reinício pelo Portal funcione, e o bootstrap avisa antes de fazer isso.
- Sem o sufixo determinístico, o endereço IPv6 desse Node é o que a máquina já tinha. Os alertas de conectividade (ADR-034) continuam lendo o endereço que ele reporta.

**Relação com outras ADRs**
- **ADR-017**: um segundo caminho de onboarding; o join token e o registro não mudam.
- **ADR-024**: o Node inicia em Manual e o gate passa para Flex para ele.
- **ADR-034** e **ADR-038**: não aplicadas pelo bootstrap nesse tipo de Node; a opção `--network-only` permanece.
- **ADR-039**: a semeadura lê o Engine em vez do ambiente, com as duas diferenças do item 4; todo o resto permanece igual.

**Implementação (fatias)**
1. Esta revisão.
2. Control Plane: a semeadura de um Node que entrou com seu próprio Engine (item 4).
3. Agent: a fila lida do Engine para a semeadura, `save_path` via mapeamento de caminhos, reinício permitido e fim das exceções para esse tipo de Node; remoção do `pause`/`resume` do qBittorrent 4.x.
4. `bootstrap-node.sh ENGINE=existing`: versão 5.0 ou superior, o reinício (item 7) e o aviso sobre UPnP.
5. Portal: remoção do selo, do reinício oculto e das configurações riscadas. Guias e um teste na bancada com um qBittorrent 5 existente em container.

# **ADR-044**

**Título:** Um Node removido que entra de novo volta como ele mesmo se for a mesma máquina, e como um novo Node se não for

**Status:** Aprovada (#62)

**Contexto**
Remover um Node (soft delete) revoga a credencial do seu Agent, devolve sua porta BitTorrent ao pool e, com `force`, remove seus torrents do Cluster. A linha fica no banco para histórico. O registro faz upsert por `(cluster_id, hostname)`, uma restrição que também cobre linhas removidas e nunca limpa `deleted_at`: uma máquina removida que entra de novo com o mesmo hostname cai na linha removida e continua invisível — seu Agent ganha uma credencial e seus heartbeats são aceitos, mas nenhuma lista o mostra. Renomear a máquina era o único caminho de volta.

Uma máquina com o hostname de um Node removido pode ser esse Node voltando (foi removido por engano ou para mudar de rede) ou uma máquina diferente que recebeu o mesmo nome (uma substituição ou uma instalação nova em hardware novo). Os dois querem resultados opostos: o primeiro quer de volta seu histórico, seus torrents, seu modo, seus overrides e sua porta; o segundo não quer nada disso.

**Problema**
(a) Distinguir a mesma máquina de outra com o mesmo hostname, sem perguntar ao operador algo que a própria máquina sabe melhor. (b) Uma remoção tem que valer: um Agent que ainda roda num Node removido tenta se registrar sozinho, e isso não pode desfazer a remoção. (c) Uma máquina diferente não deve herdar nada da linha removida.

**Alternativas consideradas**
1. **Reativar a linha removida sempre que o hostname bater.** Uma máquina substituta herda torrents que não tem. Rejeitada.
2. **Sempre criar um Node novo** (liberar o hostname na remoção). A mesma máquina que volta perde seu histórico e seus overrides, e ganha uma porta nova que seu port forward não aponta. Rejeitada.
3. **Perguntar ao operador** na hora do join. O bootstrap roda sem supervisão (`curl | sh`), e o operador responderia com o que a máquina já pode dizer sozinha. Rejeitada.
4. **Identificar a máquina.** O Agent manda o que identifica a máquina: `/etc/machine-id`, que toda instalação systemd tem e mantém entre reboots e reinstalações do SeedGrid, e o id do Node do seu arquivo de identidade, quando ainda existe. Mesma máquina → o Node volta; qualquer outra coisa → Node novo.

**Decisão** (alternativa 4)

1. **O Agent identifica a máquina no registro**: `machine_id` (de `/etc/machine-id` ou `/var/lib/dbus/machine-id`) e `node_id` (o id no arquivo de identidade, quando existe). O Control Plane guarda o `machine_id` de todo Node (`nodes.machine_id`). O bootstrap também manda o `machine_id` e o hostname no enroll.

2. **O hostname é único apenas entre Nodes ativos.** A restrição vira um índice único parcial (`WHERE deleted_at IS NULL`); linhas removidas mantêm o hostname para histórico.

3. **Mesma máquina** — uma linha removida com o mesmo hostname cujo `machine_id` é igual ao enviado, ou cujo id é igual ao `node_id` enviado:
   - **por join token** (o operador rodou o bootstrap de novo): o Node volta como era — mesmo id, histórico, modo, overrides — e os torrents removidos junto dele (aqueles com `deleted_at` igual ao do Node) são restaurados. O enroll devolve a porta BitTorrent antiga se nenhum Node ativo a ocupou nesse meio-tempo, então o port forward continua apontando para ela. Evento `NODE_RETURNED`.
   - **pelo token de registro compartilhado** (um Agent tentando sozinho depois que a remoção revogou sua credencial): recusado com `410 node_removed`. A remoção vale; o Agent registra o motivo, diz como trazer o Node de volta e tenta de novo mais devagar.

4. **Máquina diferente** — nenhuma linha removida com esse hostname bate — se registra como Node novo: linha nova, nada herdado. Uma linha removida legada, de antes de o `machine_id` ser guardado, só bate por `node_id`; sem ele, a máquina entra como nova, o que perde o histórico, mas nunca mistura duas máquinas.

**Fora do escopo**
- **Duas máquinas ativas com o mesmo hostname.** O registro continua atualizando a linha ativa, como hoje.
- **Sistema operacional reinstalado** ganha um `machine-id` novo e nenhum arquivo de identidade: entra como Node novo. O disco foi apagado; o que o Cluster sabia sobre ele virou histórico.

**Consequências**

*Positivas*
- Remover um Node por engano se resolve rodando o bootstrap de novo, com o port forward ainda válido.
- Uma máquina substituta pode usar o nome da que substituiu.
- Uma remoção não pode mais ser desfeita por um Agent que ninguém parou.

*Custos e limites aceitos*
- Máquinas virtuais ou containers clonados que compartilham `machine-id` e hostname parecem a mesma máquina. Clonar os dois já é erro de configuração no systemd.

**Relação com outras ADRs**
- **ADR-017**: o join token é o que distingue a volta do operador de uma tentativa automática do Agent.
- **ADR-043**: não afetada; um Node que entrou com Engine próprio volta do mesmo jeito.

**Implementação (fatias)**
1. Esta ADR.
2. Control Plane: migração (`machine_id`, índice único parcial), registro e enroll, `NODE_RETURNED`, `410 node_removed`.
3. Agent: `machine_id` e `node_id` no registro, tratamento do `410`; bootstrap: `machine_id` e hostname no enroll. Guias.

# **ADR-045**

**Título:** rTorrent como Torrent Engine — mais um adaptador, o mesmo Node

**Status:** Aprovada — implementação em fatias (#31)

**Contexto**
O Agent tem um único adaptador de Engine, o do qBittorrent. Muitos operadores fazem seed com rTorrent, geralmente pela interface web ruTorrent. A maioria dos provedores de seedbox já o inclui, muitas vezes em máquinas compartilhadas onde o usuário não tem root. O usuário do fórum que pediu a #31 coordena instâncias locais de qBittorrent e seedboxes remotas com ruTorrent.

A §9 de `Engine.md` diz que trocar o Engine não pode alterar o Control Plane, o Scheduler, o Portal ou a API. A ADR-043 (revisada) adiciona o princípio definido pelo operador na #32: uma vez no Cluster, um Node é igual a qualquer outro. Se um Node não tiver uma capacidade (publicação, reinício), ele não é suportado, em vez de ser suportado com limitações.

**Problema**
(a) O Agent foi escrito diretamente contra o adaptador do qBittorrent: o tipo é usado em `main` e strings de estado vazam para o loop de sincronização. (b) O rTorrent usa XML-RPC, via SCGI (socket Unix ou porta TCP) ou servidor web, e não uma API JSON WebUI. (c) O rTorrent não tem um campo único de estado. (d) Funcionalidades que o SeedGrid usa no qBittorrent e faltam no rTorrent: fila de download (`engine.*`, ADR-039), criador de torrents (publicação, ADR-019), exportação de `.torrent` e veredito de conectividade (ADR-034). (e) Em seedboxes compartilhadas, o Agent roda sem root, units de sistema do systemd ou Docker, e o ciclo de vida do rTorrent pode ser gerenciado por outro processo.

**Alternativas consideradas**
1. **Usar os endpoints HTTP do ruTorrent.** São backend de UI: não documentados, dependentes de plugins e inexistentes quando o rTorrent roda sem ruTorrent. Rejeitada.
2. **Suportar o rTorrent como um Node reduzido** (sem fila, sem publicação, sem reinício), com o Portal indicando quais configurações não se aplicam. Esse é exatamente o tipo de Node que a ADR-043 rejeitou. Rejeitada.
3. **Envolver o rTorrent num shim compatível com qBittorrent.** Mais um daemon para instalar e manter alinhado com duas APIs. Rejeitada.
4. **Um adaptador de rTorrent atrás de uma interface Engine, com o Agent fornecendo o que falta.** A fila e o criador de torrents são implementados no Agent, uma única vez, para qualquer Engine que não os tenha.

**Decisão** (alternativa 4)

1. **Uma interface de Engine no Agent.** O código do Agent depende de `engine.Engine`, não de um adaptador concreto: torrents com o estado SeedGrid já mapeado (Database.md §36), adicionar/parar/iniciar/reverificar/excluir, exportar, caminho do conteúdo, trackers, versão, disponibilidade, conectividade, preferências (porta, pasta, fila) e desligamento. O adaptador qBittorrent passa para trás dela sem mudanças. `SEEDGRID_AGENT_ENGINE` (`qbittorrent`, o padrão, ou `rtorrent`) escolhe o adaptador.

2. **Protocolo: XML-RPC, local.** `SEEDGRID_ENGINE_RTORRENT_URL` é `scgi:///path/to/rpc.socket`, `scgi://127.0.0.1:5000` ou `http(s)://…/RPC2` (com basic auth) para setups que só expõem via servidor web. O codec XML-RPC é pequeno e está no Agent: sem dependência externa. Suportado: rTorrent **0.9.8 ou superior** (`d.multicall2`, `system.client_version`).

3. **Mapeamento de estado.** `d.hashing` ≠ 0 → CHECKING; `d.message` definido por algo que não seja um tracker (erro de armazenamento, verificação de hash com falha) → ERROR; a resposta de um tracker (`Tracker: [...]`) é o veredito do tracker (#160), não o estado do torrent, como no qBittorrent; parado (`d.state` 0), ou pausado (iniciado e inativo), e completo → PAUSED; parado, incompleto e marcado como enfileirado pelo Agent → QUEUED; parado, completo e marcado como enfileirado → SEEDING (esperando vaga de upload, como o `queuedUP` do qBittorrent); parado e incompleto nos demais casos → PAUSED; iniciado e completo → SEEDING; iniciado e incompleto → DOWNLOADING. As contagens do swarm vêm do scrape dos trackers (`t.scrape_complete`/`t.scrape_incomplete`) e as de conectados, de `d.peers_complete`/`d.peers_connected`; o swarm nunca é contado abaixo do que está conectado.

4. **A fila é do Agent quando a Engine não tem.** O rTorrent inicia todo torrent que possui. Para uma Engine sem fila, o Agent aplica as configurações `engine.*` num loop curto: mantém no máximo `max_active_downloads` downloads ativos (e os limites de upload e total), para os demais e marca como enfileirados (chave `d.custom` `seedgrid`), inicia o mais antigo quando vaga abre e, com `dont_count_slow`, ignora downloads abaixo dos limites lentos por tempo maior que o timer. Só inicia torrents que ele mesmo enfileirou: torrent parado pelo operador ou pelo admission controller (ADR-021) continua parado. Com a fila ligada, um torrent adicionado a partir de um `.torrent` entra parado e enfileirado, para que adicionar muitos de uma vez não inicie todos; um magnet inicia (o rTorrent só busca os metadados enquanto ele roda) e a fila o segura em seguida; um torrent migrado inicia, para que o rTorrent verifique os dados. A fila só age depois que as configurações do Control Plane chegaram: os padrões do próprio Agent não mudam nada num rTorrent que acabou de entrar. A verificação é serial no rTorrent; `max_active_checking` não tem efeito ali. As configurações, seus significados e o Portal permanecem iguais para todo Node.

5. **O criador de torrents é do Agent.** O rTorrent não cria `.torrent`. O Agent constrói (BitTorrent v1, tamanho de peça escolhido pelo tamanho total como no qBittorrent, flag private e `source` conforme solicitado), e o adaptador carrega apontando para os dados, para a Engine verificar e semear — mesmo resultado da publicação atual (ADR-019). O adaptador qBittorrent continua usando o criador do qBittorrent. O rTorrent responde 0 a um carregamento mesmo quando descarta o `.torrent` (o 0.9.8 descarta um sem tracker), então o adaptador confirma que cada `.torrent` que carrega está lá e informa quando não está.

6. **Exportação.** O rTorrent guarda o `.torrent` de cada torrent na pasta de sessão. O adaptador lê e remove as chaves adicionadas pelo rTorrent (`rtorrent`, `libtorrent_resume`), deixando os bytes originais que o Control Plane armazena (ADR-018).

7. **Conectividade.** O rTorrent não tem veredito "firewalled". O adaptador não reporta nada (o Control Plane lê como desconhecido, ADR-034) e informa peers de entrada pela lista de peers (`p.is_incoming`), base do alerta de silêncio de entrada.

8. **Como entra: o caminho da ADR-043.** O SeedGrid não instala rTorrent; uma máquina com ele entra com `ENGINE=rtorrent` pelo mesmo fluxo de `ENGINE=existing`: nada muda na entrada, porta e pasta são lidas do rTorrent (`network.port_range`, `directory.default`), e o Node inicia em Manual. As configurações de fila começam desligadas (`engine.queueing_enabled=false`), pois o rTorrent rodava sem fila; o operador ativa a fila do Agent ao revisar o Node. Não há UPnP no rTorrent; nada a alertar.

9. **Reinício.** O reinício pelo Portal chama `system.shutdown.normal` e confia em algo para trazer o rTorrent de volta, como no qBittorrent: política de reinício de container, unit systemd (do sistema ou do usuário com lingering) ou — numa seedbox cujo provedor gerencia o rTorrent — um comando dado na instalação (`ENGINE_RESTART_CMD`, ex.: `app-rtorrent restart` do provedor), executado pelo Agent no lugar. Sem nenhum desses, o bootstrap recusa, como faz para qBittorrent.

10. **Um Node sem root.** Rodar `bootstrap-node.sh` como usuário comum instala o Agent para esse usuário: binário em `~/.local/bin`, ambiente em `~/.config/seedgrid-agent.env`, identidade em `~/.local/share/seedgrid-agent`, iniciado por unit systemd de usuário com lingering quando disponível, senão por crontab que inicia no boot e a cada minuto se não estiver rodando. Sem supervisor que reinicie, a autoatualização (ADR-027) reexecuta o novo binário em vez de sair. Funciona para qualquer Engine. Métricas de host exigem um node-exporter que o usuário possa rodar; sem ele, o Node entra sem métricas, como um Node adotado sem Docker hoje.

**Fora do escopo**
- Instalação do rTorrent. O SeedGrid ainda instala apenas o qBittorrent.
- ruTorrent: continua funcionando ao lado do Agent, como antes; o SeedGrid não depende dele.
- Transmission e Deluge: a interface do item 1 é o que precisariam, mas esta ADR não os adiciona.

**Consequências**

*Positivas*
- Um Node com rTorrent é um Node como qualquer outro: mesmos estados, configurações, publicação, migração e Portal.
- O Agent deixa de depender dos tipos do qBittorrent; o próximo Engine é um adaptador.
- A instalação rootless atende qualquer Engine numa máquina onde o operador não é root.

*Custos e limites aceitos*
- A fila do Agent reage no seu próprio loop (segundos), não dentro do Engine.
- O Agent faz o hash dos dados ao criar um `.torrent` para o rTorrent: a mesma leitura de disco que o criador do qBittorrent faz, em outro processo.
- Num seedbox, o provedor pode limitar recursos, portas e processos; o Agent não pede nada privilegiado, mas o plano do operador deve permitir um processo de longa duração.

**Relação com outras ADRs**
- **ADR-017 / ADR-043**: o rTorrent entra apenas pelo caminho definido na ADR-043.
- **ADR-019**: a publicação no rTorrent usa o criador do Agent.
- **ADR-021 / ADR-024**: a admissão para e inicia torrents pela mesma interface; a fila do Agent nunca inicia o que a admissão parou.
- **ADR-027**: a autoatualização se reexecuta quando nenhum supervisor reinicia o Agent.
- **ADR-039**: as configurações `engine.*` significam o mesmo em todo Node; para o rTorrent, o Agent as aplica.

**Implementação (fatias)**
1. Esta ADR.
2. A interface do Engine; o adaptador do qBittorrent por trás dela, sem mudança de comportamento.
3. XML-RPC sobre SCGI/HTTP e o adaptador do rTorrent, lado de leitura (torrents, estados, trackers, versão, peers).
4. O adaptador do rTorrent, lado de escrita (adicionar, parar/iniciar, rechecar, excluir, exportar, pasta).
5. A fila do Agent.
6. O criador de torrents do Agent.
7. `bootstrap-node.sh ENGINE=rtorrent`, a instalação rootless e a autoatualização por reexecução.
8. Guias e um teste em bancada com rTorrent (e ruTorrent) em container e como instalação rootless.

# **10. Template Oficial**
Toda ADR deverá seguir uma estrutura padronizada.
Campos mínimos:
- Identificador.
- Título.
- Contexto.
- Problema.
- Alternativas consideradas.
- Decisão adotada.
- Consequências.
- Benefícios.
- Limitações.
Esse padrão facilita leitura, revisão e comparação entre decisões.
# **11. Governança**
As ADRs representam a documentação oficial das decisões arquiteturais do SeedGrid.
Mudanças significativas na arquitetura deverão ser precedidas pela criação ou atualização de uma ADR correspondente.
Essa prática garante transparência, rastreabilidade e preservação do conhecimento técnico do projeto.
# **12. Relação com a Documentação**
As ADRs complementam os demais documentos da plataforma.
Enquanto os documentos funcionais descrevem **como** a plataforma opera, as ADRs registram **por que** determinadas decisões foram adotadas.
Essa separação evita duplicação de informações e preserva o contexto das escolhas arquiteturais.
# **13. Resumo**
As ADRs constituem o histórico oficial da evolução arquitetural do SeedGrid.
Elas permitem compreender:
- quais problemas motivaram cada decisão;
- quais alternativas foram avaliadas;
- quais benefícios eram esperados;
- quais limitações foram aceitas.
Esse histórico deverá acompanhar toda a evolução da plataforma.
# **14. Declaração Final**
A arquitetura do SeedGrid deverá evoluir de forma consciente e documentada.
As Architecture Decision Records representam o mecanismo oficial para registrar decisões estruturais, preservando conhecimento técnico, facilitando futuras revisões e garantindo que mudanças relevantes ocorram de maneira transparente e fundamentada.
