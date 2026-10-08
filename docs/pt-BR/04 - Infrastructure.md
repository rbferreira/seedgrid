# **SeedGrid**
**Infrastructure Specification**
Version: 1.0
Status: Draft
Document: 04 - Infrastructure.md
# **1. Objetivo**
Este documento define a infraestrutura necessária para implantação do SeedGrid.
Seu objetivo é padronizar a organização do ambiente, definir os componentes necessários e estabelecer a responsabilidade de cada equipamento participante do Cluster.
# **2. Filosofia**
A infraestrutura deverá ser simples.
Cada equipamento deverá possuir apenas as responsabilidades necessárias.
Toda inteligência permanecerá concentrada no Control Plane.
Os Nodes deverão ser facilmente substituíveis.
# **3. Arquitetura Física**
A arquitetura mínima da versão 1.0 será composta por:
Um Control Plane.
Um ou mais Nodes.
Uma rede local.
Armazenamento local em cada Node.
Exemplo
Internet
│
Firewall / Gateway
│
Switch Gigabit
───────────────┼────────────────
│                 │
Control Plane VM     Node 01
│
HD USB
│                 │
Node 02          Node 03
│                 │
HD USB            HD USB
# **4. Control Plane**
Responsável por:
Portal.
API.
Scheduler.
Banco de Dados.
Eventos.
Autenticação.
Alertas.
Configuração.
Requisitos mínimos
4 vCPU
8 GB RAM
100 GB SSD
Rede Gigabit
Sistema Operacional Linux
Docker
Docker Compose
Requisitos recomendados
8 vCPU
16 GB RAM
250 GB SSD
Backup diário
Snapshot periódico
# **5. Nodes**
Cada Node representa um executor do Cluster.
Na versão inicial, um Node será composto por:
Raspberry Pi.
Fonte de alimentação.
Armazenamento local.
Rede Ethernet.
Requisitos mínimos
Raspberry Pi 3B+
2 GB RAM
Rede Ethernet
Disco dedicado
Docker
SeedGrid Agent
Torrent Engine
Requisitos recomendados
Raspberry Pi 4 ou superior.
SSD para o sistema operacional.
HD dedicado para armazenamento dos Torrents.
Fonte oficial.
Nobreak compartilhado.
# **6. Sistema Operacional**
Control Plane
Ubuntu Server LTS.
Nodes
Debian.
Ubuntu Server.
Raspberry Pi OS Lite.
Distribuições com interface gráfica não são recomendadas.
# **7. Docker**
Todo software do SeedGrid deverá ser executado em containers.
Exceções
Sistema Operacional.
Docker.
SeedGrid Agent (a definir na implementação; poderá rodar como serviço do sistema ou em container).
# **8. Containers do Control Plane**
Inicialmente serão previstos os seguintes containers:
API Gateway
Portal
PostgreSQL
Redis (se adotado na implementação)
Scheduler
Monitoring Service
Alert Service
Event Service
Authentication Service
Configuration Service
Reverse Proxy
A organização em containers não implica obrigatoriamente em processos independentes.
Alguns serviços poderão compartilhar a mesma aplicação durante a versão 1.0.
# **9. Containers dos Nodes**
Cada Node deverá possuir apenas os componentes necessários.
Containers previstos
Torrent Engine
SeedGrid Agent (caso containerizado)
Node Exporter
SMART Exporter
Watchtower (opcional)
O objetivo é minimizar consumo de recursos.
# **10. Rede**
Todos os componentes deverão permanecer na mesma rede lógica.
Os Nodes deverão possuir endereço IP fixo ou reserva DHCP.
O Control Plane deverá possuir endereço permanente.
A comunicação entre os componentes ocorrerá exclusivamente através da rede IP.
# **11. Armazenamento**
Cada Node possuirá armazenamento independente.
O Cluster não utilizará armazenamento compartilhado.
Cada Torrent existirá fisicamente apenas no Node ao qual foi atribuído.
# **12. Diretórios do Control Plane**
Estrutura sugerida
/opt/seedgrid/
├── compose/
├── config/
├── backups/
├── logs/
├── database/
└── monitoring/
# **13. Diretórios dos Nodes**
Estrutura sugerida
/opt/seedgrid/
├── config/
├── cache/
├── logs/
└── runtime/
Armazenamento dos Torrents
/data/
├── torrents/
├── incomplete/
├── completed/
└── lost+found
O Agent nunca deverá armazenar dados permanentes dos Torrents.
Esses pertencem exclusivamente ao Engine.
# **14. Hardware**
O SeedGrid não dependerá de um fabricante específico.
Exemplos compatíveis
Raspberry Pi.
Intel NUC.
Mini PCs.
Servidores.
Máquinas Virtuais.
VPS.
NAS.
Todos serão tratados como Nodes.
# **15. Escalabilidade**
A adição de um novo Node deverá exigir apenas:
Preparação do sistema operacional.
Instalação do Docker.
Instalação do Agent.
Registro no Control Plane.
Após esse processo, o novo Node deverá tornar-se imediatamente elegível para utilização pelo Scheduler.
# **16. Objetivos**
A infraestrutura deverá privilegiar:
Baixo consumo energético.
Facilidade de expansão.
Baixo custo.
Simplicidade operacional.
Facilidade de substituição de hardware.
Alta disponibilidade dos dados de configuração.
Independência entre os Nodes.
# **17. Arquitetura do Control Plane**
O Control Plane será implantado em uma única máquina virtual.
Toda a plataforma será executada através de Docker Compose.
Estrutura lógica:
Ubuntu Server
│
Docker Engine
│
Docker Compose
│
├── Traefik
├── Portal
├── Control Plane
├── PostgreSQL
├── Prometheus
├── Grafana
├── Loki
└── Redis (opcional)
Todos os containers compartilharão uma rede Docker dedicada ao SeedGrid.
# **18. Estrutura do Node**
Cada Node será composto pelos seguintes elementos:
Sistema Operacional
│
Docker Engine
│
├── Torrent Engine
├── Node Exporter
├── SMART Exporter
└── (Watchtower)
│
SeedGrid Agent
│
Discos Locais
O Agent será responsável por coordenar todos os componentes locais.
# **19. Organização do Sistema de Arquivos**
# **Sistema Operacional**
O sistema operacional deverá permanecer isolado dos dados dos torrents.
Exemplo:
/ (SSD ou cartão SD)
Sistema Operacional
Docker
SeedGrid Agent
Logs
# **Armazenamento**
Os torrents deverão permanecer em disco dedicado: isola o sistema operacional dos dados e é o que as camadas de proteção de `docs/06 - Storage.md` §35 vigiam. Um Node de **disco único** (`/data` no disco do sistema) é uma configuração suportada, declarada na instalação (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false`, §35.3).
Exemplo:
/data
├── torrents
├── incomplete
├── completed
├── recycle
└── backup
O Engine será configurado para utilizar exclusivamente esse volume.
# **20. Organização Docker**
Cada serviço deverá possuir:
Volume próprio.
Healthcheck.
Restart automático.
Logs independentes.
Nome padronizado.
Exemplo:
seedgrid-api
seedgrid-portal
seedgrid-postgres
seedgrid-grafana
seedgrid-prometheus
seedgrid-loki
seedgrid-traefik
# **21. Redes Docker**
Serão utilizadas redes dedicadas.
Exemplo:
seedgrid-core
Comunicação interna.
- ---------------------------
seedgrid-public
Portal
Traefik
API
- ---------------------------
seedgrid-monitoring
Grafana
Prometheus
Loki
Essa separação facilita organização e futuras evoluções.
# **22. Volumes Persistentes**
Todos os dados persistentes deverão utilizar volumes dedicados.
Exemplos:
Banco.
Grafana.
Prometheus.
Loki.
Portal (quando necessário).
Configurações.
Backups.
Nenhum container deverá depender do sistema de arquivos interno da imagem.
# **23. Convenções de Hostname**
Exemplos:
Control Plane
seedgrid-controller
Nodes
seedgrid-node-01
seedgrid-node-02
seedgrid-node-03
Essa convenção facilita identificação em logs e monitoramento.
# **24. Convenções de Diretórios**
Control Plane
/opt/seedgrid
/config
/data
/logs
/backups
/docker
Nodes
/opt/seedgrid
/config
/cache
/logs
/runtime
# **25. Fluxo de Inicialização**
# **Control Plane**
1. 1.
Sistema Operacional.
↓
1. 2.
Docker.
↓
1. 3.
Banco de Dados.
↓
1. 4.
Serviços internos.
↓
1. 5.
Portal.
↓
1. 6.
Traefik.
O Portal somente deverá iniciar após a disponibilidade da API.
# **Node**
1. 1.
Sistema Operacional.
↓
1. 2.
Docker.
↓
1. 3.
Torrent Engine.
↓
1. 4.
SeedGrid Agent.
↓
1. 5.
Registro.
↓
1. 6.
Heartbeat.
↓
1. 7.
Operação.
# **26. Atualizações**
Os containers deverão ser atualizados individualmente.
A atualização de um serviço não deverá exigir atualização de todo o ambiente.
Sempre que possível, atualizações deverão preservar configurações e volumes persistentes.
# **27. Backups**
O Control Plane deverá possuir rotina própria de backup.
Itens mínimos:
Banco.
Configurações.
Volumes persistentes.
Documentação.
Os Nodes permanecerão responsáveis apenas pelos dados locais do Torrent Engine.
# **28. Substituição de Hardware**
A substituição de um Node deverá seguir fluxo simples.
Preparar novo equipamento.
Instalar sistema operacional.
Instalar Docker.
Instalar SeedGrid Agent.
Conectar armazenamento.
Registrar no Cluster.
Nenhuma configuração manual adicional deverá ser necessária.
# **29. Escalabilidade**
A infraestrutura deverá permitir crescimento gradual.
Novos Nodes poderão ser adicionados sem interrupção dos existentes.
O aumento da capacidade do Cluster deverá ocorrer pela adição de novos Nodes, preservando a independência entre eles.
# **30. Princípios**
A infraestrutura deverá permanecer:
Modular.
Reproduzível.
Containerizada.
Padronizada.
Facilmente expansível.
Cada Node deverá ser considerado descartável.
O Control Plane deverá preservar a identidade do Cluster.
# **31. Topologia Recomendada**
A infraestrutura mínima recomendada para a versão 1.0 é composta por um único Control Plane e múltiplos Nodes independentes.
Internet
│
Firewall / Router
│
Switch Gigabit
│
┌─────────────────┼─────────────────┐
│                 │                 │
│                 │                 │
Control Plane       Seed Node 01     Seed Node 02
│                 │                 │
│                 │                 │
PostgreSQL         HD USB 1 TB      HD USB 1 TB
│
│
Grafana
Prometheus
Loki
│
│
Seed Node 03
│
HD USB 500 GB
Esta topologia atende plenamente aos requisitos da versão 1.0 e poderá ser expandida com novos Nodes sem alterações estruturais.
# **32. Responsabilidade dos Equipamentos**
# **Control Plane**
Responsável por:
- Portal Web;
- API;
- Banco de Dados;
- Scheduler;
- Eventos;
- Monitoramento;
- Configuração do Cluster;
- Auditoria.
# **Seed Nodes**
Responsáveis por:
- executar o Torrent Engine;
- armazenar os torrents;
- coletar métricas locais;
- executar comandos recebidos;
- disponibilizar inventário.
Nenhuma informação persistente do Cluster deverá depender exclusivamente de um Node.
# **33. Fluxo Operacional**
Fluxo simplificado de um novo torrent.
Operador
↓
Portal
↓
API
↓
Control Plane
↓
Scheduler
↓
Node escolhido
↓
Agent
↓
qBittorrent
↓
Download
↓
Seeding
O operador nunca escolhe diretamente o Node.
# **34. Fluxo de Monitoramento**
Node
↓
Agent
↓
Control Plane
↓
Prometheus
↓
Grafana
O Agent é responsável pela coleta.
O Control Plane centraliza.
O Prometheus armazena métricas.
O Grafana apresenta informações ao operador.
# **35. Fluxo de Eventos**
Sistema Operacional
↓
Agent
↓
Control Plane
↓
Database
↓
Portal
Todos os eventos percorrem o mesmo caminho.
# **36. Disponibilidade**
A indisponibilidade de um Node não deverá afetar os demais.
A indisponibilidade do Control Plane não deverá interromper o protocolo BitTorrent.
Os torrents continuarão sendo distribuídos normalmente pelos respectivos Engines enquanto o Control Plane estiver indisponível.
# **37. Recuperação**
Após retorno do Control Plane:
- os Agents deverão reconectar automaticamente;
- Heartbeats deverão ser retomados;
- eventos pendentes deverão ser enviados;
- inventários deverão ser sincronizados quando necessário.
Nenhuma intervenção manual deverá ser necessária.
# **38. Expansão**
A expansão do Cluster deverá ocorrer exclusivamente pela adição de novos Nodes.
Fluxo recomendado:
1. Preparar o equipamento.
2. Instalar o sistema operacional.
3. Instalar Docker.
4. Instalar o SeedGrid Agent.
5. Configurar o endereço do Control Plane.
6. Registrar o Node.
Após o registro, o Scheduler poderá utilizá-lo automaticamente.
# **39. Manutenção**
Nodes poderão ser colocados em modo de manutenção.
Enquanto estiverem nesse estado:
- não receberão novos torrents;
- continuarão semeando os torrents existentes;
- permanecerão visíveis ao operador.
Esse mecanismo permitirá intervenções sem impacto na disponibilidade do Cluster.
# **40. Requisitos de Rede**
Todos os Nodes deverão possuir conectividade com o Control Plane.
Não será necessário que os Nodes se comuniquem diretamente entre si.
Essa decisão reduz o acoplamento da infraestrutura e simplifica a implantação em diferentes ambientes.
# **41. Requisitos de Armazenamento**
Cada Node é responsável exclusivamente pelo seu armazenamento local.
O SeedGrid não utilizará:
- NFS;
- SMB;
- Ceph;
- GlusterFS;
- armazenamento distribuído.
Essa decisão reduz complexidade, aumenta previsibilidade e mantém a arquitetura compatível com hardware de baixo custo.
# **42. Resumo**
A infraestrutura da versão 1.0 foi projetada para privilegiar simplicidade operacional.
Todos os componentes possuem responsabilidades bem definidas.
Os Nodes são independentes.
O Control Plane centraliza inteligência e persistência.
A expansão ocorre apenas pela adição de novos Nodes.
# **43. Declaração Final**
A infraestrutura do SeedGrid foi concebida para ser reproduzível, modular e de baixo custo.
A utilização de componentes independentes, comunicação centralizada e armazenamento local permite construir um Cluster distribuído utilizando hardware comum, preservando simplicidade de implantação e facilidade de manutenção.
Essa arquitetura constitui a base operacional da versão 1.0 do SeedGrid.
