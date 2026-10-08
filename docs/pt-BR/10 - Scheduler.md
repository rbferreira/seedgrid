# **SeedGrid**
**Scheduler Specification**
Version: 1.0
Status: Draft
Document: 10 - Scheduler.md
# **1. Objetivo**
O Scheduler é responsável por determinar em qual Node uma determinada operação deverá ser executada.
Na versão 1.0, sua principal responsabilidade será selecionar o Node mais adequado para receber um novo Torrent.
O Scheduler não executa operações.
O Scheduler toma decisões.
# **2. Filosofia**
O operador nunca deverá precisar decidir manualmente onde um Torrent será armazenado.
O Cluster deverá comportar-se como um único ambiente lógico.
A localização física dos Torrents é um detalhe de implementação.
# **3. Responsabilidades**
O Scheduler deverá:
- selecionar Nodes para novos Torrents;
- respeitar restrições do Cluster;
- considerar a saúde dos Nodes;
- distribuir carga de forma equilibrada;
- registrar os motivos de cada decisão.
O Scheduler NÃO deverá:
- comunicar-se diretamente com o Agent;
- executar comandos;
- mover arquivos;
- alterar configurações dos Nodes.
# **4. Fluxo**
Portal
↓
Control Plane
↓
Scheduler
↓
Node escolhido
↓
Control Plane
↓
Agent
↓
Torrent Engine
# **5. Processo de Decisão**
Toda decisão seguirá as etapas abaixo.
1
Obter lista de Nodes elegíveis.
↓
2
Aplicar restrições.
↓
3
Calcular Score.
↓
4
Ordenar candidatos.
↓
5
Selecionar melhor candidato.
↓
6
Registrar decisão.
↓
7
Retornar Node escolhido.
# **6. Elegibilidade**
Um Node somente poderá participar da seleção caso:
esteja ONLINE;
possua Engine operacional;
possua espaço disponível;
não esteja em MAINTENANCE;
não esteja em DRAIN;
não esteja DISABLED.
# **7. Critérios**
Cada Node receberá uma pontuação baseada em diversos fatores.
Os critérios iniciais serão:
Espaço disponível.
Health Score.
Quantidade de Torrents.
Carga do sistema.
Utilização de disco.
Utilização de CPU.
Utilização de memória.
Todos os critérios deverão possuir peso configurável.
# **8. Score**
O Scheduler calculará um Score único para cada Node.
Maior Score representa melhor candidato.
O algoritmo deverá ser determinístico.
Para um mesmo conjunto de informações, o resultado deverá ser sempre o mesmo.
# **9. Critérios Eliminatórios**
Antes do cálculo do Score, Nodes poderão ser eliminados.
Exemplos
Node OFFLINE.
SMART crítico.
Disco sem espaço.
Engine indisponível.
Health Score abaixo do mínimo.
Nodes eliminados não participarão da etapa de pontuação.
# **10. Balanceamento**
O Scheduler deverá buscar distribuição equilibrada.
Não deverá concentrar Torrents em um único Node quando existirem alternativas equivalentes.
A distribuição deverá considerar:
espaço;
quantidade de Torrents;
carga atual;
capacidade do Node.
# **11. Afinidade**
Na versão 1.0, afinidade será baseada apenas em Labels.
Exemplos
tracker=movies
country=br
priority=gold
storage=ssd
Caso uma política exija determinada Label, apenas Nodes compatíveis participarão da seleção.
# **12. Capabilities**
O Scheduler poderá utilizar Capabilities como critérios de elegibilidade.
Exemplos
SMART disponível.
SSD.
Docker.
USB3.
ARM64.
x86.
Torrent Engine compatível.
Capabilities são detectadas automaticamente pelo Agent.
Nunca pelo operador.
# **13. Health Score**
O Health Score representa a condição geral do Node.
O Scheduler deverá considerar esse valor antes da seleção.
Nodes degradados poderão continuar elegíveis, porém receberão pontuação inferior.
# **14. Espaço**
O espaço disponível deverá possuir peso elevado.
O Scheduler deverá evitar Nodes próximos do limite de utilização.
# **15. Transparência**
Toda decisão deverá ser registrada.
O operador deverá conseguir responder posteriormente:
Por que este Node foi escolhido?
Quais Nodes participaram?
Qual Score cada um recebeu?
Quais critérios eliminaram outros candidatos?
Toda decisão deverá ser auditável.
# **16. Processo de Avaliação**
Após selecionar os Nodes elegíveis, o Scheduler iniciará o processo de avaliação.
Cada Node será analisado individualmente.
A avaliação deverá utilizar exclusivamente informações persistidas pelo Control Plane.
O Scheduler nunca consultará diretamente os Agents durante uma decisão.
# **17. Critérios de Avaliação**
Os critérios abaixo compõem a avaliação inicial da versão 1.0.
**Espaço disponível**
Objetivo
Evitar Nodes próximos da capacidade máxima.
Quanto maior o espaço livre, maior será sua contribuição para a pontuação.
**Health Score**
Objetivo
Priorizar Nodes considerados saudáveis.
Nodes degradados poderão continuar participando, porém com menor prioridade.
**Quantidade de Torrents**
Objetivo
Evitar concentração excessiva de torrents em um único Node.
Esse critério considera apenas a quantidade de torrents administrados pelo SeedGrid.
**Utilização de CPU**
Objetivo
Evitar envio de novos torrents para Nodes sobrecarregados.
A utilização momentânea não deverá impedir completamente a seleção, exceto quando ultrapassar limites críticos definidos pelo Cluster.
**Utilização de Memória**
Objetivo
Distribuir carga de forma equilibrada.
O Scheduler deverá evitar Nodes operando próximos do limite de memória disponível.
**Utilização do Armazenamento**
Objetivo
Distribuir o crescimento do armazenamento entre os Nodes.
Não deverá existir preferência permanente por um único disco.
# **18. Critérios de Desempate**
Caso dois ou mais Nodes obtenham pontuação equivalente, o Scheduler deverá utilizar critérios adicionais.
Ordem sugerida:
Maior espaço livre.
Maior Health Score.
Menor quantidade de Torrents.
Node há mais tempo sem receber novos Torrents.
Menor carga média.
Persistindo empate, qualquer critério determinístico poderá ser utilizado.
# **19. Exclusão Temporária**
Um Node poderá ser temporariamente excluído das decisões.
Exemplos:
Manutenção.
Health Score crítico.
Espaço insuficiente.
Erro de comunicação recorrente.
Essa exclusão não altera o cadastro do Node.
Apenas impede sua seleção enquanto a condição permanecer.
# **20. Restrições**
O Scheduler deverá respeitar todas as restrições definidas pelo operador.
Exemplos:
Labels obrigatórias.
Capabilities obrigatórias.
Tipos específicos de armazenamento.
Essas restrições sempre terão prioridade sobre o cálculo do Score.
# **21. Registro da Decisão**
Cada decisão deverá gerar um registro contendo, no mínimo:
Data e hora.
Torrent.
Node selecionado.
Lista de candidatos.
Pontuação final.
Critérios eliminatórios.
Versão do algoritmo.
Essas informações permitirão auditoria e análise futura.
# **22. Tratamento de Falhas**
Caso nenhum Node seja elegível, o Scheduler deverá retornar uma condição de erro ao Control Plane.
O Control Plane será responsável por informar o operador.
O Scheduler não deverá tentar corrigir automaticamente a situação.
# **23. Reprodutibilidade**
Diante do mesmo conjunto de informações, o Scheduler deverá produzir o mesmo resultado.
Alterações no algoritmo deverão ser versionadas.
O histórico deverá registrar qual versão do algoritmo foi utilizada em cada decisão.
# **24. Independência**
O Scheduler não deverá conhecer detalhes do Torrent Engine.
Também não deverá depender de características específicas do sistema operacional dos Nodes.
Todas as informações necessárias deverão estar disponíveis através do modelo de dados do SeedGrid.
# **25. Performance**
As decisões deverão ocorrer rapidamente.
Objetivos da versão 1.0:
Seleção de Node inferior a 100 ms.
Mesmo com crescimento do Cluster, a decisão deverá permanecer proporcional ao número de Nodes elegíveis.
# **26. Extensibilidade**
Novos critérios poderão ser incorporados futuramente sem necessidade de alterar a arquitetura do Scheduler.
Esses critérios deverão integrar-se ao processo de avaliação existente.
# **27. Responsabilidades**
O Scheduler responde apenas à pergunta:
**"Qual é o melhor Node para executar esta operação?"**
Ele não executa a operação.
Não valida permissões.
Não comunica com o Agent.
Não altera estados do Cluster.
# **28. Princípios**
As decisões deverão ser:
Consistentes.
Auditáveis.
Determinísticas.
Reproduzíveis.
Transparentes.
Esses princípios deverão ser preservados durante toda a evolução do Scheduler.
# **29. Cenários Operacionais**
Os exemplos abaixo ilustram o comportamento esperado do Scheduler.
Os valores apresentados são apenas ilustrativos.
# **Cenário 1**
Cluster saudável.
Node 01
Status: ONLINE
Espaço: 82%
Health: 96
CPU: 18%
Torrents: 320
- ---------------------------
Node 02
Status: ONLINE
Espaço: 77%
Health: 94
CPU: 14%
Torrents: 305
- ---------------------------
Node 03
Status: ONLINE
Espaço: 91%
Health: 97
CPU: 22%
Torrents: 318
Resultado esperado
Todos os Nodes são elegíveis.
O Scheduler calcula o Score de cada candidato.
O Node com maior pontuação será selecionado.
# **Cenário 2**
Node em manutenção.
Node 01
ONLINE
- ---------------
Node 02
MAINTENANCE
- ---------------
Node 03
ONLINE
Resultado esperado
O Node 02 será removido da lista de candidatos antes do cálculo do Score.
# **Cenário 3**
Espaço insuficiente.
Node 01
Espaço Livre
4%
- ---------------
Node 02
42%
- ---------------
Node 03
51%
Resultado esperado
O Node 01 não deverá participar da seleção.
# **Cenário 4**
Health degradado.
Node 01
Health
98
- ---------------
Node 02
Health
56
- ---------------
Node 03
Health
94
Resultado esperado
O Node 02 poderá continuar elegível, porém receberá penalização em sua pontuação.
Caso o Health esteja abaixo do limite mínimo configurado, deverá ser eliminado.
# **Cenário 5**
Nenhum candidato disponível.
Resultado esperado
O Scheduler retornará falha ao Control Plane.
O Control Plane decidirá como tratar a situação.
O Scheduler não deverá criar regras especiais para esse cenário.
# **30. Registro da Decisão**
Após cada decisão, o Scheduler deverá registrar:
Identificador da decisão.
Data e hora.
Torrent.
Node escolhido.
Versão do algoritmo.
Critérios utilizados.
Pontuação final.
Lista de candidatos avaliados.
Tempo necessário para decisão.
Esses registros serão utilizados para auditoria e análise de comportamento.
# **31. Transparência**
Toda decisão deverá ser explicável.
O operador deverá conseguir visualizar:
Por que um Node foi escolhido.
Quais critérios tiveram maior influência.
Quais Nodes foram descartados.
Qual foi a classificação final.
O objetivo é eliminar decisões consideradas "caixa-preta".
# **32. Evolução do Algoritmo**
O algoritmo utilizado pelo Scheduler poderá evoluir ao longo do tempo.
Entretanto, toda alteração deverá preservar:
- determinismo;
- auditabilidade;
- compatibilidade com os contratos do Control Plane;
- registro da versão utilizada.
Mudanças de algoritmo não deverão exigir alterações no Portal ou nos Agents.
# **33. Configuração**
Os parâmetros do Scheduler deverão ser centralizados no Control Plane.
Exemplos:
Peso do espaço disponível.
Peso do Health Score.
Peso da utilização de CPU.
Peso da memória.
Limite mínimo de espaço livre.
Limite mínimo de Health Score.
Essas configurações deverão ser persistidas no banco de dados e disponibilizadas através da API administrativa.
# **34. Monitoramento**
O Scheduler deverá expor informações suficientes para acompanhamento operacional.
Exemplos:
Quantidade de decisões realizadas.
Tempo médio de decisão.
Quantidade de falhas.
Quantidade de Nodes elegíveis.
Quantidade média de candidatos por decisão.
Versão do algoritmo em uso.
Essas informações auxiliarão no diagnóstico e na evolução do algoritmo.
# **35. Testabilidade**
O Scheduler deverá ser implementado de forma que seus critérios possam ser testados isoladamente.
Cada critério deverá produzir resultados previsíveis quando submetido ao mesmo conjunto de dados.
Testes automatizados deverão validar:
- critérios eliminatórios;
- cálculo de Score;
- desempates;
- comportamento diante de falhas.
# **36. Resumo**
O Scheduler transforma um conjunto de informações do Cluster em uma decisão objetiva.
Seu funcionamento baseia-se em três etapas principais:
Selecionar
↓
Avaliar
↓
Classificar
↓
Escolher
Todo o restante da plataforma depende desse resultado.
# **37. Declaração Final**
O Scheduler representa o mecanismo de tomada de decisão do SeedGrid.
Sua responsabilidade é selecionar, de forma consistente, auditável e previsível, o Node mais adequado para executar cada operação.
O Scheduler não executa comandos, não interage diretamente com os Nodes e não possui conhecimento específico sobre o Torrent Engine.
Seu papel é fornecer decisões confiáveis para que o Control Plane coordene o Cluster como uma única plataforma distribuída.
