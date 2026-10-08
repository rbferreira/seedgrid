export const audit = {
  log: {
    title: "Auditoria",
    description: "O que cada operador fez no SeedGrid — em ordem, com quem, quando e de onde. Trilha separada de Alertas, Eventos e Feed.",
    updatedAt: "atualizado {time}",
    filter: {
      all: "Todos",
    },
    error: {
      load: "Falha ao carregar a auditoria",
    },
    status: {
      failed: "falhou",
      failedWithCode: "falhou ({code})",
    },
    time: {
      seconds: "há {count}s",
      minutes: "há {count}min",
      hours: "há {count}h",
      days: "há {count}d",
    },
    empty: "Nenhuma ação registrada ainda. Login e mudanças (torrents, Nodes, configuração, modo) aparecem aqui.",
  },
  importSections: {
    description: {
      new: "; composição da descrição: aplicada",
      replaced: "; composição da descrição: substituída a pedido",
      protected: "; composição da descrição: mantida a do operador",
      same: "; composição da descrição: já era igual",
    },
    image_host: {
      new: "; host de imagem: aplicado",
      replaced: "; host de imagem: substituído a pedido",
      protected: "; host de imagem: mantido o do operador",
      same: "; host de imagem: já era igual",
    },
  },
} as const;
