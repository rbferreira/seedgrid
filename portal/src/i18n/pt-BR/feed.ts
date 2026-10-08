export const feed = {
  feed: {
    score: "score {score}",
    title: "Feed",
    live: "ao vivo",
    description: "As decisões de orquestração do SeedGrid e as transições dos torrents, em tempo real — separado de Alertas e Eventos.",
    updatedAt: "atualizado {time}",
    error: {
      loadFailed: "Falha ao carregar o feed",
    },
    empty: "Nenhuma atividade ainda. Assim que o SeedGrid tomar decisões (agendar, dar hold, promover) ou um torrent mudar de estado, a mágica aparece aqui.",
    kind: {
      admissionHold: "Hold",
      admissionResume: "Promovido",
      scheduled: "Agendado",
      downloading: "Baixando",
      seeding: "Seedando",
      screenshotHostFailed: "Host de imagem",
      fallback: "Atividade",
    },
    filter: {
      all: "Tudo",
    },
    time: {
      secondsAgo: {
        one: "há {count}s",
        other: "há {count}s",
      },
      minutesAgo: {
        one: "há {count}min",
        other: "há {count}min",
      },
      hoursAgo: {
        one: "há {count}h",
        other: "há {count}h",
      },
      daysAgo: {
        one: "há {count}d",
        other: "há {count}d",
      },
    },
  },
} as const;
