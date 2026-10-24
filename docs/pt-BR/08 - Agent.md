# **SeedGrid**
# **Agent Specification**
Version: 1.0
Status: Draft
Document: 08 - Agent.md
# **1. Objetivo**
O SeedGrid Agent é o componente responsável por representar um Node perante o Cluster.
Todo Node participante do SeedGrid deverá possuir exatamente uma instância do Agent em execução.
O Agent é responsável por abstrair o sistema operacional, o hardware e o Torrent Engine, oferecendo ao Control Plane uma interface única e padronizada.
# **2. Filosofia**
O Agent representa o Node.
O Control Plane nunca deverá acessar diretamente:
- Sistema Operacional
- Docker
- Engine
- Discos
- Interfaces de rede
- SMART
- Arquivos
Toda interação deverá ocorrer através do Agent.
# **3. Responsabilidades**
O Agent deverá ser responsável por:
Registrar o Node.
Autenticar o Node.
Executar comandos.
Coletar inventário.
Coletar métricas.
Traduzir comandos do Control Plane.
Traduzir informações do Torrent Engine.
Monitorar saúde local.
Executar operações administrativas.
Gerenciar configuração local.
Publicar eventos.
O Agent NÃO deverá ser responsável por:
Tomar decisões de Scheduler.
Persistir dados do Cluster.
Executar autenticação de usuários.
Enviar notificações.
Gerenciar outros Nodes.
# **4. Arquitetura**
Control Plane
│
──────────────── HTTPS ─────────────────
SeedGrid Agent
┌───────────────────────────────┐
│                               │
│ Registration Manager          │
│ Inventory Manager             │
│ Torrent Manager               │
│ Storage Manager               │
│ Metrics Collector             │
│ Health Manager                │
│ Docker Manager                │
│ Configuration Manager         │
│ Command Executor              │
│ Event Publisher               │
└───────────────────────────────┘
│
Torrent Engine
│
Sistema Operacional
# **5. Componentes**
O Agent será composto pelos seguintes módulos.
# **Registration Manager**
Responsável pelo registro do Node.
Funções
Registrar novo Node.
Renovar registro.
Receber identidade.
Armazenar credenciais.
Enviar Heartbeat.
# **Inventory Manager**
Responsável pelo inventário.
Hardware.
Sistema Operacional.
CPU.
Memória.
Discos.
USB.
Rede.
Docker.
Containers.
Versões.
# **Torrent Manager**
Responsável pela comunicação com o Engine.
Adicionar Torrent.
Remover Torrent.
Pausar.
Retomar.
Recheck.
Consultar estado.
Consultar estatísticas.
Consultar arquivos.
# **Storage Manager**
Responsável pelos discos.
Inventário.
SMART.
Mount Points.
Filesystem.
Espaço.
Permissões.
# **Metrics Collector**
Responsável por coletar:
CPU.
RAM.
Disco.
Rede.
Engine.
Docker.
Temperatura.
# **Health Manager**
Responsável por calcular a saúde local.
Detectar problemas.
Publicar eventos.
Monitorar serviços.
# **Docker Manager**
Responsável por conhecer o ambiente Docker.
Containers.
Imagens.
Volumes.
Networks.
Estado.
# **Configuration Manager**
Responsável por:
Carregar configuração.
Persistir configuração local.
Receber configuração do Control Plane.
Validar configuração.
# **Command Executor**
Responsável por executar comandos enviados pelo Control Plane.
Todos os comandos deverão passar por este módulo.
# **Event Publisher**
Responsável por gerar eventos locais.
Exemplos
Engine iniciou.
Disco desconectado.
SMART falhou.
Temperatura alta.
Container reiniciado.
# **6. Ciclo de Vida**
Ao iniciar, o Agent deverá executar:
1
Carregar configuração.
↓
2
Validar ambiente.
↓
3
Descobrir Hardware.
↓
4
Inicializar módulos.
↓
5
Conectar ao Engine.
↓
6
Registrar-se no Control Plane.
↓
7
Enviar inventário.
↓
8
Entrar em operação.
# **7. Inicialização**
Caso algum módulo falhe durante a inicialização:
O Agent deverá registrar o erro.
Continuar iniciando os módulos independentes.
Entrar em modo degradado quando possível.
Evitar encerramento completo sempre que possível.
# **8. Encerramento**
Ao finalizar:
Cancelar operações pendentes.
Finalizar comunicação com o Engine.
Enviar último evento.
Persistir configuração.
Encerrar serviços internos.
# **9. Configuração**
O Agent possuirá apenas um arquivo de configuração.
Responsabilidades:
Identidade.
Control Plane.
Credenciais.
Parâmetros locais.
Logging.
Cache.
Nenhuma configuração operacional ficará distribuída em múltiplos arquivos.
# **10. Atualização**
O Agent deverá ser atualizável independentemente do Engine.
A atualização não deverá alterar configurações locais.
A atualização deverá preservar identidade do Node.
# **11. Objetivos Arquiteturais**
Baixo consumo de memória.
Baixo consumo de CPU.
Inicialização rápida.
Baixa utilização de disco.
Baixo tráfego de rede.
Facilidade de atualização.
Baixa dependência do sistema operacional.
# **12. Dependências**
O Agent dependerá apenas de:
Sistema Operacional.
Docker.
Torrent Engine.
Control Plane.
Nenhum outro componente do SeedGrid será obrigatório para sua execução local.
# **13. Princípio Fundamental**
O Agent representa integralmente o Node.
Todo acesso ao Node deverá ocorrer através dele.
O Agent é a única autoridade local reconhecida pelo Control Plane.
Essa separação deverá ser preservada durante toda a evolução do projeto.
O Control Plane decide.
O Agent executa.
O Agent nunca altera políticas do Cluster.
O Agent nunca escolhe Nodes.
O Agent nunca toma decisões de negócio.
# **14. Comunicação**
O Agent será o único ponto de comunicação entre o Node e o Control Plane.
Nenhum outro serviço deverá receber comandos diretamente do Control Plane.
Todo comando recebido deverá passar pelo Agent.
# **15. Comunicação com o Control Plane**
O Agent deverá manter comunicação segura utilizando HTTPS.
Toda comunicação será iniciada pelo Agent.
O Control Plane nunca abrirá conexões diretamente para o Node.
Essa abordagem permite funcionamento mesmo em ambientes onde os Nodes estejam atrás de NAT ou firewalls restritivos, desde que consigam estabelecer conexões de saída.
# **16. Registro**
Ao iniciar pela primeira vez, o Agent deverá:
- gerar sua identidade local;
- registrar-se no Control Plane;
- receber confirmação de registro;
- persistir sua identidade.
Após registrado, o mesmo Node deverá reutilizar sua identidade em reinicializações futuras.
# **17. Heartbeat**
O Heartbeat representa a confirmação de que o Agent permanece operacional.
O Heartbeat deverá conter apenas informações resumidas.
Exemplos:
- versão do Agent;
- versão do Engine;
- uptime;
- estado atual;
- Health Score;
- versão do inventário.
O Heartbeat não deverá transportar inventários completos nem listas de torrents.

**O Heartbeat também é o canal da configuração (ADR-039).** A resposta traz `agent_config` — os
valores efetivos de comportamento deste Node, já com a precedência resolvida pelo Control Plane
(override do Node > valor de cluster) — e o heartbeat seguinte devolve `config_version`: a versão
que o Agent aplicou **com sucesso**. O Control Plane manda o estado desejado inteiro a cada
heartbeat; perder um não custa nada. Campos novos no corpo só são enviados depois de o Control
Plane provar, numa resposta anterior, que os entende (ver `13 - API.md` §8.1).
# **18. Inventário**
O inventário deverá ser enviado:
- durante o primeiro registro;
- quando solicitado pelo Control Plane;
- quando o Agent detectar alterações relevantes.
Exemplos de alterações:
- novo disco;
- remoção de disco;
- alteração de memória;
- atualização do sistema operacional;
- alteração de interfaces de rede;
- atualização do Engine.
# **19. Configuração Local**
O Agent manterá apenas configurações necessárias para seu funcionamento.
Exemplos:
Control Plane
Credenciais
Logging
Cache
Diretórios locais
Níveis de log
Intervalos de coleta
Toda configuração operacional do Cluster pertence ao Control Plane.

**Realizado pela ADR-039 (#144).** O que é **identidade/máquina** continua só no `.env` (URL do Control
Plane, token, credencial do Engine, porta BT, `save_path`, nome do container do Engine, porta de
migração, diretório de dados). O que é **comportamento** — intervalo de heartbeat, intervalo de
polling de comandos, nível de log e a fila do Engine (`max_active_*`, rotação anti-stall) — vem do
Control Plane e o env dele é **fallback** (só vale quando o CP não manda nada: CP anterior à ADR-039,
CP fora, ou o boot antes do primeiro heartbeat). Precedência: override do Node > valor de cluster >
env > default do código.

Como o Agent aplica (`internal/remotecfg`), **a quente e sem reiniciar**:
- **Fila do Engine** → `setPreferences` só das chaves da fila (a porta BT e o `save_path` são da máquina e só são fixados na primeira aplicação do processo);
- **Intervalo do heartbeat** e **do polling de comandos** → recriam o ticker;
- **Nível de log** → `slog.LevelVar`.

Regras do Agent: (1) **valida o que recebe** — um intervalo ≤ 0 derrubaria um ticker, um teto de downloads ≤ 0 não é "sem limite" no qBittorrent — e recusa o valor ruim mantendo o anterior, sem declarar a versão como aplicada; chave desconhecida (CP mais novo) é ignorada; (2) **repete a aplicação no Engine a cada heartbeat** até dar certo (o Engine pode estar fora do ar); (3) **no boot, segura a aplicação** enquanto o Control Plane pede o relato do env (semeadura dos overrides, ADR-039 §7) — o valor de cluster nunca chega a um Node que tinha valor próprio —, e só aplica o `.env` se o Control Plane não responder em três heartbeats; (4) **nunca manda campo novo a um CP que não o entende** (o corpo do heartbeat é decodificado com `DisallowUnknownFields`). O que exige restart é o que ficou no `.env`.
# **20. Cache Local**
O Agent poderá manter cache local para reduzir chamadas ao Engine.
O cache deverá conter apenas informações transitórias.
Exemplos:
último inventário;
últimas estatísticas;
estado conhecido dos torrents;
últimos eventos.
A perda do cache nunca poderá impedir a operação do Node.
# **21. Torrent Manager**
O Torrent Manager é responsável por toda interação com o Torrent Engine.
Suas responsabilidades incluem:
Adicionar torrents.
Remover torrents.
Pausar torrents.
Retomar torrents.
Executar recheck.
Consultar arquivos.
Consultar estatísticas.
Consultar trackers.
Consultar prioridades.
Consultar progresso.
O restante do Agent nunca deverá comunicar-se diretamente com o Engine.
# **22. Storage Manager**
Responsável pelo armazenamento local.
Funções:
Descobrir discos.
Descobrir partições.
Identificar pontos de montagem.
Consultar SMART.
Consultar espaço disponível.
Consultar sistema de arquivos.
Validar permissões.
Detectar alterações físicas.
Detectar remoções inesperadas.
# **23. Inventory Manager**
Responsável pelo inventário completo do Node.
Inventariar:
Hardware.
CPU.
Memória.
Interfaces.
Discos.
Docker.
Containers.
Sistema Operacional.
Kernel.
Versões.
USB.
Arquitetura.
O objetivo é representar fielmente o ambiente onde o Agent está sendo executado.
# **24. Metrics Collector**
Responsável pela coleta de métricas instantâneas.
Exemplos:
CPU.
RAM.
Swap.
Disco.
Rede.
Temperatura.
Engine.
Docker.
As métricas coletadas serão utilizadas pelo Health Manager e exportadas para o sistema de monitoramento.
O Agent não deverá manter histórico de métricas.
# **25. Health Manager**
Responsável por avaliar continuamente a saúde do Node.
Critérios avaliados:
Disponibilidade do Engine.
Espaço disponível.
Temperatura.
SMART.
Carga do sistema.
Serviços essenciais.
Conectividade.
Heartbeat.
Caso problemas sejam identificados, o Health Manager deverá gerar eventos locais para o Event Publisher.
# **26. Docker Manager**
Responsável por conhecer o ambiente Docker do Node.
Funções:
Listar containers.
Consultar estado.
Consultar imagens.
Consultar volumes.
Consultar redes.
Verificar saúde dos containers.
O Docker Manager nunca deverá alterar containers que não pertençam ao SeedGrid sem solicitação explícita do operador.
# **27. Command Executor**
Todo comando recebido do Control Plane deverá ser executado exclusivamente por este módulo.
Exemplos:
Adicionar torrent.
Pausar torrent.
Retomar torrent.
Atualizar inventário.
Executar backup.
Atualizar configuração.
Executar recheck.
Atualizar Agent.
Extrair informações de mídia (`mediainfo`, ADR-020).
Extrair screenshots de vídeo (`extract_screenshots`, via `ffmpeg`/`ffprobe`, ADR-026).
Nenhum outro módulo executará comandos diretamente.
# **28. Event Publisher**
Responsável por transformar acontecimentos locais em eventos do Cluster.
Exemplos:
Engine iniciou.
Engine parou.
Disco conectado.
Disco removido.
SMART crítico.
Temperatura elevada.
Erro de comunicação.
Atualização concluída.
Todos os eventos serão enviados ao Control Plane.
# **29. Tratamento de Falhas**
O Agent deverá ser resiliente.
A indisponibilidade temporária de um módulo não deverá interromper os demais.
Exemplos:
Se o Docker estiver indisponível, o restante do Agent continua funcionando.
Se o SMART falhar, o inventário continua disponível.
Se o Engine estiver parado, o Node continua registrado.
Sempre que possível, falhas deverão degradar funcionalidades, nunca interromper completamente o Agent.
# **30. Logging**
Todos os módulos deverão gerar logs estruturados.
Cada entrada deverá identificar:
- módulo;
- severidade;
- horário;
- operação;
- resultado.
O nível de log deverá ser configurável.
# **31. Princípios**
O Agent nunca toma decisões de negócio.
O Agent nunca escolhe Nodes.
O Agent nunca altera políticas do Cluster.
O Agent executa.
O Control Plane decide.
Essa separação deverá ser preservada durante toda a evolução do projeto.
# **32. Relação com o Torrent Engine**
O Torrent Engine é considerado um componente externo ao Agent.
O Agent será responsável por adaptar a interface do Engine para o modelo de dados e operações do SeedGrid.
O restante do Cluster nunca deverá depender diretamente de APIs específicas do Engine.
# **33. Adaptadores (Engine Adapters)**
O Agent deverá possuir uma camada de adaptação para o Engine.
Cada Engine suportado deverá possuir seu próprio adaptador.
Exemplos futuros:
- qBittorrent Adapter
- rTorrent Adapter (ADR-045)
- Transmission Adapter
- Deluge Adapter
Todos deverão expor a mesma interface interna para o restante do Agent.
# **34. Independência do Sistema Operacional**
O Agent deverá minimizar dependências específicas do sistema operacional.
Sempre que possível, utilizar mecanismos compatíveis entre distribuições Linux.
Caso uma funcionalidade dependa de um recurso específico da distribuição, essa dependência deverá estar isolada em um módulo próprio.
# **35. Operações Administrativas**
O Agent será responsável apenas pela execução das operações.
Exemplos:
- atualização do inventário;
- atualização da configuração local;
- reinicialização do Engine;
- coleta de informações do sistema;
- execução de verificações locais.
O Agent não deverá executar operações destrutivas sem comando explícito do Control Plane.
# **36. Consistência**
Sempre que uma operação administrativa for concluída, o Agent deverá atualizar o Control Plane com o resultado da execução.
Caso a operação falhe, o Agent deverá informar:
- operação executada;
- motivo da falha;
- data e hora;
- impacto conhecido.
# **37. Reconexão**
Caso a comunicação com o Control Plane seja interrompida:
O Agent deverá:
- continuar executando normalmente;
- manter o Engine operacional;
- continuar coletando inventário quando necessário;
- continuar monitorando a saúde local;
- manter o **último valor de configuração aplicado** (ADR-039): o Control Plane fora do ar não desfaz o que já está em vigor — só um **restart** do Agent parte do `.env`.
Quando a comunicação for restabelecida:
- reenviar informações pendentes;
- sincronizar estado;
- atualizar Heartbeat.
O Node não deverá reiniciar automaticamente apenas por perda de conectividade.
# **38. Operação Offline**
Durante indisponibilidade temporária do Control Plane:
O Node continuará:
- semeando torrents;
- respondendo ao protocolo BitTorrent;
- executando monitoramento local.
Operações administrativas dependerão da disponibilidade do Control Plane.
# **39. Consumo de Recursos**
O Agent deverá ser projetado para consumo reduzido de recursos.
Objetivos da V1:
CPU em repouso inferior a 2%.
Memória inferior a 150 MB.
Uso mínimo de disco.
Baixo número de processos.
Baixo consumo de rede.
Esses valores representam metas de projeto e poderão ser revisados conforme evolução da plataforma.
# **40. Segurança**
O Agent deverá:
- armazenar apenas as credenciais estritamente necessárias;
- utilizar comunicação criptografada;
- validar identidade do Control Plane;
- proteger informações sensíveis em armazenamento local.
Nenhuma credencial de usuário final deverá ser armazenada pelo Agent.
# **41. Observabilidade**
O Agent deverá fornecer informações suficientes para diagnóstico remoto.
Exemplos:
- versão instalada;
- estado dos módulos;
- tempo de execução;
- último Heartbeat enviado;
- versão do inventário;
- Engine em uso;
- estado do Engine.
Essas informações deverão estar disponíveis ao Control Plane.
# **42. Atualizações**
A atualização do Agent deverá preservar:
- identidade do Node;
- configuração local;
- cache não crítico (quando aplicável).
Após atualização bem-sucedida, o Agent deverá:
- registrar sua nova versão;
- reenviar Heartbeat;
- informar sucesso ao Control Plane.
# **43. Escalabilidade**
A arquitetura do Agent deverá permitir inclusão de novos módulos sem alteração significativa dos módulos existentes.
Novos módulos deverão integrar-se utilizando os mesmos mecanismos de configuração, logging, eventos e comunicação já estabelecidos.
# **44. Dependências Externas**
O Agent deverá depender apenas de componentes essenciais ao funcionamento do Node.
Dependências externas deverão ser mantidas no menor número possível.
O objetivo é simplificar instalação, atualização e manutenção.
Exemplos de binários externos exigidos por funcionalidades específicas de publicação (opcionais, best-effort — a ausência não deverá impedir o restante do Agent):
- `mediainfo`: extrai metadados técnicos do arquivo de mídia principal do torrent, usados na publicação em trackers UNIT3D (ADR-020).
- `ffmpeg`/`ffprobe`: extraem frames (screenshots) do arquivo de mídia principal, também para publicação em trackers UNIT3D (ADR-026). Exigido em todos os Nodes, incluindo arquitetura ARM64 (RPi 3B+).
## 44.1 Um Engine que a máquina já rodava (ADR-043)
Um Node pode entrar com o qBittorrent que a máquina já roda (`bootstrap-node.sh ENGINE=existing`). A entrada não altera nada nesse Engine e, uma vez no Cluster, o Node é gerenciado como qualquer outro. O Agent roda com `SEEDGRID_ENGINE_ADOPTED=true`, o que muda apenas o início:
- o Node se registra com `engine_adopted`, e o Control Plane o inicia em Manual (ADR-024) para revisão do operador;
- no primeiro contato, o Agent **lê a fila do Engine** (`app/preferences`) e a reporta, em vez de seu ambiente, para a semeadura dos overrides do Node (ADR-039 §7). Os valores lidos também viram sua base. Até que a leitura tenha sucesso, o Agent não aplica nem reporta nada, nem mesmo após o período de carência do boot: aplicar o ambiente seria justamente a mudança na entrada que isso evita;
- a primeira aplicação então fixa o que o Engine já tinha: a porta BitTorrent que o enroll reservou (a própria porta do Engine), o `save_path` no storage (a pasta do host correspondente ao `save_path` do Engine, traduzida de volta pelo mapa de caminhos) e a fila que o Control Plane semeou. UPnP e a porta aleatória são desligados, como em todo Node.

Daí em diante, as configurações `engine.*` do Node são aplicadas, `restart_engine` funciona (o bootstrap garantiu que algo reinicie o qBittorrent) e o heartbeat reporta a porta reservada.

**Caminhos.** Quando o Engine roda em um container, os caminhos que ele reporta são os do container. `SEEDGRID_ENGINE_PATH_MAP` lista seus bind mounts como pares `caminho no Engine=caminho no host` separados por `;` (por exemplo `/downloads=/mnt/user/torrents`); o bootstrap escreve isso a partir do `docker inspect`. O Agent traduz todo caminho que o Engine reporta antes de ler dados (origem de migração, screenshots, MediaInfo) e todo caminho que entrega ao Engine (`save_path`, destino de migração, origem de publicação), usando o prefixo correspondente mais longo. Um mapa malformado impede o Agent de iniciar. Os dados podem ser servidos para uma migração a partir do storage (`SEEDGRID_AGENT_STORAGE_PATH`) ou de qualquer pasta mapeada do host.

**Propriedade.** O Agent roda como root, e um Engine em container geralmente não. O que o Agent extrai — um pull de migração, um upload de publicação — recebe o proprietário da pasta de destino, para que o Engine possa continuar escrevendo em um torrent parcialmente migrado. Nos Nodes que o SeedGrid instala, tudo pertence ao root e nada muda. Uma falha no `chown` (uma exportação NFS com `root_squash`, por exemplo) não interrompe a cópia.

**Identidade da máquina (ADR-044).** O registro leva o `machine_id` (`/etc/machine-id` ou `/var/lib/dbus/machine-id`) e, quando o Agent se registra novamente, o `node_id` de seu arquivo de identidade. Com eles, o Control Plane distingue um Node removido que volta de outra máquina com o mesmo hostname. Um `410 node_removed` significa que o Node desta máquina foi removido e o Agent se registrou com o token compartilhado: o Agent registra como trazê-lo de volta (um join token e o bootstrap) e tenta novamente a cada 5 minutos, em vez de a cada heartbeat.
## 44.2 rTorrent e um Node sem root (ADR-045)
`SEEDGRID_AGENT_ENGINE` escolhe o adaptador do Engine: `qbittorrent` (padrão) ou `rtorrent`. Este exige `SEEDGRID_ENGINE_RTORRENT_URL` (`scgi:///path/to/rpc.socket`, `scgi://host:port` ou `http(s)://[user:pass@]host/RPC2`). Se o Agent não tiver adaptador para o Engine, ele para na inicialização e cita a variável. Um Node com rTorrent sempre entra pelo método da §44.1 (`SEEDGRID_ENGINE_ADOPTED=true`): suas primeiras configurações são lidas dele. Como o rTorrent não tem fila, ela fica **desligada**, com os limites do Agent prontos para quando o operador a ativar. Para Engines sem fila, o Agent mantém uma (Engine.md §6.2); ela só age após as configurações do Control Plane chegarem, então nada muda num rTorrent que acabou de entrar.

`seedgrid-agent --probe-rtorrent <url>` mostra o que o bootstrap precisa antes do enroll — `version`, `port`, `port_range`, `port_random`, `directory`, `session` — em linhas `chave=valor`. O comando sai com código 1 e o motivo se o rTorrent não responder ou for anterior à versão 0.9.8. Shell não fala SCGI; o Agent fala.

**Reinício por comando do operador.** Com `SEEDGRID_ENGINE_RESTART_CMD` definido, `restart_engine` executa esse comando (`sh -c`) em vez de pedir ao Engine para fechar. É útil em seedboxes cujo provedor é dono do Engine e fornece o comando de reinício. A falha, junto com a saída impressa, é a falha do próprio comando. Funciona para qualquer Engine.

**Sem root.** O bootstrap pode instalar o Agent para um usuário comum (Deployment §50.9): binário em `~/.local/bin`, env em `~/.config/seedgrid-agent.env`, identidade em `~/.local/share/seedgrid-agent` (`SEEDGRID_AGENT_DATA_DIR`). Um serviço systemd de usuário o reinicia como faria o serviço do sistema. Já sob cron, o env traz `SEEDGRID_AGENT_SUPERVISED=false`: após `update_agent` trocar o binário, o Agent reexecuta o novo no próprio processo (mesmo PID) em vez de sair, pois o cron só o iniciaria novamente dentro de um minuto. Se isso falhar, ele sai e o cron o traz de volta. O rollback para `seedgrid-agent.prev` (ADR-027) é feito pela unidade de usuário e pelo script do cron, assim como pela unidade do sistema.
# **45. Testabilidade**
A arquitetura deverá permitir testes independentes para cada módulo.
A falha de um módulo não deverá impedir a validação dos demais.
Sempre que possível, módulos deverão ser desacoplados do Engine através da camada de adaptação.
# **46. Evolução**
A arquitetura do Agent deverá permitir futuras expansões sem necessidade de reestruturação.
Exemplos de evoluções possíveis:
- suporte a novos Engines;
- novos coletores de inventário;
- novos mecanismos de monitoramento;
- novas operações administrativas.
Essas evoluções deverão respeitar os princípios definidos neste documento.
# **47. Resumo da Arquitetura**
Control Plane
│
HTTPS / API
│
SeedGrid Agent
│
┌───────────────────────────────────────┐
│ Registration Manager                  │
│ Inventory Manager                     │
│ Torrent Manager                       │
│ Storage Manager                       │
│ Metrics Collector                     │
│ Health Manager                        │
│ Docker Manager                        │
│ Configuration Manager                 │
│ Command Executor                      │
│ Event Publisher                       │
└───────────────────────────────────────┘
│
Engine Adapter
│
Torrent Engine
│
Sistema Operacional
# **48. Declaração Final**
O Agent é o representante oficial do Node perante o Cluster.
Sua função é abstrair o ambiente local, padronizar a comunicação com o Control Plane e fornecer uma visão consistente do Node, independentemente do hardware, sistema operacional ou Torrent Engine utilizado.
Toda interação entre o Cluster e o Node deverá ocorrer exclusivamente através do Agent, preservando o desacoplamento entre os componentes e permitindo evolução independente de cada camada da arquitetura.
