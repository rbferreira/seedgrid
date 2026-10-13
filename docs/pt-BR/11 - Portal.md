# **SeedGrid**
**Portal Specification**
Version: 1.0
Status: Draft
Document: 11 - Portal.md
# **1. Objetivo**
O Portal representa a interface oficial de administração do SeedGrid.
Seu objetivo é fornecer ao operador uma visão unificada do Cluster, permitindo monitoramento, administração e operação da plataforma através de uma interface web.
# **2. Filosofia**
O Portal deverá apresentar o Cluster como uma única plataforma.
O operador não deverá precisar conhecer detalhes da infraestrutura para executar tarefas rotineiras.
Sempre que possível, a interface deverá responder perguntas, e não apenas apresentar informações.
# **3. Responsabilidades**
O Portal será responsável por:
- apresentar o estado do Cluster;
- permitir operações administrativas;
- exibir monitoramento;
- apresentar eventos;
- apresentar alertas;
- gerenciar configurações;
- administrar usuários.
O Portal NÃO deverá:
- executar regras de negócio;
- tomar decisões do Scheduler;
- comunicar-se diretamente com os Nodes;
- acessar o banco de dados.
Toda comunicação ocorrerá exclusivamente através da API.
# **4. Estrutura Geral**
O Portal será dividido em módulos.
Portal
├── Dashboard
├── Cluster
├── Nodes
├── Torrents
├── Monitoramento
├── Feed  (atividade de orquestração ao vivo — ADR-022; separado de Eventos/Alertas)
├── Eventos
├── Alertas
├── Configurações
├── Usuários
└── Administração
> **Implementado (ago/2026) — organização das telas.** A navegação tem hoje
> **Dashboard**, **Baixados**, Adicionar Torrent, Publicar, Explorar, Feed,
> Alertas & Eventos, **Saúde & Métricas**, Auditoria, Usuários e Configurações.
> O módulo Torrents está dividido em duas telas por estado (fila no Dashboard,
> acervo em Baixados — ver §20) e a *listagem* de Nodes vive dentro de
> Saúde & Métricas (§18/§24). A página por Node (`/nodes/{id}`) continua.

# **5. Dashboard**
O Dashboard representa a visão geral do Cluster.
Seu objetivo é permitir que o operador compreenda rapidamente o estado da plataforma.
Informações sugeridas:
Quantidade de Nodes.
Nodes Online.
Nodes em manutenção.
Capacidade total.
Espaço disponível.
Quantidade de Torrents.
Upload agregado.
Download agregado.
Alertas ativos.
Eventos recentes.
# **6. Módulo Cluster**
Responsável por apresentar informações globais.
Exemplos:
Nome do Cluster.
Versão.
Quantidade de Nodes.
Capacidade total.
Espaço utilizado.
Resumo operacional.
Health geral.
# **7. Módulo Nodes**
Responsável por apresentar todos os Nodes.
Cada Node deverá possuir uma página própria contendo:
Informações gerais.
Estado.
Hardware.
Inventário.
Discos.
Containers.
Capacidades.
Labels.
Tags.
Torrents hospedados.
Eventos.
Alertas.
> **Implementado (ago/2026).** A página por Node segue existindo em
> `/nodes/{id}` com inventário, recursos e os torrents hospedados. A **listagem**
> de Nodes deixou de ser tela própria: o resumo operacional está na tabela
> "Seed Nodes" do Dashboard e o console por Node (recursos, agregados e ações)
> em Saúde & Métricas. `/nodes` redireciona para lá.

# **8. Módulo Torrents**
Representa a principal área operacional.
Funcionalidades previstas:
Pesquisa.
Filtros.
Categorias.
Tags.
Detalhes.
Trackers.
Arquivos.
Histórico.
Operações administrativas.
O operador deverá enxergar todos os Torrents do Cluster em uma única lista.
# **9. Módulo Monitoramento**
Responsável por apresentar a saúde da plataforma.
Exemplos:
Health Score.
Utilização de CPU.
Utilização de memória.
Espaço em disco.
Temperatura.
Disponibilidade.
Essas informações serão consolidadas por Node.
# **10. Módulo Eventos**
Permite consulta aos eventos registrados.
Filtros sugeridos:
Período.
Node.
Torrent.
Tipo.
Severidade.
Texto livre.
# **11. Módulo Alertas**
Apresenta problemas ativos.
Cada alerta deverá conter:
Título.
Descrição.
Severidade.
Origem.
Data de criação.
Estado.
Ações disponíveis.
# **12. Módulo Configurações**
Centraliza os tunáveis de comportamento do Control Plane (ADR-023), editáveis em runtime
com **hot-reload** (valem sem redeploy). A página (`/settings`) agrupa os parâmetros por
categoria (Admissão, Alertas, Workers); cada campo traz **label + descritivo** (do catálogo),
input tipado (duração como texto `5m`/`90s`; números), indicação de "modificado" (override no
banco) e ação de **restaurar padrão**. O valor efetivo é o override da tabela `settings` sobre
o default do catálogo; o env var só semeia no 1º boot. Catálogo em `25 - Business Rules.md`.
Exemplos de parâmetros:
Admissão (intervalo, taxa mínima, graces/cooldowns).
Alertas (offline-after, saturação de disco, retenção).
Workers (intervalos de reconciler e prometheus-writer).
Agent / Engine (heartbeat, polling de comandos, nível de log e a fila do Engine — ver abaixo).

**Agent / Engine (ADR-039, #144).** A categoria traz o **valor de cluster** do comportamento dos Seed
Nodes: cada campo leva o selo **cluster · por Node** (este é o valor de todos os Nodes que não têm valor
próprio) e, por chegar a **todos** esses Nodes no próximo heartbeat, **salvar ou restaurar pede confirmação**.
Logo abaixo, a lista **Nodes com valor próprio** (`GET /api/v1/settings/overrides`) responde "se eu mudar
isto, quem **não** é afetado?", com link para o detalhe de cada Node, onde o valor próprio se edita.
# **13. Módulo Usuários**
Permite administração de operadores.
Exemplos:
Cadastro.
Permissões.
Papéis.
Sessões.
Auditoria.

## 13.1 Papéis de sistema (ADR-035)

O Portal filtra navegação e ações pelos papéis/permissões do operador autenticado
(`GET /api/v1/auth/me`). Papéis de sistema:

- **Administrator** — acesso total; vê tudo, incluindo a página **Usuários**.
- **Publisher** — gerencia conteúdo: Adicionar Torrent, Publicar, Explorar, Trackers.
- **Downloader** — consome: Explorar e baixar torrents.
- **Viewer** — somente leitura: Dashboard, Nodes, Explorar, Feed, Alertas, Métricas.

A página **Usuários** (admin) lista operadores, cria novos (username, senha,
papéis), edita papéis/status, redefine senhas e desabilita contas. Itens de menu
sem permissão não aparecem; acesso direto por URL é barrado pelo backend (403).

**Gate por botão** (refina a ADR-035, sem ADR nova). O gate de rota e de item de
menu não basta: dentro de uma página que vários papéis alcançam há botões de outros
domínios. A tela Saúde & Métricas é aberta por `metrics:read`, mas hospeda ações de
Node (`nodes:write`) e o join token (`enroll:write`); o **Dashboard** é a página de
*fallback* de toda rota negada — o que está nele está exposto a **todos** os papéis,
inclusive o que não tem permissão de nada.

A regra tem três partes:
1. **Todo botão que dispara uma rota protegida só aparece para quem tem a MESMA
   permissão que o backend exige nessa rota.** Nem uma mais frouxa (o botão aparece e
   o clique toma 403) nem uma mais estrita (some o que o papel podia usar). A
   permissão do botão é a do `permitted("…")` da rota em `api.go`.
2. **Esconder o botão, nunca o dado.** O operador somente-leitura continua vendo
   disco, memória, I/O, health, a fila, o modo de operação e que existe versão nova
   do Agent — só não vê o que não pode executar. Onde o botão era a única forma de
   mostrar um dado (o interruptor de modo de operação, o badge de "↑ versão"), o dado
   fica num elemento inerte (texto/badge), não some junto. Uma coluna de ações que
   ficaria vazia para o papel some inteira.
3. **A página recebe `can`** de `App.tsx` (que libera enquanto o perfil não resolve,
   para a UI não piscar vazia — bug #5 da review da ADR-035) em vez de consultar o
   perfil por conta própria; assim o critério é um só para o Portal inteiro. Toda
   página nova com botão de ação nasce com `can` — e toda rota que cai no Dashboard
   também o leva.

Botões gateados, por tela (permissão = a da rota que chamam):

| Tela | Botão | Permissão |
|---|---|---|
| Dashboard | Admissão Full auto/Flex | `nodes:operation-mode` |
| Dashboard | Modo por Node SG/Manual | `nodes:operation-mode` |
| Dashboard | "↑ versão" (atualizar Agent) | `nodes:write` |
| Dashboard | Fila: priorizar, pausar/retomar, reverificar | `torrents:write` |
| Dashboard | Fila: baixar conteúdo | `torrents:download` |
| Dashboard | Fila: migrar | `torrents:migrate` |
| Dashboard | Fila: excluir | `torrents:delete` |
| Detalhe do Node | Reiniciar Engine, Renomear | `nodes:write` |
| Detalhe do Node | Configuração: valor próprio / alterar / voltar ao cluster | `nodes:write` |
| Detalhe do Node | pausar/retomar, reverificar torrent | `torrents:write` |
| Detalhe do Node | excluir torrent | `torrents:delete` |
| Detalhe do Torrent | Baixar | `torrents:download` |
| Detalhe do Torrent | Pausar/Retomar, Reverificar | `torrents:write` |
| Detalhe do Torrent | Excluir | `torrents:delete` |
| Baixados | baixar / pausar·retomar·reverificar / migrar / excluir | `torrents:download` / `write` / `migrate` / `delete` |
| Saúde & Métricas | Adicionar Node | `enroll:write` |
| Saúde & Métricas | Esvaziar, Reiniciar Engine, Remover, Renomear | `nodes:write` |
| Catálogo (Explorar) | Baixar & Seedar | `trackers:seed` |
| Alertas & Eventos | Cancelar migração (migração em voo) | `torrents:migrate` |
| Configurações → Trackers → perfil | Exportar perfil | `trackers:read` |
| Configurações → Trackers → perfil | Importar perfil (arquivo → pré-visualização → aplicar) | `trackers:write` |
| Configurações → Trackers → perfil | Composição da descrição: ligar/desligar/reordenar blocos, tag do MediaInfo, Salvar e Restaurar o padrão | `trackers:write` |
| Configurações → Trackers → perfil | Host de imagem (#193): editar/remover a seção `image_host`, cadastrar/remover a chave do host | `trackers:write` |

O backend permanece a autoridade: o gate de UI evita o clique que tomaria 403 — **não**
é controle de acesso. Configurações, Usuários e Auditoria seguem gateadas só por rota: hoje
só o papel Administrator as alcança, e as ações internas delas (`settings:write`,
`trackers:write`, `notify:write`, `users:write`) também são só dele. Os botões de exportar e importar perfil de tracker são a exceção: já nascem com `can`, pela permissão da rota que chamam (tabela acima). Se um dia existir
papel customizado com acesso parcial a essas telas, os botões delas entram na tabela acima.
# **14. Navegação**
A navegação deverá privilegiar rapidez e simplicidade.
O operador deverá alcançar qualquer funcionalidade principal com poucos cliques.
A estrutura de navegação deverá permanecer consistente em todos os módulos.
# **15. Objetivos**
O Portal deverá:
- apresentar informações de forma clara;
- facilitar diagnóstico;
- reduzir operações repetitivas;
- concentrar todas as funcionalidades administrativas.
# **16. Princípio Fundamental**
O Portal representa exclusivamente a interface do operador.
Toda inteligência permanece no Control Plane.
Toda execução permanece nos Agents.
O Portal observa, administra e apresenta o Cluster como um ambiente único.
# **17. Dashboard**
O Dashboard deverá ser a página inicial da plataforma.
Seu objetivo é permitir que o operador compreenda a situação do Cluster em poucos segundos.
O Dashboard deverá priorizar informações relevantes e evitar excesso de detalhes.
# **Indicadores**
Indicadores sugeridos:
Nodes Online
Nodes em Manutenção
Nodes Offline
Capacidade Total
Espaço Utilizado
Quantidade de Torrents
Upload Total
Download Total
Alertas Ativos
Eventos Recentes
> **Implementado (ago/2026).** Além dos indicadores, o Dashboard traz os
> agregados do cluster (baixado, enviado, disco), o **gráfico de throughput
> acumulado do cluster** com períodos de 1h a 1 ano (fonte `cluster_stat_samples`
> via `/metrics/cluster-throughput`), a tabela "Seed Nodes", a tabela
> **"Fila & Downloads"** (§20) e os rankings de mais enviado/baixado — cujos
> nomes levam ao detalhe do torrent.

# **Visão Geral**
O Dashboard deverá responder perguntas como:
O Cluster está saudável?
Existe algum Node indisponível?
Há alertas críticos?
Existe risco de falta de espaço?
Há operações em andamento?
# **18. Tela de Nodes**
A listagem de Nodes deverá apresentar um resumo operacional.
Cada linha deverá conter, no mínimo:
Nome
Estado
Health Score
Espaço Livre
Quantidade de Torrents
Versão do Agent
Último Heartbeat
Labels
Tags
A partir dessa tela será possível acessar os detalhes completos do Node.

> **Implementado (ago/2026) — sem tela própria, em dois lugares.** As colunas
> acima continuam atendidas, divididas por pergunta:
> - **Dashboard → tabela "Seed Nodes"**: nome do Node (o apelido, se houver —
>   ver abaixo), estado, modo de operação (ADR-024), health, Engine, endpoint BT,
>   versão do Agent (com o botão de atualizar — ADR-027) e último heartbeat.
> - **Saúde & Métricas → cards por Node**: espaço livre, memória, I/O de disco,
>   CPU/load, uptimes, storage OK/FALHOU com o motivo, quantidade de torrents e
>   agregados de tráfego, mais as ações (Esvaziar/drain, Reiniciar Engine,
>   Remover, Adicionar Node, Detalhes →).
>
> Labels e Tags seguem não implementadas. Os botões destrutivos são gateados por
> `nodes:write`/`enroll:write` — ver §13.1.
>
> **Apelido do Node (#145).** O Node entra com o hostname do SO; o operador pode dar
> um **apelido** que vive só dentro do SeedGrid (o hostname da máquina não é tocado —
> ele prefere hostnames numerados, tipo `seedgrid-node-01`).
> - **Ao adicionar:** o modal "Adicionar Node" pergunta o nome (opcional) **antes** de
>   gerar o comando. O nome viaja no join token e é aplicado no registro; o comando
>   copiado é o mesmo de sempre.
> - **Depois:** "Renomear" no card do Node (Saúde & Métricas) e no detalhe do Node,
>   com `nodes:write`. Não toca no Node nem reinicia o Agent; vazio volta ao hostname.
> - **Onde aparece:** em toda tela que mostra o nome de um Node — Dashboard, Saúde
>   (cards, confirmações, legenda dos gráficos), Baixados, detalhe do torrent, Alertas
>   & Eventos, Feed, Analytics, seletor de Node ao adicionar torrent e o picker de
>   destino de migração. Uma só regra no código (`lib/node-label.ts`): apelido quando
>   existe, hostname senão.
> - **O hostname segue visível como dado técnico:** secundário ao lado do nome no card
>   e no detalhe do Node (que o lista também em Inventário), em tooltip nas tabelas,
>   e junto do nome nas confirmações destrutivas (remover, reiniciar Engine,
>   atualizar Agent) — o operador reconhece o Node pelo apelido, mas a ação
>   irreversível mostra a máquina real que vai sofrê-la.
> - **Fora do escopo:** o texto das mensagens de Alertas/Eventos e das notificações
>   externas continua usando o hostname (ver `13 - API.md`).

# **19. Detalhes do Node**
Cada Node possuirá uma página própria.
Seções sugeridas:
Resumo
Hardware
Sistema Operacional
Armazenamento
Interfaces de Rede
Containers
Torrent Engine
Inventário
Eventos
Alertas
Histórico
Operações disponíveis:
Entrar em manutenção.
Sair da manutenção.
Atualizar inventário.
Visualizar logs.
Consultar métricas.

**Configuração deste Node (ADR-039, #144).** Seção do detalhe do Node entre Inventário/Recursos e a lista
de torrents. É uma tabela com **Parâmetro · Valor efetivo · Origem · Ações**:
- **Valor efetivo** é o que o Agent recebe; **Origem** é *cluster* ou **valor deste Node** (em destaque — é
  a diferença deliberada em relação à frota — e mostra o que o cluster diria: «cluster: 10»).
- O selo no título diz se o Agent está **em dia**, **aguardando o Agent** (mudou e ele ainda não confirmou:
  aplica no próximo heartbeat) ou **sem suporte** (Agent anterior à ADR-039, que segue no `.env` local —
  não é erro).
- **Valor próprio** / **Alterar** / **Voltar ao cluster** exigem `nodes:write` (gate por botão, §13.1: sem a
  permissão o botão **e a coluna Ações** somem; o dado fica). O backend valida pelo mesmo `Spec` do cluster
  e a mudança é auditada com o antes e o depois. A validação do browser é só retorno rápido.
# **20. Tela de Torrents**
Esta será a principal área operacional do Portal.
A listagem deverá permitir:
Pesquisa instantânea.
Ordenação.
Filtros.
Seleção múltipla.
Paginação.
Atualização automática.
Colunas sugeridas:
Nome
Node
Status
Progresso
Seeds
Peers
Ratio
Upload
Download
Categoria
Tags

> **Implementado (ago/2026) — duas telas por estado.** Em regime quase todo
> torrent está semeando, e uma lista única enterrava o que pede ação. A divisão
> usa um predicado só (`isDone`) e o seu complemento, de modo que todo torrent
> aparece em exatamente uma das duas telas — inclusive um status novo do Agent.
> - **Dashboard → "Fila & Downloads"**: o que não está semeando (baixando,
>   aguardando, pausado, em recheck, com erro). Ações: priorizar na fila,
>   baixar conteúdo, pausar/retomar, reverificar, migrar, excluir.
> - **Baixados** (`/downloaded`): o acervo semeando, com **pesquisa instantânea
>   por nome, filtros (Node, status, tamanho mínimo, período de adição) e
>   ordenação por todas as colunas de dado** — inclusive a coluna **Node**. Abre
>   ordenada por **Adicionado**, mais recentes primeiro: quem chega à tela em
>   geral procura o que acabou de baixar. Ações: baixar conteúdo,
>   pausar/retomar, reverificar, migrar, excluir.
>
> Colunas em Baixados: Nome, Status, Tamanho, Node, Adicionado, Velocidade,
> Seeds/Peers, Baixado/Enviado, Ratio. **Adicionado** é o `created_at` do torrent
> no Control Plane — relativo até 30 dias ("há 3 h"), data depois disso, instante
> completo no hover. A migração entre Nodes preserva a data; um torrent *adotado*
> do Engine carrega o instante da adoção, não o de quando entrou no Engine. O
> filtro de período usa janelas móveis (últimas 24 h / 7 dias / 30 dias). Progresso fica de fora (em SEEDING é sempre 100%).
> Filtros e ordenação são client-side sobre `GET /api/v1/torrents` — o endpoint
> não tem paginação nem filtro, e nesta ordem de grandeza não precisa.
> Seleção múltipla e paginação seguem não implementadas (ver §34).

# **21. Detalhes do Torrent**
Cada Torrent deverá possuir uma página dedicada.
Informações sugeridas:
Resumo
Trackers
Arquivos
Estatísticas
Histórico
Eventos
Operações realizadas
Node responsável
Localização física
Operações disponíveis:
Pausar
Retomar
Recheck
Remover
Alterar Categoria
Alterar Tags

**Screenshots na publicação (ADR-026, ADR-040).** No fluxo de cadastro em tracker UNIT3D (ADR-020),
o campo de descrição ganha um botão **"Gerar screenshots"**: chama o CP com o `tracker_id` selecionado
(que orquestra a extração no Agent + upload na cadeia de hosts) e insere o BBCode retornado no início da
descrição, sem sobrescrever o que já houver (ex.: sinopse do TMDB). O operador ainda
**revisa/edita/remove** o BBCode antes de publicar — a conferência humana da ADR-020
permanece o único gate para o envio ao tracker. O painel mostra, após o upload, **onde as imagens
ficaram** (o host que hospedou) e as **falhas** dos hosts anteriores na cadeia, se houver. O botão
"Gerar BBCode" (ficha completa) também envia o `tracker_id` e exibe o mesmo resultado na barra de etapas.

**Taxonomia do tracker vem do perfil (ADR-037, #196).** Categoria, tipo e resolução do cadastro
em tracker UNIT3D **não** são listas do Portal: o painel renderiza o que o `profile` do tracker
declara (`GET /api/v1/trackers`), porque os IDs são do site — `Encode` é 3 num e 12 noutro. O papel
da categoria (`kind`: série mostra temporada/episódio, jogo abre a busca no IGDB) também vem do
perfil, e o pré-preenchimento a partir do nome casa por **rótulo** ("1080p", "WEB-DL"), nunca
reaproveitando o número que o parser devolve (que é o do samaritano). O que não casa fica em
"— selecione —": ninguém escolhe por baixo dos panos. Tracker com perfil sem categorias e tipos
avisa, no painel e na lista de trackers, em vez de oferecer selects vazios sem explicação.

Em **Configurações → Trackers de publicação**, o botão **perfil** de cada tracker abre o editor da
taxonomia (id, nome, papel e origem de cada termo) e o **"Descobrir taxonomia"**: varre N páginas do
catálogo do próprio tracker e mostra uma **proposta editável** — quantas páginas foram varridas, por
que parou, e o que é novo, o que já está no perfil e o que o site chama de outro jeito. Vem marcado só
o que é novo; renomear é opt-in; o que o operador ajustou à mão aparece como protegido. A tela diz,
em destaque, que a descoberta **só enxerga o que já foi postado**: o perfil descoberto não é completo,
e o que faltar se adiciona à mão. Nada é gravado até "Aplicar selecionados".

**Exportar e importar o perfil (ADR-037, #197).** No mesmo painel **perfil**, o bloco **Compartilhar o
perfil deste site** tem dois botões. **Exportar perfil** baixa o arquivo JSON (o Portal o busca com o
token de sessão e o entrega ao browser como arquivo) e diz, ao terminar, o que ele leva — o nome que o
operador deu ao tracker e o host — e que **nunca** leva token, passkey nem announce: o nome é rótulo livre
e vale conferir antes de publicar o arquivo. **Importar perfil** abre o seletor de arquivo e segue três
passos, sempre nesta ordem:

1. **Pré-visualização** (não grava nada): de que site é o arquivo, o aviso de que o conteúdo veio de
   fora e **não foi conferido contra o site do operador** (um id errado publica na categoria errada),
   os avisos do servidor — em especial o de arquivo de **outro site** —, e cada termo com o que
   acontece com ele: *novo*, *já está*, *renomeia* (com o nome antigo riscado) ou *mantido* —
   *você chamou de «X»* (manual) ou *o seu site usa «X»* (confirmado pela descoberta). Um termo que
   passaria a dividir o nome com outro, ou a ser a primeira categoria de um papel, traz um aviso
   âmbar **na própria linha**; e o aviso de arquivo de outro site sai também quando o arquivo (ou o
   tracker) não tem host para comparar. O resumo diz quantos termos do perfil o arquivo não traz e que o import **nunca
   remove**.
2. **A escolha**: só os termos *mantidos* (do operador ou do site dele) têm uma caixa — "usar o do arquivo". Marcar
   muda a conta na hora; o que não for marcado fica como está.
3. **Aplicar importação**: desabilitado quando não há nada a mudar. O resultado é o que o servidor
   **executou** (que pode diferir da pré-visualização se o perfil mudou no intervalo).

O que entra ganha a etiqueta **importado** (violeta): o id veio de fora e o site do operador ainda não o
confirmou; rodar a descoberta é o que o confirma. As duas ações ficam **desabilitadas com alterações não
salvas** no editor — o arquivo levaria o perfil salvo, e o import o trocaria por baixo do que está sendo
digitado. O Portal recusa antes de ler um arquivo vazio ou acima de 512 KiB (o mesmo teto do servidor)
e envia o texto do arquivo **como está**: quem valida é o servidor, e é a mensagem dele que aparece.
A conferência de rótulo do painel de publicação (`findByLabel`) só pré-preenche quando o rótulo é
**inequívoco**: com dois termos de mesmo nome no eixo (sem caixa nem acento), nenhum é escolhido e o
operador seleciona.
**Descrição do upload composta pelo Control Plane (ADR-037, #192).** O Portal **não monta mais** o
BBCode da descrição (`buildBBCode`/`buildGameBBCode` saíram de `bbcode.ts`, que ficou só com os helpers de
MediaInfo e tamanho): a **"Etapa 3: monta a ficha"** do botão de ficha completa e a escolha de um jogo
no IGDB (`pickGame`) chamam `POST /api/v1/publish/description` (`composeMediaDescription` e
`composeGameDescription`, em `api.ts`) e entregam o texto ao campo, que o operador **continua editando**.
A composição segue o perfil **do tracker** escolhido — dois sites podem pedir ordem e tag de MediaInfo
diferentes. **Falha não apaga nada:** se a chamada falhar, o erro aparece (na barra de etapas e no texto
de erro da ficha, ou junto da busca de jogo) e o campo de descrição fica **exatamente como estava**; ao
escolher um jogo, o IGDB ID é preenchido na hora, e duas escolhas seguidas valem pela última (uma resposta
atrasada não sobrescreve a descrição do jogo escolhido depois).

**Composição da descrição no editor do perfil (ADR-037, #192).** No painel **perfil** de cada tracker, o
bloco **Composição da descrição** mostra se a composição é **padrão** ou **personalizada** e, para quem
pode escrever, deixa **ligar e desligar** cada bloco (pôster, sinopse, tags, screenshots e MediaInfo; capa,
título, sobre, gêneros e plataformas para jogos), **reordenar** (↑ ↓) e escolher a **tag do MediaInfo**
(`[mediainfo]`, `[code]`, `[pre]` ou `[spoiler]`). **Salvar composição** só habilita com alteração; **Restaurar
o padrão** só aparece quando há composição própria. O catálogo de blocos e o default vêm do servidor
(`description_default` de cada tracker): o Portal não duplica a lista. Um bloco desligado fica riscado e
**não entra** na descrição; a ordem da tela é a ordem salva. As alterações não salvas **bloqueiam
exportar e importar** o perfil, como as da taxonomia. Quem só pode ler o tracker vê a composição como
**texto** (o dado fica; o controle some — §13.1). Na **pré-visualização do import**, um arquivo que traz a
composição ganha o bloco **Composição da descrição**: o que o arquivo traz em palavras (ordem e tag), o que
acontece com a do perfil — *será aplicada*, *já é a do seu perfil*, *substitui a que você configurou* — e,
quando a do operador seria **mantida**, a caixa "usar a do arquivo" (a mesma escolha, só que da seção
inteira: `?overwrite=description`).

**Host de imagem no editor do perfil (ADR-037/040, #193).** No painel **perfil** de cada tracker, o
bloco **Host de imagem do tracker** mostra se o host está **configurado** ou **só o imgbb** e, para quem
pode escrever, deixa editar os sete campos da seção (`name`, `upload_url`, `file_field`, `auth_header`,
`auth_prefix`, `url_path`, `thumbnail_path`). **Salvar host** só habilita com alteração; **Remover host**
só aparece quando há host configurado. A chave do operador (camada 3) fica num bloco separado: campo tipo
senha (sem oferta de salvar como senha) com o aviso "a chave é enviada só para `<host>`", o selo **chave
cadastrada** (quando já existe) e o aviso **o host da chave diverge do upload_url — ela não será enviada**
quando o host do `upload_url` **salvo** (não o do formulário em edição) difere do `image_host_key_host`
gravado. Uma chave **órfã** — cadastrada, mas sem a seção no perfil — aparece com o selo e o **Apagar
chave**, para poder ser removida; cadastrar uma nova exige a seção. **Cadastrar chave** e **Apagar chave**
mudam estado e são gateados por `can("trackers:write")`; quem só pode ler vê a seção como texto, com o selo
e o aviso de divergência (o dado fica; o controle some — §13.1).
As alterações não salvas **bloqueiam exportar e importar** o perfil, como as demais seções. Na
**pré-visualização do import**, um arquivo que traz a seção ganha o bloco **Host de imagem**: o que o
arquivo traz em palavras (nome e URL de upload), o que acontece com a do perfil — *será aplicado*, *já é o
do seu perfil*, *substitui o que você configurou* — e, quando a do operador seria **mantida**, a caixa
"usar o do arquivo" (a mesma escolha, só que da seção inteira: `?overwrite=image_host`).
# **22. Tela de Eventos**
Os eventos deverão funcionar como um histórico operacional do Cluster.
Filtros recomendados:
Período
Node
Torrent
Usuário
Tipo
Severidade
Texto
Os eventos deverão possuir links para os recursos relacionados.
# **23. Tela de Alertas**
Os alertas representam problemas ativos.
Cada alerta deverá apresentar:
Título
Descrição
Origem
Severidade
Data
Estado
Ações disponíveis
Filtros:
Estado
Severidade
Node
Categoria

> **Implementado (#87) — Migrações recentes e Cancelar migração.** A tela
> Alertas & Eventos (`/alerts`) tem o painel **Migrações recentes**
> (`GET /api/v1/migrations`): Node de origem → destino, estado e, quando houver, o
> motivo. Estados no idioma do operador: **Na fila**, **Em andamento** (cobre a
> transferência *e* a verificação do recheck no destino, que pode levar horas),
> **Concluída**, **Falhou** e **Cancelada** (falha encerrada pelo operador, vinda de
> `cancelled: true` — não é pintada de vermelho como falha).
>
> Cada migração **em voo** (Na fila / Em andamento) mostra o botão **Cancelar
> migração**, que é a saída para o torrent que as ações recusam
> (`409 migration_in_flight`) enquanto a migração dura. Segue o gate por botão:
> aparece só com `torrents:migrate` (Administrator e Publisher) — o **dado** da
> migração fica visível a quem lê. Pede **confirmação**, porque cancelar **descarta a
> cópia** já feita no Node destino (e, se a cópia ainda estiver em andamento, ela
> termina e só então é descartada); o torrent continua no Node de origem. Migração
> ainda na fila avisa que nada foi copiado. O resultado vira toast, e o painel
> recarrega.
# **24. Tela de Monitoramento**
Esta tela consolida informações operacionais do Cluster.
Informações sugeridas:
CPU
Memória
Disco
Rede
Temperatura
Health Score
Disponibilidade
Essas informações poderão ser apresentadas através de gráficos e indicadores.

> **Implementado (ago/2026).** A tela **Saúde & Métricas** tem: saúde do Control
> Plane (API, banco, Workers) com CPU/memória do CP; **um card por Seed Node**
> (recursos do heartbeat + agregados dos seus torrents + ações de Node); e
> "Tendências por Node" com as séries do Prometheus (CPU, memória, rede, disco,
> utilização de disco). O gráfico de **throughput acumulado do cluster** mudou
> para o Dashboard (§17): "como o cluster vem se comportando" é pergunta de
> visão geral. Temperatura segue não implementada.

# **25. Configurações**
As configurações deverão ser organizadas por categoria.
Exemplo:
Cluster
Scheduler
Monitoramento
Segurança
Integrações
Retenção
Feature Flags
Sempre que possível, alterações deverão informar seu impacto antes da confirmação.
# **26. Administração**
Área destinada à gestão da plataforma.
Exemplos:
Usuários
Papéis
Permissões
Sessões
Tokens
Auditoria
Versão da Plataforma
# **27. Pesquisa Global**
O Portal deverá disponibilizar uma pesquisa unificada.
A pesquisa deverá localizar:
Nodes
Torrents
Eventos
Alertas
Usuários
Configurações
Os resultados deverão ser agrupados por tipo de recurso.
# **28. Navegação**
A navegação deverá permanecer consistente em toda a plataforma.
Princípios:
Poucos níveis hierárquicos.
Menu lateral permanente.
Breadcrumb nas páginas internas.
Pesquisa sempre acessível.
Links cruzados entre recursos relacionados.
# **29. Responsividade**
O Portal deverá ser utilizável em:
Desktop
Notebook
Tablet
Dispositivos móveis terão suporte apenas para operações de consulta e administração simples.
Operações complexas deverão priorizar a experiência em telas maiores.
# **30. Princípios**
O Portal deverá privilegiar:
Clareza.
Rapidez.
Consistência.
Baixa curva de aprendizado.
Informações contextualizadas.
A interface deverá transmitir ao operador a sensação de estar administrando um único Cluster, e não diversos equipamentos independentes.
# **31. Experiência do Operador**
O Portal deverá ser projetado para reduzir a complexidade operacional.
O operador deverá conseguir localizar rapidamente informações, compreender o estado do Cluster e executar ações administrativas sem necessidade de conhecer detalhes internos da arquitetura.
# **32. Organização Visual**
Todas as telas deverão seguir um padrão visual consistente.
Princípios:
- mesma organização de menus;
- mesmos componentes visuais;
- mesmos padrões de cores;
- mesmas convenções de navegação;
- mesmos comportamentos para ações recorrentes.
A experiência deverá ser uniforme em toda a plataforma.

## 32.1 Semântica de cor da transferência

O eixo de transferência deverá ser colorido pelo **significado do dado**, nunca pela sua
posição na tela, na tabela ou na paleta do gráfico:

| Semântica | Cor | Onde aparece |
| --- | --- | --- |
| **Entrada** — download, baixado, leechers/peers | **azul** | velocidade ↓, "Total baixado", coluna Baixado, ação "Baixar", badge `DOWNLOADING`, atividade "Baixando" |
| **Saída** — upload, enviado, seeds, seeding | **verde** | velocidade ↑, "Total enviado", coluna Enviado, badge `SEEDING`, atividade "Seedando" |

A regra vale nos dois temas e também nas **séries dos gráficos**: uma série com o rótulo
`transfer: "down"` é azul e uma com `transfer: "up"` é verde, ainda que a paleta
categórica lhe atribuísse outra cor pela ordem de chegada. O significado viaja nesse rótulo,
não no nome da série — a legenda é traduzida (`/metrics/cluster-throughput` manda `transfer`
ao lado de um `instance` padrão, que o Portal troca pelo texto no idioma do operador). Séries sem semântica própria
(uma por Seed Node, por exemplo) continuam recebendo matizes distintos da paleta
categórica, cuja única exigência é serem distinguíveis entre si.

Num par exibido junto ("Baixado / Enviado", "Seeds / Peers", "Seeding / Baixando"), cada
metade recebe a sua cor — um par com uma cor só se lê como um valor só.

> **Implementação.** A convenção mora num lugar só: `TRANSFER` (classes) e `TRANSFER_HEX`
> (hex por tema, para o Recharts, que escreve `stroke` como atributo SVG e não resolve
> `var()`), em `portal/src/components/ui.tsx`. Tela nova consome a constante em vez de
> repetir a classe de cor à mão.
# **33. Estados da Interface**
Toda operação deverá apresentar claramente seu estado.
Exemplos:
Em execução.
Concluída.
Falhou.
Aguardando.
Cancelada.
Sempre que possível, o operador deverá receber retorno imediato sobre a solicitação realizada.
# **34. Operações em Lote**
Sempre que aplicável, o Portal deverá permitir operações sobre múltiplos recursos.
Exemplos:
Pausar vários Torrents.
Adicionar Tags.
Alterar Categoria.
Colocar múltiplos Nodes em manutenção.
Atualizar inventário de vários Nodes.
As operações deverão ser apresentadas de forma clara, informando quantidade de itens afetados.
# **35. Histórico**
Recursos importantes deverão disponibilizar histórico.
Exemplos:
Nodes.
Torrents.
Configurações.
Usuários.
Alertas.
O histórico deverá utilizar as informações já registradas pelo Control Plane.
# **36. Auditoria**
O Portal deverá disponibilizar acesso aos registros de auditoria.
Filtros sugeridos:
Usuário.
Período.
Operação.
Recurso.
Resultado.
A auditoria deverá permanecer separada dos eventos operacionais.
# **37. Feedback**
O Portal deverá informar claramente o resultado das operações.
Exemplos:
Operação concluída.
Operação agendada.
Operação recusada.
Permissão insuficiente.
Erro de comunicação.
As mensagens deverão ser objetivas e contextualizadas.
# **38. Acessibilidade**
Sempre que possível, a interface deverá seguir boas práticas de acessibilidade.
Exemplos:
Contraste adequado.
Navegação por teclado.
Textos alternativos.
Hierarquia visual consistente.
# **39. Internacionalização**
A arquitetura deverá permitir suporte a múltiplos idiomas.
A versão inicial utilizará um único idioma configurável.
Novos idiomas poderão ser adicionados sem alteração da arquitetura do Portal.
# **40. Personalização**
O Portal poderá permitir preferências individuais do operador.
Exemplos:
Tema claro ou escuro.
Quantidade de itens por página.
Página inicial.
Ordenação padrão.
Colunas visíveis.
Essas preferências não deverão alterar o comportamento global da plataforma.

> **Estado (ago/2026).** Só o tema é persistido (`localStorage`). Os filtros e a
> ordenação da tela Baixados valem apenas enquanto a página está aberta: o
> roteador é por hash puro (sem query string para deep-link) e filtro persistido
> costuma virar armadilha — dias depois a lista aparece "vazia" sem explicação.
# **41. Performance**
Objetivos da versão 1.0:
Abertura do Dashboard inferior a 2 segundos.
Pesquisa inferior a 500 ms.
Troca entre páginas inferior a 300 ms.
Atualização de informações sem necessidade de recarregar toda a interface, sempre que possível.
**Idioma (ADR-041).** O Portal é multilíngue (`pt-BR` e `en` na V1) por um catálogo próprio em `portal/src/i18n/`, sem biblioteca: um objeto TypeScript por idioma, em que o de `pt-BR` define o tipo e os demais têm de satisfazê-lo — chave faltando ou sobrando não compila. O idioma vem da escolha do operador (guardada no navegador na chave `sg-lang`, como o tema), depois do idioma do navegador, depois inglês. Nenhuma tela escreve texto de interface direto no componente; alertas, eventos e atividades têm a frase montada aqui, a partir de `code` + `payload`. O mecanismo usa `useT()` e `translate()` nos componentes, `detectLang()` resolve o idioma inicial, e o teste-guarda `no-literals.test.ts` impede que um arquivo já convertido volte a ter literal.

# **42. Dependências**
O Portal dependerá exclusivamente da API do Control Plane.
Nenhuma informação será obtida diretamente dos Agents ou do banco de dados.
Essa separação permitirá evolução independente entre frontend e backend.
# **43. Evolução**
Novos módulos poderão ser adicionados mantendo a mesma estrutura de navegação e os mesmos princípios definidos neste documento.
A arquitetura deverá favorecer expansão sem comprometer a experiência do operador.
# **44. Resumo da Arquitetura**
Operador
│
Navegador
│
Portal Web
│
API Gateway
│
Control Plane
│
┌─────────────┼─────────────┐
│             │             │
Agent 01      Agent 02      Agent 03
O Portal representa apenas a camada de apresentação.
Toda lógica permanece centralizada no Control Plane.
# **45. Declaração Final**
O Portal foi concebido para oferecer uma visão unificada do Cluster, abstraindo a complexidade da infraestrutura e permitindo que o operador concentre-se nas operações da plataforma.
Sua função é apresentar informações, facilitar a administração e fornecer uma experiência consistente para gerenciamento do SeedGrid, preservando a separação entre apresentação, decisão e execução estabelecida pela arquitetura da plataforma.
