# **SeedGrid**
# **Alerting Specification**
Version: 1.0
Status: Draft
Document: 16 - Alerting.md
# **1. Objetivo**
O sistema de Alertas do SeedGrid é responsável por identificar situações que exigem atenção do operador.
Seu objetivo é transformar informações operacionais em notificações acionáveis, reduzindo o tempo necessário para identificar problemas relevantes.
# **2. Filosofia**
Nem todo evento gera um alerta.
Nem toda métrica gera um alerta.
Um alerta representa uma condição que merece atenção do operador.
Essa distinção deverá ser preservada em toda a plataforma.
# **3. Responsabilidades**
O sistema de Alertas deverá:
- avaliar condições;
- criar alertas;
- atualizar alertas existentes;
- encerrar alertas automaticamente quando aplicável;
- disponibilizar informações ao Portal.
O sistema NÃO deverá:
- executar ações corretivas;
- modificar configurações;
- alterar estados dos Nodes.
Essas responsabilidades pertencem ao Control Plane.
# **4. Arquitetura**
Métricas
Eventos
↓
Alert Service
↓
Database
↓
Portal
↓
Operador
# **5. Fontes**
Os alertas poderão ser originados a partir de:
Métricas.
Eventos.
Health Score.
Scheduler.
API.
Control Plane.
SeedGrid Agent.
Torrent Engine.
# **6. Ciclo de Vida**
Todo alerta seguirá o mesmo ciclo.
Detectado
↓
Aberto
↓
Reconhecido
↓
Resolvido
↓
Encerrado
Nem todos os alertas precisarão passar pelo estado "Reconhecido".
# **7. Severidade**
A versão 1.0 utilizará quatro níveis.
# **Informational**
Informações relevantes.
Não exigem ação.
# **Warning**
Situações que merecem acompanhamento.
Ainda não representam falha operacional.
# **Critical**
Problemas que exigem atenção imediata.
Podem comprometer parte do Cluster.
# **Emergency**
Problemas que comprometem significativamente a operação da plataforma.
Exigem intervenção prioritária.
# **8. Estados**
Todo alerta deverá possuir um estado.
Estados previstos:
Aberto.
Reconhecido.
Resolvido.
Encerrado.
# **9. Agrupamento**
Sempre que possível, alertas relacionados deverão ser agrupados.
Exemplo
Perda de comunicação de um Node deverá gerar apenas um alerta ativo.
Novos eventos relacionados atualizarão o mesmo alerta.
# **10. Alertas Automáticos**
O sistema deverá abrir alertas automaticamente quando condições previamente definidas forem atendidas.
Exemplos:
Node Offline.
Disco sem espaço.
SMART crítico.
Engine indisponível.
Control Plane indisponível.
# **11. Encerramento**
Sempre que possível, alertas deverão ser encerrados automaticamente.
Exemplo
Node voltou a responder.
↓
Alerta encerrado.
O operador não deverá precisar fechar manualmente alertas resolvidos pelo próprio sistema.
Alertas que o sistema não tem como confirmar resolvidos — os de alcançabilidade, que dependem de uma conexão de entrada que um Node ocioso nunca recebe — **expiram** após uma validade configurável, sem notificar e sem reabrir o mesmo episódio (ver §46).
# **12. Histórico**
Todo alerta deverá manter histórico.
Exemplos:
Data de abertura.
Mudanças de severidade.
Reconhecimento.
Resolução.
Encerramento.
# **13. Relação com Eventos**
Um alerta poderá estar relacionado a diversos eventos.
Os eventos representam fatos.
O alerta representa a condição operacional derivada desses fatos.
# **14. Objetivos**
O sistema de alertas deverá:
reduzir ruído;
priorizar problemas importantes;
facilitar diagnóstico;
melhorar tempo de resposta.
# **15. Princípio Fundamental**
Alertas representam problemas.
Eventos representam acontecimentos.
Logs representam detalhes técnicos.
Essas três categorias deverão permanecer claramente separadas em toda a arquitetura.
# **16. Regras de Alerta**
Toda regra de alerta deverá possuir critérios objetivos.
Cada regra deverá definir:
- condição de ativação;
- condição de encerramento;
- severidade;
- categoria;
- descrição;
- recurso monitorado.
As regras deverão ser determinísticas.
# **17. Categorias**
Os alertas serão organizados por categoria.
Categorias previstas:
Infraestrutura.
Storage.
Rede.
Control Plane.
Nodes.
Torrent Engine.
Scheduler.
API.
Segurança.
Configuração.
Essa organização facilitará filtros e dashboards.
# **18. Alertas do Control Plane**
Exemplos:
API indisponível.
Banco de dados indisponível.
Scheduler parado.
Uso excessivo de CPU.
Uso excessivo de memória.
Espaço insuficiente.
Falhas recorrentes.
# **19. Alertas dos Nodes**
Exemplos:
Node Offline.
Heartbeat expirado.
Health Score crítico.
Temperatura elevada.
CPU excessiva.
Memória insuficiente.
Tempo de atividade anormal.
# **20. Alertas de Armazenamento**
Exemplos:
Espaço abaixo do limite.
SMART crítico.
Disco removido.
Sistema de arquivos somente leitura.
Falha de montagem.
Erros de leitura.
Erros de escrita.
# **21. Alertas do Torrent Engine**
Exemplos:
Engine indisponível.
Fila parada.
Erro de autenticação.
Falha de comunicação.
Número excessivo de erros.
# **22. Alertas do Scheduler**
Exemplos:
Nenhum Node elegível.
Tempo excessivo de decisão.
Falhas consecutivas.
Distribuição anormal.
Todos os Nodes indisponíveis.
# **23. Alertas da API**
Exemplos:
Tempo de resposta elevado.
Quantidade excessiva de erros.
Falhas de autenticação.
Requisições acima do esperado.
# **24. Regras de Agrupamento**
Alertas equivalentes deverão ser consolidados.
Exemplos:
Heartbeat expirado.
Node Offline.
Falha de comunicação.
Essas condições poderão representar um único incidente.
# **25. Supressão**
O sistema deverá evitar geração repetitiva de alertas.
Exemplos:
Um Node offline durante uma hora continuará representado por um único alerta ativo.
Eventos subsequentes apenas atualizarão seu histórico.
# **26. Dependências**
Quando possível, dependências deverão ser consideradas.
Exemplo:
Caso o Control Plane esteja indisponível, não deverão ser gerados alertas individuais para todos os Heartbeats expirados.
A causa raiz deverá ser priorizada.
# **27. Reconhecimento**
O operador poderá reconhecer um alerta.
O reconhecimento indica que o problema é conhecido.
Ele não altera a condição monitorada.
Enquanto a condição permanecer, o alerta continuará ativo.
# **28. Resolução**
Um alerta será considerado resolvido quando sua condição deixar de existir.
A resolução poderá ocorrer:
automaticamente;
ou por intervenção administrativa, quando aplicável.
# **29. Retenção**
O histórico de alertas deverá permanecer disponível para auditoria.
A política de retenção será configurável.
Alertas encerrados poderão ser arquivados sem impactar consultas operacionais.
# **30. Consulta**
O Portal deverá permitir consultas utilizando filtros.
Exemplos:
Período.
Categoria.
Severidade.
Node.
Estado.
Texto livre.
Operador responsável pelo reconhecimento.
# **31. Princípios**
O sistema de alertas deverá produzir poucas notificações, porém altamente relevantes.
A ausência de alertas deverá significar que a plataforma encontra-se operando normalmente.
O excesso de alertas deverá ser tratado como um problema de configuração da plataforma, e não como comportamento esperado.
# **32. Integração**
O sistema de alertas deverá integrar-se naturalmente aos demais componentes da plataforma.
Fluxo simplificado:
Métricas
Eventos
↓
Alert Service
↓
Database
↓
Portal
O Alert Service não deverá depender diretamente dos Agents.
# **33. Dashboard de Alertas**
O Portal deverá disponibilizar um painel dedicado aos alertas ativos.
Informações mínimas:
- quantidade por severidade;
- quantidade por categoria;
- alertas recentes;
- alertas reconhecidos;
- alertas críticos;
- histórico recente.
O objetivo é permitir rápida identificação de problemas prioritários.
# **34. Correlação**
Sempre que possível, alertas relacionados deverão compartilhar contexto.
Exemplos:
Mesmo Node.
Mesmo disco.
Mesmo Torrent Engine.
Mesmo período.
Essa correlação facilitará o diagnóstico pelo operador, sem alterar a individualidade dos eventos registrados.
# **35. Auditoria**
As seguintes operações deverão gerar registros de auditoria:
Reconhecimento.
Fechamento manual.
Alteração de severidade.
Alteração de configuração.
Esses registros permanecerão separados dos próprios alertas.
# **36. Configuração**
Os critérios utilizados pelo Alert Service deverão ser configuráveis.
Exemplos:
Limites.
Tempos.
Retenção.
Categorias.
Políticas.
A configuração permanecerá centralizada no Control Plane.
# **37. Disponibilidade**
A indisponibilidade temporária do Alert Service não deverá interromper:
- operação dos Nodes;
- Scheduler;
- Control Plane;
- Torrent Engine.
O impacto limitar-se-á à geração e atualização de alertas.
# **38. Performance**
Objetivos da versão 1.0:
Baixo consumo de CPU.
Baixo consumo de memória.
Baixa latência.
Avaliação contínua das regras sem impacto perceptível ao restante da plataforma.
# **39. Escalabilidade**
O Alert Service deverá suportar crescimento gradual do Cluster.
O aumento do número de Nodes, Torrents ou eventos não deverá exigir mudanças na arquitetura do componente.
Novas categorias de alerta poderão ser adicionadas sem alterar os contratos existentes.
# **40. Testabilidade**
As regras de alerta deverão ser testáveis isoladamente.
Cada condição deverá produzir sempre o mesmo resultado quando submetida ao mesmo conjunto de informações.
Testes automatizados deverão validar:
- ativação;
- encerramento;
- agrupamento;
- supressão;
- severidade.
# **41. Resumo da Arquitetura**
Prometheus
│
│
Métricas
│
▼
┌─────────────────┐
│  Alert Service  │
└─────────────────┘
▲
│
Eventos
│
Control Plane
│
▼
PostgreSQL
│
▼
Portal
│
▼
Operador
O Alert Service consolida informações operacionais e identifica condições que exigem atenção do operador.
# **42. Boas Práticas**
O sistema deverá seguir os seguintes princípios:
- priorizar qualidade em vez de quantidade de alertas;
- evitar notificações repetitivas;
- privilegiar causa raiz quando identificável;
- manter histórico completo;
- permitir auditoria;
- facilitar investigação.
# **43. Declaração Final**
O sistema de Alertas do SeedGrid transforma métricas e eventos em informações acionáveis.
Sua função é destacar apenas condições que merecem atenção do operador, reduzindo ruído operacional e permitindo respostas rápidas a problemas reais.
Ao permanecer desacoplado dos mecanismos de coleta, execução e monitoramento, o Alert Service preserva a modularidade da arquitetura e reforça a separação de responsabilidades estabelecida em toda a plataforma.

---

# **44. Contrato do webhook — bloco `data` versionado (#147)**

O canal **webhook** (ADR-028) faz `POST` do `notify.Message` inteiro em JSON. Além dos
campos que os canais de texto renderizam, alertas que existem para **disparar automação**
levam um bloco `data` estruturado, versionado por `schema`.

## Por que `data` e não `fields`

`fields` é uma lista `{label, value}` **para humano ler** — é o que o Telegram e o Discord
transformam em texto, e muda sempre que alguém melhora uma frase. Automação que dependesse
dele quebraria a cada ajuste de copy. `data` é o contrato de máquina e não muda por
estética.

## Versionamento

`schema` é inteiro e sobe **apenas em mudança quebrante**:

- campo novo dentro de `data` → **não** sobe;
- remover campo, ou mudar o significado de um existente → **sobe**.

O consumidor deve checar o número e **falhar alto** se não reconhecer, em vez de processar
errado em silêncio. Mensagem sem bloco estruturado omite `schema` e `data` — quem já
consome o webhook hoje não vê diferença.

## `NODE_IPV6_CHANGED` — `schema: 1`

```json
{
  "kind": "alert",
  "type": "NODE_IPV6_CHANGED",
  "severity": "WARNING",
  "title": "IPv6 do Node mudou — as regras de firewall que liberavam … não valem mais",
  "node": "seedgrid-node-01",
  "fields": [
    { "label": "Anterior", "value": "2001:db8:601:d300:be24:11ff:fe12:dee1" },
    { "label": "Atual",    "value": "2001:db8:60b:c000:be24:11ff:fe12:dee1" },
    { "label": "Porta BT", "value": "6881" }
  ],
  "at": "2026-09-07T04:12:33Z",

  "schema": 1,
  "data": {
    "event": "node_ipv6_changed",
    "node": {
      "id": "b4f312bb-cd99-4463-9fd3-276e626b17ff",
      "hostname": "seedgrid-node-01",
      "bt_port": 6881
    },
    "ipv6": {
      "previous":        "2001:db8:601:d300:be24:11ff:fe12:dee1",
      "current":         "2001:db8:60b:c000:be24:11ff:fe12:dee1",
      "prefix_previous": "2001:db8:601:d300::/64",
      "prefix_current":  "2001:db8:60b:c000::/64"
    }
  }
}
```

### `prefix_*` vem calculado pelo Control Plane, de propósito

O que a regra de firewall precisa é o **objeto de rede `/64`**, não o endereço completo —
e foi exatamente o endereço **literal** que causou o incidente de 05-06/09/2026: as regras
sobreviveram à rotação do prefixo apontando para endereços que já não existiam. Deixar o
consumidor derivar o prefixo com manipulação de string plantaria o mesmo bug do outro lado
da integração.

`prefix_previous`/`prefix_current` vêm `""` quando o endereço não pôde ser reduzido com
segurança. **Prefixo errado é pior que prefixo nenhum**: uma regra escrita com `/64`
incorreto abre o que não devia, ou fecha o que devia abrir, silenciosamente. Nesse caso o
consumidor ainda tem o endereço completo e decide o que fazer.

### Para quem consome (n8n)

1. filtrar por `type == "NODE_IPV6_CHANGED"`;
2. **verificar `schema == 1`** e abortar com erro visível se for outro;
3. usar `data.ipv6.prefix_current` para reescrever a regra do gateway, e
   `data.node.bt_port` para a porta;
4. não parsear `title`, `message` nem `fields` — eles mudam.

> **Com o sufixo determinístico (ADR-038) o endereço novo é aritmética.** Um Node que aplicou a convenção tem o endereço `<prefixo /64>::5eed:<N>`, onde `N = bt_port − 6880` (decimal no último grupo: o Node 12 é `::5eed:12`). Depois de uma rotação de prefixo sobra **uma** incógnita — o `prefix_current` — e o endereço de cada Node é `prefix_current` + `::5eed:` + `N`; sem a convenção, o sufixo vinha do MAC e cada endereço tinha de ser descoberto, Node a Node. O token fixa só o sufixo: a rotação de prefixo **continua** invalidando as regras, e este alerta continua sendo a forma de saber.

O alerta **resolve sozinho** quando o Node volta a receber conexão de entrada depois da
troca (ver §Regras, `NODE_IPV6_CHANGED`), ou **expira** após `alerting.reachability_ttl`
(§46), então a automação não precisa fechar nada. Expirar **não** entrega webhook: a
automação recebe uma chamada por rotação (mais uma a cada rotação seguinte, #209) e
nenhuma de fechamento por validade.

### Rotação seguinte com o alerta ainda `OPEN` (#209)

Um Node sem demanda no swarm pode não receber entrada por dias — o alerta fica `OPEN`
esperando o resolve. Se o IPv6 rotacionar **de novo** nesse intervalo, o webhook dispara
outra vez, com `data.ipv6.current` já refletindo o endereço MAIS RECENTE (não o que abriu
o alerta originalmente). Não é uma condição nova: o Control Plane **atualiza** o alerta
existente em vez de abrir uma segunda linha, e notifica de novo pelo mesmo caminho do
`AlertOpened` — nunca pelo de resolução, porque nada foi corrigido. A automação deve tratar
cada entrega deste webhook como "o endereço vigente agora é este", não assumir que só
recebe uma vez por incidente.


---

# **45. Resolução também notifica (#161)**

Todo alerta que o Alert Worker fecha passa pelo `alert.Service`, que é quem dispara o
hook `AlertResolved` da telemetria (ADR-028). Antes as cinco regras fechavam com `UPDATE`
direto na tabela: o tunável `notify.alerts_on_resolve` vinha **ligado por padrão** e
prometia "avisa quando resolver", mas `Service.Resolve` não tinha um único chamador em
produção. Abrir notificava; resolver, não — sem erro em log e sem nada no Portal
indicando isso.

O helper `resolveVia` centraliza o fechamento: seleciona os Nodes que satisfazem a
condição de resolve e chama `Service.Resolve` **um por Node**. É uma query a mais por
alerta fechado, e é o que permite ao Service devolver severidade, mensagem e payload de
cada alerta — sem eles a notificação sairia sem hostname nem motivo.

Vale para `NODE_OFFLINE`, `NODE_STORAGE`, `NODE_DISK_FULL`, `DISK_IO_SATURATED`,
`NODE_FIREWALLED`, `NODE_INBOUND_SILENT` e `NODE_IPV6_CHANGED`.

⚠️ **Rajada:** queda geral vira volta geral, e o resolve de todos os Nodes sai junto. O
teto `notify.rate_limit_per_min` (por canal) é o que segura isso; um canal com teto baixo
descarta os excedentes em vez de enfileirar.

Exceção: o fechamento por **validade** (§46) não notifica.


---

# **46. Validade dos alertas de alcançabilidade (#211)**

`NODE_FIREWALLED`, `NODE_INBOUND_SILENT` e `NODE_IPV6_CHANGED` deduzem que o Node está
inalcançável porque **nenhuma conexão de entrada chegou**, e só resolvem quando uma
chega. Num Node sem demanda no swarm ela nunca chega, com a porta aberta ou não, e o
alerta ficava aberto para sempre. Medido em 24/09/2026: quatro alertas abertos, todos
falsos — a porta de cada Node verificada de fora, aberta; um dos Nodes com 0 leechers
nos 116 torrents.

O que importa nesses alertas é a **chamada para fora** na abertura (webhook, Telegram,
Discord). Depois de `alerting.reachability_ttl` (default 24h; 0 = sem validade) o
alerta **expira**:

- fecha como `RESOLVED` com `expired = true`, **sem notificar** — "Resolvido" seria
  mentira, nada foi corrigido. O Portal mostra *expirou*;
- a validade conta desde a **última notificação** (`updated_at`): uma rotação nova com o
  alerta aberto notifica de novo e a renova;
- o **mesmo episódio não reabre**. Cada alerta grava em `episode_key` a identidade da
  condição — a rotação (`public_ipv6_changed_at`), o início do silêncio
  (`last_incoming_at`), o veredito do Engine (`connectable_since`) — e um episódio que já
  gerou alerta, em qualquer status, não gera outro. Sem isso a condição, que continua
  valendo, reabriria o alerta na passada seguinte e notificaria a cada ciclo da
  validade. O mesmo mecanismo acaba com a re-notificação a cada **reboot** (sair de
  ONLINE fecha o alerta; a volta o reabria);
- a expiração roda mesmo com a regra desligada, para não deixar órfão.

**Descartado:** confirmar a porta com uma sonda externa antes de fechar. É a medida
certa, mas exigiria que todo operador mantivesse um sistema falando com um endpoint de
integração do Control Plane — um usuário comum não vai fazer isso, e o alerta não pode
depender de algo que a maioria das instalações não tem.

---

# **47. Saturação de disco sustentada (#150)**

`DISK_IO_SATURATED` abria com **uma** leitura acima do limiar (`alerting.disk_saturated_pct`,
90). A leitura é `nodes.disk_io_util_pct`, a média de ~10s entre dois heartbeats, e um HD USB é
serrilhado por natureza — 87% num intervalo, 13% cinco minutos depois (node-04, 06/09/2026).
Um pico de segundos virava alerta; medido no banco de produção, dos 18 alertas abertos entre
12/09 e 02/10/2026, **16 duraram exatamente 30s** — uma passada do Alert Worker; os outros dois
duraram 9min (node-03) e 4m30 (node-02), o tipo de episódio que a regra continua pegando se a
saturação se sustentar. O gráfico do Portal
(Prometheus, janela de 2 minutos) nunca mostrava esses picos, e o operador aprendia a ignorar
um alerta que parecia falso — o pior desfecho possível para um alerta.

Agora o alerta só abre depois que o disco fica **continuamente** acima do limiar por
`alerting.disk_saturated_for` (default 90s; 0 = abre na primeira leitura, como antes):

- o Node precisa estar ≥ limiar em **todas** as passadas do worker durante a carência. Uma
  passada abaixo do limiar — ou com o Node fora de ONLINE ou sem heartbeat recente — **zera a
  contagem**;
- a granularidade é a do worker (`alerting.worker_interval`, 30s): com os defaults a abertura
  sai na 4ª leitura consecutiva. A comparação tem folga de 1/10 do intervalo, para o jitter de
  milissegundos das passadas não empurrar a abertura uma passada inteira para frente quando a
  carência é múltipla do intervalo;
- **a resolução não mudou**: sem carência (disco que aliviou é boa notícia) e com a histerese de
  15pp. Uma leitura entre a folga e o limiar mantém o alerta aberto mas não conta como saturada,
  então reabrir depois de resolver exige uma sequência nova inteira;
- o início da sequência vive **em memória** no Worker (`satSince`), por Node. Reiniciar o
  Controller zera a contagem — só **atrasa** o alerta em até a carência, nunca cria um falso. Alerta
  já aberto está no banco e não é afetado.

**Descartado (decisão do operador, 03/10/2026):** persistir `disk_io_util_pct` em `heartbeats`
(migration, para haver histórico da série que abre o alerta) e exibir essa série no card do
Node. A causa do ruído era a abertura com uma leitura, que a regra sustentada resolve; o
histórico seria um recurso à parte. O estado em memória dispensa migration e não toca o caminho
quente do heartbeat — o carimbo persistido (como o `storage_failing_since` da #177) só ganharia
a visão de cada heartbeat de 10s, enquanto o worker já enxerga o sinal a cada 30s.

---

# **48. NODE_INBOUND_SILENT: demanda ponderada e texto que não envelhece (#206, #203)**

`NODE_INBOUND_SILENT` deduz que o Node ficou inalcançável porque nenhuma conexão de entrada
chegou há `alerting.incoming_silence`, **havendo demanda**. Cobre o caso que o
`NODE_FIREWALLED` não pega — o Node que *era* alcançável e deixou de ser, sem trocar de
endereço —, porque o veredito do Engine é grudento (ver `25 - Business Rules.md`). Por isso ele fica; o que mudou
é a **precisão**.

A demanda era "existe um leecher em qualquer torrent do Node, no instante da passada", e isso
trata como a mesma coisa dois cenários sem relação: um leecher num swarm carente *precisa* do
Node; um leecher num swarm com 63 seeds abre conexão com meia dúzia deles, e não ser um dos
escolhidos durante horas é o comportamento esperado de um Node saudável. Medido em 09/09/2026,
os seis torrents que armavam o alerta em Nodes com a porta verificada de fora e aberta tinham
de 2 a 63 seeds por leecher. No acumulado, dos 27 alertas abertos entre 08/09 e 30/09/2026 num
cluster de quatro Nodes, **só o primeiro tem um problema real documentado**: 14 fecharam porque
o Node saiu de ONLINE, 2 expiraram (§46) e 11 porque uma conexão de entrada chegou.

## 48.1 Demanda ponderada pela concorrência

Um torrent só conta como demanda quando

```
num_leechers > 0   e   num_seeds <= num_leechers × alerting.incoming_demand_seeds_per_leecher
```

O default é **1**: os leechers são ao menos tão numerosos quanto os seeds — há mais bocas do que
quem sirva. `num_seeds` é o total do swarm segundo o tracker e pode já incluir o próprio Node,
daí 1 leecher / 1 seed **armar** (o único seed provavelmente somos nós) e 1 / 2 não.

| Swarm (leechers / seeds) | Conta com o default? |
|---|---|
| 1 / 0 | **sim** — o leecher não tem de quem baixar |
| 1 / 1 | **sim** |
| 3 / 3 | **sim** — a igualdade conta (a regra é `<=`) |
| 1 / 2, 3 / 17, 1 / 63 | não |
| `num_leechers` ou `num_seeds` NULL | não — desconhecido nunca acusa |

- A demanda é do **Node**: basta **um** swarm carente entre os torrents dele. Os saturados e os
  sem leecher ficam fora da conta.
- O tunável é decimal. `0` é a leitura mais estrita (só swarm **sem nenhum seed**), não
  "desligado"; um valor muito alto (ex.: 100000) volta ao comportamento antigo, em que qualquer
  leecher conta. A regra inteira continua se desligando por `alerting.incoming_silence = 0`.
- O alerta registra no payload só o que o armou (`demand_torrents`, `demand_leechers`,
  `demand_seeds`) e o log da abertura repete os três. Não há histórico de peers no banco; esse é
  o único rastro de *por que* abriu, e o que permitirá recalibrar o tunável com dado de campo.
- **Limite conhecido, mantido:** a demanda é avaliada no *instante* da passada, não acumulada
  sobre a janela. A ponderação reduz o ruído desse limite, não o elimina — um leecher que acabou
  de chegar a um swarm carente ainda arma uma regra cuja janela já estava vencida. Acumular a
  demanda exige coluna nova e fica para quando o sinal amadurecer.

**Calibração do default (dados de produção, somente leitura).** Razão seeds/leecher dos
torrents que armavam a regra em 09/09/2026: 2, 63, 20, 5,7, 15 e 16 — todos em Nodes saudáveis;
com `1`, **nenhum** arma. Em 03/10/2026, entre 1.131 torrents, seis tinham leecher, com razões
0, 1, 4, 5,5, 5,5 e 24; com `1` armam os dois primeiros (os swarms em que o Node é de fato
necessário). Um default mais frouxo (2 a 5) readmitiria justamente a faixa que gerou o ruído.

## 48.2 Janela: default de 12h para 240h

`alerting.incoming_silence` já tinha sido remendado de 12h para 48h em 09/09/2026 e seguia
ruidoso. A janela só faz sentido se for maior que o silêncio **normal** de um Node saudável,
então ele foi medido. Reconstruindo, a partir dos alertas de produção, os episódios de silêncio
de Nodes sem problema de porta conhecido (a checagem externa, nas datas em que foi feita,
encontrou a porta aberta) — alertas reabertos depois de o Node cair e voltar são o **mesmo**
silêncio, e foram encadeados —, o silêncio total até chegar uma conexão de entrada foi, em horas:

| 49 | 61 | 62 | 86 | 111 | 177 | 217 | 221 | ainda sem entrada: mais de 21 dias |
|---|---|---|---|---|---|---|---|---|

Os valores de 86, 111 e 177h são pisos (a janela somada ao tempo em que o alerta ficou aberto).
Sobre esses nove episódios, uma janela de 48h alertava em **todos**; 96h, em cinco; 168h, em
quatro; **240h, em um** — o Node que segue sem entrada. Um Node saudável passa de 2 a 9 dias sem
receber nenhuma conexão.

O custo é detectar em até 10 dias em vez de 2. É aceitável porque a causa que originou a
família de regras (rotação do prefixo IPv6) tem alerta próprio e imediato
(`NODE_IPV6_CHANGED`); este é a rede de segurança genérica, e uma rede de segurança lenta e
precisa vale mais do que uma rápida que o operador aprende a ignorar. Um Node ocioso por
natureza não se separa de um Node quebrado só pelo silêncio — para esse caso a regra depende da
demanda ponderada (§48.1) e da validade (§46). O valor gravado no banco (painel de
Configurações) prevalece sobre o default; só quem nunca o ajustou muda de comportamento.

## 48.3 Texto que não envelhece

O texto do alerta é gravado **uma vez**, na abertura, e o alerta pode ficar aberto por dias.
Endereço e duração dentro dele envelhecem: o endereço IPv6 de um Node mudou quase 38h depois de o
alerta abrir e o painel seguiu mandando liberar um endereço que não existia mais; o "há
12h0m22s" ficava congelado no valor da abertura.

A mensagem agora é fixa e neutra — *Node sem conexão de entrada além do tolerado, apesar de haver
demanda — pode estar inalcançável: confira se a porta BT do Node está liberada para o endereço
atual dele* — e o que varia vai no **payload**: `hostname`, `public_ipv6` e `bt_port` (do momento
da abertura), `last_incoming_at` (o início do silêncio) e a demanda.

- **Portal:** a tela *Alertas & Eventos* mostra, ao lado do alerta **aberto**, o endereço
  `[ipv6]:porta` **atual** do Node, lido do próprio Node (que a página já recarrega a cada ciclo),
  não do payload. Some quando o alerta fecha ou o Node não reportou endereço/porta.
- **Notificação** (Telegram, Discord, webhook): ela é um instante, então leva o endereço do
  payload como campo *Endereço do Node* e o início do silêncio como *Última entrada*. O bloco
  `data` versionado (§44) não muda: continua só no `NODE_IPV6_CHANGED`.
- `NODE_IPV6_CHANGED` é diferente: o endereço faz parte da condição, e o alerta é atualizado a
  cada rotação (#209). `NODE_FIREWALLED` ainda carrega o endereço no texto — mesma classe de
  problema, fora desta entrega.

Um alerta já aberto antes do deploy mantém o texto antigo até fechar ou expirar (§46); a
mensagem não é reescrita.

**Descartado (decisão do operador, 03/10/2026):**

- **Aposentar o alerta.** Ele cobre um caso real — o Node que deixou de ser alcançável sem trocar
  de endereço — que nenhum outro alerta pega.
- **Sonda externa de porta** (promover a checagem de fora a sinal do Control Plane). Depende de
  serviço de terceiro, e o Control Plane, de dentro da rede do Node, não mede de fora.
- **Sonda de saída IPv6 no Agent** (#203), para escolher entre "confira a liberação" e "o Node
  não sai por IPv6". A causa do incidente que a motivou foi resolvida pelo sufixo IPv6
  determinístico (#202); a sonda seria custo permanente (coluna, heartbeat, métrica) para um
  caso que deixou de ocorrer.

---

# **49. NODE_FIREWALLED: texto que não envelhece (#223)**

`NODE_FIREWALLED` abre para Nodes cujo Engine reporta `firewalled` há mais que a carência
(`alerting.firewalled_grace`): ninguém de fora consegue abrir conexão com a porta BT. É a
regra de onboarding — o Node cuja porta nunca foi liberada nasce `firewalled` e nunca sai.

O texto do alerta era gravado **uma vez**, na abertura, e podia ficar aberto por dias. Tudo
o que ele dizia e que muda com o tempo envelhecia junto: o endereço IPv6 (o provedor rotaciona
o prefixo e o painel segue mandando liberar um endereço que já não existe) e o "há Xs"
(congelado no valor da abertura). É a mesma classe de problema que a §48.3 corrigiu no
`NODE_INBOUND_SILENT` (#203), e o tratamento é o mesmo.

## 49.1 Texto constante

A mensagem agora é fixa e neutra — *Engine do Node reporta firewalled além da carência
— ninguém de fora consegue abrir conexão com a porta BT: confira se a porta BT do Node está
liberada para o endereço atual dele* — e o que varia vai no **payload**: `hostname`,
`public_ipv6` e `bt_port` (do momento da abertura) e `connectable_since` (o início do
veredito, análogo ao `last_incoming_at` do `NODE_INBOUND_SILENT`).

- **Portal:** a tela *Alertas & Eventos* mostra, ao lado do alerta **aberto**, o endereço
  `[ipv6]:porta` **atual** do Node, lido do próprio Node (que a página já recarrega a cada
  ciclo), não do payload. Some quando o alerta fecha ou o Node não reportou endereço/porta.
- **Notificação** (Telegram, Discord, webhook): ela é um instante, então leva o endereço do
  payload como campo *Endereço do Node* e o início do veredito como *Firewalled desde*. O
  bloco `data` versionado (§44) não muda: continua só no `NODE_IPV6_CHANGED`.
- `NODE_IPV6_CHANGED` é diferente: o endereço faz parte da condição, e o alerta é atualizado
  a cada rotação (§44, #209).

Um alerta já aberto antes do deploy mantém o texto antigo até fechar ou expirar (§46); a
mensagem não é reescrita.

# **50. Idioma do texto de alertas e eventos (ADR-041)**
O texto de um alerta, evento ou atividade é uma **apresentação** de `code` + `payload`, montada por quem exibe, no idioma de quem lê (ADR-041). É a generalização do que as seções 48 e 49 fizeram para o endereço do Node: o dado mora no `payload`, não na frase.

- O **Portal** monta título e frase pelo catálogo do idioma ativo, a partir do `code` e do `payload`.
- O Control Plane continua gravando `message`, no idioma padrão do cluster, como **reserva**: é o que lê quem consome a API sem catálogo, e o que o Portal mostra para um código que ainda não conhece. Nenhum dado pode existir só no `message`.
- As **notificações** saem do Control Plane no idioma do cluster (`notify.language`), pelo mesmo conjunto de chaves.
- Regra para quem cria um alerta ou evento novo: defina o `code`, ponha no `payload` **tudo** o que a frase precisa, e acrescente a entrada nos catálogos — um teste de contrato reprova código sem entrada ou frase que peça um dado ausente.
