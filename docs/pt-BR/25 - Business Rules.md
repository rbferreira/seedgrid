# **SeedGrid**
# **Business Rules**
Version: 1.0
Status: Draft
Document: 25 - Business Rules.md

# **1. Objetivo**

Este documento é o **catálogo de regras de negócio e tunáveis** do Control Plane: cada
parâmetro que governa o comportamento da orquestração, com seu identificador, default,
onde é aplicado e o racional. É a contraparte em prosa do **catálogo de código**
(`control-plane/internal/settings/catalog.go`) — a fonte única dos tunáveis (ADR-023).

> **Fonte única.** O default e a semântica de cada tunável vivem no catálogo de código.
> Este documento **espelha** esse catálogo; ao alterar um `Spec`, atualize aqui na mesma
> entrega (DoD §28). O valor **efetivo** em produção é o override no banco (tabela
> `settings`), editável no painel **Configurações** do Portal com hot-reload — o env var
> correspondente apenas semeia o banco no 1º boot.

# **1.1 Regras de acesso (ADR-035)**

Operadores autenticados atuam conforme o papel associado:

| Papel | Pode fazer | Não pode |
|---|---|---|
| **Administrator** | Tudo (wildcard), incluindo gestão de usuários | — |
| **Publisher** | Adicionar/remover/migrar torrents, publicar no tracker, gerar screenshots, gerenciar trackers | Gerir usuários, alterar configurações, alterar modo do cluster |
| **Downloader** | Explorar, adicionar torrents para download próprio, baixar (download-ticket) | Publicar, remover torrents, gerir trackers/usuários |
| **Viewer** | Somente leitura: dashboard, nodes, torrents, alertas, feed, métricas, saúde | Qualquer mutação |

Regras:

1. **Troca da própria senha** (`POST /auth/password`) exige apenas sessão — todo
   operador pode trocar a própria senha.
2. **Gestão de usuários** exige `users:read`/`users:write` (Administrator).
3. **Proteção do último admin**: não é possível desabilitar (status ou delete)
   nem remover o papel `Administrator` do último administrador **ativo** —
   admins soft-deletados no histórico não contam para a proteção.
4. **Usuário desabilitado** (soft delete ou status não-ACTIVE) não autentica e
   perde as sessões — a revogação vale para tokens já emitidos, na hora.
5. Permissão negada responde `403 forbidden` e é registrada na auditoria
   (ADR-025); o ator é o username do operador.
6. **Criar .torrent de conteúdo local** (`POST /torrents/create`) exige
   `torrents:build` — o `Downloader` adiciona torrents para download próprio
   (`torrents:create`), não publica conteúdo.

# **2. Precedência de configuração (ADR-023)**

Para cada tunável de comportamento, o valor efetivo é resolvido assim:

1. **Override no banco** (`settings.value`) — a fonte de verdade editável (painel do Portal).
2. **Default do catálogo** (código) — quando não há override.

O **env var** (coluna *Env de seed* abaixo) só é lido no 1º boot: se não há linha no banco
e o env está setado e válido, o `Store.Load` grava-o como seed inicial. Depois disso, mudar
o `.env` **não** afeta nada — o painel manda. Valores efetivos corrompidos caem no default
do catálogo no getter (o Worker nunca quebra por config inválida).

Infra/segredo (DB URL, tokens, senha admin, API keys, prom URL, http addr, log level,
migration port) **não** são tunáveis: continuam só em env (`config.go`), imutáveis em runtime.

## 2.1 Tunáveis do Agent e do Engine: valor de cluster com override por Node (ADR-039)

Os tunáveis marcados `PerNode` no catálogo (§3.1.1b) governam o **Agent e o Engine de cada Seed
Node**, não o Control Plane. Para eles a precedência tem **quatro degraus**, e o env do Node deixa
de ser fonte e vira **fallback**:

1. **Override do Node** (`node_settings`) — o desvio deliberado de UMA máquina.
2. **Valor de cluster** — o override da tabela `settings` ou, sem ele, o default do catálogo.
3. **Env do Node** (`/etc/seedgrid-agent.env`) — só vale quando o Control Plane não manda nada (CP
   anterior à ADR-039, CP inalcançável, ou o Agent no boot antes do primeiro heartbeat).
4. **Default do código** do Agent — quando nem o env existe.

O Control Plane resolve os degraus 1–2 e entrega o valor **efetivo** de cada Node na resposta do
heartbeat; o Agent aplica **a quente** o que mudou. Remover o override devolve o Node ao valor de
cluster. Quem pode: valor de cluster = `settings:write`; override por Node = `nodes:write`.

> **Por que um valor de cluster sozinho seria o erro.** A frota é heterogênea de verdade: com os
> mesmos ~13 MB/s o qBittorrent usa 0,5–2,9% de CPU num i5 e **50,7% num Atom** (hashing de peças).
> Aplicar `engine.max_active_downloads = 10` "para alinhar" o node-04 foi exatamente o erro de
> 05/09/2026 (revertido para 3). O valor de cluster serve a maioria; a exceção é um override
> explícito, visível na tela do Node e na lista de Configurações, e auditado.

# **3. Catálogo de tunáveis**

## 3.1 Admissão de downloads (`admission.*`) — ADR-021 / #21 / #22

Controlador que mantém ≤ **L** torrents baixando no cluster inteiro (o tracker limita DL
slots por conta; a passkey é compartilhada). Aplicado em
`control-plane/internal/admission/worker.go`.

| Regra (key) | Default | Unidade | Env de seed | Descrição / racional |
|---|---|---|---|---|
| `admission.interval` | 20s | duração | `SEEDGRID_ADMISSION_INTERVAL` | Período de avaliação do controlador. Menor reage mais rápido; maior reduz carga. |
| `admission.max_cluster_downloads` | 0 | inteiro | `SEEDGRID_MAX_CLUSTER_DOWNLOADS` | Teto **fallback** de downloads quando nenhum tracker reportou um L. 0 desliga o fallback. |
| `admission.dl_slot_limit_ttl` | 6h | duração | `SEEDGRID_DL_SLOT_LIMIT_TTL` | Validade de um L observado na mensagem do tracker antes de expirar. |
| `admission.min_dl_rate` | 51200 | bytes/s | `SEEDGRID_ADMISSION_MIN_DL_RATE` | Abaixo desta taxa (sustentada), um download cede o slot na rotação por taxa. 0 desliga. |
| `admission.slow_grace` | 5m | duração | `SEEDGRID_ADMISSION_SLOW_GRACE` | Tempo contínuo abaixo da taxa mínima antes de rotacionar o lento. |
| `admission.slow_cooldown` | 10m | duração | `SEEDGRID_ADMISSION_SLOW_COOLDOWN` | Quanto o rotacionado por lentidão fica fora da fila de promoção (anti-thrashing). |
| `admission.dead_grace` | 3m | duração | `SEEDGRID_ADMISSION_DEAD_GRACE` | 0 seeds **E** 0 peers **conectados** contínuos por este tempo → torrent morto, cede o slot. 0 desliga. |
| `admission.dead_cooldown` | 30m | duração | `SEEDGRID_ADMISSION_DEAD_COOLDOWN` | Quanto o morto rotacionado vai pro fim da fila — não volta mesmo com maior progresso. |
| `admission.dead_backoff_max` | 4h | duração | `SEEDGRID_ADMISSION_DEAD_BACKOFF_MAX` | Teto do backoff: cada rotação por morto **seguida** do mesmo torrent dobra o cooldown (30m → 1h → 2h…) até aqui. Conectar em algum peer zera a contagem. 0 desliga o backoff (cooldown fixo). |
| `admission.disk_min_free` | 5 GiB (5368709120) | bytes | `SEEDGRID_ADMISSION_DISK_MIN_FREE` | Margem de disco livre que nunca se compromete num Node (ADR-032). Um torrent só entra (add via `.torrent`) ou migra para um Node se o **espaço projetado livre** comportar o **tamanho total** do torrent + esta margem; abaixo dela o Node dispara `NODE_DISK_FULL`. 0 desliga o bloqueio e o alerta. |
| `admission.priority_persistent` | false | bool | `SEEDGRID_ADMISSION_PRIORITY_PERSISTENT` | Modo persistente da priorização manual (F3). Ligado: um torrent priorizado (`prioritized_at`) fica **imune** à rotação por morto/lento — segura o slot até baixar. Desligado (padrão): a prioridade só **fura a fila**; depois de admitido o torrent segue o fluxo normal e perde a prioridade se a rotação o despromover. |

> ⚠️ **Admissão consciente de disco (ADR-032).** O **espaço projetado livre** de um Node = `heartbeats.disk_free_bytes` − Σ (torrents vivos no Node) `size_bytes × (1 − progress)` — o livre de agora menos o que os torrents já presentes **ainda vão gravar** até completarem. Mede-se pelo **tamanho total** (o torrent baixa por inteiro), sem contar duas vezes o que já está no disco. A fórmula é única (`control-plane/internal/capacity`), consumida por três pontos:
> - **Scheduler** (`Select`): exclui Nodes sem espaço ao adicionar via `.torrent` (tamanho conhecido); se nenhum comporta → `no_node_with_space` (409). **Add via magnet** não tem tamanho no CP → não bloqueia (só ganha o Node mais folgado no desempate).
> - **Migração** (`Migrate`): recusa destino sem espaço → `dest_no_space` (409). Tamanho ou disco do destino desconhecido → não bloqueia (não há como julgar).
> - **Alerta** `NODE_DISK_FULL` (ver §3.2): pega o Node **já** comprometido, que o bloqueio sozinho não desfaz.

> ⚠️ **Elegibilidade no Scheduler exige storage são (`storage_ok IS NOT FALSE`).** Um Node cujo Agent reporta `/data` não gravável — ou não montado, quando o Node **declarou disco dedicado** (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true`, o padrão) — **não recebe trabalho**, mesmo ONLINE, com Engine alcançável e health alto. Razão: sem o `/data` montado o Engine grava no disco de **sistema**, e o `heartbeats.disk_total_bytes` passa a medir o rootfs — o Node parece ter espaço, passa no filtro de disco (ADR-032) e baixa até encher a raiz. O filtro é simétrico ao `engine_reachable IS NOT FALSE`: `NULL` (Agent que não reporta o campo) continua elegível, só `false` exclui. Caso que motivou: node-04 em 05/09/2026 recebeu dois torrents com `/data` ainda no eMMC de 15 GB, enquanto o CP já registrava `storage_ok = false` e só o usava para health score, alerta e métrica. Este filtro é uma das camadas de proteção contra disco de dados ausente, atrasado ou remontado — a cadeia completa (montagem por UUID, checagem do Agent, alerta, reanexação por udev, propagação de mount para o Engine) está em `docs/06 - Storage.md` §35. O Node de **disco único** (declarado `false`) não tem disco que possa faltar: o Agent confere só a escrita, ele reporta `storage_ok = true` e é elegível normalmente (§35.3) — o espaço, nesse caso, é o do disco do sistema e é vigiado pelo filtro de espaço e pelo `NODE_DISK_FULL`.

> ⚠️ **"Sem peers" é medido pelos CONECTADOS, não pelo swarm.** `torrents.num_seeds`/`num_leechers` guardam o **swarm** (scrape do tracker — o que o Portal exibe); `num_seeds_connected`/`num_leechers_connected` (migration 00030) guardam quem está **de fato conectado**. A rotação por morto usa `COALESCE(num_seeds_connected, num_seeds, 0)` — o COALESCE cai no swarm quando o Agent é anterior ao campo (NULL), preservando o comportamento antigo. Racional: o scrape pode anunciar um **seeder fantasma** (registrado, mas que nunca responde). Julgado pelo swarm, esse torrent parece vivo, nunca é dado como morto, e como é "viável" e costuma ter alto progresso, volta ao **topo** da fila a cada cooldown — queimando um slot do teto L para sempre sem baixar 1 byte. Caso real que motivou o refino: `Blade.Runner.2049` no node-02, 98,79% e **0 conexões por 5 dias**, ciclando resume→hold a cada ~15 min (jul/2026).
>
> A **fila de promoção** (`promotableHolds`) continua julgando viabilidade pelo **swarm**, de propósito: ela ordena torrents em **hold** (pausados), e pausado tem 0 conectados por definição — usar o conectado ali zeraria o critério peer-aware e a fila voltaria a ser ordenada por progresso puro (o bug que a ADR-021 corrigiu). Para o reincidente crônico quem age é o **backoff** do `dead_cooldown`, não a ordenação.

> ⚠️ **Pausar um download na mão NÃO o tira da fila — depende do modo do Node (ADR-024).**
>
> | Modo do Node | O que acontece com o pause do operador |
> |---|---|
> | **SG** (automático) | O sistema tem prioridade: a pausa é **adotada** como admission-hold (`adoptOrphanPauses`) e o torrent volta para a fila, sendo promovido quando abrir slot. Aparece no Feed como hold com o motivo "adoção de pausa manual (Node SG)". |
> | **Manual** | O operador é soberano: a pausa fica de pé até ele retomar **ou** até o Node voltar para SG — nesse momento o sistema readota o que estava parado. |
>
> Para segurar um download de verdade, ponha o **Node em Manual** (Dashboard → coluna "Modo"); pausar num Node SG só cede o slot temporariamente. **Torrent completo pausado (`progress >= 1`) nunca é adotado** — é um seeder que o operador parou, não um download na fila.
>
> Racional: sem a adoção, a pausa manual era uma saída **sem volta** da orquestração — o torrent não conta em `countActive` nem entra em `promotableHolds`, ficando invisível para o controlador. O sintoma é o cluster rodando abaixo do L com a fila parecendo travada e o Feed sem nenhuma decisão de admissão.
>
> **Um Node que entra com seu próprio qBittorrent inicia em Manual (ADR-043).** Seus torrents aparecem no Cluster como estão, e o controlador não pausa, inicia ou rotaciona nenhum deles até que o operador mova o Node para SG. O modo Manual só existe sob Flex: se o Cluster estiver em Full auto quando esse Node se registrar, o gate muda para Flex (evento `NODE_ADOPTED_MANUAL`, com `gate_moved`), e os outros Nodes permanecem em SG — o Cluster se comporta como antes. Seus downloads ativos contam para L, como os de qualquer Node em Manual. Registrar o mesmo Node novamente (credencial perdida) não altera o modo escolhido posteriormente pelo operador.

> ⚡ **Priorização manual na fila (F3) — override do operador sobre a ordem peer-aware.** O operador prioriza um torrent pelo botão ⚡ no Dashboard (`POST /torrents/{id}/prioritize`; remover = `DELETE`). Marca `torrents.prioritized_at` — só estado no Control Plane, **não** enfileira comando no Node; o controlador aplica o efeito na próxima passada. Dois efeitos:
> - **Fura a fila:** em `promotableHolds`, `prioritized_at IS NOT NULL` é o **primeiro** critério de ordenação (antes de viável→progresso) e **ignora o cooldown** de rotação — o priorizado é o próximo a ser admitido quando abre um slot. No hold de excedente (`hold`), é o **último** a ser segurado.
> - **Persistência (tunável `admission.priority_persistent`):** ligado, o priorizado é **excluído** da rotação por morto/lento — segura o slot até completar. Desligado (padrão), a rotação o trata normalmente e, ao **despromovê-lo**, limpa o `prioritized_at` (o torrent "volta a ser normal").
>
> A prioridade também é limpa quando o torrent **completa** (`clearCompletedPriority`: SEEDING/`progress >= 1`) — não há mais fila a furar. O Portal mostra o torrent priorizado em **amarelo**. ⚠️ Em modo persistente, um priorizado sem seeds **ocupa um slot sem baixar** até o operador removê-lo ou surgirem seeds — é um override consciente.

## 3.1.1 Agent (`agent.*`) — ADR-027

Atualização do Agent orquestrada pelo CP (self-update por comando, rollout **manual**).

| Regra (key) | Default | Unidade | Env de seed | Descrição / racional |
|---|---|---|---|---|
| `agent.artifacts_url` | *(vazio)* | texto | `SEEDGRID_AGENT_ARTIFACTS_URL` | URL base onde o Portal publica os binários e o `agents.json` (ex.: `http://controller.example/node`). O CP lê o manifesto daqui; o **Agent baixa o binário desta mesma URL** → precisa ser alcançável **pelos Nodes** (não vale endereço interno de rede Docker). **Vazio desliga** a funcionalidade: sem badge no Portal e o endpoint responde 503 `not_configured`. |

> **Como o update acontece:** o Portal compara `nodes.agent_version` × versão do manifesto e mostra o botão só quando há binário publicado **para a arquitetura daquele Node** (`nodes.architecture`). O comando `update_agent` leva `{version, url, sha256}`; o Agent baixa, confere o **SHA-256**, roda o candidato com **`--version`** (smoke-test: prova que executa naquele host), guarda o `.prev`, faz `rename` atômico, **confirma ao CP** e só então sai — o systemd (`Restart=always`) reergue no binário novo.
>
> ⚠️ **Um binário por arquitetura.** `/node/seedgrid-agent-amd64` e `-arm64`. O caminho único `/node/seedgrid-agent` (ARM64, herança do RPi) **não existe mais**: com a frota em amd64 ele serviria binário errado — o `bootstrap-node.sh` agora resolve a arch por `uname -m`.
>
> **Sem rollback automático por heartbeat** (fora do escopo da ADR-027): o smoke-test impede trocar por um binário que não executa, e o `ExecStartPre` do unit restaura o `.prev` se o instalado quebrar depois. Binário que sobe mas falha em runtime exige entrar no Node e restaurar o `.prev` na mão.

## 3.1.1b Agent e Engine por Node (`agent.*`, `engine.*`) — ADR-039 / #144

Valor de **cluster**; cada Node pode ter override (§2.1). O **Env no Node** é o fallback do Agent
(só vale sem o Control Plane) — **não** semeia nada no Control Plane (o catálogo não tem `EnvVar`
para estas chaves); é o que o Agent relata na semeadura (abaixo). Os **defaults** são o que a
**maioria da frota** usava no `.env` (inventário de 03/10/2026); o default do **código** do Agent
(o fallback sem env) segue sendo o antigo e **não mudou**. O contrato com o Agent é
`shared/agent-config.json`, comparado por testes nos dois lados.

| Regra (key) | Default | Faixa | Env no Node | Aplicação no Agent | Descrição / racional |
|---|---|---|---|---|---|
| `agent.heartbeat_interval` | 10s | 2s–60s | `SEEDGRID_AGENT_HEARTBEAT_INTERVAL` | recria o ticker | Período do heartbeat. Menor = o Portal e os alertas enxergam o Node mais cedo. Mantenha bem abaixo de `alerting.node_offline_after` (90s). |
| `agent.command_poll_interval` | 5s | 1s–60s | `SEEDGRID_AGENT_COMMAND_POLL_INTERVAL` | recria o ticker do Poller | Latência de uma ação do operador até o Node (ADR-014). |
| `agent.log_level` | info | debug/info/warn/error | `SEEDGRID_LOG_LEVEL` | `slog.LevelVar` | Verbosidade do log do Agent. Um override por Node evita sujar o log da frota para depurar uma máquina. |
| `engine.queueing_enabled` | true | bool | `SEEDGRID_ENGINE_QUEUEING_ENABLED` | `setPreferences` (fila) | Fila do Engine: a proteção contra saturação de I/O. Desligada, os tetos abaixo deixam de valer. |
| `engine.max_active_downloads` | 10 | 1–200 | `SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS` | `setPreferences` (fila) | Downloads simultâneos por Node. **É o parâmetro que difere por máquina**: o Atom do node-04 aguenta 3. Cinto-e-suspensório do controlador de admissão (ADR-021), que garante o L do tracker no cluster. |
| `engine.max_active_uploads` | 10000 | 1–100000 | `SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS` | `setPreferences` (fila) | Praticamente ilimitado **de propósito**: seeder enfileirado manda `event=stopped` ao tracker e leva Hit and Run. |
| `engine.max_active_torrents` | 10000 | 1–100000 | `SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS` | `setPreferences` (fila) | Idem: só downloads e verificações devem ser enfileirados. |
| `engine.max_active_checking` | 1 | 1–10 | `SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING` | `setPreferences` (fila) | Verificações (hash) simultâneas: cada uma lê o torrent inteiro do disco; mais de uma satura HD USB. |
| `engine.dont_count_slow` | false | bool | `SEEDGRID_ENGINE_DONT_COUNT_SLOW` | `setPreferences` (fila) | `false` **de propósito**: ligado, o download parado não conta no teto, o Engine ativa todos, todos anunciam de uma vez e o tracker recusa slots (thundering herd). |
| `engine.slow_dl_threshold` | 1 KB/s | 0–1000000 | `SEEDGRID_ENGINE_SLOW_DL_THRESHOLD` | `setPreferences` (fila) | Só com `dont_count_slow` ligado. |
| `engine.slow_ul_threshold` | 1 KB/s | 0–1000000 | `SEEDGRID_ENGINE_SLOW_UL_THRESHOLD` | `setPreferences` (fila) | Só com `dont_count_slow` ligado. |
| `engine.slow_inactive_timer` | 300 s | 1–86400 | `SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER` | `setPreferences` (fila) | Só com `dont_count_slow` ligado. |

**Nada de comportamento exige restart.** A aplicação do `engine.*` manda **só as chaves da fila**: a
porta BitTorrent e o `save_path` são da máquina (casam com o port forward e com o disco montado) e só
são fixados na primeira aplicação do processo. Se o Engine estiver fora do ar, a aplicação é repetida
a cada heartbeat até dar certo.

**Validação nos dois lados.** O `Spec` acima é a régua do operador (o Control Plane recusa o valor na
origem: `400 invalid_value`). O Agent ainda valida o que recebe — um intervalo ≤ 0 derrubaria um
ticker, um teto de downloads ≤ 0 não é "sem limite" no qBittorrent — e recusa o valor ruim mantendo o
anterior; uma chave que ele não conhece (CP mais novo) é ignorada.

**Selo "em dia".** O Agent devolve no heartbeat a `version` (hash dos valores) que aplicou **com
sucesso**; o Portal compara com a efetiva: **Em dia**, **Aguardando o Agent** (mudou e ainda não
chegou) ou **Agent sem suporte** (anterior à ADR-039, nunca reportou; segue no `.env`).

**Semeadura dos overrides (migração, ADR-039 §7).** Um Node que já roda tem valores no `.env`; um
Agent novo que obedecesse só o cluster os mudaria no primeiro heartbeat. Por isso o Control Plane
**semeia uma vez por Node** (`nodes.agent_config_seeded_at`), a partir do `.env` que o Agent relata,
**somente** quando todas valem: (1) é o primeiro relato do Node; (2) o Agent relatou a chave porque o
operador a **definiu** no `.env` (default do código não conta); (3) **não existe override** do Node
para a chave; (4) o valor de cluster da chave **não foi definido pelo operador** (ainda é o default);
(5) o valor é **válido** pelo `Spec` e **difere** do de cluster. Cada semeadura gera o Evento
`NODE_CONFIG_SEEDED`. Enquanto a semeadura não aconteceu o Agent **não aplica nada** (o valor de
cluster poderia sobrecarregar justamente o Node que tinha valor próprio). É rede de segurança — o
procedimento recomendado continua sendo criar os overrides **antes** de atualizar cada Agent
(`19 - Deployment.md` §50.5).

## 3.1.2 Notificações / telemetria (`notify.*`) — ADR-028

Manda alertas/eventos/orquestração para fora do Portal (Telegram, Discord, webhook genérico). Aplicado em `control-plane/internal/notify`, disparado nos pontos que já geram alerta (ADR-008), evento (ADR-006) e atividade (ADR-022).

| Regra (key) | Default | Tipo | Descrição / racional |
|---|---|---|---|
| `notify.telegram_bot_token` | *(vazio)* | texto **secreto** | Token do bot (@BotFather). **Nunca volta na API de leitura** (só `has_value`); PUT vazio = manter; "Restaurar padrão" = limpar/desligar. |
| `notify.telegram_chat_id` | *(vazio)* | texto | Destino (usuário/grupo/canal). O canal só envia com token **E** chat_id. |
| `notify.discord_webhook_url` | *(vazio)* | texto **secreto** | Webhook do canal. Preencher liga; nunca é exibida de volta. |
| `notify.webhook_url` | *(vazio)* | texto **secreto** | POST de JSON cru (`{kind,severity,title,message,node,fields,at}`) — Slack/n8n/HA sem código novo. |
| `notify.alerts_min_severity` | `WARNING` | enum `OFF/CRITICAL/WARNING/INFO` | Piso de severidade dos alertas notificados. OFF desliga a trilha sem mexer nos canais. |
| `notify.alerts_on_resolve` | `true` | bool | Avisa também quando o alerta **resolve** (Node voltou, storage recuperou). |
| `notify.events` | *(vazio)* | **multi** (checkboxes) | Eventos a avisar. Persistido como CSV de códigos, mas o painel mostra **caixas com rótulos em PT-BR** (o operador não digita `TORRENT_RECHECK`). Opções = `events.NotifiableCodes()`; vazio = nenhum. |
| `notify.activities` | *(vazio)* | **multi** (checkboxes) | Decisões de orquestração a avisar. Idem, rótulos amigáveis; opções = `activity.AllKinds()`. ⚠️ Alto volume. |
| `notify.rate_limit_per_min` | `20` | inteiro | Teto anti-flood **de evento e atividade**. Excedente descartado, contado no log e na métrica `seedgrid_notify_suppressed_total`. 0 = sem limite. ⚠️ **Alerta não entra neste teto** (ver abaixo). |

**Filtros por canal.** Cada canal repete o mesmo conjunto de chaves, prefixado por `notify.<canal>.` (`telegram`, `discord`, `webhook`): `custom`, `alerts_min_severity`, `alerts_on_resolve`, `events`, `activities`, `rate_limit_per_min`. O toggle **`custom`** (default `false`) decide se o canal usa os filtros próprios ou os gerais — canal sem ele se comporta exatamente como antes de existir roteamento.

Existe porque, sem isso, **todo canal configurado recebe tudo** que passa no filtro de cluster: não dava para mandar só a troca de IPv6 para o webhook que dispara uma automação sem o endpoint receber junto tudo o que o cluster notifica. O `custom` é um toggle explícito, e não "vazio = herda", porque vazio **já tem significado** nas trilhas (lista vazia = nenhum evento).

> ⚠️ **ALERTA NUNCA entra no teto por minuto, em canal nenhum.** Alerta existe para incomodar; um limitador que **descarta** pode comer justamente o aviso que importava, e qual morre é arbitrário (quem chegou depois da cota estourar). Antes disso, o teto era uma janela única compartilhada por todas as trilhas: uma rajada de "download concluído" podia engolir um `CRITICAL`, e o descarte só aparecia num `logger.Warn` dentro do log do container.
>
> Contra tempestade de alerta o remédio é outro, e já existe em parte: **dedup no banco** (índice `uk_alerts_open_condition`, `code` + `node_id` enquanto `OPEN`) impede re-notificar enquanto o alerta está aberto, e **regra ruidosa se conserta na regra** — não num limitador que esconderia o sintoma junto com o resto. Se um dia for preciso proteger, que seja por **coalescência** ("N alertas"), nunca descarte.

> **Segredo no catálogo.** Credencial de terceiro do operador (bot token, webhook URL) vive no painel, com `Secret` mascarando na leitura como já se faz com `trackers.api_token`. A ADR-029 estendeu isso a **todas** as chaves de terceiro (inclusive TMDB/IGDB/ptpimg — ver §3.1.3); só a infra do próprio deploy segue em env. Racional na ADR-028 (mecanismo) e ADR-029 (alcance).
>
> **Auditoria fica de fora** (são as ações do próprio operador — notificar seria eco). O valor de um campo secreto **não** é auditado (audita-se o fato da troca). Botão "Enviar teste" (`POST /api/v1/notify/test`) prova a entrega antes do primeiro incidente. Best-effort: canal externo fora do ar **nunca** quebra o fluxo que gerou o fato.
>
> **Cor do ícone (Telegram/Discord)** — 🟢 verde = sucesso explícito (`Severity: SUCCESS`: Atividade "seeding", Evento `TORRENT_MIGRATED` e `NODE_REGISTERED`, e alerta resolvido); 🔴 vermelho = `CRITICAL`; 🟠 laranja = `WARNING` (ex.: migração de torrent abortada, disco cheio/saturado, reinício do Engine, `NODE_DELETED` — remover Node é grave demais pra ser azul); 🔵 azul = default (`INFO` ou severidade ausente — nunca vira verde por omissão). O corpo é `Título` + os `Fields` do template (`Rótulo: valor`, em ordem) + `Node: <hostname>` + `Data:` ao final (o `Node` vem **depois** dos `Fields`: logo abaixo do título o hostname era lido como se fosse o nome do arquivo) (pulada quando um Field já a cobre, ex. "Concluído em"). `Node` é sempre o **hostname**, resolvido por uma consulta simples (`SELECT hostname FROM nodes WHERE id=$1`) no ponto de emissão — o mesmo padrão que o Alert Worker já usava — nunca o UUID cru do Node.
>
> **Todo Alerta/Evento/Atividade notificável tem template estruturado** em `notify/hooks.go` (`applyAlertTemplate`/`applyEventTemplate`/`applyActivityTemplate`) — não só um punhado de exemplos. Atividades: **seeding** (Nome/Tamanho/Adicionado em/Concluído em, sucesso); admission_hold/scheduled ganham Motivo (e scheduled também Pontuação/Candidatos); downloading/admission_resume ganham ao menos Arquivo. Eventos: TORRENT_CREATED/PAUSED/RESUMED/RECHECK/RESEEDED/PUBLISHED_EXTERNAL, as três fases de migração (STARTED/MIGRATED com Origem-Destino/FAILED com Motivo), TORRENT_REMOVED (Autor: `"SeedGrid"` na reconciliação automática, username do operador na remoção manual pelo Portal), NODE_REGISTERED/DELETED, ENGINE_RESTART, AGENT_UPDATE e STORAGE_RECOVERY — todos com Arquivo/Node/campos específicos em vez de uma frase única. Alertas: NODE_STORAGE (Detalhe), NODE_DISK_FULL (Faltam/Piso mínimo), DISK_IO_SATURATED (Utilização) e STORAGE_RECOVERY_FAILED (Etapa/Tentativa) somam campos ao corpo padrão (NODE_OFFLINE não precisa — o Título já diz tudo).

## 3.1.3 Integrações de terceiro (`integrations.*`) — ADR-029

Chaves de serviços externos atreladas à **conta do operador**. Ficam no catálogo (não em env) para o projeto poder ser aberto open-source sem a chave pessoal do mantenedor. Todas com seed-from-env no 1º boot (retrocompat) e mascaramento na leitura.

| Regra (key) | Default | Tipo | Descrição |
|---|---|---|---|
| `integrations.tmdb_api_key` | *(vazio)* | texto **secreto** | Chave v3 do TMDB (metadados de filmes/séries, ADR-020). Vazio desabilita o enriquecimento. |
| `integrations.igdb_client_id` | *(vazio)* | texto | Client ID do app do Twitch p/ o IGDB (jogos, #17). |
| `integrations.igdb_client_secret` | *(vazio)* | texto **secreto** | Client Secret do mesmo app. Sem o par, jogos desabilitados. |
| `integrations.imgbb_api_key` | *(vazio)* | texto **secreto** | Chave do imgbb.com (host dos screenshots, ADR-030 — substitui o ptpimg, que saiu do ar). Grátis em api.imgbb.com. Vazio desabilita screenshots. |

> **Por que saíram do env (ADR-029).** Eram lidas de `SEEDGRID_TMDB_API_KEY` etc. no boot (ADR-026, "segredo do sistema"). Mas essas chaves são da **conta pessoal** de quem opera — abrir o repositório com elas em env/exemplo vaza credencial. Agora vivem no painel, editáveis com hot-reload (os clientes releem a chave por chamada; esvaziar desliga na hora). Migração automática: o `.env` atual semeia o banco no 1º boot e passa a ser ignorado. **O que continua em env** é só a infra do próprio deploy (`POSTGRES_*`, `SEEDGRID_ADMIN_*`, `SEEDGRID_AGENT_REGISTRATION_TOKEN`) — propriedade de quem opera a instância, sem dono externo.

## 3.2 Alertas / Monitoring (`alerting.*`) — ADR-008

Alert Worker: avalia regras determinísticas em background. Aplicado em
`control-plane/internal/alert/worker.go`.

| Regra (key) | Default | Unidade | Env de seed | Descrição / racional |
|---|---|---|---|---|
| `alerting.worker_interval` | 30s | duração | `SEEDGRID_ALERT_WORKER_INTERVAL` | Período entre avaliações das regras de alerta. |
| `alerting.node_offline_after` | 90s | duração | `SEEDGRID_NODE_OFFLINE_AFTER` | Tempo sem heartbeat para um Seed Node ser marcado OFFLINE (dispara `NODE_OFFLINE`). |
| `alerting.disk_saturated_pct` | 90 | % | `SEEDGRID_DISK_IO_SATURATED_PCT` | Utilização de I/O a partir da qual o disco é considerado saturado (`DISK_IO_SATURATED`); histerese de 15pp na resolução. 0 desliga. |
| `alerting.disk_saturated_for` | 90s | duração | `SEEDGRID_DISK_IO_SATURATED_FOR` | Por quanto tempo o disco precisa ficar **continuamente** acima de `disk_saturated_pct` antes de abrir `DISK_IO_SATURATED` (#150). Uma passada do Alert Worker abaixo do limiar zera a contagem; o alerta sai na primeira passada depois que o tempo se cumpre (a cada 30s por padrão). A resolução não tem carência. A contagem fica em memória e recomeça se o Controller reiniciar. 0 abre na primeira leitura, como antes. Máx. 1h. |
| `alerting.firewalled_grace` | 30m | duração | `SEEDGRID_FIREWALLED_GRACE` | Tempo que um Node pode ficar sem receber **nenhuma** conexão de entrada antes de abrir `NODE_FIREWALLED`. Todo Node nasce `firewalled` até o primeiro peer chegar, então alertar de imediato seria alarme a cada restart do Engine. 0 desliga a regra. |
| `alerting.incoming_silence` | 240h | duração | `SEEDGRID_INCOMING_SILENCE` | Tempo sem **nenhuma** conexão de entrada, **havendo demanda** (definida pelo tunável abaixo), antes de abrir `NODE_INBOUND_SILENT`. Ao contrário de `firewalled_grace`, usa sinal que **decai**. Conexão de entrada é evento raro num acervo ocioso: um Node sem problema de porta passa de 2 a 9 dias sem receber nenhuma, e valor baixo alerta à toa (o default era 12h; calibração em `16 - Alerting.md §48.2`). O valor gravado no banco prevalece sobre o default. 0 desliga a regra. |
| `alerting.incoming_demand_seeds_per_leecher` | 1 | seeds/leecher | — | Quanto de concorrência um swarm pode ter e ainda contar como **demanda** para `NODE_INBOUND_SILENT` (#206): o torrent conta quando `num_leechers > 0` **e** `num_seeds <= num_leechers × valor`. Num swarm farto o leecher abre conexão com meia dúzia de seeds, então não ser um deles não diz nada sobre a porta. `1` = leechers ao menos tão numerosos quanto os seeds; `0` = só swarm sem nenhum seed (**não** é "desligado"); valor muito alto (ex.: 100000) volta a contar qualquer leecher. `num_seeds` ou `num_leechers` NULL = desconhecido, não conta. Aceita decimal (ex.: 2.5). |
| `alerting.ipv6_change_grace` | 15m | duração | `SEEDGRID_IPV6_CHANGE_GRACE` | Quanto o IPv6 novo precisa se sustentar antes de abrir `NODE_IPV6_CHANGED`. As privacy extensions rotacionam o sufixo sozinhas; sem carência, Node mal configurado alertaria a cada troca efêmera. 0 desliga a regra **e fecha os alertas dela que estiverem abertos** (#204). |
| `alerting.reachability_ttl` | 24h | duração | `SEEDGRID_REACHABILITY_TTL` | **Validade** de `NODE_FIREWALLED`, `NODE_INBOUND_SILENT` e `NODE_IPV6_CHANGED` (#211). Os três deduzem alcançabilidade pela ausência de entrada e só resolvem quando uma chega — o que Node sem demanda nunca recebe, mesmo com a porta aberta. Conta desde a **última notificação**; ao expirar, fecha **sem notificar** e o mesmo episódio não reabre. 0 = sem validade. |
| `alerting.sample_retention` | 168h | duração | `SEEDGRID_SAMPLE_RETENTION` | Retenção das amostras de série temporal (`torrent_stat_samples`) antes da poda. 0 desliga a purga. |

**Alertas emitidos pelo Alert Worker** (deduplicados por `(code, node_id)`, resolvidos automaticamente; os três de alcançabilidade também **expiram** — ver a nota da #211 abaixo):

| Código | Severidade | Abre quando | Resolve quando |
|---|---|---|---|
| `NODE_OFFLINE` | CRITICAL | Node ONLINE sem heartbeat há mais de `node_offline_after` | Node volta a reportar heartbeat |
| `NODE_STORAGE` | CRITICAL | Agent reporta `/data` não gravável, ou — com disco dedicado declarado — não montado (`storage_ok=false`) | `storage_ok` volta a true/NULL ou Node sai de ONLINE |
| `DISK_IO_SATURATED` | WARNING | Utilização de I/O do disco ≥ `disk_saturated_pct` **de forma contínua** por `alerting.disk_saturated_for` (#150) | Cai abaixo da folga (histerese 15pp) ou Node sai de ONLINE |
| `NODE_DISK_FULL` | WARNING | Espaço projetado livre < `admission.disk_min_free` (ADR-032) — o Node se comprometeu com mais torrents do que o disco conclui | Volta acima da margem, perde o dado de disco, ou Node sai de ONLINE |
| `NODE_FIREWALLED` | WARNING | Engine reporta `firewalled` (nenhuma conexão de ENTRADA aceita) há mais de `alerting.firewalled_grace` | Engine volta a reportar `connected`, o veredito fica desconhecido, ou Node sai de ONLINE |
| `NODE_INBOUND_SILENT` | WARNING | Nenhuma conexão de entrada há mais de `alerting.incoming_silence` **e** algum torrent do Node tem demanda ponderada: `num_leechers > 0` e `num_seeds <= num_leechers × alerting.incoming_demand_seeds_per_leecher` (#206) | Volta a receber conexão de entrada, ou Node sai de ONLINE |
| `NODE_IPV6_CHANGED` | WARNING | `public_ipv6` mudou de um endereço para outro, o novo se sustentou por mais de `alerting.ipv6_change_grace` **e** o Node ainda não recebeu entrada depois da troca | Node volta a receber conexão de entrada **depois** da troca (`last_incoming_at > public_ipv6_changed_at`), ou sai de ONLINE |

> ⚠️ **`DISK_IO_SATURATED` exige saturação sustentada (#150).** A leitura (`nodes.disk_io_util_pct`) é a média de ~10s entre dois heartbeats e a regra abria com **uma** delas. Num HD USB, serrilhado por natureza (87% num intervalo, 13% cinco minutos depois — node-04, 06/09/2026), um pico de segundos virava alerta: dos 18 abertos entre 12/09 e 02/10/2026, **16 duraram exatamente 30s** (uma passada do worker); os outros dois duraram 9min e 4m30, o tipo de episódio que a regra continua pegando se a saturação se sustentar. O gráfico do Portal usa janela de 2 minutos e nunca mostrava esses picos, então o alerta parecia falso. Agora o Node precisa estar ≥ limiar em **todas** as passadas durante `alerting.disk_saturated_for` (90s por padrão); qualquer passada abaixo — ou com o Node fora de ONLINE/sem heartbeat recente — zera a contagem. Notas de comportamento:
>
> - **A granularidade é a do Alert Worker** (`alerting.worker_interval`, 30s): com os defaults a abertura sai na 4ª leitura consecutiva (0, 30, 60, 90s). A comparação tem folga de 1/10 do intervalo para o jitter de milissegundos das passadas não adiar a abertura em uma passada inteira quando a carência é múltipla do intervalo.
> - **A resolução não mudou**: sem carência, com a histerese de 15pp. Uma leitura entre a folga e o limiar mantém o alerta aberto, mas não conta como saturada; reabrir depois de resolver exige uma sequência nova inteira.
> - **O início da sequência vive em memória** no Worker (`satSince`), sem migration: é estado derivado, e um restart do Controller só **atrasa** o alerta em até a carência — nunca cria alarme falso. Alerta já aberto não é afetado (está no banco). Carência 0 mantém o comportamento antigo.
> - **Fora de escopo, descartado:** persistir `disk_io_util_pct` em `heartbeats` (histórico da série que abre o alerta) e mostrar essa série no card do Node. A causa do ruído era a abertura com uma leitura, que a regra sustentada resolve; o histórico seria um recurso à parte.

> ⚠️ **Os alertas de alcançabilidade expiram (#211).** `NODE_FIREWALLED`, `NODE_INBOUND_SILENT` e `NODE_IPV6_CHANGED` são **inferência**: deduzem que o Node está inalcançável porque nenhuma conexão de entrada chegou, e só resolvem quando uma chega. Num Node sem demanda ela nunca chega, com a porta aberta ou não — medido em 24/09/2026, node-01 com 0 leechers nos 116 torrents, dois desses alertas abertos e a porta verificada **de fora**, aberta. O que importa é a **chamada para fora** na abertura (webhook, Telegram, Discord); depois de `alerting.reachability_ttl` o alerta fecha sozinho. Três regras de desenho:
>
> - **Fecha sem notificar.** "Resolvido: …" seria mentira — nada foi corrigido. O alerta fica com `expired = true` e o Portal mostra *expirou*.
> - **O mesmo episódio não reabre.** A condição continua valendo depois de expirar; sem isso o worker reabriria na passada seguinte e notificaria a cada ciclo da validade. Cada alerta grava a identidade da condição em `episode_key`: a rotação (`public_ipv6_changed_at`), o início do silêncio (`last_incoming_at`), o veredito (`connectable_since`). Vale também para **reboot**: sair de ONLINE fecha o alerta, e a volta encontra o episódio registrado em vez de re-notificar uma rotação antiga. Rotação nova, entrada seguida de outro silêncio ou restart do Engine mudam a chave e voltam a alertar.
> - **Conta desde a última notificação** (`updated_at`), não da abertura: uma rotação nova com o alerta aberto (#209) notifica de novo e renova a validade.
>
> As demais regras não expiram: o resolve delas é a negação da abertura sobre uma variável **medida** (heartbeat, `storage_ok`, disco), então um alerta delas aberto é condição que ainda existe. Sonda externa para confirmar a porta foi considerada e descartada como pré-requisito: um usuário comum não vai subir um sistema que fale com um endpoint de integração.

> ⚠️ **`NODE_FIREWALLED` exige `firewalled` sustentado, e o veredito é GRUDENTO.** O sinal nasce falso (todo Node está `firewalled` até o primeiro peer bater na porta), daí a carência; e uma vez `connected`, não volta atrás enquanto o Engine não reiniciar — ou seja, a regra pega Node cuja liberação **nunca funcionou**, não Node que **deixou** de ser alcançável. `NULL` é desconhecido e nunca alerta. O racional completo, as alternativas descartadas (sonda ativa do CP, probe externo, API do tracker) e o incidente que originou a regra estão na **ADR-036**; a lacuna do sinal grudento, na issue **#155**.

> ⚠️ **`NODE_FIREWALLED` e `NODE_INBOUND_SILENT` são complementares, não redundantes.** A primeira cobre **onboarding** (porta nunca liberada: o Node nasce `firewalled` e nunca sai — o veredito grudento serve bem aqui). A segunda cobre **regime** (Node que ERA alcançável e deixou de ser), que era o caso que escapava por completo: medido em 06/09/2026, node-01 e node-03 reportavam `connected` com o Engine de pé há 1 mês e 5 dias, atravessando inteiro o período em que estavam comprovadamente inalcançáveis. A segunda regra lê a **idade** de `last_incoming_at`, um carimbo que envelhece sozinho.
>
> **Por que exige demanda:** sem leecher no swarm, 0 conexões de entrada é o estado *correto* de um seeder. Medido em 07/09/2026, os quatro Nodes — todos saudáveis — tinham 0, 1, 1 e 0 conexões de entrada no instante da leitura; alertar pela ausência pura transformaria acervo ocioso em alarme permanente.
>
> **Por que ponderar a demanda (#206):** "existe um leecher" não é demanda sobre *este* Node. Um leecher num swarm com 63 seeds abre conexão com meia dúzia deles, e ficar horas sem ser um dos escolhidos é o comportamento esperado de um Node saudável. Medido em 09/09/2026, os seis torrents que armavam o alerta em Nodes com a porta verificada de fora e aberta tinham de 2 a 63 seeds por leecher; dos 27 alertas abertos entre 08/09 e 30/09, só o primeiro tem um problema real documentado. A regra pergunta *"alguém provavelmente precisaria de mim?"*, não *"alguém quer esse conteúdo?"*. Um torrent só conta quando `num_seeds <= num_leechers × alerting.incoming_demand_seeds_per_leecher` (default 1):
>
> | Swarm (leechers / seeds) | Conta com o default? |
> |---|---|
> | 1 / 0 | **sim** — o leecher não tem de quem baixar |
> | 1 / 1 | **sim** — o único seed provavelmente é o próprio Node |
> | 3 / 3 | **sim** — a igualdade conta (a regra é `<=`) |
> | 1 / 2, 3 / 17, 1 / 63 | não |
> | `num_leechers` ou `num_seeds` NULL | não — desconhecido nunca acusa |
>
> `num_seeds` é o total do swarm segundo o tracker e pode já incluir o próprio Node. A demanda é do **Node**, não do torrent: basta **um** swarm carente entre os dele. O alerta carrega, no payload, só o que armou a regra (`demand_torrents`, `demand_leechers`, `demand_seeds`) — é o único rastro de *por que* abriu, já que não há histórico de peers no banco.
>
> **Texto que não envelhece (#203).** A mensagem do `NODE_INBOUND_SILENT` é fixa e não leva endereço nem duração: é gravada uma vez, na abertura, e o alerta pode ficar aberto por dias — o endereço do Node mudou quase 38h depois da abertura e o painel seguiu mandando liberar um endereço que não existia mais. O que varia fica no payload (`public_ipv6` e `bt_port` **da abertura**, `last_incoming_at`, `hostname`, a demanda). O **Portal** mostra o endereço **atual** do Node ao lado do alerta aberto, e a **notificação** (Telegram/Discord/webhook) leva o do payload como campo "Endereço do Node" — ela é um instante, então o da abertura está certo ali.
>
> ⚠️ **`NODE_IPV6_CHANGED` é ALERTA, não Evento.** A troca em si é fato pontual, mas a CONSEQUÊNCIA — "as regras de firewall estão desatualizadas" — é condição que persiste até alguém agir. O resolve só é possível por causa do sinal que decai da #155: a pergunta certa não é "o endereço parou de mudar" e sim "o firewall voltou a deixar entrar", e é `last_incoming_at` posterior à troca que responde isso. Primeiro endereço de um Node (NULL → valor) é **descoberta**, não rotação, e nunca alerta.
>
> ⚠️ **Rotação seguinte com o alerta ainda `OPEN` ATUALIZA, não é engolida (#209).** Sem demanda no swarm, um Node pode não receber entrada por dias, e o alerta original fica `OPEN` esperando o resolve. Se o IPv6 rotacionar de novo nesse intervalo, o dedup por `(code, node_id)` do `Open()` descartaria a detecção em silêncio — o webhook do n8n nunca saberia do endereço mais recente, e ficaria reagindo pra sempre a um endereço que já não é o problema. `evaluateIPv6Changed` cobre isso chamando `Service.UpdatePayload`, que reescreve mensagem/payload da MESMA linha (preserva `opened_at`) e notifica de novo pelo canal de abertura — nunca pelo de resolução, porque nada foi corrigido.
>
>
> ⚠️ **A abertura carrega a negação do resolve, e isso não é redundância (#204).** Nada
> limpa `previous_public_ipv6`, então "o endereço mudou" é verdade **para sempre** depois
> de uma troca — diferente de todas as outras regras deste worker, cujo resolve é a
> negação da abertura sobre a mesma variável. Sem o `AND` que exclui quem já recebeu
> entrada, um Node que volta a ser alcançável satisfaz abertura e resolve na MESMA
> passada: como o dedup de abertura só vale enquanto o alerta está OPEN, ele reabre a
> cada ciclo do worker e notifica em todos. Medido em 09/09/2026: uma notificação a cada
> 30s por Node.

> **Contrato de máquina (`schema: 1`).** Só este alerta leva bloco `data` no canal webhook, porque existe para disparar automação. `fields` é para humano ler e muda quando alguém melhora uma frase; `data` é estável. `data.ipv6.prefix_previous`/`prefix_current` vêm **calculados** pelo Control Plane: a regra de firewall precisa do objeto de rede `/64`, e foi justamente o endereço literal que causou o incidente — deixar o consumidor derivar por manipulação de string plantaria o mesmo bug do outro lado. `schema` só sobe em mudança quebrante.

> **Limite conhecido:** a demanda é avaliada no *instante* da passada, não acumulada sobre a janela — não há histórico de leechers no banco (`torrent_stat_samples` guarda bytes e velocidade, não peers). Node sem demanda no momento da avaliação não alerta, ainda que tenha havido demanda durante a janela: prefere-se calar a gritar errado. A ponderação por concorrência reduz o ruído desse limite, não o elimina: um leecher que acabou de chegar a um swarm carente ainda arma uma regra cuja janela já estava vencida. `last_incoming_at` NULL (nunca vimos entrada) também não alerta, porque não distingue "recém-implantado" de "sempre inalcançável" — esse é o caso de onboarding, da regra anterior.

> `DISK_LOW`/`SMART_CRITICAL` (inventário de discos/SMART) ficam para o backlog F1 — hardware ≠ falta de espaço; `NODE_DISK_FULL` cobre a falta de espaço com o dado que o heartbeat já traz.

## 3.3 Workers internos (`workers.*`)

Intervalos antes hardcoded no `main.go`, agora tunáveis (lidos por um *provider* a cada ciclo).

| Regra (key) | Default | Unidade | Env de seed | Descrição / racional |
|---|---|---|---|---|
| `workers.reconciler_interval` | 15s | duração | — | Período do Migration Reconciler (ADR-018) que avança/finaliza migrações em voo **e promove a fila** (ADR-031). |
| `workers.prom_writer_interval` | 30s | duração | — | Período de reescrita do arquivo de targets (file_sd) do Prometheus (ADR-017). |
| `migration.max_concurrent` | 1 | inteiro | — | Quantas migrações transferem dados **ao mesmo tempo** (ADR-031); as demais ficam `QUEUED`. Teto **global do cluster** — o recurso protegido é o I/O do disco, e um drain sem trava dispara N cópias simultâneas justamente no Node em apuros. |
| `migration.recheck_stall_window` | 10m | duração | — | Quanto o recheck no Node destino pode ficar **sem avançar** antes de a migração ser abortada como travada (#79/#87). É a proteção de verdade: enquanto o progresso sobe a espera continua, por mais lento que seja o disco (10 GB no HD USB do node-03 levam ~15 min andando). Quem observa é o **Control Plane**, a cada passada do reconciler — não o Agent. Mínimo de 60s: não existe "desligar", sem watchdog um recheck travado ocuparia a vaga de `migration.max_concurrent` para sempre. |
| `migration.recheck_max_wait` | 6h | duração | — | Teto absoluto, contado do **fim da cópia** (`DONE` do `migrate_pull`), para o destino comprovar ter o que a origem tinha (#87). Só pega um Engine que reporte progresso oscilante para sempre. Pôde subir de 2h para 6h porque a espera deixou de segurar a fila de comandos do Node; o custo de um teto alto é a vaga de `migration.max_concurrent` ficar ocupada, não um Node surdo. |
| `metrics.cluster_sample_interval` | 5m | duração | `SEEDGRID_METRICS_CLUSTER_SAMPLE_INTERVAL` | Período entre snapshots do throughput acumulado do cluster (F2), gravados em `cluster_stat_samples` pelo worker `cluster-stats`. Base do gráfico de evolução no Dashboard. |
| `metrics.cluster_sample_retention` | 0 (para sempre) | duração | `SEEDGRID_METRICS_CLUSTER_SAMPLE_RETENTION` | Retenção dos snapshots do cluster. **0 = manter para sempre** (a tabela é minúscula, ~1 linha/intervalo). Para o filtro de **1 ano** funcionar, manter ≥ 1 ano ou 0. |

> 📈 **Throughput acumulado do cluster (F2).** O worker `cluster-stats` grava, a cada `metrics.cluster_sample_interval`, a **soma dos totais cumulativos** (`downloaded_total`/`uploaded_total`) de **todos** os torrents — inclusive removidos por soft-delete — em `cluster_stat_samples`. Somar os removidos mantém a curva **monotônica** ("crescimento acumulado"): um torrent apagado congela sua contribuição em vez de sumir da soma. Diferente de `torrent_stat_samples` (por torrent, bytes de sessão, 7d) e do Prometheus (30d, sem métrica de bytes) — nenhum cobre 6 meses/1 ano; por isso a tabela própria com retenção longa. O endpoint `GET /api/v1/metrics/cluster-throughput?range=<dur>` lê a série com **downsampling por `date_bin`** (≤ ~400 pontos, bucket ≥ 1 min) e devolve duas séries (Baixado/Enviado) no formato do componente `Chart`. O **Dashboard** mostra o gráfico com filtros de período (1h … 1 ano). ⚠️ Migração conta ~2× (origem soft-deletada + destino) — é tendência agregada, não contábil.

## 3.3.1 Migração de torrents — regras não parametrizadas (ADR-018/031)

Regras que vivem no domínio (`control-plane/internal/migration`, `agent/internal/commands`,
`agent/internal/transfer`) e **não** são tunáveis, indexadas aqui conforme o §1.

| Regra | Onde | Racional |
|---|---|---|
| Origem só é removida após o destino confirmar | `migration.complete()`, chamado só por `verifyAfterPull()` | Migração sem perda (ADR-018): se a transferência falhar, o torrent permanece na origem. Desde a #87 "confirmar" é o CP observar o destino — o `DONE` do pull sozinho não basta (ver abaixo). |
| Origem é **parada antes** de servir o conteúdo — e sem pausa confirmada não se autoriza | `commands.migrateServe()` → `pauseForMigration()` | Copiar um torrent que o Engine ainda grava lê blocos a meio caminho: o tar sai inconsistente e o destino só descobre no recheck. A pausa vem **antes** do `Authorize` (sem token, o destino leva 401 e não há como puxar dado em voo); se o Engine não confirmar o estado parado em 10s, o comando falha — migração abortada é recuperável, dado corrompido não. |
| Torrent com **migração em voo** sai do alcance da admissão | `admission.notMigratingPred` (em `hold`/`promotableHolds`/`adoptOrphanPauses`/`rotateDead`/`rotateSlow`) | A pausa da migração parece uma "pausa manual órfã" para o Admission Worker, que a adotava e **promovia no ciclo seguinte** — retomando o download no meio da cópia, exatamente o dado em voo que a pausa evita (observado ao vivo em 07/08/2026: pausa às 06:27:41, resume da admissão 15s depois). Enquanto a migração está `QUEUED`/`PENDING`, quem manda naquele torrent é o orquestrador de migração; terminada (COMPLETED/FAILED), ele volta a ser candidato normal. `countActive` **não** aplica o filtro: se o torrent ainda estiver anunciando, ocupa slot real no tracker e tem de contar no L. |
| Migração abortada **retoma** o torrent na origem | `migration.resumeSource()` (chamado por `fail()`) | Complemento da regra acima: "permanece na origem" só é verdade se ele voltar a rodar, senão um drain malsucedido deixa torrents parados em silêncio. Não retoma torrent em `admission_paused_at` (quem manda na fila é o Admission Worker, ADR-021/024) nem migração que nem chegou a ser promovida (sem `serve_command_id`, nada foi pausado). |
| Critério de sucesso = **progresso da origem**, não "está seedando" | `migration.judgeRecheck()` + `min_progress` no payload (no Agent, só o caminho legado: `commands.waitProgress()`) | ADR-031: torrent a 60% migra a 60% e segue baixando no destino. Exigir SEEDING reprovava toda migração de parcial, que é justamente o caso de drenar Node sem espaço. |
| Tolerância de 0,5% no piso de progresso | `migration.pieceTolerance` (espelha `commands.pieceTolerance`, usada no caminho legado) | O recheck invalida a **peça de fronteira** quando o download parou no meio dela; perder uma peça na borda é normal. |
| `min_progress` ausente ⇒ exige torrent completo | `commands.minProgressOf()` | Retrocompatibilidade: Agent novo com CP antigo cai no comportamento da ADR-018. Se isso regredisse para 0, uma migração vazia passaria como sucesso. |
| Conteúdo vazio na origem ⇒ **409 antes de qualquer cabeçalho** | `transfer.handleTransfer()` + `isEmptyContent()` | Causa raiz do incidente 2026-07-18: o 200 era escrito antes do stat, e o destino recebia um tar válido e vazio — pull "bem-sucedido" sem um byte, seguido de re-download integral do swarm (queima ratio). |
| 409 no pull **não** é retentado | `commands.pullWithRetry()` + `transfer.ErrNothingToTransfer` | Condição permanente; insistir 90s só adia o erro que o operador precisa ver. O 401 continua retentável (corrida normal entre o pull do destino e o serve da origem). |
| Token emitido só na **promoção** | `migration.promoteOne()` | Seu TTL (30min) correria durante a espera na fila e expiraria antes de a transferência começar. Desde a #87 o TTL cobre só a **transferência**: com o Agent novo o `DONE` do pull vem ao fim da cópia, e a espera do recheck tem prazo próprio (abaixo). |
| Uma migração em voo por torrent cobre `QUEUED`+`PENDING` | índice `torrent_migrations_one_inflight` | Evita dois destinos disputando o mesmo torrent, inclusive enquanto ele aguarda vaga. |
| **`DONE` do `migrate_pull` ≠ destino verificado** | `migration.Reconcile()` → `verifyAfterPull()` | #87: o Agent novo responde `DONE` assim que a cópia termina e o recheck é disparado, para não segurar a fila de comandos do Node (o Poller é serial). O CP observa a linha do hash no **Node destino** (alimentada pelo inventário) e só conclui quando ela comprova o piso; o Agent antigo, que só responde `DONE` após o recheck, cai no mesmo caminho e já encontra o destino pronto. O CP **não ramifica** pela versão do Agent. |
| Só vale leitura do destino **posterior ao `DONE`** | `migration.judgeRecheck()` (`last_synced_at > finished_at` do pull) | Uma linha sincronizada antes pode ser de antes do recheck (ou de outra cópia). Sem leitura nova, espera — e a ausência de leitura conta como estagnação. |
| Só um estado que **prova o progresso** comprova o piso | `migration.provesProgress()` (lista de permissão: `SEEDING`/`DOWNLOADING`/`QUEUED`/`PAUSED`) | Durante o recheck (`CHECKING`) o qBittorrent reporta como progresso o avanço da **verificação**: um parcial de 60% com o recheck em 70% "passaria" o piso sem ter terminado de verificar. `MOVING`/`UNKNOWN`/`ERROR` também não provam nada — lista de permissão, para que um estado não mapeado não vire prova por omissão. |
| **Watchdog do recheck vive no CP**: estagnação + teto | `migration.judgeRecheck()`, `migration.recheck_stall_window` / `migration.recheck_max_wait` | #79 movida pela #87. Parou de subir por `recheck_stall_window` → `FAILED` "recheck no destino travado"; passou de `recheck_max_wait` desde o `DONE` → `FAILED` "destino não atingiu o progresso da origem". Mesmo efeito de antes: `fail()` retoma a origem e descarta a cópia do destino. Estado em memória: reiniciar o CP só renova a janela de estagnação. |
| O Agent só devolve `DONE` cedo se o CP declarou **`async_recheck`** | `migration.promoteOne()` (payload) + `commands.migratePull()` | Campo aditivo. Sem ele (CP anterior à #87) o Agent mantém a espera e só responde `DONE` com o destino verificado — senão aquele CP leria `DONE` como "verificado" e removeria a origem sem a prova. |
| Migração em voo **se cancela**, pelo mesmo caminho seguro do watchdog | `migration.Cancel()` → `abort()` (`POST /api/v1/migrations/{id}/cancel`, `torrents:migrate`) | #87: a recusa acima só é aceitável se o operador tiver saída — e a verificação pode durar até `migration.recheck_max_wait`. `FAILED` com `error = "cancelada pelo operador"` (sem estado novo no enum): origem retomada, cópia do destino descartada. `serve`/`pull` ainda não reivindicados pelo Agent são baixados e nunca rodam (nada a desfazer por eles); durante a cópia o comando não é interrompido e a limpeza é enfileirada atrás dele. `409 migration_finished`/`migration_already_cancelled` se já terminou; repetir não reexecuta nada. |
| Concluir e cancelar **nunca vencem juntos** | `complete()` (`WHERE status = 'PENDING'`) e `abort()` (`WHERE status IN ('QUEUED','PENDING')`) | Sem a condição, um cancelamento que chegasse no instante em que o reconciler conclui sobrescreveria `COMPLETED` e mandaria apagar a ÚNICA cópia (a do destino), com a origem já removida. Quem perde não tem efeito algum (rollback / nada enfileirado). |
| Torrent em migração `PENDING` **recusa** ação do operador | `torrent.inMigration()` em `Command()`/`Remove()` → `409 migration_in_flight`; `migration.Migrate()` → `ErrAlreadyMigrating` | #87: sem o bloqueio do Poller, um pause/resume/recheck/remove executaria **no meio** do recheck do destino. Vale para o torrent migrado e para a **cópia do mesmo hash** que o destino adotou. `QUEUED` não trava (nada mexeu ainda; um drain espera horas). Complementa a regra da admissão acima, que só cobria a linha da origem. A mensagem do 409 diz que dá para cancelar a migração e qual. |

## 3.4 Publicação em tracker (`publish.*`) — ADR-020 / ADR-026

Parâmetros do fluxo de cadastro em tracker UNIT3D. Aplicado em
`control-plane/internal/settings/catalog.go`.

| Regra (key) | Default | Unidade | Env de seed | Descrição / racional |
|---|---|---|---|---|
| `publish.screenshot_count` | 4 | inteiro | `SEEDGRID_SCREENSHOT_COUNT` | Screenshots por upload — quantos frames extrair do arquivo de vídeo (via Agent/ffmpeg) ao gerar screenshots para publicação (ADR-026). |

> **BBCode de screenshots (backlog F1).** As screenshots entram na descrição no formato convencional de tracker privado: cabeçalho `[b]Screenshots[/b]` + bloco `[center]` com **thumbnails clicáveis** `[url=imagem-cheia][img]thumbnail[/img][/url]` (um por linha) — consistente com o pôster/seções do resto da ficha e evita imagem gigante inline (causa comum de "postponed"). O `imgbb` devolve o thumbnail (`data.thumb.url`, com fallback `medium`→cheia). No Portal, o botão **✨ Gerar BBCode** monta a **ficha completa** num clique (pôster + sinopse + tags + screenshots + MediaInfo) com uma **barra de etapas** mostrando cada estágio real: extrair frames no Node (comando `extract_screenshots` com ciclo próprio, `POST .../screenshots/extract` → command_id, acompanhado por `GET /commands/{id}`), hospedar no imgbb + montar o BBCode (`POST /screenshots/render`). As screenshots são **best-effort** — se a extração falhar (torrent incompleto, ffmpeg ausente, sem chave imgbb), a etapa fica âmbar e a ficha vem sem elas + aviso, pois screenshot não bloqueia o upload. O botão **📸 Gerar screenshots** isolado segue no fluxo em um passo (`POST .../screenshots`).

## 3.5 Perfil de tracker e descoberta de taxonomia — regras não parametrizadas (ADR-037 / #196)

Constantes de código, não tunáveis: são limites de **segurança de entrada** e da forma da
varredura, e afrouxá-las é uma decisão de código, não uma preferência do operador.
Aplicado em `control-plane/internal/trackerprofile` e `internal/unit3d/discover.go`.

| Regra | Valor | Onde | Racional |
|---|---|---|---|
| Descoberta só **propõe**; aplicar é outra operação | — | `discover-taxonomy` × `apply-taxonomy` | O catálogo é de terceiro e incompleto: nada vira perfil sem o operador conferir. Só o aplicar entra na Auditoria (ADR-025). |
| Descoberta **nunca sobrescreve** termo `manual`, nunca remove e nunca mexe em `kind` | — | `trackerprofile.Merge` (no servidor, sobre o estado travado) | Rodar de novo tem de ser seguro. Checar só no Portal não bastaria: o estado pode ter mudado entre a proposta e o aplicar. |
| Termo ausente do catálogo fica **intocado** | — | `Propose`/`Merge` | Ausência não é evidência — o catálogo só mostra o que alguém postou. |
| Páginas varridas | default **8**, teto **20** (50 itens cada) | `unit3d.DefaultDiscoverPages`/`MaxDiscoverPages` | ~1,3 s por página no samaritano (medido); a cota de requisições por token é do operador e é dividida com o Portal. |
| Orçamento de tempo da varredura | **20 s** | `unit3d.DiscoverBudget` | O servidor HTTP fecha a resposta em 30 s: uma varredura lenta termina antes, com resultado parcial, em vez de ser cortada sem resposta. |
| Limite de taxa (`429`) ou prazo estourado no meio | devolve o **parcial** (`stopped_reason`) | `unit3d.Discover` | As páginas já colhidas continuam valendo. Só falha (`502`) se nenhuma chegou. |
| `id` de termo | inteiro em `1..1000000`, único por eixo | `trackerprofile.ValidateInput`/`CleanName` | IDs de UNIT3D são auto-incremento de tabelas pequenas; o teto barra lixo e estouro. |
| Nome de termo | 1 a 64 caracteres, UTF-8 válido, espaços colapsados, **sem controle nem formatação invisível** (U+202E, zero-width…) | `trackerprofile.CleanName` | É só rótulo, mas um rótulo hostil inverte o texto na tela, faz dois termos "iguais" serem diferentes e quebra log/terminal. |
| Termos por eixo | no máximo **256** | `trackerprofile.MaxTermsPerAxis` | Limite de tamanho do documento. |
| Eixos | conjunto **fechado** (`category`, `type`, `resolution`) | `trackerprofile.Axes` | Eixo novo é código e `schema_version`, não um campo livre que chega num JSON. |
| `kind` | `movie`/`tv`/`game`, só em `category` | `trackerprofile.validateKind` | O Portal age sobre ele (temporada/episódio, busca de jogo); valor fora do conjunto não pode chegar lá. |
| Corpo das requisições de perfil | no máximo **1 MiB**; campo desconhecido é `400` | `decodeProfileBody` | Perfil é conteúdo não confiável. |
| Par do catálogo que não valida | **descartado e contado** (`rejected`) | `trackerprofile.Propose` | O operador vê que houve descarte, em vez de uma proposta que parece completa. |
| Erro de requisição ao tracker não leva a URL | — | `unit3d.stripURL`, em `Upload`, `Browse`, `FetchTorrent` e `fetchTorrentBytes` (#220) | A URL da requisição carrega o `api_token` na query (e a passkey, no `download_link`); o erro do `http.Client` — e o do `url.Parse` ao montar a requisição — a incluía inteira, e a mensagem vai para o Portal e para o log. |

> **A descoberta é incompleta por definição.** Só enxerga o que já foi postado: categoria, tipo ou
> resolução que ninguém usou não aparece, e o catálogo vem do mais recente para o mais antigo. Toda
> resposta traz `notice`, e a interface não pode sugerir o contrário.

## 3.6 Cadeia de upload de screenshots — regras não parametrizadas (ADR-040 / #193)

Aplicado em `control-plane/internal/imagehost` e `internal/torrent`.

| Regra | Valor | Onde | Racional |
|---|---|---|---|
| Ordem da cadeia | host do tracker → imgbb | `torrent.uploadChain` | O host do site tem prioridade (regra do site, resiliência); o imgbb é o último elo (contrato global da ADR-030 inalterado). |
| Entrada do host do tracker na cadeia | seção `image_host` presente + chave cadastrada + `Host()` == `image_host_key_host` (chave sem host gravado também é pulada) | `torrent.trackerCandidate` | A credencial vai **só para onde o operador a viu indo**; perfil que muda de host desliga a chave até o operador cadastrá-la de novo. |
| Tudo ou nada por host | falha em qualquer frame passa a vez ao próximo, que recomeça do frame 1 | `imagehost.Chain` | Os frames de uma publicação nunca misturam hosts — melhor nenhuma descrição que uma pela metade (mesma regra da ADR-030). |
| Orçamento total de upload | **25 s** | `torrent.uploadChain` | Abaixo dos 30 s do `WriteTimeout` do CP e dos 60 s do proxy no `/api/`. |
| Repartição do tempo | parte justa do restante por host; o que um host não usa passa ao seguinte | `imagehost.Chain` | Um host que trava não consome o tempo do fallback. |
| Motivo da falha | conjunto **fechado e sanitizado** (`chave cadastrada para outro host`, `HTTP <status>`, `redirecionamento recusado`, `tempo esgotado`, `falha de conexão`, `resposta sem URL`, `URL inválida na resposta`) | `imagehost.reasonFor` | Nunca o texto cru do erro: ele carrega a URL (no imgbb, a chave na query) e pode trazer texto de terceiro. |
| Validar URL antes de aceitar | absoluta, `https`, até 2048 caracteres, sem espaço/controle/`[`/`]`/aspas | `imagehost.ValidateURL` | A URL é interpolada em `[url=…][img]…[/img][/url]`; sem validação, a resposta de um host injetaria markup na descrição. Vale para o imgbb também. |
| Todos os hosts falharam | `502 screenshots_failed` com os motivos por host na mensagem | handler de screenshots | O operador continua podendo colar o BBCode à mão (ADR-026). |
| Nenhum host configurado | o mesmo erro de antes da #193 (`subir screenshots: imgbb sem api_key configurada`) | `torrent.uploadChain` | Sem `tracker_id` e sem chave imgbb, nada há para tentar. |
| Sem `tracker_id` na requisição | só o imgbb | handlers de screenshots | Comportamento antigo preservado para quem chama a API sem o campo. |
| Cada falha vira entrada no Feed | kind `screenshot_host_failed`, com torrent (no `render`, achado pelo Node e hash do comando), `Detail` `{tracker_id, host, reason, next}`; `next` = o candidato seguinte na ordem da cadeia, vazio no último | `torrent.screenshotFailureEntries` | A falha do host do site deixa de ser invisível só porque o imgbb salvou a publicação. |
| `host` no Feed e na resposta | o **nome DNS** do `upload_url`, nunca o `name` do perfil | `Spec.Host()` | O `name` é texto de terceiro (rótulo); o nome DNS é o identificador do host. |
| Motivo da falha do imgbb | status fora de 2xx vira `HTTP <status>`; 2xx sem URL vira `resposta sem URL` | `torrent.imgbbChainError` | O cliente do imgbb tem erros tipados (`imgbb.StatusError`, `imgbb.ErrNoURL`); sem a tradução, toda falha dele sairia "falha de conexão". |
| Erro de rede do imgbb sem a chave | `stripURL` corta a URL do `*url.Error` | `imgbb.uploadOne` | A chave viaja na query (`?key=`); o erro do `http.Client` carregava a URL inteira (defeito (i) da ADR-040). |
| Redirecionamento não é seguido | `CheckRedirect` devolvendo erro; 3xx é falha | `imagehost.Upload` | O Go repassa header personalizado em redirect; seguir levaria a chave para onde ninguém a cadastrou. |

As regras de **forma** da seção `image_host` (campos, tamanhos, `https`, hosts recusados) não estão
aqui: valem as de `13 - API.md` («Forma fechada e validação») e da ADR-040, aplicadas por
`trackerprofile.CleanImageHost` em toda escrita (edição, import).

> **Vínculo chave↔host.** A chave do host de imagem (`trackers.image_host_key`, camada 3) é gravada
> junto com o host do `upload_url` vigente (`image_host_key_host`). Na hora de usar, o host do
> tracker só entra na cadeia se o host atual do perfil for **exatamente** o gravado; senão é pulado
> com o motivo "chave cadastrada para outro host". É o que impede um perfil importado de trocar o
> `upload_url` e levar a chave junto.

# **4. Validação**

Toda escrita (`PUT /api/v1/settings/{key}`) é validada contra o `Spec` do catálogo: parsing
pelo tipo (duração aceita `5m`/`90s`/`168h`; inteiros e floats decimais) e limites `Min`/`Max`
(para duração, em segundos). Valor fora da faixa é recusado com `400 invalid_value` e nada é
gravado. Mudanças são auditadas como Eventos `SETTING_UPDATED` / `SETTING_RESET`.

# **5. Relação com outros documentos**

- **ADR-023** — decisão arquitetural (catálogo em código + overrides no banco + hot-reload).
- **07 - Workers** — os Workers que consomem estes tunáveis.
- **13 - API** — endpoints `/api/v1/settings`.
- **11 - Portal** — página Configurações.
- **12 - Database / Schema** — tabela `settings`.
