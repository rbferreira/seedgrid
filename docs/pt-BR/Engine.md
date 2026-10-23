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
- Transmission Adapter (futuro);
- Deluge Adapter (futuro).
## 6.1 Versões suportadas do qBittorrent (ADR-043)
Todo Node roda o qBittorrent **5.0 ou superior**. Os Nodes que o SeedGrid instala usam a versão 5.x. Um Node que entra com o qBittorrent já existente também deve atender a esse requisito. A versão 5.0 é a primeira a incluir o criador de torrents (`torrentcreator/*`), necessário para publicação (ADR-019). Ela também traz os endpoints `torrents/stop` e `torrents/start`, usados pelo adaptador. O bootstrap rejeita a versão 4.x e solicita a atualização. Um Node incapaz de publicar criaria um segundo tipo de Node.
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
