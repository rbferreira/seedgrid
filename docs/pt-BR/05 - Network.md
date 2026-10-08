# **SeedGrid**
**Network Specification**
Version: 1.0
Status: Draft
Document: 05 - Network.md
# **1. Objetivo**
Este documento define a arquitetura de rede do SeedGrid.
Seu objetivo é padronizar a comunicação entre todos os componentes da plataforma, garantindo simplicidade, segurança e previsibilidade operacional.
# **2. Filosofia**
A arquitetura de rede deverá ser simples.
Todos os componentes deverão comunicar-se utilizando protocolos amplamente suportados.
Sempre que possível, deverá ser possível implantar o SeedGrid em qualquer rede TCP/IP convencional sem necessidade de adaptações.
# **3. Topologia**
A topologia lógica será composta por:
Control Plane.
Seed Nodes.
Operadores.
Todos conectados através da mesma infraestrutura IP.
Exemplo
Operador
│
──────────────────────────────
Control Plane
│
──────────────────────────────
Seed Node 01
Seed Node 02
Seed Node 03
# **4. Modelo de Comunicação**
Toda comunicação deverá ser iniciada pelos clientes.
Portal → Control Plane
Agent → Control Plane
CLI → Control Plane
Integrações → Control Plane
O Control Plane nunca iniciará conexões diretamente para os Nodes.
# **5. Protocolos**
Protocolos previstos para a versão 1.0
HTTPS
REST
WebSocket (quando necessário)
TCP/IP
Nenhum protocolo proprietário será utilizado.
# **6. Comunicação entre Nodes**
Os Seed Nodes não deverão comunicar-se diretamente entre si.
Toda coordenação ocorrerá através do Control Plane.
Essa decisão reduz dependências e simplifica o gerenciamento da rede.
# **7. Descoberta**
A descoberta automática de Nodes não será utilizada.
Todo Node deverá conhecer previamente o endereço do Control Plane.
Esse endereço será definido durante a configuração inicial do Agent.
# **8. DNS**
Recomenda-se que o Control Plane possua um nome DNS estável.
Exemplo
seedgrid.local
ou
seedgrid.lab
Caso não exista DNS interno, poderá ser utilizado endereço IP.
# **9. Endereçamento**
Recomenda-se que todos os Nodes possuam endereço IP estável.
As opções recomendadas são:
Reserva DHCP.
Endereço IP estático.
Evitar endereços dinâmicos sem reserva.
# **10. Portas**
As portas utilizadas deverão ser mantidas no menor número possível.
Categorias
Portal.
API.
Banco de Dados.
Monitoramento.
Torrent Engine.
As portas específicas serão definidas no documento Deployment.md.
# **11. Comunicação Segura**
Toda comunicação entre Agent e Control Plane deverá utilizar TLS.
Credenciais nunca deverão trafegar em texto plano.
Certificados autoassinados poderão ser utilizados em ambientes domésticos.
Em ambientes corporativos recomenda-se utilização de certificados emitidos por autoridade certificadora confiável.
# **12. Firewall**
O Control Plane deverá expor apenas os serviços necessários.
Os Nodes deverão aceitar apenas conexões indispensáveis ao funcionamento do Torrent Engine e da administração local.
Todo o restante deverá permanecer bloqueado.
# **13. Fluxo Geral**
Portal
↓
HTTPS
↓
API
↓
Control Plane
↓
HTTPS
↓
Agent
↓
Engine
Todo o tráfego administrativo seguirá esse fluxo.
# **14. Segmentação**
Quando possível, recomenda-se manter os Nodes em uma rede dedicada ou VLAN específica.
Essa separação melhora organização e segurança, porém não é obrigatória para o funcionamento da plataforma.
# **15. Objetivos**
A arquitetura de rede deverá privilegiar:
Simplicidade.
Baixa latência.
Baixo overhead.
Facilidade de implantação.
Compatibilidade com redes domésticas e corporativas.
# **16. Princípio Fundamental**
Toda comunicação do SeedGrid deverá ocorrer através do Control Plane.
Os Nodes nunca dependerão de comunicação direta entre si para execução de suas responsabilidades.
# **17. Fluxos de Comunicação**
Todos os fluxos da plataforma deverão possuir origem, destino e responsabilidade claramente definidos.
Nenhum componente deverá comunicar-se diretamente com outro fora dos fluxos estabelecidos neste documento.
# **18. Fluxo de Registro**
Objetivo
Registrar um novo Node no Cluster.
SeedGrid Agent
↓
HTTPS
↓
API Gateway
↓
Authentication Service
↓
Node Service
↓
Database
↓
Resposta ao Agent
Esse fluxo ocorrerá apenas durante o primeiro registro ou quando um novo Node for adicionado ao Cluster.
# **19. Fluxo de Heartbeat**
Objetivo
Informar que o Node permanece operacional.
Agent
↓
HTTPS
↓
API Gateway
↓
Monitoring Service
↓
Database
Esse fluxo deverá ser leve e frequente.
Heartbeats nunca deverão transportar grandes volumes de dados.
# **20. Fluxo de Inventário**
Objetivo
Atualizar a configuração conhecida do Node.
Agent
↓
HTTPS
↓
Node Service
↓
Database
O inventário será enviado:
- no primeiro registro;
- quando solicitado;
- quando alterações forem detectadas.
# **21. Fluxo de Eventos**
Objetivo
Registrar acontecimentos relevantes.
Agent
↓
Event Service
↓
Database
↓
Portal
Exemplos:
Engine iniciado.
SMART crítico.
Disco removido.
Erro de comunicação.
# **22. Fluxo de Operações**
Objetivo
Executar ações administrativas.
Portal
↓
API
↓
Control Plane
↓
Agent
↓
Engine
Exemplos:
Adicionar torrent.
Pausar.
Retomar.
Recheck.
Atualizar configuração.
# **23. Fluxo de Consultas**
Sempre que possível:
Portal
↓
API
↓
Database
↓
Resposta
Consultas não deverão acessar diretamente os Agents, exceto quando informações em tempo real forem indispensáveis.
# **24. Fluxo de Monitoramento**
O monitoramento seguirá arquitetura pull.
Prometheus
↓
Coleta
↓
Control Plane
↓
Grafana
Quando necessário, o Prometheus poderá coletar métricas diretamente dos Exporters dos Nodes.
# **25. Fluxo de Alertas**
Evento
↓
Alert Service
↓
Database
↓
Portal
↓
Operador
Os alertas representam problemas ativos.
Nem todo evento gera um alerta.
# **26. Fluxo de Autenticação**
Portal
↓
Authentication Service
↓
Database
↓
Sessão
↓
Resposta
Os Agents utilizarão mecanismo de autenticação próprio.
Usuários nunca compartilharão credenciais com os Nodes.
# **27. Fluxo de Configuração**
Portal
↓
Configuration Service
↓
Database
↓
Resposta
Sempre que uma configuração impactar os Nodes, o Control Plane será responsável por coordenar sua distribuição.
# **28. Timeouts**
Toda comunicação deverá possuir timeout configurável.
Objetivos:
Evitar bloqueios permanentes.
Detectar indisponibilidade rapidamente.
Permitir recuperação automática.
Os valores específicos serão definidos durante a implementação.
# **29. Reconexão**
Caso a comunicação entre Agent e Control Plane seja interrompida:
O Agent deverá:
- continuar operando;
- tentar reconectar periodicamente;
- sincronizar informações pendentes após reconexão.
Esse comportamento deverá ser transparente ao operador sempre que possível.
# **30. Compressão**
Mensagens pequenas não deverão utilizar compressão.
Mensagens maiores, como inventários completos, poderão utilizar mecanismos de compressão quando isso representar benefício operacional.
Essa funcionalidade deverá permanecer transparente para os consumidores da API.
# **31. Objetivos da Rede**
A rede deverá proporcionar:
Baixa latência.
Alta previsibilidade.
Baixo consumo de banda.
Recuperação automática.
Operação contínua.
# **32. Princípios**
Os fluxos de comunicação deverão permanecer simples.
Cada informação deverá possuir um único caminho oficial dentro da arquitetura.
Essa abordagem reduz acoplamento, facilita monitoramento e simplifica futuras evoluções da plataforma.
# **33. Disponibilidade**
A rede deverá ser considerada um recurso compartilhado entre todos os componentes do Cluster.
A indisponibilidade temporária da rede não deverá interromper o funcionamento local dos Seed Nodes.
Durante falhas de comunicação:
- o Torrent Engine continuará operando;
- os torrents continuarão sendo distribuídos normalmente;
- o Agent continuará monitorando o Node;
- operações administrativas ficarão temporariamente indisponíveis.
# **34. Recuperação**
Após o restabelecimento da comunicação com o Control Plane, o Agent deverá:
- reestabelecer autenticação;
- enviar novo Heartbeat;
- sincronizar eventos pendentes;
- atualizar inventário quando necessário;
- retomar operação normal.
Todo o processo deverá ocorrer automaticamente.
# **35. Tolerância a Falhas**
O SeedGrid deverá suportar falhas temporárias de rede.
Exemplos:
Perda momentânea de conectividade.
Reinicialização do switch.
Renovação de endereço IP.
Reinício do Control Plane.
Sempre que possível, a recuperação deverá ocorrer sem intervenção do operador.
# **36. Requisitos de Latência**
A plataforma foi projetada para redes locais.
A comunicação deverá permanecer eficiente mesmo em redes domésticas.
A latência não deverá ser um fator determinante para o funcionamento do Cluster.
Entretanto, recomenda-se que todos os Nodes estejam conectados por Ethernet sempre que possível.
# **37. Largura de Banda**
O SeedGrid deverá gerar baixo tráfego administrativo.
O maior consumo de banda continuará sendo o protocolo BitTorrent.
As comunicações entre Agent e Control Plane deverão representar apenas uma pequena fração do tráfego total da infraestrutura.
# **38. Sincronização de Horário**
Todos os equipamentos deverão manter seus relógios sincronizados.
Recomenda-se utilização de NTP.
A sincronização correta é fundamental para:
- auditoria;
- eventos;
- monitoramento;
- correlação de logs.
# **39. Segurança**
Boas práticas recomendadas:
- utilizar HTTPS em toda comunicação administrativa;
- restringir acesso ao Portal;
- proteger credenciais do Agent;
- manter sistemas atualizados;
- utilizar firewall no Control Plane;
- limitar exposição de serviços à rede.
Sempre que possível, apenas Portal e API deverão estar acessíveis aos operadores.
# **40. Ambientes**
A arquitetura deverá suportar diferentes cenários.
# **Homelab**
Control Plane em máquina virtual.
Seed Nodes conectados à LAN.
DNS opcional.
Certificados autoassinados.
# **Pequeno Ambiente Corporativo**
Control Plane dedicado.
DNS interno.
Certificados válidos.
Monitoramento centralizado.
Backups automatizados.
# **Ambiente Distribuído**
A arquitetura permite a distribuição geográfica dos Nodes, desde que exista conectividade IP com o Control Plane.
Questões de latência e disponibilidade deverão ser avaliadas conforme o ambiente.
# **41. Escalabilidade**
O crescimento da rede deverá ocorrer apenas pela inclusão de novos Nodes.
Nenhuma alteração estrutural será necessária no protocolo de comunicação.
Todos os novos Nodes seguirão exatamente o mesmo fluxo de registro e operação.
# **42. Monitoramento da Rede**
O Control Plane deverá disponibilizar informações suficientes para identificar problemas de comunicação.
Exemplos:
- último Heartbeat recebido;
- Nodes desconectados;
- tempo desde o último contato;
- falhas de autenticação;
- erros de comunicação.
Essas informações deverão ser integradas ao sistema de monitoramento da plataforma.
# **43. Resumo**
A arquitetura de rede do SeedGrid baseia-se em um modelo simples e centralizado.
Operador
│
▼
Portal
│
▼
API Gateway
│
▼
Control Plane
│
┌───┼───────────────┐
▼   ▼               ▼
Agent 01       Agent 02       Agent 03
│               │               │
qBittorrent    qBittorrent    qBittorrent
Todo fluxo administrativo passa obrigatoriamente pelo Control Plane.
Os Nodes permanecem independentes e não estabelecem comunicação direta entre si.
# **44. Declaração Final**
A arquitetura de rede do SeedGrid foi projetada para privilegiar simplicidade, previsibilidade e baixo acoplamento.
A comunicação centralizada através do Control Plane reduz a complexidade operacional, facilita auditoria e monitoramento e permite que novos Nodes sejam incorporados ao Cluster sem alterações na infraestrutura existente.
Essa abordagem estabelece uma base sólida para a evolução da plataforma, preservando compatibilidade entre diferentes ambientes de implantação e diferentes gerações de hardware.
