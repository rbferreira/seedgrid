# **SeedGrid**
**Development Guide**
Version: 1.0
Status: Draft
Document: 20 - Development Guide.md
# **1. Objetivo**
Este documento define a organização do ambiente de desenvolvimento do SeedGrid.
Seu objetivo é padronizar a forma como novos desenvolvedores configuram o ambiente, compreendem a estrutura do projeto e contribuem para sua evolução.
# **2. Filosofia**
O ambiente de desenvolvimento deverá ser:
- simples;
- reproduzível;
- documentado;
- previsível.
Um novo desenvolvedor deverá conseguir iniciar sua contribuição utilizando apenas a documentação oficial.
# **3. Público-Alvo**
Este documento destina-se a:
- desenvolvedores;
- mantenedores;
- revisores;
- contribuidores externos.
# **4. Organização Geral**
O projeto será dividido em componentes independentes.
Estrutura sugerida:
seedgrid/
├── control-plane/
├── portal/
├── agent/
├── scheduler/
├── api/
├── shared/
├── infrastructure/
├── deployment/
├── docs/
├── scripts/
└── tests/
Cada componente deverá possuir responsabilidade claramente definida.
# **5. Organização dos Componentes**
Cada serviço deverá seguir uma estrutura semelhante.
Exemplo:
component/
├── src/
├── tests/
├── docs/
├── config/
├── migrations/
├── Dockerfile
└── README.md
Essa padronização facilita navegação e manutenção.
# **6. Documentação**
Cada componente deverá possuir documentação própria.
Itens mínimos:
- objetivo;
- responsabilidades;
- dependências;
- configuração;
- instruções de execução;
- testes disponíveis.
A documentação específica complementa a documentação arquitetural.
# **7. Ambiente Local**
O ambiente de desenvolvimento deverá permitir execução local dos principais componentes.
Objetivos:
- facilitar depuração;
- acelerar desenvolvimento;
- reduzir dependência de infraestrutura externa.
Sempre que possível, serviços auxiliares deverão ser executados em containers.
# **8. Dependências**
Todas as dependências deverão ser explicitamente declaradas.
Não serão aceitas dependências implícitas ou configurações manuais não documentadas.
# **9. Configuração**
As configurações deverão permanecer externas ao código-fonte.
Exemplos:
- variáveis de ambiente;
- arquivos .env;
- arquivos YAML;
- arquivos JSON.
Nenhuma configuração específica de ambiente deverá ser incorporada ao código.
# **10. Banco de Dados**
O ambiente local deverá disponibilizar uma instância do PostgreSQL compatível com produção.
Migrations deverão ser utilizadas para toda alteração estrutural.
Alterações manuais no banco não serão consideradas parte oficial do projeto.
# **11. Serviços Auxiliares**
Durante o desenvolvimento poderão ser utilizados serviços auxiliares.
Exemplos:
- PostgreSQL;
- Prometheus;
- Grafana;
- Loki;
- Reverse Proxy.
Esses componentes deverão ser inicializados de forma reproduzível.
# **12. Testes**
Cada componente deverá possuir testes compatíveis com sua responsabilidade.
Tipos previstos:
- testes unitários;
- testes de integração;
- testes funcionais.
A arquitetura deverá favorecer execução automatizada dos testes.
# **13. Scripts**
Tarefas repetitivas deverão ser automatizadas.
Exemplos:
- iniciar ambiente;
- parar ambiente;
- executar testes;
- aplicar migrations;
- limpar ambiente;
- gerar documentação.
Os scripts deverão permanecer versionados juntamente com o projeto.
# **14. Objetivos**
O ambiente de desenvolvimento deverá reduzir o tempo necessário para que um novo desenvolvedor compreenda a arquitetura e comece a contribuir.
# **15. Princípio Fundamental**
O desenvolvimento do SeedGrid deverá privilegiar simplicidade, consistência e padronização.
Toda contribuição deverá respeitar a arquitetura definida nos documentos oficiais e as decisões registradas nas ADRs.
# **16. Fluxo de Desenvolvimento**
Toda nova funcionalidade deverá seguir um fluxo padronizado.
Fluxo recomendado:
Requisito
↓
ADR (quando necessário)
↓
Implementação
↓
Testes
↓
Code Review
↓
Documentação
↓
Merge
Nenhuma funcionalidade será considerada concluída sem documentação e testes compatíveis.
# **17. Organização das Alterações**
Cada alteração deverá possuir um objetivo único e claramente definido.
Sempre que possível, mudanças distintas deverão ser implementadas em Pull Requests independentes.
Essa prática facilita revisão, testes e rastreabilidade.
# **18. Migrations**
Alterações estruturais no banco de dados deverão utilizar migrations versionadas.
Princípios:
- reversíveis, sempre que possível;
- idempotentes;
- documentadas;
- testadas.
Alterações manuais no banco não fazem parte do processo oficial.
# **19. APIs**
Toda nova API deverá seguir os padrões definidos em API.md.
Requisitos mínimos:
- autenticação;
- tratamento consistente de erros;
- versionamento;
- documentação;
- testes automatizados.
Nenhum endpoint deverá acessar diretamente componentes internos sem passar pelas camadas previstas na arquitetura.
# **20. Novos Serviços**
A criação de um novo serviço deverá ser precedida por avaliação arquitetural.
Cada novo componente deverá possuir:
- responsabilidade única;
- documentação própria;
- estratégia de monitoramento;
- estratégia de logs;
- testes;
- integração com observabilidade.
Quando representar mudança arquitetural significativa, uma nova ADR deverá ser criada.
# **21. Tratamento de Dependências**
Dependências externas deverão ser reduzidas ao mínimo necessário.
Ao adicionar uma nova dependência, o desenvolvedor deverá considerar:
- maturidade do projeto;
- licença;
- manutenção ativa;
- impacto na segurança;
- impacto operacional.
# **22. Ambiente de Testes**
O ambiente de desenvolvimento deverá permitir execução completa da plataforma utilizando dados de teste.
Objetivos:
- validar novas funcionalidades;
- reproduzir falhas;
- executar testes automatizados;
- simular múltiplos Seed Nodes.
Nenhum teste deverá depender de infraestrutura de produção.
# **23. Dados de Desenvolvimento**
Dados utilizados para desenvolvimento deverão ser fictícios.
Nunca deverão ser utilizados:
- credenciais reais;
- certificados de produção;
- informações pessoais;
- dados operacionais do ambiente produtivo.
# **24. Mock Components**
Sempre que necessário, componentes simulados poderão ser utilizados.
Exemplos:
- Agent simulado;
- Torrent Engine simulado;
- API simulada;
- eventos simulados.
Esses componentes facilitam testes e reduzem dependências entre equipes.
# **25. Revisão de Código**
Toda contribuição deverá passar por revisão.
A revisão deverá avaliar, no mínimo:
- aderência à arquitetura;
- conformidade com as ADRs;
- qualidade do código;
- cobertura de testes;
- atualização da documentação.
# **26. Versionamento**
Todos os componentes deverão seguir a mesma estratégia de versionamento da plataforma.
A versão publicada deverá identificar claramente a compatibilidade entre:
- Control Plane;
- Agent;
- Portal;
- API;
- Scheduler.
# **27. Princípios**
O processo de desenvolvimento deverá privilegiar:
- simplicidade;
- legibilidade;
- previsibilidade;
- documentação;
- testes;
- evolução incremental.
Toda contribuição deverá fortalecer a consistência da plataforma como um todo.
# **28. Definition of Done (DoD)**
Uma funcionalidade somente será considerada concluída quando atender aos critérios definidos nesta seção.
O objetivo da Definition of Done é garantir consistência, qualidade e aderência à arquitetura da plataforma.
# **Critérios Obrigatórios**
Sempre que aplicável, uma entrega deverá atender aos seguintes requisitos:
- implementação concluída;
- testes automatizados executados com sucesso;
- documentação atualizada;
- aderência às ADRs vigentes;
- APIs documentadas;
- migrations criadas e testadas;
- logs adicionados quando necessários;
- métricas adicionadas quando relevantes;
- regras de alerta revisadas quando aplicáveis;
- monitoramento atualizado quando necessário;
- revisão de código aprovada.
# **Critérios Arquiteturais**
A implementação deverá respeitar os princípios definidos na documentação oficial.
Em especial:
- responsabilidades bem definidas;
- baixo acoplamento;
- comunicação através dos componentes previstos;
- ausência de dependências implícitas;
- conformidade com a arquitetura de serviços.
# **Critérios Operacionais**
Sempre que uma alteração impactar a operação da plataforma, deverão ser avaliados:
- monitoramento;
- alertas;
- backup;
- Disaster Recovery;
- documentação operacional.
# **Critérios de Segurança**
Sempre que aplicável:
- autenticação validada;
- autorização revisada;
- tratamento adequado de erros;
- proteção de informações sensíveis;
- validação de entradas.
# **Critérios de Observabilidade**
Toda funcionalidade relevante deverá considerar:
- geração de logs;
- métricas operacionais;
- eventos;
- rastreabilidade;
- auditoria, quando aplicável.
# **Critérios de Evolução**
Sempre que uma alteração modificar a arquitetura da plataforma:
- uma ADR deverá ser criada ou atualizada;
- impactos deverão ser documentados;
- compatibilidade deverá ser avaliada.
# **29. Integração Contínua**
O projeto deverá ser compatível com pipelines de Integração Contínua (CI).
Objetivos:
- execução automática de testes;
- validação da qualidade do código;
- verificação de documentação;
- validação das migrations;
- geração de artefatos.
A arquitetura deverá favorecer automação desde o início do desenvolvimento.
O CI inclui o job **Policies (Go)**, que executa verificadores de política do projeto (módulo `tools/`): o formato das mensagens de commit (`commitmsgcheck`), as fronteiras da arquitetura (`architecturepolicy`) e o idioma do código (`languagepolicy`). O código é escrito em inglês — comentários, mensagens de log e de erro, nomes de teste e identificadores; o produto fala outros idiomas só pelos catálogos (ADR-041). Uma exceção legítima (o nome de categoria de um site brasileiro que o código precisa reconhecer, um título real citado como exemplo) vai em `tools/language-allow.txt` com o motivo.

## **Mensagens de commit**

As mensagens de commit seguem o formato Conventional Commits e são verificadas automaticamente no CI pelo `commitmsgcheck`.

Regras:
- **Assunto** (primeira linha, sem espaços à direita) no formato convencional:
  `^(feat|fix|chore|docs|refactor|test|perf|ci|build|style|revert)(\([A-Za-z0-9._/#-]+\))?!?: \S`
  Exemplos válidos: `feat: x`, `fix(deploy): x`, `feat(api)!: x`, `chore: x (#8)` (squash do GitHub).
- **Isentas**: mensagens geradas pelo próprio git — assunto que começa com `Merge ` ou com `Revert "`.
- **Sem trailer de coautoria**: qualquer linha que case `(?i)^\s*co-authored-by\s*:` reprova.
- Mensagem vazia (ou só comentários) reprova.
- No modo hook (`-file`), linhas que começam com `#` são comentários do git e são descartadas antes de checar.

Não há regra de idioma nem de tamanho.

**Rodar à mão:**

```bash
cd tools && go run ./cmd/commitmsgcheck -range origin/main..HEAD
```

**Instalar como hook `commit-msg`:**

Crie `.git/hooks/commit-msg` com o conteúdo:

```bash
#!/bin/sh
# O git passa o arquivo relativo à raiz (.git/COMMIT_EDITMSG): resolva antes do cd.
case "$1" in /*) f="$1" ;; *) f="$PWD/$1" ;; esac
cd tools && exec go run ./cmd/commitmsgcheck -file "$f"
```

## **Fronteiras da arquitetura**

As fronteiras duras da arquitetura são verificadas automaticamente no CI pelo `architecturepolicy`.

Regras:
- **agent-no-database** — nenhum `.go` sob `agent/` importa `database/sql`, `github.com/jackc/` ou `github.com/lib/pq`. Protege: só o Control Plane acessa o banco.
- **agent-no-control-plane** — nenhum `.go` sob `agent/` importa `github.com/rbferreira/seedgrid/control-plane` (ou subpacote). Protege: Agent e Control Plane falam só pela API.
- **control-plane-no-agent** — nenhum `.go` sob `control-plane/` importa `github.com/rbferreira/seedgrid/agent` (ou subpacote). Protege: o Control Plane não fala diretamente com o Engine.
- **agent-no-worker** — nenhum `.go` sob `agent/` declara identificador (pacote, tipo, função, método, parâmetro, variável, constante, campo de struct, método de interface) que contenha `worker`, ignorando caixa. Comentários e strings não contam. Protege: Worker é componente interno do Control Plane e não roda no Seed Node.
- **portal-network-only-via-api** — em `portal/src/**/*.ts` e `*.tsx`, exceto `portal/src/api.ts` e arquivos de teste, nenhuma linha usa `fetch(`, `new XMLHttpRequest`, `new WebSocket`, `new EventSource` ou `sendBeacon(`. Protege: toda conversa de rede do Portal passa pelo `api.ts`, que fala só com a API.

**Rodar à mão:**

```bash
cd tools && go run ./cmd/architecturepolicy -root ..
```
# **30. Evolução**
O processo de desenvolvimento deverá evoluir juntamente com a plataforma.
Novas práticas poderão ser incorporadas desde que respeitem os princípios definidos neste documento e nas ADRs.
# **31. Relação com os Demais Documentos**
O Development Guide complementa os demais documentos do projeto.
Enquanto os documentos arquiteturais definem **o que** deve ser construído e **por que**, este documento estabelece **como** a equipe deverá desenvolver, validar e manter a plataforma ao longo do tempo.
# **32. Resumo**
O processo oficial de desenvolvimento do SeedGrid baseia-se em:
- arquitetura documentada;
- decisões registradas em ADRs;
- documentação contínua;
- testes automatizados;
- revisão de código;
- observabilidade;
- evolução incremental.
Esses princípios deverão orientar todas as contribuições para o projeto.
# **33. Declaração Final**
O Development Guide estabelece as práticas oficiais de engenharia do SeedGrid.
Seu objetivo é garantir que a evolução da plataforma ocorra de forma consistente, previsível e alinhada à arquitetura definida pelos demais documentos, preservando qualidade técnica, rastreabilidade e facilidade de manutenção durante todo o ciclo de vida do projeto.
