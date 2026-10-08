# SeedGrid

## Storage Specification

Version: 1.0

Status: Draft

Document: 06 - Storage.md

---

# 1. Objetivo

Este documento define a arquitetura de armazenamento do SeedGrid.

Seu objetivo é estabelecer como a plataforma representa, monitora e administra recursos de armazenamento utilizados pelos Seed Nodes.

---

# 2. Filosofia

O SeedGrid administra recursos de armazenamento, não dispositivos físicos.

Toda decisão da plataforma deverá basear-se nas características do recurso disponível, independentemente da tecnologia utilizada para implementá-lo.

---

# 3. Definição

Um **Storage** representa um recurso de armazenamento disponibilizado por um Seed Node.

Esse recurso poderá corresponder a:

* um disco físico;
* uma partição;
* um volume lógico;
* um dataset;
* um compartilhamento de rede;
* qualquer outro recurso capaz de armazenar dados persistentes.

A plataforma não assume uma tecnologia específica.

---

# 4. Responsabilidades

A camada de Storage deverá:

* representar recursos de armazenamento;
* acompanhar capacidade;
* acompanhar disponibilidade;
* disponibilizar informações ao Scheduler;
* fornecer informações ao Portal;
* integrar-se ao sistema de monitoramento.

A camada de Storage não executa operações de download nem de gerenciamento de torrents.

---

# 5. Identidade

Cada Storage deverá possuir identidade própria dentro do Cluster.

Informações mínimas:

* identificador único;
* Seed Node proprietário;
* nome;
* tipo;
* capacidade;
* estado operacional.

A identidade deverá permanecer estável enquanto o recurso existir.

---

# 6. Capacidade

Todo Storage deverá informar:

* capacidade total;
* capacidade utilizada;
* capacidade disponível;
* percentual de utilização.

Essas informações deverão permanecer atualizadas para apoiar decisões do Scheduler.

---

# 7. Estado

Cada Storage deverá possuir um estado operacional.

Estados previstos:

* Available;
* Degraded;
* Full;
* Read Only;
* Offline;
* Maintenance.

O estado deverá refletir a condição atual do recurso.

---

# 8. Health

Além do estado operacional, cada Storage deverá possuir um indicador de saúde.

O Health poderá considerar:

* erros de leitura;
* erros de escrita;
* disponibilidade;
* integridade;
* informações fornecidas pelo sistema operacional.

A forma de cálculo permanece responsabilidade da implementação.

---

# 9. Inventário

Todos os Storages deverão compor o inventário oficial do Cluster.

O inventário deverá permitir identificar:

* localização;
* capacidade;
* utilização;
* estado;
* associação ao Seed Node.

---

# 10. Scheduler

O Scheduler utilizará informações de Storage durante suas decisões.

Exemplos:

* espaço disponível;
* estado operacional;
* políticas administrativas;
* capacidade remanescente.

O Scheduler não deverá depender de detalhes específicos da tecnologia de armazenamento.

---

# 11. Monitoramento

Todos os Storages deverão disponibilizar informações ao sistema de monitoramento.

Exemplos:

* utilização;
* crescimento;
* disponibilidade;
* erros;
* temperatura, quando disponível.

Essas informações poderão originar métricas, eventos e alertas.

---

# 12. Escopo

Este documento define conceitos arquiteturais.

Não estabelece tecnologias específicas de armazenamento.

A implementação poderá utilizar qualquer tecnologia compatível com os princípios definidos pela plataforma.

---

# 13. Objetivos

A arquitetura de Storage deverá:

* abstrair tecnologias físicas;
* facilitar decisões do Scheduler;
* simplificar expansão;
* reduzir acoplamento.

---

# 14. Princípio Fundamental

O SeedGrid administra recursos de armazenamento.

A implementação física desses recursos constitui responsabilidade do ambiente onde a plataforma estiver implantada.



---

# 15. Configuração

Cada Seed Node deverá possuir um ou mais diretórios configurados para armazenamento de dados.

A definição desses diretórios é responsabilidade do operador.

O SeedGrid apenas utilizará os caminhos disponibilizados pelo Node.

---

# 16. Utilização

Os recursos de armazenamento pertencem exclusivamente ao Seed Node.

O Control Plane não realiza gerenciamento direto do sistema de arquivos.

As operações locais permanecem sob responsabilidade do Agent e do Torrent Engine.

---

# 17. Capacidade Disponível

O Agent deverá informar periodicamente ao Control Plane informações consolidadas sobre o armazenamento do Seed Node.

Exemplos:

* capacidade total;
* capacidade utilizada;
* capacidade disponível;
* percentual de utilização.

Essas informações apoiarão decisões do Scheduler em nível de Node.

---

# 18. Crescimento

O aumento da capacidade de armazenamento de um Seed Node deverá ocorrer através da expansão do ambiente local.

Exemplos:

* substituição de discos;
* expansão de volumes;
* inclusão de novos dispositivos.

Após atualização do inventário, o Control Plane reconhecerá automaticamente a nova capacidade disponível.

---

# 19. Remoção

Caso um recurso de armazenamento seja removido do Seed Node, o Agent deverá atualizar o inventário correspondente.

A plataforma deverá registrar eventos relacionados à alteração e disponibilizar informações ao operador.

---

# 20. Descoberta

Os recursos de armazenamento utilizados pelo SeedGrid deverão ser identificados automaticamente pelo Agent.

O inventário deverá representar apenas os recursos explicitamente disponibilizados para utilização pela plataforma.

---

# 21. Monitoramento

Cada Seed Node deverá disponibilizar informações consolidadas sobre seu armazenamento.

Exemplos:

* utilização;
* espaço disponível;
* crescimento do consumo;
* disponibilidade;
* estado operacional.

Essas informações alimentarão o sistema de monitoramento.

---

# 22. Alertas

A camada de armazenamento poderá originar alertas.

Exemplos:

* espaço insuficiente;
* crescimento acelerado do consumo;
* indisponibilidade do armazenamento;
* mudança inesperada para modo somente leitura.

Os critérios permanecerão configuráveis.

---

# 23. Portal

O Portal deverá apresentar informações consolidadas sobre o armazenamento de cada Seed Node.

Informações recomendadas:

* capacidade total;
* utilização;
* espaço disponível;
* tendência de crescimento;
* estado operacional;
* alertas relacionados.

---

# 24. Expansão

A expansão da capacidade do Cluster ocorrerá naturalmente pela:

* ampliação da capacidade dos Seed Nodes existentes; ou
* inclusão de novos Seed Nodes.

Nenhuma dessas operações deverá exigir alterações estruturais na arquitetura da plataforma.

---

# 25. Princípios

A arquitetura de Storage deverá:

* abstrair detalhes do sistema de arquivos;
* manter independência em relação à tecnologia de armazenamento;
* fornecer informações consistentes ao operador;
* apoiar monitoramento e planejamento de capacidade;
* permanecer desacoplada das decisões arquiteturais do Scheduler.


---

# 26. Backup

O armazenamento utilizado pelos Seed Nodes poderá seguir políticas de backup independentes.

O SeedGrid deverá registrar e apresentar a política configurada para cada Seed Node, sem assumir responsabilidade direta pela proteção dos dados armazenados.

As estratégias de backup são definidas em **Backup.md**.

---

# 27. Disaster Recovery

Em cenários de recuperação, o armazenamento deverá ser tratado como um recurso pertencente ao Seed Node.

A substituição ou recuperação de um Node deverá seguir os procedimentos descritos em **Disaster Recovery.md**.

O Control Plane deverá manter o Estado do Cluster independente da implementação física do armazenamento.

---

# 28. Auditoria

Alterações administrativas relacionadas ao armazenamento deverão gerar registros de auditoria quando aplicável.

Exemplos:

* alteração de caminhos de armazenamento;
* habilitação ou desabilitação de áreas de armazenamento;
* entrada ou saída de modo de manutenção;
* alterações de configuração.

A auditoria deverá registrar:

* data e hora;
* operador;
* Seed Node afetado;
* operação executada;
* resultado.

---

# 29. Observabilidade

A camada de armazenamento deverá integrar-se ao modelo de observabilidade da plataforma.

Sempre que aplicável, deverá produzir:

* métricas;
* eventos;
* alertas;
* logs operacionais.

Cada mecanismo deverá cumprir sua finalidade específica, conforme definido em **Glossary.md**.

---

# 30. Segurança

O acesso ao armazenamento deverá respeitar os princípios de segurança definidos pela plataforma.

O SeedGrid não deverá exigir privilégios superiores aos necessários para sua operação.

Sempre que possível, deverá adotar o princípio do menor privilégio.

---

# 31. Evolução

Novas funcionalidades relacionadas ao armazenamento deverão preservar a simplicidade da arquitetura da V1.

Evoluções que modifiquem o modelo arquitetural de armazenamento deverão ser planejadas para versões futuras e precedidas por ADR específica.

---

# 32. Relação com os Demais Documentos

Este documento complementa:

* Architecture.md;
* Infrastructure.md;
* Scheduler.md;
* Monitoring.md;
* Alerting.md;
* Backup.md;
* Disaster Recovery.md;
* Glossary.md.

Sempre que houver dúvidas sobre responsabilidades relacionadas ao armazenamento, este documento deverá ser considerado a referência oficial.

---

# 33. Resumo

A arquitetura de armazenamento do SeedGrid baseia-se nos seguintes princípios:

* o armazenamento pertence ao Seed Node;
* o Control Plane administra informações, não dispositivos;
* o Agent coleta e reporta informações de armazenamento;
* o Scheduler utiliza informações consolidadas em nível de Node;
* a plataforma permanece independente da tecnologia de armazenamento utilizada.

Esses princípios garantem simplicidade, baixo acoplamento e evolução controlada da plataforma.

---

# 34. Declaração Final

A arquitetura de armazenamento do SeedGrid foi projetada para abstrair detalhes de implementação sem perder visibilidade operacional.

Ao tratar o armazenamento como um recurso administrado pelo Seed Node, a plataforma mantém uma arquitetura simples, consistente e preparada para evoluções futuras, preservando os princípios estabelecidos para a versão 1.0.

---

# 35. Proteção do Seed Node contra disco de dados ausente, atrasado ou remontado

O Engine grava os dados dos torrents em `/data`. Quando `/data` **não é** o disco de dados — porque ele ainda não foi montado, foi montado depois de o Engine subir, ou saiu e voltou —, o comportamento padrão é o pior caso possível, e **silencioso**: o Engine continua gravando, só que no disco do **sistema**. O Node segue ONLINE, o Engine segue alcançável e o espaço livre medido passa a ser o da raiz, então ele parece ter folga e continua aceitando torrents até encher o disco de sistema.

Isso só é um defeito para quem **tem** um disco de dados. Um Node de disco único — uma VM, um mini PC, um NAS — usa o disco do sistema por escolha, e para ele "`/data` é uma pasta" é o estado correto e permanente. Os dois casos são **indistinguíveis** olhando o `/data` de agora (um disco que não montou parece uma pasta local), então o SeedGrid não tenta distingui-los em tempo de execução: o operador **declara** na instalação qual é o caso deste Node, e o Agent obedece (§35.3). **A cadeia abaixo protege o Node que declarou disco dedicado**; o disco único é uma configuração suportada, com as garantias descritas em §35.3.

A proteção não é um mecanismo único. É uma cadeia de camadas independentes, e cada uma cobre uma falha que a anterior não enxerga:

| # | Camada | Quem | Falha que cobre |
|---|---|---|---|
| 1 | **Montagem por UUID, tolerante a ausência** | `bootstrap-node.sh` (`/etc/fstab`) | Disco presente no boot: monta pelo UUID do filesystem (o `/dev/sdX` muda entre boots) com `nofail` e `x-systemd.device-timeout`, de modo que um disco ausente **não impede o SO de subir**. |
| 2 | **Checagem de saúde do `/data`** | Agent (`StorageHealth`, a cada heartbeat) | `/data` que não é *mountpoint* (mesmo `st_dev` do diretório-pai) ou que não aceita escrita. O motivo (`não é mountpoint`, `somente leitura`, …) vai no heartbeat como `storage_ok=false` + `storage_detail`. **A exigência de mountpoint só vale com disco dedicado declarado** (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true`, o padrão — §35.3); a escrita vale sempre. |
| 3 | **Alerta `NODE_STORAGE`** (CRITICAL) | Control Plane | A falha **sustentada** por `alerting.storage_grace` (1m por padrão) vira alerta e notificação. A carência existe para um soluço de I/O não gritar; a resolução não tem carência. |
| 4 | **Exclusão do Scheduler** | Control Plane | Node com `storage_ok = false` **não recebe trabalho**, desde a primeira leitura ruim (aqui não há carência: é proteção, não indicador). `NULL` — Agent que não reporta o campo — segue elegível. |
| 5 | **Reanexação por udev** | `seedgrid-storage-reattach` (regra udev + unit oneshot, instalados pelo bootstrap quando `/data` tem UUID) | Disco que **cai e volta** (re-enumera com outro `/dev/sdX`): remonta `/data` se preciso, **reinicia o Engine** e rechecka os torrents em erro. |
| 6 | **Propagação de mount para o Engine** | `docker-compose.yml` (`propagation: rslave`) | Disco montado no host **depois** de o Engine subir: o mount novo atravessa para dentro do container sem recriá-lo. |
| 7 | **Anexar depois** | `bootstrap-node.sh --storage-only` | Node que nasceu sem disco: monta por UUID, instala a camada 5, religa o Engine ao `/data` novo e **declara o disco dedicado** (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true`, §35.3). Idempotente; descobre o Control Plane pelo env do Agent. |

As camadas 1, 5, 6 e 7 **impedem ou reparam** o problema no próprio Node; as 2, 3 e 4 existem porque as demais podem falhar e o operador precisa saber, e porque nenhum trabalho novo deve ir para um disco que não está lá.

No Node de **disco único** não há disco que possa faltar, então as camadas 1, 5, 6 e 7 não se aplicam (não há o que montar, reanexar ou propagar), e a exigência de *mountpoint* da camada 2 não existe. Sobra o que continua fazendo sentido: a sonda de escrita (um `/data` somente-leitura é defeito em qualquer topologia, e dispara as camadas 3 e 4) e o espaço livre, vigiado pelo `NODE_DISK_FULL` e pelo filtro de espaço do Scheduler.

## 35.1 Propagação de mount (camada 6)

Por padrão o Docker monta um bind com propagação `rprivate`: o que o host monta **depois** em `/data` não atravessa para o namespace de montagem do container. O Engine segue enxergando o diretório antigo — o disco de sistema — mesmo com o disco de dados já montado no host. O compose do Seed Node usa a sintaxe longa para escolher `rslave`:

```yaml
- type: bind
  source: /data
  target: /data
  bind:
    propagation: ${SEEDGRID_DATA_PROPAGATION:-rslave}
```

O comportamento foi **medido**, não presumido:

| Cenário | Resultado |
|---|---|
| Container sobe com `/data` ainda sendo diretório do sistema; o disco é montado no host depois | `rprivate`: o container continua vendo o diretório antigo. **`rslave`: o container vê o disco.** |
| (`rslave`) O disco é desmontado e montado de novo, sem o Engine ter arquivo aberto nele | A desmontagem e a nova montagem atravessam: o container volta a ver o diretório do sistema e depois o disco. |
| (`rslave`) O Engine **tem arquivo aberto** no disco antigo e o host remonta | O container **continua dono do mount antigo**: o mount novo não o substitui, e os handles apontam para o device que sumiu. |
| `docker restart` depois de qualquer um dos casos acima (`rprivate` ou `rslave`) | O container sobe enxergando o mount que o host tem naquele instante. |

Duas conclusões operacionais:

1. **A propagação resolve "o disco chegou depois", não "o disco caiu e voltou".** No segundo caso o Engine segue com handles do device antigo, e só um restart os derruba — por isso a camada 5 continua reiniciando o Engine. Também não é preciso *recriar* o container: o restart basta, porque o container sobe com o mount que o host tem naquele instante.
2. **Quando `/data` é um bind de um diretório do hospedeiro** (Node em container de sistema, em que o hipervisor monta o disco e o entrega por bind), o mount do hipervisor não atravessa para dentro do Node ao vivo — o bind é resolvido quando o container de sistema sobe. Nessa topologia a recuperação de disco que cai e volta é do lado do hipervisor (reiniciar o container de sistema depois de remontar), e a camada 6 só vale para discos montados *dentro* do Node.

### Pré-requisito: o mount que contém `/data` não pode ser `private`

`rslave` exige que o mount que contém o diretório de origem seja compartilhado ou *slave*. Se for `private`, o Docker **recusa criar o container** (`path /data is mounted on / but it is not a shared or slave mount`) — **não é um no-op, é uma falha**, e o Engine não sobe. Num host com systemd a raiz já é compartilhada; só um init que não faz esse ajuste cai no problema. Para não transformar um endurecimento num defeito, o bootstrap checa antes:

```sh
findmnt -no PROPAGATION -T /data    # shared | slave | shared,slave  →  rslave funciona
                                    # private                        →  o bootstrap grava rprivate
```

e, no segundo caso, grava `SEEDGRID_DATA_PROPAGATION=rprivate` no `.env` do compose (o comportamento anterior, com aviso). O mesmo valor pode ser definido à mão. Para conferir o que um Engine em execução está usando:

```sh
docker inspect seedgrid-engine -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Propagation}}{{end}}{{end}}'
# rslave
```

## 35.2 Propagação e Nodes que já existem

O compose é lido na criação do container: um Engine criado antes desta mudança continua com `rprivate` até ser recriado a partir do compose novo (`docker compose up -d`). O procedimento está em `docs/19 - Deployment.md` §50.4.

## 35.3 Disco dedicado ou disco único: a declaração `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT`

Quem decide se `/data` precisa ser um *mountpoint* é uma variável **por máquina**, no `/etc/seedgrid-agent.env` do Node, gravada pelo `bootstrap-node.sh` na instalação:

| Valor | Significa | O Agent confere | Quando o bootstrap grava |
|---|---|---|---|
| `true` | `/data` é um **disco dedicado** | mountpoint **e** escrita | montou um disco em `/data`, ou `/data` já estava montado; e sempre no `--storage-only` |
| `false` | `/data` é uma pasta no **disco do sistema** (disco único) | só a escrita | não havia disco para montar: `/data` ficou no disco do sistema |
| *(ausente)* | — | como `true` | — (é o que todo Node anterior à variável tem) |

**Por que declarada, e não inferida.** A inferência natural — "se `/data` é uma pasta e não um mountpoint, não exijo mountpoint" — erra justamente no caso que a regra existe para pegar: um disco dedicado que **não montou** é, visto do Node, uma pasta local. Qualquer detecção em tempo de execução transformaria a falha silenciosa que a §35 combate em estado "saudável". Por isso a expectativa é gravada **uma vez, na instalação**, quando o operador (e o script) sabem o que foi feito, e o Agent a trata como dado — nunca a reavalia. **Ausente é `true`**: um Node que nunca teve a variável continua exatamente como era, e qualquer erro de configuração cai para o lado seguro (Node excluído, nunca disco que não montou aceito em silêncio). Um valor que não é booleano (`flase`, `nao`) também vale `true`.

**O que o bootstrap faz em cada caminho**

- **Montou, ou achou, um disco dedicado** → grava `true`.
- **Não achou disco** (`/data` ficou no disco do sistema) → grava `false` e **avisa no terminal**: o Node vai guardar os torrents no disco do sistema, o Agent não vai exigir *mountpoint*, o espaço livre segue com o `NODE_DISK_FULL`, e como anexar um disco depois (`sh /opt/seedgrid-node/bootstrap-node.sh --storage-only`).
- **`--storage-only`** (anexar o disco depois) → grava `true` e reinicia o Agent, que só relê o env ao subir. Idempotente.
- **Segunda passada nunca rebaixa um Node que já declarou disco.** Se o bootstrap roda de novo sem achar o disco, mas o env anterior já dizia `true` (ou o `fstab` já monta `/data`), o valor continua `true`: o disco **sumiu**, o Node não virou "disco único". Só uma declaração explícita rebaixa.
- **Overrides do operador**, para o que o script não tem como saber: `STORAGE_REQUIRE_MOUNT=true` declara disco dedicado mesmo sem disco *agora* — o caso do disco que chega alguns minutos depois da instalação, em que o Node deve ficar fora do Scheduler até ele montar, em vez de aceitar torrents no disco do sistema —; `STORAGE_REQUIRE_MOUNT=false` volta um Node ao disco do sistema de propósito. Qualquer outro valor aborta o script antes de mexer em qualquer coisa.

**O que o Node de disco único recebe e o que não recebe**

- Fica com `storage_ok = true` enquanto `/data` aceitar escrita, é elegível no Scheduler e **não abre `NODE_STORAGE`**.
- O disco medido (`disk_total_bytes`, `disk_free_bytes`) passa a ser o do **sistema**, então é ele que alimenta o filtro de espaço do Scheduler (ADR-032) e o `NODE_DISK_FULL`. Convém uma margem (`admission.disk_min_free`) que cubra também o que o sistema operacional precisa.
- **Não há proteção contra "o disco não montou"** — não existe disco. Se um disco for anexado por fora do bootstrap (montado à mão em `/data`) num Node declarado `false`, a exigência **continua desligada**: o Agent não infere. Use `sh /opt/seedgrid-node/bootstrap-node.sh --storage-only`, que monta, instala a reanexação e passa o Node a `true`.

**Trocar a declaração num Node que já existe**: disco único → disco dedicado é o `--storage-only` (acima). Disco dedicado → disco único (o disco foi retirado de vez) é editar `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false` no env e reiniciar o Agent — ou rodar o bootstrap com `STORAGE_REQUIRE_MOUNT=false`. Os Nodes anteriores à variável não precisam de nenhuma ação: ausente já é `true`. Ver `docs/19 - Deployment.md` §50.4.
