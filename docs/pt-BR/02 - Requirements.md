# **SeedGrid**
# **Requirements Specification**
Version: 1.0
Status: Draft
Document: 02 - Requirements.md
# **1. Objetivo**
Este documento define todos os requisitos funcionais e não funcionais do SeedGrid.
Nenhuma implementação deverá introduzir funcionalidades que não estejam descritas neste documento sem atualização formal da especificação.
# **2. Escopo**
O SeedGrid deverá permitir a administração centralizada de múltiplos Nodes BitTorrent, abstraindo completamente sua existência física.
O operador deverá enxergar apenas um Cluster.
# **3. Definições**
# **Cluster**
Conjunto de Nodes administrados pelo mesmo Control Plane.
# **Node**
Máquina física ou virtual participante do Cluster.
Exemplos:
- Raspberry Pi
- Mini PC
- VM
- Servidor
- VPS
- NAS
# **Agent**
Daemon instalado em cada Node.
Responsável pela comunicação entre o Control Plane e o Engine.
# **Engine**
Software responsável pela comunicação BitTorrent.
Versão inicial:
- qBittorrent
# **Control Plane**
Componente responsável por todas as decisões do Cluster.
# **Portal**
Interface gráfica do operador.
# **4. Requisitos Funcionais**
# **RF-001**
O sistema deverá permitir registrar novos Nodes.
# **RF-002**
O sistema deverá permitir remover Nodes.
# **RF-003**
O sistema deverá permitir colocar Nodes em modo:
- Online
- Drain
- Maintenance
- Disabled
# **RF-004**
O sistema deverá detectar automaticamente Nodes indisponíveis.
# **RF-005**
O sistema deverá manter inventário completo de todos os Nodes.
# **RF-006**
O sistema deverá permitir adicionar torrents através de:
- Upload de arquivo .torrent
- Magnet Link
# **RF-007**
O operador nunca deverá escolher manualmente um Node como comportamento padrão.
A seleção deverá ser realizada automaticamente pelo Scheduler.
O Portal poderá oferecer uma opção avançada para forçar um Node específico quando autorizado.
# **RF-008**
Todo torrent deverá possuir um identificador único interno.
# **RF-009**
Todo torrent deverá possuir histórico completo.
# **RF-010**
Todo torrent deverá possuir estado.
Estados permitidos:
- Queued
- Downloading
- Seeding
- Paused
- Checking
- Error
- Archived
Observação
Esta lista é um subconjunto orientado ao operador.
A lista canônica de estados do Torrent é a definida em Database.md §36 (QUEUED, DOWNLOADING, SEEDING, PAUSED, CHECKING, MOVING, ERROR, ARCHIVED, DELETED, UNKNOWN).
# **RF-011**
O sistema deverá permitir pesquisar torrents utilizando:
Nome
Hash
Tracker
Status
Node
Tag
Categoria
Data
Ratio
# **RF-012**
O sistema deverá permitir mover torrents entre Nodes.
# **RF-013**
O sistema deverá permitir pausar torrents.
# **RF-014**
O sistema deverá permitir retomar torrents.
# **RF-015**
O sistema deverá permitir remover torrents.
# **RF-016**
O sistema deverá permitir remover dados físicos opcionalmente.
# **RF-017**
O sistema deverá permitir executar Recheck.
# **RF-018**
O sistema deverá armazenar histórico de upload.
# **RF-019**
O sistema deverá armazenar histórico de download.
# **RF-020**
O sistema deverá armazenar histórico de ratio.
# **5. Gestão dos Nodes**
Cada Node deverá possuir:
UUID
Hostname
Nome amigável
Descrição
Sistema Operacional
Arquitetura
CPU
RAM
Discos
Versão do Agent
Versão do Engine
Endereço IP
Data de registro
Último Heartbeat
Estado
Tags
Labels
# **6. Gestão dos Discos**
Cada disco deverá possuir:
UUID
Serial
Modelo
Capacidade
Espaço livre
Temperatura
Power On Hours
SMART
Filesystem
Ponto de montagem
Estado
# **7. Gestão dos Trackers**
Cada Tracker deverá possuir:
Nome
URL
Status
Quantidade de Torrents
Upload
Download
Ratio
Tempo médio de resposta
Quantidade de erros
Último announce
# **8. Gestão dos Usuários**
O sistema deverá suportar múltiplos usuários.
Papéis iniciais:
Administrator
Operator
Read Only
# **9. Dashboard**
O Dashboard deverá apresentar:
Saúde do Cluster
Quantidade de Nodes
Nodes Online
Nodes Offline
Upload agregado
Download agregado
Quantidade de Torrents
Quantidade de Peers
Quantidade de Seeds
Espaço utilizado
Espaço disponível
Alertas ativos
Eventos recentes
# **10. Dashboard de Node**
Cada Node deverá possuir uma página própria contendo:
Hardware
Sistema Operacional
Temperatura
CPU
RAM
Rede
Discos
SMART
Containers
Engine
Velocidade
Peers
Seeds
Torrents
Histórico
Eventos
# **11. Dashboard de Torrent**
Cada Torrent deverá possuir uma página contendo:
Nome
Hash
Node
Tracker
Tags
Categoria
Prioridade
Status
Upload
Download
Ratio
Peers
Seeds
Tempo ativo
Histórico
Logs
# **12. Dashboard de Tracker**
Cada Tracker deverá apresentar:
Disponibilidade
Tempo de resposta
Quantidade de Torrents
Quantidade de Peers
Quantidade de Seeds
Ratio
Upload
Download
Histórico
# **13. Eventos**
Toda operação relevante deverá gerar evento.
Exemplos:
Node registrado
Node removido
Node offline
Node online
Torrent criado
Torrent migrado
Torrent removido
Backup executado
Atualização executada
Erro SMART
Tracker indisponível
# **14. Alertas**
O sistema deverá gerar alertas para:
Pouco espaço
SMART crítico
Temperatura elevada
Node offline
Heartbeat perdido
Container parado
Erro de API
Tracker indisponível
Erro de announce
Uso elevado de CPU
Uso elevado de memória
# **15. Logs**
Todos os componentes deverão gerar logs estruturados.
Os logs deverão conter:
Timestamp
Node
Serviço
Severidade
Evento
Descrição
Correlation ID
# **16. API**
Todas as funcionalidades deverão estar disponíveis através de API.
Nenhuma funcionalidade deverá existir apenas na interface gráfica.
# **17. Scheduler**
O Scheduler deverá decidir automaticamente onde novos torrents serão executados.
Os critérios serão definidos em documento próprio.
# **18. Backup**
O sistema deverá permitir backup automático de:
Banco
Configurações
Metadados
Estado dos torrents
Arquivos .torrent
Arquivos FastResume
Configuração do Agent
# **19. Restore**
O sistema deverá restaurar automaticamente um Node substituído.
# **20. Atualizações**
O Agent deverá suportar atualização remota.
O Control Plane deverá suportar atualização independente.
O Portal deverá suportar atualização independente.
# **21. Segurança**
Toda comunicação deverá utilizar TLS.
Todos os usuários deverão ser autenticados.
Todas as APIs deverão exigir autenticação.
Todas as operações deverão ser auditadas.
# **22. Auditoria**
Todas as alterações deverão registrar:
Usuário
Horário
Origem
Objeto alterado
Valor anterior
Novo valor
# **23. Requisitos Não Funcionais**
# **RNF-001**
Disponibilidade contínua.
# **RNF-002**
Escalabilidade horizontal.
# **RNF-003**
Baixo consumo de memória.
# **RNF-004**
Baixo consumo de CPU.
# **RNF-005**
Atualizações sem perda de configuração.
# **RNF-006**
Observabilidade completa.
# **RNF-007**
Infraestrutura reproduzível.
# **RNF-008**
Compatibilidade inicial:
- Raspberry Pi 3B+
- Raspberry Pi 4
- Raspberry Pi 5
- Ubuntu Server
- Debian
- Raspberry Pi OS Lite
# **RNF-009**
Toda configuração deverá ser exportável.
# **RNF-010**
Nenhum componente deverá depender diretamente de interface gráfica.
# **24. Critérios de Aceitação**
O sistema será considerado funcional quando permitir:
- Registrar novos Nodes.
- Adicionar torrents sem escolher um Node manualmente.
- Visualizar o Cluster como uma entidade única.
- Substituir um Node sem perda operacional.
- Monitorar completamente todos os componentes.
- Operar exclusivamente através do Portal ou da API.
