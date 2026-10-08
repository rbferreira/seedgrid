# **SeedGrid**
**Monitoring Specification**
Version: 1.0
Status: Draft
Document: 15 - Monitoring.md
# **1. Objetivo**
O sistema de monitoramento do SeedGrid é responsável por coletar, consolidar e apresentar informações operacionais sobre toda a plataforma.
Seu objetivo é fornecer visibilidade contínua da saúde do Cluster, permitindo identificar problemas, tendências e oportunidades de otimização.
# **2. Filosofia**
O monitoramento deverá responder às seguintes perguntas:
O Cluster está saudável?
Todos os Nodes estão operacionais?
Existe risco de indisponibilidade?
Existe risco de falta de espaço?
O Scheduler está funcionando corretamente?
Os serviços estão respondendo adequadamente?
# **3. Responsabilidades**
O sistema de monitoramento deverá:
- coletar métricas;
- consolidar informações;
- calcular indicadores;
- disponibilizar dashboards;
- apoiar geração de alertas.
O monitoramento NÃO deverá:
- executar ações corretivas;
- modificar configurações;
- tomar decisões administrativas.
Essas responsabilidades pertencem a outros componentes da plataforma.
# **4. Arquitetura**
Seed Nodes
│
Exporters
│
Prometheus
│
Grafana
│
Operador
O Control Plane também fornecerá métricas para o Prometheus.
# **5. Componentes**
A solução de monitoramento será composta por:
Prometheus.
Grafana.
Exporters.
Métricas do Control Plane.
Métricas do Agent.
# **6. Fontes de Dados**
As principais fontes de informações serão:
Control Plane.
SeedGrid Agent.
Node Exporter.
SMART Exporter.
Torrent Engine.
Sistema Operacional.
# **7. Prometheus**
Responsável por:
coletar métricas;
armazenar séries temporais;
disponibilizar consultas;
fornecer dados ao Grafana.
O Prometheus será considerado a fonte oficial de métricas da plataforma.
# **8. Visualização (Portal) — ADR-016**
> **Atualizado pela ADR-016:** o Grafana foi removido da stack. A visualização de
> métricas (gráficos, painéis, dashboards) passa a ser responsabilidade do **Portal**
> (produto). Para respeitar a ADR-005 (Portal só fala com a API), o **Control Plane**
> consulta o Prometheus (PromQL) no servidor e expõe as séries via `/api/v1/metrics/...`;
> o Portal apenas plota. Fluxo: Portal → API do Control Plane → Prometheus.
> O estado/saúde de domínio do Cluster continua vindo do Postgres via API.
> Alertas por limiar de métrica ficam a cargo do Alertmanager (webhook → Control Plane).
# **9. Exporters**
Cada Node deverá disponibilizar métricas através de Exporters apropriados.
Exemplos:
Node Exporter.
SMART Exporter.
Métricas do SeedGrid Agent.
Métricas do Torrent Engine.
# **10. Métricas do Cluster**
Indicadores sugeridos:
Nodes Online.
Nodes Offline.
Nodes em manutenção.
Capacidade total.
Espaço livre.
Quantidade de Torrents.
Upload agregado.
Download agregado.
Health médio.
Alertas ativos.
# **11. Métricas dos Nodes**
Cada Node deverá fornecer, no mínimo:
CPU.
Memória.
Disco.
Rede.
Temperatura.
Uptime.
Versão do Agent.
Versão do Engine.
Health Score.
# **12. Métricas do Torrent Engine**
Indicadores sugeridos:
Quantidade de Torrents.
Torrents ativos.
Seeds.
Peers.
Upload.
Download.
Conexões.
Uso de disco.
# **13. Dashboards**
O sistema deverá possuir dashboards específicos para:
Cluster.
Nodes.
Torrents.
Scheduler.
Infraestrutura.
Control Plane.
# **14. Atualização**
As métricas deverão ser atualizadas periodicamente.
A frequência deverá equilibrar:
precisão;
consumo de recursos;
tráfego de rede.
Os intervalos específicos serão definidos durante a implementação.
# **15. Objetivos**
O sistema de monitoramento deverá permitir diagnóstico rápido da plataforma sem necessidade de acesso direto aos Nodes.
# **16. Princípio Fundamental**
Toda decisão operacional deverá basear-se em informações confiáveis e atualizadas.
O sistema de monitoramento representa a visão operacional da plataforma e deverá refletir fielmente o estado do Cluster.
# **17. Organização das Métricas**
As métricas deverão ser organizadas por domínio.
Domínios previstos:
Cluster.
Control Plane.
Nodes.
Torrent Engine.
Storage.
Rede.
Scheduler.
API.
Portal.
Essa organização facilitará dashboards, consultas e futuras integrações.
# **18. Métricas do Cluster**
Indicadores consolidados.
Exemplos:
Quantidade de Nodes.
Nodes Online.
Nodes Offline.
Nodes em manutenção.
Capacidade total.
Espaço utilizado.
Espaço disponível.
Quantidade total de Torrents.
Upload agregado.
Download agregado.
Health médio.
Alertas ativos.
Eventos por hora.
# **19. Métricas do Control Plane**
Indicadores sugeridos:
Tempo de atividade.
Utilização de CPU.
Utilização de memória.
Quantidade de requisições.
Tempo médio de resposta.
Erros da API.
Sessões ativas.
Conexões com banco.
Tempo médio de consultas.
# **20. Métricas dos Nodes**
Cada Node deverá disponibilizar:
Carga média.
CPU.
Memória.
Swap.
Disco.
IOPS.
Temperatura.
Interfaces de rede.
Espaço disponível.
Uptime.
Health Score.
Último Heartbeat.
Versão do Agent.
# **21. Métricas de Armazenamento**
Por disco:
Capacidade.
Espaço utilizado.
Espaço livre.
Temperatura.
SMART.
Erros conhecidos.
Sistema de arquivos.
Tempo de atividade.
# **22. Métricas do Torrent Engine**
Indicadores sugeridos:
Torrents ativos.
Torrents pausados.
Torrents em download.
Torrents em seeding.
Seeds.
Peers.
Velocidade de upload.
Velocidade de download.
Uso de disco.
Conexões abertas.
# **23. Métricas do Scheduler**
Indicadores operacionais.
Exemplos:
Quantidade de decisões.
Tempo médio de decisão.
Tempo máximo.
Falhas.
Quantidade média de candidatos.
Node mais selecionado.
Distribuição de Torrents.
Versão do algoritmo.
# **24. Métricas da API**
Exemplos:
Requisições por minuto.
Latência.
Erros.
Autenticações.
Sessões.
Tempo médio por endpoint.
# **25. Dashboards**
O Grafana deverá disponibilizar dashboards especializados.
Dashboard Cluster
Visão geral.
Dashboard Nodes
Estado detalhado de cada Node.
Dashboard Storage
Saúde dos discos.
SMART.
Espaço.
Dashboard Torrents
Indicadores do Torrent Engine.
Dashboard API
Saúde do Control Plane.
Dashboard Scheduler
Distribuição das decisões.
# **26. Atualização**
As métricas deverão ser coletadas em intervalos apropriados.
Informações críticas poderão ser coletadas com maior frequência.
Inventários não deverão ser tratados como métricas.
# **27. Retenção**
O período de retenção das métricas deverá ser configurável.
A política de retenção deverá considerar:
Capacidade de armazenamento.
Necessidade operacional.
Objetivos de análise histórica.
# **28. Consultas**
O sistema deverá permitir consultas históricas.
Exemplos:
Uso de CPU nas últimas 24 horas.
Espaço disponível na última semana.
Health Score do último mês.
Upload agregado nas últimas horas.
Essas consultas serão realizadas diretamente sobre o Prometheus.
# **29. Exportação**
As métricas deverão estar disponíveis para integração com ferramentas compatíveis com Prometheus.
A arquitetura não deverá impedir futuras integrações com outros sistemas de observabilidade.
# **30. Princípios**
O monitoramento deverá fornecer informações suficientes para:
Diagnóstico.
Planejamento.
Capacidade.
Análise de desempenho.
Investigação de falhas.
Sem introduzir complexidade desnecessária ou consumo excessivo de recursos.
# **31. Health Score**
O Health Score representa um indicador consolidado da saúde operacional de um Node.
Seu objetivo é fornecer uma avaliação rápida da condição geral do equipamento.
O cálculo do Health Score será realizado pelo Control Plane utilizando as informações recebidas dos Agents.
# **Objetivos**
O Health Score deverá auxiliar:
- o operador;
- o Scheduler;
- o monitoramento da plataforma.
O valor não substitui métricas individuais, apenas resume a condição geral do Node.
# **32. Histórico**
As métricas deverão permitir análise histórica.
Exemplos:
- crescimento do armazenamento;
- evolução do upload;
- disponibilidade dos Nodes;
- utilização de CPU;
- utilização de memória.
O histórico será mantido exclusivamente pelo sistema de monitoramento.
# **33. Dashboards Recomendados**
# **Dashboard Executivo**
Visão geral do Cluster.
Indicadores consolidados.
Alertas ativos.
Capacidade disponível.
Health geral.
# **Dashboard Operacional**
Estado detalhado dos Nodes.
Engine.
Discos.
Rede.
Eventos recentes.
# **Dashboard de Capacidade**
Espaço disponível.
Utilização histórica.
Crescimento.
Distribuição dos Torrents.
# **Dashboard da Plataforma**
Control Plane.
API.
Portal.
Banco de Dados.
Scheduler.
Tempo de resposta.
# **34. Diagnóstico**
O monitoramento deverá permitir responder rapidamente perguntas como:
Qual Node está indisponível?
Qual disco está próximo da capacidade máxima?
Qual serviço deixou de responder?
Qual Node apresenta pior Health Score?
Qual componente apresenta maior consumo de recursos?
# **35. Correlação**
Sempre que possível, dashboards deverão permitir navegação entre informações relacionadas.
Exemplos:
Node → Torrents.
Node → Eventos.
Node → Alertas.
Torrent → Node.
Evento → Recurso relacionado.
Essa navegação reduz o tempo necessário para diagnóstico.
# **36. Consumo de Recursos**
O próprio sistema de monitoramento deverá possuir baixo impacto sobre a plataforma.
Objetivos:
Baixo consumo de CPU.
Baixo consumo de memória.
Baixo tráfego de rede.
Baixo consumo de armazenamento.
A coleta de métricas nunca deverá comprometer a operação normal dos Nodes.
# **37. Disponibilidade**
A indisponibilidade temporária do sistema de monitoramento não deverá afetar:
- o funcionamento dos Nodes;
- o Torrent Engine;
- o Scheduler;
- o Control Plane.
O monitoramento representa uma funcionalidade de observação e não participa diretamente da operação da plataforma.
# **38. Integração**
As métricas produzidas pelo SeedGrid deverão permanecer compatíveis com o ecossistema Prometheus.
Sempre que possível, serão utilizadas convenções amplamente adotadas pela comunidade.
Isso facilitará integrações futuras e reutilização de ferramentas existentes.
# **39. Evolução**
A arquitetura deverá permitir inclusão de novas métricas sem necessidade de alterações significativas nos componentes existentes.
Novos coletores poderão ser adicionados respeitando os contratos estabelecidos pelo Agent e pelo Control Plane.
# **40. Resumo da Arquitetura**
Seed Nodes
│
┌──────────┴──────────┐
│                     │
SeedGrid Agent      Exporters
│                     │
└──────────┬──────────┘
│
Prometheus
│
Grafana
│
Operador
O monitoramento complementa a plataforma fornecendo visibilidade contínua da operação do Cluster.
# **41. Declaração Final**
O sistema de monitoramento do SeedGrid foi projetado para fornecer uma visão clara, consolidada e confiável da plataforma.
Ele apoia o operador na identificação de problemas, no planejamento de capacidade e na análise do comportamento do Cluster, preservando o desacoplamento entre monitoramento, decisão e execução estabelecido pela arquitetura geral do SeedGrid.
