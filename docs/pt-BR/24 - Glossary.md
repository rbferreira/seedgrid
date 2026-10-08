# **SeedGrid**
# **Glossary**
Version: 1.0
Status: Draft
Document: 24 - Glossary.md
# **1. Objetivo**
Este documento define a terminologia oficial utilizada pelo SeedGrid.
Seu objetivo é garantir consistência entre documentação, implementação e operação da plataforma.
Todo termo definido neste glossário deverá possuir um único significado oficial.
# **2. Filosofia**
Cada conceito deverá possuir uma definição clara e inequívoca.
Termos diferentes não deverão ser utilizados para representar o mesmo conceito.
Da mesma forma, um mesmo termo não deverá representar conceitos distintos.
# **3. Cluster**
# **Definição**
Conjunto completo de componentes que formam uma instalação do SeedGrid.
Inclui:
- Control Plane;
- Seed Nodes;
- Portal;
- API;
- Scheduler;
- Banco de Dados;
- componentes de observabilidade.
# **Não utilizar para**
- apenas um Seed Node;
- apenas um grupo de torrents.
# **4. Control Plane**
# **Definição**
Componente central responsável pela administração do Cluster.
Responsabilidades:
- gerenciamento;
- persistência do Estado do Cluster;
- coordenação;
- autenticação;
- integração entre componentes.
# **Não utilizar para**
Portal.
API.
Scheduler.
Embora esses componentes façam parte do Control Plane, eles não representam o Control Plane isoladamente.
# **5. Seed Node**
# **Definição**
Equipamento responsável por executar o Torrent Engine e armazenar dados dos torrents.
Cada Seed Node representa uma unidade operacional independente.
# **Não utilizar para**
Agent.
Torrent Engine.
Disco.
O Seed Node engloba todos esses elementos.
# **6. Agent**
# **Definição**
Serviço executado em cada Seed Node responsável pela comunicação com o Control Plane.
Funções:
- Heartbeats;
- inventário;
- execução de comandos;
- coleta de informações locais.
# **Não utilizar para**
Torrent Engine.
Scheduler.
# **7. Scheduler**
# **Definição**
Componente responsável por decidir onde novas cargas de trabalho deverão ser executadas.
A decisão utiliza informações globais do Cluster.
# **Não utilizar para**
Balanceador de carga.
Torrent Engine.
# **8. Portal**
# **Definição**
Interface gráfica oficial da plataforma.
Permite administração e visualização do Cluster.
Toda comunicação ocorre através da API.
# **Não utilizar para**
API.
Control Plane.
# **9. API**
# **Definição**
Camada oficial de comunicação entre clientes e o Control Plane.
Toda interação administrativa deverá ocorrer através da API.
# **Não utilizar para**
Banco de Dados.
Portal.
Agent.
# **10. Torrent Engine**
# **Definição**
Aplicação responsável pela execução do protocolo BitTorrent em um Seed Node.
Sua responsabilidade limita-se às operações relacionadas aos torrents.
# **Não utilizar para**
Agent.
Scheduler.
Control Plane.
# **11. Estado do Cluster**
# **Definição**
Conjunto de informações persistentes que descrevem a situação administrativa do Cluster.
Exemplos:
- Nodes;
- usuários;
- permissões;
- configurações;
- eventos;
- alertas;
- auditoria.
O Estado do Cluster representa a principal informação protegida pela plataforma.
# **12. Heartbeat**
# **Definição**
Mensagem periódica enviada pelo Agent ao Control Plane para indicar disponibilidade.
Sua ausência pode indicar falha de comunicação ou indisponibilidade do Seed Node.
# **Não utilizar para**
Evento.
Alerta.
# **13. Health Score**
# **Definição**
Indicador calculado que representa a condição operacional de um Seed Node ou do Cluster.
É utilizado como apoio à tomada de decisões pelo Scheduler e pelo operador.
Não representa, isoladamente, uma condição de falha.
# **14. Princípio**
Os termos definidos neste documento representam a nomenclatura oficial do SeedGrid.
Toda documentação futura deverá utilizar estas definições.
# **15. Log**
# **Definição**
Registro técnico detalhado produzido por um componente durante sua execução.
Os logs destinam-se principalmente ao diagnóstico, depuração e investigação de problemas.
# **Características**
- alta frequência;
- baixo nível de abstração;
- orientado a desenvolvedores e operadores.
# **Não utilizar para**
- Alertas;
- Eventos de domínio;
- Auditoria.
# **16. Evento**
# **Definição**
Representação estruturada de um fato relevante ocorrido na plataforma.
Eventos descrevem acontecimentos importantes, independentemente de representarem problemas.
# **Exemplos**
- Node registrado.
- Torrent adicionado.
- Backup concluído.
- Atualização iniciada.
# **Características**
- estruturado;
- rastreável;
- utilizado para automação e histórico operacional.
# **Não utilizar para**
Logs.
Alertas.
# **17. Alerta**
# **Definição**
Notificação gerada quando uma condição operacional exige atenção do operador.
Alertas são derivados da análise de métricas, eventos e estados conhecidos da plataforma.
# **Exemplos**
- Disco quase cheio.
- Node Offline.
- Health Score crítico.
# **Características**
- baixa frequência;
- alta relevância;
- acionável.
# **Não utilizar para**
Logs.
Eventos.
# **18. Incidente**
# **Definição**
Situação operacional que impacta ou pode impactar a disponibilidade, integridade ou funcionamento da plataforma.
Um incidente poderá estar associado a um ou mais alertas.
# **Exemplos**
- Perda de um Seed Node.
- Corrupção do banco.
- Falha do Control Plane.
# **Características**
- representa um problema operacional;
- pode originar procedimentos de Disaster Recovery;
- normalmente possui início, tratamento e encerramento.
# **19. Auditoria**
# **Definição**
Registro permanente de ações administrativas executadas por operadores ou processos autorizados.
Seu objetivo é garantir rastreabilidade e responsabilização.
# **Exemplos**
- Alteração de configuração.
- Exclusão de usuário.
- Mudança de permissões.
- Execução manual de backup.
# **Características**
- imutável;
- persistente;
- voltado à governança.
# **Não utilizar para**
Logs técnicos.
Eventos operacionais.
# **20. Métrica**
# **Definição**
Valor numérico utilizado para medir o comportamento da plataforma ao longo do tempo.
Métricas apoiam monitoramento, geração de alertas e análise de desempenho.
# **Exemplos**
- Uso de CPU.
- Uso de memória.
- Latência da API.
- Torrents ativos.
- Espaço livre em disco.
# **Características**
- série temporal;
- coletada periodicamente;
- utilizada para análises quantitativas.
# **21. Relação entre Conceitos**
Os principais conceitos de observabilidade relacionam-se da seguinte forma:
Logs
│
▼
Eventos
│
▼
Alertas
│
▼
Incidentes
A Auditoria permanece paralela a esse fluxo, registrando ações administrativas.
As Métricas alimentam Eventos, Alertas e painéis de monitoramento, mas não representam acontecimentos por si só.
# **22. Operador**
# **Definição**
Pessoa responsável pela administração, operação e manutenção do Cluster.
O Operador utiliza o Portal e demais ferramentas oficiais da plataforma para monitorar, configurar e recuperar o ambiente.
# **23. Capacidade**
# **Definição**
Objetivo estratégico de evolução da plataforma descrito no Roadmap.
Uma capacidade representa um problema que se deseja resolver, independentemente da tecnologia utilizada para isso.
Uma capacidade poderá possuir diversas implementações possíveis ao longo da evolução do projeto.
# **24. Princípio**
Logs descrevem detalhes técnicos.
Eventos descrevem fatos.
Alertas descrevem condições que exigem atenção.
Incidentes descrevem problemas operacionais.
Auditoria descreve ações administrativas.
Métricas descrevem comportamento quantitativo da plataforma.
Esses conceitos representam categorias distintas e nunca deverão ser utilizados como sinônimos.
# **25. Node Capacity**
# **Definição**
Conjunto de recursos disponíveis em um Seed Node para execução de cargas de trabalho.
Pode considerar:
- armazenamento disponível;
- CPU;
- memória;
- largura de banda;
- limites administrativos.
A capacidade é utilizada como uma das entradas para o Scheduler.
# **26. Inventory**
# **Definição**
Conjunto de informações conhecidas pelo Control Plane sobre os recursos existentes no Cluster.
Inclui, entre outros:
- Seed Nodes;
- discos;
- interfaces de rede;
- versões de software;
- Torrent Engines;
- capacidades.
O inventário é atualizado periodicamente pelos Agents.
# **27. Maintenance Mode**
# **Definição**
Estado administrativo em que um componente permanece operacional, porém indisponível para novas cargas de trabalho.
Durante esse período, o componente poderá continuar sendo monitorado e administrado.
Seu objetivo é permitir intervenções planejadas sem interromper o restante do Cluster.
# **28. Scheduler Decision**
# **Definição**
Resultado produzido pelo Scheduler indicando onde determinada carga de trabalho deverá ser executada.
Uma decisão poderá considerar:
- Health Score;
- capacidade disponível;
- políticas do Cluster;
- restrições administrativas;
- estado operacional dos Nodes.
As decisões do Scheduler deverão ser rastreáveis.
# **29. Policy**
# **Definição**
Conjunto de regras administrativas que orientam o comportamento da plataforma.
Exemplos:
- políticas de distribuição;
- políticas de backup;
- políticas de retenção;
- políticas de manutenção.
As políticas representam configurações de alto nível definidas pelo operador.
# **30. Configuration**
# **Definição**
Conjunto de parâmetros utilizados para controlar o comportamento da plataforma.
Configurações podem existir em diferentes níveis:
- Cluster;
- componente;
- Node.
Configurações não representam decisões arquiteturais permanentes.
# **31. Capability**
# **Definição**
Objetivo estratégico registrado no Roadmap.
Uma Capability descreve **o problema que se pretende resolver**, independentemente da tecnologia utilizada.
Cada Capability poderá possuir uma ou mais implementações possíveis.
# **32. Component**
# **Definição**
Unidade lógica da arquitetura do SeedGrid com responsabilidade claramente definida.
Exemplos:
- Portal;
- API;
- Scheduler;
- Agent;
- Alert Service;
- Monitoring Stack.
Cada componente deverá possuir responsabilidade única e interfaces bem definidas.
# **33. Serviço**
# **Definição**
Implementação executável de um Component.
Um Component representa um conceito arquitetural.
Um Serviço representa sua implementação operacional.
Embora frequentemente exista relação de um para um, esses conceitos não são equivalentes.
# **34. Convenções Terminológicas**
Toda documentação oficial deverá utilizar exclusivamente os termos definidos neste Glossary.
Sinônimos deverão ser evitados quando puderem gerar ambiguidades.
Novos termos deverão ser adicionados a este documento antes de serem adotados oficialmente pela plataforma.
# **35. Evolução**
O Glossary deverá evoluir juntamente com a arquitetura.
Sempre que novos conceitos estruturais forem introduzidos, suas definições oficiais deverão ser registradas antes de sua utilização na documentação ou na implementação.
# **36. Relação com os Demais Documentos**
Este Glossary complementa toda a documentação oficial do SeedGrid.
Sempre que houver dúvida sobre o significado de um termo técnico, prevalecerá a definição estabelecida neste documento.
Os demais documentos deverão referenciar estes conceitos, evitando redefinições locais.
# **37. Resumo**
O Glossary estabelece a linguagem oficial do SeedGrid.
Seu objetivo é garantir que arquitetura, documentação, implementação e operação utilizem os mesmos conceitos, preservando clareza, consistência e rastreabilidade ao longo da evolução da plataforma.
# **38. Declaração Final**
A terminologia oficial do SeedGrid representa parte integrante de sua arquitetura.
Ao definir de forma precisa os conceitos utilizados pela plataforma, este Glossary reduz ambiguidades, facilita comunicação entre equipes e assegura que toda evolução futura preserve uma linguagem comum e consistente.
