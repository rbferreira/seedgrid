# **SeedGrid**
# **Software Architecture**
Version: 1.0
Status: Draft
Document: 03 - Architecture.md
# **1. Objetivo**
Este documento define a arquitetura lógica do SeedGrid.
Todas as implementações deverão respeitar esta arquitetura.
Nenhum componente poderá comunicar-se diretamente com outro caso isso viole as regras aqui estabelecidas.
# **2. Filosofia Arquitetural**
SeedGrid é uma plataforma distribuída orientada a serviços.
O sistema é dividido em componentes independentes, desacoplados e especializados.
Cada componente possui responsabilidade única.
Cada componente pode evoluir independentemente.
# **3. Princípios**
# **Single Responsibility**
Cada componente possui uma única responsabilidade.
Exemplos:
Portal → Interface
Control Plane → Orquestração
Agent → Execução Local
Engine → BitTorrent
Scheduler → Decisão
# **API First**
Toda comunicação acontece por API.
Nenhum componente acessa banco de outro componente.
Nenhum componente acessa arquivos internos de outro componente.
# **Event Driven**
Toda alteração importante gera um evento.
Eventos nunca devem ser descartados silenciosamente.
Eventos devem ser persistidos.
# **Stateless Services**
Sempre que possível, componentes deverão ser Stateless.
Estado deverá permanecer no banco.
# **Loose Coupling**
Nenhum componente conhece detalhes internos de outro.
Cada componente conhece apenas contratos.
# **Replaceable Components**
Qualquer componente deverá poder ser substituído.
Exemplos:
qBittorrent
↓
Deluge
↓
Transmission
↓
Outro Engine
Sem alterar o restante da arquitetura.
# **4. Arquitetura Geral**
Usuário
│
Web Browser
│
────────────────────────────────────────
Portal
│
──────────────── REST API ──────────────
Control Plane
┌────────────┬────────────┐
│            │            │
│            │            │
Scheduler     Database     Event Bus
│
────────┼────────────────────────────────────────
Agent        Agent        Agent
│            │            │
Torrent Engine Torrent Engine Torrent Engine
│            │            │
Local Storage Local Storage Local Storage
# **5. Camadas**
O sistema será dividido em cinco camadas.
# **Camada 1**
Presentation Layer
Responsável pela interação humana.
Componente:
Portal
# **Camada 2**
Business Layer
Responsável pelas regras de negócio.
Componentes:
Control Plane
Scheduler
Authentication
Notifications
# **Camada 3**
Service Layer
Serviços internos.
Banco
Cache
Event Bus
Logs
# **Camada 4**
Execution Layer
Nodes.
Agent.
# **Camada 5**
Infrastructure Layer
Hardware.
Sistema Operacional.
Discos.
Rede.
Docker.
# **6. Componentes**
# **Portal**
Responsabilidade:
Interface gráfica.
Nunca toma decisões.
Nunca acessa Nodes.
Nunca acessa banco diretamente.
Sempre utiliza API.
# **Control Plane**
Responsabilidade:
Centro de inteligência.
Recebe comandos.
Executa regras.
Consulta Scheduler.
Atualiza banco.
Emite eventos.
Notifica usuários.
Nunca acessa discos dos Nodes.
Nunca executa BitTorrent.
# **Scheduler**
Responsabilidade:
Escolher o melhor Node.
Nunca comunica diretamente com Portal.
Nunca comunica diretamente com Engine.
Sempre responde ao Control Plane.
# **Agent**
Responsabilidade:
Representar o Node.
Executar comandos.
Coletar métricas.
Coletar inventário.
Traduzir chamadas do Control Plane.
Executar operações locais.
# **Torrent Engine**
Responsabilidade:
Executar protocolo BitTorrent.
Nada além disso.
# **7. Comunicação**
Toda comunicação deverá ocorrer via HTTPS.
Nunca por acesso compartilhado ao sistema operacional.
Fluxo permitido:
Portal
↓
Control Plane
↓
Agent
↓
Engine
Fluxos proibidos:
Portal → Agent
Portal → Engine
Portal → Banco
Scheduler → Agent
Scheduler → Engine
Engine → Banco
Engine → Portal
Agent → Banco
# **8. Responsabilidades**
# **Portal**
Visualização.
Pesquisa.
Configuração.
Autenticação.
# **Control Plane**
Orquestração.
Persistência.
Eventos.
Auditoria.
# **Scheduler**
Tomada de decisão.
Balanceamento.
Planejamento.
# **Agent**
Execução.
Inventário.
Monitoramento.
Controle local.
# **Engine**
BitTorrent.
# **9. Modelo dos Nodes**
Todo Node deverá possuir:
Agent
Engine
Storage
Network
Health
Capabilities
Labels
Estado
# **10. Estados dos Nodes**
ONLINE
Recebe novos torrents.
DRAIN
Não recebe novos torrents.
Continua operando.
MAINTENANCE
Executa apenas operações administrativas.
OFFLINE
Sem comunicação.
FAILED
Falha grave.
Intervenção necessária.
UNKNOWN
Estado desconhecido.
DISABLED
Desativado administrativamente.
Não participa da seleção do Scheduler.
Observação
A lista canônica de estados do Node é a definida em Database.md §11 (ONLINE, OFFLINE, DRAIN, MAINTENANCE, FAILED, UNKNOWN, DISABLED).
ONLINE, DRAIN, MAINTENANCE e DISABLED são definidos pelo operador (Requirements RF-003).
FAILED e UNKNOWN são atribuídos pelo próprio sistema; nunca escolhidos manualmente.
# **11. Heartbeat**
Todo Agent deverá enviar Heartbeat.
Informações mínimas:
Timestamp
Versão
CPU
RAM
Disco
Temperatura
Engine
Status
Uptime
Carga
Health Score
# **12. Inventário**
O Agent deverá enviar inventário.
Exemplos:
CPU
RAM
Sistema Operacional
Kernel
Docker
Discos
Interfaces
USB
Filesystem
Containers
Versões
# **13. Capabilities**
Cada Node possui Capabilities.
Exemplos:
Torrent Engine
SMART
Docker
SSD
UPS
USB3
NVMe
ARM64
x86
GPU
O Scheduler utiliza essas informações.
# **14. Labels**
Labels são informações definidas pelo operador.
Exemplos:
country=br
rack=01
tracker=anime
priority=gold
energy=low
Labels nunca são alteradas automaticamente.
# **15. Health Score**
Todo Node possui um Health Score.
Valor:
0 até 100.
Calculado continuamente.
Fatores:
CPU
RAM
Temperatura
SMART
Rede
Heartbeat
Disponibilidade
Storage
# **16. Banco**
O banco pertence exclusivamente ao Control Plane.
Nenhum outro componente possui acesso direto.
# **17. Event Bus**
Toda alteração gera evento.
Exemplos:
Torrent criado.
Torrent removido.
Node offline.
SMART falhou.
Atualização iniciada.
Eventos serão consumidos por:
Alertas.
Dashboard.
Logs.
Auditoria.
Relatórios.
# **18. Segurança**
Todo componente deverá possuir identidade própria.
Toda comunicação deverá ser autenticada.
Nenhum segredo poderá ser armazenado em código.
# **19. Escalabilidade**
O sistema deverá permitir:
Adicionar Nodes.
Remover Nodes.
Trocar Hardware.
Trocar Engine.
Trocar Sistema Operacional.
Sem alteração arquitetural.
# **20. Dependências**
Portal depende apenas da API.
Control Plane depende do banco.
Agent depende apenas do Engine.
Scheduler depende apenas do banco.
Engine depende apenas do protocolo BitTorrent.
# **21. Anti-Patterns**
São proibidos:
Acesso SSH para operações normais.
Execução manual de comandos.
Alteração direta do banco.
Configurações locais exclusivas.
Dependência da WebUI do Engine.
Comunicação direta entre Nodes.
Compartilhamento de disco entre Nodes.
# **22. Arquitetura Evolutiva**
A arquitetura deverá permitir futuras implementações de:
Múltiplos Control Planes.
Alta disponibilidade.
Plugins.
Múltiplos Engines.
SDK.
CLI.
Aplicativo Mobile.
Cloud Nodes.
Clusters federados.
# **23. Princípio Fundamental**
Nenhum componente deverá conhecer detalhes internos dos demais.
Toda interação ocorre através de contratos bem definidos.
Os Nodes fornecem capacidade.
O Control Plane fornece inteligência.
O Portal fornece experiência.
Juntos, formam um único Cluster lógico.
# **24. Perfil de Tracker (ADR-037)**
O SeedGrid publica em tracker UNIT3D. O conhecimento específico de cada site **não** é código
e **não** é configuração do operador: é um terceiro tipo de coisa, e a arquitetura o separa em
três camadas com fronteira explícita.
Camada 1 — família
Shape da API, autenticação e multipart de uma família de sites (hoje só `unit3d`).
Mora em código (`internal/unit3d`): é idêntico em todo site da família.
Camada 2 — perfil do site
Taxonomia (IDs de category/type/resolution) e, nas próximas issues, formato de nome, regras e
política de image host. Mora em **dado** (`tracker_profiles`), exportável: é o que alguém com
conta no site descobre, ajusta e compartilha. Tracker novo é uma **linha nova, não uma release nova**.
Camada 3 — config do operador
`api_token`, announce com passkey, `anon`, `modq`. Mora em `trackers`. **Nunca é exportada** e
nunca sai na API.
Princípios
- A fronteira entre as camadas 2 e 3 é **estrutural**: tabelas separadas, e nenhum tipo do perfil tem campo para credencial. Um export que vaze o token por esquecimento de filtro precisaria, antes, de um campo que não existe.
- O Portal só fala com a API (§6): ele **não conhece** a taxonomia de nenhum site — renderiza o que o perfil do tracker declara. Antes, as listas de IDs do samaritano estavam cravadas no frontend, como se fossem de todo site UNIT3D.
- Só o Control Plane acessa o banco e só o Control Plane fala com o site do tracker (descoberta e publicação, com o token do operador). O Agent não participa: ele continua executando e relatando.
- Perfil é **conteúdo não confiável** — o catálogo de um site de terceiro, a edição e o arquivo importado. Tudo passa por validação de tipos e faixas na entrada, nada é executável e nenhum campo livre vai direto para uma requisição.
- **Descoberta propõe, o operador aplica.** A descoberta lê o catálogo do próprio tracker e monta uma proposta; aplicar é uma ação separada e auditada, e roda no servidor a regra que protege o que foi ajustado à mão. Ela só enxerga o que já foi postado — o perfil descoberto **não é completo por definição**, e nada na interface pode sugerir o contrário.
- **Export/import.** O arquivo de troca é o `Profile` mais um envelope de identificação, gerado pela serialização de um tipo que **embute** o perfil e não tem campo de credencial — seção nova do perfil é exportada sem mexer no export. A origem dos termos é local e não viaja; o import atribui a sua (`imported`). Pré-visualizar (que nunca grava) e aplicar são duas chamadas; só a segunda é auditada.
Modelo de dados: `docs/12 - Database.md` §52-A. Contrato: `docs/13 - API.md` (perfil, descoberta de taxonomia, export e import). Decisão: ADR-037.
