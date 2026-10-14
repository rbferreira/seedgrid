export const settings = {
  settings: {
    page: {
      title: "Configurações",
      intro: "Tudo aqui vale na hora, sem reiniciar nada. Durações aceitam {m5}, {s90}, {h168}. Credenciais nunca são exibidas de volta — para trocar, digite a nova; para desligar, restaure o padrão.",
    },
    trackers: {
      title: "Trackers",
      blurb: "Sites onde o conteúdo é publicado.",
    },
    categories: {
      admission: "Como o cluster reparte os downloads simultâneos que o tracker permite.",
      agent: "Como os Seed Nodes se comportam: Agent e fila do Engine. Aqui está o valor de CLUSTER — vale para todo Node sem valor próprio, no próximo heartbeat; um Node que aguenta mais ou menos que a frota ganha o dele no detalhe do Node. Também: de onde os Nodes baixam suas atualizações.",
      alerting: "Quando o Control Plane abre um alerta (Node offline, disco, storage).",
      notify: "Leva alertas e eventos para fora do Portal.",
      integrations: "Chaves de serviços de terceiro. Ficam aqui, nunca no código — são suas.",
      workers: "Ritmo dos laços internos do Control Plane.",
      publish: "Publicação de conteúdo em trackers.",
    },
    subsections: {
      notify: {
        telegram: {
          title: "Telegram",
          blurb: "Bot do @BotFather + destino. Ativo quando token e chat_id estão preenchidos.",
        },
        discord: {
          title: "Discord",
          blurb: "Webhook de um canal do Discord.",
        },
        webhook: {
          title: "Webhook",
          blurb: "POST de JSON cru para uma URL sua — Slack, n8n, Home Assistant, o que for.",
        },
        rules: {
          title: "Regras de envio",
          blurb: "O que é enviado pelos canais acima, e o limite anti-flood.",
        },
      },
      integrations: {
        tmdb: {
          title: "TMDB",
          blurb: "Metadados de filmes e séries na publicação. É a sua chave (themoviedb.org).",
        },
        igdb: {
          title: "IGDB",
          blurb: "Metadados de jogos, via um app do Twitch.",
        },
        imgbb: {
          title: "imgbb",
          blurb: "Host das imagens dos screenshots da publicação.",
        },
        googleBooks: {
          title: "Google Books",
          blurb: "Segundo provedor de capa de livro/audiolivro, para o que a Open Library não indexa.",
        },
      },
      other: {
        title: "Outros",
        blurb: "Tunáveis desta categoria sem cartão próprio.",
      },
    },
    field: {
      clusterBadge: "cluster · por Node",
      clusterBadgeTitle: "Este é o valor de cluster. Cada Node pode ter um valor próprio, definido no detalhe do Node.",
      modified: "modificado",
      nothingSelected: "nada selecionado",
      selectedCount: {
        one: "{count} selecionado",
        other: "{count} selecionados",
      },
      optionOn: "Ligado",
      optionOff: "Desligado",
      placeholderSecretChange: "•••••••• — digite para trocar",
      placeholderNotConfigured: "não configurado",
      placeholderDuration: "5m, 90s, 168h",
      emptyValue: "vazio",
    },
    status: {
      on: "ligado",
      off: "desligado",
    },
    actions: {
      clear: "Limpar",
      resetDefault: "Restaurar padrão",
      resetDefaultTitle: "Voltar ao padrão (num campo de credencial, isto a apaga)",
      test: "Testar",
      sending: "enviando…",
      testEnabledTitle: "Envia uma mensagem de teste por este canal",
      testDisabledTitle: "Preencha as credenciais para testar",
    },
    confirm: {
      clusterScope: "Vale para TODOS os Nodes que não têm valor próprio, no próximo heartbeat de cada Agent, sem reiniciar nada.",
      changeCluster: "Mudar «{label}» para {value}{unit} no cluster?",
      resetCluster: "Restaurar «{label}» ao padrão ({defaultValue}) no cluster?",
    },
    messages: {
      applied: "Aplicado",
      saveFailed: "Falha ao salvar",
      loadFailed: "Falha ao carregar as configurações",
      testSent: "Enviado — confira o canal",
      sendFailed: "Falha ao enviar",
    },
  },
  specs: {
    admission: {
      interval: {
        label: "Intervalo de avaliação",
        description: "Período entre execuções do controlador de admissão. Menor = reage mais rápido a mudanças; maior = menos carga no banco/cluster.",
      },
      max_cluster_downloads: {
        label: "Limite de downloads (fallback)",
        description: "Teto de torrents baixando no cluster quando nenhum tracker reportou um L. 0 desliga o fallback (sem teto próprio).",
      },
      dl_slot_limit_ttl: {
        label: "Validade do L observado",
        description: "Por quanto tempo um L (limite de slots) observado na mensagem do tracker é considerado válido antes de expirar.",
      },
      min_dl_rate: {
        label: "Taxa mínima de download",
        description: "Abaixo desta taxa, sustentada, um download ativo é considerado lento e cede o slot na rotação por taxa saudável. 0 desliga a rotação por lentidão.",
      },
      disk_min_free: {
        label: "Margem de disco livre",
        description: "Folga de disco que nunca se compromete num Node. Um torrent só entra/migra para um Node se o espaço projetado livre (livre atual menos o que os torrents já presentes ainda vão baixar) comportar o TAMANHO TOTAL do torrent mais esta margem. Abaixo dela, o Node dispara o alerta \"disco cheio\". Default 5 GiB.",
      },
      slow_grace: {
        label: "Carência de lentidão",
        description: "Tempo contínuo abaixo da taxa mínima antes de rotacionar o download lento. Evita rotacionar por quedas passageiras. Mínimo 10min, independente da taxa configurada — descoberta/negociação inicial de peer real pode legitimamente comer boa parte de uma janela curta. Para desligar a rotação por lentidão, use admission.min_dl_rate=0 (não este campo).",
      },
      slow_cooldown: {
        label: "Cooldown de lentidão",
        description: "Quanto um download rotacionado por lentidão fica fora da fila de promoção, para não voltar já e causar thrashing.",
      },
      dead_grace: {
        label: "Carência de torrent morto",
        description: "Com 0 seeds E 0 peers contínuos por este tempo, o torrent é dado como morto (nunca completa) e cede o slot. Mínimo 5min (abaixo disso vira falso positivo — anúncio/descoberta de peer real pode legitimamente demorar mais que isso); 0 desliga a rotação por morto.",
      },
      dead_cooldown: {
        label: "Cooldown de torrent morto",
        description: "Quanto um morto rotacionado fica no fim da fila — não volta a ser promovido mesmo tendo o maior progresso.",
      },
      dead_backoff_max: {
        label: "Teto do backoff de torrent morto",
        description: "A cada vez seguida que o mesmo torrent é rotacionado por estar morto, o cooldown dobra (30m → 1h → 2h…) até este teto — um morto crônico se afasta sozinho da fila em vez de voltar a cada cooldown. Conectar em algum peer zera a contagem. 0 desliga o backoff (cooldown sempre fixo).",
      },
      priority_persistent: {
        label: "Prioridade em modo persistente",
        description: "Quando ligado, um torrent priorizado manualmente (botão ⚡) NÃO é rotacionado por estar morto (sem seeds/peers) ou lento — ele segura o slot até terminar de baixar. Desligado (padrão): a prioridade só fura a fila; depois de admitido, o torrent segue o fluxo normal e perde a prioridade se a rotação por morto/lento o despromover. Atenção: em modo persistente, um torrent priorizado sem seeds ocupa um slot sem baixar até você removê-lo ou surgirem seeds.",
      },
    },
    agent: {
      artifacts_url: {
        label: "URL dos artefatos do Agent",
        description: "Onde o Portal publica os binários do Agent e o manifesto agents.json (ex.: http://controller.example/node). O Control Plane lê o manifesto daqui para saber que versão está disponível, e o Agent BAIXA o binário desta mesma URL — ela precisa ser alcançável a partir dos Seed Nodes, não um endereço interno. Vazio desliga a atualização orquestrada (sem badge de update; o botão recusa).",
      },
      heartbeat_interval: {
        label: "Intervalo de heartbeat",
        description: "De quantos em quantos segundos o Agent avisa o Control Plane que está vivo e reporta recursos (e recebe esta configuração). Menor = o Portal e os alertas enxergam o Node mais cedo, ao custo de mais requisições. Mantenha bem abaixo de «Node offline após» (alerting.node_offline_after, 90s): acima da metade dele um heartbeat perdido já pode marcar o Node offline.",
      },
      command_poll_interval: {
        label: "Intervalo de consulta de comandos",
        description: "De quantos em quantos segundos o Agent pergunta ao Control Plane se há um comando para executar (pausar, migrar, atualizar…). É a latência de uma ação do operador até chegar ao Node.",
      },
      log_level: {
        label: "Nível de log do Agent",
        description: "Verbosidade do log do Agent no Node. `debug` ajuda a depurar um Node com problema (um override por Node evita sujar o log da frota inteira) e gera muito texto.",
      },
    },
    engine: {
      queueing_enabled: {
        label: "Fila do Engine",
        description: "Liga a fila do Engine, que limita quantos torrents ficam ativos ao mesmo tempo. É a proteção contra saturação de I/O: sem fila, adicionar muitos torrents dispara todos os downloads juntos (I/O em 100%, torrents presos em verificação e, em HD USB, queda do disco por subalimentação). Desligada, os limites «engine.max_active_*» abaixo deixam de valer.",
      },
      max_active_downloads: {
        label: "Downloads simultâneos por Node",
        description: "Teto de downloads que o Engine mantém ativos ao mesmo tempo num Node; o excedente espera em fila. Cada download concorrente custa hashing de peças e I/O: máquina fraca (Atom, HD USB) pede um número menor que a frota — ajuste com um override naquele Node, não baixando o valor de cluster. Quem garante o limite de slots do tracker no cluster inteiro é o controlador de admissão; este é o cinto-e-suspensório por máquina.",
      },
      max_active_uploads: {
        label: "Uploads simultâneos por Node",
        description: "Teto de torrents semeando ao mesmo tempo. O default é praticamente ilimitado DE PROPÓSITO: um seeder em fila (queuedUP) fica parado e manda event=stopped ao tracker, deixa de acumular seedtime e leva Hit and Run. Seeder nunca deve ser enfileirado.",
      },
      max_active_torrents: {
        label: "Torrents ativos no total por Node",
        description: "Teto de torrents ativos (baixando + semeando). Pelo mesmo motivo dos uploads, o default é praticamente ilimitado: só downloads e verificações devem ser enfileirados.",
      },
      max_active_checking: {
        label: "Verificações simultâneas por Node",
        description: "Quantos torrents o Engine verifica (hash das peças) ao mesmo tempo. Verificar lê o disco inteiro do torrent: mais de um de cada vez satura o I/O de um HD USB.",
      },
      dont_count_slow: {
        label: "Download lento deixa de contar no teto",
        description: "Desligado (padrão) DE PROPÓSITO: ligado, um download parado (0 KB/s, à espera de slot no tracker) não conta no teto, então o Engine ativa TODOS os downloads, todos anunciam de uma vez, o tracker recusa slots (thundering herd) e tudo trava. Desligado, o teto vale e o excedente fica em fila sem anunciar.",
      },
      slow_dl_threshold: {
        label: "Limiar de download lento",
        description: "Abaixo desta velocidade de download um torrent é considerado lento pelo Engine. Só tem efeito com «Download lento deixa de contar no teto» ligado.",
      },
      slow_ul_threshold: {
        label: "Limiar de upload lento",
        description: "Abaixo desta velocidade de upload um torrent é considerado lento pelo Engine. Só tem efeito com «Download lento deixa de contar no teto» ligado.",
      },
      slow_inactive_timer: {
        label: "Tempo lento até liberar o slot",
        description: "Quanto tempo contínuo abaixo do limiar o Engine espera antes de tratar o torrent como lento. Só tem efeito com «Download lento deixa de contar no teto» ligado.",
      },
    },
    integrations: {
      tmdb_api_key: {
        label: "TMDB — API key (v3)",
        description: "Chave v3 da API do TMDB (themoviedb.org) para enriquecer metadados na publicação. Vazio desabilita o enriquecimento. É a SUA chave — cadastre em themoviedb.org.",
      },
      igdb_client_id: {
        label: "IGDB — Client ID (Twitch)",
        description: "Client ID do app do Twitch usado pelo IGDB (metadados de jogos). Sem o par ID+Secret, a publicação de jogos fica desabilitada.",
      },
      igdb_client_secret: {
        label: "IGDB — Client Secret (Twitch)",
        description: "Client Secret do app do Twitch (client-credentials, máquina-a-máquina). Nunca é exibido de volta.",
      },
      imgbb_api_key: {
        label: "imgbb — API key",
        description: "Chave da API do imgbb.com para hospedar os screenshots da publicação. Crie a sua grátis em api.imgbb.com. Vazio desabilita a geração de screenshots.",
      },
      google_books_api_key: {
        label: "Google Books — API key",
        description: "Chave da API do Google Books, usada como SEGUNDO provedor de metadados de livro/audiolivro no catálogo — a Open Library não indexa boa parte do acervo em PT-BR nem lançamento recente. NECESSÁRIA na prática: sem chave a API responde 429 (Too Many Requests) depois de pouquíssimas buscas, e uma página do catálogo já passa disso — medido, não estimado. Vazio = o provedor fica inerte e o livro segue só com o que a Open Library confirmar. Crie a sua em console.cloud.google.com (habilite a Books API).",
      },
    },
    notify: {
      telegram_bot_token: {
        label: "Telegram — token do bot",
        description: "Token do bot criado no @BotFather (formato 123456:ABC-DEF…). Preencher liga o canal; restaurar o padrão desliga. Nunca é exibido de volta.",
      },
      telegram_chat_id: {
        label: "Telegram — chat_id do destino",
        description: "Para onde mandar (seu usuário, um grupo ou canal). Número, podendo ser negativo em grupos (ex.: -1001234567890). O canal só envia com token E chat_id preenchidos.",
      },
      discord_webhook_url: {
        label: "Discord — webhook URL",
        description: "URL de webhook do canal (Editar canal → Integrações → Webhooks). Preencher liga o canal. Nunca é exibida de volta.",
      },
      webhook_url: {
        label: "Webhook genérico — URL",
        description: "Recebe um POST de JSON cru a cada notificação (campos: kind, severity, title, message, node, at). Serve para Slack, n8n, Home Assistant ou qualquer coisa que aceite HTTP — sem precisar de suporte específico no SeedGrid. Nunca é exibida de volta.",
      },
      alerts_min_severity: {
        label: "Alertas — severidade mínima",
        description: "Quais alertas (Node offline, storage, disco saturado) são notificados. OFF desliga a trilha de alertas sem mexer nos canais. CRITICAL = só o que exige ação imediata; WARNING inclui degradação; INFO manda tudo.",
      },
      alerts_on_resolve: {
        label: "Alertas — avisar quando resolver",
        description: "Manda também quando o alerta se resolve (Node voltou, storage recuperou). Saber que voltou costuma importar tanto quanto saber que caiu — e evita ir olhar à toa.",
      },
      events: {
        label: "Eventos a avisar",
        description: "Fatos do dia a dia do cluster. Marque o que quer receber; sem nada marcado, não avisa nenhum.",
      },
      activities: {
        label: "Decisões de orquestração a avisar",
        description: "O que o SeedGrid faz sozinho para equilibrar os downloads. ⚠️ Costuma acontecer muitas vezes por hora — deixe desmarcado a menos que queira acompanhar de perto.",
      },
      language: {
        label: "Idioma das notificações",
        description: "Em que idioma saem as mensagens dos canais (Telegram, Discord, webhook). É do cluster, não de cada operador: a notificação vai para um canal. O Portal segue o idioma de quem o usa.",
      },
      rate_limit_per_min: {
        label: "Teto de mensagens por minuto",
        description: "Protege contra flood (ex.: Node flapando entre ONLINE/OFFLINE). O excedente da janela é descartado e o total suprimido vai para o log. 0 desliga o limite.",
      },
    },
    alerting: {
      worker_interval: {
        label: "Intervalo do Alert Worker",
        description: "Período entre avaliações das regras determinísticas de alerta.",
      },
      node_offline_after: {
        label: "Node offline após",
        description: "Tempo sem heartbeat para um Seed Node ser considerado offline (dispara alerta).",
      },
      disk_saturated_pct: {
        label: "Saturação de disco",
        description: "Percentual de utilização de I/O de disco a partir do qual o disco é considerado saturado (dispara alerta).",
      },
      disk_saturated_for: {
        label: "Saturação de disco sustentada",
        description: "Por quanto tempo o disco precisa ficar continuamente acima do limiar de saturação antes de abrir DISK_IO_SATURATED. A leitura vem do heartbeat (~10s) e oscila; exigir tempo contínuo evita alerta por pico de segundos. Uma verificação do Alert Worker abaixo do limiar zera a contagem; o alerta sai na primeira verificação depois que o tempo se cumpre. A contagem recomeça se o Controller reiniciar. A resolução não tem carência. 0 = alerta na primeira leitura.",
      },
      storage_grace: {
        label: "Carência de storage falho",
        description: "Quanto tempo o /data de um Node precisa ficar falhando antes de alertar (NODE_STORAGE). O heartbeat é de 30s e o alerta abria com UMA leitura: um soluço de I/O — esperado em HD USB subalimentado — virava alerta CRITICAL resolvido 30 segundos depois. A resolução NÃO tem carência: storage que voltou é boa notícia. 0 alerta na primeira leitura, como antes.",
      },
      firewalled_grace: {
        label: "Carência de connectability",
        description: "Tempo que um Node pode ficar sem receber nenhuma conexão de entrada antes de alertar (NODE_FIREWALLED). Todo Node nasce 'firewalled' até o primeiro peer chegar, então alertar de imediato seria alarme a cada restart do Engine. 0 desliga a regra.",
      },
      incoming_silence: {
        label: "Silêncio de entrada tolerado",
        description: "Quanto tempo um Node pode passar sem NENHUMA conexão de entrada, havendo demanda, antes de alertar (NODE_INBOUND_SILENT). Diferente da carência de connectability: aquela usa o veredito do Engine, que nunca volta atrás; esta usa um sinal que decai, e é a que pega Node que perde alcançabilidade em regime. Conexão de entrada é evento raro num acervo ocioso: um Node saudável passa dias sem receber nenhuma, e um valor baixo alerta à toa. O que conta como demanda é definido por \"Seeds por leecher que contam como demanda\". 0 desliga a regra.",
      },
      incoming_demand_seeds_per_leecher: {
        label: "Seeds por leecher que contam como demanda",
        description: "Um torrent só conta como demanda para NODE_INBOUND_SILENT quando o swarm tem no máximo este número de seeds por leecher (seeds <= leechers x valor). Num swarm farto, um leecher quase nunca abre conexão com um Node específico, então a ausência de entrada não diz nada sobre a porta. 1 conta só o swarm em que os leechers são ao menos tão numerosos quanto os seeds; 0 conta só o swarm sem nenhum seed; um valor muito alto (ex.: 100000) volta a contar qualquer leecher. Torrent sem a contagem de seeds ou de leechers não conta.",
      },
      ipv6_change_grace: {
        label: "Carência de troca de IPv6",
        description: "Quanto o endereço IPv6 novo precisa se sustentar antes de alertar (NODE_IPV6_CHANGED). As privacy extensions rotacionam o sufixo sozinhas; sem carência, um Node mal configurado alertaria a cada troca efêmera. 0 desliga a regra e fecha os alertas dela que estiverem abertos.",
      },
      reachability_ttl: {
        label: "Validade dos alertas de alcançabilidade",
        description: "Por quanto tempo NODE_FIREWALLED, NODE_INBOUND_SILENT e NODE_IPV6_CHANGED ficam abertos antes de fechar sozinhos. Os três deduzem alcançabilidade pela AUSÊNCIA de conexão de entrada e só resolvem quando uma chega — o que um Node sem demanda nunca recebe, mesmo com a porta aberta. O aviso sai na abertura; ao expirar, o alerta fecha sem notificar e não reabre enquanto a condição for a mesma (a mesma troca de IPv6, o mesmo silêncio). Conta desde a última notificação. 0 = sem validade: só fecha quando chega conexão de entrada.",
      },
      sample_retention: {
        label: "Retenção de amostras",
        description: "Por quanto tempo as amostras de métricas são mantidas antes da limpeza pelo worker.",
      },
    },
    commands: {
      redispatch_after: {
        label: "Re-entrega de comando órfão",
        description: "Idade a partir da qual um comando DISPATCHED sem confirmação do Agent é re-entregue no próximo poll (o Agent morreu entre pegar o comando e devolver o resultado). Deve ser MAIOR que o comando mais longo esperado: o Agent executa a leva em série e um migrate_pull grande segura a fila enquanto copia os dados. 0 desliga a re-entrega.",
      },
      max_attempts: {
        label: "Entregas máximas de um comando",
        description: "Quantas entregas ao Agent um comando pode receber antes de ser marcado FAILED como abandonado — evita loop infinito de re-entrega de um comando que sempre derruba o Agent.",
      },
    },
    workers: {
      maintenance_interval: {
        label: "Intervalo do Maintenance Worker",
        description: "Período da poda de retenção: comandos terminais, events, activities, audit e sessões expiradas.",
      },
      reconciler_interval: {
        label: "Intervalo do Migration Reconciler",
        description: "Período do reconciler que avança/finaliza migrações de torrent em voo.",
      },
      prom_writer_interval: {
        label: "Intervalo do Prometheus Targets Writer",
        description: "Período de reescrita do arquivo de targets (file_sd) do Prometheus com os Nodes ativos.",
      },
    },
    retention: {
      commands: {
        label: "Retenção de comandos",
        description: "Por quanto tempo comandos DONE/FAILED ficam no banco antes da poda. PENDING/DISPATCHED nunca são podados. 0 desliga.",
      },
      events: {
        label: "Retenção de eventos",
        description: "Por quanto tempo eventos ficam no banco antes da poda. 0 desliga.",
      },
      activities: {
        label: "Retenção do feed de atividade",
        description: "Por quanto tempo a trilha de orquestração fica no banco antes da poda. 0 desliga.",
      },
      audit: {
        label: "Retenção da auditoria",
        description: "Por quanto tempo a trilha de auditoria fica no banco antes da poda. Retenção maior por accountability. 0 desliga.",
      },
    },
    migration: {
      max_concurrent: {
        label: "Migrações simultâneas",
        description: "Quantas migrações de torrent podem transferir dados ao mesmo tempo. As demais aguardam na fila. Manter baixo: cada migração é uma cópia pesada de disco, e drenar um Node inteiro de uma vez satura o I/O justamente da máquina em apuros.",
      },
      recheck_stall_window: {
        label: "Estagnação do recheck da migração",
        description: "Quanto tempo o recheck no Node destino pode ficar sem avançar antes de a migração ser abortada como travada. É o que importa: enquanto o progresso sobe, a espera continua, por mais lento que seja o disco — só a estagnação reprova. Quem observa é o Control Plane, a cada passada do reconciler.",
      },
      recheck_max_wait: {
        label: "Teto do recheck da migração",
        description: "Teto absoluto, contado do fim da cópia, para o destino comprovar ter o que a origem tinha. Só pega um Engine que reporte progresso oscilante para sempre; a estagnação (acima) é a proteção de verdade. Como a espera deixou de segurar a fila de comandos do Node, o teto pode ser folgado — o custo de um teto alto é a vaga de migration.max_concurrent ficar ocupada.",
      },
    },
    metrics: {
      torrent_sample_interval: {
        label: "Intervalo de amostra por torrent",
        description: "Intervalo mínimo entre levas de amostras de série temporal por Node. O Agent sincroniza a cada ~15s; gravar amostra em todo sync é a maior fonte de crescimento do banco. 0 = amostra a cada sync.",
      },
      cluster_sample_interval: {
        label: "Intervalo de amostra do cluster",
        description: "Período entre snapshots do throughput acumulado do cluster (download/upload) gravados para o gráfico de evolução da página de Métricas (F2). Menor = curva mais fina e mais linhas no banco.",
      },
      cluster_sample_retention: {
        label: "Retenção das amostras do cluster",
        description: "Por quanto tempo os snapshots de throughput do cluster são mantidos. 0 = manter para sempre (a tabela é minúscula, ~1 linha por intervalo). Para o filtro de 1 ano funcionar, mantenha ≥ 1 ano ou 0.",
      },
    },
    publish: {
      screenshot_count: {
        label: "Screenshots por upload",
        description: "Quantos frames extrair do arquivo de vídeo ao gerar screenshots para publicação num tracker.",
      },
    },
  },
  channelSpecs: {
    custom: {
      label: "{channel} — filtros próprios",
      description: "Ligado, este canal usa os filtros abaixo em vez dos gerais. Desligado (padrão), segue os filtros gerais de notificação.",
    },
    alerts_min_severity: {
      label: "{channel} — severidade mínima de alerta",
      description: "Só vale com os filtros próprios ligados. OFF não manda alerta nenhum neste canal.",
    },
    alerts_on_resolve: {
      label: "{channel} — avisar quando o alerta resolve",
      description: "Só vale com os filtros próprios ligados.",
    },
    events: {
      label: "{channel} — eventos",
      description: "Só vale com os filtros próprios ligados. Sem nada marcado, não manda evento neste canal.",
    },
    activities: {
      label: "{channel} — decisões de orquestração",
      description: "Só vale com os filtros próprios ligados. Sem nada marcado, não manda nenhuma neste canal.",
    },
    rate_limit_per_min: {
      label: "{channel} — teto de mensagens por minuto",
      description: "Só vale com os filtros próprios ligados. 0 = sem teto — use 0 num canal que dispara automação, para uma rajada de avisos não descartar o gatilho. ⚠️ ALERTA nunca entra neste teto, em canal nenhum: alerta existe para incomodar.",
    },
  },
  choices: {
    event: {
      NODE_REGISTERED: "Node entrou no cluster",
      NODE_DELETED: "Node removido",
      TORRENT_CREATED: "Torrent adicionado",
      TORRENT_REMOVED: "Torrent removido",
      TORRENT_PAUSED: "Torrent pausado",
      TORRENT_RESUMED: "Torrent retomado",
      TORRENT_RECHECK: "Torrent em reverificação",
      TORRENT_MIGRATION_STARTED: "Migração de torrent começou",
      TORRENT_MIGRATED: "Torrent mudou de Node",
      TORRENT_MIGRATION_FAILED: "Falha ao migrar torrent",
      TORRENT_RESEEDED: "Torrent recolocado no ar",
      TORRENT_PUBLISHED_EXTERNAL: "Torrent publicado no tracker",
      ENGINE_RESTART: "Motor de torrent reiniciado",
      AGENT_UPDATE: "Agent atualizado",
      STORAGE_RECOVERY: "Disco de um Node reconectado",
    },
    activity: {
      admission_hold: "Download pausado para liberar uma vaga",
      admission_resume: "Download retomado quando abriu vaga",
      scheduled: "Torrent atribuído a um Node",
      downloading: "Torrent começou a baixar",
      seeding: "Torrent começou a semear",
      node_blocked: "Torrent não pôde ser agendado (Node com disco falhando)",
      screenshot_host_failed: "Host de imagem falhou no upload de screenshot",
    },
  },
  categoryNames: {
    admission: "Admissão",
    agent: "Agent / Engine",
    alerting: "Alertas",
    notify: "Notificações",
    integrations: "Integrações",
    workers: "Workers",
    publish: "Publicação",
  },
} as const;
