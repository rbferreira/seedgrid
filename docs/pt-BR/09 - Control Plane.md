# **SeedGrid**
# **Control Plane Specification**
Version: 1.0
Status: Draft
Document: 09 - Control Plane.md
# **1. Objetivo**
O Control Plane é o núcleo do SeedGrid.
É o componente responsável por coordenar todas as operações do Cluster, manter o estado persistente da plataforma e orquestrar a comunicação entre os Nodes e o Portal.
Nenhuma decisão operacional deverá ser tomada fora do Control Plane.
# **2. Filosofia**
O Control Plane não executa torrents.
O Control Plane não armazena arquivos dos torrents.
O Control Plane não acessa diretamente os Nodes.
O Control Plane coordena.
Os Nodes executam.
# **3. Responsabilidades**
O Control Plane é responsável por:
- manter o estado do Cluster;
- registrar Nodes;
- autenticar Nodes;
- gerenciar usuários;
- persistir informações;
- receber Heartbeats;
- receber inventários;
- receber eventos;
- distribuir comandos;
- registrar auditoria;
- fornecer APIs ao Portal;
- acionar o Scheduler;
- coordenar operações administrativas.
O Control Plane NÃO deverá:
- executar operações locais nos Nodes;
- acessar diretamente o Torrent Engine;
- acessar discos dos Nodes;
- depender de características específicas do sistema operacional dos Nodes.
# **4. Arquitetura**
Portal
│
HTTPS / REST
│
┌─────────────────────┐
│   API Gateway        │
└─────────────────────┘
│
┌──────────────────────────────┐
│                              │
│ Cluster Services             │
│                              │
└──────────────────────────────┘
│
┌────────────┼────────────┐
│            │            │
Database     Scheduler     Event Bus
│
└────────────┬────────────┘
│
HTTPS / REST
│
SeedGrid Agents
# **5. Organização Interna**
O Control Plane será composto por serviços especializados.
Esses serviços representam módulos lógicos da aplicação.
Inicialmente poderão ser executados no mesmo processo.
A arquitetura deverá permitir sua separação futura sem alterações significativas.
# **6. Serviços**
# **API Gateway**
Responsável por:
- autenticação;
- autorização;
- validação de requisições;
- roteamento interno;
- documentação da API.
# **Cluster Service**
Responsável por:
- gerenciamento do Cluster;
- configuração global;
- estado geral da plataforma.
# **Node Service**
Responsável por:
- registro de Nodes;
- estado dos Nodes;
- inventário;
- capabilities;
- labels;
- tags.
# **Torrent Service**
Responsável por:
- cadastro de torrents;
- consulta;
- atualização;
- histórico;
- operações administrativas.
# **Scheduler Service**
Responsável por decidir onde novos torrents serão executados.
Os algoritmos utilizados serão definidos em documento próprio.
# **Monitoring Service**
Responsável por:
- Health Scores;
- Heartbeats;
- integração com métricas;
- consolidação da saúde do Cluster.
# **Event Service**
Responsável por:
- recebimento;
- armazenamento;
- distribuição;
- consulta de eventos.
# **Alert Service**
Responsável por:
- geração;
- gerenciamento;
- encerramento;
- consulta de alertas.
# **Authentication Service**
Responsável por:
- usuários;
- sessões;
- permissões;
- autenticação.
# **Configuration Service**
Responsável por:
- configurações globais;
- Feature Flags;
- parâmetros do sistema.
# **7. Comunicação Interna**
Os serviços internos deverão comunicar-se utilizando interfaces bem definidas.
Nenhum serviço deverá acessar diretamente estruturas internas de outro serviço.
Toda comunicação deverá ocorrer através de contratos internos.
# **8. Persistência**
Toda persistência será realizada exclusivamente pelo Control Plane.
Os Agents nunca escreverão diretamente no banco de dados.
O Portal nunca escreverá diretamente no banco de dados.
# **9. Eventos**
Todo serviço poderá gerar eventos.
Exemplos:
Node registrado.
Torrent criado.
Configuração alterada.
Usuário autenticado.
Alerta criado.
Todos os eventos deverão ser registrados pelo Event Service.
# **10. Fluxo Geral**
Portal
↓
API Gateway
↓
Serviço responsável
↓
Banco de Dados
↓
Event Service
↓
Resposta
# **11. Fluxo dos Nodes**
Agent
↓
API Gateway
↓
Node Service
↓
Database
↓
Event Service
↓
Resposta ao Agent
# **12. Disponibilidade**
O Control Plane representa um ponto central da plataforma.
Sua indisponibilidade não interrompe o funcionamento do protocolo BitTorrent nos Nodes.
Durante sua indisponibilidade:
- os Nodes continuam semeando;
- os Engines permanecem operacionais;
- inventários deixam de ser sincronizados temporariamente;
- comandos administrativos ficam indisponíveis.
Após o retorno, os Agents deverão sincronizar automaticamente suas informações.
# **13. Objetivos Arquiteturais**
O Control Plane deverá ser:
- modular;
- escalável;
- desacoplado;
- observável;
- facilmente atualizável;
- independente da infraestrutura dos Nodes.
# **14. Princípio Fundamental**
O Control Plane representa a autoridade máxima do Cluster.
Toda decisão administrativa, operacional e de orquestração deverá partir dele.
Os demais componentes existem para executar, apresentar ou suportar essas decisões.
# **15. Registro de Nodes**
O registro de um novo Node deverá seguir o fluxo abaixo.
Node
↓
Agent
↓
API Gateway
↓
Authentication Service
↓
Node Service
↓
Database
↓
Event Service
↓
Resposta ao Agent
Após o registro:
- o Node receberá sua identidade;
- será criado seu inventário inicial;
- será registrado o primeiro Heartbeat;
- será gerado um evento de registro.
# **16. Recebimento de Heartbeats**
O Monitoring Service será responsável por receber e validar Heartbeats enviados pelos Agents.
Ao receber um Heartbeat, o sistema deverá:
- validar a identidade do Node;
- atualizar o horário do último contato;
- atualizar o Health Score;
- verificar alterações relevantes;
- registrar eventos quando necessário.
Heartbeats não deverão gerar auditoria, pois representam operações rotineiras.
# **17. Sincronização de Inventário**
O Inventory Manager do Agent enviará informações apenas quando necessário.
O Node Service deverá:
- comparar versões do inventário;
- atualizar os registros persistentes;
- registrar alterações relevantes;
- preservar histórico quando aplicável.
# **18. Ciclo de Vida de um Torrent**
Fluxo simplificado:
Portal
↓
API Gateway
↓
Torrent Service
↓
Scheduler Service
↓
Node Service
↓
Agent
↓
Torrent Engine
↓
Resposta
↓
Database
↓
Event Service
O Portal nunca deverá conhecer o Node selecionado antes da decisão do Scheduler.
# **19. Operações Administrativas**
Toda operação administrativa seguirá o mesmo padrão.
Exemplo:
Portal
↓
API Gateway
↓
Serviço responsável
↓
Node Service
↓
Agent
↓
Resultado
↓
Database
↓
Evento
Exemplos de operações:
- Pausar torrent.
- Retomar torrent.
- Executar recheck.
- Atualizar inventário.
- Reiniciar Engine.
- Atualizar configuração.
# **20. Consulta de Informações**
Operações de leitura nunca deverão consultar diretamente os Nodes.
Sempre que possível, o Portal utilizará informações persistidas no banco de dados.
A comunicação direta com o Agent será utilizada apenas quando informações em tempo real forem explicitamente necessárias.
# **21. Estado do Cluster**
O Control Plane deverá manter uma visão consolidada do Cluster.
Exemplos:
- quantidade de Nodes;
- Nodes online;
- capacidade total;
- espaço disponível;
- quantidade de torrents;
- upload agregado;
- alertas ativos.
Essa visão deverá ser utilizada pelo Dashboard principal.
# **22. Coordenação**
O Control Plane atua como coordenador.
Ele nunca executa operações locais.
Quando uma ação exigir execução em um Node:
- valida a solicitação;
- registra a intenção;
- envia o comando ao Agent;
- aguarda resposta;
- atualiza o estado persistente.
# **23. Tratamento de Erros**
Toda falha deverá ser tratada em três níveis.
Nível 1
Erro local do serviço.
Exemplo:
Falha de validação.
Nível 2
Erro de comunicação.
Exemplo:
Node indisponível.
Nível 3
Erro operacional.
Exemplo:
Engine recusou uma operação.
Cada nível deverá produzir mensagens apropriadas ao operador e eventos compatíveis com a severidade da falha.
# **24. Auditoria**
Operações iniciadas por usuários deverão gerar auditoria.
Exemplos:
- alteração de configurações;
- remoção de torrents;
- cadastro de usuários;
- mudança de permissões;
- alteração de Labels.
Operações automáticas do sistema gerarão apenas eventos operacionais.
# **25. Concorrência**
O Control Plane deverá suportar múltiplas operações simultâneas.
Operações independentes deverão poder ser executadas em paralelo.
Sempre que necessário, mecanismos de sincronização deverão impedir conflitos sobre o mesmo recurso.
Exemplos:
- dois usuários tentando alterar o mesmo Node;
- múltiplas operações sobre o mesmo torrent;
- atualização simultânea de inventário.
# **26. Escalabilidade**
A arquitetura deverá permitir crescimento horizontal.
Novos serviços poderão ser adicionados sem alterar contratos existentes.
O aumento do número de Nodes não deverá exigir alterações estruturais no Control Plane.
# **27. Disponibilização de Dados**
O Control Plane será a fonte oficial de consulta para:
- Portal;
- CLI;
- futuras integrações;
- SDK.
Nenhum desses componentes deverá acessar diretamente os Agents.
# **28. Serviços Internos**
Todos os serviços internos deverão ser independentes.
Exemplo:
O Alert Service não deverá conhecer detalhes do Scheduler.
O Scheduler não deverá conhecer detalhes do Portal.
O Torrent Service não deverá conhecer detalhes do Inventory.
A comunicação ocorrerá através de interfaces internas bem definidas.
# **29. Atualizações**
O Control Plane deverá permitir atualização sem perda do estado persistente.
Sempre que possível, atualizações deverão preservar sessões ativas e não exigir reconfiguração dos Nodes.
# **30. Resiliência**
Falhas em um serviço interno não deverão comprometer completamente os demais.
Exemplo:
Caso o Alert Service esteja indisponível:
- o Scheduler continua funcionando;
- o Torrent Service continua funcionando;
- o Node Service continua funcionando.
A indisponibilidade deverá ser limitada ao domínio afetado.
# **31. Princípios**
O Control Plane coordena.
Os serviços executam responsabilidades específicas.
Os Agents executam ações locais.
Os Nodes executam torrents.
Essa separação deverá permanecer clara durante toda a evolução da plataforma.
# **32. Segurança**
O Control Plane será responsável por centralizar toda a segurança da plataforma.
Suas responsabilidades incluem:
- autenticação de usuários;
- autenticação de Nodes;
- autorização;
- gerenciamento de sessões;
- emissão e validação de credenciais;
- auditoria de operações.
Nenhuma decisão de autorização deverá ser delegada aos Nodes.
# **33. Observabilidade**
O Control Plane deverá disponibilizar informações suficientes para diagnóstico da plataforma.
Exemplos:
- estado dos serviços internos;
- quantidade de Nodes conectados;
- Heartbeats recebidos;
- operações em andamento;
- alertas ativos;
- eventos recentes;
- utilização dos recursos do próprio Control Plane.
Essas informações deverão ser utilizadas pelo sistema de monitoramento.
# **34. Logging**
Todos os serviços deverão gerar logs estruturados.
Cada registro deverá conter, quando aplicável:
- data e hora;
- serviço responsável;
- nível de severidade;
- identificador da operação;
- usuário ou Node relacionado;
- mensagem descritiva.
Os logs deverão facilitar auditoria e diagnóstico, evitando informações redundantes.
# **35. Configuração**
Toda configuração do Control Plane deverá estar centralizada.
Exemplos:
- parâmetros do Cluster;
- autenticação;
- integrações;
- políticas de Scheduler;
- parâmetros de monitoramento;
- retenção;
- Feature Flags.
Configurações locais espalhadas entre serviços deverão ser evitadas.

**Settings Store (ADR-023).** Os tunáveis de **comportamento** (intervalos de Worker,
limiares de admissão/alerta, retenção) ficam num catálogo em código (fonte única de metadata)
com overrides na tabela `settings`, servidos por um `Store` thread-safe: leituras lock-free
nos loops, escrita valida e aplica em runtime (hot-reload). Precedência: **o banco manda; o
env só semeia** no 1º boot. Editável no painel **Configurações** do Portal via `/api/v1/settings`.
Infra e segredo (DB URL, tokens, senha admin, API keys, prom URL, portas) permanecem em env,
imutáveis em runtime. Catálogo completo em `25 - Business Rules.md`.

**Configuração do Agent e do Engine por Node (ADR-039).** O catálogo também cobre o comportamento
dos **Seed Nodes** (`agent.*`, `engine.*`, marcados `PerNode`): o valor de cluster é o do `Store`
acima e o pacote `internal/agentconfig` soma o **override por Node** (tabela `node_settings`),
versiona o conjunto (hash), registra a versão que cada Agent aplicou e **entrega os valores
efetivos na resposta do heartbeat**. O Agent aplica a quente. Precedência: override do Node >
valor de cluster > env do Node (fallback do Agent) > default do código. A migração dos Nodes
existentes é amparada por uma **semeadura única** dos overrides a partir do `.env` que o Agent
relata (`NODE_CONFIG_SEEDED`). Ver `13 - API.md` §8.1 e `19 - Deployment.md` §50.5.

# **36. Integrações**
O Control Plane será o único componente autorizado a integrar-se com sistemas externos.
Exemplos:
- serviços de autenticação;
- sistemas de monitoramento;
- plataformas de notificação;
- webhooks;
- futuras APIs públicas.
Os Nodes não deverão comunicar-se diretamente com serviços externos em nome do Cluster, salvo quando necessário para o funcionamento do próprio Torrent Engine.

**Screenshots para publicação (ADR-026/030/040).** Estende a integração UNIT3D da ADR-020: o
comando `extract_screenshots` (fila de comandos, ADR-014) manda o Agent extrair N frames
JPEG do torrent via `ffmpeg`/`ffprobe` e devolvê-los em base64 no `result` do comando; o CP
aguarda de forma bloqueante (`command.Service.WaitDone`) ou em etapas (extract + render),
decodifica os frames e os hospeda via a **cadeia de upload** (`internal/imagehost`, ADR-040):
host do tracker (seção `image_host` do perfil do site + chave do operador cadastrada e
conferida) → imgbb (chave global no catálogo, cliente `internal/imgbb`). O primeiro host
que hospedar todos os frames vence; o orçamento total é de **25 s**, repartido entre os
hosts que ainda faltam. Cada falha vira entrada no Feed (`screenshot_host_failed`,
ADR-022) com motivo do conjunto fechado da ADR-040. Exposto ao Portal via três rotas
(documentadas em `13 - API.md`): `POST /torrents/{id}/screenshots` (um passo),
`POST /torrents/{id}/screenshots/extract` (enfileira extração) e
`POST /screenshots/render` (hospeda + monta o BBCode). A conferência humana antes do
envio ao tracker (mesmo gate da ADR-020) é preservada. A quantidade de screenshots é
tunável (`publish.screenshot_count`, catálogo ADR-023 — ver `25 - Business Rules.md`).
# **37. Escalabilidade**
A arquitetura deverá permitir expansão gradual.
Exemplos:
- aumento do número de Nodes;
- aumento da quantidade de torrents;
- crescimento do número de usuários;
- inclusão de novos serviços internos.
Sempre que possível, novos componentes deverão reutilizar contratos existentes.
# **38. Evolução**
O Control Plane deverá permitir evolução sem alteração significativa da arquitetura.
Exemplos:
- novos tipos de Node;
- novos Engines;
- novos algoritmos de Scheduler;
- novas interfaces de usuário;
- novas integrações.
A arquitetura deverá favorecer extensão em vez de substituição.
# **39. Recuperação**
Após reinicialização, o Control Plane deverá:
- restaurar o estado persistido;
- disponibilizar a API;
- aceitar reconexão dos Nodes;
- atualizar o estado do Cluster conforme os Agents retomem comunicação.
Nenhuma sincronização manual deverá ser necessária.
# **40. Limites de Responsabilidade**
O Control Plane não deverá:
- manipular arquivos de torrents diretamente;
- acessar discos dos Nodes;
- executar comandos do sistema operacional dos Nodes;
- interpretar detalhes internos do Torrent Engine.
Essas responsabilidades pertencem exclusivamente ao Agent.
# **41. Dependências**
O Control Plane dependerá de:
- PostgreSQL;
- infraestrutura de monitoramento;
- serviços de autenticação;
- rede de comunicação com os Agents.
A indisponibilidade de componentes opcionais não deverá impedir a operação principal da plataforma.
# **42. Princípios de Projeto**
O Control Plane deverá permanecer:
- modular;
- previsível;
- desacoplado;
- auditável;
- escalável;
- orientado a serviços.
Esses princípios deverão orientar futuras evoluções.
# **43. Diagrama Geral**
Operador
│
Portal
│
API Gateway
│
┌───────────────────────────────────────────────┐
│               Control Plane                   │
│                                               │
│ Cluster Service                               │
│ Node Service                                  │
│ Torrent Service                               │
│ Scheduler Service                             │
│ Monitoring Service                            │
│ Event Service                                 │
│ Alert Service                                 │
│ Authentication Service                        │
│ Configuration Service                         │
└───────────────────────────────────────────────┘
│
PostgreSQL
│
HTTPS / REST
│
┌────────────┼────────────┐
│            │            │
Agent        Agent        Agent
│            │            │
Engine       Engine       Engine
│            │            │
Storage      Storage      Storage
# **44. Resumo**
O Control Plane representa a autoridade central do Cluster.
Ele mantém a visão consolidada da plataforma, coordena os serviços internos, fornece APIs para operadores e integrações, recebe informações dos Nodes e garante que todas as decisões administrativas e operacionais sejam tomadas de maneira consistente.
Toda inteligência do SeedGrid reside no Control Plane.
# **45. Declaração Final**
A arquitetura do Control Plane foi concebida para separar claramente decisões, execução e apresentação.
O Portal apresenta.
O Control Plane decide.
O Agent executa.
Os Nodes fornecem capacidade computacional e armazenamento.
Essa separação constitui a base arquitetural do SeedGrid e deverá ser preservada ao longo de toda a evolução da plataforma.
