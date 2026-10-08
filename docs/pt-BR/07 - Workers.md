# SeedGrid

## Workers Specification

Version: 1.0

Status: Draft

Document: 07 - Workers.md

---

# 1. Objetivo

Este documento define a arquitetura dos Workers do SeedGrid.

Seu objetivo é estabelecer a organização oficial do processamento assíncrono da plataforma, definindo responsabilidades, limites e princípios que deverão ser preservados por qualquer implementação.

---

# 2. Filosofia

Os Workers representam componentes arquiteturais responsáveis pela execução de atividades assíncronas do Control Plane.

Sua finalidade é desacoplar tarefas de longa duração, periódicas ou não interativas dos serviços voltados ao operador.

Workers representam responsabilidades da arquitetura, e não tecnologias ou mecanismos específicos de implementação.

---

# 3. Definição

Um Worker é um componente lógico responsável pela execução de um conjunto específico de atividades assíncronas.

Cada Worker possui responsabilidade única e bem definida.

Os Workers não representam:

* processos;
* threads;
* containers;
* executáveis;
* serviços do sistema operacional.

Esses detalhes pertencem exclusivamente à implementação.

---

# 4. Escopo

Todos os Workers pertencem ao Control Plane.

Nenhum Worker deverá executar diretamente nos Seed Nodes.

Toda interação com os Seed Nodes deverá ocorrer exclusivamente através do Agent e dos canais oficiais de comunicação definidos pela arquitetura.

---

# 5. Arquitetura da V1

A versão 1.0 do SeedGrid define cinco Workers oficiais.

Esses Workers representam a arquitetura oficial da plataforma.

São eles:

* Inventory Worker;
* Monitoring Worker;
* Alert Worker;
* Scheduler Worker;
* Maintenance Worker.

Novos Workers não fazem parte da arquitetura da V1.

---

# 6. Inventory Worker

Responsável pela atualização do inventário do Cluster.

Suas responsabilidades incluem:

* processamento das informações enviadas pelos Agents;
* atualização do inventário dos Seed Nodes;
* atualização das capacidades conhecidas;
* sincronização do Estado do Cluster relacionado ao inventário.

O Inventory Worker não executa decisões administrativas.

---

# 7. Monitoring Worker

Responsável pelo processamento das informações operacionais da plataforma.

Exemplos:

* consolidação de métricas;
* atualização dos estados operacionais;
* cálculo de indicadores;
* disponibilização de informações para observabilidade.

O Monitoring Worker não gera Alertas diretamente.

---

# 8. Alert Worker

Responsável pela avaliação das condições que originam Alertas.

Suas responsabilidades incluem:

* análise de métricas;
* análise de Eventos;
* análise do Estado do Cluster;
* abertura de Alertas;
* encerramento de Alertas.

O Alert Worker não envia notificações diretamente aos operadores.

---

# 9. Scheduler Worker

Responsável pela execução operacional das decisões produzidas pelo Scheduler.

Exemplos:

* processamento de solicitações de distribuição;
* aplicação das decisões do Scheduler;
* atualização do Estado do Cluster após a execução das operações.

As regras de decisão permanecem responsabilidade do Scheduler.

---

# 10. Maintenance Worker

Responsável pelas atividades administrativas periódicas da plataforma.

Exemplos:

* limpeza de informações temporárias;
* reconciliação de dados;
* verificações periódicas;
* rotinas administrativas agendadas.

Todas as tarefas deverão ser idempotentes sempre que tecnicamente possível.

---

# 11. Comunicação

Os Workers deverão comunicar-se apenas através dos componentes definidos pela arquitetura.

Exemplos:

* Banco de Dados;
* Scheduler;
* API interna;
* serviços internos do Control Plane.

Workers nunca deverão comunicar-se diretamente com o Portal.

---

# 12. Estado

Workers não deverão manter Estado persistente próprio.

Toda informação permanente deverá permanecer no Estado oficial do Cluster.

Workers poderão utilizar apenas informações temporárias necessárias à execução das tarefas em andamento.

---

# 13. Objetivos

A arquitetura dos Workers deverá:

* organizar o processamento assíncrono;
* reduzir acoplamento;
* facilitar manutenção;
* favorecer evolução controlada;
* manter responsabilidades claramente definidas.

---

# 14. Princípio Fundamental

A arquitetura define quais Workers existem e quais responsabilidades pertencem a cada um deles.

A implementação possui liberdade para definir como esses Workers serão executados, desde que preserve integralmente a arquitetura estabelecida neste documento.



---

# 15. Modelo de Execução

Os Workers deverão executar suas atividades de forma independente dos serviços interativos da plataforma.

Uma tarefa em execução por um Worker não deverá bloquear:

* Portal;
* API;
* Scheduler;
* demais Workers.

A arquitetura deverá privilegiar processamento assíncrono sempre que apropriado.

---

# 16. Agendamento

Cada Worker poderá executar tarefas:

* sob demanda;
* periodicamente;
* em resposta a Eventos internos.

O mecanismo de agendamento pertence à implementação e deverá permanecer transparente para a arquitetura.

---

# 17. Filas de Trabalho

Quando uma tarefa não puder ser executada imediatamente, ela poderá ser colocada em uma fila interna.

As filas representam um mecanismo de implementação e não fazem parte da arquitetura oficial.

Independentemente da tecnologia utilizada, a plataforma deverá preservar a ordem lógica e a integridade das tarefas.

---

# 18. Idempotência

Sempre que tecnicamente possível, as operações executadas pelos Workers deverão ser idempotentes.

A reexecução de uma tarefa não deverá produzir efeitos colaterais inesperados ou inconsistências no Estado do Cluster.

Quando a idempotência não for possível, a limitação deverá ser documentada.

---

# 19. Recuperação de Falhas

Falhas durante a execução de uma tarefa deverão ser tratadas de forma controlada.

A arquitetura prevê:

* registro do erro;
* possibilidade de nova execução;
* preservação da consistência do Estado do Cluster.

A estratégia de retry pertence à implementação.

---

# 20. Concorrência

A arquitetura deverá permitir a execução simultânea de diferentes Workers.

Entretanto, uma mesma responsabilidade arquitetural deverá preservar a consistência de suas operações.

Quando necessário, mecanismos de sincronização poderão ser utilizados pela implementação.

---

# 21. Monitoramento

Cada Worker deverá disponibilizar informações para observabilidade.

Informações mínimas recomendadas:

* estado atual;
* quantidade de tarefas executadas;
* quantidade de falhas;
* tempo médio de execução;
* última execução concluída.

---

# 22. Logs

Toda execução relevante deverá produzir logs estruturados.

Os registros deverão permitir responder, no mínimo:

* qual Worker executou a tarefa;
* quando iniciou;
* quando terminou;
* qual foi o resultado;
* quanto tempo levou.

---

# 23. Eventos

Os Workers poderão produzir Eventos quando ocorrerem alterações relevantes no Estado do Cluster.

Os Eventos deverão seguir os padrões definidos em **Glossary.md**.

Eventos não substituem logs nem alertas.

---

# 24. Alertas

Problemas relacionados aos Workers poderão originar Alertas.

Exemplos:

* falhas consecutivas;
* tempo de execução anormal;
* interrupção inesperada;
* tarefas acumuladas além dos limites definidos.

As regras de geração de Alertas deverão permanecer configuráveis.

---

# 25. Auditoria

Quando um Worker executar alterações administrativas no Estado do Cluster, essas alterações deverão gerar registros de auditoria conforme definido em **Audit** e **Glossary.md**.

A execução automática de uma tarefa não elimina a necessidade de rastreabilidade.

---

# 26. Princípios

A execução dos Workers deverá preservar:

* previsibilidade;
* isolamento;
* rastreabilidade;
* consistência;
* tolerância a falhas;
* observabilidade.

Esses princípios deverão orientar qualquer implementação da camada de processamento assíncrono.



---

# 27. Segurança

Os Workers deverão operar utilizando apenas os privilégios necessários para execução de suas responsabilidades.

Nenhum Worker deverá possuir permissões administrativas superiores às exigidas por sua função.

Sempre que possível, deverá ser adotado o princípio do menor privilégio.

---

# 28. Configuração

As configurações utilizadas pelos Workers deverão permanecer externas ao código-fonte.

Exemplos:

* intervalos de execução;
* limites operacionais;
* parâmetros de processamento;
* políticas de retry.

Toda configuração deverá possuir documentação correspondente.

**Tunáveis dinâmicos (ADR-023).** Os parâmetros de comportamento dos Workers — intervalos
de execução, limiares do controlador de admissão (ADR-021) e das regras de alerta (ADR-008),
retenção de amostras — são **lidos por passada** de um catálogo dinâmico (`internal/settings`),
não capturados no boot. São editáveis no painel **Configurações** do Portal e valem **sem
redeploy** (hot-reload); o valor efetivo é o override no banco (tabela `settings`) sobre o
default do catálogo. O catálogo completo está em `25 - Business Rules.md`.

---

# 29. Disponibilidade

A indisponibilidade de um Worker não deverá comprometer imediatamente o funcionamento dos demais componentes da plataforma.

Quando um Worker estiver indisponível:

* o fato deverá ser registrado;
* o operador deverá ser informado quando aplicável;
* a recuperação deverá ocorrer de forma controlada.

A arquitetura deverá privilegiar degradação controlada em vez de falhas em cascata.

---

# 30. Evolução

A arquitetura dos Workers poderá evoluir em versões futuras.

Entretanto, alterações que modifiquem:

* responsabilidades;
* quantidade de Workers oficiais;
* fluxo de processamento;
* limites arquiteturais;

deverão ser precedidas por documentação correspondente e, quando aplicável, por uma nova ADR.

---

# 31. Relação com os Demais Documentos

Este documento complementa:

* Architecture.md;
* Scheduler.md;
* Control Plane.md;
* Monitoring.md;
* Alerting.md;
* Development Guide.md;
* Coding Standards.md;
* Glossary.md.

As responsabilidades aqui definidas prevalecem sobre interpretações implícitas presentes em outros documentos.

---

# 32. Resumo

A arquitetura de Workers do SeedGrid baseia-se nos seguintes princípios:

* responsabilidades arquiteturais bem definidas;
* processamento assíncrono desacoplado;
* ausência de Estado persistente local;
* observabilidade completa;
* tolerância a falhas;
* simplicidade de implementação.

Os mecanismos utilizados para implementar esses princípios permanecem livres, desde que respeitem os contratos definidos nesta especificação.

---

# 33. Declaração Final

Os Workers constituem a camada oficial de processamento assíncrono do SeedGrid.

Ao definir responsabilidades claras e independentes dos mecanismos de execução, a plataforma preserva sua arquitetura, facilita manutenção e permite diferentes estratégias de implementação sem comprometer a consistência do sistema.

A arquitetura estabelece **quais responsabilidades existem**.

A implementação define **como essas responsabilidades serão executadas**.

Essa separação representa um dos princípios fundamentais do SeedGrid.
