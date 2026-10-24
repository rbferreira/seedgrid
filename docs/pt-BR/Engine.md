# **SeedGrid**
# **Torrent Engine Specification**
Version: 1.0
Status: Draft
Document: Engine.md
# **1. Objetivo**
Este documento define o papel do Torrent Engine na arquitetura do SeedGrid e o contrato de adaptação (Engine Adapter) que permite que a plataforma permaneça independente de um cliente BitTorrent específico.
# **2. Definição**
O Torrent Engine é a aplicação responsável pela execução do protocolo BitTorrent em um Seed Node.
Sua responsabilidade limita-se às operações relacionadas aos torrents.
Na versão 1.0 o Engine oficial é o qBittorrent.
# **3. Filosofia (Engine Agnostic)**
O SeedGrid não depende de um cliente BitTorrent específico.
O Engine é um detalhe de implementação do Seed Node.
Nenhum componente do Cluster (Control Plane, Scheduler, Portal, API) deverá depender diretamente de APIs específicas do Engine.
Toda dependência do Engine permanece isolada no Agent, através da camada de adaptação.
# **4. Responsabilidades**
O Torrent Engine deverá:
- executar o protocolo BitTorrent;
- baixar e semear torrents;
- manter o estado local dos torrents;
- expor operações (adicionar, remover, pausar, retomar, recheck) e estatísticas ao Agent.
O Torrent Engine NÃO deverá:
- comunicar-se diretamente com o Control Plane;
- tomar decisões de Scheduler;
- ser acessado diretamente pelo Portal ou pela API.
# **5. Relação com o Agent**
O Torrent Engine é considerado um componente externo ao Agent.
Toda interação entre o SeedGrid e o Engine ocorrerá exclusivamente através do Torrent Manager do Agent (ver Agent.md §21, §32).
O restante do Cluster nunca comunicará diretamente com o Engine.
A dependência da WebUI do Engine para operações normais é considerada anti-pattern (Architecture.md §21).
# **6. Engine Adapter**
O Agent deverá possuir uma camada de adaptação para o Engine.
Cada Engine suportado deverá possuir seu próprio adaptador, expondo a mesma interface interna para o restante do Agent.
Exemplos:
- qBittorrent Adapter (V1);
- rTorrent Adapter (ADR-045);
- Transmission Adapter (futuro);
- Deluge Adapter (futuro).
## 6.1 Versões suportadas do qBittorrent (ADR-043)
Todo Node roda o qBittorrent **5.0 ou superior**. Os Nodes que o SeedGrid instala usam a versão 5.x. Um Node que entra com o qBittorrent já existente também deve atender a esse requisito. A versão 5.0 é a primeira a incluir o criador de torrents (`torrentcreator/*`), necessário para publicação (ADR-019). Ela também traz os endpoints `torrents/stop` e `torrents/start`, usados pelo adaptador. O bootstrap rejeita a versão 4.x e solicita a atualização. Um Node incapaz de publicar criaria um segundo tipo de Node.
## 6.2 rTorrent (ADR-045)
Um Node pode rodar rTorrent **0.9.8 ou superior**, entrando com o rTorrent que a máquina já roda (`bootstrap-node.sh ENGINE=rtorrent`, Deployment §50.8). O SeedGrid não o instala. O adaptador se comunica via XML-RPC do rTorrent, sobre SCGI (um socket Unix ou `host:port`) ou pelo `/RPC2` de um servidor web, usando seu próprio codec leve. O ruTorrent, se presente, continua funcionando normalmente, e o SeedGrid não depende dele.

Um Node com rTorrent é como qualquer outro Node: mesmos estados, configurações, publicação, migração e Portal. O que falta no rTorrent, o Agent fornece:
- **A fila de downloads.** O rTorrent inicia todos os torrents que possui. O Agent aplica as configurações `engine.*` por conta própria, a cada poucos segundos: os mais antigos primeiro, com limites de downloads, uploads e torrents ativos. Com `dont_count_slow`, um torrent lento deixa de contar para esses limites. Ele marca o que para (chave `d.custom` `seedgrid`) e só inicia o que marcou: um torrent parado pelo operador ou pela admissão permanece parado. Com a fila ativa, um torrent adicionado via `.torrent` entra parado e enfileirado. O `max_active_checking` não tem efeito limitador: o rTorrent verifica apenas um torrent por vez. A fila só atua após a chegada das configurações do Control Plane.
- **O criador de torrents.** Para publicação, o Agent cria o `.torrent` (BitTorrent v1, tamanho de peça equivalente ao do libtorrent, flag private e `source` conforme solicitado) e o carrega apontando para os dados; o rTorrent o verifica e faz o seed.
- **A exportação do `.torrent`.** O Agent o lê da pasta de sessão do rTorrent e remove as chaves adicionadas por ele (`rtorrent`, `libtorrent_resume`), recuperando os bytes originais e o info hash.

Os estados vêm de vários campos: `d.hashing` definido é CHECKING; uma `d.message` de qualquer origem que não seja tracker (erro de armazenamento, falha na verificação de hash) é ERROR; parado ou pausado é PAUSED, exceto se a fila do Agent tiver parado o torrent, caso em que será QUEUED se incompleto e SEEDING se completo; iniciado é SEEDING quando completo e DOWNLOADING caso contrário. A resposta do tracker é o veredito do tracker, não o estado do torrent, assim como no qBittorrent. O rTorrent não informa sua conectabilidade de entrada: o Node não reporta nada (desconhecido, ADR-034) e conta peers de entrada pela lista de peers.

O adaptador lida com duas diferenças entre versões: o rTorrent 0.16 renomeou `network.port_range`/`network.port_random` para `network.listen.port.range`/`network.listen.port.random` (o adaptador tenta o nome novo e depois o antigo), e distingue XML-RPC de JSON-RPC pelo content type, sempre enviado pelo adaptador. O rTorrent responde 0 ao carregar um torrent mesmo quando o rejeita (a versão 0.9.8 rejeita torrents sem tracker), então o adaptador confirma cada `.torrent` carregado.
# **7. Contrato do Adapter**
Todo Engine Adapter deverá expor, no mínimo, as seguintes operações ao Torrent Manager:
- adicionar torrent (arquivo .torrent ou magnet). Quando a origem for um arquivo `.torrent`, o adapter deverá repassar os **bytes originais** ao Engine (não reduzir a um magnet/infohash): isso preserva os metadados — evitando que o Engine precise buscá-los na rede (estado `metaDL`) — e honra a flag `private` do torrent, indispensável para trackers privados, onde não há DHT para recuperar metadados;
- remover torrent (com ou sem dados);
- pausar torrent;
- retomar torrent;
- executar recheck;
- consultar estado, arquivos, trackers e estatísticas;
- reportar disponibilidade do Engine.
O adapter é responsável por traduzir esse contrato para a API específica do Engine.
# **8. Mapeamento de Estados**
O adapter deverá mapear os estados específicos do Engine para os estados oficiais do SeedGrid (Database.md §36):
QUEUED, DOWNLOADING, SEEDING, PAUSED, CHECKING, MOVING, ERROR, ARCHIVED, DELETED, UNKNOWN.
O modelo de dados do SeedGrid nunca deverá acoplar-se ao modelo de dados do Engine.
# **9. Independência**
A substituição do Engine (ex: qBittorrent → Transmission → Deluge) não deverá exigir alterações no Control Plane, no Scheduler, no Portal ou na API.
Apenas um novo adapter deverá ser implementado, respeitando o contrato definido neste documento.
# **10. Relação com os Demais Documentos**
Este documento complementa:
- Vision.md (§5.2 Engine Agnostic);
- Architecture.md;
- Agent.md (Torrent Manager e Engine Adapters);
- Database.md (domínio Torrent e estados);
- Scheduler.md.
Sempre que houver dúvida sobre o papel do Torrent Engine ou sobre o contrato de adaptação, este documento deverá ser considerado a referência oficial.
# **11. Declaração Final**
O Torrent Engine fornece a capacidade BitTorrent do Seed Node, permanecendo um detalhe de implementação substituível.
Ao concentrar toda dependência do Engine na camada de adaptação do Agent, o SeedGrid preserva seu princípio de independência de Engine e mantém a arquitetura desacoplada e evolutiva.
