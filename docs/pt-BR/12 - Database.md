# **SeedGrid**
# **Database Specification**
Version: 1.0
Status: Draft
Document: 12 - Database.md
# **1. Objetivo**
Este documento define o modelo de dados oficial do SeedGrid.
Toda informação persistida pelo sistema deverá obedecer às estruturas aqui definidas.
Nenhum componente poderá criar tabelas próprias fora desta especificação.
Toda alteração estrutural deverá ocorrer através de migrations versionadas.
# **2. Filosofia**
O banco de dados representa o estado do Cluster.
Ele **não é** um armazenamento de métricas em tempo real.
Ele **não é** um sistema de logs.
Ele **não é** um cache.
Sua responsabilidade é armazenar o estado persistente da plataforma.
# **3. Tecnologias**
# **Banco Principal**
PostgreSQL 17
Responsável por:
- Configuração
- Estado do Cluster
- Inventário
- Usuários
- Torrents
- Nodes
- Eventos
- Auditoria
# **Extensões obrigatórias**
uuid-ossp
pgcrypto
citext
btree_gin
pg_trgm
# **Dados não armazenados**
Os seguintes dados NÃO pertencem ao PostgreSQL:
Métricas → Prometheus
Logs → Loki
Dashboards → Grafana
Arquivos .torrent → Storage do Node
FastResume → Storage do Node
# **4. Convenções**
Toda tabela deverá utilizar:
id UUID PRIMARY KEY
Nunca utilizar SERIAL.
Toda tabela deverá possuir:
created_at
updated_at
Quando aplicável:
deleted_at
Nenhuma tabela utilizará exclusão física.
Sempre Soft Delete.
Todas as datas:
TIMESTAMP WITH TIME ZONE
UTC
Strings:
TEXT
Evitar VARCHAR.
Booleanos:
BOOLEAN
Identificadores públicos:
UUID v7 (quando disponível)
Caso indisponível:
UUID v4.
# **5. Convenções de Nome**
Tabelas
Sempre plural.
Exemplo
nodes
torrents
users
trackers
labels
Colunas
snake_case
Chaves estrangeiras
node_id
torrent_id
tracker_id
user_id
Índices
idx_nodes_hostname
idx_torrents_hash
Unique
uk_nodes_hostname
Foreign Keys
fk_torrents_node
# **6. Modelo Conceitual**
O banco será dividido em Domínios.
Cada domínio representa uma responsabilidade.
Cluster
├── Identity
├── Nodes
├── Storage
├── Torrent
├── Users
├── Monitoring
├── Inventory
├── Scheduler
├── Events
├── Audit
└── Configuration
# **7. Entidades Principais**
O sistema possuirá inicialmente as seguintes entidades.
Identity
users
roles
permissions
api_tokens
sessions
Cluster
clusters
cluster_settings
cluster_variables
Nodes
nodes
node_labels
node_capabilities
node_tags
node_states
node_versions
node_interfaces
Storage
disks
partitions
filesystems
mountpoints
storage_pools
Torrent
torrents
torrent_states
torrent_categories
torrent_tags
torrent_trackers
torrent_files
torrent_history
Inventory
hardware
cpus
memory_modules
usb_devices
network_interfaces
docker_info
containers
Monitoring
health_scores
heartbeats
alerts
notifications
Events
events
event_types
event_queue
Audit
audit_logs
audit_changes
Configuration
settings
feature_flags
system_variables
# **8. Relacionamentos Gerais**
Cluster
│
├── Nodes
│      │
│      ├── Disks
│      ├── Inventory
│      ├── Heartbeats
│      ├── Torrents
│      └── Events
│
├── Users
│
├── Trackers
│
└── Settings
# **9. Tabela clusters**
Objetivo
Representar um Cluster lógico.
Mesmo existindo apenas um inicialmente, a estrutura já suporta múltiplos Clusters.
Campos
id
uuid
name
description
timezone
created_at
updated_at
deleted_at
Relacionamentos
1:N Nodes
1:N Users
1:N Settings
# **10. Tabela nodes**
Objetivo
Representar qualquer equipamento participante do Cluster.
Nunca armazenar informações específicas de hardware nesta tabela.
Essas informações pertencem ao domínio Inventory.
Campos
id
cluster_id
uuid
hostname
display_name
description
fqdn
management_ip
management_port
agent_version
engine
engine_version
os_name
os_version
kernel_version
architecture
timezone
status
health_score
registered_at
last_seen_at
agent_config_version (ADR-039: versão da config de comportamento que o Agent aplicou; NULL = nunca reportou)
agent_config_applied_at
agent_config_seeded_at (ADR-039 §7: a semeadura dos overrides é de uma vez só por Node)
created_at
updated_at
deleted_at
Restrições
Hostname único por Cluster.
Management IP único.
UUID único globalmente.
Índices
hostname
management_ip
status
health_score
last_seen_at
# **11. Estados do Node**
ONLINE
OFFLINE
DRAIN
MAINTENANCE
FAILED
UNKNOWN
DISABLED
Observação
O estado NÃO deverá ser calculado apenas pelo Heartbeat.
O Control Plane poderá alterar o estado manualmente.
# **12. Tabela node_labels**
Objetivo
Permitir classificação lógica dos Nodes.
Labels são criadas exclusivamente pelo operador.
Nunca automaticamente.
Campos
id
node_id
key
value
description
created_at
updated_at
Exemplos
country=br
energy=low
rack=01
tracker=movies
priority=gold
# **13. Tabela node_capabilities**
Objetivo
Registrar capacidades detectadas automaticamente pelo Agent.
Nunca editadas manualmente.
Campos
id
node_id
capability
value
source
detected_at
Exemplos
docker
true
smart
true
nvme
false
ups
true
usb3
true
arm64
true
qbittorrent
true
Essas informações serão utilizadas futuramente pelo Scheduler para tomada de decisão.
# **14. Tabela node_tags**
Diferença entre Labels e Tags
Labels
Definidas pelo operador.
Utilizadas em políticas.
Persistentes.
Tags
Geradas automaticamente.
Transitórias.
Informativas.
Exemplos
hot
high-upload
low-space
overloaded
new-node
cooling
As Tags poderão ser alteradas automaticamente pelo Control Plane.
# **15. Decisões Arquiteturais**
O banco de dados nunca armazenará métricas contínuas.
Exemplos:
CPU por segundo
RAM por segundo
Temperatura por segundo
Upload instantâneo
Download instantâneo
Essas informações pertencem ao Prometheus.
O PostgreSQL armazenará apenas estados relevantes, configurações, inventário e histórico operacional.
# **16. Domínio Inventory**
# **Objetivo**
O domínio Inventory representa todas as características físicas e lógicas de um Node.
Sua finalidade é permitir que o Control Plane possua conhecimento completo sobre cada equipamento do Cluster.
O inventário é coletado exclusivamente pelo Agent.
Nenhuma informação poderá ser alterada manualmente pelo operador, exceto quando explicitamente previsto em outro documento.
# **17. Tabela hardware**
Objetivo
Armazenar informações gerais sobre o equipamento.
Relacionamento
1:1 com nodes.
Campos
id
node_id
manufacturer
model
board_model
board_revision
serial_number
cpu_serial
firmware_version
bios_version
bootloader_version
boot_mode
power_supply
created_at
updated_at
Exemplos
Raspberry Pi 3B+
Raspberry Pi 4
Dell OptiPlex
Intel NUC
KVM VM
# **18. Tabela cpus**
Objetivo
Representar processadores instalados.
Relacionamento
1:N com nodes.
Campos
id
node_id
socket
vendor
model
architecture
cores
threads
base_frequency
max_frequency
cache_l1
cache_l2
cache_l3
virtualization_supported
aes_supported
neon_supported
current_governor
created_at
updated_at
Observação
O Agent deverá atualizar a frequência atual apenas através das métricas.
O banco armazena apenas as características da CPU.
# **19. Tabela memory_modules**
Objetivo
Representar módulos de memória.
Relacionamento
1:N com nodes.
Campos
id
node_id
slot
manufacturer
capacity
type
speed
serial
part_number
created_at
updated_at
Para Raspberry Pi, onde essas informações podem não existir, o Agent deverá preencher apenas os campos disponíveis.
# **20. Tabela network_interfaces**
Objetivo
Representar interfaces de rede.
Relacionamento
1:N com nodes.
Campos
id
node_id
name
type
mac_address
driver
speed
duplex
mtu
ipv4
ipv6
gateway
dns
dhcp
link_state
created_at
updated_at
Exemplos
eth0
wlan0
usb0
bond0
# **21. Tabela usb_devices**
Objetivo
Inventariar dispositivos USB conectados.
Relacionamento
1:N com nodes.
Campos
id
node_id
vendor
product
manufacturer
serial
device_class
usb_version
bus
port
speed
power_required
created_at
updated_at
Exemplos
HD Externo
SSD USB
Pendrive
Hub USB
Nobreak
Dongle Zigbee
# **22. Tabela disks**
Objetivo
Representar dispositivos físicos de armazenamento.
Relacionamento
1:N com nodes.
Campos
id
node_id
device_name
manufacturer
model
serial
type
connection
capacity
sector_size
firmware
rotation
trim_supported
smart_supported
created_at
updated_at
Tipos suportados
HDD
SSD
NVMe
USB Flash
RAM Disk
Loop Device
Conexões suportadas
USB
SATA
NVMe
MMC
SD Card
Virtual
# **23. Tabela partitions**
Objetivo
Representar partições de discos.
Relacionamento
1:N com disks.
Campos
id
disk_id
device
filesystem
label
uuid
size
mountpoint
bootable
created_at
updated_at
# **24. Tabela filesystems**
Objetivo
Representar sistemas de arquivos conhecidos.
Relacionamento
1:N com partitions.
Campos
id
partition_id
type
block_size
compression
encryption
case_sensitive
created_at
updated_at
Tipos esperados
ext4
xfs
btrfs
zfs
ntfs
fat32
exfat
# **25. Tabela mountpoints**
Objetivo
Representar pontos de montagem ativos.
Relacionamento
1:N com partitions.
Campos
id
partition_id
path
read_only
mounted
options
available_space
used_space
reserved_space
created_at
updated_at
Exemplo
/data
/downloads
/backup
/var/lib/docker
# **26. Tabela docker_info**
Objetivo
Representar a instalação do Docker em um Node.
Relacionamento
1:1 com nodes.
Campos
id
node_id
version
compose_version
storage_driver
cgroup_version
root_directory
logging_driver
swarm_enabled
created_at
updated_at
# **27. Tabela containers**
Objetivo
Inventariar containers executados.
Relacionamento
1:N com nodes.
Campos
id
node_id
container_name
image
tag
container_id
status
restart_policy
network
ports
created_at
updated_at
Observação
O SeedGrid deverá conhecer todos os containers existentes no Node, não apenas os próprios.
Isso permitirá detectar conflitos, consumo de recursos e facilitar diagnósticos.
# **28. Tabela services**
Objetivo
Representar serviços do sistema operacional.
Relacionamento
1:N com nodes.
Campos
id
node_id
service_name
status
enabled
version
created_at
updated_at
Exemplos
docker
ssh
seedgrid-agent
cron
systemd-resolved
# **29. Inventário Parcial**
Caso alguma informação não possa ser obtida pelo Agent, o registro deverá permanecer válido.
Campos desconhecidos deverão ser armazenados como NULL.
O Agent nunca deverá inventar valores.
# **30. Atualização do Inventário**
O inventário completo NÃO deverá ser enviado a cada Heartbeat.
O Agent deverá:
- enviar um inventário completo no primeiro registro;
- reenviar apenas quando detectar mudanças;
- permitir sincronização manual solicitada pelo Control Plane.
Essa estratégia reduz tráfego, escrita no banco e processamento.
# **31. Versionamento**
Cada atualização de inventário incrementará um número de versão.
Campos adicionais:
inventory_version
inventory_updated_at
Esses atributos pertencem à tabela nodes e permitem ao Control Plane identificar rapidamente se o inventário local está sincronizado com o último inventário conhecido.
# **32. Regras Gerais**
O inventário representa a configuração do Node.
Ele não representa seu estado momentâneo.
Alterações de utilização (CPU, memória, temperatura, rede) pertencem ao domínio de métricas e não devem ser persistidas neste conjunto de tabelas.
O objetivo é permitir reconstrução fiel da configuração de cada Node, auditoria de mudanças de hardware e suporte ao algoritmo de agendamento do Scheduler.
# **33. Domínio Torrent**
# **Objetivo**
O domínio Torrent representa todos os objetos relacionados à operação BitTorrent do Cluster.
O Control Plane nunca deverá depender diretamente da estrutura de dados do Engine.
O Agent será responsável pela tradução entre os dados do Engine e o modelo de dados do SeedGrid.
# **34. Conceitos**
Um Torrent representa um objeto lógico do Cluster.
Ele sempre pertence a um único Node.
Um Torrent pode possuir:
- múltiplos arquivos;
- múltiplos trackers;
- múltiplas tags;
- múltiplos eventos;
- múltiplos históricos;
- múltiplas estatísticas.
# **35. Tabela torrents**
Objetivo
Representar um Torrent conhecido pelo Cluster.
Relacionamento
N:1 com Nodes
Campos
id
uuid
node_id
hash
name
size
piece_size
piece_count
private
engine_id
engine_version
status
priority
category_id
created_by
created_at
updated_at
deleted_at
Observações
O campo hash representa o identificador universal do torrent.
O campo engine_id representa o identificador interno utilizado pelo Engine.
O Control Plane nunca utilizará engine_id como chave de negócio.
# **36. Estados do Torrent**
Os estados oficiais do SeedGrid serão:
QUEUED
DOWNLOADING
SEEDING
PAUSED
CHECKING
MOVING
ERROR
ARCHIVED
DELETED
UNKNOWN
O Agent será responsável por mapear os estados específicos do Engine para estes estados.
# **37. Tabela torrent_categories**
Objetivo
Agrupar torrents.
Campos
id
name
description
color
icon
created_at
updated_at
Exemplos
Movies
Series
Linux
Games
Backup
ISO
Freeleech
Temp
Observação
Categorias pertencem ao SeedGrid.
Não necessariamente refletem categorias existentes no Engine.
# **38. Tabela torrent_tags**
Objetivo
Permitir classificação flexível.
Campos
id
name
color
description
system
created_at
updated_at
Exemplos
4K
HDR
Anime
PT-BR
Seeder
Rare
Pinned
Important
Tags podem ser:
Criadas manualmente.
Criadas automaticamente.
# **39. Tabela torrent_tag_relations**
Relacionamento N:N
Campos
torrent_id
tag_id
created_at
# **40. Tabela torrent_trackers**
Objetivo
Representar trackers associados ao Torrent.
Campos
id
torrent_id
tracker_url
tier
status
last_announce
last_scrape
last_error
message
created_at
updated_at
Observação
O SeedGrid poderá consolidar informações de diversos Engines.
# **41. Tabela torrent_files**
Objetivo
Representar arquivos pertencentes ao Torrent.
Campos
id
torrent_id
path
filename
extension
size
priority
selected
completed
checksum
created_at
updated_at
Observação
Essas informações permitirão futuras funcionalidades como:
visualização de conteúdo;
seleção individual de arquivos;
migração parcial.
# **42. Tabela torrent_statistics**
Objetivo
Armazenar estatísticas persistentes.
Campos
id
torrent_id
uploaded_bytes
downloaded_bytes
ratio
completed_bytes
availability
connected_peers
connected_seeds
share_time
active_time
last_activity
created_at
updated_at
Importante
A tabela NÃO armazenará histórico temporal.
Ela representa apenas o estado atual conhecido.
# **43. Tabela torrent_history**
Objetivo
Registrar mudanças importantes.
Campos
id
torrent_id
event
old_value
new_value
performed_by
created_at
Exemplos
Torrent criado
Torrent pausado
Torrent retomado
Mudança de categoria
Mudança de prioridade
Migração
Recheck
Erro
# **44. Tabela torrent_locations**
Objetivo
Representar localização física.
Campos
id
torrent_id
disk_id
partition_id
mountpoint
relative_path
absolute_path
created_at
updated_at
Importância
Nunca depender do caminho configurado no Engine.
Sempre conhecer exatamente onde os dados estão.
# **45. Tabela torrent_scheduler**
Objetivo
Registrar decisões do Scheduler.
Campos
id
torrent_id
selected_node
decision_score
decision_reason
algorithm_version
created_at
Exemplo
Node escolhido
seedgrid-node-02
Score
94
Razão
Maior espaço disponível
Essas informações serão extremamente úteis para auditoria.
# **46. Tabela torrent_operations**
Objetivo
Registrar operações administrativas.
Campos
id
torrent_id
operation
requested_by
requested_at
started_at
finished_at
status
error_message
Exemplos
Pause
Resume
Delete
Move
Recheck
Relocate
Export
Import
# **47. Tabela torrent_queue**
Objetivo
Representar fila interna de operações.
Campos
id
torrent_id
operation
priority
status
retry_count
scheduled_at
started_at
finished_at
Observação
O Control Plane nunca executará operações diretamente.
Toda ação passará pela fila.
# **48. Regras de Integridade**
Não poderá existir Torrent sem Node.
Não poderá existir arquivo sem Torrent.
Não poderá existir Tracker sem Torrent.
Não poderá existir estatística sem Torrent.
Não poderá existir operação sem Torrent.
# **49. Exclusão**
Ao remover um Torrent do Cluster:
O registro principal será marcado como deleted_at.
O histórico será preservado.
As operações permanecerão registradas.
As estatísticas poderão ser arquivadas.
Nenhum registro será removido fisicamente por padrão.
# **50. Sincronização**
O Agent enviará ao Control Plane:
Estado atual.
Alterações.
Novos Torrents.
Remoções.
Mudanças de Tracker.
Mudanças de localização.
Mudanças de estatísticas.
O Control Plane será sempre a fonte oficial do estado persistente.
# **51. Modelo Conceitual**
Node
│
├── Torrents
│      │
│      ├── Files
│      ├── Trackers
│      ├── Tags
│      ├── Statistics
│      ├── Operations
│      ├── Queue
│      ├── History
│      └── Locations
│
└── Scheduler Decisions
# **52. Princípios**
O SeedGrid representa Torrents como objetos do Cluster.
O Engine representa Torrents como objetos locais.
Esses dois modelos nunca deverão ser acoplados diretamente.
Toda tradução entre ambos será responsabilidade exclusiva do Agent.
# **52-A. Perfil de Tracker (tracker_profiles)**
Objetivo
Guardar o conhecimento que é **do site** de um tracker e não do operador (ADR-037, migration 00043).
Hoje é a taxonomia — os IDs de `category`, `type` e `resolution`, que cada admin configura no seu
site (`Encode` é 3 no samaritano e 12 no Blutopia), a **composição da descrição** do upload (#192: quais blocos
entram, em que ordem, e a tag do MediaInfo) e o **host de imagem** do site (#193, ADR-040: a Spec de forma
fechada que descreve o host próprio do tracker para a cadeia de upload dos screenshots). Formato de nome e
regras entram como seções novas do mesmo documento (#191), sem remodelar a tabela.
As três camadas da ADR-037 e onde cada uma mora
- Família (shape da API, auth, multipart): código Go (`internal/unit3d`). Não está no banco.
- Perfil do site: **esta tabela**. Exportável em JSON (#197, entregue) — quem tem conta no site descobre, ajusta e compartilha.
- Config do operador (`api_token`, announce com passkey, `image_host_key`/`image_host_key_host` — migration 00045, #193 / ADR-040 —, e no futuro `anon`/`modq`): tabela `trackers`. **Nunca exportada.** `image_host_key` é a chave do operador para o host de imagem próprio do tracker (segredo, nunca devolvida na API); `image_host_key_host` é o host DNS para o qual a chave foi cadastrada — se o `upload_url` do perfil mudar de host, a chave antiga não serve mais.
Por que é uma tabela à parte de `trackers`
O export da #197 lê só desta tabela (e, de `trackers`, o nome e o host — nunca announce, `api_url`, token nem `source`). `tracker_profiles` **não tem coluna de credencial nem de config
do operador**, de propósito: a camada 3 fica fora por construção, e não por um filtro de serialização
que alguém precise lembrar. O mesmo princípio que já vale para `trackers.announce` e `trackers.api_token`,
que nunca saem na API. Testes guardam isso: um percorre as colunas da tabela e a árvore de tipos
do perfil atrás de nomes de credencial; outro cria um tracker com passkey e token e confere que nada
disso aparece em nenhuma leitura do perfil; e, no export, um terceiro percorre a árvore de tipos do
arquivo (campo de credencial, campo opaco, `MarshalJSON` próprio) e um quarto exporta um tracker com tudo
preenchido e procura os **valores** sensíveis no texto do arquivo.
Campos
id (UUID)
tracker_id (FK trackers — um perfil **vivo** por tracker, índice único parcial `WHERE deleted_at IS NULL`)
family (TEXT, default `unit3d`; validada em Go — família nova é código novo)
schema_version (INTEGER ≥ 1, a versão do **formato** do perfil: o que o export carimba e o import confere)
taxonomy (JSONB, sempre um objeto — `CHECK jsonb_typeof(taxonomy) = 'object'`; é o **documento inteiro** do perfil: os eixos na raiz e, ao lado deles, uma chave reservada por seção — hoje `description` e `image_host`)
created_at
updated_at
deleted_at
Forma de `taxonomy`
`{"category": [{"id": 1, "name": "Filmes", "kind": "movie", "origin": "seed"}], "type": […], "resolution": […]}`
- `origin`: `seed` (migration) | `discovered` (descoberta no catálogo do site) | `manual` (o operador digitou ou renomeou) | `imported` (veio de um arquivo de perfil, #197). Decide quem pode mudar o **nome**: a descoberta e o import nunca sobrescrevem `manual` — nem, o import, `discovered` sem pedido; uma origem desconhecida é tratada como protegida (falha fechada). É estado **local** — não faz parte do arquivo de troca; o import atribui `imported` ao que escreve. Sem `CHECK` no banco (mora dentro do JSONB): o valor é validado em Go, e `imported` **não exigiu migration**.
- `description` (#192): `{"media_blocks": ["poster", "synopsis", "tags", "screenshots", "mediainfo"], "game_blocks": ["cover", "title", "about", "genres", "platforms"], "media_info_tag": "mediainfo"}` — o que está escrito é o **default**. Ids de bloco de um conjunto fechado, por tipo, sem repetição; `media_blocks: []` é "nenhum bloco" (não o default); a tag é `mediainfo`, `code`, `pre` ou `spoiler`. Chave **ausente** = o default. **Sem migration e sem coluna nova:** é uma chave a mais no documento que já existe, e a linha anterior à #192 é, byte a byte, um documento sem ela. Um eixo nunca se chama `description` (o conjunto de eixos é fechado), então a chave não colide. A leitura é estrita (tipo errado é erro, não "vazio"); a escrita valida tudo em `internal/trackerprofile` e a regrava **junto** com a taxonomia sob o mesmo `FOR UPDATE`. Contrapartida: um Control Plane anterior à #192 não lê o perfil de um tracker com a composição personalizada. `language` opcional (#74): `en` ou `pt-BR`, omitido quando é inglês.
- `image_host` (#193 / ADR-040): `{"name": "Host do site", "upload_url": "https://img.tracker.exemplo.org/api/v1/sharex/upload", "file_field": "file", "auth_header": "Authorization", "auth_prefix": "Bearer ", "url_path": "url", "thumbnail_path": "thumbnail_url"}` — a Spec de forma **fechada** do host de imagem próprio do tracker. `name` com as regras de nome (1 a 64 caracteres, sem controle nem formatação invisível); `upload_url` absoluta, `https`, host DNS (sem IP literal, sem `localhost`, sem porta, sem userinfo, sem fragmento), até 2048 caracteres; `file_field` `[A-Za-z0-9_.-]{1,64}`; `auth_header` token de header `[A-Za-z0-9-]{1,64}`, fora de `Host`, `Content-Type`, `Content-Length`, `Transfer-Encoding`, `Connection`, `Cookie`, `Upgrade`, `TE`, `Trailer` e `Proxy-*` (sem diferenciar maiúsculas); `auth_prefix` 0 a 32 caracteres ASCII imprimíveis; `url_path` 1 a 5 segmentos `[A-Za-z0-9_]{1,64}` separados por ponto; `thumbnail_path` opcional, mesmas regras. Chave **ausente** = sem host próprio (só o imgbb global na cadeia de screenshots). **Sem migration e sem coluna nova:** é uma chave a mais no documento que já existe, e a linha anterior à #193 é, byte a byte, um documento sem ela. Um eixo nunca se chama `image_host` (o conjunto de eixos é fechado), então a chave não colide. A leitura é estrita (tipo errado é erro, não "vazio"); a escrita valida tudo em `internal/trackerprofile` e a regrava **junto** com a taxonomia sob o mesmo `FOR UPDATE`. A **chave do operador** para o host é camada 3 (tabela `trackers`, #193 / ADR-040) e nunca mora aqui. Contrapartida: um Control Plane anterior à #193 não lê o perfil de um tracker com host de imagem configurado.
- `kind`: `movie` | `tv` | `game`, só em `category`. O papel semântico que o Portal usava deduzindo do número (`isTV = id 2 ou 3`, `isGame = id 5`) — que só é verdade num site.
- Por eixo: `id` em `1..1000000` e único; nome com 1 a 64 caracteres sem controle nem formatação invisível; no máximo 256 termos; ordenado por `id`.
Por que JSONB e não uma tabela por termo
- A unidade de export, import e validação é o **documento**: uma coluna por seção é o formato do arquivo contribuído, sem montar e desmontar linhas.
- Eixo novo (áudio, idioma, tag, flag) e seção nova (regras, image host) entram sem migration de tabela — só com `schema_version`.
- A taxonomia nunca é consultada por conteúdo ("quais trackers têm a categoria 5?"): é lida inteira, são poucas dezenas de termos, e o Portal a edita como um todo.
- O que o relacional daria de graça (id único por eixo, faixa, tamanho) fica em `internal/trackerprofile`, em **todo** ponto de escrita, e a escrita é serializada por `SELECT … FOR UPDATE` na linha do perfil — leitura-modificação-escrita sem trava perderia aplicações concorrentes (há teste com 8 aplicações simultâneas).
Regra de convivência entre descoberta e edição manual
- Descoberta só **propõe**; aplicar é outra operação, auditada (ADR-025).
- Id ausente do perfil → adiciona (`discovered`). Mesmo nome → nada. Nome diferente → renomeia **só se a origem não for `manual`**. O que o catálogo não mostrou fica intocado: ausência não é evidência, o catálogo só mostra o que alguém postou.
- Editar à mão (`PUT`) marca como `manual` o que for criado ou renomeado; mudar só o `kind` não troca a origem, porque a descoberta nunca mexe em `kind`.
Migração do samaritano (sem o operador reconfigurar nada)
A 00043 cria a tabela vazia: nenhum tracker ganha perfil por migration. O operador cujo site já tem perfil no repositório importa o arquivo (`profiles/<família>/<site>.json`, ADR-037); os demais rodam "Descobrir taxonomia". A migration é testada contra um banco descartável: sobe até a 00041 com um tracker cadastrado, aplica o resto e confere que nenhum perfil foi criado, que um tracker tem no máximo um perfil vivo e que o down não deixa resto.
Observações
- A linha é criada sob demanda (primeira edição ou descoberta); tracker sem linha lê-se como perfil vazio.
- Soft delete: remover o tracker faz soft delete do perfil (best-effort — toda leitura confere posse e existência do tracker, então um perfil órfão é inerte).
- Perfil é **conteúdo não confiável** (catálogo de terceiro, edição e arquivo importado): validação de tipos e faixas na entrada, nada executável, nenhum campo livre indo para requisição.
- **Export/import (#197):** o arquivo é o `Document` (envelope + `Profile`), sem campo de credencial por construção; o import escreve na mesma linha, sob o mesmo `FOR UPDATE`, e a regra de convivência (`manual` nunca é sobrescrito sem pedido; o import nunca remove) roda sobre a linha travada. Formato e validação: ADR-037 e `docs/13 - API.md`. `schema_version` é a versão do **formato** — sobe quando um campo muda de forma ou entra uma seção nova **obrigatória**; uma seção opcional e aditiva, que o export omite no default (a `description`, #192), não sobe (a coluna guarda a da linha; o arquivo carimba a que o código emite).
# **53. Domínio Identity**
# **Objetivo**
O domínio Identity é responsável por autenticação, autorização e gerenciamento de identidade dos operadores do SeedGrid.
Este domínio não possui qualquer responsabilidade sobre autenticação entre Nodes e Control Plane.
Essa comunicação será documentada em **Authentication.md**.
# **54. Tabela users**
Objetivo
Representar usuários da plataforma.
Campos
id
uuid
username
display_name
email
password_hash
status
last_login_at
created_at
updated_at
deleted_at
Status
ACTIVE
DISABLED
LOCKED
PENDING
Observações
Username deverá ser único.
E-mail deverá ser único.
Passwords nunca poderão ser armazenados em texto plano.
# **55. Tabela roles**
Objetivo
Representar papéis de acesso.
Campos
id
name
description
system
created_at
updated_at
Papéis padrão
Administrator
Operator
ReadOnly
# **56. Tabela permissions**
Objetivo
Representar permissões individuais.
Campos
id
resource
action
description
created_at
Exemplos
nodes.read
nodes.write
nodes.delete
torrents.read
torrents.write
scheduler.execute
alerts.read
cluster.admin
# **57. Tabela role_permissions**
Relacionamento
N:N
Campos
role_id
permission_id
created_at
# **58. Tabela user_roles**
Relacionamento
N:N
Campos
user_id
role_id
created_at
# **59. Tabela sessions**
Objetivo
Representar sessões autenticadas.
Campos
id
user_id
session_token
ip_address
user_agent
expires_at
last_activity
created_at
Observação
Sessões expiradas poderão ser removidas automaticamente.
# **60. Domínio Monitoring**
# **61. Tabela heartbeats**
Objetivo
Registrar o último heartbeat recebido.
Importante
Esta tabela NÃO possui histórico.
Ela representa apenas o último estado conhecido.
Campos
id
node_id
received_at
agent_version
engine_version
status
uptime
health_score
created_at
updated_at
# **62. Tabela health_scores**
Objetivo
Representar a saúde atual de um Node.
Campos
id
node_id
overall_score
cpu_score
memory_score
storage_score
network_score
temperature_score
engine_score
calculated_at
Valores
0
até
100
# **63. Domínio Events**
Objetivo
Representar acontecimentos relevantes.
Todo componente deverá gerar eventos.
# **64. Tabela event_types**
Objetivo
Catalogar tipos de eventos.
Campos
id
code
name
severity
description
Exemplos
NODE_ONLINE
NODE_OFFLINE
TORRENT_CREATED
TORRENT_DELETED
SMART_WARNING
SMART_FAILURE
BACKUP_STARTED
BACKUP_FINISHED
# **65. Tabela events**
Objetivo
Registrar eventos.
Campos
id
event_type_id
node_id
torrent_id
user_id
message
payload
severity
created_at
Observações
Payload será armazenado em JSONB.
Permitirá evolução sem migrations.
# **66. Tabela event_queue**
Objetivo
Fila interna de processamento.
Campos
id
event_id
status
attempts
last_attempt
processed_at
created_at
Status
PENDING
PROCESSING
DONE
FAILED
# **67. Domínio Alerting**
# **68. Tabela alerts**
Objetivo
Representar alertas ativos.
Campos
id
node_id
torrent_id
severity
title
description
status
created_at
resolved_at
Status
OPEN
ACKNOWLEDGED
RESOLVED
CLOSED
Severidades
INFO
WARNING
ERROR
CRITICAL
# **69. Tabela notifications**
Objetivo
Registrar notificações enviadas.
Campos
id
alert_id
channel
destination
status
sent_at
response
Exemplos
Telegram
Discord
Google Chat
Webhook
Email
# **70. Domínio Configuration**
# **71. Tabela settings**
Objetivo
Armazenar os overrides dos tunáveis de comportamento (ADR-023), sobrepondo os defaults do
catálogo de código. KV puro: a metadata (tipo, default, descrição, unidade, limites, categoria)
vive no catálogo (`internal/settings/catalog.go`), NÃO no banco.
Campos
key (PK, namespaced — ex.: admission.dead_grace)
value (string; parse pelo tipo do catálogo)
updated_at
updated_by (operador; 'env-seed' no seed de boot)
Semântica
- Override do default; remover a linha = reset ao padrão.
- Precedência: o banco manda; o env var só semeia no 1º boot (Store.Load).
- Aplicado em runtime pelo Store (hot-reload). Ver `25 - Business Rules.md`.
Exemplos
admission.dead_grace = 3m
alerting.disk_saturated_pct = 90
workers.reconciler_interval = 15s
# **71.1 Tabela node_settings**
Objetivo
Override POR NODE dos tunáveis de comportamento do Agent/Engine (ADR-039, #144), sobre o valor
de cluster da tabela `settings`. Só as chaves `PerNode` do catálogo (`agent.heartbeat_interval`,
`agent.command_poll_interval`, `agent.log_level`, `engine.*`) entram aqui.
Campos
id (UUID PK)
node_id (FK → nodes, ON DELETE CASCADE)
key (namespaced, igual à `settings`)
value (string; parse pelo tipo do catálogo)
created_at
updated_at
updated_by (operador; 'semeadura (env do Node)' quando vem da semeadura)
Restrições
UNIQUE (node_id, key)
Semântica
- Valor efetivo de um Node = override do Node > valor de cluster (`settings`/default do catálogo).
  O env do Node e o default do código são o fallback do Agent, só quando o CP não manda nada.
- Validado pelo MESMO `Spec` do catálogo (tipo, faixa, opções) — não há segunda régua.
- Remover a linha devolve o Node ao valor de cluster. Sem soft delete, como a `settings`:
  o histórico (antes e depois) fica na Auditoria (ADR-025).
- Entregue ao Agent na resposta do heartbeat (`agent_config`). Ver `13 - API.md` e `25 - Business Rules.md`.
Exemplos
(node-04, Atom) engine.max_active_downloads = 3 — o valor de cluster é 10
# **72. Tabela feature_flags**
Objetivo
Controlar funcionalidades experimentais.
Campos
id
name
enabled
description
created_at
updated_at
Exemplos
scheduler_v2
agent_auto_update
experimental_api
# **73. Tabela system_variables**
Objetivo
Armazenar variáveis internas.
Campos
id
key
value
updated_at
Exemplos
schema_version
cluster_uuid
last_backup
database_version
# **74. Regras Gerais**
Usuários nunca pertencem aos Nodes.
Eventos nunca poderão ser excluídos automaticamente.
Alertas resolvidos permanecerão disponíveis para auditoria.
Settings sempre serão globais (valor de cluster). A única exceção é o override por Node dos
tunáveis de comportamento do Agent/Engine (`node_settings`, ADR-039), que nunca substitui o
valor de cluster: só o sobrepõe, para aquele Node, e volta a ele ao ser removido.
Feature Flags nunca substituirão configurações.
Variáveis do sistema nunca serão editadas pelo operador.
# **75. Modelo Conceitual**
Users
│
├── Roles
│
├── Sessions
│
└── Permissions
Nodes
│
├── Heartbeats
├── Health Scores
├── Alerts
└── Events
Cluster
│
├── Settings
├── Feature Flags
└── System Variables
# **76. Princípios**
Eventos representam fatos ocorridos.
Alertas representam problemas ativos.
Configurações representam comportamento esperado.
Variáveis representam estado interno.
Esses quatro conceitos nunca deverão ser confundidos.
O modelo de dados deverá preservar essa separação ao longo de toda a evolução do projeto.
# **77. Auditoria**
# **Objetivo**
Garantir rastreabilidade completa das operações realizadas no Cluster.
Toda alteração relevante deverá ser auditada.
A auditoria deverá permitir reconstruir quem executou determinada ação, quando ocorreu e qual foi seu impacto.
# **78. Tabela audit_logs**
Objetivo
Registrar todas as ações administrativas.
Campos
id
user_id
node_id
torrent_id
resource
resource_id
action
ip_address
user_agent
request_id
status
created_at
Exemplos
CREATE_NODE
UPDATE_NODE
DELETE_NODE
MOVE_TORRENT
RECHECK_TORRENT
UPDATE_SETTINGS
LOGIN
LOGOUT
# **79. Tabela audit_changes**
Objetivo
Registrar alterações de dados.
Relacionamento
N:1 com audit_logs.
Campos
id
audit_log_id
field_name
old_value
new_value
created_at
Observação
Valores complexos deverão ser armazenados em JSONB.
# **80. Integridade Referencial**
Todas as Foreign Keys deverão possuir restrições explícitas.
Nunca utilizar ON DELETE CASCADE para entidades críticas.
Exemplos de entidades críticas:
nodes
torrents
users
events
alerts
Exclusões deverão ser tratadas pela aplicação.
# **81. Soft Delete**
Todas as entidades persistentes deverão utilizar Soft Delete.
Campos obrigatórios
deleted_at
deleted_by
Entidades elegíveis
Nodes
Torrents
Users
Trackers
Perfis de Tracker
Categorias
Tags
Settings
Entidades não elegíveis
Heartbeats
Health Scores
Sessions
Event Queue
Feature Flags
System Variables
# **82. Índices**
Todos os campos utilizados em pesquisa deverão possuir índice.
Índices obrigatórios
nodes.hostname
nodes.status
nodes.last_seen_at
torrents.hash
torrents.status
torrents.name
alerts.status
alerts.severity
events.created_at
users.username
users.email
Campos JSONB utilizados em filtros deverão possuir índices GIN.
# **83. Pesquisa**
O banco deverá suportar pesquisa textual eficiente.
Campos elegíveis
torrent.name
node.display_name
tracker.name
event.message
alert.title
alert.description
A implementação deverá utilizar PostgreSQL Full Text Search ou índices pg_trgm, conforme o caso.
# **84. Versionamento do Schema**
Toda alteração estrutural deverá ocorrer exclusivamente por migrations.
Cada migration deverá possuir:
identificador único
descrição
timestamp
rollback quando aplicável
Nunca alterar tabelas manualmente em produção.
# **85. Retenção**
O sistema deverá permitir políticas de retenção configuráveis.
Valores padrão recomendados
Audit Logs: 365 dias
Events: 180 dias
Notifications: 90 dias
Sessions expiradas: 30 dias
Heartbeats: manter apenas o último registro
Health Scores: manter apenas o último registro
Esses valores deverão ser configuráveis.
# **86. Backup**
O banco deverá suportar backup consistente sem interrupção do serviço.
O backup deverá incluir:
Schema
Dados
Índices
Constraints
Extensions
Não deverão ser armazenados no banco:
Arquivos .torrent
FastResume
Logs
Métricas
# **87. Restore**
O restore deverá permitir reconstrução completa do estado do Control Plane.
Após a restauração, os Agents deverão sincronizar automaticamente seus estados com o banco.
Nenhuma intervenção manual deverá ser necessária para reimportação de inventário ou identificação dos Nodes.
# **88. Performance**
Objetivos mínimos
Pesquisa de Torrent por hash: \< 50 ms
Pesquisa por nome: \< 200 ms
Listagem de Nodes: \< 100 ms
Dashboard principal: \< 500 ms
Registro de evento: \< 20 ms
Esses valores representam metas de projeto e poderão ser revisados em futuras versões.
# **89. Escalabilidade**
O modelo de dados deverá suportar, no mínimo:
100 Nodes
100.000 Torrents
10 milhões de eventos
50 usuários
1 milhão de registros de auditoria
Sem necessidade de alterações estruturais.
# **90. Consistência**
O PostgreSQL será considerado a única fonte de verdade (Source of Truth) para:
Configuração
Inventário
Estado do Cluster
Estado dos Torrents
Usuários
Eventos
Alertas
Auditoria
Nenhum componente poderá manter estado persistente paralelo para essas entidades.
# **91. Convenções Gerais**
Todos os horários deverão ser armazenados em UTC.
Todos os identificadores deverão utilizar UUID.
Todas as operações críticas deverão ocorrer dentro de transações.
Nenhuma regra de negócio deverá depender exclusivamente de triggers do banco.
A lógica de negócio pertence ao Control Plane.
# **92. Evolução**
Novas entidades deverão seguir os mesmos padrões estabelecidos neste documento.
É proibido criar tabelas "temporárias" para funcionalidades permanentes.
Toda nova entidade deverá:
- possuir responsabilidade claramente definida;
- respeitar as convenções de nomenclatura;
- utilizar auditoria quando aplicável;
- possuir estratégia de backup e retenção.
# **93. Dependências**
Os documentos abaixo dependem diretamente desta especificação:
04 - Infrastructure.md
05 - Network.md
06 - Storage.md
07 - Workers.md
08 - Agent.md
09 - Control Plane.md
10 - Scheduler.md
11 - Portal.md
13 - API.md
14 - Authentication.md
15 - Monitoring.md
16 - Alerting.md
17 - Backup.md
18 - Disaster Recovery.md
Engine.md
# **94. Resumo do Modelo**
Domínios implementados
✓ Identity
✓ Cluster
✓ Nodes
✓ Inventory
✓ Storage
✓ Torrent
✓ Perfil de Tracker (ADR-037)
✓ Monitoring
✓ Events
✓ Alerting
✓ Configuration
✓ Audit
# **95. Declaração Final**
O banco de dados do SeedGrid representa o estado persistente do Cluster.
Ele não é um mecanismo de execução, monitoramento em tempo real ou processamento de comandos.
Sua responsabilidade é garantir consistência, rastreabilidade e integridade das informações necessárias para que o Control Plane opere de forma previsível, auditável e escalável.
Toda evolução futura do SeedGrid deverá preservar os princípios estabelecidos neste documento.
