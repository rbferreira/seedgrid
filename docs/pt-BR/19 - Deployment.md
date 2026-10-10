# **SeedGrid**
**Deployment Specification**
Version: 1.0
Status: Draft
Document: 19 - Deployment.md
# **1. Objetivo**
Este documento define o processo oficial de implantação do SeedGrid.
Seu objetivo é padronizar a instalação da plataforma, reduzir diferenças entre ambientes e garantir que todos os componentes sejam implantados de forma consistente.
# **2. Filosofia**
A implantação deverá ser:
Simples.
Reproduzível.
Documentada.
Automatizável.
A arquitetura deverá permitir que um novo ambiente seja criado seguindo apenas este documento.
# **3. Escopo**
Este documento contempla:
Preparação do ambiente.
Instalação do Control Plane.
Instalação dos Seed Nodes.
Registro dos Nodes.
Validação da plataforma.
Atualizações.
Rollback.
# **4. Pré-requisitos**
Infraestrutura mínima.
Control Plane disponível.
Rede operacional.
Docker disponível.
Sistema Operacional suportado.
Conectividade IP entre todos os componentes.
# **5. Componentes**
Serão implantados (V1):
Control Plane (API + serviços internos: Scheduler, Alert Worker, Inventory/Monitoring).
Portal (SPA React servido por nginx, que também faz proxy de `/api/` e `/healthz` para o Control Plane).
PostgreSQL 17.
Prometheus.
Alertmanager.
node-exporter (um por host).
SeedGrid Agent.
Torrent Engine (qBittorrent).

> **ADR-016:** o **Grafana foi removido** — a visualização de métricas passou a viver no Portal (Control Plane consulta o Prometheus via PromQL e serve as séries em JSON; fluxo Portal → API → Prometheus). **Loki (logs) fica diferido** (gatilho de reavaliação na ADR-016), assim como **Traefik/TLS**: na V1 não há reverse proxy dedicado nem terminação TLS — o Portal serve em `:80` direto. As referências a Grafana/Loki/Traefik/certificados neste documento são de arquitetura de referência, **não** componentes ativos da V1.
# **6. Organização**
O repositório é clonado **no host do Control Plane** — onde o operador preferir; os exemplos usam `/opt/seedgrid` — e é a única fonte de artefatos de deploy. Os Seed Nodes não clonam nada: recebem o que precisam do Control Plane (§50). Estrutura relevante:
```
/opt/seedgrid
├── control-plane/       # serviço Go (cmd, internal, migrations/, Dockerfile*)
├── agent/               # serviço Go (cmd, internal, engine adapters)
├── portal/              # SPA React (Vite)
├── deploy/
│   ├── control-plane/   # docker-compose.yml + .env.example (Postgres + Controller)
│   ├── portal/          # docker-compose.yml + Dockerfile + nginx.conf
│   ├── observability/   # docker-compose.yml + prometheus.yml + rules.yml + alertmanager.yml
│   └── agent/           # docker-compose.yml (+ .env.example), seedgrid-agent.service, seedgrid-agent.env.example
└── scripts/             # automação: build-*.sh, bootstrap-controller.sh, bootstrap-node.sh, acceptance.sh
```
Config sempre externa aos containers, via arquivos `.env` gerados a partir dos `.env.example` (ver §16). Não há diretórios `config/`, `logs/` ou `backups/` versionados: dados persistentes vivem em volumes Docker nomeados (Postgres, Prometheus, Alertmanager) e o estado local do Agent em `StateDirectory`/`SEEDGRID_AGENT_DATA_DIR`.
# **7. Preparação do Control Plane**
Etapas mínimas:
Instalar Sistema Operacional.
Atualizar pacotes.
Configurar rede.
Configurar horário.
Instalar Docker.
Instalar Docker Compose.
Criar diretórios.
Preparar volumes.
# **8. Implantação do Control Plane**
Fluxo simplificado.
Sistema Operacional
↓
Docker
↓
Volumes
↓
Banco
↓
Control Plane
↓
Portal
↓
Monitoramento
Cada etapa deverá ser validada antes da próxima.
# **9. Preparação dos Seed Nodes**
Cada Node deverá seguir o mesmo procedimento.
Sistema Operacional.
Rede.
Docker.
Torrent Engine.
SeedGrid Agent.
Exporters.
Armazenamento.
# **10. Registro**
Após instalação do Agent.
Fluxo.
Node
↓
Agent
↓
Control Plane
↓
Registro
↓
Inventário
↓
Heartbeat
↓
Operação
# **11. Primeira Inicialização**
Após implantação deverão ser verificadas:
Portal.
API.
Banco.
Scheduler.
Monitoramento.
Alertas.
Agents.
Heartbeats.
Inventário.
Todos os componentes deverão estar operacionais antes da liberação da plataforma.
# **12. Expansão**
A adição de novos Nodes seguirá exatamente o mesmo processo.
Preparar equipamento.
Instalar Agent.
Registrar.
Validar.
O Scheduler poderá utilizá-lo imediatamente após o registro.
# **13. Padronização**
Todos os ambientes deverão utilizar:
Mesma estrutura de diretórios.
Mesmas convenções de nomes.
Mesmas políticas.
Mesmos mecanismos de configuração.
Essa padronização reduz erros operacionais e simplifica manutenção.
# **14. Objetivos**
A implantação deverá ser repetível.
Qualquer operador deverá conseguir reconstruir completamente um ambiente utilizando apenas esta documentação.
# **15. Princípio Fundamental**
A implantação do SeedGrid deverá privilegiar automação e simplicidade.
Sempre que possível, procedimentos manuais deverão ser substituídos por mecanismos reproduzíveis e documentados.
# **16. Configuração**
Toda configuração da plataforma deverá ser externa aos containers.
Exemplos:
- arquivos `.env` (a partir dos `.env.example` — ver §45);
- arquivos YAML (Prometheus/Alertmanager);
- configuração do Portal (nginx);
- configuração do Control Plane;
- configuração do Monitoramento.
A substituição de um container nunca deverá implicar perda de configuração.
# **17. Gerenciamento de Containers**
Cada componente da plataforma deverá possuir seu próprio container.
Containers da V1 (ADR-016):
- Portal (nginx servindo o SPA + proxy da API);
- Control Plane (`seedgrid-controller`);
- PostgreSQL (`seedgrid-postgres`);
- Prometheus (`seedgrid-prometheus`);
- Alertmanager (`seedgrid-alertmanager`);
- node-exporter (um por host);
- Torrent Engine (`seedgrid-engine`, no Seed Node — nome neutro de motor; o Agent o procura por esse nome, ver `SEEDGRID_ENGINE_CONTAINER` em §45).

O SeedGrid Agent **não** roda em container: é um binário estático executado pelo systemd no host do Seed Node. Não há Grafana, Loki nem reverse proxy dedicado na V1 (o nginx do Portal cumpre o papel de front HTTP). Essa separação favorece manutenção, atualização e isolamento de falhas.
# **18. Persistência**
Todos os dados persistentes deverão utilizar volumes dedicados.
Volumes nomeados da V1:
- `seedgrid-postgres-data` — Estado oficial do Cluster (PostgreSQL);
- `seedgrid-prometheus-data` — TSDB do Prometheus (retenção 30d);
- `seedgrid-alertmanager-data` — estado do Alertmanager.

O estado local do Agent fica no host, em `SEEDGRID_AGENT_DATA_DIR`/`StateDirectory` (identidade após o primeiro registro). Os dados dos torrents ficam em `/data` no Seed Node. Os volumes deverão permanecer independentes do ciclo de vida dos containers.
# **19. Atualização**
As atualizações deverão seguir um procedimento padronizado.
Fluxo recomendado:
Backup.
↓
Validação do backup.
↓
Atualização dos containers.
↓
Inicialização.
↓
Validação da plataforma.
↓
Liberação para operação.
Caso alguma etapa falhe, deverá ser possível executar rollback.
# **20. Rollback**
Toda atualização deverá possuir procedimento documentado de retorno.
O rollback deverá preservar:
- Estado do Cluster;
- configurações;
- inventário;
- eventos;
- auditoria.
Sempre que possível, deverá ser executado utilizando os mesmos mecanismos previstos para a implantação.
# **21. Inclusão de um Novo Node**
Procedimento resumido:
Preparar hardware.
Instalar Sistema Operacional.
Instalar Docker.
Configurar armazenamento.
Instalar Torrent Engine.
Instalar SeedGrid Agent.
Registrar Node.
Validar inventário.
Validar Heartbeat.
Liberar para produção.
O procedimento deverá ser idêntico para qualquer quantidade de Nodes.
# **22. Substituição de um Node**
A substituição de um Node deverá seguir os mesmos passos da implantação inicial.
Caso existam backups dos torrents, estes poderão ser restaurados antes da liberação do Node.
A arquitetura não deverá exigir procedimentos especiais para substituição de hardware.
# **23. Expansão do Cluster**
O crescimento do Cluster ocorrerá apenas pela adição de novos Nodes.
Não será necessária alteração estrutural do Control Plane.
Após o registro, o Scheduler poderá utilizar imediatamente os novos recursos disponíveis.
# **24. Validação**
Após qualquer implantação deverão ser verificados, no mínimo:
- Portal acessível;
- API operacional;
- Banco de Dados íntegro;
- Scheduler ativo;
- Prometheus coletando métricas;
- Alertmanager operacional (webhook chegando ao Control Plane);
- Agents conectados;
- Heartbeats atualizados.

Essa checagem é automatizada pelo `scripts/acceptance.sh` (ver §44).
# **25. Checklist de Produção**
Antes da entrada em produção deverão ser confirmados:
- backups configurados;
- monitoramento operacional (Prometheus + Alertmanager);
- alertas configurados (regras em `deploy/observability/rules.yml`);
- credenciais padrão alteradas (admin/admin → ver §46);
- sincronização de horário;
- firewall configurado (incluindo os port-forwards BitTorrent por Node — ver §47);
- documentação atualizada;
- testes básicos executados (`scripts/acceptance.sh`).

> Na V1 não há TLS/certificados a validar (sem reverse proxy dedicado; ADR-016). Terminação TLS entra em fase posterior.
# **26. Ambientes**
A arquitetura deverá suportar diferentes ambientes utilizando o mesmo processo de implantação.
Exemplos:
Homelab.
Laboratório.
Desenvolvimento.
Produção.
As diferenças deverão limitar-se às configurações externas, preservando a mesma arquitetura e organização dos componentes.
# **27. Princípios**
Toda implantação deverá ser:
- reproduzível;
- documentada;
- automatizável;
- validável;
- reversível.
Esses princípios garantem previsibilidade e reduzem riscos operacionais durante todo o ciclo de vida da plataforma.
# **28. Documentação da Implantação**
Toda implantação deverá produzir documentação mínima do ambiente.
Informações recomendadas:
- nome do Cluster;
- versão do SeedGrid;
- data da implantação;
- responsáveis;
- quantidade de Nodes;
- inventário da infraestrutura;
- políticas de backup;
- políticas de monitoramento.
Essa documentação facilitará manutenção, auditoria e futuras expansões.
# **29. Versionamento**
Todos os componentes da plataforma deverão possuir versionamento explícito.
Exemplos:
- versão do SeedGrid;
- versão do Agent;
- versão do Control Plane;
- versão do Portal;
- versão da API;
- versão do Scheduler.
O Portal deverá exibir claramente a versão em execução.

**Esquema de versão dos builds (ADR-024).** A versão dos binários vem das **tags git** do repositório, não de um valor fixo. O helper `scripts/version.sh` deriva o valor com `git describe --tags`, removendo o hash do commit (a "mescla" escolhida): na tag exata → `v1.0.0`; N commits após a tag → `v1.0.0-N`; sem tag/git → `dev` (fallback). Esse valor é injetado no binário em build time via `-ldflags "-X main.version=…"` (Control Plane e Agent), consumido pelos `build-*.sh`. O Agent reporta a versão como `agent_version` no registro/heartbeat (coluna "Agent" no Dashboard); o Control Plane expõe a sua na API. Para publicar uma versão nova, criar a tag (`git tag vX.Y.Z`) e rebuildar.
# **30. Compatibilidade**
O Control Plane deverá validar a compatibilidade dos Agents conectados.
Quando existir incompatibilidade:
- o operador deverá ser informado;
- detalhes da incompatibilidade deverão ser apresentados;
- o comportamento esperado deverá ser documentado.
A plataforma deverá privilegiar compatibilidade entre versões consecutivas, sempre que possível.
# **31. Upgrade**
O processo de atualização deverá seguir princípios de baixo risco.
Objetivos:
- preservar configurações;
- preservar Estado do Cluster;
- minimizar indisponibilidade;
- permitir rollback.
Atualizações deverão ser documentadas e versionadas.
# **32. Pós-Implantação**
Após a conclusão da implantação deverão ser realizadas validações operacionais.
Itens sugeridos:
- login no Portal;
- registro de um Node;
- recebimento de Heartbeat;
- coleta de métricas;
- geração de um evento de teste;
- consulta aos logs;
- criação de um alerta de teste;
- validação do Scheduler.
Esses testes representam a aceitação mínima do ambiente.
# **33. Auditoria**
Implantações, atualizações e rollbacks deverão gerar registros de auditoria.
Exemplos:
- data;
- operador;
- versão instalada;
- resultado;
- duração;
- observações.
Esses registros auxiliam rastreabilidade e governança.
# **34. Escalabilidade**
A arquitetura deverá permitir crescimento gradual da plataforma.
A inclusão de novos Nodes não deverá exigir mudanças no processo de implantação do Control Plane.
Da mesma forma, a atualização do Control Plane não deverá exigir reinstalação dos Nodes.
# **35. Segurança**
Antes da liberação do ambiente deverão ser verificadas:
- credenciais padrão alteradas (o operador `admin/admin` inicial: o login retorna `default_credentials: true` e o Portal exibe aviso até a troca em `POST /api/v1/auth/password`);
- `SEEDGRID_AGENT_REGISTRATION_TOKEN` forte e único (protege o registro de novos Agents);
- acesso administrativo restrito;
- portas não utilizadas desabilitadas;
- firewall operacional (Postgres exposto só em `127.0.0.1`; port-forwards BitTorrent apenas para os IPs dos Nodes);
- backups configurados.
A implantação somente deverá ser considerada concluída após essas validações.

> TLS/certificados não fazem parte da V1 (ADR-016: sem Traefik/reverse proxy dedicado). Quando a terminação TLS for adotada, exigirá ADR e atualização deste documento.
# **36. Automação**
Todos os procedimentos descritos neste documento deverão ser passíveis de automação.
A documentação representa o processo oficial; ferramentas futuras poderão automatizar parcial ou totalmente sua execução, preservando os mesmos princípios e resultados.
# **37. Evolução**
O processo de implantação deverá evoluir juntamente com a plataforma.
Novos componentes deverão incluir instruções de implantação, atualização e remoção antes de serem considerados parte oficial do SeedGrid.
# **38. Resumo da Arquitetura**
Operador
│
▼
Preparação da Infraestrutura
│
┌─────────────┴─────────────┐
▼                           ▼
Control Plane                Seed Nodes
│                           │
▼                           ▼
Validação Inicial          Registro dos Agents
└─────────────┬─────────────┘
▼
Cluster Operacional
A implantação deverá resultar em um ambiente consistente, reproduzível e pronto para expansão.
# **39. Boas Práticas**
Toda implantação deverá observar os seguintes princípios:
- utilizar configurações versionadas;
- manter infraestrutura documentada;
- validar todos os componentes antes da produção;
- automatizar tarefas repetitivas;
- testar procedimentos de atualização;
- manter procedimentos de rollback atualizados.
# **40. Declaração Final**
A estratégia de implantação do SeedGrid foi projetada para oferecer previsibilidade, simplicidade e reprodutibilidade.
Ao separar claramente os componentes do Control Plane dos Seed Nodes e manter configurações externas aos containers, a plataforma favorece automação, facilita manutenção e reduz o risco operacional durante todo o seu ciclo de vida.

# **41. Procedimento real (V1) — topologia de referência**
Uma instalação tem **um host para o Control Plane** e **um host por Seed Node**. Qualquer máquina Linux com Docker serve — servidor, VM, mini PC, Raspberry Pi (o Agent e o Engine rodam em `amd64` e `arm64`).

| Host (exemplo) | Papel | O que roda |
|---|---|---|
| `controller` · `192.168.0.10` | Control Plane | `seedgrid-controller`, `seedgrid-postgres`, `seedgrid-prometheus`, `seedgrid-alertmanager`, `seedgrid-node-exporter`, `seedgrid-portal` (nginx `:80`) |
| `node-01` · `192.168.0.11` | Seed Node 1 | `seedgrid-engine` (BitTorrent 6881), `seedgrid-node-exporter`, Agent (systemd) |
| `node-02` · `192.168.0.12` | Seed Node 2 | `seedgrid-engine` (BitTorrent 6882), `seedgrid-node-exporter`, Agent (systemd) |

Os endereços são ilustrativos. O Control Plane e os Nodes só precisam se alcançar por HTTP: cada Node fala com a porta `80` do Control Plane (Agent → API, pelo nginx do Portal), e o Prometheus do Control Plane coleta a `9100` de cada Node. Nada mais é exposto entre eles.

**Control Plane e Node ficam em hosts diferentes.** O Engine e o node-exporter do Node usam a rede do host e ocupam as portas `8080` e `9100` — as mesmas que o Control Plane usa no host dele.

# **42. Fluxo de build**
Não há imagem publicada num registry: as imagens são montadas no próprio host do Control Plane, pelos scripts `build-*.sh`, que o `bootstrap-controller.sh` chama. Cada um compila o artefato num container efêmero (`docker run`) e monta a imagem final com um Dockerfile **sem etapas `RUN`**:
- `build-controller.sh` — compila o binário Go em `golang:1.26-alpine` (`CGO_ENABLED=0`) e monta `seedgrid/controller:dev` (`Dockerfile.runtime`).
- `build-portal.sh` — roda `npm ci && npm run build` em `node:22-alpine` e monta `seedgrid/portal:dev` (nginx), já com os artefatos de Node em `/node/` (§50).
- `build-agent.sh` — compila o binário estático do Agent (`agent/bin/seedgrid-agent`); `GOARCH=arm64` cross-compila para ARM.

Os três derivam a versão do binário via `scripts/version.sh` (tags git — ver §29); sobrepor com `VERSION=…` no ambiente força um valor. `build-portal.sh` reusa `build-agent.sh` para os binários servidos aos Nodes (um por arquitetura), herdando a mesma versão.

O Dockerfile sem `RUN` existe para a imagem montar também onde `docker build` com `RUN` não funciona — tipicamente Docker rodando dentro de um container de sistema, em que o profile AppArmor não se aplica ao build. Nesses ambientes pode ser preciso passar opções extras ao `docker run` dos scripts: exporte `SEEDGRID_DOCKER_RUN_OPTS` (ex.: `--security-opt apparmor=unconfined`) antes de rodar. Num host Docker comum não é preciso nada.

Quem prefere o caminho padrão do Docker pode usar o Dockerfile multi-stage do controller (`docker compose build` em `deploy/control-plane`); para injetar a versão real nesse caminho, exporte `VERSION=$(scripts/version.sh)` antes do build (o `docker-compose.yml` repassa como build-arg).

# **43. Instalação e atualização**
**Pré-requisitos:** Docker com o plugin Compose e `git` no host do Control Plane; nos Nodes, um Linux com `curl` e acesso `root` (o onboarding instala o Docker se faltar).

**Control Plane — do zero**, a partir do clone do repositório, como `root` ou com `sudo`:
```sh
git clone <url-do-repositório> /opt/seedgrid && cd /opt/seedgrid
sh scripts/bootstrap-controller.sh
#   cria o deploy/control-plane/.env e gera os segredos (§46); compila o controller,
#   sobe Postgres+Controller, Observabilidade e Portal; no fim mostra a URL do Portal
#   e o login do admin.
```
Não há nada para editar antes. Quem prefere escolher os valores cria o `.env` a partir do
`.env.example` antes de rodar: o que já estiver preenchido é respeitado (§45). Só o Portal
(`:80`) fica exposto na rede — a API é servida por ele em `/api`, e as portas `8080` (API) e
`9090` (Prometheus) escutam apenas em `127.0.0.1` no host.

`bootstrap-controller.sh` orquestra os três composes (`deploy/control-plane`, `deploy/observability`, `deploy/portal`).

**Seed Node — do zero:** um comando, executado no Node como `root`, sem clonar o repositório (onboarding
zero-touch, §50). O Portal o mostra pronto, com o token, em **Saúde & Métricas → Adicionar Node**:
```sh
curl -fsSL http://<control-plane>/node/bootstrap-node.sh | sudo CP_URL=http://<control-plane> JOIN_TOKEN=<token> sh
```
O `sudo` é para quem não está logado como `root`; sem privilégio o script para na primeira linha e diz como rodar.

**Atualização do Control Plane:** `git pull && git fetch --tags && sh scripts/bootstrap-controller.sh` — o mesmo script da instalação. Ele reconstrói as imagens e recria só o que mudou; o `.env`, os segredos e os volumes (banco, métricas) são preservados. O `fetch --tags` garante que a versão exibida acompanhe a tag nova.

**Atualização dos Nodes:** pelo Portal — o Control Plane publica o binário novo do Agent e oferece o self-update por Node (ADR-027). Para o que não é o binário (rede, storage, compose do Engine), ver §50.4.

> **Migrations aplicadas no boot.** Não há script `migrate` separado: o controller aplica as migrations goose embutidas (`control-plane/migrations/`, embed via `embed.go`) durante a inicialização. Subir uma versão nova do controller aplica automaticamente as migrations pendentes.

# **44. Validação (aceitação fim-a-fim)**
`scripts/acceptance.sh` roda contra um ambiente já de pé (no host do Control Plane) e valida o happy path, saindo `!= 0` se algo falhar (22 verificações). Cobre: `healthz`; `/metrics` expondo `seedgrid_*`; login do operador (token) + flag `default_credentials`; Nodes `ONLINE` com `health_score`; recursos de host (disco/mem) reportados; `bt_port` único por Node; **métricas servidas pelo CP a partir do Prometheus** (`/api/v1/metrics/query`); `system/health` (API + Banco); `GET /events` e `GET /alerts`; fluxo do Scheduler (adicionar torrent → Node escolhido) e o ciclo completo do torrent (**pausar/retomar/série temporal/remover**); `samples` de torrent inexistente → `404`; troca de senha rejeitando senha fraca (`400`); **Prometheus** raspando targets (`up`); e o **Portal** servindo (`HTTP 200`) com proxy de `/healthz`. Lê credenciais de `deploy/control-plane/.env` automaticamente (variáveis `API`/`PORTAL`/`PROM` sobrescrevem os defaults).

# **45. Variáveis de ambiente**
Configuração sempre externa, via `.env` gerados dos `.env.example`.

**Control Plane** (`deploy/control-plane/.env` → container `seedgrid-controller`; ver `control-plane/internal/config/config.go`):

| Variável | Default | Obrigatória | Descrição |
|---|---|---|---|
| `SEEDGRID_DATABASE_URL` | — | **sim** | DSN do PostgreSQL (montada no compose a partir de `POSTGRES_*`). |
| `SEEDGRID_HTTP_ADDR` | `:8080` | não | Endereço de escuta da API. |
| `SEEDGRID_LOG_LEVEL` | `info` | não | Nível de log. |
| `SEEDGRID_AGENT_REGISTRATION_TOKEN` | — | **sim** | Token exigido no registro inicial de um Agent. |
| `SEEDGRID_ADMIN_USERNAME` | `admin` | não | Operador administrador inicial. |
| `SEEDGRID_ADMIN_PASSWORD` | — | **sim** | Senha inicial do admin (criado no 1º boot se não existir). |
| `SEEDGRID_ALERT_WORKER_INTERVAL` | `30s` | não | Frequência de avaliação do Alert Worker. |
| `SEEDGRID_NODE_OFFLINE_AFTER` | `90s` | não | Heartbeat vencido além disso ⇒ Node `OFFLINE`. |
| `SEEDGRID_DISK_IO_SATURATED_PCT` | `90` | não | Utilização (busy%) do disco ≥ isso ⇒ alerta `DISK_IO_SATURATED`, desde que se sustente por `alerting.disk_saturated_for` (default `90s`, #150 — ver `25 - Business Rules.md`); resolve com histerese de 15pp. `0` desliga. |
| `SEEDGRID_SAMPLE_RETENTION` | `168h` | não | Retenção do histórico por torrent (7 dias). |
| `SEEDGRID_PROMETHEUS_URL` | `http://seedgrid-prometheus:9090` | não | Prometheus consultado pelo CP (endpoints de métricas). |

> No compose, `SEEDGRID_DATABASE_URL` é derivada de `POSTGRES_DB`/`POSTGRES_USER`/`POSTGRES_PASSWORD` (definidos no `.env`).

**Agent** (`/etc/seedgrid-agent.env` no Seed Node; ver `agent/internal/config/config.go`). Desde a **ADR-039** (#144) o `.env` guarda só **identidade e máquina** — URL do Control Plane, credenciais, portas, caminhos, nome do container do Engine. O **comportamento** (intervalos, nível de log, fila do Engine) vem do Control Plane e o env dele é só **fallback** (tabela seguinte).

| Variável | Default | Obrigatória | Descrição |
|---|---|---|---|
| `SEEDGRID_CONTROL_PLANE_URL` | — | **sim** | URL do Control Plane — a porta `80` do host, servida pelo nginx do Portal (ex.: `http://controller.example`). |
| `SEEDGRID_AGENT_REGISTRATION_TOKEN` | — | 1º registro | Token de registro (após o 1º registro, a identidade vive no `StateDirectory`). |
| `SEEDGRID_AGENT_DATA_DIR` | `/var/lib/seedgrid-agent` | não | Estado local do Agent. |
| `SEEDGRID_AGENT_ENGINE` | `qbittorrent` | não | Torrent Engine. |
| `SEEDGRID_ENGINE_QBT_URL` | `http://127.0.0.1:8080` | não | WebUI API do qBittorrent. |
| `SEEDGRID_ENGINE_QBT_USERNAME` | `admin` | não | Usuário da WebUI. |
| `SEEDGRID_ENGINE_QBT_PASSWORD` | — | recomendada | Senha da WebUI. |
| `SEEDGRID_AGENT_STORAGE_PATH` | `/data` | não | Caminho de dados dos torrents. |
| `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` | `true` | não | `/data` é um **disco dedicado**? `true`: o storage só é saudável se for *mountpoint* e gravável (protege contra o disco que não montou). `false`: Node de disco único — `/data` é uma pasta no disco do sistema e só a escrita é conferida. **Declarada pelo `bootstrap-node.sh` na instalação, nunca inferida pelo Agent**; ausente (ou valor inválido) vale `true`, que é o lado seguro e o que os Nodes anteriores à variável sempre tiveram. Ver `docs/06 - Storage.md` §35.3 e §50.1. |
| `SEEDGRID_ENGINE_BT_PORT` | `6881` | não | Porta BitTorrent deste Node (única no cluster — ver §47). |
| `SEEDGRID_ENGINE_CONTAINER` | `seedgrid-engine` | não | Nome do container do Torrent Engine — o Agent faz `docker inspect` nele para reportar o uptime do Engine (`engine_started_at`). Tem que ser o `container_name` do compose do Node; nome que não bate **não dá erro**, só deixa o campo vazio. |
| `SEEDGRID_AGENT_MIGRATION_PORT` | `6900` | não | Porta do servidor de transferência de migração (ADR-018). Mesma em todos os Nodes. |

**Comportamento do Agent e do Engine — fallback (ADR-039).** Estas variáveis **não são mais a fonte**: o Control Plane entrega o valor **efetivo** de cada Node (override do Node > valor de cluster, em Configurações → Agent / Engine) na resposta do heartbeat e o Agent o aplica a quente. O env só vale **quando o Control Plane não manda nada** — CP anterior à ADR-039, CP inalcançável, ou o Agent no boot antes do primeiro heartbeat — e o default do código quando nem o env existe. O bootstrap **não** as escreve.

| Variável | Default do código | Chave no catálogo (default de **cluster**) |
|---|---|---|
| `SEEDGRID_AGENT_HEARTBEAT_INTERVAL` | `30s` | `agent.heartbeat_interval` (`10s`) |
| `SEEDGRID_AGENT_COMMAND_POLL_INTERVAL` | `5s` | `agent.command_poll_interval` (`5s`) |
| `SEEDGRID_LOG_LEVEL` | `info` | `agent.log_level` (`info`) |
| `SEEDGRID_ENGINE_QUEUEING_ENABLED` | `true` | `engine.queueing_enabled` (`true`) |
| `SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS` | `2` | `engine.max_active_downloads` (`10`) |
| `SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS` | `10000` | `engine.max_active_uploads` (`10000`) |
| `SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS` | `10000` | `engine.max_active_torrents` (`10000`) |
| `SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING` | `1` | `engine.max_active_checking` (`1`) |
| `SEEDGRID_ENGINE_DONT_COUNT_SLOW` | `false` | `engine.dont_count_slow` (`false`) |
| `SEEDGRID_ENGINE_SLOW_DL_THRESHOLD` | `1` (KB/s) | `engine.slow_dl_threshold` (`1`) |
| `SEEDGRID_ENGINE_SLOW_UL_THRESHOLD` | `1` (KB/s) | `engine.slow_ul_threshold` (`1`) |
| `SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER` | `300` (s) | `engine.slow_inactive_timer` (`300`) |

O default do **código** (`MAX_ACTIVE_DOWNLOADS=2`, `HEARTBEAT_INTERVAL=30s`) é deliberadamente o antigo: o fallback sem Control Plane é o comportamento de sempre. O default de **cluster** é o que a maioria da frota usava no `.env`. Uma consequência prática: **sem o Control Plane no boot** (queda de energia que derruba tudo junto) o Node sobe com o valor do `.env` e passa ao do Control Plane quando este voltar — mantenha no `.env` um valor sensato para a máquina. Tabela completa, faixas e a semeadura dos overrides em `25 - Business Rules.md` §3.1.1b.

Além disso, o `.env` do compose do Engine no Node define **apenas** `SEEDGRID_ENGINE_BT_PORT` — o mesmo valor de `/etc/seedgrid-agent.env`. Os dois são gravados pelo `bootstrap-node.sh` com a porta que o Control Plane atribuiu (§47).

# **46. Credencial inicial e troca de senha**
Numa instalação nova o `bootstrap-controller.sh` **gera** os segredos que estiverem vazios ou com o valor de exemplo no `.env` — senha do banco, token de registro dos Agents, senha do admin e token dos webhooks internos — e mostra o login do admin ao terminar (a senha fica em `deploy/control-plane/.env`). Ele só gera quando o banco ainda não existe: com o volume do Postgres já criado, a senha do banco e o operador admin estão gravados lá, e o script apenas avisa se encontrar valor de exemplo.

O primeiro boot cria o operador definido em `SEEDGRID_ADMIN_USERNAME`/`SEEDGRID_ADMIN_PASSWORD`. Se alguém configurar `admin/admin` à mão, o login retorna `default_credentials: true` e o **Portal exibe um aviso** até que a senha seja trocada em **`POST /api/v1/auth/password`** (página **Perfil**). Trocar a senha é item obrigatório do checklist de produção (§25, §35).

# **47. Porta BitTorrent por Node e liberação de entrada (manual, agnóstico de firewall)**
Cada Seed Node precisa de uma **porta BitTorrent única** no cluster para receber conexões de entrada (essencial para semear e manter ratio). **O Control Plane atribui a porta** no onboarding — a menor livre a partir de `6881` — e o `bootstrap-node.sh` a grava nos dois lugares que precisam concordar no Node: `/etc/seedgrid-agent.env` (o Agent fixa o `listen_port` do qBittorrent nela e desliga UPnP) e o `.env` do compose do Engine. O operador não escolhe nem digita a porta.

O que **continua manual** é o firewall: o SeedGrid é agnóstico de roteador e não cria regra nenhuma. Conforme a rede do Node:

- **IPv4 público:** port-forward `WAN <porta> → <ip-do-node>:<porta>` (TCP+UDP).
- **Atrás de CGNAT (sem IPv4 público):** o port-forward é inócuo; libere **entrada IPv6** na porta do Node (TCP+UDP) para o endereço dele. O endereço IPv6 do Node segue a mesma sequência da porta: `<prefixo>::5eed:<porta − 6880>` (ADR-038, ver §50.2) — a regra do Node 1 aponta para `::5eed:1`, a do Node 4 para `::5eed:4` —, e não muda se o Node for recriado.

- **Descoberta pelo operador:** o Agent reporta a porta no heartbeat; o Control Plane expõe `bt_port` + `last_ip` em `GET /api/v1/nodes`, e o **Portal mostra "Endpoint BT" (IP:porta)** por Node (Dashboard e página Saúde). É onde o operador vê o que liberar; o `bootstrap-node.sh` também imprime a instrução ao terminar.
- **Origem do `last_ip` (proxy-aware):** o Agent chega ao CP pelo nginx (o CP escuta só em 127.0.0.1), então o CP usa o header `X-Real-IP` do proxy — que o nginx seta com o `$remote_addr` real do Node — para gravar o `last_ip` (fallback: `X-Forwarded-For`, depois `RemoteAddr`). Sem isto, todos os Nodes registrariam o IP do proxy (incidente #84).
- Sem entrada liberada o Node ainda baixa (saída/DHT), mas semeia mal.

# **48. Seed Node adicional**
Igual ao primeiro: o mesmo comando de onboarding (§43, §50), com um token novo. O Control Plane
atribui uma porta BitTorrent distinta a cada Node e passa a coletar as métricas dele sozinho
(ADR-017) — não há passo manual por Node nem binário a copiar entre máquinas.

# **49. Funcionalidades da V1 relevantes ao deploy/operação**
- **Adicionar torrents** por **magnet** e por **upload de arquivo `.torrent`** — inclusive **múltiplos `.torrent` de uma vez**; o Scheduler escolhe o Node.
- **Série temporal por torrent** (`GET /api/v1/torrents/{id}/samples`), com retenção controlada por `SEEDGRID_SAMPLE_RETENTION`.
- **Métricas de rede por Node** e rankings apresentados no Portal.
- **Duas fontes de alerta**, unificadas no Portal (ADR-016): (1) **Alert Worker** interno do CP (regra de domínio `NODE_OFFLINE`); (2) **Prometheus/Alertmanager** (regras em `deploy/observability/rules.yml`: `NodeExporterDown`, `DiskSpaceLow`, `HighMemory`), que enviam webhook para `POST /api/v1/integrations/alertmanager`.
- **Portal** com navegação por sidebar e URL por página: Dashboard, Adicionar, Alertas & Eventos, Saúde & Métricas (com a lista e o detalhe de cada Node), Perfil e detalhe de Torrent.

# **50. Onboarding de um Seed Node — o que o `bootstrap-node.sh` configura**
O onboarding zero-touch (ADR-017) é um comando, executado no Node novo:

```sh
curl -fsSL http://<cp>/node/bootstrap-node.sh | sudo CP_URL=http://<cp> JOIN_TOKEN=<token> sh
```

O script e tudo que ele instala são **servidos pelo Control Plane** em `/node/*` (compose do Engine, unit do Agent, scripts de storage, binário do Agent por arquitetura — ADR-027); o Node não clona o repositório. Esta seção lista o que o script deixa configurado e como atualizar um Node que já existia quando uma dessas coisas mudou.

## 50.1 Passos do bootstrap

| # | Passo | Resultado |
|---|---|---|
| 1 | Docker | Instalado se faltar. |
| 2 | **Rede (IPv6)** | SLAAC garantido (ADR-034) e privacy extensions desligadas; ver §50.2. |
| 3 | **Enroll** | O Control Plane valida o join token e atribui a porta BitTorrent (menor livre da sequência, ADR-017) **e o índice do Node** (`node_index`, ADR-038). |
| 4 | Artefatos | Compose do Engine, unit do Agent, scripts de storage e o binário do Agent da arquitetura do Node, baixados de `/node/`. O binário roda `--version` antes de a unit ser instalada. |
| 5 | **Storage** | `/data` montado por UUID e auto-recuperação de hotplug (camadas 1 e 5 de `docs/06 - Storage.md` §35); o resultado vira a declaração `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` no env do Agent (passo 8). **Sem disco**, o Node sobe com `/data` no disco do sistema — uma configuração suportada: o script grava `false`, avisa no terminal e o Node trabalha normalmente (§50.1.1). |
| 6 | **Sufixo IPv6** | O endereço do Node fica `<prefixo /64>::5eed:<N>` (§50.2). Vem **antes** do Engine, para ele anunciar ao tracker o endereço definitivo desde o primeiro announce. |
| 7 | **Engine** | Container `seedgrid-engine` (§50.3), na porta BitTorrent atribuída; a senha da WebUI é gerada **no Node** e não sai dele. |
| 8 | Agent | `/etc/seedgrid-agent.env` (inclusive `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT`, do passo 5) e serviço systemd; o Agent se registra no Control Plane. O `.env` leva só **identidade e máquina**: o comportamento (intervalos, log, fila do Engine) vem do Control Plane (§45, ADR-039) — o Node novo **nasce igual ao cluster**. |

O mesmo script, com `--storage-only`, reexecuta só o passo 5 (e religa o Engine ao `/data` novo, e grava `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true`) num Node já registrado; com `--network-only`, os passos 2 e 6 (§50.2).

### 50.1.1 Storage no onboarding: disco dedicado ou disco do sistema
O script detecta o maior disco USB e monta o filesystem que ele já tem em `/data` (não-destrutivo), e **grava no env do Agent o que fez**:

| O que o script encontrou | `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` | Resultado |
|---|---|---|
| Disco montado/encontrado em `/data` | `true` | O Agent exige que `/data` seja *mountpoint* e gravável. |
| Nenhum disco (disco único: VM, mini PC, NAS) | `false` + **aviso no terminal** | `/data` fica no disco do sistema; o Agent confere só a escrita; o `NODE_DISK_FULL` vigia o espaço. O Node é elegível no Scheduler. |
| `--storage-only` com disco | `true` | O Node ganhou disco dedicado; o Agent passa a exigi-lo. |

Overrides do bootstrap, todos opcionais:

| Variável | Efeito |
|---|---|
| `STORAGE_DEVICE=/dev/sdX1` | Aponta a partição a montar em `/data`, em vez de procurar o maior disco USB. |
| `STORAGE_FORMAT=1` | Formata ext4 se a partição não tiver filesystem (**destrutivo**). |
| `STORAGE_REQUIRE_MOUNT=true` | Declara disco dedicado mesmo sem disco **agora**: o Node fica fora do Scheduler (`storage_ok=false`) até o disco montar. É o que se usa quando o disco chega depois da instalação. |
| `STORAGE_REQUIRE_MOUNT=false` | Declara disco único de propósito (não combina com `--storage-only`). |

Uma segunda passada do bootstrap **não rebaixa** um Node que já declarou disco: sem achar o disco, mas com o env anterior em `true` (ou `/data` no `fstab`), o valor continua `true` — o disco sumiu, o Node não virou "disco único". O raciocínio completo, e por que a exigência é declarada em vez de inferida, está em `docs/06 - Storage.md` §35.3.

## 50.2 Rede: um único endereço IPv6, estável
O Node precisa **anunciar** ao tracker, e ser **alcançado** em, um único endereço IPv6: é esse o endereço que o operador libera no firewall dele (ADR-034). O bootstrap cuida do que, sem ele, torna o endereço imprevisível. A interface considerada é a da rota default (IPv6 primeiro, IPv4 como alternativa); `NODE_IFACE=<interface>` a fixa à mão.

- **SLAAC garantido (ADR-034).** Se o gerenciador de rede do Node desliga o Router Advertisement (o caso de um container de sistema cujo hipervisor gera `IPv6AcceptRA=false`), o bootstrap instala um drop-in do systemd-networkd que o religa e que sobrevive à regeneração do arquivo base.
- **Privacy extensions desligadas (#142).** Com elas ligadas — o padrão de várias distribuições (`use_tempaddr = 2`) —, o Node ganha, além do endereço estável, um endereço **temporário que rotaciona sozinho**, e é o temporário que o tráfego de saída usa. O resultado é o que o tracker vê no announce não ser o endereço que o operador liberou. O bootstrap grava, em `/etc/sysctl.d/99-seedgrid-ipv6.conf`:

  ```
  net.ipv6.conf.all.use_tempaddr = 0
  net.ipv6.conf.default.use_tempaddr = 0
  net.ipv6.conf.<interface>.use_tempaddr = 0
  ```

  aplica as três chaves na hora (`sysctl -w`: só elas, nunca o arquivo inteiro) e remove os endereços temporários que já existam. `all` e `default` cobrem interfaces criadas depois do boot (um WiFi que sobe tarde, uma VLAN); a da interface vale já. É idempotente e **preserva as demais linhas** do arquivo (por exemplo um `accept_ra` feito à mão). Para conferir: `sysctl net.ipv6.conf.all.use_tempaddr` deve dar `0`, e `ip -6 addr show scope global` não deve listar nenhum endereço `temporary`.
- **Sufixo determinístico `::5eed:<N>` (ADR-038, #202).** O endereço SLAAC padrão é EUI-64, derivado do MAC: recriar o Node, trocar a placa ou migrar de hospedeiro o muda, e a regra de firewall do operador passa a apontar para o vazio — além de ser **imprevisível antes de o Node existir**. O bootstrap fixa a metade de baixo do endereço e deixa o **prefixo /64 vir do Router Advertisement**, então uma troca de prefixo do provedor continua sendo acompanhada.

### 50.2.1 A convenção

| | |
|---|---|
| Endereço | `<prefixo /64>::5eed:<N>` — por exemplo `2001:db8:1:2::5eed:4` para o Node 4. |
| `N` | O **índice do Node**: a posição dele na sequência de portas BitTorrent, `N = porta BT − 6880` (a porta 6881 é o Node 1; a 6884, o Node 4). O Control Plane o devolve no enroll como `node_index`; como a porta é única no cluster, o índice também é. Entra em **dígitos decimais** no último grupo — o Node 12 é `::5eed:12` —, de modo que se lê o número do Node direto do endereço. Vai de 1 a 1000 (a sequência de portas). |

- **Preparar a regra de firewall antes de o Node existir.** O próximo Node recebe a menor porta livre da sequência (a lista de Nodes do Portal mostra as portas em uso); o endereço dele é `<prefixo>::5eed:<porta − 6880>`, e a regra pode ser criada antes de o bootstrap rodar — o Node não passa pela janela em que já está de pé, anunciando, e ainda sem regra (nela o tracker o vê inalcançável).
- **Substituir um Node.** Remover um Node devolve a porta (e o índice) ao pool. O Node que o substitui recebe a menor porta livre — no caso comum, a que acabou de ser devolvida — e **herda o mesmo endereço**: as regras seguem valendo sem edição.
- **O token não substitui a detecção de mudança de IPv6.** Ele fixa só o **sufixo**; uma rotação de prefixo pelo provedor muda o endereço de todos os Nodes, e o alerta `NODE_IPV6_CHANGED` continua sendo como se fica sabendo (`docs/16 - Alerting.md`). O que muda é o custo do conserto: sobra uma incógnita, o prefixo novo.
- **Opt-out:** `IPV6_SUFFIX=off` no bootstrap mantém o endereço que o gerenciador de rede escolher (endereçamento delegado, DHCPv6 com estado, política própria). Um Node cuja porta BitTorrent está fora da sequência do Control Plane (instalação manual com token compartilhado) não tem índice, e o script não aplica o token nele.

### 50.2.2 Como o script aplica, por gerenciador de rede

O script descobre quem gerencia a interface da rota default e usa o mecanismo desse gerenciador; **não adivinha** — se não for um dos dois abaixo, imprime a instrução e segue. Nenhuma falha aqui interrompe o onboarding: o Node funciona com o endereço que o gerenciador escolher.

| Gerenciador | O que o script faz |
|---|---|
| **systemd-networkd** | Drop-in em `/etc/systemd/network/<arquivo>.network.d/99-seedgrid-ipv6-ra.conf` (o arquivo que o `networkctl status` informa, inclusive um gerado por netplan em `/run`) com `IPv6AcceptRA=true` e `IPv6Token=static:::5eed:<N>`. Depois `networkctl reload` e `networkctl reconfigure <interface>` (reinicia o serviço só se o `reconfigure` não existir). O drop-in sobrevive à regeneração do arquivo base. |
| **NetworkManager** | Edita o **keyfile** da conexão ativa (`/etc/NetworkManager/system-connections/`): na seção `[ipv6]`, `addr-gen-mode=eui64` e `token=::5eed:<N>`, preservando o resto do arquivo (o de uma conexão WiFi tem a senha). Fluxo: backup em `/var/backups/seedgrid-node/` → edita → `nmcli connection reload` → **confere o que o NM carregou** → `nmcli device reapply`. Se qualquer etapa falhar, o keyfile é **restaurado**. Conexão que não é keyfile em `/etc` (gerada por netplan, em `/run`) ou com `ipv6.method` diferente de `auto` não é alterada. |
| **Outro** | Imprime as instruções para o operador aplicar no gerenciador dele. |

**NetworkManager: o que não é óbvio** (e por que o script faz o que faz):

1. **O `nmcli` não consegue configurar o token.** Há um deadlock entre duas validações: o NM só aceita `ipv6.token` com `addr-gen-mode=eui64`, e o netplan (backend de gravação em Ubuntu) rejeita o par (`ipv6-address-generation` e `ipv6-address-token` são mutuamente exclusivos). Além disso o `nmcli modify` que falha **ainda assim grava o keyfile**, deixando disco e memória divergentes — um reboot aplicaria a troca sozinho, sem ninguém ter editado a regra de firewall. Por isso o script escreve o keyfile e, se algo falhar, o restaura.
2. **`::5eed:4` é gravado como `::94.237.0.4`.** É o mesmo endereço (`0x5eed` = 94.237): quando os 96 bits altos são zero, o NM usa a notação pontuada. Não é corrupção — e um comparador que reconheça só uma das formas toma o token já aplicado por não aplicado. O script normaliza os dois lados para a forma completa antes de comparar (o `nmcli -g` ainda escapa os `:` com barra).
3. **Depois do `reapply` sobra um EUI-64 em paralelo, e `autoconf=0` é a correção errada.** Quem gera o endereço do token é o **kernel**, a partir do Prefix Information Option do Router Advertisement. Desligar o `autoconf` mata a renovação junto: o endereço fica de pé com o lifetime que já tinha e **expira em cerca de duas horas**. O EUI-64 é só resíduo do instante do `reapply`; apagá-lo com `ip -6 addr del` resolve, e ele não volta com o `autoconf` preservado. O script apaga o resíduo (EUI-64, endereço de privacidade estável e temporários do mesmo `/64`, só os dinâmicos) **depois** de o endereço do token estar de pé — nunca antes.

### 50.2.3 Como conferir

**Existir não basta: o endereço tem que renovar.** O critério de aceite de qualquer procedimento que aplique o token é observar o `valid_lft` do endereço **subir** depois de um Router Advertisement (observe por cerca de 7 minutos). Foi só isso que separou "funcionou" de "vai cair sozinho daqui a duas horas".

```sh
ip -6 -o addr show dev <interface> scope global | grep '::5eed:'    # o endereço e o valid_lft
# ... espere um Router Advertisement (~7 min) e repita: o valid_lft tem que ter voltado a subir
```

Depois de trocar o endereço de um Node que **já estava no ar** (`--network-only`), **reinicie o Engine** (ele precisa reabrir o socket no endereço novo) e **reanuncie** aos trackers: sem o reanúncio, o tracker guarda o veredito "não conectável" colhido na janela entre a troca do endereço e a edição da regra de firewall.

## 50.3 Engine e `/data`
- **O nome do container é um contrato.** O Engine roda no container `seedgrid-engine`, definido pelo `container_name` do compose servido pelo Control Plane. O Agent faz `docker inspect` nesse nome para reportar o uptime do Engine (`engine_started_at`); o default é o mesmo (`SEEDGRID_ENGINE_CONTAINER`, §45). Se os dois divergem **nada falha** — o campo só fica vazio —, por isso o nome vive numa constante e um teste confere compose, bootstrap, script de reanexação e default do Agent. O nome é neutro de motor de propósito: o Engine é um conceito do produto, o qBittorrent é a implementação de hoje.
- **A chave do serviço no compose continua `qbittorrent`** (é a imagem que ele roda). Não a renomeie num Node que já roda: o compose passaria a tratar o container antigo como órfão e o deixaria de pé ao lado do novo, disputando as mesmas portas (rede host).
- **`/data` pode ser um disco dedicado ou uma pasta no disco do sistema** — o bootstrap grava qual dos dois em `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` (§50.1.1). A propagação de mount abaixo, a auto-recuperação de hotplug e a exigência de *mountpoint* só têm o que fazer com disco dedicado; com `/data` no disco do sistema elas são inofensivas: o bootstrap não instala a reanexação (não há UUID de disco), e a propagação `rslave` num `/data` que é só uma pasta da raiz compartilhada simplesmente não propaga nada.
- **O bind de `/data` usa `propagation: rslave`** — um disco montado depois de o Engine subir aparece dentro do container. O que isso resolve e o que não resolve, e o pré-requisito sobre o mount que contém `/data`, estão em `docs/06 - Storage.md` §35.1. O bootstrap grava a escolha (`rslave`, ou `rprivate` quando o host não a suporta) em `SEEDGRID_DATA_PROPAGATION` no `.env` do compose.

## 50.4 Atualizar um Node que já existe
O onboarding guarda uma cópia do script em `/opt/seedgrid-node/bootstrap-node.sh`; os comandos abaixo a usam (como `root`). Num Node instalado antes disso a cópia não existe — baixe de novo, com o modo depois de `sh -s --`: `curl -fsSL http://<cp>/node/bootstrap-node.sh | sudo sh -s -- --network-only`.


### Rede (privacy extensions e sufixo IPv6)
`sh /opt/seedgrid-node/bootstrap-node.sh --network-only` aplica os passos 2 e 6 num Node já registrado: desliga as privacy extensions e fixa o sufixo `::5eed:<N>`, com `N` calculado da porta BitTorrent que está em `/etc/seedgrid-agent.env` (`IPV6_NODE_INDEX=<N>` o fixa à mão). É idempotente: com o endereço já de pé não muda nada, e repetir não reinicia a rede. **Um Node por vez**, nesta ordem, porque o endereço muda:

1. **crie a regra de firewall do endereço novo** (`<prefixo>::5eed:<N>`, porta BT, TCP+UDP) — pode ser antes de rodar o script;
2. rode `--network-only` e confirme o endereço (§50.2.3), inclusive o `valid_lft` subindo;
3. **reinicie o Engine e reanuncie** (§50.2.3);
4. confirme o alcance de fora (a regra nova valendo) e só então apague a regra do endereço antigo;
5. passe ao próximo Node.

Um Node que já aplicou a convenção à mão (`::5eed:<N>` com o `N` certo) é reconhecido: o script vê o endereço de pé e **não mexe na rede** — só garante o `use_tempaddr = 0` (acrescentando as chaves ao arquivo do sysctl sem tocar nas outras linhas) e apaga o resíduo de EUI-64, se houver.

### Storage (disco dedicado ou disco do sistema)
**Nenhum Node existente precisa de ação:** sem `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` o Agent exige o *mountpoint*, que é o comportamento de sempre. Dois casos pedem uma mudança deliberada:

- **Node de disco único que ficou fora do cluster** (`storage_ok=false`, `não é mountpoint`, `NODE_STORAGE` aberto, sem receber torrents), por ter sido instalado quando a exigência valia para todos: declare o disco único no env e reinicie o Agent —
  ```sh
  grep -q '^SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=' /etc/seedgrid-agent.env \
    && sed -i 's/^SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=.*/SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false/' /etc/seedgrid-agent.env \
    || echo 'SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false' >> /etc/seedgrid-agent.env
  systemctl restart seedgrid-agent
  ```
  O Agent reporta `storage_ok=true` no heartbeat seguinte; o `NODE_STORAGE` resolve sozinho. Confirme que `/data` é, de fato, uma pasta no disco do sistema e que o espaço livre dele serve para o que o Node vai guardar — daqui em diante é o `NODE_DISK_FULL` que vigia.
- **Node de disco único que ganhou um disco dedicado:** `sh /opt/seedgrid-node/bootstrap-node.sh --storage-only` com o disco conectado. Monta, instala a reanexação, recria o Engine e grava `true`.

### Engine (nome do container e propagação de mount)
O compose só é lido quando o container é criado, então um Node criado antes de uma mudança de compose continua com o container antigo até ser recriado. Para as mudanças de nome do container (`seedgrid-qbittorrent` → `seedgrid-engine`) e de propagação de mount (`rprivate` → `rslave`), **um Node por vez**:

1. **Ache o compose do Node** (o diretório onde ele foi criado):
   `docker inspect <container-atual> -f '{{index .Config.Labels "com.docker.compose.project.working_dir"}}'`
2. **Troque o `docker-compose.yml`** desse diretório pelo atual — `git pull`, se for um clone do repositório, ou `curl -fsSL http://<cp>/node/docker-compose.yml -o docker-compose.yml`.
3. **Fixe o nome do container no env do Agent**, explicitamente. Vale para o Agent antigo (cujo default é outro) e para o novo, então a ordem de atualização do binário não importa:
   ```sh
   grep -q '^SEEDGRID_ENGINE_CONTAINER=' /etc/seedgrid-agent.env \
     && sed -i 's/^SEEDGRID_ENGINE_CONTAINER=.*/SEEDGRID_ENGINE_CONTAINER=seedgrid-engine/' /etc/seedgrid-agent.env \
     || echo 'SEEDGRID_ENGINE_CONTAINER=seedgrid-engine' >> /etc/seedgrid-agent.env
   ```
4. **Confira o pré-requisito do `rslave`**: `findmnt -no PROPAGATION -T /data` deve mostrar `shared`, `slave` ou `shared,slave`. Se mostrar `private`, defina `SEEDGRID_DATA_PROPAGATION=rprivate` no `.env` do compose (o Docker recusaria o container).
5. **Recrie o Engine** com o compose novo: `docker compose up -d`. Como a chave do serviço não mudou, o compose recria o container no lugar — o container antigo sai e o `seedgrid-engine` entra. O qBittorrent reinicia (alguns segundos sem semear; o estado dos torrents é retomado do disco).
6. **Reinstale o script de reanexação de storage — nos Nodes que o têm** (`ls /usr/local/bin/seedgrid-storage-reattach.sh`: são os criados pelo bootstrap com um disco de UUID conhecido; um Node montado à mão, em container de sistema, não o tem, e a recuperação do disco dele é do hipervisor). Nas versões antigas o nome do container estava fixo no `docker restart`: sem trocar o script, a reanexação por udev passa a falhar em silêncio depois da renomeação.
   `curl -fsSL http://<cp>/node/seedgrid-storage-reattach.sh -o /usr/local/bin/seedgrid-storage-reattach.sh && chmod 0755 /usr/local/bin/seedgrid-storage-reattach.sh`
7. **Reinicie o Agent** (`systemctl restart seedgrid-agent`: relê o env e, no primeiro heartbeat, reaplica no Engine a configuração que o Control Plane entrega — ADR-039) e **confira**:
   - `docker inspect seedgrid-engine -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Propagation}}{{end}}{{end}}'` → `rslave`;
   - `docker ps --format '{{.Names}}'` mostra `seedgrid-engine` e **não** o nome antigo;
   - no Portal (ou em `GET /api/v1/nodes`), o Node volta a ONLINE e `engine_started_at` está preenchido.

## 50.5 Migrar os Nodes para a configuração pelo Control Plane (ADR-039, #144)

**O que muda.** Os parâmetros de comportamento do Agent e do Engine deixam de ser decididos pelo `.env` de cada máquina e passam a vir do Control Plane (valor de **cluster** em Configurações → Agent / Engine, com **override por Node**). O `.env` fica só com identidade e máquina (§45); as variáveis de comportamento viram **fallback**.

**O perigo, e por que o procedimento tem ordem.** Um Agent novo passa a obedecer o Control Plane. Um Node cujo `.env` tinha um valor **diferente** do de cluster mudaria de comportamento no primeiro heartbeat — foi exatamente o erro de 05/09/2026, quando `MAX_ACTIVE_DOWNLOADS=10` "para alinhar" sobrecarregou o node-04 (Atom). Por isso o override do Node tem de existir **antes** de o Agent dele ser atualizado.

**Inventário (leitura do `/etc/seedgrid-agent.env` dos quatro Nodes, 03/10/2026; nenhum segredo):**

| Variável | node-01 | node-02 | node-03 | node-04 | Default de cluster |
|---|---|---|---|---|---|
| `SEEDGRID_AGENT_HEARTBEAT_INTERVAL` | `10s` | `10s` | `10s` | `10s` | `10s` |
| `SEEDGRID_AGENT_COMMAND_POLL_INTERVAL` | `5s` | `5s` | `5s` | `5s` | `5s` |
| `SEEDGRID_LOG_LEVEL` | `info` | `info` | `info` | `info` | `info` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS` | `10` | `10` | `10` | **`3`** | `10` |
| `QUEUEING_ENABLED`, `MAX_ACTIVE_UPLOADS`, `MAX_ACTIVE_TORRENTS`, `MAX_ACTIVE_CHECKING`, `DONT_COUNT_SLOW`, `SLOW_DL/UL_THRESHOLD`, `SLOW_INACTIVE_TIMER` | *(não definidas — valem os defaults do código)* | idem | idem | idem | = defaults do código (`true`, `10000`, `10000`, `1`, `false`, `1`, `1`, `300`) |

Os defaults de cluster do catálogo foram escolhidos para **bater com o que a maioria usa**. Resultado: **só o node-04 precisa de override** — `engine.max_active_downloads = 3`. Os outros três acompanham o cluster sem mudar nada. (Aproveite: `SEEDGRID_ENGINE_CONTAINER=seedgrid-qbittorrent` segue nos quatro `.env` — é identidade/máquina e **não** muda por esta migração.)

**Procedimento — Control Plane primeiro, depois os Agents um a um (ADR-027):**

1. **Deploy do Control Plane** (o CI faz). A migration `00044` cria `node_settings` e as colunas de versão (aplicada no boot do controller, depois da `00043` do perfil de tracker). Nada muda nos Nodes: os Agents atuais não conhecem `agent_config` e ignoram o campo novo da resposta do heartbeat.
2. **Confira os valores de cluster** em Configurações → Agent / Engine: devem ser os defaults da tabela acima (não é preciso editar nada).
3. **Crie o override do node-04 — antes de atualizar o Agent dele.** Portal → Node `seedgrid-node-04` → *Configuração deste Node* → **Valor próprio** em *Downloads simultâneos por Node* → `3` → Salvar. (Ou `PUT /api/v1/nodes/<id>/settings/engine.max_active_downloads` com `{"value":"3"}`.) Confirme na lista *Nodes com valor próprio*, em Configurações.
4. **Atualize um Agent por vez** (botão "↑ versão", ADR-027), começando por um i5 (node-01) e deixando o node-04 por último. Depois de cada um confira: o selo da seção *Configuração deste Node* vira **Em dia**; o log do Agent diz `configuração do Agent aplicada` com `origem: control-plane`; e, no node-04, o Engine segue com **3** downloads (`engine.max_active_downloads` efetivo = 3, origem *valor deste Node*).
5. **Nada a limpar nos `.env`.** As variáveis de comportamento continuam lá como fallback (Control Plane fora no boot). Se quiser remover as que coincidem com o cluster, remova — o Node continua igual.

**Rede de segurança: a semeadura.** Se o passo 3 for esquecido, o Agent novo do node-04 não adota o valor de cluster: o Control Plane pede o relato do `.env` (`report_local`) e o Agent **não aplica nada** antes de relatar; com `MAX_ACTIVE_DOWNLOADS=3` no `.env` e o valor de cluster ainda no default, o Control Plane **semeia** o override `3` (Evento `NODE_CONFIG_SEEDED`, visível em Alertas & Eventos) e só então o Agent aplica. As condições (e por que o procedimento recomendado continua sendo o override manual) estão em `25 - Business Rules.md` §3.1.1b. **A semeadura não age** se o valor de cluster da chave já foi editado por um operador — nesse caso o Node segue o cluster, que é a precedência documentada: confira antes de atualizar.

**Reverter.** Restaure o binário anterior do Agent (`seedgrid-agent.prev`, ADR-027): o Agent antigo volta a ler só o `.env`, que não foi tocado. Para desfazer **só** o valor próprio de um Node, use *Voltar ao cluster*. O que a semeadura criou também se remove assim — e **não reaparece** (a semeadura é de uma vez só por Node).

**Se o Control Plane for revertido** com Agents novos já em dia, um heartbeat falha (o único que ainda leva `config_version`) e os seguintes já saem sem campos novos; o Node não chega a ficar offline (limite de 90 s). Os Agents mantêm o último valor aplicado.
