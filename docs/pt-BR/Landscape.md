# SeedGrid — Landscape de Projetos Similares (prep open-source)

> **Documento não-normativo.** Não faz parte da especificação (não entra na precedência
> ADR → Architecture → Requirements). É material de apoio para o **posicionamento na
> abertura open-source** — o que já existe lá fora, onde o SeedGrid é diferente, e quem
> é referência de UX. Levantado em **jul/2026**, revisado em **set/2026** (revalidar antes
> de publicar).

## TL;DR

O SeedGrid vive em **dois eixos**, e a resposta é diferente em cada um.

**Eixo de orquestração — o nicho é vazio.** Nenhum projeto **mantido** faz o que o SeedGrid
faz por inteiro. O espaço se divide em (1) **UIs de gestão multi-instância** de qBittorrent
— operam vários clientes, mas **não orquestram**; (2) o **gêmeo arquitetural**
(OpenSeedbox) — a mesma ideia de cérebro central + nós "burros" com colocação, porém
**abandonado** e com o scheduler **nunca implementado**; (3) clientes remotos single-node.
O SeedGrid ocupa o nicho vazio: uma **plataforma de orquestração** (Control Plane + Agents
+ Scheduler) que trata os nós como **um cluster** e realmente decide (colocação, admissão,
migração, publicação).

**Eixo de publicação — o nicho é disputado, e nós estamos atrás.** O levantamento de
jul/2026 não olhou para esse lado. Existem duas ferramentas maduras de preparação e envio
de release (§4), uma delas em Go, ativa, e que **já publica no mesmo tracker que o
SeedGrid** — território que a ADR-020 assumiu sem comparar. Elas não orquestram nada, mas
a lane de publicação delas é anos mais madura que a nossa.

## 1. UIs de gestão multi-instância (não orquestram)

| Projeto | Stack | Ativo | O que faz | Por que NÃO é o SeedGrid |
|---|---|---|---|---|
| **autobrr/qui** | Go + TS, single-binary | **Sim** (v1.22.0, jun/2026) | Painel para gerenciar N instâncias qBittorrent; cross-seed; automações por regra; backups | Trata cada instância como **entidade separada** (você alterna entre elas). Sem colocação automática, sem balanceamento, sem teto cluster-wide, sem agentes decidindo. É **dashboard**, não orquestrador. |
| **qBitWebUI** (mkbula) | Web moderno | Sim | UI multi-instância, cross-seed, integração Prowlarr, monitor em tempo real | Igual: gestão/visualização, não orquestração. |
| **jesec/flood** | Node/React | Sim | Front-end para um cliente (multi-instância limitado, em discussão) | Front-end de cliente, não decide colocação. |
| **StuffAnThings/qbit_manage** | Python | Sim | Manutenção (tags, limpeza, cross-seed) via cron | Ferramenta de manutenção, single/multi instância; não coloca torrents entre nós. |

**Referência de UX:** o **qui** é bonito, rápido (single-binary Go, como o nosso CP) e
ativo — o **concorrente/benchmark mais direto** do lado "operar vários qBittorrent". Vale
acompanhar features e polish dele quando formos posicionar o SeedGrid.

## 2. Gêmeo arquitetural — OpenSeedbox (abandonado)

- **Mesma ideia central:** frontend guarda **todo o estado** + nós backend "burros" que só
  baixam, com **colocação de torrents entre nós** (`openseedbox/openseedbox`).
- **Mas:** Java/**Play Framework** + **transmission-daemon** (não qBittorrent); orientado a
  **multi-usuário** (produto de hosting); **dormente/experimental** (README: "for
  developers"); e — o mais relevante — **"o algoritmo de escolha de nó nunca foi
  implementado"**. O *coração* (Scheduler) nunca saiu do papel.
- **Não tem:** admissão/teto por conta de tracker, migração entre nós, publicação/mediainfo,
  UNIT3D, health monitoring, config hot-reload.

## 3. Clientes remotos self-hosted (single-node, fora do escopo)

boypt/simple-torrent (ex-Cloud Torrent), jpillora/cloud-torrent, exatorrent, stacks docker
de seedbox (plcharriere/seedbox). São **um** cliente remoto com UI — não há noção de cluster.

## 4. Upload assistants — o eixo de publicação (levantado set/2026)

Categoria que o levantamento de jul/2026 **não cobriu**. São ferramentas que preparam e
enviam um release para tracker privado: análise de mídia, metadados, screenshots,
descrição, dupe check, criação do `.torrent` e POST. É exatamente a lane das ADR-019 /
ADR-020 / ADR-026 / ADR-030.

| Projeto | Stack | Ativo | O que faz | Relação com o SeedGrid |
|---|---|---|---|---|
| **wastaken7/Upload-Assistant** (fork do Audionut) | Python | **Sim** | **105 trackers** + 3 indexers Usenet. ffmpeg/MediaInfo/BDInfo/DVD, TMDB/IMDB/TVDB/MusicBrainz/Discogs/IGDB, screenshots + espectrogramas, description builder de **17 blocos** com override por tracker, filas em lote, CLI + Web UI + Docker | Cobre livros, música, jogos e Usenet — fora do nosso caso de uso. Mas o **description builder** e a **coordenação com a banda do qBittorrent** são diretamente aplicáveis |
| **autobrr/upbrr** | **Go** | **Sim** (alpha, push diário) | Fluxo de **8 estágios** com conferência humana em cada um; **perfil por site** (`profile.go` / `taxonomy.go` / `rules.go` / `validation.go` / `auth.go` / `description.go`); dupe check como *evidência*, não como bloqueio; preview congelado do payload; injeção no cliente; migração de config vinda do Upload-Assistant | **Da mesma org do `qui`** (§1). E **já publica no samaritano**: `internal/trackers/impl/unit3d/sites/sam/profile.go` declara `Name: "SAM", BaseURL: "https://samaritano.cc"` |

**Nenhum dos dois orquestra nada.** São ferramentas single-host que preparam um release: não
há Node, cluster, colocação, admissão nem migração. A sobreposição com o SeedGrid é
estritamente a lane de publicação — mas ali a diferença é grande e é a nosso desfavor.

### Onde estamos atrás (set/2026)

| | SeedGrid hoje | UA / upbrr |
|---|---|---|
| dupe check no tracker | **não existe** (`unit3d.Browse` está pronta, só o Explore usa) | estágio próprio, resultado tratado como evidência |
| regras de elegibilidade | **nenhuma** — valida nome/descrição/categoria/tipo e dispara | `rules.go` + `validation.go` por site, banned groups |
| perfil de tracker | espalhado: taxonomia **no frontend**, formato de nome no pacote genérico, shape da API fixo | manifesto por site; site novo = ~25 linhas |
| description builder | 82 linhas de TS **no Portal** | 17 blocos (UA) / pacote por site (upbrr), server-side e testável |
| metadados | TMDB + IGDB (`tvdb` é enviado sempre zerado) | IMDB, TMDB, TVDB, TVMaze, TheXEM, BluRay.com |
| image host | um, fixo | política com fallback; o perfil do SAM declara que **o próprio site hospeda** |
| preview do envio | não há — "sem dry-run, POST real é o único teste" | operação **congelada**: o preview e o submit usam o mesmo estado |

Plano de correção e sequenciamento: issue **#184** (épico) e as filhas #185–#195.

### A decisão estratégica — resolvida na ADR-037

A ADR-020 construiu integração UNIT3D própria sem comparar com o que já existia. Com o
upbrr no mapa a escolha virou real, e as saídas eram duas:

- **A — publicação em casa.** O SeedGrid publica sozinho. Custo: o conhecimento de cada
  site precisa entrar no sistema de algum jeito. Ganho: publicação é **parte** da
  orquestração — o `.torrent` nasce, é semeado e é publicado no mesmo fluxo, com o
  Scheduler escolhendo o Node.
- **B — delegar.** O SeedGrid orquestra, o upbrr publica. Ganho: 100+ trackers sem manter
  nenhum. Custo: dependência externa em alpha, e o release deixa de nascer semeado no
  cluster — some a integração da ADR-019/020, validada em produção.

**Adotada a A**, na **ADR-037**. O que decidiu não foi a contagem de trackers: foi que a
publicação é um passo da orquestração, não um passo ao lado dela.

O custo da A — "e o conhecimento de cada site, de onde vem?" — é o que a ADR-037 responde,
e a resposta é o que torna a A viável num projeto open-source: o perfil do tracker é
**dado contribuível**, não código do mantenedor. Quem tem conta no site descobre a
taxonomia a partir do catálogo do próprio tracker (a API do UNIT3D devolve nome e id lado
a lado), ajusta, exporta e compartilha. O mantenedor nunca precisa ter conta em lugar
nenhum — que é exatamente o teto que o modelo do upbrr, com um pacote Go por site, impõe
a si mesmo.

## Diferenciação do SeedGrid (o nicho vazio)

> Vale para o **eixo de orquestração**. No eixo de publicação (§4) a diferenciação é
> menor e o SeedGrid é o entrante, não o incumbente.

Os *blocos* (gerenciar vários qBittorrent, seedbox distribuída) existem como **produtos de
gestão/UI**. O SeedGrid é uma **plataforma de orquestração** com combinação que nenhum
projeto ativo entrega:

- **Control Plane + Agents** (ADR-012, agent-iniciado por polling) e um **Scheduler** que
  decide colocação — o que o OpenSeedbox prometeu e não fez.
- **Admissão cluster-wide** amarrada ao limite de **DL slots da conta** do tracker (ADR-021),
  peer-aware, com rotação por lento/morto — não encontrado em nenhum outro projeto.
- **Migração de torrents entre nós** (ADR-018), **publicação/criação** com mediainfo +
  **integração UNIT3D** (ADR-019/020), **config hot-reload** (ADR-023), **modo Manual/Auto**
  (ADR-024), **Feed de orquestração** (ADR-022), **Auditoria** (ADR-025), health/observabilidade.

## Implicações para a abertura open-source

- **Posicionamento:** não competir como "mais uma UI de qBittorrent" (o qui já faz isso bem),
  e sim como **"Kubernetes de seeding"** / orquestrador de cluster BitTorrent — o Scheduler +
  admissão cluster-wide + migração são o pitch.
- **Risco:** o qui é ativo e pode crescer em direção à orquestração; acompanhar. A mesma org
  lançou o **upbrr** (§4) — a autobrr está ocupando os dois eixos vizinhos ao nosso.
- **Oportunidade:** o único que mirou orquestração (OpenSeedbox) está morto → há espaço e uma
  base de usuários órfã (seedbox distribuída com colocação).
- **Não vender publicação como diferencial.** Ela é parte do produto, mas comparada ao UA e
  ao upbrr a nossa lane de publicação é rasa (§4). O pitch é orquestração.
- Revalidar este levantamento antes de publicar (o cenário muda rápido).

## Fontes

**jul/2026:**

- autobrr/qui — https://github.com/autobrr/qui
- qBitWebUI — https://github.com/Maciejonos/qbitwebui
- qbit_manage — https://github.com/StuffAnThings/qbit_manage
- flood — https://github.com/jesec/flood
- OpenSeedbox — https://github.com/openseedbox/openseedbox · https://www.openseedbox.com/
- simple-torrent — https://github.com/boypt/simple-torrent · cloud-torrent — https://github.com/jpillora/cloud-torrent

**set/2026 (eixo de publicação):**

- wastaken7/Upload-Assistant — https://github.com/wastaken7/Upload-Assistant
- autobrr/upbrr — https://github.com/autobrr/upbrr
  (perfil do samaritano: `internal/trackers/impl/unit3d/sites/sam/profile.go`;
  modelo de perfil: `ADDING_TRACKERS.md`; fluxo de 8 estágios:
  `documentation/docs/workflow/index.md`)
