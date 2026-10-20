export const ui = {
  chart: {
    noData: "Sem dados no período.",
  },
  ui: {
    healthBadge: {
      label: "health",
    },
  },
  newVersion: {
    title: "Há uma versão nova do SeedGrid",
    body: "Esta aba ainda roda a versão anterior. Recarregue para usar a nova — algumas telas podem não funcionar direito até lá.",
    reload: "Recarregar",
  },
  otherVersions: {
    title: "Você já tem este título",
    summary: {
      one: "O cluster já tem uma versão de {title}. Quer adicionar mais esta?",
      other: "O cluster já tem {count} versões de {title}. Quer adicionar mais esta?",
    },
    inCluster: "Já no cluster",
    adding: "Você vai adicionar",
    unknownResolution: "resolução ?",
    unknownCodec: "codec ?",
    onNode: "no {node}",
    cancel: "Cancelar",
    confirm: "Adicionar mesmo assim",
  },
} as const;
