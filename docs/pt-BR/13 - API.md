# **SeedGrid**
**API Specification**
Version: 1.0
Status: Draft
Document: 13 - API.md
# **1. Objetivo**
A API representa o contrato oficial entre o Control Plane e todos os consumidores da plataforma.
Todo acesso ao SeedGrid deverá ocorrer através da API.
Nenhum componente externo deverá acessar diretamente o banco de dados ou os serviços internos.
# **2. Consumidores**
A API deverá atender os seguintes consumidores:
Portal Web
Agent
CLI
SDK
Integrações futuras
Todos utilizarão a mesma API.
# **3. Princípios**
A API deverá ser:
Consistente.
Versionada.
Documentada.
Determinística.
Segura.
Estável.
# **4. Arquitetura**
Portal
│
──────────────────────────
CLI
│
──────────────────────────
Agent
│
──────────────────────────
Integrações
│
──────────────────────────
API Gateway
│
──────────────────────────
Control Plane
# **5. Versionamento**
Toda API deverá possuir versão explícita.
Exemplo
/api/v1/
Mudanças incompatíveis deverão gerar uma nova versão.
# **6. Recursos**
Os recursos principais da API serão:
Cluster
Nodes
Torrents
Trackers
Users
Alerts
Events
Settings
Authentication
Scheduler
Monitoring
Inventory
# **7. Organização**
A API deverá ser organizada por domínio.
Exemplo
/api/v1/nodes
/api/v1/torrents
/api/v1/alerts
/api/v1/events
/api/v1/activities   (feed de orquestração — ADR-022; ?limit, ?torrent_id)

> **Webhooks internos (#116):** `POST /api/v1/integrations/alertmanager` e `/recovery`
> exigem o shared secret `SEEDGRID_INTEGRATION_TOKEN` (header `X-Seedgrid-Integration-Token`
> ou `Authorization: Bearer`). O `scripts/bootstrap-controller.sh` gera o token e o compartilha com o
> Alertmanager (`credentials_file`); sem token configurado os endpoints seguem abertos
> (compat) com warning no log.

> **Limites e paginação (#115):** toda listagem com `?limit` tem teto de **500** por
> requisição (default 100). `GET /api/v1/torrents` aceita `?limit`/`?offset` opcionais
> (ausentes = lista completa), devolve `total` no corpo e usa **ETag condicional**
> (`If-None-Match` → 304 quando nada mudou — o polling do Portal deixa de re-baixar a
> lista inteira a cada tick).
/api/v1/settings     (tunáveis dinâmicos — ADR-023)

**Settings (ADR-023).** Painel de configuração com hot-reload:
- `GET /api/v1/settings` — tunáveis agrupados por categoria (admission/alerting/workers), cada um com `key`, `type`, `label`, `description`, `unit`, `value` (efetivo), `default`, `source` (`db`/`default`), `min`/`max`.
- `PUT /api/v1/settings/{key}` — corpo `{"value": "..."}`; valida (tipo + min/max), grava, aplica em runtime e audita (Evento `SETTING_UPDATED`). `400 invalid_value` se fora da faixa.
- `DELETE /api/v1/settings/{key}` — reset ao default do catálogo (Evento `SETTING_RESET`).

Todas protegidas (operador autenticado). Ver `25 - Business Rules.md` para o catálogo.

- `GET /api/v1/settings` agora também traz `per_node: true` nos itens que são **valor de cluster** de um tunável do Agent/Engine (ADR-039, abaixo), e a categoria `agent` passa a se chamar **Agent / Engine**.
- `GET /api/v1/settings/overrides` (`settings:read`) — os Nodes com **valor próprio**: `{"data": [{"node_id", "node_label", "key", "label", "unit", "value", "cluster_value", "updated_at", "updated_by"}]}`. É o que responde, na tela de Configurações, "se eu mudar o valor de cluster, quem **não** é afetado?".

**Configuração do Agent/Engine por Node (ADR-039, #144).** O comportamento do Agent e do Engine (`agent.heartbeat_interval`, `agent.command_poll_interval`, `agent.log_level`, `engine.*` da fila) é **valor de cluster** (as rotas `/settings` acima, `settings:write`) com **override por Node**. Precedência: override do Node > valor de cluster > env do Node (fallback do Agent) > default do código. Tabela das chaves em `25 - Business Rules.md` §3.1.1b.
- `GET /api/v1/nodes/{id}/settings` (`nodes:read`) — o que vale **neste Node** e de onde vem: `{"node_id", "status", "effective_version", "applied_version", "applied_at", "seeded_at", "items": [...]}`. Cada item: `key`, `type`, `label`, `description`, `unit`, `min`/`max`, `options`, `value` (**efetivo** — o que o Agent recebe), `source` (`node`|`cluster`), `cluster_value` (o que o Node teria sem override), `cluster_source` (`db` = o operador definiu | `default` = catálogo), `override` (`null` se não há). `status`: `in_sync` (o Agent aplicou a versão de hoje), `pending` (mudou e o Agent ainda não confirmou) ou `unsupported` (o Agent nunca reportou — anterior à ADR-039, segue no `.env`). `404` se o Node não existe (ou o `id` não é UUID).
- `PUT /api/v1/nodes/{id}/settings/{key}` (`nodes:write`) — corpo `{"value": "..."}`. Valida pelo **mesmo `Spec`** do valor de cluster (tipo, faixa, opções): `400 invalid_value`. `400 not_per_node` para um tunável do Control Plane (ex.: `admission.interval`); `404 setting_not_found` para chave desconhecida e `404 node_not_found` para Node inexistente; corpo com campo desconhecido é `400 invalid_body`. Resposta: o item resolvido. Vale no próximo heartbeat do Agent, sem restart. **Auditado com o antes e o depois** (ADR-025): «Definiu a configuração «Downloads simultâneos por Node» do Node «node-04» para 3 (antes seguia o cluster)».
- `DELETE /api/v1/nodes/{id}/settings/{key}` (`nodes:write`) — remove o override: o Node volta ao valor de cluster. Idempotente. Auditado («Removeu o valor próprio de «…» do Node «…» (era 3; volta ao cluster: 10)»).
- Mudar o **valor de cluster** de uma chave `per_node` (`PUT /settings/{key}`) vale para todo Node **sem** override; a trilha de auditoria diz isso.

**Modo de operação (ADR-024).** Controle do operador sobre a admissão (gate global + modo por-Node):
- `GET /api/v1/nodes` — além de `data` (Nodes, cada um com `operation_mode` `sg`/`manual`), devolve `cluster_operation_mode` (`full_auto`/`flex`), o gate global.
- `PUT /api/v1/cluster/operation-mode` — corpo `{"mode": "full_auto"|"flex"}`. Alterna o gate; ao voltar a `full_auto`, reseta todos os Nodes para `sg` (Evento `CLUSTER_MODE_UPDATED`). `400 invalid_mode` para valor inválido.
- `PUT /api/v1/nodes/{id}/operation-mode` — corpo `{"mode": "sg"|"manual"}`. Só permitido com o cluster em `flex` (senão `409 gate_locked`); `404` se o Node não existe (Evento `NODE_MODE_UPDATED`).
- Um Node é tratado como **Manual** (operador soberano; o controlador de admissão não o toca) só quando o gate é `flex` E o Node é `manual`. Os downloads de um Node Manual **contam** no teto L (invariante).

**Apelido do Node (#145).** O Node entra no cluster com o hostname do SO, e não há razão para mexer na máquina só para nomeá-la. O **apelido** (`display_name`) é um nome que vive **só dentro do SeedGrid**: o hostname do SO continua sendo o dado técnico, intocado, e renomear não envia nada ao Node nem reinicia o Agent. O Portal exibe o apelido onde mostra o nome de um Node, caindo no hostname quando não há.
- `POST /api/v1/enroll-tokens` (`enroll:write`) — corpo **opcional** `{"display_name": "..."}`. O apelido viaja **no join token** e é aplicado quando o Node se registra: o token já é o que amarra o Node que está subindo à intenção do operador, então o nome chega ao registro sem o `bootstrap-node.sh` nem o Agent carregarem nada novo. Sem corpo é o uso original (sem apelido). A resposta ecoa `display_name` já normalizado. `409 display_name_taken` se o nome já é de um Node; `400 invalid_display_name` se não passa na validação.
- `PUT /api/v1/nodes/{id}/display-name` (`nodes:write`) — corpo `{"display_name": "..."}`. O campo é **obrigatório**: `""` remove o apelido (volta ao hostname), mas um corpo sem o campo é `400 invalid_request` — apagar por omissão seria fácil demais de fazer sem querer. `404` se o Node não existe; `409 display_name_taken`; `400 invalid_display_name`. Resposta `{"display_name": "<valor gravado>"}`.
- `GET /api/v1/nodes` já devolvia `display_name`; passa a ser preenchido, e a lista é ordenada pelo **nome exibido** (apelido, ou hostname se não há). Os payloads que carregam o hostname de um Node ganharam o apelido ao lado: `GET /api/v1/activities` → `node_display_name`; `GET /api/v1/analytics` → `display_name` em cada torrent e em `nodes[]`.

Regras do apelido:
- **Normalização:** espaços nas pontas saem e qualquer sequência de espaço/tab/quebra de linha vira **um espaço** (dois nomes que só diferem em espaço invisível são o mesmo nome para o operador). Máximo de **64 caracteres** (não bytes). Só caracteres imprimíveis: controle e zero-width são recusados — fabricariam "duplicados" visualmente idênticos que a checagem de unicidade não pegaria.
- **Unicidade — decidida: sim.** Dois Nodes não removidos nunca exibem o mesmo rótulo. A comparação **não diferencia maiúsculas** e vale contra o apelido **e contra o hostname** dos outros Nodes (um apelido igual ao hostname de outra máquina seria o mesmo problema). Motivo: o nome é o que o operador lê e escolhe nos pickers de ações destrutivas (migrar, esvaziar, remover Node) e é a chave das séries nos gráficos — nome repetido é ambiguidade justo onde errar custa dado. O nome de um Node removido volta ao pool. Um índice único parcial (`uk_nodes_display_name_per_cluster`) é a garantia contra corrida; a checagem amigável (409) é feita antes, pelo Control Plane.
- **Registro nunca falha por causa do nome.** Entre emitir o token e o Node se registrar o nome pode ter sido tomado (ou dois tokens abertos pedirem o mesmo nome — permitido de propósito, senão fechar o modal sem copiar o comando bloquearia o nome por 1h). Nesses casos o Node **entra sem apelido** — o primeiro a registrar leva o nome —, o Evento `NODE_REGISTERED` avisa, e o operador renomeia depois pelo Portal. No registro com join token vale **o nome do token**; o `display_name` que o Agent mandar no corpo é ignorado (decisão do operador, não da máquina). Re-registro sem nome não apaga o apelido existente.
- **Auditoria (ADR-025):** a troca é registrada com o antes e o depois — «Renomeou o Node «Sala» para «Quarto»», «Deu o apelido…», «Removeu o apelido…» —, porque o valor antigo não existe mais em lugar nenhum depois do UPDATE. Emitir o token com apelido registra «Gerou um token de join para o Node «X»». Tentativa sem `nodes:write` é negada com 403 e também consta da trilha.
- **Fora do escopo (continuam pelo hostname):** os textos de Alertas, Eventos e notificações externas (Telegram/Discord/webhook) — os ~30 pontos que montam mensagem leem `nodes.hostname` direto, e misturar os dois nomes numa mesma trilha seria pior que manter o identificador técnico estável nela.

**Auditoria (ADR-025).** Trilha das ações do operador — quem fez o quê, quando, de onde:
- `GET /api/v1/audit` — trilha mais recente primeiro. `?actor` filtra por operador; `?limit` (default 100, teto 500). Cada item: `actor`, `action` (descrição amigável PT-BR, ex.: "Alterou a configuração «Node offline após» para 120s" — a reserva), `action_code` e `action_params` (de onde quem lê monta a frase no idioma dele, ADR-041; ausentes nos registros anteriores à #57), `method`, `path`, `status_code`, `source_ip`, `created_at`.
- A gravação é **automática**: um middleware sobre as rotas protegidas registra toda requisição **mutadora** (POST/PUT/DELETE/PATCH) do operador; o **login** (sucesso e falha) é registrado explicitamente. GET não é auditado. Best-effort — nunca bloqueia a resposta. Trilha separada de Alertas/Eventos/Activity.

**Screenshots para publicação (ADR-026/030/040).** Estende a publicação em tracker UNIT3D (ADR-020). São três rotas:

- `POST /api/v1/torrents/{id}/screenshots` — fluxo em um passo. Corpo opcional `{"tracker_id": "…"}` (vazio continua valendo; corpo malformado ou com campo desconhecido é `400 invalid_body`); o CP dispara o comando `extract_screenshots` (fila ADR-014) contra o Agent, aguarda os frames, sobe na **cadeia de hosts** (host do tracker → imgbb, ADR-040) e devolve o BBCode. Sucesso (200): `{"bbcode": "…", "host": "…", "failures": []}`.
- `POST /api/v1/torrents/{id}/screenshots/extract` — enfileira a extração no Node e devolve o `command_id` para o Portal acompanhar a etapa "Extraindo no Node" via `GET /commands/{id}`. Resposta 202: `{"command_id": "…"}`.
- `POST /api/v1/screenshots/render` — lê os frames de um comando `extract_screenshots` concluído, sobe na cadeia de hosts e devolve o BBCode. Corpo: `{"command_id": "…", "tracker_id": "…"}` (`tracker_id` opcional). Sucesso (200): `{"bbcode": "…", "host": "…", "failures": []}`.

**Cadeia de hosts (ADR-040, #193):** a ordem é host do tracker (seção `image_host` do perfil + chave cadastrada + host conferido) → imgbb (chave global). O primeiro host que hospedar **todos** os frames vence; os frames de uma publicação **nunca misturam hosts** (tudo ou nada). Cada falha vira entrada no Feed (`screenshot_host_failed`) com o motivo sanitizado e o próximo host da fila. Orçamento total de **25 s**, repartido entre os hosts. `host` na resposta é o nome DNS do vencedor; `failures` é **sempre** uma lista, `[{"host": "…", "reason": "…"}]`, com os hosts que falharam ou foram pulados antes do vencedor (`[]` quando o primeiro venceu). Na entrada do Feed, `next` é o host seguinte na ordem da cadeia (tenha ele falhado ou vencido), vazio no último; o torrent vai junto também no `render` (achado pelo Node e pelo hash do comando). Sem `tracker_id`, só o imgbb (comportamento antigo); sem `tracker_id` com host próprio e sem chave do imgbb, a resposta é o erro de antes da #193, `502 screenshots_failed` com `subir screenshots: imgbb sem api_key configurada`.

**Erros:** `404 torrent_not_found` para torrent inexistente e `404 tracker_not_found` para tracker inexistente, removido, de outro operador ou com id malformado — a posse do tracker é conferida **antes** de extrair frames ou subir qualquer coisa; `409 no_hash` para torrent ainda sem hash; `502 screenshots_failed` quando **todos** os hosts falharam (mensagem lista os motivos por host). O BBCode retornado passa pela conferência humana no Portal antes do `POST /api/v1/torrents/{id}/publish-tracker`.

**Evidência de duplicata no tracker (#186).** `GET /api/v1/trackers/{id}/dupes?name=…` — exige `trackers:read`. Busca no catálogo do tracker releases que possam ser duplicata do que está para ser publicado, e devolve `{"query": "…", "candidates": [{…, "same_year": true}]}`. A busca usa o **título inferido** do nome (`release.Parse`), não o nome padrão inteiro: o nome completo é específico demais e o filtro por nome do UNIT3D não casaria nada.

O contrato tem duas propriedades deliberadas, e as duas existem porque a busca é **auxílio à conferência humana, não veredito**:

- **Nunca falha "para cima".** Tracker fora do ar, erro de rede ou catálogo indisponível respondem **200** com `candidates` vazio e um campo `note` explicando. Um 5xx aqui faria uma falha de auxílio impedir a publicação — e a publicação é o que o operador veio fazer.
- **O ano marca, não filtra.** `same_year` sinaliza coincidência com o ano inferido, mas nenhum candidato é escondido por divergir: o catálogo às vezes traz o ano errado, e o custo de um falso positivo é o operador olhar, enquanto o de um falso negativo é publicar duplicata num tracker privado.

O Portal exibe os candidatos e exige um "publicar mesmo assim" explícito antes de liberar o botão — o mesmo padrão do `allow_duplicate` na duplicata de cluster (#11). A decisão continua sendo do operador; o endpoint só garante que ela seja informada.

**Mesmo arquivo × outra versão (#37).** Duas perguntas diferentes, com critérios diferentes:

- **Mesmo arquivo:** no add, é o mesmo info_hash (guard do #11, `409 duplicate`). No browse do catálogo, que não traz o hash (o `magnet_link` vem `null` em tracker privado), é o **tamanho exato em bytes** confirmado por uma evidência de nome: a mesma obra, um arquivo do item com o nome do torrent (ou dentro da pasta dele) ou o casamento por nome do #12. Esse é o `in_cluster: true` do browse. Item sem tamanho cai no casamento só por nome.
- **Outra versão da mesma obra** (outra resolução, outro codec, outro release): mesma chave de título (`release.TitleKey`: título normalizado sem acento nem pontuação, ano, temporada/episódio e parte). Ano ausente de um lado não impede o casamento; anos diferentes separam. É a mesma chave do painel de duplicados do Analytics (#172).

`POST /api/v1/torrents` e `POST /api/v1/trackers/{id}/seed` aceitam `allow_other_version` (JSON; no multipart, campo de formulário `"true"`). Sem ele, quando o cluster já tem outra versão da obra, a resposta é `409 other_versions`:

```json
{"error": {"code": "other_versions", "message": "the cluster already has another version of this title: «…»",
  "versions": [{"name": "…", "resolution": "1080p", "codec": "x264", "size_bytes": 13636235264, "node": "seedgrid-node-01"}],
  "adding":   {"name": "…", "resolution": "720p",  "codec": "x264", "size_bytes": 3650722201, "node": ""}}}
```

`versions` vem da maior para a menor e `adding` é o que se tentou adicionar (sem Node, que o Scheduler ainda não escolheu). O Portal mostra as duas coisas e reenvia com `allow_other_version: true` se o operador confirmar. A checagem roda **depois** do guard de hash: arquivo idêntico continua sendo `409 duplicate`. `allow_duplicate` continua aceito na API, mas o Portal não oferece mais a 2ª cópia: arquivo idêntico é "no cluster" e pronto.

No browse (`GET /api/v1/trackers/{id}/browse`), cada item traz `other_versions` (mesmo formato de `versions`, lista vazia quando não há) para o selo "Você tem em 1080p" do card.

**Prazo do browse (#41).** A rota inteira tem **25 s**, o orçamento da ADR-040 §4: a listagem no tracker e o enriquecimento (TMDB/IGDB/Open Library) que vem depois. Se o tracker não responde no prazo, a resposta é `504 tracker_timeout` com `error.params.seconds`, como no seed; outras falhas do tracker seguem `502 browse_failed`. O enriquecimento usa o que sobrou do prazo. O item que não ficou pronto a tempo vem sem pôster e sem título traduzido, e não entra no cache: o próximo browse completa. A busca de duplicata (`/dupes`) tem o mesmo prazo e, como sempre, responde `200` com `note` quando o tracker não responde.

**Semear do catálogo do tracker (#12, #35, #37).** `POST /api/v1/trackers/{id}/seed` — exige `trackers:seed`. Corpo `{"torrent_id": "…", "allow_other_version": false}`. Baixa o `.torrent` do item, usando o `download_link` guardado pelo browse ou, sem ele, a consulta a `/api/torrents/{id}`, e o adiciona ao cluster pelo Scheduler. Responde `201` com o torrent. A conversa com o tracker tem prazo de **25 s**, o mesmo orçamento da ADR-040 §4: fica abaixo dos 30 s do `WriteTimeout` e dos 60 s do proxy no `/api/`, para que o operador receba o motivo, e não o erro do proxy. Erros:
- `504 tracker_timeout`, com `error.params.seconds`: o tracker não respondeu no prazo. Não há segunda tentativa com o `api_token`, porque o token não acelera um tracker lento e a repetição só dobraria a carga sobre ele.
- `502 fetch_failed`: o tracker recusou o download, limitou a taxa (`429`) ou entregou algo que não é `.torrent`. A mensagem diz qual dos três.
- `409 duplicate`: o mesmo arquivo já está no cluster; o Portal marca o item como "no cluster".
- `409 other_versions`: o cluster já tem outra versão da obra (ver acima).
- `409 no_eligible_node` / `409 no_node_with_space`: o Scheduler não achou Node elegível ou com espaço.
- `400 tracker_without_api`: o tracker não tem `api_url` e token cadastrados.

**Preview congelado do payload (#188).** `POST /api/v1/torrents/{id}/publish-tracker/preview` — exige `torrents:publish`. Recebe **o mesmo corpo** do `publish-tracker` e devolve o payload exato que o submit enviaria, sem tocar na rede: `{"endpoint", "tracker", "fields": {…}, "torrent_name", "torrent_size", "missing": [], "token"}`.

Existe porque a **API do UNIT3D não tem dry-run**. Sem o preview, a única forma de descobrir o que vai ser postado é postar — e um campo errado vira registro errado no catálogo de um tracker privado, sem desfazer. Aqui pesa mais do que numa ferramenta de upload comum: o `.torrent` que vai no POST já está **semeando** num Node, então um post errado não é só um registro ruim, é um infohash publicado que o cluster já está servindo.

O que faz "congelado" significar alguma coisa é o `token`, e não a tela:

- `token` é a **impressão do payload** — todos os campos de texto mais o `.torrent` (nome, tamanho e conteúdo). Trocar o arquivo mantendo a ficha idêntica publicaria outro conteúdo sob a mesma conferência, então o arquivo entra na conta.
- O submit aceita um `preview_token` opcional e, quando ele vem, **verifica**: se a ficha mudou depois da conferência, responde **409 `preview_stale`** em vez de postar o que ninguém aprovou. É opcional para não quebrar cliente existente; o Portal sempre envia.
- `fields` vem de `unit3d.Fields()`, a **mesma** função que o `Upload` usa para montar o multipart. Uma segunda montagem faria o preview poder mostrar uma coisa e o POST enviar outra — exatamente a falha que o preview deveria eliminar.

`missing` lista os obrigatórios ainda vazios (`name`, `description`, `category_id`, `type_id`). O preview não recusa por causa deles: avisa, para o operador corrigir na tela em vez de tomar um 400 no submit.

**Perfil do site e descoberta de taxonomia (ADR-037, #190/#196).** O que é do **site** — hoje a taxonomia (os IDs de `category`, `type` e `resolution`, que variam de site para site: `Encode` é 3 no samaritano e 12 no Blutopia) — mora no **perfil** do tracker, separado da credencial do operador. O perfil vem **dentro** do tracker, sem chamada extra:

- `GET /api/v1/trackers` (e as respostas de `POST`/`PUT /api/v1/trackers`) — cada tracker ganha `profile`: `{"family": "unit3d", "schema_version": 1, "taxonomy": {"category": [{"id": 1, "name": "Filmes", "kind": "movie", "origin": "seed"}, …], "type": […], "resolution": […]}}`. Os três eixos estão **sempre** presentes (lista vazia, nunca `null`), ordenados por `id`. Tracker sem linha de perfil sai com o perfil vazio. A seção `description` (#192) só aparece quando o operador personalizou a composição da descrição — ver «Composição da descrição por tracker», abaixo. É daqui que o painel de publicação monta as opções — não há mais lista cravada no Portal. **Sem `announce`, passkey nem `api_token`**: o perfil é a camada 2 da ADR-037 e a credencial é a camada 3, que nunca sai da API.
- `origin` diz de onde veio o termo — `seed` (migration), `discovered` (descoberta), `manual` (o operador digitou ou renomeou) ou `imported` (veio de um arquivo de perfil, #197) — e é **decidido pelo servidor**. É estado **local**: não viaja no arquivo de troca. `kind` (`movie`, `tv`, `game`; só em `category`) é o papel da categoria: é o que o Portal usa para mostrar temporada/episódio e a busca de jogo, no lugar do número da categoria que estava cravado.
- `PUT /api/v1/trackers/{id}/profile` — exige `trackers:write`. Corpo `{"taxonomy": {"type": [{"id": 50, "name": "PC"}, …]}}`. Os eixos **enviados substituem** os atuais (mesmo vazios); os **ausentes não são tocados** — um cliente não apaga a taxonomia por esquecer um campo. Termo novo ou renomeado vira `manual`; o que não mudou mantém a origem. Responde `200` com o perfil. Auditado: «Editou o perfil do tracker «X»».
- `POST /api/v1/trackers/{id}/discover-taxonomy` — exige `trackers:write`. Corpo opcional `{"pages": N}` (default **8**, teto **20**, páginas de 50 itens). Varre o catálogo do **próprio tracker** (`GET /api/torrents/filter`, o mesmo do browse), com o token do próprio operador, e agrega os pares distintos `(id, nome)` por eixo. **Só propõe — nunca grava.** Resposta: `pages_requested`, `pages_scanned`, `items_scanned`, `stopped_reason` (`end_of_catalog` | `max_pages` | `rate_limited` | `time_budget` | `error`), `proposal` (por eixo, itens `{id, name, seen, status}` com `status` `new` | `same` | `renamed`; `renamed` traz `current_name`, `current_origin` e `protected`), `rejected`, `warnings` e `notice`. A varredura tem um orçamento de **20 s** (o servidor fecha a resposta em 30 s): se o tracker limitar a taxa (`429`) ou o tempo acabar, a resposta é **`200` com o que já foi colhido** e o `stopped_reason` diz que parou. Só falha (`502 discover_failed`) se nenhuma página chegou. `400 tracker_without_api` sem `api_url`/token. Não é auditado: não muda estado.
- `POST /api/v1/trackers/{id}/apply-taxonomy` — exige `trackers:write`. Corpo `{"items": [{"axis": "type", "id": 50, "name": "PC", "origin": "discovered"}]}` (`origin` `discovered` — default — ou `manual`, quando o operador editou o nome na proposta; `seed` é recusado). Aplica **no servidor, sobre o estado travado** (`SELECT … FOR UPDATE`), a regra que torna seguro rodar a descoberta de novo: id ausente do perfil → **adiciona**; id presente com o mesmo nome → nada; id presente com nome diferente → **renomeia, exceto se a origem for `manual`** (aí é ignorado como `protected`); o que o catálogo não mostrou fica **intocado** — ausência não é evidência. Descoberta nunca remove e nunca mexe em `kind`. Resposta: `{"profile", "applied": [{axis, id, name, action: "added"|"renamed"}], "skipped": [{axis, id, name, reason: "same"|"protected"}]}`. Auditado: «Aplicou a taxonomia descoberta ao tracker «X»: N novo(s), M renomeado(s)».

⚠️ **O limite que o contrato carrega:** a descoberta só enxerga **o que já foi postado**. Categoria, tipo ou resolução que ninguém usou não aparece, e o catálogo é varrido do mais recente para o mais antigo — o perfil descoberto **não é completo por definição**. Por isso toda resposta traz `notice` e o Portal repete o aviso; o que faltar se adiciona à mão (`PUT …/profile`). A descoberta também **não** alcança regras do site (texto de wiki) nem formato de nome (observação) — esses dependem de quem tem a conta, e circulam pelo export/import (abaixo).

**Perfil é conteúdo não confiável.** Os pares chegam do catálogo de um site de terceiro, do operador e de um arquivo importado, e todos passam pela mesma validação antes de tocar o banco: `id` inteiro em `1..1000000`; nome com 1 a 64 caracteres, UTF-8 válido, espaços colapsados, **sem caractere de controle nem de formatação invisível** (o override de direção U+202E, o zero-width…); no máximo 256 termos por eixo; eixos de um conjunto **fechado** (`category`, `type`, `resolution`); `kind` só em `category`; corpo da requisição limitado a 1 MiB; campo desconhecido no JSON é `400`. Entrada inválida responde `400 invalid_profile` com o caminho do campo (`taxonomy.type[2].name: …`). Um par do catálogo que não passa é **descartado e contado** em `rejected` — o operador vê que houve descarte em vez de uma proposta que parece completa. Nenhum campo livre vai para uma requisição ao tracker: o POST de publicação leva só os **IDs**; o nome é rótulo.

**Export e import do perfil (ADR-037, #197).** O perfil do site é o que quem tem conta no site descobre, ajusta e **compartilha**: o export gera um arquivo, o import o aplica em outro tracker — sem o segundo usuário precisar de conta em lugar nenhum. O arquivo é a camada 2 mais um envelope de identificação; **nunca** leva `api_token`, passkey, announce, `api_url` nem `source` — não por filtro, mas porque o tipo que o gera não tem campo para eles.

> **Perfis contribuídos.** Os perfis que a comunidade compartilhou ficam versionados em `profiles/<família>/<site>.json` (ver `profiles/README.md`); são arquivos gerados por este export, validados no CI pela mesma régua do import.

- `GET /api/v1/trackers/{id}/profile/export` — exige `trackers:read`. Responde o arquivo (`application/json`, `Content-Disposition: attachment; filename="seedgrid-profile-unit3d-<host>.json"`, só `[a-z0-9-]`; `Cache-Control: no-store`). Leitura: **não** é auditado.
- `POST /api/v1/trackers/{id}/profile/import/preview` — exige `trackers:write`. O **corpo é o arquivo**, byte a byte (`curl --data-binary @perfil.json`). Valida e devolve `{"plan", "notice"}`. **Nunca grava** (nem cria a linha do perfil) e **não** é auditado.
- `POST /api/v1/trackers/{id}/profile/import` — exige `trackers:write`. Mesmo corpo. Aplica e devolve `{"profile", "plan"}`, onde `plan` é o que foi **executado** (calculado sobre a linha travada, `SELECT … FOR UPDATE`; pode diferir do que a pré-visualização mostrou se o perfil mudou no intervalo). Auditado: «Importou um perfil de arquivo no tracker «Y»: N novo(s), M renomeado(s), K papel(éis) preenchido(s); P mantido(s) por serem do operador ou já confirmados pelo site dele» — nada que veio do arquivo entra na trilha.
- Os dois de import aceitam `?overwrite=<eixo>:<id>` (repetível, ex.: `?overwrite=category:2`) — e `?overwrite=description` (composição da descrição) ou `?overwrite=image_host` (host de imagem), cada seção como uma unidade —: o pedido, termo a termo, para sobrescrever um nome **protegido** — o que o operador ajustou à mão (`manual`) ou o que o site dele confirmou na descoberta (`discovered`). Sem ele, um termo `manual` ou `discovered` **nunca** é sobrescrito. Valor malformado é `400`; pedido que não corresponde a nenhum termo ou seção protegida que o arquivo traga vira aviso em `plan.warnings`, nunca efeito.

*Pré-visualizar e aplicar são duas rotas, de propósito:* a rota fácil de chamar sem querer nunca grava, e a trilha de auditoria (que decide por método e caminho) não registra uma leitura como se fosse uma importação.

O arquivo (exemplo real, gerado pelo código a partir de um perfil de três categorias, dois tipos e duas resoluções — partindo de termos com `origin` `seed`, `manual`, `discovered` e `imported`, **nenhuma** das quais sai no arquivo):

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

- `format` — sempre `seedgrid.tracker-profile`. `schema_version` — a versão do **formato** (hoje `1`; sobe quando um campo muda de forma ou entra uma seção **obrigatória** — uma seção opcional e aditiva, que o export omite quando está no default, como a `description` da #192 e a `image_host` da #193, não sobe: ver «Composição da descrição por tracker» e «Host de imagem por tracker»). `family` — hoje só `unit3d`; tem de ser a do tracker de destino (`409 incompatible_family` se não for). `site` — só identificação: `name` (o rótulo que o operador deu ao tracker — **confira antes de compartilhar**; se não passa nas regras de nome ou **parece conter uma credencial** — barra, `?`, `=`, UUID, 20 ou mais letras/dígitos seguidos —, sai o host público, ou `tracker`: o nome nunca é truncado) e `host` (o nome DNS do host do announce, sem porta nem caminho; omitido se não for um nome de host válido). `site` **não é aplicado a nada**: serve para o operador saber o que importa e para a pré-visualização avisar quando o arquivo é de **outro site** (os ids são do site; um id errado publica na categoria errada). `taxonomy` — por eixo (`category`, `type`, `resolution`; eixo ausente não é erro), termos `{id, name, kind?}`. `description` — opcional, só quando o operador personalizou a composição da descrição do upload (#192): `{media_blocks, game_blocks, media_info_tag}`, validada como em `PUT …/profile/description`. `image_host` — opcional, só quando o tracker tem host de imagem próprio (#193): `{name, upload_url, file_field, auth_header, auth_prefix, url_path, thumbnail_path?}`, validada como em `PUT …/profile/image-host`. **O arquivo não leva `origin`** (é local a cada instalação) e o import **recusa** um arquivo que o traga; o import atribui a origem `imported` a tudo o que escreve.
- Plano (`plan`): `site`, `family`, `schema_version`, `items` (por eixo, `{axis, id, name, kind?, status, current_name?, current_origin?, sets_kind?, overwrites_protected?, warnings?}` com `status` `new` | `same` | `renamed` | `protected`), `counts` (`new`, `renamed`, `same`, `protected`, `kind_set`, `local_only`), `description` (só quando o arquivo traz a seção: `{status: new | same | protected | replaced, config, current?, overwrites_protected?}`), `image_host` (só quando o arquivo traz a seção: `{status: new | same | protected | replaced, config, current?, overwrites_protected?}`) e `warnings`. A pré-visualização traz também `notice`: o aviso de que o conteúdo veio de um arquivo e não foi conferido contra o site do operador.
- **Regra de convivência** (a do `apply-taxonomy`, estendida e mais estrita onde a descoberta já é opt-in): id ausente → **adiciona** (`imported`); mesmo nome → nada; nome diferente → **renomeia só o que é `seed` ou `imported`**; um termo `manual` ou `discovered` (o nome que o site do operador confirmou) é `protected` e fica como está — assim como uma origem que o código não conhece (falha fechada, no import e na descoberta); termo do perfil que o arquivo não traz → **intocado** — o import **nunca remove**. O `kind` do arquivo só **preenche** um papel vazio, nunca troca um que existe, e um termo `protected` não tem nem o papel tocado. Importar duas vezes o mesmo arquivo é incremental: a segunda não muda nada.
- **Colisão (um termo novo não pode ofuscar o certo).** O painel de publicação casa por nome e pré-preenche a primeira categoria de cada papel. Por isso o arquivo com dois nomes que só diferem em caixa ou acento **no mesmo eixo** é recusado (`400`, `taxonomy.category[1].name: nome repetido no eixo: igual ao do id 1…`), e a pré-visualização avisa, em `items[].warnings`, o termo novo ou renomeado cujo nome colide com o de outro id do eixo (sobre o estado em que o eixo ficaria) e o que passaria a ser a **primeira** categoria de um papel que já tinha dono; `plan.warnings` traz o resumo. Pelo mesmo motivo, **`GET …/export` responde `409 profile_not_exportable`** (com o motivo) para um perfil local com nomes repetidos: o export só entrega um arquivo que o próprio import aceitaria.
- **Validação (o arquivo é conteúdo não confiável).** Corpo até **512 KiB** (`413 payload_too_large`); UTF-8 válido; JSON de **um** valor só; chaves só `[a-z0-9_]` e **sem repetição** (o decoder padrão casa chave ignorando maiúsculas e dobras Unicode e fica com a última de duas repetidas — o arquivo revisado seria diferente do importado); no máximo 8 níveis de aninhamento; **campo desconhecido é erro**; `format` conferido; `schema_version` inteiro de `1` até a versão atual (`400 unsupported_schema_version`, dizendo a que este SeedGrid entende, quando o arquivo é de uma versão mais nova); `family` conhecida; `site.name` com as regras de nome (1 a 64 caracteres, sem controle, sem formatação invisível e sem as runas que parecem espaço sem serem espaço — U+3164, U+115F, U+1160, U+FFA0, U+2800…); `site.host` um nome DNS; e, nos termos, as mesmas regras de `PUT …/profile`. Uma **seção do perfil que este SeedGrid exporta mas ainda não sabe importar** é recusada se vier preenchida (fail closed). Erro: `400 invalid_profile` com o caminho do campo — nunca o valor recebido, que pode ter megabytes. Tracker de outro operador ou id malformado: `404`.
**Composição da descrição por tracker (ADR-037, #192).** A descrição BBCode de um upload era montada no Portal (`bbcode.ts`): fora do alcance de qualquer outro cliente da API, sem onde dois sites com markup diferente divergirem e sem teste em Go — sendo o campo que o site publica na íntegra. Agora quem a compõe é o Control Plane (`internal/description`), pela **seção `description` do perfil do tracker**: ela é do site (camada 2 da ADR-037), não do operador, e por isso viaja no export/import. O Portal só pede o texto e o entrega ao campo, que o operador **continua editando** antes de publicar — a conferência humana da ADR-020 não muda.

- `POST /api/v1/publish/description` — exige `publish:write` (a mesma do `publish/infer`). Corpo `{"tracker_id": "…", "kind": "media" | "game", "media": {"poster", "overview", "keywords", "screenshots", "mediainfo"}, "game": {"cover_url", "name", "year", "summary", "genres", "platforms"}}` — só o objeto do `kind` pedido é lido (`screenshots` é o bloco de BBCode que o `…/screenshots/render` devolveu). Responde `200 {"description": "<BBCode>"}`, composto com a seção `description` do perfil do tracker ou, sem ela, com o default. Erros: `404 tracker_not_found` para tracker inexistente, removido, de outro operador ou com id malformado; `400 invalid_request` para `kind` que não seja `media` nem `game` (a conferência vem **antes** da busca do tracker), `tracker_id` ausente, JSON malformado e campo desconhecido ou de tipo errado (inclusive dentro de `media`/`game`); `413 payload_too_large` acima de 4 MiB (o MediaInfo de um release longo passa de 100 KiB). **Só compõe texto a partir do corpo**: não lê o torrent, não consulta TMDB nem IGDB e não muda estado — por isso fica **fora da trilha de auditoria** (ADR-025), como o `publish/infer`. O `POST /api/v1/publish/infer` aceita `tracker_id` opcional (#74): a sinopse do TMDB vem no idioma do conteúdo daquele tracker; sem ele, em inglês.
- `PUT /api/v1/trackers/{id}/profile/description` — exige `trackers:write`. Corpo é a config: `{"media_blocks": ["poster", "synopsis", "tags", "screenshots", "mediainfo"], "game_blocks": ["cover", "title", "about", "genres", "platforms"], "media_info_tag": "mediainfo"}` (este é o default). Lista **ausente** = o default; lista **vazia** (`[]`) = nenhum bloco daquele tipo; tag ausente ou vazia = `mediainfo`. Grava a seção **explícita** (as três chaves sempre preenchidas) e responde `200` com o perfil. Não toca a taxonomia. Auditado: «Editou a composição da descrição do tracker «X»». `language` opcional (`en` | `pt-BR`; ausente = `en`, e `en` não é gravado): o idioma do conteúdo do site (#74) — títulos da descrição e a sinopse do TMDB; `400 invalid_profile` para outro valor.
- `DELETE /api/v1/trackers/{id}/profile/description` — exige `trackers:write`. Apaga a seção: o tracker volta ao default. Idempotente (apagar o que não existe também é `200`). Auditado: «Restaurou ao padrão a composição da descrição do tracker «X»».
- `GET /api/v1/trackers` (e as respostas de `POST`/`PUT /api/v1/trackers`) — o `profile` ganha `description` **só quando o operador personalizou** (a chave nem existe no perfil padrão), e o tracker ganha, ao lado de `profile`, `description_default`: a composição que vale sem a seção — que, por listar **todos** os blocos na ordem original, é também o **catálogo** que um editor oferece para ligar, desligar e reordenar. Vem do servidor para nenhum cliente duplicar o default.

Os blocos são um conjunto **fechado** (um bloco novo é mudança de código, não um id livre num JSON); a composição junta, **na ordem da lista**, os blocos que têm conteúdo, separados por uma linha em branco, e **omite** o que está vazio:

| id | tipo | o que sai |
|---|---|---|
| `poster` | mídia | `[center][img]URL[/img][/center]` — a URL entra como veio, sem trim |
| `synopsis` | mídia | `[b]Sinopse[/b]` + o texto (com trim) |
| `tags` | mídia | `[b]Tags[/b]` + as palavras-chave (com trim) |
| `screenshots` | mídia | o bloco pronto do CP, com trim — sem rótulo próprio |
| `mediainfo` | mídia | `[b]MediaInfo[/b]` + o dump dentro da **tag** da config (`mediainfo`, `code`, `pre` ou `spoiler`) |
| `cover` | jogo | `[center][img]URL[/img][/center]` |
| `title` | jogo | `[b]Nome (ano)[/b]` — **sai sempre** que o bloco estiver na lista, mesmo sem nome; o ano só aparece se for diferente de zero |
| `about` | jogo | `[b]Sobre[/b]` + o resumo (com trim) |
| `genres` | jogo | `[b]Gêneros[/b]` + os itens separados por `, ` (só o tamanho da lista decide: `[""]` ainda emite o bloco) |
| `platforms` | jogo | `[b]Plataformas[/b]` + os itens separados por `, ` |

**Paridade com o que o Portal montava.** Com a config **default** a saída é **byte a byte** a do TypeScript antigo — rótulos, ordem, separador e omissão de vazios. A paridade não é afirmada, é medida: as strings esperadas do teste foram calculadas **executando** o `buildBBCode`/`buildGameBBCode` originais (`git show 0525f0f:portal/src/pages/publish/bbcode.ts`) sobre 44 entradas, e coladas no teste. O que o `trim()` do JS remove **não é** o que o `strings.TrimSpace` do Go remove: o JS tira o BOM (U+FEFF) e não tira o NEL (U+0085), o Go faz o contrário — por isso o CP tem o seu `jsTrim`, e há linhas do golden só para isso (BOM, espaço sem quebra, U+2028/2029, os Zs, U+0085, U+180E, U+200B).

**A config é conteúdo não confiável** (ADR-037) — pode chegar num arquivo importado — e é validada em **todo ponto de escrita** (o `PUT`, o import e o export): `media_blocks` e `game_blocks` só aceitam ids do conjunto fechado do **seu** tipo (um bloco de jogo numa lista de mídia é recusado), sem repetição e com no máximo o tamanho do conjunto (uma lista de milhares de itens nem chega a ser percorrida); `media_info_tag` só `mediainfo`, `code`, `pre` ou `spoiler` — a tag é **interpolada em BBCode**, então nada fora dessa lista passa (`mediainfo][img=…` é recusado, e a composição ainda cai no default se uma tag inválida aparecer num banco editado à mão). Recusa: `400 invalid_profile` com o caminho do campo (`description.media_blocks[1]: bloco "x" desconhecido (use poster, …)`); a mensagem nunca devolve o valor recebido por inteiro.

**Onde mora.** Na **mesma** coluna JSONB do perfil (`tracker_profiles.taxonomy`), como a chave `description` **ao lado dos eixos** — sem migration, sem coluna nova e sem reescrever nenhuma linha: o documento que já existe é, byte a byte, um documento novo sem a seção. `schema_version` **não muda** (continua `1`): a seção é opcional e aditiva e o export a omite enquanto o perfil não a tem, então um perfil sem ela continua sendo exatamente o arquivo da v1. O custo, aceito: um SeedGrid **anterior à #192** recusa um arquivo que traga `description` com `campo desconhecido` — e **não** com `unsupported_schema_version` —, e um Control Plane anterior à #192 não lê o perfil de um tracker cuja composição foi personalizada (restaure o padrão — `DELETE` acima — antes de voltar de versão).

**No export e no import.** O arquivo ganha a chave `description` depois de `taxonomy`, **só quando o operador personalizou** — o Document embute o Profile, então a seção é exportada sem mexer no export, e o export passa pela mesma validação do import (uma seção inválida gravada à mão no banco faz o export falhar com `409 profile_not_exportable` e o motivo). No import a seção é **uma unidade**, sem origem por termo, e a regra é a do `manual`, aplicada a ela inteira: perfil **sem** composição própria (ou com o default) → **aplica** (`new`); igual → nada (`same`); **diferente → protegida** (`protected`): fica como está e `plan.warnings` diz que manteve, a menos que o operador peça `?overwrite=description` — aí **substitui** (`replaced`). O arquivo que não traz a seção **não toca** a do perfil; um arquivo **só** com a seção é um import válido (não sai o aviso de «nenhum termo»). O plano ganha `description` — `{status: new | same | protected | replaced, config, current?, overwrites_protected?}`, presente só quando o arquivo traz a seção. A taxonomia e a seção saem do mesmo plano, calculado sobre a linha travada, e são gravadas **juntas**. Auditoria: o aplicar acrescenta «; composição da descrição: aplicada | substituída a pedido | mantida a do operador | já era igual» **só quando o arquivo trouxe a seção**.

**Host de imagem por tracker (ADR-040, #193).** A política de host de imagem dos screenshots — hoje o imgbb global, chave única no catálogo — passa a ser **dado do perfil do tracker** (camada 2 da ADR-037): a seção opcional `image_host` descreve o host próprio do site, e a cadeia de upload (host do tracker → imgbb) usa o que estiver configurado. A seção é de forma **fechada** (sete campos, ADR-040) e viaja no export/import como a `description`. A **chave do operador** para o host é camada 3 (tabela `trackers`, #193 / ADR-040) e nunca sai na API nem viaja no perfil.

- `PUT /api/v1/trackers/{id}/profile/image-host` — exige `trackers:write`. Corpo é a Spec do host de imagem (#193, ADR-040): `{"name", "upload_url", "file_field", "auth_header", "auth_prefix", "url_path", "thumbnail_path"?}`. Grava a seção `image_host` no perfil e responde `200` com o perfil. Não toca a taxonomia nem a `description`. Auditado: «Editou o host de imagem do tracker «X»». A Spec é validada em todo ponto de escrita (o `PUT`, o import e o export): `name` com as regras de nome (e gravado normalizado, como o nome de termo: espaços colapsados); `upload_url` absoluta, `https`, host DNS (sem IP literal IPv4 ou IPv6, sem `localhost` nem `*.localhost`, sem host cujo último rótulo seja só dígitos ou `0x…` — `127.1`, `0x7f.1` —, sem porta, sem userinfo, sem fragmento), até 2048 caracteres; `file_field` `[A-Za-z0-9_.-]{1,64}`; `auth_header` token de header `[A-Za-z0-9-]{1,64}`, fora de `Host`, `Content-Type`, `Content-Length`, `Transfer-Encoding`, `Connection`, `Cookie`, `Upgrade`, `TE`, `Trailer` e `Proxy-*` (sem diferenciar maiúsculas); `auth_prefix` 0 a 32 caracteres ASCII imprimíveis; `url_path` 1 a 5 segmentos `[A-Za-z0-9_]{1,64}` separados por ponto; `thumbnail_path` opcional, mesmas regras. Recusa: `400 invalid_profile` com o caminho do campo (`image_host.upload_url: scheme "http" (só https)`); a mensagem nunca devolve o valor recebido por inteiro.
- `DELETE /api/v1/trackers/{id}/profile/image-host` — exige `trackers:write`. Apaga a seção: o tracker passa a usar apenas o imgbb global na cadeia de screenshots. Idempotente (apagar o que não existe também é `200`). Auditado: «Removeu o host de imagem do tracker «X»».
- `GET /api/v1/trackers` (e as respostas de `POST`/`PUT /api/v1/trackers`) — o `profile` ganha `image_host` **só quando o tracker tem host próprio** (a chave nem existe no perfil padrão).
**Chave do operador presa ao host (#193 / ADR-040).** A chave que o operador cadastra para o host de imagem do tracker é **camada 3** (credencial do operador, tabela `trackers`), nunca camada 2 (perfil do site). Ela fica **presa ao host**: ao gravar, o CP guarda junto o nome DNS do `upload_url` vigente (`Spec.Host()`), e a cadeia de screenshots confere se o host do perfil ainda bate antes de usar a chave — uma chave cadastrada para outro host é pulada com motivo. A chave **nunca** sai na API: a listagem expõe só o booleano `has_image_host_key` e o host (`image_host_key_host`).

- `PUT /api/v1/trackers/{id}/image-host-key` — exige `trackers:write`. Corpo `{"key": "..."}`; a chave é gravada **aparada** (sem espaços nem quebras nas pontas). Chave vazia (ou só espaços) → `400 invalid_request`; perfil sem `image_host` → `409 profile_without_image_host`; tracker de outro operador ou id malformado → `404 tracker_not_found`. Grava a chave **junto com o host do `upload_url` vigente** (`Spec.Host()`). Responde `{"status": "ok"}`. Auditado: «Cadastrou a chave do host de imagem «host» no tracker «X»» — o texto leva o host, **nunca** a chave.
- `DELETE /api/v1/trackers/{id}/image-host-key` — exige `trackers:write`. Apaga a chave e o host presos ao tracker. Idempotente sobre a ausência da chave (apagar o que já está apagado também é `200`); o tracker precisa existir e ser do operador (`404` caso contrário). Responde `{"status": "ok"}`. Auditado: «Removeu a chave do host de imagem do tracker «X»».
- `GET /api/v1/trackers` — cada tracker ganha `has_image_host_key` (booleano) e `image_host_key_host` (o host DNS para o qual a chave foi cadastrada, vazio se não houver chave).

**Forma fechada e validação.** A Spec descreve um host de imagem do tipo "upload multipart": sempre `POST multipart/form-data` com uma parte de arquivo (o frame JPEG, `image/jpeg`); a chave vai **só** no header `auth_header`, precedida de `auth_prefix`; a resposta é JSON e a URL da imagem sai por **caminho com pontos** (`data.link`, `image.url`; segmento numérico indexa lista). Nenhum campo carrega a chave, e nenhum nome de campo a sugere (`auth_header` diz **onde** a chave vai, não qual é). A validação é do **nome** (DNS), não do endereço: um nome que resolve para IP interno passa — a defesa real é a chave, que o operador cadastra **para aquele nome** (#193 / ADR-040). Host fora dessa forma (chave na query ou no corpo, resposta que não é JSON, upload em duas etapas) é **família nova**: PR com código, como a ADR-037 já prevê.

**Onde mora.** Na **mesma** coluna JSONB do perfil (`tracker_profiles.taxonomy`), como a chave `image_host` **ao lado dos eixos** — sem migration, sem coluna nova e sem reescrever nenhuma linha: o documento que já existe é, byte a byte, um documento novo sem a seção. `schema_version` **não muda** (continua `1`): a seção é opcional e aditiva e o export a omite enquanto o perfil não a tem, então um perfil sem ela continua sendo exatamente o arquivo da v1. O custo, aceito: um SeedGrid **anterior à #193** recusa um arquivo que traga `image_host` com `campo desconhecido` — e **não** com `unsupported_schema_version`.

**No export e no import.** O arquivo ganha a chave `image_host` depois de `description` (ou de `taxonomy`, sem `description`), **só quando o tracker tem host próprio** — o Document embute o Profile, então a seção é exportada sem mexer no export, e o export passa pela mesma validação do import (uma seção inválida gravada à mão no banco faz o export falhar com `409 profile_not_exportable` e o motivo). No import a seção é **uma unidade**, sem origem por termo, e a regra é a do `manual`, aplicada a ela inteira: perfil **sem** host próprio → **aplica** (`new`); igual → nada (`same`); **diferente → protegida** (`protected`): fica como está e `plan.warnings` diz que manteve, a menos que o operador peça `?overwrite=image_host` — aí **substitui** (`replaced`). O arquivo que não traz a seção **não toca** a do perfil. O plano ganha `image_host` — `{status: new | same | protected | replaced, config, current?, overwrites_protected?}`, presente só quando o arquivo traz a seção. A taxonomia e as seções saem do mesmo plano, calculado sobre a linha travada, e são gravadas **juntas**. Auditoria: o aplicar acrescenta «; host de imagem: aplicado | substituído a pedido | mantido o do operador | já era igual» **só quando o arquivo trouxe a seção**. A chave do operador para o host (camada 3) **nunca** entra no arquivo: ela fica em `trackers` (#193 / ADR-040) e o export só lê `tracker_profiles`.

# **8. API dos Nodes**
Responsável pela comunicação entre Agent e Control Plane.
Operações previstas:
Registro.
Heartbeat.
Inventário.
Eventos.
Atualização de estado.
Recebimento de comandos.
O Agent nunca utilizará endpoints destinados ao Portal.

## 8.1 Heartbeat e configuração de comportamento (ADR-039, #144)
`POST /api/v1/agent/heartbeat` é também o canal da configuração: a **resposta** entrega os valores efetivos do Node, e o **corpo** do heartbeat seguinte reporta o que o Agent aplicou.

Resposta (`200`):
```json
{
  "status": "ok",
  "agent_config": {
    "version": "a1b2c3d4e5f6",
    "values": {
      "engine.max_active_downloads": "3",
      "agent.heartbeat_interval": "10s",
      "...": "..."
    },
    "report_local": false
  }
}
```
- `values`: **todas** as chaves `PerNode` do catálogo, com o valor **efetivo daquele Node** (override > cluster), serializadas como no catálogo (`"10s"`, `"true"`, `"3"`). O Control Plane manda o estado desejado **inteiro** a cada heartbeat: perder um não custa nada, o próximo corrige.
- `version`: hash (12 hex) dos valores. O Agent o devolve em `config_version` **depois de aplicar tudo com sucesso**.
- `report_local: true`: a **semeadura** dos overrides ainda não aconteceu para este Node (ADR-039 §7). O Agent **não aplica nada**, e relata o env explícito dele em `local_config` no próximo heartbeat.
- O bloco é **omitido** se o Control Plane não conseguiu resolver (o heartbeat nunca falha por causa da configuração): o Agent mantém o que já aplicou.

Campos aditivos do **corpo** do heartbeat:
- `config_version` (string): a `version` que o Agent aplicou com sucesso.
- `local_config` (objeto chave→valor): o env **explícito** de comportamento do Node — só o que o operador definiu (não os defaults do código), em forma canônica. `{}` é um relato válido (Node novo, só identidade no `.env`) e **precisa** ir: é ele que marca a semeadura como feita.

> ⚠️ **Compatibilidade — o corpo do heartbeat é decodificado com `DisallowUnknownFields`.** Um campo novo mandado por um Agent novo a um Control Plane **antigo** viraria `400` e o Node ficaria `OFFLINE`. Por isso o Agent só envia `config_version`/`local_config` **depois de a última resposta bem-sucedida ter trazido `agent_config`** (a prova de que o CP os entende), e qualquer falha de heartbeat zera essa prova. Consequências: **Agent velho + CP novo** ignora `agent_config` e segue no env; **Agent novo + CP velho** não recebe `agent_config`, segue no env e nunca manda campo novo; um **rollback do CP** com Agents já em dia custa **um** heartbeat (o único que ainda leva `config_version`), não o Node (o limite de offline é de 90 s). Ordem de deploy (ADR-027): Control Plane primeiro, Agents um a um.
# **9. API Administrativa**
Responsável pelas operações realizadas pelo Portal.
Exemplos
Gerenciar Nodes.
Gerenciar Torrents.
Gerenciar usuários.
Consultar eventos.
Consultar alertas.
Gerenciar configurações.
Executar operações administrativas.
# **10. API Pública**
A arquitetura deverá permitir disponibilização futura de uma API pública.
Na versão 1.0 ela permanecerá desabilitada.
# **11. Recursos do Cluster**
Operações disponíveis:
Consultar estado.
Consultar capacidade.
Consultar estatísticas.
Consultar versão.
Consultar configuração.
# **12. Recursos dos Nodes**
Operações previstas:
Listar.
Consultar.
Registrar.
Atualizar.
Alterar estado.
Consultar inventário.
Consultar capacidades.
Consultar Labels.
Consultar Tags.
Consultar histórico.
# **13. Recursos dos Torrents**
Operações previstas:
Adicionar.
Consultar.
Pesquisar.
Pausar.
Retomar.
Remover.
Recheck.
Alterar categoria.
Alterar Tags.
Consultar arquivos.
Consultar histórico.
Consultar estatísticas.
## 13.1 Analytics do acervo (#160)

`GET /api/v1/analytics` — exige `torrents:read`. Responde, numa chamada, as perguntas
de acervo que o Portal não sabia responder: o que o tracker não reconhece mais, o que
está duplicado, o que ocupa disco sem render e de onde tirar espaço em cada Node.

**Uma rota só, e não quatro.** Os painéis se sobrepõem (duplicado que também está
parado, parado que também está morto no tracker) e quatro rotas varreriam a mesma
tabela quatro vezes para montar UMA tela. O Control Plane faz uma leitura do acervo e
deriva todos os painéis em memória.

**Sem parâmetros.** Não há `?limit`: o relatório não tem lista de exemplo para cortar.
Os contadores são sempre do acervo inteiro — resumo truncado mentiria sobre o tamanho
do problema — e `flagged` traz **todos** os torrents sinalizados, sem truncar, para que
todo contador da tela tenha lista correspondente (#169). Antes a resposta trazia
contadores completos e listas cortadas em 20, e o operador via "157 com swarm morto"
sem nenhum jeito de alcançar os 157.

Corpo da resposta:

| Campo | Tipo | Conteúdo |
|---|---|---|
| `total` / `total_bytes` | `int` / `int64` | acervo inteiro |
| `tracker_unknown` | `int` | ainda sem veredito do tracker (nulo ≠ morto) |
| `duplicates` | `DuplicateSet[]` | títulos com mais de uma cópia, do que mais libera para o que menos |
| `idle` | `IdleSummary` | `never_uploaded{,_bytes}` e `dead_swarm{,_bytes}` |
| `nodes` | `NodeUsage[]` | por Node: `torrents`, `bytes`, `idle_bytes`, `reclaim_bytes` |
| `flagged` | `Torrent[]` | **todos** os sinalizados, com as flags que os classificam |

Cada `Torrent` de `flagged` carrega as flags `never_uploaded`, `dead_swarm`,
`tracker_dead` e `duplicate_extra` (mais `duplicate_of`, o rótulo do grupo). São elas
que ligam o contador à lista.

`DuplicateSet` agrupa por **título, ano, temporada/episódio e parte**. A resolução fica
FORA da chave de propósito — é justamente o que distingue as cópias dentro de um grupo.
A **parte** entra porque `release.Parse` corta o título alternativo depois de " - " (a
regra do tracker, ADR-020) e colapsava "Fase 1" e "Fase 2" no mesmo título, marcando uma
obra como cópia excedente da outra (#172). `identical` marca o único caso de desperdício
puro — mesmo release, mesmo tamanho, em Nodes diferentes; resolução ou release diferente
o tracker aceita como dois torrents e ambos rendem ratio.

**Não há ranking de ratio nesta rota.** O `top_ratio` foi removido: era a única lista de
exemplo do relatório, e o ranking passou a ser calculado no Portal sobre a lista completa
de `GET /api/v1/torrents` (que sem `?limit` devolve o acervo inteiro), ao lado dos demais
rankings do Dashboard (#174).

Rota **somente leitura**: nada aqui remove nem enfileira comando. A remoção continua
sendo o `DELETE` de torrent, pelo mesmo caminho auditado (ADR-025).

## 13.2 Torrent em migração (#87)

`POST /api/v1/torrents/{id}/pause`, `/resume`, `/recheck` e `DELETE /api/v1/torrents/{id}` respondem
**`409 migration_in_flight`** enquanto o torrent participa de uma migração `PENDING` (transferindo ou com o
recheck do destino em verificação) — seja o torrent migrado, na origem, seja a **cópia do mesmo hash** que o
Node destino adotou. A recusa não deixa rastro: nada é enfileirado nem soft-deletado. Tente de novo quando a
migração terminar (`COMPLETED` ou `FAILED`, ver `GET /api/v1/migrations`). Migração ainda `QUEUED` não
bloqueia nada, e outros torrents dos mesmos Nodes seguem livres.

A mensagem do 409 diz que dá para **cancelar a migração** e aponta qual (o `id` na própria mensagem — vale
também quando a ação foi sobre a cópia do destino, cuja migração o operador não teria como descobrir).

`POST /api/v1/migrations/{id}/cancel` — exige `torrents:migrate` (Administrator e Publisher); auditado
(ADR-025). Encerra a migração `QUEUED` ou `PENDING` pelo **mesmo caminho seguro do watchdog**: a migração vira
`FAILED` com `error = "cancelada pelo operador"` (e `cancelled: true` na resposta), a origem é retomada e a cópia
do destino é descartada. Responde `200` com a migração. Não há estado novo no enum.

- **Durante a cópia** (o Agent destino está no `migrate_pull`): o comando não é interrompido; a migração vira
  `FAILED` na hora e a limpeza (`remove_torrent` com dados) é enfileirada atrás do pull — a cópia termina e é
  descartada em seguida. Comandos que o Agent ainda não reivindicou são baixados e nunca rodam.
- **`404 migration_not_found`** — migração inexistente (ou id malformado).
- **`409 migration_finished`** — a migração já terminou (concluída, ou falhou sozinha): não há o que cancelar.
- **`409 migration_already_cancelled`** — já foi cancelada. **Repetir é seguro:** nada é reexecutado.
- Concluir e cancelar ao mesmo tempo nunca vencem os dois: quem perde não tem efeito (ver adendo da ADR-018).

`POST /api/v1/torrents/{id}/migrate` também recusa (`409 already_migrating`) migrar a cópia de volta enquanto
a ida está em voo.

Por quê: o Agent deixou de segurar a fila de comandos do Node enquanto espera o recheck, então uma ação sobre
a cópia seria executada **no meio** da verificação (pausar a derruba; remover apaga o que o Control Plane está
prestes a dar por válido). Ver o adendo (#87) da ADR-018.

# **14. Recursos de Usuários**
Operações previstas:
Criar.
Editar.
Desabilitar.
Alterar senha.
Consultar permissões.
Consultar sessões.

## 14.1 Autorização por papéis (ADR-035)

Desde a ADR-035, todo recurso da API administrativa exige autenticação de operador
**e** uma permissão específica no formato `resource:action`. O Control Plane resolve
as permissões do operador a partir dos papéis associados (`user_roles` →
`role_permissions` → `permissions`).

Papéis de sistema:

| Papel | Acesso |
|---|---|
| `Administrator` | Total (wildcard) |
| `Publisher` | Gerencia conteúdo: torrents, trackers, publicação |
| `Downloader` | Lê e baixa conteúdo |
| `Viewer` | Somente leitura |

Endpoints de gestão de usuários (exigem `users:read`/`users:write`, admin):

- `GET /api/v1/users` — lista operadores com papéis
- `GET /api/v1/users/{id}` — operador com papéis
- `POST /api/v1/users` — cria operador (username, senha, papéis)
- `PUT /api/v1/users/{id}` — atualiza nome/email/status/papéis
- `DELETE /api/v1/users/{id}` — desabilita (soft delete) e invalida sessões
- `POST /api/v1/users/{id}/reset-password` — redefine a senha
- `GET /api/v1/auth/me` — perfil do operador autenticado (papéis/permissões)
- `POST /api/v1/auth/password` — troca a própria senha (qualquer autenticado)

Proteções:
- Não é possível desabilitar ou remover o papel de administrador do **último**
  administrador ativo (`409 last_admin`) — a contagem ignora administradores
  soft-deletados e usuários não-ACTIVE.
- Usuário desabilitado não autentica; sessões existentes são invalidadas
  (mudança de status para não-ACTIVE revoga o acesso imediatamente, mesmo com
  token vivo — `Authenticate` filtra `status = 'ACTIVE'`).
- Permissão negada responde `403 forbidden` e **entra na trilha de auditoria**
  (ADR-025): o `RequirePermission` roda por dentro do `AuditMutations`.
- `POST /torrents/create` (criar .torrent de conteúdo local, ADR-019) exige
  permissão própria `torrents:build` — distinta de `torrents:create`
  (adicionar magnet/arquivo para download).
# **15. Recursos de Monitoramento**
Consultar:
Health Score.
Heartbeats.
Estado dos Nodes.
Estado do Cluster.
Resumo operacional.
# **16. Recursos de Configuração**
Consultar configurações.
Alterar configurações.
Consultar Feature Flags.
Consultar variáveis do sistema.
# **17. Recursos de Eventos**
Consultar eventos.
Pesquisar eventos.
Filtrar por Node.
Filtrar por Torrent.
Filtrar por período.
Filtrar por severidade.
# **18. Recursos de Alertas**
Consultar alertas.
Reconhecer alerta.
Resolver alerta.
Consultar histórico.
# **19. Objetivos**
A API deverá permitir que qualquer cliente consiga administrar completamente o Cluster.
Nenhuma funcionalidade do Portal deverá depender de mecanismos internos do Control Plane.
Toda funcionalidade deverá estar disponível através da API.
# **20. Princípio Fundamental**
A API representa a fronteira oficial do SeedGrid.
Toda integração com a plataforma deverá ocorrer através dela.
Ela define os contratos da arquitetura e deverá permanecer estável ao longo da evolução do sistema.
# **21. Padrões Gerais**
Todas as operações deverão seguir convenções consistentes.
A API deverá utilizar exclusivamente HTTPS.
Os dados deverão ser serializados em JSON.
Todos os horários deverão utilizar UTC.
Todas as respostas deverão possuir estrutura uniforme.
# **22. Estrutura de Recursos**
Todos os recursos deverão seguir a mesma organização lógica.
Exemplo:
/api/v1/
├── auth/
├── cluster/
├── nodes/
├── torrents/
├── trackers/
├── users/
├── events/
├── alerts/
├── monitoring/
├── scheduler/
├── settings/
└── system/
Cada domínio será responsável apenas pelo seu próprio conjunto de operações.
# **23. Métodos**
A API deverá seguir convenções REST.
GET
Consultar informações.
POST
Criar recursos.
Executar operações.
PUT
Substituir completamente um recurso.
PATCH
Atualizar parcialmente um recurso.
DELETE
Solicitar remoção lógica de um recurso.
# **24. Paginação**
Operações que retornem coleções deverão suportar paginação.
Exemplos:
Nodes.
Torrents.
Eventos.
Alertas.
Usuários.
A paginação deverá ser consistente entre todos os recursos.
# **25. Pesquisa**
Sempre que aplicável, a API deverá permitir:
Pesquisa textual.
Ordenação.
Filtros.
Paginação.
Seleção de campos.
Esses mecanismos deverão seguir convenções comuns em todos os endpoints.
# **26. Operações Assíncronas**
Algumas operações poderão demandar tempo elevado.
Exemplos:
Recheck.
Migração de torrent.
Atualização de inventário.
Backup.
Nesses casos, a API deverá retornar imediatamente a confirmação da solicitação.
O andamento da operação será consultado posteriormente através dos recursos apropriados.
# **27. Erros**
Toda resposta de erro deverá ser consistente.
As informações mínimas incluem:
Código.
Mensagem.
Descrição.
Timestamp.
Identificador da operação.
Quando aplicável, sugestões para resolução poderão ser incluídas.

**O código é o contrato; a mensagem é reserva (ADR-041).** Na resposta `{"error": {"code", "message", "timestamp"}}`, o `code` é estável e é o que um cliente deve interpretar. O `message` é uma frase legível no idioma do cluster — serve a quem lê a resposta crua e ao cliente que não conhece aquele código —, e pode mudar de redação. O Portal traduz os códigos que conhece para o idioma do operador e usa o `message` como reserva e como detalhe. Quando a frase depende de um número ou de um nome, ele vem em `error.params`, não embutido no texto.
# **28. Idempotência**
Operações que possam ser repetidas sem efeitos colaterais deverão ser idempotentes.
Exemplos:
Pausar um torrent já pausado.
Colocar um Node em DRAIN quando ele já estiver em DRAIN.
Essas operações deverão retornar sucesso sem gerar alterações adicionais.
# **29. Recursos dos Agents**
A comunicação entre Agent e Control Plane utilizará endpoints específicos.
Responsabilidades:
Registro.
Heartbeat.
Inventário.
Eventos.
Recebimento de comandos.
Sincronização.
Esses recursos não deverão ser utilizados pelo Portal.
# **30. Recursos do Portal**
O Portal utilizará exclusivamente endpoints administrativos.
Exemplos:
Dashboard.
Pesquisa.
Configuração.
Operações sobre Torrents.
Operações sobre Nodes.
Administração de usuários.
# **31. Recursos do Scheduler**
O Scheduler será acessado apenas pelo Control Plane.
Não existirão endpoints públicos para cálculo direto de decisões.
As informações disponibilizadas ao operador serão apenas de consulta.
Exemplos:
Histórico de decisões.
Critérios utilizados.
Estatísticas.
Versão do algoritmo.
# **32. Recursos de Monitoramento**
A API deverá permitir consultar:
Estado geral do Cluster.
Estado de cada Node.
Resumo operacional.
Health Scores.
Heartbeats.
Alertas ativos.
Essas informações serão utilizadas principalmente pelo Portal.
# **33. Recursos de Eventos**
A API deverá disponibilizar mecanismos eficientes para consulta de eventos.
Filtros recomendados:
Node.
Torrent.
Usuário.
Tipo.
Severidade.
Período.
Texto livre.
# **34. Recursos de Configuração**
Toda configuração do Cluster deverá ser administrada pela API.
Exemplos:
Scheduler.
Alertas.
Autenticação.
Retenção.
Feature Flags.
Integrações.
Nenhuma configuração global deverá depender de acesso direto ao banco.
# **35. Segurança**
Todos os endpoints deverão exigir autenticação, exceto aqueles explicitamente destinados ao registro inicial de um Agent.
A autorização deverá ser baseada em permissões definidas pelo Control Plane.
# **36. Auditoria**
Toda operação administrativa iniciada por usuários deverá gerar registro de auditoria.
Operações automáticas entre Agent e Control Plane não deverão gerar auditoria, apenas eventos operacionais.
# **37. Compatibilidade**
Novas versões da API deverão preservar compatibilidade sempre que possível.
Mudanças incompatíveis deverão resultar em nova versão da API.
Versões anteriores poderão coexistir durante período de transição.
# **38. Princípios**
A API representa um contrato.
Implementações internas poderão evoluir livremente, desde que os contratos permaneçam estáveis.
Essa separação deverá permitir evolução independente do Portal, do Control Plane e dos Agents.
# **39. Fluxos de Comunicação**
A API deverá suportar os principais fluxos operacionais da plataforma.
**Registro de um Node**
Agent
↓
Registro
↓
API Gateway
↓
Authentication Service
↓
Node Service
↓
Database
↓
Resposta
**Heartbeat**
Agent
↓
Heartbeat
↓
API Gateway
↓
Monitoring Service
↓
Database
**Adição de um Torrent**
Portal
↓
API Gateway
↓
Torrent Service
↓
Scheduler
↓
Node Service
↓
Agent
↓
Engine
**Consulta de Informações**
Portal
↓
API Gateway
↓
Serviço responsável
↓
Database
↓
Resposta
Consultas deverão utilizar preferencialmente dados persistidos.
# **40. Tempos de Resposta**
Objetivos da versão 1.0.
Autenticação
Inferior a 500 ms.
Consulta de Nodes
Inferior a 300 ms.
Consulta de Torrents
Inferior a 500 ms.
Dashboard
Inferior a 1 segundo.
Registro de Heartbeat
Inferior a 100 ms.
Esses valores representam metas arquiteturais.
# **41. Limitação de Requisições**
A API deverá permitir mecanismos de limitação de requisições.
Objetivos:
Proteger o Control Plane.
Evitar uso abusivo.
Evitar loops acidentais.
Os critérios de limitação poderão variar conforme o tipo de consumidor.
Exemplos:
Portal.
Agent.
CLI.
Integrações.
# **42. Disponibilidade**
A indisponibilidade temporária da API não deverá interromper o funcionamento dos Nodes.
Os Agents deverão continuar operando localmente até o restabelecimento da comunicação.
Após a reconexão, a sincronização deverá ocorrer automaticamente.
# **43. Documentação**
Toda funcionalidade disponibilizada pela API deverá estar documentada.
A documentação deverá incluir:
Descrição.
Parâmetros.
Permissões necessárias.
Respostas possíveis.
Exemplos de utilização.
Essa documentação deverá ser gerada automaticamente sempre que possível.
# **44. Evolução**
Novos recursos deverão ser adicionados sem alterar contratos existentes.
Quando alterações incompatíveis forem inevitáveis:
Nova versão da API.
Período de coexistência.
Migração documentada.
Descontinuação planejada.
# **45. Integrações Futuras**
A arquitetura deverá permitir integração com ferramentas externas.
Exemplos:
CLI oficial.
SDK.
Aplicativos móveis.
Dashboards externos.
Automação.
Webhooks.
Essas integrações utilizarão exatamente os mesmos contratos definidos neste documento.
# **46. Dependências**
A API dependerá apenas do Control Plane.
Toda regra de negócio continuará localizada nos serviços internos.
A API nunca deverá implementar lógica de negócio própria.
Sua responsabilidade limita-se a:
Receber.
Validar.
Encaminhar.
Responder.
# **47. Monitoramento**
A API deverá disponibilizar informações suficientes para acompanhamento operacional.
Exemplos:
Quantidade de requisições.
Tempo médio de resposta.
Erros.
Autenticações.
Sessões ativas.
Versão em execução.
Essas informações serão consumidas pelo sistema de monitoramento.
# **48. Testabilidade**
Todos os contratos deverão ser testáveis independentemente da implementação.
Sempre que possível, testes automatizados deverão validar:
Estrutura das respostas.
Autorização.
Compatibilidade.
Versionamento.
Tratamento de erros.
# **49. Resumo da Arquitetura**
Portal
│
────────────┼────────────
CLI
│
────────────┼────────────
Agent
│
────────────┼────────────
Integrações Futuras
│
════════════════════════════
API Gateway
════════════════════════════
│
Control Plane
════════════════════════════
Serviços Internos
════════════════════════════
PostgreSQL
# **50. Declaração Final**
A API constitui o contrato oficial de comunicação do SeedGrid.
Ela desacopla consumidores e serviços internos, preservando estabilidade, segurança e consistência durante toda a evolução da plataforma.
Toda interação com o SeedGrid deverá ocorrer exclusivamente através desta interface, garantindo que a arquitetura permaneça modular, extensível e independente das implementações internas.
