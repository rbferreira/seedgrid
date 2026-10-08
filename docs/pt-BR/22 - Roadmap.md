# **SeedGrid**
**Product Roadmap**
Version: 1.0
Status: Draft
Document: 22 - Roadmap.md
# **1. Objetivo**
Este documento define a estratégia de evolução do SeedGrid.
Seu objetivo é registrar as capacidades previstas para futuras versões da plataforma, preservando alinhamento com a arquitetura e evitando crescimento desordenado.
# **2. Filosofia**
O Roadmap representa uma direção estratégica.
Ele não constitui compromisso de implementação.
As prioridades poderão ser revisadas conforme a evolução do projeto.
# **3. Princípios**
A evolução da plataforma deverá respeitar os seguintes princípios:
- simplicidade;
- baixo acoplamento;
- previsibilidade;
- observabilidade;
- modularidade;
- compatibilidade arquitetural.
Nenhuma evolução deverá comprometer esses princípios.
# **4. Versão 1.0**
Objetivo principal:
Disponibilizar uma plataforma estável para gerenciamento centralizado de Seed Nodes.
Capacidades:
- Control Plane;
- Portal;
- API;
- Scheduler;
- Agent;
- Monitoramento;
- Alertas;
- Backup da plataforma;
- Disaster Recovery;
- Observabilidade;
- Documentação completa.
A versão 1.0 prioriza estabilidade e arquitetura sólida.
# **5. Diretrizes para Evolução**
Toda nova funcionalidade deverá responder positivamente às seguintes perguntas:
- resolve um problema real?
- mantém simplicidade?
- respeita a arquitetura?
- reduz carga operacional?
- favorece manutenção?
Caso contrário, sua inclusão deverá ser reavaliada.
# **6. Visão para a V2**
Objetivo principal:
Transformar o SeedGrid de uma plataforma de administração em uma plataforma de operação inteligente.
A V2 deverá priorizar:
- automação;
- assistência ao operador;
- explicabilidade;
- planejamento operacional;
- redução da carga cognitiva.
# **7. Tema Estratégico**
A evolução da V2 concentra-se em um conceito central:
**Auxiliar o operador a tomar decisões melhores, sem retirar dele o controle da plataforma.**
Esse princípio deverá orientar todas as funcionalidades futuras.
# **8. Eixos de Evolução**
As funcionalidades previstas organizam-se em cinco eixos:
Operação.
Observabilidade.
Experiência do Operador.
Arquitetura.
Ferramentas de Engenharia.
Essa organização facilita priorização e evolução incremental.
# **9. Compatibilidade**
Sempre que possível, novas funcionalidades deverão preservar compatibilidade com a arquitetura da V1.
Mudanças incompatíveis deverão ser precedidas por novas ADRs.
# **10. Processo**
Toda funcionalidade relevante deverá seguir o fluxo oficial da plataforma:
Necessidade
↓
Discussão
↓
ADR (quando aplicável)
↓
Implementação
↓
Testes
↓
Documentação
# **11. Objetivos**
O Roadmap deverá orientar a evolução da plataforma sem comprometer sua consistência arquitetural.
# **12. Princípio Fundamental**
A evolução do SeedGrid deverá priorizar qualidade, coerência e sustentabilidade em vez de quantidade de funcionalidades.
# **13. Capacidade: Reduzir Latência Operacional**
# **Objetivo**
Reduzir o tempo entre uma ação do operador e sua execução no Cluster.
# **Motivação**
Melhorar a responsividade da plataforma durante operações administrativas.
# **Possíveis Implementações**
- Canal persistente entre Agent e Control Plane utilizando WebSocket.
- Outros mecanismos equivalentes que preservem os princípios arquiteturais da plataforma.
# **Benefícios Esperados**
- menor latência;
- redução de polling;
- menor consumo de recursos;
- melhor experiência operacional.
# **Prioridade**
Média.
# **Status**
Ideia.
# **14. Capacidade: Melhorar a Compreensão das Decisões**
# **Objetivo**
Permitir que o operador compreenda por que a plataforma tomou determinada decisão.
# **Motivação**
Aumentar transparência e confiança na operação do Cluster.
# **Possíveis Implementações**
- Explain Mode.
- Exibição dos critérios utilizados pelo Scheduler.
- Justificativas detalhadas para Alertas.
- Exposição do cálculo do Health Score.
# **Benefícios Esperados**
- redução da carga cognitiva;
- maior previsibilidade;
- diagnósticos mais rápidos.
# **Prioridade**
Alta.
# **Status**
Planejado.
# **15. Capacidade: Facilitar Diagnóstico Operacional**
# **Objetivo**
Reduzir o tempo necessário para identificar a origem de problemas.
# **Motivação**
Facilitar troubleshooting e auditoria operacional.
# **Possíveis Implementações**
- Node Timeline.
- Root Cause Analysis.
- Correlação automática entre Eventos e Alertas.
- Navegação contextual entre recursos.
# **Benefícios Esperados**
- diagnósticos mais rápidos;
- menor tempo de indisponibilidade;
- melhor rastreabilidade.
# **Prioridade**
Alta.
# **Status**
Planejado.
# **16. Capacidade: Assistência Operacional**
# **Objetivo**
Guiar o operador durante atividades recorrentes e procedimentos de recuperação.
# **Motivação**
Reduzir erros operacionais e facilitar a execução de tarefas.
# **Possíveis Implementações**
- Recovery Assistant.
- Runbooks guiados.
- Assistentes contextuais.
- Checklists automatizados.
# **Benefícios Esperados**
- menor curva de aprendizado;
- procedimentos consistentes;
- redução de erros humanos.
# **Prioridade**
Média.
# **Status**
Ideia.
# **17. Capacidade: Planejamento de Capacidade**
# **Objetivo**
Apoiar decisões relacionadas ao crescimento do Cluster.
# **Motivação**
Permitir planejamento proativo da infraestrutura.
# **Possíveis Implementações**
- Capacity Planner.
- Projeção de utilização.
- Tendências de crescimento.
- Estimativa de esgotamento de armazenamento.
# **Benefícios Esperados**
- melhor planejamento;
- redução de indisponibilidade;
- otimização da infraestrutura.
# **Prioridade**
Média.
# **Status**
Planejado.
# **18. Capacidade: Evolução da Engenharia**
# **Objetivo**
Aprimorar continuamente a qualidade do desenvolvimento da plataforma.
# **Motivação**
Automatizar verificações arquiteturais e reduzir inconsistências.
# **Possíveis Implementações**
- Architecture Linter.
- Validações automáticas em CI.
- Verificação de aderência às ADRs.
- Regras automáticas de observabilidade.
# **Benefícios Esperados**
- maior qualidade do código;
- menor dívida técnica;
- arquitetura mais consistente.
# **Prioridade**
Baixa.
# **Status**
Ideia.
# **19. Capacidade: Administração da Plataforma**
# **Objetivo**
Simplificar atividades administrativas e operacionais.
# **Motivação**
Reduzir tarefas repetitivas e facilitar manutenção.
# **Possíveis Implementações**
- Ferramenta oficial de administração (seedgridctl).
- Automatização de validações.
- Automatização de upgrades.
- Automatização de diagnósticos.
# **Benefícios Esperados**
- menor esforço operacional;
- maior padronização;
- redução de erros.
# **Prioridade**
Média.
# **Status**
Ideia.
# **20. Princípios**
As funcionalidades apresentadas neste documento representam direções estratégicas.
As implementações sugeridas não são obrigatórias.
Novas soluções poderão substituir as propostas atuais, desde que atendam aos mesmos objetivos arquiteturais e operacionais definidos para a plataforma.
# **21. Priorização**
As capacidades previstas no Roadmap deverão ser priorizadas considerando:
- impacto para o operador;
- aderência à arquitetura;
- redução da complexidade operacional;
- custo de implementação;
- riscos técnicos;
- maturidade da plataforma.
A prioridade poderá ser revisada ao longo da evolução do projeto.
# **22. Processo de Evolução**
A inclusão de uma nova capacidade deverá seguir um processo estruturado.
Fluxo recomendado:
Necessidade
↓
Discussão
↓
Análise Arquitetural
↓
ADR (quando aplicável)
↓
Inclusão no Roadmap
↓
Implementação
↓
Documentação
Nem toda ideia deverá entrar imediatamente no Roadmap.
# **23. Status das Capacidades**
Cada capacidade deverá possuir um estado claramente identificado.
Estados oficiais:
- Ideia
- Em avaliação
- Planejada
- Em desenvolvimento
- Concluída
- Adiada
- Cancelada
A mudança de estado deverá ser registrada para preservar o histórico da evolução da plataforma.
# **24. Revisões**
O Roadmap deverá ser revisado periodicamente.
Durante cada revisão deverão ser avaliados:
- novas necessidades;
- capacidades concluídas;
- capacidades obsoletas;
- mudanças tecnológicas;
- aderência aos princípios arquiteturais.
# **25. Relação com as ADRs**
O Roadmap descreve **para onde** a plataforma pretende evoluir.
As ADRs descrevem **como e por que** as decisões arquiteturais foram tomadas.
Sempre que uma capacidade exigir alteração significativa da arquitetura, uma nova ADR deverá ser criada antes de sua implementação.
# **26. Relação com os Demais Documentos**
O Roadmap complementa a documentação oficial do SeedGrid.
Ele não substitui:
- Vision.md;
- Requirements.md;
- Architecture.md;
- ADR.md;
- Development Guide.md.
Seu papel é orientar a evolução estratégica da plataforma.
# **27. Resumo**
A evolução do SeedGrid deverá ser orientada por capacidades, e não por tecnologias específicas.
Cada capacidade deverá possuir:
- objetivo claro;
- motivação;
- benefícios esperados;
- possíveis implementações;
- prioridade;
- status.
Essa abordagem preserva flexibilidade tecnológica e mantém o foco nos problemas que a plataforma pretende resolver.


# **28. Capacidade: Evoluir a Gestão de Armazenamento**
Objetivo

Permitir que a plataforma evolua de uma visão de armazenamento por Seed Node para uma visão baseada em recursos de armazenamento independentes.

Motivação

A V1 considera o armazenamento como um atributo do Seed Node, simplificando a arquitetura e reduzindo a complexidade operacional.

Em ambientes maiores, essa abstração poderá limitar políticas mais avançadas de alocação e utilização de armazenamento.

Possíveis Implementações
Introdução do conceito de Storage como entidade arquitetural de primeira classe.
Seleção de Storage pelo Scheduler durante a distribuição de workloads.
Classes de armazenamento (Storage Classes).
Labels e metadados de Storage.
Políticas de afinidade e elegibilidade.
Balanceamento de utilização entre Storages de um mesmo Node.
Benefícios Esperados
maior flexibilidade na distribuição de workloads;
suporte a múltiplos recursos de armazenamento por Seed Node;
políticas avançadas de alocação;
melhor utilização da capacidade disponível;
preparação para ambientes heterogêneos.
Prioridade

Baixa.

Status

Ideia.

Observações

Esta capacidade representa uma evolução arquitetural e somente deverá ser considerada após a estabilização da arquitetura da V1. Sua implementação exigirá revisão das ADRs, do modelo de domínio, do Scheduler, da API e dos componentes relacionados ao inventário e monitoramento.

# **29. Capacidade: Bloqueio de Grupos de Release Banidos por Tracker**
Objetivo

Impedir que o SeedGrid publique num tracker um release cujo grupo esteja na lista de grupos banidos daquele tracker, bloqueando o envio e informando o operador do motivo — em vez de deixar o tracker recusar (`postpone`/ban) depois do envio.

Motivação

Alguns trackers UNIT3D (ex.: samaritano.cc) proíbem explicitamente uploads de determinados grupos de release, por práticas reconhecidas como problemáticas: retag (renomeiam arquivos pra parecer qualidade superior), classificação falsa (rotulam encode simples como REMUX/WEB-DL), propaganda inserida (legendas/mediainfo/vídeo), arquivo adulterado (watermark, upscaling artificial, BD autorado não original) e spam/links externos. Hoje o SeedGrid não valida isso — só descobre quando o tracker recusa o envio manualmente.

Possíveis Implementações
- Seção nova em Configurações (Portal) para gerenciar a lista de grupos banidos, por tracker (mesmo escopo de `trackers`, ADR-020 — cada tracker pode ter sua própria lista/regra).
- Checagem no fluxo de publicação (antes do `POST /api/v1/torrents/{id}/publish-tracker`): compara o campo `Group` já extraído pelo parser de release (`control-plane/internal/release/release.go`, `Info.Group`) contra a lista do tracker de destino, comparação case-insensitive.
- Bloqueio explícito com mensagem clara ao operador (ex.: "Grupo «YIFY» não é permitido no samaritano.cc — envio bloqueado") — nunca falha silenciosamente nem deixa passar pra depois falhar no tracker.
- Seed inicial da lista para o samaritano.cc com os grupos abaixo (levantados a partir da wiki do tracker); a lista deverá ficar editável pelo operador (trackers mudam a lista com o tempo).

Lista inicial de grupos banidos (samaritano.cc)

3LTON, 4yEo, ADE, AFG, AniHLS, AniURL, AnimeRG, AROMA, aXXo, B3LLUM, BLUDV, CaNNIBal, CHD, CM8, Comando, CrEwSaDe, D3US, d3g, DeadFish, DragsterPS, DNL, DOZITOS, eSc, ELiTE, Emmid, FaNGDiNG0, FGT, Flights, FRDS, FUM, HAiKU, HD2DVD, HDS, HDTime, Hi10, Hiro360, ION10, iPlanet, JIVE, KiNGDOM, Lama, Lapumia, Leffe, LEGi0N, LOAD, MACCAULAY, MeGusta, MezRips, mHD, mRS, mSD, MSD, NeXus, nHD, NhaNc3, nikt0, NOIVTC, nSD, OFT, Oj, PiRaTeS, PlaySD, playXD, PRODJi, Radarr, RAPiDCOWS, RARBG, RCDiVX, RDN, REsuRRecTioN, RetroPeeps, RMTeam, S74Ll10n, SANTi, SicFoI, SILVEIRATeam, SPASM, SPDVD, STARCKFILMES, STUTTERSHIT, Telly, TGx, TM, TRiToN, UPiNSMOKE, URANiME, VXT, WAF, Will1869, x0r, xRed, XS, YIFY, YTS, ZKBL, ZMNT, ZmN

Benefícios Esperados
- evita `postpone`/ban por grupo banido, sem depender do operador lembrar a lista de cada tracker;
- feedback imediato no Portal, antes do envio, em vez de descoberto depois pelo staff do tracker;
- lista fica centralizada e editável, acompanhando mudanças de política do tracker ao longo do tempo.

Prioridade

Alta — motivada por caso real (rejeição de upload no samaritano.cc por padronização; a lista de grupos banidos é regra explícita do mesmo tracker).

Status

Ideia.

Observações

Complementa a ADR-020 (publicação em tracker) e a ADR-026 (screenshots) — mais uma verificação na mesma etapa de conferência humana antes do `POST` ao tracker. Exigirá uma migration nova (lista de grupos por tracker) se não for resolvido como um tunável simples de texto; avaliar no desenho da ADR correspondente antes de implementar (mudança de contrato/schema).

# **30. Declaração Final**
O Roadmap do SeedGrid representa a visão estratégica da evolução da plataforma.
Ao priorizar capacidades em vez de implementações específicas, o projeto mantém liberdade para adotar as soluções técnicas mais adequadas no momento oportuno, preservando sua arquitetura, seus princípios e sua identidade ao longo do tempo.
