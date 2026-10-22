package notify

import "strings"

// Textos das notificações (ADR-041 §4). O idioma é do CLUSTER (notify.language), não de cada
// operador: a notificação é uma mensagem para um canal, não para um leitor. As chaves de
// título cobrem todo código de messages.Registry (catalog_test.go confere), e os dois idiomas
// têm as mesmas chaves. A frase pode divergir da do Portal: notificação e tela têm tamanhos
// diferentes.

const defaultLang = "en"

var catalog = map[string]map[string]string{
	"pt-BR": {
		"resolved":    "Resolvido",
		"line.node":   "Node",
		"line.date":   "Data",
		"layout.date": "02/01 15:04:05",
		"yes":         "Sim",

		"test.title": "SeedGrid — teste de notificação",
		"test.body":  "Se você está lendo isto, o canal está funcionando. Disparado por {who}.",

		"label.file":                    "Arquivo",
		"label.name":                    "Nome",
		"label.size":                    "Tamanho",
		"label.added_at":                "Adicionado em",
		"label.completed_at":            "Concluído em",
		"label.reason":                  "Motivo",
		"label.score":                   "Pontuação",
		"label.candidates":              "Candidatos",
		"label.node_out":                "Node fora",
		"label.old_hash":                "Hash antigo",
		"label.new_hash":                "Hash novo",
		"label.destination":             "Destino",
		"label.source":                  "Origem",
		"label.author":                  "Autor",
		"label.tracker":                 "Tracker",
		"label.agent_version":           "Versão do Agent",
		"label.torrents_removed":        "Torrents removidos",
		"label.forced":                  "Forçado",
		"label.from":                    "De",
		"label.to":                      "Para",
		"label.step":                    "Etapa",
		"label.attempt":                 "Tentativa",
		"label.detail":                  "Detalhe",
		"label.missing":                 "Faltam",
		"label.min_floor":               "Piso mínimo",
		"label.io_util":                 "Utilização de I/O",
		"label.previous":                "Anterior",
		"label.current":                 "Atual",
		"label.bt_port":                 "Porta BT",
		"label.node_address":            "Endereço do Node",
		"label.last_inbound":            "Última entrada",
		"label.firewalled_since":        "Firewalled desde",
		"reason.cap":                    "teto de admissão atingido",
		"reason.slow":                   "taxa baixa, cedeu o slot",
		"reason.dead":                   "sem seeds/peers (torrent morto)",
		"reason.manual_pause_adopted":   "pausa manual adotada (Node em modo SG)",
		"reason.best_health":            "maior health score entre os Nodes elegíveis",
		"reason.best_health_with_space": "maior health score entre os Nodes com espaço para o torrent",
		"reason.storage_failed":         "storage falho",

		"title.alert.DISK_IO_SATURATED":       "Disco do Node saturado (I/O)",
		"title.alert.NODE_DISK_FULL":          "Node sem espaço para concluir a fila",
		"title.alert.NODE_FIREWALLED":         "Node inalcançável de fora (firewalled)",
		"title.alert.NODE_INBOUND_SILENT":     "Node sem conexão de entrada",
		"title.alert.NODE_IPV6_CHANGED":       "IPv6 do Node mudou",
		"title.alert.NODE_OFFLINE":            "Node sem heartbeat: marcado OFFLINE",
		"title.alert.NODE_STORAGE":            "Storage do Node não está montado/gravável",
		"title.alert.STORAGE_RECOVERY_FAILED": "Recuperação automática do storage falhou",
		"title.alert.DiskSpaceLow":            "Disco baixo no host",
		"title.alert.HighMemory":              "Memória alta no host",
		"title.alert.NodeExporterDown":        "Métricas de host indisponíveis",

		"title.event.AGENT_UPDATE":               "Atualização do Agent solicitada",
		"title.event.ENGINE_RESTART":             "Reinício do Torrent Engine solicitado",
		"title.event.NODE_ADOPTED_MANUAL":        "Node entrou com o próprio qBittorrent, em Manual",
		"title.event.NODE_CONFIG_SEEDED":         "Valores do .env viraram override por Node",
		"title.event.NODE_DELETED":               "Node removido do cluster",
		"title.event.NODE_REGISTERED":            "Node entrou no cluster",
		"title.event.STORAGE_RECOVERY":           "Recuperação do storage",
		"title.event.TORRENT_CREATED":            "Torrent adicionado ao Node",
		"title.event.TORRENT_CREATED.created":    "Conteúdo publicado (torrent criado no Node)",
		"title.event.TORRENT_MIGRATED":           "Torrent migrado com sucesso",
		"title.event.TORRENT_MIGRATION_FAILED":   "Migração de torrent abortada",
		"title.event.TORRENT_MIGRATION_STARTED":  "Migração de torrent iniciada",
		"title.event.TORRENT_PAUSED":             "Torrent pausado",
		"title.event.TORRENT_PUBLISHED_EXTERNAL": "Torrent publicado no tracker",
		"title.event.TORRENT_RECHECK":            "Reverificação de torrent solicitada",
		"title.event.TORRENT_REMOVED":            "Torrent removido",
		"title.event.TORRENT_RESEEDED":           "Torrent recolocado no ar (re-seed)",
		"title.event.TORRENT_RESUMED":            "Torrent retomado",

		"title.activity.admission_hold":         "Download em hold",
		"title.activity.admission_resume":       "Download promovido — slot liberado",
		"title.activity.downloading":            "Começou a baixar",
		"title.activity.node_blocked":           "Torrent não pôde ser agendado",
		"title.activity.scheduled":              "Agendado — o Scheduler escolheu o Node",
		"title.activity.screenshot_host_failed": "Host de imagem falhou",
		"title.activity.seeding":                "Download concluído - Seedando 🌱",
	},
	"en": {
		"resolved":    "Resolved",
		"line.node":   "Node",
		"line.date":   "Date",
		"layout.date": "Jan 2 15:04:05",
		"yes":         "Yes",

		"test.title": "SeedGrid — notification test",
		"test.body":  "If you're reading this, the channel works. Triggered by {who}.",

		"label.file":                    "File",
		"label.name":                    "Name",
		"label.size":                    "Size",
		"label.added_at":                "Added at",
		"label.completed_at":            "Completed at",
		"label.reason":                  "Reason",
		"label.score":                   "Score",
		"label.candidates":              "Candidates",
		"label.node_out":                "Node left out",
		"label.old_hash":                "Old hash",
		"label.new_hash":                "New hash",
		"label.destination":             "Destination",
		"label.source":                  "Source",
		"label.author":                  "Author",
		"label.tracker":                 "Tracker",
		"label.agent_version":           "Agent version",
		"label.torrents_removed":        "Torrents removed",
		"label.forced":                  "Forced",
		"label.from":                    "From",
		"label.to":                      "To",
		"label.step":                    "Step",
		"label.attempt":                 "Attempt",
		"label.detail":                  "Detail",
		"label.missing":                 "Short by",
		"label.min_floor":               "Minimum floor",
		"label.io_util":                 "I/O utilization",
		"label.previous":                "Previous",
		"label.current":                 "Current",
		"label.bt_port":                 "BT port",
		"label.node_address":            "Node address",
		"label.last_inbound":            "Last inbound",
		"label.firewalled_since":        "Firewalled since",
		"reason.cap":                    "admission cap reached",
		"reason.slow":                   "slow, gave up its slot",
		"reason.dead":                   "no seeds/peers (dead torrent)",
		"reason.manual_pause_adopted":   "manual pause adopted (Node in SG mode)",
		"reason.best_health":            "highest health score among eligible Nodes",
		"reason.best_health_with_space": "highest health score among Nodes with room for the torrent",
		"reason.storage_failed":         "storage failing",

		"title.alert.DISK_IO_SATURATED":       "Node disk saturated (I/O)",
		"title.alert.NODE_DISK_FULL":          "Node out of room to finish its queue",
		"title.alert.NODE_FIREWALLED":         "Node unreachable from outside (firewalled)",
		"title.alert.NODE_INBOUND_SILENT":     "Node with no inbound connections",
		"title.alert.NODE_IPV6_CHANGED":       "Node IPv6 changed",
		"title.alert.NODE_OFFLINE":            "Node missed its heartbeat: marked OFFLINE",
		"title.alert.NODE_STORAGE":            "Node storage not mounted/writable",
		"title.alert.STORAGE_RECOVERY_FAILED": "Automatic storage recovery failed",
		"title.alert.DiskSpaceLow":            "Low disk on the host",
		"title.alert.HighMemory":              "High memory on the host",
		"title.alert.NodeExporterDown":        "Host metrics unavailable",

		"title.event.AGENT_UPDATE":               "Agent update requested",
		"title.event.ENGINE_RESTART":             "Torrent Engine restart requested",
		"title.event.NODE_ADOPTED_MANUAL":        "Node joined with its own qBittorrent, in Manual",
		"title.event.NODE_CONFIG_SEEDED":         ".env values became per-Node overrides",
		"title.event.NODE_DELETED":               "Node removed from the cluster",
		"title.event.NODE_REGISTERED":            "Node joined the cluster",
		"title.event.STORAGE_RECOVERY":           "Storage recovery",
		"title.event.TORRENT_CREATED":            "Torrent added to the Node",
		"title.event.TORRENT_CREATED.created":    "Content published (torrent created on the Node)",
		"title.event.TORRENT_MIGRATED":           "Torrent migrated successfully",
		"title.event.TORRENT_MIGRATION_FAILED":   "Torrent migration aborted",
		"title.event.TORRENT_MIGRATION_STARTED":  "Torrent migration started",
		"title.event.TORRENT_PAUSED":             "Torrent paused",
		"title.event.TORRENT_PUBLISHED_EXTERNAL": "Torrent published to the tracker",
		"title.event.TORRENT_RECHECK":            "Torrent recheck requested",
		"title.event.TORRENT_REMOVED":            "Torrent removed",
		"title.event.TORRENT_RESEEDED":           "Torrent back online (re-seed)",
		"title.event.TORRENT_RESUMED":            "Torrent resumed",

		"title.activity.admission_hold":         "Download on hold",
		"title.activity.admission_resume":       "Download promoted — slot freed",
		"title.activity.downloading":            "Started downloading",
		"title.activity.node_blocked":           "Torrent could not be scheduled",
		"title.activity.scheduled":              "Scheduled — the Scheduler picked the Node",
		"title.activity.screenshot_host_failed": "Image host failed",
		"title.activity.seeding":                "Download complete - Seeding 🌱",
	},
}

// lang: o idioma configurado para as notificações; desconhecido ou vazio vira o padrão.
func (s *Service) lang() string {
	if l := s.cfg.String("notify.language"); catalog[l] != nil {
		return l
	}
	return defaultLang
}

// tr: o texto da chave no idioma, com {nome} trocado por params. Chave que falta no idioma
// cai no padrão; que falta em todos, devolve "" (quem chama decide a reserva).
func tr(lang, key string, params ...string) string {
	t, ok := catalog[lang][key]
	if !ok {
		t = catalog[defaultLang][key]
	}
	for i := 0; i+1 < len(params); i += 2 {
		t = strings.ReplaceAll(t, "{"+params[i]+"}", params[i+1])
	}
	return t
}

// titleFor: o título do código no idioma; sem entrada no catálogo, o message gravado (e, sem
// ele, o próprio código).
func titleFor(lang, kind, code, message string) string {
	if t := tr(lang, "title."+kind+"."+code); t != "" {
		return t
	}
	return titleOr(message, code)
}

// TestMessage é a mensagem do botão "enviar teste", no idioma das notificações.
func (s *Service) TestMessage(who string) Message {
	l := s.lang()
	return Message{
		Kind:     KindTest,
		Severity: "INFO",
		Title:    tr(l, "test.title"),
		Message:  tr(l, "test.body", "who", who),
		Lang:     l,
	}
}
