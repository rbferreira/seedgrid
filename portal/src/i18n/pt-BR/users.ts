export const users = {
  users: {
    title: "Usuários",
    loading: "Carregando usuários…",
    roleDesc: {
      administrator: "Acesso total",
      publisher: "Gerencia conteúdo (torrents, trackers, publicação)",
      downloader: "Lê e baixa conteúdo",
      viewer: "Somente leitura",
    },
    actions: {
      new: "＋ Novo usuário",
      create: "Criar usuário",
      creating: "Criando...",
      edit: "Editar",
      disable: "Desabilitar",
    },
    form: {
      title: "Novo operador",
      username: "Username *",
      displayName: "Nome de exibição",
      email: "E-mail",
      password: "Senha inicial *",
      roles: "Papéis",
      rolesHint: "Dica: um operador com múltiplos papéis soma as permissões.",
    },
    table: {
      user: "Usuário",
      roles: "Papéis",
      status: "Status",
      lastLogin: "Último login",
      never: "nunca",
    },
    edit: {
      passwordPlaceholder: "Nova senha (opcional)",
    },
    confirm: {
      disable: {
        title: "Desabilitar o usuário «{username}»?",
        message: "O acesso é cortado imediatamente.",
        confirm: "Desabilitar",
      },
    },
    notice: {
      created: "✓ Usuário criado.",
      updated: "✓ Usuário «{username}» atualizado.",
      disabled: "✓ Usuário «{username}» desabilitado.",
    },
    error: {
      load: "Falha ao carregar usuários.",
      createValidation: "Username obrigatório e senha com ao menos 6 caracteres.",
      create: "Falha ao criar usuário.",
      update: "Falha ao atualizar usuário.",
      disable: "Falha ao desabilitar usuário.",
    },
    footer: {
      roles: "Papéis de sistema: {roles}",
    },
  },
} as const;
